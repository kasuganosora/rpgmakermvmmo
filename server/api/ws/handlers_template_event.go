package ws

import (
	"context"
	"encoding/json"

	"github.com/kasuganosora/rpgmakermvmmo/server/game/player"
	"github.com/kasuganosora/rpgmakermvmmo/server/game/world"
	"github.com/kasuganosora/rpgmakermvmmo/server/resource"
	"go.uber.org/zap"
	"gorm.io/gorm"
)

// TemplateEventHandlers handles TemplateEvent.js related WebSocket messages.
// This provides server-side storage for self-variables (TemplateEvent.js extension)
// and variable/switch synchronization between client and server.
type TemplateEventHandlers struct {
	db     *gorm.DB
	wm     *world.WorldManager
	sm     *player.SessionManager
	res    *resource.ResourceLoader
	logger *zap.Logger

	// Cached whitelist sets (built once from MMOConfig).
	broadcastVarSet    map[int]bool
	broadcastSwitchSet map[int]bool
}

// NewTemplateEventHandlers creates TemplateEventHandlers.
func NewTemplateEventHandlers(db *gorm.DB, wm *world.WorldManager, sm *player.SessionManager, res *resource.ResourceLoader, logger *zap.Logger) *TemplateEventHandlers {
	h := &TemplateEventHandlers{
		db:     db,
		wm:     wm,
		sm:     sm,
		res:    res,
		logger: logger,
	}
	if res != nil && res.MMOConfig != nil {
		h.broadcastVarSet = res.MMOConfig.BroadcastVarSet()
		h.broadcastSwitchSet = res.MMOConfig.BroadcastSwitchSet()
	}
	return h
}

// RegisterHandlers registers TemplateEvent-related WS handlers on the router.
// Also sets up change callbacks for broadcasting whitelist variable/switch changes.
func (h *TemplateEventHandlers) RegisterHandlers(r *Router) {
	// Self-variable operations (TemplateEvent.js extension)
	r.On("self_var_get", h.HandleSelfVarGet)
	r.On("self_var_set", h.HandleSelfVarSet)
	r.On("self_var_set_batch", h.HandleSelfVarSetBatch)

	// Variable operations (for whitelist synchronization)
	r.On("var_get", h.HandleVarGet)
	r.On("var_set", h.HandleVarSet)

	// Switch operations (for whitelist synchronization)
	r.On("switch_get", h.HandleSwitchGet)
	r.On("switch_set", h.HandleSwitchSet)

	// Set up change callbacks to broadcast whitelist changes to all clients
	h.setupChangeCallbacks()
}

// setupChangeCallbacks registers callbacks on GameState to broadcast changes.
func (h *TemplateEventHandlers) setupChangeCallbacks() {
	gs := h.wm.GameState()

	// Broadcast variable changes to all clients (whitelist from MMOConfig)
	gs.SetOnVariableChange(func(variableID, value int) {
		if !h.broadcastVarSet[variableID] {
			return
		}
		payload, _ := json.Marshal(map[string]interface{}{
			"variable_id": variableID,
			"value":       value,
		})
		pkt, _ := json.Marshal(&player.Packet{Type: "var_change", Payload: payload})
		h.sm.BroadcastAll(pkt)
	})

	// Broadcast switch changes to all clients (whitelist from MMOConfig)
	gs.SetOnSwitchChange(func(switchID int, value bool) {
		if !h.broadcastSwitchSet[switchID] {
			return
		}
		payload, _ := json.Marshal(map[string]interface{}{
			"switch_id": switchID,
			"value":     value,
		})
		pkt, _ := json.Marshal(&player.Packet{Type: "switch_change", Payload: payload})
		h.sm.BroadcastAll(pkt)
	})
}

// ========================================================================
// Self-Variable Messages (TemplateEvent.js extension)
// ========================================================================

