package battle

import (
	"context"
	"fmt"
	"math/rand"
	"time"

	"github.com/kasuganosora/rpgmakermvmmo/server/resource"
	"go.uber.org/zap"
)

// BattleResult constants.
const (
	ResultWin    = 0
	ResultEscape = 1
	ResultLose   = 2
)

// ActionInput is sent by a player to choose their action.
type ActionInput struct {
	ActorIndex    int
	ActionType    int // ActionAttack, ActionSkill, etc.
	SkillID       int
	ItemID        int
	TargetIndices []int
	TargetIsActor bool
}

// BattleConfig configures a BattleInstance.
type BattleConfig struct {
	TroopID     int
	CanEscape   bool
	CanLose     bool
	Res         *resource.ResourceLoader
	Logger      *zap.Logger
	RNG         *rand.Rand          // injectable for testing
	TurnMgr     TurnManager         // nil = DefaultTurnManager
	InputTimeout time.Duration      // 0 = 2 minutes
}

// BattleInstance manages a complete battle lifecycle.
type BattleInstance struct {
	Actors  []Battler
	Enemies []Battler

	troopID     int
	canEscape   bool
	canLose     bool
	turnCount   int
	escapeRatio float64

	res    *resource.ResourceLoader
	logger *zap.Logger
	rng    *rand.Rand
	turnMgr TurnManager
	ap     *ActionProcessor

	events   chan BattleEvent
	inputCh  chan *ActionInput
	inputTimeout time.Duration
}

// NewBattleInstance creates a battle instance.
// Actors and enemies must be added before calling Run().
func NewBattleInstance(cfg BattleConfig) *BattleInstance {
	if cfg.RNG == nil {
		cfg.RNG = rand.New(rand.NewSource(time.Now().UnixNano()))
	}
	if cfg.TurnMgr == nil {
		cfg.TurnMgr = DefaultTurnManager{}
	}
	if cfg.Logger == nil {
		cfg.Logger = zap.NewNop()
	}
	timeout := cfg.InputTimeout
	if timeout == 0 {
		timeout = 2 * time.Minute
	}

	return &BattleInstance{
		troopID:     cfg.TroopID,
		canEscape:   cfg.CanEscape,
		canLose:     cfg.CanLose,
		escapeRatio: 0.5,
		res:         cfg.Res,
		logger:      cfg.Logger,
		rng:         cfg.RNG,
		turnMgr:     cfg.TurnMgr,
		ap:          &ActionProcessor{Res: cfg.Res, RNG: cfg.RNG},
		events:      make(chan BattleEvent, 64),
		inputCh:     make(chan *ActionInput, 8),
		inputTimeout: timeout,
	}
}

// Events returns the event channel. Consumers should read from this
// channel to receive battle events for broadcasting to clients.
func (b *BattleInstance) Events() <-chan BattleEvent {
	return b.events
}

// InputCh returns the input channel. External code pushes player actions here.
func (b *BattleInstance) InputCh() chan<- *ActionInput {
	return b.inputCh
}

// SubmitInput is a convenience method for submitting player input.
func (b *BattleInstance) SubmitInput(input *ActionInput) {
	select {
	case b.inputCh <- input:
	default:
	}
}

