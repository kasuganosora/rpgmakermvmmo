package ai

import (
	"testing"

	"github.com/kasuganosora/rpgmakermvmmo/server/resource"
)

// ---- Mock MonsterAccessor ----

type mockMonster struct {
	state       MonsterState
	x, y        int
	spawnX, spawnY int
	hp, maxHP   int
	target      int64
	agi         int
	cachedPath  []Point
	cachedTarget Point
	moveTimer   int
	attackTimer int
	dirtyCount  int
	dir         int
}

func (m *mockMonster) GetState() MonsterState     { return m.state }
func (m *mockMonster) SetState(s MonsterState)     { m.state = s }
func (m *mockMonster) Position() (int, int)        { return m.x, m.y }
func (m *mockMonster) SetPosition(x, y, dir int)   { m.x = x; m.y = y; m.dir = dir }
func (m *mockMonster) SpawnPosition() (int, int)   { return m.spawnX, m.spawnY }
func (m *mockMonster) GetHP() int                  { return m.hp }
func (m *mockMonster) GetMaxHP() int               { return m.maxHP }
func (m *mockMonster) GetTarget() int64            { return m.target }
func (m *mockMonster) SetTarget(id int64)          { m.target = id }
func (m *mockMonster) GetAgi() int                 { return m.agi }
func (m *mockMonster) GetCachedPath() []Point      { return m.cachedPath }
func (m *mockMonster) SetCachedPath(p []Point, t Point) { m.cachedPath = p; m.cachedTarget = t }
func (m *mockMonster) GetCachedTarget() Point      { return m.cachedTarget }
func (m *mockMonster) CanMove() bool               { return m.moveTimer <= 0 }
func (m *mockMonster) ResetMoveTimer(t int)        { m.moveTimer = t }
func (m *mockMonster) CanAttack() bool             { return m.attackTimer <= 0 }
func (m *mockMonster) ResetAttackTimer(t int)      { m.attackTimer = t }
func (m *mockMonster) MarkDirty()                  { m.dirtyCount++ }

// ---- Mock RoomAccessor ----

type mockRoom struct {
	players []PlayerInfo
	passMap *resource.PassabilityMap
	// Track TryMoveMonster calls
	lastMoveDir int
	moveResult  bool
}

func (r *mockRoom) PlayersInRange(x, y, radius int) []PlayerInfo {
	var result []PlayerInfo
	for _, p := range r.players {
		dx := p.X - x
		dy := p.Y - y
		if dx < 0 { dx = -dx }
		if dy < 0 { dy = -dy }
		if dx+dy <= radius {
			result = append(result, p)
		}
	}
	return result
}

func (r *mockRoom) PlayerByID(charID int64) *PlayerInfo {
	for _, p := range r.players {
		if p.CharID == charID {
			cp := p
			return &cp
		}
	}
	return nil
}

func (r *mockRoom) TryMoveMonster(m MonsterAccessor, dir int) bool {
	r.lastMoveDir = dir
	if r.moveResult {
		dx, dy := 0, 0
		switch dir {
		case 2: dy = 1
		case 4: dx = -1
		case 6: dx = 1
		case 8: dy = -1
		}
		x, y := m.Position()
		m.SetPosition(x+dx, y+dy, dir)
	}
	return r.moveResult
}

func (r *mockRoom) GetPassMap() *resource.PassabilityMap {
	return r.passMap
}

// ---- ThreatTable Tests ----

func TestThreatTable_AddAndTop(t *testing.T) {
	tt := NewThreatTable()
	if tt.Len() != 0 {
		t.Fatal("new table should be empty")
	}
	tt.AddThreat(1, 50)
	tt.AddThreat(2, 100)
	tt.AddThreat(1, 60) // total 110
	if tt.TopThreat() != 1 {
		t.Errorf("expected top=1 (110), got %d", tt.TopThreat())
	}
	if tt.Len() != 2 {
		t.Errorf("expected len=2, got %d", tt.Len())
	}
}

func TestThreatTable_ZeroDamageIgnored(t *testing.T) {
	tt := NewThreatTable()
	tt.AddThreat(1, 0)
	tt.AddThreat(2, -5)
	if tt.Len() != 0 {
		t.Error("zero/negative damage should not create entries")
	}
}

