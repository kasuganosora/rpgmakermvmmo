/**
 * rmmv-shim.js  –  Minimal RPG Maker MV stubs for running client plugins in Node.js.
 *
 * Provides prototype chains and no-op drawing methods so that all 11 mmo-*.js
 * plugins can be loaded via vm.runInThisContext without errors.  Rendering is
 * completely stubbed; only state mutations are observable for test assertions.
 *
 * Inheritance chains and constructor/initialize patterns match the real
 * rpg_core.js / rpg_objects.js / rpg_scenes.js / rpg_sprites.js / rpg_windows.js
 * from RPG Maker MV 1.6.1 (Project1/js/).
 */
'use strict';

const WebSocket = require('ws');

// ---- browser-like globals ----
globalThis.window = globalThis;
globalThis.WebSocket = WebSocket;

// window.addEventListener / removeEventListener stubs (used by mmo-skill-bar, mmo-chat, mmo-party, mmo-social, mmo-inventory)
globalThis._windowListeners = {};
globalThis.addEventListener = function (type, fn) {
    if (!_windowListeners[type]) _windowListeners[type] = [];
    _windowListeners[type].push(fn);
};
globalThis.removeEventListener = function (type, fn) {
    if (!_windowListeners[type]) return;
    _windowListeners[type] = _windowListeners[type].filter(function (f) { return f !== fn; });
};

// Minimal DOM stubs
const _elements = [];
globalThis.document = {
    createElement: function (tag) {
        var el = {
            tagName: tag.toUpperCase(),
            type: '',
            value: '',
            placeholder: '',
            min: '',
            style: { cssText: '', display: '' },
            textContent: '',
            innerHTML: '',
            scrollTop: 0,
            scrollHeight: 0,
            parentNode: null,
            children: [],
            appendChild: function (child) {
                child.parentNode = el;
                el.children.push(child);
                return child;
            },
            removeChild: function (child) {
                child.parentNode = null;
                el.children = el.children.filter(function (c) { return c !== child; });
            },
            addEventListener: function () {},
            removeEventListener: function () {},
            focus: function () {},
            blur: function () {}
        };
        _elements.push(el);
        return el;
    },
    body: {
        appendChild: function (el) { el.parentNode = this; },
        removeChild: function (el) { el.parentNode = null; }
    },
    head: {
        appendChild: function () {}
    },
    addEventListener: function () {},
    removeEventListener: function () {},
    querySelector: function () { return null; }
};

// Timer shims already exist in Node.js (setTimeout, setInterval, etc.)

// ---- XMLHttpRequest (wraps Node http for $MMO.http) ----
var http = require('http');
var https = require('https');
var urlMod = require('url');

globalThis.XMLHttpRequest = function () {
    this.status = 0;
    this.responseText = '';
    this.readyState = 0;
    this.onload = null;
    this.onerror = null;
    this._method = 'GET';
    this._url = '';
    this._headers = {};
    this._body = null;
};
XMLHttpRequest.prototype.open = function (method, url) {
    this._method = method;
    this._url = url;
};
XMLHttpRequest.prototype.setRequestHeader = function (k, v) {
    this._headers[k] = v;
};
XMLHttpRequest.prototype.send = function (body) {
    var self = this;
    var parsed = new URL(this._url);
    var mod = parsed.protocol === 'https:' ? https : http;
    var opts = {
        method: this._method,
        hostname: parsed.hostname,
        port: parsed.port,
        path: parsed.pathname + parsed.search,
        headers: this._headers
    };
    var req = mod.request(opts, function (res) {
        var chunks = [];
        res.setEncoding('utf8');
        res.on('data', function (chunk) { chunks.push(chunk); });
        res.on('end', function () {
            self.status = res.statusCode;
            self.responseText = chunks.join('');
            self.readyState = 4;
            if (self.onload) self.onload();
        });
    });
    req.on('error', function (e) {
        if (self.onerror) self.onerror(e);
    });
    if (body) req.write(body);
    req.end();
};

// ---- Rectangle (used by Window internals) ----
function Rectangle(x, y, width, height) {
    this.x = x || 0;
    this.y = y || 0;
    this.width = width || 0;
    this.height = height || 0;
}
globalThis.Rectangle = Rectangle;

