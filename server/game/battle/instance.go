package battle

import (
	"context"
	"fmt"
	"math/rand"
	"sync"
	"sync/atomic"
	"time"

	"github.com/kasuganosora/rpgmakermvmmo/server/resource"
	"go.uber.org/zap"
)

// BattleResult constants.
const (
	ResultWin    = 0
	ResultEscape = 1
	ResultLose   = 2
	ResultAbort  = 3 // battle aborted by script (troop event code 340)
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

// LevelCheckFn checks if an actor would level up from exp gain.
// Returns (newLevel, leveled). Called by emitBattleEnd to populate LevelUps.
type LevelCheckFn func(charID int64, expGain int) (newLevel int, leveled bool)

// ItemCheckFn returns true if the player has the item in their inventory.
type ItemCheckFn func(charID int64, itemID int) bool

// ItemConsumeFn is called when an actor uses a consumable item in battle.
// charID identifies who used it; itemID is the RMMV item database ID.
type ItemConsumeFn func(charID int64, itemID int)

// BattleConfig configures a BattleInstance.
type BattleConfig struct {
	TroopID     int
	CanEscape   bool
	CanLose     bool
	Battleback1 string              // battleback1 image name from map data
	Battleback2 string              // battleback2 image name from map data
	Res          *resource.ResourceLoader
	Logger       *zap.Logger
	RNG          *rand.Rand          // injectable for testing
	TurnMgr      TurnManager         // nil = DefaultTurnManager
	InputTimeout time.Duration       // 0 = 2 minutes
	GameVars     map[int]int         // player variable snapshot for client UI (custom gauges etc.)
	LevelCheckFn  LevelCheckFn        // nil = no level-up check
	ItemCheckFn   ItemCheckFn        // nil = skip inventory check
	ItemConsumeFn ItemConsumeFn      // nil = no item consumption
}

// BattleInstance manages a complete battle lifecycle.
type BattleInstance struct {
	Actors  []Battler
	Enemies []Battler

	troopID     int
	canEscape   bool
	canLose     bool
	battleback1 string
	battleback2 string
	turnCount   int
	escapeRatio    float64
	gameVars       map[int]int
	levelCheckFn   LevelCheckFn
	itemCheckFn    ItemCheckFn
	itemConsumeFn  ItemConsumeFn

	res    *resource.ResourceLoader
	logger *zap.Logger
	rng    *rand.Rand
	turnMgr TurnManager
	ap     *ActionProcessor

	troopEvents *TroopEventRunner

	events       chan BattleEvent
	inputCh      chan *ActionInput
	inputTimeout time.Duration

	disconnectedMu sync.Mutex
	disconnected   map[int]bool // actor index → disconnected
	escaped        map[int]bool // actor index → escaped from battle
	enemyEscaped   map[int]bool // enemy index → escaped from battle
	aborted        int32        // atomic: 1 = battle aborted by script

	troopAckCh chan struct{} // receives ack from client after troop event dialog
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
		battleback1: cfg.Battleback1,
		battleback2: cfg.Battleback2,
		escapeRatio:  0.0, // incremented by 0.1 per failed attempt; base from agility ratio
		gameVars:      cfg.GameVars,
		levelCheckFn:  cfg.LevelCheckFn,
		itemCheckFn:   cfg.ItemCheckFn,
		itemConsumeFn: cfg.ItemConsumeFn,
		res:         cfg.Res,
		logger:      cfg.Logger,
		rng:         cfg.RNG,
		turnMgr:     cfg.TurnMgr,
		ap:          &ActionProcessor{Res: cfg.Res, RNG: cfg.RNG},
		events:       make(chan BattleEvent, 64),
		inputCh:      make(chan *ActionInput, 8),
		troopAckCh:   make(chan struct{}, 1),
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

// MarkDisconnected marks an actor as disconnected. Their turns will be
// auto-guarded instead of waiting for input, allowing the battle to continue.
func (b *BattleInstance) MarkDisconnected(actorIndex int) {
	b.disconnectedMu.Lock()
	if b.disconnected == nil {
		b.disconnected = make(map[int]bool)
	}
	b.disconnected[actorIndex] = true
	b.disconnectedMu.Unlock()

	// Push a dummy input to unblock waitForInput if it's currently waiting for this actor.
	select {
	case b.inputCh <- &ActionInput{ActorIndex: actorIndex, ActionType: ActionGuard}:
	default:
	}
}

func (b *BattleInstance) isDisconnected(actorIndex int) bool {
	b.disconnectedMu.Lock()
	defer b.disconnectedMu.Unlock()
	return b.disconnected[actorIndex]
}

// MarkEscaped marks an actor as having escaped from battle.
// The actor is removed from further participation but the battle continues
// for remaining actors.
func (b *BattleInstance) MarkEscaped(actorIndex int) {
	b.disconnectedMu.Lock()
	if b.escaped == nil {
		b.escaped = make(map[int]bool)
	}
	b.escaped[actorIndex] = true
	b.disconnectedMu.Unlock()
}

// IsEscaped returns true if the actor has escaped from battle.
func (b *BattleInstance) IsEscaped(actorIndex int) bool {
	b.disconnectedMu.Lock()
	defer b.disconnectedMu.Unlock()
	return b.escaped[actorIndex]
}

// MarkEnemyEscaped marks an enemy as having escaped from battle.
func (b *BattleInstance) MarkEnemyEscaped(enemyIndex int) {
	b.disconnectedMu.Lock()
	if b.enemyEscaped == nil {
		b.enemyEscaped = make(map[int]bool)
	}
	b.enemyEscaped[enemyIndex] = true
	b.disconnectedMu.Unlock()
}

// IsEnemyEscaped returns true if the enemy has escaped from battle.
func (b *BattleInstance) IsEnemyEscaped(enemyIndex int) bool {
	b.disconnectedMu.Lock()
	defer b.disconnectedMu.Unlock()
	return b.enemyEscaped[enemyIndex]
}

// Abort signals the battle to end immediately (script abort, code 340).
func (b *BattleInstance) Abort() {
	atomic.StoreInt32(&b.aborted, 1)
}

// IsAborted returns true if the battle was aborted by script.
func (b *BattleInstance) IsAborted() bool {
	return atomic.LoadInt32(&b.aborted) == 1
}

// TroopAckCh returns the channel that external code should signal
// when the client acknowledges a troop event dialog.
func (b *BattleInstance) TroopAckCh() chan<- struct{} {
	return b.troopAckCh
}

// waitForTroopAck blocks until the client sends an acknowledgment
// or a timeout of 30 seconds elapses.
func (b *BattleInstance) waitForTroopAck() {
	timer := time.NewTimer(30 * time.Second)
	defer timer.Stop()
	select {
	case <-b.troopAckCh:
	case <-timer.C:
		b.logger.Warn("troop event ack timeout, continuing")
	}
}

// Run executes the battle main loop. Blocks until battle ends.
// Returns ResultWin, ResultEscape, or ResultLose.
func (b *BattleInstance) Run(ctx context.Context) int {
	defer close(b.events)

	// RMMV: initTp() — initialize TP to random 0-25 for all battlers.
	for _, a := range b.Actors {
		a.SetTP(b.rng.Intn(26))
	}
	for _, e := range b.Enemies {
		e.SetTP(b.rng.Intn(26))
	}

	// Initialize troop event runner if troop has pages.
	b.initTroopEvents()

	// Emit start event.
	actorSnaps := make([]BattlerSnapshot, len(b.Actors))
	for i, a := range b.Actors {
		actorSnaps[i] = SnapshotBattler(a)
	}
	enemySnaps := make([]BattlerSnapshot, len(b.Enemies))
	for i, e := range b.Enemies {
		enemySnaps[i] = SnapshotBattler(e)
	}
	b.emitEvent(&EventBattleStart{
		Actors:      actorSnaps,
		Enemies:     enemySnaps,
		Battleback1: b.battleback1,
		Battleback2: b.battleback2,
		GameVars:    b.gameVars,
	})

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

		// Evaluate troop battle events (turn start).
		if b.troopEvents != nil {
			b.troopEvents.RunTurnStart(b.turnCount)
		}

		// Check if troop event aborted the battle.
		if result := b.checkBattleEnd(); result >= 0 {
			b.emitBattleEnd(result)
			return result
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
			if bt.IsDead() || (bt.IsActor() && b.IsEscaped(bt.Index())) || (!bt.IsActor() && b.IsEnemyEscaped(bt.Index())) {
				continue
			}
			action := bt.CurrentAction()
			if action == nil {
				continue
			}

			// Handle escape attempt.
			if action.Type == ActionEscape {
				if b.canEscape && b.tryEscape() {
					if bt.IsActor() && len(b.Actors) > 1 {
						// Party battle: only this actor escapes.
						b.MarkEscaped(bt.Index())
						b.emitEvent(&EventActorEscape{ActorIndex: bt.Index()})
						// Check if all actors are now escaped/dead.
						if result := b.checkBattleEnd(); result >= 0 {
							b.emitBattleEnd(result)
							return result
						}
					} else {
						// Solo battle: end the whole battle.
						b.emitEvent(&EventBattleEnd{Result: ResultEscape})
						return ResultEscape
					}
				}
				continue
			}

			// Execute action.
			outcomes := b.ap.ProcessAction(bt, action, b.Actors, b.Enemies)

			// Consume item if applicable.
			if action.Type == ActionItem && action.ItemID > 0 && len(outcomes) > 0 && b.itemConsumeFn != nil {
				if item := b.res.Items[action.ItemID]; item != nil && item.Consumable {
					if ab, ok := bt.(*ActorBattler); ok {
						b.itemConsumeFn(ab.CharID(), action.ItemID)
					}
				}
			}

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
					hpAfter, mpAfter, tpAfter := 0, 0, 0
					if tgt != nil {
						ref = RefBattler(tgt)
						// Check removeByDamage on HP damage hits.
						if !out.Missed && out.Damage > 0 {
							removed := tgt.CheckRemoveByDamage(b.rng)
							out.RemovedStates = append(out.RemovedStates, removed...)
						}
						hpAfter = tgt.HP()
						mpAfter = tgt.MP()
						tpAfter = tgt.TP()
					}
					targets[i] = ActionResultTarget{
						Target:         ref,
						Damage:         out.Damage,
						Critical:       out.Critical,
						Missed:         out.Missed,
						HPAfter:        hpAfter,
						MPAfter:        mpAfter,
						TPAfter:        tpAfter,
						AddedStates:    out.AddedStates,
						RemovedStates:  out.RemovedStates,
						AddedBuffs:     out.AddedBuffs,
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
					Subject:        RefBattler(bt),
					SkillID:        skillIDForEvent,
					ItemID:         action.ItemID,
					Targets:        targets,
					SubjectHPAfter: bt.HP(),
					SubjectMPAfter: bt.MP(),
					SubjectTPAfter: bt.TP(),
				})
			}

			// Tick action-end states (autoRemovalTiming=1) on the subject.
			if expired := bt.TickActionEndStates(); len(expired) > 0 {
				// Include in turn_end-style event so client removes them.
				key := fmt.Sprintf("%s_%d", boolToStr(bt.IsActor()), bt.Index())
				b.emitEvent(&EventTurnEnd{ExpiredStates: map[string][]int{key: expired}})
			}

			// Handle escape effects (effect 41 dataId=0).
			for _, out := range outcomes {
				if out.Escaped && !out.Missed {
					if !out.TargetIsActor && out.TargetIndex < len(b.Enemies) {
						b.MarkEnemyEscaped(out.TargetIndex)
						b.emitEvent(&EventEnemyEscape{EnemyIndex: out.TargetIndex})
					}
				}
			}

			// Check for death state (state 1 = death in RMMV).
			b.checkDeathState()

			// Evaluate span=2 (moment) troop events after each action.
			// This catches mid-turn conditions like "enemy HP <= 20% → transform".
			if b.troopEvents != nil {
				b.troopEvents.RunMoment(b.turnCount)
			}

			if result := b.checkBattleEnd(); result >= 0 {
				b.emitBattleEnd(result)
				return result
			}
		}

		// Evaluate troop battle events (turn end).
		if b.troopEvents != nil {
			b.troopEvents.RunTurnEnd(b.turnCount)
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
		if actor.IsDead() || b.IsEscaped(actor.Index()) {
			continue
		}

		restriction := actor.Restriction()

		// restriction=4: cannot move — skip turn entirely.
		if restriction == 4 {
			actor.ClearAction()
			continue
		}

		// restriction=1/2/3: forced attack with auto-selected target.
		if restriction >= 1 && restriction <= 3 {
			action := b.makeRestrictedAction(actor, restriction)
			actor.SetAction(action)
			continue
		}

		// Disconnected actors auto-guard.
		if b.isDisconnected(actor.Index()) {
			actor.SetAction(&Action{Type: ActionGuard})
			continue
		}

		// Normal: request input from player.
		b.emitEvent(&EventInputRequest{ActorIndex: actor.Index()})

		// Wait for input.
		input, err := b.waitForInput(ctx, actor.Index())
		if err != nil {
			return err
		}
		// Validate and sanitize input before creating the action.
		validatedInput := b.validateInput(actor, input)
		action := &Action{
			Type:          validatedInput.ActionType,
			SkillID:       validatedInput.SkillID,
			ItemID:        validatedInput.ItemID,
			TargetIndices: validatedInput.TargetIndices,
			TargetIsActor: validatedInput.TargetIsActor,
			SpeedMod:      b.lookupActionSpeed(validatedInput),
		}
		actor.SetAction(action)
	}

	// Generate enemy actions via AI.
	for _, enemy := range b.Enemies {
		if enemy.IsDead() || b.IsEnemyEscaped(enemy.Index()) {
			continue
		}

		// restriction=4: cannot move.
		if enemy.Restriction() == 4 {
			enemy.ClearAction()
			continue
		}

		eb, ok := enemy.(*EnemyBattler)
		if !ok {
			continue
		}

		// restriction=1/2/3: forced attack.
		if r := enemy.Restriction(); r >= 1 && r <= 3 {
			action := b.makeRestrictedAction(enemy, r)
			enemy.SetAction(action)
			continue
		}

		action := MakeEnemyAction(eb, b.turnCount, b.Actors, b.Enemies, b.res, b.rng)
		enemy.SetAction(action)
	}

	return nil
}