func TestThreatTable_Remove(t *testing.T) {
	tt := NewThreatTable()
	tt.AddThreat(1, 50)
	tt.AddThreat(2, 100)
	tt.Remove(2)
	if tt.TopThreat() != 1 {
		t.Error("after removing 2, top should be 1")
	}
}

func TestThreatTable_Decay(t *testing.T) {
	tt := NewThreatTable()
	tt.AddThreat(1, 100)
	tt.AddThreat(2, 10)
	tt.Decay(50) // 100→50, 10→5
	if tt.TopThreat() != 1 {
		t.Error("after 50% decay, 1 should still be top")
	}
	// Decay 100% should remove all.
	tt.Decay(100)
	if tt.Len() != 0 {
		t.Errorf("100%% decay should clear table, got len=%d", tt.Len())
	}
}

func TestThreatTable_DecayZero(t *testing.T) {
	tt := NewThreatTable()
	tt.AddThreat(1, 50)
	tt.Decay(0) // noop
	if tt.Len() != 1 {
		t.Error("0% decay should be a noop")
	}
}

func TestThreatTable_Clear(t *testing.T) {
	tt := NewThreatTable()
	tt.AddThreat(1, 50)
	tt.AddThreat(2, 100)
	tt.Clear()
	if tt.Len() != 0 {
		t.Error("clear should empty the table")
	}
	if tt.TopThreat() != 0 {
		t.Error("empty table TopThreat should return 0")
	}
}

// ---- Behavior Tree Framework Tests ----

func TestSelector_FirstSuccessWins(t *testing.T) {
	called := 0
	sel := &Selector{Children: []Node{
		&ConditionNode{Fn: func(*AIContext) bool { called++; return false }},
		&ConditionNode{Fn: func(*AIContext) bool { called++; return true }},
		&ConditionNode{Fn: func(*AIContext) bool { called++; return true }},
	}}
	s := sel.Tick(&AIContext{})
	if s != StatusSuccess {
		t.Error("selector should succeed on second child")
	}
	if called != 2 {
		t.Errorf("should stop after second child, called=%d", called)
	}
}

func TestSelector_AllFail(t *testing.T) {
	sel := &Selector{Children: []Node{
		&ConditionNode{Fn: func(*AIContext) bool { return false }},
	}}
	if sel.Tick(&AIContext{}) != StatusFailure {
		t.Error("all-fail selector should return failure")
	}
}

func TestSelector_RunningStops(t *testing.T) {
	sel := &Selector{Children: []Node{
		&ActionNode{Fn: func(*AIContext) Status { return StatusRunning }},
		&ConditionNode{Fn: func(*AIContext) bool { t.Error("should not reach"); return true }},
	}}
	if sel.Tick(&AIContext{}) != StatusRunning {
		t.Error("running should propagate")
	}
}

func TestSequence_AllSucceed(t *testing.T) {
	called := 0
	seq := &Sequence{Children: []Node{
		&ConditionNode{Fn: func(*AIContext) bool { called++; return true }},
		&ConditionNode{Fn: func(*AIContext) bool { called++; return true }},
	}}
	if seq.Tick(&AIContext{}) != StatusSuccess {
		t.Error("all-succeed sequence should succeed")
	}
	if called != 2 {
		t.Error("both children should be called")
	}
}

func TestSequence_FirstFailStops(t *testing.T) {
	seq := &Sequence{Children: []Node{
		&ConditionNode{Fn: func(*AIContext) bool { return false }},
		&ConditionNode{Fn: func(*AIContext) bool { t.Error("should not reach"); return true }},
	}}
	if seq.Tick(&AIContext{}) != StatusFailure {
		t.Error("first-fail should return failure")
	}
}

func TestInverter(t *testing.T) {
	inv := &Inverter{Child: &ConditionNode{Fn: func(*AIContext) bool { return true }}}
	if inv.Tick(&AIContext{}) != StatusFailure {
		t.Error("inverter should negate success")
	}
	inv2 := &Inverter{Child: &ConditionNode{Fn: func(*AIContext) bool { return false }}}
	if inv2.Tick(&AIContext{}) != StatusSuccess {
		t.Error("inverter should negate failure")
	}
	inv3 := &Inverter{Child: &ActionNode{Fn: func(*AIContext) Status { return StatusRunning }}}
	if inv3.Tick(&AIContext{}) != StatusRunning {
		t.Error("inverter should pass through running")
	}
}

