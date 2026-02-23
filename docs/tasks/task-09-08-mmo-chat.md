# Task 09-08 - mmo-chat.js（聊天框）

> **优先级**：P1（M4）
> **里程碑**：M4
> **依赖**：task-09-01（mmo-core）

---

## 目标

实现底部左侧聊天框：多频道 Tab（全服/队伍/公会/战斗/系统/私聊）、频道颜色区分、消息输入与发送（Enter 键）、历史消息滚动（最多 100 条显示）。

---

## Todolist

- [ ] **08-1** `Window_ChatLog`（消息历史显示区）
  - [ ] 固定高度滚动区（约 120px，3-5 行消息）
  - [ ] 每条消息：`[频道] 发送者: 内容`，频道颜色编码
  - [ ] 新消息自动滚动到底部
  - [ ] 最多保留 100 条（先进先出）
- [ ] **08-2** 频道 Tab 切换
  - [ ] Tab 条：全/队/会/战/系/私（小图标或文字）
  - [ ] 选中 Tab 高亮，未读消息显示红点数字
  - [ ] 不同 Tab 显示对应频道历史
- [ ] **08-3** 消息输入框（HTML `<input>` 叠加）
  - [ ] Enter 发送消息
  - [ ] `/` 开头可切换频道前缀（`/all` `/party` `/guild` `/pm 玩家名`）
  - [ ] 输入框获取焦点时屏蔽 RMMV 键盘输入（防止移动/技能触发）
  - [ ] Escape 收起输入框
- [ ] **08-4** 频道消息路由（`chat_send` 发送）
  - [ ] 根据当前选中频道确定 `channel` 字段（world/party/guild/private）
  - [ ] 私聊频道附带 `target_id`（目标玩家名 → ID 映射由服务端处理）
- [ ] **08-5** 接收 `chat_recv` 消息
  - [ ] 追加到对应频道历史
  - [ ] 当前未显示该频道时：在其 Tab 显示未读红点
- [ ] **08-6** 系统消息显示（蓝色，服务端推送 `system_notice`）
- [ ] **08-7** 战斗频道自动填充（接收 `battle_result` 时自动生成战斗日志）

---

## 实现细节与思路

### 频道颜色表

```javascript
var CHANNEL_COLORS = {
    'world':   '#FFFFFF',  // 全服 - 白
    'party':   '#44FF88',  // 队伍 - 绿
    'guild':   '#FFD700',  // 公会 - 黄
    'battle':  '#FF6666',  // 战斗 - 红
    'system':  '#88AAFF',  // 系统 - 蓝
    'private': '#DD88FF',  // 私聊 - 紫
};
```

### 消息记录结构

```javascript
$MMO._chatHistory = {
    world:   [],
    party:   [],
    guild:   [],
    battle:  [],
    system:  [],
    private: {},   // { targetName: [...] }
};
```

### 输入框焦点处理

输入框获焦时禁用 RMMV 输入，失焦时恢复：

```javascript
chatInput.addEventListener('focus', function () {
    Input.clear();    // 清空 RMMV 输入缓冲
    $gameSystem._mmoInputFocused = true;
});
chatInput.addEventListener('blur', function () {
    $gameSystem._mmoInputFocused = false;
});

// Hook Input.isPressed，输入框激活时返回 false
var _Input_isPressed = Input.isPressed;
Input.isPressed = function (keyName) {
    if ($gameSystem._mmoInputFocused) return false;
    return _Input_isPressed.call(this, keyName);
};
```

### 战斗日志自动生成

```javascript
$MMO.on('battle_result', function (payload) {
    if (payload.attacker_id !== $MMO.charId && payload.target_id !== $MMO.charId) return;
    var msg = payload.attacker_id === $MMO.charId
        ? '对 [' + payload.target_name + '] 造成 ' + payload.damage + ' 点伤害' + (payload.is_crit ? '（暴击）' : '')
        : '受到 [' + payload.attacker_name + '] 的 ' + payload.damage + ' 点伤害';
    $MMO._addChatMessage('battle', { from_name: '战斗', content: msg, ts: Date.now() });
});
```

---

## 验收标准

1. 底部显示聊天框（含频道 Tab 和消息历史）
2. 输入消息 + Enter → `chat_recv` 消息从服务端回来后显示在历史中
3. 不同频道 Tab 显示对应颜色的消息
4. 收到非当前 Tab 的消息时 Tab 显示未读红点
5. 输入框获焦时角色不移动（键盘输入被屏蔽）
6. Escape 收起输入框，恢复正常游戏操作
