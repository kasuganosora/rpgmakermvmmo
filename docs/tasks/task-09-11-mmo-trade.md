# Task 09-11 - mmo-trade.js（交易系统 UI）

> **优先级**：P2（M4）
> **里程碑**：M4
> **依赖**：task-09-01（mmo-core）、task-09-07（mmo-inventory，物品数据）

---

## 目标

实现玩家交易窗口：双列显示我方/对方放入的物品和金币，双方均确认后完成交易。接收服务端 `trade_update` 实时同步对方操作。

---

## Todolist

- [ ] **11-1** 交易请求发起
  - [ ] 右键其他玩家精灵 → 弹出菜单（邀请组队/私聊/**发起交易**）
  - [ ] 选择交易 → `trade_request{target_player_id}`
- [ ] **11-2** 收到交易邀请（服务端 `trade_invite` 推送）
  - [ ] 弹出确认对话框：`[A] 邀请你交易，是否接受？`
  - [ ] 接受 → `trade_accept`，打开交易窗口
- [ ] **11-3** `Window_Trade`（交易窗口，全屏弹窗）
  - [ ] 左列：我方放入（物品列表 + 金币输入框）
  - [ ] 右列：对方放入（只读，显示对方实时操作）
  - [ ] 底部：`确认` / `取消` 按钮
  - [ ] 双方都点确认后交易完成，窗口关闭
  - [ ] 一方取消时双方窗口均关闭
- [ ] **11-4** 我方放入物品
  - [ ] 从背包拖拽或点击物品 → 弹出数量选择 → 加入交易列表
  - [ ] 取消放入：在交易列表中选择移除
  - [ ] 金币输入框：输入数量
  - [ ] 任何修改 → `trade_offer{items[], gold}` 发送给服务端
- [ ] **11-5** 接收 `trade_update` 消息
  - [ ] 更新对方放入物品列表（右列）
  - [ ] `phase` 字段：`negotiating` / `confirming` / `done` / `cancelled`
  - [ ] `confirmed` 字段：显示双方确认状态（✓ 标记）
- [ ] **11-6** 交易完成处理
  - [ ] 收到 `trade_update{phase: "done"}` → 显示交易完成动画，刷新背包
  - [ ] 收到 `trade_update{phase: "cancelled"}` → 显示取消提示，关闭窗口

---

## 实现细节与思路

### trade_update 处理

```javascript
$MMO.on('trade_update', function (payload) {
    if (!$MMO._tradeWindow) return;

    switch (payload.phase) {
    case 'negotiating':
        $MMO._tradeWindow.updateTheirOffer(payload.their_offer);
        $MMO._tradeWindow.updateConfirmStatus(payload.confirmed);
        break;
    case 'done':
        $MMO._tradeWindow.close();
        // inventory_update 消息会随后推送，刷新背包
        SoundManager.playSystemSound(1);   // 交易音效
        break;
    case 'cancelled':
        $MMO._tradeWindow.close();
        $MMO._showToast('交易已取消');
        break;
    }
});
```

### 确认状态显示

```
我方确认：✓  对方确认：...
→ 对方也确认后：服务端执行交易，推送 phase: "done"
```

---

## 验收标准

1. 右键其他玩家 → 菜单有"发起交易"选项
2. 对方接受后双方同时打开交易窗口
3. 我方放入物品/金币 → 对方窗口实时更新
4. 双方确认 → 窗口关闭，双方背包内容互换（inventory_update 推送）
5. 一方取消 → 双方窗口均关闭并提示
