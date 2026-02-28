/**
 * L2_Theme - Lineage 2 style color palette and drawing helpers.
 * All rendering through RMMV Bitmap canvas context.
 */
(function () {
    'use strict';

    var L2_Theme = {
        // ── Backgrounds ──
        bgDark:        '#0D0D1A',
        bgPanel:       '#151528',
        bgInput:       '#0A0A18',
        bgButton:      '#1A1A30',
        bgButtonHover: '#252545',
        bgButtonPress: '#111125',
        bgTooltip:     '#0C0C1E',
        bgHeader:      '#1E1E38',
        bgHeaderEnd:   '#12122A',
        bgOverlay:     'rgba(0,0,0,0.55)',

        // ── Borders ──
        borderDark:    '#2A2A44',
        borderLight:   '#3A3A55',
        borderGold:    '#8B7500',
        borderActive:  '#BFA530',

        // ── Text ──
        textWhite:     '#E8E8E8',
        textGray:      '#AAAAAA',
        textDim:       '#666666',
        textGold:      '#FFD700',
        textTitle:     '#DDCC88',
        textGreen:     '#44FF88',
        textRed:       '#FF6666',
        textBlue:      '#88CCFF',
        textCyan:      '#66DDDD',
        textLink:      '#6699FF',
        textLinkHover: '#99BBFF',

        // ── Bars ──
        hpFill:   '#CC2222', hpBg:   '#440000',
        mpFill:   '#2222CC', mpBg:   '#000044',
        expFill:  '#CCCC00', expBg:  '#333300',

        // ── Extra Backgrounds ──
        bgLight:       '#222240',

        // ── Misc ──
        highlight:  'rgba(255,255,255,0.06)',
        selection:  'rgba(100,140,255,0.18)',
        shadow:     'rgba(0,0,0,0.55)',
        divider:    '#2A2A44',
        success:    '#44FF88',
        warning:    '#FFAA44',
        error:      '#FF4444',
        info:       '#4488FF',

        // ── Semantic colors (aliases for component API) ──
        primaryColor:  '#4488FF',
        successColor:  '#44FF88',
        warningColor:  '#FFAA44',
        dangerColor:   '#FF4444',

        // ── Constants ──
        scrollbarWidth: 6,
        defaultGap: 4,
        defaultItemHeight: 24,
        defaultBtnHeight: 28,
        charWidth: 7,
        dragThreshold: 3,

        // ── Object Pool Configuration ──
        poolEnabled: true,
        poolMaxSize: 10,

        // ── Font Configuration ──
        // 字体回退链：优先使用系统中文字体，确保中文显示清晰
        fontFamily: '"Microsoft YaHei", "PingFang SC", "Hiragino Sans GB", "WenQuanYi Micro Hei", "Noto Sans CJK SC", sans-serif',
        
        // 字体大小配置（使用偶数大小有助于像素对齐）
        fontH1:     20,
        fontH2:     18,
        fontH3:     16,
        fontTitle:  16,
        fontNormal: 14,
        fontSmall:  12,
        fontTiny:   10,
        
        // 字体渲染选项
        fontSmoothing: false,  // 禁用字体平滑以获得更多像素感
        pixelAlignText: true,  // 强制文字对齐像素

        // ── Measurements ──
        titleBarH:    26,
        padding:      8,
        cornerRadius: 3,
        lineHeight:   24,
        iconSize:     32,
        slotSize:     40,

        // ═══════════════════════════════════════════════════
        //  Drawing Helpers (operate on RMMV Bitmap)
        // ═══════════════════════════════════════════════════

        fillRoundRect: function (bmp, x, y, w, h, r, color) {
            var ctx = bmp._context;
            if (!ctx) { bmp.fillRect(x, y, w, h, color); return; }
            var oldFill = ctx.fillStyle;
            ctx.fillStyle = color;
            ctx.beginPath();
            ctx.moveTo(x + r, y);
            ctx.lineTo(x + w - r, y);
            ctx.quadraticCurveTo(x + w, y, x + w, y + r);
            ctx.lineTo(x + w, y + h - r);
            ctx.quadraticCurveTo(x + w, y + h, x + w - r, y + h);
            ctx.lineTo(x + r, y + h);
            ctx.quadraticCurveTo(x, y + h, x, y + h - r);
            ctx.lineTo(x, y + r);
            ctx.quadraticCurveTo(x, y, x + r, y);
            ctx.closePath();
            ctx.fill();
            ctx.fillStyle = oldFill;
            bmp._setDirty();
        },

        strokeRoundRect: function (bmp, x, y, w, h, r, color, lineW) {
            var ctx = bmp._context;
            if (!ctx) return;
            var oldStroke = ctx.strokeStyle;
            var oldWidth = ctx.lineWidth;
            ctx.strokeStyle = color;
            ctx.lineWidth = lineW || 1;
            ctx.beginPath();
            ctx.moveTo(x + r, y);
            ctx.lineTo(x + w - r, y);
            ctx.quadraticCurveTo(x + w, y, x + w, y + r);
            ctx.lineTo(x + w, y + h - r);
            ctx.quadraticCurveTo(x + w, y + h, x + w - r, y + h);
            ctx.lineTo(x + r, y + h);
            ctx.quadraticCurveTo(x, y + h, x, y + h - r);
            ctx.lineTo(x, y + r);
            ctx.quadraticCurveTo(x, y, x + r, y);
            ctx.closePath();
            ctx.stroke();
            ctx.strokeStyle = oldStroke;
            ctx.lineWidth = oldWidth;
            bmp._setDirty();
        },

        drawBar: function (bmp, x, y, w, h, ratio, bgColor, fillColor) {
            bmp.fillRect(x, y, w, h, bgColor);
            if (ratio > 0) {
                var fw = Math.round(w * Math.min(ratio, 1));
                bmp.fillRect(x, y, fw, h, fillColor);
                bmp.fillRect(x, y, fw, Math.max(1, Math.floor(h / 2)), 'rgba(255,255,255,0.12)');
            }
        },

        drawPanelBg: function (bmp, x, y, w, h) {
            L2_Theme.fillRoundRect(bmp, x, y, w, h, L2_Theme.cornerRadius, L2_Theme.bgPanel);
            L2_Theme.strokeRoundRect(bmp, x, y, w, h, L2_Theme.cornerRadius, L2_Theme.borderDark);
        },

        drawTitleBar: function (bmp, x, y, w, h, title) {
            bmp.gradientFillRect(x, y, w, h, L2_Theme.bgHeader, L2_Theme.bgHeaderEnd, false);
            bmp.fillRect(x, y + h - 1, w, 1, L2_Theme.borderDark);
            if (title) {
                bmp.fontSize = L2_Theme.fontTitle;
                bmp.textColor = L2_Theme.textTitle;
                bmp.drawText(title, x + 8, y + 3, w - 36, h - 4, 'left');
            }
        },

        /** drawCloseBtn(bmp, x, y, sizeOrHover, [hover])
         *  Supports 4 args (size defaults to 14) or 5 args. */
        drawCloseBtn: function (bmp, x, y, sizeOrHover, hover) {
            var size, isHover;
            if (typeof sizeOrHover === 'boolean' || hover === undefined && typeof sizeOrHover !== 'number') {
                size = 14; isHover = !!sizeOrHover;
            } else {
                size = sizeOrHover; isHover = !!hover;
            }
            var color = isHover ? L2_Theme.textRed : L2_Theme.textGray;
            var ctx = bmp._context;
            if (!ctx) return;
            ctx.save();
            ctx.strokeStyle = color;
            ctx.lineWidth = 2;
            var m = Math.max(3, Math.floor(size * 0.22));
            ctx.beginPath();
            ctx.moveTo(x + m, y + m);
            ctx.lineTo(x + size - m, y + size - m);
            ctx.moveTo(x + size - m, y + m);
            ctx.lineTo(x + m, y + size - m);
            ctx.stroke();
            ctx.restore();
            bmp._setDirty();
        },

        /** Measure text width using a Bitmap context with caching. */
        _textWidthCache: {},
        _textWidthCacheCount: 0,
        measureText: function (bmp, text, fontSize) {
            if (!text) return 0;
            var cacheKey = text + '_' + (fontSize || 13);
            var cached = L2_Theme._textWidthCache[cacheKey];
            if (cached !== undefined) return cached;

            var ctx = bmp._context;
            if (!ctx) {
                var est = text.length * (fontSize || 13) * 0.6;
                L2_Theme._textWidthCache[cacheKey] = est;
                L2_Theme._textWidthCacheCount++;
                return est;
            }
            var old = ctx.font;
            ctx.font = (fontSize || 13) + 'px ' + L2_Theme.fontFamily;
            var w = ctx.measureText(text).width;
            ctx.font = old;

            // 限制缓存大小（O(1) 计数器）
            L2_Theme._textWidthCacheCount++;
            if (L2_Theme._textWidthCacheCount > 1000) {
                L2_Theme._textWidthCache = {};
                L2_Theme._textWidthCacheCount = 0;
            }
            L2_Theme._textWidthCache[cacheKey] = w;
            return w;
        },

        /** Clear text width cache. */
        clearTextWidthCache: function () {
            L2_Theme._textWidthCache = {};
            L2_Theme._textWidthCacheCount = 0;
        },

        /** Draw a line. */
        drawLine: function (bmp, x1, y1, x2, y2, color, lineW) {
            var ctx = bmp._context;
            if (!ctx) return;
            var oldStroke = ctx.strokeStyle;
            var oldWidth = ctx.lineWidth;
            ctx.strokeStyle = color || L2_Theme.borderDark;
            ctx.lineWidth = lineW || 1;
            ctx.beginPath();
            ctx.moveTo(x1, y1);
            ctx.lineTo(x2, y2);
            ctx.stroke();
            ctx.strokeStyle = oldStroke;
            ctx.lineWidth = oldWidth;
            bmp._setDirty();
        },

        /** Draw a circle. */
        drawCircle: function (bmp, cx, cy, r, color) {
            var ctx = bmp._context;
            if (!ctx) return;
            var oldFill = ctx.fillStyle;
            ctx.fillStyle = color;
            ctx.beginPath();
            ctx.arc(cx, cy, r, 0, Math.PI * 2);
            ctx.fill();
            ctx.fillStyle = oldFill;
            bmp._setDirty();
        },

        /** Draw a checkmark. */
        drawCheck: function (bmp, x, y, size, color) {
            var ctx = bmp._context;
            if (!ctx) return;
            var oldStroke = ctx.strokeStyle;
            var oldWidth = ctx.lineWidth;
            ctx.strokeStyle = color || L2_Theme.textGreen;
            ctx.lineWidth = 2;
            ctx.beginPath();
            ctx.moveTo(x + size * 0.2, y + size * 0.5);
            ctx.lineTo(x + size * 0.4, y + size * 0.75);
            ctx.lineTo(x + size * 0.8, y + size * 0.25);
            ctx.stroke();
            ctx.strokeStyle = oldStroke;
            ctx.lineWidth = oldWidth;
            bmp._setDirty();
        },

        /** Lighten a color by a factor (0–1). Supports hex and rgba(). */
        lighten: function (color, factor) {
            var r, g, b, a;
            // 解析 rgba/rgb 格式
            var rgbaMatch = color.match(/rgba?\(\s*(\d+)\s*,\s*(\d+)\s*,\s*(\d+)\s*(?:,\s*([\d.]+))?\s*\)/);
            if (rgbaMatch) {
                r = parseInt(rgbaMatch[1], 10);
                g = parseInt(rgbaMatch[2], 10);
                b = parseInt(rgbaMatch[3], 10);
                a = rgbaMatch[4] !== undefined ? parseFloat(rgbaMatch[4]) : 1;
                r = Math.min(255, Math.round(r + (255 - r) * factor));
                g = Math.min(255, Math.round(g + (255 - g) * factor));
                b = Math.min(255, Math.round(b + (255 - b) * factor));
                return 'rgba(' + r + ',' + g + ',' + b + ',' + a + ')';
            }
            // 解析 hex 格式
            var hex = color.replace('#', '');
            if (hex.length === 3) {
                hex = hex.charAt(0) + hex.charAt(0) + hex.charAt(1) + hex.charAt(1) + hex.charAt(2) + hex.charAt(2);
            }
            r = parseInt(hex.charAt(0) + hex.charAt(1), 16);
            g = parseInt(hex.charAt(2) + hex.charAt(3), 16);
            b = parseInt(hex.charAt(4) + hex.charAt(5), 16);
            r = Math.min(255, Math.round(r + (255 - r) * factor));
            g = Math.min(255, Math.round(g + (255 - g) * factor));
            b = Math.min(255, Math.round(b + (255 - b) * factor));
            var hexChars = [
                '#',
                (r >> 4).toString(16),
                (r & 0xF).toString(16),
                (g >> 4).toString(16),
                (g & 0xF).toString(16),
                (b >> 4).toString(16),
                (b & 0xF).toString(16)
            ];
            return hexChars.join('');
        },

        /**
         * Wrap text into lines based on max width.
         * @param {string} text - Text to wrap
         * @param {number} maxW - Max width in pixels
         * @param {number} [charW=7] - Average char width
         * @returns {string[]} Array of lines
         */
        wrapText: function (text, maxW, charW) {
            charW = charW || 7;
            if (!text) return [];
            var charsPerLine = Math.max(Math.floor(maxW / charW), 1);
            var result = [];
            var paragraphs = text.split('\n');
            for (var i = 0; i < paragraphs.length; i++) {
                var line = paragraphs[i];
                while (line.length > charsPerLine) {
                    result.push(line.substring(0, charsPerLine));
                    line = line.substring(charsPerLine);
                }
                result.push(line);
            }
            return result;
        },

        /**
         * Wrap text with max char limit per line.
         * @param {string} text - Text to wrap
         * @param {number} maxChars - Max chars per line
         * @returns {string[]} Array of lines
         */
        wrapTextByChars: function (text, maxChars) {
            if (!text) return [];
            maxChars = Math.max(1, maxChars || 30);
            var result = [];
            var paragraphs = text.split('\n');
            for (var i = 0; i < paragraphs.length; i++) {
                var line = paragraphs[i];
                while (line.length > maxChars) {
                    result.push(line.substring(0, maxChars));
                    line = line.substring(maxChars);
                }
                result.push(line);
            }
            return result;
        },

        // ═══════════════════════════════════════════════════
        //  Object Pool Management
        // ═══════════════════════════════════════════════════

        _pools: {},

        /**
         * Acquire an object from pool or create new.
         * @param {string} poolName - Pool identifier
         * @param {Function} factory - Factory function to create new object
         * @returns {object} Pooled or new object
         */
        acquire: function (poolName, factory) {
            if (!L2_Theme.poolEnabled) return factory();
            var pool = L2_Theme._pools[poolName];
            if (pool && pool.length > 0) {
                return pool.pop();
            }
            return factory();
        },

        /**
         * Release object back to pool.
         * @param {string} poolName - Pool identifier
         * @param {object} obj - Object to return to pool
         * @param {Function} resetFn - Function to reset object state
         */
        release: function (poolName, obj, resetFn) {
            if (!L2_Theme.poolEnabled || !obj) return;
            var pool = L2_Theme._pools[poolName];
            if (!pool) {
                pool = [];
                L2_Theme._pools[poolName] = pool;
            }
            if (pool.length < L2_Theme.poolMaxSize) {
                if (resetFn) resetFn(obj);
                pool.push(obj);
            }
        },

        /**
         * Clear all object pools.
         */
        clearPools: function () {
            L2_Theme._pools = {};
        },

        // ═══════════════════════════════════════════════════
        //  Optimized Text Rendering (Pixel-Perfect for CJK)
        // ═══════════════════════════════════════════════════

        /**
         * Configure canvas context for sharp text rendering.
         * @param {CanvasRenderingContext2D} ctx - Canvas context
         * @param {number} fontSize - Font size
         * @param {string} color - Text color
         */
        configureTextContext: function (ctx, fontSize, color) {
            if (!ctx) return;
            ctx.font = fontSize + 'px ' + L2_Theme.fontFamily;
            ctx.fillStyle = color;
            ctx.textBaseline = 'alphabetic';
            // 保持默认平滑设置，让字体渲染更柔和自然
        },

        /**
         * Draw text with pixel-perfect alignment.
         * Ensures text coordinates are integers for sharp rendering.
         * @param {Bitmap} bmp - RMMV Bitmap
         * @param {string} text - Text to draw
         * @param {number} x - X position
         * @param {number} y - Y position
         * @param {number} maxW - Max width
         * @param {number} lineH - Line height
         * @param {string} align - Alignment ('left'|'center'|'right')
         */
        drawTextSharp: function (bmp, text, x, y, maxW, lineH, align) {
            var ctx = bmp._context;
            if (!ctx) return;
            
            // 确保坐标为整数，避免子像素模糊
            var px = Math.round(x);
            var py = Math.round(y);
            var h = Math.round(lineH || bmp.fontSize || 14);
            
            // 计算文本 Y 位置（基线对齐）
            var baseline = Math.round(py + h * 0.75); // 0.75 是视觉中心到基线的比例
            
            var tx = px;
            var mw = Math.round(maxW || 0);
            
            if (align === 'center') {
                tx = px + Math.floor(mw / 2);
            } else if (align === 'right') {
                tx = px + mw;
            }
            
            // 设置字体渲染选项
            ctx.textAlign = align || 'left';
            
            // 绘制文字
            ctx.fillText(String(text), tx, baseline, maxW || undefined);
        },

        /**
         * Measure text width with current font settings.
         * Caches font configuration to avoid repeated DOM lookups.
         * @param {CanvasRenderingContext2D} ctx - Canvas context
         * @param {string} text - Text to measure
         * @param {number} fontSize - Font size
         * @returns {number} Text width
         */
        measureTextSharp: function (ctx, text, fontSize) {
            if (!ctx) return (text || '').length * (fontSize || 14) * 0.6;
            // 使用共享的文字宽度缓存
            var cacheKey = (text || '') + '_' + (fontSize || 14);
            var cached = L2_Theme._textWidthCache[cacheKey];
            if (cached !== undefined) return cached;
            var oldFont = ctx.font;
            ctx.font = (fontSize || 14) + 'px ' + L2_Theme.fontFamily;
            var w = Math.round(ctx.measureText(text || '').width);
            ctx.font = oldFont;
            L2_Theme._textWidthCacheCount++;
            if (L2_Theme._textWidthCacheCount > 1000) {
                L2_Theme._textWidthCache = {};
                L2_Theme._textWidthCacheCount = 0;
            }
            L2_Theme._textWidthCache[cacheKey] = w;
            return w;
        },

        /**
         * Enable/disable pixel alignment for text rendering.
         * @param {CanvasRenderingContext2D} ctx - Canvas context
         * @param {boolean} enabled - Whether to enable pixel alignment
         */
        setPixelAlignment: function (ctx, enabled) {
            if (!ctx) return;
            if (enabled) {
                ctx.imageSmoothingEnabled = false;
                ctx.imageSmoothingQuality = 'low';
                if (ctx.textRendering) ctx.textRendering = 'pixelated';
            } else {
                ctx.imageSmoothingEnabled = true;
                ctx.imageSmoothingQuality = 'high';
                if (ctx.textRendering) ctx.textRendering = 'auto';
            }
        }
    };

    window.L2_Theme = L2_Theme;
})();
