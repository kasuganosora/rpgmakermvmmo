/*:
 * @plugindesc v4.3.0 MMO 服务端权威回合制战斗（傀儡模式）。
 * @author MMO Framework
 *
 * @help
 * 本插件拦截 RMMV 的战斗系统，使其运行在"傀儡模式"下：
 * - 服务端控制所有战斗逻辑（伤害、AI、状态、奖励）
 * - 客户端通过直接精灵调用渲染动画（不使用 Window_BattleLog，
 *   因为 YEP_BattleEngineCore 会清空它）
 * - 玩家输入发送给服务端而非本地处理
 *
 * v4.0 — 完全绕过 Window_BattleLog 播放动画；使用直接
 *         target.startAnimation() + 更新循环 isAnimationPlaying() 计时。
 *         从服务端快照同步角色属性（MHP/MMP/ATK/...）。
 */

(function () {
    'use strict';

    // ═══════════════════════════════════════════════════════════
    //  傀儡模式状态变量
    // ═══════════════════════════════════════════════════════════
    /** @type {boolean} 是否处于傀儡战斗模式。 */
    var _puppetMode = false;
    /** @type {Array} 服务端发送的己方角色数据。 */
    var _puppetActors = [];
    /** @type {Array} 服务端发送的敌方数据。 */
    var _puppetEnemies = [];
    /** @type {Object|null} 待处理的输入请求数据。 */
    var _pendingInputRequest = null;
    /** @type {boolean} 战斗场景是否准备就绪。 */
    var _puppetSceneReady = false;
    /** @type {number} 场景就绪等待帧计数。 */
    var _puppetReadyFrames = 0;

    // ── 动画流水线状态 ──
    /** @type {Array} 待播放的事件队列（action/turn_end）。 */
    var _puppetEventQueue = [];
    /** @type {boolean} 是否正在处理动作动画。 */
    var _processingAction = false;
    /** @type {number} 当前动作已等待的帧数。 */
    var _puppetActionWait = 0;
    /** @type {boolean} 当前动作的伤害是否已应用。 */
    var _puppetDamageApplied = false;
    /** @type {Array} 当前动作的目标数据列表。 */
    var _puppetTargetData = [];
    /** @type {boolean} 是否正在结束战斗。 */
    var _puppetEndingBattle = false;

    // ── 目标选择状态 ──
    /** @type {number} 待发送的技能 ID。 */
    var _puppetPendingSkillId = 0;
    /** @type {number} 待发送的物品 ID。 */
    var _puppetPendingItemId = 0;

    /** 标记 $MMO 当前是否处于服务端战斗中。 */
    $MMO._serverBattle = false;

    // ═══════════════════════════════════════════════════════════
    //  CallStand.js 兼容性 — 确保存储属性存在
    //  CallStand.js 在每个插件命令中无条件访问
    //  $gameActors.actor(2).toneArray[armorId] 和 .toneWeapon[weaponId]。
    //  这些属性由 CE 2（地图上的并行自动运行）初始化，但
    //  CE 2 在战斗期间或战斗刚结束 NPC 事件恢复时不会运行。
    //  钩住 Game_Actors.actor() 来惰性确保这些属性存在。
    // ═══════════════════════════════════════════════════════════

    /**
     * 覆写 Game_Actors.actor 以确保 CallStand.js 所需属性存在。
     * Actor 2: toneArray（防具色调）、toneWeapon（武器色调）。
     * Actor 1: hairTone/hairToneb（发色色调）。
     */
    var _GA_actor = Game_Actors.prototype.actor;
    Game_Actors.prototype.actor = function (actorId) {
        var actor = _GA_actor.call(this, actorId);
        if (!actor) return actor;
        if (actorId === 2) {
            if (!actor.toneArray) actor.toneArray = new Array(1000);
            if (!actor.toneWeapon) actor.toneWeapon = new Array(100);
        }
        if (actorId === 1) {
            if (!actor.hairTone) actor.hairTone = [0, 0, 0, 0, 0];
            if (!actor.hairToneb) actor.hairToneb = [0, 0, 0, 0, 0];
        }
        return actor;
    };

    // ═══════════════════════════════════════════════════════════
    //  属性覆盖 — 傀儡模式下强制使用服务端角色属性
    //  服务端发送的属性快照存储在 actor._puppetParams 中，
    //  覆盖 RMMV 本地的 param() 计算结果。
    // ═══════════════════════════════════════════════════════════

    /**
     * 覆写 Game_BattlerBase.param，在傀儡模式下优先返回服务端属性值。
     * @param {number} paramId - 属性 ID（0=MHP, 1=MMP, 2=ATK, ...）
     * @returns {number} 属性值
     */
    var _GBB_param = Game_BattlerBase.prototype.param;
    Game_BattlerBase.prototype.param = function (paramId) {
        if (_puppetMode && this._puppetParams && this._puppetParams[paramId] !== undefined) {
            return this._puppetParams[paramId];
        }
        return _GBB_param.call(this, paramId);
    };

    // ═══════════════════════════════════════════════════════════
    //  服务端消息处理器
    // ═══════════════════════════════════════════════════════════

    /**
     * 处理 battle_battle_start 消息。
     * 初始化傀儡模式状态，创建战斗敌人，同步角色 HP/MP，
     * 设置空战场背景并推入 Scene_Battle。
     */
    $MMO.on('battle_battle_start', function (data) {
        if (!data) return;

        console.log('[Puppet] 战斗开始, actors=' +
            (data.actors ? data.actors.length : 0) +
            ' enemies=' + (data.enemies ? data.enemies.length : 0));

        // 重置所有傀儡模式状态。
        _puppetMode = true;
        $MMO._serverBattle = true;
        _puppetActors = data.actors || [];
        _puppetEnemies = data.enemies || [];
        _puppetEventQueue = [];
        _processingAction = false;
        _puppetActionWait = 0;
        _puppetDamageApplied = false;
        _puppetTargetData = [];
        _pendingInputRequest = null;
        _puppetSceneReady = false;
        _puppetReadyFrames = 0;
        _puppetPendingSkillId = 0;
        _puppetPendingItemId = 0;

        // 确保队伍数据存在（至少需要 $dataTroops[1]）。
        if (!$dataTroops || !$dataTroops[1]) {
            console.warn('[Puppet] 无可用队伍数据');
            return;
        }

        // 同步己方角色 HP/MP/TP，设置服务端属性覆盖。
        if (!$gameParty._actors) $gameParty._actors = [];
        for (var j = 0; j < _puppetActors.length; j++) {
            var actorData = _puppetActors[j];
            var actorId = j + 1;
            var actor = $gameActors.actor(actorId);
            if (actor) {
                actor._hp = actorData.hp;
                actor._mp = actorData.mp;
                actor._tp = actorData.tp || 0;
                // 用服务端计算值覆盖本地属性。
                actor._puppetParams = {};
                actor._puppetParams[0] = actorData.max_hp || actorData.hp;
                actor._puppetParams[1] = actorData.max_mp || actorData.mp;
                console.log('[Puppet] Actor ' + actorId + ' HP=' + actorData.hp +
                    '/' + actor._puppetParams[0] +
                    ' MP=' + actorData.mp + '/' + actor._puppetParams[1]);
            }
            if ($gameParty._actors.indexOf(actorId) < 0) {
                $gameParty._actors.push(actorId);
            }
        }

        // 使用队伍 1 进行战斗设置。
        BattleManager.setup(1, true, true);

        // 清空默认敌人，用服务端敌人数据重建。
        var troop = $gameTroop;
        if (troop && troop._enemies) {
            troop._enemies = [];
            /** 敌人默认位置坐标（最多支持 8 个敌人）。 */
            var defaultPositions = [
                [408, 340], [460, 380], [356, 380], [500, 310],
                [312, 310], [540, 350], [270, 350], [408, 280]
            ];
            for (var i = 0; i < _puppetEnemies.length; i++) {
                var enemyData = _puppetEnemies[i];
                var enemyId = enemyData.enemy_id || 1;
                if (!$dataEnemies[enemyId] || !$dataEnemies[enemyId].battlerName) {
                    console.warn('[Puppet] 敌人 ID ' + enemyId + ' 无精灵图，跳过');
                    continue;
                }
                var pos = defaultPositions[i] || [408, 340];
                var gameEnemy = new Game_Enemy(enemyId, pos[0], pos[1]);
                gameEnemy._hp = enemyData.hp || 1;
                gameEnemy._mp = enemyData.mp || 0;
                troop._enemies.push(gameEnemy);
            }
            troop.makeUniqueNames();
        }

        // 临时覆盖战场背景为空（使用透明/默认背景）。
        var _origBB1 = $gameMap.battleback1Name;
        var _origBB2 = $gameMap.battleback2Name;
        $gameMap.battleback1Name = function () { return ''; };
        $gameMap.battleback2Name = function () { return ''; };

        // 战斗结束回调：恢复战场背景。
        BattleManager.setEventCallback(function (result) {
            console.log('[Puppet] 战斗场景关闭, result=' + result);
            $gameMap.battleback1Name = _origBB1;
            $gameMap.battleback2Name = _origBB2;
        });
        $gamePlayer.makeEncounterCount();
        SceneManager.push(Scene_Battle);
    });

    /**
     * 处理 battle_input_request 消息。
     * 服务端请求玩家为指定角色选择行动。
     */
    $MMO.on('battle_input_request', function (data) {
        if (!_puppetMode) return;
        console.log('[Puppet] 输入请求, actor index=' + data.actor_index);
        _pendingInputRequest = data;
    });

    /**
     * 处理 battle_turn_start 消息。
     * 服务端通知回合开始。
     */
    $MMO.on('battle_turn_start', function (data) {
        if (!_puppetMode) return;
        console.log('[Puppet] 回合开始, turn=' + (data ? data.turn_count : '?'));
    });

    /**
     * 处理 battle_action_result 消息。
     * 将动作结果加入动画队列等待播放。
     */
    $MMO.on('battle_action_result', function (data) {
        if (!_puppetMode || !data) return;
        console.log('[Puppet] 动作结果入队: subject=' +
            (data.subject ? data.subject.name : '?') +
            ' skill=' + data.skill_id +
            ' item=' + (data.item_id || 0) +
            ' targets=' + (data.targets ? data.targets.length : 0));
        _puppetEventQueue.push({ type: 'action', data: data });
    });

    /**
     * 处理 battle_turn_end 消息。
     * 将回合结束事件加入队列（包含回合结束回复数据）。
     */
    $MMO.on('battle_turn_end', function (data) {
        if (!_puppetMode) return;
        console.log('[Puppet] 回合结束入队');
        _puppetEventQueue.push({ type: 'turn_end', data: data || {} });
    });

    /**
     * 处理 battle_battle_end 消息。
     * 清理傀儡状态，应用战斗奖励（经验/金币/掉落），
     * 调用 BattleManager 的胜利/逃跑/失败流程。
     * 最后通知服务端战斗结果已处理。
     *
     * 注意：本地 gainExp/gainGold/gainItem 与服务端权威模式可能产生冲突，
     * 但当前设计是服务端也会同步，此处先做本地预表现。
     */
    $MMO.on('battle_battle_end', function (data) {
        if (!_puppetMode || !data) return;
        console.log('[Puppet] 战斗结束, result=' + data.result);

        _pendingInputRequest = null;
        _puppetEventQueue = [];
        _processingAction = false;

        // result=0: 胜利，发放奖励。
        if (data.result === 0) {
            if (data.exp > 0) {
                for (var i = 0; i < $gameParty.members().length; i++) {
                    var actor = $gameParty.members()[i];
                    if (actor && actor.isAlive()) {
                        actor.gainExp(data.exp);
                    }
                }
            }
            if (data.gold > 0) {
                $gameParty.gainGold(data.gold);
            }
            if (data.drops) {
                for (var d = 0; d < data.drops.length; d++) {
                    var drop = data.drops[d];
                    var item = null;
                    if (drop.item_type === 1) item = $dataItems[drop.item_id];
                    else if (drop.item_type === 2) item = $dataWeapons[drop.item_id];
                    else if (drop.item_type === 3) item = $dataArmors[drop.item_id];
                    if (item) $gameParty.gainItem(item, drop.quantity || 1);
                }
            }
        }

        // 清除所有角色的傀儡属性覆盖。
        _clearPuppetParams();

        _puppetEndingBattle = true;
        _puppetMode = false;
        $MMO._serverBattle = false;

        // 根据结果调用对应的 BattleManager 流程。
        if (data.result === 0) {
            BattleManager.processVictory();
        } else if (data.result === 1) {
            BattleManager.processEscape();
        } else {
            BattleManager.processDefeat();
        }

        _puppetEndingBattle = false;
        // 通知服务端客户端已处理完战斗结果。
        $MMO.send('npc_battle_result', { result: data.result });
    });

    /**
     * 清除所有角色上的 _puppetParams 属性覆盖。
     * 战斗结束后恢复正常属性计算。
     */
    function _clearPuppetParams() {
        for (var a = 1; a <= 20; a++) {
            var actor = $gameActors.actor(a);
            if (actor) delete actor._puppetParams;
        }
    }

    // ═══════════════════════════════════════════════════════════
    //  傀儡队伍管理
    //  确保 $gameParty 包含正确的角色并同步 HP/MP/TP。
    // ═══════════════════════════════════════════════════════════

    /**
     * 确保傀儡队伍成员存在并且 HP/MP/TP 与服务端数据一致。
     * 在输入请求处理时调用，防止队伍数据不完整。
     */
    function _ensurePuppetParty() {
        if (!_puppetActors.length) return;
        if (!$gameParty._actors) $gameParty._actors = [];

        for (var j = 0; j < _puppetActors.length; j++) {
            var actorId = j + 1;
            if ($gameParty._actors.indexOf(actorId) < 0) {
                $gameParty._actors.push(actorId);
            }
            var actor = $gameActors.actor(actorId);
            var ad = _puppetActors[j];
            if (actor && ad) {
                actor._hp = ad.hp || 1;
                actor._mp = ad.mp || 0;
                actor._tp = ad.tp || 0;
            }
        }
        if (!$gameParty._inBattle) {
            $gameParty._inBattle = true;
        }
    }

    // ═══════════════════════════════════════════════════════════
    //  动画流水线
    //  绕过 Window_BattleLog，直接在精灵上播放动画。
    //  YEP_BattleEngineCore 清空了 Window_BattleLog 的方法，
    //  所以必须使用直接精灵调用。
    // ═══════════════════════════════════════════════════════════

    /**
     * 开始播放一个动作的动画。
     * 创建 Game_Action 查找动画 ID，在目标精灵上播放动画，
     * 对敌方行动者施加白闪视觉反馈。
     * @param {Object} data - 动作数据（subject, skill_id, item_id, targets）
     */
    function _startPuppetAction(data) {
        _processingAction = true;
        _puppetActionWait = 0;
        _puppetDamageApplied = false;
        _puppetTargetData = data.targets || [];

        var subject = _getBattler(data.subject);
        if (!subject) {
            console.warn('[Puppet] 未找到行动者，跳过');
            _processingAction = false;
            return;
        }

        // 创建 Game_Action 用于查找动画 ID。
        var skillId = data.skill_id || 1;
        var action = new Game_Action(subject);
        if (data.item_id > 0 && $dataItems[data.item_id]) {
            action.setItem(data.item_id);
        } else if ($dataSkills[skillId]) {
            action.setSkill(skillId);
        } else {
            action.setSkill(1);
        }

        var item = action.item();
        var animId = item ? item.animationId : 0;
        console.log('[Puppet] >> 动作: ' + subject.name() +
            ' skill=' + skillId + ' animId=' + animId +
            ' targets=' + _puppetTargetData.length);

        BattleManager._subject = subject;
        BattleManager._action = action;
        BattleManager._phase = 'puppetAction';

        // 解析目标引用为实际战斗者对象。
        var targets = [];
        for (var i = 0; i < _puppetTargetData.length; i++) {
            var tgt = _getBattler(_puppetTargetData[i].target);
            if (tgt) targets.push(tgt);
        }

        // ── 直接在目标上播放动画 ──
        // （绕过被 YEP BEC 清空的 Window_BattleLog）
        if (animId < 0) {
            // animationId = -1 表示使用普通攻击动画。
            _showAttackAnimation(subject, targets);
        } else if (animId > 0) {
            // 技能/物品指定动画：在所有目标上播放。
            for (var t = 0; t < targets.length; t++) {
                targets[t].startAnimation(animId, false, 0);
            }
        }
        // animId === 0: 无动画（如纯增益技能"变身"）。

        // 无动画且无可见伤害时立即应用结果。
        if (animId === 0 && !_hasVisibleDamage(_puppetTargetData)) {
            _applyAllTargetResults(_puppetTargetData);
            _puppetDamageApplied = true;
        }

        // 敌方行动者视觉反馈（白闪/震动效果）。
        if (subject.isEnemy && subject.isEnemy()) {
            var scene = SceneManager._scene;
            if (scene && scene._spriteset) {
                var sprites = scene._spriteset.battlerSprites ?
                    scene._spriteset.battlerSprites() : [];
                for (var s = 0; s < sprites.length; s++) {
                    if (sprites[s]._battler === subject) {
                        // 白闪效果表示敌人正在行动。
                        sprites[s]._effectType = 'whiten';
                        sprites[s]._effectDuration = 16;
                        break;
                    }
                }
            }
        }
    }

    /**
     * 播放普通攻击动画。
     * 角色使用 attackAnimationId1()（武器动画），
     * 无武器或返回 0 时使用默认动画 ID 1。
     * @param {Game_Battler} subject - 攻击者
     * @param {Array} targets - 目标战斗者列表
     */
    function _showAttackAnimation(subject, targets) {
        if (targets.length === 0) return;

        var attackAnimId = 1; // 默认普通攻击动画

        if (subject.attackAnimationId1) {
            attackAnimId = subject.attackAnimationId1();
        }
        // attackAnimationId1 返回 0（未定义）时使用默认动画 1。
        if (!attackAnimId || attackAnimId <= 0) {
            attackAnimId = 1;
        }

        if (attackAnimId > 0) {
            for (var t = 0; t < targets.length; t++) {
                targets[t].startAnimation(attackAnimId, false, 0);
            }
        }

        console.log('[Puppet] 攻击动画 ID=' + attackAnimId +
            ' 目标数=' + targets.length);
    }

    /**
     * 检查目标数据列表中是否有可见伤害。
     * @param {Array} targetDataList - 目标数据数组
     * @returns {boolean} 是否有伤害或未命中
     */
    function _hasVisibleDamage(targetDataList) {
        for (var i = 0; i < targetDataList.length; i++) {
            if (targetDataList[i].damage !== 0 || targetDataList[i].missed) return true;
        }
        return false;
    }

    /**
     * 对所有目标应用战斗结果（伤害弹窗、状态变化等）。
     * 应用完毕后刷新状态窗口。
     * @param {Array} targetDataList - 目标数据数组
     */
    function _applyAllTargetResults(targetDataList) {
        for (var i = 0; i < targetDataList.length; i++) {
            _applyTargetResult(targetDataList[i]);
        }
        // 刷新状态窗口显示最新 HP/MP。
        var scene = SceneManager._scene;
        if (scene && scene instanceof Scene_Battle && scene._statusWindow) {
            scene._statusWindow.refresh();
        }
    }

    /**
     * 对单个目标应用战斗结果。
     * 包括：伤害/暴击/未命中判定、HP/MP 同步、状态增减、
     * 伤害弹窗显示、死亡倒下动画、关联公共事件执行。
     * @param {Object} tgtData - 单个目标的结果数据
     */
    function _applyTargetResult(tgtData) {
        var battler = _getBattler(tgtData.target);
        if (!battler) return;

        var result = battler.result();
        result.clear();
        result.used = true;

        if (tgtData.missed) {
            // 未命中/闪避。
            result.missed = true;
            result.evaded = true;
        } else {
            // 伤害应用。
            if (tgtData.damage !== 0) {
                result.hpDamage = tgtData.damage;
                result.hpAffected = true;
            }
            result.critical = tgtData.critical || false;

            // 同步 HP/MP 到服务端计算值。
            if (tgtData.hp_after !== undefined) battler._hp = tgtData.hp_after;
            if (tgtData.mp_after !== undefined) battler._mp = tgtData.mp_after;

            // 添加状态。
            if (tgtData.added_states) {
                for (var s = 0; s < tgtData.added_states.length; s++) {
                    var stateId = tgtData.added_states[s];
                    battler.addState(stateId);
                    if (result.addedStates) result.addedStates.push(stateId);
                }
            }
            // 移除状态。
            if (tgtData.removed_states) {
                for (var r = 0; r < tgtData.removed_states.length; r++) {
                    battler.removeState(tgtData.removed_states[r]);
                    if (result.removedStates) result.removedStates.push(tgtData.removed_states[r]);
                }
            }
        }

        // 显示伤害弹窗。
        battler.startDamagePopup();

        // 处理死亡：播放倒下动画。
        if (battler.isDead && battler.isDead()) {
            battler.performCollapse();
        }

        // 执行技能效果触发的公共事件。
        if (tgtData.common_event_ids) {
            for (var ce = 0; ce < tgtData.common_event_ids.length; ce++) {
                var ceId = tgtData.common_event_ids[ce];
                console.log('[Puppet] 执行公共事件 ' + ceId);
                _executeCommonEvent(ceId);
            }
        }
    }

    /**
     * 临时启用/禁用游戏对象的 _serverSync 门控。
     * 在傀儡战斗中执行公共事件时，开关/变量/物品需要
     * 通过 mmo-npc.js 中的 _serverSync 门控，以便公共事件
     * （如 CE 1031 变身）能正确修改客户端游戏状态。
     * @param {boolean} enable - 是否启用同步门控
     */
    function _enableServerSync(enable) {
        if ($gameParty) $gameParty._serverSync = enable;
        if ($gameSwitches) $gameSwitches._serverSync = enable;
        if ($gameVariables) $gameVariables._serverSync = enable;
        if ($gameSelfSwitches) $gameSelfSwitches._serverSync = enable;
    }

    /**
     * 跳过解释器中失败的命令。
     * 如果错误发生在子解释器中（如 CE 891 被 CE 1031 调用），
     * 清除子解释器让父解释器继续；否则推进顶层解释器。
     * @param {Game_Interpreter} interp - 解释器实例
     */
    function _skipFailedInterpreterCommand(interp) {
        if (!interp) return;
        // 如果子解释器处于活动状态，错误可能来自子解释器。
        // 清除子解释器让父解释器继续执行下一条命令。
        if (interp._childInterpreter && interp._childInterpreter.isRunning()) {
            interp._childInterpreter.clear();
            interp._childInterpreter = null;
            return;
        }
        // 否则推进当前解释器到下一条命令。
        if (interp._list && interp._index < interp._list.length) {
            interp._index++;
        } else {
            interp.clear();
        }
    }

    /**
     * 在傀儡战斗中执行公共事件。
     * 将事件设置到队伍解释器上，由更新循环驱动执行。
     * @param {number} ceId - 公共事件 ID
     */
    function _executeCommonEvent(ceId) {
        if (!$dataCommonEvents || !$dataCommonEvents[ceId]) {
            console.warn('[Puppet] 公共事件 ' + ceId + ' 未找到');
            return;
        }
        var ce = $dataCommonEvents[ceId];
        if (!ce || !ce.list || ce.list.length === 0) return;

        // 直接设置到队伍解释器上。
        // 傀儡模式下 setupBattleEvent 被阻止，但我们需要公共事件运行。
        if ($gameTroop && $gameTroop._interpreter) {
            $gameTroop._interpreter.setup(ce.list);
            console.log('[Puppet] 公共事件 ' + ceId + ' 已设置到解释器 (' +
                ce.list.length + ' 条命令)');
        }
    }

    /**
     * 应用回合结束的回复数据（HP/MP/TP 自然回复）。
     * 显示回复量弹窗并刷新状态窗口。
     * @param {Object} data - 回合结束数据，包含 regen 数组
     */
    function _applyTurnEnd(data) {
        if (data.regen) {
            for (var i = 0; i < data.regen.length; i++) {
                var regen = data.regen[i];
                var battler = _getBattler(regen.battler);
                if (!battler) continue;
                battler._hp = Math.max(0, battler._hp + (regen.hp_change || 0));
                battler._mp = Math.max(0, battler._mp + (regen.mp_change || 0));
                battler._tp = Math.min(100, Math.max(0, (battler._tp || 0) + (regen.tp_change || 0)));
                if (regen.hp_change) {
                    var result = battler.result();
                    result.clear();
                    result.used = true;
                    result.hpDamage = -(regen.hp_change || 0);
                    result.hpAffected = true;
                    battler.startDamagePopup();
                }
            }
        }
        var scene = SceneManager._scene;
        if (scene && scene instanceof Scene_Battle && scene._statusWindow) {
            scene._statusWindow.refresh();
        }
    }

    /**
     * 完成当前傀儡动作，清理 BattleManager 状态。
     * 调用行动者的 performActionEnd()，重置阶段为 waiting。
     */
    function _finishPuppetAction() {
        console.log('[Puppet] 动作完成, 耗时 ' + _puppetActionWait +
            ' 帧, 队列剩余=' + _puppetEventQueue.length);
        var subj = BattleManager._subject;
        if (subj && subj.performActionEnd) subj.performActionEnd();
        BattleManager._subject = null;
        BattleManager._action = null;
        BattleManager._phase = 'waiting';
        _processingAction = false;
        _puppetDamageApplied = false;
        _puppetTargetData = [];
    }

    // ═══════════════════════════════════════════════════════════
    //  输入请求处理
    //  服务端请求玩家为角色选择行动时的处理逻辑。
    // ═══════════════════════════════════════════════════════════

    /**
     * 处理服务端的输入请求。
     * 确保队伍状态正确，设置 BattleManager 为输入阶段，
     * 激活角色命令窗口供玩家选择行动。
     * @param {Object} data - 输入请求数据（actor_index）
     */
    function _processInputRequest(data) {
        _pendingInputRequest = null;
        console.log('[Puppet] 处理输入请求, actor index=' + data.actor_index);

        _ensurePuppetParty();

        // 确保所有战斗成员有行动对象。
        var battleMembers = $gameParty.battleMembers();
        for (var i = 0; i < battleMembers.length; i++) {
            if (battleMembers[i] && (!battleMembers[i]._actions || !battleMembers[i]._actions.length)) {
                battleMembers[i].makeActions();
            }
        }

        BattleManager._actorIndex = data.actor_index;
        BattleManager._phase = 'input';

        var actor = BattleManager.actor();
        if (!actor && data.actor_index < battleMembers.length) {
            actor = battleMembers[data.actor_index];
        }

        var scene = SceneManager._scene;
        if (scene && scene instanceof Scene_Battle && scene._actorCommandWindow) {
            if (actor) {
                scene._statusWindow.select(data.actor_index);
                if (scene._partyCommandWindow) scene._partyCommandWindow.close();
                scene._actorCommandWindow.setup(actor);
                console.log('[Puppet] 命令窗口已激活: ' + actor.name());
            } else {
                console.warn('[Puppet] actor index=' + data.actor_index + ' 无对应角色');
            }
        } else {
            // 场景尚未就绪，重新入队。
            console.warn('[Puppet] 场景未就绪，重新入队输入请求');
            _pendingInputRequest = data;
        }
    }

    // ═══════════════════════════════════════════════════════════
    //  Scene_Battle 生命周期钩子
    // ═══════════════════════════════════════════════════════════

    /**
     * 覆写 Scene_Battle.start。
     * 傀儡模式下重置就绪帧计数，等待场景完全初始化。
     */
    var _Scene_Battle_start = Scene_Battle.prototype.start;
    Scene_Battle.prototype.start = function () {
        _Scene_Battle_start.call(this);
        if (_puppetMode) {
            _puppetReadyFrames = 0;
            console.log('[Puppet] Scene_Battle.start(), 等待场景稳定...');
        }
    };

    /**
     * 覆写 Scene_Battle.update — 傀儡模式的主更新循环。
     * 在傀儡模式下完全替代原始 update：
     * - 跳过 BattleManager.update()（避免本地战斗逻辑冲突）
     * - 只调用渲染/UI 相关的基础更新
     * - 驱动队伍解释器（执行战斗中的公共事件）
     * - 管理动画计时和伤害应用
     * - 处理事件队列和输入请求
     */
    var _Scene_Battle_update = Scene_Battle.prototype.update;
    Scene_Battle.prototype.update = function () {
        if (!_puppetMode) {
            _Scene_Battle_update.call(this);
            return;
        }

        // 傀儡模式下跳过原始 Scene_Battle.update。
        // 原始 update 调用 BattleManager.update() → $gameTroop.updateInterpreter()
        // 会在无错误处理的情况下执行解释器，并运行与傀儡控制冲突的
        // 本地战斗阶段逻辑。只调用我们需要的渲染/UI 部分。
        Scene_Base.prototype.update.call(this);  // 子元素、输入、淡入淡出
        if ($gameScreen) $gameScreen.update();
        if ($gameTimer) $gameTimer.update(this.isActive());
        this.updateStatusWindow();
        this.updateWindowPositions();

        // 等待场景完全初始化（15 帧后且命令窗口存在）。
        if (!_puppetSceneReady) {
            _puppetReadyFrames++;
            if (_puppetReadyFrames >= 15 && this._actorCommandWindow) {
                _puppetSceneReady = true;
                console.log('[Puppet] 场景就绪, 耗时 ' + _puppetReadyFrames + ' 帧');
            }
            return;
        }

        // ── 驱动队伍解释器（处理技能触发的公共事件）──
        if ($gameTroop && $gameTroop._interpreter && $gameTroop._interpreter.isRunning()) {
            // 临时启用 _serverSync 门控以允许公共事件修改游戏状态。
            // mmo-npc.js 中的门控会阻止开关/变量/物品修改，
            // 但 CE 1031（变身）等公共事件需要它们正常工作。
            _enableServerSync(true);
            try {
                $gameTroop._interpreter.update();
            } catch (e) {
                // ProjectB 插件命令可能依赖地图上下文状态（如
                // 并行 CE 2 初始化的 toneArray），这些在战斗期间不存在。
                // 跳过失败的命令继续执行，以便后续命令（装备、转职等）
                // 仍能执行。
                console.warn('[Puppet] CE 命令错误（跳过）:', e.message);
                _skipFailedInterpreterCommand($gameTroop._interpreter);
            }
            _enableServerSync(false);
            // 等待解释器执行完毕再处理下一个事件。
            return;
        }

        // ── 动画计时循环 ──
        if (_processingAction) {
            _puppetActionWait++;
            var animPlaying = this._spriteset && this._spriteset.isAnimationPlaying();

            // 约第 12 帧时应用伤害（视觉上"命中"的时刻）。
            if (!_puppetDamageApplied && _puppetActionWait >= 12) {
                _applyAllTargetResults(_puppetTargetData);
                _puppetDamageApplied = true;
            }

            // 结束条件：（动画播放完毕且至少 30 帧）或安全超时 180 帧。
            if ((!animPlaying && _puppetActionWait >= 30) || _puppetActionWait >= 180) {
                if (_puppetActionWait >= 180) {
                    console.warn('[Puppet] 动作超时（180 帧）');
                }
                // 如果伤害仍未应用则强制应用。
                if (!_puppetDamageApplied) {
                    _applyAllTargetResults(_puppetTargetData);
                    _puppetDamageApplied = true;
                }
                _finishPuppetAction();
            }
            return;
        }

        // ── 处理排队的事件 ──
        if (_puppetEventQueue.length > 0) {
            var evt = _puppetEventQueue.shift();
            console.log('[Puppet] 出队事件: ' + evt.type + ', 剩余=' + _puppetEventQueue.length);
            if (evt.type === 'action') {
                _startPuppetAction(evt.data);
            } else if (evt.type === 'turn_end') {
                _applyTurnEnd(evt.data);
            }
            return;
        }

        // ── 处理待处理的输入请求（仅在队列完全清空后）──
        if (_pendingInputRequest && this._actorCommandWindow) {
            _processInputRequest(_pendingInputRequest);
        }
    };

    // ═══════════════════════════════════════════════════════════
    //  BattleManager 钩子 — 傀儡模式下阻止所有本地战斗逻辑
    //  这些覆写确保 RMMV 原生的战斗流程不会干扰服务端控制。
    // ═══════════════════════════════════════════════════════════

    /** 阻止本地 startInput，改为等待服务端的输入请求。 */
    var _BM_startInput = BattleManager.startInput;
    BattleManager.startInput = function () {
        if (_puppetMode) {
            this._phase = 'waiting';
            return;
        }
        _BM_startInput.call(this);
    };

    /** 阻止本地 startTurn，回合由服务端控制。 */
    var _BM_startTurn = BattleManager.startTurn;
    BattleManager.startTurn = function () {
        if (_puppetMode) return;
        _BM_startTurn.call(this);
    };

    /** 阻止本地 updateAction，动作由服务端发送结果。 */
    var _BM_updateAction = BattleManager.updateAction;
    BattleManager.updateAction = function () {
        if (_puppetMode) return;
        _BM_updateAction.call(this);
    };

    /** 阻止本地 endAction。 */
    var _BM_endAction = BattleManager.endAction;
    BattleManager.endAction = function () {
        if (_puppetMode) return;
        _BM_endAction.call(this);
    };

    /** 阻止本地 invokeNormalAction。 */
    var _BM_invokeNormalAction = BattleManager.invokeNormalAction;
    BattleManager.invokeNormalAction = function (subject, target) {
        if (_puppetMode) return;
        _BM_invokeNormalAction.call(this, subject, target);
    };

    /** 阻止本地 setupBattleEvent（队伍事件由公共事件机制替代）。 */
    var _GT_setupBattleEvent = Game_Troop.prototype.setupBattleEvent;
    Game_Troop.prototype.setupBattleEvent = function () {
        if (_puppetMode) return;
        _GT_setupBattleEvent.call(this);
    };

    /** 阻止本地 checkBattleEnd，战斗结束由服务端判定。 */
    var _BM_checkBattleEnd = BattleManager.checkBattleEnd;
    BattleManager.checkBattleEnd = function () {
        if (_puppetMode) return false;
        return _BM_checkBattleEnd.call(this);
    };

    /** 阻止本地 checkAbort。 */
    var _BM_checkAbort = BattleManager.checkAbort;
    BattleManager.checkAbort = function () {
        if (_puppetMode) return false;
        return _BM_checkAbort.call(this);
    };

    /** 阻止本地 processVictory（除非由 battle_battle_end 触发）。 */
    var _BM_processVictory = BattleManager.processVictory;
    BattleManager.processVictory = function () {
        if (_puppetMode && !_puppetEndingBattle) return;
        _BM_processVictory.call(this);
    };

    /** 阻止本地 processDefeat（除非由 battle_battle_end 触发）。 */
    var _BM_processDefeat = BattleManager.processDefeat;
    BattleManager.processDefeat = function () {
        if (_puppetMode && !_puppetEndingBattle) return;
        _BM_processDefeat.call(this);
    };

    /** 阻止本地 processEscape（除非由 battle_battle_end 触发）。 */
    var _BM_processEscape = BattleManager.processEscape;
    BattleManager.processEscape = function () {
        if (_puppetMode && !_puppetEndingBattle) return;
        return _BM_processEscape.call(this);
    };

    // ═══════════════════════════════════════════════════════════
    //  解释器错误安全 — 在源头捕获插件命令错误
    //  傀儡模式下，技能触发的公共事件（如 CE 1031 变身）
    //  可能调用依赖地图上下文状态（toneArray、视差数据等）
    //  的插件命令，这些在战斗期间不存在。
    //  包装 command356 在最内层捕获错误，防止它们传播到
    //  SceneManager.catchException 冻结战斗。
    // ═══════════════════════════════════════════════════════════

    /**
     * 覆写 command356（插件命令）。
     * 傀儡模式下包装 try-catch，防止插件命令错误冻结战斗。
     */
    var _GI_command356 = Game_Interpreter.prototype.command356;
    Game_Interpreter.prototype.command356 = function () {
        if (_puppetMode) {
            try {
                return _GI_command356.call(this);
            } catch (e) {
                console.warn('[Puppet] 插件命令错误（跳过）: ' + e.message);
                return true; // 继续执行下一条命令
            }
        }
        return _GI_command356.call(this);
    };

    // ═══════════════════════════════════════════════════════════
    //  Scene_Battle 输入钩子
    //  拦截玩家的战斗命令选择，发送给服务端处理。
    // ═══════════════════════════════════════════════════════════

    /**
     * 覆写攻击命令。
     * 傀儡模式下打开敌人选择窗口而非本地处理。
     */
    var _Scene_Battle_commandAttack = Scene_Battle.prototype.commandAttack;
    Scene_Battle.prototype.commandAttack = function () {
        if (_puppetMode) {
            _puppetPendingSkillId = 0;
            _puppetPendingItemId = 0;
            _selectEnemyTarget.call(this);
            return;
        }
        _Scene_Battle_commandAttack.call(this);
    };

    /** 保留原始技能命令行为（打开技能列表窗口）。 */
    var _Scene_Battle_commandSkill = Scene_Battle.prototype.commandSkill;
    Scene_Battle.prototype.commandSkill = function () {
        _Scene_Battle_commandSkill.call(this);
    };

    /**
     * 覆写防御命令。
     * 傀儡模式下直接向服务端发送防御输入。
     */
    var _Scene_Battle_commandGuard = Scene_Battle.prototype.commandGuard;
    Scene_Battle.prototype.commandGuard = function () {
        if (_puppetMode) {
            $MMO.send('battle_input', {
                actor_index: BattleManager._actorIndex,
                action_type: 3,
            });
            BattleManager._phase = 'waiting';
            return;
        }
        _Scene_Battle_commandGuard.call(this);
    };

    /** 保留原始物品命令行为（打开物品列表窗口）。 */
    var _Scene_Battle_commandItem = Scene_Battle.prototype.commandItem;
    Scene_Battle.prototype.commandItem = function () {
        _Scene_Battle_commandItem.call(this);
    };

    /**
     * 覆写逃跑命令。
     * 傀儡模式下直接向服务端发送逃跑输入。
     */
    var _Scene_Battle_commandEscape = Scene_Battle.prototype.commandEscape;
    Scene_Battle.prototype.commandEscape = function () {
        if (_puppetMode) {
            $MMO.send('battle_input', {
                actor_index: BattleManager._actorIndex,
                action_type: 4,
            });
            BattleManager._phase = 'waiting';
            return;
        }
        _Scene_Battle_commandEscape.call(this);
    };

    /**
     * 覆写技能确认回调。
     * 傀儡模式下根据技能目标类型决定：
     * - 无需选择目标：直接发送输入
     * - 对敌方：打开敌人选择窗口
     * - 对己方：打开角色选择窗口
     */
    var _Scene_Battle_onSkillOk = Scene_Battle.prototype.onSkillOk;
    Scene_Battle.prototype.onSkillOk = function () {
        if (_puppetMode) {
            var skill = this._skillWindow.item();
            if (!skill) return;

            var actor = BattleManager.actor();
            if (!actor) return;
            var action = new Game_Action(actor);
            action.setSkill(skill.id);

            this._skillWindow.hide();

            if (!action.needsSelection()) {
                // 无需选择目标（如自身增益技能）。
                $MMO.send('battle_input', {
                    actor_index: BattleManager._actorIndex,
                    action_type: 1,
                    skill_id: skill.id,
                    target_indices: [BattleManager._actorIndex],
                    target_is_actor: true,
                });
                BattleManager._phase = 'waiting';
            } else if (action.isForOpponent()) {
                // 对敌方技能：选择敌人目标。
                _puppetPendingSkillId = skill.id;
                _puppetPendingItemId = 0;
                _selectEnemyTarget.call(this);
            } else {
                // 对己方技能：选择角色目标。
                _puppetPendingSkillId = skill.id;
                _puppetPendingItemId = 0;
                _selectActorTarget.call(this);
            }
            return;
        }
        _Scene_Battle_onSkillOk.call(this);
    };

    /**
     * 覆写物品确认回调。
     * 逻辑同技能确认，根据物品目标类型选择发送方式。
     */
    var _Scene_Battle_onItemOk = Scene_Battle.prototype.onItemOk;
    Scene_Battle.prototype.onItemOk = function () {
        if (_puppetMode) {
            var item = this._itemWindow.item();
            if (!item) return;

            var actor = BattleManager.actor();
            if (!actor) return;
            var action = new Game_Action(actor);
            action.setItem(item.id);

            this._itemWindow.hide();

            if (!action.needsSelection()) {
                // 无需选择目标。
                $MMO.send('battle_input', {
                    actor_index: BattleManager._actorIndex,
                    action_type: 2,
                    item_id: item.id,
                    target_indices: [BattleManager._actorIndex],
                    target_is_actor: true,
                });
                BattleManager._phase = 'waiting';
            } else if (action.isForOpponent()) {
                // 对敌方物品。
                _puppetPendingSkillId = 0;
                _puppetPendingItemId = item.id;
                _selectEnemyTarget.call(this);
            } else {
                // 对己方物品。
                _puppetPendingSkillId = 0;
                _puppetPendingItemId = item.id;
                _selectActorTarget.call(this);
            }
            return;
        }
        _Scene_Battle_onItemOk.call(this);
    };

    /**
     * 覆写敌人确认回调。
     * 傀儡模式下将选中的敌人索引和待发送的技能/物品 ID
     * 打包发送给服务端。
     */
    var _Scene_Battle_onEnemyOk = Scene_Battle.prototype.onEnemyOk;
    Scene_Battle.prototype.onEnemyOk = function () {
        if (_puppetMode) {
            var enemyIndex = this._enemyWindow.enemyIndex();
            var actionType = 0;
            var skillId = 0;
            var itemId = 0;

            if (_puppetPendingSkillId > 0) {
                actionType = 1;
                skillId = _puppetPendingSkillId;
            } else if (_puppetPendingItemId > 0) {
                actionType = 2;
                itemId = _puppetPendingItemId;
            }

            $MMO.send('battle_input', {
                actor_index: BattleManager._actorIndex,
                action_type: actionType,
                skill_id: skillId,
                item_id: itemId,
                target_indices: [enemyIndex],
                target_is_actor: false,
            });

            _puppetPendingSkillId = 0;
            _puppetPendingItemId = 0;
            this._enemyWindow.hide();
            BattleManager._phase = 'waiting';
            return;
        }
        _Scene_Battle_onEnemyOk.call(this);
    };

    /**
     * 覆写敌人取消回调。
     * 傀儡模式下隐藏敌人窗口，重新显示角色命令窗口。
     */
    var _Scene_Battle_onEnemyCancel = Scene_Battle.prototype.onEnemyCancel;
    Scene_Battle.prototype.onEnemyCancel = function () {
        if (_puppetMode) {
            this._enemyWindow.hide();
            _puppetPendingSkillId = 0;
            _puppetPendingItemId = 0;
            var actor = BattleManager.actor();
            if (actor && this._actorCommandWindow) {
                this._actorCommandWindow.setup(actor);
            }
            return;
        }
        _Scene_Battle_onEnemyCancel.call(this);
    };

    /**
     * 覆写角色确认回调（选择己方目标时）。
     * 傀儡模式下发送选中的角色索引给服务端。
     */
    var _Scene_Battle_onActorOk = Scene_Battle.prototype.onActorOk;
    Scene_Battle.prototype.onActorOk = function () {
        if (_puppetMode) {
            var actorIndex = this._actorWindow.index();
            var actionType = _puppetPendingItemId > 0 ? 2 : 1;

            $MMO.send('battle_input', {
                actor_index: BattleManager._actorIndex,
                action_type: actionType,
                skill_id: _puppetPendingSkillId,
                item_id: _puppetPendingItemId,
                target_indices: [actorIndex],
                target_is_actor: true,
            });

            _puppetPendingSkillId = 0;
            _puppetPendingItemId = 0;
            this._actorWindow.hide();
            BattleManager._phase = 'waiting';
            return;
        }
        _Scene_Battle_onActorOk.call(this);
    };

    /**
     * 覆写角色取消回调。
     * 傀儡模式下隐藏角色窗口，重新显示角色命令窗口。
     */
    var _Scene_Battle_onActorCancel = Scene_Battle.prototype.onActorCancel;
    Scene_Battle.prototype.onActorCancel = function () {
        if (_puppetMode) {
            this._actorWindow.hide();
            _puppetPendingSkillId = 0;
            _puppetPendingItemId = 0;
            var actor = BattleManager.actor();
            if (actor && this._actorCommandWindow) {
                this._actorCommandWindow.setup(actor);
            }
            return;
        }
        _Scene_Battle_onActorCancel.call(this);
    };

    // ═══════════════════════════════════════════════════════════
    //  辅助工具函数
    // ═══════════════════════════════════════════════════════════

    /**
     * 根据服务端引用查找对应的战斗者对象。
     * @param {Object} ref - 引用对象 { is_actor: boolean, index: number }
     * @returns {Game_Battler|null} 对应的战斗者，未找到返回 null
     */
    function _getBattler(ref) {
        if (!ref) return null;
        if (ref.is_actor) {
            var members = $gameParty.battleMembers();
            if (ref.index < members.length) return members[ref.index];
        } else {
            var enemies = $gameTroop.members();
            if (ref.index < enemies.length) return enemies[ref.index];
        }
        return null;
    }

    /**
     * 打开敌人选择窗口。
     * 在 Scene_Battle 上下文中调用（this = Scene_Battle）。
     */
    function _selectEnemyTarget() {
        if (this._enemyWindow) {
            this._enemyWindow.refresh();
            this._enemyWindow.show();
            this._enemyWindow.select(0);
            this._enemyWindow.activate();
        }
    }

    /**
     * 打开角色选择窗口（用于己方目标选择）。
     * 在 Scene_Battle 上下文中调用（this = Scene_Battle）。
     */
    function _selectActorTarget() {
        if (this._actorWindow) {
            this._actorWindow.refresh();
            this._actorWindow.show();
            this._actorWindow.select(0);
            this._actorWindow.activate();
        }
    }

})();
