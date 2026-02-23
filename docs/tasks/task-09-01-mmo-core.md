# Task 09-01 - mmo-core.js（WebSocket 核心）

> **优先级**：P0（M1 必须，所有其他插件依赖本插件）
> **里程碑**：M1
> **依赖**：task-09-00（mmo-loader 先于本插件加载）
> **被依赖**：所有其他 mmo-*.js 插件

---

## 目标

实现全局 `$MMO` 对象：WebSocket 连接管理（连接/断线/重连）、消息收发与事件分发、心跳、seq 序列号管理、本地存档禁用。这是所有客户端插件通信的唯一底层接口。

---

## Todolist

- [ ] **01-1** 实现 `$MMO` 全局对象骨架（挂载到 `window.$MMO`）
- [ ] **01-2** 实现 WebSocket 连接（`$MMO.connect(url)`）
- [ ] **01-3** 实现消息发送（`$MMO.send(type, payload)`，自动附加 seq）
- [ ] **01-4** 实现消息接收与事件分发（`$MMO.on(type, fn)` / `$MMO.off(type, fn)`）
- [ ] **01-5** 实现指数退避重连（1s→2s→4s→...→60s 上限，可配置最大次数）
- [ ] **01-6** 实现心跳（每 30s 发 `ping`，收到 `pong` 重置计时器）
- [ ] **01-7** 实现本地存档禁用（覆盖 `DataManager.saveGame` / `loadGame` / `isAnySavefileExists`）
- [ ] **01-8** 实现调试接口（`$MMO.debug`，仅 `$MMO_CONFIG.debug === true` 时挂载）
- [ ] **01-9** 实现 `$MMO.state` 状态机（`disconnected` / `connecting` / `connected` / `reconnecting`）
- [ ] **01-10** 实现 Session 信息存储（`$MMO.token`, `$MMO.charId`, `$MMO.accountId`）

---

## 实现细节与思路

### 完整骨架

