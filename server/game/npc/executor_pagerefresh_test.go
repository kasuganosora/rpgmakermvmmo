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
// PageRefreshFn — mid-event NPC page refresh after switch/variable changes
// ========================================================================

// TestApplySwitches_CallsPageRefreshFn verifies that applySwitches triggers
// PageRefreshFn when switches actually change value.
func TestApplySwitches_CallsPageRefreshFn(t *testing.T) {
	e := New(nil, &resource.ResourceLoader{}, nopLogger())
	s := testSession(1)
	gs := newMockGameState()

	refreshCount := 0
	opts := &ExecuteOpts{
		GameState: gs,
		MapID:     68,
		EventID:   6,
		PageRefreshFn: func(ps *player.PlayerSession) {
			refreshCount++
			assert.Equal(t, s, ps)
		},
	}

	// Set switch 331 ON (was OFF) — should trigger refresh
	e.applySwitches(s, []interface{}{float64(331), float64(331), float64(0)}, opts)
	assert.True(t, gs.GetSwitch(331))
	assert.Equal(t, 1, refreshCount, "PageRefreshFn should be called once when switch changes")

	// Also verify switch_change packet was sent
	pkts := drainPackets(t, s)
	found := false
	for _, pkt := range pkts {
		if pkt.Type == "switch_change" {
			found = true
			var data map[string]interface{}
			json.Unmarshal(pkt.Payload, &data)
			assert.Equal(t, float64(331), data["id"])
			assert.Equal(t, true, data["value"])
		}
	}
	assert.True(t, found, "switch_change packet should be sent")
}

// TestApplySwitches_NoChangeNoRefresh verifies that PageRefreshFn is NOT
// called when switches don't actually change value.
func TestApplySwitches_NoChangeNoRefresh(t *testing.T) {
	e := New(nil, &resource.ResourceLoader{}, nopLogger())
	s := testSession(1)
	gs := newMockGameState()
	gs.SetSwitch(331, true) // already ON

	refreshCount := 0
	opts := &ExecuteOpts{
		GameState: gs,
		PageRefreshFn: func(_ *player.PlayerSession) {
			refreshCount++
		},
	}

	// Set switch 331 ON when it's already ON — no change
	e.applySwitches(s, []interface{}{float64(331), float64(331), float64(0)}, opts)
	assert.Equal(t, 0, refreshCount, "PageRefreshFn should not be called when switch doesn't change")
}

// TestApplySwitches_BatchRefresh verifies that setting multiple switches
// in a range triggers PageRefreshFn exactly once.
func TestApplySwitches_BatchRefresh(t *testing.T) {
	e := New(nil, &resource.ResourceLoader{}, nopLogger())
	s := testSession(1)
	gs := newMockGameState()

	refreshCount := 0
	opts := &ExecuteOpts{
		GameState: gs,
		PageRefreshFn: func(_ *player.PlayerSession) {
			refreshCount++
		},
	}

	// Set switches 331-333 ON — one batch call
	e.applySwitches(s, []interface{}{float64(331), float64(333), float64(0)}, opts)
	assert.True(t, gs.GetSwitch(331))
	assert.True(t, gs.GetSwitch(332))
	assert.True(t, gs.GetSwitch(333))
	assert.Equal(t, 1, refreshCount, "PageRefreshFn should be called once for a batch switch change")
}

// TestApplySwitches_NilPageRefreshFn verifies no panic when PageRefreshFn is nil.
func TestApplySwitches_NilPageRefreshFn(t *testing.T) {
	e := New(nil, &resource.ResourceLoader{}, nopLogger())
	s := testSession(1)
	gs := newMockGameState()
	opts := &ExecuteOpts{GameState: gs}

	// Should not panic
	e.applySwitches(s, []interface{}{float64(331), float64(331), float64(0)}, opts)
	assert.True(t, gs.GetSwitch(331))
}

