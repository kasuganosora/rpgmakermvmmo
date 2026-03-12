package world

import (
	"testing"

	"github.com/kasuganosora/rpgmakermvmmo/server/game/ai"
)

// helper to create a minimal MonsterRuntime for group tests.
func testMonster(spawnID, x, y, hp int) *MonsterRuntime {
	return &MonsterRuntime{
		InstID:  int64(spawnID*100 + 1),
		SpawnID: spawnID,
		HP:      hp,
		MaxHP:   hp,
		X:       x,
		Y:       y,
		SpawnX:  x,
		SpawnY:  y,
		State:   ai.StateIdle,
		Threat:  ai.NewThreatTable(),
	}
}

// ---- GroupManager basic tests ----

func TestGroupManager_RegisterAndGet(t *testing.T) {
	gm := NewGroupManager()
	m := testMonster(0, 10, 10, 100)
	gm.Register(m.InstID, "goblins", "assist", m)

	g := gm.GetGroup("goblins")
	if g == nil {
		t.Fatal("expected group to exist")
	}
	if g.GroupType != "assist" {
		t.Errorf("expected assist, got %s", g.GroupType)
	}
	if len(g.Members) != 1 {
		t.Errorf("expected 1 member, got %d", len(g.Members))
	}
}

func TestGroupManager_RegisterDefaultsToAssist(t *testing.T) {
	gm := NewGroupManager()
	m := testMonster(0, 10, 10, 100)
	gm.Register(m.InstID, "goblins", "", m)

	g := gm.GetGroup("goblins")
	if g.GroupType != "assist" {
		t.Errorf("expected default assist, got %s", g.GroupType)
	}
}

func TestGroupManager_EmptyGroupID(t *testing.T) {
	gm := NewGroupManager()
	m := testMonster(0, 10, 10, 100)
	gm.Register(m.InstID, "", "assist", m)
	if len(gm.groups) != 0 {
		t.Error("empty groupID should not create group")
	}
}

func TestGroupManager_Unregister(t *testing.T) {
	gm := NewGroupManager()
	m1 := testMonster(0, 10, 10, 100)
	m2 := testMonster(1, 12, 10, 100)
	gm.Register(m1.InstID, "goblins", "assist", m1)
	gm.Register(m2.InstID, "goblins", "assist", m2)

	gm.Unregister(m1.InstID)
	g := gm.GetGroup("goblins")
	if g == nil {
		t.Fatal("group should still exist with 1 member")
	}
	if len(g.Members) != 1 {
		t.Errorf("expected 1 member after unregister, got %d", len(g.Members))
	}
}

func TestGroupManager_UnregisterEmptyGroupCleanup(t *testing.T) {
	gm := NewGroupManager()
	m := testMonster(0, 10, 10, 100)
	gm.Register(m.InstID, "goblins", "assist", m)
	gm.Unregister(m.InstID)
	if gm.GetGroup("goblins") != nil {
		t.Error("empty group should be cleaned up")
	}
}

func TestGroupManager_UnregisterNonexistent(t *testing.T) {
	gm := NewGroupManager()
	gm.Unregister(999) // should not panic
}

// ---- Assist mode tests ----

func TestAssist_InRange(t *testing.T) {
	gm := NewGroupManager()
	// Monster A at (10,10), Monster B at (12,10) — distance 2, within default range 5.
	mA := testMonster(0, 10, 10, 100)
	mB := testMonster(1, 12, 10, 100)
	gm.Register(mA.InstID, "goblins", "assist", mA)
	gm.Register(mB.InstID, "goblins", "assist", mB)

	// A gets hit by player 42.
	gm.OnMemberDamaged(mA.InstID, 42)

	// B should now have threat on player 42.
	if mB.Threat.TopThreat() != 42 {
		t.Errorf("expected B to have threat on 42, got %d", mB.Threat.TopThreat())
	}
}

func TestAssist_OutOfRange(t *testing.T) {
	gm := NewGroupManager()
	// Monster A at (10,10), Monster C at (20,10) — distance 10, outside default range 5.
	mA := testMonster(0, 10, 10, 100)
	mA.SpawnCfg = &SpawnConfig{AssistRange: 5}
	mC := testMonster(2, 20, 10, 100)
	gm.Register(mA.InstID, "goblins", "assist", mA)
	gm.Register(mC.InstID, "goblins", "assist", mC)

	gm.OnMemberDamaged(mA.InstID, 42)

	// C should NOT have threat (out of range).
	if mC.Threat.TopThreat() != 0 {
		t.Errorf("expected C to have no threat, got %d", mC.Threat.TopThreat())
	}
}

