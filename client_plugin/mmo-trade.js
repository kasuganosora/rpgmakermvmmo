/*:
 * @plugindesc v2.0.0 MMO 交易系统 — 玩家间物品/金币交易窗口（L2 UI）。
 * @author MMO Framework
 *
 * @help
 * 本插件实现玩家间交易功能：
 *
 * 交易窗口：
 * - 双列布局：左侧我的报价，右侧对方报价
 * - 物品列表（最多 5 件）+ 金币输入字段
 * - 金币字段点击后进入编辑模式（数字键输入）
 * - 确认/取消按钮，双方都确认后交易完成
 * - 确认状态标记（✓）
 * - 窗口居中显示，支持自动居中
 *
 * 交易请求弹窗：
 * - L2_Dialog 弹窗确认接受/拒绝
 * - 15 秒倒计时自动拒绝
 *
 * 安全机制：
 * - 切换场景时自动取消交易并通知服务器
 * - 断线时自动关闭交易窗口并清理弹窗
 *
 * 全局引用：
 * - window.Window_Trade — TradeWindow 构造函数
 * - $MMO._tradeWindow — 交易窗口实例
 * - $MMO._tradeData — 当前交易数据
 *
 * WebSocket 消息：
 * - trade_request — 收到交易请求
 * - trade_accepted — 交易已接受，打开窗口
 * - trade_update — 交易状态更新（协商/确认/完成/取消）
 * - trade_cancel — 对方取消交易
 * - trade_confirm/trade_cancel/trade_update/trade_accept（发送）
 */

