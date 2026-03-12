/*:
 * @plugindesc v2.0.0 MMO 插件加载器 - 从服务器远程获取所有 mmo-*.js 插件。
 * @author MMO Framework
 *
 * @param ServerURL
 * @desc WebSocket 服务器地址（例如 ws://localhost:8080）
 * @default ws://localhost:8080
 *
 * @param Debug
 * @desc 是否启用调试模式（true/false）
 * @default false
 *
 * @param ReconnectMax
 * @desc 最大重连尝试次数
 * @default 10
 *
 * @help
 * 这是唯一需要在 js/plugins.js 中注册的 MMO 插件。
 * 其余所有 mmo-*.js 插件在启动时通过 /plugins/ 端点从服务器获取，
 * 实现远程热更新而无需修改客户端文件。
 */

(function () {
    'use strict';

    /**
     * 插件加载顺序表。
     * 按依赖关系排列：core → ui → 窗口 → 认证 → 游戏功能 → HUD/辅助。
     * 所有插件通过同步 XHR 按此顺序加载，确保依赖链正确。
     */
    var LOAD_ORDER = [
        'mmo-core.js',
        'mmo-ui.js',
        'mmo-game-window.js',
        'mmo-auth.js',
        'mmo-other-players.js',
        'mmo-npc.js',
        'mmo-battle.js',
        'mmo-realtime-battle.js',
        'mmo-battle-core.js',
        'mmo-hud.js',
        'mmo-skill-bar.js',
        'mmo-inventory.js',
        'mmo-chat.js',
        'mmo-party.js',
        'mmo-social.js',
        'mmo-trade.js',
        'mmo-battle-puppet.js',
        'mmo-debug.js'
    ];

    /** 从 RMMV 插件管理器读取配置参数。 */
    var parameters = PluginManager.parameters('mmo-loader');

    /**
     * 全局 MMO 配置对象。
     * 从 RMMV 插件参数构建，供所有后续插件通过 window.MMO_CONFIG 访问。
     * @property {string} serverUrl - WebSocket 服务器地址
     * @property {boolean} debug - 是否启用调试日志
     * @property {number} reconnectMax - 最大重连尝试次数
     */
    window.MMO_CONFIG = {
        serverUrl: parameters['ServerURL'] || 'ws://localhost:8080',
        debug: (parameters['Debug'] || 'false').toLowerCase() === 'true',
        reconnectMax: parseInt(parameters['ReconnectMax'] || '10', 10)
    };

    /** 将 WebSocket 地址转换为 HTTP 基地址，用于 REST API 和插件下载。 */
    var httpBase = MMO_CONFIG.serverUrl.replace(/^ws/, 'http');

    if (MMO_CONFIG.debug) console.log('[MMO] 配置就绪。服务器: ' + MMO_CONFIG.serverUrl +
                ' 调试模式: ' + MMO_CONFIG.debug);

    // ═══════════════════════════════════════════════════════════
    //  远程插件加载器
    //  通过同步 XHR 从服务器逐个获取插件 JS 文件，
    //  使用间接 eval 在全局作用域中执行。
    //  同步加载确保正确的加载顺序，所有插件在 Scene_Boot 启动前就绪。
    // ═══════════════════════════════════════════════════════════
    var loaded = 0;
    var failed = [];
    for (var i = 0; i < LOAD_ORDER.length; i++) {
        var filename = LOAD_ORDER[i];
        /** 添加时间戳参数防止浏览器缓存旧版本。 */
        var url = httpBase + '/plugins/' + filename + '?_t=' + Date.now();
        try {
            var xhr = new XMLHttpRequest();
            xhr.open('GET', url, false); // 同步请求
            xhr.send();
            if (xhr.status === 200) {
                // 间接 eval (0, eval)() 在全局作用域执行，等同于 <script> 标签行为。
                (0, eval)(xhr.responseText);
                loaded++;
                if (MMO_CONFIG.debug) {
                    console.log('[MMO] 已加载: ' + filename);
                }
            } else {
                failed.push(filename + ' (HTTP ' + xhr.status + ')');
                console.error('[MMO] 加载失败 ' + filename + ': HTTP ' + xhr.status);
            }
        } catch (e) {
            failed.push(filename + ' (' + e.message + ')');
            console.error('[MMO] 加载出错 ' + filename + ':', e.message);
        }
    }

    if (MMO_CONFIG.debug || failed.length) console.log('[MMO] 远程加载完成: ' + loaded + '/' + LOAD_ORDER.length +
                (failed.length ? ' | 失败: ' + failed.join(', ') : ''));

    // ═══════════════════════════════════════════════════════════
    //  客户端配置管理器
    //  从 data/MMOClientConfig.json 加载客户端UI配置。
    //  若文件不存在或解析失败，使用内置默认值。
    //  配置通过 window.MMO_CLIENT_CONFIG 供所有插件访问。
    // ═══════════════════════════════════════════════════════════

    /**
     * 客户端配置默认值。
     * 当 data/MMOClientConfig.json 不存在或缺少对应字段时使用。
     */
    var CLIENT_CONFIG_DEFAULTS = {
        hud: {
            statusBar: false,
            minimap: false,
            questTracker: false
        },
        skillBar: {
            enabled: false,
            slotCount: 12
        },
        inventory: {
            enabled: false
        },
        chat: {
            enabled: false,
            maxMessages: 100,
            channels: ['world', 'party', 'guild', 'battle', 'system', 'private']
        },
        party: {
            enabled: false,
            inviteTimeoutSeconds: 30
        },
        social: {
            enabled: false,
            notificationTimeoutMs: 5000
        },
        trade: {
            enabled: false,
            requestTimeoutSeconds: 15
        },
        escMenu: false
    };

    /**
     * 深度合并两个对象，src 的值覆盖 dst，src 中缺失的字段保留 dst 的默认值。
     * @param {Object} dst - 默认值对象（不会被修改）
     * @param {Object} src - 用户配置对象
     * @returns {Object} 合并后的新对象
     */
    function mergeConfig(dst, src) {
        var result = {};
        for (var k in dst) {
            if (Object.prototype.hasOwnProperty.call(dst, k)) {
                if (src && Object.prototype.hasOwnProperty.call(src, k) &&
                    typeof dst[k] === 'object' && dst[k] !== null &&
                    typeof src[k] === 'object' && src[k] !== null) {
                    result[k] = mergeConfig(dst[k], src[k]);
                } else if (src && Object.prototype.hasOwnProperty.call(src, k)) {
                    result[k] = src[k];
                } else {
                    result[k] = dst[k];
                }
            }
        }
        return result;
    }

    /** 加载客户端配置文件，合并默认值后写入 window.MMO_CLIENT_CONFIG。 */
    (function loadClientConfig() {
        var userConfig = null;
        try {
            var cfgXhr = new XMLHttpRequest();
            cfgXhr.open('GET', 'data/MMOClientConfig.json', false); // 同步请求本地文件
            cfgXhr.send();
            if (cfgXhr.status === 200 || cfgXhr.status === 0) { // 0 = 本地文件协议
                userConfig = JSON.parse(cfgXhr.responseText);
                if (MMO_CONFIG.debug) console.log('[MMO] 已加载客户端配置: data/MMOClientConfig.json');
            } else {
                if (MMO_CONFIG.debug) console.log('[MMO] 未找到 data/MMOClientConfig.json，使用默认配置');
            }
        } catch (e) {
            if (MMO_CONFIG.debug) console.log('[MMO] 客户端配置加载失败，使用默认配置:', e.message);
        }
        window.MMO_CLIENT_CONFIG = mergeConfig(CLIENT_CONFIG_DEFAULTS, userConfig || {});
        if (MMO_CONFIG.debug) console.log('[MMO] 客户端配置:', JSON.stringify(window.MMO_CLIENT_CONFIG));
    })();

    // ═══════════════════════════════════════════════════════════
    //  自动检测 TemplateEvent.js 并加载同步钩子
    //  检测方式：
    //  1. 查找 $gameSelfSwitches.getVariableValue 特征方法
    //  2. 检查 $plugins 列表中是否包含 TemplateEvent
    //  检测在脚本执行阶段进行，此时 $gameSelfSwitches 可能尚未初始化，
    //  因此优先使用 $plugins 列表检测。
    // ═══════════════════════════════════════════════════════════
    var hasTemplateEvent = false;
    try {
        // 方式一：检测 TemplateEvent.js 特征方法
        if (typeof $gameSelfSwitches !== 'undefined' &&
            typeof $gameSelfSwitches.getVariableValue === 'function') {
            hasTemplateEvent = true;
        }
        // 方式二：检查插件列表（更可靠，不依赖运行时对象）
        if ($plugins && $plugins.some(function(p) {
            return p && p.name && p.name.toLowerCase().indexOf('templateevent') >= 0;
        })) {
            hasTemplateEvent = true;
        }
    } catch (e) {
        // 检测阶段的错误不影响主流程
    }

    if (hasTemplateEvent) {
        if (MMO_CONFIG.debug) console.log('[MMO] 检测到 TemplateEvent.js，加载同步钩子...');
        try {
            var hookUrl = httpBase + '/plugins/mmo-template-event-hook.js?_t=' + Date.now();
            var hookXhr = new XMLHttpRequest();
            hookXhr.open('GET', hookUrl, false); // 同步请求
            hookXhr.send();
            if (hookXhr.status === 200) {
                (0, eval)(hookXhr.responseText);
                if (MMO_CONFIG.debug) console.log('[MMO] TemplateEvent 同步钩子加载成功');
            } else {
                console.warn('[MMO] TemplateEvent 钩子加载失败: HTTP ' + hookXhr.status);
            }
        } catch (e) {
            console.error('[MMO] TemplateEvent 钩子加载出错:', e.message);
        }
    }

    // ═══════════════════════════════════════════════════════════
    //  客户端错误上报
    //  捕获未处理的异常和 Promise 拒绝，通过 REST API 发送到服务器，
    //  便于远程监控客户端运行状况。
    // ═══════════════════════════════════════════════════════════

    /**
     * 向服务器上报客户端错误。
     * @param {string} message - 错误消息
     * @param {string} source - 错误源文件
     * @param {number} line - 行号
     * @param {number} col - 列号
     * @param {string} stack - 堆栈信息
     */
    function reportError(message, source, line, col, stack) {
        try {
            var xhr = new XMLHttpRequest();
            xhr.open('POST', httpBase + '/api/client-error');
            xhr.setRequestHeader('Content-Type', 'application/json');
            xhr.send(JSON.stringify({
                message: String(message || ''),
                source: String(source || ''),
                line: line || 0,
                col: col || 0,
                stack: String(stack || ''),
                ua: navigator.userAgent || ''
            }));
        } catch (e) { /* 发送失败时静默忽略 */ }
    }

    /** 全局未捕获异常处理器。 */
    window.onerror = function (message, source, line, col, error) {
        var stack = (error && error.stack) ? error.stack : '';
        console.error('[MMO] 未捕获异常:', message, 'at', source + ':' + line + ':' + col);
        reportError(message, source, line, col, stack);
    };

    /** 全局未处理 Promise 拒绝处理器。 */
    window.addEventListener('unhandledrejection', function (event) {
        var reason = event.reason || {};
        var msg = reason.message || String(reason);
        var stack = reason.stack || '';
        console.error('[MMO] 未处理的 Promise 拒绝:', msg);
        reportError('UnhandledRejection: ' + msg, '', 0, 0, stack);
    });

})();
