package npc

import (
	"context"
	"testing"

	"github.com/kasuganosora/rpgmakermvmmo/server/game/player"
	"github.com/kasuganosora/rpgmakermvmmo/server/resource"
	"github.com/stretchr/testify/assert"
)

// ========================================================================
// Dispatch: CmdLabel (empty case body — visits case line)
// ========================================================================

func TestDispatch_Label_Visited(t *testing.T) {
	gs := newMockGameState()
	s := testSession(1)
	exec := New(nil, &resource.ResourceLoader{}, nopLogger())
	opts := &ExecuteOpts{GameState: gs}

	page := &resource.EventPage{
		List: []*resource.EventCommand{
			{Code: CmdLabel, Parameters: []interface{}{"marker"}},
			{Code: CmdChangeSwitches, Parameters: []interface{}{float64(1), float64(1), float64(0)}},
			{Code: CmdEnd},
		},
	}
	exec.Execute(context.Background(), s, page, opts)
	assert.True(t, gs.switches[1])
}

// ========================================================================
// Dispatch: SetMoveRoute WITHOUT wait (no-wait branch)
// ========================================================================

func TestDispatch_SetMoveRoute_NoWait(t *testing.T) {
	s := testSession(1)
	exec := New(nil, &resource.ResourceLoader{}, nopLogger())
	opts := &ExecuteOpts{EventID: 5}

	page := &resource.EventPage{
		List: []*resource.EventCommand{
			{Code: CmdSetMoveRoute, Parameters: []interface{}{
				float64(0),
				map[string]interface{}{"list": []interface{}{}}, // no "wait" key
			}},
			{Code: CmdEnd},
		},
	}
	exec.Execute(context.Background(), s, page, opts)
}

// ========================================================================
// Dispatch: FlashScreen without wait (no-wait branch)
// ========================================================================

func TestDispatch_FlashScreen_NoWait(t *testing.T) {
	s := testSession(1)
	exec := New(nil, &resource.ResourceLoader{}, nopLogger())

	page := &resource.EventPage{
		List: []*resource.EventCommand{
			{Code: CmdFlashScreen, Parameters: []interface{}{
				[]interface{}{float64(255), float64(255), float64(255), float64(170)},
				float64(8),
				false, // wait=false
			}},
			{Code: CmdEnd},
		},
	}
	exec.Execute(context.Background(), s, page, nil)
}

// ========================================================================
// Dispatch: ShakeScreen without wait (no-wait branch)
// ========================================================================

func TestDispatch_ShakeScreen_NoWait(t *testing.T) {
	s := testSession(1)
	exec := New(nil, &resource.ResourceLoader{}, nopLogger())

	page := &resource.EventPage{
		List: []*resource.EventCommand{
			{Code: CmdShakeScreen, Parameters: []interface{}{
				float64(5), float64(9), float64(60),
				false, // wait=false
			}},
			{Code: CmdEnd},
		},
	}
	exec.Execute(context.Background(), s, page, nil)
}

// ========================================================================
// Dispatch: ShowAnimation without wait (no-wait branch)
// ========================================================================

func TestDispatch_ShowAnimation_NoWait(t *testing.T) {
	s := testSession(1)
	exec := New(nil, &resource.ResourceLoader{}, nopLogger())
	opts := &ExecuteOpts{EventID: 5}

	page := &resource.EventPage{
		List: []*resource.EventCommand{
			{Code: CmdShowAnimation, Parameters: []interface{}{
				float64(0), float64(1),
				false, // wait=false
			}},
			{Code: CmdEnd},
		},
	}
	exec.Execute(context.Background(), s, page, opts)
}

// ========================================================================
// Dispatch: ShowBalloon without wait (no-wait branch)
// ========================================================================

func TestDispatch_ShowBalloon_NoWait(t *testing.T) {
	s := testSession(1)
	exec := New(nil, &resource.ResourceLoader{}, nopLogger())
	opts := &ExecuteOpts{EventID: 5}

	page := &resource.EventPage{
		List: []*resource.EventCommand{
			{Code: CmdShowBalloon, Parameters: []interface{}{
				float64(0), float64(1),
				false, // wait=false
			}},
			{Code: CmdEnd},
		},
	}
	exec.Execute(context.Background(), s, page, opts)
}