// makeRestrictedAction creates a forced attack action for restricted battlers.
// restriction=1: attack random enemy, restriction=2: attack random anyone, restriction=3: attack random ally.
func (b *BattleInstance) makeRestrictedAction(bt Battler, restriction int) *Action {
	var pool []Battler
	switch restriction {
	case 1: // attack enemy — normal behavior
		if bt.IsActor() {
			pool = b.aliveEnemies()
		} else {
			pool = b.aliveActors()
		}
	case 2: // attack anyone
		pool = append(b.aliveActors(), b.aliveEnemies()...)
	case 3: // attack ally
		if bt.IsActor() {
			pool = b.aliveActors()
		} else {
			pool = b.aliveEnemies()
		}
	}

	if len(pool) == 0 {
		return nil
	}

	target := pool[b.rng.Intn(len(pool))]
	return &Action{
		Type:          ActionAttack,
		TargetIndices: []int{target.Index()},
		TargetIsActor: target.IsActor(),
	}
}

func (b *BattleInstance) aliveActors() []Battler {
	var out []Battler
	for _, a := range b.Actors {
		if a.IsAlive() {
			out = append(out, a)
		}
	}
	return out
}

func (b *BattleInstance) aliveEnemies() []Battler {
	var out []Battler
	for _, e := range b.Enemies {
		if e.IsAlive() {
			out = append(out, e)
		}
	}
	return out
}

