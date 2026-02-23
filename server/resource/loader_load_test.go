package resource

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// writeJSON writes v as JSON to path/filename.
func writeJSON(t *testing.T, dir, filename string, v interface{}) {
	t.Helper()
	data, err := json.Marshal(v)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(dir, filename), data, 0644))
}

// setupMinimalDataDir creates a temp directory with the minimal set of RMMV JSON files
// required for ResourceLoader.Load() to succeed.
func setupMinimalDataDir(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()

	// System.json
	writeJSON(t, dir, "System.json", map[string]interface{}{
		"gameTitle":    "TestGame",
		"currencyUnit": "G",
		"startMapId":   1,
		"startX":       0,
		"startY":       0,
	})

	nullArray := []interface{}{nil}

	// All array files: first element is null (RMMV convention, ID 0 unused).
	for _, name := range []string{
		"Actors.json", "Classes.json", "Skills.json", "Items.json",
		"Weapons.json", "Armors.json", "Enemies.json", "Troops.json",
		"States.json", "Animations.json", "MapInfos.json",
		"Tilesets.json", "CommonEvents.json",
	} {
		writeJSON(t, dir, name, nullArray)
	}

	return dir
}

// ---- Load() success path ----

func TestLoader_Load_Success(t *testing.T) {
	dir := setupMinimalDataDir(t)
	rl := NewLoader(dir, "")
	err := rl.Load()
	require.NoError(t, err)

	require.NotNil(t, rl.System)
	assert.Equal(t, "TestGame", rl.System.GameTitle)
	assert.Equal(t, "G", rl.System.CurrencyUnit)
}

func TestLoader_Load_PopulatesCollections(t *testing.T) {
	dir := setupMinimalDataDir(t)

	// Add one actor and one class
	writeJSON(t, dir, "Actors.json", []*Actor{
		nil,
		{ID: 1, Name: "Hero", ClassID: 1},
	})
	writeJSON(t, dir, "Classes.json", []*Class{
		nil,
		{ID: 1, Name: "Fighter", Params: [][]int{}},
	})

	rl := NewLoader(dir, "")
	require.NoError(t, rl.Load())

	require.Len(t, rl.Actors, 2)
	assert.Equal(t, "Hero", rl.Actors[1].Name)
	require.Len(t, rl.Classes, 2)
	assert.Equal(t, "Fighter", rl.Classes[1].Name)
}

func TestLoader_Load_LoadsMap(t *testing.T) {
	dir := setupMinimalDataDir(t)

	// Write a minimal map file
	writeJSON(t, dir, "Map001.json", map[string]interface{}{
		"id":          1,
		"displayName": "Village",
		"width":       4,
		"height":      4,
		"data":        make([]int, 4*4),
		"tilesetId":   1,
		"events":      []interface{}{},
	})

	rl := NewLoader(dir, "")
	require.NoError(t, rl.Load())

	md, ok := rl.Maps[1]
	require.True(t, ok)
	assert.Equal(t, "Village", md.DisplayName)
	assert.Equal(t, 4, md.Width)
	assert.Equal(t, 4, md.Height)
}

func TestLoader_Load_BuildsPassability(t *testing.T) {
	dir := setupMinimalDataDir(t)

	// Write a tileset with passability flags
	writeJSON(t, dir, "Tilesets.json", []*Tileset{
		nil,
		{ID: 1, Name: "Base", Flags: []int{0, 0x0F, 0x01, 0x00}},
	})

	// Write a map that uses tileset 1
	writeJSON(t, dir, "Map001.json", map[string]interface{}{
		"id":          1,
		"displayName": "TestMap",
		"width":       2,
		"height":      2,
		"data":        []int{0, 1, 2, 3}, // one layer, 2x2
		"tilesetId":   1,
		"events":      []interface{}{},
	})

	rl := NewLoader(dir, "")
	require.NoError(t, rl.Load())

	pm, ok := rl.Passability[1]
	require.True(t, ok)
	require.NotNil(t, pm)
	assert.Equal(t, 2, pm.Width)
	assert.Equal(t, 2, pm.Height)
}

func TestLoader_Load_MapNoTileset(t *testing.T) {
	dir := setupMinimalDataDir(t)

	// Map references tilesetId=99 which doesn't exist → all passable by default
	writeJSON(t, dir, "Map002.json", map[string]interface{}{
		"id":          2,
		"displayName": "NoTileMap",
		"width":       3,
		"height":      3,
		"data":        make([]int, 9),
		"tilesetId":   99,
		"events":      []interface{}{},
	})

	rl := NewLoader(dir, "")
	require.NoError(t, rl.Load())

	pm, ok := rl.Passability[2]
	require.True(t, ok)
	// All tiles passable
	assert.True(t, pm.CanPass(0, 0, 2))
	assert.True(t, pm.CanPass(0, 0, 4))
}

// ---- Load() failure paths ----

func TestLoader_Load_MissingSystemJSON(t *testing.T) {
	dir := t.TempDir()
	// Don't write System.json
	rl := NewLoader(dir, "")
	err := rl.Load()
	assert.Error(t, err)
}

func TestLoader_Load_InvalidJSON(t *testing.T) {
	dir := t.TempDir()
	// Write invalid JSON for System.json
	require.NoError(t, os.WriteFile(filepath.Join(dir, "System.json"), []byte("not json"), 0644))
	rl := NewLoader(dir, "")
	err := rl.Load()
	assert.Error(t, err)
}

