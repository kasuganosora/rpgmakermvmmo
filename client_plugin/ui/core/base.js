/**
 * L2_Base - Base class for all L2 UI components.
 * Extends Window_Base with L2 theming and common utilities.
 */
(function () {
    'use strict';

    function L2_Base() { this.initialize.apply(this, arguments); }
    L2_Base.prototype = Object.create(Window_Base.prototype);
    L2_Base.prototype.constructor = L2_Base;

    L2_Base.prototype.initialize = function (x, y, w, h) {
        Window_Base.prototype.initialize.call(this, x, y, w, h);
        this.opacity = 0;       // hide RMMV windowskin
        this.backOpacity = 0;
    };

    L2_Base.prototype.standardPadding = function () { return L2_Theme.padding; };

    /** Hit test: is (mx, my) inside this component's bounds? */
    L2_Base.prototype.isInside = function (mx, my) {
        return mx >= this.x && mx <= this.x + this.width &&
               my >= this.y && my <= this.y + this.height;
    };

    /** Convert screen coords to local content coords. */
    L2_Base.prototype.toLocal = function (mx, my) {
        return { x: mx - this.x - this.padding, y: my - this.y - this.padding };
    };

    /** Shorthand for contents bitmap. */
    L2_Base.prototype.bmp = function () { return this.contents; };

    /** Shorthand for contents width. */
    L2_Base.prototype.cw = function () { return this.contentsWidth(); };

    /** Shorthand for contents height. */
    L2_Base.prototype.ch = function () { return this.contentsHeight(); };

    window.L2_Base = L2_Base;
})();
