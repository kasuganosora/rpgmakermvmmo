/*:
 * @plugindesc v3.1.0 MMO Auth - Login, character select and creation scenes (L2 UI).
 * @author MMO Framework
 */

(function () {
    'use strict';

    var BASE_URL = ($MMO._serverUrl || 'ws://localhost:8080').replace(/^ws/, 'http');

    // -----------------------------------------------------------------
    // Keyboard fix: let browser handle keys when HTML inputs are focused
    // -----------------------------------------------------------------
    var _Input_shouldPreventDefault = Input._shouldPreventDefault;
    Input._shouldPreventDefault = function (keyCode) {
        var active = document.activeElement;
        if (active && (active.tagName === 'INPUT' || active.tagName === 'TEXTAREA')) {
            return false;
        }
        return _Input_shouldPreventDefault.call(this, keyCode);
    };

    var _Input_onKeyDown = Input._onKeyDown;
    Input._onKeyDown = function (event) {
        var active = document.activeElement;
        if (active && (active.tagName === 'INPUT' || active.tagName === 'TEXTAREA')) return;
        _Input_onKeyDown.call(this, event);
    };

    var _Input_onKeyUp = Input._onKeyUp;
    Input._onKeyUp = function (event) {
        var active = document.activeElement;
        if (active && (active.tagName === 'INPUT' || active.tagName === 'TEXTAREA')) return;
        _Input_onKeyUp.call(this, event);
    };

    // -----------------------------------------------------------------
    // $MMO.http utility
    // -----------------------------------------------------------------
    $MMO.http = {
        _req: function (method, path, body) {
            return new Promise(function (resolve, reject) {
                var xhr = new XMLHttpRequest();
                xhr.open(method, BASE_URL + path);
                xhr.setRequestHeader('Content-Type', 'application/json');
                if ($MMO.token) {
                    xhr.setRequestHeader('Authorization', 'Bearer ' + $MMO.token);
                }
                xhr.onload = function () {
                    try {
                        var data = JSON.parse(xhr.responseText);
                        if (xhr.status >= 200 && xhr.status < 300) {
                            resolve(data);
                        } else {
                            reject(new Error(data.error || 'HTTP ' + xhr.status));
                        }
                    } catch (e) {
                        reject(e);
                    }
                };
                xhr.onerror = function () { reject(new Error('Network error')); };
                xhr.send(body ? JSON.stringify(body) : null);
            });
        },
        get: function (path) { return this._req('GET', path, null); },
        post: function (path, body) { return this._req('POST', path, body); },
        del: function (path, body) { return this._req('DELETE', path, body || null); }
    };

    // -----------------------------------------------------------------
    // Helpers — HTML inputs that reposition on window resize
    // -----------------------------------------------------------------
    var _trackedInputs = [];   // [{el, gx, gy, gw, gh}]

    function _repositionInputs() {
        var bw = Graphics.boxWidth || 816;
        var bh = Graphics.boxHeight || 624;
        var canvas = Graphics._canvas;
        var rect = canvas
            ? canvas.getBoundingClientRect()
            : { left: 0, top: 0, width: bw, height: bh };
        var sx = rect.width / bw;
        var sy = rect.height / bh;
        for (var i = 0; i < _trackedInputs.length; i++) {
            var t = _trackedInputs[i];
            t.el.style.left   = (rect.left + t.gx * sx) + 'px';
            t.el.style.top    = (rect.top  + t.gy * sy) + 'px';
            t.el.style.width  = (t.gw * sx) + 'px';
            t.el.style.height = (t.gh * sy) + 'px';
            t.el.style.fontSize = Math.round(13 * sy) + 'px';
        }
    }

    window.addEventListener('resize', _repositionInputs);

    function makeL2Input(type, placeholder, gameX, gameY, w, h) {
        h = h || 28;
        var el = document.createElement('input');
        el.type = type;
        el.placeholder = placeholder;
        el.style.cssText = [
            'position:fixed',
            'font-family:GameFont,sans-serif',
            'background:#0A0A18',
            'color:#E8E8E8',
            'border:1px solid #2A2A44',
            'border-radius:3px',
            'z-index:10',
            '-webkit-user-select:text',
            'user-select:text',
            'outline:none',
            'padding:2px 8px',
            'box-sizing:border-box',
            'transition:border-color 0.2s'
        ].join(';');
        el.addEventListener('focus', function () { el.style.borderColor = '#BFA530'; });
        el.addEventListener('blur', function () { el.style.borderColor = '#2A2A44'; });
        // Prevent RMMV TouchInput from stealing focus/click
        el.addEventListener('mousedown', function (e) { e.stopPropagation(); });
        el.addEventListener('touchstart', function (e) { e.stopPropagation(); });
        document.body.appendChild(el);
        _trackedInputs.push({ el: el, gx: gameX, gy: gameY, gw: w, gh: h });
        _repositionInputs();
        return el;
    }

    function removeEl(el) {
        if (el && el.parentNode) el.parentNode.removeChild(el);
        for (var i = _trackedInputs.length - 1; i >= 0; i--) {
            if (_trackedInputs[i].el === el) { _trackedInputs.splice(i, 1); break; }
        }
    }

    function createBackground(scene) {
        scene._backSprite = new Sprite(ImageManager.loadTitle1($dataSystem.title1Name));
        scene.addChild(scene._backSprite);
    }

    // -----------------------------------------------------------------
    // Glass panel — blurred background + 50% semi-transparent overlay
    // -----------------------------------------------------------------
    function createGlassPanel(scene, x, y, w, h, title) {
        // 1. Blurred copy of background, masked to panel area
        var bgBmp = scene._backSprite.bitmap;
        var blurBg = new Sprite(bgBmp);
        if (PIXI.filters && PIXI.filters.BlurFilter) {
            blurBg.filters = [new PIXI.filters.BlurFilter(8)];
        }
        var mask = new PIXI.Graphics();
        mask.beginFill(0xFFFFFF);
        mask.drawRoundedRect(x, y, w, h, 4);
        mask.endFill();
        scene.addChild(mask);
        blurBg.mask = mask;
        scene.addChild(blurBg);

        // 2. Semi-transparent dark overlay (50%)
        var overlayBmp = new Bitmap(w, h);
        overlayBmp.fillRect(0, 0, w, h, 'rgba(13,13,26,0.50)');
        var overlay = new Sprite(overlayBmp);
        overlay.x = x;
        overlay.y = y;
        scene.addChild(overlay);

        // 3. Border + title bar drawn on L2_Base
        var panel = new L2_Base(x, y, w, h);
        var c = panel.bmp();
        var cw = panel.cw(), ch = panel.ch();
        L2_Theme.strokeRoundRect(c, 0, 0, cw, ch, L2_Theme.cornerRadius,
            'rgba(100,120,180,0.35)');
        if (title) {
            L2_Theme.drawTitleBar(c, 0, 0, cw, L2_Theme.titleBarH, title);
        }
        scene.addChild(panel);
        return panel;
    }

    // -----------------------------------------------------------------
    // PanelLayout — auto-compute panel height from content rows
    // Usage: var L = new PanelLayout(x, y, w, {title:'...'});
    //        var row1Y = L.row(46);  // allocate a 46px row, returns abs Y
    //        var row2Y = L.row(46);
    //        var ph = L.height();    // auto-computed total height
    //        createGlassPanel(scene, x, y, w, ph, title);
    // -----------------------------------------------------------------
    function PanelLayout(x, y, w, opts) {
        opts = opts || {};
        this.x = x;
        this.y = y;
        this.w = w;
        this._padX = opts.padX || 8;
        this._padY = opts.padY || 16;
        this._title = opts.title || null;
        this._cy = this._title ? (L2_Theme.titleBarH + this._padY) : this._padY;
    }
    /** Allocate a row of given height, returns absolute screen Y. */
    PanelLayout.prototype.row = function (h) {
        var absY = this.y + this._cy;
        this._cy += (h || 40);
        return absY;
    };
    /** Total panel height (call after all rows are allocated). */
    PanelLayout.prototype.height = function () { return this._cy + this._padY; };
    /** Absolute X for content (label start). */
    PanelLayout.prototype.cx = function () { return this.x + this._padX; };
    /** Absolute X for input fields (after label). */
    PanelLayout.prototype.ix = function (labelW) { return this.x + this._padX + (labelW || 80) + 10; };
    /** Available width for input fields. */
    PanelLayout.prototype.iw = function (labelW) { return this.w - this._padX * 2 - (labelW || 80) - 14; };

    /** Solid opaque panel (for secondary panels). */
    function drawPanel(base, title) {
        var c = base.bmp();
        c.clear();
        var cw = base.cw(), ch = base.ch();
        L2_Theme.drawPanelBg(c, 0, 0, cw, ch);
        if (title) {
            L2_Theme.drawTitleBar(c, 0, 0, cw, L2_Theme.titleBarH, title);
        }
    }

    // -----------------------------------------------------------------
    // Character creation presets
    // -----------------------------------------------------------------
    var CLASS_OPTIONS = [
        { label: 'Warrior', value: 1 },
        { label: 'Mage',    value: 2 },
        { label: 'Archer',  value: 3 },
        { label: 'Thief',   value: 4 }
    ];

    var FACE_PRESETS = [];
    (function () {
        for (var i = 0; i < 8; i++) {
            FACE_PRESETS.push({ faceName: 'Actor1', faceIndex: i });
        }
    })();

    // =================================================================
    //  Scene_Login
    // =================================================================
    function Scene_Login() { this.initialize.apply(this, arguments); }
    Scene_Login.prototype = Object.create(Scene_Base.prototype);
    Scene_Login.prototype.constructor = Scene_Login;

    Scene_Login.prototype.initialize = function () {
        Scene_Base.prototype.initialize.call(this);
        this._inputs = [];
        this._busy = false;
    };

    Scene_Login.prototype.create = function () {
        Scene_Base.prototype.create.call(this);
        createBackground(this);

        var gw = Graphics.boxWidth;
        var pw = 400;
        var px = (gw - pw) / 2, py = 175;

        // ── Layout: compute positions first, panel height auto-derived ──
        var L = new PanelLayout(px, py, pw);
        var accountY = L.row(46);
        var passwordY = L.row(46);
        var buttonY = L.row(44);
        var ph = L.height();

        // ── Title ──
        this.addChild(new L2_Typography((gw - 300) / 2, 115, 300, {
            text: 'MMO LOGIN', level: 'h1', align: 'center', color: L2_Theme.textGold
        }));

        // ── Glass panel (height auto-computed from layout) ──
        this._panel = createGlassPanel(this, px, py, pw, ph, null);

        // Row 1: Account
        this.addChild(new L2_Typography(L.cx() + 4, accountY + 4, 80, {
            text: 'Account', color: L2_Theme.textGray
        }));
        this._userInput = makeL2Input('text', 'Enter username', L.ix(), accountY, L.iw(), 28);
        this._inputs.push(this._userInput);

        // Row 2: Password
        this.addChild(new L2_Typography(L.cx() + 4, passwordY + 4, 80, {
            text: 'Password', color: L2_Theme.textGray
        }));
        this._passInput = makeL2Input('password', 'Enter password', L.ix(), passwordY, L.iw(), 28);
        this._inputs.push(this._passInput);

        // ── Buttons (inside panel) ──
        this._loginBtn = new L2_Button(px + 55, buttonY, 'Login', {
            type: 'primary',
            onClick: this._doLogin.bind(this)
        });
        this.addChild(this._loginBtn);

        this._regBtn = new L2_Button(px + pw - 170, buttonY, 'Register', {
            onClick: this._doRegister.bind(this)
        });
        this.addChild(this._regBtn);

        // ── Keyboard shortcuts ──
        var self = this;
        this._userInput.addEventListener('keydown', function (e) {
            if (e.keyCode === 13) self._passInput.focus();
        });
        this._passInput.addEventListener('keydown', function (e) {
            if (e.keyCode === 13) self._doLogin();
        });
        this._focusTimer = setTimeout(function () { self._userInput.focus(); }, 100);
    };

    Scene_Login.prototype._doLogin = function () {
        if (this._busy) return;
        var self = this;
        var username = this._userInput.value.trim();
        var password = this._passInput.value;
        if (!username || !password) {
            L2_Message.warning('Please enter username and password.');
            return;
        }
        this._busy = true;
        $MMO.http.post('/api/auth/login', { username: username, password: password })
            .then(function (data) {
                $MMO.token = data.token;
                self._cleanup();
                SceneManager.goto(Scene_CharacterSelect);
            })
            .catch(function (e) {
                self._busy = false;
                L2_Message.error(e.message);
            });
    };

    Scene_Login.prototype._doRegister = function () {
        if (this._busy) return;
        var self = this;
        var username = this._userInput.value.trim();
        var password = this._passInput.value;
        if (!username || !password) {
            L2_Message.warning('Please enter username and password.');
            return;
        }
        this._busy = true;
        $MMO.http.post('/api/auth/register', { username: username, password: password })
            .then(function (data) {
                $MMO.token = data.token;
                self._cleanup();
                SceneManager.goto(Scene_CharacterSelect);
            })
            .catch(function (e) {
                self._busy = false;
                L2_Message.error(e.message);
            });
    };

    Scene_Login.prototype._cleanup = function () {
        this._inputs.forEach(removeEl);
        this._inputs = [];
    };

    Scene_Login.prototype.terminate = function () {
        Scene_Base.prototype.terminate.call(this);
        if (this._focusTimer) { clearTimeout(this._focusTimer); this._focusTimer = null; }
        this._cleanup();
    };

    // =================================================================
    //  Scene_CharacterSelect
    // =================================================================
    function Scene_CharacterSelect() { this.initialize.apply(this, arguments); }
    Scene_CharacterSelect.prototype = Object.create(Scene_Base.prototype);
    Scene_CharacterSelect.prototype.constructor = Scene_CharacterSelect;

    Scene_CharacterSelect.prototype.initialize = function () {
        Scene_Base.prototype.initialize.call(this);
        this._characters = [];
        this._selectedIndex = -1;
        this._cardWindows = [];
    };

    Scene_CharacterSelect.prototype.create = function () {
        Scene_Base.prototype.create.call(this);
        createBackground(this);
        this._loadCharacters();
    };

    Scene_CharacterSelect.prototype._loadCharacters = function () {
        var self = this;
        this._loader = new L2_Loading(
            (Graphics.boxWidth - 120) / 2,
            (Graphics.boxHeight - 30) / 2,
            { text: 'Loading...' }
        );
        this.addChild(this._loader);

        $MMO.http.get('/api/characters')
            .then(function (data) {
                if (self._loader) { self.removeChild(self._loader); self._loader = null; }
                self._characters = data.characters || [];
                self._selectedIndex = self._characters.length > 0 ? 0 : -1;
                self._buildUI();
            })
            .catch(function (e) {
                if (self._loader) { self.removeChild(self._loader); self._loader = null; }
                L2_Message.error('Failed to load characters: ' + e.message);
                self._buildUI();
            });
    };

    Scene_CharacterSelect.prototype._buildUI = function () {
        var self = this;
        var gw = Graphics.boxWidth;
        var gh = Graphics.boxHeight;

        // ── Title ──
        this.addChild(new L2_Typography((gw - 400) / 2, 10, 400, {
            text: 'SELECT CHARACTER', level: 'h1', align: 'center', color: L2_Theme.textGold
        }));

        // ── Info panel (left, glass — height auto-computed) ──
        if (this._characters.length > 0) {
            var infoL = new PanelLayout(16, 60, 230, { title: 'Character Info' });
            infoL.row(22);  // name
            infoL.row(28);  // level + class
            infoL.row(22);  // HP bar
            infoL.row(22);  // MP bar
            infoL.row(22);  // EXP
            this._infoPanel = createGlassPanel(this, 16, 60, 230, infoL.height(), 'Character Info');
            this._infoPanelBase = this._infoPanel;
            this._refreshInfoPanel();
        }

        // ── Character cards (center) ──
        var CARD_W = 150, CARD_H = 130, CARD_GAP = 16;
        var numCards = this._characters.length;
        var totalW = numCards * CARD_W + Math.max(0, numCards - 1) * CARD_GAP;
        var startX = (gw - totalW) / 2;

        this._cardWindows = [];
        this._characters.forEach(function (ch, i) {
            var card = self._createCard(
                startX + i * (CARD_W + CARD_GAP), 280,
                CARD_W, CARD_H, ch, i === self._selectedIndex
            );
            self.addChild(card);
            self._cardWindows.push(card);
        });

        // ── No characters message ──
        if (this._characters.length === 0) {
            this.addChild(new L2_Typography((gw - 360) / 2, gh / 2 - 30, 360, {
                text: 'No characters yet.', level: 'h2', align: 'center', color: L2_Theme.textGray
            }));
            this.addChild(new L2_Typography((gw - 360) / 2, gh / 2, 360, {
                text: 'Create one to begin your adventure!', align: 'center', color: L2_Theme.textDim
            }));
        }

        // ── Enter Game button (center) ──
        if (this._characters.length > 0) {
            this._enterBtn = new L2_Button((gw - 160) / 2, 430, 160, 36, 'Enter Game', {
                type: 'primary',
                onClick: function () { self._enterGame(); }
            });
            this.addChild(this._enterBtn);
        }

        // ── Right-side action buttons ──
        var rbX = gw - 180, rbY = gh - 150;
        this.addChild(new L2_Button(rbX, rbY, 'Create Character', {
            onClick: function () { SceneManager.goto(Scene_CharacterCreate); }
        }));

        rbY += 40;
        this.addChild(new L2_Button(rbX, rbY, 'Delete Character', {
            type: 'danger',
            onClick: function () { self._deleteCharacter(); }
        }));

        rbY += 40;
        this.addChild(new L2_Button(rbX, rbY, 'Logout', {
            type: 'text',
            onClick: function () {
                $MMO.disconnect();
                $MMO.token = null;
                SceneManager.goto(Scene_Login);
            }
        }));
    };

    // ── Card factory ──
    Scene_CharacterSelect.prototype._createCard = function (x, y, w, h, charData, selected) {
        var self = this;
        var card = new L2_Base(x, y, w, h);
        card._charData = charData;
        card._selected = selected;
        card._hover = false;
        card._faceBmp = ImageManager.loadFace(charData.face_name || 'Actor1');
        card._faceBmp.addLoadListener(function () { self._refreshCard(card); });

        var origUpdate = L2_Base.prototype.update;
        card.update = function () {
            origUpdate.call(this);
            if (!this.visible) return;
            var wasHover = this._hover;
            this._hover = this.isInside(TouchInput.x, TouchInput.y);
            if (this._hover !== wasHover) self._refreshCard(this);
            if (this._hover && TouchInput.isTriggered()) {
                self._selectCard(self._cardWindows.indexOf(this));
            }
        };

        this._refreshCard(card);
        return card;
    };

    Scene_CharacterSelect.prototype._refreshCard = function (card) {
        var c = card.bmp();
        c.clear();
        var cw = card.cw(), ch = card.ch();
        var d = card._charData;

        var bg = card._selected ? '#1A1A44' : L2_Theme.bgPanel;
        var border = card._selected ? L2_Theme.borderGold :
                     (card._hover ? L2_Theme.borderLight : L2_Theme.borderDark);
        L2_Theme.fillRoundRect(c, 0, 0, cw, ch, L2_Theme.cornerRadius, bg);
        L2_Theme.strokeRoundRect(c, 0, 0, cw, ch, L2_Theme.cornerRadius, border);

        // Face avatar (centered 56x56)
        var aSize = 56, ax = (cw - aSize) / 2;
        if (card._faceBmp && card._faceBmp.isReady()) {
            var fw = 144, fh = 144;
            var fi = d.face_index || 0;
            c.blt(card._faceBmp, (fi % 4) * fw, Math.floor(fi / 4) * fh,
                fw, fh, ax, 10, aSize, aSize);
        }

        c.fontSize = L2_Theme.fontNormal;
        c.textColor = card._selected ? L2_Theme.textGold : L2_Theme.textWhite;
        c.drawText(d.name, 0, 70, cw, 18, 'center');

        c.fontSize = L2_Theme.fontSmall;
        c.textColor = L2_Theme.textGray;
        var classText = 'Lv.' + d.level + '  ' + (d.class_name || 'Class ' + d.class_id);
        c.drawText(classText, 0, 90, cw, 16, 'center');
    };

    Scene_CharacterSelect.prototype._selectCard = function (idx) {
        if (idx < 0 || idx === this._selectedIndex) return;
        var old = this._cardWindows[this._selectedIndex];
        if (old) { old._selected = false; this._refreshCard(old); }
        this._selectedIndex = idx;
        var cur = this._cardWindows[idx];
        if (cur) { cur._selected = true; this._refreshCard(cur); }
        this._refreshInfoPanel();
    };

    Scene_CharacterSelect.prototype._refreshInfoPanel = function () {
        var panel = this._infoPanelBase;
        if (!panel || this._selectedIndex < 0) return;
        var ch = this._characters[this._selectedIndex];
        if (!ch) return;
        var c = panel.bmp();
        c.clear();
        var cw = panel.cw(), contentH = panel.ch();

        // Re-draw border + title on the glass panel
        L2_Theme.strokeRoundRect(c, 0, 0, cw, contentH, L2_Theme.cornerRadius,
            'rgba(100,120,180,0.35)');
        L2_Theme.drawTitleBar(c, 0, 0, cw, L2_Theme.titleBarH, 'Character Info');

        var y = L2_Theme.titleBarH + 10;
        var barW = cw - 20;

        // Name
        c.fontSize = L2_Theme.fontH3;
        c.textColor = L2_Theme.textGold;
        c.drawText(ch.name, 10, y, barW, 20, 'left');
        y += 22;

        // Level + class
        c.fontSize = L2_Theme.fontNormal;
        c.textColor = L2_Theme.textGray;
        c.drawText('Lv.' + ch.level + '  ' + (ch.class_name || 'Class ' + ch.class_id),
            10, y, barW, 18, 'left');
        y += 28;

        // HP bar
        var maxHP = ch.max_hp || 100, hp = ch.hp || maxHP;
        L2_Theme.drawBar(c, 10, y, barW, 16,
            Math.min(hp / maxHP, 1), L2_Theme.hpBg, L2_Theme.hpFill);
        c.fontSize = L2_Theme.fontSmall;
        c.textColor = L2_Theme.textWhite;
        c.drawText('HP ' + hp + '/' + maxHP, 14, y, barW - 8, 16, 'left');
        y += 22;

        // MP bar
        var maxMP = ch.max_mp || 50, mp = ch.mp || maxMP;
        L2_Theme.drawBar(c, 10, y, barW, 14,
            Math.min(mp / maxMP, 1), L2_Theme.mpBg, L2_Theme.mpFill);
        c.drawText('MP ' + mp + '/' + maxMP, 14, y, barW - 8, 14, 'left');
        y += 22;

        // EXP
        c.fontSize = L2_Theme.fontSmall;
        c.textColor = L2_Theme.textGold;
        c.drawText('EXP: ' + (ch.exp || 0), 10, y, barW, 16, 'left');
    };

    Scene_CharacterSelect.prototype._enterGame = function () {
        if (this._selectedIndex < 0 || this._busy) return;
        this._busy = true;
        var ch = this._characters[this._selectedIndex];
        $MMO.charID = ch.id;
        $MMO.charName = ch.name;

        var onConnected = function () {
            $MMO.off('_connected', onConnected);
            $MMO.send('enter_map', { char_id: ch.id });
        };
        $MMO.on('_connected', onConnected);
        $MMO.connect($MMO.token);
        // Stop title BGM before entering the game map.
        AudioManager.fadeOutBgm(1);
        AudioManager.stopBgs();
        AudioManager.stopMe();
        SceneManager.goto(Scene_Map);
    };

    Scene_CharacterSelect.prototype._deleteCharacter = function () {
        if (this._selectedIndex < 0) {
            L2_Message.warning('No character selected.');
            return;
        }
        var self = this;
        var ch = this._characters[this._selectedIndex];
        var dlg = new L2_Dialog({
            title: 'Delete Character',
            content: 'Delete "' + ch.name + '"?\nEnter password to confirm.\n\n',
            buttons: [
                {
                    text: 'Delete', type: 'danger',
                    onClick: function () {
                        var pw = self._delPwInput ? self._delPwInput.value : '';
                        if (!pw) { L2_Message.warning('Please enter password.'); return; }
                        dlg.close();
                        removeEl(self._delPwInput);
                        self._delPwInput = null;
                        $MMO.http.del('/api/characters/' + ch.id, { password: pw })
                            .then(function () { SceneManager.goto(Scene_CharacterSelect); })
                            .catch(function (e) { L2_Message.error(e.message); });
                    }
                },
                {
                    text: 'Cancel', type: 'default',
                    onClick: function () {
                        dlg.close();
                        removeEl(self._delPwInput);
                        self._delPwInput = null;
                    }
                }
            ],
            onClose: function () {
                removeEl(self._delPwInput);
                self._delPwInput = null;
            }
        });
        this.addChild(dlg);
        // Password input: positioned from dialog's actual layout
        var inputW = dlg.width - 60;
        var dx = dlg.x + (dlg.width - inputW) / 2;
        var dy = dlg.y + dlg._titleH + 8 + 2 * 20 + 8; // after title + 2 text lines + gap
        self._delPwInput = makeL2Input('password', 'Enter password', dx, dy, inputW, 28);
        this._delFocusTimer = setTimeout(function () { self._delPwInput.focus(); }, 50);
    };

    // =================================================================
    //  Scene_CharacterCreate
    // =================================================================
    function Scene_CharacterCreate() { this.initialize.apply(this, arguments); }
    Scene_CharacterCreate.prototype = Object.create(Scene_Base.prototype);
    Scene_CharacterCreate.prototype.constructor = Scene_CharacterCreate;

    Scene_CharacterCreate.prototype.initialize = function () {
        Scene_Base.prototype.initialize.call(this);
        this._inputs = [];
        this._selectedFace = 0;
        this._faceButtons = [];
        this._faceBitmaps = {};
        this._busy = false;
    };

    Scene_CharacterCreate.prototype.create = function () {
        Scene_Base.prototype.create.call(this);
        createBackground(this);

        var gw = Graphics.boxWidth;
        var pw = 450;
        var px = (gw - pw) / 2, py = 80;
        var faceSize = 54, faceGap = 8;
        var faceRows = 2;
        var faceGridH = faceRows * faceSize + (faceRows - 1) * faceGap;

        // ── Layout: auto-compute panel height ──
        var L = new PanelLayout(px, py, pw, { title: 'New Character' });
        var nameY   = L.row(44);
        var classY  = L.row(44);
        var faceLabelY = L.row(22);   // "Face" label
        var faceGridY  = L.row(faceGridH); // face grid
        var btnY    = L.row(44);      // buttons inside panel
        var ph = L.height();

        // ── Title ──
        this.addChild(new L2_Typography((gw - 400) / 2, 24, 400, {
            text: 'CREATE CHARACTER', level: 'h1', align: 'center', color: L2_Theme.textGold
        }));

        // ── Glass panel (height auto-computed) ──
        this._panel = createGlassPanel(this, px, py, pw, ph, 'New Character');

        // Row 1: Name
        this.addChild(new L2_Typography(L.cx() + 4, nameY + 4, 80, {
            text: 'Name', color: L2_Theme.textGray
        }));
        this._nameInput = makeL2Input('text', 'Character Name', L.ix(), nameY, L.iw(), 28);
        this._inputs.push(this._nameInput);

        // Row 2: Class
        this.addChild(new L2_Typography(L.cx() + 4, classY + 4, 80, {
            text: 'Class', color: L2_Theme.textGray
        }));
        this._classSelect = new L2_Select(L.ix(), classY, L.iw(), {
            options: CLASS_OPTIONS,
            selected: 0,
            placeholder: 'Select a class'
        });

        // Row 3: Face label
        this.addChild(new L2_Typography(L.cx() + 4, faceLabelY + 2, 80, {
            text: 'Face', color: L2_Theme.textGray
        }));

        // Face grid: 4 cols x 2 rows
        var gridX = L.ix();
        var self = this;
        FACE_PRESETS.forEach(function (preset, i) {
            var col = i % 4, row = Math.floor(i / 4);
            var fx = gridX + col * (faceSize + faceGap);
            var fy = faceGridY + row * (faceSize + faceGap);
            var faceBtn = self._createFaceButton(fx, fy, faceSize, preset, i);
            self.addChild(faceBtn);
            self._faceButtons.push(faceBtn);
        });

        // Add class select on top (z-order above face grid for dropdown)
        this.addChild(this._classSelect);

        // Preview avatar (right of grid)
        var previewX = gridX + 4 * (faceSize + faceGap) + 12;
        this._previewBg = new L2_Base(previewX, faceGridY, 80, 80);
        this.addChild(this._previewBg);
        this._refreshPreview();

        // ── Buttons (inside panel) ──
        this.addChild(new L2_Button(px + 80, btnY, 'Create', {
            type: 'primary',
            onClick: function () { self._doCreate(); }
        }));
        this.addChild(new L2_Button(px + pw - 160, btnY, 'Back', {
            onClick: function () {
                self._cleanup();
                SceneManager.goto(Scene_CharacterSelect);
            }
        }));

        // ── Keyboard ──
        this._nameInput.addEventListener('keydown', function (e) {
            if (e.keyCode === 13) self._doCreate();
        });
        this._focusTimer = setTimeout(function () { self._nameInput.focus(); }, 100);
    };

    Scene_CharacterCreate.prototype._createFaceButton = function (x, y, size, preset, index) {
        var self = this;
        var btn = new L2_Base(x, y, size + 4, size + 4);
        btn._preset = preset;
        btn._index = index;
        btn._hover = false;
        btn.standardPadding = function () { return 2; };

        var key = preset.faceName;
        if (!this._faceBitmaps[key]) {
            this._faceBitmaps[key] = ImageManager.loadFace(key);
        }
        var faceBmp = this._faceBitmaps[key];
        faceBmp.addLoadListener(function () { self._refreshFaceBtn(btn); });

        var origUpdate = L2_Base.prototype.update;
        btn.update = function () {
            origUpdate.call(this);
            if (!this.visible) return;
            var wasHover = this._hover;
            this._hover = this.isInside(TouchInput.x, TouchInput.y);
            if (this._hover !== wasHover) self._refreshFaceBtn(this);
            if (this._hover && TouchInput.isTriggered()) {
                self._selectFace(this._index);
            }
        };

        this._refreshFaceBtn(btn);
        return btn;
    };

    Scene_CharacterCreate.prototype._refreshFaceBtn = function (btn) {
        var c = btn.bmp();
        c.clear();
        var s = btn.cw();
        var sel = btn._index === this._selectedFace;
        var border = sel ? L2_Theme.borderGold :
                     (btn._hover ? L2_Theme.borderLight : L2_Theme.borderDark);
        var bg = sel ? '#1A1A44' : L2_Theme.bgPanel;

        L2_Theme.fillRoundRect(c, 0, 0, s, s, 2, bg);
        L2_Theme.strokeRoundRect(c, 0, 0, s, s, 2, border);

        var key = btn._preset.faceName;
        var faceBmp = this._faceBitmaps[key];
        if (faceBmp && faceBmp.isReady()) {
            var fw = 144, fh = 144;
            var fi = btn._preset.faceIndex;
            var pad = 3;
            c.blt(faceBmp,
                (fi % 4) * fw, Math.floor(fi / 4) * fh, fw, fh,
                pad, pad, s - pad * 2, s - pad * 2);
        }
    };

    Scene_CharacterCreate.prototype._selectFace = function (idx) {
        var old = this._selectedFace;
        this._selectedFace = idx;
        var self = this;
        this._faceButtons.forEach(function (btn) {
            if (btn._index === old || btn._index === idx) {
                self._refreshFaceBtn(btn);
            }
        });
        this._refreshPreview();
    };

    Scene_CharacterCreate.prototype._refreshPreview = function () {
        if (!this._previewBg) return;
        var c = this._previewBg.bmp();
        c.clear();
        var s = this._previewBg.cw();
        L2_Theme.fillRoundRect(c, 0, 0, s, s, L2_Theme.cornerRadius, L2_Theme.bgPanel);
        L2_Theme.strokeRoundRect(c, 0, 0, s, s, L2_Theme.cornerRadius, L2_Theme.borderGold);

        var preset = FACE_PRESETS[this._selectedFace];
        if (!preset) return;
        var key = preset.faceName;
        if (!this._faceBitmaps[key]) return;
        var faceBmp = this._faceBitmaps[key];
        if (faceBmp && faceBmp.isReady()) {
            var fw = 144, fh = 144;
            var fi = preset.faceIndex;
            var pad = 4;
            c.blt(faceBmp,
                (fi % 4) * fw, Math.floor(fi / 4) * fh, fw, fh,
                pad, pad, s - pad * 2, s - pad * 2);
        }
    };

    Scene_CharacterCreate.prototype._doCreate = function () {
        if (this._busy) return;
        var self = this;
        var name = this._nameInput.value.trim();
        if (!name) {
            L2_Message.warning('Please enter a character name.');
            return;
        }
        var classOpt = this._classSelect.getSelected();
        var classId = classOpt ? classOpt.value : 1;
        var face = FACE_PRESETS[this._selectedFace] || FACE_PRESETS[0];

        this._busy = true;
        $MMO.http.post('/api/characters', {
            name: name,
            class_id: classId,
            walk_name: face.faceName,
            walk_index: face.faceIndex,
            face_name: face.faceName,
            face_index: face.faceIndex
        }).then(function () {
            self._cleanup();
            SceneManager.goto(Scene_CharacterSelect);
        }).catch(function (e) {
            self._busy = false;
            L2_Message.error(e.message);
        });
    };

    Scene_CharacterCreate.prototype._cleanup = function () {
        this._inputs.forEach(removeEl);
        this._inputs = [];
    };

    Scene_CharacterCreate.prototype.terminate = function () {
        Scene_Base.prototype.terminate.call(this);
        if (this._focusTimer) { clearTimeout(this._focusTimer); this._focusTimer = null; }
        this._cleanup();
    };

    // -----------------------------------------------------------------
    // Override title screen → Scene_Login
    // -----------------------------------------------------------------
    Scene_Title.prototype.start = function () {
        Scene_Base.prototype.start.call(this);
        this.playTitleMusic();
        $MMO.disconnect();
        $MMO.token = null;
        $MMO.charID = null;
        SceneManager.goto(Scene_Login);
    };

    window.Scene_Login = Scene_Login;
    window.Scene_CharacterSelect = Scene_CharacterSelect;
    window.Scene_CharacterCreate = Scene_CharacterCreate;

})();
