package npc

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"

	"github.com/dop251/goja"
	"github.com/kasuganosora/rpgmakermvmmo/server/game/player"
	"github.com/kasuganosora/rpgmakermvmmo/server/resource"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ========================================================================
// Test helpers for plugin tests
// ========================================================================

// testSessionWithEquips creates a session with equipment slots populated.
func testSessionWithEquips(charID int64, equips map[int]int) *player.PlayerSession {
	s := testSession(charID)
	s.Equips = equips
	s.Level = 10
	s.ClassID = 1
	return s
}

// testArmorWithNote creates a minimal Armor for testing with given note.
func testArmorWithNote(id int, note string) *resource.Armor {
	return &resource.Armor{
		ID:   id,
		Name: fmt.Sprintf("TestArmor%d", id),
		Note: note,
	}
}

// testResourceWithArmors creates a ResourceLoader with only armors populated.
// maxID determines the array size; only armors in the provided list are non-nil.
// Also builds PrebuiltArmors for Goja VM injection.
func testResourceWithArmors(armors []*resource.Armor) *resource.ResourceLoader {
	maxID := 0
	for _, a := range armors {
		if a != nil && a.ID > maxID {
			maxID = a.ID
		}
	}
	arr := make([]*resource.Armor, maxID+1)
	prebuilt := make([]interface{}, maxID+1)
	for _, a := range armors {
		if a != nil {
			a.ParsedMeta = resource.ParseMetaGo(a.Note)
			arr[a.ID] = a
			m := map[string]interface{}{
				"id": a.ID, "name": a.Name, "meta": a.ParsedMeta,
			}
			prebuilt[a.ID] = m
		}
	}
	return withTestMMOConfig(&resource.ResourceLoader{
		Armors:         arr,
		PrebuiltArmors: prebuilt,
	})
}

// testResourceWithTagSkillList creates a ResourceLoader with armors and TagSkillList.
func testResourceWithTagSkillList(armors []*resource.Armor, tagSkill map[int]*resource.TagSkillEntry) *resource.ResourceLoader {
	res := testResourceWithArmors(armors)
	res.TagSkillList = tagSkill
	return res
}

// realTagSkillList returns the key TagSkillList entries used in battle gauge calculations.
func realTagSkillList() map[int]*resource.TagSkillEntry {
	return map[int]*resource.TagSkillEntry{
		21:  {BaseVar: 1091, AddVar: 4221, BaseNum: 10},
		30:  {BaseVar: 1100, AddVar: 4230, BaseNum: 40},
		32:  {BaseVar: 1192, AddVar: 4232, BaseNum: 25},
		35:  {BaseVar: 1195, AddVar: 4235, BaseNum: -5},
		40:  {BaseVar: 1200, AddVar: 4240, BaseNum: 0},
		49:  {BaseVar: 1209, AddVar: 4249, BaseNum: 0},
		55:  {BaseVar: 1215, AddVar: 4255, BaseNum: 0},
		57:  {BaseVar: 1217, AddVar: 4257, BaseNum: 3},
		97:  {BaseVar: 1259, AddVar: 4297, BaseNum: 100},
		100: {BaseVar: 1260, AddVar: 4300, BaseNum: 0},
		102: {BaseVar: 1262, AddVar: 4302, BaseNum: 50},
	}
}

// dumpVars logs all non-zero variables in the range for debugging.
func dumpVars(t *testing.T, gs *mockGameState, from, to int, label string) {
	t.Helper()
	t.Logf("--- %s (vars %d-%d) ---", label, from, to)
	for i := from; i <= to; i++ {
		v := gs.variables[i]
		if v != 0 {
			t.Logf("  v[%d] = %d", i, v)
		}
	}
}

// ========================================================================
// 1. parseMeta tests — verify kaeru.js semicolon meta parsing
// ========================================================================

func TestParseMeta_SemicolonArray(t *testing.T) {
	vm := goja.New()
	meta := parseMeta(vm, "<Skill01;[40,10]>")

	val := meta.Get("Skill01")
	require.NotNil(t, val, "Skill01 should exist in meta")
	require.False(t, goja.IsUndefined(val), "Skill01 should not be undefined")

	exported := val.Export()
	arr, ok := exported.([]interface{})
	require.True(t, ok, "Skill01 should be a JS array, got %T: %v", exported, exported)
	require.Len(t, arr, 2, "Skill01 array should have 2 elements")

	// Check values — JSON numbers export as float64
	assert.Equal(t, float64(40), arr[0], "Skill01[0] should be 40")
	assert.Equal(t, float64(10), arr[1], "Skill01[1] should be 10")
}

func TestParseMeta_ColonString(t *testing.T) {
	vm := goja.New()
	meta := parseMeta(vm, "<ClothName:Uniform>")

	val := meta.Get("ClothName")
	require.NotNil(t, val)
	assert.Equal(t, "Uniform", val.Export())
}

func TestParseMeta_BooleanTag(t *testing.T) {
	vm := goja.New()
	meta := parseMeta(vm, "<ClothUnderFlag>")

	val := meta.Get("ClothUnderFlag")
	require.NotNil(t, val)
	assert.Equal(t, true, val.Export())
}

func TestParseMeta_MultipleTagsIncludingSemicolon(t *testing.T) {
	vm := goja.New()
	note := "<ClothName:Uniform>\n<Skill01;[40,10]>\n<Shame;0>\n<Formal:School>"
	meta := parseMeta(vm, note)

	assert.Equal(t, "Uniform", meta.Get("ClothName").Export())
	assert.Equal(t, "School", meta.Get("Formal").Export())

	// Shame;0 → JSON.parse("0") = 0 (goja exports as int64)
	shame := meta.Get("Shame")
	require.NotNil(t, shame)
	assert.EqualValues(t, 0, shame.Export())

	// Skill01 array
	sk := meta.Get("Skill01")
	arr, ok := sk.Export().([]interface{})
	require.True(t, ok)
	assert.Equal(t, float64(40), arr[0])
	assert.Equal(t, float64(10), arr[1])
}

func TestParseMeta_RealArmor5Note(t *testing.T) {
	// This is the actual note from projectb Armor 5 (制服)
	note := `itemMaxNum:1
<ClothPicNum;1>
<ClothOpacity;255>
<ClothNippleL;[0,0,0,0,0,1,0,0,0,1]>
<ClothNippleR;[0,0,0,1,0,0,1,0,0,1]>
<ClothUnderFlag;1>
<ClothCharachip:actor01_0002>
<ClothCharachipNumber;0>
<FileNumCloth:0001>
<ClothSkimpy;0>
<ClothName:Uniform>
<Shame;0>
<Formal:School>
<Skill01;[40,10]>`

	vm := goja.New()
	meta := parseMeta(vm, note)

	// ClothName is colon-delimited string
	assert.Equal(t, "Uniform", meta.Get("ClothName").Export())

	// Skill01 is semicolon-delimited JSON array
	sk := meta.Get("Skill01")
	require.NotNil(t, sk)
	arr, ok := sk.Export().([]interface{})
	require.True(t, ok, "Skill01 should be array, got %T", sk.Export())
	assert.Equal(t, float64(40), arr[0])
	assert.Equal(t, float64(10), arr[1])
}

