package sensor

import (
	"fmt"

	"github.com/afroash/dht"
)

// DHTSensor defines the interface for reading from a DHT sensor
type DHTSensor interface {
	// Read performs a single reading from the sensor
	// Returns temperature (°C), humidity (%), and any error
	Read() (temperature float64, humidity float64, err error)

	// Close cleans up GPIO resources
	Close() error
}

// DHT11Reader implements DHTSensor for DHT11 hardware
type DHT11Reader struct {
	pin        int
	maxRetries int
	sensor     *dht.Sensor
}

// NewDHT11Reader creates a new DHT11 sensor reader
func NewDHT11Reader(pin int) *DHT11Reader {
	// TODO: Implement
	// - Create DHT11Reader with given pin
	// - Set maxRetries to 3
	// - Set retryDelay to 2 seconds (DHT11 needs time between reads
	sensor, err := dht.NewDHT11(pin)
	if err != nil {
		return nil
	}
	return &DHT11Reader{
		pin:        pin,
		maxRetries: 3,
		sensor:     sensor,
	}
}

// Read performs a reading from the DHT11 sensor with retry logic
func (d *DHT11Reader) Read() (float64, float64, error) {

	reading, err := d.sensor.ReadRetry(d.maxRetries)
	if err != nil {
		return 0, 0, fmt.Errorf("after %d retries, failed to read from sensor", d.maxRetries)
	}
	if err := validateReading(reading.Temperature, reading.Humidity); err != nil {
		return 0, 0, fmt.Errorf("invalid reading: %w", err)
	}

	return reading.Temperature, reading.Humidity, nil
}

// Close cleans up GPIO resources
func (d *DHT11Reader) Close() error {
	return d.sensor.Close()
}

// validateReading checks if temperature and humidity values are reasonable
func validateReading(temp, humidity float64) error {

	// Use relaxed bounds for sanity checking:
	const (
		minTemp     = -20.0
		maxTemp     = 60.0
		minHumidity = 0.0
		maxHumidity = 100.0
	)
	if temp < minTemp || temp > maxTemp {
		return fmt.Errorf("temperature %.1f°C is below minimum of -20°C", temp)
	}
	if humidity < minHumidity || humidity > maxHumidity {
		return fmt.Errorf("humidity %.1f%% is above maximum of 100%%", humidity)
	}
	return nil
}
