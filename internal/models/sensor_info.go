package models

import "time"

// SensorInfo contains metadata about the sensor device
type SensorInfo struct {
	// TODO: Add fields:
	// - ID (string) - unique sensor identifier
	// - Location (string) - human-readable location description
	// - SensorType (string) - e.g., "DHT11"
	// - Version (string) - client software version
	// - StartTime (time.Time) - when the sensor client started
	// Add JSON tags
	ID         string    `json:"id"`
	Location   string    `json:"location"`
	SensorType string    `json:"sensor_type"`
	Version    string    `json:"version"`
	StartTime  time.Time `json:"start_time"`
}

// Uptime returns the duration since the sensor started
func (s *SensorInfo) Uptime() time.Duration {
	// TODO: Calculate time.Since(s.StartTime)
	return time.Since(s.StartTime)
}

// NewSensorInfo creates a new SensorInfo with the current time as start time
func NewSensorInfo(id, location, sensorType, version string) *SensorInfo {
	// TODO: Create and return SensorInfo with time.Now() as StartTime
	return &SensorInfo{
		ID:         id,
		Location:   location,
		SensorType: sensorType,
		Version:    version,
		StartTime:  time.Now(),
	}
}