// TestApplySelfSwitch_CallsPageRefreshFn verifies that self-switch changes
// trigger PageRefreshFn.
func TestApplySelfSwitch_CallsPageRefreshFn(t *testing.T) {
	e := New(nil, &resource.ResourceLoader{}, nopLogger())
	s := testSession(1)
	gs := newMockGameState()

	refreshCount := 0
	opts := &ExecuteOpts{
		GameState: gs,
		MapID:     68,
		EventID:   1,
		PageRefreshFn: func(_ *player.PlayerSession) {
			refreshCount++
		},
	}

	e.applySelfSwitch(s, []interface{}{"A", float64(0)}, opts) // 0 = ON
	assert.True(t, gs.GetSelfSwitch(68, 1, "A"))
	assert.Equal(t, 1, refreshCount)
}

// ========================================================================
// Map 68 scenario — mid-event switch changes for NPC visibility
// ========================================================================

// TestMap68_MidEventSwitchRefresh simulates the TE event that sets switches
// 331/332/333 ON mid-execution. Verifies PageRefreshFn is called for each.
func TestMap68_MidEventSwitchRefresh(t *testing.T) {
	store := newMockInventoryStore()
	resLoader := &resource.ResourceLoader{
		CommonEvents: make([]*resource.CommonEvent, 10),
	}
	e := New(store, resLoader, nopLogger())
	s := testSession(1)
	gs := newMockGameState()

	refreshCalls := []int{} // track switch IDs that caused refresh
	opts := &ExecuteOpts{
		GameState: gs,
		MapID:     68,
		EventID:   6,
		PageRefreshFn: func(_ *player.PlayerSession) {
			// Record which switches are ON at refresh time
			for _, id := range []int{331, 332, 333, 334} {
				if gs.GetSwitch(id) {
					refreshCalls = append(refreshCalls, id)
				}
			}
		},
	}

	// Simulate the TE event commands:
	// 1. First turn OFF switches 331-333 (monsters hidden at start)
	cmds := []*resource.EventCommand{
		{Code: CmdChangeSwitches, Parameters: []interface{}{float64(331), float64(333), float64(1)}}, // OFF
		// ... dialog commands would go here ...
		// 2. Turn ON switch 333 (monster 3 appears)
		{Code: CmdChangeSwitches, Parameters: []interface{}{float64(333), float64(333), float64(0)}}, // ON
		// 3. Turn ON switch 332 (monster 2 appears)
		{Code: CmdChangeSwitches, Parameters: []interface{}{float64(332), float64(332), float64(0)}}, // ON
		// 4. Turn ON switch 331 (monster 1 appears)
		{Code: CmdChangeSwitches, Parameters: []interface{}{float64(331), float64(331), float64(0)}}, // ON
		// 5. Turn ON switch 334 (blood stain appears)
		{Code: CmdChangeSwitches, Parameters: []interface{}{float64(334), float64(334), float64(0)}}, // ON
		{Code: CmdEnd, Parameters: []interface{}{}},
	}

	page := &resource.EventPage{List: cmds}
	e.Execute(context.Background(), s, page, opts)

	// Verify switches are all ON after execution
	assert.True(t, gs.GetSwitch(331))
	assert.True(t, gs.GetSwitch(332))
	assert.True(t, gs.GetSwitch(333))
	assert.True(t, gs.GetSwitch(334))

	// Verify PageRefreshFn was called at each switch change point
	// First call: switches 331-333 OFF (they were already OFF, so no refresh for that)
	// Then: 333 ON, 332 ON, 331 ON, 334 ON = 4 refresh calls
	assert.True(t, len(refreshCalls) > 0, "PageRefreshFn should have been called during execution")
}

// ========================================================================
// Code 203 — Set Event Location
// ========================================================================

