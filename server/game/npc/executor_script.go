// 脚本条件：使用 Goja JS VM 评估 RMMV Script 条件分支（类型 12）。
package npc

import (
	"context"
	"regexp"
	"strings"
	"time"

	"github.com/dop251/goja"
	"github.com/kasuganosora/rpgmakermvmmo/server/game/player"
	"go.uber.org/zap"
)

var metaTagRe = regexp.MustCompile(`<([^<>:]+)(:?)([^>]*)>`)

// setupChildRe 匹配 this.setupChild($dataCommonEvents[EXPR].list, 0) 模式。
var setupChildRe = regexp.MustCompile(`this\.setupChild\(\s*\$dataCommonEvents\[(.+?)\]\.list\s*,\s*\d+\s*\)`)

// execScriptCommand 执行 RMMV code 355 脚本命令。
// 处理以下模式：
//   - this.setupChild($dataCommonEvents[EXPR].list, 0) → 调用公共事件
//   - $gameVariables._data[N] += 1 等 → 变量变更
//   - $gameSwitches._data[N] = true 等 → 开关变更
//   - $gameScreen.* / AudioManager.* → 转发给客户端（由调用方处理）
//
// 返回 handled=true 表示脚本已被服务端处理（不需要转发）。
func (e *Executor) execScriptCommand(ctx context.Context, s *player.PlayerSession, script string, opts *ExecuteOpts, depth int) (handled bool) {
	// 1. 检测 setupChild 模式
	if m := setupChildRe.FindStringSubmatch(script); m != nil {
		ceID := e.evalSetupChildTarget(m[1], s, opts)
		if ceID > 0 {
			e.logger.Info("setupChild → callCommonEvent",
				zap.Int("ce_id", ceID), zap.String("expr", m[1]))
			e.callCommonEvent(ctx, s, ceID, opts, depth)
		} else {
			e.logger.Warn("setupChild: could not resolve CE ID",
				zap.String("expr", m[1]))
		}
		return true
	}

	// 2. 检测 $gameVariables._data 或 $gameSwitches._data 变更
	if strings.Contains(script, "$gameVariables._data") ||
		strings.Contains(script, "$gameSwitches._data") ||
		strings.Contains(script, "$gameVariables.setValue") {
		e.execMutableScript(script, s, opts)
		return true
	}

	return false
}

// evalSetupChildTarget 通过 Goja VM 评估 setupChild 的目标表达式。
// 例如 "$gameVariables.value(2503)" → 返回变量 2503 的值。
func (e *Executor) evalSetupChildTarget(expr string, s *player.PlayerSession, opts *ExecuteOpts) int {
	vm := goja.New()
	timer := time.AfterFunc(100*time.Millisecond, func() {
		vm.Interrupt("setupChild eval timeout")
	})
	defer timer.Stop()

	if opts != nil && opts.GameState != nil {
		injectScriptGameState(vm, opts.GameState)
	}
	if s != nil {
		injectScriptGameActors(vm, s)
	}

	v, err := vm.RunString(expr)
	if err != nil {
		return 0
	}
	if v == nil || goja.IsUndefined(v) || goja.IsNull(v) {
		return 0
	}
	id := int(v.ToInteger())
	// 负号处理：某些脚本使用 -$gameActors.actor(1).abu.chooseSk
	if id < 0 {
		id = -id
	}
	return id
}

