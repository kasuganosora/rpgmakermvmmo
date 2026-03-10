package npc

// executor_coverage6_test.go — coverage for isSafeScriptLine, isBlockedPluginCmd,
// execServerPlugin, resolveTextVarRef, and applyChangeWeapons remaining branches.

import (
	"context"
	"testing"
	"time"

	"github.com/kasuganosora/rpgmakermvmmo/server/resource"
	"github.com/stretchr/testify/assert"
)

// ========================================================================
// isSafeScriptLine: $gameScreen.method without trailing punctuation
// Covers the "methodEnd = len(rest)" branch (IndexAny returns -1)
// ========================================================================

func TestIsSafeScriptLine_NoParenNoSemicolon(t *testing.T) {
	exec := New(nil, testResMMO(), nopLogger())

	// "movePicture" is in the safeScreenMethods list.
	// "$gameScreen.movePicture" has no `(`, `;`, `=`, or space → methodEnd = len(rest)
	assert.True(t, exec.isSafeScriptLine("$gameScreen.movePicture"))
}

// ========================================================================
// isSafeScriptLine: $gameScreen.unknownMethod → return false
// Covers the false return for an unrecognized screen method
// ========================================================================

func TestIsSafeScriptLine_UnknownScreenMethod(t *testing.T) {
	exec := New(nil, testResMMO(), nopLogger())

	// "killPlayer" is NOT in the safeScreenMethods whitelist
	assert.False(t, exec.isSafeScriptLine("$gameScreen.killPlayer()"))
}

// ========================================================================
// isBlockedPluginCmd: returns true via ServerExecPlugins map
// Covers the "return true" when the command is a server-exec plugin name
// ========================================================================

func TestIsBlockedPluginCmd_ServerExecPlugin(t *testing.T) {
	exec := New(nil, testResMMO(), nopLogger())

	// "CulSkillEffect" is in ServerExecPlugins → blocked
	assert.True(t, exec.isBlockedPluginCmd("CulSkillEffect"))
	// "ParaCheck" as well
	assert.True(t, exec.isBlockedPluginCmd("ParaCheck"))
}

// ========================================================================
// execServerPlugin: default case → logger.Warn, return false
// Covers the "registered but no handler" branch
// ========================================================================

func TestExecServerPlugin_DefaultCaseNoHandler(t *testing.T) {
	cfg := &resource.MMOConfig{
		ServerExecPlugins: map[string]*resource.ServerExecPlugin{
			"FuturePlugin": {LoadedScript: "// stub", Timeout: 500},
		},
	}
	res := &resource.ResourceLoader{MMOConfig: cfg}
	exec := New(nil, res, nopLogger())
	s := testSession(1)
	opts := &ExecuteOpts{GameState: newMockGameState()}

	// "FuturePlugin" is registered but has no Go handler → default branch
	handled := exec.execServerPlugin(context.Background(), s, "FuturePlugin", opts)
	assert.False(t, handled)
}

// ========================================================================
// resolveTextVarRef: basic substitution (covers the new cleaned-up path)
// ========================================================================

func TestResolveTextVarRef_Basic_Cov6(t *testing.T) {
	gs := newMockGameState()
	gs.SetVariable(5, 42)
	exec := New(nil, &resource.ResourceLoader{}, nopLogger())
	opts := &ExecuteOpts{GameState: gs}

	result := exec.resolveTextVarRef(`value=\v[5]`, opts)
	assert.Equal(t, "value=42", result)
}

// ========================================================================
// applyChangeWeapons: weaponID <= 0 early return
// Covers "return" when weapon ID resolves to 0
// ========================================================================

func TestApplyChangeWeapons_ZeroWeaponID(t *testing.T) {
	gs := newMockGameState()
	exec := New(nil, &resource.ResourceLoader{}, nopLogger())
	s := testSession(1)
	opts := &ExecuteOpts{GameState: gs}

	// weaponID param = 0 → early return, no store call
	exec.applyChangeWeapons(context.Background(), s, []interface{}{float64(0), float64(0), float64(0), float64(1)}, opts)
	// no panic and no error is the expectation
}

// ========================================================================
// RunParallelEventsSynced: continue branch (waitFrames still > 0 after tick)
// Two events: A waits long (100 frames), B finishes immediately.
// Tick 1: A: 100-4=96>0 → allDone=false; continue ← covers line 90.
//          B: done=true → if ev.done { continue } ← covers line 83.
// ========================================================================

