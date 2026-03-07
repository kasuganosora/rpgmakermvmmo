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

// buildCE154KeyListLoop constructs the exact event commands from CE 154's keyList loop.
// This mirrors the real CommonEvents.json CE 154 structure:
//
//	[3] code=355 "window.keyTemp = 0;"
//	[4] code=112 Loop
//	[5] code=111 type=12 "keyTemp >= keyList.length"  → if true, break
//	[6] code=113 BreakLoop
//	[8] code=412 EndConditional
//	[9] code=122 var[4870] = script("keyList[keyTemp]")  operandType=4
//	[10] code=355 "$gameParty.gainItem($dataItems[$gameVariables.value(4870)],1)"
//	[11] code=101 ShowText face="" ...
//	[12] code=401 "获得了\\ii[\\V[4870]]，存入重要物品中。"
//	[13] code=355 "keyTemp++;"
//	[14] code=122 var[4870] = 0
//	[16] code=413 RepeatAbove
func buildCE154KeyListLoop() []*resource.EventCommand {
	return []*resource.EventCommand{
		// [0] window.keyTemp = 0;
		{Code: CmdScript, Indent: 0, Parameters: []interface{}{"window.keyTemp = 0;"}},
		// [1] Loop
		{Code: CmdLoop, Indent: 0},
		// [2] if (keyTemp >= keyList.length) → break
		{Code: CmdConditionalStart, Indent: 1, Parameters: []interface{}{
			float64(12), "keyTemp >= keyList.length",
		}},
		// [3] BreakLoop
		{Code: CmdBreakLoop, Indent: 2},
		// [4] EndConditional
		{Code: CmdConditionalEnd, Indent: 1},
		// [5] var[4870] = keyList[keyTemp] (operandType=4 = script)
		{Code: CmdChangeVars, Indent: 1, Parameters: []interface{}{
			float64(4870), float64(4870), float64(0), float64(4), "keyList[keyTemp]",
		}},
		// [6] $gameParty.gainItem(...)  — server has no gainItem, will error silently
		{Code: CmdScript, Indent: 1, Parameters: []interface{}{
			"$gameParty.gainItem($dataItems[$gameVariables.value(4870)],1)",
		}},
		// [7] ShowText (code 101)
		{Code: CmdShowText, Indent: 1, Parameters: []interface{}{"", float64(0), float64(0), float64(2)}},
		// [8] Text line (code 401)
		{Code: CmdShowTextLine, Indent: 1, Parameters: []interface{}{
			"获得了\\ii[\\V[4870]]，存入重要物品中。",
		}},
		// [9] keyTemp++;
		{Code: CmdScript, Indent: 1, Parameters: []interface{}{"keyTemp++;"}},
		// [10] var[4870] = 0
		{Code: CmdChangeVars, Indent: 1, Parameters: []interface{}{
			float64(4870), float64(4870), float64(0), float64(0), float64(0),
		}},
		// [11] RepeatAbove
		{Code: CmdRepeatAbove, Indent: 0},
		// [12] End
		{Code: CmdEnd, Indent: 0},
	}
}

// TestCE154_KeyListLoop_EmptyList verifies that when keyList is empty (default),
// the CE 154 loop exits immediately without showing any dialog.
// This is the core bug fix test: previously keyList was undefined → condition errored →
// loop body executed → keyTemp++ was dropped → infinite loop with \ii[0] dialogs.
func TestCE154_KeyListLoop_EmptyList(t *testing.T) {
	gs := newMockGameState()
	exec := New(nil, &resource.ResourceLoader{}, nopLogger())
	s := testSession(1)
	// Pre-fill DialogAckCh so if a dialog IS sent, it won't block forever
	s.DialogAckCh <- struct{}{}

	page := &resource.EventPage{List: buildCE154KeyListLoop()}

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	exec.Execute(ctx, s, page, &ExecuteOpts{GameState: gs})

	// Verify no dialog was sent (the loop should have exited immediately)
	pkts := drainPackets(t, s)
	for _, pkt := range pkts {
		assert.NotEqual(t, "npc_dialog", pkt.Type,
			"No dialog should be sent when keyList is empty, but got: %s", string(pkt.Payload))
	}

	// var[4870] should remain 0 (loop body never executed)
	assert.Equal(t, 0, gs.variables[4870], "var[4870] should be 0 (loop body never ran)")

	// Verify we didn't timeout (which would indicate an infinite loop)
	assert.NoError(t, ctx.Err(), "Should complete without timeout (no infinite loop)")
}

