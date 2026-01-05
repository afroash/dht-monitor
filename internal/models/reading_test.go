// internal/models/reading_test.go
package models

import (
	"encoding/json"
	"testing"
	"time"
)

func TestReading_IsValid(t *testing.T) {
	tests := []struct {
		name     string
		reading  Reading
		expected bool
	}{
		{
			name: "valid reading",
			reading: Reading{
				SensorID:    "sensor-01",
				Temperature: 22.5,
				Humidity:    45.0,
				Timestamp:   time.Now(),
			},
			expected: true,
		},
		{
			name: "temperature too low",
			reading: Reading{
				SensorID:    "sensor-01",
				Temperature: -25.0,
				Humidity:    45.0,
				Timestamp:   time.Now(),
			},
			expected: false,
		},
		{
			name: "temperature too high",
			reading: Reading{
				SensorID:    "sensor-01",
				Temperature: 65.0,
				Humidity:    45.0,
				Timestamp:   time.Now(),
			},
			expected: false,
		},
		{
			name: "humidity too low",
			reading: Reading{
				SensorID:    "sensor-01",
				Temperature: 22.5,
				Humidity:    15.0,
				Timestamp:   time.Now(),
			},
			expected: false,
		},
		{
			name: "humidity too high",
			reading: Reading{
				SensorID:    "sensor-01",
				Temperature: 22.5,
				Humidity:    95.0,
				Timestamp:   time.Now(),
			},
			expected: false,
		},
		{
			name: "zero timestamp",
			reading: Reading{
				SensorID:    "sensor-01",
				Temperature: 22.5,
				Humidity:    45.0,
				Timestamp:   time.Time{},
			},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.reading.IsValid()
			if result != tt.expected {
				t.Errorf("IsValid() = %v, expected %v", result, tt.expected)
			}
		})
	}
}

func TestReading_JSONSerialization(t *testing.T) {
	original := Reading{
		SensorID:    "sensor-01",
		Temperature: 22.5,
		Humidity:    45.0,
		Timestamp:   time.Date(2024, 1, 1, 12, 0, 0, 0, time.UTC),
	}

	// Marshal to JSON
	data, err := json.Marshal(original)
	if err != nil {
		t.Fatalf("Failed to marshal: %v", err)
	}

	// Unmarshal back
	var decoded Reading
	err = json.Unmarshal(data, &decoded)
	if err != nil {
		t.Fatalf("Failed to unmarshal: %v", err)
	}

	// Compare
	if decoded.SensorID != original.SensorID {
		t.Errorf("SensorID mismatch: got %v, want %v", decoded.SensorID, original.SensorID)
	}
	if decoded.Temperature != original.Temperature {
		t.Errorf("Temperature mismatch: got %v, want %v", decoded.Temperature, original.Temperature)
	}
	if decoded.Humidity != original.Humidity {
		t.Errorf("Humidity mismatch: got %v, want %v", decoded.Humidity, original.Humidity)
	}
}

func TestNewReading(t *testing.T) {
	sensorID := "sensor-01"
	temp := 22.5
	humidity := 45.0

	reading := NewReading(sensorID, temp, humidity)

	if reading == nil {
		t.Fatal("NewReading returned nil")
	}
	if reading.SensorID != sensorID {
		t.Errorf("SensorID = %v, want %v", reading.SensorID, sensorID)
	}
	if reading.Temperature != temp {
		t.Errorf("Temperature = %v, want %v", reading.Temperature, temp)
	}
	if reading.Humidity != humidity {
		t.Errorf("Humidity = %v, want %v", reading.Humidity, humidity)
	}
	if reading.Timestamp.IsZero() {
		t.Error("Timestamp should not be zero")
	}
}
