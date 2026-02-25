/**
 * L2_Drawer - Slide-in panel from edge of screen.
 */
(function () {
    'use strict';

    function L2_Drawer() { this.initialize.apply(this, arguments); }
    L2_Drawer.prototype = Object.create(L2_Base.prototype);
    L2_Drawer.prototype.constructor = L2_Drawer;

    /**
     * @param {object} [opts] - { title, width, placement, closable, onClose }
     */
    L2_Drawer.prototype.initialize = function (opts) {
        opts = opts || {};
        this._title = opts.title || '';
        this._drawerWidth = opts.width || 280;
        this._placement = opts.placement || 'right'; // 'left' | 'right'
        this._closable = opts.closable !== false;
        this._onClose = opts.onClose || null;
        this._closeHover = false;
        this._slideProgress = 0; // 0 to 1
        this._opening = false;
        this._closing = false;
        this._titleH = this._title ? 36 : 0;
        this._children = [];

        var gw = Graphics.boxWidth || 816;
        var gh = Graphics.boxHeight || 624;
        var dx = this._placement === 'right' ? gw : -this._drawerWidth;
        L2_Base.prototype.initialize.call(this, dx, 0, this._drawerWidth, gh);
        this._targetX = this._placement === 'right' ? gw - this._drawerWidth : 0;
        this._hiddenX = this._placement === 'right' ? gw : -this._drawerWidth;
        this.refresh();
    };

    L2_Drawer.prototype.standardPadding = function () { return 0; };

    L2_Drawer.prototype.open = function () {
        this._opening = true;
        this._closing = false;
        this.visible = true;
    };

    L2_Drawer.prototype.close = function () {
        this._closing = true;
        this._opening = false;
    };

    L2_Drawer.prototype.refresh = function () {
        var c = this.bmp();
        c.clear();
        var w = this.width, h = this.height;

        // Background
        c.fillRect(0, 0, w, h, L2_Theme.bgPanel);
        // Border on the open side
        if (this._placement === 'right') {
            c.fillRect(0, 0, 1, h, L2_Theme.borderLight);
        } else {
            c.fillRect(w - 1, 0, 1, h, L2_Theme.borderLight);
        }

        // Title bar
        if (this._title) {
            L2_Theme.drawTitleBar(c, 0, 0, w, this._titleH, this._title);
            if (this._closable) {
                L2_Theme.drawCloseBtn(c, w - 28, 8, this._closeHover);
            }
        }
    };

    /** Returns the Y offset for content added to the drawer. */
    L2_Drawer.prototype.contentY = function () {
        return this._titleH + 8;
    };

    L2_Drawer.prototype.addContent = function (child) {
        if (!child) return;
        this._children.push(child);
        this.addChild(child);
        // 调整子元素位置以适应抽屉内容区域
        if (child.y < this.contentY()) {
            child.y = this.contentY();
        }
    };

    L2_Drawer.prototype.removeContent = function (child) {
        if (!child) return;
        var idx = this._children.indexOf(child);
        if (idx >= 0) {
            this._children.splice(idx, 1);
            if (child.parent === this) {
                this.removeChild(child);
            }
        }
    };

    L2_Drawer.prototype.update = function () {
        L2_Base.prototype.update.call(this);
        if (!this.visible) return;

        // Slide animation
        var speed = 0.12;
        if (this._opening) {
            this._slideProgress = Math.min(this._slideProgress + speed, 1);
            this.x = this._hiddenX + (this._targetX - this._hiddenX) * this._easeOut(this._slideProgress);
            if (this._slideProgress >= 1) this._opening = false;
        } else if (this._closing) {
            this._slideProgress = Math.max(this._slideProgress - speed, 0);
            this.x = this._hiddenX + (this._targetX - this._hiddenX) * this._easeOut(this._slideProgress);
            if (this._slideProgress <= 0) {
                this._closing = false;
                this.visible = false;
                if (this._onClose) this._onClose();
            }
        }

        // Close button
        var mx = TouchInput.x, my = TouchInput.y;
        var lx = mx - this.x, ly = my - this.y;
        if (this._closable && this._title) {
            var wasHover = this._closeHover;
            this._closeHover = lx >= this.width - 32 && lx <= this.width && ly >= 4 && ly <= 32;
            if (this._closeHover !== wasHover) this.markDirty();
            if (this._closeHover && TouchInput.isTriggered()) {
                this.close();
                return;
            }
        }

        // ESC to close
        if (this._closable && Input.isTriggered('cancel')) {
            this.close();
        }
    };

    L2_Drawer.prototype._easeOut = function (t) {
        return 1 - Math.pow(1 - t, 3);
    };

    window.L2_Drawer = L2_Drawer;
})();
