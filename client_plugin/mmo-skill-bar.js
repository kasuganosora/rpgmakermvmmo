/*:
 * @plugindesc v2.0.0 MMO Skill Bar - 12-slot F1-F12 skill quick bar (L2 UI).
 * @author MMO Framework
 */

(function () {
    'use strict';

    var SLOT_COUNT = 12;
    var SLOT_SIZE = 34;
    var SLOT_GAP = 2;
    var ICON_COLS = 16;  // IconSet.png has 16 columns
    var PAD = 4;

    $MMO._knownSkills = []; // all skills the character has learned
    $MMO._skillBar = Array(SLOT_COUNT).fill(null);  // [{skill_id, icon_index, name, mp_cost, cd_ms}]
    $MMO._skillCDs = {};   // skill_id → readyAt (performance.now() based)
    $MMO._playerMP = 0;
    $MMO._playerMaxMP = 1;

    // -----------------------------------------------------------------
    // SkillBar — L2_Base component
    // -----------------------------------------------------------------
    function SkillBar() { this.initialize.apply(this, arguments); }
    SkillBar.prototype = Object.create(L2_Base.prototype);
    SkillBar.prototype.constructor = SkillBar;

    SkillBar.prototype.initialize = function () {
        var w = SLOT_COUNT * (SLOT_SIZE + SLOT_GAP) + PAD * 2 - SLOT_GAP;
        var h = SLOT_SIZE + PAD * 2;
        var x = Math.floor((Graphics.boxWidth - w) / 2);
        var y = Graphics.boxHeight - h - 4;
        L2_Base.prototype.initialize.call(this, x, y, w, h);
        this._iconSet = null;
        ImageManager.loadSystem('IconSet').addLoadListener(function (bmp) {
            this._iconSet = bmp;
            this.refresh();
        }.bind(this));
        $MMO.makeDraggable(this, 'skillBar');
        this.refresh();
    };

    SkillBar.prototype.standardPadding = function () { return 0; };

    SkillBar.prototype.refresh = function () {
        var c = this.bmp();
        c.clear();
        var w = this.cw(), h = this.ch();

        // Background
        L2_Theme.fillRoundRect(c, 0, 0, w, h, L2_Theme.cornerRadius, 'rgba(13,13,26,0.70)');
        L2_Theme.strokeRoundRect(c, 0, 0, w, h, L2_Theme.cornerRadius, L2_Theme.borderDark);

        // Detect drag-hover slot
        var dragSlot = -1;
        if ($MMO._uiDrag) {
            dragSlot = $MMO._getBarSlotAt($MMO._uiDrag.x, $MMO._uiDrag.y);
        }

        for (var i = 0; i < SLOT_COUNT; i++) {
            this._drawSlot(c, i, i === dragSlot);
        }
    };

    SkillBar.prototype._drawSlot = function (c, idx, dropHighlight) {
        var x = PAD + idx * (SLOT_SIZE + SLOT_GAP);
        var y = PAD;
        var skill = $MMO._skillBar[idx];
        var slotW = SLOT_SIZE, slotH = SLOT_SIZE;

        // Slot background (gold highlight when drag-hovering)
        c.fillRect(x, y, slotW, slotH, dropHighlight ? '#2A2A4E' : (skill ? '#1A1A2E' : '#111118'));
        // Slot border
        var borderCol = dropHighlight ? L2_Theme.textGold : (skill ? L2_Theme.borderDark : '#1a1a2a');
        L2_Theme.strokeRoundRect(c, x, y, slotW, slotH, 2, borderCol);

        // Icon (scale 32→26 to fit smaller slot)
        if (skill && this._iconSet) {
            var iconIdx = skill.icon_index || 0;
            var sx = (iconIdx % ICON_COLS) * 32;
            var sy = Math.floor(iconIdx / ICON_COLS) * 32;
            c.blt(this._iconSet, sx, sy, 32, 32, x + 4, y + 2, 26, 26);
        }

        // Hotkey label
        c.fontSize = 9;
        c.textColor = L2_Theme.textGray;
        c.drawText('F' + (idx + 1), x, y + slotH - 12, slotW - 2, 10, 'right');

        // Grey out if insufficient MP
        if (skill && $MMO._playerMP < skill.mp_cost) {
            c.fillRect(x + 1, y + 1, slotW - 2, slotH - 2, 'rgba(0,0,0,0.55)');
        }

        // Cooldown overlay
        if (skill) {
            var cdRemain = this._getCDRemain(skill.skill_id);
            if (cdRemain > 0) {
                var total = skill.cd_ms || 1000;
                var ratio = cdRemain / total;
                var cdH = Math.round(slotH * ratio);
                c.fillRect(x + 1, y + 1, slotW - 2, cdH, 'rgba(0,0,80,0.60)');
                c.fontSize = 10;
                c.textColor = '#AADDFF';
                c.drawText(Math.ceil(cdRemain / 1000) + 's', x, y + 10, slotW, 14, 'center');
            }
        }
    };

    SkillBar.prototype._getCDRemain = function (skillID) {
        var readyAt = $MMO._skillCDs[skillID];
        if (!readyAt) return 0;
        return Math.max(0, readyAt - performance.now());
    };

    SkillBar.prototype.update = function () {
        L2_Base.prototype.update.call(this);
        $MMO.updateDrag(this);
        if (Graphics.frameCount % 6 === 0) this.refresh();
    };

    // -----------------------------------------------------------------
    // F1-F12 keydown listener
    // -----------------------------------------------------------------
    window.addEventListener('keydown', function (e) {
        var fKey = e.keyCode - 111; // F1=112→1, F12=123→12
        if (fKey < 1 || fKey > 12) return;
        e.preventDefault();
        var idx = fKey - 1;
        var entry = $MMO._skillBar[idx];
        if (!entry) return;

        if (entry.skill_id) {
            // Skill — MP and CD check
            if ($MMO._playerMP < entry.mp_cost) return;
            var readyAt = $MMO._skillCDs[entry.skill_id];
            if (readyAt && performance.now() < readyAt) return;
            $MMO.send('player_skill', { skill_id: entry.skill_id });
        } else if (entry.item_id) {
            // Consumable item — send use_item with inventory ID
            $MMO.send('use_item', { inv_id: entry.inv_id });
        }
    });

    // -----------------------------------------------------------------
    // Inject into Scene_Map
    // -----------------------------------------------------------------
    var _Scene_Map_createAllWindows2 = Scene_Map.prototype.createAllWindows;
    Scene_Map.prototype.createAllWindows = function () {
        _Scene_Map_createAllWindows2.call(this);
        this._mmoSkillBar = new SkillBar();
        this.addChild(this._mmoSkillBar);
        $MMO.registerBottomUI(this._mmoSkillBar);
    };

    // -----------------------------------------------------------------
    // WebSocket handlers
    // -----------------------------------------------------------------
    $MMO.on('map_init', function (data) {
        if (data.self) {
            $MMO._playerMP = data.self.mp || 0;
            $MMO._playerMaxMP = data.self.max_mp || 1;
        }
        if (data.skills) {
            $MMO._knownSkills = data.skills;
            // Auto-assign known skills to empty bar slots on first load.
            // Preserve existing assignments (from localStorage) if any.
            var saved = null;
            try { saved = JSON.parse(localStorage.getItem('mmo_skillbar_' + $MMO.charID)); } catch (e) {}
            if (saved && Array.isArray(saved)) {
                // Restore saved bar layout — match by skill_id.
                for (var i = 0; i < SLOT_COUNT; i++) {
                    if (saved[i]) {
                        var found = data.skills.find(function (s) { return s.skill_id === saved[i]; });
                        $MMO._skillBar[i] = found || null;
                    } else {
                        $MMO._skillBar[i] = null;
                    }
                }
            } else {
                // First time: fill bar with known skills in order.
                $MMO._skillBar = Array(SLOT_COUNT).fill(null);
                data.skills.forEach(function (sk, i) {
                    if (i < SLOT_COUNT) $MMO._skillBar[i] = sk;
                });
            }
            $MMO._saveSkillBar();
        }
    });

    // Persist skill bar assignment to localStorage.
    $MMO._saveSkillBar = function () {
        try {
            var ids = $MMO._skillBar.map(function (s) { return s ? s.skill_id : null; });
            localStorage.setItem('mmo_skillbar_' + $MMO.charID, JSON.stringify(ids));
        } catch (e) {}
    };

    // Assign a skill or item to a bar slot.
    $MMO.assignToBar = function (slotIdx, data) {
        if (slotIdx < 0 || slotIdx >= SLOT_COUNT) return;
        $MMO._skillBar[slotIdx] = data;
        $MMO._saveSkillBar();
    };

    $MMO.on('player_sync', function (data) {
        if (data.char_id !== $MMO.charID) return;
        if (data.mp !== undefined) $MMO._playerMP = data.mp;
        if (data.max_mp !== undefined) $MMO._playerMaxMP = data.max_mp;
    });

    $MMO.on('skill_effect', function (data) {
        if (data.char_id !== $MMO.charID) return;
        if (data.skill_id && data.cd_remaining_ms) {
            // Use monotonic performance.now() — immune to system clock changes
            $MMO._skillCDs[data.skill_id] = performance.now() + data.cd_remaining_ms;
        }
    });

    // -----------------------------------------------------------------
    // Drag-and-drop system for assigning skills/items to bar slots.
    // $MMO._uiDrag is set by SkillWindow or InventoryWindow during drag.
    // -----------------------------------------------------------------
    $MMO._uiDrag = null; // { type: 'skill'|'item', data: {...}, x, y }

    // Get which bar slot index a screen coordinate falls on, or -1.
    $MMO._getBarSlotAt = function (sx, sy) {
        var scene = SceneManager._scene;
        if (!scene || !scene._mmoSkillBar) return -1;
        var bar = scene._mmoSkillBar;
        var lx = sx - bar.x, ly = sy - bar.y;
        if (ly < PAD || ly > PAD + SLOT_SIZE) return -1;
        if (lx < PAD) return -1;
        var idx = Math.floor((lx - PAD) / (SLOT_SIZE + SLOT_GAP));
        // Check that we're within the slot, not in the gap
        var inSlotX = (lx - PAD) - idx * (SLOT_SIZE + SLOT_GAP);
        if (idx < 0 || idx >= SLOT_COUNT || inSlotX > SLOT_SIZE) return -1;
        return idx;
    };

    // Handle drop from skill/item drag onto bar.
    $MMO._handleDrop = function (sx, sy) {
        if (!$MMO._uiDrag) return;
        var slotIdx = $MMO._getBarSlotAt(sx, sy);
        if (slotIdx < 0) { $MMO._uiDrag = null; return; }
        $MMO.assignToBar(slotIdx, $MMO._uiDrag.data);
        $MMO._uiDrag = null;
    };

    // Draw drag cursor overlay on Scene_Map update.
    var _Scene_Map_update_skillbar = Scene_Map.prototype.update;
    Scene_Map.prototype.update = function () {
        _Scene_Map_update_skillbar.call(this);
        // Draw drag cursor when dragging a skill/item.
        if ($MMO._uiDrag && this._mmoSkillBar && this._mmoSkillBar._iconSet) {
            var drag = $MMO._uiDrag;
            var iconIdx = drag.data.icon_index || 0;
            if (iconIdx > 0) {
                // Highlight the target bar slot if hovering over one
                var slotIdx = $MMO._getBarSlotAt(drag.x, drag.y);
                if (slotIdx >= 0) {
                    this._mmoSkillBar.refresh(); // re-render to show highlight
                }
            }
        }
    };

    window.Window_SkillBar = SkillBar;

})();
