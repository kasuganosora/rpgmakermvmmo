/**
 * L2_Tag - Small label tag/badge.
 */
(function () {
    'use strict';

    function L2_Tag() { this.initialize.apply(this, arguments); }
    L2_Tag.prototype = Object.create(L2_Base.prototype);
    L2_Tag.prototype.constructor = L2_Tag;

    /**
     * @param {number} x
     * @param {number} y
     * @param {string} text
     * @param {object} [opts] - { color, bgColor, closable, onClose }
     */
    L2_Tag.prototype.initialize = function (x, y, text, opts) {
        opts = opts || {};
        var w = Math.max(text.length * 8 + 20 + (opts.closable ? 18 : 0), 40);
        L2_Base.prototype.initialize.call(this, x, y, w, 22 + 4);
        this._text = text;
        this._textColor = opts.color || L2_Theme.textWhite;
        this._bgColor = opts.bgColor || '#1A2A44';
        this._closable = opts.closable || false;
        this._onClose = opts.onClose || null;
        this._closeHover = false;
        this.refresh();
    };

    L2_Tag.prototype.standardPadding = function () { return 2; };

    L2_Tag.prototype.refresh = function () {
        var c = this.bmp();
        c.clear();
        var cw = this.cw(), ch = this.ch();

        L2_Theme.fillRoundRect(c, 0, 0, cw, ch, ch / 2, this._bgColor);
        c.fontSize = L2_Theme.fontSmall;
        c.textColor = this._textColor;
        c.drawText(this._text, 8, 0, cw - 16 - (this._closable ? 14 : 0), ch, 'left');

        if (this._closable) {
            var bx = cw - 16, by = (ch - 10) / 2;
            var color = this._closeHover ? L2_Theme.textRed : L2_Theme.textGray;
            var ctx = c._context;
            if (ctx) {
                ctx.save();
                ctx.strokeStyle = color;
                ctx.lineWidth = 1.5;
                ctx.beginPath();
                ctx.moveTo(bx, by);
                ctx.lineTo(bx + 10, by + 10);
                ctx.moveTo(bx + 10, by);
                ctx.lineTo(bx, by + 10);
                ctx.stroke();
                ctx.restore();
                c._setDirty();
            }
        }
    };

    L2_Tag.prototype.update = function () {
        L2_Base.prototype.update.call(this);
        if (!this.visible || !this._closable) return;
        var cw = this.cw();
        var bx = this.x + this.padding + cw - 16;
        var by = this.y + this.padding;
        var mx = TouchInput.x, my = TouchInput.y;
        var wasHover = this._closeHover;
        this._closeHover = mx >= bx && mx <= bx + 14 && my >= by && my <= by + this.ch();
        if (this._closeHover !== wasHover) this.markDirty();
        if (this._closeHover && TouchInput.isTriggered() && this._onClose) this._onClose();
    };

    window.L2_Tag = L2_Tag;
})();
