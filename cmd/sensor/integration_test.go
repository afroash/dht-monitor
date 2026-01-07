//go:build integration
// +build integration

package main

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/afroash/dht-monitor/internal/client"
	"github.com/afroash/dht-monitor/internal/config"
	"github.com/afroash/dht-monitor/internal/models"
	"github.com/afroash/dht-monitor/internal/sensor"
	"github.com/rs/zerolog"
)

// TestFullSystem tests the entire system integration
// Run with: go test -tags=integration -v ./cmd/sensor/
func TestFullSystem(t *testing.T) {
	// Load config
	cfg, err := config.LoadConfig("../../configs/sensor.local.yaml")
	if err != nil {
		t.Fatalf("Failed to load config: %v", err)
	}

	logger := zerolog.New(os.Stdout).With().Timestamp().Logger()

	// Create components
	dhtSensor := sensor.NewDHT11Reader(cfg.Sensor.GPIOPin)
	defer dhtSensor.Close()

	sensorInfo := models.NewSensorInfo(
		cfg.Sensor.ID,
		cfg.Sensor.Location,
		cfg.Sensor.Type,
		version,
	)

	reader := sensor.NewReader(dhtSensor, sensorInfo, cfg.Sensor.ReadInterval, logger)
	defer reader.Close()

	buffer := client.NewReadingBuffer(cfg.Buffer.Size, cfg.Buffer.DropOldest)

	// Run in test mode (no connection)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	// Start system
	err = run(ctx, cfg, reader, buffer, nil, logger, true)
	if err != nil && err != context.DeadlineExceeded {
		t.Fatalf("Run failed: %v", err)
	}

	// Verify we got readings
	if buffer.Size() == 0 {
		t.Error("No readings collected")
	}

	t.Logf("System test passed: %d readings collected", buffer.Size())
}
