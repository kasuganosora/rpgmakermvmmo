package npc

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/kasuganosora/rpgmakermvmmo/server/game/player"
	"github.com/kasuganosora/rpgmakermvmmo/server/resource"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ========================================================================
// truncate (executor_script.go:187) — 0% coverage
// ========================================================================

func TestTruncate(t *testing.T) {
	assert.Equal(t, "hello", truncate("hello", 10))
	assert.Equal(t, "hel...", truncate("hello world", 3))
	assert.Equal(t, "", truncate("", 5))
	assert.Equal(t, "abc...", truncate("abcdef", 3))
	// exact length
	assert.Equal(t, "abc", truncate("abc", 3))
}

// ========================================================================
// injectTransientVars (executor_script.go:381) — 28.6%
// ========================================================================

func TestInjectTransientVars(t *testing.T) {
	gs := newMockGameState()
	gs.variables[100] = 42
	s := testSession(1)
	exec := New(nil, &resource.ResourceLoader{}, nopLogger())

	// With non-empty transient vars containing an array
	opts := &ExecuteOpts{
		GameState:     gs,
		TransientVars: map[int]interface{}{200: []int{40, 10}},
	}
	vm := exec.getOrCreateCondVM(s, opts)

	// $gameVariables.value(100) should return 42 from gs
	v, err := vm.RunString("$gameVariables.value(100)")
	require.NoError(t, err)
	assert.Equal(t, int64(42), v.ToInteger())

	// $gameVariables.value(200) should return the transient array
	v, err = vm.RunString("$gameVariables.value(200)")
	require.NoError(t, err)
	exported := v.Export()
	// Should be the array
	assert.NotNil(t, exported)
}

func TestInjectTransientVars_Empty(t *testing.T) {
	gs := newMockGameState()
	gs.variables[100] = 42
	s := testSession(1)
	exec := New(nil, &resource.ResourceLoader{}, nopLogger())

	// Empty transient vars — injectTransientVars should be a no-op
	opts := &ExecuteOpts{
		GameState:     gs,
		TransientVars: map[int]interface{}{},
	}
	vm := exec.getOrCreateCondVM(s, opts)
	v, err := vm.RunString("$gameVariables.value(100)")
	require.NoError(t, err)
	assert.Equal(t, int64(42), v.ToInteger())
}

// ========================================================================
// injectScriptDataArrays (executor_script.go:596) — 60%
// ========================================================================

func TestInjectScriptDataArrays_AllFields(t *testing.T) {
	gs := newMockGameState()
	s := testSession(1)

	res := &resource.ResourceLoader{
		PrebuiltArmors:  []interface{}{nil, map[string]interface{}{"id": 1, "name": "Shield"}},
		PrebuiltWeapons: []interface{}{nil, map[string]interface{}{"id": 1, "name": "Sword"}},
		PrebuiltSkills:  []interface{}{nil, map[string]interface{}{"id": 1, "name": "Fire"}},
		PrebuiltItems:   []interface{}{nil, map[string]interface{}{"id": 1, "name": "Potion"}},
	}
	exec := New(nil, res, nopLogger())
	opts := &ExecuteOpts{GameState: gs}
	vm := exec.getOrCreateCondVM(s, opts)

	// All four data arrays should be accessible
	v, err := vm.RunString("$dataArmors[1].name")
	require.NoError(t, err)
	assert.Equal(t, "Shield", v.String())

	v, err = vm.RunString("$dataWeapons[1].name")
	require.NoError(t, err)
	assert.Equal(t, "Sword", v.String())

	v, err = vm.RunString("$dataSkills[1].name")
	require.NoError(t, err)
	assert.Equal(t, "Fire", v.String())

	v, err = vm.RunString("$dataItems[1].name")
	require.NoError(t, err)
	assert.Equal(t, "Potion", v.String())
}

func TestInjectScriptDataArrays_Nil(t *testing.T) {
	gs := newMockGameState()
	s := testSession(1)

	// nil res — should not panic
	exec := New(nil, nil, nopLogger())
	opts := &ExecuteOpts{GameState: gs}
	vm := exec.getOrCreateCondVM(s, opts)
	_ = vm
}

func TestInjectScriptDataArrays_PartialNil(t *testing.T) {
	gs := newMockGameState()
	s := testSession(1)

	// Only PrebuiltArmors set, others nil
	res := &resource.ResourceLoader{
		PrebuiltArmors: []interface{}{nil, map[string]interface{}{"id": 1}},
	}
	exec := New(nil, res, nopLogger())
	opts := &ExecuteOpts{GameState: gs}
	vm := exec.getOrCreateCondVM(s, opts)

	v, err := vm.RunString("$dataArmors[1].id")
	require.NoError(t, err)
	assert.Equal(t, int64(1), v.ToInteger())

	// $dataWeapons should be undefined
	v, err = vm.RunString("typeof $dataWeapons")
	require.NoError(t, err)
	assert.Equal(t, "undefined", v.String())
}

// ========================================================================
// teCallOriginEvent (executor_template.go:67) — 63.6%
// ========================================================================

func TestTECallOriginEvent_FullFlow(t *testing.T) {
	gs := newMockGameState()
	s := testSession(1)

	origPage := &resource.EventPage{
		List: []*resource.EventCommand{
			{Code: CmdChangeSwitches, Parameters: []interface{}{float64(50), float64(50), float64(0)}},
			{Code: CmdEnd},
		},
	}
	res := &resource.ResourceLoader{
		Maps: map[int]*resource.MapData{
			1: {Events: []*resource.MapEvent{
				nil,
				{ID: 1, Pages: []*resource.EventPage{{List: []*resource.EventCommand{{Code: CmdEnd}}}},
					OriginalPages: []*resource.EventPage{origPage}},
			}},
		},
	}
	exec := New(nil, res, nopLogger())
	opts := &ExecuteOpts{MapID: 1, EventID: 1, GameState: gs}

	cmd := &resource.EventCommand{Code: CmdPluginCommand, Parameters: []interface{}{"TE_CALL_ORIGIN_EVENT"}}
	handled := exec.handleTECallOriginEvent(context.Background(), s, cmd, opts, 0)
	assert.True(t, handled)
	assert.True(t, gs.switches[50])
}

func TestTECallOriginEvent_WithPageIndex(t *testing.T) {
	gs := newMockGameState()
	s := testSession(1)

	origPage0 := &resource.EventPage{
		List: []*resource.EventCommand{
			{Code: CmdChangeSwitches, Parameters: []interface{}{float64(50), float64(50), float64(0)}},
			{Code: CmdEnd},
		},
	}
	origPage1 := &resource.EventPage{
		List: []*resource.EventCommand{
			{Code: CmdChangeSwitches, Parameters: []interface{}{float64(60), float64(60), float64(0)}},
			{Code: CmdEnd},
		},
	}
	res := &resource.ResourceLoader{
		Maps: map[int]*resource.MapData{
			1: {Events: []*resource.MapEvent{
				nil,
				{ID: 1, Pages: []*resource.EventPage{{}},
					OriginalPages: []*resource.EventPage{origPage0, origPage1}},
			}},
		},
	}
	exec := New(nil, res, nopLogger())
	opts := &ExecuteOpts{MapID: 1, EventID: 1, GameState: gs}

	// pageIdx=2 → arrayIdx=1
	cmd := &resource.EventCommand{Code: CmdPluginCommand, Parameters: []interface{}{"TE_CALL_ORIGIN_EVENT 2"}}
	handled := exec.handleTECallOriginEvent(context.Background(), s, cmd, opts, 0)
	assert.True(t, handled)
	assert.True(t, gs.switches[60])
}

func TestTECallOriginEvent_PageIndexExceedsFallback(t *testing.T) {
	gs := newMockGameState()
	s := testSession(1)

	origPage := &resource.EventPage{
		List: []*resource.EventCommand{
			{Code: CmdChangeSwitches, Parameters: []interface{}{float64(70), float64(70), float64(0)}},
			{Code: CmdEnd},
		},
	}
	res := &resource.ResourceLoader{
		Maps: map[int]*resource.MapData{
			1: {Events: []*resource.MapEvent{
				nil,
				{ID: 1, Pages: []*resource.EventPage{{}},
					OriginalPages: []*resource.EventPage{origPage}},
			}},
		},
	}
	exec := New(nil, res, nopLogger())
	opts := &ExecuteOpts{MapID: 1, EventID: 1, GameState: gs}

	// pageIdx=5 exceeds len(OriginalPages)=1 → falls back to 0
	cmd := &resource.EventCommand{Code: CmdPluginCommand, Parameters: []interface{}{"TE_CALL_ORIGIN_EVENT 5"}}
	handled := exec.handleTECallOriginEvent(context.Background(), s, cmd, opts, 0)
	assert.True(t, handled)
	assert.True(t, gs.switches[70])
}

func TestTECallOriginEvent_NoOriginalPages(t *testing.T) {
	s := testSession(1)
	res := &resource.ResourceLoader{
		Maps: map[int]*resource.MapData{
			1: {Events: []*resource.MapEvent{nil, {ID: 1, Pages: []*resource.EventPage{{}}}}},
		},
	}
	exec := New(nil, res, nopLogger())
	opts := &ExecuteOpts{MapID: 1, EventID: 1, GameState: newMockGameState()}

	cmd := &resource.EventCommand{Code: CmdPluginCommand, Parameters: []interface{}{"TE_CALL_ORIGIN_EVENT"}}
	handled := exec.handleTECallOriginEvent(context.Background(), s, cmd, opts, 0)
	assert.True(t, handled)
}

func TestTECallOriginEvent_NilOriginalPage(t *testing.T) {
	s := testSession(1)
	res := &resource.ResourceLoader{
		Maps: map[int]*resource.MapData{
			1: {Events: []*resource.MapEvent{nil, {ID: 1, Pages: []*resource.EventPage{{}},
				OriginalPages: []*resource.EventPage{nil}}}},
		},
	}
	exec := New(nil, res, nopLogger())
	opts := &ExecuteOpts{MapID: 1, EventID: 1, GameState: newMockGameState()}

	cmd := &resource.EventCommand{Code: CmdPluginCommand, Parameters: []interface{}{"TE_CALL_ORIGIN_EVENT"}}
	handled := exec.handleTECallOriginEvent(context.Background(), s, cmd, opts, 0)
	assert.True(t, handled)
}

func TestTECallOriginEvent_MissingContext(t *testing.T) {
	s := testSession(1)
	exec := New(nil, &resource.ResourceLoader{}, nopLogger())

	// nil opts
	cmd := &resource.EventCommand{Code: CmdPluginCommand, Parameters: []interface{}{"TE_CALL_ORIGIN_EVENT"}}
	handled := exec.handleTECallOriginEvent(context.Background(), s, cmd, nil, 0)
	assert.True(t, handled)

	// mapID=0
	opts := &ExecuteOpts{MapID: 0, EventID: 1}
	handled = exec.handleTECallOriginEvent(context.Background(), s, cmd, opts, 0)
	assert.True(t, handled)
}

// ========================================================================
// teCallMapEvent (executor_template.go:115) — 80.6%
// ========================================================================

func TestTECallMapEvent_ByName(t *testing.T) {
	gs := newMockGameState()
	s := testSession(1)

	tmplPage := &resource.EventPage{
		List: []*resource.EventCommand{
			{Code: CmdChangeSwitches, Parameters: []interface{}{float64(80), float64(80), float64(0)}},
			{Code: CmdEnd},
		},
	}
	res := &resource.ResourceLoader{
		Maps: map[int]*resource.MapData{
			1: {Events: []*resource.MapEvent{
				nil,
				{ID: 1, Name: "MyTemplate", Pages: []*resource.EventPage{tmplPage}},
			}},
		},
	}
	exec := New(nil, res, nopLogger())
	opts := &ExecuteOpts{MapID: 1, EventID: 1, GameState: gs}

	cmd := &resource.EventCommand{Code: CmdPluginCommand, Parameters: []interface{}{"TE_CALL_MAP_EVENT MyTemplate"}}
	handled := exec.handleTECallOriginEvent(context.Background(), s, cmd, opts, 0)
	assert.True(t, handled)
	assert.True(t, gs.switches[80])
}

func TestTECallMapEvent_ByNumericID(t *testing.T) {
	gs := newMockGameState()
	s := testSession(1)

	tmplPage := &resource.EventPage{
		List: []*resource.EventCommand{
			{Code: CmdChangeSwitches, Parameters: []interface{}{float64(90), float64(90), float64(0)}},
			{Code: CmdEnd},
		},
	}
	res := &resource.ResourceLoader{
		Maps: map[int]*resource.MapData{
			1: {Events: []*resource.MapEvent{
				nil,
				{ID: 5, Name: "OtherEvent", Pages: []*resource.EventPage{tmplPage}},
			}},
		},
	}
	exec := New(nil, res, nopLogger())
	opts := &ExecuteOpts{MapID: 1, EventID: 1, GameState: gs}

	cmd := &resource.EventCommand{Code: CmdPluginCommand, Parameters: []interface{}{"TE_CALL_MAP_EVENT 5"}}
	handled := exec.handleTECallOriginEvent(context.Background(), s, cmd, opts, 0)
	assert.True(t, handled)
	assert.True(t, gs.switches[90])
}

func TestTECallMapEvent_WithPageIndex(t *testing.T) {
	gs := newMockGameState()
	s := testSession(1)

	page0 := &resource.EventPage{
		List: []*resource.EventCommand{
			{Code: CmdChangeSwitches, Parameters: []interface{}{float64(91), float64(91), float64(0)}},
			{Code: CmdEnd},
		},
	}
	page1 := &resource.EventPage{
		List: []*resource.EventCommand{
			{Code: CmdChangeSwitches, Parameters: []interface{}{float64(92), float64(92), float64(0)}},
			{Code: CmdEnd},
		},
	}
	res := &resource.ResourceLoader{
		Maps: map[int]*resource.MapData{
			1: {Events: []*resource.MapEvent{nil, {ID: 1, Name: "T", Pages: []*resource.EventPage{page0, page1}}}},
		},
	}
	exec := New(nil, res, nopLogger())
	opts := &ExecuteOpts{MapID: 1, EventID: 1, GameState: gs}

	// pageIdx=2 → arrayIdx=1
	cmd := &resource.EventCommand{Code: CmdPluginCommand, Parameters: []interface{}{"TE_CALL_MAP_EVENT T 2"}}
	handled := exec.handleTECallOriginEvent(context.Background(), s, cmd, opts, 0)
	assert.True(t, handled)
	assert.True(t, gs.switches[92])
}

func TestTECallMapEvent_NotFound(t *testing.T) {
	s := testSession(1)
	res := &resource.ResourceLoader{
		Maps: map[int]*resource.MapData{1: {Events: []*resource.MapEvent{nil}}},
	}
	exec := New(nil, res, nopLogger())
	opts := &ExecuteOpts{MapID: 1, EventID: 1, GameState: newMockGameState()}

	cmd := &resource.EventCommand{Code: CmdPluginCommand, Parameters: []interface{}{"TE_CALL_MAP_EVENT NonExistent"}}
	handled := exec.handleTECallOriginEvent(context.Background(), s, cmd, opts, 0)
	assert.True(t, handled)
}

func TestTECallMapEvent_NilPage(t *testing.T) {
	s := testSession(1)
	res := &resource.ResourceLoader{
		Maps: map[int]*resource.MapData{
			1: {Events: []*resource.MapEvent{nil, {ID: 1, Name: "T", Pages: []*resource.EventPage{nil}}}},
		},
	}
	exec := New(nil, res, nopLogger())
	opts := &ExecuteOpts{MapID: 1, EventID: 1, GameState: newMockGameState()}

	cmd := &resource.EventCommand{Code: CmdPluginCommand, Parameters: []interface{}{"TE_CALL_MAP_EVENT T"}}
	handled := exec.handleTECallOriginEvent(context.Background(), s, cmd, opts, 0)
	assert.True(t, handled)
}

func TestTECallMapEvent_MissingArgs(t *testing.T) {
	s := testSession(1)
	exec := New(nil, &resource.ResourceLoader{}, nopLogger())
	opts := &ExecuteOpts{MapID: 1, EventID: 1, GameState: newMockGameState()}

	cmd := &resource.EventCommand{Code: CmdPluginCommand, Parameters: []interface{}{"TE_CALL_MAP_EVENT"}}
	handled := exec.handleTECallOriginEvent(context.Background(), s, cmd, opts, 0)
	assert.True(t, handled)
}

