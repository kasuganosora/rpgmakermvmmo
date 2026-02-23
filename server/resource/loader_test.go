package resource

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ---- EquipStatsFromParams ----

func TestEquipStatsFromParams_Full(t *testing.T) {
	params := []int{100, 50, 20, 15, 10, 12, 8, 5}
	s := EquipStatsFromParams(params)
	assert.Equal(t, 100, s.MaxHP)
	assert.Equal(t, 50, s.MaxMP)
	assert.Equal(t, 20, s.Atk)
	assert.Equal(t, 15, s.Def)
	assert.Equal(t, 10, s.Mat)
	assert.Equal(t, 12, s.Mdf)
	assert.Equal(t, 8, s.Agi)
	assert.Equal(t, 5, s.Luk)
}

func TestEquipStatsFromParams_Partial(t *testing.T) {
	params := []int{200, 100} // only MaxHP and MaxMP
	s := EquipStatsFromParams(params)
	assert.Equal(t, 200, s.MaxHP)
	assert.Equal(t, 100, s.MaxMP)
	assert.Equal(t, 0, s.Atk)
	assert.Equal(t, 0, s.Def)
}

func TestEquipStatsFromParams_Empty(t *testing.T) {
	s := EquipStatsFromParams(nil)
	assert.Equal(t, EquipStats{}, s)
}

func TestEquipStatsFromParams_SingleElement(t *testing.T) {
	params := []int{500}
	s := EquipStatsFromParams(params)
	assert.Equal(t, 500, s.MaxHP)
	assert.Equal(t, 0, s.MaxMP)
}

// ---- PassabilityMap ----

func newTestPassMap(w, h int) *PassabilityMap {
	return newPassabilityMap(w, h)
}

func TestPassabilityMap_DefaultAllPassable(t *testing.T) {
	pm := newTestPassMap(5, 5)
	for y := 0; y < 5; y++ {
		for x := 0; x < 5; x++ {
			for _, dir := range []int{2, 4, 6, 8} {
				assert.True(t, pm.CanPass(x, y, dir), "(%d,%d) dir=%d should be passable", x, y, dir)
			}
		}
	}
}

func TestPassabilityMap_CanPass_OutOfBounds(t *testing.T) {
	pm := newTestPassMap(3, 3)
	assert.False(t, pm.CanPass(-1, 0, 2))
	assert.False(t, pm.CanPass(0, -1, 2))
	assert.False(t, pm.CanPass(3, 0, 2))
	assert.False(t, pm.CanPass(0, 3, 2))
}

func TestPassabilityMap_CanPass_InvalidDir(t *testing.T) {
	pm := newTestPassMap(3, 3)
	assert.False(t, pm.CanPass(0, 0, 0))   // 0 is invalid
	assert.False(t, pm.CanPass(0, 0, 1))   // 1 is invalid
	assert.False(t, pm.CanPass(0, 0, 99))  // 99 is invalid
}

func TestPassabilityMap_Directions(t *testing.T) {
	pm := newTestPassMap(3, 3)
	// Block down (dir=2) at (1,1)
	pm.data[1][1][0] = false
	assert.False(t, pm.CanPass(1, 1, 2))
	assert.True(t, pm.CanPass(1, 1, 4))
	assert.True(t, pm.CanPass(1, 1, 6))
	assert.True(t, pm.CanPass(1, 1, 8))
}

func TestPassabilityMap_AllDirectionsBlocked(t *testing.T) {
	pm := newTestPassMap(2, 2)
	pm.data[0][0] = [4]bool{false, false, false, false}
	assert.False(t, pm.CanPass(0, 0, 2))
	assert.False(t, pm.CanPass(0, 0, 4))
	assert.False(t, pm.CanPass(0, 0, 6))
	assert.False(t, pm.CanPass(0, 0, 8))
}

// ---- buildPassability layer order regression test ----

