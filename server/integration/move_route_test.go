package integration

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

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
