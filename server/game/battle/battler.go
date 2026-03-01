package battle

import (
	"github.com/kasuganosora/rpgmakermvmmo/server/resource"
)

// Action types.
const (
	ActionAttack = iota
	ActionSkill
	ActionItem
	ActionGuard
	ActionEscape
)

// Action represents a battle action chosen by or assigned to a battler.
type Action struct {
	Type          int
	SkillID       int
	ItemID        int
	TargetIndices []int
	TargetIsActor bool // true = targeting actors, false = targeting enemies
}

// StateEntry tracks an active state on a battler.
type StateEntry struct {
	StateID   int
	TurnsLeft int // -1 = no auto-removal, 0+ = turns remaining
}

// Battler represents any entity participating in battle.
type Battler interface {
	Name() string
	IsActor() bool
	Index() int
	CharID() int64

	// Param returns the effective stat value.
	// 0=mhp, 1=mmp, 2=atk, 3=def, 4=mat, 5=mdf, 6=agi, 7=luk
	Param(paramID int) int
	HP() int
	MP() int
	TP() int
	SetHP(v int)
	SetMP(v int)
	SetTP(v int)
	MaxHP() int
	MaxMP() int
	IsAlive() bool
	IsDead() bool

	// XParam: 0=hit, 1=eva, 2=cri, 3=cev, 4=mev, 5=mrf, 6=cnt, 7=hrg, 8=mrg, 9=trg
	XParam(id int) float64
	// SParam: 0=tgr, 1=grd, 2=rec, 3=pha, 4=mcr, 5=tcr, 6=pdr, 7=mdr, 8=fdr, 9=exr
	SParam(id int) float64

	AddState(stateID int, turns int)
	RemoveState(stateID int)
	HasState(stateID int) bool
	StateIDs() []int
	StateEntries() []StateEntry
	TickStateTurns() []int // decrement; returns expired state IDs

	CurrentAction() *Action
	SetAction(a *Action)
	ClearAction()

	IsGuarding() bool
	SetGuarding(v bool)

	AllTraits() []resource.Trait
	ElementRate(elementID int) float64
	StateRate(stateID int) float64

	SkillIDs() []int

	BuffLevel(paramID int) int
	AddBuff(paramID int, turns int)
	AddDebuff(paramID int, turns int)
	RemoveBuff(paramID int)
	TickBuffTurns()

	ToCharacterStats() *CharacterStats
}

// ---------------------------------------------------------------------------
//  baseBattler — shared implementation for actors and enemies
// ---------------------------------------------------------------------------

type baseBattler struct {
	name        string
	index       int
	hp, mp, tp  int
	baseParams  [8]int // 0=mhp,1=mmp,2=atk,3=def,4=mat,5=mdf,6=agi,7=luk
	equipBonus  [8]int
	states      []StateEntry
	action      *Action
	guarding    bool
	buffLevels  [8]int
	buffTurns   [8]int
	baseTraits  []resource.Trait // actor/class/enemy traits
	equipTraits []resource.Trait // equipment traits
	res         *resource.ResourceLoader
}

// Param returns (base + equip) * paramRate * buffRate, clamped ≥ 1 for mhp/mmp.
func (b *baseBattler) Param(paramID int) int {
	if paramID < 0 || paramID > 7 {
		return 0
	}
	base := float64(b.baseParams[paramID] + b.equipBonus[paramID])
	base *= b.paramRate(paramID)
	base *= b.buffRate(paramID)
	v := int(base)
	if paramID <= 1 && v < 1 {
		v = 1 // MHP/MMP minimum 1
	}
	if v < 0 {
		v = 0
	}
	return v
}

func (b *baseBattler) paramRate(paramID int) float64 {
	rate := 1.0
	for _, t := range b.allTraitsInternal() {
		if t.Code == 21 && t.DataID == paramID {
			rate *= t.Value
		}
	}
	return rate
}

func (b *baseBattler) buffRate(paramID int) float64 {
	return 1.0 + float64(b.buffLevels[paramID])*0.25
}

func (b *baseBattler) HP() int      { return b.hp }
func (b *baseBattler) MP() int      { return b.mp }
func (b *baseBattler) TP() int      { return b.tp }
func (b *baseBattler) MaxHP() int   { return b.Param(0) }
func (b *baseBattler) MaxMP() int   { return b.Param(1) }
func (b *baseBattler) Name() string { return b.name }
func (b *baseBattler) Index() int   { return b.index }

func (b *baseBattler) SetHP(v int) {
	mhp := b.Param(0)
	if v > mhp {
		v = mhp
	}
	if v < 0 {
		v = 0
	}
	b.hp = v
}

func (b *baseBattler) SetMP(v int) {
	mmp := b.Param(1)
	if v > mmp {
		v = mmp
	}
	if v < 0 {
		v = 0
	}
	b.mp = v
}

