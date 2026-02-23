/**
 * L2_Menu - Vertical navigation menu with selectable items.
 */
(function () {
    'use strict';

    function L2_Menu() { this.initialize.apply(this, arguments); }
    L2_Menu.prototype = Object.create(L2_Base.prototype);
    L2_Menu.prototype.constructor = L2_Menu;

    /**
     * @param {number} x
     * @param {number} y
     * @param {number} w
     * @param {object} [opts] - { items: [{label, icon, action}] or string[], itemHeight, activeIndex }
     */
    L2_Menu.prototype.initialize = function (x, y, w, opts) {
        opts = opts || {};
        var items = opts.items || [];
        // Support string[] shorthand
        if (items.length > 0 && typeof items[0] === 'string') {
            items = items.map(function (s) { return { label: s }; });
        }
        var itemH = opts.itemHeight || 26;
        var h = items.length * itemH + 8;
        L2_Base.prototype.initialize.call(this, x, y, w, h);
        this._items = items;
        this._itemHeight = itemH;
        this._activeIndex = opts.activeIndex !== undefined ? opts.activeIndex : -1;
        this._hoverIndex = -1;
        this.refresh();
    };

    L2_Menu.prototype.standardPadding = function () { return 4; };

    L2_Menu.prototype.setItems = function (items) {
        this._items = items || [];
        this._activeIndex = -1;
        this.refresh();
    };

    L2_Menu.prototype.setActiveIndex = function (idx) {
        this._activeIndex = idx;
        this.refresh();
    };

    L2_Menu.prototype.refresh = function () {
        var c = this.bmp();
        c.clear();
        var cw = this.cw(), ch = this.ch();
        L2_Theme.drawPanelBg(c, 0, 0, cw, ch);

        var ih = this._itemHeight;
        var self = this;
        this._items.forEach(function (item, i) {
            var iy = i * ih + 4;
            if (i === self._activeIndex) {
                c.fillRect(2, iy, cw - 4, ih, L2_Theme.selection);
                c.fillRect(2, iy, 3, ih, L2_Theme.textGold);
            } else if (i === self._hoverIndex) {
                c.fillRect(2, iy, cw - 4, ih, L2_Theme.highlight);
            }
            c.fontSize = L2_Theme.fontNormal;
            c.textColor = i === self._activeIndex ? L2_Theme.textGold : L2_Theme.textWhite;
            c.drawText(item.label || '', 12, iy + 4, cw - 24, ih - 8, 'left');
        });
    };

    L2_Menu.prototype.update = function () {
        L2_Base.prototype.update.call(this);
        if (!this.visible) return;
        var loc = this.toLocal(TouchInput.x, TouchInput.y);
        var cw = this.cw();
        var ih = this._itemHeight;
        var inside = loc.x >= 0 && loc.x < cw && loc.y >= 0 && loc.y < this.ch();
        var oldHover = this._hoverIndex;
        this._hoverIndex = inside ? Math.floor((loc.y - 4) / ih) : -1;
        if (this._hoverIndex >= this._items.length) this._hoverIndex = -1;
        if (this._hoverIndex !== oldHover) this.refresh();
        if (inside && TouchInput.isTriggered() && this._hoverIndex >= 0) {
            this._activeIndex = this._hoverIndex;
            var item = this._items[this._hoverIndex];
            if (item && item.action) item.action(this._hoverIndex);
            this.refresh();
        }
    };

    window.L2_Menu = L2_Menu;
})();
