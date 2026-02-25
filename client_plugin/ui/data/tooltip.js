/**
 * L2_Tooltip - Floating tooltip that shows near the mouse.
 */
(function () {
    'use strict';

    function L2_Tooltip() { this.initialize.apply(this, arguments); }
    L2_Tooltip.prototype = Object.create(L2_Base.prototype);
    L2_Tooltip.prototype.constructor = L2_Tooltip;

    L2_Tooltip.prototype.initialize = function () {
        L2_Base.prototype.initialize.call(this, 0, 0, 200, 60);
        this._tooltipText = '';
        this._lines = [];
        this._showTimer = 0;
        this._delay = 15; // frames before showing
        this.visible = false;
        this.refresh();
    };

    L2_Tooltip.prototype.standardPadding = function () { return 4; };

    /**
     * Show tooltip with the given text at mouse position.
     * @param {string} text
     */
    L2_Tooltip.prototype.show = function (text) {
        if (this._tooltipText === text && this.visible) return;
        this._tooltipText = text;
        this._lines = this._wrapText(text);
        this._showTimer = this._delay;
        this._resize();
        this.refresh();
    };

    L2_Tooltip.prototype.hide = function () {
        this.visible = false;
        this._tooltipText = '';
        this._showTimer = 0;
    };

    L2_Tooltip.prototype._wrapText = function (text) {
        return L2_Theme.wrapTextByChars(text, 30);
    };

    L2_Tooltip.prototype._resize = function () {
        var lineH = 18;
        var maxW = 40;
        for (var i = 0; i < this._lines.length; i++) {
            maxW = Math.max(maxW, this._lines[i].length * 7 + 16);
        }
        var newW = Math.min(maxW, 300);
        var newH = this._lines.length * lineH + 12;
        if (this.width !== newW || this.height !== newH) {
            this.width = newW;
            this.height = newH;
            this.createContents();
        }
    };

    L2_Tooltip.prototype.refresh = function () {
        var c = this.bmp();
        c.clear();
        var cw = this.cw(), ch = this.ch();

        // Background
        L2_Theme.fillRoundRect(c, 0, 0, cw, ch, 4, 'rgba(10,15,30,0.92)');
        L2_Theme.strokeRoundRect(c, 0, 0, cw, ch, 4, L2_Theme.borderLight);

        // Text
        c.fontSize = L2_Theme.fontSmall;
        c.textColor = L2_Theme.textWhite;
        var lineH = 18;
        for (var i = 0; i < this._lines.length; i++) {
            c.drawText(this._lines[i], 6, 4 + i * lineH, cw - 12, lineH, 'left');
        }
    };

    L2_Tooltip.prototype.update = function () {
        L2_Base.prototype.update.call(this);
        if (!this._tooltipText) return;
        if (this._showTimer > 0) {
            this._showTimer--;
            if (this._showTimer <= 0) {
                this.visible = true;
            }
        }
        if (this.visible) {
            // Follow mouse with offset
            var mx = TouchInput.x + 12;
            var my = TouchInput.y + 16;
            var gw = Graphics.boxWidth || 816;
            var gh = Graphics.boxHeight || 624;
            if (mx + this.width > gw) mx = TouchInput.x - this.width - 4;
            if (my + this.height > gh) my = TouchInput.y - this.height - 4;
            this.x = mx;
            this.y = my;
        }
    };

    window.L2_Tooltip = L2_Tooltip;
})();
