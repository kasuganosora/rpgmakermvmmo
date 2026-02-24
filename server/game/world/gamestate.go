package world

import (
	"fmt"
	"strings"
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

// selfVariableKey uniquely identifies a self-variable for one event on one map.
// TemplateEvent.js extension: supports numeric indices (not just A/B/C/D).
type selfVariableKey struct {
	MapID   int
	EventID int
	Index   int // e.g., 13=X, 14=Y, 15=Dir, 16=Day, 17=Seed for RandomPos
}

// pendingChange represents a pending database write.
type pendingChange struct {
	typ string // "switch", "variable", "selfswitch"
	id  interface{}
	val interface{}
}

// VariableChangeCallback is called when a global variable changes.
type VariableChangeCallback func(variableID, value int)

// SwitchChangeCallback is called when a global switch changes.
type SwitchChangeCallback func(switchID int, value bool)

// GameState holds server-authoritative game switches, variables, and self-switches.
// Switches and variables are global (shared across all maps) per RMMV convention.
// Self-switches are per (map, event, channel).
// Self-variables are TemplateEvent.js extension supporting numeric indices.
//
// State is persisted to the database: loaded on startup, saved with batching.
type GameState struct {
	mu            sync.RWMutex
	switches      map[int]bool
	variables     map[int]int
	selfSwitches  map[selfSwitchKey]bool
	selfVariables map[selfVariableKey]int // TemplateEvent.js extension
	db            *gorm.DB                // nil = no persistence (tests)
	logger        *zap.Logger

	// Batch persistence
	pending     map[string]pendingChange
	pendingMu   sync.Mutex
	flushTicker *time.Ticker
	stopCh      chan struct{}

	// Change callbacks for broadcasting to clients (TemplateEvent.js hook)
	onVariableChange VariableChangeCallback
	onSwitchChange   SwitchChangeCallback
}

// NewGameState creates an empty GameState with optional DB persistence.
func NewGameState(db *gorm.DB, logger *zap.Logger) *GameState {
	gs := &GameState{
		switches:      make(map[int]bool),
		variables:     make(map[int]int),
		selfSwitches:  make(map[selfSwitchKey]bool),
		selfVariables: make(map[selfVariableKey]int),
		db:            db,
		logger:        logger,
		pending:       make(map[string]pendingChange),
		stopCh:        make(chan struct{}),
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
// Retries on SQLite busy errors.
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

	// Batch write to database with retry on busy error.
	var err error
	for attempt := 0; attempt < 3; attempt++ {
		err = gs.db.Transaction(func(tx *gorm.DB) error {
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
				case "selfvariable":
					key := ch.id.(selfVariableKey)
					val := ch.val.(int)
					if err := tx.Clauses(clause.OnConflict{
						DoUpdates: clause.AssignmentColumns([]string{"value"}),
					}).Create(&model.GameSelfVariable{MapID: key.MapID, EventID: key.EventID, Index: key.Index, Value: val}).Error; err != nil {
						return err
					}
				}
			}
			return nil
		})
		if err == nil {
			return // success
		}
		// Check if it's a busy error
		if isSQLiteBusy(err) && attempt < 2 {
			time.Sleep(time.Duration(attempt+1) * 50 * time.Millisecond)
			continue
		}
		break // non-busy error or last attempt
	}

	if err != nil && gs.logger != nil {
		gs.logger.Error("failed to flush game state", zap.Error(err))
	}
}

// isSQLiteBusy checks if the error is a SQLite busy/database locked error.
func isSQLiteBusy(err error) bool {
	if err == nil {
		return false
	}
	errStr := err.Error()
	return strings.Contains(errStr, "database is locked") ||
		strings.Contains(errStr, "busy") ||
		strings.Contains(errStr, "SQLITE_BUSY")
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

	// Load self-variables (TemplateEvent.js extension).
	var selfVariables []model.GameSelfVariable
	if err := gs.db.Find(&selfVariables).Error; err != nil {
		return err
	}
	for _, sv := range selfVariables {
		gs.selfVariables[selfVariableKey{MapID: sv.MapID, EventID: sv.EventID, Index: sv.Index}] = sv.Value
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
// Triggers onVariableChange callback if set (for broadcasting to clients).
func (gs *GameState) SetVariable(id int, val int) {
	gs.mu.Lock()
	gs.variables[id] = val
	onChange := gs.onVariableChange
	gs.mu.Unlock()

	gs.queueChange(fmt.Sprintf("var:%d", id), pendingChange{
		typ: "variable",
		id:  id,
		val: val,
	})

	// Trigger callback outside of lock
	if onChange != nil {
		onChange(id, val)
	}
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

// SetOnVariableChange sets the callback for variable changes.
func (gs *GameState) SetOnVariableChange(cb VariableChangeCallback) {
	gs.mu.Lock()
	defer gs.mu.Unlock()
	gs.onVariableChange = cb
}

// SetOnSwitchChange sets the callback for switch changes.
func (gs *GameState) SetOnSwitchChange(cb SwitchChangeCallback) {
	gs.mu.Lock()
	defer gs.mu.Unlock()
	gs.onSwitchChange = cb
}

// GetSelfVariable returns the value of a self-variable for a specific event.
// TemplateEvent.js extension: index can be any integer (13-17 for RandomPos).
func (gs *GameState) GetSelfVariable(mapID, eventID, index int) int {
	gs.mu.RLock()
	defer gs.mu.RUnlock()
	return gs.selfVariables[selfVariableKey{MapID: mapID, EventID: eventID, Index: index}]
}

// SetSelfVariable sets the value of a self-variable for a specific event and queues it for persistence.
// TemplateEvent.js extension: index can be any integer (13-17 for RandomPos).
func (gs *GameState) SetSelfVariable(mapID, eventID, index, val int) {
	key := selfVariableKey{MapID: mapID, EventID: eventID, Index: index}
	gs.mu.Lock()
	gs.selfVariables[key] = val
	gs.mu.Unlock()

	gs.queueChange(fmt.Sprintf("sv:%d:%d:%d", mapID, eventID, index), pendingChange{
		typ: "selfvariable",
		id:  key,
		val: val,
	})
}
