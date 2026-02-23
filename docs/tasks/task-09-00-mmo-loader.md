# Task 09-00 - mmo-loader.js（主加载器插件）

> **优先级**：P0（客户端最高优先级，M1 前置）
> **里程碑**：M1 前置
> **依赖**：无（本插件是所有其他客户端插件的前提）

---

## 目标

实现 `mmo-loader.js`：用户只需在 `js/plugins.js` 注册这一个插件，Loader 自动按正确顺序加载其余所有 `mmo-*.js` 插件，并管理集中配置（服务器地址、调试开关等）。

---

## Todolist

- [ ] **00-1** 实现插件文件动态同步加载（NW.js `vm.runInThisContext`）
- [ ] **00-2** 实现加载失败降级（文件缺失时 warn + 跳过，不崩溃）
- [ ] **00-3** 实现集中配置管理（`mmo-config.json` 读取 + RMMV 插件参数兜底）
- [ ] **00-4** 实现版本/依赖检查（RMMV 版本 >= 1.6，NW.js 环境检测）
- [ ] **00-5** 实现加载进度 console 输出（方便调试）
- [ ] **00-6** 提供 `scripts/install.js` Node.js 安装脚本（自动修改 `js/plugins.js`）
- [ ] **00-7** 编写使用说明注释（插件参数说明、mmo-config.json 格式）

---

## 实现细节与思路

### 核心机制：同步加载

RPG Maker MV 运行在 NW.js（Node.js + Chromium）环境下。可以使用 Node.js 的 `vm` 模块同步执行其他插件文件，无需改动 RMMV 的插件系统：

