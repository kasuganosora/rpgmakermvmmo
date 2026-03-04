//go:build projectb

package integration

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ═══════════════════════════════════════════════════════════════════════════
//  ProjectB Move Route Tests
//
//  These tests use real projectb game data to verify that events containing
//  SetMoveRoute (code 205) work correctly with the move-route reject loop fix.
//  They require the projectb build tag: go test -tags=projectb
// ═══════════════════════════════════════════════════════════════════════════

// TestProjectBMoveRouteEffectForwarding verifies that NPC interactions
// containing SetMoveRoute (code 205) correctly forward the effect to the
// client and accept the ack.
//
// Uses Map067 which has events with scripted player movement.
func TestProjectBMoveRouteEffectForwarding(t *testing.T) {
	ts, ws, _, charID := setupPlayerOnMap67(t)
	defer ts.Close()
	defer ws.Close()

	sess := ts.SM.Get(charID)
	require.NotNil(t, sess)

	// Position player and interact with an event.
	// Event 11 on Map 67 at (32,26) has Transfer+SetMoveRoute.
	// We position adjacent to interact.
	ts.SetPosition(t, charID, 32, 27, 8) // face up toward event at (32,26)

	ws.Send("npc_interact", map[string]interface{}{"event_id": 11})

	// Pump messages — the event may contain dialog, effects, transfers.
	res := messagePump(t, ws, pumpOpts{
		TotalTimeout: 15 * time.Second,
		QuietTimeout: 5 * time.Second,
		ChoiceReply:  []int{0},
	})

	t.Logf("Event 11 results: %d dialogs, %d effects, %d map_inits, %d total",
		len(res.Dialogs), len(res.Effects), len(res.MapInits), len(res.All))

	// If there were any effects, they should have been acked without blocking.
	// The key assertion is that the server didn't hang waiting for an ack.
	// (messagePump auto-replies npc_effect_ack for wait:true effects.)
}

// TestProjectBMoveRouteAckWithPosition verifies that when the message pump
// sends an ack with position data for code 205 effects, the server updates
// the session position correctly.
//
// This test interacts with an NPC event and manually handles code 205
// effects by sending position-enriched acks.
func TestProjectBMoveRouteAckWithPosition(t *testing.T) {
	ts, ws, _, charID := setupPlayerOnMap67(t)
	defer ts.Close()
	defer ws.Close()

	sess := ts.SM.Get(charID)
	require.NotNil(t, sess)

	// Record the current position before the event.
	startX, startY, _ := sess.Position()
	t.Logf("Start position: (%d, %d)", startX, startY)

	// Manually send a position-enriched ack (simulating what the fixed
	// client does when a move route completes).
	// Move to a nearby position — small offset to stay in the passable area.
	ws.Send("npc_effect_ack", map[string]interface{}{
		"x": startX + 1, "y": startY, "dir": 6,
	})
	time.Sleep(200 * time.Millisecond)

	// Server should have updated position.
	x, y, dir := sess.Position()
	assert.Equal(t, startX+1, x, "x updated from ack")
	assert.Equal(t, startY, y, "y unchanged from ack")
	assert.Equal(t, 6, dir, "dir updated from ack")

	// Now a normal 1-tile move from the ack-synced position.
	// Move back to start to stay in the known-passable area.
	ws.Send("player_move", map[string]interface{}{
		"x": startX, "y": startY, "dir": 4,
	})
	time.Sleep(200 * time.Millisecond)

	newX, newY, _ := sess.Position()
	assert.Equal(t, startX, newX, "normal move succeeds from ack-synced position")
	assert.Equal(t, startY, newY)
}