func (b *BattleInstance) waitForInput(ctx context.Context, actorIndex int) (*ActionInput, error) {
	timer := time.NewTimer(b.inputTimeout)
	defer timer.Stop()

	for {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-timer.C:
			// Timeout: auto-guard this actor instead of failing the entire battle.
			b.logger.Warn("input timeout, auto-guarding actor", zap.Int("actor_index", actorIndex))
			return &ActionInput{ActorIndex: actorIndex, ActionType: ActionGuard}, nil
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
	// RMMV formula: 0.5 * partyAgi / troopAgi + 0.1 * escapeCount
	partyAgi := b.averageAgi(b.Actors)
	troopAgi := b.averageAgi(b.Enemies)
	if troopAgi <= 0 {
		troopAgi = 1
	}
	ratio := 0.5*float64(partyAgi)/float64(troopAgi) + b.escapeRatio
	if b.rng.Float64() < ratio {
		return true
	}
	b.escapeRatio += 0.1 // increase escape chance each failed attempt
	return false
}

func (b *BattleInstance) averageAgi(battlers []Battler) int {
	total, count := 0, 0
	for _, bt := range battlers {
		if bt.IsAlive() {
			total += bt.Param(6) // agi
			count++
		}
	}
	if count == 0 {
		return 1
	}
	return total / count
}

// checkDeathState adds death state (1) to any battler with HP=0.
func (b *BattleInstance) checkDeathState() {
	for _, a := range b.Actors {
		if a.IsDead() && !a.HasState(1) {
			a.AddState(1, -1)
			a.SetTP(0) // RMMV: TP resets to 0 on death
		}
	}
	for _, e := range b.Enemies {
		if e.IsDead() && !e.HasState(1) {
			e.AddState(1, -1)
			e.SetTP(0)
		}
	}
}

// checkBattleEnd returns the battle result or -1 if battle continues.
func (b *BattleInstance) checkBattleEnd() int {
	// Script abort takes highest priority.
	if b.IsAborted() {
		return ResultAbort
	}

	// Check if all enemies are dead or escaped.
	allEnemiesOut := true
	for _, e := range b.Enemies {
		if b.IsEnemyEscaped(e.Index()) {
			continue
		}
		if e.IsAlive() {
			allEnemiesOut = false
			break
		}
	}
	if allEnemiesOut {
		return ResultWin
	}

	// Check if all actors are dead or escaped.
	allActorsOut := true
	anyEscaped := false
	for _, a := range b.Actors {
		if b.IsEscaped(a.Index()) {
			anyEscaped = true
			continue
		}
		if a.IsAlive() {
			allActorsOut = false
			break
		}
	}
	if allActorsOut {
		if anyEscaped {
			return ResultEscape
		}
		return ResultLose
	}

	return -1
}

// processTurnEnd handles regen, state tick, and buff tick.
func (b *BattleInstance) processTurnEnd() {
	var regen []RegenEntry
	expiredStates := make(map[string][]int)
	expiredBuffs := make(map[string][]int)

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
		expiredB := bt.TickBuffTurns()
		if len(expiredB) > 0 {
			key := fmt.Sprintf("%s_%d", boolToStr(bt.IsActor()), bt.Index())
			expiredBuffs[key] = expiredB
		}
	}

	b.emitEvent(&EventTurnEnd{Regen: regen, ExpiredStates: expiredStates, ExpiredBuffs: expiredBuffs})
}

