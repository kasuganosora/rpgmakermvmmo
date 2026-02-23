// Package script provides a server-side JavaScript execution sandbox
// backed by a pool of goja VMs for safe, concurrent RMMV script execution.
package script

import (
	"context"
	"errors"
	"sync"
	"time"

	"github.com/dop251/goja"
	"go.uber.org/zap"
)

// ErrTimeout is returned when a script exceeds the execution time limit.
var ErrTimeout = errors.New("script: execution timed out")

// ErrPanic is returned when a script throws an uncaught exception.
var ErrPanic = errors.New("script: uncaught exception")

// ScriptContext provides game-state accessors available inside JS scripts.
type ScriptContext struct {
	// GetVariable returns a game variable value by ID.
	GetVariable func(id int) interface{}
	// SetVariable sets a game variable value.
	SetVariable func(id int, value interface{})
	// GetSwitch returns a game switch state.
	GetSwitch func(id int) bool
	// SetSwitch sets a game switch state.
	SetSwitch func(id int, value bool)
}

// VMPool is a thread-safe pool of pre-initialised goja runtimes.
type VMPool struct {
	pool    chan *goja.Runtime
	timeout time.Duration
	logger  *zap.Logger
	mu      sync.Mutex
	size    int
}

// NewVMPool creates a VMPool with the given concurrency size and per-script timeout.
func NewVMPool(size int, timeout time.Duration, logger *zap.Logger) *VMPool {
	if size <= 0 {
		size = 4
	}
	if timeout <= 0 {
		timeout = 500 * time.Millisecond
	}
	p := &VMPool{
		pool:    make(chan *goja.Runtime, size),
		timeout: timeout,
		logger:  logger,
		size:    size,
	}
	for i := 0; i < size; i++ {
		p.pool <- newSafeVM()
	}
	return p
}

// Run executes src inside a pooled VM with the given ScriptContext.
// Returns the value of the last expression evaluated, or an error.
func (p *VMPool) Run(ctx context.Context, src string, sc *ScriptContext) (interface{}, error) {
	select {
	case vm := <-p.pool:
		// returnToPool is set to false by runVM when the VM is tainted by a
		// timeout and must be discarded rather than returned to the pool.
		returnToPool := true
		defer func() {
			if returnToPool {
				p.pool <- vm
			}
		}()
		return p.runVM(ctx, vm, src, sc, &returnToPool)
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

func (p *VMPool) runVM(ctx context.Context, vm *goja.Runtime, src string, sc *ScriptContext, returnToPool *bool) (interface{}, error) {
	// Inject context accessors.
	if sc != nil {
		injectContext(vm, sc)
	}

	// Set up timeout interrupt.
	timer := time.AfterFunc(p.timeout, func() {
		vm.Interrupt(ErrTimeout)
	})
	defer func() {
		timer.Stop()
		// Clear interrupt so the VM is clean for the next caller (only reached
		// when returnToPool is still true).
		if *returnToPool {
			vm.ClearInterrupt()
		}
	}()

	// Recover panics from goja.
	var result goja.Value
	var runErr error
	func() {
		defer func() {
			if r := recover(); r != nil {
				runErr = ErrPanic
			}
		}()
		result, runErr = vm.RunString(src)
	}()

	if runErr != nil {
		if runErr == ErrTimeout {
			// VM is tainted after an interrupt; discard it and add a fresh one.
			*returnToPool = false
			p.pool <- newSafeVM()
			return nil, ErrTimeout
		}
		if ex, ok := runErr.(*goja.Exception); ok {
			return nil, errors.New(ex.Error())
		}
		return nil, runErr
	}

	if result == nil || goja.IsUndefined(result) || goja.IsNull(result) {
		return nil, nil
	}
	return result.Export(), nil
}

// newSafeVM creates a goja Runtime with dangerous globals removed.
func newSafeVM() *goja.Runtime {
	vm := goja.New()
	// Block dangerous globals.
	for _, name := range []string{"require", "process", "fetch", "XMLHttpRequest", "eval", "Function"} {
		vm.Set(name, goja.Undefined())
	}
	// Provide safe Math subset.
	mathObj := vm.NewObject()
	_ = mathObj.Set("floor", func(v float64) float64 { return float64(int64(v)) })
	_ = mathObj.Set("ceil", func(v float64) float64 {
		n := int64(v)
		if float64(n) < v {
			n++
		}
		return float64(n)
	})
	_ = mathObj.Set("round", func(v float64) int64 { return int64(v + 0.5) })
	_ = mathObj.Set("abs", func(v float64) float64 {
		if v < 0 {
			return -v
		}
		return v
	})
	_ = mathObj.Set("max", func(a, b float64) float64 {
		if a > b {
			return a
		}
		return b
	})
	_ = mathObj.Set("min", func(a, b float64) float64 {
		if a < b {
			return a
		}
		return b
	})
	_ = mathObj.Set("random", func() float64 { return 0 }) // deterministic in server
	vm.Set("Math", mathObj)
	return vm
}

// injectContext binds ScriptContext accessors into the VM as $game* globals.
func injectContext(vm *goja.Runtime, sc *ScriptContext) {
	if sc.GetVariable != nil && sc.SetVariable != nil {
		vars := vm.NewObject()
		_ = vars.Set("value", func(id int) interface{} { return sc.GetVariable(id) })
		_ = vars.Set("setValue", func(id int, v interface{}) { sc.SetVariable(id, v) })
		vm.Set("$gameVariables", vars)
	}
	if sc.GetSwitch != nil && sc.SetSwitch != nil {
		sw := vm.NewObject()
		_ = sw.Set("value", func(id int) bool { return sc.GetSwitch(id) })
		_ = sw.Set("setValue", func(id int, v bool) { sc.SetSwitch(id, v) })
		vm.Set("$gameSwitches", sw)
	}
}

// Sandbox wraps a VMPool and provides a simple Run interface with context support.
type Sandbox struct {
	pool   *VMPool
	logger *zap.Logger
}

// NewSandbox creates a Sandbox backed by a VMPool.
func NewSandbox(size int, timeout time.Duration, logger *zap.Logger) *Sandbox {
	return &Sandbox{
		pool:   NewVMPool(size, timeout, logger),
		logger: logger,
	}
}

// Eval executes src with the given ScriptContext, returning the result.
func (sb *Sandbox) Eval(ctx context.Context, src string, sc *ScriptContext) (interface{}, error) {
	result, err := sb.pool.Run(ctx, src, sc)
	if err != nil {
		sb.logger.Warn("script execution error",
			zap.String("src_preview", truncate(src, 80)),
			zap.Error(err))
	}
	return result, err
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}
