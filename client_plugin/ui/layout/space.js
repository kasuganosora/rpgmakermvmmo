/**
 * L2_Space - Invisible spacer for layout purposes.
 */
(function () {
    'use strict';

    function L2_Space() { this.initialize.apply(this, arguments); }
    L2_Space.prototype = Object.create(L2_Base.prototype);
    L2_Space.prototype.constructor = L2_Space;

    L2_Space.prototype.initialize = function (w, h) {
        L2_Base.prototype.initialize.call(this, 0, 0, w || 8, h || 8);
    };

    L2_Space.prototype.standardPadding = function () { return 0; };
    L2_Space.prototype.refresh = function () { this.bmp().clear(); };

    window.L2_Space = L2_Space;
})();
