// Package npc 实现 RMMV 事件指令的服务端执行引擎。
package npc

import (
	"context"
	"fmt"

	"github.com/kasuganosora/rpgmakermvmmo/server/game/player"
	"github.com/kasuganosora/rpgmakermvmmo/server/model"
	"github.com/kasuganosora/rpgmakermvmmo/server/resource"
	"go.uber.org/zap"
	"gorm.io/gorm"
)

// ---- RMMV 事件指令代码常量 ----

const (
	CmdEnd              = 0
	CmdShowText         = 101
	CmdShowTextLine     = 401
	CmdShowChoices      = 102
	CmdWhenBranch       = 402 // 选项分支（When [n]）
	CmdWhenCancel       = 403 // 选项取消分支
	CmdBranchEnd        = 404 // 选项/条件块结束
	CmdConditionalStart = 111
	CmdLoop             = 112
	CmdBreakLoop        = 113
	CmdExitEvent        = 115
	CmdCallCommonEvent  = 117
	CmdLabel            = 118
	CmdJumpToLabel      = 119
	CmdElseBranch       = 411
	CmdConditionalEnd   = 412
	CmdRepeatAbove      = 413
	CmdChangeSwitches   = 121
	CmdChangeVars       = 122
	CmdChangeSelfSwitch = 123
	CmdChangeGold       = 125
	CmdChangeItems      = 126
	CmdTransfer         = 201
	CmdWait             = 230
	CmdSetMoveRoute     = 205
	CmdMoveRouteCont    = 505
	CmdFadeout          = 221
	CmdFadein           = 222
	CmdTintScreen       = 223
	CmdFlashScreen      = 224
	CmdShakeScreen      = 225
	CmdScreenEffect     = 211 // 显示/隐藏动画
	CmdPlayBGM          = 241
	CmdStopBGM          = 242
	CmdPlayBGS          = 245
	CmdStopBGS          = 246
	CmdPlaySE           = 250
	CmdStopSE           = 251
	CmdPlayME           = 249
	CmdWaitForMoveRoute = 209
	CmdShowPicture      = 231
	CmdMovePicture      = 232
	CmdRotatePicture    = 233
	CmdTintPicture      = 234
	CmdErasePicture     = 235
	CmdShowAnimation    = 212
	CmdShowBalloon      = 213
	CmdEraseEvent       = 214
	CmdChangeHP         = 311
	CmdChangeMP         = 312
	CmdChangeState      = 313
	CmdRecoverAll       = 314
	CmdChangeEXP        = 315
	CmdChangeLevel      = 316
	CmdChangeParameter  = 317
	CmdChangeSkill      = 318
	CmdChangeEquipment  = 319
	CmdChangeName       = 320
	CmdChangeClass      = 321
	CmdChangeActorImage = 322
	CmdBattleProcessing = 301
	CmdShopProcessing   = 302
	CmdGameOver         = 353
	CmdReturnToTitle    = 354
	CmdComment          = 108
	CmdCommentCont      = 408
	CmdScript           = 355
	CmdScriptCont       = 655
	CmdPluginCommand    = 356
)

// ---- 核心接口 ----

// GameStateAccessor 提供开关、变量、独立开关的读写访问。
// TemplateEvent.js 扩展：还支持带数字索引的独立变量。
type GameStateAccessor interface {
	GetSwitch(id int) bool
	SetSwitch(id int, val bool)
	GetVariable(id int) int
	SetVariable(id int, val int)
	GetSelfSwitch(mapID, eventID int, ch string) bool
	SetSelfSwitch(mapID, eventID int, ch string, val bool)
	// 独立变量方法（TemplateEvent.js 扩展）
	GetSelfVariable(mapID, eventID, index int) int
	SetSelfVariable(mapID, eventID, index, val int)
}

// InventoryStore 提供金币和物品的持久化操作。
// 从 Executor 中提取此接口以支持单元测试中的 mock 替换。
type InventoryStore interface {
	// GetGold 查询角色当前金币数。
	GetGold(ctx context.Context, charID int64) (int64, error)
	// UpdateGold 增减角色金币（amount 可为负数）。
	UpdateGold(ctx context.Context, charID int64, amount int64) error
	// GetItem 查询角色背包中指定物品的数量，不存在返回 0。
	GetItem(ctx context.Context, charID int64, itemID int) (qty int, err error)
	// AddItem 增加物品数量，若不存在则创建记录。
	AddItem(ctx context.Context, charID int64, itemID, qty int) error
	// RemoveItem 减少物品数量，数量归零则删除记录。
	RemoveItem(ctx context.Context, charID int64, itemID, qty int) error
}

// ---- 回调类型 ----

// TransferFunc 在执行地图传送指令（201）时由 Executor 调用，执行服务端地图切换。
type TransferFunc func(s *player.PlayerSession, mapID, x, y, dir int)

// BattleFunc 在执行战斗处理指令（301）时由 Executor 调用，创建服务端权威战斗会话。
type BattleFunc func(ctx context.Context, s *player.PlayerSession, troopID int, canEscape, canLose bool) int

// ---- 执行选项 ----

