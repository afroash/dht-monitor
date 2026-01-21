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
	store           ReadingStore
	historicalStore HistoricalStore // Optional - nil if SQLite not enabled
	logger          zerolog.Logger
}

// NewAPIHandler creates a new API handler (without historical store)
func NewAPIHandler(store ReadingStore, logger zerolog.Logger) *APIHandler {
	return &APIHandler{
		store:           store,
		historicalStore: nil,
		logger:          logger,
	}
}

// NewAPIHandlerWithHistory creates a new API handler with historical store support
func NewAPIHandlerWithHistory(store ReadingStore, historicalStore HistoricalStore, logger zerolog.Logger) *APIHandler {
	return &APIHandler{
		store:           store,
		historicalStore: historicalStore,
		logger:          logger,
	}
}

// SetHistoricalStore sets the historical store (can be called after creation)
func (api *APIHandler) SetHistoricalStore(store HistoricalStore) {
	api.historicalStore = store
}

// HandleCurrent returns the current reading for a sensor
func (api *APIHandler) HandleCurrent(w http.ResponseWriter, r *http.Request) {
	sensorID := r.URL.Query().Get("sensor_id")
	if sensorID == "" {
		sensorIDs := api.store.GetSensorIDs()
		if len(sensorIDs) == 0 {
			http.Error(w, "No sensors found", http.StatusNotFound)
			return
		}
		sensorID = sensorIDs[0]
	}

	// Try memory store first (fastest for current data)
	reading := api.store.GetCurrentReading(sensorID)

	// Fall back to historical store if not in memory
	if reading == nil && api.historicalStore != nil {
		var err error
		reading, err = api.historicalStore.GetLatestReading(sensorID)
		if err != nil {
			api.logger.Error().Err(err).Msg("Failed to get latest reading from historical store")
		}
	}

	if reading == nil {
		http.Error(w, "No readings available", http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(reading)
}

// HistoryResponse wraps history results with metadata for frontend navigation
type HistoryResponse struct {
	Readings    []*models.Reading `json:"readings"`
	Count       int               `json:"count"`
	HasMore     bool              `json:"has_more"`
	WindowStart *time.Time        `json:"window_start,omitempty"`
	WindowEnd   *time.Time        `json:"window_end,omitempty"`
	Source      string            `json:"source"` // "memory" or "database"
}

// HandleHistory returns readings for charting
// Supports multiple query modes:
//   - Default: GET /api/history?limit=50 (recent from memory)
//   - Time range: GET /api/history?start=...&end=...&limit=500 (from database)
//   - Scroll back: GET /api/history?before=...&limit=100 (from database)
//   - Scroll forward: GET /api/history?after=...&limit=100 (from database)
func (api *APIHandler) HandleHistory(w http.ResponseWriter, r *http.Request) {
	query := r.URL.Query()

	// Get sensor ID
	sensorID := query.Get("sensor_id")
	if sensorID == "" {
		sensorIDs := api.store.GetSensorIDs()
		if len(sensorIDs) == 0 {
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(HistoryResponse{
				Readings: []*models.Reading{},
				Count:    0,
				Source:   "memory",
			})
			return
		}
		sensorID = sensorIDs[0]
	}

	// Parse limit
	limit := 50 // default
	if limitStr := query.Get("limit"); limitStr != "" {
		if parsed, err := strconv.Atoi(limitStr); err == nil && parsed > 0 {
			if parsed > 1000 {
				parsed = 1000 // Cap at 1000
			}
			limit = parsed
		}
	}

	// Check for historical query parameters
	startStr := query.Get("start")
	endStr := query.Get("end")
	beforeStr := query.Get("before")
	afterStr := query.Get("after")

	// Determine if this is a historical query
	isHistoricalQuery := startStr != "" || endStr != "" || beforeStr != "" || afterStr != ""

	// If historical query requested but no historical store available
	if isHistoricalQuery && api.historicalStore == nil {
		api.logger.Warn().Msg("Historical query requested but no historical store configured")
		// Fall back to memory store
		isHistoricalQuery = false
	}

	var response HistoryResponse

	if isHistoricalQuery {
		// Route to historical store
		readings, windowStart, windowEnd, err := api.queryHistoricalStore(sensorID, startStr, endStr, beforeStr, afterStr, limit)
		if err != nil {
			api.logger.Error().Err(err).Msg("Historical query failed")
			http.Error(w, "Failed to retrieve historical data", http.StatusInternalServerError)
			return
		}

		response = HistoryResponse{
			Readings:    readings,
			Count:       len(readings),
			HasMore:     len(readings) >= limit,
			WindowStart: windowStart,
			WindowEnd:   windowEnd,
			Source:      "database",
		}
	} else {
		// Use memory store (default behavior)
		readings := api.store.GetLatest(sensorID, limit)
		if readings == nil {
			readings = []*models.Reading{}
		}

		response = HistoryResponse{
			Readings: readings,
			Count:    len(readings),
			HasMore:  false, // Memory store doesn't support pagination
			Source:   "memory",
		}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

// queryHistoricalStore handles the different historical query modes
func (api *APIHandler) queryHistoricalStore(sensorID, startStr, endStr, beforeStr, afterStr string, limit int) ([]*models.Reading, *time.Time, *time.Time, error) {
	var readings []*models.Reading
	var err error
	var windowStart, windowEnd *time.Time

	if beforeStr != "" {
		// Scroll back mode: get readings before a timestamp
		before, parseErr := time.Parse(time.RFC3339, beforeStr)
		if parseErr != nil {
			return nil, nil, nil, parseErr
		}
		readings, err = api.historicalStore.GetReadingsBefore(sensorID, before, limit)
		windowEnd = &before

	} else if afterStr != "" {
		// Scroll forward mode: get readings after a timestamp
		after, parseErr := time.Parse(time.RFC3339, afterStr)
		if parseErr != nil {
			return nil, nil, nil, parseErr
		}
		readings, err = api.historicalStore.GetReadingsAfter(sensorID, after, limit)
		windowStart = &after

	} else {
		// Range mode: get readings within start/end window
		var start, end time.Time

		if startStr != "" {
			start, err = time.Parse(time.RFC3339, startStr)
			if err != nil {
				return nil, nil, nil, err
			}
		} else {
			// Default start: 6 hours ago
			start = time.Now().UTC().Add(-6 * time.Hour)
		}

		if endStr != "" {
			end, err = time.Parse(time.RFC3339, endStr)
			if err != nil {
				return nil, nil, nil, err
			}
		} else {
			// Default end: now
			end = time.Now().UTC()
		}

		readings, err = api.historicalStore.GetReadingsInRange(sensorID, start, end, limit)
		windowStart = &start
		windowEnd = &end
	}

	if err != nil {
		return nil, nil, nil, err
	}

	// Ensure we never return nil slice
	if readings == nil {
		readings = []*models.Reading{}
	}

	return readings, windowStart, windowEnd, nil
}

// HandleStats returns store statistics
func (api *APIHandler) HandleStats(w http.ResponseWriter, r *http.Request) {
	stats := api.store.Stats()

	// Build response with top-level fields always present
	response := map[string]interface{}{
		"total_readings":   stats.TotalReadings,
		"unique_sensors":   stats.UniqueSensors,
		"current_readings": stats.CurrentReadings,
		"oldest_reading":   stats.OldestReading,
		"newest_reading":   stats.NewestReading,
		"memory":           stats,
	}

	// If we have a historical store, include its stats under "database" key
	if api.historicalStore != nil {
		historicalStats, err := api.historicalStore.GetStorageStats()
		if err != nil {
			api.logger.Error().Err(err).Msg("Failed to get historical store stats")
		} else {
			response["database"] = historicalStats
		}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

// HandleDailyStats returns aggregated daily statistics (requires historical store)
// GET /api/stats/daily?sensor_id=...&start=2024-01-01&end=2024-01-07
func (api *APIHandler) HandleDailyStats(w http.ResponseWriter, r *http.Request) {
	if api.historicalStore == nil {
		http.Error(w, "Historical data not available", http.StatusServiceUnavailable)
		return
	}

	query := r.URL.Query()

	// Get sensor ID
	sensorID := query.Get("sensor_id")
	if sensorID == "" {
		sensorIDs := api.store.GetSensorIDs()
		if len(sensorIDs) > 0 {
			sensorID = sensorIDs[0]
		}
	}

	// Parse date range
	var start, end time.Time
	var err error

	if startStr := query.Get("start"); startStr != "" {
		start, err = parseDate(startStr)
		if err != nil {
			http.Error(w, "Invalid 'start' date format", http.StatusBadRequest)
			return
		}
	} else {
		// Default: last 7 days
		start = time.Now().UTC().AddDate(0, 0, -7).Truncate(24 * time.Hour)
	}

	if endStr := query.Get("end"); endStr != "" {
		end, err = parseDate(endStr)
		if err != nil {
			http.Error(w, "Invalid 'end' date format", http.StatusBadRequest)
			return
		}
		// End of the specified day
		end = end.Add(24*time.Hour - time.Second)
	} else {
		end = time.Now().UTC()
	}

	stats, err := api.historicalStore.GetDailyStats(sensorID, start, end)
	if err != nil {
		api.logger.Error().Err(err).Msg("Failed to get daily stats")
		http.Error(w, "Failed to retrieve statistics", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(stats)
}

// DashboardData contains all data for the dashboard
type DashboardData struct {
	CurrentReading *models.Reading `json:"current_reading"`
	Stats          StoreStats      `json:"stats"`
	SensorIDs      []string        `json:"sensor_ids"`
	LastUpdate     time.Time       `json:"last_update"`
	HasHistory     bool            `json:"has_history"` // True if historical store is available
}

// HandleDashboardData returns combined data for dashboard
func (api *APIHandler) HandleDashboardData(w http.ResponseWriter, r *http.Request) {
	sensorIDs := api.store.GetSensorIDs()

	var currentReading *models.Reading
	if len(sensorIDs) > 0 {
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
		HasHistory:     api.historicalStore != nil,
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(data)
}

// HandleSensors returns list of known sensors
// GET /api/sensors
func (api *APIHandler) HandleSensors(w http.ResponseWriter, r *http.Request) {
	sensors := api.store.GetSensorIDs()

	// Also get from historical store if available
	if api.historicalStore != nil {
		historicalSensors, err := api.historicalStore.GetSensorIDs()
		if err == nil {
			// Merge unique sensors
			sensorMap := make(map[string]bool)
			for _, s := range sensors {
				sensorMap[s] = true
			}
			for _, s := range historicalSensors {
				sensorMap[s] = true
			}
			sensors = make([]string, 0, len(sensorMap))
			for s := range sensorMap {
				sensors = append(sensors, s)
			}
		}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"sensors": sensors,
		"count":   len(sensors),
	})
}

// parseDate tries multiple date formats
func parseDate(s string) (time.Time, error) {
	formats := []string{
		time.RFC3339,
		"2006-01-02T15:04:05Z",
		"2006-01-02",
	}

	for _, format := range formats {
		if t, err := time.Parse(format, s); err == nil {
			return t, nil
		}
	}

	return time.Time{}, &time.ParseError{Value: s}
}
