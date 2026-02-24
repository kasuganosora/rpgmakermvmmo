package world

import (
	"encoding/json"
	"sync"
	"time"

	"github.com/kasuganosora/rpgmakermvmmo/server/game/player"
	"github.com/kasuganosora/rpgmakermvmmo/server/resource"
	"go.uber.org/zap"
)

const tickInterval = 50 * time.Millisecond // 20 TPS

// MonsterInstance is the client-visible monster state for map_init payloads.
type MonsterInstance struct {
	ID       int64  `json:"id"`
	EnemyID  int    `json:"enemy_id"`
	Name     string `json:"name"`
	X        int    `json:"x"`
	Y        int    `json:"y"`
	HP       int    `json:"hp"`
	MaxHP    int    `json:"max_hp"`
}

// DropRuntimeEntry is a live drop on the map floor.
type DropRuntimeEntry struct {
	DropID   int64
	ItemType int
	ItemID   int
	X, Y     int
	ExpireAt time.Time
	claimed  bool
}

// MapRoom manages a single map instance with its own game loop.
type MapRoom struct {
	MapID           int
	mapWidth        int // map width in tiles (for NPC bounds when passMap is nil)
	mapHeight       int // map height in tiles
	players         map[int64]*player.PlayerSession
	npcs            []*NPCRuntime
	monsters        []*MonsterInstance           // client-visible snapshot list
	runtimeMonsters map[int64]*MonsterRuntime    // instID → runtime state
	drops           map[int64]*DropRuntimeEntry
	res             *resource.ResourceLoader
	state           *GameState
	passMap         *resource.PassabilityMap
	broadcastQ      chan []byte
	mu              sync.RWMutex
	stopCh          chan struct{}
	logger          *zap.Logger
}

// newMapRoom creates a MapRoom but does not start the game loop.
func newMapRoom(mapID int, res *resource.ResourceLoader, state *GameState, logger *zap.Logger) *MapRoom {
	room := &MapRoom{
		MapID:           mapID,
		players:         make(map[int64]*player.PlayerSession),
		npcs:            []*NPCRuntime{},
		monsters:        []*MonsterInstance{},
		runtimeMonsters: make(map[int64]*MonsterRuntime),
		drops:           make(map[int64]*DropRuntimeEntry),
		res:             res,
		state:           state,
		broadcastQ:      make(chan []byte, 512),
		stopCh:          make(chan struct{}),
		logger:          logger,
	}
	if res != nil {
		room.passMap = res.Passability[mapID]
		if md, ok := res.Maps[mapID]; ok {
			room.mapWidth = md.Width
			room.mapHeight = md.Height
		}
		room.populateNPCs()
	}
	return room
}

// Run starts the 20 TPS game loop. Call in a goroutine.
func (room *MapRoom) Run() {
	ticker := time.NewTicker(tickInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			room.tick()
		case data := <-room.broadcastQ:
			room.broadcastRaw(data)
		case <-room.stopCh:
			return
		}
	}
}

// Stop signals the game loop to exit.
func (room *MapRoom) Stop() {
	select {
	case <-room.stopCh:
	default:
		close(room.stopCh)
	}
}

// StopChan returns a channel that is closed when this room is stopped.
// Use it to cancel goroutines that must not outlive the room.
func (room *MapRoom) StopChan() <-chan struct{} {
	return room.stopCh
}

// PassabilitySnapshot returns a flat boolean array (row-major, width*height)
// where true means the tile is passable in at least one direction.
// Used by the client minimap to render terrain accurately.
func (room *MapRoom) PassabilitySnapshot() map[string]interface{} {
	pm := room.passMap
	if pm == nil {
		return nil
	}
	w, h := pm.Width, pm.Height
	tiles := make([]int, w*h)
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			if pm.CanPass(x, y, 2) || pm.CanPass(x, y, 4) ||
				pm.CanPass(x, y, 6) || pm.CanPass(x, y, 8) {
				tiles[y*w+x] = 1
			}
		}
	}
	return map[string]interface{}{
		"width":  w,
		"height": h,
		"tiles":  tiles,
	}
}

