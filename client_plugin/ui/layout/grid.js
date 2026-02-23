/**
 * L2_Grid - Grid layout container for arranging children in rows/columns.
 * Does not render itself - positions children on refresh.
 */
(function () {
    'use strict';

    function L2_Grid() { this.initialize.apply(this, arguments); }
    L2_Grid.prototype = Object.create(L2_Base.prototype);
    L2_Grid.prototype.constructor = L2_Grid;

    /**
     * @param {number} x
     * @param {number} y
     * @param {number} w
     * @param {number} h
     * @param {object} [opts] - { cols, rowGap, colGap }
     */
    L2_Grid.prototype.initialize = function (x, y, w, h, opts) {
        L2_Base.prototype.initialize.call(this, x, y, w, h);
        opts = opts || {};
        this._cols = opts.cols || 1;
        this._rowGap = opts.rowGap !== undefined ? opts.rowGap : 4;
        this._colGap = opts.colGap !== undefined ? opts.colGap : 4;
        this._managed = [];
    };

    L2_Grid.prototype.standardPadding = function () { return 0; };

    /** Add a component to the grid. */
    L2_Grid.prototype.addItem = function (component) {
        this._managed.push(component);
        this.addChild(component);
        this.layoutItems();
    };

    /** Remove all grid items. */
    L2_Grid.prototype.clearItems = function () {
        var self = this;
        this._managed.forEach(function (c) {
            if (c.parent === self) self.removeChild(c);
        });
        this._managed = [];
    };

    /** Recalculate positions of all managed children. */
    L2_Grid.prototype.layoutItems = function () {
        var cols = this._cols;
        var cw = this.cw();
        var cellW = (cw - (cols - 1) * this._colGap) / cols;

        for (var i = 0; i < this._managed.length; i++) {
            var col = i % cols;
            var row = Math.floor(i / cols);
            var cx = col * (cellW + this._colGap);
            var cy = row * (this._managed[i].height + this._rowGap);
            this._managed[i].x = cx;
            this._managed[i].y = cy;
        }
    };

    /** Get all managed items. */
    L2_Grid.prototype.getItems = function () { return this._managed; };

    L2_Grid.prototype.refresh = function () {
        this.bmp().clear();
    };

    window.L2_Grid = L2_Grid;
})();
