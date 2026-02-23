/**
 * L2_Progress - Progress bar (HP/MP/EXP/Loading).
 */
(function () {
    'use strict';

    function L2_Progress() { this.initialize.apply(this, arguments); }
    L2_Progress.prototype = Object.create(L2_Base.prototype);
    L2_Progress.prototype.constructor = L2_Progress;

    /**
     * @param {number} x
     * @param {number} y
     * @param {number} w
     * @param {object} [opts] - { bgColor, fillColor, label, showText, showPercent, value, max, height }
     */
    L2_Progress.prototype.initialize = function (x, y, w, opts) {
        opts = opts || {};
        var h = opts.height || 18;
        L2_Base.prototype.initialize.call(this, x, y, w, h);
        this._bgColor = opts.bgColor || L2_Theme.hpBg;
        this._fillColor = opts.fillColor || L2_Theme.hpFill;
        this._value = opts.value || 0;
        this._maxValue = opts.max || 100;
        this._label = opts.label || '';
        this._showText = opts.showText !== false;
        this._showPercent = opts.showPercent || false;
        this.refresh();
    };

    L2_Progress.prototype.standardPadding = function () { return 0; };

    L2_Progress.prototype.setValue = function (val, max) {
        this._value = val;
        if (max !== undefined) this._maxValue = max;
        this.refresh();
    };

    L2_Progress.prototype.refresh = function () {
        var c = this.bmp();
        c.clear();
        var cw = this.cw(), ch = this.ch();
        var ratio = this._maxValue > 0 ? this._value / this._maxValue : 0;
        L2_Theme.drawBar(c, 0, 0, cw, ch, ratio, this._bgColor, this._fillColor);
        if (this._showText) {
            c.fontSize = Math.max(ch - 4, 8);
            c.textColor = L2_Theme.textWhite;
            var text = this._showPercent
                ? Math.floor(ratio * 100) + '%'
                : (this._label ? this._label + '  ' : '') + this._value + ' / ' + this._maxValue;
            c.drawText(text, 4, 0, cw - 8, ch, 'left');
        }
    };

    window.L2_Progress = L2_Progress;
})();
