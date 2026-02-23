# Task 03 - 战斗系统与怪物 AI（Battle & Monster AI）

> **优先级**：P1 — M2 里程碑
> **里程碑**：M2（击杀怪物正常获得经验，掉落物可拾取）
> **依赖**：Task 02（MapRoom + 玩家移动）

---

## 目标

实现即时战斗系统：玩家攻击/技能请求 → 服务端伤害计算 → 广播战斗结果。实现怪物行为树 AI（巡逻/追击/攻击）、怪物刷新、经验/掉落结算、掉落物拾取。

---

## Todolist

- [ ] **03-1** 实现伤害公式解析器（`game/battle/formula.go`）
  - [ ] 白名单表达式求值（只允许四则运算 + a/b 属性访问 + Math.xxx）
  - [ ] 安全：禁止 JS 注入，禁止访问非属性字段
- [ ] **03-2** 实现伤害计算流水线（`game/battle/damage.go`）
  - [ ] 套用 Skills.json damage.formula
  - [ ] 元素属性倍率（elementRates trait）
  - [ ] 状态修正（当前 Buff 的 trait）
  - [ ] 暴击判定（luk 参数）
  - [ ] 伤害浮动（±10%）
  - [ ] 最终伤害下限 0
- [ ] **03-3** 实现普通攻击处理（`HandleAttack` in `game/battle/`）
  - [ ] 距离校验（攻击范围内）
  - [ ] 调用伤害计算
  - [ ] 广播 `battle_result`
  - [ ] 目标 HP 归零时触发死亡流程
- [ ] **03-4** 实现怪物实例（`game/world/monster.go`）
  - [ ] `MonsterInstance` struct（来自 Enemies.json 的模板 + 实例状态）
  - [ ] 怪物刷新管理（`game/world/spawner.go`）：按地图配置定时刷新
- [ ] **03-5** 实现怪物 AI 行为树（`game/ai/`）
  - [ ] 基础节点：Sequence、Selector、Condition、Action
  - [ ] 条件：IsPlayerInRange, IsPlayerDetected, WasAttackedRecently
  - [ ] 行为：AttackPlayer, MoveTo（A* 寻路）, Wander, AlertNearby
  - [ ] 史莱姆 AI 示例（§7.2 的完整行为树）
- [ ] **03-6** A* 寻路算法（`game/ai/pathfinding.go`）
  - [ ] 基于地图通行性数据的 A* 实现
  - [ ] 路径缓存（同一目标短时间内复用路径）
- [ ] **03-7** 怪物死亡结算（`game/battle/loot.go`）
  - [ ] 掉落计算（Enemies.json dropItems[]，概率分母）
  - [ ] 经验分配（单人 100%，组队加成规则见 §7.1）
  - [ ] 广播 `monster_death{inst_id, drops[], exp}`
  - [ ] 掉落物生成：`drop_spawn` 广播，写入 MapRoom drops 集合
  - [ ] 怪物死亡后标记刷新计时器
- [ ] **03-8** 掉落物拾取处理（`HandlePickup`）
  - [ ] `pickup_item` 消息处理
  - [ ] 校验掉落物存在 + 玩家在范围内
  - [ ] 原子性加入背包（DB 事务）
  - [ ] 从 MapRoom drops 集合移除，广播 `drop_remove`
- [ ] **03-9** MapRoom Tick 集成战斗与 AI
  - [ ] 将 AI Tick 集成到 MapRoom.Tick()
  - [ ] 怪物移动广播 `monster_sync`
- [ ] **03-10** Hook 占位（为后续 Task 07 做准备）
  - [ ] 伤害计算前后调用 Hook 点（空实现，接口预留）
  - [ ] 怪物死亡后调用 Hook 点
- [ ] **03-11** 编写单元测试
  - [ ] formula_test.go：各类伤害公式求值正确性
  - [ ] damage_test.go：完整伤害计算流水线
  - [ ] loot_test.go：掉落概率计算（大量样本验证期望值）
  - [ ] ai_test.go：行为树节点执行逻辑

---

## 实现细节与思路

### 03-1 伤害公式白名单解析器

