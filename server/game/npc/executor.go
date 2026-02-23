// Package npc implements RMMV event command execution for server-side NPC interactions.
package npc

import (
	"context"
	"encoding/json"
	"strconv"
	"strings"
	"time"

	"github.com/kasuganosora/rpgmakermvmmo/server/game/player"
	"github.com/kasuganosora/rpgmakermvmmo/server/model"
	"github.com/kasuganosora/rpgmakermvmmo/server/resource"
	"go.uber.org/zap"
	"gorm.io/gorm"
)

// RMMV event command codes handled by the server.
const (
	CmdEnd              = 0
	CmdShowText         = 101
	CmdShowTextLine     = 401
	CmdShowChoices      = 102
	CmdWhenBranch       = 402 // choice branch (When [n])
	CmdWhenCancel       = 403 // choice cancel branch
	CmdBranchEnd        = 404 // end of choice/conditional block
	CmdConditionalStart = 111
	CmdCallCommonEvent  = 117
	CmdElseBranch       = 411
	CmdConditionalEnd   = 412
	CmdChangeSwitches   = 121
	CmdChangeVars       = 122
	CmdChangeSelfSwitch = 123
	CmdChangeGold       = 125
	CmdChangeItems      = 126
	CmdTransfer         = 201
	CmdWait             = 230
	CmdSetMoveRoute     = 205
	CmdMoveRouteCont    = 505
	CmdFadeout          = 221
	CmdFadein           = 222
	CmdTintScreen       = 223
	CmdFlashScreen      = 224
	CmdShakeScreen      = 225
	CmdScreenEffect     = 211 // show/hide animation
	CmdPlayBGM          = 241
	CmdStopBGM          = 242
	CmdPlayBGS          = 245
	CmdStopBGS          = 246
	CmdPlaySE           = 250
	CmdStopSE           = 251
	CmdPlayME           = 249
	CmdComment          = 108
	CmdCommentCont      = 408
	CmdScript           = 355
	CmdScriptCont       = 655
	CmdPluginCommand    = 356
)

// GameStateAccessor provides read/write access to switches, variables, and self-switches.
type GameStateAccessor interface {
	GetSwitch(id int) bool
	SetSwitch(id int, val bool)
	GetVariable(id int) int
	SetVariable(id int, val int)
	GetSelfSwitch(mapID, eventID int, ch string) bool
	SetSelfSwitch(mapID, eventID int, ch string, val bool)
}

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

// TransferFunc is called by the executor when a Transfer Player command (201)
// is encountered. It performs the actual server-side map transfer.
type TransferFunc func(s *player.PlayerSession, mapID, x, y, dir int)

// ExecuteOpts holds optional parameters for Execute.
type ExecuteOpts struct {
	GameState  GameStateAccessor
	MapID      int
	EventID    int
	TransferFn TransferFunc // server-side transfer handler
}

// Execute processes a single event page for the given session.
// Dialog commands are sent to the client; item/gold changes are applied immediately.
// After sending choices, it waits for the player's reply via s.ChoiceCh.
func (e *Executor) Execute(ctx context.Context, s *player.PlayerSession, page *resource.EventPage, opts *ExecuteOpts) {
	if page == nil {
		return
	}
	e.executeList(ctx, s, page.List, opts, 0)
	e.sendDialogEnd(s)
}

// maxCallDepth prevents infinite recursion from common events calling each other.
const maxCallDepth = 10

