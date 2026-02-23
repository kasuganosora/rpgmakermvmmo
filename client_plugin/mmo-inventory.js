/*:
 * @plugindesc v3.0.0 MMO Inventory - item bag as floating GameWindow.
 * @author MMO Framework
 */

(function () {
    'use strict';

    $MMO._inventory = { items: [] };
    var ITEM_TYPE = { ITEM: 1, WEAPON: 2, ARMOR: 3 };
    var COLS = 5, SLOT_SIZE = 40, ICON_COLS = 16;
    var GRID_PAD = 6;
    var WIN_W = COLS * SLOT_SIZE + GRID_PAD * 2;
    var WIN_H = 6 * SLOT_SIZE + GRID_PAD * 2 + 26; // 6 rows + title bar

    // -----------------------------------------------------------------
    // InventoryWindow — extends GameWindow
    // -----------------------------------------------------------------
    function InventoryWindow() {
        GameWindow.prototype.initialize.call(this, {
            key: 'inventory', title: '背包', width: WIN_W, height: WIN_H
        });
        this._iconSet = null;
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
        this.refresh();
    };

    InventoryWindow.prototype._loadInventory = function () {
        if (!$MMO.charID) return;
        var self = this;
        $MMO.http.get('/api/characters/' + $MMO.charID + '/inventory')
            .then(function (data) {
                $MMO._inventory.items = data.items || [];
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

            // Slot background
            c.fillRect(x, y, SLOT_SIZE - 2, SLOT_SIZE - 2, '#1A1A2E');
            L2_Theme.strokeRoundRect(c, x, y, SLOT_SIZE - 2, SLOT_SIZE - 2, 2, L2_Theme.borderDark);

            // Icon
            if (self._iconSet && item.icon_index !== undefined) {
                var iconIdx = item.icon_index;
                var sx = (iconIdx % ICON_COLS) * 32;
                var sy = Math.floor(iconIdx / ICON_COLS) * 32;
                c.blt(self._iconSet, sx, sy, 32, 32, x + 4, y + 2, 32, 32);
            }

            // Quantity
            if (item.quantity > 1) {
                c.fontSize = 10;
                c.textColor = '#FFFF88';
                c.drawText(String(item.quantity), x, y + SLOT_SIZE - 16, SLOT_SIZE - 6, 12, 'right');
            }

            // Equipped indicator
            if (item.equip_slot >= 0) {
                c.fontSize = 9;
                c.textColor = '#88FFFF';
                c.drawText('E', x + 3, y + 2, 12, 10, 'left');
            }
        });

        // Empty slot outlines
        var totalSlots = Math.max(items.length, COLS * 6);
        for (var i = items.length; i < totalSlots; i++) {
            var col = i % COLS;
            var row = Math.floor(i / COLS);
            var ex = GRID_PAD + col * SLOT_SIZE;
            var ey = topY + GRID_PAD + row * SLOT_SIZE;
            if (ey + SLOT_SIZE > h) break;
            c.fillRect(ex, ey, SLOT_SIZE - 2, SLOT_SIZE - 2, '#111118');
            L2_Theme.strokeRoundRect(c, ex, ey, SLOT_SIZE - 2, SLOT_SIZE - 2, 2, '#1a1a2a');
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
                return i.item_type === item.item_type && i.item_id === item.item_id && i.equip_slot < 0;
            });
            if (existing) {
                existing.quantity += item.quantity;
            } else {
                inv.items.push(item);
            }
        });
        (data.remove || []).forEach(function (item) {
            var idx = inv.items.findIndex(function (i) { return i.slot === item.slot; });
            if (idx >= 0) {
                inv.items[idx].quantity -= item.quantity;
                if (inv.items[idx].quantity <= 0) inv.items.splice(idx, 1);
            }
        });
        (data.update || []).forEach(function (item) {
            var existing = inv.items.find(function (i) { return i.slot === item.slot; });
            if (existing) Object.assign(existing, item);
        });
        // Refresh if inventory window is open
        if ($MMO._inventoryWindow && $MMO._inventoryWindow.visible) {
            $MMO._inventoryWindow.refresh();
        }
    });

    window.ITEM_TYPE = ITEM_TYPE;

})();
