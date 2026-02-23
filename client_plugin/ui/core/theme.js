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

        // ── Font sizes ──
        fontH1:     20,
        fontH2:     17,
        fontH3:     15,
        fontTitle:  15,
        fontNormal: 13,
        fontSmall:  11,
        fontTiny:   10,

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
            ctx.save();
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
            ctx.restore();
            bmp._setDirty();
        },

        strokeRoundRect: function (bmp, x, y, w, h, r, color, lineW) {
            var ctx = bmp._context;
            if (!ctx) return;
            ctx.save();
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
            ctx.restore();
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

        /** Measure text width using a Bitmap context. */
        measureText: function (bmp, text, fontSize) {
            var ctx = bmp._context;
            if (!ctx) return text.length * (fontSize || 13) * 0.6;
            var old = ctx.font;
            ctx.font = (fontSize || 13) + 'px GameFont';
            var w = ctx.measureText(text).width;
            ctx.font = old;
            return w;
        },

        /** Draw a line. */
        drawLine: function (bmp, x1, y1, x2, y2, color, lineW) {
            var ctx = bmp._context;
            if (!ctx) return;
            ctx.save();
            ctx.strokeStyle = color || L2_Theme.borderDark;
            ctx.lineWidth = lineW || 1;
            ctx.beginPath();
            ctx.moveTo(x1, y1);
            ctx.lineTo(x2, y2);
            ctx.stroke();
            ctx.restore();
            bmp._setDirty();
        },

        /** Draw a circle. */
        drawCircle: function (bmp, cx, cy, r, color) {
            var ctx = bmp._context;
            if (!ctx) return;
            ctx.save();
            ctx.fillStyle = color;
            ctx.beginPath();
            ctx.arc(cx, cy, r, 0, Math.PI * 2);
            ctx.fill();
            ctx.restore();
            bmp._setDirty();
        },

        /** Draw a checkmark. */
        drawCheck: function (bmp, x, y, size, color) {
            var ctx = bmp._context;
            if (!ctx) return;
            ctx.save();
            ctx.strokeStyle = color || L2_Theme.textGreen;
            ctx.lineWidth = 2;
            ctx.beginPath();
            ctx.moveTo(x + size * 0.2, y + size * 0.5);
            ctx.lineTo(x + size * 0.4, y + size * 0.75);
            ctx.lineTo(x + size * 0.8, y + size * 0.25);
            ctx.stroke();
            ctx.restore();
            bmp._setDirty();
        },

        /** Lighten a hex color by a factor (0–1). */
        lighten: function (hex, factor) {
            hex = hex.replace('#', '');
            if (hex.length === 3) hex = hex[0]+hex[0]+hex[1]+hex[1]+hex[2]+hex[2];
            var r = parseInt(hex.substring(0, 2), 16);
            var g = parseInt(hex.substring(2, 4), 16);
            var b = parseInt(hex.substring(4, 6), 16);
            r = Math.min(255, Math.round(r + (255 - r) * factor));
            g = Math.min(255, Math.round(g + (255 - g) * factor));
            b = Math.min(255, Math.round(b + (255 - b) * factor));
            return '#' + ((1 << 24) + (r << 16) + (g << 8) + b).toString(16).slice(1);
        }
    };

    window.L2_Theme = L2_Theme;
})();
