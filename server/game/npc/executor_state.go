// 状态变更：开关、变量、独立开关、金币、物品的游戏状态变更处理。
package npc

import (
	"context"
	"encoding/json"
	"fmt"

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
	for id := startID; id <= endID; id++ {
		opts.GameState.SetSwitch(id, val)
		// 同步给客户端，确保并行公共事件读取到正确值
		e.sendSwitchChange(s, id, val)
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

	for id := startID; id <= endID; id++ {
		current := opts.GameState.GetVariable(id)
		val := operandVal
		if operandType == 1 {
			val = opts.GameState.GetVariable(operandVal)
		}
		switch op {
		case 0: // 设置
			current = val
		case 1: // 加
			current += val
		case 2: // 减
			current -= val
		case 3: // 乘
			current *= val
		case 4: // 除（防止除零）
			if val != 0 {
				current /= val
			}
		case 5: // 取模（防止除零）
			if val != 0 {
				current %= val
			}
		}
		opts.GameState.SetVariable(id, current)
		// 同步给客户端，确保并行公共事件读取到正确值
		e.sendVarChange(s, id, current)
	}
}

// sendVarChange 通知客户端变量值已变更。
func (e *Executor) sendVarChange(s *player.PlayerSession, id, value int) {
	payload, _ := json.Marshal(map[string]interface{}{"id": id, "value": value})
	s.Send(&player.Packet{Type: "var_change", Payload: payload})
}

// sendSwitchChange 通知客户端开关状态已变更。
func (e *Executor) sendSwitchChange(s *player.PlayerSession, id int, value bool) {
	payload, _ := json.Marshal(map[string]interface{}{"id": id, "value": value})
	s.Send(&player.Packet{Type: "switch_change", Payload: payload})
}

// applySelfSwitch 处理 RMMV 独立开关变更指令（代码 123）。
// 参数格式：[0]=通道("A"-"D"), [1]=值(0=ON, 1=OFF)。
func (e *Executor) applySelfSwitch(params []interface{}, opts *ExecuteOpts) {
	if opts == nil || opts.GameState == nil {
		return
	}
	ch := paramStr(params, 0)
	val := paramInt(params, 1) == 0 // 0=ON
	opts.GameState.SetSelfSwitch(opts.MapID, opts.EventID, ch, val)
}

// ---- 金币/物品 ----

// maxStackQty 物品最大堆叠数量（与 RMMV 默认一致）。
const maxStackQty = 9999

// applyGold 处理 RMMV 金币变更指令（代码 125）。
// 参数格式：[0]=操作(0=增加,1=减少), [1]=操作数类型(0=常量,1=变量), [2]=操作数。
// 通过 InventoryStore 接口访问数据库，支持测试 mock。
func (e *Executor) applyGold(ctx context.Context, s *player.PlayerSession, params []interface{}) error {
	op := paramInt(params, 0)
	amount := int64(paramInt(params, 2))
	if op == 1 {
		amount = -amount
	}

	if e.store == nil {
		return fmt.Errorf("no inventory store configured")
	}

	// 扣除金币时检查余额是否充足
	if amount < 0 {
		gold, err := e.store.GetGold(ctx, s.CharID)
		if err != nil {
			e.logger.Warn("applyGold: failed to get character gold", zap.Int64("char_id", s.CharID), zap.Error(err))
			return err
		}
		if gold < -amount {
			e.logger.Warn("applyGold: insufficient gold", zap.Int64("char_id", s.CharID), zap.Int64("have", gold), zap.Int64("need", -amount))
			return fmt.Errorf("insufficient gold: have %d, need %d", gold, -amount)
		}
	}

	if err := e.store.UpdateGold(ctx, s.CharID, amount); err != nil {
		e.logger.Warn("applyGold: failed to update gold", zap.Int64("char_id", s.CharID), zap.Error(err))
		return err
	}
	return nil
}

// applyItems 处理 RMMV 物品变更指令（代码 126）。
// 参数格式：[0]=物品ID, [1]=操作(0=增加,1=减少), [2]=操作数类型, [3]=数量。
// 通过 InventoryStore 接口访问数据库，支持测试 mock。
func (e *Executor) applyItems(ctx context.Context, s *player.PlayerSession, params []interface{}) error {
	itemID := paramInt(params, 0)
	op := paramInt(params, 1)
	qty := paramInt(params, 3)
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
		// 减少物品
		currentQty, err := e.store.GetItem(ctx, s.CharID, itemID)
		if err != nil {
			e.logger.Warn("applyItems: item not found for removal", zap.Int64("char_id", s.CharID), zap.Int("item_id", itemID))
			return fmt.Errorf("item %d not found in inventory", itemID)
		}
		if currentQty < qty {
			e.logger.Warn("applyItems: insufficient quantity", zap.Int64("char_id", s.CharID), zap.Int("item_id", itemID), zap.Int("have", currentQty), zap.Int("need", qty))
			return fmt.Errorf("insufficient quantity: have %d, need %d", currentQty, qty)
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
