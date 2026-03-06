// instance.go — 副本地图（Instance Map）支持。
// 标记了 <instance> 的地图会为每个玩家/队伍创建独立的 MapRoom，
// NPC 状态干净，玩家互不可见。
package world

import (
	"strings"
	"sync/atomic"

	"github.com/kasuganosora/rpgmakermvmmo/server/game/player"
	"go.uber.org/zap"
)

// InstanceConfig describes instance behavior parsed from a map's Note field.
type InstanceConfig struct {
	Enabled bool // map is instanced
	Party   bool // party members share the same instance
	Save    bool // keep instance alive on disconnect (resume later)
}

// ParseInstanceConfig extracts instance settings from a map Note string.
// Supported tags:
//
//	<instance>            — solo instance, restart on reconnect (default)
//	<instance:party>      — party members share instance
//	<instance:save>       — save progress on disconnect
//	<instance:party:save> — party + save
func ParseInstanceConfig(note string) InstanceConfig {
	cfg := InstanceConfig{}
	// Quick check before scanning.
	if !strings.Contains(note, "<instance") {
		return cfg
	}
	for _, line := range strings.Split(note, "\n") {
		line = strings.TrimSpace(line)
		if !strings.HasPrefix(line, "<instance") {
			continue
		}
		// Strip < and >
		line = strings.TrimPrefix(line, "<")
		line = strings.TrimSuffix(line, ">")
		cfg.Enabled = true
		parts := strings.Split(line, ":")
		for _, p := range parts[1:] { // skip "instance"
			switch strings.TrimSpace(p) {
			case "party":
				cfg.Party = true
			case "save":
				cfg.Save = true
			}
		}
		break // only first <instance...> tag matters
	}
	return cfg
}

// instOwnerKey uniquely identifies an instance by map and owner.
type instOwnerKey struct {
	MapID   int
	OwnerID int64 // charID (solo) or party leader charID (party)
}

var globalInstIDCounter int64

func nextInstanceID() int64 {
	return atomic.AddInt64(&globalInstIDCounter, 1)
}

// GetOrCreateInstance returns (or creates) an instance room for the given map and owner.
// ownerID is the player's charID (solo) or party leader's charID (party).
// Returns the MapRoom and its unique instance ID.
func (wm *WorldManager) GetOrCreateInstance(mapID int, ownerID int64) (*MapRoom, int64) {
	key := instOwnerKey{MapID: mapID, OwnerID: ownerID}

	wm.mu.Lock()
	defer wm.mu.Unlock()

	// Check existing instance.
	if instID, ok := wm.instByOwner[key]; ok {
		if room, ok2 := wm.instances[instID]; ok2 {
			return room, instID
		}
		// Stale entry — clean up.
		delete(wm.instByOwner, key)
	}

	// Create new instance room.
	instID := nextInstanceID()
	room := newMapRoom(mapID, wm.res, wm.state, wm.logger)
	room.IsInstance = true
	room.InstanceID = instID
	room.OwnerID = ownerID
	wm.instances[instID] = room
	wm.instByOwner[key] = instID
	go room.Run()

	wm.logger.Info("instance room created",
		zap.Int("map_id", mapID),
		zap.Int64("instance_id", instID),
		zap.Int64("owner_id", ownerID))

	return room, instID
}

// GetInstance returns the instance room for the given ID, or nil.
func (wm *WorldManager) GetInstance(instID int64) *MapRoom {
	wm.mu.RLock()
	defer wm.mu.RUnlock()
	return wm.instances[instID]
}

// DestroyInstance stops and removes an instance room.
// Only destroys if the room has no players remaining.
func (wm *WorldManager) DestroyInstance(instID int64) {
	wm.mu.Lock()
	room, ok := wm.instances[instID]
	if !ok {
		wm.mu.Unlock()
		return
	}
	// Don't destroy if players are still inside (party instance).
	if room.PlayerCount() > 0 {
		wm.mu.Unlock()
		return
	}
	delete(wm.instances, instID)
	// Clean up owner mapping.
	for k, v := range wm.instByOwner {
		if v == instID {
			delete(wm.instByOwner, k)
			break
		}
	}
	wm.mu.Unlock()
	room.Stop()
	wm.logger.Info("instance room destroyed",
		zap.Int("map_id", room.MapID),
		zap.Int64("instance_id", instID))
}

// GetPlayerRoom returns the correct MapRoom for a player — instance or shared.
func (wm *WorldManager) GetPlayerRoom(s *player.PlayerSession) *MapRoom {
	if s.InstanceID > 0 {
		wm.mu.RLock()
		room := wm.instances[s.InstanceID]
		wm.mu.RUnlock()
		if room != nil {
			return room
		}
		// Instance gone — fall back to shared room.
	}
	wm.mu.RLock()
	defer wm.mu.RUnlock()
	return wm.rooms[s.MapID]
}
