package ai

import "time"

// AIContext is passed to every behavior tree node during a tick.
// It provides access to the monster, the map room, and shared resources.
type AIContext struct {
	Monster interface{ GetState() MonsterState }
	Room    RoomAccessor
	DeltaMS int64 // milliseconds since last tick
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

// RoomAccessor abstracts MapRoom access for the AI layer.
// Implemented by *world.MapRoom — declared here as an interface to avoid import cycle.
type RoomAccessor interface {
	PlayersInRange(x, y, radius int) []PlayerInfo
}

// PlayerInfo is the minimal player data the AI needs.
type PlayerInfo struct {
	CharID    int64
	X, Y      int
	HP        int
}

// TimeSince is a test-injectable time source.
var TimeSince = func(t time.Time) time.Duration {
	return time.Since(t)
}