func TestAssist_CustomRange(t *testing.T) {
	gm := NewGroupManager()
	mA := testMonster(0, 10, 10, 100)
	mA.SpawnCfg = &SpawnConfig{AssistRange: 15}
	mB := testMonster(1, 20, 10, 100)
	gm.Register(mA.InstID, "goblins", "assist", mA)
	gm.Register(mB.InstID, "goblins", "assist", mB)

	gm.OnMemberDamaged(mA.InstID, 42)

	// B should have threat — distance 10 is within custom range 15.
	if mB.Threat.TopThreat() != 42 {
		t.Errorf("expected B to have threat with custom range 15")
	}
}

func TestAssist_AlreadyInCombat(t *testing.T) {
	gm := NewGroupManager()
	mA := testMonster(0, 10, 10, 100)
	mB := testMonster(1, 11, 10, 100)
	mB.Threat.AddThreat(99, 50) // B already fighting player 99
	gm.Register(mA.InstID, "goblins", "assist", mA)
	gm.Register(mB.InstID, "goblins", "assist", mB)

	gm.OnMemberDamaged(mA.InstID, 42)

	// B should NOT switch target — already in combat (threat table not empty).
	if mB.Threat.TopThreat() != 99 {
		t.Errorf("B should keep existing target 99, got %d", mB.Threat.TopThreat())
	}
}

func TestAssist_DeadMonsterNotAssisted(t *testing.T) {
	gm := NewGroupManager()
	mA := testMonster(0, 10, 10, 100)
	mB := testMonster(1, 11, 10, 0) // dead
	gm.Register(mA.InstID, "goblins", "assist", mA)
	gm.Register(mB.InstID, "goblins", "assist", mB)

	gm.OnMemberDamaged(mA.InstID, 42)

	// B is dead, should not get threat.
	if mB.Threat.TopThreat() != 0 {
		t.Error("dead monster should not gain threat")
	}
}

func TestAssist_VictimNotInGroup(t *testing.T) {
	gm := NewGroupManager()
	mA := testMonster(0, 10, 10, 100)
	gm.Register(mA.InstID, "goblins", "assist", mA)

	// instID 99999 is not registered.
	gm.OnMemberDamaged(99999, 42) // should not panic
}

// ---- Linked mode tests ----

func TestLinked_AllGetThreat(t *testing.T) {
	gm := NewGroupManager()
	mBoss := testMonster(0, 30, 30, 500)
	mAdd1 := testMonster(1, 28, 30, 100)
	mAdd2 := testMonster(2, 32, 30, 100)
	gm.Register(mBoss.InstID, "boss_room", "linked", mBoss)
	gm.Register(mAdd1.InstID, "boss_room", "linked", mAdd1)
	gm.Register(mAdd2.InstID, "boss_room", "linked", mAdd2)

	// Hit add1 — all group members should get threat.
	gm.OnMemberDamaged(mAdd1.InstID, 7)

	if mBoss.Threat.TopThreat() != 7 {
		t.Errorf("boss should have threat on 7, got %d", mBoss.Threat.TopThreat())
	}
	if mAdd2.Threat.TopThreat() != 7 {
		t.Errorf("add2 should have threat on 7, got %d", mAdd2.Threat.TopThreat())
	}
}

func TestLinked_AlreadyInCombat(t *testing.T) {
	gm := NewGroupManager()
	mA := testMonster(0, 30, 30, 500)
	mA.Threat.AddThreat(99, 100) // already in combat
	mB := testMonster(1, 28, 30, 100)
	gm.Register(mA.InstID, "boss_room", "linked", mA)
	gm.Register(mB.InstID, "boss_room", "linked", mB)

	gm.OnMemberDamaged(mB.InstID, 7)

	// A already in combat, should NOT be disturbed.
	if mA.Threat.TopThreat() != 99 {
		t.Errorf("A should keep existing target 99, got %d", mA.Threat.TopThreat())
	}
	// B is the victim — threat added by TakeDamage, not by OnMemberDamaged.
	// But B has no prior threat, so it gets threat from linked propagation.
	if mB.Threat.TopThreat() != 7 {
		t.Errorf("B should have threat on 7, got %d", mB.Threat.TopThreat())
	}
}

// ---- Pack mode tests ----

