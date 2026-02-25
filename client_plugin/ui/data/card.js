/**
 * L2_Card - Content card with optional header and actions area.
 * Fixed content height calculation.
 */
(function () {
    'use strict';

    function L2_Card() { this.initialize.apply(this, arguments); }
    L2_Card.prototype = Object.create(L2_Base.prototype);
    L2_Card.prototype.constructor = L2_Card;

    L2_Card.prototype.initialize = function (x, y, w, h, opts) {
        L2_Base.prototype.initialize.call(this, x, y, w, h);
        opts = opts || {};
        this._title = opts.title || '';
        this._body = opts.body || '';
        this._footer = opts.footer || '';
        this._hover = false;
        this._onClick = opts.onClick || null;
        
        // 计算各部分高度
        this._titleHeight = this._title ? 24 : 0;
        this._footerHeight = this._footer ? 24 : 0;
        this._contentPadding = 8;
        
        this.refresh();
    };

    L2_Card.prototype.standardPadding = function () { return 4; };

    L2_Card.prototype.setContent = function (title, body, footer) {
        this._title = title || this._title;
        this._body = body || this._body;
        this._footer = footer || this._footer;
        
        // 重新计算高度
        this._titleHeight = this._title ? 24 : 0;
        this._footerHeight = this._footer ? 24 : 0;
        
        this.markDirty();
    };

    L2_Card.prototype.refresh = function () {
        var c = this.bmp();
        c.clear();
        var cw = this.cw(), ch = this.ch();

        L2_Theme.drawPanelBg(c, 0, 0, cw, ch);
        if (this._hover) {
            L2_Theme.strokeRoundRect(c, 0, 0, cw, ch, L2_Theme.cornerRadius, L2_Theme.borderGold);
        }

        var yy = this._contentPadding;
        
        // Title section
        if (this._title) {
            c.fontSize = L2_Theme.fontTitle;
            c.textColor = L2_Theme.textTitle;
            c.drawText(this._title, 10, yy, cw - 20, 20, 'left');
            yy += this._titleHeight;
            c.fillRect(8, yy, cw - 16, 1, L2_Theme.borderDark);
            yy += 6;
        }

        // Body section - 动态计算可用高度
        if (this._body) {
            var bodyHeight = ch - yy - this._contentPadding;
            if (this._footer) {
                bodyHeight -= (this._footerHeight + 6); // 6px 分隔线空间
            }
            
            c.fontSize = L2_Theme.fontNormal;
            c.textColor = L2_Theme.textGray;
            c.drawText(this._body, 10, yy, cw - 20, bodyHeight, 'left');
            yy += bodyHeight;
        }

        // Footer section
        if (this._footer) {
            yy += 6; // 分隔线前空间
            c.fillRect(8, yy, cw - 16, 1, L2_Theme.borderDark);
            yy += 6; // 分隔线后空间
            
            c.fontSize = L2_Theme.fontSmall;
            c.textColor = L2_Theme.textDim;
            c.drawText(this._footer, 10, yy, cw - 20, this._footerHeight - 6, 'left');
        }
    };

    L2_Card.prototype.update = function () {
        L2_Base.prototype.update.call(this);
        if (!this.visible) return;
        var wasHover = this._hover;
        this._hover = this.isInside(TouchInput.x, TouchInput.y);
        if (this._hover !== wasHover) this.markDirty();
        if (this._hover && TouchInput.isTriggered() && this._onClick) this._onClick();
    };

    window.L2_Card = L2_Card;
})();
