package battle

import (
	"math/rand"
	"testing"

	"github.com/kasuganosora/rpgmakermvmmo/server/resource"
)

func makeTestResForAction() *resource.ResourceLoader {
	return &resource.ResourceLoader{
		Skills: []*resource.Skill{
			nil, // index 0
			{ID: 1, Name: "Attack", MPCost: 0, Scope: 1, HitType: 1, SuccessRate: 100, Repeats: 1,
				Damage: resource.SkillDamage{Formula: "a.atk * 4 - b.def * 2", Type: 1, ElementID: 0, Critical: true, Variance: 20}},
			{ID: 2, Name: "Fire", MPCost: 10, Scope: 1, HitType: 2, SuccessRate: 100, Repeats: 1,
				Damage: resource.SkillDamage{Formula: "a.mat * 3 - b.mdf", Type: 1, ElementID: 2, Critical: true, Variance: 10}},
			{ID: 3, Name: "Heal", MPCost: 5, Scope: 7, HitType: 0, SuccessRate: 100, Repeats: 1,
				Damage: resource.SkillDamage{Formula: "a.mat * 2 + 20", Type: 3, Critical: false}},
			{ID: 4, Name: "Drain", MPCost: 8, Scope: 1, HitType: 2, SuccessRate: 100, Repeats: 1,
				Damage: resource.SkillDamage{Formula: "a.mat * 2 - b.mdf", Type: 5, Critical: false}},
			{ID: 5, Name: "Double Strike", MPCost: 0, Scope: 1, HitType: 1, SuccessRate: 100, Repeats: 2,
				Damage: resource.SkillDamage{Formula: "a.atk * 2 - b.def", Type: 1, Critical: true, Variance: 10}},
			{ID: 6, Name: "Poison Touch", MPCost: 3, Scope: 1, HitType: 1, SuccessRate: 100, Repeats: 1,
				Damage: resource.SkillDamage{Formula: "a.atk * 2 - b.def", Type: 1, Critical: false},
				Effects: []resource.SkillEffect{
					{Code: 21, DataID: 2, Value1: 1.0}, // RMMV code 21 = ADD_STATE, 100% add poison (state 2)
				}},
		},
		Items: []*resource.Item{
			nil,
			{ID: 1, Name: "Potion", Scope: 7, HitType: 0, SuccessRate: 100,
				Effects: []resource.SkillEffect{
					{Code: 11, Value1: 0.0, Value2: 50}, // RMMV code 11 = RECOVER_HP, flat 50
				}},
		},
		States: []*resource.State{
			nil,
			{ID: 1, Name: "Death"},
			{ID: 2, Name: "Poison", AutoRemovalTiming: 2, MinTurns: 3, MaxTurns: 5,
				Traits: []resource.Trait{
					{Code: 21, DataID: 7, Value: 0.5},
				}},
		},
	}
}

func makeActionActor(res *resource.ResourceLoader) *ActorBattler {
	return NewActorBattler(ActorConfig{
		CharID: 1, Name: "Hero", Index: 0, Level: 10,
		HP: 200, MP: 50,
		BaseParams: [8]int{250, 80, 30, 20, 25, 18, 15, 10},
		EquipBonus: [8]int{0, 0, 10, 5, 0, 0, 0, 0},
		Skills:     []int{1, 2, 3, 4, 5, 6},
		ActorTraits: []resource.Trait{
			{Code: 22, DataID: 0, Value: 0.95}, // 95% hit
			{Code: 22, DataID: 2, Value: 0.10}, // 10% crit
		},
		Res: res,
	})
}

func makeActionEnemy(res *resource.ResourceLoader) *EnemyBattler {
	return NewEnemyBattler(&resource.Enemy{
		ID: 1, Name: "Goblin",
		HP: 100, MP: 20,
		Atk: 15, Def: 10, Mat: 8, Mdf: 6, Agi: 12, Luk: 5,
		Traits: []resource.Trait{
			{Code: 22, DataID: 0, Value: 0.90}, // 90% hit
			{Code: 11, DataID: 2, Value: 1.5},  // fire weakness
		},
	}, 0, res)
}