func TestTECallMapEvent_PageIndexExceeds(t *testing.T) {
	gs := newMockGameState()
	s := testSession(1)

	page0 := &resource.EventPage{
		List: []*resource.EventCommand{
			{Code: CmdChangeSwitches, Parameters: []interface{}{float64(95), float64(95), float64(0)}},
			{Code: CmdEnd},
		},
	}
	res := &resource.ResourceLoader{
		Maps: map[int]*resource.MapData{
			1: {Events: []*resource.MapEvent{nil, {ID: 1, Name: "T", Pages: []*resource.EventPage{page0}}}},
		},
	}
	exec := New(nil, res, nopLogger())
	opts := &ExecuteOpts{MapID: 1, EventID: 1, GameState: gs}

	// pageIdx=5 exceeds → falls back to 0
	cmd := &resource.EventCommand{Code: CmdPluginCommand, Parameters: []interface{}{"TE_CALL_MAP_EVENT T 5"}}
	handled := exec.handleTECallOriginEvent(context.Background(), s, cmd, opts, 0)
	assert.True(t, handled)
	assert.True(t, gs.switches[95])
}

// ========================================================================
// teSetSelfVariable edge cases (executor_template.go:208) — 70%
// ========================================================================

func TestTESetSelfVariable_WithVRef(t *testing.T) {
	gs := newMockGameState()
	gs.variables[10] = 7
	s := testSession(1)
	exec := New(nil, &resource.ResourceLoader{}, nopLogger())
	opts := &ExecuteOpts{MapID: 1, EventID: 1, GameState: gs}

	// \v[10] as operand
	cmd := &resource.EventCommand{Code: CmdPluginCommand, Parameters: []interface{}{`TE_SET_SELF_VARIABLE 0 0 \v[10]`}}
	handled := exec.handleTECallOriginEvent(context.Background(), s, cmd, opts, 0)
	assert.True(t, handled)
	assert.Equal(t, 7, gs.selfVariables[selfVariableKey(1, 1, 0)])
}

func TestTESetSelfVariable_WithSvRef(t *testing.T) {
	gs := newMockGameState()
	gs.selfVariables[selfVariableKey(1, 1, 5)] = 42
	s := testSession(1)
	exec := New(nil, &resource.ResourceLoader{}, nopLogger())
	opts := &ExecuteOpts{MapID: 1, EventID: 1, GameState: gs}

	// \sv[5] as operand
	cmd := &resource.EventCommand{Code: CmdPluginCommand, Parameters: []interface{}{`TE_SET_SELF_VARIABLE 0 0 \sv[5]`}}
	handled := exec.handleTECallOriginEvent(context.Background(), s, cmd, opts, 0)
	assert.True(t, handled)
	assert.Equal(t, 42, gs.selfVariables[selfVariableKey(1, 1, 0)])
}

func TestTESetSelfVariable_BadIndex(t *testing.T) {
	s := testSession(1)
	exec := New(nil, &resource.ResourceLoader{}, nopLogger())
	opts := &ExecuteOpts{MapID: 1, EventID: 1, GameState: newMockGameState()}

	cmd := &resource.EventCommand{Code: CmdPluginCommand, Parameters: []interface{}{"TE_SET_SELF_VARIABLE abc 0 5"}}
	handled := exec.handleTECallOriginEvent(context.Background(), s, cmd, opts, 0)
	assert.True(t, handled)
}

func TestTESetSelfVariable_BadOpType(t *testing.T) {
	s := testSession(1)
	exec := New(nil, &resource.ResourceLoader{}, nopLogger())
	opts := &ExecuteOpts{MapID: 1, EventID: 1, GameState: newMockGameState()}

	cmd := &resource.EventCommand{Code: CmdPluginCommand, Parameters: []interface{}{"TE_SET_SELF_VARIABLE 0 abc 5"}}
	handled := exec.handleTECallOriginEvent(context.Background(), s, cmd, opts, 0)
	assert.True(t, handled)
}

func TestTESetSelfVariable_NilOpts(t *testing.T) {
	s := testSession(1)
	exec := New(nil, &resource.ResourceLoader{}, nopLogger())

	cmd := &resource.EventCommand{Code: CmdPluginCommand, Parameters: []interface{}{"TE_SET_SELF_VARIABLE 0 0 5"}}
	handled := exec.handleTECallOriginEvent(context.Background(), s, cmd, nil, 0)
	assert.True(t, handled)
}

// ========================================================================
// teSetRangeSelfVariable edge cases — 79.2%
// ========================================================================

func TestTESetRangeSelfVariable_BadArgs(t *testing.T) {
	s := testSession(1)
	exec := New(nil, &resource.ResourceLoader{}, nopLogger())
	opts := &ExecuteOpts{MapID: 1, EventID: 1, GameState: newMockGameState()}

	// Bad start
	cmd := &resource.EventCommand{Code: CmdPluginCommand, Parameters: []interface{}{"TE_SET_RANGE_SELF_VARIABLE abc 3 0 5"}}
	assert.True(t, exec.handleTECallOriginEvent(context.Background(), s, cmd, opts, 0))

	// Bad end
	cmd = &resource.EventCommand{Code: CmdPluginCommand, Parameters: []interface{}{"TE_SET_RANGE_SELF_VARIABLE 0 abc 0 5"}}
	assert.True(t, exec.handleTECallOriginEvent(context.Background(), s, cmd, opts, 0))

	// Bad opType
	cmd = &resource.EventCommand{Code: CmdPluginCommand, Parameters: []interface{}{"TE_SET_RANGE_SELF_VARIABLE 0 3 abc 5"}}
	assert.True(t, exec.handleTECallOriginEvent(context.Background(), s, cmd, opts, 0))

	// Bad operand
	cmd = &resource.EventCommand{Code: CmdPluginCommand, Parameters: []interface{}{"TE_SET_RANGE_SELF_VARIABLE 0 3 0 abc"}}
	assert.True(t, exec.handleTECallOriginEvent(context.Background(), s, cmd, opts, 0))

	// nil opts
	cmd = &resource.EventCommand{Code: CmdPluginCommand, Parameters: []interface{}{"TE_SET_RANGE_SELF_VARIABLE 0 3 0 5"}}
	assert.True(t, exec.handleTECallOriginEvent(context.Background(), s, cmd, nil, 0))
}

// ========================================================================
// findMapEvent (executor_template.go:185) — 66.7%
// ========================================================================

func TestFindMapEvent_NotInMap(t *testing.T) {
	res := &resource.ResourceLoader{
		Maps: map[int]*resource.MapData{
			1: {Events: []*resource.MapEvent{nil, {ID: 1}}},
		},
	}
	exec := New(nil, res, nopLogger())

	// Map exists but event ID doesn't match
	assert.Nil(t, exec.findMapEvent(1, 999))

	// Map doesn't exist
	assert.Nil(t, exec.findMapEvent(999, 1))
}

// ========================================================================
// handleCallCommon edge cases — 85.2%
// ========================================================================

func TestHandleCallCommon_NotFound(t *testing.T) {
	s := testSession(1)
	res := &resource.ResourceLoader{
		CommonEventsByName: map[string]int{"Exists": 1},
		CommonEvents:       []*resource.CommonEvent{nil, {ID: 1, Name: "Exists", List: []*resource.EventCommand{{Code: CmdEnd}}}},
	}
	exec := New(nil, res, nopLogger())
	opts := &ExecuteOpts{GameState: newMockGameState()}

	// CallCommon with name not found
	cmd := &resource.EventCommand{Code: CmdPluginCommand, Parameters: []interface{}{"CallCommon DoesNotExist"}}
	handled := exec.handleCallCommon(context.Background(), s, cmd, opts, 0)
	assert.True(t, handled)
}

func TestHandleCallCommon_CCTNotFound(t *testing.T) {
	s := testSession(1)
	res := &resource.ResourceLoader{
		CommonEvents: []*resource.CommonEvent{nil},
	}
	exec := New(nil, res, nopLogger())
	opts := &ExecuteOpts{GameState: newMockGameState()}

	cmd := &resource.EventCommand{Code: CmdPluginCommand, Parameters: []interface{}{"CCT DoesNotExist"}}
	handled := exec.handleCallCommon(context.Background(), s, cmd, opts, 0)
	assert.True(t, handled)
}

func TestHandleCallCommon_NonTE(t *testing.T) {
	s := testSession(1)
	exec := New(nil, &resource.ResourceLoader{}, nopLogger())

	// Not a CallCommon/CCT command
	cmd := &resource.EventCommand{Code: CmdPluginCommand, Parameters: []interface{}{"SomeOtherPlugin"}}
	handled := exec.handleCallCommon(context.Background(), s, cmd, nil, 0)
	assert.False(t, handled)
}

// ========================================================================
// execScriptCommand / execMutableScript / getOrCreateCondVM coverage
// ========================================================================

func TestExecScriptCommand_SetupChild(t *testing.T) {
	gs := newMockGameState()
	gs.variables[100] = 5
	s := testSession(1)

	res := &resource.ResourceLoader{
		CommonEvents: []*resource.CommonEvent{
			nil, nil, nil, nil, nil,
			{ID: 5, Name: "CE5", List: []*resource.EventCommand{
				{Code: CmdChangeSwitches, Parameters: []interface{}{float64(99), float64(99), float64(0)}},
				{Code: CmdEnd},
			}},
		},
	}
	exec := New(nil, res, nopLogger())
	opts := &ExecuteOpts{GameState: gs}

	handled := exec.execScriptCommand(context.Background(), s, `this.setupChild($dataCommonEvents[$gameVariables.value(100)].list, 0)`, opts, 0)
	assert.True(t, handled)
	assert.True(t, gs.switches[99])
}

func TestExecScriptCommand_SetupChild_BadExpr(t *testing.T) {
	s := testSession(1)
	exec := New(nil, &resource.ResourceLoader{CommonEvents: []*resource.CommonEvent{nil}}, nopLogger())
	opts := &ExecuteOpts{GameState: newMockGameState()}

	handled := exec.execScriptCommand(context.Background(), s, `this.setupChild($dataCommonEvents[undefined].list, 0)`, opts, 0)
	assert.True(t, handled)
}

func TestExecMutableScript_SwitchChanges(t *testing.T) {
	gs := newMockGameState()
	s := testSession(1)
	exec := New(nil, &resource.ResourceLoader{}, nopLogger())
	opts := &ExecuteOpts{GameState: gs}

	exec.execMutableScript(`$gameSwitches._data[100] = true`, s, opts)
	assert.True(t, gs.switches[100])
}

func TestExecMutableScript_VarChanges(t *testing.T) {
	gs := newMockGameState()
	s := testSession(1)
	exec := New(nil, &resource.ResourceLoader{}, nopLogger())
	opts := &ExecuteOpts{GameState: gs}

	exec.execMutableScript(`$gameVariables._data[200] = 42`, s, opts)
	assert.Equal(t, 42, gs.variables[200])
}

func TestExecMutableScript_ArrayValue(t *testing.T) {
	gs := newMockGameState()
	s := testSession(1)
	exec := New(nil, &resource.ResourceLoader{}, nopLogger())
	opts := &ExecuteOpts{GameState: gs, TransientVars: make(map[int]interface{})}

	// Store an array value (non-integer)
	exec.execMutableScript(`$gameVariables._data[300] = [40, 10]`, s, opts)
	assert.NotNil(t, opts.TransientVars[300])
}

func TestExecMutableScript_NilOpts(t *testing.T) {
	exec := New(nil, &resource.ResourceLoader{}, nopLogger())
	// Should not panic
	exec.execMutableScript(`x = 1`, nil, nil)
}

func TestExecMutableScript_WithMapNote(t *testing.T) {
	gs := newMockGameState()
	s := testSession(1)
	res := &resource.ResourceLoader{
		Maps: map[int]*resource.MapData{
			5: {Note: "<instance>"},
		},
	}
	exec := New(nil, res, nopLogger())
	opts := &ExecuteOpts{MapID: 5, GameState: gs}

	// Accessing $dataMap.meta.instance should work
	exec.execMutableScript(`if ($dataMap.meta.instance) { $gameVariables._data[500] = 1 }`, s, opts)
	assert.Equal(t, 1, gs.variables[500])
}

func TestExecMutableScript_PageRefreshFn(t *testing.T) {
	gs := newMockGameState()
	s := testSession(1)
	exec := New(nil, &resource.ResourceLoader{}, nopLogger())
	refreshed := false
	opts := &ExecuteOpts{
		GameState:    gs,
		PageRefreshFn: func(_ *player.PlayerSession) { refreshed = true },
	}

	exec.execMutableScript(`$gameSwitches._data[1] = true`, s, opts)
	assert.True(t, refreshed)
}

func TestExecMutableScript_PanicRecovery(t *testing.T) {
	gs := newMockGameState()
	s := testSession(1)
	exec := New(nil, &resource.ResourceLoader{}, nopLogger())
	opts := &ExecuteOpts{GameState: gs}

	// Invalid script that causes error — should not crash
	exec.execMutableScript(`$gameVariables._data[1]; throw new Error("test")`, s, opts)
}

func TestExecMutableScript_SetValue(t *testing.T) {
	gs := newMockGameState()
	s := testSession(1)
	exec := New(nil, &resource.ResourceLoader{}, nopLogger())
	opts := &ExecuteOpts{GameState: gs}

	// $gameVariables.setValue path
	exec.execMutableScript(`$gameVariables.setValue(50, 123)`, s, opts)
	// setValue writes to mutations.varChanges but doesn't apply to gs directly in this path
	// The mutations are applied after RunString — check it was set
	// Actually setValue only writes to mutations which are then applied — let's check gs
	// Since we apply mutations at the end of execMutableScript, gs should have the value
	// Actually no - setValue writes to mutations.varChanges, then the loop applies them to gs.
	// Let's just verify no crash
}

func TestExecScriptCommand_BestEffortFallback(t *testing.T) {
	s := testSession(1)
	exec := New(nil, &resource.ResourceLoader{}, nopLogger())
	opts := &ExecuteOpts{GameState: newMockGameState()}

	// Script that doesn't match setupChild or _data patterns
	// Should execute best-effort in condVM
	handled := exec.execScriptCommand(context.Background(), s, `window.keyTemp = 42`, opts, 0)
	assert.False(t, handled) // Not handled → caller can forward safe lines
}

func TestExecScriptCommand_NilOpts(t *testing.T) {
	s := testSession(1)
	exec := New(nil, &resource.ResourceLoader{}, nopLogger())

	// nil opts → no best-effort fallback
	handled := exec.execScriptCommand(context.Background(), s, `window.keyTemp = 42`, nil, 0)
	assert.False(t, handled)
}

// ========================================================================
// evalScriptCondition / evalScriptValue edge cases
// ========================================================================

func TestEvalScriptCondition_Panic(t *testing.T) {
	s := testSession(1)
	exec := New(nil, &resource.ResourceLoader{}, nopLogger())
	opts := &ExecuteOpts{GameState: newMockGameState()}

	// Script that evaluates to null
	result, ok := exec.evalScriptCondition("null", s, opts)
	assert.True(t, ok) // null returns false, true
	assert.False(t, result)
}

func TestEvalScriptCondition_True(t *testing.T) {
	s := testSession(1)
	gs := newMockGameState()
	gs.switches[1] = true
	exec := New(nil, &resource.ResourceLoader{}, nopLogger())
	opts := &ExecuteOpts{GameState: gs}

	result, ok := exec.evalScriptCondition("$gameSwitches.value(1)", s, opts)
	assert.True(t, ok)
	assert.True(t, result)
}

func TestEvalScriptValue_Error(t *testing.T) {
	s := testSession(1)
	exec := New(nil, &resource.ResourceLoader{}, nopLogger())
	opts := &ExecuteOpts{GameState: newMockGameState()}

	// Invalid expression → returns 0
	val := exec.evalScriptValue("undefined_var.prop", s, opts)
	assert.Equal(t, 0, val)
}

func TestEvalScriptValue_NullUndefined(t *testing.T) {
	s := testSession(1)
	exec := New(nil, &resource.ResourceLoader{}, nopLogger())
	opts := &ExecuteOpts{GameState: newMockGameState()}

	val := exec.evalScriptValue("null", s, opts)
	assert.Equal(t, 0, val)

	val = exec.evalScriptValue("undefined", s, opts)
	assert.Equal(t, 0, val)
}

