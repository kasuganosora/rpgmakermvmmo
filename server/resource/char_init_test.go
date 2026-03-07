package resource

import (
	"testing"
)

func TestExtractCharInit_CE1(t *testing.T) {
	// Load real projectb data.
	rl := &ResourceLoader{DataPath: "../../../projectb/www/data"}
	ces, err := loadJSONArray[CommonEvent](rl.path("CommonEvents.json"))
	if err != nil {
		t.Skipf("skipping: cannot load CommonEvents.json: %v", err)
	}
	rl.CommonEvents = ces

	data := rl.ExtractCharInit(1)
	if data == nil {
		t.Fatal("ExtractCharInit returned nil")
	}

	t.Logf("Extracted: %s", data)

	// Build lookup for easy checking.
	varMap := make(map[int]int)
	for _, v := range data.Variables {
		varMap[v.VariableID] = v.Value
	}

	// Verify key variables that were previously hardcoded.
	expectedVars := map[int]int{
		116:  95,   // 好感度
		231:  3,    // 天気
		299:  1,    // 地域タイプ
		300:  2,    // 地域サブ
		802:  100,  // 瘴気汚染/戦意 max
		1028: 200,  // 発情値 max
		1029: 100,  // 戦意 current
		1031: 2000, // 敏感値 max
		1033: 1,    // ゲージ表示フラグ
		722:  100,  // durability base
		702:  100,  // durability current (= v[722])
		741:  100,  // durability display (= v[742])
		742:  100,  // durability display (= v[722])
	}
	for vid, expected := range expectedVars {
		got, ok := varMap[vid]
		if !ok {
			t.Errorf("v[%d] not found in extracted data", vid)
			continue
		}
		if got != expected {
			t.Errorf("v[%d] = %d, want %d", vid, got, expected)
		}
	}

	// Verify range: v[722..740] should all be 100.
	for vid := 722; vid <= 740; vid++ {
		got, ok := varMap[vid]
		if !ok {
			t.Errorf("v[%d] not found (range 722..740)", vid)
		} else if got != 100 {
			t.Errorf("v[%d] = %d, want 100", vid, got)
		}
	}

	// Verify equipment.
	if len(data.Equips) < 3 {
		t.Fatalf("expected at least 3 equips, got %d", len(data.Equips))
	}
	expectedEquips := []CharInitEquip{
		{ArmorID: 5, SlotIndex: 1},   // Cloth → slot 1
		{ArmorID: 300, SlotIndex: 7}, // Leg → slot 7
		{ArmorID: 82, SlotIndex: 12}, // Special5 → slot 12
	}
	equipMap := make(map[int]int) // slotIndex → armorID
	for _, eq := range data.Equips {
		equipMap[eq.SlotIndex] = eq.ArmorID
	}
	for _, exp := range expectedEquips {
		got, ok := equipMap[exp.SlotIndex]
		if !ok {
			t.Errorf("equip slot %d not found", exp.SlotIndex)
		} else if got != exp.ArmorID {
			t.Errorf("equip slot %d: armorID=%d, want %d", exp.SlotIndex, got, exp.ArmorID)
		}
	}

	// Log all extracted data for review.
	t.Logf("Variables (%d total):", len(data.Variables))
	for _, v := range data.Variables {
		t.Logf("  v[%d] = %d", v.VariableID, v.Value)
	}
	t.Logf("Equips (%d total):", len(data.Equips))
	for _, eq := range data.Equips {
		t.Logf("  slot %d = armor %d", eq.SlotIndex, eq.ArmorID)
	}
}
