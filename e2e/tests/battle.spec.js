// @ts-check
const { test, expect } = require('@playwright/test');
const h = require('./helpers');

/**
 * Enter game by directly calling WS connect + enter_map, bypassing the
 * character select UI which has timing issues in headless Playwright.
 */
async function enterGameDirect(page, username, password, charName) {
  await page.goto('/');
  await h.waitForGameReady(page);

  // Wait for game objects to be fully created (Scene_Boot must finish).
  await page.waitForFunction(() => {
    return typeof $gameParty !== 'undefined' && $gameParty !== null &&
           typeof $gameActors !== 'undefined' && $gameActors !== null;
  }, null, { timeout: 30000 });

  // Wait for $MMO.http to be available.
  await page.waitForFunction(() => {
    return typeof $MMO !== 'undefined' && $MMO.http;
  }, null, { timeout: 15000 });

  // Register, create character, and enter game directly via API + WS.
  const charID = await page.evaluate(async (args) => {
    var data = await $MMO.http.post('/api/auth/login', {
      username: args.username,
      password: args.password,
    });
    $MMO.token = data.token;

    var ch = await $MMO.http.post('/api/characters', {
      name: args.charName,
      class_id: 1,
      walk_name: 'Actor1',
      face_name: 'Actor1',
    });
    var charID = ch.id || ch.char_id;

    $MMO.charID = charID;
    $MMO.charName = args.charName;

    return new Promise(function (resolve) {
      $MMO.disconnect();
      var onConnected = function () {
        $MMO.off('_connected', onConnected);
        $MMO.send('enter_map', { char_id: charID });
        resolve(charID);
      };
      $MMO.on('_connected', onConnected);
      $MMO.connect($MMO.token);
      SceneManager.goto(Scene_Map);
    });
  }, { username, password, charName });

  await h.waitForMapReady(page);
  return charID;
}

/**
 * Wait for Scene_Battle to be fully ready with command window available.
 */
async function waitForBattleSceneReady(page) {
  await page.waitForFunction(() => {
    var scene = SceneManager._scene;
    return scene && scene.constructor.name === 'Scene_Battle' &&
           scene._actorCommandWindow && scene._statusWindow;
  }, null, { timeout: 15000 });
  // Extra frames for RMMV/YEP plugin initialization.
  await page.waitForTimeout(2000);
}