func TestProcessAttackAction(t *testing.T) {
	res := makeTestResForAction()
	actor := makeActionActor(res)
	enemy := makeActionEnemy(res)

	ap := &ActionProcessor{Res: res, RNG: rand.New(rand.NewSource(42))}
	action := &Action{Type: ActionAttack, TargetIndices: []int{0}, TargetIsActor: false}

	outcomes := ap.ProcessAction(actor, action, []Battler{actor}, []Battler{enemy})
	if len(outcomes) == 0 {
		t.Fatal("expected at least 1 outcome")
	}

	out := outcomes[0]
	if out.Missed {
		t.Error("attack should not miss with 95% hit rate")
	}
	// ATK=40, DEF=10: damage = 40*4 - 10*2 = 140 ± 20% variance
	// ATK=40, DEF=10: base=140, with ±20% variance and possible 3x crit
	if out.Damage < 1 || out.Damage > 520 {
		t.Errorf("damage = %d, expected 1-520", out.Damage)
	}
	if enemy.HP() >= 100 {
		t.Errorf("enemy HP should be reduced, got %d", enemy.HP())
	}
}

func TestProcessSkillAction(t *testing.T) {
	res := makeTestResForAction()
	actor := makeActionActor(res)
	enemy := makeActionEnemy(res)

	ap := &ActionProcessor{Res: res, RNG: rand.New(rand.NewSource(42))}
	action := &Action{Type: ActionSkill, SkillID: 2, TargetIndices: []int{0}, TargetIsActor: false}

	mpBefore := actor.MP()
	outcomes := ap.ProcessAction(actor, action, []Battler{actor}, []Battler{enemy})

	if actor.MP() != mpBefore-10 {
		t.Errorf("MP = %d, want %d (cost 10)", actor.MP(), mpBefore-10)
	}
	if len(outcomes) == 0 {
		t.Fatal("expected outcomes")
	}
	// Fire skill with element 2, enemy has 1.5x weakness
	// MAT=25, MDF=6: base = 25*3 - 6 = 69, *1.5 fire = 103.5
	out := outcomes[0]
	if out.Missed {
		t.Error("fire should not miss (magical, 100% success)")
	}
	// MAT=25, MDF=6: base=69, *1.5 fire=103, with ±10% variance and possible 3x crit
	if out.Damage < 1 || out.Damage > 400 {
		t.Errorf("fire damage = %d, expected 1-400", out.Damage)
	}
}

func TestProcessHealAction(t *testing.T) {
	res := makeTestResForAction()
	actor := makeActionActor(res)

	actor.SetHP(100) // damage the actor

	ap := &ActionProcessor{Res: res, RNG: rand.New(rand.NewSource(42))}
	action := &Action{Type: ActionSkill, SkillID: 3, TargetIndices: []int{0}, TargetIsActor: true}

	outcomes := ap.ProcessAction(actor, action, []Battler{actor}, nil)
	if len(outcomes) == 0 {
		t.Fatal("expected outcomes")
	}

	out := outcomes[0]
	if out.Damage >= 0 {
		t.Errorf("heal should produce negative damage, got %d", out.Damage)
	}
	// MAT=25: heal = 25*2 + 20 = 70
	if actor.HP() < 100 {
		t.Errorf("HP should be > 100 after heal, got %d", actor.HP())
	}
}

func TestProcessDrainAction(t *testing.T) {
	res := makeTestResForAction()
	actor := makeActionActor(res)
	enemy := makeActionEnemy(res)

	actor.SetHP(100) // damage the actor
	actorHPBefore := actor.HP()

	ap := &ActionProcessor{Res: res, RNG: rand.New(rand.NewSource(42))}
	action := &Action{Type: ActionSkill, SkillID: 4, TargetIndices: []int{0}, TargetIsActor: false}

	outcomes := ap.ProcessAction(actor, action, []Battler{actor}, []Battler{enemy})
	if len(outcomes) == 0 {
		t.Fatal("expected outcomes")
	}

	out := outcomes[0]
	if out.Drain <= 0 {
		t.Errorf("drain should be positive, got %d", out.Drain)
	}
	if actor.HP() <= actorHPBefore {
		t.Errorf("actor HP should increase after drain: before=%d, after=%d", actorHPBefore, actor.HP())
	}
}

func TestDoubleStrike(t *testing.T) {
	res := makeTestResForAction()
	actor := makeActionActor(res)
	enemy := makeActionEnemy(res)

	ap := &ActionProcessor{Res: res, RNG: rand.New(rand.NewSource(42))}
	action := &Action{Type: ActionSkill, SkillID: 5, TargetIndices: []int{0}, TargetIsActor: false}

	outcomes := ap.ProcessAction(actor, action, []Battler{actor}, []Battler{enemy})
	if len(outcomes) != 2 {
		t.Fatalf("double strike should produce 2 outcomes, got %d", len(outcomes))
	}
}

