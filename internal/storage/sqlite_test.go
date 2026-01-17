package storage

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/rs/zerolog"

	"github.com/afroash/dht-monitor/internal/models"
)

// testLogger creates a logger for tests
func testLogger() zerolog.Logger {
	return zerolog.New(os.Stdout).With().Timestamp().Logger().Level(zerolog.DebugLevel)
}

// setupTestDB creates a temporary database for testing
func setupTestDB(t *testing.T) (*SQLiteStore, func()) {
	t.Helper()

	// Create temp directory for test database
	tmpDir, err := os.MkdirTemp("", "dht-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}

	dbPath := filepath.Join(tmpDir, "test.db")
	store, err := NewSQLiteStore(dbPath, testLogger())
	if err != nil {
		os.RemoveAll(tmpDir)
		t.Fatalf("Failed to create store: %v", err)
	}

	cleanup := func() {
		store.Close()
		os.RemoveAll(tmpDir)
	}

	return store, cleanup
}

// createTestReading creates a reading with specified parameters
func createTestReading(sensorID string, temp, humidity float64, timestamp time.Time) *models.Reading {
	return &models.Reading{
		SensorID:    sensorID,
		Temperature: temp,
		Humidity:    humidity,
		Timestamp:   timestamp,
	}
}

// TestNewSQLiteStore tests store creation
func TestNewSQLiteStore(t *testing.T) {
	store, cleanup := setupTestDB(t)
	defer cleanup()

	if store == nil {
		t.Fatal("Expected non-nil store")
	}

	if store.db == nil {
		t.Fatal("Expected non-nil database connection")
	}
}

// TestNewSQLiteStore_InvalidPath tests creation with invalid path
func TestNewSQLiteStore_InvalidPath(t *testing.T) {
	// Try to create store in non-existent directory without creating it
	_, err := NewSQLiteStore("/nonexistent/path/that/cannot/exist/test.db", testLogger())
	if err == nil {
		t.Fatal("Expected error for invalid path")
	}
}

// TestMigrate tests schema creation
func TestMigrate(t *testing.T) {
	store, cleanup := setupTestDB(t)
	defer cleanup()

	// Migrate should have been called in NewSQLiteStore
	// Verify tables exist by inserting and querying
	reading := createTestReading("test-sensor", 22.5, 45.0, time.Now())
	err := store.InsertReading(reading)
	if err != nil {
		t.Fatalf("Insert failed after migration: %v", err)
	}
}

// TestMigrate_Idempotent tests that migration can be called multiple times
func TestMigrate_Idempotent(t *testing.T) {
	store, cleanup := setupTestDB(t)
	defer cleanup()

	// Call migrate again - should not error
	err := store.Migrate()
	if err != nil {
		t.Fatalf("Second migration failed: %v", err)
	}

	// And again
	err = store.Migrate()
	if err != nil {
		t.Fatalf("Third migration failed: %v", err)
	}
}

// TestInsertReading tests single reading insertion
func TestInsertReading(t *testing.T) {
	store, cleanup := setupTestDB(t)
	defer cleanup()

	now := time.Now().UTC().Truncate(time.Second)
	reading := createTestReading("sensor-01", 23.5, 55.0, now)

	err := store.InsertReading(reading)
	if err != nil {
		t.Fatalf("InsertReading failed: %v", err)
	}

	// Verify by querying
	latest, err := store.GetLatestReading("sensor-01")
	if err != nil {
		t.Fatalf("GetLatestReading failed: %v", err)
	}

	if latest == nil {
		t.Fatal("Expected reading, got nil")
	}

	if latest.SensorID != "sensor-01" {
		t.Errorf("SensorID = %q, want %q", latest.SensorID, "sensor-01")
	}

	if latest.Temperature != 23.5 {
		t.Errorf("Temperature = %v, want %v", latest.Temperature, 23.5)
	}

	if latest.Humidity != 55.0 {
		t.Errorf("Humidity = %v, want %v", latest.Humidity, 55.0)
	}
}

// TestInsertBatch tests batch insertion
func TestInsertBatch(t *testing.T) {
	store, cleanup := setupTestDB(t)
	defer cleanup()

	baseTime := time.Now().UTC().Truncate(time.Second)
	readings := make([]*models.Reading, 100)

	for i := 0; i < 100; i++ {
		readings[i] = createTestReading(
			"sensor-01",
			20.0+float64(i)*0.1,
			40.0+float64(i)*0.1,
			baseTime.Add(time.Duration(i)*time.Minute),
		)
	}

	err := store.InsertBatch(readings)
	if err != nil {
		t.Fatalf("InsertBatch failed: %v", err)
	}

	// Verify count
	stats, err := store.GetStorageStats()
	if err != nil {
		t.Fatalf("GetStorageStats failed: %v", err)
	}

	if stats.TotalReadings != 100 {
		t.Errorf("TotalReadings = %d, want 100", stats.TotalReadings)
	}
}

// TestInsertBatch_Empty tests batch insertion with empty slice
func TestInsertBatch_Empty(t *testing.T) {
	store, cleanup := setupTestDB(t)
	defer cleanup()

	err := store.InsertBatch([]*models.Reading{})
	if err != nil {
		t.Fatalf("InsertBatch with empty slice failed: %v", err)
	}
}

// TestInsertBatch_Nil tests batch insertion with nil slice
func TestInsertBatch_Nil(t *testing.T) {
	store, cleanup := setupTestDB(t)
	defer cleanup()

	err := store.InsertBatch(nil)
	if err != nil {
		t.Fatalf("InsertBatch with nil slice failed: %v", err)
	}
}

// TestGetReadingsInRange tests querying by time range
func TestGetReadingsInRange(t *testing.T) {
	store, cleanup := setupTestDB(t)
	defer cleanup()

	// Insert readings over 24 hours
	baseTime := time.Now().UTC().Add(-24 * time.Hour).Truncate(time.Second)
	for i := 0; i < 48; i++ { // Every 30 minutes
		reading := createTestReading(
			"sensor-01",
			20.0+float64(i)*0.5,
			50.0,
			baseTime.Add(time.Duration(i)*30*time.Minute),
		)
		store.InsertReading(reading)
	}

	// Query last 6 hours
	end := time.Now().UTC()
	start := end.Add(-6 * time.Hour)

	readings, err := store.GetReadingsInRange("sensor-01", start, end, 100)
	if err != nil {
		t.Fatalf("GetReadingsInRange failed: %v", err)
	}

	// Should have approximately 12 readings (6 hours * 2 per hour)
	if len(readings) < 10 || len(readings) > 14 {
		t.Errorf("Got %d readings, expected ~12", len(readings))
	}

	// Verify order (newest first)
	for i := 1; i < len(readings); i++ {
		if readings[i].Timestamp.After(readings[i-1].Timestamp) {
			t.Errorf("Readings not in descending order at index %d", i)
		}
	}
}

// TestGetReadingsInRange_AllSensors tests querying all sensors
func TestGetReadingsInRange_AllSensors(t *testing.T) {
	store, cleanup := setupTestDB(t)
	defer cleanup()

	now := time.Now().UTC().Truncate(time.Second)

	// Insert readings for multiple sensors
	store.InsertReading(createTestReading("sensor-01", 22.0, 45.0, now))
	store.InsertReading(createTestReading("sensor-02", 23.0, 50.0, now))
	store.InsertReading(createTestReading("sensor-03", 24.0, 55.0, now))

	// Query all sensors (empty string)
	start := now.Add(-1 * time.Hour)
	end := now.Add(1 * time.Hour)

	readings, err := store.GetReadingsInRange("", start, end, 100)
	if err != nil {
		t.Fatalf("GetReadingsInRange failed: %v", err)
	}

	if len(readings) != 3 {
		t.Errorf("Got %d readings, want 3", len(readings))
	}
}

// TestGetReadingsBefore tests backward scrolling
func TestGetReadingsBefore(t *testing.T) {
	store, cleanup := setupTestDB(t)
	defer cleanup()

	// Insert 50 readings, 1 minute apart
	baseTime := time.Now().UTC().Truncate(time.Second)
	for i := 0; i < 50; i++ {
		reading := createTestReading(
			"sensor-01",
			float64(i),
			50.0,
			baseTime.Add(-time.Duration(i)*time.Minute),
		)
		store.InsertReading(reading)
	}

	// Get readings before 25 minutes ago
	before := baseTime.Add(-25 * time.Minute)
	readings, err := store.GetReadingsBefore("sensor-01", before, 10)
	if err != nil {
		t.Fatalf("GetReadingsBefore failed: %v", err)
	}

	if len(readings) != 10 {
		t.Errorf("Got %d readings, want 10", len(readings))
	}

	// All readings should be before the 'before' time
	for i, r := range readings {
		if !r.Timestamp.Before(before) {
			t.Errorf("Reading %d at %v is not before %v", i, r.Timestamp, before)
		}
	}

	// Should be in descending order (newest first)
	for i := 1; i < len(readings); i++ {
		if readings[i].Timestamp.After(readings[i-1].Timestamp) {
			t.Errorf("Readings not in descending order at index %d", i)
		}
	}
}

// TestGetReadingsAfter tests forward scrolling
func TestGetReadingsAfter(t *testing.T) {
	store, cleanup := setupTestDB(t)
	defer cleanup()

	// Insert 50 readings, 1 minute apart
	baseTime := time.Now().UTC().Truncate(time.Second)
	for i := 0; i < 50; i++ {
		reading := createTestReading(
			"sensor-01",
			float64(i),
			50.0,
			baseTime.Add(-time.Duration(i)*time.Minute),
		)
		store.InsertReading(reading)
	}

	// Get readings after 25 minutes ago
	after := baseTime.Add(-25 * time.Minute)
	readings, err := store.GetReadingsAfter("sensor-01", after, 10)
	if err != nil {
		t.Fatalf("GetReadingsAfter failed: %v", err)
	}

	if len(readings) != 10 {
		t.Errorf("Got %d readings, want 10", len(readings))
	}

	// All readings should be after the 'after' time
	for i, r := range readings {
		if !r.Timestamp.After(after) {
			t.Errorf("Reading %d at %v is not after %v", i, r.Timestamp, after)
		}
	}

	// Should be in descending order (newest first) for consistency
	for i := 1; i < len(readings); i++ {
		if readings[i].Timestamp.After(readings[i-1].Timestamp) {
			t.Errorf("Readings not in descending order at index %d", i)
		}
	}
}

// TestGetLatestReading tests getting the most recent reading
func TestGetLatestReading(t *testing.T) {
	store, cleanup := setupTestDB(t)
	defer cleanup()

	baseTime := time.Now().UTC().Truncate(time.Second)

	// Insert readings at different times
	store.InsertReading(createTestReading("sensor-01", 20.0, 45.0, baseTime.Add(-2*time.Hour)))
	store.InsertReading(createTestReading("sensor-01", 22.0, 50.0, baseTime.Add(-1*time.Hour)))
	store.InsertReading(createTestReading("sensor-01", 25.0, 55.0, baseTime)) // Most recent

	latest, err := store.GetLatestReading("sensor-01")
	if err != nil {
		t.Fatalf("GetLatestReading failed: %v", err)
	}

	if latest == nil {
		t.Fatal("Expected reading, got nil")
	}

	if latest.Temperature != 25.0 {
		t.Errorf("Temperature = %v, want 25.0 (most recent)", latest.Temperature)
	}
}

// TestGetLatestReading_NoReadings tests getting latest when none exist
func TestGetLatestReading_NoReadings(t *testing.T) {
	store, cleanup := setupTestDB(t)
	defer cleanup()

	latest, err := store.GetLatestReading("nonexistent-sensor")
	if err != nil {
		t.Fatalf("GetLatestReading failed: %v", err)
	}

	if latest != nil {
		t.Errorf("Expected nil for nonexistent sensor, got %+v", latest)
	}
}

// TestGetDailyStats tests daily statistics aggregation
func TestGetDailyStats(t *testing.T) {
	store, cleanup := setupTestDB(t)
	defer cleanup()

	// Insert readings over 3 days (all in the past to avoid timing issues)
	for day := 1; day <= 3; day++ {
		baseTime := time.Now().UTC().AddDate(0, 0, -day).Truncate(24 * time.Hour)
		for hour := 0; hour < 24; hour++ {
			reading := createTestReading(
				"sensor-01",
				20.0+float64(hour), // Temperature varies by hour
				50.0+float64(day),  // Humidity varies by day
				baseTime.Add(time.Duration(hour)*time.Hour),
			)
			store.InsertReading(reading)
		}
	}

	// Get stats for last 7 days (range covers all inserted data)
	start := time.Now().UTC().AddDate(0, 0, -7)
	end := time.Now().UTC()

	stats, err := store.GetDailyStats("sensor-01", start, end)
	if err != nil {
		t.Fatalf("GetDailyStats failed: %v", err)
	}

	if len(stats) != 3 {
		t.Errorf("Got %d daily stats, want 3", len(stats))
	}

	// Verify each day has reasonable stats
	for _, stat := range stats {
		if stat.ReadingCount != 24 {
			t.Errorf("Day %v: ReadingCount = %d, want 24", stat.Date, stat.ReadingCount)
		}

		if stat.MinTemperature >= stat.MaxTemperature {
			t.Errorf("Day %v: Min temp %v >= Max temp %v",
				stat.Date, stat.MinTemperature, stat.MaxTemperature)
		}

		if stat.AvgTemperature < stat.MinTemperature || stat.AvgTemperature > stat.MaxTemperature {
			t.Errorf("Day %v: Avg temp %v outside min/max range",
				stat.Date, stat.AvgTemperature)
		}
	}
}

// TestDeleteOlderThan tests data retention cleanup
func TestDeleteOlderThan(t *testing.T) {
	store, cleanup := setupTestDB(t)
	defer cleanup()

	now := time.Now().UTC()

	// Insert readings: 5 recent, 5 old (35 days ago)
	for i := 0; i < 5; i++ {
		store.InsertReading(createTestReading(
			"sensor-01",
			float64(i),
			50.0,
			now.Add(-time.Duration(i)*time.Hour),
		))
	}

	for i := 0; i < 5; i++ {
		store.InsertReading(createTestReading(
			"sensor-01",
			float64(i+100),
			50.0,
			now.AddDate(0, 0, -35).Add(-time.Duration(i)*time.Hour),
		))
	}

	// Verify 10 total
	stats, _ := store.GetStorageStats()
	if stats.TotalReadings != 10 {
		t.Fatalf("Expected 10 readings before cleanup, got %d", stats.TotalReadings)
	}

	// Delete readings older than 30 days
	deleted, err := store.DeleteOlderThan(30)
	if err != nil {
		t.Fatalf("DeleteOlderThan failed: %v", err)
	}

	if deleted != 5 {
		t.Errorf("Deleted %d readings, want 5", deleted)
	}

	// Verify 5 remain
	stats, _ = store.GetStorageStats()
	if stats.TotalReadings != 5 {
		t.Errorf("Expected 5 readings after cleanup, got %d", stats.TotalReadings)
	}
}

// TestGetStorageStats tests statistics retrieval
func TestGetStorageStats(t *testing.T) {
	store, cleanup := setupTestDB(t)
	defer cleanup()

	// Initially empty
	stats, err := store.GetStorageStats()
	if err != nil {
		t.Fatalf("GetStorageStats failed: %v", err)
	}

	if stats.TotalReadings != 0 {
		t.Errorf("TotalReadings = %d, want 0", stats.TotalReadings)
	}

	// Add readings from multiple sensors
	now := time.Now().UTC()
	store.InsertReading(createTestReading("sensor-01", 22.0, 45.0, now))
	store.InsertReading(createTestReading("sensor-01", 23.0, 46.0, now.Add(time.Minute)))
	store.InsertReading(createTestReading("sensor-02", 24.0, 47.0, now.Add(2*time.Minute)))

	stats, err = store.GetStorageStats()
	if err != nil {
		t.Fatalf("GetStorageStats failed: %v", err)
	}

	if stats.TotalReadings != 3 {
		t.Errorf("TotalReadings = %d, want 3", stats.TotalReadings)
	}

	if stats.UniqueSensors != 2 {
		t.Errorf("UniqueSensors = %d, want 2", stats.UniqueSensors)
	}
}

// TestGetSensorIDs tests retrieving unique sensor IDs
func TestGetSensorIDs(t *testing.T) {
	store, cleanup := setupTestDB(t)
	defer cleanup()

	now := time.Now().UTC()

	// Insert readings from various sensors
	sensors := []string{"sensor-01", "sensor-02", "sensor-03", "bedroom", "kitchen"}
	for _, s := range sensors {
		store.InsertReading(createTestReading(s, 22.0, 45.0, now))
	}

	ids, err := store.GetSensorIDs()
	if err != nil {
		t.Fatalf("GetSensorIDs failed: %v", err)
	}

	if len(ids) != 5 {
		t.Errorf("Got %d sensor IDs, want 5", len(ids))
	}

	// Verify all expected sensors are present
	idMap := make(map[string]bool)
	for _, id := range ids {
		idMap[id] = true
	}

	for _, expected := range sensors {
		if !idMap[expected] {
			t.Errorf("Missing sensor ID: %s", expected)
		}
	}
}

// TestConcurrentInserts tests thread safety of insertions
func TestConcurrentInserts(t *testing.T) {
	store, cleanup := setupTestDB(t)
	defer cleanup()

	// Run with: go test -race ./internal/storage/...
	done := make(chan bool)
	now := time.Now().UTC()

	// 10 goroutines each inserting 100 readings
	for g := 0; g < 10; g++ {
		go func(goroutineID int) {
			for i := 0; i < 100; i++ {
				reading := createTestReading(
					"sensor-01",
					float64(goroutineID*100+i),
					50.0,
					now.Add(time.Duration(goroutineID*100+i)*time.Millisecond),
				)
				store.InsertReading(reading)
			}
			done <- true
		}(g)
	}

	// Wait for all goroutines
	for i := 0; i < 10; i++ {
		<-done
	}

	// Verify total count
	stats, err := store.GetStorageStats()
	if err != nil {
		t.Fatalf("GetStorageStats failed: %v", err)
	}

	if stats.TotalReadings != 1000 {
		t.Errorf("TotalReadings = %d, want 1000", stats.TotalReadings)
	}
}

// TestConcurrentReadsAndWrites tests concurrent access patterns
func TestConcurrentReadsAndWrites(t *testing.T) {
	store, cleanup := setupTestDB(t)
	defer cleanup()

	// Pre-populate with some data
	now := time.Now().UTC()
	for i := 0; i < 100; i++ {
		store.InsertReading(createTestReading("sensor-01", float64(i), 50.0, now.Add(-time.Duration(i)*time.Minute)))
	}

	done := make(chan bool)

	// Writers
	for w := 0; w < 5; w++ {
		go func(writerID int) {
			for i := 0; i < 50; i++ {
				store.InsertReading(createTestReading(
					"sensor-01",
					float64(1000+writerID*50+i),
					50.0,
					time.Now().UTC(),
				))
			}
			done <- true
		}(w)
	}

	// Readers
	for r := 0; r < 5; r++ {
		go func() {
			for i := 0; i < 50; i++ {
				store.GetLatestReading("sensor-01")
				store.GetReadingsInRange("sensor-01", now.Add(-1*time.Hour), now, 50)
				store.GetStorageStats()
			}
			done <- true
		}()
	}

	// Wait for all
	for i := 0; i < 10; i++ {
		<-done
	}

	// Verify data integrity
	stats, _ := store.GetStorageStats()
	expected := int64(100 + 5*50) // Initial + writers
	if stats.TotalReadings != expected {
		t.Errorf("TotalReadings = %d, want %d", stats.TotalReadings, expected)
	}
}

// TestClose tests database connection closing
func TestClose(t *testing.T) {
	store, cleanup := setupTestDB(t)
	defer cleanup()

	// Insert some data
	store.InsertReading(createTestReading("sensor-01", 22.0, 45.0, time.Now().UTC()))

	// Close the connection
	err := store.Close()
	if err != nil {
		t.Fatalf("Close failed: %v", err)
	}

	// Operations should fail after close
	_, err = store.GetLatestReading("sensor-01")
	if err == nil {
		t.Error("Expected error after Close, got nil")
	}
}

// BenchmarkInsertReading benchmarks single insert performance
func BenchmarkInsertReading(b *testing.B) {
	tmpDir, _ := os.MkdirTemp("", "dht-bench-*")
	defer os.RemoveAll(tmpDir)

	store, _ := NewSQLiteStore(filepath.Join(tmpDir, "bench.db"), testLogger())
	defer store.Close()

	reading := createTestReading("sensor-01", 22.5, 45.0, time.Now().UTC())

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		store.InsertReading(reading)
	}
}

