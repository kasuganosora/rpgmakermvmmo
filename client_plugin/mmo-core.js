/*:
 * @plugindesc v1.0.0 MMO Core - WebSocket connection management and message dispatch.
 * @author MMO Framework
 */

(function () {
    'use strict';

    var STATE = { DISCONNECTED: 0, CONNECTING: 1, CONNECTED: 2, RECONNECTING: 3 };
    var HEARTBEAT_INTERVAL = 30000; // 30 s
    var MAX_RECONNECT_DELAY = 60000; // 60 s

    // -----------------------------------------------------------------
    // $MMO global object
    // -----------------------------------------------------------------
    window.$MMO = {
        token: null,
        charID: null,
        charName: null,
        _ws: null,
        _state: STATE.DISCONNECTED,
        _seq: 0,
        _handlers: {},
        _reconnectAttempts: 0,
        _reconnectTimer: null,
        _heartbeatTimer: null,
        _serverUrl: (window.MMO_CONFIG && window.MMO_CONFIG.serverUrl) || 'ws://localhost:8080',
        _debug: !!(window.MMO_CONFIG && window.MMO_CONFIG.debug),
        _reconnectMax: (window.MMO_CONFIG && window.MMO_CONFIG.reconnectMax) || 10,

        // Register a handler for a message type.
        on: function (type, fn) {
            if (!this._handlers[type]) this._handlers[type] = [];
            this._handlers[type].push(fn);
            return this;
        },

        // Unregister a handler.
        off: function (type, fn) {
            if (!this._handlers[type]) return;
            this._handlers[type] = this._handlers[type].filter(function (h) { return h !== fn; });
        },

        // Dispatch a received message to all registered handlers.
        _dispatch: function (type, payload) {
            if (this._debug) console.log('[MMO] <-', type, payload);
            var handlers = this._handlers[type];
            if (!handlers) return;
            // Copy array so handlers can safely call on/off during dispatch.
            var snapshot = handlers.slice();
            snapshot.forEach(function (fn) {
                try { fn(payload); } catch (e) { console.error('[MMO] Handler error (' + type + '):', e); }
            });
        },

        // Send a message to the server.
        send: function (type, payload) {
            if (this._state !== STATE.CONNECTED) return false;
            if (this._seq > 0xFFFFFF) this._seq = 0;
            var msg = JSON.stringify({ seq: ++this._seq, type: type, payload: payload || {} });
            if (this._debug) console.log('[MMO] ->', type, payload);
            try {
                this._ws.send(msg);
                return true;
            } catch (e) {
                console.error('[MMO] Send error:', e);
                return false;
            }
        },

        // Connect to the WebSocket server with the given token.
        connect: function (token) {
            if (this._state === STATE.CONNECTED || this._state === STATE.CONNECTING) return;
            this.token = token;
            this._doConnect();
        },

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
                if (self._debug) console.log('[MMO] Connected to server.');
            };

            ws.onmessage = function (evt) {
                try {
                    var msg = JSON.parse(evt.data);
                    self._dispatch(msg.type, msg.payload);
                } catch (e) {
                    console.error('[MMO] Parse error:', e);
                }
            };

            ws.onerror = function (e) {
                console.error('[MMO] WebSocket error:', e);
            };

            ws.onclose = function () {
                self._stopHeartbeat();
                if (self._state === STATE.CONNECTED || self._state === STATE.CONNECTING) {
                    self._ws = null;
                    self._scheduleReconnect();
                }
            };
        },

        _scheduleReconnect: function () {
            var self = this;
            if (this._reconnectAttempts >= this._reconnectMax) {
                console.error('[MMO] Max reconnection attempts reached.');
                this._state = STATE.DISCONNECTED;
                this._dispatch('_reconnect_failed', {});
                this._dispatch('_disconnected', {});
                return;
            }
            this._state = STATE.RECONNECTING;
            var delay = Math.min(1000 * Math.pow(2, this._reconnectAttempts), MAX_RECONNECT_DELAY);
            this._reconnectAttempts++;
            if (this._debug) console.log('[MMO] Reconnecting in ' + delay + 'ms (attempt ' + this._reconnectAttempts + ')');
            this._reconnectTimer = setTimeout(function () {
                self._doConnect();
            }, delay);
        },

        disconnect: function () {
            this._state = STATE.DISCONNECTED;
            this._stopHeartbeat();
            if (this._reconnectTimer) { clearTimeout(this._reconnectTimer); this._reconnectTimer = null; }
            if (this._ws) { this._ws.onclose = null; this._ws.close(); this._ws = null; }
        },

        _startHeartbeat: function () {
            var self = this;
            this._stopHeartbeat();
            this._heartbeatTimer = setInterval(function () {
                self.send('ping', { ts: Date.now() });
            }, HEARTBEAT_INTERVAL);
        },

        _stopHeartbeat: function () {
            if (this._heartbeatTimer) { clearInterval(this._heartbeatTimer); this._heartbeatTimer = null; }
        },

        isConnected: function () { return this._state === STATE.CONNECTED; },

        // Bottom-UI registry: panels hidden when RMMV message/choice is active.
        _bottomUI: [],
        _eventBusy: false,
        registerBottomUI: function (panel) {
            if (this._bottomUI.indexOf(panel) < 0) this._bottomUI.push(panel);
        },
        unregisterBottomUI: function (panel) {
            var idx = this._bottomUI.indexOf(panel);
            if (idx >= 0) this._bottomUI.splice(idx, 1);
        }
    };

    // Disable local RMMV save.
    StorageManager.save = function () {};
    StorageManager.load = function () { return null; };

    // Disable RMMV party followers — MMO has its own party system via WebSocket.
    // Default partyMembers=[1,2,3,4] would show 3 trailing follower sprites.
    Game_Followers.prototype.initialize = function () {
        this._visible = false;
        this._gathering = false;
        this._data = [];
    };

    // Only keep one RMMV actor in the party (the player avatar).
    // Default setupStartingMembers adds actors 1-4 which show in the menu.
    Game_Party.prototype.setupStartingMembers = function () {
        this._actors = [];
        this.addActor(1);
    };

    // Remove "Save" and "Formation" from the menu — MMO has no local save.
    Window_MenuCommand.prototype.addSaveCommand = function () {};
    Window_MenuCommand.prototype.addFormationCommand = function () {};

    // -----------------------------------------------------------------
    // Movement sync: send player position to server after each tile move.
    // Without this, the server never knows where the player walks and
    // saves the stale initial position on disconnect.
    // -----------------------------------------------------------------
    var _Game_Player_moveStraight = Game_Player.prototype.moveStraight;
    Game_Player.prototype.moveStraight = function (d) {
        _Game_Player_moveStraight.call(this, d);
        if (this.isMovementSucceeded() && $MMO.isConnected()) {
            $MMO.send('player_move', { x: this._x, y: this._y, dir: this._direction });
        }
    };

    var _Game_Player_moveDiagonally = Game_Player.prototype.moveDiagonally;
    Game_Player.prototype.moveDiagonally = function (horz, vert) {
        _Game_Player_moveDiagonally.call(this, horz, vert);
        if (this.isMovementSucceeded() && $MMO.isConnected()) {
            $MMO.send('player_move', { x: this._x, y: this._y, dir: this._direction });
        }
    };

    // -----------------------------------------------------------------
    // Disable client-side Transfer Player (code 201).
    // All map transfers are handled server-side: the NPC executor calls
    // enterMapRoom when it encounters command 201, which sends map_init.
    // This override is a safety net for any remaining client-side
    // interpreters (e.g. common events) — they must not trigger transfers.
    // -----------------------------------------------------------------
    Game_Interpreter.prototype.command201 = function () {
        return true; // no-op — server handles all transfers
    };

    // On map_init, transfer the player to the correct map and position from server.
    // Handles: initial login, re-login, and server-side map transfers.
    //
    // IMPORTANT: Always force setTransparent(false) here. Many RMMV games set
    // $dataSystem.optTransparent = true which makes $gamePlayer invisible on init.
    // Normally an autorun event clears this, but in MMO mode autorun events are
    // suppressed (server handles event logic). This single line prevents player
    // invisibility regardless of the game's transparency setting.
    $MMO.on('map_init', function (data) {
        if (!data || !data.self) return;
        var s = data.self;
        var mapId = s.map_id || 1;
        var x     = s.x != null ? s.x : 0;
        var y     = s.y != null ? s.y : 0;
        var dir   = s.dir || 2;

        // Store for late-loading HUD.
        $MMO._lastSelf = s;

        // Play map BGM/BGS from server data. Supplements RMMV's built-in
        // $gameMap.autoplay() to ensure correct audio even when client-side
        // map data loading hasn't completed yet.
        if (data.audio) {
            if (data.audio.bgm) AudioManager.playBgm(data.audio.bgm);
            if (data.audio.bgs) AudioManager.playBgs(data.audio.bgs);
        }

        if ($gamePlayer && $gameMap) {
            // Force player visible — handles $dataSystem.optTransparent = true.
            $gamePlayer.setTransparent(false);

            if ($gameMap.mapId() !== mapId) {
                $gamePlayer.reserveTransfer(mapId, x, y, dir, 0);
            } else {
                $gamePlayer.locate(x, y);
                $gamePlayer.setDirection(dir);
            }

            // Force refresh to apply server walk_name immediately.
            // Without this, same-map re-entry (re-login to same map) would
            // leave the player sprite stale from the previous session.
            $gamePlayer.refresh();
        }
    });

    // Handle server-initiated map transfer (fallback from NPC executor when
    // the server-side TransferFn is not available).
    $MMO.on('transfer_player', function (data) {
        if (!data || !$gamePlayer) return;
        var mapId = data.map_id || 1;
        var x     = data.x != null ? data.x : 0;
        var y     = data.y != null ? data.y : 0;
        var dir   = data.dir || 2;
        $gamePlayer.reserveTransfer(mapId, x, y, dir, 0);
    });

    // Override Game_Player.refresh so the walk sprite comes from the MMO
    // server instead of $gameParty.leader(). Without this, reserveTransfer →
    // performTransfer → refresh() would reset the sprite to the default actor.
    //
    // Also forces setTransparent(false) to ensure visibility even if refresh
    // is called during initialization before map_init arrives.
    var _GamePlayer_refresh = Game_Player.prototype.refresh;
    Game_Player.prototype.refresh = function () {
        var s = $MMO._lastSelf;
        if (s && s.walk_name) {
            this.setImage(s.walk_name, s.walk_index || 0);
            this.setTransparent(false);
        } else {
            _GamePlayer_refresh.call(this);
        }
    };

    // Handle pong response.
    $MMO.on('pong', function (payload) {
        if ($MMO._debug) console.log('[MMO] Pong, latency:', Date.now() - (payload.ts || 0), 'ms');
    });

    // Handle move_reject: server rejected a player_move due to passability or
    // speed violation. Snap the player to the server's authoritative position
    // to prevent cascading desync (every subsequent move would be rejected).
    $MMO.on('move_reject', function (data) {
        if (!data || !$gamePlayer) return;
        console.warn('[MMO] Move rejected — snapping to server position:',
            data.x, data.y, 'dir', data.dir);
        $gamePlayer.locate(data.x, data.y);
        if (data.dir) $gamePlayer.setDirection(data.dir);
    });

    // Handle generic server errors.
    $MMO.on('error', function (data) {
        console.warn('[MMO] Server error:', data && data.message);
    });

    // -----------------------------------------------------------------
    // Disconnect dialog: show alert and return to login screen.
    // -----------------------------------------------------------------
    $MMO.on('_disconnected', function () {
        // Only show the dialog if we are in-game (not on login/char-select/create).
        if (!SceneManager._scene || SceneManager._scene instanceof Scene_Title) return;
        if (typeof Scene_Login !== 'undefined' && SceneManager._scene instanceof Scene_Login) return;
        if (typeof Scene_CharacterSelect !== 'undefined' && SceneManager._scene instanceof Scene_CharacterSelect) return;
        if (typeof Scene_CharacterCreate !== 'undefined' && SceneManager._scene instanceof Scene_CharacterCreate) return;

        // Clean up state — clear server data to prevent stale sprites on re-login.
        $MMO.token = null;
        $MMO.charID = null;
        $MMO._lastSelf = null;

        alert('与服务器的连接已断开');
        SceneManager.goto(Scene_Title);
    });

    // -----------------------------------------------------------------
    // Draggable UI panels — with localStorage position persistence.
    // Usage: $MMO.makeDraggable(panel, 'key', { dragArea: {y,h}, onMove: fn })
    // Then call $MMO.updateDrag(panel) in the panel's update(). Returns true
    // while actively dragging so the caller can skip its own click handling.
    // -----------------------------------------------------------------
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
            active: false,
            pending: false,
            startX: 0, startY: 0,
            offX: 0, offY: 0,
            area: opts.dragArea || null, // { y, h } relative to panel
            onMove: opts.onMove || null
        };
    };

    $MMO.updateDrag = function (panel) {
        if (!panel._drag || !panel.visible) return false;
        var d = panel._drag;
        var tx = TouchInput.x, ty = TouchInput.y;

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
                if (!d.active && (Math.abs(tx - d.startX) + Math.abs(ty - d.startY)) > 4) {
                    d.active = true; d.pending = false;
                }
                if (d.active) {
                    panel.x = Math.max(0, Math.min(tx - d.offX, Graphics.boxWidth - panel.width));
                    panel.y = Math.max(0, Math.min(ty - d.offY, Graphics.boxHeight - panel.height));
                    if (d.onMove) d.onMove();
                    return true;
                }
            } else {
                if (d.active) {
                    try { localStorage.setItem('mmo_ui_' + d.key, JSON.stringify({ x: panel.x, y: panel.y })); } catch (e) {}
                    if (d.onMove) d.onMove();
                }
                d.active = false; d.pending = false;
            }
        }
        return d.active;
    };

    // -----------------------------------------------------------------
    // Keep $MMO._lastSelf up-to-date with player_sync data.
    // -----------------------------------------------------------------
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

    // =================================================================
    //  Window Manager — tracks open GameWindows, ESC closes topmost.
    //  NOTE: GameWindow class itself is in mmo-game-window.js (needs L2_Base).
    // =================================================================
    $MMO._gameWindows = [];

    $MMO.registerWindow = function (win) {
        if (this._gameWindows.indexOf(win) < 0) this._gameWindows.push(win);
    };

    $MMO.closeTopWindow = function () {
        for (var i = this._gameWindows.length - 1; i >= 0; i--) {
            if (this._gameWindows[i].visible) {
                this._gameWindows[i].close();
                return true;
            }
        }
        return false;
    };

    // Central action dispatch for opening/toggling windows.
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

    // =================================================================
    //  Disable RMMV menu. ESC closes topmost window / toggles system.
    //  Right-click is used for player context menus.
    // =================================================================
    Scene_Map.prototype.isMenuCalled = function () { return false; };
    Scene_Map.prototype.callMenu = function () {};

    // Keyboard shortcuts (Alt+T status, Alt+S skills, ESC close windows).
    window.addEventListener('keydown', function (e) {
        if (!(SceneManager._scene instanceof Scene_Map)) return;
        if (e.altKey && e.keyCode === 84) { // Alt+T → Status
            e.preventDefault();
            $MMO._triggerAction('status');
        }
        if (e.altKey && e.keyCode === 83) { // Alt+S → Skills
            e.preventDefault();
            $MMO._triggerAction('skills');
        }
    });

    // -----------------------------------------------------------------
    //  Prevent map-touch (character movement) when clicking on MMO UI.
    //  Uses _isMMOUI flag set on L2_Base.prototype — no L2_Base reference
    //  needed at load time, so this works even though mmo-core loads first.
    // -----------------------------------------------------------------
    var _SMpmt_core = Scene_Map.prototype.processMapTouch;
    Scene_Map.prototype.processMapTouch = function () {
        if (TouchInput.isTriggered() || TouchInput.isPressed()) {
            var tx = TouchInput.x, ty = TouchInput.y;
            var ch = this.children;
            for (var i = ch.length - 1; i >= 0; i--) {
                var c = ch[i];
                if (c && c.visible && c._isMMOUI &&
                    typeof c.isInside === 'function' && c.isInside(tx, ty)) {
                    return; // click on MMO UI — block movement
                }
            }
        }
        _SMpmt_core.call(this);
    };

    var _Scene_Map_update_core = Scene_Map.prototype.update;
    Scene_Map.prototype.update = function () {
        _Scene_Map_update_core.call(this);
        // ESC: close topmost window, or toggle system menu
        if (Input.isTriggered('cancel') || Input.isTriggered('escape')) {
            if (!$MMO.closeTopWindow()) {
                $MMO._triggerAction('system');
            }
        }
        // Hide bottom MMO UI when RMMV message/choice windows are active.
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

    // Debug interface (available in debug mode only).
    if (window.MMO_CONFIG && window.MMO_CONFIG.debug) {
        window.$MMO_DEBUG = {
            send: function (type, payload) { $MMO.send(type, payload); },
            state: function () { return $MMO._state; },
            handlers: function () { return Object.keys($MMO._handlers); }
        };
    }

})();
