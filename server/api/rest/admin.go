package rest

import (
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/kasuganosora/rpgmakermvmmo/server/game/player"
	"github.com/kasuganosora/rpgmakermvmmo/server/game/world"
	"github.com/kasuganosora/rpgmakermvmmo/server/model"
	"github.com/kasuganosora/rpgmakermvmmo/server/scheduler"
	"go.uber.org/zap"
	"gorm.io/gorm"
)

// AdminHandler handles admin-only REST endpoints.
// Routes should be protected by AdminAuth middleware.
type AdminHandler struct {
	db        *gorm.DB
	sm        *player.SessionManager
	wm        *world.WorldManager
	sched     *scheduler.Scheduler
	logger    *zap.Logger
}

// NewAdminHandler creates an AdminHandler.
func NewAdminHandler(
	db *gorm.DB,
	sm *player.SessionManager,
	wm *world.WorldManager,
	sched *scheduler.Scheduler,
	logger *zap.Logger,
) *AdminHandler {
	return &AdminHandler{db: db, sm: sm, wm: wm, sched: sched, logger: logger}
}

// Metrics returns server health metrics.
// GET /api/admin/metrics
func (h *AdminHandler) Metrics(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{
		"online_players": h.sm.Count(),
		"active_rooms":   h.wm.ActiveRoomCount(),
		"scheduler_tasks": h.sched.ListTickers(),
	})
}

// ListPlayers returns a snapshot of all online players.
// GET /api/admin/players
func (h *AdminHandler) ListPlayers(c *gin.Context) {
	sessions := h.sm.All()
	type playerInfo struct {
		CharID   int64  `json:"char_id"`
		CharName string `json:"char_name"`
		MapID    int    `json:"map_id"`
		X        int    `json:"x"`
		Y        int    `json:"y"`
	}
	result := make([]playerInfo, 0, len(sessions))
	for _, s := range sessions {
		x, y, _ := s.Position()
		result = append(result, playerInfo{
			CharID:   s.CharID,
			CharName: s.CharName,
			MapID:    s.MapID,
			X:        x,
			Y:        y,
		})
	}
	c.JSON(http.StatusOK, gin.H{"players": result, "count": len(result)})
}

// KickPlayer forcibly disconnects a player by character ID.
// POST /api/admin/kick/:id
func (h *AdminHandler) KickPlayer(c *gin.Context) {
	charID, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
		return
	}
	s := h.sm.Get(charID)
	if s == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "player not online"})
		return
	}
	s.Close()
	h.logger.Info("admin kicked player", zap.Int64("char_id", charID))
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

// BanAccount bans or unbans a player account.
// POST /api/admin/accounts/:id/ban
func (h *AdminHandler) BanAccount(c *gin.Context) {
	accountID, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
		return
	}
	var req struct {
		Ban bool `json:"ban"`
	}
	_ = c.ShouldBindJSON(&req)

	status := 1
	if req.Ban {
		status = 0
	}
	result := h.db.Model(&model.Account{}).Where("id = ?", accountID).Update("status", status)
	if result.Error != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "db error"})
		return
	}
	if result.RowsAffected == 0 {
		c.JSON(http.StatusNotFound, gin.H{"error": "account not found"})
		return
	}

	// Kick the player if currently online.
	if req.Ban {
		for _, s := range h.sm.All() {
			if s.AccountID == accountID {
				s.Close()
			}
		}
	}
	c.JSON(http.StatusOK, gin.H{"ok": true, "status": status})
}

// ListSchedulerTasks returns names of all registered ticker tasks.
// GET /api/admin/scheduler
func (h *AdminHandler) ListSchedulerTasks(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{"tasks": h.sched.ListTickers()})
}

// AdminAuth returns a middleware that checks the X-Admin-Key header.
// WARNING: if adminKey is empty all admin endpoints are disabled (503) so the
// server cannot be accidentally deployed without protection. Set a non-empty
// server.admin_key in config to enable admin routes.
func AdminAuth(adminKey string) gin.HandlerFunc {
	return func(c *gin.Context) {
		if adminKey == "" {
			c.AbortWithStatusJSON(http.StatusServiceUnavailable,
				gin.H{"error": "admin endpoints disabled: set server.admin_key in config"})
			return
		}
		key := c.GetHeader("X-Admin-Key")
		if key != adminKey {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
			return
		}
		c.Next()
	}
}
