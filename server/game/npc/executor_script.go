// 脚本条件：使用 Goja JS VM 评估 RMMV Script 条件分支（类型 12）。
package npc

import (
	"regexp"
	"time"

	"github.com/dop251/goja"
	"github.com/kasuganosora/rpgmakermvmmo/server/game/player"
)

var metaTagRe = regexp.MustCompile(`<([^<>:]+)(:?)([^>]*)>`)

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
func injectScriptGameState(vm *goja.Runtime, gs GameStateAccessor) {
	sw := vm.NewObject()
	_ = sw.Set("value", func(id int) bool { return gs.GetSwitch(id) })
	vm.Set("$gameSwitches", sw)

	vars := vm.NewObject()
	_ = vars.Set("value", func(id int) interface{} { return gs.GetVariable(id) })
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
