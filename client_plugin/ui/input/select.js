/**
 * L2_Select - Dropdown selection component.
 */
(function () {
    'use strict';

    function L2_Select() { this.initialize.apply(this, arguments); }
    L2_Select.prototype = Object.create(L2_Base.prototype);
    L2_Select.prototype.constructor = L2_Select;

    /**
     * @param {number} x
     * @param {number} y
     * @param {number} w
     * @param {object} [opts] - { options: [{label,value}], selected, placeholder, onChange }
     */
    L2_Select.prototype.initialize = function (x, y, w, opts) {
        opts = opts || {};
        L2_Base.prototype.initialize.call(this, x, y, w, 28 + 4);
        this._options = opts.options || [];
        this._selectedIndex = opts.value !== undefined ? opts.value : (opts.selectedIndex !== undefined ? opts.selectedIndex : -1);
        this._placeholder = opts.placeholder || '\u8BF7\u9009\u62E9...';
        this._onChange = opts.onChange || null;
        this._open = false;
        this._hoverOption = -1;
        this._itemHeight = 26;
        this.refresh();
    };

    L2_Select.prototype.standardPadding = function () { return 2; };

    L2_Select.prototype.getSelected = function () {
        if (!this._options || this._selectedIndex < 0 || this._selectedIndex >= this._options.length) {
            return null;
        }
        return this._options[this._selectedIndex];
    };

    L2_Select.prototype.getValue = function () {
        var sel = this.getSelected();
        return sel ? sel.value : null;
    };

    L2_Select.prototype.setSelectedIndex = function (idx) {
        if (idx === this._selectedIndex) return;
        this._selectedIndex = idx;
        if (this._onChange) this._onChange(this.getSelected(), idx);
        this.markDirty();
    };

    /** @deprecated Use setSelectedIndex instead */
    L2_Select.prototype.setValue = function (idx) {
        this.setSelectedIndex(idx);
    };

    L2_Select.prototype.refresh = function () {
        var c = this.bmp();
        c.clear();
        var cw = this.cw(), ch = this.ch();
        var sel = this.getSelected();

        // Trigger
        L2_Theme.fillRoundRect(c, 0, 0, cw, 28, L2_Theme.cornerRadius, L2_Theme.bgInput);
        L2_Theme.strokeRoundRect(c, 0, 0, cw, 28, L2_Theme.cornerRadius, L2_Theme.borderDark);
        c.fontSize = L2_Theme.fontNormal;
        c.textColor = sel ? L2_Theme.textWhite : L2_Theme.textDim;
        c.drawText(sel ? sel.label : this._placeholder, 8, 4, cw - 28, 20, 'left');
        c.textColor = L2_Theme.textGray;
        c.drawText(this._open ? '\u25B2' : '\u25BC', cw - 22, 4, 18, 20, 'center');

        // Dropdown
        if (this._open) {
            var listH = this._options.length * this._itemHeight + 4;
            var listY = 30;
            L2_Theme.fillRoundRect(c, 0, listY, cw, listH, L2_Theme.cornerRadius, L2_Theme.bgPanel);
            L2_Theme.strokeRoundRect(c, 0, listY, cw, listH, L2_Theme.cornerRadius, L2_Theme.borderDark);

            var ih = this._itemHeight;
            var self = this;
            (this._options || []).forEach(function (opt, i) {
                var iy = listY + 2 + i * ih;
                if (i === self._selectedIndex) {
                    c.fillRect(2, iy, cw - 4, ih, L2_Theme.selection);
                } else if (i === self._hoverOption) {
                    c.fillRect(2, iy, cw - 4, ih, L2_Theme.highlight);
                }
                c.fontSize = L2_Theme.fontNormal;
                c.textColor = i === self._selectedIndex ? L2_Theme.textGold : L2_Theme.textWhite;
                c.drawText(opt.label, 10, iy + 3, cw - 20, ih - 6, 'left');
            });
        }
    };

    L2_Select.prototype.update = function () {
        L2_Base.prototype.update.call(this);
        if (!this.visible) return;

        var loc = this.toLocal(TouchInput.x, TouchInput.y);
        var cw = this.cw();
        var ih = this._itemHeight;

        if (this._open) {
            var listY = 30;
            var oldHover = this._hoverOption;
            if (loc.x >= 0 && loc.x < cw && loc.y >= listY) {
                this._hoverOption = Math.floor((loc.y - listY - 2) / ih);
                if (this._hoverOption >= this._options.length) this._hoverOption = -1;
            } else {
                this._hoverOption = -1;
            }
            if (this._hoverOption !== oldHover) this.markDirty();
        }

        if (TouchInput.isTriggered()) {
            var insideTrigger = loc.x >= 0 && loc.x < cw && loc.y >= 0 && loc.y < 28;
            if (insideTrigger) {
                this._open = !this._open;
                var totalH = this._open ? 32 + (this._options ? this._options.length : 0) * ih + 8 : 32;
                this.move(this.x, this.y, this.width, totalH);
                this.createContents();
                this.markDirty();
                return;
            }
            if (this._open && this._hoverOption >= 0 && this._options && this._hoverOption < this._options.length) {
                this.setSelectedIndex(this._hoverOption);
                this._open = false;
                this.move(this.x, this.y, this.width, 32);
                this.createContents();
                this.refresh();
                return;
            }
            if (this._open) {
                this._open = false;
                this.move(this.x, this.y, this.width, 32);
                this.createContents();
                this.refresh();
            }
        }
    };

    window.L2_Select = L2_Select;
})();
