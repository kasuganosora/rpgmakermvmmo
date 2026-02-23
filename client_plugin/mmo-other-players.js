/*:
 * @plugindesc v1.0.0 MMO Other Players - renders other players on the map.
 * @author MMO Framework
 */

(function () {
    'use strict';

    var QUEUE_MAX = 10; // max queued moves before teleporting to latest

    // -----------------------------------------------------------------
    // Sprite_OtherPlayer
    // -----------------------------------------------------------------
    function Sprite_OtherPlayer(data) {
        this.initialize(data);
    }
    Sprite_OtherPlayer.prototype = Object.create(Sprite_Character.prototype);
    Sprite_OtherPlayer.prototype.constructor = Sprite_OtherPlayer;

    Sprite_OtherPlayer.prototype.initialize = function (data) {
        this._charData = data;
        // Create a dummy character object compatible with Sprite_Character.
        var ch = new Game_Character();
        ch.setImage(data.walk_name || 'Actor1', data.walk_index || 0);
        ch.setPosition(data.x || 0, data.y || 0);
        ch.setDirection(data.dir || 2);
        ch._moveSpeed = 4; // standard RMMV player speed
        Sprite_Character.prototype.initialize.call(this, ch);
        this._moveQueue = []; // queued {x, y, dir} from player_sync
        this._label = new Sprite_PlayerLabel(data);
        this.addChild(this._label);
    };

    Sprite_OtherPlayer.prototype.syncData = function (data) {
        this._charData = data;
        var c = this._character;

        // Compare against last queued position (or current logical position).
        var refX = c._x, refY = c._y;
        if (this._moveQueue.length > 0) {
            var last = this._moveQueue[this._moveQueue.length - 1];
            refX = last.x;
            refY = last.y;
        }
        var dx = Math.abs(data.x - refX);
        var dy = Math.abs(data.y - refY);

        // If more than 1 tile away (teleport / reset_pos) or queue overflow, snap.
        if (dx > 1 || dy > 1 || this._moveQueue.length >= QUEUE_MAX) {
            this._moveQueue = [];
            c._x = data.x;
            c._y = data.y;
            c._realX = data.x;
            c._realY = data.y;
            c.setDirection(data.dir || 2);
            return;
        }

        // Queue a normal 1-tile move.
        this._moveQueue.push({ x: data.x, y: data.y, dir: data.dir || 2 });
    };

    Sprite_OtherPlayer.prototype.update = function () {
        var c = this._character;

        // When the character finishes moving to the current tile, dequeue next.
        if (!c.isMoving() && this._moveQueue.length > 0) {
            var next = this._moveQueue.shift();
            if (next.x !== c._x || next.y !== c._y) {
                // Set logical position; _realX/_realY will catch up via updateMove.
                c._x = next.x;
                c._y = next.y;
            }
            c.setDirection(next.dir);
        }

        // Drive the character's smooth interpolation + walking animation.
        c.update();

        Sprite_Character.prototype.update.call(this);
        // Hide during scene fade-in so sprites don't appear before transition ends.
        if (OtherPlayerManager._fadeHide) this.opacity = 0;
        this._label.update();
    };

    // -----------------------------------------------------------------
    // Sprite_PlayerLabel
    // -----------------------------------------------------------------
    function Sprite_PlayerLabel(data) {
        this.initialize(data);
    }
    Sprite_PlayerLabel.prototype = Object.create(Sprite.prototype);
    Sprite_PlayerLabel.prototype.constructor = Sprite_PlayerLabel;

    Sprite_PlayerLabel.prototype.initialize = function (data) {
        Sprite.prototype.initialize.call(this);
        this._data = data;
        this.bitmap = new Bitmap(160, 40);
        this.anchor.x = 0.5;
        this.anchor.y = 1.0;
        this.y = -36;
        this._draw();
    };

    Sprite_PlayerLabel.prototype._draw = function () {
        var bmp = this.bitmap;
        bmp.clear();
        bmp.fontSize = 14;
        var color = '#FFFFFF';
        if (this._data.in_party) color = '#44FF88';
        else if (this._data.pk_mode) color = '#FF4444';
        bmp.textColor = color;
        var name = this._data.name || '';
        bmp.drawText(name, 0, 0, 160, 20, 'center');
        if (this._data.guild_name) {
            bmp.fontSize = 12;
            bmp.textColor = '#CCCCCC';
            bmp.drawText('[' + this._data.guild_name + ']', 0, 20, 160, 16, 'center');
        }
    };

    Sprite_PlayerLabel.prototype.update = function () {
        Sprite.prototype.update.call(this);
    };

    // -----------------------------------------------------------------
    // OtherPlayerManager
    // -----------------------------------------------------------------
    var OtherPlayerManager = {
        _sprites: {},
        _container: null,
        _pending: [],       // players received before Spriteset_Map is ready
        _initPlayers: null, // map_init player list — survives scene transitions
        _fadeHide: false,   // true while scene is fading in — hides sprites

        init: function (container) {
            this._container = container;
            this._sprites = {};
            // Merge saved players with any new joins received during scene transition.
            var toAdd = (this._initPlayers || []).concat(this._pending);
            this._initPlayers = null;
            this._pending = [];
            for (var i = 0; i < toAdd.length; i++) {
                this.add(toAdd[i]);
            }
        },

        add: function (data) {
            if (!data || !data.char_id) return;
            // If the tilemap container isn't ready yet, queue for later.
            if (!this._container) {
                this._pending.push(data);
                return;
            }
            if (this._sprites[data.char_id]) {
                this._sprites[data.char_id].syncData(data);
                return;
            }
            var sp = new Sprite_OtherPlayer(data);
            this._sprites[data.char_id] = sp;
            this._container.addChild(sp);
        },

        remove: function (charID) {
            var sp = this._sprites[charID];
            if (sp) {
                if (sp.parent) sp.parent.removeChild(sp);
                delete this._sprites[charID];
            }
            // Also clean from saved lists (handles removal during scene transitions).
            if (this._initPlayers) {
                this._initPlayers = this._initPlayers.filter(function (p) { return p.char_id !== charID; });
            }
            this._pending = this._pending.filter(function (p) { return p.char_id !== charID; });
        },

        update: function (data) {
            var sp = this._sprites[data.char_id];
            if (sp) sp.syncData(data);
        },

        get: function (charID) { return this._sprites[charID]; },

        clear: function () {
            var self = this;
            Object.keys(this._sprites).forEach(function (id) { self.remove(parseInt(id)); });
            this._pending = [];
        }
    };

    // -----------------------------------------------------------------
    // Hook Spriteset_Map to inject OtherPlayerManager.
    // -----------------------------------------------------------------
    var _Spriteset_Map_createCharacters = Spriteset_Map.prototype.createCharacters;
    Spriteset_Map.prototype.createCharacters = function () {
        _Spriteset_Map_createCharacters.call(this);
        OtherPlayerManager._fadeHide = true; // hide until fade-in completes
        OtherPlayerManager.init(this._tilemap);
    };

    // Clear fade-hide after the scene's fade-in finishes.
    var _Scene_Map_start_opm = Scene_Map.prototype.start;
    Scene_Map.prototype.start = function () {
        _Scene_Map_start_opm.call(this);
        // fadeSpeed() returns 24 frames = 400ms; wait a bit longer to be safe.
        var fadeMs = Math.round((this.fadeSpeed() + 4) * 1000 / 60);
        setTimeout(function () { OtherPlayerManager._fadeHide = false; }, fadeMs);
    };

    // Sprite_Character.updatePosition() handles positioning via screenX()/screenY()
    // which correctly accounts for tilemap scroll. No manual override needed.

    // -----------------------------------------------------------------
    // WebSocket message handlers
    // -----------------------------------------------------------------
    $MMO.on('player_join', function (data) {
        if (data.char_id === $MMO.charID) return; // skip self
        OtherPlayerManager.add(data);
    });

    $MMO.on('player_leave', function (data) {
        OtherPlayerManager.remove(data.char_id);
    });

    $MMO.on('player_sync', function (data) {
        if (data.char_id === $MMO.charID) return;
        if (OtherPlayerManager._sprites[data.char_id]) {
            OtherPlayerManager.update(data);
        } else if (OtherPlayerManager._initPlayers) {
            // Update saved position during scene transition (menu, battle).
            for (var i = 0; i < OtherPlayerManager._initPlayers.length; i++) {
                if (OtherPlayerManager._initPlayers[i].char_id === data.char_id) {
                    OtherPlayerManager._initPlayers[i].x = data.x;
                    OtherPlayerManager._initPlayers[i].y = data.y;
                    OtherPlayerManager._initPlayers[i].dir = data.dir;
                    break;
                }
            }
        }
    });

    $MMO.on('map_init', function (data) {
        var players = (data.players || []).filter(function (p) {
            return p.char_id !== $MMO.charID;
        });
        // Save for init() — this survives Scene_Map.terminate() → clear().
        OtherPlayerManager._initPlayers = players;
        OtherPlayerManager.clear();
        // Also try to add immediately (works for same-map re-entry).
        players.forEach(function (p) {
            OtherPlayerManager.add(p);
        });
    });

    $MMO.on('_disconnected', function () {
        OtherPlayerManager.clear();
    });

    // Preserve player data on scene transition (menu, battle), clear on map change.
    var _Scene_Map_terminate = Scene_Map.prototype.terminate;
    Scene_Map.prototype.terminate = function () {
        // Save current player state for re-entry (menu, battle).
        // map_init handler will overwrite _initPlayers when actually changing maps.
        var savedPlayers = OtherPlayerManager._initPlayers;
        if (!savedPlayers) {
            var saved = [];
            var sprites = OtherPlayerManager._sprites;
            Object.keys(sprites).forEach(function (id) {
                var sp = sprites[id];
                var c = sp._character;
                var d = {};
                for (var k in sp._charData) d[k] = sp._charData[k];
                d.x = c._x;
                d.y = c._y;
                d.dir = c.direction();
                saved.push(d);
            });
            if (saved.length > 0) savedPlayers = saved;
        }
        OtherPlayerManager.clear();
        OtherPlayerManager._initPlayers = savedPlayers;
        _Scene_Map_terminate.call(this);
    };

    // When tab returns from background, snap other players to latest position.
    document.addEventListener('visibilitychange', function () {
        if (document.hidden) return;
        Object.keys(OtherPlayerManager._sprites).forEach(function (id) {
            var sp = OtherPlayerManager._sprites[id];
            if (!sp._moveQueue || sp._moveQueue.length === 0) return;
            var last = sp._moveQueue[sp._moveQueue.length - 1];
            sp._moveQueue = [];
            var c = sp._character;
            c._x = last.x;
            c._y = last.y;
            c._realX = last.x;
            c._realY = last.y;
            c.setDirection(last.dir);
        });
    });

    // -----------------------------------------------------------------
    // Right-click context menu on other players (Party / Trade) — L2_Base
    // -----------------------------------------------------------------
    var CTX_W = 120, CTX_ITEM_H = 28, CTX_PAD = 4;
    var CTX_ITEMS = [
        { label: '组队', action: 'party' },
        { label: '交易', action: 'trade' }
    ];

    function PlayerContextMenu(x, y, charData) {
        this.initialize(x, y, charData);
    }
    PlayerContextMenu.prototype = Object.create(L2_Base.prototype);
    PlayerContextMenu.prototype.constructor = PlayerContextMenu;

    PlayerContextMenu.prototype.initialize = function (x, y, charData) {
        var h = CTX_ITEMS.length * CTX_ITEM_H + CTX_PAD * 2;
        // Clamp to screen bounds
        x = Math.min(x, Graphics.boxWidth - CTX_W);
        y = Math.min(y, Graphics.boxHeight - h);
        L2_Base.prototype.initialize.call(this, x, y, CTX_W, h);
        this._charData = charData;
        this._hoverIdx = -1;
        this._closed = false;
        this.refresh();
    };

    PlayerContextMenu.prototype.standardPadding = function () { return 0; };

    PlayerContextMenu.prototype.isClosed = function () { return this._closed; };

    PlayerContextMenu.prototype.refresh = function () {
        var c = this.bmp();
        c.clear();
        var w = this.cw(), h = this.ch();

        L2_Theme.fillRoundRect(c, 0, 0, w, h, L2_Theme.cornerRadius, 'rgba(13,13,26,0.90)');
        L2_Theme.strokeRoundRect(c, 0, 0, w, h, L2_Theme.cornerRadius, L2_Theme.borderDark);

        var self = this;
        CTX_ITEMS.forEach(function (item, i) {
            var iy = CTX_PAD + i * CTX_ITEM_H;
            if (i === self._hoverIdx) {
                c.fillRect(2, iy, w - 4, CTX_ITEM_H, L2_Theme.highlight);
            }
            c.fontSize = L2_Theme.fontSmall;
            c.textColor = L2_Theme.textWhite;
            c.drawText(item.label, CTX_PAD + 4, iy, w - CTX_PAD * 2 - 8, CTX_ITEM_H, 'left');
            if (i < CTX_ITEMS.length - 1) {
                c.fillRect(CTX_PAD, iy + CTX_ITEM_H - 1, w - CTX_PAD * 2, 1, L2_Theme.borderDark);
            }
        });
    };

    PlayerContextMenu.prototype.close = function () {
        this._closed = true;
        if (this.parent) this.parent.removeChild(this);
    };

    PlayerContextMenu.prototype.update = function () {
        L2_Base.prototype.update.call(this);
        if (this._closed) return;

        var mx = TouchInput.x - this.x, my = TouchInput.y - this.y;
        var inside = mx >= 0 && mx < CTX_W && my >= 0 && my < this.height;

        var oldHover = this._hoverIdx;
        if (inside) {
            this._hoverIdx = Math.floor((my - CTX_PAD) / CTX_ITEM_H);
            if (this._hoverIdx < 0 || this._hoverIdx >= CTX_ITEMS.length) this._hoverIdx = -1;
        } else {
            this._hoverIdx = -1;
        }
        if (this._hoverIdx !== oldHover) this.refresh();

        if (TouchInput.isTriggered()) {
            if (inside && this._hoverIdx >= 0) {
                var action = CTX_ITEMS[this._hoverIdx].action;
                if (action === 'party') {
                    $MMO.send('party_invite', { target_char_id: this._charData.char_id });
                } else if (action === 'trade') {
                    $MMO.send('trade_request', { target_char_id: this._charData.char_id });
                }
                this.close();
            } else {
                // Click outside — dismiss
                this.close();
            }
        }
    };

    // Hook Scene_Map to handle right-click on other players.
    var _Scene_Map_update_ctx = Scene_Map.prototype.update;
    Scene_Map.prototype.update = function () {
        _Scene_Map_update_ctx.call(this);
        this._updatePlayerContextMenu();
    };

    Scene_Map.prototype._updatePlayerContextMenu = function () {
        var menu = this._playerContextMenu;
        if (menu) {
            if (menu.isClosed()) {
                this._playerContextMenu = null;
            }
            return;
        }
        if (TouchInput.isCancelled()) {
            this._checkPlayerRightClick();
        }
    };

    Scene_Map.prototype._checkPlayerRightClick = function () {
        var screenX = TouchInput.x;
        var screenY = TouchInput.y;
        var tileX = $gameMap.canvasToMapX(screenX);
        var tileY = $gameMap.canvasToMapY(screenY);

        // Find other player at this tile.
        var target = null;
        var sprites = OtherPlayerManager._sprites;
        Object.keys(sprites).forEach(function (id) {
            var sp = sprites[id];
            var c = sp._character;
            if (Math.round(c._realX) === tileX && Math.round(c._realY) === tileY) {
                target = sp;
            }
        });

        if (!target) return;

        var menu = new PlayerContextMenu(screenX, screenY, target._charData);
        this.addChild(menu);
        this._playerContextMenu = menu;
    };

    window.OtherPlayerManager = OtherPlayerManager;
    window.Sprite_OtherPlayer = Sprite_OtherPlayer;

})();
