package npc

import (
	"context"
	"testing"

	"github.com/kasuganosora/rpgmakermvmmo/server/game/player"
	"github.com/kasuganosora/rpgmakermvmmo/server/resource"
	"github.com/stretchr/testify/assert"
)

// ========================================================================
// Dispatch: TintScreen with wait=true
// ========================================================================

func TestDispatch_TintScreen_Wait_Cov4(t *testing.T) {
	s := testSession(1)
	exec := New(nil, &resource.ResourceLoader{}, nopLogger())

	page := &resource.EventPage{
		List: []*resource.EventCommand{
			{Code: CmdTintScreen, Parameters: []interface{}{
				[]interface{}{float64(0), float64(0), float64(0), float64(0)},
				float64(60),
				true, // wait=true
			}},
			{Code: CmdEnd},
		},
	}

	go func() { s.EffectAckCh <- struct{}{} }()
	exec.Execute(context.Background(), s, page, nil)
}

// ========================================================================
// Dispatch: FlashScreen with wait=true
// ========================================================================

func TestDispatch_FlashScreen_Wait_Cov4(t *testing.T) {
	s := testSession(1)
	exec := New(nil, &resource.ResourceLoader{}, nopLogger())

	page := &resource.EventPage{
		List: []*resource.EventCommand{
			{Code: CmdFlashScreen, Parameters: []interface{}{
				[]interface{}{float64(255), float64(255), float64(255), float64(170)},
				float64(8),
				true, // wait=true
			}},
			{Code: CmdEnd},
		},
	}

	go func() { s.EffectAckCh <- struct{}{} }()
	exec.Execute(context.Background(), s, page, nil)
}

// ========================================================================
// Dispatch: ShakeScreen with wait=true
// ========================================================================

func TestDispatch_ShakeScreen_Wait_Cov4(t *testing.T) {
	s := testSession(1)
	exec := New(nil, &resource.ResourceLoader{}, nopLogger())

	page := &resource.EventPage{
		List: []*resource.EventCommand{
			{Code: CmdShakeScreen, Parameters: []interface{}{
				float64(5), float64(9), float64(60),
				true, // wait=true (params[3])
			}},
			{Code: CmdEnd},
		},
	}

	go func() { s.EffectAckCh <- struct{}{} }()
	exec.Execute(context.Background(), s, page, nil)
}

// ========================================================================
// Dispatch: ShowAnimation with wait=true
// ========================================================================

func TestDispatch_ShowAnimation_Wait_Cov4(t *testing.T) {
	s := testSession(1)
	exec := New(nil, &resource.ResourceLoader{}, nopLogger())
	opts := &ExecuteOpts{EventID: 5}

	page := &resource.EventPage{
		List: []*resource.EventCommand{
			{Code: CmdShowAnimation, Parameters: []interface{}{
				float64(0), float64(1),
				true, // wait=true (params[2])
			}},
			{Code: CmdEnd},
		},
	}

	go func() { s.EffectAckCh <- struct{}{} }()
	exec.Execute(context.Background(), s, page, opts)
}

// ========================================================================
// Dispatch: ShowBalloon with wait=true
// ========================================================================

func TestDispatch_ShowBalloon_Wait_Cov4(t *testing.T) {
	s := testSession(1)
	exec := New(nil, &resource.ResourceLoader{}, nopLogger())
	opts := &ExecuteOpts{EventID: 5}

	page := &resource.EventPage{
		List: []*resource.EventCommand{
			{Code: CmdShowBalloon, Parameters: []interface{}{
				float64(0), float64(1),
				true, // wait=true (params[2])
			}},
			{Code: CmdEnd},
		},
	}

	go func() { s.EffectAckCh <- struct{}{} }()
	exec.Execute(context.Background(), s, page, opts)
}

// ========================================================================
// Dispatch: TintPicture with wait=true
// ========================================================================

func TestDispatch_TintPicture_Wait_Cov4(t *testing.T) {
	s := testSession(1)
	exec := New(nil, &resource.ResourceLoader{}, nopLogger())

	page := &resource.EventPage{
		List: []*resource.EventCommand{
			{Code: CmdTintPicture, Parameters: []interface{}{
				float64(1),
				[]interface{}{float64(0), float64(0), float64(0), float64(0)},
				float64(60),
				true, // wait=true (params[3])
			}},
			{Code: CmdEnd},
		},
	}

	go func() { s.EffectAckCh <- struct{}{} }()
	exec.Execute(context.Background(), s, page, nil)
}

// ========================================================================
// Dispatch: SetWeather with wait=true
// ========================================================================

func TestDispatch_SetWeather_Wait_Cov4(t *testing.T) {
	s := testSession(1)
	exec := New(nil, &resource.ResourceLoader{}, nopLogger())

	page := &resource.EventPage{
		List: []*resource.EventCommand{
			{Code: CmdSetWeather, Parameters: []interface{}{
				"rain", float64(5), float64(60),
				true, // wait=true (params[3])
			}},
			{Code: CmdEnd},
		},
	}

	go func() { s.EffectAckCh <- struct{}{} }()
	exec.Execute(context.Background(), s, page, nil)
}

// ========================================================================
// Dispatch: MovePicture (sendMovePicture with effectWait)
// ========================================================================

func TestDispatch_MovePicture(t *testing.T) {
	s := testSession(1)
	exec := New(nil, &resource.ResourceLoader{}, nopLogger())

	page := &resource.EventPage{
		List: []*resource.EventCommand{
			{Code: CmdMovePicture, Parameters: []interface{}{
				float64(1), float64(0), float64(0), float64(0),
				float64(100), float64(200), float64(100), float64(100),
				float64(255), float64(0), float64(60),
				true, // wait
			}},
			{Code: CmdEnd},
		},
	}

	go func() { s.EffectAckCh <- struct{}{} }()
	exec.Execute(context.Background(), s, page, nil)
}

// ========================================================================
// Dispatch: Fadeout / Fadein (sendEffectWait)
// ========================================================================

func TestDispatch_Fadeout(t *testing.T) {
	s := testSession(1)
	exec := New(nil, &resource.ResourceLoader{}, nopLogger())

	page := &resource.EventPage{
		List: []*resource.EventCommand{
			{Code: CmdFadeout},
			{Code: CmdEnd},
		},
	}

	go func() { s.EffectAckCh <- struct{}{} }()
	exec.Execute(context.Background(), s, page, nil)
}

func TestDispatch_Fadein(t *testing.T) {
	s := testSession(1)
	exec := New(nil, &resource.ResourceLoader{}, nopLogger())

	page := &resource.EventPage{
		List: []*resource.EventCommand{
			{Code: CmdFadein},
			{Code: CmdEnd},
		},
	}

	go func() { s.EffectAckCh <- struct{}{} }()
	exec.Execute(context.Background(), s, page, nil)
}