// TestCE154_KeyListLoop_WithItems verifies that when keyList has items,
// the loop iterates correctly and sends one dialog per item, then exits.
func TestCE154_KeyListLoop_WithItems(t *testing.T) {
	gs := newMockGameState()
	exec := New(nil, &resource.ResourceLoader{}, nopLogger())
	s := testSession(1)

	page := &resource.EventPage{List: buildCE154KeyListLoop()}

	// Run executor in background so we can respond to dialogs
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// We need to set keyList = [42, 99] before the loop runs.
	// Do this by prepending a script command that sets it.
	cmds := make([]*resource.EventCommand, 0, len(page.List)+1)
	cmds = append(cmds, &resource.EventCommand{
		Code: CmdScript, Indent: 0,
		Parameters: []interface{}{"window.keyList = [42, 99];"},
	})
	cmds = append(cmds, page.List...)
	page.List = cmds

	done := make(chan struct{})
	go func() {
		defer close(done)
		exec.Execute(ctx, s, page, &ExecuteOpts{GameState: gs})
	}()

	// Respond to exactly 2 dialogs (one per item in keyList)
	dialogCount := 0
	timeout := time.After(4 * time.Second)
	for dialogCount < 2 {
		select {
		case data := <-s.SendChan:
			var pkt player.Packet
			require.NoError(t, json.Unmarshal(data, &pkt))
			if pkt.Type == "npc_dialog" {
				dialogCount++
				// Send ack
				s.DialogAckCh <- struct{}{}
			}
		case <-timeout:
			t.Fatalf("Timeout waiting for dialog %d", dialogCount+1)
		}
	}

	// Wait for executor to finish
	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("Executor did not finish after all dialogs acked")
	}

	assert.Equal(t, 2, dialogCount, "Should show exactly 2 dialogs for 2 items")
	assert.NoError(t, ctx.Err(), "Should complete without timeout")
}

// TestCE154_WindowKeyTempSync verifies that window.keyTemp = 0 and bare keyTemp
// reference the same variable (window === globalThis in browser JS).
func TestCE154_WindowKeyTempSync(t *testing.T) {
	gs := newMockGameState()
	exec := New(nil, &resource.ResourceLoader{}, nopLogger())
	s := testSession(1)

	opts := &ExecuteOpts{GameState: gs}
	vm := exec.getOrCreateCondVM(s, opts)

	// window.keyTemp should be accessible as bare keyTemp
	v, err := vm.RunString("keyTemp")
	require.NoError(t, err)
	assert.Equal(t, int64(0), v.ToInteger(), "keyTemp should start at 0")

	// Setting via window. should update the bare variable
	_, err = vm.RunString("window.keyTemp = 5")
	require.NoError(t, err)
	v, err = vm.RunString("keyTemp")
	require.NoError(t, err)
	assert.Equal(t, int64(5), v.ToInteger(), "window.keyTemp=5 should sync to keyTemp")

	// keyList should be an empty array with length 0
	v, err = vm.RunString("keyList.length")
	require.NoError(t, err)
	assert.Equal(t, int64(0), v.ToInteger(), "keyList.length should be 0")

	// window.keyList should be the same array
	v, err = vm.RunString("window.keyList.length")
	require.NoError(t, err)
	assert.Equal(t, int64(0), v.ToInteger(), "window.keyList.length should be 0")

	// The break condition: 0 >= 0 should be true
	v, err = vm.RunString("keyTemp >= keyList.length")
	require.NoError(t, err)
	assert.True(t, v.ToBoolean(), "0 >= 0 should be true (break loop)")
}

