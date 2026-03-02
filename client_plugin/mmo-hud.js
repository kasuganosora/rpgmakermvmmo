/*:
 * @plugindesc v3.0.0 MMO HUD - L2风格界面：左上角状态栏、右上角小地图、任务追踪器。
 * @author MMO Framework
 *
 * @help
 * ════════════════════════════════════════════════════════════════
 *  MMO HUD 插件 — 游戏内平视显示系统
 * ════════════════════════════════════════════════════════════════
 *
 *  本插件为MMO框架提供三个核心HUD组件：
 *
 *  1. 状态栏（StatusBar）— 左上角
 *     - 显示角色名称和等级
 *     - HP/MP/EXP 进度条（带数值标签）
 *     - 通过 player_sync 和 map_init 消息自动更新
 *
 *  2. 小地图（Minimap）— 右上角
 *     - 基于BFS的地形可达性渲染（仅绘制可通行区域）
 *     - 玩家位置（白色十字准星）
 *     - 其他玩家（蓝色点）、怪物（红色点）、NPC（黄色点）
 *     - 支持拖拽移动
 *     - 北方向指示器
 *
 *  3. 任务追踪器（QuestTracker）— 小地图下方
 *     - 最多显示3个活跃任务
 *     - 显示任务名称和目标进度
 *     - 已完成任务以绿色高亮显示
 *
 *  所有组件继承自 L2_Base，使用 L2_Theme 统一主题风格。
 *  通过 $MMO WebSocket 事件驱动数据更新。
 *
 *  全局窗口引用：
 *    Window_MMO_StatusBar  — 状态栏窗口类
 *    Window_Minimap        — 小地图窗口类
 *    Window_QuestTrack     — 任务追踪窗口类
 *
 * ════════════════════════════════════════════════════════════════
 */

