package ws

import (
	"context"
	"encoding/json"
	"sync"

	"github.com/kasuganosora/rpgmakermvmmo/server/game/battle"
	"github.com/kasuganosora/rpgmakermvmmo/server/game/party"
	"github.com/kasuganosora/rpgmakermvmmo/server/game/player"
	"github.com/kasuganosora/rpgmakermvmmo/server/resource"
	"go.uber.org/zap"
)

// BattleSessionManager manages server-authoritative battle sessions.
type BattleSessionManager struct {
	mu       sync.RWMutex
	sessions map[int64]*activeBattle // charID → active battle
	res      *resource.ResourceLoader
	partyMgr *party.Manager
	logger   *zap.Logger
}

type activeBattle struct {
	instance *battle.BattleInstance
	charIDs  []int64 // all participants' charIDs
}

// NewBattleSessionManager creates a new manager.
func NewBattleSessionManager(res *resource.ResourceLoader, partyMgr *party.Manager, logger *zap.Logger) *BattleSessionManager {
	return &BattleSessionManager{
		sessions: make(map[int64]*activeBattle),
		res:      res,
		partyMgr: partyMgr,
		logger:   logger,
	}
}

// RegisterHandlers registers battle_input WS handler.
func (bm *BattleSessionManager) RegisterHandlers(r *Router) {
	r.On("battle_input", bm.HandleBattleInput)
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

	// Create battle instance.
	bi := battle.NewBattleInstance(battle.BattleConfig{
		TroopID:   troopID,
		CanEscape: canEscape,
		CanLose:   canLose,
		Res:       bm.res,
		Logger:    bm.logger,
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
					participants = append(participants, m)
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

	// Start event broadcaster in background.
	go bm.broadcastEvents(bi, participants)

	// Run battle (blocks).
	result := bi.Run(ctx)

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
		for _, p := range participants {
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

	// TODO: Equipment bonuses and traits can be added when PlayerSession
	// tracks equipped items. For now, equipment stat bonuses are already
	// factored into the session's HP/MaxHP values.
	var equipBonus [8]int
	var equipTraits []resource.Trait

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
