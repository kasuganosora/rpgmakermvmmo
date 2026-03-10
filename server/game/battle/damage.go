package battle

import (
	"math"
	"math/rand"
	"strconv"
	"strings"

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

	// ⑤ Read YEP_DamageCore Note tags from skill meta.
	damageCap := -1     // -1 = no cap
	damageFloor := 0    // default floor is 0
	critMultiplier := 1.5
	flatCritBonus := 0
	canCrit := true
	if ctx.Skill != nil {
		canCrit = ctx.Skill.Damage.Critical
		if m := ctx.Skill.ParsedMeta; m != nil {
			if v := metaInt(m, "Damage Cap"); v > 0 {
				damageCap = v
			}
			if v := metaInt(m, "Damage Floor"); v >= 0 {
				damageFloor = v
			}
			if v := metaPercent(m, "Critical Multiplier"); v > 0 {
				critMultiplier = v / 100.0
			}
			if v := metaInt(m, "Flat Critical"); v > 0 {
				flatCritBonus = v
			}
		}
	}

	// ⑥ Crit check: base crit rate = luk / 1000 (capped at 50%).
	//    Skill must have Damage.Critical = true to be able to crit.
	critRate := math.Min(float64(ctx.Attacker.Luk)/1000.0, 0.5)
	isCrit := canCrit && rand.Float64() < critRate
	if isCrit {
		base = base*critMultiplier + float64(flatCritBonus)
	}

	// ⑦ ±10% variance (RMMV default, skip if skill variance=0).
	variance := 10
	if ctx.Skill != nil {
		variance = ctx.Skill.Damage.Variance
	}
	if variance > 0 {
		varianceFactor := float64(variance) / 100.0
		base *= 1.0 - varianceFactor/2.0 + rand.Float64()*varianceFactor
	}

	// ⑧ Clamp to [damageFloor, damageCap].
	rounded := math.Round(base)
	if rounded < float64(damageFloor) {
		rounded = float64(damageFloor)
	}
	if damageCap > 0 && rounded > float64(damageCap) {
		rounded = float64(damageCap)
	}
	finalDamage := int(rounded)

	result := DamageResult{FinalDamage: finalDamage, IsCrit: isCrit}

	// ⑧ Hook: after_damage_calc
	if AfterDamageCalc != nil {
		AfterDamageCalc(ctx, &result)
	}

	return result
}

// metaInt reads an integer from a ParsedMeta map (colon-format Note tag).
// Returns -1 if the key is absent or the value cannot be parsed.
func metaInt(m map[string]interface{}, key string) int {
	v, ok := m[key]
	if !ok {
		return -1
	}
	switch val := v.(type) {
	case string:
		n, err := strconv.Atoi(strings.TrimSpace(val))
		if err != nil {
			return -1
		}
		return n
	case float64:
		return int(val)
	case int:
		return val
	}
	return -1
}

// metaPercent reads a percentage value from a ParsedMeta map.
// Accepts "200%" or "200" formats; returns the raw number (200 for 200%).
// Returns -1 if absent or unparseable.
func metaPercent(m map[string]interface{}, key string) float64 {
	v, ok := m[key]
	if !ok {
		return -1
	}
	var s string
	switch val := v.(type) {
	case string:
		s = strings.TrimSpace(strings.TrimSuffix(strings.TrimSpace(val), "%"))
	case float64:
		return val
	default:
		return -1
	}
	f, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return -1
	}
	return f
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
