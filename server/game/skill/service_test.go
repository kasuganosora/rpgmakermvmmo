package skill

import (
	"context"
	"testing"
	"time"

	"github.com/kasuganosora/rpgmakermvmmo/server/cache"
	"github.com/kasuganosora/rpgmakermvmmo/server/game/player"
	"github.com/kasuganosora/rpgmakermvmmo/server/game/world"
	"github.com/kasuganosora/rpgmakermvmmo/server/resource"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

func newTestCache(t *testing.T) cache.Cache {
	t.Helper()
	c, err := cache.NewCache(cache.CacheConfig{})
	require.NoError(t, err)
	return c
}

func testLogger() *zap.Logger {
	l, _ := zap.NewDevelopment()
	return l
}

func newTestSession(charID int64) *player.PlayerSession {
	return &player.PlayerSession{
		CharID:   charID,
		HP:       100,
		MaxHP:    100,
		MP:       50,
		MaxMP:    50,
		SendChan: make(chan []byte, 256),
		Done:     make(chan struct{}),
	}
}

// ---- IsOnCooldown ----

func TestIsOnCooldown_NotSet(t *testing.T) {
	c := newTestCache(t)
	svc := NewSkillService(c, nil, nil, testLogger())

	onCD, err := svc.IsOnCooldown(context.Background(), 1, 1)
	require.NoError(t, err)
	assert.False(t, onCD)
}

func TestIsOnCooldown_SetAndActive(t *testing.T) {
	c := newTestCache(t)
	svc := NewSkillService(c, nil, nil, testLogger())

	// Set a 10-second cooldown
	require.NoError(t, svc.SetCooldown(context.Background(), 1, 1, 10000))

	onCD, err := svc.IsOnCooldown(context.Background(), 1, 1)
	require.NoError(t, err)
	assert.True(t, onCD)
}

func TestIsOnCooldown_Expired(t *testing.T) {
	c := newTestCache(t)
	svc := NewSkillService(c, nil, nil, testLogger())

	// Set a cooldown that already ended (negative ms → readyAt in the past)
	require.NoError(t, svc.SetCooldown(context.Background(), 1, 1, -1000))

	onCD, err := svc.IsOnCooldown(context.Background(), 1, 1)
	require.NoError(t, err)
	assert.False(t, onCD)
}

func TestIsOnCooldown_DifferentSkills(t *testing.T) {
	c := newTestCache(t)
	svc := NewSkillService(c, nil, nil, testLogger())

	// Set cooldown for skill 1 but not skill 2
	require.NoError(t, svc.SetCooldown(context.Background(), 1, 1, 10000))

	onCD1, _ := svc.IsOnCooldown(context.Background(), 1, 1)
	onCD2, _ := svc.IsOnCooldown(context.Background(), 1, 2)
	assert.True(t, onCD1)
	assert.False(t, onCD2)
}

func TestIsOnCooldown_DifferentPlayers(t *testing.T) {
	c := newTestCache(t)
	svc := NewSkillService(c, nil, nil, testLogger())

	// Cooldown for player 1, not player 2
	require.NoError(t, svc.SetCooldown(context.Background(), 1, 5, 10000))

	onCD1, _ := svc.IsOnCooldown(context.Background(), 1, 5)
	onCD2, _ := svc.IsOnCooldown(context.Background(), 2, 5)
	assert.True(t, onCD1)
	assert.False(t, onCD2)
}

// ---- SetCooldown ----

func TestSetCooldown_SetsValue(t *testing.T) {
	c := newTestCache(t)
	svc := NewSkillService(c, nil, nil, testLogger())

	err := svc.SetCooldown(context.Background(), 10, 3, 5000)
	require.NoError(t, err)

	// The key should exist in the hash
	val, err := c.HGet(context.Background(), cdKey(10), "3")
	require.NoError(t, err)
	assert.NotEmpty(t, val)
}

func TestSetCooldown_UpdatesValue(t *testing.T) {
	c := newTestCache(t)
	svc := NewSkillService(c, nil, nil, testLogger())

	// First set
	require.NoError(t, svc.SetCooldown(context.Background(), 1, 1, 1000))
	val1, _ := c.HGet(context.Background(), cdKey(1), "1")

	// Give a tiny sleep so the timestamp changes
	time.Sleep(2 * time.Millisecond)

	// Update
	require.NoError(t, svc.SetCooldown(context.Background(), 1, 1, 2000))
	val2, _ := c.HGet(context.Background(), cdKey(1), "1")

	// The second value should be greater (further in the future)
	assert.NotEqual(t, val1, val2)
}

// ---- UseSkill ----

func TestUseSkill_NilResources(t *testing.T) {
	c := newTestCache(t)
	svc := NewSkillService(c, nil, nil, testLogger())

	s := newTestSession(1)
	err := svc.UseSkill(context.Background(), s, 1, 0, "monster")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "resources not loaded")
}

