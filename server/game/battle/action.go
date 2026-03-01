package battle

import (
	"math"
	"math/rand"

	"github.com/kasuganosora/rpgmakermvmmo/server/resource"
)

// ActionProcessor handles executing battle actions with RMMV-accurate mechanics.
type ActionProcessor struct {
	Res *resource.ResourceLoader
	RNG *rand.Rand
}

// ActionOutcome is the result of processing one action against one target.
type ActionOutcome struct {
	TargetIndex    int
	TargetIsActor  bool
	Damage         int  // positive=damage, negative=heal
	Critical       bool
	Missed         bool
	Drain          int  // HP drained back to attacker (for drain skills)
	AddedStates    []int
	RemovedStates  []int
	CommonEventIDs []int // triggered common event IDs (RMMV effect code 44)
}

// ProcessAction executes a battler's action and returns outcomes per target.
func (ap *ActionProcessor) ProcessAction(
	subject Battler,
	action *Action,
	actors, enemies []Battler,
) []ActionOutcome {
	if action == nil {
		return nil
	}

	switch action.Type {
	case ActionAttack:
		return ap.processSkillAction(subject, 1, action, actors, enemies) // skill 1 = normal attack
	case ActionSkill:
		return ap.processSkillAction(subject, action.SkillID, action, actors, enemies)
	case ActionItem:
		return ap.processItemAction(subject, action.ItemID, action, actors, enemies)
	case ActionGuard:
		subject.SetGuarding(true)
		return nil
	case ActionEscape:
		return nil // handled by BattleInstance
	default:
		return nil
	}
}

func (ap *ActionProcessor) processSkillAction(
	subject Battler,
	skillID int,
	action *Action,
	actors, enemies []Battler,
) []ActionOutcome {
	skill := ap.Res.SkillByID(skillID)
	if skill == nil {
		return nil
	}

	// Deduct MP/TP cost.
	subject.SetMP(subject.MP() - skill.MPCost)
	subject.SetTP(subject.TP() - skill.TPCost)

	// TP gain on use.
	if skill.TPGain > 0 {
		subject.SetTP(subject.TP() + skill.TPGain)
	}

	targets := ap.resolveTargets(action, skill.Scope, subject, actors, enemies)
	repeats := skill.Repeats
	if repeats < 1 {
		repeats = 1
	}

	var outcomes []ActionOutcome
	for _, tgt := range targets {
		for r := 0; r < repeats; r++ {
			out := ap.applySkillToTarget(subject, tgt, skill)
			outcomes = append(outcomes, out)
		}
	}

	// Apply skill effects (state add/remove, buffs, etc.).
	for i := range outcomes {
		if !outcomes[i].Missed {
			ap.applySkillEffects(subject, ap.targetByOutcome(outcomes[i], actors, enemies), skill.Effects, &outcomes[i])
		}
	}

	return outcomes
}

func (ap *ActionProcessor) processItemAction(
	subject Battler,
	itemID int,
	action *Action,
	actors, enemies []Battler,
) []ActionOutcome {
	if itemID <= 0 || itemID >= len(ap.Res.Items) || ap.Res.Items[itemID] == nil {
		return nil
	}
	item := ap.Res.Items[itemID]

	targets := ap.resolveTargets(action, item.Scope, subject, actors, enemies)

	var outcomes []ActionOutcome
	for _, tgt := range targets {
		out := ap.applyItemToTarget(subject, tgt, item)
		outcomes = append(outcomes, out)
	}

	for i := range outcomes {
		if !outcomes[i].Missed {
			ap.applySkillEffects(subject, ap.targetByOutcome(outcomes[i], actors, enemies), item.Effects, &outcomes[i])
		}
	}

	return outcomes
}

