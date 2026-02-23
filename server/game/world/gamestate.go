package world

import (
	"sync"

	"github.com/kasuganosora/rpgmakermvmmo/server/model"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// selfSwitchKey uniquely identifies a self-switch for one event on one map.
type selfSwitchKey struct {
	MapID   int
	EventID int
	Ch      string // "A","B","C","D"
}

// GameState holds server-authoritative game switches, variables, and self-switches.
// Switches and variables are global (shared across all maps) per RMMV convention.
// Self-switches are per (map, event, channel).
//
// State is persisted to the database: loaded on startup, saved on every change.
type GameState struct {
	mu           sync.RWMutex
	switches     map[int]bool
	variables    map[int]int
	selfSwitches map[selfSwitchKey]bool
	db           *gorm.DB // nil = no persistence (tests)
}

// NewGameState creates an empty GameState with optional DB persistence.
func NewGameState(db *gorm.DB) *GameState {
	return &GameState{
		switches:     make(map[int]bool),
		variables:    make(map[int]int),
		selfSwitches: make(map[selfSwitchKey]bool),
		db:           db,
	}
}

// LoadFromDB populates the in-memory state from the database.
// Call once at server startup after NewGameState.
func (gs *GameState) LoadFromDB() error {
	if gs.db == nil {
		return nil
	}
	gs.mu.Lock()
	defer gs.mu.Unlock()

	// Load switches.
	var switches []model.GameSwitch
	if err := gs.db.Find(&switches).Error; err != nil {
		return err
	}
	for _, s := range switches {
		gs.switches[s.SwitchID] = s.Value
	}

	// Load variables.
	var vars []model.GameVariable
	if err := gs.db.Find(&vars).Error; err != nil {
		return err
	}
	for _, v := range vars {
		gs.variables[v.VariableID] = v.Value
	}

	// Load self-switches.
	var selfSwitches []model.GameSelfSwitch
	if err := gs.db.Find(&selfSwitches).Error; err != nil {
		return err
	}
	for _, ss := range selfSwitches {
		gs.selfSwitches[selfSwitchKey{MapID: ss.MapID, EventID: ss.EventID, Ch: ss.Ch}] = ss.Value
	}

	return nil
}

// GetSwitch returns the value of a global switch.
func (gs *GameState) GetSwitch(id int) bool {
	gs.mu.RLock()
	defer gs.mu.RUnlock()
	return gs.switches[id]
}

// SetSwitch sets the value of a global switch and persists it.
func (gs *GameState) SetSwitch(id int, val bool) {
	gs.mu.Lock()
	gs.switches[id] = val
	gs.mu.Unlock()

	if gs.db != nil {
		gs.db.Clauses(clause.OnConflict{
			DoUpdates: clause.AssignmentColumns([]string{"value"}),
		}).Create(&model.GameSwitch{SwitchID: id, Value: val})
	}
}

// GetVariable returns the value of a global variable.
func (gs *GameState) GetVariable(id int) int {
	gs.mu.RLock()
	defer gs.mu.RUnlock()
	return gs.variables[id]
}

// SetVariable sets the value of a global variable and persists it.
func (gs *GameState) SetVariable(id int, val int) {
	gs.mu.Lock()
	gs.variables[id] = val
	gs.mu.Unlock()

	if gs.db != nil {
		gs.db.Clauses(clause.OnConflict{
			DoUpdates: clause.AssignmentColumns([]string{"value"}),
		}).Create(&model.GameVariable{VariableID: id, Value: val})
	}
}

// GetSelfSwitch returns the value of a self-switch for a specific event.
func (gs *GameState) GetSelfSwitch(mapID, eventID int, ch string) bool {
	gs.mu.RLock()
	defer gs.mu.RUnlock()
	return gs.selfSwitches[selfSwitchKey{MapID: mapID, EventID: eventID, Ch: ch}]
}

// SetSelfSwitch sets the value of a self-switch for a specific event and persists it.
func (gs *GameState) SetSelfSwitch(mapID, eventID int, ch string, val bool) {
	gs.mu.Lock()
	gs.selfSwitches[selfSwitchKey{MapID: mapID, EventID: eventID, Ch: ch}] = val
	gs.mu.Unlock()

	if gs.db != nil {
		gs.db.Clauses(clause.OnConflict{
			DoUpdates: clause.AssignmentColumns([]string{"value"}),
		}).Create(&model.GameSelfSwitch{MapID: mapID, EventID: eventID, Ch: ch, Value: val})
	}
}
