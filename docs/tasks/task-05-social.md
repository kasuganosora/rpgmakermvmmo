# Task 05 - 社交系统（Chat, Party, Friends, Guild）

> **优先级**：P2 — M4 里程碑
> **里程碑**：M4（组队共享经验，公会聊天正常）
> **依赖**：Task 02（WS 消息路由）、Task 00（Cache/PubSub）

---

## 目标

实现多频道聊天（全服/队伍/公会/战斗/系统/私聊）、组队系统（邀请/踢人/转让/共享经验）、好友/黑名单系统、公会系统（创建/加入/管理），以及相关 REST API。

---

## Todolist

- [ ] **05-1** 实现聊天系统（`game/chat/`）
  - [ ] 05-1a `HandleChatSend` — 处理 `chat_send` 消息
  - [ ] 05-1b 全服频道：Cache `chat:world` List 存最近 200 条，PubSub 广播所有在线玩家
  - [ ] 05-1c 队伍频道：只发给同队成员（通过 SessionManager 查找）
  - [ ] 05-1d 公会频道：发给所有在线公会成员（PubSub channel `guild:{guild_id}`）
  - [ ] 05-1e 私聊频道：通过 SessionManager 找目标在线玩家，发送 `chat_recv`
  - [ ] 05-1f 战斗频道：仅在战斗结算时由服务端推送（伤害/技能日志），玩家不能主动发送
  - [ ] 05-1g 系统频道：GM 通过 Admin API 触发，SSE 广播全服公告
  - [ ] 05-1h 内容校验（长度限制 200 字，敏感词过滤占位）
  - [ ] 05-1i Hook：`on_chat_send`（可拦截消息，GM 指令处理占位）
- [ ] **05-2** 实现组队系统（`game/party/`）
  - [ ] 05-2a `HandlePartyInvite` — 邀请组队
  - [ ] 05-2b `HandlePartyLeave` — 离开队伍
  - [ ] 05-2c `HandlePartyKick` — 踢出队员（仅队长）
  - [ ] 05-2d 队伍状态管理（内存 `PartyManager`，Redis `party:{party_id}` Hash 备份）
  - [ ] 05-2e 队伍状态广播（每 1s 推送 `party_update`，含每个成员 HP/MP/Buff/地图）
  - [ ] 05-2f 接受/拒绝邀请的消息往返流程
  - [ ] 05-2g REST：`GET /api/party`（查询当前组队状态）
- [ ] **05-3** 实现好友/黑名单系统（`game/social/`）
  - [ ] 05-3a `POST /api/social/friends/request` — 发送好友申请
  - [ ] 05-3b 申请通知（目标在线时 WS 推送，离线时邮件）
  - [ ] 05-3c `POST /api/social/friends/accept/:id` — 接受申请
  - [ ] 05-3d `DELETE /api/social/friends/:id` — 删除好友
  - [ ] 05-3e `POST /api/social/block/:id` — 拉黑（status=2），拉黑后无法私聊/组队
  - [ ] 05-3f `GET /api/social/friends` — 好友列表（含在线状态）
- [ ] **05-4** 实现公会系统（`game/guild/`）
  - [ ] 05-4a `POST /api/guilds` — 创建公会（消耗金币，名字唯一）
  - [ ] 05-4b `GET /api/guilds/:id` — 公会详情（成员列表、公告、等级）
  - [ ] 05-4c `POST /api/guilds/:id/join` — 申请加入（会长同意后生效）
  - [ ] 05-4d `DELETE /api/guilds/:id/members/:cid` — 踢出成员（会长/副会长）
  - [ ] 05-4e 公会等级提升（公会金币达到阈值，可手动升级）
  - [ ] 05-4f 公会公告修改（PUT /api/guilds/:id/notice）
  - [ ] 05-4g 离线公会产出（调度任务 `offline_income`，Task 07 接入）
- [ ] **05-5** 编写单元测试
  - [ ] chat_test.go：消息路由到正确频道
  - [ ] party_test.go：组队邀请→接受→经验加成→踢人→解散
  - [ ] guild_test.go：创建→加入→踢人

---

## 实现细节与思路

### 05-1 聊天系统

**全服聊天流程**：
```
玩家A 发 chat_send{channel:"world", content:"你好"}
1. 内容校验（长度、敏感词）
2. 调用 on_chat_send Hook
3. 构造 chat_recv 消息：{channel:"world", from_name:"A", content:"你好", ts:unix_ms}
4. PubSub.Publish("chat:world", json(chat_recv))
   → 订阅 "chat:world" 的所有在线玩家收到消息（每个玩家 readPump goroutine 订阅）
5. Cache.LPush("chat:world", json(chat_recv)) + LTrim("chat:world", 0, 199)
   → 保留最近 200 条，供新玩家上线时回放历史
```

**玩家上线时拉取聊天历史**（在 `enter_map` 处理中）：
```go
history, _ := cache.LRange(ctx, "chat:world", 0, 49)  // 最近 50 条
for _, msg := range history {
    session.Send(msg)
}
```

**PubSub 订阅管理**：
- 玩家连接时订阅：`chat:world`（始终）
- 加入队伍时订阅：`party:{party_id}`
- 加入公会时订阅：`guild:{guild_id}`
- 断线/离队/离会时取消订阅

