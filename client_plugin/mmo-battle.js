/*:
 * @plugindesc v2.0.0 MMO Battle - real-time combat system (L2 UI).
 * @author MMO Framework
 */

(function () {
    'use strict';

    // Disable local random encounters.
    Game_Player.prototype.updateEncounterCount = function () {};
    Game_Player.prototype.executeEncounter = function () { return false; };

    var MOVE_SPEED = 0.0625;
    var QUEUE_MAX = 10;

    // =================================================================
    //  Sprite_Monster — L2_Theme styled HP bar + name label
    // =================================================================
    function Sprite_Monster(data) { this.initialize(data); }
    Sprite_Monster.prototype = Object.create(Sprite.prototype);
    Sprite_Monster.prototype.constructor = Sprite_Monster;

    Sprite_Monster.prototype.initialize = function (data) {
        Sprite.prototype.initialize.call(this);
        this._data = data;
        this._tileX = data.x;
        this._tileY = data.y;
        this._realX = data.x;
        this._realY = data.y;
        this._moveQueue = [];
        this.bitmap = new Bitmap(48, 48);
        this._hpBar = new Sprite(new Bitmap(48, 8));
        this._hpBar.y = 42;
        this.addChild(this._hpBar);
        this._nameLabel = new Sprite(new Bitmap(100, 18));
        this._nameLabel.x = -26;
        this._nameLabel.y = -22;
        this.addChild(this._nameLabel);
        this._drawMonster();
    };

    Sprite_Monster.prototype._drawMonster = function () {
        var bmp = this.bitmap;
        bmp.clear();
        bmp.fillRect(4, 4, 40, 40, '#884400');
        bmp.fontSize = 18;
        bmp.textColor = L2_Theme.textWhite;
        bmp.drawText('M', 0, 8, 48, 24, 'center');
        this._updateHPBar();
        var nb = this._nameLabel.bitmap;
        nb.clear();
        nb.fontSize = L2_Theme.fontSmall;
        nb.textColor = L2_Theme.textGold;
        nb.drawText(this._data.name || '', 0, 0, 100, 18, 'center');
    };

    Sprite_Monster.prototype._updateHPBar = function () {
        var ratio = this._data.max_hp > 0 ? this._data.hp / this._data.max_hp : 0;
        var bmp = this._hpBar.bitmap;
        bmp.clear();
        L2_Theme.drawBar(bmp, 0, 0, 48, 8, ratio, L2_Theme.hpBg,
            ratio > 0.5 ? '#44FF44' : ratio > 0.25 ? '#FFAA00' : L2_Theme.hpFill);
    };

    Sprite_Monster.prototype.syncData = function (data) {
        this._data = data;
        var refX = this._tileX, refY = this._tileY;
        if (this._moveQueue.length > 0) {
            var last = this._moveQueue[this._moveQueue.length - 1];
            refX = last.x; refY = last.y;
        }
        var dx = Math.abs(data.x - refX);
        var dy = Math.abs(data.y - refY);
        if (dx > 1 || dy > 1 || this._moveQueue.length >= QUEUE_MAX) {
            this._moveQueue = [];
            this._tileX = data.x; this._tileY = data.y;
            this._realX = data.x; this._realY = data.y;
        } else {
            this._moveQueue.push({ x: data.x, y: data.y });
        }
        this._updateHPBar();
    };

    Sprite_Monster.prototype.update = function () {
        var moving = (this._realX !== this._tileX || this._realY !== this._tileY);
        if (!moving && this._moveQueue.length > 0) {
            var next = this._moveQueue.shift();
            this._tileX = next.x; this._tileY = next.y;
        }
        if (this._tileX < this._realX) this._realX = Math.max(this._realX - MOVE_SPEED, this._tileX);
        if (this._tileX > this._realX) this._realX = Math.min(this._realX + MOVE_SPEED, this._tileX);
        if (this._tileY < this._realY) this._realY = Math.max(this._realY - MOVE_SPEED, this._tileY);
        if (this._tileY > this._realY) this._realY = Math.min(this._realY + MOVE_SPEED, this._tileY);
        Sprite.prototype.update.call(this);
    };

    // =================================================================
    //  Sprite_DamagePopup — floating damage number (keep Sprite-based)
    // =================================================================
    function Sprite_DamagePopup(value, isCrit, isHeal) { this.initialize(value, isCrit, isHeal); }
    Sprite_DamagePopup.prototype = Object.create(Sprite.prototype);
    Sprite_DamagePopup.prototype.constructor = Sprite_DamagePopup;

    Sprite_DamagePopup.prototype.initialize = function (value, isCrit, isHeal) {
        Sprite.prototype.initialize.call(this);
        var fontSize = isCrit ? 28 : 20;
        var w = 120, h = 40;
        this.bitmap = new Bitmap(w, h);
        this.bitmap.fontSize = fontSize;
        var color = isHeal ? '#44FF88' : isCrit ? L2_Theme.textGold : L2_Theme.textWhite;
        this.bitmap.textColor = color;
        var text = isHeal ? '+' + value : isCrit ? value + '!' : String(value);
        this.bitmap.drawText(text, 0, 0, w, h, 'center');
        this.anchor.x = 0.5;
        this.anchor.y = 1.0;
        this._vy = -2.5;
        this._life = 60;
    };

    Sprite_DamagePopup.prototype.update = function () {
        Sprite.prototype.update.call(this);
        this.y += this._vy;
        this._vy *= 0.92;
        this._life--;
        this.opacity = Math.round((this._life / 60) * 255);
        if (this._life <= 0 && this.parent) this.parent.removeChild(this);
    };

    // =================================================================
    //  Sprite_MapDrop — blinking loot drop (keep Sprite-based)
    // =================================================================
    function Sprite_MapDrop(data) { this.initialize(data); }
    Sprite_MapDrop.prototype = Object.create(Sprite.prototype);
    Sprite_MapDrop.prototype.constructor = Sprite_MapDrop;

    Sprite_MapDrop.prototype.initialize = function (data) {
        Sprite.prototype.initialize.call(this);
        this._data = data;
        this._blink = 0;
        this.bitmap = new Bitmap(32, 32);
        this.bitmap.fillRect(4, 4, 24, 24, '#FFD700');
        this.bitmap.fontSize = 18;
        this.bitmap.textColor = '#000';
        this.bitmap.drawText('★', 0, 2, 32, 28, 'center');
        this.anchor.x = 0.5;
        this.anchor.y = 1.0;
    };

    Sprite_MapDrop.prototype.update = function () {
        Sprite.prototype.update.call(this);
        this._blink = (this._blink + 3) % 360;
        this.opacity = 180 + Math.round(Math.sin(this._blink * Math.PI / 180) * 75);
    };

    // =================================================================
    //  MonsterManager
    // =================================================================
    var MonsterManager = {
        _sprites: {},
        _drops: {},
        _container: null,
        _popupContainer: null,

        init: function (container, popupContainer) {
            this._container = container;
            this._popupContainer = popupContainer || container;
            this._sprites = {};
            this._drops = {};
        },

        spawnMonster: function (data) {
            if (this._sprites[data.inst_id]) return;
            var sp = new Sprite_Monster(data);
            this._sprites[data.inst_id] = sp;
            if (this._container) this._container.addChild(sp);
        },

        updateMonster: function (data) {
            var sp = this._sprites[data.inst_id];
            if (sp) sp.syncData(data);
        },

        removeMonster: function (instID) {
            var sp = this._sprites[instID];
            if (!sp) return;
            if (sp.parent) sp.parent.removeChild(sp);
            delete this._sprites[instID];
        },

        spawnDrop: function (data) {
            if (this._drops[data.drop_id]) return;
            var sp = new Sprite_MapDrop(data);
            this._drops[data.drop_id] = sp;
            if (this._container) this._container.addChild(sp);
        },

        removeDrop: function (dropID) {
            var sp = this._drops[dropID];
            if (!sp) return;
            if (sp.parent) sp.parent.removeChild(sp);
            delete this._drops[dropID];
        },

        showDamage: function (x, y, value, isCrit, isHeal) {
            var sp = new Sprite_DamagePopup(value, isCrit, isHeal);
            sp.x = x; sp.y = y;
            if (this._popupContainer) this._popupContainer.addChild(sp);
        },

        updatePositions: function () {
            if (!$gameMap) return;
            var self = this;
            var tileW = $gameMap.tileWidth();
            var tileH = $gameMap.tileHeight();
            Object.keys(this._sprites).forEach(function (id) {
                var sp = self._sprites[id];
                sp.x = (sp._realX - $gameMap.displayX() + 0.5) * tileW;
                sp.y = (sp._realY - $gameMap.displayY() + 1.0) * tileH;
            });
            Object.keys(this._drops).forEach(function (id) {
                var sp = self._drops[id];
                sp.x = (sp._data.x - $gameMap.displayX() + 0.5) * tileW;
                sp.y = (sp._data.y - $gameMap.displayY() + 1.0) * tileH;
            });
        },

        clear: function () {
            var self = this;
            Object.keys(this._sprites).forEach(function (id) { self.removeMonster(parseInt(id)); });
            Object.keys(this._drops).forEach(function (id) { self.removeDrop(parseInt(id)); });
        }
    };

    // =================================================================
    //  Hook Spriteset_Map
    // =================================================================
    var _Spriteset_Map_createCharacters2 = Spriteset_Map.prototype.createCharacters;
    Spriteset_Map.prototype.createCharacters = function () {
        _Spriteset_Map_createCharacters2.call(this);
        MonsterManager.init(this._tilemap, this._tilemap);
    };

    var _Spriteset_Map_update2 = Spriteset_Map.prototype.update;
    Spriteset_Map.prototype.update = function () {
        _Spriteset_Map_update2.call(this);
        MonsterManager.updatePositions();
    };

    // =================================================================
    //  Attack on mouse click
    // =================================================================
    var _Scene_Map_processMapTouch = Scene_Map.prototype.processMapTouch;
    Scene_Map.prototype.processMapTouch = function () {
        if (TouchInput.isTriggered()) {
            var tileX = Math.floor((TouchInput.x + $gameMap.displayX() * $gameMap.tileWidth()) / $gameMap.tileWidth());
            var tileY = Math.floor((TouchInput.y + $gameMap.displayY() * $gameMap.tileHeight()) / $gameMap.tileHeight());
            var hit = null;
            Object.keys(MonsterManager._sprites).forEach(function (id) {
                var sp = MonsterManager._sprites[id];
                if (sp._tileX === tileX && sp._tileY === tileY) hit = parseInt(id);
            });
            if (hit !== null) {
                $MMO.send('attack', { target_id: hit, target_type: 'monster' });
                return;
            }
            var dropHit = null;
            Object.keys(MonsterManager._drops).forEach(function (id) {
                var sp = MonsterManager._drops[id];
                if (sp._data.x === tileX && sp._data.y === tileY) dropHit = parseInt(id);
            });
            if (dropHit !== null) {
                $MMO.send('pickup_item', { drop_id: dropHit });
                return;
            }
        }
        _Scene_Map_processMapTouch.call(this);
    };

    // =================================================================
    //  Death overlay — L2_Dialog style
    // =================================================================
    var _deathOverlay = null;
    function showDeathOverlay() {
        if (_deathOverlay) return;
        var w = 300, h = 120;
        _deathOverlay = new L2_Base((Graphics.boxWidth - w) / 2, (Graphics.boxHeight - h) / 2, w, h);
        _deathOverlay.standardPadding = function () { return 0; };
        var c = _deathOverlay.bmp();
        L2_Theme.fillRoundRect(c, 0, 0, w, h, L2_Theme.cornerRadius, 'rgba(26,0,0,0.80)');
        L2_Theme.strokeRoundRect(c, 0, 0, w, h, L2_Theme.cornerRadius, '#FF2222');
        c.fontSize = 36;
        c.textColor = '#FF2222';
        c.drawText('YOU DIED', 0, 20, w, 50, 'center');
        c.fontSize = L2_Theme.fontSmall;
        c.textColor = L2_Theme.textGray;
        c.drawText('Awaiting revival...', 0, 72, w, 20, 'center');
        if (SceneManager._scene) SceneManager._scene.addChild(_deathOverlay);
    }
    function hideDeathOverlay() {
        if (_deathOverlay && _deathOverlay.parent) {
            _deathOverlay.parent.removeChild(_deathOverlay);
            _deathOverlay = null;
        }
    }

    // =================================================================
    //  WebSocket handlers
    // =================================================================
    $MMO.on('map_init', function (data) {
        MonsterManager.clear();
        (data.monsters || []).forEach(function (m) { MonsterManager.spawnMonster(m); });
        (data.drops || []).forEach(function (d) { MonsterManager.spawnDrop(d); });
    });

    $MMO.on('monster_spawn', function (data) { MonsterManager.spawnMonster(data); });
    $MMO.on('monster_sync', function (data) { MonsterManager.updateMonster(data); });
    $MMO.on('monster_death', function (data) {
        MonsterManager.removeMonster(data.inst_id);
        if (data.exp_gain) console.log('[Battle] Gained ' + data.exp_gain + ' EXP');
    });
    $MMO.on('drop_spawn', function (data) { MonsterManager.spawnDrop(data); });
    $MMO.on('drop_remove', function (data) { MonsterManager.removeDrop(data.drop_id); });

    $MMO.on('battle_result', function (data) {
        if ($gameMap && SceneManager._scene && SceneManager._scene._spriteset) {
            var tileW = $gameMap.tileWidth(), tileH = $gameMap.tileHeight();
            var screenX = (data.x - $gameMap.displayX() + 0.5) * tileW;
            var screenY = (data.y - $gameMap.displayY()) * tileH;
            MonsterManager.showDamage(screenX, screenY, data.damage, data.is_crit, false);
        }
    });

    $MMO.on('skill_effect', function (data) {
        if (data.damage !== undefined && $gameMap && SceneManager._scene) {
            var tileW = $gameMap.tileWidth(), tileH = $gameMap.tileHeight();
            var screenX = (data.target_x - $gameMap.displayX() + 0.5) * tileW;
            var screenY = (data.target_y - $gameMap.displayY()) * tileH;
            MonsterManager.showDamage(screenX, screenY, Math.abs(data.damage), data.is_crit, data.damage < 0);
        }
    });

    $MMO.on('player_death', function () { showDeathOverlay(); });
    $MMO.on('player_revive', function () { hideDeathOverlay(); });

    var _Scene_Map_terminate2 = Scene_Map.prototype.terminate;
    Scene_Map.prototype.terminate = function () {
        _Scene_Map_terminate2.call(this);
        MonsterManager.clear();
        hideDeathOverlay();
    };

    window.MonsterManager = MonsterManager;

})();
