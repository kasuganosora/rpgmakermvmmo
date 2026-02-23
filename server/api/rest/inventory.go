package rest

import (
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	mw "github.com/kasuganosora/rpgmakermvmmo/server/middleware"
	"github.com/kasuganosora/rpgmakermvmmo/server/model"
	"gorm.io/gorm"
)

// InventoryHandler handles inventory REST endpoints.
type InventoryHandler struct {
	db *gorm.DB
}

// NewInventoryHandler creates a new InventoryHandler.
func NewInventoryHandler(db *gorm.DB) *InventoryHandler {
	return &InventoryHandler{db: db}
}

// List handles GET /api/characters/:id/inventory.
func (h *InventoryHandler) List(c *gin.Context) {
	accountID := mw.GetAccountID(c)
	charIDStr := c.Param("id")
	charID, err := strconv.ParseInt(charIDStr, 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
		return
	}

	// Verify character belongs to account.
	var char model.Character
	if err := h.db.Where("id = ? AND account_id = ?", charID, accountID).First(&char).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "character not found"})
		return
	}

	var items []model.Inventory
	if err := h.db.Where("char_id = ?", charID).Find(&items).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "internal error"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"inventory": items})
}
