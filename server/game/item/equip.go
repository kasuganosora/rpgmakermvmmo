package item

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/kasuganosora/rpgmakermvmmo/server/game/player"
	"github.com/kasuganosora/rpgmakermvmmo/server/model"
	"github.com/kasuganosora/rpgmakermvmmo/server/resource"
	"go.uber.org/zap"
	"gorm.io/gorm"
)

// EquipService handles equip and unequip operations.
type EquipService struct {
	db     *gorm.DB
	res    *resource.ResourceLoader
	logger *zap.Logger
}

// NewEquipService creates a new EquipService.
func NewEquipService(db *gorm.DB, res *resource.ResourceLoader, logger *zap.Logger) *EquipService {
	return &EquipService{db: db, res: res, logger: logger}
}

// Equip equips the inventory item with invID for session s.
func (svc *EquipService) Equip(ctx context.Context, s *player.PlayerSession, invID int64) error {
	return svc.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var inv model.Inventory
		if err := tx.Where("id = ? AND char_id = ?", invID, s.CharID).First(&inv).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return errors.New("item not found in inventory")
			}
			return err
		}
		if inv.Equipped {
			return errors.New("item already equipped")
		}
		if inv.Kind == model.ItemKindItem {
			return errors.New("consumable items cannot be equipped")
		}

		// Determine equip slot.
		slot := svc.equipSlot(inv)
		if slot < 0 {
			return errors.New("invalid equip slot")
		}

		// Unequip any existing item in the same slot.
		// Query all char items and filter in Go — the embedded DB engine
		// doesn't reliably handle boolean or negative-value WHERE clauses.
		var charItems []model.Inventory
		if err := tx.Where("char_id = ?", s.CharID).Find(&charItems).Error; err != nil {
			return err
		}
		for _, ci := range charItems {
			if ci.ID != invID && ci.Equipped && ci.SlotIndex == slot {
				ci.Equipped = false
				ci.SlotIndex = -1
				if err2 := tx.Save(&ci).Error; err2 != nil {
					return err2
				}
				break
			}
		}

		inv.Equipped = true
		inv.SlotIndex = slot
		return tx.Save(&inv).Error
	})
}

// Unequip removes the equipped flag from inventory item invID.
func (svc *EquipService) Unequip(ctx context.Context, s *player.PlayerSession, invID int64) error {
	return svc.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var inv model.Inventory
		if err := tx.Where("id = ? AND char_id = ?", invID, s.CharID).
			First(&inv).Error; err != nil {
			return errors.New("item not equipped")
		}
		if !inv.Equipped {
			return errors.New("item not equipped")
		}
		return tx.Exec("UPDATE inventories SET equipped = 0, slot_index = -1 WHERE id = ?",
			inv.ID).Error
	})
}

// equipSlot determines the slot index for an inventory item.
// Weapons go to slot 0, armors go to slots based on etypeId (1-4).
func (svc *EquipService) equipSlot(inv model.Inventory) int {
	if inv.Kind == model.ItemKindWeapon {
		return 0
	}
	if inv.Kind == model.ItemKindArmor && svc.res != nil {
		for _, a := range svc.res.Armors {
			if a != nil && a.ID == inv.ItemID {
				return a.EtypeID // 1=shield,2=helmet,3=body,4=accessory
			}
		}
	}
	return -1
}

// BroadcastEquipResult sends the updated stats to the player.
func BroadcastEquipResult(s *player.PlayerSession, success bool, message string) {
	payload, _ := json.Marshal(map[string]interface{}{
		"success": success,
		"message": message,
	})
	s.Send(&player.Packet{Type: "equip_result", Payload: payload})
}