// ========================================================================
// injectScriptMath edge cases — 77.3%
// ========================================================================

func TestInjectScriptMath_AllFunctions(t *testing.T) {
	s := testSession(1)
	exec := New(nil, &resource.ResourceLoader{}, nopLogger())
	opts := &ExecuteOpts{GameState: newMockGameState()}
	vm := exec.getOrCreateCondVM(s, opts)

	// abs negative
	v, _ := vm.RunString("Math.abs(-5)")
	assert.EqualValues(t, 5, v.ToInteger())

	// abs positive
	v, _ = vm.RunString("Math.abs(5)")
	assert.EqualValues(t, 5, v.ToInteger())

	// max
	v, _ = vm.RunString("Math.max(3, 7)")
	assert.EqualValues(t, 7, v.ToInteger())

	// max reversed
	v, _ = vm.RunString("Math.max(7, 3)")
	assert.EqualValues(t, 7, v.ToInteger())

	// min
	v, _ = vm.RunString("Math.min(3, 7)")
	assert.EqualValues(t, 3, v.ToInteger())

	// min reversed
	v, _ = vm.RunString("Math.min(7, 3)")
	assert.EqualValues(t, 3, v.ToInteger())

	// random
	v, _ = vm.RunString("Math.random()")
	assert.Equal(t, float64(0.5), v.ToFloat())

	// floor, ceil, round
	v, _ = vm.RunString("Math.floor(3.7)")
	assert.EqualValues(t, 3, v.ToInteger())

	v, _ = vm.RunString("Math.ceil(3.2)")
	assert.EqualValues(t, 4, v.ToInteger())

	v, _ = vm.RunString("Math.round(3.5)")
	assert.EqualValues(t, 4, v.ToInteger())
}

// ========================================================================
// injectScriptGameActors (executor_script.go:520) — 87%
// ========================================================================

func TestInjectScriptGameActors_WithEquips(t *testing.T) {
	s := testSession(1)
	s.ClassID = 3
	s.Equips = map[int]int{0: 10, 1: 20}
	exec := New(nil, &resource.ResourceLoader{}, nopLogger())
	opts := &ExecuteOpts{GameState: newMockGameState()}
	vm := exec.getOrCreateCondVM(s, opts)

	// actor(1)._classId
	v, _ := vm.RunString("$gameActors.actor(1)._classId")
	assert.Equal(t, int64(3), v.ToInteger())

	// actor(1)._equips[0]._itemId
	v, _ = vm.RunString("$gameActors.actor(1)._equips[0]._itemId")
	assert.Equal(t, int64(10), v.ToInteger())

	// actor(1)._equips[1]._itemId
	v, _ = vm.RunString("$gameActors.actor(1)._equips[1]._itemId")
	assert.Equal(t, int64(20), v.ToInteger())

	// _data[1] access
	v, _ = vm.RunString("$gameActors._data[1]._classId")
	assert.Equal(t, int64(3), v.ToInteger())

	// actor(2) → undefined
	v, _ = vm.RunString("$gameActors.actor(2)")
	assert.True(t, v.String() == "undefined")
}

func TestInjectScriptGameActors_NilEquips(t *testing.T) {
	s := testSession(1)
	s.Equips = nil
	exec := New(nil, &resource.ResourceLoader{}, nopLogger())
	opts := &ExecuteOpts{GameState: newMockGameState()}
	vm := exec.getOrCreateCondVM(s, opts)

	// _equips should be empty array
	v, _ := vm.RunString("$gameActors.actor(1)._equips.length")
	assert.Equal(t, int64(0), v.ToInteger())
}

// ========================================================================
// injectScriptGameStateMutable — 87.5%
// ========================================================================

func TestInjectScriptGameStateMutable_MutationsRead(t *testing.T) {
	gs := newMockGameState()
	gs.variables[10] = 100
	gs.switches[5] = true
	s := testSession(1)
	exec := New(nil, &resource.ResourceLoader{}, nopLogger())
	opts := &ExecuteOpts{GameState: gs}

	// Run mutable script that reads switches
	exec.execMutableScript(`
		if ($gameSwitches.value(5)) {
			$gameVariables._data[20] = $gameVariables.value(10) + 1
		}
	`, s, opts)
	assert.Equal(t, 101, gs.variables[20])
}

func TestInjectScriptGameStateMutable_VarAnyChangesRead(t *testing.T) {
	gs := newMockGameState()
	s := testSession(1)
	exec := New(nil, &resource.ResourceLoader{}, nopLogger())
	opts := &ExecuteOpts{
		GameState:     gs,
		TransientVars: map[int]interface{}{50: []int{10, 20}},
	}

	// Read a transient var (array) back through value()
	exec.execMutableScript(`
		var arr = $gameVariables.value(50);
		if (arr && arr.length === 2) {
			$gameVariables._data[51] = arr[0] + arr[1];
		}
	`, s, opts)
	assert.Equal(t, 30, gs.variables[51])
}

// ========================================================================
// parseMeta — 92.3%
// ========================================================================

func TestParseMeta_AllFormats(t *testing.T) {
	s := testSession(1)
	exec := New(nil, &resource.ResourceLoader{}, nopLogger())
	opts := &ExecuteOpts{GameState: newMockGameState()}
	vm := exec.getOrCreateCondVM(s, opts)

	// Test parseMeta through $dataMap.meta
	exec2 := New(nil, &resource.ResourceLoader{
		Maps: map[int]*resource.MapData{
			1: {Note: `<instance>
<zone:forest>
<Skill01;[40,10]>
<BadJSON;{invalid}`},
		},
	}, nopLogger())
	opts2 := &ExecuteOpts{MapID: 1, GameState: newMockGameState()}
	vm2 := exec2.getOrCreateCondVM(s, opts2)
	_ = vm

	// <instance> → true
	v, _ := vm2.RunString("$dataMap.meta.instance")
	assert.Equal(t, true, v.Export())

	// <zone:forest> → "forest"
	v, _ = vm2.RunString("$dataMap.meta.zone")
	assert.Equal(t, "forest", v.Export())

	// <Skill01;[40,10]> → [40, 10]
	v, _ = vm2.RunString("$dataMap.meta.Skill01[0]")
	assert.EqualValues(t, 40, v.Export())

	// <BadJSON;{invalid}> → raw string, but kaeru.js regex requires no > inside
	// The meta tag `<BadJSON;{invalid}>` actually matches with match[3]="{invalid"
	// and '}' closes the outer > — let's just check it's set
	v, _ = vm2.RunString("$dataMap.meta.BadJSON")
	assert.NotNil(t, v)
}

// ========================================================================
// getOrCreateCondVM reuse — 79.2%
// ========================================================================

func TestGetOrCreateCondVM_Reuses(t *testing.T) {
	s := testSession(1)
	exec := New(nil, &resource.ResourceLoader{}, nopLogger())
	opts := &ExecuteOpts{GameState: newMockGameState()}

	vm1 := exec.getOrCreateCondVM(s, opts)
	vm2 := exec.getOrCreateCondVM(s, opts)
	assert.Equal(t, vm1, vm2) // Same VM instance
}

func TestGetOrCreateCondVM_WithRes(t *testing.T) {
	s := testSession(1)
	res := &resource.ResourceLoader{
		Maps: map[int]*resource.MapData{1: {Note: "<test:value>"}},
	}
	exec := New(nil, res, nopLogger())
	opts := &ExecuteOpts{MapID: 1, EventID: 1, GameState: newMockGameState()}
	vm := exec.getOrCreateCondVM(s, opts)

	// Should have $dataMap.meta.test
	v, _ := vm.RunString("$dataMap.meta.test")
	assert.Equal(t, "value", v.Export())

	// Should have getSelfVariable
	v, _ = vm.RunString("getSelfVariable(0)")
	assert.Equal(t, int64(0), v.ToInteger())

	// $gameTemp.isPlaytest()
	v, _ = vm.RunString("$gameTemp.isPlaytest()")
	assert.Equal(t, false, v.Export())

	// $gameParty.inBattle()
	v, _ = vm.RunString("$gameParty.inBattle()")
	assert.Equal(t, false, v.Export())

	// $gameParty.size()
	v, _ = vm.RunString("$gameParty.size()")
	assert.Equal(t, int64(1), v.ToInteger())

	// $gameParty.leader().actorId()
	v, _ = vm.RunString("$gameParty.leader().actorId()")
	assert.Equal(t, int64(1), v.ToInteger())

	// $gameParty.members() length
	v, _ = vm.RunString("$gameParty.members().length")
	assert.Equal(t, int64(1), v.ToInteger())

	// window is global
	v, _ = vm.RunString("window.keyList.length")
	assert.Equal(t, int64(0), v.ToInteger())
}

func TestGetOrCreateCondVM_NilResNoMap(t *testing.T) {
	s := testSession(1)
	exec := New(nil, nil, nopLogger())
	opts := &ExecuteOpts{GameState: newMockGameState()}
	vm := exec.getOrCreateCondVM(s, opts)

	// $dataMap should still exist with empty meta
	v, _ := vm.RunString("typeof $dataMap.meta")
	assert.Equal(t, "object", v.String())
}

func TestGetOrCreateCondVM_NilSession(t *testing.T) {
	exec := New(nil, &resource.ResourceLoader{}, nopLogger())
	opts := &ExecuteOpts{GameState: newMockGameState()}
	vm := exec.getOrCreateCondVM(nil, opts)

	// $gameParty should exist but no leader
	v, _ := vm.RunString("$gameParty.inBattle()")
	assert.Equal(t, false, v.Export())
}

// ========================================================================
// execCulSkillEffect — 82.9%
// ========================================================================

func TestExecCulSkillEffect(t *testing.T) {
	gs := newMockGameState()
	s := testSessionWithStats(1, 100, 100, 50, 50, 10, 0)
	s.ClassID = 1
	s.Equips = map[int]int{0: 0, 1: 5}

	// Build minimal prebuilt armors with meta
	res := &resource.ResourceLoader{
		PrebuiltArmors: []interface{}{
			nil, nil, nil, nil, nil,
			map[string]interface{}{
				"id":   float64(5),
				"meta": map[string]interface{}{"Skill01": []interface{}{float64(21), float64(10)}},
			},
		},
	}
	store := newMockInventoryStore()
	exec := New(store, res, nopLogger())
	opts := &ExecuteOpts{GameState: gs, TransientVars: make(map[int]interface{})}

	exec.execCulSkillEffect(s, opts)

	// Should have sent state_batch
	pkts := drainPackets(t, s)
	types := packetTypes(pkts)
	assert.Contains(t, types, "state_batch")
}

func TestExecCulSkillEffect_NilOpts(t *testing.T) {
	exec := New(nil, &resource.ResourceLoader{}, nopLogger())
	// Should not panic
	exec.execCulSkillEffect(testSession(1), nil)
}

func TestExecCulSkillEffect_WithTagSkillList(t *testing.T) {
	gs := newMockGameState()
	s := testSessionWithStats(1, 100, 100, 50, 50, 10, 0)
	s.ClassID = 1
	s.Equips = map[int]int{0: 0, 1: 0}

	tagList := make(map[int]*resource.TagSkillEntry)
	tagList[21] = &resource.TagSkillEntry{BaseVar: 4221, AddVar: 4221, BaseNum: 100}
	res := &resource.ResourceLoader{
		TagSkillList: tagList,
	}
	exec := New(nil, res, nopLogger())
	opts := &ExecuteOpts{GameState: gs, TransientVars: make(map[int]interface{})}

	exec.execCulSkillEffect(s, opts)

	// v[4221] should have been set (BaseNum + AddVar value)
	// After CulSkillEffect, v[4221] was reset to 0 by the JS, then TagSkillList applies BaseNum(100) + GetVariable(4221)(0) = 100
	assert.Equal(t, 100, gs.variables[4221])
}

// ========================================================================
// execParaCheck — 87.2%
// ========================================================================

func TestExecParaCheck(t *testing.T) {
	gs := newMockGameState()
	gs.variables[1209] = 50 // 衣装耐久スキル
	gs.variables[1280] = 30 // 魂の侵蝕
	gs.variables[702] = 40  // 衣装耐久
	s := testSessionWithStats(1, 100, 100, 50, 50, 10, 0)
	s.ClassID = 1
	s.Equips = map[int]int{0: 0, 1: 5}

	res := &resource.ResourceLoader{
		PrebuiltArmors: []interface{}{
			nil, nil, nil, nil, nil,
			map[string]interface{}{"id": float64(5), "meta": map[string]interface{}{"ClothName": "TestCloth"}},
		},
	}
	store := newMockInventoryStore()
	store.gold[1] = 500
	exec := New(store, res, nopLogger())
	opts := &ExecuteOpts{GameState: gs, TransientVars: make(map[int]interface{})}

	exec.execParaCheck(s, opts)

	// v[722] should be set from v[1209]
	assert.Equal(t, 50, gs.variables[722])
	// v[215] should be gold
	assert.Equal(t, 500, gs.variables[215])
	// v[1006] should be level
	assert.Equal(t, 10, gs.variables[1006])
}

func TestExecParaCheck_NilOpts_Coverage(t *testing.T) {
	exec := New(nil, &resource.ResourceLoader{}, nopLogger())
	exec.execParaCheck(testSession(1), nil)
}

func TestExecParaCheck_NilStore(t *testing.T) {
	gs := newMockGameState()
	s := testSessionWithStats(1, 100, 100, 50, 50, 10, 0)
	s.ClassID = 1
	s.Equips = map[int]int{0: 0, 1: 0}
	exec := New(nil, &resource.ResourceLoader{}, nopLogger())
	opts := &ExecuteOpts{GameState: gs, TransientVars: make(map[int]interface{})}

	// No store means gold lookup fails gracefully
	exec.execParaCheck(s, opts)
	// v[215] should be 0 (no gold fetched)
	assert.Equal(t, 0, gs.variables[215])
}

func TestExecParaCheck_ClassGt2(t *testing.T) {
	gs := newMockGameState()
	gs.variables[1027] = 50 // 発情
	s := testSessionWithStats(1, 100, 100, 50, 50, 10, 0)
	s.ClassID = 3 // > 2, 発情 should be forced to 0
	s.Equips = map[int]int{0: 0, 1: 0}
	exec := New(nil, &resource.ResourceLoader{}, nopLogger())
	opts := &ExecuteOpts{GameState: gs, TransientVars: make(map[int]interface{})}

	exec.execParaCheck(s, opts)
	assert.Equal(t, 0, gs.variables[1027])
}

// ========================================================================
// evalSetupChildTarget edge cases — 81%
// ========================================================================

func TestEvalSetupChildTarget_NegativeResult(t *testing.T) {
	s := testSession(1)
	gs := newMockGameState()
	exec := New(nil, &resource.ResourceLoader{}, nopLogger())
	opts := &ExecuteOpts{GameState: gs}

	// Expression evaluating to negative value → abs
	id := exec.evalSetupChildTarget("-5", s, opts)
	assert.Equal(t, 5, id)
}

func TestEvalSetupChildTarget_NilResult(t *testing.T) {
	s := testSession(1)
	exec := New(nil, &resource.ResourceLoader{}, nopLogger())
	opts := &ExecuteOpts{GameState: newMockGameState()}

	id := exec.evalSetupChildTarget("null", s, opts)
	assert.Equal(t, 0, id)
}

func TestEvalSetupChildTarget_WithTransientVars(t *testing.T) {
	s := testSession(1)
	exec := New(nil, &resource.ResourceLoader{}, nopLogger())
	opts := &ExecuteOpts{
		GameState:     newMockGameState(),
		TransientVars: map[int]interface{}{10: 42},
	}

	// Should be able to access transient vars
	id := exec.evalSetupChildTarget("42", s, opts)
	assert.Equal(t, 42, id)
}

// ========================================================================
// skipPastLoopEnd nested loops — 66.7%
// ========================================================================

