package ws

import (
	"context"
	"encoding/json"

	"github.com/kasuganosora/rpgmakermvmmo/server/game/player"
	"github.com/kasuganosora/rpgmakermvmmo/server/game/world"
	"go.uber.org/zap"
	"gorm.io/gorm"
)

// Whitelist variables that should be broadcast to all clients when changed.
// These are typically time/weather related variables.
var whitelistVariableIDs = map[int]bool{
	202: true, // ターン (day counter)
	203: true, // 曜日 (day of week)
	204: true, // 時間(h)
	205: true, // 時間(m)
	206: true, // 時間帯 (time period)
	207: true, // 天候 (weather)
	211: true, // 時間表示 (formatted time display)
}

// Whitelist switches that should be broadcast to all clients when changed.
var whitelistSwitchIDs = map[int]bool{
	11:  true, // 時間切り替えフラグ (time switch flag)
	12:  true, // 時計オン (clock on)
	20:  true, // リアルタイム処理 (real-time processing)
	31:  true, // その場で時間経過 (time progression on spot)
	53:  true, // 日照 (sunlight)
	54:  true, // 日没 (sunset)
	55:  true, // 天候・雨 (rain)
	56:  true, // 天候・雲 (clouds)
	57:  true, // 天候・陽光 (sunny)
	58:  true, // 天候・瘴気 (miasma)
	87:  true, // 日数経過開始 (day count start)
	89:  true, // 平日 (weekday)
	103: true, // 時間経過オンオフ (time passage toggle)
	104: true, // 時間経過呼び出し (time passage call)
}

// TemplateEventHandlers handles TemplateEvent.js related WebSocket messages.
// This provides server-side storage for self-variables (TemplateEvent.js extension)
// and variable/switch synchronization between client and server.
type TemplateEventHandlers struct {
	db *gorm.DB
	wm *world.WorldManager
	sm *player.SessionManager
	logger *zap.Logger
}

// NewTemplateEventHandlers creates TemplateEventHandlers.
func NewTemplateEventHandlers(db *gorm.DB, wm *world.WorldManager, sm *player.SessionManager, logger *zap.Logger) *TemplateEventHandlers {
	return &TemplateEventHandlers{
		db:     db,
		wm:     wm,
		sm:     sm,
		logger: logger,
	}
}

// RegisterHandlers registers TemplateEvent-related WS handlers on the router.
// Also sets up change callbacks for broadcasting whitelist variable/switch changes.
func (h *TemplateEventHandlers) RegisterHandlers(r *Router) {
	// Self-variable operations (TemplateEvent.js extension)
	r.On("self_var_get", h.HandleSelfVarGet)
	r.On("self_var_set", h.HandleSelfVarSet)

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

	// Broadcast variable changes to all clients
	gs.SetOnVariableChange(func(variableID, value int) {
		if !whitelistVariableIDs[variableID] {
			return
		}
		payload, _ := json.Marshal(map[string]interface{}{
			"variable_id": variableID,
			"value":       value,
		})
		pkt, _ := json.Marshal(&player.Packet{Type: "var_change", Payload: payload})
		h.sm.BroadcastAll(pkt)
	})

	// Broadcast switch changes to all clients
	gs.SetOnSwitchChange(func(switchID int, value bool) {
		if !whitelistSwitchIDs[switchID] {
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
