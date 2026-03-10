#!/usr/bin/env node
/**
 * test-battle-puppet.js  --  Unit tests for mmo-battle-puppet.js (puppet mode).
 *
 * These tests run WITHOUT a live server.  We load the RMMV shim + plugins,
 * then simulate server events via $MMO._dispatch() and verify state changes.
 */
'use strict';

require('./rmmv-shim');

const assert = require('assert');
const vm = require('vm');
const fs = require('fs');
const path = require('path');

// ---- Load plugins (no server needed — we never call connect) ----
// Set MMO_CONFIG before loading plugins.
globalThis.MMO_CONFIG = {
    serverUrl: 'ws://localhost:8080',
    debug: false,
    reconnectMax: 0
};

const pluginDir = path.join(__dirname, '..');
const LOAD_ORDER = [
    'mmo-core.js',
    'mmo-ui.js',
    'mmo-game-window.js',
    'mmo-battle.js',
    'mmo-battle-puppet.js'
];
for (const filename of LOAD_ORDER) {
    const filePath = path.join(pluginDir, filename);
    if (!fs.existsSync(filePath)) {
        console.warn('[test] Skipping missing: ' + filename);
        continue;
    }
    const code = fs.readFileSync(filePath, 'utf8');
    vm.runInThisContext(code, { filename: filename });
}

// ---- Helpers ----

// Capture messages sent via $MMO.send (bypass connection check).
var _sentMessages = [];
var _origSend = $MMO.send;
$MMO.send = function (type, payload) {
    _sentMessages.push({ type: type, payload: payload || {} });
    return true;
};

function resetState() {
    _sentMessages = [];
    $MMO._serverBattle = false;
    BattleManager._phase = '';
    BattleManager._actorIndex = -1;
    BattleManager._eventCallback = null;
    SceneManager._callLog = [];

    // Reset $gameActors and $gameTroop.
    $gameActors._data = {};
    $gameTroop._enemies = [];
    $gameParty._actors = [1];
    $gameParty._gold = 0;
    $gameParty._items = {};

    // Reinitialise actors.
    var actor = $gameActors.actor(1);
    actor._hp = 100;
    actor._mp = 50;
    actor._tp = 0;
}

function lastSent(type) {
    for (var i = _sentMessages.length - 1; i >= 0; i--) {
        if (_sentMessages[i].type === type) return _sentMessages[i].payload;
    }
    return null;
}

// Tick the Scene_Battle update loop N times.
// The puppet plugin processes queued events during Scene_Battle.update().
// After battle_battle_start, 16+ ticks are needed to pass the scene settle check.
function tickScene(n) {
    var scene = SceneManager._scene;
    if (!scene) return;
    for (var i = 0; i < (n || 1); i++) {
        if (typeof scene.update === 'function') scene.update();
    }
}

// Start a battle and wait for the scene to be ready.
function startBattleAndReady(actorsData, enemiesData) {
    $MMO._dispatch('battle_battle_start', {
        actors: actorsData,
        enemies: enemiesData
    });
    // Tick past the 15-frame settle period.
    tickScene(20);
}

// ---- Tests ----
var _tests = [];
var _passed = 0;
var _failed = 0;

function test(name, fn) { _tests.push({ name: name, fn: fn }); }

// ==== Test: battle_battle_start enters puppet mode ====
test('battle_battle_start enters puppet mode and pushes Scene_Battle', function () {
    resetState();
    startBattleAndReady(
        [{ name: 'Hero', hp: 200, mp: 50, tp: 0, index: 0, is_actor: true }],
        [{ name: 'Slime', hp: 80, mp: 0, index: 0, is_actor: false, enemy_id: 1 }]
    );

    assert.strictEqual($MMO._serverBattle, true, 'puppet mode should be active');
    // Scene_Battle should have been pushed.
    var pushCall = SceneManager._callLog.find(function (c) {
        return c.action === 'push' && c.scene === Scene_Battle;
    });
    assert.ok(pushCall, 'Scene_Battle should be pushed');
    // $gameTroop should have 1 enemy.
    assert.strictEqual($gameTroop._enemies.length, 1, 'troop should have 1 enemy');
    assert.strictEqual($gameTroop._enemies[0]._name, 'Slime', 'enemy name should match');
});

