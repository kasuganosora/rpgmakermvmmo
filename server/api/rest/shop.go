package rest

import (
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	mw "github.com/kasuganosora/rpgmakermvmmo/server/middleware"
	"github.com/kasuganosora/rpgmakermvmmo/server/model"
	"github.com/kasuganosora/rpgmakermvmmo/server/resource"
	"gorm.io/gorm"
)

// ShopItem describes one purchasable entry in a shop.
type ShopItem struct {
	Kind   int `json:"kind"`    // 1=item,2=weapon,3=armor
	ItemID int `json:"item_id"`
	Price  int `json:"price"`
}

// ShopDef defines a shop's inventory.
type ShopDef struct {
	ID    int        `json:"id"`
	Name  string     `json:"name"`
	Items []ShopItem `json:"items"`
}

// ShopHandler handles shop REST endpoints.
type ShopHandler struct {
	db    *gorm.DB
	res   *resource.ResourceLoader
	shops map[int]*ShopDef
}

// NewShopHandler creates a ShopHandler.
// shops is a map of shopID → ShopDef; pass nil to use an empty default.
func NewShopHandler(db *gorm.DB, res *resource.ResourceLoader, shops map[int]*ShopDef) *ShopHandler {
	if shops == nil {
		shops = make(map[int]*ShopDef)
	}
	return &ShopHandler{db: db, res: res, shops: shops}
}

// Detail returns the shop's items with resolved names.
// GET /api/shop/:id
func (h *ShopHandler) Detail(c *gin.Context) {
	shopID, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
		return
	}
	def, ok := h.shops[shopID]
	if !ok {
		c.JSON(http.StatusNotFound, gin.H{"error": "shop not found"})
		return
	}

	type itemEntry struct {
		Kind   int    `json:"kind"`
		ItemID int    `json:"item_id"`
		Name   string `json:"name"`
		Price  int    `json:"price"`
	}
	entries := make([]itemEntry, 0, len(def.Items))
	for _, si := range def.Items {
		name := h.resolveName(si.Kind, si.ItemID)
		price := si.Price
		if price == 0 {
			price = h.resolvePrice(si.Kind, si.ItemID)
		}
		entries = append(entries, itemEntry{Kind: si.Kind, ItemID: si.ItemID, Name: name, Price: price})
	}
	c.JSON(http.StatusOK, gin.H{"shop_id": shopID, "name": def.Name, "items": entries})
}

type buyRequest struct {
	Kind   int `json:"kind"   binding:"required"`
	ItemID int `json:"item_id" binding:"required"`
	Qty    int `json:"qty"`
}

// Buy purchases an item from the shop.
// POST /api/shop/:id/buy
func (h *ShopHandler) Buy(c *gin.Context) {
	accountID := mw.GetAccountID(c)
	shopID, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
		return
	}
	def, ok := h.shops[shopID]
	if !ok {
		c.JSON(http.StatusNotFound, gin.H{"error": "shop not found"})
		return
	}

	var req buyRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if req.Qty <= 0 {
		req.Qty = 1
	}

	// Validate item is in shop.
	price := 0
	found := false
	for _, si := range def.Items {
		if si.Kind == req.Kind && si.ItemID == req.ItemID {
			price = si.Price
			if price == 0 {
				price = h.resolvePrice(si.Kind, si.ItemID)
			}
			found = true
			break
		}
	}
	if !found {
		c.JSON(http.StatusBadRequest, gin.H{"error": "item not available in this shop"})
		return
	}

	// Multiply as int64 first to avoid int overflow on large price × qty.
	total := int64(price) * int64(req.Qty)

	// Look up character ID.
	charID, err := h.getActiveCharID(c, accountID)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "no active character"})
		return
	}

	txErr := h.db.Transaction(func(tx *gorm.DB) error {
		var char model.Character
		if err := tx.Where("id = ?", charID).First(&char).Error; err != nil {
			return err
		}
		if char.Gold < total {
			return errInsufficientGold
		}
		if err := tx.Model(&char).Update("gold", gorm.Expr("gold - ?", total)).Error; err != nil {
			return err
		}
		// Add to inventory (stack if consumable item).
		const maxStackQty = 9999
		var inv model.Inventory
		findErr := tx.Where("char_id = ? AND item_id = ? AND kind = ?", charID, req.ItemID, req.Kind).
			First(&inv).Error
		if findErr != nil {
			inv = model.Inventory{CharID: charID, ItemID: req.ItemID, Kind: req.Kind, Qty: req.Qty}
			return tx.Create(&inv).Error
		}
		newQty := inv.Qty + req.Qty
		if newQty > maxStackQty {
			return errStackFull
		}
		return tx.Model(&inv).Update("qty", newQty).Error
	})
	if txErr != nil {
		if txErr == errInsufficientGold {
			c.JSON(http.StatusPaymentRequired, gin.H{"error": "insufficient gold"})
		} else if txErr == errStackFull {
			c.JSON(http.StatusBadRequest, gin.H{"error": "exceeds maximum stack size"})
		} else {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "transaction failed"})
		}
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true, "spent": total})
}

