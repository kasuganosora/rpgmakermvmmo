package ws

import (
	"context"
	"encoding/json"
	"sync"
	"time"

	"github.com/kasuganosora/rpgmakermvmmo/server/game/battle"
	"github.com/kasuganosora/rpgmakermvmmo/server/game/party"
	"github.com/kasuganosora/rpgmakermvmmo/server/game/player"
	"github.com/kasuganosora/rpgmakermvmmo/server/model"
	"github.com/kasuganosora/rpgmakermvmmo/server/resource"
	"go.uber.org/zap"
	"gorm.io/gorm"
)

// VarSnapshotFn returns a snapshot of game variables relevant to battle UI for a player.
type VarSnapshotFn func(charID int64) map[int]int

// BattleSessionManager manages server-authoritative battle sessions.
type BattleSessionManager struct {
	mu        sync.RWMutex
	sessions  map[int64]*activeBattle // charID → active battle
	db        *gorm.DB
	res       *resource.ResourceLoader
	partyMgr  *party.Manager
	varSnapFn VarSnapshotFn
	logger    *zap.Logger
}

type activeBattle struct {
	instance *battle.BattleInstance
	charIDs  []int64 // all participants' charIDs
}

// NewBattleSessionManager creates a new manager.
func NewBattleSessionManager(db *gorm.DB, res *resource.ResourceLoader, partyMgr *party.Manager, logger *zap.Logger) *BattleSessionManager {
	return &BattleSessionManager{
		sessions: make(map[int64]*activeBattle),
		db:       db,
		res:      res,
		partyMgr: partyMgr,
		logger:   logger,
	}
}

// SetVarSnapshotFn sets the function for retrieving player variable snapshots for battle UI.
func (bm *BattleSessionManager) SetVarSnapshotFn(fn VarSnapshotFn) {
	bm.varSnapFn = fn
}

// RegisterHandlers registers battle WS handlers.
func (bm *BattleSessionManager) RegisterHandlers(r *Router) {
	r.On("battle_input", bm.HandleBattleInput)
	r.On("battle_troop_ack", bm.HandleTroopAck)
}

// HandleBattleInput receives player action choices during a server-authoritative battle.
func (bm *BattleSessionManager) HandleBattleInput(_ context.Context, s *player.PlayerSession, raw json.RawMessage) error {
	var req struct {
		ActorIndex    int   `json:"actor_index"`
		ActionType    int   `json:"action_type"` // 0=attack,1=skill,2=item,3=guard,4=escape
		SkillID       int   `json:"skill_id"`
		ItemID        int   `json:"item_id"`
		TargetIndices []int `json:"target_indices"`
		TargetIsActor bool  `json:"target_is_actor"`
	}
	if err := json.Unmarshal(raw, &req); err != nil {
		return nil
	}

	bm.mu.RLock()
	ab := bm.sessions[s.CharID]
	bm.mu.RUnlock()

	if ab == nil {
		bm.logger.Debug("battle_input from player not in battle", zap.Int64("char_id", s.CharID))
		return nil
	}

	// Validate actor ownership: the player can only control their own actor.
	expectedIndex := -1
	for i, cid := range ab.charIDs {
		if cid == s.CharID {
			expectedIndex = i
			break
		}
	}
	if expectedIndex < 0 || req.ActorIndex != expectedIndex {
		bm.logger.Warn("battle_input actor_index mismatch",
			zap.Int64("char_id", s.CharID),
			zap.Int("submitted", req.ActorIndex),
			zap.Int("expected", expectedIndex))
		return nil
	}

	ab.instance.SubmitInput(&battle.ActionInput{
		ActorIndex:    req.ActorIndex,
		ActionType:    req.ActionType,
		SkillID:       req.SkillID,
		ItemID:        req.ItemID,
		TargetIndices: req.TargetIndices,
		TargetIsActor: req.TargetIsActor,
	})

	return nil
}