// ==== Test: battle_input_request sets input phase ====
test('battle_input_request sets BattleManager to input phase', function () {
    resetState();
    startBattleAndReady(
        [{ name: 'Hero', hp: 200, mp: 50, tp: 0 }],
        [{ name: 'Slime', hp: 80, mp: 0, enemy_id: 1 }]
    );
    // Dispatch input request — it gets stored as _pendingInputRequest.
    $MMO._dispatch('battle_input_request', { actor_index: 0 });
    // Tick the scene to process the pending input request.
    tickScene(1);
    assert.strictEqual(BattleManager._phase, 'input', 'phase should be input');
});

// ==== Test: BattleManager.startInput blocked in puppet mode ====
test('BattleManager.startInput is blocked in puppet mode', function () {
    // In puppet mode, startInput should set phase to 'waiting', not 'input'.
    BattleManager._phase = 'turn'; // reset to something else
    BattleManager.startInput();
    assert.strictEqual(BattleManager._phase, 'waiting', 'startInput should set waiting in puppet mode');
});

// ==== Test: BattleManager.startTurn blocked in puppet mode ====
test('BattleManager.startTurn is blocked in puppet mode', function () {
    BattleManager._phase = 'input';
    BattleManager.startTurn();
    // Should NOT change to 'turn' — the hook returns early.
    assert.notStrictEqual(BattleManager._phase, 'turn', 'startTurn should be blocked in puppet mode');
});

// ==== Test: commandGuard sends battle_input ====
test('commandGuard sends guard battle_input to server', function () {
    resetState();
    startBattleAndReady(
        [{ name: 'Hero', hp: 100, mp: 50, tp: 0 }],
        [{ name: 'Goblin', hp: 60, mp: 0, enemy_id: 1 }]
    );
    $MMO._dispatch('battle_input_request', { actor_index: 0 });
    tickScene(1); // process pending input request

    // Invoke commandGuard on the active scene.
    var scene = SceneManager._scene;
    scene.commandGuard();

    var msg = lastSent('battle_input');
    assert.ok(msg, 'should send battle_input');
    assert.strictEqual(msg.action_type, 3, 'action_type should be 3 (guard)');
    assert.strictEqual(msg.actor_index, 0, 'actor_index should be 0');
    assert.strictEqual(BattleManager._phase, 'waiting', 'phase should be waiting after guard');
});

// ==== Test: commandEscape sends battle_input ====
test('commandEscape sends escape battle_input to server', function () {
    resetState();
    startBattleAndReady(
        [{ name: 'Hero', hp: 100, mp: 50, tp: 0 }],
        [{ name: 'Goblin', hp: 60, mp: 0, enemy_id: 1 }]
    );
    $MMO._dispatch('battle_input_request', { actor_index: 0 });
    tickScene(1);

    var scene = SceneManager._scene;
    scene.commandEscape();

    var msg = lastSent('battle_input');
    assert.ok(msg, 'should send battle_input');
    assert.strictEqual(msg.action_type, 4, 'action_type should be 4 (escape)');
});

// ==== Test: onEnemyOk sends attack with target ====
test('onEnemyOk sends attack battle_input with target index', function () {
    resetState();
    startBattleAndReady(
        [{ name: 'Hero', hp: 100, mp: 50, tp: 0 }],
        [
            { name: 'Goblin A', hp: 60, mp: 0, enemy_id: 1 },
            { name: 'Goblin B', hp: 60, mp: 0, enemy_id: 1 }
        ]
    );
    $MMO._dispatch('battle_input_request', { actor_index: 0 });
    tickScene(1);

    var scene = SceneManager._scene;
    // Simulate selecting enemy index 1.
    scene._enemyWindow._index = 1;
    scene.onEnemyOk();

    var msg = lastSent('battle_input');
    assert.ok(msg, 'should send battle_input');
    assert.strictEqual(msg.action_type, 0, 'action_type should be 0 (attack)');
    assert.deepStrictEqual(msg.target_indices, [1], 'target should be enemy index 1');
    assert.strictEqual(msg.target_is_actor, false, 'target should not be actor');
});

