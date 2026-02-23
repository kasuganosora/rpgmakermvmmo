package scheduler

import (
	"sync"
	"time"

	"go.uber.org/zap"
)

// TaskFn is the function signature for scheduled tasks.
type TaskFn func()

// Scheduler manages periodic and delayed tasks.
type Scheduler struct {
	mu      sync.Mutex
	tickers map[string]*tickerEntry
	timers  map[string]*time.Timer
	crons   []func() // placeholder for cron-like tasks
	logger  *zap.Logger
	stopCh  chan struct{}
}

type tickerEntry struct {
	ticker *time.Ticker
	stopCh chan struct{}
}

// New creates a new Scheduler.
func New(logger *zap.Logger) *Scheduler {
	return &Scheduler{
		tickers: make(map[string]*tickerEntry),
		timers:  make(map[string]*time.Timer),
		stopCh:  make(chan struct{}),
		logger:  logger,
	}
}

// AddTicker registers a task to run on a fixed interval.
// If a task with the same name exists, it is replaced.
func (s *Scheduler) AddTicker(name string, interval time.Duration, fn TaskFn) {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Remove existing.
	if old, ok := s.tickers[name]; ok {
		close(old.stopCh)
		delete(s.tickers, name)
	}

	entry := &tickerEntry{
		ticker: time.NewTicker(interval),
		stopCh: make(chan struct{}),
	}
	s.tickers[name] = entry

	go func() {
		for {
			select {
			case <-entry.ticker.C:
				func() {
					defer func() {
						if r := recover(); r != nil {
							s.logger.Error("scheduler task panicked",
								zap.String("task", name),
								zap.Any("recover", r))
						}
					}()
					fn()
				}()
			case <-entry.stopCh:
				entry.ticker.Stop()
				return
			case <-s.stopCh:
				entry.ticker.Stop()
				return
			}
		}
	}()
	s.logger.Info("scheduler task registered", zap.String("name", name), zap.Duration("interval", interval))
}

// AddDelay runs fn once after the given delay.
func (s *Scheduler) AddDelay(name string, delay time.Duration, fn TaskFn) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if old, ok := s.timers[name]; ok {
		old.Stop()
	}
	s.timers[name] = time.AfterFunc(delay, func() {
		defer func() {
			if r := recover(); r != nil {
				s.logger.Error("delay task panicked",
					zap.String("task", name), zap.Any("recover", r))
			}
			s.mu.Lock()
			delete(s.timers, name)
			s.mu.Unlock()
		}()
		fn()
	})
}

// Remove stops and removes a ticker or delay task by name.
func (s *Scheduler) Remove(name string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if entry, ok := s.tickers[name]; ok {
		close(entry.stopCh)
		delete(s.tickers, name)
	}
	if t, ok := s.timers[name]; ok {
		t.Stop()
		delete(s.timers, name)
	}
}

// Stop stops all tasks.
func (s *Scheduler) Stop() {
	select {
	case <-s.stopCh:
	default:
		close(s.stopCh)
	}
}

// ListTickers returns the names of all registered ticker tasks.
func (s *Scheduler) ListTickers() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	names := make([]string, 0, len(s.tickers))
	for name := range s.tickers {
		names = append(names, name)
	}
	return names
}
