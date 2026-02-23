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
            key: 'status', title: '状态', width: 300, height: 280
        });
        this._faceBmp = null;
    }
    StatusWindow.prototype = Object.create(GameWindow.prototype);
    StatusWindow.prototype.constructor = StatusWindow;

    StatusWindow.prototype.onOpen = function () {
        var s = $MMO._lastSelf;
        var faceName = s && (s.face_name || s.walk_name);
        if (faceName && !this._faceBmp) {
            var self = this;
            // This project has no img/faces/ directory — face images are
            // actually character sprite sheets in img/characters/.
            // Load as character sprite and extract the front-facing frame.
            this._faceBmp = ImageManager.loadCharacter(faceName);
            this._faceIsCharSprite = true;
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
        if (this._faceBmp && this._faceBmp.isReady() && (s.face_name || s.walk_name)) {
            var fi = s.face_index != null ? s.face_index : (s.walk_index || 0);
            if (this._faceIsCharSprite) {
                // Character sprite sheet: extract front-facing middle frame.
                // Standard RMMV character sheet: 4 cols * 2 rows of characters,
                // each character cell = 3 frames wide * 4 directions tall.
                var isBig = (s.face_name || s.walk_name).charAt(0) === '$';
                var pw, ph, cx, cy;
                if (isBig) {
                    // Single-character sheet ($ prefix): 3 frames * 4 dirs
                    pw = this._faceBmp.width / 3;
                    ph = this._faceBmp.height / 4;
                    cx = pw;  // middle frame (column 1)
                    cy = 0;   // down-facing direction (row 0)
                } else {
                    // Standard 8-character sheet: 4 chars wide * 2 chars tall
                    var charW = this._faceBmp.width / 4;
                    var charH = this._faceBmp.height / 2;
                    pw = charW / 3;  // one frame width
                    ph = charH / 4;  // one frame height
                    var col = fi % 4, row = Math.floor(fi / 4);
                    cx = col * charW + pw;  // middle frame within character cell
                    cy = row * charH;       // down-facing direction (row 0 of cell)
                }
                c.blt(this._faceBmp, cx, cy, pw, ph, P + 2, y + 2, faceSize - 4, faceSize - 4);
            } else {
                // Standard RMMV face image: 4 cols * 2 rows, each 144x144.
                var sx = (fi % 4) * 144, sy = Math.floor(fi / 4) * 144;
                c.blt(this._faceBmp, sx, sy, 144, 144, P + 2, y + 2, faceSize - 4, faceSize - 4);
            }
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

        // Combat stats section (RMMV: atk/def/mat/mdf/agi/luk)
        c.fontSize = 11; c.textColor = L2_Theme.textGold;
        c.drawText('战斗数值', P, y, cw, 14, 'left'); y += 18;
        var halfW = Math.floor(cw / 2);
        var _sl = function (lbl, val, sx, sy) {
            c.textColor = L2_Theme.textGray; c.drawText(lbl, sx, sy, 64, 14, 'left');
            c.textColor = L2_Theme.textWhite; c.drawText(String(val), sx + 64, sy, halfW - 72, 14, 'right');
        };
        c.fontSize = 11;
        _sl('攻击力', s.atk || 0, P, y);       _sl('魔法攻击', s.mat || 0, P + halfW, y); y += 16;
        _sl('防御力', s.def || 0, P, y);       _sl('魔法防御', s.mdf || 0, P + halfW, y); y += 16;
        _sl('敏捷', s.agi || 0, P, y);         _sl('幸运', s.luk || 0, P + halfW, y); y += 16;

        c.fillRect(P, y, cw, 1, L2_Theme.borderDark); y += 6;

        // Gold
        c.fontSize = 11; c.textColor = L2_Theme.textGold;
        c.drawText('金币', P, y, 40, 14, 'left');
        c.textColor = L2_Theme.textWhite;
        c.drawText(String(s.gold || 0), P + 40, y, cw - 40, 14, 'left');
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
    //  SkillWindow — shows character's skills in a list view.
    // =================================================================
    var SK_ITEM_H = 36, SK_ICON_COLS = 16, SK_PAD = 6;
    var SK_W = 220, SK_MAX_VIS = 6;

    function SkillWindow() {
        var h = GW_TITLE_H + SK_MAX_VIS * SK_ITEM_H + SK_PAD * 2;
        GameWindow.prototype.initialize.call(this, {
            key: 'skills', title: '技能', width: SK_W, height: h
        });
        this._skills = [];
        this._hoverIdx = -1;
        this._scrollY = 0;
        this._iconSet = null;
        var self = this;
        ImageManager.loadSystem('IconSet').addLoadListener(function (bmp) {
            self._iconSet = bmp;
            if (self.visible) self.refresh();
        });
    }
    SkillWindow.prototype = Object.create(GameWindow.prototype);
    SkillWindow.prototype.constructor = SkillWindow;

    SkillWindow.prototype.onOpen = function () {
        this._skills = ($MMO._knownSkills || []).slice();
        this._scrollY = 0;
        this._hoverIdx = -1;
        this._dragIdx = -1;
        this._dragStartX = 0;
        this._dragStartY = 0;
        this.refresh();
    };

    SkillWindow.prototype.drawContent = function (c, w, h) {
        var topY = this.contentTop() + SK_PAD;
        var listH = h - topY - SK_PAD;
        var skills = this._skills;
        var self = this;

        var startIdx = Math.floor(this._scrollY / SK_ITEM_H);
        var visCount = Math.ceil(listH / SK_ITEM_H) + 1;

        for (var i = startIdx; i < Math.min(startIdx + visCount, skills.length); i++) {
            var iy = topY + (i * SK_ITEM_H - this._scrollY);
            if (iy + SK_ITEM_H < topY || iy > h - SK_PAD) continue;

            var sk = skills[i];
            var isHover = (i === self._hoverIdx);

            // Row background
            if (isHover) c.fillRect(SK_PAD, iy, w - SK_PAD * 2, SK_ITEM_H - 2, L2_Theme.highlight);

            // Icon
            if (self._iconSet && sk.icon_index) {
                var sx = (sk.icon_index % SK_ICON_COLS) * 32;
                var sy = Math.floor(sk.icon_index / SK_ICON_COLS) * 32;
                c.blt(self._iconSet, sx, sy, 32, 32, SK_PAD + 2, iy + 2, 28, 28);
            }

            // Skill name
            c.fontSize = 12;
            c.textColor = L2_Theme.textWhite;
            c.drawText(sk.name || 'Skill', SK_PAD + 34, iy, w - SK_PAD * 2 - 34, 18, 'left');

            // MP cost + CD
            c.fontSize = 10;
            c.textColor = L2_Theme.textGray;
            var info = 'MP: ' + (sk.mp_cost || 0);
            if (sk.cd_ms > 0) info += '  CD: ' + (sk.cd_ms / 1000) + 's';
            c.drawText(info, SK_PAD + 34, iy + 16, w - SK_PAD * 2 - 34, 14, 'left');

            // Hotkey label (F-key) — find by skill_id since objects may differ
            var barIdx = -1;
            for (var b = 0; b < $MMO._skillBar.length; b++) {
                if ($MMO._skillBar[b] && $MMO._skillBar[b].skill_id === sk.skill_id) { barIdx = b; break; }
            }
            if (barIdx >= 0) {
                c.fontSize = 9;
                c.textColor = L2_Theme.textGold;
                c.drawText('F' + (barIdx + 1), w - SK_PAD - 26, iy + 4, 22, 12, 'right');
            }

            // Row separator
            c.fillRect(SK_PAD, iy + SK_ITEM_H - 2, w - SK_PAD * 2, 1, L2_Theme.borderDark);
        }

        // Empty state
        if (skills.length === 0) {
            c.fontSize = 12;
            c.textColor = L2_Theme.textGray;
            c.drawText('暂无技能', 0, topY + 20, w, 18, 'center');
        }

        // Drag hint at bottom
        if (skills.length > 0) {
            c.fontSize = 9;
            c.textColor = L2_Theme.textGray;
            c.drawText('拖拽技能到快捷栏使用', 0, h - 14, w, 12, 'center');
        }

        // Scrollbar
        var totalH = skills.length * SK_ITEM_H;
        if (totalH > listH) {
            var sbW = 4;
            var thumbH = Math.max(12, Math.round(listH * (listH / totalH)));
            var maxScroll = totalH - listH;
            var thumbY = topY + Math.round((listH - thumbH) * (maxScroll > 0 ? this._scrollY / maxScroll : 0));
            c.fillRect(w - sbW, topY, sbW, listH, 'rgba(0,0,0,0.2)');
            L2_Theme.fillRoundRect(c, w - sbW, thumbY, sbW, thumbH, 2, '#444466');
        }
    };

    SkillWindow.prototype.updateContent = function () {
        var mx = TouchInput.x - this.x, my = TouchInput.y - this.y;
        var topY = this.contentTop() + SK_PAD;
        var h = this.ch();
        var listH = h - topY - SK_PAD;

        // Handle active drag (skill → skillbar)
        if (this._dragIdx >= 0) {
            if (TouchInput.isPressed()) {
                $MMO._uiDrag = {
                    type: 'skill',
                    data: this._skills[this._dragIdx],
                    x: TouchInput.x,
                    y: TouchInput.y
                };
            } else {
                // Released — check if dropped on a SkillBar slot
                $MMO._handleDrop(TouchInput.x, TouchInput.y);
                this._dragIdx = -1;
                $MMO._uiDrag = null;
            }
            return;
        }

        // Hover
        var oldHover = this._hoverIdx;
        this._hoverIdx = -1;
        if (mx >= SK_PAD && mx < this.cw() - SK_PAD && my >= topY && my < topY + listH) {
            var idx = Math.floor((my - topY + this._scrollY) / SK_ITEM_H);
            if (idx >= 0 && idx < this._skills.length) this._hoverIdx = idx;
        }
        if (this._hoverIdx !== oldHover) this.refresh();

        // Start drag on press & move
        if (TouchInput.isTriggered() && this._hoverIdx >= 0) {
            this._dragIdx = this._hoverIdx;
            this._dragStartX = TouchInput.x;
            this._dragStartY = TouchInput.y;
        }

        // Scroll
        if (this.isInside(TouchInput.x, TouchInput.y) && TouchInput.wheelY) {
            var totalH = this._skills.length * SK_ITEM_H;
            var maxScroll = Math.max(0, totalH - listH);
            this._scrollY += TouchInput.wheelY > 0 ? SK_ITEM_H * 2 : -SK_ITEM_H * 2;
            this._scrollY = Math.max(0, Math.min(this._scrollY, maxScroll));
            this.refresh();
        }
    };

    window.SkillWindow = SkillWindow;

    // =================================================================
    //  ActionBar — L2-style bottom-right icon button grid (2 rows x 3 cols).
    //  Shows icon + label per button, tooltip with hotkey on hover.
    // =================================================================
    var AB_BTNS = [
        { label: '角色', action: 'status',    icon: 84,  hotkey: 'Alt+T' },
        { label: '背包', action: 'inventory', icon: 176, hotkey: 'Alt+I' },
        { label: '技能', action: 'skills',    icon: 79,  hotkey: 'Alt+S' },
        { label: '好友', action: 'friends',   icon: 75,  hotkey: 'Alt+F' },
        { label: '公会', action: 'guild',     icon: 83,  hotkey: 'Alt+G' },
        { label: '系统', action: 'system',    icon: 236, hotkey: 'ESC' }
    ];
    var AB_COLS = 3, AB_ROWS = 2;
    var AB_BTN_SIZE = 38, AB_GAP = 2, AB_PAD = 4;
    var AB_TOOLTIP_H = 18;
    var AB_ICON_COLS = 16;

    function ActionBar() { this.initialize.apply(this, arguments); }
    ActionBar.prototype = Object.create(L2_Base.prototype);
    ActionBar.prototype.constructor = ActionBar;

    ActionBar.prototype.initialize = function () {
        var totalW = AB_COLS * (AB_BTN_SIZE + AB_GAP) - AB_GAP + AB_PAD * 2;
        var totalH = AB_TOOLTIP_H + AB_ROWS * (AB_BTN_SIZE + AB_GAP) - AB_GAP + AB_PAD * 2;
        var x = Graphics.boxWidth - totalW - 4;
        var y = Graphics.boxHeight - totalH - 4;
        L2_Base.prototype.initialize.call(this, x, y, totalW, totalH);
        this._hoverIdx = -1;
        this._iconSet = null;
        var self = this;
        ImageManager.loadSystem('IconSet').addLoadListener(function (bmp) {
            self._iconSet = bmp;
            self.refresh();
        });
        $MMO.makeDraggable(this, 'actionBar');
        this.refresh();
    };
    ActionBar.prototype.standardPadding = function () { return 0; };

    // Only block clicks on the actual button panel, not the transparent tooltip area above.
    ActionBar.prototype.isInside = function (mx, my) {
        var panelTop = this.y + AB_TOOLTIP_H;
        return mx >= this.x && mx <= this.x + this.width &&
               my >= panelTop && my <= this.y + this.height;
    };

    ActionBar.prototype.refresh = function () {
        var c = this.bmp(); c.clear();
        var w = this.cw(), h = this.ch();
        var btnY = AB_TOOLTIP_H + AB_PAD;

        // Panel background
        L2_Theme.fillRoundRect(c, 0, AB_TOOLTIP_H, w, h - AB_TOOLTIP_H, 4, 'rgba(10,10,24,0.88)');
        L2_Theme.strokeRoundRect(c, 0, AB_TOOLTIP_H, w, h - AB_TOOLTIP_H, 4, L2_Theme.borderDark);

        var self = this;
        AB_BTNS.forEach(function (btn, i) {
            var col = i % AB_COLS;
            var row = Math.floor(i / AB_COLS);
            var bx = AB_PAD + col * (AB_BTN_SIZE + AB_GAP);
            var by = btnY + row * (AB_BTN_SIZE + AB_GAP);
            var isHover = (i === self._hoverIdx);

            // Button background
            var bg = isHover ? 'rgba(60,60,120,0.92)' : 'rgba(25,25,50,0.80)';
            L2_Theme.fillRoundRect(c, bx, by, AB_BTN_SIZE, AB_BTN_SIZE, 3, bg);
            L2_Theme.strokeRoundRect(c, bx, by, AB_BTN_SIZE, AB_BTN_SIZE, 3,
                isHover ? L2_Theme.textGold : L2_Theme.borderDark);

            // Icon from IconSet (centered, 24x24 scaled from 32x32)
            if (self._iconSet) {
                var iconIdx = btn.icon || 0;
                var sx = (iconIdx % AB_ICON_COLS) * 32;
                var sy = Math.floor(iconIdx / AB_ICON_COLS) * 32;
                c.blt(self._iconSet, sx, sy, 32, 32, bx + 7, by + 2, 24, 24);
            }

            // Text label below icon
            c.fontSize = 9;
            c.textColor = isHover ? L2_Theme.textGold : L2_Theme.textGray;
            c.drawText(btn.label, bx, by + 25, AB_BTN_SIZE, 12, 'center');
        });

        // Tooltip (drawn above the button panel when hovered)
        if (self._hoverIdx >= 0) {
            var hBtn = AB_BTNS[self._hoverIdx];
            var tip = hBtn.label + ' (' + hBtn.hotkey + ')';
            L2_Theme.fillRoundRect(c, 0, 0, w, AB_TOOLTIP_H, 3, 'rgba(10,10,24,0.95)');
            L2_Theme.strokeRoundRect(c, 0, 0, w, AB_TOOLTIP_H, 3, L2_Theme.borderDark);
            c.fontSize = 11;
            c.textColor = L2_Theme.textGold;
            c.drawText(tip, 4, 0, w - 8, AB_TOOLTIP_H, 'center');
        }
    };

    ActionBar.prototype.update = function () {
        L2_Base.prototype.update.call(this);
        if ($MMO.updateDrag(this)) return;
        var mx = TouchInput.x - this.x;
        var my = TouchInput.y - this.y;
        var btnY = AB_TOOLTIP_H + AB_PAD;
        var oldHover = this._hoverIdx;

        var gridH = AB_ROWS * (AB_BTN_SIZE + AB_GAP) - AB_GAP;
        if (mx >= AB_PAD && my >= btnY && my < btnY + gridH) {
            var col = Math.floor((mx - AB_PAD) / (AB_BTN_SIZE + AB_GAP));
            var row = Math.floor((my - btnY) / (AB_BTN_SIZE + AB_GAP));
            var inBtnX = (mx - AB_PAD) - col * (AB_BTN_SIZE + AB_GAP);
            var inBtnY = (my - btnY) - row * (AB_BTN_SIZE + AB_GAP);
            var idx = row * AB_COLS + col;
            this._hoverIdx = (col >= 0 && col < AB_COLS && row >= 0 && row < AB_ROWS &&
                              idx < AB_BTNS.length && inBtnX < AB_BTN_SIZE && inBtnY < AB_BTN_SIZE) ? idx : -1;
        } else {
            this._hoverIdx = -1;
        }
        if (this._hoverIdx !== oldHover) this.refresh();
        if (TouchInput.isTriggered() && this._hoverIdx >= 0) {
            $MMO._triggerAction(AB_BTNS[this._hoverIdx].action);
        }
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
        // Skill window
        $MMO._skillWindow = new SkillWindow();
        this.addChild($MMO._skillWindow);
        $MMO.registerWindow($MMO._skillWindow);
        // System menu
        $MMO._systemMenu = new SystemMenu();
        this.addChild($MMO._systemMenu);
        $MMO.registerWindow($MMO._systemMenu);
        // Action bar
        this._mmoActionBar = new ActionBar();
        this.addChild(this._mmoActionBar);
        $MMO.registerBottomUI(this._mmoActionBar);
    };

})();