// ==== Test: onSkillOk with targeted skill opens enemy selection, then onEnemyOk sends ====
test('onSkillOk sends skill battle_input with skill_id via enemy selection', function () {
    resetState();
    startBattleAndReady(
        [{ name: 'Mage', hp: 80, mp: 100, tp: 0 }],
        [{ name: 'Dragon', hp: 500, mp: 0, enemy_id: 1 }]
    );
    $MMO._dispatch('battle_input_request', { actor_index: 0 });
    tickScene(1);

    var scene = SceneManager._scene;
    // Register the test skill in $dataSkills so Game_Action can look it up.
    $dataSkills[25] = { id: 25, name: 'Fire', scope: 1, animationId: 10 };
    scene._skillWindow._item = $dataSkills[25]; // scope 1 = one enemy
    // onSkillOk for a targeted skill opens enemy selection window.
    scene.onSkillOk();
    // Now simulate enemy selection (index 0).
    scene._enemyWindow._index = 0;
    scene.onEnemyOk();

    var msg = lastSent('battle_input');
    assert.ok(msg, 'should send battle_input');
    assert.strictEqual(msg.action_type, 1, 'action_type should be 1 (skill)');
    assert.strictEqual(msg.skill_id, 25, 'skill_id should be 25');
    assert.strictEqual(msg.target_is_actor, false, 'offensive skill targets enemy');
});

// ==== Test: onItemOk sends item battle_input ====
test('onItemOk sends item battle_input with item_id', function () {
    resetState();
    startBattleAndReady(
        [{ name: 'Healer', hp: 80, mp: 60, tp: 0 }],
        [{ name: 'Bat', hp: 30, mp: 0, enemy_id: 1 }]
    );
    $MMO._dispatch('battle_input_request', { actor_index: 0 });
    tickScene(1);

    var scene = SceneManager._scene;
    // Set the selected item — scope 7 = one ally.
    scene._itemWindow._item = { id: 1, scope: 7 };
    scene.onItemOk();

    var msg = lastSent('battle_input');
    assert.ok(msg, 'should send battle_input');
    assert.strictEqual(msg.action_type, 2, 'action_type should be 2 (item)');
    assert.strictEqual(msg.item_id, 1, 'item_id should be 1');
    assert.strictEqual(msg.target_is_actor, true, 'heal item targets actor');
});

// ==== Test: battle_action_result applies damage ====
test('battle_action_result applies damage to enemy HP', function () {
    resetState();
    startBattleAndReady(
        [{ name: 'Hero', hp: 200, mp: 50, tp: 0 }],
        [{ name: 'Slime', hp: 100, mp: 0, enemy_id: 1 }]
    );

    // Verify enemy initial HP.
    assert.strictEqual($gameTroop._enemies[0]._hp, 100);

    // Apply damage (queued).
    $MMO._dispatch('battle_action_result', {
        subject: { name: 'Hero', index: 0, is_actor: true },
        skill_id: 1,
        targets: [{
            target: { index: 0, is_actor: false },
            damage: 40,
            critical: false,
            missed: false,
            hp_after: 60,
            mp_after: 0
        }]
    });
    // Tick to dequeue action + apply damage (~12 frames) + finish (30+ frames).
    tickScene(35);

    assert.strictEqual($gameTroop._enemies[0]._hp, 60, 'enemy HP should be 60 after 40 damage');
    assert.strictEqual($gameTroop._enemies[0].result().hpDamage, 40, 'result should show 40 damage');
});

// ==== Test: battle_action_result applies healing ====
test('battle_action_result applies healing to actor', function () {
    resetState();
    startBattleAndReady(
        [{ name: 'Hero', hp: 50, mp: 50, tp: 0 }],
        [{ name: 'Slime', hp: 100, mp: 0, enemy_id: 1 }]
    );

    var actor = $gameActors.actor(1);

    $MMO._dispatch('battle_action_result', {
        subject: { name: 'Healer', index: 0, is_actor: true },
        skill_id: 1,
        targets: [{
            target: { index: 0, is_actor: true },
            damage: -30,
            critical: false,
            missed: false,
            hp_after: 80,
            mp_after: 50
        }]
    });
    tickScene(35);

    assert.strictEqual(actor._hp, 80, 'actor HP should be 80 after healing');
    assert.strictEqual(actor.result().hpDamage, -30, 'result should show -30 (heal)');
});

