// 状态变更：开关、变量、独立开关、金币、物品的游戏状态变更处理。
package npc

import (
	"context"
	"encoding/json"
	"fmt"
	"math/rand"
	"strconv"

	"github.com/kasuganosora/rpgmakermvmmo/server/game/player"
	"go.uber.org/zap"
)

// applySwitches 处理 RMMV 开关变更指令（代码 121）。
// 参数格式：[0]=起始ID, [1]=结束ID, [2]=值(0=ON, 1=OFF)。
// 支持批量设置连续 ID 范围的开关，并逐个同步给客户端。
func (e *Executor) applySwitches(s *player.PlayerSession, params []interface{}, opts *ExecuteOpts) {
	if opts == nil || opts.GameState == nil {
		return
	}
	startID := paramInt(params, 0)
	endID := paramInt(params, 1)
	val := paramInt(params, 2) == 0 // 0=ON
	changed := false
	for id := startID; id <= endID; id++ {
		if opts.GameState.GetSwitch(id) != val {
			opts.GameState.SetSwitch(id, val)
			// 同步给客户端，确保并行公共事件读取到正确值
			e.sendSwitchChange(s, id, val)
			changed = true
		}
	}
	// 开关变更后刷新 NPC 页面，使 mid-event 变更立即反映到 NPC 外观
	if changed && opts.PageRefreshFn != nil {
		opts.PageRefreshFn(s)
	}
}

// applyVariables 处理 RMMV 变量变更指令（代码 122）。
// 参数格式：[0]=起始ID, [1]=结束ID, [2]=操作(0=设置,1=加,2=减,3=乘,4=除,5=取模),
// [3]=操作数类型(0=常量,1=变量引用,2=随机), [4]=操作数或最小值, [5]=最大值(随机时)。
func (e *Executor) applyVariables(s *player.PlayerSession, params []interface{}, opts *ExecuteOpts) {
	if opts == nil || opts.GameState == nil {
		return
	}
	startID := paramInt(params, 0)
	endID := paramInt(params, 1)
	op := paramInt(params, 2)
	operandType := paramInt(params, 3)
	operandVal := paramInt(params, 4)

	// operandType=4: 脚本表达式（如 $gameActors._data[1]._equips[1]._itemId）
	var scriptVal int
	if operandType == 4 {
		scriptStr := paramStr(params, 4)
		scriptVal = e.evalScriptValue(scriptStr, s, opts)
	}

	// operandType=3: GameData（从游戏运行时数据读取）
	// params: [startID, endID, op, 3, dataType, param1, param2]
	// dataType=5: Character → param1=-1(player), param2: 0=x, 1=y, 2=dir
	// dataType=7: Map ID
	var gameDataVal int
	if operandType == 3 {
		dataType := paramInt(params, 4)
		gdParam1 := paramInt(params, 5)
		gdParam2 := paramInt(params, 6)
		switch dataType {
		case 7: // Map ID
			gameDataVal = opts.MapID
		case 5: // Character data
			if gdParam1 == -1 { // Player
				px, py, pdir := s.Position()
				switch gdParam2 {
				case 0:
					gameDataVal = px
				case 1:
					gameDataVal = py
				case 2:
					gameDataVal = pdir
				}
			}
		}
	}

	for id := startID; id <= endID; id++ {
		current := opts.GameState.GetVariable(id)
		val := operandVal
		switch operandType {
		case 1: // 变量引用
			val = opts.GameState.GetVariable(operandVal)
		case 2: // 随机（params[4]=最小值, params[5]=最大值）
			max := paramInt(params, 5)
			if max >= val {
				val = val + rand.Intn(max-val+1)
			}
		case 3: // GameData
			val = gameDataVal
		case 4: // 脚本
			val = scriptVal
		}
		newVal := current
		switch op {
		case 0: // 设置
			newVal = val
		case 1: // 加
			newVal += val
		case 2: // 减
			newVal -= val
		case 3: // 乘
			newVal *= val
		case 4: // 除（防止除零）
			if val != 0 {
				newVal /= val
			}
		case 5: // 取模（防止除零）
			if val != 0 {
				newVal %= val
			}
		}
		if newVal != current {
			opts.GameState.SetVariable(id, newVal)
			// 同步给客户端，确保并行公共事件读取到正确值
			e.sendVarChange(s, id, newVal)
		}
	}
}

