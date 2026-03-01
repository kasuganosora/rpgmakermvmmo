package battle

import (
	"math/rand"

	"github.com/kasuganosora/rpgmakermvmmo/server/resource"
)

// MakeEnemyAction selects an action for an enemy battler based on RMMV's
// AI action selection logic: filter by conditions, then weighted random by rating.
func MakeEnemyAction(
	enemy *EnemyBattler,
	turnCount int,
	actors []Battler,
	enemies []Battler,
	res *resource.ResourceLoader,
	rng *rand.Rand,
) *Action {
	if enemy.enemy == nil || len(enemy.enemy.Actions) == 0 {
		// Fallback: basic attack (skill 1) on a random alive actor.
		return makeDefaultAttack(actors, rng)
	}

	valid := filterValidActions(enemy.enemy.Actions, turnCount, enemy)
	if len(valid) == 0 {
		return makeDefaultAttack(actors, rng)
	}

	selected := weightedSelect(valid, rng)

	action := &Action{
		Type:    ActionSkill,
		SkillID: selected.SkillID,
	}

	// Resolve target based on skill scope.
	if res != nil {
		skill := res.SkillByID(selected.SkillID)
		if skill != nil {
			action.TargetIndices, action.TargetIsActor = resolveAITarget(skill.Scope, enemy, actors, enemies, rng)
		} else {
			// Unknown skill → attack random actor.
			action.TargetIndices, action.TargetIsActor = pickRandomAliveIndex(actors, rng), true
		}
	} else {
		action.TargetIndices, action.TargetIsActor = pickRandomAliveIndex(actors, rng), true
	}

	return action
}

func makeDefaultAttack(actors []Battler, rng *rand.Rand) *Action {
	return &Action{
		Type:          ActionAttack,
		SkillID:       1,
		TargetIndices: pickRandomAliveIndex(actors, rng),
		TargetIsActor: true,
	}
}

// filterValidActions checks each action's condition against the current battle state.
func filterValidActions(actions []resource.EnemyAction, turnCount int, enemy *EnemyBattler) []resource.EnemyAction {
	var valid []resource.EnemyAction
	for _, a := range actions {
		if a.Rating <= 0 {
			continue
		}
		if checkCondition(a, turnCount, enemy) {
			valid = append(valid, a)
		}
	}
	return valid
}

// checkCondition evaluates an RMMV enemy action condition.
func checkCondition(a resource.EnemyAction, turnCount int, enemy *EnemyBattler) bool {
	switch a.ConditionType {
	case 0: // Always
		return true
	case 1: // Turn
		// param1 + param2 * X matches turnCount (0-indexed).
		p1 := int(a.ConditionParam1)
		p2 := int(a.ConditionParam2)
		if p2 == 0 {
			return turnCount == p1
		}
		if turnCount < p1 {
			return false
		}
		return (turnCount-p1)%p2 == 0
	case 2: // HP%
		// RMMV stores HP ratio as 0.0-1.0 in conditionParam.
		hpRatio := 0.0
		if enemy.MaxHP() > 0 {
			hpRatio = float64(enemy.HP()) / float64(enemy.MaxHP())
		}
		return hpRatio >= a.ConditionParam1 && hpRatio <= a.ConditionParam2
	case 3: // MP%
		// RMMV stores MP ratio as 0.0-1.0 in conditionParam.
		mpRatio := 0.0
		if enemy.MaxMP() > 0 {
			mpRatio = float64(enemy.MP()) / float64(enemy.MaxMP())
		}
		return mpRatio >= a.ConditionParam1 && mpRatio <= a.ConditionParam2
	case 4: // State
		return enemy.HasState(int(a.ConditionParam1))
	case 5: // Party Level
		// Not applicable to enemies; always true
		return true
	case 6: // Switch
		// Server doesn't have game switches in battle context; always true
		return true
	default:
		return true
	}
}

