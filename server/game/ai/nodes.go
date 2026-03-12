package ai

import "math/rand"

// ---- Condition Nodes ----

// CheckPlayerInRange checks if any player is within AggroRange.
// On success, sets the monster's target to the nearest player (or top threat).
type CheckPlayerInRange struct{}

func (n *CheckPlayerInRange) Tick(ctx *AIContext) Status {
	m := ctx.Monster
	if ctx.Config == nil || ctx.Config.AggroRange <= 0 {
		return StatusFailure
	}
	x, y := m.Position()

	// Prefer top threat if available.
	if tt := ctx.ThreatTable; tt != nil && tt.Len() > 0 {
		topID := tt.TopThreat()
		if topID != 0 {
			if p := ctx.Room.PlayerByID(topID); p != nil && p.HP > 0 {
				m.SetTarget(topID)
				return StatusSuccess
			}
			// Top threat player gone, remove them.
			tt.Remove(topID)
		}
	}

	players := ctx.Room.PlayersInRange(x, y, ctx.Config.AggroRange)
	if len(players) == 0 {
		return StatusFailure
	}
	// Pick nearest.
	best := players[0]
	bestDist := manhattan(x, y, best.X, best.Y)
	for _, p := range players[1:] {
		d := manhattan(x, y, p.X, p.Y)
		if d < bestDist {
			bestDist = d
			best = p
		}
	}
	if best.HP <= 0 {
		return StatusFailure
	}
	m.SetTarget(best.CharID)
	return StatusSuccess
}

// CheckTargetAlive verifies the current target exists in the room and has HP > 0.
type CheckTargetAlive struct{}

func (n *CheckTargetAlive) Tick(ctx *AIContext) Status {
	targetID := ctx.Monster.GetTarget()
	if targetID == 0 {
		return StatusFailure
	}
	p := ctx.Room.PlayerByID(targetID)
	if p == nil || p.HP <= 0 {
		ctx.Monster.SetTarget(0)
		return StatusFailure
	}
	return StatusSuccess
}

// CheckLeashRange returns Success if the monster is within LeashRange of its spawn.
type CheckLeashRange struct{}

func (n *CheckLeashRange) Tick(ctx *AIContext) Status {
	if ctx.Config == nil || ctx.Config.LeashRange <= 0 {
		return StatusSuccess // no leash = always in range
	}
	x, y := ctx.Monster.Position()
	sx, sy := ctx.Monster.SpawnPosition()
	if manhattan(x, y, sx, sy) <= ctx.Config.LeashRange {
		return StatusSuccess
	}
	return StatusFailure
}

// CheckHPBelow returns Success if monster HP% is below FleeHPPercent.
type CheckHPBelow struct{}

func (n *CheckHPBelow) Tick(ctx *AIContext) Status {
	if ctx.Config == nil || ctx.Config.FleeHPPercent <= 0 {
		return StatusFailure
	}
	m := ctx.Monster
	maxHP := m.GetMaxHP()
	if maxHP <= 0 {
		return StatusFailure
	}
	hpPercent := m.GetHP() * 100 / maxHP
	if hpPercent < ctx.Config.FleeHPPercent {
		return StatusSuccess
	}
	return StatusFailure
}

// CheckAttackRange returns Success if the target is within AttackRange tiles.
type CheckAttackRange struct{}

func (n *CheckAttackRange) Tick(ctx *AIContext) Status {
	targetID := ctx.Monster.GetTarget()
	if targetID == 0 {
		return StatusFailure
	}
	p := ctx.Room.PlayerByID(targetID)
	if p == nil {
		return StatusFailure
	}
	attackRange := 1
	if ctx.Config != nil && ctx.Config.AttackRange > 0 {
		attackRange = ctx.Config.AttackRange
	}
	x, y := ctx.Monster.Position()
	if manhattan(x, y, p.X, p.Y) <= attackRange {
		return StatusSuccess
	}
	return StatusFailure
}

