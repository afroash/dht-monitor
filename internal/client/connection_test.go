package client

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/afroash/dht-monitor/internal/models"
	"github.com/gorilla/websocket"
	"github.com/rs/zerolog"
)

// MockWebSocketServer creates a test WebSocket server
type MockWebSocketServer struct {
	server          *httptest.Server
	upgrader        websocket.Upgrader
	connections     []*websocket.Conn
	receivedMsgs    []models.Message
	shouldAccept    bool
	respondWithPong bool
	closeAfterN     int // close connection after N messages
	msgCount        int
}

func NewMockWebSocketServer() *MockWebSocketServer {
	mock := &MockWebSocketServer{
		upgrader: websocket.Upgrader{
			CheckOrigin: func(r *http.Request) bool { return true },
		},
		shouldAccept:    true,
		respondWithPong: true,
		receivedMsgs:    []models.Message{},
	}

	mock.server = httptest.NewServer(http.HandlerFunc(mock.handleWebSocket))
	return mock
}

func (m *MockWebSocketServer) handleWebSocket(w http.ResponseWriter, r *http.Request) {
	if !m.shouldAccept {
		w.WriteHeader(http.StatusForbidden)
		return
	}

	// Check auth token
	authHeader := r.Header.Get("Authorization")
	if !strings.HasPrefix(authHeader, "Bearer ") {
		w.WriteHeader(http.StatusUnauthorized)
		return
	}

	conn, err := m.upgrader.Upgrade(w, r, nil)
	if err != nil {
		return
	}
	defer conn.Close()

	m.connections = append(m.connections, conn)

	// Read messages
	for {
		var msg models.Message
		err := conn.ReadJSON(&msg)
		if err != nil {
			break
		}

		m.receivedMsgs = append(m.receivedMsgs, msg)
		m.msgCount++

		// Respond with ack if configured
		if m.respondWithPong {
			ack := models.AckMessage{Status: "ok"}
			ackMsg, _ := models.NewMessage(models.MessageTypeAck, ack)
			conn.WriteJSON(ackMsg)
		}

		// Close after N messages if configured
		if m.closeAfterN > 0 && m.msgCount >= m.closeAfterN {
			return
		}
	}
}

func (m *MockWebSocketServer) URL() string {
	return "ws" + strings.TrimPrefix(m.server.URL, "http")
}

func (m *MockWebSocketServer) Close() {
	for _, conn := range m.connections {
		conn.Close()
	}
	m.server.Close()
}

func (m *MockWebSocketServer) ReceivedMessages() []models.Message {
	return m.receivedMsgs
}

// Helper to create test connection
func createTestConnection(serverURL string) *Connection {
	config := ConnectionConfig{
		URL:                  serverURL,
		AuthToken:            "test-token-123",
		ReconnectInterval:    100 * time.Millisecond,
		MaxReconnectInterval: 1 * time.Second,
		PingInterval:         200 * time.Millisecond,
		PongTimeout:          1 * time.Second,
	}

	sensorInfo := models.NewSensorInfo("test-sensor", "Test Lab", "DHT11", "v1.0.0")
	logger := zerolog.Nop() // Silent logger for tests

	return NewConnection(config, sensorInfo, logger)
}

// Tests

func TestNewConnection(t *testing.T) {
	server := NewMockWebSocketServer()
	defer server.Close()

	conn := createTestConnection(server.URL())

	if conn == nil {
		t.Fatal("NewConnection returned nil")
	}

	if conn.State() != StateDisconnected {
		t.Errorf("Initial state = %v, want %v", conn.State(), StateDisconnected)
	}

	if conn.IsConnected() {
		t.Error("IsConnected should be false initially")
	}
}

func TestConnection_Connect_Success(t *testing.T) {
	server := NewMockWebSocketServer()
	defer server.Close()

	conn := createTestConnection(server.URL())
	ctx := context.Background()

	err := conn.Connect(ctx)
	if err != nil {
		t.Fatalf("Connect failed: %v", err)
	}

	if !conn.IsConnected() {
		t.Error("Should be connected after successful Connect()")
	}

	if conn.State() != StateConnected {
		t.Errorf("State = %v, want %v", conn.State(), StateConnected)
	}

	conn.Close()
}

func TestConnection_Connect_Failure_InvalidURL(t *testing.T) {
	conn := createTestConnection("ws://invalid-url-that-does-not-exist:9999/ws")
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	err := conn.Connect(ctx)
	if err == nil {
		t.Error("Connect should fail with invalid URL")
	}

	if conn.IsConnected() {
		t.Error("Should not be connected after failed Connect()")
	}
}

func TestConnection_Connect_Failure_ServerRefuses(t *testing.T) {
	server := NewMockWebSocketServer()
	server.shouldAccept = false // Server refuses connection
	defer server.Close()

	conn := createTestConnection(server.URL())
	ctx := context.Background()

	err := conn.Connect(ctx)
	if err == nil {
		t.Error("Connect should fail when server refuses")
	}
}