// applySkillToTarget calculates damage/healing for a single skill hit.
func (ap *ActionProcessor) applySkillToTarget(subject, target Battler, skill *resource.Skill) ActionOutcome {
	out := ActionOutcome{
		TargetIndex:   target.Index(),
		TargetIsActor: target.IsActor(),
	}

	// Hit check.
	if !ap.checkHit(subject, target, skill.HitType, skill.SuccessRate) {
		out.Missed = true
		return out
	}

	if skill.Damage.Type == 0 {
		// No damage (effect-only skill).
		return out
	}

	dmgResult := ap.calcDamage(subject, target, &skill.Damage)
	damage := dmgResult.damage
	out.Critical = dmgResult.critical

	// Apply guard.
	if target.IsGuarding() {
		grd := target.SParam(1) // grd sparam
		if grd <= 0 {
			grd = 1.0
		}
		damage = int(float64(damage) / (2.0 * grd))
	}

	// Apply damage type.
	switch skill.Damage.Type {
	case 1: // HP damage
		target.SetHP(target.HP() - damage)
		out.Damage = damage
	case 2: // MP damage
		target.SetMP(target.MP() - damage)
		out.Damage = damage
	case 3: // HP recovery
		target.SetHP(target.HP() + damage)
		out.Damage = -damage
	case 4: // MP recovery
		target.SetMP(target.MP() + damage)
		out.Damage = -damage
	case 5: // HP drain
		target.SetHP(target.HP() - damage)
		subject.SetHP(subject.HP() + damage)
		out.Damage = damage
		out.Drain = damage
	case 6: // MP drain
		target.SetMP(target.MP() - damage)
		subject.SetMP(subject.MP() + damage)
		out.Damage = damage
		out.Drain = damage
	}

	return out
}

func (ap *ActionProcessor) applyItemToTarget(subject, target Battler, item *resource.Item) ActionOutcome {
	out := ActionOutcome{
		TargetIndex:   target.Index(),
		TargetIsActor: target.IsActor(),
	}

	if !ap.checkHit(subject, target, item.HitType, item.SuccessRate) {
		out.Missed = true
		return out
	}

	if item.Damage.Type == 0 {
		return out
	}

	dmgResult := ap.calcDamage(subject, target, &item.Damage)
	damage := dmgResult.damage
	out.Critical = dmgResult.critical

	switch item.Damage.Type {
	case 1:
		target.SetHP(target.HP() - damage)
		out.Damage = damage
	case 3:
		target.SetHP(target.HP() + damage)
		out.Damage = -damage
	case 4:
		target.SetMP(target.MP() + damage)
		out.Damage = -damage
	}

	return out
}

type damageCalcResult struct {
	damage   int
	critical bool
}

// calcDamage evaluates the damage formula with crit, variance, and element.
func (ap *ActionProcessor) calcDamage(subject, target Battler, dmg *resource.SkillDamage) damageCalcResult {
	atkStats := subject.ToCharacterStats()
	defStats := target.ToCharacterStats()

	var base float64
	if dmg.Formula == "" {
		base = float64(atkStats.Atk*4 - defStats.Def*2)
	} else {
		v, err := EvalFormula(dmg.Formula, atkStats, defStats)
		if err != nil {
			// Fallback: standard RMMV physical damage formula.
			base = float64(atkStats.Atk*4 - defStats.Def*2)
		} else {
			base = v
		}
	}

	// Element rate.
	if dmg.ElementID > 0 {
		base *= target.ElementRate(dmg.ElementID)
	}

	// Physical/Magical damage reduction via SParams.
	// pdr (code 23, id 6) for physical, mdr (code 23, id 7) for magical.
	// In RMMV, this is applied as a trait multiplier. We approximate it here.

	// Critical hit.
	isCrit := false
	if dmg.Critical {
		criRate := subject.XParam(2) // cri
		cevRate := target.XParam(3)  // cev (critical evasion)
		effectiveCri := criRate - cevRate
		if effectiveCri > 0 && ap.RNG.Float64() < effectiveCri {
			isCrit = true
			base *= 3.0 // RMMV: 3x crit multiplier
		}
	}

	// Variance.
	variance := dmg.Variance
	if variance > 0 {
		amp := math.Abs(base) * float64(variance) / 100.0
		base += amp * (ap.RNG.Float64()*2.0 - 1.0)
	}

	// Clamp to minimum 0 for damage types, keep positive for recovery.
	result := int(math.Round(base))
	if result < 0 {
		result = 0
	}

	return damageCalcResult{damage: result, critical: isCrit}
}

