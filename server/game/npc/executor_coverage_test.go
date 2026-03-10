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
// Utility functions coverage
// ========================================================================

func TestParamStr_OutOfBounds_Coverage(t *testing.T) {
	assert.Equal(t, "", paramStr(nil, 0))
	assert.Equal(t, "", paramStr([]interface{}{}, 0))
	assert.Equal(t, "", paramStr([]interface{}{42}, 0)) // not a string
	assert.Equal(t, "hello", paramStr([]interface{}{"hello"}, 0))
}

func TestParamInt_AllTypes(t *testing.T) {
	assert.Equal(t, 0, paramInt(nil, 0))
	assert.Equal(t, 0, paramInt([]interface{}{}, 0))
	assert.Equal(t, 42, paramInt([]interface{}{42}, 0))             // int
	assert.Equal(t, 42, paramInt([]interface{}{float64(42)}, 0))    // float64
	assert.Equal(t, 42, paramInt([]interface{}{int64(42)}, 0))      // int64
	assert.Equal(t, 0, paramInt([]interface{}{"not a number"}, 0))  // string
}

func TestParamList_EdgeCases(t *testing.T) {
	assert.Nil(t, paramList(nil, 0))
	assert.Nil(t, paramList([]interface{}{}, 0))
	assert.Nil(t, paramList([]interface{}{"not a list"}, 0)) // not []interface{}
	result := paramList([]interface{}{[]interface{}{"a", "b", float64(3)}}, 0)
	assert.Equal(t, []string{"a", "b"}, result) // float64(3) skipped
}

func TestTruncateStr(t *testing.T) {
	assert.Equal(t, "hello", truncateStr("hello", 10))
	assert.Equal(t, "hel...", truncateStr("hello world", 3))
	assert.Equal(t, "", truncateStr("", 5))
}

func TestAsBool_AllTypes(t *testing.T) {
	assert.True(t, asBool(true))
	assert.False(t, asBool(false))
	assert.True(t, asBool(float64(1)))
	assert.False(t, asBool(float64(0)))
	assert.True(t, asBool(1))
	assert.False(t, asBool(0))
	assert.True(t, asBool("true"))
	assert.True(t, asBool("1"))
	assert.False(t, asBool("false"))
	assert.False(t, asBool("0"))
	assert.False(t, asBool(nil))
	assert.False(t, asBool([]int{1})) // unsupported type
}

func TestExtractPluginCmdName(t *testing.T) {
	assert.Equal(t, "FaceId", extractPluginCmdName("FaceId 1 30"))
	assert.Equal(t, "ParaCheck", extractPluginCmdName("ParaCheck"))
	assert.Equal(t, "", extractPluginCmdName(""))
}

func TestContainsClientOnlyPluginCmd_Coverage(t *testing.T) {
	assert.True(t, ContainsClientOnlyPluginCmd([]*resource.EventCommand{
		{Code: CmdPluginCommand, Parameters: []interface{}{"hzchoiceevent arg1"}},
	}))
	assert.False(t, ContainsClientOnlyPluginCmd([]*resource.EventCommand{
		{Code: CmdPluginCommand, Parameters: []interface{}{"SomeOther arg1"}},
	}))
	assert.False(t, ContainsClientOnlyPluginCmd([]*resource.EventCommand{
		{Code: CmdShowText, Parameters: []interface{}{"hello"}},
	}))
	assert.False(t, ContainsClientOnlyPluginCmd(nil))
}

// ========================================================================
// resolveTextCodes coverage
// ========================================================================

func TestResolveTextCodes_AllCodes(t *testing.T) {
	gs := newMockGameState()
	gs.variables[10] = 42

	res := &resource.ResourceLoader{
		Actors: []*resource.Actor{nil, {ID: 1, Name: "Hero"}, nil, {ID: 3, Name: "NPC3"}},
	}
	exec := New(nil, res, nopLogger())
	s := testSession(1)
	s.CharName = "Luna"

	opts := &ExecuteOpts{GameState: gs}

	// \N[1] → player name
	assert.Equal(t, "Luna", exec.resolveTextCodes(`\N[1]`, s, opts))
	// \N[101] → player name
	assert.Equal(t, "Luna", exec.resolveTextCodes(`\N[101]`, s, opts))
	// \N[3] → actor name from resource
	assert.Equal(t, "NPC3", exec.resolveTextCodes(`\N[3]`, s, opts))
	// \N[99] → no match, keep original
	assert.Equal(t, `\N[99]`, exec.resolveTextCodes(`\N[99]`, s, opts))
	// \V[10] → variable value
	assert.Equal(t, "42", exec.resolveTextCodes(`\V[10]`, s, opts))
	// \P[1] → player name
	assert.Equal(t, "Luna", exec.resolveTextCodes(`\P[1]`, s, opts))
	// \P[2] → keep original
	assert.Equal(t, `\P[2]`, exec.resolveTextCodes(`\P[2]`, s, opts))
	// \V without game state
	assert.Equal(t, "0", exec.resolveTextCodes(`\V[10]`, s, nil))
	// Case insensitive
	assert.Equal(t, "42", exec.resolveTextCodes(`\v[10]`, s, opts))
}

// ========================================================================
// resolveTextVarRef coverage
// ========================================================================

func TestResolveTextVarRef(t *testing.T) {
	gs := newMockGameState()
	gs.variables[100] = 55

	exec := New(nil, nil, nopLogger())
	opts := &ExecuteOpts{GameState: gs}

	assert.Equal(t, "55", exec.resolveTextVarRef(`\v[100]`, opts))
	assert.Equal(t, "55", exec.resolveTextVarRef(`\V[100]`, opts))
	assert.Equal(t, "plain", exec.resolveTextVarRef("plain", opts))
	assert.Equal(t, `\v[100]`, exec.resolveTextVarRef(`\v[100]`, nil))
}

// ========================================================================
// Gold condition coverage
// ========================================================================

func TestEvalGoldCondition(t *testing.T) {
	store := newMockInventoryStore()
	store.gold[1] = 500

	exec := New(store, &resource.ResourceLoader{}, nopLogger())
	s := testSession(1)
	gs := newMockGameState()
	opts := &ExecuteOpts{GameState: gs}

	// Gold condition params: [condType=7, amount, op]
	// op=0 (>=): 500 >= 300 → true
	page := condPage(7, float64(300), float64(0))
	s.DialogAckCh <- struct{}{}
	exec.Execute(context.Background(), s, page, opts)
	assertDialogText(t, s, "TRUE")

	// op=1 (<=): 500 <= 300 → false
	s = testSession(1)
	page = condPage(7, float64(300), float64(1))
	s.DialogAckCh <- struct{}{}
	exec.Execute(context.Background(), s, page, opts)
	assertDialogText(t, s, "FALSE")

	// op=2 (<): 500 < 600 → true
	s = testSession(1)
	page = condPage(7, float64(600), float64(2))
	s.DialogAckCh <- struct{}{}
	exec.Execute(context.Background(), s, page, opts)
	assertDialogText(t, s, "TRUE")

	// nil store → false
	exec2 := New(nil, &resource.ResourceLoader{}, nopLogger())
	s = testSession(1)
	page = condPage(7, float64(100), float64(0))
	s.DialogAckCh <- struct{}{}
	exec2.Execute(context.Background(), s, page, opts)
	assertDialogText(t, s, "FALSE")
}

// ========================================================================
// Item condition coverage
// ========================================================================

func TestEvalItemCondition(t *testing.T) {
	store := newMockInventoryStore()
	store.items["1_10"] = 5 // charID=1, itemID=10

	exec := New(store, &resource.ResourceLoader{}, nopLogger())
	s := testSession(1)
	gs := newMockGameState()
	opts := &ExecuteOpts{GameState: gs}

	// condType=8 (item), params: [8, itemID]
	page := condPage(8, float64(10))
	s.DialogAckCh <- struct{}{}
	exec.Execute(context.Background(), s, page, opts)
	assertDialogText(t, s, "TRUE")

	// condType=8, itemID=99 → no item → false
	s = testSession(1)
	page = condPage(8, float64(99))
	s.DialogAckCh <- struct{}{}
	exec.Execute(context.Background(), s, page, opts)
	assertDialogText(t, s, "FALSE")

	// condType=9 (weapon), params: [9, weaponID, includeEquip]
	s = testSession(1)
	page = condPage(9, float64(10), float64(1))
	s.DialogAckCh <- struct{}{}
	exec.Execute(context.Background(), s, page, opts)
	assertDialogText(t, s, "TRUE")

	// condType=10 (armor) no item
	s = testSession(1)
	page = condPage(10, float64(99), float64(0))
	s.DialogAckCh <- struct{}{}
	exec.Execute(context.Background(), s, page, opts)
	assertDialogText(t, s, "FALSE")
}

// ========================================================================
// Actor condition coverage (sub-types 0,2,4,5,6)
// ========================================================================

func TestEvalActorCondition_SubTypes(t *testing.T) {
	store := newMockInventoryStore()
	exec := New(store, &resource.ResourceLoader{}, nopLogger())
	gs := newMockGameState()
	opts := &ExecuteOpts{GameState: gs}

	// Actor condition params: [condType=4, actorId, subType, compareVal]
	// subType=0 (in party) → always true
	s := testSession(1)
	page := condPage(4, float64(1), float64(0), float64(0))
	s.DialogAckCh <- struct{}{}
	exec.Execute(context.Background(), s, page, opts)
	assertDialogText(t, s, "TRUE")

	// subType=2 (class) → match
	s = testSession(1)
	s.ClassID = 3
	page = condPage(4, float64(1), float64(2), float64(3))
	s.DialogAckCh <- struct{}{}
	exec.Execute(context.Background(), s, page, opts)
	assertDialogText(t, s, "TRUE")

	// subType=2 (class) → no match
	s = testSession(1)
	s.ClassID = 1
	page = condPage(4, float64(1), float64(2), float64(3))
	s.DialogAckCh <- struct{}{}
	exec.Execute(context.Background(), s, page, opts)
	assertDialogText(t, s, "FALSE")

	// subType=6 (state) → has state
	s = testSession(1)
	s.AddState(5)
	page = condPage(4, float64(1), float64(6), float64(5))
	s.DialogAckCh <- struct{}{}
	exec.Execute(context.Background(), s, page, opts)
	assertDialogText(t, s, "TRUE")

	// subType=4 (weapon equipped) → not equipped (mock always returns false)
	s = testSession(1)
	page = condPage(4, float64(1), float64(4), float64(10))
	s.DialogAckCh <- struct{}{}
	exec.Execute(context.Background(), s, page, opts)
	assertDialogText(t, s, "FALSE")

	// subType=5 (armor equipped) → not equipped
	s = testSession(1)
	page = condPage(4, float64(1), float64(5), float64(10))
	s.DialogAckCh <- struct{}{}
	exec.Execute(context.Background(), s, page, opts)
	assertDialogText(t, s, "FALSE")

	// subType=1 (name) → not implemented → false
	s = testSession(1)
	page = condPage(4, float64(1), float64(1), float64(0))
	s.DialogAckCh <- struct{}{}
	exec.Execute(context.Background(), s, page, opts)
	assertDialogText(t, s, "FALSE")

	// nil store → false for weapon/armor/skill checks
	exec2 := New(nil, &resource.ResourceLoader{}, nopLogger())
	s = testSession(1)
	page = condPage(4, float64(1), float64(4), float64(10))
	s.DialogAckCh <- struct{}{}
	exec2.Execute(context.Background(), s, page, opts)
	assertDialogText(t, s, "FALSE")
}

// ========================================================================
// Unsupported condition types
// ========================================================================

func TestEvalCondition_UnsupportedTypes(t *testing.T) {
	exec := New(nil, &resource.ResourceLoader{}, nopLogger())
	gs := newMockGameState()
	opts := &ExecuteOpts{GameState: gs}

	// condType=3 (timer), 5 (enemy), 6 (character dir), 11 (button) → false
	for _, condType := range []int{3, 5, 6, 11} {
		s := testSession(1)
		page := condPage(condType, float64(0), float64(0))
		s.DialogAckCh <- struct{}{}
		exec.Execute(context.Background(), s, page, opts)
		assertDialogText(t, s, "FALSE")
	}
}

// ========================================================================
// evaluateCondition: nil GameState edge cases
// ========================================================================

