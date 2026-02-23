package battle

import (
	"math/rand"

	"github.com/kasuganosora/rpgmakermvmmo/server/resource"
)

// DropResult represents one item that dropped.
type DropResult struct {
	ItemType int // 1=Item 2=Weapon 3=Armor
	ItemID   int
	Quantity int
}

// CalculateDrops runs the RMMV drop probability table for an enemy.
// Each entry has a probability = 1/denominator.
func CalculateDrops(enemy *resource.Enemy) []DropResult {
	var results []DropResult
	for _, d := range enemy.DropItems {
		if d.Kind == 0 {
			continue // empty slot
		}
		if d.Denominator <= 0 {
			continue
		}
		if rand.Intn(d.Denominator) == 0 {
			results = append(results, DropResult{
				ItemType: d.Kind,
				ItemID:   d.DataID,
				Quantity: 1,
			})
		}
	}
	return results
}

// CalculateExp computes the exp each player in a group receives.
//   - partySize: number of party members in range
//   - baseExp: enemy.Exp
//
// Formula: bonus = 1.0 + (size-1)*0.1, capped at 1.4; each = baseExp * bonus / size
func CalculateExp(baseExp, partySize int) int {
	if partySize <= 0 {
		partySize = 1
	}
	bonus := 1.0 + float64(partySize-1)*0.1
	if bonus > 1.4 {
		bonus = 1.4
	}
	each := float64(baseExp) * bonus / float64(partySize)
	if each < 1 {
		each = 1
	}
	return int(each)
}

// ExpNeeded returns the total exp needed to reach the next level.
// Uses a simple quadratic formula based on RMMV defaults.
func ExpNeeded(level int) int {
	// RMMV default: base=30, extra=20, acc=30
	// nextLevelExp = basis + extra * (level - 1) + ... (simplified)
	if level <= 0 {
		return 30
	}
	return 30*level + 20*(level-1)*level/2
}
