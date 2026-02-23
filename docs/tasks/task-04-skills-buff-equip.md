# Task 04 - 技能、Buff 与装备系统（Skills, Buff, Equipment）

> **优先级**：P2 — M3 里程碑
> **里程碑**：M3（技能 CD 正常、装备属性生效）
> **依赖**：Task 03（战斗系统，伤害计算流水线）

---

## 目标

实现技能 CD 管理（服务端权威）、Buff/Debuff 生命周期（施加/叠加/过期/DOT/HOT）、装备系统（穿戴/卸下/属性计算），以及客户端技能栏快捷槽绑定。

---

## Todolist

- [ ] **04-1** 实现技能使用处理（`game/skill/`）
  - [ ] 04-1a `HandleUseSkill` — 处理 `player_skill` 消息
  - [ ] 04-1b CD 校验与更新（Cache `player:{id}:skill_cd` Hash）
  - [ ] 04-1c MP 消耗校验与扣除
  - [ ] 04-1d 范围技能：AoE 目标收集（按技能 scope 参数）
  - [ ] 04-1e 广播 `skill_effect{caster_id, skill_id, targets[], animation_id}`
- [ ] **04-2** 实现 Buff 系统（`game/skill/buff.go`）
  - [ ] 04-2a `BuffInstance` struct + 在玩家/怪物上施加 Buff
  - [ ] 04-2b Buff Tick（DOT/HOT 效果，集成到 MapRoom Tick）
  - [ ] 04-2c Buff 过期移除
  - [ ] 04-2d Buff 叠加层数逻辑（来自 States.json maxTurns / restriction）
  - [ ] 04-2e 广播 `buff_update{target_id, buff_id, stacks, duration, action}`
  - [ ] 04-2f Buff 持久化：玩家 Buff 存入 Cache `player:{id}:buffs`（List JSON）
- [ ] **04-3** 实现装备系统（`game/item/equip.go`）
  - [ ] 04-3a `HandleEquipItem` — 处理 `equip_item` 消息
  - [ ] 04-3b 装备合法性校验（装备类型 + 职业限制来自 Armors/Weapons.json etypeId、wtypeId）
  - [ ] 04-3c 属性计算：基础属性 + 装备属性加成（params 数组）
  - [ ] 04-3d DB 更新：`inventories.equip_slot` 字段
  - [ ] 04-3e 广播 `equip_result{success, char_stats}`
- [ ] **04-4** 实现物品使用处理（`game/item/use.go`）
  - [ ] 04-4a `HandleUseItem` — 处理 `player_item` 消息
  - [ ] 04-4b 物品效果执行（HP 回复、MP 回复、施加 Buff，来自 Items.json effects[]）
  - [ ] 04-4c 消耗品数量扣除
  - [ ] 04-4d 广播 `inventory_update`
- [ ] **04-5** 实现背包管理（`game/item/inventory.go`）
  - [ ] 04-5a 增加物品（带堆叠）
  - [ ] 04-5b 移除物品
  - [ ] 04-5c 丢弃物品（`HandleDropItem`，生成地图掉落物）
  - [ ] 04-5d `GET /api/characters/:id/inventory` REST 接口
- [ ] **04-6** 实现技能栏快捷槽绑定（`game/skill/shortcut.go`）
  - [ ] 快捷槽 1-12 与 char_skills.shortcut 字段的读写
  - [ ] REST 接口或 WS 消息：`skill_bind{skill_id, slot}` → 更新 DB
- [ ] **04-7** 玩家属性汇总计算（`game/player/stats.go`）
  - [ ] `CalcStats(char, equips, buffs) *CharacterStats`
  - [ ] 基础属性（来自 Classes.json params[level]）+ 装备加成 + Buff trait 修改
  - [ ] 供伤害计算（Task 03）和 CD 检查使用
- [ ] **04-8** 编写单元测试
  - [ ] skill_cd_test.go：CD 正确扣减、CD 期间拒绝、CD 结束后可用
  - [ ] buff_test.go：施加 Buff、DOT 每 Tick 扣血、Buff 过期移除、叠加层数
  - [ ] equip_test.go：装备属性计算正确
  - [ ] inventory_test.go：物品增加/减少/堆叠

---

## 实现细节与思路

### 04-1b 技能 CD（Cache Hash）

```
Cache Key：player:{playerID}:skill_cd
Type：Hash
Field：strconv.Itoa(skillID)
Value：strconv.FormatInt(readyAtUnixMs, 10)

校验逻辑：
1. cache.HGet(ctx, key, skillIDStr) → readyAtStr
2. readyAt = parseInt(readyAtStr)
3. if now < readyAt → 返回错误 "Skill still on cooldown"
4. 使用成功后：cache.HSet(ctx, key, skillIDStr, strconv.FormatInt(now + skill.CooldownMs, 10))

注意：无 TTL，只要 Cache 活着就一直有效；服务重启 LocalCache 清空 → CD 自动重置（可接受）
```

### 04-1c MP 消耗

```go
func (svc *SkillService) checkAndConsumeMp(session *PlayerSession, skill *resource.Skill) error {
    // skill.MpCost = fixed cost
    // skill.TpCost（TP 系统，可选实现）
    if session.MP < skill.MpCost {
        return errors.New("not enough MP")
    }
    session.MP -= skill.MpCost
    // 异步写 Cache 位置缓存，DB 由 auto_save 定时写
    return nil
}
```

### 04-1d 范围技能（AoE）

RPG Maker MV Skills.json 中的 `scope` 字段：
```
1  = 一个敌人
2  = 一个敌人（随机 1-4 次攻击）
3  = 一排敌人（RMMV 概念，MMO 中=圆形范围 1格）
4  = 所有敌人
7  = 一个友军
8  = 友方全体
...
```
MMO 中简化处理：
- scope 1/2 → 单目标
- scope 4（全敌）/ 3（一排）→ 圆形范围收集（按技能配置的半径，建议写在技能备注 `<range: 3>` 中）
- scope 7/8 → 友方单体/全体（含自身）