func TestEvalCondition_NilGameState(t *testing.T) {
	exec := New(nil, &resource.ResourceLoader{}, nopLogger())
	s := testSession(1)

	// condType=0 (switch) with nil opts → false
	page := condPage(0, float64(0), float64(10), float64(0))
	s.DialogAckCh <- struct{}{}
	exec.Execute(context.Background(), s, page, nil)
	assertDialogText(t, s, "FALSE")

	// condType=4 (actor) with nil GameState → false
	s = testSession(1)
	page = condPage(4, float64(0), float64(1), float64(0), float64(0))
	s.DialogAckCh <- struct{}{}
	exec.Execute(context.Background(), s, page, &ExecuteOpts{})
	assertDialogText(t, s, "FALSE")
}

// ========================================================================
// Self-switch condition (condType=2)
// ========================================================================

func TestEvalCondition_SelfSwitch(t *testing.T) {
	exec := New(nil, &resource.ResourceLoader{}, nopLogger())
	gs := newMockGameState()
	gs.SetSelfSwitch(5, 10, "B", true)

	s := testSession(1)
	opts := &ExecuteOpts{GameState: gs, MapID: 5, EventID: 10}

	// Self-switch B is ON, expected ON → true
	page := &resource.EventPage{
		List: []*resource.EventCommand{
			{Code: CmdConditionalStart, Indent: 0, Parameters: []interface{}{
				float64(2), "B", float64(0),
			}},
			{Code: CmdShowText, Indent: 1, Parameters: []interface{}{"", float64(0), float64(0), float64(2)}},
			{Code: CmdShowTextLine, Indent: 1, Parameters: []interface{}{"TRUE"}},
			{Code: CmdElseBranch, Indent: 0},
			{Code: CmdShowText, Indent: 1, Parameters: []interface{}{"", float64(0), float64(0), float64(2)}},
			{Code: CmdShowTextLine, Indent: 1, Parameters: []interface{}{"FALSE"}},
			{Code: CmdConditionalEnd, Indent: 0},
			{Code: CmdEnd, Indent: 0},
		},
	}
	s.DialogAckCh <- struct{}{}
	exec.Execute(context.Background(), s, page, opts)
	assertDialogText(t, s, "TRUE")

	// Self-switch B is ON, expected OFF → false
	s = testSession(1)
	page.List[0].Parameters = []interface{}{float64(2), "B", float64(1)}
	s.DialogAckCh <- struct{}{}
	exec.Execute(context.Background(), s, page, opts)
	assertDialogText(t, s, "FALSE")
}

// ========================================================================
// Variable condition with TransientVars
// ========================================================================

func TestEvalCondition_TransientVars(t *testing.T) {
	exec := New(nil, &resource.ResourceLoader{}, nopLogger())
	gs := newMockGameState()
	// var[50]=0, but transient has a value → treated as 1
	opts := &ExecuteOpts{
		GameState:     gs,
		TransientVars: map[int]interface{}{50: []int{1, 2, 3}},
	}

	s := testSession(1)
	// condType=1, varID=50, refType=0, refVal=0, op=3 (>): 1 > 0 → true
	page := condPage(1, float64(0), float64(50), float64(0), float64(0), float64(3))
	s.DialogAckCh <- struct{}{}
	exec.Execute(context.Background(), s, page, opts)
	assertDialogText(t, s, "TRUE")
}

// ========================================================================
// applyChangeWeapons coverage
// ========================================================================

func TestApplyChangeWeapons(t *testing.T) {
	store := newMockInventoryStore()
	exec := New(store, &resource.ResourceLoader{}, nopLogger())
	s := testSession(1)
	gs := newMockGameState()
	opts := &ExecuteOpts{GameState: gs}

	// Add weapon: weaponID=5, op=0(add), operandType=0(const), qty=2
	page := &resource.EventPage{
		List: []*resource.EventCommand{
			{Code: CmdChangeWeapons, Indent: 0, Parameters: []interface{}{
				float64(5), float64(0), float64(0), float64(2),
			}},
			{Code: CmdEnd, Indent: 0},
		},
	}
	exec.Execute(context.Background(), s, page, opts)
	assert.Equal(t, 2, store.items["1_5"])

	// Remove weapon: weaponID=5, op=1(remove), operandType=0(const), qty=1
	s = testSession(1)
	page.List[0].Parameters = []interface{}{float64(5), float64(1), float64(0), float64(1)}
	exec.Execute(context.Background(), s, page, opts)
	assert.Equal(t, 1, store.items["1_5"])
}

// ========================================================================
// applyChangeArmors coverage
// ========================================================================

func TestApplyChangeArmors(t *testing.T) {
	store := newMockInventoryStore()
	exec := New(store, &resource.ResourceLoader{}, nopLogger())
	s := testSession(1)
	gs := newMockGameState()
	opts := &ExecuteOpts{GameState: gs}

	// Add armor
	page := &resource.EventPage{
		List: []*resource.EventCommand{
			{Code: CmdChangeArmors, Indent: 0, Parameters: []interface{}{
				float64(10), float64(0), float64(0), float64(3),
			}},
			{Code: CmdEnd, Indent: 0},
		},
	}
	exec.Execute(context.Background(), s, page, opts)
	assert.Equal(t, 3, store.items["1_10"])

	// Remove armor
	s = testSession(1)
	page.List[0].Parameters = []interface{}{float64(10), float64(1), float64(0), float64(2)}
	exec.Execute(context.Background(), s, page, opts)
	assert.Equal(t, 1, store.items["1_10"])
}

// ========================================================================
// sendDialogWithChoices coverage
// ========================================================================

func TestExecute_TextThenChoices_Merged(t *testing.T) {
	exec := New(nil, &resource.ResourceLoader{}, nopLogger())
	s := testSession(1)

	page := &resource.EventPage{
		List: []*resource.EventCommand{
			{Code: CmdShowText, Indent: 0, Parameters: []interface{}{"Actor1", float64(0), float64(0), float64(2)}},
			{Code: CmdShowTextLine, Indent: 0, Parameters: []interface{}{"Pick one:"}},
			{Code: CmdShowChoices, Indent: 0, Parameters: []interface{}{
				[]interface{}{"A", "B"}, float64(-1), float64(0), float64(2), float64(0),
			}},
			{Code: CmdWhenBranch, Indent: 0, Parameters: []interface{}{float64(0)}},
			{Code: CmdBranchEnd, Indent: 0},
			{Code: CmdEnd, Indent: 0},
		},
	}

	go func() {
		time.Sleep(50 * time.Millisecond)
		s.ChoiceCh <- 0
	}()

	exec.Execute(context.Background(), s, page, &ExecuteOpts{})

	pkts := drainPackets(t, s)
	types := packetTypes(pkts)
	assert.Contains(t, types, "npc_dialog_choices")
}

// ========================================================================
// sendMovePicture / sendMovePictureNoWait coverage
// ========================================================================

func TestSendMovePicture_DirectCoords(t *testing.T) {
	exec := New(nil, &resource.ResourceLoader{}, nopLogger())
	s := testSession(1)

	// designation=0 (direct), no wait
	params := []interface{}{float64(1), nil, float64(0), float64(0), float64(100), float64(200),
		float64(100), float64(100), float64(255), float64(0), float64(30), false}
	exec.sendMovePicture(context.Background(), s, params, nil)

	pkts := drainPackets(t, s)
	require.Len(t, pkts, 1)
	assert.Equal(t, "npc_effect", pkts[0].Type)
}

func TestSendMovePicture_VarCoords(t *testing.T) {
	gs := newMockGameState()
	gs.variables[10] = 150
	gs.variables[11] = 250
	exec := New(nil, &resource.ResourceLoader{}, nopLogger())
	s := testSession(1)
	opts := &ExecuteOpts{GameState: gs}

	// designation=1 (variable), no wait
	params := []interface{}{float64(1), nil, float64(0), float64(1), float64(10), float64(11),
		float64(100), float64(100), float64(255), float64(0), float64(30), false}
	exec.sendMovePicture(context.Background(), s, params, opts)

	pkts := drainPackets(t, s)
	require.Len(t, pkts, 1)
	var data map[string]interface{}
	json.Unmarshal(pkts[0].Payload, &data)
	p := data["params"].([]interface{})
	assert.Equal(t, float64(150), p[4])
	assert.Equal(t, float64(250), p[5])
	assert.Equal(t, float64(0), p[3]) // designation changed to 0
}

func TestSendMovePicture_WithWait(t *testing.T) {
	exec := New(nil, &resource.ResourceLoader{}, nopLogger())
	s := testSession(1)

	params := []interface{}{float64(1), nil, float64(0), float64(0), float64(100), float64(200),
		float64(100), float64(100), float64(255), float64(0), float64(30), true}

	go func() {
		time.Sleep(50 * time.Millisecond)
		s.EffectAckCh <- struct{}{}
	}()

	exec.sendMovePicture(context.Background(), s, params, nil)

	pkts := drainPackets(t, s)
	require.True(t, len(pkts) >= 1)
	var data map[string]interface{}
	json.Unmarshal(pkts[0].Payload, &data)
	assert.Equal(t, true, data["wait"])
}

func TestSendMovePictureNoWait_VarCoords(t *testing.T) {
	gs := newMockGameState()
	gs.variables[20] = 300
	gs.variables[21] = 400
	exec := New(nil, &resource.ResourceLoader{}, nopLogger())
	s := testSession(1)
	opts := &ExecuteOpts{GameState: gs}

	params := []interface{}{float64(1), nil, float64(0), float64(1), float64(20), float64(21),
		float64(100), float64(100), float64(255), float64(0), float64(30), false}
	exec.sendMovePictureNoWait(s, params, opts)

	pkts := drainPackets(t, s)
	require.Len(t, pkts, 1)
	var data map[string]interface{}
	json.Unmarshal(pkts[0].Payload, &data)
	p := data["params"].([]interface{})
	assert.Equal(t, float64(300), p[4])
	assert.Equal(t, float64(400), p[5])
}

// ========================================================================
// operateSelfVariable coverage
// ========================================================================

func TestOperateSelfVariable_AllOps(t *testing.T) {
	assert.Equal(t, 10, operateSelfVariable(0, 0, 10))  // set
	assert.Equal(t, 15, operateSelfVariable(10, 1, 5))   // add
	assert.Equal(t, 5, operateSelfVariable(10, 2, 5))    // sub
	assert.Equal(t, 50, operateSelfVariable(10, 3, 5))   // mul
	assert.Equal(t, 2, operateSelfVariable(10, 4, 5))    // div
	assert.Equal(t, 3, operateSelfVariable(13, 5, 5))    // mod
	assert.Equal(t, 10, operateSelfVariable(10, 4, 0))   // div by zero
	assert.Equal(t, 10, operateSelfVariable(10, 5, 0))   // mod by zero
	assert.Equal(t, 10, operateSelfVariable(10, 99, 5))  // unknown op
}

// ========================================================================
// teSetRangeSelfVariable coverage
// ========================================================================

func TestTESetRangeSelfVariable(t *testing.T) {
	gs := newMockGameState()
	exec := New(nil, &resource.ResourceLoader{}, nopLogger())
	s := testSession(1)
	opts := &ExecuteOpts{GameState: gs, MapID: 5, EventID: 10}

	page := &resource.EventPage{
		List: []*resource.EventCommand{
			{Code: CmdPluginCommand, Indent: 0, Parameters: []interface{}{
				"TE_SET_RANGE_SELF_VARIABLE 1 3 0 42",
			}},
			{Code: CmdEnd, Indent: 0},
		},
	}
	exec.Execute(context.Background(), s, page, opts)

	assert.Equal(t, 42, gs.GetSelfVariable(5, 10, 1))
	assert.Equal(t, 42, gs.GetSelfVariable(5, 10, 2))
	assert.Equal(t, 42, gs.GetSelfVariable(5, 10, 3))

	// Verify self_var_change packets sent
	pkts := drainPackets(t, s)
	selfVarCount := 0
	for _, pkt := range pkts {
		if pkt.Type == "self_var_change" {
			selfVarCount++
		}
	}
	assert.Equal(t, 3, selfVarCount)
}

func TestTESetRangeSelfVariable_InsufficientArgs(t *testing.T) {
	gs := newMockGameState()
	exec := New(nil, &resource.ResourceLoader{}, nopLogger())
	s := testSession(1)
	opts := &ExecuteOpts{GameState: gs, MapID: 5, EventID: 10}

	// Only 3 args instead of 4
	page := &resource.EventPage{
		List: []*resource.EventCommand{
			{Code: CmdPluginCommand, Indent: 0, Parameters: []interface{}{
				"TE_SET_RANGE_SELF_VARIABLE 1 3 0",
			}},
			{Code: CmdEnd, Indent: 0},
		},
	}
	exec.Execute(context.Background(), s, page, opts)
	// Should not crash, just skip
}

// ========================================================================
// ExecuteEventByID coverage
// ========================================================================

