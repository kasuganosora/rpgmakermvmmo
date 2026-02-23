/**
 * L2_Steps - Step progress indicator (horizontal).
 */
(function () {
    'use strict';

    function L2_Steps() { this.initialize.apply(this, arguments); }
    L2_Steps.prototype = Object.create(L2_Base.prototype);
    L2_Steps.prototype.constructor = L2_Steps;

    /**
     * @param {number} x
     * @param {number} y
     * @param {number} w
     * @param {object} [opts] - { steps, current, onChange }
     */
    L2_Steps.prototype.initialize = function (x, y, w, opts) {
        L2_Base.prototype.initialize.call(this, x, y, w, 48 + 4);
        opts = opts || {};
        this._steps = opts.steps || [];
        this._current = opts.current || 0;
        this._onChange = opts.onChange || null;
        this.refresh();
    };

    L2_Steps.prototype.standardPadding = function () { return 2; };

    L2_Steps.prototype.setCurrent = function (idx) {
        this._current = Math.max(0, Math.min(idx, this._steps.length - 1));
        if (this._onChange) this._onChange(this._current);
        this.refresh();
    };

    L2_Steps.prototype.refresh = function () {
        var c = this.bmp();
        c.clear();
        var cw = this.cw(), ch = this.ch();
        var n = this._steps.length;
        if (n === 0) return;

        var stepW = cw / n;
        var dotR = 10;
        var dotY = 14;
        var self = this;

        this._steps.forEach(function (label, i) {
            var cx = stepW * i + stepW / 2;
            var done = i < self._current;
            var active = i === self._current;

            // Connector line
            if (i > 0) {
                var prevX = stepW * (i - 1) + stepW / 2;
                var lineColor = i <= self._current ? L2_Theme.textGold : L2_Theme.borderDark;
                L2_Theme.drawLine(c, prevX + dotR, dotY, cx - dotR, dotY, lineColor, 2);
            }

            // Dot
            var dotColor = done ? L2_Theme.textGold :
                          (active ? L2_Theme.textBlue : L2_Theme.borderDark);
            L2_Theme.drawCircle(c, cx, dotY, dotR, dotColor);

            // Number or check
            c.fontSize = L2_Theme.fontSmall;
            c.textColor = done || active ? '#FFFFFF' : L2_Theme.textDim;
            if (done) {
                L2_Theme.drawCheck(c, cx - 6, dotY - 6, 12, '#FFFFFF');
            } else {
                c.drawText(String(i + 1), cx - dotR, dotY - 7, dotR * 2, 14, 'center');
            }

            // Label
            c.fontSize = L2_Theme.fontSmall;
            c.textColor = active ? L2_Theme.textWhite : L2_Theme.textGray;
            c.drawText(label, cx - stepW / 2, dotY + dotR + 4, stepW, 16, 'center');
        });
    };

    window.L2_Steps = L2_Steps;
})();
