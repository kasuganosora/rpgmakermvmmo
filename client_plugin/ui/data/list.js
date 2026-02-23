/**
 * L2_List - Scrollable list with selection and hover.
 */
(function () {
    'use strict';

    function L2_List() { this.initialize.apply(this, arguments); }
    L2_List.prototype = Object.create(L2_Base.prototype);
    L2_List.prototype.constructor = L2_List;

    /**
     * @param {number} x
     * @param {number} y
     * @param {number} w
     * @param {number} h
     * @param {object} [opts] - { itemHeight, onSelect, drawItem }
     */
    L2_List.prototype.initialize = function (x, y, w, h, opts) {
        L2_Base.prototype.initialize.call(this, x, y, w, h);
        opts = opts || {};
        this._items = [];
        this._itemHeight = opts.itemHeight || 24;
        this._scrollY = 0;
        this._selectedIndex = -1;
        this._hoverIndex = -1;
        this._onSelect = opts.onSelect || null;
        this._drawItemFn = opts.drawItem || null;
        this.refresh();
    };

    L2_List.prototype.standardPadding = function () { return 4; };

    L2_List.prototype.setItems = function (items) {
        this._items = items || [];
        this._scrollY = 0;
        this._selectedIndex = -1;
        this.refresh();
    };

    L2_List.prototype.getSelected = function () {
        return this._selectedIndex >= 0 ? this._items[this._selectedIndex] : null;
    };

    L2_List.prototype.getSelectedIndex = function () { return this._selectedIndex; };

    L2_List.prototype.refresh = function () {
        var c = this.bmp();
        c.clear();
        var cw = this.cw(), ch = this.ch();

        c.fillRect(0, 0, cw, ch, L2_Theme.bgDark);
        L2_Theme.strokeRoundRect(c, 0, 0, cw, ch, 2, L2_Theme.borderDark);

        var ih = this._itemHeight;
        var sbW = 6;
        var startIdx = Math.floor(this._scrollY / ih);
        var visCount = Math.ceil(ch / ih) + 1;

        for (var i = startIdx; i < Math.min(startIdx + visCount, this._items.length); i++) {
            var iy = i * ih - this._scrollY;
            if (iy + ih < 0 || iy > ch) continue;

            if (i === this._selectedIndex) {
                c.fillRect(1, iy, cw - sbW - 2, ih, L2_Theme.selection);
            } else if (i === this._hoverIndex) {
                c.fillRect(1, iy, cw - sbW - 2, ih, L2_Theme.highlight);
            }

            c.fillRect(2, iy + ih - 1, cw - sbW - 4, 1, L2_Theme.borderDark);

            if (this._drawItemFn) {
                this._drawItemFn(c, this._items[i], 4, iy, cw - sbW - 8, ih, i);
            } else {
                var item = this._items[i];
                c.fontSize = L2_Theme.fontNormal;
                c.textColor = item.color || L2_Theme.textWhite;
                c.drawText(item.text || String(item), 4, iy + 2, cw - sbW - 8, ih - 4, 'left');
                if (item.subText) {
                    c.fontSize = L2_Theme.fontSmall;
                    c.textColor = L2_Theme.textGray;
                    c.drawText(item.subText, 4, iy + 2, cw - sbW - 8, ih - 4, 'right');
                }
            }
        }

        // Scrollbar
        var totalH = this._items.length * ih;
        if (totalH > ch) {
            var trackH = ch - 4;
            var thumbH = Math.max(20, Math.round(trackH * (ch / totalH)));
            var thumbY = 2 + Math.round((trackH - thumbH) * (this._scrollY / (totalH - ch)));
            c.fillRect(cw - sbW, 0, sbW, ch, 'rgba(0,0,0,0.3)');
            L2_Theme.fillRoundRect(c, cw - sbW, thumbY, sbW, thumbH, 2, '#444466');
        }
    };

    L2_List.prototype.update = function () {
        L2_Base.prototype.update.call(this);
        if (!this.visible) return;

        var loc = this.toLocal(TouchInput.x, TouchInput.y);
        var cw = this.cw(), ch = this.ch();
        var ih = this._itemHeight;
        var inside = loc.x >= 0 && loc.x < cw && loc.y >= 0 && loc.y < ch;

        var oldHover = this._hoverIndex;
        this._hoverIndex = inside ? Math.floor((loc.y + this._scrollY) / ih) : -1;
        if (this._hoverIndex >= this._items.length) this._hoverIndex = -1;
        if (this._hoverIndex !== oldHover) this.refresh();

        if (inside && TouchInput.isTriggered() && this._hoverIndex >= 0) {
            this._selectedIndex = this._hoverIndex;
            if (this._onSelect) this._onSelect(this._items[this._selectedIndex], this._selectedIndex);
            this.refresh();
        }

        if (inside && TouchInput.wheelY) {
            var totalH = this._items.length * ih;
            this._scrollY += TouchInput.wheelY > 0 ? ih : -ih;
            this._scrollY = Math.max(0, Math.min(this._scrollY, Math.max(0, totalH - ch)));
            this.refresh();
        }
    };

    window.L2_List = L2_List;
})();
