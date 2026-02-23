// @ts-check
const { test, expect } = require('@playwright/test');
const h = require('./helpers');

test.describe('Trade Flow', () => {

  test('player A requests trade with B, B accepts, trade window opens, then cancel', async ({ browser }) => {
    const contextA = await browser.newContext();
    const contextB = await browser.newContext();
    const pageA = await contextA.newPage();
    const pageB = await contextB.newPage();

    const usernameA = h.uniqueName('tradeA');
    const usernameB = h.uniqueName('tradeB');
    const password = 'Test1234!';

    // Setup both players.
    await Promise.all([
      h.registerAndEnterGame(pageA, usernameA, password, h.uniqueName('TrdA')),
      h.registerAndEnterGame(pageB, usernameB, password, h.uniqueName('TrdB')),
    ]);

    const stateA = await h.getMMOState(pageA);
    const stateB = await h.getMMOState(pageB);
    expect(stateA.connected).toBe(true);
    expect(stateB.connected).toBe(true);

    const charIDA = stateA.charID;
    const charIDB = stateB.charID;

    // B listens for trade_request.
    const tradeReqPromise = h.waitForMMOMessage(pageB, 'trade_request', 10000);

    // A sends trade request to B.
    await pageA.evaluate((targetId) => {
      $MMO.send('trade_request', { target_char_id: targetId });
    }, charIDB);

    // B should receive trade_request.
    const tradeReq = await tradeReqPromise;
    expect(tradeReq).toBeTruthy();
    expect(tradeReq.from_id).toBe(charIDA);
    expect(tradeReq.from_name).toBeTruthy();

    // A listens for trade_accepted (trade window open event).
    const tradeAcceptedA = h.waitForMMOMessage(pageA, 'trade_accepted', 10000);
    // B also listens for trade_accepted.
    const tradeAcceptedB = h.waitForMMOMessage(pageB, 'trade_accepted', 10000);

    // B accepts the trade.
    await pageB.evaluate((fromId) => {
      $MMO.send('trade_accept', { from_char_id: fromId });
    }, charIDA);

    // Both should receive trade_accepted.
    const [acceptedA, acceptedB] = await Promise.all([tradeAcceptedA, tradeAcceptedB]);
    expect(acceptedA).toBeTruthy();
    expect(acceptedA.session_id).toBeTruthy();
    expect(acceptedB).toBeTruthy();

    // Verify trade window is visible on A.
    const tradeWindowA = await pageA.evaluate(() => {
      return $MMO._tradeWindow ? $MMO._tradeWindow.visible : false;
    });
    expect(tradeWindowA).toBe(true);

    // B listens for trade_cancel.
    const tradeCancelB = h.waitForMMOMessage(pageB, 'trade_cancel', 10000);
    // A also listens for trade_cancel (server sends to both).
    const tradeCancelA = h.waitForMMOMessage(pageA, 'trade_cancel', 10000);

    // A cancels the trade.
    await pageA.evaluate(() => {
      $MMO.send('trade_cancel', {});
    });

    // Both should receive trade_cancel.
    await Promise.all([tradeCancelA, tradeCancelB]);

    // Trade windows should be closed.
    const tradeWindowAfterA = await pageA.evaluate(() => {
      return $MMO._tradeWindow ? $MMO._tradeWindow.visible : false;
    });
    expect(tradeWindowAfterA).toBe(false);

    await contextA.close();
    await contextB.close();
  });

  test('trade request to offline player returns error', async ({ browser }) => {
    const context = await browser.newContext();
    const page = await context.newPage();

    const username = h.uniqueName('tradeErr');
    const password = 'Test1234!';

    await h.registerAndEnterGame(page, username, password, h.uniqueName('TrdErr'));

    // Listen for error message.
    const errorPromise = h.waitForMMOMessage(page, 'error', 10000);

    // Send trade request to non-existent char_id.
    await page.evaluate(() => {
      $MMO.send('trade_request', { target_char_id: 999999 });
    });

    const err = await errorPromise;
    expect(err).toBeTruthy();
    expect(err.error).toBe('target_offline');

    await context.close();
  });
});
