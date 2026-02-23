/*:
 * @plugindesc v2.0.0 MMO Chat - multi-channel chat box (L2 canvas + HTML input).
 * @author MMO Framework
 */

(function () {
    'use strict';

    var CHANNELS = ['world', 'party', 'guild', 'battle', 'system', 'private'];
    var CHANNEL_LABELS = {
        world: 'World', party: 'Party', guild: 'Guild',
        battle: 'Battle', system: 'System', private: 'PM'
    };
    var CHANNEL_COLORS = {
        world:   '#FFFFFF',
        party:   '#44FF88',
        guild:   '#FFD700',
        battle:  '#FF6666',
        system:  '#88AAFF',
        private: '#DD88FF'
    };
    var MAX_MESSAGES = 100;
    var CHAT_W = 260, CHAT_H = 150;
    var TAB_H = 20, INPUT_H = 22, PAD = 4;
    var LOG_H = CHAT_H - TAB_H - INPUT_H - PAD;
    var LINE_H = 15;

    $MMO._chatHistory = { world: [], party: [], guild: [], battle: [], system: [], private: [] };
    $MMO._chatChannel = 'world';
    $MMO._chatUnread = {};
    CHANNELS.forEach(function (ch) { $MMO._chatUnread[ch] = 0; });

    // -----------------------------------------------------------------
    // ChatBox — L2_Base canvas panel + HTML input overlay
    // -----------------------------------------------------------------
    function ChatBox() { this.initialize.apply(this, arguments); }
    ChatBox.prototype = Object.create(L2_Base.prototype);
    ChatBox.prototype.constructor = ChatBox;

    ChatBox.prototype.initialize = function () {
        var x = 4, y = Graphics.boxHeight - CHAT_H - 4;
        L2_Base.prototype.initialize.call(this, x, y, CHAT_W, CHAT_H);
        this._scrollY = 0;
        this._hoverTab = -1;
        this._inputEl = null;
        this._inputFocused = false;
        this._createInput();
        var self = this;
        $MMO.makeDraggable(this, 'chatBox', {
            dragArea: { y: 0, h: TAB_H },
            onMove: function () { self._positionInput(); }
        });
        this.refresh();
    };

    ChatBox.prototype.standardPadding = function () { return 0; };

    ChatBox.prototype._createInput = function () {
        var el = document.createElement('input');
        el.type = 'text';
        el.placeholder = 'Enter 发送消息…';
        el.style.cssText = [
            'position:absolute',
            'background:rgba(0,0,0,0.6)',
            'color:#fff',
            'border:1px solid #444',
            'border-radius:2px',
            'padding:2px 4px',
            'font-family:monospace',
            'font-size:12px',
            'outline:none',
            'box-sizing:border-box',
            'z-index:9999'
        ].join(';');
        this._inputEl = el;
        this._positionInput();

        var self = this;
        el.addEventListener('focus', function () { self._inputFocused = true; });
        el.addEventListener('blur', function () { self._inputFocused = false; });
        el.addEventListener('keydown', function (e) {
            if (e.keyCode === 13) { // Enter
                sendMessage(el.value);
                el.value = '';
                e.stopPropagation();
            } else if (e.keyCode === 27) { // Escape
                el.blur();
                e.stopPropagation();
            }
            e.stopPropagation();
        });
        document.body.appendChild(el);
    };

    ChatBox.prototype._positionInput = function () {
        if (!this._inputEl) return;
        var el = this._inputEl;
        // Account for canvas offset within the browser page
        var canvas = Graphics._canvas || document.querySelector('canvas');
        var rect = canvas ? canvas.getBoundingClientRect() : { left: 0, top: 0, width: Graphics.boxWidth, height: Graphics.boxHeight };
        var scaleX = rect.width / Graphics.boxWidth;
        var scaleY = rect.height / Graphics.boxHeight;
        el.style.left = Math.round(rect.left + this.x * scaleX) + 'px';
        el.style.top = Math.round(rect.top + (this.y + CHAT_H - INPUT_H) * scaleY) + 'px';
        el.style.width = Math.round(CHAT_W * scaleX) + 'px';
        el.style.height = Math.round(INPUT_H * scaleY) + 'px';
    };

    ChatBox.prototype.show = function () {
        this.visible = true;
        if (this._inputEl) this._inputEl.style.display = 'block';
        this._positionInput();
    };

    ChatBox.prototype.hide = function () {
        this.visible = false;
        if (this._inputEl) {
            this._inputEl.blur();
            this._inputEl.style.display = 'none';
        }
    };

    ChatBox.prototype.destroy = function () {
        if (this._inputEl && this._inputEl.parentNode) {
            this._inputEl.parentNode.removeChild(this._inputEl);
            this._inputEl = null;
        }
    };

    ChatBox.prototype.refresh = function () {
        var c = this.bmp();
        c.clear();
        var w = this.cw(), h = this.ch();

        // Background
        L2_Theme.fillRoundRect(c, 0, 0, w, h, L2_Theme.cornerRadius, 'rgba(13,13,26,0.70)');
        L2_Theme.strokeRoundRect(c, 0, 0, w, h, L2_Theme.cornerRadius, L2_Theme.borderDark);

        // Tabs
        this._drawTabs(c, w);

        // Log area
        this._drawLog(c, w);
    };

    ChatBox.prototype._drawTabs = function (c, w) {
        var tabW = Math.floor(w / CHANNELS.length);
        var self = this;
        CHANNELS.forEach(function (ch, i) {
            var tx = i * tabW;
            var active = ch === $MMO._chatChannel;
            var hover = i === self._hoverTab;

            if (active) {
                c.fillRect(tx, 0, tabW, TAB_H, '#1E1E38');
                c.fillRect(tx, TAB_H - 2, tabW, 2, L2_Theme.textGold);
            } else if (hover) {
                c.fillRect(tx, 0, tabW, TAB_H, L2_Theme.highlight);
            }

            c.fontSize = 10;
            c.textColor = active ? L2_Theme.textGold : L2_Theme.textGray;
            var label = CHANNEL_LABELS[ch] || ch;
            if ($MMO._chatUnread[ch] > 0 && !active) label += ' ●';
            c.drawText(label, tx, 1, tabW, TAB_H - 2, 'center');

            if (i > 0) c.fillRect(tx, 3, 1, TAB_H - 6, L2_Theme.borderDark);
        });
        // Tab bottom border
        c.fillRect(0, TAB_H, w, 1, L2_Theme.borderDark);
    };

    ChatBox.prototype._drawLog = function (c, w) {
        var ch = $MMO._chatChannel;
        var hist = $MMO._chatHistory[ch] || [];
        var logY = TAB_H + 1;
        var logBottom = CHAT_H - INPUT_H;
        var logH = logBottom - logY;
        var totalH = hist.length * LINE_H;

        // Auto-scroll to bottom
        var maxScroll = Math.max(0, totalH - logH);
        this._scrollY = maxScroll;

        // Clip region — draw messages within log area
        var startIdx = Math.max(0, Math.floor(this._scrollY / LINE_H));
        var visCount = Math.ceil(logH / LINE_H) + 1;

        for (var i = startIdx; i < Math.min(startIdx + visCount, hist.length); i++) {
            var my = logY + (i * LINE_H - this._scrollY);
            if (my + LINE_H < logY || my > logBottom) continue;

            var msg = hist[i];
            c.fontSize = 12;
            c.textColor = CHANNEL_COLORS[msg.channel] || '#FFFFFF';
            var text = '[' + (msg.channel || 'w').charAt(0).toUpperCase() + '] ';
            if (msg.sender) text += msg.sender + ': ';
            text += msg.text;
            c.drawText(text, PAD, my, w - PAD * 2, LINE_H, 'left');
        }

        // Scrollbar
        if (totalH > logH) {
            var sbW = 4;
            var trackH = logH - 4;
            var thumbH = Math.max(12, Math.round(trackH * (logH / totalH)));
            var thumbY = logY + 2 + Math.round((trackH - thumbH) * (maxScroll > 0 ? this._scrollY / maxScroll : 0));
            c.fillRect(w - sbW, logY, sbW, logH, 'rgba(0,0,0,0.2)');
            L2_Theme.fillRoundRect(c, w - sbW, thumbY, sbW, thumbH, 2, '#444466');
        }
    };

    ChatBox.prototype.scrollLog = function (delta) {
        var ch = $MMO._chatChannel;
        var hist = $MMO._chatHistory[ch] || [];
        var logH = CHAT_H - TAB_H - INPUT_H - 1;
        var totalH = hist.length * LINE_H;
        var maxScroll = Math.max(0, totalH - logH);
        this._scrollY += delta > 0 ? LINE_H * 3 : -LINE_H * 3;
        this._scrollY = Math.max(0, Math.min(this._scrollY, maxScroll));
        this.refresh();
    };

    ChatBox.prototype.switchChannel = function (ch) {
        $MMO._chatChannel = ch;
        $MMO._chatUnread[ch] = 0;
        this._scrollY = 0;
        this.refresh();
    };

    ChatBox.prototype.update = function () {
        L2_Base.prototype.update.call(this);
        if (!this.visible) return;

        // Keep HTML input position in sync (handles resize, fullscreen, etc.)
        if (Graphics.frameCount % 60 === 0) this._positionInput();

        // Drag handling — skip click logic while actively dragging
        if ($MMO.updateDrag(this)) return;

        var mx = TouchInput.x - this.x;
        var my = TouchInput.y - this.y;
        var inside = mx >= 0 && mx < CHAT_W && my >= 0 && my < CHAT_H;

        // Tab hover
        var tabW = Math.floor(CHAT_W / CHANNELS.length);
        var oldHover = this._hoverTab;
        if (inside && my >= 0 && my < TAB_H) {
            this._hoverTab = Math.min(Math.floor(mx / tabW), CHANNELS.length - 1);
        } else {
            this._hoverTab = -1;
        }
        if (this._hoverTab !== oldHover) this.refresh();

        // Tab click
        if (inside && my < TAB_H && TouchInput.isTriggered()) {
            var tabIdx = Math.min(Math.floor(mx / tabW), CHANNELS.length - 1);
            this.switchChannel(CHANNELS[tabIdx]);
        }

        // Scroll on mouse wheel in log area
        if (inside && my >= TAB_H && my < CHAT_H - INPUT_H && TouchInput.wheelY) {
            this.scrollLog(TouchInput.wheelY);
        }
    };

    // -----------------------------------------------------------------
    // Chat message management
    // -----------------------------------------------------------------
    var _chatBox = null;

    function addMessage(msg) {
        var ch = msg.channel;
        if (!$MMO._chatHistory[ch]) return;
        $MMO._chatHistory[ch].push(msg);
        if ($MMO._chatHistory[ch].length > MAX_MESSAGES) {
            $MMO._chatHistory[ch].shift();
        }
        if (ch !== $MMO._chatChannel) {
            $MMO._chatUnread[ch] = ($MMO._chatUnread[ch] || 0) + 1;
        }
        if (_chatBox) _chatBox.refresh();
    }

    function sendMessage(text) {
        if (!text || !text.trim()) return;
        text = text.trim();
        var channel = $MMO._chatChannel;
        var targetID = null;
        // Slash commands
        if (text.startsWith('/')) {
            var parts = text.split(' ');
            var cmd = parts[0].toLowerCase();
            if (cmd === '/resetpos') {
                $MMO.send('reset_pos', {});
                addMessage({ channel: 'system', sender: 'SYSTEM', text: 'Requesting position reset...' });
                return;
            }
            if (cmd === '/all' || cmd === '/world') { channel = 'world'; text = parts.slice(1).join(' '); }
            else if (cmd === '/party' || cmd === '/p') { channel = 'party'; text = parts.slice(1).join(' '); }
            else if (cmd === '/guild' || cmd === '/g') { channel = 'guild'; text = parts.slice(1).join(' '); }
            else if (cmd === '/pm' || cmd === '/w') {
                if (parts.length < 3) {
                    addMessage({ channel: 'system', sender: 'SYSTEM', text: 'Usage: /w <player> <message>' });
                    return;
                }
                channel = 'private'; targetID = parts[1]; text = parts.slice(2).join(' ');
            }
        }
        if (!text) return;
        $MMO.send('chat_send', { channel: channel, content: text, target_name: targetID || undefined });
    }

    // -----------------------------------------------------------------
    // Disable RMMV input while chat input is focused
    // -----------------------------------------------------------------
    var _Input_isPressed = Input.isPressed.bind(Input);
    Input.isPressed = function (keyName) {
        if (_chatBox && _chatBox._inputFocused) return false;
        return _Input_isPressed(keyName);
    };

    var _Input_isTriggered = Input.isTriggered.bind(Input);
    Input.isTriggered = function (keyName) {
        if (_chatBox && _chatBox._inputFocused) return false;
        return _Input_isTriggered(keyName);
    };

    var _Input_isRepeated = Input.isRepeated.bind(Input);
    Input.isRepeated = function (keyName) {
        if (_chatBox && _chatBox._inputFocused) return false;
        return _Input_isRepeated(keyName);
    };

    // Enter key focuses chat input
    var _chatKeydownHandler = function (e) {
        if (_chatBox && _chatBox._inputFocused) return;
        if (e.keyCode === 13) { // Enter
            if (_chatBox && _chatBox._inputEl) _chatBox._inputEl.focus();
            e.preventDefault();
        }
    };
    window.addEventListener('keydown', _chatKeydownHandler);

    // -----------------------------------------------------------------
    // Inject into Scene_Map
    // -----------------------------------------------------------------
    var _Scene_Map_createAllWindows6 = Scene_Map.prototype.createAllWindows;
    Scene_Map.prototype.createAllWindows = function () {
        _Scene_Map_createAllWindows6.call(this);
        _chatBox = new ChatBox();
        this.addChild(_chatBox);
    };

    var _Scene_Map_start_chat = Scene_Map.prototype.start;
    Scene_Map.prototype.start = function () {
        _Scene_Map_start_chat.call(this);
        if (_chatBox) _chatBox.show();
    };

    var _Scene_Map_terminate3 = Scene_Map.prototype.terminate;
    Scene_Map.prototype.terminate = function () {
        _Scene_Map_terminate3.call(this);
        if (_chatBox) {
            _chatBox.hide();
            _chatBox.destroy();
            _chatBox = null;
        }
        window.removeEventListener('keydown', _chatKeydownHandler);
    };

    // -----------------------------------------------------------------
    // WebSocket handlers
    // -----------------------------------------------------------------
    $MMO.on('reset_pos', function (data) {
        if (!data) return;
        if (data.error) {
            addMessage({ channel: 'system', sender: 'SYSTEM', text: data.error });
            return;
        }
        if ($gamePlayer) {
            $gamePlayer.locate(data.x, data.y);
            $gamePlayer.setDirection(data.dir || 2);
        }
        addMessage({ channel: 'system', sender: 'SYSTEM', text: 'Teleported to (' + data.x + ', ' + data.y + ')' });
    });

    $MMO.on('chat_recv', function (data) {
        addMessage({ channel: data.channel || 'world', sender: data.from_name || '', text: data.content || '' });
    });

    $MMO.on('system_notice', function (data) {
        addMessage({ channel: 'system', sender: 'SYSTEM', text: data.message || '' });
    });

    $MMO.on('battle_result', function (data) {
        if (!data) return;
        var text = 'You dealt ' + (data.damage || 0) + ' damage';
        if (data.is_crit) text += ' (CRITICAL!)';
        addMessage({ channel: 'battle', sender: '', text: text });
    });

})();
