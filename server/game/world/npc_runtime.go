package world

import (
	"github.com/kasuganosora/rpgmakermvmmo/server/resource"
	"go.uber.org/zap"
)

// NPCRuntime holds the server-side runtime state for a single map event/NPC.
type NPCRuntime struct {
	EventID    int
	Name       string
	X, Y       int    // current position (may differ from MapEvent.X/Y after movement)
	Dir        int    // current facing direction
	ActivePage *resource.EventPage  // currently active page (highest matching)
	MapEvent   *resource.MapEvent   // reference to static event data

	// Movement state
	moveTimer int // ticks until next movement attempt
	routeIdx  int // current index in custom MoveRoute

	// Dirty flag — set when position/direction changes, cleared after broadcast
	dirty bool
}

// selectPage chooses the highest-index page whose conditions are met.
// RMMV convention: pages are checked from last to first; the first match wins.
// actorValid and itemValid are skipped (server doesn't track per-player actor/item conditions).
func selectPage(ev *resource.MapEvent, mapID int, state GameStateReader) *resource.EventPage {
	if state == nil {
		// No game state — return first page as default.
		if len(ev.Pages) > 0 {
			return ev.Pages[0]
		}
		return nil
	}
	for i := len(ev.Pages) - 1; i >= 0; i-- {
		page := ev.Pages[i]
		if page == nil {
			continue
		}
		if meetsConditions(&page.Conditions, mapID, ev.ID, state) {
			return page
		}
	}
	return nil
}

// meetsConditions checks whether all enabled conditions on a page are satisfied.
func meetsConditions(cond *resource.EventPageConditions, mapID, eventID int, state GameStateReader) bool {
	if cond.Switch1Valid && !state.GetSwitch(cond.Switch1ID) {
		return false
	}
	if cond.Switch2Valid && !state.GetSwitch(cond.Switch2ID) {
		return false
	}
	if cond.VariableValid && state.GetVariable(cond.VariableID) < cond.VariableValue {
		return false
	}
	if cond.SelfSwitchValid && !state.GetSelfSwitch(mapID, eventID, cond.SelfSwitchCh) {
		return false
	}
	// actorValid and itemValid are per-player conditions; skip on server.
	return true
}

// populateNPCs creates NPCRuntime entries for all events on this map.
func (room *MapRoom) populateNPCs() {
	if room.res == nil {
		return
	}
	md, ok := room.res.Maps[room.MapID]
	if !ok {
		return
	}
	for _, ev := range md.Events {
		if ev == nil || len(ev.Pages) == 0 {
			continue
		}
		activePage := selectPage(ev, room.MapID, room.state)
		dir := 2 // default facing down
		if activePage != nil && activePage.Image.Direction > 0 {
			dir = activePage.Image.Direction
		}
		npc := &NPCRuntime{
			EventID:    ev.ID,
			Name:       ev.Name,
			X:          ev.X,
			Y:          ev.Y,
			Dir:        dir,
			ActivePage: activePage,
			MapEvent:   ev,
		}
		room.npcs = append(room.npcs, npc)
		// Log template events with OriginalPages for debugging transfer detection.
		if ev.OriginalPages != nil {
			room.logger.Info("NPC with OriginalPages",
				zap.Int("event_id", ev.ID),
				zap.String("name", ev.Name),
				zap.Int("x", ev.X), zap.Int("y", ev.Y),
				zap.Int("orig_pages", len(ev.OriginalPages)),
				zap.Int("tmpl_pages", len(ev.Pages)),
				zap.Bool("has_active", activePage != nil))
		}
	}
	room.logger.Info("populated NPCs",
		zap.Int("map_id", room.MapID),
		zap.Int("count", len(room.npcs)))
}

// AllNPCs returns a snapshot slice of all NPCRuntime entries in the room.
func (room *MapRoom) AllNPCs() []*NPCRuntime {
	room.mu.RLock()
	defer room.mu.RUnlock()
	out := make([]*NPCRuntime, len(room.npcs))
	copy(out, room.npcs)
	return out
}

