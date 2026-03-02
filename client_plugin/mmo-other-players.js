/*:
 * @plugindesc v1.0.0 MMO Other Players - 在地图上渲染其他玩家角色的MMO插件。
 * @author MMO Framework
 *
 * @help
 * ═══════════════════════════════════════════════════════════════
 *  MMO 其他玩家渲染插件
 * ═══════════════════════════════════════════════════════════════
 *
 *  本插件负责：
 *  1. 在地图上显示同地图的其他在线玩家精灵
 *  2. 平滑插值移动（排队式移动系统）
 *  3. 玩家名称标签（含公会名、组队/PK状态颜色）
 *  4. 右键上下文菜单（组队/交易）
 *  5. 场景切换时保留玩家数据（菜单、战斗等）
 *  6. 标签页切回时瞬移到最新位置
 *
 *  依赖：mmo-core.js ($MMO), L2_Base, L2_Theme
 */

(function () {
    'use strict';

    /** 移动队列最大长度，超过此值时直接瞬移到最新位置 */
    var QUEUE_MAX = 10;

    // ═══════════════════════════════════════════════════════════════
    //  Sprite_OtherPlayer — 其他玩家角色精灵
    // ═══════════════════════════════════════════════════════════════

    /**
     * 其他玩家角色精灵类
     * 继承自 Sprite_Character，用于在地图上渲染其他在线玩家。
     * 内部维护一个移动队列，实现平滑的逐格移动效果。
     *
     * @constructor
     * @param {Object} data - 玩家数据对象
     * @param {number} data.char_id - 角色ID
     * @param {string} data.walk_name - 行走图文件名
     * @param {number} data.walk_index - 行走图索引
     * @param {number} data.x - 地图X坐标
     * @param {number} data.y - 地图Y坐标
     * @param {number} data.dir - 朝向（2=下, 4=左, 6=右, 8=上）
     * @param {string} data.name - 玩家名称
     * @param {string} [data.guild_name] - 公会名称
     * @param {boolean} [data.in_party] - 是否在同一队伍中
     * @param {boolean} [data.pk_mode] - 是否处于PK模式
     */
    function Sprite_OtherPlayer(data) {
        this.initialize(data);
    }
    Sprite_OtherPlayer.prototype = Object.create(Sprite_Character.prototype);
    Sprite_OtherPlayer.prototype.constructor = Sprite_OtherPlayer;

    /**
     * 初始化其他玩家精灵
     * 创建一个与 Sprite_Character 兼容的虚拟角色对象，
     * 设置行走图、位置、朝向，并创建名称标签子精灵。
     *
     * @param {Object} data - 玩家数据对象（字段说明见构造函数）
     */
    Sprite_OtherPlayer.prototype.initialize = function (data) {
        this._charData = data;
        // 创建一个与 Sprite_Character 兼容的虚拟 Game_Character 对象
        var ch = new Game_Character();
        ch.setImage(data.walk_name || 'Actor1', data.walk_index || 0);
        ch.setPosition(data.x || 0, data.y || 0);
        ch.setDirection(data.dir || 2);
        ch._moveSpeed = 4; // RMMV标准玩家移动速度
        Sprite_Character.prototype.initialize.call(this, ch);
        this._moveQueue = []; // 来自 player_sync 的移动指令队列 {x, y, dir}
        this._label = new Sprite_PlayerLabel(data);
        this.addChild(this._label);
    };

    /**
     * 同步服务器下发的玩家位置数据
     * 根据新位置与当前/队列末尾位置的距离判断：
     * - 距离>1格 或 队列溢出：直接瞬移（传送/重置位置的情况）
     * - 距离<=1格：加入移动队列，等待逐格平滑移动
     *
     * @param {Object} data - 玩家同步数据
     * @param {number} data.x - 目标X坐标
     * @param {number} data.y - 目标Y坐标
     * @param {number} data.dir - 朝向
     */
    Sprite_OtherPlayer.prototype.syncData = function (data) {
        this._charData = data;
        var c = this._character;

        // 以队列末尾位置（或当前逻辑位置）作为参考点进行距离比较
        var refX = c._x, refY = c._y;
        if (this._moveQueue.length > 0) {
            var last = this._moveQueue[this._moveQueue.length - 1];
            refX = last.x;
            refY = last.y;
        }
        var dx = Math.abs(data.x - refX);
        var dy = Math.abs(data.y - refY);

        // 距离超过1格（传送/reset_pos）或队列溢出时，直接瞬移
        if (dx > 1 || dy > 1 || this._moveQueue.length >= QUEUE_MAX) {
            this._moveQueue = [];
            c._x = data.x;
            c._y = data.y;
            c._realX = data.x;
            c._realY = data.y;
            c.setDirection(data.dir || 2);
            return;
        }

        // 正常的1格移动，加入队列等待逐帧消费
        this._moveQueue.push({ x: data.x, y: data.y, dir: data.dir || 2 });
    };

    /**
     * 每帧更新其他玩家精灵
     * 处理流程：
     * 1. 当角色完成当前移动后，从队列中取出下一个移动指令
     * 2. 设置逻辑坐标（_realX/_realY 会通过 updateMove 平滑追赶）
     * 3. 驱动角色的平滑插值和行走动画
     * 4. 调用父类更新精灵渲染
     * 5. 场景淡入期间隐藏精灵（防止过渡动画完成前闪现）
     * 6. 更新名称标签
     */
    Sprite_OtherPlayer.prototype.update = function () {
        var c = this._character;

        // 角色完成当前格移动后，从队列中取出下一个移动目标
        if (!c.isMoving() && this._moveQueue.length > 0) {
            var next = this._moveQueue.shift();
            if (next.x !== c._x || next.y !== c._y) {
                // 设置逻辑坐标；_realX/_realY 会通过 updateMove 逐帧平滑追赶
                c._x = next.x;
                c._y = next.y;
            }
            c.setDirection(next.dir);
        }

        // 驱动角色的平滑位置插值和行走动画
        c.update();

        Sprite_Character.prototype.update.call(this);
        // 场景淡入期间隐藏精灵，防止过渡动画完成前出现闪烁
        if (OtherPlayerManager._fadeHide) this.opacity = 0;
        this._label.update();
    };

    // ═══════════════════════════════════════════════════════════════
    //  Sprite_PlayerLabel — 玩家名称标签精灵
    // ═══════════════════════════════════════════════════════════════

    /**
     * 玩家名称标签精灵类
     * 继承自 Sprite，显示在玩家精灵上方。
     * 包含玩家名称和公会名称，根据状态变化颜色：
     * - 白色：普通玩家
     * - 绿色(#44FF88)：同队伍玩家
     * - 红色(#FF4444)：PK模式玩家
     *
     * @constructor
     * @param {Object} data - 玩家数据对象
     * @param {string} data.name - 玩家名称
     * @param {string} [data.guild_name] - 公会名称
     * @param {boolean} [data.in_party] - 是否同队
     * @param {boolean} [data.pk_mode] - 是否PK模式
     */
    function Sprite_PlayerLabel(data) {
        this.initialize(data);
    }
    Sprite_PlayerLabel.prototype = Object.create(Sprite.prototype);
    Sprite_PlayerLabel.prototype.constructor = Sprite_PlayerLabel;

    /**
     * 初始化名称标签精灵
     * 创建160x40的位图画布，设置锚点为底部居中，
     * 定位在角色精灵上方（y=-36），然后绘制标签内容。
     *
     * @param {Object} data - 玩家数据对象
     */
    Sprite_PlayerLabel.prototype.initialize = function (data) {
        Sprite.prototype.initialize.call(this);
        this._data = data;
        this.bitmap = new Bitmap(160, 40);
        this.anchor.x = 0.5; // 水平居中
        this.anchor.y = 1.0; // 垂直锚点在底部
        this.y = -36;        // 显示在角色精灵上方
        this._draw();
    };

    /**
     * 绘制名称标签内容
     * 在位图上绘制玩家名称（第一行）和公会名称（第二行，如有）。
     * 名称颜色根据玩家状态变化：
     * - 同队玩家：绿色 #44FF88
     * - PK模式玩家：红色 #FF4444
     * - 普通玩家：白色 #FFFFFF
     * 公会名称固定灰色 #CCCCCC，格式为 [公会名]
     */
    Sprite_PlayerLabel.prototype._draw = function () {
        var bmp = this.bitmap;
        bmp.clear();
        bmp.fontSize = 14;
        // 根据玩家状态选择名称颜色
        var color = '#FFFFFF';
        if (this._data.in_party) color = '#44FF88';       // 同队伍：绿色
        else if (this._data.pk_mode) color = '#FF4444';    // PK模式：红色
        bmp.textColor = color;
        var name = this._data.name || '';
        bmp.drawText(name, 0, 0, 160, 20, 'center');
        // 如有公会名称，在名称下方绘制灰色公会标签
        if (this._data.guild_name) {
            bmp.fontSize = 12;
            bmp.textColor = '#CCCCCC';
            bmp.drawText('[' + this._data.guild_name + ']', 0, 20, 160, 16, 'center');
        }
    };

    /**
     * 每帧更新名称标签
     * 调用父类 Sprite.prototype.update 处理基础更新逻辑。
     */
    Sprite_PlayerLabel.prototype.update = function () {
        Sprite.prototype.update.call(this);
    };

    // ═══════════════════════════════════════════════════════════════
    //  OtherPlayerManager — 其他玩家管理器（单例）
    // ═══════════════════════════════════════════════════════════════

    /**
     * 其他玩家管理器对象
     * 负责管理所有其他在线玩家精灵的生命周期，包括：
     * - 精灵的创建、更新、移除
     * - 场景切换时的数据保留与恢复
     * - Spriteset_Map 就绪前的待处理队列
     * - 场景淡入期间的隐藏控制
     *
     * @namespace OtherPlayerManager
     */
    var OtherPlayerManager = {
        _sprites: {},        // 当前活跃的精灵字典 {char_id: Sprite_OtherPlayer}
        _container: null,    // 精灵容器（Tilemap 引用）
        _pending: [],        // Spriteset_Map 就绪前收到的待添加玩家列表
        _initPlayers: null,  // map_init 下发的玩家列表——跨场景切换保留
        _fadeHide: false,    // 场景淡入期间为 true——隐藏所有精灵

        /**
         * 初始化管理器，绑定精灵容器
         * 将保存的玩家数据（_initPlayers）和待处理队列（_pending）
         * 合并后逐个创建精灵。在 Spriteset_Map.createCharacters 中调用。
         *
         * @param {PIXI.Container} container - 精灵容器（通常是 this._tilemap）
         */
        init: function (container) {
            this._container = container;
            this._sprites = {};
            // 合并已保存的玩家列表和场景切换期间新加入的玩家
            var toAdd = (this._initPlayers || []).concat(this._pending);
            this._initPlayers = null;
            this._pending = [];
            for (var i = 0; i < toAdd.length; i++) {
                this.add(toAdd[i]);
            }
        },

        /**
         * 添加一个其他玩家精灵
         * - 若容器尚未就绪（Tilemap 未创建），将数据暂存到待处理队列
         * - 若该角色ID的精灵已存在，则更新其数据（syncData）
         * - 否则创建新的 Sprite_OtherPlayer 并添加到容器中
         *
         * @param {Object} data - 玩家数据对象
         * @param {number} data.char_id - 角色ID（必须）
         */
        add: function (data) {
            if (!data || !data.char_id) return;
            // 如果 Tilemap 容器尚未就绪，暂存到待处理队列
            if (!this._container) {
                this._pending.push(data);
                return;
            }
            // 如果该角色的精灵已存在，更新数据即可
            if (this._sprites[data.char_id]) {
                this._sprites[data.char_id].syncData(data);
                return;
            }
            var sp = new Sprite_OtherPlayer(data);
            this._sprites[data.char_id] = sp;
            this._container.addChild(sp);
        },

        /**
         * 移除指定角色ID的玩家精灵
         * 从容器和精灵字典中移除，同时清理保存列表和待处理队列中的对应数据
         * （处理场景切换期间玩家离开的情况）
         *
         * @param {number} charID - 要移除的角色ID
         */
        remove: function (charID) {
            var sp = this._sprites[charID];
            if (sp) {
                if (sp.parent) sp.parent.removeChild(sp);
                delete this._sprites[charID];
            }
            // 同时从保存列表中清除（处理场景切换期间的移除）
            if (this._initPlayers) {
                this._initPlayers = this._initPlayers.filter(function (p) { return p.char_id !== charID; });
            }
            this._pending = this._pending.filter(function (p) { return p.char_id !== charID; });
        },

        /**
         * 更新指定角色的位置/朝向数据
         * 将数据传递给对应精灵的 syncData 方法处理
         *
         * @param {Object} data - 玩家同步数据
         * @param {number} data.char_id - 角色ID
         * @param {number} data.x - X坐标
         * @param {number} data.y - Y坐标
         * @param {number} data.dir - 朝向
         */
        update: function (data) {
            var sp = this._sprites[data.char_id];
            if (sp) sp.syncData(data);
        },

        /**
         * 获取指定角色ID的精灵对象
         *
         * @param {number} charID - 角色ID
         * @returns {Sprite_OtherPlayer|undefined} 对应的精灵对象，不存在则返回 undefined
         */
        get: function (charID) { return this._sprites[charID]; },

        /**
         * 清除所有其他玩家精灵
         * 逐个调用 remove 移除精灵并清空待处理队列
         */
        clear: function () {
            var self = this;
            Object.keys(this._sprites).forEach(function (id) { self.remove(parseInt(id)); });
            this._pending = [];
        }
    };

    // ═══════════════════════════════════════════════════════════════
    //  Spriteset_Map 钩子 — 注入 OtherPlayerManager
    // ═══════════════════════════════════════════════════════════════

    /**
     * 钩入 Spriteset_Map.prototype.createCharacters
     * 在地图精灵集创建角色精灵后，初始化 OtherPlayerManager，
     * 将 _tilemap 作为容器传入，并启用淡入隐藏标志。
     */
    var _Spriteset_Map_createCharacters = Spriteset_Map.prototype.createCharacters;
    Spriteset_Map.prototype.createCharacters = function () {
        _Spriteset_Map_createCharacters.call(this);
        OtherPlayerManager._fadeHide = true; // 淡入完成前隐藏其他玩家精灵
        OtherPlayerManager.init(this._tilemap);
    };

    /**
     * 钩入 Scene_Map.prototype.start
     * 在场景淡入动画完成后，关闭淡入隐藏标志，让其他玩家精灵正常显示。
     * fadeSpeed() 返回24帧（约400ms），额外等待4帧作为安全余量。
     */
    var _Scene_Map_start_opm = Scene_Map.prototype.start;
    Scene_Map.prototype.start = function () {
        _Scene_Map_start_opm.call(this);
        // fadeSpeed() 返回24帧=400ms；额外多等几帧确保安全
        var fadeMs = Math.round((this.fadeSpeed() + 4) * 1000 / 60);
        setTimeout(function () { OtherPlayerManager._fadeHide = false; }, fadeMs);
    };

    // Sprite_Character.updatePosition() 通过 screenX()/screenY() 处理定位，
    // 已正确考虑了 Tilemap 滚动偏移，无需手动覆盖。

    // ═══════════════════════════════════════════════════════════════
    //  WebSocket 消息处理器
    // ═══════════════════════════════════════════════════════════════

    /**
     * 处理 player_join 消息 — 其他玩家加入当前地图
     * 跳过自身角色，将新玩家添加到管理器中
     */
    $MMO.on('player_join', function (data) {
        if (data.char_id === $MMO.charID) return; // 跳过自身
        OtherPlayerManager.add(data);
    });

    /**
     * 处理 player_leave 消息 — 其他玩家离开当前地图
     * 从管理器中移除对应角色的精灵
     */
    $MMO.on('player_leave', function (data) {
        OtherPlayerManager.remove(data.char_id);
    });

    /**
     * 处理 player_sync 消息 — 其他玩家位置/朝向同步
     * - 若精灵已存在：直接更新位置数据
     * - 若精灵不存在但有保存数据（场景切换中）：更新保存列表中的位置，
     *   确保场景恢复后使用最新位置
     */
    $MMO.on('player_sync', function (data) {
        if (data.char_id === $MMO.charID) return;
        if (OtherPlayerManager._sprites[data.char_id]) {
            OtherPlayerManager.update(data);
        } else if (OtherPlayerManager._initPlayers) {
            // 场景切换期间（菜单、战斗），更新保存列表中的位置
            for (var i = 0; i < OtherPlayerManager._initPlayers.length; i++) {
                if (OtherPlayerManager._initPlayers[i].char_id === data.char_id) {
                    OtherPlayerManager._initPlayers[i].x = data.x;
                    OtherPlayerManager._initPlayers[i].y = data.y;
                    OtherPlayerManager._initPlayers[i].dir = data.dir;
                    break;
                }
            }
        }
    });

    /**
     * 处理 map_init 消息 — 进入地图时的初始化数据
     * 过滤掉自身角色，保存玩家列表到 _initPlayers（跨 Scene_Map.terminate → clear 保留），
     * 清除旧精灵，并尝试立即添加新玩家（同地图重入时有效）
     */
    $MMO.on('map_init', function (data) {
        var players = (data.players || []).filter(function (p) {
            return p.char_id !== $MMO.charID;
        });
        // 保存到 _initPlayers — 跨 Scene_Map.terminate() → clear() 保留
        OtherPlayerManager._initPlayers = players;
        OtherPlayerManager.clear();
        // 同时尝试立即添加（同地图重入时容器可能已就绪）
        players.forEach(function (p) {
            OtherPlayerManager.add(p);
        });
    });

    /**
     * 处理 _disconnected 消息 — WebSocket 断开连接
     * 清除所有其他玩家精灵
     */
    $MMO.on('_disconnected', function () {
        OtherPlayerManager.clear();
    });

    // ═══════════════════════════════════════════════════════════════
    //  场景切换时保留/恢复玩家数据
    // ═══════════════════════════════════════════════════════════════

    /**
     * 钩入 Scene_Map.prototype.terminate
     * 在场景销毁前保存当前所有其他玩家的状态数据，
     * 以便从菜单/战斗返回时恢复精灵。
     * 注意：切换地图时 map_init 处理器会覆盖 _initPlayers。
     */
    var _Scene_Map_terminate = Scene_Map.prototype.terminate;
    Scene_Map.prototype.terminate = function () {
        // 保存当前玩家状态，用于从菜单/战斗返回时恢复
        // 切换地图时 map_init 处理器会覆盖 _initPlayers
        var savedPlayers = OtherPlayerManager._initPlayers;
        if (!savedPlayers) {
            var saved = [];
            var sprites = OtherPlayerManager._sprites;
            Object.keys(sprites).forEach(function (id) {
                var sp = sprites[id];
                var c = sp._character;
                // 深拷贝角色数据并更新为当前实际坐标和朝向
                var d = {};
                for (var k in sp._charData) d[k] = sp._charData[k];
                d.x = c._x;
                d.y = c._y;
                d.dir = c.direction();
                saved.push(d);
            });
            if (saved.length > 0) savedPlayers = saved;
        }
        OtherPlayerManager.clear();
        OtherPlayerManager._initPlayers = savedPlayers;
        _Scene_Map_terminate.call(this);
    };

    // ═══════════════════════════════════════════════════════════════
    //  标签页可见性变化处理 — 从后台切回时瞬移
    // ═══════════════════════════════════════════════════════════════

    /**
     * 监听 document.visibilitychange 事件
     * 当用户从其他标签页切回游戏时，将所有其他玩家精灵
     * 瞬移到移动队列中的最新位置，避免长时间后台积累的
     * 大量移动指令导致角色缓慢追赶。
     */
    document.addEventListener('visibilitychange', function () {
        if (document.hidden) return;
        Object.keys(OtherPlayerManager._sprites).forEach(function (id) {
            var sp = OtherPlayerManager._sprites[id];
            if (!sp._moveQueue || sp._moveQueue.length === 0) return;
            // 取队列中最后一个位置作为瞬移目标
            var last = sp._moveQueue[sp._moveQueue.length - 1];
            sp._moveQueue = [];
            var c = sp._character;
            c._x = last.x;
            c._y = last.y;
            c._realX = last.x;
            c._realY = last.y;
            c.setDirection(last.dir);
        });
    });

    // ═══════════════════════════════════════════════════════════════
    //  右键上下文菜单 — 对其他玩家进行组队/交易操作（基于 L2_Base）
    // ═══════════════════════════════════════════════════════════════

    /** 上下文菜单宽度（像素） */
    var CTX_W = 120;
    /** 上下文菜单每项高度（像素） */
    var CTX_ITEM_H = 28;
    /** 上下文菜单内边距（像素） */
    var CTX_PAD = 4;

    /**
     * 上下文菜单项定义
     * @type {Array<{label: string, action: string}>}
     */
    var CTX_ITEMS = [
        { label: '组队', action: 'party' },
        { label: '交易', action: 'trade' }
    ];

    /**
     * 玩家右键上下文菜单类
     * 继承自 L2_Base（L2 UI主题基类），在右键点击其他玩家时弹出。
     * 提供"组队"和"交易"两个操作选项。
     *
     * @constructor
     * @param {number} x - 菜单显示的屏幕X坐标
     * @param {number} y - 菜单显示的屏幕Y坐标
     * @param {Object} charData - 目标玩家的角色数据对象
     */
    function PlayerContextMenu(x, y, charData) {
        this.initialize(x, y, charData);
    }
    PlayerContextMenu.prototype = Object.create(L2_Base.prototype);
    PlayerContextMenu.prototype.constructor = PlayerContextMenu;

    /**
     * 初始化上下文菜单
     * 计算菜单总高度，将位置限制在屏幕范围内，
     * 调用 L2_Base 父类初始化，然后绘制菜单内容。
     *
     * @param {number} x - 菜单显示的屏幕X坐标
     * @param {number} y - 菜单显示的屏幕Y坐标
     * @param {Object} charData - 目标玩家的角色数据对象
     */
    PlayerContextMenu.prototype.initialize = function (x, y, charData) {
        var h = CTX_ITEMS.length * CTX_ITEM_H + CTX_PAD * 2;
        // 限制菜单位置在屏幕边界内
        x = Math.min(x, Graphics.boxWidth - CTX_W);
        y = Math.min(y, Graphics.boxHeight - h);
        L2_Base.prototype.initialize.call(this, x, y, CTX_W, h);
        this._charData = charData;
        this._hoverIdx = -1;   // 当前鼠标悬停的菜单项索引（-1表示无悬停）
        this._closed = false;  // 菜单是否已关闭
        this.refresh();
    };

    /**
     * 返回标准内边距
     * 覆盖 L2_Base 默认值，上下文菜单不需要内边距。
     *
     * @returns {number} 固定返回0
     */
    PlayerContextMenu.prototype.standardPadding = function () { return 0; };

    /**
     * 检查菜单是否已关闭
     *
     * @returns {boolean} 菜单是否已关闭
     */
    PlayerContextMenu.prototype.isClosed = function () { return this._closed; };

    /**
     * 重绘菜单内容
     * 绘制圆角矩形背景和边框，遍历菜单项绘制文字，
     * 悬停项添加高亮背景，相邻项之间绘制分隔线。
     */
    PlayerContextMenu.prototype.refresh = function () {
        var c = this.bmp();
        c.clear();
        var w = this.cw(), h = this.ch();

        // 绘制半透明圆角矩形背景
        L2_Theme.fillRoundRect(c, 0, 0, w, h, L2_Theme.cornerRadius, 'rgba(13,13,26,0.90)');
        // 绘制边框
        L2_Theme.strokeRoundRect(c, 0, 0, w, h, L2_Theme.cornerRadius, L2_Theme.borderDark);

        var self = this;
        CTX_ITEMS.forEach(function (item, i) {
            var iy = CTX_PAD + i * CTX_ITEM_H;
            // 悬停项绘制高亮背景
            if (i === self._hoverIdx) {
                c.fillRect(2, iy, w - 4, CTX_ITEM_H, L2_Theme.highlight);
            }
            // 绘制菜单项文字
            c.fontSize = L2_Theme.fontSmall;
            c.textColor = L2_Theme.textWhite;
            c.drawText(item.label, CTX_PAD + 4, iy, w - CTX_PAD * 2 - 8, CTX_ITEM_H, 'left');
            // 非末项绘制分隔线
            if (i < CTX_ITEMS.length - 1) {
                c.fillRect(CTX_PAD, iy + CTX_ITEM_H - 1, w - CTX_PAD * 2, 1, L2_Theme.borderDark);
            }
        });
    };

    /**
     * 关闭并销毁上下文菜单
     * 设置关闭标志并从父容器中移除自身
     */
    PlayerContextMenu.prototype.close = function () {
        this._closed = true;
        if (this.parent) this.parent.removeChild(this);
    };

    /**
     * 每帧更新上下文菜单
     * 处理鼠标悬停高亮效果和点击事件：
     * - 鼠标在菜单内移动时更新悬停索引并重绘
     * - 点击菜单项时发送对应的 WebSocket 消息（party_invite/trade_request）
     * - 点击菜单外部时关闭菜单
     */
    PlayerContextMenu.prototype.update = function () {
        L2_Base.prototype.update.call(this);
        if (this._closed) return;

        // 计算鼠标相对于菜单的局部坐标
        var mx = TouchInput.x - this.x, my = TouchInput.y - this.y;
        var inside = mx >= 0 && mx < CTX_W && my >= 0 && my < this.height;

        // 更新悬停索引
        var oldHover = this._hoverIdx;
        if (inside) {
            this._hoverIdx = Math.floor((my - CTX_PAD) / CTX_ITEM_H);
            if (this._hoverIdx < 0 || this._hoverIdx >= CTX_ITEMS.length) this._hoverIdx = -1;
        } else {
            this._hoverIdx = -1;
        }
        // 悬停项变化时重绘
        if (this._hoverIdx !== oldHover) this.refresh();

        // 处理点击事件
        if (TouchInput.isTriggered()) {
            if (inside && this._hoverIdx >= 0) {
                var action = CTX_ITEMS[this._hoverIdx].action;
                if (action === 'party') {
                    // 发送组队邀请
                    $MMO.send('party_invite', { target_char_id: this._charData.char_id });
                } else if (action === 'trade') {
                    // 发送交易请求
                    $MMO.send('trade_request', { target_char_id: this._charData.char_id });
                }
                this.close();
            } else {
                // 点击菜单外部——关闭菜单
                this.close();
            }
        }
    };

    // ═══════════════════════════════════════════════════════════════
    //  Scene_Map 钩子 — 处理右键点击其他玩家
    // ═══════════════════════════════════════════════════════════════

    /**
     * 钩入 Scene_Map.prototype.update
     * 在每帧更新中额外调用玩家上下文菜单的更新逻辑
     */
    var _Scene_Map_update_ctx = Scene_Map.prototype.update;
    Scene_Map.prototype.update = function () {
        _Scene_Map_update_ctx.call(this);
        this._updatePlayerContextMenu();
    };

    /**
     * 更新玩家上下文菜单状态
     * - 若菜单已存在且已关闭：清除引用
     * - 若菜单不存在且检测到右键点击：检查是否点击了其他玩家
     */
    Scene_Map.prototype._updatePlayerContextMenu = function () {
        var menu = this._playerContextMenu;
        if (menu) {
            if (menu.isClosed()) {
                this._playerContextMenu = null;
            }
            return;
        }
        // 检测右键点击（RMMV中 isCancelled 对应鼠标右键）
        if (TouchInput.isCancelled()) {
            this._checkPlayerRightClick();
        }
    };

    /**
     * 检查右键点击位置是否有其他玩家
     * 将屏幕坐标转换为地图格子坐标，遍历所有其他玩家精灵
     * 查找位于该格子的玩家。若找到，创建上下文菜单。
     */
    Scene_Map.prototype._checkPlayerRightClick = function () {
        var screenX = TouchInput.x;
        var screenY = TouchInput.y;
        // 将屏幕坐标转换为地图格子坐标
        var tileX = $gameMap.canvasToMapX(screenX);
        var tileY = $gameMap.canvasToMapY(screenY);

        // 在该格子位置查找其他玩家
        var target = null;
        var sprites = OtherPlayerManager._sprites;
        Object.keys(sprites).forEach(function (id) {
            var sp = sprites[id];
            var c = sp._character;
            if (Math.round(c._realX) === tileX && Math.round(c._realY) === tileY) {
                target = sp;
            }
        });

        if (!target) return;

        // 在右键点击位置创建上下文菜单
        var menu = new PlayerContextMenu(screenX, screenY, target._charData);
        this.addChild(menu);
        this._playerContextMenu = menu;
    };

    // ═══════════════════════════════════════════════════════════════
    //  全局导出
    // ═══════════════════════════════════════════════════════════════

    /** 将 OtherPlayerManager 挂载到全局 window 对象，供其他插件访问 */
    window.OtherPlayerManager = OtherPlayerManager;
    /** 将 Sprite_OtherPlayer 挂载到全局 window 对象，供其他插件使用 */
    window.Sprite_OtherPlayer = Sprite_OtherPlayer;

})();
