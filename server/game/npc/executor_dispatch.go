// 指令分发：RMMV 事件指令列表的主执行循环与公共事件调用。
package npc

import (
	"context"
	"strings"
	"time"

	"github.com/kasuganosora/rpgmakermvmmo/server/game/player"
	"github.com/kasuganosora/rpgmakermvmmo/server/resource"
	"go.uber.org/zap"
)

// maxCallDepth 防止公共事件之间的无限递归调用。
const maxCallDepth = 10

// executeList 执行指令列表。返回 true 表示遇到终止指令（CmdEnd 或 CmdExitEvent）。
// depth 参数追踪公共事件递归深度，超过 maxCallDepth 时停止执行。
func (e *Executor) executeList(ctx context.Context, s *player.PlayerSession, cmds []*resource.EventCommand, opts *ExecuteOpts, depth int) bool {
	if depth > maxCallDepth {
		e.logger.Warn("common event call depth exceeded", zap.Int("depth", depth))
		return false
	}
	battleResult := map[int]int{} // indent → 战斗结果 (0=胜利, 1=逃跑, 2=败北)
	for i := 0; i < len(cmds); i++ {
		// 检查上下文取消
		select {
		case <-ctx.Done():
			return true
		default:
		}

		cmd := cmds[i]
		if cmd == nil {
			continue
		}
		switch cmd.Code {
		case CmdEnd:
			// 代码 0 同时作为子块标记（条件块内缩进 > 0）和列表终止符（缩进 0，末尾指令）。
			// RMMV 中代码 0 为空操作，解释器直接跳过。仅缩进 0 时视为真正的终止符。
			if cmd.Indent == 0 {
				return true
			}

		case CmdShowText:
			// 聚合 ShowText + ShowTextLine 序列为完整对话
			var lines []string
			face := paramStr(cmd.Parameters, 0)
			faceIndex := paramInt(cmd.Parameters, 1)
			background := paramInt(cmd.Parameters, 2)  // 0=窗口, 1=暗化, 2=透明
			positionType := paramInt(cmd.Parameters, 3) // 0=上, 1=中, 2=下
			for i+1 < len(cmds) && cmds[i+1] != nil && cmds[i+1].Code == CmdShowTextLine {
				i++
				lines = append(lines, paramStr(cmds[i].Parameters, 0))
			}
			// 服务端解析文本转义码（\N[n], \V[n], \P[n]）
			lines = e.resolveDialogLines(lines, s, opts)

			// RMMV：文本紧跟选项时合并显示
			if i+1 < len(cmds) && cmds[i+1] != nil && cmds[i+1].Code == CmdShowChoices {
				i++ // 消费选项指令
				choicesCmd := cmds[i]
				choices := paramList(choicesCmd.Parameters, 0)
				cancelType := paramInt(choicesCmd.Parameters, 1)
				choiceDefault := paramInt(choicesCmd.Parameters, 2)
				choicePosition := 2 // 默认=右
				if len(choicesCmd.Parameters) > 3 {
					choicePosition = paramInt(choicesCmd.Parameters, 3)
				}
				choiceBg := 0 // 默认=窗口
				if len(choicesCmd.Parameters) > 4 {
					choiceBg = paramInt(choicesCmd.Parameters, 4)
				}
				choices = e.resolveChoices(choices, s, opts)
				e.sendDialogWithChoices(s, face, faceIndex, background, positionType, lines,
					choices, choiceDefault, cancelType, choicePosition, choiceBg)

				choiceIdx := e.waitForChoice(ctx, s)
				if choiceIdx == -1 {
					return true // 连接断开或上下文取消
				}
				i = e.skipToChoiceBranch(cmds, i, choiceIdx, cancelType)
			} else {
				e.sendDialog(s, face, faceIndex, background, positionType, lines)
				if !e.waitForDialogAck(ctx, s) {
					return true
				}
			}

		case CmdShowChoices:
			// 独立选项指令（未与文本合并时）
			choices := paramList(cmd.Parameters, 0)
			choices = e.resolveChoices(choices, s, opts)
			cancelType := paramInt(cmd.Parameters, 1) // -1=禁止取消, 0-N=分支索引
			choiceDefault := paramInt(cmd.Parameters, 2)
			choicePosition := 2
			if len(cmd.Parameters) > 3 {
				choicePosition = paramInt(cmd.Parameters, 3)
			}
			choiceBg := 0
			if len(cmd.Parameters) > 4 {
				choiceBg = paramInt(cmd.Parameters, 4)
			}
			e.sendChoices(s, choices, choiceDefault, cancelType, choicePosition, choiceBg)

			// 等待玩家选择
			choiceIdx := e.waitForChoice(ctx, s)
			if choiceIdx == -1 {
				return true // 连接断开或上下文取消
			}

			// 跳转到匹配的 When 分支（代码 402）或取消分支（代码 403）
			i = e.skipToChoiceBranch(cmds, i, choiceIdx, cancelType)

		case CmdWhenBranch, CmdWhenCancel:
			// 正常流程中遇到 When 分支，说明已执行选定分支，需跳到 BranchEnd
			i = e.skipToBranchEnd(cmds, i, cmd.Indent)

		case CmdBranchEnd:
			// 正常流程，继续执行

		case CmdConditionalStart:
			// 条件分支：评估条件，为假则跳到 Else 或 End
			if !e.evaluateCondition(cmd.Parameters, opts) {
				i = e.skipToElseOrEnd(cmds, i, cmd.Indent)
			}

		case CmdElseBranch:
			// 正常流程中到达 Else，说明 if 分支已执行，跳到 ConditionalEnd
			i = e.skipToConditionalEnd(cmds, i, cmd.Indent)

		case CmdConditionalEnd:
			// 正常流程，继续执行

		case CmdLoop:
			// 循环起始标记，循环体紧随其后。RepeatAbove（413）回跳到此处。

		case CmdRepeatAbove:
			// 回跳到同缩进层级的 Loop（112）
			i = e.jumpToLoopStart(cmds, i, cmd.Indent)

		case CmdBreakLoop:
			// 跳出循环：跳到 RepeatAbove（413）之后的指令
			i = e.skipPastLoopEnd(cmds, i, cmd.Indent)

		case CmdExitEvent:
			// 立即终止事件执行
			return true

		case CmdLabel:
			// 标签标记，作为 JumpToLabel 的目标

		case CmdJumpToLabel:
			// 跳转到指定名称的标签
			labelName := paramStr(cmd.Parameters, 0)
			jumped := e.jumpToLabel(cmds, labelName)
			if jumped >= 0 {
				i = jumped
			}
			// 标签未找到时继续执行（RMMV 行为）

		case CmdCallCommonEvent:
			// 调用公共事件
			ceID := paramInt(cmd.Parameters, 0)
			e.callCommonEvent(ctx, s, ceID, opts, depth)

		case CmdChangeSwitches:
			e.applySwitches(s, cmd.Parameters, opts)

		case CmdChangeVars:
			e.applyVariables(s, cmd.Parameters, opts)

		case CmdChangeSelfSwitch:
			e.applySelfSwitch(cmd.Parameters, opts)

		case CmdChangeGold:
			if err := e.applyGold(ctx, s, cmd.Parameters, opts); err != nil {
				e.logger.Warn("ChangeGold failed", zap.Int64("char_id", s.CharID), zap.Error(err))
			}

		case CmdChangeItems:
			if err := e.applyItems(ctx, s, cmd.Parameters, opts); err != nil {
				e.logger.Warn("ChangeItems failed", zap.Int64("char_id", s.CharID), zap.Error(err))
			}

		case CmdChangeWeapons, CmdChangeArmors:
			// 武器/防具变更 — 参数格式同物品变更，转发给客户端
			// TODO: 服务端库存追踪（当前仅追踪普通物品）
			e.sendEffect(s, cmd)

		case CmdTransfer:
			e.transferPlayer(s, cmd.Parameters, opts)

		case CmdWait:
			// 等待 N 帧；60fps 下 frames/60 秒
			frames := paramInt(cmd.Parameters, 0)
			if frames > 0 {
				wait := time.Duration(frames) * time.Second / 60
				select {
				case <-time.After(wait):
				case <-ctx.Done():
					return true
				}
			}

		case CmdScript:
			// 拼接代码 355 + 后续 655 续行
			script := paramStr(cmd.Parameters, 0)
			for i+1 < len(cmds) && cmds[i+1] != nil && cmds[i+1].Code == CmdScriptCont {
				i++
				script += "\n" + paramStr(cmds[i].Parameters, 0)
			}
			// 仅转发安全的视觉/音效指令行，过滤潜在的状态修改指令
			var safeLines []string
			for _, line := range strings.Split(script, "\n") {
				trimmed := strings.TrimSpace(line)
				if trimmed == "" {
					continue
				}
				if strings.HasPrefix(trimmed, "$gameScreen.") ||
					strings.HasPrefix(trimmed, "AudioManager.") {
					safeLines = append(safeLines, trimmed)
				}
			}
			if len(safeLines) > 0 {
				e.sendEffect(s, &resource.EventCommand{
					Code:       CmdScript,
					Parameters: []interface{}{strings.Join(safeLines, "\n")},
				})
			}

		case CmdScriptCont:
			// 已由 CmdScript 处理，单独出现时跳过

		case CmdPluginCommand:
			// 检查是否为 TemplateEvent.js 指令
			if e.handleTECallOriginEvent(ctx, s, cmd, opts, depth) {
				continue
			}
			// 过滤立绘/演出指令（依赖复杂客户端状态，转发会导致视觉异常）
			if pluginStr := paramStr(cmd.Parameters, 0); strings.HasPrefix(pluginStr, "CallStand") ||
				strings.HasPrefix(pluginStr, "CallCutin") ||
				strings.HasPrefix(pluginStr, "EraceStand") ||
				strings.HasPrefix(pluginStr, "EraceCutin") ||
				strings.HasPrefix(pluginStr, "CallAM") {
				continue
			}
			// 其他插件指令转发给客户端执行
			e.sendEffect(s, cmd)

		case CmdSetMoveRoute:
			// 解析 charId=0（当前事件）为实际事件 ID 后转发
			e.sendMoveRoute(s, cmd, opts)
			// 跳过续行（505）
			for i+1 < len(cmds) && cmds[i+1] != nil && cmds[i+1].Code == CmdMoveRouteCont {
				i++
			}
			// 若移动路线设置了 wait=true，估算时长并等待
			if len(cmd.Parameters) > 1 {
				if mr, ok := cmd.Parameters[1].(map[string]interface{}); ok {
					if w, ok := mr["wait"]; ok && asBool(w) {
						frames := estimateMoveRouteFrames(mr)
						e.waitFrames(ctx, frames)
					}
				}
			}

		case CmdMoveRouteCont:
			// 已由 CmdSetMoveRoute 消费，跳过

		case CmdWaitForMoveRoute:
			// 等待最后一个移动路线完成（使用默认时长，因为服务端不追踪客户端状态）
			e.waitFrames(ctx, 60)

		case CmdFadeout:
			// RMMV 中淡出总是等待 fadeSpeed()=24 帧
			e.sendEffect(s, cmd)
			e.waitFrames(ctx, 24)

		case CmdFadein:
			// RMMV 中淡入总是等待 fadeSpeed()=24 帧
			e.sendEffect(s, cmd)
			e.waitFrames(ctx, 24)

		case CmdTintScreen:
			e.sendEffect(s, cmd)
			// params[2]=true 时等待色调变化完成
			if len(cmd.Parameters) > 2 && asBool(cmd.Parameters[2]) {
				frames := paramInt(cmd.Parameters, 1)
				if frames > 0 {
					e.waitFrames(ctx, frames)
				}
			}

		case CmdFlashScreen:
			e.sendEffect(s, cmd)
			// params[2]=true 时等待闪烁完成
			if len(cmd.Parameters) > 2 && asBool(cmd.Parameters[2]) {
				frames := paramInt(cmd.Parameters, 1)
				if frames > 0 {
					e.waitFrames(ctx, frames)
				}
			}

		case CmdShakeScreen:
			e.sendEffect(s, cmd)
			// params[3]=true 时等待震动完成
			if len(cmd.Parameters) > 3 && asBool(cmd.Parameters[3]) {
				frames := paramInt(cmd.Parameters, 2)
				if frames > 0 {
					e.waitFrames(ctx, frames)
				}
			}

		case CmdChangeTransparency:
			e.sendEffect(s, cmd)

		case CmdPlayBGM, CmdStopBGM, CmdPlayBGS, CmdStopBGS, CmdPlaySE, CmdStopSE, CmdPlayME:
			// 音频指令转发给客户端
			e.sendEffect(s, cmd)

		case CmdShowPicture:
			// 解析变量坐标后转发
			e.sendShowPicture(s, cmd.Parameters, opts)

		case CmdMovePicture:
			e.sendMovePicture(ctx, s, cmd.Parameters, opts)

		case CmdRotatePicture, CmdTintPicture, CmdErasePicture:
			e.sendEffect(s, cmd)

		case CmdShowAnimation, CmdShowBalloon:
			// 显示动画/气泡 — 解析 charId=0 为事件 ID
			charID := paramInt(cmd.Parameters, 0)
			if charID == 0 && opts != nil && opts.EventID > 0 {
				resolved := make([]interface{}, len(cmd.Parameters))
				copy(resolved, cmd.Parameters)
				resolved[0] = float64(opts.EventID)
				e.sendEffect(s, &resource.EventCommand{Code: cmd.Code, Parameters: resolved})
			} else {
				e.sendEffect(s, cmd)
			}

		case CmdEraseEvent:
			// 暂时隐藏事件，转发给客户端
			e.sendEffect(s, cmd)

		case CmdChangeHP:
			e.applyChangeHP(ctx, s, cmd.Parameters, opts)

		case CmdChangeMP:
			e.applyChangeMP(ctx, s, cmd.Parameters, opts)

		case CmdChangeState:
			e.applyChangeState(ctx, s, cmd.Parameters, opts)

		case CmdRecoverAll:
			e.applyRecoverAll(ctx, s, cmd.Parameters, opts)

		case CmdChangeEXP:
			e.applyChangeEXP(ctx, s, cmd.Parameters, opts)

		case CmdChangeLevel:
			e.applyChangeLevel(ctx, s, cmd.Parameters, opts)

		case CmdChangeParameter:
			// 能力值变更 — 转发给客户端（属性增益仅在会话内有效）
			e.sendEffect(s, cmd)

		case CmdChangeSkill:
			// 技能变更（学习/遗忘）— 转发给客户端
			e.sendEffect(s, cmd)

		case CmdChangeEquipment:
			// 装备变更 — 转发给客户端
			e.sendEffect(s, cmd)

		case CmdChangeName:
			// 角色名变更 — 转发给客户端
			e.sendEffect(s, cmd)

		case CmdChangeClass:
			e.applyChangeClass(ctx, s, cmd.Parameters, opts)

		case CmdChangeActorImage:
			// 角色图像变更 — 转发给客户端
			e.sendEffect(s, cmd)

		case CmdBattleProcessing:
			battleResult[cmd.Indent] = e.processBattle(ctx, s, cmd.Parameters, opts)

		case CmdBattleWin:
			// 战斗胜利分支：结果不为 0 时跳过内部指令
			if battleResult[cmd.Indent] != 0 {
				i = e.skipBranchContent(cmds, i)
			}

		case CmdBattleEscape:
			// 战斗逃跑分支：结果不为 1 时跳过内部指令
			if battleResult[cmd.Indent] != 1 {
				i = e.skipBranchContent(cmds, i)
			}

		case CmdBattleLose:
			// 战斗败北分支：结果不为 2 时跳过内部指令
			if battleResult[cmd.Indent] != 2 {
				i = e.skipBranchContent(cmds, i)
			}

		case CmdShopProcessing:
			// 商店 — 聚合续行商品（605）后转发给客户端处理
			// RMMV command302 会消费后续所有 605 指令作为额外商品
			for i+1 < len(cmds) && cmds[i+1] != nil && cmds[i+1].Code == CmdShopItem {
				i++
			}
			e.sendEffect(s, cmd)

		case CmdShopItem:
			// 已由 CmdShopProcessing 消费，单独出现时跳过

		case CmdGameOver:
			e.sendEffect(s, cmd)

		case CmdReturnToTitle:
			e.sendEffect(s, cmd)

		case CmdComment, CmdCommentCont:
			// 开发者注释，跳过
		}
	}
	return false
}

// callCommonEvent 按 ID 查找公共事件并执行其指令列表。
func (e *Executor) callCommonEvent(ctx context.Context, s *player.PlayerSession, ceID int, opts *ExecuteOpts, depth int) {
	if ceID <= 0 || ceID >= len(e.res.CommonEvents) {
		e.logger.Warn("common event ID out of range", zap.Int("ce_id", ceID))
		return
	}
	ce := e.res.CommonEvents[ceID]
	if ce == nil || len(ce.List) == 0 {
		return
	}
	e.logger.Info("calling common event", zap.Int("ce_id", ceID), zap.String("name", ce.Name), zap.Int("depth", depth+1))
	e.executeList(ctx, s, ce.List, opts, depth+1)
}
