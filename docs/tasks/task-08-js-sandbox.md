# Task 08 - 服务端 JavaScript 执行沙箱（goja VM 池）

> **优先级**：P3 — M5 后期，任务系统的 Script 指令支持
> **里程碑**：M5（支持复杂自定义 JS 逻辑）
> **依赖**：Task 06（NPC 事件执行器 Script 指令占位）、Task 07（Hook 系统）

---

## 目标

实现基于 goja 的服务端 JS 执行沙箱，支持 RPG Maker MV 事件 Script 指令（code=355/655）的服务端权威执行。包括：VM 池管理（复用 + 超时控制）、RMMV 全局对象 Mock 注入（$gameVariables 等）、server_scripts/ 目录自定义 JS 模块加载、JS ↔ Go Hook 桥接。

---

## Todolist

- [ ] **08-1** 添加 goja 依赖 + 三层执行策略路由（`game/script/`）
  - [ ] 08-1a `go get github.com/dop251/goja`
  - [ ] 08-1b `game/script/dispatcher.go`：根据脚本内容决定走白名单/goja/server_scripts
  - [ ] 08-1c 白名单判断逻辑（检测字符串是否包含控制流关键字）
- [ ] **08-2** 实现 RMMV 上下文 Mock 注入（`game/script/context.go`）
  - [ ] 08-2a `ScriptContext` struct（含玩家 ID、变量/开关访问器、背包访问器）
  - [ ] 08-2b `InjectRMMVContext(vm *goja.Runtime, ctx *ScriptContext)`（完整代码见 §7.8.4）
  - [ ] 08-2c 危险全局屏蔽（require/process/fetch/WebSocket/setTimeout 等）
  - [ ] 08-2d $gameVariables / $gameSwitches 代理（读写 Task 06 的变量系统）
  - [ ] 08-2e $gameParty 代理（gainItem/loseItem/gainGold 调用背包系统）
  - [ ] 08-2f $dataItems / $dataSkills / $dataEnemies 只读注入（ResourceLoader 数据）
- [ ] **08-3** 实现 VM 池（`game/script/pool.go`）
  - [ ] 08-3a `VMPool` struct（带缓冲 channel 的 goja.Runtime 池）
  - [ ] 08-3b `NewVMPool(size int, resource *ResourceLoader) *VMPool`（预热 + 注入只读数据）
  - [ ] 08-3c `RunScript(goCtx, script, sctx) (goja.Value, error)`（取池 → 注入 → 执行 → 归还）
  - [ ] 08-3d 超时控制（goCtx 取消时调用 `vm.Interrupt("script timeout")`）
  - [ ] 08-3e 在 Task 06 的 EventExecutor 中替换 Script 占位调用
- [ ] **08-4** 实现 server_scripts/ 自定义脚本加载（`game/script/loader.go`）
  - [ ] 08-4a 启动时扫描 `config.Script.ScriptsDir/*.js` 并逐一执行
  - [ ] 08-4b 向 JS 全局注入 `registerHook` 桥接函数
  - [ ] 08-4c `POST /admin/reload-scripts` 热重载（Admin API 调用）
  - [ ] 08-4d server_scripts 的 VM 与 VMPool 分离（共享不变数据，隔离 Hook 注册）
- [ ] **08-5** 实现 JS ↔ Go Hook 桥接（`game/script/bridge.go`）
  - [ ] 08-5a `registerHook(event, fn)` JS 函数注入
  - [ ] 08-5b JS Hook 被触发时：从 Hook 系统调用 JS 回调
  - [ ] 08-5c `ctx.interrupt(reason)` / `ctx.teleportPlayer(mapId, x, y)` 等桥接方法
- [ ] **08-6** 编写单元测试
  - [ ] 08-6a `vm_pool_test.go`：并发 N 个 goroutine 同时调用 RunScript，池大小为 8
  - [ ] 08-6b `context_test.go`：$gameVariables.setValue → getVariable 正确返回
  - [ ] 08-6c `sandbox_test.go`：尝试访问 require/process 返回 undefined
  - [ ] 08-6d `timeout_test.go`：死循环脚本 3s 后被中断并返回 timeout error
  - [ ] 08-6e `server_scripts_test.go`：加载示例 JS 文件，registerHook 注册成功

---

## 实现细节与思路

### 08-1 三层执行策略路由

