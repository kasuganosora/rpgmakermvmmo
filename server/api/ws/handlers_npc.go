package ws

import (
	"context"
	"encoding/json"
	"time"

	"github.com/kasuganosora/rpgmakermvmmo/server/game/npc"
	"github.com/kasuganosora/rpgmakermvmmo/server/game/player"
	"github.com/kasuganosora/rpgmakermvmmo/server/game/world"
	"github.com/kasuganosora/rpgmakermvmmo/server/resource"
	"go.uber.org/zap"
	"gorm.io/gorm"
)

// NPCHandlers handles NPC interaction WebSocket messages.
type NPCHandlers struct {
	db         *gorm.DB
	res        *resource.ResourceLoader
	wm         *world.WorldManager
	executor   *npc.Executor
	logger     *zap.Logger
	transferFn npc.TransferFunc // server-side map transfer callback
}

// NewNPCHandlers creates NPCHandlers.
func NewNPCHandlers(db *gorm.DB, res *resource.ResourceLoader, wm *world.WorldManager, logger *zap.Logger) *NPCHandlers {
	return &NPCHandlers{
		db:       db,
		res:      res,
		wm:       wm,
		executor: npc.NewWithDB(db, res, logger),
		logger:   logger,
	}
}

// SetTransferFunc sets the callback used when NPC events execute Transfer Player (command 201).
func (h *NPCHandlers) SetTransferFunc(fn npc.TransferFunc) {
	h.transferFn = fn
}

// RegisterHandlers registers NPC-related WS handlers on the router.
func (h *NPCHandlers) RegisterHandlers(r *Router) {
	r.On("npc_interact", h.HandleInteract)
	r.On("npc_choice_reply", h.HandleChoiceReply)
	r.On("npc_dialog_ack", h.HandleDialogAck)
	r.On("npc_effect_ack", h.HandleEffectAck)
	r.On("scene_ready", h.HandleSceneReady)
}

// npcInteractRequest is the WS payload for npc_interact.
type npcInteractRequest struct {
	EventID int `json:"event_id"`
}

// HandleInteract processes a player interacting with a map event/NPC.
// Validates proximity, gets the active page, and runs the executor.
func (h *NPCHandlers) HandleInteract(ctx context.Context, s *player.PlayerSession, raw json.RawMessage) error {
	var req npcInteractRequest
	if err := json.Unmarshal(raw, &req); err != nil || req.EventID <= 0 {
		return nil
	}

	room := h.wm.Get(s.MapID)
	if room == nil {
		return nil
	}

	npcInst := room.GetNPC(req.EventID)
	if npcInst == nil {
		h.logger.Warn("npc_interact: NPC not found",
			zap.Int("event_id", req.EventID),
			zap.Int("map_id", s.MapID))
		return nil
	}

	// Proximity check — player must be within 1 tile of the NPC.
	px, py, _ := s.Position()
	dx := px - npcInst.X
	dy := py - npcInst.Y
	if dx < 0 {
		dx = -dx
	}
	if dy < 0 {
		dy = -dy
	}
	if dx > 1 || dy > 1 {
		h.logger.Warn("npc_interact: too far",
			zap.Int64("char_id", s.CharID),
			zap.Int("event_id", req.EventID),
			zap.Int("dx", dx), zap.Int("dy", dy))
		return nil
	}

	// Build per-player composite state.
	composite, err := h.wm.PlayerStateManager().GetComposite(s.CharID)
	if err != nil {
		h.logger.Error("failed to get player state", zap.Error(err), zap.Int64("char_id", s.CharID))
		return nil
	}

	// Get per-player active page instead of base ActivePage.
	activePage := room.GetActivePageForPlayer(req.EventID, composite)
	if activePage == nil {
		return nil
	}

	h.logger.Info("npc_interact executing",
		zap.Int64("char_id", s.CharID),
		zap.Int("event_id", req.EventID),
		zap.Int("trigger", activePage.Trigger),
		zap.Int("cmd_count", len(activePage.List)),
		zap.Int("npc_x", npcInst.X),
		zap.Int("npc_y", npcInst.Y))

	// Handle action button (0), player touch (1), and event touch (2).
	// Autorun (3) and parallel (4) are started by the server automatically.
	if activePage.Trigger > 2 {
		return nil
	}

	opts := &npc.ExecuteOpts{
		GameState:  composite,
		MapID:      s.MapID,
		EventID:    req.EventID,
		TransferFn: h.transferFn,
	}

	// Execute in a goroutine so the WS handler returns immediately.
	// The executor may block waiting for choice replies.
	go func() {
		h.executor.Execute(ctx, s, activePage, opts)
		// After execution, send per-player page changes (not broadcast).
		h.sendPageChangesToPlayer(s, room, composite)
	}()

	return nil
}

// npcChoiceReplyRequest is the WS payload for npc_choice_reply.
type npcChoiceReplyRequest struct {
	ChoiceIndex int `json:"choice_index"`
}

