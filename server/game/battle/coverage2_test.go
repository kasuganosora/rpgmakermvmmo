package battle

// coverage2_test.go — Additional tests to push battle module coverage toward 90%+.
// Covers: troop_event handlers, ai.go target resolution, instance.go helpers,
// action.go item processing, formula custom functions, events.go EventType methods.

import (
	"context"
	"fmt"
	"math/rand"
	"testing"
	"time"

	"github.com/kasuganosora/rpgmakermvmmo/server/resource"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

// ═══════════════════════════════════════════════════════════════════════════
//  Troop Event Runner helpers
// ═══════════════════════════════════════════════════════════════════════════

func makeTroopRunner(pages []resource.TroopPage) (*TroopEventRunner, *troopCallLog) {
	log := &troopCallLog{}
	res := makeExtendedRes()
	// Add common events for testing.
	res.CommonEvents = make([]*resource.CommonEvent, 20)
	res.CommonEvents[1] = &resource.CommonEvent{
		ID:   1,
		Name: "TestCE",
		List: []*resource.EventCommand{
			{Code: 121, Parameters: []interface{}{float64(99), float64(99), float64(0)}}, // switch 99 ON
			{Code: 0},
		},
	}

	runner := NewTroopEventRunner(TroopEventConfig{
		Pages:    pages,
		Res:      res,
		RNG:      rand.New(rand.NewSource(42)),
		Logger:   zap.NewNop(),
		GameVars: map[int]int{10: 100, 20: 200},
		GetEnemyHP: func(index int) (int, bool) {
			if index == 0 {
				return 50, true
			}
			if index == 1 {
				return 0, false
			}
			return 100, true
		},
		GetActorHP: func(actorID int) (int, bool) {
			if actorID == 1 {
				return 80, true
			}
			return 0, false
		},
		Emit: func(evt BattleEvent) {
			log.events = append(log.events, evt)
		},
		AddState: func(isActor bool, index int, stateID int) {
			log.stateAdds = append(log.stateAdds, stateChange{isActor, index, stateID})
		},
		RemoveState: func(isActor bool, index int, stateID int) {
			log.stateRemoves = append(log.stateRemoves, stateChange{isActor, index, stateID})
		},
		ChangeEnemyHP: func(enemyIndex int, value int) {
			log.hpChanges = append(log.hpChanges, hpChange{enemyIndex, value})
		},
		ChangeEnemyMP: func(enemyIndex int, value int) {
			log.mpChanges = append(log.mpChanges, mpChange{enemyIndex, value})
		},
		ChangeEnemyTP: func(enemyIndex int, value int) {
			log.tpChanges = append(log.tpChanges, tpChange{enemyIndex, value})
		},
		TransformEnemy: func(enemyIndex int, newEnemyID int) {
			log.transforms = append(log.transforms, transform{enemyIndex, newEnemyID})
		},
		RecoverEnemy: func(enemyIndex int) {
			log.recovers = append(log.recovers, enemyIndex)
		},
		AbortFn: func() {
			log.aborted = true
		},
		WaitForAck: func() {
			log.ackCount++
		},
		ActorCount: func() int { return 2 },
		ActorDBID:  func(index int) int { return index + 1 }, // actor 0 → DB ID 1, actor 1 → DB ID 2
	})
	return runner, log
}

type stateChange struct {
	isActor bool
	index   int
	stateID int
}

type hpChange struct {
	index int
	value int
}

type mpChange struct {
	index int
	value int
}

type tpChange struct {
	index int
	value int
}

type transform struct {
	index      int
	newEnemyID int
}

type troopCallLog struct {
	events       []BattleEvent
	stateAdds    []stateChange
	stateRemoves []stateChange
	hpChanges    []hpChange
	mpChanges    []mpChange
	tpChanges    []tpChange
	transforms   []transform
	recovers     []int
	aborted      bool
	ackCount     int
}

// ═══════════════════════════════════════════════════════════════════════════
//  Troop Event: executeCommands tests
// ═══════════════════════════════════════════════════════════════════════════

func TestTroopEvent_ChangeEnemyHP_SingleAndAll(t *testing.T) {
	runner, log := makeTroopRunner(nil)

	// Code 331: Change Enemy HP, single enemy, decrease constant 50
	cmds := []resource.EventCommand{
		{Code: 331, Parameters: []interface{}{float64(0), float64(1), float64(0), float64(50)}},
		{Code: 0},
	}
	runner.executeCommands(cmds)
	require.Len(t, log.hpChanges, 1)
	assert.Equal(t, 0, log.hpChanges[0].index)
	assert.Equal(t, -50, log.hpChanges[0].value)

	// All enemies (index=-1), increase constant 30
	log.hpChanges = nil
	cmds = []resource.EventCommand{
		{Code: 331, Parameters: []interface{}{float64(-1), float64(0), float64(0), float64(30)}},
		{Code: 0},
	}
	runner.executeCommands(cmds)
	assert.Len(t, log.hpChanges, 8) // iterates 0..7
	assert.Equal(t, 30, log.hpChanges[0].value)
}

func TestTroopEvent_ChangeEnemyHP_VarOperand(t *testing.T) {
	runner, log := makeTroopRunner(nil)

	// operandType=1 (variable), operand=10 (var[10]=100), decrease
	cmds := []resource.EventCommand{
		{Code: 331, Parameters: []interface{}{float64(0), float64(1), float64(1), float64(10)}},
		{Code: 0},
	}
	runner.executeCommands(cmds)
	require.Len(t, log.hpChanges, 1)
	assert.Equal(t, -100, log.hpChanges[0].value)
}

func TestTroopEvent_ChangeEnemyMP(t *testing.T) {
	runner, log := makeTroopRunner(nil)

	// Single enemy, increase 20
	cmds := []resource.EventCommand{
		{Code: 332, Parameters: []interface{}{float64(0), float64(0), float64(0), float64(20)}},
		{Code: 0},
	}
	runner.executeCommands(cmds)
	require.Len(t, log.mpChanges, 1)
	assert.Equal(t, 20, log.mpChanges[0].value)

	// All enemies, decrease 15
	log.mpChanges = nil
	cmds = []resource.EventCommand{
		{Code: 332, Parameters: []interface{}{float64(-1), float64(1), float64(0), float64(15)}},
		{Code: 0},
	}
	runner.executeCommands(cmds)
	assert.Len(t, log.mpChanges, 8)
	assert.Equal(t, -15, log.mpChanges[0].value)
}

func TestTroopEvent_ChangeEnemyTP(t *testing.T) {
	runner, log := makeTroopRunner(nil)

	// Single enemy, increase 10
	cmds := []resource.EventCommand{
		{Code: 342, Parameters: []interface{}{float64(0), float64(0), float64(0), float64(10)}},
		{Code: 0},
	}
	runner.executeCommands(cmds)
	require.Len(t, log.tpChanges, 1)
	assert.Equal(t, 10, log.tpChanges[0].value)

	// All enemies, decrease variable
	log.tpChanges = nil
	cmds = []resource.EventCommand{
		{Code: 342, Parameters: []interface{}{float64(-1), float64(1), float64(1), float64(20)}},
		{Code: 0},
	}
	runner.executeCommands(cmds)
	assert.Len(t, log.tpChanges, 8)
	assert.Equal(t, -200, log.tpChanges[0].value) // var[20]=200
}

func TestTroopEvent_ChangeEnemyState(t *testing.T) {
	runner, log := makeTroopRunner(nil)

	// Add state 2 to enemy 0
	cmds := []resource.EventCommand{
		{Code: 333, Parameters: []interface{}{float64(0), float64(0), float64(2)}},
		{Code: 0},
	}
	runner.executeCommands(cmds)
	require.Len(t, log.stateAdds, 1)
	assert.Equal(t, stateChange{false, 0, 2}, log.stateAdds[0])

	// Remove state 2 from all enemies
	log.stateRemoves = nil
	cmds = []resource.EventCommand{
		{Code: 333, Parameters: []interface{}{float64(-1), float64(1), float64(2)}},
		{Code: 0},
	}
	runner.executeCommands(cmds)
	assert.Len(t, log.stateRemoves, 8)
}

func TestTroopEvent_ChangeState_ActorScope(t *testing.T) {
	runner, log := makeTroopRunner(nil)

	// Code 313: Change State — scope=0 (fixed), actorID=0 (all), add state 3
	cmds := []resource.EventCommand{
		{Code: 313, Parameters: []interface{}{float64(0), float64(0), float64(0), float64(3)}},
		{Code: 0},
	}
	runner.executeCommands(cmds)
	require.Len(t, log.stateAdds, 2) // 2 actors
	assert.True(t, log.stateAdds[0].isActor)

	// Specific actor by DB ID (actorID=1 → battle index 0)
	log.stateAdds = nil
	cmds = []resource.EventCommand{
		{Code: 313, Parameters: []interface{}{float64(0), float64(1), float64(0), float64(4)}},
		{Code: 0},
	}
	runner.executeCommands(cmds)
	require.Len(t, log.stateAdds, 1)
	assert.Equal(t, 0, log.stateAdds[0].index)

	// Remove state, scope=1 (variable lookup), var[10]=100 → actorID=100 (not found)
	log.stateRemoves = nil
	cmds = []resource.EventCommand{
		{Code: 313, Parameters: []interface{}{float64(1), float64(10), float64(1), float64(3)}},
		{Code: 0},
	}
	runner.executeCommands(cmds)
	assert.Len(t, log.stateRemoves, 0) // actor 100 not found
}

func TestTroopEvent_EnemyTransform(t *testing.T) {
	runner, log := makeTroopRunner(nil)

	cmds := []resource.EventCommand{
		{Code: 336, Parameters: []interface{}{float64(0), float64(2)}},
		{Code: 0},
	}
	runner.executeCommands(cmds)
	require.Len(t, log.transforms, 1)
	assert.Equal(t, transform{0, 2}, log.transforms[0])
	// Also emits event for client
	require.Len(t, log.events, 1)
	assert.Equal(t, "troop_command", log.events[0].EventType())
}

func TestTroopEvent_EnemyRecoverAll(t *testing.T) {
	runner, log := makeTroopRunner(nil)

	// Single enemy
	cmds := []resource.EventCommand{
		{Code: 334, Parameters: []interface{}{float64(0)}},
		{Code: 0},
	}
	runner.executeCommands(cmds)
	require.Len(t, log.recovers, 1)
	assert.Equal(t, 0, log.recovers[0])

	// All enemies
	log.recovers = nil
	cmds = []resource.EventCommand{
		{Code: 334, Parameters: []interface{}{float64(-1)}},
		{Code: 0},
	}
	runner.executeCommands(cmds)
	assert.Len(t, log.recovers, 8)
}

func TestTroopEvent_AbortBattle(t *testing.T) {
	runner, log := makeTroopRunner(nil)

	cmds := []resource.EventCommand{
		{Code: 340},
		{Code: 121, Parameters: []interface{}{float64(1), float64(1), float64(0)}}, // should NOT run
		{Code: 0},
	}
	runner.executeCommands(cmds)
	assert.True(t, log.aborted)
}

func TestTroopEvent_ExitEventProcessing(t *testing.T) {
	runner, log := makeTroopRunner(nil)

	cmds := []resource.EventCommand{
		{Code: 115}, // Exit Event Processing
		{Code: 331, Parameters: []interface{}{float64(0), float64(1), float64(0), float64(50)}}, // should NOT run
		{Code: 0},
	}
	runner.executeCommands(cmds)
	assert.Len(t, log.hpChanges, 0)
}

func TestTroopEvent_CommonEvent(t *testing.T) {
	runner, _ := makeTroopRunner(nil)

	cmds := []resource.EventCommand{
		{Code: 117, Parameters: []interface{}{float64(1)}}, // CE 1 sets switch 99 ON
		{Code: 0},
	}
	runner.executeCommands(cmds)
	assert.True(t, runner.switches[99])
}

func TestTroopEvent_ShowText(t *testing.T) {
	runner, log := makeTroopRunner(nil)

	cmds := []resource.EventCommand{
		{Code: 101, Parameters: []interface{}{"", float64(0), float64(0), float64(0)}},
		{Code: 401, Parameters: []interface{}{"Hello, world!"}},
		{Code: 401, Parameters: []interface{}{"Second line."}},
		{Code: 0},
	}
	runner.executeCommands(cmds)
	require.Len(t, log.events, 1)
	assert.Equal(t, 1, log.ackCount)
}

func TestTroopEvent_Script_SwitchAndVar(t *testing.T) {
	runner, _ := makeTroopRunner(nil)

	// Script: set switch
	cmds := []resource.EventCommand{
		{Code: 355, Parameters: []interface{}{"$gameSwitches.setValue(42, true)"}},
		{Code: 0},
	}
	runner.executeCommands(cmds)
	assert.True(t, runner.switches[42])

	// Script: set variable
	cmds = []resource.EventCommand{
		{Code: 355, Parameters: []interface{}{"$gameVariables.setValue(50, 999)"}},
		{Code: 0},
	}
	runner.executeCommands(cmds)
	assert.Equal(t, 999, runner.variables[50])

	// Unknown script forwards to client
	cmds = []resource.EventCommand{
		{Code: 355, Parameters: []interface{}{"someUnknownScript()"}},
		{Code: 0},
	}
	runner.executeCommands(cmds)
	// Should emit event
}

func TestTroopEvent_Script_Continuation(t *testing.T) {
	runner, _ := makeTroopRunner(nil)

	// Script with continuation lines (code 655)
	cmds := []resource.EventCommand{
		{Code: 355, Parameters: []interface{}{"$gameSwitches.setValue("}},
		{Code: 655, Parameters: []interface{}{"77, true)"}},
		{Code: 0},
	}
	runner.executeCommands(cmds)
	// The concatenated script won't match the Sscanf pattern exactly, but should not crash
}

func TestTroopEvent_PluginCommand(t *testing.T) {
	runner, log := makeTroopRunner(nil)

	cmds := []resource.EventCommand{
		{Code: 356, Parameters: []interface{}{"SomePlugin Arg1 Arg2"}},
		{Code: 0},
	}
	runner.executeCommands(cmds)
	require.Len(t, log.events, 1)
	assert.Equal(t, "troop_command", log.events[0].EventType())
}

func TestTroopEvent_AudioCommands(t *testing.T) {
	runner, log := makeTroopRunner(nil)

	// Test all audio-forwarding codes
	audioCodes := []int{250, 241, 242, 245, 246, 249}
	for _, code := range audioCodes {
		cmds := []resource.EventCommand{
			{Code: code, Parameters: []interface{}{"audio_file"}},
			{Code: 0},
		}
		runner.executeCommands(cmds)
	}
	assert.Len(t, log.events, len(audioCodes))
}

func TestTroopEvent_BattleAnimationAndForceAction(t *testing.T) {
	runner, log := makeTroopRunner(nil)

	cmds := []resource.EventCommand{
		{Code: 335, Parameters: []interface{}{float64(0)}},  // Enemy Appear
		{Code: 337, Parameters: []interface{}{float64(0), float64(1)}},  // Show Battle Animation
		{Code: 339, Parameters: []interface{}{float64(0), float64(1), float64(0)}},  // Force Action
		{Code: 0},
	}
	runner.executeCommands(cmds)
	assert.Len(t, log.events, 3)
}

func TestTroopEvent_ControlVariables_Operations(t *testing.T) {
	runner, _ := makeTroopRunner(nil)

	// Set var 1 = 100
	cmds := []resource.EventCommand{
		{Code: 122, Parameters: []interface{}{float64(1), float64(1), float64(0), float64(0), float64(100)}},
		{Code: 0},
	}
	runner.executeCommands(cmds)
	assert.Equal(t, 100, runner.variables[1])

	// Add 20
	cmds = []resource.EventCommand{
		{Code: 122, Parameters: []interface{}{float64(1), float64(1), float64(1), float64(0), float64(20)}},
		{Code: 0},
	}
	runner.executeCommands(cmds)
	assert.Equal(t, 120, runner.variables[1])

	// Sub 30
	cmds = []resource.EventCommand{
		{Code: 122, Parameters: []interface{}{float64(1), float64(1), float64(2), float64(0), float64(30)}},
		{Code: 0},
	}
	runner.executeCommands(cmds)
	assert.Equal(t, 90, runner.variables[1])

	// Mul 2
	cmds = []resource.EventCommand{
		{Code: 122, Parameters: []interface{}{float64(1), float64(1), float64(3), float64(0), float64(2)}},
		{Code: 0},
	}
	runner.executeCommands(cmds)
	assert.Equal(t, 180, runner.variables[1])

	// Div 3
	cmds = []resource.EventCommand{
		{Code: 122, Parameters: []interface{}{float64(1), float64(1), float64(4), float64(0), float64(3)}},
		{Code: 0},
	}
	runner.executeCommands(cmds)
	assert.Equal(t, 60, runner.variables[1])

	// Mod 7
	cmds = []resource.EventCommand{
		{Code: 122, Parameters: []interface{}{float64(1), float64(1), float64(5), float64(0), float64(7)}},
		{Code: 0},
	}
	runner.executeCommands(cmds)
	assert.Equal(t, 4, runner.variables[1]) // 60 % 7 = 4

	// Div by zero (no change)
	cmds = []resource.EventCommand{
		{Code: 122, Parameters: []interface{}{float64(1), float64(1), float64(4), float64(0), float64(0)}},
		{Code: 0},
	}
	runner.executeCommands(cmds)
	assert.Equal(t, 4, runner.variables[1])

	// Mod by zero (no change)
	cmds = []resource.EventCommand{
		{Code: 122, Parameters: []interface{}{float64(1), float64(1), float64(5), float64(0), float64(0)}},
		{Code: 0},
	}
	runner.executeCommands(cmds)
	assert.Equal(t, 4, runner.variables[1])
}

func TestTroopEvent_ControlVariables_OperandTypes(t *testing.T) {
	runner, _ := makeTroopRunner(nil)

	// operandType=1 (variable): set var[5] = var[10] (which is 100)
	cmds := []resource.EventCommand{
		{Code: 122, Parameters: []interface{}{float64(5), float64(5), float64(0), float64(1), float64(10)}},
		{Code: 0},
	}
	runner.executeCommands(cmds)
	assert.Equal(t, 100, runner.variables[5])

	// operandType=2 (random): set var[6] = random(1, 10)
	cmds = []resource.EventCommand{
		{Code: 122, Parameters: []interface{}{float64(6), float64(6), float64(0), float64(2), float64(1), float64(10)}},
		{Code: 0},
	}
	runner.executeCommands(cmds)
	assert.True(t, runner.variables[6] >= 1 && runner.variables[6] <= 10)
}

func TestTroopEvent_ControlVariables_Range(t *testing.T) {
	runner, _ := makeTroopRunner(nil)

	// Set vars 1-3 = 42
	cmds := []resource.EventCommand{
		{Code: 122, Parameters: []interface{}{float64(1), float64(3), float64(0), float64(0), float64(42)}},
		{Code: 0},
	}
	runner.executeCommands(cmds)
	assert.Equal(t, 42, runner.variables[1])
	assert.Equal(t, 42, runner.variables[2])
	assert.Equal(t, 42, runner.variables[3])
}

// ═══════════════════════════════════════════════════════════════════════════
//  Troop Event: Conditional branch tests
// ═══════════════════════════════════════════════════════════════════════════

func TestTroopEvent_ConditionalBranch_Switch(t *testing.T) {
	runner, _ := makeTroopRunner(nil)
	runner.switches[5] = true

	// condType=0, switchID=5, expected=0 (ON) → true
	assert.True(t, runner.evalConditionalBranch([]interface{}{float64(0), float64(5), float64(0)}))
	// expected=1 (OFF) → false
	assert.False(t, runner.evalConditionalBranch([]interface{}{float64(0), float64(5), float64(1)}))
	// Switch not set → OFF
	assert.False(t, runner.evalConditionalBranch([]interface{}{float64(0), float64(99), float64(0)}))
	assert.True(t, runner.evalConditionalBranch([]interface{}{float64(0), float64(99), float64(1)}))
}

func TestTroopEvent_ConditionalBranch_Variable(t *testing.T) {
	runner, _ := makeTroopRunner(nil)
	runner.variables[1] = 50

	// condType=1, varID=1, operandType=0 (const), operand=50, op=0 (==) → true
	assert.True(t, runner.evalConditionalBranch([]interface{}{float64(1), float64(1), float64(0), float64(50), float64(0)}))
	// op=1 (>=) → true (50>=50)
	assert.True(t, runner.evalConditionalBranch([]interface{}{float64(1), float64(1), float64(0), float64(50), float64(1)}))
	// op=2 (<=) → true (50<=50)
	assert.True(t, runner.evalConditionalBranch([]interface{}{float64(1), float64(1), float64(0), float64(50), float64(2)}))
	// op=3 (>) → false (50>50)
	assert.False(t, runner.evalConditionalBranch([]interface{}{float64(1), float64(1), float64(0), float64(50), float64(3)}))
	// op=4 (<) → false (50<50)
	assert.False(t, runner.evalConditionalBranch([]interface{}{float64(1), float64(1), float64(0), float64(50), float64(4)}))
	// op=5 (!=) → false (50!=50)
	assert.False(t, runner.evalConditionalBranch([]interface{}{float64(1), float64(1), float64(0), float64(50), float64(5)}))
	// op=5 (!=) with different value → true
	assert.True(t, runner.evalConditionalBranch([]interface{}{float64(1), float64(1), float64(0), float64(30), float64(5)}))

	// operandType=1 (variable operand)
	runner.variables[2] = 50
	assert.True(t, runner.evalConditionalBranch([]interface{}{float64(1), float64(1), float64(1), float64(2), float64(0)}))
}

func TestTroopEvent_ConditionalBranch_SelfSwitch(t *testing.T) {
	runner, _ := makeTroopRunner(nil)
	// condType=2 (Self Switch) — always false in battle
	assert.False(t, runner.evalConditionalBranch([]interface{}{float64(2), float64(0)}))
}

func TestTroopEvent_ConditionalBranch_Actor(t *testing.T) {
	runner, _ := makeTroopRunner(nil)
	// condType=4, conditionType=1 (In party) → always true
	assert.True(t, runner.evalConditionalBranch([]interface{}{float64(4), float64(1), float64(1)}))
	// conditionType=4 (State) → simplified false
	assert.False(t, runner.evalConditionalBranch([]interface{}{float64(4), float64(1), float64(4)}))
}

func TestTroopEvent_ConditionalBranch_Enemy(t *testing.T) {
	runner, _ := makeTroopRunner(nil)
	// condType=6, enemyIndex=0, conditionType=0 (Appeared/alive)
	assert.True(t, runner.evalConditionalBranch([]interface{}{float64(6), float64(0), float64(0)}))
	// Dead enemy (index 1)
	assert.False(t, runner.evalConditionalBranch([]interface{}{float64(6), float64(1), float64(0)}))
	// conditionType=1 (State) → simplified false
	assert.False(t, runner.evalConditionalBranch([]interface{}{float64(6), float64(0), float64(1)}))
}

func TestTroopEvent_ConditionalBranch_SwitchType8(t *testing.T) {
	runner, _ := makeTroopRunner(nil)
	runner.switches[10] = true
	// condType=8 → same as switch check
	assert.True(t, runner.evalConditionalBranch([]interface{}{float64(8), float64(10)}))
	assert.False(t, runner.evalConditionalBranch([]interface{}{float64(8), float64(11)}))
}

func TestTroopEvent_ConditionalBranch_ScriptType11(t *testing.T) {
	runner, _ := makeTroopRunner(nil)
	// condType=11 → not implemented, always false
	assert.False(t, runner.evalConditionalBranch([]interface{}{float64(11), float64(0)}))
}

func TestTroopEvent_ConditionalBranch_ScriptType12(t *testing.T) {
	runner, _ := makeTroopRunner(nil)
	runner.switches[165] = true
	// condType=12, script condition
	assert.True(t, runner.evalConditionalBranch([]interface{}{float64(12), "$gameSwitches.value(165)"}))
	assert.False(t, runner.evalConditionalBranch([]interface{}{float64(12), "$gameSwitches.value(999)"}))
	// Unknown script → false
	assert.False(t, runner.evalConditionalBranch([]interface{}{float64(12), "unknownCondition()"}))
}

func TestTroopEvent_ConditionalBranch_TooFewParams(t *testing.T) {
	runner, _ := makeTroopRunner(nil)
	assert.False(t, runner.evalConditionalBranch([]interface{}{float64(0)}))
}

func TestTroopEvent_ConditionalBranch_FlowControl(t *testing.T) {
	runner, _ := makeTroopRunner(nil)
	runner.switches[1] = true

	// If switch 1 ON → set var 1=1, Else → set var 1=2
	cmds := []resource.EventCommand{
		{Code: 111, Indent: 0, Parameters: []interface{}{float64(0), float64(1), float64(0)}},
		{Code: 122, Indent: 1, Parameters: []interface{}{float64(1), float64(1), float64(0), float64(0), float64(1)}},
		{Code: 411, Indent: 0},
		{Code: 122, Indent: 1, Parameters: []interface{}{float64(1), float64(1), float64(0), float64(0), float64(2)}},
		{Code: 412, Indent: 0},
		{Code: 0},
	}
	runner.executeCommands(cmds)
	assert.Equal(t, 1, runner.variables[1]) // if branch taken

	// Now test else branch
	runner.switches[1] = false
	runner.variables[1] = 0
	runner.executeCommands(cmds)
	assert.Equal(t, 2, runner.variables[1]) // else branch taken
}

func TestTroopEvent_ConditionalBranch_NoElse(t *testing.T) {
	runner, _ := makeTroopRunner(nil)
	runner.switches[1] = false

	// If switch 1 ON → set var 1=99, End Branch (no else)
	cmds := []resource.EventCommand{
		{Code: 111, Indent: 0, Parameters: []interface{}{float64(0), float64(1), float64(0)}},
		{Code: 122, Indent: 1, Parameters: []interface{}{float64(1), float64(1), float64(0), float64(0), float64(99)}},
		{Code: 412, Indent: 0},
		{Code: 0},
	}
	runner.executeCommands(cmds)
	assert.Equal(t, 0, runner.variables[1]) // if not taken, no else
}

// ═══════════════════════════════════════════════════════════════════════════
//  Troop Event: checkConditions tests
// ═══════════════════════════════════════════════════════════════════════════

func TestTroopEvent_CheckConditions_TurnEnding(t *testing.T) {
	runner, _ := makeTroopRunner(nil)

	page := &resource.TroopPage{
		Conditions: resource.TroopPageConditions{TurnEnding: true},
	}
	assert.False(t, runner.checkConditions(page, 1, false))
	assert.True(t, runner.checkConditions(page, 1, true))
}

func TestTroopEvent_CheckConditions_TurnValid(t *testing.T) {
	runner, _ := makeTroopRunner(nil)

	page := &resource.TroopPage{
		Conditions: resource.TroopPageConditions{TurnValid: true, TurnA: 1, TurnB: 2},
	}
	// turnA=1, turnB=2 → matches turn 1, 3, 5, 7...
	assert.True(t, runner.checkConditions(page, 1, false))
	assert.False(t, runner.checkConditions(page, 2, false))
	assert.True(t, runner.checkConditions(page, 3, false))
}

func TestTroopEvent_CheckConditions_EnemyHP(t *testing.T) {
	runner, _ := makeTroopRunner(nil)

	page := &resource.TroopPage{
		Conditions: resource.TroopPageConditions{EnemyValid: true, EnemyIndex: 0, EnemyHp: 60},
	}
	// Enemy 0 HP=50%, threshold 60% → 50 <= 60 → true
	assert.True(t, runner.checkConditions(page, 0, false))

	page.Conditions.EnemyHp = 30 // 50 > 30 → false
	assert.False(t, runner.checkConditions(page, 0, false))
}

func TestTroopEvent_CheckConditions_ActorHP(t *testing.T) {
	runner, _ := makeTroopRunner(nil)

	page := &resource.TroopPage{
		Conditions: resource.TroopPageConditions{ActorValid: true, ActorId: 1, ActorHp: 90},
	}
	// Actor 1 HP=80%, threshold 90% → 80 <= 90 → true
	assert.True(t, runner.checkConditions(page, 0, false))

	page.Conditions.ActorHp = 50 // 80 > 50 → false
	assert.False(t, runner.checkConditions(page, 0, false))
}

func TestTroopEvent_CheckConditions_Switch(t *testing.T) {
	runner, _ := makeTroopRunner(nil)

	page := &resource.TroopPage{
		Conditions: resource.TroopPageConditions{SwitchValid: true, SwitchId: 5},
	}
	assert.False(t, runner.checkConditions(page, 0, false))
	runner.switches[5] = true
	assert.True(t, runner.checkConditions(page, 0, false))
}

func TestTroopEvent_RunTurnEnd(t *testing.T) {
	runner, log := makeTroopRunner([]resource.TroopPage{
		{
			Span:       1,
			Conditions: resource.TroopPageConditions{TurnEnding: true},
			List: []resource.EventCommand{
				{Code: 331, Parameters: []interface{}{float64(0), float64(1), float64(0), float64(10)}},
				{Code: 0},
			},
		},
	})
	runner.RunTurnEnd(1)
	assert.Len(t, log.hpChanges, 1)
}

func TestTroopEvent_RunMoment(t *testing.T) {
	runner, log := makeTroopRunner([]resource.TroopPage{
		{
			Span:       2,
			Conditions: resource.TroopPageConditions{},
			List: []resource.EventCommand{
				{Code: 331, Parameters: []interface{}{float64(0), float64(0), float64(0), float64(5)}},
				{Code: 0},
			},
		},
	})
	runner.RunMoment(1)
	assert.Len(t, log.hpChanges, 1)
	// Span=2 pages can re-trigger
	log.hpChanges = nil
	runner.RunMoment(2)
	assert.Len(t, log.hpChanges, 1)
}

func TestTroopEvent_Span0_OnlyOnce(t *testing.T) {
	runner, log := makeTroopRunner([]resource.TroopPage{
		{
			Span:       0,
			Conditions: resource.TroopPageConditions{},
			List: []resource.EventCommand{
				{Code: 331, Parameters: []interface{}{float64(0), float64(0), float64(0), float64(5)}},
				{Code: 0},
			},
		},
	})
	runner.RunTurnStart(1)
	assert.Len(t, log.hpChanges, 1)

	// Should not fire again
	log.hpChanges = nil
	runner.RunTurnStart(2)
	assert.Len(t, log.hpChanges, 0)
}

func TestTroopEvent_Span1_OncePerTurn(t *testing.T) {
	runner, log := makeTroopRunner([]resource.TroopPage{
		{
			Span:       1,
			Conditions: resource.TroopPageConditions{},
			List: []resource.EventCommand{
				{Code: 331, Parameters: []interface{}{float64(0), float64(0), float64(0), float64(5)}},
				{Code: 0},
			},
		},
	})
	runner.RunTurnStart(1)
	assert.Len(t, log.hpChanges, 1)

	// Same turn, won't fire again via RunMoment (span=1 not in moment's allowed spans)
	// But next turn resets
	log.hpChanges = nil
	runner.RunTurnStart(2) // resets span=1, then evaluates
	assert.Len(t, log.hpChanges, 1)
}

func TestTroopEvent_MatchTurn(t *testing.T) {
	runner, _ := makeTroopRunner(nil)

	// turnB=0 → exact match
	assert.True(t, runner.matchTurn(5, 5, 0))
	assert.False(t, runner.matchTurn(4, 5, 0))

	// turnA=2, turnB=3 → 2, 5, 8, 11...
	assert.True(t, runner.matchTurn(2, 2, 3))
	assert.True(t, runner.matchTurn(5, 2, 3))
	assert.False(t, runner.matchTurn(4, 2, 3))
	assert.False(t, runner.matchTurn(1, 2, 3)) // before turnA
}

func TestTroopEvent_ParamHelpers(t *testing.T) {
	runner, _ := makeTroopRunner(nil)

	// paramInt
	assert.Equal(t, 5, runner.paramInt([]interface{}{float64(5)}, 0))
	assert.Equal(t, 3, runner.paramInt([]interface{}{3}, 0))
	assert.Equal(t, 0, runner.paramInt([]interface{}{}, 0))     // out of bounds
	assert.Equal(t, 0, runner.paramInt([]interface{}{"str"}, 0)) // wrong type

	// paramString
	assert.Equal(t, "hello", runner.paramString([]interface{}{"hello"}, 0))
	assert.Equal(t, "", runner.paramString([]interface{}{}, 0)) // out of bounds
	assert.Equal(t, "42", runner.paramString([]interface{}{42}, 0)) // non-string fallback
}

func TestTroopEvent_NilCallbacks(t *testing.T) {
	// Test that nil callbacks don't panic
	runner := NewTroopEventRunner(TroopEventConfig{
		RNG:    rand.New(rand.NewSource(42)),
		Logger: zap.NewNop(),
	})

	// These should not panic with nil callbacks
	cmds := []resource.EventCommand{
		{Code: 331, Parameters: []interface{}{float64(0), float64(1), float64(0), float64(50)}}, // changeEnemyHP nil
		{Code: 332, Parameters: []interface{}{float64(0), float64(0), float64(0), float64(20)}}, // changeEnemyMP nil
		{Code: 342, Parameters: []interface{}{float64(0), float64(0), float64(0), float64(10)}}, // changeEnemyTP nil
		{Code: 336, Parameters: []interface{}{float64(0), float64(1)}},                          // transformEnemy nil
		{Code: 334, Parameters: []interface{}{float64(0)}},                                       // recoverEnemy nil
		{Code: 333, Parameters: []interface{}{float64(0), float64(0), float64(2)}},              // addState nil
		{Code: 313, Parameters: []interface{}{float64(0), float64(0), float64(0), float64(3)}},  // handleChangeState nil
		{Code: 0},
	}
	runner.executeCommands(cmds) // should not panic
}

func TestTroopEvent_Comment(t *testing.T) {
	runner, log := makeTroopRunner(nil)

	cmds := []resource.EventCommand{
		{Code: 108, Parameters: []interface{}{"This is a comment"}},
		{Code: 408, Parameters: []interface{}{"Comment continuation"}},
		{Code: 331, Parameters: []interface{}{float64(0), float64(0), float64(0), float64(5)}},
		{Code: 0},
	}
	runner.executeCommands(cmds)
	assert.Len(t, log.hpChanges, 1)
}

func TestTroopEvent_UnhandledCode(t *testing.T) {
	runner, _ := makeTroopRunner(nil)

	cmds := []resource.EventCommand{
		{Code: 999}, // unknown code
		{Code: 128}, // Change Armors — skipped
		{Code: 0},
	}
	runner.executeCommands(cmds) // should not panic
}

// ═══════════════════════════════════════════════════════════════════════════
//  AI target resolution tests
// ═══════════════════════════════════════════════════════════════════════════

func makeAIBattlers() ([]Battler, []Battler, *EnemyBattler) {
	res := makeExtendedRes()
	actor1 := NewActorBattler(ActorConfig{
		CharID: 1, Name: "Hero", Index: 0, Level: 10,
		HP: 200, MP: 50, BaseParams: [8]int{200, 50, 30, 20, 25, 18, 15, 10},
		Skills: []int{1}, Res: res,
	})
	actor2 := NewActorBattler(ActorConfig{
		CharID: 2, Name: "Mage", Index: 1, Level: 10,
		HP: 150, MP: 80, BaseParams: [8]int{150, 80, 15, 12, 35, 25, 10, 8},
		Skills: []int{1}, Res: res,
	})
	enemy1 := NewEnemyBattler(res.Enemies[1], 0, res)
	enemy2 := NewEnemyBattler(res.Enemies[2], 1, res)

	actors := []Battler{actor1, actor2}
	enemies := []Battler{enemy1, enemy2}
	return actors, enemies, enemy1
}

func TestResolveAITarget_AllScopes(t *testing.T) {
	actors, enemies, enemy := makeAIBattlers()
	rng := rand.New(rand.NewSource(42))

	// scope 0: none
	indices, isActor := resolveAITarget(0, enemy, actors, enemies, rng)
	assert.Nil(t, indices)
	assert.False(t, isActor)

	// scope 1: 1 random actor
	indices, isActor = resolveAITarget(1, enemy, actors, enemies, rng)
	assert.Len(t, indices, 1)
	assert.True(t, isActor)

	// scope 2: all actors
	indices, isActor = resolveAITarget(2, enemy, actors, enemies, rng)
	assert.Len(t, indices, 2)
	assert.True(t, isActor)

	// scope 3: 1 random actor (same as 1)
	indices, isActor = resolveAITarget(3, enemy, actors, enemies, rng)
	assert.Len(t, indices, 1)
	assert.True(t, isActor)

	// scope 4: 2 random actors
	indices, isActor = resolveAITarget(4, enemy, actors, enemies, rng)
	assert.Len(t, indices, 2)
	assert.True(t, isActor)

	// scope 5: 3 random actors
	indices, isActor = resolveAITarget(5, enemy, actors, enemies, rng)
	assert.Len(t, indices, 3)
	assert.True(t, isActor)

	// scope 6: 4 random actors
	indices, isActor = resolveAITarget(6, enemy, actors, enemies, rng)
	assert.Len(t, indices, 4)
	assert.True(t, isActor)

	// scope 7: 1 ally (enemy side)
	indices, isActor = resolveAITarget(7, enemy, actors, enemies, rng)
	assert.Len(t, indices, 1)
	assert.False(t, isActor)

	// scope 8: all allies
	indices, isActor = resolveAITarget(8, enemy, actors, enemies, rng)
	assert.Len(t, indices, 2)
	assert.False(t, isActor)

	// scope 11: user
	indices, isActor = resolveAITarget(11, enemy, actors, enemies, rng)
	assert.Equal(t, []int{0}, indices)
	assert.False(t, isActor)

	// scope 99: default → random actor
	indices, isActor = resolveAITarget(99, enemy, actors, enemies, rng)
	assert.Len(t, indices, 1)
	assert.True(t, isActor)
}

func TestResolveAITarget_DeadTargets(t *testing.T) {
	actors, enemies, enemy := makeAIBattlers()
	rng := rand.New(rand.NewSource(42))

	// Kill one enemy for dead target tests
	enemies[1].SetHP(0)
	enemies[1].AddState(1, -1)

	// scope 9: 1 dead ally
	indices, isActor := resolveAITarget(9, enemy, actors, enemies, rng)
	assert.Len(t, indices, 1)
	assert.Equal(t, 1, indices[0])
	assert.False(t, isActor)

	// scope 10: all dead allies
	indices, isActor = resolveAITarget(10, enemy, actors, enemies, rng)
	assert.Len(t, indices, 1)
	assert.Equal(t, 1, indices[0])
	assert.False(t, isActor)
}

func TestPickNRandomAliveIndices(t *testing.T) {
	actors, _, _ := makeAIBattlers()
	rng := rand.New(rand.NewSource(42))

	indices := pickNRandomAliveIndices(actors, 3, rng)
	assert.Len(t, indices, 3)

	// All dead → nil
	for _, a := range actors {
		a.SetHP(0)
	}
	indices = pickNRandomAliveIndices(actors, 2, rng)
	assert.Nil(t, indices)
}

func TestAllAliveIndices(t *testing.T) {
	actors, _, _ := makeAIBattlers()

	indices := allAliveIndices(actors)
	assert.Equal(t, []int{0, 1}, indices)

	actors[0].SetHP(0)
	indices = allAliveIndices(actors)
	assert.Equal(t, []int{1}, indices)
}

func TestPickRandomDeadIndex(t *testing.T) {
	actors, _, _ := makeAIBattlers()
	rng := rand.New(rand.NewSource(42))

	// No dead → nil
	indices := pickRandomDeadIndex(actors, rng)
	assert.Nil(t, indices)

	// Kill one
	actors[0].SetHP(0)
	actors[0].AddState(1, -1)
	indices = pickRandomDeadIndex(actors, rng)
	assert.Equal(t, []int{0}, indices)
}

func TestAllDeadIndices(t *testing.T) {
	actors, _, _ := makeAIBattlers()

	// No dead
	indices := allDeadIndices(actors)
	assert.Nil(t, indices)

	// Kill both
	actors[0].SetHP(0)
	actors[0].AddState(1, -1)
	actors[1].SetHP(0)
	actors[1].AddState(1, -1)
	indices = allDeadIndices(actors)
	assert.Equal(t, []int{0, 1}, indices)
}

func TestCheckCondition_AllTypes(t *testing.T) {
	res := makeExtendedRes()
	enemy := NewEnemyBattler(res.Enemies[1], 0, res)
	enemy.SetHP(50) // 50/100 = 50% HP
	enemy.SetMP(10) // 10/20 = 50% MP

	// type 0: Always
	assert.True(t, checkCondition(resource.EnemyAction{ConditionType: 0}, 0, enemy))

	// type 1: Turn (exact match)
	assert.True(t, checkCondition(resource.EnemyAction{ConditionType: 1, ConditionParam1: 3, ConditionParam2: 0}, 3, enemy))
	assert.False(t, checkCondition(resource.EnemyAction{ConditionType: 1, ConditionParam1: 3, ConditionParam2: 0}, 4, enemy))

	// type 1: Turn (periodic)
	assert.True(t, checkCondition(resource.EnemyAction{ConditionType: 1, ConditionParam1: 1, ConditionParam2: 3}, 4, enemy))  // 1+3*1=4
	assert.False(t, checkCondition(resource.EnemyAction{ConditionType: 1, ConditionParam1: 1, ConditionParam2: 3}, 3, enemy)) // 1+3*n != 3

	// type 2: HP% (50% in range 0.3~0.7)
	assert.True(t, checkCondition(resource.EnemyAction{ConditionType: 2, ConditionParam1: 0.3, ConditionParam2: 0.7}, 0, enemy))
	// HP% not in range
	assert.False(t, checkCondition(resource.EnemyAction{ConditionType: 2, ConditionParam1: 0.6, ConditionParam2: 0.9}, 0, enemy))

	// type 3: MP% (50% in range 0.3~0.7)
	assert.True(t, checkCondition(resource.EnemyAction{ConditionType: 3, ConditionParam1: 0.3, ConditionParam2: 0.7}, 0, enemy))

	// type 4: State — enemy has no states
	assert.False(t, checkCondition(resource.EnemyAction{ConditionType: 4, ConditionParam1: 2}, 0, enemy))

	// type 5: Party Level → always true
	assert.True(t, checkCondition(resource.EnemyAction{ConditionType: 5}, 0, enemy))

	// type 6: Switch → always true
	assert.True(t, checkCondition(resource.EnemyAction{ConditionType: 6}, 0, enemy))
}

func TestMakeEnemyAction_NoActionsEntry(t *testing.T) {
	res := makeExtendedRes()
	enemy := NewEnemyBattler(&resource.Enemy{ID: 99, Name: "NoActions", HP: 50, Atk: 10, Def: 5}, 0, res)
	actors, _, _ := makeAIBattlers()
	rng := rand.New(rand.NewSource(42))

	action := MakeEnemyAction(enemy, 0, actors, nil, res, rng)
	require.NotNil(t, action)
	assert.Equal(t, ActionAttack, action.Type)
}

func TestMakeEnemyAction_WithSkillScope(t *testing.T) {
	res := makeExtendedRes()
	enemy := NewEnemyBattler(&resource.Enemy{
		ID: 10, Name: "Caster", HP: 100, MP: 50, Atk: 10, Def: 5,
		Actions: []resource.EnemyAction{
			{SkillID: 2, ConditionType: 0, Rating: 5}, // Fire (scope=1)
		},
	}, 0, res)
	actors, enemies, _ := makeAIBattlers()
	rng := rand.New(rand.NewSource(42))

	action := MakeEnemyAction(enemy, 0, actors, enemies, res, rng)
	require.NotNil(t, action)
	assert.Equal(t, ActionSkill, action.Type)
	assert.Equal(t, 2, action.SkillID)
	assert.True(t, action.TargetIsActor) // scope 1 targets actors
}

// ═══════════════════════════════════════════════════════════════════════════
//  Instance helpers tests
// ═══════════════════════════════════════════════════════════════════════════

func makeBattleInstance() *BattleInstance {
	res := makeExtendedRes()
	actor := makeExtActor(res)
	enemy := makeExtEnemy(res)

	return &BattleInstance{
		Actors:  []Battler{actor},
		Enemies: []Battler{enemy},
		res:     res,
		rng:     rand.New(rand.NewSource(42)),
		logger:  zap.NewNop(),
		events:  make(chan BattleEvent, 100),
	}
}

func TestMakeRestrictedAction_Restriction1(t *testing.T) {
	bi := makeBattleInstance()

	// restriction=1: actor attacks enemies
	action := bi.makeRestrictedAction(bi.Actors[0], 1)
	require.NotNil(t, action)
	assert.Equal(t, ActionAttack, action.Type)
	assert.False(t, action.TargetIsActor)
}

func TestMakeRestrictedAction_Restriction2(t *testing.T) {
	bi := makeBattleInstance()

	// restriction=2: attack anyone
	action := bi.makeRestrictedAction(bi.Actors[0], 2)
	require.NotNil(t, action)
	assert.Equal(t, ActionAttack, action.Type)
}

func TestMakeRestrictedAction_Restriction3(t *testing.T) {
	bi := makeBattleInstance()

	// restriction=3: actor attacks allies (actors)
	action := bi.makeRestrictedAction(bi.Actors[0], 3)
	require.NotNil(t, action)
	assert.Equal(t, ActionAttack, action.Type)
	assert.True(t, action.TargetIsActor)
}

func TestMakeRestrictedAction_EnemyPerspective(t *testing.T) {
	bi := makeBattleInstance()

	// restriction=1 for enemy: attacks actors
	action := bi.makeRestrictedAction(bi.Enemies[0], 1)
	require.NotNil(t, action)
	assert.True(t, action.TargetIsActor)

	// restriction=3 for enemy: attacks allies (enemies)
	action = bi.makeRestrictedAction(bi.Enemies[0], 3)
	require.NotNil(t, action)
	assert.False(t, action.TargetIsActor)
}

func TestMakeRestrictedAction_EmptyPool(t *testing.T) {
	bi := makeBattleInstance()
	// Kill all enemies
	bi.Enemies[0].SetHP(0)
	bi.Enemies[0].AddState(1, -1)

	// restriction=1: actor targets enemies, but all dead
	action := bi.makeRestrictedAction(bi.Actors[0], 1)
	assert.Nil(t, action)
}

func TestAliveActorsAndEnemies(t *testing.T) {
	bi := makeBattleInstance()

	assert.Len(t, bi.aliveActors(), 1)
	assert.Len(t, bi.aliveEnemies(), 1)

	bi.Actors[0].SetHP(0)
	assert.Len(t, bi.aliveActors(), 0)

	bi.Enemies[0].SetHP(0)
	assert.Len(t, bi.aliveEnemies(), 0)
}

func TestProcessTurnEnd_Regen(t *testing.T) {
	bi := makeBattleInstance()

	// Add XParam traits for regen
	actor := bi.Actors[0].(*ActorBattler)
	actor.baseTraits = append(actor.baseTraits, resource.Trait{Code: 22, DataID: 7, Value: 0.1})  // HRG 10%
	actor.baseTraits = append(actor.baseTraits, resource.Trait{Code: 22, DataID: 8, Value: 0.05}) // MRG 5%
	actor.baseTraits = append(actor.baseTraits, resource.Trait{Code: 22, DataID: 9, Value: 0.2})  // TRG 20%

	// Set HP/MP below max
	actor.SetHP(100)
	actor.SetMP(30)
	actor.SetTP(10)

	bi.processTurnEnd()

	// HP should have increased by ~10% of maxHP (250*0.1 = 25)
	assert.Greater(t, actor.HP(), 100)
	assert.Greater(t, actor.MP(), 30)
	assert.Greater(t, actor.TP(), 10)

	// Check event was emitted
	select {
	case evt := <-bi.events:
		assert.Equal(t, "turn_end", evt.EventType())
	default:
		t.Fatal("expected turn_end event")
	}
}

func TestProcessTurnEnd_SkipDead(t *testing.T) {
	bi := makeBattleInstance()

	// Kill the actor
	bi.Actors[0].SetHP(0)
	bi.Actors[0].AddState(1, -1)

	bi.processTurnEnd()

	// Actor HP should still be 0
	assert.Equal(t, 0, bi.Actors[0].HP())
}

func TestProcessTurnEnd_StateTick(t *testing.T) {
	bi := makeBattleInstance()

	// Add a state with turns
	bi.Actors[0].AddState(2, 1) // poison, 1 turn remaining

	bi.processTurnEnd()

	// State should have expired (ticked down to 0)
	assert.False(t, bi.Actors[0].HasState(2))
}

func TestProcessTurnEnd_BuffTick(t *testing.T) {
	bi := makeBattleInstance()

	// Add a buff with 1 turn remaining
	bi.Actors[0].AddBuff(2, 1) // ATK buff, 1 turn

	bi.processTurnEnd()

	// Buff should have expired
	assert.Equal(t, 0, bi.Actors[0].BuffLevel(2))
}

func TestLookupActionSpeed(t *testing.T) {
	bi := makeBattleInstance()

	// Attack → skill 1 speed
	input := &ActionInput{ActionType: ActionAttack}
	speed := bi.lookupActionSpeed(input)
	assert.Equal(t, 0, speed) // skill 1 speed = 0

	// Skill
	input = &ActionInput{ActionType: ActionSkill, SkillID: 2}
	speed = bi.lookupActionSpeed(input)
	assert.Equal(t, 0, speed)

	// Item
	input = &ActionInput{ActionType: ActionItem, ItemID: 1}
	speed = bi.lookupActionSpeed(input)
	assert.Equal(t, 0, speed)

	// Guard → not a skill lookup
	input = &ActionInput{ActionType: ActionGuard}
	speed = bi.lookupActionSpeed(input)
	assert.Equal(t, 0, speed)

	// Nil res
	bi.res = nil
	speed = bi.lookupActionSpeed(&ActionInput{ActionType: ActionAttack})
	assert.Equal(t, 0, speed)
}

func TestCalcStatsFromClass(t *testing.T) {
	res := makeExtendedRes()
	// Add a class with params
	res.Classes = []*resource.Class{
		nil,
		{
			ID:   1,
			Name: "Warrior",
			Params: [][]int{
				{0, 100, 200, 300}, // MHP at levels 0,1,2,3
				{0, 20, 40, 60},    // MMP
				{0, 10, 20, 30},    // ATK
				{0, 8, 16, 24},     // DEF
				{0, 5, 10, 15},     // MAT
				{0, 5, 10, 15},     // MDF
				{0, 7, 14, 21},     // AGI
				{0, 3, 6, 9},       // LUK
			},
		},
	}

	stats := calcStatsFromClass(res, 1, 2)
	require.NotNil(t, stats)
	assert.Equal(t, 200, stats[0]) // MHP at level 2
	assert.Equal(t, 40, stats[1])  // MMP at level 2
	assert.Equal(t, 20, stats[2])  // ATK at level 2

	// Negative level → clamp to 0
	stats = calcStatsFromClass(res, 1, -1)
	require.NotNil(t, stats)
	assert.Equal(t, 0, stats[0])

	// Invalid class
	stats = calcStatsFromClass(res, 99, 1)
	assert.Nil(t, stats)
}

func TestBoolToStr(t *testing.T) {
	assert.Equal(t, "actor", boolToStr(true))
	assert.Equal(t, "enemy", boolToStr(false))
}

func TestEmitBattleEnd_Win(t *testing.T) {
	bi := makeBattleInstance()
	bi.Enemies[0].SetHP(0)

	bi.emitBattleEnd(ResultWin)

	select {
	case evt := <-bi.events:
		endEvt, ok := evt.(*EventBattleEnd)
		require.True(t, ok)
		assert.Equal(t, ResultWin, endEvt.Result)
		assert.Greater(t, endEvt.Exp, 0)
		assert.Greater(t, endEvt.Gold, 0)
	default:
		t.Fatal("expected battle_end event")
	}
}

func TestEmitBattleEnd_Lose(t *testing.T) {
	bi := makeBattleInstance()

	bi.emitBattleEnd(ResultLose)

	select {
	case evt := <-bi.events:
		endEvt, ok := evt.(*EventBattleEnd)
		require.True(t, ok)
		assert.Equal(t, ResultLose, endEvt.Result)
		assert.Equal(t, 0, endEvt.Exp)
	default:
		t.Fatal("expected battle_end event")
	}
}

func TestEmitBattleEnd_WithLevelUp(t *testing.T) {
	bi := makeBattleInstance()
	bi.levelCheckFn = func(charID int64, expGain int) (int, bool) {
		return 11, true
	}
	bi.Enemies[0].SetHP(0)

	bi.emitBattleEnd(ResultWin)

	select {
	case evt := <-bi.events:
		endEvt, ok := evt.(*EventBattleEnd)
		require.True(t, ok)
		require.Len(t, endEvt.LevelUps, 1)
		assert.Equal(t, 11, endEvt.LevelUps[0].NewLevel)
	default:
		t.Fatal("expected battle_end event")
	}
}

func TestEmitEvent_ChannelFull(t *testing.T) {
	bi := makeBattleInstance()
	bi.events = make(chan BattleEvent, 1)

	// Fill the channel
	bi.emitEvent(&EventBattleEnd{})
	// This should not block (drops the event)
	bi.emitEvent(&EventBattleEnd{})
}

// ═══════════════════════════════════════════════════════════════════════════
//  Action: applyItemToTarget tests
// ═══════════════════════════════════════════════════════════════════════════

func TestApplyItemToTarget_HPDamage(t *testing.T) {
	res := makeExtendedRes()
	ap := &ActionProcessor{Res: res, RNG: rand.New(rand.NewSource(42))}
	actor := makeExtActor(res)
	enemy := makeExtEnemy(res)

	item := &resource.Item{
		ID: 10, Name: "Bomb", Scope: 1, HitType: 0, SuccessRate: 100,
		Damage: resource.SkillDamage{Formula: "100", Type: 1, Variance: 0},
	}

	out := ap.applyItemToTarget(actor, enemy, item)
	assert.Equal(t, 100, out.Damage)
	assert.False(t, out.Missed)
}

func TestApplyItemToTarget_HPRecovery(t *testing.T) {
	res := makeExtendedRes()
	ap := &ActionProcessor{Res: res, RNG: rand.New(rand.NewSource(42))}
	actor := makeExtActor(res)
	actor.SetHP(100)

	item := &resource.Item{
		ID: 10, Name: "HealItem", Scope: 7, HitType: 0, SuccessRate: 100,
		Damage: resource.SkillDamage{Formula: "50", Type: 3, Variance: 0},
	}

	out := ap.applyItemToTarget(actor, actor, item)
	assert.Equal(t, -50, out.Damage)
	assert.Equal(t, 150, actor.HP())
}

func TestApplyItemToTarget_MPDamageAndRecovery(t *testing.T) {
	res := makeExtendedRes()
	ap := &ActionProcessor{Res: res, RNG: rand.New(rand.NewSource(42))}
	actor := makeExtActor(res)
	enemy := makeExtEnemy(res)

	// MP damage
	item := &resource.Item{
		ID: 10, Scope: 1, HitType: 0, SuccessRate: 100,
		Damage: resource.SkillDamage{Formula: "10", Type: 2, Variance: 0},
	}
	out := ap.applyItemToTarget(actor, enemy, item)
	assert.Equal(t, 10, out.Damage)

	// MP recovery
	actor.SetMP(20)
	item = &resource.Item{
		ID: 11, Scope: 7, HitType: 0, SuccessRate: 100,
		Damage: resource.SkillDamage{Formula: "15", Type: 4, Variance: 0},
	}
	out = ap.applyItemToTarget(actor, actor, item)
	assert.Equal(t, -15, out.Damage)
	assert.Equal(t, 35, actor.MP())
}

func TestApplyItemToTarget_Drain(t *testing.T) {
	res := makeExtendedRes()
	ap := &ActionProcessor{Res: res, RNG: rand.New(rand.NewSource(42))}
	actor := makeExtActor(res)
	actor.SetHP(100)
	enemy := makeExtEnemy(res)

	// HP drain
	item := &resource.Item{
		ID: 10, Scope: 1, HitType: 0, SuccessRate: 100,
		Damage: resource.SkillDamage{Formula: "30", Type: 5, Variance: 0},
	}
	out := ap.applyItemToTarget(actor, enemy, item)
	assert.Equal(t, 30, out.Damage)
	assert.Equal(t, 30, out.Drain)
	assert.Equal(t, 130, actor.HP()) // healed

	// MP drain
	actor.SetMP(20)
	item = &resource.Item{
		ID: 11, Scope: 1, HitType: 0, SuccessRate: 100,
		Damage: resource.SkillDamage{Formula: "5", Type: 6, Variance: 0},
	}
	out = ap.applyItemToTarget(actor, enemy, item)
	assert.Equal(t, 5, out.Damage)
	assert.Equal(t, 5, out.Drain)
	assert.Equal(t, 25, actor.MP())
}

func TestApplyItemToTarget_NoDamageType(t *testing.T) {
	res := makeExtendedRes()
	ap := &ActionProcessor{Res: res, RNG: rand.New(rand.NewSource(42))}
	actor := makeExtActor(res)

	// Damage type 0 = no damage formula
	item := &resource.Item{
		ID: 10, Scope: 7, HitType: 0, SuccessRate: 100,
		Damage: resource.SkillDamage{Type: 0},
	}
	out := ap.applyItemToTarget(actor, actor, item)
	assert.Equal(t, 0, out.Damage)
}

func TestApplyItemToTarget_GuardReducesDamage(t *testing.T) {
	res := makeExtendedRes()
	ap := &ActionProcessor{Res: res, RNG: rand.New(rand.NewSource(42))}
	actor := makeExtActor(res)
	enemy := makeExtEnemy(res)
	enemy.SetGuarding(true)

	item := &resource.Item{
		ID: 10, Scope: 1, HitType: 0, SuccessRate: 100,
		Damage: resource.SkillDamage{Formula: "100", Type: 1, Variance: 0},
	}
	out := ap.applyItemToTarget(actor, enemy, item)
	assert.Less(t, out.Damage, 100) // guard reduces damage
}

// ═══════════════════════════════════════════════════════════════════════════
//  Action: randomDead tests
// ═══════════════════════════════════════════════════════════════════════════

func TestRandomDead_NoDead(t *testing.T) {
	res := makeExtendedRes()
	ap := &ActionProcessor{Res: res, RNG: rand.New(rand.NewSource(42))}
	actors, _, _ := makeAIBattlers()

	dead := ap.randomDead(actors, 1)
	assert.Nil(t, dead)
}

func TestRandomDead_SelectsFromDead(t *testing.T) {
	res := makeExtendedRes()
	ap := &ActionProcessor{Res: res, RNG: rand.New(rand.NewSource(42))}
	actors, _, _ := makeAIBattlers()

	actors[0].SetHP(0)
	actors[0].AddState(1, -1)

	dead := ap.randomDead(actors, 1)
	require.Len(t, dead, 1)
	assert.Equal(t, 0, dead[0].Index())

	// n >= dead count → return all
	dead = ap.randomDead(actors, 5)
	require.Len(t, dead, 1) // only 1 dead
}

// ═══════════════════════════════════════════════════════════════════════════
//  Formula: custom functions
// ═══════════════════════════════════════════════════════════════════════════

func TestFormula_CustomFunctions(t *testing.T) {
	stats := &CharacterStats{Atk: 50, Def: 20, Mat: 40, Mdf: 15}
	zero := &CharacterStats{}

	// damagecul_normal(atk, def, level, enhance)
	v, err := EvalFormula("damagecul_normal(a.atk, b.def, 10, 0)", stats, zero)
	require.NoError(t, err)
	// (50 - 0/2) * 2 * 10 = 1000
	assert.Equal(t, 1000.0, v)

	// damagecul_magic
	v, err = EvalFormula("damagecul_magic(a.mat, b.mdf, 5, 0)", stats, zero)
	require.NoError(t, err)
	assert.Equal(t, 400.0, v) // (40 - 0/2) * 2 * 5

	// damagecul_enemy_normal
	v, err = EvalFormula("damagecul_enemy_normal(a.atk, b.def, 3)", stats, &CharacterStats{Def: 10})
	require.NoError(t, err)
	assert.Equal(t, float64((50-10/2)*2*3), v)

	// damagecul_penetration
	v, err = EvalFormula("damagecul_penetration(a.atk, b.def, 2)", stats, &CharacterStats{Def: 40})
	require.NoError(t, err)
	assert.Equal(t, float64((50-40/4)*2*2), v)

	// damagecul_mat01
	v, err = EvalFormula("damagecul_mat01(a.mat, b.mdf, 4)", stats, &CharacterStats{Mdf: 20})
	require.NoError(t, err)
	assert.Equal(t, float64((40-20/2)*2*4), v)

	// Unknown function
	_, err = EvalFormula("unknown_func(1, 2)", stats, zero)
	assert.Error(t, err)
}

func TestFormula_CustomFunctionTooFewArgs(t *testing.T) {
	stats := &CharacterStats{}
	_, err := EvalFormula("damagecul_normal(10, 5)", stats, stats)
	assert.Error(t, err)

	_, err = EvalFormula("damagecul_magic(10)", stats, stats)
	assert.Error(t, err)

	_, err = EvalFormula("damagecul_enemy_normal(10)", stats, stats)
	assert.Error(t, err)

	_, err = EvalFormula("damagecul_penetration(10, 5)", stats, stats)
	assert.Error(t, err)

	_, err = EvalFormula("damagecul_mat01(10)", stats, stats)
	assert.Error(t, err)
}

func TestFormula_CustomFunctionNegativeResult(t *testing.T) {
	stats := &CharacterStats{Atk: 1}
	// (1 - 100/2) * 2 * 1 = negative → returns 0 (randMinDamage)
	v, err := EvalFormula("damagecul_normal(a.atk, 100, 1, 0)", stats, &CharacterStats{})
	require.NoError(t, err)
	assert.Equal(t, 0.0, v)
}

func TestFormula_MathMax(t *testing.T) {
	v, err := EvalFormula("Math.max(10, 20, 5)", &CharacterStats{}, &CharacterStats{})
	require.NoError(t, err)
	assert.Equal(t, 20.0, v)
}

func TestFormula_MathMin(t *testing.T) {
	v, err := EvalFormula("Math.min(10, 20, 5)", &CharacterStats{}, &CharacterStats{})
	require.NoError(t, err)
	assert.Equal(t, 5.0, v)
}

// ═══════════════════════════════════════════════════════════════════════════
//  Events: EventType methods
// ═══════════════════════════════════════════════════════════════════════════

func TestEventTypeStrings(t *testing.T) {
	assert.Equal(t, "battle_end", EventBattleEnd{}.EventType())
	assert.Equal(t, "actor_escape", EventActorEscape{}.EventType())
	assert.Equal(t, "enemy_escape", EventEnemyEscape{}.EventType())
	assert.Equal(t, "troop_command", EventTroopCommand{}.EventType())
}

// ═══════════════════════════════════════════════════════════════════════════
//  Battler: StateRate, AddBuff/AddDebuff edge cases
// ═══════════════════════════════════════════════════════════════════════════

func TestStateRate_WithTraits(t *testing.T) {
	res := makeExtendedRes()
	actor := NewActorBattler(ActorConfig{
		CharID: 1, Name: "Hero", Index: 0, Level: 10,
		HP: 200, MP: 50, BaseParams: [8]int{200, 50, 30, 20, 25, 18, 15, 10},
		Skills:      []int{1},
		ActorTraits: []resource.Trait{{Code: 13, DataID: 2, Value: 0.5}}, // 50% state rate for state 2
		Res:         res,
	})

	rate := actor.StateRate(2)
	assert.Equal(t, 0.5, rate)

	// State with no trait → default 1.0
	rate = actor.StateRate(99)
	assert.Equal(t, 1.0, rate)
}

func TestAddBuff_MaxLevel(t *testing.T) {
	res := makeExtendedRes()
	actor := makeExtActor(res)

	// Add buff multiple times to test capping
	for i := 0; i < 5; i++ {
		actor.AddBuff(2, 5)
	}
	assert.LessOrEqual(t, actor.BuffLevel(2), 2) // capped at 2
}

func TestAddDebuff_MaxLevel(t *testing.T) {
	res := makeExtendedRes()
	actor := makeExtActor(res)

	for i := 0; i < 5; i++ {
		actor.AddDebuff(2, 5)
	}
	assert.GreaterOrEqual(t, actor.BuffLevel(2), -2) // capped at -2
}

// ═══════════════════════════════════════════════════════════════════════════
//  ValidateInput: item validation
// ═══════════════════════════════════════════════════════════════════════════

func TestValidateInput_ItemValid(t *testing.T) {
	bi := makeBattleInstance()

	input := &ActionInput{ActionType: ActionItem, ItemID: 1, ActorIndex: 0}
	result := bi.validateInput(bi.Actors[0], input)
	assert.Equal(t, ActionItem, result.ActionType)
}

func TestValidateInput_ItemInvalid(t *testing.T) {
	bi := makeBattleInstance()

	// Invalid item ID
	input := &ActionInput{ActionType: ActionItem, ItemID: 99, ActorIndex: 0}
	result := bi.validateInput(bi.Actors[0], input)
	assert.Equal(t, ActionAttack, result.ActionType)
}

func TestValidateInput_ItemCheckCallback(t *testing.T) {
	bi := makeBattleInstance()
	bi.itemCheckFn = func(charID int64, itemID int) bool {
		return false // always reject
	}

	input := &ActionInput{ActionType: ActionItem, ItemID: 1, ActorIndex: 0}
	result := bi.validateInput(bi.Actors[0], input)
	assert.Equal(t, ActionAttack, result.ActionType)
}

func TestValidateInput_EscapeType(t *testing.T) {
	bi := makeBattleInstance()

	input := &ActionInput{ActionType: ActionEscape, ActorIndex: 0}
	result := bi.validateInput(bi.Actors[0], input)
	assert.Equal(t, ActionEscape, result.ActionType) // escape is valid
}

// ═══════════════════════════════════════════════════════════════════════════
//  TryEscape tests
// ═══════════════════════════════════════════════════════════════════════════

func TestTryEscape(t *testing.T) {
	bi := makeBattleInstance()

	// With extreme agi advantage, escape should eventually succeed
	actor := bi.Actors[0].(*ActorBattler)
	actor.baseParams[6] = 999 // extremely high AGI

	escaped := false
	for i := 0; i < 100; i++ {
		if bi.tryEscape() {
			escaped = true
			break
		}
	}
	assert.True(t, escaped)
}

func TestTryEscape_IncreasesRatio(t *testing.T) {
	bi := makeBattleInstance()
	initialRatio := bi.escapeRatio

	// With very low agi, first attempt likely fails
	bi.rng = rand.New(rand.NewSource(0)) // deterministic
	bi.tryEscape()
	assert.Greater(t, bi.escapeRatio, initialRatio)
}

func TestAverageAgi_NoneAlive(t *testing.T) {
	bi := makeBattleInstance()
	bi.Actors[0].SetHP(0)

	agi := bi.averageAgi(bi.Actors)
	assert.Equal(t, 1, agi) // fallback when none alive
}

// ═══════════════════════════════════════════════════════════════════════════
//  InitTroopEvents
// ═══════════════════════════════════════════════════════════════════════════

func TestInitTroopEvents_ValidTroop(t *testing.T) {
	bi := makeBattleInstance()
	bi.troopID = 1
	bi.res.Troops = make([]*resource.Troop, 5)
	bi.res.Troops[1] = &resource.Troop{
		ID:   1,
		Name: "TestTroop",
		Pages: []resource.TroopPage{
			{
				Span:       0,
				Conditions: resource.TroopPageConditions{},
				List: []resource.EventCommand{
					{Code: 121, Parameters: []interface{}{float64(1), float64(1), float64(0)}},
					{Code: 0},
				},
			},
		},
	}

	bi.initTroopEvents()
	assert.NotNil(t, bi.troopEvents)
}

func TestInitTroopEvents_NilRes(t *testing.T) {
	bi := makeBattleInstance()
	bi.res = nil
	bi.troopID = 1

	bi.initTroopEvents()
	assert.Nil(t, bi.troopEvents)
}

func TestInitTroopEvents_InvalidTroopID(t *testing.T) {
	bi := makeBattleInstance()
	bi.troopID = 999

	bi.initTroopEvents()
	assert.Nil(t, bi.troopEvents)
}

func TestInitTroopEvents_EmptyPages(t *testing.T) {
	bi := makeBattleInstance()
	bi.troopID = 1
	bi.res.Troops = make([]*resource.Troop, 5)
	bi.res.Troops[1] = &resource.Troop{ID: 1, Name: "Empty"}

	bi.initTroopEvents()
	assert.Nil(t, bi.troopEvents)
}

// ═══════════════════════════════════════════════════════════════════════════
//  WaitForInput / InputCh / TroopAckCh accessors
// ═══════════════════════════════════════════════════════════════════════════

func TestInputCh(t *testing.T) {
	bi := makeBattleInstance()
	bi.inputCh = make(chan *ActionInput, 1)

	ch := bi.InputCh()
	assert.NotNil(t, ch)
}

func TestTroopAckCh(t *testing.T) {
	bi := makeBattleInstance()
	bi.troopAckCh = make(chan struct{}, 1)

	ch := bi.TroopAckCh()
	assert.NotNil(t, ch)
}

func TestWaitForInput_CorrectActor(t *testing.T) {
	bi := makeBattleInstance()
	bi.inputCh = make(chan *ActionInput, 1)

	go func() {
		bi.inputCh <- &ActionInput{ActorIndex: 0, ActionType: ActionAttack}
	}()

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	input, err := bi.waitForInput(ctx, 0)
	require.NoError(t, err)
	assert.Equal(t, 0, input.ActorIndex)
}

func TestWaitForInput_WrongActorRetries(t *testing.T) {
	bi := makeBattleInstance()
	bi.inputCh = make(chan *ActionInput, 2)

	go func() {
		// Send wrong actor first, then correct one
		bi.inputCh <- &ActionInput{ActorIndex: 1, ActionType: ActionAttack}
		time.Sleep(10 * time.Millisecond)
		bi.inputCh <- &ActionInput{ActorIndex: 0, ActionType: ActionGuard}
	}()

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	input, err := bi.waitForInput(ctx, 0)
	require.NoError(t, err)
	assert.Equal(t, 0, input.ActorIndex)
	assert.Equal(t, ActionGuard, input.ActionType)
}

func TestWaitForInput_ContextCancelled(t *testing.T) {
	bi := makeBattleInstance()
	bi.inputCh = make(chan *ActionInput, 1)

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // immediately cancel

	_, err := bi.waitForInput(ctx, 0)
	assert.Error(t, err)
}

// ═══════════════════════════════════════════════════════════════════════════
//  CheckDeathState
// ═══════════════════════════════════════════════════════════════════════════

func TestCheckDeathState(t *testing.T) {
	bi := makeBattleInstance()

	// Kill actor but don't add death state
	bi.Actors[0].SetHP(0)
	assert.False(t, bi.Actors[0].HasState(1))

	bi.checkDeathState()
	assert.True(t, bi.Actors[0].HasState(1))
	assert.Equal(t, 0, bi.Actors[0].TP()) // TP reset on death

	// Kill enemy
	bi.Enemies[0].SetHP(0)
	bi.checkDeathState()
	assert.True(t, bi.Enemies[0].HasState(1))
}

// ═══════════════════════════════════════════════════════════════════════════
//  Weighted select edge cases
// ═══════════════════════════════════════════════════════════════════════════

func TestWeightedSelect_SingleAction(t *testing.T) {
	rng := rand.New(rand.NewSource(42))
	actions := []resource.EnemyAction{
		{SkillID: 1, Rating: 5},
	}
	selected := weightedSelect(actions, rng)
	assert.Equal(t, 1, selected.SkillID)
}

func TestWeightedSelect_LowRatingFiltered(t *testing.T) {
	rng := rand.New(rand.NewSource(42))
	actions := []resource.EnemyAction{
		{SkillID: 1, Rating: 10},
		{SkillID: 2, Rating: 5}, // rating 5 < threshold (10-2=8) → filtered
	}
	// Should always select skill 1 since skill 2 is filtered
	for i := 0; i < 20; i++ {
		selected := weightedSelect(actions, rng)
		assert.Equal(t, 1, selected.SkillID, "iteration %d", i)
	}
}

func TestFilterValidActions_ZeroRating(t *testing.T) {
	res := makeExtendedRes()
	enemy := NewEnemyBattler(res.Enemies[1], 0, res)
	actions := []resource.EnemyAction{
		{SkillID: 1, Rating: 0, ConditionType: 0}, // rating 0 → filtered
		{SkillID: 2, Rating: 5, ConditionType: 0},
	}
	valid := filterValidActions(actions, 0, enemy)
	require.Len(t, valid, 1)
	assert.Equal(t, 2, valid[0].SkillID)
}

// ═══════════════════════════════════════════════════════════════════════════
//  NewBattleInstance
// ═══════════════════════════════════════════════════════════════════════════

func TestNewBattleInstance(t *testing.T) {
	res := makeExtendedRes()
	actor := makeExtActor(res)
	enemy := makeExtEnemy(res)

	bi := NewBattleInstance(BattleConfig{
		TroopID:   1,
		Res:       res,
		Logger:    zap.NewNop(),
		CanEscape: true,
	})
	bi.Actors = []Battler{actor}
	bi.Enemies = []Battler{enemy}

	require.NotNil(t, bi)
	assert.Equal(t, 1, len(bi.Actors))
	assert.Equal(t, 1, len(bi.Enemies))
	assert.NotNil(t, bi.InputCh())
	assert.NotNil(t, bi.TroopAckCh())
	assert.NotNil(t, bi.Events())
}

// ═══════════════════════════════════════════════════════════════════════════
//  Misc: chargeTpByDamage
// ═══════════════════════════════════════════════════════════════════════════

func TestChargeTpByDamage_EdgeCases(t *testing.T) {
	res := makeExtendedRes()
	actor := makeExtActor(res)
	actor.SetTP(0)

	// Take 50 damage out of 250 max HP = 20% → 50 * 0.2 * 1.0 = 10 TP
	chargeTpByDamage(actor, 50)
	assert.Equal(t, 10, actor.TP())

	// Zero damage → no charge
	actor.SetTP(0)
	chargeTpByDamage(actor, 0)
	assert.Equal(t, 0, actor.TP())

	// Negative damage → no charge
	chargeTpByDamage(actor, -10)
	assert.Equal(t, 0, actor.TP())
}

// ═══════════════════════════════════════════════════════════════════════════
//  Misc: targetByOutcome
// ═══════════════════════════════════════════════════════════════════════════

func TestTargetByOutcome_OutOfRange(t *testing.T) {
	res := makeExtendedRes()
	ap := &ActionProcessor{Res: res, RNG: rand.New(rand.NewSource(42))}
	actors, enemies, _ := makeAIBattlers()

	out := ActionOutcome{TargetIndex: 99, TargetIsActor: true}
	target := ap.targetByOutcome(out, actors, enemies)
	assert.Nil(t, target)
}

func TestTargetByOutcome_EnemySide(t *testing.T) {
	res := makeExtendedRes()
	ap := &ActionProcessor{Res: res, RNG: rand.New(rand.NewSource(42))}
	actors, enemies, _ := makeAIBattlers()

	out := ActionOutcome{TargetIndex: 0, TargetIsActor: false}
	target := ap.targetByOutcome(out, actors, enemies)
	assert.NotNil(t, target)
	assert.Equal(t, 0, target.Index())
}

// ═══════════════════════════════════════════════════════════════════════════
//  Run: integration-style context cancellation
// ═══════════════════════════════════════════════════════════════════════════

func TestRun_ContextCancelled(t *testing.T) {
	res := makeExtendedRes()
	actor := makeExtActor(res)
	enemy := makeExtEnemy(res)

	bi := NewBattleInstance(BattleConfig{
		TroopID:   0,
		Res:       res,
		Logger:    zap.NewNop(),
		CanEscape: false,
	})
	bi.Actors = []Battler{actor}
	bi.Enemies = []Battler{enemy}

	ctx, cancel := context.WithCancel(context.Background())

	done := make(chan int, 1)
	go func() {
		done <- bi.Run(ctx)
	}()

	// Let it start, then cancel
	time.Sleep(50 * time.Millisecond)
	cancel()

	select {
	case <-done:
		// Context cancelled, Run returned
	case <-time.After(5 * time.Second):
		t.Fatal("Run did not return after context cancellation")
	}
}

// ═══════════════════════════════════════════════════════════════════════════
//  Misc formula coverage: applyMathFunc with multiple args
// ═══════════════════════════════════════════════════════════════════════════

func TestFormula_MathMaxMin_MultiArg(t *testing.T) {
	v, err := EvalFormula("Math.max(1, 5, 3, 2)", &CharacterStats{}, &CharacterStats{})
	require.NoError(t, err)
	assert.Equal(t, 5.0, v)

	v, err = EvalFormula("Math.min(10, 2, 7, 1)", &CharacterStats{}, &CharacterStats{})
	require.NoError(t, err)
	assert.Equal(t, 1.0, v)
}

func TestFormula_MathFuncErrors(t *testing.T) {
	// Math.floor with no args
	_, err := EvalFormula("Math.floor()", &CharacterStats{}, &CharacterStats{})
	assert.Error(t, err)
}

func TestFormula_TernaryLikeMax(t *testing.T) {
	// Common RMMV pattern: Math.max(expr, 0) to clamp negative
	v, err := EvalFormula("Math.max(a.atk * 4 - b.def * 2, 0)", &CharacterStats{Atk: 10}, &CharacterStats{Def: 30})
	require.NoError(t, err)
	assert.Equal(t, 0.0, v) // 40 - 60 = -20 → max(-20, 0) = 0
}

func TestFormula_LevelField(t *testing.T) {
	v, err := EvalFormula("a.level * 10", &CharacterStats{Level: 5}, &CharacterStats{})
	require.NoError(t, err)
	assert.Equal(t, 50.0, v)
}

// ═══════════════════════════════════════════════════════════════════════════
//  processSkillAction: TPGain, magic reflection, counter-attack
// ═══════════════════════════════════════════════════════════════════════════

func TestProcessSkillAction_TPGain(t *testing.T) {
	res := makeExtendedRes()
	// Add a skill with TPGain
	res.Skills = append(res.Skills, &resource.Skill{
		ID: 21, Name: "TPSkill", Scope: 1, HitType: 1, SuccessRate: 100, Repeats: 1,
		TPGain: 20,
		Damage: resource.SkillDamage{Formula: "a.atk * 2", Type: 1, Variance: 0},
	})
	ap := &ActionProcessor{Res: res, RNG: rand.New(rand.NewSource(42))}
	actor := makeExtActor(res)
	actor.SetTP(0)
	enemy := makeExtEnemy(res)

	action := &Action{
		Type:          ActionSkill,
		SkillID:       21,
		TargetIndices: []int{0},
		TargetIsActor: false,
	}
	outcomes := ap.processSkillAction(actor, 21, action, []Battler{actor}, []Battler{enemy})
	require.NotEmpty(t, outcomes)
	assert.Equal(t, 20, actor.TP()) // TP gained
}

func TestProcessSkillAction_MagicReflection(t *testing.T) {
	res := makeExtendedRes()
	ap := &ActionProcessor{Res: res, RNG: rand.New(rand.NewSource(42))}
	actor := makeExtActor(res)
	enemy := makeExtEnemy(res)
	// Give enemy magic reflection rate (xparam 5 = mrf)
	enemy.baseTraits = append(enemy.baseTraits, resource.Trait{Code: 22, DataID: 5, Value: 1.0}) // 100% reflect

	action := &Action{
		Type:          ActionSkill,
		SkillID:       2, // Fire (hitType=2, magical)
		TargetIndices: []int{0},
		TargetIsActor: false,
	}

	initialActorHP := actor.HP()
	ap.processSkillAction(actor, 2, action, []Battler{actor}, []Battler{enemy})
	// Spell should reflect back to actor (actor HP decreased)
	assert.Less(t, actor.HP(), initialActorHP)
}

func TestProcessSkillAction_CounterAttack(t *testing.T) {
	res := makeExtendedRes()
	ap := &ActionProcessor{Res: res, RNG: rand.New(rand.NewSource(0))}
	actor := makeExtActor(res)
	enemy := NewEnemyBattler(&resource.Enemy{ID: 1, Name: "TankGoblin", HP: 9999, MP: 20, Atk: 15, Def: 10, Agi: 12, Luk: 5,
		Actions: []resource.EnemyAction{{SkillID: 1, ConditionType: 0, Rating: 5}}}, 0, res)
	// Give enemy counter-attack rate (xparam 6 = cnt)
	enemy.baseTraits = append(enemy.baseTraits, resource.Trait{Code: 22, DataID: 6, Value: 1.0}) // 100% counter

	action := &Action{
		Type:          ActionSkill,
		SkillID:       1, // Attack (hitType=1, physical)
		TargetIndices: []int{0},
		TargetIsActor: false,
	}

	outcomes := ap.processSkillAction(actor, 1, action, []Battler{actor}, []Battler{enemy})
	// Should have at least 2 outcomes: original hit + counter
	hasCounter := false
	for _, out := range outcomes {
		if out.IsCounter {
			hasCounter = true
		}
	}
	assert.True(t, hasCounter, "expected counter-attack outcome")
}

func TestProcessSkillAction_MPDrain(t *testing.T) {
	res := makeExtendedRes()
	ap := &ActionProcessor{Res: res, RNG: rand.New(rand.NewSource(42))}
	actor := makeExtActor(res)
	enemy := makeExtEnemy(res)

	// Add MP drain skill
	res.Skills = append(res.Skills, &resource.Skill{
		ID: 22, Name: "MPDrain", MPCost: 0, Scope: 1, HitType: 2, SuccessRate: 100, Repeats: 1,
		Damage: resource.SkillDamage{Formula: "10", Type: 6, Variance: 0}, // MP drain
	})

	action := &Action{
		Type:          ActionSkill,
		SkillID:       22,
		TargetIndices: []int{0},
		TargetIsActor: false,
	}

	actorMPBefore := actor.MP()
	outcomes := ap.processSkillAction(actor, 22, action, []Battler{actor}, []Battler{enemy})
	require.NotEmpty(t, outcomes)
	assert.Equal(t, 10, outcomes[0].Damage)
	assert.Equal(t, actorMPBefore+10, actor.MP()) // drained MP added
}

func TestProcessSkillAction_MPRecovery(t *testing.T) {
	res := makeExtendedRes()
	ap := &ActionProcessor{Res: res, RNG: rand.New(rand.NewSource(42))}
	actor := makeExtActor(res)

	// Add MP recovery skill
	res.Skills = append(res.Skills, &resource.Skill{
		ID: 23, Name: "MPHeal", MPCost: 0, Scope: 11, HitType: 0, SuccessRate: 100, Repeats: 1,
		Damage: resource.SkillDamage{Formula: "20", Type: 4, Variance: 0}, // MP recovery
	})

	actor.SetMP(10)
	action := &Action{
		Type:          ActionSkill,
		SkillID:       23,
		TargetIndices: []int{0},
		TargetIsActor: true,
	}

	outcomes := ap.processSkillAction(actor, 23, action, []Battler{actor}, nil)
	require.NotEmpty(t, outcomes)
	assert.Equal(t, 30, actor.MP())
}

func TestProcessSkillAction_MPDamage(t *testing.T) {
	res := makeExtendedRes()
	ap := &ActionProcessor{Res: res, RNG: rand.New(rand.NewSource(42))}
	actor := makeExtActor(res)
	enemy := makeExtEnemy(res)

	// Add MP damage skill
	res.Skills = append(res.Skills, &resource.Skill{
		ID: 24, Name: "MPBurn", MPCost: 0, Scope: 1, HitType: 1, SuccessRate: 100, Repeats: 1,
		Damage: resource.SkillDamage{Formula: "8", Type: 2, Variance: 0},
	})

	action := &Action{
		Type:          ActionSkill,
		SkillID:       24,
		TargetIndices: []int{0},
		TargetIsActor: false,
	}

	enemyMPBefore := enemy.MP()
	outcomes := ap.processSkillAction(actor, 24, action, []Battler{actor}, []Battler{enemy})
	require.NotEmpty(t, outcomes)
	assert.Equal(t, enemyMPBefore-8, enemy.MP())
}

func TestApplySkillToTarget_Guard(t *testing.T) {
	res := makeExtendedRes()
	ap := &ActionProcessor{Res: res, RNG: rand.New(rand.NewSource(42))}
	actor := makeExtActor(res)
	enemy := makeExtEnemy(res)
	enemy.SetGuarding(true)

	skill := res.Skills[1] // Attack
	outGuard := ap.applySkillToTarget(actor, enemy, skill)
	enemy.SetGuarding(false)
	enemy.SetHP(enemy.MaxHP())
	outNoGuard := ap.applySkillToTarget(actor, enemy, skill)

	assert.Less(t, outGuard.Damage, outNoGuard.Damage)
}

func TestApplySkillToTarget_NoDamageType(t *testing.T) {
	res := makeExtendedRes()
	ap := &ActionProcessor{Res: res, RNG: rand.New(rand.NewSource(42))}
	actor := makeExtActor(res)

	// Skill with damage type 0 (effect-only)
	skill := &resource.Skill{
		ID: 99, Scope: 11, HitType: 0, SuccessRate: 100, Repeats: 1,
		Damage: resource.SkillDamage{Type: 0},
	}

	out := ap.applySkillToTarget(actor, actor, skill)
	assert.Equal(t, 0, out.Damage)
	assert.False(t, out.Missed)
}

// ═══════════════════════════════════════════════════════════════════════════
//  collectActions tests
// ═══════════════════════════════════════════════════════════════════════════

func TestCollectActions_NormalInput(t *testing.T) {
	bi := makeBattleInstance()
	bi.inputCh = make(chan *ActionInput, 2)

	// Pre-fill input for actor 0
	bi.inputCh <- &ActionInput{ActorIndex: 0, ActionType: ActionAttack, TargetIndices: []int{0}, TargetIsActor: false}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	err := bi.collectActions(ctx)
	require.NoError(t, err)

	// Actor should have an action set
	assert.NotNil(t, bi.Actors[0].CurrentAction())
	// Enemy should also have an action (AI generated)
	assert.NotNil(t, bi.Enemies[0].CurrentAction())
}

func TestCollectActions_Restriction4_CannotMove(t *testing.T) {
	bi := makeBattleInstance()
	bi.inputCh = make(chan *ActionInput, 2)

	// Give actor stun state (restriction=4)
	bi.Actors[0].AddState(3, 5) // Stun has restriction=4

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	err := bi.collectActions(ctx)
	require.NoError(t, err)

	// Actor should have no action (cleared)
	assert.Nil(t, bi.Actors[0].CurrentAction())
}

func TestCollectActions_Restriction1_ForcedAttack(t *testing.T) {
	bi := makeBattleInstance()
	bi.inputCh = make(chan *ActionInput, 2)

	// Give actor confusion state (restriction=1)
	bi.Actors[0].AddState(4, 5) // Confusion has restriction=1

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	err := bi.collectActions(ctx)
	require.NoError(t, err)

	// Actor should have a forced attack
	action := bi.Actors[0].CurrentAction()
	require.NotNil(t, action)
	assert.Equal(t, ActionAttack, action.Type)
}

func TestCollectActions_DeadActorSkipped(t *testing.T) {
	bi := makeBattleInstance()
	bi.inputCh = make(chan *ActionInput, 2)

	// Kill the actor
	bi.Actors[0].SetHP(0)
	bi.Actors[0].AddState(1, -1)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	err := bi.collectActions(ctx)
	require.NoError(t, err)
	// Should not hang waiting for dead actor's input
}

func TestCollectActions_DeadEnemySkipped(t *testing.T) {
	bi := makeBattleInstance()
	bi.inputCh = make(chan *ActionInput, 2)

	// Kill enemy and pre-fill actor input
	bi.Enemies[0].SetHP(0)
	bi.Enemies[0].AddState(1, -1)
	bi.inputCh <- &ActionInput{ActorIndex: 0, ActionType: ActionAttack}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	err := bi.collectActions(ctx)
	require.NoError(t, err)

	// Dead enemy should not get an action
	assert.Nil(t, bi.Enemies[0].CurrentAction())
}

func TestCollectActions_EscapedActorSkipped(t *testing.T) {
	bi := makeBattleInstance()
	bi.inputCh = make(chan *ActionInput, 2)
	bi.MarkEscaped(0) // Actor 0 escaped

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	err := bi.collectActions(ctx)
	require.NoError(t, err)
	// Should not hang waiting for escaped actor's input
}

func TestCollectActions_EnemyRestriction4(t *testing.T) {
	bi := makeBattleInstance()
	bi.inputCh = make(chan *ActionInput, 2)
	bi.inputCh <- &ActionInput{ActorIndex: 0, ActionType: ActionAttack}

	// Give enemy stun (restriction=4)
	bi.Enemies[0].AddState(3, 5)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	err := bi.collectActions(ctx)
	require.NoError(t, err)
	assert.Nil(t, bi.Enemies[0].CurrentAction())
}

// ═══════════════════════════════════════════════════════════════════════════
//  initTroopEvents callback coverage
// ═══════════════════════════════════════════════════════════════════════════

func TestInitTroopEvents_Callbacks(t *testing.T) {
	res := makeExtendedRes()
	res.Troops = make([]*resource.Troop, 5)
	res.Enemies = append(res.Enemies, &resource.Enemy{
		ID: 3, Name: "Slime", HP: 50, MP: 10, Atk: 5, Def: 3,
	})

	// Create troop with pages that exercise various callbacks
	res.Troops[1] = &resource.Troop{
		ID:   1,
		Name: "TestTroop",
		Pages: []resource.TroopPage{
			{
				Span:       2, // moment - re-triggerable
				Conditions: resource.TroopPageConditions{EnemyValid: true, EnemyIndex: 0, EnemyHp: 60},
				List: []resource.EventCommand{
					// Change enemy HP
					{Code: 331, Parameters: []interface{}{float64(0), float64(0), float64(0), float64(10)}},
					// Change enemy MP
					{Code: 332, Parameters: []interface{}{float64(0), float64(0), float64(0), float64(5)}},
					// Change enemy TP
					{Code: 342, Parameters: []interface{}{float64(0), float64(0), float64(0), float64(3)}},
					// Change enemy state (add state 2)
					{Code: 333, Parameters: []interface{}{float64(0), float64(0), float64(2)}},
					// Enemy recover all
					{Code: 334, Parameters: []interface{}{float64(0)}},
					// Enemy transform
					{Code: 336, Parameters: []interface{}{float64(0), float64(2)}},
					// Abort
					{Code: 340},
					{Code: 0},
				},
			},
		},
	}

	actor := makeExtActor(res)
	enemy := NewEnemyBattler(res.Enemies[1], 0, res)
	enemy.SetHP(50) // 50% HP

	bi := NewBattleInstance(BattleConfig{
		TroopID: 1,
		Res:     res,
		Logger:  zap.NewNop(),
	})
	bi.Actors = []Battler{actor}
	bi.Enemies = []Battler{enemy}

	bi.initTroopEvents()
	require.NotNil(t, bi.troopEvents)

	// Run moment to trigger the page (enemy at 50% HP <= 60% threshold)
	bi.troopEvents.RunMoment(0)

	// Verify the abort was called
	assert.True(t, bi.IsAborted())
}

func TestInitTroopEvents_ActorHPCallback(t *testing.T) {
	res := makeExtendedRes()
	res.Troops = make([]*resource.Troop, 5)
	res.Troops[1] = &resource.Troop{
		ID:   1,
		Name: "TestTroop",
		Pages: []resource.TroopPage{
			{
				Span:       0,
				Conditions: resource.TroopPageConditions{ActorValid: true, ActorId: 1, ActorHp: 90},
				List: []resource.EventCommand{
					{Code: 121, Parameters: []interface{}{float64(1), float64(1), float64(0)}},
					{Code: 0},
				},
			},
		},
	}

	actor := NewActorBattler(ActorConfig{
		CharID: 1, Name: "Hero", Index: 0, Level: 10,
		HP: 200, MP: 50, BaseParams: [8]int{250, 80, 30, 20, 25, 18, 15, 10},
		Skills: []int{1}, Res: res,
	})
	enemy := makeExtEnemy(res)

	bi := NewBattleInstance(BattleConfig{TroopID: 1, Res: res, Logger: zap.NewNop()})
	bi.Actors = []Battler{actor}
	bi.Enemies = []Battler{enemy}

	bi.initTroopEvents()
	require.NotNil(t, bi.troopEvents)

	// Actor at 200/250 = 80% HP → <= 90% → page triggers
	bi.troopEvents.RunTurnStart(0)
	assert.True(t, bi.troopEvents.switches[1])
}

func TestInitTroopEvents_AddState_RemoveState(t *testing.T) {
	res := makeExtendedRes()
	res.Troops = make([]*resource.Troop, 5)
	res.Troops[1] = &resource.Troop{
		ID:   1,
		Name: "TestTroop",
		Pages: []resource.TroopPage{
			{
				Span:       0,
				Conditions: resource.TroopPageConditions{},
				List: []resource.EventCommand{
					// Add state 2 to actor (code 313, scope=0, actorID=0 all, op=0 add, stateID=2)
					{Code: 313, Parameters: []interface{}{float64(0), float64(0), float64(0), float64(2)}},
					{Code: 0},
				},
			},
		},
	}

	actor := NewActorBattler(ActorConfig{
		CharID: 1, Name: "Hero", Index: 0, Level: 10,
		HP: 200, MP: 50, BaseParams: [8]int{250, 80, 30, 20, 25, 18, 15, 10},
		Skills: []int{1}, Res: res,
	})
	enemy := makeExtEnemy(res)

	bi := NewBattleInstance(BattleConfig{TroopID: 1, Res: res, Logger: zap.NewNop()})
	bi.Actors = []Battler{actor}
	bi.Enemies = []Battler{enemy}

	bi.initTroopEvents()
	bi.troopEvents.RunTurnStart(0)

	// Actor should have state 2
	assert.True(t, actor.HasState(2))
}

// ═══════════════════════════════════════════════════════════════════════════
//  processItemAction: item effects applied
// ═══════════════════════════════════════════════════════════════════════════

func TestProcessItemAction_WithEffects(t *testing.T) {
	res := makeExtendedRes()
	ap := &ActionProcessor{Res: res, RNG: rand.New(rand.NewSource(42))}
	actor := makeExtActor(res)
	actor.SetHP(100)

	// Use potion (item 1: heal 50 HP)
	action := &Action{
		Type:          ActionItem,
		ItemID:        1,
		TargetIndices: []int{0},
		TargetIsActor: true,
	}

	outcomes := ap.processItemAction(actor, 1, action, []Battler{actor}, nil)
	require.NotEmpty(t, outcomes)
	assert.Equal(t, 150, actor.HP())
}

func TestProcessItemAction_InvalidItem(t *testing.T) {
	res := makeExtendedRes()
	ap := &ActionProcessor{Res: res, RNG: rand.New(rand.NewSource(42))}
	actor := makeExtActor(res)

	outcomes := ap.processItemAction(actor, 99, nil, []Battler{actor}, nil)
	assert.Nil(t, outcomes)
}

// ═══════════════════════════════════════════════════════════════════════════
//  waitForTroopAck
// ═══════════════════════════════════════════════════════════════════════════

func TestWaitForTroopAck(t *testing.T) {
	bi := makeBattleInstance()
	bi.troopAckCh = make(chan struct{}, 1)

	// Pre-fill ack
	bi.troopAckCh <- struct{}{}

	// Should not block
	done := make(chan struct{})
	go func() {
		bi.waitForTroopAck()
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("waitForTroopAck blocked")
	}
}

// ═══════════════════════════════════════════════════════════════════════════
//  Misc: ProcessAction dispatch
// ═══════════════════════════════════════════════════════════════════════════

func TestProcessAction_SkillDispatch(t *testing.T) {
	res := makeExtendedRes()
	ap := &ActionProcessor{Res: res, RNG: rand.New(rand.NewSource(42))}
	actor := makeExtActor(res)
	enemy := makeExtEnemy(res)

	action := &Action{
		Type:          ActionSkill,
		SkillID:       1,
		TargetIndices: []int{0},
		TargetIsActor: false,
	}

	result := ap.ProcessAction(actor, action, []Battler{actor}, []Battler{enemy})
	require.NotEmpty(t, result)
}

func TestProcessAction_ItemDispatch(t *testing.T) {
	res := makeExtendedRes()
	ap := &ActionProcessor{Res: res, RNG: rand.New(rand.NewSource(42))}
	actor := makeExtActor(res)
	actor.SetHP(100)

	action := &Action{
		Type:          ActionItem,
		ItemID:        1,
		TargetIndices: []int{0},
		TargetIsActor: true,
	}

	result := ap.ProcessAction(actor, action, []Battler{actor}, nil)
	require.NotNil(t, result)
	assert.Equal(t, 150, actor.HP())
}

func TestProcessAction_GuardDispatch(t *testing.T) {
	res := makeExtendedRes()
	ap := &ActionProcessor{Res: res, RNG: rand.New(rand.NewSource(42))}
	actor := makeExtActor(res)

	action := &Action{Type: ActionGuard}

	_ = ap.ProcessAction(actor, action, []Battler{actor}, nil)
	assert.True(t, actor.IsGuarding())
}

func init() {
	// Suppress unused import warning
	_ = fmt.Sprintf
}