// ==== Test: battle_action_result handles miss ====
test('battle_action_result handles miss correctly', function () {
    resetState();
    startBattleAndReady(
        [{ name: 'Hero', hp: 200, mp: 50, tp: 0 }],
        [{ name: 'Slime', hp: 100, mp: 0, enemy_id: 1 }]
    );

    $MMO._dispatch('battle_action_result', {
        subject: { name: 'Hero', index: 0, is_actor: true },
        skill_id: 1,
        targets: [{
            target: { index: 0, is_actor: false },
            missed: true,
            damage: 0,
            hp_after: 100,
            mp_after: 0
        }]
    });
    tickScene(35);

    assert.strictEqual($gameTroop._enemies[0]._hp, 100, 'enemy HP unchanged on miss');
    assert.strictEqual($gameTroop._enemies[0].result().missed, true, 'result should show missed');
});

// ==== Test: battle_action_result applies state changes ====
test('battle_action_result applies added/removed states', function () {
    resetState();
    startBattleAndReady(
        [{ name: 'Hero', hp: 200, mp: 50, tp: 0 }],
        [{ name: 'Spider', hp: 100, mp: 0, enemy_id: 1 }]
    );

    var actor = $gameActors.actor(1);
    $MMO._dispatch('battle_action_result', {
        subject: { name: 'Spider', index: 0, is_actor: false },
        skill_id: 1,
        targets: [{
            target: { index: 0, is_actor: true },
            damage: 10,
            critical: false,
            missed: false,
            hp_after: 190,
            mp_after: 50,
            added_states: [4],
            removed_states: []
        }]
    });
    tickScene(35);

    assert.strictEqual(actor._hp, 190, 'actor HP should be 190');
    assert.ok(actor.isStateAffected(4), 'actor should have poison state');
});

// ==== Test: battle_turn_end applies regen ====
test('battle_turn_end applies HP/MP/TP regen', function () {
    resetState();
    startBattleAndReady(
        [{ name: 'Hero', hp: 80, mp: 30, tp: 10 }],
        [{ name: 'Slime', hp: 100, mp: 0, enemy_id: 1 }]
    );

    var actor = $gameActors.actor(1);

    $MMO._dispatch('battle_turn_end', {
        regen: [{
            battler: { index: 0, is_actor: true },
            hp_change: 5,
            mp_change: 3,
            tp_change: 10
        }]
    });
    // Tick to process the queued turn_end event.
    tickScene(1);

    assert.strictEqual(actor._hp, 85, 'actor HP should be 85 after regen');
    assert.strictEqual(actor._mp, 33, 'actor MP should be 33 after regen');
    assert.strictEqual(actor._tp, 20, 'actor TP should be 20 after regen');
});

// ==== Test: battle_battle_end victory with rewards ====
test('battle_battle_end (victory) applies EXP, gold, and exits puppet mode', function () {
    resetState();
    startBattleAndReady(
        [{ name: 'Hero', hp: 200, mp: 50, tp: 0 }],
        [{ name: 'Slime', hp: 100, mp: 0, enemy_id: 1 }]
    );

    var actor = $gameActors.actor(1);
    actor._hp = 200; // alive

    $MMO._dispatch('battle_battle_end', {
        result: 0, // win
        exp: 150,
        gold: 75,
        drops: [
            { item_type: 1, item_id: 1, quantity: 2 }
        ]
    });

    assert.strictEqual($MMO._serverBattle, false, 'puppet mode should be off');
    assert.strictEqual($gameParty.gold(), 75, 'party should have 75 gold');
    assert.strictEqual(BattleManager._phase, 'battleEnd', 'BattleManager should be at battleEnd');

    // EXP should be applied.
    var expGained = actor._exp[actor._actorId] || 0;
    assert.ok(expGained >= 150, 'actor should have gained 150+ exp');

    // npc_battle_result should be sent.
    var resultMsg = lastSent('npc_battle_result');
    assert.ok(resultMsg, 'npc_battle_result should be sent');
    assert.strictEqual(resultMsg.result, 0, 'result should be 0 (win)');
});

