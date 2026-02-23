# Task 09-06 - mmo-skill-bar.js（技能快捷栏）

> **优先级**：P1（M3）
> **里程碑**：M3
> **依赖**：task-09-01（mmo-core）、task-09-05（mmo-hud，布局参考）

---

## 目标

实现底部 12 格技能快捷栏（F1-F12 热键）：显示技能图标、CD 倒计时遮罩、MP 不足时变灰。支持从技能列表拖拽绑定技能到格子。

---

## Todolist

- [ ] **06-1** `Window_SkillBar`（12 格技能快捷栏）
  - [ ] 12 个技能槽位（每格 48×48px，水平排列）
  - [ ] 槽位显示：技能图标（来自 `img/system/IconSet.png`，iconIndex 字段）
  - [ ] 槽位显示：快捷键标签（`F1`-`F12`，右下角小字）
  - [ ] MP 不足时图标变灰（`Bitmap.prototype.alphaColor` 遮罩）
  - [ ] 空槽位显示占位符图案
- [ ] **06-2** CD 倒计时遮罩
  - [ ] 接收 `player_sync` 或 `skill_effect` 后更新 CD 状态
  - [ ] 每帧计算剩余 CD 百分比，绘制扇形遮罩（顺时针消失）
  - [ ] CD 结束时播放短暂高亮动画
- [ ] **06-3** F1-F12 热键绑定（发送技能使用消息）
  - [ ] 读取槽位绑定的 skill_id
  - [ ] MP 不足 → 不发送，显示红色闪烁提示
  - [ ] 技能 CD 中 → 不发送，摇晃动画
  - [ ] 发送 `player_skill{skill_id, target_id, x, y}`
- [ ] **06-4** 拖拽绑定（可选，M3+ 实现）
  - [ ] 技能列表窗口（`Window_SkillList`）：显示角色已学技能
  - [ ] 鼠标拖拽技能图标到槽位 → 发送技能绑定请求（或本地保存）
- [ ] **06-5** 技能数据来源
  - [ ] 角色技能列表：服务端通过 `map_init` 或独立消息推送 `char_skills`
  - [ ] 或通过 `GET /api/characters/:id/skills` REST 拉取（登录角色选择时）

---

## 实现细节与思路

### 技能槽位渲染

RMMV 图标来自 `img/system/IconSet.png`（16 列 × N 行，每个图标 32×32px）：

```javascript
Window_SkillBar.prototype._drawSlot = function (index) {
    var slot   = this._slots[index];   // { skill_id, icon_index, cd_ends_at }
    var x      = index * 52 + 4;
    var bmp    = this.contents;

    // 清除格子
    bmp.clearRect(x, 4, 48, 48);
    bmp.strokeRect(x, 4, 48, 48, '#555555');   // 边框

    if (!slot || !slot.skill_id) {
        // 空格子
        bmp.fillRect(x + 1, 5, 46, 46, 'rgba(0,0,0,0.4)');
        return;
    }

    // 绘制技能图标（32×32 缩放到 44×44）
    var iconX = (slot.icon_index % 16) * 32;
    var iconY = Math.floor(slot.icon_index / 16) * 32;
    bmp.blt(ImageManager.loadSystem('IconSet'), iconX, iconY, 32, 32, x + 2, 6, 44, 44);

    // MP 不足时灰色遮罩
    if (slot.needMp > ($MMO._currentMp || 0)) {
        bmp.paintOpacity = 80;
        bmp.fillRect(x + 1, 5, 46, 46, 'rgba(0,0,0,0.5)');
        bmp.paintOpacity = 255;
    }

    // F 键标签
    bmp.fontSize = 10;
    bmp.textColor = '#AAAAAA';
    bmp.drawText('F' + (index + 1), x + 2, 44, 46, 14, 'right');
};
```

### CD 扇形遮罩

用 Canvas arc 绘制扇形倒计时：

```javascript
Window_SkillBar.prototype._drawCDMask = function (index) {
    var slot = this._slots[index];
    if (!slot || !slot.cd_ends_at) return;

    var now    = Date.now();
    var remain = slot.cd_ends_at - now;
    if (remain <= 0) return;

    var total = slot.cd_total_ms || 1;
    var rate  = remain / total;       // 1.0 → 0.0
    var x     = index * 52 + 4 + 24; // 格子中心 x
    var y     = 30;                   // 格子中心 y
    var r     = 22;

    var ctx = this.contents._canvas.getContext('2d');
    ctx.save();
    ctx.beginPath();
    ctx.moveTo(x, y);
    ctx.arc(x, y, r, -Math.PI / 2, -Math.PI / 2 + 2 * Math.PI * rate, false);
    ctx.fillStyle = 'rgba(0, 0, 0, 0.65)';
    ctx.fill();
    ctx.restore();

    // 剩余秒数文字
    this.contents.fontSize = 16;
    this.contents.textColor = '#FFFFFF';
    this.contents.drawText(
        Math.ceil(remain / 1000) + 's',
        index * 52 + 4, 20, 48, 20, 'center'
    );
};
```

### F1-F12 热键监听

RMMV 默认 Input 不支持 F1-F12，需要手动监听 DOM 事件：

```javascript
document.addEventListener('keydown', function (e) {
    if (e.key >= 'F1' && e.key <= 'F12') {
        e.preventDefault();
        var idx = parseInt(e.key.slice(1)) - 1;   // F1 → 0, F12 → 11
        $MMO._skillBar && $MMO._skillBar._useSkill(idx);
    }
});

Window_SkillBar.prototype._useSkill = function (index) {
    var slot = this._slots[index];
    if (!slot || !slot.skill_id) return;

    // CD 检查（客户端本地预判，防止无效请求）
    if (slot.cd_ends_at && Date.now() < slot.cd_ends_at) {
        this._playErrorAnimation(index);  // 摇晃动画
        return;
    }

    // 发送技能使用请求
    $MMO.send('player_skill', {
        skill_id:  slot.skill_id,
        target_id: $MMO._currentTarget && $MMO._currentTarget.id,
        x:         $gamePlayer.x,
        y:         $gamePlayer.y,
    });
};
```

### 接收 skill_effect 更新 CD

```javascript
$MMO.on('skill_effect', function (payload) {
    if (payload.caster_id !== $MMO.charId) return;
    // 更新对应技能槽的 CD
    var slot = $MMO._skillBar && $MMO._skillBar._findSlotBySkillId(payload.skill_id);
    if (slot) {
        slot.cd_ends_at    = Date.now() + (payload.cooldown_ms || 0);
        slot.cd_total_ms   = payload.cooldown_ms || 0;
    }
});
```

---

## 验收标准

1. 底部显示 12 格技能栏，已绑定技能显示正确图标
2. 按 F1 发送对应技能的 `player_skill` 消息
3. 技能 CD 期间格子有扇形遮罩 + 剩余秒数，CD 结束遮罩消失
4. MP 不足时格子变灰，按键无效
5. 拖拽技能到格子成功绑定（M3+ 验收）