RPG Maker MV 的伤害公式是纯 JS 字符串，如 `"a.atk * 4 - b.def * 2"`。

**实现方案**：自定义简单表达式求值器（不用 eval/JS 引擎），解析以下内容：
- 变量：`a.atk`, `a.def`, `a.mat`, `a.mdf`, `a.agi`, `a.luk`, `a.hp`, `a.mp`, `a.level`（攻击方）
- 同上 `b.*`（防御方）
- 数字字面量：整数/浮点
- 运算符：`+`, `-`, `*`, `/`（四则运算，带括号优先级）
- 函数：`Math.floor`, `Math.ceil`, `Math.round`, `Math.max`, `Math.min`, `Math.abs`

**判断是否走白名单**（避免简单公式进 goja）：字符串不含 `if`/`function`/`var`/`let`/`const`/`;`/`{`/`}` 时走白名单求值器（<1μs），否则进 JS 沙箱（Task 08）。

**白名单求值器实现**：递归下降解析器（Recursive Descent Parser）：
```
Expr   = Term (('+' | '-') Term)*
Term   = Factor (('*' | '/') Factor)*
Factor = '(' Expr ')' | Number | Variable | Function '(' Expr (',' Expr)* ')'
Variable = 'a'.'field' | 'b'.'field'
```

### 03-2 伤害计算流水线

```go
type DamageContext struct {
    Attacker  *CharacterStats   // {atk, def, mat, mdf, agi, luk, hp, mp, level}
    Defender  *CharacterStats
    Skill     *resource.Skill   // 来自 Skills.json
    Buffs     []*BuffInstance   // 当前生效的 Buff
}

func Calculate(ctx *DamageContext) (finalDamage int, isCrit bool) {
    // ① 执行 before_damage_calc Hook（Task 07 接入，现在空实现）
    // ② 公式求值
    base := formulaEval(ctx.Skill.Damage.Formula, ctx.Attacker, ctx.Defender)
    // ③ 元素倍率
    base *= elementRate(ctx.Defender, ctx.Skill.Damage.ElementID)
    // ④ 状态修正（攻击方 buff 增伤、防御方 buff 减伤）
    base = applyBuffTraits(base, ctx.Buffs, ctx.Attacker, ctx.Defender)
    // ⑤ 暴击判定：暴击率 = 基础暴击率 + luk 修正
    isCrit = rand.Float64() < critRate(ctx.Attacker.Luk)
    if isCrit { base *= 1.5 }
    // ⑥ 浮动 ±10%
    base *= (0.9 + rand.Float64()*0.2)
    // ⑦ 下限 0
    finalDamage = max(0, int(math.Round(base)))
    // ⑧ 执行 after_damage_calc Hook（Task 07 接入）
    return
}
```

### 03-4 MonsterInstance

```go
type MonsterInstance struct {
    InstID    int64              // 实例唯一 ID（服务器自增）
    SpawnID   int                // 刷新点 ID（地图配置）
    Template  *resource.Enemy    // 来自 Enemies.json（只读）
    HP        int
    MaxHP     int
    X, Y      int
    Dir       int
    State     MonsterState       // Idle / Wander / Alert / Chase / Attack / Dead
    Target    *PlayerSession     // 当前追击目标
    AITree    *ai.BehaviorTree
    NextSpawn time.Time          // 死亡后的刷新时间
    mu        sync.Mutex
}
```

**刷新配置**（地图事件中的怪物刷新点，从 `Map*.json events` 解析，或在 config 中手动配置）：
```go
type SpawnConfig struct {
    MapID      int
    MonsterID  int     // Enemies.json ID
    X, Y       int     // 刷新位置
    MaxCount   int     // 最大同时存在数量
    RespawnSec int     // 死亡后刷新时间（秒）
}
```

### 03-5 行为树

```go
// game/ai/behavior_tree.go

type Node interface {
    Tick(ctx *AIContext) Status
}

type Status int
const (StatusSuccess Status = iota; StatusFailure; StatusRunning)

// Composite nodes
type Selector struct { Children []Node }
type Sequence struct { Children []Node }

// Leaf nodes (interface)
type ConditionNode struct { Fn func(*AIContext) bool }
type ActionNode    struct { Fn func(*AIContext) Status }
```