func TestPoisonTouchAddsState(t *testing.T) {
	res := makeTestResForAction()
	actor := makeActionActor(res)
	enemy := makeActionEnemy(res)

	ap := &ActionProcessor{Res: res, RNG: rand.New(rand.NewSource(42))}
	action := &Action{Type: ActionSkill, SkillID: 6, TargetIndices: []int{0}, TargetIsActor: false}

	outcomes := ap.ProcessAction(actor, action, []Battler{actor}, []Battler{enemy})
	if len(outcomes) == 0 {
		t.Fatal("expected outcomes")
	}

	out := outcomes[0]
	if !enemy.HasState(2) {
		t.Error("enemy should have poison state")
	}
	found := false
	for _, s := range out.AddedStates {
		if s == 2 {
			found = true
		}
	}
	if !found {
		t.Error("AddedStates should contain poison (2)")
	}
}

func TestGuardActionSetsFlag(t *testing.T) {
	res := makeTestResForAction()
	actor := makeActionActor(res)

	ap := &ActionProcessor{Res: res, RNG: rand.New(rand.NewSource(42))}
	action := &Action{Type: ActionGuard}

	outcomes := ap.ProcessAction(actor, action, nil, nil)
	if outcomes != nil {
		t.Error("guard should produce no outcomes")
	}
	if !actor.IsGuarding() {
		t.Error("actor should be guarding")
	}
}

func TestGuardReducesDamage(t *testing.T) {
	res := makeTestResForAction()
	actor := makeActionActor(res)
	enemy := makeActionEnemy(res)

	actor.SetGuarding(true)

	ap := &ActionProcessor{Res: res, RNG: rand.New(rand.NewSource(100))}

	// Enemy attacks actor
	action := &Action{Type: ActionAttack, TargetIndices: []int{0}, TargetIsActor: true}
	outcomes := ap.ProcessAction(enemy, action, []Battler{actor}, []Battler{enemy})
	guardDamage := 0
	if len(outcomes) > 0 && !outcomes[0].Missed {
		guardDamage = outcomes[0].Damage
	}

	// Reset and attack without guard
	actor.SetHP(200)
	actor.SetGuarding(false)
	ap2 := &ActionProcessor{Res: res, RNG: rand.New(rand.NewSource(100))}
	outcomes2 := ap2.ProcessAction(enemy, action, []Battler{actor}, []Battler{enemy})
	normalDamage := 0
	if len(outcomes2) > 0 && !outcomes2[0].Missed {
		normalDamage = outcomes2[0].Damage
	}

	if normalDamage > 0 && guardDamage >= normalDamage {
		t.Errorf("guard damage (%d) should be less than normal (%d)", guardDamage, normalDamage)
	}
}

func TestItemHealEffect(t *testing.T) {
	res := makeTestResForAction()
	actor := makeActionActor(res)

	actor.SetHP(100) // damage actor

	ap := &ActionProcessor{Res: res, RNG: rand.New(rand.NewSource(42))}
	action := &Action{Type: ActionItem, ItemID: 1, TargetIndices: []int{0}, TargetIsActor: true}

	outcomes := ap.ProcessAction(actor, action, []Battler{actor}, nil)
	if len(outcomes) == 0 {
		t.Fatal("expected outcomes from potion")
	}

	// Potion heals 50 HP
	if actor.HP() != 150 {
		t.Errorf("HP = %d, want 150 (100 + 50)", actor.HP())
	}
}

func TestCertainHitAlwaysHits(t *testing.T) {
	res := makeTestResForAction()
	ap := &ActionProcessor{Res: res, RNG: rand.New(rand.NewSource(42))}

	actor := makeActionActor(res)
	enemy := makeActionEnemy(res)

	// 100 certain-hit checks should all succeed
	for i := 0; i < 100; i++ {
		if !ap.checkHit(actor, enemy, 0, 100) {
			t.Fatal("certain hit should always succeed")
		}
	}
}