// HandleTroopAck receives acknowledgment from client after a troop event dialog.
func (bm *BattleSessionManager) HandleTroopAck(_ context.Context, s *player.PlayerSession, _ json.RawMessage) error {
	bm.mu.RLock()
	ab := bm.sessions[s.CharID]
	bm.mu.RUnlock()

	if ab == nil {
		return nil
	}

	select {
	case ab.instance.TroopAckCh() <- struct{}{}:
	default:
	}
	return nil
}

// RunBattle creates and runs a server-authoritative battle. Blocks until the battle ends.
// This is the BattleFunc callback used by the NPC executor.
func (bm *BattleSessionManager) RunBattle(
	ctx context.Context,
	s *player.PlayerSession,
	troopID int,
	canEscape, canLose bool,
) int {
	// Look up the troop.
	if troopID <= 0 || troopID >= len(bm.res.Troops) || bm.res.Troops[troopID] == nil {
		bm.logger.Warn("troop not found, auto-win", zap.Int("troop_id", troopID))
		return battle.ResultWin
	}
	troop := bm.res.Troops[troopID]

	// Look up battleback from map data.
	var bb1, bb2 string
	if bm.res != nil {
		if mapData := bm.res.Maps[s.MapID]; mapData != nil {
			bb1 = mapData.Battleback1Name
			bb2 = mapData.Battleback2Name
		}
	}

	// Snapshot player variables for client UI (custom gauges, CallStand, etc.).
	var gameVars map[int]int
	if bm.varSnapFn != nil {
		gameVars = bm.varSnapFn(s.CharID)
	}

	// Build level-check callback using participant sessions.
	// This will be called from emitBattleEnd to populate LevelUps.
	participantExp := make(map[int64]struct{ Exp int64; Level int })
	participantExp[s.CharID] = struct{ Exp int64; Level int }{s.Exp, s.Level}

	levelCheckFn := func(charID int64, expGain int) (int, bool) {
		pe, ok := participantExp[charID]
		if !ok {
			return 0, false
		}
		newExp := pe.Exp + int64(expGain)
		newLevel := pe.Level
		for newLevel < 99 {
			needed := int64(battle.ExpNeeded(newLevel))
			if newExp < needed {
				break
			}
			newLevel++
		}
		return newLevel, newLevel > pe.Level
	}

	// Create battle instance.
	bi := battle.NewBattleInstance(battle.BattleConfig{
		TroopID:      troopID,
		CanEscape:    canEscape,
		CanLose:      canLose,
		Battleback1:  bb1,
		Battleback2:  bb2,
		Res:          bm.res,
		Logger:       bm.logger,
		GameVars:      gameVars,
		LevelCheckFn:  levelCheckFn,
		ItemCheckFn: func(charID int64, itemID int) bool {
			if bm.db == nil {
				return true // no DB = skip check
			}
			var count int64
			bm.db.Model(&model.Inventory{}).Where("char_id = ? AND item_id = ? AND kind = 1 AND equipped = ? AND qty > 0",
				charID, itemID, false).Count(&count)
			return count > 0
		},
		ItemConsumeFn: func(charID int64, itemID int) {
			if bm.db == nil {
				return
			}
			// Decrement item qty; delete row if qty reaches 0.
			var inv model.Inventory
			if err := bm.db.Where("char_id = ? AND item_id = ? AND kind = 1 AND equipped = ?",
				charID, itemID, false).First(&inv).Error; err == nil {
				if inv.Qty <= 1 {
					bm.db.Delete(&inv)
				} else {
					bm.db.Model(&inv).Update("qty", inv.Qty-1)
				}
			}
		},
	})

	// Build actor battlers.
	participants := []*player.PlayerSession{s}

	// Party integration: add nearby party members.
	if bm.partyMgr != nil {
		p := bm.partyMgr.GetParty(s.CharID)
		if p != nil {
			mx, my, _ := s.Position()
			nearby := p.GetNearbyMembers(s.MapID, mx, my, 10)
			for _, m := range nearby {
				if m.CharID != s.CharID {
					// Skip party members currently in NPC events (EventMu held).
					if !m.EventMu.TryLock() {
						bm.logger.Debug("party member busy with event, skipping battle",
							zap.Int64("char_id", m.CharID))
						continue
					}
					m.EventMu.Unlock()
					participants = append(participants, m)
					participantExp[m.CharID] = struct{ Exp int64; Level int }{m.Exp, m.Level}
				}
			}
		}
	}

	// Build ActorBattlers for each participant.
	for idx, p := range participants {
		actor := bm.buildActorBattler(p, idx)
		bi.Actors = append(bi.Actors, actor)
	}

	// Build EnemyBattlers from troop.
	for idx, member := range troop.Members {
		if member.EnemyID <= 0 || member.EnemyID >= len(bm.res.Enemies) {
			continue
		}
		enemyData := bm.res.Enemies[member.EnemyID]
		if enemyData == nil {
			continue
		}
		eb := battle.NewEnemyBattler(enemyData, idx, bm.res)
		bi.Enemies = append(bi.Enemies, eb)
	}

	if len(bi.Enemies) == 0 {
		bm.logger.Warn("troop has no valid enemies, auto-win", zap.Int("troop_id", troopID))
		return battle.ResultWin
	}

	// Register active battle for all participants.
	charIDs := make([]int64, len(participants))
	for i, p := range participants {
		charIDs[i] = p.CharID
	}
	ab := &activeBattle{instance: bi, charIDs: charIDs}
	bm.mu.Lock()
	for _, cid := range charIDs {
		bm.sessions[cid] = ab
	}
	bm.mu.Unlock()

	// Build charID → actor index mapping for disconnect handling.
	charToActorIdx := make(map[int64]int, len(participants))
	for i, p := range participants {
		charToActorIdx[p.CharID] = i
	}

	// Monitor participant disconnects: mark them as disconnected (auto-guard)
	// instead of cancelling the entire battle. Only cancel if ALL actors disconnect.
	battleCtx, battleCancel := context.WithCancel(ctx)
	defer battleCancel()
	for _, p := range participants {
		p := p
		go func() {
			select {
			case <-p.Done:
				actorIdx := charToActorIdx[p.CharID]
				bm.logger.Info("participant disconnected during battle, auto-guarding",
					zap.Int64("char_id", p.CharID),
					zap.Int("actor_index", actorIdx))
				bi.MarkDisconnected(actorIdx)
				// If ALL participants disconnected, cancel the battle.
				allGone := true
				for _, pp := range participants {
					select {
					case <-pp.Done:
					default:
						allGone = false
					}
				}
				if allGone {
					bm.logger.Info("all participants disconnected, cancelling battle")
					battleCancel()
				}
			case <-battleCtx.Done():
			}
		}()
	}

	// Mark all participants as in-battle.
	for _, p := range participants {
		p.SetInBattle(true)
	}

	// Start event broadcaster in background.
	go bm.broadcastEvents(bi, participants)

	// Run battle (blocks).
	result := bi.Run(battleCtx)

	// Sync post-battle HP/MP/states back to sessions and DB.
	bm.syncPostBattleState(ctx, bi, participants)

	// Persist rewards on win — exclude escaped players.
	if result == battle.ResultWin {
		var rewardParticipants []*player.PlayerSession
		for i, p := range participants {
			if !bi.IsEscaped(i) {
				rewardParticipants = append(rewardParticipants, p)
			}
		}
		bm.persistBattleRewards(ctx, troop, rewardParticipants)
	}

	// Clear in-battle flag for all participants.
	for _, p := range participants {
		p.SetInBattle(false)
	}

	// Cleanup.
	bm.mu.Lock()
	for _, cid := range charIDs {
		delete(bm.sessions, cid)
	}
	bm.mu.Unlock()

	bm.logger.Info("battle ended",
		zap.Int("troop_id", troopID),
		zap.Int("result", result),
		zap.Int64("char_id", s.CharID))

	// Wait for client to return to Scene_Map before allowing NPC executor to continue.
	// The client sends scene_ready after Scene_Map is fully loaded.
	// Drain any stale signal first, then wait for the fresh one.
	// Use a 30-second timeout to prevent indefinite hang if client never sends scene_ready.
	select {
	case <-s.SceneReadyCh:
	default:
	}
	sceneTimer := time.NewTimer(30 * time.Second)
	defer sceneTimer.Stop()
	select {
	case <-s.SceneReadyCh:
	case <-s.Done:
	case <-ctx.Done():
	case <-sceneTimer.C:
		bm.logger.Warn("scene_ready timeout after battle, continuing",
			zap.Int64("char_id", s.CharID))
	}

	return result
}

