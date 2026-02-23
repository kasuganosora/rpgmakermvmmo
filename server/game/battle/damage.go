package battle

import (
	"math"
	"math/rand"

	"github.com/kasuganosora/rpgmakermvmmo/server/resource"
)

// BuffInstance represents a currently active buff/debuff on a character.
type BuffInstance struct {
	StateID     int
	ParamRates  [8]float64 // index matches RMMV param order: 0=maxHP,1=maxMP,2=atk,3=def,4=mat,5=mdf,6=agi,7=luk
	DamageBonus float64    // multiplier applied to outgoing damage (1.0 = no effect)
	DamageTaken float64    // multiplier applied to incoming damage (1.0 = no effect)
}

// DamageContext bundles everything needed to compute one damage event.
type DamageContext struct {
	Attacker    *CharacterStats
	Defender    *CharacterStats
	Skill       *resource.Skill
	AttackerBuf []*BuffInstance
	DefenderBuf []*BuffInstance
}

// DamageResult holds the outcome of a damage calculation.
type DamageResult struct {
	FinalDamage int
	IsCrit      bool
}

// BeforeDamageCalc is a hook point called before damage is computed (Task 07).
var BeforeDamageCalc func(ctx *DamageContext)

// AfterDamageCalc is a hook point called after damage is computed (Task 07).
var AfterDamageCalc func(ctx *DamageContext, result *DamageResult)

// Calculate runs the full damage computation pipeline.
func Calculate(ctx *DamageContext) DamageResult {
	// ① Hook: before_damage_calc
	if BeforeDamageCalc != nil {
		BeforeDamageCalc(ctx)
	}

	// ② Evaluate the damage formula.
	formula := ""
	if ctx.Skill != nil {
		formula = ctx.Skill.Damage.Formula
	}

	var base float64
	if formula == "" {
		// Fallback: physical auto-attack
		base = float64(ctx.Attacker.Atk*4 - ctx.Defender.Def*2)
	} else {
		v, err := EvalFormula(formula, ctx.Attacker, ctx.Defender)
		if err != nil {
			// On parse error, fall back to zero damage.
			v = 0
		}
		base = v
	}

	// ③ Element rate (simplified: if skill has element, apply defender's resistance).
	// Full implementation would read the Defender's trait table from RMMV.
	// Using 1.0 as default (no resistance).
	elementMult := 1.0
	if ctx.Skill != nil {
		elementMult = elementRate(ctx.Skill.Damage.ElementID, ctx.DefenderBuf)
	}
	base *= elementMult

	// ④ Buff trait modifiers.
	for _, b := range ctx.AttackerBuf {
		base *= b.DamageBonus
	}
	for _, b := range ctx.DefenderBuf {
		base *= b.DamageTaken
	}

	// ⑤ Crit check: base crit rate = luk / 1000 (capped at 50%).
	critRate := math.Min(float64(ctx.Attacker.Luk)/1000.0, 0.5)
	isCrit := rand.Float64() < critRate
	if isCrit {
		base *= 1.5
	}

	// ⑥ ±10% variance.
	base *= 0.9 + rand.Float64()*0.2

	// ⑦ Clamp to [0, ∞).
	finalDamage := int(math.Max(0, math.Round(base)))

	result := DamageResult{FinalDamage: finalDamage, IsCrit: isCrit}

	// ⑧ Hook: after_damage_calc
	if AfterDamageCalc != nil {
		AfterDamageCalc(ctx, &result)
	}

	return result
}

// elementRate returns the element effectiveness multiplier.
// elementID 0 = physical (no resistance). Full implementation would
// look up the defender's traits from the RMMV data; for now a simple table.
func elementRate(elementID int, defBufs []*BuffInstance) float64 {
	// Default: no resistance/weakness
	_ = defBufs
	_ = elementID
	return 1.0
}