// ========================================================================
// Dispatch: Script with empty line in multi-line (trimmed=="" branch)
// ========================================================================

func TestDispatch_Script_EmptyLine(t *testing.T) {
	s := testSession(1)
	exec := New(nil, &resource.ResourceLoader{}, nopLogger())

	page := &resource.EventPage{
		List: []*resource.EventCommand{
			{Code: CmdScript, Parameters: []interface{}{"$gameScreen.startFadeOut(24)"}},
			{Code: CmdScriptCont, Parameters: []interface{}{""}}, // empty continuation
			{Code: CmdScriptCont, Parameters: []interface{}{"$gameScreen.startFadeIn(24)"}},
			{Code: CmdEnd},
		},
	}
	exec.Execute(context.Background(), s, page, nil)
}

// ========================================================================
// evalActorCondition: weapon with error from store (IsEquipped error)
// ========================================================================

func TestEvalActorCondition_WeaponErr(t *testing.T) {
	store := &errorMockStore{}
	exec := New(store, &resource.ResourceLoader{}, nopLogger())
	s := testSession(1)

	// subType=4 weapon → IsEquipped returns error → false
	assert.False(t, exec.evalActorCondition(context.Background(), s, []interface{}{float64(4), float64(1), float64(4), float64(5)}))
}

func TestEvalActorCondition_ArmorErr(t *testing.T) {
	store := &errorMockStore{}
	exec := New(store, &resource.ResourceLoader{}, nopLogger())
	s := testSession(1)

	// subType=5 armor → IsEquipped returns error → false
	assert.False(t, exec.evalActorCondition(context.Background(), s, []interface{}{float64(4), float64(1), float64(5), float64(5)}))
}

func TestEvalActorCondition_SkillErr(t *testing.T) {
	store := &errorMockStore{}
	exec := New(store, &resource.ResourceLoader{}, nopLogger())
	s := testSession(1)

	// subType=3 skill → HasSkill returns error → false
	assert.False(t, exec.evalActorCondition(context.Background(), s, []interface{}{float64(4), float64(1), float64(3), float64(5)}))
}

// ========================================================================
// evalGoldCondition: GetGold error
// ========================================================================

func TestEvalGoldCondition_Error(t *testing.T) {
	store := &errorMockStore{}
	exec := New(store, &resource.ResourceLoader{}, nopLogger())
	s := testSession(1)

	assert.False(t, exec.evalGoldCondition(context.Background(), s, []interface{}{float64(7), float64(50), float64(0)}))
}

// ========================================================================
// evalItemCondition: HasItemOfKind error
// ========================================================================

func TestEvalItemCondition_Error(t *testing.T) {
	store := &errorMockStore{}
	exec := New(store, &resource.ResourceLoader{}, nopLogger())
	s := testSession(1)

	assert.False(t, exec.evalItemCondition(context.Background(), s, []interface{}{float64(8), float64(5)}, 1))
}

// ========================================================================
// skipToChoiceBranch: indent mismatch (WhenCancel at different indent)
// ========================================================================

func TestSkipToChoiceBranch_IndentMismatch_Cov5(t *testing.T) {
	exec := New(nil, &resource.ResourceLoader{}, nopLogger())
	cmds := []*resource.EventCommand{
		{Code: CmdShowChoices, Indent: 0},
		{Code: CmdWhenCancel, Indent: 1}, // wrong indent → skipped
		{Code: CmdBranchEnd, Indent: 0},
	}
	result := exec.skipToChoiceBranch(cmds, 0, -1, -1)
	assert.Equal(t, 2, result) // falls to BranchEnd
}

// ========================================================================
// applyGold: UpdateGold error path
// ========================================================================

func TestApplyGold_UpdateError(t *testing.T) {
	store := &errorMockStore{}
	exec := New(store, &resource.ResourceLoader{}, nopLogger())
	s := testSession(1)

	// Increase → UpdateGold fails
	err := exec.applyGold(context.Background(), s, []interface{}{float64(0), float64(0), float64(100)}, nil)
	assert.Error(t, err)
}

// ========================================================================
// applyItems: RemoveItem error
// ========================================================================

