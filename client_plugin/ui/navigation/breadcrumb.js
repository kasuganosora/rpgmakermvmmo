/**
 * L2_Breadcrumb - Path breadcrumb navigation.
 */
(function () {
    'use strict';

    function L2_Breadcrumb() { this.initialize.apply(this, arguments); }
    L2_Breadcrumb.prototype = Object.create(L2_Base.prototype);
    L2_Breadcrumb.prototype.constructor = L2_Breadcrumb;

    /**
     * @param {number} x
     * @param {number} y
     * @param {object} [opts] - { items: [{label, action}] or string[], width }
     */
    L2_Breadcrumb.prototype.initialize = function (x, y, opts) {
        opts = opts || {};
        var items = opts.items || [];
        // Support string[] shorthand
        if (items.length > 0 && typeof items[0] === 'string') {
            items = items.map(function (s) { return { label: s }; });
        }
        var w = opts.width || 200;
        L2_Base.prototype.initialize.call(this, x, y, w, 24 + 4);
        this._items = items;
        this._hoverIndex = -1;
        this.refresh();
    };

    L2_Breadcrumb.prototype.standardPadding = function () { return 2; };

    L2_Breadcrumb.prototype.setItems = function (items) {
        this._items = items || [];
        this.refresh();
    };

    L2_Breadcrumb.prototype.refresh = function () {
        var c = this.bmp();
        c.clear();
        var ch = this.ch();
        var x = 0;
        var self = this;

        this._items.forEach(function (item, i) {
            var isLast = i === self._items.length - 1;
            var isHover = i === self._hoverIndex;

            c.fontSize = L2_Theme.fontNormal;
            c.textColor = isLast ? L2_Theme.textWhite :
                          (isHover ? L2_Theme.textLinkHover : L2_Theme.textLink);
            var tw = L2_Theme.measureText(c, item.label, L2_Theme.fontNormal);
            c.drawText(item.label, x, 0, tw + 4, ch, 'left');

            // Store hit area
            item._x = x;
            item._w = tw + 4;

            x += tw + 4;
            if (!isLast) {
                c.textColor = L2_Theme.textDim;
                c.drawText(' / ', x, 0, 20, ch, 'left');
                x += 18;
            }
        });
    };

    L2_Breadcrumb.prototype.update = function () {
        L2_Base.prototype.update.call(this);
        if (!this.visible) return;
        var loc = this.toLocal(TouchInput.x, TouchInput.y);
        var ch = this.ch();
        var inside = loc.y >= 0 && loc.y < ch;
        var oldHover = this._hoverIndex;
        this._hoverIndex = -1;

        if (inside) {
            for (var i = 0; i < this._items.length - 1; i++) {
                var item = this._items[i];
                if (loc.x >= item._x && loc.x < item._x + item._w) {
                    this._hoverIndex = i;
                    break;
                }
            }
        }
        if (this._hoverIndex !== oldHover) this.refresh();
        if (TouchInput.isTriggered() && this._hoverIndex >= 0) {
            var clicked = this._items[this._hoverIndex];
            if (clicked && clicked.action) clicked.action();
        }
    };

    window.L2_Breadcrumb = L2_Breadcrumb;
})();