func TestLoader_Load_InvalidActorsJSON(t *testing.T) {
	dir := t.TempDir()
	// Write valid System.json but invalid Actors.json
	writeJSON(t, dir, "System.json", map[string]interface{}{
		"gameTitle": "Test", "currencyUnit": "G", "startMapId": 1,
	})
	require.NoError(t, os.WriteFile(filepath.Join(dir, "Actors.json"), []byte("{not array}"), 0644))
	rl := NewLoader(dir, "")
	err := rl.Load()
	assert.Error(t, err)
}

// ---- ClassByID with loaded data ----

func TestClassByID_AfterLoad(t *testing.T) {
	dir := setupMinimalDataDir(t)
	writeJSON(t, dir, "Classes.json", []*Class{
		nil,
		{ID: 1, Name: "Warrior"},
		{ID: 2, Name: "Mage"},
	})

	rl := NewLoader(dir, "")
	require.NoError(t, rl.Load())

	c1 := rl.ClassByID(1)
	require.NotNil(t, c1)
	assert.Equal(t, "Warrior", c1.Name)

	c2 := rl.ClassByID(2)
	require.NotNil(t, c2)
	assert.Equal(t, "Mage", c2.Name)

	assert.Nil(t, rl.ClassByID(99))
}

// ---- ValidWalkName / ValidFaceName with img path ----

func TestValidWalkName_FileExists(t *testing.T) {
	imgDir := t.TempDir()
	charDir := filepath.Join(imgDir, "characters")
	require.NoError(t, os.MkdirAll(charDir, 0755))
	require.NoError(t, os.WriteFile(filepath.Join(charDir, "Hero.png"), []byte{}, 0644))

	rl := NewLoader("", imgDir)
	assert.True(t, rl.ValidWalkName("Hero"))
	assert.False(t, rl.ValidWalkName("Missing"))
}

func TestValidFaceName_FileExists(t *testing.T) {
	imgDir := t.TempDir()
	faceDir := filepath.Join(imgDir, "faces")
	require.NoError(t, os.MkdirAll(faceDir, 0755))
	require.NoError(t, os.WriteFile(filepath.Join(faceDir, "Face1.png"), []byte{}, 0644))

	rl := NewLoader("", imgDir)
	assert.True(t, rl.ValidFaceName("Face1"))
	assert.False(t, rl.ValidFaceName("Missing"))
}

// ---- buildPassability edge cases ----

func TestBuildPassability_FullyImpassableTile(t *testing.T) {
	dir := setupMinimalDataDir(t)

	// Tileset with a fully impassable tile (0x0F)
	writeJSON(t, dir, "Tilesets.json", []*Tileset{
		nil,
		{ID: 1, Name: "Test", Flags: []int{0, 0x0F}},
	})
	writeJSON(t, dir, "Map001.json", map[string]interface{}{
		"id": 1, "displayName": "Wall", "width": 2, "height": 1,
		"data": []int{1, 0}, "tilesetId": 1, "events": []interface{}{},
	})

	rl := NewLoader(dir, "")
	require.NoError(t, rl.Load())

	pm := rl.Passability[1]
	require.NotNil(t, pm)
	// Tile at (0,0) has tileID=1 with flag=0x0F → fully impassable
	assert.False(t, pm.CanPass(0, 0, 2))
	assert.False(t, pm.CanPass(0, 0, 4))
	assert.False(t, pm.CanPass(0, 0, 6))
	assert.False(t, pm.CanPass(0, 0, 8))
	// Tile at (1,0) has tileID=0 → passable
	assert.True(t, pm.CanPass(1, 0, 2))
}

func TestBuildPassability_DirectionalBlocking(t *testing.T) {
	dir := setupMinimalDataDir(t)

	// Flag 0x01 = down blocked, 0x02 = left blocked, 0x04 = right blocked, 0x08 = up blocked
	writeJSON(t, dir, "Tilesets.json", []*Tileset{
		nil,
		{ID: 1, Name: "Test", Flags: []int{0, 0x01, 0x02, 0x04, 0x08}},
	})
	writeJSON(t, dir, "Map001.json", map[string]interface{}{
		"id": 1, "displayName": "Dirs", "width": 5, "height": 1,
		"data": []int{1, 2, 3, 4, 0}, "tilesetId": 1, "events": []interface{}{},
	})

	rl := NewLoader(dir, "")
	require.NoError(t, rl.Load())

	pm := rl.Passability[1]
	require.NotNil(t, pm)

	// tileID=1 (flag=0x01): down blocked
	assert.False(t, pm.CanPass(0, 0, 2)) // down
	assert.True(t, pm.CanPass(0, 0, 4))  // left passable
	assert.True(t, pm.CanPass(0, 0, 6))  // right passable
	assert.True(t, pm.CanPass(0, 0, 8))  // up passable

	// tileID=2 (flag=0x02): left blocked
	assert.True(t, pm.CanPass(1, 0, 2))
	assert.False(t, pm.CanPass(1, 0, 4)) // left
	assert.True(t, pm.CanPass(1, 0, 6))
	assert.True(t, pm.CanPass(1, 0, 8))
}
