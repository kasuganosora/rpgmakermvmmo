/*:
 * @plugindesc v2.0.0 MMO 社交系统 — 好友列表与公会面板（L2 UI）。
 * @author MMO Framework
 *
 * @help
 * 本插件实现好友列表和公会信息两个面板：
 *
 * 好友列表（Alt+F）：
 * - 按在线状态排序（在线优先）
 * - 在线绿点 / 离线灰点
 * - 显示等级和所在地图
 * - 滚轮滚动长列表
 * - 通过 REST API 加载好友列表
 *
 * 公会面板（Alt+G）：
 * - 显示公会名称、等级、资金、公告
 * - 成员列表（最多 10 人，含在线状态和职位）
 * - 职位：会长/副会长/成员
 * - 通过 REST API 加载公会数据
 *
 * 好友请求：
 * - L2_Dialog 弹窗确认接受/拒绝
 * - 实时好友上下线通知
 *
 * 全局引用：
 * - window.Window_FriendList — FriendListPanel 构造函数
 * - window.Window_GuildInfo — GuildInfoPanel 构造函数
 * - $MMO._friends — 好友列表数据
 * - $MMO._guildData — 公会数据
 * - $MMO._showToast — 通知弹出方法
 *
 * WebSocket 消息：
 * - friend_request — 好友请求
 * - friend_online / friend_offline — 好友上下线
 * - map_init — 获取公会 ID
 */

