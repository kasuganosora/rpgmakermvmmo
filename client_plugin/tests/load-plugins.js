/**
 * load-plugins.js  –  Loads actual mmo-*.js plugin files via vm.runInThisContext.
 *
 * Usage:
 *   require('./rmmv-shim');          // set up globals first
 *   require('./load-plugins')(wsUrl); // then load all plugins
 */
'use strict';

const vm = require('vm');
const fs = require('fs');
const path = require('path');

const LOAD_ORDER = [
    'mmo-core.js',
    'mmo-ui.js',
    'mmo-game-window.js',
    'mmo-auth.js',
    'mmo-other-players.js',
    'mmo-battle.js',
    'mmo-hud.js',
    'mmo-skill-bar.js',
    'mmo-inventory.js',
    'mmo-chat.js',
    'mmo-party.js',
    'mmo-social.js',
    'mmo-trade.js'
];

module.exports = function loadPlugins(wsUrl) {
    // Strip the /ws path from the URL since mmo-core.js appends it when connecting.
    // mmo-auth.js converts ws:// → http:// for REST calls, so the base must NOT include /ws.
    var baseUrl = (wsUrl || 'ws://localhost:8080').replace(/\/ws\/?$/, '');

    // Set MMO_CONFIG before loading plugins (mmo-core.js reads it).
    globalThis.MMO_CONFIG = {
        serverUrl: baseUrl,
        debug: false,
        reconnectMax: 0  // disable auto-reconnect in tests
    };

    const pluginDir = path.join(__dirname, '..');

    for (const filename of LOAD_ORDER) {
        const filePath = path.join(pluginDir, filename);
        if (!fs.existsSync(filePath)) {
            console.warn('[load-plugins] Skipping missing: ' + filename);
            continue;
        }
        const code = fs.readFileSync(filePath, 'utf8');
        vm.runInThisContext(code, { filename: filename });
    }
};