// ---- Action Nodes ----

// Wander moves the monster 1 tile in a random passable direction within WanderRadius.
type Wander struct{}

func (n *Wander) Tick(ctx *AIContext) Status {
	m := ctx.Monster
	if !m.CanMove() {
		return StatusRunning
	}
	x, y := m.Position()
	sx, sy := m.SpawnPosition()
	wanderRadius := 4
	if ctx.Config != nil && ctx.Config.WanderRadius > 0 {
		wanderRadius = ctx.Config.WanderRadius
	}
	moveInterval := 40
	if ctx.Config != nil && ctx.Config.MoveIntervalTicks > 0 {
		moveInterval = ctx.Config.MoveIntervalTicks
	}

	// Try a random direction that stays within wander radius.
	dirs := []int{2, 4, 6, 8} // down, left, right, up
	perm := rand.Perm(4)
	for _, i := range perm {
		dir := dirs[i]
		nx, ny := x+dirDX(dir), y+dirDY(dir)
		if manhattan(nx, ny, sx, sy) > wanderRadius {
			continue
		}
		if ctx.Room.TryMoveMonster(m, dir) {
			m.ResetMoveTimer(moveInterval)
			return StatusSuccess
		}
	}
	return StatusFailure
}

// ChaseTarget moves the monster 1 tile toward its current target using A* pathfinding.
type ChaseTarget struct{}

func (n *ChaseTarget) Tick(ctx *AIContext) Status {
	m := ctx.Monster
	if !m.CanMove() {
		return StatusRunning
	}
	targetID := m.GetTarget()
	if targetID == 0 {
		return StatusFailure
	}
	p := ctx.Room.PlayerByID(targetID)
	if p == nil {
		return StatusFailure
	}
	m.SetState(StateChase)

	x, y := m.Position()
	targetPt := Point{p.X, p.Y}
	cachedTarget := m.GetCachedTarget()
	path := m.GetCachedPath()

	// Recompute path if target moved significantly or path exhausted.
	if len(path) == 0 || manhattan(cachedTarget.X, cachedTarget.Y, targetPt.X, targetPt.Y) > 3 {
		pm := ctx.Room.GetPassMap()
		if pm != nil {
			path = AStar(pm, Point{x, y}, targetPt)
			m.SetCachedPath(path, targetPt)
		}
	}

	moveInterval := 10
	if ctx.Config != nil && ctx.Config.MoveIntervalTicks > 0 {
		moveInterval = ctx.Config.MoveIntervalTicks
	}

	if len(path) > 0 {
		next := path[0]
		dir := pointToDir(x, y, next.X, next.Y)
		if ctx.Room.TryMoveMonster(m, dir) {
			m.SetCachedPath(path[1:], targetPt)
			m.ResetMoveTimer(moveInterval)
			return StatusRunning
		}
		// Path blocked — clear and fallback to simple move.
		m.SetCachedPath(nil, targetPt)
	}

	// Simple fallback: move toward target without A*.
	dir := simpleDirectionToward(x, y, p.X, p.Y)
	if dir != 0 && ctx.Room.TryMoveMonster(m, dir) {
		m.ResetMoveTimer(moveInterval)
		return StatusRunning
	}
	return StatusFailure
}

// MoveToSpawn moves the monster 1 tile toward its spawn position.
type MoveToSpawn struct{}

func (n *MoveToSpawn) Tick(ctx *AIContext) Status {
	m := ctx.Monster
	x, y := m.Position()
	sx, sy := m.SpawnPosition()
	if x == sx && y == sy {
		m.SetTarget(0)
		m.SetState(StateIdle)
		if ctx.ThreatTable != nil {
			ctx.ThreatTable.Clear()
		}
		// Trigger linked group leash: all group members disengage.
		if ctx.OnLeash != nil {
			ctx.OnLeash()
		}
		return StatusSuccess
	}
	if !m.CanMove() {
		return StatusRunning
	}
	moveInterval := 10
	if ctx.Config != nil && ctx.Config.MoveIntervalTicks > 0 {
		moveInterval = ctx.Config.MoveIntervalTicks
	}
	dir := simpleDirectionToward(x, y, sx, sy)
	if dir != 0 && ctx.Room.TryMoveMonster(m, dir) {
		m.ResetMoveTimer(moveInterval)
		return StatusRunning
	}
	return StatusFailure
}