// newTestResWithSkill creates a minimal ResourceLoader with a single skill.
func newTestResWithSkill(skillID, mpCost int) *resource.ResourceLoader {
	// RMMV arrays are 1-indexed (index 0 is null)
	skills := make([]*resource.Skill, skillID+1)
	skills[skillID] = &resource.Skill{ID: skillID, Name: "TestSkill", MPCost: mpCost}
	return &resource.ResourceLoader{
		Skills: skills,
	}
}

func TestUseSkill_UnknownSkillID(t *testing.T) {
	c := newTestCache(t)
	logger := testLogger()
	res := newTestResWithSkill(1, 5)
	wm := world.NewWorldManager(nil, world.NewGameState(nil), world.NewGlobalWhitelist(), nil, logger)
	defer wm.StopAll()

	svc := NewSkillService(c, res, wm, logger)
	s := newTestSession(1)
	s.MapID = 1

	err := svc.UseSkill(context.Background(), s, 999, 0, "monster")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unknown skill_id")
}

func TestUseSkill_OnCooldown(t *testing.T) {
	c := newTestCache(t)
	logger := testLogger()
	res := newTestResWithSkill(1, 5)
	wm := world.NewWorldManager(nil, world.NewGameState(nil), world.NewGlobalWhitelist(), nil, logger)
	defer wm.StopAll()

	svc := NewSkillService(c, res, wm, logger)
	s := newTestSession(1)
	s.MapID = 1
	s.MP = 50

	// Pre-set cooldown
	require.NoError(t, svc.SetCooldown(context.Background(), s.CharID, 1, 60000))

	err := svc.UseSkill(context.Background(), s, 1, 0, "monster")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "skill still on cooldown")
}

func TestUseSkill_NotEnoughMP(t *testing.T) {
	c := newTestCache(t)
	logger := testLogger()
	res := newTestResWithSkill(1, 50)
	wm := world.NewWorldManager(nil, world.NewGameState(nil), world.NewGlobalWhitelist(), nil, logger)
	defer wm.StopAll()

	svc := NewSkillService(c, res, wm, logger)
	s := newTestSession(1)
	s.MapID = 1
	s.MP = 10 // less than MPCost=50

	err := svc.UseSkill(context.Background(), s, 1, 0, "monster")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not enough MP")
}

func TestUseSkill_NotInMap(t *testing.T) {
	c := newTestCache(t)
	logger := testLogger()
	res := newTestResWithSkill(1, 5)
	wm := world.NewWorldManager(nil, world.NewGameState(nil), world.NewGlobalWhitelist(), nil, logger)
	defer wm.StopAll()
	// Do NOT create map 999

	svc := NewSkillService(c, res, wm, logger)
	s := newTestSession(1)
	s.MapID = 999 // non-existent map
	s.MP = 50

	err := svc.UseSkill(context.Background(), s, 1, 0, "monster")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not in a map")
}

func TestUseSkill_Success_NoTargets(t *testing.T) {
	c := newTestCache(t)
	logger := testLogger()
	res := newTestResWithSkill(1, 5)
	wm := world.NewWorldManager(nil, world.NewGameState(nil), world.NewGlobalWhitelist(), nil, logger)
	defer wm.StopAll()
	wm.GetOrCreate(1) // ensure the map room exists

	svc := NewSkillService(c, res, wm, logger)
	s := newTestSession(1)
	s.MapID = 1
	s.MP = 50

	err := svc.UseSkill(context.Background(), s, 1, 0, "monster")
	require.NoError(t, err)
	// MP deducted by MPCost=5
	assert.Equal(t, 45, s.MP)
}
