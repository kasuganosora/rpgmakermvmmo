package ws

import (
	"context"
	"encoding/json"
	"sync/atomic"

	"github.com/kasuganosora/rpgmakermvmmo/server/game/ai"
	"github.com/kasuganosora/rpgmakermvmmo/server/game/battle"
	"github.com/kasuganosora/rpgmakermvmmo/server/game/item"
	"github.com/kasuganosora/rpgmakermvmmo/server/game/player"
	"github.com/kasuganosora/rpgmakermvmmo/server/game/world"
	"github.com/kasuganosora/rpgmakermvmmo/server/model"
	"github.com/kasuganosora/rpgmakermvmmo/server/resource"
	"go.uber.org/zap"
	"gorm.io/gorm"
)

// dropIDCounter generates globally unique drop IDs.
var dropIDCounter int64

// buildPlayerStats computes CharacterStats from session data (class base + equipment bonuses).
// Uses the in-memory session Equips map (always up-to-date) instead of DB queries.
func (bh *BattleHandlers) buildPlayerStats(s *player.PlayerSession) *battle.CharacterStats {
	hp, maxHP, mp, _ := s.Stats()
	level := s.GetLevel()
	if level < 1 {
		level = 1
	}
	classID := s.GetClassID()

	// Start with class base params at current level.
	// RMMV Params[paramID][level]: 0=MHP, 1=MMP, 2=ATK, 3=DEF, 4=MAT, 5=MDF, 6=AGI, 7=LUK
	var baseParams [8]int
	baseParams[0] = maxHP
	baseParams[1] = mp // fallback: current MP as base
	if bh.res != nil {
		cls := bh.res.ClassByID(classID)
		if cls != nil {
			idx := level
			for p := 0; p < 8; p++ {
				if p < len(cls.Params) {
					row := cls.Params[p]
					if idx < len(row) && row[idx] > 0 {
						baseParams[p] = row[idx]
					}
				}
			}
		}
	}

	// Add equipment bonuses from session equip map.
	equips := s.EquipsSnapshot()
	if bh.res != nil {
		for _, itemID := range equips {
			if itemID <= 0 {
				continue
			}
			// Check weapons.
			for _, w := range bh.res.Weapons {
				if w != nil && w.ID == itemID {
					es := resource.EquipStatsFromParams(w.Params)
					baseParams[0] += es.MaxHP
					baseParams[1] += es.MaxMP
					baseParams[2] += es.Atk
					baseParams[3] += es.Def
					baseParams[4] += es.Mat
					baseParams[5] += es.Mdf
					baseParams[6] += es.Agi
					baseParams[7] += es.Luk
					break
				}
			}
			// Check armors.
			for _, a := range bh.res.Armors {
				if a != nil && a.ID == itemID {
					es := resource.EquipStatsFromParams(a.Params)
					baseParams[0] += es.MaxHP
					baseParams[1] += es.MaxMP
					baseParams[2] += es.Atk
					baseParams[3] += es.Def
					baseParams[4] += es.Mat
					baseParams[5] += es.Mdf
					baseParams[6] += es.Agi
					baseParams[7] += es.Luk
					break
				}
			}
		}
	}

	return &battle.CharacterStats{
		HP:    hp,
		MP:    mp,
		Atk:   baseParams[2],
		Def:   baseParams[3],
		Mat:   baseParams[4],
		Mdf:   baseParams[5],
		Agi:   baseParams[6],
		Luk:   baseParams[7],
		Level: level,
	}
}

// BattleHandlers handles combat-related WS messages.
type BattleHandlers struct {
	db     *gorm.DB
	wm     *world.WorldManager
	res    *resource.ResourceLoader
	logger *zap.Logger
}

// NewBattleHandlers creates a new BattleHandlers.
func NewBattleHandlers(db *gorm.DB, wm *world.WorldManager, res *resource.ResourceLoader, logger *zap.Logger) *BattleHandlers {
	return &BattleHandlers{db: db, wm: wm, res: res, logger: logger}
}