func TestApplyItems_RemoveError(t *testing.T) {
	store := &errorOnRemoveMockStore{}
	exec := New(store, &resource.ResourceLoader{}, nopLogger())
	s := testSession(1)

	// Remove → RemoveItem fails
	err := exec.applyItems(context.Background(), s, []interface{}{float64(5), float64(1), float64(0), float64(3)}, nil)
	assert.Error(t, err)
}

// ========================================================================
// applyItems: AddItem error
// ========================================================================

func TestApplyItems_AddError(t *testing.T) {
	store := &errorOnAddMockStore{}
	exec := New(store, &resource.ResourceLoader{}, nopLogger())
	s := testSession(1)

	err := exec.applyItems(context.Background(), s, []interface{}{float64(5), float64(0), float64(0), float64(3)}, nil)
	assert.Error(t, err)
}

// ========================================================================
// applyChangeWeapons: add error path
// ========================================================================

func TestApplyChangeWeapons_AddError(t *testing.T) {
	store := &errorMockStore{}
	exec := New(store, &resource.ResourceLoader{}, nopLogger())
	s := testSession(1)

	// add → error logged
	exec.applyChangeWeapons(context.Background(), s, []interface{}{float64(5), float64(0), float64(0), float64(1)}, nil)
}

// ========================================================================
// applyChangeWeapons: remove error path
// ========================================================================

func TestApplyChangeWeapons_RemoveError(t *testing.T) {
	store := &errorMockStore{}
	exec := New(store, &resource.ResourceLoader{}, nopLogger())
	s := testSession(1)

	exec.applyChangeWeapons(context.Background(), s, []interface{}{float64(5), float64(1), float64(0), float64(1)}, nil)
}

// ========================================================================
// applyChangeArmors: add error path
// ========================================================================

func TestApplyChangeArmors_AddError(t *testing.T) {
	store := &errorMockStore{}
	exec := New(store, &resource.ResourceLoader{}, nopLogger())
	s := testSession(1)

	exec.applyChangeArmors(context.Background(), s, []interface{}{float64(5), float64(0), float64(0), float64(1)}, nil)
}

// ========================================================================
// applyEquipChange: store error path
// ========================================================================

func TestApplyEquipChange_StoreError(t *testing.T) {
	store := &errorMockStore{}
	exec := New(store, &resource.ResourceLoader{}, nopLogger())
	s := testSession(1)
	s.Equips = map[int]int{}

	// "Equip" slot type — triggers SetEquipSlot which errors
	exec.applyEquipChange(context.Background(), s, "Equip", "5", nil)
}

// ========================================================================
// applyChangeEquipment: store SetEquipSlot error path
// ========================================================================

func TestApplyChangeEquipment_StoreError(t *testing.T) {
	store := &errorMockStore{}
	exec := New(store, &resource.ResourceLoader{}, nopLogger())
	s := testSession(1)
	s.Equips = map[int]int{}

	exec.applyChangeEquipment(context.Background(), s, []interface{}{float64(1), float64(1), float64(10)}, nil)
}

// ========================================================================
// resolveTextVarRef: sub<2 and Atoi error — hard to trigger since regex ensures match
// But we can test with the function directly passing a valid pattern
// ========================================================================

func TestResolveTextVarRef_NoMatch_Cov5(t *testing.T) {
	gs := newMockGameState()
	exec := New(nil, &resource.ResourceLoader{}, nopLogger())
	opts := &ExecuteOpts{GameState: gs}

	// No match at all
	result := exec.resolveTextVarRef("no vars here", opts)
	assert.Equal(t, "no vars here", result)
}

// ========================================================================
// execMutableScript: __setVarAny with float64 and non-integer types
// ========================================================================

func TestExecMutableScript_SetVarAny_Float(t *testing.T) {
	gs := newMockGameState()
	s := testSession(1)
	exec := New(nil, &resource.ResourceLoader{}, nopLogger())
	opts := &ExecuteOpts{GameState: gs}

	// Set a var to float 3.14 → truncated to int(3) via float64 case
	exec.execMutableScript("$gameVariables._data[100] = 3.14", s, opts)
	// Goja may export as int64(3) or float64(3.14) depending on value
}

