package battle

import (
	"fmt"
	"math/rand"
	"strings"

	"github.com/kasuganosora/rpgmakermvmmo/server/resource"
	"go.uber.org/zap"
)

// TroopEventRunner evaluates and executes troop battle event pages.
type TroopEventRunner struct {
	pages     []resource.TroopPage
	executed  []bool // per-page: true if span=0 (battle) page already executed
	res       *resource.ResourceLoader
	rng       *rand.Rand
	logger    *zap.Logger
	switches  map[int]bool // battle-local switches
	variables map[int]int  // battle-local variables (overlay on player vars)

	// Callbacks into the battle instance.
	getEnemyHP    func(index int) (hpPercent int, alive bool)
	getActorHP    func(charID int) (hpPercent int, alive bool)
	emit          func(evt BattleEvent)
	addState      func(battlerIsActor bool, battlerIndex int, stateID int)
	removeState   func(battlerIsActor bool, battlerIndex int, stateID int)
	changeEnemyHP  func(enemyIndex int, value int) // positive=heal, negative=damage
	changeEnemyMP  func(enemyIndex int, value int)
	changeEnemyTP  func(enemyIndex int, value int)
	transformEnemy func(enemyIndex int, newEnemyID int)
	recoverEnemy   func(enemyIndex int) // full HP/MP recovery
	addEnemy       func(enemyID int)    // add a new enemy to the battle
	abortFn        func()               // called when code 340 (Abort Battle) is executed
	waitForAck     func()               // blocks until client acknowledges (for Show Text)
	actorCount     func() int           // returns number of actors in battle
	actorDBID      func(index int) int  // returns actor database ID at battle index
	enemyCount     func() int           // returns number of enemies in battle
}

// TroopEventConfig configures a TroopEventRunner.
type TroopEventConfig struct {
	Pages          []resource.TroopPage
	Res            *resource.ResourceLoader
	RNG            *rand.Rand
	Logger         *zap.Logger
	GameVars       map[int]int
	GetEnemyHP     func(index int) (hpPercent int, alive bool)
	GetActorHP     func(charID int) (hpPercent int, alive bool)
	Emit           func(evt BattleEvent)
	AddState       func(isActor bool, index int, stateID int)
	RemoveState    func(isActor bool, index int, stateID int)
	ChangeEnemyHP  func(enemyIndex int, value int)
	ChangeEnemyMP  func(enemyIndex int, value int)
	ChangeEnemyTP  func(enemyIndex int, value int)
	TransformEnemy func(enemyIndex int, newEnemyID int)
	RecoverEnemy   func(enemyIndex int)
	AddEnemy       func(enemyID int)
	AbortFn        func()
	WaitForAck     func() // blocks until client acknowledges (for Show Text)
	ActorCount     func() int          // returns number of actors in battle
	ActorDBID      func(index int) int // returns actor database ID at battle index
	EnemyCount     func() int          // returns number of enemies in battle
}

// NewTroopEventRunner creates a runner for troop battle events.
func NewTroopEventRunner(cfg TroopEventConfig) *TroopEventRunner {
	vars := make(map[int]int)
	for k, v := range cfg.GameVars {
		vars[k] = v
	}
	return &TroopEventRunner{
		pages:      cfg.Pages,
		executed:   make([]bool, len(cfg.Pages)),
		res:        cfg.Res,
		rng:        cfg.RNG,
		logger:     cfg.Logger,
		switches:   make(map[int]bool),
		variables:  vars,
		getEnemyHP: cfg.GetEnemyHP,
		getActorHP: cfg.GetActorHP,
		emit:          cfg.Emit,
		addState:      cfg.AddState,
		removeState:   cfg.RemoveState,
		changeEnemyHP: cfg.ChangeEnemyHP,
		changeEnemyMP: cfg.ChangeEnemyMP,
		changeEnemyTP: cfg.ChangeEnemyTP,
		transformEnemy: cfg.TransformEnemy,
		recoverEnemy:  cfg.RecoverEnemy,
		addEnemy:      cfg.AddEnemy,
		abortFn:       cfg.AbortFn,
		waitForAck:    cfg.WaitForAck,
		actorCount:    cfg.ActorCount,
		actorDBID:     cfg.ActorDBID,
		enemyCount:    cfg.EnemyCount,
	}
}

