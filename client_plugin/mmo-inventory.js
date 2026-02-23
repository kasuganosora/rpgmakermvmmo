/*:
 * @plugindesc v4.0.0 MMO Inventory - item bag with equip/unequip (GameWindow).
 * @author MMO Framework
 */

(function () {
    'use strict';

    $MMO._inventory = { items: [] };
    var ITEM_TYPE = { ITEM: 1, WEAPON: 2, ARMOR: 3 };
    var COLS = 5, SLOT_SIZE = 40, ICON_COLS = 16;
    var GRID_PAD = 6;
    var GW_TITLE_H = 26;
    var WIN_W = COLS * SLOT_SIZE + GRID_PAD * 2;
    var ROWS = 6;
    var WIN_H = ROWS * SLOT_SIZE + GRID_PAD * 2 + GW_TITLE_H;

    // Equipment slot names for display.
    var EQUIP_SLOT_NAMES = {
        0: '武器', 1: '盾牌', 2: '头盔', 3: '铠甲', 4: '饰品'
    };

    // -----------------------------------------------------------------
    // Resolve RMMV data for an inventory item (name, icon, description).
    // Server stores item_id + kind; client looks up from RMMV data files.
    // -----------------------------------------------------------------
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

    // -----------------------------------------------------------------
    // InventoryWindow — extends GameWindow
    // -----------------------------------------------------------------
    function InventoryWindow() {
        GameWindow.prototype.initialize.call(this, {
            key: 'inventory', title: '背包', width: WIN_W, height: WIN_H
        });
        this._iconSet = null;
        this._hoverIdx = -1;
        this._selectedIdx = -1;
        this._contextMenu = null;
        var self = this;
        ImageManager.loadSystem('IconSet').addLoadListener(function (bmp) {
            self._iconSet = bmp;
            if (self.visible) self.refresh();
        });
    }
    InventoryWindow.prototype = Object.create(GameWindow.prototype);
    InventoryWindow.prototype.constructor = InventoryWindow;

    InventoryWindow.prototype.onOpen = function () {
        this._loadInventory();
        this._selectedIdx = -1;
        this._contextMenu = null;
        this._dragItemIdx = -1;
        this._dragActive = false;
        this.refresh();
    };

    InventoryWindow.prototype.close = function () {
        this._contextMenu = null;
        this._selectedIdx = -1;
        GameWindow.prototype.close.call(this);
    };

    InventoryWindow.prototype._loadInventory = function () {
        if (!$MMO.charID) return;
        var self = this;
        $MMO.http.get('/api/characters/' + $MMO.charID + '/inventory')
            .then(function (data) {
                // Server returns { inventory: [...] }
                $MMO._inventory.items = data.inventory || data.items || [];
                if (self.visible) self.refresh();
            })
            .catch(function (e) { console.error('[mmo-inventory] Load failed:', e.message); });
    };

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

            // Slot background (highlight if selected)
            var isSelected = (i === self._selectedIdx);
            var bg = isSelected ? '#2A2A4E' : '#1A1A2E';
            c.fillRect(x, y, SLOT_SIZE - 2, SLOT_SIZE - 2, bg);
            var border = isSelected ? L2_Theme.textGold : L2_Theme.borderDark;
            L2_Theme.strokeRoundRect(c, x, y, SLOT_SIZE - 2, SLOT_SIZE - 2, 2, border);

            // Icon from RMMV data
            var data = resolveItemData(item);
            if (self._iconSet && data.iconIndex > 0) {
                var sx = (data.iconIndex % ICON_COLS) * 32;
                var sy = Math.floor(data.iconIndex / ICON_COLS) * 32;
                c.blt(self._iconSet, sx, sy, 32, 32, x + 4, y + 2, 32, 32);
            }

            // Quantity (server field: qty)
            if (item.qty > 1) {
                c.fontSize = 10;
                c.textColor = '#FFFF88';
                c.drawText(String(item.qty), x, y + SLOT_SIZE - 16, SLOT_SIZE - 6, 12, 'right');
            }

            // Equipped indicator (server field: equipped)
            if (item.equipped) {
                c.fontSize = 9;
                c.textColor = '#88FFFF';
                c.drawText('E', x + 3, y + 2, 12, 10, 'left');
            }
        });

        // Empty slot outlines
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

        // Hover tooltip: show item name at bottom
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

        // Context menu overlay
        if (self._contextMenu) {
            self._drawContextMenu(c);
        }
    };

    // -----------------------------------------------------------------
    // Context menu (click on item → equip/unequip/use)
    // -----------------------------------------------------------------
    var CTX_ITEM_H = 22, CTX_W = 80;

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

    // -----------------------------------------------------------------
    // Click / hover / drag handling
    // -----------------------------------------------------------------
    InventoryWindow.prototype.updateContent = function () {
        var mx = TouchInput.x - this.x, my = TouchInput.y - this.y;
        var topY = this.contentTop();
        var items = $MMO._inventory.items;

        // Handle active item drag (item → skillbar, after threshold)
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
                // Released — check if dropped on a SkillBar slot
                if ($MMO._handleDrop) $MMO._handleDrop(TouchInput.x, TouchInput.y);
                this._dragActive = false;
                this._dragItemIdx = -1;
                $MMO._uiDrag = null;
            }
            return;
        }

        // Pending drag: check if threshold exceeded to start real drag
        if (this._dragItemIdx >= 0 && !this._dragActive) {
            if (TouchInput.isPressed()) {
                var dist = Math.abs(TouchInput.x - this._dragStartX) + Math.abs(TouchInput.y - this._dragStartY);
                if (dist > 6) {
                    // Threshold exceeded — switch to drag mode, close context menu
                    this._dragActive = true;
                    this._contextMenu = null;
                    return;
                }
            } else {
                // Released without moving — it was a click, not a drag
                this._dragItemIdx = -1;
            }
        }

        // Context menu interaction takes priority
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

        // Grid hover
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

        // Click on item: open context menu + start pending drag (consumables only)
        if (TouchInput.isTriggered() && this._hoverIdx >= 0) {
            this._selectedIdx = this._hoverIdx;
            this._openContextMenu(this._hoverIdx);
            // Also set up pending drag for consumable items
            var clickedItem = items[this._hoverIdx];
            if (clickedItem && clickedItem.kind === ITEM_TYPE.ITEM) {
                this._dragItemIdx = this._hoverIdx;
                this._dragStartX = TouchInput.x;
                this._dragStartY = TouchInput.y;
                this._dragActive = false;
            }
        }
    };

    // -----------------------------------------------------------------
    // Alt+I toggles inventory window
    // -----------------------------------------------------------------
    window.addEventListener('keydown', function (e) {
        if (e.altKey && e.keyCode === 73) { // Alt+I
            e.preventDefault();
            if ($MMO._inventoryWindow) $MMO._inventoryWindow.toggle();
        }
    });

    // -----------------------------------------------------------------
    // Inject into Scene_Map
    // -----------------------------------------------------------------
    var _Scene_Map_createAllWindows_inv = Scene_Map.prototype.createAllWindows;
    Scene_Map.prototype.createAllWindows = function () {
        _Scene_Map_createAllWindows_inv.call(this);
        $MMO._inventoryWindow = new InventoryWindow();
        this.addChild($MMO._inventoryWindow);
        $MMO.registerWindow($MMO._inventoryWindow);
    };

    // -----------------------------------------------------------------
    // WebSocket handlers
    // -----------------------------------------------------------------
    $MMO.on('inventory_update', function (data) {
        var inv = $MMO._inventory;
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
        (data.remove || []).forEach(function (item) {
            var idx = inv.items.findIndex(function (i) { return i.id === item.id; });
            if (idx >= 0) {
                inv.items[idx].qty -= (item.qty || 1);
                if (inv.items[idx].qty <= 0) inv.items.splice(idx, 1);
            }
        });
        (data.update || []).forEach(function (item) {
            var existing = inv.items.find(function (i) { return i.id === item.id; });
            if (existing) Object.assign(existing, item);
        });
        if ($MMO._inventoryWindow && $MMO._inventoryWindow.visible) {
            $MMO._inventoryWindow.refresh();
        }
    });

    // Handle equip/unequip result — reload inventory to get updated state.
    $MMO.on('equip_result', function (data) {
        if (data && data.success && $MMO._inventoryWindow) {
            $MMO._inventoryWindow._loadInventory();
        }
    });

    window.ITEM_TYPE = ITEM_TYPE;

})();
