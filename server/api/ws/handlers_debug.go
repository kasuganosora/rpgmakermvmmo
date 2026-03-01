package ws

import (
	"context"
	"encoding/json"
	"strconv"

	"github.com/kasuganosora/rpgmakermvmmo/server/game/npc"
	"github.com/kasuganosora/rpgmakermvmmo/server/game/player"
	"github.com/kasuganosora/rpgmakermvmmo/server/game/world"
	"github.com/kasuganosora/rpgmakermvmmo/server/resource"
	"go.uber.org/zap"
	"gorm.io/gorm"
)

// DebugHandlers provides WS message handlers for test/debug mode.
// These allow direct manipulation of player state without going through
// normal game flows, enabling faster and more targeted integration tests.
type DebugHandlers struct {
	wm         *world.WorldManager
	sm         *player.SessionManager
	res        *resource.ResourceLoader
	db         *gorm.DB
	logger     *zap.Logger
	transferFn npc.TransferFunc
	battleFn   npc.BattleFunc
}

// NewDebugHandlers creates a DebugHandlers instance.
func NewDebugHandlers(wm *world.WorldManager, sm *player.SessionManager, res *resource.ResourceLoader, db *gorm.DB, logger *zap.Logger) *DebugHandlers {
	return &DebugHandlers{wm: wm, sm: sm, res: res, db: db, logger: logger}
}

// SetTransferFunc sets the callback used for cross-map teleport.
func (dh *DebugHandlers) SetTransferFunc(fn npc.TransferFunc) {
	dh.transferFn = fn
}

// SetBattleFn sets the callback used for debug_start_battle.
func (dh *DebugHandlers) SetBattleFn(fn npc.BattleFunc) {
	dh.battleFn = fn
}

// RegisterHandlers registers all debug handlers on the router.
func (dh *DebugHandlers) RegisterHandlers(r *Router) {
	r.On("debug_set_switch", dh.HandleSetSwitch)
	r.On("debug_set_variable", dh.HandleSetVariable)
	r.On("debug_teleport", dh.HandleTeleport)
	r.On("debug_set_stats", dh.HandleSetStats)
	r.On("debug_get_state", dh.HandleGetState)
	r.On("debug_trigger_ce", dh.HandleTriggerCE)
	r.On("debug_start_battle", dh.HandleStartBattle)
}

// ---- debug_set_switch ----

type debugSetSwitchReq struct {
	SwitchID int  `json:"switch_id"`
	Value    bool `json:"value"`
}

func (dh *DebugHandlers) HandleSetSwitch(_ context.Context, s *player.PlayerSession, raw json.RawMessage) error {
	var req debugSetSwitchReq
	if err := json.Unmarshal(raw, &req); err != nil {
		return nil
	}
	ps, err := dh.wm.PlayerStateManager().GetOrLoad(s.CharID)
	if err != nil {
		dh.logger.Error("debug_set_switch: failed to load state", zap.Error(err))
		return nil
	}
	ps.SetSwitch(req.SwitchID, req.Value)
	dh.sendDebugOK(s, "set_switch")
	return nil
}

// ---- debug_set_variable ----

type debugSetVariableReq struct {
	VariableID int `json:"variable_id"`
	Value      int `json:"value"`
}

func (dh *DebugHandlers) HandleSetVariable(_ context.Context, s *player.PlayerSession, raw json.RawMessage) error {
	var req debugSetVariableReq
	if err := json.Unmarshal(raw, &req); err != nil {
		return nil
	}
	ps, err := dh.wm.PlayerStateManager().GetOrLoad(s.CharID)
	if err != nil {
		dh.logger.Error("debug_set_variable: failed to load state", zap.Error(err))
		return nil
	}
	ps.SetVariable(req.VariableID, req.Value)
	dh.sendDebugOK(s, "set_variable")
	return nil
}

// ---- debug_teleport ----

type debugTeleportReq struct {
	MapID int `json:"map_id"`
	X     int `json:"x"`
	Y     int `json:"y"`
	Dir   int `json:"dir"`
}

func (dh *DebugHandlers) HandleTeleport(_ context.Context, s *player.PlayerSession, raw json.RawMessage) error {
	var req debugTeleportReq
	if err := json.Unmarshal(raw, &req); err != nil {
		return nil
	}
	if req.Dir <= 0 {
		req.Dir = 2
	}
	if req.MapID <= 0 {
		return nil
	}

	// Same-map teleport: just reposition.
	if req.MapID == s.MapID {
		s.SetPosition(req.X, req.Y, req.Dir)
		dh.sendDebugOK(s, "teleport")
		return nil
	}

	// Cross-map teleport: use the transfer function (triggers leave/enter/autorun).
	if dh.transferFn != nil {
		dh.transferFn(s, req.MapID, req.X, req.Y, req.Dir)
	}
	dh.sendDebugOK(s, "teleport")
	return nil
}

// ---- debug_set_stats ----

type debugSetStatsReq struct {
	HP    *int   `json:"hp"`
	MaxHP *int   `json:"max_hp"`
	MP    *int   `json:"mp"`
	MaxMP *int   `json:"max_mp"`
	Level *int   `json:"level"`
	Exp   *int64 `json:"exp"`
}

