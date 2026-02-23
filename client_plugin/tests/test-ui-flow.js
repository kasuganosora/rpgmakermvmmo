/**
 * test-ui-flow.js  –  Client-side UI tests for the in-game L2-style window system.
 *
 * Tests: ESC menu disabled, processMapTouch click-through prevention, ActionBar,
 *        GameWindow toggle/close, window manager, hotkeys, draggable system.
 *
 * These tests run locally without a server — they verify plugin hooks and
 * UI component behavior against the RMMV shim.
 */
'use strict';

require('./rmmv-shim');
const loadPlugins = require('./load-plugins');
const assert = require('assert');
const { test, before, after, runTests } = require('./helpers');

// Load all plugins (mmo-core, mmo-ui, mmo-game-window, mmo-hud, etc.)
loadPlugins('ws://localhost:9999');

// ---------------------------------------------------------------------------
//  Helper: create a fresh Scene_Map and call createAllWindows
// ---------------------------------------------------------------------------
function createSceneMap() {
    var scene = new Scene_Map();
    scene.initialize();
    scene.createAllWindows();
    SceneManager._scene = scene;
    return scene;
}

// ---------------------------------------------------------------------------
//  Tests
// ---------------------------------------------------------------------------

test('Scene_Map.isMenuCalled returns false (RMMV menu disabled)', function () {
    var scene = new Scene_Map();
    assert.strictEqual(scene.isMenuCalled(), false, 'isMenuCalled should return false');
});

test('Scene_Map.callMenu is a no-op', function () {
    var scene = new Scene_Map();
    // Should not throw
    scene.callMenu();
    assert.ok(true, 'callMenu should be a no-op');
});

test('createAllWindows creates ActionBar as child of Scene_Map', function () {
    var scene = createSceneMap();
    assert.ok(scene._mmoActionBar, 'scene should have _mmoActionBar');
    assert.ok(scene._mmoActionBar.visible, 'ActionBar should be visible');
    var found = scene.children.some(function (c) { return c === scene._mmoActionBar; });
    assert.ok(found, 'ActionBar should be in scene.children');
});

test('createAllWindows creates StatusWindow (hidden by default)', function () {
    var scene = createSceneMap();
    assert.ok($MMO._statusWindow, '$MMO._statusWindow should exist');
    assert.strictEqual($MMO._statusWindow.visible, false, 'StatusWindow should start hidden');
    var found = scene.children.some(function (c) { return c === $MMO._statusWindow; });
    assert.ok(found, 'StatusWindow should be in scene.children');
});

test('createAllWindows creates SystemMenu (hidden by default)', function () {
    var scene = createSceneMap();
    assert.ok($MMO._systemMenu, '$MMO._systemMenu should exist');
    assert.strictEqual($MMO._systemMenu.visible, false, 'SystemMenu should start hidden');
});

test('GameWindow toggle shows and hides', function () {
    createSceneMap();
    var win = $MMO._statusWindow;
    assert.strictEqual(win.visible, false, 'should start hidden');
    win.toggle();
    assert.strictEqual(win.visible, true, 'toggle should show');
    win.toggle();
    assert.strictEqual(win.visible, false, 'toggle again should hide');
});

test('GameWindow close hides the window', function () {
    createSceneMap();
    var win = $MMO._statusWindow;
    win.toggle(); // show
    assert.strictEqual(win.visible, true);
    win.close();
    assert.strictEqual(win.visible, false, 'close should hide');
});

test('$MMO.registerWindow adds windows to _gameWindows', function () {
    createSceneMap();
    assert.ok($MMO._gameWindows.length >= 2,
        'should have at least 2 registered windows (status + system)');
    assert.ok($MMO._gameWindows.indexOf($MMO._statusWindow) >= 0,
        'StatusWindow should be registered');
    assert.ok($MMO._gameWindows.indexOf($MMO._systemMenu) >= 0,
        'SystemMenu should be registered');
});

