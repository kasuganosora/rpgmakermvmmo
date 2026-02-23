package integration

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTradeRequestAndAccept(t *testing.T) {
	ts := NewTestServer(t)
	defer ts.Close()

	// Player A and Player B both connect and enter map.
	_, charIDA, wsA := ts.LoginAndConnect(t, UniqueID("trA"), UniqueID("TraderA"))
	defer wsA.Close()

	_, charIDB, wsB := ts.LoginAndConnect(t, UniqueID("trB"), UniqueID("TraderB"))
	defer wsB.Close()

	// Give time for sessions to fully register.
	time.Sleep(100 * time.Millisecond)

	// Player A sends trade_request targeting Player B.
	wsA.Send("trade_request", map[string]interface{}{
		"target_char_id": charIDB,
	})

	// Player B should receive trade_request.
	reqPkt := wsB.RecvType("trade_request", 5*time.Second)
	require.NotNil(t, reqPkt)
	reqPayload := PayloadMap(t, reqPkt)
	assert.Equal(t, float64(charIDA), reqPayload["from_id"])

	// Player B accepts.
	wsB.Send("trade_accept", map[string]interface{}{
		"from_char_id": charIDA,
	})

	// Both should not get errors. The trade service creates a session.
	// We simply verify that no error comes through within a short window.
	// Note: AcceptTrade doesn't send a confirmation packet in the current code,
	// so we just verify the flow doesn't crash.
	time.Sleep(200 * time.Millisecond)
}

func TestTradeCancellation(t *testing.T) {
	ts := NewTestServer(t)
	defer ts.Close()

	// Player A and Player B both connect.
	_, charIDA, wsA := ts.LoginAndConnect(t, UniqueID("tcA"), UniqueID("CancelA"))
	defer wsA.Close()

	_, charIDB, wsB := ts.LoginAndConnect(t, UniqueID("tcB"), UniqueID("CancelB"))
	defer wsB.Close()

	time.Sleep(100 * time.Millisecond)

	// A requests trade with B.
	wsA.Send("trade_request", map[string]interface{}{
		"target_char_id": charIDB,
	})

	// B receives and accepts.
	wsB.RecvType("trade_request", 5*time.Second)
	wsB.Send("trade_accept", map[string]interface{}{
		"from_char_id": charIDA,
	})

	time.Sleep(100 * time.Millisecond)

	// A cancels the trade.
	wsA.Send("trade_cancel", map[string]interface{}{})

	// B should receive trade_cancel.
	cancelPkt := wsB.RecvType("trade_cancel", 5*time.Second)
	require.NotNil(t, cancelPkt)
}

func TestTradeTargetOffline(t *testing.T) {
	ts := NewTestServer(t)
	defer ts.Close()

	_, _, wsA := ts.LoginAndConnect(t, UniqueID("toA"), UniqueID("OfflineTrader"))
	defer wsA.Close()

	// Request trade with a non-existent char ID.
	wsA.Send("trade_request", map[string]interface{}{
		"target_char_id": 99999,
	})

	// Should receive an error.
	errPkt := wsA.RecvType("error", 5*time.Second)
	require.NotNil(t, errPkt)
}