(function () {
    'use strict';

    // ═══════════════════════════════════════════════════════════
    //  状态栏（StatusBar）— 左上角：名称/等级 + HP/MP/EXP 进度条
    // ═══════════════════════════════════════════════════════════

    /** @type {number} 状态栏宽度（像素） */
    var SB_W = 230;
    /** @type {number} 状态栏高度（像素） */
    var SB_H = 100;
    /** @type {number} 状态栏内边距（像素） */
    var SB_PAD = 8;

    /**
     * 状态栏窗口类 — 显示角色基本状态信息
     *
     * 继承自 L2_Base，在左上角渲染角色名称、等级以及
     * HP、MP、EXP三条进度条。数据通过服务器同步消息更新。
     *
     * @constructor
     */
    function StatusBar() { this.initialize.apply(this, arguments); }
    StatusBar.prototype = Object.create(L2_Base.prototype);
    StatusBar.prototype.constructor = StatusBar;

    /**
     * 初始化状态栏窗口
     *
     * 设置窗口位置为左上角 (4, 4)，初始化所有状态属性的默认值，
     * 并执行首次渲染。
     *
     * @returns {void}
     */
    StatusBar.prototype.initialize = function () {
        L2_Base.prototype.initialize.call(this, 4, 4, SB_W, SB_H);
        /** @type {string} 角色名称 */
        this._name = $MMO.charName || '';
        /** @type {number} 角色等级 */
        this._level = 1;
        /** @type {number} 当前生命值 */
        this._hp = 100;
        /** @type {number} 最大生命值 */
        this._maxHP = 100;
        /** @type {number} 当前魔法值 */
        this._mp = 50;
        /** @type {number} 最大魔法值 */
        this._maxMP = 50;
        /** @type {number} 当前经验值 */
        this._exp = 0;
        /** @type {number} 升级所需经验值 */
        this._maxExp = 100;
        this.refresh();
    };

    /**
     * 获取标准内边距
     *
     * 覆盖父类方法，返回0以取消默认内边距，
     * 使状态栏可以完全自定义布局。
     *
     * @returns {number} 始终返回0
     */
    StatusBar.prototype.standardPadding = function () { return 0; };

    /**
     * 设置状态栏数据并刷新显示
     *
     * 接收服务器同步的角色状态数据，仅更新提供的字段。
     * 支持部分更新 — 未提供的字段保持原值不变。
     *
     * @param {Object} data                - 角色状态数据对象
     * @param {string} [data.name]         - 角色名称
     * @param {number} [data.level]        - 角色等级
     * @param {number} [data.hp]           - 当前生命值
     * @param {number} [data.max_hp]       - 最大生命值
     * @param {number} [data.mp]           - 当前魔法值
     * @param {number} [data.max_mp]       - 最大魔法值
     * @param {number} [data.exp]          - 当前经验值
     * @param {number} [data.next_exp]     - 升级所需经验值
     * @returns {void}
     */
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

    /**
     * 刷新状态栏的绘制内容
     *
     * 清除画布后重新绘制所有元素：
     * 1. 半透明深色圆角背景
     * 2. 等级数字（金色）和角色名称（白色）
     * 3. HP 进度条（红色系，带数值文字）
     * 4. MP 进度条（蓝色系，带数值文字）
     * 5. EXP 进度条（黄色系，带百分比文字）
     *
     * @returns {void}
     */
    StatusBar.prototype.refresh = function () {
        var c = this.bmp();
        c.clear();
        var w = this.cw(), h = this.ch();
        var barW = w - SB_PAD * 2;

        // 绘制半透明圆角背景
        L2_Theme.fillRoundRect(c, 0, 0, w, h, L2_Theme.cornerRadius, 'rgba(13,13,26,0.65)');
        L2_Theme.strokeRoundRect(c, 0, 0, w, h, L2_Theme.cornerRadius, L2_Theme.borderDark);

        // 绘制等级（金色）和角色名称（白色）
        c.fontSize = L2_Theme.fontSmall;
        c.textColor = L2_Theme.textGold;
        c.drawText(this._level, SB_PAD, SB_PAD, 24, 16, 'left');
        c.textColor = L2_Theme.textWhite;
        c.drawText(this._name, SB_PAD + 28, SB_PAD, barW - 28, 16, 'left');

        // 绘制 HP 进度条
        var y = SB_PAD + 22;
        var hpRatio = Math.min(this._hp / Math.max(this._maxHP, 1), 1);
        L2_Theme.drawBar(c, SB_PAD, y, barW, 18, hpRatio, L2_Theme.hpBg, L2_Theme.hpFill);
        c.fontSize = 11;
        c.textColor = L2_Theme.textWhite;
        c.drawText('HP  ' + this._hp + ' / ' + this._maxHP, SB_PAD + 4, y, barW - 8, 18, 'left');

        // 绘制 MP 进度条
        y += 22;
        var mpRatio = Math.min(this._mp / Math.max(this._maxMP, 1), 1);
        L2_Theme.drawBar(c, SB_PAD, y, barW, 14, mpRatio, L2_Theme.mpBg, L2_Theme.mpFill);
        c.fontSize = 11;
        c.drawText('MP  ' + this._mp + ' / ' + this._maxMP, SB_PAD + 4, y, barW - 8, 14, 'left');

        // 绘制 EXP 进度条（黄色，右侧显示百分比）
        y += 18;
        var expRatio = this._maxExp > 0 ? Math.min(this._exp / this._maxExp, 1) : 0;
        L2_Theme.drawBar(c, SB_PAD, y, barW, 10, expRatio, '#1a1a00', '#CCCC00');
        c.fontSize = 10;
        c.textColor = L2_Theme.textGray;
        c.drawText(Math.floor(expRatio * 100) + '%', SB_PAD, y, barW - 4, 10, 'right');
    };

    // ═══════════════════════════════════════════════════════════
    //  小地图（Minimap）— 右上角：地形通行性 + 实体标记点
    // ═══════════════════════════════════════════════════════════

    /** @type {number} 小地图尺寸（正方形，像素） */
    var MM_SIZE = 120;

    /**
     * 小地图窗口类 — 显示地图概览和实体位置
     *
     * 继承自 L2_Base，在右上角渲染缩略地图。
     * 使用BFS算法从玩家位置探索可达区域，仅绘制可通行地形。
     * 标记玩家（白色十字）、其他玩家（蓝色）、怪物（红色）、NPC（黄色）。
     * 支持拖拽移动。
     *
     * @constructor
     */
    function Minimap() { this.initialize.apply(this, arguments); }
    Minimap.prototype = Object.create(L2_Base.prototype);
    Minimap.prototype.constructor = Minimap;

    /**
     * 初始化小地图窗口
     *
     * 设置窗口位置为右上角，初始化各类实体标记点数组，
     * 清空地形缓存，并启用拖拽功能。
     *
     * @returns {void}
     */
    Minimap.prototype.initialize = function () {
        L2_Base.prototype.initialize.call(this, Graphics.boxWidth - MM_SIZE - 4, 4, MM_SIZE, MM_SIZE);
        /** @type {Array<{x: number, y: number}>} 其他玩家位置列表 */
        this._playerDots = [];
        /** @type {Array<{x: number, y: number}>} 怪物位置列表 */
        this._monsterDots = [];
        /** @type {Array<{event_id: number, x: number, y: number}>} NPC位置列表 */
        this._npcDots = [];
        /** @type {Bitmap|null} 地形渲染缓存位图 */
        this._terrainCache = null;
        /** @type {number} 已缓存地形对应的地图ID（-1表示无缓存） */
        this._cachedMapId = -1;
        /** @type {number} 上一帧玩家X坐标（用于检测移动） */
        this._lastPx = -1;
        /** @type {number} 上一帧玩家Y坐标（用于检测移动） */
        this._lastPy = -1;
        // 启用拖拽功能，使用 'minimap' 作为位置持久化键名
        $MMO.makeDraggable(this, 'minimap');
    };

    /**
     * 更新指定NPC在小地图上的位置
     *
     * 根据事件ID查找已有的NPC记录，若存在则更新其坐标，
     * 若不存在则新增一条记录。由服务器 npc_sync 消息触发调用。
     *
     * @param {number} eventId - NPC对应的事件ID
     * @param {number} x       - NPC的地图X坐标（图块单位）
     * @param {number} y       - NPC的地图Y坐标（图块单位）
     * @returns {void}
     */
    Minimap.prototype.updateNPC = function (eventId, x, y) {
        // 在已有NPC列表中查找匹配的事件ID
        var found = false;
        for (var i = 0; i < this._npcDots.length; i++) {
            if (this._npcDots[i].event_id === eventId) {
                this._npcDots[i].x = x;
                this._npcDots[i].y = y;
                found = true;
                break;
            }
        }
        // 未找到则添加新的NPC条目
        if (!found) {
            this._npcDots.push({ event_id: eventId, x: x, y: y });
        }
    };

    /**
     * 每帧更新小地图状态
     *
     * 调用父类更新逻辑后，处理拖拽交互。
     *
     * @returns {void}
     */
    Minimap.prototype.update = function () {
        L2_Base.prototype.update.call(this);
        $MMO.updateDrag(this);
    };

    /**
     * 获取标准内边距
     *
     * 覆盖父类方法，返回0以取消默认内边距。
     *
     * @returns {number} 始终返回0
     */
    Minimap.prototype.standardPadding = function () { return 0; };

    /**
     * 设置地图通行性数据
     *
     * 当收到新的通行性数据时，将缓存地图ID重置为-1，
     * 强制下次 refresh 时重新构建地形缓存。
     *
     * @param {Object} data - 通行性数据（当前实现中未直接使用，
     *                        地形通过 $gameMap.checkPassage 实时获取）
     * @returns {void}
     */
    Minimap.prototype.setPassability = function (data) {
        // 强制下次刷新时重建地形
        this._cachedMapId = -1;
    };

    /**
     * 构建地形缓存位图（基于BFS可达性分析）
     *
     * 核心渲染逻辑：
     * 1. 计算地图到小地图的缩放比例和单元格大小
     * 2. 构建全地图通行性网格（基于 $gameMap.checkPassage）
     * 3. 从玩家当前位置执行BFS广度优先搜索，标记所有可达图块
     * 4. 仅绘制可达的图块为绿色，不可达区域保持透明
     *
     * 结果缓存在 this._terrainCache 中，避免每帧重复计算。
     * 仅在地图切换时重建（通过 _cachedMapId 判断）。
     *
     * @returns {void}
     */
    Minimap.prototype._buildTerrain = function () {
        if (!$gameMap || !$gameMap.width()) return;

        var mw = $gameMap.width();
        var mh = $gameMap.height();
        var cw = this.cw(), ch = this.ch();

        // 计算缩放比例，使整个地图适配小地图区域
        var scaleX = cw / mw;
        var scaleY = ch / mh;
        var scale = Math.min(scaleX, scaleY);

        // 计算单元格大小（每个图块在小地图上占几个像素，最小1像素）
        var cellSize = Math.max(1, Math.floor(scale));

        // 计算地图在小地图中的像素尺寸和居中偏移量
        var mapW = mw * cellSize;
        var mapH = mh * cellSize;
        var offsetX = Math.floor((cw - mapW) / 2);
        var offsetY = Math.floor((ch - mapH) / 2);

        // 创建新的地形缓存位图
        this._terrainCache = new Bitmap(cw, ch);
        var bmp = this._terrainCache;
        bmp.clear(); // 清除为透明背景

        // 保存定位参数供标记点绘制使用
        this._cellSize = cellSize;
        this._offsetX = offsetX;
        this._offsetY = offsetY;

        // 构建通行性网格（0x0f 检测四方向通行）
        var passable = new Array(mw * mh);
        for (var y = 0; y < mh; y++) {
            for (var x = 0; x < mw; x++) {
                passable[y * mw + x] = $gameMap.isValid(x, y) && $gameMap.checkPassage(x, y, 0x0f);
            }
        }

        // 从玩家位置执行BFS，找出所有可达图块
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

                // 四方向扩展：上、右、下、左
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

        // 仅绘制可达区域为绿色，不可达区域保持透明
        for (var y = 0; y < mh; y++) {
            for (var x = 0; x < mw; x++) {
                if (!reachable[y * mw + x]) continue;

                var px = offsetX + x * cellSize;
                var py = offsetY + y * cellSize;

                if (px < 0 || py < 0 || px >= cw || py >= ch) continue;

                // 所有可通行区域统一使用绿色
                bmp.fillRect(px, py, cellSize, cellSize, '#4a9a4a');
            }
        }

        this._cachedMapId = $gameMap.mapId();
    };

    /**
     * 设置其他玩家的位置数据
     *
     * @param {Array<{x: number, y: number}>} p - 其他玩家位置数组
     * @returns {void}
     */
    Minimap.prototype.setPlayers = function (p) { this._playerDots = p; };

    /**
     * 设置怪物的位置数据
     *
     * @param {Array<{x: number, y: number}>} m - 怪物位置数组
     * @returns {void}
     */
    Minimap.prototype.setMonsters = function (m) { this._monsterDots = m; };

    /**
     * 设置NPC的位置数据
     *
     * @param {Array<{event_id: number, x: number, y: number}>} n - NPC位置数组
     * @returns {void}
     */
    Minimap.prototype.setNPCs = function (n) { this._npcDots = n; };

    /**
     * 刷新小地图的绘制内容
     *
     * 执行以下绘制步骤：
     * 1. 检测地图是否切换，若切换则重建地形缓存
     * 2. 清除画布并拷贝地形缓存位图
     * 3. 绘制北方向指示器 "N"
     * 4. 绘制自身位置（亮绿色十字准星）
     * 5. 绘制其他玩家（蓝色方点，4x4像素）
     * 6. 绘制怪物（红色方点，3x3像素）
     * 7. 绘制NPC（黄色方点，3x3像素）
     *
     * 每帧调用，但地形缓存仅在地图切换时重建。
     *
     * @returns {void}
     */
    Minimap.prototype.refresh = function () {
        if (!$gameMap || !$gameMap.width()) return;

        // 地图切换时重建地形缓存
        if (this._cachedMapId !== $gameMap.mapId()) {
            this._buildTerrain();
        }

        var cw = this.cw(), ch = this.ch();
        var c = this.bmp();

        // 先清除为透明
        c.clear();

        // 拷贝地形缓存（限制在实际位图尺寸内）
        if (this._terrainCache) {
            var tw = Math.min(this._terrainCache.width, cw);
            var th = Math.min(this._terrainCache.height, ch);
            c.blt(this._terrainCache, 0, 0, tw, th, 0, 0);
        }

        // 绘制北方向指示器
        c.fontSize = 11;
        c.textColor = L2_Theme.textWhite;
        c.drawText('N', 0, 2, cw, 12, 'center');

        // 获取定位参数（优先使用缓存值，否则实时计算）
        var cellSize = this._cellSize || Math.max(1, Math.floor(Math.min(cw / $gameMap.width(), ch / $gameMap.height())));
        var offsetX = this._offsetX || Math.floor((cw - $gameMap.width() * cellSize) / 2);
        var offsetY = this._offsetY || Math.floor((ch - $gameMap.height() * cellSize) / 2);

        // 绘制自身位置（亮绿色十字准星）— 定位到单元格中心
        var px = offsetX + Math.round($gamePlayer.x * cellSize + cellSize / 2);
        var py = offsetY + Math.round($gamePlayer.y * cellSize + cellSize / 2);

        // 十字准星：外圈浅绿横竖线
        c.fillRect(px - 6, py - 1, 13, 3, '#88FF88');
        c.fillRect(px - 1, py - 6, 3, 13, '#88FF88');
        // 中心亮绿方块
        c.fillRect(px - 2, py - 2, 5, 5, '#CCFFCC');
        // 最中心白色核心
        c.fillRect(px - 1, py - 1, 3, 3, '#FFFFFF');

        // 绘制其他玩家（蓝色方点，4x4像素）— 定位到单元格中心
        for (var i = 0; i < this._playerDots.length; i++) {
            var p = this._playerDots[i];
            var px2 = offsetX + Math.round(p.x * cellSize + cellSize / 2);
            var py2 = offsetY + Math.round(p.y * cellSize + cellSize / 2);
            c.fillRect(px2 - 1, py2 - 1, 4, 4, '#4488FF');
        }

        // 绘制怪物（红色方点，3x3像素）— 定位到单元格中心
        for (var i = 0; i < this._monsterDots.length; i++) {
            var m = this._monsterDots[i];
            var mx = offsetX + Math.round(m.x * cellSize + cellSize / 2);
            var my = offsetY + Math.round(m.y * cellSize + cellSize / 2);
            c.fillRect(mx - 1, my - 1, 3, 3, '#FF4444');
        }

        // 绘制NPC（黄色方点，3x3像素）— 定位到单元格中心
        for (var i = 0; i < this._npcDots.length; i++) {
            var n = this._npcDots[i];
            var nx = offsetX + Math.round(n.x * cellSize + cellSize / 2);
            var ny = offsetY + Math.round(n.y * cellSize + cellSize / 2);
            c.fillRect(nx - 1, ny - 1, 3, 3, '#FFDD00');
        }
    };

    // ═══════════════════════════════════════════════════════════
    //  任务追踪器（QuestTracker）— 右侧，小地图下方
    // ═══════════════════════════════════════════════════════════

    /** @type {number} 任务追踪器宽度（像素） */
    var QT_W = 196;
    /** @type {number} 任务追踪器高度（像素） */
    var QT_H = 140;
    /** @type {number} 任务追踪器内边距（像素） */
    var QT_PAD = 6;

    /**
     * 任务追踪器窗口类 — 显示当前活跃任务及进度
     *
     * 继承自 L2_Base，在小地图下方显示最多3个活跃任务。
     * 每个任务条目包含任务名称和第一个目标的进度信息。
     * 已完成的任务以绿色高亮显示。
     *
     * @constructor
     */
    function QuestTracker() { this.initialize.apply(this, arguments); }
    QuestTracker.prototype = Object.create(L2_Base.prototype);
    QuestTracker.prototype.constructor = QuestTracker;

    /**
     * 初始化任务追踪器窗口
     *
     * 设置窗口位置在小地图正下方（右上角区域），
     * 初始化空的任务列表。
     *
     * @returns {void}
     */
    QuestTracker.prototype.initialize = function () {
        L2_Base.prototype.initialize.call(this, Graphics.boxWidth - QT_W - 4, MM_SIZE + 12, QT_W, QT_H);
        /** @type {Array<Object>} 当前追踪的任务列表（最多3个） */
        this._quests = [];
    };

    /**
     * 获取标准内边距
     *
     * 覆盖父类方法，返回0以取消默认内边距。
     *
     * @returns {number} 始终返回0
     */
    QuestTracker.prototype.standardPadding = function () { return 0; };

    /**
     * 设置追踪的任务列表并刷新显示
     *
     * 截取前3个任务进行显示，多余的任务将被忽略。
     *
     * @param {Array<Object>} quests            - 任务数据数组
     * @param {string}  quests[].name           - 任务名称
     * @param {boolean} quests[].completed      - 是否已完成
     * @param {Array}   [quests[].objectives]   - 任务目标列表
     * @param {string}  quests[].objectives[].label    - 目标描述文字
     * @param {number}  quests[].objectives[].current  - 当前完成数量
     * @param {number}  quests[].objectives[].required - 目标所需数量
     * @returns {void}
     */
    QuestTracker.prototype.setQuests = function (quests) {
        this._quests = quests.slice(0, 3);
        this.refresh();
    };

    /**
     * 刷新任务追踪器的绘制内容
     *
     * 清除画布后重新绘制：
     * 1. 半透明圆角背景
     * 2. 逐条渲染任务条目（每条占44像素高度）
     *    - 任务名称（已完成为绿色，否则为白色）
     *    - 第一个目标的进度文字（灰色，格式：标签: 当前/目标）
     *
     * @returns {void}
     */
    QuestTracker.prototype.refresh = function () {
        var c = this.bmp();
        c.clear();
        var w = this.cw(), h = this.ch();

        // 绘制半透明圆角背景
        L2_Theme.fillRoundRect(c, 0, 0, w, h, L2_Theme.cornerRadius, 'rgba(13,13,26,0.50)');
        L2_Theme.strokeRoundRect(c, 0, 0, w, h, L2_Theme.cornerRadius, L2_Theme.borderDark);

        // 逐条绘制任务条目
        this._quests.forEach(function (q, i) {
            var y = QT_PAD + i * 44;
            // 任务名称（已完成为绿色，未完成为白色）
            c.fontSize = L2_Theme.fontSmall;
            c.textColor = q.completed ? '#44FF88' : L2_Theme.textWhite;
            c.drawText(q.name, QT_PAD, y, w - QT_PAD * 2, 18, 'left');
            // 第一个目标的进度信息
            if (q.objectives && q.objectives.length > 0) {
                c.fontSize = 11;
                c.textColor = L2_Theme.textGray;
                var obj = q.objectives[0];
                c.drawText(obj.label + ': ' + obj.current + '/' + obj.required,
                    QT_PAD, y + 18, w - QT_PAD * 2, 14, 'left');
            }
        });
    };

    // ═══════════════════════════════════════════════════════════
    //  注入 Scene_Map — 创建HUD窗口并绑定更新逻辑
    // ═══════════════════════════════════════════════════════════

    /**
     * 保存 Scene_Map.prototype.createAllWindows 的原始引用，
     * 用于在覆写中调用原始逻辑（别名模式）。
     * @type {Function}
     */
    var _Scene_Map_createAllWindows = Scene_Map.prototype.createAllWindows;

    /**
     * 覆写 Scene_Map.createAllWindows — 注入MMO HUD窗口
     *
     * 在原始窗口创建完成后，额外创建并添加三个HUD组件：
     * - 状态栏（左上角）
     * - 小地图（右上角）
     * - 任务追踪器（小地图下方）
     *
     * 若存在上次同步的角色数据（$MMO._lastSelf），立即应用到状态栏。
     *
     * @returns {void}
     */
    Scene_Map.prototype.createAllWindows = function () {
        _Scene_Map_createAllWindows.call(this);
        this._mmoStatusBar = new StatusBar();
        this._mmoMinimap = new Minimap();
        this._mmoQuestTrack = new QuestTracker();
        this.addChild(this._mmoStatusBar);
        this.addChild(this._mmoMinimap);
        this.addChild(this._mmoQuestTrack);
        // 若有缓存的角色数据，立即应用
        if ($MMO._lastSelf) this._mmoStatusBar.setData($MMO._lastSelf);
    };

    /**
     * 保存 Scene_Map.prototype.update 的原始引用，
     * 用于在覆写中调用原始逻辑（别名模式）。
     * @type {Function}
     */
    var _Scene_Map_update2 = Scene_Map.prototype.update;

    /**
     * 覆写 Scene_Map.update — 注入小地图数据更新和刷新逻辑
     *
     * 在原始更新逻辑之后：
     * 1. 每15帧从 OtherPlayerManager 和 MonsterManager 收集实体位置数据
     *    - 其他玩家位置从精灵的 _character 对象获取
     *    - 怪物位置从精灵的 _tileX/_tileY 获取
     *    - NPC位置通过 npc_sync 事件单独更新，不在此处收集
     * 2. 每帧调用小地图的 refresh 方法（内部地形缓存避免重复计算）
     *
     * @returns {void}
     */
    Scene_Map.prototype.update = function () {
        _Scene_Map_update2.call(this);

        if (this._mmoMinimap && $gameMap) {
            // 每15帧更新一次玩家/怪物/NPC的位置数据
            if (Graphics.frameCount % 15 === 0) {
                var players = [];
                var monsters = [];
                var npcs = [];

                // 从 OtherPlayerManager 收集其他玩家位置
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

                // 从 MonsterManager 收集怪物位置
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
                // NPC位置通过 npc_sync 事件更新，不在此处设置
            }

            // 每帧刷新小地图绘制（地形缓存机制避免重复计算）
            this._mmoMinimap.refresh();
        }
    };

    // ═══════════════════════════════════════════════════════════
    //  WebSocket 消息处理器 — 服务器数据同步
    // ═══════════════════════════════════════════════════════════

    /**
     * 处理 player_sync 消息 — 角色状态同步
     *
     * 当服务器推送角色状态变化时（HP/MP/等级等），
     * 检查是否是当前角色的数据，若是则更新状态栏。
     *
     * @param {Object} data          - 同步数据
     * @param {number} data.char_id  - 角色ID
     */
    $MMO.on('player_sync', function (data) {
        if (data.char_id !== $MMO.charID) return;
        if (SceneManager._scene && SceneManager._scene._mmoStatusBar) {
            SceneManager._scene._mmoStatusBar.setData(data);
        }
    });

    /**
     * 处理 map_init 消息 — 进入地图初始化
     *
     * 玩家进入新地图时服务器发送的初始化数据。
     * 包含角色自身状态（self）和地图通行性数据（passability）。
     * 同时清空NPC列表，等待新地图的NPC同步。
     *
     * @param {Object} data               - 地图初始化数据
     * @param {Object} [data.self]         - 角色自身状态数据
     * @param {Object} [data.passability]  - 地图通行性数据
     */
    $MMO.on('map_init', function (data) {
        if (data.self && SceneManager._scene && SceneManager._scene._mmoStatusBar) {
            SceneManager._scene._mmoStatusBar.setData(data.self);
        }
        if (data.passability && SceneManager._scene && SceneManager._scene._mmoMinimap) {
            SceneManager._scene._mmoMinimap.setPassability(data.passability);
        }
        // 切换地图时清空NPC列表
        if (SceneManager._scene && SceneManager._scene._mmoMinimap) {
            SceneManager._scene._mmoMinimap._npcDots = [];
        }
    });

    /**
     * 处理 npc_sync 消息 — NPC位置同步
     *
     * 服务器推送单个NPC的位置更新，更新小地图上对应的NPC标记点。
     *
     * @param {Object} data            - NPC同步数据
     * @param {number} data.event_id   - NPC事件ID
     * @param {number} data.x          - NPC的地图X坐标
     * @param {number} data.y          - NPC的地图Y坐标
     */
    $MMO.on('npc_sync', function (data) {
        if (data && SceneManager._scene && SceneManager._scene._mmoMinimap) {
            SceneManager._scene._mmoMinimap.updateNPC(data.event_id, data.x, data.y);
        }
    });

    /**
     * 处理 exp_gain 消息 — 经验值获取通知
     *
     * 当角色获得经验值时，更新状态栏的经验值和等级显示。
     * 仅更新提供的字段（total_exp 和/或 level）。
     *
     * @param {Object} data              - 经验值数据
     * @param {number} [data.total_exp]  - 当前总经验值
     * @param {number} [data.level]      - 当前等级（升级时提供）
     */
    $MMO.on('exp_gain', function (data) {
        if (!data) return;
        if (SceneManager._scene && SceneManager._scene._mmoStatusBar) {
            var update = {};
            if (data.total_exp !== undefined) update.exp = data.total_exp;
            if (data.level !== undefined)     update.level = data.level;
            SceneManager._scene._mmoStatusBar.setData(update);
        }
    });

    /**
     * 处理 quest_update 消息 — 任务状态更新
     *
     * 当任务状态发生变化时（新任务、进度更新、完成等），
     * 更新本地任务追踪列表并刷新追踪器显示。
     * 若任务已存在则更新，否则添加为新任务。
     *
     * @param {Object} data             - 任务更新数据
     * @param {number} data.quest_id    - 任务唯一ID
     * @param {string} data.name        - 任务名称
     * @param {boolean} data.completed  - 是否已完成
     * @param {Array}  data.objectives  - 任务目标列表
     */
    $MMO.on('quest_update', function (data) {
        if (!data) return;
        // 初始化追踪任务列表（若不存在）
        $MMO._trackedQuests = $MMO._trackedQuests || [];
        // 查找并更新已有任务，或添加新任务
        var found = false;
        $MMO._trackedQuests.forEach(function (q, i) {
            if (q.quest_id === data.quest_id) {
                $MMO._trackedQuests[i] = data;
                found = true;
            }
        });
        if (!found) $MMO._trackedQuests.push(data);
        // 刷新任务追踪器显示
        if (SceneManager._scene && SceneManager._scene._mmoQuestTrack) {
            SceneManager._scene._mmoQuestTrack.setQuests($MMO._trackedQuests);
        }
    });

    // ═══════════════════════════════════════════════════════════
    //  全局窗口类导出 — 供外部模块引用
    // ═══════════════════════════════════════════════════════════

    /** @global Window_MMO_StatusBar — 状态栏窗口类的全局引用 */
    window.Window_MMO_StatusBar = StatusBar;
    /** @global Window_Minimap — 小地图窗口类的全局引用 */
    window.Window_Minimap = Minimap;
    /** @global Window_QuestTrack — 任务追踪器窗口类的全局引用 */
    window.Window_QuestTrack = QuestTracker;

})();
