package storage

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/rs/zerolog"
)

// setupTestRetentionCleaner creates test store and cleaner
func setupTestRetentionCleaner(t *testing.T, config RetentionCleanerConfig) (*SQLiteStore, *RetentionCleaner, func()) {
	t.Helper()

	tmpDir, err := os.MkdirTemp("", "dht-retention-test-*")
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

	cleaner := NewRetentionCleaner(store, config, logger)

	cleanup := func() {
		cleaner.Stop()
		store.Close()
		os.RemoveAll(tmpDir)
	}

	return store, cleaner, cleanup
}

// TestNewRetentionCleaner tests cleaner creation
func TestNewRetentionCleaner(t *testing.T) {
	_, cleaner, cleanup := setupTestRetentionCleaner(t, DefaultRetentionCleanerConfig())
	defer cleanup()

	if cleaner == nil {
		t.Fatal("Expected non-nil cleaner")
	}
}

// TestRetentionCleaner_RunNow tests immediate cleanup
func TestRetentionCleaner_RunNow(t *testing.T) {
	config := RetentionCleanerConfig{
		RetentionDays: 30,
		CleanupPeriod: 1 * time.Hour, // Long period so we test manual trigger
	}

	store, cleaner, cleanup := setupTestRetentionCleaner(t, config)
	defer cleanup()

	// Wait for initial cleanup to complete
	time.Sleep(50 * time.Millisecond)

	now := time.Now().UTC()

	// Insert old readings (35 days ago)
	for i := 0; i < 10; i++ {
		reading := createTestReading(
			"sensor-01",
			float64(i),
			50.0,
			now.AddDate(0, 0, -35).Add(-time.Duration(i)*time.Hour),
		)
		store.InsertReading(reading)
	}

	// Insert recent readings
	for i := 0; i < 10; i++ {
		reading := createTestReading(
			"sensor-01",
			float64(i+100),
			50.0,
			now.Add(-time.Duration(i)*time.Hour),
		)
		store.InsertReading(reading)
	}

	// Verify 20 total
	stats, _ := store.GetStorageStats()
	if stats.TotalReadings != 20 {
		t.Fatalf("Expected 20 readings, got %d", stats.TotalReadings)
	}

	// Run cleanup manually
	cleaner.RunNow()

	// Wait a bit for cleanup to complete
	time.Sleep(100 * time.Millisecond)

	// Verify old readings removed
	stats, _ = store.GetStorageStats()
	if stats.TotalReadings != 10 {
		t.Errorf("Expected 10 readings after cleanup, got %d", stats.TotalReadings)
	}

	// Check cleaner stats
	cleanerStats := cleaner.Stats()
	if cleanerStats.LastDeleteCount != 10 {
		t.Errorf("LastDeleteCount = %d, want 10", cleanerStats.LastDeleteCount)
	}
}

// TestRetentionCleaner_PeriodicCleanup tests automatic periodic cleanup
func TestRetentionCleaner_PeriodicCleanup(t *testing.T) {
	config := RetentionCleanerConfig{
		RetentionDays: 1,                     // 1 day retention
		CleanupPeriod: 50 * time.Millisecond, // Fast for testing
	}

	store, cleaner, cleanup := setupTestRetentionCleaner(t, config)
	defer cleanup()

	now := time.Now().UTC()

	// Insert old readings (2 days ago)
	for i := 0; i < 5; i++ {
		reading := createTestReading(
			"sensor-01",
			float64(i),
			50.0,
			now.AddDate(0, 0, -2),
		)
		store.InsertReading(reading)
	}

	// Wait for automatic cleanup
	time.Sleep(200 * time.Millisecond)

	// Should have cleaned up
	stats := cleaner.Stats()
	if stats.TotalCleanups < 2 {
		t.Errorf("TotalCleanups = %d, expected >= 2", stats.TotalCleanups)
	}

	if stats.TotalDeleted != 5 {
		t.Errorf("TotalDeleted = %d, want 5", stats.TotalDeleted)
	}
}

