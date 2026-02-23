# Task 02 - 网络层与地图系统（WebSocket, MapRoom, Player Sync）

> **优先级**：P1 — M1 里程碑核心
> **里程碑**：M1（5个客户端同时在线，互相看到对方移动）
> **依赖**：Task 00（基础层），Task 01（Auth，用于 WebSocket 握手验证）

---

## 目标

实现 WebSocket 连接管理、消息路由分发、`MapRoom` 地图实例（20 TPS 游戏帧）、玩家进入/离开地图、位置同步（`player_move`）。完成后多个客户端可以连接，进入同一地图，互相看到实时移动。

---

## Todolist

- [ ] **02-1** 实现 WebSocket 连接管理（`api/ws/`）
  - [ ] 02-1a `PlayerSession` struct + 发送 goroutine
  - [ ] 02-1b WS 握手与 Token 验证
  - [ ] 02-1c 心跳机制（客户端 ping / 服务端 60s 超时断开）
  - [ ] 02-1d 断线重连：60s 内重连可恢复会话
- [ ] **02-2** 实现消息路由器（`api/ws/router.go`）
  - [ ] 统一消息格式解析 `{seq, type, payload}`
  - [ ] Handler 注册表（map[type]HandlerFunc）
  - [ ] seq 单调递增校验（防重放攻击）
- [ ] **02-3** 实现 `SessionManager`（`game/player/session_manager.go`）
  - [ ] 在线玩家 map（playerID → *PlayerSession）
  - [ ] 玩家上线/下线通知
  - [ ] 线程安全（sync.RWMutex）
- [ ] **02-4** 实现 `MapRoom`（`game/world/maproom.go`）
  - [ ] 20 TPS 游戏帧驱动（50ms ticker）
  - [ ] 玩家/NPC/怪物集合管理
  - [ ] broadcastQ 广播队列
  - [ ] Tick() 帧逻辑骨架（移动处理占位、AI占位、广播增量状态）
- [ ] **02-5** 实现 `WorldManager`（`game/world/world.go`）
  - [ ] 管理所有 MapRoom 实例（mapID → *MapRoom）
  - [ ] 按需创建/销毁 MapRoom
- [ ] **02-6** 实现进入地图处理（`HandleEnterMap`）
  - [ ] `enter_map` 消息处理：将玩家加入 MapRoom
  - [ ] 推送 `map_init`（当前地图所有玩家/NPC/怪物列表）
  - [ ] 3s 无敌保护 → 推送 `protection_end`
  - [ ] 广播 `player_join` 给地图其他玩家
- [ ] **02-7** 实现移动处理（`HandleMove`）
  - [ ] `player_move` 消息处理
  - [ ] 服务端通行性校验（ResourceLoader.Passability）
  - [ ] 移动速度合法性校验（最大允许速度 × 1.3 容错）
  - [ ] 广播 `player_sync` 给地图所有玩家
- [ ] **02-8** 实现玩家下线处理
  - [ ] WS 连接关闭时：将玩家从 MapRoom 移除，广播 `player_leave`
  - [ ] 异步保存玩家最后位置到 DB
- [ ] **02-9** 实现 SSE 端点骨架（`api/sse/sse.go`）
  - [ ] `GET /sse` — 建立 SSE 连接，供系统公告推送使用（内容后续任务填充）
- [ ] **02-10** 编写集成测试
  - [ ] 模拟 5 个 WS 客户端同时连接，进入同一地图，发送 move，断言其他客户端收到 player_sync

---

## 实现细节与思路

### 02-1a PlayerSession

```go
// game/player/session.go
type PlayerSession struct {
    PlayerID  int64
    AccountID int64
    CharID    int64
    Conn      *websocket.Conn
    MapID     int
    X, Y      int
    Dir       int           // RPG Maker方向：2下 4左 6右 8上
    SendChan  chan []byte   // 序列化后的 JSON，发送 goroutine 消费
    Done      chan struct{}
    TraceID   string        // 当前处理中消息的 trace-id
    LastSeq   uint64        // 最后处理的消息 seq（单调递增校验用）
    mu        sync.Mutex    // 保护可变字段（位置、MapID等）
}
```

