package server

import (
	"encoding/json"
	"net/http"
	"strconv"
	"time"

	"github.com/afroash/dht-monitor/internal/models"
	"github.com/rs/zerolog"
)

// APIHandler handles HTTP API requests for the dashboard
type APIHandler struct {
	store  ReadingStore
	logger zerolog.Logger
}

// NewAPIHandler creates a new API handler
func NewAPIHandler(store ReadingStore, logger zerolog.Logger) *APIHandler {
	return &APIHandler{
		store:  store,
		logger: logger,
	}
}

// HandleCurrent returns the current reading for a sensor
func (api *APIHandler) HandleCurrent(w http.ResponseWriter, r *http.Request) {
	// TODO: Get sensor ID from query param (default to first sensor)
	sensorID := r.URL.Query().Get("sensor_id")
	if sensorID == "" {
		sensorIDs := api.store.GetSensorIDs()
		if len(sensorIDs) == 0 {
			http.Error(w, "No sensors found", http.StatusNotFound)
			return
		}
		sensorID = sensorIDs[0]
	}

	reading := api.store.GetCurrentReading(sensorID)
	if reading == nil {
		http.Error(w, "No readings available", http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(reading)

}

// HandleHistory returns recent readings for charting
func (api *APIHandler) HandleHistory(w http.ResponseWriter, r *http.Request) {
	sensorID := r.URL.Query().Get("sensor_id")
	if sensorID == "" {
		sensorIDs := api.store.GetSensorIDs()
		if len(sensorIDs) == 0 {
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode([]models.Reading{})
			return
		}
		sensorID = sensorIDs[0]
	}

	limitStr := r.URL.Query().Get("limit")
	limit := 50 // default
	if limitStr != "" {
		if parsed, err := strconv.Atoi(limitStr); err == nil && parsed > 0 {
			limit = parsed
		}
	}

	readings := api.store.GetLatest(sensorID, limit)

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(readings)
}

// HandleStats returns store statistics
func (api *APIHandler) HandleStats(w http.ResponseWriter, r *http.Request) {
	stats := api.store.Stats()

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(stats)
}

// DashboardData contains all data for the dashboard
type DashboardData struct {
	CurrentReading *models.Reading `json:"current_reading"`
	Stats          StoreStats      `json:"stats"`
	SensorIDs      []string        `json:"sensor_ids"`
	LastUpdate     time.Time       `json:"last_update"`
}

// HandleDashboardData returns combined data for dashboard (htmx-friendly)
func (api *APIHandler) HandleDashboardData(w http.ResponseWriter, r *http.Request) {
	sensorIDs := api.store.GetSensorIDs()

	var currentReading *models.Reading
	if len(sensorIDs) > 0 {
		// Check if a specific sensor was requested
		requestedSensor := r.URL.Query().Get("sensor_id")
		if requestedSensor != "" {
			currentReading = api.store.GetCurrentReading(requestedSensor)
		} else {
			currentReading = api.store.GetCurrentReading(sensorIDs[0])
		}
	}

	data := DashboardData{
		CurrentReading: currentReading,
		Stats:          api.store.Stats(),
		SensorIDs:      sensorIDs,
		LastUpdate:     time.Now(),
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(data)
}
