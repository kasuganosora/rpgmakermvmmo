/**
 * @plugindesc v1.0.0 MMO Debug - client log forwarding to server.
 * @author MMO Framework
 *
 * @help
 * Hooks console.log / console.warn / console.error and forwards
 * [MMO] / [NPCManager] prefixed messages to the server via WebSocket.
 * Server prints them with zap alongside its own logs.
 *
 * Enable/disable at runtime:
 *   $MMO.debugLog = true;   // start forwarding
 *   $MMO.debugLog = false;  // stop forwarding
 *
 * Or press F9 in-game to toggle.
 */
(function () {
    'use strict';

    // Only activate when MMO_CONFIG.debug is true.
    if (!window.MMO_CONFIG || !MMO_CONFIG.debug) return;

    // ---------------------------------------------------------------
    //  State
    // ---------------------------------------------------------------
    var _enabled = true;        // forwarding on by default in debug mode
    var _buffer = [];           // batch buffer
    var _flushTimer = null;
    var FLUSH_INTERVAL = 200;   // ms — batch window
    var MAX_BUFFER = 50;        // max entries per flush
    var PREFIXES = ['[MMO', '[NPCManager', '[Sprite_ServerNPC'];

    // Expose toggle
    Object.defineProperty($MMO, 'debugLog', {
        get: function () { return _enabled; },
        set: function (v) {
            _enabled = !!v;
            console.log('[MMO-Debug] forwarding ' + (_enabled ? 'ON' : 'OFF'));
        }
    });

    // ---------------------------------------------------------------
    //  Console hooks
    // ---------------------------------------------------------------
    function shouldForward(args) {
        if (!_enabled) return false;
        if (!args.length) return false;
        var first = String(args[0]);
        for (var i = 0; i < PREFIXES.length; i++) {
            if (first.indexOf(PREFIXES[i]) >= 0) return true;
        }
        return false;
    }

    function argsToString(args) {
        var parts = [];
        for (var i = 0; i < args.length; i++) {
            var a = args[i];
            if (a === null) { parts.push('null'); continue; }
            if (a === undefined) { parts.push('undefined'); continue; }
            if (typeof a === 'object') {
                try { parts.push(JSON.stringify(a)); }
                catch (e) { parts.push(String(a)); }
            } else {
                parts.push(String(a));
            }
        }
        return parts.join(' ');
    }

    function enqueue(level, args) {
        if (!shouldForward(args)) return;
        _buffer.push({
            t: Date.now(),
            l: level,
            m: argsToString(args)
        });
        if (_buffer.length >= MAX_BUFFER) flush();
        else if (!_flushTimer) {
            _flushTimer = setTimeout(flush, FLUSH_INTERVAL);
        }
    }

    function flush() {
        if (_flushTimer) { clearTimeout(_flushTimer); _flushTimer = null; }
        if (!_buffer.length) return;
        if (!$MMO || !$MMO.isConnected || !$MMO.isConnected()) {
            _buffer = []; // drop if not connected
            return;
        }
        $MMO.send('client_log', { entries: _buffer });
        _buffer = [];
    }

    // Hook console methods
    var _origLog = console.log;
    var _origWarn = console.warn;
    var _origError = console.error;

    console.log = function () {
        _origLog.apply(console, arguments);
        enqueue('INFO', Array.prototype.slice.call(arguments));
    };
    console.warn = function () {
        _origWarn.apply(console, arguments);
        enqueue('WARN', Array.prototype.slice.call(arguments));
    };
    console.error = function () {
        _origError.apply(console, arguments);
        enqueue('ERROR', Array.prototype.slice.call(arguments));
    };

    // ---------------------------------------------------------------
    //  F9 toggle
    // ---------------------------------------------------------------
    document.addEventListener('keydown', function (e) {
        if (e.key === 'F9' || e.keyCode === 120) {
            $MMO.debugLog = !$MMO.debugLog;
        }
    });

    // Flush on disconnect
    $MMO.on('_disconnected', function () {
        _buffer = [];
        if (_flushTimer) { clearTimeout(_flushTimer); _flushTimer = null; }
    });

    console.log('[MMO-Debug] client log forwarding enabled (F9 to toggle)');
})();