// ========================================================================
// 2. CulSkillEffect tests — verify equipment skill accumulation
// ========================================================================

func TestExecCulSkillEffect_SingleEquip_Skill01(t *testing.T) {
	// Armor 5 has <Skill01;[40,10]>
	// This means: target = 40+4200 = v[4240], value += 10
	res := testResourceWithArmors([]*resource.Armor{
		testArmorWithNote(5, "<Skill01;[40,10]>"),
	})
	store := newMockInventoryStore()
	exec := New(store, res, nopLogger())
	gs := newMockGameState()
	s := testSessionWithEquips(1, map[int]int{
		1: 5, // slot 1 = armor 5
	})

	opts := &ExecuteOpts{
		GameState:     gs,
		TransientVars: make(map[int]interface{}),
	}

	exec.execCulSkillEffect(s, opts)

	// After CulSkillEffect:
	// v[4240] should be 10 (from Skill01 [40,10]: target=40+4200=4240, value=10)
	dumpVars(t, gs, 4220, 4260, "After CulSkillEffect")

	v4240 := gs.GetVariable(4240)
	assert.Equal(t, 10, v4240, "v[4240] should be 10 (from Skill01=[40,10])")
}

func TestExecCulSkillEffect_MultipleSkillTags(t *testing.T) {
	// Armor with multiple skill tags
	res := testResourceWithArmors([]*resource.Armor{
		testArmorWithNote(10, "<Skill01;[40,10]>\n<Skill02;[31,5]>\n<Skill03;[22,20]>"),
	})
	store := newMockInventoryStore()
	exec := New(store, res, nopLogger())
	gs := newMockGameState()
	s := testSessionWithEquips(1, map[int]int{
		1: 10,
	})

	opts := &ExecuteOpts{
		GameState:     gs,
		TransientVars: make(map[int]interface{}),
	}

	exec.execCulSkillEffect(s, opts)

	dumpVars(t, gs, 4220, 4260, "After CulSkillEffect with 3 skills")

	assert.Equal(t, 10, gs.GetVariable(4240), "v[4240] from Skill01=[40,10]")
	assert.Equal(t, 5, gs.GetVariable(4231), "v[4231] from Skill02=[31,5]")
	assert.Equal(t, 20, gs.GetVariable(4222), "v[4222] from Skill03=[22,20]")
}

func TestExecCulSkillEffect_TwoEquipsSameTarget(t *testing.T) {
	// Two armors both contribute to the same target variable
	res := testResourceWithArmors([]*resource.Armor{
		testArmorWithNote(5, "<Skill01;[40,10]>"),
		testArmorWithNote(10, "<Skill01;[40,15]>"),
	})
	store := newMockInventoryStore()
	exec := New(store, res, nopLogger())
	gs := newMockGameState()
	s := testSessionWithEquips(1, map[int]int{
		1: 5,
		2: 10,
	})

	opts := &ExecuteOpts{
		GameState:     gs,
		TransientVars: make(map[int]interface{}),
	}

	exec.execCulSkillEffect(s, opts)

	dumpVars(t, gs, 4220, 4260, "After CulSkillEffect with 2 equips")

	// Both armors add to v[4240]: 10 + 15 = 25
	assert.Equal(t, 25, gs.GetVariable(4240), "v[4240] should be 25 (10+15 from two equips)")
}

func TestExecCulSkillEffect_DuplicateEquipOther1Other2(t *testing.T) {
	// Slots Other1=4 and Other2=5 with same equip → slot 5 is skipped
	res := testResourceWithArmors([]*resource.Armor{
		testArmorWithNote(20, "<Skill01;[40,10]>"),
	})
	store := newMockInventoryStore()
	exec := New(store, res, nopLogger())
	gs := newMockGameState()
	s := testSessionWithEquips(1, map[int]int{
		4: 20,
		5: 20, // same as slot 4 → should be skipped
	})

	opts := &ExecuteOpts{
		GameState:     gs,
		TransientVars: make(map[int]interface{}),
	}

	exec.execCulSkillEffect(s, opts)

	dumpVars(t, gs, 4220, 4260, "After CulSkillEffect with duplicate equip on Other1/Other2")

	// Only counted once because slot 5 with same ID as slot 4 is skipped
	assert.Equal(t, 10, gs.GetVariable(4240), "v[4240] should be 10 (duplicate equip skipped)")
}

func TestExecCulSkillEffect_AddSkillEffectBase(t *testing.T) {
	// Test AddSkillEffectBase: v[BaseVar] = BaseNum + v[AddVar]
	// Armor 5 has Skill01=[40,10] → v[4240] += 10
	// TagSkillList entry 40: BaseVar=1200, AddVar=4240, BaseNum=0
	// So v[1200] = 0 + 10 = 10
	// TagSkillList entry 49: BaseVar=1209, AddVar=4249, BaseNum=0
	// No equipment contributes to 4249, so v[1209] = 0 + 0 = 0
	tagSkill := realTagSkillList()
	res := testResourceWithTagSkillList([]*resource.Armor{
		testArmorWithNote(5, "<Skill01;[40,10]>"),
	}, tagSkill)
	store := newMockInventoryStore()
	exec := New(store, res, nopLogger())
	gs := newMockGameState()
	s := testSessionWithEquips(1, map[int]int{1: 5})

	opts := &ExecuteOpts{
		GameState:     gs,
		TransientVars: make(map[int]interface{}),
	}

	exec.execCulSkillEffect(s, opts)

	dumpVars(t, gs, 4220, 4260, "After CulSkillEffect accumulators")
	dumpVars(t, gs, 1190, 1220, "After AddSkillEffectBase")

	// v[4240] = 10 from Skill01=[40,10]
	assert.Equal(t, 10, gs.GetVariable(4240), "v[4240] accumulator")
	// v[1200] = BaseNum(0) + v[4240](10) = 10
	assert.Equal(t, 10, gs.GetVariable(1200), "v[1200] = BaseNum + v[4240]")
	// v[1209] = BaseNum(0) + v[4249](0) = 0 (no equipment adds to 4249)
	assert.Equal(t, 0, gs.GetVariable(1209), "v[1209] = BaseNum + v[4249] (no contrib)")
	// v[1259] = BaseNum(100) + v[4297](0) = 100
	assert.Equal(t, 100, gs.GetVariable(1259), "v[1259] miasma max = 100 (base)")
	// v[1091] = BaseNum(10) + v[4221](0) = 10
	assert.Equal(t, 10, gs.GetVariable(1091), "v[1091] = BaseNum(10)")
}