// broadcastEvents reads BattleEvents from the instance and sends them to all participants.
func (bm *BattleSessionManager) broadcastEvents(bi *battle.BattleInstance, participants []*player.PlayerSession) {
	for evt := range bi.Events() {
		pktType := "battle_" + evt.EventType()
		payload, err := json.Marshal(evt)
		if err != nil {
			bm.logger.Error("marshal battle event", zap.Error(err))
			continue
		}
		pkt := &player.Packet{Type: pktType, Payload: payload}
		raw, _ := json.Marshal(pkt)

		// When a single actor escapes in a party battle, send them a
		// personal battle_end(escape) so their client exits the battle scene,
		// then skip sending the actor_escape event to the escaped player
		// (they don't need it — they already got battle_end).
		if esc, ok := evt.(*battle.EventActorEscape); ok {
			if esc.ActorIndex < len(participants) {
				escapedPlayer := participants[esc.ActorIndex]
				// Send personal battle_end with escape result to the escaped player.
				endPayload, _ := json.Marshal(&battle.EventBattleEnd{Result: battle.ResultEscape})
				endPkt := &player.Packet{Type: "battle_battle_end", Payload: endPayload}
				escapedPlayer.Send(endPkt)
				escapedPlayer.SetInBattle(false)

				// Remove escaped player from active battle sessions.
				bm.mu.Lock()
				delete(bm.sessions, escapedPlayer.CharID)
				bm.mu.Unlock()
			}
			// Send actor_escape to remaining (non-escaped) participants so they see the departure.
			for i, p := range participants {
				if i != esc.ActorIndex && !bi.IsEscaped(i) {
					p.SendRaw(raw)
				}
			}
			continue
		}

		// Normal broadcast: send to all non-escaped participants.
		for i, p := range participants {
			if bi.IsEscaped(i) {
				continue
			}
			p.SendRaw(raw)
		}
	}
}

