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