// RegisterHandlers registers all battle handlers on the router.
func (bh *BattleHandlers) RegisterHandlers(r *Router) {
	r.On("attack", bh.HandleAttack)
	r.On("pickup_item", bh.HandlePickup)
	r.On("revive_request", bh.HandleReviveRequest)

	// Inject monster→player damage callback into WorldManager.
	bh.wm.SetMonsterDamageFunc(bh.handleMonsterAttackPlayer)
}

// handleMonsterAttackPlayer is called by monster AI when an AttackTarget node fires.
func (bh *BattleHandlers) handleMonsterAttackPlayer(m *world.MonsterRuntime, targetCharID int64, room *world.MapRoom) {
	s := room.GetPlayerSession(targetCharID)
	if s == nil || s.IsDead() {
		return
	}

	// Build monster attacker stats from template.
	t := m.Template
	if t == nil {
		return
	}
	atkStats := &battle.CharacterStats{
		HP: m.HP, MP: 0,
		Atk: t.Atk, Def: t.Def, Mat: t.Mat, Mdf: t.Mdf,
		Agi: t.Agi, Luk: t.Luk,
		Level: 1,
	}

	// Build player defender stats.
	defStats := bh.buildPlayerStats(s)

	dmgCtx := &battle.DamageContext{
		Attacker: atkStats,
		Defender: defStats,
	}
	result := battle.Calculate(dmgCtx)

	newHP, dead := s.ApplyDamage(result.FinalDamage)

	// Broadcast battle_result (monster → player).
	px, py, _ := s.Position()
	brPayload, err := json.Marshal(map[string]interface{}{
		"attacker_id":   m.InstID,
		"attacker_type": "monster",
		"target_id":     targetCharID,
		"target_type":   "player",
		"damage":        result.FinalDamage,
		"is_crit":       result.IsCrit,
		"target_hp":     newHP,
		"x":             px,
		"y":             py,
	})
	if err != nil {
		return
	}
	brPkt, _ := json.Marshal(&player.Packet{Type: "battle_result", Payload: brPayload})
	room.Broadcast(brPkt)

	if dead {
		bh.handlePlayerDeath(s, room)
	}
}

// ------------------------------------------------------------------ attack

type attackReq struct {
	TargetID   int64 `json:"target_id"`
	TargetType string `json:"target_type"` // "player" | "monster"
	SkillID    int   `json:"skill_id"`    // 0 = basic attack
}

// battleConfig returns the BattleMMOConfig (may be nil).
func (bh *BattleHandlers) battleConfig() *resource.BattleMMOConfig {
	if bh.res != nil && bh.res.MMOConfig != nil {
		return bh.res.MMOConfig.Battle
	}
	return nil
}

// HandleAttack processes a player attack request.
func (bh *BattleHandlers) HandleAttack(_ context.Context, s *player.PlayerSession, raw json.RawMessage) error {
	var req attackReq
	if err := json.Unmarshal(raw, &req); err != nil {
		return err
	}

	// Combat mode check: field attacks disabled in "turnbased" mode.
	cfg := bh.battleConfig()
	if cfg.GetCombatMode() == "turnbased" {
		sendError(s, "field attacks disabled in turn-based mode")
		return nil
	}

	// Dead players cannot attack.
	if s.IsDead() {
		sendError(s, "cannot attack while dead")
		return nil
	}

	// GCD check.
	if !s.CheckAttackGCD(cfg.GetGCDMs()) {
		sendError(s, "attack on cooldown")
		return nil
	}

	room := bh.wm.GetPlayerRoom(s)
	if room == nil {
		sendError(s, "not in a map")
		return nil
	}

	// Build attacker stats from session (class base params + equipment bonuses).
	atkStats := bh.buildPlayerStats(s)

	// Look up skill.
	var skill *resource.Skill
	if req.SkillID > 0 && bh.res != nil {
		for _, sk := range bh.res.Skills {
			if sk != nil && sk.ID == req.SkillID {
				skill = sk
				break
			}
		}
	}

	// Skill cost enforcement.
	if cfg != nil && cfg.EnforceSkillCosts && skill != nil {
		if skill.MPCost > 0 {
			if !s.ConsumeMP(skill.MPCost) {
				sendError(s, "not enough MP")
				return nil
			}
		}
	}

	switch req.TargetType {
	case "monster":
		bh.attackMonster(s, room, req.TargetID, atkStats, skill)
	case "player":
		// PvP: placeholder
		sendError(s, "pvp not enabled")
	default:
		sendError(s, "unknown target_type")
	}
	return nil
}

