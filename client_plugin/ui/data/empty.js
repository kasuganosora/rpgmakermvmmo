/**
 * L2_Empty - Empty state placeholder with icon and message.
 */
(function () {
    'use strict';

    function L2_Empty() { this.initialize.apply(this, arguments); }
    L2_Empty.prototype = Object.create(L2_Base.prototype);
    L2_Empty.prototype.constructor = L2_Empty;

    /**
     * @param {number} x
     * @param {number} y
     * @param {number} w
     * @param {object} [opts] - { text, iconIndex }
     */
    L2_Empty.prototype.initialize = function (x, y, w, opts) {
        opts = opts || {};
        this._emptyText = opts.text || '暂无数据';
        this._iconIndex = opts.iconIndex != null ? opts.iconIndex : -1;
        var h = this._iconIndex >= 0 ? 80 : 50;
        L2_Base.prototype.initialize.call(this, x, y, w, h);
        this.refresh();
    };

    L2_Empty.prototype.standardPadding = function () { return 4; };

    L2_Empty.prototype.refresh = function () {
        var c = this.bmp();
        c.clear();
        var cw = this.cw(), ch = this.ch();
        var yy = 0;

        // Icon
        if (this._iconIndex >= 0) {
            var bitmap = ImageManager.loadSystem('IconSet');
            if (bitmap && bitmap.isReady()) {
                var pw = 32, ph = 32;
                var sx = (this._iconIndex % 16) * pw;
                var sy = Math.floor(this._iconIndex / 16) * ph;
                c.blt(bitmap, sx, sy, pw, ph, (cw - 32) / 2, yy + 4);
            }
            yy += 40;
        }

        // Text
        c.fontSize = L2_Theme.fontNormal;
        c.textColor = L2_Theme.textGray;
        c.drawText(this._emptyText, 0, yy, cw, 24, 'center');
    };

    window.L2_Empty = L2_Empty;
})();