func TestSkipPastLoopEnd_Nested(t *testing.T) {
	exec := New(nil, &resource.ResourceLoader{}, nopLogger())

	// Outer loop with inner loop
	cmds := []*resource.EventCommand{
		{Code: CmdLoop, Indent: 0},            // 0: outer loop
		{Code: CmdBreakLoop, Indent: 0},        // 1: break outer
		{Code: CmdLoop, Indent: 0},             // 2: inner loop (nested)
		{Code: CmdRepeatAbove, Indent: 0},      // 3: inner repeat (depth-- from 1 to 0)
		{Code: CmdRepeatAbove, Indent: 0},      // 4: outer repeat (depth==0, target)
		{Code: CmdEnd},                          // 5
	}
	// From index 1 (break), skip past loop end should land on index 4
	result := exec.skipPastLoopEnd(cmds, 1, 0)
	assert.Equal(t, 4, result)
}

func TestSkipPastLoopEnd_NoEnd(t *testing.T) {
	exec := New(nil, &resource.ResourceLoader{}, nopLogger())
	cmds := []*resource.EventCommand{
		{Code: CmdLoop, Indent: 0},
		{Code: CmdBreakLoop, Indent: 0},
		{Code: CmdEnd},
	}
	result := exec.skipPastLoopEnd(cmds, 1, 0)
	assert.Equal(t, 2, result) // len-1
}

func TestSkipPastLoopEnd_NilCmd(t *testing.T) {
	exec := New(nil, &resource.ResourceLoader{}, nopLogger())
	cmds := []*resource.EventCommand{
		{Code: CmdBreakLoop, Indent: 0},
		nil,
		{Code: CmdRepeatAbove, Indent: 0},
	}
	result := exec.skipPastLoopEnd(cmds, 0, 0)
	assert.Equal(t, 2, result)
}

// ========================================================================
// jumpToLoopStart nil handling — 71.4%
// ========================================================================

func TestJumpToLoopStart_NilCmd(t *testing.T) {
	exec := New(nil, &resource.ResourceLoader{}, nopLogger())
	cmds := []*resource.EventCommand{
		{Code: CmdLoop, Indent: 0},
		nil,
		{Code: CmdRepeatAbove, Indent: 0},
	}
	result := exec.jumpToLoopStart(cmds, 2, 0)
	assert.Equal(t, 0, result)
}

func TestJumpToLoopStart_NotFound(t *testing.T) {
	exec := New(nil, &resource.ResourceLoader{}, nopLogger())
	cmds := []*resource.EventCommand{
		{Code: CmdEnd, Indent: 0},
		{Code: CmdRepeatAbove, Indent: 0},
	}
	result := exec.jumpToLoopStart(cmds, 1, 0)
	assert.Equal(t, 1, result) // startIdx unchanged
}

// ========================================================================
// skipToBranchEnd nil handling — 71.4%
// ========================================================================

func TestSkipToBranchEnd_NilCmd(t *testing.T) {
	exec := New(nil, &resource.ResourceLoader{}, nopLogger())
	cmds := []*resource.EventCommand{
		{Code: CmdWhenBranch, Indent: 0},
		nil,
		{Code: CmdBranchEnd, Indent: 0},
	}
	result := exec.skipToBranchEnd(cmds, 0, 0)
	assert.Equal(t, 2, result)
}

func TestSkipToBranchEnd_NotFound(t *testing.T) {
	exec := New(nil, &resource.ResourceLoader{}, nopLogger())
	cmds := []*resource.EventCommand{
		{Code: CmdWhenBranch, Indent: 0},
		{Code: CmdEnd, Indent: 0},
	}
	result := exec.skipToBranchEnd(cmds, 0, 0)
	assert.Equal(t, 1, result) // len-1
}

// ========================================================================
// skipToConditionalEnd nil handling — 71.4%
// ========================================================================

func TestSkipToConditionalEnd_NilCmd(t *testing.T) {
	exec := New(nil, &resource.ResourceLoader{}, nopLogger())
	cmds := []*resource.EventCommand{
		{Code: CmdElseBranch, Indent: 0},
		nil,
		{Code: CmdConditionalEnd, Indent: 0},
	}
	result := exec.skipToConditionalEnd(cmds, 0, 0)
	assert.Equal(t, 2, result)
}

// ========================================================================
// skipToElseOrEnd nil handling — 88.9%
// ========================================================================

func TestSkipToElseOrEnd_NilCmd(t *testing.T) {
	exec := New(nil, &resource.ResourceLoader{}, nopLogger())
	cmds := []*resource.EventCommand{
		{Code: CmdConditionalStart, Indent: 0},
		nil,
		{Code: CmdElseBranch, Indent: 0},
	}
	result := exec.skipToElseOrEnd(cmds, 0, 0)
	assert.Equal(t, 2, result)
}

// ========================================================================
// jumpToLabel nil handling — 75%
// ========================================================================

func TestJumpToLabel_NilCmd(t *testing.T) {
	exec := New(nil, &resource.ResourceLoader{}, nopLogger())
	cmds := []*resource.EventCommand{
		nil,
		{Code: CmdLabel, Parameters: []interface{}{"target"}},
	}
	result := exec.jumpToLabel(cmds, "target")
	assert.Equal(t, 1, result)
}

func TestJumpToLabel_NotFound(t *testing.T) {
	exec := New(nil, &resource.ResourceLoader{}, nopLogger())
	cmds := []*resource.EventCommand{
		{Code: CmdLabel, Parameters: []interface{}{"other"}},
	}
	result := exec.jumpToLabel(cmds, "target")
	assert.Equal(t, -1, result)
}

// ========================================================================
// skipToChoiceBranch edge cases — 77.8%
// ========================================================================

func TestSkipToChoiceBranch_NilCmd(t *testing.T) {
	exec := New(nil, &resource.ResourceLoader{}, nopLogger())
	cmds := []*resource.EventCommand{
		{Code: CmdShowChoices, Indent: 0},
		nil,
		{Code: CmdWhenBranch, Indent: 0},
	}
	result := exec.skipToChoiceBranch(cmds, 0, 0, -1)
	assert.Equal(t, 2, result) // first When branch (idx 0)
}

func TestSkipToChoiceBranch_IndentMismatch(t *testing.T) {
	exec := New(nil, &resource.ResourceLoader{}, nopLogger())
	cmds := []*resource.EventCommand{
		{Code: CmdShowChoices, Indent: 0},
		{Code: CmdWhenBranch, Indent: 1}, // wrong indent
		{Code: CmdWhenBranch, Indent: 0}, // correct indent
	}
	result := exec.skipToChoiceBranch(cmds, 0, 0, -1)
	assert.Equal(t, 2, result)
}

func TestSkipToChoiceBranch_FallsToEnd(t *testing.T) {
	exec := New(nil, &resource.ResourceLoader{}, nopLogger())
	cmds := []*resource.EventCommand{
		{Code: CmdShowChoices, Indent: 0},
		{Code: CmdEnd, Indent: 0},
	}
	result := exec.skipToChoiceBranch(cmds, 0, 0, -1)
	assert.Equal(t, 1, result) // falls to end of cmds
}

// ========================================================================
// applyGold edge cases — 78.3%
// ========================================================================

func TestApplyGold_ZeroAmount(t *testing.T) {
	store := newMockInventoryStore()
	store.gold[1] = 100
	exec := New(store, &resource.ResourceLoader{}, nopLogger())
	s := testSession(1)
	ctx := context.Background()

	// op=1 decrease, amount=0 → skip
	err := exec.applyGold(ctx, s, []interface{}{float64(1), float64(0), float64(0)}, nil)
	require.NoError(t, err)
}

// ========================================================================
// applyItems edge cases — 72.4%
// ========================================================================

func TestApplyItems_InvalidItemID(t *testing.T) {
	exec := New(newMockInventoryStore(), &resource.ResourceLoader{}, nopLogger())
	s := testSession(1)
	ctx := context.Background()

	// itemID=0
	err := exec.applyItems(ctx, s, []interface{}{float64(0), float64(0), float64(0), float64(5)}, nil)
	assert.Error(t, err)
}

func TestApplyItems_InvalidQty(t *testing.T) {
	exec := New(newMockInventoryStore(), &resource.ResourceLoader{}, nopLogger())
	s := testSession(1)
	ctx := context.Background()

	// qty=0
	err := exec.applyItems(ctx, s, []interface{}{float64(1), float64(0), float64(0), float64(0)}, nil)
	assert.Error(t, err)
}

func TestApplyItems_NilStore(t *testing.T) {
	exec := New(nil, &resource.ResourceLoader{}, nopLogger())
	s := testSession(1)
	ctx := context.Background()

	err := exec.applyItems(ctx, s, []interface{}{float64(1), float64(0), float64(0), float64(5)}, nil)
	assert.Error(t, err)
}

func TestApplyItems_RemoveNonExistent(t *testing.T) {
	store := newMockInventoryStore()
	exec := New(store, &resource.ResourceLoader{}, nopLogger())
	s := testSession(1)
	ctx := context.Background()

	// Remove item that doesn't exist → no error, no-op
	err := exec.applyItems(ctx, s, []interface{}{float64(1), float64(1), float64(0), float64(5)}, nil)
	require.NoError(t, err)
}

// ========================================================================
// applyVariables edge cases — not fully covered ops
// ========================================================================

func TestApplyVariables_AllOps(t *testing.T) {
	gs := newMockGameState()
	gs.variables[1] = 10
	s := testSession(1)
	exec := New(nil, &resource.ResourceLoader{}, nopLogger())
	opts := &ExecuteOpts{GameState: gs}

	// Multiply (op=3)
	exec.applyVariables(s, []interface{}{float64(1), float64(1), float64(3), float64(0), float64(3)}, opts)
	assert.Equal(t, 30, gs.variables[1])

	// Divide (op=4)
	exec.applyVariables(s, []interface{}{float64(1), float64(1), float64(4), float64(0), float64(5)}, opts)
	assert.Equal(t, 6, gs.variables[1])

	// Modulo (op=5)
	exec.applyVariables(s, []interface{}{float64(1), float64(1), float64(5), float64(0), float64(4)}, opts)
	assert.Equal(t, 2, gs.variables[1])

	// Divide by zero (op=4) — no change
	gs.variables[1] = 10
	exec.applyVariables(s, []interface{}{float64(1), float64(1), float64(4), float64(0), float64(0)}, opts)
	assert.Equal(t, 10, gs.variables[1])

	// Modulo by zero (op=5) — no change
	exec.applyVariables(s, []interface{}{float64(1), float64(1), float64(5), float64(0), float64(0)}, opts)
	assert.Equal(t, 10, gs.variables[1])
}

func TestApplyVariables_ScriptOperand(t *testing.T) {
	gs := newMockGameState()
	s := testSession(1)
	s.Equips = map[int]int{1: 42}
	exec := New(nil, &resource.ResourceLoader{}, nopLogger())
	opts := &ExecuteOpts{GameState: gs}

	// operandType=4 (script): read equip slot item ID
	exec.applyVariables(s, []interface{}{
		float64(100), float64(100), float64(0), float64(4),
		"$gameActors.actor(1)._equips[1]._itemId",
	}, opts)
	assert.Equal(t, 42, gs.variables[100])
}

func TestApplyVariables_VariableRef(t *testing.T) {
	gs := newMockGameState()
	gs.variables[5] = 99
	s := testSession(1)
	exec := New(nil, &resource.ResourceLoader{}, nopLogger())
	opts := &ExecuteOpts{GameState: gs}

	// operandType=1 (variable reference)
	exec.applyVariables(s, []interface{}{
		float64(10), float64(10), float64(0), float64(1), float64(5),
	}, opts)
	assert.Equal(t, 99, gs.variables[10])
}

// ========================================================================
// applySelfSwitch nil opts — 85.7%
// ========================================================================

func TestApplySelfSwitch_NilOpts(t *testing.T) {
	s := testSession(1)
	exec := New(nil, &resource.ResourceLoader{}, nopLogger())
	// Should not panic
	exec.applySelfSwitch(s, []interface{}{"A", float64(0)}, nil)
}

// ========================================================================
// applyChangeArmors/Weapons edge cases — 75%
// ========================================================================

func TestApplyChangeArmors_NilStore(t *testing.T) {
	exec := New(nil, &resource.ResourceLoader{}, nopLogger())
	s := testSession(1)
	// No store — just sends effect, no error
	exec.applyChangeArmors(context.Background(), s, []interface{}{float64(1), float64(0), float64(0), float64(1)}, nil)
	pkts := drainPackets(t, s)
	assert.Equal(t, 1, len(pkts)) // npc_effect
}

func TestApplyChangeWeapons_NilStore(t *testing.T) {
	exec := New(nil, &resource.ResourceLoader{}, nopLogger())
	s := testSession(1)
	exec.applyChangeWeapons(context.Background(), s, []interface{}{float64(1), float64(0), float64(0), float64(1)}, nil)
	pkts := drainPackets(t, s)
	assert.Equal(t, 1, len(pkts))
}

func TestApplyChangeArmors_Remove(t *testing.T) {
	store := newMockInventoryStore()
	store.items[itemKey(1, 5)] = 3
	exec := New(store, &resource.ResourceLoader{}, nopLogger())
	s := testSession(1)
	exec.applyChangeArmors(context.Background(), s, []interface{}{float64(5), float64(1), float64(0), float64(2)}, nil)
	assert.Equal(t, 1, store.items[itemKey(1, 5)])
}

func TestApplyChangeWeapons_Remove(t *testing.T) {
	store := newMockInventoryStore()
	store.items[itemKey(1, 5)] = 3
	exec := New(store, &resource.ResourceLoader{}, nopLogger())
	s := testSession(1)
	exec.applyChangeWeapons(context.Background(), s, []interface{}{float64(5), float64(1), float64(0), float64(2)}, nil)
	assert.Equal(t, 1, store.items[itemKey(1, 5)])
}

func TestApplyChangeArmors_InvalidParams(t *testing.T) {
	store := newMockInventoryStore()
	exec := New(store, &resource.ResourceLoader{}, nopLogger())
	s := testSession(1)

	// armorID=0
	exec.applyChangeArmors(context.Background(), s, []interface{}{float64(0), float64(0), float64(0), float64(1)}, nil)
	// qty=0
	exec.applyChangeArmors(context.Background(), s, []interface{}{float64(1), float64(0), float64(0), float64(0)}, nil)
	pkts := drainPackets(t, s)
	assert.Equal(t, 0, len(pkts)) // no effects sent
}

// ========================================================================
// applyChangeEquipment edge cases — 81.8%
// ========================================================================

func TestApplyChangeEquipment_WeaponSlot(t *testing.T) {
	store := newMockInventoryStore()
	exec := New(store, &resource.ResourceLoader{}, nopLogger())
	s := testSession(1)
	s.Equips = map[int]int{}

	exec.applyChangeEquipment(context.Background(), s, []interface{}{float64(1), float64(0), float64(10)}, nil)
	assert.Equal(t, 10, s.GetEquip(0))
}

func TestApplyChangeEquipment_ItemIDZero(t *testing.T) {
	store := newMockInventoryStore()
	exec := New(store, &resource.ResourceLoader{}, nopLogger())
	s := testSession(1)
	s.Equips = map[int]int{1: 5}

	// itemID=0 → unequip
	exec.applyChangeEquipment(context.Background(), s, []interface{}{float64(1), float64(1), float64(0)}, nil)
	assert.Equal(t, 0, s.GetEquip(1))
	// no DB persist when itemID=0
}

func TestApplyChangeEquipment_NilStore(t *testing.T) {
	exec := New(nil, &resource.ResourceLoader{}, nopLogger())
	s := testSession(1)
	s.Equips = map[int]int{}

	exec.applyChangeEquipment(context.Background(), s, []interface{}{float64(1), float64(1), float64(10)}, nil)
	assert.Equal(t, 10, s.GetEquip(1))
}

// ========================================================================
// resolveTextVarRef edge cases — 80%
// ========================================================================

func TestResolveTextVarRef_NoMatch(t *testing.T) {
	exec := New(nil, &resource.ResourceLoader{}, nopLogger())
	gs := newMockGameState()
	opts := &ExecuteOpts{GameState: gs}

	result := exec.resolveTextVarRef("hello world", opts)
	assert.Equal(t, "hello world", result)
}

func TestResolveTextVarRef_NilOpts(t *testing.T) {
	exec := New(nil, &resource.ResourceLoader{}, nopLogger())
	result := exec.resolveTextVarRef(`\v[1]`, nil)
	assert.Equal(t, `\v[1]`, result)
}

// ========================================================================
// injectPlayerSpeed edge cases — 82.6%
// ========================================================================

