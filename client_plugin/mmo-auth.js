/*:
 * @plugindesc v3.1.0 MMO 认证模块 - 登录、角色选择与角色创建场景（L2 UI 主题）。
 * @author MMO Framework
 *
 * @help
 * ════════════════════════════════════════════════════════════════════
 *  MMO 认证插件 (mmo-auth.js)
 * ════════════════════════════════════════════════════════════════════
 *
 *  本插件为 MMO 框架提供完整的用户认证流程，包含三个自定义场景：
 *
 *  1. Scene_Login          - 用户登录/注册界面
 *  2. Scene_CharacterSelect - 角色选择界面（支持多角色）
 *  3. Scene_CharacterCreate - 角色创建界面（选择名称、职业、外观）
 *
 *  本插件覆盖了 Scene_Title 的 start 方法，使游戏启动时
 *  自动跳转到 Scene_Login 而非默认标题画面。
 *
 *  所有 UI 组件采用 L2 主题系统（毛玻璃面板、金色高亮等）。
 *  HTML 原生输入框用于账号/密码，确保中文输入法兼容。
 *
 *  依赖插件：
 *  - mmo-core.js（提供 $MMO 全局对象）
 *  - L2 UI 组件库（L2_Base, L2_Button, L2_Typography 等）
 *
 *  无插件参数。
 * ════════════════════════════════════════════════════════════════════
 */

