/*:
 * @plugindesc v2.0.0 MMO Party - party panel with HP/MP bars (L2 UI).
 * @author MMO Framework
 */

(function () {
    'use strict';

    $MMO._partyData = null;

    // -----------------------------------------------------------------
    // PartyPanel — compact L2_Base panel, auto-resizes to member count
    // -----------------------------------------------------------------
    var HEADER_H = 22;
    var MEMBER_H = 38;
    var PAD = 6;
    var PANEL_W = 200;

    function PartyPanel() { this.initialize.apply(this, arguments); }
    PartyPanel.prototype = Object.create(L2_Base.prototype);
    PartyPanel.prototype.constructor = PartyPanel;

    PartyPanel.prototype.initialize = function () {
        L2_Base.prototype.initialize.call(this, 4, 0, PANEL_W, HEADER_H + PAD * 2);
        this.visible = false;
        this._members = [];
    };

    PartyPanel.prototype.standardPadding = function () { return 0; };

    PartyPanel.prototype.setData = function (data) {
        this._members = data.members || [];
        var contentH = HEADER_H + this._members.length * MEMBER_H;
        var newH = contentH + PAD * 2;
        if (this.height !== newH) {
            this.move(this.x, (Graphics.boxHeight - newH) / 2, PANEL_W, newH);
            this._refreshBitmap();
        }
        this.refresh();
    };

    PartyPanel.prototype._refreshBitmap = function () {
        if (this.bitmap) this.bitmap = null;
        this.bitmap = new Bitmap(PANEL_W, this.height);
    };

    PartyPanel.prototype.refresh = function () {
        var c = this.bmp();
        c.clear();
        var w = this.cw(), h = this.ch();

        // Background
        L2_Theme.fillRoundRect(c, 0, 0, w, h, L2_Theme.cornerRadius, 'rgba(13,13,26,0.65)');
        L2_Theme.strokeRoundRect(c, 0, 0, w, h, L2_Theme.cornerRadius, L2_Theme.borderDark);

        // Header
        c.fontSize = L2_Theme.fontSmall;
        c.textColor = L2_Theme.textGold;
        c.drawText('Party [' + this._members.length + ']', PAD, PAD, w - PAD * 2, 16, 'left');

        var barW = w - PAD * 2;
        var self = this;
        this._members.forEach(function (m, i) {
            var y = PAD + HEADER_H + i * MEMBER_H;
            var offline = !m.online;
            var diffMap = m.map_id !== undefined && $gameMap && m.map_id !== $gameMap.mapId();

            // Name
            c.fontSize = L2_Theme.fontSmall;
            c.textColor = offline ? L2_Theme.textDim : L2_Theme.textWhite;
            c.drawText(m.name || '?', PAD, y, barW - 40, 14, 'left');

            // Status tag
            if (offline) {
                c.textColor = L2_Theme.textDim;
                c.fontSize = 10;
                c.drawText('[Offline]', barW - 36, y, 40, 14, 'right');
            } else if (diffMap) {
                c.textColor = L2_Theme.textGray;
                c.fontSize = 10;
                c.drawText('[Away]', barW - 36, y, 40, 14, 'right');
            }

            // HP bar
            var dim = offline || diffMap;
            var hpRatio = m.max_hp > 0 ? Math.min(m.hp / m.max_hp, 1) : 0;
            L2_Theme.drawBar(c, PAD, y + 16, barW, 8,
                hpRatio, dim ? '#222' : L2_Theme.hpBg, dim ? '#446644' : L2_Theme.hpFill);

            // MP bar
            var mpRatio = m.max_mp > 0 ? Math.min(m.mp / m.max_mp, 1) : 0;
            L2_Theme.drawBar(c, PAD, y + 26, barW, 6,
                mpRatio, dim ? '#222' : L2_Theme.mpBg, dim ? '#444488' : L2_Theme.mpFill);
        });
    };

    // -----------------------------------------------------------------
    // Invite dialog — single L2_Dialog with countdown (auto-centered)
    // -----------------------------------------------------------------
    var _inviteDialog = null;
    var _inviteTimer = null;

    function showInviteDialog(data) {
        if (_inviteDialog) return;

        var countdown = 30;
        var dlg = new L2_Dialog({
            title: 'Party Invite',
            content: (data.from_name || '?') + ' invites you to a party.\n' +
                     countdown + 's to auto-decline.',
            closable: false,
            buttons: [
                {
                    text: 'Accept', type: 'primary',
                    onClick: function () { respond(true); }
                },
                {
                    text: 'Decline', type: 'danger',
                    onClick: function () { respond(false); }
                }
            ]
        });

        var scene = SceneManager._scene;
        if (scene) scene.addChild(dlg);

        _inviteTimer = setInterval(function () {
            countdown--;
            if (countdown <= 0) { respond(false); return; }
            // Update content text with new countdown
            dlg._content = (data.from_name || '?') + ' invites you to a party.\n' +
                           countdown + 's to auto-decline.';
            dlg._contentLines = dlg._wrapText(dlg._content, dlg.width - 40);
            dlg.refresh();
        }, 1000);

        function respond(accept) {
            clearInterval(_inviteTimer);
            _inviteTimer = null;
            dlg.close();
            _inviteDialog = null;
            $MMO.send('party_invite_response', { accept: accept, from_id: data.from_id });
        }

        _inviteDialog = { respond: respond };
    }

    // -----------------------------------------------------------------
    // Inject into Scene_Map
    // -----------------------------------------------------------------
    var _Scene_Map_createAllWindows3 = Scene_Map.prototype.createAllWindows;
    Scene_Map.prototype.createAllWindows = function () {
        _Scene_Map_createAllWindows3.call(this);
        this._partyPanel = new PartyPanel();
        this.addChild(this._partyPanel);
        $MMO._partyPanel = this._partyPanel;
    };

    // No more manual click detection needed — L2_Dialog handles it internally.

    // -----------------------------------------------------------------
    // WebSocket handlers
    // -----------------------------------------------------------------
    $MMO.on('party_update', function (data) {
        $MMO._partyData = data;
        if (!$MMO._partyPanel) return;
        if (!data.members || data.members.length === 0) {
            $MMO._partyPanel.visible = false;
            return;
        }
        $MMO._partyPanel.setData(data);
        $MMO._partyPanel.visible = true;
    });

    $MMO.on('party_invite_request', function (data) {
        showInviteDialog(data);
    });

    var _Scene_Map_terminate_party = Scene_Map.prototype.terminate;
    Scene_Map.prototype.terminate = function () {
        _Scene_Map_terminate_party.call(this);
        if (_inviteTimer) { clearInterval(_inviteTimer); _inviteTimer = null; }
        _inviteDialog = null;
    };

    $MMO.on('_disconnected', function () {
        $MMO._partyData = null;
        if ($MMO._partyPanel) $MMO._partyPanel.visible = false;
        // Clean up invite dialog without sending (WS already closed)
        if (_inviteDialog) {
            if (_inviteTimer) { clearInterval(_inviteTimer); _inviteTimer = null; }
            _inviteDialog = null;
        }
    });

    window.PartyPanel = PartyPanel;

})();
