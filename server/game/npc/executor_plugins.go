// 插件指令服务端执行：CulSkillEffect、ParaCheck 等纯计算型插件。
// 这些插件通过 Goja VM 执行，读写 $gameVariables/_data 和 $dataArmors.meta 等数据。
package npc

import (
	"context"
	"time"

	"github.com/dop251/goja"
	"github.com/kasuganosora/rpgmakermvmmo/server/game/player"
	"go.uber.org/zap"
)

// getPluginScript returns the loaded JS script for a server-exec plugin.
// Returns empty string if no config or script not found.
func (e *Executor) getPluginScript(name string) string {
	cfg := e.mmoConfig()
	if cfg == nil {
		return ""
	}
	if p, ok := cfg.ServerExecPlugins[name]; ok && p != nil {
		return p.LoadedScript
	}
	return ""
}

// getPluginTimeout returns the execution timeout for a server-exec plugin in ms.
// Defaults to 200ms if not configured.
func (e *Executor) getPluginTimeout(name string) time.Duration {
	cfg := e.mmoConfig()
	if cfg != nil {
		if p, ok := cfg.ServerExecPlugins[name]; ok && p != nil && p.Timeout > 0 {
			return time.Duration(p.Timeout) * time.Millisecond
		}
	}
	return 200 * time.Millisecond
}

// getPluginTagSkillRange returns the [start, end] range for TagSkillList post-processing.
// Returns (0, 0) if not configured.
func (e *Executor) getPluginTagSkillRange(name string) (int, int) {
	cfg := e.mmoConfig()
	if cfg != nil {
		if p, ok := cfg.ServerExecPlugins[name]; ok && p != nil && len(p.TagSkillRange) >= 2 {
			return p.TagSkillRange[0], p.TagSkillRange[1]
		}
	}
	return 0, 0
}

// execCulSkillEffect 在 Goja VM 中执行 CulSkillEffect 插件逻辑。
func (e *Executor) execCulSkillEffect(s *player.PlayerSession, opts *ExecuteOpts) {
	if opts == nil || opts.GameState == nil {
		return
	}
	scriptJS := e.getPluginScript("CulSkillEffect")
	if scriptJS == "" {
		e.logger.Warn("CulSkillEffect script not loaded")
		return
	}

	vm := goja.New()
	timeout := e.getPluginTimeout("CulSkillEffect")
	timer := time.AfterFunc(timeout, func() {
		vm.Interrupt("CulSkillEffect timeout")
	})
	defer timer.Stop()

	if opts.TransientVars == nil {
		opts.TransientVars = make(map[int]interface{})
	}
	mutations := &scriptMutations{
		varChanges:    make(map[int]int),
		switchChanges: make(map[int]bool),
		varAnyChanges: opts.TransientVars,
	}

	injectScriptMath(vm)
	injectScriptGameStateMutable(vm, opts.GameState, true, mutations)
	if s != nil {
		injectScriptGameActors(vm, s)
	}
	if e.res != nil {
		injectScriptDataArrays(vm, e.res)
	}

	var runErr error
	panicked := false
	func() {
		defer func() {
			if r := recover(); r != nil {
				panicked = true
			}
		}()
		_, runErr = vm.RunString(scriptJS)
	}()
	if panicked || runErr != nil {
		e.logger.Warn("CulSkillEffect execution failed", zap.Error(runErr))
		return
	}

	// Apply mutations to game state (collect all for batch send)
	for id, val := range mutations.varChanges {
		opts.GameState.SetVariable(id, val)
	}
	for id, val := range mutations.switchChanges {
		opts.GameState.SetSwitch(id, val)
	}

	// AddSkillEffectBase: apply TagSkillList base values + accumulated effects.
	// Range from MMOConfig.serverExecPlugins.CulSkillEffect.tagSkillListRange.
	tagStart, tagEnd := e.getPluginTagSkillRange("CulSkillEffect")
	if e.res != nil && e.res.TagSkillList != nil && tagStart > 0 {
		for i := tagStart; i <= tagEnd; i++ {
			entry := e.res.TagSkillList[i]
			if entry == nil {
				continue
			}
			addVal := opts.GameState.GetVariable(entry.AddVar)
			result := entry.BaseNum + addVal
			if result != opts.GameState.GetVariable(entry.BaseVar) {
				opts.GameState.SetVariable(entry.BaseVar, result)
				mutations.varChanges[entry.BaseVar] = result
			}
		}
	}

	// Batch send all changes in a single message
	e.sendStateBatch(s, mutations.varChanges, mutations.switchChanges)

	e.logger.Info("CulSkillEffect executed",
		zap.Int64("char_id", s.CharID),
		zap.Int("v1209", opts.GameState.GetVariable(1209)))
}

// execParaCheck 在 Goja VM 中执行 ParaCheck 插件核心逻辑。
func (e *Executor) execParaCheck(s *player.PlayerSession, opts *ExecuteOpts) {
	if opts == nil || opts.GameState == nil {
		return
	}
	scriptJS := e.getPluginScript("ParaCheck")
	if scriptJS == "" {
		e.logger.Warn("ParaCheck script not loaded")
		return
	}

	vm := goja.New()
	timeout := e.getPluginTimeout("ParaCheck")
	timer := time.AfterFunc(timeout, func() {
		vm.Interrupt("ParaCheck timeout")
	})
	defer timer.Stop()

	if opts.TransientVars == nil {
		opts.TransientVars = make(map[int]interface{})
	}
	mutations := &scriptMutations{
		varChanges:    make(map[int]int),
		switchChanges: make(map[int]bool),
		varAnyChanges: opts.TransientVars,
	}

	injectScriptMath(vm)
	injectScriptGameStateMutable(vm, opts.GameState, true, mutations)
	if s != nil {
		injectScriptGameActors(vm, s)
	}
	if e.res != nil {
		injectScriptDataArrays(vm, e.res)
	}

	// Inject player-specific values not available in standard VM
	vm.Set("__playerLevel", s.Level)
	gold := int64(0)
	if e.store != nil {
		g, err := e.store.GetGold(context.Background(), s.CharID)
		if err == nil {
			gold = g
		}
	}
	vm.Set("__gold", gold)
	vm.Set("__classId", s.ClassID)

	var runErr error
	panicked := false
	func() {
		defer func() {
			if r := recover(); r != nil {
				panicked = true
			}
		}()
		_, runErr = vm.RunString(scriptJS)
	}()
	if panicked || runErr != nil {
		e.logger.Warn("ParaCheck execution failed", zap.Error(runErr))
		return
	}

	// Apply mutations to game state, then batch send
	for id, val := range mutations.varChanges {
		opts.GameState.SetVariable(id, val)
	}
	for id, val := range mutations.switchChanges {
		opts.GameState.SetSwitch(id, val)
	}
	e.sendStateBatch(s, mutations.varChanges, mutations.switchChanges)

	e.logger.Info("ParaCheck executed",
		zap.Int64("char_id", s.CharID),
		zap.Int("v722", opts.GameState.GetVariable(722)),
		zap.Int("v702", opts.GameState.GetVariable(702)),
		zap.Int("v741", opts.GameState.GetVariable(741)),
		zap.Int("v742", opts.GameState.GetVariable(742)))
}
