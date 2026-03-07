package battle

import (
	"math/rand"
	"testing"

	"github.com/kasuganosora/rpgmakermvmmo/server/resource"
	"go.uber.org/zap"
)

func TestTroopEventRunner_TurnCondition(t *testing.T) {
	var emitted []BattleEvent
	runner := NewTroopEventRunner(TroopEventConfig{
		Pages: []resource.TroopPage{
			{
				Conditions: resource.TroopPageConditions{TurnValid: true, TurnA: 1, TurnB: 0},
				Span:       0, // battle — execute once
				List: []resource.EventCommand{
					{Code: 121, Parameters: []interface{}{float64(10), float64(10), float64(0)}}, // Switch 10 ON
					{Code: 0},
				},
			},
		},
		Res:    makeTestRes(),
		RNG:    rand.New(rand.NewSource(42)),
		Logger: zap.NewNop(),
		Emit: func(evt BattleEvent) {
			emitted = append(emitted, evt)
		},
	})

	// Turn 0 — should not match (turnA=1)
	runner.RunTurnStart(0)
	if runner.switches[10] {
		t.Error("switch 10 should not be set on turn 0")
	}

	// Turn 1 — should match
	runner.RunTurnStart(1)
	if !runner.switches[10] {
		t.Error("switch 10 should be set on turn 1")
	}

	// Turn 1 again — span=0 means already executed, should not re-execute
	runner.switches[10] = false
	runner.RunTurnStart(1)
	if runner.switches[10] {
		t.Error("span=0 page should not execute again")
	}
}

func TestTroopEventRunner_TurnBRepeating(t *testing.T) {
	runner := NewTroopEventRunner(TroopEventConfig{
		Pages: []resource.TroopPage{
			{
				Conditions: resource.TroopPageConditions{TurnValid: true, TurnA: 0, TurnB: 2},
				Span:       1, // turn — repeat each matching turn
				List: []resource.EventCommand{
					{Code: 121, Parameters: []interface{}{float64(5), float64(5), float64(0)}},
					{Code: 0},
				},
			},
		},
		Res:    makeTestRes(),
		RNG:    rand.New(rand.NewSource(42)),
		Logger: zap.NewNop(),
	})

	// turnA=0, turnB=2 → matches turns 0, 2, 4, 6...
	runner.RunTurnStart(0)
	if !runner.switches[5] {
		t.Error("should match turn 0")
	}

	runner.switches[5] = false
	runner.RunTurnStart(1)
	if runner.switches[5] {
		t.Error("should NOT match turn 1")
	}

	runner.RunTurnStart(2)
	if !runner.switches[5] {
		t.Error("should match turn 2")
	}
}

func TestTroopEventRunner_EnemyHPCondition(t *testing.T) {
	runner := NewTroopEventRunner(TroopEventConfig{
		Pages: []resource.TroopPage{
			{
				Conditions: resource.TroopPageConditions{EnemyValid: true, EnemyIndex: 0, EnemyHp: 50},
				Span:       0,
				List: []resource.EventCommand{
					{Code: 121, Parameters: []interface{}{float64(20), float64(20), float64(0)}},
					{Code: 0},
				},
			},
		},
		Res:    makeTestRes(),
		RNG:    rand.New(rand.NewSource(42)),
		Logger: zap.NewNop(),
		GetEnemyHP: func(index int) (int, bool) {
			if index == 0 {
				return 30, true // 30% HP
			}
			return 100, true
		},
	})

	// Enemy 0 at 30% HP, threshold is 50% → condition met
	runner.RunTurnStart(1)
	if !runner.switches[20] {
		t.Error("should trigger when enemy HP <= threshold")
	}
}

func TestTroopEventRunner_ConditionalBranch(t *testing.T) {
	runner := NewTroopEventRunner(TroopEventConfig{
		Pages: []resource.TroopPage{
			{
				Conditions: resource.TroopPageConditions{TurnValid: true, TurnA: 0, TurnB: 0},
				Span:       1,
				List: []resource.EventCommand{
					// if switch 1 == ON
					{Code: 111, Indent: 0, Parameters: []interface{}{float64(0), float64(1), float64(0)}},
					// then: set switch 50 ON
					{Code: 121, Indent: 1, Parameters: []interface{}{float64(50), float64(50), float64(0)}},
					// else
					{Code: 411, Indent: 0},
					// set switch 51 ON
					{Code: 121, Indent: 1, Parameters: []interface{}{float64(51), float64(51), float64(0)}},
					// end branch
					{Code: 412, Indent: 0},
					{Code: 0},
				},
			},
		},
		Res:    makeTestRes(),
		RNG:    rand.New(rand.NewSource(42)),
		Logger: zap.NewNop(),
	})

	// Switch 1 is OFF → should take else branch → switch 51
	runner.RunTurnStart(0)
	if runner.switches[50] {
		t.Error("switch 50 should NOT be set (condition false)")
	}
	if !runner.switches[51] {
		t.Error("switch 51 should be set (else branch)")
	}

	// Now set switch 1 ON and re-run
	runner.switches[1] = true
	runner.switches[50] = false
	runner.switches[51] = false
	runner.RunTurnStart(0)
	if !runner.switches[50] {
		t.Error("switch 50 should be set (condition true)")
	}
	if runner.switches[51] {
		t.Error("switch 51 should NOT be set (skipped else)")
	}
}

func TestTroopEventRunner_ControlVariables(t *testing.T) {
	runner := NewTroopEventRunner(TroopEventConfig{
		Pages: []resource.TroopPage{
			{
				Conditions: resource.TroopPageConditions{TurnValid: true, TurnA: 0, TurnB: 0},
				Span:       1,
				List: []resource.EventCommand{
					// Set var 100 = 42
					{Code: 122, Parameters: []interface{}{float64(100), float64(100), float64(0), float64(0), float64(42)}},
					// Add 8 to var 100
					{Code: 122, Parameters: []interface{}{float64(100), float64(100), float64(1), float64(0), float64(8)}},
					{Code: 0},
				},
			},
		},
		Res:    makeTestRes(),
		RNG:    rand.New(rand.NewSource(42)),
		Logger: zap.NewNop(),
	})

	runner.RunTurnStart(0)
	if runner.variables[100] != 50 {
		t.Errorf("var 100 should be 50, got %d", runner.variables[100])
	}
}
