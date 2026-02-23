# Task 09-02 - mmo-auth.js（认证与角色场景）

> **优先级**：P0（M1 必须）
> **里程碑**：M1
> **依赖**：task-09-01（mmo-core，需要 `$MMO` 对象）

---

## 目标

替换 `Scene_Title` 为 `Scene_Login`（用户名/密码登录），实现 `Scene_CharacterSelect`（角色选择，最多3个槽位）和 `Scene_CharacterCreate`（角色创建，外观/职业选择）。Token 存内存，不写 localStorage。

---

## Todolist

- [ ] **02-1** `Scene_Login`：替换 Scene_Title
  - [ ] 用户名输入框（HTML `<input>` 或 RMMV Bitmap 文字输入）
  - [ ] 密码输入框（密文显示）
  - [ ] 登录按钮（调用 `POST /api/auth/login`）
  - [ ] 错误提示区域
  - [ ] 登录成功 → 跳转 `Scene_CharacterSelect`
- [ ] **02-2** `Scene_CharacterSelect`：角色选择
  - [ ] 最多 3 个角色卡片（行走图预览 + 名称/职业/等级/上次登录时间）
  - [ ] 新建角色按钮 → `Scene_CharacterCreate`
  - [ ] 删除角色按钮（需二次确认弹窗）
  - [ ] 进入游戏按钮 → 跳转地图场景并发送 `enter_map` WS 消息
  - [ ] 调用 `GET /api/characters` 加载角色列表
- [ ] **02-3** `Scene_CharacterCreate`：角色创建
  - [ ] 行走图选择（从服务端获取或本地 `img/characters/` 枚举）
  - [ ] 脸图选择（从 `img/faces/` 枚举）
  - [ ] 职业选择（从服务端 `$MMO_CONFIG.classes` 或 ResourceLoader 数据）
  - [ ] 角色名称输入框（唯一性由服务端校验）
  - [ ] 确认创建 → 调用 `POST /api/characters` → 返回选择界面
- [ ] **02-4** HTTP 工具函数（`$MMO.http`）
  - [ ] `$MMO.http.post(path, body)` → Promise
  - [ ] `$MMO.http.get(path)` → Promise
  - [ ] 自动附加 `Authorization: Bearer {token}` Header
  - [ ] 自动解析 JSON 响应，错误时 reject 并附带 `message`
- [ ] **02-5** 场景跳转 Hook（拦截 `Scene_Title`）

---

## 实现细节与思路

### 场景跳转拦截

```javascript
// 拦截 Scene_Title → Scene_Login
var _SceneManager_goto = SceneManager.goto;
SceneManager.goto = function (sceneClass) {
    if (sceneClass === Scene_Title) {
        sceneClass = Scene_Login;
    }
    _SceneManager_goto.call(this, sceneClass);
};

// 拦截 Scene_Boot.startNormalGame（新游戏启动）
var _Scene_Boot_startNormalGame = Scene_Boot.prototype.startNormalGame;
Scene_Boot.prototype.startNormalGame = function () {
    // 不调用 super，改为跳转到登录场景
    SceneManager.goto(Scene_Login);
};
```

### HTTP 工具

```javascript
// 挂载到 $MMO.http（mmo-auth.js 在 mmo-core 之后加载，$MMO 已存在）
$MMO.http = {
    _baseUrl: function () {
        var wsUrl = $MMO_CONFIG.serverUrl || 'ws://localhost:8080';
        return wsUrl.replace(/^ws/, 'http');  // ws → http, wss → https
    },
    _headers: function () {
        var h = { 'Content-Type': 'application/json' };
        if ($MMO.token) h['Authorization'] = 'Bearer ' + $MMO.token;
        return h;
    },
    get: function (path) {
        return fetch(this._baseUrl() + path, { headers: this._headers() })
            .then(function (r) { return r.json(); })
            .then(function (data) {
                if (data.error) return Promise.reject(new Error(data.error));
                return data;
            });
    },
    post: function (path, body) {
        return fetch(this._baseUrl() + path, {
            method: 'POST',
            headers: this._headers(),
            body: JSON.stringify(body),
        }).then(function (r) { return r.json(); })
          .then(function (data) {
              if (data.error) return Promise.reject(new Error(data.error));
              return data;
          });
    },
};
```