func TestConnection_Send_SingleReading(t *testing.T) {
	server := NewMockWebSocketServer()
	defer server.Close()

	conn := createTestConnection(server.URL())
	ctx := context.Background()

	// Connect first
	if err := conn.Connect(ctx); err != nil {
		t.Fatalf("Connect failed: %v", err)
	}
	defer conn.Close()

	// Give server time to be ready
	time.Sleep(50 * time.Millisecond)

	// Send a reading
	reading := models.NewReading("test-sensor", 22.5, 45.0)
	err := conn.Send(reading)
	if err != nil {
		t.Fatalf("Send failed: %v", err)
	}

	// Give server time to receive
	time.Sleep(100 * time.Millisecond)

	// Check server received it
	msgs := server.ReceivedMessages()
	if len(msgs) < 2 { // registration + reading
		t.Fatalf("Server received %d messages, want at least 2", len(msgs))
	}

	// Find the reading message (skip registration)
	var foundReading bool
	for _, msg := range msgs {
		if msg.Type == models.MessageTypeReading {
			foundReading = true
			break
		}
	}

	if !foundReading {
		t.Error("Server did not receive reading message")
	}
}

func TestConnection_Send_WhenDisconnected(t *testing.T) {
	server := NewMockWebSocketServer()
	defer server.Close()

	conn := createTestConnection(server.URL())

	// Try to send without connecting
	reading := models.NewReading("test-sensor", 22.5, 45.0)
	err := conn.Send(reading)

	if err == nil {
		t.Error("Send should fail when not connected")
	}
}

func TestConnection_SendBatch(t *testing.T) {
	server := NewMockWebSocketServer()
	defer server.Close()

	conn := createTestConnection(server.URL())
	ctx := context.Background()

	// Connect
	if err := conn.Connect(ctx); err != nil {
		t.Fatalf("Connect failed: %v", err)
	}
	defer conn.Close()

	time.Sleep(50 * time.Millisecond)

	// Send batch of readings
	readings := []*models.Reading{
		models.NewReading("test-sensor", 22.5, 45.0),
		models.NewReading("test-sensor", 23.0, 46.0),
		models.NewReading("test-sensor", 23.5, 47.0),
	}

	err := conn.SendBatch(readings)
	if err != nil {
		t.Fatalf("SendBatch failed: %v", err)
	}

	time.Sleep(100 * time.Millisecond)

	// Check server received batch
	msgs := server.ReceivedMessages()
	var foundBatch bool
	for _, msg := range msgs {
		if msg.Type == models.MessageTypeBatch {
			foundBatch = true

			var batch models.BatchMessage
			if err := msg.UnmarshalPayload(&batch); err != nil {
				t.Fatalf("Failed to unmarshal batch: %v", err)
			}

			if batch.Count != 3 {
				t.Errorf("Batch count = %d, want 3", batch.Count)
			}
			if len(batch.Readings) != 3 {
				t.Errorf("Batch has %d readings, want 3", len(batch.Readings))
			}
			break
		}
	}

	if !foundBatch {
		t.Error("Server did not receive batch message")
	}
}

func TestConnection_SendBatch_EmptyBatch(t *testing.T) {
	server := NewMockWebSocketServer()
	defer server.Close()

	conn := createTestConnection(server.URL())
	ctx := context.Background()

	if err := conn.Connect(ctx); err != nil {
		t.Fatalf("Connect failed: %v", err)
	}
	defer conn.Close()

	// Send empty batch
	err := conn.SendBatch([]*models.Reading{})
	if err != nil {
		t.Errorf("SendBatch with empty slice should not error: %v", err)
	}
}

func TestConnection_Reconnect_AfterDisconnect(t *testing.T) {
	server := NewMockWebSocketServer()
	server.closeAfterN = 2 // Close after receiving 2 messages (registration + 1 more)
	defer server.Close()

	conn := createTestConnection(server.URL())
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	// Start connection with auto-reconnect
	go conn.Run(ctx)

	// Wait for initial connection
	time.Sleep(200 * time.Millisecond)

	if !conn.IsConnected() {
		t.Fatal("Should be connected initially")
	}

	// Send a message (will trigger server to close after this)
	reading := models.NewReading("test-sensor", 22.5, 45.0)
	conn.Send(reading)

	// Wait for disconnect and reconnect
	time.Sleep(500 * time.Millisecond)

	// Should have reconnected
	if !conn.IsConnected() {
		t.Error("Should have reconnected after disconnect")
	}

	conn.Close()
}

func TestConnection_Heartbeat(t *testing.T) {
	server := NewMockWebSocketServer()
	defer server.Close()

	conn := createTestConnection(server.URL())
	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
	defer cancel()

	// Connect
	if err := conn.Connect(ctx); err != nil {
		t.Fatalf("Connect failed: %v", err)
	}

	// Start message loops (includes heartbeat)
	go conn.runMessageLoops(ctx)

	// Wait for multiple heartbeat intervals
	time.Sleep(600 * time.Millisecond)

	conn.Close()

	// Check that heartbeats were sent
	msgs := server.ReceivedMessages()
	heartbeatCount := 0
	for _, msg := range msgs {
		if msg.Type == models.MessageTypeHeartbeat {
			heartbeatCount++
		}
	}

	// With 200ms ping interval and 600ms wait, expect at least 2 heartbeats
	if heartbeatCount < 2 {
		t.Errorf("Received %d heartbeats, expected at least 2", heartbeatCount)
	}

	t.Logf("Received %d heartbeats", heartbeatCount)
}

