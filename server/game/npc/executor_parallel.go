// 平行事件同步执行器：在单个 goroutine 中同步推进所有平行事件，
// 确保帧完美同步（如玩家与 NPC 并排行走）。
package npc

import (
	"context"
	"encoding/json"
	"strings"
	"time"

	"github.com/kasuganosora/rpgmakermvmmo/server/game/player"
	"github.com/kasuganosora/rpgmakermvmmo/server/resource"
)

// ParallelEventState 追踪单个平行事件的执行状态。
type ParallelEventState struct {
	EventID    int
	Cmds       []*resource.EventCommand
	MoveSpeed  int  // 事件页移动速度（用于同步玩家速度）
	idx        int  // 当前指令索引
	waitFrames int  // 剩余等待帧数（>0 时跳过执行）
	done       bool // 本轮循环是否结束
}

// NewParallelEventState 创建平行事件状态。
func NewParallelEventState(eventID int, cmds []*resource.EventCommand, moveSpeed int) *ParallelEventState {
	return &ParallelEventState{
		EventID:   eventID,
		Cmds:      cmds,
		MoveSpeed: moveSpeed,
	}
}

// RunParallelEventsSynced 在单个 goroutine 中同步运行所有平行事件。
// 每个 tick 推进所有事件直到它们都遇到 Wait，然后统一等待。
// 这模拟了 RMMV 中所有平行事件在同一帧更新的行为。
func (e *Executor) RunParallelEventsSynced(
	ctx context.Context,
	s *player.PlayerSession,
	events []*ParallelEventState,
	opts *ExecuteOpts,
) {
	if len(events) == 0 {
		return
	}

	// 计算最慢的移动速度来确定 tick 间隔。
	// RMMV: distancePerFrame = 2^speed/256，一格移动帧数 = 256/2^speed
	// 默认 moveSpeed 3 → 32帧 ≈ 533ms；moveSpeed 4 → 16帧 ≈ 267ms
	// 每个 tick 代表 framesPerTick 帧，用于 Wait 帧数倒计时。
	slowestSpeed := 6
	for _, ev := range events {
		if ev.MoveSpeed > 0 && ev.MoveSpeed < slowestSpeed {
			slowestSpeed = ev.MoveSpeed
		}
	}
	framesPerTile := 256 >> slowestSpeed // 256 / 2^speed
	framesPerTick := framesPerTile       // 每个 tick = 一格移动时间
	tickMs := framesPerTick * 1000 / 60
	if tickMs < 100 {
		tickMs = 100
	}

	ticker := time.NewTicker(time.Duration(tickMs) * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		default:
		}

		// 检查玩家是否仍在同一地图（防止发送过期效果）
		if s.MapID != opts.MapID {
			return
		}

		// 推进所有事件：先扣减等待帧数，然后执行指令直到 Wait
		allDone := true
		for _, ev := range events {
			if ev.done {
				continue
			}
			// 扣减等待帧数
			if ev.waitFrames > 0 {
				ev.waitFrames -= framesPerTick
				if ev.waitFrames > 0 {
					allDone = false
					continue // 仍在等待
				}
				ev.waitFrames = 0
			}
			allDone = false
			ev.done = e.stepUntilWait(ctx, s, ev, opts)
		}

		// 所有事件本轮结束 → 重置（平行事件自动循环）
		if allDone {
			for _, ev := range events {
				ev.idx = 0
				ev.done = false
				ev.waitFrames = 0
			}
		}

		// 统一等待下一个 tick
		select {
		case <-ticker.C:
		case <-ctx.Done():
			return
		}
	}
}

