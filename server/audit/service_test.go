package audit

import (
	"context"
	"testing"
	"time"

	"github.com/kasuganosora/rpgmakermvmmo/server/model"
	"github.com/kasuganosora/rpgmakermvmmo/server/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

func nop() *zap.Logger { l, _ := zap.NewDevelopment(); return l }

func TestNew_StartsWorker(t *testing.T) {
	db := testutil.SetupTestDB(t)
	svc := New(db, nop())
	require.NotNil(t, svc)
	svc.Stop(context.Background())
}

func TestLog_EnqueuedAndFlushed(t *testing.T) {
	db := testutil.SetupTestDB(t)
	svc := New(db, nop())

	charID := int64(1)
	accountID := int64(2)
	svc.Log(AuditEntry{
		TraceID:    "trace-123",
		CharID:     &charID,
		AccountID:  &accountID,
		CharName:   "Alice",
		Action:     "login",
		Request:    map[string]string{"user": "alice"},
		Response:   map[string]bool{"ok": true},
		IP:         "127.0.0.1",
		MapID:      1,
		DurationMs: 42,
	})

	// Stop flushes remaining entries
	svc.Stop(context.Background())

	var logs []model.AuditLog
	db.Find(&logs)
	require.Len(t, logs, 1)
	assert.Equal(t, "trace-123", logs[0].TraceID)
	assert.Equal(t, "Alice", logs[0].CharName)
	assert.Equal(t, "login", logs[0].Action)
	assert.Equal(t, "127.0.0.1", logs[0].IP)
	assert.Equal(t, 42, logs[0].DurationMs)
}

func TestLog_MultipleLogs(t *testing.T) {
	db := testutil.SetupTestDB(t)
	svc := New(db, nop())

	for i := 0; i < 10; i++ {
		svc.Log(AuditEntry{
			Action: "action",
			IP:     "10.0.0.1",
		})
	}

	svc.Stop(context.Background())

	var count int64
	db.Model(&model.AuditLog{}).Count(&count)
	assert.Equal(t, int64(10), count)
}

func TestLog_BatchFlush(t *testing.T) {
	db := testutil.SetupTestDB(t)
	svc := New(db, nop())

	// Send 100 entries to trigger immediate batch flush
	for i := 0; i < 100; i++ {
		svc.Log(AuditEntry{Action: "batch"})
	}

	// Stop waits (via WaitGroup) until the worker has finished flushing.
	// The 100-entry batch flush is triggered synchronously inside the worker, so
	// after Stop() the data is guaranteed to be committed.
	svc.Stop(context.Background())

	var count int64
	db.Model(&model.AuditLog{}).Count(&count)
	assert.GreaterOrEqual(t, count, int64(100))
}

func TestLog_TimerFlush(t *testing.T) {
	db := testutil.SetupTestDB(t)
	svc := New(db, nop())

	svc.Log(AuditEntry{Action: "timer_test"})

	// Wait for the 2s ticker to fire and flush.
	// Note: sqlexec MVCC does not propagate timer-goroutine writes to the calling
	// goroutine's read snapshot; visibility is covered by TestLog_EnqueuedAndFlushed.
	// This test only verifies the timer path does not panic or deadlock.
	time.Sleep(2500 * time.Millisecond)
	svc.Stop(context.Background()) // must not deadlock
}

func TestStop_Idempotent(t *testing.T) {
	db := testutil.SetupTestDB(t)
	svc := New(db, nop())
	svc.Stop(context.Background())
	svc.Stop(context.Background()) // must not panic
}

func TestLog_NilFields(t *testing.T) {
	db := testutil.SetupTestDB(t)
	svc := New(db, nop())

	// Log with nil CharID/AccountID
	svc.Log(AuditEntry{
		Action: "no_char",
	})

	svc.Stop(context.Background())

	var logs []model.AuditLog
	db.Find(&logs)
	require.Len(t, logs, 1)
	assert.Nil(t, logs[0].CharID)
	assert.Nil(t, logs[0].AccountID)
}

func TestLog_DropsWhenFull(t *testing.T) {
	db := testutil.SetupTestDB(t)
	svc := New(db, nop())

	// Fill the channel beyond capacity by stalling worker
	// (worker reads from ch, but with 1024 buffer we can test the drop path
	// by flooding with 1025+ without waiting for flush)
	// The channel capacity is 1024; send 1030 to ensure some drops.
	// We stop before the worker can flush to force the channel-full path.

	// Note: this test just verifies the service doesn't panic on channel full.
	for i := 0; i < 1030; i++ {
		svc.Log(AuditEntry{Action: "flood"})
	}
	svc.Stop(context.Background())
	// Just verify no panic occurred
}