// HandleChoiceReply processes a player's dialog choice reply.
func (h *NPCHandlers) HandleChoiceReply(_ context.Context, s *player.PlayerSession, raw json.RawMessage) error {
	var req npcChoiceReplyRequest
	if err := json.Unmarshal(raw, &req); err != nil {
		return nil
	}
	// Non-blocking send to the executor's choice channel.
	select {
	case s.ChoiceCh <- req.ChoiceIndex:
	default:
		h.logger.Warn("choice reply dropped (no waiting executor)",
			zap.Int64("char_id", s.CharID))
	}
	return nil
}

// HandleSceneReady processes the client's signal that Scene_Map is fully loaded.
func (h *NPCHandlers) HandleSceneReady(_ context.Context, s *player.PlayerSession, _ json.RawMessage) error {
	select {
	case s.SceneReadyCh <- struct{}{}:
	default:
	}
	return nil
}

// HandleDialogAck processes a client's acknowledgment that a dialog was dismissed.
func (h *NPCHandlers) HandleDialogAck(_ context.Context, s *player.PlayerSession, _ json.RawMessage) error {
	select {
	case s.DialogAckCh <- struct{}{}:
	default:
		// No executor waiting — ignore.
	}
	return nil
}

// HandleEffectAck processes a client's acknowledgment that a visual effect has finished playing.
func (h *NPCHandlers) HandleEffectAck(_ context.Context, s *player.PlayerSession, _ json.RawMessage) error {
	select {
	case s.EffectAckCh <- struct{}{}:
	default:
		// No executor waiting — ignore.
	}
	return nil
}

// ExecuteAutoruns runs all autorun (trigger=3) events on the given map for a player.
// Called after a player enters a map via the autorunFn callback.
// Waits for the client to signal scene_ready before executing.
func (h *NPCHandlers) ExecuteAutoruns(s *player.PlayerSession, mapID int) {
	room := h.wm.Get(mapID)
	if room == nil {
		return
	}

	composite, err := h.wm.PlayerStateManager().GetComposite(s.CharID)
	if err != nil {
		h.logger.Error("failed to get player state for autoruns", zap.Error(err),
			zap.Int64("char_id", s.CharID))
		return
	}

	autoruns := room.GetAutorunNPCsForPlayer(composite)
	if len(autoruns) == 0 {
		return
	}

	// Wait for client Scene_Map to be fully loaded before executing autoruns.
	// This ensures Window_Message exists to display dialogs.
	h.logger.Info("waiting for scene_ready before autoruns",
		zap.Int64("char_id", s.CharID),
		zap.Int("map_id", mapID))
	timer := time.NewTimer(15 * time.Second)
	defer timer.Stop()
	select {
	case <-s.SceneReadyCh:
		// Client is ready.
	case <-timer.C:
		h.logger.Warn("scene_ready timeout, running autoruns anyway",
			zap.Int64("char_id", s.CharID))
	case <-s.Done:
		return // session closed
	}

	h.logger.Info("executing autorun events",
		zap.Int64("char_id", s.CharID),
		zap.Int("map_id", mapID),
		zap.Int("count", len(autoruns)))

	for _, npcInst := range autoruns {
		activePage := room.GetActivePageForPlayer(npcInst.EventID, composite)
		if activePage == nil || activePage.Trigger != 3 || len(activePage.List) <= 1 {
			continue
		}
		opts := &npc.ExecuteOpts{
			GameState:  composite,
			MapID:      mapID,
			EventID:    npcInst.EventID,
			TransferFn: h.transferFn,
		}
		h.executor.Execute(context.Background(), s, activePage, opts)

		// Send per-player page changes after each autorun.
		h.sendPageChangesToPlayer(s, room, composite)
	}
}

// sendPageChangesToPlayer re-evaluates all NPC pages for a single player's state
// and sends npc_page_change packets only to that player for any NPCs whose
// per-player page differs from the base (global) page.
func (h *NPCHandlers) sendPageChangesToPlayer(s *player.PlayerSession, room *world.MapRoom, state world.GameStateReader) {
	npcs := room.AllNPCs()
	for _, npcInst := range npcs {
		playerPage := room.GetActivePageForPlayer(npcInst.EventID, state)
		// Compare with base page — if they differ, send update to this player.
		if playerPage == npcInst.ActivePage {
			continue
		}
		data := map[string]interface{}{
			"event_id": npcInst.EventID,
			"dir":      npcInst.Dir,
		}
		if playerPage != nil {
			data["walk_name"] = playerPage.Image.CharacterName
			data["walk_index"] = playerPage.Image.CharacterIndex
			data["priority_type"] = playerPage.PriorityType
			data["step_anime"] = playerPage.StepAnime
			data["direction_fix"] = playerPage.DirectionFix
			data["through"] = playerPage.Through
			data["walk_anime"] = playerPage.WalkAnime
		} else {
			data["walk_name"] = ""
			data["walk_index"] = 0
			data["priority_type"] = 0
			data["step_anime"] = false
			data["direction_fix"] = false
			data["through"] = false
			data["walk_anime"] = false
		}
		payload, _ := json.Marshal(data)
		pkt, _ := json.Marshal(&player.Packet{Type: "npc_page_change", Payload: payload})
		s.SendRaw(pkt)
	}
}