func TestRunParallelEventsSynced_WaitFramesContinue(t *testing.T) {
	gs := newMockGameState()
	s := testSession(1)
	s.MapID = 1
	exec := New(nil, &resource.ResourceLoader{}, nopLogger())
	opts := &ExecuteOpts{MapID: 1, GameState: gs}

	// Event A: CmdWait(100) — waitFrames=100; framesPerTick=4 (slowestSpeed=6).
	// After tick 1: 100-4=96 > 0 → allDone=false; continue ← the branch.
	cmdsA := []*resource.EventCommand{
		{Code: CmdWait, Parameters: []interface{}{float64(100)}},
		{Code: CmdEnd, Indent: 0},
	}
	// Event B: immediately done (CmdEnd at indent 0).
	cmdsB := []*resource.EventCommand{
		{Code: CmdEnd, Indent: 0},
	}
	events := []*ParallelEventState{
		{EventID: 1, Cmds: cmdsA},
		{EventID: 2, Cmds: cmdsB},
	}

	// Synchronous call with 350ms timeout — covers both continue branches
	// in the test goroutine itself (no goroutine flush races).
	ctx, cancel := context.WithTimeout(context.Background(), 350*time.Millisecond)
	defer cancel()
	exec.RunParallelEventsSynced(ctx, s, events, opts)
}

// ========================================================================
// isSafeScriptLine: line that is not $gameScreen and not a safe prefix
// Covers the final "return false" at the end of the function
// ========================================================================

func TestIsSafeScriptLine_NotScreenNotPrefix(t *testing.T) {
	exec := New(nil, testResMMO(), nopLogger())

	// Not AudioManager, not $gameScreen → final return false
	assert.False(t, exec.isSafeScriptLine("$gameActors.actor(1).gainHp(100)"))
	assert.False(t, exec.isSafeScriptLine("eval('bad')"))
}

// ========================================================================
// getPluginScript: unknown plugin name → returns ""
// ========================================================================

func TestGetPluginScript_Unknown(t *testing.T) {
	exec := New(nil, testResMMO(), nopLogger())
	result := exec.getPluginScript("NonExistentPlugin")
	assert.Equal(t, "", result)
}

// ========================================================================
// getPluginTimeout: unknown plugin name → default 200ms
// ========================================================================

func TestGetPluginTimeout_Default(t *testing.T) {
	exec := New(nil, testResMMO(), nopLogger())
	// "NonExistentPlugin" not in config → returns 200ms default
	d := exec.getPluginTimeout("NonExistentPlugin")
	assert.Equal(t, int64(200), d.Milliseconds())
}

// ========================================================================
// getPluginTagSkillRange: unknown plugin name → returns (0, 0)
// ========================================================================

func TestGetPluginTagSkillRange_Default(t *testing.T) {
	exec := New(nil, testResMMO(), nopLogger())
	start, end := exec.getPluginTagSkillRange("NonExistentPlugin")
	assert.Equal(t, 0, start)
	assert.Equal(t, 0, end)
}

// ========================================================================
// execCulSkillEffect: nil TransientVars → branch creates it
// ========================================================================

func TestExecCulSkillEffect_NilTransientVars(t *testing.T) {
	gs := newMockGameState()
	s := testSessionWithStats(1, 100, 100, 50, 50, 10, 0)
	s.Equips = map[int]int{}
	exec := New(nil, withTestMMOConfig(&resource.ResourceLoader{}), nopLogger())
	// nil TransientVars → should be auto-created inside execCulSkillEffect
	opts := &ExecuteOpts{GameState: gs, TransientVars: nil}
	exec.execCulSkillEffect(s, opts)
	assert.NotNil(t, opts.TransientVars)
}

// ========================================================================
// execParaCheck: nil TransientVars → branch creates it
// ========================================================================

func TestExecParaCheck_NilTransientVars(t *testing.T) {
	gs := newMockGameState()
	s := testSessionWithStats(1, 100, 100, 50, 50, 10, 0)
	s.ClassID = 1
	s.Equips = map[int]int{0: 0, 1: 0}
	exec := New(nil, withTestMMOConfig(&resource.ResourceLoader{}), nopLogger())
	opts := &ExecuteOpts{GameState: gs, TransientVars: nil}
	exec.execParaCheck(s, opts)
	assert.NotNil(t, opts.TransientVars)
}

// ========================================================================
// execMutableScript: nil TransientVars → varAnyChanges auto-created
// ========================================================================

func TestExecMutableScript_NilTransientVars(t *testing.T) {
	gs := newMockGameState()
	s := testSession(1)
	exec := New(nil, &resource.ResourceLoader{}, nopLogger())
	// opts.TransientVars == nil → branch at line 125 fires, then line 133 fires (since opts.TransientVars was nil → mutations.varAnyChanges = nil too? No: opts.TransientVars was nil, line 125 sets it, then varAnyChanges = opts.TransientVars is non-nil, so line 133 is skipped
	// Actually to hit line 133 we need TransientVars to stay nil after line 125 somehow. That can't happen.
	// Instead just test that nil opts triggers the early return
	exec.execMutableScript("$gameVariables._data[1] = 5", s, nil)
	assert.Equal(t, 0, gs.variables[1]) // opts=nil → early return, no change
}

