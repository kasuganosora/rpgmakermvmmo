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
        this._npcDots = [];  // Array of {event_id, x, y}
        this._terrainCache = null;
        this._cachedMapId = -1;
        this._lastPx = -1;
        this._lastPy = -1;
        $MMO.makeDraggable(this, 'minimap');
    };

    // Update NPC from server sync data
    Minimap.prototype.updateNPC = function (eventId, x, y) {
        // Find existing NPC
        var found = false;
        for (var i = 0; i < this._npcDots.length; i++) {
            if (this._npcDots[i].event_id === eventId) {
                this._npcDots[i].x = x;
                this._npcDots[i].y = y;
                found = true;
                break;
            }
        }
        // Add new NPC if not found
        if (!found) {
            this._npcDots.push({ event_id: eventId, x: x, y: y });
        }
    };

    Minimap.prototype.update = function () {
        L2_Base.prototype.update.call(this);
        $MMO.updateDrag(this);
    };

    Minimap.prototype.standardPadding = function () { return 0; };

    Minimap.prototype.setPassability = function (data) {
        // Force rebuild on next refresh
        this._cachedMapId = -1;
    };

    // Build terrain using BFS from player position
    // Only reachable areas are drawn, walls and unreachable areas are transparent
    Minimap.prototype._buildTerrain = function () {
        if (!$gameMap || !$gameMap.width()) return;
        
        var mw = $gameMap.width();
        var mh = $gameMap.height();
        var cw = this.cw(), ch = this.ch();
        
        // Calculate scale to fit map within minimap
        var scaleX = cw / mw;
        var scaleY = ch / mh;
        var scale = Math.min(scaleX, scaleY);
        
        // Calculate cell size
        var cellSize = Math.max(1, Math.floor(scale));
        
        // Calculate map pixel dimensions and center offset
        var mapW = mw * cellSize;
        var mapH = mh * cellSize;
        var offsetX = Math.floor((cw - mapW) / 2);
        var offsetY = Math.floor((ch - mapH) / 2);
        
        // Create bitmap
        this._terrainCache = new Bitmap(cw, ch);
        var bmp = this._terrainCache;
        bmp.clear(); // Transparent background
        
        // Store for marker positioning
        this._cellSize = cellSize;
        this._offsetX = offsetX;
        this._offsetY = offsetY;
        
        // Build passability grid
        var passable = new Array(mw * mh);
        for (var y = 0; y < mh; y++) {
            for (var x = 0; x < mw; x++) {
                passable[y * mw + x] = $gameMap.isValid(x, y) && $gameMap.checkPassage(x, y, 0x0f);
            }
        }
        
        // BFS from player position to find reachable tiles
        var reachable = new Array(mw * mh).fill(false);
        var queue = [];
        var startX = $gamePlayer.x;
        var startY = $gamePlayer.y;
        
        if ($gameMap.isValid(startX, startY) && passable[startY * mw + startX]) {
            queue.push(startX, startY);
            reachable[startY * mw + startX] = true;
            
            var head = 0;
            while (head < queue.length) {
                var cx = queue[head++];
                var cy = queue[head++];
                var cidx = cy * mw + cx;
                
                // Four directions: up, right, down, left
                var dirs = [[0, -1], [1, 0], [0, 1], [-1, 0]];
                for (var i = 0; i < 4; i++) {
                    var nx = cx + dirs[i][0];
                    var ny = cy + dirs[i][1];
                    var nidx = ny * mw + nx;
                    
                    if (nx >= 0 && nx < mw && ny >= 0 && ny < mh && !reachable[nidx] && passable[nidx]) {
                        reachable[nidx] = true;
                        queue.push(nx, ny);
                    }
                }
            }
        }
        
        // Draw only reachable areas (green), everything else stays transparent
        for (var y = 0; y < mh; y++) {
            for (var x = 0; x < mw; x++) {
                if (!reachable[y * mw + x]) continue;
                
                var px = offsetX + x * cellSize;
                var py = offsetY + y * cellSize;
                
                if (px < 0 || py < 0 || px >= cw || py >= ch) continue;
                
                // Single green color for all passable areas
                bmp.fillRect(px, py, cellSize, cellSize, '#4a9a4a');
            }
        }
        
        this._cachedMapId = $gameMap.mapId();
    };

    Minimap.prototype.setPlayers = function (p) { this._playerDots = p; };
    Minimap.prototype.setMonsters = function (m) { this._monsterDots = m; };
    Minimap.prototype.setNPCs = function (n) { this._npcDots = n; };

    Minimap.prototype.refresh = function () {
        if (!$gameMap || !$gameMap.width()) return;

        // Rebuild terrain on map change
        if (this._cachedMapId !== $gameMap.mapId()) {
            this._buildTerrain();
        }

        var cw = this.cw(), ch = this.ch();
        var c = this.bmp();

        // Clear to transparent first
        c.clear();
        
        // Copy terrain (clamped to actual bitmap size)
        if (this._terrainCache) {
            var tw = Math.min(this._terrainCache.width, cw);
            var th = Math.min(this._terrainCache.height, ch);
            c.blt(this._terrainCache, 0, 0, tw, th, 0, 0);
        }

        // North indicator
        c.fontSize = 11;
        c.textColor = L2_Theme.textWhite;
        c.drawText('N', 0, 2, cw, 12, 'center');

        var cellSize = this._cellSize || Math.max(1, Math.floor(Math.min(cw / $gameMap.width(), ch / $gameMap.height())));
        var offsetX = this._offsetX || Math.floor((cw - $gameMap.width() * cellSize) / 2);
        var offsetY = this._offsetY || Math.floor((ch - $gameMap.height() * cellSize) / 2);

        // Self (bright green with crosshair) - use cell center
        var px = offsetX + Math.round($gamePlayer.x * cellSize + cellSize / 2);
        var py = offsetY + Math.round($gamePlayer.y * cellSize + cellSize / 2);
        
        c.fillRect(px - 6, py - 1, 13, 3, '#88FF88');
        c.fillRect(px - 1, py - 6, 3, 13, '#88FF88');
        c.fillRect(px - 2, py - 2, 5, 5, '#CCFFCC');
        c.fillRect(px - 1, py - 1, 3, 3, '#FFFFFF');

        // Other players (blue) - use cell center
        for (var i = 0; i < this._playerDots.length; i++) {
            var p = this._playerDots[i];
            var px2 = offsetX + Math.round(p.x * cellSize + cellSize / 2);
            var py2 = offsetY + Math.round(p.y * cellSize + cellSize / 2);
            c.fillRect(px2 - 1, py2 - 1, 4, 4, '#4488FF');
        }

        // Monsters (red) - use cell center
        for (var i = 0; i < this._monsterDots.length; i++) {
            var m = this._monsterDots[i];
            var mx = offsetX + Math.round(m.x * cellSize + cellSize / 2);
            var my = offsetY + Math.round(m.y * cellSize + cellSize / 2);
            c.fillRect(mx - 1, my - 1, 3, 3, '#FF4444');
        }

        // NPCs (yellow) - use cell center, 3px dot
        for (var i = 0; i < this._npcDots.length; i++) {
            var n = this._npcDots[i];
            var nx = offsetX + Math.round(n.x * cellSize + cellSize / 2);
            var ny = offsetY + Math.round(n.y * cellSize + cellSize / 2);
            c.fillRect(nx - 1, ny - 1, 3, 3, '#FFDD00');
        }
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
        
        if (this._mmoMinimap && $gameMap) {
            // Update player/monster/npc data every 15 frames
            if (Graphics.frameCount % 15 === 0) {
                var players = [];
                var monsters = [];
                var npcs = [];
                
                if (window.OtherPlayerManager && OtherPlayerManager._sprites) {
                    var sprites = OtherPlayerManager._sprites;
                    for (var id in sprites) {
                        if (sprites.hasOwnProperty(id)) {
                            var sp = sprites[id];
                            if (sp._character) {
                                players.push({ x: sp._character.x, y: sp._character.y });
                            }
                        }
                    }
                }
                
                if (window.MonsterManager && MonsterManager._sprites) {
                    var mSprites = MonsterManager._sprites;
                    for (var id in mSprites) {
                        if (mSprites.hasOwnProperty(id)) {
                            var sp = mSprites[id];
                            monsters.push({ x: sp._tileX, y: sp._tileY });
                        }
                    }
                }
                
                            this._mmoMinimap.setPlayers(players);
                this._mmoMinimap.setMonsters(monsters);
                // NPCs are updated via npc_sync event, not here
            }
            
            // Refresh minimap every frame (internal throttling in refresh)
            this._mmoMinimap.refresh();
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
        if (data.passability && SceneManager._scene && SceneManager._scene._mmoMinimap) {
            SceneManager._scene._mmoMinimap.setPassability(data.passability);
        }
        // Clear NPC list on map change
        if (SceneManager._scene && SceneManager._scene._mmoMinimap) {
            SceneManager._scene._mmoMinimap._npcDots = [];
        }
    });

    $MMO.on('npc_sync', function (data) {
        if (data && SceneManager._scene && SceneManager._scene._mmoMinimap) {
            SceneManager._scene._mmoMinimap.updateNPC(data.event_id, data.x, data.y);
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