// ---- Bitmap (no-op drawing) ----
// Real: rpg_core.js line 830
function Bitmap(w, h) {
    this.initialize.apply(this, arguments);
}
Bitmap.prototype.initialize = function (w, h) {
    this.width = w || 0;
    this.height = h || 0;
    // Real default: rpg_core.js line 863
    this.fontSize = 28;
    this.fontFace = 'GameFont';
    this.fontItalic = false;
    // Real default: rpg_core.js line 879
    this.textColor = '#ffffff';
    this.outlineColor = 'rgba(0, 0, 0, 0.5)';
    this.outlineWidth = 4;
    this._paintOpacity = 255;
    this._loadListeners = [];
};
Bitmap.prototype.clear = function () {};
Bitmap.prototype.drawText = function () {};
Bitmap.prototype.fillRect = function () {};
Bitmap.prototype.clearRect = function () {};
Bitmap.prototype.fillAll = function () {};
Bitmap.prototype.gradientFillRect = function () {};
Bitmap.prototype.drawCircle = function () {};
Bitmap.prototype.blt = function () {};
Bitmap.prototype.measureTextWidth = function () { return 0; };
Bitmap.prototype.addLoadListener = function (fn) {
    // Real: calls immediately if already ready (rpg_core.js line 1509)
    fn(this);
};
Bitmap.prototype.isReady = function () { return true; };
Object.defineProperty(Bitmap.prototype, 'paintOpacity', {
    get: function () { return this._paintOpacity; },
    set: function (value) { this._paintOpacity = value; },
    configurable: true
});
globalThis.Bitmap = Bitmap;

// ---- Sprite ----
// Real: rpg_core.js line 3945  (Sprite → PIXI.Sprite → PIXI.Container)
// We skip PIXI inheritance but replicate the initialize chain and properties.
function Sprite() {
    this.initialize.apply(this, arguments);
}
Sprite._counter = 0;
Sprite.prototype.initialize = function (bitmap) {
    // Real sets up PIXI.Sprite internals; we stub container properties.
    this._bitmap = null;
    this.x = 0;
    this.y = 0;
    this.scale = { x: 1, y: 1 };
    this.anchor = { x: 0, y: 0 };
    this.opacity = 255;
    this.visible = true;
    this.parent = null;
    this.children = [];
    this.spriteId = Sprite._counter++;
    this.opaque = false;
    // Real assigns via defineProperty setter (triggers texture refresh).
    // We store directly since rendering is stubbed.
    this.bitmap = bitmap || null;
};
Sprite.prototype.update = function () {};
Sprite.prototype.addChild = function (child) {
    child.parent = this;
    this.children.push(child);
    return child;
};
Sprite.prototype.removeChild = function (child) {
    child.parent = null;
    this.children = this.children.filter(function (c) { return c !== child; });
};
Sprite.prototype.move = function (x, y) {
    this.x = x;
    this.y = y;
};
globalThis.Sprite = Sprite;

// ---- Sprite_Base ----
// Real: rpg_sprites.js line 10  (Sprite_Base → Sprite → PIXI.Sprite)
function Sprite_Base() {
    this.initialize.apply(this, arguments);
}
Sprite_Base.prototype = Object.create(Sprite.prototype);
Sprite_Base.prototype.constructor = Sprite_Base;
Sprite_Base.prototype.initialize = function () {
    Sprite.prototype.initialize.call(this);
    this._animationSprites = [];
    this._effectTarget = this;
    this._hiding = false;
};
Sprite_Base.prototype.update = function () {
    Sprite.prototype.update.call(this);
};
globalThis.Sprite_Base = Sprite_Base;

// ---- Sprite_Character ----
// Real: rpg_sprites.js line 185  (Sprite_Character → Sprite_Base → Sprite)
function Sprite_Character() {
    this.initialize.apply(this, arguments);
}
Sprite_Character.prototype = Object.create(Sprite_Base.prototype);
Sprite_Character.prototype.constructor = Sprite_Character;
Sprite_Character.prototype.initialize = function (character) {
    Sprite_Base.prototype.initialize.call(this);
    this.initMembers();
    this.setCharacter(character);
};
// Real: rpg_sprites.js line 197
Sprite_Character.prototype.initMembers = function () {
    this.anchor.x = 0.5;
    this.anchor.y = 1;
    this._character = null;
    this._balloonDuration = 0;
    this._tilesetId = 0;
    this._upperBody = null;
    this._lowerBody = null;
};
Sprite_Character.prototype.setCharacter = function (character) {
    this._character = character;
};
Sprite_Character.prototype.update = function () {
    Sprite_Base.prototype.update.call(this);
};
globalThis.Sprite_Character = Sprite_Character;

