/**
 * L2_FullscreenWindow - Full-screen overlay window with dark background.
 */
(function () {
    'use strict';

    function L2_FullscreenWindow() { this.initialize.apply(this, arguments); }
    L2_FullscreenWindow.prototype = Object.create(L2_Base.prototype);
    L2_FullscreenWindow.prototype.constructor = L2_FullscreenWindow;

    /**
     * @param {object} [opts] - { title, closable, onClose, bgOpacity }
     */
    L2_FullscreenWindow.prototype.initialize = function (opts) {
        opts = opts || {};
        L2_Base.prototype.initialize.call(this, 0, 0, Graphics.boxWidth, Graphics.boxHeight);
        this._title = opts.title || '';
        this._closable = opts.closable !== false;
        this._onClose = opts.onClose || null;
        this._bgOpacity = opts.bgOpacity !== undefined ? opts.bgOpacity : 0.7;
        this._closeHover = false;
        this.refresh();
    };

    L2_FullscreenWindow.prototype.standardPadding = function () { return 0; };

    L2_FullscreenWindow.prototype.contentY = function () {
        return this._title ? L2_Theme.titleBarH + 8 : 8;
    };

    L2_FullscreenWindow.prototype.contentH = function () {
        return this.ch() - this.contentY() - 8;
    };

    L2_FullscreenWindow.prototype.refresh = function () {
        var c = this.bmp();
        c.clear();
        var cw = this.cw(), ch = this.ch();

        // Dark overlay background
        c.fillRect(0, 0, cw, ch, 'rgba(0,0,0,' + this._bgOpacity + ')');

        // Title bar
        if (this._title) {
            c.gradientFillRect(0, 0, cw, L2_Theme.titleBarH, L2_Theme.bgHeader, L2_Theme.bgHeaderEnd, false);
            c.fillRect(0, L2_Theme.titleBarH - 1, cw, 1, L2_Theme.borderDark);
            c.fontSize = L2_Theme.fontH2;
            c.textColor = L2_Theme.textTitle;
            c.drawText(this._title, 16, 3, cw - 48, L2_Theme.titleBarH - 4, 'left');
        }

        // Close button
        if (this._closable) {
            var btnSize = 20;
            L2_Theme.drawCloseBtn(c, cw - btnSize - 8, 3, btnSize, this._closeHover);
        }

        this.drawContent(c, cw, ch);
    };

    L2_FullscreenWindow.prototype.drawContent = function (/* bmp, cw, ch */) {};

    L2_FullscreenWindow.prototype.update = function () {
        L2_Base.prototype.update.call(this);
        if (!this.visible) return;
        if (this._closable) {
            var cw = this.cw();
            var btnSize = 20;
            var btnX = cw - btnSize - 8;
            var btnY = 3;
            var mx = TouchInput.x, my = TouchInput.y;
            var wasHover = this._closeHover;
            this._closeHover = mx >= btnX && mx <= btnX + btnSize &&
                               my >= btnY && my <= btnY + btnSize;
            if (this._closeHover !== wasHover) this.refresh();
            if (this._closeHover && TouchInput.isTriggered()) {
                this.visible = false;
                if (this._onClose) this._onClose();
            }
        }
        // ESC to close
        if (Input.isTriggered('escape') || Input.isTriggered('cancel')) {
            this.visible = false;
            if (this._onClose) this._onClose();
        }
    };

    L2_FullscreenWindow.prototype.onClose = function (fn) { this._onClose = fn; };

    window.L2_FullscreenWindow = L2_FullscreenWindow;
})();