// getEnemyCount returns the number of enemies, falling back to 8 (RMMV default).
func (r *TroopEventRunner) getEnemyCount() int {
	if r.enemyCount != nil {
		return r.enemyCount()
	}
	return 8
}

// RunTurnStart evaluates span=0 (battle-once) and span=1 (turn) pages at turn start.
// Also resets span=1 executed flags so they can fire again this turn.
func (r *TroopEventRunner) RunTurnStart(turnCount int) {
	// Reset span=1 pages so they can trigger again this turn.
	for i, page := range r.pages {
		if page.Span == 1 {
			r.executed[i] = false
		}
	}
	r.evaluate(turnCount, false, 0, 1) // span 0 and 1
}

// RunTurnEnd evaluates span=0 and span=1 pages with turnEnding condition.
func (r *TroopEventRunner) RunTurnEnd(turnCount int) {
	r.evaluate(turnCount, true, 0, 1)
}

// RunMoment evaluates span=0 (battle-once) and span=2 (moment) pages.
// Called after each action to detect mid-turn condition changes (e.g. enemy HP thresholds).
func (r *TroopEventRunner) RunMoment(turnCount int) {
	r.evaluate(turnCount, false, 0, 2) // span 0 and 2
}

// evaluate runs troop event pages whose span matches one of the allowed values.
func (r *TroopEventRunner) evaluate(turnCount int, isTurnEnding bool, allowedSpans ...int) {
	spanOK := func(s int) bool {
		for _, a := range allowedSpans {
			if s == a {
				return true
			}
		}
		return false
	}

	for i, page := range r.pages {
		if !spanOK(page.Span) {
			continue
		}

		// Already executed: span=0 once per battle, span=1 once per turn.
		if (page.Span == 0 || page.Span == 1) && r.executed[i] {
			continue
		}

		if !r.checkConditions(&page, turnCount, isTurnEnding) {
			continue
		}

		// Skip pages with no meaningful commands.
		if len(page.List) <= 1 { // just the terminal code=0
			continue
		}

		r.logger.Debug("troop event page triggered",
			zap.Int("page", i),
			zap.Int("span", page.Span))

		r.executeCommands(page.List)

		if page.Span == 0 || page.Span == 1 {
			r.executed[i] = true
		}
	}
}

func (r *TroopEventRunner) checkConditions(page *resource.TroopPage, turnCount int, isTurnEnding bool) bool {
	cond := &page.Conditions

	// Turn ending check.
	if cond.TurnEnding {
		if !isTurnEnding {
			return false
		}
	}

	// Turn condition: turnA + turnB * X, where X is any non-negative integer.
	// If turnA=0, turnB=0 → always matches (on turn 0+).
	if cond.TurnValid {
		if !r.matchTurn(turnCount, cond.TurnA, cond.TurnB) {
			return false
		}
	}

	// Enemy HP condition.
	if cond.EnemyValid {
		if r.getEnemyHP == nil {
			return false
		}
		hpPct, alive := r.getEnemyHP(cond.EnemyIndex)
		if !alive {
			// Enemy dead — treat as HP=0.
			hpPct = 0
		}
		if hpPct > cond.EnemyHp {
			return false
		}
	}

	// Actor HP condition.
	if cond.ActorValid {
		if r.getActorHP == nil {
			return false
		}
		hpPct, alive := r.getActorHP(cond.ActorId)
		if !alive {
			hpPct = 0
		}
		if hpPct > cond.ActorHp {
			return false
		}
	}

	// Switch condition.
	if cond.SwitchValid {
		if !r.switches[cond.SwitchId] {
			return false
		}
	}

	return true
}