### 05-2 组队系统

**组队状态（内存 `Party` struct）**：
```go
type Party struct {
    ID       int64
    LeaderID int64
    Members  []*PlayerSession  // 最多4人
    mu       sync.RWMutex
}
```

**邀请流程**：
```
A 发 party_invite{target_player_id: B_id}
1. 校验A没有被邀请CD（防刷屏）
2. 查 SessionManager 找 B 的 session
3. B 不在线 → 返回错误 "玩家不在线"
4. B 已在队伍 → 返回错误 "玩家已有队伍"
5. 向 B 发 WS 消息：party_invite_request{from_name:"A", party_id:...}
6. 等待 B 的响应（30s 超时）

B 发 party_invite_response{accept: true}
1. 校验邀请是否有效（未过期）
2. B 加入 Party，Party.Members 增加
3. 推送 party_update 给所有成员
4. B 订阅 PubSub "party:{party_id}"
```

**party_update 广播**（每 1s 由 Ticker 触发，只有队伍成员变化时立即触发）：
```json
{
  "type": "party_update",
  "payload": {
    "party_id": 100,
    "leader_id": 20001,
    "members": [
      {"char_id": 20001, "name": "Alice", "class_id": 1, "hp": 80, "max_hp": 120,
       "mp": 40, "max_mp": 60, "buffs": [], "map_id": 3, "online": true},
      {"char_id": 20002, "name": "Bob",   "class_id": 2, "hp": 100, "max_hp": 100, ...}
    ]
  }
}
```

**经验共享**（修改 Task 03 的 Task 03-7 怪物死亡结算）：
```go
// 找在场队员：同地图 + 视野范围（距怪物 <= 12格）
func (party *Party) GetNearbyMembers(mapID int, x, y int) []*PlayerSession {
    party.mu.RLock()
    defer party.mu.RUnlock()
    var result []*PlayerSession
    for _, m := range party.Members {
        if m.MapID == mapID && distance(m.X, m.Y, x, y) <= 12 {
            result = append(result, m)
        }
    }
    return result
}
```

### 05-3 好友系统

好友数据存 DB `friendships` 表，online 状态从 `SessionManager` 实时查询：

```go
// GET /api/social/friends
func (h *SocialHandler) ListFriends(c *gin.Context) {
    charID := getCharID(c)
    friends, _ := h.db.Where("char_id = ? AND status = 1", charID).Find(&model.Friendship{})
    for _, f := range friends {
        f.Online = h.sessionMgr.IsOnline(f.FriendID)
    }
    c.JSON(200, friends)
}
```

**私聊防拉黑**：发送私聊前检查目标是否在双方的黑名单中：
```go
func isBlocked(db *gorm.DB, senderID, targetID int64) bool {
    var count int64
    db.Model(&model.Friendship{}).
        Where("(char_id = ? AND friend_id = ? AND status = 2) OR (char_id = ? AND friend_id = ? AND status = 2)",
            senderID, targetID, targetID, senderID).Count(&count)
    return count > 0
}
```

### 05-4 公会系统

**公会创建费用**：`config.Game.GuildCreateCost`（默认 10000 金）。

**公会频道聊天**：
- 所有在线公会成员在登录时订阅 `guild:{guild_id}` PubSub channel
- 公会聊天直接 Publish 到该 channel

**公会成员上线通知**（在 WS 连接成功后）：
```go
// 查玩家的 guild_id
// PubSub.Publish("guild:{guild_id}", json({type:"guild_member_online", name:...}))
```

---

## 路由注册（补充）

```go
// 社交 REST（需要 Auth）
social := r.Group("/api/social", middleware.Auth(cfg.Security))
{
    social.GET("/friends",             socialHandler.ListFriends)
    social.POST("/friends/request",    socialHandler.SendFriendRequest)
    social.POST("/friends/accept/:id", socialHandler.AcceptFriendRequest)
    social.DELETE("/friends/:id",      socialHandler.DeleteFriend)
    social.POST("/block/:id",          socialHandler.BlockPlayer)
}

// 公会 REST
guilds := r.Group("/api/guilds", middleware.Auth(cfg.Security))
{
    guilds.POST("",                   guildHandler.Create)
    guilds.GET("/:id",                guildHandler.Detail)
    guilds.POST("/:id/join",          guildHandler.Join)
    guilds.DELETE("/:id/members/:cid",guildHandler.KickMember)
    guilds.PUT("/:id/notice",         guildHandler.UpdateNotice)
}
```

WS 路由（在 `main.go` 补充注册）：
```go
router.On("party_invite",  party.HandleInvite)
router.On("party_leave",   party.HandleLeave)
router.On("party_kick",    party.HandleKick)
router.On("party_invite_response", party.HandleInviteResponse)
router.On("chat_send",     chat.HandleSend)
```

---

## 验收标准

1. 全服聊天消息在 <500ms 内送达所有在线玩家
2. 组队邀请→接受→组队成功，队内 HP/MP 实时同步（party_update 每秒推送）
3. 击杀怪物后组队经验加成正确（1个队友 = ×1.1，3个队友 = ×1.3）
4. 好友申请→接受→好友列表显示在线状态
5. 公会聊天只有同公会成员收到
6. 私聊被拉黑时发送失败返回错误
7. 单元测试通过