// AttackTarget deals damage to the current target (if within range and cooldown ready).
type AttackTarget struct{}

func (n *AttackTarget) Tick(ctx *AIContext) Status {
	m := ctx.Monster
	if !m.CanAttack() {
		return StatusRunning
	}
	targetID := m.GetTarget()
	if targetID == 0 {
		return StatusFailure
	}
	m.SetState(StateAttack)
	cooldown := 20
	if ctx.Config != nil && ctx.Config.AttackCooldownTicks > 0 {
		cooldown = ctx.Config.AttackCooldownTicks
	}
	m.ResetAttackTimer(cooldown)
	// Actual damage is applied by the room/handler via DamageCallback.
	if ctx.DamageCallback != nil {
		ctx.DamageCallback(m, targetID)
	}
	return StatusSuccess
}

// FollowLeaderTarget makes pack followers adopt the pack leader's target.
// Returns Success if a leader target was found and added to threat, Failure otherwise.
type FollowLeaderTarget struct{}

func (n *FollowLeaderTarget) Tick(ctx *AIContext) Status {
	if ctx.GroupInfo == nil || ctx.GroupInfo.GroupType != "pack" {
		return StatusFailure
	}
	target := ctx.GroupInfo.LeaderTarget
	if target == 0 {
		return StatusFailure
	}
	// Add minimal threat so the follower targets the leader's target.
	if ctx.ThreatTable != nil && ctx.ThreatTable.Len() == 0 {
		ctx.ThreatTable.AddThreat(target, 1)
	}
	return StatusSuccess
}

// SetStateNode sets the monster's AI state (for client visual feedback).
type SetStateNode struct {
	State MonsterState
}

func (n *SetStateNode) Tick(ctx *AIContext) Status {
	ctx.Monster.SetState(n.State)
	return StatusSuccess
}

// ---- Helpers ----

func manhattan(x1, y1, x2, y2 int) int {
	dx := x1 - x2
	if dx < 0 {
		dx = -dx
	}
	dy := y1 - y2
	if dy < 0 {
		dy = -dy
	}
	return dx + dy
}

// dirDX/dirDY return the delta for RMMV directions.
func dirDX(dir int) int {
	switch dir {
	case 4:
		return -1
	case 6:
		return 1
	}
	return 0
}

func dirDY(dir int) int {
	switch dir {
	case 2:
		return 1
	case 8:
		return -1
	}
	return 0
}

// pointToDir returns the RMMV direction from (x,y) to (nx,ny).
func pointToDir(x, y, nx, ny int) int {
	dx := nx - x
	dy := ny - y
	if dx > 0 {
		return 6 // right
	}
	if dx < 0 {
		return 4 // left
	}
	if dy > 0 {
		return 2 // down
	}
	if dy < 0 {
		return 8 // up
	}
	return 2 // default
}

// simpleDirectionToward picks the best RMMV direction to move from (x,y) toward (tx,ty).
func simpleDirectionToward(x, y, tx, ty int) int {
	dx := tx - x
	dy := ty - y
	if dx == 0 && dy == 0 {
		return 0
	}
	absDX := dx
	if absDX < 0 {
		absDX = -absDX
	}
	absDY := dy
	if absDY < 0 {
		absDY = -absDY
	}
	if absDX >= absDY {
		if dx > 0 {
			return 6
		}
		return 4
	}
	if dy > 0 {
		return 2
	}
	return 8
}
