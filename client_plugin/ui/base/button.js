/**
 * L2_Button - Clickable button with hover/pressed states.
 */
(function () {
    'use strict';

    function L2_Button() { this.initialize.apply(this, arguments); }
    L2_Button.prototype = Object.create(L2_Base.prototype);
    L2_Button.prototype.constructor = L2_Button;

    /**
     * Supports two signatures:
     *   new L2_Button(x, y, label, opts)           — auto-size
     *   new L2_Button(x, y, w, h, label, opts)     — explicit size
     */
    L2_Button.prototype.initialize = function (x, y, wOrLabel, hOrOpts, label, opts) {
        if (typeof wOrLabel === 'string') {
            // Auto-size: (x, y, label, opts)
            label = wOrLabel;
            opts = hOrOpts || {};
            var autoW = Math.max(label.length * 12 + 20 + (opts.icon >= 0 ? 28 : 0), 50);
            L2_Base.prototype.initialize.call(this, x, y, autoW + 4, 28 + 4);
        } else {
            // Explicit: (x, y, w, h, label, opts)
            opts = opts || {};
            L2_Base.prototype.initialize.call(this, x, y, wOrLabel, hOrOpts);
        }
        this._label = label || '';
        this._onClick = opts.onClick || null;
        this._iconIndex = opts.icon !== undefined ? opts.icon : -1;
        this._type = opts.type || 'default';
        this._enabled = opts.enabled !== false;
        this._hover = false;
        this._pressed = false;
        this.refresh();
    };

    L2_Button.prototype.standardPadding = function () { return 2; };

    L2_Button.prototype.setLabel = function (t) { this._label = t; this.markDirty(); };
    L2_Button.prototype.setEnabled = function (b) { this._enabled = b; this.markDirty(); };
    L2_Button.prototype.setOnClick = function (fn) { this._onClick = fn; };

    L2_Button.prototype._getColors = function () {
        var bg, border, text;
        switch (this._type) {
            case 'primary':
                bg = this._pressed ? '#0D2A60' : (this._hover ? '#1A3A70' : '#142A55');
                border = this._hover ? '#4488DD' : '#335599';
                text = L2_Theme.textWhite;
                break;
            case 'success':
                bg = this._pressed ? '#0D2A15' : (this._hover ? '#1A3A25' : '#142A1A');
                border = this._hover ? '#44DD88' : '#338855';
                text = L2_Theme.textGreen;
                break;
            case 'danger':
                bg = this._pressed ? '#2A0D0D' : (this._hover ? '#3A1A1A' : '#2A1414');
                border = this._hover ? '#DD4444' : '#993333';
                text = L2_Theme.textRed;
                break;
            case 'text':
                bg = this._hover ? L2_Theme.highlight : 'rgba(0,0,0,0)';
                border = 'rgba(0,0,0,0)';
                text = this._hover ? L2_Theme.textWhite : L2_Theme.textGray;
                break;
            default:
                bg = this._pressed ? L2_Theme.bgButtonPress :
                     (this._hover ? L2_Theme.bgButtonHover : L2_Theme.bgButton);
                border = this._hover ? L2_Theme.borderGold : L2_Theme.borderDark;
                text = L2_Theme.textWhite;
        }
        if (!this._enabled) {
            bg = '#111118';
            border = '#1A1A22';
            text = L2_Theme.textDim;
        }
        return { bg: bg, border: border, text: text };
    };

    L2_Button.prototype.refresh = function () {
        var c = this.bmp();
        var cw = this.cw(), ch = this.ch();
        var colors = this._getColors();

        // 只有背景层脏时才重绘背景
        if (this.isLayerDirty('bg')) {
            c.clear();
            if (this._type !== 'text') {
                L2_Theme.fillRoundRect(c, 0, 0, cw, ch, L2_Theme.cornerRadius, colors.bg);
                L2_Theme.strokeRoundRect(c, 0, 0, cw, ch, L2_Theme.cornerRadius, colors.border);
            }
            this.markLayerClean('bg');
        }

        // 内容层总是重绘（因为文字可能变化）
        if (this.isLayerDirty('content')) {
            // 如果背景不是透明的，需要清除内容区域
            if (this._type === 'text') {
                c.clear();
            }
            
            var textX = 0, textW = cw;
            // Draw icon if present
            if (this._iconIndex >= 0) {
                var iconSize = 20;
                var iconX = 6;
                var iconY = (ch - iconSize) / 2;
                // Draw from IconSet
                if (ImageManager && ImageManager.loadSystem) {
                    var iconSet = ImageManager.loadSystem('IconSet');
                    if (iconSet && iconSet.isReady()) {
                        var pw = Window_Base._iconWidth || 32;
                        var ph = Window_Base._iconHeight || 32;
                        var sx = (this._iconIndex % 16) * pw;
                        var sy = Math.floor(this._iconIndex / 16) * ph;
                        c.blt(iconSet, sx, sy, pw, ph, iconX, iconY, iconSize, iconSize);
                    }
                }
                textX = iconX + iconSize + 4;
                textW = cw - textX - 4;
            }

            // 使用锐化文字渲染
            var ctx = c._context;
            if (ctx) {
                L2_Theme.configureTextContext(ctx, L2_Theme.fontNormal, colors.text);
                var align = this._iconIndex >= 0 ? 'left' : 'center';
                L2_Theme.drawTextSharp(c, this._label, textX, 0, textW, ch, align);
            } else {
                c.fontSize = L2_Theme.fontNormal;
                c.textColor = colors.text;
                c.drawText(this._label, textX, 0, textW, ch, this._iconIndex >= 0 ? 'left' : 'center');
            }
            this.markLayerClean('content');
        }
    };

    L2_Button.prototype.update = function () {
        L2_Base.prototype.update.call(this);
        if (!this.visible) return;

        var mx = TouchInput.x, my = TouchInput.y;
        var inside = this.isInside(mx, my);
        var wasHover = this._hover;
        var wasPressed = this._pressed;
        this._hover = inside && this._enabled;

        if (this._enabled && inside && TouchInput.isTriggered()) {
            this._pressed = true;
        }
        if (this._pressed && !TouchInput.isPressed()) {
            this._pressed = false;
            if (inside && this._onClick) this._onClick();
        }
        if (!TouchInput.isPressed()) this._pressed = false;

        // 只有当状态真正改变时才标记为脏
        if (wasHover !== this._hover || wasPressed !== this._pressed) {
            this.markDirty('bg');
            this.markDirty('content');
        }
    };

    /**
     * Optimized hover state update for batch processing.
     * @private
     */
    L2_Button.prototype._updateHoverState = function (inside, triggered, pressed) {
        var wasHover = this._hover;
        var wasPressed = this._pressed;
        this._hover = inside && this._enabled;
        
        if (this._enabled && inside && triggered) {
            this._pressed = true;
        }
        if (this._pressed && !pressed) {
            this._pressed = false;
            if (inside && this._onClick) this._onClick();
        }
        if (!pressed) this._pressed = false;
        
        if (wasHover !== this._hover || wasPressed !== this._pressed) {
            this.markDirty('bg');
            this.markDirty('content');
        }
    };

    window.L2_Button = L2_Button;
})();
