/*:
 * @plugindesc v2.0.0 MMO NPC - 服务端权威的 NPC 渲染与交互。
 * @author MMO Framework
 *
 * @help
 * 所有事件由服务端完全权威控制。客户端：
 *   - 不创建 Game_Event 对象（setupEvents 清空 _events）
 *   - 通过 Sprite_ServerNPC 从服务器数据（map_init.npcs）渲染 NPC
 *   - 通过 npc_sync 消息同步 NPC 位置
 *   - 在动作键按下时向服务器发送 npc_interact
 *   - 从服务器 npc_dialog / npc_choices 消息显示对话/选项
 *   - 传送事件由服务端在 HandleMove 中检测
 *
 * 确保零客户端事件处理 — 客户端只是服务器状态的纯渲染层。
 */

(function () {
    'use strict';

    /** NPC 移动队列最大长度。超出时直接瞬移到目标位置。 */
    var QUEUE_MAX = 10;

    // ═══════════════════════════════════════════════════════════
    //  Sprite_ServerNPC — 渲染服务端控制的 NPC 精灵
    //  继承 RMMV 的 Sprite_Character，使用 Game_Character 作为内部角色对象。
    //  支持角色图（walk_name）和图块图（tile_id）两种显示模式。
    // ═══════════════════════════════════════════════════════════

    /**
     * 服务端 NPC 精灵构造函数。
     * @param {Object} data - 服务器 NPC 数据
     * @param {number} data.event_id - 事件 ID
     * @param {string} data.walk_name - 行走图文件名
     * @param {number} data.walk_index - 行走图索引
     * @param {number} data.tile_id - 图块 ID（用于门/书架等）
     * @param {number} data.x - 地图 X 坐标
     * @param {number} data.y - 地图 Y 坐标
     * @param {number} data.dir - 朝向（2/4/6/8）
     * @param {number} data.priority_type - 优先级类型（0=下层,1=同层,2=上层）
     * @param {boolean} data.step_anime - 是否播放踏步动画
     * @param {boolean} data.direction_fix - 是否固定朝向
     * @param {boolean} data.through - 是否可穿透
     * @param {boolean} data.walk_anime - 是否播放行走动画
     */
    function Sprite_ServerNPC(data) {
        this.initialize(data);
    }
    Sprite_ServerNPC.prototype = Object.create(Sprite_Character.prototype);
    Sprite_ServerNPC.prototype.constructor = Sprite_ServerNPC;

    /**
     * 初始化 NPC 精灵。
     * 创建内部 Game_Character 并设置图像、位置、属性。
     * 无行走图且无图块 ID 的事件设为不可见（如传送触发器等隐形事件）。
     * @param {Object} data - 服务器 NPC 数据
     */
    Sprite_ServerNPC.prototype.initialize = function (data) {
        this._npcData = data;
        var ch = new Game_Character();
        var walkName = data.walk_name || '';
        var walkIndex = data.walk_index || 0;
        var tileId = data.tile_id || 0;

        if (tileId > 0) {
            // 图块事件（如门、书架）：使用 setTileImage 渲染正确的图块集图形。
            ch.setTileImage(tileId);
        } else {
            ch.setImage(walkName, walkIndex);
        }
        ch.setPosition(data.x || 0, data.y || 0);
        ch.setDirection(data.dir || 2);
        ch._moveSpeed = 3; // 标准 NPC 移动速度
        ch._priorityType = data.priority_type != null ? data.priority_type : 1;
        ch._stepAnime = !!data.step_anime;
        ch._directionFix = !!data.direction_fix;
        ch._through = !!data.through;
        ch._walkAnime = data.walk_anime != null ? data.walk_anime : true;
        Sprite_Character.prototype.initialize.call(this, ch);
        /** @type {Array} 待处理的移动指令队列。 */
        this._moveQueue = [];
        // 无图像的事件（隐形传送触发器等）设为不可见。
        if (!walkName && tileId === 0) this.visible = false;
    };

    /**
     * 同步 NPC 数据（位置/朝向）。
     * 距离超过 1 格或队列溢出时直接瞬移；否则加入移动队列平滑插值。
     * 仅朝向变化时直接设置朝向，不入队。
     * @param {Object} data - 同步数据 {event_id, x, y, dir}
     */
    Sprite_ServerNPC.prototype.syncData = function (data) {
        this._npcData = data;
        var c = this._character;

        // 以队列末尾位置为参考（而非当前位置），避免重复入队。
        var refX = c._x, refY = c._y;
        if (this._moveQueue.length > 0) {
            var last = this._moveQueue[this._moveQueue.length - 1];
            refX = last.x;
            refY = last.y;
        }
        var dx = Math.abs(data.x - refX);
        var dy = Math.abs(data.y - refY);

        // 距离过远或队列已满 — 直接瞬移。
        if (dx > 1 || dy > 1 || this._moveQueue.length >= QUEUE_MAX) {
            this._moveQueue = [];
            c._x = data.x;
            c._y = data.y;
            c._realX = data.x;
            c._realY = data.y;
            c.setDirection(data.dir || 2);
            return;
        }

        // 坐标有变化 — 加入移动队列。
        if (data.x !== refX || data.y !== refY) {
            this._moveQueue.push({ x: data.x, y: data.y, dir: data.dir || 2 });
        } else if (data.dir && data.dir !== c.direction()) {
            // 仅朝向变化 — 直接设置。
            c.setDirection(data.dir);
        }
    };

    /**
     * 处理页面切换（npc_page_change）。
     * 更新行走图/图块图、朝向、优先级等显示属性。
     * @param {Object} data - 页面数据
     */
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
        // 有图像时显示，否则隐藏。
        this.visible = !!walkName || tileId > 0;
    };

    /**
     * 每帧更新。
     * 当角色未在移动中时，从队列取出下一个移动指令并设置目标坐标。
     * Game_Character.update() 会自动将 _realX/_realY 插值到 _x/_y。
     */
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

    // ═══════════════════════════════════════════════════════════
    //  NPCManager — 管理所有服务端控制的 NPC 精灵
    //  负责精灵的创建、同步、页面切换和清理。
    //  支持场景切换时保存/恢复 NPC 数据。
    // ═══════════════════════════════════════════════════════════
    var NPCManager = {
        /** @type {Object.<number, Sprite_ServerNPC>} event_id → 精灵的映射。 */
        _sprites: {},
        /** @type {Object|null} 精灵容器（Spriteset_Map 的 _tilemap）。 */
        _container: null,
        /** @type {Array} 容器未就绪时暂存的 NPC 数据。 */
        _pending: [],
        /** @type {Array|null} 跨场景保存的 NPC 列表。 */
        _initNPCs: null,

        /**
         * 初始化管理器，绑定精灵容器。
         * 将保存的 NPC 数据和待处理队列合并后创建精灵。
         * @param {Object} container - PIXI 容器（通常为 Spriteset_Map._tilemap）
         */
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

        /**
         * 从 map_init 数据填充所有 NPC。
         * 先清空现有精灵，再创建新精灵。同时保存原始数据供场景恢复。
         * @param {Array} npcList - NPC 数据数组
         */
        populate: function (npcList) {
            this._initNPCs = npcList;
            this.clear();
            for (var i = 0; i < npcList.length; i++) {
                this._addOne(npcList[i]);
            }
        },

        /**
         * 添加单个 NPC。容器未就绪时加入待处理队列；已存在时更新数据。
         * @param {Object} data - NPC 数据
         */
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

        /**
         * 同步 NPC 位置/朝向（来自 npc_sync 消息）。
         * @param {Object} data - 同步数据
         */
        sync: function (data) {
            var sp = this._sprites[data.event_id];
            if (sp) sp.syncData(data);
        },

        /**
         * 处理 NPC 页面切换（来自 npc_page_change 消息）。
         * @param {Object} data - 页面数据
         */
        pageChange: function (data) {
            var sp = this._sprites[data.event_id];
            if (sp) sp.pageChange(data);
        },

        /**
         * 获取指定事件 ID 的 NPC 精灵。
         * @param {number} eventID - 事件 ID
         * @returns {Sprite_ServerNPC|undefined}
         */
        get: function (eventID) {
            return this._sprites[eventID];
        },

        /**
         * 清除所有 NPC 精灵，从容器中移除。
         */
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

    // ═══════════════════════════════════════════════════════════
    //  注入 NPCManager 到 Spriteset_Map
    // ═══════════════════════════════════════════════════════════

    /** 在 Spriteset_Map 创建角色精灵后初始化 NPCManager。 */
    var _Spriteset_Map_createCharacters_npc = Spriteset_Map.prototype.createCharacters;
    Spriteset_Map.prototype.createCharacters = function () {
        _Spriteset_Map_createCharacters_npc.call(this);
        NPCManager.init(this._tilemap);
    };

    /**
     * 场景切换时保存 NPC 数据（如打开菜单、进入战斗）。
     * 从当前精灵中提取最新坐标/朝向，供场景恢复时重建精灵。
     */
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

    // ═══════════════════════════════════════════════════════════
    //  禁用客户端 RMMV 事件处理（服务端权威）
    //
    //  所有事件通过 Sprite_ServerNPC 从服务器数据渲染。
    //  传送事件由服务端在 HandleMove 中检测。
    // ═══════════════════════════════════════════════════════════

    /**
     * 覆写 setupEvents：跳过创建 Game_Event 对象。
     * 保留公共事件（用于客户端并行公共事件，如画面色调、HUD 肖像等）。
     * _events 清空后，tileEvents() 方法自然返回空数组，无需额外覆写。
     */
    var _Game_Map_setupEvents = Game_Map.prototype.setupEvents;
    Game_Map.prototype.setupEvents = function () {
        // 保留并行公共事件（trigger=2 且对应开关为 ON 时激活）。
        this._commonEvents = this.parallelCommonEvents().map(function (ce) {
            return new Game_CommonEvent(ce.id);
        });
        // 清空 _events — 所有地图事件由服务端控制。
        this._events = [];
    };

    /**
     * 覆写 Game_Map.updateEvents — 确保并行公共事件正常运行。
     *
     * 关键问题：Game_CommonEvent.update() 仅在 _interpreter 存在时才执行，
     * 而 _interpreter 只由 refresh() 创建。refresh() 的触发链为：
     *   $gameSwitches.onChange() → $gameMap.requestRefresh() → refresh()
     *
     * 但游戏插件（OriginalCommands.js 等）直接写 $gameSwitches._data[N]
     * 绕过了 onChange()，导致 refresh() 永远不被调用，_interpreter 永远不创建，
     * 并行公共事件永远不执行。
     *
     * 修复：在每次 update 前强制调用 refresh()，确保条件变化时创建 interpreter。
     */
    var _Game_Map_updateEvents = Game_Map.prototype.updateEvents;
    Game_Map.prototype.updateEvents = function () {
        // 对每个公共事件先调用 refresh() 再 update()，
        // 确保开关变化后 interpreter 被正确创建。
        if (this._commonEvents) {
            for (var i = 0; i < this._commonEvents.length; i++) {
                this._commonEvents[i].refresh();
                this._commonEvents[i].update();
            }
        }
        // _events 为空，原始 updateEvents 中的事件循环不会产生副作用，
        // 但仍调用以保留其他可能的逻辑（如 map interpreter）。
        _Game_Map_updateEvents.call(this);
    };

    // ═══════════════════════════════════════════════════════════
    //  绕过所有 RMMV 事件触发门控
    //  RMMV 的事件触发链经过 100+ 个插件覆写
    //  （MKR_PlayerMoveForbid 始终返回 false，CommonInterceptor 保持
    //   $gameMap.isEventRunning() 为 true，等等）会静默阻断交互。
    //  在 MMO 模式下绕过整个链，直接检查 NPC。
    // ═══════════════════════════════════════════════════════════

    /**
     * 覆写 updateNonmoving — 处理玩家触碰（trigger 1/2）和动作键（trigger 0）。
     * 不调用 triggerAction/triggerButtonAction/checkEventTriggerThere
     * 以绕过被 157 个插件覆写破坏的触发链。
     * @param {boolean} wasMoving - 上一帧是否在移动
     */
    Game_Player.prototype.updateNonmoving = function (wasMoving) {
        if (wasMoving) {
            $gameParty.onPlayerWalk();
            // 玩家触碰/事件触碰：移动完成后检查当前格子。
            this.startMapEvent(this.x, this.y, [1, 2], false);
        }
        // 动作键：直接检测 OK 按键 — 绕过 triggerAction、canMove、
        // triggerButtonAction 和所有插件包装器。
        if (!$gameMessage.isBusy() && Input.isTriggered('ok')) {
            var dir = this.direction();
            var x2 = $gameMap.roundXWithDirection(this.x, dir);
            var y2 = $gameMap.roundYWithDirection(this.y, dir);
            // 检查玩家面对的格子（最常见情况）。
            this.startMapEvent(x2, y2, [0, 1, 2], true);
            // 也检查玩家自身所在格子（trigger=0 的脚下事件）。
            this.startMapEvent(this.x, this.y, [0], false);
        }
        if (wasMoving) {
            this.updateEncounterCount();
        } else {
            $gameTemp.clearDestination();
        }
    };

    /** canStartLocalEvents — 始终返回 true，保留残留 RMMV 代码路径的兼容性。 */
    Game_Player.prototype.canStartLocalEvents = function () {
        return true;
    };

    /**
     * 覆写 canMove — 对话期间阻止移动，但不阻止事件触发。
     * @returns {boolean} 是否允许移动
     */
    Game_Player.prototype.canMove = function () {
        if ($gameMessage.isBusy()) return false;
        if (this.isMoveRouteForcing() || this.areFollowersGathering()) return false;
        if (this._waitCount > 0) return false;
        return true;
    };

    /**
     * 覆写 startMapEvent — 向服务器发送 npc_interact 而非运行客户端事件解释器。
     * 由 updateNonmoving 直接调用。
     * 通过 Math.round(_realX) 判断 NPC 位置，支持移动中的碰撞检测。
     * @param {number} x - 目标 X 坐标
     * @param {number} y - 目标 Y 坐标
     * @param {Array} triggers - 触发类型数组 [0=动作键, 1=玩家触碰, 2=事件触碰]
     * @param {boolean} normal - 是否为法线方向检测
     */
    Game_Player.prototype.startMapEvent = function (x, y, triggers, normal) {
        if ($gameMessage.isBusy()) {
            if (MMO_CONFIG.debug) console.log('[MMO-NPC] startMapEvent 被阻止: $gameMessage.isBusy()');
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
            console.log('[MMO-NPC] startMapEvent: 坐标', x, y, '处无 NPC（精灵总数:', ids.length + '）');
        }
    };

    /**
     * 扩展碰撞检测以包含服务端 NPC 精灵。
     * 玩家无法穿过同优先级（priorityType=1）且不可穿透（through=false）的 NPC。
     */
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
            if (c._priorityType !== 1) continue; // 仅同优先级碰撞
            if (c._through) continue;            // 可穿透 NPC 不碰撞
            if (Math.round(c._realX) === x && Math.round(c._realY) === y) {
                return true;
            }
        }
        return false;
    };

    // ═══════════════════════════════════════════════════════════
    //  服务端对话显示 — npc_dialog / npc_choices / npc_dialog_end
    //  服务端 executor 发送的对话/选项指令，客户端通过 RMMV 的
    //  $gameMessage API 显示对话窗口和选项列表。
    // ═══════════════════════════════════════════════════════════

    /** @type {boolean} 标记当前是否有 NPC 对话激活。 */
    $MMO._npcDialogActive = false;

    /**
     * 处理 npc_dialog 消息 — 显示文本对话。
     * 设置面部图像、背景类型、位置类型，并逐行添加文本。
     * 若当前不在 Scene_Map 则暂存消息，在进入场景后处理。
     */
    $MMO.on('npc_dialog', function (data) {
        if (!data) return;
        $MMO._npcDialogActive = true;
        $MMO._npcDialogPending = true;
        var face = data.face || '';
        var faceIndex = data.face_index || 0;
        var background = data.background != null ? data.background : 0;
        var positionType = data.position_type != null ? data.position_type : 2;
        var lines = data.lines || [];

        // 不在 Scene_Map 时暂存，进入场景后处理。
        if (!(SceneManager._scene instanceof Scene_Map)) {
            console.warn('[MMO-NPC] 收到 npc_dialog 但不在 Scene_Map，暂存');
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

    /**
     * 覆写 Scene_Map.start — 发送 scene_ready 并处理暂存的对话。
     * 服务端在收到 scene_ready 后才开始执行自动运行事件，
     * 确保 Window_Message 等 UI 已创建完成。
     */
    var _Scene_Map_start_npc = Scene_Map.prototype.start;
    Scene_Map.prototype.start = function () {
        _Scene_Map_start_npc.call(this);
        // 通知服务端客户端场景已就绪，可以开始执行自动运行事件。
        if ($MMO && $MMO.send) {
            $MMO.send('scene_ready', {});
        }
        // 处理暂存的对话（场景加载期间收到的消息）。
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
            // 暂存数据可能同时包含选项（npc_dialog_choices 格式）。
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

    /**
     * 处理 npc_dialog_choices 消息 — 文本 + 选项组合显示。
     * RMMV 中"显示文本"后跟"显示选项"时文本保持可见，
     * 服务端将两者合并为一条消息发送以保持同步。
     */
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
        // RMMV 要求至少一行文本才能触发消息窗口。
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

    /**
     * 处理 npc_choices 消息 — 仅选项（无文本）。
     * RMMV 中纯选项也需要至少一行文本来触发消息窗口。
     */
    $MMO.on('npc_choices', function (data) {
        if (!data || !data.choices) return;
        $MMO._npcDialogActive = true;
        var choices = data.choices;
        var choiceDefault = data.default_type || 0;
        var choiceCancel = data.cancel_type != null ? data.cancel_type : -1;
        var choicePosition = data.position_type != null ? data.position_type : 2;
        var choiceBg = data.background || 0;

        // RMMV 要求至少一行文本才能触发消息窗口。
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

    /**
     * 处理 npc_dialog_end 消息 — 强制关闭对话。
     * 当服务端超时或事件执行完毕时发送，确保选项窗口不会卡住玩家。
     */
    $MMO.on('npc_dialog_end', function () {
        $MMO._npcDialogActive = false;
        $MMO._npcDialogPending = false;
        if ($gameMessage && $gameMessage.isBusy()) {
            $gameMessage.clear();
        }
    });

    /**
     * 钩子 Window_Message.terminateMessage — 发送对话确认。
     * 玩家按 OK 关闭文本对话时触发，通知服务端继续执行下一条指令。
     */
    var _Window_Message_terminateMessage = Window_Message.prototype.terminateMessage;
    Window_Message.prototype.terminateMessage = function () {
        _Window_Message_terminateMessage.call(this);
        if ($MMO._npcDialogPending) {
            $MMO._npcDialogPending = false;
            $MMO.send('npc_dialog_ack', {});
        }
    };

    // ═══════════════════════════════════════════════════════════
    //  服务端转发的视觉/音频效果 — npc_effect
    //
    //  服务端转发有视觉/音频副作用的 RMMV 指令代码。
    //  客户端使用 RMMV 内置 API 执行这些效果。
    //  指令代码与 RMMV 事件指令 ID 一一对应。
    //
    //  等待确认机制（wait:true）：
    //  服务端发送 wait:true 时，客户端在效果播放完成后
    //  发送 npc_effect_ack 通知服务端继续执行。
    //  两种回复策略：
    //  - 定时回复：已知持续帧数的效果（fadeout/tint/shake 等）
    //  - 轮询回复：完成状态不确定的效果（动画/气泡/移动路线）
    // ═══════════════════════════════════════════════════════════

    /**
     * 脚本 eval 白名单前缀。
     * 仅允许执行以这些前缀开头的脚本，防止被恶意内容利用。
     * 服务端已做过滤（只转发 $gameScreen. 和 AudioManager. 开头的脚本），
     * 客户端做二次验证以深度防御。
     */
    var SCRIPT_WHITELIST = ['$gameScreen.', 'AudioManager.'];

    $MMO.on('npc_effect', function (data) {
        if (!data) return;
        var code = data.code;
        var p = data.params || [];
        var needAck = !!data.wait;

        switch (code) {
        // --- 画面效果 ---
        case 221: // 淡出画面 — RMMV 使用 fadeSpeed()=24 帧
            $gameScreen.startFadeOut(24);
            break;
        case 222: // 淡入画面 — RMMV 使用 fadeSpeed()=24 帧
            $gameScreen.startFadeIn(24);
            break;
        case 223: // 更改画面色调 — [色调数组, 持续帧数, 等待]
            var tone = p[0] || [0, 0, 0, 0];
            $gameScreen.startTint(tone, paramInt(p, 1) || 60);
            break;
        case 224: // 闪烁画面 — [颜色数组, 持续帧数, 等待]
            var color = p[0] || [255, 255, 255, 170];
            $gameScreen.startFlash(color, paramInt(p, 1) || 30);
            break;
        case 225: // 震动画面 — [强度, 速度, 持续帧数, 等待]
            $gameScreen.startShake(paramInt(p, 0) || 5, paramInt(p, 1) || 5, paramInt(p, 2) || 30);
            break;

        // --- 天气效果 ---
        case 236: // 设置天气 — [类型, 强度, 持续帧数, 等待]
            if ($gameScreen && !$gameParty.inBattle()) {
                $gameScreen.changeWeather(
                    (p[0] || 'none').toString(),
                    paramInt(p, 1),
                    paramInt(p, 2)
                );
            }
            break;

        // --- 等待移动路线完成（代码 209） ---
        case 209:
            // 服务端发送 wait:true 时，客户端在移动路线完成后回复 ack。
            // 通过 pollEffectAck 监测 isMoveRouteForcing()。
            break;

        // --- 音频 ---
        case 241: // 播放 BGM — [{name, volume, pitch, pan}]
            if (p[0]) AudioManager.playBgm(p[0]);
            break;
        case 242: // 停止 BGM
            AudioManager.stopBgm();
            break;
        case 245: // 播放 BGS — [{name, volume, pitch, pan}]
            if (p[0]) AudioManager.playBgs(p[0]);
            break;
        case 246: // 停止 BGS
            AudioManager.stopBgs();
            break;
        case 249: // 播放 ME — [{name, volume, pitch, pan}]
            if (p[0]) AudioManager.playMe(p[0]);
            break;
        case 250: // 播放 SE — [{name, volume, pitch, pan}]
            if (p[0]) AudioManager.playSe(p[0]);
            break;
        case 251: // 停止 SE
            AudioManager.stopSe();
            break;

        // --- 更改透明度（代码 211） ---
        case 211:
            // RMMV code 211: params[0]=0 → 开启透明（不可见），
            //                params[0]=1 → 关闭透明（可见）。
            if ($gamePlayer) {
                $gamePlayer.setTransparent(paramInt(p, 0) === 0);
            }
            break;

        // --- 设置移动路线（代码 205） ---
        case 205:
            // params: [0]=角色ID（-1=玩家, 0=当前事件, N=事件ID）, [1]=moveRoute
            // 服务端已将 charId=0 解析为实际事件 ID。
            var charId = paramInt(p, 0);
            var moveRoute = p[1];
            if (!moveRoute) break;
            if (charId === -1 && $gamePlayer) {
                // 强制玩家角色执行移动路线。
                $gamePlayer.forceMoveRoute(moveRoute);
            } else if (charId > 0) {
                // NPC 移动路线：找到 NPC 精灵并应用到其角色对象。
                var npcSprite = NPCManager.get(charId);
                if (npcSprite && npcSprite._character) {
                    npcSprite._character.forceMoveRoute(moveRoute);
                } else if (MMO_CONFIG.debug) {
                    console.warn('[MMO-NPC] 移动路线: 未找到 event_id=' + charId + ' 的 NPC 精灵');
                }
            }
            break;

        // --- 图片操作 ---
        case 231: // 显示图片
            // params: [图片ID, 名称, 原点, 指定方式, x, y, 缩放X, 缩放Y, 不透明度, 混合模式]
            // 服务端已将变量坐标解析为直接值（designation=1→0）。
            if ($gameScreen) {
                $gameScreen.showPicture(
                    paramInt(p, 0),             // 图片ID
                    (p[1] || '').toString(),     // 名称
                    paramInt(p, 2),             // 原点
                    paramInt(p, 4),             // x（已解析）
                    paramInt(p, 5),             // y（已解析）
                    p[6] != null ? paramInt(p, 6) : 100,   // 缩放X
                    p[7] != null ? paramInt(p, 7) : 100,   // 缩放Y
                    p[8] != null ? paramInt(p, 8) : 255,   // 不透明度
                    paramInt(p, 9)              // 混合模式
                );
            }
            break;
        case 232: // 移动图片
            // params: [图片ID, (保留), 原点, 指定方式, x, y, 缩放X, 缩放Y, 不透明度, 混合模式, 持续帧数, 等待]
            // 服务端已将变量坐标解析（designation=1→0）。
            if ($gameScreen) {
                $gameScreen.movePicture(
                    paramInt(p, 0),             // 图片ID
                    paramInt(p, 2),             // 原点（RMMV 跳过 p[1]）
                    paramInt(p, 4),             // x（已解析）
                    paramInt(p, 5),             // y（已解析）
                    p[6] != null ? paramInt(p, 6) : 100,   // 缩放X
                    p[7] != null ? paramInt(p, 7) : 100,   // 缩放Y
                    p[8] != null ? paramInt(p, 8) : 255,   // 不透明度
                    paramInt(p, 9),             // 混合模式
                    paramInt(p, 10) || 1        // 持续帧数
                );
            }
            break;
        case 233: // 旋转图片
            if ($gameScreen) {
                $gameScreen.rotatePicture(paramInt(p, 0), paramInt(p, 1));
            }
            break;
        case 234: // 更改图片色调 — [图片ID, 色调, 持续帧数, 等待]
            if ($gameScreen) {
                $gameScreen.tintPicture(paramInt(p, 0), p[1] || [0,0,0,0], paramInt(p, 2) || 1);
            }
            break;
        case 235: // 消除图片
            if ($gameScreen) {
                $gameScreen.erasePicture(paramInt(p, 0));
            }
            break;

        // --- 显示动画/气泡 ---
        case 212: // 显示动画 — [角色ID, 动画ID, 等待]
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
        case 213: // 显示气泡图标 — [角色ID, 气泡ID, 等待]
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

        // --- 暂时消除事件 ---
        case 214:
            // 服务端消除的事件通过 NPC 可见性系统处理。
            break;

        // --- 金币/物品/武器/防具变更 ---
        case 125: // 增减金币
        case 126: // 增减物品
        case 127: // 增减武器
        case 128: // 增减防具
        // --- 角色属性变更（服务端转发） ---
        case 311: // 增减HP
        case 312: // 增减MP
        case 313: // 更改状态
        case 314: // 完全恢复
        case 315: // 增减经验值
        case 316: // 增减等级
        case 317: // 增减能力值
        case 318: // 增减技能
        case 319: // 变更装备
        case 320: // 变更名称
        case 321: // 变更职业
        case 322: // 变更角色图像
            // 通过 Game_Interpreter 执行以触发所有 RMMV 副作用。
            try {
                var si = new Game_Interpreter();
                si._eventId = 0;
                si._mapId = $gameMap ? $gameMap.mapId() : 0;
                // 构建只含此指令和终止符的指令列表。
                si._list = [
                    { code: code, indent: 0, parameters: p },
                    { code: 0, indent: 0, parameters: [] }
                ];
                si._index = 0;
                si.executeCommand();
            } catch (ex) {
                console.warn('[MMO-NPC] 属性指令 ' + code + ' 执行错误:', ex.message);
            }
            break;

        // --- 战斗/商店/游戏结束 ---
        case 301: // 战斗处理 — 服务端发送到客户端的回退方案
            try {
                var bi = new Game_Interpreter();
                bi._list = [
                    { code: 301, indent: 0, parameters: p },
                    { code: 0, indent: 0, parameters: [] }
                ];
                bi._index = 0;
                bi.executeCommand();
            } catch (ex) {
                console.warn('[MMO-NPC] 战斗指令执行错误:', ex.message);
            }
            break;
        case 302: // 商店处理
            // 服务端将 605（商品续行）聚合到 shop_goods 中。
            // 构建正确的指令列表供 RMMV command302 消费 605 条目。
            try {
                var shopInterp = new Game_Interpreter();
                var shopList = [{ code: 302, indent: 0, parameters: p }];
                if (data.shop_goods) {
                    for (var gi = 0; gi < data.shop_goods.length; gi++) {
                        shopList.push({ code: 605, indent: 0, parameters: data.shop_goods[gi] });
                    }
                }
                shopList.push({ code: 0, indent: 0, parameters: [] });
                shopInterp._list = shopList;
                shopInterp._index = 0;
                shopInterp.executeCommand();
            } catch (ex) {
                console.warn('[MMO-NPC] 商店指令执行错误:', ex.message);
            }
            break;
        case 353: // 游戏结束
            SceneManager.goto(Scene_Gameover);
            break;
        case 354: // 返回标题
            SceneManager.goto(Scene_Title);
            break;

        // --- 脚本（代码 355） ---
        case 355:
            // 服务端转发拼接后的脚本块供客户端执行。
            // 处理 $gameScreen.startTint() 等视觉指令。
            // 安全策略：仅允许白名单前缀的脚本执行。
            var scriptText = (p[0] || '').toString().trim();
            if (scriptText) {
                var allowed = false;
                for (var wi = 0; wi < SCRIPT_WHITELIST.length; wi++) {
                    if (scriptText.indexOf(SCRIPT_WHITELIST[wi]) === 0) {
                        allowed = true;
                        break;
                    }
                }
                if (allowed) {
                    try {
                        eval(scriptText);
                    } catch (ex) {
                        if (MMO_CONFIG.debug) {
                            console.warn('[MMO-NPC] 脚本执行错误:', ex.message, scriptText.substring(0, 100));
                        }
                    }
                } else if (MMO_CONFIG.debug) {
                    console.log('[MMO-NPC] 脚本被白名单拦截:', scriptText.substring(0, 80));
                }
            }
            break;

        // --- 插件指令（代码 356） ---
        case 356:
            // params: [0]="插件名 参数1 参数2 ..."
            var pluginCmd = (p[0] || '').toString();
            if (pluginCmd) {
                var args = pluginCmd.split(' ');
                var command = args.shift();
                // 创建 Game_Interpreter 实例以便插件覆写
                // （MPP_ChoiceEX、YEP_MessageCore 等）能访问其方法。
                try {
                    var interp = new Game_Interpreter();
                    interp._eventId = 0;
                    interp._mapId = $gameMap ? $gameMap.mapId() : 0;
                    interp.pluginCommand(command, args);
                } catch (ex) {
                    console.warn('[MMO-NPC] 插件指令执行错误:', command, ex.message);
                }
            }
            break;
        }

        // ──────────────────────────────────────────────────────
        //  效果确认（Effect Ack）
        //  服务端发送 wait:true 时，客户端在效果播放完成后
        //  发送 npc_effect_ack 通知服务端继续执行下一条事件指令。
        // ──────────────────────────────────────────────────────
        if (needAck) {
            switch (code) {
            // 已知持续帧数的效果 — 使用定时器延迟回复
            case 221: // 淡出画面
            case 222: // 淡入画面
                _scheduleEffectAck(24); // fadeSpeed() = 24 帧
                break;
            case 223: // 画面色调 — 持续帧数 = params[1]
                _scheduleEffectAck(paramInt(p, 1) || 1);
                break;
            case 224: // 画面闪烁 — 持续帧数 = params[1]
                _scheduleEffectAck(paramInt(p, 1) || 1);
                break;
            case 225: // 画面震动 — 持续帧数 = params[2]
                _scheduleEffectAck(paramInt(p, 2) || 1);
                break;
            case 232: // 移动图片 — 持续帧数 = params[10]
                _scheduleEffectAck(paramInt(p, 10) || 1);
                break;
            case 234: // 图片色调 — 持续帧数 = params[2]
                _scheduleEffectAck(paramInt(p, 2) || 1);
                break;
            case 236: // 天气效果 — 持续帧数 = params[2]
                _scheduleEffectAck(paramInt(p, 2) || 1);
                break;
            // 动画/气泡 — 通过轮询检测播放完成
            case 212: // 显示动画
                (function () {
                    var cid = paramInt(p, 0);
                    // 等待 2 帧（约33ms）让精灵系统处理动画请求，避免竞态。
                    setTimeout(function () {
                        _pollEffectAck(function () {
                            var ch = (cid === -1) ? $gamePlayer : _npcChar(cid);
                            return !ch || !ch.isAnimationPlaying();
                        });
                    }, 33);
                })();
                break;
            case 213: // 显示气泡
                (function () {
                    var cid = paramInt(p, 0);
                    setTimeout(function () {
                        _pollEffectAck(function () {
                            var ch = (cid === -1) ? $gamePlayer : _npcChar(cid);
                            return !ch || !ch.isBalloonPlaying();
                        });
                    }, 33);
                })();
                break;
            // 移动路线 — 轮询检测移动完成
            case 205: // 设置移动路线
            case 209: // 等待移动路线
                (function () {
                    var cid = paramInt(p, 0);
                    setTimeout(function () {
                        _pollEffectAck(function () {
                            var ch = (cid === -1) ? $gamePlayer : _npcChar(cid);
                            return !ch || !ch.isMoveRouteForcing();
                        });
                    }, 33);
                })();
                break;
            default:
                // 未知效果 — 立即回复避免服务端死锁。
                $MMO.send('npc_effect_ack', {});
                break;
            }
        }
    });

    /**
     * 按帧数延时发送 effect ack。
     * 将 RMMV 帧数转换为毫秒（帧数 × 1000/60）后使用 setTimeout。
     * @param {number} frames - 等待帧数
     */
    function _scheduleEffectAck(frames) {
        setTimeout(function () {
            $MMO.send('npc_effect_ack', {});
        }, Math.max(1, frames) * 1000 / 60);
    }

    /**
     * 轮询检测条件满足后发送 effect ack。
     * 每 16ms（约 1 帧）检查一次条件函数，满足或超过 30 秒安全超时后发送。
     * @param {Function} checkFn - 条件函数，返回 true 表示效果已完成
     */
    function _pollEffectAck(checkFn) {
        var elapsed = 0;
        var pollId = setInterval(function () {
            elapsed += 16;
            if (checkFn() || elapsed > 30000) {
                clearInterval(pollId);
                $MMO.send('npc_effect_ack', {});
            }
        }, 16);
    }

    /**
     * 获取 NPC 精灵对应的 Game_Character 对象。
     * @param {number} eventId - 事件 ID
     * @returns {Game_Character|null}
     */
    function _npcChar(eventId) {
        var sp = NPCManager.get(eventId);
        return sp ? sp._character : null;
    }

    /**
     * 安全提取参数数组中指定索引的整数值。
     * 索引越界时返回 0，非数字值转换为 0。
     * @param {Array} arr - 参数数组
     * @param {number} idx - 索引
     * @returns {number}
     */
    function paramInt(arr, idx) {
        if (idx >= arr.length) return 0;
        return Number(arr[idx]) || 0;
    }

    // ═══════════════════════════════════════════════════════════
    //  WebSocket 消息处理
    // ═══════════════════════════════════════════════════════════

    /** map_init 消息 — 填充 NPC 列表。 */
    $MMO.on('map_init', function (data) {
        var npcs = data.npcs || [];
        NPCManager.populate(npcs);
    });

    /** npc_sync 消息 — 同步 NPC 位置/朝向。 */
    $MMO.on('npc_sync', function (data) {
        if (data && data.event_id) {
            NPCManager.sync(data);
        }
    });

    /** npc_page_change 消息 — 更新 NPC 页面显示属性。 */
    $MMO.on('npc_page_change', function (data) {
        if (data && data.event_id) {
            NPCManager.pageChange(data);
        }
    });

    /** 连接断开时清除所有 NPC 精灵。 */
    $MMO.on('_disconnected', function () {
        NPCManager.clear();
    });

    // ═══════════════════════════════════════════════════════════
    //  导出全局对象
    // ═══════════════════════════════════════════════════════════
    window.NPCManager = NPCManager;
    window.Sprite_ServerNPC = Sprite_ServerNPC;

})();
