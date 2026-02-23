/**
 * L2_Divider - Horizontal or vertical dividing line.
 */
(function () {
    'use strict';

    function L2_Divider() { this.initialize.apply(this, arguments); }
    L2_Divider.prototype = Object.create(L2_Base.prototype);
    L2_Divider.prototype.constructor = L2_Divider;

    /**
     * @param {number} x
     * @param {number} y
     * @param {number} length
     * @param {object} [opts] - { vertical, color, text }
     */
    L2_Divider.prototype.initialize = function (x, y, length, opts) {
        opts = opts || {};
        var vertical = opts.vertical || false;
        var w = vertical ? 8 : length;
        var h = vertical ? length : 8;
        L2_Base.prototype.initialize.call(this, x, y, w + 16, h + 16);
        this._vertical = vertical;
        this._lineColor = opts.color || L2_Theme.divider;
        this._text = opts.text || '';
        this._length = length;
        this.refresh();
    };

    L2_Divider.prototype.standardPadding = function () { return 0; };

    L2_Divider.prototype.refresh = function () {
        var c = this.bmp();
        c.clear();
        var cw = this.cw(), ch = this.ch();
        if (this._vertical) {
            L2_Theme.drawLine(c, cw / 2, 0, cw / 2, ch, this._lineColor);
        } else if (this._text) {
            var tw = L2_Theme.measureText(c, this._text, L2_Theme.fontSmall);
            var cx = cw / 2;
            var half = tw / 2 + 8;
            L2_Theme.drawLine(c, 0, ch / 2, cx - half, ch / 2, this._lineColor);
            L2_Theme.drawLine(c, cx + half, ch / 2, cw, ch / 2, this._lineColor);
            c.fontSize = L2_Theme.fontSmall;
            c.textColor = L2_Theme.textGray;
            c.drawText(this._text, 0, 0, cw, ch, 'center');
        } else {
            L2_Theme.drawLine(c, 0, ch / 2, cw, ch / 2, this._lineColor);
        }
    };

    window.L2_Divider = L2_Divider;
})();
