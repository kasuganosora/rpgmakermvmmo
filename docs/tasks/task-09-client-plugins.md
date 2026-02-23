# Task 09 - 客户端插件总览（Client Plugins Overview）

> **优先级**：P1（核心）/ P2（UI 类）
> **依赖**：对应服务端 Task 就绪后联调

---

## 设计原则：单一入口 + 自动加载

用户只需在 `js/plugins.js` 中注册 **一个** 插件：

```javascript
var $plugins = [
    {
        "name":        "mmo-loader",
        "status":      true,
        "description": "MMO 套件加载器（自动加载所有 mmo-* 插件）",
        "filename":    "mmo-loader.js",
        "parameters":  {}
    }
];
```

`mmo-loader.js` 负责按正确顺序自动加载其余所有插件，用户无需手动排序或担心漏项。

---

## 插件加载顺序

```
mmo-loader.js              ← 唯一需要在 plugins.js 注册的入口
  │
  ├── [1] mmo-core.js          WebSocket 核心、消息分发、心跳、重连
  ├── [2] mmo-auth.js          登录/注册场景、角色选择/创建场景
  ├── [3] mmo-other-players.js 其他玩家渲染（Sprite_OtherPlayer）
  ├── [4] mmo-battle.js        即时战斗 UI（攻击/动画/飘字/掉落物）
  ├── [5] mmo-hud.js           HUD（HP/MP/EXP 条、小地图、任务追踪、功能按钮）
  ├── [6] mmo-skill-bar.js     技能快捷栏（F1-F12，12格，CD 遮罩）
  ├── [7] mmo-inventory.js     背包/装备 UI
  ├── [8] mmo-chat.js          聊天框（多频道）
  ├── [9] mmo-party.js         组队面板
  ├── [10] mmo-social.js       好友/公会 UI
  └── [11] mmo-trade.js        交易系统 UI
```

---

## 子任务文件索引

| 文件 | 插件 | 里程碑 | 核心功能 |
|------|------|-------|---------|
| [task-09-00-mmo-loader.md](task-09-00-mmo-loader.md) | `mmo-loader.js` | M1 前置 | 动态加载所有 mmo-* 插件、配置管理、依赖检查 |
| [task-09-01-mmo-core.md](task-09-01-mmo-core.md) | `mmo-core.js` | M1 | WebSocket 连接/重连/心跳、消息路由、本地存档禁用 |
| [task-09-02-mmo-auth.md](task-09-02-mmo-auth.md) | `mmo-auth.js` | M1 | Scene_Login、Scene_CharacterSelect、Scene_CharacterCreate |
| [task-09-03-mmo-other-players.md](task-09-03-mmo-other-players.md) | `mmo-other-players.js` | M1 | Sprite_OtherPlayer、头顶标签、线性插值同步 |
| [task-09-04-mmo-battle.md](task-09-04-mmo-battle.md) | `mmo-battle.js` | M2 | 攻击请求、动画播放、伤害飘字、掉落物拾取 |
| [task-09-05-mmo-hud.md](task-09-05-mmo-hud.md) | `mmo-hud.js` | M3 | HP/MP/EXP 条、小地图、任务追踪、功能按钮 2×3 |
| [task-09-06-mmo-skill-bar.md](task-09-06-mmo-skill-bar.md) | `mmo-skill-bar.js` | M3 | 12 格技能栏、F1-F12 热键、CD 遮罩、拖拽绑定 |
| [task-09-07-mmo-inventory.md](task-09-07-mmo-inventory.md) | `mmo-inventory.js` | M3 | 背包 Grid、装备栏、物品操作（使用/装备/丢弃） |
| [task-09-08-mmo-chat.md](task-09-08-mmo-chat.md) | `mmo-chat.js` | M4 | 多频道 Tab、颜色标识、输入框、历史消息 |
| [task-09-09-mmo-party.md](task-09-09-mmo-party.md) | `mmo-party.js` | M4 | 队员 HP/MP/Buff 面板、邀请/踢人流程 |
| [task-09-10-mmo-social.md](task-09-10-mmo-social.md) | `mmo-social.js` | M4 | 好友列表、公会面板 |
| [task-09-11-mmo-trade.md](task-09-11-mmo-trade.md) | `mmo-trade.js` | M4 | 交易窗口、双方物品展示、确认流程 |

---

## 文件目录结构

```
Project1/
├── js/
│   ├── plugins.js              ← 只注册 mmo-loader
│   └── plugins/
│       ├── mmo-loader.js       ← 主加载器（配置 + 动态加载）
│       ├── mmo-core.js
│       ├── mmo-auth.js
│       ├── mmo-other-players.js
│       ├── mmo-battle.js
│       ├── mmo-hud.js
│       ├── mmo-skill-bar.js
│       ├── mmo-inventory.js
│       ├── mmo-chat.js
│       ├── mmo-party.js
│       ├── mmo-social.js
│       └── mmo-trade.js
└── mmo-config.json             ← 用户配置（服务器地址等）
```

---

## 验收标准（总体）

1. `js/plugins.js` 中只有 `mmo-loader` 一个条目
2. 游戏启动时所有 mmo-* 插件按顺序自动加载
3. 任一插件文件缺失时 Loader 给出明确错误提示，不影响其他插件加载
4. 各插件独立验收标准见对应子任务文件
