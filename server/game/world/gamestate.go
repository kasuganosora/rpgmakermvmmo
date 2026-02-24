package world

import (
	"fmt"
	"sync"
	"time"

	"github.com/kasuganosora/rpgmakermvmmo/server/model"
	"go.uber.org/zap"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// selfSwitchKey uniquely identifies a self-switch for one event on one map.
type selfSwitchKey struct {
	MapID   int
	EventID int
	Ch      string // "A","B","C","D"
}

// pendingChange represents a pending database write.
type pendingChange struct {
	typ string // "switch", "variable", "selfswitch"
	id  interface{}
	val interface{}
}

// GameState holds server-authoritative game switches, variables, and self-switches.
// Switches and variables are global (shared across all maps) per RMMV convention.
// Self-switches are per (map, event, channel).
//
// State is persisted to the database: loaded on startup, saved with batching.
type GameState struct {
	mu           sync.RWMutex
	switches     map[int]bool
	variables    map[int]int
	selfSwitches map[selfSwitchKey]bool
	db           *gorm.DB // nil = no persistence (tests)
	logger       *zap.Logger

	// Batch persistence
	pending     map[string]pendingChange
	pendingMu   sync.Mutex
	flushTicker *time.Ticker
	stopCh      chan struct{}
}

// NewGameState creates an empty GameState with optional DB persistence.
func NewGameState(db *gorm.DB, logger *zap.Logger) *GameState {
	gs := &GameState{
		switches:     make(map[int]bool),
		variables:    make(map[int]int),
		selfSwitches: make(map[selfSwitchKey]bool),
		db:           db,
		logger:       logger,
		pending:      make(map[string]pendingChange),
		stopCh:       make(chan struct{}),
	}

	// Start background flusher if DB is available.
	if db != nil {
		gs.flushTicker = time.NewTicker(5 * time.Second)
		go gs.batchFlusher()
	}

	return gs
}

// Stop stops the background flusher and flushes remaining changes.
func (gs *GameState) Stop() {
	if gs.flushTicker != nil {
		gs.flushTicker.Stop()
		close(gs.stopCh)
		gs.Flush()
	}
}

// batchFlusher periodically flushes pending changes to the database.
func (gs *GameState) batchFlusher() {
	for {
		select {
		case <-gs.flushTicker.C:
			gs.Flush()
		case <-gs.stopCh:
			return
		}
	}
}

// Flush writes all pending changes to the database.
func (gs *GameState) Flush() {
	if gs.db == nil {
		return
	}

	gs.pendingMu.Lock()
	if len(gs.pending) == 0 {
		gs.pendingMu.Unlock()
		return
	}

	// Copy pending changes.
	changes := make([]pendingChange, 0, len(gs.pending))
	for _, ch := range gs.pending {
		changes = append(changes, ch)
	}
	gs.pending = make(map[string]pendingChange)
	gs.pendingMu.Unlock()

	// Batch write to database.
	err := gs.db.Transaction(func(tx *gorm.DB) error {
		for _, ch := range changes {
			switch ch.typ {
			case "switch":
				id := ch.id.(int)
				val := ch.val.(bool)
				if err := tx.Clauses(clause.OnConflict{
					DoUpdates: clause.AssignmentColumns([]string{"value"}),
				}).Create(&model.GameSwitch{SwitchID: id, Value: val}).Error; err != nil {
					return err
				}
			case "variable":
				id := ch.id.(int)
				val := ch.val.(int)
				if err := tx.Clauses(clause.OnConflict{
					DoUpdates: clause.AssignmentColumns([]string{"value"}),
				}).Create(&model.GameVariable{VariableID: id, Value: val}).Error; err != nil {
					return err
				}
			case "selfswitch":
				key := ch.id.(selfSwitchKey)
				val := ch.val.(bool)
				if err := tx.Clauses(clause.OnConflict{
					DoUpdates: clause.AssignmentColumns([]string{"value"}),
				}).Create(&model.GameSelfSwitch{MapID: key.MapID, EventID: key.EventID, Ch: key.Ch, Value: val}).Error; err != nil {
					return err
				}
			}
		}
		return nil
	})

	if err != nil && gs.logger != nil {
		gs.logger.Error("failed to flush game state", zap.Error(err))
	}
}

// queueChange adds a change to the pending queue.
func (gs *GameState) queueChange(key string, change pendingChange) {
	gs.pendingMu.Lock()
	gs.pending[key] = change
	gs.pendingMu.Unlock()
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

// SetSwitch sets the value of a global switch and queues it for persistence.
func (gs *GameState) SetSwitch(id int, val bool) {
	gs.mu.Lock()
	gs.switches[id] = val
	gs.mu.Unlock()

	gs.queueChange(fmt.Sprintf("sw:%d", id), pendingChange{
		typ: "switch",
		id:  id,
		val: val,
	})
}

// GetVariable returns the value of a global variable.
func (gs *GameState) GetVariable(id int) int {
	gs.mu.RLock()
	defer gs.mu.RUnlock()
	return gs.variables[id]
}

// SetVariable sets the value of a global variable and queues it for persistence.
func (gs *GameState) SetVariable(id int, val int) {
	gs.mu.Lock()
	gs.variables[id] = val
	gs.mu.Unlock()

	gs.queueChange(fmt.Sprintf("var:%d", id), pendingChange{
		typ: "variable",
		id:  id,
		val: val,
	})
}

// GetSelfSwitch returns the value of a self-switch for a specific event.
func (gs *GameState) GetSelfSwitch(mapID, eventID int, ch string) bool {
	gs.mu.RLock()
	defer gs.mu.RUnlock()
	return gs.selfSwitches[selfSwitchKey{MapID: mapID, EventID: eventID, Ch: ch}]
}

// SetSelfSwitch sets the value of a self-switch for a specific event and queues it for persistence.
func (gs *GameState) SetSelfSwitch(mapID, eventID int, ch string, val bool) {
	key := selfSwitchKey{MapID: mapID, EventID: eventID, Ch: ch}
	gs.mu.Lock()
	gs.selfSwitches[key] = val
	gs.mu.Unlock()

	gs.queueChange(fmt.Sprintf("ss:%d:%d:%s", mapID, eventID, ch), pendingChange{
		typ: "selfswitch",
		id:  key,
		val: val,
	})
}