// Run executes the battle main loop. Blocks until battle ends.
// Returns ResultWin, ResultEscape, or ResultLose.
func (b *BattleInstance) Run(ctx context.Context) int {
	defer close(b.events)

	// Emit start event.
	actorSnaps := make([]BattlerSnapshot, len(b.Actors))
	for i, a := range b.Actors {
		actorSnaps[i] = SnapshotBattler(a)
	}
	enemySnaps := make([]BattlerSnapshot, len(b.Enemies))
	for i, e := range b.Enemies {
		enemySnaps[i] = SnapshotBattler(e)
	}
	b.emitEvent(&EventBattleStart{Actors: actorSnaps, Enemies: enemySnaps})

	for {
		b.turnCount++
		b.logger.Debug("battle turn start", zap.Int("turn", b.turnCount))

		// Reset guard for all battlers.
		for _, a := range b.Actors {
			a.SetGuarding(false)
		}
		for _, e := range b.Enemies {
			e.SetGuarding(false)
		}

		// Collect inputs: player actions + enemy AI.
		if err := b.collectActions(ctx); err != nil {
			b.logger.Warn("action collection failed", zap.Error(err))
			return ResultLose
		}

		// Determine action order.
		order := b.turnMgr.MakeActionOrder(b.Actors, b.Enemies, b.rng)

		orderRefs := make([]BattlerRef, len(order))
		for i, bt := range order {
			orderRefs[i] = RefBattler(bt)
		}
		b.emitEvent(&EventTurnStart{TurnCount: b.turnCount, Order: orderRefs})

		// Execute actions in order.
		for _, bt := range order {
			if bt.IsDead() {
				continue
			}
			action := bt.CurrentAction()
			if action == nil {
				continue
			}

			// Handle escape attempt.
			if action.Type == ActionEscape {
				if b.canEscape && b.tryEscape() {
					b.emitEvent(&EventBattleEnd{Result: ResultEscape})
					return ResultEscape
				}
				continue
			}

			// Execute action.
			outcomes := b.ap.ProcessAction(bt, action, b.Actors, b.Enemies)

			// Build event.
			if len(outcomes) > 0 {
				targets := make([]ActionResultTarget, len(outcomes))
				for i, out := range outcomes {
					var tgt Battler
					if out.TargetIsActor && out.TargetIndex < len(b.Actors) {
						tgt = b.Actors[out.TargetIndex]
					} else if !out.TargetIsActor && out.TargetIndex < len(b.Enemies) {
						tgt = b.Enemies[out.TargetIndex]
					}
					ref := BattlerRef{}
					hpAfter, mpAfter := 0, 0
					if tgt != nil {
						ref = RefBattler(tgt)
						hpAfter = tgt.HP()
						mpAfter = tgt.MP()
					}
					targets[i] = ActionResultTarget{
						Target:         ref,
						Damage:         out.Damage,
						Critical:       out.Critical,
						Missed:         out.Missed,
						HPAfter:        hpAfter,
						MPAfter:        mpAfter,
						AddedStates:    out.AddedStates,
						RemovedStates:  out.RemovedStates,
						CommonEventIDs: out.CommonEventIDs,
					}
				}
				// For ActionAttack, the actual skill used is always 1 (normal attack),
				// but action.SkillID is 0 (from client input). Send the real skill ID.
				skillIDForEvent := action.SkillID
				if action.Type == ActionAttack {
					skillIDForEvent = 1
				}
				b.emitEvent(&EventActionResult{
					Subject: RefBattler(bt),
					SkillID: skillIDForEvent,
					ItemID:  action.ItemID,
					Targets: targets,
				})
			}

			// Check for death state (state 1 = death in RMMV).
			b.checkDeathState()

			if result := b.checkBattleEnd(); result >= 0 {
				b.emitBattleEnd(result)
				return result
			}
		}

		// Turn end: regen, tick states/buffs.
		b.processTurnEnd()

		if result := b.checkBattleEnd(); result >= 0 {
			b.emitBattleEnd(result)
			return result
		}

		// Clear actions.
		for _, a := range b.Actors {
			a.ClearAction()
		}
		for _, e := range b.Enemies {
			e.ClearAction()
		}
	}
}

// collectActions gathers actions from all alive battlers.
// For actors: waits for player input. For enemies: uses AI.
func (b *BattleInstance) collectActions(ctx context.Context) error {
	// Collect actor inputs.
	for _, actor := range b.Actors {
		if actor.IsDead() {
			continue
		}

		// Request input from player.
		b.emitEvent(&EventInputRequest{ActorIndex: actor.Index()})

		// Wait for input.
		input, err := b.waitForInput(ctx, actor.Index())
		if err != nil {
			return err
		}
		action := &Action{
			Type:          input.ActionType,
			SkillID:       input.SkillID,
			ItemID:        input.ItemID,
			TargetIndices: input.TargetIndices,
			TargetIsActor: input.TargetIsActor,
		}
		actor.SetAction(action)
	}

	// Generate enemy actions via AI.
	for _, enemy := range b.Enemies {
		if enemy.IsDead() {
			continue
		}
		eb, ok := enemy.(*EnemyBattler)
		if !ok {
			continue
		}
		action := MakeEnemyAction(eb, b.turnCount, b.Actors, b.Enemies, b.res, b.rng)
		enemy.SetAction(action)
	}

	return nil
}

func (b *BattleInstance) waitForInput(ctx context.Context, actorIndex int) (*ActionInput, error) {
	timer := time.NewTimer(b.inputTimeout)
	defer timer.Stop()

	for {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-timer.C:
			return nil, fmt.Errorf("input timeout for actor %d", actorIndex)
		case input := <-b.inputCh:
			if input.ActorIndex == actorIndex {
				return input, nil
			}
			// Wrong actor — put it back and keep waiting.
			select {
			case b.inputCh <- input:
			default:
			}
		}
	}
}

