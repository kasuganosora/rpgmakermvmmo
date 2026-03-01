/*:
 * @plugindesc v2.0.0 MMO NPC - server-controlled NPC rendering and interaction.
 * @author MMO Framework
 *
 * @help
 * All events are fully server-authoritative. The client:
 *   - Does NOT create Game_Event objects (setupEvents clears _events)
 *   - Renders NPCs via Sprite_ServerNPC from server data (map_init.npcs)
 *   - Syncs NPC positions from npc_sync messages
 *   - Sends npc_interact to server on action button press near an NPC
 *   - Displays dialog/choices from server npc_dialog / npc_choices messages
 *   - Transfer events are detected server-side in HandleMove
 *
 * This ensures zero client-side event processing — the client is a pure
 * rendering layer for server state.
 */

(function () {
    'use strict';

    var QUEUE_MAX = 10;

    // -----------------------------------------------------------------
    // Sprite_ServerNPC — renders a server-controlled NPC on the map.
    // -----------------------------------------------------------------
    function Sprite_ServerNPC(data) {
        this.initialize(data);
    }
    Sprite_ServerNPC.prototype = Object.create(Sprite_Character.prototype);
    Sprite_ServerNPC.prototype.constructor = Sprite_ServerNPC;

    Sprite_ServerNPC.prototype.initialize = function (data) {
        this._npcData = data;
        var ch = new Game_Character();
        var walkName = data.walk_name || '';
        var walkIndex = data.walk_index || 0;
        var tileId = data.tile_id || 0;

        if (tileId > 0) {
            // Tile-based event (e.g. doors, bookshelves): use setTileImage so
            // Sprite_Character renders the correct tileset graphic.
            ch.setTileImage(tileId);
        } else {
            ch.setImage(walkName, walkIndex);
        }
        ch.setPosition(data.x || 0, data.y || 0);
        ch.setDirection(data.dir || 2);
        ch._moveSpeed = 3; // standard NPC speed
        ch._priorityType = data.priority_type != null ? data.priority_type : 1;
        ch._stepAnime = !!data.step_anime;
        ch._directionFix = !!data.direction_fix;
        ch._through = !!data.through;
        ch._walkAnime = data.walk_anime != null ? data.walk_anime : true;
        Sprite_Character.prototype.initialize.call(this, ch);
        this._moveQueue = [];
        // Hide NPCs with no sprite image (invisible events like transfer triggers).
        if (!walkName && tileId === 0) this.visible = false;
    };

    Sprite_ServerNPC.prototype.syncData = function (data) {
        this._npcData = data;
        var c = this._character;

        var refX = c._x, refY = c._y;
        if (this._moveQueue.length > 0) {
            var last = this._moveQueue[this._moveQueue.length - 1];
            refX = last.x;
            refY = last.y;
        }
        var dx = Math.abs(data.x - refX);
        var dy = Math.abs(data.y - refY);

        if (dx > 1 || dy > 1 || this._moveQueue.length >= QUEUE_MAX) {
            this._moveQueue = [];
            c._x = data.x;
            c._y = data.y;
            c._realX = data.x;
            c._realY = data.y;
            c.setDirection(data.dir || 2);
            return;
        }

        if (data.x !== refX || data.y !== refY) {
            this._moveQueue.push({ x: data.x, y: data.y, dir: data.dir || 2 });
        } else if (data.dir && data.dir !== c.direction()) {
            c.setDirection(data.dir);
        }
    };

    Sprite_ServerNPC.prototype.pageChange = function (data) {
        var c = this._character;
        var walkName = data.walk_name || '';
        var walkIndex = data.walk_index || 0;
        var tileId = data.tile_id || 0;
        if (tileId > 0) {
            c.setTileImage(tileId);
        } else {
            c.setImage(walkName, walkIndex);
        }
        if (data.dir) c.setDirection(data.dir);
        c._priorityType = data.priority_type != null ? data.priority_type : 1;
        c._stepAnime = !!data.step_anime;
        c._directionFix = !!data.direction_fix;
        c._through = !!data.through;
        c._walkAnime = data.walk_anime != null ? data.walk_anime : true;
        this.visible = !!walkName || tileId > 0;
    };

    Sprite_ServerNPC.prototype.update = function () {
        var c = this._character;
        if (!c.isMoving() && this._moveQueue.length > 0) {
            var next = this._moveQueue.shift();
            if (next.x !== c._x || next.y !== c._y) {
                c._x = next.x;
                c._y = next.y;
            }
            c.setDirection(next.dir);
        }
        c.update();
        Sprite_Character.prototype.update.call(this);
    };

    // -----------------------------------------------------------------
    // NPCManager — manages all server-controlled NPC sprites.
    // -----------------------------------------------------------------
    var NPCManager = {
        _sprites: {},    // event_id → Sprite_ServerNPC
        _container: null,
        _pending: [],
        _initNPCs: null, // NPC list from map_init — survives scene transitions

        init: function (container) {
            this._container = container;
            this._sprites = {};
            var toAdd = (this._initNPCs || []).concat(this._pending);
            this._initNPCs = null;
            this._pending = [];
            for (var i = 0; i < toAdd.length; i++) {
                this._addOne(toAdd[i]);
            }
        },

        populate: function (npcList) {
            this._initNPCs = npcList;
            this.clear();
            for (var i = 0; i < npcList.length; i++) {
                this._addOne(npcList[i]);
            }
        },

        _addOne: function (data) {
            if (!data || !data.event_id) return;
            if (!this._container) {
                this._pending.push(data);
                return;
            }
            if (this._sprites[data.event_id]) {
                this._sprites[data.event_id].syncData(data);
                return;
            }
            var sp = new Sprite_ServerNPC(data);
            this._sprites[data.event_id] = sp;
            this._container.addChild(sp);
        },

        sync: function (data) {
            var sp = this._sprites[data.event_id];
            if (sp) sp.syncData(data);
        },

        pageChange: function (data) {
            var sp = this._sprites[data.event_id];
            if (sp) sp.pageChange(data);
        },

        get: function (eventID) {
            return this._sprites[eventID];
        },

        clear: function () {
            var self = this;
            Object.keys(this._sprites).forEach(function (id) {
                var sp = self._sprites[id];
                if (sp && sp.parent) sp.parent.removeChild(sp);
            });
            this._sprites = {};
            this._pending = [];
        }
    };

    // -----------------------------------------------------------------
    // Hook Spriteset_Map to inject NPCManager
    // -----------------------------------------------------------------
    var _Spriteset_Map_createCharacters_npc = Spriteset_Map.prototype.createCharacters;
    Spriteset_Map.prototype.createCharacters = function () {
        _Spriteset_Map_createCharacters_npc.call(this);
        NPCManager.init(this._tilemap);
    };

    // Preserve NPC data on scene transition (menu, battle).
    var _Scene_Map_terminate_npc = Scene_Map.prototype.terminate;
    Scene_Map.prototype.terminate = function () {
        var savedNPCs = NPCManager._initNPCs;
        if (!savedNPCs) {
            var saved = [];
            var sprites = NPCManager._sprites;
            Object.keys(sprites).forEach(function (id) {
                var sp = sprites[id];
                var c = sp._character;
                var d = {};
                for (var k in sp._npcData) d[k] = sp._npcData[k];
                d.x = c._x;
                d.y = c._y;
                d.dir = c.direction();
                saved.push(d);
            });
            if (saved.length > 0) savedNPCs = saved;
        }
        NPCManager.clear();
        NPCManager._initNPCs = savedNPCs;
        _Scene_Map_terminate_npc.call(this);
    };

    // -----------------------------------------------------------------
    // Disable client-side RMMV event processing (server-authoritative).
    //
    // All events are rendered via Sprite_ServerNPC from server data.
    // Transfer events are detected server-side in HandleMove.
    // -----------------------------------------------------------------

    // Override setupEvents: skip creating Game_Event objects entirely.
    var _Game_Map_setupEvents = Game_Map.prototype.setupEvents;
    Game_Map.prototype.setupEvents = function () {
        // Keep common events (for any client-side parallel common events).
        this._commonEvents = this.parallelCommonEvents().map(function (ce) {
            return new Game_CommonEvent(ce.id);
        });
        // Empty the _events array — server controls all map events.
        // With _events = [], the prototype tileEvents() method (used by
        // checkPassage / isPassable / minimap) naturally returns [] without
        // needing to override. Do NOT assign this.tileEvents = [] as that
        // replaces the prototype function with an array, breaking
        // $gameMap.tileEvents(x,y) calls and corrupting the minimap.
        this._events = [];
    };

    // ---- Bypass ALL RMMV event-triggering gates ----
    // RMMV's event trigger chain passes through 100+ plugin overrides
    // (MKR_PlayerMoveForbid always returns false, CommonInterceptor keeps
    // $gameMap.isEventRunning() true, etc.) that silently block interactions.
    // In MMO mode we bypass the ENTIRE chain and check NPCs directly.

    // updateNonmoving: handle BOTH player-touch (trigger 1/2) when the
    // player finishes moving, AND action-button (trigger 0) when OK pressed.
    // Does NOT call triggerAction/triggerButtonAction/checkEventTriggerThere
    // which go through the broken 157-plugin override chain.
    Game_Player.prototype.updateNonmoving = function (wasMoving) {
        if (wasMoving) {
            $gameParty.onPlayerWalk();
            // Player Touch / Event Touch: check current tile after moving.
            this.startMapEvent(this.x, this.y, [1, 2], false);
        }
        // Action Button: direct OK-press check — bypasses triggerAction,
        // canMove, triggerButtonAction, and ALL plugin wrappers.
        if (!$gameMessage.isBusy() && Input.isTriggered('ok')) {
            var dir = this.direction();
            var x2 = $gameMap.roundXWithDirection(this.x, dir);
            var y2 = $gameMap.roundYWithDirection(this.y, dir);
            // Check the tile the player is facing (most common case).
            this.startMapEvent(x2, y2, [0, 1, 2], true);
            // Also check the player's own tile (trigger=0 events AT player position).
            this.startMapEvent(this.x, this.y, [0], false);
        }
        if (wasMoving) {
            this.updateEncounterCount();
        } else {
            $gameTemp.clearDestination();
        }
    };

    // canStartLocalEvents — still needed by any remaining RMMV code paths.
    Game_Player.prototype.canStartLocalEvents = function () {
        return true;
    };

    // canMove — block movement during dialogs but don't block event triggering.
    Game_Player.prototype.canMove = function () {
        if ($gameMessage.isBusy()) return false;
        if (this.isMoveRouteForcing() || this.areFollowersGathering()) return false;
        if (this._waitCount > 0) return false;
        return true;
    };

    // startMapEvent: send npc_interact to server instead of running the
    // client-side event interpreter. Called directly from updateNonmoving.
    Game_Player.prototype.startMapEvent = function (x, y, triggers, normal) {
        if ($gameMessage.isBusy()) {
            if (MMO_CONFIG.debug) console.log('[MMO-NPC] startMapEvent blocked: $gameMessage.isBusy()');
            return;
        }
        var sprites = NPCManager._sprites;
        var ids = Object.keys(sprites);
        for (var i = 0; i < ids.length; i++) {
            var sp = sprites[ids[i]];
            if (!sp) continue;
            var c = sp._character;
            if (Math.round(c._realX) === x && Math.round(c._realY) === y) {
                if (triggers.indexOf(0) >= 0 || triggers.indexOf(1) >= 0 || triggers.indexOf(2) >= 0) {
                    $MMO.send('npc_interact', { event_id: sp._npcData.event_id });
                    return;
                }
            }
        }
        if (MMO_CONFIG.debug) {
            console.log('[MMO-NPC] startMapEvent: no NPC at', x, y,
                '(total sprites:', ids.length + ')');
        }
    };

    // Extend collision check to include server NPC sprites.
    // Players cannot walk through NPCs with same-priority (priorityType=1).
    var _Game_CharacterBase_isCollidedWithCharacters = Game_CharacterBase.prototype.isCollidedWithCharacters;
    Game_CharacterBase.prototype.isCollidedWithCharacters = function (x, y) {
        var result = _Game_CharacterBase_isCollidedWithCharacters.call(this, x, y);
        if (result) return true;
        var sprites = NPCManager._sprites;
        var ids = Object.keys(sprites);
        for (var i = 0; i < ids.length; i++) {
            var sp = sprites[ids[i]];
            if (!sp || !sp.visible) continue;
            var c = sp._character;
            if (c._priorityType !== 1) continue;
            if (c._through) continue;
            if (Math.round(c._realX) === x && Math.round(c._realY) === y) {
                return true;
            }
        }
        return false;
    };

    // -----------------------------------------------------------------
    // Server dialog display — npc_dialog / npc_choices / npc_dialog_end
    // -----------------------------------------------------------------

    $MMO._npcDialogActive = false;

    $MMO.on('npc_dialog', function (data) {
        if (!data) return;
        $MMO._npcDialogActive = true;
        $MMO._npcDialogPending = true;
        var face = data.face || '';
        var faceIndex = data.face_index || 0;
        var background = data.background != null ? data.background : 0;
        var positionType = data.position_type != null ? data.position_type : 2;
        var lines = data.lines || [];

        // Ensure we're on Scene_Map before showing dialog
        if (!(SceneManager._scene instanceof Scene_Map)) {
            console.warn('[MMO-NPC] Received npc_dialog but not on Scene_Map, queueing');
            $MMO._queuedDialog = data;
            return;
        }

        $gameMessage.setFaceImage(face, faceIndex);
        $gameMessage.setBackground(background);
        $gameMessage.setPositionType(positionType);
        for (var i = 0; i < lines.length; i++) {
            $gameMessage.add(lines[i]);
        }
    });

    // Process any queued dialogs when entering Scene_Map
    var _Scene_Map_start_npc = Scene_Map.prototype.start;
    Scene_Map.prototype.start = function () {
        _Scene_Map_start_npc.call(this);
        // Send scene_ready so the server knows it can start autorun events.
        if ($MMO && $MMO.send) {
            $MMO.send('scene_ready', {});
        }
        // Process queued dialog if any
        if ($MMO._queuedDialog) {
            var data = $MMO._queuedDialog;
            $MMO._queuedDialog = null;
            $MMO._npcDialogActive = true;
            $MMO._npcDialogPending = true;
            $gameMessage.setFaceImage(data.face || '', data.face_index || 0);
            $gameMessage.setBackground(data.background != null ? data.background : 0);
            $gameMessage.setPositionType(data.position_type != null ? data.position_type : 2);
            var lines = data.lines || [];
            for (var i = 0; i < lines.length; i++) {
                $gameMessage.add(lines[i]);
            }
            // If queued data also has choices (npc_dialog_choices), set them up too.
            if (data.choices && data.choices.length > 0) {
                if (!$gameMessage.hasText()) $gameMessage.add('');
                var cd = data.choice_default || 0;
                var cc = data.choice_cancel != null ? data.choice_cancel : -1;
                $gameMessage.setChoices(data.choices, cd, cc);
                if ($gameMessage.setChoicePositionType) {
                    $gameMessage.setChoicePositionType(data.choice_position != null ? data.choice_position : 2);
                }
                if ($gameMessage.setChoiceBackground) {
                    $gameMessage.setChoiceBackground(data.choice_background || 0);
                }
                $gameMessage.setChoiceCallback(function (index) {
                    $MMO.send('npc_choice_reply', { choice_index: index });
                });
            }
        }
    };

    // Combined text + choices (RMMV: Show Text followed by Show Choices).
    // Server sends both together so the text stays visible while choices show.
    $MMO.on('npc_dialog_choices', function (data) {
        if (!data) return;
        $MMO._npcDialogActive = true;
        var face = data.face || '';
        var faceIndex = data.face_index || 0;
        var background = data.background != null ? data.background : 0;
        var positionType = data.position_type != null ? data.position_type : 2;
        var lines = data.lines || [];
        var choices = data.choices || [];
        var choiceDefault = data.choice_default || 0;
        var choiceCancel = data.choice_cancel != null ? data.choice_cancel : -1;
        var choicePosition = data.choice_position != null ? data.choice_position : 2;
        var choiceBg = data.choice_background || 0;

        if (!(SceneManager._scene instanceof Scene_Map)) {
            $MMO._queuedDialog = data;
            return;
        }

        $gameMessage.setFaceImage(face, faceIndex);
        $gameMessage.setBackground(background);
        $gameMessage.setPositionType(positionType);
        for (var i = 0; i < lines.length; i++) {
            $gameMessage.add(lines[i]);
        }
        if (!$gameMessage.hasText()) {
            $gameMessage.add('');
        }
        $gameMessage.setChoices(choices, choiceDefault, choiceCancel);
        if ($gameMessage.setChoicePositionType) {
            $gameMessage.setChoicePositionType(choicePosition);
        }
        if ($gameMessage.setChoiceBackground) {
            $gameMessage.setChoiceBackground(choiceBg);
        }
        $gameMessage.setChoiceCallback(function (index) {
            $MMO.send('npc_choice_reply', { choice_index: index });
        });
    });

    $MMO.on('npc_choices', function (data) {
        if (!data || !data.choices) return;
        $MMO._npcDialogActive = true;
        var choices = data.choices;
        var choiceDefault = data.default_type || 0;
        var choiceCancel = data.cancel_type != null ? data.cancel_type : -1;
        var choicePosition = data.position_type != null ? data.position_type : 2;
        var choiceBg = data.background || 0;

        // RMMV requires at least one text line to trigger the message window.
        if (!$gameMessage.hasText()) {
            $gameMessage.add('');
        }
        $gameMessage.setChoices(choices, choiceDefault, choiceCancel);
        if ($gameMessage.setChoicePositionType) {
            $gameMessage.setChoicePositionType(choicePosition);
        }
        if ($gameMessage.setChoiceBackground) {
            $gameMessage.setChoiceBackground(choiceBg);
        }
        $gameMessage.setChoiceCallback(function (index) {
            $MMO.send('npc_choice_reply', { choice_index: index });
        });
    });

    $MMO.on('npc_dialog_end', function () {
        $MMO._npcDialogActive = false;
        $MMO._npcDialogPending = false;
    });

    // Hook terminateMessage to send dialog acknowledgment to server.
    // This fires when the player presses OK to dismiss a text dialog.
    var _Window_Message_terminateMessage = Window_Message.prototype.terminateMessage;
    Window_Message.prototype.terminateMessage = function () {
        _Window_Message_terminateMessage.call(this);
        if ($MMO._npcDialogPending) {
            $MMO._npcDialogPending = false;
            $MMO.send('npc_dialog_ack', {});
        }
    };

    // -----------------------------------------------------------------
    // Server-forwarded visual/audio effects — npc_effect
    //
    // The server forwards RMMV command codes that have visual/audio side
    // effects. The client executes them using RMMV's built-in APIs.
    // Command codes match the RMMV event command IDs.
    // -----------------------------------------------------------------

    $MMO.on('npc_effect', function (data) {
        if (!data) return;
        var code = data.code;
        var p = data.params || [];

        switch (code) {
        // --- Screen effects ---
        case 221: // Fadeout Screen
            $gameScreen.startFadeOut(paramInt(p, 0) || 30);
            break;
        case 222: // Fadein Screen
            $gameScreen.startFadeIn(paramInt(p, 0) || 30);
            break;
        case 223: // Tint Screen — [tone_array, duration, wait]
            var tone = p[0] || [0, 0, 0, 0];
            $gameScreen.startTint(tone, paramInt(p, 1) || 60);
            break;
        case 224: // Flash Screen — [color_array, duration, wait]
            var color = p[0] || [255, 255, 255, 170];
            $gameScreen.startFlash(color, paramInt(p, 1) || 30);
            break;
        case 225: // Shake Screen — [power, speed, duration, wait]
            $gameScreen.startShake(paramInt(p, 0) || 5, paramInt(p, 1) || 5, paramInt(p, 2) || 30);
            break;

        // --- Audio ---
        case 241: // Play BGM — [{name, volume, pitch, pan}]
            if (p[0]) AudioManager.playBgm(p[0]);
            break;
        case 242: // Stop BGM
            AudioManager.stopBgm();
            break;
        case 245: // Play BGS — [{name, volume, pitch, pan}]
            if (p[0]) AudioManager.playBgs(p[0]);
            break;
        case 246: // Stop BGS
            AudioManager.stopBgs();
            break;
        case 249: // Play ME — [{name, volume, pitch, pan}]
            if (p[0]) AudioManager.playMe(p[0]);
            break;
        case 250: // Play SE — [{name, volume, pitch, pan}]
            if (p[0]) AudioManager.playSe(p[0]);
            break;
        case 251: // Stop SE
            AudioManager.stopSe();
            break;

        // --- Show/Hide animation (code 211) ---
        case 211: // Change Screen Color Tone / show-hide
            // In RMMV code 211 is "Change Transparency" for events.
            // params: [0]=character_id, [1]=opacity or transparency flag
            break;

        // --- Move Route (code 205) ---
        case 205:
            // params: [0]=character_id (-1=player, 0=this_event, N=event_id), [1]=moveRoute
            // Server resolves charId=0 to actual event_id before sending.
            var charId = paramInt(p, 0);
            var moveRoute = p[1];
            if (!moveRoute) break;
            if (charId === -1 && $gamePlayer) {
                // Force move route on the player character.
                $gamePlayer.forceMoveRoute(moveRoute);
            } else if (charId > 0) {
                // NPC move route: find the NPC sprite and apply to its character.
                var npcSprite = NPCManager.get(charId);
                if (npcSprite && npcSprite._character) {
                    npcSprite._character.forceMoveRoute(moveRoute);
                } else if (MMO_CONFIG.debug) {
                    console.warn('[MMO-NPC] Move route: NPC sprite not found for event_id=' + charId);
                }
            }
            break;

        // --- Pictures ---
        case 231: // Show Picture
            // params: [pictureId, name, origin, directDesignation?, x, y, scaleX, scaleY, opacity, blendMode]
            // Server already resolves variable-based coordinates (designation=1→0).
            if ($gameScreen) {
                $gameScreen.showPicture(
                    paramInt(p, 0),             // pictureId
                    (p[1] || '').toString(),     // name
                    paramInt(p, 2),             // origin
                    paramInt(p, 4),             // x
                    paramInt(p, 5),             // y
                    p[6] != null ? paramInt(p, 6) : 100,   // scaleX
                    p[7] != null ? paramInt(p, 7) : 100,   // scaleY
                    p[8] != null ? paramInt(p, 8) : 255,   // opacity
                    paramInt(p, 9)              // blendMode
                );
            }
            break;
        case 232: // Move Picture
            // params: [pictureId, origin, directDesignation?, x, y, scaleX, scaleY, opacity, blendMode, duration, wait]
            // Server already resolves variable-based coordinates.
            if ($gameScreen) {
                $gameScreen.movePicture(
                    paramInt(p, 0),             // pictureId
                    paramInt(p, 1),             // origin
                    paramInt(p, 3),             // x
                    paramInt(p, 4),             // y
                    p[5] != null ? paramInt(p, 5) : 100,   // scaleX
                    p[6] != null ? paramInt(p, 6) : 100,   // scaleY
                    p[7] != null ? paramInt(p, 7) : 255,   // opacity
                    paramInt(p, 8),             // blendMode
                    paramInt(p, 9) || 1         // duration
                );
            }
            break;
        case 233: // Rotate Picture
            if ($gameScreen) {
                $gameScreen.rotatePicture(paramInt(p, 0), paramInt(p, 1));
            }
            break;
        case 234: // Tint Picture — [pictureId, tone, duration, wait]
            if ($gameScreen) {
                $gameScreen.tintPicture(paramInt(p, 0), p[1] || [0,0,0,0], paramInt(p, 2) || 1);
            }
            break;
        case 235: // Erase Picture
            if ($gameScreen) {
                $gameScreen.erasePicture(paramInt(p, 0));
            }
            break;

        // --- Show Animation / Balloon ---
        case 212: // Show Animation — [charId, animationId, wait]
            var animCharId = paramInt(p, 0);
            var animTarget = null;
            if (animCharId === -1 && $gamePlayer) {
                animTarget = $gamePlayer;
            } else if (animCharId > 0) {
                var animNpc = NPCManager.get(animCharId);
                if (animNpc) animTarget = animNpc._character;
            }
            if (animTarget) animTarget.requestAnimation(paramInt(p, 1));
            break;
        case 213: // Show Balloon Icon — [charId, balloonId, wait]
            var balloonCharId = paramInt(p, 0);
            var balloonTarget = null;
            if (balloonCharId === -1 && $gamePlayer) {
                balloonTarget = $gamePlayer;
            } else if (balloonCharId > 0) {
                var balloonNpc = NPCManager.get(balloonCharId);
                if (balloonNpc) balloonTarget = balloonNpc._character;
            }
            if (balloonTarget) balloonTarget.requestBalloon(paramInt(p, 1));
            break;

        // --- Erase Event ---
        case 214:
            // Server-erased events — handled via NPC visibility system.
            break;

        // --- Stat changes (forwarded from server) ---
        case 311: // Change HP
        case 312: // Change MP
        case 313: // Change State
        case 314: // Recover All
        case 315: // Change EXP
        case 316: // Change Level
        case 317: // Change Parameter
        case 318: // Change Skill
        case 319: // Change Equipment
        case 320: // Change Name
        case 321: // Change Class
        case 322: // Change Actor Images
            // Execute via Game_Interpreter so all RMMV side-effects fire.
            try {
                var si = new Game_Interpreter();
                si._eventId = 0;
                si._mapId = $gameMap ? $gameMap.mapId() : 0;
                // Build a command list with just this command + terminator.
                si._list = [
                    { code: code, indent: 0, parameters: p },
                    { code: 0, indent: 0, parameters: [] }
                ];
                si._index = 0;
                si.executeCommand();
            } catch (ex) {
                console.warn('[MMO-NPC] Stat command ' + code + ' error:', ex.message);
            }
            break;

        // --- Battle / Shop / Game Over ---
        case 301: // Battle Processing — fallback if server sends to client
            try {
                var bi = new Game_Interpreter();
                bi._list = [
                    { code: 301, indent: 0, parameters: p },
                    { code: 0, indent: 0, parameters: [] }
                ];
                bi._index = 0;
                bi.executeCommand();
            } catch (ex) {
                console.warn('[MMO-NPC] Battle command error:', ex.message);
            }
            break;
        case 302: // Shop Processing
            try {
                var shopInterp = new Game_Interpreter();
                shopInterp._list = [
                    { code: 302, indent: 0, parameters: p },
                    { code: 0, indent: 0, parameters: [] }
                ];
                shopInterp._index = 0;
                shopInterp.executeCommand();
            } catch (ex) {
                console.warn('[MMO-NPC] Shop command error:', ex.message);
            }
            break;
        case 353: // Game Over
            SceneManager.goto(Scene_Gameover);
            break;
        case 354: // Return to Title
            SceneManager.goto(Scene_Title);
            break;

        // --- Script (code 355) ---
        case 355:
            // Server forwards concatenated script blocks for client evaluation.
            // Handles visual commands like $gameScreen.startTint().
            var scriptText = (p[0] || '').toString();
            if (scriptText) {
                try {
                    eval(scriptText);
                } catch (ex) {
                    if (MMO_CONFIG.debug) {
                        console.warn('[MMO-NPC] Script eval error:', ex.message, scriptText.substring(0, 100));
                    }
                }
            }
            break;

        // --- Plugin Command (code 356) ---
        case 356:
            // params: [0]="PluginName arg1 arg2 ..."
            var pluginCmd = (p[0] || '').toString();
            if (pluginCmd) {
                var args = pluginCmd.split(' ');
                var command = args.shift();
                // Create a proper Game_Interpreter instance so plugin overrides
                // (MPP_ChoiceEX, YEP_MessageCore, etc.) have their expected methods.
                try {
                    var interp = new Game_Interpreter();
                    interp._eventId = 0;
                    interp._mapId = $gameMap ? $gameMap.mapId() : 0;
                    interp.pluginCommand(command, args);
                } catch (ex) {
                    console.warn('[MMO-NPC] Plugin command error:', command, ex.message);
                }
            }
            break;
        }
    });

    // Helper: safely extract int from param array.
    function paramInt(arr, idx) {
        if (idx >= arr.length) return 0;
        return Number(arr[idx]) || 0;
    }

    // -----------------------------------------------------------------
    // WebSocket message handlers
    // -----------------------------------------------------------------
    $MMO.on('map_init', function (data) {
        var npcs = data.npcs || [];
        NPCManager.populate(npcs);
    });

    $MMO.on('npc_sync', function (data) {
        if (data && data.event_id) {
            NPCManager.sync(data);
        }
    });

    $MMO.on('npc_page_change', function (data) {
        if (data && data.event_id) {
            NPCManager.pageChange(data);
        }
    });

    $MMO.on('_disconnected', function () {
        NPCManager.clear();
    });

    // -----------------------------------------------------------------
    // Exports
    // -----------------------------------------------------------------
    window.NPCManager = NPCManager;
    window.Sprite_ServerNPC = Sprite_ServerNPC;

})();
