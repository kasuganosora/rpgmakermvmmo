# Task 09-05 - mmo-hud.js（HUD 层）

> **优先级**：P1（M3）
> **里程碑**：M3
> **依赖**：task-09-01（mmo-core）

---

## 目标

在 `Scene_Map` 上叠加 HUD 层：HP/MP/EXP 进度条（右上角）、小地图（右上角）、任务追踪面板（右中）、功能按钮 2×3 矩阵（右下角，含快捷键 Alt+C/G/I/K/F/Q）。

---

## Todolist

- [ ] **05-1** `Window_MMO_StatusBar`（HP/MP/EXP 条）
  - [ ] 三条进度条：HP（红）/ MP（蓝）/ EXP（黄），附带当前值/最大值文字
  - [ ] 接收 `player_sync` / `exp_gain` / `battle_result` 更新
  - [ ] 升级时播放闪光动画
- [ ] **05-2** `Window_Minimap`（小地图）
  - [ ] 渲染当前地图通行性遮罩（灰色背景 + 不通行格子深色）
  - [ ] 玩家位置（绿色圆点，自身）
  - [ ] 其他玩家（蓝色小点）
  - [ ] 怪物/NPC（红色小点）
  - [ ] 追踪任务目标（金色星形）
  - [ ] 接收 `map_init` 时初始化地图尺寸和通行性数据
  - [ ] 每帧更新点位
- [ ] **05-3** `Window_QuestTrack`（任务追踪，右中）
  - [ ] 显示最多 3 条追踪任务的名称和目标进度
  - [ ] 接收 `quest_update` 刷新
  - [ ] 进度完成时文字变绿（`已完成`）
- [ ] **05-4** `Window_MMO_Buttons`（功能按钮 2×3，右下）
  - [ ] 6 个按钮：玩家信息/公会/背包/技能/社交/任务日志
  - [ ] 快捷键：Alt+C / Alt+G / Alt+I / Alt+K / Alt+F / Alt+Q
  - [ ] 点击/快捷键打开对应子窗口（调用对应插件的 open 方法）
- [ ] **05-5** Hook `Scene_Map.prototype.createAllWindows`（追加 HUD 窗口）
- [ ] **05-6** HUD 显示/隐藏控制（对话框/菜单激活时自动隐藏）

---

## 实现细节与思路

### 布局参考（§2.2.4）

```
┌──────────────────────────────────────────────────────┐
│                                       [HP/MP/EXP 条]  │  ← 右上角 x: Graphics.width - 220
│                                       [小地图 150×150]│  ← HP条下方
│                 地图渲染区域                            │
│  [组队面板-左中]        [任务追踪-右中]                  │
│                                                       │
│  [聊天框]        [技能栏 12格]       [功能按钮 2×3]    │  ← 底部
└──────────────────────────────────────────────────────┘
```

### Window_MMO_StatusBar

```javascript
function Window_MMO_StatusBar() { this.initialize.apply(this, arguments); }
Window_MMO_StatusBar.prototype = Object.create(Window_Base.prototype);

Window_MMO_StatusBar.prototype.initialize = function () {
    var w = 220, h = 90;
    var x = Graphics.width - w - 4;
    var y = 4;
    Window_Base.prototype.initialize.call(this, x, y, w, h);
    this.opacity      = 0;    // 透明边框
    this.contentsOpacity = 200;
    this._data = { hp: 0, maxHp: 100, mp: 0, maxMp: 100, exp: 0, nextExp: 100 };
    this.refresh();
};

Window_MMO_StatusBar.prototype.refresh = function () {
    this.contents.clear();
    var d = this._data;
    this._drawBar(0,  0, d.hp,  d.maxHp,  '#FF4444', '#882222', 'HP');
    this._drawBar(0, 26, d.mp,  d.maxMp,  '#4488FF', '#224488', 'MP');
    this._drawBar(0, 52, d.exp, d.nextExp, '#FFD700', '#886600', 'EXP');
};

Window_MMO_StatusBar.prototype._drawBar = function (x, y, cur, max, fg, bg, label) {
    var w    = this.contentsWidth() - 40;
    var rate = Math.min(1, Math.max(0, cur / (max || 1)));
    this.contents.fillRect(x + 32, y + 6, w, 14, bg);
    this.contents.fillRect(x + 32, y + 6, Math.floor(w * rate), 14, fg);
    this.contents.fontSize = 13;
    this.contents.drawText(label, x, y + 4, 30, 16, 'right');
    this.contents.drawText(cur + '/' + max, x + 32, y + 4, w, 14, 'right');
};

// 接收服务端数据更新
$MMO.on('player_sync', function (p) {
    if (p.player_id !== $MMO.charId) return;
    if ($MMO._statusBar) {
        $MMO._statusBar._data.hp  = p.hp;
        $MMO._statusBar._data.mp  = p.mp;
        $MMO._statusBar.refresh();
    }
});
$MMO.on('exp_gain', function (p) {
    if ($MMO._statusBar) {
        $MMO._statusBar._data.exp     = p.total_exp;
        $MMO._statusBar._data.nextExp = p.next_level_exp;
        $MMO._statusBar.refresh();
        if (p.level_up) $MMO._statusBar._playLevelUpEffect();
    }
});
```

