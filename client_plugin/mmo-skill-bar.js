/*:
 * @plugindesc v2.0.0 MMO 技能快捷栏 — 12 格 F1-F12 技能/物品快捷栏（L2 UI）。
 * @author MMO Framework
 *
 * @help
 * 本插件实现底部居中的 12 格技能快捷栏：
 *
 * 功能特性：
 * - F1-F12 快捷键直接释放技能或使用物品
 * - 技能图标来自 RMMV IconSet.png（32→26 缩放）
 * - 冷却时间覆盖层（蓝色半透明 + 倒计时秒数）
 * - MP 不足时灰显
 * - 支持从技能窗口/背包窗口拖放指定快捷栏
 * - 布局持久化到 localStorage（按角色 ID 隔离）
 * - 可拖拽移动位置
 *
 * 全局引用：
 * - window.Window_SkillBar — SkillBar 构造函数
 * - $MMO._skillBar — 当前 12 格绑定数据
 * - $MMO._knownSkills — 角色已学习的全部技能
 * - $MMO._skillCDs — 技能冷却状态（skill_id → readyAt）
 *
 * WebSocket 消息：
 * - map_init — 初始化技能列表与 MP
 * - player_sync — 实时 MP 更新
 * - skill_effect — 技能冷却同步
 * - player_skill（发送）— 请求释放技能
 * - use_item（发送）— 请求使用物品
 */

