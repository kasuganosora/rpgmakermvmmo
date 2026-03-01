package npc

import (
	"context"

	"github.com/kasuganosora/rpgmakermvmmo/server/game/player"
	"github.com/kasuganosora/rpgmakermvmmo/server/resource"
	"go.uber.org/zap"
)

// ---------- Change HP (311) ----------
// params: [0]=fixedActor(0=var), [1]=actorId/varId, [2]=operation(0=inc,1=dec),
//         [3]=operandType(0=constant,1=variable), [4]=operand, [5]=allowDeath
func (e *Executor) applyChangeHP(ctx context.Context, s *player.PlayerSession, params []interface{}, opts *ExecuteOpts) {
	amount := e.resolveOperand(params, 3, 4, opts)
	if paramInt(params, 2) == 1 { // decrease
		amount = -amount
	}
	allowDeath := paramInt(params, 5) != 0

	s.HP += amount
	if s.HP > s.MaxHP {
		s.HP = s.MaxHP
	}
	if s.HP < 0 {
		if allowDeath {
			s.HP = 0
		} else {
			s.HP = 1
		}
	}

	// Forward to client for visual update.
	e.sendEffect(s, &resource.EventCommand{Code: CmdChangeHP, Parameters: params})
}

// ---------- Change MP (312) ----------
// params: [0]=fixedActor, [1]=actorId/varId, [2]=operation(0=inc,1=dec),
//         [3]=operandType(0=constant,1=variable), [4]=operand
func (e *Executor) applyChangeMP(ctx context.Context, s *player.PlayerSession, params []interface{}, opts *ExecuteOpts) {
	amount := e.resolveOperand(params, 3, 4, opts)
	if paramInt(params, 2) == 1 { // decrease
		amount = -amount
	}

	s.MP += amount
	if s.MP > s.MaxMP {
		s.MP = s.MaxMP
	}
	if s.MP < 0 {
		s.MP = 0
	}

	e.sendEffect(s, &resource.EventCommand{Code: CmdChangeMP, Parameters: params})
}

// ---------- Change State (313) ----------
// params: [0]=fixedActor, [1]=actorId/varId, [2]=operation(0=add,1=remove), [3]=stateId
func (e *Executor) applyChangeState(ctx context.Context, s *player.PlayerSession, params []interface{}, opts *ExecuteOpts) {
	// State management is mostly client-side for now. Forward to client.
	e.sendEffect(s, &resource.EventCommand{Code: CmdChangeState, Parameters: params})
}

// ---------- Recover All (314) ----------
// params: [0]=fixedActor, [1]=actorId/varId
func (e *Executor) applyRecoverAll(ctx context.Context, s *player.PlayerSession, params []interface{}, opts *ExecuteOpts) {
	s.HP = s.MaxHP
	s.MP = s.MaxMP
	e.sendEffect(s, &resource.EventCommand{Code: CmdRecoverAll, Parameters: params})
}

// ---------- Change EXP (315) ----------
// params: [0]=fixedActor, [1]=actorId/varId, [2]=operation(0=inc,1=dec),
//         [3]=operandType(0=constant,1=variable), [4]=operand, [5]=showLevelUp
func (e *Executor) applyChangeEXP(ctx context.Context, s *player.PlayerSession, params []interface{}, opts *ExecuteOpts) {
	amount := e.resolveOperand(params, 3, 4, opts)
	if paramInt(params, 2) == 1 {
		amount = -amount
	}
	s.Exp += int64(amount)
	if s.Exp < 0 {
		s.Exp = 0
	}
	e.sendEffect(s, &resource.EventCommand{Code: CmdChangeEXP, Parameters: params})
}

// ---------- Change Level (316) ----------
// params: [0]=fixedActor, [1]=actorId/varId, [2]=operation(0=inc,1=dec),
//         [3]=operandType(0=constant,1=variable), [4]=operand, [5]=showLevelUp
func (e *Executor) applyChangeLevel(ctx context.Context, s *player.PlayerSession, params []interface{}, opts *ExecuteOpts) {
	amount := e.resolveOperand(params, 3, 4, opts)
	if paramInt(params, 2) == 1 {
		amount = -amount
	}
	s.Level += amount
	if s.Level < 1 {
		s.Level = 1
	}
	e.sendEffect(s, &resource.EventCommand{Code: CmdChangeLevel, Parameters: params})
}