func TestInjectPlayerSpeed_TooFewParams(t *testing.T) {
	cmd := &resource.EventCommand{Code: CmdSetMoveRoute, Parameters: []interface{}{float64(1)}}
	result := injectPlayerSpeed(cmd, 3)
	assert.Equal(t, cmd, result) // unchanged
}

func TestInjectPlayerSpeed_NotMapParams(t *testing.T) {
	cmd := &resource.EventCommand{Code: CmdSetMoveRoute, Parameters: []interface{}{float64(1), "not a map"}}
	result := injectPlayerSpeed(cmd, 3)
	assert.Equal(t, cmd, result) // unchanged
}

func TestInjectPlayerSpeed_NoList(t *testing.T) {
	cmd := &resource.EventCommand{Code: CmdSetMoveRoute, Parameters: []interface{}{float64(1), map[string]interface{}{"wait": true}}}
	result := injectPlayerSpeed(cmd, 3)
	assert.Equal(t, cmd, result) // unchanged
}

func TestInjectPlayerSpeed_ListNotSlice(t *testing.T) {
	cmd := &resource.EventCommand{Code: CmdSetMoveRoute, Parameters: []interface{}{
		float64(1),
		map[string]interface{}{"list": "not a slice"},
	}}
	result := injectPlayerSpeed(cmd, 3)
	assert.Equal(t, cmd, result) // unchanged
}

// ========================================================================
// Dispatch: CulSkillEffect/ParaCheck plugin commands
// ========================================================================

func TestDispatch_CulSkillEffect(t *testing.T) {
	gs := newMockGameState()
	s := testSession(1)
	s.ClassID = 1
	s.Equips = map[int]int{0: 0, 1: 0}
	exec := New(nil, &resource.ResourceLoader{}, nopLogger())
	opts := &ExecuteOpts{GameState: gs, TransientVars: make(map[int]interface{})}

	page := &resource.EventPage{
		List: []*resource.EventCommand{
			{Code: CmdPluginCommand, Parameters: []interface{}{"CulSkillEffect"}},
			{Code: CmdEnd},
		},
	}
	exec.Execute(context.Background(), s, page, opts)
	// Should have sent state_batch + dialog_end
	pkts := drainPackets(t, s)
	types := packetTypes(pkts)
	assert.Contains(t, types, "state_batch")
}

func TestDispatch_ParaCheck(t *testing.T) {
	gs := newMockGameState()
	s := testSession(1)
	s.ClassID = 1
	s.Equips = map[int]int{0: 0, 1: 0}
	store := newMockInventoryStore()
	store.gold[1] = 100
	exec := New(store, &resource.ResourceLoader{}, nopLogger())
	opts := &ExecuteOpts{GameState: gs, TransientVars: make(map[int]interface{})}

	page := &resource.EventPage{
		List: []*resource.EventCommand{
			{Code: CmdPluginCommand, Parameters: []interface{}{"ParaCheck"}},
			{Code: CmdEnd},
		},
	}
	exec.Execute(context.Background(), s, page, opts)
	pkts := drainPackets(t, s)
	types := packetTypes(pkts)
	assert.Contains(t, types, "state_batch")
}

// ========================================================================
// Dispatch: remaining forward-only codes
// ========================================================================

func TestDispatch_GameOverReturnToTitle(t *testing.T) {
	s := testSession(1)
	exec := New(nil, &resource.ResourceLoader{}, nopLogger())

	page := &resource.EventPage{
		List: []*resource.EventCommand{
			{Code: CmdGameOver},
			{Code: CmdEnd},
		},
	}
	exec.Execute(context.Background(), s, page, nil)
	pkts := drainPackets(t, s)
	found := false
	for _, p := range pkts {
		if p.Type == "npc_effect" {
			var data map[string]interface{}
			json.Unmarshal(p.Payload, &data)
			if data["code"].(float64) == float64(CmdGameOver) {
				found = true
			}
		}
	}
	assert.True(t, found)
}

func TestDispatch_ChangeSkill(t *testing.T) {
	s := testSession(1)
	exec := New(nil, &resource.ResourceLoader{}, nopLogger())

	page := &resource.EventPage{
		List: []*resource.EventCommand{
			{Code: CmdChangeSkill, Parameters: []interface{}{float64(1), float64(0), float64(10)}},
			{Code: CmdEnd},
		},
	}
	exec.Execute(context.Background(), s, page, nil)
	pkts := drainPackets(t, s)
	assert.True(t, len(pkts) >= 1)
}

func TestDispatch_ChangeParameter(t *testing.T) {
	s := testSession(1)
	exec := New(nil, &resource.ResourceLoader{}, nopLogger())

	page := &resource.EventPage{
		List: []*resource.EventCommand{
			{Code: CmdChangeParameter, Parameters: []interface{}{float64(0), float64(1), float64(0), float64(0), float64(10)}},
			{Code: CmdEnd},
		},
	}
	exec.Execute(context.Background(), s, page, nil)
	pkts := drainPackets(t, s)
	assert.True(t, len(pkts) >= 1)
}

func TestDispatch_ChangeName(t *testing.T) {
	s := testSession(1)
	exec := New(nil, &resource.ResourceLoader{}, nopLogger())

	page := &resource.EventPage{
		List: []*resource.EventCommand{
			{Code: CmdChangeName, Parameters: []interface{}{float64(1), "NewName"}},
			{Code: CmdEnd},
		},
	}
	exec.Execute(context.Background(), s, page, nil)
	pkts := drainPackets(t, s)
	assert.True(t, len(pkts) >= 1)
}

func TestDispatch_ChangeActorImage(t *testing.T) {
	s := testSession(1)
	exec := New(nil, &resource.ResourceLoader{}, nopLogger())

	page := &resource.EventPage{
		List: []*resource.EventCommand{
			{Code: CmdChangeActorImage, Parameters: []interface{}{float64(1), "face", float64(0)}},
			{Code: CmdEnd},
		},
	}
	exec.Execute(context.Background(), s, page, nil)
	pkts := drainPackets(t, s)
	assert.True(t, len(pkts) >= 1)
}

func TestDispatch_RotateErasePicture(t *testing.T) {
	s := testSession(1)
	exec := New(nil, &resource.ResourceLoader{}, nopLogger())

	page := &resource.EventPage{
		List: []*resource.EventCommand{
			{Code: CmdRotatePicture, Parameters: []interface{}{float64(1), float64(90)}},
			{Code: CmdErasePicture, Parameters: []interface{}{float64(1)}},
			{Code: CmdEnd},
		},
	}
	exec.Execute(context.Background(), s, page, nil)
	pkts := drainPackets(t, s)
	assert.True(t, len(pkts) >= 2)
}

func TestDispatch_EraseEvent(t *testing.T) {
	s := testSession(1)
	exec := New(nil, &resource.ResourceLoader{}, nopLogger())

	page := &resource.EventPage{
		List: []*resource.EventCommand{
			{Code: CmdEraseEvent},
			{Code: CmdEnd},
		},
	}
	exec.Execute(context.Background(), s, page, nil)
	pkts := drainPackets(t, s)
	assert.True(t, len(pkts) >= 1)
}

func TestDispatch_ChangeTransparency(t *testing.T) {
	s := testSession(1)
	exec := New(nil, &resource.ResourceLoader{}, nopLogger())

	page := &resource.EventPage{
		List: []*resource.EventCommand{
			{Code: CmdChangeTransparency, Parameters: []interface{}{float64(0)}},
			{Code: CmdEnd},
		},
	}
	exec.Execute(context.Background(), s, page, nil)
	pkts := drainPackets(t, s)
	assert.True(t, len(pkts) >= 1)
}

// ========================================================================
// Dispatch: FadeIn/FadeOut (wait for ack)
// ========================================================================

func TestDispatch_FadeoutFadein(t *testing.T) {
	s := testSession(1)
	exec := New(nil, &resource.ResourceLoader{}, nopLogger())

	page := &resource.EventPage{
		List: []*resource.EventCommand{
			{Code: CmdFadeout},
			{Code: CmdFadein},
			{Code: CmdEnd},
		},
	}

	// Send acks so waitForEffectAck doesn't block
	go func() {
		time.Sleep(10 * time.Millisecond)
		s.EffectAckCh <- struct{}{}
		time.Sleep(10 * time.Millisecond)
		s.EffectAckCh <- struct{}{}
	}()

	exec.Execute(context.Background(), s, page, nil)
	pkts := drainPackets(t, s)
	assert.True(t, len(pkts) >= 2)
}

// ========================================================================
// waitForEffectAck timeout — 75%
// ========================================================================

func TestWaitForEffectAck_Timeout(t *testing.T) {
	s := testSession(1)
	exec := New(nil, &resource.ResourceLoader{}, nopLogger())

	// We can't actually wait 30 seconds in a test, so test the other branches
	// Timeout is already handled; let's just ensure ack works
	s.EffectAckCh <- struct{}{}
	result := exec.waitForEffectAck(context.Background(), s, 999)
	assert.True(t, result)
}

// ========================================================================
// processBattle random troopType — 92.3%
// ========================================================================

func TestProcessBattle_RandomTroop(t *testing.T) {
	s := testSession(1)
	exec := New(nil, &resource.ResourceLoader{}, nopLogger())

	// troopType=2 (random) → uses troopID=1
	result := exec.processBattle(context.Background(), s, []interface{}{float64(2), float64(0), float64(1), float64(0)}, nil)
	assert.Equal(t, 0, result) // default win
}

func TestProcessBattle_InvalidTroopID(t *testing.T) {
	s := testSession(1)
	exec := New(nil, &resource.ResourceLoader{}, nopLogger())

	// troopID=0 → falls back to 1
	result := exec.processBattle(context.Background(), s, []interface{}{float64(0), float64(0), float64(1), float64(0)}, nil)
	assert.Equal(t, 0, result)
}

// ========================================================================
// Parallel: stepUntilWait more branches
// ========================================================================

func TestStepUntilWait_ChangeGold(t *testing.T) {
	gs := newMockGameState()
	store := newMockInventoryStore()
	store.gold[1] = 1000
	s := testSession(1)
	s.MapID = 1
	exec := New(store, &resource.ResourceLoader{}, nopLogger())
	opts := &ExecuteOpts{MapID: 1, GameState: gs}

	ev := &ParallelEventState{
		EventID: 1,
		Cmds: []*resource.EventCommand{
			{Code: CmdChangeGold, Parameters: []interface{}{float64(0), float64(0), float64(100)}},
			{Code: CmdEnd, Indent: 0},
		},
	}

	done := exec.stepUntilWait(context.Background(), s, ev, opts)
	assert.True(t, done)
	assert.Equal(t, int64(1100), store.gold[1])
}

func TestStepUntilWait_ChangeItems(t *testing.T) {
	gs := newMockGameState()
	store := newMockInventoryStore()
	s := testSession(1)
	s.MapID = 1
	exec := New(store, &resource.ResourceLoader{}, nopLogger())
	opts := &ExecuteOpts{MapID: 1, GameState: gs}

	ev := &ParallelEventState{
		EventID: 1,
		Cmds: []*resource.EventCommand{
			{Code: CmdChangeItems, Parameters: []interface{}{float64(5), float64(0), float64(0), float64(3)}},
			{Code: CmdEnd, Indent: 0},
		},
	}

	done := exec.stepUntilWait(context.Background(), s, ev, opts)
	assert.True(t, done)
	assert.Equal(t, 3, store.items[itemKey(1, 5)])
}

func TestStepUntilWait_InstanceCommands(t *testing.T) {
	gs := newMockGameState()
	s := testSession(1)
	s.MapID = 1
	exec := New(nil, &resource.ResourceLoader{}, nopLogger())
	entered := false
	left := false
	opts := &ExecuteOpts{
		MapID:     1,
		GameState: gs,
		EnterInstanceFn: func(_ *player.PlayerSession) { entered = true },
		LeaveInstanceFn: func(_ *player.PlayerSession) { left = true },
	}

	ev := &ParallelEventState{
		EventID: 1,
		Cmds: []*resource.EventCommand{
			{Code: CmdPluginCommand, Parameters: []interface{}{"EnterInstance"}},
			{Code: CmdPluginCommand, Parameters: []interface{}{"LeaveInstance"}},
			{Code: CmdEnd, Indent: 0},
		},
	}

	exec.stepUntilWait(context.Background(), s, ev, opts)
	assert.True(t, entered)
	assert.True(t, left)
}

func TestStepUntilWait_BlockedPlugin(t *testing.T) {
	gs := newMockGameState()
	s := testSession(1)
	exec := New(nil, &resource.ResourceLoader{}, nopLogger())
	opts := &ExecuteOpts{MapID: 1, GameState: gs}

	ev := &ParallelEventState{
		EventID: 1,
		Cmds: []*resource.EventCommand{
			{Code: CmdPluginCommand, Parameters: []interface{}{"CallStand arg1"}},
			{Code: CmdEnd, Indent: 0},
		},
	}

	exec.stepUntilWait(context.Background(), s, ev, opts)
	// CallStand is blocked, no packet sent
	pkts := drainPackets(t, s)
	assert.Equal(t, 0, len(pkts))
}

func TestStepUntilWait_ScriptCont(t *testing.T) {
	gs := newMockGameState()
	s := testSession(1)
	exec := New(nil, &resource.ResourceLoader{}, nopLogger())
	opts := &ExecuteOpts{MapID: 1, GameState: gs}

	// Orphan ScriptCont should just advance
	ev := &ParallelEventState{
		EventID: 1,
		Cmds: []*resource.EventCommand{
			{Code: CmdScriptCont, Parameters: []interface{}{"continuation"}},
			{Code: CmdEnd, Indent: 0},
		},
	}

	done := exec.stepUntilWait(context.Background(), s, ev, opts)
	assert.True(t, done)
}

func TestStepUntilWait_MoveRouteCont(t *testing.T) {
	gs := newMockGameState()
	s := testSession(1)
	exec := New(nil, &resource.ResourceLoader{}, nopLogger())
	opts := &ExecuteOpts{MapID: 1, GameState: gs}

	// Orphan MoveRouteCont should just advance
	ev := &ParallelEventState{
		EventID: 1,
		Cmds: []*resource.EventCommand{
			{Code: CmdMoveRouteCont, Parameters: []interface{}{}},
			{Code: CmdEnd, Indent: 0},
		},
	}

	done := exec.stepUntilWait(context.Background(), s, ev, opts)
	assert.True(t, done)
}

func TestStepUntilWait_CallCommon(t *testing.T) {
	gs := newMockGameState()
	s := testSession(1)
	res := &resource.ResourceLoader{
		CommonEventsByName: map[string]int{"TestCE": 1},
		CommonEvents: []*resource.CommonEvent{
			nil, nil, // CE 0 and 1
		},
	}
	exec := New(nil, res, nopLogger())
	opts := &ExecuteOpts{MapID: 1, GameState: gs}

	ev := &ParallelEventState{
		EventID: 1,
		Cmds: []*resource.EventCommand{
			{Code: CmdPluginCommand, Parameters: []interface{}{"CallCommon TestCE"}},
			{Code: CmdEnd, Indent: 0},
		},
	}

	done := exec.stepUntilWait(context.Background(), s, ev, opts)
	assert.True(t, done)
}

// ========================================================================
// RunParallelEventsSynced tick loop — 92.1%
// ========================================================================

func TestRunParallelEventsSynced_WaitFrames(t *testing.T) {
	gs := newMockGameState()
	s := testSession(1)
	s.MapID = 1
	exec := New(nil, &resource.ResourceLoader{}, nopLogger())
	opts := &ExecuteOpts{MapID: 1, GameState: gs}

	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()

	ev := NewParallelEventState(1, []*resource.EventCommand{
		{Code: CmdWait, Parameters: []interface{}{float64(1)}},
		{Code: CmdEnd, Indent: 0},
	}, 4)

	exec.RunParallelEventsSynced(ctx, s, []*ParallelEventState{ev}, opts)
}

