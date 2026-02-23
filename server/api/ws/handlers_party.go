package ws

import (
	"context"
	"encoding/json"

	"github.com/kasuganosora/rpgmakermvmmo/server/game/party"
	"github.com/kasuganosora/rpgmakermvmmo/server/game/player"
	"go.uber.org/zap"
)

// PartyHandlers handles party-related WebSocket messages.
type PartyHandlers struct {
	mgr    *party.Manager
	sm     *player.SessionManager
	logger *zap.Logger
}

// NewPartyHandlers creates PartyHandlers.
func NewPartyHandlers(mgr *party.Manager, sm *player.SessionManager, logger *zap.Logger) *PartyHandlers {
	return &PartyHandlers{mgr: mgr, sm: sm, logger: logger}
}

// RegisterHandlers registers party WS handlers.
func (h *PartyHandlers) RegisterHandlers(r *Router) {
	r.On("party_invite", h.HandleInvite)
	r.On("party_invite_response", h.HandleInviteResponse)
	r.On("party_leave", h.HandleLeave)
}

type partyInvitePayload struct {
	TargetCharID int64 `json:"target_char_id"`
}

// HandleInvite sends a party invite from the sender to the target.
func (h *PartyHandlers) HandleInvite(_ context.Context, s *player.PlayerSession, raw json.RawMessage) error {
	var req partyInvitePayload
	if err := json.Unmarshal(raw, &req); err != nil {
		return nil
	}
	target := h.sm.Get(req.TargetCharID)
	if target == nil {
		replyError(s, "target_offline")
		return nil
	}
	if err := h.mgr.InvitePlayer(s, target); err != nil {
		replyError(s, err.Error())
	}
	return nil
}

type partyInviteResponsePayload struct {
	Accept bool  `json:"accept"`
	FromID int64 `json:"from_id"`
}

// HandleInviteResponse handles the target's accept/decline of a party invite.
func (h *PartyHandlers) HandleInviteResponse(_ context.Context, s *player.PlayerSession, raw json.RawMessage) error {
	var req partyInviteResponsePayload
	if err := json.Unmarshal(raw, &req); err != nil {
		return nil
	}
	if !req.Accept {
		h.mgr.DeclineInvite(s.CharID)
		return nil
	}
	inviter := h.sm.Get(req.FromID)
	if inviter == nil {
		replyError(s, "inviter_offline")
		h.mgr.DeclineInvite(s.CharID)
		return nil
	}
	if err := h.mgr.AcceptInvite(s, inviter); err != nil {
		replyError(s, err.Error())
	}
	return nil
}

// HandleLeave removes the player from their party.
func (h *PartyHandlers) HandleLeave(_ context.Context, s *player.PlayerSession, _ json.RawMessage) error {
	h.mgr.LeaveParty(s)
	return nil
}