// ---- Game_CharacterBase ----
// Real: rpg_objects.js line 6244  (standalone base class)
function Game_CharacterBase() {
    this.initialize.apply(this, arguments);
}

// Real: rpg_objects.js line 6248-6251  (x/y are read-only getters over _x/_y)
Object.defineProperties(Game_CharacterBase.prototype, {
    x: { get: function () { return this._x; }, configurable: true },
    y: { get: function () { return this._y; }, configurable: true }
});

Game_CharacterBase.prototype.initialize = function () {
    this.initMembers();
};

// Real: rpg_objects.js line 6257
Game_CharacterBase.prototype.initMembers = function () {
    this._x = 0;
    this._y = 0;
    this._realX = 0;
    this._realY = 0;
    this._moveSpeed = 4;
    this._moveFrequency = 6;
    this._opacity = 255;
    this._blendMode = 0;
    this._direction = 2;
    this._pattern = 1;
    this._priorityType = 1;
    this._tileId = 0;
    this._characterName = '';
    this._characterIndex = 0;
    this._isObjectCharacter = false;
    this._walkAnime = true;
    this._stepAnime = false;
    this._directionFix = false;
    this._through = false;
    this._transparent = false;
    this._bushDepth = 0;
    this._animationId = 0;
    this._balloonId = 0;
    this._animationPlaying = false;
    this._balloonPlaying = false;
    this._animationCount = 0;
    this._stopCount = 0;
    this._jumpCount = 0;
    this._jumpPeak = 0;
    this._movementSuccess = true;
};

// Real: rpg_objects.js line 6443
Game_CharacterBase.prototype.setPosition = function (x, y) {
    this._x = Math.round(x);
    this._y = Math.round(y);
    this._realX = x;
    this._realY = y;
};

// Real: rpg_objects.js line 6464
Game_CharacterBase.prototype.direction = function () {
    return this._direction;
};

// Real: rpg_objects.js line 6468
Game_CharacterBase.prototype.setDirection = function (d) {
    if (!this.isDirectionFixed() && d) {
        this._direction = d;
    }
    this.resetStopCount();
};

Game_CharacterBase.prototype.isDirectionFixed = function () {
    return this._directionFix;
};

Game_CharacterBase.prototype.resetStopCount = function () {
    this._stopCount = 0;
};

// Real: rpg_objects.js line 6652
Game_CharacterBase.prototype.characterName = function () {
    return this._characterName;
};

Game_CharacterBase.prototype.characterIndex = function () {
    return this._characterIndex;
};

// Real: rpg_objects.js line 6660
Game_CharacterBase.prototype.setImage = function (characterName, characterIndex) {
    this._tileId = 0;
    this._characterName = characterName;
    this._characterIndex = characterIndex;
    this._isObjectCharacter = ImageManager.isObjectCharacter(characterName);
};

Game_CharacterBase.prototype.isTransparent = function () {
    return this._transparent;
};

Game_CharacterBase.prototype.setTransparent = function (transparent) {
    this._transparent = transparent;
};

Game_CharacterBase.prototype.isThrough = function () {
    return this._through;
};

// Real: rpg_objects.js line 6439
Game_CharacterBase.prototype.locate = function (x, y) {
    this.setPosition(x, y);
};

Game_CharacterBase.prototype.screenX = function () {
    return Math.round(this.scrolledX() * 48 + 24);
};

Game_CharacterBase.prototype.screenY = function () {
    return Math.round(this.scrolledY() * 48 + 48 - this.shiftY() - this.jumpHeight());
};

Game_CharacterBase.prototype.scrolledX = function () {
    return this._realX;
};

Game_CharacterBase.prototype.scrolledY = function () {
    return this._realY;
};

Game_CharacterBase.prototype.shiftY = function () {
    return 0;
};

Game_CharacterBase.prototype.jumpHeight = function () {
    return 0;
};

globalThis.Game_CharacterBase = Game_CharacterBase;

// ---- Game_Character ----
// Real: rpg_objects.js line 6834  (Game_Character → Game_CharacterBase)
function Game_Character() {
    this.initialize.apply(this, arguments);
}
Game_Character.prototype = Object.create(Game_CharacterBase.prototype);
Game_Character.prototype.constructor = Game_Character;

