package battle

import (
	"math/rand"
	"testing"

	"github.com/kasuganosora/rpgmakermvmmo/server/resource"
)

func makeAITestRes() *resource.ResourceLoader {
	return &resource.ResourceLoader{
		Skills: []*resource.Skill{
			nil,
			{ID: 1, Name: "Attack", Scope: 1, HitType: 1, SuccessRate: 100,
				Damage: resource.SkillDamage{Formula: "a.atk * 4 - b.def * 2", Type: 1}},
			{ID: 2, Name: "Heal", Scope: 11, HitType: 0, SuccessRate: 100,
				Damage: resource.SkillDamage{Formula: "100", Type: 3}},
		},
		States: []*resource.State{nil, {ID: 1, Name: "Death"}},
	}
}

func makeAIEnemy(actions []resource.EnemyAction, hp, maxHP int) *EnemyBattler {
	e := NewEnemyBattler(&resource.Enemy{
		ID: 1, Name: "Boss", HP: maxHP, MP: 50,
		Atk: 20, Def: 10, Mat: 10, Mdf: 10, Agi: 15, Luk: 5,
		Actions: actions,
		Traits:  []resource.Trait{{Code: 22, DataID: 0, Value: 0.95}},
	}, 0, nil)
	e.SetHP(hp)
	return e
}

func TestMakeEnemyAction_AlwaysCondition(t *testing.T) {
	res := makeAITestRes()
	rng := rand.New(rand.NewSource(42))

	enemy := makeAIEnemy([]resource.EnemyAction{
		{SkillID: 1, ConditionType: 0, Rating: 5},
	}, 100, 100)
	actor := NewActorBattler(ActorConfig{
		Name: "Hero", Index: 0, HP: 100,
		BaseParams: [8]int{100, 50, 20, 10, 10, 10, 10, 10},
	})

	action := MakeEnemyAction(enemy, 0, []Battler{actor}, []Battler{enemy}, res, rng)
	if action == nil {
		t.Fatal("action should not be nil")
	}
	if action.SkillID != 1 {
		t.Errorf("skillID = %d, want 1", action.SkillID)
	}
}

func TestMakeEnemyAction_HPCondition(t *testing.T) {
	res := makeAITestRes()

	// Actions: attack (always) + heal (when HP ratio 0.0-0.5 i.e. 0-50%)
	actions := []resource.EnemyAction{
		{SkillID: 1, ConditionType: 0, Rating: 5},
		{SkillID: 2, ConditionType: 2, ConditionParam1: 0, ConditionParam2: 0.5, Rating: 9},
	}

	// At full HP → heal should not trigger, only attack
	enemy := makeAIEnemy(actions, 100, 100)
	actor := NewActorBattler(ActorConfig{Name: "Hero", Index: 0, HP: 100, BaseParams: [8]int{100, 50, 20, 10, 10, 10, 10, 10}})

	valid := filterValidActions(enemy.enemy.Actions, 0, enemy)
	if len(valid) != 1 {
		t.Errorf("at full HP: valid = %d, want 1 (only attack)", len(valid))
	}

	// At 30% HP → both should be valid, but heal has higher rating
	enemy2 := makeAIEnemy(actions, 30, 100)
	valid2 := filterValidActions(enemy2.enemy.Actions, 0, enemy2)
	if len(valid2) != 2 {
		t.Errorf("at 30%% HP: valid = %d, want 2", len(valid2))
	}

	// With rating 9, heal should be selected most often
	healCount := 0
	for i := 0; i < 100; i++ {
		rng2 := rand.New(rand.NewSource(int64(i)))
		a := MakeEnemyAction(enemy2, 0, []Battler{actor}, []Battler{enemy2}, res, rng2)
		if a != nil && a.SkillID == 2 {
			healCount++
		}
	}
	if healCount < 50 {
		t.Errorf("heal selected %d/100 times, expected >50 (rating 9 vs 5)", healCount)
	}
}

func TestMakeEnemyAction_TurnCondition(t *testing.T) {
	actions := []resource.EnemyAction{
		{SkillID: 1, ConditionType: 0, Rating: 5},                                   // always
		{SkillID: 2, ConditionType: 1, ConditionParam1: 2, ConditionParam2: 3, Rating: 5}, // turn 2 + 3*X
	}

	enemy := makeAIEnemy(actions, 100, 100)

	// Turn 0: only attack
	valid := filterValidActions(actions, 0, enemy)
	if len(valid) != 1 {
		t.Errorf("turn 0: valid = %d, want 1", len(valid))
	}

	// Turn 2: both
	valid = filterValidActions(actions, 2, enemy)
	if len(valid) != 2 {
		t.Errorf("turn 2: valid = %d, want 2", len(valid))
	}

	// Turn 5: 2 + 3*1 = 5 → both
	valid = filterValidActions(actions, 5, enemy)
	if len(valid) != 2 {
		t.Errorf("turn 5: valid = %d, want 2", len(valid))
	}

	// Turn 3: not matching (3-2=1, 1%3 != 0)
	valid = filterValidActions(actions, 3, enemy)
	if len(valid) != 1 {
		t.Errorf("turn 3: valid = %d, want 1", len(valid))
	}
}

func TestMakeEnemyAction_NoActions(t *testing.T) {
	res := makeAITestRes()
	rng := rand.New(rand.NewSource(42))

	enemy := makeAIEnemy(nil, 100, 100)
	actor := NewActorBattler(ActorConfig{
		Name: "Hero", Index: 0, HP: 100,
		BaseParams: [8]int{100, 50, 20, 10, 10, 10, 10, 10},
	})

	action := MakeEnemyAction(enemy, 0, []Battler{actor}, []Battler{enemy}, res, rng)
	if action == nil {
		t.Fatal("should fall back to default attack")
	}
	if action.Type != ActionAttack {
		t.Errorf("type = %d, want ActionAttack", action.Type)
	}
}

func TestWeightedSelect_HigherRatingPreferred(t *testing.T) {
	actions := []resource.EnemyAction{
		{SkillID: 1, Rating: 1},
		{SkillID: 2, Rating: 9},
	}

	counts := make(map[int]int)
	for i := 0; i < 1000; i++ {
		rng := rand.New(rand.NewSource(int64(i)))
		selected := weightedSelect(actions, rng)
		counts[selected.SkillID]++
	}

	// Rating 9 should be selected much more often than rating 1
	if counts[2] < counts[1] {
		t.Errorf("higher rating (9) selected %d times vs lower (1) %d times", counts[2], counts[1])
	}
}

func TestCheckCondition_StateCondition(t *testing.T) {
	enemy := makeAIEnemy(nil, 100, 100)

	a := resource.EnemyAction{ConditionType: 4, ConditionParam1: 5} // requires state 5

	if checkCondition(a, 0, enemy) {
		t.Error("should fail: enemy doesn't have state 5")
	}

	enemy.AddState(5, 3)
	if !checkCondition(a, 0, enemy) {
		t.Error("should pass: enemy has state 5")
	}
}