// buildActorBattler creates an ActorBattler from a PlayerSession.
func (bm *BattleSessionManager) buildActorBattler(s *player.PlayerSession, index int) *battle.ActorBattler {
	hp, maxHP, mp, maxMP := s.Stats()

	// Get base params from class, fall back to session's maxHP/maxMP.
	var baseParams [8]int
	baseParams[0] = maxHP
	baseParams[1] = maxMP

	if bm.res != nil {
		cls := bm.res.ClassByID(s.ClassID)
		if cls != nil {
			level := s.Level
			if level < 1 {
				level = 1
			}
			idx := level // RMMV Params[paramID][level] — index 0 is unused placeholder
			safeGet := func(paramID int) int {
				if paramID >= len(cls.Params) {
					return 0
				}
				row := cls.Params[paramID]
				if idx >= len(row) {
					return 0
				}
				return row[idx]
			}
			for p := 0; p < 8; p++ {
				v := safeGet(p)
				if v > 0 {
					baseParams[p] = v
				}
				// If class param is 0 but session has maxHP/maxMP, keep session value (set above).
			}
		}
	}

	// Load equipped items from DB and compute stat bonuses + traits.
	var equipBonus [8]int
	var equipTraits []resource.Trait
	if bm.db != nil && bm.res != nil {
		var invItems []model.Inventory
		bm.db.Where("char_id = ? AND equipped = ?", s.CharID, true).Find(&invItems)
		for _, inv := range invItems {
			if inv.Kind == 2 { // weapon
				for _, w := range bm.res.Weapons {
					if w != nil && w.ID == inv.ItemID {
						es := resource.EquipStatsFromParams(w.Params)
						equipBonus[0] += es.MaxHP
						equipBonus[1] += es.MaxMP
						equipBonus[2] += es.Atk
						equipBonus[3] += es.Def
						equipBonus[4] += es.Mat
						equipBonus[5] += es.Mdf
						equipBonus[6] += es.Agi
						equipBonus[7] += es.Luk
						if w.Traits != nil {
							equipTraits = append(equipTraits, w.Traits...)
						}
						break
					}
				}
			} else if inv.Kind == 3 { // armor
				for _, a := range bm.res.Armors {
					if a != nil && a.ID == inv.ItemID {
						es := resource.EquipStatsFromParams(a.Params)
						equipBonus[0] += es.MaxHP
						equipBonus[1] += es.MaxMP
						equipBonus[2] += es.Atk
						equipBonus[3] += es.Def
						equipBonus[4] += es.Mat
						equipBonus[5] += es.Mdf
						equipBonus[6] += es.Agi
						equipBonus[7] += es.Luk
						if a.Traits != nil {
							equipTraits = append(equipTraits, a.Traits...)
						}
						break
					}
				}
			}
		}
	}

	// Collect class traits.
	var actorTraits []resource.Trait
	if bm.res != nil {
		cls := bm.res.ClassByID(s.ClassID)
		if cls != nil {
			actorTraits = append(actorTraits, cls.Traits...)
		}
	}

	// Get learned skills.
	var skills []int
	if bm.res != nil {
		skills = bm.res.SkillsForLevel(s.ClassID, s.Level)
	}

	bm.logger.Debug("buildActorBattler",
		zap.String("name", s.CharName),
		zap.Int("class_id", s.ClassID),
		zap.Int("level", s.Level),
		zap.Int("hp", hp), zap.Int("maxHP", maxHP),
		zap.Int("mp", mp), zap.Int("maxMP", maxMP),
		zap.Int("base_mhp", baseParams[0]), zap.Int("base_mmp", baseParams[1]),
		zap.Int("base_atk", baseParams[2]), zap.Int("base_def", baseParams[3]),
	)

	return battle.NewActorBattler(battle.ActorConfig{
		CharID:      s.CharID,
		Name:        s.CharName,
		Index:       index,
		ClassID:     s.ClassID,
		Level:       s.Level,
		HP:          hp,
		MP:          mp,
		BaseParams:  baseParams,
		EquipBonus:  equipBonus,
		Skills:      skills,
		ActorTraits: actorTraits,
		EquipTraits: equipTraits,
		Res:         bm.res,
	})
}

