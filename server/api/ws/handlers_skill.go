package ws

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/kasuganosora/rpgmakermvmmo/server/game/item"
	"github.com/kasuganosora/rpgmakermvmmo/server/game/player"
	gskill "github.com/kasuganosora/rpgmakermvmmo/server/game/skill"
	"github.com/kasuganosora/rpgmakermvmmo/server/game/world"
	"github.com/kasuganosora/rpgmakermvmmo/server/model"
	"github.com/kasuganosora/rpgmakermvmmo/server/resource"
	"go.uber.org/zap"
	"gorm.io/gorm"
)

// SkillItemHandlers handles skill use, equip, item use, and shop WS messages.
type SkillItemHandlers struct {
	db       *gorm.DB
	res      *resource.ResourceLoader
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
		db:       db,
		res:      res,
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
	r.On("shop_buy", sh.HandleShopBuy)
	r.On("shop_sell", sh.HandleShopSell)
	r.On("shop_close", sh.HandleShopClose)
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
	inv, err := sh.invSvc.UseItem(ctx, s.CharID, req.InvID)
	if err != nil {
		sendError(s, "use item failed: "+err.Error())
		return nil
	}
	// TODO: apply item effects (HP/MP restore, buff) based on Items.json effects[]

	// Notify client of the consumed item so the inventory UI stays in sync.
	item.NotifyUpdate(s, nil, []model.Inventory{{ID: inv.ID, ItemID: inv.ItemID, Kind: inv.Kind, Qty: 1}})

	sh.logger.Info("item used", zap.Int64("char_id", s.CharID), zap.Int64("inv_id", req.InvID), zap.Int("item_id", inv.ItemID))
	return nil
}

// ------------------------------------------------------------------ shop_buy

type shopBuyReq struct {
	GoodsType int `json:"goods_type"` // RMMV goods type: 0=item, 1=weapon, 2=armor
	ItemID    int `json:"item_id"`
	Qty       int `json:"qty"`
}

// rmmvGoodsToKind converts RMMV goods type (0/1/2) to model ItemKind (1/2/3).
func rmmvGoodsToKind(goodsType int) int {
	return goodsType + 1
}

// HandleShopBuy processes a shop_buy request. The player must have an active
// shop (ShopGoods set by the executor). Validates the item is in the shop,
// deducts gold, adds to inventory, and responds with the result.
func (sh *SkillItemHandlers) HandleShopBuy(_ context.Context, s *player.PlayerSession, raw json.RawMessage) error {
	var req shopBuyReq
	if err := json.Unmarshal(raw, &req); err != nil {
		return err
	}
	if req.Qty <= 0 {
		req.Qty = 1
	}

	if s.ShopGoods == nil {
		sendError(s, "no shop open")
		return nil
	}

	// Validate item is in the active shop and resolve price.
	price, err := sh.findShopPrice(s.ShopGoods, req.GoodsType, req.ItemID)
	if err != nil {
		sendError(s, err.Error())
		return nil
	}

	total := int64(price) * int64(req.Qty)
	kind := rmmvGoodsToKind(req.GoodsType)
	ctx := context.Background()

	txErr := sh.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var char model.Character
		if err := tx.Where("id = ?", s.CharID).First(&char).Error; err != nil {
			return err
		}
		if char.Gold < total {
			return errors.New("insufficient gold")
		}
		if err := tx.Model(&char).Update("gold", char.Gold-total).Error; err != nil {
			return err
		}
		// Consumables stack; weapons/armors create separate rows.
		if kind == model.ItemKindItem {
			var inv model.Inventory
			findErr := tx.Where("char_id = ? AND item_id = ? AND kind = ?", s.CharID, req.ItemID, kind).
				First(&inv).Error
			if findErr != nil {
				inv = model.Inventory{CharID: s.CharID, ItemID: req.ItemID, Kind: kind, Qty: req.Qty}
				return tx.Create(&inv).Error
			}
			newQty := inv.Qty + req.Qty
			if newQty > 9999 {
				return errors.New("exceeds maximum stack size")
			}
			return tx.Model(&inv).Update("qty", newQty).Error
		}
		for i := 0; i < req.Qty; i++ {
			inv := model.Inventory{CharID: s.CharID, ItemID: req.ItemID, Kind: kind, Qty: 1}
			if err := tx.Create(&inv).Error; err != nil {
				return err
			}
		}
		return nil
	})
	if txErr != nil {
		sendError(s, "shop buy failed: "+txErr.Error())
		return nil
	}

	payload, _ := json.Marshal(map[string]interface{}{
		"ok":    true,
		"spent": total,
	})
	s.Send(&player.Packet{Type: "shop_buy_result", Payload: payload})
	sh.logger.Info("shop buy",
		zap.Int64("char_id", s.CharID), zap.Int("item_id", req.ItemID),
		zap.Int("kind", kind), zap.Int("qty", req.Qty), zap.Int64("spent", total))
	return nil
}