func TestExecuteEventByID(t *testing.T) {
	_ = newMockGameState()
	res := &resource.ResourceLoader{
		Maps: map[int]*resource.MapData{
			1: {
				Events: []*resource.MapEvent{
					nil,
					{
						ID:   1,
						Name: "TestEvent",
						Pages: []*resource.EventPage{
							{
								List: []*resource.EventCommand{
									{Code: CmdChangeVars, Indent: 0, Parameters: []interface{}{
										float64(1), float64(1), float64(0), float64(0), float64(99),
									}},
									{Code: CmdEnd, Indent: 0},
								},
							},
						},
					},
				},
			},
		},
	}
	exec := New(nil, res, nopLogger())
	s := testSession(1)

	// Mock: ExecuteEventByID doesn't accept opts with GameState.
	// It creates its own opts. So we need the Execute path to have GameState.
	// Actually ExecuteEventByID creates opts without GameState, so vars won't change.
	// Let's verify it at least doesn't crash and sends dialog_end.
	exec.ExecuteEventByID(context.Background(), s, 1, 1)
	pkts := drainPackets(t, s)
	types := packetTypes(pkts)
	assert.Contains(t, types, "npc_dialog_end")

	// Non-existent map
	exec.ExecuteEventByID(context.Background(), s, 999, 1)

	// Non-existent event
	exec.ExecuteEventByID(context.Background(), s, 1, 999)
}

// ========================================================================
// SendStateSyncAfterExecution coverage
// ========================================================================

func TestSendStateSyncAfterExecution(t *testing.T) {
	exec := New(nil, nil, nopLogger())
	// Just ensure it doesn't panic
	exec.SendStateSyncAfterExecution(context.Background(), nil, nil)
}

// ========================================================================
// waitForDialogAck / waitForChoice disconnect paths
// ========================================================================

func TestWaitForDialogAck_Disconnect(t *testing.T) {
	exec := New(nil, nil, nopLogger())
	s := testSession(1)
	close(s.Done)
	assert.False(t, exec.waitForDialogAck(context.Background(), s))
}

func TestWaitForDialogAck_ContextCancel(t *testing.T) {
	exec := New(nil, nil, nopLogger())
	s := testSession(1)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	assert.False(t, exec.waitForDialogAck(ctx, s))
}

func TestWaitForChoice_Disconnect(t *testing.T) {
	exec := New(nil, nil, nopLogger())
	s := testSession(1)
	close(s.Done)
	assert.Equal(t, -1, exec.waitForChoice(context.Background(), s))
}

func TestWaitForChoice_ContextCancel(t *testing.T) {
	exec := New(nil, nil, nopLogger())
	s := testSession(1)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	assert.Equal(t, -1, exec.waitForChoice(ctx, s))
}

// ========================================================================
// waitForEffectAck disconnect + context cancel paths
// ========================================================================

func TestWaitForEffectAck_Disconnect(t *testing.T) {
	exec := New(nil, nil, nopLogger())
	s := testSession(1)
	close(s.Done)
	assert.False(t, exec.waitForEffectAck(context.Background(), s, 221))
}

func TestWaitForEffectAck_ContextCancel(t *testing.T) {
	exec := New(nil, nil, nopLogger())
	s := testSession(1)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	assert.False(t, exec.waitForEffectAck(ctx, s, 221))
}

// ========================================================================
// transferPlayer with variable mode (mode=1)
// ========================================================================

func TestTransferPlayer_VarMode(t *testing.T) {
	gs := newMockGameState()
	gs.variables[10] = 5  // mapID
	gs.variables[11] = 20 // x
	gs.variables[12] = 30 // y

	exec := New(nil, &resource.ResourceLoader{}, nopLogger())
	s := testSession(1)

	var tMapID, tX, tY int
	opts := &ExecuteOpts{
		GameState: gs,
		MapID:     1,
		TransferFn: func(s *player.PlayerSession, mapID, x, y, dir int) {
			tMapID = mapID
			tX = x
			tY = y
		},
	}

	page := &resource.EventPage{
		List: []*resource.EventCommand{
			{Code: CmdTransfer, Indent: 0, Parameters: []interface{}{
				float64(1), float64(10), float64(11), float64(12), float64(2),
			}},
			{Code: CmdEnd, Indent: 0},
		},
	}
	exec.Execute(context.Background(), s, page, opts)
	assert.Equal(t, 5, tMapID)
	assert.Equal(t, 20, tX)
	assert.Equal(t, 30, tY)
}

// ========================================================================
// applyGold edge cases
// ========================================================================

func TestApplyGold_VariableOperand(t *testing.T) {
	store := newMockInventoryStore()
	store.gold[1] = 100
	gs := newMockGameState()
	gs.variables[5] = 50

	exec := New(store, &resource.ResourceLoader{}, nopLogger())
	s := testSession(1)
	opts := &ExecuteOpts{GameState: gs}

	// op=0(add), operandType=1(variable), operand=varID 5 → add 50
	page := &resource.EventPage{
		List: []*resource.EventCommand{
			{Code: CmdChangeGold, Indent: 0, Parameters: []interface{}{
				float64(0), float64(1), float64(5),
			}},
			{Code: CmdEnd, Indent: 0},
		},
	}
	exec.Execute(context.Background(), s, page, opts)
	assert.Equal(t, int64(150), store.gold[1])
}

func TestApplyGold_ClampToZero(t *testing.T) {
	store := newMockInventoryStore()
	store.gold[1] = 30

	exec := New(store, &resource.ResourceLoader{}, nopLogger())
	s := testSession(1)

	// op=1(decrease), operandType=0(const), operand=100 → clamp to -30
	page := &resource.EventPage{
		List: []*resource.EventCommand{
			{Code: CmdChangeGold, Indent: 0, Parameters: []interface{}{
				float64(1), float64(0), float64(100),
			}},
			{Code: CmdEnd, Indent: 0},
		},
	}
	exec.Execute(context.Background(), s, page, &ExecuteOpts{})
	assert.Equal(t, int64(0), store.gold[1])
}

func TestApplyGold_NilStore(t *testing.T) {
	exec := New(nil, &resource.ResourceLoader{}, nopLogger())
	s := testSession(1)

	page := &resource.EventPage{
		List: []*resource.EventCommand{
			{Code: CmdChangeGold, Indent: 0, Parameters: []interface{}{
				float64(0), float64(0), float64(100),
			}},
			{Code: CmdEnd, Indent: 0},
		},
	}
	// Should not panic
	exec.Execute(context.Background(), s, page, &ExecuteOpts{})
}

// ========================================================================
// applyItems edge cases
// ========================================================================

func TestApplyItems_RemoveClamp(t *testing.T) {
	store := newMockInventoryStore()
	store.items["1_10"] = 3

	exec := New(store, &resource.ResourceLoader{}, nopLogger())
	s := testSession(1)

	// Remove 100 of item 10 (only has 3) → clamp to 3
	page := &resource.EventPage{
		List: []*resource.EventCommand{
			{Code: CmdChangeItems, Indent: 0, Parameters: []interface{}{
				float64(10), float64(1), float64(0), float64(100),
			}},
			{Code: CmdEnd, Indent: 0},
		},
	}
	exec.Execute(context.Background(), s, page, &ExecuteOpts{})
	assert.Equal(t, 0, store.items["1_10"])
}

func TestApplyItems_AddExceedsMax(t *testing.T) {
	store := newMockInventoryStore()
	store.items["1_10"] = 9990

	exec := New(store, &resource.ResourceLoader{}, nopLogger())
	s := testSession(1)

	// Add 100 to item with 9990 → exceeds 9999
	page := &resource.EventPage{
		List: []*resource.EventCommand{
			{Code: CmdChangeItems, Indent: 0, Parameters: []interface{}{
				float64(10), float64(0), float64(0), float64(100),
			}},
			{Code: CmdEnd, Indent: 0},
		},
	}
	exec.Execute(context.Background(), s, page, &ExecuteOpts{})
	// Should NOT have added (exceeds max)
	assert.Equal(t, 9990, store.items["1_10"])
}

func TestApplyItems_VariableOperand(t *testing.T) {
	store := newMockInventoryStore()
	gs := newMockGameState()
	gs.variables[5] = 3

	exec := New(store, &resource.ResourceLoader{}, nopLogger())
	s := testSession(1)
	opts := &ExecuteOpts{GameState: gs}

	// Add item 10, operandType=1(var), var 5 = 3
	page := &resource.EventPage{
		List: []*resource.EventCommand{
			{Code: CmdChangeItems, Indent: 0, Parameters: []interface{}{
				float64(10), float64(0), float64(1), float64(5),
			}},
			{Code: CmdEnd, Indent: 0},
		},
	}
	exec.Execute(context.Background(), s, page, opts)
	assert.Equal(t, 3, store.items["1_10"])
}

// ========================================================================
// applyVariables: random operand, gamedata operand, script operand
// ========================================================================

func TestApplyVariables_Random(t *testing.T) {
	gs := newMockGameState()
	exec := New(nil, &resource.ResourceLoader{}, nopLogger())
	s := testSession(1)
	opts := &ExecuteOpts{GameState: gs}

	// op=0(set), operandType=2(random), min=5, max=5 → always 5
	page := &resource.EventPage{
		List: []*resource.EventCommand{
			{Code: CmdChangeVars, Indent: 0, Parameters: []interface{}{
				float64(1), float64(1), float64(0), float64(2), float64(5), float64(5),
			}},
			{Code: CmdEnd, Indent: 0},
		},
	}
	exec.Execute(context.Background(), s, page, opts)
	assert.Equal(t, 5, gs.variables[1])
}

func TestApplyVariables_GameData_MapID(t *testing.T) {
	gs := newMockGameState()
	exec := New(nil, &resource.ResourceLoader{}, nopLogger())
	s := testSession(1)
	opts := &ExecuteOpts{GameState: gs, MapID: 42}

	// op=0(set), operandType=3(gamedata), dataType=7(mapID)
	page := &resource.EventPage{
		List: []*resource.EventCommand{
			{Code: CmdChangeVars, Indent: 0, Parameters: []interface{}{
				float64(1), float64(1), float64(0), float64(3), float64(7), float64(0), float64(0),
			}},
			{Code: CmdEnd, Indent: 0},
		},
	}
	exec.Execute(context.Background(), s, page, opts)
	assert.Equal(t, 42, gs.variables[1])
}

func TestApplyVariables_GameData_PlayerPosition(t *testing.T) {
	gs := newMockGameState()
	exec := New(nil, &resource.ResourceLoader{}, nopLogger())
	s := testSession(1)
	s.SetPosition(10, 20, 4)
	opts := &ExecuteOpts{GameState: gs, MapID: 1}

	// dataType=5(character), param1=-1(player), param2=0(x)
	page := &resource.EventPage{
		List: []*resource.EventCommand{
			{Code: CmdChangeVars, Indent: 0, Parameters: []interface{}{
				float64(1), float64(1), float64(0), float64(3), float64(5), float64(-1), float64(0),
			}},
			{Code: CmdEnd, Indent: 0},
		},
	}
	exec.Execute(context.Background(), s, page, opts)
	assert.Equal(t, 10, gs.variables[1])

	// param2=1(y)
	s = testSession(1)
	s.SetPosition(10, 20, 4)
	page.List[0].Parameters = []interface{}{
		float64(2), float64(2), float64(0), float64(3), float64(5), float64(-1), float64(1),
	}
	exec.Execute(context.Background(), s, page, opts)
	assert.Equal(t, 20, gs.variables[2])

	// param2=2(dir)
	s = testSession(1)
	s.SetPosition(10, 20, 4)
	page.List[0].Parameters = []interface{}{
		float64(3), float64(3), float64(0), float64(3), float64(5), float64(-1), float64(2),
	}
	exec.Execute(context.Background(), s, page, opts)
	assert.Equal(t, 4, gs.variables[3])
}

// ========================================================================
// HP/MP/State/EXP/Level changes
// ========================================================================