// GetNPC returns the NPCRuntime for the given event ID, or nil.
func (room *MapRoom) GetNPC(eventID int) *NPCRuntime {
	room.mu.RLock()
	defer room.mu.RUnlock()
	for _, n := range room.npcs {
		if n.EventID == eventID {
			return n
		}
	}
	return nil
}

// NPCSnapshot returns a snapshot of all NPCs suitable for map_init.
func (room *MapRoom) NPCSnapshot() []map[string]interface{} {
	room.mu.RLock()
	defer room.mu.RUnlock()
	out := make([]map[string]interface{}, 0, len(room.npcs))
	for _, n := range room.npcs {
		snap := map[string]interface{}{
			"event_id": n.EventID,
			"name":     n.Name,
			"x":        n.X,
			"y":        n.Y,
			"dir":      n.Dir,
		}
		if n.ActivePage != nil {
			snap["walk_name"] = n.ActivePage.Image.CharacterName
			snap["walk_index"] = n.ActivePage.Image.CharacterIndex
			snap["priority_type"] = n.ActivePage.PriorityType
			snap["move_type"] = n.ActivePage.MoveType
			snap["step_anime"] = n.ActivePage.StepAnime
			snap["direction_fix"] = n.ActivePage.DirectionFix
			snap["through"] = n.ActivePage.Through
			snap["walk_anime"] = n.ActivePage.WalkAnime
		} else {
			// No active page — NPC is invisible
			snap["walk_name"] = ""
			snap["walk_index"] = 0
			snap["priority_type"] = 0
			snap["move_type"] = 0
			snap["step_anime"] = false
			snap["direction_fix"] = false
			snap["through"] = false
			snap["walk_anime"] = false
		}
		out = append(out, snap)
	}
	return out
}

// TransferData holds the destination info parsed from an RMMV Transfer Player command (code 201).
type TransferData struct {
	MapID int
	X     int
	Y     int
	Dir   int
}

// GetTransferAt checks whether position (x, y) has a transfer event that should auto-trigger.
//
// Strategy: check the NPC's ACTIVE page first. If the active page has trigger 1/2 and a
// transfer command → return it. If the active page has trigger 1/2 but NO transfer (e.g.,
// it shows "don't go out" dialog) → return nil so the executor handles it via npc_interact.
// If no active page exists → fall back to scanning all pages for navigation safety.
func (room *MapRoom) GetTransferAt(x, y int) *TransferData {
	room.mu.RLock()
	defer room.mu.RUnlock()
	for _, n := range room.npcs {
		if n.X != x || n.Y != y {
			continue
		}
		if n.MapEvent == nil || len(n.MapEvent.Pages) == 0 {
			continue
		}

		// Case 1: NPC has an active page — respect its behavior.
		if n.ActivePage != nil {
			if n.ActivePage.Trigger != 1 && n.ActivePage.Trigger != 2 {
				continue // active page isn't player/event touch — skip this NPC
			}
			// Check if active page has a transfer command.
			if td := findTransferInPage(n.ActivePage); td != nil {
				return td
			}
			// For TemplateEvent events: the template page may use TE_CALL_ORIGIN_EVENT
			// (code 356) to call back to the original event's commands. Check if the
			// active page calls the origin event and the original page has a transfer.
			hasTE := hasCallOriginEvent(n.ActivePage)
			hasOrig := n.MapEvent.OriginalPages != nil
			room.logger.Info("GetTransferAt: TE check",
				zap.Int("event_id", n.EventID),
				zap.Bool("has_te_call", hasTE),
				zap.Bool("has_original_pages", hasOrig),
				zap.Int("active_trigger", n.ActivePage.Trigger),
				zap.Int("active_cmd_count", len(n.ActivePage.List)))
			if hasOrig && hasTE {
				for _, origPage := range n.MapEvent.OriginalPages {
					if origPage == nil {
						continue
					}
					if td := findTransferInPage(origPage); td != nil {
						room.logger.Info("GetTransferAt: found transfer in original page",
							zap.Int("event_id", n.EventID),
							zap.Int("dest_map", td.MapID))
						return td
					}
				}
			}
			// Active page has trigger 1/2 but NO transfer command.
			// It has other commands (dialog, move route, etc.) that the executor
			// should handle via npc_interact. Don't auto-transfer.
			return nil
		}

		// Case 2: No active page (conditions not met) — fall back to scanning
		// all pages for navigation safety so players can still navigate between maps.
		for _, page := range n.MapEvent.Pages {
			if page == nil {
				continue
			}
			if page.Trigger != 1 && page.Trigger != 2 {
				continue
			}
			if td := findTransferInPage(page); td != nil {
				return td
			}
		}
	}
	return nil
}