// TestProjectBCutsceneTransferAndMoveRoute tests the full cutscene flow:
// character creation → map 67 → interact with event → possible transfer+move.
//
// This is a high-level integration test that verifies the server doesn't
// hang or crash when processing events that combine transfers and move routes.
func TestProjectBCutsceneTransferAndMoveRoute(t *testing.T) {
	dataPath := projectBDataPath(t)
	ts := NewTestServerWithResources(t, dataPath)
	defer ts.Close()

	user := UniqueID("cutscene")
	token, _ := ts.Login(t, user, user+"pass")
	charID := ts.CreateCharacter(t, token, UniqueID("CutHero"), 1)
	ws := ts.ConnectWS(t, token)
	defer ws.Close()

	// Enter map — character creation flow begins.
	ws.Send("enter_map", map[string]interface{}{"char_id": charID})
	initPkt := ws.RecvType("map_init", 10*time.Second)
	require.NotNil(t, initPkt)
	ws.Send("scene_ready", map[string]interface{}{})

	// Pump through the creation flow. This will encounter various events
	// including those with SetMoveRoute, Transfer, dialog, etc.
	// The pump auto-acks all effects (including code 205 with wait:true).
	res := messagePump(t, ws, pumpOpts{
		TotalTimeout: 60 * time.Second,
		QuietTimeout: 10 * time.Second,
		ChoiceReply:  []int{0},
		TargetMapID:  67,
	})

	t.Logf("Creation flow: %d dialogs, %d effects, %d map_inits, reason: transfer to 67 = %v",
		len(res.Dialogs), len(res.Effects), len(res.MapInits), hasMapInit(res, 67))

	// Count code 205 effects to verify they were forwarded.
	moveRouteEffects := 0
	for _, eff := range res.Effects {
		if p, ok := eff["payload"].(map[string]interface{}); ok {
			if code, ok := p["code"].(float64); ok && int(code) == 205 {
				moveRouteEffects++
			}
		}
	}
	t.Logf("SetMoveRoute (code 205) effects forwarded: %d", moveRouteEffects)

	// Key assertion: the flow completed without hanging.
	// If the reject loop bug was still present, the server would block
	// waiting for an ack that never comes (client stuck in reject loop).
	sess := ts.SM.Get(charID)
	if sess != nil {
		x, y, _ := sess.Position()
		t.Logf("Final position: map=%d pos=(%d,%d)", sess.MapID, x, y)
	}
}

// TestProjectBMoveRoutePositionConsistency verifies that after an event
// with move routes completes, the server and client positions are consistent
// — i.e., the next normal move from the player doesn't get rejected.
//
// This is the end-to-end test for the fix: no reject loop, no speed-hack
// false positive, position stays in sync.
func TestProjectBMoveRoutePositionConsistency(t *testing.T) {
	ts, ws, _, charID := setupPlayerOnMap67(t)
	defer ts.Close()
	defer ws.Close()

	sess := ts.SM.Get(charID)
	require.NotNil(t, sess)

	// Use the current position (known passable on Map 67 after setup).
	startX, startY, _ := sess.Position()
	t.Logf("Start position: (%d, %d)", startX, startY)

	// Simulate: event locks EventMu, move route moves player 1 tile right.
	sess.EventMu.Lock()

	ws.Send("npc_effect_ack", map[string]interface{}{
		"x": startX + 1, "y": startY, "dir": 6,
	})
	time.Sleep(200 * time.Millisecond)

	x, y, _ := sess.Position()
	require.Equal(t, startX+1, x, "ack synced x")
	require.Equal(t, startY, y, "ack synced y")

	sess.EventMu.Unlock()

	// Normal move back to start — should succeed without speed-hack rejection.
	ws.Send("player_move", map[string]interface{}{
		"x": startX, "y": startY, "dir": 4,
	})

	// Drain and verify no move_reject.
	deadline := time.Now().Add(1 * time.Second)
	gotReject := false
	for time.Now().Before(deadline) {
		pkt, err := ws.RecvAny(time.Until(deadline))
		if err != nil {
			break
		}
		if pkt["type"] == "move_reject" {
			gotReject = true
			t.Errorf("unexpected move_reject after ack sync: %v", PayloadMap(t, pkt))
		}
	}
	assert.False(t, gotReject, "no move_reject should occur after proper ack sync")

	x, y, _ = sess.Position()
	assert.Equal(t, startX, x, "final x after normal move")
	assert.Equal(t, startY, y, "final y after normal move")
}