// execMutableScript 在 Goja VM 中执行可变状态脚本。
// 使用 Proxy 拦截 _data 访问，将变更同步到 GameState。
func (e *Executor) execMutableScript(script string, s *player.PlayerSession, opts *ExecuteOpts) {
	if opts == nil || opts.GameState == nil {
		return
	}
	gs := opts.GameState

	vm := goja.New()
	timer := time.AfterFunc(100*time.Millisecond, func() {
		vm.Interrupt("mutable script timeout")
	})
	defer timer.Stop()

	for _, name := range []string{"require", "process", "fetch", "eval", "Function"} {
		vm.Set(name, goja.Undefined())
	}
	injectScriptMath(vm)

	mutations := &scriptMutations{
		varChanges:    make(map[int]int),
		switchChanges: make(map[int]bool),
	}

	injectScriptGameStateMutable(vm, gs, true, mutations)
	if s != nil {
		injectScriptGameActors(vm, s)
	}
	if e.res != nil {
		note := ""
		if md, ok := e.res.Maps[opts.MapID]; ok {
			note = md.Note
		}
		injectScriptDataMap(vm, note)
	}

	// 注入常用全局变量（用于某些脚本中的临时变量如 i, value 等）
	// 不做特殊处理，让 JS 自然解析

	var runErr error
	panicked := false
	func() {
		defer func() {
			if r := recover(); r != nil {
				panicked = true
			}
		}()
		_, runErr = vm.RunString(script)
	}()
	if panicked || runErr != nil {
		e.logger.Debug("mutable script error",
			zap.String("script", truncate(script, 120)),
			zap.Error(runErr))
		return
	}

	// 应用变量变更
	for id, val := range mutations.varChanges {
		gs.SetVariable(id, val)
		e.sendVarChange(s, id, val)
	}
	// 应用开关变更
	for id, val := range mutations.switchChanges {
		gs.SetSwitch(id, val)
		e.sendSwitchChange(s, id, val)
	}
	// 触发页面刷新
	if (len(mutations.switchChanges) > 0 || len(mutations.varChanges) > 0) && opts.PageRefreshFn != nil {
		opts.PageRefreshFn(s)
	}
}

// truncate 截断字符串到指定长度。
func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}


// evalScriptCondition 使用 Goja VM 评估 Script 条件（类型 12）。
// 注入 $gameSwitches、$gameVariables、$dataMap.meta、$gameActors 等 RMMV 全局对象。
// 返回 (result, true) 表示评估成功，(false, false) 表示脚本引用了未知全局对象。
func (e *Executor) evalScriptCondition(script string, s *player.PlayerSession, opts *ExecuteOpts) (bool, bool) {
	vm := goja.New()

	// 超时保护：100ms
	timer := time.AfterFunc(100*time.Millisecond, func() {
		vm.Interrupt("script condition timeout")
	})
	defer timer.Stop()

	// 安全限制
	for _, name := range []string{"require", "process", "fetch", "eval", "Function"} {
		vm.Set(name, goja.Undefined())
	}

	// 注入 Math（确定性随机数）
	injectScriptMath(vm)

	// 注入 $gameTemp（isPlaytest 在服务端始终返回 false）
	gt := vm.NewObject()
	_ = gt.Set("isPlaytest", func() bool { return false })
	vm.Set("$gameTemp", gt)

	// 注入 ConfigManager（服务端默认值）
	cm := vm.NewObject()
	vm.Set("ConfigManager", cm)

	// 注入 $gameParty（最小实现）
	// inBattle() 在事件执行期间始终返回 false（服务端不在战斗场景）。
	// leader() 返回 actor(1) 的最小对象。
	gp := vm.NewObject()
	_ = gp.Set("inBattle", func() bool { return false })
	_ = gp.Set("size", func() int { return 1 })
	if s != nil {
		leader := vm.NewObject()
		_ = leader.Set("_classId", s.ClassID)
		_ = leader.Set("actorId", func() int { return 1 })
		_ = gp.Set("leader", func() goja.Value { return leader })
		_ = gp.Set("members", func() []interface{} { return []interface{}{leader} })
	}
	vm.Set("$gameParty", gp)

	// 注入 $gameSwitches 和 $gameVariables
	if opts != nil && opts.GameState != nil {
		gs := opts.GameState
		injectScriptGameState(vm, gs)

		// 注入 getSelfVariable（TemplateEvent.js 扩展）。
		// 非严格模式下 this === globalThis，所以 this.getSelfVariable(N) 可用。
		mapID, eventID := opts.MapID, opts.EventID
		vm.Set("getSelfVariable", func(idx int) int {
			return gs.GetSelfVariable(mapID, eventID, idx)
		})
	}

	// 注入 $gameActors（从会话数据构建最小对象）
	if s != nil {
		injectScriptGameActors(vm, s)
	}

	// 注入 $dataMap.meta（从地图 Note 字段解析）
	// 始终注入 $dataMap（即使 note 为空），防止 $dataMap.meta[...] 抛出 ReferenceError。
	if opts != nil && e.res != nil {
		note := ""
		if md, ok := e.res.Maps[opts.MapID]; ok {
			note = md.Note
		}
		injectScriptDataMap(vm, note)
	} else {
		injectScriptDataMap(vm, "")
	}

	// 执行脚本，将结果转为布尔值（匹配 RMMV 的 !!eval(script) 行为）
	// 运行时错误（如访问 undefined 的属性）视为 false，与 RMMV 的 !!eval() 行为一致。
	var v goja.Value
	var runErr error
	panicked := false
	func() {
		defer func() {
			if r := recover(); r != nil {
				panicked = true
			}
		}()
		v, runErr = vm.RunString(script)
	}()
	if panicked || runErr != nil {
		return false, true // 运行时错误 = false（RMMV eval 抛出异常时条件不满足）
	}
	if v == nil || goja.IsUndefined(v) || goja.IsNull(v) {
		return false, true
	}
	return v.ToBoolean(), true
}

