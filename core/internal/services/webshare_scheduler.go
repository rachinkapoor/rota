package services

import (
	"context"
	"sync"
	"time"

	"github.com/alpkeskin/rota/core/pkg/logger"
)

// WebshareScheduler handles automatic synchronization scheduling
type WebshareScheduler struct {
	syncService *WebshareSyncService
	interval    time.Duration
	ticker      *time.Ticker
	stopChan    chan struct{}
	mu          sync.Mutex
	running     bool
	logger      *logger.Logger
}

// NewWebshareScheduler creates a new WebshareScheduler
func NewWebshareScheduler(syncService *WebshareSyncService, intervalSeconds int, log *logger.Logger) *WebshareScheduler {
	return &WebshareScheduler{
		syncService: syncService,
		interval:    time.Duration(intervalSeconds) * time.Second,
		stopChan:    make(chan struct{}),
		logger:      log,
	}
}

// Start starts the scheduler if interval > 0
func (s *WebshareScheduler) Start(ctx context.Context) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.running {
		return
	}

	// Don't start if interval is 0 or negative
	if s.interval <= 0 {
		s.logger.Info("webshare auto-sync disabled (interval is 0)")
		return
	}

	s.running = true
	s.ticker = time.NewTicker(s.interval)
	s.logger.Info("webshare auto-sync scheduler started", "interval", s.interval)

	go s.run(ctx)
}

// Stop stops the scheduler
func (s *WebshareScheduler) Stop() {
	s.mu.Lock()
	defer s.mu.Unlock()

	if !s.running {
		return
	}

	s.running = false
	if s.ticker != nil {
		s.ticker.Stop()
	}
	close(s.stopChan)
	s.logger.Info("webshare auto-sync scheduler stopped")
}

// run is the main scheduler loop
func (s *WebshareScheduler) run(ctx context.Context) {
	// Perform initial sync after interval
	select {
	case <-time.After(s.interval):
		s.triggerSync(ctx)
	case <-s.stopChan:
		return
	case <-ctx.Done():
		return
	}

	// Then sync on ticker interval
	for {
		select {
		case <-s.ticker.C:
			s.triggerSync(ctx)
		case <-s.stopChan:
			return
		case <-ctx.Done():
			return
		}
	}
}

// triggerSync triggers a sync if one is not already in progress
func (s *WebshareScheduler) triggerSync(ctx context.Context) {
	if s.syncService.IsSyncing() {
		s.logger.Debug("skipping sync, already in progress")
		return
	}

	s.logger.Info("triggering automatic webshare sync")
	if err := s.syncService.Sync(ctx); err != nil {
		s.logger.Error("automatic sync failed", "error", err)
	}
}

// IsRunning returns whether the scheduler is running
func (s *WebshareScheduler) IsRunning() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.running
}
