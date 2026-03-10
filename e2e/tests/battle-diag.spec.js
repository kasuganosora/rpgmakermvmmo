const { test, expect } = require('@playwright/test');
const h = require('./helpers');

test('diagnose party state', async ({ page }) => {
  await page.goto('/');
  await h.waitForGameReady(page);

  // Wait for game to fully boot past Scene_Boot (game objects created).
  await page.waitForFunction(() => {
    return typeof $gameParty !== 'undefined' && $gameParty !== null &&
           typeof $gamePlayer !== 'undefined' && $gamePlayer !== null;
  }, null, { timeout: 30000 });

  // Check party state after boot.
  const bootState = await page.evaluate(() => ({
    actors: $gameParty ? ($gameParty._actors || []).slice() : 'no_party',
    scene: SceneManager._scene ? SceneManager._scene.constructor.name : 'none',
  }));
  console.log('After full boot:', JSON.stringify(bootState));

  // Wait for $MMO.http.
  await page.waitForFunction(() => typeof $MMO !== 'undefined' && $MMO.http, null, { timeout: 15000 });

  // Check party again.
  const preLoginState = await page.evaluate(() => ({
    actors: $gameParty ? ($gameParty._actors || []).slice() : 'no_party',
    scene: SceneManager._scene ? SceneManager._scene.constructor.name : 'none',
  }));
  console.log('Before login:', JSON.stringify(preLoginState));

  // Register.
  const username = h.uniqueName('diag');
  await h.register(page, username, 'Test1234!');
  await h.createCharacter(page, h.uniqueName('Diag'));

  // Get char list.
  const chars = await page.evaluate(async () => {
    var data = await $MMO.http.get('/api/characters');
    return data.characters;
  });
  console.log('Characters:', JSON.stringify(chars));

  if (!chars || chars.length === 0) {
    console.log('No characters found, aborting');
    return;
  }

  // Connect WS.
  await page.evaluate(async () => {
    $MMO.disconnect();
    await new Promise(function (resolve) {
      var onConn = function () {
        $MMO.off('_connected', onConn);
        resolve();
      };
      $MMO.on('_connected', onConn);
      $MMO.connect($MMO.token);
    });
  });

  const preMapState = await page.evaluate(() => ({
    actors: $gameParty ? ($gameParty._actors || []).slice() : 'no_party',
    scene: SceneManager._scene ? SceneManager._scene.constructor.name : 'none',
    connected: $MMO.isConnected(),
  }));
  console.log('After WS connect:', JSON.stringify(preMapState));

  // Enter map.
  await page.evaluate((charId) => {
    $MMO.charID = charId;
    $MMO.send('enter_map', { char_id: charId });
    SceneManager.goto(Scene_Map);
  }, chars[0].id);

  await h.waitForMapReady(page);

  const mapState = await page.evaluate(() => ({
    actors: $gameParty ? ($gameParty._actors || []).slice() : 'no_party',
    scene: SceneManager._scene ? SceneManager._scene.constructor.name : 'none',
    members: $gameParty ? $gameParty.members().length : 0,
    battleMembers: $gameParty ? $gameParty.battleMembers().length : 0,
    hasActor1: $gameActors ? !!$gameActors.actor(1) : false,
    actor1Name: $gameActors && $gameActors.actor(1) ? $gameActors.actor(1).name() : 'none',
  }));
  console.log('After map ready:', JSON.stringify(mapState));

  // Deeply inspect addActor behavior.
  const deepDiag = await page.evaluate(() => {
    var arr = $gameParty._actors;
    var isArray = Array.isArray(arr);
    var beforeLen = arr ? arr.length : -1;

    // Test raw push on the array.
    var pushResult = null;
    if (arr) {
      arr.push(99);
      pushResult = arr.slice();
      arr.pop(); // restore
    }

    // Check if addActor is overridden by a plugin.
    var addActorSrc = $gameParty.addActor.toString().substring(0, 200);

    // Check if contains works.
    var containsResult = arr ? arr.contains(1) : 'no_arr';

    // Try manual push.
    if (arr) arr.push(1);
    var afterPush = arr ? arr.slice() : [];

    return {
      isArray: isArray,
      beforeLen: beforeLen,
      pushTest: pushResult,
      containsResult: containsResult,
      afterManualPush: afterPush,
      addActorSrc: addActorSrc,
    };
  });
  console.log('Deep diagnostic:', JSON.stringify(deepDiag));

  // Now check if addActor clears the array.
  const addActorTrace = await page.evaluate(() => {
    // Monkey-patch push to trace calls.
    var origPush = Array.prototype.push;
    var pushCalls = [];
    $gameParty._actors = []; // reset
    var origArr = $gameParty._actors;
    Object.defineProperty(origArr, 'push', {
      value: function() {
        pushCalls.push({ args: Array.from(arguments), stack: new Error().stack.substring(0, 500) });
        return origPush.apply(this, arguments);
      }
    });

    try {
      $gameParty.addActor(1);
    } catch (e) {
      pushCalls.push({ error: e.message });
    }

    return {
      pushCalls: pushCalls,
      actorsAfter: $gameParty._actors.slice(),
    };
  });
  console.log('AddActor trace:', JSON.stringify(addActorTrace));

  expect(true).toBe(true); // Always pass for diagnostics.
});