// Real: rpg_objects.js line 6888
Game_Character.prototype.initialize = function () {
    Game_CharacterBase.prototype.initialize.call(this);
};

// Real: rpg_objects.js line 6892
Game_Character.prototype.initMembers = function () {
    Game_CharacterBase.prototype.initMembers.call(this);
    this._moveRouteForcing = false;
    this._moveRoute = null;
    this._moveRouteIndex = 0;
    this._originalMoveRoute = null;
    this._originalMoveRouteIndex = 0;
    this._waitCount = 0;
};

globalThis.Game_Character = Game_Character;

// ---- Game_Player ----
// Real: rpg_objects.js line 7395  (Game_Player → Game_Character → Game_CharacterBase)
function Game_Player() {
    this.initialize.apply(this, arguments);
}
Game_Player.prototype = Object.create(Game_Character.prototype);
Game_Player.prototype.constructor = Game_Player;

// Real: rpg_objects.js line 7402
Game_Player.prototype.initialize = function () {
    Game_Character.prototype.initialize.call(this);
    this.setTransparent($dataSystem.optTransparent);
};

// Real: rpg_objects.js line 7407
Game_Player.prototype.initMembers = function () {
    Game_Character.prototype.initMembers.call(this);
    this._vehicleType = 'walk';
    this._vehicleGettingOn = false;
    this._vehicleGettingOff = false;
    this._dashing = false;
    this._needsMapReload = false;
    this._transferring = false;
    this._newMapId = 0;
    this._newX = 0;
    this._newY = 0;
    this._newDirection = 0;
    this._fadeType = 0;
    this._encounterCount = 0;
};

Game_Player.prototype.updateEncounterCount = function () {};
Game_Player.prototype.executeEncounter = function () { return false; };
Game_Player.prototype.canMove = function () { return true; };
Game_Player.prototype.reserveTransfer = function (mapId, x, y, d, fadeType) {
    this._transferring = true;
    this._newMapId = mapId;
    this._newX = x;
    this._newY = y;
    this._newDirection = d;
    this._fadeType = fadeType;
};
Game_Player.prototype.isMovementSucceeded = function () { return this._movementSuccess; };
globalThis.Game_Player = Game_Player;

// ---- Game_Followers (stub for Game_Player.initMembers) ----
function Game_Followers() {
    this._data = [];
}
globalThis.Game_Followers = Game_Followers;

// ---- Scene_Base ----
// Real: rpg_scenes.js line 14  (Scene_Base → Stage → PIXI.Container)
// We skip Stage/PIXI but replicate the initialize chain and children management.
function Scene_Base() {
    this.initialize.apply(this, arguments);
}
Scene_Base.prototype.initialize = function () {
    this._active = false;
    this._fadeSign = 0;
    this._fadeDuration = 0;
    this._fadeSprite = null;
    this.children = [];
};
Scene_Base.prototype.create = function () {};
Scene_Base.prototype.start = function () {};
Scene_Base.prototype.update = function () {};
Scene_Base.prototype.stop = function () { this._active = false; };
Scene_Base.prototype.terminate = function () {};
Scene_Base.prototype.addChild = function (child) {
    if (child) {
        child.parent = this;
        this.children.push(child);
    }
    return child;
};
Scene_Base.prototype.removeChild = function (child) {
    if (child) {
        child.parent = null;
        this.children = this.children.filter(function (c) { return c !== child; });
    }
};
globalThis.Scene_Base = Scene_Base;

// ---- Scene_Map ----
// Real: rpg_scenes.js  (Scene_Map → Scene_Base → Stage)
function Scene_Map() {
    this.initialize.apply(this, arguments);
}
Scene_Map.prototype = Object.create(Scene_Base.prototype);
Scene_Map.prototype.constructor = Scene_Map;
Scene_Map.prototype.createAllWindows = function () {};
Scene_Map.prototype.processMapTouch = function () {};
Scene_Map.prototype.start = function () { Scene_Base.prototype.start.call(this); };
globalThis.Scene_Map = Scene_Map;

// ---- Scene_Title ----
function Scene_Title() {
    this.initialize.apply(this, arguments);
}
Scene_Title.prototype = Object.create(Scene_Base.prototype);
Scene_Title.prototype.constructor = Scene_Title;
Scene_Title.prototype.start = function () {};
globalThis.Scene_Title = Scene_Title;