// executeList runs a command list. Returns true if a terminating command (CmdEnd) was hit.
func (e *Executor) executeList(ctx context.Context, s *player.PlayerSession, cmds []*resource.EventCommand, opts *ExecuteOpts, depth int) bool {
	if depth > maxCallDepth {
		e.logger.Warn("common event call depth exceeded", zap.Int("depth", depth))
		return false
	}
	for i := 0; i < len(cmds); i++ {
		// Check context cancellation.
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
			// Code 0 appears both as sub-block markers (within conditionals at
			// indent > 0) and as the final list terminator (indent 0, last cmd).
			// In RMMV, code 0 is a no-op — the interpreter just moves on.
			// Only treat it as a true terminator if it's at indent 0.
			if cmd.Indent == 0 {
				return true
			}

		case CmdShowText:
			var lines []string
			face := paramStr(cmd.Parameters, 0)
			faceIndex := paramInt(cmd.Parameters, 1)
			for i+1 < len(cmds) && cmds[i+1] != nil && cmds[i+1].Code == CmdShowTextLine {
				i++
				lines = append(lines, paramStr(cmds[i].Parameters, 0))
			}
			e.sendDialog(s, face, faceIndex, lines)
			// Wait for client to acknowledge the dialog before continuing.
			if !e.waitForDialogAck(ctx, s) {
				return true
			}

		case CmdShowChoices:
			choices := paramList(cmd.Parameters, 0)
			cancelType := paramInt(cmd.Parameters, 1) // -1=disallow, 0-N=branch index
			e.sendChoices(s, choices)

			// Wait for player's choice reply.
			choiceIdx := e.waitForChoice(ctx, s)
			if choiceIdx < 0 {
				// Context cancelled or timeout — abort execution.
				return true
			}

			// Skip to the matching When branch (code 402) or cancel branch (code 403).
			i = e.skipToChoiceBranch(cmds, i, choiceIdx, cancelType)

		case CmdWhenBranch, CmdWhenCancel:
			// If we encounter a When branch during normal flow, it means
			// we already executed the chosen branch and need to skip to BranchEnd.
			i = e.skipToBranchEnd(cmds, i, cmd.Indent)

		case CmdBranchEnd:
			// Normal flow — continue.

		case CmdConditionalStart:
			if !e.evaluateCondition(cmd.Parameters, opts) {
				// Skip to ElseBranch (411) or ConditionalEnd (412) at same indent.
				i = e.skipToElseOrEnd(cmds, i, cmd.Indent)
			}

		case CmdElseBranch:
			// If we reach ElseBranch during normal flow, the if-branch was taken.
			// Skip to ConditionalEnd.
			i = e.skipToConditionalEnd(cmds, i, cmd.Indent)

		case CmdConditionalEnd:
			// Normal flow — continue.

		case CmdCallCommonEvent:
			ceID := paramInt(cmd.Parameters, 0)
			e.callCommonEvent(ctx, s, ceID, opts, depth)

		case CmdChangeSwitches:
			e.applySwitches(cmd.Parameters, opts)

		case CmdChangeVars:
			e.applyVariables(cmd.Parameters, opts)

		case CmdChangeSelfSwitch:
			e.applySelfSwitch(cmd.Parameters, opts)

		case CmdChangeGold:
			e.applyGold(ctx, s, cmd.Parameters)

		case CmdChangeItems:
			e.applyItems(ctx, s, cmd.Parameters)

		case CmdTransfer:
			e.transferPlayer(s, cmd.Parameters, opts)

		case CmdWait:
			// Wait N frames; at 60fps: frames/60 seconds.
			frames := paramInt(cmd.Parameters, 0)
			if frames > 0 {
				wait := time.Duration(frames) * time.Second / 60
				select {
				case <-time.After(wait):
				case <-ctx.Done():
					return true
				}
			}

		case CmdScript, CmdScriptCont:
			// Script execution placeholder — skip for now.

		case CmdPluginCommand:
			// Check for TE_CALL_ORIGIN_EVENT (TemplateEvent.js callback to original event).
			if e.handleTECallOriginEvent(ctx, s, cmd, opts, depth) {
				continue
			}
			// Other plugin commands — forward to client for execution.
			e.sendEffect(s, cmd)

		case CmdSetMoveRoute:
			// Move route — forward to client for rendering.
			e.sendEffect(s, cmd)
			// Skip any continuation lines (code 505).
			for i+1 < len(cmds) && cmds[i+1] != nil && cmds[i+1].Code == CmdMoveRouteCont {
				i++
			}

		case CmdMoveRouteCont:
			// Already consumed by CmdSetMoveRoute — skip.

		case CmdFadeout, CmdFadein, CmdTintScreen, CmdFlashScreen, CmdShakeScreen, CmdScreenEffect:
			// Screen effects — forward to client.
			e.sendEffect(s, cmd)

		case CmdPlayBGM, CmdStopBGM, CmdPlayBGS, CmdStopBGS, CmdPlaySE, CmdStopSE, CmdPlayME:
			// Audio — forward to client.
			e.sendEffect(s, cmd)

		case CmdComment, CmdCommentCont:
			// Developer comments — skip.
		}
	}
	return false
}