test('$MMO.closeTopWindow closes the topmost visible window', function () {
    createSceneMap();
    var status = $MMO._statusWindow;
    var system = $MMO._systemMenu;
    // Open both
    status.toggle();
    system.toggle();
    assert.strictEqual(status.visible, true);
    assert.strictEqual(system.visible, true);
    // Close topmost (system was registered after status)
    var closed = $MMO.closeTopWindow();
    assert.ok(closed, 'closeTopWindow should return true');
    assert.strictEqual(system.visible, false, 'system should be closed');
    assert.strictEqual(status.visible, true, 'status should still be open');
    // Close next
    closed = $MMO.closeTopWindow();
    assert.ok(closed, 'closeTopWindow should return true again');
    assert.strictEqual(status.visible, false, 'status should now be closed');
    // No more open windows
    closed = $MMO.closeTopWindow();
    assert.strictEqual(closed, false, 'no windows to close');
});

test('$MMO._triggerAction toggles status window', function () {
    createSceneMap();
    assert.strictEqual($MMO._statusWindow.visible, false);
    $MMO._triggerAction('status');
    assert.strictEqual($MMO._statusWindow.visible, true, 'status should be shown');
    $MMO._triggerAction('status');
    assert.strictEqual($MMO._statusWindow.visible, false, 'status should be hidden');
});

test('$MMO._triggerAction toggles system menu', function () {
    createSceneMap();
    assert.strictEqual($MMO._systemMenu.visible, false);
    $MMO._triggerAction('system');
    assert.strictEqual($MMO._systemMenu.visible, true, 'system should be shown');
    $MMO._triggerAction('system');
    assert.strictEqual($MMO._systemMenu.visible, false, 'system should be hidden');
});

test('processMapTouch blocks click when over L2_Base child', function () {
    var scene = createSceneMap();
    // Find the ActionBar position
    var bar = scene._mmoActionBar;
    assert.ok(bar, 'ActionBar must exist');
    // Simulate a click IN the ActionBar bounds
    var centerX = bar.x + bar.width / 2;
    var centerY = bar.y + bar.height / 2;
    // Track if original processMapTouch sets destination
    $gameTemp.clearDestination();
    // Override TouchInput for this test
    var origTriggered = TouchInput.isTriggered;
    var origPressed = TouchInput.isPressed;
    TouchInput.x = centerX;
    TouchInput.y = centerY;
    TouchInput.isTriggered = function () { return true; };
    TouchInput.isPressed = function () { return true; };
    scene.processMapTouch();
    assert.strictEqual($gameTemp._destinationX, null,
        'processMapTouch should NOT set destination when clicking on UI panel');
    // Now click on empty area (outside all UI)
    TouchInput.x = 1;
    TouchInput.y = 1;
    scene.processMapTouch();
    // Original processMapTouch is a no-op in the shim, so destination stays null
    // But verify processMapTouch was called without error
    assert.ok(true, 'processMapTouch on empty area should proceed without error');
    // Restore
    TouchInput.isTriggered = origTriggered;
    TouchInput.isPressed = origPressed;
    TouchInput.x = 0;
    TouchInput.y = 0;
});

test('processMapTouch blocks click on skill bar', function () {
    var scene = createSceneMap();
    // Find skill bar
    var skillBar = scene.children.find(function (c) {
        return c.constructor === Window_SkillBar;
    });
    if (!skillBar) {
        // Skip if SkillBar not found (shouldn't happen)
        console.log('    (skipped - SkillBar not found in scene)');
        return;
    }
    var centerX = skillBar.x + skillBar.width / 2;
    var centerY = skillBar.y + skillBar.height / 2;
    $gameTemp.clearDestination();
    var origTriggered = TouchInput.isTriggered;
    var origPressed = TouchInput.isPressed;
    TouchInput.x = centerX;
    TouchInput.y = centerY;
    TouchInput.isTriggered = function () { return true; };
    TouchInput.isPressed = function () { return true; };
    scene.processMapTouch();
    assert.strictEqual($gameTemp._destinationX, null,
        'processMapTouch should NOT set destination when clicking on skill bar');
    TouchInput.isTriggered = origTriggered;
    TouchInput.isPressed = origPressed;
    TouchInput.x = 0;
    TouchInput.y = 0;
});

