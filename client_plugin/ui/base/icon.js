/**
 * L2_Icon - Renders an icon from the RMMV IconSet.
 */
(function () {
    'use strict';

    function L2_Icon() { this.initialize.apply(this, arguments); }
    L2_Icon.prototype = Object.create(L2_Base.prototype);
    L2_Icon.prototype.constructor = L2_Icon;

    /**
     * @param {number} x
     * @param {number} y
     * @param {number} iconIndex
     * @param {object} [opts] - { size, onClick }
     */
    L2_Icon.prototype.initialize = function (x, y, iconIndex, opts) {
        opts = opts || {};
        var size = opts.size || L2_Theme.iconSize;
        L2_Base.prototype.initialize.call(this, x, y, size + 4, size + 4);
        this._iconIndex = iconIndex || 0;
        this._iconSize = size;
        this._onClick = opts.onClick || null;
        this.refresh();
    };

    L2_Icon.prototype.standardPadding = function () { return 2; };

    L2_Icon.prototype.setIcon = function (index) {
        this._iconIndex = index;
        this.markDirty();
    };

    L2_Icon.prototype.refresh = function () {
        var c = this.bmp();
        c.clear();
        if (this._iconIndex < 0) return;

        if (ImageManager && ImageManager.loadSystem) {
            var iconSet = ImageManager.loadSystem('IconSet');
            if (iconSet && iconSet.isReady()) {
                var pw = Window_Base._iconWidth || 32;
                var ph = Window_Base._iconHeight || 32;
                var sx = (this._iconIndex % 16) * pw;
                var sy = Math.floor(this._iconIndex / 16) * ph;
                c.blt(iconSet, sx, sy, pw, ph, 0, 0, this._iconSize, this._iconSize);
            }
        }
    };

    L2_Icon.prototype.update = function () {
        L2_Base.prototype.update.call(this);
        if (!this.visible || !this._onClick) return;
        if (TouchInput.isTriggered() && this.isInside(TouchInput.x, TouchInput.y)) {
            this._onClick(this._iconIndex);
        }
    };

    window.L2_Icon = L2_Icon;
})();
