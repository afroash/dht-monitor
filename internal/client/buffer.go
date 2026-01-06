package client

import (
	"fmt"
	"sync"
	"time"

	"github.com/afroash/dht-monitor/internal/models"
)

// ReadingBuffer is a thread-safe circular buffer for sensor readings
type ReadingBuffer struct {
	// TODO: Add fields:
	// - readings ([]*models.Reading) - slice to hold readings
	// - capacity (int) - max number of readings
	// - dropOldest (bool) - true = drop oldest when full, false = drop newest
	// - mutex (sync.RWMutex) - for thread safety
	// - stats (*BufferStats) - statistics tracking
	readings   []*models.Reading
	capacity   int
	dropOldest bool
	mutex      sync.RWMutex
	stats      BufferStats
}

// BufferStats tracks buffer usage statistics
type BufferStats struct {
	TotalPushed   int64
	TotalDropped  int64
	HighWaterMark int
	LastPushTime  time.Time
	LastDropTime  time.Time
}

// NewReadingBuffer creates a new reading buffer with given capacity
func NewReadingBuffer(capacity int, dropOldest bool) *ReadingBuffer {
	// TODO: Implement
	// - Create buffer with empty slice (or pre-allocated slice of capacity)
	// - Initialize stats
	// - Return buffer
	//
	// Tip: Pre-allocate with make([]*models.Reading, 0, capacity)
	return &ReadingBuffer{
		readings:   make([]*models.Reading, 0, capacity),
		capacity:   capacity,
		dropOldest: dropOldest,
		stats:      BufferStats{},
	}
}

// Push adds a reading to the buffer
// Returns true if successful, false if dropped (when full and dropOldest=false)
func (rb *ReadingBuffer) Push(reading *models.Reading) bool {
	rb.mutex.Lock()
	defer rb.mutex.Unlock()

	if len(rb.readings) >= rb.capacity {
		if rb.dropOldest {
			rb.readings = rb.readings[1:]
			rb.stats.TotalDropped++
			rb.stats.LastDropTime = time.Now()
		} else {
			rb.stats.TotalDropped++
			rb.stats.LastDropTime = time.Now()
			return false
		}
	}
	rb.readings = append(rb.readings, reading)
	rb.stats.TotalPushed++
	rb.stats.LastPushTime = time.Now()

	if len(rb.readings) > rb.stats.HighWaterMark {
		rb.stats.HighWaterMark = len(rb.readings)
	}

	return true
}

// PopBatch removes and returns up to n readings from the buffer
// Returns slice of readings (may be less than n if buffer has fewer)
// Removes from the front (oldest first)
func (rb *ReadingBuffer) PopBatch(n int) []*models.Reading {

	rb.mutex.Lock()
	defer rb.mutex.Unlock()

	count := min(n, len(rb.readings))
	if count == 0 {
		return nil
	}
	result := make([]*models.Reading, count)
	copy(result, rb.readings[:count])
	rb.readings = rb.readings[count:]
	return result
}

// Peek returns up to n readings without removing them
// Useful for inspecting buffer contents
func (rb *ReadingBuffer) Peek(n int) []*models.Reading {

	rb.mutex.RLock()
	defer rb.mutex.RUnlock()

	count := min(n, len(rb.readings))
	if count == 0 {
		return nil
	}

	result := make([]*models.Reading, count)
	copy(result, rb.readings[:count])
	return result
}

// Size returns the current number of readings in the buffer
func (rb *ReadingBuffer) Size() int {
	rb.mutex.RLock()
	defer rb.mutex.RUnlock()
	return len(rb.readings)
}

// IsFull returns true if buffer is at capacity
func (rb *ReadingBuffer) IsFull() bool {
	rb.mutex.RLock()
	defer rb.mutex.RUnlock()
	return len(rb.readings) >= rb.capacity
}

// IsEmpty returns true if buffer has no readings
func (rb *ReadingBuffer) IsEmpty() bool {
	rb.mutex.RLock()
	defer rb.mutex.RUnlock()
	return len(rb.readings) == 0
}

// Clear removes all readings from the buffer
func (rb *ReadingBuffer) Clear() {

	rb.mutex.Lock()
	defer rb.mutex.Unlock()
	rb.readings = make([]*models.Reading, 0)
	rb.stats.TotalPushed = 0
	rb.stats.TotalDropped = 0
	rb.stats.LastPushTime = time.Time{}
	rb.stats.LastDropTime = time.Time{}
}

// Capacity returns the maximum capacity of the buffer
func (rb *ReadingBuffer) Capacity() int {
	// No lock needed, capacity doesn't change
	return rb.capacity
}

// Stats returns a copy of current buffer statistics
func (rb *ReadingBuffer) Stats() BufferStats {
	rb.mutex.RLock()
	defer rb.mutex.RUnlock()
	return BufferStats{
		TotalPushed:   rb.stats.TotalPushed,
		TotalDropped:  rb.stats.TotalDropped,
		HighWaterMark: rb.stats.HighWaterMark,
		LastPushTime:  rb.stats.LastPushTime,
		LastDropTime:  rb.stats.LastDropTime,
	}
}

// String returns a human-readable representation of buffer state
func (rb *ReadingBuffer) String() string {
	// TODO: Implement
	// Return something like: "Buffer[12/1000, dropped: 5, mode: drop-oldest]"
	rb.mutex.RLock()
	defer rb.mutex.RUnlock()

	mode := "drop-newest"
	if rb.dropOldest {
		mode = "drop-oldest"
	}

	return fmt.Sprintf("Buffer[%d/%d, dropped: %d, mode: %s]",
		len(rb.readings),
		rb.capacity,
		rb.stats.TotalDropped,
		mode,
	)
}

// Helper function for min (Go doesn't have built-in min for ints in older versions)
func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
