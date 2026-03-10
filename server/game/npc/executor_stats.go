// 角色属性：HP、MP、状态、经验值、等级、职业变更及战斗处理。
package npc

import (
	"context"
	"fmt"
	"regexp"
	"strconv"

	"github.com/kasuganosora/rpgmakermvmmo/server/game/player"
	"github.com/kasuganosora/rpgmakermvmmo/server/resource"
	"go.uber.org/zap"
)

var reTextVarRef = regexp.MustCompile(`\\{1,2}[vV]\[(\d+)\]`)

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
func (e *Executor) applyChangeState(ctx context.Context, s *player.PlayerSession, params []interface{}, opts *ExecuteOpts) {
	op := paramInt(params, 2)   // 0=附加, 1=解除
	stateID := paramInt(params, 3)
	if stateID > 0 {
		if op == 0 {
			s.AddState(stateID)
		} else {
			s.RemoveState(stateID)
		}
	}
	e.sendEffect(s, &resource.EventCommand{Code: CmdChangeState, Parameters: params})
}

// applyRecoverAll 处理 RMMV 完全恢复指令（代码 314）。
// 参数格式：[0]=固定角色, [1]=角色ID/变量ID。
// 将 HP 和 MP 恢复至上限。
func (e *Executor) applyRecoverAll(ctx context.Context, s *player.PlayerSession, params []interface{}, opts *ExecuteOpts) {
	s.HP = s.MaxHP
	s.MP = s.MaxMP
	s.ClearStates()
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
			newMHP := classParam(newClass, 0, level)
			newMMP := classParam(newClass, 1, level)

			if newMHP > 0 {
				s.MaxHP = newMHP
				s.HP = newMHP // 职业变更时完全恢复 HP（匹配 ProjectB 的 ParaCheck 行为）
			}
			if newMMP > 0 {
				s.MaxMP = newMMP
				s.MP = 0 // MP 归零（魔法消耗型资源，变身后重新积累）
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

// ---- 装备变更 ----

// equipSlotTypeMap 将 EquipChange 插件命令的槽位类型名映射到槽位索引。
// 映射来自 OriginalCommands.js 中 EquipChange 的 ETypeID 赋值。
var equipSlotTypeMap = map[string]int{
	"Weapon": 0, "武装": 0, "0": 0,
	"Cloth": 1, "衣装": 1, "1": 1,
	"ClothOption": 2, "衣装オプション": 2, "2": 2,
	"Option": 3, "追加外装": 3, "3": 3,
	"Other": 4, "その他": 4, "4": 4,
	"Special": 6, "特殊": 6, "6": 6,
	"Leg": 7, "脚": 7, "7": 7,
	"Special1": 8, "13": 8,
	"Special2": 9, "14": 9,
	"Special3": 10, "15": 10,
	"Special4": 11, "16": 11,
	"Special5": 12, "17": 12,
	"Special6": 13, "18": 13,
}

// applyEquipChange 处理 EquipChange 插件命令。
// 格式：EquipChange <SlotType> <ArmorID>
// 更新 session.Equips 并持久化到 DB，然后转发给客户端。
func (e *Executor) applyEquipChange(ctx context.Context, s *player.PlayerSession, slotType string, armorIDStr string, opts *ExecuteOpts) {
	slotIndex, ok := equipSlotTypeMap[slotType]
	if !ok {
		e.logger.Warn("applyEquipChange: unknown slot type", zap.String("slot_type", slotType))
		return
	}

	armorID := 0
	// 解析 \v[N] 变量引用（RMMV 文本代码）
	if opts != nil && opts.GameState != nil {
		armorIDStr = e.resolveTextVarRef(armorIDStr, opts)
	}
	if _, err := fmt.Sscanf(armorIDStr, "%d", &armorID); err != nil {
		e.logger.Warn("applyEquipChange: invalid armor ID", zap.String("raw", armorIDStr))
		return
	}

	// 更新 session 内存状态
	s.SetEquip(slotIndex, armorID)

	// 设置变量 v[2701]=slotIndex, v[2703]=armorID（EquipChange 插件行为）
	if opts != nil && opts.GameState != nil {
		opts.GameState.SetVariable(2701, slotIndex)
		e.sendVarChange(s, 2701, slotIndex)
		opts.GameState.SetVariable(2703, armorID)
		e.sendVarChange(s, 2703, armorID)
	}

	// 持久化到 DB
	kind := 3 // 防具
	if slotIndex == 0 {
		kind = 2 // 武器
	}
	if e.store != nil {
		if err := e.store.SetEquipSlot(ctx, s.CharID, slotIndex, armorID, kind); err != nil {
			e.logger.Warn("applyEquipChange: DB persist failed",
				zap.Int64("char_id", s.CharID), zap.Int("slot", slotIndex), zap.Int("armor_id", armorID), zap.Error(err))
		}
	}

	// 同步给客户端——因为 EquipChange 插件命令的 setupChild(CE 838)
	// 在 npc_effect 一次性 Interpreter 中不会执行，所以发送专用消息。
	e.sendEquipChange(s, slotIndex, armorID, kind)

	e.logger.Info("equip change",
		zap.Int64("char_id", s.CharID), zap.String("slot_type", slotType),
		zap.Int("slot_index", slotIndex), zap.Int("armor_id", armorID))
}

// applyChangeArmors 处理 RMMV 防具变更指令（代码 128）。
// 参数格式：[0]=armorId, [1]=op(0=add,1=remove), [2]=operandType(0=const,1=var), [3]=quantity, [4]=includeEquip。
// 这是背包增减操作（非装备操作），持久化到 DB。
func (e *Executor) applyChangeArmors(ctx context.Context, s *player.PlayerSession, params []interface{}, opts *ExecuteOpts) {
	armorID := paramInt(params, 0)
	op := paramInt(params, 1)
	qty := e.resolveOperand(params, 2, 3, opts)
	if armorID <= 0 || qty <= 0 {
		return
	}

	if e.store != nil {
		if op == 1 {
			// 移除
			if err := e.store.RemoveArmorOrWeapon(ctx, s.CharID, armorID, 3, qty); err != nil {
				e.logger.Warn("applyChangeArmors: remove failed", zap.Int64("char_id", s.CharID), zap.Error(err))
			}
		} else {
			// 添加
			if err := e.store.AddArmorOrWeapon(ctx, s.CharID, armorID, 3, qty); err != nil {
				e.logger.Warn("applyChangeArmors: add failed", zap.Int64("char_id", s.CharID), zap.Error(err))
			}
		}
	}

	e.sendEffect(s, &resource.EventCommand{Code: CmdChangeArmors, Parameters: params})
}

// applyChangeWeapons 处理 RMMV 武器变更指令（代码 127）。
// 参数格式同防具变更。
func (e *Executor) applyChangeWeapons(ctx context.Context, s *player.PlayerSession, params []interface{}, opts *ExecuteOpts) {
	weaponID := paramInt(params, 0)
	op := paramInt(params, 1)
	qty := e.resolveOperand(params, 2, 3, opts)
	if weaponID <= 0 || qty <= 0 {
		return
	}

	if e.store != nil {
		if op == 1 {
			if err := e.store.RemoveArmorOrWeapon(ctx, s.CharID, weaponID, 2, qty); err != nil {
				e.logger.Warn("applyChangeWeapons: remove failed", zap.Int64("char_id", s.CharID), zap.Error(err))
			}
		} else {
			if err := e.store.AddArmorOrWeapon(ctx, s.CharID, weaponID, 2, qty); err != nil {
				e.logger.Warn("applyChangeWeapons: add failed", zap.Int64("char_id", s.CharID), zap.Error(err))
			}
		}
	}

	e.sendEffect(s, &resource.EventCommand{Code: CmdChangeWeapons, Parameters: params})
}

// applyChangeEquipment 处理 RMMV 装备变更指令（代码 319）。
// 参数格式：[0]=actorId, [1]=etypeId, [2]=itemId。
// etypeId 在 RMMV 中是装备类型（1=武器,2=盾,3=头,4=身体,5=饰品）。
// 对于 ProjectB 使用 TMEquipSlotEx，etypeId 直接映射到槽位索引（非标准）。
func (e *Executor) applyChangeEquipment(ctx context.Context, s *player.PlayerSession, params []interface{}, opts *ExecuteOpts) {
	etypeID := paramInt(params, 1)
	itemID := paramInt(params, 2)

	// etypeId 在 TMEquipSlotEx 下就是槽位索引
	slotIndex := etypeID
	s.SetEquip(slotIndex, itemID)

	kind := 3 // 防具
	if slotIndex == 0 {
		kind = 2 // 武器
	}
	if e.store != nil && itemID > 0 {
		if err := e.store.SetEquipSlot(ctx, s.CharID, slotIndex, itemID, kind); err != nil {
			e.logger.Warn("applyChangeEquipment: DB persist failed",
				zap.Int64("char_id", s.CharID), zap.Int("slot", slotIndex), zap.Int("item_id", itemID), zap.Error(err))
		}
	}

	e.sendEffect(s, &resource.EventCommand{Code: CmdChangeEquipment, Parameters: params})
}

// resolveTextVarRef 替换字符串中的 \v[N] / \V[N] 变量引用为实际值。
// RMMV 插件命令参数中常用 \v[N] 引用游戏变量。
func (e *Executor) resolveTextVarRef(s string, opts *ExecuteOpts) string {
	if opts == nil || opts.GameState == nil {
		return s
	}
	return reTextVarRef.ReplaceAllStringFunc(s, func(match string) string {
		sub := reTextVarRef.FindStringSubmatch(match)
		// sub[1] is always a valid integer: reTextVarRef requires \d+
		varID, _ := strconv.Atoi(sub[1])
		return strconv.Itoa(opts.GameState.GetVariable(varID))
	})
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
