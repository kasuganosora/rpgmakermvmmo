// 角色属性：HP、MP、状态、经验值、等级、职业变更及战斗处理。
package npc

import (
	"context"
	"math"

	"github.com/kasuganosora/rpgmakermvmmo/server/game/player"
	"github.com/kasuganosora/rpgmakermvmmo/server/resource"
	"go.uber.org/zap"
)

// applyChangeHP 处理 RMMV HP 变更指令（代码 311）。
// 参数格式：[0]=固定角色(0=变量), [1]=角色ID/变量ID, [2]=操作(0=增加,1=减少),
// [3]=操作数类型(0=常量,1=变量), [4]=操作数, [5]=允许死亡。
// HP 变更后受上限约束；不允许死亡时最低为 1。
func (e *Executor) applyChangeHP(ctx context.Context, s *player.PlayerSession, params []interface{}, opts *ExecuteOpts) {
	amount := e.resolveOperand(params, 3, 4, opts)
	if paramInt(params, 2) == 1 { // 减少
		amount = -amount
	}
	allowDeath := paramInt(params, 5) != 0

	s.HP += amount
	if s.HP > s.MaxHP {
		s.HP = s.MaxHP
	}
	if s.HP <= 0 {
		if allowDeath {
			s.HP = 0
		} else {
			s.HP = 1
		}
	}

	// 转发给客户端用于视觉更新
	e.sendEffect(s, &resource.EventCommand{Code: CmdChangeHP, Parameters: params})
}

// applyChangeMP 处理 RMMV MP 变更指令（代码 312）。
// 参数格式：[0]=固定角色, [1]=角色ID/变量ID, [2]=操作(0=增加,1=减少),
// [3]=操作数类型(0=常量,1=变量), [4]=操作数。
// MP 变更后限制在 0 至 MaxMP 之间。
func (e *Executor) applyChangeMP(ctx context.Context, s *player.PlayerSession, params []interface{}, opts *ExecuteOpts) {
	amount := e.resolveOperand(params, 3, 4, opts)
	if paramInt(params, 2) == 1 { // 减少
		amount = -amount
	}

	s.MP += amount
	if s.MP > s.MaxMP {
		s.MP = s.MaxMP
	}
	if s.MP < 0 {
		s.MP = 0
	}

	e.sendEffect(s, &resource.EventCommand{Code: CmdChangeMP, Parameters: params})
}

// applyChangeState 处理 RMMV 状态变更指令（代码 313）。
// 参数格式：[0]=固定角色, [1]=角色ID/变量ID, [2]=操作(0=附加,1=解除), [3]=状态ID。
// 状态管理目前主要在客户端完成，服务端仅转发。
func (e *Executor) applyChangeState(ctx context.Context, s *player.PlayerSession, params []interface{}, opts *ExecuteOpts) {
	e.sendEffect(s, &resource.EventCommand{Code: CmdChangeState, Parameters: params})
}

// applyRecoverAll 处理 RMMV 完全恢复指令（代码 314）。
// 参数格式：[0]=固定角色, [1]=角色ID/变量ID。
// 将 HP 和 MP 恢复至上限。
func (e *Executor) applyRecoverAll(ctx context.Context, s *player.PlayerSession, params []interface{}, opts *ExecuteOpts) {
	s.HP = s.MaxHP
	s.MP = s.MaxMP
	e.sendEffect(s, &resource.EventCommand{Code: CmdRecoverAll, Parameters: params})
}

// applyChangeEXP 处理 RMMV 经验值变更指令（代码 315）。
// 参数格式：[0]=固定角色, [1]=角色ID/变量ID, [2]=操作(0=增加,1=减少),
// [3]=操作数类型(0=常量,1=变量), [4]=操作数, [5]=显示升级提示。
// 经验值不会低于 0。
func (e *Executor) applyChangeEXP(ctx context.Context, s *player.PlayerSession, params []interface{}, opts *ExecuteOpts) {
	amount := e.resolveOperand(params, 3, 4, opts)
	if paramInt(params, 2) == 1 {
		amount = -amount
	}
	s.Exp += int64(amount)
	if s.Exp < 0 {
		s.Exp = 0
	}
	e.sendEffect(s, &resource.EventCommand{Code: CmdChangeEXP, Parameters: params})
}

// applyChangeLevel 处理 RMMV 等级变更指令（代码 316）。
// 参数格式：[0]=固定角色, [1]=角色ID/变量ID, [2]=操作(0=增加,1=减少),
// [3]=操作数类型(0=常量,1=变量), [4]=操作数, [5]=显示升级提示。
// 等级不会低于 1。
func (e *Executor) applyChangeLevel(ctx context.Context, s *player.PlayerSession, params []interface{}, opts *ExecuteOpts) {
	amount := e.resolveOperand(params, 3, 4, opts)
	if paramInt(params, 2) == 1 {
		amount = -amount
	}
	s.Level += amount
	if s.Level < 1 {
		s.Level = 1
	}
	e.sendEffect(s, &resource.EventCommand{Code: CmdChangeLevel, Parameters: params})
}

