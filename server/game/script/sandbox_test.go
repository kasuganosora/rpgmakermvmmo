package script

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

func nop() *zap.Logger { l, _ := zap.NewDevelopment(); return l }

func newSandbox(t *testing.T) *Sandbox {
	t.Helper()
	return NewSandbox(2, 200*time.Millisecond, nop())
}

func TestSandbox_BasicArithmetic(t *testing.T) {
	sb := newSandbox(t)
	out, err := sb.Eval(context.Background(), "1 + 2", nil)
	require.NoError(t, err)
	assert.Equal(t, int64(3), out)
}

func TestSandbox_ReturnString(t *testing.T) {
	sb := newSandbox(t)
	out, err := sb.Eval(context.Background(), `"hello"`, nil)
	require.NoError(t, err)
	assert.Equal(t, "hello", out)
}

func TestSandbox_NullResult(t *testing.T) {
	sb := newSandbox(t)
	out, err := sb.Eval(context.Background(), "null", nil)
	require.NoError(t, err)
	assert.Nil(t, out)
}

func TestSandbox_UndefinedResult(t *testing.T) {
	sb := newSandbox(t)
	out, err := sb.Eval(context.Background(), "undefined", nil)
	require.NoError(t, err)
	assert.Nil(t, out)
}

func TestSandbox_SyntaxError(t *testing.T) {
	sb := newSandbox(t)
	_, err := sb.Eval(context.Background(), "{{{{ broken", nil)
	assert.Error(t, err)
}

func TestSandbox_RuntimeException(t *testing.T) {
	sb := newSandbox(t)
	_, err := sb.Eval(context.Background(), `throw new Error("boom")`, nil)
	assert.Error(t, err)
}

func TestSandbox_Timeout(t *testing.T) {
	sb := NewSandbox(1, 50*time.Millisecond, nop())
	_, err := sb.Eval(context.Background(), `while(true){}`, nil)
	assert.True(t, errors.Is(err, ErrTimeout), "expected ErrTimeout, got %v", err)
}

func TestSandbox_ContextCancel(t *testing.T) {
	// Create a pool of size 1. Run a blocking script first to occupy the VM.
	// Then try to run with a cancelled context; no VM is available so ctx.Done wins.
	sb := NewSandbox(1, 5*time.Second, nop())

	// Occupy the sole VM with a long-running script in background.
	vmBusy := make(chan struct{})
	go func() {
		// This script will block until the VM's timeout (5s). We just need it to hold the VM.
		ctx2, cancel2 := context.WithTimeout(context.Background(), 200*time.Millisecond)
		defer cancel2()
		close(vmBusy)
		sb.Eval(ctx2, `var i=0; while(i<1e8){i++;}`, nil)
	}()
	<-vmBusy
	time.Sleep(5 * time.Millisecond) // let the goroutine acquire the VM

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := sb.Eval(ctx, "1+1", nil)
	assert.Error(t, err)
}

func TestSandbox_MathFloor(t *testing.T) {
	sb := newSandbox(t)
	out, err := sb.Eval(context.Background(), "Math.floor(3.9)", nil)
	require.NoError(t, err)
	// goja exports whole-number float64 as int64
	assert.EqualValues(t, 3, out)
}

func TestSandbox_MathCeil(t *testing.T) {
	sb := newSandbox(t)
	out, err := sb.Eval(context.Background(), "Math.ceil(3.1)", nil)
	require.NoError(t, err)
	assert.EqualValues(t, 4, out)
}

func TestSandbox_MathAbs(t *testing.T) {
	sb := newSandbox(t)
	out, err := sb.Eval(context.Background(), "Math.abs(-5)", nil)
	require.NoError(t, err)
	assert.EqualValues(t, 5, out)
}

func TestSandbox_MathMax(t *testing.T) {
	sb := newSandbox(t)
	out, err := sb.Eval(context.Background(), "Math.max(3, 7)", nil)
	require.NoError(t, err)
	assert.EqualValues(t, 7, out)
}

func TestSandbox_MathMin(t *testing.T) {
	sb := newSandbox(t)
	out, err := sb.Eval(context.Background(), "Math.min(3, 7)", nil)
	require.NoError(t, err)
	assert.EqualValues(t, 3, out)
}

func TestSandbox_MathRound(t *testing.T) {
	sb := newSandbox(t)
	out, err := sb.Eval(context.Background(), "Math.round(2.6)", nil)
	require.NoError(t, err)
	assert.Equal(t, int64(3), out)
}

func TestSandbox_BlockedRequire(t *testing.T) {
	sb := newSandbox(t)
	_, err := sb.Eval(context.Background(), "require('fs')", nil)
	assert.Error(t, err)
}

func TestSandbox_BlockedProcess(t *testing.T) {
	sb := newSandbox(t)
	_, err := sb.Eval(context.Background(), "process.exit(0)", nil)
	assert.Error(t, err)
}

func TestSandbox_BlockedEval(t *testing.T) {
	sb := newSandbox(t)
	_, err := sb.Eval(context.Background(), "eval('1+1')", nil)
	assert.Error(t, err)
}

func TestSandbox_GameVariables_GetSet(t *testing.T) {
	vars := map[int]interface{}{1: 100}
	sc := &ScriptContext{
		GetVariable: func(id int) interface{} { return vars[id] },
		SetVariable: func(id int, v interface{}) { vars[id] = v },
		GetSwitch:   func(id int) bool { return false },
		SetSwitch:   func(id int, v bool) {},
	}
	sb := newSandbox(t)
	_, err := sb.Eval(context.Background(), `$gameVariables.setValue(1, 200)`, sc)
	require.NoError(t, err)
	assert.Equal(t, int64(200), vars[1])
}

func TestSandbox_GameVariables_Get(t *testing.T) {
	sc := &ScriptContext{
		GetVariable: func(id int) interface{} { return 42 },
		SetVariable: func(id int, v interface{}) {},
		GetSwitch:   func(id int) bool { return false },
		SetSwitch:   func(id int, v bool) {},
	}
	sb := newSandbox(t)
	out, err := sb.Eval(context.Background(), `$gameVariables.value(1)`, sc)
	require.NoError(t, err)
	assert.Equal(t, int64(42), out)
}

func TestSandbox_GameSwitches(t *testing.T) {
	switches := map[int]bool{}
	sc := &ScriptContext{
		GetVariable: func(id int) interface{} { return 0 },
		SetVariable: func(id int, v interface{}) {},
		GetSwitch:   func(id int) bool { return switches[id] },
		SetSwitch:   func(id int, v bool) { switches[id] = v },
	}
	sb := newSandbox(t)
	_, err := sb.Eval(context.Background(), `$gameSwitches.setValue(2, true)`, sc)
	require.NoError(t, err)
	assert.True(t, switches[2])
}

func TestSandbox_VMPool_Concurrent(t *testing.T) {
	sb := NewSandbox(4, 200*time.Millisecond, nop())
	done := make(chan error, 10)
	for i := 0; i < 10; i++ {
		go func(n int) {
			_, err := sb.Eval(context.Background(), "1+1", nil)
			done <- err
		}(i)
	}
	for i := 0; i < 10; i++ {
		require.NoError(t, <-done)
	}
}

func TestNewVMPool_Defaults(t *testing.T) {
	p := NewVMPool(0, 0, nop()) // 0 → use defaults (4 VMs, 500ms)
	assert.NotNil(t, p)
}
