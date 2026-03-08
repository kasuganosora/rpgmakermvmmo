package npc

import (
	"context"
	"testing"

	"github.com/kasuganosora/rpgmakermvmmo/server/model"
	"github.com/kasuganosora/rpgmakermvmmo/server/resource"
	"github.com/kasuganosora/rpgmakermvmmo/server/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func setupGormStore(t *testing.T) (*gormInventoryStore, func()) {
	t.Helper()
	db := testutil.SetupTestDB(t)

	// Create a test character
	char := model.Character{Name: "Test", AccountID: 1, Gold: 1000}
	require.NoError(t, db.Create(&char).Error)

	return &gormInventoryStore{db: db}, func() {}
}

func TestGormStore_GetGold(t *testing.T) {
	store, cleanup := setupGormStore(t)
	defer cleanup()

	gold, err := store.GetGold(context.Background(), 1)
	require.NoError(t, err)
	assert.Equal(t, int64(1000), gold)

	// Non-existent char
	_, err = store.GetGold(context.Background(), 999)
	assert.Error(t, err)
}

func TestGormStore_UpdateGold(t *testing.T) {
	store, cleanup := setupGormStore(t)
	defer cleanup()

	err := store.UpdateGold(context.Background(), 1, 500)
	require.NoError(t, err)

	gold, _ := store.GetGold(context.Background(), 1)
	assert.Equal(t, int64(1500), gold)

	// Decrease
	err = store.UpdateGold(context.Background(), 1, -300)
	require.NoError(t, err)
	gold, _ = store.GetGold(context.Background(), 1)
	assert.Equal(t, int64(1200), gold)
}

func TestGormStore_AddGetRemoveItem(t *testing.T) {
	store, cleanup := setupGormStore(t)
	defer cleanup()
	ctx := context.Background()

	// Get non-existent item
	qty, err := store.GetItem(ctx, 1, 10)
	assert.Error(t, err)
	assert.Equal(t, 0, qty)

	// Add item
	err = store.AddItem(ctx, 1, 10, 5)
	require.NoError(t, err)

	qty, err = store.GetItem(ctx, 1, 10)
	require.NoError(t, err)
	assert.Equal(t, 5, qty)

	// Add more
	err = store.AddItem(ctx, 1, 10, 3)
	require.NoError(t, err)

	qty, err = store.GetItem(ctx, 1, 10)
	require.NoError(t, err)
	assert.Equal(t, 8, qty)

	// Remove partial
	err = store.RemoveItem(ctx, 1, 10, 3)
	require.NoError(t, err)

	qty, err = store.GetItem(ctx, 1, 10)
	require.NoError(t, err)
	assert.Equal(t, 5, qty)

	// Remove all (should delete record)
	err = store.RemoveItem(ctx, 1, 10, 5)
	require.NoError(t, err)

	_, err = store.GetItem(ctx, 1, 10)
	assert.Error(t, err) // record deleted

	// Remove non-existent
	err = store.RemoveItem(ctx, 1, 99, 1)
	assert.Error(t, err)
}

func TestGormStore_HasItemOfKind(t *testing.T) {
	store, cleanup := setupGormStore(t)
	defer cleanup()
	ctx := context.Background()

	// Create inventory item
	inv := model.Inventory{CharID: 1, ItemID: 10, Kind: model.ItemKindWeapon, Qty: 1, Equipped: false}
	require.NoError(t, store.db.Create(&inv).Error)

	// Has item, not including equipped
	has, err := store.HasItemOfKind(ctx, 1, 10, model.ItemKindWeapon, false)
	require.NoError(t, err)
	assert.True(t, has)

	// Has item, including equipped
	has, err = store.HasItemOfKind(ctx, 1, 10, model.ItemKindWeapon, true)
	require.NoError(t, err)
	assert.True(t, has)

	// Non-existent
	has, err = store.HasItemOfKind(ctx, 1, 99, model.ItemKindWeapon, true)
	require.NoError(t, err)
	assert.False(t, has)
}

