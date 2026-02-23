package world

import (
	"sync"
	"sync/atomic"
	"time"

	"github.com/kasuganosora/rpgmakermvmmo/server/game/ai"
	"github.com/kasuganosora/rpgmakermvmmo/server/resource"
)

// instIDCounter generates unique monster instance IDs.
var instIDCounter int64

func nextInstID() int64 {
	return atomic.AddInt64(&instIDCounter, 1)
}

// MonsterRuntime is the runtime state of a live monster instance.
type MonsterRuntime struct {
	InstID    int64
	SpawnID   int
	Template  *resource.Enemy
	HP        int
	MaxHP     int
	X, Y      int
	Dir       int
	State     ai.MonsterState
	Target    int64 // CharID of current target (0=none)
	AITree    *ai.BehaviorTree
	NextSpawn time.Time  // populated when dead
	LastHitAt time.Time
	CachedPath []ai.Point
	CachedTarget ai.Point

	mu sync.Mutex
}

// GetState implements ai.AIContext monster interface.
func (m *MonsterRuntime) GetState() ai.MonsterState {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.State
}

// SetState updates the monster state.
func (m *MonsterRuntime) SetState(s ai.MonsterState) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.State = s
}

// TakeDamage applies damage to the monster. Returns true if HP reached 0.
func (m *MonsterRuntime) TakeDamage(dmg int, attackerID int64) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.HP -= dmg
	if m.HP < 0 {
		m.HP = 0
	}
	m.LastHitAt = time.Now()
	if m.Target == 0 {
		m.Target = attackerID
	}
	return m.HP == 0
}

// SpawnConfig describes a monster spawn point on a map.
type SpawnConfig struct {
	MapID      int
	MonsterID  int // Enemies.json ID
	X, Y       int
	MaxCount   int
	RespawnSec int
}

// NewMonster creates a fresh MonsterRuntime from a template.
func NewMonster(template *resource.Enemy, spawnID, x, y int) *MonsterRuntime {
	return &MonsterRuntime{
		InstID:   nextInstID(),
		SpawnID:  spawnID,
		Template: template,
		HP:       template.HP,
		MaxHP:    template.HP,
		X:        x,
		Y:        y,
		Dir:      2, // face down by default
		State:    ai.StateIdle,
	}
}

// PlayersInRange returns players within radius tiles of (x, y).
// Implements ai.RoomAccessor for MapRoom.
func (room *MapRoom) PlayersInRange(x, y, radius int) []ai.PlayerInfo {
	room.mu.RLock()
	defer room.mu.RUnlock()
	var result []ai.PlayerInfo
	for _, s := range room.players {
		px, py, _ := s.Position()
		dx := px - x
		dy := py - y
		if dx < 0 {
			dx = -dx
		}
		if dy < 0 {
			dy = -dy
		}
		if dx+dy <= radius {
			result = append(result, ai.PlayerInfo{
				CharID: s.CharID,
				X:      px,
				Y:      py,
				HP:     s.HP,
			})
		}
	}
	return result
}