// stepUntilWait 推进单个平行事件直到遇到 Wait 或执行结束。
// 返回 true 表示事件本轮执行完毕（遇到结尾的 CmdEnd 或 ExitEvent）。
func (e *Executor) stepUntilWait(
	ctx context.Context,
	s *player.PlayerSession,
	ev *ParallelEventState,
	opts *ExecuteOpts,
) bool {
	cmds := ev.Cmds
	// 临时设置当前事件 ID
	savedEventID := opts.EventID
	opts.EventID = ev.EventID
	defer func() { opts.EventID = savedEventID }()

	for ev.idx < len(cmds) {
		select {
		case <-ctx.Done():
			return true
		default:
		}

		// 每条指令前检查玩家是否仍在同一地图
		if s.MapID != opts.MapID {
			return true
		}

		cmd := cmds[ev.idx]
		if cmd == nil {
			ev.idx++
			continue
		}

		switch cmd.Code {
		case CmdEnd:
			if cmd.Indent == 0 {
				return true // 列表结束
			}
			ev.idx++

		case CmdLoop:
			ev.idx++

		case CmdRepeatAbove:
			ev.idx = e.jumpToLoopStart(cmds, ev.idx, cmd.Indent)
			ev.idx++ // jumpToLoopStart 返回 Loop 位置，+1 跳到循环体

		case CmdBreakLoop:
			ev.idx = e.skipPastLoopEnd(cmds, ev.idx, cmd.Indent)
			ev.idx++

		case CmdExitEvent:
			return true

		case CmdConditionalStart:
			if !e.evaluateCondition(ctx, s, cmd.Parameters, opts) {
				ev.idx = e.skipToElseOrEnd(cmds, ev.idx, cmd.Indent)
			} else {
				ev.idx++
			}

		case CmdElseBranch:
			ev.idx = e.skipToConditionalEnd(cmds, ev.idx, cmd.Indent)

		case CmdConditionalEnd, CmdBranchEnd:
			ev.idx++

		case CmdSetMoveRoute:
			resolved := e.resolveCharIDCommand(cmd, opts)
			// 若目标为玩家（charId=-1）且事件有指定移动速度，
			// 注入 ROUTE_CHANGE_SPEED 确保玩家与 NPC 同速行走。
			if charID := paramInt(resolved.Parameters, 0); charID == -1 && ev.MoveSpeed > 0 {
				resolved = injectPlayerSpeed(resolved, ev.MoveSpeed)
			}
			sendParallelEffect(s, resolved, opts.MapID)
			// 跳过续行（505）
			ev.idx++
			for ev.idx < len(cmds) && cmds[ev.idx] != nil && cmds[ev.idx].Code == CmdMoveRouteCont {
				ev.idx++
			}

		case CmdMoveRouteCont:
			ev.idx++ // 已由 SetMoveRoute 消费

		case CmdWait:
			// 设置等待帧数，交出控制权。
			// 外层循环每 tick 扣减 framesPerTick 帧，归零后继续执行。
			frames := paramInt(cmd.Parameters, 0)
			if frames < 1 {
				frames = 1
			}
			ev.waitFrames = frames
			ev.idx++
			return false // 未结束，只是暂停

		case CmdChangeSwitches:
			e.applySwitches(s, cmd.Parameters, opts)
			ev.idx++

		case CmdChangeVars:
			e.applyVariables(s, cmd.Parameters, opts)
			ev.idx++

		case CmdChangeSelfSwitch:
			e.applySelfSwitch(s, cmd.Parameters, opts)
			ev.idx++

		case CmdCallCommonEvent:
			ceID := paramInt(cmd.Parameters, 0)
			e.callCommonEvent(ctx, s, ceID, opts, 0)
			ev.idx++

		case CmdComment, CmdCommentCont:
			ev.idx++

		case CmdLabel:
			ev.idx++

		case CmdJumpToLabel:
			labelName := paramStr(cmd.Parameters, 0)
			jumped := e.jumpToLabel(cmds, labelName)
			if jumped >= 0 {
				ev.idx = jumped
			} else {
				ev.idx++
			}

		case CmdPluginCommand:
			// 检查是否为 TemplateEvent.js 指令（需服务端处理）
			if e.handleTECallOriginEvent(ctx, s, cmd, opts, 0) {
				ev.idx++
				continue
			}
			pluginStr := paramStr(cmd.Parameters, 0)
			// 副本地图命令
			if pluginStr == "EnterInstance" {
				if opts != nil && opts.EnterInstanceFn != nil {
					opts.EnterInstanceFn(s)
				}
				ev.idx++
				continue
			}
			if pluginStr == "LeaveInstance" {
				if opts != nil && opts.LeaveInstanceFn != nil {
					opts.LeaveInstanceFn(s)
				}
				ev.idx++
				continue
			}
			// 转发插件指令给客户端（包括立绘 CallStand/CallCutin/CallAM）
			sendParallelEffect(s, cmd, opts.MapID)
			ev.idx++

		case CmdScript:
			// 过滤脚本——只转发 $gameScreen. 和 AudioManager. 开头的行
			script := paramStr(cmd.Parameters, 0)
			// 合并续行（code 655）
			for ev.idx+1 < len(cmds) && cmds[ev.idx+1] != nil && cmds[ev.idx+1].Code == CmdScriptCont {
				ev.idx++
				script += "\n" + paramStr(cmds[ev.idx].Parameters, 0)
			}
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
				sendParallelEffect(s, &resource.EventCommand{
					Code:       CmdScript,
					Parameters: []interface{}{strings.Join(safeLines, "\n")},
				}, opts.MapID)
			}
			ev.idx++

		case CmdScriptCont:
			ev.idx++ // 已由 CmdScript 处理

		// ---- 服务端状态变更（需 DB 持久化）----

		case CmdChangeGold:
			if err := e.applyGold(ctx, s, cmd.Parameters, opts); err == nil {
				sendParallelEffect(s, cmd, opts.MapID)
			}
			ev.idx++

		case CmdChangeItems:
			if err := e.applyItems(ctx, s, cmd.Parameters, opts); err == nil {
				sendParallelEffect(s, cmd, opts.MapID)
			}
			ev.idx++

		case CmdChangeHP:
			e.applyChangeHP(ctx, s, cmd.Parameters, opts)
			ev.idx++

		case CmdChangeMP:
			e.applyChangeMP(ctx, s, cmd.Parameters, opts)
			ev.idx++

		case CmdChangeState:
			e.applyChangeState(ctx, s, cmd.Parameters, opts)
			ev.idx++

		case CmdRecoverAll:
			e.applyRecoverAll(ctx, s, cmd.Parameters, opts)
			ev.idx++

		case CmdChangeEXP:
			e.applyChangeEXP(ctx, s, cmd.Parameters, opts)
			ev.idx++

		case CmdChangeLevel:
			e.applyChangeLevel(ctx, s, cmd.Parameters, opts)
			ev.idx++

		case CmdChangeClass:
			e.applyChangeClass(ctx, s, cmd.Parameters, opts)
			ev.idx++

		// ---- charID 解析（charId=0 → 当前事件 ID）----

		case CmdSetEventLocation:
			resolved := e.resolveCharIDCommand(cmd, opts)
			sendParallelEffect(s, resolved, opts.MapID)
			ev.idx++

		case CmdShowAnimation:
			resolved := e.resolveCharIDCommand(cmd, opts)
			sendParallelEffect(s, resolved, opts.MapID)
			ev.idx++

		case CmdShowBalloon:
			resolved := e.resolveCharIDCommand(cmd, opts)
			sendParallelEffect(s, resolved, opts.MapID)
			ev.idx++

		// ---- 图片指令（需变量坐标解析）----

		case CmdShowPicture:
			e.sendShowPicture(s, cmd.Parameters, opts)
			ev.idx++

		case CmdMovePicture:
			// 平行事件中解析变量坐标后转发（不等待动画完成）
			e.sendMovePictureNoWait(s, cmd.Parameters, opts)
			ev.idx++

		default:
			// 其他指令（音效、图片等）直接转发
			sendParallelEffect(s, cmd, opts.MapID)
			ev.idx++
		}
	}
	return true // 执行到列表末尾
}

