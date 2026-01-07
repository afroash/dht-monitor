package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"github.com/afroash/dht-monitor/internal/client"
	"github.com/afroash/dht-monitor/internal/config"
	"github.com/afroash/dht-monitor/internal/models"
	"github.com/afroash/dht-monitor/internal/sensor"
	"github.com/rs/zerolog"
)

const version = "v0.1.0"

func main() {
	// Parse command line flags
	configPath := flag.String("config", "configs/sensor.local.yaml", "path to config file")
	testMode := flag.Bool("test", false, "run in test mode (no server connection)")
	flag.Parse()

	// Load configuration
	cfg, err := config.LoadConfig(*configPath)
	if err != nil {
		log.Fatalf("Failed to load config: %v", err)
	}
	fmt.Println(cfg)

	// Setup logging
	var logger zerolog.Logger
	if cfg.Logging.FilePath != "" {
		file, err := os.OpenFile(cfg.Logging.FilePath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
		if err != nil {
			log.Fatalf("Failed to open log file: %v", err)
		}
		logger = zerolog.New(file).With().Timestamp().Logger()
	} else {
		logger = zerolog.New(os.Stdout).With().Timestamp().Logger()
	}
	logger = logger.Level(parseLogLevel(cfg.Logging.Level))

	// Log startup banner
	logger.Info().
		Str("version", version).
		Str("sensor_id", cfg.Sensor.ID).
		Str("location", cfg.Sensor.Location).
		Bool("test_mode", *testMode).
		Msg("Starting sensor client")

	// Initialize sensor reader
	dhtSensor := sensor.NewDHT11Reader(cfg.Sensor.GPIOPin)
	defer dhtSensor.Close()
	sensorInfo := models.NewSensorInfo(
		cfg.Sensor.ID,
		cfg.Sensor.Location,
		cfg.Sensor.Type,
		version,
	)
	reader := sensor.NewReader(
		dhtSensor,
		sensorInfo,
		cfg.Sensor.ReadInterval,
		logger,
	)
	defer reader.Close()

	// Initialize buffer
	buffer := client.NewReadingBuffer(cfg.Buffer.Size, cfg.Buffer.DropOldest)
	logger.Info().Int("buffer_capacity", buffer.Capacity()).Bool("drop_oldest", cfg.Buffer.DropOldest).Msg("Initialized buffer")

	// Initialize connection (if not test mode)
	connConfig := client.ConnectionConfig{
		URL:                  cfg.Server.URL,
		AuthToken:            cfg.Server.AuthToken,
		ReconnectInterval:    cfg.Server.ReconnectInterval,
		MaxReconnectInterval: cfg.Server.MaxReconnectInterval,
		PingInterval:         cfg.Server.PingInterval,
		PongTimeout:          cfg.Server.PongTimeout,
	}
	connection := client.NewConnection(connConfig, sensorInfo, logger)
	defer connection.Close()

	// Setup graceful shutdown
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		sig := <-sigChan
		logger.Info().Str("signal", sig.String()).Msg("Received shutdown signal")
		cancel()
	}()

	// Start the application
	if err := run(ctx, cfg, reader, buffer, connection, logger, *testMode); err != nil && err != context.Canceled {
		logger.Error().Err(err).Msg("Application error")
		os.Exit(1)
	}
	logger.Info().Msg("Application shutdown complete")
}

// parseLogLevel converts string to zerolog.Level
func parseLogLevel(level string) zerolog.Level {
	switch level {
	case "debug":
		return zerolog.DebugLevel
	case "info":
		return zerolog.InfoLevel
	case "warn":
		return zerolog.WarnLevel
	case "error":
		return zerolog.ErrorLevel
	default:
		return zerolog.InfoLevel
	}
}

// run is the main application coordination loop
func run(
	ctx context.Context,
	cfg *config.Config,
	reader *sensor.Reader,
	buffer *client.ReadingBuffer,
	conn *client.Connection,
	logger zerolog.Logger,
	testMode bool,
) error {
	var wg sync.WaitGroup
	errChan := make(chan error, 3)

	// 1. Start sensor reading goroutine
	wg.Add(1)
	go func() {
		defer wg.Done()
		logger.Info().Msg("Starting sensor reader...")
		if err := reader.Start(ctx); err != nil && err != context.Canceled {
			logger.Error().Err(err).Msg("Sensor reader error")
			errChan <- err
		}
	}()

	// 2. Start connection manager (if not test mode)
	if !testMode && conn != nil {
		wg.Add(1)
		go func() {
			defer wg.Done()
			logger.Info().Msg("Starting connection manager...")
			if err := conn.Run(ctx); err != nil && err != context.Canceled {
				logger.Error().Err(err).Msg("Connection error")
				errChan <- err
			}
		}()
	}

	// 3. Start data coordinator
	wg.Add(1)
	go func() {
		defer wg.Done()
		logger.Info().Msg("Starting data coordinator...")
		if err := coordinateData(ctx, reader, buffer, conn, logger, testMode); err != nil && err != context.Canceled {
			logger.Error().Err(err).Msg("Data coordinator error")
			errChan <- err
		}
	}()

	// Wait for all goroutines to finish, then close errChan
	go func() {
		wg.Wait()
		close(errChan)
	}()

	// Return first error or nil when all done
	for err := range errChan {
		if err != nil {
			return err
		}
	}
	return nil
}

