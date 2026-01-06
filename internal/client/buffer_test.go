package client

import (
	"sync"
	"testing"
	"time"

	"github.com/afroash/dht-monitor/internal/models"
)

func TestNewReadingBuffer(t *testing.T) {
	buf := NewReadingBuffer(100, true)

	if buf == nil {
		t.Fatal("NewReadingBuffer returned nil")
	}
	if buf.Capacity() != 100 {
		t.Errorf("Capacity = %d, want 100", buf.Capacity())
	}
	if buf.Size() != 0 {
		t.Errorf("Initial size = %d, want 0", buf.Size())
	}
	if !buf.IsEmpty() {
		t.Error("New buffer should be empty")
	}
}

func TestBuffer_PushAndSize(t *testing.T) {
	buf := NewReadingBuffer(10, true)

	reading := models.NewReading("sensor-01", 22.5, 45.0)

	ok := buf.Push(reading)
	if !ok {
		t.Error("Push failed on empty buffer")
	}

	if buf.Size() != 1 {
		t.Errorf("Size = %d, want 1", buf.Size())
	}

	if buf.IsEmpty() {
		t.Error("Buffer should not be empty after push")
	}
}

func TestBuffer_PopBatch(t *testing.T) {
	buf := NewReadingBuffer(10, true)

	// Push 5 readings
	for i := 0; i < 5; i++ {
		reading := models.NewReading("sensor-01", float64(20+i), float64(40+i))
		buf.Push(reading)
	}

	// Pop 3
	readings := buf.PopBatch(3)

	if len(readings) != 3 {
		t.Errorf("PopBatch(3) returned %d readings, want 3", len(readings))
	}

	if buf.Size() != 2 {
		t.Errorf("Size after pop = %d, want 2", buf.Size())
	}

	// Verify FIFO order (oldest first)
	if readings[0].Temperature != 20.0 {
		t.Errorf("First popped temp = %v, want 20.0", readings[0].Temperature)
	}
	if readings[2].Temperature != 22.0 {
		t.Errorf("Third popped temp = %v, want 22.0", readings[2].Temperature)
	}
}

func TestBuffer_PopBatch_MoreThanAvailable(t *testing.T) {
	buf := NewReadingBuffer(10, true)

	// Push 3 readings
	for i := 0; i < 3; i++ {
		buf.Push(models.NewReading("sensor-01", 22.0, 45.0))
	}

	// Try to pop 10 (more than available)
	readings := buf.PopBatch(10)

	if len(readings) != 3 {
		t.Errorf("PopBatch(10) with 3 available returned %d, want 3", len(readings))
	}

	if !buf.IsEmpty() {
		t.Error("Buffer should be empty after popping all")
	}
}

func TestBuffer_Peek(t *testing.T) {
	buf := NewReadingBuffer(10, true)

	// Push 5 readings
	for i := 0; i < 5; i++ {
		buf.Push(models.NewReading("sensor-01", float64(20+i), 45.0))
	}

	// Peek at 3
	readings := buf.Peek(3)

	if len(readings) != 3 {
		t.Errorf("Peek(3) returned %d readings, want 3", len(readings))
	}

	// Buffer size should NOT change
	if buf.Size() != 5 {
		t.Errorf("Size after peek = %d, want 5 (unchanged)", buf.Size())
	}

	// Should get oldest first
	if readings[0].Temperature != 20.0 {
		t.Errorf("First peeked temp = %v, want 20.0", readings[0].Temperature)
	}
}

func TestBuffer_DropOldest(t *testing.T) {
	buf := NewReadingBuffer(3, true) // capacity 3, drop oldest

	// Fill buffer
	for i := 0; i < 3; i++ {
		buf.Push(models.NewReading("sensor-01", float64(20+i), 45.0))
	}

	if !buf.IsFull() {
		t.Error("Buffer should be full")
	}

	// Push one more (should drop oldest)
	buf.Push(models.NewReading("sensor-01", 99.0, 45.0))

	// Should still be full
	if !buf.IsFull() {
		t.Error("Buffer should still be full")
	}

	// Check that oldest was dropped
	readings := buf.PopBatch(3)

	// First reading should now be 21.0 (original 20.0 was dropped)
	if readings[0].Temperature != 21.0 {
		t.Errorf("After drop-oldest, first temp = %v, want 21.0", readings[0].Temperature)
	}

	// Last should be the new one
	if readings[2].Temperature != 99.0 {
		t.Errorf("After drop-oldest, last temp = %v, want 99.0", readings[2].Temperature)
	}
}

