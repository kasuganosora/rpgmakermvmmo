package battle

import (
	"testing"

	"github.com/kasuganosora/rpgmakermvmmo/server/resource"
)

func makeTestRes() *resource.ResourceLoader {
	return &resource.ResourceLoader{
		States: []*resource.State{
			nil, // index 0
			{ID: 1, Name: "Death"},
			{ID: 2, Name: "Poison", AutoRemovalTiming: 2, MinTurns: 3, MaxTurns: 5,
				Traits: []resource.Trait{
					{Code: 21, DataID: 7, Value: 0.5}, // luk * 0.5
				},
			},
			{ID: 3, Name: "ATK Up",
				Traits: []resource.Trait{
					{Code: 21, DataID: 2, Value: 1.5}, // atk * 1.5
				},
			},
		},
	}
}

func makeTestActor(res *resource.ResourceLoader) *ActorBattler {
	return NewActorBattler(ActorConfig{
		CharID: 100,
		Name:   "Hero",
		Index:  0,
		ClassID: 1,
		Level:   10,
		HP:      200,
		MP:      50,
		BaseParams: [8]int{250, 80, 30, 20, 25, 18, 15, 10},
		EquipBonus: [8]int{0, 0, 10, 5, 0, 0, 3, 0},
		Skills:     []int{1, 2, 3},
		ActorTraits: []resource.Trait{
			{Code: 22, DataID: 0, Value: 0.95}, // hit rate 95%
			{Code: 22, DataID: 2, Value: 0.04}, // crit rate 4%
		},
		EquipTraits: []resource.Trait{
			{Code: 22, DataID: 2, Value: 0.05}, // +5% crit from weapon
		},
		Res: res,
	})
}

func makeTestEnemy(res *resource.ResourceLoader) *EnemyBattler {
	return NewEnemyBattler(&resource.Enemy{
		ID: 1, Name: "Slime",
		HP: 100, MP: 20,
		Atk: 15, Def: 10, Mat: 8, Mdf: 6, Agi: 12, Luk: 5,
		Exp: 30, Gold: 10,
		Actions: []resource.EnemyAction{
			{SkillID: 1, Rating: 5}, // attack
			{SkillID: 4, Rating: 3, ConditionType: 2, ConditionParam1: 0, ConditionParam2: 50}, // heal when HP < 50%
		},
		Traits: []resource.Trait{
			{Code: 22, DataID: 0, Value: 0.95}, // hit 95%
			{Code: 11, DataID: 2, Value: 1.5},  // weak to fire (element 2)
		},
	}, 0, res)
}

// --- Creation tests ---

func TestActorBattlerCreation(t *testing.T) {
	res := makeTestRes()
	a := makeTestActor(res)

	if a.Name() != "Hero" {
		t.Errorf("name = %q, want Hero", a.Name())
	}
	if !a.IsActor() {
		t.Error("IsActor() = false")
	}
	if a.CharID() != 100 {
		t.Errorf("CharID = %d, want 100", a.CharID())
	}
	if a.Level() != 10 {
		t.Errorf("Level = %d, want 10", a.Level())
	}
	if a.Index() != 0 {
		t.Errorf("Index = %d, want 0", a.Index())
	}
}

func TestEnemyBattlerCreation(t *testing.T) {
	res := makeTestRes()
	e := makeTestEnemy(res)

	if e.Name() != "Slime" {
		t.Errorf("name = %q, want Slime", e.Name())
	}
	if e.IsActor() {
		t.Error("IsActor() = true for enemy")
	}
	if e.CharID() != 0 {
		t.Errorf("CharID = %d, want 0", e.CharID())
	}
	if e.EnemyID() != 1 {
		t.Errorf("EnemyID = %d, want 1", e.EnemyID())
	}
}

// --- Param tests ---

func TestActorParams(t *testing.T) {
	res := makeTestRes()
	a := makeTestActor(res)

	// MHP = base 250 + equip 0 = 250
	if got := a.Param(0); got != 250 {
		t.Errorf("MHP = %d, want 250", got)
	}
	// ATK = base 30 + equip 10 = 40
	if got := a.Param(2); got != 40 {
		t.Errorf("ATK = %d, want 40", got)
	}
	// DEF = base 20 + equip 5 = 25
	if got := a.Param(3); got != 25 {
		t.Errorf("DEF = %d, want 25", got)
	}
	// AGI = base 15 + equip 3 = 18
	if got := a.Param(6); got != 18 {
		t.Errorf("AGI = %d, want 18", got)
	}
}