// TestRetentionCleaner_NoDataToDelete tests cleanup with no old data
func TestRetentionCleaner_NoDataToDelete(t *testing.T) {
	config := RetentionCleanerConfig{
		RetentionDays: 30,
		CleanupPeriod: 1 * time.Hour,
	}

	store, cleaner, cleanup := setupTestRetentionCleaner(t, config)
	defer cleanup()

	// Insert only recent readings
	for i := 0; i < 10; i++ {
		reading := createTestReading(
			"sensor-01",
			float64(i),
			50.0,
			time.Now().UTC().Add(-time.Duration(i)*time.Hour),
		)
		store.InsertReading(reading)
	}

	// Run cleanup
	cleaner.RunNow()
	time.Sleep(100 * time.Millisecond)

	// Nothing should be deleted
	stats, _ := store.GetStorageStats()
	if stats.TotalReadings != 10 {
		t.Errorf("TotalReadings = %d, want 10 (nothing should be deleted)", stats.TotalReadings)
	}

	cleanerStats := cleaner.Stats()
	if cleanerStats.LastDeleteCount != 0 {
		t.Errorf("LastDeleteCount = %d, want 0", cleanerStats.LastDeleteCount)
	}
}

// TestRetentionCleaner_Stop tests graceful shutdown
func TestRetentionCleaner_Stop(t *testing.T) {
	config := RetentionCleanerConfig{
		RetentionDays: 30,
		CleanupPeriod: 50 * time.Millisecond,
	}

	_, cleaner, cleanup := setupTestRetentionCleaner(t, config)

	// Let it run a few cycles
	time.Sleep(150 * time.Millisecond)

	// Stop should complete without hanging
	done := make(chan bool)
	go func() {
		cleaner.Stop()
		done <- true
	}()

	select {
	case <-done:
		// Good, stopped successfully
	case <-time.After(1 * time.Second):
		t.Fatal("Stop() timed out")
	}

	cleanup()
}

// TestRetentionCleaner_Stats tests statistics tracking
func TestRetentionCleaner_Stats(t *testing.T) {
	config := RetentionCleanerConfig{
		RetentionDays: 30,
		CleanupPeriod: 1 * time.Hour,
	}

	_, cleaner, cleanup := setupTestRetentionCleaner(t, config)
	defer cleanup()

	// Initial cleanup should have run
	time.Sleep(100 * time.Millisecond)

	stats := cleaner.Stats()

	if stats.RetentionDays != 30 {
		t.Errorf("RetentionDays = %d, want 30", stats.RetentionDays)
	}

	if stats.TotalCleanups < 1 {
		t.Errorf("TotalCleanups = %d, expected >= 1", stats.TotalCleanups)
	}

	if stats.LastCleanup.IsZero() {
		t.Error("LastCleanup should not be zero")
	}
}

// TestRetentionCleaner_MultipleRetentionPeriods tests different retention settings
func TestRetentionCleaner_MultipleRetentionPeriods(t *testing.T) {
	testCases := []struct {
		name          string
		retentionDays int
		oldDataDays   int
		shouldDelete  bool
	}{
		{"30 day retention, 35 day old data", 30, 35, true},
		{"30 day retention, 25 day old data", 30, 25, false},
		{"7 day retention, 10 day old data", 7, 10, true},
		{"1 day retention, 2 day old data", 1, 2, true},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			config := RetentionCleanerConfig{
				RetentionDays: tc.retentionDays,
				CleanupPeriod: 1 * time.Hour,
			}

			store, cleaner, cleanup := setupTestRetentionCleaner(t, config)
			defer cleanup()

			now := time.Now().UTC()

			// Insert old reading
			reading := createTestReading(
				"sensor-01",
				22.0,
				50.0,
				now.AddDate(0, 0, -tc.oldDataDays),
			)
			store.InsertReading(reading)

			// Run cleanup
			cleaner.RunNow()
			time.Sleep(100 * time.Millisecond)

			stats, _ := store.GetStorageStats()

			if tc.shouldDelete && stats.TotalReadings != 0 {
				t.Errorf("Expected reading to be deleted, but TotalReadings = %d", stats.TotalReadings)
			}

			if !tc.shouldDelete && stats.TotalReadings != 1 {
				t.Errorf("Expected reading to be kept, but TotalReadings = %d", stats.TotalReadings)
			}
		})
	}
}
