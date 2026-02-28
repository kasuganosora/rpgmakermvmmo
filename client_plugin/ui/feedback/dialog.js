/**
 * L2_Dialog - Modal dialog with title, content, and action buttons.
 * Auto-centered with resize support.
 */
(function () {
    'use strict';

    function L2_Dialog() { this.initialize.apply(this, arguments); }
    L2_Dialog.prototype = Object.create(L2_Base.prototype);
    L2_Dialog.prototype.constructor = L2_Dialog;

    /**
     * @param {object} [opts] - { title, content, width, buttons: [{text, type, onClick}], closable, onClose }
     */
    L2_Dialog.prototype.initialize = function (opts) {
        opts = opts || {};
        this._title = opts.title || '';
        this._content = opts.content || '';
        this._closable = opts.closable !== false;
        this._onClose = opts.onClose || null;
        this._buttons = opts.buttons || [];
        this._closeHover = false;
        this._hoverBtn = -1;

        // Button layout constants
        this._btnWidth = 80;
        this._btnHeight = 30;
        this._btnGap = 12;

        var dw = opts.width || 360;
        this._contentLines = L2_Theme.wrapText(this._content, dw - 40, 7);
        var titleH = this._title ? 36 : 0;
        var contentH = Math.max(this._contentLines.length * 20 + 16, 40);
        var btnH = this._buttons.length > 0 ? 44 : 0;
        var dh = titleH + contentH + btnH + 8;

        // Initialize with centered position
        var gw = Graphics.boxWidth || 816;
        var gh = Graphics.boxHeight || 624;
        var dx = (gw - dw) / 2;
        var dy = (gh - dh) / 2;

        L2_Base.prototype.initialize.call(this, dx, dy, dw, dh);
        this._titleH = titleH;
        this._contentH = contentH;
        this._btnH = btnH;
        
        // 强制启用自动居中
        this._isCentered = true;
        this._centerOffsetX = 0;
        this._centerOffsetY = 0;
        
        // 预计算按钮布局
        this._calculateButtonLayout();
        
        this.refresh();
    };

    L2_Dialog.prototype.standardPadding = function () { return 0; };

    /** Calculate button layout positions */
    L2_Dialog.prototype._calculateButtonLayout = function () {
        if (this._buttons.length === 0) {
            this._btnStartX = 0;
            return;
        }
        var totalBtnW = this._buttons.length * this._btnWidth + (this._buttons.length - 1) * this._btnGap;
        this._btnStartX = (this.width - totalBtnW) / 2;
        this._btnY = this._titleH + this._contentH + 6;
    };

    /** Get button rect for hit testing */
    L2_Dialog.prototype._getButtonRect = function (index) {
        var bx = this._btnStartX + index * (this._btnWidth + this._btnGap);
        return {
            x: bx,
            y: this._btnY,
            w: this._btnWidth,
            h: this._btnHeight
        };
    };

    L2_Dialog.prototype._wrapText = function (text, maxW) {
        return L2_Theme.wrapText(text, maxW, 7);
    };

    L2_Dialog.prototype.refresh = function () {
        var c = this.bmp();
        c.clear();
        var w = this.width, h = this.height;

        // Dialog body
        L2_Theme.drawPanelBg(c, 0, 0, w, h);

        var yy = 0;
        // Title bar
        if (this._title) {
            L2_Theme.drawTitleBar(c, 0, 0, w, this._titleH, this._title);
            if (this._closable) {
                L2_Theme.drawCloseBtn(c, w - 28, 8, this._closeHover);
            }
            yy = this._titleH;
        }

        // Content
        c.fontSize = L2_Theme.fontNormal;
        c.textColor = L2_Theme.textWhite;
        for (var i = 0; i < this._contentLines.length; i++) {
            c.drawText(this._contentLines[i], 20, yy + 8 + i * 20, w - 40, 20, 'left');
        }
        yy += this._contentH;

        // Buttons
        if (this._buttons.length > 0) {
            for (var j = 0; j < this._buttons.length; j++) {
                var btn = this._buttons[j];
                var hover = this._hoverBtn === j;
                var color = L2_Theme.primaryColor;
                if (btn.type === 'danger') color = L2_Theme.dangerColor;
                else if (btn.type === 'default') color = L2_Theme.bgLight;

                var bg = hover ? L2_Theme.lighten(color, 0.15) : color;
                var rect = this._getButtonRect(j);
                L2_Theme.fillRoundRect(c, rect.x, rect.y, rect.w, rect.h, 4, bg);

                c.fontSize = L2_Theme.fontSmall;
                c.textColor = L2_Theme.textWhite;
                c.drawText(btn.text || '', rect.x, rect.y, rect.w, rect.h, 'center');
            }
        }
    };

    L2_Dialog.prototype.update = function () {
        L2_Base.prototype.update.call(this);
        if (!this.visible) return;

        var mx = TouchInput.x, my = TouchInput.y;
        var lx = mx - this.x, ly = my - this.y;
        var w = this.width;

        // Close button hover
        if (this._closable && this._title) {
            var wasCloseHover = this._closeHover;
            this._closeHover = lx >= w - 32 && lx <= w && ly >= 4 && ly <= 32;
            if (this._closeHover !== wasCloseHover) this.markDirty();
            if (this._closeHover && TouchInput.isTriggered()) {
                this.close();
                return;
            }
        }

        // Button hover and click (guard against null after close)
        if (this._buttons && this._buttons.length > 0) {
            var oldHover = this._hoverBtn;
            this._hoverBtn = -1;

            for (var j = 0; j < this._buttons.length; j++) {
                var rect = this._getButtonRect(j);
                if (lx >= rect.x && lx <= rect.x + rect.w && ly >= rect.y && ly <= rect.y + rect.h) {
                    this._hoverBtn = j;
                }
            }
            if (this._hoverBtn !== oldHover) this.markDirty();

            if (TouchInput.isTriggered() && this._hoverBtn >= 0) {
                var onClick = this._buttons[this._hoverBtn].onClick;
                if (onClick) onClick();
            }
        }

        // ESC to close
        if (this._closable && Input.isTriggered('cancel')) {
            this.close();
        }
    };

    L2_Dialog.prototype.close = function () {
        if (this._closed) return;
        this._closed = true;
        this.visible = false;
        if (this.parent && this.parent.removeChild) {
            this.parent.removeChild(this);
        }
        var onClose = this._onClose;
        // 清理引用
        this._onClose = null;
        this._buttons = null;
        if (onClose) onClose();
    };

    // Static convenience methods
    L2_Dialog.alert = function (title, content, onOk) {
        var d = new L2_Dialog({
            title: title,
            content: content,
            buttons: [{ text: '确定', type: 'primary', onClick: function () { d.close(); if (onOk) onOk(); } }]
        });
        if (SceneManager._scene) SceneManager._scene.addChild(d);
        return d;
    };

    L2_Dialog.confirm = function (title, content, onOk, onCancel) {
        var d = new L2_Dialog({
            title: title,
            content: content,
            buttons: [
                { text: '确定', type: 'primary', onClick: function () { d.close(); if (onOk) onOk(); } },
                { text: '取消', type: 'default', onClick: function () { d.close(); if (onCancel) onCancel(); } }
            ]
        });
        if (SceneManager._scene) SceneManager._scene.addChild(d);
        return d;
    };

    window.L2_Dialog = L2_Dialog;
})();
