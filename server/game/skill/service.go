package skill

import (
	"context"
	"encoding/json"
	"errors"
	"strconv"
	"time"

	"github.com/kasuganosora/rpgmakermvmmo/server/cache"
	"github.com/kasuganosora/rpgmakermvmmo/server/game/battle"
	"github.com/kasuganosora/rpgmakermvmmo/server/game/player"
	"github.com/kasuganosora/rpgmakermvmmo/server/game/world"
	"github.com/kasuganosora/rpgmakermvmmo/server/model"
	"github.com/kasuganosora/rpgmakermvmmo/server/resource"
	"go.uber.org/zap"
	"gorm.io/gorm"
)

// SkillService handles skill use, CD, and AoE.
type SkillService struct {
	cache  cache.Cache
	res    *resource.ResourceLoader
	wm     *world.WorldManager
	db     *gorm.DB
	logger *zap.Logger
}

// NewSkillService creates a new SkillService.
func NewSkillService(c cache.Cache, res *resource.ResourceLoader, wm *world.WorldManager, db *gorm.DB, logger *zap.Logger) *SkillService {
	return &SkillService{cache: c, res: res, wm: wm, db: db, logger: logger}
}

// cdKey returns the Cache key for a player's skill cooldown hash.
func cdKey(playerID int64) string {
	return "player:" + strconv.FormatInt(playerID, 10) + ":skill_cd"
}

// IsOnCooldown reports whether skillID is still on cooldown for playerID.
func (svc *SkillService) IsOnCooldown(ctx context.Context, playerID int64, skillID int) (bool, error) {
	val, err := svc.cache.HGet(ctx, cdKey(playerID), strconv.Itoa(skillID))
	if err != nil || val == "" {
		return false, nil
	}
	readyAt, err := strconv.ParseInt(val, 10, 64)
	if err != nil {
		return false, nil
	}
	return time.Now().UnixMilli() < readyAt, nil
}

// SetCooldown records a cooldown for skillID that expires after cdMs milliseconds.
func (svc *SkillService) SetCooldown(ctx context.Context, playerID int64, skillID int, cdMs int64) error {
	readyAt := time.Now().UnixMilli() + cdMs
	return svc.cache.HSet(ctx, cdKey(playerID), strconv.Itoa(skillID), strconv.FormatInt(readyAt, 10))
}

// buildAttackerStats builds CharacterStats from session and database.
func (svc *SkillService) buildAttackerStats(s *player.PlayerSession) *battle.CharacterStats {
	stats := &battle.CharacterStats{
		HP:    s.HP,
		MP:    s.MP,
		Level: s.Level,
		Atk:   10, Def: 5, Mat: 10, Mdf: 5, Agi: 10, Luk: 10, // defaults
	}

	// Load from database for accurate stats.
	if svc.db != nil {
		var char model.Character
		if err := svc.db.First(&char, s.CharID).Error; err == nil {
			stats.Atk = char.Atk
			stats.Def = char.Def
			stats.Mat = char.Mat
			stats.Mdf = char.Mdf
			stats.Agi = char.Agi
			stats.Luk = char.Luk
			stats.Level = char.Level
		}

		// Add equipment bonuses.
		if svc.res != nil {
			var invItems []model.Inventory
			if err := svc.db.Where("char_id = ? AND equipped = ?", s.CharID, true).Find(&invItems).Error; err == nil {
				for _, inv := range invItems {
					var es *resource.EquipStats
					if inv.Kind == 2 { // weapon
						for _, w := range svc.res.Weapons {
							if w != nil && w.ID == inv.ItemID {
								s := resource.EquipStatsFromParams(w.Params)
								es = &s
								break
							}
						}
					} else if inv.Kind == 3 { // armor
						for _, a := range svc.res.Armors {
							if a != nil && a.ID == inv.ItemID {
								s := resource.EquipStatsFromParams(a.Params)
								es = &s
								break
							}
						}
					}
					if es != nil {
						stats.Atk += es.Atk
						stats.Def += es.Def
						stats.Mat += es.Mat
						stats.Mdf += es.Mdf
						stats.Agi += es.Agi
						stats.Luk += es.Luk
					}
				}
			}
		}
	}

	return stats
}

// UseSkill handles a player_skill message.
func (svc *SkillService) UseSkill(
	ctx context.Context,
	s *player.PlayerSession,
	skillID int,
	targetID int64,
	targetType string,
) error {
	if svc.res == nil {
		return errors.New("resources not loaded")
	}

	// Find skill.
	var skill *resource.Skill
	for _, sk := range svc.res.Skills {
		if sk != nil && sk.ID == skillID {
			skill = sk
			break
		}
	}
	if skill == nil {
		return errors.New("unknown skill_id")
	}

	// CD check.
	onCD, err := svc.IsOnCooldown(ctx, s.CharID, skillID)
	if err != nil {
		return err
	}
	if onCD {
		return errors.New("skill still on cooldown")
	}

	// MP check.
	if s.MP < skill.MPCost {
		return errors.New("not enough MP")
	}

	// Deduct MP.
	s.MP -= skill.MPCost

	// Set cooldown (placeholder: 1 second default CD for skills without explicit CD).
	_ = svc.SetCooldown(ctx, s.CharID, skillID, 1000)

	// Build attack context using real character stats.
	atkStats := svc.buildAttackerStats(s)

	room := svc.wm.Get(s.MapID)
	if room == nil {
		return errors.New("not in a map")
	}

	// Collect targets.
	var targets []int64
	if targetID != 0 {
		targets = []int64{targetID}
	}

	// Compute damage for each target and broadcast.
	var targetResults []interface{}
	for _, tid := range targets {
		if targetType == "monster" {
			m := room.GetMonster(tid)
			if m == nil {
				continue
			}
			defStats := &battle.CharacterStats{
				HP: m.HP, Def: m.Template.Def, Mdf: m.Template.Mdf,
				Atk: m.Template.Atk, Mat: m.Template.Mat,
				Agi: m.Template.Agi, Luk: m.Template.Luk,
			}
			dmg := battle.Calculate(&battle.DamageContext{
				Attacker: atkStats,
				Defender: defStats,
				Skill:    skill,
			})
			m.TakeDamage(dmg.FinalDamage, s.CharID)
			targetResults = append(targetResults, map[string]interface{}{
				"target_id":   tid,
				"target_type": "monster",
				"damage":      dmg.FinalDamage,
				"is_heal":     false,
			})
		}
	}

	// Broadcast skill_effect with remaining CD in ms (client-safe, no system clock dependency).
	payload, _ := json.Marshal(map[string]interface{}{
		"caster_id":      s.CharID,
		"char_id":        s.CharID,
		"skill_id":       skillID,
		"animation_id":   skill.IconIndex, // placeholder
		"cd_remaining_ms": 1000,
		"targets":        targetResults,
	})
	pkt, _ := json.Marshal(&player.Packet{Type: "skill_effect", Payload: payload})
	room.Broadcast(pkt)

	return nil
}