// tick is called every 50 ms (20 TPS).
func (room *MapRoom) tick() {
	room.cleanStaleSessions()
	room.broadcastDirtyPlayers()
	room.tickNPCs()
	room.tickMonsters()
	room.cleanExpiredDrops()
}

// cleanStaleSessions removes players whose connections have been closed.
// This is a safety net; the primary cleanup happens in handleDisconnect.
func (room *MapRoom) cleanStaleSessions() {
	room.mu.Lock()
	var stale []int64
	for id, s := range room.players {
		if s.IsClosed() {
			stale = append(stale, id)
		}
	}
	for _, id := range stale {
		delete(room.players, id)
	}
	room.mu.Unlock()

	// Broadcast player_leave outside the lock.
	for _, id := range stale {
		payload, _ := json.Marshal(map[string]interface{}{"char_id": id})
		pkt, _ := json.Marshal(&player.Packet{Type: "player_leave", Payload: payload})
		room.broadcastRaw(pkt)
		room.logger.Info("removed stale player from room",
			zap.Int64("char_id", id), zap.Int("map_id", room.MapID))
	}
}

// broadcastDirtyPlayers sends player_sync for any player whose position changed.
func (room *MapRoom) broadcastDirtyPlayers() {
	room.mu.RLock()
	defer room.mu.RUnlock()

	for _, s := range room.players {
		if !s.ResetDirty() {
			continue
		}
		x, y, dir := s.Position()
		hp, _, mp, _ := s.Stats()
		payload, _ := json.Marshal(map[string]interface{}{
			"char_id": s.CharID,
			"x":       x,
			"y":       y,
			"dir":     dir,
			"hp":      hp,
			"mp":      mp,
			"state":   "normal",
		})
		pkt, _ := json.Marshal(&player.Packet{
			Type:    "player_sync",
			Payload: payload,
		})
		room.broadcastRaw(pkt)
	}
}

// tickMonsters runs one AI tick for each live monster.
func (room *MapRoom) tickMonsters() {
	room.mu.RLock()
	rms := make([]*MonsterRuntime, 0, len(room.runtimeMonsters))
	for _, m := range room.runtimeMonsters {
		rms = append(rms, m)
	}
	room.mu.RUnlock()

	for _, m := range rms {
		if m.GetState() == 6 { // dead
			continue
		}
		if m.AITree != nil {
			// TODO: build ai.AIContext and tick the behavior tree
		}
	}
}

// cleanExpiredDrops removes drops that have been on the ground too long.
func (room *MapRoom) cleanExpiredDrops() {
	now := time.Now()
	room.mu.Lock()
	for id, d := range room.drops {
		if !d.ExpireAt.IsZero() && now.After(d.ExpireAt) {
			delete(room.drops, id)
		}
	}
	room.mu.Unlock()
}

// Broadcast enqueues data to be sent to all players in the room.
func (room *MapRoom) Broadcast(data []byte) {
	select {
	case room.broadcastQ <- data:
	default:
		room.logger.Warn("broadcastQ full, dropping packet", zap.Int("map_id", room.MapID))
	}
}

// BroadcastExcept sends data to all players except the one with excludeCharID.
func (room *MapRoom) BroadcastExcept(data []byte, excludeCharID int64) {
	room.mu.RLock()
	defer room.mu.RUnlock()
	for _, s := range room.players {
		if s.CharID != excludeCharID {
			s.SendRaw(data)
		}
	}
}

// broadcastRaw delivers data to every player currently in the room.
func (room *MapRoom) broadcastRaw(data []byte) {
	room.mu.RLock()
	defer room.mu.RUnlock()
	for _, s := range room.players {
		s.SendRaw(data)
	}
}

// AddPlayer adds a PlayerSession to this MapRoom.
func (room *MapRoom) AddPlayer(s *player.PlayerSession) {
	room.mu.Lock()
	defer room.mu.Unlock()
	room.players[s.CharID] = s
}

// RemovePlayer removes a PlayerSession from the MapRoom.
func (room *MapRoom) RemovePlayer(charID int64) {
	room.mu.Lock()
	defer room.mu.Unlock()
	delete(room.players, charID)
}