**AIContext**：
```go
type AIContext struct {
    Monster  *MonsterInstance
    Room     *MapRoom
    Resource *resource.ResourceLoader
    Cache    cache.Cache
    DeltaMS  int64   // 本帧时间（ms）
}
```

**史莱姆 AI 构建**（参见 §7.2）：
```go
func BuildSlimeAI() *BehaviorTree {
    return &BehaviorTree{Root: &Selector{Children: []Node{
        // 攻击范围内：攻击
        &Sequence{Children: []Node{
            &ConditionNode{Fn: IsPlayerInRange(2)},
            &ActionNode{Fn: AttackPlayer(skillID: 1)},
        }},
        // 已检测到玩家：追击
        &Sequence{Children: []Node{
            &ConditionNode{Fn: IsPlayerDetected(6)},
            &ActionNode{Fn: MoveTo(PathfindingAStar)},
        }},
        // 受攻击：警觉
        &Sequence{Children: []Node{
            &ConditionNode{Fn: WasAttackedRecently(3 * time.Second)},
            &ActionNode{Fn: AlertNearby(4)},
        }},
        // 默认：随机游走
        &ActionNode{Fn: Wander(radius: 3, interval: 3*time.Second)},
    }}}
}
```

### 03-6 A* 寻路

```go
// game/ai/pathfinding.go
func AStar(passability *resource.PassabilityMap, from, to Point) []Point
```

优化：
- 地图格子通常 <500×500，A* 足够
- 路径缓存：`monster.cachedPath` + `monster.cachedTarget`，若目标位置未变且路径有效则复用
- 每帧只移动一格（与 MapRoom Tick 频率一致）

### 03-7 经验分配规则

```
单人：exp = enemy.exp * 1.0
组队（在场队员 = 在同一地图 + 视野范围内）：
  expBonus = 1.0 + (memberCount - 1) * 0.1   （上限 1.4）
  eachExp = enemy.exp * expBonus / memberCount
  → 每人比单打获得更多总量，但个人量相对少
```

写入 DB 并推送 `exp_gain`，触发升级检查：
```go
if char.Exp >= ExpTable[char.Level+1] {
    char.Level++
    // 重新计算属性（来自 Classes.json params）
    // 推送 exp_gain{level_up: true, new_level: ...}
    // 调用 Hook: on_player_level_up（Task 07 接入）
}
```

### 03-8 掉落物

```go
type MapDrop struct {
    DropID    int64
    ItemType  int    // 1:物品 2:武器 3:防具
    ItemID    int
    Quantity  int
    X, Y      int
    OwnerID   int64  // 怪物击杀者，0=公共可拾取
    ExpireAt  time.Time  // config.Game.DropLifetimeSec 后过期
}
```

MapRoom 维护 `drops map[int64]*MapDrop`，`drop_cleanup` 调度任务（Task 07）清理过期掉落物。

---

## 关键 S2C 消息格式

**battle_result**：
```json
{
  "type": "battle_result",
  "payload": {
    "attacker_id": 20001,
    "target_id": 9001,
    "target_type": "monster",
    "damage": 128,
    "is_crit": false,
    "effects": [],
    "target_hp": 72,
    "target_max_hp": 200
  }
}
```

**monster_death**：
```json
{
  "type": "monster_death",
  "payload": {
    "inst_id": 9001,
    "drops": [{"drop_id": 1, "item_type": 1, "item_id": 5, "qty": 2, "x": 10, "y": 6}],
    "exp": 50
  }
}
```

---

## 验收标准

1. 玩家点击攻击 → 服务端收到请求 → 计算伤害 → 广播 `battle_result`
2. 怪物 HP 归零 → 广播 `monster_death` + `drop_spawn`，怪物消失
3. 玩家获得正确经验，组队经验加成正确
4. 怪物 AI 在 50ms 帧内完成行为树 Tick（<1ms 计算时间）
5. 怪物追击玩家：调用 A* 找路，每帧移动一格，广播 `monster_sync`
6. 拾取掉落物 → 背包增加物品，广播 `drop_remove`
7. 单元测试：伤害公式、掉落概率统计、AI 行为树节点逻辑