### Window_Minimap

```javascript
Window_Minimap.prototype.initialize = function () {
    var size = ($MMO_CONFIG.minimap || {}).size || 150;
    var x = Graphics.width - size - 4;
    var y = 98;   // HP 条下方
    Window_Base.prototype.initialize.call(this, x, y, size + 16, size + 16);
    this.opacity = 0;
    this._mapSize   = null;   // {width, height} 单位：格子
    this._passability = null;  // [y][x] boolean
    this._size = size;
};

Window_Minimap.prototype.setMapData = function (mapW, mapH, passability) {
    this._mapW       = mapW;
    this._mapH       = mapH;
    this._passability = passability;
    this._drawBase();   // 绘制静态背景（只需绘制一次）
};

Window_Minimap.prototype._drawBase = function () {
    var bmp = this.contents;
    bmp.fillAll('rgba(0,0,0,0.8)');
    var cw = this._size / this._mapW;
    var ch = this._size / this._mapH;
    for (var y = 0; y < this._mapH; y++) {
        for (var x = 0; x < this._mapW; x++) {
            if (!this._passability[y][x]) {
                bmp.fillRect(
                    Math.floor(x * cw), Math.floor(y * ch),
                    Math.ceil(cw), Math.ceil(ch),
                    'rgba(80,80,80,0.9)'
                );
            }
        }
    }
};

// 每帧 update：重绘动态点位（玩家/怪物/NPC）
Window_Minimap.prototype.update = function () {
    Window_Base.prototype.update.call(this);
    this._updateDots();
};
```

### 功能按钮快捷键

```javascript
// Hook Scene_Map.prototype.update 监听快捷键
var _Scene_Map_update = Scene_Map.prototype.update;
Scene_Map.prototype.update = function () {
    _Scene_Map_update.call(this);
    var ALT = Input._currentState['alt'];   // NW.js 下需要自定义 alt 键监听
    if (ALT) {
        if (Input.isTriggered('c')) $MMO._hud.openCharInfo();
        if (Input.isTriggered('g')) $MMO._hud.openGuild();
        if (Input.isTriggered('i')) $MMO._hud.openInventory();
        if (Input.isTriggered('k')) $MMO._hud.openSkills();
        if (Input.isTriggered('f')) $MMO._hud.openSocial();
        if (Input.isTriggered('q')) $MMO._hud.openQuestLog();
    }
};
```

---

## 验收标准

1. 右上角显示 HP/MP/EXP 进度条，受伤时 HP 条减少
2. 小地图正确显示当前地图轮廓（不可通行格子为深色）
3. 自身玩家在小地图显示绿色点，其他玩家蓝色点，怪物红色点
4. 任务追踪面板显示当前追踪任务进度，`quest_update` 后自动刷新
5. Alt+I 打开背包，Alt+Q 打开任务日志等快捷键正常工作
6. 与 NPC 对话时 HUD 自动隐藏，对话结束后恢复