func TestApplyChangeHP(t *testing.T) {
	exec := New(nil, &resource.ResourceLoader{}, nopLogger())
	s := testSessionWithStats(1, 100, 200, 0, 50, 10, 0)
	gs := newMockGameState()
	opts := &ExecuteOpts{GameState: gs}

	// Increase HP by 50
	page := &resource.EventPage{
		List: []*resource.EventCommand{
			{Code: CmdChangeHP, Indent: 0, Parameters: []interface{}{
				float64(0), float64(1), float64(0), float64(0), float64(50), float64(0),
			}},
			{Code: CmdEnd, Indent: 0},
		},
	}
	exec.Execute(context.Background(), s, page, opts)
	assert.Equal(t, 150, s.HP)

	// Decrease HP by 200, allowDeath=false → clamp to 1
	s = testSessionWithStats(1, 100, 200, 0, 50, 10, 0)
	page.List[0].Parameters = []interface{}{
		float64(0), float64(1), float64(1), float64(0), float64(200), float64(0),
	}
	exec.Execute(context.Background(), s, page, opts)
	assert.Equal(t, 1, s.HP)

	// Decrease HP by 200, allowDeath=true → clamp to 0
	s = testSessionWithStats(1, 100, 200, 0, 50, 10, 0)
	page.List[0].Parameters = []interface{}{
		float64(0), float64(1), float64(1), float64(0), float64(200), float64(1),
	}
	exec.Execute(context.Background(), s, page, opts)
	assert.Equal(t, 0, s.HP)

	// Increase beyond max → clamp to maxHP
	s = testSessionWithStats(1, 190, 200, 0, 50, 10, 0)
	page.List[0].Parameters = []interface{}{
		float64(0), float64(1), float64(0), float64(0), float64(50), float64(0),
	}
	exec.Execute(context.Background(), s, page, opts)
	assert.Equal(t, 200, s.HP)
}

func TestApplyChangeMP(t *testing.T) {
	exec := New(nil, &resource.ResourceLoader{}, nopLogger())
	s := testSessionWithStats(1, 200, 200, 30, 50, 10, 0)
	gs := newMockGameState()
	opts := &ExecuteOpts{GameState: gs}

	// Increase MP by 10
	page := &resource.EventPage{
		List: []*resource.EventCommand{
			{Code: CmdChangeMP, Indent: 0, Parameters: []interface{}{
				float64(0), float64(1), float64(0), float64(0), float64(10),
			}},
			{Code: CmdEnd, Indent: 0},
		},
	}
	exec.Execute(context.Background(), s, page, opts)
	assert.Equal(t, 40, s.MP)

	// Decrease MP below 0 → clamp to 0
	s = testSessionWithStats(1, 200, 200, 10, 50, 10, 0)
	page.List[0].Parameters = []interface{}{
		float64(0), float64(1), float64(1), float64(0), float64(20),
	}
	exec.Execute(context.Background(), s, page, opts)
	assert.Equal(t, 0, s.MP)

	// Increase MP beyond max → clamp
	s = testSessionWithStats(1, 200, 200, 45, 50, 10, 0)
	page.List[0].Parameters = []interface{}{
		float64(0), float64(1), float64(0), float64(0), float64(20),
	}
	exec.Execute(context.Background(), s, page, opts)
	assert.Equal(t, 50, s.MP)
}

func TestApplyChangeState(t *testing.T) {
	exec := New(nil, &resource.ResourceLoader{}, nopLogger())
	s := testSession(1)
	gs := newMockGameState()
	opts := &ExecuteOpts{GameState: gs}

	// Add state 5
	page := &resource.EventPage{
		List: []*resource.EventCommand{
			{Code: CmdChangeState, Indent: 0, Parameters: []interface{}{
				float64(0), float64(1), float64(0), float64(5),
			}},
			{Code: CmdEnd, Indent: 0},
		},
	}
	exec.Execute(context.Background(), s, page, opts)
	assert.True(t, s.HasState(5))

	// Remove state 5
	s = testSession(1)
	s.AddState(5)
	page.List[0].Parameters = []interface{}{float64(0), float64(1), float64(1), float64(5)}
	exec.Execute(context.Background(), s, page, opts)
	assert.False(t, s.HasState(5))
}

func TestApplyRecoverAll(t *testing.T) {
	exec := New(nil, &resource.ResourceLoader{}, nopLogger())
	s := testSessionWithStats(1, 50, 200, 10, 50, 10, 0)
	s.AddState(3)

	page := &resource.EventPage{
		List: []*resource.EventCommand{
			{Code: CmdRecoverAll, Indent: 0, Parameters: []interface{}{float64(0), float64(1)}},
			{Code: CmdEnd, Indent: 0},
		},
	}
	exec.Execute(context.Background(), s, page, &ExecuteOpts{})
	assert.Equal(t, 200, s.HP)
	assert.Equal(t, 50, s.MP)
	assert.False(t, s.HasState(3))
}

func TestApplyChangeEXP(t *testing.T) {
	exec := New(nil, &resource.ResourceLoader{}, nopLogger())
	s := testSessionWithStats(1, 200, 200, 50, 50, 10, 100)

	page := &resource.EventPage{
		List: []*resource.EventCommand{
			{Code: CmdChangeEXP, Indent: 0, Parameters: []interface{}{
				float64(0), float64(1), float64(0), float64(0), float64(50), float64(0),
			}},
			{Code: CmdEnd, Indent: 0},
		},
	}
	exec.Execute(context.Background(), s, page, &ExecuteOpts{})
	assert.Equal(t, int64(150), s.Exp)

	// Decrease below 0 → clamp to 0
	s = testSessionWithStats(1, 200, 200, 50, 50, 10, 30)
	page.List[0].Parameters = []interface{}{
		float64(0), float64(1), float64(1), float64(0), float64(100), float64(0),
	}
	exec.Execute(context.Background(), s, page, &ExecuteOpts{})
	assert.Equal(t, int64(0), s.Exp)
}

func TestApplyChangeLevel(t *testing.T) {
	exec := New(nil, &resource.ResourceLoader{}, nopLogger())
	s := testSessionWithStats(1, 200, 200, 50, 50, 10, 0)

	page := &resource.EventPage{
		List: []*resource.EventCommand{
			{Code: CmdChangeLevel, Indent: 0, Parameters: []interface{}{
				float64(0), float64(1), float64(0), float64(0), float64(5), float64(0),
			}},
			{Code: CmdEnd, Indent: 0},
		},
	}
	exec.Execute(context.Background(), s, page, &ExecuteOpts{})
	assert.Equal(t, 15, s.Level)

	// Decrease below 1 → clamp to 1
	s = testSessionWithStats(1, 200, 200, 50, 50, 3, 0)
	page.List[0].Parameters = []interface{}{
		float64(0), float64(1), float64(1), float64(0), float64(10), float64(0),
	}
	exec.Execute(context.Background(), s, page, &ExecuteOpts{})
	assert.Equal(t, 1, s.Level)
}

// ========================================================================
// Battle processing
// ========================================================================

func TestProcessBattle_WithBattleFn(t *testing.T) {
	exec := New(nil, &resource.ResourceLoader{}, nopLogger())
	s := testSession(1)
	gs := newMockGameState()

	battleCalled := false
	opts := &ExecuteOpts{
		GameState: gs,
		BattleFn: func(ctx context.Context, s *player.PlayerSession, troopID int, canEscape, canLose bool) int {
			battleCalled = true
			assert.Equal(t, 5, troopID)
			assert.True(t, canEscape)
			assert.False(t, canLose)
			return 0 // win
		},
	}

	page := &resource.EventPage{
		List: []*resource.EventCommand{
			{Code: CmdBattleProcessing, Indent: 0, Parameters: []interface{}{
				float64(0), float64(5), float64(1), float64(0),
			}},
			{Code: CmdEnd, Indent: 0},
		},
	}
	exec.Execute(context.Background(), s, page, opts)
	assert.True(t, battleCalled)
}

func TestProcessBattle_VarReference(t *testing.T) {
	exec := New(nil, &resource.ResourceLoader{}, nopLogger())
	s := testSession(1)
	gs := newMockGameState()
	gs.variables[10] = 7

	var gotTroopID int
	opts := &ExecuteOpts{
		GameState: gs,
		BattleFn: func(ctx context.Context, s *player.PlayerSession, troopID int, canEscape, canLose bool) int {
			gotTroopID = troopID
			return 0
		},
	}

	page := &resource.EventPage{
		List: []*resource.EventCommand{
			{Code: CmdBattleProcessing, Indent: 0, Parameters: []interface{}{
				float64(1), float64(10), float64(0), float64(0), // type=1(var), varID=10
			}},
			{Code: CmdEnd, Indent: 0},
		},
	}
	exec.Execute(context.Background(), s, page, opts)
	assert.Equal(t, 7, gotTroopID)
}

func TestProcessBattle_Branches(t *testing.T) {
	exec := New(nil, &resource.ResourceLoader{}, nopLogger())
	gs := newMockGameState()

	// Test escape branch (result=1)
	opts := &ExecuteOpts{
		GameState: gs,
		BattleFn: func(ctx context.Context, s *player.PlayerSession, troopID int, canEscape, canLose bool) int {
			return 1 // escape
		},
	}

	s := testSession(1)
	page := &resource.EventPage{
		List: []*resource.EventCommand{
			{Code: CmdBattleProcessing, Indent: 0, Parameters: []interface{}{
				float64(0), float64(1), float64(1), float64(1),
			}},
			{Code: CmdBattleWin, Indent: 0},
			{Code: CmdChangeVars, Indent: 1, Parameters: []interface{}{
				float64(1), float64(1), float64(0), float64(0), float64(10),
			}},
			{Code: CmdBattleEscape, Indent: 0},
			{Code: CmdChangeVars, Indent: 1, Parameters: []interface{}{
				float64(2), float64(2), float64(0), float64(0), float64(20),
			}},
			{Code: CmdBattleLose, Indent: 0},
			{Code: CmdChangeVars, Indent: 1, Parameters: []interface{}{
				float64(3), float64(3), float64(0), float64(0), float64(30),
			}},
			{Code: CmdEnd, Indent: 0},
		},
	}
	exec.Execute(context.Background(), s, page, opts)
	assert.Equal(t, 0, gs.variables[1], "Win branch should be skipped")
	assert.Equal(t, 20, gs.variables[2], "Escape branch should execute")
	assert.Equal(t, 0, gs.variables[3], "Lose branch should be skipped")
}

// ========================================================================
// Shop processing
// ========================================================================

func TestShopProcessing(t *testing.T) {
	exec := New(nil, &resource.ResourceLoader{}, nopLogger())
	s := testSession(1)

	page := &resource.EventPage{
		List: []*resource.EventCommand{
			{Code: CmdShopProcessing, Indent: 0, Parameters: []interface{}{float64(0), float64(1), float64(0)}},
			{Code: CmdShopItem, Indent: 0, Parameters: []interface{}{float64(1), float64(5), float64(0)}},
			{Code: CmdShopItem, Indent: 0, Parameters: []interface{}{float64(1), float64(10), float64(0)}},
			{Code: CmdEnd, Indent: 0},
		},
	}
	exec.Execute(context.Background(), s, page, &ExecuteOpts{})

	pkts := drainPackets(t, s)
	found := false
	for _, pkt := range pkts {
		if pkt.Type == "npc_effect" {
			var data map[string]interface{}
			json.Unmarshal(pkt.Payload, &data)
			if data["code"] == float64(CmdShopProcessing) {
				found = true
				goods := data["shop_goods"].([]interface{})
				assert.Len(t, goods, 2)
			}
		}
	}
	assert.True(t, found)
	assert.Len(t, s.ShopGoods, 3) // first + 2 extras
}

// ========================================================================
// Dispatch: screen effects with wait flags
// ========================================================================

func TestDispatch_TintScreen_Wait(t *testing.T) {
	exec := New(nil, &resource.ResourceLoader{}, nopLogger())
	s := testSession(1)

	page := &resource.EventPage{
		List: []*resource.EventCommand{
			{Code: CmdTintScreen, Indent: 0, Parameters: []interface{}{
				[]interface{}{float64(0), float64(0), float64(0), float64(0)}, float64(60), true,
			}},
			{Code: CmdEnd, Indent: 0},
		},
	}

	go func() {
		time.Sleep(50 * time.Millisecond)
		s.EffectAckCh <- struct{}{}
	}()
	exec.Execute(context.Background(), s, page, &ExecuteOpts{})
	pkts := drainPackets(t, s)
	assert.True(t, len(pkts) >= 1)
}

func TestDispatch_FlashScreen_Wait(t *testing.T) {
	exec := New(nil, &resource.ResourceLoader{}, nopLogger())
	s := testSession(1)

	page := &resource.EventPage{
		List: []*resource.EventCommand{
			{Code: CmdFlashScreen, Indent: 0, Parameters: []interface{}{
				[]interface{}{float64(255), float64(255), float64(255), float64(170)}, float64(8), true,
			}},
			{Code: CmdEnd, Indent: 0},
		},
	}

	go func() {
		time.Sleep(50 * time.Millisecond)
		s.EffectAckCh <- struct{}{}
	}()
	exec.Execute(context.Background(), s, page, &ExecuteOpts{})
}

