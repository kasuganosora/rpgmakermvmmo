/**
 * L2_SubWindow - Draggable, closable sub-window panel with title bar.
 */
(function () {
    'use strict';

    function L2_SubWindow() { this.initialize.apply(this, arguments); }
    L2_SubWindow.prototype = Object.create(L2_Base.prototype);
    L2_SubWindow.prototype.constructor = L2_SubWindow;

    /**
     * @param {number} x
     * @param {number} y
     * @param {number} w
     * @param {number} h
     * @param {object} [opts] - { title, closable, draggable, onClose }
     */
    L2_SubWindow.prototype.initialize = function (x, y, w, h, opts) {
        L2_Base.prototype.initialize.call(this, x, y, w, h);
        opts = opts || {};
        this._title = opts.title || '';
        this._closable = opts.closable !== false;
        this._draggable = opts.draggable !== false;
        this._onClose = opts.onClose || null;
        this._dragging = false;
        this._dragOffX = 0;
        this._dragOffY = 0;
        this._closeHover = false;
        this.refresh();
    };

    L2_SubWindow.prototype.setTitle = function (t) {
        this._title = t;
        this.refresh();
    };

    L2_SubWindow.prototype.onClose = function (fn) { this._onClose = fn; };

    /** Content Y start (below title bar). */
    L2_SubWindow.prototype.contentY = function () {
        return this._title ? L2_Theme.titleBarH + 4 : 4;
    };

    /** Content height (minus title bar and padding). */
    L2_SubWindow.prototype.contentH = function () {
        return this.ch() - this.contentY() - 4;
    };

    L2_SubWindow.prototype.refresh = function () {
        var c = this.bmp();
        c.clear();
        var cw = this.cw(), ch = this.ch();

        // Panel background
        L2_Theme.drawPanelBg(c, 0, 0, cw, ch);

        // Title bar
        if (this._title) {
            L2_Theme.drawTitleBar(c, 0, 0, cw, L2_Theme.titleBarH, this._title);
        }

        // Close button
        if (this._closable) {
            var btnSize = 18;
            L2_Theme.drawCloseBtn(c, cw - btnSize - 4, 4, btnSize, this._closeHover);
        }

        // Let subclasses draw content
        this.drawContent(c, cw, ch);
    };

    /** Override in subclasses for custom content. */
    L2_SubWindow.prototype.drawContent = function (/* bmp, cw, ch */) {};

    L2_SubWindow.prototype.update = function () {
        L2_Base.prototype.update.call(this);
        if (!this.visible) return;
        this._handleDrag();
        this._handleClose();
    };

    L2_SubWindow.prototype._handleDrag = function () {
        if (!this._draggable) return;
        if (TouchInput.isPressed()) {
            var tx = TouchInput.x, ty = TouchInput.y;
            if (!this._dragging && TouchInput.isTriggered()) {
                var barH = this._title ? L2_Theme.titleBarH : 0;
                if (barH > 0 && this.isInside(tx, ty) &&
                    ty <= this.y + this.padding + barH) {
                    this._dragging = true;
                    this._dragOffX = tx - this.x;
                    this._dragOffY = ty - this.y;
                }
            }
            if (this._dragging) {
                this.x = Math.max(0, Math.min(tx - this._dragOffX, Graphics.boxWidth - this.width));
                this.y = Math.max(0, Math.min(ty - this._dragOffY, Graphics.boxHeight - this.height));
            }
        } else {
            this._dragging = false;
        }
    };

    L2_SubWindow.prototype._handleClose = function () {
        if (!this._closable) return;
        var cw = this.cw();
        var btnSize = 18;
        var btnX = this.x + this.padding + cw - btnSize - 4;
        var btnY = this.y + this.padding + 4;
        var mx = TouchInput.x, my = TouchInput.y;
        var wasHover = this._closeHover;
        this._closeHover = mx >= btnX && mx <= btnX + btnSize &&
                           my >= btnY && my <= btnY + btnSize;
        if (this._closeHover !== wasHover) this.refresh();
        if (this._closeHover && TouchInput.isTriggered()) {
            this.visible = false;
            if (this._onClose) this._onClose();
        }
    };

    window.L2_SubWindow = L2_SubWindow;
})();
