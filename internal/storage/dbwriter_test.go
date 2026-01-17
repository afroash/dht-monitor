package storage

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/rs/zerolog"
)

// setupTestDBWriter creates a test store and writer
func setupTestDBWriter(t *testing.T, config DBWriterConfig) (*SQLiteStore, *DBWriter, func()) {
	t.Helper()

	tmpDir, err := os.MkdirTemp("", "dht-writer-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}

	dbPath := filepath.Join(tmpDir, "test.db")
	logger := zerolog.New(os.Stdout).With().Timestamp().Logger().Level(zerolog.DebugLevel)

	store, err := NewSQLiteStore(dbPath, logger)
	if err != nil {
		os.RemoveAll(tmpDir)
		t.Fatalf("Failed to create store: %v", err)
	}

	writer := NewDBWriter(store, config, logger)

	cleanup := func() {
		writer.Stop()
		store.Close()
		os.RemoveAll(tmpDir)
	}

	return store, writer, cleanup
}

// TestNewDBWriter tests writer creation
func TestNewDBWriter(t *testing.T) {
	store, writer, cleanup := setupTestDBWriter(t, DefaultDBWriterConfig())
	defer cleanup()

	if writer == nil {
		t.Fatal("Expected non-nil writer")
	}

	if store == nil {
		t.Fatal("Expected non-nil store")
	}
}

// TestDBWriter_Write tests queuing readings
func TestDBWriter_Write(t *testing.T) {
	_, writer, cleanup := setupTestDBWriter(t, DefaultDBWriterConfig())
	defer cleanup()

	reading := createTestReading("sensor-01", 22.5, 45.0, time.Now().UTC())

	ok := writer.Write(reading)
	if !ok {
		t.Error("Write should return true when channel has space")
	}

	stats := writer.Stats()
	if stats.QueueLength == 0 {
		t.Error("Queue should have at least one item after write")
	}
}

// TestDBWriter_BatchFlush tests automatic batch flushing
func TestDBWriter_BatchFlush(t *testing.T) {
	config := DBWriterConfig{
		BatchSize:   10,
		FlushPeriod: 5 * time.Second, // Long period so we test batch size trigger
		ChannelSize: 100,
	}

	store, writer, cleanup := setupTestDBWriter(t, config)
	defer cleanup()

	// Write exactly batch size readings
	for i := 0; i < 10; i++ {
		reading := createTestReading("sensor-01", float64(i), 45.0, time.Now().UTC())
		writer.Write(reading)
	}

	// Give time for flush to occur
	time.Sleep(100 * time.Millisecond)

	// Check database has the readings
	stats, err := store.GetStorageStats()
	if err != nil {
		t.Fatalf("GetStorageStats failed: %v", err)
	}

	if stats.TotalReadings != 10 {
		t.Errorf("TotalReadings = %d, want 10", stats.TotalReadings)
	}

	// Check writer stats
	writerStats := writer.Stats()
	if writerStats.TotalWritten != 10 {
		t.Errorf("TotalWritten = %d, want 10", writerStats.TotalWritten)
	}

	if writerStats.TotalBatches != 1 {
		t.Errorf("TotalBatches = %d, want 1", writerStats.TotalBatches)
	}
}

// TestDBWriter_PeriodicFlush tests time-based flushing
func TestDBWriter_PeriodicFlush(t *testing.T) {
	config := DBWriterConfig{
		BatchSize:   100, // Large batch size so we test time trigger
		FlushPeriod: 50 * time.Millisecond,
		ChannelSize: 100,
	}

	store, writer, cleanup := setupTestDBWriter(t, config)
	defer cleanup()

	// Write fewer than batch size
	for i := 0; i < 5; i++ {
		reading := createTestReading("sensor-01", float64(i), 45.0, time.Now().UTC())
		writer.Write(reading)
	}

	// Wait for periodic flush
	time.Sleep(100 * time.Millisecond)

	// Check database
	stats, _ := store.GetStorageStats()
	if stats.TotalReadings != 5 {
		t.Errorf("TotalReadings = %d, want 5", stats.TotalReadings)
	}
}

// TestDBWriter_Stop tests graceful shutdown
func TestDBWriter_Stop(t *testing.T) {
	config := DBWriterConfig{
		BatchSize:   100,              // Large batch size
		FlushPeriod: 10 * time.Second, // Long period
		ChannelSize: 100,
	}

	store, writer, cleanup := setupTestDBWriter(t, config)

	// Write some readings (less than batch, before periodic flush)
	for i := 0; i < 15; i++ {
		reading := createTestReading("sensor-01", float64(i), 45.0, time.Now().UTC())
		writer.Write(reading)
	}

	// Stop should flush remaining
	writer.Stop()

	// Check all readings were written
	stats, _ := store.GetStorageStats()
	if stats.TotalReadings != 15 {
		t.Errorf("TotalReadings = %d, want 15 (remaining should be flushed on stop)", stats.TotalReadings)
	}

	// Clean up - cleanup() calls Stop() again which should be safe (idempotent)
	cleanup()
}

// TestDBWriter_ChannelFull tests behavior when channel is full
func TestDBWriter_ChannelFull(t *testing.T) {
	config := DBWriterConfig{
		BatchSize:   1000,             // Very large batch
		FlushPeriod: 10 * time.Second, // Very long period
		ChannelSize: 5,                // Very small channel
	}

	_, writer, cleanup := setupTestDBWriter(t, config)
	defer cleanup()

	// Fill the channel
	for i := 0; i < 5; i++ {
		reading := createTestReading("sensor-01", float64(i), 45.0, time.Now().UTC())
		writer.Write(reading)
	}

	// This write should fail (channel full)
	reading := createTestReading("sensor-01", 99.0, 45.0, time.Now().UTC())
	ok := writer.Write(reading)

	if ok {
		t.Error("Write should return false when channel is full")
	}
}

// TestDBWriter_HighVolume tests high volume writing
func TestDBWriter_HighVolume(t *testing.T) {
	config := DBWriterConfig{
		BatchSize:   100,
		FlushPeriod: 100 * time.Millisecond,
		ChannelSize: 10000,
	}

	store, writer, cleanup := setupTestDBWriter(t, config)
	defer cleanup()

	// Write 1000 readings quickly
	for i := 0; i < 1000; i++ {
		reading := createTestReading("sensor-01", float64(i), 45.0, time.Now().UTC())
		writer.Write(reading)
	}

	// Wait for all batches to flush
	time.Sleep(500 * time.Millisecond)

	// Verify
	stats, _ := store.GetStorageStats()
	if stats.TotalReadings != 1000 {
		t.Errorf("TotalReadings = %d, want 1000", stats.TotalReadings)
	}

	writerStats := writer.Stats()
	if writerStats.TotalWritten != 1000 {
		t.Errorf("TotalWritten = %d, want 1000", writerStats.TotalWritten)
	}

	// Should have ~10 batches (1000 / 100)
	if writerStats.TotalBatches < 9 || writerStats.TotalBatches > 12 {
		t.Errorf("TotalBatches = %d, expected ~10", writerStats.TotalBatches)
	}
}

// TestDBWriter_ConcurrentWrites tests thread safety
func TestDBWriter_ConcurrentWrites(t *testing.T) {
	config := DBWriterConfig{
		BatchSize:   50,
		FlushPeriod: 50 * time.Millisecond,
		ChannelSize: 5000,
	}

	store, writer, cleanup := setupTestDBWriter(t, config)
	defer cleanup()

	// 10 goroutines each writing 100 readings
	done := make(chan bool)
	for g := 0; g < 10; g++ {
		go func(goroutineID int) {
			for i := 0; i < 100; i++ {
				reading := createTestReading(
					"sensor-01",
					float64(goroutineID*100+i),
					45.0,
					time.Now().UTC(),
				)
				writer.Write(reading)
			}
			done <- true
		}(g)
	}

	// Wait for all goroutines
	for i := 0; i < 10; i++ {
		<-done
	}

	// Wait for flushes
	time.Sleep(500 * time.Millisecond)

	// Verify all written
	stats, _ := store.GetStorageStats()
	if stats.TotalReadings != 1000 {
		t.Errorf("TotalReadings = %d, want 1000", stats.TotalReadings)
	}
}

// TestDBWriter_Stats tests statistics tracking
func TestDBWriter_Stats(t *testing.T) {
	config := DBWriterConfig{
		BatchSize:   10,
		FlushPeriod: 50 * time.Millisecond,
		ChannelSize: 100,
	}

	_, writer, cleanup := setupTestDBWriter(t, config)
	defer cleanup()

	// Initial stats
	stats := writer.Stats()
	if stats.TotalWritten != 0 {
		t.Errorf("Initial TotalWritten = %d, want 0", stats.TotalWritten)
	}

	// Write and wait for flush
	for i := 0; i < 25; i++ {
		reading := createTestReading("sensor-01", float64(i), 45.0, time.Now().UTC())
		writer.Write(reading)
	}
	time.Sleep(200 * time.Millisecond)

	// Check stats
	stats = writer.Stats()
	if stats.TotalWritten != 25 {
		t.Errorf("TotalWritten = %d, want 25", stats.TotalWritten)
	}

	if stats.TotalBatches < 2 {
		t.Errorf("TotalBatches = %d, want >= 2", stats.TotalBatches)
	}

	if stats.LastWriteTime.IsZero() {
		t.Error("LastWriteTime should not be zero")
	}
}

// BenchmarkDBWriter_Write benchmarks the write method
func BenchmarkDBWriter_Write(b *testing.B) {
	tmpDir, _ := os.MkdirTemp("", "dht-writer-bench-*")
	defer os.RemoveAll(tmpDir)

	logger := zerolog.New(os.Stdout).With().Timestamp().Logger().Level(zerolog.ErrorLevel)
	store, _ := NewSQLiteStore(filepath.Join(tmpDir, "bench.db"), logger)
	defer store.Close()

	config := DBWriterConfig{
		BatchSize:   100,
		FlushPeriod: 100 * time.Millisecond,
		ChannelSize: 100000,
	}
	writer := NewDBWriter(store, config, logger)
	defer writer.Stop()

	reading := createTestReading("sensor-01", 22.5, 45.0, time.Now().UTC())

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		writer.Write(reading)
	}
}