func TestDispatch_ShakeScreen_Wait(t *testing.T) {
	exec := New(nil, &resource.ResourceLoader{}, nopLogger())
	s := testSession(1)

	page := &resource.EventPage{
		List: []*resource.EventCommand{
			{Code: CmdShakeScreen, Indent: 0, Parameters: []interface{}{
				float64(5), float64(9), float64(60), true,
			}},
			{Code: CmdEnd, Indent: 0},
		},
	}

	go func() {
		time.Sleep(50 * time.Millisecond)
		s.EffectAckCh <- struct{}{}
	}()
	exec.Execute(context.Background(), s, page, &ExecuteOpts{})
}

func TestDispatch_ShowAnimation_Wait(t *testing.T) {
	exec := New(nil, &resource.ResourceLoader{}, nopLogger())
	s := testSession(1)

	page := &resource.EventPage{
		List: []*resource.EventCommand{
			{Code: CmdShowAnimation, Indent: 0, Parameters: []interface{}{
				float64(0), float64(1), true,
			}},
			{Code: CmdEnd, Indent: 0},
		},
	}

	go func() {
		time.Sleep(50 * time.Millisecond)
		s.EffectAckCh <- struct{}{}
	}()
	exec.Execute(context.Background(), s, page, &ExecuteOpts{EventID: 5})
}

func TestDispatch_ShowBalloon_Wait(t *testing.T) {
	exec := New(nil, &resource.ResourceLoader{}, nopLogger())
	s := testSession(1)

	page := &resource.EventPage{
		List: []*resource.EventCommand{
			{Code: CmdShowBalloon, Indent: 0, Parameters: []interface{}{
				float64(0), float64(1), true,
			}},
			{Code: CmdEnd, Indent: 0},
		},
	}

	go func() {
		time.Sleep(50 * time.Millisecond)
		s.EffectAckCh <- struct{}{}
	}()
	exec.Execute(context.Background(), s, page, &ExecuteOpts{EventID: 5})
}

func TestDispatch_TintPicture_Wait(t *testing.T) {
	exec := New(nil, &resource.ResourceLoader{}, nopLogger())
	s := testSession(1)

	page := &resource.EventPage{
		List: []*resource.EventCommand{
			{Code: CmdTintPicture, Indent: 0, Parameters: []interface{}{
				float64(1),
				[]interface{}{float64(0), float64(0), float64(0), float64(0)},
				float64(60),
				true,
			}},
			{Code: CmdEnd, Indent: 0},
		},
	}

	go func() {
		time.Sleep(50 * time.Millisecond)
		s.EffectAckCh <- struct{}{}
	}()
	exec.Execute(context.Background(), s, page, &ExecuteOpts{})
}

func TestDispatch_SetWeather_Wait(t *testing.T) {
	exec := New(nil, &resource.ResourceLoader{}, nopLogger())
	s := testSession(1)

	page := &resource.EventPage{
		List: []*resource.EventCommand{
			{Code: CmdSetWeather, Indent: 0, Parameters: []interface{}{
				"rain", float64(5), float64(60), true,
			}},
			{Code: CmdEnd, Indent: 0},
		},
	}

	go func() {
		time.Sleep(50 * time.Millisecond)
		s.EffectAckCh <- struct{}{}
	}()
	exec.Execute(context.Background(), s, page, &ExecuteOpts{})
}

// ========================================================================
// Dispatch: various forward-only codes
// ========================================================================

func TestDispatch_ForwardOnlyCodes(t *testing.T) {
	exec := New(nil, &resource.ResourceLoader{}, nopLogger())

	codes := []int{
		CmdChangeTransparency, CmdPlayBGM, CmdStopBGM, CmdPlayBGS, CmdStopBGS,
		CmdPlaySE, CmdStopSE, CmdPlayME, CmdRotatePicture, CmdErasePicture,
		CmdEraseEvent, CmdChangeParameter, CmdChangeSkill, CmdChangeName,
		CmdChangeActorImage, CmdGameOver, CmdReturnToTitle,
	}

	for _, code := range codes {
		s := testSession(1)
		page := &resource.EventPage{
			List: []*resource.EventCommand{
				{Code: code, Indent: 0, Parameters: []interface{}{float64(1)}},
				{Code: CmdEnd, Indent: 0},
			},
		}
		exec.Execute(context.Background(), s, page, &ExecuteOpts{})
		pkts := drainPackets(t, s)
		effectFound := false
		for _, pkt := range pkts {
			if pkt.Type == "npc_effect" {
				effectFound = true
			}
		}
		assert.True(t, effectFound, "code %d should forward as npc_effect", code)
	}
}

// ========================================================================
// Dispatch: SetMoveRoute with wait
// ========================================================================

func TestDispatch_SetMoveRoute_Wait(t *testing.T) {
	exec := New(nil, &resource.ResourceLoader{}, nopLogger())
	s := testSession(1)

	page := &resource.EventPage{
		List: []*resource.EventCommand{
			{Code: CmdSetMoveRoute, Indent: 0, Parameters: []interface{}{
				float64(1),
				map[string]interface{}{"wait": true, "list": []interface{}{}},
			}},
			{Code: CmdMoveRouteCont, Indent: 0, Parameters: []interface{}{float64(0)}},
			{Code: CmdEnd, Indent: 0},
		},
	}

	go func() {
		time.Sleep(50 * time.Millisecond)
		s.EffectAckCh <- struct{}{}
	}()
	exec.Execute(context.Background(), s, page, &ExecuteOpts{EventID: 5})
}

func TestDispatch_WaitForMoveRoute(t *testing.T) {
	exec := New(nil, &resource.ResourceLoader{}, nopLogger())
	s := testSession(1)

	page := &resource.EventPage{
		List: []*resource.EventCommand{
			{Code: CmdWaitForMoveRoute, Indent: 0, Parameters: []interface{}{}},
			{Code: CmdEnd, Indent: 0},
		},
	}

	go func() {
		time.Sleep(50 * time.Millisecond)
		s.EffectAckCh <- struct{}{}
	}()
	exec.Execute(context.Background(), s, page, &ExecuteOpts{})
}

// ========================================================================
// Dispatch: EquipChange plugin command
// ========================================================================

func TestDispatch_EquipChange(t *testing.T) {
	store := newMockInventoryStore()
	exec := New(store, &resource.ResourceLoader{}, nopLogger())
	s := testSession(1)
	gs := newMockGameState()
	opts := &ExecuteOpts{GameState: gs}

	page := &resource.EventPage{
		List: []*resource.EventCommand{
			{Code: CmdPluginCommand, Indent: 0, Parameters: []interface{}{"EquipChange Cloth 55"}},
			{Code: CmdEnd, Indent: 0},
		},
	}
	exec.Execute(context.Background(), s, page, opts)

	// Check session equip updated
	equips := s.Equips
	assert.Equal(t, 55, equips[1]) // Cloth → slot 1

	// Check var sync
	assert.Equal(t, 1, gs.variables[2701])  // slot index
	assert.Equal(t, 55, gs.variables[2703]) // armor ID
}

// ========================================================================
// sendStateBatch coverage
// ========================================================================

func TestSendStateBatch(t *testing.T) {
	exec := New(nil, nil, nopLogger())
	s := testSession(1)

	// Empty → no packet
	exec.sendStateBatch(s, nil, nil)
	pkts := drainPackets(t, s)
	assert.Len(t, pkts, 0)

	// With data
	exec.sendStateBatch(s,
		map[int]int{10: 42, 20: 99},
		map[int]bool{5: true, 6: false},
	)
	pkts = drainPackets(t, s)
	require.Len(t, pkts, 1)
	assert.Equal(t, "state_batch", pkts[0].Type)
}

// ========================================================================
// Parallel event: stepUntilWait coverage
// ========================================================================

func TestStepUntilWait_BasicFlow(t *testing.T) {
	gs := newMockGameState()
	exec := New(nil, &resource.ResourceLoader{}, nopLogger())
	s := testSession(1)
	s.MapID = 1

	ev := NewParallelEventState(1, []*resource.EventCommand{
		{Code: CmdChangeVars, Indent: 0, Parameters: []interface{}{
			float64(1), float64(1), float64(0), float64(0), float64(42),
		}},
		{Code: CmdWait, Indent: 0, Parameters: []interface{}{float64(10)}},
		{Code: CmdEnd, Indent: 0},
	}, 3)

	opts := &ExecuteOpts{GameState: gs, MapID: 1, EventID: 1}

	// First step: execute var change, hit Wait, return false
	done := exec.stepUntilWait(context.Background(), s, ev, opts)
	assert.False(t, done)
	assert.Equal(t, 42, gs.variables[1])
	assert.Equal(t, 10, ev.waitFrames)

	// Continue after wait: hit End, return true
	ev.waitFrames = 0
	done = exec.stepUntilWait(context.Background(), s, ev, opts)
	assert.True(t, done)
}

func TestStepUntilWait_LoopAndBreak(t *testing.T) {
	gs := newMockGameState()
	exec := New(nil, &resource.ResourceLoader{}, nopLogger())
	s := testSession(1)
	s.MapID = 1

	ev := NewParallelEventState(1, []*resource.EventCommand{
		{Code: CmdLoop, Indent: 0},
		{Code: CmdChangeVars, Indent: 1, Parameters: []interface{}{
			float64(1), float64(1), float64(1), float64(0), float64(1),
		}},
		{Code: CmdConditionalStart, Indent: 1, Parameters: []interface{}{
			float64(1), float64(1), float64(0), float64(3), float64(1),
		}},
		{Code: CmdBreakLoop, Indent: 2},
		{Code: CmdConditionalEnd, Indent: 1},
		{Code: CmdRepeatAbove, Indent: 0},
		{Code: CmdEnd, Indent: 0},
	}, 3)

	opts := &ExecuteOpts{GameState: gs, MapID: 1, EventID: 1}
	done := exec.stepUntilWait(context.Background(), s, ev, opts)
	assert.True(t, done)
	assert.Equal(t, 3, gs.variables[1])
}

func TestStepUntilWait_MapChanged(t *testing.T) {
	gs := newMockGameState()
	exec := New(nil, &resource.ResourceLoader{}, nopLogger())
	s := testSession(1)
	s.MapID = 2 // different from opts.MapID

	ev := NewParallelEventState(1, []*resource.EventCommand{
		{Code: CmdChangeVars, Indent: 0, Parameters: []interface{}{
			float64(1), float64(1), float64(0), float64(0), float64(99),
		}},
		{Code: CmdEnd, Indent: 0},
	}, 3)

	opts := &ExecuteOpts{GameState: gs, MapID: 1, EventID: 1}
	done := exec.stepUntilWait(context.Background(), s, ev, opts)
	assert.True(t, done)
	assert.Equal(t, 0, gs.variables[1]) // should not have executed
}

func TestStepUntilWait_ContextCancel(t *testing.T) {
	exec := New(nil, &resource.ResourceLoader{}, nopLogger())
	s := testSession(1)
	s.MapID = 1

	ev := NewParallelEventState(1, []*resource.EventCommand{
		{Code: CmdChangeVars, Indent: 0, Parameters: []interface{}{
			float64(1), float64(1), float64(0), float64(0), float64(99),
		}},
		{Code: CmdEnd, Indent: 0},
	}, 3)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	opts := &ExecuteOpts{GameState: newMockGameState(), MapID: 1, EventID: 1}
	done := exec.stepUntilWait(ctx, s, ev, opts)
	assert.True(t, done)
}

func TestStepUntilWait_NilCmd(t *testing.T) {
	exec := New(nil, &resource.ResourceLoader{}, nopLogger())
	s := testSession(1)
	s.MapID = 1

	ev := NewParallelEventState(1, []*resource.EventCommand{
		nil,
		{Code: CmdEnd, Indent: 0},
	}, 3)

	opts := &ExecuteOpts{GameState: newMockGameState(), MapID: 1, EventID: 1}
	done := exec.stepUntilWait(context.Background(), s, ev, opts)
	assert.True(t, done)
}

func TestStepUntilWait_ExitEvent(t *testing.T) {
	exec := New(nil, &resource.ResourceLoader{}, nopLogger())
	s := testSession(1)
	s.MapID = 1

	ev := NewParallelEventState(1, []*resource.EventCommand{
		{Code: CmdExitEvent, Indent: 0},
		{Code: CmdEnd, Indent: 0},
	}, 3)

	opts := &ExecuteOpts{GameState: newMockGameState(), MapID: 1, EventID: 1}
	done := exec.stepUntilWait(context.Background(), s, ev, opts)
	assert.True(t, done)
}

