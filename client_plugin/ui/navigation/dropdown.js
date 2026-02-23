/**
 * L2_Dropdown - Dropdown menu triggered by clicking a button.
 */
(function () {
    'use strict';

    function L2_Dropdown() { this.initialize.apply(this, arguments); }
    L2_Dropdown.prototype = Object.create(L2_Base.prototype);
    L2_Dropdown.prototype.constructor = L2_Dropdown;

    /**
     * @param {number} x
     * @param {number} y
     * @param {number} w
     * @param {string} label - trigger button label
     * @param {Array} options - [{label, action, color}]
     */
    L2_Dropdown.prototype.initialize = function (x, y, w, label, options) {
        L2_Base.prototype.initialize.call(this, x, y, w, 28 + 4);
        this._label = label || '';
        this._options = options || [];
        this._open = false;
        this._hoverOption = -1;
        this._itemHeight = 26;
        this.refresh();
    };

    L2_Dropdown.prototype.standardPadding = function () { return 2; };

    L2_Dropdown.prototype.refresh = function () {
        var c = this.bmp();
        c.clear();
        var cw = this.cw(), ch = this.ch();

        // Trigger button
        L2_Theme.fillRoundRect(c, 0, 0, cw, 28, L2_Theme.cornerRadius, L2_Theme.bgButton);
        L2_Theme.strokeRoundRect(c, 0, 0, cw, 28, L2_Theme.cornerRadius, L2_Theme.borderDark);
        c.fontSize = L2_Theme.fontNormal;
        c.textColor = L2_Theme.textWhite;
        c.drawText(this._label, 8, 4, cw - 28, 20, 'left');
        // Arrow
        c.textColor = L2_Theme.textGray;
        c.drawText(this._open ? '\u25B2' : '\u25BC', cw - 22, 4, 18, 20, 'center');

        // Dropdown list
        if (this._open) {
            var listH = this._options.length * this._itemHeight + 4;
            var listY = 30;
            L2_Theme.fillRoundRect(c, 0, listY, cw, listH, L2_Theme.cornerRadius, L2_Theme.bgPanel);
            L2_Theme.strokeRoundRect(c, 0, listY, cw, listH, L2_Theme.cornerRadius, L2_Theme.borderDark);

            var ih = this._itemHeight;
            var self = this;
            this._options.forEach(function (opt, i) {
                var iy = listY + 2 + i * ih;
                if (i === self._hoverOption) {
                    c.fillRect(2, iy, cw - 4, ih, L2_Theme.highlight);
                }
                c.fontSize = L2_Theme.fontNormal;
                c.textColor = opt.color || L2_Theme.textWhite;
                c.drawText(opt.label, 10, iy + 3, cw - 20, ih - 6, 'left');
            });
        }
    };

    L2_Dropdown.prototype.update = function () {
        L2_Base.prototype.update.call(this);
        if (!this.visible) return;

        var loc = this.toLocal(TouchInput.x, TouchInput.y);
        var cw = this.cw();
        var ih = this._itemHeight;

        if (this._open) {
            var listY = 30;
            var insideList = loc.x >= 0 && loc.x < cw &&
                            loc.y >= listY && loc.y < listY + this._options.length * ih + 4;
            var oldHover = this._hoverOption;
            this._hoverOption = insideList ? Math.floor((loc.y - listY - 2) / ih) : -1;
            if (this._hoverOption >= this._options.length) this._hoverOption = -1;
            if (this._hoverOption !== oldHover) this.refresh();
        }

        if (TouchInput.isTriggered()) {
            var insideTrigger = loc.x >= 0 && loc.x < cw && loc.y >= 0 && loc.y < 28;
            if (insideTrigger) {
                this._open = !this._open;
                // Resize to fit dropdown
                var totalH = this._open ? 32 + this._options.length * ih + 8 : 32;
                this.move(this.x, this.y, this.width, totalH);
                this.createContents();
                this.refresh();
                return;
            }
            if (this._open && this._hoverOption >= 0) {
                var opt = this._options[this._hoverOption];
                this._open = false;
                this.move(this.x, this.y, this.width, 32);
                this.createContents();
                this.refresh();
                if (opt && opt.action) opt.action();
                return;
            }
            // Click outside closes
            if (this._open) {
                this._open = false;
                this.move(this.x, this.y, this.width, 32);
                this.createContents();
                this.refresh();
            }
        }
    };

    window.L2_Dropdown = L2_Dropdown;
})();