// sendParallelEffect 发送带 map_id 标记的 npc_effect，
// 客户端会校验 map_id 与当前地图是否一致，丢弃过期效果。
// 解决地图切换时 WS 消息顺序导致旧地图效果污染新地图 NPC 的问题。
func sendParallelEffect(s *player.PlayerSession, cmd *resource.EventCommand, mapID int) {
	payload, _ := json.Marshal(map[string]interface{}{
		"code":   cmd.Code,
		"indent": cmd.Indent,
		"params": cmd.Parameters,
		"map_id": mapID,
	})
	s.Send(&player.Packet{Type: "npc_effect", Payload: payload})
}

// injectPlayerSpeed 在移动路线开头注入 ROUTE_CHANGE_SPEED 指令，
// 使玩家的移动速度与 NPC 保持一致。
// RMMV 移动路线指令代码 29 = ROUTE_CHANGE_SPEED，参数 [0]=速度值。
func injectPlayerSpeed(cmd *resource.EventCommand, speed int) *resource.EventCommand {
	if len(cmd.Parameters) < 2 {
		return cmd
	}
	mr, ok := cmd.Parameters[1].(map[string]interface{})
	if !ok {
		return cmd
	}
	list, ok := mr["list"]
	if !ok {
		return cmd
	}
	listSlice, ok := list.([]interface{})
	if !ok {
		return cmd
	}

	// 构造 ROUTE_CHANGE_SPEED 指令
	speedCmd := map[string]interface{}{
		"code":   float64(29), // ROUTE_CHANGE_SPEED
		"indent": nil,
		"parameters": []interface{}{float64(speed)},
	}

	// 在列表开头插入速度变更指令
	newList := make([]interface{}, 0, len(listSlice)+1)
	newList = append(newList, speedCmd)
	newList = append(newList, listSlice...)

	// 构造新的参数（不修改原始数据）
	newMR := make(map[string]interface{}, len(mr))
	for k, v := range mr {
		newMR[k] = v
	}
	newMR["list"] = newList

	newParams := make([]interface{}, len(cmd.Parameters))
	copy(newParams, cmd.Parameters)
	newParams[1] = newMR

	return &resource.EventCommand{Code: cmd.Code, Parameters: newParams}
}
