package battle

import (
	"context"
	"encoding/json"
	"math/rand"
	"strings"
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

// ---------------------------------------------------------------------------
// Bug #5: BattlerSnapshot missing computed params, skills, equips
// Client needs all 8 params to sync _puppetParams, classID for traits,
// and skills for command window.
// ---------------------------------------------------------------------------

func TestSnapshotContainsParams(t *testing.T) {
	res := makeInstanceRes()
	actor := NewActorBattler(ActorConfig{
		CharID: 1, Name: "Hero", Index: 0, ClassID: 3, Level: 5,
		HP: 200, MP: 30,
		BaseParams:  [8]int{250, 40, 30, 20, 15, 10, 18, 8},
		EquipBonus:  [8]int{0, 0, 10, 5, 0, 0, 2, 0},
		Skills:      []int{1, 2, 7},
		ActorTraits: []resource.Trait{{Code: 22, DataID: 0, Value: 0.95}},
		Res:         res,
	})

	snap := SnapshotBattler(actor)

	// Params should contain all 8 effective values.
	if len(snap.Params) != 8 {
		t.Fatalf("Params len = %d, want 8", len(snap.Params))
	}
	// MHP = (250+0) * 1.0 = 250 (no paramRate trait for param 0 that changes multiplicatively here,
	// only xparam code 22 which doesn't affect param 0)
	if snap.Params[0] != actor.Param(0) {
		t.Errorf("Params[0] (MHP) = %d, want %d", snap.Params[0], actor.Param(0))
	}
	// ATK = (30+10) * 1.0 = 40
	if snap.Params[2] != actor.Param(2) {
		t.Errorf("Params[2] (ATK) = %d, want %d", snap.Params[2], actor.Param(2))
	}
	// AGI = (18+2) * 1.0 = 20
	if snap.Params[6] != actor.Param(6) {
		t.Errorf("Params[6] (AGI) = %d, want %d", snap.Params[6], actor.Param(6))
	}

	// Skills should be present for actors.
	if len(snap.Skills) != 3 {
		t.Errorf("Skills len = %d, want 3", len(snap.Skills))
	}
}

func TestBattleStartEventIncludesGameVars(t *testing.T) {
	res := makeInstanceRes()

	actor := NewActorBattler(ActorConfig{
		CharID: 1, Name: "Hero", Index: 0, Level: 10,
		HP: 500, MP: 50,
		BaseParams: [8]int{500, 50, 50, 30, 20, 10, 20, 10},
		Res:        res,
	})
	enemy := NewEnemyBattler(&resource.Enemy{
		ID: 1, Name: "Slime", HP: 5, MP: 0,
		Atk: 1, Def: 1, Mat: 1, Mdf: 1, Agi: 1, Luk: 1,
		Actions: []resource.EnemyAction{{SkillID: 1, ConditionType: 0, Rating: 5}},
	}, 0, res)

	testVars := map[int]int{702: 100, 722: 200, 1026: 50, 1027: 30}

	bi := NewBattleInstance(BattleConfig{
		TroopID:      1,
		Res:          res,
		RNG:          rand.New(rand.NewSource(42)),
		InputTimeout: 5 * time.Second,
		GameVars:     testVars,
	})
	bi.Actors = []Battler{actor}
	bi.Enemies = []Battler{enemy}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	var startEvt *EventBattleStart
	go func() {
		for evt := range bi.Events() {
			if bs, ok := evt.(*EventBattleStart); ok {
				startEvt = bs
			}
			if ir, ok := evt.(*EventInputRequest); ok {
				bi.SubmitInput(&ActionInput{
					ActorIndex:    ir.ActorIndex,
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
	if startEvt.GameVars == nil {
		t.Fatal("GameVars is nil")
	}
	if startEvt.GameVars[702] != 100 {
		t.Errorf("GameVars[702] = %d, want 100", startEvt.GameVars[702])
	}
	if startEvt.GameVars[1026] != 50 {
		t.Errorf("GameVars[1026] = %d, want 50", startEvt.GameVars[1026])
	}
}

func TestSnapshotContainsParamsEnemy(t *testing.T) {
	res := makeInstanceRes()
	enemy := NewEnemyBattler(&resource.Enemy{
		ID: 1, Name: "Goblin", HP: 80, MP: 10,
		Atk: 15, Def: 8, Mat: 5, Mdf: 5, Agi: 12, Luk: 3,
		Actions: []resource.EnemyAction{{SkillID: 1, ConditionType: 0, Rating: 5}},
	}, 0, res)

	snap := SnapshotBattler(enemy)

	if len(snap.Params) != 8 {
		t.Fatalf("Params len = %d, want 8", len(snap.Params))
	}
	if snap.Params[2] != 15 {
		t.Errorf("Params[2] (ATK) = %d, want 15", snap.Params[2])
	}
	if snap.Params[6] != 12 {
		t.Errorf("Params[6] (AGI) = %d, want 12", snap.Params[6])
	}
}

// ---------------------------------------------------------------------------
// Bug #6: chargeTpByDamage — RMMV grants TP when taking HP damage
// Formula: 50 * (damage / mhp) * tcr
// ---------------------------------------------------------------------------

func TestChargeTpByDamage(t *testing.T) {
	res := makeTestRes()
	actor := NewActorBattler(ActorConfig{
		Name: "Hero", Index: 0, HP: 100,
		BaseParams: [8]int{100, 50, 30, 20, 10, 10, 10, 10},
		Res:        res,
	})
	actor.SetTP(0)

	// 50 damage on 100 MHP = 50% damage rate → 50 * 0.5 * 1.0 = 25 TP
	chargeTpByDamage(actor, 50)
	if actor.TP() != 25 {
		t.Errorf("TP after 50%% damage = %d, want 25", actor.TP())
	}

	// 10 damage on 100 MHP = 10% → 50 * 0.1 * 1.0 = 5 TP → total 30
	chargeTpByDamage(actor, 10)
	if actor.TP() != 30 {
		t.Errorf("TP after 10%% more damage = %d, want 30", actor.TP())
	}

	// 0 or negative damage should not charge TP.
	chargeTpByDamage(actor, 0)
	chargeTpByDamage(actor, -5)
	if actor.TP() != 30 {
		t.Errorf("TP after 0/negative damage = %d, want 30", actor.TP())
	}
}

func TestChargeTpByDamageCapsAt100(t *testing.T) {
	res := makeTestRes()
	actor := NewActorBattler(ActorConfig{
		Name: "Hero", Index: 0, HP: 100,
		BaseParams: [8]int{100, 50, 30, 20, 10, 10, 10, 10},
		Res:        res,
	})
	actor.SetTP(90)

	// 100 damage → 50 * 1.0 * 1.0 = 50 → 90+50=140, clamped to 100
	chargeTpByDamage(actor, 100)
	if actor.TP() != 100 {
		t.Errorf("TP after full damage from 90 = %d, want 100", actor.TP())
	}
}

// ---------------------------------------------------------------------------
// Bug #7: DropResult JSON tags — ensure snake_case serialization
// ---------------------------------------------------------------------------

func TestDropResultJSONTags(t *testing.T) {
	drop := DropResult{ItemType: 1, ItemID: 5, Quantity: 2}
	data, err := json.Marshal(drop)
	if err != nil {
		t.Fatal(err)
	}
	s := string(data)
	if !strings.Contains(s, `"item_type"`) {
		t.Errorf("JSON should contain item_type, got: %s", s)
	}
	if !strings.Contains(s, `"item_id"`) {
		t.Errorf("JSON should contain item_id, got: %s", s)
	}
	if !strings.Contains(s, `"quantity"`) {
		t.Errorf("JSON should contain quantity, got: %s", s)
	}
}

// ---------------------------------------------------------------------------
// Bug #8: ActionResultTarget should include tp_after
// ---------------------------------------------------------------------------

func TestActionResultTargetHasTPAfter(t *testing.T) {
	art := ActionResultTarget{
		Target:  BattlerRef{Index: 0, IsActor: true, Name: "Hero"},
		Damage:  10,
		HPAfter: 90,
		MPAfter: 50,
		TPAfter: 25,
	}
	data, err := json.Marshal(art)
	if err != nil {
		t.Fatal(err)
	}
	s := string(data)
	if !strings.Contains(s, `"tp_after":25`) {
		t.Errorf("JSON should contain tp_after:25, got: %s", s)
	}
}
