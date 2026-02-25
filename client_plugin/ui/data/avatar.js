/**
 * L2_Avatar - Circular or square avatar with optional image/text/icon.
 */
(function () {
    'use strict';

    function L2_Avatar() { this.initialize.apply(this, arguments); }
    L2_Avatar.prototype = Object.create(L2_Base.prototype);
    L2_Avatar.prototype.constructor = L2_Avatar;

    /**
     * @param {number} x
     * @param {number} y
     * @param {object} [opts] - { size, shape, text, iconIndex, faceName, faceIndex, bgColor }
     */
    L2_Avatar.prototype.initialize = function (x, y, opts) {
        opts = opts || {};
        this._size = opts.size || L2_Theme.iconSize;
        this._shape = opts.shape || 'circle'; // 'circle' | 'square'
        this._avatarText = opts.text || '';
        this._iconIndex = opts.iconIndex != null ? opts.iconIndex : -1;
        this._faceName = opts.faceName || '';
        this._faceIndex = opts.faceIndex || 0;
        this._avatarBg = opts.bgColor || L2_Theme.primaryColor;
        L2_Base.prototype.initialize.call(this, x, y, this._size + 4, this._size + 4);
        if (this._faceName) {
            this._faceBitmap = ImageManager.loadFace(this._faceName);
            this._faceBitmap.addLoadListener(this.refresh.bind(this));
        }
        this.refresh();
    };

    L2_Avatar.prototype.standardPadding = function () { return 2; };

    L2_Avatar.prototype.refresh = function () {
        var c = this.bmp();
        c.clear();
        var s = this._size;
        var ctx = c._context;
        if (!ctx) return;

        ctx.save();
        if (this._shape === 'circle') {
            ctx.beginPath();
            ctx.arc(s / 2, s / 2, s / 2, 0, Math.PI * 2);
            ctx.closePath();
            ctx.clip();
        }

        // Background
        ctx.fillStyle = this._avatarBg;
        ctx.fillRect(0, 0, s, s);

        if (this._faceName && this._faceBitmap && this._faceBitmap.isReady()) {
            // Draw face from RMMV face sheet (144x144 per face, 4 cols)
            var fw = 144, fh = 144;
            var fx = (this._faceIndex % 4) * fw;
            var fy = Math.floor(this._faceIndex / 4) * fh;
            c.blt(this._faceBitmap, fx, fy, fw, fh, 0, 0, s, s);
        } else if (this._iconIndex >= 0) {
            // Draw icon centered
            var iconBitmap = ImageManager.loadSystem('IconSet');
            var pw = 32, ph = 32;
            var sx = (this._iconIndex % 16) * pw;
            var sy = Math.floor(this._iconIndex / 16) * ph;
            var dx = (s - pw) / 2, dy = (s - ph) / 2;
            c.blt(iconBitmap, sx, sy, pw, ph, dx, dy);
        } else if (this._avatarText) {
            // Draw text centered
            c.fontSize = Math.floor(s * 0.5);
            c.textColor = L2_Theme.textWhite;
            c.drawText(this._avatarText.charAt(0).toUpperCase(), 0, 0, s, s, 'center');
        }

        ctx.restore();
        c._setDirty();
    };

    window.L2_Avatar = L2_Avatar;
})();