// evalScriptValue 使用 Goja VM 评估脚本表达式并返回整数值。
// 用于 operandType=4（脚本）的变量赋值，如 $gameActors._data[1]._equips[1]._itemId。
func (e *Executor) evalScriptValue(script string, s *player.PlayerSession, opts *ExecuteOpts) int {
	vm := goja.New()

	timer := time.AfterFunc(100*time.Millisecond, func() {
		vm.Interrupt("script value timeout")
	})
	defer timer.Stop()

	for _, name := range []string{"require", "process", "fetch", "eval", "Function"} {
		vm.Set(name, goja.Undefined())
	}

	injectScriptMath(vm)

	if opts != nil && opts.GameState != nil {
		injectScriptGameState(vm, opts.GameState)
	}
	if s != nil {
		injectScriptGameActors(vm, s)
	}

	var v goja.Value
	var runErr error
	panicked := false
	func() {
		defer func() {
			if r := recover(); r != nil {
				panicked = true
			}
		}()
		v, runErr = vm.RunString(script)
	}()
	if panicked || runErr != nil {
		return 0
	}
	if v == nil || goja.IsUndefined(v) || goja.IsNull(v) {
		return 0
	}
	return int(v.ToInteger())
}

// injectScriptMath 注入安全的 Math 对象。
func injectScriptMath(vm *goja.Runtime) {
	m := vm.NewObject()
	_ = m.Set("floor", func(v float64) float64 { return float64(int64(v)) })
	_ = m.Set("ceil", func(v float64) float64 {
		n := int64(v)
		if float64(n) < v {
			n++
		}
		return float64(n)
	})
	_ = m.Set("round", func(v float64) int64 { return int64(v + 0.5) })
	_ = m.Set("abs", func(v float64) float64 {
		if v < 0 {
			return -v
		}
		return v
	})
	_ = m.Set("max", func(a, b float64) float64 {
		if a > b {
			return a
		}
		return b
	})
	_ = m.Set("min", func(a, b float64) float64 {
		if a < b {
			return a
		}
		return b
	})
	_ = m.Set("random", func() float64 { return 0.5 })
	vm.Set("Math", m)
}

// injectScriptGameState 注入 $gameSwitches 和 $gameVariables。
// mutable=true 时添加 _data Proxy 支持直接下标读写（用于 code 355 脚本执行）。
func injectScriptGameState(vm *goja.Runtime, gs GameStateAccessor) {
	injectScriptGameStateMutable(vm, gs, false, nil)
}

// scriptMutations 收集脚本执行期间的变量/开关变更。
type scriptMutations struct {
	varChanges    map[int]int
	switchChanges map[int]bool
}

