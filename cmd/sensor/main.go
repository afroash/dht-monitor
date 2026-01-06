package main

import (
	"context"
	"flag"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/afroash/dht-monitor/internal/config"
	"github.com/afroash/dht-monitor/internal/models"
	"github.com/afroash/dht-monitor/internal/sensor"
	"github.com/rs/zerolog"
)

func main() {
	configPath := flag.String("config", "configs/sensor.local.yaml", "path to config file")
	flag.Parse()

	// Load config
	cfg, err := config.LoadConfig(*configPath)
	if err != nil {
		log.Fatalf("Failed to load config: %v", err)
	}

	// Setup logger
	logger := zerolog.New(os.Stdout).With().Timestamp().Logger()
	if cfg.Logging.Level == "debug" {
		logger = logger.Level(zerolog.DebugLevel)
	}

	logger.Info().Msg("Starting DHT sensor test...")

	// Create sensor
	dhtSensor := sensor.NewDHT11Reader(cfg.Sensor.GPIOPin)
	defer dhtSensor.Close()

	// Create sensor info
	sensorInfo := models.NewSensorInfo(
		cfg.Sensor.ID,
		cfg.Sensor.Location,
		cfg.Sensor.Type,
		"v0.1.0-test",
	)

	// Create reader
	reader := sensor.NewReader(
		dhtSensor,
		sensorInfo,
		cfg.Sensor.ReadInterval,
		logger,
	)
	defer reader.Close()

	// Test single read first
	logger.Info().Msg("Testing single read...")
	reading, err := reader.ReadOnce()
	if err != nil {
		logger.Error().Err(err).Msg("Single read failed")
	} else {
		logger.Info().
			Float64("temperature", reading.Temperature).
			Float64("humidity", reading.Humidity).
			Msg("Single read successful")
	}

	// Start continuous reading
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Handle shutdown gracefully
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	// Start reader in background
	go func() {
		if err := reader.Start(ctx); err != nil {
			logger.Error().Err(err).Msg("Reader stopped with error")
		}
	}()

	// Print readings as they arrive
	go func() {
		for reading := range reader.Readings() {
			logger.Info().
				Str("sensor_id", reading.SensorID).
				Float64("temperature", reading.Temperature).
				Float64("humidity", reading.Humidity).
				Time("timestamp", reading.Timestamp).
				Msg("📊 New reading")
		}
	}()

	logger.Info().
		Dur("interval", cfg.Sensor.ReadInterval).
		Msg("🌡️  Reading sensor continuously... Press Ctrl+C to stop")

	// Wait for shutdown signal
	<-sigChan
	logger.Info().Msg("Shutting down...")
	cancel()

	// Give goroutines time to clean up
	time.Sleep(1 * time.Second)
	logger.Info().Msg("Goodbye!")
}