func TestEnemyParams(t *testing.T) {
	res := makeTestRes()
	e := makeTestEnemy(res)

	if got := e.Param(0); got != 100 {
		t.Errorf("MHP = %d, want 100", got)
	}
	if got := e.Param(2); got != 15 {
		t.Errorf("ATK = %d, want 15", got)
	}
}

func TestParamWithTraitModifier(t *testing.T) {
	res := makeTestRes()
	a := makeTestActor(res)

	// Add ATK Up state (code=21, dataID=2, value=1.5)
	a.AddState(3, 5)
	// ATK = (30 + 10) * 1.5 = 60
	if got := a.Param(2); got != 60 {
		t.Errorf("ATK with state = %d, want 60", got)
	}
}

func TestParamWithBuff(t *testing.T) {
	res := makeTestRes()
	a := makeTestActor(res)

	a.AddBuff(2, 5) // +1 ATK buff
	// ATK = (30 + 10) * 1.0 * 1.25 = 50
	if got := a.Param(2); got != 50 {
		t.Errorf("ATK with buff = %d, want 50", got)
	}

	a.AddBuff(2, 5) // +2 ATK buff
	// ATK = (30 + 10) * 1.0 * 1.50 = 60
	if got := a.Param(2); got != 60 {
		t.Errorf("ATK with buff+2 = %d, want 60", got)
	}
}

func TestParamWithDebuff(t *testing.T) {
	res := makeTestRes()
	e := makeTestEnemy(res)

	e.AddDebuff(3, 3) // -1 DEF debuff
	// DEF = 10 * 1.0 * 0.75 = 7
	if got := e.Param(3); got != 7 {
		t.Errorf("DEF with debuff = %d, want 7", got)
	}
}

func TestParamMinimums(t *testing.T) {
	res := makeTestRes()
	// MHP minimum is 1 even with heavy debuffs
	a := NewActorBattler(ActorConfig{
		Name:       "Weak",
		BaseParams: [8]int{1, 1, 0, 0, 0, 0, 0, 0},
		Res:        res,
	})
	a.AddDebuff(0, 5)
	a.AddDebuff(0, 5) // -2
	// MHP = 1 * 0.5 = 0 → clamped to 1
	if got := a.Param(0); got < 1 {
		t.Errorf("MHP should be >= 1, got %d", got)
	}
}

// --- HP/MP/TP tests ---

func TestHPClamping(t *testing.T) {
	res := makeTestRes()
	a := makeTestActor(res)

	a.SetHP(999)
	if a.HP() != a.MaxHP() {
		t.Errorf("HP = %d, want MaxHP %d", a.HP(), a.MaxHP())
	}
	a.SetHP(-10)
	if a.HP() != 0 {
		t.Errorf("HP = %d, want 0", a.HP())
	}
}

func TestMPClamping(t *testing.T) {
	res := makeTestRes()
	a := makeTestActor(res)

	a.SetMP(999)
	if a.MP() != a.MaxMP() {
		t.Errorf("MP = %d, want MaxMP %d", a.MP(), a.MaxMP())
	}
}

func TestTPClamping(t *testing.T) {
	res := makeTestRes()
	a := makeTestActor(res)

	a.SetTP(150)
	if a.TP() != 100 {
		t.Errorf("TP = %d, want 100", a.TP())
	}
	a.SetTP(-5)
	if a.TP() != 0 {
		t.Errorf("TP = %d, want 0", a.TP())
	}
}

func TestAliveAndDead(t *testing.T) {
	res := makeTestRes()
	a := makeTestActor(res)

	if !a.IsAlive() {
		t.Error("should be alive")
	}
	a.SetHP(0)
	if !a.IsDead() {
		t.Error("should be dead")
	}
	if a.IsAlive() {
		t.Error("should not be alive when HP=0")
	}
}

func TestInitialHPClamp(t *testing.T) {
	res := makeTestRes()
	// HP > MHP → clamped at creation
	a := NewActorBattler(ActorConfig{
		Name:       "Over",
		HP:         9999,
		MP:         9999,
		BaseParams: [8]int{100, 50, 10, 10, 10, 10, 10, 10},
		Res:        res,
	})
	if a.HP() != 100 {
		t.Errorf("HP = %d, want 100 (clamped to MHP)", a.HP())
	}
	if a.MP() != 50 {
		t.Errorf("MP = %d, want 50 (clamped to MMP)", a.MP())
	}
}

