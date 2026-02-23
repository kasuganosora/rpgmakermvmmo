package rest

import (
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/kasuganosora/rpgmakermvmmo/server/config"
	mw "github.com/kasuganosora/rpgmakermvmmo/server/middleware"
	"github.com/kasuganosora/rpgmakermvmmo/server/model"
	"github.com/kasuganosora/rpgmakermvmmo/server/resource"
	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"
)

const maxCharacters = 3

// CharacterHandler handles character REST endpoints.
type CharacterHandler struct {
	db   *gorm.DB
	res  *resource.ResourceLoader
	game config.GameConfig
}

// NewCharacterHandler creates a new CharacterHandler.
func NewCharacterHandler(db *gorm.DB, res *resource.ResourceLoader, game config.GameConfig) *CharacterHandler {
	return &CharacterHandler{db: db, res: res, game: game}
}

// List handles GET /api/characters.
func (h *CharacterHandler) List(c *gin.Context) {
	accountID := mw.GetAccountID(c)
	var chars []model.Character
	if err := h.db.Where("account_id = ?", accountID).Find(&chars).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "internal error"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"characters": chars})
}

type createCharacterRequest struct {
	Name      string `json:"name"       binding:"required,min=1,max=32"`
	ClassID   int    `json:"class_id"   binding:"required"`
	WalkName  string `json:"walk_name"  binding:"required"`
	WalkIndex int    `json:"walk_index"`
	FaceName  string `json:"face_name"  binding:"required"`
	FaceIndex int    `json:"face_index"`
}

// Create handles POST /api/characters.
func (h *CharacterHandler) Create(c *gin.Context) {
	accountID := mw.GetAccountID(c)

	var req createCharacterRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// Max characters check (use Find instead of Count: sqlexec Count support is limited)
	var existing []model.Character
	if err := h.db.Select("id").Where("account_id = ?", accountID).Find(&existing).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "internal error"})
		return
	}
	if len(existing) >= maxCharacters {
		c.JSON(http.StatusBadRequest, gin.H{"error": "max characters reached"})
		return
	}

	// Class ID validation
	if h.res != nil {
		if cls := h.res.ClassByID(req.ClassID); cls == nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid class_id"})
			return
		}
		// Walk/face image validation (skip if no img path configured)
		if !h.res.ValidWalkName(req.WalkName) {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid walk_name"})
			return
		}
		if !h.res.ValidFaceName(req.FaceName) {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid face_name"})
			return
		}
	}

	// Use RMMV System.json start position if available; fall back to config.
	startMap, startX, startY := h.game.StartMapID, h.game.StartX, h.game.StartY
	if h.res != nil && h.res.System != nil && h.res.System.StartMapID > 0 {
		startMap = h.res.System.StartMapID
		startX = h.res.System.StartX
		startY = h.res.System.StartY
	}

	char := &model.Character{
		AccountID: accountID,
		Name:      req.Name,
		ClassID:   req.ClassID,
		WalkName:  req.WalkName,
		WalkIndex: req.WalkIndex,
		FaceName:  req.FaceName,
		FaceIndex: req.FaceIndex,
		Level:     1,
		HP:        100, MaxHP: 100,
		MP:        50, MaxMP: 50,
		MapID:     startMap,
		MapX:      startX,
		MapY:      startY,
		Direction: 2,
	}

	if err := h.db.Create(char).Error; err != nil {
		if isUniqueViolation(err) {
			c.JSON(http.StatusBadRequest, gin.H{"error": "character name already taken"})
		} else {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "internal error"})
		}
		return
	}

	// Assign starting skills based on class learnings at level 1.
	if h.res != nil {
		skillIDs := h.res.SkillsForLevel(req.ClassID, 1)
		for _, sid := range skillIDs {
			cs := &model.CharSkill{CharID: char.ID, SkillID: sid, Level: 1}
			h.db.Create(cs)
		}
	}

	c.JSON(http.StatusCreated, char)
}

type deleteCharacterRequest struct {
	Password string `json:"password" binding:"required"`
}

// Delete handles DELETE /api/characters/:id.
func (h *CharacterHandler) Delete(c *gin.Context) {
	accountID := mw.GetAccountID(c)
	charID, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
		return
	}

	var req deleteCharacterRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "password required"})
		return
	}

	// Verify the account password.
	var acc model.Account
	if err := h.db.First(&acc, accountID).Error; err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}
	if err := bcrypt.CompareHashAndPassword([]byte(acc.PasswordHash), []byte(req.Password)); err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "wrong password"})
		return
	}

	// Delete only if the character belongs to this account.
	result := h.db.Where("id = ? AND account_id = ?", charID, accountID).Delete(&model.Character{})
	if result.Error != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "internal error"})
		return
	}
	if result.RowsAffected == 0 {
		c.JSON(http.StatusNotFound, gin.H{"error": "character not found"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "deleted"})
}