// selfVarGetRequest is the WS payload for self_var_get.
type selfVarGetRequest struct {
	MapID   int `json:"map_id"`
	EventID int `json:"event_id"`
	Index   int `json:"index"` // e.g., 13=X, 14=Y, 15=Dir, 16=Day, 17=Seed
}

// selfVarGetResponse is the WS response for self_var_get.
type selfVarGetResponse struct {
	MapID   int `json:"map_id"`
	EventID int `json:"event_id"`
	Index   int `json:"index"`
	Value   int `json:"value"`
}

// HandleSelfVarGet handles requests for self-variable values.
// TemplateEvent.js extension: reads from per-player state.
func (h *TemplateEventHandlers) HandleSelfVarGet(ctx context.Context, s *player.PlayerSession, raw json.RawMessage) error {
	var req selfVarGetRequest
	if err := json.Unmarshal(raw, &req); err != nil {
		return nil
	}

	// Get player's composite state
	composite, err := h.wm.PlayerStateManager().GetComposite(s.CharID)
	if err != nil {
		h.logger.Error("failed to get player state for self_var_get", zap.Error(err), zap.Int64("char_id", s.CharID))
		return nil
	}

	value := composite.GetSelfVariable(req.MapID, req.EventID, req.Index)

	// Send response
	reply := selfVarGetResponse{
		MapID:   req.MapID,
		EventID: req.EventID,
		Index:   req.Index,
		Value:   value,
	}
	replyData, _ := json.Marshal(reply)
	s.Send(&player.Packet{Type: "self_var_get_reply", Payload: replyData})
	return nil
}

// selfVarSetRequest is the WS payload for self_var_set.
type selfVarSetRequest struct {
	MapID   int `json:"map_id"`
	EventID int `json:"event_id"`
	Index   int `json:"index"`
	Value   int `json:"value"`
}

// HandleSelfVarSet handles updates to self-variable values.
// TemplateEvent.js extension: writes to per-player state.
func (h *TemplateEventHandlers) HandleSelfVarSet(ctx context.Context, s *player.PlayerSession, raw json.RawMessage) error {
	var req selfVarSetRequest
	if err := json.Unmarshal(raw, &req); err != nil {
		return nil
	}

	// Get player's composite state
	composite, err := h.wm.PlayerStateManager().GetComposite(s.CharID)
	if err != nil {
		h.logger.Error("failed to get player state for self_var_set", zap.Error(err), zap.Int64("char_id", s.CharID))
		return nil
	}

	// Set the value
	composite.SetSelfVariable(req.MapID, req.EventID, req.Index, req.Value)

	h.logger.Debug("self_var_set",
		zap.Int64("char_id", s.CharID),
		zap.Int("map_id", req.MapID),
		zap.Int("event_id", req.EventID),
		zap.Int("index", req.Index),
		zap.Int("value", req.Value))

	return nil
}

// selfVarSetBatchRequest is the WS payload for self_var_set_batch.
type selfVarSetBatchRequest struct {
	Changes []selfVarSetRequest `json:"changes"`
}

// HandleSelfVarSetBatch handles batch updates to self-variable values.
// Client mmo-template-event-hook.js sends batched changes for efficiency.
func (h *TemplateEventHandlers) HandleSelfVarSetBatch(ctx context.Context, s *player.PlayerSession, raw json.RawMessage) error {
	var req selfVarSetBatchRequest
	if err := json.Unmarshal(raw, &req); err != nil {
		return nil
	}

	if len(req.Changes) == 0 {
		return nil
	}

	composite, err := h.wm.PlayerStateManager().GetComposite(s.CharID)
	if err != nil {
		h.logger.Error("failed to get player state for self_var_set_batch", zap.Error(err), zap.Int64("char_id", s.CharID))
		return nil
	}

	for _, change := range req.Changes {
		composite.SetSelfVariable(change.MapID, change.EventID, change.Index, change.Value)
	}

	h.logger.Debug("self_var_set_batch",
		zap.Int64("char_id", s.CharID),
		zap.Int("count", len(req.Changes)))

	return nil
}