func TestStepUntilWait_ElseBranch(t *testing.T) {
	gs := newMockGameState()
	exec := New(nil, &resource.ResourceLoader{}, nopLogger())
	s := testSession(1)
	s.MapID = 1

	ev := NewParallelEventState(1, []*resource.EventCommand{
		{Code: CmdConditionalStart, Indent: 0, Parameters: []interface{}{
			float64(0), float64(1), float64(0), // switch 1 = ON?
		}},
		{Code: CmdChangeVars, Indent: 1, Parameters: []interface{}{
			float64(1), float64(1), float64(0), float64(0), float64(10),
		}},
		{Code: CmdElseBranch, Indent: 0},
		{Code: CmdChangeVars, Indent: 1, Parameters: []interface{}{
			float64(1), float64(1), float64(0), float64(0), float64(20),
		}},
		{Code: CmdConditionalEnd, Indent: 0},
		{Code: CmdEnd, Indent: 0},
	}, 3)

	// Switch 1 is ON → if branch, then ElseBranch skips to ConditionalEnd
	gs.switches[1] = true
	opts := &ExecuteOpts{GameState: gs, MapID: 1, EventID: 1}
	done := exec.stepUntilWait(context.Background(), s, ev, opts)
	assert.True(t, done)
	assert.Equal(t, 10, gs.variables[1])
}

func TestStepUntilWait_LabelJump(t *testing.T) {
	gs := newMockGameState()
	exec := New(nil, &resource.ResourceLoader{}, nopLogger())
	s := testSession(1)
	s.MapID = 1

	ev := NewParallelEventState(1, []*resource.EventCommand{
		{Code: CmdJumpToLabel, Indent: 0, Parameters: []interface{}{"end"}},
		{Code: CmdChangeVars, Indent: 0, Parameters: []interface{}{
			float64(1), float64(1), float64(0), float64(0), float64(99),
		}},
		{Code: CmdLabel, Indent: 0, Parameters: []interface{}{"end"}},
		{Code: CmdEnd, Indent: 0},
	}, 3)

	opts := &ExecuteOpts{GameState: gs, MapID: 1, EventID: 1}
	done := exec.stepUntilWait(context.Background(), s, ev, opts)
	assert.True(t, done)
	assert.Equal(t, 0, gs.variables[1]) // skipped
}

func TestStepUntilWait_Comment(t *testing.T) {
	exec := New(nil, &resource.ResourceLoader{}, nopLogger())
	s := testSession(1)
	s.MapID = 1

	ev := NewParallelEventState(1, []*resource.EventCommand{
		{Code: CmdComment, Indent: 0, Parameters: []interface{}{"hello"}},
		{Code: CmdCommentCont, Indent: 0, Parameters: []interface{}{"world"}},
		{Code: CmdEnd, Indent: 0},
	}, 3)

	opts := &ExecuteOpts{GameState: newMockGameState(), MapID: 1, EventID: 1}
	done := exec.stepUntilWait(context.Background(), s, ev, opts)
	assert.True(t, done)
}

func TestStepUntilWait_SetMoveRoute(t *testing.T) {
	exec := New(nil, &resource.ResourceLoader{}, nopLogger())
	s := testSession(1)
	s.MapID = 1

	ev := NewParallelEventState(1, []*resource.EventCommand{
		{Code: CmdSetMoveRoute, Indent: 0, Parameters: []interface{}{
			float64(1), map[string]interface{}{"list": []interface{}{}, "wait": false},
		}},
		{Code: CmdMoveRouteCont, Indent: 0, Parameters: []interface{}{float64(0)}},
		{Code: CmdEnd, Indent: 0},
	}, 3)

	opts := &ExecuteOpts{GameState: newMockGameState(), MapID: 1, EventID: 1}
	done := exec.stepUntilWait(context.Background(), s, ev, opts)
	assert.True(t, done)
}

func TestStepUntilWait_SetMoveRoute_PlayerSpeed(t *testing.T) {
	exec := New(nil, &resource.ResourceLoader{}, nopLogger())
	s := testSession(1)
	s.MapID = 1

	ev := NewParallelEventState(1, []*resource.EventCommand{
		{Code: CmdSetMoveRoute, Indent: 0, Parameters: []interface{}{
			float64(-1), map[string]interface{}{"list": []interface{}{
				map[string]interface{}{"code": float64(1)},
			}, "wait": false},
		}},
		{Code: CmdEnd, Indent: 0},
	}, 4)

	opts := &ExecuteOpts{GameState: newMockGameState(), MapID: 1, EventID: 1}
	done := exec.stepUntilWait(context.Background(), s, ev, opts)
	assert.True(t, done)

	// Check speed injection in effect
	pkts := drainPackets(t, s)
	assert.True(t, len(pkts) >= 1)
}

func TestStepUntilWait_DefaultForward(t *testing.T) {
	exec := New(nil, &resource.ResourceLoader{}, nopLogger())
	s := testSession(1)
	s.MapID = 1

	ev := NewParallelEventState(1, []*resource.EventCommand{
		{Code: CmdPlaySE, Indent: 0, Parameters: []interface{}{
			map[string]interface{}{"name": "Test", "volume": float64(80)},
		}},
		{Code: CmdEnd, Indent: 0},
	}, 3)

	opts := &ExecuteOpts{GameState: newMockGameState(), MapID: 1, EventID: 1}
	done := exec.stepUntilWait(context.Background(), s, ev, opts)
	assert.True(t, done)
	pkts := drainPackets(t, s)
	assert.True(t, len(pkts) >= 1)
}

// ========================================================================
// Parallel event: ShowPicture/MovePicture in stepUntilWait
// ========================================================================

func TestStepUntilWait_ShowPicture(t *testing.T) {
	gs := newMockGameState()
	gs.variables[10] = 100
	gs.variables[11] = 200
	exec := New(nil, &resource.ResourceLoader{}, nopLogger())
	s := testSession(1)
	s.MapID = 1

	ev := NewParallelEventState(1, []*resource.EventCommand{
		{Code: CmdShowPicture, Indent: 0, Parameters: []interface{}{
			float64(1), "pic", float64(0), float64(1), float64(10), float64(11),
			float64(100), float64(100), float64(255), float64(0),
		}},
		{Code: CmdEnd, Indent: 0},
	}, 3)

	opts := &ExecuteOpts{GameState: gs, MapID: 1, EventID: 1}
	exec.stepUntilWait(context.Background(), s, ev, opts)
}

func TestStepUntilWait_MovePicture(t *testing.T) {
	exec := New(nil, &resource.ResourceLoader{}, nopLogger())
	s := testSession(1)
	s.MapID = 1

	ev := NewParallelEventState(1, []*resource.EventCommand{
		{Code: CmdMovePicture, Indent: 0, Parameters: []interface{}{
			float64(1), nil, float64(0), float64(0), float64(100), float64(200),
			float64(100), float64(100), float64(255), float64(0), float64(30), false,
		}},
		{Code: CmdEnd, Indent: 0},
	}, 3)

	opts := &ExecuteOpts{GameState: newMockGameState(), MapID: 1, EventID: 1}
	exec.stepUntilWait(context.Background(), s, ev, opts)
}

// ========================================================================
// Parallel: ChangeGold/Items/HP/MP/State/EXP/Level/Class in stepUntilWait
// ========================================================================

func TestStepUntilWait_StateChanges(t *testing.T) {
	store := newMockInventoryStore()
	store.gold[1] = 100
	gs := newMockGameState()
	exec := New(store, &resource.ResourceLoader{}, nopLogger())
	s := testSessionWithStats(1, 100, 200, 20, 50, 10, 100)
	s.MapID = 1

	ev := NewParallelEventState(1, []*resource.EventCommand{
		{Code: CmdChangeGold, Indent: 0, Parameters: []interface{}{float64(0), float64(0), float64(50)}},
		{Code: CmdChangeItems, Indent: 0, Parameters: []interface{}{float64(5), float64(0), float64(0), float64(3)}},
		{Code: CmdChangeHP, Indent: 0, Parameters: []interface{}{float64(0), float64(1), float64(0), float64(0), float64(10), float64(0)}},
		{Code: CmdChangeMP, Indent: 0, Parameters: []interface{}{float64(0), float64(1), float64(0), float64(0), float64(5)}},
		{Code: CmdChangeState, Indent: 0, Parameters: []interface{}{float64(0), float64(1), float64(0), float64(3)}},
		{Code: CmdRecoverAll, Indent: 0, Parameters: []interface{}{float64(0), float64(1)}},
		{Code: CmdChangeEXP, Indent: 0, Parameters: []interface{}{float64(0), float64(1), float64(0), float64(0), float64(50), float64(0)}},
		{Code: CmdChangeLevel, Indent: 0, Parameters: []interface{}{float64(0), float64(1), float64(0), float64(0), float64(2), float64(0)}},
		{Code: CmdEnd, Indent: 0},
	}, 3)

	opts := &ExecuteOpts{GameState: gs, MapID: 1, EventID: 1}
	done := exec.stepUntilWait(context.Background(), s, ev, opts)
	assert.True(t, done)
	assert.Equal(t, int64(150), store.gold[1])
	assert.Equal(t, 12, s.Level)
}

// ========================================================================
// Parallel: SetEventLocation, ShowAnimation, ShowBalloon
// ========================================================================

func TestStepUntilWait_CharIDResolve(t *testing.T) {
	exec := New(nil, &resource.ResourceLoader{}, nopLogger())
	s := testSession(1)
	s.MapID = 1

	ev := NewParallelEventState(1, []*resource.EventCommand{
		{Code: CmdSetEventLocation, Indent: 0, Parameters: []interface{}{float64(0), float64(5), float64(10)}},
		{Code: CmdShowAnimation, Indent: 0, Parameters: []interface{}{float64(0), float64(1), false}},
		{Code: CmdShowBalloon, Indent: 0, Parameters: []interface{}{float64(0), float64(2), false}},
		{Code: CmdEnd, Indent: 0},
	}, 3)

	opts := &ExecuteOpts{GameState: newMockGameState(), MapID: 1, EventID: 5}
	exec.stepUntilWait(context.Background(), s, ev, opts)
	pkts := drainPackets(t, s)
	assert.Equal(t, 3, len(pkts))
}

// ========================================================================
// Parallel: Plugin commands (EnterInstance, LeaveInstance, blocked, CallCommon)
// ========================================================================

func TestStepUntilWait_PluginCommands(t *testing.T) {
	exec := New(nil, &resource.ResourceLoader{CommonEvents: make([]*resource.CommonEvent, 1)}, nopLogger())
	s := testSession(1)
	s.MapID = 1

	enterCalled := false
	leaveCalled := false

	ev := NewParallelEventState(1, []*resource.EventCommand{
		{Code: CmdPluginCommand, Indent: 0, Parameters: []interface{}{"EnterInstance"}},
		{Code: CmdPluginCommand, Indent: 0, Parameters: []interface{}{"LeaveInstance"}},
		{Code: CmdPluginCommand, Indent: 0, Parameters: []interface{}{"CallStand arg1"}},
		{Code: CmdPluginCommand, Indent: 0, Parameters: []interface{}{"SomeForwardCmd"}},
		{Code: CmdEnd, Indent: 0},
	}, 3)

	opts := &ExecuteOpts{
		GameState:       newMockGameState(),
		MapID:           1,
		EventID:         1,
		EnterInstanceFn: func(s *player.PlayerSession) { enterCalled = true },
		LeaveInstanceFn: func(s *player.PlayerSession) { leaveCalled = true },
	}
	exec.stepUntilWait(context.Background(), s, ev, opts)
	assert.True(t, enterCalled)
	assert.True(t, leaveCalled)
}

// ========================================================================
// Parallel: Script commands
// ========================================================================

func TestStepUntilWait_Script(t *testing.T) {
	gs := newMockGameState()
	exec := New(nil, &resource.ResourceLoader{}, nopLogger())
	s := testSession(1)
	s.MapID = 1

	ev := NewParallelEventState(1, []*resource.EventCommand{
		{Code: CmdScript, Indent: 0, Parameters: []interface{}{"$gameVariables._data[1] = 42"}},
		{Code: CmdScriptCont, Indent: 0, Parameters: []interface{}{"// continued"}},
		{Code: CmdEnd, Indent: 0},
	}, 3)

	opts := &ExecuteOpts{GameState: gs, MapID: 1, EventID: 1}
	exec.stepUntilWait(context.Background(), s, ev, opts)
	assert.Equal(t, 42, gs.variables[1])
}

func TestStepUntilWait_ScriptSafeForward(t *testing.T) {
	exec := New(nil, &resource.ResourceLoader{}, nopLogger())
	s := testSession(1)
	s.MapID = 1

	ev := NewParallelEventState(1, []*resource.EventCommand{
		{Code: CmdScript, Indent: 0, Parameters: []interface{}{"$gameScreen.startTint([0,0,0,0], 60)"}},
		{Code: CmdEnd, Indent: 0},
	}, 3)

	opts := &ExecuteOpts{GameState: newMockGameState(), MapID: 1, EventID: 1}
	exec.stepUntilWait(context.Background(), s, ev, opts)
	pkts := drainPackets(t, s)
	effectFound := false
	for _, pkt := range pkts {
		if pkt.Type == "npc_effect" {
			effectFound = true
		}
	}
	assert.True(t, effectFound)
}

