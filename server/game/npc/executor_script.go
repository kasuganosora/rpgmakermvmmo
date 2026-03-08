// 脚本条件：使用 Goja JS VM 评估 RMMV Script 条件分支（类型 12）。
package npc

import (
	"context"
	"encoding/json"
	"math"
	"regexp"
	"strings"
	"time"

	"github.com/dop251/goja"
	"github.com/kasuganosora/rpgmakermvmmo/server/game/player"
	"github.com/kasuganosora/rpgmakermvmmo/server/resource"
	"go.uber.org/zap"
)

// metaTagRe matches RMMV meta tags including kaeru.js semicolon extension.
// Format: <key> → true, <key:value> → string, <key;json> → JSON.parse(json).
var metaTagRe = regexp.MustCompile(`<([^<>:;]+)([:;]?)([^>]*)>`)

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

	// 3. 兜底：在条件 VM 中尽力执行脚本（best-effort）。
	//    处理 window.keyTemp=0、keyTemp++ 等裸变量操作。
	//    错误被静默忽略（如 $gameParty.gainItem 不存在）。
	//    不 return true —— 让调用方继续做 $gameScreen.*/AudioManager.* 安全转发。
	if opts != nil {
		vm := e.getOrCreateCondVM(s, opts)
		_, _ = vm.RunString(script)
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
	if e.res != nil {
		injectScriptDataArrays(vm, e.res)
	}
	if opts != nil {
		injectTransientVars(vm, opts.TransientVars)
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

	// Ensure TransientVars is initialized for cross-CE non-integer variable storage
	if opts != nil && opts.TransientVars == nil {
		opts.TransientVars = make(map[int]interface{})
	}
	mutations := &scriptMutations{
		varChanges:    make(map[int]int),
		switchChanges: make(map[int]bool),
		varAnyChanges: opts.TransientVars,
	}
	if mutations.varAnyChanges == nil {
		mutations.varAnyChanges = make(map[int]interface{})
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
		injectScriptDataArrays(vm, e.res)
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


// gojaCondVM holds a reusable Goja VM for script condition evaluation.
// All injected game state uses live closures, so the VM sees updated values
// without re-injection. Only created once per event execution chain.
type gojaCondVM struct {
	vm *goja.Runtime
}

// getOrCreateCondVM returns the cached condition VM, creating it on first use.
// The VM is stored in opts.cachedCondVM and reused for all subsequent calls.
func (e *Executor) getOrCreateCondVM(s *player.PlayerSession, opts *ExecuteOpts) *goja.Runtime {
	if opts.cachedCondVM != nil {
		return opts.cachedCondVM.vm
	}

	vm := goja.New()

	// 安全限制
	for _, name := range []string{"require", "process", "fetch", "eval", "Function"} {
		vm.Set(name, goja.Undefined())
	}

	injectScriptMath(vm)

	gt := vm.NewObject()
	_ = gt.Set("isPlaytest", func() bool { return false })
	vm.Set("$gameTemp", gt)

	cm := vm.NewObject()
	vm.Set("ConfigManager", cm)

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

	if opts.GameState != nil {
		injectScriptGameState(vm, opts.GameState)

		mapID, eventID := opts.MapID, opts.EventID
		gs := opts.GameState
		vm.Set("getSelfVariable", func(idx int) int {
			return gs.GetSelfVariable(mapID, eventID, idx)
		})
	}

	if s != nil {
		injectScriptGameActors(vm, s)
	}

	if opts.TransientVars != nil {
		injectTransientVars(vm, opts.TransientVars)
	}

	if e.res != nil {
		note := ""
		if md, ok := e.res.Maps[opts.MapID]; ok {
			note = md.Note
		}
		injectScriptDataMap(vm, note)
		injectScriptDataArrays(vm, e.res)
	} else {
		injectScriptDataMap(vm, "")
	}

	// window 对象：浏览器 JS 中 window 就是全局对象（window.x === x）。
	// 游戏脚本使用 window.keyList / window.keyTemp / window.Qcd 等存储临时状态。
	// 初始化 keyList=[] 使 CE 154 的掉落物循环在无掉落时正确跳过（0 >= 0 → break）。
	// 使用 globalThis 代理使 window.x = v 和 x 保持同步。
	_, _ = vm.RunString(`
		var keyList = [];
		var keyTemp = 0;
		var Qcd = false;
	`)
	// 将 window 设为全局对象自身的引用，使 window.x = v 等同于 x = v
	vm.Set("window", vm.GlobalObject())

	opts.cachedCondVM = &gojaCondVM{vm: vm}
	return vm
}

// evalScriptCondition 使用 Goja VM 评估 Script 条件（类型 12）。
// 复用 opts 中缓存的 VM 以避免重复创建（~125ms/次 → ~0.1ms/次）。
// 返回 (result, true) 表示评估成功，(false, false) 表示脚本引用了未知全局对象。
func (e *Executor) evalScriptCondition(script string, s *player.PlayerSession, opts *ExecuteOpts) (bool, bool) {
	vm := e.getOrCreateCondVM(s, opts)

	// 超时保护：100ms
	timer := time.AfterFunc(100*time.Millisecond, func() {
		vm.Interrupt("script condition timeout")
	})
	defer timer.Stop()

	// 执行脚本，将结果转为布尔值（匹配 RMMV 的 !!eval(script) 行为）
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
		return false, true
	}
	if v == nil || goja.IsUndefined(v) || goja.IsNull(v) {
		return false, true
	}
	return v.ToBoolean(), true
}

// evalScriptValue 使用缓存的 Goja VM 评估脚本表达式并返回整数值。
// 用于 operandType=4（脚本）的变量赋值，如 $gameActors._data[1]._equips[1]._itemId。
func (e *Executor) evalScriptValue(script string, s *player.PlayerSession, opts *ExecuteOpts) int {
	vm := e.getOrCreateCondVM(s, opts)

	timer := time.AfterFunc(100*time.Millisecond, func() {
		vm.Interrupt("script value timeout")
	})
	defer timer.Stop()

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
	_ = m.Set("floor", func(v float64) float64 { return math.Floor(v) })
	_ = m.Set("ceil", func(v float64) float64 { return math.Ceil(v) })
	_ = m.Set("round", func(v float64) float64 { return math.Round(v) })
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

// injectTransientVars patches $gameVariables.value() to also check transient vars
// (non-integer values like arrays stored during script execution chains).
func injectTransientVars(vm *goja.Runtime, transient map[int]interface{}) {
	if len(transient) == 0 {
		return
	}
	// Store lookup function accessible from JS — returns the value or undefined
	vm.Set("__getTransient", func(id int) interface{} {
		if v, ok := transient[id]; ok {
			return v
		}
		return goja.Undefined()
	})
	// Wrap existing value() to check transient vars first
	_, _ = vm.RunString(`
		(function() {
			var origValue = $gameVariables.value.bind($gameVariables);
			$gameVariables.value = function(id) {
				var tv = __getTransient(id);
				if (tv !== undefined) return tv;
				return origValue(id);
			};
		})();
	`)
}

// scriptMutations 收集脚本执行期间的变量/开关变更。
type scriptMutations struct {
	varChanges    map[int]int
	switchChanges map[int]bool
	// varAnyChanges stores non-integer variable values (arrays, strings etc.)
	// used by scripts like CE 937 that store arrays in game variables.
	varAnyChanges map[int]interface{}
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
			// Check non-integer values first (arrays from kaeru.js meta parsing)
			if v, ok := mutations.varAnyChanges[id]; ok {
				return v
			}
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
		// set handler accepts any type: numbers go to varChanges, others to varAnyChanges
		// (kaeru.js stores arrays like [40,10] from meta.Skill01 into game variables)
		_, _ = vm.RunString(`
			var __varProxy = new Proxy({}, {
				get: function(t, p) {
					var id = parseInt(p);
					if (!isNaN(id)) return $gameVariables.value(id);
					return undefined;
				},
				set: function(t, p, v) {
					var id = parseInt(p);
					if (!isNaN(id)) __setVarAny(id, v);
					return true;
				}
			});
		`)
		vm.Set("__setVarAny", func(call goja.FunctionCall) goja.Value {
			id := int(call.Argument(0).ToInteger())
			val := call.Argument(1)
			exported := val.Export()
			switch v := exported.(type) {
			case int64:
				mutations.varChanges[id] = int(v)
				delete(mutations.varAnyChanges, id)
			case float64:
				mutations.varChanges[id] = int(v)
				delete(mutations.varAnyChanges, id)
			default:
				// Arrays, strings, etc. — store as-is for script access
				mutations.varAnyChanges[id] = exported
			}
			return goja.Undefined()
		})
		proxy := vm.Get("__varProxy")
		_ = vars.Set("_data", proxy)
	}
	vm.Set("$gameVariables", vars)
}

// injectScriptDataMap 从地图 Note 字段解析 RMMV meta 标签并注入 $dataMap。
func injectScriptDataMap(vm *goja.Runtime, note string) {
	dm := vm.NewObject()
	_ = dm.Set("meta", parseMeta(vm, note))
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

// parseMeta 从 RMMV Note 字段解析 meta 标签。
// 格式（kaeru.js 扩展）：
//   <key> → meta[key]=true
//   <key:value> → meta[key]="value"（字符串）
//   <key;json> → meta[key]=JSON.parse(json)（数组/数值等原生类型）
func parseMeta(vm *goja.Runtime, note string) *goja.Object {
	meta := vm.NewObject()
	if note == "" {
		return meta
	}
	for _, match := range metaTagRe.FindAllStringSubmatch(note, -1) {
		key := match[1]
		switch match[2] {
		case ":":
			_ = meta.Set(key, match[3])
		case ";":
			// kaeru.js: semicolon means JSON.parse the value
			var parsed interface{}
			if err := json.Unmarshal([]byte(match[3]), &parsed); err == nil {
				_ = meta.Set(key, parsed)
			} else {
				_ = meta.Set(key, match[3])
			}
		default:
			_ = meta.Set(key, true)
		}
	}
	return meta
}

// injectScriptDataArrays 注入 $dataArmors/$dataWeapons/$dataSkills/$dataItems。
// 使用 ResourceLoader 中预构建的 Go 切片，通过 vm.ToValue() 批量转换，
// 避免逐个创建 goja 对象和运行时 parseMeta 正则。
func injectScriptDataArrays(vm *goja.Runtime, res *resource.ResourceLoader) {
	if res == nil {
		return
	}
	if res.PrebuiltArmors != nil {
		vm.Set("$dataArmors", res.PrebuiltArmors)
	}
	if res.PrebuiltWeapons != nil {
		vm.Set("$dataWeapons", res.PrebuiltWeapons)
	}
	if res.PrebuiltSkills != nil {
		vm.Set("$dataSkills", res.PrebuiltSkills)
	}
	if res.PrebuiltItems != nil {
		vm.Set("$dataItems", res.PrebuiltItems)
	}
}