// initTroopEvents sets up the TroopEventRunner from troop data.
func (b *BattleInstance) initTroopEvents() {
	if b.res == nil || b.troopID <= 0 || b.troopID >= len(b.res.Troops) {
		return
	}
	troop := b.res.Troops[b.troopID]
	if troop == nil || len(troop.Pages) == 0 {
		return
	}

	b.troopEvents = NewTroopEventRunner(TroopEventConfig{
		Pages:    troop.Pages,
		Res:      b.res,
		RNG:      b.rng,
		Logger:   b.logger,
		GameVars: b.gameVars,
		GetEnemyHP: func(index int) (int, bool) {
			if index < 0 || index >= len(b.Enemies) {
				return 0, false
			}
			e := b.Enemies[index]
			if e.IsDead() {
				return 0, false
			}
			if e.MaxHP() == 0 {
				return 100, true
			}
			return e.HP() * 100 / e.MaxHP(), true
		},
		GetActorHP: func(actorID int) (int, bool) {
			for _, a := range b.Actors {
				if ab, ok := a.(*ActorBattler); ok && int(ab.CharID()) == actorID {
					if a.IsDead() {
						return 0, false
					}
					if a.MaxHP() == 0 {
						return 100, true
					}
					return a.HP() * 100 / a.MaxHP(), true
				}
			}
			return 0, false
		},
		Emit: func(evt BattleEvent) {
			b.emitEvent(evt)
		},
		AbortFn: func() {
			b.Abort()
		},
		AddState: func(isActor bool, index int, stateID int) {
			var target Battler
			if isActor {
				if index >= 0 && index < len(b.Actors) {
					target = b.Actors[index]
				}
			} else {
				if index >= 0 && index < len(b.Enemies) {
					target = b.Enemies[index]
				}
			}
			if target != nil {
				target.AddState(stateID, -1) // default: no auto-removal for battle events
			}
		},
		RemoveState: func(isActor bool, index int, stateID int) {
			var target Battler
			if isActor {
				if index >= 0 && index < len(b.Actors) {
					target = b.Actors[index]
				}
			} else {
				if index >= 0 && index < len(b.Enemies) {
					target = b.Enemies[index]
				}
			}
			if target != nil {
				target.RemoveState(stateID)
			}
		},
		ChangeEnemyHP: func(enemyIndex int, value int) {
			if enemyIndex < 0 || enemyIndex >= len(b.Enemies) {
				return
			}
			e := b.Enemies[enemyIndex]
			newHP := e.HP() + value
			if newHP < 0 {
				newHP = 0
			}
			if newHP > e.MaxHP() {
				newHP = e.MaxHP()
			}
			e.SetHP(newHP)
		},
		ChangeEnemyMP: func(enemyIndex int, value int) {
			if enemyIndex < 0 || enemyIndex >= len(b.Enemies) {
				return
			}
			e := b.Enemies[enemyIndex]
			newMP := e.MP() + value
			if newMP < 0 {
				newMP = 0
			}
			if newMP > e.MaxMP() {
				newMP = e.MaxMP()
			}
			e.SetMP(newMP)
		},
		ChangeEnemyTP: func(enemyIndex int, value int) {
			if enemyIndex < 0 || enemyIndex >= len(b.Enemies) {
				return
			}
			e := b.Enemies[enemyIndex]
			newTP := e.TP() + value
			if newTP < 0 {
				newTP = 0
			}
			if newTP > 100 {
				newTP = 100
			}
			e.SetTP(newTP)
		},
		TransformEnemy: func(enemyIndex int, newEnemyID int) {
			if enemyIndex < 0 || enemyIndex >= len(b.Enemies) {
				return
			}
			eb, ok := b.Enemies[enemyIndex].(*EnemyBattler)
			if !ok {
				return
			}
			if b.res == nil || newEnemyID <= 0 || newEnemyID >= len(b.res.Enemies) {
				return
			}
			newEnemy := b.res.Enemies[newEnemyID]
			if newEnemy == nil {
				return
			}
			b.logger.Info("enemy transform",
				zap.Int("enemy_index", enemyIndex),
				zap.String("old_name", eb.Name()),
				zap.String("new_name", newEnemy.Name))
			eb.Transform(newEnemy)
		},
		RecoverEnemy: func(enemyIndex int) {
			if enemyIndex < 0 || enemyIndex >= len(b.Enemies) {
				return
			}
			e := b.Enemies[enemyIndex]
			e.SetHP(e.MaxHP())
			e.SetMP(e.MaxMP())
		},
		AddEnemy: func(enemyID int) {
			if b.res == nil || enemyID <= 0 || enemyID >= len(b.res.Enemies) {
				return
			}
			enemyData := b.res.Enemies[enemyID]
			if enemyData == nil {
				return
			}
			newIndex := len(b.Enemies)
			eb := NewEnemyBattler(enemyData, newIndex, b.res)
			b.Enemies = append(b.Enemies, eb)
			b.logger.Info("enemy added mid-battle",
				zap.Int("enemy_id", enemyID),
				zap.String("name", enemyData.Name),
				zap.Int("index", newIndex))
		},
		WaitForAck: func() {
			b.waitForTroopAck()
		},
		ActorCount: func() int {
			return len(b.Actors)
		},
		ActorDBID: func(index int) int {
			if index < 0 || index >= len(b.Actors) {
				return -1
			}
			if ab, ok := b.Actors[index].(*ActorBattler); ok {
				return int(ab.CharID())
			}
			return -1
		},
	})
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
				drops := CalculateDropsRNG(eb.enemy, b.rng)
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

		// Check level-ups if callback is provided.
		if b.levelCheckFn != nil {
			for _, a := range b.Actors {
				if !a.IsAlive() {
					continue
				}
				ab, ok := a.(*ActorBattler)
				if !ok {
					continue
				}
				newLevel, leveled := b.levelCheckFn(ab.CharID(), expEach)
				if leveled {
					evt.LevelUps = append(evt.LevelUps, LevelUpEntry{
						ActorIndex: ab.Index(),
						NewLevel:   newLevel,
					})
				}
			}
		}
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

// validateInput checks that a player's action input is legal and falls back
// to normal attack if the input is invalid (wrong skill, insufficient MP/TP, etc.).
func (b *BattleInstance) validateInput(actor Battler, input *ActionInput) *ActionInput {
	// Validate action type range.
	if input.ActionType < ActionAttack || input.ActionType > ActionEscape {
		input.ActionType = ActionAttack
		return input
	}

	if input.ActionType == ActionSkill {
		skill := b.res.SkillByID(input.SkillID)
		if skill == nil {
			b.logger.Warn("invalid skill ID, falling back to attack",
				zap.Int("skill_id", input.SkillID))
			input.ActionType = ActionAttack
			input.SkillID = 0
			return input
		}
		// Check actor knows this skill (skill 1 = normal attack, always allowed).
		if input.SkillID != 1 {
			known := false
			for _, sid := range actor.SkillIDs() {
				if sid == input.SkillID {
					known = true
					break
				}
			}
			if !known {
				b.logger.Warn("actor does not know skill, falling back to attack",
					zap.Int("skill_id", input.SkillID))
				input.ActionType = ActionAttack
				input.SkillID = 0
				return input
			}
		}
		// Check MP/TP cost.
		if actor.MP() < skill.MPCost || actor.TP() < skill.TPCost {
			b.logger.Warn("insufficient MP/TP for skill, falling back to attack",
				zap.Int("skill_id", input.SkillID),
				zap.Int("mp", actor.MP()), zap.Int("mp_cost", skill.MPCost),
				zap.Int("tp", actor.TP()), zap.Int("tp_cost", skill.TPCost))
			input.ActionType = ActionAttack
			input.SkillID = 0
			return input
		}
	}

	if input.ActionType == ActionItem {
		if input.ItemID <= 0 || b.res == nil || input.ItemID >= len(b.res.Items) || b.res.Items[input.ItemID] == nil {
			b.logger.Warn("invalid item ID, falling back to attack",
				zap.Int("item_id", input.ItemID))
			input.ActionType = ActionAttack
			input.ItemID = 0
			return input
		}
		// Check inventory via callback.
		if b.itemCheckFn != nil {
			if ab, ok := actor.(*ActorBattler); ok {
				if !b.itemCheckFn(ab.CharID(), input.ItemID) {
					b.logger.Warn("item not in inventory, falling back to attack",
						zap.Int("item_id", input.ItemID))
					input.ActionType = ActionAttack
					input.ItemID = 0
					return input
				}
			}
		}
	}

	return input
}

// lookupActionSpeed returns the speed modifier for a player action input.
// RMMV: skill.speed or item.speed determines turn order priority.
func (b *BattleInstance) lookupActionSpeed(input *ActionInput) int {
	if b.res == nil {
		return 0
	}
	switch input.ActionType {
	case ActionAttack:
		if s := b.res.SkillByID(1); s != nil {
			return s.Speed
		}
	case ActionSkill:
		if s := b.res.SkillByID(input.SkillID); s != nil {
			return s.Speed
		}
	case ActionItem:
		if input.ItemID > 0 && input.ItemID < len(b.res.Items) && b.res.Items[input.ItemID] != nil {
			return b.res.Items[input.ItemID].Speed
		}
	}
	return 0
}

// calcStatsFromClass extracts base params from a class at a given level.
// RMMV: params[paramID][level] where index 0 = level 0, index 1 = level 1, etc.
// Returns [8]int for mhp, mmp, atk, def, mat, mdf, agi, luk; nil if class not found.
func calcStatsFromClass(res *resource.ResourceLoader, classID, level int) []int {
	cls := res.ClassByID(classID)
	if cls == nil {
		return nil
	}
	if level < 0 {
		level = 0
	}
	result := make([]int, 8)
	for p := 0; p < 8; p++ {
		if p >= len(cls.Params) {
			continue
		}
		row := cls.Params[p]
		if level < len(row) {
			result[p] = row[level]
		}
	}
	return result
}

func boolToStr(v bool) string {
	if v {
		return "actor"
	}
	return "enemy"
}
