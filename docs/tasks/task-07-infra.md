# Task 07 - 基础设施层（Hook/Plugin, Scheduler, Audit, Security, Admin）

> **优先级**：P2 — 贯穿全项目，可与其他 Task 并行，逐步接入
> **里程碑**：M1-M5 各阶段逐步完善
> **依赖**：Task 00（基础层）；各游戏系统 Task 在预留 Hook 接口后接入

---

## 目标

实现 Hook 系统（优先级队列，支持拦截/修改）、插件热加载管理器（Go Plugin + Yaegi）、调度系统（Cron/Delay/Ticker）、结构化审计日志、Gin 安全中间件（JWT/限流/seq 校验），以及内网 Admin REST API 和 Chrome DevTools 调试接口。

---

## Todolist

- [ ] **07-1** 实现 Hook 系统（`plugin/hook/`）
  - [ ] 07-1a `HookCenter`：注册/注销/触发 Hook
  - [ ] 07-1b 优先级队列（priority int，小数值先执行）
  - [ ] 07-1c `HookContext`：可传递参数 + `Interrupt(reason string)` 中断后续 Hook
  - [ ] 07-1d 线程安全（读多写少，`sync.RWMutex`）
  - [ ] 07-1e 全部 §8.2 中的 Hook 事件常量定义
  - [ ] 07-1f 在各游戏系统中接入预留的 Hook 调用点（Task 03/04/05/06 已预留）
- [ ] **07-2** 实现插件管理器（`plugin/manager.go`）
  - [ ] 07-2a 插件接口定义（`ServerPlugin`、`PluginContext`，完整代码见 §8.1）
  - [ ] 07-2b Yaegi 解释器插件加载（跨平台热加载）
  - [ ] 07-2c Go Plugin（.so）加载（仅 Linux）
  - [ ] 07-2d 热加载流程：OnUnload → 注销 Hook/Cron → 加载新版本 → OnLoad
  - [ ] 07-2e PluginContext 实现（封装 DB/Cache/Scheduler/Logger/Resource 访问）
- [ ] **07-3** 实现调度系统（`scheduler/`）
  - [ ] 07-3a `Scheduler` 接口（AddCron/AddDelay/AddTicker/Remove/List）
  - [ ] 07-3b 基于 `robfig/cron/v3` 实现 AddCron
  - [ ] 07-3c 基于 `time.AfterFunc` 实现 AddDelay
  - [ ] 07-3d 基于 `time.NewTicker` 实现 AddTicker
  - [ ] 07-3e 注册所有内置调度任务（§9 列表）
    - `auto_save`：每 5 分钟批量写入在线玩家数据到 DB
    - `ranking_update`：每 5 分钟同步等级/战力到 Cache ZSet
    - `session_cleanup`：每 10 分钟清理心跳超时会话
    - `drop_cleanup`：每 5 分钟清理过期掉落物
    - `ranking_rebuild`：服务启动时立即执行（LocalCache 模式下重建 ZSet）
    - `daily_reset`：每日 0:00
    - `weekly_reset`：每周一 0:00
    - `offline_income`：每 1 小时（公会离线产出）
    - `cleanup_audit_logs`：非 MySQL 模式下每日 3:00 删除 90 天前审计日志
- [ ] **07-4** 实现审计日志（`audit/`）
  - [ ] 07-4a `AuditService.Log(entry AuditEntry)` — 异步批量写入 DB（channel + worker goroutine）
  - [ ] 07-4b `AuditEntry` struct（对应 §10.2 的所有字段）
  - [ ] 07-4c Gin 中间件：请求完成后自动写审计日志（针对关键 REST 接口）
  - [ ] 07-4d WS 关键操作（交易、GM 指令）手动调用 AuditService.Log
- [ ] **07-5** 实现安全中间件（补充 Task 01 已有中间件）
  - [ ] 07-5a WS 层 seq 单调递增校验（在 Router.Dispatch 中实现）
  - [ ] 07-5b WS 层频率限制：每连接 20 条/秒（令牌桶），超限断开连接并记录
  - [ ] 07-5c JSON Schema 结构校验（关键消息的必要字段和类型验证）
  - [ ] 07-5d 范围校验工具函数（坐标、ID 合法性验证）
  - [ ] 07-5e IP 白名单中间件（Admin API）