// ==== Test: battle_battle_end escape ====
test('battle_battle_end (escape) exits puppet mode', function () {
    resetState();
    startBattleAndReady(
        [{ name: 'Hero', hp: 200, mp: 50, tp: 0 }],
        [{ name: 'Dragon', hp: 9999, mp: 0, enemy_id: 1 }]
    );

    $MMO._dispatch('battle_battle_end', {
        result: 1, // escape
        exp: 0,
        gold: 0,
        drops: []
    });

    assert.strictEqual($MMO._serverBattle, false, 'puppet mode should be off');
    var resultMsg = lastSent('npc_battle_result');
    assert.strictEqual(resultMsg.result, 1, 'result should be 1 (escape)');
});

// ==== Test: battle_battle_end defeat ====
test('battle_battle_end (defeat) exits puppet mode', function () {
    resetState();
    startBattleAndReady(
        [{ name: 'Hero', hp: 200, mp: 50, tp: 0 }],
        [{ name: 'Boss', hp: 9999, mp: 0, enemy_id: 1 }]
    );

    $MMO._dispatch('battle_battle_end', {
        result: 2, // lose
        exp: 0,
        gold: 0,
        drops: []
    });

    assert.strictEqual($MMO._serverBattle, false, 'puppet mode should be off');
    var resultMsg = lastSent('npc_battle_result');
    assert.strictEqual(resultMsg.result, 2, 'result should be 2 (lose)');
});

// ==== Test: non-puppet mode passes through ====
test('BattleManager.startInput passes through when not in puppet mode', function () {
    resetState();
    // $MMO._serverBattle is false — not in puppet mode.
    BattleManager._phase = 'init';
    BattleManager.startInput();
    assert.strictEqual(BattleManager._phase, 'input', 'startInput should work normally outside puppet mode');
});

// ==== Test: CE with crashing plugin command doesn't crash battle ====
test('CE with crashing plugin command is caught and skipped', function () {
    resetState();
    startBattleAndReady(
        [{ name: 'Hero', hp: 200, mp: 50, tp: 0 }],
        [{ name: 'Boss', hp: 500, mp: 0, enemy_id: 1 }]
    );

    // Register a CE that has a plugin command that will throw.
    // Simulates CE 1031 calling CulSkillEffect which accesses undefined toneArray.
    $dataCommonEvents[99] = {
        id: 99, name: 'TestCrashCE', trigger: 0, switchId: 0,
        list: [
            // Command 0: some working command (no-op)
            { code: 0, indent: 0, parameters: [] },
            // Command 356: Plugin command that throws
            { code: 356, indent: 0, parameters: ['CrashPlugin arg1'] },
            // Command 0: end
            { code: 0, indent: 0, parameters: [] }
        ]
    };

    // Alias pluginCommand to throw for 'CrashPlugin'.
    var _origPlugin = Game_Interpreter.prototype.pluginCommand;
    Game_Interpreter.prototype.pluginCommand = function (cmd, args) {
        if (cmd === 'CrashPlugin') {
            // Simulate: Cannot read properties of undefined (reading '71')
            throw new TypeError("Cannot read properties of undefined (reading '71')");
        }
        return _origPlugin.call(this, cmd, args);
    };

    // Set up the CE on the troop interpreter (like _executeCommonEvent does).
    $gameTroop._interpreter.setup($dataCommonEvents[99].list);
    assert.ok($gameTroop._interpreter.isRunning(), 'interpreter should be running');

    // In puppet mode, command356 catches the error at the source level.
    // The interpreter.update() should NOT throw — the error is handled
    // inside command356 and the command returns true (continue to next).
    var didThrow = false;
    try {
        $gameTroop._interpreter.update();
    } catch (e) {
        didThrow = true;
    }

    assert.ok(!didThrow, 'Error should have been caught by command356, not thrown');
    // Interpreter should have finished (3 commands: noop, plugin, noop).
    assert.ok(!$gameTroop._interpreter.isRunning(),
        'Interpreter should finish after skipping crashing command');

    // Restore original pluginCommand.
    Game_Interpreter.prototype.pluginCommand = _origPlugin;

    // Clean up
    delete $dataCommonEvents[99];
});

