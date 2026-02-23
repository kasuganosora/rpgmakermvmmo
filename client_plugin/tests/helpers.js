/**
 * helpers.js  –  HTTP client, raw WebSocket client, and assertion helpers
 *               for E2E tests.
 */
'use strict';

const http = require('http');
const assert = require('assert');
const WebSocket = require('ws');

// ---- Unique ID generator ----
let _counter = 0;
function uniqueID(prefix) {
    return (prefix || 'e2e') + '_' + Date.now() + '_' + (++_counter);
}

// ---- HTTP helpers (uses Node.js built-in http) ----

function httpRequest(method, baseUrl, path, body, token) {
    return new Promise(function (resolve, reject) {
        const url = new URL(path, baseUrl);
        const opts = {
            method: method,
            hostname: url.hostname,
            port: url.port,
            path: url.pathname + url.search,
            headers: { 'Content-Type': 'application/json' }
        };
        if (token) {
            opts.headers['Authorization'] = 'Bearer ' + token;
        }
        const req = http.request(opts, function (res) {
            let data = '';
            res.on('data', function (chunk) { data += chunk; });
            res.on('end', function () {
                let parsed = null;
                try { parsed = JSON.parse(data); } catch (e) { parsed = data; }
                resolve({ status: res.statusCode, data: parsed });
            });
        });
        req.on('error', reject);
        if (body) req.write(JSON.stringify(body));
        req.end();
    });
}

function httpPost(baseUrl, path, body, token) {
    return httpRequest('POST', baseUrl, path, body, token);
}

function httpGet(baseUrl, path, token) {
    return httpRequest('GET', baseUrl, path, null, token);
}

function httpPut(baseUrl, path, body, token) {
    return httpRequest('PUT', baseUrl, path, body, token);
}

// ---- Raw WebSocket client for "player 2" ----

class RawWSClient {
    constructor() {
        this._ws = null;
        this._seq = 0;
        this._pending = [];   // [{type, resolve, timer}]
        this._messages = [];  // buffered received messages
    }

    connect(wsUrl, token) {
        return new Promise((resolve, reject) => {
            const url = wsUrl + '?token=' + encodeURIComponent(token);
            this._ws = new WebSocket(url);
            this._ws.on('open', () => resolve());
            this._ws.on('error', (e) => reject(e));
            this._ws.on('message', (raw) => {
                let msg;
                try { msg = JSON.parse(raw.toString()); } catch (e) { return; }
                // Check pending waiters
                for (let i = 0; i < this._pending.length; i++) {
                    const p = this._pending[i];
                    if (msg.type === p.type) {
                        clearTimeout(p.timer);
                        this._pending.splice(i, 1);
                        p.resolve(msg.payload || msg);
                        return;
                    }
                }
                this._messages.push(msg);
            });
        });
    }

    send(type, payload) {
        if (!this._ws || this._ws.readyState !== WebSocket.OPEN) return;
        this._ws.send(JSON.stringify({
            seq: ++this._seq,
            type: type,
            payload: payload || {}
        }));
    }

    recvType(type, timeoutMs) {
        timeoutMs = timeoutMs || 5000;
        // Check buffer first
        for (let i = 0; i < this._messages.length; i++) {
            if (this._messages[i].type === type) {
                const msg = this._messages.splice(i, 1)[0];
                return Promise.resolve(msg.payload || msg);
            }
        }
        // Wait for it
        return new Promise((resolve, reject) => {
            const timer = setTimeout(() => {
                this._pending = this._pending.filter(p => p.type !== type || p.resolve !== resolve);
                reject(new Error('Timeout waiting for message type: ' + type));
            }, timeoutMs);
            this._pending.push({ type, resolve, timer });
        });
    }

    close() {
        if (this._ws) {
            this._ws.onclose = null;
            this._ws.close();
            this._ws = null;
        }
        // Clean up pending
        this._pending.forEach(p => clearTimeout(p.timer));
        this._pending = [];
    }
}

// ---- $MMO event waiter ----

/**
 * Wait for a specific $MMO event type to fire.  Returns a Promise
 * that resolves with the payload.
 */
function waitForEvent(type, timeoutMs) {
    timeoutMs = timeoutMs || 5000;
    return new Promise(function (resolve, reject) {
        const timer = setTimeout(function () {
            $MMO.off(type, handler);
            reject(new Error('Timeout waiting for $MMO event: ' + type));
        }, timeoutMs);
        function handler(payload) {
            clearTimeout(timer);
            $MMO.off(type, handler);
            resolve(payload);
        }
        $MMO.on(type, handler);
    });
}

/**
 * Login via HTTP + set $MMO.token.
 */
async function login(httpUrl, username, password) {
    const res = await httpPost(httpUrl, '/api/auth/login', { username, password });
    assert.strictEqual(res.status, 200, 'login should return 200, got ' + res.status);
    assert.ok(res.data.token, 'login should return token');
    return { token: res.data.token, accountID: res.data.account_id };
}

/**
 * Create a character via HTTP.
 */
async function createCharacter(httpUrl, token, name) {
    const res = await httpPost(httpUrl, '/api/characters', {
        name: name,
        class_id: 1,
        walk_name: 'Actor1',
        walk_index: 0,
        face_name: 'Actor1',
        face_index: 0
    }, token);
    assert.strictEqual(res.status, 201, 'create char should return 201, got ' + res.status);
    assert.ok(res.data.id, 'create char should return id');
    return res.data.id;
}

/**
 * Connect $MMO to the WS server and wait for _connected event.
 */
function connectMMO(wsUrl, token) {
    return new Promise(function (resolve, reject) {
        const timer = setTimeout(function () {
            reject(new Error('Timeout: $MMO did not connect within 5s'));
        }, 5000);
        $MMO.on('_connected', function onConn() {
            clearTimeout(timer);
            $MMO.off('_connected', onConn);
            resolve();
        });
        // Override reconnect max to 0 so no auto-reconnect
        $MMO._reconnectMax = 0;
        $MMO._serverUrl = wsUrl.replace(/\/ws$/, '');
        $MMO.connect(token);
    });
}

/**
 * Sleep helper.
 */
function sleep(ms) {
    return new Promise(resolve => setTimeout(resolve, ms));
}

// ---- Simple test runner ----

const _tests = [];
let _beforeFn = null;
let _afterFn = null;

function before(fn) { _beforeFn = fn; }
function after(fn) { _afterFn = fn; }
function test(name, fn) { _tests.push({ name, fn }); }

async function runTests(suiteName) {
    let passed = 0;
    let failed = 0;
    console.log('\n=== ' + suiteName + ' ===');
    if (_beforeFn) await _beforeFn();
    for (const t of _tests) {
        try {
            await t.fn();
            console.log('  PASS: ' + t.name);
            passed++;
        } catch (e) {
            console.error('  FAIL: ' + t.name);
            console.error('    ' + e.message);
            if (e.stack) {
                const lines = e.stack.split('\n').slice(1, 4);
                lines.forEach(l => console.error('    ' + l.trim()));
            }
            failed++;
        }
    }
    if (_afterFn) {
        try { await _afterFn(); } catch (e) { /* ignore cleanup errors */ }
    }
    console.log('  Results: ' + passed + ' passed, ' + failed + ' failed\n');
    return failed;
}

module.exports = {
    uniqueID,
    httpPost,
    httpGet,
    httpPut,
    RawWSClient,
    waitForEvent,
    login,
    createCharacter,
    connectMMO,
    sleep,
    before,
    after,
    test,
    runTests
};
