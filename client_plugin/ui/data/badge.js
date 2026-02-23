/**
 * L2_Badge - Small count or status dot displayed on another element.
 */
(function () {
    'use strict';

    function L2_Badge() { this.initialize.apply(this, arguments); }
    L2_Badge.prototype = Object.create(L2_Base.prototype);
    L2_Badge.prototype.constructor = L2_Badge;

    /**
     * @param {number} x
     * @param {number} y
     * @param {object} [opts] - { count, maxCount, dot, color, offset }
     */
    L2_Badge.prototype.initialize = function (x, y, opts) {
        opts = opts || {};
        this._count = opts.count != null ? opts.count : 0;
        this._maxCount = opts.maxCount || 99;
        this._dot = opts.dot || false;
        this._badgeColor = opts.color || L2_Theme.dangerColor;
        this._offset = opts.offset || { x: 0, y: 0 };
        var size = this._dot ? 10 : 22;
        L2_Base.prototype.initialize.call(this, x + this._offset.x, y + this._offset.y, size + 4, size + 4);
        this.refresh();
    };

    L2_Badge.prototype.standardPadding = function () { return 2; };

    L2_Badge.prototype.setCount = function (n) {
        if (this._count !== n) { this._count = n; this.refresh(); }
    };

    L2_Badge.prototype.refresh = function () {
        var c = this.bmp();
        c.clear();
        if (this._count <= 0 && !this._dot) return;
        var cw = this.cw(), ch = this.ch();
        var ctx = c._context;
        if (!ctx) return;

        ctx.save();
        if (this._dot) {
            var r = 4;
            ctx.fillStyle = this._badgeColor;
            ctx.beginPath();
            ctx.arc(cw / 2, ch / 2, r, 0, Math.PI * 2);
            ctx.fill();
        } else {
            var text = this._count > this._maxCount ? this._maxCount + '+' : String(this._count);
            var tw = Math.max(text.length * 7 + 6, 18);
            var bh = 16;
            var bx = (cw - tw) / 2, by = (ch - bh) / 2;
            ctx.fillStyle = this._badgeColor;
            L2_Theme.fillRoundRect(c, bx, by, tw, bh, bh / 2, this._badgeColor);
            c.fontSize = 11;
            c.textColor = '#FFFFFF';
            c.drawText(text, bx, by, tw, bh, 'center');
        }
        ctx.restore();
        c._setDirty();
    };

    window.L2_Badge = L2_Badge;
})();