// matchTurn implements RMMV turn condition: matches when turnCount == turnA + turnB * n for some n >= 0.
func (r *TroopEventRunner) matchTurn(turnCount, turnA, turnB int) bool {
	if turnB == 0 {
		return turnCount == turnA
	}
	if turnCount < turnA {
		return false
	}
	return (turnCount-turnA)%turnB == 0
}

// executeCommands runs a list of event commands (battle event subset).
func (r *TroopEventRunner) executeCommands(list []resource.EventCommand) {
	idx := 0
	for idx < len(list) {
		cmd := list[idx]
		if cmd.Code == 0 {
			break
		}

		switch cmd.Code {
		case 108, 408:
			// Comment / comment continuation — skip.
			idx++
			continue

		case 111:
			// Conditional Branch — evaluate condition and skip to else/end if false.
			result := r.evalConditionalBranch(cmd.Parameters)
			if !result {
				idx = r.skipToElseOrEnd(list, idx, cmd.Indent)
			} else {
				idx++
			}
			continue

		case 411:
			// Else — if we reach this, we executed the "if" block, so skip to end.
			idx = r.skipToEnd(list, idx, cmd.Indent)
			continue

		case 412:
			// End Branch — nothing to do.
			idx++
			continue

		case 101:
			// Show Text — forward to client as battle dialogue, wait for ack.
			r.handleShowText(cmd, list, &idx)
			continue // handleShowText advances idx

		case 115:
			// Exit Event Processing — stop executing this page.
			return

		case 117:
			// Common Event — execute the common event's command list.
			r.handleCommonEvent(cmd)

		case 121:
			// Control Switches.
			r.handleControlSwitches(cmd)

		case 122:
			// Control Variables.
			r.handleControlVariables(cmd)

		case 128:
			// Change Armors — not applicable in battle, skip.

		case 250:
			// Play SE — forward to client.
			if r.emit != nil {
				r.emit(&EventTroopCommand{Code: 250, Params: cmd.Parameters})
			}

		case 241:
			// Play BGM — forward to client.
			if r.emit != nil {
				r.emit(&EventTroopCommand{Code: 241, Params: cmd.Parameters})
			}

		case 242:
			// Fadeout BGM — forward to client.
			if r.emit != nil {
				r.emit(&EventTroopCommand{Code: 242, Params: cmd.Parameters})
			}

		case 245:
			// Play BGS — forward to client.
			if r.emit != nil {
				r.emit(&EventTroopCommand{Code: 245, Params: cmd.Parameters})
			}

		case 246:
			// Fadeout BGS — forward to client.
			if r.emit != nil {
				r.emit(&EventTroopCommand{Code: 246, Params: cmd.Parameters})
			}

		case 249:
			// Play ME — forward to client.
			if r.emit != nil {
				r.emit(&EventTroopCommand{Code: 249, Params: cmd.Parameters})
			}

		case 313:
			// Change State (map-event style) — add/remove state.
			r.handleChangeState(cmd)

		case 331:
			// Change Enemy HP.
			r.handleChangeEnemyHP(cmd)

		case 332:
			// Change Enemy MP.
			r.handleChangeEnemyMP(cmd)

		case 333:
			// Change Enemy State — add/remove state on enemy.
			r.handleChangeEnemyState(cmd)

		case 334:
			// Enemy Recover All — fully restore HP/MP.
			r.handleEnemyRecoverAll(cmd)

		case 335:
			// Enemy Appear — forward to client.
			if r.emit != nil {
				r.emit(&EventTroopCommand{Code: 335, Params: cmd.Parameters})
			}

		case 336:
			// Enemy Transform — change enemy to a different enemy type mid-battle.
			r.handleEnemyTransform(cmd)

		case 337:
			// Show Battle Animation — forward to client.
			if r.emit != nil {
				r.emit(&EventTroopCommand{Code: 337, Params: cmd.Parameters})
			}

		case 339:
			// Force Action — forward to client.
			if r.emit != nil {
				r.emit(&EventTroopCommand{Code: 339, Params: cmd.Parameters})
			}

		case 340:
			// Abort Battle — end battle immediately via script.
			r.logger.Info("troop event: abort battle (code 340)")
			if r.abortFn != nil {
				r.abortFn()
			}
			return

		case 342:
			// Change Enemy TP.
			r.handleChangeEnemyTP(cmd)

		case 355:
			// Script — collect continuation lines and evaluate.
			script := r.paramString(cmd.Parameters, 0)
			for idx+1 < len(list) && list[idx+1].Code == 655 {
				idx++
				script += "\n" + r.paramString(list[idx].Parameters, 0)
			}
			r.handleScript(script)

		case 356:
			// Plugin Command — handle known commands.
			r.handlePluginCommand(cmd)

		case 401:
			// Show Text continuation — handled by 101.

		default:
			r.logger.Debug("troop event: unhandled command",
				zap.Int("code", cmd.Code))
		}

		idx++
	}
}