// hasCallOriginEvent checks if a page contains a TE_CALL_ORIGIN_EVENT plugin command
// (TemplateEvent.js code 356 with "TE固有イベント呼び出し" or "TE_CALL_ORIGIN_EVENT").
func hasCallOriginEvent(page *resource.EventPage) bool {
	for _, cmd := range page.List {
		if cmd == nil || cmd.Code != 356 {
			continue
		}
		if len(cmd.Parameters) > 0 {
			s, _ := cmd.Parameters[0].(string)
			if s == "TE固有イベント呼び出し" || s == "TE_CALL_ORIGIN_EVENT" {
				return true
			}
		}
	}
	return false
}

// findTransferInPage looks for a Transfer Player command (code 201) in a page's command list.
func findTransferInPage(page *resource.EventPage) *TransferData {
	for _, cmd := range page.List {
		if cmd == nil || cmd.Code != 201 {
			continue
		}
		// RMMV command 201 parameters: [mode, mapId, x, y, direction, fadeType]
		if len(cmd.Parameters) < 5 {
			continue
		}
		mode := toInt(cmd.Parameters[0])
		if mode != 0 {
			continue // skip variable-based transfers
		}
		return &TransferData{
			MapID: toInt(cmd.Parameters[1]),
			X:     toInt(cmd.Parameters[2]),
			Y:     toInt(cmd.Parameters[3]),
			Dir:   toInt(cmd.Parameters[4]),
		}
	}
	return nil
}

// GetEntryPoints returns positions of all transfer events on this map.
// Transfer events (trigger 1/2 with command 201) are doorways/portals — their
// positions are guaranteed valid player-standing positions set by the map maker.
func (room *MapRoom) GetEntryPoints() []struct{ X, Y int } {
	room.mu.RLock()
	defer room.mu.RUnlock()
	var points []struct{ X, Y int }
	seen := make(map[[2]int]bool)
	for _, n := range room.npcs {
		if n.MapEvent == nil {
			continue
		}
		for _, page := range n.MapEvent.Pages {
			if page == nil {
				continue
			}
			if page.Trigger != 1 && page.Trigger != 2 {
				continue
			}
			for _, cmd := range page.List {
				if cmd != nil && cmd.Code == 201 {
					key := [2]int{n.X, n.Y}
					if !seen[key] {
						seen[key] = true
						points = append(points, struct{ X, Y int }{n.X, n.Y})
					}
					break
				}
			}
		}
	}
	return points
}

// toInt converts an interface{} (typically float64 from JSON) to int.
func toInt(v interface{}) int {
	switch n := v.(type) {
	case float64:
		return int(n)
	case int:
		return n
	case int64:
		return int(n)
	default:
		return 0
	}
}

// GetAutorunNPCs returns NPCs whose active page has trigger=3 (autorun).
// These events should be executed automatically when a player enters the map.
func (room *MapRoom) GetAutorunNPCs() []*NPCRuntime {
	room.mu.RLock()
	defer room.mu.RUnlock()
	var result []*NPCRuntime
	for _, n := range room.npcs {
		if n.ActivePage != nil && n.ActivePage.Trigger == 3 && len(n.ActivePage.List) > 1 {
			// trigger=3 is autorun; require >1 commands (the last is always code 0 end marker)
			result = append(result, n)
		}
	}
	return result
}

// RefreshNPCPages re-evaluates all NPC active pages (global state) and returns event IDs that changed.
// Used for base/movement page updates. For per-player page changes, use the handler-level logic.
func (room *MapRoom) RefreshNPCPages() []int {
	room.mu.Lock()
	defer room.mu.Unlock()
	var changed []int
	for _, n := range room.npcs {
		oldPage := n.ActivePage
		newPage := selectPage(n.MapEvent, room.MapID, room.state)
		if oldPage != newPage {
			n.ActivePage = newPage
			// Update direction from new page image if available
			if newPage != nil && newPage.Image.Direction > 0 {
				n.Dir = newPage.Image.Direction
			}
			changed = append(changed, n.EventID)
		}
	}
	return changed
}

