/**
 * L2_Base - Base class for all L2 UI components.
 * Extends Window_Base with L2 theming and common utilities.
 * 
 * Performance features:
 * - Layered rendering (static/dynamic layers)
 * - Dirty region tracking
 * - Cached property access
 */
(function () {
    'use strict';

    // Global cache for expensive calculations
    var _colorCache = {};
    var _textWidthCache = {};
    var _cacheLimit = 1000;

    function L2_Base() { this.initialize.apply(this, arguments); }
    L2_Base.prototype = Object.create(Window_Base.prototype);
    L2_Base.prototype.constructor = L2_Base;

    L2_Base.prototype.initialize = function (x, y, w, h) {
        Window_Base.prototype.initialize.call(this, x, y, w, h);
        this.opacity = 0;       // hide RMMV windowskin
        this.backOpacity = 0;
        this._dirty = true;     // 脏检查标志
        this._dirtyLayers = { bg: true, content: true }; // 分层脏标记
        this._destroyed = false;
        this._cacheKey = null;  // 缓存键
        this._lastFrameUpdated = 0; // 帧率控制
        this._lastVisibleState = false; // 用于输入拦截器状态跟踪
        
        // Auto-register with input blocker if initially visible
        this._updateInputBlocker();
        this._updateInterval = 1;   // 默认每帧更新
    };

    /** Mark component as needing redraw. */
    L2_Base.prototype.markDirty = function (layer) {
        this._dirty = true;
        if (layer) {
            this._dirtyLayers[layer] = true;
        } else {
            this._dirtyLayers.bg = true;
            this._dirtyLayers.content = true;
        }
    };

    /** Mark specific layer as clean after render. */
    L2_Base.prototype.markLayerClean = function (layer) {
        if (layer) {
            this._dirtyLayers[layer] = false;
        }
        // 如果所有层都干净了，整体标记为干净
        if (!this._dirtyLayers.bg && !this._dirtyLayers.content) {
            this._dirty = false;
        }
    };

    /** Check if component needs redraw. */
    L2_Base.prototype.isDirty = function () {
        return this._dirty;
    };

    /** Check if specific layer needs redraw. */
    L2_Base.prototype.isLayerDirty = function (layer) {
        return this._dirtyLayers[layer] !== false;
    };

    /** Set update interval for frame skipping (performance control). */
    L2_Base.prototype.setUpdateInterval = function (frames) {
        this._updateInterval = Math.max(1, frames);
    };

    /** Override update to support dirty checking and frame skipping. */
    var _baseUpdate = L2_Base.prototype.update;
    L2_Base.prototype.update = function () {
        _baseUpdate.call(this);
        
        // 更新输入拦截器状态
        this._updateInputBlocker();
        
        // 帧跳过优化
        this._lastFrameUpdated++;
        if (this._lastFrameUpdated < this._updateInterval) {
            return;
        }
        this._lastFrameUpdated = 0;
        
        if (this._dirty) {
            this.refresh();
            this._dirtyLayers.bg = false;
            this._dirtyLayers.content = false;
            this._dirty = false;
        }
    };

    /** Static method to clear global caches. */
    L2_Base.clearCaches = function () {
        _colorCache = {};
        _textWidthCache = {};
    };

    /** Get cached color value. */
    L2_Base.getCachedColor = function (key, calculator) {
        if (_colorCache[key] !== undefined) {
            return _colorCache[key];
        }
        var value = calculator();
        // 限制缓存大小
        if (Object.keys(_colorCache).length > _cacheLimit) {
            _colorCache = {};
        }
        _colorCache[key] = value;
        return value;
    };

    /**
     * Auto-register/unregister from input blocker based on visibility.
     * Called automatically in update method.
     */
    L2_Base.prototype._updateInputBlocker = function () {
        if (typeof L2_InputBlocker === 'undefined') return;
        
        var wasVisible = this._lastVisibleState;
        var isVisible = this.visible && !this._destroyed;
        
        if (wasVisible !== isVisible) {
            this._lastVisibleState = isVisible;
            if (isVisible) {
                L2_InputBlocker.register(this);
            } else {
                L2_InputBlocker.unregister(this);
            }
        }
    };

    /**
     * Destroy the component and cleanup resources.
     * Override in subclasses for specific cleanup.
     */
    L2_Base.prototype.destroy = function () {
        if (this._destroyed) return;
        this._destroyed = true;
        
        // Unregister from input blocker
        if (typeof L2_InputBlocker !== 'undefined') {
            L2_InputBlocker.unregister(this);
        }
        
        // Remove from parent
        if (this.parent) {
            this.parent.removeChild(this);
        }
        
        // Cleanup children
        if (this.children) {
            for (var i = this.children.length - 1; i >= 0; i--) {
                var child = this.children[i];
                if (child && child.destroy) {
                    child.destroy();
                }
            }
            this.children = [];
        }
        
        // Cleanup bitmap
        if (this.contents) {
            this.contents = null;
        }
        
        // Remove event listeners if any
        this._onChange = null;
        this._onClick = null;
        this._onSelect = null;
        this._onClose = null;
        this._onSubmit = null;
    };

    L2_Base.prototype.standardPadding = function () { return L2_Theme.padding; };

    /** Hit test: is (mx, my) inside this component's bounds? */
    L2_Base.prototype.isInside = function (mx, my) {
        return mx >= this.x && mx <= this.x + this.width &&
               my >= this.y && my <= this.y + this.height;
    };

    /** Convert screen coords to local content coords. */
    L2_Base.prototype.toLocal = function (mx, my) {
        return { x: mx - this.x - this.padding, y: my - this.y - this.padding };
    };

    /** Shorthand for contents bitmap. */
    L2_Base.prototype.bmp = function () { return this.contents; };

    /** Shorthand for contents width. */
    L2_Base.prototype.cw = function () { return this.contentsWidth(); };

    /** Shorthand for contents height. */
    L2_Base.prototype.ch = function () { return this.contentsHeight(); };

    /**
     * Batch update multiple components efficiently.
     * Call this instead of individual updates when possible.
     */
    L2_Base.batchUpdate = function (components) {
        var mx = TouchInput.x;
        var my = TouchInput.y;
        var triggered = TouchInput.isTriggered();
        var pressed = TouchInput.isPressed();
        
        for (var i = 0; i < components.length; i++) {
            var comp = components[i];
            if (!comp || !comp.visible || comp._destroyed) continue;
            
            // 批量检测鼠标状态
            if (comp._updateHoverState) {
                var inside = comp.isInside(mx, my);
                comp._updateHoverState(inside, triggered, pressed);
            }
            
            // 调用标准 update
            if (comp.update) {
                comp.update();
            }
        }
    };

    /**
     * Fast hit test without creating objects.
     * Returns -1 if not inside, or the component index if inside.
     */
    L2_Base.fastHitTest = function (components, mx, my) {
        for (var i = components.length - 1; i >= 0; i--) {
            var comp = components[i];
            if (comp && comp.visible && !comp._destroyed &&
                mx >= comp.x && mx <= comp.x + comp.width &&
                my >= comp.y && my <= comp.y + comp.height) {
                return i;
            }
        }
        return -1;
    };

    window.L2_Base = L2_Base;
})();