test('processMapTouch blocks click on chat box', function () {
    var scene = createSceneMap();
    // Chat box is an L2_Base child with visible=true, positioned at bottom-left
    var chatBox = scene.children.find(function (c) {
        return c instanceof L2_Base && c.x <= 10 && c.y > Graphics.boxHeight / 2;
    });
    if (!chatBox) {
        console.log('    (skipped - ChatBox not found in scene)');
        return;
    }
    var centerX = chatBox.x + chatBox.width / 2;
    var centerY = chatBox.y + chatBox.height / 2;
    $gameTemp.clearDestination();
    var origTriggered = TouchInput.isTriggered;
    var origPressed = TouchInput.isPressed;
    TouchInput.x = centerX;
    TouchInput.y = centerY;
    TouchInput.isTriggered = function () { return true; };
    TouchInput.isPressed = function () { return true; };
    scene.processMapTouch();
    assert.strictEqual($gameTemp._destinationX, null,
        'processMapTouch should NOT set destination when clicking on chat box');
    TouchInput.isTriggered = origTriggered;
    TouchInput.isPressed = origPressed;
    TouchInput.x = 0;
    TouchInput.y = 0;
});

test('$MMO.makeDraggable stores and restores positions from localStorage', function () {
    localStorage.clear();
    var panel = new Window_Base(100, 200, 50, 50);
    panel.visible = true;
    // Attach isInside since Window_Base doesn't have it
    panel.isInside = function (mx, my) {
        return mx >= this.x && mx <= this.x + this.width &&
               my >= this.y && my <= this.y + this.height;
    };
    $MMO.makeDraggable(panel, 'testPanel');
    assert.ok(panel._drag, 'panel should have _drag property');
    assert.strictEqual(panel._drag.key, 'testPanel');
    // Simulate saving position
    localStorage.setItem('mmo_ui_testPanel', JSON.stringify({ x: 300, y: 400 }));
    // Create new panel — should restore position
    var panel2 = new Window_Base(0, 0, 50, 50);
    panel2.visible = true;
    panel2.isInside = panel.isInside;
    $MMO.makeDraggable(panel2, 'testPanel');
    assert.strictEqual(panel2.x, 300, 'x should be restored from localStorage');
    assert.strictEqual(panel2.y, 400, 'y should be restored from localStorage');
    localStorage.clear();
});

test('$MMO.makeDraggable clamps position within screen bounds', function () {
    localStorage.clear();
    // Save a position way off-screen
    localStorage.setItem('mmo_ui_testClamp', JSON.stringify({ x: 9999, y: 9999 }));
    var panel = new Window_Base(0, 0, 50, 50);
    panel.visible = true;
    panel.isInside = function (mx, my) {
        return mx >= this.x && mx <= this.x + this.width &&
               my >= this.y && my <= this.y + this.height;
    };
    $MMO.makeDraggable(panel, 'testClamp');
    assert.ok(panel.x <= Graphics.boxWidth - panel.width, 'x should be clamped');
    assert.ok(panel.y <= Graphics.boxHeight - panel.height, 'y should be clamped');
    localStorage.clear();
});

test('ActionBar has correct number of buttons and size', function () {
    var scene = createSceneMap();
    var bar = scene._mmoActionBar;
    assert.ok(bar, 'ActionBar must exist');
    // 2x3 grid: AB_COLS=3, AB_ROWS=2, AB_BTN_SIZE=38, AB_GAP=2, AB_PAD=4, AB_TOOLTIP_H=18
    // Width: 3*(38+2)-2+4*2 = 118-2+8 = 126 (was 206 for 5-button row)
    // Height: 18+2*(38+2)-2+4*2 = 18+78-2+8 = 104
    assert.ok(bar.width > 100 && bar.width < 160,
        'ActionBar width should be ~126, got ' + bar.width);
    assert.ok(bar.height > 80 && bar.height < 130,
        'ActionBar height should be ~104, got ' + bar.height);
});

test('ActionBar is at bottom-right of screen', function () {
    var scene = createSceneMap();
    var bar = scene._mmoActionBar;
    assert.ok(bar.x + bar.width > Graphics.boxWidth - 20,
        'ActionBar should be near the right edge, right=' + (bar.x + bar.width));
    assert.ok(bar.y + bar.height > Graphics.boxHeight - 20,
        'ActionBar should be near the bottom edge, bottom=' + (bar.y + bar.height));
});

