/*:
 * @plugindesc v2.0.0 MMO 队伍面板 — 显示队伍成员 HP/MP 状态（L2 UI）。
 * @author MMO Framework
 *
 * @help
 * 本插件实现队伍面板及入队邀请弹窗：
 *
 * 功能特性：
 * - 左侧紧凑面板，自动根据成员数量调整高度
 * - 每个成员显示名称、HP/MP 进度条
 * - 离线成员灰显 + [Offline] 标签
 * - 不同地图成员灰显 + [Away] 标签
 * - 队伍邀请弹窗带 30 秒自动拒绝倒计时
 * - 断线时自动清理队伍数据和邀请弹窗
 *
 * 全局引用：
 * - window.PartyPanel — PartyPanel 构造函数
 * - $MMO._partyData — 当前队伍数据
 * - $MMO._partyPanel — 队伍面板实例
 *
 * WebSocket 消息：
 * - party_update — 队伍成员状态更新
 * - party_invite_request — 收到入队邀请
 * - party_invite_response（发送）— 回复邀请
 */

(function () {
    'use strict';

    // ═══════════════════════════════════════════════════════════
    //  全局状态
    // ═══════════════════════════════════════════════════════════
    /** @type {Object|null} 当前队伍数据。 */
    $MMO._partyData = null;

    // ═══════════════════════════════════════════════════════════
    //  常量配置
    // ═══════════════════════════════════════════════════════════
    /** @type {number} 标题栏高度。 */
    var HEADER_H = 22;
    /** @type {number} 每个成员行高度。 */
    var MEMBER_H = 38;
    /** @type {number} 面板内边距。 */
    var PAD = 6;
    /** @type {number} 面板宽度。 */
    var PANEL_W = 200;

    // ═══════════════════════════════════════════════════════════
    //  PartyPanel — 继承 L2_Base 的队伍面板
    //  根据成员数量自动调整高度，纵向居中显示。
    // ═══════════════════════════════════════════════════════════

    /**
     * 队伍面板构造函数。
     * @constructor
     */
    function PartyPanel() { this.initialize.apply(this, arguments); }
    PartyPanel.prototype = Object.create(L2_Base.prototype);
    PartyPanel.prototype.constructor = PartyPanel;

    /**
     * 初始化面板：默认隐藏，无成员数据。
     */
    PartyPanel.prototype.initialize = function () {
        L2_Base.prototype.initialize.call(this, 4, 0, PANEL_W, HEADER_H + PAD * 2);
        this.visible = false;
        /** @type {Array} 队伍成员列表。 */
        this._members = [];
    };

    /**
     * 禁用标准内边距。
     * @returns {number} 0
     */
    PartyPanel.prototype.standardPadding = function () { return 0; };

    /**
     * 设置队伍数据并刷新显示。
     * 根据成员数量动态调整面板高度，纵向居中。
     * @param {Object} data - 队伍数据，包含 members 数组
     */
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

    /**
     * 重建位图以适应新的面板高度。
     * @private
     */
    PartyPanel.prototype._refreshBitmap = function () {
        if (this.bitmap) this.bitmap = null;
        this.bitmap = new Bitmap(PANEL_W, this.height);
    };

    /**
     * 重绘队伍面板：背景、标题、成员列表（名称 + HP/MP 条）。
     */
    PartyPanel.prototype.refresh = function () {
        var c = this.bmp();
        c.clear();
        var w = this.cw(), h = this.ch();

        // 绘制半透明圆角背景。
        L2_Theme.fillRoundRect(c, 0, 0, w, h, L2_Theme.cornerRadius, 'rgba(13,13,26,0.65)');
        L2_Theme.strokeRoundRect(c, 0, 0, w, h, L2_Theme.cornerRadius, L2_Theme.borderDark);

        // 标题：队伍 [成员数]。
        c.fontSize = L2_Theme.fontSmall;
        c.textColor = L2_Theme.textGold;
        c.drawText('Party [' + this._members.length + ']', PAD, PAD, w - PAD * 2, 16, 'left');

        var barW = w - PAD * 2;
        var self = this;
        this._members.forEach(function (m, i) {
            var y = PAD + HEADER_H + i * MEMBER_H;
            var offline = !m.online;
            var diffMap = m.map_id !== undefined && $gameMap && m.map_id !== $gameMap.mapId();

            // 成员名称。
            c.fontSize = L2_Theme.fontSmall;
            c.textColor = offline ? L2_Theme.textDim : L2_Theme.textWhite;
            c.drawText(m.name || '?', PAD, y, barW - 40, 14, 'left');

            // 状态标签：离线/不同地图。
            if (offline) {
                c.textColor = L2_Theme.textDim;
                c.fontSize = 10;
                c.drawText('[Offline]', barW - 36, y, 40, 14, 'right');
            } else if (diffMap) {
                c.textColor = L2_Theme.textGray;
                c.fontSize = 10;
                c.drawText('[Away]', barW - 36, y, 40, 14, 'right');
            }

            // HP 进度条（离线/不同地图时灰显）。
            var dim = offline || diffMap;
            var hpRatio = m.max_hp > 0 ? Math.min(m.hp / m.max_hp, 1) : 0;
            L2_Theme.drawBar(c, PAD, y + 16, barW, 8,
                hpRatio, dim ? '#222' : L2_Theme.hpBg, dim ? '#446644' : L2_Theme.hpFill);

            // MP 进度条。
            var mpRatio = m.max_mp > 0 ? Math.min(m.mp / m.max_mp, 1) : 0;
            L2_Theme.drawBar(c, PAD, y + 26, barW, 6,
                mpRatio, dim ? '#222' : L2_Theme.mpBg, dim ? '#444488' : L2_Theme.mpFill);
        });
    };

    // ═══════════════════════════════════════════════════════════
    //  队伍邀请弹窗 — L2_Dialog 带倒计时自动拒绝
    // ═══════════════════════════════════════════════════════════
    /** @type {Object|null} 当前邀请弹窗引用。 */
    var _inviteDialog = null;
    /** @type {number|null} 倒计时定时器 ID。 */
    var _inviteTimer = null;

    /**
     * 显示队伍邀请弹窗。
     * 30 秒内无操作自动拒绝。重复邀请时忽略后续请求。
     * @param {Object} data - 邀请数据 {from_name, from_id}
     */
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

        // 每秒更新倒计时文本，归零后自动拒绝。
        _inviteTimer = setInterval(function () {
            countdown--;
            if (countdown <= 0) { respond(false); return; }
            dlg._content = (data.from_name || '?') + ' invites you to a party.\n' +
                           countdown + 's to auto-decline.';
            dlg._contentLines = dlg._wrapText(dlg._content, dlg.width - 40);
            dlg.refresh();
        }, 1000);

        /**
         * 回复邀请并清理弹窗。
         * @param {boolean} accept - 是否接受邀请
         */
        function respond(accept) {
            clearInterval(_inviteTimer);
            _inviteTimer = null;
            dlg.close();
            _inviteDialog = null;
            $MMO.send('party_invite_response', { accept: accept, from_id: data.from_id });
        }

        _inviteDialog = { respond: respond };
    }

    // ═══════════════════════════════════════════════════════════
    //  注入 Scene_Map — 创建队伍面板
    // ═══════════════════════════════════════════════════════════

    /** @type {Function} 原始 Scene_Map.createAllWindows 引用。 */
    var _Scene_Map_createAllWindows3 = Scene_Map.prototype.createAllWindows;

    /**
     * 覆写 Scene_Map.createAllWindows：追加创建队伍面板。
     */
    Scene_Map.prototype.createAllWindows = function () {
        _Scene_Map_createAllWindows3.call(this);
        this._partyPanel = new PartyPanel();
        this.addChild(this._partyPanel);
        $MMO._partyPanel = this._partyPanel;
    };

    // ═══════════════════════════════════════════════════════════
    //  WebSocket 消息处理器
    // ═══════════════════════════════════════════════════════════

    /**
     * party_update：队伍成员数据更新。
     * 无成员时隐藏面板，有成员时设置数据并显示。
     */
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

    /**
     * party_invite_request：收到入队邀请，显示确认弹窗。
     */
    $MMO.on('party_invite_request', function (data) {
        showInviteDialog(data);
    });

    /** @type {Function} 原始 Scene_Map.terminate 引用。 */
    var _Scene_Map_terminate_party = Scene_Map.prototype.terminate;

    /**
     * 覆写 Scene_Map.terminate：清理邀请弹窗定时器。
     */
    Scene_Map.prototype.terminate = function () {
        _Scene_Map_terminate_party.call(this);
        if (_inviteTimer) { clearInterval(_inviteTimer); _inviteTimer = null; }
        _inviteDialog = null;
    };

    /**
     * 断线处理：清空队伍数据、隐藏面板、清理邀请弹窗。
     */
    $MMO.on('_disconnected', function () {
        $MMO._partyData = null;
        if ($MMO._partyPanel) $MMO._partyPanel.visible = false;
        if (_inviteDialog) {
            if (_inviteTimer) { clearInterval(_inviteTimer); _inviteTimer = null; }
            _inviteDialog = null;
        }
    });

    // ═══════════════════════════════════════════════════════════
    //  全局窗口类导出
    // ═══════════════════════════════════════════════════════════
    window.PartyPanel = PartyPanel;

})();
