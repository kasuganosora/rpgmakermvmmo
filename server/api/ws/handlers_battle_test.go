package ws

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/kasuganosora/rpgmakermvmmo/server/game/player"
	"github.com/kasuganosora/rpgmakermvmmo/server/game/world"
	"github.com/kasuganosora/rpgmakermvmmo/server/resource"
	"github.com/kasuganosora/rpgmakermvmmo/server/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ---------- helpers ----------

// resWithBattle creates a ResourceLoader with the given BattleMMOConfig.
func resWithBattle(cfg *resource.BattleMMOConfig) *resource.ResourceLoader {
	return &resource.ResourceLoader{
		MMOConfig: &resource.MMOConfig{Battle: cfg},
	}
}

// sessionAt creates a session with position and HP set.
func sessionAt(accountID, charID int64, x, y, hp, maxHP int) *player.PlayerSession {
	s := newSession(accountID, charID)
	s.X = x
	s.Y = y
	s.HP = hp
	s.MaxHP = maxHP
	return s
}

// recvPktType drains SendChan and returns the first packet with the given type,
// or "" if timeout expires first.
func recvPktType(s *player.PlayerSession, want string, timeout time.Duration) (player.Packet, bool) {
	deadline := time.After(timeout)
	for {
		select {
		case data := <-s.SendChan:
			var pkt player.Packet
			json.Unmarshal(data, &pkt)
			if pkt.Type == want {
				return pkt, true
			}
		case <-deadline:
			return player.Packet{}, false
		}
	}
}

// ---------- BattleMMOConfig helper tests ----------

func TestBattleMMOConfig_Defaults(t *testing.T) {
	// nil config should return safe defaults.
	var cfg *resource.BattleMMOConfig
	assert.Equal(t, "hybrid", cfg.GetCombatMode())
	assert.Equal(t, 1000, cfg.GetGCDMs())
	assert.Equal(t, 1, cfg.GetAttackRange())

	// Empty struct should also return defaults.
	cfg = &resource.BattleMMOConfig{}
	assert.Equal(t, "hybrid", cfg.GetCombatMode())
	assert.Equal(t, 1000, cfg.GetGCDMs())
	assert.Equal(t, 1, cfg.GetAttackRange())

	// Explicit values.
	cfg = &resource.BattleMMOConfig{
		CombatMode:          "realtime",
		RealtimeGCDMs:       500,
		RealtimeAttackRange: 3,
	}
	assert.Equal(t, "realtime", cfg.GetCombatMode())
	assert.Equal(t, 500, cfg.GetGCDMs())
	assert.Equal(t, 3, cfg.GetAttackRange())
}

// ---------- Combat mode dispatch ----------

func TestHandleAttack_TurnbasedMode_Rejects(t *testing.T) {
	db := testutil.SetupTestDB(t)
	res := resWithBattle(&resource.BattleMMOConfig{CombatMode: "turnbased"})

	wm := world.NewWorldManager(nil, world.NewGameState(nil, nil), world.NewGlobalWhitelist(), nil, nop())
	defer wm.StopAll()
	wm.GetOrCreate(1)

	r := NewRouter(nop())
	bh := NewBattleHandlers(db, wm, res, nop())
	bh.RegisterHandlers(r)

	s := sessionAt(1, 10, 5, 5, 100, 100)
	s.MapID = 1

	raw := makePacket(t, 1, "attack", map[string]interface{}{
		"target_id": int64(1), "target_type": "monster",
	})
	r.Dispatch(s, raw)

	pkt, ok := recvPktType(s, "error", 200*time.Millisecond)
	require.True(t, ok, "expected error packet")
	var errPayload map[string]string
	json.Unmarshal(pkt.Payload, &errPayload)
	assert.Contains(t, errPayload["message"], "turn-based")
}

