// 控制流：RMMV 事件指令的分支跳转与条件评估逻辑。
package npc

import (
	"context"

	"github.com/kasuganosora/rpgmakermvmmo/server/game/player"
	"github.com/kasuganosora/rpgmakermvmmo/server/model"
	"github.com/kasuganosora/rpgmakermvmmo/server/resource"
	"go.uber.org/zap"
)

// skipToChoiceBranch 在 ShowChoices 指令之后，跳转到与玩家选择对应的 When 分支。
// 参数 choiceIdx 为玩家选择的选项索引（从 0 开始），cancelType 为取消类型。
// 返回目标指令的索引位置。
func (e *Executor) skipToChoiceBranch(cmds []*resource.EventCommand, startIdx, choiceIdx, cancelType int) int {
	indent := cmds[startIdx].Indent
	branchCount := 0
	for j := startIdx + 1; j < len(cmds); j++ {
		c := cmds[j]
		if c == nil {
			continue
		}
		if c.Indent != indent {
			continue
		}
		if c.Code == CmdWhenBranch {
			if branchCount == choiceIdx {
				return j // 从该分支的下一条指令开始执行
			}
			branchCount++
		}
		if c.Code == CmdWhenCancel {
			if choiceIdx < 0 || choiceIdx == cancelType {
				return j
			}
		}
		if c.Code == CmdBranchEnd {
			return j // 未找到匹配分支，跳到块结尾继续执行
		}
	}
	return len(cmds) - 1
}

// skipToBranchEnd 向前扫描到指定缩进层级的 BranchEnd（代码 404）。
// 用于在已执行选定分支后跳过其余未选中的分支。
func (e *Executor) skipToBranchEnd(cmds []*resource.EventCommand, startIdx, indent int) int {
	for j := startIdx + 1; j < len(cmds); j++ {
		c := cmds[j]
		if c == nil {
			continue
		}
		if c.Code == CmdBranchEnd && c.Indent == indent {
			return j
		}
	}
	return len(cmds) - 1
}

// skipToElseOrEnd 向前扫描到指定缩进层级的 ElseBranch（411）或 ConditionalEnd（412）。
// 用于条件分支判断为假时跳过 if 块。
func (e *Executor) skipToElseOrEnd(cmds []*resource.EventCommand, startIdx, indent int) int {
	for j := startIdx + 1; j < len(cmds); j++ {
		c := cmds[j]
		if c == nil {
			continue
		}
		if c.Indent != indent {
			continue
		}
		if c.Code == CmdElseBranch || c.Code == CmdConditionalEnd {
			return j
		}
	}
	return len(cmds) - 1
}

// skipToConditionalEnd 向前扫描到指定缩进层级的 ConditionalEnd（412）。
// 用于在 if 分支执行完成后跳过 else 块。
func (e *Executor) skipToConditionalEnd(cmds []*resource.EventCommand, startIdx, indent int) int {
	for j := startIdx + 1; j < len(cmds); j++ {
		c := cmds[j]
		if c == nil {
			continue
		}
		if c.Code == CmdConditionalEnd && c.Indent == indent {
			return j
		}
	}
	return len(cmds) - 1
}

// jumpToLoopStart 从 RepeatAbove（413）位置向后扫描，查找同缩进层级的 Loop（112）。
// 返回 Loop 指令的索引，使 for 循环的 i++ 将执行定位到 Loop 之后的第一条指令。
func (e *Executor) jumpToLoopStart(cmds []*resource.EventCommand, startIdx, indent int) int {
	for j := startIdx - 1; j >= 0; j-- {
		c := cmds[j]
		if c == nil {
			continue
		}
		if c.Code == CmdLoop && c.Indent == indent {
			return j // 由 for 循环的 i++ 自动跳到 Loop+1
		}
	}
	// 未找到 Loop 起点（不应发生），保持当前位置
	return startIdx
}

