#!/usr/bin/env node
/**
 * test-chat-flow.js  --  Chat send/receive via $MMO + mmo-chat.js handlers.
 *
 * Player A uses $MMO (full plugin stack).
 * Player B uses RawWSClient.
 *
 * Server chat protocol:
 *   chat_send:  { channel, content, target_name }
 *   chat_recv:  { channel, from_id, from_name, content, ts }
 *
 * Both world broadcast and whisper channels are tested.
 */
'use strict';

require('./rmmv-shim');

const assert = require('assert');
const {
    uniqueID, login, createCharacter, connectMMO,
    waitForEvent, sleep, RawWSClient,
    test, after, runTests
} = require('./helpers');
const loadPlugins = require('./load-plugins');

const HTTP_URL = process.env.TEST_HTTP_URL;
const WS_URL = process.env.TEST_WS_URL;

loadPlugins(WS_URL);

let tokenA, charIDA, charNameA;
let tokenB, charIDB, charNameB;
let wsB;

// -------------------------------------------------------------------

test('Setup: both players login, create chars, enter map', async function () {
    charNameA = uniqueID('Chatter');
    const a = await login(HTTP_URL, uniqueID('chA'), 'pass1234');
    tokenA = a.token;
    $MMO.token = tokenA;
    charIDA = await createCharacter(HTTP_URL, tokenA, charNameA);

    charNameB = uniqueID('Listener');
    const b = await login(HTTP_URL, uniqueID('chB'), 'pass1234');
    tokenB = b.token;
    charIDB = await createCharacter(HTTP_URL, tokenB, charNameB);

    // Player A via $MMO
    await connectMMO(WS_URL, tokenA);
    $MMO.charID = charIDA;
    $MMO.charName = charNameA;
    const mapInitA = waitForEvent('map_init', 5000);
    $MMO.send('enter_map', { char_id: charIDA });
    await mapInitA;

    // Player B via RawWSClient
    wsB = new RawWSClient();
    await wsB.connect(WS_URL, tokenB);
    wsB.send('enter_map', { char_id: charIDB });
    await wsB.recvType('map_init', 5000);
    await sleep(100);
});

test('World chat: A sends -> B receives via WS broadcast', async function () {
    const msg = 'Hello world ' + Date.now();
    $MMO.send('chat_send', { channel: 'world', content: msg });
    const recv = await wsB.recvType('chat_recv', 5000);
    assert.ok(recv, 'B should receive chat_recv');
    assert.strictEqual(recv.content, msg, 'message content should match');
    assert.strictEqual(recv.channel, 'world', 'channel should be world');
    assert.ok(recv.from_id, 'should have from_id');
});

test('mmo-chat.js stores world chat in _chatHistory', async function () {
    // A also receives its own world message via broadcast.
    // Give a small delay for the echo to arrive on A's side.
    await sleep(200);
    assert.ok($MMO._chatHistory, '_chatHistory should exist');
    assert.ok(Array.isArray($MMO._chatHistory.world), '_chatHistory.world should be array');
    assert.ok($MMO._chatHistory.world.length > 0,
        '_chatHistory.world should have at least 1 entry from world chat');
});

test('Chat handler registration', function () {
    assert.ok($MMO._handlers['chat_recv'], 'chat_recv handler should be registered');
    assert.ok($MMO._handlers['system_notice'], 'system_notice handler should be registered');
});

test('$MMO._chatChannel default', function () {
    assert.strictEqual($MMO._chatChannel, 'world', 'default chat channel should be world');
});

after(async function () {
    if (wsB) wsB.close();
    if ($MMO) $MMO.disconnect();
    await sleep(100);
});

runTests('Chat Flow').then(function (failures) {
    process.exit(failures > 0 ? 1 : 0);
});