// BenchmarkInsertBatch benchmarks batch insert performance
func BenchmarkInsertBatch(b *testing.B) {
	tmpDir, _ := os.MkdirTemp("", "dht-bench-*")
	defer os.RemoveAll(tmpDir)

	store, _ := NewSQLiteStore(filepath.Join(tmpDir, "bench.db"), testLogger())
	defer store.Close()

	// Create batch of 100 readings
	now := time.Now().UTC()
	readings := make([]*models.Reading, 100)
	for i := 0; i < 100; i++ {
		readings[i] = createTestReading("sensor-01", 22.5, 45.0, now.Add(time.Duration(i)*time.Second))
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		store.InsertBatch(readings)
	}
}

// BenchmarkGetReadingsInRange benchmarks range queries
func BenchmarkGetReadingsInRange(b *testing.B) {
	tmpDir, _ := os.MkdirTemp("", "dht-bench-*")
	defer os.RemoveAll(tmpDir)

	store, _ := NewSQLiteStore(filepath.Join(tmpDir, "bench.db"), testLogger())
	defer store.Close()

	// Pre-populate with data
	now := time.Now().UTC()
	readings := make([]*models.Reading, 10000)
	for i := 0; i < 10000; i++ {
		readings[i] = createTestReading("sensor-01", 22.5, 45.0, now.Add(-time.Duration(i)*time.Minute))
	}
	store.InsertBatch(readings)

	start := now.Add(-6 * time.Hour)
	end := now

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		store.GetReadingsInRange("sensor-01", start, end, 500)
	}
}