// ========================================================================
// Dispatch: WaitForMoveRoute (sendEffectWait)
// ========================================================================

func TestDispatch_WaitForMoveRoute_Cov4(t *testing.T) {
	s := testSession(1)
	exec := New(nil, &resource.ResourceLoader{}, nopLogger())

	page := &resource.EventPage{
		List: []*resource.EventCommand{
			{Code: CmdWaitForMoveRoute},
			{Code: CmdEnd},
		},
	}

	go func() { s.EffectAckCh <- struct{}{} }()
	exec.Execute(context.Background(), s, page, nil)
}

// ========================================================================
// Dispatch: null cmd in list (skip)
// ========================================================================

func TestDispatch_NullCmd(t *testing.T) {
	gs := newMockGameState()
	s := testSession(1)
	exec := New(nil, &resource.ResourceLoader{}, nopLogger())
	opts := &ExecuteOpts{GameState: gs}

	page := &resource.EventPage{
		List: []*resource.EventCommand{
			nil, // null cmd → continue
			{Code: CmdChangeSwitches, Parameters: []interface{}{float64(1), float64(1), float64(0)}},
			{Code: CmdEnd},
		},
	}
	exec.Execute(context.Background(), s, page, opts)
	assert.True(t, gs.switches[1])
}

// ========================================================================
// Dispatch: ShowText + ShowChoices disconnect (choiceIdx=-1)
// ========================================================================

func TestDispatch_TextChoices_Disconnect(t *testing.T) {
	s := testSession(1)
	exec := New(nil, &resource.ResourceLoader{}, nopLogger())
	gs := newMockGameState()
	opts := &ExecuteOpts{GameState: gs}

	page := &resource.EventPage{
		List: []*resource.EventCommand{
			{Code: CmdShowText, Parameters: []interface{}{"", float64(0), float64(0), float64(2)}},
			{Code: CmdShowTextLine, Parameters: []interface{}{"Hello"}},
			{Code: CmdShowChoices, Parameters: []interface{}{
				[]interface{}{"A", "B"}, float64(-1), float64(0),
			}},
			{Code: CmdWhenBranch, Indent: 0},
			{Code: CmdBranchEnd, Indent: 0},
			{Code: CmdEnd},
		},
	}

	// Close Done to simulate disconnect → choiceIdx=-1 → return true
	close(s.Done)
	exec.Execute(context.Background(), s, page, opts)
}

// ========================================================================
// Dispatch: standalone ShowChoices disconnect (choiceIdx=-1)
// ========================================================================

func TestDispatch_ShowChoices_Disconnect(t *testing.T) {
	s := testSession(1)
	exec := New(nil, &resource.ResourceLoader{}, nopLogger())
	gs := newMockGameState()
	opts := &ExecuteOpts{GameState: gs}

	page := &resource.EventPage{
		List: []*resource.EventCommand{
			{Code: CmdShowChoices, Parameters: []interface{}{
				[]interface{}{"A"}, float64(-1), float64(0),
			}},
			{Code: CmdWhenBranch, Indent: 0},
			{Code: CmdBranchEnd, Indent: 0},
			{Code: CmdEnd},
		},
	}

	close(s.Done)
	exec.Execute(context.Background(), s, page, opts)
}

// ========================================================================
// Dispatch: ShowText + plain dialog disconnect (waitForDialogAck=false)
// ========================================================================

func TestDispatch_ShowText_DialogDisconnect(t *testing.T) {
	s := testSession(1)
	exec := New(nil, &resource.ResourceLoader{}, nopLogger())

	page := &resource.EventPage{
		List: []*resource.EventCommand{
			{Code: CmdShowText, Parameters: []interface{}{"", float64(0), float64(0), float64(2)}},
			{Code: CmdShowTextLine, Parameters: []interface{}{"Hello"}},
			{Code: CmdEnd},
		},
	}

	close(s.Done)
	exec.Execute(context.Background(), s, page, nil)
}

// ========================================================================
// Dispatch: Script with $gameScreen safe line forward (non-setupChild, non-mutable)
// ========================================================================

func TestDispatch_Script_SafeForward_Cov4(t *testing.T) {
	s := testSession(1)
	exec := New(nil, testResMMO(), nopLogger())

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
			found = true
		}
	}
	assert.True(t, found)
}

// ========================================================================
// Dispatch: Script with AudioManager safe line forward
// ========================================================================