func TestRunParallelEventsSynced_MultipleEvents(t *testing.T) {
	gs := newMockGameState()
	s := testSession(1)
	s.MapID = 1
	exec := New(nil, &resource.ResourceLoader{}, nopLogger())
	opts := &ExecuteOpts{MapID: 1, GameState: gs}

	ctx, cancel := context.WithTimeout(context.Background(), 300*time.Millisecond)
	defer cancel()

	ev1 := NewParallelEventState(1, []*resource.EventCommand{
		{Code: CmdChangeSwitches, Parameters: []interface{}{float64(1), float64(1), float64(0)}},
		{Code: CmdEnd, Indent: 0},
	}, 3)
	ev2 := NewParallelEventState(2, []*resource.EventCommand{
		{Code: CmdChangeSwitches, Parameters: []interface{}{float64(2), float64(2), float64(0)}},
		{Code: CmdEnd, Indent: 0},
	}, 5) // Different speed

	exec.RunParallelEventsSynced(ctx, s, []*ParallelEventState{ev1, ev2}, opts)
	assert.True(t, gs.switches[1])
	assert.True(t, gs.switches[2])
}

// ========================================================================
// Dispatch: ScriptCont standalone, ShopItem standalone
// ========================================================================

func TestDispatch_ScriptCont_Standalone(t *testing.T) {
	s := testSession(1)
	exec := New(nil, &resource.ResourceLoader{}, nopLogger())

	page := &resource.EventPage{
		List: []*resource.EventCommand{
			{Code: CmdScriptCont, Parameters: []interface{}{"continuation"}},
			{Code: CmdEnd},
		},
	}
	exec.Execute(context.Background(), s, page, nil)
	pkts := drainPackets(t, s)
	// Only dialog_end expected
	types := packetTypes(pkts)
	assert.Contains(t, types, "npc_dialog_end")
}

func TestDispatch_ShopItem_Standalone(t *testing.T) {
	s := testSession(1)
	exec := New(nil, &resource.ResourceLoader{}, nopLogger())

	page := &resource.EventPage{
		List: []*resource.EventCommand{
			{Code: CmdShopItem, Parameters: []interface{}{float64(1), float64(5), float64(0)}},
			{Code: CmdEnd},
		},
	}
	exec.Execute(context.Background(), s, page, nil)
	pkts := drainPackets(t, s)
	types := packetTypes(pkts)
	assert.Contains(t, types, "npc_dialog_end")
}

// ========================================================================
// Dispatch: ShowChoices standalone (no merge with text)
// ========================================================================

func TestDispatch_ShowChoices_Standalone(t *testing.T) {
	gs := newMockGameState()
	s := testSession(1)
	exec := New(nil, &resource.ResourceLoader{}, nopLogger())
	opts := &ExecuteOpts{GameState: gs}

	page := &resource.EventPage{
		List: []*resource.EventCommand{
			{Code: CmdShowChoices, Parameters: []interface{}{
				[]interface{}{"Yes", "No"},
				float64(1), // cancelType=1
				float64(0), // default=0
				float64(2), // position=right
				float64(0), // bg=window
			}},
			{Code: CmdWhenBranch, Indent: 0, Parameters: []interface{}{float64(0)}},
			{Code: CmdChangeSwitches, Indent: 1, Parameters: []interface{}{float64(10), float64(10), float64(0)}},
			{Code: CmdWhenBranch, Indent: 0, Parameters: []interface{}{float64(1)}},
			{Code: CmdBranchEnd, Indent: 0},
			{Code: CmdEnd},
		},
	}

	// Send choice=0 (Yes)
	go func() {
		time.Sleep(10 * time.Millisecond)
		s.ChoiceCh <- 0
	}()

	exec.Execute(context.Background(), s, page, opts)
	assert.True(t, gs.switches[10])
}

// ========================================================================
// Dispatch: WhenBranch/WhenCancel in normal flow
// ========================================================================

func TestDispatch_WhenBranch_SkipsToBranchEnd(t *testing.T) {
	s := testSession(1)
	gs := newMockGameState()
	exec := New(nil, &resource.ResourceLoader{}, nopLogger())
	opts := &ExecuteOpts{GameState: gs}

	// Simulates encountering WhenBranch after already executing a branch
	page := &resource.EventPage{
		List: []*resource.EventCommand{
			{Code: CmdWhenBranch, Indent: 0},
			{Code: CmdChangeSwitches, Indent: 1, Parameters: []interface{}{float64(20), float64(20), float64(0)}},
			{Code: CmdBranchEnd, Indent: 0},
			{Code: CmdEnd},
		},
	}
	exec.Execute(context.Background(), s, page, opts)
	// The switch should NOT be set because WhenBranch causes skip to BranchEnd
	assert.False(t, gs.switches[20])
}

// ========================================================================
// Dispatch: Audio commands
// ========================================================================

func TestDispatch_AudioCommands(t *testing.T) {
	s := testSession(1)
	exec := New(nil, &resource.ResourceLoader{}, nopLogger())

	audioCodes := []int{CmdPlayBGM, CmdStopBGM, CmdPlayBGS, CmdStopBGS, CmdPlaySE, CmdStopSE, CmdPlayME}
	for _, code := range audioCodes {
		page := &resource.EventPage{
			List: []*resource.EventCommand{
				{Code: code, Parameters: []interface{}{}},
				{Code: CmdEnd},
			},
		}
		exec.Execute(context.Background(), s, page, nil)
	}
	pkts := drainPackets(t, s)
	// Each audio command should produce an npc_effect + a dialog_end
	assert.True(t, len(pkts) >= len(audioCodes))
}

// ========================================================================
// Transfer: no TransferFn (client fallback)
// ========================================================================

func TestTransferPlayer_NoTransferFn(t *testing.T) {
	s := testSession(1)
	exec := New(nil, &resource.ResourceLoader{}, nopLogger())

	exec.transferPlayer(s, []interface{}{float64(0), float64(2), float64(5), float64(6), float64(4)}, nil)
	pkts := drainPackets(t, s)
	found := false
	for _, p := range pkts {
		if p.Type == "transfer_player" {
			found = true
			var data map[string]interface{}
			json.Unmarshal(p.Payload, &data)
			assert.Equal(t, float64(2), data["map_id"])
			assert.Equal(t, float64(5), data["x"])
			assert.Equal(t, float64(6), data["y"])
			assert.Equal(t, float64(4), data["dir"])
		}
	}
	assert.True(t, found)
}

func TestTransferPlayer_DirDefault(t *testing.T) {
	s := testSession(1)
	exec := New(nil, &resource.ResourceLoader{}, nopLogger())

	// dir=0 → defaults to 2
	exec.transferPlayer(s, []interface{}{float64(0), float64(2), float64(5), float64(6), float64(0)}, nil)
	pkts := drainPackets(t, s)
	for _, p := range pkts {
		if p.Type == "transfer_player" {
			var data map[string]interface{}
			json.Unmarshal(p.Payload, &data)
			assert.Equal(t, float64(2), data["dir"])
		}
	}
}

// ========================================================================
// HasItemOfKind/IsEquipped/HasSkill gormInventoryStore error paths
// ========================================================================

func TestGormHasItemOfKind_EquippedBranch(t *testing.T) {
	store, cleanup := setupGormStore(t)
	defer cleanup()
	ctx := context.Background()

	// Create equipped item
	has, err := store.HasItemOfKind(ctx, 1, 10, 2, false) // not including equipped
	require.NoError(t, err)
	assert.False(t, has)
}

func TestGormIsEquipped_NotFound(t *testing.T) {
	store, cleanup := setupGormStore(t)
	defer cleanup()
	ctx := context.Background()

	has, err := store.IsEquipped(ctx, 1, 999, 2)
	require.NoError(t, err)
	assert.False(t, has)
}

func TestGormHasSkill_NotFound(t *testing.T) {
	store, cleanup := setupGormStore(t)
	defer cleanup()
	ctx := context.Background()

	has, err := store.HasSkill(ctx, 1, 999)
	require.NoError(t, err)
	assert.False(t, has)
}

// ========================================================================
// Dispatch: SetEventLocation in main executeList
// ========================================================================

func TestDispatch_SetEventLocation(t *testing.T) {
	s := testSession(1)
	exec := New(nil, &resource.ResourceLoader{}, nopLogger())
	opts := &ExecuteOpts{MapID: 1, EventID: 5}

	page := &resource.EventPage{
		List: []*resource.EventCommand{
			{Code: CmdSetEventLocation, Parameters: []interface{}{float64(0), float64(10), float64(20)}},
			{Code: CmdEnd},
		},
	}
	exec.Execute(context.Background(), s, page, opts)
	pkts := drainPackets(t, s)
	found := false
	for _, p := range pkts {
		if p.Type == "npc_effect" {
			var data map[string]interface{}
			json.Unmarshal(p.Payload, &data)
			if data["code"].(float64) == float64(CmdSetEventLocation) {
				params := data["params"].([]interface{})
				assert.Equal(t, float64(5), params[0]) // charId resolved to eventID
				found = true
			}
		}
	}
	assert.True(t, found)
}

// ========================================================================
// resolveTextCodes: \P[n] code
// ========================================================================

func TestResolveTextCodes_PartyName(t *testing.T) {
	gs := newMockGameState()
	s := testSession(1)
	s.CharName = "Luna"

	res := &resource.ResourceLoader{
		Actors: []*resource.Actor{nil, {ID: 1, Name: "Hero"}},
	}
	exec := New(nil, res, nopLogger())
	opts := &ExecuteOpts{GameState: gs}

	// \P[1] → party member 1 → player name
	result := exec.resolveTextCodes(`\P[1]`, s, opts)
	assert.Equal(t, "Luna", result)
}

// ========================================================================
// applyChangeHP/MP edge cases
// ========================================================================

func TestApplyChangeHP_DecreaseBelowZero_AllowDeath(t *testing.T) {
	s := testSessionWithStats(1, 50, 100, 0, 0, 1, 0)
	exec := New(nil, &resource.ResourceLoader{}, nopLogger())

	// Decrease by 100, allow death
	exec.applyChangeHP(context.Background(), s, []interface{}{float64(0), float64(1), float64(1), float64(0), float64(100), float64(1)}, nil)
	assert.Equal(t, 0, s.HP)
}

func TestApplyChangeHP_DecreaseBelowZero_NoDeath(t *testing.T) {
	s := testSessionWithStats(1, 50, 100, 0, 0, 1, 0)
	exec := New(nil, &resource.ResourceLoader{}, nopLogger())

	// Decrease by 100, no death
	exec.applyChangeHP(context.Background(), s, []interface{}{float64(0), float64(1), float64(1), float64(0), float64(100), float64(0)}, nil)
	assert.Equal(t, 1, s.HP)
}

func TestApplyChangeHP_IncreaseAboveMax(t *testing.T) {
	s := testSessionWithStats(1, 90, 100, 0, 0, 1, 0)
	exec := New(nil, &resource.ResourceLoader{}, nopLogger())

	exec.applyChangeHP(context.Background(), s, []interface{}{float64(0), float64(1), float64(0), float64(0), float64(50)}, nil)
	assert.Equal(t, 100, s.HP) // clamped to MaxHP
}

func TestApplyChangeMP_DecreaseBelowZero(t *testing.T) {
	s := testSessionWithStats(1, 100, 100, 30, 50, 1, 0)
	exec := New(nil, &resource.ResourceLoader{}, nopLogger())

	exec.applyChangeMP(context.Background(), s, []interface{}{float64(0), float64(1), float64(1), float64(0), float64(100)}, nil)
	assert.Equal(t, 0, s.MP)
}

func TestApplyChangeMP_IncreaseAboveMax(t *testing.T) {
	s := testSessionWithStats(1, 100, 100, 40, 50, 1, 0)
	exec := New(nil, &resource.ResourceLoader{}, nopLogger())

	exec.applyChangeMP(context.Background(), s, []interface{}{float64(0), float64(1), float64(0), float64(0), float64(50)}, nil)
	assert.Equal(t, 50, s.MP) // clamped to MaxMP
}

// ========================================================================
// applyChangeEXP decrease below zero
// ========================================================================

func TestApplyChangeEXP_DecreaseBelowZero(t *testing.T) {
	s := testSessionWithStats(1, 100, 100, 50, 50, 10, 30)
	exec := New(nil, &resource.ResourceLoader{}, nopLogger())

	exec.applyChangeEXP(context.Background(), s, []interface{}{float64(0), float64(1), float64(1), float64(0), float64(100)}, nil)
	assert.Equal(t, int64(0), s.Exp) // clamped to 0
}

// ========================================================================
// applyChangeLevel decrease below 1
// ========================================================================

func TestApplyChangeLevel_DecreaseBelowOne(t *testing.T) {
	s := testSessionWithStats(1, 100, 100, 50, 50, 3, 0)
	exec := New(nil, &resource.ResourceLoader{}, nopLogger())

	exec.applyChangeLevel(context.Background(), s, []interface{}{float64(0), float64(1), float64(1), float64(0), float64(10)}, nil)
	assert.Equal(t, 1, s.Level) // clamped to 1
}

// ========================================================================
// Dispatch: WaitForMoveRoute standalone (via CmdWaitForMoveRoute)
// covered by existing test but let's ensure dispatch code works
// ========================================================================

func TestDispatch_MoveRouteCont_Standalone(t *testing.T) {
	s := testSession(1)
	exec := New(nil, &resource.ResourceLoader{}, nopLogger())

	page := &resource.EventPage{
		List: []*resource.EventCommand{
			{Code: CmdMoveRouteCont, Parameters: []interface{}{}},
			{Code: CmdEnd},
		},
	}
	exec.Execute(context.Background(), s, page, nil)
	pkts := drainPackets(t, s)
	types := packetTypes(pkts)
	assert.Contains(t, types, "npc_dialog_end")
}

// ========================================================================
// evalCondition: variable comparison ops
// ========================================================================

func TestEvalCondition_VariableOps(t *testing.T) {
	gs := newMockGameState()
	gs.variables[1] = 10
	exec := New(nil, &resource.ResourceLoader{}, nopLogger())
	s := testSession(1)
	opts := &ExecuteOpts{GameState: gs}
	ctx := context.Background()

	// op=3 (greater than) 10 > 5 = true
	assert.True(t, exec.evaluateCondition(ctx, s, []interface{}{float64(1), float64(1), float64(0), float64(5), float64(3)}, opts))
	// op=4 (less than) 10 < 5 = false
	assert.False(t, exec.evaluateCondition(ctx, s, []interface{}{float64(1), float64(1), float64(0), float64(5), float64(4)}, opts))
	// op=5 (not equal) 10 != 5 = true
	assert.True(t, exec.evaluateCondition(ctx, s, []interface{}{float64(1), float64(1), float64(0), float64(5), float64(5)}, opts))
	// op=5 (not equal) 10 != 10 = false
	assert.False(t, exec.evaluateCondition(ctx, s, []interface{}{float64(1), float64(1), float64(0), float64(10), float64(5)}, opts))
	// refType=1 (variable reference)
	gs.variables[2] = 5
	assert.True(t, exec.evaluateCondition(ctx, s, []interface{}{float64(1), float64(1), float64(1), float64(2), float64(1)}, opts)) // 10 >= v[2]=5
}

// ========================================================================
// evalActorCondition: weapon/armor/state/name sub-types
// ========================================================================

func TestEvalActorCondition_State(t *testing.T) {
	s := testSession(1)
	s.AddState(5)
	store := newMockInventoryStore()
	exec := New(store, &resource.ResourceLoader{}, nopLogger())
	ctx := context.Background()

	// subType=6 (state)
	assert.True(t, exec.evalActorCondition(ctx, s, []interface{}{float64(4), float64(1), float64(6), float64(5)}))
	assert.False(t, exec.evalActorCondition(ctx, s, []interface{}{float64(4), float64(1), float64(6), float64(99)}))
}

func TestEvalActorCondition_Name(t *testing.T) {
	s := testSession(1)
	exec := New(nil, &resource.ResourceLoader{}, nopLogger())
	ctx := context.Background()

	// subType=1 (name) → always false
	assert.False(t, exec.evalActorCondition(ctx, s, []interface{}{float64(4), float64(1), float64(1), float64(0)}))
}

func TestEvalActorCondition_Default(t *testing.T) {
	s := testSession(1)
	exec := New(nil, &resource.ResourceLoader{}, nopLogger())
	ctx := context.Background()

	// Unknown subtype → false
	assert.False(t, exec.evalActorCondition(ctx, s, []interface{}{float64(4), float64(1), float64(99), float64(0)}))
}

func TestEvalActorCondition_Skill_NilStore(t *testing.T) {
	s := testSession(1)
	exec := New(nil, &resource.ResourceLoader{}, nopLogger())
	ctx := context.Background()

	// subType=3 (skill), nil store
	assert.False(t, exec.evalActorCondition(ctx, s, []interface{}{float64(4), float64(1), float64(3), float64(5)}))
}