```javascript
(function () {
    'use strict';

    var cfg = window.$MMO_CONFIG || {};

    var MMO = window.$MMO = {
        // ─── 状态 ─────────────────────────────────────────────
        state:      'disconnected',   // disconnected|connecting|connected|reconnecting
        token:      null,
        charId:     null,
        accountId:  null,

        // ─── 内部 ─────────────────────────────────────────────
        _ws:             null,
        _seq:            0,
        _handlers:       {},          // {type → [fn, ...]}
        _reconnectDelay: cfg.reconnectDelay || 1000,
        _reconnectCount: 0,
        _maxReconnects:  cfg.maxReconnects  || 10,
        _heartbeatTimer: null,
        _serverUrl:      null,
        _debug:          !!cfg.debug,

        // ─── 公共 API ─────────────────────────────────────────

        /** 建立 WebSocket 连接 */
        connect: function (url) {
            this._serverUrl = url || cfg.serverUrl;
            if (!this.token) {
                console.error('[MMO Core] connect() 前必须先设置 $MMO.token');
                return;
            }
            var wsUrl = this._serverUrl + '/ws?token=' + encodeURIComponent(this.token);
            this._log('连接到：' + wsUrl);
            this.state = 'connecting';
            this._ws   = new WebSocket(wsUrl);
            this._bindEvents();
        },

        /** 断开连接（不触发重连） */
        disconnect: function () {
            this._reconnectCount = Infinity;   // 阻止重连
            if (this._ws) this._ws.close();
            this._cleanup();
        },

        /** 发送消息 */
        send: function (type, payload) {
            if (!this._ws || this._ws.readyState !== WebSocket.OPEN) {
                this._log('send() 时连接未就绪，已丢弃：' + type, 'warn');
                return false;
            }
            var pkt = { seq: ++this._seq, type: type, payload: payload || {} };
            this._ws.send(JSON.stringify(pkt));
            if (this._debug) console.log('[MMO→S]', pkt);
            return true;
        },

        /** 注册消息处理器 */
        on: function (type, fn) {
            if (!this._handlers[type]) this._handlers[type] = [];
            this._handlers[type].push(fn);
        },

        /** 注销消息处理器 */
        off: function (type, fn) {
            var list = this._handlers[type];
            if (!list) return;
            var idx = list.indexOf(fn);
            if (idx >= 0) list.splice(idx, 1);
        },

        // ─── 内部方法 ─────────────────────────────────────────

        _bindEvents: function () {
            var self = this;
            this._ws.onopen = function () {
                self.state            = 'connected';
                self._reconnectDelay  = cfg.reconnectDelay || 1000;
                self._reconnectCount  = 0;
                self._startHeartbeat();
                self._log('已连接');
                self._dispatch('$connected', {});
            };
            this._ws.onmessage = function (e) {
                var pkt;
                try { pkt = JSON.parse(e.data); } catch (ex) { return; }
                if (self._debug) console.log('[MMO←S]', pkt);
                self._dispatch(pkt.type, pkt.payload);
            };
            this._ws.onclose = function (ev) {
                self._log('连接关闭 code=' + ev.code, 'warn');
                self._cleanup();
                self._dispatch('$disconnected', { code: ev.code });
                self._scheduleReconnect();
            };
            this._ws.onerror = function (e) {
                self._log('WebSocket 错误', 'error');
            };
        },

        _dispatch: function (type, payload) {
            var fns = this._handlers[type] || [];
            for (var i = 0; i < fns.length; i++) {
                try { fns[i](payload); }
                catch (e) { console.error('[MMO Core] handler error [' + type + ']', e); }
            }
        },

        _startHeartbeat: function () {
            var self = this;
            var sec  = cfg.heartbeatSec || 30;
            this._heartbeatTimer = setInterval(function () {
                self.send('ping', { ts: Date.now() });
            }, sec * 1000);
        },

        _cleanup: function () {
            clearInterval(this._heartbeatTimer);
            this._heartbeatTimer = null;
            this.state = 'disconnected';
        },

        _scheduleReconnect: function () {
            if (this._reconnectCount >= this._maxReconnects) {
                this._log('重连次数已达上限（' + this._maxReconnects + '），停止重连', 'error');
                this._dispatch('$reconnect_failed', {});
                return;
            }
            var self  = this;
            var delay = this._reconnectDelay;
            this._log('将在 ' + delay + 'ms 后重连（第 ' + (this._reconnectCount + 1) + ' 次）...');
            this.state = 'reconnecting';
            setTimeout(function () {
                self._reconnectCount++;
                self._reconnectDelay = Math.min(self._reconnectDelay * 2, 60000);
                self.connect();
            }, delay);
        },

        _log: function (msg, level) {
            if (!this._debug && level !== 'error' && level !== 'warn') return;
            var fn = console[level || 'log'];
            fn('[MMO Core] ' + msg);
        },
    };

    // ─── pong 处理 ────────────────────────────────────────────
    MMO.on('pong', function (payload) {
        // 收到 pong 表示连接正常，无需额外操作
    });

    // ─── 禁用本地存档 ─────────────────────────────────────────
    DataManager.saveGame             = function () { return Promise.resolve(false); };
    DataManager.loadGame             = function () { return Promise.resolve(false); };
    DataManager.isAnySavefileExists  = function () { return false; };
    DataManager.latestSavefileId     = function () { return 0; };

    // 隐藏存档/读档菜单项（覆盖 Window_MenuCommand）
    var _addOriginalCommands = Window_MenuCommand.prototype.addOriginalCommands;
    Window_MenuCommand.prototype.addOriginalCommands = function () {
        _addOriginalCommands.call(this);
        // 让 Save/Load 命令不可用
    };
    Window_MenuCommand.prototype.isSaveEnabled = function () { return false; };

    // ─── 调试接口 ─────────────────────────────────────────────
    if (cfg.debug) {
        MMO.debug = {
            sendRaw:        function (type, payload) { MMO.send(type, payload); },
            getState:       function () { return MMO.state; },
            getSession:     function () { return { token: MMO.token, charId: MMO.charId }; },
            enablePacketLog: function () { MMO._debug = true; },
            disablePacketLog: function () { MMO._debug = false; },
            simulateLag:    function (ms) {
                var orig = MMO._ws.send.bind(MMO._ws);
                MMO._ws.send = function (data) { setTimeout(function () { orig(data); }, ms); };
            },
        };
        window.$MMO = MMO;   // 确保 DevTools 可访问
        console.log('[MMO Core] 调试模式已开启，使用 $MMO.debug 访问调试接口');
    }

})();
```

### 依赖关系说明

其他插件获取配置和连接的方式：
```javascript
// 任意其他插件中
var cfg = window.$MMO_CONFIG;   // Loader 注入的配置
var mmo = window.$MMO;          // Core 注入的连接对象

mmo.on('player_sync', function(payload) { /* ... */ });
mmo.send('player_move', { x: 5, y: 3, dir: 6 });
```

内置事件（Core 内部触发，供其他插件监听）：
```
$connected       — WebSocket 连接建立
$disconnected    — 连接断开（含 code 字段）
$reconnect_failed — 重连次数超限
```

---

## 验收标准

1. 游戏启动后 Console 显示 `[MMO Core] 已连接`
2. 断线后自动重连，延迟指数增长（1s → 2s → 4s ...）
3. 心跳每 30s 发送 `ping`，服务端返回 `pong`
4. 尝试存档时：无任何存档操作执行，菜单中存档项不可选
5. `$MMO.debug.sendRaw('ping', {ts: Date.now()})` 在 DevTools 中可执行
