/*:
 * @plugindesc v2.0.0 MMO Battle - 实时战斗系统 (L2 UI 风格)。
 * @author MMO Framework
 *
 * @help
 * 本插件实现了 MMO 实时战斗系统的客户端部分，包括：
 * - 怪物精灵渲染与平滑移动插值
 * - 伤害数字弹出显示（支持暴击/治疗）
 * - 地面掉落物闪烁显示与拾取
 * - 鼠标点击攻击怪物/拾取物品
 * - 玩家死亡遮罩界面
 * - WebSocket 消息处理（怪物生成/同步/死亡、掉落物、战斗结果等）
 *
 * 所有战斗逻辑由服务器权威计算，客户端仅负责渲染和用户输入。
 */

(function () {
    'use strict';

    // ═══════════════════════════════════════════════════════════════
    //  禁用本地随机遇敌
    //  MMO 模式下所有战斗由服务器控制，客户端不应触发随机遇敌
    // ═══════════════════════════════════════════════════════════════

    /**
     * 覆盖遇敌计数更新，使其不执行任何操作。
     * 在 MMO 模式下，遇敌完全由服务器控制。
     */
    Game_Player.prototype.updateEncounterCount = function () {};

    /**
     * 覆盖遇敌执行，始终返回 false 以阻止本地遇敌触发。
     * @returns {boolean} 始终返回 false
     */
    Game_Player.prototype.executeEncounter = function () { return false; };

    /** 怪物精灵每帧移动的像素速度（用于平滑插值） */
    var MOVE_SPEED = 0.0625;

    /** 移动队列最大长度，超出时直接瞬移到目标位置 */
    var QUEUE_MAX = 10;

    // ═══════════════════════════════════════════════════════════════
    //  Sprite_Monster — L2 主题风格的怪物精灵（含 HP 条和名称标签）
    // ═══════════════════════════════════════════════════════════════

    /**
     * 怪物精灵类，继承自 RMMV 的 Sprite。
     * 在地图上渲染一个怪物，包含：
     * - 棕色方块作为怪物本体（中央显示 "M"）
     * - 底部 HP 条（根据血量比例变色：绿→橙→红）
     * - 顶部名称标签（金色文字）
     * - 平滑移动插值系统（通过移动队列实现）
     *
     * @constructor
     * @param {Object} data - 服务器发来的怪物数据对象
     * @param {number} data.x - 怪物所在的地图格子 X 坐标
     * @param {number} data.y - 怪物所在的地图格子 Y 坐标
     * @param {number} data.hp - 怪物当前生命值
     * @param {number} data.max_hp - 怪物最大生命值
     * @param {string} data.name - 怪物名称
     * @param {number} data.inst_id - 怪物实例唯一标识符
     */
    function Sprite_Monster(data) { this.initialize(data); }
    Sprite_Monster.prototype = Object.create(Sprite.prototype);
    Sprite_Monster.prototype.constructor = Sprite_Monster;

    /**
     * 初始化怪物精灵。
     * 创建位图、HP 条子精灵、名称标签子精灵，并设置初始位置。
     *
     * @param {Object} data - 服务器发来的怪物数据对象
     */
    Sprite_Monster.prototype.initialize = function (data) {
        Sprite.prototype.initialize.call(this);
        this._data = data;           // 怪物原始数据
        this._tileX = data.x;        // 目标格子坐标 X（移动目的地）
        this._tileY = data.y;        // 目标格子坐标 Y（移动目的地）
        this._realX = data.x;        // 当前实际渲染坐标 X（用于平滑插值）
        this._realY = data.y;        // 当前实际渲染坐标 Y（用于平滑插值）
        this._moveQueue = [];        // 移动目标队列，用于缓冲服务器发来的连续移动
        this.bitmap = new Bitmap(48, 48);  // 怪物本体位图（48x48 像素）

        // 创建 HP 条子精灵（48x8 像素，位于怪物底部）
        this._hpBar = new Sprite(new Bitmap(48, 8));
        this._hpBar.y = 42;
        this.addChild(this._hpBar);

        // 创建名称标签子精灵（100x18 像素，位于怪物顶部偏左）
        this._nameLabel = new Sprite(new Bitmap(100, 18));
        this._nameLabel.x = -26;
        this._nameLabel.y = -22;
        this.addChild(this._nameLabel);

        // 绘制怪物外观
        this._drawMonster();
    };

    /**
     * 绘制怪物外观。
     * 包括：棕色方块本体、中央 "M" 字母、HP 条、名称标签。
     * 当怪物数据更新时可重新调用此方法刷新显示。
     */
    Sprite_Monster.prototype._drawMonster = function () {
        var bmp = this.bitmap;
        bmp.clear();
        bmp.fillRect(4, 4, 40, 40, '#884400');  // 棕色方块作为怪物本体
        bmp.fontSize = 18;
        bmp.textColor = L2_Theme.textWhite;
        bmp.drawText('M', 0, 8, 48, 24, 'center');  // 中央显示 "M" 标识
        this._updateHPBar();

        // 绘制名称标签（金色小字体）
        var nb = this._nameLabel.bitmap;
        nb.clear();
        nb.fontSize = L2_Theme.fontSmall;
        nb.textColor = L2_Theme.textGold;
        nb.drawText(this._data.name || '', 0, 0, 100, 18, 'center');
    };

    /**
     * 更新 HP 条显示。
     * 根据当前 HP 与最大 HP 的比例绘制血条，颜色规则：
     * - HP > 50%: 绿色 (#44FF44)
     * - HP > 25%: 橙色 (#FFAA00)
     * - HP <= 25%: 红色 (L2_Theme.hpFill)
     */
    Sprite_Monster.prototype._updateHPBar = function () {
        var ratio = this._data.max_hp > 0 ? this._data.hp / this._data.max_hp : 0;
        var bmp = this._hpBar.bitmap;
        bmp.clear();
        L2_Theme.drawBar(bmp, 0, 0, 48, 8, ratio, L2_Theme.hpBg,
            ratio > 0.5 ? '#44FF44' : ratio > 0.25 ? '#FFAA00' : L2_Theme.hpFill);
    };

    /**
     * 同步服务器发来的怪物数据。
     * 将新位置加入移动队列以实现平滑移动，同时更新 HP 条。
     *
     * 位移判断逻辑：
     * - 如果新位置与当前（或队列末尾）位置相差超过 1 格，或队列已满 → 直接瞬移
     * - 否则将新位置压入移动队列，等待 update() 逐帧消费
     *
     * @param {Object} data - 服务器发来的最新怪物数据
     */
    Sprite_Monster.prototype.syncData = function (data) {
        this._data = data;

        // 取当前参考位置：优先使用移动队列末尾，否则使用当前目标格子
        var refX = this._tileX, refY = this._tileY;
        if (this._moveQueue.length > 0) {
            var last = this._moveQueue[this._moveQueue.length - 1];
            refX = last.x; refY = last.y;
        }

        // 计算与参考位置的距离差
        var dx = Math.abs(data.x - refX);
        var dy = Math.abs(data.y - refY);

        if (dx > 1 || dy > 1 || this._moveQueue.length >= QUEUE_MAX) {
            // 距离过远或队列溢出 → 清空队列，直接瞬移到新位置
            this._moveQueue = [];
            this._tileX = data.x; this._tileY = data.y;
            this._realX = data.x; this._realY = data.y;
        } else {
            // 正常移动 → 将新位置加入队列
            this._moveQueue.push({ x: data.x, y: data.y });
        }
        this._updateHPBar();
    };

    /**
     * 每帧更新怪物精灵。
     * 处理平滑移动插值：
     * 1. 如果当前没有在移动且队列中有待处理目标 → 取出下一个目标
     * 2. 按 MOVE_SPEED 速度向目标位置逐帧靠近
     */
    Sprite_Monster.prototype.update = function () {
        // 检查是否正在移动中（实际坐标是否已到达目标格子坐标）
        var moving = (this._realX !== this._tileX || this._realY !== this._tileY);
        if (!moving && this._moveQueue.length > 0) {
            // 当前移动完成，从队列中取出下一个目标
            var next = this._moveQueue.shift();
            this._tileX = next.x; this._tileY = next.y;
        }

        // 按 MOVE_SPEED 速度向目标格子坐标平滑移动
        if (this._tileX < this._realX) this._realX = Math.max(this._realX - MOVE_SPEED, this._tileX);
        if (this._tileX > this._realX) this._realX = Math.min(this._realX + MOVE_SPEED, this._tileX);
        if (this._tileY < this._realY) this._realY = Math.max(this._realY - MOVE_SPEED, this._tileY);
        if (this._tileY > this._realY) this._realY = Math.min(this._realY + MOVE_SPEED, this._tileY);

        Sprite.prototype.update.call(this);
    };

    // ═══════════════════════════════════════════════════════════════
    //  Sprite_DamagePopup — 浮动伤害数字弹出（基于 Sprite 实现）
    //  支持普通伤害、暴击伤害、治疗数值三种样式
    // ═══════════════════════════════════════════════════════════════

    /**
     * 伤害数字弹出精灵类，继承自 RMMV 的 Sprite。
     * 在指定位置显示一个向上飘动并逐渐消失的伤害数字。
     *
     * 样式规则：
     * - 普通伤害：白色，20 号字体，显示纯数字
     * - 暴击伤害：金色，28 号字体，数字后加 "!"
     * - 治疗数值：绿色，20 号字体，数字前加 "+"
     *
     * @constructor
     * @param {number} value - 伤害/治疗数值
     * @param {boolean} isCrit - 是否为暴击
     * @param {boolean} isHeal - 是否为治疗
     */
    function Sprite_DamagePopup(value, isCrit, isHeal) { this.initialize(value, isCrit, isHeal); }
    Sprite_DamagePopup.prototype = Object.create(Sprite.prototype);
    Sprite_DamagePopup.prototype.constructor = Sprite_DamagePopup;

    /**
     * 初始化伤害弹出精灵。
     * 创建位图并绘制伤害文字，设置初始上抛速度和生命周期。
     *
     * @param {number} value - 伤害/治疗数值
     * @param {boolean} isCrit - 是否为暴击（影响字体大小和颜色）
     * @param {boolean} isHeal - 是否为治疗（影响颜色和前缀符号）
     */
    Sprite_DamagePopup.prototype.initialize = function (value, isCrit, isHeal) {
        Sprite.prototype.initialize.call(this);
        var fontSize = isCrit ? 28 : 20;  // 暴击使用更大字体
        var w = 120, h = 40;
        this.bitmap = new Bitmap(w, h);
        this.bitmap.fontSize = fontSize;

        // 根据伤害类型选择颜色：治疗=绿色，暴击=金色，普通=白色
        var color = isHeal ? '#44FF88' : isCrit ? L2_Theme.textGold : L2_Theme.textWhite;
        this.bitmap.textColor = color;

        // 根据伤害类型格式化文本：治疗="+数值"，暴击="数值!"，普通="数值"
        var text = isHeal ? '+' + value : isCrit ? value + '!' : String(value);
        this.bitmap.drawText(text, 0, 0, w, h, 'center');

        this.anchor.x = 0.5;   // 锚点水平居中
        this.anchor.y = 1.0;   // 锚点垂直底部
        this._vy = -2.5;       // 初始上抛速度（每帧向上移动的像素数）
        this._life = 60;       // 生命周期（帧数，60 帧 ≈ 1 秒）
    };

    /**
     * 每帧更新伤害弹出精灵。
     * 处理向上飘动动画：
     * - 每帧按 _vy 速度移动 Y 坐标
     * - 速度逐帧衰减（乘以 0.92），模拟减速效果
     * - 透明度随生命周期线性递减
     * - 生命周期结束后自动从父容器中移除
     */
    Sprite_DamagePopup.prototype.update = function () {
        Sprite.prototype.update.call(this);
        this.y += this._vy;          // 应用垂直速度
        this._vy *= 0.92;            // 速度衰减，产生减速飘动效果
        this._life--;                // 生命周期递减
        this.opacity = Math.round((this._life / 60) * 255);  // 透明度随剩余生命线性递减
        if (this._life <= 0 && this.parent) this.parent.removeChild(this);  // 生命结束后自动销毁
    };

    // ═══════════════════════════════════════════════════════════════
    //  Sprite_MapDrop — 地面掉落物精灵（闪烁的金色星星）
    //  基于 Sprite 实现，带呼吸闪烁动画
    // ═══════════════════════════════════════════════════════════════

    /**
     * 地面掉落物精灵类，继承自 RMMV 的 Sprite。
     * 在地图上渲染一个金色星星图标，带有正弦波透明度闪烁动画。
     *
     * @constructor
     * @param {Object} data - 掉落物数据对象
     * @param {number} data.x - 掉落物所在地图格子 X 坐标
     * @param {number} data.y - 掉落物所在地图格子 Y 坐标
     * @param {number} data.drop_id - 掉落物唯一标识符
     */
    function Sprite_MapDrop(data) { this.initialize(data); }
    Sprite_MapDrop.prototype = Object.create(Sprite.prototype);
    Sprite_MapDrop.prototype.constructor = Sprite_MapDrop;

    /**
     * 初始化掉落物精灵。
     * 创建 32x32 位图，绘制金色方块背景和黑色星星符号。
     *
     * @param {Object} data - 掉落物数据对象
     */
    Sprite_MapDrop.prototype.initialize = function (data) {
        Sprite.prototype.initialize.call(this);
        this._data = data;     // 保存掉落物数据引用
        this._blink = 0;       // 闪烁动画的相位角度（0-359 度循环）
        this.bitmap = new Bitmap(32, 32);
        this.bitmap.fillRect(4, 4, 24, 24, '#FFD700');  // 金色方块背景
        this.bitmap.fontSize = 18;
        this.bitmap.textColor = '#000';
        this.bitmap.drawText('★', 0, 2, 32, 28, 'center');  // 黑色星星符号
        this.anchor.x = 0.5;   // 锚点水平居中
        this.anchor.y = 1.0;   // 锚点垂直底部
    };

    /**
     * 每帧更新掉落物精灵。
     * 通过正弦波函数产生透明度闪烁效果：
     * - 相位角每帧递增 3 度，在 0-359 度之间循环
     * - 透明度在 105 (180-75) 到 255 (180+75) 之间波动
     */
    Sprite_MapDrop.prototype.update = function () {
        Sprite.prototype.update.call(this);
        this._blink = (this._blink + 3) % 360;
        this.opacity = 180 + Math.round(Math.sin(this._blink * Math.PI / 180) * 75);
    };

    // ═══════════════════════════════════════════════════════════════
    //  MonsterManager — 怪物与掉落物管理器（单例）
    //  统一管理地图上所有怪物精灵、掉落物精灵、伤害弹出的生命周期
    // ═══════════════════════════════════════════════════════════════

    /**
     * 怪物管理器单例对象。
     * 职责：
     * - 管理所有怪物精灵的创建、更新、销毁
     * - 管理所有掉落物精灵的创建、销毁
     * - 显示伤害数字弹出
     * - 每帧更新所有精灵的屏幕坐标（基于地图滚动偏移）
     */
    var MonsterManager = {
        _sprites: {},          // 怪物精灵字典 {inst_id: Sprite_Monster}
        _drops: {},            // 掉落物精灵字典 {drop_id: Sprite_MapDrop}
        _container: null,      // 怪物和掉落物精灵的父容器（通常是 tilemap）
        _popupContainer: null, // 伤害弹出精灵的父容器

        /**
         * 初始化管理器，绑定精灵容器并清空所有缓存。
         * 在 Spriteset_Map.createCharacters 中调用。
         *
         * @param {Sprite} container - 怪物/掉落物精灵的父容器
         * @param {Sprite} popupContainer - 伤害弹出精灵的父容器（可选，默认同 container）
         */
        init: function (container, popupContainer) {
            this._container = container;
            this._popupContainer = popupContainer || container;
            this._sprites = {};
            this._drops = {};
        },

        /**
         * 在地图上生成一个怪物精灵。
         * 如果该 inst_id 的怪物已存在则跳过（防止重复生成）。
         *
         * @param {Object} data - 怪物数据对象（需包含 inst_id, x, y, hp, max_hp, name）
         */
        spawnMonster: function (data) {
            if (this._sprites[data.inst_id]) return;  // 已存在则跳过
            var sp = new Sprite_Monster(data);
            this._sprites[data.inst_id] = sp;
            if (this._container) this._container.addChild(sp);
        },

        /**
         * 同步更新指定怪物的数据（位置、HP 等）。
         * 由服务器 monster_sync 消息触发。
         *
         * @param {Object} data - 怪物最新数据对象（需包含 inst_id）
         */
        updateMonster: function (data) {
            var sp = this._sprites[data.inst_id];
            if (sp) sp.syncData(data);
        },

        /**
         * 从地图上移除指定怪物精灵。
         * 从父容器中移除并从精灵字典中删除。
         *
         * @param {number} instID - 怪物实例唯一标识符
         */
        removeMonster: function (instID) {
            var sp = this._sprites[instID];
            if (!sp) return;
            if (sp.parent) sp.parent.removeChild(sp);
            delete this._sprites[instID];
        },

        /**
         * 在地图上生成一个掉落物精灵。
         * 如果该 drop_id 的掉落物已存在则跳过。
         *
         * @param {Object} data - 掉落物数据对象（需包含 drop_id, x, y）
         */
        spawnDrop: function (data) {
            if (this._drops[data.drop_id]) return;  // 已存在则跳过
            var sp = new Sprite_MapDrop(data);
            this._drops[data.drop_id] = sp;
            if (this._container) this._container.addChild(sp);
        },

        /**
         * 从地图上移除指定掉落物精灵。
         *
         * @param {number} dropID - 掉落物唯一标识符
         */
        removeDrop: function (dropID) {
            var sp = this._drops[dropID];
            if (!sp) return;
            if (sp.parent) sp.parent.removeChild(sp);
            delete this._drops[dropID];
        },

        /**
         * 在指定屏幕坐标显示伤害数字弹出。
         *
         * @param {number} x - 屏幕 X 坐标
         * @param {number} y - 屏幕 Y 坐标
         * @param {number} value - 伤害/治疗数值
         * @param {boolean} isCrit - 是否为暴击
         * @param {boolean} isHeal - 是否为治疗
         */
        showDamage: function (x, y, value, isCrit, isHeal) {
            var sp = new Sprite_DamagePopup(value, isCrit, isHeal);
            sp.x = x; sp.y = y;
            if (this._popupContainer) this._popupContainer.addChild(sp);
        },

        /**
         * 每帧更新所有怪物和掉落物精灵的屏幕坐标。
         * 根据地图的滚动偏移量 (displayX/displayY) 将格子坐标转换为屏幕像素坐标。
         * 在 Spriteset_Map.update 中每帧调用。
         *
         * 坐标转换公式：
         * - screenX = (格子X - 地图水平偏移 + 0.5) * 格子宽度
         * - screenY = (格子Y - 地图垂直偏移 + 1.0) * 格子高度
         * 其中 +0.5 使精灵水平居中于格子，+1.0 使精灵底部对齐格子底边
         */
        updatePositions: function () {
            if (!$gameMap) return;
            var self = this;
            var tileW = $gameMap.tileWidth();
            var tileH = $gameMap.tileHeight();

            // 更新所有怪物精灵的屏幕坐标（使用 _realX/_realY 实现平滑移动）
            Object.keys(this._sprites).forEach(function (id) {
                var sp = self._sprites[id];
                sp.x = (sp._realX - $gameMap.displayX() + 0.5) * tileW;
                sp.y = (sp._realY - $gameMap.displayY() + 1.0) * tileH;
            });

            // 更新所有掉落物精灵的屏幕坐标（使用固定的 _data.x/_data.y）
            Object.keys(this._drops).forEach(function (id) {
                var sp = self._drops[id];
                sp.x = (sp._data.x - $gameMap.displayX() + 0.5) * tileW;
                sp.y = (sp._data.y - $gameMap.displayY() + 1.0) * tileH;
            });
        },

        /**
         * 清空所有怪物和掉落物精灵。
         * 从父容器中移除并清空精灵字典。
         * 在切换地图或场景结束时调用。
         */
        clear: function () {
            var self = this;
            Object.keys(this._sprites).forEach(function (id) { self.removeMonster(parseInt(id)); });
            Object.keys(this._drops).forEach(function (id) { self.removeDrop(parseInt(id)); });
        }
    };

    // ═══════════════════════════════════════════════════════════════
    //  挂接 Spriteset_Map — 在地图精灵集中初始化怪物管理器
    // ═══════════════════════════════════════════════════════════════

    /** 保存原始的 createCharacters 方法引用 */
    var _Spriteset_Map_createCharacters2 = Spriteset_Map.prototype.createCharacters;

    /**
     * 扩展 Spriteset_Map.createCharacters。
     * 在原始角色精灵创建完成后，初始化 MonsterManager，
     * 将 tilemap 作为怪物精灵和伤害弹出的父容器。
     */
    Spriteset_Map.prototype.createCharacters = function () {
        _Spriteset_Map_createCharacters2.call(this);
        MonsterManager.init(this._tilemap, this._tilemap);
    };

    /** 保存原始的 update 方法引用 */
    var _Spriteset_Map_update2 = Spriteset_Map.prototype.update;

    /**
     * 扩展 Spriteset_Map.update。
     * 在原始更新逻辑后，每帧调用 MonsterManager.updatePositions()
     * 以同步所有怪物/掉落物精灵的屏幕坐标。
     */
    Spriteset_Map.prototype.update = function () {
        _Spriteset_Map_update2.call(this);
        MonsterManager.updatePositions();
    };

    // ═══════════════════════════════════════════════════════════════
    //  鼠标点击攻击与拾取 — 挂接 Scene_Map.processMapTouch
    // ═══════════════════════════════════════════════════════════════

    /** 保存原始的 processMapTouch 方法引用 */
    var _Scene_Map_processMapTouch = Scene_Map.prototype.processMapTouch;

    /**
     * 扩展 Scene_Map.processMapTouch。
     * 在鼠标/触摸点击时，优先检测是否点击了怪物或掉落物：
     * 1. 将屏幕点击坐标转换为地图格子坐标
     * 2. 遍历所有怪物精灵，检查是否有怪物在该格子 → 发送 attack 消息
     * 3. 遍历所有掉落物精灵，检查是否有掉落物在该格子 → 发送 pickup_item 消息
     * 4. 如果都没命中，则执行原始的地图点击处理（角色移动等）
     */
    Scene_Map.prototype.processMapTouch = function () {
        if (TouchInput.isTriggered()) {
            // 将屏幕像素坐标转换为地图格子坐标
            var tileX = Math.floor((TouchInput.x + $gameMap.displayX() * $gameMap.tileWidth()) / $gameMap.tileWidth());
            var tileY = Math.floor((TouchInput.y + $gameMap.displayY() * $gameMap.tileHeight()) / $gameMap.tileHeight());

            // 检测是否点击了怪物
            var hit = null;
            Object.keys(MonsterManager._sprites).forEach(function (id) {
                var sp = MonsterManager._sprites[id];
                if (sp._tileX === tileX && sp._tileY === tileY) hit = parseInt(id);
            });
            if (hit !== null) {
                // 命中怪物 → 发送攻击请求到服务器
                $MMO.send('attack', { target_id: hit, target_type: 'monster' });
                return;
            }

            // 检测是否点击了掉落物
            var dropHit = null;
            Object.keys(MonsterManager._drops).forEach(function (id) {
                var sp = MonsterManager._drops[id];
                if (sp._data.x === tileX && sp._data.y === tileY) dropHit = parseInt(id);
            });
            if (dropHit !== null) {
                // 命中掉落物 → 发送拾取请求到服务器
                $MMO.send('pickup_item', { drop_id: dropHit });
                return;
            }
        }
        // 未命中任何战斗目标，执行原始的地图点击处理
        _Scene_Map_processMapTouch.call(this);
    };

    // ═══════════════════════════════════════════════════════════════
    //  死亡遮罩界面 — L2_Dialog 风格的全屏覆盖
    //  玩家死亡时显示 "YOU DIED" 和 "等待复活..." 提示
    // ═══════════════════════════════════════════════════════════════

    /** 死亡遮罩窗口引用，null 表示当前未显示 */
    var _deathOverlay = null;

    /**
     * 显示死亡遮罩界面。
     * 在屏幕中央创建一个 300x120 的半透明暗红色窗口，
     * 显示 "YOU DIED" 大字和 "Awaiting revival..." 小字提示。
     * 如果遮罩已经在显示中则不重复创建。
     */
    function showDeathOverlay() {
        if (_deathOverlay) return;  // 防止重复显示
        var w = 300, h = 120;

        // 创建 L2_Base 窗口，居中放置
        _deathOverlay = new L2_Base((Graphics.boxWidth - w) / 2, (Graphics.boxHeight - h) / 2, w, h);
        _deathOverlay.standardPadding = function () { return 0; };

        var c = _deathOverlay.bmp();
        // 绘制半透明暗红色圆角背景
        L2_Theme.fillRoundRect(c, 0, 0, w, h, L2_Theme.cornerRadius, 'rgba(26,0,0,0.80)');
        // 绘制红色圆角边框
        L2_Theme.strokeRoundRect(c, 0, 0, w, h, L2_Theme.cornerRadius, '#FF2222');

        // 绘制 "YOU DIED" 大字（红色，36 号字体）
        c.fontSize = 36;
        c.textColor = '#FF2222';
        c.drawText('YOU DIED', 0, 20, w, 50, 'center');

        // 绘制 "Awaiting revival..." 提示（灰色，小字体）
        c.fontSize = L2_Theme.fontSmall;
        c.textColor = L2_Theme.textGray;
        c.drawText('Awaiting revival...', 0, 72, w, 20, 'center');

        // 将遮罩添加到当前场景
        if (SceneManager._scene) SceneManager._scene.addChild(_deathOverlay);
    }

    /**
     * 隐藏死亡遮罩界面。
     * 从父容器中移除并清空引用。
     * 在玩家复活或场景切换时调用。
     */
    function hideDeathOverlay() {
        if (_deathOverlay && _deathOverlay.parent) {
            _deathOverlay.parent.removeChild(_deathOverlay);
            _deathOverlay = null;
        }
    }

    // ═══════════════════════════════════════════════════════════════
    //  WebSocket 消息处理器 — 处理服务器发来的战斗相关消息
    // ═══════════════════════════════════════════════════════════════

    /**
     * 处理 map_init 消息：进入新地图时的初始化。
     * 清空所有现有怪物和掉落物，然后根据服务器数据重新生成。
     *
     * @param {Object} data - 地图初始化数据
     * @param {Array} [data.monsters] - 当前地图上的怪物列表
     * @param {Array} [data.drops] - 当前地图上的掉落物列表
     */
    $MMO.on('map_init', function (data) {
        MonsterManager.clear();
        (data.monsters || []).forEach(function (m) { MonsterManager.spawnMonster(m); });
        (data.drops || []).forEach(function (d) { MonsterManager.spawnDrop(d); });
    });

    /**
     * 处理 monster_spawn 消息：服务器通知有新怪物生成。
     * @param {Object} data - 怪物数据
     */
    $MMO.on('monster_spawn', function (data) { MonsterManager.spawnMonster(data); });

    /**
     * 处理 monster_sync 消息：服务器同步怪物最新状态（位置、HP 等）。
     * @param {Object} data - 怪物最新数据
     */
    $MMO.on('monster_sync', function (data) { MonsterManager.updateMonster(data); });

    /**
     * 处理 monster_death 消息：服务器通知怪物死亡。
     * 从地图上移除对应怪物精灵。
     *
     * @param {Object} data - 死亡怪物数据
     * @param {number} data.inst_id - 死亡怪物的实例 ID
     */
    $MMO.on('monster_death', function (data) {
        MonsterManager.removeMonster(data.inst_id);
    });

    /**
     * 处理 drop_spawn 消息：服务器通知有新掉落物生成。
     * @param {Object} data - 掉落物数据
     */
    $MMO.on('drop_spawn', function (data) { MonsterManager.spawnDrop(data); });

    /**
     * 处理 drop_remove 消息：服务器通知掉落物被拾取或消失。
     * @param {Object} data - 掉落物数据
     * @param {number} data.drop_id - 被移除的掉落物 ID
     */
    $MMO.on('drop_remove', function (data) { MonsterManager.removeDrop(data.drop_id); });

    /**
     * 处理 battle_result 消息：服务器返回的攻击结果。
     * 在怪物位置显示伤害数字弹出。
     *
     * @param {Object} data - 战斗结果数据
     * @param {number} data.x - 目标格子 X 坐标
     * @param {number} data.y - 目标格子 Y 坐标
     * @param {number} data.damage - 造成的伤害值
     * @param {boolean} data.is_crit - 是否暴击
     */
    $MMO.on('battle_result', function (data) {
        if ($gameMap && SceneManager._scene && SceneManager._scene._spriteset) {
            var tileW = $gameMap.tileWidth(), tileH = $gameMap.tileHeight();
            // 将格子坐标转换为屏幕坐标
            var screenX = (data.x - $gameMap.displayX() + 0.5) * tileW;
            var screenY = (data.y - $gameMap.displayY()) * tileH;
            MonsterManager.showDamage(screenX, screenY, data.damage, data.is_crit, false);
        }
    });

    /**
     * 处理 skill_effect 消息：技能效果结果。
     * 在目标位置显示伤害/治疗数字弹出。
     * 如果 damage 为负值则视为治疗效果。
     *
     * @param {Object} data - 技能效果数据
     * @param {number} data.target_x - 目标格子 X 坐标
     * @param {number} data.target_y - 目标格子 Y 坐标
     * @param {number} data.damage - 伤害值（负值表示治疗）
     * @param {boolean} data.is_crit - 是否暴击
     */
    $MMO.on('skill_effect', function (data) {
        if (data.damage !== undefined && $gameMap && SceneManager._scene) {
            var tileW = $gameMap.tileWidth(), tileH = $gameMap.tileHeight();
            // 将格子坐标转换为屏幕坐标
            var screenX = (data.target_x - $gameMap.displayX() + 0.5) * tileW;
            var screenY = (data.target_y - $gameMap.displayY()) * tileH;
            // Math.abs 取绝对值显示，负值 damage 标记为治疗
            MonsterManager.showDamage(screenX, screenY, Math.abs(data.damage), data.is_crit, data.damage < 0);
        }
    });

    /**
     * 处理 player_death 消息：玩家死亡，显示死亡遮罩。
     */
    $MMO.on('player_death', function () { showDeathOverlay(); });

    /**
     * 处理 player_revive 消息：玩家复活，隐藏死亡遮罩。
     */
    $MMO.on('player_revive', function () { hideDeathOverlay(); });

    // ═══════════════════════════════════════════════════════════════
    //  场景切换清理 — 挂接 Scene_Map.terminate
    // ═══════════════════════════════════════════════════════════════

    /** 保存原始的 terminate 方法引用 */
    var _Scene_Map_terminate2 = Scene_Map.prototype.terminate;

    /**
     * 扩展 Scene_Map.terminate。
     * 在地图场景结束时清理所有战斗相关资源：
     * - 清空所有怪物和掉落物精灵
     * - 隐藏死亡遮罩界面
     */
    Scene_Map.prototype.terminate = function () {
        _Scene_Map_terminate2.call(this);
        MonsterManager.clear();
        hideDeathOverlay();
    };

    // ═══════════════════════════════════════════════════════════════
    //  导出到全局命名空间
    // ═══════════════════════════════════════════════════════════════

    /** 将 MonsterManager 导出到 window 全局对象，供其他插件或调试使用 */
    window.MonsterManager = MonsterManager;

})();