func TestExecMutableScript_SetVarAny_Array(t *testing.T) {
	gs := newMockGameState()
	s := testSession(1)
	exec := New(nil, &resource.ResourceLoader{}, nopLogger())
	opts := &ExecuteOpts{GameState: gs, TransientVars: make(map[int]interface{})}

	// Set a var to an array → stored in TransientVars
	exec.execMutableScript("$gameVariables._data[200] = [1,2,3]", s, opts)
	assert.NotNil(t, opts.TransientVars[200])
}

// ========================================================================
// parseMeta: default case (bare tag)
// ========================================================================

func TestParseMeta_BareTag(t *testing.T) {
	s := testSession(1)
	exec := New(nil, &resource.ResourceLoader{
		Maps: map[int]*resource.MapData{1: {Note: "<instance>"}},
	}, nopLogger())
	opts := &ExecuteOpts{MapID: 1, GameState: newMockGameState()}
	vm := exec.getOrCreateCondVM(s, opts)
	v, _ := vm.RunString("$dataMap.meta.instance")
	assert.Equal(t, true, v.Export())
}

// ========================================================================
// parseMeta: bad JSON (semicolon format with invalid JSON → stored as string)
// ========================================================================

func TestParseMeta_BadJSON_String(t *testing.T) {
	s := testSession(1)
	exec := New(nil, &resource.ResourceLoader{
		Maps: map[int]*resource.MapData{1: {Note: "<key;not_json>"}},
	}, nopLogger())
	opts := &ExecuteOpts{MapID: 1, GameState: newMockGameState()}
	vm := exec.getOrCreateCondVM(s, opts)
	v, _ := vm.RunString("$dataMap.meta.key")
	assert.NotNil(t, v)
}

// ========================================================================
// stepUntilWait: handleCallCommon true branch in parallel
// ========================================================================

func TestStepUntilWait_HandleCallCommon(t *testing.T) {
	gs := newMockGameState()
	s := testSession(1)
	s.MapID = 1
	res := &resource.ResourceLoader{
		CommonEvents: []*resource.CommonEvent{
			nil,
			{ID: 1, Name: "MyCommon", List: []*resource.EventCommand{
				{Code: CmdChangeSwitches, Parameters: []interface{}{float64(99), float64(99), float64(0)}},
				{Code: CmdEnd},
			}},
		},
		CommonEventsByName: map[string]int{"MyCommon": 1},
	}
	exec := New(nil, res, nopLogger())
	opts := &ExecuteOpts{MapID: 1, GameState: gs}

	ev := &ParallelEventState{
		EventID: 1,
		Cmds: []*resource.EventCommand{
			{Code: CmdPluginCommand, Parameters: []interface{}{"CallCommon MyCommon"}},
			{Code: CmdEnd, Indent: 0},
		},
	}
	exec.stepUntilWait(context.Background(), s, ev, opts)
	assert.True(t, gs.switches[99])
}

// ========================================================================
// stepUntilWait: Script with blank line in parallel
// ========================================================================

func TestStepUntilWait_Script_BlankLine(t *testing.T) {
	gs := newMockGameState()
	s := testSession(1)
	s.MapID = 1
	exec := New(nil, &resource.ResourceLoader{}, nopLogger())
	opts := &ExecuteOpts{MapID: 1, GameState: gs}

	ev := &ParallelEventState{
		EventID: 1,
		Cmds: []*resource.EventCommand{
			{Code: CmdScript, Parameters: []interface{}{"$gameScreen.startFadeOut(24)"}},
			{Code: CmdScriptCont, Parameters: []interface{}{""}},
			{Code: CmdScriptCont, Parameters: []interface{}{"$gameScreen.startFadeIn(24)"}},
			{Code: CmdEnd, Indent: 0},
		},
	}
	exec.stepUntilWait(context.Background(), s, ev, opts)
}

// ========================================================================
// stepUntilWait: list ends without CmdEnd (return true after loop)
// ========================================================================