```javascript
// js/plugins/mmo-loader.js

/*:
 * @plugindesc v1.0.0 MMO 套件加载器 - 只需注册此插件，自动加载全部 mmo-* 组件
 * @author MakerPGM-MMO
 *
 * @param serverUrl
 * @text 服务器地址
 * @type string
 * @default ws://localhost:8080
 * @desc WebSocket 服务器地址（wss:// 为加密连接）
 *
 * @param debug
 * @text 调试模式
 * @type boolean
 * @default false
 * @desc 开启后在 DevTools Console 中显示详细日志
 *
 * @param configFile
 * @text 配置文件路径
 * @type string
 * @default mmo-config.json
 * @desc 外部 JSON 配置文件路径（相对于游戏根目录），优先级高于上方参数
 *
 * @help
 * === MakerPGM-MMO 套件 ===
 *
 * 使用方法：
 *   1. 将整个 mmo/ 文件夹复制到 Project1/js/plugins/ 下
 *   2. 在 RPG Maker MV 插件管理器中只添加 "mmo-loader"
 *   3. 配置服务器地址参数（或在 mmo-config.json 中配置）
 *   4. 运行游戏
 *
 * 注意：本插件需要在 NW.js 桌面模式下运行（RPG Maker MV 默认环境）
 */

(function () {
    'use strict';

    // ─── 加载顺序（顺序不可随意调整） ───────────────────────────
    var PLUGIN_LOAD_ORDER = [
        'mmo-core',           // 1. 必须最先：WebSocket + 全局 $MMO 对象
        'mmo-auth',           // 2. 依赖 mmo-core
        'mmo-other-players',  // 3. 依赖 mmo-core
        'mmo-battle',         // 4. 依赖 mmo-core + mmo-other-players
        'mmo-hud',            // 5. 依赖 mmo-core
        'mmo-skill-bar',      // 6. 依赖 mmo-core + mmo-hud
        'mmo-inventory',      // 7. 依赖 mmo-core
        'mmo-chat',           // 8. 依赖 mmo-core
        'mmo-party',          // 9. 依赖 mmo-core
        'mmo-social',         // 10. 依赖 mmo-core
        'mmo-trade',          // 11. 依赖 mmo-core + mmo-inventory
    ];

    // ─── 读取配置 ────────────────────────────────────────────────
    var params = PluginManager.parameters('mmo-loader');

    function loadConfig() {
        var config = {
            serverUrl: params['serverUrl'] || 'ws://localhost:8080',
            debug:     params['debug'] === 'true',
        };

        // 尝试从外部 JSON 文件读取（覆盖插件参数）
        try {
            var fs       = require('fs');
            var path     = require('path');
            var cfgPath  = path.join(process.cwd(), params['configFile'] || 'mmo-config.json');
            if (fs.existsSync(cfgPath)) {
                var external = JSON.parse(fs.readFileSync(cfgPath, 'utf8'));
                Object.assign(config, external);
            }
        } catch (e) {
            // 文件不存在或解析失败，使用插件参数默认值
        }
        return config;
    }

    // ─── 动态同步加载插件 ────────────────────────────────────────
    function loadPlugin(name) {
        try {
            var fs   = require('fs');
            var path = require('path');
            var vm   = require('vm');

            var pluginPath = path.join(
                process.cwd(), 'js', 'plugins', name + '.js'
            );

            if (!fs.existsSync(pluginPath)) {
                console.warn('[MMO Loader] 插件文件不存在，已跳过：' + pluginPath);
                return false;
            }

            var code = fs.readFileSync(pluginPath, 'utf8');
            // 使用 vm.runInThisContext 在主 window 上下文中执行，
            // 效果等同于 <script> 标签加载，所有全局变量可见
            vm.runInThisContext(code, {
                filename: pluginPath,
                displayErrors: true,
            });

            if (window.$MMO && window.$MMO._debug) {
                console.log('[MMO Loader] ✓ 已加载：' + name);
            }
            return true;
        } catch (e) {
            console.error('[MMO Loader] 加载失败：' + name, e);
            return false;
        }
    }

    // ─── 主流程 ──────────────────────────────────────────────────
    (function bootstrap() {
        // 1. 检查运行环境
        if (typeof require !== 'function') {
            // 浏览器环境（非 NW.js）——使用异步 fallback
            console.warn('[MMO Loader] 非 NW.js 环境，尝试异步加载（开发调试用）');
            loadPluginsAsync(PLUGIN_LOAD_ORDER);
            return;
        }

        // 2. 检查 RMMV 版本
        if (typeof Utils !== 'undefined' && Utils.RPGMAKER_VERSION) {
            var ver = Utils.RPGMAKER_VERSION.split('.').map(Number);
            if (ver[0] < 1 || (ver[0] === 1 && ver[1] < 6)) {
                console.error('[MMO Loader] 需要 RPG Maker MV >= 1.6，当前版本：' + Utils.RPGMAKER_VERSION);
            }
        }

        // 3. 加载配置，挂载到全局（供所有插件使用）
        var config = loadConfig();
        window.$MMO_CONFIG = config;

        console.log('[MMO Loader] 开始加载 MMO 插件套件...');
        console.log('[MMO Loader] 服务器：' + config.serverUrl);

        // 4. 按顺序同步加载所有插件
        var loaded = 0, failed = [];
        PLUGIN_LOAD_ORDER.forEach(function (name) {
            if (loadPlugin(name)) {
                loaded++;
            } else {
                failed.push(name);
            }
        });

        console.log('[MMO Loader] 完成：' + loaded + '/' + PLUGIN_LOAD_ORDER.length + ' 个插件已加载');
        if (failed.length > 0) {
            console.warn('[MMO Loader] 以下插件未能加载：' + failed.join(', '));
        }
    })();

    // ─── 浏览器环境异步 Fallback（仅供 localhost 开发调试） ─────
    function loadPluginsAsync(names) {
        var index = 0;
        function next() {
            if (index >= names.length) return;
            var script = document.createElement('script');
            script.src = 'js/plugins/' + names[index] + '.js';
            script.onload  = function () { index++; next(); };
            script.onerror = function () {
                console.warn('[MMO Loader] 异步加载失败：' + names[index]);
                index++; next();
            };
            document.head.appendChild(script);
        }
        next();
    }

})();
```

