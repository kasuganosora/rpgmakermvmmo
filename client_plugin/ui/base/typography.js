/**
 * L2_Typography - Text display with heading levels (H1-H3), paragraph, caption.
 */
(function () {
    'use strict';

    function L2_Typography() { this.initialize.apply(this, arguments); }
    L2_Typography.prototype = Object.create(L2_Base.prototype);
    L2_Typography.prototype.constructor = L2_Typography;

    /**
     * @param {number} x
     * @param {number} y
     * @param {number} w
     * @param {object} [opts] - { text, level:'h1'|'h2'|'h3'|'body'|'caption', color, align }
     */
    L2_Typography.prototype.initialize = function (x, y, w, opts) {
        opts = opts || {};
        this._level = opts.level || 'body';
        var h = L2_Typography._heightForLevel(this._level);
        L2_Base.prototype.initialize.call(this, x, y, w, h);
        this._text = opts.text || '';
        this._textColor = opts.color || null;
        this._align = opts.align || 'left';
        this.refresh();
    };

    L2_Typography.prototype.standardPadding = function () { return 2; };

    L2_Typography._heightForLevel = function (level) {
        switch (level) {
            case 'h1': return L2_Theme.fontH1 + 12;
            case 'h2': return L2_Theme.fontH2 + 10;
            case 'h3': return L2_Theme.fontH3 + 8;
            case 'caption': return L2_Theme.fontTiny + 8;
            default: return L2_Theme.fontNormal + 8;
        }
    };

    L2_Typography.prototype.setText = function (t) {
        this._text = t;
        this.refresh();
    };

    L2_Typography.prototype.refresh = function () {
        var c = this.bmp();
        c.clear();
        var cw = this.cw(), ch = this.ch();

        switch (this._level) {
            case 'h1':
                c.fontSize = L2_Theme.fontH1;
                c.textColor = this._textColor || L2_Theme.textWhite;
                break;
            case 'h2':
                c.fontSize = L2_Theme.fontH2;
                c.textColor = this._textColor || L2_Theme.textWhite;
                break;
            case 'h3':
                c.fontSize = L2_Theme.fontH3;
                c.textColor = this._textColor || L2_Theme.textTitle;
                break;
            case 'caption':
                c.fontSize = L2_Theme.fontTiny;
                c.textColor = this._textColor || L2_Theme.textDim;
                break;
            default:
                c.fontSize = L2_Theme.fontNormal;
                c.textColor = this._textColor || L2_Theme.textGray;
        }

        c.drawText(this._text, 0, 0, cw, ch, this._align);
    };

    window.L2_Typography = L2_Typography;
})();