// --- State tests ---

func TestStateAddRemove(t *testing.T) {
	res := makeTestRes()
	a := makeTestActor(res)

	a.AddState(2, 3)
	if !a.HasState(2) {
		t.Error("should have state 2")
	}
	ids := a.StateIDs()
	if len(ids) != 1 || ids[0] != 2 {
		t.Errorf("StateIDs = %v, want [2]", ids)
	}

	a.RemoveState(2)
	if a.HasState(2) {
		t.Error("should not have state 2 after removal")
	}
}

func TestStateDuplicateRefreshes(t *testing.T) {
	res := makeTestRes()
	a := makeTestActor(res)

	a.AddState(2, 3)
	a.AddState(2, 5) // refresh to 5 turns
	entries := a.StateEntries()
	if len(entries) != 1 {
		t.Fatalf("expected 1 state entry, got %d", len(entries))
	}
	if entries[0].TurnsLeft != 5 {
		t.Errorf("TurnsLeft = %d, want 5", entries[0].TurnsLeft)
	}
}

func TestStateDuplicateNoDowngrade(t *testing.T) {
	res := makeTestRes()
	a := makeTestActor(res)

	a.AddState(2, 5)
	a.AddState(2, 2) // should NOT downgrade
	entries := a.StateEntries()
	if entries[0].TurnsLeft != 5 {
		t.Errorf("TurnsLeft = %d, want 5 (should not downgrade)", entries[0].TurnsLeft)
	}
}

func TestTickStateTurns(t *testing.T) {
	res := makeTestRes()
	a := makeTestActor(res)

	a.AddState(2, 2) // 2 turns
	a.AddState(3, -1) // permanent

	expired := a.TickStateTurns()
	if len(expired) != 0 {
		t.Errorf("tick 1: expired = %v, want none", expired)
	}
	if !a.HasState(2) {
		t.Error("state 2 should still exist after tick 1")
	}

	expired = a.TickStateTurns()
	if len(expired) != 1 || expired[0] != 2 {
		t.Errorf("tick 2: expired = %v, want [2]", expired)
	}
	if a.HasState(2) {
		t.Error("state 2 should be removed after tick 2")
	}
	if !a.HasState(3) {
		t.Error("permanent state 3 should still exist")
	}
}

func TestStateTraitsAffectParams(t *testing.T) {
	res := makeTestRes()
	a := makeTestActor(res)

	baseLuk := a.Param(7) // LUK = 10 + 0 = 10
	a.AddState(2, 3)      // Poison: luk * 0.5
	poisonLuk := a.Param(7)

	if poisonLuk != baseLuk/2 {
		t.Errorf("LUK with poison = %d, want %d", poisonLuk, baseLuk/2)
	}

	a.RemoveState(2)
	if a.Param(7) != baseLuk {
		t.Errorf("LUK after poison removal = %d, want %d", a.Param(7), baseLuk)
	}
}

// --- Buff tests ---

func TestBuffLevelClamping(t *testing.T) {
	res := makeTestRes()
	a := makeTestActor(res)

	for i := 0; i < 5; i++ {
		a.AddBuff(2, 10)
	}
	if a.BuffLevel(2) != 2 {
		t.Errorf("buff level = %d, want 2 (max)", a.BuffLevel(2))
	}

	for i := 0; i < 10; i++ {
		a.AddDebuff(2, 10)
	}
	if a.BuffLevel(2) != -2 {
		t.Errorf("buff level = %d, want -2 (min)", a.BuffLevel(2))
	}
}

func TestTickBuffTurns(t *testing.T) {
	res := makeTestRes()
	a := makeTestActor(res)

	a.AddBuff(2, 2) // 2 turns
	a.TickBuffTurns()
	if a.BuffLevel(2) != 1 {
		t.Errorf("buff should still be active, level = %d", a.BuffLevel(2))
	}
	a.TickBuffTurns()
	if a.BuffLevel(2) != 0 {
		t.Errorf("buff should have expired, level = %d", a.BuffLevel(2))
	}
}

func TestRemoveBuff(t *testing.T) {
	res := makeTestRes()
	a := makeTestActor(res)

	a.AddBuff(2, 10)
	a.RemoveBuff(2)
	if a.BuffLevel(2) != 0 {
		t.Errorf("buff level = %d after removal, want 0", a.BuffLevel(2))
	}
}

