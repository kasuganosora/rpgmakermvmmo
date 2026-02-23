/*:
 * @plugindesc v2.0.0 MMO Plugin Loader - fetches all mmo-*.js plugins from the server.
 * @author MMO Framework
 *
 * @param ServerURL
 * @desc WebSocket server address (e.g. ws://localhost:8080)
 * @default ws://localhost:8080
 *
 * @param Debug
 * @desc Enable debug mode (true/false)
 * @default false
 *
 * @param ReconnectMax
 * @desc Maximum reconnection attempts
 * @default 10
 *
 * @help
 * This is the ONLY MMO plugin you need to register in js/plugins.js.
 * All other mmo-*.js plugins are fetched from the server at startup
 * via the /plugins/ endpoint, enabling remote updates without
 * touching the client files.
 */

(function () {
    'use strict';

    var LOAD_ORDER = [
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

    var parameters = PluginManager.parameters('mmo-loader');

    // Build MMO_CONFIG from plugin parameters.
    window.MMO_CONFIG = {
        serverUrl: parameters['ServerURL'] || 'ws://localhost:8080',
        debug: (parameters['Debug'] || 'false').toLowerCase() === 'true',
        reconnectMax: parseInt(parameters['ReconnectMax'] || '10', 10)
    };

    var httpBase = MMO_CONFIG.serverUrl.replace(/^ws/, 'http');

    console.log('[MMO] Config ready. Server: ' + MMO_CONFIG.serverUrl +
                ' Debug: ' + MMO_CONFIG.debug);

    // ---- Remote plugin loader ----
    // Fetches each plugin JS file from the server via synchronous XHR
    // and evaluates it in the global scope. Synchronous ensures correct
    // load order and that all plugins are ready before Scene_Boot starts.
    var loaded = 0;
    var failed = [];
    for (var i = 0; i < LOAD_ORDER.length; i++) {
        var filename = LOAD_ORDER[i];
        var url = httpBase + '/plugins/' + filename + '?_t=' + Date.now();
        try {
            var xhr = new XMLHttpRequest();
            xhr.open('GET', url, false); // synchronous
            xhr.send();
            if (xhr.status === 200) {
                // Indirect eval runs in global scope, matching <script> tag behavior.
                (0, eval)(xhr.responseText);
                loaded++;
                if (MMO_CONFIG.debug) {
                    console.log('[MMO] Loaded: ' + filename);
                }
            } else {
                failed.push(filename + ' (HTTP ' + xhr.status + ')');
                console.error('[MMO] Failed to load ' + filename + ': HTTP ' + xhr.status);
            }
        } catch (e) {
            failed.push(filename + ' (' + e.message + ')');
            console.error('[MMO] Error loading ' + filename + ':', e.message);
        }
    }

    console.log('[MMO] Remote load complete: ' + loaded + '/' + LOAD_ORDER.length +
                (failed.length ? ' | FAILED: ' + failed.join(', ') : ''));

    // ---- Client error reporter ----
    function reportError(message, source, line, col, stack) {
        try {
            var xhr = new XMLHttpRequest();
            xhr.open('POST', httpBase + '/api/client-error');
            xhr.setRequestHeader('Content-Type', 'application/json');
            xhr.send(JSON.stringify({
                message: String(message || ''),
                source: String(source || ''),
                line: line || 0,
                col: col || 0,
                stack: String(stack || ''),
                ua: navigator.userAgent || ''
            }));
        } catch (e) { /* ignore send failures */ }
    }

    window.onerror = function (message, source, line, col, error) {
        var stack = (error && error.stack) ? error.stack : '';
        console.error('[MMO] Uncaught error:', message, 'at', source + ':' + line + ':' + col);
        reportError(message, source, line, col, stack);
    };

    window.addEventListener('unhandledrejection', function (event) {
        var reason = event.reason || {};
        var msg = reason.message || String(reason);
        var stack = reason.stack || '';
        console.error('[MMO] Unhandled promise rejection:', msg);
        reportError('UnhandledRejection: ' + msg, '', 0, 0, stack);
    });

})();