(function () {
    'use strict';

    // ═══════════════════════════════════════════════════════════
    //  常量配置
    // ═══════════════════════════════════════════════════════════
    /** @type {number} 快捷栏格子数（F1-F12）。 */
    var SLOT_COUNT = 12;
    /** @type {number} 每格像素尺寸。 */
    var SLOT_SIZE = 34;
    /** @type {number} 格间距（像素）。 */
    var SLOT_GAP = 2;
    /** @type {number} IconSet.png 每行图标列数（RMMV 标准 16 列）。 */
    var ICON_COLS = 16;
    /** @type {number} 面板内边距。 */
    var PAD = 4;

    // ═══════════════════════════════════════════════════════════
    //  全局状态
    // ═══════════════════════════════════════════════════════════
    /** @type {Array} 角色已学习的全部技能列表。 */
    $MMO._knownSkills = [];
    /** @type {Array} 12 格快捷栏绑定数据，每项为 {skill_id, icon_index, name, mp_cost, cd_ms} 或 null。 */
    $MMO._skillBar = Array(SLOT_COUNT).fill(null);
    /** @type {Object} 技能冷却状态，skill_id → readyAt（基于 performance.now()）。 */
    $MMO._skillCDs = {};
    /** @type {number} 当前玩家 MP。 */
    $MMO._playerMP = 0;
    /** @type {number} 当前玩家最大 MP。 */
    $MMO._playerMaxMP = 1;

    // ═══════════════════════════════════════════════════════════
    //  SkillBar — 继承 L2_Base 的技能快捷栏面板
    //  底部居中显示 12 个图标格，支持拖放与快捷键。
    // ═══════════════════════════════════════════════════════════

    /**
     * 技能快捷栏构造函数。
     * @constructor
     */
    function SkillBar() { this.initialize.apply(this, arguments); }
    SkillBar.prototype = Object.create(L2_Base.prototype);
    SkillBar.prototype.constructor = SkillBar;

    /**
     * 初始化快捷栏：计算尺寸、居中放置、加载图标集。
     */
    SkillBar.prototype.initialize = function () {
        var w = SLOT_COUNT * (SLOT_SIZE + SLOT_GAP) + PAD * 2 - SLOT_GAP;
        var h = SLOT_SIZE + PAD * 2;
        var x = Math.floor((Graphics.boxWidth - w) / 2);
        var y = Graphics.boxHeight - h - 4;
        L2_Base.prototype.initialize.call(this, x, y, w, h);
        /** @type {Bitmap|null} 缓存的 IconSet 位图。 */
        this._iconSet = null;
        ImageManager.loadSystem('IconSet').addLoadListener(function (bmp) {
            this._iconSet = bmp;
            this.refresh();
        }.bind(this));
        $MMO.makeDraggable(this, 'skillBar');
        this.refresh();
    };

    /**
     * 禁用标准内边距（L2_Base 默认为 0）。
     * @returns {number} 0
     */
    SkillBar.prototype.standardPadding = function () { return 0; };

    /**
     * 重绘整个快捷栏：背景 + 12 个格子。
     * 检测拖放悬停状态以高亮目标格。
     */
    SkillBar.prototype.refresh = function () {
        var c = this.bmp();
        c.clear();
        var w = this.cw(), h = this.ch();

        // 绘制半透明圆角背景。
        L2_Theme.fillRoundRect(c, 0, 0, w, h, L2_Theme.cornerRadius, 'rgba(13,13,26,0.70)');
        L2_Theme.strokeRoundRect(c, 0, 0, w, h, L2_Theme.cornerRadius, L2_Theme.borderDark);

        // 检测拖放悬停所在格子索引。
        var dragSlot = -1;
        if ($MMO._uiDrag) {
            dragSlot = $MMO._getBarSlotAt($MMO._uiDrag.x, $MMO._uiDrag.y);
        }

        for (var i = 0; i < SLOT_COUNT; i++) {
            this._drawSlot(c, i, i === dragSlot);
        }
    };

    /**
     * 绘制单个快捷栏格子。
     * 包含：格子背景/边框、技能图标、快捷键标签、MP 不足灰显、冷却覆盖层。
     * @param {Bitmap} c - 目标位图
     * @param {number} idx - 格子索引（0-11）
     * @param {boolean} dropHighlight - 是否为拖放悬停目标（金色高亮）
     */
    SkillBar.prototype._drawSlot = function (c, idx, dropHighlight) {
        var x = PAD + idx * (SLOT_SIZE + SLOT_GAP);
        var y = PAD;
        var skill = $MMO._skillBar[idx];
        var slotW = SLOT_SIZE, slotH = SLOT_SIZE;

        // 格子背景色：拖放悬停高亮 > 已绑定技能 > 空格。
        c.fillRect(x, y, slotW, slotH, dropHighlight ? '#2A2A4E' : (skill ? '#1A1A2E' : '#111118'));
        // 格子边框色。
        var borderCol = dropHighlight ? L2_Theme.textGold : (skill ? L2_Theme.borderDark : '#1a1a2a');
        L2_Theme.strokeRoundRect(c, x, y, slotW, slotH, 2, borderCol);

        // 绘制技能图标（从 IconSet 裁切 32×32，缩放到 26×26）。
        if (skill && this._iconSet) {
            var iconIdx = skill.icon_index || 0;
            var sx = (iconIdx % ICON_COLS) * 32;
            var sy = Math.floor(iconIdx / ICON_COLS) * 32;
            c.blt(this._iconSet, sx, sy, 32, 32, x + 4, y + 2, 26, 26);
        }

        // 右下角快捷键标签（F1-F12）。
        c.fontSize = 9;
        c.textColor = L2_Theme.textGray;
        c.drawText('F' + (idx + 1), x, y + slotH - 12, slotW - 2, 10, 'right');

        // MP 不足时整格灰显。
        if (skill && $MMO._playerMP < skill.mp_cost) {
            c.fillRect(x + 1, y + 1, slotW - 2, slotH - 2, 'rgba(0,0,0,0.55)');
        }

        // 冷却中覆盖层：从上到下蓝色进度条 + 剩余秒数。
        if (skill) {
            var cdRemain = this._getCDRemain(skill.skill_id);
            if (cdRemain > 0) {
                var total = skill.cd_ms || 1000;
                var ratio = cdRemain / total;
                var cdH = Math.round(slotH * ratio);
                c.fillRect(x + 1, y + 1, slotW - 2, cdH, 'rgba(0,0,80,0.60)');
                c.fontSize = 10;
                c.textColor = '#AADDFF';
                c.drawText(Math.ceil(cdRemain / 1000) + 's', x, y + 10, slotW, 14, 'center');
            }
        }
    };

    /**
     * 获取指定技能的剩余冷却时间（毫秒）。
     * 使用 performance.now() 单调时钟，不受系统时间调整影响。
     * @param {number} skillID - 技能 ID
     * @returns {number} 剩余冷却毫秒数，0 表示已就绪
     */
    SkillBar.prototype._getCDRemain = function (skillID) {
        var readyAt = $MMO._skillCDs[skillID];
        if (!readyAt) return 0;
        return Math.max(0, readyAt - performance.now());
    };

    /**
     * 每帧更新：处理拖放交互，每 6 帧刷新一次显示（冷却倒计时动画）。
     */
    SkillBar.prototype.update = function () {
        L2_Base.prototype.update.call(this);
        $MMO.updateDrag(this);
        if (Graphics.frameCount % 6 === 0) this.refresh();
    };

    // ═══════════════════════════════════════════════════════════
    //  F1-F12 快捷键监听
    //  按下 F 键释放对应格子的技能或使用物品。
    //  释放前检查 MP 和冷却状态。
    // ═══════════════════════════════════════════════════════════

    /**
     * 全局 keydown 监听器：拦截 F1-F12 并执行快捷栏操作。
     */
    window.addEventListener('keydown', function (e) {
        var fKey = e.keyCode - 111; // F1=112→1, F12=123→12
        if (fKey < 1 || fKey > 12) return;
        e.preventDefault();
        var idx = fKey - 1;
        var entry = $MMO._skillBar[idx];
        if (!entry) return;

        if (entry.skill_id) {
            // 技能：检查 MP 和冷却。
            if ($MMO._playerMP < entry.mp_cost) return;
            var readyAt = $MMO._skillCDs[entry.skill_id];
            if (readyAt && performance.now() < readyAt) return;
            $MMO.send('player_skill', { skill_id: entry.skill_id });
        } else if (entry.item_id) {
            // 消耗品：发送使用请求。
            $MMO.send('use_item', { inv_id: entry.inv_id });
        }
    });

    // ═══════════════════════════════════════════════════════════
    //  注入 Scene_Map — 创建快捷栏并添加到场景
    // ═══════════════════════════════════════════════════════════

    /** @type {Function} 原始 Scene_Map.createAllWindows 引用。 */
    var _Scene_Map_createAllWindows2 = Scene_Map.prototype.createAllWindows;

    /**
     * 覆写 Scene_Map.createAllWindows：追加创建技能快捷栏。
     */
    Scene_Map.prototype.createAllWindows = function () {
        _Scene_Map_createAllWindows2.call(this);
        this._mmoSkillBar = new SkillBar();
        this.addChild(this._mmoSkillBar);
        $MMO.registerBottomUI(this._mmoSkillBar);
    };

    // ═══════════════════════════════════════════════════════════
    //  WebSocket 消息处理器
    // ═══════════════════════════════════════════════════════════

    /**
     * map_init：初始化技能列表与 MP。
     * 从 localStorage 恢复快捷栏布局，首次进入则按顺序自动填充。
     */
    $MMO.on('map_init', function (data) {
        if (data.self) {
            $MMO._playerMP = data.self.mp || 0;
            $MMO._playerMaxMP = data.self.max_mp || 1;
        }
        if (data.skills) {
            $MMO._knownSkills = data.skills;
            // 尝试从 localStorage 恢复已保存的快捷栏布局。
            var saved = null;
            try { saved = JSON.parse(localStorage.getItem('mmo_skillbar_' + $MMO.charID)); } catch (e) {}
            if (saved && Array.isArray(saved)) {
                // 恢复已保存布局：按 skill_id 匹配当前技能数据。
                for (var i = 0; i < SLOT_COUNT; i++) {
                    if (saved[i]) {
                        var found = data.skills.find(function (s) { return s.skill_id === saved[i]; });
                        $MMO._skillBar[i] = found || null;
                    } else {
                        $MMO._skillBar[i] = null;
                    }
                }
            } else {
                // 首次加载：按顺序自动填充已学习的技能。
                $MMO._skillBar = Array(SLOT_COUNT).fill(null);
                data.skills.forEach(function (sk, i) {
                    if (i < SLOT_COUNT) $MMO._skillBar[i] = sk;
                });
            }
            $MMO._saveSkillBar();
        }
    });

    /**
     * 将当前快捷栏布局持久化到 localStorage。
     * 按角色 ID 隔离存储，仅保存 skill_id 数组。
     */
    $MMO._saveSkillBar = function () {
        try {
            var ids = $MMO._skillBar.map(function (s) { return s ? s.skill_id : null; });
            localStorage.setItem('mmo_skillbar_' + $MMO.charID, JSON.stringify(ids));
        } catch (e) {}
    };

    /**
     * 将技能或物品绑定到指定快捷栏格子。
     * @param {number} slotIdx - 目标格子索引（0-11）
     * @param {Object} data - 绑定数据（技能或物品对象）
     */
    $MMO.assignToBar = function (slotIdx, data) {
        if (slotIdx < 0 || slotIdx >= SLOT_COUNT) return;
        $MMO._skillBar[slotIdx] = data;
        $MMO._saveSkillBar();
    };

    /**
     * player_sync：实时更新玩家 MP 状态。
     */
    $MMO.on('player_sync', function (data) {
        if (data.char_id !== $MMO.charID) return;
        if (data.mp !== undefined) $MMO._playerMP = data.mp;
        if (data.max_mp !== undefined) $MMO._playerMaxMP = data.max_mp;
    });

    /**
     * skill_effect：服务器通知技能冷却开始。
     * 使用 performance.now() 单调时钟记录冷却结束时间。
     */
    $MMO.on('skill_effect', function (data) {
        if (data.char_id !== $MMO.charID) return;
        if (data.skill_id && data.cd_remaining_ms) {
            // 基于单调时钟 performance.now()，不受系统时间修改影响。
            $MMO._skillCDs[data.skill_id] = performance.now() + data.cd_remaining_ms;
        }
    });

    // ═══════════════════════════════════════════════════════════
    //  拖放系统 — 技能/物品拖放到快捷栏
    //  $MMO._uiDrag 由 SkillWindow 或 InventoryWindow 在拖拽时设置。
    // ═══════════════════════════════════════════════════════════

    /** @type {Object|null} 当前拖拽状态 { type: 'skill'|'item', data: {...}, x, y }。 */
    $MMO._uiDrag = null;

    /**
     * 根据屏幕坐标判断落在哪个快捷栏格子上。
     * @param {number} sx - 屏幕 X 坐标
     * @param {number} sy - 屏幕 Y 坐标
     * @returns {number} 格子索引（0-11），不在格子上返回 -1
     */
    $MMO._getBarSlotAt = function (sx, sy) {
        var scene = SceneManager._scene;
        if (!scene || !scene._mmoSkillBar) return -1;
        var bar = scene._mmoSkillBar;
        var lx = sx - bar.x, ly = sy - bar.y;
        if (ly < PAD || ly > PAD + SLOT_SIZE) return -1;
        if (lx < PAD) return -1;
        var idx = Math.floor((lx - PAD) / (SLOT_SIZE + SLOT_GAP));
        // 确认坐标在格子内而非间距中。
        var inSlotX = (lx - PAD) - idx * (SLOT_SIZE + SLOT_GAP);
        if (idx < 0 || idx >= SLOT_COUNT || inSlotX > SLOT_SIZE) return -1;
        return idx;
    };

    /**
     * 处理拖放释放：将拖拽的技能/物品绑定到目标格子。
     * @param {number} sx - 释放位置屏幕 X 坐标
     * @param {number} sy - 释放位置屏幕 Y 坐标
     */
    $MMO._handleDrop = function (sx, sy) {
        if (!$MMO._uiDrag) return;
        var slotIdx = $MMO._getBarSlotAt(sx, sy);
        if (slotIdx < 0) { $MMO._uiDrag = null; return; }
        $MMO.assignToBar(slotIdx, $MMO._uiDrag.data);
        $MMO._uiDrag = null;
    };

    /** @type {Function} 原始 Scene_Map.update 引用。 */
    var _Scene_Map_update_skillbar = Scene_Map.prototype.update;

    /**
     * 覆写 Scene_Map.update：拖拽过程中高亮目标格子。
     */
    Scene_Map.prototype.update = function () {
        _Scene_Map_update_skillbar.call(this);
        // 拖拽中悬停在快捷栏格子上时刷新高亮。
        if ($MMO._uiDrag && this._mmoSkillBar && this._mmoSkillBar._iconSet) {
            var drag = $MMO._uiDrag;
            var iconIdx = drag.data.icon_index || 0;
            if (iconIdx > 0) {
                var slotIdx = $MMO._getBarSlotAt(drag.x, drag.y);
                if (slotIdx >= 0) {
                    this._mmoSkillBar.refresh();
                }
            }
        }
    };

    // ═══════════════════════════════════════════════════════════
    //  全局窗口类导出
    // ═══════════════════════════════════════════════════════════
    window.Window_SkillBar = SkillBar;

})();
