# Task 09-07 - mmo-inventory.js（背包与装备 UI）

> **优先级**：P1（M3）
> **里程碑**：M3
> **依赖**：task-09-01（mmo-core）、task-09-05（mmo-hud，Alt+I 按钮触发）

---

## 目标

实现背包窗口（Grid 布局，显示物品图标/数量）和装备栏（槽位式）。支持物品使用、装备穿戴/卸下、物品丢弃操作。接收服务端 `inventory_update` 实时刷新。

---

## Todolist

- [ ] **07-1** `Scene_MMO_Inventory`（背包+装备弹出场景/窗口）
  - [ ] `Window_ItemGrid`：Grid 布局物品格（6列 × N行，每格 48×48px）
  - [ ] 格子内容：物品图标（`img/icons/` IconSet）+ 数量角标
  - [ ] 鼠标悬停/选中显示物品名称/说明 tooltip
  - [ ] 右键菜单：使用 / 装备 / 丢弃（弃置数量输入）
- [ ] **07-2** `Window_EquipPanel`（装备槽位，6-12 个槽）
  - [ ] 槽位标签（头/身/手/脚/武器/副手/饰品等，来自 RMMV etypeId）
  - [ ] 点击槽位 → 弹出可装备物品选择列表
  - [ ] 卸下装备：点击已装备槽位 → 确认后发送 `equip_item{item_id, equip_slot: 0}`
- [ ] **07-3** 物品操作消息处理
  - [ ] 使用物品 → `player_item{item_id, target_id: self}`
  - [ ] 装备物品 → `equip_item{item_id, equip_slot}`
  - [ ] 丢弃物品 → `drop_item{item_id, quantity}`（需二次确认）
- [ ] **07-4** `inventory_update` 消息处理（实时刷新）
  - [ ] `add[]`：将新物品追加到对应格子（或增加数量）
  - [ ] `remove[]`：减少数量/移除格子
  - [ ] `update[]`：更新格子数量（装备状态变更）
- [ ] **07-5** 初始背包数据加载
  - [ ] 角色选择后调用 `GET /api/characters/:id/inventory` 拉取完整背包
  - [ ] 存储到 `$MMO._inventory`（内存，实时 update 维护）

---

## 实现细节与思路

### 数据模型

```javascript
// $MMO._inventory 内存结构
$MMO._inventory = {
    items: [],    // [{slot, item_type, item_id, quantity, equip_slot}]
};

// 物品类型
var ITEM_TYPE = { ITEM: 1, WEAPON: 2, ARMOR: 3 };
```

### inventory_update 处理

```javascript
$MMO.on('inventory_update', function (payload) {
    var inv = $MMO._inventory;

    (payload.add || []).forEach(function (entry) {
        var existing = inv.items.find(function (i) {
            return i.item_type === entry.item_type && i.item_id === entry.item_id && i.equip_slot === 0;
        });
        if (existing && entry.item_type === 1) {  // 物品可堆叠
            existing.quantity += entry.quantity;
        } else {
            inv.items.push(entry);
        }
    });

    (payload.remove || []).forEach(function (entry) {
        var idx = inv.items.findIndex(function (i) { return i.slot === entry.slot; });
        if (idx >= 0) {
            inv.items[idx].quantity -= entry.quantity;
            if (inv.items[idx].quantity <= 0) inv.items.splice(idx, 1);
        }
    });

    (payload.update || []).forEach(function (entry) {
        var item = inv.items.find(function (i) { return i.slot === entry.slot; });
        if (item) Object.assign(item, entry);
    });

    // 刷新界面（如果背包窗口当前打开）
    if ($MMO._inventoryWindow && $MMO._inventoryWindow.isOpen()) {
        $MMO._inventoryWindow.refresh();
    }
});
```

### equip_result 处理

```javascript
$MMO.on('equip_result', function (payload) {
    if (!payload.success) {
        // 显示错误提示
        return;
    }
    // 更新本地属性显示（payload.char_stats）
    $MMO._charStats = payload.char_stats;
});
```

---

## 验收标准

1. Alt+I 打开背包，显示所有物品图标和数量
2. 右键物品 → 使用/装备/丢弃菜单正确响应
3. 装备槽位正确显示已装备物品，卸下后返回背包
4. `inventory_update` 后背包内容实时刷新（无需关闭重开）
5. 丢弃有确认弹窗，确认后物品消失