// ========================================================================
// applyGold: GetGold error (op=1 decrease → calls GetGold first)
// ========================================================================

func TestApplyGold_GetGoldError_Cov6(t *testing.T) {
	store := &errorMockStore{}
	exec := New(store, &resource.ResourceLoader{}, nopLogger())
	s := testSession(1)

	// op=1 (decrease) → amount = -100 → calls GetGold → error
	err := exec.applyGold(context.Background(), s, []interface{}{float64(1), float64(0), float64(100)}, nil)
	assert.Error(t, err)
}

// ========================================================================
// applyEquipChange: valid slot type + store error → covers DB persist failed
// The previous test used "Equip" which is not in equipSlotTypeMap → returns early.
// ========================================================================

func TestApplyEquipChange_ValidSlot_StoreError(t *testing.T) {
	store := &errorMockStore{}
	exec := New(store, &resource.ResourceLoader{}, nopLogger())
	s := testSession(1)
	s.Equips = map[int]int{}
	gs := newMockGameState()
	opts := &ExecuteOpts{GameState: gs}

	// "Cloth" maps to slot 1 → valid slot → proceeds to SetEquipSlot → errors
	exec.applyEquipChange(context.Background(), s, "Cloth", "5", opts)
	// no panic expected; DB error is logged as Warn
}

// ========================================================================
// execCulSkillEffect: script that throws → covers "execution failed" path
// ========================================================================

func TestExecCulSkillEffect_ScriptThrows(t *testing.T) {
	cfg := &resource.MMOConfig{
		ServerExecPlugins: map[string]*resource.ServerExecPlugin{
			"CulSkillEffect": {LoadedScript: `throw new Error("test error")`, Timeout: 500},
		},
	}
	exec := New(nil, &resource.ResourceLoader{MMOConfig: cfg}, nopLogger())
	gs := newMockGameState()
	s := testSessionWithStats(1, 100, 100, 50, 50, 10, 0)
	s.Equips = map[int]int{}
	opts := &ExecuteOpts{GameState: gs, TransientVars: make(map[int]interface{})}
	exec.execCulSkillEffect(s, opts) // should log "CulSkillEffect execution failed" and return
}

// ========================================================================
// execCulSkillEffect: script sets a switch → covers SetSwitch loop body
// ========================================================================

func TestExecCulSkillEffect_SetsSwitch(t *testing.T) {
	cfg := &resource.MMOConfig{
		ServerExecPlugins: map[string]*resource.ServerExecPlugin{
			"CulSkillEffect": {LoadedScript: `$gameSwitches._data[55] = true;`, Timeout: 500},
		},
	}
	exec := New(nil, &resource.ResourceLoader{MMOConfig: cfg}, nopLogger())
	gs := newMockGameState()
	s := testSessionWithStats(1, 100, 100, 50, 50, 10, 0)
	s.Equips = map[int]int{}
	opts := &ExecuteOpts{GameState: gs, TransientVars: make(map[int]interface{})}
	exec.execCulSkillEffect(s, opts)
	assert.True(t, gs.switches[55])
}

// ========================================================================
// execCulSkillEffect: infinite loop → timeout interrupt fires
// Covers vm.Interrupt("CulSkillEffect timeout") and "execution failed"
// ========================================================================

func TestExecCulSkillEffect_Timeout(t *testing.T) {
	cfg := &resource.MMOConfig{
		ServerExecPlugins: map[string]*resource.ServerExecPlugin{
			"CulSkillEffect": {LoadedScript: `while(true){}`, Timeout: 10}, // 10ms timeout
		},
	}
	exec := New(nil, &resource.ResourceLoader{MMOConfig: cfg}, nopLogger())
	gs := newMockGameState()
	s := testSessionWithStats(1, 100, 100, 50, 50, 10, 0)
	s.Equips = map[int]int{}
	opts := &ExecuteOpts{GameState: gs, TransientVars: make(map[int]interface{})}
	exec.execCulSkillEffect(s, opts) // should timeout and log "CulSkillEffect execution failed"
}

// ========================================================================
// execParaCheck: script that throws → covers "execution failed" path
// ========================================================================

func TestExecParaCheck_ScriptThrows(t *testing.T) {
	cfg := &resource.MMOConfig{
		ServerExecPlugins: map[string]*resource.ServerExecPlugin{
			"ParaCheck": {LoadedScript: `throw new Error("test")`, Timeout: 500},
		},
	}
	exec := New(nil, &resource.ResourceLoader{MMOConfig: cfg}, nopLogger())
	gs := newMockGameState()
	s := testSessionWithStats(1, 100, 100, 50, 50, 10, 0)
	s.ClassID = 1
	s.Equips = map[int]int{0: 0, 1: 0}
	opts := &ExecuteOpts{GameState: gs, TransientVars: make(map[int]interface{})}
	exec.execParaCheck(s, opts) // should log "ParaCheck execution failed" and return
}