- [ ] **07-6** 实现 Admin REST API（`api/rest/admin.go`）
  - [ ] 玩家管理：list/get/kick/ban/give-item/give-exp/teleport/buff
  - [ ] 地图管理：list rooms/get players/get monsters
  - [ ] 系统操作：broadcast/reload-resources/reload-plugins
  - [ ] 调度器：list tasks（含下次执行时间）
  - [ ] 监控指标：`GET /admin/metrics`（在线人数/TPS/平均延迟/GC 统计）
- [ ] **07-7** 实现 Chrome DevTools 调试适配（`debug/`）
  - [ ] `--debug` 启动参数解析
  - [ ] 监听 `--debug-port`（默认 9229）
  - [ ] 集成 Go 调试协议（`go tool pprof` HTTP 端点作为替代方案）
  - [ ] WebSocket 消息实时捕获与回放（存入内存 ring buffer，Admin API 读取）
- [ ] **07-8** 编写单元测试
  - [ ] hook_test.go：多优先级 Hook 执行顺序、Interrupt 中断
  - [ ] scheduler_test.go：AddTicker 按时执行，Remove 停止
  - [ ] audit_test.go：写入 AuditLog 后 DB 可查到

---

## 实现细节与思路

### 07-1 Hook 系统

```go
// plugin/hook/center.go

type HookFn func(ctx context.Context, event string, data interface{}) (interface{}, error)

type hookEntry struct {
    priority int
    fn       HookFn
    id       uintptr   // fn 的函数指针，用于 Unregister
}

type HookCenter struct {
    hooks map[string][]*hookEntry   // event → sorted by priority
    mu    sync.RWMutex
}

// Trigger 按优先级执行所有注册的 Hook，返回最终 data（可被修改）
// 任一 Hook 返回 ErrInterrupt → 停止执行并返回 interrupt reason
func (hc *HookCenter) Trigger(ctx context.Context, event string, data interface{}) (interface{}, error) {
    hc.mu.RLock()
    entries := hc.hooks[event]
    hc.mu.RUnlock()
    for _, e := range entries {
        var err error
        data, err = e.fn(ctx, event, data)
        if errors.Is(err, ErrInterrupt) {
            return data, err
        }
    }
    return data, nil
}
```

**Hook 事件常量**（§8.2）：
```go
const (
    HookBeforePlayerMove  = "before_player_move"
    HookAfterPlayerMove   = "after_player_move"
    HookBeforeDamageCalc  = "before_damage_calc"
    HookAfterDamageCalc   = "after_damage_calc"
    HookBeforeSkillUse    = "before_skill_use"
    HookAfterSkillUse     = "after_skill_use"
    HookAfterMonsterDeath = "after_monster_death"
    HookBeforeItemUse     = "before_item_use"
    HookOnQuestComplete   = "on_quest_complete"
    HookOnPlayerLevelUp   = "on_player_level_up"
    HookOnPlayerLogin     = "on_player_login"
    HookOnPlayerLogout    = "on_player_logout"
    HookOnChatSend        = "on_chat_send"
    HookBeforeTradeCommit = "before_trade_commit"
    HookOnMapEnter        = "on_map_enter"
)
```

**接入现有系统**（在对应 Task 的代码中补充调用）：
```go
// Task 03 damage.go 中接入
data, err := hookCenter.Trigger(ctx, hook.HookBeforeDamageCalc, &DamageContext{...})
if err != nil { return 0, false, err }  // interrupt = 本次攻击取消
dmgCtx := data.(*DamageContext)
// ... 计算 ...
hookCenter.Trigger(ctx, hook.HookAfterDamageCalc, &AfterDamageData{Damage: finalDamage})
```

### 07-2 插件管理器

**PluginContext 实现**：
```go
type pluginContextImpl struct {
    db        *gorm.DB
    cache     cache.Cache
    pubsub    cache.PubSub
    scheduler scheduler.Scheduler
    logger    *zap.Logger
    resource  *resource.ResourceLoader
    hookCenter *hook.HookCenter
    sessionMgr *player.SessionManager
    worldMgr  *world.WorldManager
}

// 实现 §8.1 中的所有方法
func (p *pluginContextImpl) GetDB() *gorm.DB         { return p.db }
func (p *pluginContextImpl) GetCache() cache.Cache    { return p.cache }
func (p *pluginContextImpl) GetPubSub() cache.PubSub  { return p.pubsub }
// ...
```

**Yaegi 热加载**：
```go
import "github.com/traefik/yaegi/interp"
import "github.com/traefik/yaegi/stdlib"

func loadYaegiPlugin(path string, ctx PluginContext) (ServerPlugin, error) {
    i := interp.New(interp.Options{})
    i.Use(stdlib.Symbols)
    // 导出框架 API 到 Yaegi 环境
    i.Use(exportedSymbols)  // 包含 PluginContext、Hook 等接口
    _, err := i.EvalPath(path)
    v, err := i.Eval("plugin.New()")
    plugin := v.Interface().(ServerPlugin)
    return plugin, nil
}
```

