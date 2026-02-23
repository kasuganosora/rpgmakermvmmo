package ws

import (
	"context"
	"net/http"
	"runtime/debug"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
	"github.com/kasuganosora/rpgmakermvmmo/server/cache"
	"github.com/kasuganosora/rpgmakermvmmo/server/config"
	"github.com/kasuganosora/rpgmakermvmmo/server/game/party"
	"github.com/kasuganosora/rpgmakermvmmo/server/game/player"
	"github.com/kasuganosora/rpgmakermvmmo/server/game/trade"
	"github.com/kasuganosora/rpgmakermvmmo/server/game/world"
	mw "github.com/kasuganosora/rpgmakermvmmo/server/middleware"
	"github.com/kasuganosora/rpgmakermvmmo/server/model"
	"go.uber.org/zap"
	"gorm.io/gorm"
)

// Handler is the Gin handler for GET /ws.
type Handler struct {
	db       *gorm.DB
	cache    cache.Cache
	sec      config.SecurityConfig
	sm       *player.SessionManager
	wm       *world.WorldManager
	partyMgr *party.Manager
	tradeSvc *trade.Service
	router   *Router
	logger   *zap.Logger
	upgrader websocket.Upgrader
}

// NewHandler creates a new WebSocket Handler.
// sec.AllowedOrigins controls which WebSocket origins are accepted.
// An empty slice permits all origins (development only).
func NewHandler(
	db *gorm.DB,
	c cache.Cache,
	sec config.SecurityConfig,
	sm *player.SessionManager,
	wm *world.WorldManager,
	partyMgr *party.Manager,
	tradeSvc *trade.Service,
	router *Router,
	logger *zap.Logger,
) *Handler {
	h := &Handler{
		db:       db,
		cache:    c,
		sec:      sec,
		sm:       sm,
		wm:       wm,
		partyMgr: partyMgr,
		tradeSvc: tradeSvc,
		router:   router,
		logger:   logger,
	}
	allowed := sec.AllowedOrigins
	h.upgrader = websocket.Upgrader{
		ReadBufferSize:  4096,
		WriteBufferSize: 4096,
		CheckOrigin: func(r *http.Request) bool {
			if len(allowed) == 0 {
				return true // dev mode: allow all
			}
			origin := r.Header.Get("Origin")
			for _, o := range allowed {
				if o == origin {
					return true
				}
			}
			return false
		},
	}
	return h
}

// ServeWS handles GET /ws?token=<jwt>.
func (h *Handler) ServeWS(c *gin.Context) {
	tokenStr := c.Query("token")
	if tokenStr == "" {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "missing token"})
		return
	}

	// Validate JWT.
	claims, err := mw.ParseToken(tokenStr, h.sec.JWTSecret)
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid token"})
		return
	}

	// Validate session cache.
	ctx, cancel := context.WithTimeout(c.Request.Context(), 2*time.Second)
	defer cancel()
	exists, err := h.cache.Exists(ctx, "session:"+tokenStr)
	if err != nil || !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "session expired"})
		return
	}

	// Upgrade to WebSocket.
	conn, err := h.upgrader.Upgrade(c.Writer, c.Request, nil)
	if err != nil {
		h.logger.Error("ws upgrade failed", zap.Error(err))
		return
	}

	// Create session with logger.
	sess := player.NewPlayerSession(claims.AccountID, 0, conn, h.logger)

	// Start read pump (blocks until connection closes).
	h.sm.Register(sess)
	h.readPump(sess)
}

// readPump reads messages from the WebSocket connection and dispatches them.
func (h *Handler) readPump(s *player.PlayerSession) {
	defer func() {
		h.handleDisconnect(s)
	}()

	s.SetReadDeadline()
	s.Conn.SetPongHandler(func(string) error {
		s.SetReadDeadline()
		return nil
	})

	for {
		_, raw, err := s.Conn.ReadMessage()
		if err != nil {
			if websocket.IsUnexpectedCloseError(err,
				websocket.CloseGoingAway,
				websocket.CloseNormalClosure,
				websocket.CloseNoStatusReceived) {
				h.logger.Warn("ws unexpected close",
					zap.Int64("account_id", s.AccountID),
					zap.Error(err))
			}
			return
		}
		// Reset read deadline on any message (heartbeat or otherwise).
		s.SetReadDeadline()
		h.router.Dispatch(s, raw)
	}
}

// handleDisconnect cleans up the session after the connection closes.
func (h *Handler) handleDisconnect(s *player.PlayerSession) {
	s.Close()

	// Remove from map room and broadcast player_leave.
	if s.MapID != 0 && s.CharID != 0 {
		leaveMap(s, h.wm, h.logger)
	}

	// Clean up party state (leave party + remove pending invites).
	if s.CharID != 0 && h.partyMgr != nil {
		h.partyMgr.LeaveParty(s)
		h.partyMgr.CleanupInvites(s.CharID)
	}

	// Clean up active trade session.
	if s.CharID != 0 && h.tradeSvc != nil {
		h.tradeSvc.Cancel(s)
	}

	// Unload per-player game state cache.
	if s.CharID != 0 {
		h.wm.PlayerStateManager().Unload(s.CharID)
	}

	h.sm.Unregister(s.CharID)
	h.logger.Info("player disconnected",
		zap.Int64("account_id", s.AccountID),
		zap.Int64("char_id", s.CharID))

	// Async: save last position to DB.
	if s.CharID != 0 {
		go func() {
			defer func() {
				if r := recover(); r != nil {
					h.logger.Error("panic in disconnect save",
						zap.Int64("char_id", s.CharID),
						zap.Any("recover", r),
						zap.String("stack", string(debug.Stack())))
				}
			}()
			x, y, dir := s.Position()
			h.db.Model(&model.Character{}).
				Where("id = ?", s.CharID).
				Updates(map[string]interface{}{
					"map_id":    s.MapID,
					"map_x":     x,
					"map_y":     y,
					"direction": dir,
					"hp":        s.HP,
					"mp":        s.MP,
					"level":     s.Level,
					"exp":       s.Exp,
				})
		}()
	}
}
