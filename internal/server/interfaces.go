package server

import (
	"time"

	"github.com/afroash/dht-monitor/internal/models"
	"github.com/afroash/dht-monitor/internal/storage"
)

// ReadingStore defines the interface for real-time reading storage
// MemoryStore implements this interface
type ReadingStore interface {
	// Add adds a reading to the store
	Add(reading *models.Reading)

	// GetLatest returns the n most recent readings for a sensor (newest first)
	GetLatest(sensorID string, n int) []*models.Reading

	// GetCurrentReading returns the most recent reading for a sensor
	GetCurrentReading(sensorID string) *models.Reading

	// GetSensorIDs returns list of all sensor IDs that have sent data
	GetSensorIDs() []string

	// Stats returns statistics about the store
	Stats() StoreStats

	// GetAll returns all readings from all sensors
	GetAll() []*models.Reading

	// Clear removes all data from the store
	Clear()
}

// HistoricalStore defines the interface for historical/persistent storage
// storage.SQLiteStore implements this interface
type HistoricalStore interface {
	// GetReadingsInRange returns readings within a time range
	GetReadingsInRange(sensorID string, start, end time.Time, limit int) ([]*models.Reading, error)

	// GetReadingsBefore returns readings before a timestamp (for scrolling back)
	GetReadingsBefore(sensorID string, before time.Time, limit int) ([]*models.Reading, error)

	// GetReadingsAfter returns readings after a timestamp (for scrolling forward)
	GetReadingsAfter(sensorID string, after time.Time, limit int) ([]*models.Reading, error)

	// GetLatestReading returns the most recent reading for a sensor
	GetLatestReading(sensorID string) (*models.Reading, error)

	// GetSensorIDs returns list of all unique sensor IDs
	GetSensorIDs() ([]string, error)

	// GetDailyStats returns aggregated daily statistics
	GetDailyStats(sensorID string, start, end time.Time) ([]storage.DailyStat, error)

	// GetStorageStats returns database statistics
	GetStorageStats() (*storage.StorageStats, error)
}