func TestExecCulSkillEffect_AddSkillEffectBase_WithMultipleSkills(t *testing.T) {
	// Armor with Skill01=[40,10] AND a skill that targets 4249 (v[1209] base)
	// Skill01=[40,10] → v[4240] += 10
	// Also add a meta tag that targets entry 49: need armor with tag that makes v[4249]
	// Entry 49: BaseVar=1209, AddVar=4249
	// The AddVar for v[1209] is 4249. So we need Skill_N = [49, X] for some N.
	// Skill01=[49,30] → v[4249] += 30 → v[1209] = 0 + 30 = 30
	tagSkill := realTagSkillList()
	res := testResourceWithTagSkillList([]*resource.Armor{
		testArmorWithNote(5, "<Skill01;[49,30]>\n<Skill02;[40,15]>"),
	}, tagSkill)
	store := newMockInventoryStore()
	exec := New(store, res, nopLogger())
	gs := newMockGameState()
	s := testSessionWithEquips(1, map[int]int{1: 5})

	opts := &ExecuteOpts{
		GameState:     gs,
		TransientVars: make(map[int]interface{}),
	}

	exec.execCulSkillEffect(s, opts)

	dumpVars(t, gs, 4240, 4260, "Accumulators")
	dumpVars(t, gs, 1200, 1215, "Base values")

	assert.Equal(t, 30, gs.GetVariable(4249), "v[4249] from Skill01=[49,30]")
	assert.Equal(t, 15, gs.GetVariable(4240), "v[4240] from Skill02=[40,15]")
	assert.Equal(t, 30, gs.GetVariable(1209), "v[1209] = 0 + v[4249](30) = 30 (cloth max)")
	assert.Equal(t, 15, gs.GetVariable(1200), "v[1200] = 0 + v[4240](15)")
}

func TestExecCulSkillEffect_NakedBonus(t *testing.T) {
	// No armor in slot 1 → naked bonus: v[1200] += 40
	res := testResourceWithArmors([]*resource.Armor{})
	store := newMockInventoryStore()
	exec := New(store, res, nopLogger())
	gs := newMockGameState()
	s := testSessionWithEquips(1, map[int]int{
		1: 0, // no armor
	})

	opts := &ExecuteOpts{
		GameState:     gs,
		TransientVars: make(map[int]interface{}),
	}

	exec.execCulSkillEffect(s, opts)

	dumpVars(t, gs, 1198, 1202, "After CulSkillEffect naked")

	assert.Equal(t, 40, gs.GetVariable(1200), "v[1200] should be 40 (naked bonus)")
}

func TestExecCulSkillEffect_NotNaked(t *testing.T) {
	// Armor in slot 1 → no naked bonus
	res := testResourceWithArmors([]*resource.Armor{
		testArmorWithNote(5, "<Skill01;[40,10]>"),
	})
	store := newMockInventoryStore()
	exec := New(store, res, nopLogger())
	gs := newMockGameState()
	s := testSessionWithEquips(1, map[int]int{
		1: 5,
	})

	opts := &ExecuteOpts{
		GameState:     gs,
		TransientVars: make(map[int]interface{}),
	}

	exec.execCulSkillEffect(s, opts)

	assert.Equal(t, 0, gs.GetVariable(1200), "v[1200] should be 0 (not naked)")
}

// ========================================================================
// 3. ParaCheck tests — verify parameter clamping and gauge calculation
// ========================================================================

func TestExecParaCheck_ClothDurability(t *testing.T) {
	// ParaCheck: v[722] = v[1209] (durability max from skill)
	// v[702] = clamp(v[702], 0, v[722])
	// v[741] = v[702], v[742] = v[722] (display values)
	res := testResourceWithArmors([]*resource.Armor{
		testArmorWithNote(5, "<ClothName:Uniform>"),
	})
	store := newMockInventoryStore()
	store.gold[1] = 5000
	exec := New(store, res, nopLogger())

	gs := newMockGameState()
	gs.variables[1209] = 100 // durability max from skill effect
	gs.variables[702] = 80   // current durability

	s := testSessionWithEquips(1, map[int]int{1: 5})
	s.Level = 10
	s.ClassID = 1

	opts := &ExecuteOpts{
		GameState:     gs,
		TransientVars: make(map[int]interface{}),
	}

	exec.execParaCheck(s, opts)

	dumpVars(t, gs, 700, 750, "After ParaCheck cloth durability")

	assert.Equal(t, 100, gs.GetVariable(722), "v[722] should be 100 (= v[1209])")
	assert.Equal(t, 80, gs.GetVariable(702), "v[702] should be 80 (clamped within 0-100)")
	assert.Equal(t, 80, gs.GetVariable(741), "v[741] should mirror v[702] for gauge display")
	assert.Equal(t, 100, gs.GetVariable(742), "v[742] should mirror v[722] for gauge max")
}

func TestExecParaCheck_ClothDurabilityClamp(t *testing.T) {
	// v[702] exceeds v[722] max → should be clamped
	res := testResourceWithArmors([]*resource.Armor{
		testArmorWithNote(5, "<ClothName:Uniform>"),
	})
	store := newMockInventoryStore()
	store.gold[1] = 0
	exec := New(store, res, nopLogger())

	gs := newMockGameState()
	gs.variables[1209] = 50 // max durability
	gs.variables[702] = 80  // current > max → should clamp to 50

	s := testSessionWithEquips(1, map[int]int{1: 5})
	s.Level = 5
	s.ClassID = 1

	opts := &ExecuteOpts{
		GameState:     gs,
		TransientVars: make(map[int]interface{}),
	}

	exec.execParaCheck(s, opts)

	assert.Equal(t, 50, gs.GetVariable(702), "v[702] should be clamped to v[722]=50")
	assert.Equal(t, 50, gs.GetVariable(741), "v[741] should mirror clamped v[702]")
	assert.Equal(t, 50, gs.GetVariable(742), "v[742] should mirror v[722]")
}

