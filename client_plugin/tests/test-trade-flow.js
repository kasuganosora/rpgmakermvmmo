#!/usr/bin/env node
/**
 * test-trade-flow.js  --  Trade request, accept, cancel between two players.
 *
 * Player A uses $MMO (full plugin stack).
 * Player B uses RawWSClient.
 *
 * Server trade protocol:
 *   trade_request  -> target receives {from_id, from_name}
 *   trade_accept   -> initiator notified, session starts
 *   trade_cancel   -> both receive {reason: "cancelled"}
 *   error          -> {error: "target_offline"} / {error: "already in a trade"}
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

let tokenA, charIDA;
let tokenB, charIDB;
let wsB;

// -------------------------------------------------------------------

test('Setup: both players login, create chars, enter map', async function () {
    const a = await login(HTTP_URL, uniqueID('trA'), 'pass1234');
    tokenA = a.token;
    $MMO.token = tokenA;
    charIDA = await createCharacter(HTTP_URL, tokenA, uniqueID('TraderA'));

    const b = await login(HTTP_URL, uniqueID('trB'), 'pass1234');
    tokenB = b.token;
    charIDB = await createCharacter(HTTP_URL, tokenB, uniqueID('TraderB'));

    // Player A via $MMO
    await connectMMO(WS_URL, tokenA);
    $MMO.charID = charIDA;
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

test('Player A sends trade_request -> Player B receives it', async function () {
    $MMO.send('trade_request', { target_char_id: charIDB });
    const req = await wsB.recvType('trade_request', 5000);
    assert.ok(req, 'B should receive trade_request');
    assert.strictEqual(req.from_id, charIDA,
        'trade_request from_id should be A\'s charID');
});

test('Player B accepts trade', async function () {
    wsB.send('trade_accept', { from_char_id: charIDA });
    // Server starts trade session.  No explicit confirmation message.
    // Just verify no crash.
    await sleep(300);
});

test('Player A cancels trade -> Player B receives trade_cancel', async function () {
    $MMO.send('trade_cancel', {});
    const cancel = await wsB.recvType('trade_cancel', 5000);
    assert.ok(cancel, 'B should receive trade_cancel');
    assert.strictEqual(cancel.reason, 'cancelled', 'reason should be cancelled');
});

test('Trade with offline player -> error', async function () {
    // Wait for trade state to fully clean up
    await sleep(200);
    const errPromise = waitForEvent('error', 5000);
    $MMO.send('trade_request', { target_char_id: 99999 });
    const err = await errPromise;
    assert.ok(err, 'should receive error for offline target');
    assert.ok(err.error === 'target_offline' || err.message === 'target_offline',
        'error should indicate target offline, got: ' + JSON.stringify(err));
});

test('mmo-trade.js handler registration', function () {
    assert.ok($MMO._handlers['trade_request'], 'trade_request handler should be registered');
    assert.ok($MMO._handlers['trade_accepted'], 'trade_accepted handler should be registered');
    assert.ok($MMO._handlers['trade_update'], 'trade_update handler should be registered');
    assert.ok($MMO._handlers['trade_cancel'], 'trade_cancel handler should be registered');
});

after(async function () {
    if (wsB) wsB.close();
    if ($MMO) $MMO.disconnect();
    await sleep(100);
});

runTests('Trade Flow').then(function (failures) {
    process.exit(failures > 0 ? 1 : 0);
});
