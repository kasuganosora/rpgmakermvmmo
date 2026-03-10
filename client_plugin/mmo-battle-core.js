/*:
 * @plugindesc v1.0.0 YEP_BattleEngineCore 兼容层 — puppet 模式下禁用本地 phase machine。
 * @author MMO Framework
 *
 * @help
 * 必须在 YEP_BattleEngineCore.js 之后、mmo-battle-puppet.js 之前加载。
 * 通过 mmo-loader.js 远程加载，自动在所有 YEP 插件之后执行。
 *
 * 仅在 puppet 战斗模式（$MMO._serverBattle === true）下激活：
 *  - 阻止 YEP 的 processTurn / startAction / updatePhase 触发本地计算
 *  - 解除 YEP Window_BattleLog.isBusy() / BattleManager.isBusy() 等待，
 *    确保 puppet 模式下 endBattle() 能正常关闭 Scene_Battle
 *
 * 不影响非 MMO 的正常 RMMV 战斗（$MMO._serverBattle === false 时行为不变）。
 */

(function () {
    'use strict';

    // ═══════════════════════════════════════════════════════════
    //  BattleManager — 阻止 YEP 本地 phase machine 在 puppet 模式下运行
    //
    //  mmo-battle-puppet.js 已完全替换 Scene_Battle.prototype.update，
    //  因此 BattleManager.update() 和内部 phase 驱动器在 puppet 模式下
    //  理论上不会被调用。以下覆写为双重保险，防止意外触发路径。
    // ═══════════════════════════════════════════════════════════

    /**
     * YEP 的 processTurn 设置 _processTurn=true 并驱动回合推进。
     * puppet 模式下回合由服务端控制，禁止本地执行。
     */
    var _YEP_BM_processTurn = BattleManager.processTurn;
    BattleManager.processTurn = function () {
        if ($MMO._serverBattle) return;
        _YEP_BM_processTurn.call(this);
    };

    /**
     * YEP 的 startAction 调用 applyGlobal()，会在本地执行伤害计算。
     * puppet 模式下所有伤害在服务端计算，禁止本地执行。
     */
    var _YEP_BM_startAction = BattleManager.startAction;
    BattleManager.startAction = function () {
        if ($MMO._serverBattle) return;
        _YEP_BM_startAction.call(this);
    };

    /**
     * YEP 的 updatePhase 是自定义阶段驱动器（_phaseSteps 队列），
     * puppet 模式下阶段由 _puppetUpdate() 控制，禁止 YEP 阶段驱动。
     */
    if (typeof BattleManager.updatePhase === 'function') {
        var _YEP_BM_updatePhase = BattleManager.updatePhase;
        BattleManager.updatePhase = function () {
            if ($MMO._serverBattle) return;
            _YEP_BM_updatePhase.call(this);
        };
    }

    // ═══════════════════════════════════════════════════════════
    //  Window_BattleLog — 解除 isBusy 等待
    //  YEP_BattleEngineCore 重写 Window_BattleLog.isBusy()，
    //  在 _waitCount > 0 时返回 true，导致 Scene_Battle.isBusy() 阻塞。
    //  puppet 模式下跳过 Window_BattleLog 流程，强制返回 false。
    // ═══════════════════════════════════════════════════════════

    var _WBL_isBusy = Window_BattleLog.prototype.isBusy;
    Window_BattleLog.prototype.isBusy = function () {
        if ($MMO._serverBattle) return false;
        return _WBL_isBusy.call(this);
    };

    // ═══════════════════════════════════════════════════════════
    //  BattleManager.isBusy — YEP 扩展版包含 _logWindow.isBusy() 检查
    //  puppet 模式下绕过，确保 endBattle() 能正常关闭场景。
    // ═══════════════════════════════════════════════════════════

    var _YEP_BM_isBusy = BattleManager.isBusy;
    BattleManager.isBusy = function () {
        if ($MMO._serverBattle) return false;
        return _YEP_BM_isBusy.call(this);
    };

    // ═══════════════════════════════════════════════════════════
    //  BaseTroopEvents 兼容
    //  YEP_BaseTroopEvents 将 base troop 的事件页合并到所有战斗中。
    //  服务端已在 initTroopEvents 中处理 baseTroopId 合并，
    //  客户端只需确保本地 $gameTroop 不重复执行 base troop 事件。
    //
    //  YEP_BaseTroopEvents 通过 Game_Troop.setup() 注入额外 pages，
    //  puppet 模式下 $gameTroop 不执行战斗事件（由服务端执行），
    //  无需额外处理 — 记录于此说明原因。
    // ═══════════════════════════════════════════════════════════

    // ═══════════════════════════════════════════════════════════
    //  YEP_X_VisualHpGauge 兼容
    //  YEP HP 条通过 requestEffect('whiten') 触发显示。
    //  puppet 模式下 battle_action 消息处理后，enemy battler HP 已更新，
    //  HP 条会在下一帧 update() 中自动刷新 — 无需额外处理。
    // ═══════════════════════════════════════════════════════════

})();