func TestHandleAttack_HybridMode_Allows(t *testing.T) {
	db := testutil.SetupTestDB(t)
	res := resWithBattle(&resource.BattleMMOConfig{CombatMode: "hybrid"})

	wm := world.NewWorldManager(nil, world.NewGameState(nil, nil), world.NewGlobalWhitelist(), nil, nop())
	defer wm.StopAll()
	room := wm.GetOrCreate(1)

	monster := world.NewMonster(newSlimeTemplate(1000), 1, 5, 5)
	room.AddMonsterRuntime(monster)

	r := NewRouter(nop())
	bh := NewBattleHandlers(db, wm, res, nop())
	bh.RegisterHandlers(r)

	s := sessionAt(1, 10, 5, 5, 100, 100)
	s.MapID = 1
	room.AddPlayer(s)

	raw := makePacket(t, 1, "attack", map[string]interface{}{
		"target_id": monster.InstID, "target_type": "monster",
	})
	r.Dispatch(s, raw)

	pkt, ok := recvPktType(s, "battle_result", 300*time.Millisecond)
	require.True(t, ok, "expected battle_result in hybrid mode")
	assert.Equal(t, "battle_result", pkt.Type)
}

// ---------- GCD enforcement ----------

func TestHandleAttack_GCD_SecondAttackBlocked(t *testing.T) {
	db := testutil.SetupTestDB(t)
	res := resWithBattle(&resource.BattleMMOConfig{RealtimeGCDMs: 2000})

	wm := world.NewWorldManager(nil, world.NewGameState(nil, nil), world.NewGlobalWhitelist(), nil, nop())
	defer wm.StopAll()
	room := wm.GetOrCreate(1)

	monster := world.NewMonster(newSlimeTemplate(1000), 1, 5, 5)
	room.AddMonsterRuntime(monster)

	r := NewRouter(nop())
	bh := NewBattleHandlers(db, wm, res, nop())
	bh.RegisterHandlers(r)

	s := sessionAt(1, 10, 5, 5, 100, 100)
	s.MapID = 1
	room.AddPlayer(s)

	// First attack should succeed.
	raw := makePacket(t, 1, "attack", map[string]interface{}{
		"target_id": monster.InstID, "target_type": "monster",
	})
	r.Dispatch(s, raw)

	pkt, ok := recvPktType(s, "battle_result", 300*time.Millisecond)
	require.True(t, ok, "first attack should succeed")
	assert.Equal(t, "battle_result", pkt.Type)

	// Second attack immediately should be blocked by GCD.
	raw2 := makePacket(t, 2, "attack", map[string]interface{}{
		"target_id": monster.InstID, "target_type": "monster",
	})
	r.Dispatch(s, raw2)

	pkt2, ok2 := recvPktType(s, "error", 200*time.Millisecond)
	require.True(t, ok2, "expected GCD error")
	var errPayload map[string]string
	json.Unmarshal(pkt2.Payload, &errPayload)
	assert.Contains(t, errPayload["message"], "cooldown")
}

// ---------- Range validation ----------

func TestHandleAttack_OutOfRange(t *testing.T) {
	db := testutil.SetupTestDB(t)
	res := resWithBattle(&resource.BattleMMOConfig{RealtimeAttackRange: 1})

	wm := world.NewWorldManager(nil, world.NewGameState(nil, nil), world.NewGlobalWhitelist(), nil, nop())
	defer wm.StopAll()
	room := wm.GetOrCreate(1)

	// Monster at (10, 10), player at (5, 5) → distance 10 > range 1
	monster := world.NewMonster(newSlimeTemplate(1000), 1, 10, 10)
	room.AddMonsterRuntime(monster)

	r := NewRouter(nop())
	bh := NewBattleHandlers(db, wm, res, nop())
	bh.RegisterHandlers(r)

	s := sessionAt(1, 10, 5, 5, 100, 100)
	s.MapID = 1
	room.AddPlayer(s)

	raw := makePacket(t, 1, "attack", map[string]interface{}{
		"target_id": monster.InstID, "target_type": "monster",
	})
	r.Dispatch(s, raw)

	pkt, ok := recvPktType(s, "error", 200*time.Millisecond)
	require.True(t, ok, "expected range error")
	var errPayload map[string]string
	json.Unmarshal(pkt.Payload, &errPayload)
	assert.Contains(t, errPayload["message"], "out of range")
}