```go
// game/script/dispatcher.go

var controlFlowKeywords = []string{
    "if", "for", "while", "function", "var", "let", "const", ";", "{", "}", "=>",
}

func isSimpleFormula(script string) bool {
    for _, kw := range controlFlowKeywords {
        if strings.Contains(script, kw) {
            return false
        }
    }
    return true
}

// Dispatch 决定执行路径
func Dispatch(goCtx context.Context, script string, sctx *ScriptContext) (interface{}, error) {
    if isSimpleFormula(script) {
        // Layer 1：白名单表达式求值器（Task 03 的 formula.go）
        return formulaEval(script, sctx.Attacker, sctx.Defender)
    }
    // Layer 2：goja VM 池
    return GlobalVMPool.RunScript(goCtx, script, sctx)
}
```

### 08-2 ScriptContext

```go
// game/script/context.go

type ScriptContext struct {
    CharID   int64
    MapID    int
    Resource *resource.ResourceLoader

    // 变量/开关访问（代理到 Task 06 的 player.Variables）
    GetVariable func(id int) int
    SetVariable func(id int, val int)
    GetSwitch   func(id int) bool
    SetSwitch   func(id int, val bool)

    // 背包访问（代理到 Task 04 的 item.InventoryService）
    GetLeader  func() map[string]interface{}   // 返回角色属性 JSON 对象
    GetMembers func() []map[string]interface{}
    GainItem   func(itemID, amount int)
    LoseItem   func(itemID, amount int)
    GetGold    func() int64
    GainGold   func(amount int64)

    // 附加动作（供 server_scripts 使用）
    TeleportPlayer func(mapID, x, y int)
}
```

**InjectRMMVContext 完整实现**（参见 §7.8.4）：
- `$gameVariables`：代理 `GetVariable` / `SetVariable`
- `$gameSwitches`：代理 `GetSwitch` / `SetSwitch`
- `$gameParty`：代理 `GetLeader` / `GetMembers` / `GainItem` / `LoseItem` / `GetGold` / `GainGold`
- `$dataItems`：`ctx.Resource.Items`（只读 slice，转为 goja 可用的 JS 对象）
- `$dataSkills`：`ctx.Resource.Skills`
- `$dataEnemies`：`ctx.Resource.Enemies`

**危险全局屏蔽**（防沙箱逃逸）：
```go
dangerGlobals := []string{
    "require", "process", "fetch", "XMLHttpRequest",
    "WebSocket", "setTimeout", "setInterval", "__proto__",
    "global", "globalThis", "eval",
}
for _, g := range dangerGlobals {
    vm.Set(g, goja.Undefined())
}
```

### 08-3 VM 池

**完整实现**（参见 §7.8.5）：

```go
// game/script/pool.go

type VMPool struct {
    pool chan *goja.Runtime
}

func NewVMPool(size int, res *resource.ResourceLoader) *VMPool {
    p := &VMPool{pool: make(chan *goja.Runtime, size)}
    for i := 0; i < size; i++ {
        vm := goja.New()
        // 注入不变的只读 RMMV 数据（可复用，避免每次重新转换）
        vm.Set("$dataItems",   toGojaValue(vm, res.Items))
        vm.Set("$dataSkills",  toGojaValue(vm, res.Skills))
        vm.Set("$dataEnemies", toGojaValue(vm, res.Enemies))
        p.pool <- vm
    }
    return p
}

func (p *VMPool) RunScript(
    goCtx context.Context,
    script string,
    sctx *ScriptContext,
) (goja.Value, error) {
    vm := <-p.pool
    defer func() {
        // 重置玩家相关状态（屏蔽掉 SetVariable 等的副作用，使 VM 可复用）
        // 注意：$dataItems 等只读数据无需重置
        p.pool <- vm
    }()

    // 注入玩家相关可变上下文（每次调用覆盖）
    InjectRMMVContext(vm, sctx)

    // 超时控制
    stop := make(chan struct{})
    go func() {
        select {
        case <-goCtx.Done():
            vm.Interrupt("script timeout")
        case <-stop:
        }
    }()
    defer close(stop)

    val, err := vm.RunString(script)
    if err != nil {
        // goja.Interrupt 错误：超时
        if isInterruptError(err) {
            return nil, fmt.Errorf("script timeout: %w", err)
        }
        return nil, err  // JS 语法错误或运行时错误
    }
    return val, nil
}
```

**VM 复用注意事项**：
- 只读数据（`$dataItems` 等）注入一次复用 ✅
- 玩家变量（`$gameVariables` 等）每次调用前重新注入 ✅
- goja.Runtime 的全局作用域可能有脚本间变量泄漏（用 `var` 定义的变量残留）
- 解决方案：在 VM 上为每次脚本执行创建新的 Function 作用域包裹执行：
  ```go
  wrappedScript := fmt.Sprintf("(function(){\n%s\n})()", script)
  vm.RunString(wrappedScript)
  ```