func (bh *BattleHandlers) attackMonster(
	s *player.PlayerSession,
	room *world.MapRoom,
	instID int64,
	atkStats *battle.CharacterStats,
	skill *resource.Skill,
) {
	monster := room.GetMonster(instID)
	if monster == nil {
		sendError(s, "monster not found")
		return
	}

	// Range validation (Manhattan distance). Only enforced when battle config exists.
	if cfg := bh.battleConfig(); cfg != nil {
		maxRange := cfg.GetAttackRange()
		px, py, _ := s.Position()
		dx := px - monster.X
		if dx < 0 {
			dx = -dx
		}
		dy := py - monster.Y
		if dy < 0 {
			dy = -dy
		}
		if dx+dy > maxRange {
			sendError(s, "target out of range")
			return
		}
	}

	// Build defender stats from monster template.
	t := monster.Template
	defStats := &battle.CharacterStats{
		HP: monster.HP, MP: 0,
		Atk: t.Atk, Def: t.Def, Mat: t.Mat, Mdf: t.Mdf,
		Agi: t.Agi, Luk: t.Luk,
		Level: 1,
	}

	dmgCtx := &battle.DamageContext{
		Attacker: atkStats,
		Defender: defStats,
		Skill:    skill,
	}
	result := battle.Calculate(dmgCtx)

	dead := monster.TakeDamage(result.FinalDamage, s.CharID)

	// Broadcast battle_result (includes monster x/y for client damage popup positioning).
	brPayload, err := json.Marshal(map[string]interface{}{
		"attacker_id":   s.CharID,
		"target_id":     instID,
		"target_type":   "monster",
		"damage":        result.FinalDamage,
		"is_crit":       result.IsCrit,
		"effects":       []interface{}{},
		"target_hp":     monster.HP,
		"target_max_hp": monster.MaxHP,
		"x":             monster.X,
		"y":             monster.Y,
	})
	if err != nil {
		bh.logger.Error("failed to marshal battle_result payload", zap.Error(err))
		return
	}
	brPkt, err := json.Marshal(&player.Packet{Type: "battle_result", Payload: brPayload})
	if err != nil {
		bh.logger.Error("failed to marshal battle_result packet", zap.Error(err))
		return
	}
	room.Broadcast(brPkt)

	if dead {
		bh.handleMonsterDeath(s, room, monster)
	}
}

