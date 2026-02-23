/**
 * L2_Collapse - Expandable/collapsible content panels.
 */
(function () {
    'use strict';

    function L2_Collapse() { this.initialize.apply(this, arguments); }
    L2_Collapse.prototype = Object.create(L2_Base.prototype);
    L2_Collapse.prototype.constructor = L2_Collapse;

    /**
     * @param {number} x
     * @param {number} y
     * @param {number} w
     * @param {object} [opts] - { items: [{title, content, expanded}], accordion }
     */
    L2_Collapse.prototype.initialize = function (x, y, w, opts) {
        opts = opts || {};
        this._items = (opts.items || []).map(function (it) {
            return { title: it.title || '', content: it.content || '', expanded: !!it.expanded };
        });
        this._accordion = opts.accordion || false;
        this._headerH = 28;
        this._lineH = 18;
        this._hoverIdx = -1;
        var h = this._calcHeight();
        L2_Base.prototype.initialize.call(this, x, y, w, h + 4);
        this.refresh();
    };

    L2_Collapse.prototype.standardPadding = function () { return 2; };

    L2_Collapse.prototype._calcHeight = function () {
        var h = 0;
        for (var i = 0; i < this._items.length; i++) {
            h += this._headerH;
            if (this._items[i].expanded) {
                var lines = this._wrapText(this._items[i].content);
                h += lines.length * this._lineH + 8;
            }
        }
        return Math.max(h, this._headerH);
    };

    L2_Collapse.prototype._wrapText = function (text) {
        if (!text) return [''];
        var maxW = this.cw() - 20;
        var charW = 7;
        var charsPerLine = Math.max(Math.floor(maxW / charW), 1);
        var result = [];
        var lines = text.split('\n');
        for (var i = 0; i < lines.length; i++) {
            var line = lines[i];
            while (line.length > charsPerLine) {
                result.push(line.substring(0, charsPerLine));
                line = line.substring(charsPerLine);
            }
            result.push(line);
        }
        return result;
    };

    L2_Collapse.prototype.refresh = function () {
        var newH = this._calcHeight() + 4;
        if (this.height !== newH) {
            this.height = newH;
            this.createContents();
        }
        var c = this.bmp();
        c.clear();
        var cw = this.cw(), yy = 0;

        for (var i = 0; i < this._items.length; i++) {
            var item = this._items[i];
            // Header bg
            var hBg = (this._hoverIdx === i) ? L2_Theme.bgLight : L2_Theme.bgPanel;
            c.fillRect(0, yy, cw, this._headerH, hBg);
            // Arrow
            c.fontSize = L2_Theme.fontSmall;
            c.textColor = L2_Theme.textGold;
            var arrow = item.expanded ? '▼' : '▶';
            c.drawText(arrow, 6, yy, 14, this._headerH, 'left');
            // Title
            c.textColor = L2_Theme.textWhite;
            c.drawText(item.title, 22, yy, cw - 28, this._headerH, 'left');
            // Separator
            c.fillRect(0, yy + this._headerH - 1, cw, 1, L2_Theme.borderDark);
            yy += this._headerH;

            if (item.expanded) {
                var lines = this._wrapText(item.content);
                c.fontSize = L2_Theme.fontSmall;
                c.textColor = L2_Theme.textGray;
                for (var j = 0; j < lines.length; j++) {
                    c.drawText(lines[j], 10, yy + 4 + j * this._lineH, cw - 20, this._lineH, 'left');
                }
                yy += lines.length * this._lineH + 8;
            }
        }
    };

    L2_Collapse.prototype.update = function () {
        L2_Base.prototype.update.call(this);
        if (!this.visible || this._items.length === 0) return;
        var mx = TouchInput.x, my = TouchInput.y;
        var lx = mx - this.x - this.padding;
        var ly = my - this.y - this.padding;
        var oldHover = this._hoverIdx;
        this._hoverIdx = -1;

        var yy = 0;
        for (var i = 0; i < this._items.length; i++) {
            if (ly >= yy && ly < yy + this._headerH && lx >= 0 && lx < this.cw()) {
                this._hoverIdx = i;
            }
            yy += this._headerH;
            if (this._items[i].expanded) {
                var lines = this._wrapText(this._items[i].content);
                yy += lines.length * this._lineH + 8;
            }
        }
        if (this._hoverIdx !== oldHover) this.refresh();

        if (TouchInput.isTriggered() && this._hoverIdx >= 0) {
            this._toggle(this._hoverIdx);
        }
    };

    L2_Collapse.prototype._toggle = function (idx) {
        if (this._accordion) {
            for (var i = 0; i < this._items.length; i++) {
                if (i !== idx) this._items[i].expanded = false;
            }
        }
        this._items[idx].expanded = !this._items[idx].expanded;
        this.refresh();
    };

    window.L2_Collapse = L2_Collapse;
})();
