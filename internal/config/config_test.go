// internal/config/config_test.go
package config

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestLoadConfig(t *testing.T) {
	// Create a temporary config file
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "test-config.yaml")

	configContent := `
sensor:
  id: "test-sensor-01"
  location: "Test Lab"
  type: "DHT11"
  gpio_pin: 4
  read_interval: 30s

server:
  url: "wss://example.com/sensor-stream"
  auth_token: "test-token-12345"
  connect_timeout: 10s
  reconnect_interval: 1s
  max_reconnect_interval: 5m
  ping_interval: 30s
  pong_timeout: 10s

buffer:
  size: 1000
  drop_oldest: true
  persist_to_disk: false

logging:
  level: "info"
  format: "json"
  file_path: "/var/log/sensor.log"
  max_size_mb: 10
  max_backups: 3
`

	err := os.WriteFile(configPath, []byte(configContent), 0644)
	if err != nil {
		t.Fatalf("Failed to write test config: %v", err)
	}

	// Load config
	cfg, err := LoadConfig(configPath)
	if err != nil {
		t.Fatalf("LoadConfig failed: %v", err)
	}

	// Verify values
	if cfg.Sensor.ID != "test-sensor-01" {
		t.Errorf("Sensor.ID = %v, want test-sensor-01", cfg.Sensor.ID)
	}
	if cfg.Sensor.GPIOPin != 4 {
		t.Errorf("Sensor.GPIOPin = %v, want 4", cfg.Sensor.GPIOPin)
	}
	if cfg.Sensor.ReadInterval != 30*time.Second {
		t.Errorf("Sensor.ReadInterval = %v, want 30s", cfg.Sensor.ReadInterval)
	}
	if cfg.Server.URL != "wss://example.com/sensor-stream" {
		t.Errorf("Server.URL = %v", cfg.Server.URL)
	}
	if cfg.Server.AuthToken != "test-token-12345" {
		t.Errorf("Server.AuthToken = %v", cfg.Server.AuthToken)
	}
	if cfg.Buffer.Size != 1000 {
		t.Errorf("Buffer.Size = %v, want 1000", cfg.Buffer.Size)
	}
	if cfg.Logging.Level != "info" {
		t.Errorf("Logging.Level = %v, want info", cfg.Logging.Level)
	}
}

func TestConfig_ApplyDefaults(t *testing.T) {
	cfg := &Config{}
	cfg.ApplyDefaults()

	// Check that defaults are applied
	if cfg.Sensor.Type != "DHT11" {
		t.Errorf("Default Sensor.Type = %v, want DHT11", cfg.Sensor.Type)
	}
	if cfg.Sensor.ReadInterval != 30*time.Second {
		t.Errorf("Default ReadInterval = %v, want 30s", cfg.Sensor.ReadInterval)
	}
	if cfg.Buffer.Size != 1000 {
		t.Errorf("Default Buffer.Size = %v, want 1000", cfg.Buffer.Size)
	}
	if !cfg.Buffer.DropOldest {
		t.Error("Default Buffer.DropOldest should be true")
	}
	if cfg.Logging.Level != "info" {
		t.Errorf("Default Logging.Level = %v, want info", cfg.Logging.Level)
	}
}

func TestConfig_OverrideFromEnv(t *testing.T) {
	// Set environment variables
	os.Setenv("SENSOR_ID", "env-sensor-01")
	os.Setenv("SERVER_URL", "wss://env-server.com/ws")
	os.Setenv("SERVER_AUTH_TOKEN", "env-token-xyz")
	os.Setenv("LOG_LEVEL", "debug")
	defer func() {
		os.Unsetenv("SENSOR_ID")
		os.Unsetenv("SERVER_URL")
		os.Unsetenv("SERVER_AUTH_TOKEN")
		os.Unsetenv("LOG_LEVEL")
	}()

	cfg := &Config{
		Sensor: SensorConfig{
			ID: "config-sensor",
		},
		Server: ServerConfig{
			URL:       "wss://config-server.com/ws",
			AuthToken: "config-token",
		},
		Logging: LoggingConfig{
			Level: "info",
		},
	}

	cfg.OverrideFromEnv()

	// Check that env vars override config values
	if cfg.Sensor.ID != "env-sensor-01" {
		t.Errorf("Sensor.ID = %v, want env-sensor-01", cfg.Sensor.ID)
	}
	if cfg.Server.URL != "wss://env-server.com/ws" {
		t.Errorf("Server.URL = %v", cfg.Server.URL)
	}
	if cfg.Server.AuthToken != "env-token-xyz" {
		t.Errorf("Server.AuthToken = %v", cfg.Server.AuthToken)
	}
	if cfg.Logging.Level != "debug" {
		t.Errorf("Logging.Level = %v, want debug", cfg.Logging.Level)
	}
}

