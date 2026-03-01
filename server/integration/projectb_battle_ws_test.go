//go:build projectb

package integration

import (
	"context"
	"net"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ---------------------------------------------------------------------------
// battlePump — reads WS messages and responds to battle protocol
// ---------------------------------------------------------------------------

type battlePumpResult struct {
	BattleStart   map[string]interface{}   // battle_battle_start payload
	InputRequests []map[string]interface{} // battle_input_request payloads
	ActionResults []map[string]interface{} // battle_action_result payloads
	TurnStarts    []map[string]interface{} // battle_turn_start payloads
	TurnEnds      []map[string]interface{} // battle_turn_end payloads
	BattleEnd     map[string]interface{}   // battle_battle_end payload
	All           []map[string]interface{} // all received packets
	BattleResult  int                      // 0=win, 1=escape, 2=lose, -1=not ended
}

type battlePumpOpts struct {
	Timeout    time.Duration
	ActionType int // 0=attack(default), 3=guard, 4=escape
}

// battlePump reads WS messages in a loop, automatically responding to
// battle_input_request with the configured action. Stops when
// battle_battle_end is received or timeout expires.
func battlePump(t *testing.T, ws *WSClient, opts battlePumpOpts) *battlePumpResult {
	t.Helper()
	if opts.Timeout == 0 {
		opts.Timeout = 30 * time.Second
	}
	res := &battlePumpResult{BattleResult: -1}
	deadline := time.Now().Add(opts.Timeout)

	for time.Now().Before(deadline) && res.BattleResult < 0 {
		remaining := time.Until(deadline)
		if remaining <= 0 {
			break
		}
		wait := 2 * time.Second
		if wait > remaining {
			wait = remaining
		}

		pkt, err := ws.RecvAny(wait)
		if err != nil {
			if netErr, ok := err.(net.Error); ok && netErr.Timeout() {
				continue
			}
			t.Logf("[battlePump] WS error: %v", err)
			break
		}

		res.All = append(res.All, pkt)
		msgType, _ := pkt["type"].(string)

		switch msgType {
		case "battle_battle_start":
			payload := PayloadMap(t, pkt)
			res.BattleStart = payload
			actors, _ := payload["actors"].([]interface{})
			enemies, _ := payload["enemies"].([]interface{})
			t.Logf("[battle] START: %d actors, %d enemies", len(actors), len(enemies))

		case "battle_input_request":
			payload := PayloadMap(t, pkt)
			res.InputRequests = append(res.InputRequests, payload)
			actorIdx := int(payload["actor_index"].(float64))
			t.Logf("[battle] INPUT_REQUEST: actor_index=%d → sending action_type=%d", actorIdx, opts.ActionType)
			ws.Send("battle_input", map[string]interface{}{
				"actor_index":    actorIdx,
				"action_type":    opts.ActionType,
				"target_indices": []int{0},
				"target_is_actor": false,
			})

		case "battle_turn_start":
			payload := PayloadMap(t, pkt)
			res.TurnStarts = append(res.TurnStarts, payload)
			t.Logf("[battle] TURN_START: turn=%v, order_count=%d",
				payload["turn_count"], len(payloadSlice(payload, "order")))

		case "battle_action_result":
			payload := PayloadMap(t, pkt)
			res.ActionResults = append(res.ActionResults, payload)
			subject, _ := payload["subject"].(map[string]interface{})
			targets := payloadSlice(payload, "targets")
			subName := ""
			if subject != nil {
				subName, _ = subject["name"].(string)
			}
			for _, tgt := range targets {
				tgtMap, _ := tgt.(map[string]interface{})
				if tgtMap == nil {
					continue
				}
				tgtRef, _ := tgtMap["target"].(map[string]interface{})
				tgtName := ""
				if tgtRef != nil {
					tgtName, _ = tgtRef["name"].(string)
				}
				t.Logf("[battle] ACTION: %s → %s (damage=%v, hp_after=%v, missed=%v)",
					subName, tgtName, tgtMap["damage"], tgtMap["hp_after"], tgtMap["missed"])
			}

		case "battle_turn_end":
			payload := PayloadMap(t, pkt)
			res.TurnEnds = append(res.TurnEnds, payload)
			t.Logf("[battle] TURN_END")

		case "battle_battle_end":
			payload := PayloadMap(t, pkt)
			res.BattleEnd = payload
			res.BattleResult = int(payload["result"].(float64))
			t.Logf("[battle] END: result=%d, exp=%v, gold=%v",
				res.BattleResult, payload["exp"], payload["gold"])

		default:
			// Non-battle messages (autoruns, map updates, etc.) — skip
			t.Logf("[battlePump] skipping: %s", msgType)
		}
	}
	return res
}

func payloadSlice(m map[string]interface{}, key string) []interface{} {
	v, _ := m[key].([]interface{})
	return v
}

// ---------------------------------------------------------------------------
// Tests
// ---------------------------------------------------------------------------

// TestProjectBBattle_WSFullFlow tests the complete server-authoritative battle
// protocol through WebSocket: login → enter_map → trigger battle → exchange
// battle messages → verify result.
func TestProjectBBattle_WSFullFlow(t *testing.T) {
	dataPath := projectBDataPath(t)
	ts := NewTestServerWithResources(t, dataPath)
	defer ts.Close()
	require.NotNil(t, ts.BattleMgr, "BattleSessionManager should be wired")

	// Login and connect.
	user := UniqueID("btlws")
	token, _ := ts.Login(t, user, user+"pass")
	charID := ts.CreateCharacter(t, token, UniqueID("warrior"), 1)
	ws := ts.ConnectWS(t, token)
	defer ws.Close()

	// Enter map.
	ws.Send("enter_map", map[string]interface{}{"char_id": charID})
	initPkt := ws.RecvType("map_init", 10*time.Second)
	require.NotNil(t, initPkt)
	ws.Send("scene_ready", map[string]interface{}{})
	time.Sleep(2 * time.Second)

	// Boost player stats so they can win.
	ws.DebugSetStats(t, map[string]interface{}{
		"level":  10,
		"hp":     500,
		"max_hp": 500,
		"mp":     50,
		"max_mp": 50,
	})

	// Get session.
	sess := ts.SM.Get(charID)
	require.NotNil(t, sess, "player session should exist")

	// Start battle via BattleSessionManager in background.
	troopID := 4
	require.True(t, troopID < len(ts.Res.Troops) && ts.Res.Troops[troopID] != nil,
		"troop %d should exist", troopID)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	resultCh := make(chan int, 1)
	go func() {
		result := ts.BattleMgr.RunBattle(ctx, sess, troopID, true, true)
		resultCh <- result
	}()

	// Run battle pump: auto-attack, collect all messages.
	res := battlePump(t, ws, battlePumpOpts{Timeout: 30 * time.Second, ActionType: 0})

	// --- Verify full protocol was exercised ---

	// battle_battle_start
	require.NotNil(t, res.BattleStart, "should receive battle_battle_start")
	actors := payloadSlice(res.BattleStart, "actors")
	enemies := payloadSlice(res.BattleStart, "enemies")
	assert.True(t, len(actors) > 0, "battle should have actors")
	assert.True(t, len(enemies) > 0, "battle should have enemies")

	// Verify actor snapshot.
	actor0, _ := actors[0].(map[string]interface{})
	require.NotNil(t, actor0)
	assert.Equal(t, true, actor0["is_actor"])
	assert.True(t, actor0["hp"].(float64) > 0, "actor should have HP")
	assert.True(t, actor0["max_hp"].(float64) > 0, "actor should have MaxHP")
	t.Logf("Actor: name=%v, hp=%v/%v, class_id=%v, level=%v",
		actor0["name"], actor0["hp"], actor0["max_hp"], actor0["class_id"], actor0["level"])

	// Verify enemy snapshot.
	enemy0, _ := enemies[0].(map[string]interface{})
	require.NotNil(t, enemy0)
	assert.Equal(t, false, enemy0["is_actor"])
	assert.True(t, enemy0["hp"].(float64) > 0, "enemy should have HP")
	assert.True(t, enemy0["enemy_id"].(float64) > 0, "enemy should have enemy_id")
	t.Logf("Enemy: name=%v, hp=%v/%v, enemy_id=%v",
		enemy0["name"], enemy0["hp"], enemy0["max_hp"], enemy0["enemy_id"])

	// battle_input_request — should have received at least one.
	assert.True(t, len(res.InputRequests) > 0, "should receive at least 1 input_request")

	// battle_turn_start — should have at least 1 turn.
	assert.True(t, len(res.TurnStarts) > 0, "should have at least 1 turn_start")

	// battle_action_result — should have at least 1 action outcome.
	assert.True(t, len(res.ActionResults) > 0, "should have at least 1 action_result")

	// Verify action_result structure.
	ar0 := res.ActionResults[0]
	subject, _ := ar0["subject"].(map[string]interface{})
	require.NotNil(t, subject, "action_result should have subject")
	assert.NotEmpty(t, subject["name"], "subject should have name")
	targets := payloadSlice(ar0, "targets")
	assert.True(t, len(targets) > 0, "action_result should have targets")

	tgt0, _ := targets[0].(map[string]interface{})
	require.NotNil(t, tgt0)
	_, hasDamage := tgt0["damage"]
	assert.True(t, hasDamage, "target should have damage field")
	_, hasHPAfter := tgt0["hp_after"]
	assert.True(t, hasHPAfter, "target should have hp_after field")

	// battle_turn_end — at least 1.
	assert.True(t, len(res.TurnEnds) > 0, "should have at least 1 turn_end")

	// battle_battle_end — the final result.
	require.NotNil(t, res.BattleEnd, "should receive battle_battle_end")
	assert.Equal(t, 0, res.BattleResult, "should win the battle (result=0)")
	assert.True(t, res.BattleEnd["exp"].(float64) >= 0, "should have exp reward")
	assert.True(t, res.BattleEnd["gold"].(float64) >= 0, "should have gold reward")

	// Wait for RunBattle to finish.
	select {
	case result := <-resultCh:
		assert.Equal(t, 0, result, "RunBattle should return win")
	case <-time.After(5 * time.Second):
		t.Fatal("RunBattle did not return in time")
	}

	t.Logf("=== Battle Summary ===")
	t.Logf("  Turns: %d", len(res.TurnStarts))
	t.Logf("  Input Requests: %d", len(res.InputRequests))
	t.Logf("  Action Results: %d", len(res.ActionResults))
	t.Logf("  Total Messages: %d", len(res.All))
}

// TestProjectBBattle_WSEscape tests escape via the WS battle protocol.
func TestProjectBBattle_WSEscape(t *testing.T) {
	dataPath := projectBDataPath(t)
	ts := NewTestServerWithResources(t, dataPath)
	defer ts.Close()
	require.NotNil(t, ts.BattleMgr)

	user := UniqueID("btlesc")
	token, _ := ts.Login(t, user, user+"pass")
	charID := ts.CreateCharacter(t, token, UniqueID("runner"), 1)
	ws := ts.ConnectWS(t, token)
	defer ws.Close()

	ws.Send("enter_map", map[string]interface{}{"char_id": charID})
	initPkt := ws.RecvType("map_init", 10*time.Second)
	require.NotNil(t, initPkt)
	ws.Send("scene_ready", map[string]interface{}{})
	time.Sleep(2 * time.Second)

	ws.DebugSetStats(t, map[string]interface{}{
		"level":  10,
		"hp":     500,
		"max_hp": 500,
	})

	sess := ts.SM.Get(charID)
	require.NotNil(t, sess)

	troopID := 4
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	resultCh := make(chan int, 1)
	go func() {
		result := ts.BattleMgr.RunBattle(ctx, sess, troopID, true, true)
		resultCh <- result
	}()

	// Auto-escape on every input request.
	res := battlePump(t, ws, battlePumpOpts{Timeout: 30 * time.Second, ActionType: 4})

	require.NotNil(t, res.BattleStart, "should receive battle_battle_start")
	assert.True(t, len(res.InputRequests) > 0, "should receive input_request(s)")

	// Result should be escape (1). Escape probability increases per attempt.
	require.NotNil(t, res.BattleEnd, "should receive battle_battle_end")
	assert.Equal(t, 1, res.BattleResult, "should escape (result=1)")

	select {
	case result := <-resultCh:
		assert.Equal(t, 1, result, "RunBattle should return escape")
	case <-time.After(5 * time.Second):
		t.Fatal("RunBattle did not return in time")
	}
}

// TestProjectBBattle_WSProtocolOrder verifies that battle events arrive in the
// correct protocol order: battle_start → (input_request → turn_start →
// action_result → turn_end)* → battle_end.
func TestProjectBBattle_WSProtocolOrder(t *testing.T) {
	dataPath := projectBDataPath(t)
	ts := NewTestServerWithResources(t, dataPath)
	defer ts.Close()
	require.NotNil(t, ts.BattleMgr)

	user := UniqueID("btlord")
	token, _ := ts.Login(t, user, user+"pass")
	charID := ts.CreateCharacter(t, token, UniqueID("knight"), 1)
	ws := ts.ConnectWS(t, token)
	defer ws.Close()

	ws.Send("enter_map", map[string]interface{}{"char_id": charID})
	initPkt := ws.RecvType("map_init", 10*time.Second)
	require.NotNil(t, initPkt)
	ws.Send("scene_ready", map[string]interface{}{})
	time.Sleep(2 * time.Second)

	// Use high level to win quickly (1-2 turns).
	ws.DebugSetStats(t, map[string]interface{}{
		"level":  30,
		"hp":     2000,
		"max_hp": 2000,
		"mp":     200,
		"max_mp": 200,
	})

	sess := ts.SM.Get(charID)
	require.NotNil(t, sess)

	troopID := 4
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	resultCh := make(chan int, 1)
	go func() {
		result := ts.BattleMgr.RunBattle(ctx, sess, troopID, true, true)
		resultCh <- result
	}()

	res := battlePump(t, ws, battlePumpOpts{Timeout: 30 * time.Second, ActionType: 0})

	// Verify strict message ordering.
	battleTypes := []string{}
	for _, pkt := range res.All {
		msgType, _ := pkt["type"].(string)
		switch msgType {
		case "battle_battle_start", "battle_input_request", "battle_turn_start",
			"battle_action_result", "battle_turn_end", "battle_battle_end":
			battleTypes = append(battleTypes, msgType)
		}
	}

	require.True(t, len(battleTypes) >= 5, "should have at least 5 battle messages, got %d: %v",
		len(battleTypes), battleTypes)

	// First message must be battle_start.
	assert.Equal(t, "battle_battle_start", battleTypes[0], "first battle message should be battle_start")

	// Last message must be battle_end.
	assert.Equal(t, "battle_battle_end", battleTypes[len(battleTypes)-1],
		"last battle message should be battle_end")

	// battle_start must come before any input_request.
	startIdx := indexOf(battleTypes, "battle_battle_start")
	firstInput := indexOf(battleTypes, "battle_input_request")
	assert.True(t, firstInput > startIdx, "input_request should come after battle_start")

	// input_request should come before turn_start (within a turn).
	firstTurn := indexOf(battleTypes, "battle_turn_start")
	assert.True(t, firstTurn > firstInput, "turn_start should come after input_request")

	// turn_start should come before action_result.
	firstAction := indexOf(battleTypes, "battle_action_result")
	if firstAction >= 0 {
		assert.True(t, firstAction > firstTurn, "action_result should come after turn_start")
	}

	// Should win quickly with level 30.
	assert.Equal(t, 0, res.BattleResult, "high level hero should win")

	t.Logf("Protocol order: %v", battleTypes)

	select {
	case <-resultCh:
	case <-time.After(5 * time.Second):
		t.Fatal("RunBattle did not return in time")
	}
}

// TestProjectBBattle_WSSnapshotFields verifies that BattlerSnapshot contains
// all required fields for the client to set up Scene_Battle properly.
func TestProjectBBattle_WSSnapshotFields(t *testing.T) {
	dataPath := projectBDataPath(t)
	ts := NewTestServerWithResources(t, dataPath)
	defer ts.Close()
	require.NotNil(t, ts.BattleMgr)

	user := UniqueID("btlsnp")
	token, _ := ts.Login(t, user, user+"pass")
	charID := ts.CreateCharacter(t, token, UniqueID("mage"), 1)
	ws := ts.ConnectWS(t, token)
	defer ws.Close()

	ws.Send("enter_map", map[string]interface{}{"char_id": charID})
	initPkt := ws.RecvType("map_init", 10*time.Second)
	require.NotNil(t, initPkt)
	ws.Send("scene_ready", map[string]interface{}{})
	time.Sleep(2 * time.Second)

	ws.DebugSetStats(t, map[string]interface{}{
		"level":  15,
		"hp":     800,
		"max_hp": 800,
		"mp":     100,
		"max_mp": 100,
	})

	sess := ts.SM.Get(charID)
	require.NotNil(t, sess)

	troopID := 4
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	go func() {
		ts.BattleMgr.RunBattle(ctx, sess, troopID, true, true)
	}()

	// Just capture battle_start, then cancel.
	var battleStart map[string]interface{}
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		pkt, err := ws.RecvAny(5 * time.Second)
		if err != nil {
			break
		}
		if pkt["type"] == "battle_battle_start" {
			battleStart = PayloadMap(t, pkt)
			break
		}
	}
	cancel() // end the battle

	require.NotNil(t, battleStart, "should receive battle_battle_start")

	// Verify actor snapshot fields.
	actors := payloadSlice(battleStart, "actors")
	require.True(t, len(actors) > 0)
	a, _ := actors[0].(map[string]interface{})
	require.NotNil(t, a)

	requiredActorFields := []string{"index", "is_actor", "name", "hp", "max_hp", "mp", "max_mp", "tp", "states", "class_id", "level"}
	for _, field := range requiredActorFields {
		_, exists := a[field]
		assert.True(t, exists, "actor snapshot should have field: %s (got: %v)", field, a)
	}
	assert.True(t, a["class_id"].(float64) > 0, "actor should have class_id > 0")
	assert.True(t, a["level"].(float64) > 0, "actor should have level > 0")

	// Verify enemy snapshot fields.
	enemies := payloadSlice(battleStart, "enemies")
	require.True(t, len(enemies) > 0)
	e, _ := enemies[0].(map[string]interface{})
	require.NotNil(t, e)

	requiredEnemyFields := []string{"index", "is_actor", "name", "hp", "max_hp", "mp", "max_mp", "tp", "states", "enemy_id"}
	for _, field := range requiredEnemyFields {
		_, exists := e[field]
		assert.True(t, exists, "enemy snapshot should have field: %s (got: %v)", field, e)
	}
	assert.True(t, e["enemy_id"].(float64) > 0, "enemy should have enemy_id > 0")
}

// indexOf returns the first index of needle in haystack, or -1 if not found.
func indexOf(haystack []string, needle string) int {
	for i, v := range haystack {
		if v == needle {
			return i
		}
	}
	return -1
}