// --- Conditional branch evaluation ---

func (r *TroopEventRunner) evalConditionalBranch(params []interface{}) bool {
	if len(params) < 2 {
		return false
	}
	condType := r.paramInt(params, 0)
	switch condType {
	case 0: // Switch
		switchID := r.paramInt(params, 1)
		expected := r.paramInt(params, 2) // 0=ON, 1=OFF
		val := r.switches[switchID]
		if expected == 0 {
			return val
		}
		return !val
	case 1: // Variable
		varID := r.paramInt(params, 1)
		operandType := r.paramInt(params, 2) // 0=constant, 1=variable
		operand := r.paramInt(params, 3)
		if operandType == 1 {
			operand = r.variables[operand]
		}
		op := r.paramInt(params, 4) // 0=equal, 1=>=, 2=<=, 3=>, 4=<, 5=!=
		val := r.variables[varID]
		switch op {
		case 0:
			return val == operand
		case 1:
			return val >= operand
		case 2:
			return val <= operand
		case 3:
			return val > operand
		case 4:
			return val < operand
		case 5:
			return val != operand
		}
	case 2: // Self Switch — not applicable in battle.
		return false
	case 4: // Actor — check actor conditions.
		return r.evalActorCondition(params)
	case 6: // Enemy — check enemy conditions.
		return r.evalEnemyCondition(params)
	case 8: // Switch (same as 0 in some RMMV versions)
		switchID := r.paramInt(params, 1)
		return r.switches[switchID]
	case 11: // Script condition
		// Not implemented for battle events.
		return false
	case 12: // Script condition
		scriptStr := r.paramString(params, 1)
		return r.evalScriptCondition(scriptStr)
	}
	return false
}

func (r *TroopEventRunner) evalActorCondition(params []interface{}) bool {
	// params: [4, actorId, conditionType, ...]
	if r.getActorHP == nil {
		return false
	}
	conditionType := r.paramInt(params, 2)
	switch conditionType {
	case 1: // In party — always true in battle
		return true
	case 4: // State
		// Would need state check on actor — simplified
		return false
	}
	return false
}

func (r *TroopEventRunner) evalEnemyCondition(params []interface{}) bool {
	// params: [6, enemyIndex, conditionType, ...]
	if r.getEnemyHP == nil {
		return false
	}
	enemyIndex := r.paramInt(params, 1)
	conditionType := r.paramInt(params, 2)
	switch conditionType {
	case 0: // Appeared (alive)
		_, alive := r.getEnemyHP(enemyIndex)
		return alive
	case 1: // State
		// Would need state check — simplified
		return false
	}
	return false
}

func (r *TroopEventRunner) evalScriptCondition(script string) bool {
	// Handle simple variable comparisons.
	// e.g., "$gameSwitches.value(165)" or "$gameVariables.value(206) >= 3"
	script = strings.TrimSpace(script)

	// $gameSwitches.value(N)
	if strings.HasPrefix(script, "$gameSwitches.value(") {
		var switchID int
		fmt.Sscanf(script, "$gameSwitches.value(%d)", &switchID)
		return r.switches[switchID]
	}

	return false
}

// --- Command handlers ---