// ---- Scene_Boot ----
function Scene_Boot() {
    this.initialize.apply(this, arguments);
}
Scene_Boot.prototype = Object.create(Scene_Base.prototype);
Scene_Boot.prototype.constructor = Scene_Boot;
Scene_Boot.prototype.start = function () {};
globalThis.Scene_Boot = Scene_Boot;

// ---- Window_Base ----
// Real: rpg_windows.js line 10  (Window_Base → Window → PIXI.Container)
// We skip Window/PIXI but replicate the initialize chain.
function Window_Base() {
    this.initialize.apply(this, arguments);
}
Window_Base._iconWidth  = 32;
Window_Base._iconHeight = 32;
Window_Base._faceWidth  = 144;
Window_Base._faceHeight = 144;

// Real: rpg_windows.js line 17
Window_Base.prototype.initialize = function (x, y, w, h) {
    this.x = x || 0;
    this.y = y || 0;
    this.width = w || 0;
    this.height = h || 0;
    this.padding = 18;
    this.visible = true;
    this.opacity = 255;
    this.openness = 255;
    this._opening = false;
    this._closing = false;
    this._dimmerSprite = null;
    this.parent = null;
    this.children = [];
    this.createContents();
};
Window_Base.prototype.createContents = function () {
    this.contents = new Bitmap(this.contentsWidth(), this.contentsHeight());
    this.resetFontSettings();
};
Window_Base.prototype.contentsWidth = function () { return Math.max(this.width - this.padding * 2, 0); };
Window_Base.prototype.contentsHeight = function () { return Math.max(this.height - this.padding * 2, 0); };
Window_Base.prototype.standardFontSize = function () { return 28; };
Window_Base.prototype.standardPadding = function () { return 18; };
Window_Base.prototype.lineHeight = function () { return 36; };
Window_Base.prototype.resetFontSettings = function () {
    this.contents.fontFace = 'GameFont';
    this.contents.fontSize = this.standardFontSize();
    this.resetTextColor();
};
Window_Base.prototype.resetTextColor = function () {
    this.contents.textColor = '#ffffff';
};
Window_Base.prototype.drawText = function () {};
Window_Base.prototype.changeTextColor = function () {};
Window_Base.prototype.loadWindowskin = function () {};
Window_Base.prototype.updatePadding = function () { this.padding = this.standardPadding(); };
Window_Base.prototype.updateBackOpacity = function () {};
Window_Base.prototype.updateTone = function () {};
Window_Base.prototype.addChild = function (child) {
    if (child) child.parent = this;
    this.children.push(child);
    return child;
};
Window_Base.prototype.removeChild = function (child) {
    if (child) child.parent = null;
    this.children = this.children.filter(function (c) { return c !== child; });
};
Window_Base.prototype.update = function () {};
Window_Base.prototype.refresh = function () {};
Window_Base.prototype.move = function (x, y, width, height) {
    this.x = x;
    this.y = y;
    if (width !== undefined) this.width = width;
    if (height !== undefined) this.height = height;
};
Window_Base.prototype.open = function () { this.openness = 255; this._opening = false; };
Window_Base.prototype.close = function () { this.openness = 0; this._closing = false; };
Window_Base.prototype.isOpen = function () { return this.openness >= 255; };
Window_Base.prototype.isClosed = function () { return this.openness <= 0; };
globalThis.Window_Base = Window_Base;

// ---- Spriteset_Map ----
// Real: rpg_sprites.js  (Spriteset_Map → Spriteset_Base → Sprite)
function Spriteset_Map() {
    this._tilemap = {
        children: [],
        addChild: function (child) {
            child.parent = this;
            this.children.push(child);
        },
        removeChild: function (child) {
            child.parent = null;
            this.children = this.children.filter(function (c) { return c !== child; });
        }
    };
}
Spriteset_Map.prototype.createCharacters = function () {};
Spriteset_Map.prototype.update = function () {};
globalThis.Spriteset_Map = Spriteset_Map;

// ---- Global game objects ----
globalThis.$gameMap = {
    mapId: function () { return 1; },
    width: function () { return 30; },
    height: function () { return 30; },
    displayX: function () { return 0; },
    displayY: function () { return 0; },
    tileWidth: function () { return 48; },
    tileHeight: function () { return 48; },
    isPassable: function () { return true; },
    canvasToMapX: function (x) { return Math.floor(x / 48); },
    canvasToMapY: function (y) { return Math.floor(y / 48); }
};

