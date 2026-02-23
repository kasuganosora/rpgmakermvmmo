package audit

import (
	"context"
	"encoding/json"
	"sync"
	"time"

	"github.com/kasuganosora/rpgmakermvmmo/server/model"
	"go.uber.org/zap"
	"gorm.io/datatypes"
	"gorm.io/gorm"
)

// AuditEntry holds one audit event to be logged.
type AuditEntry struct {
	TraceID   string
	CharID    *int64
	AccountID *int64
	CharName  string
	Action    string
	Request   interface{}
	Response  interface{}
	Error     string
	IP        string
	MapID     int
	DurationMs int
}

// Service logs audit entries asynchronously in batches.
type Service struct {
	db     *gorm.DB
	ch     chan *model.AuditLog
	stopCh chan struct{}
	wg     sync.WaitGroup
	logger *zap.Logger
}

// New creates a new audit Service and starts its background worker.
func New(db *gorm.DB, logger *zap.Logger) *Service {
	svc := &Service{
		db:     db,
		ch:     make(chan *model.AuditLog, 1024),
		stopCh: make(chan struct{}),
		logger: logger,
	}
	svc.wg.Add(1)
	go svc.worker()
	return svc
}

// Log enqueues an audit entry for async DB write.
func (svc *Service) Log(entry AuditEntry) {
	reqJSON, _ := json.Marshal(entry.Request)
	respJSON, _ := json.Marshal(entry.Response)
	record := &model.AuditLog{
		TraceID:    entry.TraceID,
		CharID:     entry.CharID,
		AccountID:  entry.AccountID,
		CharName:   entry.CharName,
		Action:     entry.Action,
		Request:    datatypes.JSON(reqJSON),
		Response:   datatypes.JSON(respJSON),
		Error:      entry.Error,
		IP:         entry.IP,
		MapID:      entry.MapID,
		DurationMs: entry.DurationMs,
	}
	select {
	case svc.ch <- record:
	default:
		svc.logger.Warn("audit channel full, dropping entry",
			zap.String("action", entry.Action))
	}
}

// Stop flushes remaining entries and shuts down the worker.
// It blocks until the worker goroutine has finished.
func (svc *Service) Stop(_ context.Context) {
	select {
	case <-svc.stopCh:
	default:
		close(svc.stopCh)
	}
	svc.wg.Wait()
}

func (svc *Service) worker() {
	defer svc.wg.Done()
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()

	batch := make([]*model.AuditLog, 0, 100)

	flush := func() {
		if len(batch) == 0 {
			return
		}
		if err := svc.db.Create(&batch).Error; err != nil {
			svc.logger.Error("audit batch write failed", zap.Error(err))
		}
		batch = batch[:0]
	}

	for {
		select {
		case entry := <-svc.ch:
			batch = append(batch, entry)
			if len(batch) >= 100 {
				flush()
			}
		case <-ticker.C:
			flush()
		case <-svc.stopCh:
			// Drain remaining entries.
			for {
				select {
				case entry := <-svc.ch:
					batch = append(batch, entry)
				default:
					flush()
					return
				}
			}
		}
	}
}