// skipPastLoopEnd 从 BreakLoop（113）位置向前扫描，查找配对的 RepeatAbove（413）。
// 使用深度计数器处理嵌套循环，匹配 RMMV Game_Interpreter.command113 的行为：
// 遇到 Loop（112）时深度+1，遇到 RepeatAbove（413）时深度-1，深度为 0 时即为目标。
// 返回 RepeatAbove 的索引，使执行跳出循环继续后续指令。
func (e *Executor) skipPastLoopEnd(cmds []*resource.EventCommand, startIdx, _ int) int {
	depth := 0
	for j := startIdx + 1; j < len(cmds); j++ {
		c := cmds[j]
		if c == nil {
			continue
		}
		if c.Code == CmdLoop {
			depth++
		}
		if c.Code == CmdRepeatAbove {
			if depth > 0 {
				depth--
			} else {
				return j
			}
		}
	}
	return len(cmds) - 1
}

// skipBranchContent 跳过当前分支的内部指令（缩进大于当前指令的所有后续指令）。
// 匹配 RMMV Game_Interpreter.skipBranch 的行为：
// while (_list[_index+1].indent > _indent) { _index++; }
// 返回最后一条被跳过指令的索引，for 循环的 i++ 将定位到下一条同级指令。
func (e *Executor) skipBranchContent(cmds []*resource.EventCommand, startIdx int) int {
	indent := cmds[startIdx].Indent
	j := startIdx
	for j+1 < len(cmds) && cmds[j+1] != nil && cmds[j+1].Indent > indent {
		j++
	}
	return j
}

// jumpToLabel 在整个指令列表中查找名称匹配的 Label（118）指令。
// 返回 Label 的索引位置，未找到返回 -1（RMMV 行为：标签不存在时继续执行）。
func (e *Executor) jumpToLabel(cmds []*resource.EventCommand, labelName string) int {
	for j := 0; j < len(cmds); j++ {
		c := cmds[j]
		if c == nil {
			continue
		}
		if c.Code == CmdLabel && len(c.Parameters) > 0 {
			if paramStr(c.Parameters, 0) == labelName {
				return j
			}
		}
	}
	return -1
}

// evaluateCondition 评估 RMMV 条件分支（代码 111）。
// 参数格式：[0]=条件类型, 后续参数取决于类型。
// 支持的条件类型：0=开关, 1=变量比较, 2=独立开关, 4=角色, 7=金币, 8=物品, 9=武器, 10=防具,
// 12=脚本（通过 Goja VM 评估，支持 $gameSwitches/$gameVariables/$dataMap.meta；不可评估时默认 true）。
// 类型 3(计时器), 5(敌人), 6(角色方向), 11(按键) 服务端默认不满足。
func (e *Executor) evaluateCondition(ctx context.Context, s *player.PlayerSession, params []interface{}, opts *ExecuteOpts) bool {
	condType := paramInt(params, 0)
	if opts == nil || opts.GameState == nil {
		if condType > 2 {
			return false
		}
		return false
	}
	gs := opts.GameState
	switch condType {
	case 0: // 开关条件
		switchID := paramInt(params, 1)
		expected := paramInt(params, 2) // 0=ON, 1=OFF
		val := gs.GetSwitch(switchID)
		if expected == 0 {
			return val
		}
		return !val

	case 1: // 变量比较条件
		varID := paramInt(params, 1)
		refType := paramInt(params, 2) // 0=常量, 1=变量引用
		refVal := paramInt(params, 3)
		op := paramInt(params, 4) // 0=等于, 1=大于等于, 2=小于等于, 3=大于, 4=小于, 5=不等于
		varVal := gs.GetVariable(varID)
		compareVal := refVal
		if refType == 1 {
			compareVal = gs.GetVariable(refVal)
		}
		switch op {
		case 0:
			return varVal == compareVal
		case 1:
			return varVal >= compareVal
		case 2:
			return varVal <= compareVal
		case 3:
			return varVal > compareVal
		case 4:
			return varVal < compareVal
		case 5:
			return varVal != compareVal
		}

	case 2: // 独立开关条件
		ch := paramStr(params, 1)       // "A","B","C","D"
		expected := paramInt(params, 2) // 0=ON, 1=OFF
		val := gs.GetSelfSwitch(opts.MapID, opts.EventID, ch)
		if expected == 0 {
			return val
		}
		return !val

	case 4: // 角色条件
		return e.evalActorCondition(ctx, s, params)

	case 7: // 金币条件
		return e.evalGoldCondition(ctx, s, params)

	case 8: // 物品条件
		return e.evalItemCondition(ctx, s, params, model.ItemKindItem)

	case 9: // 武器条件
		return e.evalItemCondition(ctx, s, params, model.ItemKindWeapon)

	case 10: // 防具条件
		return e.evalItemCondition(ctx, s, params, model.ItemKindArmor)

	case 12: // 脚本条件
		script := paramStr(params, 1)
		result, _ := e.evalScriptCondition(script, s, opts)
		return result

	default:
		// 类型 3(计时器), 5(敌人), 6(角色方向), 11(按键)
		// 服务端无法评估，默认不满足。
		e.logger.Debug("unsupported condition type, defaulting to false",
			zap.Int("cond_type", condType))
		return false
	}
	return false
}

