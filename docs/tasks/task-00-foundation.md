# Task 00 - 基础框架搭建（Foundation）

> **优先级**：P0 — 所有其他任务的前置依赖，必须最先完成
> **里程碑**：无（基础设施层，贯穿整个项目）
> **依赖**：无

---

## 目标

搭建整个服务器的骨架：目录结构、配置加载、数据库适配层、缓存适配层、RPG Maker MV 资源加载器、Go module 初始化。完成后可以运行空服务器 + 单元测试零外部依赖通过。

---

## Todolist

- [ ] **00-1** 初始化 Go 模块与目录结构
- [ ] **00-2** 实现配置文件加载（config.yaml → struct）
- [ ] **00-3** 实现 DB 适配层（`db/` 包）
  - [ ] 00-3a `db/adapter.go` — `Open()` 工厂函数
  - [ ] 00-3b `db/embedded/driver.go` — sqlexec 内嵌 XML/Memory 适配
  - [ ] 00-3c `db/sqlite/driver.go` — SQLite 适配（modernc.org/sqlite 纯 Go）
  - [ ] 00-3d `db/mysql/driver.go` — MySQL 适配
- [ ] **00-4** 实现数据库 Model + AutoMigrate（`model/` 包）
- [ ] **00-5** 实现 Cache/PubSub 适配层（`cache/` 包）
  - [ ] 00-5a `cache/adapter.go` — 接口定义 + 工厂函数
  - [ ] 00-5b `cache/redis/client.go` — Redis 实现（go-redis/v9）
  - [ ] 00-5c `cache/local/cache.go` — LocalCache 纯 Go 实现
  - [ ] 00-5d `cache/local/pubsub.go` — LocalPubSub fan-out 实现
- [ ] **00-6** 实现 RMMV 资源加载器（`resource/loader.go`）
- [ ] **00-7** 实现测试工具包（`testutil/setup.go`）
- [ ] **00-8** 编写各层单元测试，CI 可以零外部依赖通过
- [ ] **00-9** 实现 `main.go` 骨架（加载配置 → 初始化 DB → 初始化 Cache → 加载资源 → 启动 Gin）

---

## 实现细节与思路

### 00-1 目录结构

按设计文档 §3.1 创建以下目录和空 `.go` 文件占位：

```
server/
├── main.go
├── config/
│   ├── config.go         # Config struct + Load()
│   └── config.yaml       # 默认配置（copy 自设计文档 §13.2）
├── api/
│   ├── ws/
│   ├── rest/
│   └── sse/
├── game/
│   ├── world/
│   ├── player/
│   ├── battle/
│   ├── ai/
│   ├── quest/
│   ├── skill/
│   ├── item/
│   ├── party/
│   ├── guild/
│   ├── social/
│   ├── chat/
│   ├── trade/
│   └── script/
├── resource/
│   └── loader.go
├── plugin/
│   ├── manager.go
│   └── hook/
├── scheduler/
├── audit/
├── debug/
├── middleware/
├── model/
├── db/
│   ├── adapter.go
│   ├── embedded/
│   ├── sqlite/
│   └── mysql/
├── cache/
│   ├── adapter.go
│   ├── redis/
│   └── local/
└── testutil/
    └── setup.go
```

Go module 名称：`github.com/yourorg/makerpgmv-mmo`（可自定义）

```bash
go mod init github.com/yourorg/makerpgmv-mmo
```

### 00-2 配置加载

使用 `github.com/spf13/viper` 读取 YAML，映射到 Go struct：

```go
// config/config.go
type Config struct {
    Server   ServerConfig
    RPGMaker RPGMakerConfig
    Database DatabaseConfig
    Cache    CacheConfig
    Game     GameConfig
    Security SecurityConfig
    Plugins  PluginsConfig
    Script   ScriptConfig
}
```

各子 struct 字段对应 §13.2 的 config.yaml 内容。
`Load(path string) (*Config, error)` 用 viper 读取并 Unmarshal。

### 00-3 DB 适配层

**关键依赖**：
```
github.com/kasuganosora/sqlexec/pkg/api
github.com/kasuganosora/sqlexec/pkg/gorm   # GORM Dialector
gorm.io/gorm
gorm.io/driver/sqlite                       # 底层使用 modernc.org/sqlite（纯Go）
gorm.io/driver/mysql
```

**`db/adapter.go`**（完整代码已在设计文档 §5.1）：
- `DBMode` 类型 + 4个常量
- `DatabaseConfig` struct
- `Open(cfg DatabaseConfig) (*gorm.DB, error)` switch 工厂

