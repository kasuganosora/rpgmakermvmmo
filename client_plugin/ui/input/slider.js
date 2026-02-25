/**
 * L2_Slider - Draggable range slider.
 */
(function () {
    'use strict';

    function L2_Slider() { this.initialize.apply(this, arguments); }
    L2_Slider.prototype = Object.create(L2_Base.prototype);
    L2_Slider.prototype.constructor = L2_Slider;

    /**
     * @param {number} x
     * @param {number} y
     * @param {number} w
     * @param {object} [opts] - { min, max, value, step, showValue, onChange }
     */
    L2_Slider.prototype.initialize = function (x, y, w, opts) {
        L2_Base.prototype.initialize.call(this, x, y, w, 28 + 4);
        opts = opts || {};
        this._min = opts.min !== undefined ? opts.min : 0;
        this._max = opts.max !== undefined ? opts.max : 100;
        this._value = opts.value !== undefined ? opts.value : this._min;
        this._step = opts.step || 1;
        this._showValue = opts.showValue !== false;
        this._onChange = opts.onChange || null;
        this._dragging = false;
        this.refresh();
    };

    L2_Slider.prototype.standardPadding = function () { return 2; };

    L2_Slider.prototype.getValue = function () { return this._value; };
    L2_Slider.prototype.setValue = function (v) {
        v = Math.max(this._min, Math.min(v, this._max));
        v = Math.round(v / this._step) * this._step;
        if (v !== this._value) {
            this._value = v;
            if (this._onChange) this._onChange(v);
            this.markDirty();
        }
    };

    L2_Slider.prototype.refresh = function () {
        var c = this.bmp();
        c.clear();
        var cw = this.cw(), ch = this.ch();
        var valueW = this._showValue ? 40 : 0;
        var trackW = cw - valueW;
        var trackH = 6;
        var trackY = ch / 2 - trackH / 2;
        var ratio = (this._value - this._min) / Math.max(this._max - this._min, 1);
        var knobX = Math.round(ratio * (trackW - 8)) + 4;
        var knobR = 8;

        // Track bg
        L2_Theme.fillRoundRect(c, 0, trackY, trackW, trackH, 3, L2_Theme.borderDark);
        // Track fill
        L2_Theme.fillRoundRect(c, 0, trackY, knobX, trackH, 3, L2_Theme.textBlue);
        // Knob
        L2_Theme.drawCircle(c, knobX, ch / 2, knobR, '#FFFFFF');
        L2_Theme.drawCircle(c, knobX, ch / 2, knobR - 2, L2_Theme.textBlue);

        // Value label
        if (this._showValue) {
            c.fontSize = L2_Theme.fontSmall;
            c.textColor = L2_Theme.textWhite;
            c.drawText(String(this._value), trackW + 4, 0, valueW - 4, ch, 'left');
        }
    };

    L2_Slider.prototype.update = function () {
        L2_Base.prototype.update.call(this);
        if (!this.visible) return;

        var loc = this.toLocal(TouchInput.x, TouchInput.y);
        var cw = this.cw();
        var valueW = this._showValue ? 40 : 0;
        var trackW = cw - valueW;

        if (TouchInput.isTriggered() && this.isInside(TouchInput.x, TouchInput.y)) {
            this._dragging = true;
        }
        if (!TouchInput.isPressed()) this._dragging = false;

        if (this._dragging) {
            var ratio = Math.max(0, Math.min((loc.x - 4) / (trackW - 8), 1));
            var val = this._min + ratio * (this._max - this._min);
            this.setValue(val);
        }
    };

    window.L2_Slider = L2_Slider;
})();
