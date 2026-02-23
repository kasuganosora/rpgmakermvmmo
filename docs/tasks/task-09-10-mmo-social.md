# Task 09-10 - mmo-social.js（好友与公会 UI）

> **优先级**：P2（M4）
> **里程碑**：M4
> **依赖**：task-09-01（mmo-core）、task-09-05（mmo-hud，Alt+F/Alt+G 触发）

---

## 目标

实现好友列表面板（在线状态、私聊、删除）和公会信息面板（成员列表、公告、公会等级）。

---

## Todolist

- [ ] **10-1** `Window_FriendList`（好友列表）
  - [ ] 每个好友一行：昵称 + 在线状态指示（绿点/灰点）+ 等级 + 地图名
  - [ ] 在线好友排前，离线好友排后并置灰
  - [ ] 右键菜单：私聊 / 组队邀请 / 删除好友
  - [ ] 好友申请通知（服务端推送 `friend_request`）→ 弹出接受/拒绝对话框
- [ ] **10-2** `Window_GuildInfo`（公会面板）
  - [ ] 公会名称 + 等级 + 当前金币
  - [ ] 公告栏（可滚动文本）
  - [ ] 成员列表（昵称/职位/等级/在线状态）
  - [ ] 会长/副会长专属按钮：修改公告 / 踢出成员 / 管理申请
- [ ] **10-3** REST API 调用
  - [ ] `GET /api/social/friends` → 填充好友列表（含在线状态）
  - [ ] `GET /api/guilds/:id` → 填充公会信息
  - [ ] Alt+F 打开好友面板时触发加载
  - [ ] Alt+G 打开公会面板时触发加载
- [ ] **10-4** 好友在线/离线实时通知（WS 推送 `friend_online` / `friend_offline`）
  - [ ] 收到通知时更新列表中对应好友的在线状态

---

## 实现细节与思路

### 好友列表加载

```javascript
Window_FriendList.prototype.loadFriends = function () {
    var self = this;
    $MMO.http.get('/api/social/friends').then(function (data) {
        // 在线好友排前（online: true）
        data.friends.sort(function (a, b) { return (b.online ? 1 : 0) - (a.online ? 1 : 0); });
        self._friends = data.friends;
        self.refresh();
    });
};
```

### 好友申请通知

```javascript
$MMO.on('friend_request', function (payload) {
    // 弹出通知对话框（非阻塞，右下角 toast 风格）
    $MMO._showToast(payload.from_name + ' 申请添加你为好友', {
        onAccept: function () {
            $MMO.http.post('/api/social/friends/accept/' + payload.from_id, {});
        },
        onDecline: function () { /* 拒绝不需要发送请求 */ },
        timeout: 20000,
    });
});
```

---

## 验收标准

1. Alt+F 打开好友列表，在线好友显示绿点在线状态
2. 点击私聊 → 聊天框自动切换到私聊频道并填充目标名称
3. Alt+G 打开公会面板，显示成员列表和公告
4. 收到好友申请时右下角弹出通知，可接受/拒绝
5. 好友上线/下线时列表状态实时更新
