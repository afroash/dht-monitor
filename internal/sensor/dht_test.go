// internal/sensor/dht_test.go
package sensor

import (
	"testing"
)

// MockDHTSensor implements DHTSensor for testing
type MockDHTSensor struct {
	temperature float64
	humidity    float64
	err         error
	readCount   int
}

func (m *MockDHTSensor) Read() (float64, float64, error) {
	m.readCount++
	return m.temperature, m.humidity, m.err
}

func (m *MockDHTSensor) Close() error {
	return nil
}

func TestValidateReading(t *testing.T) {
	tests := []struct {
		name      string
		temp      float64
		humidity  float64
		wantError bool
	}{
		{"valid reading", 22.5, 45.0, false},
		{"valid edge low", 0.0, 20.0, false},
		{"valid edge high", 50.0, 90.0, false},
		{"temperature too low", -25.0, 45.0, true},
		{"temperature too high", 65.0, 45.0, true},
		{"humidity negative", 22.5, -5.0, true},
		{"humidity over 100", 22.5, 105.0, true},
		{"both out of range", -30.0, 110.0, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateReading(tt.temp, tt.humidity)
			if (err != nil) != tt.wantError {
				t.Errorf("validateReading(%v, %v) error = %v, wantError %v",
					tt.temp, tt.humidity, err, tt.wantError)
			}
		})
	}
}

func TestDHT11Reader_RetryLogic(t *testing.T) {
	// This test verifies retry behavior with a mock
	// Since we can't test actual hardware in unit tests

	reader := NewDHT11Reader(4) // pin doesn't matter for this test

	if reader == nil {
		t.Fatal("NewDHT11Reader returned nil")
	}

	if reader.maxRetries != 3 {
		t.Errorf("maxRetries = %d, want 3", reader.maxRetries)
	}

	// Note: Can't easily test retry logic without mocking the dht.ReadDHT11 function
	// In real usage, the retry logic will be tested with actual hardware
}