// TestBuildPassability_SkipsShadowAndRegionLayers verifies that the passability
// computation only reads tile layers 0-3, matching RMMV's layeredTiles().
// Layer 4 (shadows) and layer 5 (regions) must NOT affect passability.
// Regression test: the server previously iterated ALL layers (0-5) which caused
// shadow/region values to be looked up in tileset flags, producing incorrect results.
func TestBuildPassability_SkipsShadowAndRegionLayers(t *testing.T) {
	// Create a 2x2 map with 6 layers.
	// Layers 0-3: tile 1 (passable, flag=0)
	// Layer 4 (shadow): value 5
	// Layer 5 (region): value 200
	w, h := 2, 2
	data := make([]int, 6*w*h)
	for layer := 0; layer < 4; layer++ {
		for i := 0; i < w*h; i++ {
			data[layer*w*h+i] = 1 // tile 1
		}
	}
	for i := 0; i < w*h; i++ {
		data[4*w*h+i] = 5   // shadow value
		data[5*w*h+i] = 200 // region value
	}

	// Tileset: tile 1 = fully passable (flag 0).
	// tile 5 = fully blocked (flag 0x0F = all direction bits set).
	// tile 200 = fully blocked (flag 0x0F).
	// If shadow/region layers are incorrectly processed, they'd hit tiles 5 or 200
	// and potentially block movement.
	flags := make([]int, 256)
	flags[0] = 0x10 // tile 0: star tile (skipped)
	flags[1] = 0    // tile 1: fully passable
	flags[5] = 0x0F // tile 5: fully blocked (shadow value)
	flags[200] = 0x0F // tile 200: fully blocked (region value)

	rl := NewLoader("", "")
	rl.Tilesets = []*Tileset{{ID: 1, Flags: flags}}
	rl.Maps = map[int]*MapData{
		1: {ID: 1, Width: w, Height: h, TilesetID: 1, Data: data},
	}
	rl.buildPassability()

	pm := rl.Passability[1]
	require.NotNil(t, pm)

	// All tiles should be passable — shadow (5) and region (200) must NOT be
	// checked against tileset flags.
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			for _, dir := range []int{2, 4, 6, 8} {
				assert.True(t, pm.CanPass(x, y, dir),
					"(%d,%d) dir=%d should be passable (shadow/region must not affect passability)", x, y, dir)
			}
		}
	}
}

// TestBuildPassability_BottomLayerFirst verifies that RMMV's layer iteration
// order is correct: layer 0 (bottom) is checked first, not layer 3 (top).
// The first non-star tile determines passability.
func TestBuildPassability_BottomLayerFirst(t *testing.T) {
	w, h := 1, 1
	// Layer 0: tile 1 (passable, flag=0)
	// Layer 1: tile 2 (blocked, flag=0x0F)
	// Layer 2-3: tile 0 (star, flag=0x10, skipped)
	data := make([]int, 4*w*h) // 4 layers only
	data[0] = 1 // layer 0: passable tile
	data[1] = 2 // layer 1: blocked tile
	data[2] = 0 // layer 2: star tile
	data[3] = 0 // layer 3: star tile

	flags := make([]int, 16)
	flags[0] = 0x10 // star tile (skipped)
	flags[1] = 0    // fully passable
	flags[2] = 0x0F // fully blocked

	rl := NewLoader("", "")
	rl.Tilesets = []*Tileset{{ID: 1, Flags: flags}}
	rl.Maps = map[int]*MapData{
		1: {ID: 1, Width: w, Height: h, TilesetID: 1, Data: data},
	}
	rl.buildPassability()

	pm := rl.Passability[1]
	require.NotNil(t, pm)

	// Layer 0 (tile 1, passable) should win since it's checked first.
	// If the server incorrectly iterates top-to-bottom, layer 1 (blocked) would win.
	for _, dir := range []int{2, 4, 6, 8} {
		assert.True(t, pm.CanPass(0, 0, dir),
			"dir=%d: layer 0 (passable) should take priority over layer 1 (blocked)", dir)
	}
}

// ---- ResourceLoader construction ----

func TestNewLoader(t *testing.T) {
	rl := NewLoader("/data", "/img")
	require.NotNil(t, rl)
	assert.Equal(t, "/data", rl.DataPath)
	assert.Equal(t, "/img", rl.ImgPath)
	assert.NotNil(t, rl.Maps)
	assert.NotNil(t, rl.Passability)
}

func TestLoader_Load_InvalidPath(t *testing.T) {
	rl := NewLoader("/nonexistent/path", "")
	err := rl.Load()
	assert.Error(t, err)
}

func TestLoader_ValidWalkName_NoImgPath(t *testing.T) {
	rl := NewLoader("", "")
	assert.True(t, rl.ValidWalkName("anything")) // no img path → always true
}

func TestLoader_ValidFaceName_NoImgPath(t *testing.T) {
	rl := NewLoader("", "")
	assert.True(t, rl.ValidFaceName("anything"))
}

func TestLoader_ClassByID_NotFound(t *testing.T) {
	rl := NewLoader("", "")
	rl.Classes = []*Class{{ID: 1, Name: "Warrior"}}
	assert.Nil(t, rl.ClassByID(99))
}

func TestLoader_ClassByID_Found(t *testing.T) {
	rl := NewLoader("", "")
	rl.Classes = []*Class{nil, {ID: 1, Name: "Warrior"}, {ID: 2, Name: "Mage"}}
	c := rl.ClassByID(2)
	require.NotNil(t, c)
	assert.Equal(t, "Mage", c.Name)
}