// --- XParam / SParam tests ---

func TestXParam(t *testing.T) {
	res := makeTestRes()
	a := makeTestActor(res)

	// Hit rate: actor 0.95 (from actorTraits)
	hit := a.XParam(0)
	if hit != 0.95 {
		t.Errorf("hit rate = %f, want 0.95", hit)
	}

	// Crit rate: actor 0.04 + equip 0.05 = 0.09
	cri := a.XParam(2)
	if cri != 0.09 {
		t.Errorf("crit rate = %f, want 0.09", cri)
	}
}

func TestSParam(t *testing.T) {
	res := makeTestRes()
	a := makeTestActor(res)

	// No SParam traits → default 1.0
	tgr := a.SParam(0)
	if tgr != 1.0 {
		t.Errorf("tgr = %f, want 1.0", tgr)
	}
}

// --- Element / State rate tests ---

func TestElementRate(t *testing.T) {
	res := makeTestRes()
	e := makeTestEnemy(res)

	// Enemy has fire weakness: code=11, dataID=2, value=1.5
	fire := e.ElementRate(2)
	if fire != 1.5 {
		t.Errorf("fire rate = %f, want 1.5", fire)
	}

	// No ice trait → default 1.0
	ice := e.ElementRate(3)
	if ice != 1.0 {
		t.Errorf("ice rate = %f, want 1.0", ice)
	}
}

func TestStateRate(t *testing.T) {
	res := makeTestRes()
	a := makeTestActor(res)

	// No state rate traits → default 1.0
	rate := a.StateRate(1)
	if rate != 1.0 {
		t.Errorf("state rate = %f, want 1.0", rate)
	}
}

// --- Action tests ---

func TestActionSetClear(t *testing.T) {
	res := makeTestRes()
	a := makeTestActor(res)

	if a.CurrentAction() != nil {
		t.Error("initial action should be nil")
	}

	act := &Action{Type: ActionAttack, TargetIndices: []int{0}}
	a.SetAction(act)
	if a.CurrentAction() != act {
		t.Error("action not set")
	}

	a.ClearAction()
	if a.CurrentAction() != nil {
		t.Error("action should be nil after clear")
	}
}

// --- Guard tests ---

func TestGuard(t *testing.T) {
	res := makeTestRes()
	a := makeTestActor(res)

	if a.IsGuarding() {
		t.Error("should not be guarding initially")
	}
	a.SetGuarding(true)
	if !a.IsGuarding() {
		t.Error("should be guarding")
	}
}

// --- SkillIDs tests ---

func TestActorSkillIDs(t *testing.T) {
	res := makeTestRes()
	a := makeTestActor(res)

	ids := a.SkillIDs()
	if len(ids) != 3 {
		t.Fatalf("len = %d, want 3", len(ids))
	}
	if ids[0] != 1 || ids[1] != 2 || ids[2] != 3 {
		t.Errorf("skills = %v, want [1,2,3]", ids)
	}
}

func TestEnemySkillIDs(t *testing.T) {
	res := makeTestRes()
	e := makeTestEnemy(res)

	ids := e.SkillIDs()
	if len(ids) != 2 {
		t.Fatalf("len = %d, want 2", len(ids))
	}
}

// --- ToCharacterStats tests ---

func TestToCharacterStats(t *testing.T) {
	res := makeTestRes()
	a := makeTestActor(res)

	s := a.ToCharacterStats()
	if s.HP != 200 {
		t.Errorf("HP = %d, want 200", s.HP)
	}
	if s.Atk != 40 { // 30 + 10
		t.Errorf("Atk = %d, want 40", s.Atk)
	}
	if s.Level != 10 {
		t.Errorf("Level = %d, want 10", s.Level)
	}
}

func TestEnemyToCharacterStats(t *testing.T) {
	res := makeTestRes()
	e := makeTestEnemy(res)

	s := e.ToCharacterStats()
	if s.HP != 100 {
		t.Errorf("HP = %d, want 100", s.HP)
	}
	if s.Atk != 15 {
		t.Errorf("Atk = %d, want 15", s.Atk)
	}
	if s.Level != 1 {
		t.Errorf("Level = %d, want 1", s.Level)
	}
}

// --- Interface compliance ---

func TestBattlerInterfaceCompliance(t *testing.T) {
	res := makeTestRes()
	var _ Battler = makeTestActor(res)
	var _ Battler = makeTestEnemy(res)
}
