//go:build projectb

package integration

import (
	"context"
	"fmt"
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
	idx := level // RMMV: Params[paramID][level] — index 0 is level 0 placeholder
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
	idx := level // RMMV: Params[paramID][level], index 0 is unused placeholder
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

// ---------------------------------------------------------------------------
// Test: Verify buildActorFromClass uses correct level index
// Production code uses idx=level (RMMV 1-indexed); verify test helper matches.
// ---------------------------------------------------------------------------

func TestProjectBBattle_LevelIndexCorrectness(t *testing.T) {
	res := loadProjectBRes(t)

	// Build actor at level 5 using test helper.
	actor := buildActorFromClass(res, 1, "TestHero", 1, 5)
	require.NotNil(t, actor, "class 1 should exist")

	// Build using the same logic as production (idx = level).
	cls := res.ClassByID(1)
	require.NotNil(t, cls)
	prodHP := 0
	if 5 < len(cls.Params[0]) {
		prodHP = cls.Params[0][5] // index = level = 5
	}

	assert.Equal(t, prodHP, actor.Param(0),
		"test helper MaxHP should match production (idx=level=%d)", 5)
	t.Logf("Level 5: MaxHP=%d (params[0][5]=%d)", actor.Param(0), prodHP)
}

// ---------------------------------------------------------------------------
// Test: Damage formula produces expected results with real skill data
// ---------------------------------------------------------------------------

func TestProjectBBattle_DamageFormula(t *testing.T) {
	res := loadProjectBRes(t)

	// Skill 1: formula = "a.atk * 4 - b.def * 2"
	// Class 1 Level 10: ATK=13, DEF=13. Enemy 15: DEF=10.
	// Expected base damage: 13*4 - 10*2 = 52 - 20 = 32 (before variance).
	actor := buildActorFromClass(res, 1, "DmgTest", 1, 10)
	require.NotNil(t, actor)

	troopID := 4
	troop := res.Troops[troopID]
	require.NotNil(t, troop)
	enemyData := res.Enemies[troop.Members[0].EnemyID]
	require.NotNil(t, enemyData)
	enemy := battle.NewEnemyBattler(enemyData, 0, res)

	atk := actor.Param(2)
	def := enemy.Param(3)
	expectedBase := atk*4 - def*2
	t.Logf("ATK=%d, DEF=%d, expected base=%d", atk, def, expectedBase)

	// Run multiple auto-attacks to collect damage values.
	bi := battle.NewBattleInstance(battle.BattleConfig{
		TroopID:      troopID,
		Res:          res,
		RNG:          rand.New(rand.NewSource(42)),
		InputTimeout: 5 * time.Second,
	})
	bi.Actors = []battle.Battler{actor}
	bi.Enemies = []battle.Battler{enemy}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	var damages []int
	go func() {
		for evt := range bi.Events() {
			switch e := evt.(type) {
			case *battle.EventInputRequest:
				bi.SubmitInput(&battle.ActionInput{
					ActorIndex:    0,
					ActionType:    battle.ActionAttack,
					TargetIndices: []int{0},
					TargetIsActor: false,
				})
			case *battle.EventActionResult:
				for _, tgt := range e.Targets {
					if !tgt.Missed && tgt.Target.IsActor == false {
						damages = append(damages, tgt.Damage)
					}
				}
			}
		}
	}()

	bi.Run(ctx)

	require.Greater(t, len(damages), 0, "should have dealt damage")

	// Verify damage is in expected range (base ± 15% variance + possible crit 3x).
	minExpected := int(float64(expectedBase) * 0.85)
	maxExpected := int(float64(expectedBase) * 1.15 * 3.0) // with potential crit
	for i, dmg := range damages {
		assert.GreaterOrEqual(t, dmg, minExpected,
			"damage[%d]=%d should be >= %d (base %d - 15%%)", i, dmg, minExpected, expectedBase)
		assert.LessOrEqual(t, dmg, maxExpected,
			"damage[%d]=%d should be <= %d (base %d + 15%% * 3x crit)", i, dmg, maxExpected, expectedBase)
	}
	t.Logf("Damage values: %v (expected base %d ± 15%%)", damages, expectedBase)
}

// ---------------------------------------------------------------------------
// Test: Skill with MP cost deducts MP correctly
// ---------------------------------------------------------------------------

func TestProjectBBattle_SkillMPCost(t *testing.T) {
	res := loadProjectBRes(t)

	// Skill 71 (光之矢): mpCost=5, scope=1 (single enemy), learned at level 1.
	// Class 1 learns skill 71 at level 1, so it's in the actor's skill list.
	skill71 := res.SkillByID(71)
	require.NotNil(t, skill71, "skill 71 should exist")
	require.Equal(t, 5, skill71.MPCost, "skill 71 MP cost should be 5")

	actor := buildActorFromClass(res, 1, "MagicTest", 1, 10)
	require.NotNil(t, actor)
	startMP := actor.MP()
	require.Greater(t, startMP, skill71.MPCost,
		"actor should have enough MP for skill 71")

	// Verify skill 71 is in actor's skill list.
	hasSkill := false
	for _, sid := range actor.SkillIDs() {
		if sid == 71 {
			hasSkill = true
			break
		}
	}
	require.True(t, hasSkill, "actor at level 10 should know skill 71 (learned at level 1)")

	troopID := 4
	troop := res.Troops[troopID]
	enemyData := res.Enemies[troop.Members[0].EnemyID]
	enemy := battle.NewEnemyBattler(enemyData, 0, res)

	bi := battle.NewBattleInstance(battle.BattleConfig{
		TroopID:      troopID,
		Res:          res,
		RNG:          rand.New(rand.NewSource(42)),
		InputTimeout: 5 * time.Second,
	})
	bi.Actors = []battle.Battler{actor}
	bi.Enemies = []battle.Battler{enemy}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	skillUsed := false
	go func() {
		for evt := range bi.Events() {
			if evt.EventType() == "input_request" {
				if !skillUsed {
					skillUsed = true
					// Use skill 71 on first turn.
					bi.SubmitInput(&battle.ActionInput{
						ActorIndex:    0,
						ActionType:    battle.ActionSkill,
						SkillID:       71,
						TargetIndices: []int{0},
						TargetIsActor: false,
					})
				} else {
					// Normal attack on subsequent turns.
					bi.SubmitInput(&battle.ActionInput{
						ActorIndex:    0,
						ActionType:    battle.ActionAttack,
						TargetIndices: []int{0},
						TargetIsActor: false,
					})
				}
			}
		}
	}()

	bi.Run(ctx)

	// After using skill 71 (MP cost 5), actor's MP should have decreased.
	assert.Less(t, actor.MP(), startMP,
		"actor MP should decrease after using skill 71 (cost=%d, start=%d, now=%d)",
		skill71.MPCost, startMP, actor.MP())
	t.Logf("MP: start=%d, after skill=%d (cost=%d)", startMP, actor.MP(), skill71.MPCost)
}

// ---------------------------------------------------------------------------
// Test: Guard reduces damage taken
// ---------------------------------------------------------------------------

func TestProjectBBattle_GuardReducesDamage(t *testing.T) {
	res := loadProjectBRes(t)

	// Run two 1-turn battles with same seed to compare damage taken per hit.
	// Battle 1: actor attacks (not guarding) → enemy hits back.
	// Battle 2: actor guards → enemy hits back (should be halved).
	troopID := 4
	troop := res.Troops[troopID]
	require.NotNil(t, troop)

	collectEnemyDamage := func(seed int64, guard bool) int {
		actor := buildActorFromClass(res, 1, "GuardTest", 1, 10)
		enemyData := res.Enemies[troop.Members[0].EnemyID]
		enemy := battle.NewEnemyBattler(enemyData, 0, res)

		bi := battle.NewBattleInstance(battle.BattleConfig{
			TroopID:      troopID,
			Res:          res,
			RNG:          rand.New(rand.NewSource(seed)),
			InputTimeout: 5 * time.Second,
		})
		bi.Actors = []battle.Battler{actor}
		bi.Enemies = []battle.Battler{enemy}

		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		totalDmgToActor := 0
		turnCount := 0
		go func() {
			for evt := range bi.Events() {
				switch e := evt.(type) {
				case *battle.EventInputRequest:
					if guard {
						bi.SubmitInput(&battle.ActionInput{
							ActorIndex: e.ActorIndex,
							ActionType: battle.ActionGuard,
						})
					} else {
						bi.SubmitInput(&battle.ActionInput{
							ActorIndex:    e.ActorIndex,
							ActionType:    battle.ActionAttack,
							TargetIndices: []int{0},
							TargetIsActor: false,
						})
					}
				case *battle.EventActionResult:
					for _, tgt := range e.Targets {
						if tgt.Target.IsActor && !tgt.Missed && tgt.Damage > 0 {
							totalDmgToActor += tgt.Damage
						}
					}
				case *battle.EventTurnEnd:
					turnCount++
					if turnCount >= 1 {
						cancel() // stop after 1 turn
					}
				}
			}
		}()

		bi.Run(ctx)
		return totalDmgToActor
	}

	// Use same seed for comparable RNG sequences.
	dmgNoGuard := collectEnemyDamage(42, false)
	dmgWithGuard := collectEnemyDamage(42, true)

	t.Logf("Damage to actor: no guard=%d, with guard=%d", dmgNoGuard, dmgWithGuard)

	// Guard should halve damage (RMMV: GRD = 2x damage rate when guarding).
	// With same RNG seed, the enemy's attack should deal less damage when guarding.
	if dmgNoGuard > 0 {
		assert.Less(t, dmgWithGuard, dmgNoGuard,
			"guard should reduce damage taken (no guard=%d, guard=%d)", dmgNoGuard, dmgWithGuard)
	}
}

// ---------------------------------------------------------------------------
// Test: Enemy AI selects valid actions
// ---------------------------------------------------------------------------

func TestProjectBBattle_EnemyAI(t *testing.T) {
	res := loadProjectBRes(t)

	// Test that enemy AI doesn't crash and produces valid actions.
	for _, troopID := range []int{4, 5, 6} {
		troop := res.Troops[troopID]
		if troop == nil {
			continue
		}
		t.Run(fmt.Sprintf("Troop%d", troopID), func(t *testing.T) {
			actor := buildActorFromClass(res, 1, "AITest", 1, 15)
			require.NotNil(t, actor)

			bi := battle.NewBattleInstance(battle.BattleConfig{
				TroopID:      troopID,
				Res:          res,
				RNG:          rand.New(rand.NewSource(42)),
				InputTimeout: 5 * time.Second,
			})
			bi.Actors = []battle.Battler{actor}
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
			require.Greater(t, len(bi.Enemies), 0)

			// Run battle — verify no panic/deadlock.
			result := runAutoAttackBattle(t, bi, 15*time.Second)
			assert.Contains(t, []int{battle.ResultWin, battle.ResultLose, battle.ResultEscape}, result,
				"battle should end with a valid result")
			t.Logf("Troop %d: result=%d", troopID, result)
		})
	}
}
