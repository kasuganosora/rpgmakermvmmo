package ws

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ---- NPCHandlers: HandleEffectAck ----

func TestHandleEffectAck_EmptyPayload(t *testing.T) {
	// Empty ack should signal EffectAckCh without changing position.
	h := NewNPCHandlers(nil, nil, nil, nop())

	s := newSession(1, 10)
	s.EffectAckCh = make(chan struct{}, 1)
	s.SetPosition(3, 7, 2)

	raw, _ := json.Marshal(map[string]interface{}{})
	err := h.HandleEffectAck(nil, s, raw)
	require.NoError(t, err)

	// Channel should be signalled.
	select {
	case <-s.EffectAckCh:
		// OK
	default:
		t.Error("expected EffectAckCh to be signalled")
	}

	// Position should remain unchanged.
	x, y, dir := s.Position()
	assert.Equal(t, 3, x, "x unchanged")
	assert.Equal(t, 7, y, "y unchanged")
	assert.Equal(t, 2, dir, "dir unchanged")
}

func TestHandleEffectAck_WithPosition(t *testing.T) {
	// Ack with position data should update session position.
	h := NewNPCHandlers(nil, nil, nil, nop())

	s := newSession(1, 10)
	s.EffectAckCh = make(chan struct{}, 1)
	s.SetPosition(1, 1, 2) // initial position

	raw, _ := json.Marshal(map[string]interface{}{
		"x": 5, "y": 10, "dir": 6,
	})
	err := h.HandleEffectAck(nil, s, raw)
	require.NoError(t, err)

	// Position should be updated to ack values.
	x, y, dir := s.Position()
	assert.Equal(t, 5, x, "x updated from ack")
	assert.Equal(t, 10, y, "y updated from ack")
	assert.Equal(t, 6, dir, "dir updated from ack")

	// Channel should also be signalled.
	select {
	case <-s.EffectAckCh:
		// OK
	default:
		t.Error("expected EffectAckCh to be signalled")
	}
}

func TestHandleEffectAck_PartialPosition(t *testing.T) {
	// Ack with x,y but no dir should update x,y and keep existing dir.
	h := NewNPCHandlers(nil, nil, nil, nop())

	s := newSession(1, 10)
	s.EffectAckCh = make(chan struct{}, 1)
	s.SetPosition(1, 1, 8) // initial: facing up

	raw, _ := json.Marshal(map[string]interface{}{
		"x": 3, "y": 5,
	})
	err := h.HandleEffectAck(nil, s, raw)
	require.NoError(t, err)

	x, y, dir := s.Position()
	assert.Equal(t, 3, x, "x updated")
	assert.Equal(t, 5, y, "y updated")
	assert.Equal(t, 8, dir, "dir preserved (no dir in ack)")
}

func TestHandleEffectAck_OnlyX_NoUpdate(t *testing.T) {
	// Ack with only x (no y) should NOT update position — both x and y required.
	h := NewNPCHandlers(nil, nil, nil, nop())

	s := newSession(1, 10)
	s.EffectAckCh = make(chan struct{}, 1)
	s.SetPosition(1, 1, 2)

	raw, _ := json.Marshal(map[string]interface{}{
		"x": 99,
	})
	err := h.HandleEffectAck(nil, s, raw)
	require.NoError(t, err)

	x, y, _ := s.Position()
	assert.Equal(t, 1, x, "x unchanged — incomplete position")
	assert.Equal(t, 1, y, "y unchanged — incomplete position")
}

func TestHandleEffectAck_ZeroPosition(t *testing.T) {
	// Ack with x=0, y=0 should update position (origin is valid).
	h := NewNPCHandlers(nil, nil, nil, nop())

	s := newSession(1, 10)
	s.EffectAckCh = make(chan struct{}, 1)
	s.SetPosition(5, 5, 4)

	raw, _ := json.Marshal(map[string]interface{}{
		"x": 0, "y": 0, "dir": 2,
	})
	err := h.HandleEffectAck(nil, s, raw)
	require.NoError(t, err)

	x, y, dir := s.Position()
	assert.Equal(t, 0, x, "x=0 is valid")
	assert.Equal(t, 0, y, "y=0 is valid")
	assert.Equal(t, 2, dir, "dir updated")
}

func TestHandleEffectAck_NoExecutorWaiting(t *testing.T) {
	// When no executor is waiting (EffectAckCh full), should not panic or block.
	h := NewNPCHandlers(nil, nil, nil, nop())

	s := newSession(1, 10)
	s.EffectAckCh = make(chan struct{}, 1)
	// Pre-fill the channel to simulate "no executor waiting".
	s.EffectAckCh <- struct{}{}

	raw, _ := json.Marshal(map[string]interface{}{})

	done := make(chan struct{})
	go func() {
		err := h.HandleEffectAck(nil, s, raw)
		assert.NoError(t, err)
		close(done)
	}()

	select {
	case <-done:
		// OK — did not block
	case <-time.After(200 * time.Millisecond):
		t.Error("HandleEffectAck should not block when channel is full")
	}
}

func TestHandleEffectAck_ViaRouter(t *testing.T) {
	// End-to-end: dispatch npc_effect_ack through the router with position data.
	h := NewNPCHandlers(nil, nil, nil, nop())

	r := NewRouter(nop())
	h.RegisterHandlers(r)

	s := newSession(1, 10)
	s.EffectAckCh = make(chan struct{}, 1)
	s.SetPosition(0, 0, 2)

	raw := makePacket(t, 1, "npc_effect_ack", map[string]interface{}{
		"x": 8, "y": 12, "dir": 4,
	})
	r.Dispatch(s, raw)

	// Give async dispatch a moment.
	time.Sleep(50 * time.Millisecond)

	x, y, dir := s.Position()
	assert.Equal(t, 8, x)
	assert.Equal(t, 12, y)
	assert.Equal(t, 4, dir)
}
