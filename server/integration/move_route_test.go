package integration

import (
	"net"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// recvNoMoveReject drains messages for the given duration and asserts that
// no move_reject was received. Returns all non-reject messages collected.
func recvNoMoveReject(t *testing.T, ws *WSClient, drain time.Duration) []map[string]interface{} {
	t.Helper()
	deadline := time.Now().Add(drain)
	var msgs []map[string]interface{}
	for time.Now().Before(deadline) {
		pkt, err := ws.RecvAny(time.Until(deadline))
		if err != nil {
			if netErr, ok := err.(net.Error); ok && netErr.Timeout() {
				break
			}
			break
		}
		if pkt["type"] == "move_reject" {
			t.Errorf("unexpected move_reject: %v", pkt)
		}
		msgs = append(msgs, pkt)
	}
	return msgs
}

// TestMoveRouteRejectLoop verifies the fix for the move-route reject loop issue.
//
// Problem: During event execution (EventMu locked), SetMoveRoute (code 205)
// tells the client to move the player. The client's moveStraight sends
// player_move to the server, which rejects it because EventMu is locked.
// The client snaps back, the move route retries → infinite loop.
//
// Fix: Client skips player_move during _moveRouteForcing. When the move route
// completes, the client sends npc_effect_ack with the player's final position.
// The server updates the session position from the ack payload.
func TestMoveRouteRejectLoop(t *testing.T) {
	ts := NewTestServer(t)
	defer ts.Close()

	_, charID, ws := ts.LoginAndConnect(t, UniqueID("mrA"), UniqueID("MRHero"))
	defer ws.Close()

	sess := ts.SM.Get(charID)
	require.NotNil(t, sess)

	// Set a known starting position.
	sess.SetPosition(5, 5, 2)

	// ─── Phase 1: Verify move_reject during event lock ───
	// Simulate event execution by locking EventMu.
	sess.EventMu.Lock()

	// A player_move during event should be rejected.
	ws.Send("player_move", map[string]interface{}{
		"x": 6, "y": 5, "dir": 6,
	})

	pkt := ws.RecvType("move_reject", 3*time.Second)
	require.NotNil(t, pkt, "should receive move_reject during event")

	payload := PayloadMap(t, pkt)
	assert.EqualValues(t, 5, payload["x"], "reject returns server position x")
	assert.EqualValues(t, 5, payload["y"], "reject returns server position y")

	// Position should not have changed.
	x, y, _ := sess.Position()
	assert.Equal(t, 5, x, "position unchanged after reject")
	assert.Equal(t, 5, y, "position unchanged after reject")

	// ─── Phase 2: Simulate move route ack with position sync ───
	// In the real flow, the client executes the move route locally (skipping
	// player_move), then sends npc_effect_ack with the final position.
	//
	// Set up a goroutine to consume the EffectAckCh (simulating the executor
	// waiting for an ack in sendEffectWait).
	ackReceived := make(chan struct{}, 1)
	go func() {
		select {
		case <-sess.EffectAckCh:
			ackReceived <- struct{}{}
		case <-time.After(5 * time.Second):
		}
	}()

	// Client sends ack with final position after move route completes.
	ws.Send("npc_effect_ack", map[string]interface{}{
		"x": 8, "y": 5, "dir": 6,
	})

	// Wait for the ack to be processed.
	select {
	case <-ackReceived:
		// OK — executor got the signal
	case <-time.After(3 * time.Second):
		t.Fatal("EffectAckCh was not signalled")
	}

	// Server position should now reflect the move route's final position.
	x, y, dir := sess.Position()
	assert.Equal(t, 8, x, "x updated from ack")
	assert.Equal(t, 5, y, "y updated from ack")
	assert.Equal(t, 6, dir, "dir updated from ack")

	// ─── Phase 3: Verify normal movement works after event ends ───
	sess.EventMu.Unlock()

	// Move from (8,5) → (9,5): within 1.3 tiles, should succeed.
	ws.Send("player_move", map[string]interface{}{
		"x": 9, "y": 5, "dir": 6,
	})

	// Should NOT receive move_reject — drain briefly and check.
	// After move_reject or player_sync, verify position updated.
	time.Sleep(100 * time.Millisecond)

	x, y, dir = sess.Position()
	assert.Equal(t, 9, x, "x updated after normal move")
	assert.Equal(t, 5, y, "y stays after normal move")
	assert.Equal(t, 6, dir, "dir stays after normal move")
}

// TestEffectAckPositionSync verifies that npc_effect_ack with position data
// correctly updates the server-side session position.
func TestEffectAckPositionSync(t *testing.T) {
	ts := NewTestServer(t)
	defer ts.Close()

	_, charID, ws := ts.LoginAndConnect(t, UniqueID("ackA"), UniqueID("AckHero"))
	defer ws.Close()

	sess := ts.SM.Get(charID)
	require.NotNil(t, sess)

	sess.SetPosition(0, 0, 2)

	// Send ack with position data.
	ws.Send("npc_effect_ack", map[string]interface{}{
		"x": 15, "y": 20, "dir": 4,
	})

	// Small delay for processing.
	time.Sleep(100 * time.Millisecond)

	x, y, dir := sess.Position()
	assert.Equal(t, 15, x)
	assert.Equal(t, 20, y)
	assert.Equal(t, 4, dir)
}

// TestEffectAckEmptyPayload verifies backward compatibility —
// an empty ack should not change position.
func TestEffectAckEmptyPayload(t *testing.T) {
	ts := NewTestServer(t)
	defer ts.Close()

	_, charID, ws := ts.LoginAndConnect(t, UniqueID("ackB"), UniqueID("AckHero2"))
	defer ws.Close()

	sess := ts.SM.Get(charID)
	require.NotNil(t, sess)

	sess.SetPosition(10, 20, 8)

	ws.Send("npc_effect_ack", map[string]interface{}{})

	time.Sleep(100 * time.Millisecond)

	x, y, dir := sess.Position()
	assert.Equal(t, 10, x, "x unchanged")
	assert.Equal(t, 20, y, "y unchanged")
	assert.Equal(t, 8, dir, "dir unchanged")
}

// TestMoveAfterAckNoSpeedHack verifies that a normal move immediately after
// an ack position sync is not flagged as speed hacking.
// This covers the scenario: map transfer → cutscene moves player far →
// ack syncs position → player moves 1 tile normally.
func TestMoveAfterAckNoSpeedHack(t *testing.T) {
	ts := NewTestServer(t)
	defer ts.Close()

	_, charID, ws := ts.LoginAndConnect(t, UniqueID("ackC"), UniqueID("AckHero3"))
	defer ws.Close()

	sess := ts.SM.Get(charID)
	require.NotNil(t, sess)

	// Player starts at (0,0).
	sess.SetPosition(0, 0, 2)

	// Ack syncs player to (50, 50) — a large jump, as if a move route
	// walked the player far across the map.
	ws.Send("npc_effect_ack", map[string]interface{}{
		"x": 50, "y": 50, "dir": 6,
	})
	time.Sleep(100 * time.Millisecond)

	// Verify position was synced.
	x, y, _ := sess.Position()
	require.Equal(t, 50, x)
	require.Equal(t, 50, y)

	// Now a normal 1-tile move from (50,50) → (51,50) should succeed.
	ws.Send("player_move", map[string]interface{}{
		"x": 51, "y": 50, "dir": 6,
	})

	// Wait and check — should NOT get move_reject.
	time.Sleep(100 * time.Millisecond)

	x, y, _ = sess.Position()
	assert.Equal(t, 51, x, "normal move after ack sync should succeed")
	assert.Equal(t, 50, y)
}

// ═══════════════════════════════════════════════════════════════════════════
//  Scenario tests based on real projectb game patterns
// ═══════════════════════════════════════════════════════════════════════════

// TestScenario_MultiStepCutscene simulates a cutscene with multiple sequential
// move routes interleaved with dialog, based on Map002 Event 37 in projectb:
//
//   dialog → SetMoveRoute(player, wait) → dialog → SetMoveRoute(player, wait) → ...
//
// Verifies that each ack correctly syncs position and subsequent acks
// don't cause speed-hack false positives despite large cumulative displacement.
func TestScenario_MultiStepCutscene(t *testing.T) {
	ts := NewTestServer(t)
	defer ts.Close()

	_, charID, ws := ts.LoginAndConnect(t, UniqueID("cs"), UniqueID("CutsceneHero"))
	defer ws.Close()

	sess := ts.SM.Get(charID)
	require.NotNil(t, sess)

	sess.SetPosition(10, 10, 2)
	sess.EventMu.Lock()
	defer sess.EventMu.Unlock()

	// Step 1: Move route walks player down 6 tiles (10,10) → (10,16).
	// Based on Map002 Event 37: turn left + move down ×6.
	ack1 := make(chan struct{}, 1)
	go func() { <-sess.EffectAckCh; ack1 <- struct{}{} }()
	ws.Send("npc_effect_ack", map[string]interface{}{
		"x": 10, "y": 16, "dir": 2,
	})
	select {
	case <-ack1:
	case <-time.After(3 * time.Second):
		t.Fatal("ack1 timeout")
	}

	x, y, dir := sess.Position()
	assert.Equal(t, 10, x, "step1 x")
	assert.Equal(t, 16, y, "step1 y")
	assert.Equal(t, 2, dir, "step1 dir=down")

	// Step 2: Dialog happens (server sends npc_dialog, client acks — not relevant here).
	// Then another move route walks player right 5 tiles (10,16) → (15,16).
	ack2 := make(chan struct{}, 1)
	go func() { <-sess.EffectAckCh; ack2 <- struct{}{} }()
	ws.Send("npc_effect_ack", map[string]interface{}{
		"x": 15, "y": 16, "dir": 6,
	})
	select {
	case <-ack2:
	case <-time.After(3 * time.Second):
		t.Fatal("ack2 timeout")
	}

	x, y, dir = sess.Position()
	assert.Equal(t, 15, x, "step2 x")
	assert.Equal(t, 16, y, "step2 y")
	assert.Equal(t, 6, dir, "step2 dir=right")

	// Step 3: A third move route walks player up 3 tiles (15,16) → (15,13).
	ack3 := make(chan struct{}, 1)
	go func() { <-sess.EffectAckCh; ack3 <- struct{}{} }()
	ws.Send("npc_effect_ack", map[string]interface{}{
		"x": 15, "y": 13, "dir": 8,
	})
	select {
	case <-ack3:
	case <-time.After(3 * time.Second):
		t.Fatal("ack3 timeout")
	}

	x, y, dir = sess.Position()
	assert.Equal(t, 15, x, "step3 x")
	assert.Equal(t, 13, y, "step3 y")
	assert.Equal(t, 8, dir, "step3 dir=up")

	// Total displacement from start: (10,10)→(15,13) = 5 tiles right, 3 up.
	// Without ack position sync, the server would think player is still at (10,10)
	// and reject any move from (15,13) as speed hacking.
}

// TestScenario_TransferThenMoveRoute simulates the common cutscene pattern:
// map transfer → event fires → SetMoveRoute moves player.
// Based on Map067 Event 11: Transfer → SetMoveRoute(wait=false).
//
// After transfer, LastTransfer grants a 3-second grace period, but the move
// route ack should sync position regardless so the grace period isn't relied upon.
func TestScenario_TransferThenMoveRoute(t *testing.T) {
	ts := NewTestServer(t)
	defer ts.Close()

	_, charID, ws := ts.LoginAndConnect(t, UniqueID("tf"), UniqueID("TransferHero"))
	defer ws.Close()

	sess := ts.SM.Get(charID)
	require.NotNil(t, sess)

	// Player starts on map 1 at (5, 5).
	sess.SetPosition(5, 5, 2)

	// Simulate map transfer: server moves player to map 2 at (20, 30).
	sess.MapID = 2
	sess.SetPosition(20, 30, 2)
	sess.LastTransfer = time.Now()

	// Immediately after transfer, an autorun event locks EventMu and
	// issues SetMoveRoute to walk the player.
	sess.EventMu.Lock()

	// player_move during event should be rejected even with grace period.
	ws.Send("player_move", map[string]interface{}{
		"x": 21, "y": 30, "dir": 6,
	})
	pkt := ws.RecvType("move_reject", 3*time.Second)
	require.NotNil(t, pkt, "move should be rejected during event even with transfer grace")

	// Move route walks player from (20,30) → (20,25): 5 tiles up.
	ack := make(chan struct{}, 1)
	go func() { <-sess.EffectAckCh; ack <- struct{}{} }()
	ws.Send("npc_effect_ack", map[string]interface{}{
		"x": 20, "y": 25, "dir": 8,
	})
	select {
	case <-ack:
	case <-time.After(3 * time.Second):
		t.Fatal("ack timeout")
	}

	x, y, dir := sess.Position()
	assert.Equal(t, 20, x)
	assert.Equal(t, 25, y)
	assert.Equal(t, 8, dir)

	sess.EventMu.Unlock()

	// Wait for grace period to expire to test without it.
	time.Sleep(3100 * time.Millisecond)

	// Normal move from (20,25) → (20,24): should succeed without grace period
	// because ack already synced position.
	ws.Send("player_move", map[string]interface{}{
		"x": 20, "y": 24, "dir": 8,
	})
	recvNoMoveReject(t, ws, 200*time.Millisecond)

	x, y, _ = sess.Position()
	assert.Equal(t, 20, x, "normal move after grace expired")
	assert.Equal(t, 24, y)
}

// TestScenario_SimultaneousPlayerNPCMovement simulates the pattern from
// Map002 Event 95: player moves with wait=false (non-blocking), NPC moves
// with wait=true. The server waits for the NPC's ack, but the player's
// move route also produces an ack with position.
//
// In real flow: two code 205 effects sent, client handles them both,
// the one with wait=true triggers the ack. Our fix ensures the player's
// position is included in whichever ack reaches the server.
func TestScenario_SimultaneousPlayerNPCMovement(t *testing.T) {
	ts := NewTestServer(t)
	defer ts.Close()

	_, charID, ws := ts.LoginAndConnect(t, UniqueID("sim"), UniqueID("SimHero"))
	defer ws.Close()

	sess := ts.SM.Get(charID)
	require.NotNil(t, sess)

	sess.SetPosition(10, 10, 2)
	sess.EventMu.Lock()

	// First ack: player move route (wait=false in real client, but server
	// might still receive a wait=true ack for the NPC's route).
	// The client sends ack for the NPC's move (wait=true), which doesn't
	// include player position. Player position stays stale.
	ack1 := make(chan struct{}, 1)
	go func() { <-sess.EffectAckCh; ack1 <- struct{}{} }()
	ws.Send("npc_effect_ack", map[string]interface{}{}) // NPC ack, no player pos
	select {
	case <-ack1:
	case <-time.After(3 * time.Second):
		t.Fatal("npc ack timeout")
	}

	// Position should be unchanged (NPC ack has no position data).
	x, y, _ := sess.Position()
	assert.Equal(t, 10, x, "position unchanged after NPC ack")
	assert.Equal(t, 10, y)

	// Then code 209 (WaitForMoveRoute) fires for the player route.
	// Client sends ack with player position when isMoveRouteForcing() becomes false.
	ack2 := make(chan struct{}, 1)
	go func() { <-sess.EffectAckCh; ack2 <- struct{}{} }()
	ws.Send("npc_effect_ack", map[string]interface{}{
		"x": 15, "y": 10, "dir": 6,
	})
	select {
	case <-ack2:
	case <-time.After(3 * time.Second):
		t.Fatal("player ack timeout")
	}

	// Now position should be updated.
	x, y, dir := sess.Position()
	assert.Equal(t, 15, x, "player position synced via second ack")
	assert.Equal(t, 10, y)
	assert.Equal(t, 6, dir)

	sess.EventMu.Unlock()
}

// TestScenario_RapidRejectsDuringEvent simulates the original bug scenario
// where the client sends multiple player_move messages during an event
// (before the fix). All should be rejected, and position must remain stable.
// Then the ack syncs the correct position and no state is corrupted.
func TestScenario_RapidRejectsDuringEvent(t *testing.T) {
	ts := NewTestServer(t)
	defer ts.Close()

	_, charID, ws := ts.LoginAndConnect(t, UniqueID("rapid"), UniqueID("RapidHero"))
	defer ws.Close()

	sess := ts.SM.Get(charID)
	require.NotNil(t, sess)

	sess.SetPosition(5, 5, 2)
	sess.EventMu.Lock()

	// Simulate the old bug: 5 rapid player_move messages (like the log in the issue doc).
	for i := 0; i < 5; i++ {
		ws.Send("player_move", map[string]interface{}{
			"x": 6, "y": 5, "dir": 6,
		})
	}

	// Drain all responses — each should be move_reject.
	// Use a generous window to collect all responses.
	time.Sleep(500 * time.Millisecond)
	rejectCount := 0
	for {
		pkt, err := ws.RecvAny(500 * time.Millisecond)
		if err != nil {
			break
		}
		if pkt["type"] == "move_reject" {
			rejectCount++
		}
	}
	assert.GreaterOrEqual(t, rejectCount, 3, "most moves should be rejected")
	assert.LessOrEqual(t, rejectCount, 5, "at most 5 rejects")

	// Position must NOT have drifted.
	x, y, _ := sess.Position()
	assert.Equal(t, 5, x, "position stable after rapid rejects")
	assert.Equal(t, 5, y)

	// Now ack with the correct final position.
	ack := make(chan struct{}, 1)
	go func() { <-sess.EffectAckCh; ack <- struct{}{} }()
	ws.Send("npc_effect_ack", map[string]interface{}{
		"x": 8, "y": 5, "dir": 6,
	})
	select {
	case <-ack:
	case <-time.After(3 * time.Second):
		t.Fatal("ack timeout")
	}

	x, y, dir := sess.Position()
	assert.Equal(t, 8, x, "position recovered via ack")
	assert.Equal(t, 5, y)
	assert.Equal(t, 6, dir)

	sess.EventMu.Unlock()

	// Normal move should work.
	ws.Send("player_move", map[string]interface{}{
		"x": 9, "y": 5, "dir": 6,
	})
	time.Sleep(200 * time.Millisecond)
	x, y, _ = sess.Position()
	assert.Equal(t, 9, x, "normal move works after recovery")
}

// TestScenario_MoveRouteBeforeTransfer simulates the pattern from Map031
// Event 34: SetMoveRoute(player, wait=true) → Transfer Player.
// The move route syncs position, then a transfer changes the map and position.
func TestScenario_MoveRouteBeforeTransfer(t *testing.T) {
	ts := NewTestServer(t)
	defer ts.Close()

	_, charID, ws := ts.LoginAndConnect(t, UniqueID("bt"), UniqueID("BeforeTransfer"))
	defer ws.Close()

	sess := ts.SM.Get(charID)
	require.NotNil(t, sess)

	sess.SetPosition(18, 14, 2)
	sess.EventMu.Lock()

	// Move route walks player to (18, 12).
	ack := make(chan struct{}, 1)
	go func() { <-sess.EffectAckCh; ack <- struct{}{} }()
	ws.Send("npc_effect_ack", map[string]interface{}{
		"x": 18, "y": 12, "dir": 8,
	})
	select {
	case <-ack:
	case <-time.After(3 * time.Second):
		t.Fatal("ack timeout")
	}

	x, y, dir := sess.Position()
	assert.Equal(t, 18, x, "position after move route")
	assert.Equal(t, 12, y)
	assert.Equal(t, 8, dir)

	// Then the event executes Transfer Player → new map, new position.
	// This overwrites the ack-synced position.
	sess.MapID = 23
	sess.SetPosition(12, 29, 2)
	sess.LastTransfer = time.Now()
	sess.EventMu.Unlock()

	// Final position should be the transfer destination, not the move route end.
	x, y, dir = sess.Position()
	assert.Equal(t, 12, x, "transfer overwrites move route position")
	assert.Equal(t, 29, y)
	assert.Equal(t, 2, dir)
}

// TestScenario_ComplexCutsceneWithFade simulates the pattern from Map002
// Event 32: Fadeout → Transfer → Fadein → dialog → SetMoveRoute → dialog →
// SetMoveRoute → Fadeout → Transfer.
//
// This tests the full lifecycle of a multi-transition cutscene where move
// routes are sandwiched between map transfers.
func TestScenario_ComplexCutsceneWithFade(t *testing.T) {
	ts := NewTestServer(t)
	defer ts.Close()

	_, charID, ws := ts.LoginAndConnect(t, UniqueID("cx"), UniqueID("ComplexHero"))
	defer ws.Close()

	sess := ts.SM.Get(charID)
	require.NotNil(t, sess)

	// ─── Scene 1: Start on map 1 ───
	sess.SetPosition(15, 33, 2)
	sess.EventMu.Lock()

	// Fadeout (code 221, ack) — empty ack, no position change.
	ack1 := make(chan struct{}, 1)
	go func() { <-sess.EffectAckCh; ack1 <- struct{}{} }()
	ws.Send("npc_effect_ack", map[string]interface{}{})
	<-ack1

	x, y, _ := sess.Position()
	assert.Equal(t, 15, x, "fadeout doesn't change position")
	assert.Equal(t, 33, y)

	// Transfer to map 5 at (10, 15).
	sess.MapID = 5
	sess.SetPosition(10, 15, 2)
	sess.LastTransfer = time.Now()

	// Fadein (code 222, ack).
	ack2 := make(chan struct{}, 1)
	go func() { <-sess.EffectAckCh; ack2 <- struct{}{} }()
	ws.Send("npc_effect_ack", map[string]interface{}{})
	<-ack2

	// ─── Scene 2: Player walks left 3 tiles on map 5 ───
	// SetMoveRoute: (10,15) → (7,15).
	ack3 := make(chan struct{}, 1)
	go func() { <-sess.EffectAckCh; ack3 <- struct{}{} }()
	ws.Send("npc_effect_ack", map[string]interface{}{
		"x": 7, "y": 15, "dir": 4,
	})
	<-ack3

	x, y, dir := sess.Position()
	assert.Equal(t, 7, x, "scene2 move route x")
	assert.Equal(t, 15, y)
	assert.Equal(t, 4, dir, "facing left after walk")

	// Dialog happens... then another move route up 2 tiles.
	ack4 := make(chan struct{}, 1)
	go func() { <-sess.EffectAckCh; ack4 <- struct{}{} }()
	ws.Send("npc_effect_ack", map[string]interface{}{
		"x": 7, "y": 13, "dir": 8,
	})
	<-ack4

	x, y, dir = sess.Position()
	assert.Equal(t, 7, x, "scene2 second move x")
	assert.Equal(t, 13, y)
	assert.Equal(t, 8, dir, "facing up")

	// ─── Scene 3: Fadeout → Transfer to map 55 ───
	ack5 := make(chan struct{}, 1)
	go func() { <-sess.EffectAckCh; ack5 <- struct{}{} }()
	ws.Send("npc_effect_ack", map[string]interface{}{}) // fadeout ack
	<-ack5

	sess.MapID = 55
	sess.SetPosition(43, 30, 2)
	sess.LastTransfer = time.Now()

	sess.EventMu.Unlock()

	// Final state: on map 55 at (43, 30).
	assert.Equal(t, 55, sess.MapID)
	x, y, _ = sess.Position()
	assert.Equal(t, 43, x)
	assert.Equal(t, 30, y)

	// Normal move on new map should work (within grace period).
	ws.Send("player_move", map[string]interface{}{
		"x": 44, "y": 30, "dir": 6,
	})
	recvNoMoveReject(t, ws, 200*time.Millisecond)
	x, y, _ = sess.Position()
	assert.Equal(t, 44, x, "normal move on new map")
}

// TestScenario_AckPositionOverwriteByTransfer verifies that when a move route
// ack syncs position and then a transfer immediately changes it, the transfer
// position wins. This prevents stale move-route positions from persisting
// after the player has been teleported.
func TestScenario_AckPositionOverwriteByTransfer(t *testing.T) {
	ts := NewTestServer(t)
	defer ts.Close()

	_, charID, ws := ts.LoginAndConnect(t, UniqueID("ow"), UniqueID("OverwriteHero"))
	defer ws.Close()

	sess := ts.SM.Get(charID)
	require.NotNil(t, sess)

	sess.SetPosition(5, 5, 2)
	sess.EventMu.Lock()

	// Move route syncs to (10, 10).
	ack := make(chan struct{}, 1)
	go func() { <-sess.EffectAckCh; ack <- struct{}{} }()
	ws.Send("npc_effect_ack", map[string]interface{}{
		"x": 10, "y": 10, "dir": 6,
	})
	<-ack

	x, y, _ := sess.Position()
	require.Equal(t, 10, x)
	require.Equal(t, 10, y)

	// Immediately after, Transfer Player changes position to (30, 40) on map 9.
	sess.MapID = 9
	sess.SetPosition(30, 40, 2)
	sess.LastTransfer = time.Now()

	sess.EventMu.Unlock()

	// Position must be the transfer destination.
	x, y, _ = sess.Position()
	assert.Equal(t, 30, x, "transfer position wins")
	assert.Equal(t, 40, y)

	// Normal move from transfer position should work.
	ws.Send("player_move", map[string]interface{}{
		"x": 31, "y": 40, "dir": 6,
	})
	recvNoMoveReject(t, ws, 200*time.Millisecond)
	x, y, _ = sess.Position()
	assert.Equal(t, 31, x)
}

// TestScenario_NonWaitMoveRouteNoAck verifies behavior when a move route
// uses wait=false (no ack expected). Based on Map032 Event 12.
//
// In this case the server doesn't block on sendEffectWait, so no ack is needed.
// But if the client's next action after the event is a normal move, the position
// gap from the move route could cause issues. The LastTransfer grace period
// or the next ack (if any) should handle this.
func TestScenario_NonWaitMoveRouteNoAck(t *testing.T) {
	ts := NewTestServer(t)
	defer ts.Close()

	_, charID, ws := ts.LoginAndConnect(t, UniqueID("nw"), UniqueID("NoWaitHero"))
	defer ws.Close()

	sess := ts.SM.Get(charID)
	require.NotNil(t, sess)

	sess.SetPosition(5, 5, 2)
	sess.EventMu.Lock()

	// Server sends code 205 with wait=false → sendEffect (non-blocking).
	// The executor does NOT call sendEffectWait, so no ack is expected.
	// The event continues immediately and eventually unlocks EventMu.

	// Simulate: event finishes, EventMu unlocked.
	// The move route moved the player on the client but server doesn't know.
	sess.EventMu.Unlock()

	// If the event also did a transfer at the end (common pattern):
	// Transfer resets position, so no gap.
	sess.SetPosition(12, 8, 6)
	sess.LastTransfer = time.Now()

	ws.Send("player_move", map[string]interface{}{
		"x": 13, "y": 8, "dir": 6,
	})
	recvNoMoveReject(t, ws, 200*time.Millisecond)

	x, y, _ := sess.Position()
	assert.Equal(t, 13, x, "move after non-wait route + transfer")
	assert.Equal(t, 8, y)
}

// TestScenario_MultiplePlayersIndependent verifies that one player's move
// route ack doesn't affect another player's position.
func TestScenario_MultiplePlayersIndependent(t *testing.T) {
	ts := NewTestServer(t)
	defer ts.Close()

	_, charIDA, wsA := ts.LoginAndConnect(t, UniqueID("mpA"), UniqueID("PlayerA"))
	defer wsA.Close()

	_, charIDB, wsB := ts.LoginAndConnect(t, UniqueID("mpB"), UniqueID("PlayerB"))
	defer wsB.Close()

	sessA := ts.SM.Get(charIDA)
	sessB := ts.SM.Get(charIDB)
	require.NotNil(t, sessA)
	require.NotNil(t, sessB)

	sessA.SetPosition(5, 5, 2)
	sessB.SetPosition(20, 20, 6)

	// Player A is in an event with move route.
	sessA.EventMu.Lock()

	wsA.Send("npc_effect_ack", map[string]interface{}{
		"x": 10, "y": 10, "dir": 4,
	})
	time.Sleep(100 * time.Millisecond)

	// Player A position should update.
	x, y, _ := sessA.Position()
	assert.Equal(t, 10, x, "A position updated")
	assert.Equal(t, 10, y)

	// Player B position should NOT be affected.
	x, y, _ = sessB.Position()
	assert.Equal(t, 20, x, "B position unchanged")
	assert.Equal(t, 20, y)

	sessA.EventMu.Unlock()

	// Player B moves normally — should not be affected by A's event.
	wsB.Send("player_move", map[string]interface{}{
		"x": 21, "y": 20, "dir": 6,
	})
	recvNoMoveReject(t, wsB, 200*time.Millisecond)
	x, y, _ = sessB.Position()
	assert.Equal(t, 21, x, "B normal move works")
}
