/**
 * L2_Link - Clickable text link with hover underline.
 */
(function () {
    'use strict';

    function L2_Link() { this.initialize.apply(this, arguments); }
    L2_Link.prototype = Object.create(L2_Base.prototype);
    L2_Link.prototype.constructor = L2_Link;

    /**
     * @param {number} x
     * @param {number} y
     * @param {string} text
     * @param {object} [opts] - { onClick, color, hoverColor }
     */
    L2_Link.prototype.initialize = function (x, y, text, opts) {
        opts = opts || {};
        var w = Math.max((text || '').length * 10 + 8, 40);
        L2_Base.prototype.initialize.call(this, x, y, w, 22);
        this._text = text || '';
        this._onClick = opts.onClick || null;
        this._color = opts.color || L2_Theme.textLink;
        this._hoverColor = opts.hoverColor || L2_Theme.textLinkHover;
        this._hover = false;
        this.refresh();
    };

    L2_Link.prototype.standardPadding = function () { return 2; };

    L2_Link.prototype.refresh = function () {
        var c = this.bmp();
        c.clear();
        var cw = this.cw(), ch = this.ch();
        c.fontSize = L2_Theme.fontNormal;
        c.textColor = this._hover ? this._hoverColor : this._color;
        c.drawText(this._text, 0, 0, cw, ch, 'left');
        if (this._hover) {
            var tw = L2_Theme.measureText(c, this._text, L2_Theme.fontNormal);
            L2_Theme.drawLine(c, 0, ch - 2, Math.min(tw, cw), ch - 2,
                this._hoverColor);
        }
    };

    L2_Link.prototype.update = function () {
        L2_Base.prototype.update.call(this);
        if (!this.visible) return;
        var wasHover = this._hover;
        this._hover = this.isInside(TouchInput.x, TouchInput.y);
        if (this._hover !== wasHover) this.markDirty();
        if (this._hover && TouchInput.isTriggered() && this._onClick) {
            this._onClick();
        }
    };

    window.L2_Link = L2_Link;
})();