func (b *baseBattler) SetTP(v int) {
	if v > 100 {
		v = 100
	}
	if v < 0 {
		v = 0
	}
	b.tp = v
}

func (b *baseBattler) IsAlive() bool { return b.hp > 0 }
func (b *baseBattler) IsDead() bool  { return b.hp <= 0 }

// XParam = sum of trait code 22 values.
func (b *baseBattler) XParam(id int) float64 {
	sum := 0.0
	for _, t := range b.allTraitsInternal() {
		if t.Code == 22 && t.DataID == id {
			sum += t.Value
		}
	}
	return sum
}

// SParam = product of trait code 23 values (default 1.0).
func (b *baseBattler) SParam(id int) float64 {
	rate := 1.0
	for _, t := range b.allTraitsInternal() {
		if t.Code == 23 && t.DataID == id {
			rate *= t.Value
		}
	}
	return rate
}

// --- State management ---

func (b *baseBattler) AddState(stateID int, turns int) {
	for i := range b.states {
		if b.states[i].StateID == stateID {
			if turns > b.states[i].TurnsLeft {
				b.states[i].TurnsLeft = turns
			}
			return
		}
	}
	b.states = append(b.states, StateEntry{StateID: stateID, TurnsLeft: turns})
}

func (b *baseBattler) RemoveState(stateID int) {
	for i, s := range b.states {
		if s.StateID == stateID {
			b.states = append(b.states[:i], b.states[i+1:]...)
			return
		}
	}
}

func (b *baseBattler) HasState(stateID int) bool {
	for _, s := range b.states {
		if s.StateID == stateID {
			return true
		}
	}
	return false
}

func (b *baseBattler) StateIDs() []int {
	ids := make([]int, len(b.states))
	for i, s := range b.states {
		ids[i] = s.StateID
	}
	return ids
}

func (b *baseBattler) StateEntries() []StateEntry {
	out := make([]StateEntry, len(b.states))
	copy(out, b.states)
	return out
}

func (b *baseBattler) TickStateTurns() []int {
	var expired []int
	remaining := b.states[:0]
	for _, s := range b.states {
		if s.TurnsLeft < 0 {
			remaining = append(remaining, s)
			continue
		}
		s.TurnsLeft--
		if s.TurnsLeft <= 0 {
			expired = append(expired, s.StateID)
		} else {
			remaining = append(remaining, s)
		}
	}
	b.states = remaining
	return expired
}

// --- Action management ---

func (b *baseBattler) CurrentAction() *Action { return b.action }
func (b *baseBattler) SetAction(a *Action)    { b.action = a }
func (b *baseBattler) ClearAction()           { b.action = nil }

// --- Guard ---

func (b *baseBattler) IsGuarding() bool   { return b.guarding }
func (b *baseBattler) SetGuarding(v bool) { b.guarding = v }

// --- Buff management ---

func (b *baseBattler) BuffLevel(paramID int) int {
	if paramID < 0 || paramID > 7 {
		return 0
	}
	return b.buffLevels[paramID]
}

func (b *baseBattler) AddBuff(paramID int, turns int) {
	if paramID < 0 || paramID > 7 {
		return
	}
	b.buffLevels[paramID]++
	if b.buffLevels[paramID] > 2 {
		b.buffLevels[paramID] = 2
	}
	b.buffTurns[paramID] = turns
}

func (b *baseBattler) AddDebuff(paramID int, turns int) {
	if paramID < 0 || paramID > 7 {
		return
	}
	b.buffLevels[paramID]--
	if b.buffLevels[paramID] < -2 {
		b.buffLevels[paramID] = -2
	}
	b.buffTurns[paramID] = turns
}

func (b *baseBattler) RemoveBuff(paramID int) {
	if paramID < 0 || paramID > 7 {
		return
	}
	b.buffLevels[paramID] = 0
	b.buffTurns[paramID] = 0
}

func (b *baseBattler) TickBuffTurns() {
	for i := range b.buffLevels {
		if b.buffLevels[i] == 0 {
			continue
		}
		b.buffTurns[i]--
		if b.buffTurns[i] <= 0 {
			b.buffLevels[i] = 0
			b.buffTurns[i] = 0
		}
	}
}

// --- Traits ---

func (b *baseBattler) allTraitsInternal() []resource.Trait {
	var all []resource.Trait
	all = append(all, b.baseTraits...)
	all = append(all, b.equipTraits...)
	if b.res != nil {
		for _, se := range b.states {
			for _, st := range b.res.States {
				if st != nil && st.ID == se.StateID {
					all = append(all, st.Traits...)
					break
				}
			}
		}
	}
	return all
}

func (b *baseBattler) AllTraits() []resource.Trait {
	return b.allTraitsInternal()
}