func TestBehaviorTree_NilRoot(t *testing.T) {
	bt := &BehaviorTree{}
	if bt.Tick(&AIContext{}) != StatusFailure {
		t.Error("nil root should return failure")
	}
}

// ---- Node Tests ----

func TestCheckPlayerInRange_NoConfig(t *testing.T) {
	m := &mockMonster{x: 5, y: 5}
	room := &mockRoom{players: []PlayerInfo{{CharID: 1, X: 6, Y: 5, HP: 100}}}
	ctx := &AIContext{Monster: m, Room: room, Config: nil}
	if (&CheckPlayerInRange{}).Tick(ctx) != StatusFailure {
		t.Error("nil config should fail")
	}
}

func TestCheckPlayerInRange_FindsNearest(t *testing.T) {
	m := &mockMonster{x: 5, y: 5}
	room := &mockRoom{players: []PlayerInfo{
		{CharID: 1, X: 10, Y: 5, HP: 100},
		{CharID: 2, X: 6, Y: 5, HP: 100},
	}}
	ctx := &AIContext{Monster: m, Room: room, Config: &AIProfile{AggroRange: 8}}
	if (&CheckPlayerInRange{}).Tick(ctx) != StatusSuccess {
		t.Error("should find player in range")
	}
	if m.target != 2 {
		t.Errorf("should target nearest player (2), got %d", m.target)
	}
}

func TestCheckPlayerInRange_PrefersThreat(t *testing.T) {
	m := &mockMonster{x: 5, y: 5}
	room := &mockRoom{players: []PlayerInfo{
		{CharID: 1, X: 6, Y: 5, HP: 100},
		{CharID: 2, X: 7, Y: 5, HP: 100},
	}}
	tt := NewThreatTable()
	tt.AddThreat(2, 100) // char 2 has more threat but is farther
	ctx := &AIContext{Monster: m, Room: room, Config: &AIProfile{AggroRange: 8}, ThreatTable: tt}
	if (&CheckPlayerInRange{}).Tick(ctx) != StatusSuccess {
		t.Error("should succeed")
	}
	if m.target != 2 {
		t.Errorf("should target top threat (2), got %d", m.target)
	}
}

func TestCheckPlayerInRange_ThreatGone(t *testing.T) {
	m := &mockMonster{x: 5, y: 5}
	room := &mockRoom{players: []PlayerInfo{
		{CharID: 1, X: 6, Y: 5, HP: 100},
	}}
	tt := NewThreatTable()
	tt.AddThreat(99, 200) // char 99 not in room
	ctx := &AIContext{Monster: m, Room: room, Config: &AIProfile{AggroRange: 8}, ThreatTable: tt}
	if (&CheckPlayerInRange{}).Tick(ctx) != StatusSuccess {
		t.Error("should fall through to nearest player")
	}
	if m.target != 1 {
		t.Errorf("should target nearest player (1), got %d", m.target)
	}
	if tt.Len() != 0 {
		t.Error("gone player should be removed from threat table")
	}
}

func TestCheckPlayerInRange_DeadPlayerIgnored(t *testing.T) {
	m := &mockMonster{x: 5, y: 5}
	room := &mockRoom{players: []PlayerInfo{
		{CharID: 1, X: 6, Y: 5, HP: 0}, // dead
	}}
	ctx := &AIContext{Monster: m, Room: room, Config: &AIProfile{AggroRange: 8}}
	if (&CheckPlayerInRange{}).Tick(ctx) != StatusFailure {
		t.Error("dead players should not be targeted")
	}
}

func TestCheckPlayerInRange_NoPlayersInRange(t *testing.T) {
	m := &mockMonster{x: 5, y: 5}
	room := &mockRoom{players: []PlayerInfo{
		{CharID: 1, X: 50, Y: 50, HP: 100}, // far away
	}}
	ctx := &AIContext{Monster: m, Room: room, Config: &AIProfile{AggroRange: 3}}
	if (&CheckPlayerInRange{}).Tick(ctx) != StatusFailure {
		t.Error("no players in range should fail")
	}
}

