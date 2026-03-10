package item

import (
	"context"
	"testing"

	"github.com/kasuganosora/rpgmakermvmmo/server/model"
	"github.com/kasuganosora/rpgmakermvmmo/server/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

func setupInventory(t *testing.T) (*InventoryService, int64) {
	t.Helper()
	db := testutil.SetupTestDB(t)
	svc := NewInventoryService(db, zap.NewNop())
	// Create a character manually (we only need char_id for inventory).
	charID := int64(1)
	return svc, charID
}

// ═══════════════════════════════════════════════════════════════════════════
//  AddItem tests
// ═══════════════════════════════════════════════════════════════════════════

func TestAddItem_Consumable_NewSlot(t *testing.T) {
	svc, charID := setupInventory(t)
	ctx := context.Background()

	err := svc.AddItem(ctx, charID, model.ItemKindItem, 1, 5)
	require.NoError(t, err)

	items, err := svc.List(ctx, charID)
	require.NoError(t, err)
	require.Len(t, items, 1)
	assert.Equal(t, 1, items[0].ItemID)
	assert.Equal(t, 5, items[0].Qty)
}

func TestAddItem_Consumable_StackOnExisting(t *testing.T) {
	svc, charID := setupInventory(t)
	ctx := context.Background()

	require.NoError(t, svc.AddItem(ctx, charID, model.ItemKindItem, 1, 5))
	require.NoError(t, svc.AddItem(ctx, charID, model.ItemKindItem, 1, 10))

	items, err := svc.List(ctx, charID)
	require.NoError(t, err)
	require.Len(t, items, 1) // stacked into same slot
	assert.Equal(t, 15, items[0].Qty)
}

func TestAddItem_Consumable_ExceedsMaxStack(t *testing.T) {
	svc, charID := setupInventory(t)
	ctx := context.Background()

	require.NoError(t, svc.AddItem(ctx, charID, model.ItemKindItem, 1, 9990))
	err := svc.AddItem(ctx, charID, model.ItemKindItem, 1, 20) // 9990+20=10010 > 9999
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "exceeds maximum stack size")
}

func TestAddItem_Consumable_ExactMaxStack(t *testing.T) {
	svc, charID := setupInventory(t)
	ctx := context.Background()

	require.NoError(t, svc.AddItem(ctx, charID, model.ItemKindItem, 1, 5000))
	err := svc.AddItem(ctx, charID, model.ItemKindItem, 1, 4999) // 5000+4999=9999, exactly at limit
	require.NoError(t, err)

	items, _ := svc.List(ctx, charID)
	assert.Equal(t, 9999, items[0].Qty)
}

func TestAddItem_Weapon_SeparateRows(t *testing.T) {
	svc, charID := setupInventory(t)
	ctx := context.Background()

	require.NoError(t, svc.AddItem(ctx, charID, model.ItemKindWeapon, 5, 1))
	require.NoError(t, svc.AddItem(ctx, charID, model.ItemKindWeapon, 5, 1))

	items, err := svc.List(ctx, charID)
	require.NoError(t, err)
	assert.Len(t, items, 2) // weapons don't stack
}

func TestAddItem_Armor_SeparateRows(t *testing.T) {
	svc, charID := setupInventory(t)
	ctx := context.Background()

	require.NoError(t, svc.AddItem(ctx, charID, model.ItemKindArmor, 10, 1))
	require.NoError(t, svc.AddItem(ctx, charID, model.ItemKindArmor, 10, 1))

	items, err := svc.List(ctx, charID)
	require.NoError(t, err)
	assert.Len(t, items, 2) // armors don't stack
}

func TestAddItem_SlotLimitReached(t *testing.T) {
	svc, charID := setupInventory(t)
	ctx := context.Background()

	// Fill up 99 slots with unique weapons
	for i := 1; i <= 99; i++ {
		require.NoError(t, svc.AddItem(ctx, charID, model.ItemKindWeapon, i, 1))
	}

	// 100th item should fail
	err := svc.AddItem(ctx, charID, model.ItemKindWeapon, 100, 1)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "inventory full")
}

func TestAddItem_ConsumableStackBypassesSlotLimit(t *testing.T) {
	svc, charID := setupInventory(t)
	ctx := context.Background()

	// Add one consumable
	require.NoError(t, svc.AddItem(ctx, charID, model.ItemKindItem, 1, 1))

	// Fill remaining 98 slots
	for i := 2; i <= 99; i++ {
		require.NoError(t, svc.AddItem(ctx, charID, model.ItemKindWeapon, i, 1))
	}

	// Adding to existing consumable stack should still work (no new slot needed)
	err := svc.AddItem(ctx, charID, model.ItemKindItem, 1, 5)
	require.NoError(t, err)

	items, _ := svc.List(ctx, charID)
	found := false
	for _, item := range items {
		if item.ItemID == 1 && item.Kind == model.ItemKindItem {
			assert.Equal(t, 6, item.Qty)
			found = true
		}
	}
	assert.True(t, found)
}

// ═══════════════════════════════════════════════════════════════════════════
//  RemoveItem tests
// ═══════════════════════════════════════════════════════════════════════════