// weightedSelect picks an action using RMMV's rating-based weighted selection.
// RMMV algorithm: maxRating = max rating among valid; filter to actions within
// maxRating - 2; then weight each action as rating - (maxRating - 3).
func weightedSelect(actions []resource.EnemyAction, rng *rand.Rand) resource.EnemyAction {
	if len(actions) == 1 {
		return actions[0]
	}

	maxRating := 0
	for _, a := range actions {
		if a.Rating > maxRating {
			maxRating = a.Rating
		}
	}

	threshold := maxRating - 2
	var filtered []resource.EnemyAction
	for _, a := range actions {
		if a.Rating >= threshold {
			filtered = append(filtered, a)
		}
	}
	if len(filtered) == 0 {
		filtered = actions
	}

	base := maxRating - 3
	totalWeight := 0
	for _, a := range filtered {
		w := a.Rating - base
		if w < 1 {
			w = 1
		}
		totalWeight += w
	}

	roll := rng.Intn(totalWeight)
	for _, a := range filtered {
		w := a.Rating - base
		if w < 1 {
			w = 1
		}
		roll -= w
		if roll < 0 {
			return a
		}
	}
	return filtered[len(filtered)-1]
}

// resolveAITarget determines target indices based on scope for an enemy.
func resolveAITarget(scope int, enemy *EnemyBattler, actors, enemies []Battler, rng *rand.Rand) ([]int, bool) {
	switch scope {
	case 0: // none
		return nil, false
	case 1, 3: // 1 enemy / 1 random enemy → for enemy AI, "enemy" means actors
		return pickRandomAliveIndex(actors, rng), true
	case 2: // all enemies → all actors
		return allAliveIndices(actors), true
	case 4: // 2 random
		return pickNRandomAliveIndices(actors, 2, rng), true
	case 5: // 3 random
		return pickNRandomAliveIndices(actors, 3, rng), true
	case 6: // 4 random
		return pickNRandomAliveIndices(actors, 4, rng), true
	case 7: // 1 ally → for enemy AI, "ally" means enemies
		return pickRandomAliveIndex(enemies, rng), false
	case 8: // all allies
		return allAliveIndices(enemies), false
	case 9: // 1 dead ally
		return pickRandomDeadIndex(enemies, rng), false
	case 10: // all dead allies
		return allDeadIndices(enemies), false
	case 11: // user
		return []int{enemy.Index()}, false
	default:
		return pickRandomAliveIndex(actors, rng), true
	}
}

func pickRandomAliveIndex(pool []Battler, rng *rand.Rand) []int {
	var alive []int
	for _, b := range pool {
		if b.IsAlive() {
			alive = append(alive, b.Index())
		}
	}
	if len(alive) == 0 {
		return nil
	}
	return []int{alive[rng.Intn(len(alive))]}
}

func pickNRandomAliveIndices(pool []Battler, n int, rng *rand.Rand) []int {
	var alive []int
	for _, b := range pool {
		if b.IsAlive() {
			alive = append(alive, b.Index())
		}
	}
	if len(alive) == 0 {
		return nil
	}
	result := make([]int, n)
	for i := 0; i < n; i++ {
		result[i] = alive[rng.Intn(len(alive))]
	}
	return result
}

func allAliveIndices(pool []Battler) []int {
	var indices []int
	for _, b := range pool {
		if b.IsAlive() {
			indices = append(indices, b.Index())
		}
	}
	return indices
}

func pickRandomDeadIndex(pool []Battler, rng *rand.Rand) []int {
	var dead []int
	for _, b := range pool {
		if b.IsDead() {
			dead = append(dead, b.Index())
		}
	}
	if len(dead) == 0 {
		return nil
	}
	return []int{dead[rng.Intn(len(dead))]}
}

func allDeadIndices(pool []Battler) []int {
	var indices []int
	for _, b := range pool {
		if b.IsDead() {
			indices = append(indices, b.Index())
		}
	}
	return indices
}