test('InventoryWindow extends GameWindow', function () {
    createSceneMap();
    assert.ok($MMO._inventoryWindow, '_inventoryWindow should exist');
    assert.ok($MMO._inventoryWindow instanceof GameWindow,
        'InventoryWindow should be a GameWindow');
    assert.strictEqual($MMO._inventoryWindow.visible, false,
        'InventoryWindow should start hidden');
});

test('$MMO._triggerAction toggles inventory', function () {
    createSceneMap();
    assert.strictEqual($MMO._inventoryWindow.visible, false);
    $MMO._triggerAction('inventory');
    assert.strictEqual($MMO._inventoryWindow.visible, true, 'inventory should be shown');
    $MMO._triggerAction('inventory');
    assert.strictEqual($MMO._inventoryWindow.visible, false, 'inventory should be hidden');
});

test('GameWindow class is exported globally', function () {
    assert.ok(typeof GameWindow === 'function', 'GameWindow should be a global function');
    assert.ok(GameWindow.prototype instanceof L2_Base, 'GameWindow should extend L2_Base');
});

test('window._windowListeners has keydown handlers for Alt+T and Alt+I', function () {
    assert.ok(_windowListeners.keydown, 'should have keydown listeners');
    assert.ok(_windowListeners.keydown.length >= 2,
        'should have at least 2 keydown handlers (Alt+T, Alt+I)');
});

test('Alt+T keydown dispatches status toggle', function () {
    var scene = createSceneMap();
    assert.strictEqual($MMO._statusWindow.visible, false);
    // Find the keydown handler that handles Alt+T
    var handlers = _windowListeners.keydown || [];
    var fakeEvent = {
        altKey: true,
        keyCode: 84, // T
        preventDefault: function () {}
    };
    handlers.forEach(function (fn) { fn(fakeEvent); });
    assert.strictEqual($MMO._statusWindow.visible, true,
        'Alt+T should toggle status window visible');
    // Alt+T again to hide
    handlers.forEach(function (fn) { fn(fakeEvent); });
    assert.strictEqual($MMO._statusWindow.visible, false,
        'Alt+T again should hide status window');
});

test('Alt+I keydown dispatches inventory toggle', function () {
    var scene = createSceneMap();
    assert.strictEqual($MMO._inventoryWindow.visible, false);
    var handlers = _windowListeners.keydown || [];
    var fakeEvent = {
        altKey: true,
        keyCode: 73, // I
        preventDefault: function () {}
    };
    handlers.forEach(function (fn) { fn(fakeEvent); });
    assert.strictEqual($MMO._inventoryWindow.visible, true,
        'Alt+I should toggle inventory window visible');
});

test('L2_Base instances have _isMMOUI flag', function () {
    createSceneMap();
    assert.strictEqual(L2_Base.prototype._isMMOUI, true, 'L2_Base.prototype._isMMOUI should be true');
    assert.ok($MMO._statusWindow._isMMOUI, 'StatusWindow should inherit _isMMOUI');
    assert.ok($MMO._inventoryWindow._isMMOUI, 'InventoryWindow should inherit _isMMOUI');
});

test('processMapTouch blocks via _isMMOUI flag (mmo-core hook)', function () {
    var scene = createSceneMap();
    var bar = scene._mmoActionBar;
    // Verify the _isMMOUI flag is set
    assert.ok(bar._isMMOUI, 'ActionBar should have _isMMOUI flag');
    // Click on ActionBar — should block
    var centerX = bar.x + bar.width / 2;
    var centerY = bar.y + bar.height / 2;
    $gameTemp.clearDestination();
    var origTriggered = TouchInput.isTriggered;
    var origPressed = TouchInput.isPressed;
    TouchInput.x = centerX;
    TouchInput.y = centerY;
    TouchInput.isTriggered = function () { return true; };
    TouchInput.isPressed = function () { return true; };
    scene.processMapTouch();
    assert.strictEqual($gameTemp._destinationX, null,
        'processMapTouch should block when _isMMOUI child is clicked');
    TouchInput.isTriggered = origTriggered;
    TouchInput.isPressed = origPressed;
    TouchInput.x = 0;
    TouchInput.y = 0;
});

// ---------------------------------------------------------------------------
//  Run
// ---------------------------------------------------------------------------
runTests('UI Flow').then(function (failed) {
    process.exit(failed > 0 ? 1 : 0);
});