// checkHit determines if an action hits based on hit type and success rate.
func (ap *ActionProcessor) checkHit(subject, target Battler, hitType, successRate int) bool {
	if successRate <= 0 {
		successRate = 100
	}
	rate := float64(successRate) / 100.0

	switch hitType {
	case 0: // certain hit
		return true
	case 1: // physical
		hit := subject.XParam(0) // hit rate
		eva := target.XParam(1)  // evasion
		rate *= hit
		rate *= (1.0 - eva)
	case 2: // magical
		mev := target.XParam(4) // magic evasion
		rate *= (1.0 - mev)
	}

	return ap.RNG.Float64() < rate
}

// resolveTargets maps the action's target info to actual Battler instances.
func (ap *ActionProcessor) resolveTargets(
	action *Action,
	scope int,
	subject Battler,
	actors, enemies []Battler,
) []Battler {
	// If action has explicit targets, use those.
	if len(action.TargetIndices) > 0 {
		var pool []Battler
		if action.TargetIsActor {
			pool = actors
		} else {
			pool = enemies
		}
		// For actors attacking enemies: action.TargetIsActor=false → pool=enemies
		// For enemies attacking actors: action.TargetIsActor=true → pool=actors
		var targets []Battler
		for _, idx := range action.TargetIndices {
			if idx >= 0 && idx < len(pool) && pool[idx].IsAlive() {
				targets = append(targets, pool[idx])
			}
		}
		if len(targets) > 0 {
			return targets
		}
		// Fall through to scope-based resolution if explicit targets invalid.
	}

	return ap.resolveByScope(scope, subject, actors, enemies)
}

// resolveByScope resolves targets based on RMMV scope values.
func (ap *ActionProcessor) resolveByScope(
	scope int,
	subject Battler,
	actors, enemies []Battler,
) []Battler {
	var friendlies, opponents []Battler
	if subject.IsActor() {
		friendlies = actors
		opponents = enemies
	} else {
		friendlies = enemies
		opponents = actors
	}

	switch scope {
	case 0: // none
		return nil
	case 1: // 1 enemy
		return ap.randomAlive(opponents, 1)
	case 2: // all enemies
		return aliveAll(opponents)
	case 3: // 1 random enemy
		return ap.randomAlive(opponents, 1)
	case 4: // 2 random enemies
		return ap.randomAlive(opponents, 2)
	case 5: // 3 random enemies
		return ap.randomAlive(opponents, 3)
	case 6: // 4 random enemies
		return ap.randomAlive(opponents, 4)
	case 7: // 1 ally
		return ap.randomAlive(friendlies, 1)
	case 8: // all allies
		return aliveAll(friendlies)
	case 9: // 1 ally (dead)
		return ap.randomDead(friendlies, 1)
	case 10: // all allies (dead)
		return deadAll(friendlies)
	case 11: // user
		return []Battler{subject}
	default:
		return nil
	}
}

func (ap *ActionProcessor) randomAlive(pool []Battler, n int) []Battler {
	var alive []Battler
	for _, b := range pool {
		if b.IsAlive() {
			alive = append(alive, b)
		}
	}
	if len(alive) == 0 {
		return nil
	}
	if n >= len(alive) {
		return alive
	}
	// For random target selection, pick n with replacement (RMMV behavior for random scopes).
	var result []Battler
	for i := 0; i < n; i++ {
		result = append(result, alive[ap.RNG.Intn(len(alive))])
	}
	return result
}

func (ap *ActionProcessor) randomDead(pool []Battler, n int) []Battler {
	var dead []Battler
	for _, b := range pool {
		if b.IsDead() {
			dead = append(dead, b)
		}
	}
	if len(dead) == 0 {
		return nil
	}
	if n >= len(dead) {
		return dead
	}
	result := make([]Battler, n)
	for i := 0; i < n; i++ {
		result[i] = dead[ap.RNG.Intn(len(dead))]
	}
	return result
}

