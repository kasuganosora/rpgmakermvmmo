/*:
 * @plugindesc v4.0.0 MMO 背包系统 — 物品栏/装备管理（GameWindow 扩展）。
 * @author MMO Framework
 *
 * @help
 * 本插件实现背包物品管理窗口（继承 GameWindow 浮动窗口）：
 *
 * 功能特性：
 * - 5 列 × 6 行物品格子，图标来自 RMMV IconSet.png
 * - 物品类型：道具（ITEM=1）、武器（WEAPON=2）、防具（ARMOR=3）
 * - 右键菜单：装备/卸下/使用/取消
 * - 装备中物品显示 "E" 标记
 * - 悬停显示物品名称和装备槽位
 * - 消耗品支持拖放到技能快捷栏
 * - Alt+I 快捷键切换显示/隐藏
 * - 通过 REST API 加载完整背包列表
 * - 增量更新通过 WebSocket 推送
 *
 * 全局引用：
 * - window.ITEM_TYPE — 物品类型常量 {ITEM, WEAPON, ARMOR}
 * - $MMO._inventory — 背包数据 {items: [...]}
 * - $MMO._inventoryWindow — 背包窗口实例
 *
 * WebSocket 消息：
 * - inventory_update — 增量物品更新（add/remove/update）
 * - equip_result — 装备/卸下操作结果
 * - equip_item/unequip_item/use_item（发送）— 操作请求
 */

