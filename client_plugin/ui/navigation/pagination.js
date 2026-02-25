/**
 * L2_Pagination - Page navigation with numbered buttons.
 */
(function () {
    'use strict';

    function L2_Pagination() { this.initialize.apply(this, arguments); }
    L2_Pagination.prototype = Object.create(L2_Base.prototype);
    L2_Pagination.prototype.constructor = L2_Pagination;

    /**
     * @param {number} x
     * @param {number} y
     * @param {object} [opts] - { total, pageSize, totalPages, current, currentPage, onChange }
     */
    L2_Pagination.prototype.initialize = function (x, y, opts) {
        opts = opts || {};
        var w = opts.width || 220;
        L2_Base.prototype.initialize.call(this, x, y, w, 32 + 4);
        if (opts.total && opts.pageSize) {
            this._totalPages = Math.ceil(opts.total / opts.pageSize);
        } else {
            this._totalPages = opts.totalPages || 1;
        }
        this._currentPage = opts.current || opts.currentPage || 1;
        this._onChange = opts.onChange || null;
        this._hoverBtn = -1; // -2=prev, -3=next, 0..n=page buttons
        this.refresh();
    };

    L2_Pagination.prototype.standardPadding = function () { return 2; };

    L2_Pagination.prototype.setPage = function (p) {
        this._currentPage = Math.max(1, Math.min(p, this._totalPages));
        if (this._onChange) this._onChange(this._currentPage);
        this.markDirty();
    };

    L2_Pagination.prototype.setTotalPages = function (n) {
        this._totalPages = Math.max(1, n);
        if (this._currentPage > this._totalPages) this._currentPage = this._totalPages;
        this.markDirty();
    };

    L2_Pagination.prototype.refresh = function () {
        var c = this.bmp();
        c.clear();
        var cw = this.cw(), ch = this.ch();
        var btnW = 28, btnH = 24, gap = 4;
        var cx = (cw - (this._totalPages + 2) * (btnW + gap)) / 2;

        // Prev button
        this._drawBtn(c, cx, 4, btnW, btnH, '\u25C0', this._hoverBtn === -2, this._currentPage > 1);
        cx += btnW + gap;

        // Page numbers
        var maxShow = Math.min(this._totalPages, Math.floor((cw - 80) / (btnW + gap)));
        var startP = Math.max(1, this._currentPage - Math.floor(maxShow / 2));
        var endP = Math.min(this._totalPages, startP + maxShow - 1);
        startP = Math.max(1, endP - maxShow + 1);

        for (var p = startP; p <= endP; p++) {
            var active = p === this._currentPage;
            var hover = this._hoverBtn === p;
            this._drawBtn(c, cx, 4, btnW, btnH, String(p), hover, true, active);
            cx += btnW + gap;
        }

        // Next button
        this._drawBtn(c, cx, 4, btnW, btnH, '\u25B6', this._hoverBtn === -3, this._currentPage < this._totalPages);
    };

    L2_Pagination.prototype._drawBtn = function (c, x, y, w, h, label, hover, enabled, active) {
        var bg = active ? '#1A2A55' : (hover && enabled ? L2_Theme.bgButtonHover : L2_Theme.bgButton);
        var border = active ? L2_Theme.borderActive : (hover && enabled ? L2_Theme.borderGold : L2_Theme.borderDark);
        L2_Theme.fillRoundRect(c, x, y, w, h, 2, bg);
        L2_Theme.strokeRoundRect(c, x, y, w, h, 2, border);
        c.fontSize = L2_Theme.fontSmall;
        c.textColor = enabled ? (active ? L2_Theme.textGold : L2_Theme.textWhite) : L2_Theme.textDim;
        c.drawText(label, x, y + 2, w, h - 4, 'center');
    };

    L2_Pagination.prototype.update = function () {
        L2_Base.prototype.update.call(this);
        if (!this.visible) return;

        var loc = this.toLocal(TouchInput.x, TouchInput.y);
        var cw = this.cw();
        var btnW = 28, gap = 4;
        var maxShow = Math.min(this._totalPages, Math.floor((cw - 80) / (btnW + gap)));
        var startP = Math.max(1, this._currentPage - Math.floor(maxShow / 2));
        var endP = Math.min(this._totalPages, startP + maxShow - 1);
        startP = Math.max(1, endP - maxShow + 1);

        var cx = (cw - (endP - startP + 3) * (btnW + gap)) / 2;
        var oldHover = this._hoverBtn;
        this._hoverBtn = -1;

        // Prev
        if (loc.x >= cx && loc.x < cx + btnW && loc.y >= 4 && loc.y < 28) {
            this._hoverBtn = -2;
        }
        cx += btnW + gap;

        for (var p = startP; p <= endP; p++) {
            if (loc.x >= cx && loc.x < cx + btnW && loc.y >= 4 && loc.y < 28) {
                this._hoverBtn = p;
            }
            cx += btnW + gap;
        }

        // Next
        if (loc.x >= cx && loc.x < cx + btnW && loc.y >= 4 && loc.y < 28) {
            this._hoverBtn = -3;
        }

        if (this._hoverBtn !== oldHover) this.markDirty();

        if (TouchInput.isTriggered()) {
            if (this._hoverBtn === -2 && this._currentPage > 1) {
                this.setPage(this._currentPage - 1);
            } else if (this._hoverBtn === -3 && this._currentPage < this._totalPages) {
                this.setPage(this._currentPage + 1);
            } else if (this._hoverBtn > 0) {
                this.setPage(this._hoverBtn);
            }
        }
    };

    window.L2_Pagination = L2_Pagination;
})();