// ========================================================================
// Parallel: CmdEnd with indent > 0 (not terminal)
// ========================================================================

func TestStepUntilWait_CmdEndNonTerminal(t *testing.T) {
	exec := New(nil, &resource.ResourceLoader{}, nopLogger())
	gs := newMockGameState()
	s := testSession(1)
	s.MapID = 1

	ev := NewParallelEventState(1, []*resource.EventCommand{
		{Code: CmdEnd, Indent: 1}, // non-terminal (inside a block)
		{Code: CmdChangeVars, Indent: 0, Parameters: []interface{}{
			float64(1), float64(1), float64(0), float64(0), float64(7),
		}},
		{Code: CmdEnd, Indent: 0}, // terminal
	}, 3)

	opts := &ExecuteOpts{GameState: gs, MapID: 1, EventID: 1}
	done := exec.stepUntilWait(context.Background(), s, ev, opts)
	assert.True(t, done)
	assert.Equal(t, 7, gs.variables[1])
}

// ========================================================================
// Parallel: CallCommonEvent
// ========================================================================

func TestStepUntilWait_CallCommonEvent(t *testing.T) {
	gs := newMockGameState()
	rl := &resource.ResourceLoader{
		CommonEvents: []*resource.CommonEvent{
			nil,
			{ID: 1, Name: "CE1", List: []*resource.EventCommand{
				{Code: CmdChangeVars, Indent: 0, Parameters: []interface{}{
					float64(1), float64(1), float64(0), float64(0), float64(88),
				}},
				{Code: CmdEnd, Indent: 0},
			}},
		},
	}
	exec := New(nil, rl, nopLogger())
	s := testSession(1)
	s.MapID = 1

	ev := NewParallelEventState(1, []*resource.EventCommand{
		{Code: CmdCallCommonEvent, Indent: 0, Parameters: []interface{}{float64(1)}},
		{Code: CmdEnd, Indent: 0},
	}, 3)

	opts := &ExecuteOpts{GameState: gs, MapID: 1, EventID: 1}
	exec.stepUntilWait(context.Background(), s, ev, opts)
	assert.Equal(t, 88, gs.variables[1])
}

// ========================================================================
// TE_CALL_ORIGIN_EVENT edge cases
// ========================================================================

func TestHandleTECallOriginEvent_DebugDisplay(t *testing.T) {
	exec := New(nil, &resource.ResourceLoader{}, nopLogger())
	s := testSession(1)

	cmd := &resource.EventCommand{Code: CmdPluginCommand, Parameters: []interface{}{"TE関連データ値デバッグ表示"}}
	handled := exec.handleTECallOriginEvent(context.Background(), s, cmd, &ExecuteOpts{}, 0)
	assert.True(t, handled)
}

func TestHandleTECallOriginEvent_Empty(t *testing.T) {
	exec := New(nil, &resource.ResourceLoader{}, nopLogger())
	s := testSession(1)

	cmd := &resource.EventCommand{Code: CmdPluginCommand, Parameters: []interface{}{}}
	handled := exec.handleTECallOriginEvent(context.Background(), s, cmd, &ExecuteOpts{}, 0)
	assert.False(t, handled)

	cmd2 := &resource.EventCommand{Code: CmdPluginCommand, Parameters: []interface{}{""}}
	handled2 := exec.handleTECallOriginEvent(context.Background(), s, cmd2, &ExecuteOpts{}, 0)
	assert.False(t, handled2)
}

// ========================================================================
// resolveCharIDCommand
// ========================================================================

func TestResolveCharIDCommand(t *testing.T) {
	exec := New(nil, nil, nopLogger())

	// charId=0, eventID=5 → resolved to 5
	cmd := &resource.EventCommand{Code: CmdShowAnimation, Parameters: []interface{}{float64(0), float64(1)}}
	resolved := exec.resolveCharIDCommand(cmd, &ExecuteOpts{EventID: 5})
	assert.Equal(t, float64(5), resolved.Parameters[0])

	// charId=3 → no change
	cmd2 := &resource.EventCommand{Code: CmdShowAnimation, Parameters: []interface{}{float64(3), float64(1)}}
	resolved2 := exec.resolveCharIDCommand(cmd2, &ExecuteOpts{EventID: 5})
	assert.Equal(t, float64(3), resolved2.Parameters[0])

	// charId=0, nil opts → no change
	resolved3 := exec.resolveCharIDCommand(cmd, nil)
	assert.Equal(t, float64(0), resolved3.Parameters[0])
}

// ========================================================================
// Dispatch: EnterInstance/LeaveInstance in executeList
// ========================================================================

func TestDispatch_InstanceCommands(t *testing.T) {
	exec := New(nil, &resource.ResourceLoader{}, nopLogger())
	s := testSession(1)

	enterCalled := false
	leaveCalled := false

	page := &resource.EventPage{
		List: []*resource.EventCommand{
			{Code: CmdPluginCommand, Indent: 0, Parameters: []interface{}{"EnterInstance"}},
			{Code: CmdPluginCommand, Indent: 0, Parameters: []interface{}{"LeaveInstance"}},
			{Code: CmdEnd, Indent: 0},
		},
	}

	opts := &ExecuteOpts{
		EnterInstanceFn: func(s *player.PlayerSession) { enterCalled = true },
		LeaveInstanceFn: func(s *player.PlayerSession) { leaveCalled = true },
	}
	exec.Execute(context.Background(), s, page, opts)
	assert.True(t, enterCalled)
	assert.True(t, leaveCalled)
}

// ========================================================================
// Dispatch: blocked plugin commands
// ========================================================================

func TestDispatch_BlockedPluginCmds(t *testing.T) {
	exec := New(nil, testResMMO(), nopLogger())
	s := testSession(1)

	page := &resource.EventPage{
		List: []*resource.EventCommand{
			{Code: CmdPluginCommand, Indent: 0, Parameters: []interface{}{"CallStand arg1 arg2"}},
			{Code: CmdPluginCommand, Indent: 0, Parameters: []interface{}{"EraceStand"}},
			{Code: CmdPluginCommand, Indent: 0, Parameters: []interface{}{"CulPartLV"}},
			{Code: CmdEnd, Indent: 0},
		},
	}
	exec.Execute(context.Background(), s, page, &ExecuteOpts{})
	pkts := drainPackets(t, s)
	// No npc_effect for blocked commands
	for _, pkt := range pkts {
		if pkt.Type == "npc_effect" {
			t.Fatal("blocked plugin command should not be forwarded")
		}
	}
}

// ========================================================================
// Dispatch: maxCallDepth
// ========================================================================

func TestDispatch_MaxCallDepth(t *testing.T) {
	gs := newMockGameState()
	rl := &resource.ResourceLoader{
		CommonEvents: make([]*resource.CommonEvent, 2),
	}
	// CE 1 calls itself recursively
	rl.CommonEvents[1] = &resource.CommonEvent{
		ID: 1, Name: "Recursive",
		List: []*resource.EventCommand{
			{Code: CmdChangeVars, Indent: 0, Parameters: []interface{}{
				float64(1), float64(1), float64(1), float64(0), float64(1),
			}},
			{Code: CmdCallCommonEvent, Indent: 0, Parameters: []interface{}{float64(1)}},
			{Code: CmdEnd, Indent: 0},
		},
	}

	exec := New(nil, rl, nopLogger())
	s := testSession(1)

	page := &resource.EventPage{
		List: []*resource.EventCommand{
			{Code: CmdCallCommonEvent, Indent: 0, Parameters: []interface{}{float64(1)}},
			{Code: CmdEnd, Indent: 0},
		},
	}
	exec.Execute(context.Background(), s, page, &ExecuteOpts{GameState: gs})

	// Should stop at maxCallDepth (10), not infinite
	assert.True(t, gs.variables[1] <= maxCallDepth+1)
	assert.True(t, gs.variables[1] > 0)
}

// ========================================================================
// Dispatch: callCommonEvent edge cases
// ========================================================================

func TestCallCommonEvent_InvalidID(t *testing.T) {
	rl := &resource.ResourceLoader{
		CommonEvents: []*resource.CommonEvent{nil, nil},
	}
	exec := New(nil, rl, nopLogger())
	s := testSession(1)

	// CE 0 → out of range
	exec.callCommonEvent(context.Background(), s, 0, &ExecuteOpts{}, 0)
	// CE 1 → nil
	exec.callCommonEvent(context.Background(), s, 1, &ExecuteOpts{}, 0)
	// CE 99 → out of range
	exec.callCommonEvent(context.Background(), s, 99, &ExecuteOpts{}, 0)
	// Should not panic
}

// ========================================================================
// Script condition (condType=12)
// ========================================================================

func TestEvalCondition_Script(t *testing.T) {
	gs := newMockGameState()
	gs.variables[10] = 5

	exec := New(nil, &resource.ResourceLoader{}, nopLogger())
	s := testSession(1)
	opts := &ExecuteOpts{GameState: gs}

	// Script: $gameVariables.value(10) > 3 → true
	page := &resource.EventPage{
		List: []*resource.EventCommand{
			{Code: CmdConditionalStart, Indent: 0, Parameters: []interface{}{
				float64(12), "$gameVariables.value(10) > 3",
			}},
			{Code: CmdShowText, Indent: 1, Parameters: []interface{}{"", float64(0), float64(0), float64(2)}},
			{Code: CmdShowTextLine, Indent: 1, Parameters: []interface{}{"TRUE"}},
			{Code: CmdElseBranch, Indent: 0},
			{Code: CmdShowText, Indent: 1, Parameters: []interface{}{"", float64(0), float64(0), float64(2)}},
			{Code: CmdShowTextLine, Indent: 1, Parameters: []interface{}{"FALSE"}},
			{Code: CmdConditionalEnd, Indent: 0},
			{Code: CmdEnd, Indent: 0},
		},
	}
	s.DialogAckCh <- struct{}{}
	exec.Execute(context.Background(), s, page, opts)
	assertDialogText(t, s, "TRUE")
}

// ========================================================================
// Switch OFF condition
// ========================================================================

func TestEvalCondition_Switch_OFF(t *testing.T) {
	gs := newMockGameState()
	gs.switches[10] = false

	exec := New(nil, &resource.ResourceLoader{}, nopLogger())
	s := testSession(1)
	opts := &ExecuteOpts{GameState: gs}

	// Switch 10 is OFF, expected OFF (param[2]=1) → true
	page := condPage(0, float64(0), float64(10), float64(1))
	s.DialogAckCh <- struct{}{}
	exec.Execute(context.Background(), s, page, opts)
	assertDialogText(t, s, "TRUE")
}

// ========================================================================
// applyEquipChange: unknown slot, var ref
// ========================================================================

func TestApplyEquipChange_UnknownSlot(t *testing.T) {
	exec := New(nil, &resource.ResourceLoader{}, nopLogger())
	s := testSession(1)
	// Should not panic
	exec.applyEquipChange(context.Background(), s, "UnknownSlot", "10", nil)
}

func TestApplyEquipChange_VarRef(t *testing.T) {
	gs := newMockGameState()
	gs.variables[100] = 42
	store := newMockInventoryStore()
	exec := New(store, &resource.ResourceLoader{}, nopLogger())
	s := testSession(1)
	opts := &ExecuteOpts{GameState: gs}

	exec.applyEquipChange(context.Background(), s, "Leg", `\v[100]`, opts)
	equips := s.Equips
	assert.Equal(t, 42, equips[7]) // Leg → slot 7
}

// ========================================================================
// applyChangeEquipment
// ========================================================================

func TestApplyChangeEquipment(t *testing.T) {
	store := newMockInventoryStore()
	exec := New(store, &resource.ResourceLoader{}, nopLogger())
	s := testSession(1)

	page := &resource.EventPage{
		List: []*resource.EventCommand{
			{Code: CmdChangeEquipment, Indent: 0, Parameters: []interface{}{
				float64(1), float64(3), float64(55),
			}},
			{Code: CmdEnd, Indent: 0},
		},
	}
	exec.Execute(context.Background(), s, page, &ExecuteOpts{})
	equips := s.Equips
	assert.Equal(t, 55, equips[3])
}

// ========================================================================
// HandleCallCommon (CallCommon / CCT plugin commands)
// ========================================================================