(function () {
    'use strict';

    // ═══════════════════════════════════════════════════════════════
    //  基础配置
    // ═══════════════════════════════════════════════════════════════

    /**
     * 服务器 HTTP 基地址。
     * 从 $MMO._serverUrl（WebSocket 地址）转换为 HTTP 地址。
     * 例如：ws://localhost:8080 → http://localhost:8080
     * @type {string}
     */
    var BASE_URL = ($MMO._serverUrl || 'ws://localhost:8080').replace(/^ws/, 'http');

    // ═══════════════════════════════════════════════════════════════
    //  键盘输入修复：HTML 输入框获得焦点时让浏览器处理按键
    // ═══════════════════════════════════════════════════════════════
    // RMMV 默认会拦截所有键盘事件用于游戏输入处理。
    // 当 HTML <input> 或 <textarea> 获得焦点时，必须放行按键事件，
    // 否则用户无法在输入框中正常打字。

    /**
     * 保存 Input._shouldPreventDefault 的原始实现。
     * @type {Function}
     */
    var _Input_shouldPreventDefault = Input._shouldPreventDefault;

    /**
     * 覆盖 Input._shouldPreventDefault，当焦点在 HTML 输入元素上时不阻止默认行为。
     * @param {number} keyCode - 按键代码
     * @returns {boolean} 是否应阻止默认行为
     */
    Input._shouldPreventDefault = function (keyCode) {
        var active = document.activeElement;
        if (active && (active.tagName === 'INPUT' || active.tagName === 'TEXTAREA')) {
            return false;
        }
        return _Input_shouldPreventDefault.call(this, keyCode);
    };

    /**
     * 保存 Input._onKeyDown 的原始实现。
     * @type {Function}
     */
    var _Input_onKeyDown = Input._onKeyDown;

    /**
     * 覆盖 Input._onKeyDown，当焦点在 HTML 输入元素上时跳过 RMMV 按键处理。
     * @param {KeyboardEvent} event - 键盘按下事件
     * @returns {void}
     */
    Input._onKeyDown = function (event) {
        var active = document.activeElement;
        if (active && (active.tagName === 'INPUT' || active.tagName === 'TEXTAREA')) return;
        _Input_onKeyDown.call(this, event);
    };

    /**
     * 保存 Input._onKeyUp 的原始实现。
     * @type {Function}
     */
    var _Input_onKeyUp = Input._onKeyUp;

    /**
     * 覆盖 Input._onKeyUp，当焦点在 HTML 输入元素上时跳过 RMMV 按键处理。
     * @param {KeyboardEvent} event - 键盘释放事件
     * @returns {void}
     */
    Input._onKeyUp = function (event) {
        var active = document.activeElement;
        if (active && (active.tagName === 'INPUT' || active.tagName === 'TEXTAREA')) return;
        _Input_onKeyUp.call(this, event);
    };

    // ═══════════════════════════════════════════════════════════════
    //  $MMO.http — HTTP 请求工具
    // ═══════════════════════════════════════════════════════════════
    // 封装 XMLHttpRequest，提供 Promise 风格的 GET/POST/DELETE 方法。
    // 自动附加 JWT 认证令牌（如已登录），自动序列化/反序列化 JSON。

    /**
     * HTTP 请求工具对象，挂载在 $MMO 上供全局使用。
     * @namespace $MMO.http
     */
    $MMO.http = {
        /**
         * 发送 HTTP 请求的内部方法。
         * 自动设置 Content-Type 为 JSON，自动附加 Authorization 头。
         * @param {string} method - HTTP 方法（GET/POST/DELETE）
         * @param {string} path - 请求路径（相对于 BASE_URL）
         * @param {Object|null} body - 请求体对象（会被 JSON.stringify 序列化）
         * @returns {Promise<Object>} 解析后的 JSON 响应数据
         */
        _req: function (method, path, body) {
            return new Promise(function (resolve, reject) {
                var xhr = new XMLHttpRequest();
                xhr.open(method, BASE_URL + path);
                xhr.setRequestHeader('Content-Type', 'application/json');
                if ($MMO.token) {
                    xhr.setRequestHeader('Authorization', 'Bearer ' + $MMO.token);
                }
                xhr.onload = function () {
                    try {
                        var data = JSON.parse(xhr.responseText);
                        if (xhr.status >= 200 && xhr.status < 300) {
                            resolve(data);
                        } else {
                            reject(new Error(data.error || 'HTTP ' + xhr.status));
                        }
                    } catch (e) {
                        reject(e);
                    }
                };
                xhr.onerror = function () { reject(new Error('Network error')); };
                xhr.send(body ? JSON.stringify(body) : null);
            });
        },

        /**
         * 发送 GET 请求。
         * @param {string} path - 请求路径（相对于 BASE_URL）
         * @returns {Promise<Object>} 解析后的 JSON 响应数据
         */
        get: function (path) { return this._req('GET', path, null); },

        /**
         * 发送 POST 请求。
         * @param {string} path - 请求路径（相对于 BASE_URL）
         * @param {Object} body - 请求体对象
         * @returns {Promise<Object>} 解析后的 JSON 响应数据
         */
        post: function (path, body) { return this._req('POST', path, body); },

        /**
         * 发送 DELETE 请求。
         * @param {string} path - 请求路径（相对于 BASE_URL）
         * @param {Object|null} body - 请求体对象（可选）
         * @returns {Promise<Object>} 解析后的 JSON 响应数据
         */
        del: function (path, body) { return this._req('DELETE', path, body || null); }
    };

    // ═══════════════════════════════════════════════════════════════
    //  辅助函数 — 可自动重定位的 HTML 输入框
    // ═══════════════════════════════════════════════════════════════
    // RMMV 的画布会随窗口大小缩放，HTML 输入框必须跟随缩放。
    // 所有通过 makeL2Input 创建的输入框会被记录在 _trackedInputs 中，
    // 并在窗口 resize 事件触发时自动重新计算位置和尺寸。

    /**
     * 跟踪的输入框列表，每项包含元素引用和游戏坐标。
     * @type {Array<{el: HTMLInputElement, gx: number, gy: number, gw: number, gh: number}>}
     */
    var _trackedInputs = [];

    /**
     * 重新计算所有被跟踪输入框的位置和尺寸。
     * 根据画布的实际渲染尺寸与游戏逻辑尺寸的比例进行缩放，
     * 确保输入框始终与画布上的对应位置对齐。
     * @returns {void}
     */
    function _repositionInputs() {
        var bw = Graphics.boxWidth || 816;
        var bh = Graphics.boxHeight || 624;
        var canvas = Graphics._canvas;
        var rect = canvas
            ? canvas.getBoundingClientRect()
            : { left: 0, top: 0, width: bw, height: bh };
        var sx = rect.width / bw;
        var sy = rect.height / bh;
        for (var i = 0; i < _trackedInputs.length; i++) {
            var t = _trackedInputs[i];
            t.el.style.left   = (rect.left + t.gx * sx) + 'px';
            t.el.style.top    = (rect.top  + t.gy * sy) + 'px';
            t.el.style.width  = (t.gw * sx) + 'px';
            t.el.style.height = (t.gh * sy) + 'px';
            t.el.style.fontSize = Math.round(13 * sy) + 'px';
        }
    }

    // 监听窗口大小变化，自动重新定位输入框
    window.addEventListener('resize', _repositionInputs);

    /**
     * 创建一个 L2 主题风格的 HTML 输入框。
     * 输入框使用固定定位，叠加在游戏画布上方，样式匹配 L2 暗色主题。
     * 自动阻止 RMMV 的 TouchInput 和键盘事件拦截，确保输入框正常工作。
     * @param {string} type - 输入类型（'text' 或 'password'）
     * @param {string} placeholder - 占位提示文本
     * @param {number} gameX - 游戏坐标系中的 X 位置
     * @param {number} gameY - 游戏坐标系中的 Y 位置
     * @param {number} w - 游戏坐标系中的宽度
     * @param {number} [h=28] - 游戏坐标系中的高度（默认 28）
     * @returns {HTMLInputElement} 创建的输入框元素
     */
    function makeL2Input(type, placeholder, gameX, gameY, w, h) {
        h = h || 28;
        var el = document.createElement('input');
        el.type = type;
        el.placeholder = placeholder;
        el.style.cssText = [
            'position:fixed',
            'font-family:GameFont,sans-serif',
            'background:#0A0A18',
            'color:#E8E8E8',
            'border:1px solid #2A2A44',
            'border-radius:3px',
            'z-index:10000',
            '-webkit-user-select:text',
            'user-select:text',
            'outline:none',
            'padding:2px 8px',
            'box-sizing:border-box',
            'transition:border-color 0.2s',
            'pointer-events:auto'
        ].join(';');
        // 获得焦点时高亮边框为金色
        el.addEventListener('focus', function () { el.style.borderColor = '#BFA530'; });
        // 失去焦点时恢复默认边框颜色
        el.addEventListener('blur', function () { el.style.borderColor = '#2A2A44'; });
        // 阻止 RMMV TouchInput 抢夺焦点/点击事件
        el.addEventListener('mousedown', function (e) { e.stopPropagation(); el.focus(); });
        el.addEventListener('touchstart', function (e) { e.stopPropagation(); el.focus(); });
        el.addEventListener('click', function (e) { e.stopPropagation(); el.focus(); });
        // 阻止 RPG Maker 拦截键盘输入
        el.addEventListener('keydown', function (e) { e.stopPropagation(); });
        el.addEventListener('keyup', function (e) { e.stopPropagation(); });
        el.addEventListener('keypress', function (e) { e.stopPropagation(); });
        document.body.appendChild(el);
        _trackedInputs.push({ el: el, gx: gameX, gy: gameY, gw: w, gh: h });
        _repositionInputs();
        return el;
    }

    /**
     * 从 DOM 中移除一个 HTML 元素，并从跟踪列表中清除。
     * @param {HTMLElement} el - 要移除的元素
     * @returns {void}
     */
    function removeEl(el) {
        if (el && el.parentNode) el.parentNode.removeChild(el);
        for (var i = _trackedInputs.length - 1; i >= 0; i--) {
            if (_trackedInputs[i].el === el) { _trackedInputs.splice(i, 1); break; }
        }
    }

    /**
     * 为场景创建标题画面背景精灵。
     * 使用 $dataSystem.title1Name 作为背景图片资源。
     * @param {Scene_Base} scene - 目标场景实例
     * @returns {void}
     */
    function createBackground(scene) {
        scene._backSprite = new Sprite(ImageManager.loadTitle1($dataSystem.title1Name));
        scene.addChild(scene._backSprite);
    }

    // ═══════════════════════════════════════════════════════════════
    //  毛玻璃面板 — 模糊背景 + 50% 半透明遮罩
    // ═══════════════════════════════════════════════════════════════

    /**
     * 创建毛玻璃效果面板。
     * 该面板由三层构成：
     *   1. 模糊处理的背景副本（通过圆角矩形遮罩裁剪）
     *   2. 50% 透明度的深色遮罩层
     *   3. L2_Base 面板（绘制边框和可选标题栏）
     * @param {Scene_Base} scene - 目标场景实例
     * @param {number} x - 面板左上角 X 坐标
     * @param {number} y - 面板左上角 Y 坐标
     * @param {number} w - 面板宽度
     * @param {number} h - 面板高度
     * @param {string|null} title - 标题栏文本（传 null 则无标题栏）
     * @returns {L2_Base} 面板的 L2_Base 实例
     */
    function createGlassPanel(scene, x, y, w, h, title) {
        // 第一层：模糊处理的背景副本，通过遮罩限制在面板区域内
        var bgBmp = scene._backSprite.bitmap;
        var blurBg = new Sprite(bgBmp);
        if (PIXI.filters && PIXI.filters.BlurFilter) {
            blurBg.filters = [new PIXI.filters.BlurFilter(8)];
        }
        var mask = new PIXI.Graphics();
        mask.beginFill(0xFFFFFF);
        mask.drawRoundedRect(x, y, w, h, 4);
        mask.endFill();
        scene.addChild(mask);
        blurBg.mask = mask;
        scene.addChild(blurBg);

        // 第二层：50% 透明度的深色遮罩
        var overlayBmp = new Bitmap(w, h);
        overlayBmp.fillRect(0, 0, w, h, 'rgba(13,13,26,0.50)');
        var overlay = new Sprite(overlayBmp);
        overlay.x = x;
        overlay.y = y;
        scene.addChild(overlay);

        // 第三层：L2_Base 面板，绘制边框和标题栏
        var panel = new L2_Base(x, y, w, h);
        var c = panel.bmp();
        var cw = panel.cw(), ch = panel.ch();
        L2_Theme.strokeRoundRect(c, 0, 0, cw, ch, L2_Theme.cornerRadius,
            'rgba(100,120,180,0.35)');
        if (title) {
            L2_Theme.drawTitleBar(c, 0, 0, cw, L2_Theme.titleBarH, title);
        }
        scene.addChild(panel);
        return panel;
    }

    // ═══════════════════════════════════════════════════════════════
    //  PanelLayout — 自动计算面板高度的布局工具
    // ═══════════════════════════════════════════════════════════════
    // 用法示例：
    //   var L = new PanelLayout(x, y, w, {title:'...'});
    //   var row1Y = L.row(46);  // 分配一个 46px 高的行，返回绝对 Y 坐标
    //   var row2Y = L.row(46);
    //   var ph = L.height();    // 自动计算出的总高度
    //   createGlassPanel(scene, x, y, w, ph, title);

    /**
     * 面板布局计算器。
     * 通过逐行分配空间来自动计算面板总高度，避免手动计算。
     * @constructor
     * @param {number} x - 面板左上角 X 坐标
     * @param {number} y - 面板左上角 Y 坐标
     * @param {number} w - 面板宽度
     * @param {Object} [opts] - 可选配置
     * @param {number} [opts.padX=8] - 水平内边距
     * @param {number} [opts.padY=16] - 垂直内边距
     * @param {string|null} [opts.title=null] - 标题文本（有标题时会预留标题栏高度）
     */
    function PanelLayout(x, y, w, opts) {
        opts = opts || {};
        this.x = x;
        this.y = y;
        this.w = w;
        this._padX = opts.padX || 8;
        this._padY = opts.padY || 16;
        this._title = opts.title || null;
        this._cy = this._title ? (L2_Theme.titleBarH + this._padY) : this._padY;
    }

    /**
     * 分配一行空间并返回该行的绝对屏幕 Y 坐标。
     * @param {number} [h=40] - 行高（像素）
     * @returns {number} 该行的绝对 Y 坐标
     */
    PanelLayout.prototype.row = function (h) {
        var absY = this.y + this._cy;
        this._cy += (h || 40);
        return absY;
    };

    /**
     * 获取面板总高度（在所有行分配完毕后调用）。
     * @returns {number} 面板总高度（包含上下内边距）
     */
    PanelLayout.prototype.height = function () { return this._cy + this._padY; };

    /**
     * 获取内容区域的起始绝对 X 坐标（用于标签文字）。
     * @returns {number} 内容区域的绝对 X 坐标
     */
    PanelLayout.prototype.cx = function () { return this.x + this._padX; };

    /**
     * 获取输入框的起始绝对 X 坐标（标签之后的位置）。
     * @param {number} [labelW=80] - 标签宽度
     * @returns {number} 输入框的绝对 X 坐标
     */
    PanelLayout.prototype.ix = function (labelW) { return this.x + this._padX + (labelW || 80) + 10; };

    /**
     * 获取输入框的可用宽度。
     * @param {number} [labelW=80] - 标签宽度
     * @returns {number} 输入框可用宽度
     */
    PanelLayout.prototype.iw = function (labelW) { return this.w - this._padX * 2 - (labelW || 80) - 14; };

    /**
     * 绘制不透明实心面板（用于次要面板）。
     * @param {L2_Base} base - L2_Base 面板实例
     * @param {string|null} title - 标题文本（传 null 则无标题栏）
     * @returns {void}
     */
    function drawPanel(base, title) {
        var c = base.bmp();
        c.clear();
        var cw = base.cw(), ch = base.ch();
        L2_Theme.drawPanelBg(c, 0, 0, cw, ch);
        if (title) {
            L2_Theme.drawTitleBar(c, 0, 0, cw, L2_Theme.titleBarH, title);
        }
    }

    // ═══════════════════════════════════════════════════════════════
    //  角色创建预设数据
    // ═══════════════════════════════════════════════════════════════

    /**
     * 职业选项列表。
     * label 为显示名称，value 为对应的职业 ID。
     * @type {Array<{label: string, value: number}>}
     */
    var CLASS_OPTIONS = [
        { label: '未变身', value: 1 },
        { label: '变身中', value: 2 },
        { label: '魅魔',   value: 3 }
    ];

    /**
     * 角色外观（立绘）预设列表。
     * faceName 为角色行走图资源文件名，faceIndex 为该文件中的索引。
     * @type {Array<{faceName: string, faceIndex: number}>}
     */
    var FACE_PRESETS = [
        { faceName: 'actor01_0001', faceIndex: 0 },
        { faceName: 'actor01_0002', faceIndex: 0 },
        { faceName: 'actor01_0003', faceIndex: 0 },
        { faceName: 'actor01_0004', faceIndex: 0 },
        { faceName: 'actor02_0001', faceIndex: 0 },
        { faceName: 'actor03_0001', faceIndex: 0 },
        { faceName: 'Actor_Heroine', faceIndex: 0 },
        { faceName: 'actor_Memoria', faceIndex: 0 }
    ];

    // ═══════════════════════════════════════════════════════════════
    //  Scene_Login — 登录场景
    // ═══════════════════════════════════════════════════════════════
    // 用户输入账号和密码，可选择登录或注册。
    // 登录/注册成功后获取 JWT 令牌并跳转至角色选择界面。

    /**
     * 登录场景构造函数。
     * 提供账号密码输入、登录按钮和注册按钮。
     * @constructor
     */
    function Scene_Login() { this.initialize.apply(this, arguments); }
    Scene_Login.prototype = Object.create(Scene_Base.prototype);
    Scene_Login.prototype.constructor = Scene_Login;

    /**
     * 初始化登录场景。
     * 创建输入框数组和繁忙状态标志。
     * @returns {void}
     */
    Scene_Login.prototype.initialize = function () {
        Scene_Base.prototype.initialize.call(this);
        /** @type {HTMLInputElement[]} 场景中的所有 HTML 输入框 */
        this._inputs = [];
        /** @type {boolean} 是否正在处理请求（防止重复提交） */
        this._busy = false;
    };

    /**
     * 创建登录场景的所有 UI 元素。
     * 包括：背景、标题文字、毛玻璃面板、账号/密码输入框、登录/注册按钮。
     * 设置回车键快捷操作：账号框回车跳转到密码框，密码框回车触发登录。
     * @returns {void}
     */
    Scene_Login.prototype.create = function () {
        Scene_Base.prototype.create.call(this);
        createBackground(this);

        var gw = Graphics.boxWidth;
        var pw = 400;
        var px = (gw - pw) / 2, py = 175;

        // ── 布局：先计算各行位置，面板高度自动推导 ──
        var L = new PanelLayout(px, py, pw);
        var accountY = L.row(46);
        var passwordY = L.row(46);
        var buttonY = L.row(44);
        var ph = L.height();

        // ── 标题文字 ──
        this.addChild(new L2_Typography((gw - 300) / 2, 115, 300, {
            text: 'MMO LOGIN', level: 'h1', align: 'center', color: L2_Theme.textGold
        }));

        // ── 毛玻璃面板（高度由布局自动计算） ──
        this._panel = createGlassPanel(this, px, py, pw, ph, null);

        // 第一行：账号输入
        this.addChild(new L2_Typography(L.cx() + 4, accountY + 4, 80, {
            text: 'Account', color: L2_Theme.textGray
        }));
        this._userInput = makeL2Input('text', 'Enter username', L.ix(), accountY, L.iw(), 28);
        this._inputs.push(this._userInput);

        // 第二行：密码输入
        this.addChild(new L2_Typography(L.cx() + 4, passwordY + 4, 80, {
            text: 'Password', color: L2_Theme.textGray
        }));
        this._passInput = makeL2Input('password', 'Enter password', L.ix(), passwordY, L.iw(), 28);
        this._inputs.push(this._passInput);

        // ── 按钮（位于面板内部） ──
        this._loginBtn = new L2_Button(px + 55, buttonY, 'Login', {
            type: 'primary',
            onClick: this._doLogin.bind(this)
        });
        this.addChild(this._loginBtn);

        this._regBtn = new L2_Button(px + pw - 170, buttonY, 'Register', {
            onClick: this._doRegister.bind(this)
        });
        this.addChild(this._regBtn);

        // ── 键盘快捷键 ──
        var self = this;
        // 账号框按回车 → 跳转到密码框
        this._userInput.addEventListener('keydown', function (e) {
            if (e.keyCode === 13) { e.stopPropagation(); self._passInput.focus(); }
        });
        // 密码框按回车 → 触发登录
        this._passInput.addEventListener('keydown', function (e) {
            if (e.keyCode === 13) { e.stopPropagation(); self._doLogin(); }
        });
    };

    /**
     * 场景启动时的处理。
     * 确保所有输入框可见且可交互，延迟 200ms 后自动聚焦账号输入框。
     * @returns {void}
     */
    Scene_Login.prototype.start = function () {
        Scene_Base.prototype.start.call(this);
        // 确保输入框可见且可获取焦点
        this._inputs.forEach(function (el) {
            el.style.display = 'block';
            el.style.visibility = 'visible';
            el.style.opacity = '1';
        });
        var self = this;
        this._focusTimer = setTimeout(function () {
            if (self._userInput) self._userInput.focus();
        }, 200);
    };

    /**
     * 场景停止时的处理（尚未销毁）。
     * 隐藏所有 HTML 输入框，防止在场景切换过程中可见。
     * @returns {void}
     */
    Scene_Login.prototype.stop = function () {
        // 场景停止时隐藏输入框（此时场景尚未销毁）
        this._inputs.forEach(function (el) {
            el.style.display = 'none';
        });
        Scene_Base.prototype.stop.call(this);
    };

    /**
     * 执行登录操作。
     * 校验用户名和密码非空后，向服务器发送登录请求。
     * 成功后保存 JWT 令牌并跳转至角色选择界面。
     * @returns {void}
     */
    Scene_Login.prototype._doLogin = function () {
        if (this._busy) return;
        var self = this;
        var username = this._userInput.value.trim();
        var password = this._passInput.value;
        if (!username || !password) {
            L2_Message.warning('Please enter username and password.');
            return;
        }
        this._busy = true;
        $MMO.http.post('/api/auth/login', { username: username, password: password })
            .then(function (data) {
                $MMO.token = data.token;
                self._cleanup();
                SceneManager.goto(Scene_CharacterSelect);
            })
            .catch(function (e) {
                self._busy = false;
                L2_Message.error(e.message);
            });
    };

    /**
     * 执行注册操作。
     * 校验用户名和密码非空后，向服务器发送注册请求。
     * 注册成功后自动登录（服务器返回 JWT 令牌），跳转至角色选择界面。
     * @returns {void}
     */
    Scene_Login.prototype._doRegister = function () {
        if (this._busy) return;
        var self = this;
        var username = this._userInput.value.trim();
        var password = this._passInput.value;
        if (!username || !password) {
            L2_Message.warning('Please enter username and password.');
            return;
        }
        this._busy = true;
        $MMO.http.post('/api/auth/register', { username: username, password: password })
            .then(function (data) {
                $MMO.token = data.token;
                self._cleanup();
                SceneManager.goto(Scene_CharacterSelect);
            })
            .catch(function (e) {
                self._busy = false;
                L2_Message.error(e.message);
            });
    };

    /**
     * 清理场景中的所有 HTML 输入框。
     * 从 DOM 中移除并清空输入框数组。
     * @returns {void}
     */
    Scene_Login.prototype._cleanup = function () {
        this._inputs.forEach(removeEl);
        this._inputs = [];
    };

    /**
     * 场景销毁时的处理。
     * 清除聚焦定时器并清理所有输入框。
     * @returns {void}
     */
    Scene_Login.prototype.terminate = function () {
        Scene_Base.prototype.terminate.call(this);
        if (this._focusTimer) { clearTimeout(this._focusTimer); this._focusTimer = null; }
        this._cleanup();
    };

    // ═══════════════════════════════════════════════════════════════
    //  Scene_CharacterSelect — 角色选择场景
    // ═══════════════════════════════════════════════════════════════
    // 展示当前账号下的所有角色卡片，支持：
    //   - 选择角色进入游戏
    //   - 跳转到角色创建界面
    //   - 删除已有角色（需密码确认）
    //   - 退出登录

    /**
     * 角色选择场景构造函数。
     * @constructor
     */
    function Scene_CharacterSelect() { this.initialize.apply(this, arguments); }
    Scene_CharacterSelect.prototype = Object.create(Scene_Base.prototype);
    Scene_CharacterSelect.prototype.constructor = Scene_CharacterSelect;

    /**
     * 初始化角色选择场景。
     * @returns {void}
     */
    Scene_CharacterSelect.prototype.initialize = function () {
        Scene_Base.prototype.initialize.call(this);
        /** @type {Array<Object>} 服务器返回的角色数据列表 */
        this._characters = [];
        /** @type {number} 当前选中的角色索引（-1 表示未选中） */
        this._selectedIndex = -1;
        /** @type {Array<L2_Base>} 角色卡片精灵数组 */
        this._cardWindows = [];
    };

    /**
     * 创建场景，加载背景并发起角色列表请求。
     * @returns {void}
     */
    Scene_CharacterSelect.prototype.create = function () {
        Scene_Base.prototype.create.call(this);
        createBackground(this);
        this._loadCharacters();
    };

    /**
     * 从服务器加载当前账号的角色列表。
     * 加载期间显示 L2_Loading 加载动画。
     * 加载完成后调用 _buildUI 构建界面。
     * @returns {void}
     */
    Scene_CharacterSelect.prototype._loadCharacters = function () {
        var self = this;
        this._loader = new L2_Loading(
            (Graphics.boxWidth - 120) / 2,
            (Graphics.boxHeight - 30) / 2,
            { text: 'Loading...' }
        );
        this.addChild(this._loader);

        $MMO.http.get('/api/characters')
            .then(function (data) {
                if (self._loader) { self.removeChild(self._loader); self._loader = null; }
                self._characters = data.characters || [];
                self._selectedIndex = self._characters.length > 0 ? 0 : -1;
                self._buildUI();
            })
            .catch(function (e) {
                if (self._loader) { self.removeChild(self._loader); self._loader = null; }
                L2_Message.error('Failed to load characters: ' + e.message);
                self._buildUI();
            });
    };

    /**
     * 构建角色选择界面的所有 UI 元素。
     * 包括：标题、角色信息面板（左侧）、角色卡片（居中）、
     * 进入游戏按钮、创建角色/删除角色/退出登录按钮（右下）。
     * @returns {void}
     */
    Scene_CharacterSelect.prototype._buildUI = function () {
        var self = this;
        var gw = Graphics.boxWidth;
        var gh = Graphics.boxHeight;

        // ── 标题文字 ──
        this.addChild(new L2_Typography((gw - 400) / 2, 10, 400, {
            text: 'SELECT CHARACTER', level: 'h1', align: 'center', color: L2_Theme.textGold
        }));

        // ── 角色信息面板（左侧，毛玻璃效果，高度自动计算） ──
        if (this._characters.length > 0) {
            var infoL = new PanelLayout(16, 60, 230, { title: 'Character Info' });
            infoL.row(22);  // 名称行
            infoL.row(28);  // 等级 + 职业行
            infoL.row(22);  // HP 血条行
            infoL.row(22);  // MP 蓝条行
            infoL.row(22);  // 经验值行
            this._infoPanel = createGlassPanel(this, 16, 60, 230, infoL.height(), 'Character Info');
            this._infoPanelBase = this._infoPanel;
            this._refreshInfoPanel();
        }

        // ── 角色卡片（居中排列） ──
        var CARD_W = 150, CARD_H = 130, CARD_GAP = 16;
        var numCards = this._characters.length;
        var totalW = numCards * CARD_W + Math.max(0, numCards - 1) * CARD_GAP;
        var startX = (gw - totalW) / 2;

        this._cardWindows = [];
        this._characters.forEach(function (ch, i) {
            var card = self._createCard(
                startX + i * (CARD_W + CARD_GAP), 280,
                CARD_W, CARD_H, ch, i === self._selectedIndex
            );
            self.addChild(card);
            self._cardWindows.push(card);
        });

        // ── 无角色时的提示信息 ──
        if (this._characters.length === 0) {
            this.addChild(new L2_Typography((gw - 360) / 2, gh / 2 - 30, 360, {
                text: 'No characters yet.', level: 'h2', align: 'center', color: L2_Theme.textGray
            }));
            this.addChild(new L2_Typography((gw - 360) / 2, gh / 2, 360, {
                text: 'Create one to begin your adventure!', align: 'center', color: L2_Theme.textDim
            }));
        }

        // ── 进入游戏按钮（居中） ──
        if (this._characters.length > 0) {
            this._enterBtn = new L2_Button((gw - 160) / 2, 430, 160, 36, 'Enter Game', {
                type: 'primary',
                onClick: function () { self._enterGame(); }
            });
            this.addChild(this._enterBtn);
        }

        // ── 右下角操作按钮组 ──
        var rbX = gw - 180, rbY = gh - 150;
        this.addChild(new L2_Button(rbX, rbY, 'Create Character', {
            onClick: function () { SceneManager.goto(Scene_CharacterCreate); }
        }));

        rbY += 40;
        this.addChild(new L2_Button(rbX, rbY, 'Delete Character', {
            type: 'danger',
            onClick: function () { self._deleteCharacter(); }
        }));

        rbY += 40;
        this.addChild(new L2_Button(rbX, rbY, 'Logout', {
            type: 'text',
            onClick: function () {
                $MMO.disconnect();
                $MMO.token = null;
                SceneManager.goto(Scene_Login);
            }
        }));
    };

    // ── 角色卡片工厂方法 ──

    /**
     * 创建一张角色卡片精灵。
     * 卡片显示角色头像、名称和等级/职业，支持鼠标悬停高亮和点击选中。
     * @param {number} x - 卡片左上角 X 坐标
     * @param {number} y - 卡片左上角 Y 坐标
     * @param {number} w - 卡片宽度
     * @param {number} h - 卡片高度
     * @param {Object} charData - 角色数据对象（包含 name, level, class_id, walk_name 等字段）
     * @param {boolean} selected - 是否为当前选中状态
     * @returns {L2_Base} 角色卡片精灵
     */
    Scene_CharacterSelect.prototype._createCard = function (x, y, w, h, charData, selected) {
        var self = this;
        var card = new L2_Base(x, y, w, h);
        card._charData = charData;
        card._selected = selected;
        card._hover = false;
        // 加载角色行走图（优先使用 walk_name，其次 face_name，兜底使用默认资源）
        card._faceBmp = ImageManager.loadCharacter(charData.walk_name || charData.face_name || 'actor01_0001');
        card._faceBmp.addLoadListener(function () { self._refreshCard(card); });

        var origUpdate = L2_Base.prototype.update;
        card.update = function () {
            origUpdate.call(this);
            if (!this.visible) return;
            // 检测鼠标悬停状态变化，触发重绘
            var wasHover = this._hover;
            this._hover = this.isInside(TouchInput.x, TouchInput.y);
            if (this._hover !== wasHover) self._refreshCard(this);
            // 点击选中卡片
            if (this._hover && TouchInput.isTriggered()) {
                self._selectCard(self._cardWindows.indexOf(this));
            }
        };

        this._refreshCard(card);
        return card;
    };

    /**
     * 重绘角色卡片的内容。
     * 根据选中/悬停状态绘制不同的背景色和边框色，
     * 绘制角色行走图头像（居中 56x56）、名称和等级/职业信息。
     * @param {L2_Base} card - 要重绘的角色卡片
     * @returns {void}
     */
    Scene_CharacterSelect.prototype._refreshCard = function (card) {
        var c = card.bmp();
        c.clear();
        var cw = card.cw(), ch = card.ch();
        var d = card._charData;

        // 根据选中/悬停状态设置背景色和边框色
        var bg = card._selected ? '#1A1A44' : L2_Theme.bgPanel;
        var border = card._selected ? L2_Theme.borderGold :
                     (card._hover ? L2_Theme.borderLight : L2_Theme.borderDark);
        L2_Theme.fillRoundRect(c, 0, 0, cw, ch, L2_Theme.cornerRadius, bg);
        L2_Theme.strokeRoundRect(c, 0, 0, cw, ch, L2_Theme.cornerRadius, border);

        // 角色头像（居中 56x56，从行走图中裁剪正面朝下中间帧）
        var aSize = 56, ax = (cw - aSize) / 2;
        if (card._faceBmp && card._faceBmp.isReady()) {
            var wn = d.walk_name || d.face_name || '';
            var isBig = wn.indexOf('$') === 0;
            var pw = card._faceBmp.width / (isBig ? 3 : 12);
            var ph = card._faceBmp.height / (isBig ? 4 : 8);
            var fi = d.walk_index || d.face_index || 0;
            var cx = (isBig ? 1 : (fi % 4) * 3 + 1) * pw;
            var cy = (isBig ? 0 : Math.floor(fi / 4) * 4) * ph;
            c.blt(card._faceBmp, cx, cy, pw, ph,
                ax, 10, aSize, aSize);
        }

        // 角色名称
        c.fontSize = L2_Theme.fontNormal;
        c.textColor = card._selected ? L2_Theme.textGold : L2_Theme.textWhite;
        c.drawText(d.name, 0, 70, cw, 18, 'center');

        // 等级和职业
        c.fontSize = L2_Theme.fontSmall;
        c.textColor = L2_Theme.textGray;
        var classText = 'Lv.' + d.level + '  ' + (d.class_name || 'Class ' + d.class_id);
        c.drawText(classText, 0, 90, cw, 16, 'center');
    };

    /**
     * 选中指定索引的角色卡片。
     * 取消前一个选中卡片的高亮，高亮新选中的卡片，并刷新信息面板。
     * @param {number} idx - 要选中的角色索引
     * @returns {void}
     */
    Scene_CharacterSelect.prototype._selectCard = function (idx) {
        if (idx < 0 || idx === this._selectedIndex) return;
        var old = this._cardWindows[this._selectedIndex];
        if (old) { old._selected = false; this._refreshCard(old); }
        this._selectedIndex = idx;
        var cur = this._cardWindows[idx];
        if (cur) { cur._selected = true; this._refreshCard(cur); }
        this._refreshInfoPanel();
    };

    /**
     * 刷新左侧角色信息面板的内容。
     * 根据当前选中的角色数据重绘名称、等级/职业、HP/MP 血蓝条和经验值。
     * @returns {void}
     */
    Scene_CharacterSelect.prototype._refreshInfoPanel = function () {
        var panel = this._infoPanelBase;
        if (!panel || this._selectedIndex < 0) return;
        var ch = this._characters[this._selectedIndex];
        if (!ch) return;
        var c = panel.bmp();
        c.clear();
        var cw = panel.cw(), contentH = panel.ch();

        // 重绘毛玻璃面板的边框和标题栏
        L2_Theme.strokeRoundRect(c, 0, 0, cw, contentH, L2_Theme.cornerRadius,
            'rgba(100,120,180,0.35)');
        L2_Theme.drawTitleBar(c, 0, 0, cw, L2_Theme.titleBarH, 'Character Info');

        var y = L2_Theme.titleBarH + 10;
        var barW = cw - 20;

        // 角色名称
        c.fontSize = L2_Theme.fontH3;
        c.textColor = L2_Theme.textGold;
        c.drawText(ch.name, 10, y, barW, 20, 'left');
        y += 22;

        // 等级 + 职业
        c.fontSize = L2_Theme.fontNormal;
        c.textColor = L2_Theme.textGray;
        c.drawText('Lv.' + ch.level + '  ' + (ch.class_name || 'Class ' + ch.class_id),
            10, y, barW, 18, 'left');
        y += 28;

        // HP 血条
        var maxHP = ch.max_hp || 100, hp = ch.hp || maxHP;
        L2_Theme.drawBar(c, 10, y, barW, 16,
            Math.min(hp / maxHP, 1), L2_Theme.hpBg, L2_Theme.hpFill);
        c.fontSize = L2_Theme.fontSmall;
        c.textColor = L2_Theme.textWhite;
        c.drawText('HP ' + hp + '/' + maxHP, 14, y, barW - 8, 16, 'left');
        y += 22;

        // MP 蓝条
        var maxMP = ch.max_mp || 50, mp = ch.mp || maxMP;
        L2_Theme.drawBar(c, 10, y, barW, 14,
            Math.min(mp / maxMP, 1), L2_Theme.mpBg, L2_Theme.mpFill);
        c.drawText('MP ' + mp + '/' + maxMP, 14, y, barW - 8, 14, 'left');
        y += 22;

        // 经验值
        c.fontSize = L2_Theme.fontSmall;
        c.textColor = L2_Theme.textGold;
        c.drawText('EXP: ' + (ch.exp || 0), 10, y, barW, 16, 'left');
    };

    /**
     * 使用选中的角色进入游戏。
     * 流程：断开旧连接 → 建立新 WebSocket 连接 → 发送 enter_map 消息 →
     * 停止标题 BGM → 切换到 Scene_Map。
     *
     * 必须先断开再重连，否则上一次会话的残留 WS 连接会导致
     * $MMO.connect() 跳过连接，_connected 事件不触发，
     * enter_map 永远不会发送，玩家将停留在空白地图上。
     * @returns {void}
     */
    Scene_CharacterSelect.prototype._enterGame = function () {
        if (this._selectedIndex < 0 || this._busy) return;
        this._busy = true;
        var ch = this._characters[this._selectedIndex];
        $MMO.charID = ch.id;
        $MMO.charName = ch.name;

        // 必须先断开旧连接，确保连接状态干净。
        // 不这样做的话，自动重连残留的旧 WS 会导致 connect() 判断为已连接而跳过，
        // _connected 回调不会触发，enter_map 消息不会发送，
        // 用户将停留在空白的 Scene_Map 上。
        $MMO.disconnect();

        var onConnected = function () {
            $MMO.off('_connected', onConnected);
            $MMO.send('enter_map', { char_id: ch.id });
        };
        $MMO.on('_connected', onConnected);
        $MMO.connect($MMO.token);
        // 进入游戏地图前停止标题 BGM
        AudioManager.fadeOutBgm(1);
        AudioManager.stopBgs();
        AudioManager.stopMe();
        SceneManager.goto(Scene_Map);
    };

    /**
     * 删除选中的角色。
     * 弹出确认对话框，要求用户输入密码进行二次验证。
     * 删除成功后断开 WS 连接、清除角色 ID，重新加载角色选择界面。
     * @returns {void}
     */
    Scene_CharacterSelect.prototype._deleteCharacter = function () {
        if (this._selectedIndex < 0) {
            L2_Message.warning('No character selected.');
            return;
        }
        var self = this;
        var ch = this._characters[this._selectedIndex];
        var dlg = new L2_Dialog({
            title: 'Delete Character',
            content: 'Delete "' + ch.name + '"?\nEnter password to confirm.\n\n',
            buttons: [
                {
                    text: 'Delete', type: 'danger',
                    onClick: function () {
                        var pw = self._delPwInput ? self._delPwInput.value : '';
                        if (!pw) { L2_Message.warning('Please enter password.'); return; }
                        dlg.close();
                        removeEl(self._delPwInput);
                        self._delPwInput = null;
                        $MMO.http.del('/api/characters/' + ch.id, { password: pw })
                            .then(function () {
                                // 断开可能残留的 WS 连接，清除角色 ID
                                // 防止自动重连干扰后续操作
                                $MMO.disconnect();
                                $MMO.charID = null;
                                SceneManager.goto(Scene_CharacterSelect);
                            })
                            .catch(function (e) { L2_Message.error(e.message); });
                    }
                },
                {
                    text: 'Cancel', type: 'default',
                    onClick: function () {
                        dlg.close();
                        removeEl(self._delPwInput);
                        self._delPwInput = null;
                    }
                }
            ],
            onClose: function () {
                removeEl(self._delPwInput);
                self._delPwInput = null;
            }
        });
        this.addChild(dlg);
        // 密码输入框：位置根据对话框实际布局计算
        var inputW = dlg.width - 60;
        var dx = dlg.x + (dlg.width - inputW) / 2;
        var dy = dlg.y + dlg._titleH + 8 + 2 * 20 + 8; // 标题栏 + 2行文本 + 间距之后
        self._delPwInput = makeL2Input('password', 'Enter password', dx, dy, inputW, 28);
        self._delPwInput.addEventListener('keydown', function (e) {
            if (e.keyCode === 13) { e.stopPropagation(); }
        });
        this._delFocusTimer = setTimeout(function () {
            if (self._delPwInput) {
                self._delPwInput.style.display = 'block';
                self._delPwInput.style.visibility = 'visible';
                self._delPwInput.focus();
            }
        }, 100);
    };

    /**
     * 场景停止时的处理。
     * 隐藏删除角色对话框中的密码输入框。
     * @returns {void}
     */
    Scene_CharacterSelect.prototype.stop = function () {
        // 场景停止时隐藏删除密码输入框
        if (this._delPwInput) {
            this._delPwInput.style.display = 'none';
        }
        Scene_Base.prototype.stop.call(this);
    };

    /**
     * 场景销毁时的处理。
     * 清除聚焦定时器，移除密码输入框。
     * @returns {void}
     */
    Scene_CharacterSelect.prototype.terminate = function () {
        Scene_Base.prototype.terminate.call(this);
        if (this._delFocusTimer) { clearTimeout(this._delFocusTimer); this._delFocusTimer = null; }
        if (this._delPwInput) { removeEl(this._delPwInput); this._delPwInput = null; }
    };

    // ═══════════════════════════════════════════════════════════════
    //  Scene_CharacterCreate — 角色创建场景
    // ═══════════════════════════════════════════════════════════════
    // 用户输入角色名称，选择职业和外观（行走图），创建新角色。
    // 创建成功后自动返回角色选择界面。

    /**
     * 角色创建场景构造函数。
     * @constructor
     */
    function Scene_CharacterCreate() { this.initialize.apply(this, arguments); }
    Scene_CharacterCreate.prototype = Object.create(Scene_Base.prototype);
    Scene_CharacterCreate.prototype.constructor = Scene_CharacterCreate;

    /**
     * 初始化角色创建场景。
     * @returns {void}
     */
    Scene_CharacterCreate.prototype.initialize = function () {
        Scene_Base.prototype.initialize.call(this);
        /** @type {HTMLInputElement[]} 场景中的所有 HTML 输入框 */
        this._inputs = [];
        /** @type {number} 当前选中的外观预设索引 */
        this._selectedFace = 0;
        /** @type {Array<L2_Base>} 外观选择按钮数组 */
        this._faceButtons = [];
        /** @type {Object<string, Bitmap>} 行走图位图缓存（以资源名为键） */
        this._faceBitmaps = {};
        /** @type {boolean} 是否正在处理请求（防止重复提交） */
        this._busy = false;
    };

    /**
     * 创建角色创建场景的所有 UI 元素。
     * 包括：背景、标题文字、毛玻璃面板、名称输入框、职业下拉选择、
     * 外观选择网格（4列x2行）、预览头像、创建/返回按钮。
     * @returns {void}
     */
    Scene_CharacterCreate.prototype.create = function () {
        Scene_Base.prototype.create.call(this);
        createBackground(this);

        var gw = Graphics.boxWidth;
        var pw = 450;
        var px = (gw - pw) / 2, py = 80;
        var faceSize = 54, faceGap = 8;
        var faceRows = 2;
        var faceGridH = faceRows * faceSize + (faceRows - 1) * faceGap;

        // ── 布局：自动计算面板高度 ──
        var L = new PanelLayout(px, py, pw, { title: 'New Character' });
        var nameY   = L.row(44);
        var classY  = L.row(44);
        var faceLabelY = L.row(22);   // "外观"标签行
        var faceGridY  = L.row(faceGridH); // 外观选择网格行
        var btnY    = L.row(44);      // 按钮行
        var ph = L.height();

        // ── 标题文字 ──
        this.addChild(new L2_Typography((gw - 400) / 2, 24, 400, {
            text: 'CREATE CHARACTER', level: 'h1', align: 'center', color: L2_Theme.textGold
        }));

        // ── 毛玻璃面板（高度由布局自动计算） ──
        this._panel = createGlassPanel(this, px, py, pw, ph, 'New Character');

        // 第一行：角色名称
        this.addChild(new L2_Typography(L.cx() + 4, nameY + 4, 80, {
            text: 'Name', color: L2_Theme.textGray
        }));
        this._nameInput = makeL2Input('text', 'Character Name', L.ix(), nameY, L.iw(), 28);
        this._inputs.push(this._nameInput);

        // 第二行：职业选择
        this.addChild(new L2_Typography(L.cx() + 4, classY + 4, 80, {
            text: 'Class', color: L2_Theme.textGray
        }));
        this._classSelect = new L2_Select(L.ix(), classY, L.iw(), {
            options: CLASS_OPTIONS,
            selected: 0,
            placeholder: 'Select a class'
        });

        // 第三行：外观标签
        this.addChild(new L2_Typography(L.cx() + 4, faceLabelY + 2, 80, {
            text: 'Face', color: L2_Theme.textGray
        }));

        // 外观选择网格：4 列 x 2 行
        var gridX = L.ix();
        var self = this;
        FACE_PRESETS.forEach(function (preset, i) {
            var col = i % 4, row = Math.floor(i / 4);
            var fx = gridX + col * (faceSize + faceGap);
            var fy = faceGridY + row * (faceSize + faceGap);
            var faceBtn = self._createFaceButton(fx, fy, faceSize, preset, i);
            self.addChild(faceBtn);
            self._faceButtons.push(faceBtn);
        });

        // 职业下拉框置于最上层（z-order 高于外观网格，避免下拉菜单被遮挡）
        this.addChild(this._classSelect);

        // 预览头像（位于网格右侧）
        var previewX = gridX + 4 * (faceSize + faceGap) + 12;
        this._previewBg = new L2_Base(previewX, faceGridY, 80, 80);
        this.addChild(this._previewBg);
        this._refreshPreview();

        // ── 按钮（位于面板内部） ──
        this.addChild(new L2_Button(px + 80, btnY, 'Create', {
            type: 'primary',
            onClick: function () { self._doCreate(); }
        }));
        this.addChild(new L2_Button(px + pw - 160, btnY, 'Back', {
            onClick: function () {
                self._cleanup();
                SceneManager.goto(Scene_CharacterSelect);
            }
        }));

        // ── 键盘快捷键：名称框按回车直接创建角色 ──
        this._nameInput.addEventListener('keydown', function (e) {
            if (e.keyCode === 13) { e.stopPropagation(); self._doCreate(); }
        });
    };

    /**
     * 场景启动时的处理。
     * 确保所有输入框可见且可交互，延迟 200ms 后自动聚焦名称输入框。
     * @returns {void}
     */
    Scene_CharacterCreate.prototype.start = function () {
        Scene_Base.prototype.start.call(this);
        // 确保输入框可见且可获取焦点
        this._inputs.forEach(function (el) {
            el.style.display = 'block';
            el.style.visibility = 'visible';
            el.style.opacity = '1';
        });
        var self = this;
        this._focusTimer = setTimeout(function () {
            if (self._nameInput) self._nameInput.focus();
        }, 200);
    };

    /**
     * 场景停止时的处理。
     * 隐藏所有 HTML 输入框。
     * @returns {void}
     */
    Scene_CharacterCreate.prototype.stop = function () {
        // 场景停止时隐藏输入框
        this._inputs.forEach(function (el) {
            el.style.display = 'none';
        });
        Scene_Base.prototype.stop.call(this);
    };

    /**
     * 创建一个外观选择按钮。
     * 按钮显示角色行走图的正面缩略图，支持悬停高亮和点击选中。
     * @param {number} x - 按钮左上角 X 坐标
     * @param {number} y - 按钮左上角 Y 坐标
     * @param {number} size - 按钮尺寸（正方形，边长）
     * @param {Object} preset - 外观预设数据（包含 faceName 和 faceIndex）
     * @param {number} index - 该预设在 FACE_PRESETS 中的索引
     * @returns {L2_Base} 外观选择按钮精灵
     */
    Scene_CharacterCreate.prototype._createFaceButton = function (x, y, size, preset, index) {
        var self = this;
        var btn = new L2_Base(x, y, size + 4, size + 4);
        btn._preset = preset;
        btn._index = index;
        btn._hover = false;
        btn.standardPadding = function () { return 2; };

        // 加载行走图资源（同一资源名共享缓存，避免重复加载）
        var key = preset.faceName;
        if (!this._faceBitmaps[key]) {
            this._faceBitmaps[key] = ImageManager.loadCharacter(key);
        }
        var charBmp = this._faceBitmaps[key];
        charBmp.addLoadListener(function () { self._refreshFaceBtn(btn); });

        var origUpdate = L2_Base.prototype.update;
        btn.update = function () {
            origUpdate.call(this);
            if (!this.visible) return;
            // 检测鼠标悬停状态变化
            var wasHover = this._hover;
            this._hover = this.isInside(TouchInput.x, TouchInput.y);
            if (this._hover !== wasHover) self._refreshFaceBtn(this);
            // 点击选中外观
            if (this._hover && TouchInput.isTriggered()) {
                self._selectFace(this._index);
            }
        };

        this._refreshFaceBtn(btn);
        return btn;
    };

    /**
     * 重绘外观选择按钮的内容。
     * 根据选中/悬停状态绘制不同的背景色和边框色，
     * 从行走图中裁剪正面朝下中间帧并缩放绘制到按钮上。
     * @param {L2_Base} btn - 要重绘的外观按钮
     * @returns {void}
     */
    Scene_CharacterCreate.prototype._refreshFaceBtn = function (btn) {
        var c = btn.bmp();
        c.clear();
        var s = btn.cw();
        var sel = btn._index === this._selectedFace;
        var border = sel ? L2_Theme.borderGold :
                     (btn._hover ? L2_Theme.borderLight : L2_Theme.borderDark);
        var bg = sel ? '#1A1A44' : L2_Theme.bgPanel;

        L2_Theme.fillRoundRect(c, 0, 0, s, s, 2, bg);
        L2_Theme.strokeRoundRect(c, 0, 0, s, s, 2, border);

        var key = btn._preset.faceName;
        var charBmp = this._faceBitmaps[key];
        if (charBmp && charBmp.isReady()) {
            // 从行走图中裁剪角色正面（朝下）中间帧
            var isBig = key.indexOf('$') === 0;
            var pw = charBmp.width / (isBig ? 3 : 12);
            var ph = charBmp.height / (isBig ? 4 : 8);
            var fi = btn._preset.faceIndex;
            var cx = (isBig ? 1 : (fi % 4) * 3 + 1) * pw;
            var cy = (isBig ? 0 : Math.floor(fi / 4) * 4) * ph;
            var pad = 3;
            c.blt(charBmp, cx, cy, pw, ph,
                pad, pad, s - pad * 2, s - pad * 2);
        }
    };

    /**
     * 选中指定索引的外观预设。
     * 刷新旧选中和新选中按钮的外观，并更新预览头像。
     * @param {number} idx - 要选中的外观预设索引
     * @returns {void}
     */
    Scene_CharacterCreate.prototype._selectFace = function (idx) {
        var old = this._selectedFace;
        this._selectedFace = idx;
        var self = this;
        this._faceButtons.forEach(function (btn) {
            if (btn._index === old || btn._index === idx) {
                self._refreshFaceBtn(btn);
            }
        });
        this._refreshPreview();
    };

    /**
     * 刷新预览头像区域。
     * 在右侧 80x80 的预览框中绘制当前选中的外观行走图。
     * @returns {void}
     */
    Scene_CharacterCreate.prototype._refreshPreview = function () {
        if (!this._previewBg) return;
        var c = this._previewBg.bmp();
        c.clear();
        var s = this._previewBg.cw();
        L2_Theme.fillRoundRect(c, 0, 0, s, s, L2_Theme.cornerRadius, L2_Theme.bgPanel);
        L2_Theme.strokeRoundRect(c, 0, 0, s, s, L2_Theme.cornerRadius, L2_Theme.borderGold);

        var preset = FACE_PRESETS[this._selectedFace];
        if (!preset) return;
        var key = preset.faceName;
        var charBmp = this._faceBitmaps[key];
        if (charBmp && charBmp.isReady()) {
            var isBig = key.indexOf('$') === 0;
            var pw = charBmp.width / (isBig ? 3 : 12);
            var ph = charBmp.height / (isBig ? 4 : 8);
            var fi = preset.faceIndex;
            var cx = (isBig ? 1 : (fi % 4) * 3 + 1) * pw;
            var cy = (isBig ? 0 : Math.floor(fi / 4) * 4) * ph;
            var pad = 4;
            c.blt(charBmp, cx, cy, pw, ph,
                pad, pad, s - pad * 2, s - pad * 2);
        }
    };

    /**
     * 执行角色创建操作。
     * 校验名称非空后，收集职业和外观选择数据，向服务器发送创建请求。
     * 创建成功后清理输入框并返回角色选择界面。
     * @returns {void}
     */
    Scene_CharacterCreate.prototype._doCreate = function () {
        if (this._busy) return;
        var self = this;
        var name = this._nameInput.value.trim();
        if (!name) {
            L2_Message.warning('Please enter a character name.');
            return;
        }
        var classOpt = this._classSelect.getSelected();
        var classId = classOpt ? classOpt.value : 1;
        var face = FACE_PRESETS[this._selectedFace] || FACE_PRESETS[0];

        this._busy = true;
        $MMO.http.post('/api/characters', {
            name: name,
            class_id: classId,
            walk_name: face.faceName,
            walk_index: face.faceIndex,
            face_name: face.faceName,
            face_index: face.faceIndex
        }).then(function () {
            self._cleanup();
            SceneManager.goto(Scene_CharacterSelect);
        }).catch(function (e) {
            self._busy = false;
            L2_Message.error(e.message);
        });
    };

    /**
     * 清理场景中的所有 HTML 输入框。
     * @returns {void}
     */
    Scene_CharacterCreate.prototype._cleanup = function () {
        this._inputs.forEach(removeEl);
        this._inputs = [];
    };

    /**
     * 场景销毁时的处理。
     * 清除聚焦定时器并清理所有输入框。
     * @returns {void}
     */
    Scene_CharacterCreate.prototype.terminate = function () {
        Scene_Base.prototype.terminate.call(this);
        if (this._focusTimer) { clearTimeout(this._focusTimer); this._focusTimer = null; }
        this._cleanup();
    };

    // ═══════════════════════════════════════════════════════════════
    //  覆盖标题画面 → 跳转至 Scene_Login
    // ═══════════════════════════════════════════════════════════════
    // 替换 Scene_Title 的 start 方法，使游戏启动时：
    //   1. 播放标题音乐
    //   2. 断开可能残留的 WS 连接
    //   3. 清除认证状态（token、charID）
    //   4. 立即跳转到登录界面

    /**
     * 覆盖 Scene_Title.prototype.start。
     * 播放标题音乐后立即跳转至登录场景，跳过默认标题画面。
     * 同时清除所有残留的连接和认证状态。
     * @returns {void}
     */
    Scene_Title.prototype.start = function () {
        Scene_Base.prototype.start.call(this);
        this.playTitleMusic();
        $MMO.disconnect();
        $MMO.token = null;
        $MMO.charID = null;
        SceneManager.goto(Scene_Login);
    };

    // ═══════════════════════════════════════════════════════════════
    //  导出场景到全局作用域
    // ═══════════════════════════════════════════════════════════════

    /** @global Scene_Login 登录场景类 */
    window.Scene_Login = Scene_Login;
    /** @global Scene_CharacterSelect 角色选择场景类 */
    window.Scene_CharacterSelect = Scene_CharacterSelect;
    /** @global Scene_CharacterCreate 角色创建场景类 */
    window.Scene_CharacterCreate = Scene_CharacterCreate;

})();