func (dh *DebugHandlers) HandleSetStats(_ context.Context, s *player.PlayerSession, raw json.RawMessage) error {
	var req debugSetStatsReq
	if err := json.Unmarshal(raw, &req); err != nil {
		return nil
	}
	hp, maxHP, mp, maxMP := s.Stats()
	if req.HP != nil {
		hp = *req.HP
	}
	if req.MaxHP != nil {
		maxHP = *req.MaxHP
	}
	if req.MP != nil {
		mp = *req.MP
	}
	if req.MaxMP != nil {
		maxMP = *req.MaxMP
	}
	s.SetStats(hp, maxHP, mp, maxMP)
	if req.Level != nil {
		s.Level = *req.Level
	}
	if req.Exp != nil {
		s.Exp = *req.Exp
	}
	dh.sendDebugOK(s, "set_stats")
	return nil
}

// ---- debug_get_state ----

func (dh *DebugHandlers) HandleGetState(_ context.Context, s *player.PlayerSession, _ json.RawMessage) error {
	x, y, dir := s.Position()
	hp, maxHP, mp, maxMP := s.Stats()

	result := map[string]interface{}{
		"char_id": s.CharID,
		"map_id":  s.MapID,
		"x":       x,
		"y":       y,
		"dir":     dir,
		"hp":      hp,
		"max_hp":  maxHP,
		"mp":      mp,
		"max_mp":  maxMP,
		"level":   s.Level,
		"exp":     s.Exp,
	}

	ps, err := dh.wm.PlayerStateManager().GetOrLoad(s.CharID)
	if err == nil {
		// Convert switch/variable maps to string-keyed maps for JSON.
		switches := ps.SwitchesSnapshot()
		swMap := make(map[string]bool, len(switches))
		for id, val := range switches {
			swMap[strconv.Itoa(id)] = val
		}
		variables := ps.VariablesSnapshot()
		varMap := make(map[string]int, len(variables))
		for id, val := range variables {
			varMap[strconv.Itoa(id)] = val
		}
		result["switches"] = swMap
		result["variables"] = varMap
	}

	payload, _ := json.Marshal(result)
	s.Send(&player.Packet{Type: "debug_state", Payload: payload})
	return nil
}

// ---- debug_trigger_ce ----

type debugTriggerCEReq struct {
	CEID int `json:"ce_id"`
}

func (dh *DebugHandlers) HandleTriggerCE(_ context.Context, s *player.PlayerSession, raw json.RawMessage) error {
	var req debugTriggerCEReq
	if err := json.Unmarshal(raw, &req); err != nil || req.CEID <= 0 {
		return nil
	}

	if dh.res == nil || req.CEID >= len(dh.res.CommonEvents) || dh.res.CommonEvents[req.CEID] == nil {
		dh.logger.Warn("debug_trigger_ce: CE not found", zap.Int("ce_id", req.CEID))
		return nil
	}

	ce := dh.res.CommonEvents[req.CEID]
	page := &resource.EventPage{
		Trigger: 3, // treat as autorun for executor
		List:    ce.List,
	}

	composite, err := dh.wm.PlayerStateManager().GetComposite(s.CharID)
	if err != nil {
		dh.logger.Error("debug_trigger_ce: failed to get composite state", zap.Error(err))
		return nil
	}

	executor := npc.New(dh.db, dh.res, dh.logger)
	opts := &npc.ExecuteOpts{
		GameState:  composite,
		MapID:      s.MapID,
		EventID:    0,
		TransferFn: dh.transferFn,
	}

	go func() {
		defer func() {
			if r := recover(); r != nil {
				dh.logger.Error("debug_trigger_ce: panic recovered",
					zap.Int("ce_id", req.CEID),
					zap.Int64("char_id", s.CharID),
					zap.Any("panic", r))
			}
		}()
		executor.Execute(context.Background(), s, page, opts)
		executor.SendStateSyncAfterExecution(context.Background(), s, opts)
	}()

	dh.sendDebugOK(s, "trigger_ce")
	return nil
}

// ---- debug_start_battle ----

type debugStartBattleReq struct {
	TroopID   int  `json:"troop_id"`
	CanEscape bool `json:"can_escape"`
	CanLose   bool `json:"can_lose"`
}

func (dh *DebugHandlers) HandleStartBattle(ctx context.Context, s *player.PlayerSession, raw json.RawMessage) error {
	if dh.battleFn == nil {
		dh.logger.Warn("debug_start_battle: no battleFn configured")
		return nil
	}
	var req debugStartBattleReq
	if err := json.Unmarshal(raw, &req); err != nil {
		return nil
	}
	if req.TroopID <= 0 {
		req.TroopID = 1
	}

	go func() {
		defer func() {
			if r := recover(); r != nil {
				dh.logger.Error("debug_start_battle: panic recovered",
					zap.Int("troop_id", req.TroopID),
					zap.Int64("char_id", s.CharID),
					zap.Any("panic", r))
			}
		}()
		dh.battleFn(ctx, s, req.TroopID, req.CanEscape, req.CanLose)
	}()

	dh.sendDebugOK(s, "start_battle")
	return nil
}

// ---- helpers ----

func (dh *DebugHandlers) sendDebugOK(s *player.PlayerSession, action string) {
	payload, _ := json.Marshal(map[string]interface{}{"action": action})
	s.Send(&player.Packet{Type: "debug_ok", Payload: payload})
}
