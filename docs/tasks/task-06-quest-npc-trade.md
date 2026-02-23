# Task 06 - 任务、NPC 事件与交易系统

> **优先级**：P2 — M5 里程碑
> **里程碑**：M5（完成一条完整任务链）
> **依赖**：Task 03（怪物击杀 Hook）、Task 04（背包/物品）、Task 05（PubSub/Session）

---

## 目标

实现 NPC 对话事件解释执行（服务端逐条执行 RMMV 事件指令）、任务系统（接受/进度追踪/完成/奖励）、玩家间交易系统（防物品复制的分布式锁 + DB 事务），以及副本地图管理。

---

## Todolist

- [ ] **06-1** 实现 NPC/事件解释器（`game/world/event_executor.go`）
  - [ ] 06-1a 事件指令遍历执行器（for loop over list[]）
  - [ ] 06-1b `ShowText (code=101/401)`：推送 `npc_dialog`，等待客户端继续
  - [ ] 06-1c `ShowChoices (code=102/402)`：推送带 choices 的 `npc_dialog`，等待 `npc_choice` 回应
  - [ ] 06-1d `ConditionalBranch (code=111)`：基于 switch/variable/char 条件分支
  - [ ] 06-1e `ChangeItems (code=126)` / `ChangeGold (code=125)`：调用背包服务
  - [ ] 06-1f `TransferPlayer (code=201)`：推送 `enter_map` 命令（地图传送）
  - [ ] 06-1g `ChangeSwitch (code=121)` / `ChangeVariable (code=122)`：更新玩家 switch/variable（存 Cache + 异步写 DB）
  - [ ] 06-1h `Script (code=355/655)`：调用 JS 沙箱（Task 08 接入，此处预留接口）
  - [ ] 06-1i `Wait (code=230)`：等待 N 帧（基于 50ms tick）
  - [ ] 06-1j `StartBattle (code=301)`：保留 RMMV 回合制战斗（仅剧情用，推送 battle_start 占位）
  - [ ] 06-1k NPC 触发条件页选择（`page.conditions`：switch/variable/selfswitch/actor）
- [ ] **06-2** 实现 NPC 交互处理（`HandleNPCInteract` + `HandleNPCChoice`）
  - [ ] `npc_interact{npc_id, event_id}` → 找对应 MapRoom 中的 NPC，选择有效事件页，启动执行器
  - [ ] `npc_choice{choice_index}` → 继续执行器
  - [ ] 每个玩家同时只能有一个事件执行会话（`session.eventCtx`）
- [ ] **06-3** 实现任务系统（`game/quest/`）
  - [ ] 06-3a 任务解析：从 CommonEvents.json 提取任务定义（name, conditions, objectives, rewards）
  - [ ] 06-3b `HandleQuestAccept` — 接受任务（校验条件、无重复、写 DB）
  - [ ] 06-3c `HandleQuestAbandon` — 放弃任务
  - [ ] 06-3d `HandleQuestTrack` — 设置追踪任务（最多3条）
  - [ ] 06-3e 任务进度更新（Task 03 怪物死亡 Hook → kill 类型目标）
  - [ ] 06-3f 任务进度更新（Task 04 背包 Hook → collect 类型目标）
  - [ ] 06-3g 任务进度更新（Task 02 位置 Hook → goto 类型目标）
  - [ ] 06-3h 任务进度更新（NPC 对话结束 → talk 类型目标）
  - [ ] 06-3i 任务完成检测 + 奖励发放（经验/金币/物品）+ 推送 `quest_update`
- [ ] **06-4** 实现玩家变量/开关系统（`game/player/variables.go`）
  - [ ] Cache `player:{id}:vars` Hash（varID → value）
  - [ ] Cache `player:{id}:switches` Hash（switchID → "0"/"1"）
  - [ ] 玩家上线时从 DB 加载，下线时（或定时）写回 DB
  - [ ] 需要在 characters 表新增 `vars` 和`switches` JSON 列（GORM AutoMigrate）
- [ ] **06-5** 实现交易系统（`game/trade/`）
  - [ ] 06-5a `HandleTradeRequest` — 发起交易请求
  - [ ] 06-5b `HandleTradeOffer` — 放入物品/金币（双方实时看到变化）
  - [ ] 06-5c `HandleTradeConfirm` — 确认交易
  - [ ] 06-5d `HandleTradeCancel` — 取消交易
  - [ ] 06-5e 交易提交（`TradeService.Commit`）：分布式锁 + DB 事务
  - [ ] 06-5f 审计日志：action=trade_commit（Task 07 接入，此处预留接口）
