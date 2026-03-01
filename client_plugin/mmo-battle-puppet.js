/*:
 * @plugindesc v4.3.0 MMO Battle Puppet Mode - server-authoritative turn-based combat.
 * @author MMO Framework
 *
 * @help
 * This plugin intercepts RMMV's battle system to operate in "puppet mode":
 * - Server controls all battle logic (damage, AI, states, rewards)
 * - Client renders animations via direct sprite calls (not Window_BattleLog,
 *   which YEP_BattleEngineCore empties)
 * - Player inputs are sent to the server instead of processed locally
 *
 * v4.0 — bypasses Window_BattleLog entirely for animation; uses direct
 *         target.startAnimation() + update loop isAnimationPlaying() timing.
 *         Syncs actor stats (MHP/MMP/ATK/...) from server snapshots.
 */

(function () {
    'use strict';

    // =================================================================
    //  Puppet mode state
    // =================================================================
    var _puppetMode = false;
    var _puppetActors = [];
    var _puppetEnemies = [];
    var _pendingInputRequest = null;
    var _puppetSceneReady = false;
    var _puppetReadyFrames = 0;

    // Animation pipeline state
    var _puppetEventQueue = [];
    var _processingAction = false;
    var _puppetActionWait = 0;
    var _puppetDamageApplied = false;
    var _puppetTargetData = [];   // current action's target data
    var _puppetEndingBattle = false;

    // Target selection state
    var _puppetPendingSkillId = 0;
    var _puppetPendingItemId = 0;

    $MMO._serverBattle = false;

    // =================================================================
    //  CallStand.js compatibility — ensure storage properties exist
    // =================================================================
    // CallStand.js unconditionally accesses $gameActors.actor(2).toneArray[armorId]
    // and .toneWeapon[weaponId] in its pluginCommand chain for EVERY plugin command.
    // These properties are initialized by CE 2 (parallel autorun on the map), but
    // CE 2 doesn't run during battle or immediately after battle when NPC events
    // resume. Hook Game_Actors.actor() to lazily ensure these properties exist.
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

    // =================================================================
    //  Param override — force actor stats from server during puppet mode
    // =================================================================

    var _GBB_param = Game_BattlerBase.prototype.param;
    Game_BattlerBase.prototype.param = function (paramId) {
        if (_puppetMode && this._puppetParams && this._puppetParams[paramId] !== undefined) {
            return this._puppetParams[paramId];
        }
        return _GBB_param.call(this, paramId);
    };

    // =================================================================
    //  Server event handlers
    // =================================================================

    $MMO.on('battle_battle_start', function (data) {
        if (!data) return;

        console.log('[Puppet] Battle start, actors=' +
            (data.actors ? data.actors.length : 0) +
            ' enemies=' + (data.enemies ? data.enemies.length : 0));

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

        if (!$dataTroops || !$dataTroops[1]) {
            console.warn('[Puppet] No troop data available');
            return;
        }

        if (!$gameParty._actors) $gameParty._actors = [];
        for (var j = 0; j < _puppetActors.length; j++) {
            var actorData = _puppetActors[j];
            var actorId = j + 1;
            var actor = $gameActors.actor(actorId);
            if (actor) {
                actor._hp = actorData.hp;
                actor._mp = actorData.mp;
                actor._tp = actorData.tp || 0;
                // Override params with server-computed values.
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

        BattleManager.setup(1, true, true);

        var troop = $gameTroop;
        if (troop && troop._enemies) {
            troop._enemies = [];
            var defaultPositions = [
                [408, 340], [460, 380], [356, 380], [500, 310],
                [312, 310], [540, 350], [270, 350], [408, 280]
            ];
            for (var i = 0; i < _puppetEnemies.length; i++) {
                var enemyData = _puppetEnemies[i];
                var enemyId = enemyData.enemy_id || 1;
                if (!$dataEnemies[enemyId] || !$dataEnemies[enemyId].battlerName) {
                    console.warn('[Puppet] Enemy ID ' + enemyId + ' has no sprite, skipping');
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

        var _origBB1 = $gameMap.battleback1Name;
        var _origBB2 = $gameMap.battleback2Name;
        $gameMap.battleback1Name = function () { return ''; };
        $gameMap.battleback2Name = function () { return ''; };

        BattleManager.setEventCallback(function (result) {
            console.log('[Puppet] Battle scene closed, result=' + result);
            $gameMap.battleback1Name = _origBB1;
            $gameMap.battleback2Name = _origBB2;
        });
        $gamePlayer.makeEncounterCount();
        SceneManager.push(Scene_Battle);
    });

    $MMO.on('battle_input_request', function (data) {
        if (!_puppetMode) return;
        console.log('[Puppet] Input request for actor index=' + data.actor_index);
        _pendingInputRequest = data;
    });

    $MMO.on('battle_turn_start', function (data) {
        if (!_puppetMode) return;
        console.log('[Puppet] Turn start, turn=' + (data ? data.turn_count : '?'));
    });

    $MMO.on('battle_action_result', function (data) {
        if (!_puppetMode || !data) return;
        console.log('[Puppet] Action result queued: subject=' +
            (data.subject ? data.subject.name : '?') +
            ' skill=' + data.skill_id +
            ' item=' + (data.item_id || 0) +
            ' targets=' + (data.targets ? data.targets.length : 0));
        _puppetEventQueue.push({ type: 'action', data: data });
    });

    $MMO.on('battle_turn_end', function (data) {
        if (!_puppetMode) return;
        console.log('[Puppet] Turn end queued');
        _puppetEventQueue.push({ type: 'turn_end', data: data || {} });
    });

    $MMO.on('battle_battle_end', function (data) {
        if (!_puppetMode || !data) return;
        console.log('[Puppet] Battle end, result=' + data.result);

        _pendingInputRequest = null;
        _puppetEventQueue = [];
        _processingAction = false;

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

        // Clean up puppet param overrides.
        _clearPuppetParams();

        _puppetEndingBattle = true;
        _puppetMode = false;
        $MMO._serverBattle = false;

        if (data.result === 0) {
            BattleManager.processVictory();
        } else if (data.result === 1) {
            BattleManager.processEscape();
        } else {
            BattleManager.processDefeat();
        }

        _puppetEndingBattle = false;
        $MMO.send('npc_battle_result', { result: data.result });
    });

    function _clearPuppetParams() {
        for (var a = 1; a <= 20; a++) {
            var actor = $gameActors.actor(a);
            if (actor) delete actor._puppetParams;
        }
    }

    // =================================================================
    //  Puppet party management
    // =================================================================

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

    // =================================================================
    //  Animation pipeline — direct sprite calls, bypasses Window_BattleLog
    // =================================================================

    function _startPuppetAction(data) {
        _processingAction = true;
        _puppetActionWait = 0;
        _puppetDamageApplied = false;
        _puppetTargetData = data.targets || [];

        var subject = _getBattler(data.subject);
        if (!subject) {
            console.warn('[Puppet] Subject not found, skipping');
            _processingAction = false;
            return;
        }

        // Create Game_Action for animation ID lookup.
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
        console.log('[Puppet] >> Action: ' + subject.name() +
            ' skill=' + skillId + ' animId=' + animId +
            ' targets=' + _puppetTargetData.length);

        BattleManager._subject = subject;
        BattleManager._action = action;
        BattleManager._phase = 'puppetAction';

        // Resolve target battler objects.
        var targets = [];
        for (var i = 0; i < _puppetTargetData.length; i++) {
            var tgt = _getBattler(_puppetTargetData[i].target);
            if (tgt) targets.push(tgt);
        }

        // --- Start animation directly on targets ---
        // (bypasses Window_BattleLog which YEP BEC empties)

        if (animId < 0) {
            // animationId = -1 means "use attack animation"
            _showAttackAnimation(subject, targets);
        } else if (animId > 0) {
            // Named skill/item animation: play on all targets
            for (var t = 0; t < targets.length; t++) {
                targets[t].startAnimation(animId, false, 0);
            }
        }
        // animId === 0: no animation (e.g., buff-only skills like 変身)

        // If no animation and no damage, apply results immediately.
        if (animId === 0 && !_hasVisibleDamage(_puppetTargetData)) {
            _applyAllTargetResults(_puppetTargetData);
            _puppetDamageApplied = true;
        }

        // Subject visual feedback (enemy flash/shake for acting)
        if (subject.isEnemy && subject.isEnemy()) {
            var scene = SceneManager._scene;
            if (scene && scene._spriteset) {
                var sprites = scene._spriteset.battlerSprites ?
                    scene._spriteset.battlerSprites() : [];
                for (var s = 0; s < sprites.length; s++) {
                    if (sprites[s]._battler === subject) {
                        // White flash to show enemy is acting
                        sprites[s]._effectType = 'whiten';
                        sprites[s]._effectDuration = 16;
                        break;
                    }
                }
            }
        }
    }

    // Show attack animation: handles both actors (weapon anim) and enemies
    function _showAttackAnimation(subject, targets) {
        if (targets.length === 0) return;

        var attackAnimId = 1; // Default normal attack animation

        if (subject.attackAnimationId1) {
            attackAnimId = subject.attackAnimationId1();
        }
        // If attackAnimationId1 returned 0 (no animation defined), use default 1
        if (!attackAnimId || attackAnimId <= 0) {
            attackAnimId = 1;
        }

        if (attackAnimId > 0) {
            for (var t = 0; t < targets.length; t++) {
                targets[t].startAnimation(attackAnimId, false, 0);
            }
        }

        console.log('[Puppet] Attack animation ID=' + attackAnimId +
            ' on ' + targets.length + ' targets');
    }

    function _hasVisibleDamage(targetDataList) {
        for (var i = 0; i < targetDataList.length; i++) {
            if (targetDataList[i].damage !== 0 || targetDataList[i].missed) return true;
        }
        return false;
    }

    // Apply all target results (damage popups, state changes, etc.)
    function _applyAllTargetResults(targetDataList) {
        for (var i = 0; i < targetDataList.length; i++) {
            _applyTargetResult(targetDataList[i]);
        }
        // Refresh status window
        var scene = SceneManager._scene;
        if (scene && scene instanceof Scene_Battle && scene._statusWindow) {
            scene._statusWindow.refresh();
        }
    }

    // Apply a single target's result (damage, states, etc.)
    function _applyTargetResult(tgtData) {
        var battler = _getBattler(tgtData.target);
        if (!battler) return;

        var result = battler.result();
        result.clear();
        result.used = true;

        if (tgtData.missed) {
            result.missed = true;
            result.evaded = true;
        } else {
            if (tgtData.damage !== 0) {
                result.hpDamage = tgtData.damage;
                result.hpAffected = true;
            }
            result.critical = tgtData.critical || false;

            if (tgtData.hp_after !== undefined) battler._hp = tgtData.hp_after;
            if (tgtData.mp_after !== undefined) battler._mp = tgtData.mp_after;

            if (tgtData.added_states) {
                for (var s = 0; s < tgtData.added_states.length; s++) {
                    var stateId = tgtData.added_states[s];
                    battler.addState(stateId);
                    if (result.addedStates) result.addedStates.push(stateId);
                }
            }
            if (tgtData.removed_states) {
                for (var r = 0; r < tgtData.removed_states.length; r++) {
                    battler.removeState(tgtData.removed_states[r]);
                    if (result.removedStates) result.removedStates.push(tgtData.removed_states[r]);
                }
            }
        }

        // Show damage popup
        battler.startDamagePopup();

        // Handle death
        if (battler.isDead && battler.isDead()) {
            battler.performCollapse();
        }

        // Execute common events triggered by skill effects.
        if (tgtData.common_event_ids) {
            for (var ce = 0; ce < tgtData.common_event_ids.length; ce++) {
                var ceId = tgtData.common_event_ids[ce];
                console.log('[Puppet] Executing common event ' + ceId);
                _executeCommonEvent(ceId);
            }
        }
    }

    // Temporarily enable/disable _serverSync gates on game objects.
    // During puppet battle CE execution, switches/variables/items need to
    // pass through the _serverSync gates in mmo-npc.js so that common events
    // (e.g., CE 1031 transformation) can modify game state client-side.
    function _enableServerSync(enable) {
        if ($gameParty) $gameParty._serverSync = enable;
        if ($gameSwitches) $gameSwitches._serverSync = enable;
        if ($gameVariables) $gameVariables._serverSync = enable;
        if ($gameSelfSwitches) $gameSelfSwitches._serverSync = enable;
    }

    // Skip past a failed command in the interpreter chain. If the error
    // occurred in a child interpreter (e.g., CE 891 called from CE 1031),
    // clear the child so the parent can resume. Otherwise advance the
    // top-level interpreter past the failing command.
    function _skipFailedInterpreterCommand(interp) {
        if (!interp) return;
        // If a child interpreter is active, the error likely came from there.
        // Clear the child to let the parent continue with its next command.
        if (interp._childInterpreter && interp._childInterpreter.isRunning()) {
            interp._childInterpreter.clear();
            interp._childInterpreter = null;
            return;
        }
        // Otherwise advance past the failing command in this interpreter.
        if (interp._list && interp._index < interp._list.length) {
            interp._index++;
        } else {
            interp.clear();
        }
    }

    // Execute a common event during puppet battle.
    // Sets up the event on the troop interpreter; the update loop ticks it.
    function _executeCommonEvent(ceId) {
        if (!$dataCommonEvents || !$dataCommonEvents[ceId]) {
            console.warn('[Puppet] Common event ' + ceId + ' not found');
            return;
        }
        var ce = $dataCommonEvents[ceId];
        if (!ce || !ce.list || ce.list.length === 0) return;

        // Set up on the troop interpreter directly.
        // In puppet mode, setupBattleEvent is blocked, but we want CEs to run.
        if ($gameTroop && $gameTroop._interpreter) {
            $gameTroop._interpreter.setup(ce.list);
            console.log('[Puppet] Common event ' + ceId + ' set up on interpreter (' +
                ce.list.length + ' commands)');
        }
    }

    // Apply turn-end regen data.
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

    // Finish current puppet action, clean up BattleManager state.
    function _finishPuppetAction() {
        console.log('[Puppet] Action finished after ' + _puppetActionWait +
            ' frames, queue=' + _puppetEventQueue.length);
        var subj = BattleManager._subject;
        if (subj && subj.performActionEnd) subj.performActionEnd();
        BattleManager._subject = null;
        BattleManager._action = null;
        BattleManager._phase = 'waiting';
        _processingAction = false;
        _puppetDamageApplied = false;
        _puppetTargetData = [];
    }

    // =================================================================
    //  Input request processing
    // =================================================================

    function _processInputRequest(data) {
        _pendingInputRequest = null;
        console.log('[Puppet] Processing input request for actor index=' + data.actor_index);

        _ensurePuppetParty();

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
                console.log('[Puppet] Command window activated for ' + actor.name());
            } else {
                console.warn('[Puppet] No actor for index=' + data.actor_index);
            }
        } else {
            console.warn('[Puppet] Scene not ready, re-queuing input request');
            _pendingInputRequest = data;
        }
    }

    // =================================================================
    //  Scene_Battle lifecycle hooks
    // =================================================================

    var _Scene_Battle_start = Scene_Battle.prototype.start;
    Scene_Battle.prototype.start = function () {
        _Scene_Battle_start.call(this);
        if (_puppetMode) {
            _puppetReadyFrames = 0;
            console.log('[Puppet] Scene_Battle.start(), waiting for settle...');
        }
    };

    // Main update hook — drives the puppet event queue and animation timing.
    var _Scene_Battle_update = Scene_Battle.prototype.update;
    Scene_Battle.prototype.update = function () {
        if (!_puppetMode) {
            _Scene_Battle_update.call(this);
            return;
        }

        // In puppet mode, skip the original Scene_Battle.update entirely.
        // It calls BattleManager.update() → $gameTroop.updateInterpreter()
        // which ticks the interpreter without error handling AND runs native
        // battle phase logic that conflicts with puppet control.
        // Instead, call only the rendering/UI parts we need.
        Scene_Base.prototype.update.call(this);  // children, input, fading
        if ($gameScreen) $gameScreen.update();
        if ($gameTimer) $gameTimer.update(this.isActive());
        this.updateStatusWindow();
        this.updateWindowPositions();

        // Wait for scene to fully initialize.
        if (!_puppetSceneReady) {
            _puppetReadyFrames++;
            if (_puppetReadyFrames >= 15 && this._actorCommandWindow) {
                _puppetSceneReady = true;
                console.log('[Puppet] Scene ready after ' + _puppetReadyFrames + ' frames');
            }
            return;
        }

        // --- Tick troop interpreter (for common events triggered by skills) ---
        if ($gameTroop && $gameTroop._interpreter && $gameTroop._interpreter.isRunning()) {
            // Temporarily allow state changes while executing CEs in puppet battle.
            // The _serverSync gates in mmo-npc.js block switches/variables/items,
            // but CEs like 1031 (transformation) need them to function correctly.
            _enableServerSync(true);
            try {
                $gameTroop._interpreter.update();
            } catch (e) {
                // ProjectB plugin commands may depend on map-context state (e.g.,
                // toneArray initialized by parallel CE 2) that doesn't exist
                // during battle. Skip the failing command and continue the CE
                // so that subsequent commands (equipment, class change, etc.)
                // can still execute.
                console.warn('[Puppet] CE command error (skipping):', e.message);
                _skipFailedInterpreterCommand($gameTroop._interpreter);
            }
            _enableServerSync(false);
            // Wait for interpreter to finish before processing next event.
            return;
        }

        // --- Animation timing loop ---
        if (_processingAction) {
            _puppetActionWait++;
            var animPlaying = this._spriteset && this._spriteset.isAnimationPlaying();

            // Apply damage ~12 frames in (when the "hit" lands visually)
            if (!_puppetDamageApplied && _puppetActionWait >= 12) {
                _applyAllTargetResults(_puppetTargetData);
                _puppetDamageApplied = true;
            }

            // Finish when: (animation done AND minimum 30 frames) OR safety timeout 180 frames
            if ((!animPlaying && _puppetActionWait >= 30) || _puppetActionWait >= 180) {
                if (_puppetActionWait >= 180) {
                    console.warn('[Puppet] Action timeout after 180 frames');
                }
                // Apply damage if somehow still pending
                if (!_puppetDamageApplied) {
                    _applyAllTargetResults(_puppetTargetData);
                    _puppetDamageApplied = true;
                }
                _finishPuppetAction();
            }
            return;
        }

        // --- Process queued events ---
        if (_puppetEventQueue.length > 0) {
            var evt = _puppetEventQueue.shift();
            console.log('[Puppet] Dequeuing event: ' + evt.type + ', remaining=' + _puppetEventQueue.length);
            if (evt.type === 'action') {
                _startPuppetAction(evt.data);
            } else if (evt.type === 'turn_end') {
                _applyTurnEnd(evt.data);
            }
            return;
        }

        // --- Process pending input request (only after queue fully drained) ---
        if (_pendingInputRequest && this._actorCommandWindow) {
            _processInputRequest(_pendingInputRequest);
        }
    };

    // =================================================================
    //  BattleManager hooks — block all local battle logic in puppet mode
    // =================================================================

    var _BM_startInput = BattleManager.startInput;
    BattleManager.startInput = function () {
        if (_puppetMode) {
            this._phase = 'waiting';
            return;
        }
        _BM_startInput.call(this);
    };

    var _BM_startTurn = BattleManager.startTurn;
    BattleManager.startTurn = function () {
        if (_puppetMode) return;
        _BM_startTurn.call(this);
    };

    var _BM_updateAction = BattleManager.updateAction;
    BattleManager.updateAction = function () {
        if (_puppetMode) return;
        _BM_updateAction.call(this);
    };

    var _BM_endAction = BattleManager.endAction;
    BattleManager.endAction = function () {
        if (_puppetMode) return;
        _BM_endAction.call(this);
    };

    var _BM_invokeNormalAction = BattleManager.invokeNormalAction;
    BattleManager.invokeNormalAction = function (subject, target) {
        if (_puppetMode) return;
        _BM_invokeNormalAction.call(this, subject, target);
    };

    var _GT_setupBattleEvent = Game_Troop.prototype.setupBattleEvent;
    Game_Troop.prototype.setupBattleEvent = function () {
        if (_puppetMode) return;
        _GT_setupBattleEvent.call(this);
    };

    var _BM_checkBattleEnd = BattleManager.checkBattleEnd;
    BattleManager.checkBattleEnd = function () {
        if (_puppetMode) return false;
        return _BM_checkBattleEnd.call(this);
    };

    var _BM_checkAbort = BattleManager.checkAbort;
    BattleManager.checkAbort = function () {
        if (_puppetMode) return false;
        return _BM_checkAbort.call(this);
    };

    var _BM_processVictory = BattleManager.processVictory;
    BattleManager.processVictory = function () {
        if (_puppetMode && !_puppetEndingBattle) return;
        _BM_processVictory.call(this);
    };

    var _BM_processDefeat = BattleManager.processDefeat;
    BattleManager.processDefeat = function () {
        if (_puppetMode && !_puppetEndingBattle) return;
        _BM_processDefeat.call(this);
    };

    var _BM_processEscape = BattleManager.processEscape;
    BattleManager.processEscape = function () {
        if (_puppetMode && !_puppetEndingBattle) return;
        return _BM_processEscape.call(this);
    };

    // =================================================================
    //  Interpreter error safety — catch plugin command errors at source
    // =================================================================
    // In puppet mode, CEs triggered by skills (e.g., CE 1031 transformation)
    // may call plugin commands that depend on map-context state (toneArray,
    // parallax data, etc.) which doesn't exist during battle. Wrapping
    // command356 catches errors at the innermost level before they propagate
    // up to SceneManager.catchException and freeze the battle.
    var _GI_command356 = Game_Interpreter.prototype.command356;
    Game_Interpreter.prototype.command356 = function () {
        if (_puppetMode) {
            try {
                return _GI_command356.call(this);
            } catch (e) {
                console.warn('[Puppet] Plugin command error (skipping): ' + e.message);
                return true; // Continue to next command
            }
        }
        return _GI_command356.call(this);
    };

    // =================================================================
    //  Scene_Battle input hooks
    // =================================================================

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

    var _Scene_Battle_commandSkill = Scene_Battle.prototype.commandSkill;
    Scene_Battle.prototype.commandSkill = function () {
        _Scene_Battle_commandSkill.call(this);
    };

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

    var _Scene_Battle_commandItem = Scene_Battle.prototype.commandItem;
    Scene_Battle.prototype.commandItem = function () {
        _Scene_Battle_commandItem.call(this);
    };

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
                $MMO.send('battle_input', {
                    actor_index: BattleManager._actorIndex,
                    action_type: 1,
                    skill_id: skill.id,
                    target_indices: [BattleManager._actorIndex],
                    target_is_actor: true,
                });
                BattleManager._phase = 'waiting';
            } else if (action.isForOpponent()) {
                _puppetPendingSkillId = skill.id;
                _puppetPendingItemId = 0;
                _selectEnemyTarget.call(this);
            } else {
                _puppetPendingSkillId = skill.id;
                _puppetPendingItemId = 0;
                _selectActorTarget.call(this);
            }
            return;
        }
        _Scene_Battle_onSkillOk.call(this);
    };

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
                $MMO.send('battle_input', {
                    actor_index: BattleManager._actorIndex,
                    action_type: 2,
                    item_id: item.id,
                    target_indices: [BattleManager._actorIndex],
                    target_is_actor: true,
                });
                BattleManager._phase = 'waiting';
            } else if (action.isForOpponent()) {
                _puppetPendingSkillId = 0;
                _puppetPendingItemId = item.id;
                _selectEnemyTarget.call(this);
            } else {
                _puppetPendingSkillId = 0;
                _puppetPendingItemId = item.id;
                _selectActorTarget.call(this);
            }
            return;
        }
        _Scene_Battle_onItemOk.call(this);
    };

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

    // =================================================================
    //  Helpers
    // =================================================================

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

    function _selectEnemyTarget() {
        if (this._enemyWindow) {
            this._enemyWindow.refresh();
            this._enemyWindow.show();
            this._enemyWindow.select(0);
            this._enemyWindow.activate();
        }
    }

    function _selectActorTarget() {
        if (this._actorWindow) {
            this._actorWindow.refresh();
            this._actorWindow.show();
            this._actorWindow.select(0);
            this._actorWindow.activate();
        }
    }

})();