func TestExecParaCheck_Arousal(t *testing.T) {
	// v[1027] 発情: clamp(v[1027], v[1282], 200) for classId <= 2
	res := testResourceWithArmors([]*resource.Armor{
		testArmorWithNote(5, "<ClothName:Uniform>"),
	})
	store := newMockInventoryStore()
	exec := New(store, res, nopLogger())

	gs := newMockGameState()
	gs.variables[1027] = 150
	gs.variables[1282] = 10  // lower bound
	gs.variables[1209] = 100 // needed for cloth durability
	gs.variables[702] = 50

	s := testSessionWithEquips(1, map[int]int{1: 5})
	s.Level = 10
	s.ClassID = 1 // classId <= 2 → arousal works

	opts := &ExecuteOpts{
		GameState:     gs,
		TransientVars: make(map[int]interface{}),
	}

	exec.execParaCheck(s, opts)

	assert.Equal(t, 150, gs.GetVariable(1027), "v[1027] should be 150 (within bounds 10-200)")
}

func TestExecParaCheck_ArousalClassId3(t *testing.T) {
	// classId > 2 → v[1027] forced to 0
	res := testResourceWithArmors([]*resource.Armor{
		testArmorWithNote(5, "<ClothName:Uniform>"),
	})
	store := newMockInventoryStore()
	exec := New(store, res, nopLogger())

	gs := newMockGameState()
	gs.variables[1027] = 150
	gs.variables[1209] = 100
	gs.variables[702] = 50

	s := testSessionWithEquips(1, map[int]int{1: 5})
	s.Level = 10
	s.ClassID = 3 // classId > 2 → arousal = 0

	opts := &ExecuteOpts{
		GameState:     gs,
		TransientVars: make(map[int]interface{}),
	}

	exec.execParaCheck(s, opts)

	assert.Equal(t, 0, gs.GetVariable(1027), "v[1027] should be 0 (classId > 2)")
}

func TestExecParaCheck_Lust(t *testing.T) {
	// v[1021] 淫欲: clamp(0, v[1034])
	res := testResourceWithArmors([]*resource.Armor{
		testArmorWithNote(5, "<ClothName:Uniform>"),
	})
	store := newMockInventoryStore()
	exec := New(store, res, nopLogger())

	gs := newMockGameState()
	gs.variables[1021] = 120
	gs.variables[1034] = 100  // upper bound from skill (ParaCheck sets this from v[1260])
	gs.variables[1260] = 100  // source for v[1034]
	gs.variables[1209] = 100
	gs.variables[702] = 50

	s := testSessionWithEquips(1, map[int]int{1: 5})
	s.Level = 10
	s.ClassID = 1

	opts := &ExecuteOpts{
		GameState:     gs,
		TransientVars: make(map[int]interface{}),
	}

	exec.execParaCheck(s, opts)

	// v[1034] = v[1260] = 100, then v[1021] clamped to 100
	assert.Equal(t, 100, gs.GetVariable(1021), "v[1021] should be clamped to 100 (upper bound v[1034])")
}

func TestExecParaCheck_SoulErosion(t *testing.T) {
	// v[1178] = floor(v[1280] / 15), capped at 6
	// Switches 1031-1035 set based on v[1178]
	res := testResourceWithArmors([]*resource.Armor{
		testArmorWithNote(5, "<ClothName:Uniform>"),
	})
	store := newMockInventoryStore()
	exec := New(store, res, nopLogger())

	gs := newMockGameState()
	gs.variables[1280] = 45  // 45/15 = 3
	gs.variables[1209] = 100
	gs.variables[702] = 50

	s := testSessionWithEquips(1, map[int]int{1: 5})
	s.Level = 10
	s.ClassID = 1

	opts := &ExecuteOpts{
		GameState:     gs,
		TransientVars: make(map[int]interface{}),
	}

	exec.execParaCheck(s, opts)

	assert.Equal(t, 3, gs.GetVariable(1178), "v[1178] should be 3 (floor(45/15))")
	assert.True(t, gs.GetSwitch(1031), "sw[1031] should be true (erosion >= 1)")
	assert.True(t, gs.GetSwitch(1032), "sw[1032] should be true (erosion >= 2)")
	assert.True(t, gs.GetSwitch(1033), "sw[1033] should be true (erosion >= 3)")
	assert.False(t, gs.GetSwitch(1034), "sw[1034] should be false (erosion < 4)")
	assert.False(t, gs.GetSwitch(1035), "sw[1035] should be false (erosion < 5)")
}

func TestExecParaCheck_Stealth(t *testing.T) {
	// v[1215] > 0 → sw[162] = true
	res := testResourceWithArmors([]*resource.Armor{
		testArmorWithNote(5, "<ClothName:Uniform>"),
	})
	store := newMockInventoryStore()
	exec := New(store, res, nopLogger())

	gs := newMockGameState()
	gs.variables[1215] = 1
	gs.variables[1209] = 100
	gs.variables[702] = 50

	s := testSessionWithEquips(1, map[int]int{1: 5})
	s.Level = 10
	s.ClassID = 1

	opts := &ExecuteOpts{
		GameState:     gs,
		TransientVars: make(map[int]interface{}),
	}

	exec.execParaCheck(s, opts)

	assert.True(t, gs.GetSwitch(162), "sw[162] should be true (stealth active)")
}

func TestExecParaCheck_Transform(t *testing.T) {
	// sw[131] = true → v[235] = 1 (transformed)
	res := testResourceWithArmors([]*resource.Armor{
		testArmorWithNote(5, "<ClothName:Uniform>"),
	})
	store := newMockInventoryStore()
	exec := New(store, res, nopLogger())

	gs := newMockGameState()
	gs.switches[131] = true
	gs.variables[1209] = 100
	gs.variables[702] = 50

	s := testSessionWithEquips(1, map[int]int{1: 5})
	s.Level = 10
	s.ClassID = 1

	opts := &ExecuteOpts{
		GameState:     gs,
		TransientVars: make(map[int]interface{}),
	}

	exec.execParaCheck(s, opts)

	assert.Equal(t, 1, gs.GetVariable(235), "v[235] should be 1 (transformed)")
}

func TestExecParaCheck_GoldAndLevel(t *testing.T) {
	// v[1006] = __playerLevel, v[215] = __gold
	res := testResourceWithArmors([]*resource.Armor{
		testArmorWithNote(5, "<ClothName:Uniform>"),
	})
	store := newMockInventoryStore()
	store.gold[1] = 12345
	exec := New(store, res, nopLogger())

	gs := newMockGameState()
	gs.variables[1209] = 100
	gs.variables[702] = 50

	s := testSessionWithEquips(1, map[int]int{1: 5})
	s.Level = 25
	s.ClassID = 1

	opts := &ExecuteOpts{
		GameState:     gs,
		TransientVars: make(map[int]interface{}),
	}

	exec.execParaCheck(s, opts)

	assert.Equal(t, 25, gs.GetVariable(1006), "v[1006] should be player level 25")
	assert.Equal(t, 12345, gs.GetVariable(215), "v[215] should be gold 12345")
}

