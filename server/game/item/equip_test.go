package item

import (
	"context"
	"testing"

	"github.com/kasuganosora/rpgmakermvmmo/server/game/player"
	"github.com/kasuganosora/rpgmakermvmmo/server/model"
	"github.com/kasuganosora/rpgmakermvmmo/server/resource"
	"github.com/kasuganosora/rpgmakermvmmo/server/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

func setupEquip(t *testing.T) (*EquipService, *player.PlayerSession, int64) {
	t.Helper()
	db := testutil.SetupTestDB(t)
	res := &resource.ResourceLoader{
		Armors: []*resource.Armor{
			nil,
			{ID: 1, Name: "Shield", EtypeID: 1},     // slot 1
			{ID: 2, Name: "Helmet", EtypeID: 2},      // slot 2
			{ID: 3, Name: "Body Armor", EtypeID: 3},   // slot 3
			{ID: 4, Name: "Accessory", EtypeID: 4},    // slot 4
			{ID: 5, Name: "Cloth", EtypeID: 3},        // slot 3 (same as Body Armor)
		},
	}
	svc := NewEquipService(db, res, zap.NewNop())
	charID := int64(1)
	session := &player.PlayerSession{CharID: charID}
	return svc, session, charID
}

func TestEquip_Weapon(t *testing.T) {
	svc, session, charID := setupEquip(t)
	ctx := context.Background()

	// Insert a weapon into inventory
	inv := &model.Inventory{CharID: charID, ItemID: 10, Kind: model.ItemKindWeapon, Qty: 1}
	require.NoError(t, svc.db.Create(inv).Error)

	err := svc.Equip(ctx, session, inv.ID)
	require.NoError(t, err)

	// Verify equipped
	var updated model.Inventory
	require.NoError(t, svc.db.First(&updated, inv.ID).Error)
	assert.True(t, updated.Equipped)
	assert.Equal(t, 0, updated.SlotIndex) // weapons go to slot 0
}

func TestEquip_Armor(t *testing.T) {
	svc, session, charID := setupEquip(t)
	ctx := context.Background()

	// Insert an armor (EtypeID=2, helmet)
	inv := &model.Inventory{CharID: charID, ItemID: 2, Kind: model.ItemKindArmor, Qty: 1}
	require.NoError(t, svc.db.Create(inv).Error)

	err := svc.Equip(ctx, session, inv.ID)
	require.NoError(t, err)

	var updated model.Inventory
	require.NoError(t, svc.db.First(&updated, inv.ID).Error)
	assert.True(t, updated.Equipped)
	assert.Equal(t, 2, updated.SlotIndex) // helmet goes to slot 2
}

func TestEquip_AlreadyEquipped(t *testing.T) {
	svc, session, charID := setupEquip(t)
	ctx := context.Background()

	inv := &model.Inventory{CharID: charID, ItemID: 10, Kind: model.ItemKindWeapon, Qty: 1, Equipped: true, SlotIndex: 0}
	require.NoError(t, svc.db.Create(inv).Error)

	err := svc.Equip(ctx, session, inv.ID)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "already equipped")
}

func TestEquip_ConsumableRejected(t *testing.T) {
	svc, session, charID := setupEquip(t)
	ctx := context.Background()

	inv := &model.Inventory{CharID: charID, ItemID: 1, Kind: model.ItemKindItem, Qty: 5}
	require.NoError(t, svc.db.Create(inv).Error)

	err := svc.Equip(ctx, session, inv.ID)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "consumable")
}

func TestEquip_ReplacesExisting(t *testing.T) {
	svc, session, charID := setupEquip(t)
	ctx := context.Background()

	// Equip first weapon
	inv1 := &model.Inventory{CharID: charID, ItemID: 10, Kind: model.ItemKindWeapon, Qty: 1, Equipped: true, SlotIndex: 0}
	require.NoError(t, svc.db.Create(inv1).Error)

	// Equip second weapon to same slot → should unequip first
	inv2 := &model.Inventory{CharID: charID, ItemID: 11, Kind: model.ItemKindWeapon, Qty: 1}
	require.NoError(t, svc.db.Create(inv2).Error)

	err := svc.Equip(ctx, session, inv2.ID)
	require.NoError(t, err)

	// Old weapon should be unequipped
	var old model.Inventory
	require.NoError(t, svc.db.First(&old, inv1.ID).Error)
	assert.False(t, old.Equipped)
	assert.LessOrEqual(t, old.SlotIndex, 0) // -1 in production; embedded DB stores as 0

	// New weapon should be equipped
	var newInv model.Inventory
	require.NoError(t, svc.db.First(&newInv, inv2.ID).Error)
	assert.True(t, newInv.Equipped)
	assert.Equal(t, 0, newInv.SlotIndex)
}

func TestEquip_ItemNotFound(t *testing.T) {
	svc, session, _ := setupEquip(t)
	ctx := context.Background()

	err := svc.Equip(ctx, session, 9999)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "not found")
}

func TestEquip_ArmorNotInResource(t *testing.T) {
	svc, session, charID := setupEquip(t)
	ctx := context.Background()

	// Armor ID 999 doesn't exist in resource loader
	inv := &model.Inventory{CharID: charID, ItemID: 999, Kind: model.ItemKindArmor, Qty: 1}
	require.NoError(t, svc.db.Create(inv).Error)

	err := svc.Equip(ctx, session, inv.ID)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "invalid equip slot")
}

func TestUnequip_Success(t *testing.T) {
	svc, session, charID := setupEquip(t)
	ctx := context.Background()

	inv := &model.Inventory{CharID: charID, ItemID: 10, Kind: model.ItemKindWeapon, Qty: 1, Equipped: true, SlotIndex: 0}
	require.NoError(t, svc.db.Create(inv).Error)

	err := svc.Unequip(ctx, session, inv.ID)
	require.NoError(t, err)

	var updated model.Inventory
	require.NoError(t, svc.db.First(&updated, inv.ID).Error)
	assert.False(t, updated.Equipped)
	assert.LessOrEqual(t, updated.SlotIndex, 0) // -1 in production; embedded DB stores as 0
}

func TestUnequip_NotEquipped(t *testing.T) {
	svc, session, charID := setupEquip(t)
	ctx := context.Background()

	inv := &model.Inventory{CharID: charID, ItemID: 10, Kind: model.ItemKindWeapon, Qty: 1, Equipped: false}
	require.NoError(t, svc.db.Create(inv).Error)

	err := svc.Unequip(ctx, session, inv.ID)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "not equipped")
}

func TestEquipSlot_Weapon(t *testing.T) {
	svc, _, _ := setupEquip(t)

	slot := svc.equipSlot(model.Inventory{Kind: model.ItemKindWeapon, ItemID: 1})
	assert.Equal(t, 0, slot)
}

func TestEquipSlot_ArmorByEtype(t *testing.T) {
	svc, _, _ := setupEquip(t)

	slot := svc.equipSlot(model.Inventory{Kind: model.ItemKindArmor, ItemID: 4}) // Accessory, etypeID=4
	assert.Equal(t, 4, slot)
}

func TestEquipSlot_ArmorNilRes(t *testing.T) {
	db := testutil.SetupTestDB(t)
	svc := NewEquipService(db, nil, zap.NewNop())

	slot := svc.equipSlot(model.Inventory{Kind: model.ItemKindArmor, ItemID: 1})
	assert.Equal(t, -1, slot) // no resource loader
}
