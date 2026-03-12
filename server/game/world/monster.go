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
	SpawnX    int // original spawn position X
	SpawnY    int // original spawn position Y
	State     ai.MonsterState
	Target    int64 // CharID of current target (0=none)
	AITree    *ai.BehaviorTree
	Profile   *ai.AIProfile // AI behavior configuration
	Threat    *ai.ThreatTable
	NextSpawn time.Time // populated when dead
	LastHitAt time.Time
	MoveTimer int // ticks until next movement allowed
	CachedPath   []ai.Point
	CachedTarget ai.Point
	AttackTimer  int // ticks until next attack allowed
	dirty        bool

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
	m.dirty = true
	if m.Target == 0 {
		m.Target = attackerID
	}
	if m.Threat != nil {
		m.Threat.AddThreat(attackerID, dmg)
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
	AIOverride string // optional: override enemy's Note AI profile name
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
		SpawnX:   x,
		SpawnY:   y,
		Dir:      2, // face down by default
		State:    ai.StateIdle,
		Threat:   ai.NewThreatTable(),
	}
}

// ---- MonsterAccessor interface implementation ----

// Position returns the monster's current tile position.
func (m *MonsterRuntime) Position() (int, int) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.X, m.Y
}

// SetPosition sets position and direction, marks dirty.
func (m *MonsterRuntime) SetPosition(x, y, dir int) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.X = x
	m.Y = y
	m.Dir = dir
	m.dirty = true
}

// SpawnPosition returns the original spawn point.
func (m *MonsterRuntime) SpawnPosition() (int, int) {
	return m.SpawnX, m.SpawnY
}

// GetHP returns current HP.
func (m *MonsterRuntime) GetHP() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.HP
}

// GetMaxHP returns max HP.
func (m *MonsterRuntime) GetMaxHP() int {
	return m.MaxHP
}

// GetTarget returns the current target charID.
func (m *MonsterRuntime) GetTarget() int64 {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.Target
}

// SetTarget sets the current target charID.
func (m *MonsterRuntime) SetTarget(id int64) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.Target = id
}

// GetAgi returns the monster's agility stat.
func (m *MonsterRuntime) GetAgi() int {
	if m.Template != nil {
		return m.Template.Agi
	}
	return 0
}

// GetCachedPath returns the cached A* path.
func (m *MonsterRuntime) GetCachedPath() []ai.Point {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.CachedPath
}

// SetCachedPath updates the cached path and target.
func (m *MonsterRuntime) SetCachedPath(path []ai.Point, target ai.Point) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.CachedPath = path
	m.CachedTarget = target
}

// GetCachedTarget returns the position the cached path was computed toward.
func (m *MonsterRuntime) GetCachedTarget() ai.Point {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.CachedTarget
}

// CanMove returns true if the move cooldown has expired.
func (m *MonsterRuntime) CanMove() bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.MoveTimer <= 0
}

// ResetMoveTimer sets the move cooldown.
func (m *MonsterRuntime) ResetMoveTimer(ticks int) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.MoveTimer = ticks
}

// CanAttack returns true if the attack cooldown has expired.
func (m *MonsterRuntime) CanAttack() bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.AttackTimer <= 0
}

// ResetAttackTimer sets the attack cooldown.
func (m *MonsterRuntime) ResetAttackTimer(ticks int) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.AttackTimer = ticks
}

// MarkDirty flags the monster for position broadcast.
func (m *MonsterRuntime) MarkDirty() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.dirty = true
}

// ---- RoomAccessor interface implementation on MapRoom ----

// PlayersInRange returns players within radius tiles of (x, y).
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

// PlayerByID returns player info for a specific character, or nil if not in room.
func (room *MapRoom) PlayerByID(charID int64) *ai.PlayerInfo {
	room.mu.RLock()
	defer room.mu.RUnlock()
	s, ok := room.players[charID]
	if !ok {
		return nil
	}
	px, py, _ := s.Position()
	return &ai.PlayerInfo{
		CharID: s.CharID,
		X:      px,
		Y:      py,
		HP:     s.HP,
	}
}

// TryMoveMonster attempts to move a monster one tile in the given direction.
// Checks bounds (with wrapping for looping maps) and tile passability.
// Returns true if the move succeeded.
func (room *MapRoom) TryMoveMonster(m ai.MonsterAccessor, dir int) bool {
	dx, dy := dirDelta(dir)
	mx, my := m.Position()
	nx, ny := mx+dx, my+dy

	// Bounds/wrapping check.
	if room.passMap != nil {
		nx = room.passMap.RoundX(nx)
		ny = room.passMap.RoundY(ny)
		if !room.passMap.IsValid(nx, ny) {
			return false
		}
	} else {
		w, h := room.mapWidth, room.mapHeight
		if w <= 0 || h <= 0 {
			return false
		}
		if nx < 0 || nx >= w || ny < 0 || ny >= h {
			return false
		}
	}

	// Tile passability — CheckPassage combines event tiles + static map.
	// CanPass already handles coordinate wrapping internally.
	if room.passMap != nil {
		if !room.CheckPassage(mx, my, dir) {
			return false
		}
		if !room.CheckPassage(nx, ny, reverseDir(dir)) {
			return false
		}
	}

	m.SetPosition(nx, ny, dir)
	return true
}

// GetPassMap returns the passability map for A* pathfinding.
func (room *MapRoom) GetPassMap() *resource.PassabilityMap {
	return room.passMap
}
