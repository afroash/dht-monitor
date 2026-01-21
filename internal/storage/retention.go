package storage

import (
	"sync"
	"time"

	"github.com/rs/zerolog"
)

// RetentionCleaner periodically removes old readings from the database
type RetentionCleaner struct {
	store         *SQLiteStore
	logger        zerolog.Logger
	retentionDays int
	cleanupPeriod time.Duration
	stopChan      chan struct{}
	stopOnce      sync.Once
	wg            sync.WaitGroup

	// Stats
	mu              sync.RWMutex
	totalDeleted    int64
	totalCleanups   int64
	lastCleanup     time.Time
	lastDeleteCount int64
}

// RetentionCleanerConfig holds configuration for the cleaner
type RetentionCleanerConfig struct {
	RetentionDays int           // Number of days to keep data (default: 30)
	CleanupPeriod time.Duration // How often to run cleanup (default: 1 hour)
}

// DefaultRetentionCleanerConfig returns sensible defaults
func DefaultRetentionCleanerConfig() RetentionCleanerConfig {
	return RetentionCleanerConfig{
		RetentionDays: 30,
		CleanupPeriod: 1 * time.Hour,
	}
}

// RetentionCleanerStats contains statistics about the cleaner
type RetentionCleanerStats struct {
	TotalDeleted    int64     `json:"total_deleted"`
	TotalCleanups   int64     `json:"total_cleanups"`
	LastCleanup     time.Time `json:"last_cleanup,omitempty"`
	LastDeleteCount int64     `json:"last_delete_count"`
	RetentionDays   int       `json:"retention_days"`
}

// NewRetentionCleaner creates and starts a new retention cleaner
func NewRetentionCleaner(store *SQLiteStore, config RetentionCleanerConfig, logger zerolog.Logger) *RetentionCleaner {
	cleanupPeriod := config.CleanupPeriod

	// Validate CleanupPeriod to prevent time.NewTicker panic
	if cleanupPeriod <= 0 {
		defaultPeriod := 1 * time.Hour
		logger.Warn().
			Dur("provided_period", cleanupPeriod).
			Dur("default_period", defaultPeriod).
			Msg("Invalid CleanupPeriod provided (zero or negative), using default")
		cleanupPeriod = defaultPeriod
	}

	c := &RetentionCleaner{
		store:         store,
		logger:        logger,
		retentionDays: config.RetentionDays,
		cleanupPeriod: cleanupPeriod,
		stopChan:      make(chan struct{}),
	}

	c.wg.Add(1)
	go c.cleanupLoop()

	logger.Info().
		Int("retention_days", config.RetentionDays).
		Dur("cleanup_period", cleanupPeriod).
		Msg("RetentionCleaner started")

	return c
}

// cleanupLoop runs the periodic cleanup
func (c *RetentionCleaner) cleanupLoop() {
	defer c.wg.Done()

	// Run initial cleanup
	c.runCleanup()

	ticker := time.NewTicker(c.cleanupPeriod)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			c.runCleanup()
		case <-c.stopChan:
			c.logger.Info().Msg("RetentionCleaner stopped")
			return
		}
	}
}

// runCleanup performs the actual cleanup operation
func (c *RetentionCleaner) runCleanup() {
	deleted, err := c.store.DeleteOlderThan(c.retentionDays)

	c.mu.Lock()
	c.totalCleanups++
	c.lastCleanup = time.Now()

	if err != nil {
		c.logger.Error().Err(err).Msg("Retention cleanup failed")
	} else {
		c.totalDeleted += deleted
		c.lastDeleteCount = deleted
		if deleted > 0 {
			c.logger.Info().
				Int64("deleted", deleted).
				Int("retention_days", c.retentionDays).
				Msg("Retention cleanup completed")
		} else {
			c.logger.Debug().
				Int("retention_days", c.retentionDays).
				Msg("Retention cleanup completed, no old data to delete")
		}
	}
	c.mu.Unlock()
}

// Stop gracefully stops the cleaner
func (c *RetentionCleaner) Stop() {
	c.stopOnce.Do(func() {
		close(c.stopChan)
		c.wg.Wait()
	})
}

// Stats returns current cleaner statistics
func (c *RetentionCleaner) Stats() RetentionCleanerStats {
	c.mu.RLock()
	defer c.mu.RUnlock()

	return RetentionCleanerStats{
		TotalDeleted:    c.totalDeleted,
		TotalCleanups:   c.totalCleanups,
		LastCleanup:     c.lastCleanup,
		LastDeleteCount: c.lastDeleteCount,
		RetentionDays:   c.retentionDays,
	}
}

// RunNow triggers an immediate cleanup (useful for testing or manual triggers)
func (c *RetentionCleaner) RunNow() {
	c.runCleanup()
}
