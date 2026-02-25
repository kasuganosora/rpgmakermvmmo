/**
 * L2_Layout - Flex-like layout container (horizontal/vertical).
 * Positions children sequentially with gap spacing.
 */
(function () {
    'use strict';

    function L2_Layout() { this.initialize.apply(this, arguments); }
    L2_Layout.prototype = Object.create(L2_Base.prototype);
    L2_Layout.prototype.constructor = L2_Layout;

    /**
     * @param {number} x
     * @param {number} y
     * @param {number} w
     * @param {number} h
     * @param {object} [opts] - { direction:'horizontal'|'vertical', gap, align:'start'|'center'|'end' }
     */
    L2_Layout.prototype.initialize = function (x, y, w, h, opts) {
        L2_Base.prototype.initialize.call(this, x, y, w, h);
        opts = opts || {};
        this._direction = opts.direction || 'horizontal';
        this._gap = opts.gap !== undefined ? opts.gap : L2_Theme.defaultGap;
        this._align = opts.align || 'start';
        this._managed = [];
    };

    L2_Layout.prototype.standardPadding = function () { return 0; };

    L2_Layout.prototype.addItem = function (component) {
        this._managed.push(component);
        this.addChild(component);
        this.layoutItems();
    };

    L2_Layout.prototype.clearItems = function () {
        var self = this;
        this._managed.forEach(function (c) {
            if (c.parent === self) self.removeChild(c);
        });
        this._managed = [];
    };

    L2_Layout.prototype.layoutItems = function () {
        var pos = 0;
        var isH = this._direction === 'horizontal';
        var containerSize = isH ? this.ch() : this.cw();

        for (var i = 0; i < this._managed.length; i++) {
            var item = this._managed[i];
            var itemSize = isH ? item.height : item.width;
            var crossPos = 0;

            if (this._align === 'center') {
                crossPos = (containerSize - itemSize) / 2;
            } else if (this._align === 'end') {
                crossPos = containerSize - itemSize;
            }

            if (isH) {
                item.x = pos;
                item.y = crossPos;
            } else {
                item.x = crossPos;
                item.y = pos;
            }
            pos += (isH ? item.width : item.height) + this._gap;
        }
    };

    L2_Layout.prototype.refresh = function () { this.bmp().clear(); };

    window.L2_Layout = L2_Layout;
})();