**池大小配置**：`config.Script.VMPoolSize`，默认 = `runtime.NumCPU()`。

### 08-4 server_scripts/ 加载

```go
// game/script/loader.go

type ScriptLoader struct {
    vm       *goja.Runtime    // 独立 VM（与 VMPool 分离）
    hookCenter *hook.HookCenter
    scriptsDir string
}

func (l *ScriptLoader) Load() error {
    entries, _ := os.ReadDir(l.scriptsDir)
    for _, e := range entries {
        if !strings.HasSuffix(e.Name(), ".js") { continue }
        data, _ := os.ReadFile(filepath.Join(l.scriptsDir, e.Name()))
        if _, err := l.vm.RunString(string(data)); err != nil {
            log.Error("failed to load script", zap.String("file", e.Name()), zap.Error(err))
        }
    }
    return nil
}

func (l *ScriptLoader) InjectBridges() {
    // 注入 registerHook 桥接函数
    l.vm.Set("registerHook", func(call goja.FunctionCall) goja.Value {
        event := call.Argument(0).String()
        fn    := call.Argument(1)  // JS function
        l.hookCenter.Register(event, 50, func(ctx context.Context, ev string, data interface{}) (interface{}, error) {
            // 将 data 转为 goja 可用的对象，调用 JS fn
            jsFn, _ := goja.AssertFunction(fn)
            jsCtx := buildJSHookContext(l.vm, data)
            jsFn(goja.Undefined(), l.vm.ToValue(jsCtx))
            return data, checkInterrupt(jsCtx)
        })
        return goja.Undefined()
    })
}
```

### 08-5 JS Hook 上下文对象

传给 JS Hook 回调的 `ctx` 对象：
```javascript
// JS 侧看到的 ctx：
ctx.item        // {id, name, price, ...}（只读）
ctx.player      // {hp, maxHp, level, ...}（只读）
ctx.interrupt(reason)   // 调用后 Go 侧停止后续 Hook
ctx.teleportPlayer(mapId, x, y)  // 触发传送
ctx.gainItem(itemId, qty)
ctx.gainGold(amount)
```

### 08-6 关键测试用例

```go
// sandbox_test.go

func TestDangerousGlobalBlocked(t *testing.T) {
    pool := NewVMPool(1, mockResource())
    ctx := context.Background()
    sctx := &ScriptContext{CharID: 1}

    // require 被屏蔽，脚本应该返回 undefined 而不是报错
    val, err := pool.RunScript(ctx, "require", sctx)
    assert.NoError(t, err)
    assert.True(t, val == nil || val.Export() == nil)
}

func TestScriptTimeout(t *testing.T) {
    pool := NewVMPool(1, mockResource())
    ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
    defer cancel()

    _, err := pool.RunScript(ctx, "while(true){}", &ScriptContext{})
    assert.Error(t, err)
    assert.Contains(t, err.Error(), "timeout")
}

func TestVariableProxy(t *testing.T) {
    vars := map[int]int{}
    sctx := &ScriptContext{
        GetVariable: func(id int) int { return vars[id] },
        SetVariable: func(id int, val int) { vars[id] = val },
    }
    pool := NewVMPool(1, mockResource())
    pool.RunScript(context.Background(), "$gameVariables.setValue(10, 42);", sctx)
    assert.Equal(t, 42, vars[10])
}
```

---

## 配置项

```yaml
script:
  vm_pool_size:  8       # goja VM 池大小（建议 = CPU 核数）
  timeout_ms:    3000    # 单条脚本最大执行时间（毫秒）
  scripts_dir:   "./server_scripts"   # 自定义 JS 脚本目录
  allow_custom:  true    # 是否允许加载 server_scripts/（生产环境可关闭）
```

---

## 验收标准

1. RMMV 事件 Script 指令（code=355）通过 VM 池执行，`$gameVariables.setValue(10, 1)` 正确修改服务端变量
2. 死循环脚本 3s 后被中断，返回 timeout 错误，**不影响其他请求**
3. `require`/`process` 等危险全局访问返回 undefined（不报错，防止事件流中断）
4. VM 池并发 8 goroutine 同时执行，不死锁（pool channel 保证互斥）
5. `server_scripts/custom_items.js` 注册的 Hook 在物品使用时被调用
6. `/admin/reload-scripts` 重新加载 JS 文件后新逻辑生效
7. 所有单元测试通过
