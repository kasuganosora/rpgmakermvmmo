/**
 * L2_InputNumber - Numeric input with +/- buttons.
 */
(function () {
    'use strict';

    function L2_InputNumber() { this.initialize.apply(this, arguments); }
    L2_InputNumber.prototype = Object.create(L2_Base.prototype);
    L2_InputNumber.prototype.constructor = L2_InputNumber;

    /**
     * @param {number} x
     * @param {number} y
     * @param {object} [opts] - { min, max, step, value, width, onChange }
     */
    L2_InputNumber.prototype.initialize = function (x, y, opts) {
        opts = opts || {};
        var w = opts.width || 130;
        L2_Base.prototype.initialize.call(this, x, y, w, 28 + 4);
        this._min = opts.min !== undefined ? opts.min : 0;
        this._max = opts.max !== undefined ? opts.max : 99999;
        this._step = opts.step || 1;
        this._value = opts.value || this._min;
        this._onChange = opts.onChange || null;
        this._focused = false;
        this._text = String(this._value);
        this._hoverBtn = 0; // -1=minus, 1=plus
        this._cursorBlink = 0;
        this.refresh();
    };

    L2_InputNumber.prototype.standardPadding = function () { return 2; };

    L2_InputNumber.prototype.getValue = function () { return this._value; };
    L2_InputNumber.prototype.setValue = function (v) {
        var newVal = Math.max(this._min, Math.min(v, this._max));
        if (newVal === this._value) return;
        this._value = newVal;
        this._text = String(this._value);
        if (this._onChange) this._onChange(this._value);
        this.markDirty();
    };

    L2_InputNumber.prototype.refresh = function () {
        var c = this.bmp();
        c.clear();
        var cw = this.cw(), ch = this.ch();
        var btnW = 24;

        // Minus button
        var minusBg = this._hoverBtn === -1 ? L2_Theme.bgButtonHover : L2_Theme.bgButton;
        L2_Theme.fillRoundRect(c, 0, 0, btnW, ch, 2, minusBg);
        L2_Theme.strokeRoundRect(c, 0, 0, btnW, ch, 2, L2_Theme.borderDark);
        c.fontSize = L2_Theme.fontNormal;
        c.textColor = L2_Theme.textWhite;
        c.drawText('-', 0, 0, btnW, ch, 'center');

        // Number field
        var fieldX = btnW + 2;
        var fieldW = cw - btnW * 2 - 4;
        c.fillRect(fieldX, 0, fieldW, ch, this._focused ? '#0E0E22' : L2_Theme.bgInput);
        L2_Theme.strokeRoundRect(c, fieldX, 0, fieldW, ch, 2,
            this._focused ? L2_Theme.borderActive : L2_Theme.borderDark);
        c.fontSize = L2_Theme.fontNormal;
        c.textColor = L2_Theme.textWhite;
        var display = this._focused ? this._text : String(this._value);
        if (this._focused && this._cursorBlink < 30) display += '|';
        c.drawText(display, fieldX + 4, 0, fieldW - 8, ch, 'center');

        // Plus button
        var plusX = cw - btnW;
        var plusBg = this._hoverBtn === 1 ? L2_Theme.bgButtonHover : L2_Theme.bgButton;
        L2_Theme.fillRoundRect(c, plusX, 0, btnW, ch, 2, plusBg);
        L2_Theme.strokeRoundRect(c, plusX, 0, btnW, ch, 2, L2_Theme.borderDark);
        c.textColor = L2_Theme.textWhite;
        c.drawText('+', plusX, 0, btnW, ch, 'center');
    };

    L2_InputNumber.prototype.update = function () {
        L2_Base.prototype.update.call(this);
        if (!this.visible) return;

        var loc = this.toLocal(TouchInput.x, TouchInput.y);
        var cw = this.cw(), ch = this.ch();
        var btnW = 24;

        // Hover
        var oldHover = this._hoverBtn;
        this._hoverBtn = 0;
        if (loc.y >= 0 && loc.y < ch) {
            if (loc.x >= 0 && loc.x < btnW) this._hoverBtn = -1;
            else if (loc.x >= cw - btnW) this._hoverBtn = 1;
        }
        if (this._hoverBtn !== oldHover) this.markDirty();

        if (TouchInput.isTriggered()) {
            if (this._hoverBtn === -1) {
                this.setValue(this._value - this._step);
                return;
            }
            if (this._hoverBtn === 1) {
                this.setValue(this._value + this._step);
                return;
            }
            var insideField = loc.x >= btnW && loc.x < cw - btnW && loc.y >= 0 && loc.y < ch;
            this._focused = insideField;
            if (this._focused) this._text = String(this._value);
            this._cursorBlink = 0;
            this.markDirty();
        }

        if (!this._focused) return;

        this._cursorBlink = (this._cursorBlink + 1) % 60;
        if (this._cursorBlink === 0 || this._cursorBlink === 30) this.markDirty();

        var changed = false;
        for (var k = 0; k <= 9; k++) {
            if (Input.isTriggered(String(k))) {
                if (this._text === '0') this._text = '';
                this._text += String(k);
                changed = true;
            }
        }
        if (Input.isTriggered('backspace')) {
            this._text = this._text.slice(0, -1) || '0';
            changed = true;
        }
        if (Input.isTriggered('ok') || Input.isTriggered('escape')) {
            this._focused = false;
            this._value = Math.max(this._min, Math.min(parseInt(this._text, 10) || 0, this._max));
            this._text = String(this._value);
            if (this._onChange) this._onChange(this._value);
            this.markDirty();
        }
        if (changed) this.markDirty();
    };

    window.L2_InputNumber = L2_InputNumber;
})();