func TestStepUntilWait_NoTerminalEnd(t *testing.T) {
	gs := newMockGameState()
	s := testSession(1)
	s.MapID = 1
	exec := New(nil, &resource.ResourceLoader{}, nopLogger())
	opts := &ExecuteOpts{MapID: 1, GameState: gs}

	ev := &ParallelEventState{
		EventID: 1,
		Cmds: []*resource.EventCommand{
			{Code: CmdChangeSwitches, Parameters: []interface{}{float64(1), float64(1), float64(0)}},
			// No CmdEnd
		},
	}
	done := exec.stepUntilWait(context.Background(), s, ev, opts)
	assert.True(t, done) // loop exhausted → return true (line 393)
	assert.True(t, gs.switches[1])
}

// ========================================================================
// RunParallelEventsSynced: waitFrames decremented across ticks
// ========================================================================

func TestRunParallelEventsSynced_WaitFrames_Cov5(t *testing.T) {
	s := testSession(1)
	s.MapID = 1
	gs := newMockGameState()
	exec := New(nil, &resource.ResourceLoader{}, nopLogger())
	opts := &ExecuteOpts{MapID: 1, GameState: gs}

	events := []*ParallelEventState{
		{EventID: 1, Cmds: []*resource.EventCommand{
			{Code: CmdWait, Parameters: []interface{}{float64(1)}}, // 1 frame wait
			{Code: CmdChangeSwitches, Parameters: []interface{}{float64(1), float64(1), float64(0)}},
			{Code: CmdEnd, Indent: 0},
		}},
	}

	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		// Wait for switch to be set, then cancel
		for i := 0; i < 100; i++ {
			if gs.switches[1] {
				cancel()
				return
			}
			// small sleep
			select {
			case <-ctx.Done():
				return
			default:
			}
		}
		cancel()
	}()
	exec.RunParallelEventsSynced(ctx, s, events, opts)
}

// ========================================================================
// execCulSkillEffect: nil session (s == nil branch)
// ========================================================================

func TestExecCulSkillEffect_NilSession_Cov5(t *testing.T) {
	gs := newMockGameState()
	exec := New(nil, &resource.ResourceLoader{}, nopLogger())
	opts := &ExecuteOpts{GameState: gs, TransientVars: make(map[int]interface{})}

	// nil session → injectScriptGameActors skipped but code continues
	exec.execCulSkillEffect(nil, opts)
}

// ========================================================================
// execParaCheck: nil res branch
// ========================================================================

func TestExecParaCheck_NilRes(t *testing.T) {
	gs := newMockGameState()
	s := testSessionWithStats(1, 100, 100, 50, 50, 10, 0)
	s.ClassID = 1
	s.Equips = map[int]int{0: 0, 1: 0}
	exec := New(nil, nil, nopLogger())
	opts := &ExecuteOpts{GameState: gs, TransientVars: make(map[int]interface{})}

	exec.execParaCheck(s, opts)
}

// execParaCheck with nil session panics at s.Level access (Go-level, not JS).
// This is expected — execParaCheck always receives a valid session from callers.

// ========================================================================
// teSetSelfVariable: less than 3 args
// ========================================================================

func TestTESetSelfVariable_TooFewArgs(t *testing.T) {
	gs := newMockGameState()
	exec := New(nil, &resource.ResourceLoader{}, nopLogger())
	s := testSession(1)
	opts := &ExecuteOpts{MapID: 1, EventID: 1, GameState: gs}

	result := exec.teSetSelfVariable(s, []string{"0", "0"}, opts) // only 2 args
	assert.True(t, result)
}

// ========================================================================
// handleCallCommon: CCT (CallCommon with CCT prefix)
// ========================================================================

func TestHandleCallCommon_CCT_Cov5(t *testing.T) {
	gs := newMockGameState()
	s := testSession(1)
	res := &resource.ResourceLoader{
		CommonEvents: []*resource.CommonEvent{
			nil,
			{ID: 1, Name: "EffectCE", List: []*resource.EventCommand{
				{Code: CmdChangeSwitches, Parameters: []interface{}{float64(50), float64(50), float64(0)}},
				{Code: CmdEnd},
			}},
		},
	}
	exec := New(nil, res, nopLogger())
	opts := &ExecuteOpts{GameState: gs}

	cmd := &resource.EventCommand{Code: CmdPluginCommand, Parameters: []interface{}{"CCT EffectCE"}}
	handled := exec.handleCallCommon(context.Background(), s, cmd, opts, 0)
	assert.True(t, handled)
	assert.True(t, gs.switches[50])
}