func TestCheckTargetAlive(t *testing.T) {
	m := &mockMonster{target: 1}
	room := &mockRoom{players: []PlayerInfo{{CharID: 1, X: 5, Y: 5, HP: 100}}}
	ctx := &AIContext{Monster: m, Room: room}
	if (&CheckTargetAlive{}).Tick(ctx) != StatusSuccess {
		t.Error("alive target should succeed")
	}

	// Target dead.
	room.players[0].HP = 0
	if (&CheckTargetAlive{}).Tick(ctx) != StatusFailure {
		t.Error("dead target should fail")
	}
	if m.target != 0 {
		t.Error("dead target should be cleared")
	}

	// No target.
	m.target = 0
	if (&CheckTargetAlive{}).Tick(ctx) != StatusFailure {
		t.Error("no target should fail")
	}
}

func TestCheckTargetAlive_NotInRoom(t *testing.T) {
	m := &mockMonster{target: 99}
	room := &mockRoom{}
	ctx := &AIContext{Monster: m, Room: room}
	if (&CheckTargetAlive{}).Tick(ctx) != StatusFailure {
		t.Error("target not in room should fail")
	}
	if m.target != 0 {
		t.Error("missing target should be cleared")
	}
}

func TestCheckLeashRange(t *testing.T) {
	m := &mockMonster{x: 5, y: 5, spawnX: 5, spawnY: 5}
	ctx := &AIContext{Monster: m, Config: &AIProfile{LeashRange: 3}}
	if (&CheckLeashRange{}).Tick(ctx) != StatusSuccess {
		t.Error("at spawn should be in range")
	}
	m.x = 10 // 5 tiles away
	if (&CheckLeashRange{}).Tick(ctx) != StatusFailure {
		t.Error("5 tiles away with leash 3 should fail")
	}
}

func TestCheckLeashRange_NoLeash(t *testing.T) {
	m := &mockMonster{x: 100, y: 100, spawnX: 0, spawnY: 0}
	ctx := &AIContext{Monster: m, Config: &AIProfile{LeashRange: 0}}
	if (&CheckLeashRange{}).Tick(ctx) != StatusSuccess {
		t.Error("leash 0 should always pass (no leash)")
	}
}

func TestCheckLeashRange_NilConfig(t *testing.T) {
	m := &mockMonster{x: 100, y: 100}
	ctx := &AIContext{Monster: m}
	if (&CheckLeashRange{}).Tick(ctx) != StatusSuccess {
		t.Error("nil config should mean no leash")
	}
}

func TestCheckHPBelow(t *testing.T) {
	m := &mockMonster{hp: 10, maxHP: 100}
	ctx := &AIContext{Monster: m, Config: &AIProfile{FleeHPPercent: 20}}
	if (&CheckHPBelow{}).Tick(ctx) != StatusSuccess {
		t.Error("10% < 20% should trigger flee")
	}
	m.hp = 50
	if (&CheckHPBelow{}).Tick(ctx) != StatusFailure {
		t.Error("50% >= 20% should not trigger flee")
	}
}

func TestCheckHPBelow_NoFlee(t *testing.T) {
	m := &mockMonster{hp: 1, maxHP: 100}
	ctx := &AIContext{Monster: m, Config: &AIProfile{FleeHPPercent: 0}}
	if (&CheckHPBelow{}).Tick(ctx) != StatusFailure {
		t.Error("flee 0 should never trigger")
	}
}

func TestCheckHPBelow_ZeroMaxHP(t *testing.T) {
	m := &mockMonster{hp: 0, maxHP: 0}
	ctx := &AIContext{Monster: m, Config: &AIProfile{FleeHPPercent: 20}}
	if (&CheckHPBelow{}).Tick(ctx) != StatusFailure {
		t.Error("zero maxHP should fail")
	}
}

func TestCheckAttackRange(t *testing.T) {
	m := &mockMonster{x: 5, y: 5, target: 1}
	room := &mockRoom{players: []PlayerInfo{{CharID: 1, X: 6, Y: 5, HP: 100}}}
	ctx := &AIContext{Monster: m, Room: room, Config: &AIProfile{AttackRange: 1}}
	if (&CheckAttackRange{}).Tick(ctx) != StatusSuccess {
		t.Error("1 tile away with range 1 should succeed")
	}
	room.players[0].X = 8 // 3 tiles away
	if (&CheckAttackRange{}).Tick(ctx) != StatusFailure {
		t.Error("3 tiles away with range 1 should fail")
	}
}

