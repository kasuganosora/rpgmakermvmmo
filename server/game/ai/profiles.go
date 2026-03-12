package ai

// AIProfile defines the behavioral parameters for a monster AI.
type AIProfile struct {
	Name               string `json:"name"`
	AggroRange         int    `json:"aggroRange"`         // tiles to detect players (0 = passive)
	LeashRange         int    `json:"leashRange"`         // max distance from spawn before returning (0 = infinite)
	AttackRange        int    `json:"attackRange"`        // tiles to attack (1 = melee)
	AttackCooldownTicks int   `json:"attackCooldownTicks"` // ticks between attacks
	MoveIntervalTicks  int    `json:"moveIntervalTicks"`  // ticks between movement steps
	WanderRadius       int    `json:"wanderRadius"`       // max tiles from spawn for wander
	FleeHPPercent      int    `json:"fleeHPPercent"`      // HP% threshold to trigger flee (0 = never)
}

// Built-in profile templates.
var DefaultProfiles = map[string]*AIProfile{
	"passive": {
		Name:              "passive",
		AggroRange:        0,
		LeashRange:        8,
		MoveIntervalTicks: 40, // 2 seconds per step
		WanderRadius:      4,
	},
	"aggressive": {
		Name:               "aggressive",
		AggroRange:         8,
		LeashRange:         15,
		AttackRange:        1,
		AttackCooldownTicks: 20, // 1 second
		MoveIntervalTicks:  10, // 0.5 seconds per step
		WanderRadius:       4,
	},
	"territorial": {
		Name:               "territorial",
		AggroRange:         5,
		LeashRange:         8,
		AttackRange:        1,
		AttackCooldownTicks: 20,
		MoveIntervalTicks:  10,
		WanderRadius:       3,
	},
	"boss": {
		Name:               "boss",
		AggroRange:         12,
		LeashRange:         0, // infinite
		AttackRange:        1,
		AttackCooldownTicks: 15,
		MoveIntervalTicks:  8,
		WanderRadius:       0, // stand still when idle
	},
}

// BuildTree creates a BehaviorTree for the given profile.
func BuildTree(profile *AIProfile) *BehaviorTree {
	if profile == nil {
		return nil
	}
	if profile.AggroRange <= 0 {
		return buildPassiveTree(profile)
	}
	return buildAggressiveTree(profile)
}

// buildPassiveTree creates a tree that only wanders.
func buildPassiveTree(cfg *AIProfile) *BehaviorTree {
	return &BehaviorTree{
		Root: &Wander{},
	}
}

// buildAggressiveTree creates a tree that detects, chases, attacks, and returns.
func buildAggressiveTree(cfg *AIProfile) *BehaviorTree {
	root := &Selector{Children: []Node{
		// Priority 1: Flee when low HP (if configured)
		buildFleeBranch(cfg),
		// Priority 2: Attack/Chase current target
		buildCombatBranch(cfg),
		// Priority 3: Detect new target
		buildDetectBranch(cfg),
		// Priority 4: Return to spawn if beyond leash
		buildReturnBranch(cfg),
		// Priority 5: Idle wander
		&Wander{},
	}}
	return &BehaviorTree{Root: root}
}

func buildFleeBranch(cfg *AIProfile) Node {
	if cfg.FleeHPPercent <= 0 {
		// No flee behavior — always fail so selector moves on.
		return &ConditionNode{Fn: func(*AIContext) bool { return false }}
	}
	return &Sequence{Children: []Node{
		&CheckHPBelow{},
		&SetStateNode{State: StateWander},
		&MoveToSpawn{},
	}}
}

func buildCombatBranch(cfg *AIProfile) Node {
	return &Sequence{Children: []Node{
		&CheckTargetAlive{},
		&CheckLeashRange{},
		&Selector{Children: []Node{
			&Sequence{Children: []Node{
				&CheckAttackRange{},
				&AttackTarget{},
			}},
			&ChaseTarget{},
		}},
	}}
}

func buildDetectBranch(cfg *AIProfile) Node {
	return &Sequence{Children: []Node{
		&CheckPlayerInRange{},
		&SetStateNode{State: StateAlert},
	}}
}

func buildReturnBranch(cfg *AIProfile) Node {
	if cfg.LeashRange <= 0 {
		// No leash — always fail so selector moves on.
		return &ConditionNode{Fn: func(*AIContext) bool { return false }}
	}
	return &Sequence{Children: []Node{
		&Inverter{Child: &CheckLeashRange{}},
		&SetStateNode{State: StateWander},
		&MoveToSpawn{},
	}}
}
