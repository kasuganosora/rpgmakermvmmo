#!/usr/bin/env node
/**
 * test-social-flow.js  –  Guild create/join and friend request/accept via REST.
 *
 * Tests $MMO.http (mmo-auth.js) against real server for social endpoints.
 */
'use strict';

require('./rmmv-shim');

const assert = require('assert');
const {
    uniqueID, httpPost, httpGet, httpPut,
    login, createCharacter, connectMMO,
    waitForEvent, sleep,
    test, after, runTests
} = require('./helpers');
const loadPlugins = require('./load-plugins');

const HTTP_URL = process.env.TEST_HTTP_URL;
const WS_URL = process.env.TEST_WS_URL;

loadPlugins(WS_URL);

let tokenA, charIDA;
let tokenB, charIDB;
let guildID;

// -------------------------------------------------------------------

test('Setup: two players with characters', async function () {
    const a = await login(HTTP_URL, uniqueID('socA'), 'pass1234');
    tokenA = a.token;
    $MMO.token = tokenA;
    charIDA = await createCharacter(HTTP_URL, tokenA, uniqueID('SocialA'));

    const b = await login(HTTP_URL, uniqueID('socB'), 'pass1234');
    tokenB = b.token;
    charIDB = await createCharacter(HTTP_URL, tokenB, uniqueID('SocialB'));

    // Player A enters map (needed for active character context)
    await connectMMO(WS_URL, tokenA);
    $MMO.charID = charIDA;
    const mapInitA = waitForEvent('map_init', 5000);
    $MMO.send('enter_map', { char_id: charIDA });
    await mapInitA;
});

test('Create guild via $MMO.http.post', async function () {
    const guildName = uniqueID('TestGuild');
    const data = await $MMO.http.post('/api/guilds', {
        name: guildName,
        notice: 'E2E test guild'
    });
    assert.ok(data.id > 0, 'should return guild id');
    guildID = data.id;
});

test('Get guild detail via $MMO.http.get', async function () {
    const data = await $MMO.http.get('/api/guilds/' + guildID);
    assert.ok(data.guild, 'should have guild info');
    assert.ok(data.members, 'should have members list');
    assert.strictEqual(data.members.length, 1, 'should have 1 member (leader)');
});

test('Player B joins guild via HTTP', async function () {
    const res = await httpPost(HTTP_URL, '/api/guilds/' + guildID + '/join', {}, tokenB);
    assert.strictEqual(res.status, 200, 'join should return 200');

    // Verify 2 members
    const detail = await httpGet(HTTP_URL, '/api/guilds/' + guildID, tokenA);
    assert.strictEqual(detail.status, 200);
    assert.strictEqual(detail.data.members.length, 2, 'guild should have 2 members');
});

test('Non-leader cannot update guild notice', async function () {
    const res = await httpPut(HTTP_URL, '/api/guilds/' + guildID + '/notice',
        { notice: 'Hacked!' }, tokenB);
    assert.strictEqual(res.status, 403, 'non-leader should get 403');
});

test('Friend request and accept', async function () {
    // Player A sends friend request to Player B
    const reqRes = await httpPost(HTTP_URL, '/api/social/friends/request',
        { target_char_id: charIDB }, tokenA);
    assert.strictEqual(reqRes.status, 201, 'friend request should return 201');

    // Player A friends list should be empty (pending)
    const friendsA = await httpGet(HTTP_URL, '/api/social/friends', tokenA);
    assert.strictEqual(friendsA.status, 200);
    assert.strictEqual(friendsA.data.friends.length, 0, 'friends should be empty while pending');
});

test('mmo-social.js handler registration', function () {
    assert.ok($MMO._handlers['friend_request'], 'friend_request handler should be registered');
    assert.ok($MMO._handlers['friend_online'], 'friend_online handler should be registered');
    assert.ok($MMO._handlers['friend_offline'], 'friend_offline handler should be registered');
    assert.ok(typeof Window_FriendList === 'function', 'Window_FriendList should exist');
    assert.ok(typeof Window_GuildInfo === 'function', 'Window_GuildInfo should exist');
});

test('$MMO._showToast exists (mmo-social.js)', function () {
    assert.ok(typeof $MMO._showToast === 'function', '_showToast should be a function');
});

test('All plugin handlers registered', function () {
    // Verify key handlers from each plugin
    const expected = [
        'pong',            // mmo-core.js
        'map_init',        // mmo-other-players, mmo-battle, mmo-hud, mmo-skill-bar, mmo-social
        'player_join',     // mmo-other-players
        'player_leave',    // mmo-other-players
        'player_sync',     // mmo-other-players, mmo-hud, mmo-skill-bar
        'monster_spawn',   // mmo-battle
        'monster_death',   // mmo-battle
        'battle_result',   // mmo-battle, mmo-chat
        'inventory_update',// mmo-inventory
        'chat_recv',       // mmo-chat
        'party_update',    // mmo-party
        'trade_request',   // mmo-trade
    ];
    for (const type of expected) {
        assert.ok($MMO._handlers[type] && $MMO._handlers[type].length > 0,
            'handler for "' + type + '" should be registered');
    }
});

after(async function () {
    if ($MMO) $MMO.disconnect();
    await sleep(100);
});

runTests('Social Flow').then(function (failures) {
    process.exit(failures > 0 ? 1 : 0);
});
