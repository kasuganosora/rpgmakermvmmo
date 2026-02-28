/**
 * L2_Textarea - Multi-line text display/input area.
 */
(function () {
    'use strict';

    function L2_Textarea() { this.initialize.apply(this, arguments); }
    L2_Textarea.prototype = Object.create(L2_Base.prototype);
    L2_Textarea.prototype.constructor = L2_Textarea;

    L2_Textarea.prototype.initialize = function (x, y, w, h, opts) {
        L2_Base.prototype.initialize.call(this, x, y, w, h);
        opts = opts || {};
        this._text = opts.text || '';
        this._lineHeight = opts.lineHeight || 18;
        this._scrollY = 0;
        this._editable = opts.editable || false;
        this._focused = false;
        this._cursorBlink = 0;
        this._onChange = opts.onChange || null;
        this.refresh();
    };

    L2_Textarea.prototype.standardPadding = function () { return 4; };

    L2_Textarea.prototype.getText = function () { return this._text; };
    L2_Textarea.prototype.setText = function (t) { this._text = t; this._scrollY = 0; this._cachedLinesText = null; this.markDirty(); };

    L2_Textarea.prototype.refresh = function () {
        var c = this.bmp();
        c.clear();
        var cw = this.cw(), ch = this.ch();

        c.fillRect(0, 0, cw, ch, L2_Theme.bgDark);
        L2_Theme.strokeRoundRect(c, 0, 0, cw, ch, 2,
            this._focused ? L2_Theme.borderActive : L2_Theme.borderDark);

        c.fontSize = L2_Theme.fontNormal;
        c.textColor = L2_Theme.textWhite;

        // 使用缓存的行数组避免每帧 split
        if (!this._cachedLines || this._cachedLinesText !== this._text) {
            this._cachedLines = this._text.split('\n');
            this._cachedLinesText = this._text;
        }
        var lines = this._cachedLines;
        var lh = this._lineHeight;
        var startLine = Math.floor(this._scrollY / lh);
        var visLines = Math.ceil(ch / lh);

        for (var i = startLine; i < Math.min(startLine + visLines, lines.length); i++) {
            var ly = i * lh - this._scrollY + 2;
            if (ly + lh < 0 || ly > ch) continue;
            c.drawText(lines[i], 4, ly, cw - 8, lh, 'left');
        }

        // Scrollbar
        var totalH = lines.length * lh;
        if (totalH > ch) {
            var sbW = 4;
            var thumbH = Math.max(16, Math.round(ch * (ch / totalH)));
            var thumbY = Math.round((ch - thumbH) * (this._scrollY / (totalH - ch)));
            c.fillRect(cw - sbW, 0, sbW, ch, 'rgba(0,0,0,0.3)');
            L2_Theme.fillRoundRect(c, cw - sbW, thumbY, sbW, thumbH, 2, '#444466');
        }
    };

    L2_Textarea.prototype.update = function () {
        L2_Base.prototype.update.call(this);
        if (!this.visible) return;

        var inside = this.isInside(TouchInput.x, TouchInput.y);

        if (TouchInput.isTriggered()) {
            this._focused = inside && this._editable;
            this._cursorBlink = 0;
            this.markDirty();
        }

        // Scroll
        if (inside && TouchInput.wheelY) {
            var lh = this._lineHeight;
            if (!this._cachedLines || this._cachedLinesText !== this._text) {
                this._cachedLines = this._text.split('\n');
                this._cachedLinesText = this._text;
            }
            var totalH = this._cachedLines.length * lh;
            this._scrollY += TouchInput.wheelY > 0 ? lh * 2 : -lh * 2;
            this._scrollY = Math.max(0, Math.min(this._scrollY, Math.max(0, totalH - this.ch())));
            this.markDirty();
        }

        // Keyboard input (when editable and focused)
        if (this._focused && this._editable) {
            this._cursorBlink = (this._cursorBlink + 1) % 60;
            var changed = false;
            // 读取按键状态
            var keyMap = Input._keys;
            if (keyMap) {
                var validChars = 'abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789!@#$%^&*()_+-=[]{}|;:\'",.<>?/~`\\ ';
                for (var key in keyMap) {
                    if (keyMap[key] && validChars.indexOf(key) >= 0) {
                        this._text += key;
                        keyMap[key] = false;
                        changed = true;
                    }
                }
            }
            if (Input.isTriggered('backspace') && this._text.length > 0) {
                this._text = this._text.slice(0, -1);
                changed = true;
            }
            if (Input.isTriggered('escape')) { this._focused = false; this.markDirty(); }
            if (changed) {
                if (this._onChange) this._onChange(this._text);
                this.markDirty();
            }
        }
    };

    window.L2_Textarea = L2_Textarea;
})();
