# MakerPGM-MMO 框架设计文档

> 目标：将标准 RPG Maker MV 项目（Project1）改造为 MMORPG，提供开箱即用的 MMO 套件，游戏开发者只需专注于 RPG Maker MV 内的游戏设计，无需关心服务器实现细节。

---

## 目录

1. [总体架构](#1-总体架构)
2. [客户端设计](#2-客户端设计)
3. [服务器端设计](#3-服务器端设计)
4. [通信协议](#4-通信协议)
5. [数据库设计](#5-数据库设计)
6. [资源读取与映射](#6-资源读取与映射)
7. [游戏系统详细设计](#7-游戏系统详细设计)
8. [插件与扩展系统](#8-插件与扩展系统)
9. [调度系统](#9-调度系统)
10. [日志与审计](#10-日志与审计)
11. [调试系统](#11-调试系统)
12. [安全性设计](#12-安全性设计)
13. [部署与运维](#13-部署与运维)

---

## 1. 总体架构

### 1.1 设计原则

- **最小侵入**：客户端以插件形式注入，不修改 RPG Maker MV 原有引擎文件（`rpg_core.js`、`rpg_objects.js` 等）
- **开箱即用**：服务器端直接读取 RPG Maker MV 的 `data/` 目录，自动解析地图、事件、角色、技能等定义
- **服务端权威**：所有游戏逻辑（战斗、移动、技能、物品）在服务端执行，客户端只负责渲染和 UI
- **可扩展性**：服务端提供完善的 Hook 系统和插件热加载机制
- **设计思想**： 所有组件都使用DDD领域涉及原则，遇到问题时候使用TDD测试驱动开发

### 1.2 系统层次图

```
┌──────────────────────────────────────────────────────┐
│                    客户端（浏览器/NW.js）               │
│  ┌─────────────────────────────────────────────────┐  │
│  │         MMO 插件层（plugins/mmo-*.js）            │  │
│  │  登录场景 │ 角色选择 │ HUD │ 聊天 │ 技能栏 │ 小地图 │  │
│  └──────────────────────┬──────────────────────────┘  │
│  ┌───────────────────────▼─────────────────────────┐  │
│  │         RPG Maker MV 引擎层（只读渲染）           │  │
│  │  rpg_core │ rpg_objects │ rpg_scenes │ rpg_windows │  │
│  └──────────────────────┬──────────────────────────┘  │
└─────────────────────────┼────────────────────────────┘
                          │ WebSocket / HTTP REST / SSE
┌─────────────────────────▼────────────────────────────┐
│                    服务器端（Go）                       │
│  ┌─────────────────────────────────────────────────┐  │
│  │                  API 网关层（Gin）                │  │
│  │   WebSocket Handler │ REST Handler │ SSE Handler  │  │
│  └──────────────────────┬──────────────────────────┘  │
│  ┌───────────────────────▼─────────────────────────┐  │
│  │                   游戏逻辑层                      │  │
│  │  战斗 │ 地图 │ AI │ 任务 │ 社交 │ 组队 │ 聊天     │  │
│  │  技能 │ Buff │ 装备 │ 物品 │ NPC │ 怪物 │ 交易    │  │
│  └──────────────────────┬──────────────────────────┘  │
│  ┌───────────────────────▼─────────────────────────┐  │
│  │                  Hook/插件中间层                  │  │
│  └──────────────────────┬──────────────────────────┘  │
│  ┌───────────┬───────────▼──────────┬───────────────┐  │
│  │  DB 适配层 │  Cache/PubSub 适配层 │  调度系统       │  │
│  │嵌入/SQLite/│ Redis / LocalCache   │  定时/事件任务  │  │
│  │  MySQL    │  会话/缓存/消息推送   │                │  │
│  └───────────┴──────────────────────┴───────────────┘  │
└──────────────────────────────────────────────────────┘
```

### 1.3 技术栈

| 层次 | 技术选型 | 说明 |
|------|---------|------|
| 客户端渲染 | RPG Maker MV + PIXI.js | 原有引擎不变 |
| 客户端插件 | JavaScript (ES5) | 兼容 RPG Maker MV 插件规范 |
| 服务端框架 | Go + Gin + GORM v2 | 高并发，低延迟，ORM 简化数据库操作 |
| 实时通信 | WebSocket | 游戏状态双向实时同步 |
| 推送通知 | SSE (Server-Sent Events) | 服务端单向推送（公告、邮件） |
| REST API | HTTP + JSON | 非实时操作（登录、商城等） |
| 持久化数据库 | sqlexec 内嵌（默认）/ SQLite / MySQL 8.0 | 多模式按环境切换；内嵌模式零外部依赖 |
| 缓存/会话/Pub-Sub | Redis 7.x（可选）/ 内置 LocalCache | Redis 不可用时自动降级为单进程内存实现 |
| JS 沙箱 | goja（纯 Go ES5.1+） | RMMV Script 命令服务端执行、自定义 JS 逻辑 |
| 服务端插件 | Go Plugin / Yaegi | 服务端 Go 插件热加载 |

---

## 2. 客户端设计

### 2.1 插件注入原则

客户端所有改造通过 RPG Maker MV 标准插件机制注入，无需修改引擎核心文件。插件列表：

```
plugins/
├── mmo-core.js          # 核心：WebSocket 连接管理、消息分发、状态机
├── mmo-auth.js          # 登录/注册/角色选择场景替换
├── mmo-hud.js           # HUD：HP/MP/EXP 条、小地图、任务追踪
├── mmo-chat.js          # 聊天框（全服/队伍/部队/战斗频道）
├── mmo-skill-bar.js     # 技能栏（12 格 + F1-F12 热键）
├── mmo-battle.js        # 即时战斗：替换踩地雷模式
├── mmo-party.js         # 组队系统 HUD
├── mmo-inventory.js     # 背包/装备/物品 UI
├── mmo-social.js        # 好友/公会/社交 UI
├── mmo-trade.js         # 交易系统 UI
└── mmo-other-players.js # 其他玩家角色同步渲染
```

### 2.2 场景改造

#### 2.2.1 标题场景 → 登录场景

```
原：Scene_Title（开始游戏 / 继续游戏 / 关于）
改：Scene_Login
    ├── 用户名输入框
    ├── 密码输入框（密文显示）
    ├── 登录按钮（用户名不存在时自动注册）
    └── 错误提示区域
```

- 登录成功后跳转到 `Scene_CharacterSelect`
- Token 存储于内存（不写入本地存档，防止盗号）

#### 2.2.2 新增：角色选择场景

```
Scene_CharacterSelect
    ├── 角色卡片列表（最多 3 个角色槽位）
    │   ├── 角色立绘/行走图预览
    │   ├── 角色名称、职业、等级
    │   └── 上次登录时间
    ├── 新建角色按钮 → Scene_CharacterCreate
    ├── 删除角色按钮（需二次确认）
    └── 进入游戏按钮
```

#### 2.2.3 新增：角色创建场景

```
Scene_CharacterCreate
    ├── 外观编辑器
    │   ├── 行走图选择（使用项目 img/characters/ 素材）
    │   ├── 脸图选择（使用项目 img/faces/ 素材）
    │   └── 职业选择（来自 data/Classes.json）
    ├── 角色名称输入框（唯一性由服务端校验）
    ├── 属性点分配（可选，由服务端规则决定是否开放）
    └── 确认创建按钮
```

#### 2.2.4 地图场景 HUD 布局

进入游戏地图后（`Scene_Map`），MMO 插件叠加以下 UI 层：

```
┌────────────────────────────────────────────────────────────┐
│                                          [HP/MP/EXP 条]    │ ← 右上角
│                                          [小地图]           │ ← 右上角（HP条下方）
│                                                            │
│                      游戏地图渲染区域                        │
│               （其他玩家 / NPC / 怪物同步）                   │
│                                                            │
│  [组队面板]     [任务追踪]                                   │ ← 左中 / 右中
│                                                            │
│  [聊天框+输入框]    [技能栏 F1-F12（12格）]  [功能按钮 2×3]  │ ← 底部
└────────────────────────────────────────────────────────────┘
```

**功能按钮（右下 2行×3列）**：

| 位置 | 按钮 | 快捷键 |
|------|------|--------|
| [1,1] | 玩家信息 | Alt+C |
| [1,2] | 部队/公会 | Alt+G |
| [1,3] | 背包 | Alt+I |
| [2,1] | 技能列表 | Alt+K |
| [2,2] | 社交/好友 | Alt+F |
| [2,3] | 任务日志 | Alt+Q |

**技能栏**（中下，12 格）：

- F1-F12 对应 12 个技能快捷槽
- 鼠标拖拽技能列表中的技能到格子中绑定
- 格子显示：技能图标、CD 倒计时遮罩、MP 不足时变灰

**聊天框**（左下）支持以下频道：

| 频道 | 标识 | 颜色 | 可见范围 |
|------|------|------|---------|
| 全服 | [全] | 白色 | 全服所有玩家 |
| 队伍 | [队] | 绿色 | 同队玩家 |
| 部队/公会 | [会] | 黄色 | 同公会玩家 |
| 战斗 | [战] | 红色 | 当前战斗的伤害/技能日志 |
| 系统 | [系] | 蓝色 | 系统通知（只读） |
| 私聊 | [私] | 紫色 | 点对点私信 |

**小地图**（右上）：

- 渲染当前地图的通行性遮罩（来自 `Map*.json` 的 passability 数据）
- 标注当前 NPC/怪物位置（红点）
- 标注其他玩家位置（蓝点）
- 标注当前追踪任务的目标位置（金色星形）

**任务追踪**（右中）：

- 显示当前正在追踪的任务名称及各目标完成进度
- 在任务日志菜单中可选择哪些任务显示在此处（最多 3 个）

**组队面板**（左中，组队后显示）：

- 每个队员一行：昵称、职业图标、HP 条、MP 条、当前 Buff 图标列表

#### 2.2.5 进入地图的无敌保护流程

```
客户端                          服务器
  │                               │
  │──── WS: enter_map(mapId) ────►│
  │                               │ 冻结玩家状态，标记为保护中
  │◄─── WS: map_init(全地图状态) ──│
  │  [渲染地图 + 其他玩家，本人无敌] │
  │                               │ 完成地图数据推送
  │◄─── WS: protection_end ───────│
  │  [解除无敌，可以移动/攻击]       │
```

保护期间：NPC/其他玩家无法对其发起攻击，自己也无法移动或使用技能。

### 2.3 即时战斗模式

原 RPG Maker MV 的踩地雷进入回合制战斗改为：

```
地图上即时战斗流程：

① 玩家进入怪物感知范围（服务端检测）
② 服务端通知客户端怪物进入追击状态
③ 玩家按普通攻击键 / 技能键
④ 客户端发送攻击请求（含目标 ID）到服务端
⑤ 服务端校验合法性（距离、CD、状态）
⑥ 服务端计算伤害/技能效果（基于 Skills.json 公式）
⑦ 广播战斗结果给地图内周围玩家
⑧ 客户端播放攻击动画（来自 Animations.json 的 animationId）+ 显示伤害飘字
⑨ 怪物 HP 归零 → 服务端计算掉落/经验/任务进度，推送给相关玩家
```

客户端只负责播放动画效果，所有数值由服务端权威计算，客户端无法伪造。

### 2.4 其他玩家渲染

- 使用 `Sprite_Character` 子类 `Sprite_OtherPlayer` 渲染其他玩家
- 通过 WebSocket 接收位置增量同步，客户端做插值平滑（线性插值）
- 玩家头上显示：昵称 + 职业图标 + 等级 + 公会名（可配置显示层数）
- 不同状态用头名颜色区分：普通（白）、组队中（绿）、PK 模式（红）、GM（金）

### 2.5 本地存档禁用

MMO 模式下，原 RPG Maker MV 的本地存档（`localStorage`）功能完全禁用，角色数据全部由服务端管理。`DataManager.saveGame` 和 `DataManager.loadGame` 被插件替换为空操作。

---

## 3. 服务器端设计

### 3.1 目录结构

```
server/
├── main.go                  # 入口，初始化所有子系统
├── config/
│   └── config.yaml          # 服务配置（端口、DB、Redis、游戏参数）
├── api/
│   ├── ws/                  # WebSocket 消息处理器
│   ├── rest/                # REST API 路由（Gin）
│   └── sse/                 # SSE 推送端点
├── game/
│   ├── world/               # 世界管理（地图 Room 实例管理）
│   ├── player/              # 玩家会话管理
│   ├── battle/              # 即时战斗系统
│   ├── ai/                  # AI 行为树/状态机（NPC/怪物）
│   ├── quest/               # 任务系统
│   ├── skill/               # 技能/Buff 系统
│   ├── item/                # 物品/装备系统
│   ├── party/               # 组队系统
│   ├── guild/               # 公会系统
│   ├── social/              # 好友/黑名单
│   ├── chat/                # 聊天系统（多频道）
│   ├── trade/               # 玩家交易系统
│   └── script/              # goja JS 沙箱（VM 池、RMMV 上下文 Mock）
├── resource/
│   └── loader.go            # RPG Maker MV data/ 目录读取器
├── plugin/
│   ├── manager.go           # 插件热加载管理器
│   └── hook/                # Hook 注册中心
├── scheduler/               # 调度系统
├── audit/                   # 审计日志写入
├── debug/                   # Chrome DevTools 调试适配层
├── middleware/              # Gin 中间件（认证、限流、TraceID 注入、日志）
├── model/                   # 数据库模型（GORM Model struct + AutoMigrate）
├── db/
│   ├── adapter.go           # Open() 工厂：按 mode 返回 *gorm.DB
│   ├── embedded/            # sqlexec 内嵌适配（XML 引擎 / Memory 引擎）
│   ├── sqlite/              # SQLite 适配（GORM + modernc.org/sqlite，纯 Go）
│   └── mysql/               # MySQL 适配（GORM + go-sql-driver/mysql）
└── cache/
    ├── adapter.go           # Cache / PubSub 接口定义 + 工厂函数
    ├── redis/               # Redis 实现（go-redis/v9）
    └── local/               # 本地内存实现（单进程，无外部依赖）
```

### 3.2 WebSocket 连接管理

每个在线玩家对应一个 `PlayerSession`：

```go
type PlayerSession struct {
    PlayerID  int64
    AccountID int64
    Conn      *websocket.Conn
    MapID     int
    SendChan  chan Packet      // 发送队列（独立 goroutine 异步写，避免并发写 WS）
    Done      chan struct{}    // 关闭信号
    TraceID   string          // 当前请求的 trace-id（每条消息重新生成）
}
```

- **发送**：独立 goroutine 消费 `SendChan`，串行写入 WebSocket，避免并发写
- **心跳**：客户端每 30s 发 `ping`，服务端超 60s 未收到则主动断开并触发下线逻辑
- **断线重连**：客户端有指数退避重连机制，服务端 60s 内重连可恢复会话

### 3.3 消息路由

所有 WebSocket 消息统一格式：

```json
{
  "seq":     12345,
  "type":    "player_move",
  "payload": { "x": 10, "y": 5, "dir": 4 }
}
```

服务端 Handler 注册（路由表模式）：

```go
router.On("enter_map",    world.HandleEnterMap)
router.On("player_move",  battle.HandleMove)
router.On("player_attack",battle.HandleAttack)
router.On("player_skill", skill.HandleUseSkill)
router.On("player_item",  item.HandleUseItem)
router.On("chat_send",    chat.HandleSend)
router.On("party_invite", party.HandleInvite)
router.On("trade_request",trade.HandleRequest)
router.On("npc_interact", world.HandleNPCInteract)
router.On("quest_accept", quest.HandleAccept)
router.On("ping",         session.HandlePing)
```

每个 Handler 执行前后经过 Hook 中间件，Hook 可修改参数或中断流程。

### 3.4 地图实例（MapRoom）

每张地图是一个独立的 `MapRoom`，由独立 goroutine 驱动游戏帧（20 TPS，50ms/帧）：

```
MapRoom (goroutine)
  ├── players    map[int64]*PlayerSession   在此地图的所有玩家
  ├── npcs       []*NPCInstance             NPC 实例
  ├── monsters   []*MonsterInstance         怪物实例
  ├── broadcastQ chan Packet                广播队列
  └── Tick() 每 50ms
       ├── 处理本帧所有移动请求（校验通行性）
       ├── 处理本帧所有攻击/技能请求
       ├── 更新所有怪物/NPC 的 AI 行为树
       ├── 处理 Buff/DOT Tick（伤害、持续效果）
       ├── 检查怪物刷新计时器
       └── 广播本帧增量状态快照给所有玩家
```

副本地图每次创建独立 `MapRoom` 实例，全部玩家离开后自动回收。

---

## 4. 通信协议

### 4.1 协议分类

| 场景 | 协议 | 原因 |
|------|------|------|
| 登录/注册/角色管理/背包查询 | HTTP REST | 无需实时，操作频率低 |
| 游戏状态同步（移动/战斗/技能/聊天） | WebSocket | 双向实时，低延迟 |
| 服务端主动通知（公告/邮件/系统事件） | SSE | 服务端单向推送，无需双向通道 |
| 静态资源（图片/音频） | HTTP | 标准静态文件服务 |

### 4.2 客户端 → 服务端 消息类型（C2S）

| type | 说明 | 关键 payload 字段 |
|------|------|-----------------|
| `enter_map` | 进入地图 | `map_id`, `char_id` |
| `player_move` | 移动 | `x`, `y`, `dir`, `seq` |
| `player_attack` | 普通攻击 | `target_id`, `target_type` |
| `player_skill` | 使用技能 | `skill_id`, `target_id`, `x`, `y` |
| `player_item` | 使用物品 | `item_id`, `target_id` |
| `chat_send` | 发送聊天 | `channel`, `content`, `target_id?` |
| `party_invite` | 邀请组队 | `target_player_id` |
| `party_leave` | 离开队伍 | — |
| `party_kick` | 踢出队员 | `target_player_id` |
| `trade_request` | 发起交易 | `target_player_id` |
| `trade_offer` | 放入交易物 | `items[]`, `gold` |
| `trade_confirm` | 确认交易 | — |
| `trade_cancel` | 取消交易 | — |
| `npc_interact` | 与 NPC 交互 | `npc_id`, `event_id` |
| `npc_choice` | 事件选项选择 | `choice_index` |
| `quest_accept` | 接受任务 | `quest_id` |
| `quest_abandon` | 放弃任务 | `quest_id` |
| `quest_track` | 设置追踪任务 | `quest_ids[]` |
| `equip_item` | 装备/卸下 | `item_id`, `equip_slot` |
| `drop_item` | 丢弃物品 | `item_id`, `quantity` |
| `pickup_item` | 拾取掉落物 | `drop_id` |
| `ping` | 心跳 | `ts` |

### 4.3 服务端 → 客户端 消息类型（S2C）

| type | 说明 | 关键 payload 字段 |
|------|------|-----------------|
| `map_init` | 地图初始化全量数据 | `players[]`, `npcs[]`, `monsters[]`, `drops[]` |
| `protection_end` | 解除进场无敌 | — |
| `player_sync` | 玩家位置/状态增量 | `player_id`, `x`, `y`, `dir`, `hp`, `mp`, `state` |
| `battle_result` | 战斗结果 | `attacker_id`, `target_id`, `damage`, `effects[]`, `target_hp` |
| `skill_effect` | 技能效果 | `caster_id`, `skill_id`, `targets[]`, `animation_id` |
| `buff_update` | Buff 施加/消除 | `target_id`, `buff_id`, `stacks`, `duration`, `action` |
| `player_death` | 玩家死亡 | `player_id`, `killer_id`, `killer_name` |
| `player_revive` | 玩家复活 | `player_id`, `map_id`, `x`, `y`, `hp`, `mp` |
| `monster_spawn` | 怪物刷新 | `inst_id`, `monster_id`, `x`, `y`, `hp`, `max_hp` |
| `monster_sync` | 怪物移动/状态 | `inst_id`, `x`, `y`, `dir`, `hp`, `state` |
| `monster_death` | 怪物死亡 | `inst_id`, `drops[]`, `exp` |
| `drop_spawn` | 地图掉落物出现 | `drop_id`, `item_id`, `x`, `y` |
| `drop_remove` | 地图掉落物消失 | `drop_id` |
| `player_join` | 其他玩家进入视野 | `player_data{id,name,class,level,x,y,dir,hp,mp,buffs[]}` |
| `player_leave` | 其他玩家离开视野 | `player_id` |
| `npc_dialog` | NPC 对话内容 | `npc_id`, `face_name`, `face_index`, `text`, `choices[]?` |
| `npc_dialog_close` | 关闭对话框 | — |
| `chat_recv` | 收到聊天消息 | `channel`, `from_name`, `content`, `ts` |
| `party_update` | 队伍状态全量刷新 | `party_id`, `leader_id`, `members[]` |
| `quest_update` | 任务进度更新 | `quest_id`, `progress{}`, `completed`, `rewards?` |
| `inventory_update` | 背包变化 | `add[]`, `remove[]`, `update[]` |
| `exp_gain` | 获得经验 | `exp`, `total_exp`, `level_up?`, `new_level?` |
| `gold_update` | 金币变化 | `delta`, `total` |
| `system_notice` | 系统公告 | `content`, `level(info|warn|event)` |
| `trade_update` | 交易界面状态 | `phase`, `my_offer`, `their_offer`, `confirmed` |
| `equip_result` | 装备操作结果 | `success`, `char_stats` |
| `pong` | 心跳回应 | `client_ts`, `server_ts` |

### 4.4 REST API 端点

```
# 认证
POST   /api/auth/login              登录（用户名不存在时自动注册）
POST   /api/auth/logout             登出（使 Token 失效）
POST   /api/auth/refresh            刷新 JWT Token

# 角色管理
GET    /api/characters              获取账号下角色列表（含外观、等级）
POST   /api/characters              创建角色（校验名字唯一、素材合法性）
DELETE /api/characters/:id          删除角色（需密码二次确认）

# 背包（非实时查询，WebSocket 负责实时变化推送）
GET    /api/characters/:id/inventory  获取完整背包列表

# 商城
GET    /api/shop/items              获取商城商品列表
POST   /api/shop/buy                购买商品

# 排行榜
GET    /api/ranking/level           等级排行榜（Top 100）
GET    /api/ranking/combat          战力排行榜（Top 100）

# 公会
GET    /api/guilds/:id              公会详情
POST   /api/guilds                  创建公会
POST   /api/guilds/:id/join         申请加入公会
DELETE /api/guilds/:id/members/:cid 踢出成员（会长/副会长操作）

# 邮件
GET    /api/mail                    邮件列表
POST   /api/mail/:id/claim          领取邮件附件
DELETE /api/mail/:id                删除邮件
```

---

## 5. 数据库设计

### 5.0 多模式数据库架构

框架同时支持四种数据库运行模式，通过配置文件 `database.mode` 字段切换，业务代码完全不感知底层存储引擎：

| 模式 | 配置值 | 存储后端 | 适用场景 |
|------|-------|---------|---------|
| **内嵌 XML**（默认） | `embedded_xml` | `kasuganosora/sqlexec` + XML 文件 | 生产环境零外部依赖部署 |
| **内嵌内存** | `embedded_memory` | `kasuganosora/sqlexec` + Memory | 单元测试、CI/CD 流水线 |
| **SQLite** | `sqlite` | SQLite 3（纯 Go 驱动） | 本机开发调试、小规模单机部署 |
| **MySQL** | `mysql` | MySQL 8.0 | 大规模生产、高并发写入 |

> **sqlexec**（`github.com/kasuganosora/sqlexec`）是一个内嵌式 MySQL 协议兼容数据库，支持 XML 持久化引擎和 Memory 引擎，并提供完整 GORM Dialector，使得 MySQL 模式与内嵌模式在 GORM 层完全互换，无需任何代码修改。

### 5.1 DB 适配层

所有模式通过统一的 `*gorm.DB` 实例访问，由工厂函数 `db.Open()` 根据配置创建：

```go
// db/adapter.go

type DBMode string

const (
    ModeEmbeddedXML    DBMode = "embedded_xml"    // 内嵌 XML 持久化（生产默认）
    ModeEmbeddedMemory DBMode = "embedded_memory" // 内嵌内存（单元测试）
    ModeSQLite         DBMode = "sqlite"
    ModeMySQL          DBMode = "mysql"
)

type DatabaseConfig struct {
    Mode         DBMode
    EmbeddedPath string // embedded_xml 模式：XML 文件存储目录
    SQLitePath   string // sqlite 模式：.db 文件路径
    DSN          string // mysql 模式：连接字符串
    MaxOpenConns int
    MaxIdleConns int
    ConnMaxLifetime time.Duration
}

// Open 根据配置返回 GORM DB 实例，调用方无需关心底层引擎
func Open(cfg DatabaseConfig) (*gorm.DB, error) {
    switch cfg.Mode {
    case ModeEmbeddedXML:
        return embedded.Open(cfg.EmbeddedPath, embedded.EngineXML)
    case ModeEmbeddedMemory:
        return embedded.Open(":memory:", embedded.EngineMemory)
    case ModeSQLite:
        return sqlite.Open(cfg.SQLitePath)
    case ModeMySQL:
        return mysql.Open(cfg.DSN, cfg.MaxOpenConns, cfg.MaxIdleConns, cfg.ConnMaxLifetime)
    default:
        return embedded.Open(cfg.EmbeddedPath, embedded.EngineXML)
    }
}
```

**sqlexec 内嵌适配器**（`db/embedded/driver.go`）：

```go
import (
    sqlexecapi  "github.com/kasuganosora/sqlexec/pkg/api"
    sqlexecgorm "github.com/kasuganosora/sqlexec/pkg/gorm" // GORM Dialector
)

type EngineType int
const (
    EngineXML    EngineType = iota // XML 文件持久化
    EngineMemory                    // 纯内存（测试用）
)

func Open(dataPath string, eng EngineType) (*gorm.DB, error) {
    opts := &sqlexecapi.Options{DataPath: dataPath}
    if eng == EngineMemory {
        opts.DataPath = ""
        opts.InMemory  = true
    }
    instance := sqlexecapi.NewDB(opts)
    return gorm.Open(sqlexecgorm.Open(instance), &gorm.Config{
        Logger: logger.Default.LogMode(logger.Warn),
    })
}
```

**SQLite 适配器**（`db/sqlite/driver.go`，使用 `modernc.org/sqlite` 纯 Go 驱动，无 CGO）：

```go
import "gorm.io/driver/sqlite"

func Open(path string) (*gorm.DB, error) {
    return gorm.Open(sqlite.Open(path), &gorm.Config{})
}
```

服务启动时自动执行 GORM AutoMigrate，所有模式统一建表：

```go
// model/migrate.go
func AutoMigrate(db *gorm.DB) error {
    return db.AutoMigrate(
        &Account{}, &Character{}, &Inventory{}, &CharSkill{},
        &QuestProgress{}, &Friendship{}, &Guild{}, &GuildMember{},
        &Mail{}, &AuditLog{},
    )
}
```

### 5.2 跨引擎兼容说明

各引擎对 SQL 特性的支持差异及处理策略：

| SQL 特性 | sqlexec（XML/Memory） | SQLite | MySQL | 处理策略 |
|---------|---------------------|--------|-------|---------|
| `AUTO_INCREMENT` | ✅ | ✅ | ✅ | GORM 自动处理方言差异 |
| `JSON` 列类型 | ✅（序列化文本） | ✅（文本存储） | ✅（原生） | 统一用 `datatypes.JSON` 序列化 |
| `UNIQUE KEY` | ✅ | ✅ | ✅ | GORM tag `uniqueIndex` |
| `PARTITION BY RANGE` | ❌ | ❌ | ✅ | 仅 MySQL 模式启用；非 MySQL 模式定期按 `created_at` 物理删除旧审计日志 |
| `ON UPDATE NOW()` | ✅ | 部分 | ✅ | 用 GORM `autoUpdateTime` tag 替代 |
| 全文索引 | ✅（sqlexec 内置） | ❌ | ✅ | 聊天搜索在非 MySQL 模式下降级为 `LIKE` 查询 |
| 外键约束 | ❌（逻辑维护） | ✅（需手动开启） | ✅ | 业务层保证一致性，不依赖数据库外键 |

**审计日志分区策略（按模式）**：

```
MySQL 模式：
  audit_logs 表使用 PARTITION BY RANGE(MONTH(created_at)) 分区，共 12 个分区
  调度任务 archive_audit_logs（每月 1 日 2:00）将过期分区导出到 JSON 文件后 DROP

非 MySQL 模式（sqlexec/SQLite）：
  audit_logs 无分区，调度任务 cleanup_audit_logs（每日 3:00）
  DELETE FROM audit_logs WHERE created_at < NOW() - INTERVAL '90 days'
```

### 5.3 核心表 Schema（DDL 参考）

实际由 GORM AutoMigrate 管理，DDL 仅供参考：

```sql
-- 账号表
CREATE TABLE accounts (
    id            BIGINT PRIMARY KEY AUTO_INCREMENT,
    username      VARCHAR(32) UNIQUE NOT NULL,
    password_hash VARCHAR(64) NOT NULL,        -- bcrypt(cost=12)
    email         VARCHAR(128),
    status        TINYINT DEFAULT 1,           -- 0:封禁 1:正常
    created_at    DATETIME DEFAULT NOW(),
    last_login_at DATETIME,
    last_login_ip VARCHAR(45)
);

-- 角色表
CREATE TABLE characters (
    id            BIGINT PRIMARY KEY AUTO_INCREMENT,
    account_id    BIGINT NOT NULL,
    name          VARCHAR(32) UNIQUE NOT NULL,
    class_id      INT NOT NULL,                -- 对应 data/Classes.json ID
    walk_name     VARCHAR(64),                 -- img/characters/ 文件名
    walk_index    INT DEFAULT 0,               -- 行走图格子索引
    face_name     VARCHAR(64),                 -- img/faces/ 文件名
    face_index    INT DEFAULT 0,
    level         INT DEFAULT 1,
    exp           BIGINT DEFAULT 0,
    hp            INT NOT NULL,
    mp            INT NOT NULL,
    max_hp        INT NOT NULL,
    max_mp        INT NOT NULL,
    atk           INT DEFAULT 10,
    def           INT DEFAULT 5,
    mat           INT DEFAULT 10,
    mdf           INT DEFAULT 5,
    agi           INT DEFAULT 10,
    luk           INT DEFAULT 10,
    gold          BIGINT DEFAULT 0,
    map_id        INT DEFAULT 1,
    map_x         INT DEFAULT 5,
    map_y         INT DEFAULT 5,
    direction     TINYINT DEFAULT 2,           -- RPG Maker 标准：2下 4左 6右 8上
    party_id      BIGINT DEFAULT NULL,
    guild_id      BIGINT DEFAULT NULL,
    created_at    DATETIME DEFAULT NOW(),
    updated_at    DATETIME DEFAULT NOW() ON UPDATE NOW(),
    INDEX idx_account (account_id)
);

-- 物品/背包表
CREATE TABLE inventories (
    id           BIGINT PRIMARY KEY AUTO_INCREMENT,
    char_id      BIGINT NOT NULL,
    slot         INT NOT NULL,
    item_type    TINYINT NOT NULL,             -- 1:物品 2:武器 3:防具
    item_id      INT NOT NULL,                 -- 对应 data/Items|Weapons|Armors.json ID
    quantity     INT DEFAULT 1,
    equip_slot   TINYINT DEFAULT 0,            -- 0:未装备 1-12:装备部位（对应 RMMV 装备槽）
    UNIQUE KEY uk_char_slot (char_id, slot),
    INDEX idx_char (char_id)
);

-- 技能表（已习得技能 + 快捷栏绑定）
CREATE TABLE char_skills (
    id        BIGINT PRIMARY KEY AUTO_INCREMENT,
    char_id   BIGINT NOT NULL,
    skill_id  INT NOT NULL,                    -- 对应 data/Skills.json ID
    level     INT DEFAULT 1,
    shortcut  TINYINT DEFAULT 0,               -- 0:未绑定 1-12:快捷栏槽位
    INDEX idx_char (char_id),
    UNIQUE KEY uk_char_skill (char_id, skill_id)
);

-- 任务进度表
CREATE TABLE quest_progress (
    id           BIGINT PRIMARY KEY AUTO_INCREMENT,
    char_id      BIGINT NOT NULL,
    quest_id     INT NOT NULL,                 -- 对应 CommonEvents.json ID
    status       TINYINT DEFAULT 0,            -- 0:进行中 1:完成 2:失败
    progress     JSON,                         -- {"kill_slime":3,"collect_herb":1}
    accepted_at  DATETIME DEFAULT NOW(),
    completed_at DATETIME,
    UNIQUE KEY uk_char_quest (char_id, quest_id),
    INDEX idx_char (char_id)
);

-- 好友关系表
CREATE TABLE friendships (
    id         BIGINT PRIMARY KEY AUTO_INCREMENT,
    char_id    BIGINT NOT NULL,
    friend_id  BIGINT NOT NULL,
    status     TINYINT DEFAULT 0,              -- 0:申请中 1:好友 2:黑名单
    created_at DATETIME DEFAULT NOW(),
    UNIQUE KEY uk_friendship (char_id, friend_id)
);

-- 公会表
CREATE TABLE guilds (
    id         BIGINT PRIMARY KEY AUTO_INCREMENT,
    name       VARCHAR(32) UNIQUE NOT NULL,
    leader_id  BIGINT NOT NULL,
    notice     TEXT,
    gold       BIGINT DEFAULT 0,
    level      INT DEFAULT 1,
    created_at DATETIME DEFAULT NOW()
);

-- 公会成员表
CREATE TABLE guild_members (
    guild_id  BIGINT NOT NULL,
    char_id   BIGINT NOT NULL,
    rank      TINYINT DEFAULT 3,               -- 1:会长 2:副会长 3:成员
    joined_at DATETIME DEFAULT NOW(),
    PRIMARY KEY (guild_id, char_id),
    INDEX idx_char (char_id)
);

-- 邮件表
CREATE TABLE mails (
    id          BIGINT PRIMARY KEY AUTO_INCREMENT,
    to_char_id  BIGINT NOT NULL,
    from_name   VARCHAR(32) DEFAULT '系统',
    subject     VARCHAR(64),
    body        TEXT,
    attachment  JSON,                          -- [{type,item_id,qty},{gold:100}]
    claimed     TINYINT DEFAULT 0,
    created_at  DATETIME DEFAULT NOW(),
    expire_at   DATETIME,
    INDEX idx_to (to_char_id)
);

-- 审计日志表（高写入，可考虑按月分区）
CREATE TABLE audit_logs (
    id          BIGINT PRIMARY KEY AUTO_INCREMENT,
    trace_id    VARCHAR(36) NOT NULL,
    char_id     BIGINT,
    account_id  BIGINT,
    char_name   VARCHAR(32),
    action      VARCHAR(64) NOT NULL,
    request     JSON,
    response    JSON,
    error       TEXT,
    ip          VARCHAR(45),
    map_id      INT,
    duration_ms INT,
    created_at  DATETIME(3) DEFAULT NOW(3),
    INDEX idx_char (char_id),
    INDEX idx_trace (trace_id),
    INDEX idx_created (created_at)
) PARTITION BY RANGE (MONTH(created_at)) (
    PARTITION p1 VALUES LESS THAN (2),
    PARTITION p2 VALUES LESS THAN (3),
    -- ... 按月自动分区
    PARTITION p12 VALUES LESS THAN (13)
);
```

### 5.4 Cache/PubSub 适配接口

当配置文件中 `cache.redis_addr` 为空时，框架自动使用内置 `LocalCache`；配置了 Redis 地址则使用 Redis。两种实现遵循同一接口，业务代码无需改动：

```go
// cache/adapter.go

// Cache 接口：KV、Hash、Set、ZSet、List + 原子锁操作
type Cache interface {
    // KV
    Get(ctx context.Context, key string) (string, error)
    Set(ctx context.Context, key string, value string, ttl time.Duration) error
    Del(ctx context.Context, keys ...string) error
    Exists(ctx context.Context, key string) (bool, error)
    // 原子 Set-if-Not-Exists（分布式锁）
    SetNX(ctx context.Context, key string, value string, ttl time.Duration) (bool, error)
    Expire(ctx context.Context, key string, ttl time.Duration) error
    // Hash
    HSet(ctx context.Context, key, field, value string) error
    HGet(ctx context.Context, key, field string) (string, error)
    HGetAll(ctx context.Context, key string) (map[string]string, error)
    HDel(ctx context.Context, key string, fields ...string) error
    // Set
    SAdd(ctx context.Context, key string, members ...string) error
    SRem(ctx context.Context, key string, members ...string) error
    SMembers(ctx context.Context, key string) ([]string, error)
    SIsMember(ctx context.Context, key, member string) (bool, error)
    // ZSet（排行榜）
    ZAdd(ctx context.Context, key string, score float64, member string) error
    ZRevRange(ctx context.Context, key string, start, stop int64) ([]string, error)
    ZScore(ctx context.Context, key, member string) (float64, error)
    // List（聊天历史等）
    LPush(ctx context.Context, key string, values ...string) error
    LRange(ctx context.Context, key string, start, stop int64) ([]string, error)
    LTrim(ctx context.Context, key string, start, stop int64) error
}

// PubSub 接口：频道订阅/发布（玩家同步、跨 goroutine 广播）
type PubSub interface {
    Publish(ctx context.Context, channel, message string) error
    // Subscribe 返回消息 channel + 取消订阅函数
    Subscribe(ctx context.Context, channels ...string) (<-chan *Message, func(), error)
}

type Message struct {
    Channel string
    Payload string
}

// 工厂函数：redis_addr 非空则 Redis，否则 LocalCache
func NewCache(cfg CacheConfig) (Cache, error) {
    if cfg.RedisAddr != "" {
        return redis.New(cfg)
    }
    return local.NewCache(cfg)
}

func NewPubSub(cfg CacheConfig) (PubSub, error) {
    if cfg.RedisAddr != "" {
        return redis.NewPubSub(cfg)
    }
    return local.NewPubSub(cfg)
}
```

### 5.5 LocalCache 本地实现（无 Redis 降级方案）

`LocalCache` 为纯 Go 实现，适合单进程开发/测试场景，无任何外部依赖：

```
LocalCache 内部结构：
  KV/Expire：sync.Map 存储，value 封装 {data, expireAt}
             后台 GC goroutine 每 30s 扫描清理过期 key

  Hash：     sync.Map[key] → sync.Map[field → value]

  Set：      sync.Map[key] → map[string]struct{} + sync.RWMutex

  ZSet：     sync.Map[key] → 内存有序列表 []ZEntry{member, score}
             ZAdd 时 O(log n) 插入排序；ZRevRange 直接切片返回

  List：     sync.Map[key] → []string（头插）+ sync.Mutex
             LTrim 保持最大长度，超出时删除尾部

  SetNX：    使用 sync.Mutex + 原子比较实现进程内互斥锁

LocalPubSub 内部结构：
  subscribers sync.Map[channel] → []chan *Message（fan-out）
  Publish     遍历订阅者，非阻塞投递（chan 默认 buffer=256），
              投递失败（chan 满）记录 warn 日志后丢弃
  Subscribe   创建专属 chan，注册到 channel 订阅列表
  Unsubscribe 从列表移除，关闭专属 chan，触发调用方退出读循环
```

**⚠️ LocalCache/LocalPubSub 限制（必须阅读）**：

| 限制 | 影响 | 应对方案 |
|------|------|---------|
| SetNX 锁仅单进程内有效 | 多进程/多机部署物品复制风险 | 多进程部署必须配置 Redis |
| PubSub 消息不跨进程 | 地图 Room 分在不同进程时广播丢失 | 同上 |
| 内存无持久化 | 服务重启后缓存清空 | 重启后从 DB 重建（自动执行） |
| ZSet 排行榜 | 重启后重建，期间数据空 | 调度任务 `ranking_rebuild` 在启动时立即执行 |
| 仅适合 < 500 并发 | 并发高时 sync.Mutex 争用 | 高并发必须使用 Redis |

### 5.6 单元测试配置

内嵌内存模式 + LocalCache 组合使单元测试**零外部依赖**、毫秒级启动：

```go
// testutil/setup.go

func SetupTestDB(t *testing.T) *gorm.DB {
    db, err := dbadapter.Open(dbadapter.DatabaseConfig{
        Mode: dbadapter.ModeEmbeddedMemory,
    })
    require.NoError(t, err)
    require.NoError(t, model.AutoMigrate(db))
    // sqlexec memory 实例随 GC 自动释放，无需显式 Close
    return db
}

func SetupTestCache(t *testing.T) (cache.Cache, cache.PubSub) {
    cfg := cache.CacheConfig{} // RedisAddr 为空 → 自动使用 LocalCache
    c, _  := cache.NewCache(cfg)
    ps, _ := cache.NewPubSub(cfg)
    return c, ps
}

// 示例：交易系统测试，无需 Docker 或任何外部服务
func TestTradeCommit(t *testing.T) {
    db       := SetupTestDB(t)
    c, _     := SetupTestCache(t)
    svc      := trade.NewService(db, c)

    // 准备角色和物品数据
    charA := &model.Character{Name: "Alice", Gold: 1000}
    charB := &model.Character{Name: "Bob",   Gold: 500}
    db.Create(charA); db.Create(charB)

    session := &trade.Session{
        PlayerA: charA.ID, PlayerB: charB.ID,
        OfferA:  []trade.Item{{ItemID: 1, Qty: 1}},
        GoldA:   100,
    }
    err := svc.Commit(context.Background(), session)
    assert.NoError(t, err)
    // 断言 B 的背包新增物品...
}
```

### 5.7 Redis 数据结构（Redis 模式下）

| Key 模式 | 类型 | 用途 | TTL |
|---------|------|------|-----|
| `session:{token}` | Hash | 玩家会话（player_id, account_id, char_id） | 24h |
| `online:players` | Set | 在线玩家 ID 集合 | — |
| `map:{map_id}:players` | Set | 某地图在线玩家 | — |
| `player:{id}:pos` | Hash | 玩家当前位置缓存 `{x,y,map_id,dir}` | 5min |
| `player:{id}:buffs` | List | 当前 Buff 列表（JSON 序列化） | — |
| `player:{id}:skill_cd` | Hash | 技能 CD 到期时间戳 `{skill_id: unix_ms}` | — |
| `party:{party_id}` | Hash | 队伍信息 | — |
| `rank:level` | ZSet | 等级排行榜（score = level*1e9+exp） | — |
| `rank:combat` | ZSet | 战力排行榜 | — |
| `chat:world` | List | 全服聊天最近 200 条 | — |
| `monster:cd:{room_id}:{spawn_id}` | String | 怪物刷新 CD 倒计时 | 按配置 |
| `lock:trade:{sorted_ids}` | String | 交易分布式锁（防物品复制） | 30s |
| `lock:item:{char_id}:{item_id}` | String | 物品操作分布式锁 | 5s |

---

## 6. 资源读取与映射

### 6.1 自动读取 RPG Maker MV data/ 目录

服务端启动时扫描客户端项目的 `data/` 目录，构建只读内存数据库，支持热重载（Admin API 触发）：

```go
type ResourceLoader struct {
    DataPath string  // 指向 Project1/data/
}

func (r *ResourceLoader) Load() error {
    r.LoadSystem()        // System.json → 全局设置（初始地图、起始队伍）
    r.LoadActors()        // Actors.json → 玩家角色模板（初始属性、职业、装备）
    r.LoadClasses()       // Classes.json → 职业（成长曲线、学习技能列表）
    r.LoadSkills()        // Skills.json → 技能（效果、MP消耗、伤害公式、动画ID）
    r.LoadItems()         // Items.json → 物品（效果类型、价格、稀有度）
    r.LoadWeapons()       // Weapons.json → 武器（属性加成、装备类型）
    r.LoadArmors()        // Armors.json → 防具（属性加成、装备部位）
    r.LoadEnemies()       // Enemies.json → 怪物（AI参数、行动列表、掉落表）
    r.LoadTroops()        // Troops.json → 怪物组合（副本/野外刷新配置）
    r.LoadStates()        // States.json → 状态/Buff（效果 traits、持续时间）
    r.LoadAnimations()    // Animations.json → 动画帧（供客户端播放参考）
    r.LoadMaps()          // Map*.json → 地图（图块数据、事件、通行性位图）
    r.LoadMapInfos()      // MapInfos.json → 地图树形结构（地图名称、父子关系）
    r.LoadCommonEvents()  // CommonEvents.json → 公共事件（任务脚本）
    r.LoadTilesets()      // Tilesets.json → 图块集（通行性 flags 解析）
    return nil
}
```

### 6.2 关键映射规则

**地图通行性解析**（用于服务端权威移动验证）：

```
Map*.json 中 data[] 是三维数组 [layer][y*width+x]，存储 tileId
Tilesets.json 中 flags[] 存储每个 tileId 的通行性位掩码：
  bit 0: 下方不可通行
  bit 1: 左方不可通行
  bit 2: 右方不可通行
  bit 3: 上方不可通行
  bit 4: 梯子
  bit 5: 灌木
  bit 6: 计数器
  bit 7: 伤害地板
  ...

服务端构建 passability[y][x][dir] 布尔图，用于每次移动校验
```

**怪物 AI 参数来源**：

- `Enemies.json` → `actions[]`：行动模式列表（各行动的技能ID、使用条件、评分权重）
- `Enemies.json` → `dropItems[]`：掉落表（物品类型、ID、掉落概率分母）
- `Enemies.json` → `traits[]`：特殊属性（元素抗性、状态免疫、行动次数）

**技能伤害公式解析**（兼容 RPG Maker MV 标准公式）：

```
Skills.json damage.formula 字段示例：
  "a.atk * 4 - b.def * 2"
  "a.mat * 2.5 - b.mdf * 1.5"
  "a.luk * 0.5 + 100"

服务端使用白名单表达式解析器（只允许：a/b 对象的属性访问、四则运算、Math.函数）
a 对应攻击方属性 {atk, def, mat, mdf, agi, luk, hp, mp, level}
b 对应防御方属性（同上）
禁止访问任何其他 Go/JS 运行时对象
```

**NPC 事件解析**：

```
Map*.json events[] 中每个事件包含 pages[]，每个 page 包含：
  conditions：触发条件（switch、variable、self_switch、actor in party）
  list：事件指令列表（Show Text、Conditional Branch、Transfer、Battle、Get Item 等）

服务端逐条解释执行事件指令，客户端只接收 npc_dialog、inventory_update 等结果消息
```

---

## 7. 游戏系统详细设计

### 7.1 战斗系统

**伤害计算流程**：

```
① Hook: before_damage_calc(attacker, target, skill) → 可修改属性
② 套用 Skills.json 的 damage.formula（白名单求值器）
③ 套用元素属性倍率（target 的 elementRates trait）
④ 套用状态修正（attacker/target 当前 State 的 trait 修改）
⑤ 暴击判定（luk 参数影响暴击率）
⑥ 伤害浮动（±10% 随机）
⑦ 最终伤害 = max(0, 计算结果)
⑧ Hook: after_damage_calc(damage, attacker, target) → 可修改最终伤害
⑨ 广播 battle_result，更新内存 HP，异步写 Redis 缓存
```

**经验分配**（怪物死亡时）：

- 单人：击杀者获得 100% 经验
- 组队：经验按 `1 + (队员数-1) * 0.1` 系数加成，在场队员平均分配（需在同一地图的视野范围内）
- 经验超出升级阈值时自动升级，触发 Hook `on_player_level_up`

### 7.2 AI 系统

怪物和 NPC AI 采用行为树（Behavior Tree）实现，参见 [`docs/engine/ai/`](../ai/) 系列文档：

```
史莱姆 AI 行为树示例：

Selector（选择器，选第一个成功的子节点）
├── Sequence（玩家在攻击范围内）
│   ├── Condition: IsPlayerInRange(radius=2)
│   └── Action: AttackPlayer(skill_id=1)   使用普通攻击
├── Sequence（已检测到玩家，追击）
│   ├── Condition: IsPlayerDetected(radius=6)
│   └── Action: MoveTo(target=player, pathfinding=A*)
├── Sequence（受到攻击，警觉）
│   ├── Condition: WasAttackedRecently(window=3s)
│   └── Action: AlertNearby(radius=4)      通知附近同类
└── Action: Wander(radius=3, interval=3s)  随机游走

AI Tick 频率：随地图 Room 50ms 帧执行
```

AI 状态（当前目标、追击计时器）持久化到 Redis，服务端重启后可恢复怪物状态。

### 7.3 任务系统

任务定义来源于 `data/CommonEvents.json`（以公共事件作为任务脚本容器）：

```
服务端解析出的 Quest 结构：
{
  id:          int            CommonEvent ID
  name:        string         任务名称（第一个 Show Text 指令提取）
  conditions:  []QuestCond    接受条件（等级要求、前置任务ID）
  objectives:  []QuestObj     {type: kill/collect/goto/talk, target_id, count}
  rewards:     QuestReward    {exp, gold, items[]}
  repeatable:  bool
}
```

**任务触发点**（服务端事件钩子）：

| 触发动作 | 更新逻辑 |
|---------|---------|
| 怪物死亡 | 检查所有进行中任务的 `kill` 类型目标 |
| 物品进入背包 | 检查 `collect` 类型目标 |
| 玩家进入地图坐标区域 | 检查 `goto` 类型目标 |
| NPC 对话结束 | 检查 `talk` 类型目标 |

进度存于 MySQL `quest_progress.progress` JSON 字段，完成时触发奖励发放 + `quest_update` 推送。

### 7.4 组队系统

```
规则：
- 最多 4 人成队（可由配置文件调整）
- 队长可邀请/踢人/转让队长
- 经验加成：每增加 1 个在场队员，基础经验 ×1.1（上限 ×1.4）
- 同地图：共享怪物击杀任务进度
- 不同地图：经验/掉落各自独立

队伍状态广播（每 1s 推送一次 party_update）：
每个成员：{char_id, name, class_id, hp, max_hp, mp, max_mp, buffs[], map_id, online}

副本（Instance Map）：组队时可申请进入专属副本地图，Server 动态创建 MapRoom
```

### 7.5 技能与 Buff 系统

```
技能 CD 管理（服务端）：
- 每个角色在 Redis 维护：player:{id}:skill_cd → Hash{skill_id: ready_at_unix_ms}
- 收到技能请求时：若 now < ready_at 则拒绝（返回错误）
- 使用成功后：ready_at = now + skill.cooldown_ms

Buff 系统（内存 + Redis 双存储）：
BuffInstance {
    buff_id    int         对应 States.json ID
    stacks     int         叠加层数
    expire_at  time.Time   到期时间（-1 表示永久）
    tick_ms    int         DOT/HOT Tick 间隔
    next_tick  time.Time
}

每帧 Tick 时：
  for each buff in player.buffs:
    if now >= buff.next_tick:
      apply_tick_effect(buff)    # 如 DOT 扣血、HOT 回血
      buff.next_tick += tick_ms
    if now >= buff.expire_at:
      remove_buff(player, buff)
      push S2C: buff_update(action=remove)
```

### 7.6 交易系统

防止物品复制使用分布式锁 + 数据库事务：

```
流程：
① A 发起请求 → B 收到邀请，确认后进入交易状态
② 双方进入 TradeSession（此时背包的被交易物品被锁定，无法使用/丢弃）
③ 双方放入物品/金币（可修改，对方实时看到变化）
④ 双方点"确认"
⑤ 服务端加分布式锁：lock:trade:{min(A,B)}_{max(A,B)}（Redis SETNX TTL 30s）
⑥ 数据库事务：
   - 检查双方实际拥有放入的物品（防时间差攻击）
   - 从 A 背包移除 A 的交易物，加入 B 背包
   - 从 B 背包移除 B 的交易物，加入 A 背包
   - 金币原子性转移
⑦ 释放锁，推送双方 inventory_update
⑧ 写入审计日志（action=trade_commit）
```

### 7.7 NPC 对话与事件系统

```
NPC 对话流程（服务端解释执行事件）：

① 客户端发送 npc_interact{npc_id, event_id}
② 服务端查找对应事件 page（根据触发条件、switch 状态选择有效 page）
③ 开始逐条执行事件指令列表：
   - Show Text → 推送 npc_dialog{text, face_name, face_index, choices?}
   - Show Choices → 等待客户端 npc_choice 回应
   - Conditional Branch → 服务端根据 gameVariables/gameSwitches 分支执行
   - Change Items/Gold → 修改角色背包/金币，推送 inventory_update/gold_update
   - Transfer Player → 推送 enter_map（地图传送）
   - Change Switch/Variable → 更新角色的 switches/variables（存 MySQL）
   - Start Battle → 触发特殊 Boss 战（保留 RMMV 回合制战斗用于剧情战）
   - **Script** → 进入 JS 沙箱执行（见 7.8 节）
④ 事件执行结束，推送 npc_dialog_close
```

### 7.8 服务端 JavaScript 执行沙箱

#### 7.8.1 问题背景

RPG Maker MV 事件系统中有一类特殊指令：**Script（脚本）**，允许开发者在事件页里写任意 JavaScript。常见用途包括：

```javascript
// 事件 Script 指令示例（Map*.json event.pages[].list 中 code=355/655 的条目）
$gameVariables.setValue(10, $gameVariables.value(10) + 1);   // 变量运算
$gameSwitches.setValue(5, true);                              // 开关控制
$gameParty.gainItem($dataItems[3], 2);                        // 给予物品
if ($gameParty.leader().level >= 10) { ... }                  // 条件判断
```

在单机模式下这些脚本由客户端 JS 引擎直接执行。MMO 模式下，服务端权威执行事件，必须能运行这些脚本同时：
1. 隔离沙箱——脚本不能访问服务器文件系统、网络或 Go 运行时
2. 超时保护——防止死循环挂死 goroutine
3. 上下文注入——让脚本看到服务端的游戏状态（变量、开关、背包等）

#### 7.8.2 引擎选型：goja 而非 v8go

| 维度 | `goja`（推荐） | `v8go` |
|------|--------------|--------|
| **实现方式** | 纯 Go，无 CGO | CGO + 预编译 V8 静态库 |
| **Windows 支持** | ✅ 完整支持 | ❌ **无 Windows 预编译库**（Issue #7 长期未解决） |
| **跨平台编译** | ✅ 任意 GOOS/GOARCH | ❌ 无法交叉编译 |
| **JS 标准** | ES5.1 + 部分 ES2015（~80%） | 完整 V8（ES2023+） |
| **RMMV 兼容性** | ✅ RMMV 使用 ES5，完全覆盖 | ✅ 过剩 |
| **沙箱** | ✅ 默认隔离，无 FS/网络/goroutine | 需手动配置 |
| **生产使用** | Grafana k6、Nakama 游戏后端 | 部分商业项目 |
| **部署复杂度** | 零依赖，`go build` 直接可用 | 需要 C++ 工具链 |

> **结论**：RMMV 开发者主要在 Windows 工作，v8go 无 Windows 支持直接排除。goja 是 Nakama（开源游戏后端框架）的脚本引擎选择，与本项目场景高度吻合。

#### 7.8.3 三层执行策略

并非所有 JS 都走沙箱，按复杂度分三层以保障性能：

```
Layer 1：白名单公式解析器（已有，零开销）
  适用：纯数学表达式，如 "a.atk * 4 - b.def * 2"
  判定：字符串不含 if/function/var/let/const/;
  性能：< 1μs/次

Layer 2：goja VM 池沙箱（新增）
  适用：RMMV 事件 Script 命令、复杂技能公式、自定义条件分支
  判定：其余所有 JS 字符串
  性能：首次 ~100μs（VM 热身），后续池化 ~10μs/次

Layer 3：服务端 JS 脚本文件（新增，可选）
  适用：开发者在 server_scripts/*.js 中写的复杂游戏逻辑模块
  运行方式：goja Runtime + require() 模块系统
  加载时机：服务启动时预加载，热重载由 Admin API 触发
```

#### 7.8.4 RMMV 上下文 Mock

在 goja 沙箱中注入与 RMMV 兼容的全局对象，代理到服务端真实状态：

```go
// game/script/context.go

func InjectRMMVContext(vm *goja.Runtime, ctx *ScriptContext) {
    // $gameVariables → 代理到角色变量（存 MySQL char_variables 表）
    vm.Set("$gameVariables", vm.ToValue(map[string]interface{}{
        "value":    ctx.GetVariable,              // func(id int) int
        "setValue": ctx.SetVariable,              // func(id int, val int)
    }))

    // $gameSwitches → 代理到角色开关
    vm.Set("$gameSwitches", vm.ToValue(map[string]interface{}{
        "value":    ctx.GetSwitch,                // func(id int) bool
        "setValue": ctx.SetSwitch,                // func(id int, val bool)
    }))

    // $gameParty → 只读视图，gainItem/loseItem 代理到背包系统
    vm.Set("$gameParty", vm.ToValue(map[string]interface{}{
        "leader":   ctx.GetLeader,               // 返回角色属性对象
        "members":  ctx.GetMembers,
        "gainItem": ctx.GainItem,                // func(item, amount int) → 写入背包
        "loseItem": ctx.LoseItem,
        "gold":     ctx.GetGold,
        "gainGold": ctx.GainGold,
    }))

    // $dataItems / $dataWeapons / $dataArmors → 只读 RMMV 数据（来自 ResourceLoader）
    vm.Set("$dataItems",   ctx.Resource.Items)
    vm.Set("$dataSkills",  ctx.Resource.Skills)
    vm.Set("$dataEnemies", ctx.Resource.Enemies)

    // 保留安全的全局对象
    // Math, JSON, String, Number, Array, Object, parseInt, parseFloat 保持默认

    // 屏蔽危险全局
    for _, danger := range []string{
        "require", "process", "fetch", "XMLHttpRequest",
        "WebSocket", "setTimeout", "setInterval", "__proto__",
    } {
        vm.Set(danger, goja.Undefined())
    }
}
```

#### 7.8.5 VM 池与超时控制

```go
// game/script/pool.go

type VMPool struct {
    pool chan *goja.Runtime
}

func NewVMPool(size int, resource *ResourceLoader) *VMPool {
    p := &VMPool{pool: make(chan *goja.Runtime, size)}
    for i := 0; i < size; i++ {
        vm := goja.New()
        // 注入不变的只读数据（RMMV data/ 资源），可复用
        vm.Set("$dataItems",   resource.Items)
        vm.Set("$dataSkills",  resource.Skills)
        vm.Set("$dataEnemies", resource.Enemies)
        p.pool <- vm
    }
    return p
}

// RunScript 从池中取 VM，注入玩家上下文，执行脚本，带超时保护
func (p *VMPool) RunScript(
    goCtx context.Context,
    script string,
    sctx *ScriptContext,
) (goja.Value, error) {
    vm := <-p.pool
    defer func() { p.pool <- vm }()

    // 注入玩家相关可变上下文
    InjectRMMVContext(vm, sctx)

    // 超时：goCtx 取消时中断 VM
    stop := make(chan struct{})
    go func() {
        select {
        case <-goCtx.Done():
            vm.Interrupt("script timeout")
        case <-stop:
        }
    }()
    defer close(stop)

    return vm.RunString(script)
}

// 全局 VM 池配置（config.yaml script.vm_pool_size，默认 8）
var GlobalVMPool = NewVMPool(8, resourceLoader)
```

调用方式（在事件执行器中）：

```go
// 执行事件 Script 指令
case EventCodeScript: // code == 355
    ctx, cancel := context.WithTimeout(baseCtx, 3*time.Second)
    defer cancel()
    _, err := script.GlobalVMPool.RunScript(ctx, cmd.Parameters[0], scriptCtx)
    if err != nil {
        log.Warn("script error", zap.String("script", cmd.Parameters[0]), zap.Error(err))
        // 脚本出错不中断事件流，记录日志后继续执行下一条指令
    }
```

#### 7.8.6 服务端自定义脚本文件

开发者可在 `server_scripts/` 目录放置 `.js` 文件，实现复杂机制：

```
server_scripts/
├── custom_battle.js       # 自定义战斗公式、特殊怪物行为
├── event_hooks.js         # 用 JS 实现的 Hook 逻辑（注册到 Go Hook 系统）
├── daily_activity.js      # 日常活动规则
└── custom_items.js        # 自定义物品使用效果
```

```javascript
// server_scripts/custom_items.js 示例

// 注册一个物品使用 Hook（框架暴露 registerHook 给 JS 层）
registerHook("before_item_use", function(ctx) {
    var item = ctx.item;
    var player = ctx.player;

    // 物品 ID 99：自定义复活草，只在 HP < 30% 时可以使用
    if (item.id === 99 && player.hp > player.maxHp * 0.3) {
        ctx.interrupt("HP 高于 30% 时无法使用复活草");
        return;
    }

    // 物品 ID 100：传送卷轴，随机传送到地图 3-8 的某张地图
    if (item.id === 100) {
        var mapId = Math.floor(Math.random() * 6) + 3;
        ctx.teleportPlayer(mapId, 5, 5);
        ctx.interrupt("OK"); // 拦截默认物品效果
    }
});
```

框架在服务启动时用 `goja.Runtime.RunString()` 加载这些文件，并将 `registerHook` 等桥接函数注入到 JS 全局环境，实现 JS ↔ Go Hook 系统的双向互通。

#### 7.8.7 更新：目录结构与配置

在 `server/` 中新增：

```
server_scripts/          # 开发者自定义 JS 脚本（热重载）
server/
└── game/
    └── script/
        ├── pool.go      # VM 池管理
        ├── context.go   # RMMV 上下文 Mock 注入
        └── loader.go    # server_scripts/ 目录加载器
```

`config.yaml` 新增：

```yaml
script:
  vm_pool_size:    8        # goja VM 池大小（建议 = CPU 核数）
  timeout_ms:      3000     # 单条 Script 指令最大执行时间（毫秒）
  scripts_dir:     "./server_scripts"  # 自定义 JS 脚本目录
  allow_custom:    true     # 是否允许加载 server_scripts/（生产环境可关闭）
```

---

## 8. 插件与扩展系统

### 8.1 服务端插件接口

```go
// 插件必须实现的接口
type ServerPlugin interface {
    Name()    string
    Version() string
    OnLoad(ctx PluginContext) error
    OnUnload() error
}

// 插件可用的服务端 API
type PluginContext interface {
    RegisterHook(event string, priority int, handler HookFn) error
    UnregisterHook(event string, handler HookFn) error
    GetDB()        *gorm.DB      // 返回当前运行模式的 DB 实例（sqlexec/SQLite/MySQL 均统一为 *gorm.DB）
    GetCache()     cache.Cache   // 返回 Cache 实现（Redis 或 LocalCache，由配置决定）
    GetPubSub()    cache.PubSub  // 返回 PubSub 实现（Redis 或 LocalPubSub）
    GetScheduler() Scheduler
    Logger()       *zap.Logger
    GetResource()  *ResourceLoader  // 访问 RMMV 数据
    BroadcastMap(mapID int, packet Packet)
    SendToPlayer(playerID int64, packet Packet)
}
```

### 8.2 Hook 系统

Hook 采用优先级队列，多个插件可同时注册同一 Hook：

| Hook 事件 | 触发时机 | 可修改内容 | 可中断 |
|---------|---------|---------|-------|
| `before_player_move` | 移动请求校验前 | 目标坐标、是否允许 | 是 |
| `after_player_move` | 移动成功后 | 触发额外效果 | 否 |
| `before_damage_calc` | 伤害计算前 | 攻击/防御属性 | 是 |
| `after_damage_calc` | 伤害计算后 | 最终伤害值 | 否 |
| `before_skill_use` | 技能使用前 | 参数、是否允许 | 是 |
| `after_skill_use` | 技能使用后 | 触发额外效果 | 否 |
| `after_monster_death` | 怪物死亡后 | 掉落物品列表、经验值 | 否 |
| `before_item_use` | 物品使用前 | 是否允许 | 是 |
| `on_quest_complete` | 任务完成时 | 额外奖励注入 | 否 |
| `on_player_level_up` | 玩家升级时 | 额外属性点/技能 | 否 |
| `on_player_login` | 登录成功后 | 登录奖励逻辑 | 否 |
| `on_player_logout` | 玩家下线时 | 清理/统计 | 否 |
| `on_chat_send` | 聊天消息发送前 | 内容过滤、GM 指令处理 | 是 |
| `before_trade_commit` | 交易提交前 | 税收、额外校验 | 是 |
| `on_map_enter` | 进入地图后 | 入场 Buff、特殊限制 | 否 |

### 8.3 插件热加载

```
方式一：Go Plugin（.so）
  限制：只支持 Linux，需重新编译
  优点：性能最高，完整 Go 生态

方式二：Yaegi（Go 脚本解释器）
  限制：性能略低
  优点：跨平台，无需编译，真正热重载
  推荐用于：游戏规则调整类插件

热加载流程（两种方式相同）：
  1. 将插件文件放入 plugins/ 目录
  2. POST /admin/plugins/reload
  3. 服务端：
     a. 调用旧插件 OnUnload()，注销其所有 Hook 和 Cron
     b. 加载新插件文件
     c. 调用新插件 OnLoad(ctx)
     d. 无需重启，在线玩家无感知
```

---

## 9. 调度系统

调度系统处理定期与非定期任务，接口设计：

```go
type Scheduler interface {
    // Cron 定时任务（标准 Cron 表达式）
    AddCron(name, cron string, fn TaskFn) error
    // 延迟任务（N 时间后执行一次）
    AddDelay(name string, delay time.Duration, fn TaskFn) error
    // 周期任务（每 N 时间执行，立即开始）
    AddTicker(name string, interval time.Duration, fn TaskFn) error
    // 移除任务（插件 OnUnload 时必须调用）
    Remove(name string) error
    // 列出所有任务及下次执行时间（Admin API 用）
    List() []TaskInfo
}
```

**内置调度任务**：

| 任务名 | 周期 | 说明 |
|-------|------|------|
| `auto_save` | 5 分钟 | 将所有在线玩家数据批量写入 MySQL |
| `monster_respawn` | 50ms（与 MapRoom Tick 联动） | 按地图配置检查并刷新已死亡怪物 |
| `buff_cleanup` | 50ms（与 MapRoom Tick 联动） | 清理过期 Buff，处理 DOT/HOT |
| `ranking_update` | 5 分钟 | 将角色等级/战力数据同步到 Redis ZSet |
| `offline_income` | 1 小时 | 计算公会离线产出，发放邮件 |
| `daily_reset` | 每日 0:00 | 重置日常任务进度、竞技场次数 |
| `weekly_reset` | 每周一 0:00 | 重置周常任务、发放排行榜奖励邮件 |
| `session_cleanup` | 10 分钟 | 清理心跳超时的僵尸会话 |
| `drop_cleanup` | 5 分钟 | 清理地图上超时未被拾取的掉落物 |

---

## 10. 日志与审计

### 10.1 日志分层

采用 `go.uber.org/zap` 结构化日志：

| 日志类型 | 输出目标 | 用途 |
|---------|---------|------|
| 访问日志 | 文件（按日轮转） | 所有 WebSocket 消息收发、REST 请求 |
| 游戏逻辑日志 | 文件（按日轮转） | 战斗结算、任务完成、交易等关键事件 |
| 错误日志 | 文件 + 告警通知（Webhook） | Panic、数据库错误、系统异常 |
| 审计日志 | MySQL `audit_logs` 表 | 需要可追溯的操作（交易、GM 操作等） |
| 性能日志 | 文件 | 慢请求（>100ms）、慢 SQL（>50ms） |

### 10.2 审计日志字段

每条审计日志包含以下所有字段：

```json
{
  "trace_id":    "550e8400-e29b-41d4-a716-446655440000",
  "account_id":  10001,
  "char_id":     20001,
  "char_name":   "勇者小明",
  "action":      "player_attack",
  "ip":          "123.45.67.89",
  "map_id":      3,
  "request": {
    "target_id":   9001,
    "target_type": "monster"
  },
  "response": {
    "damage":    128,
    "target_hp": 72,
    "kill":      false
  },
  "error":       null,
  "duration_ms": 2,
  "created_at":  "2026-02-22T10:30:00.123Z"
}
```

### 10.3 Trace ID 传播

每条 WebSocket 消息在服务端处理时生成唯一 Trace ID（UUID v4），存入 `PlayerSession.TraceID`，贯穿该消息的完整处理链路（包括数据库操作、Hook 调用、跨 goroutine 传递），所有日志输出携带此 ID，便于问题快速定位。

---

## 11. 调试系统

### 11.1 Chrome DevTools 远程调试

```
启动命令：
  ./server --debug --debug-port=9229

Chrome 访问：
  地址栏输入 chrome://inspect
  点击 "Configure..." 添加 localhost:9229
  即可在 DevTools 中调试 Go 服务端

支持功能：
  - Sources 面板：设置断点，单步执行任意 Hook 点
  - Console 面板：执行调试命令（查询玩家状态、触发 GM 指令）
  - Memory 面板：查看堆内存，排查泄漏
  - Network 面板：WebSocket 消息实时捕获与回放
```

### 11.2 Admin REST API

仅在内网/Debug 模式下开放：

```
GET  /admin/players                     在线玩家列表（含地图、坐标、状态）
GET  /admin/players/:id                 单个玩家完整状态快照
POST /admin/players/:id/kick            强制踢出玩家
POST /admin/players/:id/ban             封禁账号（status=0）
POST /admin/players/:id/give-item       给予物品（GM 指令）
POST /admin/players/:id/give-exp        给予经验（GM 指令）
POST /admin/players/:id/teleport        传送至指定地图坐标
POST /admin/players/:id/buff            施加/移除 Buff（测试用）

GET  /admin/maps                        所有地图 Room 状态（在线人数、怪物数）
GET  /admin/maps/:id/players            某地图所有玩家列表
GET  /admin/maps/:id/monsters           某地图所有怪物实例状态

POST /admin/broadcast                   全服系统公告
POST /admin/reload-resources            热重载 RPG Maker MV data/ 目录
POST /admin/plugins/reload              热加载指定插件
GET  /admin/plugins                     已加载插件列表
GET  /admin/scheduler/tasks             调度任务列表及下次执行时间
GET  /admin/metrics                     实时性能指标（在线人数、TPS、平均延迟、GC）
```

### 11.3 客户端调试接口

在 NW.js 的开发者工具（F12）中，`mmo-core.js` 挂载调试对象：

```javascript
// 仅在 DEBUG=true 时挂载
window.$MMO = {
    debug: {
        sendRaw: function(type, payload) { /* 手动发 WS 消息 */ },
        getSession: function() { return currentSession; },
        getMapState: function() { return currentMapState; },
        enablePacketLog: function() { /* 开启收发消息日志 */ },
        disablePacketLog: function() { /* 关闭 */ },
        simulateLag: function(ms) { /* 模拟网络延迟 */ }
    }
};
```

---

## 12. 安全性设计

### 12.1 认证与授权

- 登录/注册通过 HTTPS REST 接口，返回 JWT Token（HS256，有效期 24h）
- WebSocket 握手时在 URL 参数携带 Token：`wss://server/ws?token=xxx`
- 每条 WebSocket 消息处理前，从 Redis 验证 Token 对应的 Session 是否有效
- 密码使用 bcrypt 哈希（cost=12）存储，服务端不存储明文
- GM/Admin API 通过独立的内网 IP 白名单 + API Key 双重保护

### 12.2 反作弊机制

| 作弊类型 | 防护措施 |
|---------|---------|
| 移动加速/瞬移 | 服务端校验相邻两帧的坐标变化量（最大允许值 = 角色速度 × 时间 × 1.3 容错系数） |
| 穿墙 | 服务端基于 Tilesets.json passability 数据进行通行性检查 |
| 无限 HP | HP 全部在服务端计算，客户端显示值来自服务端推送，本地修改无效 |
| 技能 CD 绕过 | 服务端以 Redis 时间戳为准，早于 CD 结束的请求直接拒绝 |
| 物品复制 | 所有物品增减在数据库事务内原子执行，关键操作加分布式锁 |
| 伤害篡改 | 伤害公式完全在服务端执行，客户端数据不参与计算 |
| 重放攻击 | C2S 消息包含单调递增 `seq` 字段，服务端拒绝旧 seq |

### 12.3 输入校验与限流

所有客户端数据在服务端进行以下校验：

1. **结构校验**：JSON Schema 验证必要字段和类型
2. **范围校验**：坐标、物品槽位、skill_id、npc_id 等均在合法范围内
3. **存在性校验**：target_id 对应的玩家/怪物/NPC 确实存在于同一地图
4. **权限校验**：只能操作自己的物品、只有队长能踢人等
5. **频率限制**：Redis 令牌桶，每连接最多 20 条/秒，超过则断开连接并记录日志

---

## 13. 部署与运维

### 13.1 推荐部署结构（单服初期，< 1000 人）

```
┌──────────────────────────────────────────┐
│              云服务器（Linux）             │
│                                          │
│  ┌────────────────────────────────────┐  │
│  │   Nginx（SSL 终结 + 反向代理）       │  │
│  │   wss://game.example.com → :8080   │  │
│  │   静态文件：Project1/ → /game/      │  │
│  └───────────────────┬────────────────┘  │
│                      │                   │
│  ┌───────────────────▼────────────────┐  │
│  │         Go 游戏服务器               │  │
│  │   :8080（WS + REST + SSE）          │  │
│  │   :9229（DevTools，仅内网）          │  │
│  └───────────────────┬────────────────┘  │
│                      │                   │
│  ┌───────────────────▼────────────────┐  │
│  │  DB（按模式）      Cache（按配置）   │  │
│  │  内嵌XML / SQLite  LocalCache（默认）│  │
│  │  MySQL :3306       Redis :6379（可选）│  │
│  └────────────────────────────────────┘  │
└──────────────────────────────────────────┘

RPG Maker MV 客户端文件（Project1/）
由 Nginx 直接提供静态文件服务，玩家通过浏览器或 NW.js 访问

> **零依赖快速启动**：`database.mode: embedded_xml` + `cache.redis_addr: ""`
> 无需安装 MySQL 或 Redis，直接运行 `./server` 即可启动完整 MMO 服务器。
```

### 13.2 配置文件（config.yaml）

```yaml
server:
  port: 8080
  debug: false
  debug_port: 9229
  admin_key: "your-admin-api-key"

rpgmaker:
  data_path: "/app/Project1/data"   # RPG Maker MV data/ 目录
  img_path:  "/app/Project1/img"

database:
  # 可选值：embedded_xml（默认） | embedded_memory | sqlite | mysql
  mode: "embedded_xml"

  # embedded_xml 模式：XML 数据文件存储目录（相对于服务器工作目录）
  embedded_path: "./data/db"

  # sqlite 模式：.db 文件路径
  sqlite_path: "./data/game.db"

  # mysql 模式：连接字符串（仅 mode=mysql 时生效）
  mysql_dsn:        "user:pass@tcp(127.0.0.1:3306)/rpg_mmo?charset=utf8mb4&parseTime=True&loc=UTC"
  mysql_max_open:   50
  mysql_max_idle:   10
  mysql_max_life:   "1h"

cache:
  # Redis 地址留空则自动使用内置 LocalCache（单进程模式）
  redis_addr:     ""          # 示例：127.0.0.1:6379（留空 = 使用 LocalCache）
  redis_password: ""
  redis_db:       0
  # LocalCache 参数（redis_addr 为空时生效）
  local_gc_interval: "30s"   # 过期 key 清理间隔
  local_pubsub_buf:  256     # 每个订阅者的消息缓冲 channel 大小

game:
  map_tick_ms:      50       # 地图帧率，50ms = 20 TPS
  save_interval_s:  300      # 自动存档间隔（秒）
  max_party_size:   4        # 最大组队人数
  pvp_enabled:      false    # 是否开启 PVP
  start_map_id:     1        # 新角色起始地图（来自 System.json startMapId）
  start_x:          5
  start_y:          5
  protection_ms:    3000     # 进场无敌保护时长（毫秒）
  drop_lifetime_s:  300      # 掉落物在地图上存在的最长时间

security:
  jwt_secret:          "change-me-in-production"
  jwt_ttl_h:           24
  bcrypt_cost:         12
  rate_limit_per_sec:  20    # 每连接每秒最大消息数
  admin_ip_whitelist:  ["127.0.0.1", "10.0.0.0/8"]

plugins:
  dir:       "/app/plugins"
  auto_load: true
```

### 13.3 扩展路线

| 在线人数 | DB 模式 | Cache 模式 | 推荐扩展方案 |
|---------|---------|-----------|------------|
| < 200 | `embedded_xml` | LocalCache | 单服零依赖，适合独立开发者快速上线 |
| < 1000 | `sqlite` 或 `embedded_xml` | LocalCache 或 Redis 单实例 | 单服，有 Redis 时开启排行榜/跨进程锁 |
| 1000-5000 | `mysql` | Redis 单实例 | 切换 MySQL，添加只读从库分离读写 |
| 5000-20000 | `mysql` + 读写分离 | Redis Sentinel 高可用 | 地图分服：按地图 ID 区间路由到不同 Go 进程 |
| 20000+ | MySQL 分库分表 | Redis Cluster | 微服务拆分：战斗服/聊天服/登录服，引入 Kafka/Redis Stream 做服务间通信 |

---

## 附录 A：客户端与 RPG Maker MV 兼容性说明

| 功能 | RPG Maker MV 原有行为 | MMO 套件替换/扩展 |
|------|---------------------|-----------------|
| 存档/读档 | 本地 localStorage | **禁用**，角色数据全部由服务端管理 |
| 战斗触发 | 踩地雷进入回合制战斗 | **替换为即时战斗**；特殊剧情 Boss 战保留回合制 |
| 事件执行 | 客户端本地执行事件指令列表 | **迁移至服务端执行**，客户端接收结果推送 |
| NPC 对话 | 本地 `Show Text` 指令 | 服务端解析事件页后通过 `npc_dialog` 推送内容 |
| 标题场景 | 新游戏 / 继续 / 关于 | **替换为登录场景** |
| 游戏变量/开关 | 本地 `$gameVariables/$gameSwitches` | 关键任务变量同步到服务端 MySQL |
| 菜单场景 | 本地角色属性、背包、技能菜单 | **替换为与服务端数据同步的 MMO 面板** |
| 地图事件触发 | 玩家步入触发区自动执行 | 服务端检测位置触发，推送执行结果 |

## 附录 B：开发里程碑（参考）

| 阶段 | 核心功能 | 验收标准 |
|------|---------|---------|
| **M1** 网络连通 | 登录 → 角色创建/选择 → 进入地图 → 多玩家可见 | 5 个客户端同时在线，互相看到对方移动 |
| **M2** 基础战斗 | 即时战斗、怪物 AI 追击、经验/掉落结算 | 击杀怪物正常获得经验，掉落物可拾取 |
| **M3** 角色成长 | 技能系统、Buff/Debuff、装备系统、升级属性成长 | 技能 CD 正常、装备属性生效 |
| **M4** 社交系统 | 聊天（多频道）、组队、好友、公会 | 组队共享经验，公会聊天正常 |
| **M5** 内容系统 | 任务系统、NPC 对话、副本、活动调度 | 完成一条完整任务链 |
| **M6** 生产就绪 | 安全加固、压力测试、GM 工具、监控告警 | 500 并发无崩溃，关键操作审计完整 |
