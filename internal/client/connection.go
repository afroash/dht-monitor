package client

import (
	"context"
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/afroash/dht-monitor/internal/models"
	"github.com/gorilla/websocket"
	"github.com/rs/zerolog"
)

// ConnectionState represents the current state of the connection
type ConnectionState int

const (
	StateDisconnected ConnectionState = iota
	StateConnecting
	StateConnected
)

func (cs ConnectionState) String() string {
	switch cs {
	case StateDisconnected:
		return "disconnected"
	case StateConnecting:
		return "connecting"
	case StateConnected:
		return "connected"
	default:
		return "unknown"
	}
}

// Connection manages the WebSocket connection to the server
type Connection struct {
	URL                      string
	AuthToken                string
	conn                     *websocket.Conn
	state                    ConnectionState
	stateMutex               sync.RWMutex
	logger                   zerolog.Logger
	sensorInfo               *models.SensorInfo
	reconnectInterval        time.Duration
	maxReconnectInterval     time.Duration
	currentReconnectInterval time.Duration
	pingInterval             time.Duration
	pongTimeout              time.Duration
	lastPong                 time.Time
	lastPongMutex            sync.RWMutex
	stopChan                 chan struct{}
	doneChan                 chan struct{}
}

// ConnectionConfig holds configuration for the connection
type ConnectionConfig struct {
	URL                  string
	AuthToken            string
	ReconnectInterval    time.Duration
	MaxReconnectInterval time.Duration
	PingInterval         time.Duration
	PongTimeout          time.Duration
}

// NewConnection creates a new connection manager
func NewConnection(config ConnectionConfig, sensorInfo *models.SensorInfo, logger zerolog.Logger) *Connection {
	return &Connection{
		URL:                      config.URL,
		AuthToken:                config.AuthToken,
		conn:                     nil,
		state:                    StateDisconnected,
		stateMutex:               sync.RWMutex{},
		logger:                   logger,
		sensorInfo:               sensorInfo,
		reconnectInterval:        config.ReconnectInterval,
		maxReconnectInterval:     config.MaxReconnectInterval,
		currentReconnectInterval: config.ReconnectInterval,
		pingInterval:             config.PingInterval,
		pongTimeout:              config.PongTimeout,
		lastPong:                 time.Time{},
		stopChan:                 make(chan struct{}),
		doneChan:                 make(chan struct{}),
	}
}

// setState safely updates the connection state
func (c *Connection) setState(state ConnectionState) {
	c.stateMutex.Lock()
	defer c.stateMutex.Unlock()
	c.state = state
	c.logger.Info().Str("state", state.String()).Msg("Connection state updated")
}

// State returns the current connection state
func (c *Connection) State() ConnectionState {
	c.stateMutex.RLock()
	defer c.stateMutex.RUnlock()
	return c.state
}

// IsConnected returns true if currently connected
func (c *Connection) IsConnected() bool {
	c.stateMutex.RLock()
	defer c.stateMutex.RUnlock()
	return c.state == StateConnected
}

// Connect establishes a WebSocket connection to the server]
func (c *Connection) Connect(ctx context.Context) error {
	c.setState(StateConnecting)
	c.logger.Info().Str("url", c.URL).Msg("Connecting to server...")

	dialer := websocket.Dialer{
		HandshakeTimeout: 10 * time.Second,
	}

	header := http.Header{}
	header.Set("Authorization", "Bearer "+c.AuthToken)

	conn, resp, err := dialer.DialContext(ctx, c.URL, header)
	if err != nil {
		c.setState(StateDisconnected)
		return fmt.Errorf("dial failed: %w", err)
	}
	defer resp.Body.Close()

	c.conn = conn
	c.setState(StateConnected)
	c.currentReconnectInterval = c.reconnectInterval // reset backoff
	c.logger.Info().Msg("Connected to server")

	if err := c.sendRegistration(); err != nil {
		c.logger.Warn().Err(err).Msg("Failed to send registration")
		return err
	}

	return nil
}

// sendRegistration sends initial sensor info to server
func (c *Connection) sendRegistration() error {
	heartbeat := models.HeartbeatMessage{
		SensorID:   c.sensorInfo.ID,
		Uptime:     int64(c.sensorInfo.Uptime().Seconds()),
		BufferSize: 0,
	}
	msg, err := models.NewMessage(models.MessageTypeHeartbeat, heartbeat)
	if err != nil {
		return err
	}
	return c.sendMessage(msg)
}

// Run starts the connection manager with auto-reconnect
// Blocks until context is cancelled
func (c *Connection) Run(ctx context.Context) error {
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		if err := c.Connect(ctx); err != nil {
			c.logger.Warn().Err(err).Msg("Connection failed")
			c.waitBeforeReconnect(ctx)
			continue
		}

		c.runMessageLoops(ctx)

		c.logger.Info().Msg("Connection lost, will reconnect")
		c.waitBeforeReconnect(ctx)
	}
}

// waitBeforeReconnect waits before next reconnection attempt with exponential backoff
func (c *Connection) waitBeforeReconnect(ctx context.Context) {
	c.logger.Info().Dur("delay", c.currentReconnectInterval).Msg("Waiting before reconnect")
	select {
	case <-time.After(c.currentReconnectInterval):
	case <-ctx.Done():
		return
	}
	c.currentReconnectInterval *= 2
	if c.currentReconnectInterval > c.maxReconnectInterval {
		c.currentReconnectInterval = c.maxReconnectInterval
	}
}