func TestPack_FirstMemberBecomesLeader(t *testing.T) {
	gm := NewGroupManager()
	m1 := testMonster(0, 10, 10, 200)
	m2 := testMonster(1, 12, 10, 100)
	gm.Register(m1.InstID, "wolves", "pack", m1)
	gm.Register(m2.InstID, "wolves", "pack", m2)

	g := gm.GetGroup("wolves")
	if g.LeaderID != m1.InstID {
		t.Errorf("first registered should be leader, got %d", g.LeaderID)
	}
}

func TestPack_DamageGoesToLeader(t *testing.T) {
	gm := NewGroupManager()
	mLeader := testMonster(0, 10, 10, 200)
	mFollower := testMonster(1, 12, 10, 100)
	gm.Register(mLeader.InstID, "wolves", "pack", mLeader)
	gm.Register(mFollower.InstID, "wolves", "pack", mFollower)

	// Hit follower — only leader should get threat.
	gm.OnMemberDamaged(mFollower.InstID, 42)

	if mLeader.Threat.TopThreat() != 42 {
		t.Errorf("leader should have threat on 42, got %d", mLeader.Threat.TopThreat())
	}
	// Follower should NOT get threat from pack propagation.
	if mFollower.Threat.TopThreat() != 0 {
		t.Errorf("follower should have no threat, got %d", mFollower.Threat.TopThreat())
	}
}

func TestPack_LeaderSuccession(t *testing.T) {
	gm := NewGroupManager()
	m1 := testMonster(0, 10, 10, 100) // leader, lower HP
	m2 := testMonster(1, 12, 10, 200) // follower, higher HP
	m3 := testMonster(2, 14, 10, 150) // follower, medium HP
	gm.Register(m1.InstID, "wolves", "pack", m1)
	gm.Register(m2.InstID, "wolves", "pack", m2)
	gm.Register(m3.InstID, "wolves", "pack", m3)

	// Leader dies.
	gm.Unregister(m1.InstID)

	g := gm.GetGroup("wolves")
	if g == nil {
		t.Fatal("group should still exist")
	}
	// New leader should be the one with highest HP (m2, instID 101).
	if g.LeaderID != m2.InstID {
		t.Errorf("expected new leader to be instID %d (HP 200), got %d", m2.InstID, g.LeaderID)
	}
}

func TestPack_LeaderAlreadyInCombat(t *testing.T) {
	gm := NewGroupManager()
	mLeader := testMonster(0, 10, 10, 200)
	mLeader.Threat.AddThreat(99, 50) // leader already fighting
	mFollower := testMonster(1, 12, 10, 100)
	gm.Register(mLeader.InstID, "wolves", "pack", mLeader)
	gm.Register(mFollower.InstID, "wolves", "pack", mFollower)

	// Hit follower — leader already in combat, should not add more threat.
	gm.OnMemberDamaged(mFollower.InstID, 42)

	if mLeader.Threat.TopThreat() != 99 {
		t.Errorf("leader should keep existing target 99, got %d", mLeader.Threat.TopThreat())
	}
}

// ---- FollowLeaderTarget BT node tests ----

func TestFollowLeaderTarget_PackMode(t *testing.T) {
	tt := ai.NewThreatTable()
	ctx := &ai.AIContext{
		ThreatTable: tt,
		GroupInfo:    &ai.GroupInfo{GroupType: "pack", LeaderTarget: 42},
	}
	node := &ai.FollowLeaderTarget{}
	status := node.Tick(ctx)
	if status != ai.StatusSuccess {
		t.Errorf("expected Success, got %v", status)
	}
	if tt.TopThreat() != 42 {
		t.Errorf("expected threat on 42, got %d", tt.TopThreat())
	}
}

func TestFollowLeaderTarget_NotPackMode(t *testing.T) {
	ctx := &ai.AIContext{
		ThreatTable: ai.NewThreatTable(),
		GroupInfo:    &ai.GroupInfo{GroupType: "assist", LeaderTarget: 42},
	}
	node := &ai.FollowLeaderTarget{}
	if node.Tick(ctx) != ai.StatusFailure {
		t.Error("expected Failure for non-pack mode")
	}
}

func TestFollowLeaderTarget_NoGroupInfo(t *testing.T) {
	ctx := &ai.AIContext{ThreatTable: ai.NewThreatTable()}
	node := &ai.FollowLeaderTarget{}
	if node.Tick(ctx) != ai.StatusFailure {
		t.Error("expected Failure when no GroupInfo")
	}
}

func TestFollowLeaderTarget_NoLeaderTarget(t *testing.T) {
	ctx := &ai.AIContext{
		ThreatTable: ai.NewThreatTable(),
		GroupInfo:    &ai.GroupInfo{GroupType: "pack", LeaderTarget: 0},
	}
	node := &ai.FollowLeaderTarget{}
	if node.Tick(ctx) != ai.StatusFailure {
		t.Error("expected Failure when leader has no target")
	}
}