func TestBuffer_DropNewest(t *testing.T) {
	buf := NewReadingBuffer(3, false) // capacity 3, drop newest

	// Fill buffer
	for i := 0; i < 3; i++ {
		buf.Push(models.NewReading("sensor-01", float64(20+i), 45.0))
	}

	// Push one more (should be dropped)
	ok := buf.Push(models.NewReading("sensor-01", 99.0, 45.0))

	if ok {
		t.Error("Push should return false when buffer full and drop-newest")
	}

	// Buffer should still have original 3
	readings := buf.PopBatch(3)

	// Should still have original readings, newest was dropped
	if readings[2].Temperature != 22.0 {
		t.Errorf("Last temp = %v, want 22.0 (99.0 should be dropped)", readings[2].Temperature)
	}
}

func TestBuffer_Clear(t *testing.T) {
	buf := NewReadingBuffer(10, true)

	// Add some readings
	for i := 0; i < 5; i++ {
		buf.Push(models.NewReading("sensor-01", 22.0, 45.0))
	}

	buf.Clear()

	if !buf.IsEmpty() {
		t.Error("Buffer should be empty after Clear()")
	}
	if buf.Size() != 0 {
		t.Errorf("Size after clear = %d, want 0", buf.Size())
	}
}

func TestBuffer_Stats(t *testing.T) {
	buf := NewReadingBuffer(3, true)

	// Push 5 readings (will drop 2)
	for i := 0; i < 5; i++ {
		buf.Push(models.NewReading("sensor-01", 22.0, 45.0))
	}

	stats := buf.Stats()

	if stats.TotalPushed != 5 {
		t.Errorf("TotalPushed = %d, want 5", stats.TotalPushed)
	}

	if stats.TotalDropped != 2 {
		t.Errorf("TotalDropped = %d, want 2", stats.TotalDropped)
	}

	if stats.HighWaterMark != 3 {
		t.Errorf("HighWaterMark = %d, want 3", stats.HighWaterMark)
	}

	if stats.LastPushTime.IsZero() {
		t.Error("LastPushTime should be set")
	}
}

func TestBuffer_ThreadSafety(t *testing.T) {
	buf := NewReadingBuffer(1000, true)

	var wg sync.WaitGroup

	// Concurrent pushers
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			for j := 0; j < 100; j++ {
				reading := models.NewReading("sensor-01", float64(id*100+j), 45.0)
				buf.Push(reading)
			}
		}(i)
	}

	// Concurrent poppers
	for i := 0; i < 5; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 50; j++ {
				buf.PopBatch(10)
				time.Sleep(1 * time.Millisecond)
			}
		}()
	}

	// Concurrent readers
	for i := 0; i < 5; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 100; j++ {
				buf.Size()
				buf.IsEmpty()
				buf.IsFull()
				buf.Stats()
			}
		}()
	}

	wg.Wait()

	// No race conditions should occur
	// Run with: go test -race ./internal/client/...
	t.Logf("Final buffer state: %s", buf.String())
}

func TestBuffer_FIFO_Order(t *testing.T) {
	buf := NewReadingBuffer(100, true)

	// Push readings with sequential temperatures
	for i := 0; i < 10; i++ {
		reading := models.NewReading("sensor-01", float64(i), 45.0)
		buf.Push(reading)
	}

	// Pop all and verify order
	readings := buf.PopBatch(10)

	for i, reading := range readings {
		if reading.Temperature != float64(i) {
			t.Errorf("Reading %d has temp %v, want %v (FIFO order broken)",
				i, reading.Temperature, float64(i))
		}
	}
}

func BenchmarkBuffer_Push(b *testing.B) {
	buf := NewReadingBuffer(10000, true)
	reading := models.NewReading("sensor-01", 22.5, 45.0)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		buf.Push(reading)
	}
}

func BenchmarkBuffer_PopBatch(b *testing.B) {
	buf := NewReadingBuffer(10000, true)

	// Pre-fill buffer
	for i := 0; i < 10000; i++ {
		buf.Push(models.NewReading("sensor-01", 22.5, 45.0))
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		buf.PopBatch(100)
	}
}