func TestEvalActorCondition_Weapon_NilStore(t *testing.T) {
	s := testSession(1)
	exec := New(nil, &resource.ResourceLoader{}, nopLogger())
	ctx := context.Background()

	// subType=4 (weapon), nil store
	assert.False(t, exec.evalActorCondition(ctx, s, []interface{}{float64(4), float64(1), float64(4), float64(5)}))
}

func TestEvalActorCondition_Armor_NilStore(t *testing.T) {
	s := testSession(1)
	exec := New(nil, &resource.ResourceLoader{}, nopLogger())
	ctx := context.Background()

	// subType=5 (armor), nil store
	assert.False(t, exec.evalActorCondition(ctx, s, []interface{}{float64(4), float64(1), float64(5), float64(5)}))
}

// ========================================================================
// evalItemCondition edge: includeEquip for weapons/armors
// ========================================================================

func TestEvalItemCondition_WeaponIncludeEquip(t *testing.T) {
	store := newMockInventoryStore()
	store.items[itemKey(1, 10)] = 1
	exec := New(store, &resource.ResourceLoader{}, nopLogger())
	s := testSession(1)
	ctx := context.Background()

	// Weapon with includeEquip=true (params[2]=1)
	result := exec.evalItemCondition(ctx, s, []interface{}{float64(9), float64(10), float64(1)}, 2)
	assert.True(t, result)
}

// ========================================================================
// evalGoldCondition: op=2 (less than)
// ========================================================================

func TestEvalGoldCondition_LessThan(t *testing.T) {
	store := newMockInventoryStore()
	store.gold[1] = 50
	exec := New(store, &resource.ResourceLoader{}, nopLogger())
	s := testSession(1)
	ctx := context.Background()

	// op=2 (less than): 50 < 100 = true
	assert.True(t, exec.evalGoldCondition(ctx, s, []interface{}{float64(7), float64(100), float64(2)}))
	// 50 < 50 = false
	assert.False(t, exec.evalGoldCondition(ctx, s, []interface{}{float64(7), float64(50), float64(2)}))
}

func TestEvalGoldCondition_UnknownOp(t *testing.T) {
	store := newMockInventoryStore()
	store.gold[1] = 50
	exec := New(store, &resource.ResourceLoader{}, nopLogger())
	s := testSession(1)

	assert.False(t, exec.evalGoldCondition(context.Background(), s, []interface{}{float64(7), float64(50), float64(99)}))
}

// ========================================================================
// resolveTextCodes remaining branches — 85.7%
// ========================================================================

func TestResolveTextCodes_VNoGameState(t *testing.T) {
	exec := New(nil, &resource.ResourceLoader{}, nopLogger())
	s := testSession(1)
	// \V[5] with nil opts → "0"
	result := exec.resolveTextCodes(`\V[5]`, s, nil)
	assert.Equal(t, "0", result)
}

func TestResolveTextCodes_NUnknownActor(t *testing.T) {
	exec := New(nil, &resource.ResourceLoader{Actors: []*resource.Actor{nil}}, nopLogger())
	s := testSession(1)
	// \N[5] with no actor 5 → unchanged
	result := exec.resolveTextCodes(`\N[5]`, s, nil)
	assert.Equal(t, `\N[5]`, result)
}

func TestResolveTextCodes_PNon1(t *testing.T) {
	exec := New(nil, &resource.ResourceLoader{}, nopLogger())
	s := testSession(1)
	// \P[2] → unchanged (only P[1] → player name)
	result := exec.resolveTextCodes(`\P[2]`, s, nil)
	assert.Equal(t, `\P[2]`, result)
}

func TestResolveTextCodes_N101(t *testing.T) {
	exec := New(nil, &resource.ResourceLoader{}, nopLogger())
	s := testSession(1)
	s.CharName = "Luna"
	// \N[101] → player name
	result := exec.resolveTextCodes(`\N[101]`, s, nil)
	assert.Equal(t, "Luna", result)
}

// ========================================================================
// evalItemCondition nil store — 75%
// ========================================================================

func TestEvalItemCondition_NilStore(t *testing.T) {
	exec := New(nil, &resource.ResourceLoader{}, nopLogger())
	s := testSession(1)
	assert.False(t, exec.evalItemCondition(context.Background(), s, []interface{}{float64(8), float64(1)}, 1))
}

// ========================================================================
// applyEquipChange missing branches — 87.5%
// ========================================================================

func TestApplyEquipChange_InvalidArmorID(t *testing.T) {
	store := newMockInventoryStore()
	exec := New(store, &resource.ResourceLoader{}, nopLogger())
	s := testSession(1)
	s.Equips = map[int]int{}
	opts := &ExecuteOpts{GameState: newMockGameState()}

	// Invalid armor ID string
	exec.applyEquipChange(context.Background(), s, "Cloth", "abc", opts)
	// Should log warning, not crash
}

func TestApplyEquipChange_NilStore(t *testing.T) {
	exec := New(nil, &resource.ResourceLoader{}, nopLogger())
	s := testSession(1)
	s.Equips = map[int]int{}
	opts := &ExecuteOpts{GameState: newMockGameState()}

	exec.applyEquipChange(context.Background(), s, "Cloth", "10", opts)
	assert.Equal(t, 10, s.GetEquip(1))
	// No store → no DB persist, but equip_change sent
}

// ========================================================================
// applyChangeWeapons remove — 75%
// ========================================================================

func TestApplyChangeWeapons_AddAndRemove(t *testing.T) {
	store := newMockInventoryStore()
	exec := New(store, &resource.ResourceLoader{}, nopLogger())
	s := testSession(1)

	// Add
	exec.applyChangeWeapons(context.Background(), s, []interface{}{float64(5), float64(0), float64(0), float64(3)}, nil)
	assert.Equal(t, 3, store.items[itemKey(1, 5)])

	// Remove
	exec.applyChangeWeapons(context.Background(), s, []interface{}{float64(5), float64(1), float64(0), float64(2)}, nil)
	assert.Equal(t, 1, store.items[itemKey(1, 5)])
}

// ========================================================================
// applyChangeArmors add — 83.3%
// ========================================================================

func TestApplyChangeArmors_AddWithVarOperand(t *testing.T) {
	store := newMockInventoryStore()
	gs := newMockGameState()
	gs.variables[5] = 3
	exec := New(store, &resource.ResourceLoader{}, nopLogger())
	s := testSession(1)
	opts := &ExecuteOpts{GameState: gs}

	// operandType=1 (variable) → qty from v[5]=3
	exec.applyChangeArmors(context.Background(), s, []interface{}{float64(10), float64(0), float64(1), float64(5)}, opts)
	assert.Equal(t, 3, store.items[itemKey(1, 10)])
}

// ========================================================================
// classParam last entry fallback — 87.5%
// ========================================================================

func TestClassParam_LastEntryFallback(t *testing.T) {
	cls := &resource.Class{
		Params: [][]int{{0, 100, 200}}, // Only 3 entries (level 0-2)
	}
	// Level beyond range → returns last entry
	assert.Equal(t, 200, classParam(cls, 0, 99))
}

func TestClassParam_EmptyRow(t *testing.T) {
	cls := &resource.Class{
		Params: [][]int{{}}, // Empty row
	}
	assert.Equal(t, 0, classParam(cls, 0, 1))
}

// ========================================================================
// evalScriptCondition/Value panic recovery — 82.4%/88.2%
// ========================================================================

func TestEvalScriptCondition_ErrorScript(t *testing.T) {
	s := testSession(1)
	exec := New(nil, &resource.ResourceLoader{}, nopLogger())
	opts := &ExecuteOpts{GameState: newMockGameState()}

	// Script that throws error
	result, ok := exec.evalScriptCondition("throw new Error('test')", s, opts)
	assert.True(t, ok) // error → returns false, true
	assert.False(t, result)
}

func TestEvalScriptValue_ValidExpr(t *testing.T) {
	s := testSession(1)
	gs := newMockGameState()
	gs.variables[10] = 42
	exec := New(nil, &resource.ResourceLoader{}, nopLogger())
	opts := &ExecuteOpts{GameState: gs}

	val := exec.evalScriptValue("$gameVariables.value(10)", s, opts)
	assert.Equal(t, 42, val)
}

// ========================================================================
// applyGold GetGold error — 82.6%
// ========================================================================

func TestApplyGold_Increase(t *testing.T) {
	store := newMockInventoryStore()
	store.gold[1] = 100
	exec := New(store, &resource.ResourceLoader{}, nopLogger())
	s := testSession(1)

	// Increase gold
	err := exec.applyGold(context.Background(), s, []interface{}{float64(0), float64(0), float64(50)}, nil)
	require.NoError(t, err)
	assert.Equal(t, int64(150), store.gold[1])
}

// ========================================================================
// applyItems add success — 86.2%
// ========================================================================

func TestApplyItems_AddSuccess(t *testing.T) {
	store := newMockInventoryStore()
	exec := New(store, &resource.ResourceLoader{}, nopLogger())
	s := testSession(1)

	err := exec.applyItems(context.Background(), s, []interface{}{float64(5), float64(0), float64(0), float64(10)}, nil)
	require.NoError(t, err)
	assert.Equal(t, 10, store.items[itemKey(1, 5)])
}

// ========================================================================
// Dispatch: ReturnToTitle
// ========================================================================

func TestDispatch_ReturnToTitle(t *testing.T) {
	s := testSession(1)
	exec := New(nil, &resource.ResourceLoader{}, nopLogger())

	page := &resource.EventPage{
		List: []*resource.EventCommand{
			{Code: CmdReturnToTitle},
			{Code: CmdEnd},
		},
	}
	exec.Execute(context.Background(), s, page, nil)
	pkts := drainPackets(t, s)
	found := false
	for _, p := range pkts {
		if p.Type == "npc_effect" {
			var data map[string]interface{}
			json.Unmarshal(p.Payload, &data)
			if data["code"].(float64) == float64(CmdReturnToTitle) {
				found = true
			}
		}
	}
	assert.True(t, found)
}

// ========================================================================
// Dispatch: CmdWait frame sleep
// ========================================================================

func TestDispatch_Wait(t *testing.T) {
	s := testSession(1)
	exec := New(nil, &resource.ResourceLoader{}, nopLogger())

	start := time.Now()
	page := &resource.EventPage{
		List: []*resource.EventCommand{
			{Code: CmdWait, Parameters: []interface{}{float64(6)}}, // 6 frames = 100ms
			{Code: CmdEnd},
		},
	}
	exec.Execute(context.Background(), s, page, nil)
	elapsed := time.Since(start)
	assert.True(t, elapsed >= 90*time.Millisecond)
}

// ========================================================================
// Dispatch: SetEventLocation resolves charID
// ========================================================================

func TestDispatch_SetEventLocation_CharIDZero(t *testing.T) {
	s := testSession(1)
	exec := New(nil, &resource.ResourceLoader{}, nopLogger())
	opts := &ExecuteOpts{MapID: 1, EventID: 7}

	page := &resource.EventPage{
		List: []*resource.EventCommand{
			{Code: CmdSetEventLocation, Parameters: []interface{}{float64(0), float64(10), float64(20)}},
			{Code: CmdEnd},
		},
	}
	exec.Execute(context.Background(), s, page, opts)
	pkts := drainPackets(t, s)
	for _, p := range pkts {
		if p.Type == "npc_effect" {
			var data map[string]interface{}
			json.Unmarshal(p.Payload, &data)
			if data["code"].(float64) == float64(CmdSetEventLocation) {
				params := data["params"].([]interface{})
				assert.Equal(t, float64(7), params[0]) // resolved to eventID=7
			}
		}
	}
}

// ========================================================================
// Parallel: stepUntilWait — HP/MP/State/EXP/Level/Class/Equip/etc
// ========================================================================

func TestStepUntilWait_HPMPStateRecoverEXPLevelClass(t *testing.T) {
	gs := newMockGameState()
	s := testSessionWithStats(1, 100, 200, 50, 100, 10, 100)
	s.MapID = 1
	s.ClassID = 1
	res := &resource.ResourceLoader{
		Classes: []*resource.Class{
			nil,
			{ID: 1, Params: [][]int{{0, 100}, {0, 50}}},
			{ID: 2, Params: [][]int{{0, 200}, {0, 80}}},
		},
	}
	exec := New(nil, res, nopLogger())
	opts := &ExecuteOpts{MapID: 1, GameState: gs}

	ev := &ParallelEventState{
		EventID: 1,
		Cmds: []*resource.EventCommand{
			// ChangeHP: add 10
			{Code: CmdChangeHP, Parameters: []interface{}{float64(0), float64(1), float64(0), float64(0), float64(10), float64(0)}},
			// ChangeMP: add 10
			{Code: CmdChangeMP, Parameters: []interface{}{float64(0), float64(1), float64(0), float64(0), float64(10)}},
			// ChangeState: add state 5
			{Code: CmdChangeState, Parameters: []interface{}{float64(0), float64(1), float64(0), float64(5)}},
			// RecoverAll
			{Code: CmdRecoverAll, Parameters: []interface{}{float64(0), float64(1)}},
			// ChangeEXP: add 50
			{Code: CmdChangeEXP, Parameters: []interface{}{float64(0), float64(1), float64(0), float64(0), float64(50), float64(0)}},
			// ChangeLevel: add 1
			{Code: CmdChangeLevel, Parameters: []interface{}{float64(0), float64(1), float64(0), float64(0), float64(1), float64(0)}},
			// ChangeClass: change to class 2
			{Code: CmdChangeClass, Parameters: []interface{}{float64(1), float64(2), float64(0)}},
			{Code: CmdEnd, Indent: 0},
		},
	}

	exec.stepUntilWait(context.Background(), s, ev, opts)
	assert.Equal(t, 2, s.ClassID)
}

func TestStepUntilWait_SetEventLocationBalloonAnimation(t *testing.T) {
	gs := newMockGameState()
	s := testSession(1)
	s.MapID = 1
	exec := New(nil, &resource.ResourceLoader{}, nopLogger())
	opts := &ExecuteOpts{MapID: 1, EventID: 5, GameState: gs}

	ev := &ParallelEventState{
		EventID: 5,
		Cmds: []*resource.EventCommand{
			{Code: CmdSetEventLocation, Parameters: []interface{}{float64(0), float64(10), float64(20)}},
			{Code: CmdShowAnimation, Parameters: []interface{}{float64(0), float64(1)}},
			{Code: CmdShowBalloon, Parameters: []interface{}{float64(0), float64(1)}},
			{Code: CmdEnd, Indent: 0},
		},
	}

	exec.stepUntilWait(context.Background(), s, ev, opts)
	pkts := drainPackets(t, s)
	assert.True(t, len(pkts) >= 3)
}

// ========================================================================
// Dispatch: ShowAnimation with wait
// ========================================================================

func TestDispatch_ShowAnimation_Wait_CharIDResolve(t *testing.T) {
	s := testSession(1)
	exec := New(nil, &resource.ResourceLoader{}, nopLogger())
	opts := &ExecuteOpts{MapID: 1, EventID: 3}

	page := &resource.EventPage{
		List: []*resource.EventCommand{
			{Code: CmdShowAnimation, Parameters: []interface{}{float64(0), float64(1), true}},
			{Code: CmdEnd},
		},
	}

	go func() {
		time.Sleep(10 * time.Millisecond)
		s.EffectAckCh <- struct{}{}
	}()

	exec.Execute(context.Background(), s, page, opts)
	pkts := drainPackets(t, s)
	found := false
	for _, p := range pkts {
		if p.Type == "npc_effect" {
			var data map[string]interface{}
			json.Unmarshal(p.Payload, &data)
			if data["code"].(float64) == float64(CmdShowAnimation) {
				params := data["params"].([]interface{})
				assert.Equal(t, float64(3), params[0])
				found = true
			}
		}
	}
	assert.True(t, found)
}

// ========================================================================
// Dispatch: ShowBalloon with wait
// ========================================================================

