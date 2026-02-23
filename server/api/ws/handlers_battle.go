package ws

import (
	"context"
	"encoding/json"
	"time"

	"github.com/kasuganosora/rpgmakermvmmo/server/game/battle"
	"github.com/kasuganosora/rpgmakermvmmo/server/game/player"
	"github.com/kasuganosora/rpgmakermvmmo/server/game/world"
	"github.com/kasuganosora/rpgmakermvmmo/server/model"
	"github.com/kasuganosora/rpgmakermvmmo/server/resource"
	"go.uber.org/zap"
	"gorm.io/gorm"
)

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
}

// ------------------------------------------------------------------ attack

type attackReq struct {
	TargetID   int64 `json:"target_id"`
	TargetType string `json:"target_type"` // "player" | "monster"
	SkillID    int   `json:"skill_id"`    // 0 = basic attack
}

// HandleAttack processes a player attack request.
func (bh *BattleHandlers) HandleAttack(_ context.Context, s *player.PlayerSession, raw json.RawMessage) error {
	var req attackReq
	if err := json.Unmarshal(raw, &req); err != nil {
		return err
	}

	room := bh.wm.Get(s.MapID)
	if room == nil {
		sendError(s, "not in a map")
		return nil
	}

	// Build attacker stats from session.
	atkStats := &battle.CharacterStats{
		HP: s.HP, MP: s.MP,
		// These should be loaded from DB; simplified here using session values.
		Atk: 10, Def: 5, Mat: 10, Mdf: 5, Agi: 10, Luk: 10,
		Level: 1,
	}

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

	// Broadcast battle_result.
	brPayload, err := json.Marshal(map[string]interface{}{
		"attacker_id":   s.CharID,
		"target_id":     instID,
		"target_type":   "monster",
		"damage":        result.FinalDamage,
		"is_crit":       result.IsCrit,
		"effects":       []interface{}{},
		"target_hp":     monster.HP,
		"target_max_hp": monster.MaxHP,
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
	monster.SetState(6) // StateDead placeholder

	// Calculate drops.
	drops := battle.CalculateDrops(monster.Template)
	var dropInfos []interface{}
	dropID := int64(1)
	x, y := monster.X, monster.Y
	for _, d := range drops {
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
		dropID++
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

	// Schedule respawn: select on room stop channel so the goroutine doesn't
	// outlive the room and cause a leak.
	go func() {
		select {
		case <-time.After(30 * time.Second):
			room.RemoveMonster(monster.InstID)
		case <-room.StopChan():
			// Room was stopped; skip respawn.
		}
	}()
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
		s.Level = char.Level
		s.Exp = char.Exp

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

	room := bh.wm.Get(s.MapID)
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
	err := bh.db.Transaction(func(tx *gorm.DB) error {
		// ConsumeDrop is protected by room.mu; only one caller wins the race.
		if !room.ConsumeDrop(req.DropID) {
			return nil // drop already taken by another player
		}
		consumed = true
		inv := &model.Inventory{
			CharID: s.CharID,
			ItemID: drop.ItemID,
			Kind:   drop.ItemType,
			Qty:    1,
		}
		return tx.Create(inv).Error
	})
	if err != nil {
		return err
	}
	if !consumed {
		sendError(s, "item already picked up")
		return nil
	}

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
