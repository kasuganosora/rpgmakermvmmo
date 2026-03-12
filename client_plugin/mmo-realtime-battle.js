/*:
 * @plugindesc v1.0.0 MMO 即时战斗模式模块 — 战斗模式切换、目标锁定、攻击冷却、自动攻击、复活请求。
 * @author MMO Framework
 *
 * @help
 * 本插件根据服务器配置的 combatMode（turnbased / realtime / hybrid）
 * 自动切换客户端战斗行为：
 *
 * ■ turnbased 模式：
 *   - 禁用地图上的点击攻击怪物
 *   - 所有战斗走 Scene_Battle（傀儡模式）
 *
 * ■ realtime 模式：
 *   - 启用地图上的即时战斗（点击/快捷键攻击）
 *   - 禁用 Scene_Battle（遇到强制战斗时自动胜利跳过）
 *   - 显示目标锁定指示器、攻击冷却条
 *   - 支持 Tab 键切换目标、空格键攻击、A 键自动攻击
 *   - 玩家死亡后按 R 键请求复活
 *
 * ■ hybrid 模式（默认）：
 *   - 两者共存：即时攻击和回合制战斗均可用
 *
 * 战斗模式通过 map_init 消息的 combat_mode 字段从服务器获取，
 * 不需要客户端额外配置。
 *
 * 依赖：mmo-core.js, mmo-battle.js
 * 加载顺序：在 mmo-battle.js 之后
 */

