/*:
 * @plugindesc v1.0.0 MMO 浮动窗口系统 — L2 风格浮动窗口（状态、系统、快捷栏）。
 * @author MMO Framework
 *
 * @help
 * 本插件提供：
 * - GameWindow: 可复用的 L2 风格浮动窗口基类（标题栏、关闭按钮、拖拽移动）
 * - StatusWindow: 角色详细状态窗口（头像、HP/MP/EXP、战斗属性、金币）
 * - SystemMenu: 系统菜单弹窗（返回登录/关闭）
 * - SkillWindow: 技能列表窗口（图标、名称、MP 消耗、快捷键标记、拖拽分配）
 * - ActionBar: 底部快捷按钮栏（2行3列，图标+标签+热键提示）
 */

(function () {
    'use strict';

    // ═══════════════════════════════════════════════════════════
    //  GameWindow — 可复用的 L2 风格浮动窗口
    //  支持标题栏、关闭按钮、拖拽移动，限制在屏幕范围内。
    // ═══════════════════════════════════════════════════════════

    /** @type {number} 标题栏高度。 */
    var GW_TITLE_H = 26;

    /**
     * 浮动窗口基类。
     * @param {Object} opts - 配置项
     * @param {string} opts.title - 窗口标题
     * @param {string} opts.key - 窗口标识键（用于拖拽位置持久化）
     * @param {boolean} opts.closable - 是否可关闭（默认 true）
     * @param {number} opts.width - 窗口宽度
     * @param {number} opts.height - 窗口高度
     */
    function GameWindow(opts) { this.initialize(opts); }
    GameWindow.prototype = Object.create(L2_Base.prototype);
    GameWindow.prototype.constructor = GameWindow;

    /**
     * 初始化浮动窗口。
     * 居中显示，启用标题栏区域拖拽。
     * @param {Object} opts - 配置项
     */
    GameWindow.prototype.initialize = function (opts) {
        opts = opts || {};
        this._gwTitle = opts.title || '';
        this._gwKey = opts.key || '';
        this._gwClosable = opts.closable !== false;
        this._closeHover = false;
        var w = opts.width || 300, h = opts.height || 400;

        // 居中初始化。
        var x = Math.floor((Graphics.boxWidth - w) / 2);
        var y = Math.floor((Graphics.boxHeight - h) / 2);

        L2_Base.prototype.initialize.call(this, x, y, w, h);
        this.visible = false;

        // 启用窗口大小调整时自动居中。
        this._isCentered = true;

        // 仅标题栏区域可拖拽。
        $MMO.makeDraggable(this, 'gw_' + this._gwKey, {
            dragArea: { y: 0, h: GW_TITLE_H }
        });
    };

    /** 无内边距。 */
    GameWindow.prototype.standardPadding = function () { return 0; };
    /** 内容区域起始 Y 坐标（标题栏下方）。 */
    GameWindow.prototype.contentTop = function () { return GW_TITLE_H; };
    /** 内容区域可用高度。 */
    GameWindow.prototype.contentHeight = function () { return this.ch() - GW_TITLE_H; };

    /**
     * 重绘窗口。
     * 绘制背景、边框、标题栏、关闭按钮，然后调用 drawContent。
     */
    GameWindow.prototype.refresh = function () {
        var c = this.bmp();
        c.clear();
        var w = this.cw(), h = this.ch();
        // 背景。
        L2_Theme.fillRoundRect(c, 0, 0, w, h, 4, 'rgba(13,13,26,0.92)');
        L2_Theme.strokeRoundRect(c, 0, 0, w, h, 4, L2_Theme.borderDark);
        // 标题栏。
        c.fillRect(1, 1, w - 2, GW_TITLE_H - 1, 'rgba(25,25,50,0.98)');
        c.fillRect(0, GW_TITLE_H, w, 1, L2_Theme.borderDark);
        c.fontSize = 12;
        c.textColor = L2_Theme.textGold;
        c.drawText(this._gwTitle, 8, 0, w - 36, GW_TITLE_H, 'center');
        // 关闭按钮。
        if (this._gwClosable) L2_Theme.drawCloseBtn(c, w - 22, 4, this._closeHover);
        this.drawContent(c, w, h);
    };

    /**
     * 绘制窗口内容（由子类覆写）。
     * @param {Bitmap} c - 绘图上下文
     * @param {number} w - 内容宽度
     * @param {number} h - 内容高度
     */
    GameWindow.prototype.drawContent = function () {};

    /**
     * 切换窗口显示/隐藏。
     * 显示时触发 onOpen 回调。
     */
    GameWindow.prototype.toggle = function () {
        this.visible = !this.visible;
        if (this.visible) this.onOpen();
    };

    /** 窗口打开时的回调，刷新内容。 */
    GameWindow.prototype.onOpen = function () { this.refresh(); };
    /** 关闭窗口。 */
    GameWindow.prototype.close = function () { this.visible = false; };

    /**
     * 每帧更新。
     * 处理拖拽、关闭按钮悬停/点击、内容更新。
     */
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

    /**
     * 更新窗口内容（由子类覆写）。
     */
    GameWindow.prototype.updateContent = function () {};

    window.GameWindow = GameWindow;

    // ═══════════════════════════════════════════════════════════
    //  StatusWindow — 角色详细状态窗口
    //  显示头像、名称/等级/职业、HP/MP/EXP 条、
    //  战斗属性（攻防敏运）和金币。
    // ═══════════════════════════════════════════════════════════

    /**
     * 角色状态窗口。
     * 从 $MMO._lastSelf 获取角色数据，使用角色行走图作为头像。
     */
    function StatusWindow() {
        GameWindow.prototype.initialize.call(this, {
            key: 'status', title: '状态', width: 300, height: 280
        });
        /** @type {Bitmap|null} 头像位图。 */
        this._faceBmp = null;
    }
    StatusWindow.prototype = Object.create(GameWindow.prototype);
    StatusWindow.prototype.constructor = StatusWindow;

    /**
     * 窗口打开回调。
     * 加载角色行走图（本项目无 img/faces/ 目录，使用 img/characters/）。
     */
    StatusWindow.prototype.onOpen = function () {
        var s = $MMO._lastSelf;
        var faceName = s && (s.face_name || s.walk_name);
        if (faceName && !this._faceBmp) {
            var self = this;
            // 本项目无 img/faces/ 目录 — 头像实际是 img/characters/ 中的角色行走图。
            // 加载为角色精灵图并提取正面朝下帧。
            this._faceBmp = ImageManager.loadCharacter(faceName);
            this._faceIsCharSprite = true;
            this._faceBmp.addLoadListener(function () { self.refresh(); });
        }
        this.refresh();
    };

    /**
     * 绘制状态窗口内容。
     * 包括：头像、名称/等级/职业、HP/MP/EXP 条、
     * 战斗属性（攻击力/防御力/魔法攻击/魔法防御/敏捷/幸运）、金币。
     * @param {Bitmap} c - 绘图上下文
     * @param {number} w - 内容宽度
     * @param {number} h - 内容高度
     */
    StatusWindow.prototype.drawContent = function (c, w, h) {
        var s = $MMO._lastSelf || {};
        var y = this.contentTop() + 6;
        var P = 10, cw = w - P * 2;

        // ── 头像 ──
        var faceSize = 64;
        c.fillRect(P, y, faceSize, faceSize, '#111122');
        L2_Theme.strokeRoundRect(c, P, y, faceSize, faceSize, 2, L2_Theme.borderDark);
        if (this._faceBmp && this._faceBmp.isReady() && (s.face_name || s.walk_name)) {
            var fi = s.face_index != null ? s.face_index : (s.walk_index || 0);
            if (this._faceIsCharSprite) {
                // 角色精灵图：提取正面朝下的中间帧。
                // 标准 RMMV 角色图：4列 * 2行角色，
                // 每个角色格 = 3帧宽 * 4方向高。
                var isBig = (s.face_name || s.walk_name).charAt(0) === '$';
                var pw, ph, cx, cy;
                if (isBig) {
                    // 单角色图（$ 前缀）：3帧 * 4方向
                    pw = this._faceBmp.width / 3;
                    ph = this._faceBmp.height / 4;
                    cx = pw;  // 中间帧（第1列）
                    cy = 0;   // 朝下方向（第0行）
                } else {
                    // 标准8角色图：4角色宽 * 2角色高
                    var charW = this._faceBmp.width / 4;
                    var charH = this._faceBmp.height / 2;
                    pw = charW / 3;  // 单帧宽度
                    ph = charH / 4;  // 单帧高度
                    var col = fi % 4, row = Math.floor(fi / 4);
                    cx = col * charW + pw;  // 角色格内的中间帧
                    cy = row * charH;       // 朝下方向（格内第0行）
                }
                c.blt(this._faceBmp, cx, cy, pw, ph, P + 2, y + 2, faceSize - 4, faceSize - 4);
            } else {
                // 标准 RMMV 头像图：4列 * 2行，每个 144x144。
                var sx = (fi % 4) * 144, sy = Math.floor(fi / 4) * 144;
                c.blt(this._faceBmp, sx, sy, 144, 144, P + 2, y + 2, faceSize - 4, faceSize - 4);
            }
        }

        // ── 名称 + 等级 + 职业 ──
        var ix = P + faceSize + 8, iw = w - ix - P;
        c.fontSize = 14; c.textColor = L2_Theme.textWhite;
        c.drawText(s.name || $MMO.charName || '???', ix, y, iw, 18, 'left');
        c.fontSize = 11; c.textColor = L2_Theme.textGold;
        c.drawText('Lv. ' + (s.level || 1), ix, y + 18, 50, 14, 'left');
        if (s.class_name) {
            c.textColor = L2_Theme.textGray;
            c.drawText(s.class_name, ix + 50, y + 18, iw - 50, 14, 'left');
        }

        // ── HP/MP/EXP 条 ──
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

        // ── 战斗属性（RMMV: atk/def/mat/mdf/agi/luk）──
        c.fontSize = 11; c.textColor = L2_Theme.textGold;
        c.drawText('战斗数值', P, y, cw, 14, 'left'); y += 18;
        var halfW = Math.floor(cw / 2);
        /** 绘制单个属性标签+数值。 */
        var _sl = function (lbl, val, sx, sy) {
            c.textColor = L2_Theme.textGray; c.drawText(lbl, sx, sy, 64, 14, 'left');
            c.textColor = L2_Theme.textWhite; c.drawText(String(val), sx + 64, sy, halfW - 72, 14, 'right');
        };
        c.fontSize = 11;
        _sl('攻击力', s.atk || 0, P, y);       _sl('魔法攻击', s.mat || 0, P + halfW, y); y += 16;
        _sl('防御力', s.def || 0, P, y);       _sl('魔法防御', s.mdf || 0, P + halfW, y); y += 16;
        _sl('敏捷', s.agi || 0, P, y);         _sl('幸运', s.luk || 0, P + halfW, y); y += 16;

        c.fillRect(P, y, cw, 1, L2_Theme.borderDark); y += 6;

        // ── 金币 ──
        c.fontSize = 11; c.textColor = L2_Theme.textGold;
        c.drawText('金币', P, y, 40, 14, 'left');
        c.textColor = L2_Theme.textWhite;
        c.drawText(String(s.gold || 0), P + 40, y, cw - 40, 14, 'left');
    };

    // ═══════════════════════════════════════════════════════════
    //  SystemMenu — 系统菜单弹窗
    //  提供返回登录和关闭选项。
    // ═══════════════════════════════════════════════════════════

    /** @type {Array} 系统菜单选项。 */
    var SYS_ITEMS = [
        { label: '返回登录', action: 'logout' },
        { label: '关闭', action: 'close' }
    ];
    /** @type {number} 菜单项高度。 */
    var SYS_ITEM_H = 28;

    /**
     * 系统菜单窗口。
     */
    function SystemMenu() {
        var h = SYS_ITEMS.length * SYS_ITEM_H + GW_TITLE_H + 8;
        GameWindow.prototype.initialize.call(this, {
            key: 'system', title: '系统', width: 160, height: h
        });
        this._hoverIdx = -1;
    }
    SystemMenu.prototype = Object.create(GameWindow.prototype);
    SystemMenu.prototype.constructor = SystemMenu;

    /**
     * 绘制系统菜单内容。
     * @param {Bitmap} c - 绘图上下文
     * @param {number} w - 内容宽度
     */
    SystemMenu.prototype.drawContent = function (c, w) {
        var self = this;
        SYS_ITEMS.forEach(function (item, i) {
            var iy = GW_TITLE_H + 4 + i * SYS_ITEM_H;
            // 悬停高亮。
            if (i === self._hoverIdx) c.fillRect(4, iy, w - 8, SYS_ITEM_H, L2_Theme.highlight);
            c.fontSize = 12; c.textColor = L2_Theme.textWhite;
            c.drawText(item.label, 12, iy, w - 24, SYS_ITEM_H, 'left');
            // 分隔线。
            if (i < SYS_ITEMS.length - 1) c.fillRect(12, iy + SYS_ITEM_H - 1, w - 24, 1, L2_Theme.borderDark);
        });
    };

    /**
     * 更新系统菜单交互。
     * 处理悬停高亮和点击事件（登出/关闭）。
     */
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

    // ═══════════════════════════════════════════════════════════
    //  SkillWindow — 技能列表窗口
    //  显示角色已学技能，支持滚动、悬停高亮、拖拽到快捷栏。
    // ═══════════════════════════════════════════════════════════

    /** @type {number} 技能项高度。 */
    var SK_ITEM_H = 36;
    /** @type {number} IconSet 列数。 */
    var SK_ICON_COLS = 16;
    /** @type {number} 内边距。 */
    var SK_PAD = 6;
    /** @type {number} 窗口宽度。 */
    var SK_W = 220;
    /** @type {number} 最大可见技能数。 */
    var SK_MAX_VIS = 6;

    /**
     * 技能列表窗口。
     * 从 $MMO._knownSkills 获取技能列表。
     */
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

    /**
     * 窗口打开回调。
     * 从 $MMO._knownSkills 复制技能列表，重置滚动和选择状态。
     */
    SkillWindow.prototype.onOpen = function () {
        this._skills = ($MMO._knownSkills || []).slice();
        this._scrollY = 0;
        this._hoverIdx = -1;
        this._dragIdx = -1;
        this._dragStartX = 0;
        this._dragStartY = 0;
        this.refresh();
    };

    /**
     * 绘制技能列表内容。
     * 包括：技能图标、名称、MP消耗/CD、快捷键标记、滚动条。
     * @param {Bitmap} c - 绘图上下文
     * @param {number} w - 内容宽度
     * @param {number} h - 内容高度
     */
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

            // 行背景（悬停时高亮）。
            if (isHover) c.fillRect(SK_PAD, iy, w - SK_PAD * 2, SK_ITEM_H - 2, L2_Theme.highlight);

            // 技能图标。
            if (self._iconSet && sk.icon_index) {
                var sx = (sk.icon_index % SK_ICON_COLS) * 32;
                var sy = Math.floor(sk.icon_index / SK_ICON_COLS) * 32;
                c.blt(self._iconSet, sx, sy, 32, 32, SK_PAD + 2, iy + 2, 28, 28);
            }

            // 技能名称。
            c.fontSize = 12;
            c.textColor = L2_Theme.textWhite;
            c.drawText(sk.name || 'Skill', SK_PAD + 34, iy, w - SK_PAD * 2 - 34, 18, 'left');

            // MP 消耗 + 冷却时间。
            c.fontSize = 10;
            c.textColor = L2_Theme.textGray;
            var info = 'MP: ' + (sk.mp_cost || 0);
            if (sk.cd_ms > 0) info += '  CD: ' + (sk.cd_ms / 1000) + 's';
            c.drawText(info, SK_PAD + 34, iy + 16, w - SK_PAD * 2 - 34, 14, 'left');

            // 快捷键标记（F键）— 通过 skill_id 匹配查找。
            var barIdx = -1;
            for (var b = 0; b < $MMO._skillBar.length; b++) {
                if ($MMO._skillBar[b] && $MMO._skillBar[b].skill_id === sk.skill_id) { barIdx = b; break; }
            }
            if (barIdx >= 0) {
                c.fontSize = 9;
                c.textColor = L2_Theme.textGold;
                c.drawText('F' + (barIdx + 1), w - SK_PAD - 26, iy + 4, 22, 12, 'right');
            }

            // 行分隔线。
            c.fillRect(SK_PAD, iy + SK_ITEM_H - 2, w - SK_PAD * 2, 1, L2_Theme.borderDark);
        }

        // 空状态提示。
        if (skills.length === 0) {
            c.fontSize = 12;
            c.textColor = L2_Theme.textGray;
            c.drawText('暂无技能', 0, topY + 20, w, 18, 'center');
        }

        // 底部拖拽提示。
        if (skills.length > 0) {
            c.fontSize = 9;
            c.textColor = L2_Theme.textGray;
            c.drawText('拖拽技能到快捷栏使用', 0, h - 14, w, 12, 'center');
        }

        // 滚动条。
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

    /**
     * 更新技能窗口交互。
     * 处理拖拽（技能 → 快捷栏）、悬停、点击、滚动。
     */
    SkillWindow.prototype.updateContent = function () {
        var mx = TouchInput.x - this.x, my = TouchInput.y - this.y;
        var topY = this.contentTop() + SK_PAD;
        var h = this.ch();
        var listH = h - topY - SK_PAD;

        // 处理活跃拖拽（技能 → 快捷栏）。
        if (this._dragIdx >= 0) {
            if (TouchInput.isPressed()) {
                $MMO._uiDrag = {
                    type: 'skill',
                    data: this._skills[this._dragIdx],
                    x: TouchInput.x,
                    y: TouchInput.y
                };
            } else {
                // 释放 — 检查是否放置在快捷栏槽位上。
                $MMO._handleDrop(TouchInput.x, TouchInput.y);
                this._dragIdx = -1;
                $MMO._uiDrag = null;
            }
            return;
        }

        // 悬停检测。
        var oldHover = this._hoverIdx;
        this._hoverIdx = -1;
        if (mx >= SK_PAD && mx < this.cw() - SK_PAD && my >= topY && my < topY + listH) {
            var idx = Math.floor((my - topY + this._scrollY) / SK_ITEM_H);
            if (idx >= 0 && idx < this._skills.length) this._hoverIdx = idx;
        }
        if (this._hoverIdx !== oldHover) this.refresh();

        // 按下+移动开始拖拽。
        if (TouchInput.isTriggered() && this._hoverIdx >= 0) {
            this._dragIdx = this._hoverIdx;
            this._dragStartX = TouchInput.x;
            this._dragStartY = TouchInput.y;
        }

        // 滚轮滚动。
        if (this.isInside(TouchInput.x, TouchInput.y) && TouchInput.wheelY) {
            var totalH = this._skills.length * SK_ITEM_H;
            var maxScroll = Math.max(0, totalH - listH);
            this._scrollY += TouchInput.wheelY > 0 ? SK_ITEM_H * 2 : -SK_ITEM_H * 2;
            this._scrollY = Math.max(0, Math.min(this._scrollY, maxScroll));
            this.refresh();
        }
    };

    window.SkillWindow = SkillWindow;

    // ═══════════════════════════════════════════════════════════
    //  ActionBar — 底部快捷按钮栏（2行3列）
    //  显示图标+标签+热键提示，悬停时显示 tooltip。
    // ═══════════════════════════════════════════════════════════

    /** @type {Array} 快捷按钮定义。 */
    var AB_BTNS = [
        { label: '角色', action: 'status',    icon: 84,  hotkey: 'Alt+T' },
        { label: '背包', action: 'inventory', icon: 176, hotkey: 'Alt+I' },
        { label: '技能', action: 'skills',    icon: 79,  hotkey: 'Alt+S' },
        { label: '好友', action: 'friends',   icon: 75,  hotkey: 'Alt+F' },
        { label: '公会', action: 'guild',     icon: 83,  hotkey: 'Alt+G' },
        { label: '系统', action: 'system',    icon: 236, hotkey: 'ESC' }
    ];
    /** @type {number} 列数。 */
    var AB_COLS = 3;
    /** @type {number} 行数。 */
    var AB_ROWS = 2;
    /** @type {number} 按钮尺寸。 */
    var AB_BTN_SIZE = 38;
    /** @type {number} 按钮间距。 */
    var AB_GAP = 2;
    /** @type {number} 外边距。 */
    var AB_PAD = 4;
    /** @type {number} 提示条高度。 */
    var AB_TOOLTIP_H = 18;
    /** @type {number} IconSet 列数。 */
    var AB_ICON_COLS = 16;

    /**
     * 底部快捷按钮栏。
     */
    function ActionBar() { this.initialize.apply(this, arguments); }
    ActionBar.prototype = Object.create(L2_Base.prototype);
    ActionBar.prototype.constructor = ActionBar;

    /**
     * 初始化快捷栏。
     * 定位在屏幕右下角，加载 IconSet，启用拖拽。
     */
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

    /** 无内边距。 */
    ActionBar.prototype.standardPadding = function () { return 0; };

    /**
     * 覆写 isInside — 仅按钮面板区域响应点击，
     * 不包括上方的透明 tooltip 区域。
     * @param {number} mx - 屏幕 X
     * @param {number} my - 屏幕 Y
     * @returns {boolean}
     */
    ActionBar.prototype.isInside = function (mx, my) {
        var panelTop = this.y + AB_TOOLTIP_H;
        return mx >= this.x && mx <= this.x + this.width &&
               my >= panelTop && my <= this.y + this.height;
    };

    /**
     * 重绘快捷栏。
     * 绘制面板背景、按钮（图标+标签）、悬停 tooltip。
     */
    ActionBar.prototype.refresh = function () {
        var c = this.bmp(); c.clear();
        var w = this.cw(), h = this.ch();
        var btnY = AB_TOOLTIP_H + AB_PAD;

        // 面板背景。
        L2_Theme.fillRoundRect(c, 0, AB_TOOLTIP_H, w, h - AB_TOOLTIP_H, 4, 'rgba(10,10,24,0.88)');
        L2_Theme.strokeRoundRect(c, 0, AB_TOOLTIP_H, w, h - AB_TOOLTIP_H, 4, L2_Theme.borderDark);

        var self = this;
        AB_BTNS.forEach(function (btn, i) {
            var col = i % AB_COLS;
            var row = Math.floor(i / AB_COLS);
            var bx = AB_PAD + col * (AB_BTN_SIZE + AB_GAP);
            var by = btnY + row * (AB_BTN_SIZE + AB_GAP);
            var isHover = (i === self._hoverIdx);

            // 按钮背景。
            var bg = isHover ? 'rgba(60,60,120,0.92)' : 'rgba(25,25,50,0.80)';
            L2_Theme.fillRoundRect(c, bx, by, AB_BTN_SIZE, AB_BTN_SIZE, 3, bg);
            L2_Theme.strokeRoundRect(c, bx, by, AB_BTN_SIZE, AB_BTN_SIZE, 3,
                isHover ? L2_Theme.textGold : L2_Theme.borderDark);

            // 图标（从 IconSet 居中绘制，32x32 缩放到 24x24）。
            if (self._iconSet) {
                var iconIdx = btn.icon || 0;
                var sx = (iconIdx % AB_ICON_COLS) * 32;
                var sy = Math.floor(iconIdx / AB_ICON_COLS) * 32;
                c.blt(self._iconSet, sx, sy, 32, 32, bx + 7, by + 2, 24, 24);
            }

            // 图标下方的文字标签。
            c.fontSize = 9;
            c.textColor = isHover ? L2_Theme.textGold : L2_Theme.textGray;
            c.drawText(btn.label, bx, by + 25, AB_BTN_SIZE, 12, 'center');
        });

        // 悬停时显示 tooltip（按钮面板上方）。
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

    /**
     * 每帧更新快捷栏。
     * 处理拖拽、按钮悬停检测和点击触发。
     */
    ActionBar.prototype.update = function () {
        L2_Base.prototype.update.call(this);
        if ($MMO.updateDrag(this)) return;
        var mx = TouchInput.x - this.x;
        var my = TouchInput.y - this.y;
        var btnY = AB_TOOLTIP_H + AB_PAD;
        var oldHover = this._hoverIdx;

        // 按钮网格悬停检测。
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
        // 点击按钮触发对应操作。
        if (TouchInput.isTriggered() && this._hoverIdx >= 0) {
            $MMO._triggerAction(AB_BTNS[this._hoverIdx].action);
        }
    };

    // ═══════════════════════════════════════════════════════════
    //  注入 Scene_Map
    //  在 Scene_Map 创建窗口时注入状态窗口、技能窗口、
    //  系统菜单和快捷栏。
    // ═══════════════════════════════════════════════════════════

    /**
     * 覆写 Scene_Map.createAllWindows。
     * 注入 StatusWindow、SkillWindow、SystemMenu、ActionBar。
     */
    var _Scene_Map_createAllWindows_gw = Scene_Map.prototype.createAllWindows;
    Scene_Map.prototype.createAllWindows = function () {
        _Scene_Map_createAllWindows_gw.call(this);
        // 状态窗口。
        $MMO._statusWindow = new StatusWindow();
        this.addChild($MMO._statusWindow);
        $MMO.registerWindow($MMO._statusWindow);
        // 技能窗口。
        $MMO._skillWindow = new SkillWindow();
        this.addChild($MMO._skillWindow);
        $MMO.registerWindow($MMO._skillWindow);
        // 系统菜单。
        $MMO._systemMenu = new SystemMenu();
        this.addChild($MMO._systemMenu);
        $MMO.registerWindow($MMO._systemMenu);
        // 快捷栏。
        this._mmoActionBar = new ActionBar();
        this.addChild(this._mmoActionBar);
        $MMO.registerBottomUI(this._mmoActionBar);
    };

})();