func TestGormStore_IsEquipped(t *testing.T) {
	store, cleanup := setupGormStore(t)
	defer cleanup()
	ctx := context.Background()

	// Not equipped
	has, err := store.IsEquipped(ctx, 1, 10, model.ItemKindArmor)
	require.NoError(t, err)
	assert.False(t, has)

	// Create equipped item
	inv := model.Inventory{CharID: 1, ItemID: 10, Kind: model.ItemKindArmor, Qty: 1, Equipped: true, SlotIndex: 1}
	require.NoError(t, store.db.Create(&inv).Error)

	has, err = store.IsEquipped(ctx, 1, 10, model.ItemKindArmor)
	require.NoError(t, err)
	assert.True(t, has)
}

func TestGormStore_HasSkill(t *testing.T) {
	store, cleanup := setupGormStore(t)
	defer cleanup()
	ctx := context.Background()

	// No skill
	has, err := store.HasSkill(ctx, 1, 808)
	require.NoError(t, err)
	assert.False(t, has)

	// Add skill
	skill := model.CharSkill{CharID: 1, SkillID: 808}
	require.NoError(t, store.db.Create(&skill).Error)

	has, err = store.HasSkill(ctx, 1, 808)
	require.NoError(t, err)
	assert.True(t, has)
}

func TestGormStore_SetEquipSlot(t *testing.T) {
	store, cleanup := setupGormStore(t)
	defer cleanup()
	ctx := context.Background()

	// Equip item (no existing in inventory → creates new)
	err := store.SetEquipSlot(ctx, 1, 1, 55, model.ItemKindArmor)
	require.NoError(t, err)

	// Verify equipped
	has, err := store.IsEquipped(ctx, 1, 55, model.ItemKindArmor)
	require.NoError(t, err)
	assert.True(t, has)

	// Unequip slot (itemID=0)
	err = store.SetEquipSlot(ctx, 1, 1, 0, model.ItemKindArmor)
	require.NoError(t, err)

	// Equip a new item in the same slot (existing item in inventory)
	inv := model.Inventory{CharID: 1, ItemID: 60, Kind: model.ItemKindArmor, Qty: 1, Equipped: false}
	require.NoError(t, store.db.Create(&inv).Error)
	err = store.SetEquipSlot(ctx, 1, 1, 60, model.ItemKindArmor)
	require.NoError(t, err)
}

func TestGormStore_AddRemoveArmorOrWeapon(t *testing.T) {
	store, cleanup := setupGormStore(t)
	defer cleanup()
	ctx := context.Background()

	// Add weapon
	err := store.AddArmorOrWeapon(ctx, 1, 5, model.ItemKindWeapon, 2)
	require.NoError(t, err)

	// Add more
	err = store.AddArmorOrWeapon(ctx, 1, 5, model.ItemKindWeapon, 3)
	require.NoError(t, err)

	// Verify qty
	var inv model.Inventory
	require.NoError(t, store.db.Where("char_id = ? AND item_id = ? AND kind = ?", 1, 5, model.ItemKindWeapon).First(&inv).Error)
	assert.Equal(t, 5, inv.Qty)

	// Remove partial
	err = store.RemoveArmorOrWeapon(ctx, 1, 5, model.ItemKindWeapon, 3)
	require.NoError(t, err)

	require.NoError(t, store.db.Where("char_id = ? AND item_id = ? AND kind = ?", 1, 5, model.ItemKindWeapon).First(&inv).Error)
	assert.Equal(t, 2, inv.Qty)

	// Remove all
	err = store.RemoveArmorOrWeapon(ctx, 1, 5, model.ItemKindWeapon, 2)
	require.NoError(t, err)

	// Remove non-existent
	err = store.RemoveArmorOrWeapon(ctx, 1, 99, model.ItemKindWeapon, 1)
	assert.Error(t, err)
}

func TestNewWithDB(t *testing.T) {
	db := testutil.SetupTestDB(t)
	exec := NewWithDB(db, &resource.ResourceLoader{}, nopLogger())
	assert.NotNil(t, exec)
	assert.NotNil(t, exec.store)
}