### mmo-config.json 格式

放置在游戏根目录（与 `Game.exe` 同级）：

```json
{
    "serverUrl":      "wss://game.example.com",
    "debug":          false,
    "reconnectDelay": 1000,
    "maxReconnects":  10,
    "heartbeatSec":   30,
    "sessionTimeoutSec": 60,
    "minimap": {
        "size":     150,
        "opacity":  0.85
    },
    "chat": {
        "maxHistory": 100,
        "maxLength":  200
    },
    "skillBar": {
        "slots": 12
    }
}
```

`mmo-loader.js` 读取后挂载到 `window.$MMO_CONFIG`，各子插件直接读取该对象，无需重复定义配置。

### scripts/install.js（一键安装脚本）

提供 Node.js 脚本，自动修改 `js/plugins.js`，避免用户手动编辑出错：

```javascript
// scripts/install.js
// 用法：node scripts/install.js [game_path]
// 示例：node scripts/install.js "C:/RPGMakerProjects/Project1"

const fs   = require('fs');
const path = require('path');

const gamePath   = process.argv[2] || process.cwd();
const pluginsJs  = path.join(gamePath, 'js', 'plugins.js');
const pluginsDir = path.join(gamePath, 'js', 'plugins');

if (!fs.existsSync(pluginsJs)) {
    console.error('找不到 js/plugins.js，请确认游戏路径正确');
    process.exit(1);
}

// 读取现有 plugins.js
let content = fs.readFileSync(pluginsJs, 'utf8');

// 检查是否已安装
if (content.includes('"mmo-loader"')) {
    console.log('✓ mmo-loader 已安装，无需重复操作');
    process.exit(0);
}

// 移除旧的 mmo-* 条目（防止重复）
content = content.replace(/\s*\{[^}]*"mmo-[^"]*"[^}]*\},?\n?/g, '');

// 在 $plugins 数组末尾插入 mmo-loader
const entry = `    {"name":"mmo-loader","status":true,"description":"MMO 套件加载器","filename":"mmo-loader.js","parameters":{"serverUrl":"ws://localhost:8080","debug":"false","configFile":"mmo-config.json"}}`;
content = content.replace(/(\$plugins\s*=\s*\[)([\s\S]*?)(\];)/, function(_, open, middle, close) {
    const trimmed = middle.trimEnd().replace(/,\s*$/, '');
    return open + trimmed + (trimmed ? ',\n' : '\n') + entry + '\n' + close;
});

// 备份原文件
fs.copyFileSync(pluginsJs, pluginsJs + '.bak');
fs.writeFileSync(pluginsJs, content, 'utf8');

console.log('✓ 安装完成！已备份原 plugins.js 为 plugins.js.bak');
console.log('  请将 mmo/ 文件夹内容复制到：' + pluginsDir);
```

### 目录结构

```
Project1/
├── js/
│   ├── plugins.js              ← 只注册 mmo-loader（install.js 自动修改）
│   └── plugins/
│       ├── mmo-loader.js       ← 本文件（入口）
│       ├── mmo-core.js
│       ├── mmo-auth.js
│       └── ...
├── mmo-config.json             ← 用户配置（可选，优先级高于插件参数）
└── scripts/
    └── install.js              ← 一键安装脚本
```

---

## 验收标准

1. `js/plugins.js` 中只有 `mmo-loader` 一个 mmo 相关条目
2. 游戏启动时 Console 输出：`[MMO Loader] 完成：11/11 个插件已加载`
3. 某个插件文件缺失时：输出 warn 日志，其余插件正常加载（不崩溃）
4. `mmo-config.json` 存在时优先使用其 `serverUrl`，文件不存在时使用插件参数默认值
5. `node scripts/install.js` 成功修改 `plugins.js` 并备份原文件
6. RMMV 版本 < 1.6 时 Console 输出警告信息
