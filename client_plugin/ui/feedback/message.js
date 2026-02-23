/**
 * L2_Message - Temporary top-center message bar (like toast at top).
 */
(function () {
    'use strict';

    var _messageQueue = [];
    var _activeMessages = [];
    var _maxVisible = 5;

    function L2_Message() { this.initialize.apply(this, arguments); }
    L2_Message.prototype = Object.create(L2_Base.prototype);
    L2_Message.prototype.constructor = L2_Message;

    /**
     * @param {string} text
     * @param {object} [opts] - { type, duration }
     */
    L2_Message.prototype.initialize = function (text, opts) {
        opts = opts || {};
        this._msgText = text || '';
        this._msgType = opts.type || 'info'; // 'info' | 'success' | 'warning' | 'error'
        this._duration = opts.duration || 120; // frames
        this._timer = this._duration;
        this._fadeOut = false;

        var tw = Math.max(text.length * 8 + 40, 120);
        var gw = Graphics.boxWidth || 816;
        L2_Base.prototype.initialize.call(this, (gw - tw) / 2, -30, tw, 30);
        this._targetY = 10;
        this.refresh();
    };

    L2_Message.prototype.standardPadding = function () { return 0; };

    L2_Message.prototype._typeColor = function () {
        switch (this._msgType) {
            case 'success': return L2_Theme.successColor;
            case 'warning': return L2_Theme.warningColor;
            case 'error': return L2_Theme.dangerColor;
            default: return L2_Theme.primaryColor;
        }
    };

    L2_Message.prototype._typeIcon = function () {
        switch (this._msgType) {
            case 'success': return '✓';
            case 'warning': return '!';
            case 'error': return '✕';
            default: return 'i';
        }
    };

    L2_Message.prototype.refresh = function () {
        var c = this.bmp();
        c.clear();
        var w = this.width, h = this.height;

        // Background
        L2_Theme.fillRoundRect(c, 0, 0, w, h, 4, 'rgba(20,30,50,0.95)');
        // Left accent
        var accentColor = this._typeColor();
        c.fillRect(0, 0, 3, h, accentColor);

        // Icon
        c.fontSize = 13;
        c.textColor = accentColor;
        c.drawText(this._typeIcon(), 10, 0, 16, h, 'center');

        // Text
        c.fontSize = L2_Theme.fontSmall;
        c.textColor = L2_Theme.textWhite;
        c.drawText(this._msgText, 30, 0, w - 40, h, 'left');
    };

    L2_Message.prototype.update = function () {
        L2_Base.prototype.update.call(this);
        if (!this.visible) return;

        // Slide in
        if (this.y < this._targetY) {
            this.y = Math.min(this.y + 4, this._targetY);
        }

        // Timer
        this._timer--;
        if (this._timer <= 0 && !this._fadeOut) {
            this._fadeOut = true;
        }

        // Fade out
        if (this._fadeOut) {
            this.opacity -= 8;
            if (this.opacity <= 0) {
                this.visible = false;
                if (this.parent) this.parent.removeChild(this);
                var idx = _activeMessages.indexOf(this);
                if (idx >= 0) _activeMessages.splice(idx, 1);
                // Reposition remaining
                _repositionMessages();
            }
        }
    };

    function _repositionMessages() {
        var yy = 10;
        for (var i = 0; i < _activeMessages.length; i++) {
            _activeMessages[i]._targetY = yy;
            yy += _activeMessages[i].height + 6;
        }
    }

    // Static API
    L2_Message.show = function (text, type, duration) {
        var msg = new L2_Message(text, { type: type, duration: duration });
        _activeMessages.push(msg);
        _repositionMessages();
        if (SceneManager._scene) SceneManager._scene.addChild(msg);
        msg.visible = true;
        msg.opacity = 255;
        return msg;
    };

    L2_Message.info = function (text, duration) { return L2_Message.show(text, 'info', duration); };
    L2_Message.success = function (text, duration) { return L2_Message.show(text, 'success', duration); };
    L2_Message.warning = function (text, duration) { return L2_Message.show(text, 'warning', duration); };
    L2_Message.error = function (text, duration) { return L2_Message.show(text, 'error', duration); };

    window.L2_Message = L2_Message;
})();
