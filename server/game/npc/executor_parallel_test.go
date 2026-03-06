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

// ---- Helper: build Map 110-style parallel event commands ----

// makeParallelMoveCommands creates commands matching Map 110's pattern:
// Loop { SetMoveRoute(charId, [MoveRight]), Wait(1), if Switch OFF → Break }
func makeParallelMoveCommands(charID int, switchID int) []*resource.EventCommand {
	moveRoute := map[string]interface{}{
		"list": []interface{}{
			map[string]interface{}{"code": float64(3), "indent": nil}, // ROUTE_MOVE_RIGHT
			map[string]interface{}{"code": float64(0)},               // ROUTE_END
		},
		"repeat":    false,
		"skippable": false,
		"wait":      false,
	}
	return []*resource.EventCommand{
		{Code: CmdLoop, Indent: 0},                                                           // [0] Loop
		{Code: CmdSetMoveRoute, Indent: 1, Parameters: []interface{}{float64(charID), moveRoute}}, // [1] SetMoveRoute
		{Code: CmdMoveRouteCont, Indent: 1, Parameters: []interface{}{map[string]interface{}{"code": float64(3), "indent": nil}}}, // [2] 505
		{Code: CmdWait, Indent: 1, Parameters: []interface{}{float64(1)}},                    // [3] Wait(1)
		{Code: CmdConditionalStart, Indent: 1, Parameters: []interface{}{float64(0), float64(switchID), float64(1)}}, // [4] If Switch OFF
		{Code: CmdBreakLoop, Indent: 2},                                                       // [5] Break
		{Code: CmdEnd, Indent: 2},                                                             // [6] End (inner)
		{Code: CmdConditionalEnd, Indent: 1},                                                  // [7] End Conditional
		{Code: CmdEnd, Indent: 1},                                                             // [8] End (loop body)
		{Code: CmdRepeatAbove, Indent: 0},                                                     // [9] RepeatAbove
		{Code: CmdEnd, Indent: 0},                                                             // [10] End (list)
	}
}

// parseNPCEffect extracts code, params, and map_id from an npc_effect packet.
func parseNPCEffect(t *testing.T, pkt *player.Packet) (int, []interface{}, int) {
	t.Helper()
	require.Equal(t, "npc_effect", pkt.Type)
	var data map[string]interface{}
	require.NoError(t, json.Unmarshal(pkt.Payload, &data))
	code := int(data["code"].(float64))
	params, _ := data["params"].([]interface{})
	mapID := 0
	if mid, ok := data["map_id"]; ok && mid != nil {
		mapID = int(mid.(float64))
	}
	return code, params, mapID
}

// extractMoveRouteList extracts the move route list from SetMoveRoute params.
func extractMoveRouteList(t *testing.T, params []interface{}) []map[string]interface{} {
	t.Helper()
	require.True(t, len(params) >= 2, "SetMoveRoute needs at least 2 params")
	mr, ok := params[1].(map[string]interface{})
	require.True(t, ok, "params[1] should be a map")
	list, ok := mr["list"].([]interface{})
	require.True(t, ok, "move route should have list")
	var result []map[string]interface{}
	for _, item := range list {
		m, ok := item.(map[string]interface{})
		require.True(t, ok)
		result = append(result, m)
	}
	return result
}

// ---- Tests ----

func TestInjectPlayerSpeed(t *testing.T) {
	cmd := &resource.EventCommand{
		Code: CmdSetMoveRoute,
		Parameters: []interface{}{
			float64(-1), // charId = player
			map[string]interface{}{
				"list": []interface{}{
					map[string]interface{}{"code": float64(3), "indent": nil}, // MoveRight
					map[string]interface{}{"code": float64(0)},               // End
				},
				"repeat":    false,
				"skippable": false,
				"wait":      false,
			},
		},
	}

	result := injectPlayerSpeed(cmd, 3)

	// Original should not be modified
	origMR := cmd.Parameters[1].(map[string]interface{})
	origList := origMR["list"].([]interface{})
	assert.Equal(t, 2, len(origList), "original list should be unchanged")

	// Result should have 3 items: ChangeSpeed + MoveRight + End
	resultMR := result.Parameters[1].(map[string]interface{})
	resultList := resultMR["list"].([]interface{})
	require.Equal(t, 3, len(resultList), "injected list should have 3 items")

	// First item: ROUTE_CHANGE_SPEED(3)
	speedCmd := resultList[0].(map[string]interface{})
	assert.Equal(t, float64(29), speedCmd["code"])
	speedParams := speedCmd["parameters"].([]interface{})
	assert.Equal(t, float64(3), speedParams[0])

	// Second item: ROUTE_MOVE_RIGHT
	moveCmd := resultList[1].(map[string]interface{})
	assert.Equal(t, float64(3), moveCmd["code"])

	// Third: End
	endCmd := resultList[2].(map[string]interface{})
	assert.Equal(t, float64(0), endCmd["code"])
}

