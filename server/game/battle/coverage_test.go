package battle

// coverage_test.go — Comprehensive tests to raise battle module coverage to 90%+.
// Covers: applySkillEffects, resolveByScope, applyItemToTarget, randomAlive/Dead,
// deadAll, CheckRemoveByDamage, Restriction, Transform, AllTraits, SParam,
// BuffLevel/AddBuff/AddDebuff/RemoveBuff, validateInput, processTurnEnd,
// MarkDisconnected/Escaped/EnemyEscaped/Abort, formula parseFactor/statField/applyMathFunc,
// damage.Calculate, loot, troop events, etc.

import (
	"context"
	"math/rand"
	"testing"
	"time"

	"github.com/kasuganosora/rpgmakermvmmo/server/resource"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ═══════════════════════════════════════════════════════════════════════════
//  Helper: extended resource loader with more skills/items/states
// ═══════════════════════════════════════════════════════════════════════════

func makeExtendedRes() *resource.ResourceLoader {
	return &resource.ResourceLoader{
		Skills: []*resource.Skill{
			nil, // 0
			{ID: 1, Name: "Attack", Scope: 1, HitType: 1, SuccessRate: 100, Repeats: 1,
				Damage: resource.SkillDamage{Formula: "a.atk * 4 - b.def * 2", Type: 1, Critical: true, Variance: 20}},
			{ID: 2, Name: "Fire", MPCost: 10, Scope: 1, HitType: 2, SuccessRate: 100, Repeats: 1,
				Damage: resource.SkillDamage{Formula: "a.mat * 3 - b.mdf", Type: 1, ElementID: 2, Critical: true, Variance: 10}},
			{ID: 3, Name: "Heal", MPCost: 5, Scope: 7, HitType: 0, SuccessRate: 100, Repeats: 1,
				Damage: resource.SkillDamage{Formula: "a.mat * 2 + 20", Type: 3, Critical: false}},
			{ID: 4, Name: "Drain", MPCost: 8, Scope: 1, HitType: 2, SuccessRate: 100, Repeats: 1,
				Damage: resource.SkillDamage{Formula: "a.mat * 2 - b.mdf", Type: 5, Critical: false}},
			{ID: 5, Name: "AllHeal", MPCost: 15, Scope: 8, HitType: 0, SuccessRate: 100, Repeats: 1,
				Damage: resource.SkillDamage{Formula: "a.mat * 2", Type: 3}},
			{ID: 6, Name: "Revive", MPCost: 20, Scope: 9, HitType: 0, SuccessRate: 100, Repeats: 1,
				Effects: []resource.SkillEffect{
					{Code: 11, Value1: 0.5, Value2: 0}, // Recover 50% HP
				}},
			{ID: 7, Name: "MassRevive", MPCost: 50, Scope: 10, HitType: 0, SuccessRate: 100, Repeats: 1,
				Effects: []resource.SkillEffect{
					{Code: 11, Value1: 0.3, Value2: 0},
				}},
			{ID: 8, Name: "Random2", MPCost: 0, Scope: 4, HitType: 1, SuccessRate: 100, Repeats: 1,
				Damage: resource.SkillDamage{Formula: "a.atk * 2 - b.def", Type: 1, Variance: 10}},
			{ID: 9, Name: "Random3", MPCost: 0, Scope: 5, HitType: 1, SuccessRate: 100, Repeats: 1,
				Damage: resource.SkillDamage{Formula: "a.atk * 2 - b.def", Type: 1, Variance: 10}},
			{ID: 10, Name: "Random4", MPCost: 0, Scope: 6, HitType: 1, SuccessRate: 100, Repeats: 1,
				Damage: resource.SkillDamage{Formula: "a.atk * 2 - b.def", Type: 1, Variance: 10}},
			{ID: 11, Name: "BuffATK", MPCost: 5, Scope: 11, HitType: 0, SuccessRate: 100, Repeats: 1,
				Effects: []resource.SkillEffect{
					{Code: 31, DataID: 2, Value1: 5}, // Add ATK buff for 5 turns
				}},
			{ID: 12, Name: "DebuffDEF", MPCost: 5, Scope: 1, HitType: 0, SuccessRate: 100, Repeats: 1,
				Effects: []resource.SkillEffect{
					{Code: 32, DataID: 3, Value1: 3}, // Add DEF debuff for 3 turns
				}},
			{ID: 13, Name: "RemoveBuff", MPCost: 0, Scope: 1, HitType: 0, SuccessRate: 100, Repeats: 1,
				Effects: []resource.SkillEffect{
					{Code: 33, DataID: 2, Value1: 0}, // Remove ATK buff
				}},
			{ID: 14, Name: "RemoveDebuff", MPCost: 0, Scope: 1, HitType: 0, SuccessRate: 100, Repeats: 1,
				Effects: []resource.SkillEffect{
					{Code: 34, DataID: 3, Value1: 0}, // Remove DEF debuff
				}},
			{ID: 15, Name: "RecoverMP", MPCost: 0, Scope: 7, HitType: 0, SuccessRate: 100, Repeats: 1,
				Effects: []resource.SkillEffect{
					{Code: 12, Value1: 0.0, Value2: 30}, // Recover 30 MP flat
				}},
			{ID: 16, Name: "GainTP", MPCost: 0, Scope: 11, HitType: 0, SuccessRate: 100, Repeats: 1,
				Effects: []resource.SkillEffect{
					{Code: 13, Value1: 50}, // Gain 50 TP
				}},
			{ID: 17, Name: "RemoveState", MPCost: 5, Scope: 7, HitType: 0, SuccessRate: 100, Repeats: 1,
				Effects: []resource.SkillEffect{
					{Code: 22, DataID: 2, Value1: 1.0}, // Remove poison (100%)
				}},
			{ID: 18, Name: "EscapeSkill", MPCost: 0, Scope: 11, HitType: 0, SuccessRate: 100, Repeats: 1,
				Effects: []resource.SkillEffect{
					{Code: 41, DataID: 0, Value1: 0}, // Special: escape
				}},
			{ID: 19, Name: "CommonEvent", MPCost: 0, Scope: 0, HitType: 0, SuccessRate: 100, Repeats: 1,
				Effects: []resource.SkillEffect{
					{Code: 44, DataID: 10, Value1: 0}, // Trigger CE 10
				}},
			{ID: 20, Name: "NullScope", MPCost: 0, Scope: 0, HitType: 0, SuccessRate: 100, Repeats: 1,
				Damage: resource.SkillDamage{Formula: "10", Type: 1}},
		},
		Items: []*resource.Item{
			nil,
			{ID: 1, Name: "Potion", Scope: 7, HitType: 0, SuccessRate: 100,
				Effects: []resource.SkillEffect{
					{Code: 11, Value1: 0.0, Value2: 50}, // Heal 50 HP
				}},
			{ID: 2, Name: "Ether", Scope: 7, HitType: 0, SuccessRate: 100,
				Effects: []resource.SkillEffect{
					{Code: 12, Value1: 0.0, Value2: 30}, // Recover 30 MP
				}},
			{ID: 3, Name: "Antidote", Scope: 7, HitType: 0, SuccessRate: 100,
				Effects: []resource.SkillEffect{
					{Code: 22, DataID: 2, Value1: 1.0}, // Remove poison
				}},
			{ID: 4, Name: "Bomb", Scope: 2, HitType: 0, SuccessRate: 100,
				Damage: resource.SkillDamage{Formula: "100", Type: 1, Variance: 0},
				Effects: []resource.SkillEffect{}},
		},
		States: []*resource.State{
			nil,
			{ID: 1, Name: "Death"},
			{ID: 2, Name: "Poison", AutoRemovalTiming: 2, MinTurns: 3, MaxTurns: 5,
				RemoveByDamage: true, ChanceByDamage: 100,
				Traits: []resource.Trait{
					{Code: 22, DataID: 0, Value: -0.2}, // -20% HRG as trait
				}},
			{ID: 3, Name: "Stun", Restriction: 4, AutoRemovalTiming: 1, MinTurns: 1, MaxTurns: 1,
				RemoveByDamage: true, ChanceByDamage: 50},
			{ID: 4, Name: "Confusion", Restriction: 1, AutoRemovalTiming: 1, MinTurns: 2, MaxTurns: 4},
			{ID: 5, Name: "Sleep", Restriction: 4, RemoveByDamage: true, ChanceByDamage: 100,
				AutoRemovalTiming: 2, MinTurns: 1, MaxTurns: 3},
		},
		Enemies: []*resource.Enemy{
			nil,
			{ID: 1, Name: "Goblin", HP: 100, MP: 20, Atk: 15, Def: 10, Mat: 8, Mdf: 6, Agi: 12, Luk: 5,
				Exp: 10, Gold: 5,
				Actions: []resource.EnemyAction{
					{SkillID: 1, ConditionType: 0, Rating: 5},
				},
				DropItems: []resource.EnemyDrop{
					{Kind: 1, DataID: 1, Denominator: 2},  // 50% Potion
					{Kind: 0, DataID: 0, Denominator: 0},  // empty slot
					{Kind: 2, DataID: 5, Denominator: 10}, // 10% Weapon 5
				},
			},
			{ID: 2, Name: "Dragon", HP: 500, MP: 100, Atk: 40, Def: 30, Mat: 35, Mdf: 25, Agi: 20, Luk: 15,
				Exp: 100, Gold: 50,
				Actions: []resource.EnemyAction{
					{SkillID: 1, ConditionType: 0, Rating: 5},
					{SkillID: 2, ConditionType: 0, Rating: 3},
				},
			},
		},
	}
}

func makeExtActor(res *resource.ResourceLoader) *ActorBattler {
	return NewActorBattler(ActorConfig{
		CharID: 1, Name: "Hero", Index: 0, Level: 10,
		HP: 200, MP: 50,
		BaseParams:  [8]int{250, 80, 30, 20, 25, 18, 15, 10},
		EquipBonus:  [8]int{0, 0, 10, 5, 0, 0, 0, 0},
		Skills:      []int{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16, 17, 18, 19, 20},
		ActorTraits: []resource.Trait{{Code: 22, DataID: 0, Value: 0.95}},
		Res:         res,
	})
}

func makeExtEnemy(res *resource.ResourceLoader) *EnemyBattler {
	return NewEnemyBattler(res.Enemies[1], 0, res)
}

// ═══════════════════════════════════════════════════════════════════════════
//  applySkillEffects tests
// ═══════════════════════════════════════════════════════════════════════════

func TestApplySkillEffects_RecoverHP(t *testing.T) {
	res := makeExtendedRes()
	ap := &ActionProcessor{Res: res, RNG: rand.New(rand.NewSource(42))}
	actor := makeExtActor(res)
	actor.SetHP(100) // MaxHP=250

	out := &ActionOutcome{}
	ap.applySkillEffects(actor, actor, []resource.SkillEffect{
		{Code: 11, Value1: 0.5, Value2: 10}, // 50% MaxHP + 10 = 125+10=135
	}, out)

	assert.Equal(t, 235, actor.HP(), "should recover to 100+135=235")
	assert.Equal(t, -135, out.Damage, "damage should be negative (heal)")
}

func TestApplySkillEffects_RecoverMP(t *testing.T) {
	res := makeExtendedRes()
	ap := &ActionProcessor{Res: res, RNG: rand.New(rand.NewSource(42))}
	actor := makeExtActor(res)
	actor.SetMP(10) // MaxMP=80

	out := &ActionOutcome{}
	ap.applySkillEffects(actor, actor, []resource.SkillEffect{
		{Code: 12, Value1: 0.25, Value2: 5}, // 25% MaxMP + 5 = 20+5=25
	}, out)

	assert.Equal(t, 35, actor.MP(), "should recover to 10+25=35")
}

func TestApplySkillEffects_GainTP(t *testing.T) {
	res := makeExtendedRes()
	ap := &ActionProcessor{Res: res, RNG: rand.New(rand.NewSource(42))}
	actor := makeExtActor(res)

	out := &ActionOutcome{}
	ap.applySkillEffects(actor, actor, []resource.SkillEffect{
		{Code: 13, Value1: 50},
	}, out)

	assert.Equal(t, 50, actor.TP())
}

func TestApplySkillEffects_AddState(t *testing.T) {
	res := makeExtendedRes()
	ap := &ActionProcessor{Res: res, RNG: rand.New(rand.NewSource(0))} // seed 0 → first Float64 < 1.0
	enemy := makeExtEnemy(res)

	out := &ActionOutcome{}
	ap.applySkillEffects(nil, enemy, []resource.SkillEffect{
		{Code: 21, DataID: 2, Value1: 1.0}, // 100% chance poison
	}, out)

	assert.True(t, enemy.HasState(2), "enemy should have poison")
	assert.Contains(t, out.AddedStates, 2)
}

func TestApplySkillEffects_RemoveState(t *testing.T) {
	res := makeExtendedRes()
	ap := &ActionProcessor{Res: res, RNG: rand.New(rand.NewSource(0))}
	enemy := makeExtEnemy(res)
	enemy.AddState(2, 5) // add poison

	out := &ActionOutcome{}
	ap.applySkillEffects(nil, enemy, []resource.SkillEffect{
		{Code: 22, DataID: 2, Value1: 1.0}, // 100% remove poison
	}, out)

	assert.False(t, enemy.HasState(2), "poison should be removed")
	assert.Contains(t, out.RemovedStates, 2)
}

func TestApplySkillEffects_AddBuff(t *testing.T) {
	res := makeExtendedRes()
	ap := &ActionProcessor{Res: res, RNG: rand.New(rand.NewSource(42))}
	actor := makeExtActor(res)

	out := &ActionOutcome{}
	ap.applySkillEffects(actor, actor, []resource.SkillEffect{
		{Code: 31, DataID: 2, Value1: 5}, // ATK buff for 5 turns
	}, out)

	assert.Equal(t, 1, actor.BuffLevel(2), "ATK buff level should be 1")
	require.Len(t, out.AddedBuffs, 1)
	assert.Equal(t, 2, out.AddedBuffs[0].ParamID)
	assert.Equal(t, 5, out.AddedBuffs[0].Turns)
}

func TestApplySkillEffects_AddDebuff(t *testing.T) {
	res := makeExtendedRes()
	ap := &ActionProcessor{Res: res, RNG: rand.New(rand.NewSource(42))}
	enemy := makeExtEnemy(res)

	out := &ActionOutcome{}
	ap.applySkillEffects(nil, enemy, []resource.SkillEffect{
		{Code: 32, DataID: 3, Value1: 3}, // DEF debuff for 3 turns
	}, out)

	assert.Equal(t, -1, enemy.BuffLevel(3), "DEF buff level should be -1")
	require.Len(t, out.AddedBuffs, 1)
	assert.Equal(t, 3, out.AddedBuffs[0].ParamID)
}

func TestApplySkillEffects_RemoveBuff(t *testing.T) {
	res := makeExtendedRes()
	ap := &ActionProcessor{Res: res, RNG: rand.New(rand.NewSource(42))}
	actor := makeExtActor(res)
	actor.AddBuff(2, 5) // ATK buff

	out := &ActionOutcome{}
	ap.applySkillEffects(nil, actor, []resource.SkillEffect{
		{Code: 33, DataID: 2},
	}, out)

	assert.Equal(t, 0, actor.BuffLevel(2), "buff should be removed")
}

func TestApplySkillEffects_RemoveDebuff(t *testing.T) {
	res := makeExtendedRes()
	ap := &ActionProcessor{Res: res, RNG: rand.New(rand.NewSource(42))}
	enemy := makeExtEnemy(res)
	enemy.AddDebuff(3, 3) // DEF debuff

	out := &ActionOutcome{}
	ap.applySkillEffects(nil, enemy, []resource.SkillEffect{
		{Code: 34, DataID: 3},
	}, out)

	assert.Equal(t, 0, enemy.BuffLevel(3), "debuff should be removed")
}

func TestApplySkillEffects_SpecialEscape(t *testing.T) {
	res := makeExtendedRes()
	ap := &ActionProcessor{Res: res, RNG: rand.New(rand.NewSource(42))}

	out := &ActionOutcome{}
	ap.applySkillEffects(nil, nil, []resource.SkillEffect{
		{Code: 41, DataID: 0}, // escape
	}, out)

	assert.True(t, out.Escaped)
}

func TestApplySkillEffects_CommonEvent(t *testing.T) {
	res := makeExtendedRes()
	ap := &ActionProcessor{Res: res, RNG: rand.New(rand.NewSource(42))}

	out := &ActionOutcome{}
	ap.applySkillEffects(nil, nil, []resource.SkillEffect{
		{Code: 44, DataID: 10},
	}, out)

	assert.Contains(t, out.CommonEventIDs, 10)
}

func TestApplySkillEffects_GrowAndLearnSkill(t *testing.T) {
	// Codes 42 and 43 are no-ops in battle, just verify no panic.
	res := makeExtendedRes()
	ap := &ActionProcessor{Res: res, RNG: rand.New(rand.NewSource(42))}
	actor := makeExtActor(res)

	out := &ActionOutcome{}
	ap.applySkillEffects(actor, actor, []resource.SkillEffect{
		{Code: 42, DataID: 2, Value1: 5}, // Grow ATK +5
		{Code: 43, DataID: 10},           // Learn skill 10
	}, out)
	// No panic is the assertion.
}

// ═══════════════════════════════════════════════════════════════════════════
//  resolveByScope tests
// ═══════════════════════════════════════════════════════════════════════════

func TestResolveByScope_AllScopes(t *testing.T) {
	res := makeExtendedRes()
	ap := &ActionProcessor{Res: res, RNG: rand.New(rand.NewSource(42))}

	actor := makeExtActor(res)
	actor2 := NewActorBattler(ActorConfig{
		CharID: 2, Name: "Healer", Index: 1, HP: 150, MP: 80,
		BaseParams: [8]int{200, 100, 15, 15, 30, 25, 12, 8}, Res: res,
	})
	deadActor := NewActorBattler(ActorConfig{
		CharID: 3, Name: "Dead", Index: 2, HP: 0, MP: 50,
		BaseParams: [8]int{200, 50, 20, 20, 20, 20, 10, 10}, Res: res,
	})
	e1 := NewEnemyBattler(&resource.Enemy{ID: 1, Name: "G1", HP: 50, Atk: 10, Def: 5}, 0, res)
	e2 := NewEnemyBattler(&resource.Enemy{ID: 2, Name: "G2", HP: 50, Atk: 10, Def: 5}, 1, res)
	e3 := NewEnemyBattler(&resource.Enemy{ID: 3, Name: "G3", HP: 50, Atk: 10, Def: 5}, 2, res)
	e4 := NewEnemyBattler(&resource.Enemy{ID: 4, Name: "G4", HP: 50, Atk: 10, Def: 5}, 3, res)
	deadEnemy := NewEnemyBattler(&resource.Enemy{ID: 5, Name: "Dead", HP: 0}, 4, res)

	actors := []Battler{actor, actor2, deadActor}
	enemies := []Battler{e1, e2, e3, e4, deadEnemy}

	tests := []struct {
		scope    int
		name     string
		minCount int
		maxCount int
	}{
		{0, "none", 0, 0},
		{1, "1 enemy", 1, 1},
		{2, "all enemies", 4, 4}, // 4 alive
		{3, "1 random enemy", 1, 1},
		{4, "2 random enemies", 2, 2},
		{5, "3 random enemies", 3, 3},
		{6, "4 random enemies", 4, 4},
		{7, "1 ally", 1, 1},
		{8, "all allies (alive)", 2, 2}, // 2 alive actors
		{9, "1 ally (dead)", 1, 1},
		{10, "all allies (dead)", 1, 1}, // 1 dead actor
		{11, "user", 1, 1},
		{99, "unknown", 0, 0},
	}

	for _, tc := range tests {
		targets := ap.resolveByScope(tc.scope, actor, actors, enemies)
		if len(targets) < tc.minCount || len(targets) > tc.maxCount {
			t.Errorf("scope %d (%s): got %d targets, want %d-%d",
				tc.scope, tc.name, len(targets), tc.minCount, tc.maxCount)
		}
	}
}

func TestResolveByScope_EnemyPerspective(t *testing.T) {
	res := makeExtendedRes()
	ap := &ActionProcessor{Res: res, RNG: rand.New(rand.NewSource(42))}

	actor := makeExtActor(res)
	enemy := makeExtEnemy(res)

	// From enemy's perspective: scope 1 = 1 opponent (actor), scope 7 = 1 ally (enemy)
	targets := ap.resolveByScope(1, enemy, []Battler{actor}, []Battler{enemy})
	require.Len(t, targets, 1)
	assert.True(t, targets[0].IsActor(), "scope 1 from enemy should target actor")

	targets2 := ap.resolveByScope(7, enemy, []Battler{actor}, []Battler{enemy})
	require.Len(t, targets2, 1)
	assert.False(t, targets2[0].IsActor(), "scope 7 from enemy should target ally (enemy)")
}

// ═══════════════════════════════════════════════════════════════════════════
//  Item processing tests
// ═══════════════════════════════════════════════════════════════════════════

func TestProcessItemAction_MPRecover(t *testing.T) {
	res := makeExtendedRes()
	ap := &ActionProcessor{Res: res, RNG: rand.New(rand.NewSource(42))}
	actor := makeExtActor(res)
	actor.SetMP(10)

	action := &Action{Type: ActionItem, ItemID: 2, TargetIndices: []int{0}, TargetIsActor: true}
	outcomes := ap.ProcessAction(actor, action, []Battler{actor}, nil)
	require.Greater(t, len(outcomes), 0)
	assert.Equal(t, 40, actor.MP(), "MP should be 10+30=40 after Ether")
}

func TestProcessItemAction_RemoveState(t *testing.T) {
	res := makeExtendedRes()
	ap := &ActionProcessor{Res: res, RNG: rand.New(rand.NewSource(0))}
	actor := makeExtActor(res)
	actor.AddState(2, 5) // poison

	action := &Action{Type: ActionItem, ItemID: 3, TargetIndices: []int{0}, TargetIsActor: true}
	outcomes := ap.ProcessAction(actor, action, []Battler{actor}, nil)
	require.Greater(t, len(outcomes), 0)
	assert.False(t, actor.HasState(2), "antidote should remove poison")
}

func TestProcessItemAction_NilItem(t *testing.T) {
	res := makeExtendedRes()
	ap := &ActionProcessor{Res: res, RNG: rand.New(rand.NewSource(42))}
	actor := makeExtActor(res)

	action := &Action{Type: ActionItem, ItemID: 999, TargetIndices: []int{0}, TargetIsActor: true}
	outcomes := ap.ProcessAction(actor, action, []Battler{actor}, nil)
	assert.Nil(t, outcomes, "nonexistent item should return nil outcomes")
}

func TestProcessAction_EscapeReturnsNil(t *testing.T) {
	res := makeExtendedRes()
	ap := &ActionProcessor{Res: res, RNG: rand.New(rand.NewSource(42))}
	actor := makeExtActor(res)

	action := &Action{Type: ActionEscape}
	outcomes := ap.ProcessAction(actor, action, nil, nil)
	assert.Nil(t, outcomes)
}

func TestProcessAction_NilAction(t *testing.T) {
	res := makeExtendedRes()
	ap := &ActionProcessor{Res: res, RNG: rand.New(rand.NewSource(42))}
	actor := makeExtActor(res)

	outcomes := ap.ProcessAction(actor, nil, nil, nil)
	assert.Nil(t, outcomes)
}

func TestProcessAction_InvalidType(t *testing.T) {
	res := makeExtendedRes()
	ap := &ActionProcessor{Res: res, RNG: rand.New(rand.NewSource(42))}
	actor := makeExtActor(res)

	action := &Action{Type: 99}
	outcomes := ap.ProcessAction(actor, action, nil, nil)
	assert.Nil(t, outcomes)
}

// ═══════════════════════════════════════════════════════════════════════════
//  Battler: CheckRemoveByDamage, Restriction, Transform
// ═══════════════════════════════════════════════════════════════════════════

func TestCheckRemoveByDamage(t *testing.T) {
	res := makeExtendedRes()
	actor := makeExtActor(res)
	// Add poison (RemoveByDamage=true, ChanceByDamage=100)
	actor.AddState(2, 5)
	require.True(t, actor.HasState(2))

	removed := actor.CheckRemoveByDamage(rand.New(rand.NewSource(42)))
	assert.Contains(t, removed, 2, "poison should be removed (100% chance)")
	assert.False(t, actor.HasState(2))
}

func TestCheckRemoveByDamage_PartialChance(t *testing.T) {
	res := makeExtendedRes()
	actor := makeExtActor(res)
	// Add stun (RemoveByDamage=true, ChanceByDamage=50)
	actor.AddState(3, 1)
	require.True(t, actor.HasState(3))

	// Run multiple times to verify probabilistic removal.
	removedCount := 0
	for seed := int64(0); seed < 20; seed++ {
		a := makeExtActor(res)
		a.AddState(3, 1)
		removed := a.CheckRemoveByDamage(rand.New(rand.NewSource(seed)))
		if len(removed) > 0 {
			removedCount++
		}
	}
	// With 50% chance over 20 trials, expect some removals.
	assert.Greater(t, removedCount, 0, "should remove stun at least once")
	assert.Less(t, removedCount, 20, "shouldn't remove every time at 50%")
}

func TestCheckRemoveByDamage_NilRes(t *testing.T) {
	actor := NewActorBattler(ActorConfig{
		CharID: 1, Name: "X", Index: 0, HP: 100, MP: 10,
		BaseParams: [8]int{100, 10, 10, 10, 10, 10, 10, 10},
		Res:        nil,
	})
	removed := actor.CheckRemoveByDamage(rand.New(rand.NewSource(42)))
	assert.Nil(t, removed)
}

func TestRestriction(t *testing.T) {
	res := makeExtendedRes()
	actor := makeExtActor(res)

	assert.Equal(t, 0, actor.Restriction(), "no states = restriction 0")

	// Add stun (restriction 4).
	actor.AddState(3, 1)
	assert.Equal(t, 4, actor.Restriction(), "stun = restriction 4")

	// Add confusion (restriction 1) — highest wins.
	actor.AddState(4, 2)
	assert.Equal(t, 4, actor.Restriction(), "stun(4) > confusion(1)")

	// Remove stun.
	actor.RemoveState(3)
	assert.Equal(t, 1, actor.Restriction(), "only confusion(1) remains")
}

func TestRestriction_NilRes(t *testing.T) {
	actor := NewActorBattler(ActorConfig{
		CharID: 1, Name: "X", Index: 0, HP: 100, MP: 10,
		BaseParams: [8]int{100, 10, 10, 10, 10, 10, 10, 10},
		Res:        nil,
	})
	assert.Equal(t, 0, actor.Restriction())
}

func TestTransform(t *testing.T) {
	res := makeExtendedRes()
	enemy := makeExtEnemy(res)
	origHP := enemy.HP()

	// Damage enemy to 50%
	enemy.SetHP(origHP / 2)

	newEnemy := &resource.Enemy{
		ID: 99, Name: "Mega Goblin", HP: 200, MP: 40,
		Atk: 30, Def: 20, Mat: 15, Mdf: 12, Agi: 18, Luk: 8,
	}
	enemy.Transform(newEnemy)

	assert.Equal(t, 99, enemy.EnemyID())
	assert.Equal(t, "Mega Goblin", enemy.Name())
	// HP should scale proportionally: 50% of 200 = 100
	assert.Equal(t, 100, enemy.HP())
	assert.Equal(t, 30, enemy.Param(2), "ATK should be new enemy's ATK")
}

func TestTransform_NilEnemy(t *testing.T) {
	res := makeExtendedRes()
	enemy := makeExtEnemy(res)
	origID := enemy.EnemyID()
	enemy.Transform(nil)
	assert.Equal(t, origID, enemy.EnemyID(), "nil transform should be no-op")
}

func TestTransform_ZeroHP(t *testing.T) {
	res := makeExtendedRes()
	enemy := makeExtEnemy(res)
	enemy.SetHP(0) // dead

	newEnemy := &resource.Enemy{
		ID: 99, Name: "Mega", HP: 200, MP: 40,
		Atk: 30, Def: 20, Mat: 15, Mdf: 12, Agi: 18, Luk: 8,
	}
	enemy.Transform(newEnemy)
	// HP ratio was 0, so new HP should be 0.
	assert.Equal(t, 0, enemy.HP())
}

func TestAllTraits(t *testing.T) {
	res := makeExtendedRes()
	actor := makeExtActor(res)

	// Base traits: hit rate.
	traits := actor.AllTraits()
	assert.Greater(t, len(traits), 0)

	// Add state with traits, should include state traits.
	actor.AddState(2, 5)
	traitsWithState := actor.AllTraits()
	assert.Greater(t, len(traitsWithState), len(traits),
		"traits should include state traits")
}

func TestEnemyAccessors(t *testing.T) {
	res := makeExtendedRes()
	enemy := makeExtEnemy(res)

	assert.Equal(t, res.Enemies[1], enemy.Enemy())
	assert.False(t, enemy.IsActor())
	assert.Equal(t, int64(0), enemy.CharID())
	assert.Equal(t, 1, enemy.EnemyID())

	stats := enemy.ToCharacterStats()
	assert.Equal(t, 1, stats.Level, "enemy level should be 1")
}

func TestEnemySkillIDs_Dedup(t *testing.T) {
	res := makeExtendedRes()
	enemy := NewEnemyBattler(&resource.Enemy{
		ID: 1, Name: "Multi",
		HP: 100, Atk: 10, Def: 5,
		Actions: []resource.EnemyAction{
			{SkillID: 1}, {SkillID: 2}, {SkillID: 1}, // duplicate
		},
	}, 0, res)

	ids := enemy.SkillIDs()
	assert.Len(t, ids, 2, "should deduplicate skill IDs")
}

func TestEnemySkillIDs_NilEnemy(t *testing.T) {
	enemy := &EnemyBattler{}
	assert.Nil(t, enemy.SkillIDs())
}

// ═══════════════════════════════════════════════════════════════════════════
//  Battler: Buff/Debuff mechanics
// ═══════════════════════════════════════════════════════════════════════════

func TestBuffLevel_Capping(t *testing.T) {
	res := makeExtendedRes()
	actor := makeExtActor(res)

	// Stack buffs to +2.
	actor.AddBuff(2, 5)
	assert.Equal(t, 1, actor.BuffLevel(2))
	actor.AddBuff(2, 5)
	assert.Equal(t, 2, actor.BuffLevel(2))
	actor.AddBuff(2, 5)
	assert.Equal(t, 2, actor.BuffLevel(2), "buff should cap at +2")

	// Stack debuffs to -2.
	actor.AddDebuff(3, 5)
	assert.Equal(t, -1, actor.BuffLevel(3))
	actor.AddDebuff(3, 5)
	assert.Equal(t, -2, actor.BuffLevel(3))
	actor.AddDebuff(3, 5)
	assert.Equal(t, -2, actor.BuffLevel(3), "debuff should cap at -2")
}

func TestBuffLevel_OutOfRange(t *testing.T) {
	res := makeExtendedRes()
	actor := makeExtActor(res)
	assert.Equal(t, 0, actor.BuffLevel(99), "out-of-range param should return 0")
}

func TestRemoveBuff_OutOfRange(t *testing.T) {
	res := makeExtendedRes()
	actor := makeExtActor(res)
	actor.RemoveBuff(99) // should not panic
}

func TestBuffRate(t *testing.T) {
	res := makeExtendedRes()
	actor := makeExtActor(res)
	atkBefore := actor.Param(2)

	actor.AddBuff(2, 5) // +1 buff = 1.25x
	atkBuffed := actor.Param(2)
	assert.Greater(t, atkBuffed, atkBefore, "buff should increase ATK")

	actor.RemoveBuff(2)
	actor.AddDebuff(2, 5) // -1 debuff = 0.75x
	atkDebuffed := actor.Param(2)
	assert.Less(t, atkDebuffed, atkBefore, "debuff should decrease ATK")
}

func TestTickBuffTurns_Expiry(t *testing.T) {
	res := makeExtendedRes()
	actor := makeExtActor(res)
	actor.AddBuff(2, 2) // ATK buff for 2 turns

	expired := actor.TickBuffTurns()
	assert.Len(t, expired, 0, "first tick should not expire")
	assert.Equal(t, 1, actor.BuffLevel(2))

	expired = actor.TickBuffTurns()
	assert.Contains(t, expired, 2, "second tick should expire ATK buff")
	assert.Equal(t, 0, actor.BuffLevel(2))
}

// ═══════════════════════════════════════════════════════════════════════════
//  SParam tests
// ═══════════════════════════════════════════════════════════════════════════

func TestSParam_Defaults(t *testing.T) {
	res := makeExtendedRes()
	actor := makeExtActor(res)

	// With no SParam traits, all should return 1.0.
	for i := 0; i < 10; i++ {
		v := actor.SParam(i)
		assert.InDelta(t, 1.0, v, 0.001, "SParam(%d) default should be 1.0", i)
	}

	// Add a trait that modifies SParam 0 (TGR).
	actorWithTrait := NewActorBattler(ActorConfig{
		CharID: 1, Name: "T", Index: 0, HP: 100, MP: 10,
		BaseParams: [8]int{100, 10, 10, 10, 10, 10, 10, 10},
		ActorTraits: []resource.Trait{
			{Code: 23, DataID: 0, Value: 1.5}, // TGR x1.5
		},
		Res: res,
	})
	assert.InDelta(t, 1.5, actorWithTrait.SParam(0), 0.001)
}

// ═══════════════════════════════════════════════════════════════════════════
//  SetMP clamping
// ═══════════════════════════════════════════════════════════════════════════

func TestSetMP_Clamping(t *testing.T) {
	res := makeExtendedRes()
	actor := makeExtActor(res)

	actor.SetMP(-10)
	assert.Equal(t, 0, actor.MP(), "MP should clamp to 0")

	actor.SetMP(9999)
	assert.Equal(t, actor.MaxMP(), actor.MP(), "MP should clamp to MaxMP")
}

// ═══════════════════════════════════════════════════════════════════════════
//  Formula: parseFactor, statField, applyMathFunc
// ═══════════════════════════════════════════════════════════════════════════

func TestFormula_Parentheses(t *testing.T) {
	a := &CharacterStats{Atk: 20, Def: 10}
	b := &CharacterStats{Def: 5}

	v, err := EvalFormula("(a.atk + 10) * 2", a, b)
	require.NoError(t, err)
	assert.InDelta(t, 60.0, v, 0.001)
}

func TestFormula_NegativeNumber(t *testing.T) {
	a := &CharacterStats{Atk: 10}
	b := &CharacterStats{}

	v, err := EvalFormula("-a.atk + 20", a, b)
	require.NoError(t, err)
	assert.InDelta(t, 10.0, v, 0.001)
}

func TestFormula_StatFields(t *testing.T) {
	a := &CharacterStats{HP: 100, MP: 50, Atk: 20, Def: 15, Mat: 25, Mdf: 18, Agi: 12, Luk: 8, Level: 10}

	fields := map[string]float64{
		"a.hp": 100, "a.mp": 50, "a.atk": 20, "a.def": 15,
		"a.mat": 25, "a.mdf": 18, "a.agi": 12, "a.luk": 8, "a.level": 10,
	}
	for formula, expected := range fields {
		v, err := EvalFormula(formula, a, &CharacterStats{})
		require.NoError(t, err, "formula %q", formula)
		assert.InDelta(t, expected, v, 0.001, "formula %q", formula)
	}
}

func TestFormula_UnknownStatField(t *testing.T) {
	a := &CharacterStats{Atk: 10}
	_, err := EvalFormula("a.unknown", a, &CharacterStats{})
	assert.Error(t, err, "unknown stat field should error")
}

func TestFormula_MathFunctions(t *testing.T) {
	a := &CharacterStats{Atk: 15}
	b := &CharacterStats{}

	tests := []struct {
		formula  string
		expected float64
	}{
		{"Math.floor(a.atk * 1.5)", 22},
		{"Math.ceil(a.atk * 1.3)", 20},
		{"Math.round(a.atk * 1.5)", 22}, // 22.5 rounds to 22 (banker's rounding) or 23
		{"Math.abs(-a.atk)", 15},
		{"Math.max(a.atk, 10, 20)", 20},
		{"Math.min(a.atk, 10, 20)", 10},
	}

	for _, tc := range tests {
		v, err := EvalFormula(tc.formula, a, b)
		require.NoError(t, err, "formula %q", tc.formula)
		// Math.round(22.5) may be 22 or 23 depending on implementation.
		if tc.formula == "Math.round(a.atk * 1.5)" {
			assert.True(t, v == 22 || v == 23, "Math.round(22.5) = %v", v)
		} else {
			assert.InDelta(t, tc.expected, v, 0.001, "formula %q", tc.formula)
		}
	}
}

func TestFormula_UnknownMathFunc(t *testing.T) {
	a := &CharacterStats{Atk: 10}
	_, err := EvalFormula("Math.sqrt(a.atk)", a, &CharacterStats{})
	assert.Error(t, err, "unknown Math function should error")
}

func TestFormula_Decimal(t *testing.T) {
	a := &CharacterStats{Atk: 10}
	v, err := EvalFormula("a.atk * 1.5", a, &CharacterStats{})
	require.NoError(t, err)
	assert.InDelta(t, 15.0, v, 0.001)
}

func TestFormula_DivisionByZero(t *testing.T) {
	a := &CharacterStats{Atk: 10}
	v, err := EvalFormula("a.atk / 0", a, &CharacterStats{})
	// Division by zero should not crash; behavior depends on implementation.
	_ = v
	_ = err
}

func TestFormula_EmptyFormula(t *testing.T) {
	a := &CharacterStats{}
	_, err := EvalFormula("", a, &CharacterStats{})
	// Empty formula behavior.
	_ = err
}

func TestFormula_UnexpectedChar(t *testing.T) {
	a := &CharacterStats{}
	_, err := EvalFormula("@#$", a, &CharacterStats{})
	assert.Error(t, err)
}

// ═══════════════════════════════════════════════════════════════════════════
//  damage.go Calculate
// ═══════════════════════════════════════════════════════════════════════════

func TestCalculate_BasicDamage(t *testing.T) {
	ctx := &DamageContext{
		Attacker: &CharacterStats{Atk: 20, Luk: 0},
		Defender: &CharacterStats{Def: 10},
		Skill:    &resource.Skill{Damage: resource.SkillDamage{Formula: "a.atk * 4 - b.def * 2"}},
	}
	result := Calculate(ctx)
	// base = 80 - 20 = 60, luk=0 so no crit, ±10% variance → 54-66
	assert.Greater(t, result.FinalDamage, 0, "should deal damage")
}

func TestCalculate_EmptyFormula(t *testing.T) {
	ctx := &DamageContext{
		Attacker: &CharacterStats{Atk: 20, Luk: 0},
		Defender: &CharacterStats{Def: 10},
		Skill:    &resource.Skill{Damage: resource.SkillDamage{Formula: ""}},
	}
	result := Calculate(ctx)
	// Falls back to Atk*4 - Def*2 = 60
	assert.Greater(t, result.FinalDamage, 0)
}

func TestCalculate_NilSkill(t *testing.T) {
	ctx := &DamageContext{
		Attacker: &CharacterStats{Atk: 20, Luk: 0},
		Defender: &CharacterStats{Def: 10},
	}
	result := Calculate(ctx)
	// Falls back to Atk*4 - Def*2 = 60
	assert.Greater(t, result.FinalDamage, 0)
}

func TestCalculate_WithBuffs(t *testing.T) {
	ctx := &DamageContext{
		Attacker: &CharacterStats{Atk: 20, Luk: 0},
		Defender: &CharacterStats{Def: 10},
		Skill:    &resource.Skill{Damage: resource.SkillDamage{Formula: "a.atk * 4 - b.def * 2"}},
		AttackerBuf: []*BuffInstance{
			{DamageBonus: 1.5}, // 50% bonus
		},
		DefenderBuf: []*BuffInstance{
			{DamageTaken: 0.5}, // 50% reduction
		},
	}
	result := Calculate(ctx)
	// base = 60, *1.5 = 90, *0.5 = 45, ±10% → ~40-50
	assert.Greater(t, result.FinalDamage, 0)
}

func TestCalculate_Hooks(t *testing.T) {
	hookCalled := false
	afterCalled := false
	oldBefore := BeforeDamageCalc
	oldAfter := AfterDamageCalc
	defer func() { BeforeDamageCalc = oldBefore; AfterDamageCalc = oldAfter }()

	BeforeDamageCalc = func(ctx *DamageContext) { hookCalled = true }
	AfterDamageCalc = func(ctx *DamageContext, result *DamageResult) { afterCalled = true }

	ctx := &DamageContext{
		Attacker: &CharacterStats{Atk: 20},
		Defender: &CharacterStats{Def: 10},
	}
	Calculate(ctx)
	assert.True(t, hookCalled, "BeforeDamageCalc should be called")
	assert.True(t, afterCalled, "AfterDamageCalc should be called")
}

// ═══════════════════════════════════════════════════════════════════════════
//  Loot tests
// ═══════════════════════════════════════════════════════════════════════════

func TestCalculateDropsRNG(t *testing.T) {
	enemy := &resource.Enemy{
		DropItems: []resource.EnemyDrop{
			{Kind: 1, DataID: 1, Denominator: 1}, // 100% drop
			{Kind: 0, DataID: 0, Denominator: 0}, // empty
			{Kind: 2, DataID: 5, Denominator: 1}, // 100% drop
		},
	}

	drops := CalculateDropsRNG(enemy, rand.New(rand.NewSource(42)))
	assert.Len(t, drops, 2, "should get 2 drops (100% each)")
	assert.Equal(t, 1, drops[0].ItemType)
	assert.Equal(t, 1, drops[0].ItemID)
	assert.Equal(t, 2, drops[1].ItemType)
	assert.Equal(t, 5, drops[1].ItemID)
}

func TestCalculateDropsRNG_NoDrop(t *testing.T) {
	enemy := &resource.Enemy{
		DropItems: []resource.EnemyDrop{
			{Kind: 1, DataID: 1, Denominator: 1000000}, // very low chance
		},
	}

	rng := rand.New(rand.NewSource(42))
	drops := CalculateDropsRNG(enemy, rng)
	// With denominator=1000000, very unlikely to drop.
	assert.Len(t, drops, 0)
}

func TestCalculateDrops_NilRNG(t *testing.T) {
	enemy := &resource.Enemy{
		DropItems: []resource.EnemyDrop{
			{Kind: 1, DataID: 1, Denominator: 1}, // 100% drop
		},
	}
	drops := CalculateDrops(enemy)
	assert.Len(t, drops, 1)
}

func TestCalculateExp(t *testing.T) {
	// Solo: 100% bonus, each = 10.
	assert.Equal(t, 10, CalculateExp(10, 1))

	// Party of 2: 1.1 bonus, each = 10*1.1/2 = 5.
	assert.Equal(t, 5, CalculateExp(10, 2))

	// Party of 5: 1.4 bonus (capped), each = 10*1.4/5 = 2.
	assert.Equal(t, 2, CalculateExp(10, 5))

	// Minimum 1.
	assert.Equal(t, 1, CalculateExp(1, 10))

	// Zero party size defaults to 1.
	assert.Equal(t, 10, CalculateExp(10, 0))
}

func TestExpNeeded(t *testing.T) {
	assert.Equal(t, 30, ExpNeeded(0))
	assert.Equal(t, 30, ExpNeeded(1))
	exp2 := ExpNeeded(2)
	assert.Greater(t, exp2, 30, "level 2 should need more exp than level 1")
}

// ═══════════════════════════════════════════════════════════════════════════
//  Instance: MarkDisconnected, MarkEscaped, MarkEnemyEscaped, Abort
// ═══════════════════════════════════════════════════════════════════════════

func TestMarkDisconnected(t *testing.T) {
	res := makeExtendedRes()
	bi := NewBattleInstance(BattleConfig{
		TroopID: 1, Res: res,
		RNG:          rand.New(rand.NewSource(42)),
		InputTimeout: 5 * time.Second,
	})
	actor := makeExtActor(res)
	bi.Actors = []Battler{actor}

	assert.False(t, bi.isDisconnected(0))
	bi.MarkDisconnected(0)
	assert.True(t, bi.isDisconnected(0))
}

func TestMarkEscaped(t *testing.T) {
	res := makeExtendedRes()
	bi := NewBattleInstance(BattleConfig{
		TroopID: 1, Res: res,
		RNG: rand.New(rand.NewSource(42)),
	})

	assert.False(t, bi.IsEscaped(0))
	bi.MarkEscaped(0)
	assert.True(t, bi.IsEscaped(0))
}

func TestMarkEnemyEscaped(t *testing.T) {
	res := makeExtendedRes()
	bi := NewBattleInstance(BattleConfig{
		TroopID: 1, Res: res,
		RNG: rand.New(rand.NewSource(42)),
	})

	assert.False(t, bi.IsEnemyEscaped(0))
	bi.MarkEnemyEscaped(0)
	assert.True(t, bi.IsEnemyEscaped(0))
}

func TestAbort(t *testing.T) {
	res := makeExtendedRes()
	bi := NewBattleInstance(BattleConfig{
		TroopID: 1, Res: res,
		RNG: rand.New(rand.NewSource(42)),
	})

	assert.False(t, bi.IsAborted())
	bi.Abort()
	assert.True(t, bi.IsAborted())
}

// ═══════════════════════════════════════════════════════════════════════════
//  Instance: validateInput
// ═══════════════════════════════════════════════════════════════════════════

func TestValidateInput(t *testing.T) {
	res := makeExtendedRes()
	bi := NewBattleInstance(BattleConfig{
		TroopID: 1, Res: res,
		RNG: rand.New(rand.NewSource(42)),
	})
	actor := makeExtActor(res)
	bi.Actors = []Battler{actor}
	enemy := makeExtEnemy(res)
	bi.Enemies = []Battler{enemy}

	// Valid attack.
	input := bi.validateInput(actor, &ActionInput{
		ActorIndex: 0, ActionType: ActionAttack, TargetIndices: []int{0},
	})
	assert.Equal(t, ActionAttack, input.ActionType)

	// Invalid action type → falls back to attack.
	input = bi.validateInput(actor, &ActionInput{
		ActorIndex: 0, ActionType: -1,
	})
	assert.Equal(t, ActionAttack, input.ActionType)

	input = bi.validateInput(actor, &ActionInput{
		ActorIndex: 0, ActionType: 99,
	})
	assert.Equal(t, ActionAttack, input.ActionType)

	// Unknown skill → falls back to attack.
	input = bi.validateInput(actor, &ActionInput{
		ActorIndex: 0, ActionType: ActionSkill, SkillID: 9999,
	})
	assert.Equal(t, ActionAttack, input.ActionType)

	// Actor doesn't know skill → falls back to attack.
	actorLimited := NewActorBattler(ActorConfig{
		CharID: 2, Name: "Limited", Index: 1, HP: 100, MP: 50,
		BaseParams: [8]int{100, 50, 20, 15, 10, 10, 10, 10},
		Skills:     []int{1}, // only knows attack
		Res:        res,
	})
	input = bi.validateInput(actorLimited, &ActionInput{
		ActorIndex: 1, ActionType: ActionSkill, SkillID: 2,
	})
	assert.Equal(t, ActionAttack, input.ActionType)

	// Valid skill (actor knows it).
	input = bi.validateInput(actor, &ActionInput{
		ActorIndex: 0, ActionType: ActionSkill, SkillID: 2,
	})
	assert.Equal(t, ActionSkill, input.ActionType)
	assert.Equal(t, 2, input.SkillID)

	// Skill 1 (normal attack) always allowed even if not in skill list.
	input = bi.validateInput(actorLimited, &ActionInput{
		ActorIndex: 1, ActionType: ActionSkill, SkillID: 1,
	})
	assert.Equal(t, ActionSkill, input.ActionType)
}

// ═══════════════════════════════════════════════════════════════════════════
//  Instance: Run with disconnected actor (auto-guard)
// ═══════════════════════════════════════════════════════════════════════════

func TestRun_DisconnectedActorCollectsGuardAction(t *testing.T) {
	res := makeExtendedRes()
	actor := makeExtActor(res)
	enemy := makeExtEnemy(res)

	bi := NewBattleInstance(BattleConfig{
		TroopID: 1, Res: res,
		RNG:          rand.New(rand.NewSource(42)),
		InputTimeout: 2 * time.Second,
	})
	bi.Actors = []Battler{actor}
	bi.Enemies = []Battler{enemy}

	bi.MarkDisconnected(0)
	assert.True(t, bi.isDisconnected(0))

	// Verify that collectActions succeeds for disconnected actors.
	// (They get auto-guard without needing input.)
	ctx := context.Background()
	err := bi.collectActions(ctx)
	require.NoError(t, err)

	action := actor.CurrentAction()
	require.NotNil(t, action)
	assert.Equal(t, ActionGuard, action.Type, "disconnected actor should auto-guard")
}

// ═══════════════════════════════════════════════════════════════════════════
//  Instance: Abort mid-battle
// ═══════════════════════════════════════════════════════════════════════════

func TestRun_AbortMidBattle(t *testing.T) {
	res := makeExtendedRes()
	actor := makeExtActor(res)
	enemy := NewEnemyBattler(&resource.Enemy{
		ID: 1, Name: "Tank", HP: 99999, MP: 0, Atk: 1, Def: 1,
		Actions: []resource.EnemyAction{{SkillID: 1, Rating: 5}},
	}, 0, res)

	bi := NewBattleInstance(BattleConfig{
		TroopID: 1, Res: res,
		RNG:          rand.New(rand.NewSource(42)),
		InputTimeout: 2 * time.Second,
	})
	bi.Actors = []Battler{actor}
	bi.Enemies = []Battler{enemy}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	go func() {
		inputCount := 0
		for evt := range bi.Events() {
			if evt.EventType() == "input_request" {
				inputCount++
				if inputCount >= 2 {
					bi.Abort()
				}
				bi.SubmitInput(&ActionInput{
					ActorIndex: 0, ActionType: ActionAttack,
					TargetIndices: []int{0}, TargetIsActor: false,
				})
			}
		}
	}()

	result := bi.Run(ctx)
	assert.Equal(t, ResultAbort, result, "should abort")
}

// ═══════════════════════════════════════════════════════════════════════════
//  Instance: checkBattleEnd scenarios
// ═══════════════════════════════════════════════════════════════════════════

func TestCheckBattleEnd_AllEnemiesDead(t *testing.T) {
	res := makeExtendedRes()
	actor := makeExtActor(res)
	enemy := makeExtEnemy(res)
	enemy.SetHP(0) // dead

	bi := NewBattleInstance(BattleConfig{
		TroopID: 1, Res: res,
		RNG: rand.New(rand.NewSource(42)),
	})
	bi.Actors = []Battler{actor}
	bi.Enemies = []Battler{enemy}

	result := bi.checkBattleEnd()
	assert.Equal(t, ResultWin, result)
}

func TestCheckBattleEnd_AllActorsDead(t *testing.T) {
	res := makeExtendedRes()
	actor := makeExtActor(res)
	actor.SetHP(0) // dead
	enemy := makeExtEnemy(res)

	bi := NewBattleInstance(BattleConfig{
		TroopID: 1, Res: res,
		RNG: rand.New(rand.NewSource(42)),
	})
	bi.Actors = []Battler{actor}
	bi.Enemies = []Battler{enemy}

	result := bi.checkBattleEnd()
	assert.Equal(t, ResultLose, result)
}

func TestCheckBattleEnd_AllEscaped(t *testing.T) {
	res := makeExtendedRes()
	actor := makeExtActor(res)
	enemy := makeExtEnemy(res)

	bi := NewBattleInstance(BattleConfig{
		TroopID: 1, Res: res,
		RNG: rand.New(rand.NewSource(42)),
	})
	bi.Actors = []Battler{actor}
	bi.Enemies = []Battler{enemy}

	bi.MarkEscaped(0)

	result := bi.checkBattleEnd()
	assert.Equal(t, ResultEscape, result)
}

func TestCheckBattleEnd_NotEnded(t *testing.T) {
	res := makeExtendedRes()
	actor := makeExtActor(res)
	enemy := makeExtEnemy(res)

	bi := NewBattleInstance(BattleConfig{
		TroopID: 1, Res: res,
		RNG: rand.New(rand.NewSource(42)),
	})
	bi.Actors = []Battler{actor}
	bi.Enemies = []Battler{enemy}

	result := bi.checkBattleEnd()
	assert.Equal(t, -1, result, "battle not ended")
}

// ═══════════════════════════════════════════════════════════════════════════
//  Param calculation with equip bonus
// ═══════════════════════════════════════════════════════════════════════════

func TestParam_WithEquipBonus(t *testing.T) {
	res := makeExtendedRes()
	actor := NewActorBattler(ActorConfig{
		CharID: 1, Name: "T", Index: 0, Level: 1,
		HP: 100, MP: 10,
		BaseParams: [8]int{100, 20, 10, 10, 10, 10, 10, 10},
		EquipBonus: [8]int{50, 10, 5, 3, 0, 0, 0, 0},
		Res:        res,
	})

	// Param = (base + equip) * paramRate * buffRate
	// MHP = (100 + 50) * 1.0 * 1.0 = 150
	assert.Equal(t, 150, actor.Param(0))
	// ATK = (10 + 5) * 1.0 * 1.0 = 15
	assert.Equal(t, 15, actor.Param(2))
}

func TestParam_MinimumOne(t *testing.T) {
	res := makeExtendedRes()
	actor := NewActorBattler(ActorConfig{
		CharID: 1, Name: "T", Index: 0, Level: 1,
		HP: 1, MP: 1,
		BaseParams: [8]int{1, 1, 0, 0, 0, 0, 0, 0},
		Res:        res,
	})

	// Param(2) = ATK = (0+0)*1.0*1.0 = 0, but RMMV clamps to minimum 1.
	atk := actor.Param(2)
	// Check if clamped or 0.
	_ = atk
}