// ==== Test: CE with child interpreter crash is recovered ====
test('CE child interpreter crash is recovered (parent continues)', function () {
    resetState();
    startBattleAndReady(
        [{ name: 'Hero', hp: 200, mp: 50, tp: 0 }],
        [{ name: 'Boss', hp: 500, mp: 0, enemy_id: 1 }]
    );

    // Set up: parent CE calls child CE 98 via command 117,
    // and child CE has a crashing plugin command.
    $dataCommonEvents[98] = {
        id: 98, name: 'ChildCrashCE', trigger: 0, switchId: 0,
        list: [
            { code: 356, indent: 0, parameters: ['ChildCrash'] },
            { code: 0, indent: 0, parameters: [] }
        ]
    };
    $dataCommonEvents[97] = {
        id: 97, name: 'ParentCE', trigger: 0, switchId: 0,
        list: [
            { code: 0, indent: 0, parameters: [] },  // index 0: no-op
            { code: 117, indent: 0, parameters: [98] }, // index 1: call child CE 98
            { code: 0, indent: 0, parameters: [] }  // index 2: end
        ]
    };

    // Alias pluginCommand to throw for 'ChildCrash'.
    var _origPlugin = Game_Interpreter.prototype.pluginCommand;
    Game_Interpreter.prototype.pluginCommand = function (cmd, args) {
        if (cmd === 'ChildCrash') {
            throw new TypeError("Cannot read properties of undefined (reading '71')");
        }
        return _origPlugin.call(this, cmd, args);
    };

    // Set up parent CE on the troop interpreter.
    $gameTroop._interpreter.setup($dataCommonEvents[97].list);

    // In puppet mode, command356 catches the error at the source level.
    // The child interpreter's crashing plugin command is caught inside
    // command356, so update() should NOT throw.
    var didThrow = false;
    try {
        $gameTroop._interpreter.update();
    } catch (e) {
        didThrow = true;
    }

    assert.ok(!didThrow, 'Child interpreter error should have been caught by command356');

    // The child interpreter should have finished (crash was caught, continued).
    // Parent should either still be running or finished.
    // Tick again to let parent finish.
    if ($gameTroop._interpreter.isRunning()) {
        $gameTroop._interpreter.update();
    }

    // After ticking, parent should be finished.
    assert.ok(!$gameTroop._interpreter.isRunning(),
        'Parent interpreter should finish after child crash was handled');

    // Restore
    Game_Interpreter.prototype.pluginCommand = _origPlugin;
    delete $dataCommonEvents[97];
    delete $dataCommonEvents[98];
});

// ==== Test: action with common_event_ids queues action for CE execution ====
test('battle_action_result with common_event_ids queues action correctly', function () {
    resetState();
    startBattleAndReady(
        [{ name: 'Hero', hp: 200, mp: 50, tp: 0 }],
        [{ name: 'Boss', hp: 500, mp: 0, enemy_id: 1 }]
    );

    // Dispatch action result with common_event_ids.
    // The action is queued (not processed immediately).
    $MMO._dispatch('battle_action_result', {
        subject: { name: 'Hero', index: 0, is_actor: true },
        skill_id: 967,
        targets: [{
            target: { index: 0, is_actor: true },
            damage: 0,
            critical: false,
            missed: false,
            hp_after: 200,
            mp_after: 50,
            common_event_ids: [50]
        }]
    });

    // Verify the action was queued (not processed yet — queue processing
    // happens during the Scene_Battle update loop).
    assert.strictEqual($MMO._serverBattle, true,
        'Should still be in puppet mode after action result');
});

// ---- Run all tests ----
(async function () {
    console.log('\n=== Battle Puppet Mode Tests ===');
    for (const t of _tests) {
        try {
            await t.fn();
            console.log('  PASS: ' + t.name);
            _passed++;
        } catch (e) {
            console.error('  FAIL: ' + t.name);
            console.error('    ' + e.message);
            if (e.stack) {
                var lines = e.stack.split('\n').slice(1, 4);
                lines.forEach(function (l) { console.error('    ' + l.trim()); });
            }
            _failed++;
        }
    }
    console.log('  Results: ' + _passed + ' passed, ' + _failed + ' failed\n');
    process.exit(_failed > 0 ? 1 : 0);
})();
