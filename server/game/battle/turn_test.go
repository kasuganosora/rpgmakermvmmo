package battle

import (
	"math/rand"
	"testing"

	"github.com/kasuganosora/rpgmakermvmmo/server/resource"
)

func TestDefaultTurnManagerOrder(t *testing.T) {
	res := makeTestRes()

	// Actor with AGI=50, Enemy with AGI=10
	fast := NewActorBattler(ActorConfig{
		Name:       "Fast",
		Index:      0,
		BaseParams: [8]int{100, 50, 20, 10, 10, 10, 50, 10},
		HP:         100,
		Res:        res,
	})
	slow := NewEnemyBattler(&resource.Enemy{
		ID: 1, Name: "Slow", HP: 100, MP: 10,
		Atk: 10, Def: 5, Mat: 5, Mdf: 5, Agi: 10, Luk: 5,
	}, 0, res)

	tm := DefaultTurnManager{}
	rng := rand.New(rand.NewSource(42))

	order := tm.MakeActionOrder([]Battler{fast}, []Battler{slow}, rng)
	if len(order) != 2 {
		t.Fatalf("len = %d, want 2", len(order))
	}
	// Fast (AGI=50) should almost always go first with seed 42
	if order[0].Name() != "Fast" {
		t.Errorf("first = %s, want Fast", order[0].Name())
	}
}

func TestDefaultTurnManagerSkipsDead(t *testing.T) {
	res := makeTestRes()

	alive := NewActorBattler(ActorConfig{
		Name:       "Alive",
		Index:      0,
		HP:         100,
		BaseParams: [8]int{100, 50, 20, 10, 10, 10, 20, 10},
		Res:        res,
	})
	dead := NewActorBattler(ActorConfig{
		Name:       "Dead",
		Index:      1,
		HP:         0,
		BaseParams: [8]int{100, 50, 20, 10, 10, 10, 20, 10},
		Res:        res,
	})

	tm := DefaultTurnManager{}
	rng := rand.New(rand.NewSource(1))

	order := tm.MakeActionOrder([]Battler{alive, dead}, nil, rng)
	if len(order) != 1 {
		t.Fatalf("len = %d, want 1 (dead excluded)", len(order))
	}
	if order[0].Name() != "Alive" {
		t.Errorf("expected Alive, got %s", order[0].Name())
	}
}

func TestDefaultTurnManagerDeterministic(t *testing.T) {
	res := makeTestRes()

	a := NewActorBattler(ActorConfig{
		Name: "A", Index: 0, HP: 100,
		BaseParams: [8]int{100, 50, 20, 10, 10, 10, 20, 10},
		Res:        res,
	})
	b := NewEnemyBattler(&resource.Enemy{
		ID: 2, Name: "B", HP: 100, MP: 10,
		Atk: 10, Def: 5, Mat: 5, Mdf: 5, Agi: 20, Luk: 5,
	}, 0, res)

	tm := DefaultTurnManager{}

	// Same seed → same order
	rng1 := rand.New(rand.NewSource(99))
	order1 := tm.MakeActionOrder([]Battler{a}, []Battler{b}, rng1)

	rng2 := rand.New(rand.NewSource(99))
	order2 := tm.MakeActionOrder([]Battler{a}, []Battler{b}, rng2)

	if order1[0].Name() != order2[0].Name() || order1[1].Name() != order2[1].Name() {
		t.Error("same seed should produce same order")
	}
}
