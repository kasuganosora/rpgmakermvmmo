package ws

import (
	"context"
	"encoding/json"

	"github.com/google/uuid"
	"github.com/kasuganosora/rpgmakermvmmo/server/game/player"
	"go.uber.org/zap"
)

// HandlerFunc processes a decoded WS message payload.
type HandlerFunc func(ctx context.Context, session *player.PlayerSession, payload json.RawMessage) error

// Router dispatches incoming WS packets to registered handlers.
type Router struct {
	handlers map[string]HandlerFunc
	logger   *zap.Logger
}

// NewRouter creates a new Router.
func NewRouter(logger *zap.Logger) *Router {
	return &Router{
		handlers: make(map[string]HandlerFunc),
		logger:   logger,
	}
}

// On registers a HandlerFunc for the given message type.
func (r *Router) On(msgType string, fn HandlerFunc) {
	r.handlers[msgType] = fn
}

// Dispatch decodes raw bytes, validates seq, and invokes the appropriate handler.
func (r *Router) Dispatch(s *player.PlayerSession, raw []byte) {
	var pkt player.Packet
	if err := json.Unmarshal(raw, &pkt); err != nil {
		r.logger.Warn("malformed packet",
			zap.Int64("account_id", s.AccountID),
			zap.Error(err))
		return
	}

	// Monotonic seq check (anti-replay). Seq == 0 means no seq tracking.
	if pkt.Seq != 0 && pkt.Seq <= s.LastSeq {
		r.logger.Warn("replayed or out-of-order packet",
			zap.Int64("account_id", s.AccountID),
			zap.Uint64("seq", pkt.Seq),
			zap.Uint64("last_seq", s.LastSeq))
		return
	}
	if pkt.Seq != 0 {
		s.LastSeq = pkt.Seq
	}

	// Assign a trace ID for this message dispatch.
	s.TraceID = uuid.NewString()
	ctx := context.WithValue(context.Background(), ctxKeyTraceID{}, s.TraceID)

	fn, ok := r.handlers[pkt.Type]
	if !ok {
		r.logger.Debug("unhandled message type",
			zap.String("type", pkt.Type),
			zap.Int64("account_id", s.AccountID))
		return
	}

	if err := fn(ctx, s, pkt.Payload); err != nil {
		r.logger.Error("handler error",
			zap.String("type", pkt.Type),
			zap.Int64("account_id", s.AccountID),
			zap.String("trace_id", s.TraceID),
			zap.Error(err))
	}
}

type ctxKeyTraceID struct{}

// TraceIDFromCtx extracts the trace ID from a handler context.
func TraceIDFromCtx(ctx context.Context) string {
	if v, ok := ctx.Value(ctxKeyTraceID{}).(string); ok {
		return v
	}
	return ""
}