**`db/embedded/driver.go`**：
```go
type EngineType int
const (EngineXML EngineType = iota; EngineMemory)
func Open(dataPath string, eng EngineType) (*gorm.DB, error)
```
- `EngineXML`：`sqlexecapi.Options{DataPath: dataPath}`
- `EngineMemory`：`sqlexecapi.Options{InMemory: true}`
- 用 `sqlexecgorm.Open(instance)` 创建 GORM Dialector

**`db/sqlite/driver.go`**：
```go
import "gorm.io/driver/sqlite"
func Open(path string) (*gorm.DB, error) {
    return gorm.Open(sqlite.Open(path), &gorm.Config{})
}
```
注意：`gorm.io/driver/sqlite` v1.5+ 默认使用 `modernc.org/sqlite`（纯Go，无CGO）。

**`db/mysql/driver.go`**：
```go
func Open(dsn string, maxOpen, maxIdle int, maxLife time.Duration) (*gorm.DB, error)
```
设置连接池参数：`db.DB()` 后调用 `SetMaxOpenConns` / `SetMaxIdleConns` / `SetConnMaxLifetime`。

### 00-4 Model + AutoMigrate

在 `model/` 下创建以下 GORM Model struct（对应 §5.3 的 DDL）：

| 文件 | Struct |
|------|--------|
| `model/account.go` | `Account` |
| `model/character.go` | `Character` |
| `model/inventory.go` | `Inventory` |
| `model/skill.go` | `CharSkill` |
| `model/quest.go` | `QuestProgress` |
| `model/friendship.go` | `Friendship` |
| `model/guild.go` | `Guild`, `GuildMember` |
| `model/mail.go` | `Mail` |
| `model/audit.go` | `AuditLog` |
| `model/migrate.go` | `AutoMigrate(*gorm.DB) error` |

**GORM tag 要求**：
- 主键用 `gorm:"primaryKey;autoIncrement"`
- 唯一索引用 `gorm:"uniqueIndex"`
- JSON 字段用 `datatypes.JSON`（`gorm.io/datatypes`），跨所有 DB 引擎序列化为文本
- `created_at` / `updated_at` 用 GORM 内嵌 `gorm.Model` 或 `autoCreateTime` / `autoUpdateTime` tag
- 不使用数据库外键约束（业务层维护一致性）

**`AutoMigrate` 注意**：
- `PARTITION BY RANGE` 是 MySQL 专用特性，**不能**放进 GORM Model，需要在 `db/mysql/driver.go` 的 `Open()` 后单独执行 raw SQL（先 `AutoMigrate` 建表，再 `ALTER TABLE audit_logs PARTITION BY RANGE...`），且仅在 MySQL 模式下执行。

### 00-5 Cache/PubSub 适配层

**`cache/adapter.go`**（完整接口代码在 §5.4）：
- `Cache` interface（KV / Hash / Set / ZSet / List + SetNX + Expire）
- `PubSub` interface（Publish / Subscribe）
- `Message` struct
- `CacheConfig` struct（RedisAddr, RedisPassword, RedisDB, LocalGCInterval, LocalPubSubBuf）
- `NewCache(cfg) (Cache, error)`：RedisAddr 非空 → redis.New，否则 local.NewCache
- `NewPubSub(cfg) (PubSub, error)`：同上

**`cache/redis/client.go`**：
- 依赖 `github.com/redis/go-redis/v9`
- 实现 `Cache` 和 `PubSub` 接口，直接映射到 go-redis 的对应方法
- `PubSub.Subscribe`：用 `go-redis` 的 `PubSub.Channel()` 转为 `<-chan *Message`

**`cache/local/cache.go`**：
按 §5.5 的说明实现：
- KV：`sync.Map`，value 是 `struct{ data string; expireAt time.Time }`
- 后台 GC goroutine（ticker 间隔 = `CacheConfig.LocalGCInterval`）
- Hash：`sync.Map[key] → *sync.Map` （内层 field→value）
- Set：`sync.Map[key] → *lockedSet`（内部 `map[string]struct{} + sync.RWMutex`）
- ZSet：`sync.Map[key] → *zset`（`[]ZEntry{member, score}`，按 score 降序有序插入）
- List：`sync.Map[key] → *lockedList`（`[]string + sync.Mutex`，LPush 头插）
- SetNX：取全局 `sync.Mutex`，Get-Check-Set 原子操作