**发送 goroutine**（每个 Session 独立）：
```go
func (s *PlayerSession) writePump() {
    for {
        select {
        case data := <-s.SendChan:
            s.Conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
            s.Conn.WriteMessage(websocket.TextMessage, data)
        case <-s.Done:
            return
        }
    }
}
```

**Session.Send 方法**：序列化 Packet 为 JSON，非阻塞投入 SendChan（chan 满则记录 warn 日志后丢弃，防止慢客户端阻塞服务端）。

### 02-1b WS 握手

使用 `github.com/gorilla/websocket` 或 `nhooyr.io/websocket`：

```
GET /ws?token=xxx
1. 从 URL query 取 token
2. 调用 jwt.ParseToken + Cache 验证 session
3. 验证失败 → HTTP 401 拒绝 Upgrade
4. 成功 → Upgrade 到 WebSocket，创建 PlayerSession，启动 readPump + writePump goroutine
5. 将 Session 注册到 SessionManager
```

### 02-1c 心跳

```
客户端每 30s 发 {"type":"ping","seq":N,"payload":{"ts": client_unix_ms}}
服务端：
  - 收到 ping → 更新 session 的 lastPing 时间，回复 {"type":"pong","payload":{"client_ts":...,"server_ts":...}}
  - readPump 设置 Read Deadline = 60s，超时自动触发 connection close error
  - readPump error → 执行下线流程（RemoveFromMap + broadcast player_leave + SavePosition）
```

### 02-2 消息路由器

```go
type Packet struct {
    Seq     uint64          `json:"seq"`
    Type    string          `json:"type"`
    Payload json.RawMessage `json:"payload"`
}

type HandlerFunc func(ctx context.Context, session *PlayerSession, payload json.RawMessage) error

type Router struct {
    handlers map[string]HandlerFunc
}

func (r *Router) On(msgType string, fn HandlerFunc) { ... }
func (r *Router) Dispatch(session *PlayerSession, raw []byte) {
    var pkt Packet
    json.Unmarshal(raw, &pkt)
    // seq 校验：pkt.Seq <= session.LastSeq → 拒绝（重放攻击）
    // 生成 TraceID，存入 session.TraceID
    // 调用 handlers[pkt.Type]
}
```

**注意**：Handler 注册在 `main.go` 的初始化阶段：
```go
router.On("enter_map",    world.HandleEnterMap)
router.On("player_move",  world.HandleMove)
router.On("ping",         session.HandlePing)
// M2+ 的 handler 留空或 TODO
```

### 02-4 MapRoom 游戏帧

```go
type MapRoom struct {
    MapID      int
    players    map[int64]*PlayerSession
    npcs       []*NPCInstance     // 后续 Task 填充
    monsters   []*MonsterInstance // 后续 Task 填充
    broadcastQ chan []byte
    mu         sync.RWMutex
    stopCh     chan struct{}
}

func (room *MapRoom) Run() {
    ticker := time.NewTicker(50 * time.Millisecond) // 20 TPS
    defer ticker.Stop()
    for {
        select {
        case <-ticker.C:
            room.Tick()
        case data := <-room.broadcastQ:
            room.mu.RLock()
            for _, s := range room.players {
                s.Send(data)
            }
            room.mu.RUnlock()
        case <-room.stopCh:
            return
        }
    }
}

func (room *MapRoom) Tick() {
    // 本阶段：只广播在线玩家的位置快照
    // M2 阶段再加：移动请求处理、AI tick、战斗帧
    room.broadcastPlayerStates()
}
```

**广播优化**：不要每帧都广播所有玩家的全量数据，只广播本帧有位置变化的玩家（`dirtyPlayers` set）。