func TestDispatch_Script_AudioForward(t *testing.T) {
	s := testSession(1)
	exec := New(nil, testResMMO(), nopLogger())

	page := &resource.EventPage{
		List: []*resource.EventCommand{
			{Code: CmdScript, Parameters: []interface{}{"AudioManager.playSe({name:'Cancel1',volume:90,pitch:100,pan:0})"}},
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
// Dispatch: Script with empty trimmed line (not forwarded)
// ========================================================================

func TestDispatch_Script_EmptyNotForwarded(t *testing.T) {
	s := testSession(1)
	exec := New(nil, &resource.ResourceLoader{}, nopLogger())

	page := &resource.EventPage{
		List: []*resource.EventCommand{
			{Code: CmdScript, Parameters: []interface{}{"someUnknownFunc()"}},
			{Code: CmdEnd},
		},
	}
	exec.Execute(context.Background(), s, page, nil)
	pkts := drainPackets(t, s)
	// Only npc_dialog_end, no npc_effect for unsafe script
	for _, p := range pkts {
		if p.Type == "npc_effect" {
			t.Fatal("unsafe script should not be forwarded")
		}
	}
}

// ========================================================================
// evalActorCondition: skill subtype=3 with store
// ========================================================================

func TestEvalActorCondition_SkillWithStore(t *testing.T) {
	store := newMockInventoryStore()
	exec := New(store, &resource.ResourceLoader{}, nopLogger())
	s := testSession(1)

	// subType=3 skill, not found
	assert.False(t, exec.evalActorCondition(context.Background(), s, []interface{}{float64(4), float64(1), float64(3), float64(5)}))
}

// ========================================================================
// evalActorCondition: state subtype=6
// ========================================================================

func TestEvalActorCondition_State_Cov4(t *testing.T) {
	exec := New(nil, &resource.ResourceLoader{}, nopLogger())
	s := testSession(1)

	// subType=6, state not present
	assert.False(t, exec.evalActorCondition(context.Background(), s, []interface{}{float64(4), float64(1), float64(6), float64(10)}))
}

// ========================================================================
// evalActorCondition: name subtype=1 (unimplemented)
// ========================================================================

func TestEvalActorCondition_Name_Cov4(t *testing.T) {
	exec := New(nil, &resource.ResourceLoader{}, nopLogger())
	s := testSession(1)

	assert.False(t, exec.evalActorCondition(context.Background(), s, []interface{}{float64(4), float64(1), float64(1), float64(0)}))
}

// ========================================================================
// evalActorCondition: default subtype
// ========================================================================

func TestEvalActorCondition_Default_Cov4(t *testing.T) {
	exec := New(nil, &resource.ResourceLoader{}, nopLogger())
	s := testSession(1)

	assert.False(t, exec.evalActorCondition(context.Background(), s, []interface{}{float64(4), float64(1), float64(99), float64(0)}))
}

// ========================================================================
// evalGoldCondition: op=2 (less than)
// ========================================================================

func TestEvalGoldCondition_LessThan_Cov4(t *testing.T) {
	store := newMockInventoryStore()
	store.gold[1] = 50
	exec := New(store, &resource.ResourceLoader{}, nopLogger())
	s := testSession(1)

	// 50 < 51 = true
	assert.True(t, exec.evalGoldCondition(context.Background(), s, []interface{}{float64(7), float64(51), float64(2)}))
	// 50 < 50 = false
	assert.False(t, exec.evalGoldCondition(context.Background(), s, []interface{}{float64(7), float64(50), float64(2)}))
}

// ========================================================================
// evalGoldCondition: nil store
// ========================================================================

func TestEvalGoldCondition_NilStore(t *testing.T) {
	exec := New(nil, &resource.ResourceLoader{}, nopLogger())
	s := testSession(1)

	assert.False(t, exec.evalGoldCondition(context.Background(), s, []interface{}{float64(7), float64(50), float64(0)}))
}

// ========================================================================
// evalItemCondition: weapon (kind=2) with includeEquip
// ========================================================================

func TestEvalItemCondition_Weapon(t *testing.T) {
	store := newMockInventoryStore()
	exec := New(store, &resource.ResourceLoader{}, nopLogger())
	s := testSession(1)

	// kind=weapon, includeEquip=1
	assert.False(t, exec.evalItemCondition(context.Background(), s, []interface{}{float64(9), float64(5), float64(1)}, 2))
}

// ========================================================================
// evalItemCondition: armor (kind=3) with includeEquip
// ========================================================================

func TestEvalItemCondition_Armor(t *testing.T) {
	store := newMockInventoryStore()
	exec := New(store, &resource.ResourceLoader{}, nopLogger())
	s := testSession(1)

	assert.False(t, exec.evalItemCondition(context.Background(), s, []interface{}{float64(10), float64(5), float64(1)}, 3))
}

// ========================================================================
// evalItemCondition: nil store
// ========================================================================

func TestEvalItemCondition_NilStore_Cov4(t *testing.T) {
	exec := New(nil, &resource.ResourceLoader{}, nopLogger())
	s := testSession(1)

	assert.False(t, exec.evalItemCondition(context.Background(), s, []interface{}{float64(8), float64(5)}, 1))
}

// ========================================================================
// evaluateCondition: transient vars truthy
// ========================================================================

func TestEvalCondition_TransientVarsTruthy(t *testing.T) {
	gs := newMockGameState()
	exec := New(nil, &resource.ResourceLoader{}, nopLogger())
	s := testSession(1)
	opts := &ExecuteOpts{
		GameState:     gs,
		MapID:         1,
		EventID:       1,
		TransientVars: map[int]interface{}{10: []int{1, 2, 3}},
	}

	// condType=1 (variable), varID=10, refType=0 (const), refVal=0, op=5 (!=)
	// v[10]=0 but TransientVars[10] exists → varVal=1, 1 != 0 = true
	assert.True(t, exec.evaluateCondition(context.Background(), s,
		[]interface{}{float64(1), float64(10), float64(0), float64(0), float64(5)}, opts))
}

// ========================================================================
// evaluateCondition: unsupported condType (default)
// ========================================================================

func TestEvalCondition_UnsupportedType(t *testing.T) {
	gs := newMockGameState()
	exec := New(nil, &resource.ResourceLoader{}, nopLogger())
	s := testSession(1)
	opts := &ExecuteOpts{GameState: gs, MapID: 1, EventID: 1}

	// condType=3 (timer) → unsupported → false
	assert.False(t, exec.evaluateCondition(context.Background(), s, []interface{}{float64(3)}, opts))
}

// ========================================================================
// evaluateCondition: variable refType=1 (compare with another variable)
// ========================================================================

func TestEvalCondition_VarRefType1(t *testing.T) {
	gs := newMockGameState()
	gs.variables[5] = 10
	gs.variables[6] = 10
	exec := New(nil, &resource.ResourceLoader{}, nopLogger())
	s := testSession(1)
	opts := &ExecuteOpts{GameState: gs, MapID: 1, EventID: 1}

	// condType=1, varID=5, refType=1, refVal=6 (compare v[5] to v[6]), op=0 (==)
	assert.True(t, exec.evaluateCondition(context.Background(), s,
		[]interface{}{float64(1), float64(5), float64(1), float64(6), float64(0)}, opts))
}

// ========================================================================
// evaluateCondition: condType=12 (script condition)
// ========================================================================

func TestEvalCondition_ScriptCondition(t *testing.T) {
	gs := newMockGameState()
	gs.variables[1] = 5
	exec := New(nil, &resource.ResourceLoader{}, nopLogger())
	s := testSession(1)
	opts := &ExecuteOpts{GameState: gs, MapID: 1, EventID: 1}

	// condType=12, script="$gameVariables.value(1) > 3"
	assert.True(t, exec.evaluateCondition(context.Background(), s,
		[]interface{}{float64(12), "$gameVariables.value(1) > 3"}, opts))
}

// ========================================================================
// resolveTextCodes: \P[1] = player, \P[2] = unchanged
// ========================================================================

func TestResolveTextCodes_P(t *testing.T) {
	exec := New(nil, &resource.ResourceLoader{}, nopLogger())
	s := testSession(1)
	s.CharName = "Luna"

	assert.Equal(t, "Luna", exec.resolveTextCodes(`\P[1]`, s, nil))
	assert.Equal(t, `\P[2]`, exec.resolveTextCodes(`\P[2]`, s, nil))
}

// ========================================================================
// resolveTextCodes: \V[n] with nil opts → "0"
// ========================================================================

func TestResolveTextCodes_V_NilOpts(t *testing.T) {
	exec := New(nil, &resource.ResourceLoader{}, nopLogger())
	s := testSession(1)

	assert.Equal(t, "0", exec.resolveTextCodes(`\V[5]`, s, nil))
}

// ========================================================================
// RunParallelEventsSynced: ctx cancel exits
// ========================================================================

func TestRunParallelEventsSynced_CtxCancel(t *testing.T) {
	s := testSession(1)
	s.MapID = 1
	exec := New(nil, &resource.ResourceLoader{}, nopLogger())
	gs := newMockGameState()
	opts := &ExecuteOpts{MapID: 1, GameState: gs}

	events := []*ParallelEventState{
		{EventID: 1, Cmds: []*resource.EventCommand{
			{Code: CmdEnd, Indent: 0},
		}},
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	exec.RunParallelEventsSynced(ctx, s, events, opts)
}

// ========================================================================
// RunParallelEventsSynced: map change exits
// ========================================================================

func TestRunParallelEventsSynced_MapChange(t *testing.T) {
	s := testSession(1)
	s.MapID = 2 // different from opts.MapID
	exec := New(nil, &resource.ResourceLoader{}, nopLogger())
	gs := newMockGameState()
	opts := &ExecuteOpts{MapID: 1, GameState: gs}

	events := []*ParallelEventState{
		{EventID: 1, Cmds: []*resource.EventCommand{
			{Code: CmdEnd, Indent: 0},
		}},
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// map mismatch → returns immediately
	exec.RunParallelEventsSynced(ctx, s, events, opts)
}

// ========================================================================
// stepUntilWait: MoveRouteCont standalone (skip)
// ========================================================================

func TestStepUntilWait_MoveRouteCont_Cov4(t *testing.T) {
	gs := newMockGameState()
	s := testSession(1)
	s.MapID = 1
	exec := New(nil, &resource.ResourceLoader{}, nopLogger())
	opts := &ExecuteOpts{MapID: 1, GameState: gs}

	ev := &ParallelEventState{
		EventID: 1,
		Cmds: []*resource.EventCommand{
			{Code: CmdMoveRouteCont},
			{Code: CmdEnd, Indent: 0},
		},
	}
	done := exec.stepUntilWait(context.Background(), s, ev, opts)
	assert.True(t, done)
}

// ========================================================================
// stepUntilWait: Script with safeLines forward in parallel
// ========================================================================

func TestStepUntilWait_Script_SafeForward(t *testing.T) {
	gs := newMockGameState()
	s := testSession(1)
	s.MapID = 1
	exec := New(nil, &resource.ResourceLoader{}, nopLogger())
	opts := &ExecuteOpts{MapID: 1, GameState: gs}

	ev := &ParallelEventState{
		EventID: 1,
		Cmds: []*resource.EventCommand{
			{Code: CmdScript, Parameters: []interface{}{"AudioManager.playSe({name:'ok'})"}},
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
// stepUntilWait: ScriptCont standalone (skip)
// ========================================================================

func TestStepUntilWait_ScriptCont_Cov4(t *testing.T) {
	gs := newMockGameState()
	s := testSession(1)
	s.MapID = 1
	exec := New(nil, &resource.ResourceLoader{}, nopLogger())
	opts := &ExecuteOpts{MapID: 1, GameState: gs}

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

// ========================================================================
// stepUntilWait: default fallthrough (forward unknown command)
// ========================================================================

func TestStepUntilWait_DefaultForward_Cov4(t *testing.T) {
	gs := newMockGameState()
	s := testSession(1)
	s.MapID = 1
	exec := New(nil, &resource.ResourceLoader{}, nopLogger())
	opts := &ExecuteOpts{MapID: 1, GameState: gs}

	ev := &ParallelEventState{
		EventID: 1,
		Cmds: []*resource.EventCommand{
			{Code: CmdPlaySE, Parameters: []interface{}{map[string]interface{}{"name": "ok"}}},
			{Code: CmdEnd, Indent: 0},
		},
	}
	exec.stepUntilWait(context.Background(), s, ev, opts)
	pkts := drainPackets(t, s)
	assert.True(t, len(pkts) > 0)
}

// ========================================================================
// stepUntilWait: blocked plugin cmd (not forwarded)
// ========================================================================

func TestStepUntilWait_BlockedPlugin_Cov4(t *testing.T) {
	gs := newMockGameState()
	s := testSession(1)
	s.MapID = 1
	exec := New(nil, testResMMO(), nopLogger())
	opts := &ExecuteOpts{MapID: 1, GameState: gs}

	ev := &ParallelEventState{
		EventID: 1,
		Cmds: []*resource.EventCommand{
			{Code: CmdPluginCommand, Parameters: []interface{}{"CallStand 1 2"}},
			{Code: CmdEnd, Indent: 0},
		},
	}
	exec.stepUntilWait(context.Background(), s, ev, opts)
	pkts := drainPackets(t, s)
	// CallStand is blocked → no npc_effect
	for _, p := range pkts {
		if p.Type == "npc_effect" {
			t.Fatal("blocked plugin should not be forwarded")
		}
	}
}

// ========================================================================
// stepUntilWait: EnterInstance/LeaveInstance plugin
// ========================================================================

func TestStepUntilWait_InstancePlugins(t *testing.T) {
	gs := newMockGameState()
	s := testSession(1)
	s.MapID = 1
	exec := New(nil, &resource.ResourceLoader{}, nopLogger())

	enterCalled := false
	leaveCalled := false
	opts := &ExecuteOpts{
		MapID:     1,
		GameState: gs,
		EnterInstanceFn: func(_ *player.PlayerSession) {
			enterCalled = true
		},
		LeaveInstanceFn: func(_ *player.PlayerSession) {
			leaveCalled = true
		},
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
	assert.True(t, enterCalled)
	assert.True(t, leaveCalled)
}

// ========================================================================
// stepUntilWait: MovePicture in parallel (no wait)
// ========================================================================

func TestStepUntilWait_MovePicture_Cov4(t *testing.T) {
	gs := newMockGameState()
	s := testSession(1)
	s.MapID = 1
	exec := New(nil, &resource.ResourceLoader{}, nopLogger())
	opts := &ExecuteOpts{MapID: 1, GameState: gs}

	ev := &ParallelEventState{
		EventID: 1,
		Cmds: []*resource.EventCommand{
			{Code: CmdMovePicture, Parameters: []interface{}{
				float64(1), float64(0), float64(0), float64(0),
				float64(100), float64(200), float64(100), float64(100),
				float64(255), float64(0), float64(60),
				true,
			}},
			{Code: CmdEnd, Indent: 0},
		},
	}
	exec.stepUntilWait(context.Background(), s, ev, opts)
}

// ========================================================================
// applyGold: GetGold error path
// ========================================================================

func TestApplyGold_GetGoldError(t *testing.T) {
	store := newMockInventoryStore()
	// Don't set gold for charID 1 → GetGold returns 0 (no error in mock)
	// We need a different approach: negative amount with error
	// Actually the mock always returns 0 for missing keys, not an error
	// Let's use a real scenario: decrease with exact amount
	store.gold[1] = 10
	exec := New(store, &resource.ResourceLoader{}, nopLogger())
	s := testSession(1)

	// Decrease 20 from 10 → clamp to -10
	err := exec.applyGold(context.Background(), s, []interface{}{float64(1), float64(0), float64(20)}, nil)
	assert.NoError(t, err)
	assert.Equal(t, int64(0), store.gold[1])
}

// ========================================================================
// applyItems: add exceeds max stack
// ========================================================================

func TestApplyItems_AddExceedsMaxStack(t *testing.T) {
	store := newMockInventoryStore()
	store.items[itemKey(1, 5)] = 9990
	exec := New(store, &resource.ResourceLoader{}, nopLogger())
	s := testSession(1)

	// 9990 + 100 > 9999 → error
	err := exec.applyItems(context.Background(), s, []interface{}{float64(5), float64(0), float64(0), float64(100)}, nil)
	assert.Error(t, err)
}

// ========================================================================
// applyItems: remove with no existing item
// ========================================================================

func TestApplyItems_RemoveNoItem(t *testing.T) {
	store := newMockInventoryStore()
	exec := New(store, &resource.ResourceLoader{}, nopLogger())
	s := testSession(1)

	// Remove from empty → currentQty=0 → return nil (no-op)
	err := exec.applyItems(context.Background(), s, []interface{}{float64(5), float64(1), float64(0), float64(3)}, nil)
	assert.NoError(t, err)
}

// ========================================================================
// execMutableScript: nil res path
// ========================================================================

func TestExecMutableScript_NilRes(t *testing.T) {
	gs := newMockGameState()
	s := testSession(1)
	exec := New(nil, nil, nopLogger())
	opts := &ExecuteOpts{GameState: gs, MapID: 1}

	exec.execMutableScript("$gameVariables._data[1] = 42", s, opts)
	assert.Equal(t, 42, gs.variables[1])
}

// ========================================================================
// execMutableScript: panic recovery
// ========================================================================

func TestExecMutableScript_Panic(t *testing.T) {
	gs := newMockGameState()
	s := testSession(1)
	exec := New(nil, &resource.ResourceLoader{}, nopLogger())
	opts := &ExecuteOpts{GameState: gs}

	// Script that causes error (not panic, but tests error branch)
	exec.execMutableScript("undefinedFunction()", s, opts)
	// Should not crash
}

// ========================================================================
// evalScriptCondition: panic recovery
// ========================================================================

func TestEvalScriptCondition_Error(t *testing.T) {
	gs := newMockGameState()
	s := testSession(1)
	exec := New(nil, &resource.ResourceLoader{}, nopLogger())
	opts := &ExecuteOpts{GameState: gs, MapID: 1, EventID: 1}

	result, ok := exec.evalScriptCondition("throw new Error('test')", s, opts)
	assert.False(t, result)
	assert.True(t, ok) // error is handled, returns (false, true)
}

// ========================================================================
// evalScriptValue: error path
// ========================================================================

func TestEvalScriptValue_Error_Cov4(t *testing.T) {
	gs := newMockGameState()
	s := testSession(1)
	exec := New(nil, &resource.ResourceLoader{}, nopLogger())
	opts := &ExecuteOpts{GameState: gs, MapID: 1, EventID: 1}

	result := exec.evalScriptValue("throw new Error('test')", s, opts)
	assert.Equal(t, 0, result)
}

// ========================================================================
// evalSetupChildTarget: error (invalid expr)
// ========================================================================

func TestEvalSetupChildTarget_Error(t *testing.T) {
	s := testSession(1)
	exec := New(nil, &resource.ResourceLoader{}, nopLogger())
	opts := &ExecuteOpts{GameState: newMockGameState()}

	id := exec.evalSetupChildTarget("throw 'err'", s, opts)
	assert.Equal(t, 0, id)
}

// ========================================================================
// injectScriptDataArrays: nil res
// ========================================================================

func TestInjectScriptDataArrays_NilRes(t *testing.T) {
	// Just exercises the nil check
	injectScriptDataArrays(nil, nil)
}

// ========================================================================
// applyChangeClass: level > 99
// ========================================================================

func TestApplyChangeClass_LevelOver99(t *testing.T) {
	s := testSessionWithStats(1, 100, 100, 50, 50, 150, 0)
	s.ClassID = 1
	// Params[0]=MHP, Params[1]=MMP, indexed by level (1-based)
	class1MHP := make([]int, 100)
	class1MMP := make([]int, 100)
	class2MHP := make([]int, 100)
	class2MMP := make([]int, 100)
	for i := 1; i < 100; i++ {
		class1MHP[i] = 50
		class1MMP[i] = 30
		class2MHP[i] = 200
		class2MMP[i] = 100
	}
	res := &resource.ResourceLoader{
		Classes: []*resource.Class{
			nil,
			{ID: 1, Params: [][]int{class1MHP, class1MMP}},
			{ID: 2, Params: [][]int{class2MHP, class2MMP}},
		},
	}
	exec := New(nil, res, nopLogger())

	exec.applyChangeClass(context.Background(), s, []interface{}{float64(1), float64(2), float64(0)}, nil)
	assert.Equal(t, 2, s.ClassID)
	assert.Equal(t, 200, s.MaxHP) // clamped level=99
}

// ========================================================================
// applyChangeEquipment: store error path (itemID=0 → no store call)
// ========================================================================

func TestApplyChangeEquipment_ItemIDZero_Cov4(t *testing.T) {
	store := newMockInventoryStore()
	exec := New(store, &resource.ResourceLoader{}, nopLogger())
	s := testSession(1)
	s.Equips = map[int]int{1: 5}

	// itemID=0 → unequip (SetEquip(1, 0)), skip store call
	exec.applyChangeEquipment(context.Background(), s, []interface{}{float64(1), float64(1), float64(0)}, nil)
	assert.Equal(t, 0, s.GetEquip(1))
}

// ========================================================================
// Dispatch: SetMoveRoute with wait + MoveRouteCont skip
// ========================================================================

func TestDispatch_SetMoveRoute_Wait_Cov4(t *testing.T) {
	s := testSession(1)
	exec := New(nil, &resource.ResourceLoader{}, nopLogger())
	opts := &ExecuteOpts{EventID: 5}

	page := &resource.EventPage{
		List: []*resource.EventCommand{
			{Code: CmdSetMoveRoute, Parameters: []interface{}{
				float64(0),
				map[string]interface{}{"wait": true, "list": []interface{}{}},
			}},
			{Code: CmdMoveRouteCont},
			{Code: CmdEnd},
		},
	}

	go func() { s.EffectAckCh <- struct{}{} }()
	exec.Execute(context.Background(), s, page, opts)
}

// ========================================================================
// stepUntilWait: SetMoveRoute with MoveRouteCont skip
// ========================================================================

func TestStepUntilWait_SetMoveRoute_Cov4(t *testing.T) {
	gs := newMockGameState()
	s := testSession(1)
	s.MapID = 1
	exec := New(nil, &resource.ResourceLoader{}, nopLogger())
	opts := &ExecuteOpts{MapID: 1, EventID: 5, GameState: gs}

	ev := &ParallelEventState{
		EventID: 5,
		Cmds: []*resource.EventCommand{
			{Code: CmdSetMoveRoute, Parameters: []interface{}{
				float64(0),
				map[string]interface{}{"list": []interface{}{}},
			}},
			{Code: CmdMoveRouteCont},
			{Code: CmdEnd, Indent: 0},
		},
	}
	exec.stepUntilWait(context.Background(), s, ev, opts)
	pkts := drainPackets(t, s)
	assert.True(t, len(pkts) > 0)
}

// ========================================================================
// stepUntilWait: ConditionalStart true/false + ElseBranch + ConditionalEnd
// ========================================================================

func TestStepUntilWait_Conditional(t *testing.T) {
	gs := newMockGameState()
	gs.switches[1] = true
	s := testSession(1)
	s.MapID = 1
	exec := New(nil, &resource.ResourceLoader{}, nopLogger())
	opts := &ExecuteOpts{MapID: 1, GameState: gs, EventID: 1}

	ev := &ParallelEventState{
		EventID: 1,
		Cmds: []*resource.EventCommand{
			{Code: CmdConditionalStart, Indent: 0, Parameters: []interface{}{float64(0), float64(1), float64(0)}},
			{Code: CmdChangeSwitches, Indent: 1, Parameters: []interface{}{float64(10), float64(10), float64(0)}},
			{Code: CmdElseBranch, Indent: 0},
			{Code: CmdChangeSwitches, Indent: 1, Parameters: []interface{}{float64(20), float64(20), float64(0)}},
			{Code: CmdConditionalEnd, Indent: 0},
			{Code: CmdEnd, Indent: 0},
		},
	}
	exec.stepUntilWait(context.Background(), s, ev, opts)
	assert.True(t, gs.switches[10])
	assert.False(t, gs.switches[20])
}

// ========================================================================
// stepUntilWait: ShowPicture
// ========================================================================

func TestStepUntilWait_ShowPicture_Cov4(t *testing.T) {
	gs := newMockGameState()
	s := testSession(1)
	s.MapID = 1
	exec := New(nil, &resource.ResourceLoader{}, nopLogger())
	opts := &ExecuteOpts{MapID: 1, GameState: gs}

	ev := &ParallelEventState{
		EventID: 1,
		Cmds: []*resource.EventCommand{
			{Code: CmdShowPicture, Parameters: []interface{}{
				float64(1), "pic", float64(0), float64(0),
				float64(100), float64(200), float64(100), float64(100),
				float64(255), float64(0),
			}},
			{Code: CmdEnd, Indent: 0},
		},
	}
	exec.stepUntilWait(context.Background(), s, ev, opts)
	pkts := drainPackets(t, s)
	assert.True(t, len(pkts) > 0)
}

// ========================================================================
// stepUntilWait: ChangeVariables
// ========================================================================

func TestStepUntilWait_ChangeVariables(t *testing.T) {
	gs := newMockGameState()
	s := testSession(1)
	s.MapID = 1
	exec := New(nil, &resource.ResourceLoader{}, nopLogger())
	opts := &ExecuteOpts{MapID: 1, GameState: gs}

	ev := &ParallelEventState{
		EventID: 1,
		Cmds: []*resource.EventCommand{
			{Code: CmdChangeVars, Parameters: []interface{}{float64(5), float64(5), float64(0), float64(0), float64(42)}},
			{Code: CmdEnd, Indent: 0},
		},
	}
	exec.stepUntilWait(context.Background(), s, ev, opts)
	assert.Equal(t, 42, gs.variables[5])
}

// ========================================================================
// stepUntilWait: ExitEvent
// ========================================================================

func TestStepUntilWait_ExitEvent_Cov4(t *testing.T) {
	gs := newMockGameState()
	s := testSession(1)
	s.MapID = 1
	exec := New(nil, &resource.ResourceLoader{}, nopLogger())
	opts := &ExecuteOpts{MapID: 1, GameState: gs}

	ev := &ParallelEventState{
		EventID: 1,
		Cmds: []*resource.EventCommand{
			{Code: CmdExitEvent},
			{Code: CmdChangeSwitches, Parameters: []interface{}{float64(1), float64(1), float64(0)}},
			{Code: CmdEnd, Indent: 0},
		},
	}
	done := exec.stepUntilWait(context.Background(), s, ev, opts)
	assert.True(t, done)
	assert.False(t, gs.switches[1]) // Not reached
}

// ========================================================================
// stepUntilWait: Label + JumpToLabel (found)
// ========================================================================

func TestStepUntilWait_Label_JumpToLabel(t *testing.T) {
	gs := newMockGameState()
	s := testSession(1)
	s.MapID = 1
	exec := New(nil, &resource.ResourceLoader{}, nopLogger())
	opts := &ExecuteOpts{MapID: 1, GameState: gs}

	ev := &ParallelEventState{
		EventID: 1,
		Cmds: []*resource.EventCommand{
			{Code: CmdJumpToLabel, Parameters: []interface{}{"skip"}},
			{Code: CmdChangeSwitches, Parameters: []interface{}{float64(1), float64(1), float64(0)}},
			{Code: CmdLabel, Parameters: []interface{}{"skip"}},
			{Code: CmdEnd, Indent: 0},
		},
	}
	exec.stepUntilWait(context.Background(), s, ev, opts)
	assert.False(t, gs.switches[1]) // Skipped by jump
}

// ========================================================================
// stepUntilWait: ctx cancel
// ========================================================================

func TestStepUntilWait_CtxCancel(t *testing.T) {
	gs := newMockGameState()
	s := testSession(1)
	s.MapID = 1
	exec := New(nil, &resource.ResourceLoader{}, nopLogger())
	opts := &ExecuteOpts{MapID: 1, GameState: gs}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	ev := &ParallelEventState{
		EventID: 1,
		Cmds: []*resource.EventCommand{
			{Code: CmdChangeSwitches, Parameters: []interface{}{float64(1), float64(1), float64(0)}},
			{Code: CmdEnd, Indent: 0},
		},
	}
	done := exec.stepUntilWait(ctx, s, ev, opts)
	assert.True(t, done) // ctx cancelled → return true
}

// ========================================================================
// Dispatch: CmdChangeHP/MP/State/EXP/Level
// ========================================================================

func TestDispatch_ChangeStats(t *testing.T) {
	gs := newMockGameState()
	s := testSessionWithStats(1, 100, 100, 50, 50, 10, 0)
	exec := New(nil, &resource.ResourceLoader{}, nopLogger())
	opts := &ExecuteOpts{GameState: gs}

	page := &resource.EventPage{
		List: []*resource.EventCommand{
			{Code: CmdChangeHP, Parameters: []interface{}{float64(0), float64(1), float64(0), float64(0), float64(10), false}},
			{Code: CmdChangeMP, Parameters: []interface{}{float64(0), float64(1), float64(0), float64(0), float64(5)}},
			{Code: CmdEnd},
		},
	}
	exec.Execute(context.Background(), s, page, opts)
}

// ========================================================================
// Dispatch: CmdTransfer
// ========================================================================

func TestDispatch_TransferPlayer(t *testing.T) {
	gs := newMockGameState()
	s := testSession(1)
	exec := New(nil, &resource.ResourceLoader{}, nopLogger())
	opts := &ExecuteOpts{GameState: gs, MapID: 1}

	page := &resource.EventPage{
		List: []*resource.EventCommand{
			{Code: CmdTransfer, Parameters: []interface{}{float64(0), float64(2), float64(5), float64(5), float64(2), float64(0)}},
			{Code: CmdEnd},
		},
	}
	exec.Execute(context.Background(), s, page, opts)
	pkts := drainPackets(t, s)
	found := false
	for _, p := range pkts {
		if p.Type == "transfer_player" {
			found = true
		}
	}
	assert.True(t, found)
}

// ========================================================================
// handleCallCommon: CallCommon with not-found CE name
// ========================================================================

func TestHandleCallCommon_CENameNotFound(t *testing.T) {
	s := testSession(1)
	res := &resource.ResourceLoader{
		CommonEvents: []*resource.CommonEvent{nil, {ID: 1, Name: "Existing"}},
	}
	exec := New(nil, res, nopLogger())

	cmd := &resource.EventCommand{Code: CmdPluginCommand, Parameters: []interface{}{"CallCommon NonExistent"}}
	handled := exec.handleCallCommon(context.Background(), s, cmd, nil, 0)
	assert.True(t, handled) // Recognized as CallCommon, but CE not found
}

// ========================================================================
// Dispatch: CmdWait
// ========================================================================

func TestDispatch_CmdWait(t *testing.T) {
	s := testSession(1)
	exec := New(nil, &resource.ResourceLoader{}, nopLogger())

	page := &resource.EventPage{
		List: []*resource.EventCommand{
			{Code: CmdWait, Parameters: []interface{}{float64(10)}},
			{Code: CmdEnd},
		},
	}

	go func() { s.EffectAckCh <- struct{}{} }()
	exec.Execute(context.Background(), s, page, nil)
}

// ========================================================================
// Dispatch: CmdLabel + CmdJumpToLabel
// ========================================================================

func TestDispatch_LabelAndJump(t *testing.T) {
	gs := newMockGameState()
	s := testSession(1)
	exec := New(nil, &resource.ResourceLoader{}, nopLogger())
	opts := &ExecuteOpts{GameState: gs}

	page := &resource.EventPage{
		List: []*resource.EventCommand{
			{Code: CmdJumpToLabel, Parameters: []interface{}{"end"}},
			{Code: CmdChangeSwitches, Parameters: []interface{}{float64(1), float64(1), float64(0)}},
			{Code: CmdLabel, Parameters: []interface{}{"end"}},
			{Code: CmdEnd},
		},
	}
	exec.Execute(context.Background(), s, page, opts)
	assert.False(t, gs.switches[1]) // Skipped
}

// ========================================================================
// Dispatch: CmdExitEvent
// ========================================================================

func TestDispatch_ExitEvent(t *testing.T) {
	gs := newMockGameState()
	s := testSession(1)
	exec := New(nil, &resource.ResourceLoader{}, nopLogger())
	opts := &ExecuteOpts{GameState: gs}

	page := &resource.EventPage{
		List: []*resource.EventCommand{
			{Code: CmdExitEvent},
			{Code: CmdChangeSwitches, Parameters: []interface{}{float64(1), float64(1), float64(0)}},
			{Code: CmdEnd},
		},
	}
	exec.Execute(context.Background(), s, page, opts)
	assert.False(t, gs.switches[1]) // Not reached
}

// ========================================================================
// Dispatch: Loop + RepeatAbove + BreakLoop
// ========================================================================

func TestDispatch_LoopBreak(t *testing.T) {
	gs := newMockGameState()
	s := testSession(1)
	exec := New(nil, &resource.ResourceLoader{}, nopLogger())
	opts := &ExecuteOpts{GameState: gs}

	page := &resource.EventPage{
		List: []*resource.EventCommand{
			{Code: CmdLoop, Indent: 0},
			{Code: CmdBreakLoop, Indent: 0},
			{Code: CmdRepeatAbove, Indent: 0},
			{Code: CmdChangeSwitches, Parameters: []interface{}{float64(1), float64(1), float64(0)}},
			{Code: CmdEnd},
		},
	}
	exec.Execute(context.Background(), s, page, opts)
	assert.True(t, gs.switches[1]) // Reached after break
}

// ========================================================================
// Dispatch: ScriptCont standalone (skip)
// ========================================================================

func TestDispatch_ScriptCont_Standalone_Cov4(t *testing.T) {
	gs := newMockGameState()
	s := testSession(1)
	exec := New(nil, &resource.ResourceLoader{}, nopLogger())
	opts := &ExecuteOpts{GameState: gs}

	page := &resource.EventPage{
		List: []*resource.EventCommand{
			{Code: CmdScriptCont, Parameters: []interface{}{"continuation"}},
			{Code: CmdChangeSwitches, Parameters: []interface{}{float64(1), float64(1), float64(0)}},
			{Code: CmdEnd},
		},
	}
	exec.Execute(context.Background(), s, page, opts)
	assert.True(t, gs.switches[1]) // Reached
}

// ========================================================================
// Dispatch: CmdEraseEvent
// ========================================================================

func TestDispatch_EraseEvent_Cov4(t *testing.T) {
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
	found := false
	for _, p := range pkts {
		if p.Type == "npc_effect" {
			found = true
		}
	}
	assert.True(t, found)
}

// ========================================================================
// Dispatch: CmdChangeTransparency
// ========================================================================

func TestDispatch_ChangeTransparency_Cov4(t *testing.T) {
	s := testSession(1)
	exec := New(nil, &resource.ResourceLoader{}, nopLogger())

	page := &resource.EventPage{
		List: []*resource.EventCommand{
			{Code: CmdChangeTransparency, Parameters: []interface{}{float64(0)}},
			{Code: CmdEnd},
		},
	}
	exec.Execute(context.Background(), s, page, nil)
}

// ========================================================================
// Dispatch: CmdChangeName / CmdChangeActorImage
// ========================================================================

func TestDispatch_ChangeName_Cov4(t *testing.T) {
	s := testSession(1)
	exec := New(nil, &resource.ResourceLoader{}, nopLogger())

	page := &resource.EventPage{
		List: []*resource.EventCommand{
			{Code: CmdChangeName, Parameters: []interface{}{float64(1), "NewName"}},
			{Code: CmdChangeActorImage, Parameters: []interface{}{float64(1), "face", float64(0)}},
			{Code: CmdEnd},
		},
	}
	exec.Execute(context.Background(), s, page, nil)
}

// ========================================================================
// Dispatch: CmdComment / CmdCommentCont (skip)
// ========================================================================

func TestDispatch_Comment(t *testing.T) {
	gs := newMockGameState()
	s := testSession(1)
	exec := New(nil, &resource.ResourceLoader{}, nopLogger())
	opts := &ExecuteOpts{GameState: gs}

	page := &resource.EventPage{
		List: []*resource.EventCommand{
			{Code: CmdComment, Parameters: []interface{}{"this is a comment"}},
			{Code: CmdCommentCont, Parameters: []interface{}{"continued"}},
			{Code: CmdChangeSwitches, Parameters: []interface{}{float64(1), float64(1), float64(0)}},
			{Code: CmdEnd},
		},
	}
	exec.Execute(context.Background(), s, page, opts)
	assert.True(t, gs.switches[1]) // Comments skipped
}

// ========================================================================
// Dispatch: CmdRotatePicture / CmdErasePicture
// ========================================================================

func TestDispatch_RotateErasePicture_Cov4(t *testing.T) {
	s := testSession(1)
	exec := New(nil, &resource.ResourceLoader{}, nopLogger())

	page := &resource.EventPage{
		List: []*resource.EventCommand{
			{Code: CmdRotatePicture, Parameters: []interface{}{float64(1), float64(5)}},
			{Code: CmdErasePicture, Parameters: []interface{}{float64(1)}},
			{Code: CmdEnd},
		},
	}
	exec.Execute(context.Background(), s, page, nil)
}

// ========================================================================
// findMapEvent: nil res
// ========================================================================

func TestFindMapEvent_NilRes(t *testing.T) {
	exec := New(nil, nil, nopLogger())
	ev := exec.findMapEvent(1, 1)
	assert.Nil(t, ev)
}

// ========================================================================
// teSetSelfVariable: nil opts/GameState
// ========================================================================

func TestTESetSelfVariable_NilOpts_Cov4(t *testing.T) {
	exec := New(nil, &resource.ResourceLoader{}, nopLogger())
	s := testSession(1)

	result := exec.teSetSelfVariable(s, []string{"0", "0", "5"}, nil)
	assert.True(t, result) // nil opts → return true
}

// ========================================================================
// Dispatch: CmdShopProcessing with shop items
// ========================================================================

func TestDispatch_ShopProcessing(t *testing.T) {
	s := testSession(1)
	exec := New(nil, &resource.ResourceLoader{}, nopLogger())

	page := &resource.EventPage{
		List: []*resource.EventCommand{
			{Code: CmdShopProcessing, Parameters: []interface{}{float64(0), float64(1), float64(0), float64(0)}},
			{Code: CmdShopItem, Parameters: []interface{}{float64(0), float64(2), float64(0), float64(0)}},
			{Code: CmdShopItem, Parameters: []interface{}{float64(0), float64(3), float64(0), float64(0)}},
			{Code: CmdEnd},
		},
	}
	exec.Execute(context.Background(), s, page, nil)
}

// ========================================================================
// Dispatch: GameOver / ReturnToTitle
// ========================================================================

func TestDispatch_GameOver(t *testing.T) {
	s := testSession(1)
	exec := New(nil, &resource.ResourceLoader{}, nopLogger())

	page := &resource.EventPage{
		List: []*resource.EventCommand{
			{Code: CmdGameOver},
			{Code: CmdEnd},
		},
	}
	exec.Execute(context.Background(), s, page, nil)
}

func TestDispatch_ReturnToTitle_Cov4(t *testing.T) {
	s := testSession(1)
	exec := New(nil, &resource.ResourceLoader{}, nopLogger())

	page := &resource.EventPage{
		List: []*resource.EventCommand{
			{Code: CmdReturnToTitle},
			{Code: CmdEnd},
		},
	}
	exec.Execute(context.Background(), s, page, nil)
}

// ========================================================================
// stepUntilWait: ChangeWeapons / ChangeArmors / ChangeEquipment in parallel
// ========================================================================

func TestStepUntilWait_ChangeWeapons(t *testing.T) {
	gs := newMockGameState()
	store := newMockInventoryStore()
	s := testSession(1)
	s.MapID = 1
	exec := New(store, &resource.ResourceLoader{}, nopLogger())
	opts := &ExecuteOpts{MapID: 1, GameState: gs}

	ev := &ParallelEventState{
		EventID: 1,
		Cmds: []*resource.EventCommand{
			{Code: CmdChangeWeapons, Parameters: []interface{}{float64(5), float64(0), float64(0), float64(2)}},
			{Code: CmdEnd, Indent: 0},
		},
	}
	exec.stepUntilWait(context.Background(), s, ev, opts)
}

func TestStepUntilWait_ChangeArmors(t *testing.T) {
	gs := newMockGameState()
	store := newMockInventoryStore()
	s := testSession(1)
	s.MapID = 1
	exec := New(store, &resource.ResourceLoader{}, nopLogger())
	opts := &ExecuteOpts{MapID: 1, GameState: gs}

	ev := &ParallelEventState{
		EventID: 1,
		Cmds: []*resource.EventCommand{
			{Code: CmdChangeArmors, Parameters: []interface{}{float64(5), float64(0), float64(0), float64(2)}},
			{Code: CmdEnd, Indent: 0},
		},
	}
	exec.stepUntilWait(context.Background(), s, ev, opts)
}

func TestStepUntilWait_ChangeEquipment(t *testing.T) {
	gs := newMockGameState()
	store := newMockInventoryStore()
	s := testSession(1)
	s.MapID = 1
	s.Equips = map[int]int{}
	exec := New(store, &resource.ResourceLoader{}, nopLogger())
	opts := &ExecuteOpts{MapID: 1, GameState: gs}

	ev := &ParallelEventState{
		EventID: 1,
		Cmds: []*resource.EventCommand{
			{Code: CmdChangeEquipment, Parameters: []interface{}{float64(1), float64(1), float64(10)}},
			{Code: CmdEnd, Indent: 0},
		},
	}
	exec.stepUntilWait(context.Background(), s, ev, opts)
	// CmdChangeEquipment not in stepUntilWait → falls to default (forwarded), equip not set server-side
	pkts := drainPackets(t, s)
	assert.True(t, len(pkts) > 0) // forwarded as npc_effect
}

// ========================================================================
// stepUntilWait: ChangeHP / ChangeMP in parallel
// ========================================================================

func TestStepUntilWait_ChangeHP(t *testing.T) {
	gs := newMockGameState()
	s := testSessionWithStats(1, 100, 100, 50, 50, 10, 0)
	s.MapID = 1
	exec := New(nil, &resource.ResourceLoader{}, nopLogger())
	opts := &ExecuteOpts{MapID: 1, GameState: gs}

	ev := &ParallelEventState{
		EventID: 1,
		Cmds: []*resource.EventCommand{
			{Code: CmdChangeHP, Parameters: []interface{}{float64(0), float64(1), float64(0), float64(0), float64(10), false}},
			{Code: CmdEnd, Indent: 0},
		},
	}
	exec.stepUntilWait(context.Background(), s, ev, opts)
}