(function () {
    'use strict';

    // ═══════════════════════════════════════════════════════════════
    //  战斗模式状态
    // ═══════════════════════════════════════════════════════════════

    /** 当前战斗模式，从服务器 map_init 获取 */
    var _combatMode = 'hybrid';

    /** 当前锁定的目标怪物 inst_id（0 = 无目标） */
    var _lockedTarget = 0;

    /** 是否启用自动攻击 */
    var _autoAttack = false;

    /** 自动攻击计时器（帧） */
    var _autoAttackTimer = 0;

    /** 自动攻击间隔（帧，与 GCD 同步） */
    var _autoAttackInterval = 60; // 默认 1 秒 (60 帧)

    /** 攻击冷却剩余（帧） */
    var _attackCooldown = 0;

    /** 攻击冷却总时长（帧） */
    var _attackCooldownMax = 60;

    /** 玩家是否死亡 */
    var _isDead = false;

    // ═══════════════════════════════════════════════════════════════
    //  公共 API — 挂载到 $MMO
    // ═══════════════════════════════════════════════════════════════

    /**
     * 获取当前战斗模式。
     * @returns {string} "turnbased" | "realtime" | "hybrid"
     */
    $MMO.getCombatMode = function () { return _combatMode; };

    /**
     * 即时攻击是否可用（realtime 或 hybrid 模式）。
     * @returns {boolean}
     */
    $MMO.isFieldAttackEnabled = function () { return _combatMode !== 'turnbased'; };

    /**
     * 回合制战斗是否可用（turnbased 或 hybrid 模式）。
     * @returns {boolean}
     */
    $MMO.isTurnBattleEnabled = function () { return _combatMode !== 'realtime'; };

    /**
     * 获取当前锁定目标 ID。
     * @returns {number} inst_id 或 0
     */
    $MMO.getLockedTarget = function () { return _lockedTarget; };

    // ═══════════════════════════════════════════════════════════════
    //  map_init — 从服务器获取战斗模式
    // ═══════════════════════════════════════════════════════════════

    $MMO.on('map_init', function (data) {
        if (data.combat_mode) {
            _combatMode = data.combat_mode;
        } else {
            _combatMode = 'hybrid';
        }
        // 重置状态
        _lockedTarget = 0;
        _autoAttack = false;
        _autoAttackTimer = 0;
        _attackCooldown = 0;
        _isDead = false;

        if ($MMO._debug) {
            console.log('[MMO-RT] 战斗模式: ' + _combatMode);
        }
    });

    // ═══════════════════════════════════════════════════════════════
    //  攻击冷却同步
    // ═══════════════════════════════════════════════════════════════

    /** 服务器返回 attack_cooldown 错误时触发冷却显示 */
    $MMO.on('error', function (data) {
        if (data && data.message === 'attack on cooldown') {
            // 服务器告知正在冷却，刷新冷却条
            _attackCooldown = _attackCooldownMax;
        }
    });

    /** 攻击成功后设置冷却 */
    $MMO.on('battle_result', function (data) {
        // 攻击成功，开始冷却
        _attackCooldown = _attackCooldownMax;
    });

    // ═══════════════════════════════════════════════════════════════
    //  死亡/复活状态追踪
    // ═══════════════════════════════════════════════════════════════

    $MMO.on('player_death', function () {
        _isDead = true;
        _autoAttack = false;
        _lockedTarget = 0;
    });

    $MMO.on('player_revive', function () {
        _isDead = false;
    });

    // ═══════════════════════════════════════════════════════════════
    //  点击攻击拦截 — turnbased 模式下禁用
    // ═══════════════════════════════════════════════════════════════

    // 保存 mmo-battle.js 中设置的 processMapTouch
    var _prevProcessMapTouch = Scene_Map.prototype.processMapTouch;

    /**
     * 覆写 processMapTouch，在 turnbased 模式下阻止怪物攻击点击。
     * 在 realtime/hybrid 模式下额外实现目标锁定。
     */
    Scene_Map.prototype.processMapTouch = function () {
        if (TouchInput.isTriggered() && $gameMap && $MMO.isFieldAttackEnabled()) {
            // 检查 L2_InputBlocker
            if (typeof L2_InputBlocker !== 'undefined' &&
                L2_InputBlocker.isBlocking(TouchInput.x, TouchInput.y)) {
                return;
            }
            var tileX = Math.floor((TouchInput.x + $gameMap.displayX() * $gameMap.tileWidth()) / $gameMap.tileWidth());
            var tileY = Math.floor((TouchInput.y + $gameMap.displayY() * $gameMap.tileHeight()) / $gameMap.tileHeight());

            // 检测怪物
            if (window.MonsterManager) {
                var hit = null;
                Object.keys(MonsterManager._sprites).forEach(function (id) {
                    var sp = MonsterManager._sprites[id];
                    if (sp._tileX === tileX && sp._tileY === tileY) hit = parseInt(id);
                });
                if (hit !== null) {
                    // 锁定目标并攻击
                    _lockedTarget = hit;
                    if (!_isDead && _attackCooldown <= 0) {
                        $MMO.send('attack', { target_id: hit, target_type: 'monster' });
                    }
                    return;
                }

                // 检测掉落物
                var dropHit = null;
                Object.keys(MonsterManager._drops).forEach(function (id) {
                    var sp = MonsterManager._drops[id];
                    if (sp._data.x === tileX && sp._data.y === tileY) dropHit = parseInt(id);
                });
                if (dropHit !== null) {
                    $MMO.send('pickup_item', { drop_id: dropHit });
                    return;
                }
            }
            // 点击空地取消锁定
            _lockedTarget = 0;
            _autoAttack = false;
        }

        if (_combatMode === 'turnbased') {
            // turnbased 模式下跳过 mmo-battle.js 的攻击处理，
            // 直接调用最原始的 processMapTouch（角色移动）
            // _prevProcessMapTouch is mmo-battle.js's version which internally
            // falls through to the original RMMV movement handler
            _prevProcessMapTouch.call(this);
            return;
        }

        // hybrid 模式下不再调用 _prevProcessMapTouch（我们已经处理了攻击），
        // 但仍需要处理角色移动
        _prevProcessMapTouch.call(this);
    };

    // ═══════════════════════════════════════════════════════════════
    //  键盘快捷键
    // ═══════════════════════════════════════════════════════════════

    /**
     * 键盘按键处理：
     * - Space/Z: 攻击锁定目标
     * - Tab: 切换最近的目标
     * - A: 切换自动攻击
     * - R: 死亡时请求复活
     */
    var _Scene_Map_update = Scene_Map.prototype.update;
    Scene_Map.prototype.update = function () {
        _Scene_Map_update.call(this);

        if (!$MMO.isFieldAttackEnabled()) return;

        // 冷却递减
        if (_attackCooldown > 0) _attackCooldown--;

        // R 键复活
        if (_isDead && Input.isTriggered('r')) {
            $MMO.send('revive_request', {});
            return;
        }

        if (_isDead) return;

        // Tab 键切换目标
        if (Input.isTriggered('tab')) {
            _switchTarget();
        }

        // Space/Z 键攻击
        if (Input.isTriggered('ok') && _lockedTarget && _attackCooldown <= 0) {
            _sendAttack(_lockedTarget);
        }

        // A 键切换自动攻击
        if (Input.isTriggered('a')) {
            _autoAttack = !_autoAttack;
            if (_autoAttack && !_lockedTarget) {
                _switchTarget(); // 自动选择最近目标
            }
        }

        // 自动攻击逻辑
        if (_autoAttack && _lockedTarget && _attackCooldown <= 0) {
            _autoAttackTimer++;
            if (_autoAttackTimer >= 10) { // 小延迟防止过快
                _autoAttackTimer = 0;
                _sendAttack(_lockedTarget);
            }
        }

        // 检查锁定目标是否仍然存在
        if (_lockedTarget && window.MonsterManager && !MonsterManager._sprites[_lockedTarget]) {
            _lockedTarget = 0;
            if (_autoAttack) {
                // 自动切换到下一个目标
                _switchTarget();
                if (!_lockedTarget) _autoAttack = false;
            }
        }
    };

    /**
     * 发送攻击请求。
     * @param {number} targetId - 怪物 inst_id
     */
    function _sendAttack(targetId) {
        _attackCooldown = _attackCooldownMax;
        $MMO.send('attack', { target_id: targetId, target_type: 'monster' });
    }

    /**
     * 切换到距离玩家最近的怪物目标。
     */
    function _switchTarget() {
        if (!window.MonsterManager || !$gamePlayer) return;
        var px = $gamePlayer.x, py = $gamePlayer.y;
        var bestId = 0, bestDist = Infinity;

        Object.keys(MonsterManager._sprites).forEach(function (id) {
            var sp = MonsterManager._sprites[id];
            var d = Math.abs(sp._tileX - px) + Math.abs(sp._tileY - py);
            // 跳过当前目标，选择下一个最近的
            if (parseInt(id) === _lockedTarget && Object.keys(MonsterManager._sprites).length > 1) return;
            if (d < bestDist) {
                bestDist = d;
                bestId = parseInt(id);
            }
        });
        _lockedTarget = bestId;
    }

    // 注册 RMMV 输入映射
    if (!Input.keyMapper[82]) Input.keyMapper[82] = 'r';     // R key
    if (!Input.keyMapper[65]) Input.keyMapper[65] = 'a';     // A key

    // ═══════════════════════════════════════════════════════════════
    //  目标锁定指示器 — 在锁定的怪物下方绘制高亮圈
    // ═══════════════════════════════════════════════════════════════

    /** 目标指示器精灵 */
    var _targetIndicator = null;
    /** 指示器闪烁相位 */
    var _indicatorPhase = 0;

    /**
     * 创建目标锁定指示器精灵。
     * @returns {Sprite} 指示器精灵
     */
    function createTargetIndicator() {
        var sp = new Sprite(new Bitmap(56, 56));
        var bmp = sp.bitmap;
        var ctx = bmp._context;
        var cx = 28, cy = 28, r = 24;
        ctx.beginPath();
        ctx.arc(cx, cy, r, 0, Math.PI * 2);
        ctx.strokeStyle = 'rgba(255, 215, 0, 0.7)';
        ctx.lineWidth = 3;
        ctx.stroke();
        bmp._setDirty();
        sp.anchor.x = 0.5;
        sp.anchor.y = 0.5;
        sp.visible = false;
        return sp;
    }

    // 挂接 Spriteset_Map 创建目标指示器
    var _Spriteset_Map_createCharacters3 = Spriteset_Map.prototype.createCharacters;
    Spriteset_Map.prototype.createCharacters = function () {
        _Spriteset_Map_createCharacters3.call(this);
        _targetIndicator = createTargetIndicator();
        this._tilemap.addChild(_targetIndicator);
    };

    // 挂接 Spriteset_Map.update 更新指示器位置
    var _Spriteset_Map_update3 = Spriteset_Map.prototype.update;
    Spriteset_Map.prototype.update = function () {
        _Spriteset_Map_update3.call(this);
        _updateTargetIndicator();
        _updateCooldownHUD();
    };

    /**
     * 每帧更新目标锁定指示器。
     */
    function _updateTargetIndicator() {
        if (!_targetIndicator || !$gameMap) return;
        if (!_lockedTarget || !window.MonsterManager || !MonsterManager._sprites[_lockedTarget]) {
            _targetIndicator.visible = false;
            return;
        }
        var sp = MonsterManager._sprites[_lockedTarget];
        var tileW = $gameMap.tileWidth(), tileH = $gameMap.tileHeight();
        _targetIndicator.x = (sp._realX - $gameMap.displayX() + 0.5) * tileW;
        _targetIndicator.y = (sp._realY - $gameMap.displayY() + 1.0) * tileH;
        _targetIndicator.visible = true;

        // 脉冲动画
        _indicatorPhase = (_indicatorPhase + 2) % 360;
        _targetIndicator.opacity = 180 + Math.round(Math.sin(_indicatorPhase * Math.PI / 180) * 75);
    }

    // ═══════════════════════════════════════════════════════════════
    //  HUD — 攻击冷却条 + 自动攻击指示 + 战斗模式标签
    // ═══════════════════════════════════════════════════════════════

    /** 冷却条精灵 */
    var _cooldownBar = null;
    /** 战斗模式标签精灵 */
    var _modeLabel = null;
    /** 自动攻击标签精灵 */
    var _autoLabel = null;

    // 在 Scene_Map 创建 HUD 元素
    var _Scene_Map_createDisplayObjects = Scene_Map.prototype.createDisplayObjects;
    Scene_Map.prototype.createDisplayObjects = function () {
        _Scene_Map_createDisplayObjects.call(this);
        _createCombatHUD(this);
    };

    /**
     * 创建战斗 HUD 元素。
     * @param {Scene_Map} scene - 当前场景
     */
    function _createCombatHUD(scene) {
        // 冷却条（屏幕底部中央偏上）
        _cooldownBar = new Sprite(new Bitmap(120, 8));
        _cooldownBar.x = (Graphics.boxWidth - 120) / 2;
        _cooldownBar.y = Graphics.boxHeight - 80;
        _cooldownBar.visible = false;
        scene.addChild(_cooldownBar);

        // 战斗模式标签（左上角）
        _modeLabel = new Sprite(new Bitmap(160, 22));
        _modeLabel.x = 8;
        _modeLabel.y = 4;
        _drawModeLabel();
        scene.addChild(_modeLabel);

        // 自动攻击标签（战斗模式标签下方）
        _autoLabel = new Sprite(new Bitmap(120, 20));
        _autoLabel.x = 8;
        _autoLabel.y = 28;
        _autoLabel.visible = false;
        scene.addChild(_autoLabel);
    }

    /**
     * 绘制战斗模式标签。
     */
    function _drawModeLabel() {
        if (!_modeLabel) return;
        var bmp = _modeLabel.bitmap;
        bmp.clear();
        bmp.fontSize = 14;

        var modeText, color;
        switch (_combatMode) {
            case 'turnbased':
                modeText = 'TURN-BASED';
                color = '#6699FF';
                break;
            case 'realtime':
                modeText = 'REAL-TIME';
                color = '#FF6644';
                break;
            default:
                modeText = 'HYBRID';
                color = '#FFCC00';
                break;
        }
        bmp.textColor = color;
        bmp.drawText('[' + modeText + ']', 0, 0, 160, 22, 'left');
    }

    /**
     * 每帧更新冷却条和自动攻击标签。
     */
    function _updateCooldownHUD() {
        // 冷却条
        if (_cooldownBar) {
            if (_attackCooldown > 0 && $MMO.isFieldAttackEnabled()) {
                _cooldownBar.visible = true;
                var bmp = _cooldownBar.bitmap;
                bmp.clear();
                var ratio = _attackCooldown / _attackCooldownMax;
                // 背景
                bmp.fillRect(0, 0, 120, 8, 'rgba(0,0,0,0.6)');
                // 填充（从右到左消退）
                var fillW = Math.round(120 * ratio);
                bmp.fillRect(0, 0, fillW, 8, 'rgba(100,180,255,0.8)');
            } else {
                _cooldownBar.visible = false;
            }
        }

        // 自动攻击标签
        if (_autoLabel) {
            if (_autoAttack && $MMO.isFieldAttackEnabled()) {
                _autoLabel.visible = true;
                var abmp = _autoLabel.bitmap;
                abmp.clear();
                abmp.fontSize = 13;
                abmp.textColor = '#44FF44';
                abmp.drawText('AUTO-ATK [A]', 0, 0, 120, 20, 'left');
            } else {
                _autoLabel.visible = false;
            }
        }
    }

    // 切换地图时重绘模式标签
    $MMO.on('map_init', function () {
        setTimeout(function () { _drawModeLabel(); }, 100);
    });

    // ═══════════════════════════════════════════════════════════════
    //  realtime 模式下拦截 Scene_Battle
    //  当 combatMode 为 "realtime" 时，事件触发的战斗自动跳过
    // ═══════════════════════════════════════════════════════════════

    var _BattleManager_setup = BattleManager.setup;
    BattleManager.setup = function (troopId, canEscape, canLose) {
        _BattleManager_setup.call(this, troopId, canEscape, canLose);
        if (_combatMode === 'realtime' && !$MMO._serverBattle) {
            // 标记自动胜利（puppet mode では無効）
            this._realtimeAutoWin = true;
        }
    };

    var _BattleManager_update = BattleManager.update;
    BattleManager.update = function () {
        if (this._realtimeAutoWin) {
            // realtime 模式下跳过战斗，直接胜利
            this._realtimeAutoWin = false;
            this.processVictory();
            return;
        }
        _BattleManager_update.call(this);
    };

    // ═══════════════════════════════════════════════════════════════
    //  场景清理
    // ═══════════════════════════════════════════════════════════════

    var _Scene_Map_terminate3 = Scene_Map.prototype.terminate;
    Scene_Map.prototype.terminate = function () {
        _Scene_Map_terminate3.call(this);
        _lockedTarget = 0;
        _autoAttack = false;
        _targetIndicator = null;
        _cooldownBar = null;
        _modeLabel = null;
        _autoLabel = null;
    };

    // ═══════════════════════════════════════════════════════════════
    //  怪物死亡时清除锁定
    // ═══════════════════════════════════════════════════════════════

    $MMO.on('monster_death', function (data) {
        if (data.inst_id === _lockedTarget) {
            _lockedTarget = 0;
            if (_autoAttack) {
                // 自动切换到下一个目标
                setTimeout(function () {
                    _switchTarget();
                    if (!_lockedTarget) _autoAttack = false;
                }, 100);
            }
        }
    });

})();