// syncPostBattleState syncs HP/MP and states from battle actors back to sessions and DB.
func (bm *BattleSessionManager) syncPostBattleState(ctx context.Context, bi *battle.BattleInstance, participants []*player.PlayerSession) {
	// Build charID → participant lookup.
	pMap := make(map[int64]*player.PlayerSession, len(participants))
	for _, p := range participants {
		pMap[p.CharID] = p
	}

	for _, a := range bi.Actors {
		ab, ok := a.(*battle.ActorBattler)
		if !ok {
			continue
		}
		p, ok := pMap[ab.CharID()]
		if !ok {
			continue
		}

		// Sync HP/MP back to session.
		finalHP := a.HP()
		finalMP := a.MP()
		p.HP = finalHP
		p.MP = finalMP

		// Sync states: keep states where removeAtBattleEnd == false.
		p.States = make(map[int]bool)
		if bm.res != nil {
			for _, se := range a.StateEntries() {
				if se.StateID > 0 && se.StateID < len(bm.res.States) {
					stData := bm.res.States[se.StateID]
					if stData != nil && !stData.RemoveAtBattleEnd {
						p.States[se.StateID] = true
					}
				}
			}
		}

		// Persist HP/MP/MaxHP/MaxMP/ClassID to DB.
		if bm.db != nil {
			if err := bm.db.WithContext(ctx).Model(&model.Character{}).Where("id = ?", p.CharID).
				Updates(map[string]interface{}{
					"hp":       finalHP,
					"mp":       finalMP,
					"max_hp":   p.MaxHP,
					"max_mp":   p.MaxMP,
					"class_id": p.ClassID,
				}).Error; err != nil {
				bm.logger.Error("syncPostBattleState", zap.Int64("char_id", p.CharID), zap.Error(err))
			}
		}
	}
}

