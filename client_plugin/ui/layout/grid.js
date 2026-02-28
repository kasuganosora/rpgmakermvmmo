/**
 * L2_Grid - Grid layout container for arranging children in rows/columns.
 * Auto-sizes columns and handles responsive layout.
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
        opts = opts || {};
        L2_Base.prototype.initialize.call(this, x, y, w, h);
        this._cols = Math.max(1, opts.cols || 1);
        this._rowGap = opts.rowGap !== undefined ? opts.rowGap : L2_Theme.defaultGap;
        this._colGap = opts.colGap !== undefined ? opts.colGap : L2_Theme.defaultGap;
        this._managed = [];
        this._cellWidth = 0;
        this._lastCellWidth = 0;
        this._calculateCellWidth();
    };

    L2_Grid.prototype.standardPadding = function () { return 0; };

    /** Recalculate cell width when container size changes */
    L2_Grid.prototype._calculateCellWidth = function () {
        var cw = this.cw();
        var cols = this._cols;
        // 均分计算，最后一个 cell 吃掉剩余像素避免空白
        this._cellWidth = Math.floor((cw - (cols - 1) * this._colGap) / cols);
        this._lastCellWidth = cw - (cols - 1) * (this._cellWidth + this._colGap);
    };

    /** Get cell width for a specific column (last column may be wider) */
    L2_Grid.prototype._getCellWidth = function (colIndex) {
        return colIndex === this._cols - 1 ? this._lastCellWidth : this._cellWidth;
    };

    /** Add a component to the grid. */
    L2_Grid.prototype.addItem = function (component) {
        this._managed.push(component);
        this.addChild(component);
        this.layoutItems();
    };

    /** Batch add multiple items (avoids O(N²) layout recalc). */
    L2_Grid.prototype.addItems = function (components) {
        for (var i = 0; i < components.length; i++) {
            this._managed.push(components[i]);
            this.addChild(components[i]);
        }
        this.layoutItems();
    };

    /** Remove all grid items. */
    L2_Grid.prototype.clearItems = function () {
        var self = this;
        this._managed.forEach(function (c) {
            if (c.parent === self) self.removeChild(c);
            if (c.destroy) c.destroy();
        });
        this._managed = [];
    };

    /** Update column count and recalculate layout. */
    L2_Grid.prototype.setColumns = function (cols) {
        this._cols = Math.max(1, cols || 1);
        this._calculateCellWidth();
        this.layoutItems();
    };

    /** Recalculate positions of all managed children. */
    L2_Grid.prototype.layoutItems = function () {
        this._calculateCellWidth();
        var cols = this._cols;
        var rowY = 0;
        var lastRow = -1;
        var rowHeight = 0;

        for (var i = 0; i < this._managed.length; i++) {
            var col = i % cols;
            var row = Math.floor(i / cols);
            var cellW = this._getCellWidth(col);
            var cx = col * (this._cellWidth + this._colGap);

            // 换行时累计上一行的最大高度
            if (row !== lastRow) {
                if (lastRow >= 0) {
                    rowY += rowHeight + this._rowGap;
                }
                lastRow = row;
                rowHeight = 0;
            }
            rowHeight = Math.max(rowHeight, this._managed[i].height);

            this._managed[i].x = cx;
            this._managed[i].y = rowY;

            // 自动调整子元素宽度以适应 cell
            if (this._managed[i].width !== cellW) {
                this._managed[i].width = cellW;
                if (this._managed[i].refresh) {
                    this._managed[i].refresh();
                }
            }
        }
    };

    /** Handle resize - recalculate and relayout */
    L2_Grid.prototype.onResize = function () {
        this._calculateCellWidth();
        this.layoutItems();
    };

    /** Get all managed items. */
    L2_Grid.prototype.getItems = function () { return this._managed; };

    L2_Grid.prototype.refresh = function () {
        this.bmp().clear();
    };

    window.L2_Grid = L2_Grid;
})();
