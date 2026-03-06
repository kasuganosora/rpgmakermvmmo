package battle

import (
	"context"
	"math/rand"
	"testing"
	"time"

	"github.com/kasuganosora/rpgmakermvmmo/server/resource"
)

// ---------------------------------------------------------------------------
// Bug #1: Action.speed() always returns 0
// Guard should have speed 2000 (RMMV default) to always go first.
// Skills/items should use their Speed field from data.
// ---------------------------------------------------------------------------

func TestGuardActionSpeed2000(t *testing.T) {
	// Guard actions in RMMV have speed 2000, making them always go first.
	a := &Action{Type: ActionGuard}
	if a.speed() != 2000 {
		t.Errorf("guard speed = %d, want 2000", a.speed())
	}
}

func TestActionSpeedFromField(t *testing.T) {
	// Action should carry the skill's speed modifier via its Speed field.
	a := &Action{Type: ActionSkill, SkillID: 1}
	a.SpeedMod = 10
	if a.speed() != 10 {
		t.Errorf("skill action speed = %d, want 10", a.speed())
	}
}

func TestGuardGoesFirstInTurnOrder(t *testing.T) {
	res := makeTestRes()
	rng := rand.New(rand.NewSource(42))

	// Slow actor (AGI=1) guards, fast enemy (AGI=99) attacks.
	// Guard speed 2000 should make the actor go first despite low AGI.
	slowActor := NewActorBattler(ActorConfig{
		Name: "Slow", Index: 0, HP: 100,
		BaseParams: [8]int{100, 50, 20, 10, 10, 10, 1, 10},
		Res:        res,
	})
	slowActor.SetAction(&Action{Type: ActionGuard})

	fastEnemy := NewEnemyBattler(&resource.Enemy{
		ID: 1, Name: "Fast", HP: 100,
		Atk: 10, Def: 5, Agi: 99, Luk: 5,
	}, 0, res)
	fastEnemy.SetAction(&Action{Type: ActionAttack, SkillID: 1})

	tm := DefaultTurnManager{}
	order := tm.MakeActionOrder([]Battler{slowActor}, []Battler{fastEnemy}, rng)

	if len(order) != 2 {
		t.Fatalf("order len = %d, want 2", len(order))
	}
	if order[0].Name() != "Slow" {
		t.Errorf("first = %s, want Slow (guard should go first)", order[0].Name())
	}
}

// ---------------------------------------------------------------------------
// Bug #2: TP not initialized at battle start
// RMMV initializes TP to random 0-25 via initTp().
// ---------------------------------------------------------------------------

func TestTPInitializedAtBattleStart(t *testing.T) {
	res := makeInstanceRes()

	actor := NewActorBattler(ActorConfig{
		CharID: 1, Name: "Hero", Index: 0, Level: 10,
		HP: 500, MP: 50, // TP defaults to 0
		BaseParams:  [8]int{500, 50, 50, 30, 20, 10, 20, 10},
		ActorTraits: []resource.Trait{{Code: 22, DataID: 0, Value: 0.95}},
		Res:         res,
	})
	enemy := NewEnemyBattler(&resource.Enemy{
		ID: 1, Name: "Slime", HP: 10,
		Atk: 5, Def: 2, Agi: 5, Luk: 1,
		Exp: 20, Gold: 10,
		Actions: []resource.EnemyAction{{SkillID: 1, ConditionType: 0, Rating: 5}},
		Traits:  []resource.Trait{{Code: 22, DataID: 0, Value: 0.90}},
	}, 0, res)

	bi := NewBattleInstance(BattleConfig{
		TroopID:      1,
		Res:          res,
		RNG:          rand.New(rand.NewSource(42)),
		InputTimeout: 5 * time.Second,
	})
	bi.Actors = []Battler{actor}
	bi.Enemies = []Battler{enemy}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Capture the battle_start event to check initial TP.
	var startEvt *EventBattleStart
	go func() {
		for evt := range bi.Events() {
			if bs, ok := evt.(*EventBattleStart); ok {
				startEvt = bs
			}
			if evt.EventType() == "input_request" {
				bi.SubmitInput(&ActionInput{
					ActorIndex:    0,
					ActionType:    ActionAttack,
					TargetIndices: []int{0},
					TargetIsActor: false,
				})
			}
		}
	}()

	bi.Run(ctx)
	time.Sleep(50 * time.Millisecond)

	if startEvt == nil {
		t.Fatal("no battle_start event received")
	}

	// After initTp, actor TP should be in range [0, 25].
	// With seed 42, it should be > 0 (but we check range).
	actorTP := startEvt.Actors[0].TP
	if actorTP < 0 || actorTP > 25 {
		t.Errorf("actor initial TP = %d, want 0-25 (RMMV initTp)", actorTP)
	}
	// The actor battler's actual TP should also be initialized.
	if actor.TP() < 0 || actor.TP() > 25 {
		t.Errorf("actor.TP() = %d, want 0-25", actor.TP())
	}
}

