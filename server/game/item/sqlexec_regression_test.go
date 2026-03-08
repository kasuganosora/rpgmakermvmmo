package item

import (
	"testing"

	"github.com/kasuganosora/rpgmakermvmmo/server/model"
	"github.com/kasuganosora/rpgmakermvmmo/server/testutil"
	"gorm.io/gorm"
)

// TestSqlexec_BooleanWhere tests that boolean WHERE clauses work correctly.
// Previously, `WHERE equipped = false` and `WHERE equipped = ?` with false
// failed to match any rows in sqlexec.
func TestSqlexec_BooleanWhere(t *testing.T) {
	db := testutil.SetupTestDB(t)

	// Create two rows: one equipped, one not.
	inv1 := model.Inventory{CharID: 1, ItemID: 10, Kind: model.ItemKindWeapon, Qty: 1, Equipped: true}
	inv2 := model.Inventory{CharID: 1, ItemID: 20, Kind: model.ItemKindWeapon, Qty: 1, Equipped: false}
	if err := db.Create(&inv1).Error; err != nil {
		t.Fatalf("create inv1: %v", err)
	}
	if err := db.Create(&inv2).Error; err != nil {
		t.Fatalf("create inv2: %v", err)
	}

	// Test 1: WHERE equipped = ? with false — should find inv2
	var unequipped []model.Inventory
	if err := db.Where("equipped = ?", false).Find(&unequipped).Error; err != nil {
		t.Fatalf("query equipped=false: %v", err)
	}
	if len(unequipped) != 1 {
		t.Errorf("expected 1 unequipped row, got %d", len(unequipped))
	} else if unequipped[0].ItemID != 20 {
		t.Errorf("expected item_id=20, got %d", unequipped[0].ItemID)
	}

	// Test 2: WHERE equipped = ? with true — should find inv1
	var equipped []model.Inventory
	if err := db.Where("equipped = ?", true).Find(&equipped).Error; err != nil {
		t.Fatalf("query equipped=true: %v", err)
	}
	if len(equipped) != 1 {
		t.Errorf("expected 1 equipped row, got %d", len(equipped))
	} else if equipped[0].ItemID != 10 {
		t.Errorf("expected item_id=10, got %d", equipped[0].ItemID)
	}

	t.Logf("Boolean WHERE: unequipped=%d rows, equipped=%d rows", len(unequipped), len(equipped))
}

// TestSqlexec_NegativeInteger tests that negative integers are stored and
// retrieved correctly. Previously, `slot_index = -1` was stored as 0.
func TestSqlexec_NegativeInteger(t *testing.T) {
	db := testutil.SetupTestDB(t)

	inv := model.Inventory{CharID: 1, ItemID: 10, Kind: model.ItemKindWeapon, Qty: 1, SlotIndex: -1}
	if err := db.Create(&inv).Error; err != nil {
		t.Fatalf("create: %v", err)
	}

	// Read back and verify.
	var result model.Inventory
	if err := db.Where("id = ?", inv.ID).First(&result).Error; err != nil {
		t.Fatalf("read back: %v", err)
	}
	if result.SlotIndex != -1 {
		t.Errorf("expected slot_index=-1, got %d", result.SlotIndex)
	}

	// Query by negative value.
	var bySlot []model.Inventory
	if err := db.Where("slot_index = ?", -1).Find(&bySlot).Error; err != nil {
		t.Fatalf("query slot_index=-1: %v", err)
	}
	if len(bySlot) != 1 {
		t.Errorf("expected 1 row with slot_index=-1, got %d", len(bySlot))
	}

	t.Logf("Negative integer: stored=%d, query_match=%d rows", result.SlotIndex, len(bySlot))
}

// TestSqlexec_SaveBoolFalse tests that Save() correctly persists a boolean
// field set to false. Previously, Save() with Equipped=false didn't persist.
func TestSqlexec_SaveBoolFalse(t *testing.T) {
	db := testutil.SetupTestDB(t)

	inv := model.Inventory{CharID: 1, ItemID: 10, Kind: model.ItemKindWeapon, Qty: 1, Equipped: true, SlotIndex: 3}
	if err := db.Create(&inv).Error; err != nil {
		t.Fatalf("create: %v", err)
	}

	// Unequip via Save()
	inv.Equipped = false
	inv.SlotIndex = -1
	if err := db.Save(&inv).Error; err != nil {
		t.Fatalf("save: %v", err)
	}

	// Read back
	var result model.Inventory
	if err := db.Where("id = ?", inv.ID).First(&result).Error; err != nil {
		t.Fatalf("read back: %v", err)
	}
	if result.Equipped {
		t.Errorf("expected equipped=false after Save, got true")
	}
	if result.SlotIndex != -1 {
		t.Errorf("expected slot_index=-1 after Save, got %d", result.SlotIndex)
	}

	t.Logf("Save bool false: equipped=%v, slot_index=%d", result.Equipped, result.SlotIndex)
}

// TestSqlexec_GormExpr tests that gorm.Expr works for arithmetic updates.
// This is relevant for shop buy/sell where gold is deducted/added.
func TestSqlexec_GormExpr(t *testing.T) {
	db := testutil.SetupTestDB(t)

	char := model.Character{AccountID: 1, Name: "test", ClassID: 1, HP: 100, MaxHP: 100, Gold: 1000}
	if err := db.Create(&char).Error; err != nil {
		t.Fatalf("create: %v", err)
	}

	// Test gorm.Expr for arithmetic
	if err := db.Model(&char).Update("gold", gorm.Expr("gold - ?", 300)).Error; err != nil {
		t.Fatalf("gorm.Expr update: %v", err)
	}

	var result model.Character
	if err := db.Where("id = ?", char.ID).First(&result).Error; err != nil {
		t.Fatalf("read back: %v", err)
	}
	if result.Gold != 700 {
		t.Errorf("expected gold=700 after Expr deduction, got %d", result.Gold)
	}

	t.Logf("gorm.Expr: gold after deducting 300 = %d", result.Gold)
}