**目标收集（AoE）**：
```go
func collectTargets(room *MapRoom, center Point, radius float64, team int) []Combatant {
    // team: 0=玩家, 1=怪物
    // 遍历 room.monsters（或 players），计算欧氏距离 <= radius
}
```

### 04-2 Buff 系统

**施加 Buff**（`AddBuff`）：
```go
func AddBuff(target Combatant, state *resource.State, duration time.Duration) {
    // 检查是否已有同 ID Buff
    existing := target.FindBuff(state.ID)
    if existing != nil {
        // 刷新持续时间（或叠加层数，取决于 States.json 的 maxTurns 配置）
        existing.ExpireAt = time.Now().Add(duration)
        existing.Stacks = min(existing.Stacks+1, state.MaxTurns)
    } else {
        target.Buffs = append(target.Buffs, &BuffInstance{
            BuffID:   state.ID,
            Stacks:   1,
            ExpireAt: time.Now().Add(duration),
            TickMS:   computeTickInterval(state),  // 来自 States.json 的 auto_removal_timing
        })
    }
    // 广播 buff_update{action: "add"}
}
```

**Buff Tick**（集成到 MapRoom Tick，每帧执行）：
```go
func tickBuffs(target Combatant, now time.Time) {
    for i := len(target.Buffs) - 1; i >= 0; i-- {
        buff := target.Buffs[i]
        // DOT（持续伤害，State trait restrictionType = 伤害）
        if now.After(buff.NextTick) {
            applyBuffTick(target, buff)
            buff.NextTick = now.Add(time.Duration(buff.TickMS) * time.Millisecond)
        }
        // 过期
        if buff.ExpireAt.Before(now) && !buff.ExpireAt.IsZero() {
            removeBuff(target, i)   // 广播 buff_update{action: "remove"}
        }
    }
}
```

**Buff trait 影响伤害计算**（在 Task 03 damage.go 中扩展）：
```
States.json traits[] 中 code=22 是属性倍率（paramRate）：
  dataId=0→maxHp, 1→maxMp, 2→atk, 3→def, 4→mat, 5→mdf, 6→agi, 7→luk
  value=1.2 → atk 增加 20%
```

### 04-3 装备系统

**装备槽位映射**（来自 RMMV 标准，Armors.etypeId）：
```
etypeId：1=盾 2=头盔 3=铠甲 4=饰品
Weapons.wtypeId：1=匕首 2=剑 3=...（游戏自定义）
Characters.json 中的 equips[] 定义默认装备
```

**属性计算**：
```go
// Characters 基础属性（Classes.json params[level][statIndex]）
// params[0]=maxHp, [1]=maxMp, [2]=atk, [3]=def, [4]=mat, [5]=mdf, [6]=agi, [7]=luk
baseStats := classes[char.ClassID].Params[char.Level]

// 装备属性加成（Weapons/Armors.json params 数组）
for _, equip := range char.Equips {
    for i, v := range equip.Params {
        baseStats[i] += v
    }
}

// Buff trait 倍率修改
for _, buff := range char.Buffs {
    for _, trait := range states[buff.BuffID].Traits {
        if trait.Code == 22 { // paramRate
            baseStats[trait.DataID] = int(float64(baseStats[trait.DataID]) * trait.Value)
        }
    }
}
```

### 04-5 背包管理

```go
type InventoryService struct {
    db    *gorm.DB
    cache cache.Cache
}

// AddItem：原子操作（DB 事务内）
// 1. 查找同类型同 ID 的现有格子（可堆叠时）
// 2. 堆叠 or 找空格子 INSERT
// 3. 推送 inventory_update{add: [{slot, item_type, item_id, quantity}]}
func (s *InventoryService) AddItem(ctx context.Context, charID int64, itemType, itemID, qty int) error

// RemoveItem：原子操作
// 扣减数量，归零则删除行
func (s *InventoryService) RemoveItem(ctx context.Context, charID int64, itemType, itemID, qty int) error
```

**堆叠规则**（来自 Items.json）：
- `item.consumable = true` → 可堆叠（同格子最大 99 个）
- 武器/防具：不堆叠（每件一行）

---

## 关键 S2C 消息格式

**skill_effect**：
```json
{
  "type": "skill_effect",
  "payload": {
    "caster_id": 20001,
    "skill_id": 5,
    "animation_id": 12,
    "targets": [
      {"target_id": 9001, "target_type": "monster", "damage": 250, "is_heal": false}
    ]
  }
}
```

**buff_update**：
```json
{
  "type": "buff_update",
  "payload": {
    "target_id": 20001,
    "target_type": "player",
    "buff_id": 3,
    "stacks": 1,
    "duration_ms": 10000,
    "action": "add"
  }
}
```

**inventory_update**：
```json
{
  "type": "inventory_update",
  "payload": {
    "add": [{"slot": 5, "item_type": 1, "item_id": 7, "quantity": 2}],
    "remove": [],
    "update": []
  }
}
```

---

## 验收标准

1. 使用技能后 CD 生效，CD 期间请求返回错误，CD 结束后可再次使用
2. 技能 MP 不足时被拒绝，MP 足够时正确扣减
3. Buff 施加后每帧 Tick，DOT 正确扣血，Buff 到期自动移除并广播
4. 装备穿上后属性正确更新（atk 增加反映在伤害计算中）
5. 拾取物品进入背包，背包满时拒绝拾取
6. 单元测试：CD 时序、Buff DOT 计算、装备属性叠加