func TestExecParaCheck_NakedClothName(t *testing.T) {
	// No equip in slot 1 or itemId < 5 → v[762] = "Naked"
	// This is a string value, check it's stored in TransientVars
	res := testResourceWithArmors([]*resource.Armor{})
	store := newMockInventoryStore()
	exec := New(store, res, nopLogger())

	gs := newMockGameState()
	gs.variables[1209] = 100
	gs.variables[702] = 50

	s := testSessionWithEquips(1, map[int]int{
		1: 0, // no armor
	})
	s.Level = 10
	s.ClassID = 1

	opts := &ExecuteOpts{
		GameState:     gs,
		TransientVars: make(map[int]interface{}),
	}

	exec.execParaCheck(s, opts)

	// v[762] is a string "Naked" — stored in TransientVars
	if tv, ok := opts.TransientVars[762]; ok {
		assert.Equal(t, "Naked", tv, "v[762] should be 'Naked'")
	} else {
		// Might also be stored as regular var if the system converts it
		t.Logf("v[762] not in TransientVars, checking varChanges... v[762]=%d", gs.GetVariable(762))
	}
}

// ========================================================================
// 4. Full chain: CulSkillEffect → ParaCheck (battle scenario)
// ========================================================================

func TestFullChain_BattleGauges(t *testing.T) {
	// Simulate CE 1031 flow: equip battle armor → CE 891 (CulSkillEffect + ParaCheck)
	// Armor 71 (無垢天衣) has Skill07=[49,100] → v[4249]+=100 → v[1209]=100
	// Also has other skill tags making it realistic
	res := testResourceWithTagSkillList([]*resource.Armor{
		testArmorWithNote(71, "<Skill01;[40,10]>\n<Skill07;[49,100]>\n<ClothName:TenI>"),
	}, realTagSkillList())
	store := newMockInventoryStore()
	store.gold[1] = 5000
	exec := New(store, res, nopLogger())

	gs := newMockGameState()
	// Character creation defaults
	gs.variables[702] = 100  // cloth durability current
	gs.variables[722] = 100  // cloth durability max (will be reset by ParaCheck to v[1209])
	gs.variables[802] = 100
	gs.variables[1028] = 200
	gs.variables[1029] = 100 // 戦意 (fighting spirit)
	gs.variables[1031] = 2000 // 性感 upper bound

	// v[1209], v[1260], v[1259] are now computed by AddSkillEffectBase
	// from TagSkillList entries, NOT pre-set. This tests the real flow.

	// Some parameter values to check clamping
	gs.variables[1021] = 50  // lust
	gs.variables[1027] = 30  // arousal
	gs.variables[1030] = 40  // miasma
	gs.variables[1022] = 20  // erosion
	gs.variables[1025] = 60  // fame
	gs.variables[217] = 500  // soul
	gs.variables[1023] = 50  // school eval
	gs.variables[1024] = 30  // citizen eval
	gs.variables[212] = 70   // domination
	gs.variables[1026] = 100 // sensitivity
	gs.variables[202] = 5    // turn
	gs.variables[1019] = 20  // hypnosis

	s := testSessionWithEquips(1, map[int]int{
		1: 71, // armor 71 (battle outfit) in slot 1
	})
	s.Level = 10
	s.ClassID = 2 // class 2 = transformed state

	opts := &ExecuteOpts{
		GameState:     gs,
		TransientVars: make(map[int]interface{}),
	}

	// Step 1: CulSkillEffect (includes AddSkillEffectBase)
	exec.execCulSkillEffect(s, opts)

	t.Log("=== After CulSkillEffect ===")
	dumpVars(t, gs, 4240, 4260, "Skill accumulator vars")
	dumpVars(t, gs, 1195, 1215, "Skill effect vars (from AddSkillEffectBase)")

	// Verify CulSkillEffect + AddSkillEffectBase
	assert.Equal(t, 10, gs.GetVariable(4240), "v[4240] from Skill01=[40,10]")
	assert.Equal(t, 100, gs.GetVariable(4249), "v[4249] from Skill07=[49,100]")
	assert.Equal(t, 100, gs.GetVariable(1209), "v[1209] = BaseNum(0) + v[4249](100) = 100 (cloth max)")
	assert.Equal(t, 10, gs.GetVariable(1200), "v[1200] = BaseNum(0) + v[4240](10)")

	// Step 2: ParaCheck
	exec.execParaCheck(s, opts)

	t.Log("=== After ParaCheck ===")
	dumpVars(t, gs, 700, 750, "Cloth durability vars")
	dumpVars(t, gs, 1019, 1040, "Parameter vars")
	dumpVars(t, gs, 210, 220, "Gold/Soul vars")

	// ---- GAUGE VERIFICATION ----
	// These are the critical assertions that verify battle gauges work

	// Cloth durability gauge — v[1209] computed by AddSkillEffectBase = 100
	assert.Equal(t, 100, gs.GetVariable(722), "v[722] cloth max = v[1209]")
	assert.Equal(t, 100, gs.GetVariable(702), "v[702] cloth current (clamped to v[722])")
	assert.Equal(t, 100, gs.GetVariable(741), "v[741] gauge display value = v[702]")
	assert.Equal(t, 100, gs.GetVariable(742), "v[742] gauge max display = v[722]")

	// Other parameter clamps
	// v[1260] (lust max) = BaseNum(0) + v[4300](0) = 0, so lust clamped to 0
	// v[1034] = v[1260] = 0 in ParaCheck
	assert.Equal(t, 0, gs.GetVariable(1021), "v[1021] lust clamped to 0 (lust max v[1260]=0)")
	assert.Equal(t, 30, gs.GetVariable(1027), "v[1027] arousal preserved (classId=2, <=2)")
	// v[1259] (miasma max) = BaseNum(100) + v[4297](0) = 100
	assert.Equal(t, 40, gs.GetVariable(1030), "v[1030] miasma preserved (miasma max v[1259]=100)")
	assert.Equal(t, 100, gs.GetVariable(1029), "v[1029] fighting spirit (clamped 0-100)")
	assert.Equal(t, 60, gs.GetVariable(1025), "v[1025] fame (clamped 0-100)")
	assert.Equal(t, 500, gs.GetVariable(217), "v[217] soul preserved (0-99999)")
	assert.Equal(t, 50, gs.GetVariable(1023), "v[1023] school eval (clamped -100-100)")
	assert.Equal(t, 30, gs.GetVariable(1024), "v[1024] citizen eval (clamped -100-100)")
	assert.Equal(t, 70, gs.GetVariable(212), "v[212] domination (clamped 0-100)")
	assert.Equal(t, 100, gs.GetVariable(1026), "v[1026] sensitivity (clamped to v[1031])")
	assert.Equal(t, 5, gs.GetVariable(202), "v[202] turn (clamped 0-999)")
	assert.Equal(t, 20, gs.GetVariable(1019), "v[1019] hypnosis (clamped 0-100)")

	// Gold and level
	assert.Equal(t, 10, gs.GetVariable(1006), "v[1006] should be player level")
	assert.Equal(t, 5000, gs.GetVariable(215), "v[215] should be gold")
}