// TestSetEventLocation_ForwardsAsEffect verifies code 203 is forwarded
// to client as npc_effect with resolved character ID.
func TestSetEventLocation_ForwardsAsEffect(t *testing.T) {
	resLoader := &resource.ResourceLoader{
		CommonEvents: make([]*resource.CommonEvent, 2),
	}
	e := New(nil, resLoader, nopLogger())
	s := testSession(1)

	cmds := []*resource.EventCommand{
		// Set event 3 to position (16, 23) facing right
		{Code: CmdSetEventLocation, Parameters: []interface{}{float64(3), float64(0), float64(16), float64(23), float64(6)}},
		{Code: CmdEnd, Parameters: []interface{}{}},
	}

	page := &resource.EventPage{List: cmds}
	e.Execute(context.Background(), s, page, &ExecuteOpts{
		GameState: newMockGameState(),
		MapID:     68,
		EventID:   6,
	})

	// Find the npc_effect packet for code 203
	pkts := drainPackets(t, s)
	found := false
	for _, pkt := range pkts {
		if pkt.Type == "npc_effect" {
			var data map[string]interface{}
			json.Unmarshal(pkt.Payload, &data)
			if data["code"] != nil && int(data["code"].(float64)) == 203 {
				found = true
				params := data["params"].([]interface{})
				assert.Equal(t, float64(3), params[0], "charId should be 3")
				assert.Equal(t, float64(0), params[1], "designation type should be direct")
				assert.Equal(t, float64(16), params[2], "x should be 16")
				assert.Equal(t, float64(23), params[3], "y should be 23")
				assert.Equal(t, float64(6), params[4], "direction should be 6 (right)")
			}
		}
	}
	assert.True(t, found, "code 203 should be forwarded as npc_effect")
}

// TestSetEventLocation_ResolvesCharID0 verifies that charID=0 is resolved
// to the current event ID.
func TestSetEventLocation_ResolvesCharID0(t *testing.T) {
	resLoader := &resource.ResourceLoader{
		CommonEvents: make([]*resource.CommonEvent, 2),
	}
	e := New(nil, resLoader, nopLogger())
	s := testSession(1)

	cmds := []*resource.EventCommand{
		// charID=0 means "this event" → should be resolved to EventID 6
		{Code: CmdSetEventLocation, Parameters: []interface{}{float64(0), float64(0), float64(10), float64(20), float64(2)}},
		{Code: CmdEnd, Parameters: []interface{}{}},
	}

	page := &resource.EventPage{List: cmds}
	e.Execute(context.Background(), s, page, &ExecuteOpts{
		GameState: newMockGameState(),
		MapID:     68,
		EventID:   6,
	})

	pkts := drainPackets(t, s)
	for _, pkt := range pkts {
		if pkt.Type == "npc_effect" {
			var data map[string]interface{}
			json.Unmarshal(pkt.Payload, &data)
			if data["code"] != nil && int(data["code"].(float64)) == 203 {
				params := data["params"].([]interface{})
				assert.Equal(t, float64(6), params[0], "charID=0 should be resolved to EventID 6")
				return
			}
		}
	}
	t.Fatal("code 203 npc_effect not found")
}

// ========================================================================
// Full Map 68 TE event flow simulation
// ========================================================================