// TestCE154_KeyTempIncrement verifies that keyTemp++ works in the condition VM
// (previously silently dropped because it didn't match any execScriptCommand pattern).
func TestCE154_KeyTempIncrement(t *testing.T) {
	gs := newMockGameState()
	exec := New(nil, &resource.ResourceLoader{}, nopLogger())
	s := testSession(1)

	opts := &ExecuteOpts{GameState: gs}
	vm := exec.getOrCreateCondVM(s, opts)

	// Simulate keyTemp++ being executed
	_, err := vm.RunString("keyTemp++")
	require.NoError(t, err)

	v, err := vm.RunString("keyTemp")
	require.NoError(t, err)
	assert.Equal(t, int64(1), v.ToInteger(), "keyTemp++ should increment to 1")

	// Do it again
	_, err = vm.RunString("keyTemp++")
	require.NoError(t, err)

	v, err = vm.RunString("keyTemp")
	require.NoError(t, err)
	assert.Equal(t, int64(2), v.ToInteger(), "keyTemp++ again should increment to 2")
}

// TestExecScriptCommand_BestEffort verifies that unmatched code 355 scripts
// are executed best-effort in the condition VM.
func TestExecScriptCommand_BestEffort(t *testing.T) {
	gs := newMockGameState()
	exec := New(nil, &resource.ResourceLoader{}, nopLogger())
	s := testSession(1)
	opts := &ExecuteOpts{GameState: gs}

	// "window.keyTemp = 42;" should be executed in condition VM
	handled := exec.execScriptCommand(context.Background(), s, "window.keyTemp = 42;", opts, 0)
	assert.False(t, handled, "Best-effort scripts return false (allow safe-line filter)")

	vm := exec.getOrCreateCondVM(s, opts)
	v, err := vm.RunString("keyTemp")
	require.NoError(t, err)
	assert.Equal(t, int64(42), v.ToInteger(), "window.keyTemp = 42 should be reflected globally")

	// "keyTemp++;" should also work
	exec.execScriptCommand(context.Background(), s, "keyTemp++;", opts, 0)
	v, err = vm.RunString("keyTemp")
	require.NoError(t, err)
	assert.Equal(t, int64(43), v.ToInteger(), "keyTemp++ should increment")

	// Invalid scripts should not panic
	exec.execScriptCommand(context.Background(), s, "$gameParty.gainItem(null, 1)", opts, 0)
	// No assertion needed — just verify no panic/crash
}

// TestContainsClientOnlyPluginCmd verifies detection of client-only plugin commands.
func TestContainsClientOnlyPluginCmd(t *testing.T) {
	// Event with hzChoiceEvent should be detected
	cmdsWithHz := []*resource.EventCommand{
		{Code: CmdScript, Parameters: []interface{}{"window.keyTemp = 0;"}},
		{Code: CmdPluginCommand, Parameters: []interface{}{"HzChoiceCustom foo disabled"}},
		{Code: CmdPluginCommand, Parameters: []interface{}{"hzChoiceEvent list location 2931"}},
		{Code: CmdEnd},
	}
	assert.True(t, ContainsClientOnlyPluginCmd(cmdsWithHz), "should detect hzChoiceEvent")

	// Event without hzChoiceEvent should not be detected
	cmdsWithout := []*resource.EventCommand{
		{Code: CmdPluginCommand, Parameters: []interface{}{"HzChoiceCustom foo disabled"}},
		{Code: CmdPluginCommand, Parameters: []interface{}{"CallCommon TestCE"}},
		{Code: CmdEnd},
	}
	assert.False(t, ContainsClientOnlyPluginCmd(cmdsWithout), "should not detect without hzChoiceEvent")

	// Empty list
	assert.False(t, ContainsClientOnlyPluginCmd(nil), "nil should return false")
}
