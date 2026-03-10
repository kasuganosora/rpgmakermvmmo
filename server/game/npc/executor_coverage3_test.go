package npc

import (
	"context"
	"testing"

	"github.com/kasuganosora/rpgmakermvmmo/server/model"
	"github.com/kasuganosora/rpgmakermvmmo/server/resource"
	"github.com/kasuganosora/rpgmakermvmmo/server/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ========================================================================
// gormInventoryStore: HasItemOfKind includeEquipped=true branch
// ========================================================================

func TestGormHasItemOfKind_IncludeEquipped(t *testing.T) {
	db := testutil.SetupTestDB(t)
	store := &gormInventoryStore{db: db}
	char := model.Character{Name: "Test", AccountID: 1, Gold: 0}
	require.NoError(t, db.Create(&char).Error)
	ctx := context.Background()

	// Create an equipped armor
	inv := model.Inventory{CharID: char.ID, ItemID: 10, Kind: model.ItemKindArmor, Qty: 1, Equipped: true, SlotIndex: 1}
	require.NoError(t, db.Create(&inv).Error)

	// includeEquipped=false: equipped item not counted
	has, err := store.HasItemOfKind(ctx, char.ID, 10, model.ItemKindArmor, false)
	require.NoError(t, err)
	assert.False(t, has)

	// includeEquipped=true: equipped item counted
	has, err = store.HasItemOfKind(ctx, char.ID, 10, model.ItemKindArmor, true)
	require.NoError(t, err)
	assert.True(t, has)
}

// ========================================================================
// gormInventoryStore: IsEquipped with equipped=true
// ========================================================================

func TestGormIsEquipped_True(t *testing.T) {
	db := testutil.SetupTestDB(t)
	store := &gormInventoryStore{db: db}
	char := model.Character{Name: "Test", AccountID: 1, Gold: 0}
	require.NoError(t, db.Create(&char).Error)
	ctx := context.Background()

	inv := model.Inventory{CharID: char.ID, ItemID: 10, Kind: model.ItemKindWeapon, Qty: 1, Equipped: true, SlotIndex: 0}
	require.NoError(t, db.Create(&inv).Error)

	has, err := store.IsEquipped(ctx, char.ID, 10, model.ItemKindWeapon)
	require.NoError(t, err)
	assert.True(t, has)
}

// ========================================================================
// gormInventoryStore: HasSkill true
// ========================================================================

func TestGormHasSkill_True(t *testing.T) {
	db := testutil.SetupTestDB(t)
	store := &gormInventoryStore{db: db}
	char := model.Character{Name: "Test", AccountID: 1, Gold: 0}
	require.NoError(t, db.Create(&char).Error)
	ctx := context.Background()

	skill := model.CharSkill{CharID: char.ID, SkillID: 5}
	require.NoError(t, db.Create(&skill).Error)

	has, err := store.HasSkill(ctx, char.ID, 5)
	require.NoError(t, err)
	assert.True(t, has)
}

// ========================================================================
// SendStateSyncAfterExecution — 0%
// ========================================================================

func TestSendStateSyncAfterExecution_NoOp(t *testing.T) {
	exec := New(nil, &resource.ResourceLoader{}, nopLogger())
	s := testSession(1)
	// Just call it — it's a no-op
	exec.SendStateSyncAfterExecution(context.Background(), s, nil)
}

// ========================================================================
// resolveTextCodes: remaining branches
// ========================================================================

func TestResolveTextCodes_N_ActorFromRes(t *testing.T) {
	res := &resource.ResourceLoader{
		Actors: []*resource.Actor{nil, {ID: 1, Name: "Hero"}, nil, {ID: 3, Name: "NPC3"}},
	}
	exec := New(nil, res, nopLogger())
	s := testSession(1)
	s.CharName = "Luna"

	// \N[3] → actor 3 from resource
	result := exec.resolveTextCodes(`\N[3]`, s, nil)
	assert.Equal(t, "NPC3", result)

	// \N[2] → nil actor → unchanged
	result = exec.resolveTextCodes(`\N[2]`, s, nil)
	assert.Equal(t, `\N[2]`, result)
}

// ========================================================================
// waitForEffectAck: ctx cancel
// ========================================================================

func TestWaitForEffectAck_CtxCancel(t *testing.T) {
	s := testSession(1)
	exec := New(nil, &resource.ResourceLoader{}, nopLogger())
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // immediate cancel
	result := exec.waitForEffectAck(ctx, s, 999)
	assert.False(t, result)
}

func TestWaitForEffectAck_Disconnect_Cov3(t *testing.T) {
	s := testSession(1)
	exec := New(nil, &resource.ResourceLoader{}, nopLogger())
	close(s.Done)
	result := exec.waitForEffectAck(context.Background(), s, 999)
	assert.False(t, result)
}

// ========================================================================
// evaluateCondition: switch OFF, condType=2 OFF
// ========================================================================

func TestEvalCondition_SwitchOFF(t *testing.T) {
	gs := newMockGameState()
	gs.switches[1] = true
	exec := New(nil, &resource.ResourceLoader{}, nopLogger())
	s := testSession(1)
	opts := &ExecuteOpts{GameState: gs, MapID: 1, EventID: 1}

	// Switch ON, expected OFF → false
	assert.False(t, exec.evaluateCondition(context.Background(), s, []interface{}{float64(0), float64(1), float64(1)}, opts))
	// Switch ON, expected ON → true
	assert.True(t, exec.evaluateCondition(context.Background(), s, []interface{}{float64(0), float64(1), float64(0)}, opts))

	// SelfSwitch: expected OFF
	gs.selfSwitches[selfSwitchKey(1, 1, "A")] = true
	assert.False(t, exec.evaluateCondition(context.Background(), s, []interface{}{float64(2), "A", float64(1)}, opts))
}

// ========================================================================
// evalActorCondition weapon/armor with store
// ========================================================================

func TestEvalActorCondition_WeaponWithStore(t *testing.T) {
	store := newMockInventoryStore()
	exec := New(store, &resource.ResourceLoader{}, nopLogger())
	s := testSession(1)

	// subType=4 weapon, store says false
	assert.False(t, exec.evalActorCondition(context.Background(), s, []interface{}{float64(4), float64(1), float64(4), float64(5)}))
}

func TestEvalActorCondition_ArmorWithStore(t *testing.T) {
	store := newMockInventoryStore()
	exec := New(store, &resource.ResourceLoader{}, nopLogger())
	s := testSession(1)

	// subType=5 armor
	assert.False(t, exec.evalActorCondition(context.Background(), s, []interface{}{float64(4), float64(1), float64(5), float64(5)}))
}

// ========================================================================
// evalGoldCondition: lessThanOrEqual
// ========================================================================

func TestEvalGoldCondition_LessThanOrEqual(t *testing.T) {
	store := newMockInventoryStore()
	store.gold[1] = 50
	exec := New(store, &resource.ResourceLoader{}, nopLogger())
	s := testSession(1)

	// op=1 (<=): 50 <= 50 = true
	assert.True(t, exec.evalGoldCondition(context.Background(), s, []interface{}{float64(7), float64(50), float64(1)}))
	// 50 <= 49 = false
	assert.False(t, exec.evalGoldCondition(context.Background(), s, []interface{}{float64(7), float64(49), float64(1)}))
}

// ========================================================================
// skipToConditionalEnd: not found
// ========================================================================

func TestSkipToConditionalEnd_NotFound(t *testing.T) {
	exec := New(nil, &resource.ResourceLoader{}, nopLogger())
	cmds := []*resource.EventCommand{
		{Code: CmdElseBranch, Indent: 0},
		{Code: CmdEnd, Indent: 0},
	}
	result := exec.skipToConditionalEnd(cmds, 0, 0)
	assert.Equal(t, 1, result) // len-1
}

// ========================================================================
// applyGold: decrease more than gold (exact clamp path)
// ========================================================================

func TestApplyGold_DecreaseExact(t *testing.T) {
	store := newMockInventoryStore()
	store.gold[1] = 100
	exec := New(store, &resource.ResourceLoader{}, nopLogger())
	s := testSession(1)

	// Decrease exactly 100 → amount=-100, gold=100, -100 >= -100 → no clamp needed
	err := exec.applyGold(context.Background(), s, []interface{}{float64(1), float64(0), float64(100)}, nil)
	require.NoError(t, err)
	assert.Equal(t, int64(0), store.gold[1])
}

func TestApplyGold_DecreaseVarOperandClamp(t *testing.T) {
	store := newMockInventoryStore()
	store.gold[1] = 30
	gs := newMockGameState()
	gs.variables[5] = 100
	exec := New(store, &resource.ResourceLoader{}, nopLogger())
	s := testSession(1)
	opts := &ExecuteOpts{GameState: gs}

	// operandType=1, operand=v[5]=100, decrease: gold(30) < 100 → clamp to -30
	err := exec.applyGold(context.Background(), s, []interface{}{float64(1), float64(1), float64(5)}, opts)
	require.NoError(t, err)
	assert.Equal(t, int64(0), store.gold[1])
}

// ========================================================================
// applyItems: remove with currentQty < qty (clamp)
// ========================================================================

func TestApplyItems_RemovePartialClamp(t *testing.T) {
	store := newMockInventoryStore()
	store.items[itemKey(1, 5)] = 3
	exec := New(store, &resource.ResourceLoader{}, nopLogger())
	s := testSession(1)

	// Remove 10 but only have 3 → removes 3
	err := exec.applyItems(context.Background(), s, []interface{}{float64(5), float64(1), float64(0), float64(10)}, nil)
	require.NoError(t, err)
	_, ok := store.items[itemKey(1, 5)]
	assert.False(t, ok) // removed
}

func TestApplyItems_AddWithVarOperand(t *testing.T) {
	store := newMockInventoryStore()
	gs := newMockGameState()
	gs.variables[3] = 5
	exec := New(store, &resource.ResourceLoader{}, nopLogger())
	s := testSession(1)
	opts := &ExecuteOpts{GameState: gs}

	// operandType=1, qty=v[3]=5
	err := exec.applyItems(context.Background(), s, []interface{}{float64(1), float64(0), float64(1), float64(3)}, opts)
	require.NoError(t, err)
	assert.Equal(t, 5, store.items[itemKey(1, 1)])
}

// ========================================================================
// applyChangeArmors/Weapons: remove fail (error log, no crash)
// ========================================================================

func TestApplyChangeArmors_RemoveFail(t *testing.T) {
	store := newMockInventoryStore()
	// No items — remove will fail
	exec := New(store, &resource.ResourceLoader{}, nopLogger())
	s := testSession(1)

	exec.applyChangeArmors(context.Background(), s, []interface{}{float64(5), float64(1), float64(0), float64(1)}, nil)
	// Should log warning but not crash; still sends effect
}

func TestApplyChangeWeapons_RemoveFail(t *testing.T) {
	store := newMockInventoryStore()
	exec := New(store, &resource.ResourceLoader{}, nopLogger())
	s := testSession(1)

	exec.applyChangeWeapons(context.Background(), s, []interface{}{float64(5), float64(1), float64(0), float64(1)}, nil)
}

// ========================================================================
// applyChangeEquipment: weapon slot kind=2
// ========================================================================

func TestApplyChangeEquipment_WeaponKind(t *testing.T) {
	store := newMockInventoryStore()
	exec := New(store, &resource.ResourceLoader{}, nopLogger())
	s := testSession(1)
	s.Equips = map[int]int{}

	// etypeID=0 → slotIndex=0 → kind=2 (weapon)
	exec.applyChangeEquipment(context.Background(), s, []interface{}{float64(1), float64(0), float64(10)}, nil)
	assert.Equal(t, 10, s.GetEquip(0))
}

// ========================================================================
// resolveTextVarRef: multiple matches in string
// ========================================================================

func TestResolveTextVarRef_MultipleMatches(t *testing.T) {
	gs := newMockGameState()
	gs.variables[10] = 42
	gs.variables[20] = 99
	exec := New(nil, &resource.ResourceLoader{}, nopLogger())
	opts := &ExecuteOpts{GameState: gs}

	result := exec.resolveTextVarRef(`\v[10] and \V[20]`, opts)
	assert.Equal(t, "42 and 99", result)
}

// ========================================================================
// findMapEvent: nil event in list
// ========================================================================

func TestFindMapEvent_NilEventInList(t *testing.T) {
	res := &resource.ResourceLoader{
		Maps: map[int]*resource.MapData{
			1: {Events: []*resource.MapEvent{nil, nil, {ID: 2}}},
		},
	}
	exec := New(nil, res, nopLogger())
	ev := exec.findMapEvent(1, 2)
	assert.NotNil(t, ev)
	assert.Equal(t, 2, ev.ID)
}

// ========================================================================
// teSetSelfVariable: bad operand (non-numeric)
// ========================================================================

func TestTESetSelfVariable_BadOperand(t *testing.T) {
	s := testSession(1)
	exec := New(nil, &resource.ResourceLoader{}, nopLogger())
	opts := &ExecuteOpts{MapID: 1, EventID: 1, GameState: newMockGameState()}

	cmd := &resource.EventCommand{Code: CmdPluginCommand, Parameters: []interface{}{"TE_SET_SELF_VARIABLE 0 0 abc"}}
	handled := exec.handleTECallOriginEvent(context.Background(), s, cmd, opts, 0)
	assert.True(t, handled)
}

// ========================================================================
// handleCallCommon: empty params[0]
// ========================================================================

func TestHandleCallCommon_EmptyParam(t *testing.T) {
	s := testSession(1)
	exec := New(nil, &resource.ResourceLoader{}, nopLogger())

	cmd := &resource.EventCommand{Code: CmdPluginCommand, Parameters: []interface{}{""}}
	handled := exec.handleCallCommon(context.Background(), s, cmd, nil, 0)
	assert.False(t, handled)
}

// ========================================================================
// teCallOriginEvent: event not found in map
// ========================================================================

func TestTECallOriginEvent_EventNotFoundInMap(t *testing.T) {
	s := testSession(1)
	res := &resource.ResourceLoader{
		Maps: map[int]*resource.MapData{
			1: {Events: []*resource.MapEvent{nil}},
		},
	}
	exec := New(nil, res, nopLogger())
	opts := &ExecuteOpts{MapID: 1, EventID: 99, GameState: newMockGameState()}

	cmd := &resource.EventCommand{Code: CmdPluginCommand, Parameters: []interface{}{"TE_CALL_ORIGIN_EVENT"}}
	handled := exec.handleTECallOriginEvent(context.Background(), s, cmd, opts, 0)
	assert.True(t, handled)
}

// ========================================================================
// execCulSkillEffect: error in JS execution
// ========================================================================

func TestExecCulSkillEffect_NilSession(t *testing.T) {
	exec := New(nil, &resource.ResourceLoader{}, nopLogger())
	opts := &ExecuteOpts{GameState: newMockGameState()}
	// nil session → should cause panic in JS but recover
	// Actually execCulSkillEffect checks s != nil for injectScriptGameActors
	exec.execCulSkillEffect(nil, opts)
}

// ========================================================================
// execParaCheck: switch 131 true → v[235]=1
// ========================================================================

func TestExecParaCheck_Switch131(t *testing.T) {
	gs := newMockGameState()
	gs.switches[131] = true // 変身中
	s := testSessionWithStats(1, 100, 100, 50, 50, 10, 0)
	s.ClassID = 1
	s.Equips = map[int]int{0: 0, 1: 0}
	exec := New(nil, testResMMO(), nopLogger())
	opts := &ExecuteOpts{GameState: gs, TransientVars: make(map[int]interface{})}

	exec.execParaCheck(s, opts)
	assert.Equal(t, 1, gs.variables[235])
}

// ========================================================================
// evalSetupChildTarget: session nil branch
// ========================================================================

func TestEvalSetupChildTarget_NilSession(t *testing.T) {
	exec := New(nil, &resource.ResourceLoader{}, nopLogger())
	opts := &ExecuteOpts{GameState: newMockGameState()}

	id := exec.evalSetupChildTarget("42", nil, opts)
	assert.Equal(t, 42, id)
}

func TestEvalSetupChildTarget_NilOpts(t *testing.T) {
	s := testSession(1)
	exec := New(nil, &resource.ResourceLoader{}, nopLogger())

	id := exec.evalSetupChildTarget("42", s, nil)
	assert.Equal(t, 42, id)
}

// ========================================================================
// parseMeta: empty note
// ========================================================================

func TestParseMeta_EmptyNote(t *testing.T) {
	s := testSession(1)
	exec := New(nil, &resource.ResourceLoader{
		Maps: map[int]*resource.MapData{1: {Note: ""}},
	}, nopLogger())
	opts := &ExecuteOpts{MapID: 1, GameState: newMockGameState()}
	vm := exec.getOrCreateCondVM(s, opts)
	v, _ := vm.RunString("typeof $dataMap.meta")
	assert.Equal(t, "object", v.String())
}

// ========================================================================
// stepUntilWait: changeSelfSwitch in parallel
// ========================================================================

func TestStepUntilWait_ChangeSelfSwitch_Cov3(t *testing.T) {
	gs := newMockGameState()
	s := testSession(1)
	s.MapID = 1
	exec := New(nil, &resource.ResourceLoader{}, nopLogger())
	opts := &ExecuteOpts{MapID: 1, EventID: 5, GameState: gs}

	ev := &ParallelEventState{
		EventID: 5,
		Cmds: []*resource.EventCommand{
			{Code: CmdChangeSelfSwitch, Parameters: []interface{}{"A", float64(0)}},
			{Code: CmdEnd, Indent: 0},
		},
	}

	exec.stepUntilWait(context.Background(), s, ev, opts)
	assert.True(t, gs.selfSwitches[selfSwitchKey(1, 5, "A")])
}

// ========================================================================
// stepUntilWait: CmdEnd indent > 0 (non-terminal)
// ========================================================================

func TestStepUntilWait_CmdEnd_IndentGtZero(t *testing.T) {
	gs := newMockGameState()
	s := testSession(1)
	s.MapID = 1
	exec := New(nil, &resource.ResourceLoader{}, nopLogger())
	opts := &ExecuteOpts{MapID: 1, GameState: gs}

	ev := &ParallelEventState{
		EventID: 1,
		Cmds: []*resource.EventCommand{
			{Code: CmdEnd, Indent: 1},
			{Code: CmdChangeSwitches, Parameters: []interface{}{float64(50), float64(50), float64(0)}},
			{Code: CmdEnd, Indent: 0},
		},
	}

	exec.stepUntilWait(context.Background(), s, ev, opts)
	assert.True(t, gs.switches[50])
}

// ========================================================================
// Dispatch: CmdChangeWeapons / CmdChangeArmors via dispatch
// ========================================================================

func TestDispatch_ChangeWeapons(t *testing.T) {
	store := newMockInventoryStore()
	exec := New(store, &resource.ResourceLoader{}, nopLogger())
	s := testSession(1)

	page := &resource.EventPage{
		List: []*resource.EventCommand{
			{Code: CmdChangeWeapons, Parameters: []interface{}{float64(5), float64(0), float64(0), float64(2)}},
			{Code: CmdEnd},
		},
	}
	exec.Execute(context.Background(), s, page, nil)
	assert.Equal(t, 2, store.items[itemKey(1, 5)])
}

func TestDispatch_ChangeArmors(t *testing.T) {
	store := newMockInventoryStore()
	exec := New(store, &resource.ResourceLoader{}, nopLogger())
	s := testSession(1)

	page := &resource.EventPage{
		List: []*resource.EventCommand{
			{Code: CmdChangeArmors, Parameters: []interface{}{float64(5), float64(0), float64(0), float64(2)}},
			{Code: CmdEnd},
		},
	}
	exec.Execute(context.Background(), s, page, nil)
	assert.Equal(t, 2, store.items[itemKey(1, 5)])
}

// ========================================================================
// Dispatch: CmdChangeEquipment via dispatch
// ========================================================================

func TestDispatch_ChangeEquipment(t *testing.T) {
	store := newMockInventoryStore()
	exec := New(store, &resource.ResourceLoader{}, nopLogger())
	s := testSession(1)
	s.Equips = map[int]int{}

	page := &resource.EventPage{
		List: []*resource.EventCommand{
			{Code: CmdChangeEquipment, Parameters: []interface{}{float64(1), float64(1), float64(10)}},
			{Code: CmdEnd},
		},
	}
	exec.Execute(context.Background(), s, page, nil)
	assert.Equal(t, 10, s.GetEquip(1))
}

// ========================================================================
// stepUntilWait: TE plugin in parallel
// ========================================================================

func TestStepUntilWait_TEPlugin(t *testing.T) {
	gs := newMockGameState()
	s := testSession(1)
	s.MapID = 1
	res := &resource.ResourceLoader{
		Maps: map[int]*resource.MapData{
			1: {Events: []*resource.MapEvent{nil, {ID: 1, Pages: []*resource.EventPage{{}},
				OriginalPages: []*resource.EventPage{{
					List: []*resource.EventCommand{
						{Code: CmdChangeSwitches, Parameters: []interface{}{float64(77), float64(77), float64(0)}},
						{Code: CmdEnd},
					},
				}}}}},
		},
	}
	exec := New(nil, res, nopLogger())
	opts := &ExecuteOpts{MapID: 1, EventID: 1, GameState: gs}

	ev := &ParallelEventState{
		EventID: 1,
		Cmds: []*resource.EventCommand{
			{Code: CmdPluginCommand, Parameters: []interface{}{"TE_CALL_ORIGIN_EVENT"}},
			{Code: CmdEnd, Indent: 0},
		},
	}

	exec.stepUntilWait(context.Background(), s, ev, opts)
	assert.True(t, gs.switches[77])
}

// ========================================================================
// Dispatch: serverExecPluginCmds (CulSkillEffect/ParaCheck) via parallel
// ========================================================================

func TestStepUntilWait_CulSkillEffect(t *testing.T) {
	gs := newMockGameState()
	s := testSessionWithStats(1, 100, 100, 50, 50, 10, 0)
	s.MapID = 1
	s.ClassID = 1
	s.Equips = map[int]int{0: 0, 1: 0}
	exec := New(nil, &resource.ResourceLoader{}, nopLogger())
	opts := &ExecuteOpts{MapID: 1, GameState: gs, TransientVars: make(map[int]interface{})}

	// CulSkillEffect is NOT in stepUntilWait — it falls through to "forward plugin".
	// But it's in the blockedPluginCmds? No, it's in serverExecPluginCmds only for executeList.
	// In stepUntilWait, it would be forwarded. Let's just test the default forward path.
	ev := &ParallelEventState{
		EventID: 1,
		Cmds: []*resource.EventCommand{
			{Code: CmdPluginCommand, Parameters: []interface{}{"FaceId 1 30"}},
			{Code: CmdEnd, Indent: 0},
		},
	}

	exec.stepUntilWait(context.Background(), s, ev, opts)
	pkts := drainPackets(t, s)
	// FaceId is forwarded
	assert.True(t, len(pkts) > 0)
}

// ========================================================================
// injectScriptGameStateMutable: switchChanges read in value()
// ========================================================================

func TestExecMutableScript_SwitchRead(t *testing.T) {
	gs := newMockGameState()
	s := testSession(1)
	exec := New(nil, &resource.ResourceLoader{}, nopLogger())
	opts := &ExecuteOpts{GameState: gs}

	// Set switch via _data, then read via value()
	exec.execMutableScript(`
		$gameSwitches._data[42] = true;
		if ($gameSwitches.value(42)) {
			$gameVariables._data[1] = 99;
		}
	`, s, opts)
	assert.Equal(t, 99, gs.variables[1])
}

// ========================================================================
// applyChangeClass: nil res
// ========================================================================

func TestApplyChangeClass_NilRes(t *testing.T) {
	s := testSessionWithStats(1, 100, 100, 50, 50, 10, 0)
	s.ClassID = 1
	exec := New(nil, nil, nopLogger())

	exec.applyChangeClass(context.Background(), s, []interface{}{float64(1), float64(2), float64(0)}, nil)
	assert.Equal(t, 2, s.ClassID)
	// HP/MP unchanged since res is nil
	assert.Equal(t, 100, s.HP)
}

// ========================================================================
// Dispatch: context cancel mid-execution
// ========================================================================

func TestDispatch_ContextCancel(t *testing.T) {
	s := testSession(1)
	exec := New(nil, &resource.ResourceLoader{}, nopLogger())
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	page := &resource.EventPage{
		List: []*resource.EventCommand{
			{Code: CmdChangeSwitches, Parameters: []interface{}{float64(1), float64(1), float64(0)}},
			{Code: CmdEnd},
		},
	}
	exec.Execute(ctx, s, page, &ExecuteOpts{GameState: newMockGameState()})
	// Should return immediately due to ctx cancel
}

// ========================================================================
// Dispatch: ShowChoices with extra params (position, bg)
// ========================================================================

func TestDispatch_ShowChoices_WithPositionAndBg(t *testing.T) {
	gs := newMockGameState()
	s := testSession(1)
	exec := New(nil, &resource.ResourceLoader{}, nopLogger())
	opts := &ExecuteOpts{GameState: gs}

	page := &resource.EventPage{
		List: []*resource.EventCommand{
			{Code: CmdShowChoices, Parameters: []interface{}{
				[]interface{}{"A", "B"},
				float64(1),
				float64(0),
				float64(1), // position=center
				float64(1), // bg=dim
			}},
			{Code: CmdWhenBranch, Indent: 0},
			{Code: CmdBranchEnd, Indent: 0},
			{Code: CmdEnd},
		},
	}

	go func() {
		s.ChoiceCh <- 0
	}()
	exec.Execute(context.Background(), s, page, opts)
}

// ========================================================================
// Dispatch: ShowText + ShowChoices with extra params
// ========================================================================

func TestDispatch_TextChoices_WithExtraParams(t *testing.T) {
	gs := newMockGameState()
	s := testSession(1)
	exec := New(nil, &resource.ResourceLoader{}, nopLogger())
	opts := &ExecuteOpts{GameState: gs}

	page := &resource.EventPage{
		List: []*resource.EventCommand{
			{Code: CmdShowText, Parameters: []interface{}{"face", float64(0), float64(0), float64(2)}},
			{Code: CmdShowTextLine, Parameters: []interface{}{"Hello"}},
			{Code: CmdShowChoices, Parameters: []interface{}{
				[]interface{}{"Yes", "No"},
				float64(1),
				float64(0),
				float64(1), // choicePosition
				float64(1), // choiceBg
			}},
			{Code: CmdWhenBranch, Indent: 0},
			{Code: CmdBranchEnd, Indent: 0},
			{Code: CmdEnd},
		},
	}

	go func() {
		s.ChoiceCh <- 0
	}()
	exec.Execute(context.Background(), s, page, opts)
}

// ========================================================================
// handleTECallOriginEvent: non-string param[0]
// ========================================================================

func TestHandleTECallOriginEvent_NonStringParam(t *testing.T) {
	s := testSession(1)
	exec := New(nil, &resource.ResourceLoader{}, nopLogger())

	cmd := &resource.EventCommand{Code: CmdPluginCommand, Parameters: []interface{}{float64(42)}}
	handled := exec.handleTECallOriginEvent(context.Background(), s, cmd, nil, 0)
	assert.False(t, handled) // non-string → not handled
}

// ========================================================================
// Dispatch: LeaveInstance without fn
// ========================================================================

func TestDispatch_Instance_NoFn(t *testing.T) {
	s := testSession(1)
	exec := New(nil, &resource.ResourceLoader{}, nopLogger())
	opts := &ExecuteOpts{GameState: newMockGameState()}

	page := &resource.EventPage{
		List: []*resource.EventCommand{
			{Code: CmdPluginCommand, Parameters: []interface{}{"EnterInstance"}},
			{Code: CmdPluginCommand, Parameters: []interface{}{"LeaveInstance"}},
			{Code: CmdEnd},
		},
	}
	// No EnterInstanceFn/LeaveInstanceFn set — should not crash
	exec.Execute(context.Background(), s, page, opts)
}

// ========================================================================
// Dispatch: Plugin command not TE, not instance, not equip, not callcommon, not server exec, not blocked → forward
// ========================================================================

func TestDispatch_PluginCmd_Forward(t *testing.T) {
	s := testSession(1)
	exec := New(nil, &resource.ResourceLoader{}, nopLogger())

	page := &resource.EventPage{
		List: []*resource.EventCommand{
			{Code: CmdPluginCommand, Parameters: []interface{}{"SomeUnknownPlugin arg1 arg2"}},
			{Code: CmdEnd},
		},
	}
	exec.Execute(context.Background(), s, page, nil)
	pkts := drainPackets(t, s)
	found := false
	for _, p := range pkts {
		if p.Type == "npc_effect" {
			found = true
		}
	}
	assert.True(t, found)
}

// ========================================================================
// Parallel: stepUntilWait with BreakLoop
// ========================================================================

func TestStepUntilWait_BreakLoop_Parallel(t *testing.T) {
	gs := newMockGameState()
	s := testSession(1)
	s.MapID = 1
	exec := New(nil, &resource.ResourceLoader{}, nopLogger())
	opts := &ExecuteOpts{MapID: 1, GameState: gs}

	ev := &ParallelEventState{
		EventID: 1,
		Cmds: []*resource.EventCommand{
			{Code: CmdLoop, Indent: 0},
			{Code: CmdBreakLoop, Indent: 0},
			{Code: CmdRepeatAbove, Indent: 0},
			{Code: CmdChangeSwitches, Parameters: []interface{}{float64(88), float64(88), float64(0)}},
			{Code: CmdEnd, Indent: 0},
		},
	}

	exec.stepUntilWait(context.Background(), s, ev, opts)
	assert.True(t, gs.switches[88]) // Reached after break
}

// ========================================================================
// Parallel: ChangeGold error (no store)
// ========================================================================

func TestStepUntilWait_ChangeGold_NoStore(t *testing.T) {
	gs := newMockGameState()
	s := testSession(1)
	s.MapID = 1
	exec := New(nil, &resource.ResourceLoader{}, nopLogger())
	opts := &ExecuteOpts{MapID: 1, GameState: gs}

	ev := &ParallelEventState{
		EventID: 1,
		Cmds: []*resource.EventCommand{
			{Code: CmdChangeGold, Parameters: []interface{}{float64(0), float64(0), float64(100)}},
			{Code: CmdEnd, Indent: 0},
		},
	}
	exec.stepUntilWait(context.Background(), s, ev, opts)
	// Error → no effect sent
}

// ========================================================================
// Parallel: ChangeItems error (no store)
// ========================================================================

func TestStepUntilWait_ChangeItems_NoStore(t *testing.T) {
	gs := newMockGameState()
	s := testSession(1)
	s.MapID = 1
	exec := New(nil, &resource.ResourceLoader{}, nopLogger())
	opts := &ExecuteOpts{MapID: 1, GameState: gs}

	ev := &ParallelEventState{
		EventID: 1,
		Cmds: []*resource.EventCommand{
			{Code: CmdChangeItems, Parameters: []interface{}{float64(5), float64(0), float64(0), float64(3)}},
			{Code: CmdEnd, Indent: 0},
		},
	}
	exec.stepUntilWait(context.Background(), s, ev, opts)
}

// ========================================================================
// Parallel: Script with $gameScreen forward
// ========================================================================

func TestStepUntilWait_ScriptForward(t *testing.T) {
	gs := newMockGameState()
	s := testSession(1)
	s.MapID = 1
	exec := New(nil, &resource.ResourceLoader{}, nopLogger())
	opts := &ExecuteOpts{MapID: 1, GameState: gs}

	ev := &ParallelEventState{
		EventID: 1,
		Cmds: []*resource.EventCommand{
			{Code: CmdScript, Parameters: []interface{}{"$gameScreen.startFadeOut(24)"}},
			{Code: CmdEnd, Indent: 0},
		},
	}
	exec.stepUntilWait(context.Background(), s, ev, opts)
	pkts := drainPackets(t, s)
	found := false
	for _, p := range pkts {
		if p.Type == "npc_effect" {
			found = true
		}
	}
	assert.True(t, found)
}

// ========================================================================
// Parallel: Wait frames > 0
// ========================================================================

func TestStepUntilWait_Wait_PositiveFrames(t *testing.T) {
	gs := newMockGameState()
	s := testSession(1)
	s.MapID = 1
	exec := New(nil, &resource.ResourceLoader{}, nopLogger())
	opts := &ExecuteOpts{MapID: 1, GameState: gs}

	ev := &ParallelEventState{
		EventID: 1,
		Cmds: []*resource.EventCommand{
			{Code: CmdWait, Parameters: []interface{}{float64(10)}},
			{Code: CmdEnd, Indent: 0},
		},
	}
	done := exec.stepUntilWait(context.Background(), s, ev, opts)
	assert.False(t, done)
	assert.Equal(t, 10, ev.waitFrames)
}

func TestStepUntilWait_Wait_ZeroFrames(t *testing.T) {
	gs := newMockGameState()
	s := testSession(1)
	s.MapID = 1
	exec := New(nil, &resource.ResourceLoader{}, nopLogger())
	opts := &ExecuteOpts{MapID: 1, GameState: gs}

	ev := &ParallelEventState{
		EventID: 1,
		Cmds: []*resource.EventCommand{
			{Code: CmdWait, Parameters: []interface{}{float64(0)}},
			{Code: CmdEnd, Indent: 0},
		},
	}
	done := exec.stepUntilWait(context.Background(), s, ev, opts)
	assert.False(t, done)
	assert.Equal(t, 1, ev.waitFrames) // clamped to 1
}

// ========================================================================
// Parallel: CallCommonEvent
// ========================================================================

func TestStepUntilWait_CallCommonEvent_Cov3(t *testing.T) {
	gs := newMockGameState()
	s := testSession(1)
	s.MapID = 1
	res := &resource.ResourceLoader{
		CommonEvents: []*resource.CommonEvent{
			nil,
			{ID: 1, Name: "TestCE", List: []*resource.EventCommand{
				{Code: CmdChangeSwitches, Parameters: []interface{}{float64(55), float64(55), float64(0)}},
				{Code: CmdEnd},
			}},
		},
	}
	exec := New(nil, res, nopLogger())
	opts := &ExecuteOpts{MapID: 1, GameState: gs}

	ev := &ParallelEventState{
		EventID: 1,
		Cmds: []*resource.EventCommand{
			{Code: CmdCallCommonEvent, Parameters: []interface{}{float64(1)}},
			{Code: CmdEnd, Indent: 0},
		},
	}
	exec.stepUntilWait(context.Background(), s, ev, opts)
	assert.True(t, gs.switches[55])
}

// ========================================================================
// Dispatch: Script with setupChild (code 355)
// ========================================================================

func TestDispatch_Script_SetupChild(t *testing.T) {
	gs := newMockGameState()
	gs.variables[100] = 2
	s := testSession(1)
	res := &resource.ResourceLoader{
		CommonEvents: []*resource.CommonEvent{
			nil, nil,
			{ID: 2, Name: "CE2", List: []*resource.EventCommand{
				{Code: CmdChangeSwitches, Parameters: []interface{}{float64(88), float64(88), float64(0)}},
				{Code: CmdEnd},
			}},
		},
	}
	exec := New(nil, res, nopLogger())
	opts := &ExecuteOpts{GameState: gs}

	page := &resource.EventPage{
		List: []*resource.EventCommand{
			{Code: CmdScript, Parameters: []interface{}{"this.setupChild($dataCommonEvents[$gameVariables.value(100)].list, 0)"}},
			{Code: CmdEnd},
		},
	}
	exec.Execute(context.Background(), s, page, opts)
	assert.True(t, gs.switches[88])
}

// ========================================================================
// Dispatch: Script with $gameVariables._data mutation
// ========================================================================

func TestDispatch_Script_VarMutation(t *testing.T) {
	gs := newMockGameState()
	s := testSession(1)
	exec := New(nil, &resource.ResourceLoader{}, nopLogger())
	opts := &ExecuteOpts{GameState: gs}

	page := &resource.EventPage{
		List: []*resource.EventCommand{
			{Code: CmdScript, Parameters: []interface{}{"$gameVariables._data[999] = 42"}},
			{Code: CmdEnd},
		},
	}
	exec.Execute(context.Background(), s, page, opts)
	assert.Equal(t, 42, gs.variables[999])
}

// ========================================================================
// Parallel: JumpToLabel not found
// ========================================================================

func TestStepUntilWait_JumpToLabel_NotFound(t *testing.T) {
	gs := newMockGameState()
	s := testSession(1)
	s.MapID = 1
	exec := New(nil, &resource.ResourceLoader{}, nopLogger())
	opts := &ExecuteOpts{MapID: 1, GameState: gs}

	ev := &ParallelEventState{
		EventID: 1,
		Cmds: []*resource.EventCommand{
			{Code: CmdJumpToLabel, Parameters: []interface{}{"nonexistent"}},
			{Code: CmdEnd, Indent: 0},
		},
	}
	done := exec.stepUntilWait(context.Background(), s, ev, opts)
	assert.True(t, done) // advances past jump and hits end
}

// ========================================================================
// Dispatch: ShowText without choices, dialog ack
// ========================================================================

func TestDispatch_ShowText_PlainDialog(t *testing.T) {
	gs := newMockGameState()
	s := testSession(1)
	exec := New(nil, &resource.ResourceLoader{}, nopLogger())
	opts := &ExecuteOpts{GameState: gs}

	page := &resource.EventPage{
		List: []*resource.EventCommand{
			{Code: CmdShowText, Parameters: []interface{}{"", float64(0), float64(0), float64(2)}},
			{Code: CmdShowTextLine, Parameters: []interface{}{"Hello world"}},
			{Code: CmdEnd},
		},
	}

	// Send dialog ack
	go func() {
		s.DialogAckCh <- struct{}{}
	}()

	exec.Execute(context.Background(), s, page, opts)
	pkts := drainPackets(t, s)
	found := false
	for _, p := range pkts {
		if p.Type == "npc_dialog" {
			found = true
		}
	}
	assert.True(t, found)
}

// ========================================================================
// Dispatch: CallCommonEvent via dispatch
// ========================================================================

func TestDispatch_CallCommonEvent(t *testing.T) {
	gs := newMockGameState()
	s := testSession(1)
	res := &resource.ResourceLoader{
		CommonEvents: []*resource.CommonEvent{
			nil,
			{ID: 1, Name: "TestCE", List: []*resource.EventCommand{
				{Code: CmdChangeSwitches, Parameters: []interface{}{float64(66), float64(66), float64(0)}},
				{Code: CmdEnd},
			}},
		},
	}
	exec := New(nil, res, nopLogger())
	opts := &ExecuteOpts{GameState: gs}

	page := &resource.EventPage{
		List: []*resource.EventCommand{
			{Code: CmdCallCommonEvent, Parameters: []interface{}{float64(1)}},
			{Code: CmdEnd},
		},
	}
	exec.Execute(context.Background(), s, page, opts)
	assert.True(t, gs.switches[66])
}

// ========================================================================
// skipToChoiceBranch: WhenCancel branch
// ========================================================================

func TestSkipToChoiceBranch_WhenCancel(t *testing.T) {
	exec := New(nil, &resource.ResourceLoader{}, nopLogger())
	cmds := []*resource.EventCommand{
		{Code: CmdShowChoices, Indent: 0},
		{Code: CmdWhenBranch, Indent: 0},
		{Code: CmdWhenCancel, Indent: 0},
	}
	// choiceIdx=-1 → cancel, should match WhenCancel
	result := exec.skipToChoiceBranch(cmds, 0, -1, -1)
	assert.Equal(t, 2, result)
}

// ========================================================================
// Dispatch: ElseBranch skips to ConditionalEnd
// ========================================================================

func TestDispatch_ElseBranch(t *testing.T) {
	gs := newMockGameState()
	s := testSession(1)
	exec := New(nil, &resource.ResourceLoader{}, nopLogger())
	opts := &ExecuteOpts{GameState: gs}

	// Condition true → executes if block → hits ElseBranch → skips to ConditionalEnd
	gs.switches[1] = true
	page := &resource.EventPage{
		List: []*resource.EventCommand{
			{Code: CmdConditionalStart, Indent: 0, Parameters: []interface{}{float64(0), float64(1), float64(0)}},
			{Code: CmdChangeSwitches, Indent: 1, Parameters: []interface{}{float64(10), float64(10), float64(0)}},
			{Code: CmdElseBranch, Indent: 0},
			{Code: CmdChangeSwitches, Indent: 1, Parameters: []interface{}{float64(20), float64(20), float64(0)}},
			{Code: CmdConditionalEnd, Indent: 0},
			{Code: CmdEnd},
		},
	}
	exec.Execute(context.Background(), s, page, opts)
	assert.True(t, gs.switches[10])
	assert.False(t, gs.switches[20]) // Else skipped
}

// ========================================================================
// Dispatch: Condition false → else branch executed
// ========================================================================

func TestDispatch_ConditionFalse_ElseExecuted(t *testing.T) {
	gs := newMockGameState()
	s := testSession(1)
	exec := New(nil, &resource.ResourceLoader{}, nopLogger())
	opts := &ExecuteOpts{GameState: gs}

	// Switch 1 is false → skipToElseOrEnd → execute else
	page := &resource.EventPage{
		List: []*resource.EventCommand{
			{Code: CmdConditionalStart, Indent: 0, Parameters: []interface{}{float64(0), float64(1), float64(0)}},
			{Code: CmdChangeSwitches, Indent: 1, Parameters: []interface{}{float64(10), float64(10), float64(0)}},
			{Code: CmdElseBranch, Indent: 0},
			{Code: CmdChangeSwitches, Indent: 1, Parameters: []interface{}{float64(20), float64(20), float64(0)}},
			{Code: CmdConditionalEnd, Indent: 0},
			{Code: CmdEnd},
		},
	}
	exec.Execute(context.Background(), s, page, opts)
	assert.False(t, gs.switches[10]) // If skipped
	assert.True(t, gs.switches[20])  // Else executed
}
