/**
 * L2_Statistic - Display a statistic value with title and optional prefix/suffix.
 */
(function () {
    'use strict';

    function L2_Statistic() { this.initialize.apply(this, arguments); }
    L2_Statistic.prototype = Object.create(L2_Base.prototype);
    L2_Statistic.prototype.constructor = L2_Statistic;

    /**
     * @param {number} x
     * @param {number} y
     * @param {object} [opts] - { title, value, prefix, suffix, color, trend }
     */
    L2_Statistic.prototype.initialize = function (x, y, opts) {
        opts = opts || {};
        this._title = opts.title || '';
        this._value = opts.value != null ? opts.value : 0;
        this._prefix = opts.prefix || '';
        this._suffix = opts.suffix || '';
        this._valueColor = opts.color || L2_Theme.textWhite;
        this._trend = opts.trend || ''; // 'up' | 'down' | ''
        var w = opts.width || 120;
        L2_Base.prototype.initialize.call(this, x, y, w, 56);
        this.refresh();
    };

    L2_Statistic.prototype.standardPadding = function () { return 4; };

    L2_Statistic.prototype.setValue = function (v) {
        if (this._value !== v) { this._value = v; this.refresh(); }
    };

    L2_Statistic.prototype.refresh = function () {
        var c = this.bmp();
        c.clear();
        var cw = this.cw();

        // Title
        c.fontSize = L2_Theme.fontSmall;
        c.textColor = L2_Theme.textGray;
        c.drawText(this._title, 0, 0, cw, 18, 'left');

        // Value line
        var valStr = this._prefix + String(this._value) + this._suffix;
        c.fontSize = 22;
        this._valueColor = this._valueColor || L2_Theme.textWhite;
        if (this._trend === 'up') {
            c.textColor = L2_Theme.successColor;
        } else if (this._trend === 'down') {
            c.textColor = L2_Theme.dangerColor;
        } else {
            c.textColor = this._valueColor;
        }
        c.drawText(valStr, 0, 20, cw, 28, 'left');

        // Trend arrow
        if (this._trend === 'up') {
            c.textColor = L2_Theme.successColor;
            c.drawText('↑', cw - 20, 20, 20, 28, 'right');
        } else if (this._trend === 'down') {
            c.textColor = L2_Theme.dangerColor;
            c.drawText('↓', cw - 20, 20, 20, 28, 'right');
        }
    };

    window.L2_Statistic = L2_Statistic;
})();