// ---------- Change Class (321) ----------
// params: [0]=actorId, [1]=classId, [2]=keepExp
func (e *Executor) applyChangeClass(ctx context.Context, s *player.PlayerSession, params []interface{}, opts *ExecuteOpts) {
	classID := paramInt(params, 1)
	if classID <= 0 {
		return
	}

	oldClassID := s.ClassID
	s.ClassID = classID

	// Scale HP/MP proportionally based on new class base params.
	if e.res != nil {
		oldClass := e.res.ClassByID(oldClassID)
		newClass := e.res.ClassByID(classID)
		if oldClass != nil && newClass != nil && s.Level > 0 {
			level := s.Level
			if level > 99 {
				level = 99
			}
			// Params: [0]=MHP, [1]=MMP, index by level (1-based)
			oldMHP := classParam(oldClass, 0, level)
			newMHP := classParam(newClass, 0, level)
			oldMMP := classParam(oldClass, 1, level)
			newMMP := classParam(newClass, 1, level)

			if oldMHP > 0 && newMHP > 0 {
				ratio := float64(s.HP) / float64(oldMHP)
				s.MaxHP = newMHP
				s.HP = int(ratio * float64(newMHP))
				if s.HP < 1 {
					s.HP = 1
				}
			}
			if oldMMP > 0 && newMMP > 0 {
				ratio := float64(s.MP) / float64(oldMMP)
				s.MaxMP = newMMP
				s.MP = int(ratio * float64(newMMP))
				if s.MP < 0 {
					s.MP = 0
				}
			}
		}
	}

	e.logger.Info("change class",
		zap.Int64("char_id", s.CharID),
		zap.Int("old_class", oldClassID),
		zap.Int("new_class", classID))

	// Forward to client.
	e.sendEffect(s, &resource.EventCommand{Code: CmdChangeClass, Parameters: params})
}

// classParam reads a base parameter value from class data at the given level.
// Params is [paramIndex][level], 1-based level index.
func classParam(cls *resource.Class, paramIdx, level int) int {
	if cls == nil || paramIdx >= len(cls.Params) {
		return 0
	}
	row := cls.Params[paramIdx]
	if level < len(row) {
		return row[level]
	}
	if len(row) > 0 {
		return row[len(row)-1]
	}
	return 0
}

// ---------- Battle Processing (301) ----------
// params: [0]=troopIdType(0=direct,1=variable,2=random), [1]=troopId/varId,
//         [2]=canEscape, [3]=canLose
func (e *Executor) processBattle(ctx context.Context, s *player.PlayerSession, params []interface{}, opts *ExecuteOpts) {
	troopType := paramInt(params, 0)
	troopID := paramInt(params, 1)

	if troopType == 1 && opts != nil && opts.GameState != nil {
		// Variable reference.
		troopID = opts.GameState.GetVariable(troopID)
	}
	// Type 2 = random encounter (use troop 1 as fallback).
	if troopType == 2 || troopID <= 0 {
		troopID = 1
	}

	canEscape := paramInt(params, 2) != 0
	canLose := paramInt(params, 3) != 0

	if opts != nil && opts.BattleFn != nil {
		opts.BattleFn(ctx, s, troopID, canEscape, canLose)
	} else {
		// No battle function wired — send battle_start to client as fallback.
		e.logger.Warn("no BattleFn configured, sending battle to client",
			zap.Int("troop_id", troopID))
		e.sendEffect(s, &resource.EventCommand{Code: CmdBattleProcessing, Parameters: params})
	}
}

// resolveOperand reads a constant or variable-referenced integer from params.
func (e *Executor) resolveOperand(params []interface{}, typeIdx, valIdx int, opts *ExecuteOpts) int {
	opType := paramInt(params, typeIdx)
	val := paramInt(params, valIdx)
	if opType == 1 && opts != nil && opts.GameState != nil {
		return opts.GameState.GetVariable(val)
	}
	return val
}
