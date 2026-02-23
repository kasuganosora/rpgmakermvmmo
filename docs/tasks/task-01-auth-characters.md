# Task 01 - 认证与角色管理（Auth & Character Management）

> **优先级**：P1 — M1 里程碑第一部分
> **里程碑**：M1（5个客户端同时在线，互相看到对方移动）
> **依赖**：Task 00（DB 适配层、Model、Cache 适配层）

---

## 目标

实现玩家注册/登录（JWT）、角色 CRUD（创建/列表/删除），以及 Gin 中间件（认证、限流、TraceID、日志）。完成后玩家可以从客户端登录并选择/创建角色。

---

## Todolist

- [ ] **01-1** 实现 Gin 中间件
  - [ ] 01-1a `middleware/traceid.go` — 每请求生成 UUID TraceID，注入 context
  - [ ] 01-1b `middleware/logger.go` — zap 结构化访问日志（method, path, status, duration_ms, trace_id）
  - [ ] 01-1c `middleware/recovery.go` — panic 捕获，返回 500 + 记录错误日志
  - [ ] 01-1d `middleware/auth.go` — JWT Token 验证（REST），从 Header `Authorization: Bearer xxx`
  - [ ] 01-1e `middleware/ratelimit.go` — IP 级别令牌桶限流（WebSocket 限流在 WS 层做，REST 也需要）
- [ ] **01-2** 实现认证服务（`api/rest/auth.go`）
  - [ ] 01-2a `POST /api/auth/login` — 用户名不存在时自动注册
  - [ ] 01-2b `POST /api/auth/logout` — 使 Token 失效（删除 Cache 中的 session）
  - [ ] 01-2c `POST /api/auth/refresh` — 刷新 JWT Token
- [ ] **01-3** 实现 JWT 工具（`middleware/jwt.go`）
  - [ ] 生成 Token（HS256，payload: account_id, char_id, exp）
  - [ ] 验证 Token + 从 Cache 查 Session 有效性
  - [ ] Session 存入 Cache：`session:{token}` → Hash{account_id, char_id}，TTL = jwt_ttl_h
- [ ] **01-4** 实现角色管理 REST API（`api/rest/character.go`）
  - [ ] 01-4a `GET /api/characters` — 返回当前账号的角色列表（最多3个）
  - [ ] 01-4b `POST /api/characters` — 创建角色（校验名字唯一、职业合法、素材合法）
  - [ ] 01-4c `DELETE /api/characters/:id` — 删除角色（需密码二次确认）
- [ ] **01-5** 编写单元测试
  - [ ] auth_test.go：注册→登录→Token 验证→登出
  - [ ] character_test.go：创建角色、重名校验、角色列表、删除角色

---

## 实现细节与思路

### 01-2a POST /api/auth/login

```
请求体：{"username": "alice", "password": "pass123"}

逻辑：
1. 查 accounts 表 WHERE username = ?
2. 不存在 → 自动注册（bcrypt.GenerateFromPassword(cost=12)，INSERT account）
3. 存在 → bcrypt.CompareHashAndPassword 验证密码
4. 密码错误 → 返回 401
5. 账号 status=0（封禁）→ 返回 403
6. 生成 JWT Token（HS256，含 account_id，exp = now + jwt_ttl_h）
7. 将 session 写入 Cache：
   cache.HSet(ctx, "session:"+token, "account_id", strconv.FormatInt(accountID, 10))
   cache.Expire(ctx, "session:"+token, jwt_ttl_h)
8. 更新 accounts.last_login_at 和 last_login_ip
9. 返回 {"token": "...", "account_id": 10001}

注意：用户名不存在时自动注册的逻辑需要防止并发注册同名（数据库 UNIQUE KEY 兜底，
      捕获唯一键冲突错误返回"用户名已被注册"而不是服务器错误）
```

### 01-3 JWT 工具

使用 `github.com/golang-jwt/jwt/v5`：

```go
type Claims struct {
    AccountID int64 `json:"account_id"`
    jwt.RegisteredClaims
}

func GenerateToken(accountID int64, secret string, ttl time.Duration) (string, error)
func ParseToken(tokenStr string, secret string) (*Claims, error)
```

**双重验证**：
1. JWT 签名有效（防篡改）
2. Cache 中 `session:{token}` 存在（防已登出的 Token 重用）

`middleware/auth.go` 从 `Authorization: Bearer xxx` 提取 Token，验证后将 `AccountID` 存入 `gin.Context`（`c.Set("account_id", claims.AccountID)`）。

### 01-4b POST /api/characters（创建角色）

```
请求体：{
    "name":       "勇者小明",
    "class_id":   1,
    "walk_name":  "Actor1",
    "walk_index": 0,
    "face_name":  "Actor1",
    "face_index": 0
}

校验：
1. 账号下角色数 < 3（否则返回 400）
2. name 唯一性（characters 表 UNIQUE KEY，捕获冲突）
3. class_id 在 ResourceLoader.Classes 中存在
4. walk_name 对应的文件在 img/characters/ 下存在（字符串白名单，防路径遍历）
5. face_name 对应文件在 img/faces/ 下存在

角色初始属性来源于：
- ResourceLoader.Actors[classId].initialLevel = 1
- ResourceLoader.Classes[classId].params[level][atk/def/...] → 计算初始属性值
- 起始位置来自 config.Game.StartMapId / StartX / StartY

INSERT character，返回角色基本信息
```

### 01-5 单元测试

使用 `testutil.SetupTestDB()` + `testutil.SetupTestCache()`：

```go
func TestLoginAutoRegister(t *testing.T) {
    // 首次登录用户名不存在 → 自动注册 + 返回 Token
}

func TestLoginWrongPassword(t *testing.T) {
    // 密码错误 → 401
}

func TestCreateCharacter(t *testing.T) {
    // 正常创建 → 返回角色数据
    // 重名 → 400
    // 超过3个 → 400
    // class_id 不存在 → 400
}
```

---

## 路由注册

在 `main.go` 中注册：

```go
auth := r.Group("/api/auth")
{
    auth.POST("/login",   authHandler.Login)
    auth.POST("/logout",  authHandler.Logout)   // 需要 Auth 中间件
    auth.POST("/refresh", authHandler.Refresh)  // 需要 Auth 中间件
}

chars := r.Group("/api/characters", middleware.Auth(cfg.Security))
{
    chars.GET("",        charHandler.List)
    chars.POST("",       charHandler.Create)
    chars.DELETE("/:id", charHandler.Delete)
}
```

---

## 验收标准

1. `POST /api/auth/login` 首次调用自动注册并返回 JWT Token
2. Token 写入 Cache，`POST /api/auth/logout` 后 Token 失效
3. `GET /api/characters` 返回当前账号角色列表（JWT 验证）
4. `POST /api/characters` 创建角色，重名返回 400
5. 单元测试全部通过（零外部依赖）