### 02-6 HandleEnterMap

```go
func HandleEnterMap(ctx context.Context, session *PlayerSession, payload json.RawMessage) error {
    var req struct {
        MapID  int   `json:"map_id"`
        CharID int64 `json:"char_id"`
    }
    json.Unmarshal(payload, &req)

    // 1. 校验 CharID 属于当前 AccountID（防止操控他人角色）
    // 2. 从 DB 加载角色位置（或用 req 中的 map_id 覆盖）
    // 3. 若玩家已在某个 MapRoom，先执行离开逻辑
    // 4. 将 session 加入目标 MapRoom（WorldManager.GetOrCreate(mapID)）
    // 5. 推送 map_init（当前地图快照）
    // 6. 广播 player_join 给地图其他玩家
    // 7. 启动 3s 保护计时器（time.AfterFunc），到期发送 protection_end
}
```

**map_init payload**：
```json
{
  "players": [{"id": 1, "name": "...", "x": 5, "y": 5, "dir": 2, "hp": 100, ...}],
  "npcs": [],
  "monsters": [],
  "drops": []
}
```

### 02-7 HandleMove

```go
func HandleMove(ctx context.Context, session *PlayerSession, payload json.RawMessage) error {
    var req struct {
        X   int `json:"x"`
        Y   int `json:"y"`
        Dir int `json:"dir"`
        Seq uint64 `json:"seq"`
    }
    json.Unmarshal(payload, &req)

    // 1. 获取玩家当前位置
    // 2. 距离合法性：|dx|+|dy| <= maxSpeedPerTick * 1.3（基于50ms帧时间）
    // 3. 通行性：resource.Passability[mapID][req.Y][req.X][dir] == true
    // 4. 更新 session 位置
    // 5. 标记 session 为 dirty（由 Tick 批量广播）
    // 或者立即广播 player_sync（简单实现）
}
```

**移动速度**：RPG Maker MV 默认步速 = 4（每帧移动 4 像素），一个格子 = 48 像素，约每格 12 帧 = 600ms。服务端允许每帧最多移动 1 格（含 1.3 容错 = 1.3 格），避免网络延迟误判。

### 02-10 集成测试（测试用 WebSocket 客户端）

使用 Go 的 `net/http/httptest` + gorilla/websocket：

```go
func TestMultiPlayerSync(t *testing.T) {
    // 启动测试服务器
    // 同时连接 5 个 WS 客户端，各自 enter_map(mapID=1)
    // 客户端0 发送 player_move{x:6, y:5}
    // 断言其他4个客户端在 200ms 内收到 player_sync 包
}
```

---

## 消息格式参考

**player_sync（S2C）**：
```json
{
  "type": "player_sync",
  "payload": {
    "player_id": 20001,
    "x": 6, "y": 5, "dir": 6,
    "hp": 100, "mp": 50,
    "state": "normal"
  }
}
```

**player_join（S2C）**：
```json
{
  "type": "player_join",
  "payload": {
    "id": 20001,
    "name": "勇者小明",
    "class_id": 1,
    "level": 5,
    "x": 5, "y": 5, "dir": 2,
    "hp": 100, "max_hp": 120,
    "mp": 50, "max_mp": 60,
    "walk_name": "Actor1",
    "walk_index": 0,
    "buffs": []
  }
}
```

---

## 验收标准

1. WS 连接成功（Token 验证通过），无效 Token 返回 101 升级失败
2. 客户端发 `enter_map` 后收到 `map_init` + 3s 后收到 `protection_end`
3. 客户端发 `player_move` 后，同一地图其他客户端在 <100ms 内收到 `player_sync`
4. 客户端断线后其他客户端收到 `player_leave`
5. 移动越界（超过地图边界或通行性不通过）服务端拒绝，位置不更新
6. 集成测试 5 客户端并发测试通过
