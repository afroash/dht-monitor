// internal/sensor/reader_test.go
package sensor

import (
	"context"
	"testing"
	"time"

	"github.com/afroash/dht-monitor/internal/models"
	"github.com/rs/zerolog"
)

func TestReader_ReadOnce(t *testing.T) {
	mock := &MockDHTSensor{
		temperature: 22.5,
		humidity:    45.0,
	}

	info := models.NewSensorInfo("test-sensor", "Test Lab", "DHT11", "v1.0.0")
	logger := zerolog.Nop()
	reader := NewReader(mock, info, 30*time.Second, logger)

	reading, err := reader.ReadOnce()
	if err != nil {
		t.Fatalf("ReadOnce() failed: %v", err)
	}

	if reading == nil {
		t.Fatal("ReadOnce() returned nil reading")
	}

	if reading.Temperature != 22.5 {
		t.Errorf("Temperature = %v, want 22.5", reading.Temperature)
	}
	if reading.Humidity != 45.0 {
		t.Errorf("Humidity = %v, want 45.0", reading.Humidity)
	}
	if reading.SensorID != "test-sensor" {
		t.Errorf("SensorID = %v, want test-sensor", reading.SensorID)
	}
	if reading.Timestamp.IsZero() {
		t.Error("Timestamp should not be zero")
	}
}

func TestReader_Start(t *testing.T) {
	mock := &MockDHTSensor{
		temperature: 22.5,
		humidity:    45.0,
	}

	info := models.NewSensorInfo("test-sensor", "Test Lab", "DHT11", "v1.0.0")
	logger := zerolog.Nop()

	// Use short interval for testing
	reader := NewReader(mock, info, 100*time.Millisecond, logger)

	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()

	// Start reader
	go reader.Start(ctx)

	// Collect readings
	readings := []*models.Reading{}
	timeout := time.After(600 * time.Millisecond)

readLoop:
	for {
		select {
		case reading, ok := <-reader.Readings():
			if !ok {
				break readLoop
			}
			readings = append(readings, reading)
		case <-timeout:
			break readLoop
		}
	}

	// Should have gotten ~4-5 readings (500ms / 100ms)
	if len(readings) < 3 {
		t.Errorf("Got %d readings, expected at least 3", len(readings))
	}

	// Check that mock was called
	if mock.readCount < 3 {
		t.Errorf("Mock read count = %d, expected at least 3", mock.readCount)
	}
}