// ========================================================================
// Variable Messages (for global whitelist synchronization)
// ========================================================================

// varGetRequest is the WS payload for var_get.
type varGetRequest struct {
	VariableID int `json:"variable_id"`
}

// varGetResponse is the WS response for var_get.
type varGetResponse struct {
	VariableID int `json:"variable_id"`
	Value      int `json:"value"`
}

// HandleVarGet handles requests for variable values.
// Returns the value from global state (for whitelist variables).
func (h *TemplateEventHandlers) HandleVarGet(ctx context.Context, s *player.PlayerSession, raw json.RawMessage) error {
	var req varGetRequest
	if err := json.Unmarshal(raw, &req); err != nil {
		return nil
	}

	// Get from global game state
	gs := h.wm.GameState()
	value := gs.GetVariable(req.VariableID)

	reply := varGetResponse{
		VariableID: req.VariableID,
		Value:      value,
	}
	replyData, _ := json.Marshal(reply)
	s.Send(&player.Packet{Type: "var_get_reply", Payload: replyData})
	return nil
}

// varSetRequest is the WS payload for var_set.
type varSetRequest struct {
	VariableID int `json:"variable_id"`
	Value      int `json:"value"`
}

// HandleVarSet handles updates to variable values.
// Updates global state (for whitelist variables).
func (h *TemplateEventHandlers) HandleVarSet(ctx context.Context, s *player.PlayerSession, raw json.RawMessage) error {
	var req varSetRequest
	if err := json.Unmarshal(raw, &req); err != nil {
		return nil
	}

	// Set in global game state
	gs := h.wm.GameState()
	gs.SetVariable(req.VariableID, req.Value)

	h.logger.Debug("var_set",
		zap.Int64("char_id", s.CharID),
		zap.Int("variable_id", req.VariableID),
		zap.Int("value", req.Value))

	return nil
}

// ========================================================================
// Switch Messages (for global whitelist synchronization)
// ========================================================================

// switchGetRequest is the WS payload for switch_get.
type switchGetRequest struct {
	SwitchID int `json:"switch_id"`
}

// switchGetResponse is the WS response for switch_get.
type switchGetResponse struct {
	SwitchID int  `json:"switch_id"`
	Value    bool `json:"value"`
}

// HandleSwitchGet handles requests for switch values.
// Returns the value from global state.
func (h *TemplateEventHandlers) HandleSwitchGet(ctx context.Context, s *player.PlayerSession, raw json.RawMessage) error {
	var req switchGetRequest
	if err := json.Unmarshal(raw, &req); err != nil {
		return nil
	}

	// Get from global game state
	gs := h.wm.GameState()
	value := gs.GetSwitch(req.SwitchID)

	reply := switchGetResponse{
		SwitchID: req.SwitchID,
		Value:    value,
	}
	replyData, _ := json.Marshal(reply)
	s.Send(&player.Packet{Type: "switch_get_reply", Payload: replyData})
	return nil
}

// switchSetRequest is the WS payload for switch_set.
type switchSetRequest struct {
	SwitchID int  `json:"switch_id"`
	Value    bool `json:"value"`
}

// HandleSwitchSet handles updates to switch values.
// Updates global state.
func (h *TemplateEventHandlers) HandleSwitchSet(ctx context.Context, s *player.PlayerSession, raw json.RawMessage) error {
	var req switchSetRequest
	if err := json.Unmarshal(raw, &req); err != nil {
		return nil
	}

	// Set in global game state
	gs := h.wm.GameState()
	gs.SetSwitch(req.SwitchID, req.Value)

	h.logger.Debug("switch_set",
		zap.Int64("char_id", s.CharID),
		zap.Int("switch_id", req.SwitchID),
		zap.Bool("value", req.Value))

	return nil
}