func TestHandleCallCommon(t *testing.T) {
	gs := newMockGameState()
	rl := &resource.ResourceLoader{
		CommonEvents: []*resource.CommonEvent{
			nil,
			{ID: 1, Name: "MyEvent", List: []*resource.EventCommand{
				{Code: CmdChangeVars, Indent: 0, Parameters: []interface{}{
					float64(1), float64(1), float64(0), float64(0), float64(55),
				}},
				{Code: CmdEnd, Indent: 0},
			}},
		},
		CommonEventsByName: map[string]int{"MyEvent": 1},
	}
	exec := New(nil, rl, nopLogger())
	s := testSession(1)
	opts := &ExecuteOpts{GameState: gs}

	// CallCommon MyEvent
	page := &resource.EventPage{
		List: []*resource.EventCommand{
			{Code: CmdPluginCommand, Indent: 0, Parameters: []interface{}{"CallCommon MyEvent"}},
			{Code: CmdEnd, Indent: 0},
		},
	}
	exec.Execute(context.Background(), s, page, opts)
	assert.Equal(t, 55, gs.variables[1])
}

func TestHandleCallCommon_CCT(t *testing.T) {
	gs := newMockGameState()
	rl := &resource.ResourceLoader{
		CommonEvents: []*resource.CommonEvent{
			nil,
			{ID: 1, Name: "Prefix:Test", List: []*resource.EventCommand{
				{Code: CmdChangeVars, Indent: 0, Parameters: []interface{}{
					float64(1), float64(1), float64(0), float64(0), float64(66),
				}},
				{Code: CmdEnd, Indent: 0},
			}},
		},
	}
	exec := New(nil, rl, nopLogger())
	s := testSession(1)
	opts := &ExecuteOpts{GameState: gs}

	page := &resource.EventPage{
		List: []*resource.EventCommand{
			{Code: CmdPluginCommand, Indent: 0, Parameters: []interface{}{"CCT Prefix"}},
			{Code: CmdEnd, Indent: 0},
		},
	}
	exec.Execute(context.Background(), s, page, opts)
	assert.Equal(t, 66, gs.variables[1])
}

func TestHandleCallCommon_MissingArgs(t *testing.T) {
	rl := &resource.ResourceLoader{
		CommonEvents: []*resource.CommonEvent{nil},
	}
	exec := New(nil, rl, nopLogger())
	s := testSession(1)

	// CallCommon with no name
	page := &resource.EventPage{
		List: []*resource.EventCommand{
			{Code: CmdPluginCommand, Indent: 0, Parameters: []interface{}{"CallCommon"}},
			{Code: CmdPluginCommand, Indent: 0, Parameters: []interface{}{"CCT"}},
			{Code: CmdEnd, Indent: 0},
		},
	}
	exec.Execute(context.Background(), s, page, &ExecuteOpts{})
	// Should not panic
}

// ========================================================================
// skipToChoiceBranch: cancel branch
// ========================================================================

func TestSkipToChoiceBranch_Cancel(t *testing.T) {
	// In RMMV, cancelling a choice sends the cancelType value as the choice index.
	// Here cancelType=2, so client sends 2 to select the cancel branch.
	exec := New(nil, &resource.ResourceLoader{}, nopLogger())
	s := testSession(1)

	page := &resource.EventPage{
		List: []*resource.EventCommand{
			{Code: CmdShowChoices, Indent: 0, Parameters: []interface{}{
				[]interface{}{"Yes", "No"}, float64(2), // cancelType=2 → branch index 2
			}},
			{Code: CmdWhenBranch, Indent: 0, Parameters: []interface{}{float64(0)}},
			{Code: CmdWhenBranch, Indent: 0, Parameters: []interface{}{float64(1)}},
			{Code: CmdWhenCancel, Indent: 0},
			{Code: CmdChangeVars, Indent: 1, Parameters: []interface{}{
				float64(1), float64(1), float64(0), float64(0), float64(77),
			}},
			{Code: CmdBranchEnd, Indent: 0},
			{Code: CmdEnd, Indent: 0},
		},
	}

	gs := newMockGameState()
	go func() {
		time.Sleep(50 * time.Millisecond)
		s.ChoiceCh <- 2 // cancel (matches cancelType=2)
	}()
	exec.Execute(context.Background(), s, page, &ExecuteOpts{GameState: gs})
	assert.Equal(t, 77, gs.variables[1])
}

// ========================================================================
// ShowPicture: variable coords
// ========================================================================

func TestSendShowPicture_VarCoords(t *testing.T) {
	gs := newMockGameState()
	gs.variables[10] = 150
	gs.variables[11] = 250
	exec := New(nil, &resource.ResourceLoader{}, nopLogger())
	s := testSession(1)

	params := []interface{}{
		float64(1), "pic", float64(0), float64(1), float64(10), float64(11),
		float64(100), float64(100), float64(255), float64(0),
	}
	exec.sendShowPicture(s, params, &ExecuteOpts{GameState: gs})

	pkts := drainPackets(t, s)
	require.Len(t, pkts, 1)
	var data map[string]interface{}
	json.Unmarshal(pkts[0].Payload, &data)
	p := data["params"].([]interface{})
	assert.Equal(t, float64(150), p[4])
	assert.Equal(t, float64(250), p[5])
	assert.Equal(t, float64(0), p[3])
}

// ========================================================================
// classParam edge cases
// ========================================================================

func TestClassParam_EdgeCases(t *testing.T) {
	assert.Equal(t, 0, classParam(nil, 0, 1))
	cls := &resource.Class{Params: [][]int{{0, 100, 200}}}
	assert.Equal(t, 200, classParam(cls, 0, 2))
	assert.Equal(t, 200, classParam(cls, 0, 99)) // beyond row → last element
	assert.Equal(t, 0, classParam(cls, 5, 1))    // paramIdx out of range
}

// ========================================================================
// resolveOperand
// ========================================================================

func TestResolveOperand(t *testing.T) {
	gs := newMockGameState()
	gs.variables[5] = 42

	exec := New(nil, nil, nopLogger())

	// type=0 (constant) → val directly
	params := []interface{}{float64(0), float64(10)}
	assert.Equal(t, 10, exec.resolveOperand(params, 0, 1, nil))

	// type=1 (variable) → read from gs
	params2 := []interface{}{float64(1), float64(5)}
	assert.Equal(t, 42, exec.resolveOperand(params2, 0, 1, &ExecuteOpts{GameState: gs}))
}

// ========================================================================
// Parallel: RunParallelEventsSynced
// ========================================================================

func TestRunParallelEventsSynced_Empty(t *testing.T) {
	exec := New(nil, &resource.ResourceLoader{}, nopLogger())
	s := testSession(1)
	// Empty events → immediate return
	exec.RunParallelEventsSynced(context.Background(), s, nil, &ExecuteOpts{})
}

func TestRunParallelEventsSynced_ContextCancel(t *testing.T) {
	gs := newMockGameState()
	exec := New(nil, &resource.ResourceLoader{}, nopLogger())
	s := testSession(1)
	s.MapID = 1

	events := []*ParallelEventState{
		NewParallelEventState(1, []*resource.EventCommand{
			{Code: CmdWait, Indent: 0, Parameters: []interface{}{float64(600)}},
			{Code: CmdEnd, Indent: 0},
		}, 3),
	}

	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()
	opts := &ExecuteOpts{GameState: gs, MapID: 1}

	start := time.Now()
	exec.RunParallelEventsSynced(ctx, s, events, opts)
	assert.True(t, time.Since(start) < 2*time.Second)
}

func TestRunParallelEventsSynced_MapChanged(t *testing.T) {
	gs := newMockGameState()
	exec := New(nil, &resource.ResourceLoader{}, nopLogger())
	s := testSession(1)
	s.MapID = 2 // different from opts.MapID

	events := []*ParallelEventState{
		NewParallelEventState(1, []*resource.EventCommand{
			{Code: CmdEnd, Indent: 0},
		}, 3),
	}

	opts := &ExecuteOpts{GameState: gs, MapID: 1}
	exec.RunParallelEventsSynced(context.Background(), s, events, opts)
	// Should return immediately since map doesn't match
}

// ========================================================================
// Parallel: applySwitches with PageRefreshFn
// ========================================================================

func TestApplySwitches_PageRefresh(t *testing.T) {
	gs := newMockGameState()
	exec := New(nil, &resource.ResourceLoader{}, nopLogger())
	s := testSession(1)

	refreshCalled := false
	opts := &ExecuteOpts{
		GameState: gs,
		PageRefreshFn: func(s *player.PlayerSession) {
			refreshCalled = true
		},
	}

	page := &resource.EventPage{
		List: []*resource.EventCommand{
			{Code: CmdChangeSwitches, Indent: 0, Parameters: []interface{}{
				float64(100), float64(100), float64(0),
			}},
			{Code: CmdEnd, Indent: 0},
		},
	}
	exec.Execute(context.Background(), s, page, opts)
	assert.True(t, refreshCalled)
	assert.True(t, gs.switches[100])
}

// ========================================================================
// applySelfSwitch with PageRefreshFn
// ========================================================================

func TestApplySelfSwitch_PageRefresh(t *testing.T) {
	gs := newMockGameState()
	exec := New(nil, &resource.ResourceLoader{}, nopLogger())
	s := testSession(1)

	refreshCalled := false
	opts := &ExecuteOpts{
		GameState: gs,
		MapID:     5,
		EventID:   10,
		PageRefreshFn: func(s *player.PlayerSession) {
			refreshCalled = true
		},
	}

	page := &resource.EventPage{
		List: []*resource.EventCommand{
			{Code: CmdChangeSelfSwitch, Indent: 0, Parameters: []interface{}{
				"A", float64(0),
			}},
			{Code: CmdEnd, Indent: 0},
		},
	}
	exec.Execute(context.Background(), s, page, opts)
	assert.True(t, refreshCalled)
}

// ========================================================================
// applySwitches: special switches 15/85
// ========================================================================

func TestApplySwitches_SpecialSwitchResend(t *testing.T) {
	gs := newMockGameState()
	gs.switches[15] = true // already ON
	res := withTestMMOConfig(&resource.ResourceLoader{})
	res.MMOConfig.AlwaysSendSwitches = []int{15, 85}
	exec := New(nil, res, nopLogger())
	s := testSession(1)
	opts := &ExecuteOpts{GameState: gs}

	// Set switch 15 to ON (already ON) → should still send switch_change
	page := &resource.EventPage{
		List: []*resource.EventCommand{
			{Code: CmdChangeSwitches, Indent: 0, Parameters: []interface{}{
				float64(15), float64(15), float64(0),
			}},
			{Code: CmdEnd, Indent: 0},
		},
	}
	exec.Execute(context.Background(), s, page, opts)
	pkts := drainPackets(t, s)
	switchChangeFound := false
	for _, pkt := range pkts {
		if pkt.Type == "switch_change" {
			switchChangeFound = true
		}
	}
	assert.True(t, switchChangeFound, "special switch 15 should send change even when unchanged")
}

// ========================================================================
// Helpers
// ========================================================================

// condPage creates a page with a conditional branch test structure.
// params are passed as condType + additional params to CmdConditionalStart.
func condPage(condType int, extraParams ...interface{}) *resource.EventPage {
	params := make([]interface{}, 0, 1+len(extraParams))
	params = append(params, float64(condType))
	params = append(params, extraParams...)

	return &resource.EventPage{
		List: []*resource.EventCommand{
			{Code: CmdConditionalStart, Indent: 0, Parameters: params},
			{Code: CmdShowText, Indent: 1, Parameters: []interface{}{"", float64(0), float64(0), float64(2)}},
			{Code: CmdShowTextLine, Indent: 1, Parameters: []interface{}{"TRUE"}},
			{Code: CmdElseBranch, Indent: 0},
			{Code: CmdShowText, Indent: 1, Parameters: []interface{}{"", float64(0), float64(0), float64(2)}},
			{Code: CmdShowTextLine, Indent: 1, Parameters: []interface{}{"FALSE"}},
			{Code: CmdConditionalEnd, Indent: 0},
			{Code: CmdEnd, Indent: 0},
		},
	}
}

// assertDialogText checks that the first dialog packet has the expected text.
func assertDialogText(t *testing.T, s *player.PlayerSession, expected string) {
	t.Helper()
	pkts := drainPackets(t, s)
	for _, pkt := range pkts {
		if pkt.Type == "npc_dialog" {
			var data map[string]interface{}
			json.Unmarshal(pkt.Payload, &data)
			lines := data["lines"].([]interface{})
			assert.Equal(t, expected, lines[0])
			return
		}
	}
	t.Fatalf("no npc_dialog packet found, expected text %q", expected)
}
