package world

import (
	"sync"
	"sync/atomic"

	"github.com/kasuganosora/rpgmakermvmmo/server/resource"
	"go.uber.org/zap"
	"gorm.io/gorm"
)

// WorldManager manages all active MapRoom instances.
type WorldManager struct {
	mu          sync.RWMutex
	rooms       map[int]*MapRoom            // shared rooms (mapID → room)
	instances   map[int64]*MapRoom          // instance rooms (instanceID → room)
	instByOwner map[instOwnerKey]int64      // (mapID, ownerID) → instanceID
	res         *resource.ResourceLoader
	state       *GameState
	psm         *PlayerStateManager
	whitelist   *GlobalWhitelist
	logger      *zap.Logger
}

// NewWorldManager creates a new WorldManager.
func NewWorldManager(res *resource.ResourceLoader, state *GameState, wl *GlobalWhitelist, db *gorm.DB, logger *zap.Logger) *WorldManager {
	psm := NewPlayerStateManager(state, wl, db)
	return &WorldManager{
		rooms:       make(map[int]*MapRoom),
		instances:   make(map[int64]*MapRoom),
		instByOwner: make(map[instOwnerKey]int64),
		res:         res,
		state:       state,
		psm:         psm,
		whitelist:   wl,
		logger:      logger,
	}
}

// GameState returns the global game state.
func (wm *WorldManager) GameState() *GameState { return wm.state }

// PlayerStateManager returns the per-player state manager.
func (wm *WorldManager) PlayerStateManager() *PlayerStateManager { return wm.psm }

// GetOrCreate returns the MapRoom for mapID, creating and starting it if needed.
func (wm *WorldManager) GetOrCreate(mapID int) *MapRoom {
	// Fast path: room already exists.
	wm.mu.RLock()
	room, ok := wm.rooms[mapID]
	wm.mu.RUnlock()
	if ok {
		return room
	}

	// Slow path: create a new room.
	wm.mu.Lock()
	defer wm.mu.Unlock()
	// Double-check after acquiring write lock.
	if room, ok = wm.rooms[mapID]; ok {
		return room
	}
	room = newMapRoom(mapID, wm.res, wm.state, wm.logger)
	wm.rooms[mapID] = room
	go room.Run()
	// Set up monster spawners from MMOConfig if configured.
	wm.setupSpawners(room, mapID)
	wm.logger.Info("map room created", zap.Int("map_id", mapID))
	return room
}

// setupSpawners creates a Spawner for the room from MMOConfig.MonsterSpawns.
func (wm *WorldManager) setupSpawners(room *MapRoom, mapID int) {
	if wm.res == nil || wm.res.MMOConfig == nil {
		return
	}
	var configs []SpawnConfig
	for _, sc := range wm.res.MMOConfig.MonsterSpawns {
		if sc.MapID != mapID {
			continue
		}
		configs = append(configs, SpawnConfig{
			MapID:      sc.MapID,
			MonsterID:  sc.EnemyID,
			X:          sc.X,
			Y:          sc.Y,
			MaxCount:   sc.MaxCount,
			RespawnSec: sc.RespawnSec,
			AIOverride: sc.AIOverride,
		})
	}
	if len(configs) == 0 {
		return
	}
	sp := NewSpawner(room, wm.res, configs, wm.logger)
	sp.SpawnAll()
	wm.logger.Info("monster spawner initialized",
		zap.Int("map_id", mapID), zap.Int("spawn_groups", len(configs)))
}

// Get returns the MapRoom for mapID, or nil if it does not exist.
func (wm *WorldManager) Get(mapID int) *MapRoom {
	wm.mu.RLock()
	defer wm.mu.RUnlock()
	return wm.rooms[mapID]
}

// Destroy stops and removes the MapRoom for mapID (used when the last player leaves).
func (wm *WorldManager) Destroy(mapID int) {
	wm.mu.Lock()
	room, ok := wm.rooms[mapID]
	if ok {
		delete(wm.rooms, mapID)
	}
	wm.mu.Unlock()
	if ok {
		room.Stop()
		wm.logger.Info("map room destroyed", zap.Int("map_id", mapID))
	}
}

// ActiveRoomCount returns the number of active shared map rooms.
func (wm *WorldManager) ActiveRoomCount() int {
	wm.mu.RLock()
	defer wm.mu.RUnlock()
	return len(wm.rooms)
}

// ActiveInstanceCount returns the number of active instance rooms.
func (wm *WorldManager) ActiveInstanceCount() int {
	wm.mu.RLock()
	defer wm.mu.RUnlock()
	return len(wm.instances)
}

// TotalBroadcastDrops returns the sum of dropped broadcast packets across all rooms.
func (wm *WorldManager) TotalBroadcastDrops() int64 {
	wm.mu.RLock()
	defer wm.mu.RUnlock()
	var total int64
	for _, r := range wm.rooms {
		total += atomic.LoadInt64(&r.BroadcastDrops)
	}
	for _, r := range wm.instances {
		total += atomic.LoadInt64(&r.BroadcastDrops)
	}
	return total
}

// StopAll stops all active map rooms and instances (used at server shutdown).
func (wm *WorldManager) StopAll() {
	wm.mu.Lock()
	rooms := make([]*MapRoom, 0, len(wm.rooms)+len(wm.instances))
	for _, r := range wm.rooms {
		rooms = append(rooms, r)
	}
	for _, r := range wm.instances {
		rooms = append(rooms, r)
	}
	wm.rooms = make(map[int]*MapRoom)
	wm.instances = make(map[int64]*MapRoom)
	wm.instByOwner = make(map[instOwnerKey]int64)
	wm.mu.Unlock()
	for _, r := range rooms {
		r.Stop()
	}
}
