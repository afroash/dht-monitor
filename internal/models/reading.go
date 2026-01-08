package models

import (
	"fmt"
	"time"
)

// Reading represents a reading from a DHT sensor on our raspberry pi.
type Reading struct {
	SensorID    string    `json:"sensor_id"`
	Timestamp   time.Time `json:"timestamp"`
	Humidity    float64   `json:"humidity"`
	Temperature float64   `json:"temperature"`
}

// IsValid checks if the reading values are within acceptable ranges
// DHT11 ranges: temp -20 to 60°C, humidity 20-90%
func (r *Reading) IsValid() bool {
	const (
		minTemp     = -20.0
		maxTemp     = 60.0
		minHumidity = 0.0
		maxHumidity = 100.0
	)

	if r.SensorID == "" {
		return false
	}

	// Check timestamp
	if r.Timestamp.IsZero() {
		return false
	}

	// Check temperature range
	if r.Temperature < minTemp || r.Temperature > maxTemp {
		return false
	}

	// Check humidity range
	if r.Humidity < minHumidity || r.Humidity > maxHumidity {
		return false
	}

	return true
}

// get the reading as a string
func (r *Reading) String() string {

	return fmt.Sprintf("SensorID: %s, Timestamp: %s, Humidity: %.1f%%, Temperature: %.1f°C",
		r.SensorID,
		r.Timestamp.Format(time.RFC3339),
		r.Humidity,
		r.Temperature)
}

// NewReading creates a new Reading with the current timestamp
func NewReading(sensorID string, temperature, humidity float64) *Reading {
	return &Reading{
		SensorID:    sensorID,
		Timestamp:   time.Now(),
		Humidity:    humidity,
		Temperature: temperature,
	}
}

// Copy returns a deep copy of the Reading
func (r *Reading) Copy() *Reading {
	if r == nil {
		return nil
	}
	return &Reading{
		SensorID:    r.SensorID,
		Timestamp:   r.Timestamp,
		Humidity:    r.Humidity,
		Temperature: r.Temperature,
	}
}