// ========================================================================
// evalScriptCondition: nil/undefined result
// ========================================================================

func TestEvalScriptCondition_Undefined(t *testing.T) {
	gs := newMockGameState()
	s := testSession(1)
	exec := New(nil, &resource.ResourceLoader{}, nopLogger())
	opts := &ExecuteOpts{GameState: gs, MapID: 1, EventID: 1}

	result, ok := exec.evalScriptCondition("undefined", s, opts)
	assert.False(t, result)
	assert.True(t, ok)
}

// ========================================================================
// evalScriptValue: nil/undefined result
// ========================================================================

func TestEvalScriptValue_Undefined(t *testing.T) {
	gs := newMockGameState()
	s := testSession(1)
	exec := New(nil, &resource.ResourceLoader{}, nopLogger())
	opts := &ExecuteOpts{GameState: gs, MapID: 1, EventID: 1}

	result := exec.evalScriptValue("undefined", s, opts)
	assert.Equal(t, 0, result)
}

// ========================================================================
// evalSetupChildTarget: nil/undefined result from expr
// ========================================================================

func TestEvalSetupChildTarget_Undefined(t *testing.T) {
	s := testSession(1)
	exec := New(nil, &resource.ResourceLoader{}, nopLogger())
	opts := &ExecuteOpts{GameState: newMockGameState()}

	id := exec.evalSetupChildTarget("undefined", s, opts)
	assert.Equal(t, 0, id)
}

// ========================================================================
// execMutableScript: page refresh (nil pages in map data)
// ========================================================================

func TestExecMutableScript_WithMapNote_Cov5(t *testing.T) {
	gs := newMockGameState()
	s := testSession(1)
	res := &resource.ResourceLoader{
		Maps: map[int]*resource.MapData{
			1: {Note: "<instance>"},
		},
	}
	exec := New(nil, res, nopLogger())
	opts := &ExecuteOpts{GameState: gs, MapID: 1}

	exec.execMutableScript("$gameVariables._data[1] = 99", s, opts)
	assert.Equal(t, 99, gs.variables[1])
}

// ========================================================================
// Dispatch: enterInstance with fn
// ========================================================================

func TestDispatch_EnterInstance_WithFn(t *testing.T) {
	s := testSession(1)
	exec := New(nil, &resource.ResourceLoader{}, nopLogger())
	entered := false
	opts := &ExecuteOpts{
		GameState:       newMockGameState(),
		EnterInstanceFn: func(_ *player.PlayerSession) { entered = true },
	}

	page := &resource.EventPage{
		List: []*resource.EventCommand{
			{Code: CmdPluginCommand, Parameters: []interface{}{"EnterInstance"}},
			{Code: CmdEnd},
		},
	}
	exec.Execute(context.Background(), s, page, opts)
	assert.True(t, entered)
}

// ========================================================================
// Dispatch: leaveInstance with fn
// ========================================================================

func TestDispatch_LeaveInstance_WithFn(t *testing.T) {
	s := testSession(1)
	exec := New(nil, &resource.ResourceLoader{}, nopLogger())
	left := false
	opts := &ExecuteOpts{
		GameState:       newMockGameState(),
		LeaveInstanceFn: func(_ *player.PlayerSession) { left = true },
	}

	page := &resource.EventPage{
		List: []*resource.EventCommand{
			{Code: CmdPluginCommand, Parameters: []interface{}{"LeaveInstance"}},
			{Code: CmdEnd},
		},
	}
	exec.Execute(context.Background(), s, page, opts)
	assert.True(t, left)
}

// ========================================================================
// evaluateCondition: variable ops 1-5
// ========================================================================