(function () {
    'use strict';

    // ═══════════════════════════════════════════════════════════
    //  全局状态与常量
    // ═══════════════════════════════════════════════════════════
    /** @type {Object} 背包数据，items 为物品数组。 */
    $MMO._inventory = { items: [] };

    /** @type {Object} 物品类型枚举常量。 */
    var ITEM_TYPE = { ITEM: 1, WEAPON: 2, ARMOR: 3 };

    /** @type {number} 格子列数。 */
    var COLS = 5;
    /** @type {number} 每格像素尺寸。 */
    var SLOT_SIZE = 40;
    /** @type {number} IconSet.png 每行图标列数。 */
    var ICON_COLS = 16;
    /** @type {number} 格子区域内边距。 */
    var GRID_PAD = 6;
    /** @type {number} GameWindow 标题栏高度。 */
    var GW_TITLE_H = 26;
    /** @type {number} 窗口宽度。 */
    var WIN_W = COLS * SLOT_SIZE + GRID_PAD * 2;
    /** @type {number} 格子行数。 */
    var ROWS = 6;
    /** @type {number} 窗口高度。 */
    var WIN_H = ROWS * SLOT_SIZE + GRID_PAD * 2 + GW_TITLE_H;

    /** @type {Object} 装备槽位中文名称映射。 */
    var EQUIP_SLOT_NAMES = {
        0: '武器', 1: '盾牌', 2: '头盔', 3: '铠甲', 4: '饰品'
    };

    // ═══════════════════════════════════════════════════════════
    //  RMMV 数据查找
    //  服务器存储 item_id + kind，客户端从 RMMV 数据文件查找
    //  名称、图标索引、描述等本地化信息。
    // ═══════════════════════════════════════════════════════════

    /**
     * 根据物品 ID 和类型从 RMMV 数据库查找本地信息。
     * @param {Object} item - 服务器物品数据 {item_id, kind}
     * @returns {Object} {name, iconIndex, description}
     */
    function resolveItemData(item) {
        var db = null;
        if (item.kind === ITEM_TYPE.ITEM && typeof $dataItems !== 'undefined') {
            db = $dataItems[item.item_id];
        } else if (item.kind === ITEM_TYPE.WEAPON && typeof $dataWeapons !== 'undefined') {
            db = $dataWeapons[item.item_id];
        } else if (item.kind === ITEM_TYPE.ARMOR && typeof $dataArmors !== 'undefined') {
            db = $dataArmors[item.item_id];
        }
        return {
            name: db ? db.name : ('Item #' + item.item_id),
            iconIndex: db ? db.iconIndex : 0,
            description: db ? db.description : ''
        };
    }

    // ═══════════════════════════════════════════════════════════
    //  InventoryWindow — 继承 GameWindow 的背包窗口
    //  5×6 物品格子 + 右键菜单 + 拖放支持。
    // ═══════════════════════════════════════════════════════════

    /**
     * 背包窗口构造函数。
     * 继承 GameWindow 获得标题栏、关闭按钮、拖拽移动功能。
     * @constructor
     */
    function InventoryWindow() {
        GameWindow.prototype.initialize.call(this, {
            key: 'inventory', title: '背包', width: WIN_W, height: WIN_H
        });
        /** @type {Bitmap|null} 缓存的 IconSet 位图。 */
        this._iconSet = null;
        /** @type {number} 当前悬停物品索引。 */
        this._hoverIdx = -1;
        /** @type {number} 当前选中物品索引。 */
        this._selectedIdx = -1;
        /** @type {Object|null} 右键菜单状态。 */
        this._contextMenu = null;
        var self = this;
        ImageManager.loadSystem('IconSet').addLoadListener(function (bmp) {
            self._iconSet = bmp;
            if (self.visible) self.refresh();
        });
    }
    InventoryWindow.prototype = Object.create(GameWindow.prototype);
    InventoryWindow.prototype.constructor = InventoryWindow;

    /**
     * 窗口打开时：从服务器加载背包数据，重置交互状态。
     */
    InventoryWindow.prototype.onOpen = function () {
        this._loadInventory();
        this._selectedIdx = -1;
        this._contextMenu = null;
        this._dragItemIdx = -1;
        this._dragActive = false;
        this.refresh();
    };

    /**
     * 关闭窗口时清理右键菜单和选中状态。
     */
    InventoryWindow.prototype.close = function () {
        this._contextMenu = null;
        this._selectedIdx = -1;
        GameWindow.prototype.close.call(this);
    };

    /**
     * 通过 REST API 加载完整背包列表。
     * @private
     */
    InventoryWindow.prototype._loadInventory = function () {
        if (!$MMO.charID) return;
        var self = this;
        $MMO.http.get('/api/characters/' + $MMO.charID + '/inventory')
            .then(function (data) {
                // 服务器返回 { inventory: [...] } 或 { items: [...] }。
                $MMO._inventory.items = data.inventory || data.items || [];
                if (self.visible) self.refresh();
            })
            .catch(function (e) { console.error('[mmo-inventory] 加载失败:', e.message); });
    };

    /**
     * 绘制背包内容：物品格子、图标、数量、装备标记、悬停提示、右键菜单。
     * 由 GameWindow 基类在 refresh() 中调用。
     * @param {Bitmap} c - 目标位图
     * @param {number} w - 内容区宽度
     * @param {number} h - 内容区高度
     */
    InventoryWindow.prototype.drawContent = function (c, w, h) {
        var topY = this.contentTop();
        var items = $MMO._inventory.items;
        var self = this;

        items.forEach(function (item, i) {
            var col = i % COLS;
            var row = Math.floor(i / COLS);
            var x = GRID_PAD + col * SLOT_SIZE;
            var y = topY + GRID_PAD + row * SLOT_SIZE;
            if (y + SLOT_SIZE > h) return;

            // 格子背景色（选中时高亮）。
            var isSelected = (i === self._selectedIdx);
            var bg = isSelected ? '#2A2A4E' : '#1A1A2E';
            c.fillRect(x, y, SLOT_SIZE - 2, SLOT_SIZE - 2, bg);
            var border = isSelected ? L2_Theme.textGold : L2_Theme.borderDark;
            L2_Theme.strokeRoundRect(c, x, y, SLOT_SIZE - 2, SLOT_SIZE - 2, 2, border);

            // 从 RMMV 数据库查找图标并绘制。
            var data = resolveItemData(item);
            if (self._iconSet && data.iconIndex > 0) {
                var sx = (data.iconIndex % ICON_COLS) * 32;
                var sy = Math.floor(data.iconIndex / ICON_COLS) * 32;
                c.blt(self._iconSet, sx, sy, 32, 32, x + 4, y + 2, 32, 32);
            }

            // 数量标签（仅数量 > 1 时显示）。
            if (item.qty > 1) {
                c.fontSize = 10;
                c.textColor = '#FFFF88';
                c.drawText(String(item.qty), x, y + SLOT_SIZE - 16, SLOT_SIZE - 6, 12, 'right');
            }

            // 装备中标记 "E"。
            if (item.equipped) {
                c.fontSize = 9;
                c.textColor = '#88FFFF';
                c.drawText('E', x + 3, y + 2, 12, 10, 'left');
            }
        });

        // 空格子轮廓。
        var totalSlots = Math.max(items.length, COLS * ROWS);
        for (var i = items.length; i < totalSlots; i++) {
            var col = i % COLS;
            var row = Math.floor(i / COLS);
            var ex = GRID_PAD + col * SLOT_SIZE;
            var ey = topY + GRID_PAD + row * SLOT_SIZE;
            if (ey + SLOT_SIZE > h) break;
            c.fillRect(ex, ey, SLOT_SIZE - 2, SLOT_SIZE - 2, '#111118');
            L2_Theme.strokeRoundRect(c, ex, ey, SLOT_SIZE - 2, SLOT_SIZE - 2, 2, '#1a1a2a');
        }

        // 悬停提示：底部显示物品名称和装备状态。
        if (self._hoverIdx >= 0 && self._hoverIdx < items.length) {
            var hItem = items[self._hoverIdx];
            var hData = resolveItemData(hItem);
            var tipText = hData.name;
            if (hItem.equipped) {
                var slotName = EQUIP_SLOT_NAMES[hItem.slot_index] || '';
                tipText += ' [' + slotName + '装备中]';
            }
            c.fontSize = 10;
            c.textColor = L2_Theme.textGold;
            c.drawText(tipText, GRID_PAD, h - 14, w - GRID_PAD * 2, 12, 'center');
        }

        // 绘制右键菜单覆盖层。
        if (self._contextMenu) {
            self._drawContextMenu(c);
        }
    };

    // ═══════════════════════════════════════════════════════════
    //  右键菜单（点击物品 → 装备/卸下/使用）
    // ═══════════════════════════════════════════════════════════
    /** @type {number} 菜单项高度。 */
    var CTX_ITEM_H = 22;
    /** @type {number} 菜单宽度。 */
    var CTX_W = 80;

    /**
     * 打开物品右键菜单。
     * 根据物品类型和状态生成菜单项：装备/卸下/使用/取消。
     * @param {number} itemIdx - 物品在背包中的索引
     */
    InventoryWindow.prototype._openContextMenu = function (itemIdx) {
        var item = $MMO._inventory.items[itemIdx];
        if (!item) return;
        var menuItems = [];
        if (item.equipped) {
            menuItems.push({ label: '卸下', action: 'unequip' });
        } else if (item.kind === ITEM_TYPE.WEAPON || item.kind === ITEM_TYPE.ARMOR) {
            menuItems.push({ label: '装备', action: 'equip' });
        }
        if (item.kind === ITEM_TYPE.ITEM) {
            menuItems.push({ label: '使用', action: 'use' });
        }
        menuItems.push({ label: '取消', action: 'cancel' });

        // 计算菜单位置（物品格子右侧，限制在窗口范围内）。
        var col = itemIdx % COLS;
        var row = Math.floor(itemIdx / COLS);
        var sx = GRID_PAD + col * SLOT_SIZE + SLOT_SIZE;
        var sy = this.contentTop() + GRID_PAD + row * SLOT_SIZE;

        this._contextMenu = {
            x: Math.min(sx, this.cw() - CTX_W - 4),
            y: Math.min(sy, this.ch() - menuItems.length * CTX_ITEM_H - 8),
            items: menuItems,
            hoverIdx: -1,
            itemIdx: itemIdx
        };
        this.refresh();
    };

    /**
     * 绘制右键菜单覆盖层。
     * @param {Bitmap} c - 目标位图
     */
    InventoryWindow.prototype._drawContextMenu = function (c) {
        var cm = this._contextMenu;
        var mx = cm.x, my = cm.y;
        var mh = cm.items.length * CTX_ITEM_H + 4;
        L2_Theme.fillRoundRect(c, mx, my, CTX_W, mh, 3, 'rgba(20,20,40,0.96)');
        L2_Theme.strokeRoundRect(c, mx, my, CTX_W, mh, 3, L2_Theme.borderDark);
        cm.items.forEach(function (mi, i) {
            var iy = my + 2 + i * CTX_ITEM_H;
            if (i === cm.hoverIdx) {
                c.fillRect(mx + 2, iy, CTX_W - 4, CTX_ITEM_H, L2_Theme.highlight);
            }
            c.fontSize = 11;
            c.textColor = L2_Theme.textWhite;
            c.drawText(mi.label, mx + 8, iy, CTX_W - 16, CTX_ITEM_H, 'left');
        });
    };

    /**
     * 处理右键菜单操作：向服务器发送装备/卸下/使用请求。
     * @param {string} action - 操作类型 'equip'|'unequip'|'use'|'cancel'
     */
    InventoryWindow.prototype._handleContextAction = function (action) {
        var itemIdx = this._contextMenu ? this._contextMenu.itemIdx : -1;
        var item = $MMO._inventory.items[itemIdx];
        this._contextMenu = null;
        this._selectedIdx = -1;
        if (!item) { this.refresh(); return; }

        if (action === 'equip') {
            $MMO.send('equip_item', { inv_id: item.id });
        } else if (action === 'unequip') {
            $MMO.send('unequip_item', { inv_id: item.id });
        } else if (action === 'use') {
            $MMO.send('use_item', { inv_id: item.id });
        }
        this.refresh();
    };

    // ═══════════════════════════════════════════════════════════
    //  点击/悬停/拖放交互处理
    // ═══════════════════════════════════════════════════════════

    /**
     * 每帧更新背包交互逻辑。
     * 优先级：拖放中 > 待拖放 > 右键菜单 > 格子悬停 > 格子点击。
     * 消耗品支持拖放到技能快捷栏（超过 6px 阈值后启动拖放）。
     */
    InventoryWindow.prototype.updateContent = function () {
        var mx = TouchInput.x - this.x, my = TouchInput.y - this.y;
        var topY = this.contentTop();
        var items = $MMO._inventory.items;

        // 拖放进行中：更新拖拽位置，释放时检查是否落在快捷栏上。
        if (this._dragActive) {
            if (TouchInput.isPressed()) {
                var dragItem = items[this._dragItemIdx];
                if (dragItem) {
                    var dData = resolveItemData(dragItem);
                    $MMO._uiDrag = {
                        type: 'item',
                        data: {
                            item_id: dragItem.item_id,
                            kind: dragItem.kind,
                            inv_id: dragItem.id,
                            icon_index: dData.iconIndex,
                            name: dData.name,
                            mp_cost: 0
                        },
                        x: TouchInput.x,
                        y: TouchInput.y
                    };
                }
            } else {
                // 释放时检查是否落在快捷栏格子上。
                if ($MMO._handleDrop) $MMO._handleDrop(TouchInput.x, TouchInput.y);
                this._dragActive = false;
                this._dragItemIdx = -1;
                $MMO._uiDrag = null;
            }
            return;
        }

        // 待拖放：检查是否超过移动阈值（6px）启动真正的拖放。
        if (this._dragItemIdx >= 0 && !this._dragActive) {
            if (TouchInput.isPressed()) {
                var dist = Math.abs(TouchInput.x - this._dragStartX) + Math.abs(TouchInput.y - this._dragStartY);
                if (dist > 6) {
                    // 超过阈值：切换到拖放模式，关闭右键菜单。
                    this._dragActive = true;
                    this._contextMenu = null;
                    return;
                }
            } else {
                // 未移动就释放：视为点击而非拖放。
                this._dragItemIdx = -1;
            }
        }

        // 右键菜单交互优先。
        if (this._contextMenu) {
            var cm = this._contextMenu;
            var inCtx = mx >= cm.x && mx < cm.x + CTX_W &&
                        my >= cm.y && my < cm.y + cm.items.length * CTX_ITEM_H + 4;
            var oldCtxHover = cm.hoverIdx;
            if (inCtx) {
                cm.hoverIdx = Math.floor((my - cm.y - 2) / CTX_ITEM_H);
                if (cm.hoverIdx < 0 || cm.hoverIdx >= cm.items.length) cm.hoverIdx = -1;
            } else {
                cm.hoverIdx = -1;
            }
            if (cm.hoverIdx !== oldCtxHover) this.refresh();
            if (TouchInput.isTriggered()) {
                if (inCtx && cm.hoverIdx >= 0) {
                    this._handleContextAction(cm.items[cm.hoverIdx].action);
                } else {
                    this._contextMenu = null;
                    this._selectedIdx = -1;
                    this.refresh();
                }
            }
            return;
        }

        // 格子悬停检测。
        var oldHover = this._hoverIdx;
        this._hoverIdx = -1;
        if (mx >= GRID_PAD && my >= topY + GRID_PAD) {
            var col = Math.floor((mx - GRID_PAD) / SLOT_SIZE);
            var row = Math.floor((my - topY - GRID_PAD) / SLOT_SIZE);
            if (col >= 0 && col < COLS && row >= 0 && row < ROWS) {
                var idx = row * COLS + col;
                if (idx < items.length) this._hoverIdx = idx;
            }
        }
        if (this._hoverIdx !== oldHover) this.refresh();

        // 点击物品：打开右键菜单 + 消耗品启动待拖放。
        if (TouchInput.isTriggered() && this._hoverIdx >= 0) {
            this._selectedIdx = this._hoverIdx;
            this._openContextMenu(this._hoverIdx);
            // 消耗品同时设置待拖放状态。
            var clickedItem = items[this._hoverIdx];
            if (clickedItem && clickedItem.kind === ITEM_TYPE.ITEM) {
                this._dragItemIdx = this._hoverIdx;
                this._dragStartX = TouchInput.x;
                this._dragStartY = TouchInput.y;
                this._dragActive = false;
            }
        }
    };

    // ═══════════════════════════════════════════════════════════
    //  Alt+I 快捷键切换背包窗口
    // ═══════════════════════════════════════════════════════════

    /**
     * 全局 keydown 监听器：Alt+I 切换背包窗口显示/隐藏。
     */
    window.addEventListener('keydown', function (e) {
        if (e.altKey && e.keyCode === 73) { // Alt+I
            e.preventDefault();
            if ($MMO._inventoryWindow) $MMO._inventoryWindow.toggle();
        }
    });

    // ═══════════════════════════════════════════════════════════
    //  注入 Scene_Map — 创建背包窗口
    // ═══════════════════════════════════════════════════════════

    /** @type {Function} 原始 Scene_Map.createAllWindows 引用。 */
    var _Scene_Map_createAllWindows_inv = Scene_Map.prototype.createAllWindows;

    /**
     * 覆写 Scene_Map.createAllWindows：追加创建背包窗口。
     */
    Scene_Map.prototype.createAllWindows = function () {
        _Scene_Map_createAllWindows_inv.call(this);
        $MMO._inventoryWindow = new InventoryWindow();
        this.addChild($MMO._inventoryWindow);
        $MMO.registerWindow($MMO._inventoryWindow);
    };

    // ═══════════════════════════════════════════════════════════
    //  WebSocket 消息处理器
    // ═══════════════════════════════════════════════════════════

    /**
     * inventory_update：增量背包更新。
     * 支持三种操作：add（新增/叠加）、remove（移除/减少）、update（属性变更）。
     */
    $MMO.on('inventory_update', function (data) {
        var inv = $MMO._inventory;
        // 新增物品：同类型未装备的道具叠加数量，其他直接追加。
        (data.add || []).forEach(function (item) {
            var existing = inv.items.find(function (i) {
                return i.kind === item.kind && i.item_id === item.item_id && !i.equipped;
            });
            if (existing && item.kind === ITEM_TYPE.ITEM) {
                existing.qty += (item.qty || 1);
            } else {
                inv.items.push(item);
            }
        });
        // 移除物品：减少数量，归零时从列表删除。
        (data.remove || []).forEach(function (item) {
            var idx = inv.items.findIndex(function (i) { return i.id === item.id; });
            if (idx >= 0) {
                inv.items[idx].qty -= (item.qty || 1);
                if (inv.items[idx].qty <= 0) inv.items.splice(idx, 1);
            }
        });
        // 更新物品属性（如 equipped 状态变更）。
        (data.update || []).forEach(function (item) {
            var existing = inv.items.find(function (i) { return i.id === item.id; });
            if (existing) Object.assign(existing, item);
        });
        if ($MMO._inventoryWindow && $MMO._inventoryWindow.visible) {
            $MMO._inventoryWindow.refresh();
        }
    });

    /**
     * equip_result：装备/卸下操作成功后重新加载背包。
     */
    $MMO.on('equip_result', function (data) {
        if (data && data.success && $MMO._inventoryWindow) {
            $MMO._inventoryWindow._loadInventory();
        }
    });

    // ═══════════════════════════════════════════════════════════
    //  全局常量导出
    // ═══════════════════════════════════════════════════════════
    window.ITEM_TYPE = ITEM_TYPE;

})();