func TestInjectPlayerSpeed_NotPlayer(t *testing.T) {
	cmd := &resource.EventCommand{
		Code: CmdSetMoveRoute,
		Parameters: []interface{}{
			float64(2), // charId = event 2, NOT player
			map[string]interface{}{
				"list": []interface{}{
					map[string]interface{}{"code": float64(3)},
					map[string]interface{}{"code": float64(0)},
				},
			},
		},
	}
	// Should not inject for non-player target
	result := injectPlayerSpeed(cmd, 3)
	// injectPlayerSpeed doesn't check charId - the caller does.
	// But let's verify the function still works (it injects regardless).
	resultMR := result.Parameters[1].(map[string]interface{})
	resultList := resultMR["list"].([]interface{})
	assert.Equal(t, 3, len(resultList), "inject should still work")
}

func TestStepUntilWait_SendsBothEffectsBeforeWait(t *testing.T) {
	e := &Executor{logger: nopLogger()}
	s := testSession(1)
	s.MapID = 110

	gs := newMockGameState()
	gs.switches[330] = true // Switch 330 ON → loop continues

	opts := &ExecuteOpts{
		GameState: gs,
		MapID:     110,
	}

	// Event 3: SetMoveRoute(player, MoveRight) + Wait(1)
	ev3 := NewParallelEventState(3, makeParallelMoveCommands(-1, 330), 3)
	// Event 4: SetMoveRoute(event#2, MoveRight) + Wait(1)
	ev4 := NewParallelEventState(4, makeParallelMoveCommands(2, 330), 3)

	// Step event 3
	done3 := e.stepUntilWait(context.Background(), s, ev3, opts)
	assert.False(t, done3, "event 3 should pause at Wait, not finish")

	// Step event 4
	done4 := e.stepUntilWait(context.Background(), s, ev4, opts)
	assert.False(t, done4, "event 4 should pause at Wait, not finish")

	// Check packets: should have 2 npc_effect packets (one per event)
	pkts := drainPackets(t, s)
	require.Equal(t, 2, len(pkts), "should send exactly 2 effects (one per event)")

	// First: SetMoveRoute for player (charId=-1) with injected speed
	code1, params1, mapID1 := parseNPCEffect(t, pkts[0])
	assert.Equal(t, CmdSetMoveRoute, code1)
	assert.Equal(t, float64(-1), params1[0], "first effect targets player")
	assert.Equal(t, 110, mapID1, "effect should be tagged with map_id 110")

	// Verify speed injection
	list1 := extractMoveRouteList(t, params1)
	assert.Equal(t, 3, len(list1), "player route should have ChangeSpeed + MoveRight + End")
	assert.Equal(t, float64(29), list1[0]["code"], "first command should be ROUTE_CHANGE_SPEED")

	// Second: SetMoveRoute for NPC event#2 (charId=2) - no speed injection
	code2, params2, mapID2 := parseNPCEffect(t, pkts[1])
	assert.Equal(t, CmdSetMoveRoute, code2)
	assert.Equal(t, float64(2), params2[0], "second effect targets NPC event#2")
	assert.Equal(t, 110, mapID2, "NPC effect should be tagged with map_id 110")

	list2 := extractMoveRouteList(t, params2)
	assert.Equal(t, 2, len(list2), "NPC route should NOT have speed injection")
	assert.Equal(t, float64(3), list2[0]["code"], "NPC first command should be MoveRight")
}

func TestStepUntilWait_BreaksOnSwitchOff(t *testing.T) {
	e := &Executor{logger: nopLogger()}
	s := testSession(1)
	s.MapID = 110

	gs := newMockGameState()
	gs.switches[330] = false // Switch 330 OFF → should break

	opts := &ExecuteOpts{
		GameState: gs,
		MapID:     110,
	}

	ev := NewParallelEventState(3, makeParallelMoveCommands(-1, 330), 3)
	done := e.stepUntilWait(context.Background(), s, ev, opts)

	// Should have sent MoveRight + hit Wait + then on next tick:
	// the first step sends MoveRight, hits Wait, returns false
	assert.False(t, done, "first step should hit Wait")

	// Step again - now conditional branch checks Switch 330 OFF → Break
	done = e.stepUntilWait(context.Background(), s, ev, opts)
	// After break, it should skip past RepeatAbove and hit End(indent=0)
	assert.True(t, done, "second step should break out of loop")
}

