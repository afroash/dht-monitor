package storage

import (
	"sync"
	"time"

	"github.com/afroash/dht-monitor/internal/models"
	"github.com/rs/zerolog"
)

// DBWriter handles async batched writes to the database
type DBWriter struct {
	store       *SQLiteStore
	logger      zerolog.Logger
	writeChan   chan *models.Reading
	batchSize   int
	flushPeriod time.Duration
	stopChan    chan struct{}
	stopOnce    sync.Once
	wg          sync.WaitGroup

	// Stats
	mu            sync.RWMutex
	totalWritten  int64
	totalBatches  int64
	totalErrors   int64
	lastWriteTime time.Time
}

// DBWriterConfig holds configuration for the async writer
type DBWriterConfig struct {
	BatchSize   int           // Number of readings to batch before writing (default: 100)
	FlushPeriod time.Duration // Max time between flushes (default: 5s)
	ChannelSize int           // Size of the write channel buffer (default: 1000)
}

// DefaultDBWriterConfig returns sensible defaults
func DefaultDBWriterConfig() DBWriterConfig {
	return DBWriterConfig{
		BatchSize:   100,
		FlushPeriod: 5 * time.Second,
		ChannelSize: 1000,
	}
}

// DBWriterStats contains statistics about the writer
type DBWriterStats struct {
	TotalWritten  int64     `json:"total_written"`
	TotalBatches  int64     `json:"total_batches"`
	TotalErrors   int64     `json:"total_errors"`
	LastWriteTime time.Time `json:"last_write_time,omitempty"`
	QueueLength   int       `json:"queue_length"`
}

// NewDBWriter creates a new async database writer
func NewDBWriter(store *SQLiteStore, config DBWriterConfig, logger zerolog.Logger) *DBWriter {
	w := &DBWriter{
		store:       store,
		logger:      logger,
		writeChan:   make(chan *models.Reading, config.ChannelSize),
		batchSize:   config.BatchSize,
		flushPeriod: config.FlushPeriod,
		stopChan:    make(chan struct{}),
	}

	w.wg.Add(1)
	go w.writerLoop()

	logger.Info().
		Int("batch_size", config.BatchSize).
		Dur("flush_period", config.FlushPeriod).
		Int("channel_size", config.ChannelSize).
		Msg("DBWriter started")

	return w
}

// Write queues a reading for async writing to the database
// Returns true if queued, false if dropped (channel full)
func (w *DBWriter) Write(reading *models.Reading) bool {
	select {
	case w.writeChan <- reading:
		return true
	default:
		w.logger.Warn().Msg("DBWriter channel full, dropping reading")
		return false
	}
}

// writerLoop is the background goroutine that batches and writes readings
func (w *DBWriter) writerLoop() {
	defer w.wg.Done()

	batch := make([]*models.Reading, 0, w.batchSize)
	ticker := time.NewTicker(w.flushPeriod)
	defer ticker.Stop()

	for {
		select {
		case reading := <-w.writeChan:
			batch = append(batch, reading)
			if len(batch) >= w.batchSize {
				w.flush(batch)
				batch = make([]*models.Reading, 0, w.batchSize)
			}

		case <-ticker.C:
			if len(batch) > 0 {
				w.flush(batch)
				batch = make([]*models.Reading, 0, w.batchSize)
			}

		case <-w.stopChan:
			// Drain remaining readings from channel
			draining := true
			for draining {
				select {
				case reading := <-w.writeChan:
					batch = append(batch, reading)
				default:
					draining = false
				}
			}
			// Flush any remaining
			if len(batch) > 0 {
				w.flush(batch)
			}
			w.logger.Info().Msg("DBWriter stopped")
			return
		}
	}
}

// flush writes a batch to the database
func (w *DBWriter) flush(batch []*models.Reading) {
	if len(batch) == 0 {
		return
	}

	err := w.store.InsertBatch(batch)

	w.mu.Lock()
	if err != nil {
		w.totalErrors++
		w.logger.Error().Err(err).Int("batch_size", len(batch)).Msg("Failed to write batch")
	} else {
		w.totalWritten += int64(len(batch))
		w.totalBatches++
		w.lastWriteTime = time.Now()
		w.logger.Debug().Int("count", len(batch)).Msg("Flushed batch")
	}
	w.mu.Unlock()
}

// Stop gracefully stops the writer, flushing any remaining data
func (w *DBWriter) Stop() {
	w.stopOnce.Do(func() {
		close(w.stopChan)
		w.wg.Wait()
	})
}

// Stats returns current writer statistics
func (w *DBWriter) Stats() DBWriterStats {
	w.mu.RLock()
	defer w.mu.RUnlock()

	return DBWriterStats{
		TotalWritten:  w.totalWritten,
		TotalBatches:  w.totalBatches,
		TotalErrors:   w.totalErrors,
		LastWriteTime: w.lastWriteTime,
		QueueLength:   len(w.writeChan),
	}
}
