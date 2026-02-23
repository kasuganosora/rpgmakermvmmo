# Task 09-04 - mmo-battle.js（即时战斗 UI）

> **优先级**：P1（M2）
> **里程碑**：M2
> **依赖**：task-09-01（mmo-core）、task-09-03（mmo-other-players，怪物精灵复用）

---

## 目标

替换 RMMV 踩地雷回合制战斗为即时战斗：玩家点击攻击/按技能键 → 发送 WS 请求 → 接收服务端结算结果 → 播放 RMMV 动画 + 显示伤害飘字。实现掉落物渲染与点击拾取。

---

## Todolist

- [ ] **04-1** 屏蔽默认战斗触发（随机踩地雷）
  - [ ] 覆盖 `Game_Player.prototype.checkEventTriggerTouch`，阻止随机战斗
  - [ ] 覆盖 `BattleManager.startBattle`，阻止本地战斗
- [ ] **04-2** 实现攻击输入（`Game_Player` Hook）
  - [ ] 左键点击地图上的怪物精灵 → 发送 `player_attack{target_id, target_type}`
  - [ ] 攻击按键（默认 Z/回车）对最近怪物自动攻击
  - [ ] 点击时高亮目标（外框闪烁效果）
- [ ] **04-3** 实现怪物精灵渲染（`Sprite_Monster`，复用 `Sprite_Character`）
  - [ ] 接收 `monster_spawn`：在地图创建怪物精灵（行走图来自 Enemies.json 的 battlerName 或配置）
  - [ ] 接收 `monster_sync`：更新位置（同 Sprite_OtherPlayer 的插值逻辑）
  - [ ] 接收 `monster_death`：播放死亡动画后移除精灵
  - [ ] 怪物头顶显示血条（HP 百分比，红色进度条）
- [ ] **04-4** 实现伤害动画（`battle_result` / `skill_effect` 处理）
  - [ ] 播放 RMMV 动画（`animationId`）到目标精灵（`$gameTemp.requestAnimation`）
  - [ ] 显示伤害飘字（`Sprite_DamagePopup` 子类，从目标位置浮上消失）
  - [ ] 暴击显示：字体更大 + 黄色
  - [ ] 治疗显示：绿色文字 + "+" 前缀
- [ ] **04-5** 实现掉落物渲染（`Sprite_MapDrop`）
  - [ ] 接收 `drop_spawn`：创建掉落物精灵（使用物品图标 `img/icons/`）
  - [ ] 点击掉落物 → 发送 `pickup_item{drop_id}`
  - [ ] 接收 `drop_remove`：移除掉落物精灵
  - [ ] 掉落物显示闪烁效果（引导玩家注意）
- [ ] **04-6** 玩家死亡/复活处理
  - [ ] 接收 `player_death`：若是自身，显示死亡界面（遮罩 + "您已死亡" 提示）
  - [ ] 接收 `player_revive`：若是自身，解除死亡界面，传送到复活坐标

---

## 实现细节与思路

### 屏蔽踩地雷

```javascript
// 屏蔽随机战斗触发
Game_Player.prototype.checkEventTriggerTouch = function (x, y) {
    // MMO 模式下不触发随机战斗
    if (!$gameMap.isEventRunning()) {
        this.checkEventTriggerThere([1, 2]);
    }
};
BattleManager.startBattle = function () {
    // 屏蔽本地战斗（仅 Script 触发的剧情战斗例外，由服务端 push battle_start 消息处理）
    console.warn('[MMO Battle] 本地战斗已被 MMO 模式屏蔽');
};
```

### 伤害飘字（Sprite_DamagePopup）

```javascript
function Sprite_DamagePopup(value, options) {
    this.initialize(value, options);
}
Sprite_DamagePopup.prototype = Object.create(Sprite.prototype);

Sprite_DamagePopup.prototype.initialize = function (value, opts) {
    Sprite.prototype.initialize.call(this);
    opts = opts || {};
    var text  = opts.isHeal ? '+' + value : String(value);
    var color = opts.isHeal ? '#44FF88' : (opts.isCrit ? '#FFD700' : '#FFFFFF');
    var size  = opts.isCrit ? 32 : 22;

    var bmp = new Bitmap(120, size + 8);
    bmp.fontSize  = size;
    bmp.textColor = color;
    bmp.outlineWidth = 3;
    bmp.drawText(text, 0, 0, 120, size + 8, 'center');
    this.bitmap = bmp;
    this.anchor.set(0.5, 1.0);

    this._vy      = -2.0;    // 初速度（向上）
    this._life    = 80;      // 帧生命周期
    this._maxLife = 80;
};

Sprite_DamagePopup.prototype.update = function () {
    Sprite.prototype.update.call(this);
    this.y      += this._vy;
    this._vy    *= 0.95;      // 减速
    this._life--;
    this.opacity = 255 * (this._life / this._maxLife);
    if (this._life <= 0) this.parent && this.parent.removeChild(this);
};

// 在 battle_result 处理中创建飘字
$MMO.on('battle_result', function (payload) {
    var targetSprite = findSpriteByID(payload.target_id, payload.target_type);
    if (!targetSprite) return;

    // 播放动画
    if (payload.animation_id) {
        $gameTemp.requestAnimation([targetSprite._character], payload.animation_id);
    }

    // 创建飘字
    var popup = new Sprite_DamagePopup(payload.damage, {
        isCrit: payload.is_crit,
        isHeal: false,
    });
    popup.x = targetSprite.x;
    popup.y = targetSprite.y;
    SceneManager._scene._spriteset.addChild(popup);

    // 更新目标血条
    if (payload.target_type === 'monster') {
        var mSprite = $MMO._monsterManager.get(payload.target_id);
        if (mSprite) mSprite.updateHpBar(payload.target_hp, payload.target_max_hp);
    }
});
```

### 怪物血条

```javascript
Sprite_Monster.prototype._createHpBar = function () {
    var bmp = new Bitmap(48, 6);
    this._hpBarBg  = new Sprite(new Bitmap(48, 6));
    this._hpBarFg  = new Sprite(bmp);
    this._hpBarBg.bitmap.fillAll('#333');
    this.addChild(this._hpBarBg);
    this.addChild(this._hpBarFg);
    this._hpBarBg.y = -70;
    this._hpBarFg.y = -70;
};

Sprite_Monster.prototype.updateHpBar = function (hp, maxHp) {
    var rate = Math.max(0, hp / maxHp);
    var color = rate > 0.5 ? '#44FF44' : (rate > 0.25 ? '#FFAA00' : '#FF3333');
    this._hpBarFg.bitmap.clear();
    this._hpBarFg.bitmap.fillRect(0, 0, Math.floor(48 * rate), 6, color);
};
```

---

## 验收标准

1. 点击地图上的怪物精灵 → 发送 `player_attack` 消息
2. 收到 `battle_result` → 播放动画 + 伤害飘字从怪物位置浮上
3. 怪物血条实时更新（颜色随 HP 百分比变化）
4. 怪物死亡 → 精灵消失，显示掉落物闪烁图标
5. 点击掉落物 → 发送 `pickup_item`，收到 `drop_remove` 后图标消失
6. 玩家死亡 → 显示死亡遮罩；收到 `player_revive` → 遮罩消失