globalThis.$dataSystem = { title1Name: '', optTransparent: false };

// ---- Game_Party (used by mmo-core.js) ----
function Game_Party() { this._actors = []; }
Game_Party.prototype.setupStartingMembers = function () { this._actors = [1]; };
Game_Party.prototype.addActor = function (id) { this._actors.push(id); };
globalThis.Game_Party = Game_Party;

// ---- Window_MenuCommand (used by mmo-core.js) ----
function Window_MenuCommand() {}
Window_MenuCommand.prototype = Object.create(Window_Base.prototype);
Window_MenuCommand.prototype.constructor = Window_MenuCommand;
Window_MenuCommand.prototype.addSaveCommand = function () {};
Window_MenuCommand.prototype.addFormationCommand = function () {};
globalThis.Window_MenuCommand = Window_MenuCommand;

// ---- Game_Interpreter (used by mmo-core.js map transfer) ----
function Game_Interpreter() { this._params = []; }
Game_Interpreter.prototype.command201 = function () { return true; };
Game_Interpreter.prototype.setWaitMode = function () {};
globalThis.Game_Interpreter = Game_Interpreter;

// ---- $gameTemp (used by processMapTouch) ----
globalThis.$gameTemp = {
    _destinationX: null,
    _destinationY: null,
    setDestination: function (x, y) { this._destinationX = x; this._destinationY = y; },
    clearDestination: function () { this._destinationX = null; this._destinationY = null; },
    isDestinationValid: function () { return this._destinationX !== null; }
};

// ---- $gameVariables (used by command201) ----
globalThis.$gameVariables = { value: function () { return 0; } };

// Must create $gamePlayer AFTER $dataSystem is defined (Game_Player.initialize reads it)
globalThis.$gamePlayer = new Game_Player();
// Place player at a default position for testing
$gamePlayer.setPosition(5, 5);

globalThis.Graphics = {
    boxWidth: 816,
    boxHeight: 624,
    frameCount: 0
};

// ---- SceneManager with call log (spy) ----
globalThis.SceneManager = {
    _scene: null,
    _callLog: [],
    goto: function (SceneClass) {
        this._callLog.push({ action: 'goto', scene: SceneClass });
        if (this._scene && this._scene.stop) this._scene.stop();
        try { this._scene = new SceneClass(); } catch (e) { this._scene = {}; }
    },
    push: function (SceneClass) {
        this._callLog.push({ action: 'push', scene: SceneClass });
        try { this._scene = new SceneClass(); } catch (e) { this._scene = {}; }
    },
    pop: function () {
        this._callLog.push({ action: 'pop' });
    }
};

globalThis.Input = {
    isTriggered: function () { return false; },
    isPressed: function () { return false; },
    isRepeated: function () { return false; },
    keyMapper: {}
};

globalThis.TouchInput = {
    isTriggered: function () { return false; },
    isPressed: function () { return false; },
    x: 0,
    y: 0,
    wheelY: 0
};

globalThis.ImageManager = {
    loadTitle1: function () { return new Bitmap(1, 1); },
    loadSystem: function () { return new Bitmap(1, 1); },
    loadCharacter: function () { return new Bitmap(1, 1); },
    loadFace: function () { return new Bitmap(1, 1); },
    // Real: rpg_managers.js line 905
    isObjectCharacter: function (filename) {
        var sign = (filename || '').match(/^[!$]+/);
        return sign && sign[0].indexOf('!') >= 0;
    }
};

globalThis.PluginManager = {
    parameters: function () { return {}; }
};

globalThis.StorageManager = {
    save: function () {},
    load: function () { return null; }
};

globalThis.DataManager = {};

// Real: rpg_core.js line 183
globalThis.Utils = {
    RPGMAKER_VERSION: '1.6.1',
    generateRuntimeId: function () { return Date.now(); }
};

// ---- localStorage stub ----
var _lsStore = {};
globalThis.localStorage = {
    getItem: function (k) { return _lsStore[k] || null; },
    setItem: function (k, v) { _lsStore[k] = String(v); },
    removeItem: function (k) { delete _lsStore[k]; },
    clear: function () { _lsStore = {}; }
};

// ---- Promise polyfill (Node.js has native Promise, but ensure window.Promise exists) ----
globalThis.Promise = Promise;
