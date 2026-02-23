/*:
 * @plugindesc v3.0.0 MMO HUD - L2 UI: status top-left, minimap top-right, quest tracker.
 * @author MMO Framework
 */

(function () {
    'use strict';

    // =================================================================
    //  StatusBar — top-left: name/level + HP/MP/EXP bars
    // =================================================================
    var SB_W = 230, SB_H = 100, SB_PAD = 8;

    function StatusBar() { this.initialize.apply(this, arguments); }
    StatusBar.prototype = Object.create(L2_Base.prototype);
    StatusBar.prototype.constructor = StatusBar;

    StatusBar.prototype.initialize = function () {
        L2_Base.prototype.initialize.call(this, 4, 4, SB_W, SB_H);
        this._name = $MMO.charName || '';
        this._level = 1;
        this._hp = 100; this._maxHP = 100;
        this._mp = 50;  this._maxMP = 50;
        this._exp = 0;  this._maxExp = 100;
        this.refresh();
    };

    StatusBar.prototype.standardPadding = function () { return 0; };

    StatusBar.prototype.setData = function (data) {
        if (data.name !== undefined)     this._name   = data.name;
        if (data.level !== undefined)    this._level  = data.level;
        if (data.hp !== undefined)       this._hp     = data.hp;
        if (data.max_hp !== undefined)   this._maxHP  = data.max_hp;
        if (data.mp !== undefined)       this._mp     = data.mp;
        if (data.max_mp !== undefined)   this._maxMP  = data.max_mp;
        if (data.exp !== undefined)      this._exp    = data.exp;
        if (data.next_exp !== undefined) this._maxExp = data.next_exp;
        this.refresh();
    };

    StatusBar.prototype.refresh = function () {
        var c = this.bmp();
        c.clear();
        var w = this.cw(), h = this.ch();
        var barW = w - SB_PAD * 2;

        // Background
        L2_Theme.fillRoundRect(c, 0, 0, w, h, L2_Theme.cornerRadius, 'rgba(13,13,26,0.65)');
        L2_Theme.strokeRoundRect(c, 0, 0, w, h, L2_Theme.cornerRadius, L2_Theme.borderDark);

        // Name + Level
        c.fontSize = L2_Theme.fontSmall;
        c.textColor = L2_Theme.textGold;
        c.drawText(this._level, SB_PAD, SB_PAD, 24, 16, 'left');
        c.textColor = L2_Theme.textWhite;
        c.drawText(this._name, SB_PAD + 28, SB_PAD, barW - 28, 16, 'left');

        // HP bar
        var y = SB_PAD + 22;
        var hpRatio = Math.min(this._hp / Math.max(this._maxHP, 1), 1);
        L2_Theme.drawBar(c, SB_PAD, y, barW, 18, hpRatio, L2_Theme.hpBg, L2_Theme.hpFill);
        c.fontSize = 11;
        c.textColor = L2_Theme.textWhite;
        c.drawText('HP  ' + this._hp + ' / ' + this._maxHP, SB_PAD + 4, y, barW - 8, 18, 'left');

        // MP bar
        y += 22;
        var mpRatio = Math.min(this._mp / Math.max(this._maxMP, 1), 1);
        L2_Theme.drawBar(c, SB_PAD, y, barW, 14, mpRatio, L2_Theme.mpBg, L2_Theme.mpFill);
        c.fontSize = 11;
        c.drawText('MP  ' + this._mp + ' / ' + this._maxMP, SB_PAD + 4, y, barW - 8, 14, 'left');

        // EXP bar
        y += 18;
        var expRatio = this._maxExp > 0 ? Math.min(this._exp / this._maxExp, 1) : 0;
        L2_Theme.drawBar(c, SB_PAD, y, barW, 10, expRatio, '#1a1a00', '#CCCC00');
        c.fontSize = 10;
        c.textColor = L2_Theme.textGray;
        c.drawText(Math.floor(expRatio * 100) + '%', SB_PAD, y, barW - 4, 10, 'right');
    };

    // =================================================================
    //  Minimap — top-right: terrain passability + dots
    // =================================================================
    var MM_SIZE = 120;

    function Minimap() { this.initialize.apply(this, arguments); }
    Minimap.prototype = Object.create(L2_Base.prototype);
    Minimap.prototype.constructor = Minimap;

    Minimap.prototype.initialize = function () {
        L2_Base.prototype.initialize.call(this, Graphics.boxWidth - MM_SIZE - 4, 4, MM_SIZE, MM_SIZE);
        this._playerDots = [];
        this._monsterDots = [];
        this._terrainCache = null;
        this._cachedMapId = -1;
        $MMO.makeDraggable(this, 'minimap');
    };

    Minimap.prototype.update = function () {
        L2_Base.prototype.update.call(this);
        $MMO.updateDrag(this);
    };

    Minimap.prototype.standardPadding = function () { return 0; };

    Minimap.prototype._buildTerrain = function () {
        if (!$gameMap || !$gameMap.width()) return;
        var mw = $gameMap.width(), mh = $gameMap.height();
        var cw = this.cw(), ch = this.ch();
        var scaleX = cw / mw, scaleY = ch / mh;

        this._terrainCache = new Bitmap(cw, ch);
        var bmp = this._terrainCache;
        bmp.fillRect(0, 0, cw, ch, '#0a0a1a');
        for (var y = 0; y < mh; y++) {
            for (var x = 0; x < mw; x++) {
                if ($gameMap.isPassable(x, y, 2) || $gameMap.isPassable(x, y, 4) ||
                    $gameMap.isPassable(x, y, 6) || $gameMap.isPassable(x, y, 8)) {
                    var px = Math.floor(x * scaleX);
                    var py = Math.floor(y * scaleY);
                    var pw = Math.max(1, Math.ceil(scaleX));
                    var ph = Math.max(1, Math.ceil(scaleY));
                    bmp.fillRect(px, py, pw, ph, '#1a3a1a');
                }
            }
        }
        this._cachedMapId = $gameMap.mapId();
    };

    Minimap.prototype.setPlayers = function (p) { this._playerDots = p; this.refresh(); };
    Minimap.prototype.setMonsters = function (m) { this._monsterDots = m; this.refresh(); };

    Minimap.prototype.refresh = function () {
        var c = this.bmp();
        c.clear();
        if (!$gameMap || !$gameMap.width()) return;

        if (this._cachedMapId !== $gameMap.mapId()) this._buildTerrain();

        var cw = this.cw(), ch = this.ch();
        var mw = $gameMap.width(), mh = $gameMap.height();
        var scaleX = cw / mw, scaleY = ch / mh;

        // Terrain
        if (this._terrainCache) {
            c.blt(this._terrainCache, 0, 0, cw, ch, 0, 0);
        } else {
            c.fillRect(0, 0, cw, ch, '#222');
        }

        // Border
        L2_Theme.strokeRoundRect(c, 0, 0, cw, ch, L2_Theme.cornerRadius, L2_Theme.borderDark);

        // North indicator
        c.fontSize = 11;
        c.textColor = L2_Theme.textWhite;
        c.drawText('N', 0, 2, cw, 12, 'center');

        // Self (green)
        var px = Math.round($gamePlayer.x * scaleX);
        var py = Math.round($gamePlayer.y * scaleY);
        c.fillRect(px - 2, py - 2, 5, 5, '#44FF44');

        // Other players (blue)
        this._playerDots.forEach(function (p) {
            c.fillRect(Math.round(p.x * scaleX) - 1, Math.round(p.y * scaleY) - 1, 4, 4, '#4488FF');
        });

        // Monsters (red)
        this._monsterDots.forEach(function (m) {
            c.fillRect(Math.round(m.x * scaleX) - 1, Math.round(m.y * scaleY) - 1, 3, 3, '#FF4444');
        });
    };

    // =================================================================
    //  QuestTracker — right side, below minimap
    // =================================================================
    var QT_W = 196, QT_H = 140, QT_PAD = 6;

    function QuestTracker() { this.initialize.apply(this, arguments); }
    QuestTracker.prototype = Object.create(L2_Base.prototype);
    QuestTracker.prototype.constructor = QuestTracker;

    QuestTracker.prototype.initialize = function () {
        L2_Base.prototype.initialize.call(this, Graphics.boxWidth - QT_W - 4, MM_SIZE + 12, QT_W, QT_H);
        this._quests = [];
    };

    QuestTracker.prototype.standardPadding = function () { return 0; };

    QuestTracker.prototype.setQuests = function (quests) {
        this._quests = quests.slice(0, 3);
        this.refresh();
    };

    QuestTracker.prototype.refresh = function () {
        var c = this.bmp();
        c.clear();
        var w = this.cw(), h = this.ch();

        // Background
        L2_Theme.fillRoundRect(c, 0, 0, w, h, L2_Theme.cornerRadius, 'rgba(13,13,26,0.50)');
        L2_Theme.strokeRoundRect(c, 0, 0, w, h, L2_Theme.cornerRadius, L2_Theme.borderDark);

        this._quests.forEach(function (q, i) {
            var y = QT_PAD + i * 44;
            c.fontSize = L2_Theme.fontSmall;
            c.textColor = q.completed ? '#44FF88' : L2_Theme.textWhite;
            c.drawText(q.name, QT_PAD, y, w - QT_PAD * 2, 18, 'left');
            if (q.objectives && q.objectives.length > 0) {
                c.fontSize = 11;
                c.textColor = L2_Theme.textGray;
                var obj = q.objectives[0];
                c.drawText(obj.label + ': ' + obj.current + '/' + obj.required,
                    QT_PAD, y + 18, w - QT_PAD * 2, 14, 'left');
            }
        });
    };

    // =================================================================
    //  Inject into Scene_Map
    // =================================================================
    var _Scene_Map_createAllWindows = Scene_Map.prototype.createAllWindows;
    Scene_Map.prototype.createAllWindows = function () {
        _Scene_Map_createAllWindows.call(this);
        this._mmoStatusBar = new StatusBar();
        this._mmoMinimap = new Minimap();
        this._mmoQuestTrack = new QuestTracker();
        this.addChild(this._mmoStatusBar);
        this.addChild(this._mmoMinimap);
        this.addChild(this._mmoQuestTrack);
        if ($MMO._lastSelf) this._mmoStatusBar.setData($MMO._lastSelf);
    };

    var _Scene_Map_update2 = Scene_Map.prototype.update;
    Scene_Map.prototype.update = function () {
        _Scene_Map_update2.call(this);
        if (this._mmoMinimap && $gameMap && Graphics.frameCount % 30 === 0) {
            var players = window.OtherPlayerManager ? Object.values(OtherPlayerManager._sprites).map(function (sp) {
                return { x: sp._character.x, y: sp._character.y };
            }) : [];
            var monsters = window.MonsterManager ? Object.keys(MonsterManager._sprites).map(function (id) {
                var sp = MonsterManager._sprites[id];
                return { x: sp._tileX, y: sp._tileY };
            }) : [];
            this._mmoMinimap.setPlayers(players);
            this._mmoMinimap.setMonsters(monsters);
        }
    };

    // =================================================================
    //  WebSocket handlers
    // =================================================================
    $MMO.on('player_sync', function (data) {
        if (data.char_id !== $MMO.charID) return;
        if (SceneManager._scene && SceneManager._scene._mmoStatusBar) {
            SceneManager._scene._mmoStatusBar.setData(data);
        }
    });

    $MMO.on('map_init', function (data) {
        if (data.self && SceneManager._scene && SceneManager._scene._mmoStatusBar) {
            SceneManager._scene._mmoStatusBar.setData(data.self);
        }
    });

    $MMO.on('exp_gain', function (data) {
        if (!data) return;
        if (SceneManager._scene && SceneManager._scene._mmoStatusBar) {
            var update = {};
            if (data.total_exp !== undefined) update.exp = data.total_exp;
            if (data.level !== undefined)     update.level = data.level;
            SceneManager._scene._mmoStatusBar.setData(update);
        }
    });

    $MMO.on('quest_update', function (data) {
        if (!data) return;
        $MMO._trackedQuests = $MMO._trackedQuests || [];
        var found = false;
        $MMO._trackedQuests.forEach(function (q, i) {
            if (q.quest_id === data.quest_id) {
                $MMO._trackedQuests[i] = data;
                found = true;
            }
        });
        if (!found) $MMO._trackedQuests.push(data);
        if (SceneManager._scene && SceneManager._scene._mmoQuestTrack) {
            SceneManager._scene._mmoQuestTrack.setQuests($MMO._trackedQuests);
        }
    });

    window.Window_MMO_StatusBar = StatusBar;
    window.Window_Minimap = Minimap;
    window.Window_QuestTrack = QuestTracker;

})();
