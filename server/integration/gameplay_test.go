package integration

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestEnterMapAndReceiveInit(t *testing.T) {
	ts := NewTestServer(t)
	defer ts.Close()

	token, _ := ts.Login(t, UniqueID("mapA"), "pass1234")
	charID := ts.CreateCharacter(t, token, UniqueID("MapHero"), 1)

	ws := ts.ConnectWS(t, token)
	defer ws.Close()

	// Send enter_map.
	ws.Send("enter_map", map[string]interface{}{"char_id": charID})

	// Should receive map_init.
	pkt := ws.RecvType("map_init", 5*time.Second)
	require.NotNil(t, pkt)
	assert.Equal(t, "map_init", pkt["type"])
}

func TestTwoPlayersOnSameMap(t *testing.T) {
	ts := NewTestServer(t)
	defer ts.Close()

	// Player A enters map.
	_, charIDA, wsA := ts.LoginAndConnect(t, UniqueID("pA"), UniqueID("PlayerA"))
	defer wsA.Close()

	// Player B enters the same map.
	tokenB, _ := ts.Login(t, UniqueID("pB"), UniqueID("pBpass"))
	charIDB := ts.CreateCharacter(t, tokenB, UniqueID("PlayerB"), 1)
	wsB := ts.ConnectWS(t, tokenB)
	defer wsB.Close()

	wsB.Send("enter_map", map[string]interface{}{"char_id": charIDB})

	// Player B receives map_init that should include Player A in the players list.
	pkt := wsB.RecvType("map_init", 5*time.Second)
	require.NotNil(t, pkt)
	payload := PayloadMap(t, pkt)
	players, ok := payload["players"].([]interface{})
	require.True(t, ok, "map_init should have players array")

	// Find Player A in the players list.
	found := false
	for _, p := range players {
		pm, ok := p.(map[string]interface{})
		if !ok {
			continue
		}
		if int64(pm["char_id"].(float64)) == charIDA {
			found = true
			break
		}
	}
	assert.True(t, found, "Player A should be in Player B's map_init players list")

	// Player A should receive player_join for Player B.
	joinPkt := wsA.RecvType("player_join", 5*time.Second)
	require.NotNil(t, joinPkt)
	joinPayload := PayloadMap(t, joinPkt)
	assert.Equal(t, float64(charIDB), joinPayload["char_id"])
}

func TestPlayerMoveSync(t *testing.T) {
	ts := NewTestServer(t)
	defer ts.Close()

	// Both players enter the same map.
	_, _, wsA := ts.LoginAndConnect(t, UniqueID("moveA"), UniqueID("MoverA"))
	defer wsA.Close()

	_, _, wsB := ts.LoginAndConnect(t, UniqueID("moveB"), UniqueID("MoverB"))
	defer wsB.Close()

	// Give some time for both to be in the room.
	time.Sleep(100 * time.Millisecond)

	// Player A moves.
	wsA.Send("player_move", map[string]interface{}{
		"x":   1,
		"y":   0,
		"dir": 6, // right
	})

	// Player B should receive player_sync for Player A.
	syncPkt := wsB.RecvType("player_sync", 5*time.Second)
	require.NotNil(t, syncPkt)
	syncPayload := PayloadMap(t, syncPkt)
	assert.NotNil(t, syncPayload["char_id"])
}

func TestPingPong(t *testing.T) {
	ts := NewTestServer(t)
	defer ts.Close()

	_, _, ws := ts.LoginAndConnect(t, UniqueID("ping"), UniqueID("Pinger"))
	defer ws.Close()

	// Send ping.
	ws.Send("ping", map[string]interface{}{"ts": time.Now().UnixMilli()})

	// Should receive pong.
	pkt := ws.RecvType("pong", 5*time.Second)
	require.NotNil(t, pkt)
	assert.Equal(t, "pong", pkt["type"])
}
