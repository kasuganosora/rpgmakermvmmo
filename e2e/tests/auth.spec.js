// @ts-check
const { test, expect } = require('@playwright/test');
const h = require('./helpers');

test.describe('Auth & Enter Game', () => {

  test('register, create character, and enter game via UI', async ({ page }) => {
    const username = h.uniqueName('auth');
    const password = 'Test1234!';
    const charName = 'Hero_' + Date.now();

    await page.goto('/');
    await h.waitForGameReady(page);

    // Scene_Login should be active with HTML inputs.
    await page.waitForSelector('input[type="text"]', { timeout: 10000 });

    // Fill in credentials.
    await page.fill('input[type="text"]', username);
    await page.fill('input[type="password"]', password);

    // Trigger login (server auto-registers on first login).
    await page.evaluate(() => {
      SceneManager._scene._doLogin();
    });

    // Should transition to Scene_CharacterSelect.
    await h.waitForScene(page, 'Scene_CharacterSelect');

    // Verify token was set.
    const token = await page.evaluate(() => $MMO.token);
    expect(token).toBeTruthy();

    // No characters yet — navigate to character creation.
    await page.evaluate(() => {
      SceneManager.goto(Scene_CharacterCreate);
    });
    await h.waitForScene(page, 'Scene_CharacterCreate');

    // Fill character name (use placeholder selector to distinguish from class input).
    await page.waitForSelector('input[placeholder="Character Name"]', { timeout: 5000 });
    await page.fill('input[placeholder="Character Name"]', charName);

    // Trigger creation.
    await page.evaluate(() => {
      SceneManager._scene._doCreate();
    });

    // Should return to character select with 1 character.
    await h.waitForScene(page, 'Scene_CharacterSelect');
    await page.waitForFunction(() => {
      var scene = SceneManager._scene;
      return scene._characters && scene._characters.length > 0;
    }, null, { timeout: 10000 });

    // Enter the game.
    await h.selectFirstCharacter(page);
    await h.waitForMapReady(page);

    // Verify connected state.
    const state = await h.getMMOState(page);
    expect(state.connected).toBe(true);
    expect(state.charID).toBeGreaterThan(0);
    expect(state.lastSelf).toBeTruthy();
    expect(state.lastSelf.map_id).toBeGreaterThan(0);
  });

  test('login with existing account', async ({ page }) => {
    const username = h.uniqueName('login');
    const password = 'Test1234!';
    const charName = h.uniqueName('LoginCh');

    // Pre-setup: register and create character via API.
    await page.goto('/');
    await h.waitForGameReady(page);
    await page.waitForFunction(() => {
      return typeof $MMO !== 'undefined' && $MMO.http;
    }, null, { timeout: 15000 });
    await h.register(page, username, password);
    await h.createCharacter(page, charName);

    // Reload to get a fresh login screen.
    await page.reload();
    await h.waitForGameReady(page);

    // Login via UI.
    await h.login(page, username, password);

    // Should go to character select.
    await h.waitForScene(page, 'Scene_CharacterSelect');

    // Enter game.
    await h.selectFirstCharacter(page);
    await h.waitForMapReady(page);

    const state = await h.getMMOState(page);
    expect(state.connected).toBe(true);
    expect(state.charName).toBe(charName);
  });

  test('two players see each other on the map', async ({ browser }) => {
    const contextA = await browser.newContext();
    const contextB = await browser.newContext();
    const pageA = await contextA.newPage();
    const pageB = await contextB.newPage();

    const usernameA = h.uniqueName('seeA');
    const usernameB = h.uniqueName('seeB');
    const password = 'Test1234!';

    // Setup both players.
    await Promise.all([
      h.registerAndEnterGame(pageA, usernameA, password, h.uniqueName('SeeA')),
      h.registerAndEnterGame(pageB, usernameB, password, h.uniqueName('SeeB')),
    ]);

    const stateA = await h.getMMOState(pageA);
    const stateB = await h.getMMOState(pageB);
    expect(stateA.connected).toBe(true);
    expect(stateB.connected).toBe(true);

    // Wait a moment for player_join messages to propagate.
    await pageA.waitForTimeout(2000);

    // Player A should see Player B (or vice versa).
    const otherCountA = await h.getOtherPlayerCount(pageA);
    const otherCountB = await h.getOtherPlayerCount(pageB);
    // At least one should see the other (both are on the same default map).
    expect(otherCountA + otherCountB).toBeGreaterThan(0);

    await contextA.close();
    await contextB.close();
  });
});