func (r *TroopEventRunner) handleControlSwitches(cmd resource.EventCommand) {
	// params: [startID, endID, value] — value: 0=ON, 1=OFF
	startID := r.paramInt(cmd.Parameters, 0)
	endID := r.paramInt(cmd.Parameters, 1)
	value := r.paramInt(cmd.Parameters, 2)
	for id := startID; id <= endID; id++ {
		r.switches[id] = (value == 0)
	}
}

func (r *TroopEventRunner) handleControlVariables(cmd resource.EventCommand) {
	// params: [startID, endID, operationType, operandType, operand, ...]
	startID := r.paramInt(cmd.Parameters, 0)
	endID := r.paramInt(cmd.Parameters, 1)
	opType := r.paramInt(cmd.Parameters, 2)    // 0=set, 1=add, 2=sub, 3=mul, 4=div, 5=mod
	operandType := r.paramInt(cmd.Parameters, 3) // 0=constant, 1=variable, 2=random, ...
	operand := r.paramInt(cmd.Parameters, 4)

	val := 0
	switch operandType {
	case 0: // Constant
		val = operand
	case 1: // Variable
		val = r.variables[operand]
	case 2: // Random
		min := operand
		max := r.paramInt(cmd.Parameters, 5)
		if max > min {
			val = min + r.rng.Intn(max-min+1)
		} else {
			val = min
		}
	}

	for id := startID; id <= endID; id++ {
		switch opType {
		case 0:
			r.variables[id] = val
		case 1:
			r.variables[id] += val
		case 2:
			r.variables[id] -= val
		case 3:
			r.variables[id] *= val
		case 4:
			if val != 0 {
				r.variables[id] /= val
			}
		case 5:
			if val != 0 {
				r.variables[id] %= val
			}
		}
	}
}

func (r *TroopEventRunner) handleChangeState(cmd resource.EventCommand) {
	// RMMV code 313: Change State (targets party actors, not enemies).
	// params: [scope, actorId, operation, stateId]
	// scope: 0=fixed actor ID, 1=variable lookup
	// actorId: 0=all party members, >0=specific actor database ID
	// operation: 0=add, 1=remove
	scope := r.paramInt(cmd.Parameters, 0)
	actorID := r.paramInt(cmd.Parameters, 1)
	operation := r.paramInt(cmd.Parameters, 2)
	stateID := r.paramInt(cmd.Parameters, 3)

	if scope == 1 {
		actorID = r.variables[actorID]
	}

	handler := r.addState
	if operation == 1 {
		handler = r.removeState
	}
	if handler == nil {
		return
	}

	if actorID == 0 {
		// All party actors.
		if r.actorCount != nil {
			for i := 0; i < r.actorCount(); i++ {
				handler(true, i, stateID)
			}
		}
	} else {
		// Find actor by database ID → battle index.
		if r.actorCount != nil && r.actorDBID != nil {
			for i := 0; i < r.actorCount(); i++ {
				if r.actorDBID(i) == actorID {
					handler(true, i, stateID)
					break
				}
			}
		}
	}
}

// handleChangeEnemyHP — code 331.
// RMMV params: [enemyIndex, operation, operandType, operand, allowKnockout]
// enemyIndex: -1=all enemies, 0+=specific index
// operation: 0=increase, 1=decrease
// operandType: 0=constant, 1=variable
func (r *TroopEventRunner) handleChangeEnemyHP(cmd resource.EventCommand) {
	if r.changeEnemyHP == nil {
		return
	}
	enemyIdx := r.paramInt(cmd.Parameters, 0)
	operation := r.paramInt(cmd.Parameters, 1) // 0=increase, 1=decrease
	operandType := r.paramInt(cmd.Parameters, 2)
	operand := r.paramInt(cmd.Parameters, 3)

	val := operand
	if operandType == 1 {
		val = r.variables[operand]
	}
	if operation == 1 {
		val = -val // decrease
	}

	if enemyIdx == -1 {
		// All enemies.
		for i := 0; i < r.getEnemyCount(); i++ {
			r.changeEnemyHP(i, val)
		}
	} else {
		r.changeEnemyHP(enemyIdx, val)
	}
}