// sendVarChange 通知客户端变量值已变更。
func (e *Executor) sendVarChange(s *player.PlayerSession, id, value int) {
	payload, _ := json.Marshal(map[string]interface{}{"id": id, "value": value})
	s.Send(&player.Packet{Type: "var_change", Payload: payload})
}

// sendEquipChange 通知客户端装备槽位已变更。
// 因为 EquipChange 插件命令通过 setupChild(CE 838) 执行实际换装，
// 而 npc_effect 的一次性 Interpreter 不会运行子解释器，所以需要专用消息。
func (e *Executor) sendEquipChange(s *player.PlayerSession, slotIndex, itemID, kind int) {
	payload, _ := json.Marshal(map[string]interface{}{
		"slot_index": slotIndex,
		"item_id":    itemID,
		"kind":       kind,
	})
	s.Send(&player.Packet{Type: "equip_change", Payload: payload})
}

// sendSwitchChange 通知客户端开关状态已变更。
func (e *Executor) sendSwitchChange(s *player.PlayerSession, id int, value bool) {
	payload, _ := json.Marshal(map[string]interface{}{"id": id, "value": value})
	s.Send(&player.Packet{Type: "switch_change", Payload: payload})
}

// sendStateBatch 批量发送变量和开关变更，减少消息数量。
// 用于 CulSkillEffect/ParaCheck 等一次性产生大量变更的场景，
// 避免每个变更单独发送导致客户端 OOM。
func (e *Executor) sendStateBatch(s *player.PlayerSession, varChanges map[int]int, switchChanges map[int]bool) {
	if len(varChanges) == 0 && len(switchChanges) == 0 {
		return
	}
	data := make(map[string]interface{}, 2)
	if len(varChanges) > 0 {
		vars := make(map[string]interface{}, len(varChanges))
		for id, val := range varChanges {
			vars[strconv.Itoa(id)] = val
		}
		data["vars"] = vars
	}
	if len(switchChanges) > 0 {
		switches := make(map[string]interface{}, len(switchChanges))
		for id, val := range switchChanges {
			switches[strconv.Itoa(id)] = val
		}
		data["switches"] = switches
	}
	payload, _ := json.Marshal(data)
	s.Send(&player.Packet{Type: "state_batch", Payload: payload})
}

// applySelfSwitch 处理 RMMV 独立开关变更指令（代码 123）。
// 参数格式：[0]=通道("A"-"D"), [1]=值(0=ON, 1=OFF)。
func (e *Executor) applySelfSwitch(s *player.PlayerSession, params []interface{}, opts *ExecuteOpts) {
	if opts == nil || opts.GameState == nil {
		return
	}
	ch := paramStr(params, 0)
	val := paramInt(params, 1) == 0 // 0=ON
	opts.GameState.SetSelfSwitch(opts.MapID, opts.EventID, ch, val)
	// 独立开关变更后刷新 NPC 页面
	if opts.PageRefreshFn != nil {
		opts.PageRefreshFn(s)
	}
}

// ---- 金币/物品 ----

// maxStackQty 物品最大堆叠数量（与 RMMV 默认一致）。
const maxStackQty = 9999

