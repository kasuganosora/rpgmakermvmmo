package world

import (
	"sync"

	"github.com/kasuganosora/rpgmakermvmmo/server/model"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// ---------------------------------------------------------------------------
// GameStateReader — read-only interface used by selectPage / meetsConditions.
// Both *GameState and *CompositeGameState satisfy this.
// ---------------------------------------------------------------------------

// GameStateReader provides read access to switches, variables, and self-switches.
type GameStateReader interface {
	GetSwitch(id int) bool
	GetVariable(id int) int
	GetSelfSwitch(mapID, eventID int, ch string) bool
}

// ---------------------------------------------------------------------------
// GlobalWhitelist — IDs that remain in the shared global GameState.
// ---------------------------------------------------------------------------

// GlobalWhitelist defines which switch/variable IDs remain global.
// IDs not in these sets are per-player.
type GlobalWhitelist struct {
	Switches  map[int]bool
	Variables map[int]bool
	// Self-switches are always per-player (no whitelist).
}

// NewGlobalWhitelist creates an empty whitelist (all per-player by default).
func NewGlobalWhitelist() *GlobalWhitelist {
	return &GlobalWhitelist{
		Switches:  make(map[int]bool),
		Variables: make(map[int]bool),
	}
}

// IsSwitchGlobal returns true if the given switch ID is whitelisted as global.
func (w *GlobalWhitelist) IsSwitchGlobal(id int) bool {
	return w.Switches[id]
}

// IsVariableGlobal returns true if the given variable ID is whitelisted as global.
func (w *GlobalWhitelist) IsVariableGlobal(id int) bool {
	return w.Variables[id]
}

// ---------------------------------------------------------------------------
// PlayerGameState — per-character in-memory state.
// ---------------------------------------------------------------------------

// PlayerGameState holds in-memory per-player switches, variables, self-switches, and self-variables.
type PlayerGameState struct {
	mu            sync.RWMutex
	charID        int64
	switches      map[int]bool
	variables     map[int]int
	selfSwitches  map[selfSwitchKey]bool
	selfVariables map[selfVariableKey]int // TemplateEvent.js extension
	db            *gorm.DB                // nil = no persistence (tests)
}

// NewPlayerGameState creates an empty PlayerGameState.
func NewPlayerGameState(charID int64, db *gorm.DB) *PlayerGameState {
	return &PlayerGameState{
		charID:        charID,
		switches:      make(map[int]bool),
		variables:     make(map[int]int),
		selfSwitches:  make(map[selfSwitchKey]bool),
		selfVariables: make(map[selfVariableKey]int),
		db:            db,
	}
}

// LoadFromDB populates from the char_switches, char_variables, char_self_switches tables.
func (ps *PlayerGameState) LoadFromDB() error {
	if ps.db == nil {
		return nil
	}
	ps.mu.Lock()
	defer ps.mu.Unlock()

	var switches []model.CharSwitch
	if err := ps.db.Where("char_id = ?", ps.charID).Find(&switches).Error; err != nil {
		return err
	}
	for _, s := range switches {
		ps.switches[s.SwitchID] = s.Value
	}

	var vars []model.CharVariable
	if err := ps.db.Where("char_id = ?", ps.charID).Find(&vars).Error; err != nil {
		return err
	}
	for _, v := range vars {
		ps.variables[v.VariableID] = v.Value
	}

	var selfSwitches []model.CharSelfSwitch
	if err := ps.db.Where("char_id = ?", ps.charID).Find(&selfSwitches).Error; err != nil {
		return err
	}
	for _, ss := range selfSwitches {
		ps.selfSwitches[selfSwitchKey{MapID: ss.MapID, EventID: ss.EventID, Ch: ss.Ch}] = ss.Value
	}

	var selfVariables []model.CharSelfVariable
	if err := ps.db.Where("char_id = ?", ps.charID).Find(&selfVariables).Error; err != nil {
		return err
	}
	for _, sv := range selfVariables {
		ps.selfVariables[selfVariableKey{MapID: sv.MapID, EventID: sv.EventID, Index: sv.Index}] = sv.Value
	}

	return nil
}

// GetSwitch returns the value of a per-player switch.
func (ps *PlayerGameState) GetSwitch(id int) bool {
	ps.mu.RLock()
	defer ps.mu.RUnlock()
	return ps.switches[id]
}

// SetSwitch sets the value of a per-player switch and persists it.
func (ps *PlayerGameState) SetSwitch(id int, val bool) {
	ps.mu.Lock()
	ps.switches[id] = val
	ps.mu.Unlock()

	if ps.db != nil {
		ps.db.Clauses(clause.OnConflict{
			DoUpdates: clause.AssignmentColumns([]string{"value"}),
		}).Create(&model.CharSwitch{CharID: ps.charID, SwitchID: id, Value: val})
	}
}

// GetVariable returns the value of a per-player variable.
func (ps *PlayerGameState) GetVariable(id int) int {
	ps.mu.RLock()
	defer ps.mu.RUnlock()
	return ps.variables[id]
}

// SetVariable sets the value of a per-player variable and persists it.
func (ps *PlayerGameState) SetVariable(id int, val int) {
	ps.mu.Lock()
	ps.variables[id] = val
	ps.mu.Unlock()

	if ps.db != nil {
		ps.db.Clauses(clause.OnConflict{
			DoUpdates: clause.AssignmentColumns([]string{"value"}),
		}).Create(&model.CharVariable{CharID: ps.charID, VariableID: id, Value: val})
	}
}

// SwitchesSnapshot returns a copy of all set switches.
func (ps *PlayerGameState) SwitchesSnapshot() map[int]bool {
	ps.mu.RLock()
	defer ps.mu.RUnlock()
	cp := make(map[int]bool, len(ps.switches))
	for k, v := range ps.switches {
		cp[k] = v
	}
	return cp
}

// VariablesSnapshot returns a copy of all set variables.
func (ps *PlayerGameState) VariablesSnapshot() map[int]int {
	ps.mu.RLock()
	defer ps.mu.RUnlock()
	cp := make(map[int]int, len(ps.variables))
	for k, v := range ps.variables {
		cp[k] = v
	}
	return cp
}

// GetSelfSwitch returns the value of a per-player self-switch.
func (ps *PlayerGameState) GetSelfSwitch(mapID, eventID int, ch string) bool {
	ps.mu.RLock()
	defer ps.mu.RUnlock()
	return ps.selfSwitches[selfSwitchKey{MapID: mapID, EventID: eventID, Ch: ch}]
}

// SetSelfSwitch sets the value of a per-player self-switch and persists it.
func (ps *PlayerGameState) SetSelfSwitch(mapID, eventID int, ch string, val bool) {
	ps.mu.Lock()
	ps.selfSwitches[selfSwitchKey{MapID: mapID, EventID: eventID, Ch: ch}] = val
	ps.mu.Unlock()

	if ps.db != nil {
		ps.db.Clauses(clause.OnConflict{
			DoUpdates: clause.AssignmentColumns([]string{"value"}),
		}).Create(&model.CharSelfSwitch{CharID: ps.charID, MapID: mapID, EventID: eventID, Ch: ch, Value: val})
	}
}

// GetSelfVariable returns the value of a per-player self-variable.
// TemplateEvent.js extension: index can be any integer (13-17 for RandomPos).
func (ps *PlayerGameState) GetSelfVariable(mapID, eventID, index int) int {
	ps.mu.RLock()
	defer ps.mu.RUnlock()
	return ps.selfVariables[selfVariableKey{MapID: mapID, EventID: eventID, Index: index}]
}

// SetSelfVariable sets the value of a per-player self-variable and persists it.
// TemplateEvent.js extension: index can be any integer (13-17 for RandomPos).
func (ps *PlayerGameState) SetSelfVariable(mapID, eventID, index, val int) {
	ps.mu.Lock()
	ps.selfVariables[selfVariableKey{MapID: mapID, EventID: eventID, Index: index}] = val
	ps.mu.Unlock()

	if ps.db != nil {
		ps.db.Clauses(clause.OnConflict{
			DoUpdates: clause.AssignmentColumns([]string{"value"}),
		}).Create(&model.CharSelfVariable{CharID: ps.charID, MapID: mapID, EventID: eventID, Index: index, Value: val})
	}
}

// ---------------------------------------------------------------------------
// CompositeGameState — routes reads/writes to global or player based on whitelist.
// Implements both GameStateReader and npc.GameStateAccessor.
// ---------------------------------------------------------------------------

// CompositeGameState routes reads/writes to global or player state based on the whitelist.
type CompositeGameState struct {
	global    *GameState
	player    *PlayerGameState
	whitelist *GlobalWhitelist
}

// NewCompositeGameState creates a composite that routes based on the whitelist.
func NewCompositeGameState(global *GameState, player *PlayerGameState, whitelist *GlobalWhitelist) *CompositeGameState {
	return &CompositeGameState{
		global:    global,
		player:    player,
		whitelist: whitelist,
	}
}

func (c *CompositeGameState) GetSwitch(id int) bool {
	if c.whitelist.IsSwitchGlobal(id) {
		return c.global.GetSwitch(id)
	}
	return c.player.GetSwitch(id)
}

func (c *CompositeGameState) SetSwitch(id int, val bool) {
	if c.whitelist.IsSwitchGlobal(id) {
		c.global.SetSwitch(id, val)
		return
	}
	c.player.SetSwitch(id, val)
}

func (c *CompositeGameState) GetVariable(id int) int {
	if c.whitelist.IsVariableGlobal(id) {
		return c.global.GetVariable(id)
	}
	return c.player.GetVariable(id)
}

func (c *CompositeGameState) SetVariable(id int, val int) {
	if c.whitelist.IsVariableGlobal(id) {
		c.global.SetVariable(id, val)
		return
	}
	c.player.SetVariable(id, val)
}

// Self-switches are always per-player.
func (c *CompositeGameState) GetSelfSwitch(mapID, eventID int, ch string) bool {
	return c.player.GetSelfSwitch(mapID, eventID, ch)
}

func (c *CompositeGameState) SetSelfSwitch(mapID, eventID int, ch string, val bool) {
	c.player.SetSelfSwitch(mapID, eventID, ch, val)
}

// Self-variables are always per-player (TemplateEvent.js extension).
func (c *CompositeGameState) GetSelfVariable(mapID, eventID, index int) int {
	return c.player.GetSelfVariable(mapID, eventID, index)
}

func (c *CompositeGameState) SetSelfVariable(mapID, eventID, index, val int) {
	c.player.SetSelfVariable(mapID, eventID, index, val)
}

// ---------------------------------------------------------------------------
// PlayerStateManager — manages per-player state lifecycle.
// ---------------------------------------------------------------------------

// PlayerStateManager manages in-memory PlayerGameState instances for connected players.
type PlayerStateManager struct {
	mu        sync.RWMutex
	states    map[int64]*PlayerGameState // charID → state
	whitelist *GlobalWhitelist
	global    *GameState
	db        *gorm.DB
}

// NewPlayerStateManager creates a new PlayerStateManager.
func NewPlayerStateManager(global *GameState, whitelist *GlobalWhitelist, db *gorm.DB) *PlayerStateManager {
	return &PlayerStateManager{
		states:    make(map[int64]*PlayerGameState),
		whitelist: whitelist,
		global:    global,
		db:        db,
	}
}

// GetOrLoad returns the PlayerGameState for charID, loading from DB if not cached.
func (m *PlayerStateManager) GetOrLoad(charID int64) (*PlayerGameState, error) {
	m.mu.RLock()
	ps, ok := m.states[charID]
	m.mu.RUnlock()
	if ok {
		return ps, nil
	}

	m.mu.Lock()
	defer m.mu.Unlock()
	// Double-check after acquiring write lock.
	if ps, ok = m.states[charID]; ok {
		return ps, nil
	}

	ps = NewPlayerGameState(charID, m.db)
	if err := ps.LoadFromDB(); err != nil {
		return nil, err
	}
	m.states[charID] = ps
	return ps, nil
}

// GetComposite returns a CompositeGameState for the given charID.
func (m *PlayerStateManager) GetComposite(charID int64) (*CompositeGameState, error) {
	ps, err := m.GetOrLoad(charID)
	if err != nil {
		return nil, err
	}
	return NewCompositeGameState(m.global, ps, m.whitelist), nil
}

// Unload removes the cached state for a disconnected player.
func (m *PlayerStateManager) Unload(charID int64) {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.states, charID)
}