// TestMap68_TEEventFlow_SwitchSequence simulates the complete switch
// manipulation sequence from template event 95 (危険な帰り道2).
// This is a comprehensive test covering the flow:
// 1. Turn OFF switches 331-333 (hide monsters)
// 2. Turn ON switch 333 (show monster 3)
// 3. Turn ON switch 332 (show monster 2)
// 4. Turn ON switch 331 (show monster 1)
// 5. Turn ON switch 334 (show blood stain)
func TestMap68_TEEventFlow_SwitchSequence(t *testing.T) {
	resLoader := &resource.ResourceLoader{
		CommonEvents: make([]*resource.CommonEvent, 10),
	}
	e := New(nil, resLoader, nopLogger())
	s := testSession(1)
	gs := newMockGameState()
	// Pre-set some switches ON to verify they get turned OFF first
	gs.SetSwitch(331, true)
	gs.SetSwitch(332, true)

	type refreshSnapshot struct {
		sw331, sw332, sw333, sw334 bool
	}
	var snapshots []refreshSnapshot

	opts := &ExecuteOpts{
		GameState: gs,
		MapID:     68,
		EventID:   6,
		PageRefreshFn: func(_ *player.PlayerSession) {
			snapshots = append(snapshots, refreshSnapshot{
				sw331: gs.GetSwitch(331),
				sw332: gs.GetSwitch(332),
				sw333: gs.GetSwitch(333),
				sw334: gs.GetSwitch(334),
			})
		},
	}

	cmds := []*resource.EventCommand{
		// Step 1: Turn OFF switches 331-333 (hide monsters before repositioning)
		{Code: CmdChangeSwitches, Parameters: []interface{}{float64(331), float64(333), float64(1)}}, // 1=OFF

		// Step 2: Set event locations (code 203) — reposition monsters
		{Code: CmdSetEventLocation, Parameters: []interface{}{float64(3), float64(0), float64(16), float64(23), float64(6)}},
		{Code: CmdSetEventLocation, Parameters: []interface{}{float64(4), float64(0), float64(15), float64(23), float64(6)}},

		// Step 3: Turn ON switch 333 (monster 3 appears at new position)
		{Code: CmdChangeSwitches, Parameters: []interface{}{float64(333), float64(333), float64(0)}}, // 0=ON

		// Step 4: Turn ON switch 332 (monster 2 appears)
		{Code: CmdChangeSwitches, Parameters: []interface{}{float64(332), float64(332), float64(0)}}, // 0=ON

		// Step 5: Turn ON switch 331 (monster 1 appears)
		{Code: CmdChangeSwitches, Parameters: []interface{}{float64(331), float64(331), float64(0)}}, // 0=ON

		// Step 6: Turn ON switch 334 (blood stain)
		{Code: CmdChangeSwitches, Parameters: []interface{}{float64(334), float64(334), float64(0)}}, // 0=ON

		{Code: CmdEnd, Parameters: []interface{}{}},
	}

	page := &resource.EventPage{List: cmds}
	e.Execute(context.Background(), s, page, opts)

	// Verify all switches are ON after execution
	require.True(t, gs.GetSwitch(331))
	require.True(t, gs.GetSwitch(332))
	require.True(t, gs.GetSwitch(333))
	require.True(t, gs.GetSwitch(334))

	// Verify refresh snapshots show progressive switch activation
	require.GreaterOrEqual(t, len(snapshots), 5,
		"PageRefreshFn should be called at least 5 times (1 OFF batch + 4 ON changes)")

	// First snapshot: after turning OFF 331-333 (since 331/332 were ON, this is a change)
	assert.False(t, snapshots[0].sw331, "snapshot 0: switch 331 should be OFF")
	assert.False(t, snapshots[0].sw332, "snapshot 0: switch 332 should be OFF")
	assert.False(t, snapshots[0].sw333, "snapshot 0: switch 333 should be OFF")

	// Last snapshot: all should be ON
	last := snapshots[len(snapshots)-1]
	assert.True(t, last.sw334, "last snapshot: switch 334 should be ON")

	// Verify code 203 packets were sent
	pkts := drainPackets(t, s)
	code203Count := 0
	for _, pkt := range pkts {
		if pkt.Type == "npc_effect" {
			var data map[string]interface{}
			json.Unmarshal(pkt.Payload, &data)
			if data["code"] != nil && int(data["code"].(float64)) == 203 {
				code203Count++
			}
		}
	}
	assert.Equal(t, 2, code203Count, "should have 2 code 203 (Set Event Location) packets")
}

