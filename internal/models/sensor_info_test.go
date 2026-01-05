// internal/models/sensor_info_test.go
package models

import (
	"encoding/json"
	"testing"
	"time"
)

func TestNewSensorInfo(t *testing.T) {
	id := "sensor-01"
	location := "Living Room"
	sensorType := "DHT11"
	version := "v1.0.0"

	info := NewSensorInfo(id, location, sensorType, version)

	if info == nil {
		t.Fatal("NewSensorInfo returned nil")
	}
	if info.ID != id {
		t.Errorf("ID = %v, want %v", info.ID, id)
	}
	if info.Location != location {
		t.Errorf("Location = %v, want %v", info.Location, location)
	}
	if info.SensorType != sensorType {
		t.Errorf("SensorType = %v, want %v", info.SensorType, sensorType)
	}
	if info.Version != version {
		t.Errorf("Version = %v, want %v", info.Version, version)
	}
	if info.StartTime.IsZero() {
		t.Error("StartTime should not be zero")
	}
}

func TestSensorInfo_Uptime(t *testing.T) {
	info := &SensorInfo{
		ID:         "sensor-01",
		Location:   "Test",
		SensorType: "DHT11",
		Version:    "v1.0.0",
		StartTime:  time.Now().Add(-1 * time.Hour),
	}

	uptime := info.Uptime()

	// Should be approximately 1 hour (within a second tolerance)
	if uptime < 59*time.Minute || uptime > 61*time.Minute {
		t.Errorf("Uptime = %v, expected approximately 1 hour", uptime)
	}
}

func TestSensorInfo_JSONSerialization(t *testing.T) {
	original := NewSensorInfo("sensor-01", "Living Room", "DHT11", "v1.0.0")

	data, err := json.Marshal(original)
	if err != nil {
		t.Fatalf("Marshal failed: %v", err)
	}

	var decoded SensorInfo
	err = json.Unmarshal(data, &decoded)
	if err != nil {
		t.Fatalf("Unmarshal failed: %v", err)
	}

	if decoded.ID != original.ID {
		t.Errorf("ID mismatch")
	}
	if decoded.Location != original.Location {
		t.Errorf("Location mismatch")
	}
}
