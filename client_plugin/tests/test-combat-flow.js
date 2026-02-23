#!/usr/bin/env node
/**
 * test-combat-flow.js  --  Monster attack, battle_result, monster state management.
 *
 * Both client and server use "attack" message type.
 * Server does NOT send "self" or "skills" in map_init (not implemented yet).
 */
'use strict';

require('./rmmv-shim');

const assert = require('assert');
const {
    uniqueID, login, createCharacter, connectMMO,
    waitForEvent, sleep,
    test, after, runTests
} = require('./helpers');
const loadPlugins = require('./load-plugins');

const HTTP_URL = process.env.TEST_HTTP_URL;
const WS_URL = process.env.TEST_WS_URL;

loadPlugins(WS_URL);

let token, charID;
let mapInitData = null;

// -------------------------------------------------------------------

test('Login + create char + enter map', async function () {
    const a = await login(HTTP_URL, uniqueID('combat'), 'pass1234');
    token = a.token;
    $MMO.token = token;
    charID = await createCharacter(HTTP_URL, token, uniqueID('Fighter'));
    await connectMMO(WS_URL, token);
    $MMO.charID = charID;
    const mapInitPromise = waitForEvent('map_init', 5000);
    $MMO.send('enter_map', { char_id: charID });
    mapInitData = await mapInitPromise;
    assert.ok(mapInitData, 'should receive map_init');
});

test('MonsterManager initialized from map_init', function () {
    assert.ok(typeof MonsterManager !== 'undefined', 'MonsterManager should exist');
    // Monsters from map_init were processed by mmo-battle.js handler
    const monsters = mapInitData.monsters || [];
    const spriteCount = Object.keys(MonsterManager._sprites).length;
    assert.strictEqual(spriteCount, monsters.length,
        'MonsterManager should have ' + monsters.length + ' monster sprites, got ' + spriteCount);
});

test('Attack invalid target -> receive error', async function () {
    // No monsters on test map, so attacking a fake target should return error.
    const errPromise = waitForEvent('error', 5000);
    $MMO.send('attack', {
        target_id: 99999,
        target_type: 'monster'
    });
    const err = await errPromise;
    assert.ok(err, 'should receive error for invalid target');
    assert.ok(err.message === 'monster not found' || err.message === 'not in a map',
        'error message should indicate invalid target, got: ' + err.message);
});

test('mmo-battle.js handler registration', function () {
    assert.ok($MMO._handlers['battle_result'], 'battle_result handler should be registered');
    assert.ok($MMO._handlers['monster_spawn'], 'monster_spawn handler should be registered');
    assert.ok($MMO._handlers['monster_death'], 'monster_death handler should be registered');
    assert.ok($MMO._handlers['map_init'], 'map_init handler should be registered');
});

test('Skill bar array initialized', function () {
    // mmo-skill-bar.js initializes _skillBar as empty array
    assert.ok(Array.isArray($MMO._skillBar), '_skillBar should be array');
    // Server doesn't send skills in map_init, so bar stays empty
    // Just verify the data structure exists
});

after(async function () {
    if ($MMO) $MMO.disconnect();
    await sleep(100);
});

runTests('Combat Flow').then(function (failures) {
    process.exit(failures > 0 ? 1 : 0);
});
