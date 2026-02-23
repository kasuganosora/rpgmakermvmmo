/**
 * L2_Notification - Corner notification popup with title and content.
 */
(function () {
    'use strict';

    var _activeNotifs = [];

    function L2_Notification() { this.initialize.apply(this, arguments); }
    L2_Notification.prototype = Object.create(L2_Base.prototype);
    L2_Notification.prototype.constructor = L2_Notification;

    /**
     * @param {object} [opts] - { title, content, type, duration, placement, closable, onClose }
     */
    L2_Notification.prototype.initialize = function (opts) {
        opts = opts || {};
        this._nTitle = opts.title || '';
        this._nContent = opts.content || '';
        this._nType = opts.type || 'info';
        this._duration = opts.duration || 180; // frames (~3s)
        this._placement = opts.placement || 'topRight';
        this._closable = opts.closable !== false;
        this._onClose = opts.onClose || null;
        this._timer = this._duration;
        this._fadeOut = false;
        this._closeHover = false;

        var nw = 280;
        var contentLines = this._wrapText(this._nContent, nw - 30);
        this._contentLines = contentLines;
        var nh = 24 + contentLines.length * 18 + 16;

        var gw = Graphics.boxWidth || 816;
        var startX = this._placement.indexOf('Right') >= 0 ? gw : -nw;
        L2_Base.prototype.initialize.call(this, startX, 0, nw, nh);
        this._targetX = this._placement.indexOf('Right') >= 0 ? gw - nw - 12 : 12;
        this.refresh();
    };

    L2_Notification.prototype.standardPadding = function () { return 0; };

    L2_Notification.prototype._wrapText = function (text, maxW) {
        if (!text) return [];
        var charW = 7;
        var cpl = Math.max(Math.floor(maxW / charW), 1);
        var result = [];
        var paragraphs = text.split('\n');
        for (var i = 0; i < paragraphs.length; i++) {
            var line = paragraphs[i];
            while (line.length > cpl) {
                result.push(line.substring(0, cpl));
                line = line.substring(cpl);
            }
            result.push(line);
        }
        return result;
    };

    L2_Notification.prototype._typeColor = function () {
        switch (this._nType) {
            case 'success': return L2_Theme.successColor;
            case 'warning': return L2_Theme.warningColor;
            case 'error': return L2_Theme.dangerColor;
            default: return L2_Theme.primaryColor;
        }
    };

    L2_Notification.prototype.refresh = function () {
        var c = this.bmp();
        c.clear();
        var w = this.width, h = this.height;

        // Background
        L2_Theme.fillRoundRect(c, 0, 0, w, h, 6, 'rgba(15,22,40,0.95)');
        L2_Theme.strokeRoundRect(c, 0, 0, w, h, 6, L2_Theme.borderLight);

        // Left accent
        c.fillRect(0, 6, 3, h - 12, this._typeColor());

        // Title
        c.fontSize = L2_Theme.fontNormal;
        c.textColor = L2_Theme.textWhite;
        c.drawText(this._nTitle, 12, 8, w - 40, 20, 'left');

        // Close button
        if (this._closable) {
            L2_Theme.drawCloseBtn(c, w - 22, 8, this._closeHover);
        }

        // Content
        c.fontSize = L2_Theme.fontSmall;
        c.textColor = L2_Theme.textGray;
        for (var i = 0; i < this._contentLines.length; i++) {
            c.drawText(this._contentLines[i], 12, 30 + i * 18, w - 24, 18, 'left');
        }
    };

    L2_Notification.prototype.update = function () {
        L2_Base.prototype.update.call(this);
        if (!this.visible) return;

        // Slide in
        var speed = 6;
        if (this._placement.indexOf('Right') >= 0) {
            if (this.x > this._targetX) this.x = Math.max(this.x - speed * 3, this._targetX);
        } else {
            if (this.x < this._targetX) this.x = Math.min(this.x + speed * 3, this._targetX);
        }

        // Close button hover
        var mx = TouchInput.x, my = TouchInput.y;
        var lx = mx - this.x, ly = my - this.y;
        if (this._closable) {
            var wasHover = this._closeHover;
            this._closeHover = lx >= this.width - 26 && lx <= this.width - 4 && ly >= 4 && ly <= 24;
            if (this._closeHover !== wasHover) this.refresh();
            if (this._closeHover && TouchInput.isTriggered()) {
                this._dismiss();
                return;
            }
        }

        // Timer
        if (this._duration > 0) {
            this._timer--;
            if (this._timer <= 0 && !this._fadeOut) this._fadeOut = true;
        }

        // Fade out
        if (this._fadeOut) {
            this.opacity -= 6;
            if (this.opacity <= 0) this._dismiss();
        }
    };

    L2_Notification.prototype._dismiss = function () {
        this.visible = false;
        if (this.parent) this.parent.removeChild(this);
        var idx = _activeNotifs.indexOf(this);
        if (idx >= 0) _activeNotifs.splice(idx, 1);
        _repositionNotifs();
        if (this._onClose) this._onClose();
    };

    function _repositionNotifs() {
        var yy = 12;
        for (var i = 0; i < _activeNotifs.length; i++) {
            _activeNotifs[i]._targetY = yy;
            // Smoothly move to target Y
            _activeNotifs[i].y = yy;
            yy += _activeNotifs[i].height + 8;
        }
    }

    // Static API
    L2_Notification.show = function (opts) {
        var n = new L2_Notification(opts);
        _activeNotifs.push(n);
        _repositionNotifs();
        if (SceneManager._scene) SceneManager._scene.addChild(n);
        n.visible = true;
        n.opacity = 255;
        return n;
    };

    L2_Notification.info = function (title, content, duration) {
        return L2_Notification.show({ title: title, content: content, type: 'info', duration: duration });
    };
    L2_Notification.success = function (title, content, duration) {
        return L2_Notification.show({ title: title, content: content, type: 'success', duration: duration });
    };
    L2_Notification.warning = function (title, content, duration) {
        return L2_Notification.show({ title: title, content: content, type: 'warning', duration: duration });
    };
    L2_Notification.error = function (title, content, duration) {
        return L2_Notification.show({ title: title, content: content, type: 'error', duration: duration });
    };

    window.L2_Notification = L2_Notification;
})();
