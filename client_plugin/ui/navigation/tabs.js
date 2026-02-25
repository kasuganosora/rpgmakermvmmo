/**
 * L2_Tabs - Tab bar for switching content panels.
 */
(function () {
    'use strict';

    function L2_Tabs() { this.initialize.apply(this, arguments); }
    L2_Tabs.prototype = Object.create(L2_Base.prototype);
    L2_Tabs.prototype.constructor = L2_Tabs;

    /**
     * @param {number} x
     * @param {number} y
     * @param {number} w
     * @param {object} [opts] - { tabs, onChange, activeIndex }
     */
    L2_Tabs.prototype.initialize = function (x, y, w, opts) {
        opts = opts || {};
        L2_Base.prototype.initialize.call(this, x, y, w, 28 + 16);
        this._tabs = opts.tabs || [];
        this._activeTab = opts.activeIndex || 0;
        this._hoverTab = -1;
        this._onChange = opts.onChange || null;
        this.refresh();
    };

    L2_Tabs.prototype.standardPadding = function () { return 0; };

    L2_Tabs.prototype.getActiveTab = function () { return this._activeTab; };
    L2_Tabs.prototype.getActiveLabel = function () { return this._tabs[this._activeTab]; };

    L2_Tabs.prototype.setActiveTab = function (idx) {
        if (idx === this._activeTab || idx < 0 || idx >= this._tabs.length || !this._tabs.length) return;
        this._activeTab = idx;
        if (this._onChange) this._onChange(idx, this._tabs[idx]);
        this.markDirty();
    };

    L2_Tabs.prototype.refresh = function () {
        var c = this.bmp();
        c.clear();
        var cw = this.cw(), ch = this.ch();
        var tw = Math.floor(cw / Math.max(this._tabs.length, 1));
        var self = this;

        c.fillRect(0, 0, cw, ch, L2_Theme.bgPanel);

        this._tabs.forEach(function (label, i) {
            var tx = i * tw;
            var active = i === self._activeTab;
            var hover = i === self._hoverTab;

            if (active) {
                c.fillRect(tx, 0, tw, ch, '#1E1E38');
                c.fillRect(tx, ch - 2, tw, 2, L2_Theme.textGold);
            } else if (hover) {
                c.fillRect(tx, 0, tw, ch, L2_Theme.highlight);
            }

            c.fontSize = L2_Theme.fontNormal;
            c.textColor = active ? L2_Theme.textGold : L2_Theme.textGray;
            c.drawText(label, tx, 0, tw, ch, 'center');

            if (i > 0) c.fillRect(tx, 4, 1, ch - 8, L2_Theme.borderDark);
        });
    };

    L2_Tabs.prototype.update = function () {
        L2_Base.prototype.update.call(this);
        if (!this.visible) return;

        var mx = TouchInput.x - this.x;
        var my = TouchInput.y - this.y;
        var tw = Math.floor(this.cw() / Math.max(this._tabs.length, 1));
        var inside = mx >= 0 && mx < this.width && my >= 0 && my < this.height;
        var oldHover = this._hoverTab;
        var tabIdx = -1;
        if (inside && this._tabs.length > 0) {
            tabIdx = Math.floor(mx / tw);
            tabIdx = Math.max(0, Math.min(tabIdx, this._tabs.length - 1));
        }
        this._hoverTab = tabIdx;
        if (this._hoverTab !== oldHover) this.markDirty();
        if (inside && TouchInput.isTriggered() && tabIdx >= 0) {
            this.setActiveTab(tabIdx);
        }
    };

    window.L2_Tabs = L2_Tabs;
})();
