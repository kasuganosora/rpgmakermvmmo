package ws

import (
	"context"
	"encoding/json"

	"github.com/kasuganosora/rpgmakermvmmo/server/game/item"
	"github.com/kasuganosora/rpgmakermvmmo/server/game/player"
	gskill "github.com/kasuganosora/rpgmakermvmmo/server/game/skill"
	"github.com/kasuganosora/rpgmakermvmmo/server/game/world"
	"github.com/kasuganosora/rpgmakermvmmo/server/resource"
	"go.uber.org/zap"
	"gorm.io/gorm"
)

// SkillItemHandlers handles skill use, equip, and item use WS messages.
type SkillItemHandlers struct {
	skillSvc *gskill.SkillService
	invSvc   *item.InventoryService
	equipSvc *item.EquipService
	wm       *world.WorldManager
	logger   *zap.Logger
}

// NewSkillItemHandlers creates a new SkillItemHandlers.
func NewSkillItemHandlers(
	db *gorm.DB,
	res *resource.ResourceLoader,
	wm *world.WorldManager,
	skillSvc *gskill.SkillService,
	logger *zap.Logger,
) *SkillItemHandlers {
	return &SkillItemHandlers{
		skillSvc: skillSvc,
		invSvc:   item.NewInventoryService(db, logger),
		equipSvc: item.NewEquipService(db, res, logger),
		wm:       wm,
		logger:   logger,
	}
}

// RegisterHandlers registers skill/item handlers on the router.
func (sh *SkillItemHandlers) RegisterHandlers(r *Router) {
	r.On("player_skill", sh.HandleUseSkill)
	r.On("equip_item", sh.HandleEquipItem)
	r.On("unequip_item", sh.HandleUnequipItem)
	r.On("use_item", sh.HandleUseItem)
}

// ------------------------------------------------------------------ player_skill

type useSkillReq struct {
	SkillID    int    `json:"skill_id"`
	TargetID   int64  `json:"target_id"`
	TargetType string `json:"target_type"`
}

func (sh *SkillItemHandlers) HandleUseSkill(_ context.Context, s *player.PlayerSession, raw json.RawMessage) error {
	var req useSkillReq
	if err := json.Unmarshal(raw, &req); err != nil {
		return err
	}
	if err := sh.skillSvc.UseSkill(context.Background(), s, req.SkillID, req.TargetID, req.TargetType); err != nil {
		sendError(s, err.Error())
	}
	return nil
}

// ------------------------------------------------------------------ equip_item

type equipReq struct {
	InvID int64 `json:"inv_id"`
}

func (sh *SkillItemHandlers) HandleEquipItem(_ context.Context, s *player.PlayerSession, raw json.RawMessage) error {
	var req equipReq
	if err := json.Unmarshal(raw, &req); err != nil {
		return err
	}
	if err := sh.equipSvc.Equip(context.Background(), s, req.InvID); err != nil {
		item.BroadcastEquipResult(s, false, err.Error())
	} else {
		item.BroadcastEquipResult(s, true, "equipped")
	}
	return nil
}

// ------------------------------------------------------------------ unequip_item

func (sh *SkillItemHandlers) HandleUnequipItem(_ context.Context, s *player.PlayerSession, raw json.RawMessage) error {
	var req equipReq
	if err := json.Unmarshal(raw, &req); err != nil {
		return err
	}
	if err := sh.equipSvc.Unequip(context.Background(), s, req.InvID); err != nil {
		item.BroadcastEquipResult(s, false, err.Error())
	} else {
		item.BroadcastEquipResult(s, true, "unequipped")
	}
	return nil
}

// ------------------------------------------------------------------ use_item (consumable)

type useItemReq struct {
	InvID int64 `json:"inv_id"`
}

func (sh *SkillItemHandlers) HandleUseItem(_ context.Context, s *player.PlayerSession, raw json.RawMessage) error {
	var req useItemReq
	if err := json.Unmarshal(raw, &req); err != nil {
		return err
	}
	ctx := context.Background()
	if err := sh.invSvc.RemoveItem(ctx, s.CharID, 1 /*ItemKindItem*/, int(req.InvID), 1); err != nil {
		sendError(s, "use item failed: "+err.Error())
		return nil
	}
	// TODO: apply item effects (HP/MP restore, buff) based on Items.json effects[]
	sh.logger.Info("item used", zap.Int64("char_id", s.CharID), zap.Int64("inv_id", req.InvID))
	return nil
}
