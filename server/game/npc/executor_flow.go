// 控制流：RMMV 事件指令的分支跳转与条件评估逻辑。
package npc

import "github.com/kasuganosora/rpgmakermvmmo/server/resource"

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
// 支持的条件类型：0=开关, 1=变量比较, 2=独立开关。
// 类型 3-12 为客户端特定条件（计时器、角色、敌人等），服务端默认视为满足。
func (e *Executor) evaluateCondition(params []interface{}, opts *ExecuteOpts) bool {
	condType := paramInt(params, 0)
	if opts == nil || opts.GameState == nil {
		// 无游戏状态时，未知条件默认视为满足
		if condType > 2 {
			return true
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

	// 类型 3-12 为客户端特定条件（计时器、角色、敌人等），服务端跳过
	default:
		return true // 未知条件默认视为满足（安全默认值）
	}
	return false
}