// evalActorCondition 评估角色条件（类型 4）。
// params: [0]=4, [1]=actorId, [2]=子类型, [3]=比较值
// 子类型: 0=在队伍中, 1=名字, 2=职业, 3=技能, 4=武器, 5=防具, 6=状态
func (e *Executor) evalActorCondition(ctx context.Context, s *player.PlayerSession, params []interface{}) bool {
	// actorID := paramInt(params, 1) // 单人游戏始终为 actor 1
	subType := paramInt(params, 2)
	compareVal := paramInt(params, 3)

	switch subType {
	case 0: // 在队伍中 — 单人游戏中主角始终在队伍
		return true

	case 2: // 职业
		return s.ClassID == compareVal

	case 4: // 装备了武器
		if e.store == nil {
			return false
		}
		has, err := e.store.IsEquipped(ctx, s.CharID, compareVal, model.ItemKindWeapon)
		if err != nil {
			return false
		}
		return has

	case 5: // 装备了防具
		if e.store == nil {
			return false
		}
		has, err := e.store.IsEquipped(ctx, s.CharID, compareVal, model.ItemKindArmor)
		if err != nil {
			return false
		}
		return has

	case 6: // 状态
		return s.HasState(compareVal)

	case 3: // 技能
		if e.store == nil {
			return false
		}
		has, err := e.store.HasSkill(ctx, s.CharID, compareVal)
		if err != nil {
			return false
		}
		return has

	case 1:
		// 1=名字 — 服务端暂不跟踪，默认不满足
		e.logger.Debug("actor condition sub-type not implemented",
			zap.Int("sub_type", subType), zap.Int("compare", compareVal))
		return false

	default:
		return false
	}
}

// evalGoldCondition 评估金币条件（类型 7）。
// params: [0]=7, [1]=金额, [2]=操作符(0=大于等于, 1=小于等于, 2=小于)
func (e *Executor) evalGoldCondition(ctx context.Context, s *player.PlayerSession, params []interface{}) bool {
	if e.store == nil {
		return false
	}
	amount := int64(paramInt(params, 1))
	op := paramInt(params, 2)
	gold, err := e.store.GetGold(ctx, s.CharID)
	if err != nil {
		return false
	}
	switch op {
	case 0:
		return gold >= amount
	case 1:
		return gold <= amount
	case 2:
		return gold < amount
	}
	return false
}

// evalItemCondition 评估物品/武器/防具持有条件（类型 8/9/10）。
// 类型 8: params: [0]=8, [1]=物品ID
// 类型 9: params: [0]=9, [1]=武器ID, [2]=含装备
// 类型 10: params: [0]=10, [1]=防具ID, [2]=含装备
func (e *Executor) evalItemCondition(ctx context.Context, s *player.PlayerSession, params []interface{}, kind int) bool {
	if e.store == nil {
		return false
	}
	itemID := paramInt(params, 1)
	includeEquip := kind != model.ItemKindItem && asBool(params[2]) // 消耗品无装备概念
	has, err := e.store.HasItemOfKind(ctx, s.CharID, itemID, kind, includeEquip)
	if err != nil {
		return false
	}
	return has
}
