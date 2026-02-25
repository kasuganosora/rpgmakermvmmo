/**
 * L2_Tree - Hierarchical tree view with expand/collapse.
 */
(function () {
    'use strict';

    function L2_Tree() { this.initialize.apply(this, arguments); }
    L2_Tree.prototype = Object.create(L2_Base.prototype);
    L2_Tree.prototype.constructor = L2_Tree;

    /**
     * @param {number} x
     * @param {number} y
     * @param {number} w
     * @param {number} h
     * @param {object} [opts] - { data: [{label, children, expanded, iconIndex}], onSelect }
     */
    L2_Tree.prototype.initialize = function (x, y, w, h, opts) {
        opts = opts || {};
        L2_Base.prototype.initialize.call(this, x, y, w, h);
        this._data = opts.data || [];
        this._onSelect = opts.onSelect || null;
        this._nodeH = L2_Theme.defaultItemHeight;
        this._indent = 18;
        this._scrollY = 0;
        this._hoverNode = null;
        this._selectedNode = null;
        this._flatNodes = [];
        this._rebuildFlat();
        this.refresh();
    };

    L2_Tree.prototype._rebuildFlat = function () {
        this._flatNodes = [];
        var self = this;
        function walk(nodes, depth) {
            for (var i = 0; i < nodes.length; i++) {
                var n = nodes[i];
                if (n.expanded === undefined) n.expanded = false;
                self._flatNodes.push({ node: n, depth: depth });
                if (n.children && n.children.length > 0 && n.expanded) {
                    walk(n.children, depth + 1);
                }
            }
        }
        walk(this._data, 0);
    };

    L2_Tree.prototype.refresh = function () {
        var c = this.bmp();
        c.clear();
        var cw = this.cw(), ch = this.ch();

        c.fillRect(0, 0, cw, ch, L2_Theme.bgDark);
        c.fontSize = L2_Theme.fontSmall;

        var startIdx = Math.floor(this._scrollY / this._nodeH);
        var visCount = Math.ceil(ch / this._nodeH) + 1;

        for (var i = startIdx; i < Math.min(startIdx + visCount, this._flatNodes.length); i++) {
            var item = this._flatNodes[i];
            var ny = i * this._nodeH - this._scrollY;
            var nx = item.depth * this._indent;

            // Hover/select bg
            if (this._selectedNode === item.node) {
                c.fillRect(0, ny, cw, this._nodeH, L2_Theme.primaryColor + '44');
            } else if (this._hoverNode === item.node) {
                c.fillRect(0, ny, cw, this._nodeH, L2_Theme.bgLight);
            }

            // Expand arrow
            if (item.node.children && item.node.children.length > 0) {
                c.textColor = L2_Theme.textGold;
                var arrow = item.node.expanded ? '▼' : '▶';
                c.drawText(arrow, nx + 2, ny, 14, this._nodeH, 'left');
            }

            // Icon
            var textX = nx + 18;
            if (item.node.iconIndex != null && item.node.iconIndex >= 0) {
                this._drawIcon(c, item.node.iconIndex, textX, ny + (this._nodeH - 16) / 2);
                textX += 20;
            }

            // Label
            c.textColor = this._selectedNode === item.node ? L2_Theme.textGold : L2_Theme.textWhite;
            c.drawText(item.node.label || '', textX, ny, cw - textX - 4, this._nodeH, 'left');
        }

        // Scrollbar
        var totalH = this._flatNodes.length * this._nodeH;
        if (totalH > ch) {
            var sbW = L2_Theme.scrollbarWidth;
            var barH = Math.max(ch * ch / totalH, 20);
            var barY = (this._scrollY / (totalH - ch)) * (ch - barH);
            c.fillRect(cw - sbW, barY, sbW, barH, L2_Theme.textGray + '66');
        }
    };

    L2_Tree.prototype._drawIcon = function (c, iconIndex, dx, dy) {
        var bitmap = ImageManager.loadSystem('IconSet');
        if (!bitmap || !bitmap.isReady()) return;
        var pw = 32, ph = 32;
        var sx = (iconIndex % 16) * pw;
        var sy = Math.floor(iconIndex / 16) * ph;
        c.blt(bitmap, sx, sy, pw, ph, dx, dy, 16, 16);
    };

    L2_Tree.prototype.update = function () {
        L2_Base.prototype.update.call(this);
        if (!this.visible) return;
        var mx = TouchInput.x, my = TouchInput.y;
        var lx = mx - this.x - this.padding;
        var ly = my - this.y - this.padding;
        var inside = lx >= 0 && lx < this.cw() && ly >= 0 && ly < this.ch();

        // Scroll
        if (inside && TouchInput.wheelY) {
            var totalH = this._flatNodes.length * this._nodeH;
            this._scrollY = Math.max(0, Math.min(this._scrollY + TouchInput.wheelY * 20, totalH - this.ch()));
            this.refresh();
        }

        // Hover
        var oldHover = this._hoverNode;
        this._hoverNode = null;
        if (inside) {
            var idx = Math.floor((ly + this._scrollY) / this._nodeH);
            if (idx >= 0 && idx < this._flatNodes.length) {
                this._hoverNode = this._flatNodes[idx].node;
            }
        }
        if (this._hoverNode !== oldHover) this.refresh();

        // Click
        if (inside && TouchInput.isTriggered() && this._hoverNode) {
            var node = this._hoverNode;
            if (node.children && node.children.length > 0) {
                node.expanded = !node.expanded;
                this._rebuildFlat();
            }
            this._selectedNode = node;
            if (this._onSelect) this._onSelect(node);
            this.markDirty();
        }
    };

    window.L2_Tree = L2_Tree;
})();