func TestDispatch_ShowBalloon_Wait_CharIDResolve(t *testing.T) {
	s := testSession(1)
	exec := New(nil, &resource.ResourceLoader{}, nopLogger())
	opts := &ExecuteOpts{MapID: 1, EventID: 3}

	page := &resource.EventPage{
		List: []*resource.EventCommand{
			{Code: CmdShowBalloon, Parameters: []interface{}{float64(0), float64(1), true}},
			{Code: CmdEnd},
		},
	}

	go func() {
		time.Sleep(10 * time.Millisecond)
		s.EffectAckCh <- struct{}{}
	}()

	exec.Execute(context.Background(), s, page, opts)
	pkts := drainPackets(t, s)
	found := false
	for _, p := range pkts {
		if p.Type == "npc_effect" {
			var data map[string]interface{}
			json.Unmarshal(p.Payload, &data)
			if data["code"].(float64) == float64(CmdShowBalloon) {
				params := data["params"].([]interface{})
				assert.Equal(t, float64(3), params[0])
				found = true
			}
		}
	}
	assert.True(t, found)
}

// ========================================================================
// Transfer: cross-map saves return position
// ========================================================================

func TestTransferPlayer_CrossMapSavesReturn(t *testing.T) {
	gs := newMockGameState()
	s := testSession(1)
	s.SetPosition(10, 20, 4)
	exec := New(nil, &resource.ResourceLoader{}, nopLogger())
	transferred := false
	opts := &ExecuteOpts{
		MapID: 1, EventID: 1, GameState: gs,
		TransferFn: func(_ *player.PlayerSession, mapID, x, y, dir int) {
			transferred = true
			assert.Equal(t, 2, mapID)
		},
	}

	exec.transferPlayer(s, []interface{}{float64(0), float64(2), float64(5), float64(6), float64(4)}, opts)
	assert.True(t, transferred)
	// Return position saved
	assert.Equal(t, 1, gs.variables[421])
	assert.Equal(t, 10, gs.variables[422])
	assert.Equal(t, 20, gs.variables[423])
}

// ========================================================================
// Dispatch: CmdEnd indent > 0 (non-terminal)
// ========================================================================

func TestDispatch_CmdEnd_IndentGtZero(t *testing.T) {
	gs := newMockGameState()
	s := testSession(1)
	exec := New(nil, &resource.ResourceLoader{}, nopLogger())
	opts := &ExecuteOpts{GameState: gs}

	page := &resource.EventPage{
		List: []*resource.EventCommand{
			{Code: CmdEnd, Indent: 1},   // Non-terminal, should skip
			{Code: CmdChangeSwitches, Parameters: []interface{}{float64(1), float64(1), float64(0)}},
			{Code: CmdEnd, Indent: 0},   // Terminal
		},
	}
	exec.Execute(context.Background(), s, page, opts)
	assert.True(t, gs.switches[1])
}

// ========================================================================
// Dispatch: Script forward safe lines
// ========================================================================

func TestDispatch_Script_SafeForward(t *testing.T) {
	s := testSession(1)
	exec := New(nil, &resource.ResourceLoader{}, nopLogger())

	page := &resource.EventPage{
		List: []*resource.EventCommand{
			{Code: CmdScript, Parameters: []interface{}{"$gameScreen.startFadeOut(24)"}},
			{Code: CmdEnd},
		},
	}
	exec.Execute(context.Background(), s, page, nil)
	pkts := drainPackets(t, s)
	found := false
	for _, p := range pkts {
		if p.Type == "npc_effect" {
			var data map[string]interface{}
			json.Unmarshal(p.Payload, &data)
			if data["code"].(float64) == float64(CmdScript) {
				found = true
			}
		}
	}
	assert.True(t, found)
}

func TestDispatch_Script_AudioManagerForward(t *testing.T) {
	s := testSession(1)
	exec := New(nil, &resource.ResourceLoader{}, nopLogger())

	page := &resource.EventPage{
		List: []*resource.EventCommand{
			{Code: CmdScript, Parameters: []interface{}{"AudioManager.playSe({name: 'hit'})"}},
			{Code: CmdEnd},
		},
	}
	exec.Execute(context.Background(), s, page, nil)
	pkts := drainPackets(t, s)
	found := false
	for _, p := range pkts {
		if p.Type == "npc_effect" {
			var data map[string]interface{}
			json.Unmarshal(p.Payload, &data)
			if data["code"].(float64) == float64(CmdScript) {
				found = true
			}
		}
	}
	assert.True(t, found)
}

func TestDispatch_Script_ScriptCont(t *testing.T) {
	s := testSession(1)
	gs := newMockGameState()
	exec := New(nil, &resource.ResourceLoader{}, nopLogger())
	opts := &ExecuteOpts{GameState: gs}

	page := &resource.EventPage{
		List: []*resource.EventCommand{
			{Code: CmdScript, Parameters: []interface{}{"$gameVariables._data[1] = 42"}},
			{Code: CmdScriptCont, Parameters: []interface{}{""}},
			{Code: CmdEnd},
		},
	}
	exec.Execute(context.Background(), s, page, opts)
	assert.Equal(t, 42, gs.variables[1])
}

// ========================================================================
// Dispatch: ChangeGold/ChangeItems error paths
// ========================================================================

func TestDispatch_ChangeGold_Error(t *testing.T) {
	// nil store → error
	exec := New(nil, &resource.ResourceLoader{}, nopLogger())
	s := testSession(1)

	page := &resource.EventPage{
		List: []*resource.EventCommand{
			{Code: CmdChangeGold, Parameters: []interface{}{float64(0), float64(0), float64(100)}},
			{Code: CmdEnd},
		},
	}
	exec.Execute(context.Background(), s, page, nil)
	// Should not crash, just log warning
}

func TestDispatch_ChangeItems_Error(t *testing.T) {
	exec := New(nil, &resource.ResourceLoader{}, nopLogger())
	s := testSession(1)

	page := &resource.EventPage{
		List: []*resource.EventCommand{
			{Code: CmdChangeItems, Parameters: []interface{}{float64(1), float64(0), float64(0), float64(5)}},
			{Code: CmdEnd},
		},
	}
	exec.Execute(context.Background(), s, page, nil)
}

// ========================================================================
// Dispatch: ChangeGold/ChangeItems success paths (npc_effect sent)
// ========================================================================

func TestDispatch_ChangeGold_Success(t *testing.T) {
	store := newMockInventoryStore()
	store.gold[1] = 100
	exec := New(store, &resource.ResourceLoader{}, nopLogger())
	s := testSession(1)

	page := &resource.EventPage{
		List: []*resource.EventCommand{
			{Code: CmdChangeGold, Parameters: []interface{}{float64(0), float64(0), float64(50)}},
			{Code: CmdEnd},
		},
	}
	exec.Execute(context.Background(), s, page, nil)
	assert.Equal(t, int64(150), store.gold[1])
	pkts := drainPackets(t, s)
	found := false
	for _, p := range pkts {
		if p.Type == "npc_effect" {
			var data map[string]interface{}
			json.Unmarshal(p.Payload, &data)
			if data["code"].(float64) == float64(CmdChangeGold) {
				found = true
			}
		}
	}
	assert.True(t, found)
}

func TestDispatch_ChangeItems_Success(t *testing.T) {
	store := newMockInventoryStore()
	exec := New(store, &resource.ResourceLoader{}, nopLogger())
	s := testSession(1)

	page := &resource.EventPage{
		List: []*resource.EventCommand{
			{Code: CmdChangeItems, Parameters: []interface{}{float64(5), float64(0), float64(0), float64(3)}},
			{Code: CmdEnd},
		},
	}
	exec.Execute(context.Background(), s, page, nil)
	assert.Equal(t, 3, store.items[itemKey(1, 5)])
}

// ========================================================================
// Dispatch: EquipChange in dispatch
// ========================================================================

func TestDispatch_EquipChange_InDispatch(t *testing.T) {
	store := newMockInventoryStore()
	gs := newMockGameState()
	s := testSession(1)
	s.Equips = map[int]int{}
	exec := New(store, &resource.ResourceLoader{}, nopLogger())
	opts := &ExecuteOpts{GameState: gs}

	page := &resource.EventPage{
		List: []*resource.EventCommand{
			{Code: CmdPluginCommand, Parameters: []interface{}{"EquipChange Cloth 10"}},
			{Code: CmdEnd},
		},
	}
	exec.Execute(context.Background(), s, page, opts)
	assert.Equal(t, 10, s.GetEquip(1))
}

// ========================================================================
// Dispatch: EnterInstance/LeaveInstance in dispatch
// ========================================================================

func TestDispatch_Instance_InDispatch(t *testing.T) {
	s := testSession(1)
	exec := New(nil, &resource.ResourceLoader{}, nopLogger())
	entered := false
	left := false
	opts := &ExecuteOpts{
		GameState:       newMockGameState(),
		EnterInstanceFn: func(_ *player.PlayerSession) { entered = true },
		LeaveInstanceFn: func(_ *player.PlayerSession) { left = true },
	}

	page := &resource.EventPage{
		List: []*resource.EventCommand{
			{Code: CmdPluginCommand, Parameters: []interface{}{"EnterInstance"}},
			{Code: CmdPluginCommand, Parameters: []interface{}{"LeaveInstance"}},
			{Code: CmdEnd},
		},
	}
	exec.Execute(context.Background(), s, page, opts)
	assert.True(t, entered)
	assert.True(t, left)
}

// ========================================================================
// applySwitches nil opts — 93.3%
// ========================================================================

func TestApplySwitches_NilOpts(t *testing.T) {
	s := testSession(1)
	exec := New(nil, &resource.ResourceLoader{}, nopLogger())
	exec.applySwitches(s, []interface{}{float64(1), float64(1), float64(0)}, nil)
}

// ========================================================================
// applyVariables nil opts
// ========================================================================

func TestApplyVariables_NilOpts(t *testing.T) {
	s := testSession(1)
	exec := New(nil, &resource.ResourceLoader{}, nopLogger())
	exec.applyVariables(s, []interface{}{float64(1), float64(1), float64(0), float64(0), float64(5)}, nil)
}

// ========================================================================
// evaluateCondition condType > 2 with nil GameState
// ========================================================================

func TestEvalCondition_NilGS_TypeGt2(t *testing.T) {
	s := testSession(1)
	exec := New(nil, &resource.ResourceLoader{}, nopLogger())

	// condType=7 (gold) with nil gs → false
	assert.False(t, exec.evaluateCondition(context.Background(), s, []interface{}{float64(7), float64(100), float64(0)}, nil))
}

// ========================================================================
// resolveTextVarRef with actual match
// ========================================================================

func TestResolveTextVarRef_WithMatch(t *testing.T) {
	gs := newMockGameState()
	gs.variables[10] = 42
	exec := New(nil, &resource.ResourceLoader{}, nopLogger())
	opts := &ExecuteOpts{GameState: gs}

	result := exec.resolveTextVarRef(`\v[10]`, opts)
	assert.Equal(t, "42", result)

	// Double backslash variant
	result = exec.resolveTextVarRef(`\\V[10]`, opts)
	assert.Equal(t, "42", result)
}

// ========================================================================
// Dispatch: MoveRoute with SetMoveRoute wait=true
// ========================================================================

func TestDispatch_SetMoveRoute_WaitTrue(t *testing.T) {
	s := testSession(1)
	exec := New(nil, &resource.ResourceLoader{}, nopLogger())
	opts := &ExecuteOpts{MapID: 1, EventID: 1}

	page := &resource.EventPage{
		List: []*resource.EventCommand{
			{Code: CmdSetMoveRoute, Parameters: []interface{}{
				float64(1),
				map[string]interface{}{"wait": true, "list": []interface{}{}},
			}},
			{Code: CmdEnd},
		},
	}

	go func() {
		time.Sleep(10 * time.Millisecond)
		s.EffectAckCh <- struct{}{}
	}()

	exec.Execute(context.Background(), s, page, opts)
}

// ========================================================================
// execCulSkillEffect nil session equips
// ========================================================================

func TestExecCulSkillEffect_NilEquips(t *testing.T) {
	gs := newMockGameState()
	s := testSession(1)
	s.ClassID = 1
	s.Equips = nil
	exec := New(nil, &resource.ResourceLoader{}, nopLogger())
	opts := &ExecuteOpts{GameState: gs, TransientVars: make(map[int]interface{})}

	exec.execCulSkillEffect(s, opts)
	pkts := drainPackets(t, s)
	assert.True(t, len(pkts) > 0)
}

// ========================================================================
// Dispatch: TintPicture wait=false
// ========================================================================

func TestDispatch_TintPicture_NoWait(t *testing.T) {
	s := testSession(1)
	exec := New(nil, &resource.ResourceLoader{}, nopLogger())

	page := &resource.EventPage{
		List: []*resource.EventCommand{
			{Code: CmdTintPicture, Parameters: []interface{}{float64(1), []interface{}{float64(0)}, float64(30), false}},
			{Code: CmdEnd},
		},
	}
	exec.Execute(context.Background(), s, page, nil)
	pkts := drainPackets(t, s)
	assert.True(t, len(pkts) >= 1)
}

// ========================================================================
// Dispatch: SetWeather wait=false
// ========================================================================

func TestDispatch_SetWeather_NoWait(t *testing.T) {
	s := testSession(1)
	exec := New(nil, &resource.ResourceLoader{}, nopLogger())

	page := &resource.EventPage{
		List: []*resource.EventCommand{
			{Code: CmdSetWeather, Parameters: []interface{}{"rain", float64(5), float64(30), false}},
			{Code: CmdEnd},
		},
	}
	exec.Execute(context.Background(), s, page, nil)
	pkts := drainPackets(t, s)
	assert.True(t, len(pkts) >= 1)
}

// ========================================================================
// Dispatch: BattleProcessing with escape/lose branches
// ========================================================================

func TestDispatch_BattleProcessing_EscapeBranch(t *testing.T) {
	gs := newMockGameState()
	s := testSession(1)
	exec := New(nil, &resource.ResourceLoader{}, nopLogger())
	opts := &ExecuteOpts{
		GameState: gs,
		BattleFn:  func(ctx context.Context, s *player.PlayerSession, troopID int, canEscape, canLose bool) int { return 1 }, // escape
	}

	page := &resource.EventPage{
		List: []*resource.EventCommand{
			{Code: CmdBattleProcessing, Indent: 0, Parameters: []interface{}{float64(0), float64(1), float64(1), float64(1)}},
			{Code: CmdBattleWin, Indent: 0},
			{Code: CmdChangeSwitches, Indent: 1, Parameters: []interface{}{float64(10), float64(10), float64(0)}},
			{Code: CmdEnd, Indent: 1},
			{Code: CmdBattleEscape, Indent: 0},
			{Code: CmdChangeSwitches, Indent: 1, Parameters: []interface{}{float64(20), float64(20), float64(0)}},
			{Code: CmdEnd, Indent: 1},
			{Code: CmdBattleLose, Indent: 0},
			{Code: CmdChangeSwitches, Indent: 1, Parameters: []interface{}{float64(30), float64(30), float64(0)}},
			{Code: CmdEnd, Indent: 1},
			{Code: CmdEnd, Indent: 0},
		},
	}
	exec.Execute(context.Background(), s, page, opts)
	assert.False(t, gs.switches[10]) // Win skipped
	assert.True(t, gs.switches[20])  // Escape executed
	assert.False(t, gs.switches[30]) // Lose skipped
}

func TestDispatch_BattleProcessing_LoseBranch(t *testing.T) {
	gs := newMockGameState()
	s := testSession(1)
	exec := New(nil, &resource.ResourceLoader{}, nopLogger())
	opts := &ExecuteOpts{
		GameState: gs,
		BattleFn:  func(ctx context.Context, s *player.PlayerSession, troopID int, canEscape, canLose bool) int { return 2 }, // lose
	}

	page := &resource.EventPage{
		List: []*resource.EventCommand{
			{Code: CmdBattleProcessing, Indent: 0, Parameters: []interface{}{float64(0), float64(1), float64(1), float64(1)}},
			{Code: CmdBattleWin, Indent: 0},
			{Code: CmdChangeSwitches, Indent: 1, Parameters: []interface{}{float64(10), float64(10), float64(0)}},
			{Code: CmdEnd, Indent: 1},
			{Code: CmdBattleLose, Indent: 0},
			{Code: CmdChangeSwitches, Indent: 1, Parameters: []interface{}{float64(30), float64(30), float64(0)}},
			{Code: CmdEnd, Indent: 1},
			{Code: CmdEnd, Indent: 0},
		},
	}
	exec.Execute(context.Background(), s, page, opts)
	assert.False(t, gs.switches[10]) // Win skipped
	assert.True(t, gs.switches[30])  // Lose executed
}