func TestScopeAllEnemies(t *testing.T) {
	res := makeTestResForAction()
	ap := &ActionProcessor{Res: res, RNG: rand.New(rand.NewSource(42))}

	actor := makeActionActor(res)
	e1 := NewEnemyBattler(&resource.Enemy{ID: 1, Name: "G1", HP: 50, Atk: 10, Def: 5}, 0, res)
	e2 := NewEnemyBattler(&resource.Enemy{ID: 2, Name: "G2", HP: 50, Atk: 10, Def: 5}, 1, res)

	targets := ap.resolveByScope(2, actor, []Battler{actor}, []Battler{e1, e2})
	if len(targets) != 2 {
		t.Errorf("scope 2 (all enemies) should return 2, got %d", len(targets))
	}
}

func TestScopeUser(t *testing.T) {
	res := makeTestResForAction()
	ap := &ActionProcessor{Res: res, RNG: rand.New(rand.NewSource(42))}

	actor := makeActionActor(res)

	targets := ap.resolveByScope(11, actor, []Battler{actor}, nil)
	if len(targets) != 1 || targets[0] != actor {
		t.Error("scope 11 (user) should return the subject")
	}
}

// --- ProjectB custom formula integration tests ---

func makeProjectBRes() *resource.ResourceLoader {
	return &resource.ResourceLoader{
		Skills: []*resource.Skill{
			nil,
			// Skill 1: normal attack
			{ID: 1, Name: "Attack", Scope: 1, HitType: 1, SuccessRate: 100, Repeats: 1,
				Damage: resource.SkillDamage{Formula: "a.atk * 4 - b.def * 2", Type: 1, Critical: true, Variance: 20}},
			nil, nil, nil, nil, nil, nil, nil, nil, nil, // 2-10
			nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, // 11-20
			nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, // 21-30
			nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, // 31-40
			nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, // 41-50
			nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, // 51-60
			nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, // 61-70
			// Skill 71: 光之矢 (Light Arrow)
			{ID: 71, Name: "光之矢", MPCost: 5, Scope: 1, HitType: 2, SuccessRate: 100, Repeats: 1,
				Damage: resource.SkillDamage{
					Formula:   "damagecul_magic(a.mat, b.mdf, 3, 1224)",
					Type:      1,
					ElementID: 8,
					Critical:  false,
					Variance:  20,
				}},
		},
		States: []*resource.State{
			nil,
			{ID: 1, Name: "Death"},
		},
	}
}

func makeSkill263Res() *resource.ResourceLoader {
	res := makeProjectBRes()
	// Extend Skills slice to hold skill 263
	for len(res.Skills) <= 263 {
		res.Skills = append(res.Skills, nil)
	}
	res.Skills[263] = &resource.Skill{
		ID: 263, Name: "藤的反弹", MPCost: 0, Scope: 1, HitType: 1, SuccessRate: 100, Repeats: 1,
		Damage: resource.SkillDamage{
			Formula:   "damagecul_enemy_normal(a.atk, b.def, 2)",
			Type:      1,
			ElementID: -1,
			Critical:  true,
			Variance:  20,
		},
		Effects: []resource.SkillEffect{
			{Code: 21, DataID: 0, Value1: 1.0}, // add state 0 (death) on hit
		},
	}
	return res
}

func TestProjectBSkill71_DamageNonZero(t *testing.T) {
	res := makeProjectBRes()
	// Actor: class 1, level 1, mat from class base params
	actor := NewActorBattler(ActorConfig{
		CharID: 1, Name: "天音", Index: 0, ClassID: 1, Level: 1,
		HP: 100, MP: 15,
		BaseParams: [8]int{200, 15, 10, 10, 10, 10, 10, 3},
		Skills:     []int{71},
		Res:        res,
	})
	// Enemy 36: tentacle
	enemy := NewEnemyBattler(&resource.Enemy{
		ID: 36, Name: "触手:海葵", HP: 100, MP: 0,
		Atk: 14, Def: 10, Mat: 10, Mdf: 10, Agi: 10, Luk: 3,
		Actions: []resource.EnemyAction{{SkillID: 263, ConditionType: 0, Rating: 3}},
	}, 0, res)

	ap := &ActionProcessor{Res: res, RNG: rand.New(rand.NewSource(42))}
	action := &Action{Type: ActionSkill, SkillID: 71, TargetIndices: []int{0}, TargetIsActor: false}

	outcomes := ap.ProcessAction(actor, action, []Battler{actor}, []Battler{enemy})
	if len(outcomes) == 0 {
		t.Fatal("expected outcomes from skill 71")
	}

	out := outcomes[0]
	if out.Missed {
		t.Error("skill 71 should not miss (magical, 100% success)")
	}
	// Expected base: damagecul_magic(10, 10, 3, 1224) = (10-5)*6 = 30
	// With ±20% variance → 24-36, no crit
	if out.Damage < 1 {
		t.Errorf("skill 71 damage = %d, want > 0 (formula: damagecul_magic(a.mat, b.mdf, 3, 1224))", out.Damage)
	}
	if enemy.HP() >= 100 {
		t.Errorf("enemy HP should be reduced from 100, got %d", enemy.HP())
	}
	t.Logf("Skill 71 damage=%d, enemy HP=%d", out.Damage, enemy.HP())
}

