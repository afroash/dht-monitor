package config

import (
	"fmt"
	"os"
	"time"

	"gopkg.in/yaml.v3"
)

// Config holds all configuration for the sensor client
type Config struct {
	Sensor  SensorConfig  `yaml:"sensor"`
	Server  ServerConfig  `yaml:"server"`
	Buffer  BufferConfig  `yaml:"buffer"`
	Logging LoggingConfig `yaml:"logging"`
}

// SensorConfig contains sensor-specific settings
type SensorConfig struct {
	// TODO: Add fields:
	// - ID (string) - unique sensor identifier
	// - Location (string) - where the sensor is located
	// - Type (string) - sensor type (e.g., "DHT11")
	// - GPIOPin (int) - GPIO pin number
	// - ReadInterval (time.Duration) - how often to read (default 30s)
	ID           string        `yaml:"id"`
	Location     string        `yaml:"location"`
	Type         string        `yaml:"type"`
	GPIOPin      int           `yaml:"gpio_pin"`
	ReadInterval time.Duration `yaml:"read_interval"`
}

// ServerConfig contains connection settings for the remote server
type ServerConfig struct {
	// TODO: Add fields:
	// - URL (string) - WebSocket URL (e.g., "wss://example.com/sensor-stream")
	// - AuthToken (string) - authentication token
	// - ConnectTimeout (time.Duration) - connection timeout
	// - ReconnectInterval (time.Duration) - initial reconnect delay
	// - MaxReconnectInterval (time.Duration) - max reconnect delay
	// - PingInterval (time.Duration) - heartbeat/ping interval
	// - PongTimeout (time.Duration) - pong response timeout
	URL                  string        `yaml:"url"`
	AuthToken            string        `yaml:"auth_token"`
	ConnectTimeout       time.Duration `yaml:"connect_timeout"`
	ReconnectInterval    time.Duration `yaml:"reconnect_interval"`
	MaxReconnectInterval time.Duration `yaml:"max_reconnect_interval"`
	PingInterval         time.Duration `yaml:"ping_interval"`
	PongTimeout          time.Duration `yaml:"pong_timeout"`
}

// BufferConfig contains settings for the reading buffer
type BufferConfig struct {
	// TODO: Add fields:
	// - Size (int) - max number of readings to buffer (default 1000)
	// - DropOldest (bool) - drop oldest when full (vs newest)
	// - PersistToDisk (bool) - write to disk when full (future feature)
	// - PersistPath (string) - path for disk persistence
	Size          int    `yaml:"size"`
	DropOldest    bool   `yaml:"drop_oldest"`
	PersistToDisk bool   `yaml:"persist_to_disk"`
	PersistPath   string `yaml:"persist_path"`
}

// LoggingConfig contains logging settings
type LoggingConfig struct {
	// TODO: Add fields:
	// - Level (string) - log level: "debug", "info", "warn", "error"
	// - Format (string) - "json" or "text"
	// - FilePath (string) - log file path (empty = stdout only)
	// - MaxSizeMB (int) - max log file size before rotation
	// - MaxBackups (int) - number of old log files to keep
	Level      string `yaml:"level"`
	Format     string `yaml:"format"`
	FilePath   string `yaml:"file_path"`
	MaxSizeMB  int    `yaml:"max_size_mb"`
	MaxBackups int    `yaml:"max_backups"`
}

// LoadConfig loads configuration from a YAML file
func LoadConfig(path string) (*Config, error) {
	// TODO: Implement
	// - Read file from path
	yamlData, err := os.ReadFile(path)
	if err != nil {
		fmt.Println("Error reading config file:", err)
		return nil, err
	}
	var config Config
	err = yaml.Unmarshal(yamlData, &config)
	if err != nil {
		fmt.Println("Error unmarshalling config file:", err)
		return nil, err
	}

	config.ApplyDefaults()
	config.OverrideFromEnv()
	if err := config.Validate(); err != nil {
		fmt.Println("Error validating config:", err)
		return nil, err
	}
	return &config, nil

}

