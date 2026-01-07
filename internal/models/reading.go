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
	if r.SensorID == "" {
		return false
	}
	if r.Timestamp.IsZero() {
		return false
	}
	if r.Humidity < 20 || r.Humidity > 90 {
		return false
	}
	if r.Temperature < -20 || r.Temperature > 60 {
		return false
	}
	return true
}

// get the reading as a string
func (r *Reading) String() string {
	// TODO: Format as "SensorID: temp=XX.X°C humidity=XX.X% at YYYY-MM-DD HH:MM:SS"
	return fmt.Sprintf("SensorID: %s, Timestamp: %s, Humidity: %.1f%%, Temperature: %.1f°C",
		r.SensorID,
		r.Timestamp.Format(time.RFC3339),
		r.Humidity,
		r.Temperature)
}

// NewReading creates a new Reading with the current timestamp
func NewReading(sensorID string, temperature, humidity float64) *Reading {
	// TODO: Create and return a Reading with provided values and time.Now()
	return &Reading{
		SensorID:    sensorID,
		Timestamp:   time.Now(),
		Humidity:    humidity,
		Temperature: temperature,
	}
}
