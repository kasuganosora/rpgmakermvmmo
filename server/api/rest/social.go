package rest

import (
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	mw "github.com/kasuganosora/rpgmakermvmmo/server/middleware"
	"github.com/kasuganosora/rpgmakermvmmo/server/model"
	"github.com/kasuganosora/rpgmakermvmmo/server/game/player"
	"gorm.io/gorm"
)

// SocialHandler handles friends and social REST endpoints.
type SocialHandler struct {
	db *gorm.DB
	sm *player.SessionManager
}

// NewSocialHandler creates a new SocialHandler.
func NewSocialHandler(db *gorm.DB, sm *player.SessionManager) *SocialHandler {
	return &SocialHandler{db: db, sm: sm}
}

// ListFriends handles GET /api/social/friends.
func (h *SocialHandler) ListFriends(c *gin.Context) {
	accountID := mw.GetAccountID(c)
	charID := getCharIDForAccount(h.db, accountID)
	if charID == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "no character selected"})
		return
	}

	var friends []model.Friendship
	if err := h.db.Where("char_id = ? AND status = 1", charID).Find(&friends).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "internal error"})
		return
	}

	type FriendInfo struct {
		model.Friendship
		Online bool `json:"online"`
	}
	result := make([]FriendInfo, len(friends))
	for i, f := range friends {
		result[i] = FriendInfo{
			Friendship: f,
			Online:     h.sm.IsOnline(f.FriendID),
		}
	}
	c.JSON(http.StatusOK, gin.H{"friends": result})
}

// SendFriendRequest handles POST /api/social/friends/request.
func (h *SocialHandler) SendFriendRequest(c *gin.Context) {
	accountID := mw.GetAccountID(c)
	charID := getCharIDForAccount(h.db, accountID)
	if charID == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "no character selected"})
		return
	}

	var req struct {
		TargetCharID int64 `json:"target_char_id" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	friendship := &model.Friendship{
		CharID:   charID,
		FriendID: req.TargetCharID,
		Status:   0, // pending
	}
	if err := h.db.Create(friendship).Error; err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "request failed"})
		return
	}
	c.JSON(http.StatusCreated, gin.H{"message": "request sent"})
}

// AcceptFriendRequest handles POST /api/social/friends/accept/:id.
func (h *SocialHandler) AcceptFriendRequest(c *gin.Context) {
	accountID := mw.GetAccountID(c)
	charID := getCharIDForAccount(h.db, accountID)
	if charID == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "no character selected"})
		return
	}

	reqID, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
		return
	}

	// Find the pending request.
	var friendship model.Friendship
	if err := h.db.Where("id = ? AND friend_id = ? AND status = 0", reqID, charID).
		First(&friendship).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "request not found"})
		return
	}

	// Both writes must succeed atomically; a half-accepted friendship would
	// make the relationship one-directional.
	if err := h.db.Transaction(func(tx *gorm.DB) error {
		friendship.Status = 1
		if err := tx.Save(&friendship).Error; err != nil {
			return err
		}
		return tx.Create(&model.Friendship{
			CharID:   charID,
			FriendID: friendship.CharID,
			Status:   1,
		}).Error
	}); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "internal error"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "accepted"})
}

// DeleteFriend handles DELETE /api/social/friends/:id.
func (h *SocialHandler) DeleteFriend(c *gin.Context) {
	accountID := mw.GetAccountID(c)
	charID := getCharIDForAccount(h.db, accountID)
	friendID, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || charID == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request"})
		return
	}
	h.db.Where("(char_id = ? AND friend_id = ?) OR (char_id = ? AND friend_id = ?)",
		charID, friendID, friendID, charID).Delete(&model.Friendship{})
	c.JSON(http.StatusOK, gin.H{"message": "deleted"})
}

// BlockPlayer handles POST /api/social/block/:id.
func (h *SocialHandler) BlockPlayer(c *gin.Context) {
	accountID := mw.GetAccountID(c)
	charID := getCharIDForAccount(h.db, accountID)
	targetID, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || charID == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request"})
		return
	}

	var f model.Friendship
	err = h.db.Where("char_id = ? AND friend_id = ?", charID, targetID).First(&f).Error
	if err != nil {
		f = model.Friendship{CharID: charID, FriendID: targetID}
	}
	f.Status = 2 // blocked
	h.db.Save(&f)
	c.JSON(http.StatusOK, gin.H{"message": "blocked"})
}

// getCharIDForAccount returns the first character ID for an account.
// In a full implementation this would require the client to specify which character.
func getCharIDForAccount(db *gorm.DB, accountID int64) int64 {
	var char model.Character
	if err := db.Where("account_id = ?", accountID).First(&char).Error; err != nil {
		return 0
	}
	return char.ID
}
