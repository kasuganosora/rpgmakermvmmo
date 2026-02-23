package ws

import (
	"context"
	"encoding/json"

	"github.com/kasuganosora/rpgmakermvmmo/server/game/player"
	"github.com/kasuganosora/rpgmakermvmmo/server/game/trade"
	"go.uber.org/zap"
	"gorm.io/gorm"
)

// TradeHandlers handles player-to-player trade WebSocket messages.
type TradeHandlers struct {
	db      *gorm.DB
	svc     *trade.Service
	sm      *player.SessionManager
	logger  *zap.Logger
}

// NewTradeHandlers creates TradeHandlers.
func NewTradeHandlers(db *gorm.DB, svc *trade.Service, sm *player.SessionManager, logger *zap.Logger) *TradeHandlers {
	return &TradeHandlers{db: db, svc: svc, sm: sm, logger: logger}
}

// RegisterHandlers registers trade WS handlers.
func (h *TradeHandlers) RegisterHandlers(r *Router) {
	r.On("trade_request", h.HandleRequest)
	r.On("trade_accept", h.HandleAccept)
	r.On("trade_update", h.HandleUpdate)
	r.On("trade_confirm", h.HandleConfirm)
	r.On("trade_cancel", h.HandleCancel)
}

type tradeRequestPayload struct {
	TargetCharID int64 `json:"target_char_id"`
}

// HandleRequest initiates a trade request to another player.
func (h *TradeHandlers) HandleRequest(_ context.Context, s *player.PlayerSession, raw json.RawMessage) error {
	var req tradeRequestPayload
	if err := json.Unmarshal(raw, &req); err != nil {
		return nil
	}
	target := h.sm.Get(req.TargetCharID)
	if target == nil {
		replyError(s, "target_offline")
		return nil
	}
	if err := h.svc.RequestTrade(s, target); err != nil {
		replyError(s, err.Error())
	}
	return nil
}

type tradeAcceptPayload struct {
	FromCharID int64 `json:"from_char_id"`
}

// HandleAccept accepts an incoming trade request.
func (h *TradeHandlers) HandleAccept(_ context.Context, s *player.PlayerSession, raw json.RawMessage) error {
	var req tradeAcceptPayload
	if err := json.Unmarshal(raw, &req); err != nil {
		return nil
	}
	initiator := h.sm.Get(req.FromCharID)
	if initiator == nil {
		replyError(s, "initiator_offline")
		return nil
	}
	sess := h.svc.AcceptTrade(initiator, s)
	if sess != nil {
		payload, _ := json.Marshal(map[string]interface{}{"session_id": sess.ID})
		initiator.Send(&player.Packet{Type: "trade_accepted", Payload: payload})
		s.Send(&player.Packet{Type: "trade_accepted", Payload: payload})
	}
	return nil
}

type tradeUpdatePayload struct {
	ItemIDs []int64 `json:"item_ids"`
	Gold    int64   `json:"gold"`
}

// HandleUpdate updates the current player's trade offer.
func (h *TradeHandlers) HandleUpdate(_ context.Context, s *player.PlayerSession, raw json.RawMessage) error {
	var req tradeUpdatePayload
	if err := json.Unmarshal(raw, &req); err != nil {
		return nil
	}
	if err := h.svc.UpdateOffer(s, req.ItemIDs, req.Gold); err != nil {
		replyError(s, err.Error())
	}
	return nil
}

// HandleConfirm confirms the current player's side of the trade.
func (h *TradeHandlers) HandleConfirm(ctx context.Context, s *player.PlayerSession, _ json.RawMessage) error {
	if err := h.svc.Confirm(ctx, s); err != nil {
		replyError(s, err.Error())
	}
	return nil
}

// HandleCancel cancels the current trade.
func (h *TradeHandlers) HandleCancel(_ context.Context, s *player.PlayerSession, _ json.RawMessage) error {
	h.svc.Cancel(s)
	return nil
}

// replyError sends a generic error packet back to the player.
func replyError(s *player.PlayerSession, msg string) {
	payload, _ := json.Marshal(map[string]string{"error": msg})
	s.Send(&player.Packet{Type: "error", Payload: payload})
}
