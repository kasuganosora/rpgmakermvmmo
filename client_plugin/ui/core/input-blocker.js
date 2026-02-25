/**
 * L2_InputBlocker - Prevents UI events from propagating to the game engine.
 * 
 * This module intercepts game-level input when UI components are visible,
 * preventing the player character from moving when clicking on UI elements.
 * UI components continue to receive events normally.
 */
(function () {
    'use strict';

    var _registeredUI = [];
    var _isHooked = false;

    function L2_InputBlocker() {
        throw new Error('L2_InputBlocker is a static class');
    }

    /**
     * Register a UI component to block input events.
     * @param {L2_Base} uiComponent - The UI component to register
     */
    L2_InputBlocker.register = function (uiComponent) {
        if (!uiComponent || _registeredUI.indexOf(uiComponent) >= 0) return;
        _registeredUI.push(uiComponent);
        _ensureHooked();
    };

    /**
     * Unregister a UI component.
     * @param {L2_Base} uiComponent - The UI component to unregister
     */
    L2_InputBlocker.unregister = function (uiComponent) {
        var idx = _registeredUI.indexOf(uiComponent);
        if (idx >= 0) {
            _registeredUI.splice(idx, 1);
        }
    };

    /**
     * Check if any registered UI is visible and blocking at the given coordinates.
     * @param {number} x - Screen X coordinate
     * @param {number} y - Screen Y coordinate
     * @returns {boolean} True if input should be blocked
     */
    L2_InputBlocker.isBlocking = function (x, y) {
        for (var i = _registeredUI.length - 1; i >= 0; i--) {
            var ui = _registeredUI[i];
            if (ui && ui.visible && !ui._destroyed && ui.isInside && ui.isInside(x, y)) {
                return true;
            }
        }
        return false;
    };

    /**
     * Check if any UI is currently visible (regardless of mouse position).
     * @returns {boolean}
     */
    L2_InputBlocker.hasVisibleUI = function () {
        for (var i = 0; i < _registeredUI.length; i++) {
            var ui = _registeredUI[i];
            if (ui && ui.visible && !ui._destroyed) {
                return true;
            }
        }
        return false;
    };

    /**
     * Clear all registered UI components.
     */
    L2_InputBlocker.clear = function () {
        _registeredUI = [];
    };

    /**
     * Hook game methods to intercept events.
     * Only blocks game actions, does not interfere with UI.
     * @private
     */
    function _ensureHooked() {
        if (_isHooked) return;
        _isHooked = true;

        // Wait for RMMV to initialize
        if (typeof TouchInput === 'undefined' || typeof Scene_Map === 'undefined') {
            setTimeout(_ensureHooked, 100);
            return;
        }

        // Hook Scene_Map.prototype.processMapTouch
        // This is where the game processes clicks on the map
        if (Scene_Map.prototype.processMapTouch) {
            var _processMapTouch = Scene_Map.prototype.processMapTouch;
            Scene_Map.prototype.processMapTouch = function () {
                // Only block if clicking on UI
                if (TouchInput.isTriggered() && L2_InputBlocker.isBlocking(TouchInput.x, TouchInput.y)) {
                    // Consume the trigger but don't process map touch
                    return;
                }
                _processMapTouch.call(this);
            };
        }

        // Hook Game_Player.prototype.moveByInput
        // This is where player movement from input is processed
        if (Game_Player.prototype.moveByInput) {
            var _moveByInput = Game_Player.prototype.moveByInput;
            Game_Player.prototype.moveByInput = function () {
                // Check if there's a pending click on UI - only if game is initialized
                if (typeof TouchInput !== 'undefined' && TouchInput && 
                    TouchInput.isTriggered() && L2_InputBlocker.isBlocking(TouchInput.x, TouchInput.y)) {
                    // Don't move player
                    return;
                }
                _moveByInput.call(this);
            };
        }

        // Hook Game_Temp.prototype.setDestination
        // This sets the target position for player movement
        if (Game_Temp.prototype.setDestination) {
            var _setDestination = Game_Temp.prototype.setDestination;
            Game_Temp.prototype.setDestination = function (x, y) {
                // Check if clicking on UI - only if game is fully initialized
                if (typeof $gameMap !== 'undefined' && $gameMap && 
                    typeof $gamePlayer !== 'undefined' && $gamePlayer &&
                    typeof TouchInput !== 'undefined' && TouchInput.x !== undefined) {
                    if (L2_InputBlocker.isBlocking(TouchInput.x, TouchInput.y)) {
                        // Don't set destination if clicking on UI
                        return;
                    }
                }
                _setDestination.call(this, x, y);
            };
        }
    }

    window.L2_InputBlocker = L2_InputBlocker;
})();