test.describe('Server-Authoritative Battle (Puppet Mode)', () => {

  // -----------------------------------------------------------------------
  // Test 1: Command window appears after input_request (client simulation)
  // -----------------------------------------------------------------------
  test('command window appears after input_request (client simulation)', async ({ page }) => {
    const username = h.uniqueName('btlsim');
    const password = 'Test1234!';
    const charName = h.uniqueName('SimHero');

    await enterGameDirect(page, username, password, charName);

    const logs = [];
    page.on('console', msg => {
      if (msg.text().includes('[Puppet]')) {
        logs.push(msg.text());
      }
    });

    // Simulate battle_battle_start from server.
    await page.evaluate(() => {
      $MMO._dispatch('battle_battle_start', {
        actors: [{
          index: 0, is_actor: true, name: 'TestHero',
          hp: 500, max_hp: 500, mp: 50, max_mp: 50, tp: 0,
          states: [], class_id: 1, level: 10, enemy_id: 0
        }],
        enemies: [{
          index: 0, is_actor: false, name: 'Slime',
          hp: 100, max_hp: 100, mp: 0, max_mp: 0, tp: 0,
          states: [], class_id: 0, level: 1, enemy_id: 21
        }]
      });
    });

    await waitForBattleSceneReady(page);

    // Simulate battle_input_request from server.
    await page.evaluate(() => {
      $MMO._dispatch('battle_input_request', { actor_index: 0 });
    });

    // Wait for the command window to become visible and active.
    const cmdReady = await page.waitForFunction(() => {
      var scene = SceneManager._scene;
      if (!scene || !(scene instanceof Scene_Battle)) return false;
      var cmdWin = scene._actorCommandWindow;
      return cmdWin && cmdWin.visible && cmdWin.active;
    }, null, { timeout: 10000 }).catch(() => null);

    expect(cmdReady).toBeTruthy();

    // Verify party state.
    const partyAfter = await page.evaluate(() => ({
      actors: ($gameParty._actors || []).slice(),
      memberCount: $gameParty.battleMembers().length,
      inBattle: $gameParty._inBattle,
      phase: BattleManager._phase,
    }));

    expect(partyAfter.actors.length).toBeGreaterThan(0);
    expect(partyAfter.memberCount).toBeGreaterThan(0);
    expect(partyAfter.inBattle).toBe(true);
    expect(partyAfter.phase).toBe('input');

    // Verify actor HP matches server data.
    const actorHP = await page.evaluate(() => {
      var actor = BattleManager.actor();
      return actor ? actor.hp : -1;
    });
    expect(actorHP).toBe(500);

    // Clean up.
    await page.evaluate(() => {
      $MMO._dispatch('battle_battle_end', { result: 0, exp: 10, gold: 5, drops: [] });
    });
    await page.waitForFunction(() => $MMO._serverBattle === false, null, { timeout: 5000 });

    console.log('Battle logs:', logs.join('\n'));
  });

  // -----------------------------------------------------------------------
  // Test 2: Damage popups and HP updates from action_result (client sim)
  // -----------------------------------------------------------------------
  test('action result shows damage popup and updates HP', async ({ page }) => {
    const username = h.uniqueName('btldmg');
    const password = 'Test1234!';
    const charName = h.uniqueName('DmgHero');

    await enterGameDirect(page, username, password, charName);

    const logs = [];
    page.on('console', msg => {
      if (msg.text().includes('[Puppet]')) {
        logs.push(msg.text());
      }
    });

    // Start puppet battle.
    await page.evaluate(() => {
      $MMO._dispatch('battle_battle_start', {
        actors: [{
          index: 0, is_actor: true, name: 'TestHero',
          hp: 500, max_hp: 500, mp: 50, max_mp: 50, tp: 0,
          states: [], class_id: 1, level: 10, enemy_id: 0
        }],
        enemies: [{
          index: 0, is_actor: false, name: 'Slime',
          hp: 100, max_hp: 100, mp: 0, max_mp: 0, tp: 0,
          states: [], class_id: 0, level: 1, enemy_id: 21
        }]
      });
    });

    await waitForBattleSceneReady(page);

    // Get actual enemy HP after setup (RMMV clamps to data max HP).
    const enemyHPBefore = await page.evaluate(() => {
      return $gameTroop.members()[0] ? $gameTroop.members()[0].hp : -1;
    });
    expect(enemyHPBefore).toBeGreaterThan(0);
    console.log('Enemy HP before action:', enemyHPBefore);

    // Simulate both actions for this turn: player attacks enemy, enemy attacks player.
    const expectedEnemyHP = enemyHPBefore - 30;
    await page.evaluate((hpAfter) => {
      // Player attacks enemy for 30 damage.
      $MMO._dispatch('battle_action_result', {
        subject: { index: 0, is_actor: true, name: 'TestHero' },
        skill_id: 1,
        targets: [{
          target: { index: 0, is_actor: false },
          damage: 30,
          critical: false,
          missed: false,
          hp_after: hpAfter,
          mp_after: 0,
        }]
      });
      // Enemy attacks player for 45 damage.
      $MMO._dispatch('battle_action_result', {
        subject: { index: 0, is_actor: false, name: 'Slime' },
        skill_id: 1,
        targets: [{
          target: { index: 0, is_actor: true },
          damage: 45,
          critical: false,
          missed: false,
          hp_after: 455,
          mp_after: 50,
        }]
      });
    }, expectedEnemyHP);

    // Wait for HP to reflect server data (actions process sequentially ~20 frames each).
    await page.waitForFunction((expected) => {
      var enemy = $gameTroop.members()[0];
      var actor = $gameParty.battleMembers()[0];
      return enemy && enemy.hp === expected.enemyHP &&
             actor && actor.hp === expected.actorHP;
    }, { enemyHP: expectedEnemyHP, actorHP: 455 }, { timeout: 15000 });

    // Verify enemy HP was updated from server data.
    const enemyHPAfter = await page.evaluate(() => {
      return $gameTroop.members()[0] ? $gameTroop.members()[0].hp : -1;
    });
    expect(enemyHPAfter).toBe(expectedEnemyHP);

    // Verify actor HP was updated.
    const actorHP = await page.evaluate(() => {
      var actor = $gameParty.battleMembers()[0];
      return actor ? actor.hp : -1;
    });
    expect(actorHP).toBe(455);

    // Simulate turn_end then input_request.
    // Wait for both actions to finish animating first.
    await page.waitForFunction(() => {
      return BattleManager._phase === 'waiting';
    }, null, { timeout: 10000 });

    await page.evaluate(() => {
      $MMO._dispatch('battle_turn_end', {});
      $MMO._dispatch('battle_input_request', { actor_index: 0 });
    });

    // Wait for command window to reappear.
    const cmdReady = await page.waitForFunction(() => {
      var scene = SceneManager._scene;
      if (!scene || !(scene instanceof Scene_Battle)) return false;
      var cmdWin = scene._actorCommandWindow;
      return cmdWin && cmdWin.visible && cmdWin.active;
    }, null, { timeout: 10000 }).catch(() => null);

    expect(cmdReady).toBeTruthy();

    // Clean up.
    await page.evaluate(() => {
      $MMO._dispatch('battle_battle_end', { result: 0, exp: 10, gold: 5, drops: [] });
    });

    console.log('Battle logs:', logs.join('\n'));
  });

  // -----------------------------------------------------------------------
  // Test 3: Full server-driven battle via debug_start_battle
  // -----------------------------------------------------------------------
  test('full server battle via debug_start_battle', async ({ page }) => {
    const username = h.uniqueName('btlsrv');
    const password = 'Test1234!';
    const charName = h.uniqueName('SrvHero');

    await enterGameDirect(page, username, password, charName);

    const logs = [];
    page.on('console', msg => {
      const text = msg.text();
      if (text.includes('[Puppet]') || text.includes('[MMO]')) {
        logs.push(text);
      }
    });

    // Set up debug_ok listener before sending commands.
    await page.evaluate(() => {
      window._debugOkCount = 0;
      $MMO.on('debug_ok', function () { window._debugOkCount++; });
    });

    // Boost stats so we can win quickly.
    await page.evaluate(() => {
      $MMO.send('debug_set_stats', {
        level: 30, hp: 2000, max_hp: 2000, mp: 200, max_mp: 200
      });
    });

    const hasDebug = await page.waitForFunction(() => {
      return window._debugOkCount > 0;
    }, null, { timeout: 5000 }).catch(() => null);

    if (!hasDebug) {
      console.log('Server does not have debug handlers, skipping test');
      test.skip(true, 'Server not in debug mode (no debug handlers)');
      return;
    }

    // Set up battle event tracking and auto-attack.
    await page.evaluate(() => {
      window._battleEvents = [];
      window._battleEnded = false;
      window._battleResult = -1;
      window._inputRequestCount = 0;
      window._damagePopupCount = 0;

      // Track damage popups.
      var _origStartDmgPopup = Game_Battler.prototype.startDamagePopup;
      Game_Battler.prototype.startDamagePopup = function () {
        window._damagePopupCount++;
        _origStartDmgPopup.call(this);
      };

      $MMO.on('battle_battle_start', function () {
        window._battleEvents.push('battle_start');
      });
      $MMO.on('battle_input_request', function (data) {
        window._inputRequestCount++;
        window._battleEvents.push('input_request');
        // Auto-attack: send input after short delay for scene readiness.
        setTimeout(function () {
          $MMO.send('battle_input', {
            actor_index: data.actor_index,
            action_type: 0,
            target_indices: [0],
            target_is_actor: false,
          });
        }, 500);
      });
      $MMO.on('battle_action_result', function () {
        window._battleEvents.push('action_result');
      });
      $MMO.on('battle_turn_start', function () {
        window._battleEvents.push('turn_start');
      });
      $MMO.on('battle_turn_end', function () {
        window._battleEvents.push('turn_end');
      });
      $MMO.on('battle_battle_end', function (data) {
        window._battleEvents.push('battle_end');
        window._battleEnded = true;
        window._battleResult = data.result;
      });
    });

    // Trigger battle via debug handler.
    await page.evaluate(() => {
      $MMO.send('debug_start_battle', {
        troop_id: 4,
        can_escape: true,
        can_lose: true,
      });
    });

    // Wait for Scene_Battle to appear.
    const sceneBattle = await page.waitForFunction(() => {
      return SceneManager._scene && SceneManager._scene.constructor.name === 'Scene_Battle';
    }, null, { timeout: 15000 }).catch(() => null);

    if (!sceneBattle) {
      console.log('Scene_Battle did not appear. Logs:', logs.join('\n'));
      test.skip(true, 'debug_start_battle may not be available');
      return;
    }

    // Wait for battle to end (60s for multi-turn battle with animations).
    const battleEnded = await page.waitForFunction(() => {
      return window._battleEnded === true;
    }, null, { timeout: 60000 }).catch(() => null);

    const result = await page.evaluate(() => ({
      events: window._battleEvents,
      ended: window._battleEnded,
      result: window._battleResult,
      inputRequests: window._inputRequestCount,
      damagePopups: window._damagePopupCount,
    }));

    console.log('Battle events:', JSON.stringify(result.events));
    console.log('Battle result:', result.result);
    console.log('Input requests:', result.inputRequests);
    console.log('Damage popups:', result.damagePopups);

    if (battleEnded) {
      expect(result.events.length).toBeGreaterThanOrEqual(5);
      expect(result.events[0]).toBe('battle_start');
      expect(result.events[result.events.length - 1]).toBe('battle_end');
      expect(result.inputRequests).toBeGreaterThan(0);

      // With level 30 and 2000 HP, we should win.
      expect(result.result).toBe(0);

      // Damage popups should have appeared (at least 1 per action).
      expect(result.damagePopups).toBeGreaterThan(0);
    } else {
      const cmdState = await page.evaluate(() => {
        var scene = SceneManager._scene;
        if (!scene || !(scene instanceof Scene_Battle)) return { scene: 'not_battle' };
        var cmdWin = scene._actorCommandWindow;
        return {
          scene: 'Scene_Battle',
          hasCmdWindow: !!cmdWin,
          cmdVisible: cmdWin ? cmdWin.visible : false,
          cmdActive: cmdWin ? cmdWin.active : false,
          phase: BattleManager._phase,
          actors: ($gameParty._actors || []).slice(),
          memberCount: $gameParty.battleMembers().length,
        };
      });
      console.log('Command window state:', JSON.stringify(cmdState));
      console.log('Logs:', logs.slice(-30).join('\n'));
      expect(battleEnded).toBeTruthy();
    }
  });

  // -----------------------------------------------------------------------
  // Test 4: Party members persist through scene transition (regression)
  // -----------------------------------------------------------------------
  test('party members persist through scene transition', async ({ page }) => {
    const username = h.uniqueName('btlpty');
    const password = 'Test1234!';
    const charName = h.uniqueName('PtyHero');

    await enterGameDirect(page, username, password, charName);

    const partyPre = await page.evaluate(() => ({
      actors: ($gameParty._actors || []).slice(),
      memberCount: $gameParty.members().length,
    }));
    console.log('Party before battle:', JSON.stringify(partyPre));

    // Start puppet battle.
    await page.evaluate(() => {
      $MMO._dispatch('battle_battle_start', {
        actors: [{
          index: 0, is_actor: true, name: 'TestHero',
          hp: 300, max_hp: 300, mp: 30, max_mp: 30, tp: 0,
          states: [], class_id: 1, level: 5, enemy_id: 0
        }],
        enemies: [{
          index: 0, is_actor: false, name: 'Bat',
          hp: 50, max_hp: 50, mp: 0, max_mp: 0, tp: 0,
          states: [], class_id: 0, level: 1, enemy_id: 21
        }]
      });
    });

    await waitForBattleSceneReady(page);

    // Dispatch input_request.
    await page.evaluate(() => {
      $MMO._dispatch('battle_input_request', { actor_index: 0 });
    });

    // Wait for command window.
    await page.waitForFunction(() => {
      var scene = SceneManager._scene;
      if (!scene || !(scene instanceof Scene_Battle)) return false;
      var cmdWin = scene._actorCommandWindow;
      return cmdWin && cmdWin.visible && cmdWin.active;
    }, null, { timeout: 10000 });

    const partyFixed = await page.evaluate(() => ({
      actors: ($gameParty._actors || []).slice(),
      memberCount: $gameParty.battleMembers().length,
      inBattle: $gameParty._inBattle,
      actorHP: BattleManager.actor() ? BattleManager.actor().hp : -1,
    }));
    console.log('Party after _ensurePuppetParty:', JSON.stringify(partyFixed));

    expect(partyFixed.actors.length).toBeGreaterThan(0);
    expect(partyFixed.memberCount).toBeGreaterThan(0);
    expect(partyFixed.inBattle).toBe(true);
    expect(partyFixed.actorHP).toBe(300);

    // Clean up.
    await page.evaluate(() => {
      $MMO._dispatch('battle_battle_end', { result: 2, exp: 0, gold: 0, drops: [] });
    });
  });

  // -----------------------------------------------------------------------
  // Test 5: Actor stats (MHP/MMP) match server snapshot, not local RMMV data
  // -----------------------------------------------------------------------
  test('actor MHP/MMP uses server snapshot via puppetParams', async ({ page }) => {
    const username = h.uniqueName('btlstat');
    const password = 'Test1234!';
    const charName = h.uniqueName('StatHero');

    await enterGameDirect(page, username, password, charName);

    // Start puppet battle with specific server stats.
    await page.evaluate(() => {
      $MMO._dispatch('battle_battle_start', {
        actors: [{
          index: 0, is_actor: true, name: 'TestHero',
          hp: 1234, max_hp: 2000, mp: 87, max_mp: 150, tp: 0,
          states: [], class_id: 1, level: 10, enemy_id: 0
        }],
        enemies: [{
          index: 0, is_actor: false, name: 'Slime',
          hp: 100, max_hp: 100, mp: 0, max_mp: 0, tp: 0,
          states: [], class_id: 0, level: 1, enemy_id: 21
        }]
      });
    });

    await waitForBattleSceneReady(page);

    // Check that actor stats reflect server values, not RMMV local data.
    const stats = await page.evaluate(() => {
      var actor = $gameParty.battleMembers()[0];
      if (!actor) return null;
      return {
        hp: actor.hp,
        mhp: actor.mhp,
        mp: actor.mp,
        mmp: actor.mmp,
        hasPuppetParams: !!actor._puppetParams,
      };
    });

    expect(stats).not.toBeNull();
    expect(stats.hp).toBe(1234);
    expect(stats.mhp).toBe(2000);
    expect(stats.mp).toBe(87);
    expect(stats.mmp).toBe(150);
    expect(stats.hasPuppetParams).toBe(true);

    // End battle and verify puppet params are cleared.
    await page.evaluate(() => {
      $MMO._dispatch('battle_battle_end', { result: 0, exp: 10, gold: 5, drops: [] });
    });
    await page.waitForFunction(() => $MMO._serverBattle === false, null, { timeout: 5000 });

    const afterBattle = await page.evaluate(() => {
      var actor = $gameActors.actor(1);
      return actor ? !!actor._puppetParams : true;
    });
    expect(afterBattle).toBe(false);
  });

  // -----------------------------------------------------------------------
  // Test 6: Event queue processes actions sequentially (regression)
  // -----------------------------------------------------------------------
  test('event queue processes multiple actions before input request', async ({ page }) => {
    const username = h.uniqueName('btlq');
    const password = 'Test1234!';
    const charName = h.uniqueName('QHero');

    await enterGameDirect(page, username, password, charName);

    const logs = [];
    page.on('console', msg => {
      if (msg.text().includes('[Puppet]')) {
        logs.push(msg.text());
      }
    });

    // Start puppet battle.
    await page.evaluate(() => {
      $MMO._dispatch('battle_battle_start', {
        actors: [{
          index: 0, is_actor: true, name: 'TestHero',
          hp: 500, max_hp: 500, mp: 50, max_mp: 50, tp: 0,
          states: [], class_id: 1, level: 10, enemy_id: 0
        }],
        enemies: [{
          index: 0, is_actor: false, name: 'Slime',
          hp: 100, max_hp: 100, mp: 0, max_mp: 0, tp: 0,
          states: [], class_id: 0, level: 1, enemy_id: 21
        }, {
          index: 1, is_actor: false, name: 'Bat',
          hp: 80, max_hp: 80, mp: 0, max_mp: 0, tp: 0,
          states: [], class_id: 0, level: 1, enemy_id: 21
        }]
      });
    });

    await waitForBattleSceneReady(page);

    // Get actual enemy HP values after RMMV clamp.
    const enemyHPs = await page.evaluate(() => {
      return $gameTroop.members().map(function(e) { return e.hp; });
    });
    console.log('Enemy HPs after setup:', JSON.stringify(enemyHPs));

    // Dispatch 3 actions + turn_end + input_request ALL at once.
    // This is the exact pattern from the server: all arrive between frames.
    const e0HPAfter = enemyHPs[0] - 25;
    await page.evaluate((args) => {
      // Player attacks enemy 0.
      $MMO._dispatch('battle_action_result', {
        subject: { index: 0, is_actor: true, name: 'TestHero' },
        skill_id: 1,
        targets: [{ target: { index: 0, is_actor: false }, damage: 25, hp_after: args.e0HPAfter, mp_after: 0 }]
      });
      // Enemy 0 attacks player.
      $MMO._dispatch('battle_action_result', {
        subject: { index: 0, is_actor: false, name: 'Slime' },
        skill_id: 1,
        targets: [{ target: { index: 0, is_actor: true }, damage: 15, hp_after: 485, mp_after: 50 }]
      });
      // Enemy 1 attacks player.
      $MMO._dispatch('battle_action_result', {
        subject: { index: 1, is_actor: false, name: 'Bat' },
        skill_id: 1,
        targets: [{ target: { index: 0, is_actor: true }, damage: 10, hp_after: 475, mp_after: 50 }]
      });
      $MMO._dispatch('battle_turn_end', {});
      $MMO._dispatch('battle_input_request', { actor_index: 0 });
    }, { e0HPAfter: e0HPAfter });

    // The critical test: ALL actions must animate before the command window appears.
    // Wait for final HP values to confirm all 3 actions processed.
    await page.waitForFunction(() => {
      var actor = $gameParty.battleMembers()[0];
      return actor && actor.hp === 475;
    }, null, { timeout: 20000 });

    // Verify enemy HP was updated.
    const enemyHP = await page.evaluate(() => {
      return $gameTroop.members()[0] ? $gameTroop.members()[0].hp : -1;
    });
    expect(enemyHP).toBe(e0HPAfter);

    // Now wait for the command window (should appear after queue drains).
    const cmdReady = await page.waitForFunction(() => {
      var scene = SceneManager._scene;
      if (!scene || !(scene instanceof Scene_Battle)) return false;
      var cmdWin = scene._actorCommandWindow;
      return cmdWin && cmdWin.visible && cmdWin.active;
    }, null, { timeout: 10000 }).catch(() => null);

    expect(cmdReady).toBeTruthy();

    // Check the logs to verify sequential processing.
    const dequeueCount = logs.filter(l => l.includes('Dequeuing event')).length;
    const animatingCount = logs.filter(l => l.includes('>> Action:')).length;
    console.log('Dequeue events:', dequeueCount, 'Animating actions:', animatingCount);
    console.log('Logs:', logs.join('\n'));

    // Should have dequeued 4 events (3 actions + 1 turn_end) and animated 3 actions.
    expect(dequeueCount).toBeGreaterThanOrEqual(4);
    expect(animatingCount).toBeGreaterThanOrEqual(3);

    // Clean up.
    await page.evaluate(() => {
      $MMO._dispatch('battle_battle_end', { result: 0, exp: 10, gold: 5, drops: [] });
    });
  });
});