func TestCheckAttackRange_NoTarget(t *testing.T) {
	m := &mockMonster{target: 0}
	ctx := &AIContext{Monster: m, Room: &mockRoom{}}
	if (&CheckAttackRange{}).Tick(ctx) != StatusFailure {
		t.Error("no target should fail")
	}
}

func TestCheckAttackRange_DefaultRange(t *testing.T) {
	m := &mockMonster{x: 5, y: 5, target: 1}
	room := &mockRoom{players: []PlayerInfo{{CharID: 1, X: 6, Y: 5, HP: 100}}}
	ctx := &AIContext{Monster: m, Room: room} // no config, defaults to range 1
	if (&CheckAttackRange{}).Tick(ctx) != StatusSuccess {
		t.Error("default range 1 should work")
	}
}

func TestWander_MovesRandomly(t *testing.T) {
	m := &mockMonster{x: 5, y: 5, spawnX: 5, spawnY: 5}
	room := &mockRoom{moveResult: true}
	ctx := &AIContext{Monster: m, Room: room, Config: &AIProfile{WanderRadius: 4, MoveIntervalTicks: 10}}
	status := (&Wander{}).Tick(ctx)
	if status != StatusSuccess {
		t.Error("should succeed when movement works")
	}
	if m.moveTimer != 10 {
		t.Errorf("move timer should be 10, got %d", m.moveTimer)
	}
}

func TestWander_CannotMove(t *testing.T) {
	m := &mockMonster{x: 5, y: 5, spawnX: 5, spawnY: 5, moveTimer: 5}
	room := &mockRoom{moveResult: true}
	ctx := &AIContext{Monster: m, Room: room, Config: &AIProfile{WanderRadius: 4}}
	status := (&Wander{}).Tick(ctx)
	if status != StatusRunning {
		t.Error("should return running when move timer active")
	}
}

func TestWander_AllBlocked(t *testing.T) {
	m := &mockMonster{x: 5, y: 5, spawnX: 5, spawnY: 5}
	room := &mockRoom{moveResult: false}
	ctx := &AIContext{Monster: m, Room: room, Config: &AIProfile{WanderRadius: 4}}
	status := (&Wander{}).Tick(ctx)
	if status != StatusFailure {
		t.Error("should fail when all directions blocked")
	}
}

func TestChaseTarget_NoTarget(t *testing.T) {
	m := &mockMonster{target: 0}
	ctx := &AIContext{Monster: m, Room: &mockRoom{}}
	if (&ChaseTarget{}).Tick(ctx) != StatusFailure {
		t.Error("no target should fail")
	}
}

func TestChaseTarget_MovesToward(t *testing.T) {
	m := &mockMonster{x: 5, y: 5, target: 1}
	room := &mockRoom{
		players:    []PlayerInfo{{CharID: 1, X: 8, Y: 5, HP: 100}},
		moveResult: true,
	}
	ctx := &AIContext{Monster: m, Room: room, Config: &AIProfile{MoveIntervalTicks: 10}}
	status := (&ChaseTarget{}).Tick(ctx)
	if status != StatusRunning {
		t.Error("chase should return running")
	}
	if m.state != StateChase {
		t.Error("should set chase state")
	}
}

func TestChaseTarget_PlayerGone(t *testing.T) {
	m := &mockMonster{x: 5, y: 5, target: 99}
	room := &mockRoom{moveResult: true}
	ctx := &AIContext{Monster: m, Room: room}
	if (&ChaseTarget{}).Tick(ctx) != StatusFailure {
		t.Error("target not in room should fail")
	}
}

func TestChaseTarget_CooldownRunning(t *testing.T) {
	m := &mockMonster{x: 5, y: 5, target: 1, moveTimer: 3}
	room := &mockRoom{
		players:    []PlayerInfo{{CharID: 1, X: 8, Y: 5, HP: 100}},
		moveResult: true,
	}
	ctx := &AIContext{Monster: m, Room: room}
	if (&ChaseTarget{}).Tick(ctx) != StatusRunning {
		t.Error("should return running while on cooldown")
	}
}

