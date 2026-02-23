/*:
 * @plugindesc v1.0.0 MMO Game Window - Floating L2-style windows (status, system, action bar).
 * @author MMO Framework
 */

(function () {
    'use strict';

    // =================================================================
    //  GameWindow — Reusable floating window (L2 style) with title bar,
    //  close button, and drag-to-move. Subclass and override drawContent().
    // =================================================================
    var GW_TITLE_H = 26;

    function GameWindow(opts) { this.initialize(opts); }
    GameWindow.prototype = Object.create(L2_Base.prototype);
    GameWindow.prototype.constructor = GameWindow;

    GameWindow.prototype.initialize = function (opts) {
        opts = opts || {};
        this._gwTitle = opts.title || '';
        this._gwKey = opts.key || '';
        this._gwClosable = opts.closable !== false;
        this._closeHover = false;
        var w = opts.width || 300, h = opts.height || 400;
        var x = Math.floor((Graphics.boxWidth - w) / 2);
        var y = Math.floor((Graphics.boxHeight - h) / 2);
        L2_Base.prototype.initialize.call(this, x, y, w, h);
        this.visible = false;
        $MMO.makeDraggable(this, 'gw_' + this._gwKey, {
            dragArea: { y: 0, h: GW_TITLE_H }
        });
    };

    GameWindow.prototype.standardPadding = function () { return 0; };
    GameWindow.prototype.contentTop = function () { return GW_TITLE_H; };
    GameWindow.prototype.contentHeight = function () { return this.ch() - GW_TITLE_H; };

    GameWindow.prototype.refresh = function () {
        var c = this.bmp();
        c.clear();
        var w = this.cw(), h = this.ch();
        // Background
        L2_Theme.fillRoundRect(c, 0, 0, w, h, 4, 'rgba(13,13,26,0.92)');
        L2_Theme.strokeRoundRect(c, 0, 0, w, h, 4, L2_Theme.borderDark);
        // Title bar
        c.fillRect(1, 1, w - 2, GW_TITLE_H - 1, 'rgba(25,25,50,0.98)');
        c.fillRect(0, GW_TITLE_H, w, 1, L2_Theme.borderDark);
        c.fontSize = 12;
        c.textColor = L2_Theme.textGold;
        c.drawText(this._gwTitle, 8, 0, w - 36, GW_TITLE_H, 'center');
        if (this._gwClosable) L2_Theme.drawCloseBtn(c, w - 22, 4, this._closeHover);
        this.drawContent(c, w, h);
    };

    GameWindow.prototype.drawContent = function () {};

    GameWindow.prototype.toggle = function () {
        this.visible = !this.visible;
        if (this.visible) this.onOpen();
    };
    GameWindow.prototype.onOpen = function () { this.refresh(); };
    GameWindow.prototype.close = function () { this.visible = false; };

    GameWindow.prototype.update = function () {
        L2_Base.prototype.update.call(this);
        if (!this.visible) return;
        if ($MMO.updateDrag(this)) return;
        var mx = TouchInput.x - this.x, my = TouchInput.y - this.y;
        var w = this.cw();
        var wasHover = this._closeHover;
        this._closeHover = this._gwClosable && mx >= w - 26 && mx <= w - 2 && my >= 2 && my <= GW_TITLE_H - 2;
        if (this._closeHover !== wasHover) this.refresh();
        if (TouchInput.isTriggered() && this._closeHover) { this.close(); return; }
        this.updateContent();
    };

    GameWindow.prototype.updateContent = function () {};

    window.GameWindow = GameWindow;

    // =================================================================
    //  StatusWindow — detailed character stats (L2 style 状态 window).
    // =================================================================
    function StatusWindow() {
        GameWindow.prototype.initialize.call(this, {
            key: 'status', title: '状态', width: 300, height: 360
        });
        this._faceBmp = null;
    }
    StatusWindow.prototype = Object.create(GameWindow.prototype);
    StatusWindow.prototype.constructor = StatusWindow;

    StatusWindow.prototype.onOpen = function () {
        var s = $MMO._lastSelf;
        if (s && s.face_name && !this._faceBmp) {
            var self = this;
            this._faceBmp = ImageManager.loadFace(s.face_name);
            this._faceBmp.addLoadListener(function () { self.refresh(); });
        }
        this.refresh();
    };

    StatusWindow.prototype.drawContent = function (c, w, h) {
        var s = $MMO._lastSelf || {};
        var y = this.contentTop() + 6;
        var P = 10, cw = w - P * 2;

        // Face
        var faceSize = 64;
        c.fillRect(P, y, faceSize, faceSize, '#111122');
        L2_Theme.strokeRoundRect(c, P, y, faceSize, faceSize, 2, L2_Theme.borderDark);
        if (this._faceBmp && this._faceBmp.isReady() && s.face_name) {
            var fi = s.face_index || 0;
            var sx = (fi % 4) * 144, sy = Math.floor(fi / 4) * 144;
            c.blt(this._faceBmp, sx, sy, 144, 144, P + 2, y + 2, faceSize - 4, faceSize - 4);
        }

        // Name + Level + Class
        var ix = P + faceSize + 8, iw = w - ix - P;
        c.fontSize = 14; c.textColor = L2_Theme.textWhite;
        c.drawText(s.name || $MMO.charName || '???', ix, y, iw, 18, 'left');
        c.fontSize = 11; c.textColor = L2_Theme.textGold;
        c.drawText('Lv. ' + (s.level || 1), ix, y + 18, 50, 14, 'left');
        if (s.class_name) {
            c.textColor = L2_Theme.textGray;
            c.drawText(s.class_name, ix + 50, y + 18, iw - 50, 14, 'left');
        }

        // Bars
        var barX = ix, barW = iw, barY = y + 36;
        var hp = s.hp != null ? s.hp : 100, maxHP = s.max_hp || 1;
        L2_Theme.drawBar(c, barX, barY, barW, 14, hp / maxHP, L2_Theme.hpBg, L2_Theme.hpFill);
        c.fontSize = 10; c.textColor = L2_Theme.textWhite;
        c.drawText('HP ' + hp + '/' + maxHP, barX + 3, barY, barW - 6, 14, 'left');
        barY += 16;
        var mp = s.mp != null ? s.mp : 50, maxMP = s.max_mp || 1;
        L2_Theme.drawBar(c, barX, barY, barW, 12, mp / maxMP, L2_Theme.mpBg, L2_Theme.mpFill);
        c.fontSize = 10;
        c.drawText('MP ' + mp + '/' + maxMP, barX + 3, barY, barW - 6, 12, 'left');
        barY += 14;
        var exp = s.exp || 0, maxExp = s.next_exp || 1;
        var expR = maxExp > 0 ? Math.min(exp / maxExp, 1) : 0;
        L2_Theme.drawBar(c, barX, barY, barW, 10, expR, '#1a1a00', '#CCCC00');
        c.fontSize = 9; c.textColor = L2_Theme.textGray;
        c.drawText('EXP ' + Math.floor(expR * 100) + '%', barX + 3, barY, barW - 6, 10, 'left');

        y += faceSize + 10;
        c.fillRect(P, y, cw, 1, L2_Theme.borderDark); y += 6;

        // Combat stats section
        c.fontSize = 11; c.textColor = L2_Theme.textGold;
        c.drawText('战斗数值', P, y, cw, 14, 'left'); y += 18;
        var halfW = Math.floor(cw / 2);
        var _sl = function (lbl, val, sx, sy) {
            c.textColor = L2_Theme.textGray; c.drawText(lbl, sx, sy, 64, 14, 'left');
            c.textColor = L2_Theme.textWhite; c.drawText(String(val), sx + 64, sy, halfW - 72, 14, 'right');
        };
        c.fontSize = 11;
        _sl('攻击力', s.attack || 0, P, y);       _sl('魔法攻击', s.magic_attack || 0, P + halfW, y); y += 16;
        _sl('防御力', s.defense || 0, P, y);       _sl('魔法防御', s.magic_defense || 0, P + halfW, y); y += 16;
        _sl('命中率', s.accuracy || 0, P, y);      _sl('回避率', s.evasion || 0, P + halfW, y); y += 16;
        _sl('攻击速度', s.attack_speed || 0, P, y); _sl('移动速度', s.speed || 0, P + halfW, y); y += 20;

        c.fillRect(P, y, cw, 1, L2_Theme.borderDark); y += 6;

        // Base stats section
        c.fontSize = 11; c.textColor = L2_Theme.textGold;
        c.drawText('基本能力', P, y, cw, 14, 'left'); y += 18;
        var thirdW = Math.floor(cw / 3);
        var _bs = function (lbl, val, sx, sy) {
            c.textColor = L2_Theme.textGray; c.drawText(lbl, sx, sy, 28, 14, 'left');
            c.textColor = L2_Theme.textWhite; c.drawText(String(val), sx + 28, sy, thirdW - 36, 14, 'right');
        };
        c.fontSize = 11;
        _bs('力量', s.str || 0, P, y);            _bs('敏捷', s.dex || 0, P + thirdW, y);        _bs('体质', s.con || 0, P + thirdW * 2, y); y += 16;
        _bs('智力', s.intelligence || 0, P, y);   _bs('智慧', s.wis || 0, P + thirdW, y);        _bs('精神', s.spirit || 0, P + thirdW * 2, y);
    };

    // =================================================================
    //  SystemMenu — small popup with system options.
    // =================================================================
    var SYS_ITEMS = [
        { label: '返回登录', action: 'logout' },
        { label: '关闭', action: 'close' }
    ];
    var SYS_ITEM_H = 28;

    function SystemMenu() {
        var h = SYS_ITEMS.length * SYS_ITEM_H + GW_TITLE_H + 8;
        GameWindow.prototype.initialize.call(this, {
            key: 'system', title: '系统', width: 160, height: h
        });
        this._hoverIdx = -1;
    }
    SystemMenu.prototype = Object.create(GameWindow.prototype);
    SystemMenu.prototype.constructor = SystemMenu;

    SystemMenu.prototype.drawContent = function (c, w) {
        var self = this;
        SYS_ITEMS.forEach(function (item, i) {
            var iy = GW_TITLE_H + 4 + i * SYS_ITEM_H;
            if (i === self._hoverIdx) c.fillRect(4, iy, w - 8, SYS_ITEM_H, L2_Theme.highlight);
            c.fontSize = 12; c.textColor = L2_Theme.textWhite;
            c.drawText(item.label, 12, iy, w - 24, SYS_ITEM_H, 'left');
            if (i < SYS_ITEMS.length - 1) c.fillRect(12, iy + SYS_ITEM_H - 1, w - 24, 1, L2_Theme.borderDark);
        });
    };

    SystemMenu.prototype.updateContent = function () {
        var mx = TouchInput.x - this.x, my = TouchInput.y - this.y;
        var w = this.cw();
        var inside = mx >= 0 && mx < w && my >= GW_TITLE_H;
        var oldHover = this._hoverIdx;
        if (inside) {
            var idx = Math.floor((my - GW_TITLE_H - 4) / SYS_ITEM_H);
            this._hoverIdx = (idx >= 0 && idx < SYS_ITEMS.length) ? idx : -1;
        } else {
            this._hoverIdx = -1;
        }
        if (this._hoverIdx !== oldHover) this.refresh();
        if (TouchInput.isTriggered() && this._hoverIdx >= 0) {
            var action = SYS_ITEMS[this._hoverIdx].action;
            if (action === 'logout') {
                this.close();
                $MMO.disconnect();
                SceneManager.goto(Scene_Title);
            } else if (action === 'close') {
                this.close();
            }
        }
    };

    // =================================================================
    //  ActionBar — bottom-right button strip for opening windows.
    //  Positioned above the skill bar to avoid overlap.
    // =================================================================
    var AB_BTNS = [
        { label: '状态', action: 'status' },
        { label: '背包', action: 'inventory' },
        { label: '好友', action: 'friends' },
        { label: '公会', action: 'guild' },
        { label: '系统', action: 'system' }
    ];
    var AB_BTN_W = 48, AB_BTN_H = 26, AB_GAP = 3, AB_PAD = 5;

    function ActionBar() { this.initialize.apply(this, arguments); }
    ActionBar.prototype = Object.create(L2_Base.prototype);
    ActionBar.prototype.constructor = ActionBar;

    ActionBar.prototype.initialize = function () {
        var totalW = AB_BTNS.length * (AB_BTN_W + AB_GAP) - AB_GAP + AB_PAD * 2;
        var totalH = AB_BTN_H + AB_PAD * 2;
        var x = Graphics.boxWidth - totalW - 6;
        // Place above the skill bar (skill bar is ~46px from bottom)
        var y = Graphics.boxHeight - totalH - 50;
        L2_Base.prototype.initialize.call(this, x, y, totalW, totalH);
        this._hoverIdx = -1;
        $MMO.makeDraggable(this, 'actionBar');
        this.refresh();
    };
    ActionBar.prototype.standardPadding = function () { return 0; };

    ActionBar.prototype.refresh = function () {
        var c = this.bmp(); c.clear();
        var w = this.cw(), h = this.ch();
        // Solid dark background for visibility
        L2_Theme.fillRoundRect(c, 0, 0, w, h, 4, 'rgba(10,10,24,0.88)');
        L2_Theme.strokeRoundRect(c, 0, 0, w, h, 4, L2_Theme.borderDark);
        var self = this;
        AB_BTNS.forEach(function (btn, i) {
            var bx = AB_PAD + i * (AB_BTN_W + AB_GAP), by = AB_PAD;
            // Button background — always visible fill
            var btnBg = i === self._hoverIdx ? 'rgba(60,60,120,0.90)' : 'rgba(30,30,60,0.80)';
            L2_Theme.fillRoundRect(c, bx, by, AB_BTN_W, AB_BTN_H, 3, btnBg);
            L2_Theme.strokeRoundRect(c, bx, by, AB_BTN_W, AB_BTN_H, 3,
                i === self._hoverIdx ? L2_Theme.textGold : L2_Theme.borderDark);
            c.fontSize = 12;
            c.textColor = i === self._hoverIdx ? L2_Theme.textGold : L2_Theme.textWhite;
            c.drawText(btn.label, bx, by, AB_BTN_W, AB_BTN_H, 'center');
        });
    };

    ActionBar.prototype.update = function () {
        L2_Base.prototype.update.call(this);
        if ($MMO.updateDrag(this)) return;
        var mx = TouchInput.x - this.x - AB_PAD, my = TouchInput.y - this.y - AB_PAD;
        var oldHover = this._hoverIdx;
        if (mx >= 0 && my >= 0 && my < AB_BTN_H) {
            var idx = Math.floor(mx / (AB_BTN_W + AB_GAP));
            var localX = mx - idx * (AB_BTN_W + AB_GAP);
            this._hoverIdx = (idx >= 0 && idx < AB_BTNS.length && localX < AB_BTN_W) ? idx : -1;
        } else {
            this._hoverIdx = -1;
        }
        if (this._hoverIdx !== oldHover) this.refresh();
        if (TouchInput.isTriggered() && this._hoverIdx >= 0) {
            $MMO._triggerAction(AB_BTNS[this._hoverIdx].action);
        }
    };

    // -----------------------------------------------------------------
    // Prevent map-touch (character movement) when clicking on UI panels.
    // Uses both _isMMOUI flag and instanceof L2_Base for maximum safety.
    // This is a backup layer — mmo-core.js also has this check.
    // -----------------------------------------------------------------
    var _Scene_Map_processMapTouch = Scene_Map.prototype.processMapTouch;
    Scene_Map.prototype.processMapTouch = function () {
        if (TouchInput.isTriggered() || TouchInput.isPressed()) {
            var tx = TouchInput.x, ty = TouchInput.y;
            var children = this.children;
            for (var i = children.length - 1; i >= 0; i--) {
                var child = children[i];
                if (child && child.visible &&
                    (child._isMMOUI || child instanceof L2_Base) &&
                    typeof child.isInside === 'function' && child.isInside(tx, ty)) {
                    return; // click is on UI — skip movement
                }
            }
        }
        _Scene_Map_processMapTouch.call(this);
    };

    // -----------------------------------------------------------------
    // Inject windows into Scene_Map.
    // -----------------------------------------------------------------
    var _Scene_Map_createAllWindows_gw = Scene_Map.prototype.createAllWindows;
    Scene_Map.prototype.createAllWindows = function () {
        _Scene_Map_createAllWindows_gw.call(this);
        // Status window
        $MMO._statusWindow = new StatusWindow();
        this.addChild($MMO._statusWindow);
        $MMO.registerWindow($MMO._statusWindow);
        // System menu
        $MMO._systemMenu = new SystemMenu();
        this.addChild($MMO._systemMenu);
        $MMO.registerWindow($MMO._systemMenu);
        // Action bar
        this._mmoActionBar = new ActionBar();
        this.addChild(this._mmoActionBar);
    };

})();