// ExecuteOpts 包含 Execute 方法的可选参数。
type ExecuteOpts struct {
	GameState  GameStateAccessor // 当前执行的游戏状态访问器
	MapID      int               // 当前地图 ID
	EventID    int               // 当前事件 ID
	TransferFn TransferFunc      // 服务端传送处理器
	BattleFn   BattleFunc        // 服务端战斗处理器
}

// ---- Executor 核心结构体 ----

// Executor 为玩家会话执行 RMMV 事件指令列表。
type Executor struct {
	store  InventoryStore           // 金币/物品持久化（可 mock）
	res    *resource.ResourceLoader // RMMV 资源数据（只读）
	logger *zap.Logger
}

// New 创建 Executor，接受 InventoryStore 接口以支持测试 mock。
func New(store InventoryStore, res *resource.ResourceLoader, logger *zap.Logger) *Executor {
	return &Executor{store: store, res: res, logger: logger}
}

// NewWithDB 创建使用 GORM 数据库的 Executor（生产环境便捷构造函数）。
func NewWithDB(db *gorm.DB, res *resource.ResourceLoader, logger *zap.Logger) *Executor {
	return New(&gormInventoryStore{db: db}, res, logger)
}

// ---- 公共 API ----

// Execute 处理单个事件页的所有指令。
// 对话指令发送给客户端；物品/金币变更立即生效。
// 发送选项后阻塞等待玩家通过 s.ChoiceCh 回复。
func (e *Executor) Execute(ctx context.Context, s *player.PlayerSession, page *resource.EventPage, opts *ExecuteOpts) {
	if page == nil {
		return
	}
	e.executeList(ctx, s, page.List, opts, 0)
	e.sendDialogEnd(s)
}

// SendStateSyncAfterExecution 在 Execute 完成后发送待同步的状态更新。
// 当前为空操作，因为状态变更已在执行过程中逐条发送。
// 此方法作为未来批量同步优化的扩展点保留。
func (e *Executor) SendStateSyncAfterExecution(_ context.Context, _ *player.PlayerSession, _ *ExecuteOpts) {
}

// ExecuteEventByID 按事件 ID 查找地图事件并执行其第一页。
func (e *Executor) ExecuteEventByID(ctx context.Context, s *player.PlayerSession, mapID, eventID int) {
	md, ok := e.res.Maps[mapID]
	if !ok {
		return
	}
	for _, ev := range md.Events {
		if ev == nil || ev.ID != eventID {
			continue
		}
		if len(ev.Pages) > 0 {
			e.Execute(ctx, s, ev.Pages[0], &ExecuteOpts{MapID: mapID, EventID: eventID})
		}
		return
	}
	e.logger.Warn("event not found", zap.Int("map_id", mapID), zap.Int("event_id", eventID))
}

// ---- gormInventoryStore：基于 GORM 的 InventoryStore 默认实现 ----

// gormInventoryStore 使用 GORM 实现金币和物品的数据库持久化。
type gormInventoryStore struct {
	db *gorm.DB
}

// GetGold 查询角色当前金币数。
func (s *gormInventoryStore) GetGold(ctx context.Context, charID int64) (int64, error) {
	var char model.Character
	if err := s.db.WithContext(ctx).Select("gold").First(&char, charID).Error; err != nil {
		return 0, err
	}
	return char.Gold, nil
}

// UpdateGold 增减角色金币。
func (s *gormInventoryStore) UpdateGold(ctx context.Context, charID int64, amount int64) error {
	return s.db.WithContext(ctx).Model(&model.Character{}).Where("id = ?", charID).
		Update("gold", gorm.Expr("gold + ?", amount)).Error
}

// GetItem 查询角色背包中指定物品的数量。
func (s *gormInventoryStore) GetItem(ctx context.Context, charID int64, itemID int) (int, error) {
	var inv model.Inventory
	err := s.db.WithContext(ctx).
		Where("char_id = ? AND item_id = ? AND kind = 1", charID, itemID).
		First(&inv).Error
	if err != nil {
		return 0, err
	}
	return inv.Qty, nil
}

// AddItem 增加物品数量，若不存在则创建新记录。
func (s *gormInventoryStore) AddItem(ctx context.Context, charID int64, itemID, qty int) error {
	var inv model.Inventory
	err := s.db.WithContext(ctx).
		Where("char_id = ? AND item_id = ? AND kind = 1", charID, itemID).
		First(&inv).Error
	if err != nil {
		inv = model.Inventory{CharID: charID, ItemID: itemID, Kind: model.ItemKindItem, Qty: qty}
		return s.db.WithContext(ctx).Create(&inv).Error
	}
	return s.db.WithContext(ctx).Model(&inv).Update("qty", inv.Qty+qty).Error
}

// RemoveItem 减少物品数量，数量归零时删除记录。
func (s *gormInventoryStore) RemoveItem(ctx context.Context, charID int64, itemID, qty int) error {
	var inv model.Inventory
	if err := s.db.WithContext(ctx).
		Where("char_id = ? AND item_id = ? AND kind = 1", charID, itemID).
		First(&inv).Error; err != nil {
		return fmt.Errorf("item %d not found in inventory", itemID)
	}
	newQty := inv.Qty - qty
	if newQty <= 0 {
		return s.db.WithContext(ctx).Delete(&inv).Error
	}
	return s.db.WithContext(ctx).Model(&inv).Update("qty", newQty).Error
}