// ---------------------------------------------------------------------------
// Per-player variants — compute NPC state using player-specific GameStateReader.
// ---------------------------------------------------------------------------

// NPCSnapshotForPlayer returns NPC snapshots using per-player page computation.
func (room *MapRoom) NPCSnapshotForPlayer(state GameStateReader) []map[string]interface{} {
	room.mu.RLock()
	defer room.mu.RUnlock()
	out := make([]map[string]interface{}, 0, len(room.npcs))
	for _, n := range room.npcs {
		activePage := selectPage(n.MapEvent, room.MapID, state)
		snap := map[string]interface{}{
			"event_id": n.EventID,
			"name":     n.Name,
			"x":        n.X,
			"y":        n.Y,
			"dir":      n.Dir,
		}
		if activePage != nil {
			snap["walk_name"] = activePage.Image.CharacterName
			snap["walk_index"] = activePage.Image.CharacterIndex
			snap["priority_type"] = activePage.PriorityType
			snap["move_type"] = activePage.MoveType
			snap["step_anime"] = activePage.StepAnime
			snap["direction_fix"] = activePage.DirectionFix
			snap["through"] = activePage.Through
			snap["walk_anime"] = activePage.WalkAnime
		} else {
			snap["walk_name"] = ""
			snap["walk_index"] = 0
			snap["priority_type"] = 0
			snap["move_type"] = 0
			snap["step_anime"] = false
			snap["direction_fix"] = false
			snap["through"] = false
			snap["walk_anime"] = false
		}
		out = append(out, snap)
	}
	return out
}

// GetTransferAtForPlayer checks transfer at (x, y) using per-player state for page selection.
func (room *MapRoom) GetTransferAtForPlayer(x, y int, state GameStateReader) *TransferData {
	room.mu.RLock()
	defer room.mu.RUnlock()
	for _, n := range room.npcs {
		if n.X != x || n.Y != y {
			continue
		}
		if n.MapEvent == nil || len(n.MapEvent.Pages) == 0 {
			continue
		}

		activePage := selectPage(n.MapEvent, room.MapID, state)

		if activePage != nil {
			if activePage.Trigger != 1 && activePage.Trigger != 2 {
				continue
			}
			if td := findTransferInPage(activePage); td != nil {
				return td
			}
			hasTE := hasCallOriginEvent(activePage)
			hasOrig := n.MapEvent.OriginalPages != nil
			if hasOrig && hasTE {
				for _, origPage := range n.MapEvent.OriginalPages {
					if origPage == nil {
						continue
					}
					if td := findTransferInPage(origPage); td != nil {
						return td
					}
				}
			}
			return nil
		}

		// No active page fallback — scan all pages for navigation safety.
		for _, page := range n.MapEvent.Pages {
			if page == nil {
				continue
			}
			if page.Trigger != 1 && page.Trigger != 2 {
				continue
			}
			if td := findTransferInPage(page); td != nil {
				return td
			}
		}
	}
	return nil
}

// GetActivePageForPlayer computes the per-player active page for a specific NPC.
func (room *MapRoom) GetActivePageForPlayer(eventID int, state GameStateReader) *resource.EventPage {
	room.mu.RLock()
	defer room.mu.RUnlock()
	for _, n := range room.npcs {
		if n.EventID == eventID {
			return selectPage(n.MapEvent, room.MapID, state)
		}
	}
	return nil
}

// GetAutorunNPCsForPlayer returns NPCs whose per-player active page has trigger=3.
func (room *MapRoom) GetAutorunNPCsForPlayer(state GameStateReader) []*NPCRuntime {
	room.mu.RLock()
	defer room.mu.RUnlock()
	var result []*NPCRuntime
	for _, n := range room.npcs {
		page := selectPage(n.MapEvent, room.MapID, state)
		if page != nil && page.Trigger == 3 && len(page.List) > 1 {
			result = append(result, n)
		}
	}
	return result
}