func TestStepUntilWait_StopsOnMapIDMismatch(t *testing.T) {
	e := &Executor{logger: nopLogger()}
	s := testSession(1)
	s.MapID = 67 // Player transferred to map 67!

	gs := newMockGameState()
	gs.switches[330] = true

	opts := &ExecuteOpts{
		GameState: gs,
		MapID:     110, // Parallel events are for map 110
	}

	ev := NewParallelEventState(3, makeParallelMoveCommands(-1, 330), 3)
	done := e.stepUntilWait(context.Background(), s, ev, opts)
	assert.True(t, done, "should stop immediately on MapID mismatch")

	// No packets should be sent
	pkts := drainPackets(t, s)
	assert.Equal(t, 0, len(pkts), "no effects should be sent after map transfer")
}

func TestRunParallelEventsSynced_StopsOnMapIDChange(t *testing.T) {
	e := &Executor{logger: nopLogger()}
	s := testSession(1)
	s.MapID = 110

	gs := newMockGameState()
	gs.switches[330] = true

	opts := &ExecuteOpts{
		GameState: gs,
		MapID:     110,
	}

	events := []*ParallelEventState{
		NewParallelEventState(3, makeParallelMoveCommands(-1, 330), 3),
		NewParallelEventState(4, makeParallelMoveCommands(2, 330), 3),
	}

	ctx, cancel := context.WithCancel(context.Background())

	done := make(chan struct{})
	go func() {
		e.RunParallelEventsSynced(ctx, s, events, opts)
		close(done)
	}()

	// Let first tick execute
	time.Sleep(100 * time.Millisecond)

	// Simulate map transfer
	s.MapID = 67
	cancel()

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("RunParallelEventsSynced did not stop after MapID change")
	}

	// Should have sent exactly 2 effects (one tick's worth)
	pkts := drainPackets(t, s)
	assert.Equal(t, 2, len(pkts), "should have sent effects for exactly one tick")
}