(function () {
    'use strict';

    // ═══════════════════════════════════════════════════════════
    //  全局状态
    // ═══════════════════════════════════════════════════════════
    /** @type {Array} 好友列表数据。 */
    $MMO._friends = [];
    /** @type {Object|null} 公会数据。 */
    $MMO._guildData = null;

    // ═══════════════════════════════════════════════════════════
    //  常量配置
    // ═══════════════════════════════════════════════════════════
    /** @type {number} 面板宽度（好友列表）。 */
    var PANEL_W = 300;
    /** @type {number} 面板高度（好友列表）。 */
    var PANEL_H = 360;
    /** @type {number} 内边距。 */
    var PAD = 8;
    /** @type {number} 标题栏高度。 */
    var HEADER_H = 28;
    /** @type {number} 每行好友高度。 */
    var ITEM_H = 26;

    // ═══════════════════════════════════════════════════════════
    //  FriendListPanel — 继承 L2_Base 的好友列表面板
    //  居中显示，支持滚动、关闭按钮、在线状态标记。
    // ═══════════════════════════════════════════════════════════

    /**
     * 好友列表面板构造函数。
     * @constructor
     */
    function FriendListPanel() { this.initialize.apply(this, arguments); }
    FriendListPanel.prototype = Object.create(L2_Base.prototype);
    FriendListPanel.prototype.constructor = FriendListPanel;

    /**
     * 初始化面板：居中放置，默认隐藏，启用自动居中。
     */
    FriendListPanel.prototype.initialize = function () {
        L2_Base.prototype.initialize.call(this,
            (Graphics.boxWidth - PANEL_W) / 2, (Graphics.boxHeight - PANEL_H) / 2,
            PANEL_W, PANEL_H);
        this.visible = false;
        /** @type {Array} 好友数据列表。 */
        this._friends = [];
        /** @type {number} 列表滚动偏移量。 */
        this._scrollY = 0;
        /** @type {boolean} 关闭按钮是否悬停。 */
        this._closeHover = false;

        // 启用窗口缩放时自动居中。
        this._isCentered = true;
    };

    /**
     * 禁用标准内边距。
     * @returns {number} 0
     */
    FriendListPanel.prototype.standardPadding = function () { return 0; };

    /**
     * 通过 REST API 加载好友列表。
     * 按在线状态排序（在线优先），加载完成后刷新显示。
     */
    FriendListPanel.prototype.loadFriends = function () {
        var self = this;
        $MMO.http.get('/api/social/friends').then(function (data) {
            self._friends = (data.friends || []).sort(function (a, b) {
                return (b.online ? 1 : 0) - (a.online ? 1 : 0);
            });
            $MMO._friends = self._friends;
            self._scrollY = 0;
            self.refresh();
        }).catch(function (e) { console.error('[mmo-social] 加载好友列表失败:', e.message); });
    };

    /**
     * 重绘好友列表：背景、标题、好友行（在线点/名称/等级/地图）、滚动条。
     */
    FriendListPanel.prototype.refresh = function () {
        var c = this.bmp();
        c.clear();
        var w = this.cw(), h = this.ch();

        // 绘制半透明圆角背景。
        L2_Theme.fillRoundRect(c, 0, 0, w, h, L2_Theme.cornerRadius, 'rgba(13,13,26,0.85)');
        L2_Theme.strokeRoundRect(c, 0, 0, w, h, L2_Theme.cornerRadius, L2_Theme.borderDark);

        // 标题：好友 (数量)。
        c.fontSize = 14;
        c.textColor = L2_Theme.textGold;
        c.drawText('好友 (' + this._friends.length + ')', PAD, PAD, w - PAD * 2 - 20, 18, 'left');

        // 关闭按钮。
        L2_Theme.drawCloseBtn(c, w - 22, PAD, this._closeHover);

        // 分隔线。
        c.fillRect(PAD, HEADER_H + PAD, w - PAD * 2, 1, L2_Theme.borderDark);

        // 好友列表（仅绘制可见范围）。
        var listY = HEADER_H + PAD + 4;
        var listH = h - listY - PAD;
        var friends = this._friends;
        var startIdx = Math.floor(this._scrollY / ITEM_H);
        var visCount = Math.ceil(listH / ITEM_H) + 1;

        for (var i = startIdx; i < Math.min(startIdx + visCount, friends.length); i++) {
            var iy = listY + (i * ITEM_H - this._scrollY);
            if (iy + ITEM_H < listY || iy > h - PAD) continue;

            var f = friends[i];
            // 在线状态指示点（绿色在线/灰色离线）。
            var dotColor = f.online ? '#44FF44' : '#555555';
            c.fillRect(PAD + 2, iy + 8, 8, 8, dotColor);

            // 名称 + 等级。
            c.fontSize = 13;
            c.textColor = f.online ? L2_Theme.textWhite : L2_Theme.textDim;
            c.drawText((f.name || '?') + '  Lv.' + (f.level || '?'), PAD + 16, iy, 200, ITEM_H, 'left');

            // 所在地图名称（仅在线时显示）。
            if (f.map_name && f.online) {
                c.fontSize = 11;
                c.textColor = L2_Theme.textGray;
                c.drawText(f.map_name, w - PAD - 80, iy, 76, ITEM_H, 'right');
            }

            // 行分隔线。
            c.fillRect(PAD, iy + ITEM_H - 1, w - PAD * 2, 1, L2_Theme.borderDark);
        }

        // 滚动条（列表内容超过可视区域时显示）。
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

    /**
     * 更新指定好友的在线状态。
     * @param {number} charID - 角色 ID
     * @param {boolean} online - 是否在线
     */
    FriendListPanel.prototype.updateFriendStatus = function (charID, online) {
        this._friends.forEach(function (f) {
            if (f.char_id === charID) f.online = online;
        });
        if (this.visible) this.refresh();
    };

    /**
     * 每帧更新：关闭按钮悬停、点击关闭、滚轮滚动。
     */
    FriendListPanel.prototype.update = function () {
        L2_Base.prototype.update.call(this);
        if (!this.visible) return;

        var mx = TouchInput.x - this.x, my = TouchInput.y - this.y;
        var w = this.cw();

        // 关闭按钮悬停检测。
        var wasHover = this._closeHover;
        this._closeHover = mx >= w - 26 && mx <= w - 4 && my >= PAD - 4 && my <= PAD + 20;
        if (this._closeHover !== wasHover) this.refresh();

        // 点击关闭按钮。
        if (TouchInput.isTriggered()) {
            if (this._closeHover) { this.visible = false; return; }
        }

        // 滚轮滚动。
        if (this.isInside(TouchInput.x, TouchInput.y) && TouchInput.wheelY) {
            var listH = this.ch() - HEADER_H - PAD - 4 - PAD;
            var totalH = this._friends.length * ITEM_H;
            this._scrollY += TouchInput.wheelY > 0 ? ITEM_H * 2 : -ITEM_H * 2;
            this._scrollY = Math.max(0, Math.min(this._scrollY, Math.max(0, totalH - listH)));
            this.refresh();
        }
    };

    // ═══════════════════════════════════════════════════════════
    //  GuildInfoPanel — 继承 L2_Base 的公会信息面板
    //  居中显示公会名称、等级、资金、公告、成员列表。
    // ═══════════════════════════════════════════════════════════
    /** @type {number} 公会面板宽度。 */
    var GUILD_W = 340;
    /** @type {number} 公会面板高度。 */
    var GUILD_H = 380;

    /**
     * 公会信息面板构造函数。
     * @constructor
     */
    function GuildInfoPanel() { this.initialize.apply(this, arguments); }
    GuildInfoPanel.prototype = Object.create(L2_Base.prototype);
    GuildInfoPanel.prototype.constructor = GuildInfoPanel;

    /**
     * 初始化面板：居中放置，默认隐藏，启用自动居中。
     */
    GuildInfoPanel.prototype.initialize = function () {
        L2_Base.prototype.initialize.call(this,
            (Graphics.boxWidth - GUILD_W) / 2, (Graphics.boxHeight - GUILD_H) / 2,
            GUILD_W, GUILD_H);
        this.visible = false;
        /** @type {Object|null} 公会数据。 */
        this._data = null;
        /** @type {boolean} 关闭按钮是否悬停。 */
        this._closeHover = false;

        // 启用窗口缩放时自动居中。
        this._isCentered = true;
    };

    /**
     * 禁用标准内边距。
     * @returns {number} 0
     */
    GuildInfoPanel.prototype.standardPadding = function () { return 0; };

    /**
     * 通过 REST API 加载公会数据。
     * @param {number} guildID - 公会 ID
     */
    GuildInfoPanel.prototype.loadGuild = function (guildID) {
        var self = this;
        $MMO.http.get('/api/guilds/' + guildID).then(function (data) {
            self._data = data;
            $MMO._guildData = data;
            self.refresh();
        }).catch(function (e) { console.error('[mmo-social] 加载公会数据失败:', e.message); });
    };

    /**
     * 重绘公会面板：背景、关闭按钮、公会信息、成员列表。
     * 无公会时显示"未加入公会"提示。
     */
    GuildInfoPanel.prototype.refresh = function () {
        var c = this.bmp();
        c.clear();
        var w = this.cw(), h = this.ch();

        // 绘制半透明圆角背景。
        L2_Theme.fillRoundRect(c, 0, 0, w, h, L2_Theme.cornerRadius, 'rgba(13,13,26,0.85)');
        L2_Theme.strokeRoundRect(c, 0, 0, w, h, L2_Theme.cornerRadius, L2_Theme.borderDark);

        // 关闭按钮。
        L2_Theme.drawCloseBtn(c, w - 22, PAD, this._closeHover);

        // 无公会数据时显示提示。
        if (!this._data) {
            c.fontSize = 14;
            c.textColor = L2_Theme.textGray;
            c.drawText('未加入公会', 0, h / 2 - 10, w, 20, 'center');
            return;
        }

        var g = this._data.guild || this._data;
        var members = this._data.members || [];

        // 公会名称 + 等级。
        c.fontSize = 16;
        c.textColor = L2_Theme.textGold;
        c.drawText((g.name || '') + '  Lv.' + (g.level || 1), PAD, PAD, w - PAD * 2 - 20, 22, 'left');

        // 公会资金。
        c.fontSize = 12;
        c.textColor = L2_Theme.textGray;
        c.drawText('公会资金: ' + (g.gold || 0), PAD, 34, w - PAD * 2, 16, 'left');

        // 公会公告（最多显示 80 字符）。
        c.fontSize = 12;
        c.textColor = L2_Theme.textWhite;
        var notice = (g.notice || '暂无公告').substring(0, 80);
        c.drawText(notice, PAD, 54, w - PAD * 2, 16, 'left');

        // 分隔线。
        c.fillRect(PAD, 76, w - PAD * 2, 1, L2_Theme.borderDark);

        // 成员列表标题。
        c.fontSize = 13;
        c.textColor = L2_Theme.textGold;
        c.drawText('成员 (' + members.length + ')', PAD, 82, w - PAD * 2, 18, 'left');

        // 成员列表（最多显示 10 人）。
        members.slice(0, 10).forEach(function (m, i) {
            var y = 104 + i * 24;
            if (y + 24 > h - PAD) return;

            // 在线状态指示点。
            c.fillRect(PAD + 2, y + 7, 6, 6, m.online ? '#44FF44' : '#555555');

            // 名称 + 职位 + 等级。
            c.fontSize = 13;
            c.textColor = m.online ? L2_Theme.textWhite : L2_Theme.textDim;
            var rankLabel = ['', '会长', '副会长', '成员'][m.rank] || '成员';
            c.drawText((m.name || '?') + '  [' + rankLabel + ']  Lv.' + (m.level || '?'),
                PAD + 14, y, w - PAD * 2 - 14, 20, 'left');
        });
    };

    /**
     * 每帧更新：关闭按钮悬停、点击关闭。
     */
    GuildInfoPanel.prototype.update = function () {
        L2_Base.prototype.update.call(this);
        if (!this.visible) return;

        var mx = TouchInput.x - this.x, my = TouchInput.y - this.y;
        var w = this.cw();

        // 关闭按钮悬停检测。
        var wasHover = this._closeHover;
        this._closeHover = mx >= w - 26 && mx <= w - 4 && my >= PAD - 4 && my <= PAD + 20;
        if (this._closeHover !== wasHover) this.refresh();

        // 点击关闭按钮。
        if (TouchInput.isTriggered() && this._closeHover) {
            this.visible = false;
        }
    };

    // ═══════════════════════════════════════════════════════════
    //  好友请求弹窗 — L2_Dialog 确认/拒绝
    // ═══════════════════════════════════════════════════════════

    /**
     * 通用消息提示方法。
     * 支持交互式通知（带 onAccept 回调）和普通信息通知。
     * @param {string} msg - 提示消息文本
     * @param {Object} [options] - 选项 {timeout, onAccept}
     */
    $MMO._showToast = function (msg, options) {
        options = options || {};
        if (options.onAccept) {
            // 交互式通知（带接受操作）。
            var notif = L2_Notification.show({
                title: '好友请求',
                content: msg,
                type: 'info',
                duration: (options.timeout || 5000) / 1000 * 60, // 毫秒转帧数
                closable: true,
                onClose: function () {}
            });
            // 点击通知时触发接受回调。
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

    // ═══════════════════════════════════════════════════════════
    //  注入 Scene_Map — 创建好友列表和公会面板
    // ═══════════════════════════════════════════════════════════

    /** @type {Function} 原始 Scene_Map.createAllWindows 引用。 */
    var _Scene_Map_createAllWindows4 = Scene_Map.prototype.createAllWindows;

    /**
     * 覆写 Scene_Map.createAllWindows：追加创建好友列表和公会面板。
     */
    Scene_Map.prototype.createAllWindows = function () {
        _Scene_Map_createAllWindows4.call(this);
        this._friendList = new FriendListPanel();
        this._guildInfo = new GuildInfoPanel();
        this.addChild(this._friendList);
        this.addChild(this._guildInfo);
        $MMO._friendListWin = this._friendList;
        $MMO._guildInfoWin = this._guildInfo;
    };

    // ═══════════════════════════════════════════════════════════
    //  Alt+F / Alt+G 快捷键
    // ═══════════════════════════════════════════════════════════

    /**
     * 全局 keydown 监听器：Alt+F 切换好友列表，Alt+G 切换公会面板。
     */
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

    // ═══════════════════════════════════════════════════════════
    //  WebSocket 消息处理器
    // ═══════════════════════════════════════════════════════════

    /**
     * friend_request：收到好友请求，弹出确认对话框。
     */
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

    /**
     * friend_online：好友上线通知。
     */
    $MMO.on('friend_online', function (data) {
        if ($MMO._friendListWin) $MMO._friendListWin.updateFriendStatus(data.char_id, true);
    });

    /**
     * friend_offline：好友下线通知。
     */
    $MMO.on('friend_offline', function (data) {
        if ($MMO._friendListWin) $MMO._friendListWin.updateFriendStatus(data.char_id, false);
    });

    /**
     * map_init：从初始化数据中获取公会 ID。
     */
    $MMO.on('map_init', function (data) {
        if (data.self && data.self.guild_id) $MMO._guildID = data.self.guild_id;
    });

    // ═══════════════════════════════════════════════════════════
    //  全局窗口类导出
    // ═══════════════════════════════════════════════════════════
    window.Window_FriendList = FriendListPanel;
    window.Window_GuildInfo = GuildInfoPanel;

})();