**`cache/local/pubsub.go`**：
```
subscribers: sync.Map[channel → []*subscriber]
subscriber: struct{ ch chan *Message; unsub func() }
Publish: 遍历 channel 下的所有 subscriber，非阻塞 send（select with default）
Subscribe: 创建 chan，注册，返回 chan + cancel func
```

### 00-6 RMMV 资源加载器

**`resource/loader.go`**：

```go
type ResourceLoader struct {
    DataPath string
    // 按 §6.1 的字段列表
    System     *SystemData
    Actors     []*Actor
    Classes    []*Class
    Skills     []*Skill
    Items      []*Item
    Weapons    []*Weapon
    Armors     []*Armor
    Enemies    []*Enemy
    Troops     []*Troop
    States     []*State
    Animations []*Animation
    Maps       map[int]*MapData      // map_id → MapData
    MapInfos   []*MapInfo
    CommonEvents []*CommonEvent
    Tilesets   []*Tileset
    // 派生数据（预处理）
    Passability map[int]*PassabilityMap  // map_id → PassabilityMap
}
```

实现步骤：
1. `Load() error`：依次调用各 `load*()` 私有方法
2. 每个 `load*()` 方法：`os.ReadFile(path/to/Xxx.json)` → `json.Unmarshal`
3. 地图文件特殊处理：扫描 `data/` 目录下所有 `Map[0-9]+\.json` 文件（跳过 `MapInfos.json`）
4. `buildPassability()`：解析 §6.2 的通行性算法，构建 `PassabilityMap`

**通行性构建**（§6.2）：
```
Map*.json data[] = tileId 三维数组 [layer][y*width+x]
Tilesets.json flags[] = 每个 tileId 的通行性位掩码

PassabilityMap[y][x][dir] bool:
  dir: 0=down(2), 1=left(4), 2=right(6), 3=up(8)
  合并所有图层的 tileId flags，任一层不可通行则该格不可通行
```

### 00-7 测试工具包

**`testutil/setup.go`**（完整代码在 §5.6）：
- `SetupTestDB(t *testing.T) *gorm.DB` — ModeEmbeddedMemory + AutoMigrate
- `SetupTestCache(t *testing.T) (cache.Cache, cache.PubSub)` — LocalCache（空 RedisAddr）

### 00-8 单元测试

每个包至少一个测试文件：

| 测试文件 | 测试内容 |
|---------|---------|
| `db/adapter_test.go` | 测试 4 种模式的 Open() 成功，AutoMigrate 无报错 |
| `cache/local/cache_test.go` | Get/Set/Del/TTL/SetNX/Hash/Set/ZSet/List 各方法 |
| `cache/local/pubsub_test.go` | Publish → Subscribe 收到消息，Unsubscribe 后停止 |
| `resource/loader_test.go` | 用测试 fixtures（`testdata/data/`）加载，验证解析字段 |
| `model/migrate_test.go` | SetupTestDB 后验证表存在 |

### 00-9 main.go 骨架

```go
func main() {
    cfg := config.Load("config/config.yaml")
    db  := db.Open(cfg.Database)
    model.AutoMigrate(db)
    c, ps := cache.NewCache(cfg.Cache), cache.NewPubSub(cfg.Cache)
    res  := resource.NewLoader(cfg.RPGMaker.DataPath)
    res.Load()

    r := gin.New()
    r.Use(middleware.Logger(), middleware.Recovery(), middleware.TraceID())
    // WS / REST / SSE 路由注册（此阶段只注册占位 handler）
    r.Run(fmt.Sprintf(":%d", cfg.Server.Port))
}
```

---

## 关键依赖包版本

```go
require (
    github.com/kasuganosora/sqlexec      latest
    gorm.io/gorm                         v1.25+
    gorm.io/driver/sqlite                v1.5+    // 自动使用 modernc.org/sqlite
    gorm.io/driver/mysql                 v1.5+
    gorm.io/datatypes                    v1.2+
    github.com/redis/go-redis/v9         v9.x
    github.com/gin-gonic/gin             v1.9+
    github.com/spf13/viper               v1.18+
    go.uber.org/zap                      v1.27+
    github.com/google/uuid               v1.6+
    github.com/stretchr/testify          v1.9+
)
```

---

## 验收标准

1. `go build ./...` 无错误
2. `go test ./...` 全部通过，无需任何外部服务（MySQL/Redis）
3. `./server` 启动后日志输出：DB 初始化成功、资源加载成功、Gin 监听端口
4. `embedded_xml` 模式下 `data/db/` 目录被创建并写入初始 XML 文件