func TestRunParallelEventsSynced_AllEventsStepInSameTick(t *testing.T) {
	e := &Executor{logger: nopLogger()}
	s := testSession(1)
	s.MapID = 110

	gs := newMockGameState()
	gs.switches[330] = true

	opts := &ExecuteOpts{
		GameState: gs,
		MapID:     110,
	}

	events := []*ParallelEventState{
		NewParallelEventState(3, makeParallelMoveCommands(-1, 330), 3),
		NewParallelEventState(4, makeParallelMoveCommands(2, 330), 3),
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go e.RunParallelEventsSynced(ctx, s, events, opts)

	// Wait for first tick + a bit extra
	time.Sleep(200 * time.Millisecond)
	cancel()
	time.Sleep(50 * time.Millisecond)

	pkts := drainPackets(t, s)
	// First tick should produce exactly 2 effects (player + NPC)
	require.True(t, len(pkts) >= 2, "should have at least 2 effects from first tick")

	// Verify both effects are SetMoveRoute with correct map_id
	code1, params1, mid1 := parseNPCEffect(t, pkts[0])
	assert.Equal(t, CmdSetMoveRoute, code1)
	assert.Equal(t, float64(-1), params1[0], "first should target player")
	assert.Equal(t, 110, mid1)

	code2, params2, mid2 := parseNPCEffect(t, pkts[1])
	assert.Equal(t, CmdSetMoveRoute, code2)
	assert.Equal(t, float64(2), params2[0], "second should target NPC")
	assert.Equal(t, 110, mid2)
}

func TestRunParallelEventsSynced_EventsRestartAfterCompletion(t *testing.T) {
	e := &Executor{logger: nopLogger()}
	s := testSession(1)
	s.MapID = 110

	gs := newMockGameState()
	gs.switches[330] = false // OFF → events will break immediately after MoveRight+Wait

	opts := &ExecuteOpts{
		GameState: gs,
		MapID:     110,
	}

	events := []*ParallelEventState{
		NewParallelEventState(3, makeParallelMoveCommands(-1, 330), 3),
		NewParallelEventState(4, makeParallelMoveCommands(2, 330), 3),
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go e.RunParallelEventsSynced(ctx, s, events, opts)

	// Wait for several ticks to verify events restart
	time.Sleep(2 * time.Second)
	cancel()
	time.Sleep(50 * time.Millisecond)

	pkts := drainPackets(t, s)
	// With switch OFF, each tick: MoveRight → Wait → (next step) check condition → break → done → restart
	// So each tick still sends 2 effects (one per event), and events restart
	// Over ~2 seconds with 533ms tick, we expect ~3-4 ticks × 2 effects = 6-8 packets
	assert.True(t, len(pkts) >= 4, "events should restart and send effects on multiple ticks, got %d", len(pkts))
}

func TestTickInterval_MatchesMoveSpeed(t *testing.T) {
	// moveSpeed 3: 256/2^3 = 32 frames, 32*1000/60 = 533ms
	// moveSpeed 4: 256/2^4 = 16 frames, 16*1000/60 = 266ms
	tests := []struct {
		speed    int
		expected int // milliseconds
	}{
		{3, 533},
		{4, 266},
		{5, 133},
		{2, 1066},
	}
	for _, tt := range tests {
		framesPerTile := 256 >> tt.speed
		tickMs := framesPerTile * 1000 / 60
		assert.Equal(t, tt.expected, tickMs, "speed %d tick", tt.speed)
	}
}

func TestStepUntilWait_SetsWaitFrames(t *testing.T) {
	e := &Executor{logger: nopLogger()}
	s := testSession(1)
	s.MapID = 110

	gs := newMockGameState()
	gs.switches[330] = true

	opts := &ExecuteOpts{GameState: gs, MapID: 110}

	// Event with Wait(60) — should set waitFrames to 60
	ev := NewParallelEventState(3, makeParallelMoveCommands(-1, 330), 3)
	// Modify the Wait command to Wait(60) instead of Wait(1)
	ev.Cmds[3] = &resource.EventCommand{Code: CmdWait, Indent: 1, Parameters: []interface{}{float64(60)}}

	done := e.stepUntilWait(context.Background(), s, ev, opts)
	assert.False(t, done, "should pause at Wait")
	assert.Equal(t, 60, ev.waitFrames, "waitFrames should be 60")
}

func TestStepUntilWait_ChangeSelfSwitch(t *testing.T) {
	e := &Executor{logger: nopLogger()}
	s := testSession(1)
	s.MapID = 110

	gs := newMockGameState()
	opts := &ExecuteOpts{GameState: gs, MapID: 110, EventID: 5}

	// Simple command list: ChangeSelfSwitch(A, ON) → End
	cmds := []*resource.EventCommand{
		{Code: CmdChangeSelfSwitch, Indent: 0, Parameters: []interface{}{"A", float64(0)}}, // 0=ON
		{Code: CmdEnd, Indent: 0},
	}
	ev := NewParallelEventState(5, cmds, 3)

	done := e.stepUntilWait(context.Background(), s, ev, opts)
	assert.True(t, done, "should finish (hit End)")

	// Self switch should be set
	assert.True(t, gs.GetSelfSwitch(110, 5, "A"), "self switch A should be ON")
}

func TestRunParallelEventsSynced_WaitFrameCountdown(t *testing.T) {
	e := &Executor{logger: nopLogger()}
	s := testSession(1)
	s.MapID = 110

	gs := newMockGameState()
	gs.switches[330] = true

	opts := &ExecuteOpts{GameState: gs, MapID: 110}

	// Event with: SetMoveRoute → Wait(60) → Loop back
	// At moveSpeed 3, tick = 32 frames. Wait(60) needs 2 ticks (32+32=64 > 60).
	cmds := makeParallelMoveCommands(-1, 330)
	cmds[3] = &resource.EventCommand{Code: CmdWait, Indent: 1, Parameters: []interface{}{float64(60)}}

	events := []*ParallelEventState{
		NewParallelEventState(3, cmds, 3),
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go e.RunParallelEventsSynced(ctx, s, events, opts)

	// Wait ~3 ticks (3 * 533ms ≈ 1.6s). With Wait(60) needing 2 ticks,
	// we should get roughly 1 MoveRight in ~3 ticks (tick1: move, tick2: wait, tick3: move).
	time.Sleep(1800 * time.Millisecond)
	cancel()
	time.Sleep(50 * time.Millisecond)

	pkts := drainPackets(t, s)
	// With Wait(1): 3 ticks → 3 effects
	// With Wait(60): tick 0: MoveRight + set waitFrames=60
	//   tick 1: waitFrames=60-32=28 (still waiting)
	//   tick 2: waitFrames=28-32=-4 → resume → conditional → loop → MoveRight + set wait=60
	//   tick 3: waiting...
	// So in ~3 ticks: ~2 MoveRight effects (not 3)
	assert.True(t, len(pkts) <= 3 && len(pkts) >= 1,
		"Wait(60) should produce fewer effects than Wait(1), got %d", len(pkts))
}
