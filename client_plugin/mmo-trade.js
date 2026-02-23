/*:
 * @plugindesc v2.0.0 MMO Trade - player-to-player trade window (L2 UI).
 * @author MMO Framework
 */

(function () {
    'use strict';

    $MMO._tradeWindow = null;
    $MMO._tradeData = null;

    var TRADE_W = 500, TRADE_H = 360, PAD = 10;

    // -----------------------------------------------------------------
    // TradeWindow — L2_Base panel
    // -----------------------------------------------------------------
    function TradeWindow() { this.initialize.apply(this, arguments); }
    TradeWindow.prototype = Object.create(L2_Base.prototype);
    TradeWindow.prototype.constructor = TradeWindow;

    TradeWindow.prototype.initialize = function () {
        L2_Base.prototype.initialize.call(this,
            (Graphics.boxWidth - TRADE_W) / 2,
            (Graphics.boxHeight - TRADE_H) / 2,
            TRADE_W, TRADE_H);
        this.visible = false;
        this._myItems = [];
        this._myGold = 0;
        this._theirItems = [];
        this._theirGold = 0;
        this._myConfirmed = false;
        this._theirConfirmed = false;
        this._sessionID = null;
        this._goldEditing = false;
        this._goldText = '0';
        this._hoverBtn = null; // 'confirm' | 'cancel' | null
    };

    TradeWindow.prototype.standardPadding = function () { return 0; };

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

    TradeWindow.prototype.close = function () {
        this.visible = false;
        this._goldEditing = false;
    };

    TradeWindow.prototype.updateTheirOffer = function (offer) {
        this._theirItems = offer.items || [];
        this._theirGold = offer.gold || 0;
        this.refresh();
    };

    TradeWindow.prototype.updateConfirmStatus = function (confirmed) {
        this._myConfirmed = confirmed && confirmed.me;
        this._theirConfirmed = confirmed && confirmed.them;
        this.refresh();
    };

    TradeWindow.prototype._sendOffer = function () {
        $MMO.send('trade_update', {
            item_ids: this._myItems.map(function (i) { return i.id; }),
            gold: this._myGold
        });
    };

    TradeWindow.prototype.refresh = function () {
        var c = this.bmp();
        c.clear();
        var w = this.cw(), h = this.ch();
        var half = Math.floor((w - PAD * 3) / 2);
        var leftX = PAD, rightX = PAD * 2 + half;
        var btnH = 36, btnY = h - PAD - btnH;

        // Background
        L2_Theme.fillRoundRect(c, 0, 0, w, h, L2_Theme.cornerRadius, 'rgba(13,13,26,0.85)');
        L2_Theme.strokeRoundRect(c, 0, 0, w, h, L2_Theme.cornerRadius, L2_Theme.borderDark);

        // Title bar
        c.fontSize = 14;
        c.textColor = L2_Theme.textGold;
        c.drawText('交易', 0, PAD, w, 18, 'center');

        // Divider (vertical)
        var divX = PAD + half + Math.floor(PAD / 2);
        c.fillRect(divX, 34, 1, btnY - 40, L2_Theme.borderDark);

        // Column headers
        var headerY = 34;
        c.fontSize = L2_Theme.fontSmall;
        c.textColor = L2_Theme.textGold;
        c.drawText('我的报价' + (this._myConfirmed ? ' ✓' : ''), leftX, headerY, half, 16, 'left');
        c.drawText('对方报价' + (this._theirConfirmed ? ' ✓' : ''), rightX, headerY, half, 16, 'left');

        // My items
        c.fontSize = 12;
        c.textColor = L2_Theme.textWhite;
        var itemY = headerY + 22;
        this._myItems.slice(0, 5).forEach(function (item, i) {
            c.drawText((item.name || 'Item') + ' x' + (item.qty || 1), leftX, itemY + i * 22, half, 18, 'left');
        });

        // My gold (editable field)
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

        // Their items
        c.textColor = L2_Theme.textWhite;
        c.fontSize = 12;
        this._theirItems.slice(0, 5).forEach(function (item, i) {
            c.drawText((item.name || 'Item') + ' x' + (item.qty || 1), rightX, itemY + i * 22, half, 18, 'left');
        });

        // Their gold
        c.textColor = '#FFFF88';
        c.fontSize = 13;
        c.drawText('金币: ' + this._theirGold, rightX, goldY + 3, half, 20, 'left');

        // Confirm button
        var confirmW = half, cancelW = half;
        var confirmHover = this._hoverBtn === 'confirm';
        var cancelHover = this._hoverBtn === 'cancel';
        L2_Theme.fillRoundRect(c, leftX, btnY, confirmW, btnH, 3,
            confirmHover ? '#2a5a2a' : '#1A3A1A');
        L2_Theme.strokeRoundRect(c, leftX, btnY, confirmW, btnH, 3, '#44FF88');
        c.fontSize = 14;
        c.textColor = '#44FF88';
        c.drawText('确认', leftX, btnY, confirmW, btnH, 'center');

        // Cancel button
        L2_Theme.fillRoundRect(c, rightX, btnY, cancelW, btnH, 3,
            cancelHover ? '#5a2a2a' : '#3A1A1A');
        L2_Theme.strokeRoundRect(c, rightX, btnY, cancelW, btnH, 3, '#FF6666');
        c.textColor = '#FF6666';
        c.drawText('取消', rightX, btnY, cancelW, btnH, 'center');
    };

    TradeWindow.prototype.update = function () {
        L2_Base.prototype.update.call(this);
        if (!this.visible) return;

        var w = this.cw(), h = this.ch();
        var half = Math.floor((w - PAD * 3) / 2);
        var leftX = PAD, rightX = PAD * 2 + half;
        var btnH = 36, btnY = h - PAD - btnH;

        // Hover detection
        var mx = TouchInput.x - this.x;
        var my = TouchInput.y - this.y;
        var oldHover = this._hoverBtn;
        this._hoverBtn = null;
        if (my >= btnY && my <= btnY + btnH) {
            if (mx >= leftX && mx <= leftX + half) this._hoverBtn = 'confirm';
            else if (mx >= rightX && mx <= rightX + half) this._hoverBtn = 'cancel';
        }
        if (this._hoverBtn !== oldHover) this.refresh();

        // Handle keyboard input for gold editing
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

        // Click detection
        if (TouchInput.isTriggered()) {
            var lx = TouchInput.x - this.x;
            var ly = TouchInput.y - this.y;

            if (!this.isInside(TouchInput.x, TouchInput.y)) return;

            // Gold field click
            var goldY = 34 + 22 + 120;
            if (lx >= leftX && lx <= leftX + 140 && ly >= goldY && ly <= goldY + 26) {
                this._goldEditing = true;
                this._goldText = String(this._myGold);
                this.refresh();
                return;
            }

            // Buttons
            if (ly >= btnY && ly <= btnY + btnH) {
                if (lx >= leftX && lx <= leftX + half) {
                    $MMO.send('trade_confirm', {});
                } else if (lx >= rightX && lx <= rightX + half) {
                    $MMO.send('trade_cancel', {});
                }
            }
        }
    };

    // -----------------------------------------------------------------
    // Trade request dialog — L2_Dialog
    // -----------------------------------------------------------------
    var _tradeRequestDialog = null;

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

        var timer = setInterval(function () {
            countdown--;
            if (countdown <= 0) { respond(false); return; }
            dlg._content = (data.from_name || '?') + ' 请求与你交易\n' +
                           countdown + 's 后自动拒绝';
            dlg._contentLines = dlg._wrapText(dlg._content, dlg.width - 40);
            dlg.refresh();
        }, 1000);

        function respond(accept) {
            clearInterval(timer);
            dlg.close();
            _tradeRequestDialog = null;
            if (accept) {
                $MMO.send('trade_accept', { from_char_id: data.from_id });
            }
        }

        _tradeRequestDialog = { respond: respond };
    }

    // -----------------------------------------------------------------
    // Inject into Scene_Map
    // -----------------------------------------------------------------
    var _Scene_Map_createAllWindows5 = Scene_Map.prototype.createAllWindows;
    Scene_Map.prototype.createAllWindows = function () {
        _Scene_Map_createAllWindows5.call(this);
        this._tradeWindow = new TradeWindow();
        this.addChild(this._tradeWindow);
        $MMO._tradeWindow = this._tradeWindow;
    };

    var _Scene_Map_terminate4 = Scene_Map.prototype.terminate;
    Scene_Map.prototype.terminate = function () {
        _Scene_Map_terminate4.call(this);
        if ($MMO._tradeWindow && $MMO._tradeWindow.visible) {
            $MMO.send('trade_cancel', {});
            $MMO._tradeWindow.close();
        }
        if (_tradeRequestDialog) _tradeRequestDialog.respond(false);
        this._tradeWindow = null;
        $MMO._tradeWindow = null;
    };

    // -----------------------------------------------------------------
    // WebSocket handlers
    // -----------------------------------------------------------------
    $MMO.on('trade_request', function (data) {
        showTradeRequestDialog(data);
    });

    $MMO.on('trade_accepted', function (data) {
        if ($MMO._tradeWindow) {
            $MMO._tradeWindow.open(data);
        }
    });

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

    $MMO.on('trade_cancel', function () {
        if ($MMO._tradeWindow) $MMO._tradeWindow.close();
    });

    $MMO.on('_disconnected', function () {
        if ($MMO._tradeWindow) $MMO._tradeWindow.close();
        if (_tradeRequestDialog) _tradeRequestDialog.respond(false);
    });

    window.Window_Trade = TradeWindow;

})();