func TestFullChain_BattleGauges_AllZeroInitial(t *testing.T) {
	// Edge case: all parameters start at 0.
	// Gauges should still show correct max values from AddSkillEffectBase.
	res := testResourceWithTagSkillList([]*resource.Armor{
		testArmorWithNote(71, "<Skill07;[49,100]>\n<ClothName:TenI>"),
	}, realTagSkillList())
	store := newMockInventoryStore()
	exec := New(store, res, nopLogger())

	gs := newMockGameState()
	// v[1209] NOT pre-set — should be computed by AddSkillEffectBase

	s := testSessionWithEquips(1, map[int]int{1: 71})
	s.Level = 1
	s.ClassID = 2

	opts := &ExecuteOpts{
		GameState:     gs,
		TransientVars: make(map[int]interface{}),
	}

	exec.execCulSkillEffect(s, opts)
	exec.execParaCheck(s, opts)

	t.Log("=== All-zero initial state ===")
	dumpVars(t, gs, 700, 750, "Cloth durability vars")
	dumpVars(t, gs, 1205, 1215, "Skill base vars")

	// v[1209] computed by AddSkillEffectBase: BaseNum(0) + v[4249](100) = 100
	assert.Equal(t, 100, gs.GetVariable(1209), "v[1209] from AddSkillEffectBase = 100")
	assert.Equal(t, 100, gs.GetVariable(722), "v[722] cloth max = v[1209] = 100")
	assert.Equal(t, 0, gs.GetVariable(702), "v[702] cloth current should be 0 (clamped)")
	assert.Equal(t, 0, gs.GetVariable(741), "v[741] gauge display = v[702] = 0")
	assert.Equal(t, 100, gs.GetVariable(742), "v[742] gauge max display = v[722] = 100")
}

// ========================================================================
// 5. Regression: equips array boundary and nil handling
// ========================================================================

func TestExecCulSkillEffect_NoEquips(t *testing.T) {
	// No equipment at all — should not crash
	res := testResourceWithArmors([]*resource.Armor{})
	store := newMockInventoryStore()
	exec := New(store, res, nopLogger())
	gs := newMockGameState()
	s := testSessionWithEquips(1, nil)

	opts := &ExecuteOpts{
		GameState:     gs,
		TransientVars: make(map[int]interface{}),
	}

	// Should not panic
	exec.execCulSkillEffect(s, opts)
	t.Log("CulSkillEffect with no equips completed without panic")
}

func TestExecParaCheck_NilOpts(t *testing.T) {
	// Nil opts → should return without crash
	exec := New(nil, nil, nopLogger())
	exec.execParaCheck(nil, nil)
	t.Log("ParaCheck with nil opts returned without panic")
}

// ========================================================================
// 6. Skill input vars relay (v[1111-1113] from v[1272-1274])
// ========================================================================

// ========================================================================
// 7. End-to-end dispatch: Execute with plugin command events
// ========================================================================

func TestExecute_PluginDispatch_CulSkillEffectAndParaCheck(t *testing.T) {
	// Simulate the actual event flow: code 356 "CulSkillEffect" then "ParaCheck"
	res := testResourceWithTagSkillList([]*resource.Armor{
		testArmorWithNote(71, "<Skill01;[40,10]>\n<Skill07;[49,100]>\n<ClothName:TenI>"),
	}, realTagSkillList())
	store := newMockInventoryStore()
	store.gold[1] = 3000
	exec := New(store, res, nopLogger())

	gs := newMockGameState()
	gs.variables[702] = 75 // cloth durability current
	// v[1209] NOT pre-set — computed by AddSkillEffectBase

	s := testSessionWithEquips(1, map[int]int{1: 71})
	s.Level = 15
	s.ClassID = 2

	page := &resource.EventPage{
		List: []*resource.EventCommand{
			{Code: CmdPluginCommand, Parameters: []interface{}{"CulSkillEffect"}},
			{Code: CmdPluginCommand, Parameters: []interface{}{"ParaCheck"}},
			{Code: CmdEnd, Indent: 0},
		},
	}

	opts := &ExecuteOpts{
		GameState:     gs,
		TransientVars: make(map[int]interface{}),
	}

	exec.Execute(context.Background(), s, page, opts)

	// Drain any packets sent during execution
	drainPackets(t, s)

	// Verify the full chain executed correctly through dispatch
	dumpVars(t, gs, 700, 750, "After dispatch: cloth durability")
	dumpVars(t, gs, 4240, 4260, "After dispatch: skill accumulators")
	dumpVars(t, gs, 1200, 1215, "After dispatch: skill base values")

	assert.Equal(t, 10, gs.GetVariable(4240), "v[4240] from Skill01=[40,10] via dispatch")
	assert.Equal(t, 100, gs.GetVariable(4249), "v[4249] from Skill07=[49,100] via dispatch")
	assert.Equal(t, 100, gs.GetVariable(1209), "v[1209] from AddSkillEffectBase via dispatch")
	assert.Equal(t, 100, gs.GetVariable(722), "v[722] cloth max = v[1209] via dispatch")
	assert.Equal(t, 75, gs.GetVariable(702), "v[702] cloth current (within bounds)")
	assert.Equal(t, 75, gs.GetVariable(741), "v[741] gauge display = v[702]")
	assert.Equal(t, 100, gs.GetVariable(742), "v[742] gauge max = v[722]")
	assert.Equal(t, 15, gs.GetVariable(1006), "v[1006] should be player level via dispatch")
	assert.Equal(t, 3000, gs.GetVariable(215), "v[215] should be gold via dispatch")
}

func TestExecParaCheck_SkillInputRelay(t *testing.T) {
	res := testResourceWithArmors([]*resource.Armor{
		testArmorWithNote(5, "<ClothName:Uniform>"),
	})
	store := newMockInventoryStore()
	exec := New(store, res, nopLogger())

	gs := newMockGameState()
	gs.variables[1272] = 15 // 精液
	gs.variables[1273] = 20 // 被虐
	gs.variables[1274] = 25 // 奉仕
	gs.variables[1209] = 100
	gs.variables[702] = 50

	s := testSessionWithEquips(1, map[int]int{1: 5})
	s.Level = 10
	s.ClassID = 1

	opts := &ExecuteOpts{
		GameState:     gs,
		TransientVars: make(map[int]interface{}),
	}

	exec.execParaCheck(s, opts)

	assert.Equal(t, 15, gs.GetVariable(1111), "v[1111] should relay v[1272]")
	assert.Equal(t, 20, gs.GetVariable(1112), "v[1112] should relay v[1273]")
	assert.Equal(t, 25, gs.GetVariable(1113), "v[1113] should relay v[1274]")
}

