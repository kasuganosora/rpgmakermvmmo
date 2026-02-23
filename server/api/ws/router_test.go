package ws

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/kasuganosora/rpgmakermvmmo/server/game/player"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

func nop() *zap.Logger { l, _ := zap.NewDevelopment(); return l }

// newSession creates a minimal PlayerSession for testing.
func newSession(accountID, charID int64) *player.PlayerSession {
	return &player.PlayerSession{
		AccountID: accountID,
		CharID:    charID,
		SendChan:  make(chan []byte, 256),
		Done:      make(chan struct{}),
	}
}

func makePacket(t *testing.T, seq uint64, msgType string, payload interface{}) []byte {
	t.Helper()
	p, _ := json.Marshal(payload)
	pkt := player.Packet{Seq: seq, Type: msgType, Payload: p}
	b, err := json.Marshal(pkt)
	require.NoError(t, err)
	return b
}

func TestRouter_On_Dispatch_Basic(t *testing.T) {
	r := NewRouter(nop())
	called := false
	r.On("ping", func(ctx context.Context, s *player.PlayerSession, payload json.RawMessage) error {
		called = true
		return nil
	})

	s := newSession(1, 1)
	r.Dispatch(s, makePacket(t, 1, "ping", nil))
	assert.True(t, called)
}

func TestRouter_Dispatch_MalformedJSON(t *testing.T) {
	r := NewRouter(nop())
	s := newSession(1, 1)
	// Should not panic
	r.Dispatch(s, []byte("not json"))
}

func TestRouter_Dispatch_UnknownType(t *testing.T) {
	r := NewRouter(nop())
	called := false
	r.On("known", func(_ context.Context, _ *player.PlayerSession, _ json.RawMessage) error {
		called = true
		return nil
	})
	s := newSession(1, 1)
	r.Dispatch(s, makePacket(t, 1, "unknown", nil))
	assert.False(t, called)
}

func TestRouter_Dispatch_AntiReplay_RejectsOldSeq(t *testing.T) {
	r := NewRouter(nop())
	var callCount int
	r.On("msg", func(_ context.Context, _ *player.PlayerSession, _ json.RawMessage) error {
		callCount++
		return nil
	})
	s := newSession(1, 1)

	// First message with seq=5 → accepted
	r.Dispatch(s, makePacket(t, 5, "msg", nil))
	assert.Equal(t, 1, callCount)

	// Same seq=5 → rejected (replay)
	r.Dispatch(s, makePacket(t, 5, "msg", nil))
	assert.Equal(t, 1, callCount)

	// Lower seq=3 → rejected
	r.Dispatch(s, makePacket(t, 3, "msg", nil))
	assert.Equal(t, 1, callCount)
}

func TestRouter_Dispatch_AntiReplay_AcceptsNewSeq(t *testing.T) {
	r := NewRouter(nop())
	var callCount int
	r.On("msg", func(_ context.Context, _ *player.PlayerSession, _ json.RawMessage) error {
		callCount++
		return nil
	})
	s := newSession(1, 1)

	r.Dispatch(s, makePacket(t, 10, "msg", nil))
	r.Dispatch(s, makePacket(t, 11, "msg", nil))
	r.Dispatch(s, makePacket(t, 100, "msg", nil))
	assert.Equal(t, 3, callCount)
}

func TestRouter_Dispatch_SeqZero_SkipsAntiReplay(t *testing.T) {
	r := NewRouter(nop())
	var callCount int
	r.On("msg", func(_ context.Context, _ *player.PlayerSession, _ json.RawMessage) error {
		callCount++
		return nil
	})
	s := newSession(1, 1)
	s.LastSeq = 100 // high seq already seen

	// Seq=0 should bypass anti-replay
	r.Dispatch(s, makePacket(t, 0, "msg", nil))
	r.Dispatch(s, makePacket(t, 0, "msg", nil))
	assert.Equal(t, 2, callCount)
}

func TestRouter_Dispatch_PayloadPassed(t *testing.T) {
	r := NewRouter(nop())
	var got map[string]interface{}
	r.On("data", func(_ context.Context, _ *player.PlayerSession, raw json.RawMessage) error {
		return json.Unmarshal(raw, &got)
	})
	s := newSession(1, 1)
	r.Dispatch(s, makePacket(t, 1, "data", map[string]interface{}{"key": "value"}))
	assert.Equal(t, "value", got["key"])
}

func TestRouter_Dispatch_HandlerError_NosPanic(t *testing.T) {
	r := NewRouter(nop())
	r.On("err", func(_ context.Context, _ *player.PlayerSession, _ json.RawMessage) error {
		return assert.AnError
	})
	s := newSession(1, 1)
	// Should not panic even when handler returns error
	r.Dispatch(s, makePacket(t, 1, "err", nil))
}

func TestRouter_TraceIDFromCtx_Present(t *testing.T) {
	r := NewRouter(nop())
	var traceID string
	r.On("trace", func(ctx context.Context, _ *player.PlayerSession, _ json.RawMessage) error {
		traceID = TraceIDFromCtx(ctx)
		return nil
	})
	s := newSession(1, 1)
	r.Dispatch(s, makePacket(t, 1, "trace", nil))
	assert.NotEmpty(t, traceID)
}

func TestTraceIDFromCtx_Missing(t *testing.T) {
	id := TraceIDFromCtx(context.Background())
	assert.Equal(t, "", id)
}

func TestRouter_MultipleHandlers(t *testing.T) {
	r := NewRouter(nop())
	var calls []string
	r.On("a", func(_ context.Context, _ *player.PlayerSession, _ json.RawMessage) error {
		calls = append(calls, "a")
		return nil
	})
	r.On("b", func(_ context.Context, _ *player.PlayerSession, _ json.RawMessage) error {
		calls = append(calls, "b")
		return nil
	})
	s := newSession(1, 1)
	r.Dispatch(s, makePacket(t, 1, "a", nil))
	r.Dispatch(s, makePacket(t, 2, "b", nil))
	assert.Equal(t, []string{"a", "b"}, calls)
}

func TestRouter_ReplaceHandler(t *testing.T) {
	r := NewRouter(nop())
	var calls []string
	r.On("msg", func(_ context.Context, _ *player.PlayerSession, _ json.RawMessage) error {
		calls = append(calls, "first")
		return nil
	})
	r.On("msg", func(_ context.Context, _ *player.PlayerSession, _ json.RawMessage) error {
		calls = append(calls, "second")
		return nil
	})
	s := newSession(1, 1)
	r.Dispatch(s, makePacket(t, 1, "msg", nil))
	assert.Equal(t, []string{"second"}, calls)
}