func TestMoveToSpawn_AlreadyAtSpawn(t *testing.T) {
	m := &mockMonster{x: 5, y: 5, spawnX: 5, spawnY: 5, target: 1}
	tt := NewThreatTable()
	tt.AddThreat(1, 50)
	ctx := &AIContext{Monster: m, Room: &mockRoom{moveResult: true}, ThreatTable: tt}
	status := (&MoveToSpawn{}).Tick(ctx)
	if status != StatusSuccess {
		t.Error("at spawn should succeed")
	}
	if m.target != 0 {
		t.Error("should clear target at spawn")
	}
	if m.state != StateIdle {
		t.Error("should set idle state")
	}
	if tt.Len() != 0 {
		t.Error("should clear threat table")
	}
}

func TestMoveToSpawn_MovesHome(t *testing.T) {
	m := &mockMonster{x: 10, y: 5, spawnX: 5, spawnY: 5}
	room := &mockRoom{moveResult: true}
	ctx := &AIContext{Monster: m, Room: room, Config: &AIProfile{MoveIntervalTicks: 10}}
	status := (&MoveToSpawn{}).Tick(ctx)
	if status != StatusRunning {
		t.Error("not at spawn should return running")
	}
}

func TestAttackTarget_OnCooldown(t *testing.T) {
	m := &mockMonster{target: 1, attackTimer: 5}
	ctx := &AIContext{Monster: m}
	if (&AttackTarget{}).Tick(ctx) != StatusRunning {
		t.Error("on cooldown should return running")
	}
}

func TestAttackTarget_NoTarget(t *testing.T) {
	m := &mockMonster{target: 0}
	ctx := &AIContext{Monster: m}
	if (&AttackTarget{}).Tick(ctx) != StatusFailure {
		t.Error("no target should fail")
	}
}

func TestAttackTarget_FiresCallback(t *testing.T) {
	m := &mockMonster{target: 1}
	var calledWith int64
	ctx := &AIContext{
		Monster:        m,
		Config:         &AIProfile{AttackCooldownTicks: 15},
		DamageCallback: func(mon MonsterAccessor, tid int64) { calledWith = tid },
	}
	status := (&AttackTarget{}).Tick(ctx)
	if status != StatusSuccess {
		t.Error("should succeed")
	}
	if calledWith != 1 {
		t.Errorf("callback should fire with target 1, got %d", calledWith)
	}
	if m.attackTimer != 15 {
		t.Errorf("should reset attack timer to 15, got %d", m.attackTimer)
	}
	if m.state != StateAttack {
		t.Error("should set attack state")
	}
}

func TestSetStateNode(t *testing.T) {
	m := &mockMonster{}
	ctx := &AIContext{Monster: m}
	n := &SetStateNode{State: StateAlert}
	if n.Tick(ctx) != StatusSuccess {
		t.Error("should succeed")
	}
	if m.state != StateAlert {
		t.Error("should set alert state")
	}
}

// ---- Config Parser Tests ----

func TestParseAIProfile_NoTag(t *testing.T) {
	if ParseAIProfile("", nil) != nil {
		t.Error("empty note should return nil")
	}
	if ParseAIProfile("no ai tags here", nil) != nil {
		t.Error("no <AI:...> tag should return nil")
	}
}

func TestParseAIProfile_BuiltinProfile(t *testing.T) {
	p := ParseAIProfile("<AI:aggressive>", nil)
	if p == nil {
		t.Fatal("should parse aggressive profile")
	}
	if p.AggroRange != 8 {
		t.Errorf("aggressive aggroRange should be 8, got %d", p.AggroRange)
	}
}

func TestParseAIProfile_CaseInsensitive(t *testing.T) {
	p := ParseAIProfile("<AI:Aggressive>", nil)
	if p == nil {
		t.Fatal("should be case-insensitive")
	}
	if p.AggroRange != 8 {
		t.Error("should match aggressive profile")
	}
}

func TestParseAIProfile_CustomProfile(t *testing.T) {
	custom := map[string]*AIProfile{
		"ranged": {Name: "ranged", AggroRange: 10, AttackRange: 3},
	}
	p := ParseAIProfile("<AI:ranged>", custom)
	if p == nil {
		t.Fatal("should find custom profile")
	}
	if p.AttackRange != 3 {
		t.Errorf("custom attackRange should be 3, got %d", p.AttackRange)
	}
}