// TestMap68_DialogWithSwitchChanges tests that dialog + switch changes
// interleave correctly: dialog is shown, then switches change and
// PageRefreshFn fires.
func TestMap68_DialogWithSwitchChanges(t *testing.T) {
	resLoader := &resource.ResourceLoader{
		CommonEvents: make([]*resource.CommonEvent, 10),
	}
	e := New(nil, resLoader, nopLogger())
	s := testSession(1)
	gs := newMockGameState()

	refreshCalled := false
	opts := &ExecuteOpts{
		GameState: gs,
		MapID:     68,
		EventID:   6,
		PageRefreshFn: func(_ *player.PlayerSession) {
			refreshCalled = true
		},
	}

	cmds := []*resource.EventCommand{
		// Show dialog
		{Code: CmdShowText, Parameters: []interface{}{"", float64(0), float64(0), float64(2)}},
		{Code: CmdShowTextLine, Parameters: []interface{}{"怪物出现了！"}},
		// After dialog ack, turn ON switch 333
		{Code: CmdChangeSwitches, Parameters: []interface{}{float64(333), float64(333), float64(0)}},
		{Code: CmdEnd, Parameters: []interface{}{}},
	}

	// Run in goroutine since dialog waits for ack
	done := make(chan struct{})
	go func() {
		defer close(done)
		page := &resource.EventPage{List: cmds}
		e.Execute(context.Background(), s, page, opts)
	}()

	// Wait for dialog packet
	pkt := recvPacket(t, s, 2*time.Second)
	require.Equal(t, "npc_dialog", pkt.Type)

	// Send dialog ack
	s.DialogAckCh <- struct{}{}

	// Wait for execution to complete
	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("timeout waiting for execution")
	}

	assert.True(t, gs.GetSwitch(333), "switch 333 should be ON")
	assert.True(t, refreshCalled, "PageRefreshFn should be called after switch change")
}

// TestPluginCommand_PortraitBlocked verifies that portrait-related plugin commands
// (CallStand, CallCutin, EraceStand, EraceCutin, CallAM) are blocked from forwarding.
// These are managed by client-side parallel CE 201 → CE 210 → CallStand chain.
// Server forwarding conflicts with the client's 10-frame CE 201 loop timing.
func TestPluginCommand_PortraitBlocked(t *testing.T) {
	resLoader := withTestMMOConfig(&resource.ResourceLoader{
		CommonEvents: make([]*resource.CommonEvent, 2),
	})
	e := New(nil, resLoader, nopLogger())
	s := testSession(1)

	cmds := []*resource.EventCommand{
		{Code: CmdPluginCommand, Parameters: []interface{}{"CallStand 1 0 0"}},
		{Code: CmdPluginCommand, Parameters: []interface{}{"CallStandForce 1"}},
		{Code: CmdPluginCommand, Parameters: []interface{}{"CallCutin 1"}},
		{Code: CmdPluginCommand, Parameters: []interface{}{"EraceStand"}},
		{Code: CmdPluginCommand, Parameters: []interface{}{"EraceStand1"}},
		{Code: CmdPluginCommand, Parameters: []interface{}{"EraceCutin"}},
		{Code: CmdPluginCommand, Parameters: []interface{}{"CallAM 1"}},
		// FaceId should still be forwarded (sets var[895] for portrait expression).
		{Code: CmdPluginCommand, Parameters: []interface{}{"FaceId 1 6"}},
		{Code: CmdEnd, Parameters: []interface{}{}},
	}

	page := &resource.EventPage{List: cmds}
	e.Execute(context.Background(), s, page, &ExecuteOpts{
		GameState: newMockGameState(),
		MapID:     68,
		EventID:   6,
	})

	pkts := drainPackets(t, s)
	var cmdsForwarded []string
	for _, pkt := range pkts {
		if pkt.Type == "npc_effect" {
			var data map[string]interface{}
			json.Unmarshal(pkt.Payload, &data)
			if data["code"] != nil && int(data["code"].(float64)) == 356 {
				params := data["params"].([]interface{})
				cmdsForwarded = append(cmdsForwarded, params[0].(string))
			}
		}
	}

	// Portrait commands should be blocked.
	assert.NotContains(t, cmdsForwarded, "CallStand 1 0 0")
	assert.NotContains(t, cmdsForwarded, "CallStandForce 1")
	assert.NotContains(t, cmdsForwarded, "CallCutin 1")
	assert.NotContains(t, cmdsForwarded, "EraceStand")
	assert.NotContains(t, cmdsForwarded, "EraceStand1")
	assert.NotContains(t, cmdsForwarded, "EraceCutin")
	assert.NotContains(t, cmdsForwarded, "CallAM 1")

	// FaceId should still be forwarded — it sets var[895] for the portrait expression.
	assert.Contains(t, cmdsForwarded, "FaceId 1 6")
}