func TestProjectBSkill263_DamageNonZero(t *testing.T) {
	res := makeSkill263Res()
	// Actor: class 1, level 1
	actor := NewActorBattler(ActorConfig{
		CharID: 1, Name: "天音", Index: 0, ClassID: 1, Level: 1,
		HP: 100, MP: 15,
		BaseParams: [8]int{200, 15, 10, 10, 10, 10, 10, 3},
		Skills:     []int{71},
		Res:        res,
	})
	// Enemy 36 with hit rate trait
	enemy := NewEnemyBattler(&resource.Enemy{
		ID: 36, Name: "触手:海葵", HP: 100, MP: 0,
		Atk: 14, Def: 10, Mat: 10, Mdf: 10, Agi: 10, Luk: 3,
		Actions: []resource.EnemyAction{{SkillID: 263, ConditionType: 0, Rating: 3}},
		Traits: []resource.Trait{
			{Code: 22, DataID: 0, Value: 0.95}, // 95% hit rate
		},
	}, 0, res)

	ap := &ActionProcessor{Res: res, RNG: rand.New(rand.NewSource(42))}
	// Enemy uses skill 263
	action := &Action{Type: ActionSkill, SkillID: 263, TargetIndices: []int{0}, TargetIsActor: true}

	outcomes := ap.ProcessAction(enemy, action, []Battler{actor}, []Battler{enemy})
	if len(outcomes) == 0 {
		t.Fatal("expected outcomes from skill 263")
	}

	out := outcomes[0]
	if out.Missed {
		t.Error("skill 263 should not miss (physical, 100% success)")
	}
	// Expected base: damagecul_enemy_normal(14, 10, 2) = (14-5)*4 = 36
	// With ±20% variance → 29-43
	if out.Damage < 1 {
		t.Errorf("skill 263 damage = %d, want > 0 (formula: damagecul_enemy_normal(a.atk, b.def, 2))", out.Damage)
	}
	if actor.HP() >= 100 {
		t.Errorf("actor HP should be reduced from 100, got %d", actor.HP())
	}
	t.Logf("Skill 263 damage=%d, actor HP=%d", out.Damage, actor.HP())
}

func TestCalcDamage_FallbackOnUnknownFormula(t *testing.T) {
	res := &resource.ResourceLoader{
		Skills: []*resource.Skill{nil},
	}
	actor := NewActorBattler(ActorConfig{
		CharID: 1, Name: "Hero", Index: 0, Level: 5,
		HP: 100, MP: 20,
		BaseParams: [8]int{100, 20, 30, 15, 20, 10, 15, 10},
		Res:        res,
	})
	enemy := NewEnemyBattler(&resource.Enemy{
		ID: 1, Name: "Mob", HP: 50, Atk: 10, Def: 8,
	}, 0, res)

	ap := &ActionProcessor{Res: res, RNG: rand.New(rand.NewSource(42))}

	// Use a completely unknown formula
	dmg := &resource.SkillDamage{
		Formula:  "some_unknown_plugin_func(a.atk, b.def)",
		Type:     1,
		Variance: 0,
	}

	result := ap.calcDamage(actor, enemy, dmg)
	// Should fall back to atk*4 - def*2 = 30*4 - 8*2 = 104
	if result.damage < 1 {
		t.Errorf("fallback damage = %d, want > 0 (should fallback to atk*4 - def*2)", result.damage)
	}
	t.Logf("Fallback damage=%d (ATK=%d, DEF=%d)", result.damage, actor.Param(2), enemy.Param(3))
}