func TestRemoveItem_PartialQuantity(t *testing.T) {
	svc, charID := setupInventory(t)
	ctx := context.Background()

	require.NoError(t, svc.AddItem(ctx, charID, model.ItemKindItem, 1, 10))
	require.NoError(t, svc.RemoveItem(ctx, charID, model.ItemKindItem, 1, 3))

	items, _ := svc.List(ctx, charID)
	require.Len(t, items, 1)
	assert.Equal(t, 7, items[0].Qty)
}

func TestRemoveItem_ExactQuantity(t *testing.T) {
	svc, charID := setupInventory(t)
	ctx := context.Background()

	require.NoError(t, svc.AddItem(ctx, charID, model.ItemKindItem, 1, 5))
	require.NoError(t, svc.RemoveItem(ctx, charID, model.ItemKindItem, 1, 5))

	items, _ := svc.List(ctx, charID)
	assert.Len(t, items, 0) // deleted when qty reaches 0
}

func TestRemoveItem_InsufficientQuantity(t *testing.T) {
	svc, charID := setupInventory(t)
	ctx := context.Background()

	require.NoError(t, svc.AddItem(ctx, charID, model.ItemKindItem, 1, 3))
	err := svc.RemoveItem(ctx, charID, model.ItemKindItem, 1, 10)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "not enough items")
}

func TestRemoveItem_NotFound(t *testing.T) {
	svc, charID := setupInventory(t)
	ctx := context.Background()

	err := svc.RemoveItem(ctx, charID, model.ItemKindItem, 999, 1)
	assert.Error(t, err) // record not found
}

// ═══════════════════════════════════════════════════════════════════════════
//  UseItem tests
// ═══════════════════════════════════════════════════════════════════════════

func TestUseItem_Consumable(t *testing.T) {
	svc, charID := setupInventory(t)
	ctx := context.Background()

	require.NoError(t, svc.AddItem(ctx, charID, model.ItemKindItem, 1, 3))
	items, _ := svc.List(ctx, charID)
	invID := items[0].ID

	inv, err := svc.UseItem(ctx, charID, invID)
	require.NoError(t, err)
	assert.Equal(t, 1, inv.ItemID)
	assert.Equal(t, model.ItemKindItem, inv.Kind)

	// qty should decrease to 2
	items, _ = svc.List(ctx, charID)
	require.Len(t, items, 1)
	assert.Equal(t, 2, items[0].Qty)
}

func TestUseItem_LastOne_DeletesRow(t *testing.T) {
	svc, charID := setupInventory(t)
	ctx := context.Background()

	require.NoError(t, svc.AddItem(ctx, charID, model.ItemKindItem, 1, 1))
	items, _ := svc.List(ctx, charID)
	invID := items[0].ID

	_, err := svc.UseItem(ctx, charID, invID)
	require.NoError(t, err)

	items, _ = svc.List(ctx, charID)
	assert.Len(t, items, 0) // deleted
}

func TestUseItem_NonConsumable_Rejected(t *testing.T) {
	svc, charID := setupInventory(t)
	ctx := context.Background()

	require.NoError(t, svc.AddItem(ctx, charID, model.ItemKindWeapon, 5, 1))
	items, _ := svc.List(ctx, charID)
	invID := items[0].ID

	_, err := svc.UseItem(ctx, charID, invID)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "only consumable items can be used")
}

func TestUseItem_NotFound(t *testing.T) {
	svc, charID := setupInventory(t)
	ctx := context.Background()

	_, err := svc.UseItem(ctx, charID, 9999)
	assert.Error(t, err)
}

func TestUseItem_WrongCharacter(t *testing.T) {
	svc, charID := setupInventory(t)
	ctx := context.Background()

	require.NoError(t, svc.AddItem(ctx, charID, model.ItemKindItem, 1, 3))
	items, _ := svc.List(ctx, charID)
	invID := items[0].ID

	// Try using with wrong charID
	_, err := svc.UseItem(ctx, 9999, invID)
	assert.Error(t, err)
}

// ═══════════════════════════════════════════════════════════════════════════
//  List tests
// ═══════════════════════════════════════════════════════════════════════════

func TestList_Empty(t *testing.T) {
	svc, charID := setupInventory(t)
	ctx := context.Background()

	items, err := svc.List(ctx, charID)
	require.NoError(t, err)
	assert.Len(t, items, 0)
}

func TestList_MultipleItems(t *testing.T) {
	svc, charID := setupInventory(t)
	ctx := context.Background()

	require.NoError(t, svc.AddItem(ctx, charID, model.ItemKindItem, 1, 5))
	require.NoError(t, svc.AddItem(ctx, charID, model.ItemKindWeapon, 10, 1))
	require.NoError(t, svc.AddItem(ctx, charID, model.ItemKindArmor, 20, 1))

	items, err := svc.List(ctx, charID)
	require.NoError(t, err)
	assert.Len(t, items, 3)
}

// ═══════════════════════════════════════════════════════════════════════════
//  Edge cases
// ═══════════════════════════════════════════════════════════════════════════

func TestAddItem_DifferentConsumables_DifferentSlots(t *testing.T) {
	svc, charID := setupInventory(t)
	ctx := context.Background()

	require.NoError(t, svc.AddItem(ctx, charID, model.ItemKindItem, 1, 5))
	require.NoError(t, svc.AddItem(ctx, charID, model.ItemKindItem, 2, 3))

	items, _ := svc.List(ctx, charID)
	assert.Len(t, items, 2) // different items, different slots
}