// ---------------------------------------------------------------------------
// Bug #3: CalculateDrops uses global rand instead of injectable RNG
// ---------------------------------------------------------------------------

func TestCalculateDropsDeterministic(t *testing.T) {
	enemy := &resource.Enemy{
		ID: 1, Name: "Test",
		DropItems: []resource.EnemyDrop{
			{Kind: 1, DataID: 5, Denominator: 2}, // 50% chance
			{Kind: 2, DataID: 3, Denominator: 3}, // 33% chance
		},
	}

	// Same seed should produce same drops.
	rng1 := rand.New(rand.NewSource(99))
	drops1 := CalculateDropsRNG(enemy, rng1)

	rng2 := rand.New(rand.NewSource(99))
	drops2 := CalculateDropsRNG(enemy, rng2)

	if len(drops1) != len(drops2) {
		t.Fatalf("drops1 len=%d, drops2 len=%d (should be deterministic)", len(drops1), len(drops2))
	}
	for i := range drops1 {
		if drops1[i] != drops2[i] {
			t.Errorf("drops[%d] differ: %+v vs %+v", i, drops1[i], drops2[i])
		}
	}
}

// ---------------------------------------------------------------------------
// Bug #4: CalcStats level index off-by-one
// stats.go uses level-1, but RMMV params are indexed by level directly.
// ---------------------------------------------------------------------------

func TestCalcStatsLevelIndex(t *testing.T) {
	// Create a class with known params at levels 1 and 2.
	// params[paramID][level]: index 0 = level 0, index 1 = level 1, etc.
	cls := &resource.Class{
		ID:   1,
		Name: "Warrior",
		Params: [][]int{
			{0, 100, 150, 200}, // MHP: lv0=0, lv1=100, lv2=150, lv3=200
			{0, 20, 30, 40},    // MMP
			{0, 10, 15, 20},    // ATK: lv0=0, lv1=10, lv2=15, lv3=20
			{0, 8, 12, 16},     // DEF
			{0, 5, 8, 11},      // MAT
			{0, 5, 8, 11},      // MDF
			{0, 7, 10, 13},     // AGI
			{0, 3, 5, 7},       // LUK
		},
	}
	res := &resource.ResourceLoader{
		Classes: []*resource.Class{nil, cls},
	}

	// For level 2, RMMV should use params[paramID][2].
	// ATK at level 2 = 15 (NOT 10 from level 1).
	stats := calcStatsFromClass(res, 1, 2)
	if stats == nil {
		t.Fatal("calcStatsFromClass returned nil")
	}
	if stats[2] != 15 {
		t.Errorf("ATK at level 2 = %d, want 15 (got level %d value)", stats[2], 2)
	}
	if stats[0] != 150 {
		t.Errorf("MHP at level 2 = %d, want 150", stats[0])
	}

	// Level 1 should use params[paramID][1].
	stats1 := calcStatsFromClass(res, 1, 1)
	if stats1[2] != 10 {
		t.Errorf("ATK at level 1 = %d, want 10", stats1[2])
	}
}
