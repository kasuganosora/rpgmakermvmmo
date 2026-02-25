/**
 * L2_Checkbox - Toggle checkbox with label.
 */
(function () {
    'use strict';

    function L2_Checkbox() { this.initialize.apply(this, arguments); }
    L2_Checkbox.prototype = Object.create(L2_Base.prototype);
    L2_Checkbox.prototype.constructor = L2_Checkbox;

    /**
     * @param {number} x
     * @param {number} y
     * @param {object} [opts] - { label, checked, onChange }
     */
    L2_Checkbox.prototype.initialize = function (x, y, opts) {
        opts = opts || {};
        this._label = opts.label || '';
        var w = opts.width || Math.max(this._label.length * 10 + 28, 60);
        L2_Base.prototype.initialize.call(this, x, y, w, 24 + 4);
        this._checked = opts.checked || false;
        this._onChange = opts.onChange || null;
        this._hover = false;
        this.refresh();
    };

    L2_Checkbox.prototype.standardPadding = function () { return 2; };

    L2_Checkbox.prototype.isChecked = function () { return this._checked; };
    L2_Checkbox.prototype.setChecked = function (b) {
        this._checked = b;
        if (this._onChange) this._onChange(b);
        this.markDirty();
    };

    L2_Checkbox.prototype.refresh = function () {
        var c = this.bmp();
        c.clear();
        var ch = this.ch();
        var boxSize = 16;
        var boxY = (ch - boxSize) / 2;

        // Box
        var bg = this._checked ? '#1A2A55' : L2_Theme.bgInput;
        var border = this._checked ? L2_Theme.borderActive :
                     (this._hover ? L2_Theme.borderGold : L2_Theme.borderDark);
        L2_Theme.fillRoundRect(c, 0, boxY, boxSize, boxSize, 2, bg);
        L2_Theme.strokeRoundRect(c, 0, boxY, boxSize, boxSize, 2, border);

        if (this._checked) {
            L2_Theme.drawCheck(c, 0, boxY, boxSize, L2_Theme.textGold);
        }

        // Label
        c.fontSize = L2_Theme.fontNormal;
        c.textColor = L2_Theme.textWhite;
        c.drawText(this._label, boxSize + 8, 0, this.cw() - boxSize - 8, ch, 'left');
    };

    L2_Checkbox.prototype.update = function () {
        L2_Base.prototype.update.call(this);
        if (!this.visible) return;
        var wasHover = this._hover;
        this._hover = this.isInside(TouchInput.x, TouchInput.y);
        if (this._hover !== wasHover) this.markDirty();
        if (this._hover && TouchInput.isTriggered()) {
            this.setChecked(!this._checked);
        }
    };

    window.L2_Checkbox = L2_Checkbox;
})();