- [ ] **06-6** 实现副本系统（`game/world/instance.go`）
  - [ ] 组队申请进入副本 → WorldManager 动态创建独立 MapRoom
  - [ ] 所有玩家离开副本 → 自动销毁 MapRoom 并清理资源
- [ ] **06-7** 商城 REST API（`api/rest/shop.go`）
  - [ ] `GET /api/shop/items` — 商城商品列表（从 config 中配置可售物品）
  - [ ] `POST /api/shop/buy` — 购买（扣金币，背包加物品，审计日志）
- [ ] **06-8** 邮件 REST API（`api/rest/mail.go`）
  - [ ] `GET /api/mail` — 邮件列表
  - [ ] `POST /api/mail/:id/claim` — 领取附件
  - [ ] `DELETE /api/mail/:id` — 删除邮件
- [ ] **06-9** 排行榜 REST API（`api/rest/ranking.go`）
  - [ ] `GET /api/ranking/level` — 等级排行（从 Cache ZSet `rank:level`）
  - [ ] `GET /api/ranking/combat` — 战力排行（从 Cache ZSet `rank:combat`）
- [ ] **06-10** 编写单元测试
  - [ ] event_executor_test.go：ShowText→ShowChoices→ChangeItems 流程
  - [ ] quest_test.go：接受任务→kill进度更新→完成→奖励发放
  - [ ] trade_test.go：完整交易流程（TestTradeCommit，参见 §5.6 示例）

---

## 实现细节与思路

### 06-1 事件执行器

RMMV 事件指令列表（`list[]`）是线性数组，执行时需要维护一个"程序计数器"（PC）。部分指令有跳转语义（ConditionalBranch 的 else 分支、Loop 等），用 indent 字段来辅助跳转。

```go
type EventExecutor struct {
    session    *PlayerSession
    commands   []resource.EventCommand  // event page 的 list[]
    pc         int                      // 当前执行位置
    waitChan   chan int                  // 等待客户端 choice 输入时阻塞
    ctx        context.Context
    cancel     context.CancelFunc
    questSvc   *quest.Service
    invSvc     *item.InventoryService
    worldMgr   *world.WorldManager
    cache      cache.Cache
}

func (e *EventExecutor) Run() {
    for e.pc < len(e.commands) {
        cmd := e.commands[e.pc]
        switch cmd.Code {
        case 101: e.execShowText(cmd)
        case 102: e.execShowChoices(cmd)
        case 111: e.execConditionalBranch(cmd)
        case 121: e.execChangeSwitch(cmd)
        case 122: e.execChangeVariable(cmd)
        case 125: e.execChangeGold(cmd)
        case 126: e.execChangeItems(cmd)
        case 201: e.execTransferPlayer(cmd)
        case 230: e.execWait(cmd)
        case 301: e.execStartBattle(cmd)
        case 355, 655: e.execScript(cmd)  // → Task 08
        default:
            // 未实现的指令：跳过（保证向前兼容）
        }
        e.pc++
    }
    e.session.Send(packet("npc_dialog_close", nil))
}
```

**ShowChoices 等待**：
```go
func (e *EventExecutor) execShowChoices(cmd resource.EventCommand) {
    choices := cmd.Parameters[0].([]string)
    e.session.Send(packet("npc_dialog", map[string]interface{}{
        "npc_id": 0, "text": "", "choices": choices,
    }))
    // 等待玩家选择（最多 60s）
    select {
    case idx := <-e.waitChan:
        e.jumpToChoice(idx, choices)  // 移动 pc 到对应分支
    case <-time.After(60 * time.Second):
        e.cancel()  // 超时关闭执行器
    case <-e.ctx.Done():
        return
    }
}
```

**ConditionalBranch（code=111）**：
RMMV 的 ConditionalBranch 使用 indent 字段来标记 else 和 end 分支：
- PC 跳转时扫描后续指令，找到相同 indent 的 `402（else）` 或 `412（end）` 指令

### 06-2 事件会话管理

每个玩家同时只能有一个活跃的事件执行器（防止同时触发多个 NPC）：

```go
// PlayerSession 新增字段
type PlayerSession struct {
    ...
    eventExec   *EventExecutor  // nil 表示无活跃事件
    eventMu     sync.Mutex
}

func HandleNPCInteract(ctx context.Context, session *PlayerSession, payload json.RawMessage) error {
    session.eventMu.Lock()
    defer session.eventMu.Unlock()
    if session.eventExec != nil {
        return errors.New("already in event")
    }
    // ... 创建并启动 EventExecutor goroutine ...
}
```

### 06-3 任务系统