// runMessageLoops runs read and write loops until connection fails
func (c *Connection) runMessageLoops(ctx context.Context) {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	var wg sync.WaitGroup

	wg.Add(1)
	go func() {
		defer wg.Done()
		c.readLoop(ctx)
	}()

	wg.Add(1)
	go func() {
		defer wg.Done()
		c.heartbeatLoop(ctx)
	}()

	wg.Wait()
	c.disconnect()
}

// disconnect closes the WebSocket connection
func (c *Connection) disconnect() {
	c.stateMutex.Lock()
	if c.conn != nil {
		c.conn.Close()
	}
	c.state = StateDisconnected
	c.stateMutex.Unlock()
	c.logger.Info().Msg("Connection disconnected")
}

// Send sends a single reading to the server
func (c *Connection) Send(reading *models.Reading) error {
	if !c.IsConnected() {
		return fmt.Errorf("not connected")
	}
	msg, err := models.NewMessage(models.MessageTypeReading, reading)
	if err != nil {
		return fmt.Errorf("failed to create message: %w", err)
	}
	return c.sendMessage(msg)
}

// sendMessage sends a message over the WebSocket
func (c *Connection) sendMessage(msg *models.Message) error {
	c.stateMutex.Lock()
	defer c.stateMutex.Unlock()
	c.conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
	return c.conn.WriteJSON(msg)
}

// SendBatch sends multiple readings in one message
func (c *Connection) SendBatch(readings []*models.Reading) error {
	if !c.IsConnected() {
		return fmt.Errorf("not connected")
	}

	if len(readings) == 0 {
		return nil
	}

	batchReadings := make([]models.Reading, len(readings))
	for i, r := range readings {
		batchReadings[i] = *r
	}
	batch := models.BatchMessage{
		Readings: batchReadings,
		Count:    len(readings),
	}

	msg, err := models.NewMessage(models.MessageTypeBatch, batch)
	if err != nil {
		return fmt.Errorf("failed to create batch message: %w", err)
	}
	if err := c.sendMessage(msg); err != nil {
		return err
	}
	c.logger.Info().Int("count", len(readings)).Msg("Sent batch of readings")
	return nil
}

// readLoop reads messages from the server
func (c *Connection) readLoop(ctx context.Context) {
	c.logger.Debug().Msg("Starting read loop")
	defer c.logger.Debug().Msg("Read loop stopped")

	for {
		select {
		case <-ctx.Done():
			return
		default:
		}
		var msg models.Message
		if err := c.conn.ReadJSON(&msg); err != nil {
			c.logger.Warn().Err(err).Msg("Read error")
			return
		}
		c.handleMessage(&msg)
	}
}

// handleMessage processes a message received from the server
func (c *Connection) handleMessage(msg *models.Message) {
	c.logger.Debug().Str("type", string(msg.Type)).Msg("Received message")
	switch msg.Type {
	case models.MessageTypeAck:
		c.logger.Debug().Msg("Received ack")
		c.updateLastPong()
	case models.MessageTypeError:
		var errMsg models.ErrorMessage
		if err := msg.UnmarshalPayload(&errMsg); err == nil {
			c.logger.Warn().Str("code", errMsg.Code).Str("msg", errMsg.Message).Msg("Server error")
		}
	case models.MessageTypeConfig:
		c.logger.Info().Msg("Received config update")
	default:
		c.logger.Debug().Str("type", string(msg.Type)).Msg("Unknown message type")
	}
}

// updateLastPong records that we received a pong
func (c *Connection) updateLastPong() {
	c.lastPongMutex.Lock()
	defer c.lastPongMutex.Unlock()
	c.lastPong = time.Now()
}

// timeSinceLastPong returns duration since last pong
func (c *Connection) timeSinceLastPong() time.Duration {
	c.lastPongMutex.RLock()
	defer c.lastPongMutex.RUnlock()
	return time.Since(c.lastPong)
}

// heartbeatLoop sends periodic pings and monitors connection health
func (c *Connection) heartbeatLoop(ctx context.Context) {
	c.logger.Debug().Msg("Starting heartbeat loop")
	defer c.logger.Debug().Msg("Heartbeat loop stopped")

	ticker := time.NewTicker(c.pingInterval)
	defer ticker.Stop()
	c.updateLastPong()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if err := c.sendHeartbeat(); err != nil {
				c.logger.Warn().Err(err).Msg("Failed to send heartbeat")
				return
			}
			if c.timeSinceLastPong() > c.pongTimeout {
				c.logger.Warn().Msg("No pong received, connection appears dead")
				return
			}
		}
	}
}

// sendHeartbeat sends a heartbeat message to the server
func (c *Connection) sendHeartbeat() error {
	heartbeat := models.HeartbeatMessage{
		SensorID:   c.sensorInfo.ID,
		Uptime:     int64(c.sensorInfo.Uptime().Seconds()),
		BufferSize: 0,
	}
	msg, err := models.NewMessage(models.MessageTypeHeartbeat, heartbeat)
	if err != nil {
		return err
	}
	return c.sendMessage(msg)
}

// Close gracefully shuts down the connection
func (c *Connection) Close() error {
	c.logger.Info().Msg("Closing connection")

	select {
	case c.stopChan <- struct{}{}:
	default:
	}

	if c.conn != nil {
		c.conn.WriteControl(
			websocket.CloseMessage,
			websocket.FormatCloseMessage(websocket.CloseNormalClosure, ""),
			time.Now().Add(time.Second),
		)
		c.conn.Close()
	}

	c.setState(StateDisconnected)
	c.logger.Info().Msg("Connection closed")
	return nil
}
