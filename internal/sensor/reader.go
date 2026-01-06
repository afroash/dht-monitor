package sensor

import (
	"context"
	"time"

	"github.com/afroash/dht-monitor/internal/models"
	"github.com/rs/zerolog"
)

// Reader orchestrates periodic sensor readings
type Reader struct {
	sensor     DHTSensor
	sensorInfo *models.SensorInfo
	interval   time.Duration
	logger     zerolog.Logger
	readings   chan *models.Reading
}

// NewReader creates a new sensor reader
func NewReader(sensor DHTSensor, info *models.SensorInfo, interval time.Duration, logger zerolog.Logger) *Reader {
	return &Reader{
		sensor:     sensor,
		sensorInfo: info,
		interval:   interval,
		logger:     logger,
		readings:   make(chan *models.Reading, 10),
	}
}

// Start begins periodic reading from the sensor
// Runs in a goroutine until context is cancelled
func (r *Reader) Start(ctx context.Context) error {
	ticker := time.NewTicker(r.interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
			r.readAndPublish()
		}
	}

}

// ReadOnce performs a single reading (useful for testing)
func (r *Reader) ReadOnce() (*models.Reading, error) {
	temperature, humidity, err := r.sensor.Read()
	if err != nil {
		return nil, err
	}
	return models.NewReading(r.sensorInfo.ID, temperature, humidity), nil
}

// readAndPublish performs a read and publishes to the channel
func (r *Reader) readAndPublish() {
	reading, err := r.ReadOnce()
	if err != nil {
		r.logger.Error().Err(err).Msg("failed to read from sensor")
		return
	}
	r.readings <- reading
	r.logger.Info().Msgf("read from sensor: %s", reading.String())
}

// Readings returns the channel where readings are published
func (r *Reader) Readings() <-chan *models.Reading {
	return r.readings
}

// Close stops the reader and cleans up resources
func (r *Reader) Close() error {
	return r.sensor.Close()
}
