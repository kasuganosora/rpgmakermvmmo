package rest

import (
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	mw "github.com/kasuganosora/rpgmakermvmmo/server/middleware"
	"github.com/kasuganosora/rpgmakermvmmo/server/model"
	"gorm.io/gorm"
)

// MailHandler handles in-game mail REST endpoints.
type MailHandler struct {
	db *gorm.DB
}

// NewMailHandler creates a MailHandler.
func NewMailHandler(db *gorm.DB) *MailHandler {
	return &MailHandler{db: db}
}

// List returns all non-expired mails for the given character.
// GET /api/characters/:id/mail
func (h *MailHandler) List(c *gin.Context) {
	accountID := mw.GetAccountID(c)
	charID, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
		return
	}
	if !h.ownsChar(accountID, charID) {
		c.JSON(http.StatusForbidden, gin.H{"error": "forbidden"})
		return
	}

	var mails []model.Mail
	h.db.Where("to_char_id = ? AND (expire_at IS NULL OR expire_at > ?)", charID, time.Now()).
		Order("created_at DESC").
		Limit(50).
		Find(&mails)
	c.JSON(http.StatusOK, gin.H{"mails": mails})
}

// Claim claims (and marks) the attachment of a mail.
// POST /api/characters/:id/mail/:mail_id/claim
func (h *MailHandler) Claim(c *gin.Context) {
	accountID := mw.GetAccountID(c)
	charID, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
		return
	}
	if !h.ownsChar(accountID, charID) {
		c.JSON(http.StatusForbidden, gin.H{"error": "forbidden"})
		return
	}

	mailID, err := strconv.ParseInt(c.Param("mail_id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid mail_id"})
		return
	}

	var mail model.Mail
	if err := h.db.Where("id = ? AND to_char_id = ?", mailID, charID).First(&mail).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "mail not found"})
		return
	}
	if mail.Claimed != 0 {
		c.JSON(http.StatusConflict, gin.H{"error": "already claimed"})
		return
	}

	h.db.Model(&mail).Update("claimed", 1)
	c.JSON(http.StatusOK, gin.H{"ok": true, "attachment": mail.Attachment})
}

func (h *MailHandler) ownsChar(accountID, charID int64) bool {
	var char model.Character
	return h.db.Where("id = ? AND account_id = ?", charID, accountID).First(&char).Error == nil
}