// ForEachPlayer calls fn for every player in the room (under read lock).
func (room *MapRoom) ForEachPlayer(fn func(*player.PlayerSession)) {
	room.mu.RLock()
	defer room.mu.RUnlock()
	for _, s := range room.players {
		fn(s)
	}
}

// PlayerCount returns the current number of players in the room.
func (room *MapRoom) PlayerCount() int {
	room.mu.RLock()
	defer room.mu.RUnlock()
	return len(room.players)
}

// PlayerSnapshot returns a snapshot of all players suitable for map_init.
func (room *MapRoom) PlayerSnapshot() []map[string]interface{} {
	room.mu.RLock()
	defer room.mu.RUnlock()
	out := make([]map[string]interface{}, 0, len(room.players))
	for _, s := range room.players {
		x, y, dir := s.Position()
		hp, maxHP, mp, maxMP := s.Stats()
		out = append(out, map[string]interface{}{
			"char_id":    s.CharID,
			"name":       s.CharName,
			"walk_name":  s.WalkName,
			"walk_index": s.WalkIndex,
			"x":          x,
			"y":          y,
			"dir":        dir,
			"hp":         hp,
			"max_hp":     maxHP,
			"mp":         mp,
			"max_mp":     maxMP,
		})
	}
	return out
}

// ---- Monster management ----

// AddMonsterRuntime registers a MonsterRuntime and its client snapshot.
func (room *MapRoom) AddMonsterRuntime(m *MonsterRuntime) {
	room.mu.Lock()
	defer room.mu.Unlock()
	room.runtimeMonsters[m.InstID] = m
	room.monsters = append(room.monsters, &MonsterInstance{
		ID:      m.InstID,
		EnemyID: m.Template.ID,
		Name:    m.Template.Name,
		X:       m.X,
		Y:       m.Y,
		HP:      m.HP,
		MaxHP:   m.MaxHP,
	})
}

// GetMonster returns the MonsterRuntime for instID, or nil if not found.
func (room *MapRoom) GetMonster(instID int64) *MonsterRuntime {
	room.mu.RLock()
	defer room.mu.RUnlock()
	return room.runtimeMonsters[instID]
}

// RemoveMonster removes a monster from the room.
func (room *MapRoom) RemoveMonster(instID int64) {
	room.mu.Lock()
	defer room.mu.Unlock()
	delete(room.runtimeMonsters, instID)
	for i, m := range room.monsters {
		if m.ID == instID {
			room.monsters = append(room.monsters[:i], room.monsters[i+1:]...)
			break
		}
	}
}

// MonsterSnapshot returns the client-visible monster list.
func (room *MapRoom) MonsterSnapshot() []*MonsterInstance {
	room.mu.RLock()
	defer room.mu.RUnlock()
	out := make([]*MonsterInstance, len(room.monsters))
	copy(out, room.monsters)
	return out
}

// ---- Drop management ----

// AddDrop creates a new drop entry on the map floor.
// expire = 5 minutes by default.
func (room *MapRoom) AddDrop(dropID int64, itemType, itemID, x, y int) {
	room.mu.Lock()
	defer room.mu.Unlock()
	room.drops[dropID] = &DropRuntimeEntry{
		DropID:   dropID,
		ItemType: itemType,
		ItemID:   itemID,
		X:        x,
		Y:        y,
		ExpireAt: time.Now().Add(5 * time.Minute),
	}
}

// GetDrop returns the drop entry for dropID, or nil.
func (room *MapRoom) GetDrop(dropID int64) *DropRuntimeEntry {
	room.mu.RLock()
	defer room.mu.RUnlock()
	d := room.drops[dropID]
	if d == nil || d.claimed {
		return nil
	}
	return d
}

// ConsumeDrop atomically marks a drop as claimed and removes it.
// Returns false if the drop was already claimed or doesn't exist.
func (room *MapRoom) ConsumeDrop(dropID int64) bool {
	room.mu.Lock()
	defer room.mu.Unlock()
	d, ok := room.drops[dropID]
	if !ok || d.claimed {
		return false
	}
	d.claimed = true
	delete(room.drops, dropID)
	return true
}