// injectScriptGameStateMutable 注入 $gameSwitches 和 $gameVariables。
// 当 mutable=true 时，_data 使用 Proxy 拦截读写操作，变更记录到 mutations 中。
func injectScriptGameStateMutable(vm *goja.Runtime, gs GameStateAccessor, mutable bool, mutations *scriptMutations) {
	sw := vm.NewObject()
	_ = sw.Set("value", func(id int) bool {
		if mutations != nil {
			if v, ok := mutations.switchChanges[id]; ok {
				return v
			}
		}
		return gs.GetSwitch(id)
	})
	if mutable && mutations != nil {
		// 注入 _data Proxy 实现 $gameSwitches._data[N] = true/false
		_, _ = vm.RunString(`
			var __swProxy = new Proxy({}, {
				get: function(t, p) {
					var id = parseInt(p);
					if (!isNaN(id)) return $gameSwitches.value(id);
					return undefined;
				},
				set: function(t, p, v) {
					var id = parseInt(p);
					if (!isNaN(id)) __setSw(id, !!v);
					return true;
				}
			});
		`)
		vm.Set("__setSw", func(id int, val bool) {
			mutations.switchChanges[id] = val
		})
		proxy := vm.Get("__swProxy")
		_ = sw.Set("_data", proxy)
	}
	vm.Set("$gameSwitches", sw)

	vars := vm.NewObject()
	_ = vars.Set("value", func(id int) interface{} {
		if mutations != nil {
			if v, ok := mutations.varChanges[id]; ok {
				return v
			}
		}
		return gs.GetVariable(id)
	})
	_ = vars.Set("setValue", func(id, val int) {
		if mutations != nil {
			mutations.varChanges[id] = val
		}
	})
	if mutable && mutations != nil {
		// 注入 _data Proxy 实现 $gameVariables._data[N] += 1 等操作
		_, _ = vm.RunString(`
			var __varProxy = new Proxy({}, {
				get: function(t, p) {
					var id = parseInt(p);
					if (!isNaN(id)) return $gameVariables.value(id);
					return undefined;
				},
				set: function(t, p, v) {
					var id = parseInt(p);
					if (!isNaN(id)) __setVar(id, typeof v === 'number' ? v : 0);
					return true;
				}
			});
		`)
		vm.Set("__setVar", func(id, val int) {
			mutations.varChanges[id] = val
		})
		proxy := vm.Get("__varProxy")
		_ = vars.Set("_data", proxy)
	}
	vm.Set("$gameVariables", vars)
}

// injectScriptDataMap 从地图 Note 字段解析 RMMV meta 标签并注入 $dataMap。
// RMMV 格式：<key> → meta[key]=true, <key:value> → meta[key]="value"（始终为字符串）。
func injectScriptDataMap(vm *goja.Runtime, note string) {
	dm := vm.NewObject()
	meta := vm.NewObject()
	for _, match := range metaTagRe.FindAllStringSubmatch(note, -1) {
		key := match[1]
		if match[2] == ":" {
			// RMMV 将 <key:value> 存储为字符串；JS 类型转换处理数值比较
			_ = meta.Set(key, match[3])
		} else {
			_ = meta.Set(key, true)
		}
	}
	_ = dm.Set("meta", meta)
	vm.Set("$dataMap", dm)
}

// injectScriptGameActors 注入最小的 $gameActors 对象。
// 支持 $gameActors.actor(1)._classId、$gameActors.actor(1).save 等常用脚本条件。
// actor(1).save 不设置（undefined），使 !$gameActors.actor(1).save 为 true（新角色初始化检查）。
// 访问 .save.key 等子属性时 JS 抛出 TypeError，evalScriptCondition 捕获后返回 false。
func injectScriptGameActors(vm *goja.Runtime, s *player.PlayerSession) {
	// 构建 actor 对象（仅 actor 1 = 当前玩家）
	actor1 := vm.NewObject()
	_ = actor1.Set("_classId", s.ClassID)

	// 注入 _equips 数组（RMMV Game_Item 对象数组）
	// 每个元素有 _itemId 属性，索引 = slot index
	if s.Equips != nil {
		// 找到最大 slot index 来确定数组大小
		maxSlot := 0
		for slot := range s.Equips {
			if slot > maxSlot {
				maxSlot = slot
			}
		}
		equipsArr := make([]interface{}, maxSlot+1)
		for i := 0; i <= maxSlot; i++ {
			item := vm.NewObject()
			_ = item.Set("_itemId", s.GetEquip(i))
			equipsArr[i] = item
		}
		_ = actor1.Set("_equips", equipsArr)
	} else {
		_ = actor1.Set("_equips", []interface{}{})
	}

	// $gameActors.actor(id) 方法
	ga := vm.NewObject()
	_ = ga.Set("actor", func(id int) goja.Value {
		if id == 1 {
			return actor1
		}
		return goja.Undefined()
	})

	// $gameActors._data[1] 直接访问模式
	data := vm.NewObject()
	_ = data.Set("1", actor1)
	_ = ga.Set("_data", data)

	vm.Set("$gameActors", ga)
}