**任务目标结构**（从 CommonEvents.json 解析，使用事件注释约定）：
```
// 建议使用事件页第一条 Comment 指令（code=108）存储任务元数据 JSON
// 示例：{"quest":{"objectives":[{"type":"kill","target_id":3,"count":5}],"rewards":{"exp":200,"gold":500}}}
```

**任务触发钩子**（需要 Task 03/04/02 各自在完成动作后调用）：

```go
// 怪物死亡时（在 Task 03 loot.go 调用）
func (svc *QuestService) OnMonsterKill(ctx context.Context, charID int64, monsterID int) {
    // 查该角色进行中的任务，找类型=kill且target_id=monsterID的目标
    // 进度 +1，检查是否完成
}

// 物品进入背包（在 Task 04 inventory.go 调用）
func (svc *QuestService) OnItemGain(ctx context.Context, charID int64, itemID, qty int) {
    // 查进行中的collect类型任务
}

// 玩家进入地图（在 Task 02 HandleEnterMap 调用）
func (svc *QuestService) OnMapEnter(ctx context.Context, charID int64, mapID, x, y int) {
    // 查进行中的goto类型任务（有些任务是到达特定区域）
}
```

### 06-5 交易系统

**分布式锁设计**（防止物品复制）：

```go
func (svc *TradeService) Commit(ctx context.Context, trade *TradeSession) error {
    // 1. 生成锁 key（确保顺序：小ID在前）
    a, b := trade.PlayerA, trade.PlayerB
    if a > b { a, b = b, a }
    lockKey := fmt.Sprintf("lock:trade:%d_%d", a, b)

    // 2. 尝试加锁（SetNX，TTL 30s）
    ok, err := svc.cache.SetNX(ctx, lockKey, "1", 30*time.Second)
    if !ok || err != nil {
        return errors.New("trade in progress, please retry")
    }
    defer svc.cache.Del(ctx, lockKey)

    // 3. 数据库事务
    return svc.db.Transaction(func(tx *gorm.DB) error {
        // 3a. 重新从DB查双方物品（防时间差攻击）
        // 3b. 校验双方实际拥有要交易的物品
        // 3c. 原子移动：A的物品→B背包，B的物品→A背包
        // 3d. 金币原子转移
        return nil
    })
}
```

**LocalCache 限制提示**（§5.5）：单进程时 SetNX 有效；多进程必须配置 Redis。

### 06-6 副本系统

```go
// WorldManager 新增方法
func (wm *WorldManager) CreateInstance(baseMapID int, partyID int64) (*MapRoom, error) {
    instanceID := generateInstanceID(baseMapID, partyID)
    room := NewMapRoom(instanceID, wm.resource.Maps[baseMapID])  // 复制地图数据
    go room.Run()
    wm.mu.Lock()
    wm.rooms[instanceID] = room
    wm.mu.Unlock()
    return room, nil
}

// MapRoom 中玩家数量归零时自动回收
func (room *MapRoom) onPlayerLeave() {
    room.mu.RLock()
    count := len(room.players)
    room.mu.RUnlock()
    if count == 0 && room.IsInstance {
        close(room.stopCh)    // 停止 Tick goroutine
        wm.RemoveRoom(room.MapID)
    }
}
```

---

## 关键 S2C 消息格式

**npc_dialog**：
```json
{
  "type": "npc_dialog",
  "payload": {
    "npc_id": 1,
    "face_name": "Actor1",
    "face_index": 0,
    "text": "欢迎来到小镇！你有什么需要帮助的吗？",
    "choices": ["购买物品", "接受任务", "再见"]
  }
}
```

**quest_update**：
```json
{
  "type": "quest_update",
  "payload": {
    "quest_id": 5,
    "progress": {"kill_slime": 3, "kill_goblin": 0},
    "objectives": [
      {"type": "kill", "target_id": 3, "target_name": "史莱姆", "current": 3, "required": 5},
      {"type": "kill", "target_id": 7, "target_name": "哥布林", "current": 0, "required": 3}
    ],
    "completed": false,
    "rewards": null
  }
}
```

---

## 验收标准

1. 与 NPC 交互触发对话，ShowText/ShowChoices 正确推送，玩家选择后事件继续执行
2. 接受任务 → 击杀目标怪物 → 任务进度正确更新 → 完成时自动发放奖励
3. 任务奖励正确发放（经验/金币/物品）并推送
4. 双人交易成功完成物品互换（DB 事务原子性），无物品复制
5. 一方取消交易时双方正确退出交易状态
6. 副本创建后所有玩家离开时自动销毁
7. 排行榜从 Cache ZSet 正确读取
8. 单元测试全部通过（包括 TestTradeCommit 示例）
