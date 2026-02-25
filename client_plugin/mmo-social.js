/*:
 * @plugindesc v2.0.0 MMO Social - friends list and guild panel (L2 UI).
 * @author MMO Framework
 */

(function () {
    'use strict';

    $MMO._friends = [];
    $MMO._guildData = null;

    var PANEL_W = 300, PANEL_H = 360, PAD = 8;
    var HEADER_H = 28, ITEM_H = 26;

    // -----------------------------------------------------------------
    // FriendListPanel — L2_Base with auto-centering
    // -----------------------------------------------------------------
    function FriendListPanel() { this.initialize.apply(this, arguments); }
    FriendListPanel.prototype = Object.create(L2_Base.prototype);
    FriendListPanel.prototype.constructor = FriendListPanel;

    FriendListPanel.prototype.initialize = function () {
        L2_Base.prototype.initialize.call(this,
            (Graphics.boxWidth - PANEL_W) / 2, (Graphics.boxHeight - PANEL_H) / 2,
            PANEL_W, PANEL_H);
        this.visible = false;
        this._friends = [];
        this._scrollY = 0;
        this._closeHover = false;
        
        // Enable auto-centering on resize
        this._isCentered = true;
    };

    FriendListPanel.prototype.standardPadding = function () { return 0; };

    FriendListPanel.prototype.loadFriends = function () {
        var self = this;
        $MMO.http.get('/api/social/friends').then(function (data) {
            self._friends = (data.friends || []).sort(function (a, b) {
                return (b.online ? 1 : 0) - (a.online ? 1 : 0);
            });
            $MMO._friends = self._friends;
            self._scrollY = 0;
            self.refresh();
        }).catch(function (e) { console.error('[mmo-social] loadFriends:', e.message); });
    };

    FriendListPanel.prototype.refresh = function () {
        var c = this.bmp();
        c.clear();
        var w = this.cw(), h = this.ch();

        // Background
        L2_Theme.fillRoundRect(c, 0, 0, w, h, L2_Theme.cornerRadius, 'rgba(13,13,26,0.85)');
        L2_Theme.strokeRoundRect(c, 0, 0, w, h, L2_Theme.cornerRadius, L2_Theme.borderDark);

        // Header
        c.fontSize = 14;
        c.textColor = L2_Theme.textGold;
        c.drawText('好友 (' + this._friends.length + ')', PAD, PAD, w - PAD * 2 - 20, 18, 'left');

        // Close button
        L2_Theme.drawCloseBtn(c, w - 22, PAD, this._closeHover);

        // Separator
        c.fillRect(PAD, HEADER_H + PAD, w - PAD * 2, 1, L2_Theme.borderDark);

        // Friend list
        var listY = HEADER_H + PAD + 4;
        var listH = h - listY - PAD;
        var friends = this._friends;
        var startIdx = Math.floor(this._scrollY / ITEM_H);
        var visCount = Math.ceil(listH / ITEM_H) + 1;

        for (var i = startIdx; i < Math.min(startIdx + visCount, friends.length); i++) {
            var iy = listY + (i * ITEM_H - this._scrollY);
            if (iy + ITEM_H < listY || iy > h - PAD) continue;

            var f = friends[i];
            // Online dot
            var dotColor = f.online ? '#44FF44' : '#555555';
            c.fillRect(PAD + 2, iy + 8, 8, 8, dotColor);

            // Name + Level
            c.fontSize = 13;
            c.textColor = f.online ? L2_Theme.textWhite : L2_Theme.textDim;
            c.drawText((f.name || '?') + '  Lv.' + (f.level || '?'), PAD + 16, iy, 200, ITEM_H, 'left');

            // Map name
            if (f.map_name && f.online) {
                c.fontSize = 11;
                c.textColor = L2_Theme.textGray;
                c.drawText(f.map_name, w - PAD - 80, iy, 76, ITEM_H, 'right');
            }

            // Row separator
            c.fillRect(PAD, iy + ITEM_H - 1, w - PAD * 2, 1, L2_Theme.borderDark);
        }

        // Scrollbar
        var totalH = friends.length * ITEM_H;
        if (totalH > listH) {
            var sbW = 4;
            var trackH = listH;
            var thumbH = Math.max(12, Math.round(trackH * (listH / totalH)));
            var thumbY = listY + Math.round((trackH - thumbH) * (this._scrollY / (totalH - listH)));
            c.fillRect(w - sbW, listY, sbW, listH, 'rgba(0,0,0,0.2)');
            L2_Theme.fillRoundRect(c, w - sbW, thumbY, sbW, thumbH, 2, '#444466');
        }
    };

    FriendListPanel.prototype.updateFriendStatus = function (charID, online) {
        this._friends.forEach(function (f) {
            if (f.char_id === charID) f.online = online;
        });
        if (this.visible) this.refresh();
    };

    FriendListPanel.prototype.update = function () {
        L2_Base.prototype.update.call(this);
        if (!this.visible) return;

        var mx = TouchInput.x - this.x, my = TouchInput.y - this.y;
        var w = this.cw();

        // Close button hover
        var wasHover = this._closeHover;
        this._closeHover = mx >= w - 26 && mx <= w - 4 && my >= PAD - 4 && my <= PAD + 20;
        if (this._closeHover !== wasHover) this.refresh();

        if (TouchInput.isTriggered()) {
            if (this._closeHover) { this.visible = false; return; }
        }

        // Scroll
        if (this.isInside(TouchInput.x, TouchInput.y) && TouchInput.wheelY) {
            var listH = this.ch() - HEADER_H - PAD - 4 - PAD;
            var totalH = this._friends.length * ITEM_H;
            this._scrollY += TouchInput.wheelY > 0 ? ITEM_H * 2 : -ITEM_H * 2;
            this._scrollY = Math.max(0, Math.min(this._scrollY, Math.max(0, totalH - listH)));
            this.refresh();
        }
    };

    // -----------------------------------------------------------------
    // GuildInfoPanel — L2_Base with auto-centering
    // -----------------------------------------------------------------
    var GUILD_W = 340, GUILD_H = 380;

    function GuildInfoPanel() { this.initialize.apply(this, arguments); }
    GuildInfoPanel.prototype = Object.create(L2_Base.prototype);
    GuildInfoPanel.prototype.constructor = GuildInfoPanel;

    GuildInfoPanel.prototype.initialize = function () {
        L2_Base.prototype.initialize.call(this,
            (Graphics.boxWidth - GUILD_W) / 2, (Graphics.boxHeight - GUILD_H) / 2,
            GUILD_W, GUILD_H);
        this.visible = false;
        this._data = null;
        this._closeHover = false;
        
        // Enable auto-centering on resize
        this._isCentered = true;
    };

    GuildInfoPanel.prototype.standardPadding = function () { return 0; };

    GuildInfoPanel.prototype.loadGuild = function (guildID) {
        var self = this;
        $MMO.http.get('/api/guilds/' + guildID).then(function (data) {
            self._data = data;
            $MMO._guildData = data;
            self.refresh();
        }).catch(function (e) { console.error('[mmo-social] loadGuild:', e.message); });
    };

    GuildInfoPanel.prototype.refresh = function () {
        var c = this.bmp();
        c.clear();
        var w = this.cw(), h = this.ch();

        // Background
        L2_Theme.fillRoundRect(c, 0, 0, w, h, L2_Theme.cornerRadius, 'rgba(13,13,26,0.85)');
        L2_Theme.strokeRoundRect(c, 0, 0, w, h, L2_Theme.cornerRadius, L2_Theme.borderDark);

        // Close button
        L2_Theme.drawCloseBtn(c, w - 22, PAD, this._closeHover);

        if (!this._data) {
            c.fontSize = 14;
            c.textColor = L2_Theme.textGray;
            c.drawText('未加入公会', 0, h / 2 - 10, w, 20, 'center');
            return;
        }

        var g = this._data.guild || this._data;
        var members = this._data.members || [];

        // Guild name + level
        c.fontSize = 16;
        c.textColor = L2_Theme.textGold;
        c.drawText((g.name || '') + '  Lv.' + (g.level || 1), PAD, PAD, w - PAD * 2 - 20, 22, 'left');

        // Gold
        c.fontSize = 12;
        c.textColor = L2_Theme.textGray;
        c.drawText('公会资金: ' + (g.gold || 0), PAD, 34, w - PAD * 2, 16, 'left');

        // Notice
        c.fontSize = 12;
        c.textColor = L2_Theme.textWhite;
        var notice = (g.notice || '暂无公告').substring(0, 80);
        c.drawText(notice, PAD, 54, w - PAD * 2, 16, 'left');

        // Separator
        c.fillRect(PAD, 76, w - PAD * 2, 1, L2_Theme.borderDark);

        // Members header
        c.fontSize = 13;
        c.textColor = L2_Theme.textGold;
        c.drawText('成员 (' + members.length + ')', PAD, 82, w - PAD * 2, 18, 'left');

        // Member list
        members.slice(0, 10).forEach(function (m, i) {
            var y = 104 + i * 24;
            if (y + 24 > h - PAD) return;

            // Online dot
            c.fillRect(PAD + 2, y + 7, 6, 6, m.online ? '#44FF44' : '#555555');

            c.fontSize = 13;
            c.textColor = m.online ? L2_Theme.textWhite : L2_Theme.textDim;
            var rankLabel = ['', '会长', '副会长', '成员'][m.rank] || '成员';
            c.drawText((m.name || '?') + '  [' + rankLabel + ']  Lv.' + (m.level || '?'),
                PAD + 14, y, w - PAD * 2 - 14, 20, 'left');
        });
    };

    GuildInfoPanel.prototype.update = function () {
        L2_Base.prototype.update.call(this);
        if (!this.visible) return;

        var mx = TouchInput.x - this.x, my = TouchInput.y - this.y;
        var w = this.cw();

        var wasHover = this._closeHover;
        this._closeHover = mx >= w - 26 && mx <= w - 4 && my >= PAD - 4 && my <= PAD + 20;
        if (this._closeHover !== wasHover) this.refresh();

        if (TouchInput.isTriggered() && this._closeHover) {
            this.visible = false;
        }
    };

    // -----------------------------------------------------------------
    // Friend request notification — uses L2_Notification
    // -----------------------------------------------------------------
    $MMO._showToast = function (msg, options) {
        options = options || {};
        if (options.onAccept) {
            // Interactive notification with accept action
            var notif = L2_Notification.show({
                title: '好友请求',
                content: msg,
                type: 'info',
                duration: (options.timeout || 5000) / 1000 * 60, // ms to frames
                closable: true,
                onClose: function () {}
            });
            // Accept on click for now (L2_Notification doesn't have custom buttons)
            // The user can accept via the friend list panel
            if (options.onAccept) {
                var origDismiss = notif._dismiss.bind(notif);
                notif._dismiss = function () {
                    options.onAccept();
                    origDismiss();
                };
            }
        } else {
            L2_Notification.info('提示', msg, (options.timeout || 5000) / 1000 * 60);
        }
    };

    // -----------------------------------------------------------------
    // Inject into Scene_Map
    // -----------------------------------------------------------------
    var _Scene_Map_createAllWindows4 = Scene_Map.prototype.createAllWindows;
    Scene_Map.prototype.createAllWindows = function () {
        _Scene_Map_createAllWindows4.call(this);
        this._friendList = new FriendListPanel();
        this._guildInfo = new GuildInfoPanel();
        this.addChild(this._friendList);
        this.addChild(this._guildInfo);
        $MMO._friendListWin = this._friendList;
        $MMO._guildInfoWin = this._guildInfo;
    };

    // -----------------------------------------------------------------
    // Alt+F and Alt+G hotkeys
    // -----------------------------------------------------------------
    window.addEventListener('keydown', function (e) {
        if (e.altKey && e.keyCode === 70) { // Alt+F
            e.preventDefault();
            var win = $MMO._friendListWin;
            if (!win) return;
            win.visible = !win.visible;
            if (win.visible) {
                win.refresh();
                win.loadFriends();
            }
        }
        if (e.altKey && e.keyCode === 71) { // Alt+G
            e.preventDefault();
            var gwin = $MMO._guildInfoWin;
            if (!gwin) return;
            gwin.visible = !gwin.visible;
            if (gwin.visible) {
                gwin.refresh();
                if ($MMO._guildID) gwin.loadGuild($MMO._guildID);
            }
        }
    });

    // -----------------------------------------------------------------
    // WebSocket handlers
    // -----------------------------------------------------------------
    $MMO.on('friend_request', function (data) {
        var dlg = new L2_Dialog({
            title: '好友请求',
            content: (data.from_name || '?') + ' 想加你为好友',
            closable: true,
            buttons: [
                {
                    text: '接受', type: 'primary',
                    onClick: function () {
                        $MMO.http.post('/api/social/friends/accept/' + data.from_id, {});
                        dlg.close();
                    }
                },
                {
                    text: '拒绝', type: 'danger',
                    onClick: function () { dlg.close(); }
                }
            ]
        });
        if (SceneManager._scene) SceneManager._scene.addChild(dlg);
    });

    $MMO.on('friend_online', function (data) {
        if ($MMO._friendListWin) $MMO._friendListWin.updateFriendStatus(data.char_id, true);
    });

    $MMO.on('friend_offline', function (data) {
        if ($MMO._friendListWin) $MMO._friendListWin.updateFriendStatus(data.char_id, false);
    });

    $MMO.on('map_init', function (data) {
        if (data.self && data.self.guild_id) $MMO._guildID = data.self.guild_id;
    });

    window.Window_FriendList = FriendListPanel;
    window.Window_GuildInfo = GuildInfoPanel;

})();