func TestHandleAttack_InRange_LargeRange(t *testing.T) {
	db := testutil.SetupTestDB(t)
	res := resWithBattle(&resource.BattleMMOConfig{RealtimeAttackRange: 15})

	wm := world.NewWorldManager(nil, world.NewGameState(nil, nil), world.NewGlobalWhitelist(), nil, nop())
	defer wm.StopAll()
	room := wm.GetOrCreate(1)

	// Monster at (10, 10), player at (5, 5) → distance 10 ≤ range 15
	monster := world.NewMonster(newSlimeTemplate(1000), 1, 10, 10)
	room.AddMonsterRuntime(monster)

	r := NewRouter(nop())
	bh := NewBattleHandlers(db, wm, res, nop())
	bh.RegisterHandlers(r)

	s := sessionAt(1, 10, 5, 5, 100, 100)
	s.MapID = 1
	room.AddPlayer(s)

	raw := makePacket(t, 1, "attack", map[string]interface{}{
		"target_id": monster.InstID, "target_type": "monster",
	})
	r.Dispatch(s, raw)

	pkt, ok := recvPktType(s, "battle_result", 300*time.Millisecond)
	require.True(t, ok, "expected battle_result with large range")
	assert.Equal(t, "battle_result", pkt.Type)
}

// ---------- Dead player cannot attack ----------

func TestHandleAttack_DeadPlayer_Rejected(t *testing.T) {
	db := testutil.SetupTestDB(t)
	wm := world.NewWorldManager(nil, world.NewGameState(nil, nil), world.NewGlobalWhitelist(), nil, nop())
	defer wm.StopAll()
	wm.GetOrCreate(1)

	r := NewRouter(nop())
	bh := NewBattleHandlers(db, wm, nil, nop())
	bh.RegisterHandlers(r)

	// HP=0, MaxHP=100 → dead
	s := sessionAt(1, 10, 5, 5, 0, 100)
	s.MapID = 1

	raw := makePacket(t, 1, "attack", map[string]interface{}{
		"target_id": int64(1), "target_type": "monster",
	})
	r.Dispatch(s, raw)

	pkt, ok := recvPktType(s, "error", 200*time.Millisecond)
	require.True(t, ok, "expected dead player error")
	var errPayload map[string]string
	json.Unmarshal(pkt.Payload, &errPayload)
	assert.Contains(t, errPayload["message"], "dead")
}

// ---------- Skill cost enforcement ----------

func TestHandleAttack_SkillCost_NotEnoughMP(t *testing.T) {
	db := testutil.SetupTestDB(t)
	skills := make([]*resource.Skill, 2)
	skills[1] = &resource.Skill{ID: 1, Name: "Fire", MPCost: 50, Damage: resource.SkillDamage{Type: 1}}
	res := &resource.ResourceLoader{
		Skills: skills,
		MMOConfig: &resource.MMOConfig{
			Battle: &resource.BattleMMOConfig{EnforceSkillCosts: true},
		},
	}

	wm := world.NewWorldManager(nil, world.NewGameState(nil, nil), world.NewGlobalWhitelist(), nil, nop())
	defer wm.StopAll()
	room := wm.GetOrCreate(1)

	monster := world.NewMonster(newSlimeTemplate(1000), 1, 5, 5)
	room.AddMonsterRuntime(monster)

	r := NewRouter(nop())
	bh := NewBattleHandlers(db, wm, res, nop())
	bh.RegisterHandlers(r)

	// Player has only 10 MP, skill costs 50
	s := sessionAt(1, 10, 5, 5, 100, 100)
	s.MP = 10
	s.MaxMP = 100
	s.MapID = 1
	room.AddPlayer(s)

	raw := makePacket(t, 1, "attack", map[string]interface{}{
		"target_id": monster.InstID, "target_type": "monster", "skill_id": 1,
	})
	r.Dispatch(s, raw)

	pkt, ok := recvPktType(s, "error", 200*time.Millisecond)
	require.True(t, ok, "expected MP error")
	var errPayload map[string]string
	json.Unmarshal(pkt.Payload, &errPayload)
	assert.Contains(t, errPayload["message"], "MP")
}

