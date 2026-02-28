/**
 * L2_Layout - Flex-like layout container (horizontal/vertical).
 * Positions children sequentially with gap spacing and automatic wrapping.
 */
(function () {
    'use strict';

    function L2_Layout() { this.initialize.apply(this, arguments); }
    L2_Layout.prototype = Object.create(L2_Base.prototype);
    L2_Layout.prototype.constructor = L2_Layout;

    /**
     * @param {number} x
     * @param {number} y
     * @param {number} w
     * @param {number} h
     * @param {object} [opts] - { 
     *     direction:'horizontal'|'vertical', 
     *     gap, 
     *     align:'start'|'center'|'end'
     * }
     */
    L2_Layout.prototype.initialize = function (x, y, w, h, opts) {
        L2_Base.prototype.initialize.call(this, x, y, w, h);
        opts = opts || {};
        this._direction = opts.direction || 'horizontal';
        this._gap = opts.gap !== undefined ? opts.gap : L2_Theme.defaultGap;
        this._align = opts.align || 'start';
        this._managed = [];
    };

    L2_Layout.prototype.standardPadding = function () { return 0; };

    L2_Layout.prototype.addItem = function (component) {
        this._managed.push(component);
        this.addChild(component);
        this.layoutItems();
    };

    /** Batch add multiple items (avoids O(N²) layout recalc). */
    L2_Layout.prototype.addItems = function (components) {
        for (var i = 0; i < components.length; i++) {
            this._managed.push(components[i]);
            this.addChild(components[i]);
        }
        this.layoutItems();
    };

    L2_Layout.prototype.clearItems = function () {
        var self = this;
        this._managed.forEach(function (c) {
            if (c.parent === self) self.removeChild(c);
            if (c.destroy) c.destroy();
        });
        this._managed = [];
    };

    L2_Layout.prototype.layoutItems = function () {
        var isH = this._direction === 'horizontal';
        var containerMain = isH ? this.cw() : this.ch();  // 主轴容器大小
        var containerCross = isH ? this.ch() : this.cw(); // 交叉轴容器大小
        
        var pos = 0;
        var crossPos = 0;
        var lineStartIdx = 0;
        var lineSize = 0;  // 当前行/列在交叉轴上的大小
        
        // 第一遍：计算位置，处理换行
        for (var i = 0; i < this._managed.length; i++) {
            var item = this._managed[i];
            var itemMain = isH ? item.width : item.height;
            var itemCross = isH ? item.height : item.width;
            
            // 检查是否需要换行（自动换行）
            if (i > lineStartIdx && pos + itemMain > containerMain) {
                // 完成上一行的布局（应用对齐）
                this._layoutLine(lineStartIdx, i - 1, crossPos, lineSize, containerCross);
                // 开始新行
                crossPos += lineSize + this._gap;
                pos = 0;
                lineStartIdx = i;
                lineSize = 0;
            }
            
            // 计算交叉轴位置
            var itemCrossPos = crossPos;
            // 应用对齐
            if (this._align === 'center') {
                itemCrossPos = crossPos + Math.floor((lineSize - itemCross) / 2);
            } else if (this._align === 'end') {
                itemCrossPos = crossPos + lineSize - itemCross;
            }
            
            // 临时存储位置（最终位置可能在换行处理后才确定）
            item._tempMain = pos;
            item._tempCross = itemCrossPos;
            
            pos += itemMain + this._gap;
            lineSize = Math.max(lineSize, itemCross);
        }
        
        // 布局最后一行
        if (lineStartIdx < this._managed.length) {
            this._layoutLine(lineStartIdx, this._managed.length - 1, crossPos, lineSize, containerCross);
        }
    };

    /** Layout a line of items with alignment */
    L2_Layout.prototype._layoutLine = function (startIdx, endIdx, crossPos, lineSize, containerCross) {
        var isH = this._direction === 'horizontal';
        
        for (var i = startIdx; i <= endIdx; i++) {
            var item = this._managed[i];
            var itemCross = isH ? item.height : item.width;
            
            // 计算交叉轴对齐位置
            var finalCross = crossPos;
            // 在 wrap 模式下也应用对齐
            if (this._align === 'center') {
                finalCross = crossPos + Math.floor((lineSize - itemCross) / 2);
            } else if (this._align === 'end') {
                finalCross = crossPos + lineSize - itemCross;
            }
            
            if (isH) {
                item.x = item._tempMain;
                item.y = finalCross;
            } else {
                item.x = finalCross;
                item.y = item._tempMain;
            }
            
            // 清理临时属性（用赋值代替 delete 避免 V8 去优化）
            item._tempMain = undefined;
            item._tempCross = undefined;
        }
    };

    L2_Layout.prototype.refresh = function () { this.bmp().clear(); };

    window.L2_Layout = L2_Layout;
})();
