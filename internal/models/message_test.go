// internal/models/message_test.go
package models

import (
	"encoding/json"
	"testing"
	"time"
)

func TestNewMessage(t *testing.T) {
	reading := Reading{
		SensorID:    "sensor-01",
		Temperature: 22.5,
		Humidity:    45.0,
		Timestamp:   time.Now(),
	}

	msg, err := NewMessage(MessageTypeReading, reading)
	if err != nil {
		t.Fatalf("NewMessage failed: %v", err)
	}

	if msg.Type != MessageTypeReading {
		t.Errorf("Type = %v, want %v", msg.Type, MessageTypeReading)
	}

	if msg.Timestamp.IsZero() {
		t.Error("Timestamp should not be zero")
	}

	if len(msg.Payload) == 0 {
		t.Error("Payload should not be empty")
	}
}

func TestMessage_UnmarshalPayload(t *testing.T) {
	original := Reading{
		SensorID:    "sensor-01",
		Temperature: 22.5,
		Humidity:    45.0,
		Timestamp:   time.Date(2024, 1, 1, 12, 0, 0, 0, time.UTC),
	}

	msg, err := NewMessage(MessageTypeReading, original)
	if err != nil {
		t.Fatalf("NewMessage failed: %v", err)
	}

	var decoded Reading
	err = msg.UnmarshalPayload(&decoded)
	if err != nil {
		t.Fatalf("UnmarshalPayload failed: %v", err)
	}

	if decoded.SensorID != original.SensorID {
		t.Errorf("SensorID mismatch")
	}
	if decoded.Temperature != original.Temperature {
		t.Errorf("Temperature mismatch")
	}
}

func TestBatchMessage(t *testing.T) {
	readings := []Reading{
		{SensorID: "sensor-01", Temperature: 22.5, Humidity: 45.0, Timestamp: time.Now()},
		{SensorID: "sensor-01", Temperature: 23.0, Humidity: 46.0, Timestamp: time.Now()},
	}

	batch := BatchMessage{
		Readings: readings,
		Count:    len(readings),
	}

	msg, err := NewMessage(MessageTypeBatch, batch)
	if err != nil {
		t.Fatalf("NewMessage failed: %v", err)
	}

	var decoded BatchMessage
	err = msg.UnmarshalPayload(&decoded)
	if err != nil {
		t.Fatalf("UnmarshalPayload failed: %v", err)
	}

	if decoded.Count != 2 {
		t.Errorf("Count = %d, want 2", decoded.Count)
	}
	if len(decoded.Readings) != 2 {
		t.Errorf("len(Readings) = %d, want 2", len(decoded.Readings))
	}
}

func TestMessage_JSONRoundtrip(t *testing.T) {
	reading := Reading{
		SensorID:    "sensor-01",
		Temperature: 22.5,
		Humidity:    45.0,
		Timestamp:   time.Date(2024, 1, 1, 12, 0, 0, 0, time.UTC),
	}

	msg, err := NewMessage(MessageTypeReading, reading)
	if err != nil {
		t.Fatalf("NewMessage failed: %v", err)
	}

	// Marshal to JSON
	data, err := json.Marshal(msg)
	if err != nil {
		t.Fatalf("Marshal failed: %v", err)
	}

	// Unmarshal back
	var decoded Message
	err = json.Unmarshal(data, &decoded)
	if err != nil {
		t.Fatalf("Unmarshal failed: %v", err)
	}

	if decoded.Type != msg.Type {
		t.Errorf("Type mismatch")
	}

	// Verify payload
	var decodedReading Reading
	err = decoded.UnmarshalPayload(&decodedReading)
	if err != nil {
		t.Fatalf("UnmarshalPayload failed: %v", err)
	}

	if decodedReading.SensorID != reading.SensorID {
		t.Error("Payload mismatch")
	}
}