// ------------------------------------------------------------------ shop_sell

type shopSellReq struct {
	GoodsType int `json:"goods_type"` // 0=item, 1=weapon, 2=armor
	ItemID    int `json:"item_id"`
	Qty       int `json:"qty"`
}

// HandleShopSell sells an item back to the shop at half price.
func (sh *SkillItemHandlers) HandleShopSell(_ context.Context, s *player.PlayerSession, raw json.RawMessage) error {
	var req shopSellReq
	if err := json.Unmarshal(raw, &req); err != nil {
		return err
	}
	if req.Qty <= 0 {
		req.Qty = 1
	}

	kind := rmmvGoodsToKind(req.GoodsType)
	basePrice := sh.resolveItemPrice(kind, req.ItemID)
	sellPrice := int64(basePrice / 2)
	if sellPrice <= 0 {
		sendError(s, "item cannot be sold")
		return nil
	}
	earned := sellPrice * int64(req.Qty)
	ctx := context.Background()

	txErr := sh.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var inv model.Inventory
		if err := tx.Where("char_id = ? AND item_id = ? AND kind = ?", s.CharID, req.ItemID, kind).
			First(&inv).Error; err != nil {
			return errors.New("item not found in inventory")
		}
		if inv.Qty < req.Qty {
			return fmt.Errorf("not enough items (have %d, want %d)", inv.Qty, req.Qty)
		}
		if inv.Qty == req.Qty {
			if err := tx.Delete(&inv).Error; err != nil {
				return err
			}
		} else {
			if err := tx.Model(&inv).Update("qty", inv.Qty-req.Qty).Error; err != nil {
				return err
			}
		}
		var char model.Character
		if err := tx.Where("id = ?", s.CharID).First(&char).Error; err != nil {
			return err
		}
		return tx.Model(&char).Update("gold", char.Gold+earned).Error
	})
	if txErr != nil {
		sendError(s, "shop sell failed: "+txErr.Error())
		return nil
	}

	payload, _ := json.Marshal(map[string]interface{}{
		"ok":     true,
		"earned": earned,
	})
	s.Send(&player.Packet{Type: "shop_sell_result", Payload: payload})
	sh.logger.Info("shop sell",
		zap.Int64("char_id", s.CharID), zap.Int("item_id", req.ItemID),
		zap.Int("kind", kind), zap.Int("qty", req.Qty), zap.Int64("earned", earned))
	return nil
}

// ------------------------------------------------------------------ shop_close

// HandleShopClose clears the active shop goods from the session.
func (sh *SkillItemHandlers) HandleShopClose(_ context.Context, s *player.PlayerSession, _ json.RawMessage) error {
	s.ShopGoods = nil
	return nil
}

// ---- shop helpers ----

// findShopPrice validates that an item is in the active shop goods and returns
// its effective price. RMMV goods format: [type, id, priceType, price].
// priceType=0 uses the database price; priceType=1 uses the custom price.
func (sh *SkillItemHandlers) findShopPrice(goods [][]interface{}, goodsType, itemID int) (int, error) {
	for _, g := range goods {
		if len(g) < 2 {
			continue
		}
		gType := asInt(g[0])
		gID := asInt(g[1])
		if gType == goodsType && gID == itemID {
			// Check for custom price (priceType=1, price in g[3]).
			if len(g) >= 4 && asInt(g[2]) == 1 {
				return asInt(g[3]), nil
			}
			// Use database price.
			kind := rmmvGoodsToKind(goodsType)
			return sh.resolveItemPrice(kind, itemID), nil
		}
	}
	return 0, errors.New("item not available in this shop")
}

// resolveItemPrice looks up the base price from resource data.
func (sh *SkillItemHandlers) resolveItemPrice(kind, itemID int) int {
	if sh.res == nil {
		return 0
	}
	switch kind {
	case model.ItemKindItem:
		for _, it := range sh.res.Items {
			if it != nil && it.ID == itemID {
				return it.Price
			}
		}
	case model.ItemKindWeapon:
		for _, w := range sh.res.Weapons {
			if w != nil && w.ID == itemID {
				return w.Price
			}
		}
	case model.ItemKindArmor:
		for _, a := range sh.res.Armors {
			if a != nil && a.ID == itemID {
				return a.Price
			}
		}
	}
	return 0
}

// asInt converts an interface{} (typically float64 from JSON) to int.
func asInt(v interface{}) int {
	switch n := v.(type) {
	case float64:
		return int(n)
	case int:
		return n
	case int64:
		return int(n)
	}
	return 0
}
