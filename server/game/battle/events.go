package battle

// BattleEvent is emitted by BattleInstance for the WS layer to consume.
type BattleEvent interface {
	EventType() string
}

// BattlerRef identifies a battler in event payloads.
type BattlerRef struct {
	Index   int    `json:"index"`
	IsActor bool   `json:"is_actor"`
	Name    string `json:"name"`
}

// BattlerSnapshot is a full snapshot of a battler's state.
type BattlerSnapshot struct {
	Index      int    `json:"index"`
	IsActor    bool   `json:"is_actor"`
	Name       string `json:"name"`
	HP         int    `json:"hp"`
	MaxHP      int    `json:"max_hp"`
	MP         int    `json:"mp"`
	MaxMP      int    `json:"max_mp"`
	TP         int    `json:"tp"`
	States     []int  `json:"states"`
	Params     [8]int `json:"params"`                  // effective params: mhp,mmp,atk,def,mat,mdf,agi,luk
	BuffLevels [8]int `json:"buff_levels"`             // buff/debuff level per param (-2 to +2)
	Skills     []int  `json:"skills,omitempty"`        // learned skill IDs (actors)
	EnemyID    int    `json:"enemy_id,omitempty"`      // for enemies: RMMV enemy database ID
	ClassID    int    `json:"class_id,omitempty"`      // for actors: RMMV class database ID
	Level      int    `json:"level,omitempty"`         // for actors: current level
}

func SnapshotBattler(b Battler) BattlerSnapshot {
	s := BattlerSnapshot{
		Index:   b.Index(),
		IsActor: b.IsActor(),
		Name:    b.Name(),
		HP:      b.HP(),
		MaxHP:   b.MaxHP(),
		MP:      b.MP(),
		MaxMP:   b.MaxMP(),
		TP:      b.TP(),
		States:  b.StateIDs(),
	}
	for i := 0; i < 8; i++ {
		s.Params[i] = b.Param(i)
		s.BuffLevels[i] = b.BuffLevel(i)
	}
	if eb, ok := b.(*EnemyBattler); ok {
		s.EnemyID = eb.EnemyID()
	}
	if ab, ok := b.(*ActorBattler); ok {
		s.ClassID = ab.ClassID()
		s.Level = ab.Level()
		s.Skills = ab.SkillIDs()
	}
	return s
}

func RefBattler(b Battler) BattlerRef {
	return BattlerRef{Index: b.Index(), IsActor: b.IsActor(), Name: b.Name()}
}

// --- Concrete event types ---

type EventBattleStart struct {
	Actors       []BattlerSnapshot `json:"actors"`
	Enemies      []BattlerSnapshot `json:"enemies"`
	Battleback1  string            `json:"battleback1,omitempty"`
	Battleback2  string            `json:"battleback2,omitempty"`
	GameVars     map[int]int       `json:"game_vars,omitempty"`     // player variable snapshot for client UI
	GameSwitches map[int]bool      `json:"game_switches,omitempty"` // player switch snapshot (e.g. switch 131 transformation)
}

func (EventBattleStart) EventType() string { return "battle_start" }

type EventTurnStart struct {
	TurnCount int          `json:"turn_count"`
	Order     []BattlerRef `json:"order"`
}

func (EventTurnStart) EventType() string { return "turn_start" }

type EventInputRequest struct {
	ActorIndex int `json:"actor_index"`
}

func (EventInputRequest) EventType() string { return "input_request" }

// BuffChange records a single buff/debuff change on a target.
type BuffChange struct {
	ParamID  int `json:"param_id"`  // 0-7
	NewLevel int `json:"new_level"` // -2 to +2
	Turns    int `json:"turns"`
}

type ActionResultTarget struct {
	Target         BattlerRef   `json:"target"`
	Damage         int          `json:"damage"` // positive=damage, negative=heal
	Critical       bool         `json:"critical"`
	Missed         bool         `json:"missed"`
	HPAfter        int          `json:"hp_after"`
	MPAfter        int          `json:"mp_after"`
	TPAfter        int          `json:"tp_after"`
	AddedStates    []int        `json:"added_states,omitempty"`
	RemovedStates  []int        `json:"removed_states,omitempty"`
	AddedBuffs     []BuffChange `json:"added_buffs,omitempty"`
	CommonEventIDs []int        `json:"common_event_ids,omitempty"`
}

type EventActionResult struct {
	Subject       BattlerRef           `json:"subject"`
	SkillID       int                  `json:"skill_id"`
	ItemID        int                  `json:"item_id,omitempty"`
	Targets       []ActionResultTarget `json:"targets"`
	SubjectHPAfter int                 `json:"subject_hp_after"` // subject HP after costs/drain
	SubjectMPAfter int                 `json:"subject_mp_after"`
	SubjectTPAfter int                 `json:"subject_tp_after"`
}

func (EventActionResult) EventType() string { return "action_result" }

type RegenEntry struct {
	Battler  BattlerRef `json:"battler"`
	HPChange int        `json:"hp_change"`
	MPChange int        `json:"mp_change"`
	TPChange int        `json:"tp_change"`
}

type EventTurnEnd struct {
	Regen         []RegenEntry     `json:"regen,omitempty"`
	ExpiredStates map[string][]int `json:"expired_states,omitempty"` // "actor_0" → [stateIDs]
	ExpiredBuffs  map[string][]int `json:"expired_buffs,omitempty"`  // "actor_0" → [paramIDs]
}

func (EventTurnEnd) EventType() string { return "turn_end" }

type LevelUpEntry struct {
	ActorIndex int `json:"actor_index"`
	NewLevel   int `json:"new_level"`
}

type EventBattleEnd struct {
	Result    int           `json:"result"` // 0=win, 1=escape, 2=lose
	Exp       int           `json:"exp"`
	Gold      int           `json:"gold"`
	Drops     []DropResult  `json:"drops,omitempty"`
	LevelUps  []LevelUpEntry `json:"level_ups,omitempty"`
}

func (EventBattleEnd) EventType() string { return "battle_end" }

// EventActorEscape is emitted when a single actor escapes from a party battle.
// The battle continues for remaining actors.
type EventActorEscape struct {
	ActorIndex int `json:"actor_index"`
}

func (EventActorEscape) EventType() string { return "actor_escape" }

// EventEnemyEscape is emitted when an enemy escapes from battle (effect 41).
type EventEnemyEscape struct {
	EnemyIndex int `json:"enemy_index"`
}

func (EventEnemyEscape) EventType() string { return "enemy_escape" }

// EventTroopCommand is emitted when a troop battle event executes a command
// that needs client-side handling (Play SE, Plugin Command, Script, etc.).
type EventTroopCommand struct {
	Code   int           `json:"code"`
	Params []interface{} `json:"params,omitempty"`
}

func (EventTroopCommand) EventType() string { return "troop_command" }