type sellRequest struct {
	InvID int `json:"inv_id" binding:"required"`
	Qty   int `json:"qty"`
}

// Sell sells an inventory item back to the shop.
// POST /api/shop/:id/sell
func (h *ShopHandler) Sell(c *gin.Context) {
	accountID := mw.GetAccountID(c)
	_, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
		return
	}

	var req sellRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if req.Qty <= 0 {
		req.Qty = 1
	}

	charID, err := h.getActiveCharID(c, accountID)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "no active character"})
		return
	}

	txErr := h.db.Transaction(func(tx *gorm.DB) error {
		var inv model.Inventory
		if err := tx.Where("id = ? AND char_id = ?", req.InvID, charID).First(&inv).Error; err != nil {
			return errItemNotFound
		}
		if inv.Qty < req.Qty {
			req.Qty = inv.Qty
		}
		sellPrice := int64(h.resolvePrice(inv.Kind, inv.ItemID) / 2)
		earned := sellPrice * int64(req.Qty)

		newQty := inv.Qty - req.Qty
		if newQty <= 0 {
			tx.Delete(&inv)
		} else {
			tx.Model(&inv).Update("qty", newQty)
		}
		return tx.Model(&model.Character{}).Where("id = ?", charID).
			Update("gold", gorm.Expr("gold + ?", earned)).Error
	})
	if txErr != nil {
		if txErr == errItemNotFound {
			c.JSON(http.StatusNotFound, gin.H{"error": "item not found"})
		} else {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "transaction failed"})
		}
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

// ---- helpers ----

var (
	errInsufficientGold = &shopError{"insufficient gold"}
	errItemNotFound     = &shopError{"item not found"}
	errStackFull        = &shopError{"exceeds maximum stack size"}
)

type shopError struct{ msg string }

func (e *shopError) Error() string { return e.msg }

func (h *ShopHandler) resolveName(kind, itemID int) string {
	if h.res == nil {
		return ""
	}
	switch kind {
	case model.ItemKindItem:
		for _, it := range h.res.Items {
			if it != nil && it.ID == itemID {
				return it.Name
			}
		}
	case model.ItemKindWeapon:
		for _, w := range h.res.Weapons {
			if w != nil && w.ID == itemID {
				return w.Name
			}
		}
	case model.ItemKindArmor:
		for _, a := range h.res.Armors {
			if a != nil && a.ID == itemID {
				return a.Name
			}
		}
	}
	return ""
}

func (h *ShopHandler) resolvePrice(kind, itemID int) int {
	if h.res == nil {
		return 0
	}
	switch kind {
	case model.ItemKindItem:
		for _, it := range h.res.Items {
			if it != nil && it.ID == itemID {
				return it.Price
			}
		}
	case model.ItemKindWeapon:
		for _, w := range h.res.Weapons {
			if w != nil && w.ID == itemID {
				return w.Price
			}
		}
	case model.ItemKindArmor:
		for _, a := range h.res.Armors {
			if a != nil && a.ID == itemID {
				return a.Price
			}
		}
	}
	return 0
}

func (h *ShopHandler) getActiveCharID(c *gin.Context, accountID int64) (int64, error) {
	// Use char_id query param; for a full implementation this would come from the session.
	if cidStr := c.Query("char_id"); cidStr != "" {
		return strconv.ParseInt(cidStr, 10, 64)
	}
	// Fall back: first character of the account.
	var char model.Character
	if err := h.db.Where("account_id = ?", accountID).First(&char).Error; err != nil {
		return 0, err
	}
	return char.ID, nil
}
