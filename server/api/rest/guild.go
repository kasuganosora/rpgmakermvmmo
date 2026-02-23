package rest

import (
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	mw "github.com/kasuganosora/rpgmakermvmmo/server/middleware"
	"github.com/kasuganosora/rpgmakermvmmo/server/model"
	"gorm.io/gorm"
)

// GuildHandler handles guild REST endpoints.
type GuildHandler struct {
	db *gorm.DB
}

// NewGuildHandler creates a new GuildHandler.
func NewGuildHandler(db *gorm.DB) *GuildHandler {
	return &GuildHandler{db: db}
}

type createGuildRequest struct {
	Name   string `json:"name"   binding:"required,min=2,max=32"`
	Notice string `json:"notice" binding:"max=200"`
}

// Create handles POST /api/guilds.
func (h *GuildHandler) Create(c *gin.Context) {
	accountID := mw.GetAccountID(c)
	charID := getCharIDForAccount(h.db, accountID)
	if charID == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "no character"})
		return
	}

	var req createGuildRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// All three writes must succeed atomically; partial failure would leave an
	// orphaned guild with no members or a character with a stale guild_id.
	var guild model.Guild
	if err := h.db.Transaction(func(tx *gorm.DB) error {
		guild = model.Guild{
			Name:     req.Name,
			LeaderID: charID,
			Notice:   req.Notice,
			Level:    1,
		}
		if err := tx.Create(&guild).Error; err != nil {
			return err
		}
		if err := tx.Create(&model.GuildMember{GuildID: guild.ID, CharID: charID, Rank: 1}).Error; err != nil {
			return err
		}
		return tx.Model(&model.Character{}).Where("id = ?", charID).Update("guild_id", guild.ID).Error
	}); err != nil {
		if isUniqueViolation(err) {
			c.JSON(http.StatusBadRequest, gin.H{"error": "guild name already taken"})
		} else {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "internal error"})
		}
		return
	}

	c.JSON(http.StatusCreated, guild)
}

// Detail handles GET /api/guilds/:id.
func (h *GuildHandler) Detail(c *gin.Context) {
	guildID, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
		return
	}

	var guild model.Guild
	if err := h.db.First(&guild, guildID).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "guild not found"})
		return
	}

	var members []model.GuildMember
	h.db.Where("guild_id = ?", guildID).Find(&members)

	c.JSON(http.StatusOK, gin.H{"guild": guild, "members": members})
}

// Join handles POST /api/guilds/:id/join.
func (h *GuildHandler) Join(c *gin.Context) {
	accountID := mw.GetAccountID(c)
	charID := getCharIDForAccount(h.db, accountID)
	if charID == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "no character"})
		return
	}

	guildID, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
		return
	}

	var guild model.Guild
	if err := h.db.First(&guild, guildID).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "guild not found"})
		return
	}

	// Simplified: auto-accept (full version requires leader approval).
	if err := h.db.Create(&model.GuildMember{GuildID: guildID, CharID: charID, Rank: 4}).Error; err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "join failed"})
		return
	}
	h.db.Model(&model.Character{}).Where("id = ?", charID).Update("guild_id", guildID)
	c.JSON(http.StatusOK, gin.H{"message": "joined"})
}

// KickMember handles DELETE /api/guilds/:id/members/:cid.
func (h *GuildHandler) KickMember(c *gin.Context) {
	accountID := mw.GetAccountID(c)
	requesterCharID := getCharIDForAccount(h.db, accountID)
	guildID, err1 := strconv.ParseInt(c.Param("id"), 10, 64)
	targetCharID, err2 := strconv.ParseInt(c.Param("cid"), 10, 64)
	if err1 != nil || err2 != nil || requesterCharID == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request"})
		return
	}

	// Verify requester is guild leader or officer.
	var requesterMember model.GuildMember
	if err := h.db.Where("guild_id = ? AND char_id = ?", guildID, requesterCharID).
		First(&requesterMember).Error; err != nil {
		c.JSON(http.StatusForbidden, gin.H{"error": "not a guild member"})
		return
	}
	if requesterMember.Rank > 2 {
		c.JSON(http.StatusForbidden, gin.H{"error": "insufficient rank"})
		return
	}

	h.db.Where("guild_id = ? AND char_id = ?", guildID, targetCharID).Delete(&model.GuildMember{})
	h.db.Model(&model.Character{}).Where("id = ?", targetCharID).Update("guild_id", nil)
	c.JSON(http.StatusOK, gin.H{"message": "kicked"})
}

// UpdateNotice handles PUT /api/guilds/:id/notice.
func (h *GuildHandler) UpdateNotice(c *gin.Context) {
	accountID := mw.GetAccountID(c)
	charID := getCharIDForAccount(h.db, accountID)
	guildID, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || charID == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request"})
		return
	}

	var guild model.Guild
	if err := h.db.First(&guild, guildID).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "guild not found"})
		return
	}
	if guild.LeaderID != charID {
		c.JSON(http.StatusForbidden, gin.H{"error": "only leader can update notice"})
		return
	}

	var req struct {
		Notice string `json:"notice" binding:"max=500"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	h.db.Model(&guild).Update("notice", req.Notice)
	c.JSON(http.StatusOK, gin.H{"message": "updated"})
}
