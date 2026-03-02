/*:
 * @plugindesc v2.0.0 MMO 聊天系统 — 多频道聊天框（L2 画布 + HTML 输入框）。
 * @author MMO Framework
 *
 * @help
 * 本插件实现左下角多频道聊天框：
 *
 * 频道列表：
 * - world（世界）— 白色，全服广播
 * - party（队伍）— 绿色，队伍内消息
 * - guild（公会）— 金色，公会内消息
 * - battle（战斗）— 红色，战斗日志
 * - system（系统）— 蓝色，系统通知
 * - private（私聊）— 紫色，点对点消息
 *
 * 功能特性：
 * - L2 画布面板 + HTML input 覆盖层（支持中文输入法）
 * - 点击频道标签切换，未读频道显示 ● 标记
 * - Enter 键聚焦输入框，Escape 键失焦
 * - 聊天输入聚焦时自动屏蔽 RMMV 键盘输入
 * - 斜杠命令：/resetpos、/all、/party、/guild、/pm
 * - 消息历史上限 100 条，超出自动移除最早消息
 * - 支持鼠标滚轮滚动消息历史
 * - 可拖拽移动位置（仅在标签栏区域拖拽）
 * - 窗口缩放时自动重定位
 *
 * 全局引用：
 * - $MMO._chatHistory — 各频道消息历史
 * - $MMO._chatChannel — 当前活跃频道
 * - $MMO._chatUnread — 各频道未读计数
 *
 * WebSocket 消息：
 * - chat_recv — 收到聊天消息
 * - system_notice — 系统通知
 * - battle_result — 战斗结果日志
 * - reset_pos — 位置重置结果
 * - chat_send（发送）— 发送聊天消息
 */