func TestParseAIProfile_Overrides(t *testing.T) {
	note := "<AI:passive>\n<AI Aggro Range:5>\n<AI Attack Range:2>\n<AI Flee HP:25>"
	p := ParseAIProfile(note, nil)
	if p == nil {
		t.Fatal("should parse")
	}
	if p.AggroRange != 5 {
		t.Errorf("aggro range override should be 5, got %d", p.AggroRange)
	}
	if p.AttackRange != 2 {
		t.Errorf("attack range override should be 2, got %d", p.AttackRange)
	}
	if p.FleeHPPercent != 25 {
		t.Errorf("flee hp override should be 25, got %d", p.FleeHPPercent)
	}
	// Original wander radius should be preserved.
	if p.WanderRadius != 4 {
		t.Errorf("wander radius should be preserved at 4, got %d", p.WanderRadius)
	}
}

func TestParseAIProfile_InvalidOverrideValue(t *testing.T) {
	note := "<AI:aggressive>\n<AI Aggro Range:notanumber>"
	p := ParseAIProfile(note, nil)
	if p == nil {
		t.Fatal("should still parse base profile")
	}
	if p.AggroRange != 8 {
		t.Error("invalid override should be ignored, keeping default")
	}
}

func TestParseAIProfile_AllOverrides(t *testing.T) {
	note := "<AI:aggressive>\n<AI Leash Range:20>\n<AI Attack Cooldown:30>\n<AI Move Interval:5>\n<AI Wander Radius:6>"
	p := ParseAIProfile(note, nil)
	if p == nil {
		t.Fatal("should parse")
	}
	if p.LeashRange != 20 {
		t.Errorf("leash range should be 20, got %d", p.LeashRange)
	}
	if p.AttackCooldownTicks != 30 {
		t.Errorf("attack cooldown should be 30, got %d", p.AttackCooldownTicks)
	}
	if p.MoveIntervalTicks != 5 {
		t.Errorf("move interval should be 5, got %d", p.MoveIntervalTicks)
	}
	if p.WanderRadius != 6 {
		t.Errorf("wander radius should be 6, got %d", p.WanderRadius)
	}
}

// ---- Profile Builder Tests ----

func TestBuildTree_NilProfile(t *testing.T) {
	if BuildTree(nil) != nil {
		t.Error("nil profile should return nil tree")
	}
}

func TestBuildTree_PassiveHasWanderOnly(t *testing.T) {
	bt := BuildTree(DefaultProfiles["passive"])
	if bt == nil {
		t.Fatal("passive tree should not be nil")
	}
	// Tick with a mock — passive should wander.
	m := &mockMonster{x: 5, y: 5, spawnX: 5, spawnY: 5}
	room := &mockRoom{moveResult: true}
	ctx := &AIContext{Monster: m, Room: room, Config: DefaultProfiles["passive"]}
	bt.Tick(ctx)
	// Monster should have moved (moveTimer set).
	if m.moveTimer == 0 {
		t.Error("passive tree should trigger wander movement")
	}
}

func TestBuildTree_AggressiveDetectsPlayer(t *testing.T) {
	bt := BuildTree(DefaultProfiles["aggressive"])
	if bt == nil {
		t.Fatal("aggressive tree should not be nil")
	}
	m := &mockMonster{x: 5, y: 5, spawnX: 5, spawnY: 5}
	room := &mockRoom{
		players:    []PlayerInfo{{CharID: 1, X: 6, Y: 5, HP: 100}},
		moveResult: true,
	}
	ctx := &AIContext{Monster: m, Room: room, Config: DefaultProfiles["aggressive"]}
	bt.Tick(ctx)
	if m.target != 1 {
		t.Error("aggressive tree should detect and target player")
	}
	if m.state != StateAlert {
		t.Errorf("should set alert state on detection, got %d", m.state)
	}
}

func TestBuildTree_AggressiveChasesTarget(t *testing.T) {
	bt := BuildTree(DefaultProfiles["aggressive"])
	m := &mockMonster{x: 5, y: 5, spawnX: 5, spawnY: 5, target: 1}
	room := &mockRoom{
		players:    []PlayerInfo{{CharID: 1, X: 8, Y: 5, HP: 100}},
		moveResult: true,
	}
	ctx := &AIContext{Monster: m, Room: room, Config: DefaultProfiles["aggressive"]}
	bt.Tick(ctx) // Should chase (target alive, in leash, not in attack range)
	if m.state != StateChase {
		t.Errorf("should chase target, state=%d", m.state)
	}
}

