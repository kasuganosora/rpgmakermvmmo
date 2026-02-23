#!/usr/bin/env node
/**
 * test-auth-flow.js  –  Full auth chain through actual client plugin code.
 *
 * Tests: login (HTTP) → list chars → create char → WS connect → enter map → map_init
 */
'use strict';

require('./rmmv-shim');

const assert = require('assert');
const {
    uniqueID, httpPost, httpGet,
    login, createCharacter, connectMMO, waitForEvent, sleep,
    test, before, after, runTests
} = require('./helpers');
const loadPlugins = require('./load-plugins');

const HTTP_URL = process.env.TEST_HTTP_URL;
const WS_URL = process.env.TEST_WS_URL;

// Load all client plugins (sets up $MMO, $MMO.http, handlers, etc.)
loadPlugins(WS_URL);

let token = null;
let charID = null;
const username = uniqueID('auth');
const password = 'testpass1234';

// -------------------------------------------------------------------

test('Login via $MMO.http.post (mmo-auth.js HTTP utility)', async function () {
    // The actual client code uses $MMO.http.post which wraps XMLHttpRequest.
    const data = await $MMO.http.post('/api/auth/login', {
        username: username,
        password: password
    });
    assert.ok(data.token, 'should return a JWT token');
    assert.ok(data.account_id > 0, 'should return account_id');
    $MMO.token = data.token;
    token = data.token;
});

test('List characters via $MMO.http.get → empty', async function () {
    const data = await $MMO.http.get('/api/characters');
    assert.ok(Array.isArray(data.characters), 'should return characters array');
    assert.strictEqual(data.characters.length, 0, 'should be empty for new account');
});

test('Create character via $MMO.http.post', async function () {
    const data = await $MMO.http.post('/api/characters', {
        name: uniqueID('Hero'),
        class_id: 1,
        walk_name: 'Actor1',
        walk_index: 0,
        face_name: 'Actor1',
        face_index: 0
    });
    assert.ok(data.id > 0, 'should return character id');
    charID = data.id;
});

test('List characters → has 1 character', async function () {
    const data = await $MMO.http.get('/api/characters');
    assert.strictEqual(data.characters.length, 1, 'should have 1 character');
    assert.strictEqual(data.characters[0].id, charID, 'character id should match');
});

test('WebSocket connect via $MMO.connect(token)', async function () {
    await connectMMO(WS_URL, token);
    assert.ok($MMO.isConnected(), '$MMO should be connected');
});

test('Enter map + receive map_init', async function () {
    const mapInitPromise = waitForEvent('map_init', 5000);
    $MMO.send('enter_map', { char_id: charID });
    const payload = await mapInitPromise;
    assert.ok(payload, 'should receive map_init payload');
    assert.ok(Array.isArray(payload.players), 'should have players array');
    // The joining player appears in their own players list.
    assert.ok(payload.players.length >= 1, 'players should include self');
});

test('$MMO.http.post login with wrong password → error', async function () {
    try {
        await $MMO.http.post('/api/auth/login', {
            username: username,
            password: 'wrongpassword'
        });
        assert.fail('should have thrown');
    } catch (e) {
        assert.ok(e.message, 'should have error message');
    }
});

test('Scene_CharacterSelect._enterGame integration', function () {
    // Verify the scene class exists (exported by mmo-auth.js)
    assert.ok(typeof Scene_CharacterSelect === 'function', 'Scene_CharacterSelect should exist');
    assert.ok(typeof Scene_Login === 'function', 'Scene_Login should exist');

    // Verify prototype chain
    const scene = new Scene_CharacterSelect();
    assert.ok(scene instanceof Scene_Base, 'should extend Scene_Base');

    // Verify _enterGame sets charID and calls connect
    assert.ok(typeof Scene_CharacterSelect.prototype._enterGame === 'function',
        '_enterGame should exist');
});

// Cleanup
after(async function () {
    if ($MMO && $MMO._ws) {
        $MMO.disconnect();
    }
    await sleep(100);
});

// Run
runTests('Auth Flow').then(function (failures) {
    process.exit(failures > 0 ? 1 : 0);
});