(function () {
    'use strict';

    // ═══════════════════════════════════════════════════════════
    //  常量配置
    // ═══════════════════════════════════════════════════════════
    /** @type {Array<string>} 全部频道标识符列表。 */
    var CHANNELS = ['world', 'party', 'guild', 'battle', 'system', 'private'];

    /** @type {Object} 频道显示名称映射。 */
    var CHANNEL_LABELS = {
        world: 'World', party: 'Party', guild: 'Guild',
        battle: 'Battle', system: 'System', private: 'PM'
    };

    /** @type {Object} 频道消息颜色映射。 */
    var CHANNEL_COLORS = {
        world:   '#FFFFFF',
        party:   '#44FF88',
        guild:   '#FFD700',
        battle:  '#FF6666',
        system:  '#88AAFF',
        private: '#DD88FF'
    };

    /** @type {number} 每频道最大消息历史条数。 */
    var MAX_MESSAGES = 100;
    /** @type {number} 聊天框宽度。 */
    var CHAT_W = 260;
    /** @type {number} 聊天框高度。 */
    var CHAT_H = 150;
    /** @type {number} 频道标签栏高度。 */
    var TAB_H = 20;
    /** @type {number} HTML 输入框高度。 */
    var INPUT_H = 22;
    /** @type {number} 内边距。 */
    var PAD = 4;
    /** @type {number} 消息日志区域高度。 */
    var LOG_H = CHAT_H - TAB_H - INPUT_H - PAD;
    /** @type {number} 单行消息高度。 */
    var LINE_H = 15;

    // ═══════════════════════════════════════════════════════════
    //  全局状态
    // ═══════════════════════════════════════════════════════════
    /** @type {Object} 各频道消息历史，键为频道名，值为消息数组。 */
    $MMO._chatHistory = { world: [], party: [], guild: [], battle: [], system: [], private: [] };
    /** @type {string} 当前活跃频道。 */
    $MMO._chatChannel = 'world';
    /** @type {Object} 各频道未读消息计数。 */
    $MMO._chatUnread = {};
    CHANNELS.forEach(function (ch) { $MMO._chatUnread[ch] = 0; });

    // ═══════════════════════════════════════════════════════════
    //  ChatBox — 继承 L2_Base 的聊天面板
    //  画布绘制频道标签和消息日志，HTML input 覆盖层处理文字输入。
    // ═══════════════════════════════════════════════════════════

    /**
     * 聊天框构造函数。
     * @constructor
     */
    function ChatBox() { this.initialize.apply(this, arguments); }
    ChatBox.prototype = Object.create(L2_Base.prototype);
    ChatBox.prototype.constructor = ChatBox;

    /**
     * 初始化聊天框：放置在左下角，创建 HTML 输入框，注册拖拽。
     */
    ChatBox.prototype.initialize = function () {
        var x = 4, y = Graphics.boxHeight - CHAT_H - 4;
        L2_Base.prototype.initialize.call(this, x, y, CHAT_W, CHAT_H);
        /** @type {number} 消息日志滚动偏移量。 */
        this._scrollY = 0;
        /** @type {number} 当前悬停的频道标签索引。 */
        this._hoverTab = -1;
        /** @type {HTMLInputElement|null} HTML 输入框元素。 */
        this._inputEl = null;
        /** @type {boolean} 输入框是否处于聚焦状态。 */
        this._inputFocused = false;
        this._createInput();
        var self = this;
        $MMO.makeDraggable(this, 'chatBox', {
            dragArea: { y: 0, h: TAB_H },
            onMove: function () { self._positionInput(); }
        });
        this.refresh();
    };

    /**
     * 禁用标准内边距。
     * @returns {number} 0
     */
    ChatBox.prototype.standardPadding = function () { return 0; };

    /**
     * 创建 HTML input 元素并添加到 document.body。
     * 设置样式、事件监听器（焦点/按键/鼠标阻断）。
     * 使用 HTML 原生输入框以支持中文输入法和系统剪贴板。
     * @private
     */
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
            if (e.keyCode === 13) { // Enter：发送消息。
                sendMessage(el.value);
                el.value = '';
                e.stopPropagation();
            } else if (e.keyCode === 27) { // Escape：失焦。
                el.blur();
                e.stopPropagation();
            }
            e.stopPropagation();
        });
        // 阻止 RMMV 的 TouchInput 处理输入框上的鼠标/触摸事件。
        el.addEventListener('mousedown', function (e) { e.stopPropagation(); });
        el.addEventListener('touchstart', function (e) { e.stopPropagation(); });
        el.addEventListener('keyup', function (e) { e.stopPropagation(); });
        document.body.appendChild(el);
    };

    /**
     * 重新计算 HTML 输入框在页面中的绝对位置。
     * 考虑画布缩放和偏移，确保输入框与聊天框底部对齐。
     * @private
     */
    ChatBox.prototype._positionInput = function () {
        if (!this._inputEl) return;
        var el = this._inputEl;
        // 获取画布在页面中的实际位置和缩放比。
        var canvas = Graphics._canvas || document.querySelector('canvas');
        var rect = canvas ? canvas.getBoundingClientRect() : { left: 0, top: 0, width: Graphics.boxWidth, height: Graphics.boxHeight };
        var scaleX = rect.width / Graphics.boxWidth;
        var scaleY = rect.height / Graphics.boxHeight;
        el.style.left = Math.round(rect.left + this.x * scaleX) + 'px';
        el.style.top = Math.round(rect.top + (this.y + CHAT_H - INPUT_H) * scaleY) + 'px';
        el.style.width = Math.round(CHAT_W * scaleX) + 'px';
        el.style.height = Math.round(INPUT_H * scaleY) + 'px';
    };

    /**
     * 显示聊天框及 HTML 输入框。
     */
    ChatBox.prototype.show = function () {
        this.visible = true;
        if (this._inputEl) this._inputEl.style.display = 'block';
        this._positionInput();
    };

    /**
     * 隐藏聊天框及 HTML 输入框。
     */
    ChatBox.prototype.hide = function () {
        this.visible = false;
        if (this._inputEl) {
            this._inputEl.blur();
            this._inputEl.style.display = 'none';
        }
    };

    /**
     * 销毁聊天框：从 DOM 移除 HTML 输入框。
     */
    ChatBox.prototype.destroy = function () {
        if (this._inputEl && this._inputEl.parentNode) {
            this._inputEl.parentNode.removeChild(this._inputEl);
            this._inputEl = null;
        }
    };

    /**
     * 窗口缩放回调：保持聊天框在左下角并重定位输入框。
     * @param {number} oldWidth - 旧画布宽度
     * @param {number} oldHeight - 旧画布高度
     * @param {number} newWidth - 新画布宽度
     * @param {number} newHeight - 新画布高度
     */
    ChatBox.prototype.onResize = function (oldWidth, oldHeight, newWidth, newHeight) {
        this.x = 4;
        this.y = newHeight - CHAT_H - 4;
        this._positionInput();
    };

    /**
     * 重绘聊天框：背景 + 频道标签 + 消息日志。
     */
    ChatBox.prototype.refresh = function () {
        var c = this.bmp();
        c.clear();
        var w = this.cw(), h = this.ch();

        // 绘制半透明圆角背景。
        L2_Theme.fillRoundRect(c, 0, 0, w, h, L2_Theme.cornerRadius, 'rgba(13,13,26,0.70)');
        L2_Theme.strokeRoundRect(c, 0, 0, w, h, L2_Theme.cornerRadius, L2_Theme.borderDark);

        // 绘制频道标签栏。
        this._drawTabs(c, w);

        // 绘制消息日志区域。
        this._drawLog(c, w);
    };

    /**
     * 绘制频道标签栏。
     * 均分宽度（剩余像素分配给前几个标签），当前频道金色下划线高亮。
     * @param {Bitmap} c - 目标位图
     * @param {number} w - 面板宽度
     */
    ChatBox.prototype._drawTabs = function (c, w) {
        // 均分标签宽度，余数像素分配给前几个标签。
        var baseTabW = Math.floor(w / CHANNELS.length);
        var remainder = w - baseTabW * CHANNELS.length;

        var self = this;
        var currentX = 0;

        CHANNELS.forEach(function (ch, i) {
            var tabW = baseTabW + (i < remainder ? 1 : 0);
            var active = ch === $MMO._chatChannel;
            var hover = i === self._hoverTab;

            // 当前频道深色背景 + 底部金色线，悬停频道高亮背景。
            if (active) {
                c.fillRect(currentX, 0, tabW, TAB_H, '#1E1E38');
                c.fillRect(currentX, TAB_H - 2, tabW, 2, L2_Theme.textGold);
            } else if (hover) {
                c.fillRect(currentX, 0, tabW, TAB_H, L2_Theme.highlight);
            }

            // 标签文字（未读频道追加 ● 标记）。
            c.fontSize = 10;
            c.textColor = active ? L2_Theme.textGold : L2_Theme.textGray;
            var label = CHANNEL_LABELS[ch] || ch;
            if ($MMO._chatUnread[ch] > 0 && !active) label += ' ●';
            c.drawText(label, currentX, 1, tabW, TAB_H - 2, 'center');

            // 标签间分隔线。
            if (i > 0) c.fillRect(currentX, 3, 1, TAB_H - 6, L2_Theme.borderDark);

            currentX += tabW;
        });
        // 标签栏底部分隔线。
        c.fillRect(0, TAB_H, w, 1, L2_Theme.borderDark);
    };

    /**
     * 绘制消息日志区域。
     * 自动滚动到最新消息，支持滚动条显示。
     * @param {Bitmap} c - 目标位图
     * @param {number} w - 面板宽度
     */
    ChatBox.prototype._drawLog = function (c, w) {
        var ch = $MMO._chatChannel;
        var hist = $MMO._chatHistory[ch] || [];
        var logY = TAB_H + 1;
        var logBottom = CHAT_H - INPUT_H;
        var logH = logBottom - logY;
        var totalH = hist.length * LINE_H;

        // 自动滚动到底部（最新消息）。
        var maxScroll = Math.max(0, totalH - logH);
        this._scrollY = maxScroll;

        // 仅绘制可见范围内的消息行。
        var startIdx = Math.max(0, Math.floor(this._scrollY / LINE_H));
        var visCount = Math.ceil(logH / LINE_H) + 1;

        for (var i = startIdx; i < Math.min(startIdx + visCount, hist.length); i++) {
            var my = logY + (i * LINE_H - this._scrollY);
            if (my + LINE_H < logY || my > logBottom) continue;

            var msg = hist[i];
            c.fontSize = 12;
            c.textColor = CHANNEL_COLORS[msg.channel] || '#FFFFFF';
            // 格式：[频道首字母] 发送者: 消息内容。
            var text = '[' + (msg.channel || 'w').charAt(0).toUpperCase() + '] ';
            if (msg.sender) text += msg.sender + ': ';
            text += msg.text;
            c.drawText(text, PAD, my, w - PAD * 2, LINE_H, 'left');
        }

        // 消息总高度超过可视区域时绘制滚动条。
        if (totalH > logH) {
            var sbW = 4;
            var trackH = logH - 4;
            var thumbH = Math.max(12, Math.round(trackH * (logH / totalH)));
            var thumbY = logY + 2 + Math.round((trackH - thumbH) * (maxScroll > 0 ? this._scrollY / maxScroll : 0));
            c.fillRect(w - sbW, logY, sbW, logH, 'rgba(0,0,0,0.2)');
            L2_Theme.fillRoundRect(c, w - sbW, thumbY, sbW, thumbH, 2, '#444466');
        }
    };

    /**
     * 手动滚动消息日志。
     * @param {number} delta - 滚动方向（正数向下，负数向上）
     */
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

    /**
     * 切换到指定频道并清除未读计数。
     * @param {string} ch - 频道标识符
     */
    ChatBox.prototype.switchChannel = function (ch) {
        $MMO._chatChannel = ch;
        $MMO._chatUnread[ch] = 0;
        this._scrollY = 0;
        this.refresh();
    };

    /**
     * 每帧更新：输入框位置同步、拖拽处理、标签悬停/点击、滚轮滚动。
     */
    ChatBox.prototype.update = function () {
        L2_Base.prototype.update.call(this);
        if (!this.visible) return;

        // 每 60 帧同步 HTML 输入框位置（应对窗口缩放/全屏切换）。
        if (Graphics.frameCount % 60 === 0) this._positionInput();

        // 拖拽处理：拖拽中跳过点击逻辑。
        if ($MMO.updateDrag(this)) return;

        var mx = TouchInput.x - this.x;
        var my = TouchInput.y - this.y;
        var inside = mx >= 0 && mx < CHAT_W && my >= 0 && my < CHAT_H;

        // 频道标签悬停检测（使用均分宽度计算）。
        var baseTabW = Math.floor(CHAT_W / CHANNELS.length);
        var remainder = CHAT_W - baseTabW * CHANNELS.length;
        var oldHover = this._hoverTab;

        if (inside && my >= 0 && my < TAB_H) {
            var currentX = 0;
            this._hoverTab = -1;
            for (var i = 0; i < CHANNELS.length; i++) {
                var tabW = baseTabW + (i < remainder ? 1 : 0);
                if (mx >= currentX && mx < currentX + tabW) {
                    this._hoverTab = i;
                    break;
                }
                currentX += tabW;
            }
        } else {
            this._hoverTab = -1;
        }
        if (this._hoverTab !== oldHover) this.refresh();

        // 点击频道标签切换频道。
        if (inside && my < TAB_H && TouchInput.isTriggered() && this._hoverTab >= 0) {
            this.switchChannel(CHANNELS[this._hoverTab]);
        }

        // 日志区域鼠标滚轮滚动。
        if (inside && my >= TAB_H && my < CHAT_H - INPUT_H && TouchInput.wheelY) {
            this.scrollLog(TouchInput.wheelY);
        }
    };

    // ═══════════════════════════════════════════════════════════
    //  消息管理 — 添加消息与发送消息
    // ═══════════════════════════════════════════════════════════
    /** @type {ChatBox|null} 当前聊天框实例引用。 */
    var _chatBox = null;

    /**
     * 向指定频道添加一条消息。
     * 超过 MAX_MESSAGES 时移除最早的消息。
     * 非当前频道的消息增加未读计数。
     * @param {Object} msg - 消息对象 {channel, sender, text}
     */
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

    /**
     * 发送聊天消息。
     * 支持斜杠命令：
     * - /resetpos — 请求位置重置
     * - /all, /world — 切换到世界频道
     * - /party, /p — 切换到队伍频道
     * - /guild, /g — 切换到公会频道
     * - /pm, /w <玩家> <消息> — 发送私聊
     * @param {string} text - 原始输入文本
     */
    function sendMessage(text) {
        if (!text || !text.trim()) return;
        text = text.trim();
        var channel = $MMO._chatChannel;
        var targetID = null;
        // 解析斜杠命令。
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

    // ═══════════════════════════════════════════════════════════
    //  RMMV 输入屏蔽 — 聊天输入聚焦时阻止游戏键盘输入
    // ═══════════════════════════════════════════════════════════

    /** @type {Function} 原始 Input.isPressed 方法。 */
    var _Input_isPressed = Input.isPressed.bind(Input);
    /**
     * 覆写 Input.isPressed：聊天输入聚焦时返回 false。
     * @param {string} keyName - 键名
     * @returns {boolean}
     */
    Input.isPressed = function (keyName) {
        if (_chatBox && _chatBox._inputFocused) return false;
        return _Input_isPressed(keyName);
    };

    /** @type {Function} 原始 Input.isTriggered 方法。 */
    var _Input_isTriggered = Input.isTriggered.bind(Input);
    /**
     * 覆写 Input.isTriggered：聊天输入聚焦时返回 false。
     * @param {string} keyName - 键名
     * @returns {boolean}
     */
    Input.isTriggered = function (keyName) {
        if (_chatBox && _chatBox._inputFocused) return false;
        return _Input_isTriggered(keyName);
    };

    /** @type {Function} 原始 Input.isRepeated 方法。 */
    var _Input_isRepeated = Input.isRepeated.bind(Input);
    /**
     * 覆写 Input.isRepeated：聊天输入聚焦时返回 false。
     * @param {string} keyName - 键名
     * @returns {boolean}
     */
    Input.isRepeated = function (keyName) {
        if (_chatBox && _chatBox._inputFocused) return false;
        return _Input_isRepeated(keyName);
    };

    /**
     * Enter 键全局监听器：聊天未聚焦时按 Enter 自动聚焦输入框。
     * @param {KeyboardEvent} e - 键盘事件
     */
    var _chatKeydownHandler = function (e) {
        if (_chatBox && _chatBox._inputFocused) return;
        if (e.keyCode === 13) { // Enter
            if (_chatBox && _chatBox._inputEl) _chatBox._inputEl.focus();
            e.preventDefault();
        }
    };
    window.addEventListener('keydown', _chatKeydownHandler);

    // ═══════════════════════════════════════════════════════════
    //  注入 Scene_Map — 创建聊天框
    // ═══════════════════════════════════════════════════════════

    /** @type {Function} 原始 Scene_Map.createAllWindows 引用。 */
    var _Scene_Map_createAllWindows6 = Scene_Map.prototype.createAllWindows;

    /**
     * 覆写 Scene_Map.createAllWindows：追加创建聊天框。
     */
    Scene_Map.prototype.createAllWindows = function () {
        _Scene_Map_createAllWindows6.call(this);
        _chatBox = new ChatBox();
        this.addChild(_chatBox);
        $MMO.registerBottomUI(_chatBox);
    };

    /** @type {Function} 原始 Scene_Map.start 引用。 */
    var _Scene_Map_start_chat = Scene_Map.prototype.start;

    /**
     * 覆写 Scene_Map.start：场景开始时显示聊天框。
     */
    Scene_Map.prototype.start = function () {
        _Scene_Map_start_chat.call(this);
        if (_chatBox) _chatBox.show();
    };

    /** @type {Function} 原始 Scene_Map.terminate 引用。 */
    var _Scene_Map_terminate3 = Scene_Map.prototype.terminate;

    /**
     * 覆写 Scene_Map.terminate：清理聊天框和键盘监听器。
     */
    Scene_Map.prototype.terminate = function () {
        _Scene_Map_terminate3.call(this);
        if (_chatBox) {
            $MMO.unregisterBottomUI(_chatBox);
            _chatBox.hide();
            _chatBox.destroy();
            _chatBox = null;
        }
        window.removeEventListener('keydown', _chatKeydownHandler);
    };

    // ═══════════════════════════════════════════════════════════
    //  WebSocket 消息处理器
    // ═══════════════════════════════════════════════════════════

    /**
     * reset_pos：位置重置结果。
     * 成功时传送玩家到目标坐标，失败时显示错误消息。
     */
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

    /**
     * chat_recv：收到其他玩家的聊天消息。
     */
    $MMO.on('chat_recv', function (data) {
        addMessage({ channel: data.channel || 'world', sender: data.from_name || '', text: data.content || '' });
    });

    /**
     * system_notice：系统通知消息。
     */
    $MMO.on('system_notice', function (data) {
        addMessage({ channel: 'system', sender: 'SYSTEM', text: data.message || '' });
    });

    /**
     * battle_result：战斗结果日志。
     */
    $MMO.on('battle_result', function (data) {
        if (!data) return;
        var text = 'You dealt ' + (data.damage || 0) + ' damage';
        if (data.is_crit) text += ' (CRITICAL!)';
        addMessage({ channel: 'battle', sender: '', text: text });
    });

})();
