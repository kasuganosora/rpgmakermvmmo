// @ts-check
const { test, expect } = require('@playwright/test');
const h = require('./helpers');

test.describe('Party Invite Flow', () => {

  test('player A invites player B, B accepts, both get party_update', async ({ browser }) => {
    const contextA = await browser.newContext();
    const contextB = await browser.newContext();
    const pageA = await contextA.newPage();
    const pageB = await contextB.newPage();

    const usernameA = h.uniqueName('partyA');
    const usernameB = h.uniqueName('partyB');
    const password = 'Test1234!';

    // Setup both players: register, create character, enter game.
    await Promise.all([
      h.registerAndEnterGame(pageA, usernameA, password, h.uniqueName('PtyA')),
      h.registerAndEnterGame(pageB, usernameB, password, h.uniqueName('PtyB')),
    ]);

    // Get char IDs.
    const stateA = await h.getMMOState(pageA);
    const stateB = await h.getMMOState(pageB);
    expect(stateA.connected).toBe(true);
    expect(stateB.connected).toBe(true);

    const charIDA = stateA.charID;
    const charIDB = stateB.charID;

    // Player B: start listening for party_invite_request BEFORE A sends it.
    const invitePromise = h.waitForMMOMessage(pageB, 'party_invite_request', 10000);

    // Player A: send party invite to Player B.
    await pageA.evaluate((targetId) => {
      $MMO.send('party_invite', { target_char_id: targetId });
    }, charIDB);

    // Player B should receive the invite.
    const invite = await invitePromise;
    expect(invite).toBeTruthy();
    expect(invite.from_id).toBe(charIDA);
    expect(invite.from_name).toBeTruthy();

    // Both players: start listening for party_update.
    const partyUpdateA = h.waitForMMOMessage(pageA, 'party_update', 10000);
    const partyUpdateB = h.waitForMMOMessage(pageB, 'party_update', 10000);

    // Player B: accept the invite.
    await pageB.evaluate((fromId) => {
      $MMO.send('party_invite_response', { accept: true, from_id: fromId });
    }, charIDA);

    // Both should receive party_update.
    const [updateA, updateB] = await Promise.all([partyUpdateA, partyUpdateB]);

    expect(updateA.members).toBeDefined();
    expect(updateA.members.length).toBe(2);
    expect(updateB.members).toBeDefined();
    expect(updateB.members.length).toBe(2);

    // Verify party member char_ids.
    const memberIdsA = updateA.members.map((m) => m.char_id).sort();
    expect(memberIdsA).toEqual([charIDA, charIDB].sort());

    // Verify party panel is visible on Player A (set by mmo-party.js handler).
    const panelVisibleA = await pageA.evaluate(() => {
      return $MMO._partyPanel ? $MMO._partyPanel.visible : false;
    });
    expect(panelVisibleA).toBe(true);

    await contextA.close();
    await contextB.close();
  });

  test('player B declines invite, no party is formed', async ({ browser }) => {
    const contextA = await browser.newContext();
    const contextB = await browser.newContext();
    const pageA = await contextA.newPage();
    const pageB = await contextB.newPage();

    const usernameA = h.uniqueName('decA');
    const usernameB = h.uniqueName('decB');
    const password = 'Test1234!';

    await Promise.all([
      h.registerAndEnterGame(pageA, usernameA, password, h.uniqueName('DecA')),
      h.registerAndEnterGame(pageB, usernameB, password, h.uniqueName('DecB')),
    ]);

    const stateA = await h.getMMOState(pageA);
    const stateB = await h.getMMOState(pageB);
    const charIDA = stateA.charID;
    const charIDB = stateB.charID;

    // B listens for invite.
    const invitePromise = h.waitForMMOMessage(pageB, 'party_invite_request', 10000);

    // A invites B.
    await pageA.evaluate((targetId) => {
      $MMO.send('party_invite', { target_char_id: targetId });
    }, charIDB);

    await invitePromise;

    // B declines.
    await pageB.evaluate((fromId) => {
      $MMO.send('party_invite_response', { accept: false, from_id: fromId });
    }, charIDA);

    // Wait a moment, then verify no party exists.
    await pageA.waitForTimeout(1000);

    const partyA = await pageA.evaluate(() => $MMO._partyData);
    expect(partyA).toBeFalsy();

    const panelVisibleA = await pageA.evaluate(() => {
      return $MMO._partyPanel ? $MMO._partyPanel.visible : false;
    });
    expect(panelVisibleA).toBe(false);

    await contextA.close();
    await contextB.close();
  });

  test('party invite to offline player returns error', async ({ browser }) => {
    const context = await browser.newContext();
    const page = await context.newPage();

    const username = h.uniqueName('partyErr');
    const password = 'Test1234!';

    await h.registerAndEnterGame(page, username, password, h.uniqueName('PtyErr'));

    // Listen for error message.
    const errorPromise = h.waitForMMOMessage(page, 'error', 10000);

    // Send invite to non-existent char_id.
    await page.evaluate(() => {
      $MMO.send('party_invite', { target_char_id: 999999 });
    });

    const err = await errorPromise;
    expect(err).toBeTruthy();
    expect(err.error).toBe('target_offline');

    await context.close();
  });
});