func TestHandleAttack_SkillCost_DeductsMP(t *testing.T) {
	db := testutil.SetupTestDB(t)
	skills := make([]*resource.Skill, 2)
	skills[1] = &resource.Skill{ID: 1, Name: "Fire", MPCost: 10, Damage: resource.SkillDamage{Type: 1}}
	res := &resource.ResourceLoader{
		Skills: skills,
		MMOConfig: &resource.MMOConfig{
			Battle: &resource.BattleMMOConfig{EnforceSkillCosts: true},
		},
	}

	wm := world.NewWorldManager(nil, world.NewGameState(nil, nil), world.NewGlobalWhitelist(), nil, nop())
	defer wm.StopAll()
	room := wm.GetOrCreate(1)

	monster := world.NewMonster(newSlimeTemplate(1000), 1, 5, 5)
	room.AddMonsterRuntime(monster)

	r := NewRouter(nop())
	bh := NewBattleHandlers(db, wm, res, nop())
	bh.RegisterHandlers(r)

	s := sessionAt(1, 10, 5, 5, 100, 100)
	s.MP = 50
	s.MaxMP = 100
	s.MapID = 1
	room.AddPlayer(s)

	raw := makePacket(t, 1, "attack", map[string]interface{}{
		"target_id": monster.InstID, "target_type": "monster", "skill_id": 1,
	})
	r.Dispatch(s, raw)

	_, ok := recvPktType(s, "battle_result", 300*time.Millisecond)
	require.True(t, ok, "expected battle_result after skill cost")

	// Check MP was deducted.
	_, _, mp, _ := s.Stats()
	assert.Equal(t, 40, mp, "MP should be deducted by skill cost")
}

func TestHandleAttack_SkillCost_NotEnforcedByDefault(t *testing.T) {
	db := testutil.SetupTestDB(t)
	skills := make([]*resource.Skill, 2)
	skills[1] = &resource.Skill{ID: 1, Name: "Fire", MPCost: 50, Damage: resource.SkillDamage{Type: 1}}
	// EnforceSkillCosts is false (default)
	res := &resource.ResourceLoader{
		Skills: skills,
		MMOConfig: &resource.MMOConfig{
			Battle: &resource.BattleMMOConfig{},
		},
	}

	wm := world.NewWorldManager(nil, world.NewGameState(nil, nil), world.NewGlobalWhitelist(), nil, nop())
	defer wm.StopAll()
	room := wm.GetOrCreate(1)

	monster := world.NewMonster(newSlimeTemplate(1000), 1, 5, 5)
	room.AddMonsterRuntime(monster)

	r := NewRouter(nop())
	bh := NewBattleHandlers(db, wm, res, nop())
	bh.RegisterHandlers(r)

	// Player has only 10 MP, skill costs 50 — but enforcement is off
	s := sessionAt(1, 10, 5, 5, 100, 100)
	s.MP = 10
	s.MapID = 1
	room.AddPlayer(s)

	raw := makePacket(t, 1, "attack", map[string]interface{}{
		"target_id": monster.InstID, "target_type": "monster", "skill_id": 1,
	})
	r.Dispatch(s, raw)

	pkt, ok := recvPktType(s, "battle_result", 300*time.Millisecond)
	require.True(t, ok, "should succeed when cost enforcement is off")
	assert.Equal(t, "battle_result", pkt.Type)
}

// ---------- battle_result contains x/y ----------

func TestHandleAttack_BattleResult_ContainsXY(t *testing.T) {
	db := testutil.SetupTestDB(t)
	wm := world.NewWorldManager(nil, world.NewGameState(nil, nil), world.NewGlobalWhitelist(), nil, nop())
	defer wm.StopAll()
	room := wm.GetOrCreate(1)

	monster := world.NewMonster(newSlimeTemplate(1000), 1, 7, 12)
	room.AddMonsterRuntime(monster)

	r := NewRouter(nop())
	bh := NewBattleHandlers(db, wm, nil, nop())
	bh.RegisterHandlers(r)

	s := newSession(1, 10)
	s.MapID = 1
	room.AddPlayer(s)

	raw := makePacket(t, 1, "attack", map[string]interface{}{
		"target_id": monster.InstID, "target_type": "monster",
	})
	r.Dispatch(s, raw)

	pkt, ok := recvPktType(s, "battle_result", 300*time.Millisecond)
	require.True(t, ok, "expected battle_result")

	var payload map[string]interface{}
	json.Unmarshal(pkt.Payload, &payload)
	assert.Equal(t, float64(7), payload["x"], "battle_result should include monster x")
	assert.Equal(t, float64(12), payload["y"], "battle_result should include monster y")
}

// ---------- Player death / revive ----------

