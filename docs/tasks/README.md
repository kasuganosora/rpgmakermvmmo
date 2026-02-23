# MakerPGM-MMO 任务分解总览

> 参考设计文档：[makerpgmv_mmo框架.md](../makerpgmv_mmo框架.md)

---

## 任务列表（按阶段/优先级）

| 任务文件 | 阶段 | 优先级 | 里程碑 | 核心内容 | 依赖 |
|---------|------|-------|-------|---------|------|
| [task-00-foundation.md](task-00-foundation.md) | P0 基础 | 🔴 最高 | 前置 | Go 项目初始化、DB 适配层（sqlexec/SQLite/MySQL）、Cache/PubSub 适配层（Redis/LocalCache）、RMMV 资源加载器 | 无 |
| [task-01-auth-characters.md](task-01-auth-characters.md) | P0 基础 | 🔴 最高 | M1 | 注册/登录（JWT）、角色 CRUD、Gin 中间件（Auth/限流/TraceID/日志） | Task 00 |
| [task-02-network-map.md](task-02-network-map.md) | P1 M1 | 🔴 最高 | M1 | WebSocket 连接管理、消息路由、MapRoom（20TPS）、玩家进入地图、移动同步 | Task 00, 01 |
| [task-03-combat-ai.md](task-03-combat-ai.md) | P1 M2 | 🟠 高 | M2 | 伤害计算、普通攻击、怪物 AI 行为树、A* 寻路、经验/掉落结算、拾取 | Task 02 |
| [task-04-skills-buff-equip.md](task-04-skills-buff-equip.md) | P2 M3 | 🟡 中 | M3 | 技能 CD、MP 消耗、Buff/DOT/HOT、装备系统、背包管理 | Task 03 |
| [task-05-social.md](task-05-social.md) | P2 M4 | 🟡 中 | M4 | 多频道聊天、组队系统、好友/黑名单、公会系统 | Task 02, 00 |
| [task-06-quest-npc-trade.md](task-06-quest-npc-trade.md) | P2 M5 | 🟡 中 | M5 | NPC 事件解释执行、任务系统、玩家交易（分布式锁+事务）、副本、商城、邮件 | Task 03, 04, 05 |
| [task-07-infra.md](task-07-infra.md) | P2 横切 | 🟡 中 | 全程 | Hook/Plugin 系统、调度器、审计日志、安全中间件、Admin API、调试接口 | Task 00（逐步接入） |
| [task-08-js-sandbox.md](task-08-js-sandbox.md) | P3 M5后 | 🟢 低 | M5+ | goja VM 池、RMMV 上下文 Mock、server_scripts/ 自定义 JS、JS↔Go Hook 桥接 | Task 06, 07 |
| [task-09-client-plugins.md](task-09-client-plugins.md) | — | — | 总览 | 客户端插件总览索引（单一入口设计、加载顺序、子任务索引） | — |
| [task-09-00-mmo-loader.md](task-09-00-mmo-loader.md) | P0 客户端 | 🔴 最高 | M1 前置 | `mmo-loader.js`：单一入口、动态同步加载所有插件、mmo-config.json、install.js | 无 |
| [task-09-01-mmo-core.md](task-09-01-mmo-core.md) | P0 客户端 | 🔴 最高 | M1 | `mmo-core.js`：WebSocket/重连/心跳/消息分发/本地存档禁用 | task-09-00 |
| [task-09-02-mmo-auth.md](task-09-02-mmo-auth.md) | P0 客户端 | 🔴 最高 | M1 | `mmo-auth.js`：Scene_Login/Scene_CharacterSelect/Scene_CharacterCreate | task-09-01 |
| [task-09-03-mmo-other-players.md](task-09-03-mmo-other-players.md) | P0 客户端 | 🔴 最高 | M1 | `mmo-other-players.js`：Sprite_OtherPlayer、头顶标签、线性插值 | task-09-01 |
| [task-09-04-mmo-battle.md](task-09-04-mmo-battle.md) | P1 客户端 | 🟠 高 | M2 | `mmo-battle.js`：即时战斗 UI、伤害飘字、怪物精灵、掉落物拾取 | task-09-01,03 |
| [task-09-05-mmo-hud.md](task-09-05-mmo-hud.md) | P1 客户端 | 🟠 高 | M3 | `mmo-hud.js`：HP/MP/EXP 条、小地图、任务追踪、功能按钮 2×3 | task-09-01 |
| [task-09-06-mmo-skill-bar.md](task-09-06-mmo-skill-bar.md) | P1 客户端 | 🟠 高 | M3 | `mmo-skill-bar.js`：12 格技能栏、F1-F12 热键、CD 扇形遮罩 | task-09-01,05 |
| [task-09-07-mmo-inventory.md](task-09-07-mmo-inventory.md) | P1 客户端 | 🟠 高 | M3 | `mmo-inventory.js`：背包 Grid、装备槽位、物品操作 | task-09-01,05 |
| [task-09-08-mmo-chat.md](task-09-08-mmo-chat.md) | P1 客户端 | 🟡 中 | M4 | `mmo-chat.js`：多频道 Tab、颜色编码、输入框焦点管理 | task-09-01 |
| [task-09-09-mmo-party.md](task-09-09-mmo-party.md) | P1 客户端 | 🟡 中 | M4 | `mmo-party.js`：队员 HP/MP/Buff 面板、邀请/踢人交互 | task-09-01,05 |
| [task-09-10-mmo-social.md](task-09-10-mmo-social.md) | P2 客户端 | 🟡 中 | M4 | `mmo-social.js`：好友列表在线状态、公会信息面板 | task-09-01,05 |
| [task-09-11-mmo-trade.md](task-09-11-mmo-trade.md) | P2 客户端 | 🟡 中 | M4 | `mmo-trade.js`：双列交易窗口、实时同步、确认流程 | task-09-01,07 |

