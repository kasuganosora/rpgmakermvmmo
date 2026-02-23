/**
 * L2_Loading - Animated loading spinner.
 */
(function () {
    'use strict';

    function L2_Loading() { this.initialize.apply(this, arguments); }
    L2_Loading.prototype = Object.create(L2_Base.prototype);
    L2_Loading.prototype.constructor = L2_Loading;

    /**
     * @param {number} x
     * @param {number} y
     * @param {object} [opts] - { size, text, color }
     */
    L2_Loading.prototype.initialize = function (x, y, opts) {
        opts = opts || {};
        this._spinSize = opts.size || 24;
        this._loadingText = opts.text || '';
        this._spinColor = opts.color || L2_Theme.primaryColor;
        this._angle = 0;
        var w = this._loadingText ? this._spinSize + 8 + this._loadingText.length * 8 + 8 : this._spinSize + 8;
        var h = Math.max(this._spinSize + 8, 28);
        L2_Base.prototype.initialize.call(this, x, y, w, h);
        this.refresh();
    };

    L2_Loading.prototype.standardPadding = function () { return 4; };

    L2_Loading.prototype.refresh = function () {
        var c = this.bmp();
        c.clear();
        var cw = this.cw(), ch = this.ch();
        var ctx = c._context;
        if (!ctx) return;

        var cx = this._spinSize / 2;
        var cy = ch / 2;
        var r = this._spinSize / 2 - 2;

        ctx.save();
        // Background circle
        ctx.beginPath();
        ctx.arc(cx, cy, r, 0, Math.PI * 2);
        ctx.strokeStyle = L2_Theme.borderDark;
        ctx.lineWidth = 3;
        ctx.stroke();

        // Spinning arc
        ctx.beginPath();
        ctx.arc(cx, cy, r, this._angle, this._angle + Math.PI * 0.75);
        ctx.strokeStyle = this._spinColor;
        ctx.lineWidth = 3;
        ctx.lineCap = 'round';
        ctx.stroke();
        ctx.restore();

        // Text
        if (this._loadingText) {
            c.fontSize = L2_Theme.fontSmall;
            c.textColor = L2_Theme.textGray;
            c.drawText(this._loadingText, this._spinSize + 6, 0, cw - this._spinSize - 8, ch, 'left');
        }
        c._setDirty();
    };

    L2_Loading.prototype.update = function () {
        L2_Base.prototype.update.call(this);
        if (!this.visible) return;
        this._angle += 0.08;
        if (this._angle > Math.PI * 2) this._angle -= Math.PI * 2;
        this.refresh();
    };

    window.L2_Loading = L2_Loading;
})();
