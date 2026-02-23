# Task 09-03 - mmo-other-players.js（其他玩家渲染）

> **优先级**：P0（M1 必须）
> **里程碑**：M1
> **依赖**：task-09-01（mmo-core）

---

## 目标

渲染地图上其他在线玩家的角色精灵（行走图）、头顶标签（昵称/职业/等级）。接收 `player_join` / `player_leave` / `player_sync` 消息，客户端做线性插值实现平滑移动。

---

## Todolist

- [ ] **03-1** `Sprite_OtherPlayer`（继承 `Sprite_Character`）
  - [ ] 初始化：绑定行走图 `setImage(walkName, walkIndex)`
  - [ ] 位置同步：`_targetX`/`_targetY` 插值目标（服务端坐标）
  - [ ] 每帧插值更新（`update`，平滑系数可配置，默认 0.3）
  - [ ] 方向更新（`setDirection`）
- [ ] **03-2** 头顶标签 `Sprite_PlayerLabel`（`Sprite_Base` 子类）
  - [ ] 绘制昵称（字体大小 14px）
  - [ ] 绘制职业图标（可选，用 RMMV face 图的一角作为职业标识）
  - [ ] 绘制等级（`Lv.5` 格式）
  - [ ] 公会名（可配置是否显示）
  - [ ] 状态颜色：普通=白、组队中=绿、PK模式=红、GM=金
- [ ] **03-3** `OtherPlayerManager`：管理所有其他玩家精灵
  - [ ] `add(playerData)` → 创建 `Sprite_OtherPlayer`，加入 Spriteset_Map
  - [ ] `remove(playerID)` → 从 Spriteset_Map 移除精灵
  - [ ] `update(syncData)` → 更新目标位置/方向/HP/状态
  - [ ] `get(playerID)` → 返回已有精灵
  - [ ] `clear()` → 地图切换时清空所有
- [ ] **03-4** WS 消息监听
  - [ ] `player_join`：`OtherPlayerManager.add(payload)`
  - [ ] `player_leave`：`OtherPlayerManager.remove(payload.player_id)`
  - [ ] `player_sync`：`OtherPlayerManager.update(payload)`
  - [ ] `map_init`：遍历 `payload.players`，对每个非自身玩家调用 `add`
- [ ] **03-5** Hook `Spriteset_Map.prototype.createCharacters`（在其后追加精灵层）
- [ ] **03-6** 地图切换时清理（Hook `Scene_Map.prototype.terminate`）

---

## 实现细节与思路

### 坐标系转换

RMMV 中角色位置使用 tile 坐标（格子单位），精灵屏幕坐标由 `$gameMap.tileWidth()` / `tileHeight()` 转换。`Sprite_Character` 已处理坐标转换，复用即可。

服务端推送的 `x`, `y` 也是 tile 坐标（与 RMMV 一致），直接赋值给 `_character._realX` / `_character._realY`。

### 插值平滑

```javascript
Sprite_OtherPlayer.prototype.updatePosition = function () {
    var alpha = 0.25;   // 插值系数（越大越跟手，越小越平滑）
    var char  = this._character;
    char._realX += (this._targetX - char._realX) * alpha;
    char._realY += (this._targetY - char._realY) * alpha;
    // 当距离极小时直接对齐，防止永远到不了目标
    if (Math.abs(char._realX - this._targetX) < 0.01) char._realX = this._targetX;
    if (Math.abs(char._realY - this._targetY) < 0.01) char._realY = this._targetY;
};
```

### 头顶标签绘制

```javascript
function Sprite_PlayerLabel(playerData) { this.initialize(playerData); }
Sprite_PlayerLabel.prototype = Object.create(Sprite.prototype);

Sprite_PlayerLabel.prototype.initialize = function (data) {
    Sprite.prototype.initialize.call(this);
    this._playerData = data;
    this._drawLabel();
};

Sprite_PlayerLabel.prototype._drawLabel = function () {
    var data = this._playerData;
    var text = data.name + '  Lv.' + data.level;
    if (data.guild_name) text += '\n[' + data.guild_name + ']';

    var bmp  = new Bitmap(200, data.guild_name ? 48 : 30);
    var color = this._getNameColor(data);
    bmp.drawText(text, 0, 0, 200, 30, 'center');
    bmp.textColor = color;
    bmp.fontSize  = 14;
    this.bitmap = bmp;

    // 精灵锚点：水平居中，垂直在角色头顶上方
    this.anchor.x = 0.5;
    this.anchor.y = 1.0;
    this.y = -52;  // 角色头顶偏移（可按角色高度调整）
};

Sprite_PlayerLabel.prototype._getNameColor = function (data) {
    if (data.is_gm)    return '#FFD700';   // GM - 金色
    if (data.is_pk)    return '#FF4444';   // PK - 红色
    if (data.in_party) return '#44FF88';   // 组队 - 绿色
    return '#FFFFFF';                       // 普通 - 白色
};
```

### 与 Spriteset_Map 集成

```javascript
var _Spriteset_Map_createCharacters = Spriteset_Map.prototype.createCharacters;
Spriteset_Map.prototype.createCharacters = function () {
    _Spriteset_Map_createCharacters.call(this);
    // 挂载 OtherPlayerManager 到 _tilemap，确保深度正确
    this._otherPlayerContainer = new PIXI.Container();
    this._tilemap.addChild(this._otherPlayerContainer);
    $MMO._otherPlayerContainer = this._otherPlayerContainer;
};
```

### map_init 处理

```javascript
$MMO.on('map_init', function (payload) {
    OtherPlayerManager.clear();
    (payload.players || []).forEach(function (p) {
        if (p.id !== $MMO.charId) {   // 排除自身
            OtherPlayerManager.add(p);
        }
    });
});
```

---

## 验收标准

1. 两个客户端进入同一地图 → 互相看到对方的角色精灵（行走图正确）
2. 角色头顶显示昵称 + 等级标签
3. 一方移动 → 另一方看到平滑移动（无瞬移跳跃，有线性插值过渡）
4. 一方断线 → 另一方看到其精灵消失（收到 `player_leave`）
5. 切换地图时旧地图的其他玩家精灵全部清除
