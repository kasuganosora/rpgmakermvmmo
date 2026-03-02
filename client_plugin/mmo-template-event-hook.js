//=============================================================================
// MMO TemplateEvent 同步钩子
// 拦截 TemplateEvent.js 的自变量修改，批量同步到服务器实现玩家隔离状态。
// 索引 13-17 为 RandomPos 保留（x, y, dir, day, seed）。
//=============================================================================

(function () {
    'use strict';

    // ═══════════════════════════════════════════════════════════
    //  配置
    // ═══════════════════════════════════════════════════════════
    /** @type {Object} 插件配置常量。 */
    var CONFIG = {
        DEBUG: false,
        /** 自变量批量同步间隔（毫秒）。 */
        SYNC_INTERVAL: 5000,
        /** 每批次最大变更数量。 */
        BATCH_SIZE: 10
    };

    // ═══════════════════════════════════════════════════════════
    //  状态
    // ═══════════════════════════════════════════════════════════
    /** @type {boolean} 服务器是否可用。 */
    var _isServerAvailable = false;
    /** @type {boolean} 是否检测到 TemplateEvent.js。 */
    var _templateEventDetected = false;
    /** @type {Object} 待同步变更队列，键为 mapId_eventId_index。 */
    var _pendingChanges = {};
    /** @type {number|null} 批量刷新定时器 ID。 */
    var _flushTimer = null;

    // ═══════════════════════════════════════════════════════════
    //  调试日志
    // ═══════════════════════════════════════════════════════════

    /**
     * 输出调试日志（仅在 CONFIG.DEBUG 开启时）。
     */
    function debugLog() {
        if (CONFIG.DEBUG) {
            var args = ['[MMO-TemplateEvent-Hook]'];
            for (var i = 0; i < arguments.length; i++) args.push(arguments[i]);
            console.log.apply(console, args);
        }
    }

    // ═══════════════════════════════════════════════════════════
    //  服务器可用性检测
    //  通过 $MMO.isConnected() 判断 WebSocket 是否已连接。
    // ═══════════════════════════════════════════════════════════

    /**
     * 检查服务器是否可用。
     * @returns {boolean}
     */
    function checkServerAvailability() {
        _isServerAvailable = typeof $MMO !== 'undefined' &&
            $MMO.isConnected && $MMO.isConnected();
        return _isServerAvailable;
    }

    // ═══════════════════════════════════════════════════════════
    //  变更批量队列与同步
    //  自变量修改先入队，按 SYNC_INTERVAL 周期批量发送到服务器。
    //  相同键（mapId_eventId_index）的重复修改会被合并。
    // ═══════════════════════════════════════════════════════════

    /**
     * 将自变量变更加入待同步队列。
     * 首次入队时启动批量刷新定时器。
     * @param {number} mapId - 地图 ID
     * @param {number} eventId - 事件 ID
     * @param {number} index - 自变量索引
     * @param {*} value - 新值
     */
    function queueChange(mapId, eventId, index, value) {
        if (!checkServerAvailability()) return;

        var key = mapId + '_' + eventId + '_' + index;
        _pendingChanges[key] = { mapId: mapId, eventId: eventId, index: index, value: value };

        if (!_flushTimer) {
            _flushTimer = setTimeout(flushChanges, CONFIG.SYNC_INTERVAL);
        }
    }

    /**
     * 将队列中的所有变更批量发送到服务器。
     * 每批最多 BATCH_SIZE 条，超出时分多批发送。
     * 服务器不可用时丢弃所有待发送变更。
     */
    function flushChanges() {
        _flushTimer = null;

        if (!checkServerAvailability()) {
            _pendingChanges = {};
            return;
        }

        var keys = Object.keys(_pendingChanges);
        var changes = [];
        for (var i = 0; i < keys.length; i++) {
            changes.push(_pendingChanges[keys[i]]);
        }
        _pendingChanges = {};

        if (changes.length === 0) return;

        // 分批发送到服务器。
        for (var j = 0; j < changes.length; j += CONFIG.BATCH_SIZE) {
            var batch = changes.slice(j, j + CONFIG.BATCH_SIZE);
            debugLog('发送批次:', batch);
            $MMO.send('self_var_set_batch', { changes: batch });
        }
    }

    // ═══════════════════════════════════════════════════════════
    //  钩子 Game_SelfSwitches.setValue
    //  拦截数值型自变量修改（索引 >= 13，为 TemplateEvent 保留）。
    //  始终调用原始方法以保持客户端行为一致。
    // ═══════════════════════════════════════════════════════════

    /**
     * 覆写 Game_SelfSwitches.setValue 以拦截自变量修改。
     * 仅对索引 >= 13 的数值型键（TemplateEvent 自变量）进行同步。
     * 始终调用原始方法以保持本地状态正确。
     */
    function hookGameSelfSwitches() {
        var originalSetValue = Game_SelfSwitches.prototype.setValue;

        Game_SelfSwitches.prototype.setValue = function (key, value) {
            // key 格式: [mapId, eventId, switchChar/index]
            // TemplateEvent 自变量使用数值索引（>= 13）。
            if (Array.isArray(key) && key.length >= 3) {
                var ch = key[2];
                if (typeof ch === 'number' && ch >= 13) {
                    debugLog('拦截自变量修改:', { mapId: key[0], eventId: key[1], index: ch, value: value });
                    queueChange(key[0], key[1], ch, value);
                }
            }

            // 始终调用原始方法。
            return originalSetValue.call(this, key, value);
        };

        debugLog('Game_SelfSwitches.setValue 钩子已安装');
    }

    // ═══════════════════════════════════════════════════════════
    //  钩子 TemplateEvent.js 专有方法（如存在）
    //  直接拦截 TemplateEvent.setSelfVariable 以更精确地捕获变更。
    // ═══════════════════════════════════════════════════════════

    /**
     * 检测并钩住 TemplateEvent.js 的 setSelfVariable 方法。
     * @returns {boolean} 是否成功检测到 TemplateEvent.js
     */
    function hookTemplateEvent() {
        if (typeof TemplateEvent === 'undefined') {
            debugLog('未检测到 TemplateEvent');
            return false;
        }

        _templateEventDetected = true;
        debugLog('已检测到 TemplateEvent.js');

        // 钩住 setSelfVariable（如存在）。
        if (TemplateEvent.setSelfVariable) {
            var originalSetSelfVariable = TemplateEvent.setSelfVariable;
            TemplateEvent.setSelfVariable = function (eventId, mapId, index, value) {
                debugLog('TemplateEvent.setSelfVariable 拦截:', { eventId: eventId, mapId: mapId, index: index, value: value });
                queueChange(mapId, eventId, index, value);
                return originalSetSelfVariable.apply(this, arguments);
            };
        }

        return true;
    }

    // ═══════════════════════════════════════════════════════════
    //  初始化
    //  等待 $MMO 就绪后安装所有钩子。
    //  30 秒超时后停止等待，防止未连接时无限轮询。
    // ═══════════════════════════════════════════════════════════

    /**
     * 初始化插件。等待 $MMO 可用后安装钩子。
     */
    function initialize() {
        debugLog('初始化中...');

        // 等待 $MMO 就绪。
        var checkInterval = setInterval(function () {
            if (typeof $MMO !== 'undefined' && $MMO.isConnected) {
                clearInterval(checkInterval);

                // 检查服务器可用性。
                checkServerAvailability();
                debugLog('服务器可用:', _isServerAvailable);

                // 安装 RMMV 钩子。
                if (typeof Game_SelfSwitches !== 'undefined') {
                    hookGameSelfSwitches();
                } else {
                    console.warn('[MMO-TemplateEvent-Hook] Game_SelfSwitches 不存在');
                }

                // 检测并钩住 TemplateEvent.js。
                hookTemplateEvent();

                // 场景切换时刷新待发送变更（防止切换地图时丢失）。
                var originalTerminate = Scene_Map.prototype.terminate;
                Scene_Map.prototype.terminate = function () {
                    flushChanges();
                    return originalTerminate.call(this);
                };

                debugLog('初始化完成');
            }
        }, 100);

        // 30 秒超时后停止等待。
        setTimeout(function () {
            clearInterval(checkInterval);
        }, 30000);
    }

    // ═══════════════════════════════════════════════════════════
    //  公开 API
    // ═══════════════════════════════════════════════════════════
    window.MMO_TEMPLATE_EVENT_HOOK = {
        /** 检查服务器是否可用。 */
        isServerAvailable: function () { return _isServerAvailable; },
        /** 检查是否检测到 TemplateEvent.js。 */
        isTemplateEventDetected: function () { return _templateEventDetected; },
        /** 手动触发变更刷新。 */
        flush: flushChanges,
        /** 获取待发送变更数量。 */
        getPendingCount: function () { return Object.keys(_pendingChanges).length; }
    };

    // DOM 就绪后自动初始化。
    if (document.readyState === 'loading') {
        document.addEventListener('DOMContentLoaded', initialize);
    } else {
        initialize();
    }

    debugLog('插件已加载');
})();
