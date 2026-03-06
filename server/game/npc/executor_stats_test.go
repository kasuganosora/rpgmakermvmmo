package npc

import (
	"context"
	"testing"

	"github.com/kasuganosora/rpgmakermvmmo/server/resource"
	"github.com/stretchr/testify/assert"
)

// makeClassParams builds a Params matrix for a class.
// RMMV format: Params[paramIdx][level], index 0 = unused, index 1 = level 1.
func makeClassParams(mhp, mmp int) [][]int {
	params := make([][]int, 8)
	for i := range params {
		params[i] = make([]int, 100)
	}
	for lvl := 1; lvl < 100; lvl++ {
		params[0][lvl] = mhp // MaxHP
		params[1][lvl] = mmp // MaxMP
		params[2][lvl] = 20  // Atk
		params[3][lvl] = 15  // Def
		params[4][lvl] = 10  // Mat
		params[5][lvl] = 10  // Mdf
		params[6][lvl] = 10  // Agi
		params[7][lvl] = 10  // Luk
	}
	return params
}

func testResWithClasses() *resource.ResourceLoader {
	return &resource.ResourceLoader{
		Classes: []*resource.Class{
			nil,
			{ID: 1, Name: "Civilian", Params: makeClassParams(200, 25)},
			{ID: 2, Name: "MagicalGirl", Params: makeClassParams(400, 50)},
			{ID: 3, Name: "Warrior", Params: makeClassParams(600, 30)},
		},
	}
}

func TestApplyChangeClass_HPFullRestore_MPZero(t *testing.T) {
	// After class change, HP should be set to new class's maxHP and MP should be 0.
	res := testResWithClasses()
	e := New(newMockInventoryStore(), res, nopLogger())
	s := testSessionWithStats(1, 100, 200, 20, 25, 10, 0)
	s.ClassID = 1

	// Change class from 1 (Civilian, maxHP=200) to 2 (MagicalGirl, maxHP=400).
	params := []interface{}{float64(1), float64(2), true}
	e.applyChangeClass(context.Background(), s, params, nil)

	assert.Equal(t, 2, s.ClassID, "ClassID should be updated to 2")
	assert.Equal(t, 400, s.MaxHP, "MaxHP should match new class param")
	assert.Equal(t, 400, s.HP, "HP should be fully restored to new MaxHP")
	assert.Equal(t, 50, s.MaxMP, "MaxMP should match new class param")
	assert.Equal(t, 0, s.MP, "MP should be zero after class change")
}

func TestApplyChangeClass_SameClass_NoChange(t *testing.T) {
	// Changing to the same class should still recalculate stats.
	res := testResWithClasses()
	e := New(newMockInventoryStore(), res, nopLogger())
	s := testSessionWithStats(1, 50, 200, 10, 25, 10, 0)
	s.ClassID = 1

	params := []interface{}{float64(1), float64(1), true}
	e.applyChangeClass(context.Background(), s, params, nil)

	assert.Equal(t, 1, s.ClassID)
	assert.Equal(t, 200, s.HP, "HP restored to maxHP even for same class")
	assert.Equal(t, 0, s.MP, "MP zeroed even for same class")
}

func TestApplyChangeClass_InvalidClassID_NoOp(t *testing.T) {
	res := testResWithClasses()
	e := New(newMockInventoryStore(), res, nopLogger())
	s := testSessionWithStats(1, 100, 200, 20, 25, 10, 0)
	s.ClassID = 1

	// classID 0 → should be no-op.
	params := []interface{}{float64(1), float64(0), true}
	e.applyChangeClass(context.Background(), s, params, nil)

	assert.Equal(t, 1, s.ClassID, "ClassID should remain unchanged")
	assert.Equal(t, 100, s.HP, "HP should remain unchanged")
}

func TestApplyChangeClass_ToWarrior_HigherHP(t *testing.T) {
	res := testResWithClasses()
	e := New(newMockInventoryStore(), res, nopLogger())
	s := testSessionWithStats(1, 300, 400, 40, 50, 10, 0)
	s.ClassID = 2

	// Change from MagicalGirl (maxHP=400) to Warrior (maxHP=600).
	params := []interface{}{float64(1), float64(3), true}
	e.applyChangeClass(context.Background(), s, params, nil)

	assert.Equal(t, 3, s.ClassID)
	assert.Equal(t, 600, s.MaxHP, "MaxHP should be warrior's 600")
	assert.Equal(t, 600, s.HP, "HP fully restored")
	assert.Equal(t, 30, s.MaxMP, "MaxMP should be warrior's 30")
	assert.Equal(t, 0, s.MP, "MP zeroed")
}

func TestApplyChangeClass_SendsEffect(t *testing.T) {
	// Verify that applyChangeClass sends npc_effect packet to client.
	res := testResWithClasses()
	e := New(newMockInventoryStore(), res, nopLogger())
	s := testSessionWithStats(1, 100, 200, 20, 25, 10, 0)
	s.ClassID = 1

	params := []interface{}{float64(1), float64(2), true}
	e.applyChangeClass(context.Background(), s, params, nil)

	pkts := drainPackets(t, s)
	found := false
	for _, pkt := range pkts {
		if pkt.Type == "npc_effect" {
			found = true
			break
		}
	}
	assert.True(t, found, "should send npc_effect packet for class change")
}
