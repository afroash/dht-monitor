package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"github.com/afroash/dht-monitor/internal/config"
	"github.com/afroash/dht-monitor/internal/server"
	"github.com/afroash/dht-monitor/internal/storage"

	"github.com/rs/zerolog"
)

const version = "v0.2.0"

func main() {
	// Parse flags
	configPath := flag.String("config", "configs/server.yaml", "path to config file")
	flag.Parse()

	// TODO: Load configuration
	cfg, err := config.LoadAppConfig(*configPath)
	if err != nil {
		log.Fatalf("Failed to load config: %v", err)
	}
	//fmt.Println(cfg.String())
	store := server.NewMemoryStore(cfg.Storage.BufferSize)

	// Similar to sensor client
	logger := zerolog.New(os.Stdout).With().Timestamp().Logger()

	logger.Info().
		Str("version", version).
		Int("port", cfg.Server.Port).
		Msg("Starting DHT Monitor Server")

	var sqliteStore *storage.SQLiteStore
	var dbWriter *storage.DBWriter
	var retentionCleaner *storage.RetentionCleaner

	// Setup database
	if cfg.Database.Enabled {
		// Ensure data directory exists
		dataDir := filepath.Dir(cfg.Database.Path)
		if err := os.MkdirAll(dataDir, 0755); err != nil {
			log.Fatalf("Failed to create data directory: %v", err)
		}
		sqliteStore, err = storage.NewSQLiteStore(cfg.Database.Path, logger)
		if err != nil {
			log.Fatalf("Failed to create SQLite store: %v", err)
		}
		log.Printf("SQLite store created at %s", cfg.Database.Path)

		// Create Async DB Writer
		writerConfig := storage.DBWriterConfig{
			BatchSize:   cfg.Database.BatchSize,
			FlushPeriod: cfg.Database.FlushPeriod,
			ChannelSize: cfg.Database.ChannelSize,
		}

		dbWriter = storage.NewDBWriter(sqliteStore, writerConfig, logger)

		// Create Retention Cleaner
		cleanerConfig := storage.RetentionCleanerConfig{
			RetentionDays: cfg.Database.RetentionDays,
			CleanupPeriod: cfg.Database.CleanupPeriod,
		}
		retentionCleaner = storage.NewRetentionCleaner(sqliteStore, cleanerConfig, logger)
		logger.Info().Int("retention_days", cfg.Database.RetentionDays).Dur("cleanup_period", cfg.Database.CleanupPeriod).Msg("RetentionCleaner started")

	}

	mux := http.NewServeMux()

	var apiHandler *server.APIHandler
	if sqliteStore != nil {
		apiHandler = server.NewAPIHandlerWithHistory(store, sqliteStore, logger)
	} else {
		apiHandler = server.NewAPIHandler(store, logger)
	}

	// Serve static dashboard
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		// Only serve dashboard for exact root path or /index.html
		if r.URL.Path != "/" && r.URL.Path != "/index.html" {
			http.NotFound(w, r)
			return
		}
		logger.Info().Str("path", r.URL.Path).Msg("Serving dashboard")
		http.ServeFile(w, r, "web/templates/dashboard.html")
	})

	// API endpoints
	mux.HandleFunc("/api/current", apiHandler.HandleCurrent)
	mux.HandleFunc("/api/history", apiHandler.HandleHistory)
	mux.HandleFunc("/api/stats", apiHandler.HandleStats)
	mux.HandleFunc("/api/daily/stats", apiHandler.HandleDailyStats)
	mux.HandleFunc("/api/sensors", apiHandler.HandleSensors)
	mux.HandleFunc("/api/dashboard-data", apiHandler.HandleDashboardData)

	// Health check endpoint
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		fmt.Fprintf(w, `{"status":"ok","version":"%s"}`, version)
	})
	mux.HandleFunc("/stats", func(w http.ResponseWriter, r *http.Request) {
		stats := store.Stats()
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(stats)
	})

	mux.HandleFunc("/readings/latest", func(w http.ResponseWriter, r *http.Request) {
		sensorID := r.URL.Query().Get("sensor_id")
		if sensorID == "" {
			sensorID = "pi-sensor-01" // Default
		}

		readings := store.GetLatest(sensorID, 10)
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(readings)
	})

	handler := server.NewHandler(
		cfg.Server.AuthToken,
		store,
		logger,
		cfg.Server.AllowedOrigins...,
	)
	if dbWriter != nil {
		handler.SetDBWriter(dbWriter)
	}

	mux.HandleFunc("/sensor-stream", handler.ServeHTTP)

	server := &http.Server{
		Addr:         fmt.Sprintf("%s:%d", cfg.Server.Host, cfg.Server.Port),
		Handler:      mux,
		ReadTimeout:  cfg.Server.ReadTimeout,
		WriteTimeout: cfg.Server.WriteTimeout,
	}

	// Start server in goroutine
	go func() {
		logger.Info().Str("addr", server.Addr).Msg("Server listening")
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logger.Fatal().Err(err).Msg("Server failed")
		}
	}()

	// Wait for shutdown signal
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
	<-sigChan

	logger.Info().Msg("Shutting down server...")

	if dbWriter != nil {
		dbWriter.Stop()
		logger.Info().Msg("DBWriter stopped")
	}
	if retentionCleaner != nil {
		retentionCleaner.Stop()
		logger.Info().Msg("RetentionCleaner stopped")
	}
	if sqliteStore != nil {
		sqliteStore.Close()
		logger.Info().Msg("SQLiteStore closed")
	}

	// Graceful shutdown
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := server.Shutdown(ctx); err != nil {
		logger.Error().Err(err).Msg("Server shutdown error")
	}

	logger.Info().Msg("Server stopped")
}
