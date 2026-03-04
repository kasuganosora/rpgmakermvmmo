# Issue: Move Route 期间 player_move 被 EventMu 拒绝导致循环

## 现象

事件执行期间，服务器发送 SetMoveRoute（code 205, wait:true）让玩家角色移动。
客户端执行移动路线后，`Game_Player.moveStraight` 触发 `player_move` 消息发送到服务器。
但此时 EventMu 被事件锁持有，HandleMove 的 TryLock 失败，返回 `move_reject`。
客户端收到 move_reject 后回弹到原位，移动路线再次尝试 → 无限循环。

## 客户端日志重现

```
<- npc_effect {code: 205, indent: 0, params: Array(2), wait: true}   // 服务器: 移动路线(wait)
-> player_move {x: 2, y: 17, dir: 6}                                  // 客户端: 移动路线触发的移动
<- move_reject {dir: 6, x: 1, y: 17}                                  // 服务器: EventMu锁定,拒绝
[MMO] 移动被拒绝 — 回弹到服务器位置: 1 17 方向 6
-> player_move {x: 2, y: 17, dir: 6}                                  // 再次尝试...
<- move_reject {dir: 6, x: 1, y: 17}                                  // 再次拒绝...
(重复 5 次)
-> npc_effect_ack {}                                                    // 最终移动路线完成(?)
```

## 根本原因

1. **EventMu 事件锁**：HandleMove 中 `s.EventMu.TryLock()` 失败时发送 move_reject，
   但没有区分「玩家主动点击移动」和「服务器移动路线触发的移动」。
2. **客户端移动路线执行**：RMMV 的 SetMoveRoute 通过 `Game_Player.moveStraight()` 移动角色，
   而 mmo-core.js 在 `moveStraight` 中发送 `player_move` 到服务器。
3. **两者冲突**：事件执行中（EventMu 锁定），移动路线本身需要玩家移动，但移动被拒绝。

## 解决方案

### 方案 A：客户端区分移动来源（推荐）

在 `Game_Player.moveStraight` 的服务器同步逻辑中，检查移动是否来自移动路线：

```javascript
// mmo-core.js 或 mmo-npc.js 中的 moveStraight override
var _orig_moveStraight = Game_Player.prototype.moveStraight;
Game_Player.prototype.moveStraight = function(d) {
    _orig_moveStraight.call(this, d);
    // 移动路线触发的移动不需要发送 player_move — 服务器已知道
    if (this._moveRouteForcing) return;
    // 正常发送 player_move...
};
```

`_moveRouteForcing` 是 RMMV `Game_Character` 的内置属性，在 SetMoveRoute 执行时为 true。

### 方案 B：服务器端接受事件中的移动

在 HandleMove 中，不使用 EventMu 检查，改为通过其他方式（如标志位）区分：

```go
// 事件中但允许移动路线产生的移动
if !s.EventMu.TryLock() {
    if !s.MoveRouteActive {
        sendMoveReject(s)
        return nil
    }
    // 移动路线移动 — 更新位置但不释放锁
}
```

### 方案 C：服务器端直接更新位置

在 `sendEffectWait(cmd 205)` 时，服务器解析移动路线参数，直接更新玩家位置，
不依赖客户端 `player_move` 反馈。

## 影响范围

- 所有包含 SetMoveRoute（code 205）且 target=player 的事件指令
- 主要影响剧情演出中的玩家移动（如开场剧情中的走路动画）
- 不影响 NPC 的移动路线（NPC 移动不触发 player_move）

## 临时缓解

移动路线最终完成后 npc_effect_ack 会发送，事件继续执行。
但期间有 5+ 次无效的 move_reject 往返，造成短暂的位置闪烁和网络浪费。
