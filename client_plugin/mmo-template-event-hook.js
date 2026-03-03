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
    //  钩子 Game_SelfSwitches.setVariableValue
    //  拦截 TemplateEvent.js 自变量修改，同步到服务器。
    //  TemplateEvent.js 使用独立的 setVariableValue 方法（非 setValue）。
    //  key 格式：[mapId, eventId, index]，值为数字。
    // ═══════════════════════════════════════════════════════════

    /**
     * 覆写 Game_SelfSwitches.setVariableValue 以拦截自变量修改。
     * TemplateEvent.js 自变量通过此方法存取，与 setValue（A/B/C/D 自开关）完全独立。
     * 始终调用原始方法以保持本地状态正确。
     */
    function hookGameSelfSwitches() {
        // 等待 TemplateEvent.js 安装 setVariableValue（它可能在本插件之后加载）。
        if (!Game_SelfSwitches.prototype.setVariableValue) {
            debugLog('setVariableValue 尚未定义，延迟 hook');
            return false;
        }

        var originalSetVariableValue = Game_SelfSwitches.prototype.setVariableValue;

        Game_SelfSwitches.prototype.setVariableValue = function (key, value) {
            // key 格式: [mapId, eventId, index]
            if (Array.isArray(key) && key.length >= 3) {
                var mapId = key[0];
                var eventId = key[1];
                var index = key[2];
                debugLog('拦截自变量修改:', { mapId: mapId, eventId: eventId, index: index, value: value });
                queueChange(mapId, eventId, index, typeof value === 'number' ? value : 0);
            }

            // 始终调用原始方法。
            return originalSetVariableValue.call(this, key, value);
        };

        debugLog('Game_SelfSwitches.setVariableValue 钩子已安装');
        return true;
    }

    // ═══════════════════════════════════════════════════════════
    //  检测 TemplateEvent.js
    //  TemplateEvent.js 不创建全局对象，通过 setVariableValue 方法判断。
    // ═══════════════════════════════════════════════════════════

    /**
     * 检测 TemplateEvent.js 是否已加载。
     * TemplateEvent.js 使用 IIFE 模式，通过检查其在 Game_SelfSwitches 上
     * 添加的 setVariableValue 方法来判断。
     * @returns {boolean} 是否已加载
     */
    function detectTemplateEvent() {
        if (typeof Game_SelfSwitches !== 'undefined' &&
            Game_SelfSwitches.prototype.setVariableValue) {
            _templateEventDetected = true;
            debugLog('已检测到 TemplateEvent.js（通过 setVariableValue）');
            return true;
        }
        return false;
    }

    // ═══════════════════════════════════════════════════════════
    //  服务端→客户端同步：self_var_change 处理器
    //  服务端执行 TE_SET_SELF_VARIABLE 后推送变更，客户端更新本地状态。
    // ═══════════════════════════════════════════════════════════

    /**
     * 注册 self_var_change 消息处理器。
     * 服务端通过此消息推送自变量变更，客户端更新 $gameSelfSwitches._variableData。
     */
    function registerSelfVarChangeHandler() {
        if (typeof $MMO === 'undefined' || !$MMO.on) return;

        $MMO.on('self_var_change', function (data) {
            if (!$gameSelfSwitches || !$gameSelfSwitches.setVariableValue) return;

            var key = [data.map_id, data.event_id, data.index];
            debugLog('服务端自变量变更:', { mapId: data.map_id, eventId: data.event_id, index: data.index, value: data.value });

            // 直接写入 _variableData 避免触发 hook 回传服务器。
            if ($gameSelfSwitches._variableData) {
                $gameSelfSwitches._variableData[key] = data.value;
                $gameSelfSwitches.onChange();
            }
        });

        debugLog('self_var_change 处理器已注册');
    }

    // ═══════════════════════════════════════════════════════════
    //  初始化
    //  等待 $MMO 和 TemplateEvent.js 就绪后安装所有钩子。
    //  30 秒超时后停止等待，防止未连接时无限轮询。
    // ═══════════════════════════════════════════════════════════

    /**
     * 初始化插件。等待 $MMO 可用后安装钩子。
     */
    function initialize() {
        debugLog('初始化中...');

        var hookInstalled = false;
        // 等待 $MMO 和 TemplateEvent.js 就绪。
        var checkInterval = setInterval(function () {
            if (typeof $MMO === 'undefined' || !$MMO.isConnected) return;

            // 首次检测到 $MMO 时注册服务端→客户端处理器。
            if (!hookInstalled) {
                checkServerAvailability();
                debugLog('服务器可用:', _isServerAvailable);

                // 注册服务端推送的自变量变更处理器。
                registerSelfVarChangeHandler();

                // 场景切换时刷新待发送变更（防止切换地图时丢失）。
                if (typeof Scene_Map !== 'undefined') {
                    var originalTerminate = Scene_Map.prototype.terminate;
                    Scene_Map.prototype.terminate = function () {
                        flushChanges();
                        return originalTerminate.call(this);
                    };
                }
            }

            // 检测 TemplateEvent.js 并安装 setVariableValue 钩子。
            if (!hookInstalled && detectTemplateEvent()) {
                hookInstalled = hookGameSelfSwitches();
            }

            // 所有钩子安装完毕后停止轮询。
            if (hookInstalled) {
                clearInterval(checkInterval);
                debugLog('初始化完成');
            }
        }, 200);

        // 30 秒超时后停止等待。
        setTimeout(function () {
            if (!hookInstalled) {
                clearInterval(checkInterval);
                debugLog('初始化超时，TemplateEvent.js 可能未加载');
            }
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