// callCommonEvent looks up a common event by ID and executes its command list.
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
			e.Execute(ctx, s, ev.Pages[0], &ExecuteOpts{MapID: mapID, EventID: eventID})
		}
		return
	}
	e.logger.Warn("event not found", zap.Int("map_id", mapID), zap.Int("event_id", eventID))
}

// ---- dialog helpers ----

func (e *Executor) sendDialog(s *player.PlayerSession, faceName string, faceIndex int, lines []string) {
	payload, _ := json.Marshal(map[string]interface{}{
		"face":       faceName,
		"face_index": faceIndex,
		"lines":      lines,
	})
	s.Send(&player.Packet{Type: "npc_dialog", Payload: payload})
}

func (e *Executor) sendChoices(s *player.PlayerSession, choices []string) {
	payload, _ := json.Marshal(map[string]interface{}{
		"choices": choices,
	})
	s.Send(&player.Packet{Type: "npc_choices", Payload: payload})
}

func (e *Executor) sendDialogEnd(s *player.PlayerSession) {
	s.Send(&player.Packet{Type: "npc_dialog_end"})
}

// ---- dialog/choice waiting ----

const dialogAckTimeout = 60 * time.Second
const choiceTimeout = 30 * time.Second

// waitForDialogAck blocks until the client acknowledges the dialog or context expires.
// Returns false on timeout/cancel.
func (e *Executor) waitForDialogAck(ctx context.Context, s *player.PlayerSession) bool {
	timer := time.NewTimer(dialogAckTimeout)
	defer timer.Stop()
	select {
	case <-s.DialogAckCh:
		return true
	case <-timer.C:
		e.logger.Warn("dialog ack timeout", zap.Int64("char_id", s.CharID))
		return false
	case <-ctx.Done():
		return false
	}
}

// waitForChoice blocks until the player sends a choice reply or context expires.
// Returns -1 on timeout/cancel.
func (e *Executor) waitForChoice(ctx context.Context, s *player.PlayerSession) int {
	timer := time.NewTimer(choiceTimeout)
	defer timer.Stop()
	select {
	case idx := <-s.ChoiceCh:
		return idx
	case <-timer.C:
		e.logger.Warn("choice timeout", zap.Int64("char_id", s.CharID))
		return -1
	case <-ctx.Done():
		return -1
	}
}

// ---- branch navigation ----

// skipToChoiceBranch advances past the ShowChoices to the matching When branch.
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
				return j // execution will continue from the command after this
			}
			branchCount++
		}
		if c.Code == CmdWhenCancel {
			if choiceIdx < 0 || choiceIdx == cancelType {
				return j
			}
		}
		if c.Code == CmdBranchEnd {
			return j // no matching branch found; continue after block
		}
	}
	return len(cmds) - 1
}

// skipToBranchEnd skips forward to the BranchEnd (code 404) at the given indent level.
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

// skipToElseOrEnd skips to ElseBranch (411) or ConditionalEnd (412) at indent.
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

// skipToConditionalEnd skips to ConditionalEnd (412) at indent.
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

// ---- conditional evaluation ----

