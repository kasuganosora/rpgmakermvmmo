/**
 * L2_Switch - Toggle switch (on/off).
 */
(function () {
    'use strict';

    function L2_Switch() { this.initialize.apply(this, arguments); }
    L2_Switch.prototype = Object.create(L2_Base.prototype);
    L2_Switch.prototype.constructor = L2_Switch;

    /**
     * @param {number} x
     * @param {number} y
     * @param {object} [opts] - { on, label, onChange }
     */
    L2_Switch.prototype.initialize = function (x, y, opts) {
        opts = opts || {};
        this._label = opts.label || '';
        L2_Base.prototype.initialize.call(this, x, y, (opts.width || 90) + 4, 24 + 4);
        this._on = opts.on || opts.value || false;
        this._onChange = opts.onChange || null;
        this._hover = false;
        this._animPos = this._on ? 1 : 0;
        this.refresh();
    };

    L2_Switch.prototype.standardPadding = function () { return 2; };

    L2_Switch.prototype.isOn = function () { return this._on; };
    L2_Switch.prototype.setOn = function (b) {
        this._on = b;
        if (this._onChange) this._onChange(b);
        this.refresh();
    };

    L2_Switch.prototype.refresh = function () {
        var c = this.bmp();
        c.clear();
        var ch = this.ch();
        var trackW = 36, trackH = 18;
        var trackY = (ch - trackH) / 2;
        var knobR = 7;

        // Track
        var trackColor = this._on ? '#1A4A2A' : '#2A2A3A';
        var borderColor = this._on ? L2_Theme.textGreen : L2_Theme.borderDark;
        L2_Theme.fillRoundRect(c, 0, trackY, trackW, trackH, trackH / 2, trackColor);
        L2_Theme.strokeRoundRect(c, 0, trackY, trackW, trackH, trackH / 2, borderColor);

        // Knob
        var knobX = this._on ? trackW - knobR - 3 : knobR + 3;
        var knobColor = this._on ? L2_Theme.textGreen : L2_Theme.textGray;
        L2_Theme.drawCircle(c, knobX, trackY + trackH / 2, knobR, knobColor);

        // Label
        c.fontSize = L2_Theme.fontNormal;
        c.textColor = L2_Theme.textWhite;
        c.drawText(this._label, trackW + 8, 0, this.cw() - trackW - 8, ch, 'left');
    };

    L2_Switch.prototype.update = function () {
        L2_Base.prototype.update.call(this);
        if (!this.visible) return;
        var wasHover = this._hover;
        this._hover = this.isInside(TouchInput.x, TouchInput.y);
        if (this._hover !== wasHover) this.refresh();
        if (this._hover && TouchInput.isTriggered()) {
            this.setOn(!this._on);
        }
    };

    window.L2_Switch = L2_Switch;
})();