### 07-3 调度系统

```go
// scheduler/scheduler.go
import "github.com/robfig/cron/v3"

type schedulerImpl struct {
    cron    *cron.Cron
    tickers map[string]*time.Ticker
    delays  map[string]*time.Timer
    mu      sync.Mutex
}
```

**auto_save 实现**（批量写 DB）：
```go
scheduler.AddTicker("auto_save", 5*time.Minute, func() {
    sessions := sessionMgr.AllSessions()
    for _, s := range sessions {
        // 将内存中的 HP/MP/X/Y/MapID 等写入 DB（只更新变化字段）
        db.Model(&model.Character{}).Where("id = ?", s.CharID).Updates(map[string]interface{}{
            "hp": s.HP, "mp": s.MP, "map_id": s.MapID, "map_x": s.X, "map_y": s.Y,
        })
    }
})
```

**ranking_update 实现**：
```go
scheduler.AddTicker("ranking_update", 5*time.Minute, func() {
    var chars []model.Character
    db.Select("id, name, level, exp").Find(&chars)
    for _, c := range chars {
        score := float64(c.Level)*1e9 + float64(c.Exp)
        cache.ZAdd(ctx, "rank:level", score, fmt.Sprintf("%d:%s", c.ID, c.Name))
    }
})
```

### 07-4 审计日志

**异步批量写入**（防止每次写 DB 阻塞游戏逻辑）：
```go
type AuditService struct {
    db     *gorm.DB
    queue  chan *AuditEntry
    batch  []*AuditEntry
}

func (s *AuditService) Start() {
    go func() {
        ticker := time.NewTicker(5 * time.Second)
        for {
            select {
            case entry := <-s.queue:
                s.batch = append(s.batch, entry)
                if len(s.batch) >= 100 {
                    s.flush()
                }
            case <-ticker.C:
                if len(s.batch) > 0 {
                    s.flush()
                }
            }
        }
    }()
}

func (s *AuditService) flush() {
    s.db.Create(&s.batch)  // 批量插入
    s.batch = s.batch[:0]
}
```

### 07-5 WS 频率限制（令牌桶）

```go
// PlayerSession 新增字段
type PlayerSession struct {
    ...
    limiter *rate.Limiter  // golang.org/x/time/rate
}

// 创建 Session 时初始化
s.limiter = rate.NewLimiter(rate.Limit(cfg.Security.RateLimitPerSec), cfg.Security.RateLimitPerSec)

// readPump 中每条消息前检查
func (s *PlayerSession) readPump() {
    for {
        _, msg, err := s.Conn.ReadMessage()
        if err != nil { break }
        if !s.limiter.Allow() {
            log.Warn("rate limit exceeded", zap.Int64("player", s.PlayerID))
            s.Conn.Close()  // 超限断开
            break
        }
        router.Dispatch(s, msg)
    }
}
```

### 07-6 Admin API

**metrics 端点**（使用 Go runtime 统计）：
```go
func (h *AdminHandler) Metrics(c *gin.Context) {
    var memStats runtime.MemStats
    runtime.ReadMemStats(&memStats)
    c.JSON(200, gin.H{
        "online_players":  sessionMgr.Count(),
        "map_rooms":       worldMgr.Count(),
        "goroutines":      runtime.NumGoroutine(),
        "heap_alloc_mb":   float64(memStats.HeapAlloc) / 1024 / 1024,
        "gc_count":        memStats.NumGC,
        "avg_latency_ms":  metrics.AvgLatency(),  // 自定义 metrics 累积
    })
}
```

---

## 验收标准

1. Hook 注册后在对应事件触发时执行，优先级高的先执行，Interrupt 阻止后续 Hook
2. 调度任务按时触发：auto_save 每 5 分钟将在线玩家写入 DB
3. 交易/GM操作审计日志正确写入 DB（异步，延迟 <5s）
4. WS 消息超过 20条/秒时连接被强制断开并记录日志
5. `GET /admin/metrics` 返回在线人数、goroutine 数等实时指标
6. `POST /admin/players/:id/give-item` 成功给玩家背包加物品并写审计日志
7. 插件热加载：放入 plugins/ 目录后调用 reload API，新插件 Hook 生效，旧插件 Hook 注销
8. 单元测试全部通过