---

* client_plugins 是客户端插件代码目录
* server 是服务端代码目录

## 开发里程碑对应关系

```
M1 网络连通（5客户端同时在线互相看到移动）
  服务端：Task 00 + Task 01 + Task 02
  客户端：task-09-00（loader） + task-09-01（core） + task-09-02（auth） + task-09-03（other-players）

M2 基础战斗（击杀怪物获得经验，掉落物可拾取）
  服务端：Task 03
  客户端：task-09-04（battle）

M3 角色成长（技能CD正常，装备属性生效）
  服务端：Task 04
  客户端：task-09-05（hud） + task-09-06（skill-bar） + task-09-07（inventory）

M4 社交系统（组队共享经验，公会聊天正常）
  服务端：Task 05
  客户端：task-09-08（chat） + task-09-09（party） + task-09-10（social） + task-09-11（trade）

M5 内容系统（完成一条完整任务链）
  服务端：Task 06 + Task 08（Script指令）
  基础设施：Task 07（全程逐步完善）
```

---

## 并行开发建议

以下任务可以并行推进：

```
Track A（服务端核心）：Task 00 → Task 01 → Task 02 → Task 03 → Task 04 → Task 06
Track B（社交系统）：Task 05（Task 02 就绪后即可开始）
Track C（基础设施）：Task 07（Task 00 就绪后即可开始，逐步与 Track A 接入）
Track D（客户端）：Task 09（Track A 对应阶段就绪后联调）
Track E（JS 沙箱）：Task 08（Task 06 就绪后开始）
```

---

## Agent 使用说明

每个 task-*.md 文件均包含：
1. **Todolist** — 可直接 checkbox 跟踪的子任务列表
2. **实现细节与思路** — 关键代码示例、算法说明、注意事项
3. **验收标准** — 该 task 完成的判断依据

Agent 在实现某 task 时，应：
1. 阅读对应 task-*.md 文件
2. 参考 [makerpgmv_mmo框架.md](../makerpgmv_mmo框架.md) 获取完整设计细节
3. 按 Todolist 逐项实现，完成后更新 checkbox
4. 按验收标准自检，确保测试通过
