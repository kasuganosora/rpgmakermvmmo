/**
 * L2_Base - Base class for all L2 UI components.
 * Extends Window_Base with L2 theming and common utilities.
 * 
 * Performance features:
 * - Layered rendering (static/dynamic layers)
 * - Dirty region tracking
 * - Cached property access
 * - Auto-resize support
 */
(function () {
    'use strict';

    // Global cache for expensive calculations
    var _colorCache = {};
    var _colorCacheCount = 0;
    var _cacheLimit = 1000;

    // Track all L2 components for resize events
    var _allComponents = [];
    var _lastGraphicsWidth = 0;
    var _lastGraphicsHeight = 0;
    var _lastResizeFrame = -1;

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
        
        // Resize handling
        this._resizeHandler = null;  // 可选的 resize 回调
        this._isCentered = false;    // 标记是否居中对齐
        this._centerOffsetX = 0;     // 相对中心的偏移
        this._centerOffsetY = 0;
        
        // Auto-register with input blocker if initially visible
        this._updateInputBlocker();
        this._updateInterval = 1;   // 默认每帧更新
        
        // Register for global resize
        _allComponents.push(this);
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

    /**
     * Set component to be centered on screen with optional offset.
     * @param {number} [offsetX=0] - Offset from center X
     * @param {number} [offsetY=0] - Offset from center Y
     */
    L2_Base.prototype.setCentered = function (offsetX, offsetY) {
        this._isCentered = true;
        this._centerOffsetX = offsetX || 0;
        this._centerOffsetY = offsetY || 0;
        this._applyCentering();
    };

    /** Apply centering based on current screen size */
    L2_Base.prototype._applyCentering = function () {
        if (!this._isCentered) return;
        var gw = Graphics.boxWidth || 816;
        var gh = Graphics.boxHeight || 624;
        this.x = Math.floor((gw - this.width) / 2) + this._centerOffsetX;
        this.y = Math.floor((gh - this.height) / 2) + this._centerOffsetY;
    };

    /**
     * Called when screen is resized.
     * Override in subclasses for custom resize behavior.
     * @param {number} oldWidth - Previous screen width
     * @param {number} oldHeight - Previous screen height
     * @param {number} newWidth - New screen width
     * @param {number} newHeight - New screen height
     */
    L2_Base.prototype.onResize = function (oldWidth, oldHeight, newWidth, newHeight) {
        // 默认行为：如果设置了居中，则重新居中
        if (this._isCentered) {
            this._applyCentering();
        }
        // 如果有自定义 resize 处理器
        if (this._resizeHandler) {
            this._resizeHandler(oldWidth, oldHeight, newWidth, newHeight);
        }
    };

    /** Set a custom resize handler */
    L2_Base.prototype.setResizeHandler = function (handler) {
        this._resizeHandler = handler;
    };

    /** Override update to support dirty checking and frame skipping. */
    var _baseUpdate = L2_Base.prototype.update;
    L2_Base.prototype.update = function () {
        _baseUpdate.call(this);
        
        // 更新输入拦截器状态
        this._updateInputBlocker();
        
        // 检查全局 resize（静态，每帧只执行一次）
        L2_Base._checkGlobalResize();
        
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

    /** Static: Check if screen size has changed (runs once per frame). */
    L2_Base._checkGlobalResize = function () {
        var frame = Graphics.frameCount || 0;
        if (frame === _lastResizeFrame) return;
        _lastResizeFrame = frame;

        var currentW = Graphics.boxWidth || 816;
        var currentH = Graphics.boxHeight || 624;

        if (currentW !== _lastGraphicsWidth || currentH !== _lastGraphicsHeight) {
            var oldW = _lastGraphicsWidth || currentW;
            var oldH = _lastGraphicsHeight || currentH;
            _lastGraphicsWidth = currentW;
            _lastGraphicsHeight = currentH;

            // 通知所有组件
            L2_Base._notifyResize(oldW, oldH, currentW, currentH);
        }
    };

    /** Static: Notify all components of resize */
    L2_Base._notifyResize = function (oldW, oldH, newW, newH) {
        for (var i = 0; i < _allComponents.length; i++) {
            var comp = _allComponents[i];
            if (comp && !comp._destroyed && comp.onResize) {
                comp.onResize(oldW, oldH, newW, newH);
            }
        }
    };

    /** Static: Purge destroyed components from tracking list (call on scene change). */
    L2_Base._purgeDestroyed = function () {
        _allComponents = _allComponents.filter(function (c) { return c && !c._destroyed; });
    };

    // Hook into SceneManager scene change to clean up stale refs
    if (typeof SceneManager !== 'undefined') {
        var _smGoto = SceneManager.goto;
        if (_smGoto) {
            SceneManager.goto = function (sceneClass) {
                L2_Base._purgeDestroyed();
                if (typeof L2_InputBlocker !== 'undefined') L2_InputBlocker.clear();
                _smGoto.call(this, sceneClass);
            };
        }
    }

    /** Static method to clear global caches. */
    L2_Base.clearCaches = function () {
        _colorCache = {};
        _colorCacheCount = 0;
        if (typeof L2_Theme !== 'undefined') L2_Theme.clearTextWidthCache();
    };

    /** Get cached color value. */
    L2_Base.getCachedColor = function (key, calculator) {
        if (_colorCache[key] !== undefined) {
            return _colorCache[key];
        }
        var value = calculator();
        // 限制缓存大小（O(1) 计数器代替 O(n) Object.keys）
        _colorCacheCount++;
        if (_colorCacheCount > _cacheLimit) {
            _colorCache = {};
            _colorCacheCount = 0;
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
        
        // Remove from global component list
        var idx = _allComponents.indexOf(this);
        if (idx >= 0) {
            _allComponents.splice(idx, 1);
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
        this._resizeHandler = null;
    };

    L2_Base.prototype.standardPadding = function () { return L2_Theme.padding; };

    /** Hit test: is (mx, my) inside this component's bounds? */
    L2_Base.prototype.isInside = function (mx, my) {
        return mx >= this.x && mx < this.x + this.width &&
               my >= this.y && my < this.y + this.height;
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
                mx >= comp.x && mx < comp.x + comp.width &&
                my >= comp.y && my < comp.y + comp.height) {
                return i;
            }
        }
        return -1;
    };

    window.L2_Base = L2_Base;
})();