// handleChangeEnemyMP — code 332.
// Same parameter layout as 331.
func (r *TroopEventRunner) handleChangeEnemyMP(cmd resource.EventCommand) {
	if r.changeEnemyMP == nil {
		return
	}
	enemyIdx := r.paramInt(cmd.Parameters, 0)
	operation := r.paramInt(cmd.Parameters, 1)
	operandType := r.paramInt(cmd.Parameters, 2)
	operand := r.paramInt(cmd.Parameters, 3)

	val := operand
	if operandType == 1 {
		val = r.variables[operand]
	}
	if operation == 1 {
		val = -val
	}

	if enemyIdx == -1 {
		for i := 0; i < r.getEnemyCount(); i++ {
			r.changeEnemyMP(i, val)
		}
	} else {
		r.changeEnemyMP(enemyIdx, val)
	}
}

// handleChangeEnemyState — code 333.
// RMMV params: [enemyIndex, operation, stateId]
// operation: 0=add, 1=remove
func (r *TroopEventRunner) handleChangeEnemyState(cmd resource.EventCommand) {
	enemyIdx := r.paramInt(cmd.Parameters, 0)
	operation := r.paramInt(cmd.Parameters, 1) // 0=add, 1=remove
	stateID := r.paramInt(cmd.Parameters, 2)

	handler := r.addState
	if operation == 1 {
		handler = r.removeState
	}
	if handler == nil {
		return
	}

	if enemyIdx == -1 {
		for i := 0; i < r.getEnemyCount(); i++ {
			handler(false, i, stateID)
		}
	} else {
		handler(false, enemyIdx, stateID)
	}
}

// handleChangeEnemyTP — code 342.
// Same parameter layout as 331.
func (r *TroopEventRunner) handleChangeEnemyTP(cmd resource.EventCommand) {
	if r.changeEnemyTP == nil {
		return
	}
	enemyIdx := r.paramInt(cmd.Parameters, 0)
	operation := r.paramInt(cmd.Parameters, 1)
	operandType := r.paramInt(cmd.Parameters, 2)
	operand := r.paramInt(cmd.Parameters, 3)

	val := operand
	if operandType == 1 {
		val = r.variables[operand]
	}
	if operation == 1 {
		val = -val
	}

	if enemyIdx == -1 {
		for i := 0; i < r.getEnemyCount(); i++ {
			r.changeEnemyTP(i, val)
		}
	} else {
		r.changeEnemyTP(enemyIdx, val)
	}
}

// handleEnemyTransform — code 336.
// RMMV params: [enemyIndex, newEnemyId]
func (r *TroopEventRunner) handleEnemyTransform(cmd resource.EventCommand) {
	if r.transformEnemy == nil {
		return
	}
	enemyIdx := r.paramInt(cmd.Parameters, 0)
	newEnemyID := r.paramInt(cmd.Parameters, 1)
	r.transformEnemy(enemyIdx, newEnemyID)

	// Also forward to client for visual update.
	if r.emit != nil {
		r.emit(&EventTroopCommand{Code: 336, Params: cmd.Parameters})
	}
}

// handleEnemyRecoverAll — code 334.
// RMMV params: [enemyIndex]
// enemyIndex: -1=all enemies, 0+=specific index
func (r *TroopEventRunner) handleEnemyRecoverAll(cmd resource.EventCommand) {
	if r.recoverEnemy == nil {
		return
	}
	enemyIdx := r.paramInt(cmd.Parameters, 0)
	if enemyIdx == -1 {
		for i := 0; i < r.getEnemyCount(); i++ {
			r.recoverEnemy(i)
		}
	} else {
		r.recoverEnemy(enemyIdx)
	}
}