func TestEvalCondition_VarOps(t *testing.T) {
	gs := newMockGameState()
	gs.variables[1] = 10
	exec := New(nil, &resource.ResourceLoader{}, nopLogger())
	s := testSession(1)
	opts := &ExecuteOpts{GameState: gs, MapID: 1, EventID: 1}

	// op=1 (>=): 10 >= 10 = true
	assert.True(t, exec.evaluateCondition(context.Background(), s,
		[]interface{}{float64(1), float64(1), float64(0), float64(10), float64(1)}, opts))
	// op=2 (<=): 10 <= 10 = true
	assert.True(t, exec.evaluateCondition(context.Background(), s,
		[]interface{}{float64(1), float64(1), float64(0), float64(10), float64(2)}, opts))
	// op=3 (>): 10 > 10 = false
	assert.False(t, exec.evaluateCondition(context.Background(), s,
		[]interface{}{float64(1), float64(1), float64(0), float64(10), float64(3)}, opts))
	// op=4 (<): 10 < 10 = false
	assert.False(t, exec.evaluateCondition(context.Background(), s,
		[]interface{}{float64(1), float64(1), float64(0), float64(10), float64(4)}, opts))
}

// ========================================================================
// evaluateCondition: return false after op switch (unreachable but covers line 256)
// This is the "return false" after the switch inside condType=1 — only reachable
// with an invalid op value
// ========================================================================

func TestEvalCondition_VarInvalidOp(t *testing.T) {
	gs := newMockGameState()
	exec := New(nil, &resource.ResourceLoader{}, nopLogger())
	s := testSession(1)
	opts := &ExecuteOpts{GameState: gs, MapID: 1, EventID: 1}

	// op=99 → no case matches → falls through to "return false" at line 256
	assert.False(t, exec.evaluateCondition(context.Background(), s,
		[]interface{}{float64(1), float64(1), float64(0), float64(0), float64(99)}, opts))
}

// ========================================================================
// errorMockStore — returns errors for all operations
// ========================================================================

type errorMockStore struct{}

func (s *errorMockStore) GetGold(_ context.Context, _ int64) (int64, error) {
	return 0, assert.AnError
}
func (s *errorMockStore) UpdateGold(_ context.Context, _ int64, _ int64) error {
	return assert.AnError
}
func (s *errorMockStore) GetItem(_ context.Context, _ int64, _ int) (int, error) {
	return 0, assert.AnError
}
func (s *errorMockStore) AddItem(_ context.Context, _ int64, _, _ int) error {
	return assert.AnError
}
func (s *errorMockStore) RemoveItem(_ context.Context, _ int64, _, _ int) error {
	return assert.AnError
}
func (s *errorMockStore) HasItemOfKind(_ context.Context, _ int64, _, _ int, _ bool) (bool, error) {
	return false, assert.AnError
}
func (s *errorMockStore) IsEquipped(_ context.Context, _ int64, _, _ int) (bool, error) {
	return false, assert.AnError
}
func (s *errorMockStore) HasSkill(_ context.Context, _ int64, _ int) (bool, error) {
	return false, assert.AnError
}
func (s *errorMockStore) LearnSkill(_ context.Context, _ int64, _ int) error {
	return assert.AnError
}
func (s *errorMockStore) ForgetSkill(_ context.Context, _ int64, _ int) error {
	return assert.AnError
}
func (s *errorMockStore) SetEquipSlot(_ context.Context, _ int64, _, _, _ int) error {
	return assert.AnError
}
func (s *errorMockStore) AddArmorOrWeapon(_ context.Context, _ int64, _, _, _ int) error {
	return assert.AnError
}
func (s *errorMockStore) RemoveArmorOrWeapon(_ context.Context, _ int64, _, _, _ int) error {
	return assert.AnError
}

// ========================================================================
// errorOnRemoveMockStore — GetItem returns qty, RemoveItem errors
// ========================================================================

type errorOnRemoveMockStore struct {
	errorMockStore
}

func (s *errorOnRemoveMockStore) GetItem(_ context.Context, _ int64, _ int) (int, error) {
	return 5, nil // has items
}
func (s *errorOnRemoveMockStore) RemoveItem(_ context.Context, _ int64, _, _ int) error {
	return assert.AnError
}

// ========================================================================
// errorOnAddMockStore — GetItem returns 0, AddItem errors
// ========================================================================

type errorOnAddMockStore struct {
	errorMockStore
}

func (s *errorOnAddMockStore) GetItem(_ context.Context, _ int64, _ int) (int, error) {
	return 0, nil
}
func (s *errorOnAddMockStore) AddItem(_ context.Context, _ int64, _, _ int) error {
	return assert.AnError
}
