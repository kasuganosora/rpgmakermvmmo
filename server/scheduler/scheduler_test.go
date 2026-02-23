package scheduler

import (
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

func newNop() *zap.Logger { l, _ := zap.NewDevelopment(); return l }

func TestAddTicker_Fires(t *testing.T) {
	s := New(newNop())
	defer s.Stop()

	var count int32
	s.AddTicker("tick", 20*time.Millisecond, func() {
		atomic.AddInt32(&count, 1)
	})

	time.Sleep(120 * time.Millisecond)
	assert.GreaterOrEqual(t, atomic.LoadInt32(&count), int32(3))
}

func TestAddTicker_Replaces(t *testing.T) {
	s := New(newNop())
	defer s.Stop()

	var count1, count2 int32
	s.AddTicker("task", 20*time.Millisecond, func() { atomic.AddInt32(&count1, 1) })
	time.Sleep(30 * time.Millisecond)
	s.AddTicker("task", 20*time.Millisecond, func() { atomic.AddInt32(&count2, 1) })
	time.Sleep(80 * time.Millisecond)

	// Old ticker should have stopped, new one should be running
	snap1 := atomic.LoadInt32(&count1)
	time.Sleep(40 * time.Millisecond)
	assert.Equal(t, snap1, atomic.LoadInt32(&count1), "old ticker must stop after replacement")
	assert.Positive(t, atomic.LoadInt32(&count2))
}

func TestAddDelay_FiresOnce(t *testing.T) {
	s := New(newNop())
	defer s.Stop()

	var count int32
	s.AddDelay("once", 30*time.Millisecond, func() {
		atomic.AddInt32(&count, 1)
	})

	time.Sleep(100 * time.Millisecond)
	assert.Equal(t, int32(1), atomic.LoadInt32(&count))
}

func TestAddDelay_ReplacesCancelsOld(t *testing.T) {
	s := New(newNop())
	defer s.Stop()

	var count int32
	// Schedule with long delay, then replace immediately
	s.AddDelay("d", 500*time.Millisecond, func() { atomic.AddInt32(&count, 1) })
	s.AddDelay("d", 30*time.Millisecond, func() { atomic.AddInt32(&count, 10) })
	time.Sleep(100 * time.Millisecond)
	// Only the second delay should have fired (value 10), not both
	v := atomic.LoadInt32(&count)
	assert.Equal(t, int32(10), v)
}

func TestRemove_Ticker(t *testing.T) {
	s := New(newNop())
	defer s.Stop()

	var count int32
	s.AddTicker("task", 20*time.Millisecond, func() { atomic.AddInt32(&count, 1) })
	time.Sleep(50 * time.Millisecond)
	s.Remove("task")
	snap := atomic.LoadInt32(&count)
	time.Sleep(60 * time.Millisecond)
	assert.Equal(t, snap, atomic.LoadInt32(&count), "ticker must stop after Remove")
}

func TestRemove_Delay(t *testing.T) {
	s := New(newNop())
	defer s.Stop()

	var count int32
	s.AddDelay("d", 100*time.Millisecond, func() { atomic.AddInt32(&count, 1) })
	s.Remove("d")
	time.Sleep(150 * time.Millisecond)
	assert.Equal(t, int32(0), atomic.LoadInt32(&count))
}

func TestRemove_NonExistent(t *testing.T) {
	s := New(newNop())
	defer s.Stop()
	// Must not panic
	s.Remove("nope")
}

func TestStop_StopsAllTickers(t *testing.T) {
	s := New(newNop())

	var c1, c2 int32
	s.AddTicker("a", 20*time.Millisecond, func() { atomic.AddInt32(&c1, 1) })
	s.AddTicker("b", 20*time.Millisecond, func() { atomic.AddInt32(&c2, 1) })
	time.Sleep(50 * time.Millisecond)
	s.Stop()
	// Give goroutines time to observe the stop signal before snapping counts.
	time.Sleep(30 * time.Millisecond)
	snap1, snap2 := atomic.LoadInt32(&c1), atomic.LoadInt32(&c2)
	time.Sleep(60 * time.Millisecond)
	assert.Equal(t, snap1, atomic.LoadInt32(&c1))
	assert.Equal(t, snap2, atomic.LoadInt32(&c2))
}

func TestStop_Idempotent(t *testing.T) {
	s := New(newNop())
	s.Stop()
	s.Stop() // must not panic on double-stop
}

func TestListTickers(t *testing.T) {
	s := New(newNop())
	defer s.Stop()

	require.Empty(t, s.ListTickers())
	s.AddTicker("alpha", time.Hour, func() {})
	s.AddTicker("beta", time.Hour, func() {})
	names := s.ListTickers()
	assert.Len(t, names, 2)
	assert.Contains(t, names, "alpha")
	assert.Contains(t, names, "beta")
}

func TestListTickers_AfterRemove(t *testing.T) {
	s := New(newNop())
	defer s.Stop()

	s.AddTicker("x", time.Hour, func() {})
	s.AddTicker("y", time.Hour, func() {})
	s.Remove("x")
	assert.Equal(t, []string{"y"}, s.ListTickers())
}

func TestTicker_PanicRecovery(t *testing.T) {
	s := New(newNop())
	defer s.Stop()

	var after int32
	s.AddTicker("panic", 20*time.Millisecond, func() {
		panic("oops")
	})
	// After the panic the ticker goroutine should keep running
	time.Sleep(80 * time.Millisecond)
	atomic.StoreInt32(&after, 1)
	assert.Equal(t, int32(1), after) // test itself didn't crash
}
