/**
 * L2_Table - Data table with columns and rows.
 */
(function () {
    'use strict';

    function L2_Table() { this.initialize.apply(this, arguments); }
    L2_Table.prototype = Object.create(L2_Base.prototype);
    L2_Table.prototype.constructor = L2_Table;

    /**
     * @param {number} x
     * @param {number} y
     * @param {number} w
     * @param {number} h
     * @param {object} [opts] - { columns: [{key, label, width}], rowHeight, onRowClick }
     */
    L2_Table.prototype.initialize = function (x, y, w, h, opts) {
        L2_Base.prototype.initialize.call(this, x, y, w, h);
        opts = opts || {};
        this._columns = opts.columns || [];
        this._rows = [];
        this._rowHeight = opts.rowHeight || L2_Theme.defaultItemHeight;
        this._headerHeight = 26;
        this._scrollY = 0;
        this._selectedRow = -1;
        this._hoverRow = -1;
        this._onRowClick = opts.onRowClick || null;
        this.refresh();
    };

    L2_Table.prototype.standardPadding = function () { return 4; };

    L2_Table.prototype.setRows = function (rows) {
        this._rows = rows || [];
        this._scrollY = 0;
        this._selectedRow = -1;
        this.markDirty();
    };

    L2_Table.prototype.refresh = function () {
        var c = this.bmp();
        var cw = this.cw(), ch = this.ch();
        var hh = this._headerHeight;
        var rh = this._rowHeight;

        // 只有背景层脏时才重绘背景和表头
        if (this.isLayerDirty('bg')) {
            c.clear();
            c.fillRect(0, 0, cw, ch, L2_Theme.bgDark);
            L2_Theme.strokeRoundRect(c, 0, 0, cw, ch, 2, L2_Theme.borderDark);

            // Header
            c.fillRect(0, 0, cw, hh, '#1A1A30');
            c.fillRect(0, hh - 1, cw, 1, L2_Theme.borderDark);

            var colX = 0;
            var self = this;
            this._columns.forEach(function (col) {
                var colW = col.width || Math.floor(cw / self._columns.length);
                c.fontSize = L2_Theme.fontSmall;
                c.textColor = L2_Theme.textGold;
                c.drawText(col.label || col.key, colX + 4, 4, colW - 8, hh - 8, 'left');
                colX += colW;
            });
            this.markLayerClean('bg');
        }

        // 内容层需要重绘行数据
        if (this.isLayerDirty('content')) {
            // 重新绘制数据区域（清除之前的行数据）
            c.fillRect(0, hh, cw, ch - hh, L2_Theme.bgDark);
            
            // Rows
            var bodyH = ch - hh;
            var startIdx = Math.floor(this._scrollY / rh);
            var visCount = Math.ceil(bodyH / rh) + 1;

            for (var i = startIdx; i < Math.min(startIdx + visCount, this._rows.length); i++) {
                var ry = hh + i * rh - this._scrollY;
                if (ry + rh < hh || ry > ch) continue;

                if (i === this._selectedRow) {
                    c.fillRect(1, ry, cw - 2, rh, L2_Theme.selection);
                } else if (i === this._hoverRow) {
                    c.fillRect(1, ry, cw - 2, rh, L2_Theme.highlight);
                } else if (i % 2 === 1) {
                    // Alternating stripe
                    c.fillRect(1, ry, cw - 2, rh, 'rgba(255,255,255,0.02)');
                }

                c.fillRect(2, ry + rh - 1, cw - 4, 1, 'rgba(255,255,255,0.03)');

                var colX = 0;
                var row = this._rows[i];
                this._columns.forEach(function (col) {
                    var colW = col.width || Math.floor(cw / self._columns.length);
                    c.fontSize = L2_Theme.fontNormal;
                    c.textColor = L2_Theme.textWhite;
                    var val = row[col.key] !== undefined ? String(row[col.key]) : '';
                    c.drawText(val, colX + 4, ry + 2, colW - 8, rh - 4, 'left');
                    colX += colW;
                });
            }
            this.markLayerClean('content');
        }
    };

    L2_Table.prototype.update = function () {
        L2_Base.prototype.update.call(this);
        if (!this.visible) return;

        var loc = this.toLocal(TouchInput.x, TouchInput.y);
        var cw = this.cw(), ch = this.ch();
        var hh = this._headerHeight;
        var rh = this._rowHeight;
        var inside = loc.x >= 0 && loc.x < cw && loc.y >= hh && loc.y < ch;

        var oldHover = this._hoverRow;
        this._hoverRow = inside ? Math.floor((loc.y - hh + this._scrollY) / rh) : -1;
        if (this._hoverRow >= this._rows.length) this._hoverRow = -1;
        if (this._hoverRow !== oldHover) this.markDirty('content');

        if (inside && TouchInput.isTriggered() && this._hoverRow >= 0) {
            this._selectedRow = this._hoverRow;
            if (this._onRowClick) this._onRowClick(this._rows[this._selectedRow], this._selectedRow);
            this.markDirty('content');
        }

        if (inside && TouchInput.wheelY) {
            var totalH = this._rows.length * rh;
            var bodyH = ch - hh;
            var oldScrollY = this._scrollY;
            this._scrollY += TouchInput.wheelY > 0 ? rh : -rh;
            this._scrollY = Math.max(0, Math.min(this._scrollY, Math.max(0, totalH - bodyH)));
            if (this._scrollY !== oldScrollY) {
                this.markDirty('content');
            }
        }
    };

    window.L2_Table = L2_Table;
})();