// applyChangeClass 处理 RMMV 职业变更指令（代码 321）。
// 参数格式：[0]=角色ID, [1]=职业ID, [2]=保留经验值。
// 变更职业时按新旧职业的基础参数等比缩放 HP/MP。
// 这对 ProjectB 中的战斗变身（CE 1031 → 变更职业 → 以正确属性开始战斗）至关重要。
func (e *Executor) applyChangeClass(ctx context.Context, s *player.PlayerSession, params []interface{}, opts *ExecuteOpts) {
	classID := paramInt(params, 1)
	if classID <= 0 {
		return
	}

	oldClassID := s.ClassID
	s.ClassID = classID

	// 根据新职业的基础参数等比缩放 HP/MP
	if e.res != nil {
		oldClass := e.res.ClassByID(oldClassID)
		newClass := e.res.ClassByID(classID)
		if oldClass != nil && newClass != nil && s.Level > 0 {
			level := s.Level
			if level > 99 {
				level = 99
			}
			// 参数索引：[0]=最大HP, [1]=最大MP，按等级索引（1起始）
			oldMHP := classParam(oldClass, 0, level)
			newMHP := classParam(newClass, 0, level)
			oldMMP := classParam(oldClass, 1, level)
			newMMP := classParam(newClass, 1, level)

			if oldMHP > 0 && newMHP > 0 {
				ratio := float64(s.HP) / float64(oldMHP)
				s.MaxHP = newMHP
				s.HP = int(math.Ceil(ratio * float64(newMHP)))
				if s.HP < 1 {
					s.HP = 1
				}
			}
			if oldMMP > 0 && newMMP > 0 {
				ratio := float64(s.MP) / float64(oldMMP)
				s.MaxMP = newMMP
				s.MP = int(math.Ceil(ratio * float64(newMMP)))
				if s.MP < 0 {
					s.MP = 0
				}
			}
		}
	}

	e.logger.Info("change class",
		zap.Int64("char_id", s.CharID),
		zap.Int("old_class", oldClassID),
		zap.Int("new_class", classID))

	// 转发给客户端
	e.sendEffect(s, &resource.EventCommand{Code: CmdChangeClass, Parameters: params})
}

// classParam 从职业数据中读取指定等级的基础参数值。
// 参数矩阵格式：Params[paramIdx][level]，等级为 1 起始索引。
func classParam(cls *resource.Class, paramIdx, level int) int {
	if cls == nil || paramIdx >= len(cls.Params) {
		return 0
	}
	row := cls.Params[paramIdx]
	if level < len(row) {
		return row[level]
	}
	if len(row) > 0 {
		return row[len(row)-1]
	}
	return 0
}

// processBattle 处理 RMMV 战斗处理指令（代码 301）。
// 参数格式：[0]=敌群ID类型(0=直接指定,1=变量引用,2=随机), [1]=敌群ID/变量ID,
// [2]=允许逃跑, [3]=允许战败。
// 返回战斗结果：0=胜利, 1=逃跑, 2=败北。未配置战斗处理器时返回 0（默认胜利）。
// 优先调用 BattleFn 回调创建服务端战斗；未配置时退回客户端处理。
func (e *Executor) processBattle(ctx context.Context, s *player.PlayerSession, params []interface{}, opts *ExecuteOpts) int {
	troopType := paramInt(params, 0)
	troopID := paramInt(params, 1)

	if troopType == 1 && opts != nil && opts.GameState != nil {
		// 变量引用：从游戏变量读取敌群 ID
		troopID = opts.GameState.GetVariable(troopID)
	}
	// 类型 2 = 随机遇敌（使用敌群 1 作为回退）
	if troopType == 2 || troopID <= 0 {
		troopID = 1
	}

	canEscape := paramInt(params, 2) != 0
	canLose := paramInt(params, 3) != 0

	if opts != nil && opts.BattleFn != nil {
		return opts.BattleFn(ctx, s, troopID, canEscape, canLose)
	}

	// 未配置战斗处理器，退回客户端处理
	e.logger.Warn("no BattleFn configured, sending battle to client",
		zap.Int("troop_id", troopID))
	e.sendEffect(s, &resource.EventCommand{Code: CmdBattleProcessing, Parameters: params})
	return 0 // 默认胜利
}

// resolveOperand 从参数中读取常量或变量引用的整数值。
// typeIdx 指定操作数类型参数的索引，valIdx 指定操作数值参数的索引。
// 类型为 1 时从 GameState 中读取变量值。
func (e *Executor) resolveOperand(params []interface{}, typeIdx, valIdx int, opts *ExecuteOpts) int {
	opType := paramInt(params, typeIdx)
	val := paramInt(params, valIdx)
	if opType == 1 && opts != nil && opts.GameState != nil {
		return opts.GameState.GetVariable(val)
	}
	return val
}
