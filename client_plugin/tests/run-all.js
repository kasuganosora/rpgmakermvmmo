#!/usr/bin/env node
/**
 * run-all.js  –  Discovers and runs all test-*.js files.
 *
 * Usage:  node run-all.js <HTTP_URL> <WS_URL>
 * Example: node run-all.js http://127.0.0.1:54321 ws://127.0.0.1:54321/ws
 *
 * Each test file runs in a separate child process for global isolation
 * (since plugins modify globalThis).
 */
'use strict';

const { execFileSync } = require('child_process');
const fs = require('fs');
const path = require('path');

const HTTP_URL = process.argv[2];
const WS_URL = process.argv[3];

if (!HTTP_URL || !WS_URL) {
    console.error('Usage: node run-all.js <HTTP_URL> <WS_URL>');
    process.exit(1);
}

// Discover test files
const testDir = __dirname;
const testFiles = fs.readdirSync(testDir)
    .filter(f => f.startsWith('test-') && f.endsWith('.js'))
    .sort();

console.log('E2E Test Runner: ' + testFiles.length + ' test file(s) found');
console.log('Server: ' + HTTP_URL);
console.log('WebSocket: ' + WS_URL);

let totalPassed = 0;
let totalFailed = 0;

for (const file of testFiles) {
    const filePath = path.join(testDir, file);
    try {
        const output = execFileSync(process.execPath, [filePath], {
            env: {
                ...process.env,
                TEST_HTTP_URL: HTTP_URL,
                TEST_WS_URL: WS_URL
            },
            timeout: 30000,
            encoding: 'utf8',
            stdio: ['pipe', 'pipe', 'pipe']
        });
        // Parse results from output
        process.stdout.write(output);
        const match = output.match(/(\d+) passed, (\d+) failed/);
        if (match) {
            totalPassed += parseInt(match[1]);
            totalFailed += parseInt(match[2]);
        }
    } catch (e) {
        console.error('FATAL: ' + file + ' crashed');
        if (e.stdout) process.stdout.write(e.stdout);
        if (e.stderr) process.stderr.write(e.stderr);
        totalFailed++;
    }
}

console.log('\n========================================');
console.log('Total: ' + totalPassed + ' passed, ' + totalFailed + ' failed');
console.log('========================================');

process.exit(totalFailed > 0 ? 1 : 0);