### Scene_Login 结构

使用 HTML `<input>` 叠加在 Canvas 上（RMMV NW.js 环境下可直接操作 DOM）：

```javascript
function Scene_Login() { this.initialize.apply(this, arguments); }
Scene_Login.prototype = Object.create(Scene_Base.prototype);
Scene_Login.prototype.constructor = Scene_Login;

Scene_Login.prototype.create = function () {
    Scene_Base.prototype.create.call(this);
    this._createBackground();
    this._createHtmlForm();    // 用 HTML <input> 处理文字输入
    this._createLoginButton(); // RMMV Window_Command 风格的按钮
    this._createErrorWindow();
};

Scene_Login.prototype._createHtmlForm = function () {
    // 创建叠加在 Canvas 上的 HTML 输入元素
    this._usernameInput = this._createInput('text',     '用户名', 300, 250);
    this._passwordInput = this._createInput('password', '密码',  300, 310);
};

Scene_Login.prototype._createInput = function (type, placeholder, x, y) {
    var input = document.createElement('input');
    input.type        = type;
    input.placeholder = placeholder;
    input.style.cssText = [
        'position: absolute',
        'left: ' + x + 'px',
        'top: '  + y + 'px',
        'width: 200px',
        'font-size: 18px',
        'padding: 4px 8px',
        'background: rgba(0,0,0,0.7)',
        'color: #fff',
        'border: 1px solid #888',
        'outline: none',
        'z-index: 100',
    ].join(';');
    document.body.appendChild(input);
    this._htmlElements = this._htmlElements || [];
    this._htmlElements.push(input);
    return input;
};

Scene_Login.prototype.terminate = function () {
    Scene_Base.prototype.terminate.call(this);
    // 场景结束时移除 HTML 元素，防止残留
    (this._htmlElements || []).forEach(function (el) { el.remove(); });
};

Scene_Login.prototype._onLogin = function () {
    var self     = this;
    var username = this._usernameInput.value.trim();
    var password = this._passwordInput.value;
    if (!username || !password) {
        this._showError('请填写用户名和密码');
        return;
    }
    $MMO.http.post('/api/auth/login', { username: username, password: password })
        .then(function (data) {
            $MMO.token     = data.token;
            $MMO.accountId = data.account_id;
            SceneManager.goto(Scene_CharacterSelect);
        })
        .catch(function (e) {
            self._showError(e.message || '登录失败，请重试');
        });
};
```

### Scene_CharacterSelect

```javascript
Scene_CharacterSelect.prototype._loadCharacters = function () {
    var self = this;
    $MMO.http.get('/api/characters').then(function (data) {
        self._characters = data.characters || [];
        self._refreshCharacterCards();
    });
};

Scene_CharacterSelect.prototype._onEnterGame = function () {
    var char = this._characters[this._selectedIndex];
    $MMO.charId = char.id;
    // 先跳场景，enter_map 在 Scene_Map 的 start 中发送
    SceneManager.goto(Scene_Map);
};
```

### Scene_Map 接入（在 mmo-auth.js 中 Hook）

```javascript
var _Scene_Map_start = Scene_Map.prototype.start;
Scene_Map.prototype.start = function () {
    _Scene_Map_start.call(this);
    if ($MMO.charId && $MMO.token && !$MMO._mapEntered) {
        $MMO._mapEntered = true;
        // 建立 WS 连接，连接成功后发送 enter_map
        $MMO.connect($MMO_CONFIG.serverUrl);
        $MMO.on('$connected', function () {
            $MMO.send('enter_map', {
                map_id:  $gameMap.mapId(),
                char_id: $MMO.charId,
            });
        });
    }
};
```

---

## 验收标准

1. 游戏启动显示登录界面（非标题界面）
2. 正确用户名密码 → Token 存 `$MMO.token`，跳转角色选择
3. 错误密码 → 显示错误提示，停留在登录界面
4. 角色选择：显示已有角色卡片（行走图 + 名称/职业/等级）
5. 创建角色：名称重复时显示服务端错误信息
6. 进入游戏：`enter_map` WS 消息发送，收到 `map_init` 后地图正常显示
7. 场景切换时 HTML 输入框正确移除（无残留元素）
