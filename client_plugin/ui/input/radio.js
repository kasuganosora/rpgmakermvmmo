/**
 * L2_Radio - Radio button group.
 */
(function () {
    'use strict';

    function L2_Radio() { this.initialize.apply(this, arguments); }
    L2_Radio.prototype = Object.create(L2_Base.prototype);
    L2_Radio.prototype.constructor = L2_Radio;

    /**
     * @param {number} x
     * @param {number} y
     * @param {object} [opts] - { items, selected, direction:'vertical'|'horizontal', onChange }
     */
    L2_Radio.prototype.initialize = function (x, y, opts) {
        opts = opts || {};
        this._radioOptions = opts.items || [];
        this._direction = opts.direction || 'vertical';
        var w = opts.width || 200;
        var itemH = 24;
        var h = this._direction === 'vertical'
            ? this._radioOptions.length * itemH + 8
            : itemH + 8;
        L2_Base.prototype.initialize.call(this, x, y, w, h);
        this._selectedIndex = opts.value !== undefined ? opts.value : (opts.selectedIndex !== undefined ? opts.selectedIndex : 0);
        this._onChange = opts.onChange || null;
        this._hoverIndex = -1;
        this.refresh();
    };

    L2_Radio.prototype.standardPadding = function () { return 4; };

    L2_Radio.prototype.getSelectedIndex = function () { return this._selectedIndex; };
    L2_Radio.prototype.getSelectedLabel = function () { return this._radioOptions[this._selectedIndex]; };

    L2_Radio.prototype.setSelectedIndex = function (idx) {
        if (idx === this._selectedIndex) return;
        this._selectedIndex = idx;
        if (this._onChange) this._onChange(idx, this._radioOptions[idx]);
        this.markDirty();
    };

    /** @deprecated Use setSelectedIndex instead */
    L2_Radio.prototype.setValue = function (idx) {
        this.setSelectedIndex(idx);
    };

    L2_Radio.prototype.refresh = function () {
        var c = this.bmp();
        c.clear();
        var cw = this.cw(), ch = this.ch();
        var itemH = 24;
        var isH = this._direction === 'horizontal';
        var itemW = isH ? Math.floor(cw / Math.max(this._radioOptions.length, 1)) : cw;
        var self = this;

        this._radioOptions.forEach(function (label, i) {
            var ix = isH ? i * itemW : 0;
            var iy = isH ? 0 : i * itemH;
            var selected = i === self._selectedIndex;
            var hover = i === self._hoverIndex;
            var dotR = 7;
            var dotCX = ix + dotR + 2;
            var dotCY = iy + itemH / 2;

            // Outer circle
            L2_Theme.drawCircle(c, dotCX, dotCY, dotR,
                selected ? L2_Theme.borderActive :
                (hover ? L2_Theme.borderGold : L2_Theme.borderDark));
            // Inner bg
            L2_Theme.drawCircle(c, dotCX, dotCY, dotR - 2, L2_Theme.bgInput);
            // Selected dot
            if (selected) {
                L2_Theme.drawCircle(c, dotCX, dotCY, 3, L2_Theme.textGold);
            }

            c.fontSize = L2_Theme.fontNormal;
            c.textColor = L2_Theme.textWhite;
            c.drawText(label, ix + dotR * 2 + 8, iy, itemW - dotR * 2 - 10, itemH, 'left');
        });
    };

    L2_Radio.prototype.update = function () {
        L2_Base.prototype.update.call(this);
        if (!this.visible) return;
        var loc = this.toLocal(TouchInput.x, TouchInput.y);
        var itemH = 24;
        var isH = this._direction === 'horizontal';
        var itemW = isH ? Math.floor(this.cw() / Math.max(this._radioOptions.length, 1)) : this.cw();
        var oldHover = this._hoverIndex;
        this._hoverIndex = -1;

        if (loc.x >= 0 && loc.x < this.cw() && loc.y >= 0 && loc.y < this.ch()) {
            if (isH) {
                this._hoverIndex = Math.min(Math.floor(loc.x / itemW), this._radioOptions.length - 1);
            } else {
                this._hoverIndex = Math.min(Math.floor(loc.y / itemH), this._radioOptions.length - 1);
            }
        }
        if (this._hoverIndex !== oldHover) this.markDirty();
        if (TouchInput.isTriggered() && this._hoverIndex >= 0) {
            this.setSelectedIndex(this._hoverIndex);
        }
    };

    window.L2_Radio = L2_Radio;
})();
