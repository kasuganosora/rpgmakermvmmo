//go:build projectb

package integration

import (
	"context"
	"math/rand"
	"testing"
	"time"

	"github.com/kasuganosora/rpgmakermvmmo/server/game/battle"
	"github.com/kasuganosora/rpgmakermvmmo/server/resource"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

// loadProjectBRes loads ProjectB resources for battle tests.
func loadProjectBRes(t *testing.T) *resource.ResourceLoader {
	t.Helper()
	dataPath := projectBDataPath(t)
	res := resource.NewLoader(dataPath, "")
	require.NoError(t, res.Load(), "failed to load resources")
	return res
}

// buildActorFromClass creates an ActorBattler using real class data.
func buildActorFromClass(res *resource.ResourceLoader, charID int64, name string, classID, level int) *battle.ActorBattler {
	cls := res.ClassByID(classID)
	if cls == nil {
		return nil
	}

	var baseParams [8]int
	idx := level - 1
	if idx < 0 {
		idx = 0
	}
	for p := 0; p < 8; p++ {
		if p < len(cls.Params) && idx < len(cls.Params[p]) {
			baseParams[p] = cls.Params[p][idx]
		}
	}

	skills := res.SkillsForLevel(classID, level)

	return battle.NewActorBattler(battle.ActorConfig{
		CharID:      charID,
		Name:        name,
		Index:       0,
		ClassID:     classID,
		Level:       level,
		HP:          baseParams[0], // start at full HP
		MP:          baseParams[1],
		BaseParams:  baseParams,
		Skills:      skills,
		ActorTraits: cls.Traits,
		Res:         res,
	})
}

// runAutoAttackBattle runs a battle where actors always attack enemy 0.
// Returns the battle result.
func runAutoAttackBattle(t *testing.T, bi *battle.BattleInstance, timeout time.Duration) int {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	go func() {
		for evt := range bi.Events() {
			if evt.EventType() == "input_request" {
				bi.SubmitInput(&battle.ActionInput{
					ActorIndex:    0,
					ActionType:    battle.ActionAttack,
					TargetIndices: []int{0},
					TargetIsActor: false,
				})
			}
		}
	}()

	return bi.Run(ctx)
}

// ---------------------------------------------------------------------------
// Tests
// ---------------------------------------------------------------------------

// TestProjectBBattle_TroopBasic verifies the battle engine works with real
// ProjectB data: class params, enemy stats, skill formulas, traits.
// Uses Troop 4 (enemy #15: HP=100 ATK=10) vs Class 1 Level 5 hero.
func TestProjectBBattle_TroopBasic(t *testing.T) {
	res := loadProjectBRes(t)

	troopID := 4
	require.True(t, troopID < len(res.Troops) && res.Troops[troopID] != nil,
		"troop %d should exist", troopID)

	actor := buildActorFromClass(res, 1, "TestHero", 1, 5)
	require.NotNil(t, actor, "class 1 should exist")

	bi := battle.NewBattleInstance(battle.BattleConfig{
		TroopID:      troopID,
		Res:          res,
		RNG:          rand.New(rand.NewSource(42)),
		InputTimeout: 5 * time.Second,
	})
	bi.Actors = []battle.Battler{actor}

	// Build enemies from real troop data.
	troop := res.Troops[troopID]
	for idx, member := range troop.Members {
		require.True(t, member.EnemyID > 0 && member.EnemyID < len(res.Enemies),
			"enemy ID %d should be valid", member.EnemyID)
		enemyData := res.Enemies[member.EnemyID]
		require.NotNil(t, enemyData, "enemy %d data should exist", member.EnemyID)
		eb := battle.NewEnemyBattler(enemyData, idx, res)
		bi.Enemies = append(bi.Enemies, eb)
	}
	require.True(t, len(bi.Enemies) > 0, "troop should have enemies")

	t.Logf("Battle: %s (HP=%d ATK=%d) vs %d enemies (HP=%d ATK=%d)",
		actor.Name(), actor.HP(), actor.Param(2),
		len(bi.Enemies), bi.Enemies[0].HP(), bi.Enemies[0].Param(2))

	result := runAutoAttackBattle(t, bi, 30*time.Second)
	assert.Equal(t, battle.ResultWin, result, "hero at level 5 should beat weak enemy")
	assert.True(t, actor.IsAlive(), "hero should survive")
}

// TestProjectBBattle_HighLevelSweep verifies a high-level hero one-shots
// weak enemies (verifies damage formula scales correctly with real data).
func TestProjectBBattle_HighLevelSweep(t *testing.T) {
	res := loadProjectBRes(t)

	troopID := 5 // enemy #5: HP=100 ATK=10
	require.True(t, troopID < len(res.Troops) && res.Troops[troopID] != nil,
		"troop %d should exist", troopID)

	// Level 30 hero should massively outscale.
	actor := buildActorFromClass(res, 1, "OverpoweredHero", 1, 30)
	require.NotNil(t, actor)

	bi := battle.NewBattleInstance(battle.BattleConfig{
		TroopID:      troopID,
		Res:          res,
		RNG:          rand.New(rand.NewSource(99)),
		InputTimeout: 5 * time.Second,
	})
	bi.Actors = []battle.Battler{actor}

	troop := res.Troops[troopID]
	for idx, member := range troop.Members {
		if member.EnemyID <= 0 || member.EnemyID >= len(res.Enemies) {
			continue
		}
		enemyData := res.Enemies[member.EnemyID]
		if enemyData == nil {
			continue
		}
		bi.Enemies = append(bi.Enemies, battle.NewEnemyBattler(enemyData, idx, res))
	}
	require.True(t, len(bi.Enemies) > 0)

	t.Logf("Battle: %s (HP=%d ATK=%d) vs enemy (HP=%d DEF=%d)",
		actor.Name(), actor.HP(), actor.Param(2),
		bi.Enemies[0].HP(), bi.Enemies[0].Param(3))

	result := runAutoAttackBattle(t, bi, 10*time.Second)
	assert.Equal(t, battle.ResultWin, result)

	// Hero should not have taken much damage — at level 30 they should
	// kill the enemy in 1 turn before it can act (higher AGI).
	assert.True(t, actor.HP() > actor.MaxHP()/2,
		"hero should have >50%% HP remaining, got %d/%d", actor.HP(), actor.MaxHP())
}

// TestProjectBBattle_Escape verifies escape mechanics with real data.
func TestProjectBBattle_Escape(t *testing.T) {
	res := loadProjectBRes(t)

	troopID := 6 // enemy #13: HP=100
	require.True(t, troopID < len(res.Troops) && res.Troops[troopID] != nil,
		"troop %d should exist", troopID)

	actor := buildActorFromClass(res, 1, "Runner", 1, 10)
	require.NotNil(t, actor)

	bi := battle.NewBattleInstance(battle.BattleConfig{
		TroopID:      troopID,
		CanEscape:    true,
		Res:          res,
		RNG:          rand.New(rand.NewSource(42)),
		InputTimeout: 5 * time.Second,
	})
	bi.Actors = []battle.Battler{actor}

	troop := res.Troops[troopID]
	for idx, member := range troop.Members {
		if member.EnemyID <= 0 || member.EnemyID >= len(res.Enemies) {
			continue
		}
		enemyData := res.Enemies[member.EnemyID]
		if enemyData == nil {
			continue
		}
		bi.Enemies = append(bi.Enemies, battle.NewEnemyBattler(enemyData, idx, res))
	}
	require.True(t, len(bi.Enemies) > 0)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Always try to escape.
	go func() {
		for evt := range bi.Events() {
			if evt.EventType() == "input_request" {
				bi.SubmitInput(&battle.ActionInput{
					ActorIndex: 0,
					ActionType: battle.ActionEscape,
				})
			}
		}
	}()

	result := bi.Run(ctx)
	// Should eventually escape (probability increases each attempt).
	assert.Equal(t, battle.ResultEscape, result, "should eventually escape")
}

// TestProjectBBattle_WithRealSession verifies the battle engine works
// with a player session created via the full test server stack.
func TestProjectBBattle_WithRealSession(t *testing.T) {
	dataPath := projectBDataPath(t)
	ts := NewTestServerWithResources(t, dataPath)

	// Create user + character.
	user := UniqueID("battle")
	token, _ := ts.Login(t, user, user+"pass")
	charID := ts.CreateCharacter(t, token, UniqueID("fighter"), 1)
	ws := ts.ConnectWS(t, token)
	defer ws.Close()

	// Enter map and wait for map_init.
	ws.Send("enter_map", map[string]interface{}{"char_id": charID})
	initPkt := ws.RecvType("map_init", 10*time.Second)
	require.NotNil(t, initPkt, "should get map_init")
	ws.Send("scene_ready", map[string]interface{}{})

	// Wait for autoruns to settle.
	time.Sleep(2 * time.Second)

	// Get the player session.
	sess := ts.SM.Get(charID)
	require.NotNil(t, sess, "player session should exist")

	// Build actor from session's class data at a combat-viable level.
	// Freshly created characters start at level 1 with minimal stats,
	// so we use level 10 to have meaningful combat.
	cls := ts.Res.ClassByID(sess.ClassID)
	require.NotNil(t, cls, "class should exist")

	level := 10
	var baseParams [8]int
	idx := level - 1
	for p := 0; p < 8 && p < len(cls.Params); p++ {
		if idx < len(cls.Params[p]) {
			baseParams[p] = cls.Params[p][idx]
		}
	}
	hp := baseParams[0]
	mp := baseParams[1]

	troopID := 4
	require.True(t, troopID < len(ts.Res.Troops) && ts.Res.Troops[troopID] != nil)

	bi := battle.NewBattleInstance(battle.BattleConfig{
		TroopID:      troopID,
		Res:          ts.Res,
		RNG:          rand.New(rand.NewSource(42)),
		InputTimeout: 5 * time.Second,
	})

	actor := battle.NewActorBattler(battle.ActorConfig{
		CharID:      sess.CharID,
		Name:        sess.CharName,
		Index:       0,
		ClassID:     sess.ClassID,
		Level:       level,
		HP:          hp,
		MP:          mp,
		BaseParams:  baseParams,
		ActorTraits: cls.Traits,
		Skills:      ts.Res.SkillsForLevel(sess.ClassID, level),
		Res:         ts.Res,
	})
	bi.Actors = []battle.Battler{actor}

	troop := ts.Res.Troops[troopID]
	for i, member := range troop.Members {
		if member.EnemyID <= 0 || member.EnemyID >= len(ts.Res.Enemies) {
			continue
		}
		enemyData := ts.Res.Enemies[member.EnemyID]
		if enemyData == nil {
			continue
		}
		bi.Enemies = append(bi.Enemies, battle.NewEnemyBattler(enemyData, i, ts.Res))
	}
	require.True(t, len(bi.Enemies) > 0)

	t.Logf("Battle via session: %s (charID=%d, class=%d, level=%d, HP=%d, ATK=%d) vs %d enemies",
		sess.CharName, sess.CharID, sess.ClassID, level, hp, actor.Param(2),
		len(bi.Enemies))

	result := runAutoAttackBattle(t, bi, 30*time.Second)
	assert.Equal(t, battle.ResultWin, result, "player should beat weak enemy")
}