// evaluateCondition checks an RMMV conditional branch (code 111).
// Parameters: [0]=conditionType, then type-specific params.
func (e *Executor) evaluateCondition(params []interface{}, opts *ExecuteOpts) bool {
	condType := paramInt(params, 0)
	gs := opts.GameState
	switch condType {
	case 0: // Switch
		switchID := paramInt(params, 1)
		expected := paramInt(params, 2) // 0=ON, 1=OFF
		if gs == nil {
			return false
		}
		val := gs.GetSwitch(switchID)
		if expected == 0 {
			return val
		}
		return !val

	case 1: // Variable
		varID := paramInt(params, 1)
		refType := paramInt(params, 2)  // 0=constant, 1=variable
		refVal := paramInt(params, 3)
		op := paramInt(params, 4) // 0=eq, 1=gte, 2=lte, 3=gt, 4=lt, 5=ne
		if gs == nil {
			return false
		}
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

	case 2: // Self-switch
		ch := paramStr(params, 1)     // "A","B","C","D"
		expected := paramInt(params, 2) // 0=ON, 1=OFF
		if gs == nil || opts == nil {
			return false
		}
		val := gs.GetSelfSwitch(opts.MapID, opts.EventID, ch)
		if expected == 0 {
			return val
		}
		return !val

	// Types 3-12 are player-specific (timer, actor, enemy, etc.) — skip on server.
	default:
		return true // unknown condition → treat as met (safe default)
	}
	return false
}

// ---- state changes ----

// applySwitches handles RMMV ChangeSwitches (code 121).
// Parameters: [0]=startID, [1]=endID, [2]=value (0=ON, 1=OFF)
func (e *Executor) applySwitches(params []interface{}, opts *ExecuteOpts) {
	if opts == nil || opts.GameState == nil {
		return
	}
	startID := paramInt(params, 0)
	endID := paramInt(params, 1)
	val := paramInt(params, 2) == 0 // 0=ON
	for id := startID; id <= endID; id++ {
		opts.GameState.SetSwitch(id, val)
	}
}

// applyVariables handles RMMV ChangeVariables (code 122).
// Parameters: [0]=startID, [1]=endID, [2]=operation(0=set,1=add,2=sub,3=mul,4=div,5=mod),
//
//	[3]=operandType(0=const,1=var,2=random), [4]=operand or min, [5]=max (for random)
func (e *Executor) applyVariables(params []interface{}, opts *ExecuteOpts) {
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
		case 0:
			current = val
		case 1:
			current += val
		case 2:
			current -= val
		case 3:
			current *= val
		case 4:
			if val != 0 {
				current /= val
			}
		case 5:
			if val != 0 {
				current %= val
			}
		}
		opts.GameState.SetVariable(id, current)
	}
}

// applySelfSwitch handles RMMV ChangeSelfSwitch (code 123).
// Parameters: [0]=channel("A"-"D"), [1]=value (0=ON, 1=OFF)
func (e *Executor) applySelfSwitch(params []interface{}, opts *ExecuteOpts) {
	if opts == nil || opts.GameState == nil {
		return
	}
	ch := paramStr(params, 0)
	val := paramInt(params, 1) == 0 // 0=ON
	opts.GameState.SetSelfSwitch(opts.MapID, opts.EventID, ch, val)
}

// ---- gold/items ----

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

// ---- transfer ----