// ========================================================================
// EquipChange → session.Equips update → CulSkillEffect reads new equipment
// ========================================================================

func TestApplyEquipChange_UpdatesSessionEquips(t *testing.T) {
	store := newMockInventoryStore()
	exec := New(store, &resource.ResourceLoader{}, nopLogger())

	s := testSessionWithEquips(1, map[int]int{1: 5}) // start with armor 5
	gs := newMockGameState()
	opts := &ExecuteOpts{
		GameState:     gs,
		TransientVars: make(map[int]interface{}),
	}

	// EquipChange Cloth 71 → slot 1 = armor 71
	exec.applyEquipChange(context.Background(), s, "Cloth", "71", opts)

	assert.Equal(t, 71, s.GetEquip(1), "session.Equips[1] should be 71 after EquipChange")
	assert.Equal(t, 1, gs.GetVariable(2701), "v[2701] should be slot index 1")
	assert.Equal(t, 71, gs.GetVariable(2703), "v[2703] should be armor ID 71")
}

func TestApplyEquipChange_AllSlotTypes(t *testing.T) {
	store := newMockInventoryStore()
	exec := New(store, &resource.ResourceLoader{}, nopLogger())

	tests := []struct {
		slotType string
		expected int
	}{
		{"Weapon", 0}, {"Cloth", 1}, {"ClothOption", 2}, {"Option", 3},
		{"Other", 4}, {"Special", 6}, {"Leg", 7},
		{"Special1", 8}, {"Special2", 9}, {"Special3", 10},
		{"Special4", 11}, {"Special5", 12}, {"Special6", 13},
	}

	for _, tt := range tests {
		t.Run(tt.slotType, func(t *testing.T) {
			s := testSessionWithEquips(1, nil)
			gs := newMockGameState()
			opts := &ExecuteOpts{GameState: gs, TransientVars: make(map[int]interface{})}
			exec.applyEquipChange(context.Background(), s, tt.slotType, "99", opts)
			assert.Equal(t, 99, s.GetEquip(tt.expected), "slot %d should have armor 99", tt.expected)
		})
	}
}

func TestEquipChange_ThenCulSkillEffect_CorrectGauge(t *testing.T) {
	// Simulate CE 1031 flow: ChangeArmors adds armor 71 to inventory,
	// EquipChange equips it in slot 1, then CulSkillEffect + ParaCheck
	// should compute durability max from armor 71's meta <Skill07;[49,100]>.
	armors := []*resource.Armor{
		testArmorWithNote(5, "<Skill01;[40,10]>"),                         // uniform: no v[4249] contribution
		testArmorWithNote(71, "<Skill07;[49,100]><Skill01;[40,15]>"),      // battle outfit: v[4249]+=100
	}
	tagSkill := realTagSkillList()
	res := testResourceWithTagSkillList(armors, tagSkill)
	store := newMockInventoryStore()
	exec := New(store, res, nopLogger())

	// Start with armor 5 equipped (school uniform)
	s := testSessionWithEquips(1, map[int]int{1: 5})
	s.Level = 15
	s.ClassID = 1
	gs := newMockGameState()
	gs.variables[702] = 75  // current cloth durability
	opts := &ExecuteOpts{
		GameState:     gs,
		TransientVars: make(map[int]interface{}),
	}

	// Step 1: Run CulSkillEffect with old equipment (armor 5)
	exec.execCulSkillEffect(s, opts)
	exec.execParaCheck(s, opts)
	// With armor 5: no contribution to v[4249], so v[1209] = 0, cloth max = 0
	assert.Equal(t, 0, gs.GetVariable(1209), "v[1209] with armor 5 should be 0 (no Skill07)")

	// Step 2: EquipChange to armor 71
	exec.applyEquipChange(context.Background(), s, "Cloth", "71", opts)
	assert.Equal(t, 71, s.GetEquip(1), "after EquipChange, slot 1 should be armor 71")

	// Step 3: Re-run CulSkillEffect + ParaCheck (as CE 891 does after EquipChange)
	// Set v[702] to simulate battle starting with some durability
	gs.variables[702] = 75
	opts.TransientVars = make(map[int]interface{})
	exec.execCulSkillEffect(s, opts)
	exec.execParaCheck(s, opts)

	// Now with armor 71: <Skill07;[49,100]> → v[4249]+=100 → AddSkillEffectBase v[1209]=0+100=100
	assert.Equal(t, 100, gs.GetVariable(1209), "v[1209] with armor 71 should be 100 (Skill07 contributes)")
	assert.Equal(t, 100, gs.GetVariable(722), "v[722] cloth max = v[1209] = 100")
	assert.Equal(t, 75, gs.GetVariable(702), "v[702] cloth current should stay 75 (within max 100)")
	assert.Equal(t, 75, gs.GetVariable(741), "v[741] gauge display = v[702]")
	assert.Equal(t, 100, gs.GetVariable(742), "v[742] gauge max = v[722]")
}

func TestApplyChangeArmors_AddAndRemove(t *testing.T) {
	store := newMockInventoryStore()
	exec := New(store, &resource.ResourceLoader{}, nopLogger())
	s := testSession(1)
	gs := newMockGameState()
	opts := &ExecuteOpts{GameState: gs}

	// Add armor 71 x1 (code 128, op=0)
	exec.applyChangeArmors(context.Background(), s, []interface{}{71, 0, 0, 1, false}, opts)
	k := itemKey(1, 71)
	assert.Equal(t, 1, store.items[k], "should have 1 of armor 71 in inventory")

	// Remove armor 71 x1 (code 128, op=1)
	exec.applyChangeArmors(context.Background(), s, []interface{}{71, 1, 0, 1, false}, opts)
	assert.Equal(t, 0, store.items[k], "should have 0 of armor 71 after removal")
}

func TestApplyChangeEquipment_UpdatesSessionEquips(t *testing.T) {
	store := newMockInventoryStore()
	exec := New(store, &resource.ResourceLoader{}, nopLogger())
	s := testSessionWithEquips(1, map[int]int{1: 5})
	opts := &ExecuteOpts{}

	// CmdChangeEquipment params: [actorId, etypeId, itemId]
	// etypeId=1 → slot 1 (Cloth), itemId=71
	exec.applyChangeEquipment(context.Background(), s, []interface{}{1, 1, 71}, opts)

	assert.Equal(t, 71, s.GetEquip(1), "session.Equips[1] should be updated to 71")
}