func TestBuildTree_AggressiveAttacks(t *testing.T) {
	bt := BuildTree(DefaultProfiles["aggressive"])
	var attacked bool
	m := &mockMonster{x: 5, y: 5, spawnX: 5, spawnY: 5, target: 1}
	room := &mockRoom{
		players:    []PlayerInfo{{CharID: 1, X: 6, Y: 5, HP: 100}}, // 1 tile away
		moveResult: true,
	}
	ctx := &AIContext{
		Monster:        m,
		Room:           room,
		Config:         DefaultProfiles["aggressive"],
		DamageCallback: func(MonsterAccessor, int64) { attacked = true },
	}
	bt.Tick(ctx)
	if !attacked {
		t.Error("should attack when target in range")
	}
}

func TestBuildTree_BossNoLeash(t *testing.T) {
	bt := BuildTree(DefaultProfiles["boss"])
	if bt == nil {
		t.Fatal("boss tree should not be nil")
	}
	m := &mockMonster{x: 50, y: 50, spawnX: 0, spawnY: 0, target: 1}
	room := &mockRoom{
		players:    []PlayerInfo{{CharID: 1, X: 51, Y: 50, HP: 100}},
		moveResult: true,
	}
	var attacked bool
	ctx := &AIContext{
		Monster:        m,
		Room:           room,
		Config:         DefaultProfiles["boss"],
		DamageCallback: func(MonsterAccessor, int64) { attacked = true },
	}
	bt.Tick(ctx)
	// Boss has no leash, should still attack even far from spawn.
	if !attacked {
		t.Error("boss with no leash should attack regardless of distance from spawn")
	}
}

// ---- Helper Tests ----

func TestManhattan(t *testing.T) {
	if manhattan(0, 0, 3, 4) != 7 {
		t.Error("manhattan(0,0,3,4) should be 7")
	}
	if manhattan(5, 5, 5, 5) != 0 {
		t.Error("same point should be 0")
	}
}

func TestDirDXDY(t *testing.T) {
	if dirDX(4) != -1 { t.Error("left dx should be -1") }
	if dirDX(6) != 1 { t.Error("right dx should be 1") }
	if dirDX(2) != 0 { t.Error("down dx should be 0") }
	if dirDY(2) != 1 { t.Error("down dy should be 1") }
	if dirDY(8) != -1 { t.Error("up dy should be -1") }
	if dirDY(6) != 0 { t.Error("right dy should be 0") }
}

func TestPointToDir(t *testing.T) {
	if pointToDir(5, 5, 6, 5) != 6 { t.Error("right") }
	if pointToDir(5, 5, 4, 5) != 4 { t.Error("left") }
	if pointToDir(5, 5, 5, 6) != 2 { t.Error("down") }
	if pointToDir(5, 5, 5, 4) != 8 { t.Error("up") }
	if pointToDir(5, 5, 5, 5) != 2 { t.Error("same point default") }
}

func TestSimpleDirectionToward(t *testing.T) {
	if simpleDirectionToward(5, 5, 5, 5) != 0 { t.Error("same point") }
	if simpleDirectionToward(5, 5, 8, 5) != 6 { t.Error("should go right") }
	if simpleDirectionToward(5, 5, 2, 5) != 4 { t.Error("should go left") }
	if simpleDirectionToward(5, 5, 5, 8) != 2 { t.Error("should go down") }
	if simpleDirectionToward(5, 5, 5, 2) != 8 { t.Error("should go up") }
	// Diagonal — prefer horizontal when equal.
	if d := simpleDirectionToward(5, 5, 8, 8); d != 6 {
		t.Errorf("diagonal should prefer horizontal, got %d", d)
	}
}

// ---- Pathfinding Tests ----

func TestAStar_NilMap(t *testing.T) {
	if AStar(nil, Point{0, 0}, Point{1, 1}) != nil {
		t.Error("nil map should return nil")
	}
}

func TestAStar_SamePoint(t *testing.T) {
	pm := &resource.PassabilityMap{Width: 10, Height: 10}
	path := AStar(pm, Point{5, 5}, Point{5, 5})
	if path == nil || len(path) != 0 {
		t.Error("same point should return empty path")
	}
}
