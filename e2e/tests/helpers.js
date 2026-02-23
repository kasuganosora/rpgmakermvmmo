// Shared helpers for E2E tests.

/**
 * Wait for the RMMV game to fully boot (SceneManager running).
 */
async function waitForGameReady(page) {
  await page.waitForFunction(() => {
    return typeof SceneManager !== 'undefined' && SceneManager._scene != null;
  }, null, { timeout: 15000 });
}

/**
 * Wait for a specific RMMV scene to be active.
 */
async function waitForScene(page, sceneName) {
  await page.waitForFunction((name) => {
    return SceneManager._scene && SceneManager._scene.constructor.name === name;
  }, sceneName, { timeout: 15000 });
}

/**
 * Register/login a new account via the $MMO.http API.
 * The server auto-registers on first login, so this uses /api/auth/login.
 * Requires the game page to be loaded (so $MMO.http is available).
 */
async function register(page, username, password) {
  return page.evaluate(async (creds) => {
    var data = await $MMO.http.post('/api/auth/login', {
      username: creds.username,
      password: creds.password,
    });
    $MMO.token = data.token;
    return { token: data.token };
  }, { username, password });
}

/**
 * Login via the HTML input fields on Scene_Login.
 */
async function login(page, username, password) {
  await waitForGameReady(page);
  // Wait for Scene_Login to appear and HTML inputs to be created.
  await page.waitForSelector('input[type="text"]', { timeout: 10000 });
  await page.fill('input[type="text"]', username);
  await page.fill('input[type="password"]', password);
  await page.press('input[type="password"]', 'Enter');
}

/**
 * Create a character via the $MMO.http API.
 */
async function createCharacter(page, name) {
  return page.evaluate(async (charName) => {
    return await $MMO.http.post('/api/characters', {
      name: charName,
      class_id: 1,
      walk_name: 'Actor1',
      face_name: 'Actor1',
    });
  }, name);
}

/**
 * Wait for WS connection and map_init.
 * After character select, the client connects WS and enters the map.
 */
async function waitForMapReady(page) {
  await page.waitForFunction(() => {
    return typeof $MMO !== 'undefined' &&
           $MMO.isConnected() &&
           $MMO._lastSelf != null;
  }, null, { timeout: 15000 });
  // Give the map a moment to render sprites.
  await page.waitForTimeout(1000);
}

/**
 * Select the first character in Scene_CharacterSelect and enter the game.
 * Uses page.evaluate to trigger the selection since the UI is canvas-based.
 */
async function selectFirstCharacter(page) {
  // Wait for character select scene.
  await page.waitForFunction(() => {
    return SceneManager._scene &&
           SceneManager._scene.constructor.name === 'Scene_CharacterSelect';
  }, null, { timeout: 10000 });

  // Wait for characters to be loaded.
  await page.waitForFunction(() => {
    var scene = SceneManager._scene;
    return scene._characters && scene._characters.length > 0;
  }, null, { timeout: 10000 });

  // Click the first character slot (canvas-based, so use evaluate).
  await page.evaluate(() => {
    var scene = SceneManager._scene;
    if (scene._enterGame) {
      // Directly enter with the first character.
      scene._selectedIndex = 0;
      scene._enterGame();
    }
  });
}

/**
 * Full login flow: login → select character → wait for map.
 */
async function loginAndEnterMap(page, username, password) {
  await login(page, username, password);
  await selectFirstCharacter(page);
  await waitForMapReady(page);
}

/**
 * Full setup flow using API: register → create character → enter game.
 * Faster and more reliable than going through UI for each step.
 */
async function registerAndEnterGame(page, username, password, charName) {
  await page.goto('/');
  await waitForGameReady(page);

  // Wait for $MMO.http to be available (loaded by mmo-auth.js plugin).
  await page.waitForFunction(() => {
    return typeof $MMO !== 'undefined' && $MMO.http;
  }, null, { timeout: 15000 });

  // Register and create character via API.
  await register(page, username, password);
  await createCharacter(page, charName);

  // Navigate to character select and enter the game.
  await page.evaluate(() => {
    SceneManager.goto(Scene_CharacterSelect);
  });

  await selectFirstCharacter(page);
  await waitForMapReady(page);
}

/**
 * Listen for a specific $MMO message type. Returns a promise that resolves
 * with the payload when the message is received.
 */
async function waitForMMOMessage(page, type, timeoutMs = 10000) {
  return page.evaluate(([msgType, timeout]) => {
    return new Promise((resolve, reject) => {
      var timer = setTimeout(() => {
        $MMO.off(msgType, handler);
        reject(new Error('Timeout waiting for ' + msgType));
      }, timeout);
      var handler = function (data) {
        clearTimeout(timer);
        $MMO.off(msgType, handler);
        resolve(data);
      };
      $MMO.on(msgType, handler);
    });
  }, [type, timeoutMs]);
}

/**
 * Get the current $MMO state.
 */
async function getMMOState(page) {
  return page.evaluate(() => ({
    connected: $MMO.isConnected(),
    charID: $MMO.charID,
    charName: $MMO.charName,
    lastSelf: $MMO._lastSelf,
    partyData: $MMO._partyData,
  }));
}

/**
 * Get OtherPlayerManager sprite count.
 */
async function getOtherPlayerCount(page) {
  return page.evaluate(() => {
    return Object.keys(OtherPlayerManager._sprites).length;
  });
}

/**
 * Get other player char IDs.
 */
async function getOtherPlayerIDs(page) {
  return page.evaluate(() => {
    return Object.keys(OtherPlayerManager._sprites).map(Number);
  });
}

/**
 * Generate a unique test username.
 */
function uniqueName(prefix) {
  return prefix + '_' + Date.now() + '_' + Math.random().toString(36).slice(2, 6);
}

module.exports = {
  waitForGameReady,
  waitForScene,
  register,
  login,
  createCharacter,
  waitForMapReady,
  selectFirstCharacter,
  loginAndEnterMap,
  registerAndEnterGame,
  waitForMMOMessage,
  getMMOState,
  getOtherPlayerCount,
  getOtherPlayerIDs,
  uniqueName,
};