func (b *BattleInstance) tryEscape() bool {
	if b.rng.Float64() < b.escapeRatio {
		return true
	}
	b.escapeRatio += 0.1 // increase escape chance each failed attempt
	return false
}

// checkDeathState adds death state (1) to any battler with HP=0.
func (b *BattleInstance) checkDeathState() {
	for _, a := range b.Actors {
		if a.IsDead() && !a.HasState(1) {
			a.AddState(1, -1)
		}
	}
	for _, e := range b.Enemies {
		if e.IsDead() && !e.HasState(1) {
			e.AddState(1, -1)
		}
	}
}

// checkBattleEnd returns the battle result or -1 if battle continues.
func (b *BattleInstance) checkBattleEnd() int {
	allEnemiesDead := true
	for _, e := range b.Enemies {
		if e.IsAlive() {
			allEnemiesDead = false
			break
		}
	}
	if allEnemiesDead {
		return ResultWin
	}

	allActorsDead := true
	for _, a := range b.Actors {
		if a.IsAlive() {
			allActorsDead = false
			break
		}
	}
	if allActorsDead {
		return ResultLose
	}

	return -1
}

// processTurnEnd handles regen, state tick, and buff tick.
func (b *BattleInstance) processTurnEnd() {
	var regen []RegenEntry
	expiredStates := make(map[string][]int)

	allBattlers := append(append([]Battler{}, b.Actors...), b.Enemies...)
	for _, bt := range allBattlers {
		if bt.IsDead() {
			continue
		}

		// HP/MP/TP regeneration via XParams (hrg=7, mrg=8, trg=9).
		hrg := bt.XParam(7) // HP regen rate
		mrg := bt.XParam(8) // MP regen rate
		trg := bt.XParam(9) // TP regen rate

		hpChange, mpChange, tpChange := 0, 0, 0
		if hrg != 0 {
			hpChange = int(float64(bt.MaxHP()) * hrg)
			bt.SetHP(bt.HP() + hpChange)
		}
		if mrg != 0 {
			mpChange = int(float64(bt.MaxMP()) * mrg)
			bt.SetMP(bt.MP() + mpChange)
		}
		if trg != 0 {
			tpChange = int(100.0 * trg)
			bt.SetTP(bt.TP() + tpChange)
		}

		if hpChange != 0 || mpChange != 0 || tpChange != 0 {
			regen = append(regen, RegenEntry{
				Battler:  RefBattler(bt),
				HPChange: hpChange,
				MPChange: mpChange,
				TPChange: tpChange,
			})
		}

		// Tick state turns.
		expired := bt.TickStateTurns()
		if len(expired) > 0 {
			key := fmt.Sprintf("%s_%d", boolToStr(bt.IsActor()), bt.Index())
			expiredStates[key] = expired
		}

		// Tick buff turns.
		bt.TickBuffTurns()
	}

	b.emitEvent(&EventTurnEnd{Regen: regen, ExpiredStates: expiredStates})
}

func (b *BattleInstance) emitBattleEnd(result int) {
	evt := &EventBattleEnd{Result: result}

	if result == ResultWin {
		// Calculate rewards.
		totalExp := 0
		totalGold := 0
		var allDrops []DropResult
		for _, e := range b.Enemies {
			if eb, ok := e.(*EnemyBattler); ok && eb.enemy != nil {
				totalExp += eb.enemy.Exp
				totalGold += eb.enemy.Gold
				drops := CalculateDrops(eb.enemy)
				allDrops = append(allDrops, drops...)
			}
		}

		// Party exp split.
		aliveActors := 0
		for _, a := range b.Actors {
			if a.IsAlive() {
				aliveActors++
			}
		}
		expEach := CalculateExp(totalExp, aliveActors)

		evt.Exp = expEach
		evt.Gold = totalGold
		evt.Drops = allDrops
	}

	b.emitEvent(evt)
}

func (b *BattleInstance) emitEvent(evt BattleEvent) {
	select {
	case b.events <- evt:
	default:
		b.logger.Warn("battle event dropped (channel full)", zap.String("type", evt.EventType()))
	}
}

func boolToStr(v bool) string {
	if v {
		return "actor"
	}
	return "enemy"
}