// transferPlayer performs a server-side map transfer via the TransferFn callback.
// Parameters: [0]=mode(0=direct,1=var), [1]=mapID, [2]=x, [3]=y, [4]=dir
func (e *Executor) transferPlayer(s *player.PlayerSession, params []interface{}, opts *ExecuteOpts) {
	mode := paramInt(params, 0)
	mapID := paramInt(params, 1)
	x := paramInt(params, 2)
	y := paramInt(params, 3)
	dir := paramInt(params, 4)

	// Mode 1: resolve values from game variables.
	if mode == 1 && opts != nil && opts.GameState != nil {
		mapID = opts.GameState.GetVariable(mapID)
		x = opts.GameState.GetVariable(x)
		y = opts.GameState.GetVariable(y)
	}

	if dir <= 0 {
		dir = 2
	}

	e.logger.Info("executor transferPlayer",
		zap.Int64("char_id", s.CharID),
		zap.Int("mode", mode),
		zap.Int("dest_map", mapID),
		zap.Int("dest_x", x),
		zap.Int("dest_y", y),
		zap.Int("dest_dir", dir),
		zap.Int("from_map", opts.MapID),
		zap.Int("event_id", opts.EventID))

	if opts != nil && opts.TransferFn != nil {
		opts.TransferFn(s, mapID, x, y, dir)
		return
	}

	// Fallback: send transfer_player to client (no server-side handler available).
	e.logger.Warn("no TransferFn set, sending client-side transfer",
		zap.Int("map_id", mapID), zap.Int("x", x), zap.Int("y", y))
	payload, _ := json.Marshal(map[string]interface{}{
		"map_id": mapID,
		"x":      x,
		"y":      y,
		"dir":    dir,
	})
	s.Send(&player.Packet{Type: "transfer_player", Payload: payload})
}

// ---- TemplateEvent plugin command handling ----

// handleTECallOriginEvent checks if a plugin command (code 356) is a
// TemplateEvent.js command that should be handled server-side.
// Returns true if the command was handled (or intentionally skipped), false
// if it should be forwarded to the client.
//
// Handled TE commands:
//   - TE固有イベント呼び出し / TE_CALL_ORIGIN_EVENT — execute original event's page
//   - TE_CALL_MAP_EVENT / TEテンプレート呼び出し — call a template event by name + page
//   - TE_SET_SELF_VARIABLE — set a self-variable (absorbed silently)
//   - TE関連データ値デバッグ表示 — debug display (skipped)
func (e *Executor) handleTECallOriginEvent(ctx context.Context, s *player.PlayerSession, cmd *resource.EventCommand, opts *ExecuteOpts, depth int) bool {
	if len(cmd.Parameters) == 0 {
		return false
	}
	raw, _ := cmd.Parameters[0].(string)
	if raw == "" {
		return false
	}

	// Parse "CommandName arg1 arg2 ..." (RMMV code 356 format).
	parts := strings.Fields(raw)
	cmdName := parts[0]
	cmdArgs := parts[1:]

	switch cmdName {
	case "TE固有イベント呼び出し", "TE_CALL_ORIGIN_EVENT":
		return e.teCallOriginEvent(ctx, s, cmdArgs, opts, depth)

	case "TEテンプレート呼び出し", "TE_CALL_MAP_EVENT":
		return e.teCallMapEvent(ctx, s, cmdArgs, opts, depth)

	case "TE_SET_SELF_VARIABLE":
		// Self-variable management — absorb silently (not yet tracked server-side).
		return true

	case "TE関連データ値デバッグ表示":
		// Debug display — skip on server.
		return true
	}

	// Not a TE command — let the caller forward it to the client.
	return false
}

// teCallOriginEvent handles TE_CALL_ORIGIN_EVENT: executes the original
// (pre-template) event's page commands.
func (e *Executor) teCallOriginEvent(ctx context.Context, s *player.PlayerSession, args []string, opts *ExecuteOpts, depth int) bool {
	if opts == nil || opts.MapID <= 0 || opts.EventID <= 0 {
		e.logger.Warn("TE_CALL_ORIGIN_EVENT: missing map/event context")
		return true
	}

	mapEvent := e.findMapEvent(opts.MapID, opts.EventID)
	if mapEvent == nil {
		e.logger.Warn("TE_CALL_ORIGIN_EVENT: event not found",
			zap.Int("map_id", opts.MapID), zap.Int("event_id", opts.EventID))
		return true
	}

	if len(mapEvent.OriginalPages) == 0 {
		e.logger.Warn("TE_CALL_ORIGIN_EVENT: no original pages",
			zap.Int("map_id", opts.MapID), zap.Int("event_id", opts.EventID))
		return true
	}

	// Optional page index argument (defaults to 0).
	pageIdx := 0
	if len(args) > 0 {
		if idx, err := strconv.Atoi(args[0]); err == nil && idx >= 0 {
			pageIdx = idx
		}
	}
	if pageIdx >= len(mapEvent.OriginalPages) {
		pageIdx = 0
	}

	origPage := mapEvent.OriginalPages[pageIdx]
	if origPage == nil || len(origPage.List) == 0 {
		return true
	}

	e.logger.Info("TE_CALL_ORIGIN_EVENT: executing original page",
		zap.Int("map_id", opts.MapID),
		zap.Int("event_id", opts.EventID),
		zap.Int("page_idx", pageIdx),
		zap.Int("cmd_count", len(origPage.List)))

	e.executeList(ctx, s, origPage.List, opts, depth+1)
	return true
}