func TestFollowLeaderTarget_AlreadyHasThreat(t *testing.T) {
	tt := ai.NewThreatTable()
	tt.AddThreat(99, 100) // already has threat
	ctx := &ai.AIContext{
		ThreatTable: tt,
		GroupInfo:    &ai.GroupInfo{GroupType: "pack", LeaderTarget: 42},
	}
	node := &ai.FollowLeaderTarget{}
	node.Tick(ctx)
	// Should NOT overwrite existing threat.
	if tt.TopThreat() != 99 {
		t.Errorf("should keep existing target 99, got %d", tt.TopThreat())
	}
}

// ---- ClearGroupThreats tests ----

func TestClearGroupThreats(t *testing.T) {
	gm := NewGroupManager()
	m1 := testMonster(0, 10, 10, 100)
	m2 := testMonster(1, 12, 10, 100)
	m1.Threat.AddThreat(42, 50)
	m2.Threat.AddThreat(42, 30)
	gm.Register(m1.InstID, "linked_group", "linked", m1)
	gm.Register(m2.InstID, "linked_group", "linked", m2)

	gm.ClearGroupThreats("linked_group")

	if m1.Threat.Len() != 0 {
		t.Error("m1 threat should be cleared")
	}
	if m2.Threat.Len() != 0 {
		t.Error("m2 threat should be cleared")
	}
}

func TestClearGroupThreats_NonexistentGroup(t *testing.T) {
	gm := NewGroupManager()
	gm.ClearGroupThreats("no_such_group") // should not panic
}

// ---- OnDamaged callback integration test ----

func TestOnDamaged_TriggersGroupAssist(t *testing.T) {
	gm := NewGroupManager()
	mA := testMonster(0, 10, 10, 100)
	mB := testMonster(1, 11, 10, 100)
	gm.Register(mA.InstID, "goblins", "assist", mA)
	gm.Register(mB.InstID, "goblins", "assist", mB)

	// Wire OnDamaged callback like the spawner does.
	mA.OnDamaged = func(monster *MonsterRuntime, attackerCharID int64) {
		gm.OnMemberDamaged(monster.InstID, attackerCharID)
	}

	// Simulate TakeDamage which triggers OnDamaged.
	mA.TakeDamage(10, 42)

	// B should have been alerted via group assist.
	if mB.Threat.TopThreat() != 42 {
		t.Errorf("B should have threat on 42 via OnDamaged, got %d", mB.Threat.TopThreat())
	}
}

// ---- No-group backward compatibility ----

func TestNoGroup_MonsterBehaviorUnchanged(t *testing.T) {
	gm := NewGroupManager()
	m := testMonster(0, 10, 10, 100)
	// Not registered to any group.
	m.TakeDamage(10, 42)

	// Only the monster itself should have threat.
	if m.Threat.TopThreat() != 42 {
		t.Errorf("expected threat on 42, got %d", m.Threat.TopThreat())
	}
	// GroupManager should have no groups.
	if len(gm.groups) != 0 {
		t.Error("no groups should exist")
	}
}

// ---- Mixed groups on same map ----

func TestMixedGroups_IndependentBehavior(t *testing.T) {
	gm := NewGroupManager()
	// Group A: goblins
	mGoblin1 := testMonster(0, 10, 10, 100)
	mGoblin2 := testMonster(1, 11, 10, 100)
	gm.Register(mGoblin1.InstID, "goblins", "assist", mGoblin1)
	gm.Register(mGoblin2.InstID, "goblins", "assist", mGoblin2)

	// Group B: wolves
	mWolf1 := testMonster(2, 20, 20, 150)
	mWolf2 := testMonster(3, 21, 20, 100)
	gm.Register(mWolf1.InstID, "wolves", "linked", mWolf1)
	gm.Register(mWolf2.InstID, "wolves", "linked", mWolf2)

	// Hit goblin1 — only goblin2 should react, not wolves.
	gm.OnMemberDamaged(mGoblin1.InstID, 42)

	if mGoblin2.Threat.TopThreat() != 42 {
		t.Error("goblin2 should have threat")
	}
	if mWolf1.Threat.TopThreat() != 0 {
		t.Error("wolf1 should NOT have threat from goblin attack")
	}
	if mWolf2.Threat.TopThreat() != 0 {
		t.Error("wolf2 should NOT have threat from goblin attack")
	}
}
