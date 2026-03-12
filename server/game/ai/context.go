package ai

import (
	"time"

	"github.com/kasuganosora/rpgmakermvmmo/server/resource"
)

// GroupInfo provides group-level context for monsters in a group.
type GroupInfo struct {
	GroupType    string // "assist" | "linked" | "pack"
	LeaderTarget int64 // pack mode: leader's current target charID (0 = none)
}

// AIContext is passed to every behavior tree node during a tick.
type AIContext struct {
	Monster        MonsterAccessor
	Room           RoomAccessor
	DeltaMS        int64 // milliseconds since last tick
	Config         *AIProfile
	ThreatTable    *ThreatTable
	DamageCallback func(m MonsterAccessor, targetCharID int64) // called by AttackTarget node
	GroupInfo      *GroupInfo // nil if not in a group
	OnLeash        func() // called when monster returns to spawn (for linked group leash)
}

// MonsterAccessor provides read/write access to monster state for BT nodes.
type MonsterAccessor interface {
	GetState() MonsterState
	SetState(MonsterState)
	Position() (x, y int)
	SetPosition(x, y, dir int)
	SpawnPosition() (x, y int)
	GetHP() int
	GetMaxHP() int
	GetTarget() int64
	SetTarget(int64)
	GetAgi() int
	GetCachedPath() []Point
	SetCachedPath(path []Point, target Point)
	GetCachedTarget() Point
	CanMove() bool
	ResetMoveTimer(ticks int)
	CanAttack() bool
	ResetAttackTimer(ticks int)
	MarkDirty()
}

// RoomAccessor abstracts MapRoom access for the AI layer.
type RoomAccessor interface {
	PlayersInRange(x, y, radius int) []PlayerInfo
	PlayerByID(charID int64) *PlayerInfo
	TryMoveMonster(m MonsterAccessor, dir int) bool
	GetPassMap() *resource.PassabilityMap
}

// MonsterState enumerates the high-level AI states of a monster.
type MonsterState int

const (
	StateIdle   MonsterState = iota
	StateWander              // random patrol
	StateAlert               // detected a player, not yet chasing
	StateChase               // actively pursuing a player
	StateAttack              // close enough to attack
	StateDead
)

// PlayerInfo is the minimal player data the AI needs.
type PlayerInfo struct {
	CharID int64
	X, Y   int
	HP     int
}

// TimeSince is a test-injectable time source.
var TimeSince = func(t time.Time) time.Duration {
	return time.Since(t)
}