(function () {
    'use strict';

    // ═══════════════════════════════════════════════════════════
    //  全局状态
    // ═══════════════════════════════════════════════════════════
    /** @type {TradeWindow|null} 交易窗口实例。 */
    $MMO._tradeWindow = null;
    /** @type {Object|null} 当前交易数据。 */
    $MMO._tradeData = null;

    // ═══════════════════════════════════════════════════════════
    //  常量配置
    // ═══════════════════════════════════════════════════════════
    /** @type {number} 交易窗口宽度。 */
    var TRADE_W = 500;
    /** @type {number} 交易窗口高度。 */
    var TRADE_H = 360;
    /** @type {number} 内边距。 */
    var PAD = 10;

    // ═══════════════════════════════════════════════════════════
    //  TradeWindow — 继承 L2_Base 的交易窗口
    //  屏幕居中，双列布局显示双方报价。
    // ═══════════════════════════════════════════════════════════

    /**
     * 交易窗口构造函数。
     * @constructor
     */
    function TradeWindow() { this.initialize.apply(this, arguments); }
    TradeWindow.prototype = Object.create(L2_Base.prototype);
    TradeWindow.prototype.constructor = TradeWindow;

    /**
     * 初始化交易窗口：居中放置，默认隐藏，重置所有交易状态。
     */
    TradeWindow.prototype.initialize = function () {
        L2_Base.prototype.initialize.call(this,
            (Graphics.boxWidth - TRADE_W) / 2,
            (Graphics.boxHeight - TRADE_H) / 2,
            TRADE_W, TRADE_H);
        this.visible = false;
        /** @type {Array} 我方报价物品列表。 */
        this._myItems = [];
        /** @type {number} 我方报价金币数量。 */
        this._myGold = 0;
        /** @type {Array} 对方报价物品列表。 */
        this._theirItems = [];
        /** @type {number} 对方报价金币数量。 */
        this._theirGold = 0;
        /** @type {boolean} 我方是否已确认。 */
        this._myConfirmed = false;
        /** @type {boolean} 对方是否已确认。 */
        this._theirConfirmed = false;
        /** @type {string|null} 交易会话 ID。 */
        this._sessionID = null;
        /** @type {boolean} 金币字段是否处于编辑模式。 */
        this._goldEditing = false;
        /** @type {string} 金币编辑中的文本。 */
        this._goldText = '0';
        /** @type {string|null} 悬停的按钮 'confirm'|'cancel'|null。 */
        this._hoverBtn = null;

        // 启用窗口缩放时自动居中。
        this._isCentered = true;
    };

    /**
     * 禁用标准内边距。
     * @returns {number} 0
     */
    TradeWindow.prototype.standardPadding = function () { return 0; };

    /**
     * 打开交易窗口：重置所有状态并显示。
     * @param {Object} data - 交易数据 {session_id}
     */
    TradeWindow.prototype.open = function (data) {
        this._sessionID = data.session_id;
        this._myItems = [];
        this._myGold = 0;
        this._theirItems = [];
        this._theirGold = 0;
        this._myConfirmed = false;
        this._theirConfirmed = false;
        this._goldEditing = false;
        this._goldText = '0';
        this._hoverBtn = null;
        this.visible = true;
        this.refresh();
    };

    /**
     * 关闭交易窗口。
     */
    TradeWindow.prototype.close = function () {
        this.visible = false;
        this._goldEditing = false;
    };

    /**
     * 更新对方的报价内容。
     * @param {Object} offer - 对方报价 {items, gold}
     */
    TradeWindow.prototype.updateTheirOffer = function (offer) {
        this._theirItems = offer.items || [];
        this._theirGold = offer.gold || 0;
        this.refresh();
    };

    /**
     * 更新双方确认状态。
     * @param {Object} confirmed - 确认状态 {me, them}
     */
    TradeWindow.prototype.updateConfirmStatus = function (confirmed) {
        this._myConfirmed = confirmed && confirmed.me;
        this._theirConfirmed = confirmed && confirmed.them;
        this.refresh();
    };

    /**
     * 向服务器发送我方当前报价。
     * @private
     */
    TradeWindow.prototype._sendOffer = function () {
        $MMO.send('trade_update', {
            item_ids: this._myItems.map(function (i) { return i.id; }),
            gold: this._myGold
        });
    };

    /**
     * 重绘交易窗口。
     * 布局：标题 → 左列（我的报价）+ 右列（对方报价）→ 确认/取消按钮。
     */
    TradeWindow.prototype.refresh = function () {
        var c = this.bmp();
        c.clear();
        var w = this.cw(), h = this.ch();
        var half = Math.floor((w - PAD * 3) / 2);
        var leftX = PAD, rightX = PAD * 2 + half;
        var btnH = 36, btnY = h - PAD - btnH;

        // 绘制半透明圆角背景。
        L2_Theme.fillRoundRect(c, 0, 0, w, h, L2_Theme.cornerRadius, 'rgba(13,13,26,0.85)');
        L2_Theme.strokeRoundRect(c, 0, 0, w, h, L2_Theme.cornerRadius, L2_Theme.borderDark);

        // 标题栏。
        c.fontSize = 14;
        c.textColor = L2_Theme.textGold;
        c.drawText('交易', 0, PAD, w, 18, 'center');

        // 垂直分隔线。
        var divX = PAD + half + Math.floor(PAD / 2);
        c.fillRect(divX, 34, 1, btnY - 40, L2_Theme.borderDark);

        // 列标题（含确认标记 ✓）。
        var headerY = 34;
        c.fontSize = L2_Theme.fontSmall;
        c.textColor = L2_Theme.textGold;
        c.drawText('我的报价' + (this._myConfirmed ? ' ✓' : ''), leftX, headerY, half, 16, 'left');
        c.drawText('对方报价' + (this._theirConfirmed ? ' ✓' : ''), rightX, headerY, half, 16, 'left');

        // 我方物品列表（最多 5 件）。
        c.fontSize = 12;
        c.textColor = L2_Theme.textWhite;
        var itemY = headerY + 22;
        this._myItems.slice(0, 5).forEach(function (item, i) {
            c.drawText((item.name || 'Item') + ' x' + (item.qty || 1), leftX, itemY + i * 22, half, 18, 'left');
        });

        // 我方金币字段（可编辑，点击进入编辑模式）。
        var goldY = itemY + 120;
        var goldFieldW = 140;
        c.fillRect(leftX, goldY, goldFieldW, 26,
            this._goldEditing ? 'rgba(68,136,255,0.20)' : 'rgba(0,0,0,0.40)');
        L2_Theme.strokeRoundRect(c, leftX, goldY, goldFieldW, 26, 2,
            this._goldEditing ? L2_Theme.borderGold : L2_Theme.borderDark);
        c.fontSize = 13;
        c.textColor = '#FFFF88';
        var goldDisplay = this._goldEditing ? this._goldText + '_' : String(this._myGold);
        c.drawText('金币: ' + goldDisplay, leftX + 4, goldY + 3, goldFieldW - 8, 20, 'left');

        // 对方物品列表。
        c.textColor = L2_Theme.textWhite;
        c.fontSize = 12;
        this._theirItems.slice(0, 5).forEach(function (item, i) {
            c.drawText((item.name || 'Item') + ' x' + (item.qty || 1), rightX, itemY + i * 22, half, 18, 'left');
        });

        // 对方金币。
        c.textColor = '#FFFF88';
        c.fontSize = 13;
        c.drawText('金币: ' + this._theirGold, rightX, goldY + 3, half, 20, 'left');

        // 确认按钮（绿色）。
        var confirmW = half, cancelW = half;
        var confirmHover = this._hoverBtn === 'confirm';
        var cancelHover = this._hoverBtn === 'cancel';
        L2_Theme.fillRoundRect(c, leftX, btnY, confirmW, btnH, 3,
            confirmHover ? '#2a5a2a' : '#1A3A1A');
        L2_Theme.strokeRoundRect(c, leftX, btnY, confirmW, btnH, 3, '#44FF88');
        c.fontSize = 14;
        c.textColor = '#44FF88';
        c.drawText('确认', leftX, btnY, confirmW, btnH, 'center');

        // 取消按钮（红色）。
        L2_Theme.fillRoundRect(c, rightX, btnY, cancelW, btnH, 3,
            cancelHover ? '#5a2a2a' : '#3A1A1A');
        L2_Theme.strokeRoundRect(c, rightX, btnY, cancelW, btnH, 3, '#FF6666');
        c.textColor = '#FF6666';
        c.drawText('取消', rightX, btnY, cancelW, btnH, 'center');
    };

    /**
     * 每帧更新：按钮悬停检测、金币键盘输入、点击处理。
     */
    TradeWindow.prototype.update = function () {
        L2_Base.prototype.update.call(this);
        if (!this.visible) return;

        var w = this.cw(), h = this.ch();
        var half = Math.floor((w - PAD * 3) / 2);
        var leftX = PAD, rightX = PAD * 2 + half;
        var btnH = 36, btnY = h - PAD - btnH;

        // 按钮悬停检测。
        var mx = TouchInput.x - this.x;
        var my = TouchInput.y - this.y;
        var oldHover = this._hoverBtn;
        this._hoverBtn = null;
        if (my >= btnY && my <= btnY + btnH) {
            if (mx >= leftX && mx <= leftX + half) this._hoverBtn = 'confirm';
            else if (mx >= rightX && mx <= rightX + half) this._hoverBtn = 'cancel';
        }
        if (this._hoverBtn !== oldHover) this.refresh();

        // 金币编辑模式：数字键输入、退格删除、回车/ESC 确认。
        if (this._goldEditing) {
            for (var k = 0; k <= 9; k++) {
                if (Input.isTriggered(String(k))) {
                    if (this._goldText === '0') this._goldText = '';
                    if (this._goldText.length < 9) this._goldText += String(k);
                    this.refresh();
                }
            }
            if (Input.isTriggered('backspace')) {
                this._goldText = this._goldText.slice(0, -1) || '0';
                this.refresh();
            }
            if (Input.isTriggered('ok') || Input.isTriggered('escape')) {
                this._goldEditing = false;
                this._myGold = parseInt(this._goldText, 10) || 0;
                this._sendOffer();
                this.refresh();
            }
        }

        // 点击处理。
        if (TouchInput.isTriggered()) {
            var lx = TouchInput.x - this.x;
            var ly = TouchInput.y - this.y;

            if (!this.isInside(TouchInput.x, TouchInput.y)) return;

            // 点击金币字段：进入编辑模式。
            var goldY = 34 + 22 + 120;
            if (lx >= leftX && lx <= leftX + 140 && ly >= goldY && ly <= goldY + 26) {
                this._goldEditing = true;
                this._goldText = String(this._myGold);
                this.refresh();
                return;
            }

            // 点击确认/取消按钮。
            if (ly >= btnY && ly <= btnY + btnH) {
                if (lx >= leftX && lx <= leftX + half) {
                    $MMO.send('trade_confirm', {});
                } else if (lx >= rightX && lx <= rightX + half) {
                    $MMO.send('trade_cancel', {});
                }
            }
        }
    };

    // ═══════════════════════════════════════════════════════════
    //  交易请求弹窗 — L2_Dialog 带倒计时自动拒绝
    // ═══════════════════════════════════════════════════════════
    /** @type {Object|null} 当前交易请求弹窗引用。 */
    var _tradeRequestDialog = null;
    /** @type {number|null} 倒计时定时器 ID。 */
    var _tradeRequestTimer = null;

    /**
     * 显示交易请求弹窗。
     * 15 秒内无操作自动拒绝。重复请求时忽略。
     * @param {Object} data - 请求数据 {from_name, from_id}
     */
    function showTradeRequestDialog(data) {
        if (_tradeRequestDialog) return;

        var countdown = 15;
        var dlg = new L2_Dialog({
            title: '交易请求',
            content: (data.from_name || '?') + ' 请求与你交易\n' +
                     countdown + 's 后自动拒绝',
            closable: false,
            buttons: [
                {
                    text: '接受', type: 'primary',
                    onClick: function () { respond(true); }
                },
                {
                    text: '拒绝', type: 'danger',
                    onClick: function () { respond(false); }
                }
            ]
        });

        var scene = SceneManager._scene;
        if (scene) scene.addChild(dlg);

        // 每秒更新倒计时文本，归零后自动拒绝。
        _tradeRequestTimer = setInterval(function () {
            countdown--;
            if (countdown <= 0) { respond(false); return; }
            dlg._content = (data.from_name || '?') + ' 请求与你交易\n' +
                           countdown + 's 后自动拒绝';
            dlg._contentLines = dlg._wrapText(dlg._content, dlg.width - 40);
            dlg.refresh();
        }, 1000);

        /**
         * 回复交易请求并清理弹窗。
         * @param {boolean} accept - 是否接受交易
         */
        function respond(accept) {
            clearInterval(_tradeRequestTimer);
            _tradeRequestTimer = null;
            dlg.close();
            _tradeRequestDialog = null;
            if (accept) {
                $MMO.send('trade_accept', { from_char_id: data.from_id });
            }
        }

        _tradeRequestDialog = { respond: respond };
    }

    // ═══════════════════════════════════════════════════════════
    //  注入 Scene_Map — 创建交易窗口
    // ═══════════════════════════════════════════════════════════

    /** @type {Function} 原始 Scene_Map.createAllWindows 引用。 */
    var _Scene_Map_createAllWindows5 = Scene_Map.prototype.createAllWindows;

    /**
     * 覆写 Scene_Map.createAllWindows：追加创建交易窗口。
     */
    Scene_Map.prototype.createAllWindows = function () {
        _Scene_Map_createAllWindows5.call(this);
        this._tradeWindow = new TradeWindow();
        this.addChild(this._tradeWindow);
        $MMO._tradeWindow = this._tradeWindow;
    };

    /** @type {Function} 原始 Scene_Map.terminate 引用。 */
    var _Scene_Map_terminate4 = Scene_Map.prototype.terminate;

    /**
     * 覆写 Scene_Map.terminate：切换场景时自动取消交易、清理弹窗。
     */
    Scene_Map.prototype.terminate = function () {
        _Scene_Map_terminate4.call(this);
        // 交易中切换场景：通知服务器取消并关闭窗口。
        if ($MMO._tradeWindow && $MMO._tradeWindow.visible) {
            $MMO.send('trade_cancel', {});
            $MMO._tradeWindow.close();
        }
        // 清理交易请求弹窗定时器。
        if (_tradeRequestTimer) { clearInterval(_tradeRequestTimer); _tradeRequestTimer = null; }
        if (_tradeRequestDialog) { _tradeRequestDialog = null; }
        this._tradeWindow = null;
        $MMO._tradeWindow = null;
    };

    // ═══════════════════════════════════════════════════════════
    //  WebSocket 消息处理器
    // ═══════════════════════════════════════════════════════════

    /**
     * trade_request：收到交易请求，显示确认弹窗。
     */
    $MMO.on('trade_request', function (data) {
        showTradeRequestDialog(data);
    });

    /**
     * trade_accepted：交易已被接受，打开交易窗口。
     */
    $MMO.on('trade_accepted', function (data) {
        if ($MMO._tradeWindow) {
            $MMO._tradeWindow.open(data);
        }
    });

    /**
     * trade_update：交易状态更新。
     * 根据 phase 字段处理不同阶段：
     * - negotiating/confirming：更新对方报价和确认状态
     * - done/cancelled：关闭交易窗口
     */
    $MMO.on('trade_update', function (data) {
        if (!$MMO._tradeWindow || !$MMO._tradeWindow.visible) return;
        switch (data.phase) {
        case 'negotiating':
        case 'confirming':
            if (data.their_offer) $MMO._tradeWindow.updateTheirOffer(data.their_offer);
            if (data.confirmed) $MMO._tradeWindow.updateConfirmStatus(data.confirmed);
            break;
        case 'done':
        case 'cancelled':
            $MMO._tradeWindow.close();
            break;
        }
    });

    /**
     * trade_cancel：对方取消交易，关闭窗口。
     */
    $MMO.on('trade_cancel', function () {
        if ($MMO._tradeWindow) $MMO._tradeWindow.close();
    });

    /**
     * 断线处理：关闭交易窗口，清理请求弹窗（不发送消息，WS 已断开）。
     */
    $MMO.on('_disconnected', function () {
        if ($MMO._tradeWindow) $MMO._tradeWindow.close();
        if (_tradeRequestTimer) { clearInterval(_tradeRequestTimer); _tradeRequestTimer = null; }
        _tradeRequestDialog = null;
    });

    // ═══════════════════════════════════════════════════════════
    //  全局窗口类导出
    // ═══════════════════════════════════════════════════════════
    window.Window_Trade = TradeWindow;

})();
