/**
 * L2_Input - Canvas-rendered text input field.
 */
(function () {
    'use strict';

    function L2_Input() { this.initialize.apply(this, arguments); }
    L2_Input.prototype = Object.create(L2_Base.prototype);
    L2_Input.prototype.constructor = L2_Input;

    /**
     * @param {number} x
     * @param {number} y
     * @param {number} w
     * @param {object} [opts] - { placeholder, maxLength, onChange, onSubmit, password }
     */
    L2_Input.prototype.initialize = function (x, y, w, opts) {
        opts = opts || {};
        L2_Base.prototype.initialize.call(this, x, y, w, (opts.height || 28) + 4);
        this._text = '';
        this._placeholder = opts.placeholder || '';
        this._maxLength = opts.maxLength || 100;
        this._password = opts.password || false;
        this._focused = false;
        this._cursorBlink = 0;
        this._onChange = opts.onChange || null;
        this._onSubmit = opts.onSubmit || null;
        this.refresh();
    };

    L2_Input.prototype.standardPadding = function () { return 2; };

    L2_Input.prototype.getText = function () { return this._text; };
    L2_Input.prototype.setText = function (t) { this._text = String(t); this.refresh(); };
    L2_Input.prototype.setFocused = function (b) { this._focused = b; this._cursorBlink = 0; this.refresh(); };
    L2_Input.prototype.isFocused = function () { return this._focused; };

    L2_Input.prototype.refresh = function () {
        var c = this.bmp();
        c.clear();
        var cw = this.cw(), ch = this.ch();

        c.fillRect(0, 0, cw, ch, this._focused ? '#0E0E22' : L2_Theme.bgInput);
        L2_Theme.strokeRoundRect(c, 0, 0, cw, ch, 2,
            this._focused ? L2_Theme.borderActive : L2_Theme.borderDark);

        c.fontSize = L2_Theme.fontNormal;
        if (this._text) {
            c.textColor = L2_Theme.textWhite;
            var display = this._password ? '\u2022'.repeat(this._text.length) : this._text;
            if (this._focused && this._cursorBlink < 30) display += '|';
            c.drawText(display, 6, 0, cw - 12, ch, 'left');
        } else {
            c.textColor = L2_Theme.textDim;
            var ph = this._placeholder;
            if (this._focused && this._cursorBlink < 30) ph = '|';
            c.drawText(ph, 6, 0, cw - 12, ch, 'left');
        }
    };

    L2_Input.prototype.update = function () {
        L2_Base.prototype.update.call(this);
        if (!this.visible) return;

        if (TouchInput.isTriggered()) {
            this.setFocused(this.isInside(TouchInput.x, TouchInput.y));
        }

        if (!this._focused) return;

        this._cursorBlink = (this._cursorBlink + 1) % 60;
        if (this._cursorBlink === 0 || this._cursorBlink === 30) this.refresh();

        var changed = false;
        var chars = 'abcdefghijklmnopqrstuvwxyz0123456789';
        for (var ci = 0; ci < chars.length; ci++) {
            var ch = chars[ci];
            if (Input.isTriggered(ch) && this._text.length < this._maxLength) {
                this._text += ch;
                changed = true;
            }
        }
        // Space
        if (Input.isTriggered('space') && this._text.length < this._maxLength) {
            this._text += ' ';
            changed = true;
        }
        if (Input.isTriggered('backspace') && this._text.length > 0) {
            this._text = this._text.slice(0, -1);
            changed = true;
        }
        if (Input.isTriggered('ok')) {
            this._focused = false;
            if (this._onSubmit) this._onSubmit(this._text);
            this.refresh();
        }
        if (Input.isTriggered('escape')) {
            this._focused = false;
            this.refresh();
        }
        if (changed) {
            if (this._onChange) this._onChange(this._text);
            this.refresh();
        }
    };

    window.L2_Input = L2_Input;
})();
