/**
 * L2_ChatBubble - Auto-sizing chat bubble with text wrapping and tail pointer.
 * Supports typewriter animation with center-outward bubble expansion.
 */
(function () {
    'use strict';

    var PAD_X = 12;
    var PAD_Y = 8;
    var LINE_H = 18;
    var TAIL_H = 8;
    var TAIL_HALF = 6;
    var MIN_W = 40;
    var WRAP_MARGIN = 4;
    var DEFAULT_MAX_W = 200;
    var DEFAULT_CORNER_R = 8;
    var DEFAULT_FONT_SIZE = 13;
    var DEFAULT_TYPEWRITER_SPEED = 12; // frames per char (~0.2s at 60fps)
    var DEFAULT_TEXT_DELAY = 12; // frames (~0.2s at 60fps)

    function L2_ChatBubble() { this.initialize.apply(this, arguments); }
    L2_ChatBubble.prototype = Object.create(L2_Base.prototype);
    L2_ChatBubble.prototype.constructor = L2_ChatBubble;

    /**
     * @param {number} x
     * @param {number} y
     * @param {string} text
     * @param {object} [opts]
     * @param {number} [opts.maxWidth=200]
     * @param {number} [opts.fontSize=13]
     * @param {string} [opts.textColor='#333']
     * @param {string} [opts.bgColor='rgba(255,255,255,0.95)']
     * @param {string} [opts.borderColor='rgba(0,0,0,0.15)']
     * @param {number} [opts.tailSize=8]
     * @param {string} [opts.tailAlign='center'] - 'left'|'center'|'right'
     * @param {number} [opts.duration=300] - frames, 0=permanent
     * @param {number} [opts.cornerRadius=8]
     * @param {number|boolean} [opts.typewriter=0] - frames per char (0=disabled, true=default 12)
     * @param {number} [opts.textDelay=12] - frames text lags behind bubble
     * @param {function} [opts.onDispose]
     * @param {function} [opts.onTypewriterDone]
     */
    L2_ChatBubble.prototype.initialize = function (x, y, text, opts) {
        opts = opts || {};
        this._text = text || '';
        this._maxWidth = opts.maxWidth || DEFAULT_MAX_W;
        this._fontSize = opts.fontSize || DEFAULT_FONT_SIZE;
        this._textColor = opts.textColor || '#333333';
        this._bgColor = opts.bgColor || 'rgba(255,255,255,0.95)';
        this._borderColor = opts.borderColor || 'rgba(0,0,0,0.15)';
        this._tailH = opts.tailSize !== undefined ? opts.tailSize : TAIL_H;
        this._tailAlign = opts.tailAlign || 'center';
        this._cornerR = opts.cornerRadius !== undefined ? opts.cornerRadius : DEFAULT_CORNER_R;
        this._duration = opts.duration !== undefined ? opts.duration : 300;
        this._timer = this._duration;
        this._fading = false;
        this._disposed = false;
        this._onDispose = opts.onDispose || null;
        this._lines = [];

        // Typewriter
        var tw = opts.typewriter;
        this._typewriterSpeed = tw === true ? DEFAULT_TYPEWRITER_SPEED : (tw || 0);
        this._typewriterFrame = 0;
        this._revealedChars = 0;
        this._totalChars = 0;
        this._typewriterDone = false;
        this._onTypewriterDone = opts.onTypewriterDone || null;
        this._textDelay = opts.textDelay !== undefined ? opts.textDelay : DEFAULT_TEXT_DELAY;
        this._elapsedFrames = 0;

        // Init with maxWidth so bitmap is large enough.
        // Accurate layout is deferred to first refresh() when context exists.
        var initH = LINE_H + PAD_Y * 2 + this._tailH;
        L2_Base.prototype.initialize.call(this, x, y, this._maxWidth, initH);
        this._needsLayout = true;

        // Do initial layout (may use fallback estimates if no context yet)
        this._doLayout();
        this.refresh();
    };

    L2_ChatBubble.prototype.standardPadding = function () { return 0; };

    L2_ChatBubble.prototype.setText = function (text) {
        if (this._text === text) return;
        this._text = text;
        this._needsLayout = true;
        if (this._typewriterSpeed > 0) {
            this._revealedChars = 0;
            this._typewriterDone = false;
            this._typewriterFrame = 0;
            this._elapsedFrames = 0;
        }
        this.markDirty();
    };

    L2_ChatBubble.prototype._countTotalChars = function () {
        var total = 0;
        for (var i = 0; i < this._lines.length; i++) total += this._lines[i].length;
        return total;
    };

    L2_ChatBubble.prototype.skipTypewriter = function () {
        if (this._typewriterDone) return;
        this._revealedChars = this._totalChars;
        this._typewriterDone = true;
        this._elapsedFrames = Infinity;
        this.markDirty();
        if (this._onTypewriterDone) this._onTypewriterDone();
    };

    // ---- Text wrapping ----

    L2_ChatBubble.prototype._wrapTextAccurate = function (text, maxW) {
        var bmp = this.bmp();
        var fontSize = this._fontSize;
        if (!text) return [];
        var paragraphs = text.split('\n');
        var result = [];
        for (var p = 0; p < paragraphs.length; p++) {
            var line = paragraphs[p];
            if (!line) { result.push(''); continue; }
            var current = '';
            for (var i = 0; i < line.length; i++) {
                var test = current + line[i];
                var w = L2_Theme.measureText(bmp, test, fontSize);
                if (w > maxW - WRAP_MARGIN && current.length > 0) {
                    result.push(current);
                    current = line[i];
                } else {
                    current = test;
                }
            }
            if (current) result.push(current);
        }
        return result;
    };

    /** Full layout: wrap text, count chars, set window to final size. */
    L2_ChatBubble.prototype._doLayout = function () {
        var contentMaxW = this._maxWidth - PAD_X * 2;
        this._lines = this._wrapTextAccurate(this._text, contentMaxW);
        this._totalChars = this._countTotalChars();

        if (this._typewriterSpeed <= 0) {
            this._revealedChars = this._totalChars;
            this._typewriterDone = true;
        }

        // Measure widest line for final bubble width
        var bmp = this.bmp();
        var fontSize = this._fontSize;
        var maxLineW = 0;
        for (var i = 0; i < this._lines.length; i++) {
            var w = L2_Theme.measureText(bmp, this._lines[i], fontSize);
            maxLineW = Math.max(maxLineW, w);
        }
        var fullW = Math.max(Math.ceil(maxLineW) + PAD_X * 2 + 4, MIN_W);
        fullW = Math.min(fullW, this._maxWidth);
        var fullH = Math.max(this._lines.length, 1) * LINE_H + PAD_Y * 2 + this._tailH;

        if (this.width !== fullW || this.height !== fullH) {
            this.width = fullW;
            this.height = fullH;
            this.createContents();
        }
        this._needsLayout = false;
    };

    // ---- Visible text helpers ----

    L2_ChatBubble.prototype._getVisibleLines = function (charCount) {
        var charsLeft = charCount;
        var visible = [];
        for (var i = 0; i < this._lines.length; i++) {
            if (charsLeft <= 0) break;
            var line = this._lines[i];
            if (line.length <= charsLeft) {
                visible.push(line);
                charsLeft -= line.length;
            } else {
                visible.push(line.substring(0, charsLeft));
                charsLeft = 0;
            }
        }
        return visible;
    };

    L2_ChatBubble.prototype._measureVisibleWidth = function (visible) {
        var bmp = this.bmp();
        var fontSize = this._fontSize;
        var maxW = 0;
        for (var i = 0; i < visible.length; i++) {
            var w = L2_Theme.measureText(bmp, visible[i], fontSize);
            maxW = Math.max(maxW, w);
        }
        return maxW;
    };

    // ---- Render ----

    L2_ChatBubble.prototype.refresh = function () {
        var c = this.bmp();
        c.clear();
        var ctx = c._context;
        if (!ctx) return;

        // Re-layout with accurate context if needed
        if (this._needsLayout) {
            var savedCtx = ctx;
            this._doLayout();
            // createContents may have made a new bitmap — restore context
            c = this.bmp();
            c._context = savedCtx;
            ctx = savedCtx;
        }

        var fullW = this.cw();
        var fullH = this.ch();
        var tailH = this._tailH;

        // ---- Calculate bubble size from revealed chars ----
        var bubbleChars = this._revealedChars;
        var bubbleVisible = this._getVisibleLines(bubbleChars);
        if (bubbleVisible.length === 0 && bubbleChars <= 0) return;
        if (bubbleVisible.length === 0) bubbleVisible = [''];

        var visTextW = this._measureVisibleWidth(bubbleVisible);
        var vw = Math.max(Math.ceil(visTextW) + PAD_X * 2 + 4, MIN_W);
        vw = Math.min(vw, fullW);
        var vBodyH = bubbleVisible.length * LINE_H + PAD_Y * 2;

        // ---- Position: expansion direction based on tailAlign ----
        var offsetX;
        if (this._tailAlign === 'left') {
            offsetX = 0;                              // tail left, expand right
        } else if (this._tailAlign === 'right') {
            offsetX = fullW - vw;                     // tail right, expand left
        } else {
            offsetX = Math.floor((fullW - vw) / 2);  // center, expand both
        }
        var tailTopY = fullH - tailH;
        var offsetY = tailTopY - vBodyH;

        var r = Math.min(this._cornerR, vw / 2, vBodyH / 2);

        // Tail X: fixed position based on tailAlign
        var tailCx;
        if (this._tailAlign === 'left') {
            tailCx = Math.max(r + TAIL_HALF + 2, 16);
        } else if (this._tailAlign === 'right') {
            tailCx = fullW - Math.max(r + TAIL_HALF + 2, 16);
        } else {
            tailCx = Math.floor(fullW / 2);
        }
        tailCx = Math.max(offsetX + TAIL_HALF + 2, Math.min(tailCx, offsetX + vw - TAIL_HALF - 2));

        // ---- Build bubble + tail path ----
        ctx.save();
        ctx.beginPath();
        ctx.moveTo(offsetX + r, offsetY);
        ctx.lineTo(offsetX + vw - r, offsetY);
        ctx.quadraticCurveTo(offsetX + vw, offsetY, offsetX + vw, offsetY + r);
        ctx.lineTo(offsetX + vw, tailTopY - r);
        ctx.quadraticCurveTo(offsetX + vw, tailTopY, offsetX + vw - r, tailTopY);
        ctx.lineTo(tailCx + TAIL_HALF, tailTopY);
        ctx.lineTo(tailCx, tailTopY + tailH);
        ctx.lineTo(tailCx - TAIL_HALF, tailTopY);
        ctx.lineTo(offsetX + r, tailTopY);
        ctx.quadraticCurveTo(offsetX, tailTopY, offsetX, tailTopY - r);
        ctx.lineTo(offsetX, offsetY + r);
        ctx.quadraticCurveTo(offsetX, offsetY, offsetX + r, offsetY);
        ctx.closePath();

        // Fill
        var oldFill = ctx.fillStyle;
        ctx.fillStyle = this._bgColor;
        ctx.fill();
        ctx.fillStyle = oldFill;

        // Stroke
        var oldStroke = ctx.strokeStyle;
        var oldLineW = ctx.lineWidth;
        ctx.strokeStyle = this._borderColor;
        ctx.lineWidth = 1;
        ctx.stroke();
        ctx.strokeStyle = oldStroke;
        ctx.lineWidth = oldLineW;

        // Clip to bubble shape
        ctx.clip();

        // ---- Text: delayed behind bubble expansion ----
        var textChars;
        if (this._typewriterSpeed > 0 && !this._typewriterDone) {
            var delayChars = Math.ceil(this._textDelay / Math.max(this._typewriterSpeed, 1));
            textChars = Math.max(0, this._revealedChars - delayChars);
        } else {
            textChars = this._totalChars;
        }

        if (textChars > 0) {
            var textVisible = this._getVisibleLines(textChars);
            ctx.font = this._fontSize + 'px ' + L2_Theme.fontFamily;
            ctx.fillStyle = this._textColor;
            ctx.textAlign = 'left';
            ctx.textBaseline = 'alphabetic';

            // Text at final positions, clipped by bubble shape
            for (var i = 0; i < textVisible.length; i++) {
                var tx = Math.round(offsetX + PAD_X);
                var ty = Math.round(offsetY + PAD_Y + i * LINE_H + LINE_H * 0.75);
                ctx.fillText(textVisible[i], tx, ty);
            }
        }

        ctx.restore();
    };

    // ---- Update ----

    L2_ChatBubble.prototype.update = function () {
        L2_Base.prototype.update.call(this);
        if (this._disposed || !this.visible) return;

        if (this._typewriterSpeed > 0 && !this._typewriterDone) {
            this._elapsedFrames++;
            this._typewriterFrame++;
            if (this._typewriterFrame >= this._typewriterSpeed) {
                this._typewriterFrame = 0;
                this._revealedChars++;
                this.markDirty();
                if (this._revealedChars >= this._totalChars) {
                    this._typewriterDone = true;
                    if (this._onTypewriterDone) this._onTypewriterDone();
                }
            }
            // Also mark dirty for text delay catch-up
            if (!this._typewriterDone) this.markDirty();
            return;
        }

        // Continued text delay catch-up after bubble done
        if (this._typewriterSpeed > 0 && this._typewriterDone) {
            var delayChars = Math.ceil(this._textDelay / Math.max(this._typewriterSpeed, 1));
            if (this._revealedChars - delayChars < this._totalChars) {
                this._elapsedFrames++;
                this.markDirty();
                return;
            }
        }

        if (this._duration > 0 && !this._fading) {
            this._timer--;
            if (this._timer <= 0) {
                this._fading = true;
            }
        }

        if (this._fading) {
            this.opacity = Math.max(0, this.opacity - 6);
            if (this.opacity <= 0) {
                this._dispose();
            }
        }
    };

    L2_ChatBubble.prototype._dispose = function () {
        if (this._disposed) return;
        this._disposed = true;
        this.visible = false;
        if (this.parent) this.parent.removeChild(this);
        this.destroy();
        if (this._onDispose) this._onDispose();
    };

    window.L2_ChatBubble = L2_ChatBubble;
})();
