package config

import (
	"fmt"
	"os"
	"strconv"
	"time"

	"gopkg.in/yaml.v3"
)

// ServerConfig holds server-specific configuration
type AppConfig struct {
	Server  ServerSettings  `yaml:"server"`
	Storage StorageSettings `yaml:"storage"`
	Logging LoggingConfig   `yaml:"logging"` // Reuse from Phase 1
}

// ServerSettings contains HTTP server configuration
type ServerSettings struct {
	// TODO: Add fields:
	// - Port (int)
	// - Host (string)
	// - AuthToken (string)
	// - ReadTimeout (time.Duration)
	// - WriteTimeout (time.Duration)
	Port           int           `yaml:"port"`
	Host           string        `yaml:"host"`
	AuthToken      string        `yaml:"auth_token"`
	ReadTimeout    time.Duration `yaml:"read_timeout"`
	WriteTimeout   time.Duration `yaml:"write_timeout"`
	AllowedOrigins []string      `yaml:"allowed_origins"`
}

// StorageSettings contains storage configuration
type StorageSettings struct {
	BufferSize    int    `yaml:"buffer_size"`
	DBPath        string `yaml:"db_path"`
	RetentionDays int    `yaml:"retention_days"`
}

// LoadServerConfig loads server configuration from YAML file
func LoadAppConfig(path string) (*AppConfig, error) {
	// TODO: Implement
	// - Read file
	// - Unmarshal YAML
	// - Apply defaults
	// - Override from environment
	// - Validate
	yamlData, err := os.ReadFile(path)
	if err != nil {
		fmt.Println("Error reading config file:", err)
		return nil, err
	}
	var config AppConfig
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

// ApplyDefaults sets default values for server config
func (ac *AppConfig) ApplyDefaults() {
	if ac.Server.Port == 0 {
		ac.Server.Port = 8081
	}
	if ac.Server.Host == "" {
		ac.Server.Host = "localhost"
	}
	if ac.Server.ReadTimeout == 0 {
		ac.Server.ReadTimeout = 60 * time.Second
	}
	if ac.Server.WriteTimeout == 0 {
		ac.Server.WriteTimeout = 10 * time.Second
	}
	if ac.Storage.BufferSize == 0 {
		ac.Storage.BufferSize = 100
	}
	if ac.Storage.DBPath == "" {
		ac.Storage.DBPath = "./data/dht-monitor.db"
	}
	if ac.Storage.RetentionDays == 0 {
		ac.Storage.RetentionDays = 30
	}
}

// OverrideFromEnv overrides config from environment variables
func (ac *AppConfig) OverrideFromEnv() {
	if v := os.Getenv("SERVER_PORT"); v != "" {
		port, err := strconv.Atoi(v)
		if err != nil {
			fmt.Println("Error converting port to int:", err)
			return
		}
		ac.Server.Port = port
	}
	if v := os.Getenv("SERVER_HOST"); v != "" {
		ac.Server.Host = v
	}
	if v := os.Getenv("SERVER_AUTH_TOKEN"); v != "" {
		ac.Server.AuthToken = v
	}
	if v := os.Getenv("LOG_LEVEL"); v != "" {
		ac.Logging.Level = v
	}
}

// Validate checks if server configuration is valid
func (ac *AppConfig) Validate() error {
	// TODO: Validate
	// - Port in range 1-65535
	// - AuthToken not empty
	// - BufferSize >= 10
	// - RetentionDays > 0

	if ac.Server.Port < 1 || ac.Server.Port > 65535 {
		return fmt.Errorf("port must be between 1 and 65535")
	}
	if ac.Server.AuthToken == "" {
		return fmt.Errorf("auth token is required")
	}
	if ac.Storage.BufferSize < 10 {
		return fmt.Errorf("buffer size must be at least 10")
	}
	return nil
}

// String returns a safe string representation (hides auth token)
func (ac *AppConfig) String() string {
	return fmt.Sprintf("AppConfig{Server: %+v, Storage: %+v, Logging: %+v}",
		ac.Server,
		ac.Storage,
		ac.Logging,
	)
}
