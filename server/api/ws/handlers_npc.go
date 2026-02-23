package ws

import (
	"context"
	"encoding/json"

	"github.com/kasuganosora/rpgmakermvmmo/server/game/npc"
	"github.com/kasuganosora/rpgmakermvmmo/server/game/player"
	"github.com/kasuganosora/rpgmakermvmmo/server/resource"
	"go.uber.org/zap"
	"gorm.io/gorm"
)

// NPCHandlers handles NPC interaction WebSocket messages.
type NPCHandlers struct {
	db       *gorm.DB
	res      *resource.ResourceLoader
	executor *npc.Executor
	logger   *zap.Logger
}

// NewNPCHandlers creates NPCHandlers.
func NewNPCHandlers(db *gorm.DB, res *resource.ResourceLoader, logger *zap.Logger) *NPCHandlers {
	return &NPCHandlers{
		db:       db,
		res:      res,
		executor: npc.New(db, res, logger),
		logger:   logger,
	}
}

// RegisterHandlers registers NPC-related WS handlers on the router.
func (h *NPCHandlers) RegisterHandlers(r *Router) {
	r.On("npc_interact", h.HandleInteract)
}

// npcInteractRequest is the WS payload for npc_interact.
type npcInteractRequest struct {
	EventID int `json:"event_id"`
}

// HandleInteract processes a player interacting with a map event/NPC.
// Payload: {"event_id": 3}
func (h *NPCHandlers) HandleInteract(ctx context.Context, s *player.PlayerSession, raw json.RawMessage) error {
	var req npcInteractRequest
	if err := json.Unmarshal(raw, &req); err != nil || req.EventID <= 0 {
		return nil
	}

	go func() {
		h.executor.ExecuteEventByID(ctx, s, s.MapID, req.EventID)
	}()
	return nil
}
