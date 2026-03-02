/*:
 * @plugindesc v1.0.0 MMO 核心 - WebSocket 连接管理与消息分发。
 * @author MMO Framework
 */

(function () {
    'use strict';

    /** 连接状态枚举。 */
    var STATE = { DISCONNECTED: 0, CONNECTING: 1, CONNECTED: 2, RECONNECTING: 3 };
    /** 心跳间隔（毫秒）。服务端期望 30 秒内收到 ping，否则判定超时。 */
    var HEARTBEAT_INTERVAL = 30000;
    /** 重连延迟上限（毫秒）。指数退避最大不超过 60 秒。 */
    var MAX_RECONNECT_DELAY = 60000;

    // ═══════════════════════════════════════════════════════════
    //  $MMO 全局单例
    //  所有 MMO 插件通过此对象进行 WebSocket 通信和状态共享。
    // ═══════════════════════════════════════════════════════════
    window.$MMO = {
        /** @type {string|null} JWT 认证令牌。 */
        token: null,
        /** @type {number|null} 当前角色 ID。 */
        charID: null,
        /** @type {string|null} 当前角色名称。 */
        charName: null,
        /** @type {WebSocket|null} 当前 WebSocket 连接实例。 */
        _ws: null,
        /** @type {number} 当前连接状态（STATE 枚举值）。 */
        _state: STATE.DISCONNECTED,
        /** @type {number} 消息序列号，用于请求-响应配对。 */
        _seq: 0,
        /** @type {Object.<string, Function[]>} 消息类型 → 处理函数数组的映射。 */
        _handlers: {},
        /** @type {number} 当前重连尝试次数。 */
        _reconnectAttempts: 0,
        /** @type {number|null} 重连定时器 ID。 */
        _reconnectTimer: null,
        /** @type {number|null} 心跳定时器 ID。 */
        _heartbeatTimer: null,
        /** @type {string} WebSocket 服务器地址。 */
        _serverUrl: (window.MMO_CONFIG && window.MMO_CONFIG.serverUrl) || 'ws://localhost:8080',
        /** @type {boolean} 调试模式开关。 */
        _debug: !!(window.MMO_CONFIG && window.MMO_CONFIG.debug),
        /** @type {number} 最大重连尝试次数。 */
        _reconnectMax: (window.MMO_CONFIG && window.MMO_CONFIG.reconnectMax) || 10,

        /**
         * 注册消息处理函数。
         * 同一消息类型可注册多个处理函数，按注册顺序依次调用。
         * @param {string} type - 消息类型（如 'map_init'、'player_sync'）
         * @param {Function} fn - 处理函数，接收 payload 参数
         * @returns {Object} 返回 $MMO 自身，支持链式调用
         */
        on: function (type, fn) {
            if (!this._handlers[type]) this._handlers[type] = [];
            this._handlers[type].push(fn);
            return this;
        },

        /**
         * 取消注册消息处理函数。
         * @param {string} type - 消息类型
         * @param {Function} fn - 要移除的处理函数引用
         */
        off: function (type, fn) {
            if (!this._handlers[type]) return;
            this._handlers[type] = this._handlers[type].filter(function (h) { return h !== fn; });
        },

        /**
         * 分发接收到的消息给所有已注册的处理函数。
         * 复制处理函数数组再遍历，防止处理函数中调用 on/off 导致迭代异常。
         * 每个处理函数独立 try-catch，单个处理函数抛异常不影响其他处理函数执行。
         * @param {string} type - 消息类型
         * @param {Object} payload - 消息载荷
         */
        _dispatch: function (type, payload) {
            if (this._debug) console.log('[MMO] <-', type, payload);
            var handlers = this._handlers[type];
            if (!handlers) return;
            var snapshot = handlers.slice();
            snapshot.forEach(function (fn) {
                try { fn(payload); } catch (e) { console.error('[MMO] 处理函数异常 (' + type + '):', e); }
            });
        },

        /**
         * 向服务器发送消息。
         * 自动附加递增序列号。序列号溢出 0xFFFFFF 后归零。
         * @param {string} type - 消息类型（如 'player_move'、'npc_interact'）
         * @param {Object} [payload={}] - 消息载荷
         * @returns {boolean} 发送成功返回 true，未连接或异常返回 false
         */
        send: function (type, payload) {
            if (this._state !== STATE.CONNECTED) return false;
            if (this._seq > 0xFFFFFF) this._seq = 0;
            var msg = JSON.stringify({ seq: ++this._seq, type: type, payload: payload || {} });
            if (this._debug) console.log('[MMO] ->', type, payload);
            try {
                this._ws.send(msg);
                return true;
            } catch (e) {
                console.error('[MMO] 发送失败:', e);
                return false;
            }
        },

        /**
         * 使用给定的 JWT 令牌连接 WebSocket 服务器。
         * 若已连接或正在连接中，则忽略重复调用。
         * @param {string} token - JWT 认证令牌
         */
        connect: function (token) {
            if (this._state === STATE.CONNECTED || this._state === STATE.CONNECTING) return;
            this.token = token;
            this._doConnect();
        },

        /**
         * 内部方法：建立 WebSocket 连接。
         * 连接成功后重置重连计数、启动心跳、分发 _connected 事件。
         * 连接关闭后自动触发重连调度。
         */
        _doConnect: function () {
            var self = this;
            this._state = STATE.CONNECTING;
            var url = this._serverUrl.replace(/^http/, 'ws') + '/ws?token=' + encodeURIComponent(this.token);
            var ws = new WebSocket(url);
            this._ws = ws;

            ws.onopen = function () {
                self._state = STATE.CONNECTED;
                self._reconnectAttempts = 0;
                self._startHeartbeat();
                self._dispatch('_connected', {});
                if (self._debug) console.log('[MMO] 已连接到服务器。');
            };

            ws.onmessage = function (evt) {
                try {
                    var msg = JSON.parse(evt.data);
                    self._dispatch(msg.type, msg.payload);
                } catch (e) {
                    console.error('[MMO] 消息解析失败:', e);
                }
            };

            ws.onerror = function (e) {
                console.error('[MMO] WebSocket 错误:', e);
            };

            ws.onclose = function () {
                self._stopHeartbeat();
                if (self._state === STATE.CONNECTED || self._state === STATE.CONNECTING) {
                    self._ws = null;
                    self._scheduleReconnect();
                }
            };
        },

        /**
         * 调度重连。使用指数退避策略：1s → 2s → 4s → ... → 最大60s。
         * 达到最大重连次数后放弃，分发 _reconnect_failed 和 _disconnected 事件。
         */
        _scheduleReconnect: function () {
            var self = this;
            if (this._reconnectAttempts >= this._reconnectMax) {
                console.error('[MMO] 已达到最大重连次数。');
                this._state = STATE.DISCONNECTED;
                this._dispatch('_reconnect_failed', {});
                this._dispatch('_disconnected', {});
                return;
            }
            this._state = STATE.RECONNECTING;
            var delay = Math.min(1000 * Math.pow(2, this._reconnectAttempts), MAX_RECONNECT_DELAY);
            this._reconnectAttempts++;
            if (this._debug) console.log('[MMO] ' + delay + 'ms 后尝试重连（第 ' + this._reconnectAttempts + ' 次）');
            this._reconnectTimer = setTimeout(function () {
                self._doConnect();
            }, delay);
        },

        /**
         * 主动断开连接。清除心跳和重连定时器，关闭 WebSocket。
         * 设置 onclose=null 防止关闭事件触发自动重连。
         */
        disconnect: function () {
            this._state = STATE.DISCONNECTED;
            this._stopHeartbeat();
            if (this._reconnectTimer) { clearTimeout(this._reconnectTimer); this._reconnectTimer = null; }
            if (this._ws) { this._ws.onclose = null; this._ws.close(); this._ws = null; }
        },

        /** 启动心跳定时器，每 30 秒发送一次 ping 消息。 */
        _startHeartbeat: function () {
            var self = this;
            this._stopHeartbeat();
            this._heartbeatTimer = setInterval(function () {
                self.send('ping', { ts: Date.now() });
            }, HEARTBEAT_INTERVAL);
        },

        /** 停止心跳定时器。 */
        _stopHeartbeat: function () {
            if (this._heartbeatTimer) { clearInterval(this._heartbeatTimer); this._heartbeatTimer = null; }
        },

        /**
         * 检查当前是否已连接。
         * @returns {boolean}
         */
        isConnected: function () { return this._state === STATE.CONNECTED; },

        // ═══════════════════════════════════════════════════════════
        //  底部 UI 注册表
        //  当 RMMV 对话框/选项窗口激活时，自动隐藏已注册的底部面板
        //  （如快捷栏、聊天框），避免遮挡游戏对话。
        // ═══════════════════════════════════════════════════════════
        /** @type {Array} 已注册的底部 UI 面板列表。 */
        _bottomUI: [],
        /** @type {boolean} 当前是否处于事件忙碌状态。 */
        _eventBusy: false,

        /**
         * 注册底部 UI 面板，在对话激活时自动隐藏。
         * @param {Object} panel - 面板对象（需支持 visible/hide/show）
         */
        registerBottomUI: function (panel) {
            if (this._bottomUI.indexOf(panel) < 0) this._bottomUI.push(panel);
        },

        /**
         * 取消注册底部 UI 面板。
         * @param {Object} panel - 面板对象
         */
        unregisterBottomUI: function (panel) {
            var idx = this._bottomUI.indexOf(panel);
            if (idx >= 0) this._bottomUI.splice(idx, 1);
        }
    };

    // ═══════════════════════════════════════════════════════════
    //  禁用 RMMV 本地存档
    //  MMO 模式下所有游戏状态由服务器管理，禁用客户端存档避免混乱。
    //  但保留系统设置文件（fileId=0）的存取，允许玩家保持音量等偏好。
    // ═══════════════════════════════════════════════════════════
    var _StorageManager_save = StorageManager.save;
    StorageManager.save = function (fileId, json) {
        if (fileId === 0) return _StorageManager_save.call(this, fileId, json);
        // 非系统设置的存档操作静默忽略
    };
    var _StorageManager_load = StorageManager.load;
    StorageManager.load = function (fileId) {
        if (fileId === 0) return _StorageManager_load.call(this, fileId);
        return null;
    };

    /**
     * 禁用 RMMV 队伍跟随系统。
     * 默认 partyMembers=[1,2,3,4] 会显示 3 个尾随精灵，
     * MMO 使用独立的队伍系统通过 WebSocket 管理。
     */
    Game_Followers.prototype.initialize = function () {
        this._visible = false;
        this._gathering = false;
        this._data = [];
    };

    /**
     * 限制 RMMV 队伍只保留一个角色（玩家本体）。
     * 默认 setupStartingMembers 会添加角色 1-4 到队伍中，
     * MMO 模式下只需角色 1 作为玩家化身。
     */
    Game_Party.prototype.setupStartingMembers = function () {
        this._actors = [];
        this.addActor(1);
    };

    /** 移除菜单中的"保存"命令 — MMO 无本地存档。 */
    Window_MenuCommand.prototype.addSaveCommand = function () {};
    /** 移除菜单中的"整队"命令 — MMO 通过服务器管理队伍。 */
    Window_MenuCommand.prototype.addFormationCommand = function () {};

    // ═══════════════════════════════════════════════════════════
    //  移动同步：每次移动后将玩家位置发送到服务器。
    //  若不同步，服务器保存的始终是初始位置，
    //  断线重连后玩家会被传送回起点。
    // ═══════════════════════════════════════════════════════════

    /**
     * 覆写直线移动 — 移动成功后向服务器发送 player_move 消息。
     * 保留原始 moveStraight 逻辑，仅在移动成功时追加网络同步。
     */
    var _Game_Player_moveStraight = Game_Player.prototype.moveStraight;
    Game_Player.prototype.moveStraight = function (d) {
        _Game_Player_moveStraight.call(this, d);
        if (this.isMovementSucceeded() && $MMO.isConnected()) {
            $MMO.send('player_move', { x: this._x, y: this._y, dir: this._direction });
        }
    };

    /**
     * 覆写对角线移动 — 移动成功后向服务器发送 player_move 消息。
     * 对角移动的方向值取决于最终朝向（非组合方向）。
     */
    var _Game_Player_moveDiagonally = Game_Player.prototype.moveDiagonally;
    Game_Player.prototype.moveDiagonally = function (horz, vert) {
        _Game_Player_moveDiagonally.call(this, horz, vert);
        if (this.isMovementSucceeded() && $MMO.isConnected()) {
            $MMO.send('player_move', { x: this._x, y: this._y, dir: this._direction });
        }
    };

    // ═══════════════════════════════════════════════════════════
    //  禁用客户端地图传送（代码 201）
    //  所有地图传送由服务端 NPC 执行器处理：遇到 command 201 时
    //  调用 enterMapRoom 发送 map_init，客户端收到后执行传送。
    //  此覆写作为安全网，防止残留的客户端解释器（如公共事件）触发传送。
    // ═══════════════════════════════════════════════════════════
    Game_Interpreter.prototype.command201 = function () {
        return true; // 空操作 — 传送由服务器处理
    };

    // ═══════════════════════════════════════════════════════════
    //  map_init 消息处理
    //  在初始登录、重新登录、服务端地图传送时触发。
    //  同步玩家位置、变量/开关、装备、音频到客户端。
    //
    //  玩家透明度：遵循 $dataSystem.optTransparent 设置，
    //  不强制设为可见。服务器会转发 code 211（改变透明度）
    //  来显式控制可见性 — 例如地图 2 事件 156 params=[1]（设为可见）。
    //  这保持了原始游戏在地图 20（难度选择/片头）玩家不可见的行为。
    // ═══════════════════════════════════════════════════════════
    $MMO.on('map_init', function (data) {
        if (!data || !data.self) return;
        var s = data.self;
        var mapId = s.map_id || 1;
        var x     = s.x != null ? s.x : 0;
        var y     = s.y != null ? s.y : 0;
        var dir   = s.dir || 2;

        // 保存到 _lastSelf，供 HUD、状态窗口等延迟加载的组件使用。
        $MMO._lastSelf = s;

        // 从服务器同步变量到客户端。
        // 客户端并行公共事件需要正确的变量/开关值来计算视觉效果（如昼夜色调）。
        if (data.variables && $gameVariables) {
            var vars = data.variables;
            for (var k in vars) {
                if (vars.hasOwnProperty(k)) {
                    $gameVariables._data[parseInt(k, 10)] = vars[k];
                }
            }
        }
        if (data.switches && $gameSwitches) {
            var sw = data.switches;
            for (var k in sw) {
                if (sw.hasOwnProperty(k)) {
                    $gameSwitches._data[parseInt(k, 10)] = sw[k];
                }
            }
        }

        // 同步装备到 $gameActors，使客户端渲染插件
        // （CallCutin.js、CallStand.js）能看到正确的装备状态。
        // 每项格式：{slot_index, item_id, kind}，kind: 2=武器, 3=防具。
        if (data.equips && $gameActors) {
            var actor = $gameActors.actor(1);
            if (actor && actor._equips) {
                for (var i = 0; i < data.equips.length; i++) {
                    var eq = data.equips[i];
                    var slot = eq.slot_index;
                    if (slot >= 0 && slot < actor._equips.length) {
                        var isWeapon = (eq.kind === 2);
                        actor._equips[slot].setEquip(isWeapon, eq.item_id);
                    }
                }
            }
        }

        // 播放服务器指定的地图 BGM/BGS。
        // 补充 RMMV 内置的 $gameMap.autoplay()，确保即使客户端地图数据
        // 尚未加载完成也能播放正确的音频。
        if (data.audio) {
            if (data.audio.bgm) AudioManager.playBgm(data.audio.bgm);
            if (data.audio.bgs) AudioManager.playBgs(data.audio.bgs);
        }

        if ($gamePlayer && $gameMap) {
            if ($gameMap.mapId() !== mapId) {
                // 跨地图传送：使用 RMMV 的 reserveTransfer 异步加载新地图。
                $gamePlayer.reserveTransfer(mapId, x, y, dir, 0);
            } else {
                // 同地图重定位：直接设置坐标和朝向。
                $gamePlayer.locate(x, y);
                $gamePlayer.setDirection(dir);
            }

            // 强制刷新精灵以立即应用服务器的 walk_name。
            // 否则同地图重入（重新登录到同一地图）会保留上次会话的旧精灵。
            $gamePlayer.refresh();
        }
    });

    // ═══════════════════════════════════════════════════════════
    //  实时增量状态同步
    //  服务器在 NPC 执行器修改变量/开关时推送增量更新，
    //  客户端直接写入 _data 数组（绕过 onChange 触发器，
    //  因为公共事件刷新由下面的 updateEvents 覆写单独处理）。
    // ═══════════════════════════════════════════════════════════

    /** 处理服务器推送的变量变更。 */
    $MMO.on('var_change', function (data) {
        if ($gameVariables && data && data.id != null) {
            $gameVariables._data[data.id] = data.value || 0;
        }
    });

    /** 处理服务器推送的开关变更。 */
    $MMO.on('switch_change', function (data) {
        if ($gameSwitches && data && data.id != null) {
            $gameSwitches._data[data.id] = !!data.value;
        }
    });

    /**
     * 处理服务器发起的地图传送（NPC 执行器无 TransferFn 时的回退方案）。
     * 参数格式与 RMMV command 201 相同：map_id, x, y, dir。
     */
    $MMO.on('transfer_player', function (data) {
        if (!data || !$gamePlayer) return;
        var mapId = data.map_id || 1;
        var x     = data.x != null ? data.x : 0;
        var y     = data.y != null ? data.y : 0;
        var dir   = data.dir || 2;
        $gamePlayer.reserveTransfer(mapId, x, y, dir, 0);
    });

    // ═══════════════════════════════════════════════════════════
    //  覆写 Game_Player.refresh
    //  使行走精灵来自 MMO 服务器而非 $gameParty.leader()。
    //  否则 reserveTransfer → performTransfer → refresh() 会将精灵
    //  重置为默认角色外观。
    //  不修改透明度 — 让 $dataSystem.optTransparent 和服务器 code 211 控制。
    // ═══════════════════════════════════════════════════════════
    var _GamePlayer_refresh = Game_Player.prototype.refresh;
    Game_Player.prototype.refresh = function () {
        var s = $MMO._lastSelf;
        if (s && s.walk_name) {
            this.setImage(s.walk_name, s.walk_index || 0);
        } else {
            _GamePlayer_refresh.call(this);
        }
    };

    /** 处理心跳 pong 响应，在调试模式下输出延迟。 */
    $MMO.on('pong', function (payload) {
        if ($MMO._debug) console.log('[MMO] Pong, 延迟:', Date.now() - (payload.client_ts || 0), 'ms');
    });

    /**
     * 处理移动拒绝：服务器因通行度或速度违规拒绝了 player_move。
     * 将玩家瞬移到服务器的权威位置，防止连锁偏移
     * （否则后续每次移动都会因坐标不同步而被拒绝）。
     */
    $MMO.on('move_reject', function (data) {
        if (!data || !$gamePlayer) return;
        console.warn('[MMO] 移动被拒绝 — 回弹到服务器位置:',
            data.x, data.y, '方向', data.dir);
        $gamePlayer.locate(data.x, data.y);
        if (data.dir) $gamePlayer.setDirection(data.dir);
    });

    /** 处理服务器通用错误消息。 */
    $MMO.on('error', function (data) {
        console.warn('[MMO] 服务器错误:', data && data.message);
    });

    // ═══════════════════════════════════════════════════════════
    //  断线提示：弹窗通知并返回登录界面。
    //  仅在游戏内场景（非登录/角色选择/创建）时显示。
    // ═══════════════════════════════════════════════════════════
    $MMO.on('_disconnected', function () {
        // 仅在游戏内（非登录/角色选择/创建场景）时显示断线提示。
        if (!SceneManager._scene || SceneManager._scene instanceof Scene_Title) return;
        if (typeof Scene_Login !== 'undefined' && SceneManager._scene instanceof Scene_Login) return;
        if (typeof Scene_CharacterSelect !== 'undefined' && SceneManager._scene instanceof Scene_CharacterSelect) return;
        if (typeof Scene_CharacterCreate !== 'undefined' && SceneManager._scene instanceof Scene_CharacterCreate) return;

        // 清理状态，防止重新登录时显示过期的精灵。
        $MMO.token = null;
        $MMO.charID = null;
        $MMO._lastSelf = null;

        alert('与服务器的连接已断开');
        SceneManager.goto(Scene_Title);
    });

    // ═══════════════════════════════════════════════════════════
    //  可拖拽 UI 面板 — 支持 localStorage 位置持久化。
    //  用法：$MMO.makeDraggable(panel, 'key', { dragArea: {y,h}, onMove: fn })
    //  在面板的 update() 中调用 $MMO.updateDrag(panel)，
    //  拖拽进行中返回 true 以便调用方跳过自身的点击处理。
    // ═══════════════════════════════════════════════════════════

    /**
     * 初始化面板的拖拽功能。
     * 从 localStorage 恢复上次保存的位置，约束在屏幕范围内。
     * @param {Object} panel - 面板精灵（需有 x, y, width, height 属性）
     * @param {string} key - 存储键名（自动加 'mmo_ui_' 前缀）
     * @param {Object} [opts] - 选项
     * @param {Object} [opts.dragArea] - 可拖拽区域 {y, h}，相对于面板
     * @param {Function} [opts.onMove] - 拖拽时的回调
     */
    $MMO.makeDraggable = function (panel, key, opts) {
        opts = opts || {};
        var saved = null;
        try { saved = JSON.parse(localStorage.getItem('mmo_ui_' + key)); } catch (e) {}
        if (saved) {
            panel.x = Math.max(0, Math.min(saved.x, Graphics.boxWidth - panel.width));
            panel.y = Math.max(0, Math.min(saved.y, Graphics.boxHeight - panel.height));
        }
        panel._drag = {
            key: key,
            active: false,       // 正在拖拽中
            pending: false,      // 已按下但未达到拖拽阈值
            startX: 0, startY: 0,
            offX: 0, offY: 0,
            area: opts.dragArea || null,
            onMove: opts.onMove || null
        };
    };

    /**
     * 每帧更新面板的拖拽状态。
     * 需要在面板的 update() 方法中持续调用。
     * 拖拽阈值为 4 像素，防止误触。
     * @param {Object} panel - 面板精灵
     * @returns {boolean} 正在拖拽返回 true
     */
    $MMO.updateDrag = function (panel) {
        if (!panel._drag || !panel.visible) return false;
        var d = panel._drag;
        var tx = TouchInput.x, ty = TouchInput.y;

        // 检测触摸开始：在面板内且在可拖拽区域内。
        if (TouchInput.isTriggered() && panel.isInside(tx, ty)) {
            var ly = ty - panel.y;
            if (!d.area || (ly >= d.area.y && ly < d.area.y + d.area.h)) {
                d.startX = tx; d.startY = ty;
                d.offX = tx - panel.x; d.offY = ty - panel.y;
                d.pending = true; d.active = false;
            }
        }

        if (d.pending || d.active) {
            if (TouchInput.isPressed()) {
                // 超过阈值后才进入拖拽状态。
                if (!d.active && (Math.abs(tx - d.startX) + Math.abs(ty - d.startY)) > 4) {
                    d.active = true; d.pending = false;
                }
                if (d.active) {
                    // 约束面板在屏幕范围内。
                    panel.x = Math.max(0, Math.min(tx - d.offX, Graphics.boxWidth - panel.width));
                    panel.y = Math.max(0, Math.min(ty - d.offY, Graphics.boxHeight - panel.height));
                    if (d.onMove) d.onMove();
                    return true;
                }
            } else {
                // 触摸释放：保存位置到 localStorage。
                if (d.active) {
                    try { localStorage.setItem('mmo_ui_' + d.key, JSON.stringify({ x: panel.x, y: panel.y })); } catch (e) {}
                    if (d.onMove) d.onMove();
                }
                d.active = false; d.pending = false;
            }
        }
        return d.active;
    };

    // ═══════════════════════════════════════════════════════════
    //  player_sync 处理 — 保持 $MMO._lastSelf 与服务器同步。
    //  HUD 状态栏等组件从 _lastSelf 读取 HP/MP/等级等数据。
    // ═══════════════════════════════════════════════════════════
    $MMO.on('player_sync', function (data) {
        if (data.char_id !== $MMO.charID) return;
        if (!$MMO._lastSelf) $MMO._lastSelf = {};
        var s = $MMO._lastSelf;
        if (data.hp !== undefined)     s.hp = data.hp;
        if (data.max_hp !== undefined) s.max_hp = data.max_hp;
        if (data.mp !== undefined)     s.mp = data.mp;
        if (data.max_mp !== undefined) s.max_mp = data.max_mp;
        if (data.level !== undefined)  s.level = data.level;
        if (data.exp !== undefined)    s.exp = data.exp;
        if (data.next_exp !== undefined) s.next_exp = data.next_exp;
    });

    // ═══════════════════════════════════════════════════════════
    //  窗口管理器 — 跟踪已打开的 GameWindow，ESC 关闭最顶层。
    //  注意：GameWindow 类定义在 mmo-game-window.js（依赖 L2_Base）。
    // ═══════════════════════════════════════════════════════════
    /** @type {Array} 已注册的游戏窗口列表。 */
    $MMO._gameWindows = [];

    /**
     * 注册一个游戏窗口到窗口管理器。
     * @param {Object} win - GameWindow 实例
     */
    $MMO.registerWindow = function (win) {
        if (this._gameWindows.indexOf(win) < 0) this._gameWindows.push(win);
    };

    /**
     * 关闭最顶层可见窗口。从后往前遍历（后注册的在上层）。
     * @returns {boolean} 成功关闭返回 true，无可见窗口返回 false
     */
    $MMO.closeTopWindow = function () {
        for (var i = this._gameWindows.length - 1; i >= 0; i--) {
            if (this._gameWindows[i].visible) {
                this._gameWindows[i].close();
                return true;
            }
        }
        return false;
    };

    /**
     * 中央操作分发 — 打开/切换各功能窗口。
     * @param {string} action - 操作标识：'status'|'skills'|'inventory'|'friends'|'guild'|'system'
     */
    $MMO._triggerAction = function (action) {
        if (action === 'status' && $MMO._statusWindow)       $MMO._statusWindow.toggle();
        else if (action === 'skills' && $MMO._skillWindow)   $MMO._skillWindow.toggle();
        else if (action === 'inventory' && $MMO._inventoryWindow) $MMO._inventoryWindow.toggle();
        else if (action === 'friends' && $MMO._friendListWin) {
            $MMO._friendListWin.visible = !$MMO._friendListWin.visible;
            if ($MMO._friendListWin.visible) {
                $MMO._friendListWin.refresh();
                $MMO._friendListWin.loadFriends();
            }
        }
        else if (action === 'guild' && $MMO._guildInfoWin) {
            $MMO._guildInfoWin.visible = !$MMO._guildInfoWin.visible;
            if ($MMO._guildInfoWin.visible) {
                $MMO._guildInfoWin.refresh();
                if ($MMO._guildID) $MMO._guildInfoWin.loadGuild($MMO._guildID);
            }
        }
        else if (action === 'system' && $MMO._systemMenu) $MMO._systemMenu.toggle();
    };

    // ═══════════════════════════════════════════════════════════
    //  禁用 RMMV 菜单。ESC 改为关闭最顶层窗口/切换系统菜单。
    //  右键用于玩家上下文菜单（组队/交易等）。
    // ═══════════════════════════════════════════════════════════
    Scene_Map.prototype.isMenuCalled = function () { return false; };
    Scene_Map.prototype.callMenu = function () {};

    /**
     * 全局键盘快捷键监听。
     * Alt+T: 打开/关闭状态窗口
     * Alt+S: 打开/关闭技能窗口
     */
    window.addEventListener('keydown', function (e) {
        if (!(SceneManager._scene instanceof Scene_Map)) return;
        if (e.altKey && e.keyCode === 84) { // Alt+T → 状态
            e.preventDefault();
            $MMO._triggerAction('status');
        }
        if (e.altKey && e.keyCode === 83) { // Alt+S → 技能
            e.preventDefault();
            $MMO._triggerAction('skills');
        }
    });

    // ═══════════════════════════════════════════════════════════
    //  防止点击 MMO UI 时触发地图点击移动。
    //  通过检查 L2_Base 上的 _isMMOUI 标记来判断点击目标。
    //  无需在加载时引用 L2_Base — mmo-core 先于 mmo-ui 加载。
    // ═══════════════════════════════════════════════════════════
    var _SMpmt_core = Scene_Map.prototype.processMapTouch;
    Scene_Map.prototype.processMapTouch = function () {
        if (TouchInput.isTriggered() || TouchInput.isPressed()) {
            var tx = TouchInput.x, ty = TouchInput.y;
            var ch = this.children;
            for (var i = ch.length - 1; i >= 0; i--) {
                var c = ch[i];
                if (c && c.visible && c._isMMOUI &&
                    typeof c.isInside === 'function' && c.isInside(tx, ty)) {
                    return; // 点击在 MMO UI 上 — 阻止移动
                }
            }
        }
        _SMpmt_core.call(this);
    };

    /**
     * 覆写 Scene_Map.update — 添加 ESC 关闭窗口和底部 UI 自动隐藏逻辑。
     * ESC 按下时：优先关闭最顶层窗口，若无窗口则切换系统菜单。
     * 对话激活时：自动隐藏所有已注册的底部 UI 面板。
     */
    var _Scene_Map_update_core = Scene_Map.prototype.update;
    Scene_Map.prototype.update = function () {
        _Scene_Map_update_core.call(this);
        // ESC：关闭最顶层窗口，或切换系统菜单。
        if (Input.isTriggered('cancel') || Input.isTriggered('escape')) {
            if (!$MMO.closeTopWindow()) {
                $MMO._triggerAction('system');
            }
        }
        // RMMV 对话/选项窗口激活时隐藏底部 MMO UI。
        var busy = !!($gameMessage && $gameMessage.isBusy());
        if (busy !== $MMO._eventBusy) {
            $MMO._eventBusy = busy;
            $MMO._bottomUI.forEach(function (panel) {
                if (busy) {
                    panel._mmoHiddenByEvent = panel.visible;
                    if (typeof panel.hide === 'function') panel.hide();
                    else panel.visible = false;
                } else if (panel._mmoHiddenByEvent) {
                    if (typeof panel.show === 'function') panel.show();
                    else panel.visible = true;
                    panel._mmoHiddenByEvent = false;
                }
            });
        }
    };

    // ═══════════════════════════════════════════════════════════
    //  调试接口（仅在调试模式下可用）。
    //  通过浏览器控制台使用：$MMO_DEBUG.send('type', {payload})
    // ═══════════════════════════════════════════════════════════
    if (window.MMO_CONFIG && window.MMO_CONFIG.debug) {
        window.$MMO_DEBUG = {
            /** 手动发送 WebSocket 消息。 */
            send: function (type, payload) { $MMO.send(type, payload); },
            /** 查看当前连接状态。 */
            state: function () { return $MMO._state; },
            /** 列出所有已注册的消息类型。 */
            handlers: function () { return Object.keys($MMO._handlers); }
        };
    }

})();
