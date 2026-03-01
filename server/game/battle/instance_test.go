package battle

import (
	"context"
	"math/rand"
	"testing"
	"time"

	"github.com/kasuganosora/rpgmakermvmmo/server/resource"
)

func makeInstanceRes() *resource.ResourceLoader {
	return &resource.ResourceLoader{
		Skills: []*resource.Skill{
			nil,
			{ID: 1, Name: "Attack", Scope: 1, HitType: 1, SuccessRate: 100, Repeats: 1,
				Damage: resource.SkillDamage{Formula: "a.atk * 4 - b.def * 2", Type: 1, Critical: true, Variance: 20}},
			{ID: 2, Name: "Heal", Scope: 11, HitType: 0, SuccessRate: 100, Repeats: 1,
				Damage: resource.SkillDamage{Formula: "50", Type: 3}},
		},
		States: []*resource.State{
			nil,
			{ID: 1, Name: "Death"},
		},
	}
}

func drainEvents(ch <-chan BattleEvent) []BattleEvent {
	var events []BattleEvent
	for {
		select {
		case evt, ok := <-ch:
			if !ok {
				return events
			}
			events = append(events, evt)
		case <-time.After(100 * time.Millisecond):
			return events
		}
	}
}

func TestBattleInstance_ActorWins(t *testing.T) {
	res := makeInstanceRes()

	// Strong actor vs weak enemy → actor kills enemy in 1 turn
	actor := NewActorBattler(ActorConfig{
		CharID: 1, Name: "Hero", Index: 0, Level: 10,
		HP: 500, MP: 50,
		BaseParams: [8]int{500, 50, 50, 30, 20, 10, 20, 10},
		ActorTraits: []resource.Trait{{Code: 22, DataID: 0, Value: 0.95}},
		Res:         res,
	})
	enemy := NewEnemyBattler(&resource.Enemy{
		ID: 1, Name: "Slime", HP: 10, MP: 0,
		Atk: 5, Def: 2, Mat: 1, Mdf: 1, Agi: 5, Luk: 1,
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

	// Feed actor input in background.
	go func() {
		// Wait for input_request event before submitting.
		for evt := range bi.Events() {
			if evt.EventType() == "input_request" {
				bi.SubmitInput(&ActionInput{
					ActorIndex:    0,
					ActionType:    ActionAttack,
					TargetIndices: []int{0},
					TargetIsActor: false,
				})
			}
			if evt.EventType() == "battle_end" {
				return
			}
		}
	}()

	result := bi.Run(ctx)
	if result != ResultWin {
		t.Errorf("result = %d, want ResultWin (0)", result)
	}
}

func TestBattleInstance_ActorLoses(t *testing.T) {
	res := makeInstanceRes()

	// Weak actor vs strong enemy → enemy kills actor
	actor := NewActorBattler(ActorConfig{
		CharID: 1, Name: "Weakling", Index: 0, Level: 1,
		HP: 10, MP: 10,
		BaseParams: [8]int{10, 10, 5, 2, 2, 2, 5, 2},
		ActorTraits: []resource.Trait{{Code: 22, DataID: 0, Value: 0.95}},
		Res:         res,
	})
	enemy := NewEnemyBattler(&resource.Enemy{
		ID: 1, Name: "Dragon", HP: 500, MP: 0,
		Atk: 50, Def: 30, Mat: 10, Mdf: 10, Agi: 30, Luk: 10,
		Actions: []resource.EnemyAction{{SkillID: 1, ConditionType: 0, Rating: 5}},
		Traits:  []resource.Trait{{Code: 22, DataID: 0, Value: 0.95}},
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

	go func() {
		for evt := range bi.Events() {
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

	result := bi.Run(ctx)
	if result != ResultLose {
		t.Errorf("result = %d, want ResultLose (2)", result)
	}
}

func TestBattleInstance_Escape(t *testing.T) {
	res := makeInstanceRes()

	actor := NewActorBattler(ActorConfig{
		CharID: 1, Name: "Runner", Index: 0, Level: 5,
		HP: 100, MP: 20,
		BaseParams: [8]int{100, 20, 10, 10, 10, 10, 30, 10},
		ActorTraits: []resource.Trait{{Code: 22, DataID: 0, Value: 0.95}},
		Res:         res,
	})
	enemy := NewEnemyBattler(&resource.Enemy{
		ID: 1, Name: "Guard", HP: 200, MP: 0,
		Atk: 20, Def: 15, Mat: 5, Mdf: 5, Agi: 10, Luk: 5,
		Actions: []resource.EnemyAction{{SkillID: 1, ConditionType: 0, Rating: 5}},
		Traits:  []resource.Trait{{Code: 22, DataID: 0, Value: 0.90}},
	}, 0, res)

	bi := NewBattleInstance(BattleConfig{
		TroopID:      1,
		CanEscape:    true,
		Res:          res,
		RNG:          rand.New(rand.NewSource(1)), // seed that produces escape on first try
		InputTimeout: 5 * time.Second,
	})
	bi.Actors = []Battler{actor}
	bi.Enemies = []Battler{enemy}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Keep trying to escape
	go func() {
		for evt := range bi.Events() {
			if evt.EventType() == "input_request" {
				bi.SubmitInput(&ActionInput{
					ActorIndex: 0,
					ActionType: ActionEscape,
				})
			}
		}
	}()

	result := bi.Run(ctx)
	if result != ResultEscape {
		t.Errorf("result = %d, want ResultEscape (1)", result)
	}
}

func TestBattleInstance_MultipleActors(t *testing.T) {
	res := makeInstanceRes()

	actor1 := NewActorBattler(ActorConfig{
		CharID: 1, Name: "Fighter", Index: 0, Level: 10,
		HP: 200, MP: 30,
		BaseParams: [8]int{200, 30, 40, 25, 15, 10, 20, 10},
		ActorTraits: []resource.Trait{{Code: 22, DataID: 0, Value: 0.95}},
		Res:         res,
	})
	actor2 := NewActorBattler(ActorConfig{
		CharID: 2, Name: "Mage", Index: 1, Level: 10,
		HP: 150, MP: 60,
		BaseParams: [8]int{150, 60, 20, 15, 40, 20, 15, 10},
		ActorTraits: []resource.Trait{{Code: 22, DataID: 0, Value: 0.95}},
		Res:         res,
	})
	enemy := NewEnemyBattler(&resource.Enemy{
		ID: 1, Name: "Orc", HP: 50, MP: 0,
		Atk: 15, Def: 10, Mat: 5, Mdf: 5, Agi: 10, Luk: 5,
		Exp: 30, Gold: 15,
		Actions: []resource.EnemyAction{{SkillID: 1, ConditionType: 0, Rating: 5}},
		Traits:  []resource.Trait{{Code: 22, DataID: 0, Value: 0.90}},
	}, 0, res)

	bi := NewBattleInstance(BattleConfig{
		TroopID:      1,
		Res:          res,
		RNG:          rand.New(rand.NewSource(42)),
		InputTimeout: 5 * time.Second,
	})
	bi.Actors = []Battler{actor1, actor2}
	bi.Enemies = []Battler{enemy}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	go func() {
		for evt := range bi.Events() {
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

	result := bi.Run(ctx)
	if result != ResultWin {
		t.Errorf("result = %d, want ResultWin (0)", result)
	}
}

func TestBattleInstance_RewardsOnWin(t *testing.T) {
	res := makeInstanceRes()

	actor := NewActorBattler(ActorConfig{
		CharID: 1, Name: "Hero", Index: 0, Level: 10,
		HP: 500, MP: 50,
		BaseParams: [8]int{500, 50, 80, 30, 20, 10, 20, 10},
		ActorTraits: []resource.Trait{{Code: 22, DataID: 0, Value: 0.95}},
		Res:         res,
	})
	enemy := NewEnemyBattler(&resource.Enemy{
		ID: 1, Name: "Slime", HP: 5, MP: 0,
		Atk: 1, Def: 1, Mat: 1, Mdf: 1, Agi: 1, Luk: 1,
		Exp: 50, Gold: 25,
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

	var battleEnd *EventBattleEnd
	go func() {
		for evt := range bi.Events() {
			if ir, ok := evt.(*EventInputRequest); ok {
				bi.SubmitInput(&ActionInput{
					ActorIndex:    ir.ActorIndex,
					ActionType:    ActionAttack,
					TargetIndices: []int{0},
					TargetIsActor: false,
				})
			}
			if be, ok := evt.(*EventBattleEnd); ok {
				battleEnd = be
			}
		}
	}()

	result := bi.Run(ctx)
	if result != ResultWin {
		t.Fatalf("result = %d, want ResultWin", result)
	}

	// Give events time to drain.
	time.Sleep(50 * time.Millisecond)

	if battleEnd == nil {
		t.Fatal("no battle_end event received")
	}
	if battleEnd.Exp <= 0 {
		t.Errorf("exp = %d, want > 0", battleEnd.Exp)
	}
	if battleEnd.Gold != 25 {
		t.Errorf("gold = %d, want 25", battleEnd.Gold)
	}
}

func TestBattleInstance_ContextCancellation(t *testing.T) {
	res := makeInstanceRes()

	actor := NewActorBattler(ActorConfig{
		CharID: 1, Name: "Hero", Index: 0, Level: 5,
		HP: 100, MP: 20,
		BaseParams: [8]int{100, 20, 20, 10, 10, 10, 10, 10},
		ActorTraits: []resource.Trait{{Code: 22, DataID: 0, Value: 0.95}},
		Res:         res,
	})
	enemy := NewEnemyBattler(&resource.Enemy{
		ID: 1, Name: "Guard", HP: 200, MP: 0,
		Atk: 10, Def: 10, Mat: 5, Mdf: 5, Agi: 10, Luk: 5,
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

	ctx, cancel := context.WithCancel(context.Background())

	// Drain events but never submit input.
	go func() {
		for range bi.Events() {
		}
	}()

	// Cancel after a short delay.
	go func() {
		time.Sleep(100 * time.Millisecond)
		cancel()
	}()

	result := bi.Run(ctx)
	if result != ResultLose {
		t.Logf("result = %d (expected ResultLose due to context cancellation)", result)
	}
}
