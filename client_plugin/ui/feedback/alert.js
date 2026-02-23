/**
 * L2_Alert - Static alert banner with type, message, and optional close.
 */
(function () {
    'use strict';

    function L2_Alert() { this.initialize.apply(this, arguments); }
    L2_Alert.prototype = Object.create(L2_Base.prototype);
    L2_Alert.prototype.constructor = L2_Alert;

    /**
     * @param {number} x
     * @param {number} y
     * @param {number} w
     * @param {object} [opts] - { title, message, type, closable, onClose }
     */
    L2_Alert.prototype.initialize = function (x, y, w, opts) {
        opts = opts || {};
        this._alertTitle = opts.title || '';
        this._alertMsg = opts.message || '';
        this._alertType = opts.type || 'info'; // 'info' | 'success' | 'warning' | 'error'
        this._closable = opts.closable || false;
        this._onClose = opts.onClose || null;
        this._closeHover = false;
        var h = this._alertTitle ? 52 : 32;
        L2_Base.prototype.initialize.call(this, x, y, w, h);
        this.refresh();
    };

    L2_Alert.prototype.standardPadding = function () { return 0; };

    L2_Alert.prototype._typeColors = function () {
        switch (this._alertType) {
            case 'success': return { bg: '#0D2818', border: L2_Theme.successColor, text: '#52C41A' };
            case 'warning': return { bg: '#2D2200', border: L2_Theme.warningColor, text: '#FAAD14' };
            case 'error': return { bg: '#2D0A0A', border: L2_Theme.dangerColor, text: '#FF4D4F' };
            default: return { bg: '#0A1628', border: L2_Theme.primaryColor, text: '#1890FF' };
        }
    };

    L2_Alert.prototype._typeIcon = function () {
        switch (this._alertType) {
            case 'success': return '✓';
            case 'warning': return '⚠';
            case 'error': return '✕';
            default: return 'ℹ';
        }
    };

    L2_Alert.prototype.refresh = function () {
        var c = this.bmp();
        c.clear();
        var w = this.width, h = this.height;
        var colors = this._typeColors();

        // Background
        L2_Theme.fillRoundRect(c, 0, 0, w, h, 4, colors.bg);
        L2_Theme.strokeRoundRect(c, 0, 0, w, h, 4, colors.border + '66');

        // Icon
        c.fontSize = 14;
        c.textColor = colors.text;
        c.drawText(this._typeIcon(), 10, 0, 18, this._alertTitle ? 28 : h, 'center');

        // Title + Message
        var tx = 32;
        if (this._alertTitle) {
            c.fontSize = L2_Theme.fontNormal;
            c.textColor = colors.text;
            c.drawText(this._alertTitle, tx, 4, w - tx - 30, 20, 'left');
            c.fontSize = L2_Theme.fontSmall;
            c.textColor = L2_Theme.textGray;
            c.drawText(this._alertMsg, tx, 26, w - tx - 30, 18, 'left');
        } else {
            c.fontSize = L2_Theme.fontSmall;
            c.textColor = L2_Theme.textWhite;
            c.drawText(this._alertMsg, tx, 0, w - tx - 30, h, 'left');
        }

        // Close button
        if (this._closable) {
            L2_Theme.drawCloseBtn(c, w - 22, (h - 14) / 2, this._closeHover);
        }
    };

    L2_Alert.prototype.update = function () {
        L2_Base.prototype.update.call(this);
        if (!this.visible || !this._closable) return;
        var mx = TouchInput.x, my = TouchInput.y;
        var lx = mx - this.x, ly = my - this.y;
        var wasHover = this._closeHover;
        this._closeHover = lx >= this.width - 26 && lx <= this.width - 4 && ly >= 2 && ly <= this.height - 2;
        if (this._closeHover !== wasHover) this.refresh();
        if (this._closeHover && TouchInput.isTriggered()) {
            this.visible = false;
            if (this.parent) this.parent.removeChild(this);
            if (this._onClose) this._onClose();
        }
    };

    window.L2_Alert = L2_Alert;
})();
