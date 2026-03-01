package battle

import (
	"math/rand"
	"sort"
)

// TurnManager determines the action order for a battle turn.
type TurnManager interface {
	// MakeActionOrder sorts battlers by their effective speed for the turn.
	// The returned slice is a new ordering — the input slices are not modified.
	MakeActionOrder(actors, enemies []Battler, rng *rand.Rand) []Battler
}

// DefaultTurnManager implements RMMV's default AGI-based turn ordering.
// Speed = AGI + action speed modifier + random(0, floor(5 + AGI/4)).
type DefaultTurnManager struct{}

func (DefaultTurnManager) MakeActionOrder(actors, enemies []Battler, rng *rand.Rand) []Battler {
	var all []Battler
	for _, b := range actors {
		if b.IsAlive() {
			all = append(all, b)
		}
	}
	for _, b := range enemies {
		if b.IsAlive() {
			all = append(all, b)
		}
	}

	type entry struct {
		battler Battler
		speed   int
	}
	entries := make([]entry, len(all))
	for i, b := range all {
		agi := b.Param(6)
		actionSpeed := 0
		if a := b.CurrentAction(); a != nil {
			actionSpeed = a.speed()
		}
		randomRange := 5 + agi/4
		if randomRange < 1 {
			randomRange = 1
		}
		speed := agi + actionSpeed + rng.Intn(randomRange)
		entries[i] = entry{battler: b, speed: speed}
	}

	sort.SliceStable(entries, func(i, j int) bool {
		return entries[i].speed > entries[j].speed
	})

	result := make([]Battler, len(entries))
	for i, e := range entries {
		result[i] = e.battler
	}
	return result
}

// speed returns the action speed modifier (from skill/item).
// Returns 0 if no action is set.
func (a *Action) speed() int {
	// Speed modifiers will be populated when the action is created
	// from skill/item data. For now, return 0.
	return 0
}