// teCallMapEvent handles TE_CALL_MAP_EVENT: calls a template event from the
// template map by name, executing a specific page's commands.
// Format: "TE_CALL_MAP_EVENT templateName pageIndex"
func (e *Executor) teCallMapEvent(ctx context.Context, s *player.PlayerSession, args []string, opts *ExecuteOpts, depth int) bool {
	if len(args) < 1 {
		e.logger.Warn("TE_CALL_MAP_EVENT: missing template name")
		return true
	}
	tmplName := args[0]
	pageIdx := 0
	if len(args) > 1 {
		if idx, err := strconv.Atoi(args[1]); err == nil && idx >= 0 {
			pageIdx = idx
		}
	}

	// Find the template event in the template map.
	// We search all maps for the event by name — the template map is typically
	// identified by TemplateMapId, but we don't have that config here.
	// Instead, search all maps for a matching event name.
	var tmplEvent *resource.MapEvent
	for _, md := range e.res.Maps {
		if md == nil {
			continue
		}
		for _, ev := range md.Events {
			if ev != nil && ev.Name == tmplName {
				tmplEvent = ev
				break
			}
		}
		if tmplEvent != nil {
			break
		}
	}

	if tmplEvent == nil {
		e.logger.Warn("TE_CALL_MAP_EVENT: template not found",
			zap.String("name", tmplName))
		return true
	}

	// RMMV page indices are 1-based in the plugin command but 0-based in array.
	// TemplateEvent.js uses 1-based page index in plugin commands.
	arrayIdx := pageIdx - 1
	if arrayIdx < 0 {
		arrayIdx = 0
	}
	if arrayIdx >= len(tmplEvent.Pages) {
		arrayIdx = 0
	}

	page := tmplEvent.Pages[arrayIdx]
	if page == nil || len(page.List) == 0 {
		return true
	}

	e.logger.Info("TE_CALL_MAP_EVENT: executing template page",
		zap.String("template", tmplName),
		zap.Int("page_idx", arrayIdx),
		zap.Int("cmd_count", len(page.List)))

	e.executeList(ctx, s, page.List, opts, depth+1)
	return true
}

// findMapEvent looks up a MapEvent by map ID and event ID.
func (e *Executor) findMapEvent(mapID, eventID int) *resource.MapEvent {
	md, ok := e.res.Maps[mapID]
	if !ok {
		return nil
	}
	for _, ev := range md.Events {
		if ev != nil && ev.ID == eventID {
			return ev
		}
	}
	return nil
}

// ---- visual effect forwarding ----

// sendEffect forwards a visual/audio RMMV command to the client as an npc_effect message.
// The client is expected to execute the corresponding RMMV function and acknowledge completion.
func (e *Executor) sendEffect(s *player.PlayerSession, cmd *resource.EventCommand) {
	payload, _ := json.Marshal(map[string]interface{}{
		"code":   cmd.Code,
		"indent": cmd.Indent,
		"params": cmd.Parameters,
	})
	s.Send(&player.Packet{Type: "npc_effect", Payload: payload})
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
