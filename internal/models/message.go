package models

import (
	"encoding/json"
	"time"
)

// MessageType represents the type of WebSocket message
type MessageType string

const (
	MessageTypeReading   MessageType = "reading"
	MessageTypeBatch     MessageType = "batch"
	MessageTypeHeartbeat MessageType = "heartbeat"
	MessageTypeAck       MessageType = "ack"
	MessageTypeError     MessageType = "error"
	MessageTypeConfig    MessageType = "config"
)

// Message is the envelope for all WebSocket communications
type Message struct {
	Type      MessageType     `json:"type"`
	Payload   json.RawMessage `json:"payload"`
	Timestamp time.Time       `json:"timestamp"`
}

// NewMessage creates a new message with the given type and payload
func NewMessage(msgType MessageType, payload interface{}) (*Message, error) {
	payloadJSON, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}
	return &Message{
		Type:      msgType,
		Payload:   payloadJSON,
		Timestamp: time.Now(),
	}, nil
}

// ReadingMessage is the payload for MessageTypeReading
type ReadingMessage struct {
	Timestamp   time.Time `json:"timestamp"`
	SensorID    string    `json:"sensor_id"`
	Humidity    float64   `json:"humidity"`
	Temperature float64   `json:"temperature"`
}

// BatchMessage is the payload for MessageTypeBatch
type BatchMessage struct {
	Readings []Reading `json:"readings"`
	Count    int       `json:"count"`
}

// HeartbeatMessage is the payload for MessageTypeHeartbeat
type HeartbeatMessage struct {
	SensorID   string `json:"sensor_id"`
	Uptime     int64  `json:"uptime"`
	BufferSize int    `json:"buffer_size"`
}

// AckMessage is the payload for MessageTypeAck
type AckMessage struct {
	MessageID string `json:"message_id"`
	Status    string `json:"status"`
}

// ErrorMessage is the payload for MessageTypeError
type ErrorMessage struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

// ConfigMessage is the payload for MessageTypeConfig
type ConfigMessage struct {
	ReadInterval int    `json:"read_interval"`
	BufferSize   int    `json:"buffer_size"`
	SensorID     string `json:"sensor_id"`
}

// UnmarshalPayload unmarshals the message payload into the provided struct
func (m *Message) UnmarshalPayload(v interface{}) error {
	err := json.Unmarshal(m.Payload, v)
	if err != nil {
		return err
	}
	return nil
}