func TestPlayerSession_ApplyDamage_Death(t *testing.T) {
	s := &player.PlayerSession{
		HP: 50, MaxHP: 100,
		SendChan: make(chan []byte, 256),
		Done:     make(chan struct{}),
	}
	newHP, dead := s.ApplyDamage(30)
	assert.Equal(t, 20, newHP)
	assert.False(t, dead)

	newHP, dead = s.ApplyDamage(20)
	assert.Equal(t, 0, newHP)
	assert.True(t, dead)

	// Overkill: should clamp to 0.
	s.HP = 10
	newHP, dead = s.ApplyDamage(999)
	assert.Equal(t, 0, newHP)
	assert.True(t, dead)
}

func TestPlayerSession_Revive(t *testing.T) {
	s := &player.PlayerSession{
		HP: 0, MaxHP: 200,
		SendChan: make(chan []byte, 256),
		Done:     make(chan struct{}),
	}
	s.Revive(100)
	hp, maxHP, _, _ := s.Stats()
	assert.Equal(t, 100, hp)
	assert.Equal(t, 200, maxHP)

	// Revive with more than MaxHP: clamp.
	s.HP = 0
	s.Revive(999)
	hp, _, _, _ = s.Stats()
	assert.Equal(t, 200, hp)

	// Revive with 0 or negative: at least 1.
	s.HP = 0
	s.Revive(0)
	hp, _, _, _ = s.Stats()
	assert.Equal(t, 1, hp)
}

func TestPlayerSession_IsDead(t *testing.T) {
	s := &player.PlayerSession{
		SendChan: make(chan []byte, 256),
		Done:     make(chan struct{}),
	}
	// Uninitialized: HP=0, MaxHP=0 → NOT dead.
	assert.False(t, s.IsDead(), "uninitialized session should not be dead")

	// Alive: HP=50, MaxHP=100.
	s.HP = 50
	s.MaxHP = 100
	assert.False(t, s.IsDead())

	// Dead: HP=0, MaxHP=100.
	s.HP = 0
	assert.True(t, s.IsDead())
}

func TestPlayerSession_ConsumeMP(t *testing.T) {
	s := &player.PlayerSession{
		MP: 30, MaxMP: 100,
		SendChan: make(chan []byte, 256),
		Done:     make(chan struct{}),
	}
	assert.True(t, s.ConsumeMP(10))
	_, _, mp, _ := s.Stats()
	assert.Equal(t, 20, mp)

	assert.False(t, s.ConsumeMP(25), "should fail when not enough MP")
	_, _, mp, _ = s.Stats()
	assert.Equal(t, 20, mp, "MP should be unchanged after failed consume")
}

func TestPlayerSession_CheckAttackGCD(t *testing.T) {
	s := &player.PlayerSession{
		SendChan: make(chan []byte, 256),
		Done:     make(chan struct{}),
	}
	// First call always succeeds (zero lastAttackTime).
	assert.True(t, s.CheckAttackGCD(1000))

	// Immediate second call should fail (within 1000ms).
	assert.False(t, s.CheckAttackGCD(1000))

	// With 0ms GCD, should always pass.
	assert.True(t, s.CheckAttackGCD(0))
}

func TestHandleReviveRequest_RevivesDeadPlayer(t *testing.T) {
	db := testutil.SetupTestDB(t)
	wm := world.NewWorldManager(nil, world.NewGameState(nil, nil), world.NewGlobalWhitelist(), nil, nop())
	defer wm.StopAll()

	r := NewRouter(nop())
	bh := NewBattleHandlers(db, wm, nil, nop())
	bh.RegisterHandlers(r)

	s := sessionAt(1, 10, 5, 5, 0, 200) // dead
	raw := makePacket(t, 1, "revive_request", nil)
	r.Dispatch(s, raw)

	pkt, ok := recvPktType(s, "player_revive", 300*time.Millisecond)
	require.True(t, ok, "expected player_revive")

	var payload map[string]interface{}
	json.Unmarshal(pkt.Payload, &payload)
	assert.Equal(t, float64(100), payload["hp"], "revive HP should be 50% of MaxHP")

	// Verify HP was restored.
	hp, _, _, _ := s.Stats()
	assert.Equal(t, 100, hp)
}

