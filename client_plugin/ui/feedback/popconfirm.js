/**
 * L2_Popconfirm - Small confirmation popup attached to an anchor position.
 */
(function () {
    'use strict';

    function L2_Popconfirm() { this.initialize.apply(this, arguments); }
    L2_Popconfirm.prototype = Object.create(L2_Base.prototype);
    L2_Popconfirm.prototype.constructor = L2_Popconfirm;

    /**
     * @param {number} anchorX - X position to anchor near
     * @param {number} anchorY - Y position to anchor near
     * @param {object} [opts] - { text, okText, cancelText, onOk, onCancel, placement }
     */
    L2_Popconfirm.prototype.initialize = function (anchorX, anchorY, opts) {
        opts = opts || {};
        this._pcText = opts.text || '确定执行此操作？';
        this._okText = opts.okText || '确定';
        this._cancelText = opts.cancelText || '取消';
        this._onOk = opts.onOk || null;
        this._onCancel = opts.onCancel || null;
        this._placement = opts.placement || 'top'; // 'top' | 'bottom'
        this._hoverBtn = -1; // 0=ok, 1=cancel

        var pw = Math.max(this._pcText.length * 8 + 24, 160);
        var ph = 64;

        var px, py;
        if (this._placement === 'bottom') {
            px = anchorX - pw / 2;
            py = anchorY + 8;
        } else {
            px = anchorX - pw / 2;
            py = anchorY - ph - 8;
        }

        // Clamp to screen
        var gw = Graphics.boxWidth || 816;
        var gh = Graphics.boxHeight || 624;
        px = Math.max(4, Math.min(px, gw - pw - 4));
        py = Math.max(4, Math.min(py, gh - ph - 4));

        L2_Base.prototype.initialize.call(this, px, py, pw, ph);
        this.refresh();
    };

    L2_Popconfirm.prototype.standardPadding = function () { return 0; };

    L2_Popconfirm.prototype.refresh = function () {
        var c = this.bmp();
        c.clear();
        var w = this.width, h = this.height;

        // Background
        L2_Theme.fillRoundRect(c, 0, 0, w, h, 6, 'rgba(15,22,40,0.96)');
        L2_Theme.strokeRoundRect(c, 0, 0, w, h, 6, L2_Theme.borderLight);

        // Warning icon + text
        c.fontSize = L2_Theme.fontSmall;
        c.textColor = L2_Theme.warningColor;
        c.drawText('⚠', 8, 6, 16, 20, 'center');
        c.textColor = L2_Theme.textWhite;
        c.drawText(this._pcText, 28, 6, w - 36, 20, 'left');

        // Buttons
        var btnW = 50, btnH = 22, gap = 8;
        var bx = w - btnW * 2 - gap - 10;
        var by = h - btnH - 10;

        // Cancel button
        var cancelBg = this._hoverBtn === 1 ? L2_Theme.bgLight : L2_Theme.borderDark;
        L2_Theme.fillRoundRect(c, bx, by, btnW, btnH, 3, cancelBg);
        c.fontSize = 12;
        c.textColor = L2_Theme.textWhite;
        c.drawText(this._cancelText, bx, by, btnW, btnH, 'center');

        // OK button
        bx += btnW + gap;
        var okBg = this._hoverBtn === 0 ? L2_Theme.lighten(L2_Theme.primaryColor, 0.15) : L2_Theme.primaryColor;
        L2_Theme.fillRoundRect(c, bx, by, btnW, btnH, 3, okBg);
        c.textColor = L2_Theme.textWhite;
        c.drawText(this._okText, bx, by, btnW, btnH, 'center');
    };

    L2_Popconfirm.prototype.update = function () {
        L2_Base.prototype.update.call(this);
        if (!this.visible) return;

        var mx = TouchInput.x, my = TouchInput.y;
        var lx = mx - this.x, ly = my - this.y;
        var w = this.width, h = this.height;
        var btnW = 50, btnH = 22, gap = 8;
        var by = h - btnH - 10;
        var bx0 = w - btnW * 2 - gap - 10;

        var oldHover = this._hoverBtn;
        this._hoverBtn = -1;

        // OK button area
        var okX = bx0 + btnW + gap;
        if (lx >= okX && lx <= okX + btnW && ly >= by && ly <= by + btnH) {
            this._hoverBtn = 0;
        }
        // Cancel button area
        if (lx >= bx0 && lx <= bx0 + btnW && ly >= by && ly <= by + btnH) {
            this._hoverBtn = 1;
        }

        if (this._hoverBtn !== oldHover) this.refresh();

        if (TouchInput.isTriggered()) {
            if (this._hoverBtn === 0) {
                this._dismiss();
                if (this._onOk) this._onOk();
            } else if (this._hoverBtn === 1) {
                this._dismiss();
                if (this._onCancel) this._onCancel();
            } else if (lx < 0 || lx > w || ly < 0 || ly > h) {
                // Click outside dismisses
                this._dismiss();
                if (this._onCancel) this._onCancel();
            }
        }

        // ESC to cancel
        if (Input.isTriggered('cancel')) {
            this._dismiss();
            if (this._onCancel) this._onCancel();
        }
    };

    L2_Popconfirm.prototype._dismiss = function () {
        this.visible = false;
        if (this.parent) this.parent.removeChild(this);
    };

    window.L2_Popconfirm = L2_Popconfirm;
})();