// coordinateData handles the flow of readings from sensor to server
func coordinateData(
	ctx context.Context,
	reader *sensor.Reader,
	buffer *client.ReadingBuffer,
	conn *client.Connection,
	logger zerolog.Logger,
	testMode bool,
) error {
	statusTicker := time.NewTicker(5 * time.Minute)
	defer statusTicker.Stop()

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()

		case reading, ok := <-reader.Readings():
			if !ok {
				logger.Info().Msg("Readings channel closed")
				return nil
			}
			handleNewReading(reading, buffer, conn, logger, testMode)

		case <-statusTicker.C:
			logStatus(buffer, conn, logger)

			// If connected and buffer has data, try to drain it
			if !testMode && conn != nil && conn.IsConnected() && buffer.Size() > 0 {
				drainBuffer(buffer, conn, logger)
			}
		}
	}
}

// handleNewReading processes a single new reading
func handleNewReading(
	reading *models.Reading,
	buffer *client.ReadingBuffer,
	conn *client.Connection,
	logger zerolog.Logger,
	testMode bool,
) {
	// Offline mode - buffer everything
	if testMode || conn == nil || !conn.IsConnected() {
		if !buffer.Push(reading) {
			logger.Warn().Msg("Buffer full, reading dropped")
		} else {
			logger.Debug().
				Float64("temp", reading.Temperature).
				Float64("humidity", reading.Humidity).
				Int("buffer_size", buffer.Size()).
				Msg("🧺 Reading buffered (offline)")
		}
		return
	}

	// Online mode
	if buffer.IsEmpty() {
		// Try to send immediately
		if err := conn.Send(reading); err != nil {
			logger.Warn().Err(err).Msg("Failed to send reading, buffering")
			buffer.Push(reading)
		} else {
			logger.Info().
				Float64("temp", reading.Temperature).
				Float64("humidity", reading.Humidity).
				Msg("Reading sent")
		}
	} else {
		// Buffer has data, add this reading and drain
		buffer.Push(reading)
		drainBuffer(buffer, conn, logger)
	}
}

// drainBuffer sends all buffered readings to the server
func drainBuffer(buffer *client.ReadingBuffer, conn *client.Connection, logger zerolog.Logger) {
	if !conn.IsConnected() {
		return
	}

	const batchSize = 50

	for buffer.Size() > 0 {
		readings := buffer.PopBatch(batchSize)
		if len(readings) == 0 {
			break
		}

		if err := conn.SendBatch(readings); err != nil {
			logger.Warn().
				Err(err).
				Int("count", len(readings)).
				Msg("Failed to send batch, stopping drain")
			// Readings are lost from buffer - in production you might want
			// a more sophisticated retry mechanism
			break
		}

		logger.Info().
			Int("count", len(readings)).
			Int("remaining", buffer.Size()).
			Msg("Batch sent")

		// Small delay between batches to not overwhelm server
		time.Sleep(100 * time.Millisecond)
	}

	if buffer.IsEmpty() {
		logger.Info().Msg("Buffer drained successfully")
	}
}

// logStatus logs current application status
func logStatus(buffer *client.ReadingBuffer, conn *client.Connection, logger zerolog.Logger) {
	stats := buffer.Stats()

	connState := "test-mode"
	if conn != nil {
		connState = conn.State().String()
	}

	logEvent := logger.Info().
		Str("connection", connState).
		Int("buffer_size", buffer.Size()).
		Int("buffer_capacity", buffer.Capacity()).
		Int64("total_readings", stats.TotalPushed).
		Int64("dropped", stats.TotalDropped).
		Int("high_water", stats.HighWaterMark)

	if !stats.LastPushTime.IsZero() {
		logEvent = logEvent.Time("last_reading", stats.LastPushTime)
	}

	logEvent.Msg("Status update")
}