func (b *baseBattler) ElementRate(elementID int) float64 {
	rate := 1.0
	for _, t := range b.allTraitsInternal() {
		if t.Code == 11 && t.DataID == elementID {
			rate *= t.Value
		}
	}
	return rate
}

func (b *baseBattler) StateRate(stateID int) float64 {
	rate := 1.0
	for _, t := range b.allTraitsInternal() {
		if t.Code == 13 && t.DataID == stateID {
			rate *= t.Value
		}
	}
	return rate
}

func (b *baseBattler) ToCharacterStats() *CharacterStats {
	return &CharacterStats{
		HP:  b.hp,
		MP:  b.mp,
		Atk: b.Param(2),
		Def: b.Param(3),
		Mat: b.Param(4),
		Mdf: b.Param(5),
		Agi: b.Param(6),
		Luk: b.Param(7),
	}
}

// ---------------------------------------------------------------------------
//  ActorBattler
// ---------------------------------------------------------------------------

// ActorBattler represents a player character in battle.
type ActorBattler struct {
	baseBattler
	charID  int64
	level   int
	classID int
	skills  []int
}

// ActorConfig holds the data needed to construct an ActorBattler.
type ActorConfig struct {
	CharID      int64
	Name        string
	Index       int
	ClassID     int
	Level       int
	HP, MP      int
	BaseParams  [8]int           // from Classes.json params at level
	EquipBonus  [8]int           // sum of equipped Weapon/Armor params
	Skills      []int            // learned skill IDs
	ActorTraits []resource.Trait // actor + class traits merged
	EquipTraits []resource.Trait // all equipment traits merged
	Res         *resource.ResourceLoader
}

// NewActorBattler creates an ActorBattler from the given config.
func NewActorBattler(cfg ActorConfig) *ActorBattler {
	a := &ActorBattler{
		charID:  cfg.CharID,
		level:   cfg.Level,
		classID: cfg.ClassID,
		skills:  cfg.Skills,
	}
	a.name = cfg.Name
	a.index = cfg.Index
	a.baseParams = cfg.BaseParams
	a.equipBonus = cfg.EquipBonus
	a.hp = cfg.HP
	a.mp = cfg.MP
	a.baseTraits = cfg.ActorTraits
	a.equipTraits = cfg.EquipTraits
	a.res = cfg.Res

	// Clamp HP/MP to max.
	if mhp := a.Param(0); a.hp > mhp {
		a.hp = mhp
	}
	if mmp := a.Param(1); a.mp > mmp {
		a.mp = mmp
	}
	return a
}

func (a *ActorBattler) IsActor() bool { return true }
func (a *ActorBattler) CharID() int64 { return a.charID }
func (a *ActorBattler) Level() int    { return a.level }
func (a *ActorBattler) ClassID() int  { return a.classID }

func (a *ActorBattler) SkillIDs() []int {
	out := make([]int, len(a.skills))
	copy(out, a.skills)
	return out
}

func (a *ActorBattler) ToCharacterStats() *CharacterStats {
	s := a.baseBattler.ToCharacterStats()
	s.Level = a.level
	return s
}

// ---------------------------------------------------------------------------
//  EnemyBattler
// ---------------------------------------------------------------------------

// EnemyBattler represents a monster in battle.
type EnemyBattler struct {
	baseBattler
	enemyID int
	enemy   *resource.Enemy
}

// NewEnemyBattler creates an EnemyBattler from enemy resource data.
func NewEnemyBattler(enemy *resource.Enemy, index int, res *resource.ResourceLoader) *EnemyBattler {
	e := &EnemyBattler{
		enemyID: enemy.ID,
		enemy:   enemy,
	}
	e.name = enemy.Name
	e.index = index
	e.baseParams = [8]int{
		enemy.HP, enemy.MP,
		enemy.Atk, enemy.Def,
		enemy.Mat, enemy.Mdf,
		enemy.Agi, enemy.Luk,
	}
	e.hp = enemy.HP
	e.mp = enemy.MP
	e.baseTraits = enemy.Traits
	e.res = res
	return e
}

func (e *EnemyBattler) IsActor() bool          { return false }
func (e *EnemyBattler) CharID() int64           { return 0 }
func (e *EnemyBattler) Enemy() *resource.Enemy  { return e.enemy }
func (e *EnemyBattler) EnemyID() int            { return e.enemyID }

// SkillIDs returns unique skill IDs from the enemy's action table.
func (e *EnemyBattler) SkillIDs() []int {
	if e.enemy == nil {
		return nil
	}
	seen := make(map[int]bool)
	var ids []int
	for _, a := range e.enemy.Actions {
		if !seen[a.SkillID] {
			seen[a.SkillID] = true
			ids = append(ids, a.SkillID)
		}
	}
	return ids
}

func (e *EnemyBattler) ToCharacterStats() *CharacterStats {
	s := e.baseBattler.ToCharacterStats()
	s.Level = 1
	return s
}