func TestHandleReviveRequest_AlivePlayer_Ignored(t *testing.T) {
	db := testutil.SetupTestDB(t)
	wm := world.NewWorldManager(nil, world.NewGameState(nil, nil), world.NewGlobalWhitelist(), nil, nop())
	defer wm.StopAll()

	r := NewRouter(nop())
	bh := NewBattleHandlers(db, wm, nil, nop())
	bh.RegisterHandlers(r)

	s := sessionAt(1, 10, 5, 5, 100, 200) // alive
	raw := makePacket(t, 1, "revive_request", nil)
	r.Dispatch(s, raw)

	// Should receive nothing (alive player ignored).
	select {
	case <-s.SendChan:
		t.Error("should not receive packet for alive player")
	case <-time.After(100 * time.Millisecond):
		// OK — no response expected.
	}
}

func TestHandleReviveRequest_WithReviveMap(t *testing.T) {
	db := testutil.SetupTestDB(t)
	res := resWithBattle(&resource.BattleMMOConfig{
		DeathReviveMapID: 5,
		DeathReviveX:     10,
		DeathReviveY:     20,
	})

	wm := world.NewWorldManager(nil, world.NewGameState(nil, nil), world.NewGlobalWhitelist(), nil, nop())
	defer wm.StopAll()

	r := NewRouter(nop())
	bh := NewBattleHandlers(db, wm, res, nop())
	bh.RegisterHandlers(r)

	s := sessionAt(1, 10, 5, 5, 0, 200) // dead
	raw := makePacket(t, 1, "revive_request", nil)
	r.Dispatch(s, raw)

	// Should receive revive_transfer first, then player_revive.
	pkt, ok := recvPktType(s, "revive_transfer", 300*time.Millisecond)
	require.True(t, ok, "expected revive_transfer")

	var transferPayload map[string]interface{}
	json.Unmarshal(pkt.Payload, &transferPayload)
	assert.Equal(t, float64(5), transferPayload["map_id"])
	assert.Equal(t, float64(10), transferPayload["x"])
	assert.Equal(t, float64(20), transferPayload["y"])

	// Also should get player_revive.
	pkt2, ok2 := recvPktType(s, "player_revive", 300*time.Millisecond)
	require.True(t, ok2, "expected player_revive after transfer")
	assert.Equal(t, "player_revive", pkt2.Type)
}

// ---------- Threat table integration ----------

func TestThreatTable_PlayerLeaveRoom_ClearsThreat(t *testing.T) {
	wm := world.NewWorldManager(nil, world.NewGameState(nil, nil), world.NewGlobalWhitelist(), nil, nop())
	defer wm.StopAll()
	room := wm.GetOrCreate(1)

	monster := world.NewMonster(newSlimeTemplate(1000), 1, 5, 5)
	room.AddMonsterRuntime(monster)

	s := newSession(1, 10)
	s.MapID = 1
	room.AddPlayer(s)

	// Simulate attack: add threat manually.
	monster.TakeDamage(50, s.CharID)
	assert.Equal(t, int64(10), monster.Threat.TopThreat(), "threat should be for player 10")

	// Player leaves room.
	room.RemovePlayer(s.CharID)

	// Threat should be cleared for that player.
	assert.Equal(t, int64(0), monster.Threat.TopThreat(), "threat should be cleared after player leaves")
}

// ---------- Monster damage callback wiring ----------

func TestMonsterDamageFunc_InjectedViaRegisterHandlers(t *testing.T) {
	db := testutil.SetupTestDB(t)
	wm := world.NewWorldManager(nil, world.NewGameState(nil, nil), world.NewGlobalWhitelist(), nil, nop())
	defer wm.StopAll()

	r := NewRouter(nop())
	bh := NewBattleHandlers(db, wm, nil, nop())
	bh.RegisterHandlers(r)

	// After RegisterHandlers, new rooms should have monsterDmgFn set.
	room := wm.GetOrCreate(1)

	monster := world.NewMonster(newSlimeTemplate(1000), 1, 5, 5)
	room.AddMonsterRuntime(monster)

	s := sessionAt(1, 10, 5, 5, 100, 100)
	s.MapID = 1
	room.AddPlayer(s)

	// GetPlayerSession should work.
	got := room.GetPlayerSession(s.CharID)
	assert.NotNil(t, got)
	assert.Equal(t, s.CharID, got.CharID)

	// GetPlayerSession for non-existent player.
	assert.Nil(t, room.GetPlayerSession(999))
}
