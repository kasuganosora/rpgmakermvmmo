// Package npc implements RMMV event command execution for server-side NPC interactions.
package npc

import (
	"context"
	"encoding/json"

	"github.com/kasuganosora/rpgmakermvmmo/server/game/player"
	"github.com/kasuganosora/rpgmakermvmmo/server/model"
	"github.com/kasuganosora/rpgmakermvmmo/server/resource"
	"go.uber.org/zap"
	"gorm.io/gorm"
)

// RMMV event command codes handled by the server.
const (
	CmdEnd            = 0
	CmdShowText       = 101
	CmdShowTextLine   = 401
	CmdShowChoices    = 102
	CmdChangeSwitches = 121
	CmdChangeVars     = 122
	CmdChangeGold     = 125
	CmdChangeItems    = 126
	CmdTransfer       = 201
	CmdScript         = 355
	CmdScriptCont     = 655
)

// Executor runs RMMV event command lists for a player session.
type Executor struct {
	db     *gorm.DB
	res    *resource.ResourceLoader
	logger *zap.Logger
}

// New creates a new NPC Executor.
func New(db *gorm.DB, res *resource.ResourceLoader, logger *zap.Logger) *Executor {
	return &Executor{db: db, res: res, logger: logger}
}

// Execute processes a single event page for the given session.
// Dialog commands are sent to the client; item/gold changes are applied immediately.
func (e *Executor) Execute(ctx context.Context, s *player.PlayerSession, page *resource.EventPage) {
	if page == nil {
		return
	}
	cmds := page.List
	for i := 0; i < len(cmds); i++ {
		cmd := cmds[i]
		if cmd == nil {
			continue
		}
		switch cmd.Code {
		case CmdEnd:
			return

		case CmdShowText:
			// Collect continuation lines (code 401).
			var lines []string
			face := paramStr(cmd.Parameters, 0)
			for i+1 < len(cmds) && cmds[i+1] != nil && cmds[i+1].Code == CmdShowTextLine {
				i++
				lines = append(lines, paramStr(cmds[i].Parameters, 0))
			}
			e.sendDialog(s, face, lines)

		case CmdShowChoices:
			choices := paramList(cmd.Parameters, 0)
			e.sendChoices(s, choices)

		case CmdChangeGold:
			e.applyGold(ctx, s, cmd.Parameters)

		case CmdChangeItems:
			e.applyItems(ctx, s, cmd.Parameters)

		case CmdTransfer:
			e.transferPlayer(s, cmd.Parameters)

		case CmdScript, CmdScriptCont:
			// Script execution is handled by the JS sandbox (Task 08).
			// Skipped here.
		}
	}
}

// ExecuteEventByID finds and runs the first page of a map event by event ID.
func (e *Executor) ExecuteEventByID(ctx context.Context, s *player.PlayerSession, mapID, eventID int) {
	md, ok := e.res.Maps[mapID]
	if !ok {
		return
	}
	for _, ev := range md.Events {
		if ev == nil || ev.ID != eventID {
			continue
		}
		if len(ev.Pages) > 0 {
			e.Execute(ctx, s, ev.Pages[0])
		}
		return
	}
	e.logger.Warn("event not found", zap.Int("map_id", mapID), zap.Int("event_id", eventID))
}

// ---- helpers ----

func (e *Executor) sendDialog(s *player.PlayerSession, faceName string, lines []string) {
	payload, _ := json.Marshal(map[string]interface{}{
		"face":  faceName,
		"lines": lines,
	})
	s.Send(&player.Packet{Type: "npc_dialog", Payload: payload})
}

func (e *Executor) sendChoices(s *player.PlayerSession, choices []string) {
	payload, _ := json.Marshal(map[string]interface{}{
		"choices": choices,
	})
	s.Send(&player.Packet{Type: "npc_choices", Payload: payload})
}

// applyGold changes the player's gold based on RMMV ChangeGold command parameters.
// Parameters: [0]=operation(0=+,1=-), [1]=operandType(0=const,1=var), [2]=operand
func (e *Executor) applyGold(ctx context.Context, s *player.PlayerSession, params []interface{}) {
	op := paramInt(params, 0)
	amount := int64(paramInt(params, 2))
	if op == 1 {
		amount = -amount
	}
	e.db.WithContext(ctx).Model(&model.Character{}).Where("id = ?", s.CharID).
		Update("gold", gorm.Expr("gold + ?", amount))
}

// applyItems changes the player's inventory based on RMMV ChangeItems parameters.
// Parameters: [0]=itemID, [1]=operation(0=+,1=-), [2]=operandType, [3]=operand
func (e *Executor) applyItems(ctx context.Context, s *player.PlayerSession, params []interface{}) {
	itemID := paramInt(params, 0)
	op := paramInt(params, 1)
	qty := paramInt(params, 3)
	if itemID <= 0 {
		return
	}
	if op == 1 {
		// Remove item.
		var inv model.Inventory
		if err := e.db.WithContext(ctx).
			Where("char_id = ? AND item_id = ? AND kind = 1", s.CharID, itemID).
			First(&inv).Error; err != nil {
			return
		}
		newQty := inv.Qty - qty
		if newQty <= 0 {
			e.db.WithContext(ctx).Delete(&inv)
		} else {
			e.db.WithContext(ctx).Model(&inv).Update("qty", newQty)
		}
	} else {
		// Add item.
		var inv model.Inventory
		err := e.db.WithContext(ctx).
			Where("char_id = ? AND item_id = ? AND kind = 1", s.CharID, itemID).
			First(&inv).Error
		if err != nil {
			inv = model.Inventory{CharID: s.CharID, ItemID: itemID, Kind: model.ItemKindItem, Qty: qty}
			e.db.WithContext(ctx).Create(&inv)
		} else {
			e.db.WithContext(ctx).Model(&inv).Update("qty", inv.Qty+qty)
		}
	}
}

// transferPlayer sends a map transfer command to the client.
// Parameters: [0]=mode(0=direct,1=var), [1]=mapID, [2]=x, [3]=y, [4]=dir
func (e *Executor) transferPlayer(s *player.PlayerSession, params []interface{}) {
	mapID := paramInt(params, 1)
	x := paramInt(params, 2)
	y := paramInt(params, 3)
	dir := paramInt(params, 4)
	payload, _ := json.Marshal(map[string]interface{}{
		"map_id": mapID,
		"x":      x,
		"y":      y,
		"dir":    dir,
	})
	s.Send(&player.Packet{Type: "transfer_player", Payload: payload})
}

// ---- parameter helpers ----

func paramStr(params []interface{}, idx int) string {
	if idx >= len(params) {
		return ""
	}
	if s, ok := params[idx].(string); ok {
		return s
	}
	return ""
}

func paramInt(params []interface{}, idx int) int {
	if idx >= len(params) {
		return 0
	}
	switch v := params[idx].(type) {
	case int:
		return v
	case float64:
		return int(v)
	case int64:
		return int(v)
	}
	return 0
}

func paramList(params []interface{}, idx int) []string {
	if idx >= len(params) {
		return nil
	}
	raw, ok := params[idx].([]interface{})
	if !ok {
		return nil
	}
	result := make([]string, 0, len(raw))
	for _, v := range raw {
		if s, ok := v.(string); ok {
			result = append(result, s)
		}
	}
	return result
}
