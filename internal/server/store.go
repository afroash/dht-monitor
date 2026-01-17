package server

import (
	"sync"
	"time"

	"github.com/afroash/dht-monitor/internal/models"
)

// MemoryStore is an in-memory ring buffer for sensor readings
type MemoryStore struct {
	capacity      int
	data          map[string][]*models.Reading
	mutex         sync.RWMutex
	totalReadings int64
}

// NewMemoryStore creates a new in-memory store
func NewMemoryStore(capacity int) *MemoryStore {

	return &MemoryStore{
		capacity:      capacity,
		data:          make(map[string][]*models.Reading),
		mutex:         sync.RWMutex{},
		totalReadings: 0,
	}
}

// Add adds a reading to the store
func (ms *MemoryStore) Add(reading *models.Reading) {

	ms.mutex.Lock()
	defer ms.mutex.Unlock()

	readings := ms.data[reading.SensorID]
	if len(readings) >= ms.capacity {
		readings = readings[1:] // Remove oldest
	}
	readings = append(readings, reading)
	ms.data[reading.SensorID] = readings
	ms.totalReadings++
}

// GetLatest returns the n most recent readings for a sensor
func (ms *MemoryStore) GetLatest(sensorID string, n int) []*models.Reading {
	ms.mutex.RLock()
	defer ms.mutex.RUnlock()

	readings := ms.data[sensorID]
	if len(readings) == 0 {
		return nil
	}

	start := len(readings) - n
	if start < 0 {
		start = 0
	}

	// Return copies, newest first
	result := make([]*models.Reading, len(readings)-start)
	for i, j := len(readings)-1, 0; i >= start; i, j = i-1, j+1 {
		result[j] = readings[i].Copy()
	}
	return result
}

// GetAll returns all readings from all sensors
func (ms *MemoryStore) GetAll() []*models.Reading {
	ms.mutex.RLock()
	defer ms.mutex.RUnlock()

	// Collect copies of all readings from all sensors
	result := make([]*models.Reading, 0)
	for _, sensorReadings := range ms.data {
		for _, reading := range sensorReadings {
			result = append(result, reading.Copy())
		}
	}
	return result
}

// GetCurrentReading returns the most recent reading for a sensor
func (ms *MemoryStore) GetCurrentReading(sensorID string) *models.Reading {
	ms.mutex.RLock()
	defer ms.mutex.RUnlock()

	readings := ms.data[sensorID]
	if len(readings) == 0 {
		return nil
	}
	// Return a copy, not a pointer to internal data
	return readings[len(readings)-1].Copy()
}

// GetSensorIDs returns list of all sensor IDs that have sent data
func (ms *MemoryStore) GetSensorIDs() []string {
	// TODO: Implement with RLock
	// - Return slice of keys from ms.data map

	ms.mutex.RLock()
	defer ms.mutex.RUnlock()

	keys := make([]string, 0, len(ms.data))
	for key := range ms.data {
		keys = append(keys, key)
	}
	return keys
}

// Stats returns statistics about the store
func (ms *MemoryStore) Stats() StoreStats {
	// TODO: Implement with RLock
	// - Count total readings across all sensors
	// - Count unique sensors
	// - Return stats

	ms.mutex.RLock()
	defer ms.mutex.RUnlock()

	totalReadings := ms.totalReadings
	uniqueSensors := len(ms.data)
	currentReadings := 0
	for _, readings := range ms.data {
		currentReadings += len(readings)
	}
	return StoreStats{
		TotalReadings:   totalReadings,
		UniqueSensors:   uniqueSensors,
		CurrentReadings: currentReadings,
	}

}

// StoreStats contains statistics about the memory store
type StoreStats struct {
	TotalReadings   int64     `json:"total_readings"`
	UniqueSensors   int       `json:"unique_sensors"`
	CurrentReadings int       `json:"current_readings"` // In memory now
	OldestReading   time.Time `json:"oldest_reading,omitempty"`
	NewestReading   time.Time `json:"newest_reading,omitempty"`
}

// Clear removes all data from the store
func (ms *MemoryStore) Clear() {
	ms.mutex.Lock()
	defer ms.mutex.Unlock()

	ms.data = make(map[string][]*models.Reading)
	ms.totalReadings = 0
}