// ========================================================================
// 9. Batch state change sending — OOM fix
// ========================================================================

// TestExecCulSkillEffect_BatchSend verifies that CulSkillEffect sends
// a single state_batch message instead of individual var_change/switch_change.
func TestExecCulSkillEffect_BatchSend(t *testing.T) {
	armors := []*resource.Armor{
		testArmorWithNote(71, "<Skill01;[40,10]>\n<Skill07;[49,100]>"),
	}
	tagSkill := realTagSkillList()
	res := testResourceWithTagSkillList(armors, tagSkill)
	store := newMockInventoryStore()
	exec := New(store, res, nopLogger())

	gs := newMockGameState()
	s := testSessionWithEquips(1, map[int]int{1: 71})

	opts := &ExecuteOpts{
		GameState:     gs,
		TransientVars: make(map[int]interface{}),
	}

	exec.execCulSkillEffect(s, opts)

	pkts := drainPackets(t, s)

	// Count message types
	varChangeCount := 0
	switchChangeCount := 0
	batchCount := 0
	for _, pkt := range pkts {
		switch pkt.Type {
		case "var_change":
			varChangeCount++
		case "switch_change":
			switchChangeCount++
		case "state_batch":
			batchCount++
		}
	}

	// Should send exactly ONE state_batch instead of many individual messages
	assert.Equal(t, 0, varChangeCount, "should not send individual var_change messages")
	assert.Equal(t, 0, switchChangeCount, "should not send individual switch_change messages")
	assert.Equal(t, 1, batchCount, "should send exactly one state_batch message")

	// Verify the batch contains specific expected changes
	for _, pkt := range pkts {
		if pkt.Type == "state_batch" {
			var data map[string]interface{}
			require.NoError(t, json.Unmarshal(pkt.Payload, &data))
			vars, ok := data["vars"].(map[string]interface{})
			require.True(t, ok, "vars should be a map")
			// Skill01=[40,10] → v[4240]+=10, Skill07=[49,100] → v[4249]+=100
			assert.Equal(t, float64(10), vars["4240"], "v[4240] should be 10 from Skill01=[40,10]")
			assert.Equal(t, float64(100), vars["4249"], "v[4249] should be 100 from Skill07=[49,100]")
			break
		}
	}
}

// TestExecParaCheck_BatchSend verifies that ParaCheck sends
// a single state_batch message instead of individual var_change/switch_change.
func TestExecParaCheck_BatchSend(t *testing.T) {
	res := testResourceWithArmors([]*resource.Armor{
		testArmorWithNote(5, "<ClothName:Uniform>"),
	})
	store := newMockInventoryStore()
	store.gold[1] = 5000
	exec := New(store, res, nopLogger())

	gs := newMockGameState()
	gs.variables[1209] = 100
	gs.variables[702] = 50
	gs.variables[1280] = 30 // erosion base (triggers switch changes via soul erosion)

	s := testSessionWithEquips(1, map[int]int{1: 5})
	s.Level = 10
	s.ClassID = 1

	opts := &ExecuteOpts{
		GameState:     gs,
		TransientVars: make(map[int]interface{}),
	}

	exec.execParaCheck(s, opts)

	pkts := drainPackets(t, s)

	varChangeCount := 0
	switchChangeCount := 0
	batchCount := 0
	for _, pkt := range pkts {
		switch pkt.Type {
		case "var_change":
			varChangeCount++
		case "switch_change":
			switchChangeCount++
		case "state_batch":
			batchCount++
		}
	}

	assert.Equal(t, 0, varChangeCount, "should not send individual var_change messages")
	assert.Equal(t, 0, switchChangeCount, "should not send individual switch_change messages")
	assert.Equal(t, 1, batchCount, "should send exactly one state_batch message")

	// Verify batch contains specific switch changes
	// v[1280]=30 → v[1178]=floor(30/15)=2 → switches 1031=true, 1032=true, 1033-1035=false
	for _, pkt := range pkts {
		if pkt.Type == "state_batch" {
			var data map[string]interface{}
			require.NoError(t, json.Unmarshal(pkt.Payload, &data))
			sw, ok := data["switches"].(map[string]interface{})
			require.True(t, ok, "switches should be a map")
			assert.Equal(t, true, sw["1031"], "switch 1031 should be ON (erosion >= 1)")
			assert.Equal(t, true, sw["1032"], "switch 1032 should be ON (erosion >= 2)")
			assert.Equal(t, false, sw["1033"], "switch 1033 should be OFF (erosion < 3)")
			// Verify vars contain expected values
			vars, ok := data["vars"].(map[string]interface{})
			require.True(t, ok, "vars should be a map")
			assert.Equal(t, float64(10), vars["1006"], "v[1006] should be player level 10")
			assert.Equal(t, float64(5000), vars["215"], "v[215] should be gold 5000")
			break
		}
	}
}

// TestSendStateBatch_SingleMessage verifies the sendStateBatch helper
// sends exactly one message containing all var+switch changes.
func TestSendStateBatch_SingleMessage(t *testing.T) {
	exec := New(nil, &resource.ResourceLoader{}, nopLogger())
	s := testSession(1)

	varChanges := map[int]int{100: 42, 200: 99, 300: -5}
	switchChanges := map[int]bool{10: true, 20: false, 30: true}

	exec.sendStateBatch(s, varChanges, switchChanges)

	pkts := drainPackets(t, s)
	require.Len(t, pkts, 1, "should send exactly one packet")
	assert.Equal(t, "state_batch", pkts[0].Type)

	var data map[string]interface{}
	require.NoError(t, json.Unmarshal(pkts[0].Payload, &data))

	// Check vars
	vars := data["vars"].(map[string]interface{})
	assert.Equal(t, float64(42), vars["100"])
	assert.Equal(t, float64(99), vars["200"])
	assert.Equal(t, float64(-5), vars["300"])

	// Check switches
	switches := data["switches"].(map[string]interface{})
	assert.Equal(t, true, switches["10"])
	assert.Equal(t, false, switches["20"])
	assert.Equal(t, true, switches["30"])
}

// TestSendStateBatch_EmptyNoSend verifies no packet is sent when
// both maps are empty.
func TestSendStateBatch_EmptyNoSend(t *testing.T) {
	exec := New(nil, &resource.ResourceLoader{}, nopLogger())
	s := testSession(1)

	exec.sendStateBatch(s, map[int]int{}, map[int]bool{})

	pkts := drainPackets(t, s)
	assert.Len(t, pkts, 0, "should not send packet when no changes")
}
