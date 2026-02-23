package item

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/kasuganosora/rpgmakermvmmo/server/game/player"
	"github.com/kasuganosora/rpgmakermvmmo/server/model"
	"go.uber.org/zap"
	"gorm.io/gorm"
)

const maxInventorySlots = 99

// InventoryService handles all bag operations.
type InventoryService struct {
	db     *gorm.DB
	logger *zap.Logger
}

// NewInventoryService creates a new InventoryService.
func NewInventoryService(db *gorm.DB, logger *zap.Logger) *InventoryService {
	return &InventoryService{db: db, logger: logger}
}

// AddItem adds qty of (itemType, itemID) to charID's inventory.
// Consumables stack up to 99 per slot; weapons/armors are always separate rows.
func (svc *InventoryService) AddItem(ctx context.Context, charID int64, itemType, itemID, qty int) error {
	return svc.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if itemType == model.ItemKindItem {
			// Check if there's an existing stackable slot.
			var existing model.Inventory
			err := tx.Where("char_id = ? AND item_id = ? AND kind = ? AND equipped = false", charID, itemID, itemType).
				First(&existing).Error
			if err == nil {
				newQty := existing.Qty + qty
				if newQty > maxInventorySlots {
					return errors.New("inventory full")
				}
				return tx.Model(&existing).Update("qty", newQty).Error
			}
			if !errors.Is(err, gorm.ErrRecordNotFound) {
				return err
			}
		}
		// Check total slot count.
		var count int64
		if err := tx.Model(&model.Inventory{}).Where("char_id = ?", charID).Count(&count).Error; err != nil {
			return err
		}
		if count >= maxInventorySlots {
			return errors.New("inventory full")
		}
		inv := &model.Inventory{CharID: charID, ItemID: itemID, Kind: itemType, Qty: qty}
		return tx.Create(inv).Error
	})
}

// RemoveItem decrements qty from a slot. Deletes the slot if qty hits 0.
func (svc *InventoryService) RemoveItem(ctx context.Context, charID int64, itemType, itemID, qty int) error {
	return svc.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var inv model.Inventory
		if err := tx.Where("char_id = ? AND item_id = ? AND kind = ?", charID, itemID, itemType).
			First(&inv).Error; err != nil {
			return err
		}
		if inv.Qty < qty {
			return errors.New("not enough items")
		}
		if inv.Qty == qty {
			return tx.Delete(&inv).Error
		}
		return tx.Model(&inv).Update("qty", inv.Qty-qty).Error
	})
}

// List returns all inventory rows for charID.
func (svc *InventoryService) List(ctx context.Context, charID int64) ([]model.Inventory, error) {
	var items []model.Inventory
	err := svc.db.WithContext(ctx).Where("char_id = ?", charID).Find(&items).Error
	return items, err
}

// NotifyUpdate sends an inventory_update packet to the player.
func NotifyUpdate(s *player.PlayerSession, added, removed []model.Inventory) {
	payload, _ := json.Marshal(map[string]interface{}{
		"add":    added,
		"remove": removed,
		"update": []interface{}{},
	})
	s.Send(&player.Packet{Type: "inventory_update", Payload: payload})
}