// applyGold 处理 RMMV 金币变更指令（代码 125）。
// 参数格式：[0]=操作(0=增加,1=减少), [1]=操作数类型(0=常量,1=变量), [2]=操作数。
// 使用 RMMV operateValue 模式：operandType=1 时从游戏变量读取实际值。
// 通过 InventoryStore 接口访问数据库，支持测试 mock。
func (e *Executor) applyGold(ctx context.Context, s *player.PlayerSession, params []interface{}, opts *ExecuteOpts) error {
	op := paramInt(params, 0)
	operandType := paramInt(params, 1)
	operand := paramInt(params, 2)
	// RMMV operateValue：operandType=1 时从变量读取
	if operandType == 1 && opts != nil && opts.GameState != nil {
		operand = opts.GameState.GetVariable(operand)
	}
	amount := int64(operand)
	if op == 1 {
		amount = -amount
	}

	if e.store == nil {
		return fmt.Errorf("no inventory store configured")
	}

	// RMMV 行为：金币扣除不足时钳位到 0，不拒绝操作。
	if amount < 0 {
		gold, err := e.store.GetGold(ctx, s.CharID)
		if err != nil {
			e.logger.Warn("applyGold: failed to get character gold", zap.Int64("char_id", s.CharID), zap.Error(err))
			return err
		}
		if gold < -amount {
			// 钳位：将扣除量调整为实际持有量，使余额归零
			amount = -gold
		}
	}

	if amount == 0 {
		return nil // 无变更
	}
	if err := e.store.UpdateGold(ctx, s.CharID, amount); err != nil {
		e.logger.Warn("applyGold: failed to update gold", zap.Int64("char_id", s.CharID), zap.Error(err))
		return err
	}
	return nil
}

// applyItems 处理 RMMV 物品变更指令（代码 126）。
// 参数格式：[0]=物品ID, [1]=操作(0=增加,1=减少), [2]=操作数类型(0=常量,1=变量), [3]=操作数。
// 使用 RMMV operateValue 模式：operandType=1 时从游戏变量读取实际值。
// 通过 InventoryStore 接口访问数据库，支持测试 mock。
func (e *Executor) applyItems(ctx context.Context, s *player.PlayerSession, params []interface{}, opts *ExecuteOpts) error {
	itemID := paramInt(params, 0)
	op := paramInt(params, 1)
	operandType := paramInt(params, 2)
	qty := paramInt(params, 3)
	// RMMV operateValue：operandType=1 时从变量读取
	if operandType == 1 && opts != nil && opts.GameState != nil {
		qty = opts.GameState.GetVariable(qty)
	}
	if itemID <= 0 {
		return fmt.Errorf("invalid item_id: %d", itemID)
	}
	if qty <= 0 {
		return fmt.Errorf("invalid quantity: %d", qty)
	}

	if e.store == nil {
		return fmt.Errorf("no inventory store configured")
	}

	if op == 1 {
		// RMMV 行为：物品不足时钳位到 0（移除实际持有量），不拒绝操作。
		currentQty, err := e.store.GetItem(ctx, s.CharID, itemID)
		if err != nil || currentQty <= 0 {
			return nil // 物品不存在，无需移除
		}
		if currentQty < qty {
			qty = currentQty // 钳位：仅移除实际持有量
		}
		if err := e.store.RemoveItem(ctx, s.CharID, itemID, qty); err != nil {
			e.logger.Warn("applyItems: failed to remove items", zap.Int64("char_id", s.CharID), zap.Error(err))
			return err
		}
	} else {
		// 增加物品（检查堆叠上限）
		currentQty, _ := e.store.GetItem(ctx, s.CharID, itemID)
		if currentQty+qty > maxStackQty {
			e.logger.Warn("applyItems: exceeds max stack", zap.Int64("char_id", s.CharID), zap.Int("item_id", itemID), zap.Int("new_qty", currentQty+qty))
			return fmt.Errorf("exceeds maximum stack size: %d", maxStackQty)
		}
		if err := e.store.AddItem(ctx, s.CharID, itemID, qty); err != nil {
			e.logger.Warn("applyItems: failed to add items", zap.Int64("char_id", s.CharID), zap.Error(err))
			return err
		}
	}
	return nil
}
