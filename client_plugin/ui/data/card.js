/**
 * L2_Card - Content card with optional header and actions area.
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
        this.refresh();
    };

    L2_Card.prototype.standardPadding = function () { return 4; };

    L2_Card.prototype.setContent = function (title, body, footer) {
        this._title = title || this._title;
        this._body = body || this._body;
        this._footer = footer || this._footer;
        this.refresh();
    };

    L2_Card.prototype.refresh = function () {
        var c = this.bmp();
        c.clear();
        var cw = this.cw(), ch = this.ch();

        L2_Theme.drawPanelBg(c, 0, 0, cw, ch);
        if (this._hover) {
            L2_Theme.strokeRoundRect(c, 0, 0, cw, ch, L2_Theme.cornerRadius, L2_Theme.borderGold);
        }

        var yy = 8;
        if (this._title) {
            c.fontSize = L2_Theme.fontTitle;
            c.textColor = L2_Theme.textTitle;
            c.drawText(this._title, 10, yy, cw - 20, 20, 'left');
            yy += 24;
            c.fillRect(8, yy, cw - 16, 1, L2_Theme.borderDark);
            yy += 6;
        }

        if (this._body) {
            c.fontSize = L2_Theme.fontNormal;
            c.textColor = L2_Theme.textGray;
            c.drawText(this._body, 10, yy, cw - 20, ch - yy - 30, 'left');
        }

        if (this._footer) {
            c.fillRect(8, ch - 28, cw - 16, 1, L2_Theme.borderDark);
            c.fontSize = L2_Theme.fontSmall;
            c.textColor = L2_Theme.textDim;
            c.drawText(this._footer, 10, ch - 22, cw - 20, 18, 'left');
        }
    };

    L2_Card.prototype.update = function () {
        L2_Base.prototype.update.call(this);
        if (!this.visible) return;
        var wasHover = this._hover;
        this._hover = this.isInside(TouchInput.x, TouchInput.y);
        if (this._hover !== wasHover) this.refresh();
        if (this._hover && TouchInput.isTriggered() && this._onClick) this._onClick();
    };

    window.L2_Card = L2_Card;
})();