func TestConfig_Validate(t *testing.T) {
	tests := []struct {
		name      string
		config    Config
		wantError bool
	}{
		{
			name: "valid config",
			config: Config{
				Sensor: SensorConfig{
					ID:           "sensor-01",
					GPIOPin:      4,
					ReadInterval: 30 * time.Second,
				},
				Server: ServerConfig{
					URL:               "wss://example.com/ws",
					AuthToken:         "token123",
					ReconnectInterval: 1 * time.Second,
				},
				Buffer: BufferConfig{
					Size: 1000,
				},
			},
			wantError: false,
		},
		{
			name: "missing sensor ID",
			config: Config{
				Sensor: SensorConfig{
					ID:      "", // invalid
					GPIOPin: 4,
				},
				Server: ServerConfig{
					URL:       "wss://example.com/ws",
					AuthToken: "token123",
				},
			},
			wantError: true,
		},
		{
			name: "invalid GPIO pin",
			config: Config{
				Sensor: SensorConfig{
					ID:      "sensor-01",
					GPIOPin: 0, // invalid
				},
				Server: ServerConfig{
					URL:       "wss://example.com/ws",
					AuthToken: "token123",
				},
			},
			wantError: true,
		},
		{
			name: "missing server URL",
			config: Config{
				Sensor: SensorConfig{
					ID:      "sensor-01",
					GPIOPin: 4,
				},
				Server: ServerConfig{
					URL:       "", // invalid
					AuthToken: "token123",
				},
			},
			wantError: true,
		},
		{
			name: "missing auth token",
			config: Config{
				Sensor: SensorConfig{
					ID:      "sensor-01",
					GPIOPin: 4,
				},
				Server: ServerConfig{
					URL:       "wss://example.com/ws",
					AuthToken: "", // invalid
				},
			},
			wantError: true,
		},
		{
			name: "invalid server URL scheme",
			config: Config{
				Sensor: SensorConfig{
					ID:      "sensor-01",
					GPIOPin: 4,
				},
				Server: ServerConfig{
					URL:       "http://example.com/ws", // should be ws:// or wss://
					AuthToken: "token123",
				},
			},
			wantError: true,
		},
		{
			name: "buffer size too small",
			config: Config{
				Sensor: SensorConfig{
					ID:      "sensor-01",
					GPIOPin: 4,
				},
				Server: ServerConfig{
					URL:       "wss://example.com/ws",
					AuthToken: "token123",
				},
				Buffer: BufferConfig{
					Size: 5, // too small
				},
			},
			wantError: true,
		},
		{
			name: "read interval too short",
			config: Config{
				Sensor: SensorConfig{
					ID:           "sensor-01",
					GPIOPin:      4,
					ReadInterval: 500 * time.Millisecond, // too short
				},
				Server: ServerConfig{
					URL:       "wss://example.com/ws",
					AuthToken: "token123",
				},
			},
			wantError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.config.Validate()
			if tt.wantError && err == nil {
				t.Error("Validate() expected error, got nil")
			}
			if !tt.wantError && err != nil {
				t.Errorf("Validate() unexpected error: %v", err)
			}
		})
	}
}

func TestConfig_String_MasksToken(t *testing.T) {
	cfg := &Config{
		Sensor: SensorConfig{
			ID: "sensor-01",
		},
		Server: ServerConfig{
			URL:       "wss://example.com/ws",
			AuthToken: "secret-token-12345",
		},
	}

	str := cfg.String()

	// Should not contain full token
	if contains(str, "secret-token-12345") {
		t.Error("String() should mask auth token")
	}

	// Should contain masked version
	if !contains(str, "secr****") {
		t.Error("String() should contain masked token")
	}
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > len(substr) && (s[:len(substr)] == substr || s[len(s)-len(substr):] == substr || containsMiddle(s, substr)))
}

func containsMiddle(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