// ApplyDefaults sets default values for any unset fields
func (c *Config) ApplyDefaults() {
	// Only set defaults if fields are zero values
	if c.Sensor.Type == "" {
		c.Sensor.Type = "DHT11"
	}
	if c.Sensor.ReadInterval == 0 {
		c.Sensor.ReadInterval = 30 * time.Second
	}
	if c.Server.ConnectTimeout == 0 {
		c.Server.ConnectTimeout = 10 * time.Second
	}
	if c.Server.ReconnectInterval == 0 {
		c.Server.ReconnectInterval = 1 * time.Second
	}
	if c.Server.MaxReconnectInterval == 0 {
		c.Server.MaxReconnectInterval = 5 * time.Minute
	}
	if c.Server.PingInterval == 0 {
		c.Server.PingInterval = 30 * time.Second
	}
	if c.Server.PongTimeout == 0 {
		c.Server.PongTimeout = 10 * time.Second
	}
	if c.Buffer.Size == 0 {
		c.Buffer.Size = 1000
		c.Buffer.DropOldest = true
	}
	if c.Logging.Level == "" {
		c.Logging.Level = "info"
	}
	if c.Logging.Format == "" {
		c.Logging.Format = "json"
	}
	if c.Logging.MaxSizeMB == 0 {
		c.Logging.MaxSizeMB = 100
	}
	if c.Logging.MaxBackups == 0 {
		c.Logging.MaxBackups = 10
	}
	// Sensor defaults:
	//   - Type: "DHT11"
	//   - ReadInterval: 30 * time.Second
	// Server defaults:
	//   - ConnectTimeout: 10 * time.Second
	//   - ReconnectInterval: 1 * time.Second
	//   - MaxReconnectInterval: 5 * time.Minute
	//   - PingInterval: 30 * time.Second
	//   - PongTimeout: 10 * time.Second
	// Buffer defaults:
	//   - Size: 1000
	//   - DropOldest: true
	// Logging defaults:
	//   - Level: "info"
	//   - Format: "json"
}

// OverrideFromEnv overrides config values from environment variables
func (c *Config) OverrideFromEnv() {
	// Only override if environment variable is set (non-empty)
	if v := os.Getenv("SENSOR_ID"); v != "" {
		c.Sensor.ID = v
	}
	if v := os.Getenv("SENSOR_LOCATION"); v != "" {
		c.Sensor.Location = v
	}
	if v := os.Getenv("SERVER_URL"); v != "" {
		c.Server.URL = v
	}
	if v := os.Getenv("SERVER_AUTH_TOKEN"); v != "" {
		c.Server.AuthToken = v
	}
	if v := os.Getenv("LOG_LEVEL"); v != "" {
		c.Logging.Level = v
	}
}

// Validate checks if the configuration is valid
func (c *Config) Validate() error {
	// TODO: Validate required fields and ranges
	// Required:
	//   - Sensor.ID (non-empty)
	//   - Sensor.GPIOPin (> 0)
	//   - Server.URL (non-empty, starts with ws:// or wss://)
	//   - Server.AuthToken (non-empty)
	// Ranges:
	//   - Sensor.ReadInterval (>= 1s)
	//   - Buffer.Size (>= 10, <= 100000)
	//   - Server.ReconnectInterval (>= 1s)
	// Return error with descriptive message if invalid
	if c.Sensor.ID == "" {
		return fmt.Errorf("sensor ID is required")
	}
	if c.Sensor.GPIOPin <= 0 {
		return fmt.Errorf("GPIO pin must be greater than 0")
	}
	if c.Server.URL == "" {
		return fmt.Errorf("server URL is required")
	}
	if c.Server.AuthToken == "" {
		return fmt.Errorf("server auth token is required")
	}
	if c.Sensor.ReadInterval < 1*time.Second {
		return fmt.Errorf("read interval must be at least 1 second")
	}
	if c.Buffer.Size < 10 || c.Buffer.Size > 100000 {
		return fmt.Errorf("buffer size must be between 10 and 100000")
	}
	return nil
}

// String returns a safe string representation (hides auth token)
func (c *Config) String() string {
	// TODO: Return formatted string with config values
	// IMPORTANT: Mask Server.AuthToken (show only first 4 chars + "...")
	return fmt.Sprintf("Config{Sensor: %+v, Server: [URL=%s, Token=%s...], Buffer: %+v, Logging: %+v}",
		c.Sensor,
		c.Server.URL,
		maskToken(c.Server.AuthToken),
		c.Buffer,
		c.Logging,
	)
}

// maskToken masks all but first 4 characters of a token
func maskToken(token string) string {
	if len(token) <= 4 {
		return "****"
	}
	return token[:4] + "****"
}
