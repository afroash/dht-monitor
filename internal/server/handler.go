package server

import (
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/afroash/dht-monitor/internal/models"
	"github.com/gorilla/websocket"
	"github.com/rs/zerolog"
)

// Handler manages WebSocket connections from sensors
type Handler struct {
	// TODO: Add fields:
	upgrader      websocket.Upgrader
	authToken     string
	store         ReadingStore
	logger        zerolog.Logger
	activeSensors map[string]*SensorConnection
	mutex         sync.RWMutex
}

// SensorConnection represents an active sensor connection
type SensorConnection struct {
	SensorID    string `json:"sensor_id"`
	Conn        *websocket.Conn
	LastSeen    time.Time
	ConnectedAt time.Time
}

// NewHandler creates a new WebSocket handler
func NewHandler(authToken string, store ReadingStore, logger zerolog.Logger) *Handler {
	return &Handler{
		upgrader: websocket.Upgrader{
			ReadBufferSize:  1024,
			WriteBufferSize: 1024,
			CheckOrigin:     func(r *http.Request) bool { return true },
		},
		authToken:     authToken,
		store:         store,
		logger:        logger,
		activeSensors: make(map[string]*SensorConnection),
		mutex:         sync.RWMutex{},
	}
}

// ServeHTTP handles WebSocket connection requests
func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	// Check auth token from header
	// Expected format: "Bearer <token>"
	token := r.Header.Get("Authorization")
	if !h.validateToken(token) {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}
	// Upgrade connection
	conn, err := h.upgrader.Upgrade(w, r, nil)
	if err != nil {
		h.logger.Error().Err(err).Msg("Failed to upgrade connection")
		return
	}

	// Handle the connection
	h.handleConnection(conn)

}

// validateToken checks if the auth token is valid
func (h *Handler) validateToken(authHeader string) bool {
	if !strings.HasPrefix(authHeader, "Bearer ") {
		return false
	}
	token := strings.TrimPrefix(authHeader, "Bearer ")
	if token != h.authToken {
		return false
	}
	return true
}

// handleConnection manages a single WebSocket connection
func (h *Handler) handleConnection(conn *websocket.Conn) {
	sensorID := conn.RemoteAddr().String()
	sensorConn := &SensorConnection{
		SensorID:    sensorID,
		Conn:        conn,
		LastSeen:    time.Now(),
		ConnectedAt: time.Now(),
	}
	h.activeSensors[sensorID] = sensorConn

	defer conn.Close()
	defer h.removeSensor(sensorID)

	// Set read deadline

	conn.SetReadDeadline(time.Now().Add(pongWait))
	conn.SetPongHandler(func(string) error {
		conn.SetReadDeadline(time.Now().Add(pongWait))
		return nil
	})

	// Read loop
	for {
		var msg models.Message
		err := conn.ReadJSON(&msg)
		if err != nil {
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
				h.logger.Warn().Err(err).Msg("WebSocket error")
			}
			break
		}
		h.handleMessage(conn, &msg)
	}
}

// handleMessage processes a single message from the sensor
func (h *Handler) handleMessage(conn *websocket.Conn, msg *models.Message) {
	h.logger.Debug().Str("type", string(msg.Type)).Msg("Received message")

	switch msg.Type {
	case models.MessageTypeReading:
		h.handleReading(msg)
	case models.MessageTypeBatch:
		h.handleBatch(msg)
	case models.MessageTypeHeartbeat:
		h.handleHeartbeat(msg)
	default:
		h.logger.Warn().Str("type", string(msg.Type)).Msg("Unknown message type")
	}

	h.sendAck(conn)
}

// handleReading processes a single reading
func (h *Handler) handleReading(msg *models.Message) {
	var readingMsg models.ReadingMessage
	if err := msg.UnmarshalPayload(&readingMsg); err != nil {
		h.logger.Error().Err(err).Msg("Failed to unmarshal reading")
		return
	}
	reading := models.Reading{
		SensorID:    readingMsg.SensorID,
		Timestamp:   readingMsg.Timestamp,
		Humidity:    readingMsg.Humidity,
		Temperature: readingMsg.Temperature,
	}
	if reading.IsValid() {
		h.store.Add(&reading)
	}
	h.logger.Info().Str("sensor_id", reading.SensorID).Float64("temp", reading.Temperature).Float64("humidity", reading.Humidity).Msg("Reading stored")
}

// handleBatch processes a batch of readings
func (h *Handler) handleBatch(msg *models.Message) {
	var batch models.BatchMessage
	if err := msg.UnmarshalPayload(&batch); err != nil {
		h.logger.Error().Err(err).Msg("Failed to unmarshal batch")
		return
	}
	for _, reading := range batch.Readings {
		if reading.IsValid() {
			h.store.Add(&reading)
		}
	}
	h.logger.Info().Int("count", batch.Count).Msg("Batch stored")
}

// handleHeartbeat processes a heartbeat message
func (h *Handler) handleHeartbeat(msg *models.Message) {
	var heartbeat models.HeartbeatMessage
	if err := msg.UnmarshalPayload(&heartbeat); err != nil {
		h.logger.Error().Err(err).Msg("Failed to unmarshal heartbeat")
		return
	}
	h.updateSensorLastSeen(heartbeat.SensorID)
	h.logger.Debug().Str("sensor_id", heartbeat.SensorID).Int64("uptime", heartbeat.Uptime).Msg("Heartbeat received")
}

// sendAck sends an acknowledgment message
func (h *Handler) sendAck(conn *websocket.Conn) {
	ack := models.AckMessage{Status: "ok"}
	msg, err := models.NewMessage(models.MessageTypeAck, ack)
	if err != nil {
		h.logger.Error().Err(err).Msg("Failed to create ack message")
		return
	}
	conn.SetWriteDeadline(time.Now().Add(writeWait))
	if err := conn.WriteJSON(msg); err != nil {
		h.logger.Warn().Err(err).Msg("Failed to send ack")
	}
}

// updateSensorLastSeen updates the last seen timestamp for a sensor
func (h *Handler) updateSensorLastSeen(sensorID string) {
	h.mutex.Lock()
	defer h.mutex.Unlock()
	if sensor, exists := h.activeSensors[sensorID]; exists {
		sensor.LastSeen = time.Now()
	}
}

// removeSensor removes a sensor from the active sensors map
func (h *Handler) removeSensor(sensorID string) {
	h.mutex.Lock()
	defer h.mutex.Unlock()
	delete(h.activeSensors, sensorID)
	h.logger.Info().Str("sensor_id", sensorID).Msg("Sensor disconnected")
}

// GetActiveSensors returns a list of currently connected sensors
func (h *Handler) GetActiveSensors() []SensorConnection {
	h.mutex.RLock()
	defer h.mutex.RUnlock()
	sensors := make([]SensorConnection, 0, len(h.activeSensors))
	for _, sensor := range h.activeSensors {
		sensors = append(sensors, *sensor)
	}
	return sensors
}

// ReadingStore interface for storing readings
type ReadingStore interface {
	Add(reading *models.Reading)
	GetLatest(sensorID string, n int) []*models.Reading
	GetAll() []*models.Reading
	GetCurrentReading(sensorID string) *models.Reading
	GetSensorIDs() []string
	Stats() StoreStats
}

// Constants for WebSocket timeouts
const (
	writeWait  = 10 * time.Second
	pongWait   = 60 * time.Second
	pingPeriod = (pongWait * 9) / 10
)
