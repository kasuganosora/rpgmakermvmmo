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
		s.Send(&player.Packet{Type: "event_end"}) // 客户端已设置 _serverEventActive
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
		s.Send(&player.Packet{Type: "event_end"}) // 客户端已设置 _serverEventActive
		return nil
	}

	// Build per-player composite state.
	composite, err := h.wm.PlayerStateManager().GetComposite(s.CharID)
	if err != nil {
		h.logger.Error("failed to get player state", zap.Error(err), zap.Int64("char_id", s.CharID))
		s.Send(&player.Packet{Type: "event_end"}) // 客户端已设置 _serverEventActive
		return nil
	}

	// Get per-player active page instead of base ActivePage.
	activePage := room.GetActivePageForPlayer(req.EventID, composite)
	if activePage == nil {
		s.Send(&player.Packet{Type: "event_end"}) // 客户端已设置 _serverEventActive
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
		s.Send(&player.Packet{Type: "event_end"}) // 客户端已设置 _serverEventActive
		return nil
	}

	// 防止并发事件：已有事件运行时拒绝新的交互。
	// 不发送 event_end：已有事件正在运行，其结束时会发送 event_end。
	if !s.EventMu.TryLock() {
		h.logger.Info("npc_interact rejected: player already in event",
			zap.Int64("char_id", s.CharID),
			zap.Int("event_id", req.EventID))
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
	startMapID := s.MapID
	go func() {
		defer s.EventMu.Unlock()
		// 通知客户端事件开始，阻止移动和交互。
		s.Send(&player.Packet{Type: "event_start"})
		// 预设标志：假设可能发生 Transfer，autorun 需要接管 event_end。
		// 如果没有发生 Transfer，在下方清除并发送 event_end。
		s.SetNeedEventEnd(true)

		h.executor.Execute(ctx, s, activePage, opts)
		// After execution, send per-player page changes (not broadcast).
		h.sendPageChangesToPlayer(s, room, composite)
		// 如果执行过程中发生了 Transfer，还需更新新地图的 NPC 页面。
		if s.MapID != startMapID {
			if currentRoom := h.wm.Get(s.MapID); currentRoom != nil {
				if fc, err := h.wm.PlayerStateManager().GetComposite(s.CharID); err == nil {
					h.sendPageChangesToPlayer(s, currentRoom, fc)
				}
			}
			// Transfer 发生 — needEventEnd 保持为 true。
			// autorun goroutine（由 EnterMapRoom 生成）将在完成后发送 event_end。
			// 此处不发送 event_end，避免客户端在 autorun 开始前短暂解除移动锁。
			return
		}

		// 未发生 Transfer — 清除标志并发送 event_end。
		s.SetNeedEventEnd(false)
		s.Send(&player.Packet{Type: "event_end"})
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
	// sendEventEndIfNeeded 清除 needEventEnd 标志并发送 event_end（如果需要）。
	// 用于前一个事件（HandleInteract）发生了 Transfer 后未发送 event_end 的情况。
	sendEventEndIfNeeded := func() {
		if s.ClearNeedEventEnd() {
			s.Send(&player.Packet{Type: "event_end"})
		}
	}

	room := h.wm.Get(mapID)
	if room == nil {
		sendEventEndIfNeeded()
		return
	}

	composite, err := h.wm.PlayerStateManager().GetComposite(s.CharID)
	if err != nil {
		h.logger.Error("failed to get player state for autoruns", zap.Error(err),
			zap.Int64("char_id", s.CharID))
		sendEventEndIfNeeded()
		return
	}

	autoruns := room.GetAutorunNPCsForPlayer(composite)
	if len(autoruns) == 0 {
		sendEventEndIfNeeded()
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
		sendEventEndIfNeeded()
		return // session closed
	}

	// Stale check: player may have already left this map during scene_ready wait.
	if s.MapID != mapID {
		h.logger.Info("autorun skipped: player already left map",
			zap.Int64("char_id", s.CharID),
			zap.Int("expected_map", mapID),
			zap.Int("current_map", s.MapID))
		sendEventEndIfNeeded()
		return
	}

	// 阻塞等待当前事件完成，序列化自动运行事件。
	// 若玩家正在执行 NPC 事件（EventMu 被锁），自动运行排队等待。
	s.EventMu.Lock()

	// Stale check after acquiring lock — player may have transferred during wait.
	if s.MapID != mapID {
		s.EventMu.Unlock()
		h.logger.Info("autorun skipped: player left map while waiting for event lock",
			zap.Int64("char_id", s.CharID),
			zap.Int("expected_map", mapID),
			zap.Int("current_map", s.MapID))
		sendEventEndIfNeeded()
		return
	}

	defer s.EventMu.Unlock()

	// 检查是否继承了前一个事件的 event_start（Transfer 后未发送 event_end）。
	inherited := s.ClearNeedEventEnd()
	if !inherited {
		// 正常的 autorun 调用 — 发送自己的 event_start。
		s.Send(&player.Packet{Type: "event_start"})
	}
	// 无论是否继承，autorun 完成后都发送 event_end。
	defer s.Send(&player.Packet{Type: "event_end"})

	h.logger.Info("executing autorun events",
		zap.Int64("char_id", s.CharID),
		zap.Int("map_id", mapID),
		zap.Int("count", len(autoruns)))

	for _, npcInst := range autoruns {
		// If a previous autorun transferred the player, stop executing this map's autoruns.
		if s.MapID != mapID {
			break
		}
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

	// Autorun 中可能执行了 Transfer（cmd 201），导致玩家已在不同地图。
	// 此时上面的 sendPageChangesToPlayer 只更新了原始地图（room）的 NPC，
	// 新地图的 NPC 页面未被重新评估。
	// 例：Map 20 autorun 先 Transfer 到 Map 67，再设 Switch 317=ON，
	// 但 Map 67 的 map_init 发送时 switch 317 还是 OFF → 柜子事件不显示。
	// 这里重新获取 composite（反映 autorun 中设置的最新开关/变量），
	// 并对玩家当前所在地图发送 page changes。
	if s.MapID != mapID {
		currentRoom := h.wm.Get(s.MapID)
		if currentRoom != nil {
			freshComposite, err := h.wm.PlayerStateManager().GetComposite(s.CharID)
			if err == nil {
				h.sendPageChangesToPlayer(s, currentRoom, freshComposite)
			}
		}
	}
}

// ExecuteTouchEvent runs a touch-trigger (trigger 1/2) event at (x, y) for a player.
// Called by GameHandlers.HandleMove when the player steps onto an event that has
// no top-level transfer command (conditional transfers, dialogs, etc.).
func (h *NPCHandlers) ExecuteTouchEvent(s *player.PlayerSession, mapID, x, y int) {
	room := h.wm.Get(mapID)
	if room == nil {
		return
	}

	composite, err := h.wm.PlayerStateManager().GetComposite(s.CharID)
	if err != nil {
		return
	}

	eventID, activePage := room.GetTouchEventAtForPlayer(x, y, composite)
	if activePage == nil {
		return
	}

	// Non-blocking lock — don't block movement if an event is already running.
	if !s.EventMu.TryLock() {
		return
	}

	h.logger.Info("touch event executing",
		zap.Int64("char_id", s.CharID),
		zap.Int("map_id", mapID),
		zap.Int("event_id", eventID),
		zap.Int("x", x), zap.Int("y", y),
		zap.Int("trigger", activePage.Trigger))

	opts := &npc.ExecuteOpts{
		GameState:  composite,
		MapID:      mapID,
		EventID:    eventID,
		TransferFn: h.transferFn,
	}

	startMapID := s.MapID
	go func() {
		defer s.EventMu.Unlock()
		s.Send(&player.Packet{Type: "event_start"})
		s.SetNeedEventEnd(true)

		h.executor.Execute(context.Background(), s, activePage, opts)
		h.sendPageChangesToPlayer(s, room, composite)

		if s.MapID != startMapID {
			if currentRoom := h.wm.Get(s.MapID); currentRoom != nil {
				if fc, err := h.wm.PlayerStateManager().GetComposite(s.CharID); err == nil {
					h.sendPageChangesToPlayer(s, currentRoom, fc)
				}
			}
			// Transfer happened — needEventEnd stays true for autorun to pick up.
			return
		}

		s.SetNeedEventEnd(false)
		s.Send(&player.Packet{Type: "event_end"})
	}()
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
			img := playerPage.Image
			walkName := img.CharacterName
			// 图块事件（柜子、门等）使用 TileID 而非行走图。
			if img.TileID > 0 {
				walkName = ""
				data["tile_id"] = img.TileID
			}
			h.logger.Info("npc_page_change",
				zap.Int("event_id", npcInst.EventID),
				zap.String("walk_name", walkName),
				zap.Int("tile_id", img.TileID),
				zap.Bool("playerPage_nil", playerPage == nil),
				zap.Bool("globalPage_nil", npcInst.ActivePage == nil))
			data["walk_name"] = walkName
			data["walk_index"] = img.CharacterIndex
			data["priority_type"] = playerPage.PriorityType
			data["step_anime"] = playerPage.StepAnime
			data["direction_fix"] = playerPage.DirectionFix
			data["through"] = playerPage.Through
			data["walk_anime"] = playerPage.WalkAnime
			// YEP_IconsOnEvents: extract <Icon on Event: N> from commands/note.
			if icon := world.ExtractIconOnEvent(npcInst.MapEvent, playerPage); icon > 0 {
				data["icon_on_event"] = icon
			}
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