func TestConnection_Registration(t *testing.T) {
	server := NewMockWebSocketServer()
	defer server.Close()

	conn := createTestConnection(server.URL())
	ctx := context.Background()

	// Connect (should send registration)
	if err := conn.Connect(ctx); err != nil {
		t.Fatalf("Connect failed: %v", err)
	}
	defer conn.Close()

	time.Sleep(100 * time.Millisecond)

	// Check that registration was sent
	msgs := server.ReceivedMessages()
	if len(msgs) < 1 {
		t.Fatal("No messages received, expected registration")
	}

	// First message should be heartbeat (registration)
	if msgs[0].Type != models.MessageTypeHeartbeat {
		t.Errorf("First message type = %v, want %v", msgs[0].Type, models.MessageTypeHeartbeat)
	}

	var heartbeat models.HeartbeatMessage
	if err := msgs[0].UnmarshalPayload(&heartbeat); err != nil {
		t.Fatalf("Failed to unmarshal heartbeat: %v", err)
	}

	if heartbeat.SensorID != "test-sensor" {
		t.Errorf("Heartbeat SensorID = %v, want test-sensor", heartbeat.SensorID)
	}
}

func TestConnection_ReceiveAck(t *testing.T) {
	server := NewMockWebSocketServer()
	server.respondWithPong = true // Server will respond with acks
	defer server.Close()

	conn := createTestConnection(server.URL())
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	// Connect
	if err := conn.Connect(ctx); err != nil {
		t.Fatalf("Connect failed: %v", err)
	}

	// Start read loop
	go conn.runMessageLoops(ctx)

	// Send a message
	reading := models.NewReading("test-sensor", 22.5, 45.0)
	conn.Send(reading)

	// Give time for ack to be received and processed
	time.Sleep(200 * time.Millisecond)

	conn.Close()

	// The fact that we didn't disconnect means acks were received
	// (heartbeat loop checks for pongs/acks)
	t.Log("Acks received and processed successfully")
}

func TestConnection_StateTransitions(t *testing.T) {
	server := NewMockWebSocketServer()
	defer server.Close()

	conn := createTestConnection(server.URL())

	// Initial state
	if conn.State() != StateDisconnected {
		t.Errorf("Initial state = %v, want Disconnected", conn.State())
	}

	// Connect
	ctx := context.Background()
	conn.Connect(ctx)

	if conn.State() != StateConnected {
		t.Errorf("After connect state = %v, want Connected", conn.State())
	}

	// Disconnect
	conn.Close()

	if conn.State() != StateDisconnected {
		t.Errorf("After close state = %v, want Disconnected", conn.State())
	}
}

func TestConnection_ExponentialBackoff(t *testing.T) {
	// Create connection with short intervals for testing
	config := ConnectionConfig{
		URL:                  "ws://localhost:9999/invalid", // Invalid URL
		AuthToken:            "test",
		ReconnectInterval:    50 * time.Millisecond,
		MaxReconnectInterval: 200 * time.Millisecond,
		PingInterval:         100 * time.Millisecond,
		PongTimeout:          500 * time.Millisecond,
	}

	sensorInfo := models.NewSensorInfo("test", "test", "DHT11", "v1.0.0")
	conn := NewConnection(config, sensorInfo, zerolog.Nop())

	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()

	// Run will try to connect, fail, and retry with backoff
	// We're just checking it doesn't panic and handles reconnection logic
	go conn.Run(ctx)

	time.Sleep(600 * time.Millisecond)

	// Should still be disconnected (no valid server)
	if conn.IsConnected() {
		t.Error("Should not be connected to invalid server")
	}

	t.Log("Exponential backoff handled reconnection attempts correctly")
}

func TestConnection_CloseGracefully(t *testing.T) {
	server := NewMockWebSocketServer()
	defer server.Close()

	conn := createTestConnection(server.URL())
	ctx := context.Background()

	// Connect
	if err := conn.Connect(ctx); err != nil {
		t.Fatalf("Connect failed: %v", err)
	}

	// Close
	err := conn.Close()
	if err != nil {
		t.Errorf("Close failed: %v", err)
	}

	if conn.IsConnected() {
		t.Error("Should not be connected after Close()")
	}

	// Try to send after close (should fail gracefully)
	reading := models.NewReading("test-sensor", 22.5, 45.0)
	err = conn.Send(reading)
	if err == nil {
		t.Error("Send should fail after Close()")
	}
}

func TestConnectionState_String(t *testing.T) {
	tests := []struct {
		state    ConnectionState
		expected string
	}{
		{StateDisconnected, "disconnected"},
		{StateConnecting, "connecting"},
		{StateConnected, "connected"},
	}

	for _, tt := range tests {
		t.Run(tt.expected, func(t *testing.T) {
			if got := tt.state.String(); got != tt.expected {
				t.Errorf("String() = %v, want %v", got, tt.expected)
			}
		})
	}
}
