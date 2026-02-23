package sse

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/kasuganosora/rpgmakermvmmo/server/cache"
	"github.com/kasuganosora/rpgmakermvmmo/server/config"
	mw "github.com/kasuganosora/rpgmakermvmmo/server/middleware"
	"go.uber.org/zap"
)

const announceChannel = "announce"

// Handler handles the SSE endpoint.
type Handler struct {
	pubsub cache.PubSub
	sec    config.SecurityConfig
	c      cache.Cache
	logger *zap.Logger
}

// NewHandler creates a new SSE Handler.
func NewHandler(pubsub cache.PubSub, c cache.Cache, sec config.SecurityConfig, logger *zap.Logger) *Handler {
	return &Handler{pubsub: pubsub, c: c, sec: sec, logger: logger}
}

// ServeSSE handles GET /sse?token=<jwt>.
// It streams server-sent events to authenticated clients.
// Currently delivers system announcements published to the "announce" channel.
func (h *Handler) ServeSSE(c *gin.Context) {
	tokenStr := c.Query("token")
	if tokenStr == "" {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "missing token"})
		return
	}

	_, err := mw.ParseToken(tokenStr, h.sec.JWTSecret)
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid token"})
		return
	}

	ctx, cancel := context.WithTimeout(c.Request.Context(), 2*time.Second)
	defer cancel()
	exists, err := h.c.Exists(ctx, "session:"+tokenStr)
	if err != nil || !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "session expired"})
		return
	}

	// Set SSE headers.
	c.Header("Content-Type", "text/event-stream")
	c.Header("Cache-Control", "no-cache")
	c.Header("Connection", "keep-alive")
	c.Header("X-Accel-Buffering", "no")

	subCtx, subCancel := context.WithCancel(c.Request.Context())
	defer subCancel()

	msgCh, unsub, err := h.pubsub.Subscribe(subCtx, announceChannel)
	if err != nil {
		h.logger.Error("sse subscribe failed", zap.Error(err))
		c.Status(http.StatusInternalServerError)
		return
	}
	defer unsub()

	// Send initial connected event.
	fmt.Fprintf(c.Writer, "event: connected\ndata: {}\n\n")
	c.Writer.Flush()

	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case msg, ok := <-msgCh:
			if !ok {
				return
			}
			fmt.Fprintf(c.Writer, "event: announce\ndata: %s\n\n", msg.Payload)
			c.Writer.Flush()

		case <-ticker.C:
			// Keepalive comment to prevent proxy timeouts.
			fmt.Fprintf(c.Writer, ": keepalive\n\n")
			c.Writer.Flush()

		case <-c.Request.Context().Done():
			return
		}
	}
}

// Announce publishes an announcement message to all SSE subscribers.
func (h *Handler) Announce(ctx context.Context, message string) error {
	return h.pubsub.Publish(ctx, announceChannel, message)
}
