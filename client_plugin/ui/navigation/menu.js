/**
 * L2_Menu - Vertical navigation menu with selectable items.
 * Fixed padding calculations.
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
     * @param {object} [opts] - { items: [{label, icon, action}] or string[], itemHeight, activeIndex, maxHeight }
     */
    L2_Menu.prototype.initialize = function (x, y, w, opts) {
        opts = opts || {};
        var items = opts.items || [];
        // Support string[] shorthand
        if (items.length > 0 && typeof items[0] === 'string') {
            items = items.map(function (s) { return { label: s }; });
        }
        var itemH = opts.itemHeight || L2_Theme.defaultItemHeight;
        this._maxHeight = opts.maxHeight || 0; // 0 表示不限制高度
        
        // 计算内容高度：padding + items + padding
        this._contentPadding = 4;  // 内容区内边距
        var totalContentH = items.length * itemH + this._contentPadding * 2;
        var h = this._maxHeight > 0 ? Math.min(totalContentH, this._maxHeight) : totalContentH;
        
        L2_Base.prototype.initialize.call(this, x, y, w, h);
        this._items = items;
        this._itemHeight = itemH;
        this._activeIndex = opts.activeIndex !== undefined ? opts.activeIndex : -1;
        this._hoverIndex = -1;
        this._scrollY = 0;
        this.refresh();
    };

    L2_Menu.prototype.standardPadding = function () { return 4; };

    L2_Menu.prototype.setItems = function (items) {
        this._items = items || [];
        this._activeIndex = -1;
        this.markDirty();
    };

    L2_Menu.prototype.setActiveIndex = function (idx) {
        this._activeIndex = idx;
        this.markDirty();
    };

    L2_Menu.prototype.refresh = function () {
        var c = this.bmp();
        c.clear();
        var cw = this.cw(), ch = this.ch();
        L2_Theme.drawPanelBg(c, 0, 0, cw, ch);

        var ih = this._itemHeight;
        var sbW = this._needsScrollbar() ? L2_Theme.scrollbarWidth : 0;
        var startIdx = Math.floor(this._scrollY / ih);
        var visCount = Math.ceil(ch / ih) + 1;

        for (var i = startIdx; i < Math.min(startIdx + visCount, this._items.length); i++) {
            // 使用统一的 padding 计算 item Y 位置
            var iy = this._contentPadding + i * ih - this._scrollY;
            if (iy + ih < 0 || iy > ch) continue;

            if (i === this._activeIndex) {
                c.fillRect(2, iy, cw - sbW - 4, ih, L2_Theme.selection);
                c.fillRect(2, iy, 3, ih, L2_Theme.textGold);
            } else if (i === this._hoverIndex) {
                c.fillRect(2, iy, cw - sbW - 4, ih, L2_Theme.highlight);
            }
            c.fontSize = L2_Theme.fontNormal;
            c.textColor = i === this._activeIndex ? L2_Theme.textGold : L2_Theme.textWhite;
            c.drawText(this._items[i].label || '', 12, iy + 4, cw - sbW - 24, ih - 8, 'left');
        }

        // 滚动条
        if (this._needsScrollbar()) {
            var totalH = this._items.length * ih + this._contentPadding * 2;
            var trackH = ch - 4;
            var thumbH = Math.max(20, Math.round(trackH * (ch / totalH)));
            var maxScroll = this._getMaxScroll();
            var thumbY = 2 + (maxScroll > 0 ? Math.round((trackH - thumbH) * (this._scrollY / maxScroll)) : 0);
            c.fillRect(cw - sbW, 0, sbW, ch, 'rgba(0,0,0,0.3)');
            L2_Theme.fillRoundRect(c, cw - sbW, thumbY, sbW, thumbH, 2, '#444466');
        }
    };

    L2_Menu.prototype._needsScrollbar = function () {
        if (this._maxHeight <= 0) return false;
        var totalH = this._items.length * this._itemHeight + this._contentPadding * 2;
        return totalH > this._maxHeight;
    };

    L2_Menu.prototype._getMaxScroll = function () {
        var totalH = this._items.length * this._itemHeight + this._contentPadding * 2;
        return Math.max(0, totalH - this.height);
    };

    L2_Menu.prototype.update = function () {
        L2_Base.prototype.update.call(this);
        if (!this.visible) return;
        var loc = this.toLocal(TouchInput.x, TouchInput.y);
        var cw = this.cw();
        var ih = this._itemHeight;
        var ch = this.ch();
        var sbW = this._needsScrollbar() ? L2_Theme.scrollbarWidth : 0;
        var inside = loc.x >= 0 && loc.x < cw - sbW && loc.y >= 0 && loc.y < ch;
        var oldHover = this._hoverIndex;
        
        // 使用统一的 padding 计算 hover index
        this._hoverIndex = inside ? Math.floor((loc.y + this._scrollY - this._contentPadding) / ih) : -1;
        if (this._hoverIndex >= this._items.length) this._hoverIndex = -1;
        if (this._hoverIndex !== oldHover) this.markDirty();

        // 滚轮滚动
        if (inside && TouchInput.wheelY && this._needsScrollbar()) {
            this._scrollY += TouchInput.wheelY > 0 ? ih : -ih;
            this._scrollY = Math.max(0, Math.min(this._scrollY, this._getMaxScroll()));
            this.markDirty();
        }

        if (inside && TouchInput.isTriggered() && this._hoverIndex >= 0) {
            this._activeIndex = this._hoverIndex;
            var item = this._items[this._hoverIndex];
            if (item && item.action) item.action(this._hoverIndex);
            this.markDirty();
        }
    };

    window.L2_Menu = L2_Menu;
})();