func (bh *BattleHandlers) handleMonsterDeath(
	s *player.PlayerSession,
	room *world.MapRoom,
	monster *world.MonsterRuntime,
) {
	monster.SetState(ai.StateDead)

	// Clear threat table on death.
	if monster.Threat != nil {
		monster.Threat.Clear()
	}

	// Calculate drops.
	drops := battle.CalculateDrops(monster.Template)
	var dropInfos []interface{}
	x, y := monster.X, monster.Y
	for _, d := range drops {
		dropID := atomic.AddInt64(&dropIDCounter, 1)
		di := map[string]interface{}{
			"drop_id":   dropID,
			"item_type": d.ItemType,
			"item_id":   d.ItemID,
			"qty":       d.Quantity,
			"x":         x,
			"y":         y,
		}
		dropInfos = append(dropInfos, di)
		room.AddDrop(dropID, d.ItemType, d.ItemID, x, y)
	}

	// Broadcast monster_death.
	deathPayload, err := json.Marshal(map[string]interface{}{
		"inst_id": monster.InstID,
		"drops":   dropInfos,
		"exp":     monster.Template.Exp,
	})
	if err != nil {
		bh.logger.Error("failed to marshal monster_death payload", zap.Error(err))
		return
	}
	deathPkt, err := json.Marshal(&player.Packet{Type: "monster_death", Payload: deathPayload})
	if err != nil {
		bh.logger.Error("failed to marshal monster_death packet", zap.Error(err))
		return
	}
	room.Broadcast(deathPkt)

	// Award exp to the killer (single-player for now).
	expEach := battle.CalculateExp(monster.Template.Exp, 1)
	bh.awardExp(s, expEach)

	// Broadcast drop_spawn for each drop.
	for _, di := range dropInfos {
		spawnPayload, err := json.Marshal(di)
		if err != nil {
			bh.logger.Error("failed to marshal drop_spawn payload", zap.Error(err))
			continue
		}
		spawnPkt, err := json.Marshal(&player.Packet{Type: "drop_spawn", Payload: spawnPayload})
		if err != nil {
			bh.logger.Error("failed to marshal drop_spawn packet", zap.Error(err))
			continue
		}
		room.Broadcast(spawnPkt)
	}

	// Delegate cleanup + respawn to the room's spawner system.
	// If a spawner is configured, it uses SpawnConfig.RespawnSec;
	// otherwise the monster is simply removed after 5s (RMMV default: no respawn).
	room.NotifyMonsterDeath(monster.InstID)
}

func (bh *BattleHandlers) awardExp(s *player.PlayerSession, exp int) {
	// Async DB update wrapped in a transaction to prevent the read-modify-write
	// race when multiple monsters die in quick succession.
	go func() {
		var char model.Character
		var levelBefore int
		err := bh.db.Transaction(func(tx *gorm.DB) error {
			if err := tx.First(&char, s.CharID).Error; err != nil {
				return err
			}
			levelBefore = char.Level
			char.Exp += int64(exp)
			for char.Exp >= int64(battle.ExpNeeded(char.Level)) && char.Level < 99 {
				char.Level++
			}
			return tx.Model(&char).Updates(map[string]interface{}{
				"exp":   char.Exp,
				"level": char.Level,
			}).Error
		})
		if err != nil {
			bh.logger.Error("awardExp db error",
				zap.Int64("char_id", s.CharID),
				zap.Error(err))
			return
		}

		// Sync back to session so disconnect save has correct values.
		s.SetLevel(char.Level)
		s.SetExp(char.Exp)

		expPayload, err := json.Marshal(map[string]interface{}{
			"exp":       exp,
			"total_exp": char.Exp,
			"level":     char.Level,
			"level_up":  char.Level > levelBefore,
		})
		if err != nil {
			bh.logger.Error("awardExp marshal error", zap.Error(err))
			return
		}
		s.Send(&player.Packet{Type: "exp_gain", Payload: expPayload})
	}()
}

// ------------------------------------------------------------------ player death / revive

// handlePlayerDeath is called when a player's HP reaches 0.
func (bh *BattleHandlers) handlePlayerDeath(s *player.PlayerSession, room *world.MapRoom) {
	// Apply exp penalty if configured.
	cfg := bh.battleConfig()
	if cfg != nil && cfg.DeathPenaltyExpPct > 0 {
		go bh.applyDeathPenalty(s, cfg.DeathPenaltyExpPct)
	}

	// Notify the dead player (triggers gray death overlay on client).
	s.Send(&player.Packet{Type: "player_death"})

	// Broadcast to room so other players see the death effect.
	deathPayload, _ := json.Marshal(map[string]interface{}{
		"char_id": s.CharID,
	})
	pkt, _ := json.Marshal(&player.Packet{Type: "player_die_effect", Payload: deathPayload})
	room.Broadcast(pkt)
}

// applyDeathPenalty deducts a percentage of current-level exp on death.
func (bh *BattleHandlers) applyDeathPenalty(s *player.PlayerSession, pct int) {
	exp := s.GetExp()
	penalty := exp * int64(pct) / 100
	if penalty <= 0 {
		return
	}
	newExp := exp - penalty
	if newExp < 0 {
		newExp = 0
	}
	s.SetExp(newExp)

	// Persist to DB.
	if bh.db != nil {
		bh.db.Model(&model.Character{}).Where("id = ?", s.CharID).Update("exp", newExp)
	}
}