func aliveAll(pool []Battler) []Battler {
	var alive []Battler
	for _, b := range pool {
		if b.IsAlive() {
			alive = append(alive, b)
		}
	}
	return alive
}

func deadAll(pool []Battler) []Battler {
	var dead []Battler
	for _, b := range pool {
		if b.IsDead() {
			dead = append(dead, b)
		}
	}
	return dead
}

// applySkillEffects processes the Effects array of a skill/item.
func (ap *ActionProcessor) applySkillEffects(
	subject, target Battler,
	effects []resource.SkillEffect,
	out *ActionOutcome,
) {
	// RMMV Effect Codes — from rpg_objects.js Game_Action constants:
	//  11=RECOVER_HP, 12=RECOVER_MP, 13=GAIN_TP
	//  21=ADD_STATE,  22=REMOVE_STATE
	//  31=ADD_BUFF,   32=ADD_DEBUFF, 33=REMOVE_BUFF, 34=REMOVE_DEBUFF
	//  41=SPECIAL,    42=GROW, 43=LEARN_SKILL, 44=COMMON_EVENT
	for _, eff := range effects {
		switch eff.Code {
		case 11: // Recover HP — value1=rate, value2=flat
			amount := int(float64(target.MaxHP()) * eff.Value1)
			amount += int(eff.Value2)
			target.SetHP(target.HP() + amount)
			if out.Damage == 0 {
				out.Damage = -amount
			}
		case 12: // Recover MP — value1=rate, value2=flat
			amount := int(float64(target.MaxMP()) * eff.Value1)
			amount += int(eff.Value2)
			target.SetMP(target.MP() + amount)
		case 13: // Gain TP — value1=flat
			target.SetTP(target.TP() + int(eff.Value1))
		case 21: // Add State — dataId=stateID, value1=chance
			stateID := eff.DataID
			chance := eff.Value1
			if stateID > 0 {
				chance *= target.StateRate(stateID)
			}
			if ap.RNG.Float64() < chance {
				target.AddState(stateID, -1)
				if ap.Res != nil {
					for _, st := range ap.Res.States {
						if st != nil && st.ID == stateID && st.AutoRemovalTiming > 0 {
							turns := st.MinTurns
							if st.MaxTurns > st.MinTurns {
								turns += ap.RNG.Intn(st.MaxTurns - st.MinTurns + 1)
							}
							target.AddState(stateID, turns)
							break
						}
					}
				}
				out.AddedStates = append(out.AddedStates, stateID)
			}
		case 22: // Remove State — dataId=stateID, value1=chance
			stateID := eff.DataID
			if ap.RNG.Float64() < eff.Value1 {
				target.RemoveState(stateID)
				out.RemovedStates = append(out.RemovedStates, stateID)
			}
		case 31: // Add Buff — dataId=paramId(0-7), value1=turns
			target.AddBuff(eff.DataID, int(eff.Value1))
		case 32: // Add Debuff — dataId=paramId(0-7), value1=turns
			target.AddDebuff(eff.DataID, int(eff.Value1))
		case 33: // Remove Buff — dataId=paramId(0-7)
			target.RemoveBuff(eff.DataID)
		case 34: // Remove Debuff — dataId=paramId(0-7)
			target.RemoveBuff(eff.DataID)
		case 41: // Special Effect — dataId=0 means escape
			if eff.DataID == 0 {
				// Handled by BattleInstance
			}
		case 42: // Grow (permanent stat increase)
			// Not implemented in battle context
		case 43: // Learn Skill
			// Not applicable in battle context
		case 44: // Common Event — dataId=commonEventId
			out.CommonEventIDs = append(out.CommonEventIDs, eff.DataID)
		}
	}
}

func (ap *ActionProcessor) targetByOutcome(out ActionOutcome, actors, enemies []Battler) Battler {
	var pool []Battler
	if out.TargetIsActor {
		pool = actors
	} else {
		pool = enemies
	}
	if out.TargetIndex >= 0 && out.TargetIndex < len(pool) {
		return pool[out.TargetIndex]
	}
	return nil
}