// handleCommonEvent — code 117.
// RMMV params: [commonEventId]
// Executes the common event's command list inline.
func (r *TroopEventRunner) handleCommonEvent(cmd resource.EventCommand) {
	ceID := r.paramInt(cmd.Parameters, 0)
	if r.res == nil || ceID <= 0 || ceID >= len(r.res.CommonEvents) {
		return
	}
	ce := r.res.CommonEvents[ceID]
	if ce == nil || len(ce.List) == 0 {
		return
	}
	r.logger.Debug("troop event: executing common event",
		zap.Int("common_event_id", ceID),
		zap.String("name", ce.Name))

	// Convert []*EventCommand to []EventCommand for executeCommands.
	cmds := make([]resource.EventCommand, len(ce.List))
	for i, c := range ce.List {
		if c != nil {
			cmds[i] = *c
		}
	}
	r.executeCommands(cmds)
}

func (r *TroopEventRunner) handleScript(script string) {
	// Handle known script patterns used in projectb troop events.
	script = strings.TrimSpace(script)

	// $gameSwitches.setValue(N, true/false)
	if strings.HasPrefix(script, "$gameSwitches.setValue(") {
		var switchID int
		var valStr string
		fmt.Sscanf(script, "$gameSwitches.setValue(%d, %s", &switchID, &valStr)
		r.switches[switchID] = strings.HasPrefix(valStr, "true")
		return
	}

	// $gameVariables.setValue(N, value)
	if strings.HasPrefix(script, "$gameVariables.setValue(") {
		var varID, val int
		fmt.Sscanf(script, "$gameVariables.setValue(%d, %d", &varID, &val)
		r.variables[varID] = val
		return
	}

	// Forward unknown scripts as event for client-side execution.
	if r.emit != nil {
		r.emit(&EventTroopCommand{Code: 355, Params: []interface{}{script}})
	}
}

func (r *TroopEventRunner) handlePluginCommand(cmd resource.EventCommand) {
	// Forward plugin commands to client for execution.
	if r.emit != nil {
		r.emit(&EventTroopCommand{Code: 356, Params: cmd.Parameters})
	}
}

func (r *TroopEventRunner) handleShowText(cmd resource.EventCommand, list []resource.EventCommand, idx *int) {
	// Collect text lines (code 401 continuations).
	lines := []string{}
	*idx++
	for *idx < len(list) && list[*idx].Code == 401 {
		lines = append(lines, r.paramString(list[*idx].Parameters, 0))
		*idx++
	}
	// Forward as battle dialogue event (emit even with no continuation lines).
	if r.emit != nil {
		textPayload := strings.Join(lines, "\n")
		r.emit(&EventTroopCommand{
			Code:   101,
			Params: append(cmd.Parameters, textPayload),
		})
		// Wait for client to finish displaying the text before continuing.
		if r.waitForAck != nil {
			r.waitForAck()
		}
	}
}

// --- Flow control helpers ---

func (r *TroopEventRunner) skipToElseOrEnd(list []resource.EventCommand, startIdx, indent int) int {
	depth := 0
	for i := startIdx + 1; i < len(list); i++ {
		if list[i].Code == 111 && list[i].Indent == indent {
			depth++
		}
		if depth == 0 {
			if list[i].Code == 411 && list[i].Indent == indent {
				return i + 1 // skip to after else
			}
			if list[i].Code == 412 && list[i].Indent == indent {
				return i + 1 // skip to after end
			}
		}
		if list[i].Code == 412 && list[i].Indent == indent {
			if depth > 0 {
				depth--
			} else {
				return i + 1
			}
		}
	}
	return len(list) // shouldn't happen
}

func (r *TroopEventRunner) skipToEnd(list []resource.EventCommand, startIdx, indent int) int {
	for i := startIdx + 1; i < len(list); i++ {
		if list[i].Code == 412 && list[i].Indent == indent {
			return i + 1
		}
	}
	return len(list)
}

// --- Parameter helpers ---

func (r *TroopEventRunner) paramInt(params []interface{}, idx int) int {
	if idx >= len(params) {
		return 0
	}
	switch v := params[idx].(type) {
	case float64:
		return int(v)
	case int:
		return v
	}
	return 0
}

func (r *TroopEventRunner) paramString(params []interface{}, idx int) string {
	if idx >= len(params) {
		return ""
	}
	if s, ok := params[idx].(string); ok {
		return s
	}
	return fmt.Sprintf("%v", params[idx])
}