// HandleReviveRequest processes a client's request to revive after death.
func (bh *BattleHandlers) HandleReviveRequest(_ context.Context, s *player.PlayerSession, _ json.RawMessage) error {
	if !s.IsDead() {
		return nil
	}

	cfg := bh.battleConfig()

	// Determine revive HP (50% of max by default).
	_, maxHP, _, _ := s.Stats()
	reviveHP := maxHP / 2
	if reviveHP < 1 {
		reviveHP = 1
	}
	s.Revive(reviveHP)

	// Check if we need to transfer to a revive map.
	if cfg != nil && cfg.DeathReviveMapID > 0 {
		// Send transfer command to client (the transfer handler will move the player).
		transferPayload, _ := json.Marshal(map[string]interface{}{
			"map_id": cfg.DeathReviveMapID,
			"x":      cfg.DeathReviveX,
			"y":      cfg.DeathReviveY,
		})
		s.Send(&player.Packet{Type: "revive_transfer", Payload: transferPayload})
	}

	// Send revive confirmation (hides death overlay).
	revivePayload, _ := json.Marshal(map[string]interface{}{
		"hp": reviveHP,
	})
	s.Send(&player.Packet{Type: "player_revive", Payload: revivePayload})

	return nil
}

// ------------------------------------------------------------------ pickup_item

type pickupReq struct {
	DropID int64 `json:"drop_id"`
}

// HandlePickup processes a pickup_item request.
func (bh *BattleHandlers) HandlePickup(_ context.Context, s *player.PlayerSession, raw json.RawMessage) error {
	var req pickupReq
	if err := json.Unmarshal(raw, &req); err != nil {
		return err
	}

	room := bh.wm.GetPlayerRoom(s)
	if room == nil {
		sendError(s, "not in a map")
		return nil
	}

	drop := room.GetDrop(req.DropID)
	if drop == nil {
		sendError(s, "drop not found")
		return nil
	}

	// Check proximity (Manhattan distance ≤ 2).
	px, py, _ := s.Position()
	dx := drop.X - px
	dy := drop.Y - py
	if dx < 0 {
		dx = -dx
	}
	if dy < 0 {
		dy = -dy
	}
	if dx+dy > 2 {
		sendError(s, "too far to pick up")
		return nil
	}

	// Attempt to claim the drop and add to inventory atomically.
	// consumed is set inside the transaction so we know whether to broadcast.
	var consumed bool
	var pickedInv model.Inventory
	err := bh.db.Transaction(func(tx *gorm.DB) error {
		// ConsumeDrop is protected by room.mu; only one caller wins the race.
		if !room.ConsumeDrop(req.DropID) {
			return nil // drop already taken by another player
		}
		consumed = true
		pickedInv = model.Inventory{
			CharID: s.CharID,
			ItemID: drop.ItemID,
			Kind:   drop.ItemType,
			Qty:    1,
		}
		return tx.Create(&pickedInv).Error
	})
	if err != nil {
		return err
	}
	if !consumed {
		sendError(s, "item already picked up")
		return nil
	}

	// Notify the picker that an item was added to their inventory.
	item.NotifyUpdate(s, []model.Inventory{pickedInv}, nil)

	// Broadcast drop_remove only after a successful claim.
	removePayload, err := json.Marshal(map[string]interface{}{"drop_id": req.DropID})
	if err != nil {
		bh.logger.Error("failed to marshal drop_remove payload", zap.Error(err))
		return nil
	}
	removePkt, err := json.Marshal(&player.Packet{Type: "drop_remove", Payload: removePayload})
	if err != nil {
		bh.logger.Error("failed to marshal drop_remove packet", zap.Error(err))
		return nil
	}
	room.Broadcast(removePkt)

	bh.logger.Info("item picked up",
		zap.Int64("char_id", s.CharID),
		zap.Int64("drop_id", req.DropID))
	return nil
}