// persistBattleRewards awards exp, gold, and drops to all alive participants after a win.
func (bm *BattleSessionManager) persistBattleRewards(ctx context.Context, troop *resource.Troop, participants []*player.PlayerSession) {
	if bm.db == nil {
		return
	}

	// Compute rewards from troop enemies.
	totalExp := 0
	totalGold := 0
	var allDrops []battle.DropResult
	for _, member := range troop.Members {
		if member.EnemyID <= 0 || member.EnemyID >= len(bm.res.Enemies) {
			continue
		}
		enemyData := bm.res.Enemies[member.EnemyID]
		if enemyData == nil {
			continue
		}
		totalExp += enemyData.Exp
		totalGold += enemyData.Gold
		allDrops = append(allDrops, battle.CalculateDrops(enemyData)...)
	}

	aliveCount := 0
	for _, p := range participants {
		hp, _, _, _ := p.Stats()
		if hp > 0 {
			aliveCount++
		}
	}
	expEach := battle.CalculateExp(totalExp, aliveCount)

	// Persist per participant.
	for _, p := range participants {
		hp, _, _, _ := p.Stats()
		if hp <= 0 {
			continue // dead actors don't get rewards
		}

		// Exp + level in a transaction to prevent races.
		var char model.Character
		err := bm.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
			if err := tx.First(&char, p.CharID).Error; err != nil {
				return err
			}
			char.Exp += int64(expEach)
			for char.Level < 99 && char.Exp >= int64(battle.ExpNeeded(char.Level)) {
				char.Level++
			}
			return tx.Model(&char).Updates(map[string]interface{}{
				"exp":   char.Exp,
				"level": char.Level,
			}).Error
		})
		if err != nil {
			bm.logger.Error("persistBattleRewards exp", zap.Int64("char_id", p.CharID), zap.Error(err))
		} else {
			p.Level = char.Level
			p.Exp = char.Exp
		}

		// Gold.
		if totalGold > 0 {
			if err := bm.db.WithContext(ctx).Model(&model.Character{}).Where("id = ?", p.CharID).
				Update("gold", gorm.Expr("gold + ?", totalGold)).Error; err != nil {
				bm.logger.Error("persistBattleRewards gold", zap.Int64("char_id", p.CharID), zap.Error(err))
			}
		}

		// Drops — add to inventory.
		for _, drop := range allDrops {
			inv := model.Inventory{
				CharID: p.CharID,
				ItemID: drop.ItemID,
				Kind:   drop.ItemType,
				Qty:    drop.Quantity,
			}
			// Upsert: if same char+item+kind exists and not equipped, increment qty.
			var existing model.Inventory
			if err := bm.db.WithContext(ctx).Where("char_id = ? AND item_id = ? AND kind = ? AND equipped = ?",
				p.CharID, drop.ItemID, drop.ItemType, false).First(&existing).Error; err == nil {
				bm.db.WithContext(ctx).Model(&existing).Update("qty", existing.Qty+drop.Quantity)
			} else {
				bm.db.WithContext(ctx).Create(&inv)
			}
		}
	}

	bm.logger.Info("battle rewards persisted",
		zap.Int("exp_each", expEach),
		zap.Int("gold", totalGold),
		zap.Int("drops", len(allDrops)),
		zap.Int("participants", len(participants)))
}
