//=============================================================================
// MMO TemplateEvent Hook Plugin
// Purpose: Intercept TemplateEvent.js calls and sync self-variables to server
//          for per-player state isolation
//=============================================================================

(function () {
    'use strict';

    //=============================================================================
    // Configuration
    //=============================================================================
    const CONFIG = {
        DEBUG: false,
        SYNC_INTERVAL: 5000,        // Sync interval for self-var changes (ms)
        BATCH_SIZE: 10              // Max changes per batch
    };

    //=============================================================================
    // State
    //=============================================================================
    let _isServerAvailable = false;
    let _templateEventDetected = false;
    let _pendingChanges = {};       // mapId_eventId_index -> value
    let _flushTimer = null;

    //=============================================================================
    // Debug Logging
    //=============================================================================
    function debugLog() {
        if (CONFIG.DEBUG) {
            console.log('[MMO-TemplateEvent-Hook]', ...arguments);
        }
    }

    //=============================================================================
    // Server Availability Check
    //=============================================================================
    function checkServerAvailability() {
        _isServerAvailable = typeof MMO !== 'undefined' &&
            MMO.SocketManager &&
            MMO.SocketManager.socket &&
            MMO.SocketManager.socket.connected;
        return _isServerAvailable;
    }

    //=============================================================================
    // Change Batching and Sync
    //=============================================================================
    function queueChange(mapId, eventId, index, value) {
        if (!checkServerAvailability()) return;

        const key = `${mapId}_${eventId}_${index}`;
        _pendingChanges[key] = { mapId, eventId, index, value };

        if (!_flushTimer) {
            _flushTimer = setTimeout(flushChanges, CONFIG.SYNC_INTERVAL);
        }
    }

    function flushChanges() {
        _flushTimer = null;

        if (!checkServerAvailability()) {
            _pendingChanges = {};
            return;
        }

        const changes = Object.values(_pendingChanges);
        _pendingChanges = {};

        if (changes.length === 0) return;

        // Send batched changes to server
        for (let i = 0; i < changes.length; i += CONFIG.BATCH_SIZE) {
            const batch = changes.slice(i, i + CONFIG.BATCH_SIZE);

            debugLog('Sending batch:', batch);

            MMO.SocketManager.send({
                type: 'self_var_set_batch',
                payload: {
                    changes: batch
                }
            });
        }
    }

    //=============================================================================
    // Hook Game_SelfSwitches
    //=============================================================================
    function hookGameSelfSwitches() {
        const originalSetValue = Game_SelfSwitches.prototype.setValue;

        Game_SelfSwitches.prototype.setValue = function (key, value) {
            const [mapId, eventId, ch] = key;

            // Check if this is a numeric self-variable (index >= 13 for TemplateEvent)
            if (typeof ch === 'number' && ch >= 13) {
                debugLog('Intercepted self-variable set:', { mapId, eventId, index: ch, value });
                queueChange(mapId, eventId, ch, value);
            }

            // Always call original
            return originalSetValue.call(this, key, value);
        };

        console.log('[MMO-TemplateEvent-Hook] Game_SelfSwitches.setValue hooked');
    }

    //=============================================================================
    // Hook TemplateEvent.js specific methods (if present)
    //=============================================================================
    function hookTemplateEvent() {
        if (typeof TemplateEvent === 'undefined') {
            console.log('[MMO-TemplateEvent-Hook] TemplateEvent not detected');
            return false;
        }

        _templateEventDetected = true;
        console.log('[MMO-TemplateEvent-Hook] TemplateEvent.js detected');

        // Hook setSelfVariable if it exists
        if (TemplateEvent.setSelfVariable) {
            const originalSetSelfVariable = TemplateEvent.setSelfVariable;
            TemplateEvent.setSelfVariable = function (eventId, mapId, index, value) {
                debugLog('TemplateEvent.setSelfVariable intercepted:', { eventId, mapId, index, value });
                queueChange(mapId, eventId, index, value);
                return originalSetSelfVariable.apply(this, arguments);
            };
        }

        return true;
    }

    //=============================================================================
    // Initialize
    //=============================================================================
    function initialize() {
        console.log('[MMO-TemplateEvent-Hook] Initializing...');

        // Wait for MMO to be ready
        const checkInterval = setInterval(function () {
            if (typeof MMO !== 'undefined' && MMO.SocketManager) {
                clearInterval(checkInterval);

                // Check server availability
                checkServerAvailability();
                console.log('[MMO-TemplateEvent-Hook] Server available:', _isServerAvailable);

                // Hook into RPG Maker MV
                if (typeof Game_SelfSwitches !== 'undefined') {
                    hookGameSelfSwitches();
                } else {
                    console.warn('[MMO-TemplateEvent-Hook] Game_SelfSwitches not found');
                }

                // Hook TemplateEvent.js if present
                hookTemplateEvent();

                // Set up sync on scene changes (flush pending changes)
                const originalTerminate = Scene_Map.prototype.terminate;
                Scene_Map.prototype.terminate = function () {
                    flushChanges();
                    return originalTerminate.call(this);
                };

                console.log('[MMO-TemplateEvent-Hook] Initialization complete');
            }
        }, 100);

        // Timeout after 30 seconds
        setTimeout(function () {
            clearInterval(checkInterval);
        }, 30000);
    }

    //=============================================================================
    // Public API
    //=============================================================================
    window.MMO_TEMPLATE_EVENT_HOOK = {
        isServerAvailable: function () { return _isServerAvailable; },
        isTemplateEventDetected: function () { return _templateEventDetected; },
        flush: flushChanges,
        getPendingCount: function () { return Object.keys(_pendingChanges).length; }
    };

    // Auto-initialize when DOM is ready
    if (document.readyState === 'loading') {
        document.addEventListener('DOMContentLoaded', initialize);
    } else {
        initialize();
    }

    console.log('[MMO-TemplateEvent-Hook] Plugin loaded');
})();
