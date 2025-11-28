package prom

import (
	"testing"
)

func TestNewRingBuffer(t *testing.T) {
	rb := NewRingBuffer[int](5)
	if rb.Len() != 0 {
		t.Errorf("expected Len() = 0, got %d", rb.Len())
	}
	if rb.Cap() != 5 {
		t.Errorf("expected Cap() = 5, got %d", rb.Cap())
	}
	if !rb.IsEmpty() {
		t.Error("expected IsEmpty() = true")
	}
	if rb.IsFull() {
		t.Error("expected IsFull() = false")
	}
}

func TestNewRingBufferZeroSize(t *testing.T) {
	rb := NewRingBuffer[int](0)
	if rb.Cap() != 1 {
		t.Errorf("expected Cap() = 1 for zero size, got %d", rb.Cap())
	}
}

func TestRingBufferAdd(t *testing.T) {
	rb := NewRingBuffer[int](3)

	rb.Add(1)
	if rb.Len() != 1 {
		t.Errorf("expected Len() = 1, got %d", rb.Len())
	}

	rb.Add(2)
	rb.Add(3)
	if rb.Len() != 3 {
		t.Errorf("expected Len() = 3, got %d", rb.Len())
	}
	if !rb.IsFull() {
		t.Error("expected IsFull() = true")
	}

	// Add one more - should overwrite oldest
	rb.Add(4)
	if rb.Len() != 3 {
		t.Errorf("expected Len() = 3 after overflow, got %d", rb.Len())
	}
}

func TestRingBufferFirstLast(t *testing.T) {
	rb := NewRingBuffer[int](3)

	// Empty buffer
	_, ok := rb.First()
	if ok {
		t.Error("expected First() to return false for empty buffer")
	}
	_, ok = rb.Last()
	if ok {
		t.Error("expected Last() to return false for empty buffer")
	}

	// Add some values
	rb.Add(1)
	rb.Add(2)
	rb.Add(3)

	first, ok := rb.First()
	if !ok || first != 1 {
		t.Errorf("expected First() = 1, got %d", first)
	}

	last, ok := rb.Last()
	if !ok || last != 3 {
		t.Errorf("expected Last() = 3, got %d", last)
	}

	// Overflow - oldest should now be 2
	rb.Add(4)

	first, ok = rb.First()
	if !ok || first != 2 {
		t.Errorf("expected First() = 2 after overflow, got %d", first)
	}

	last, ok = rb.Last()
	if !ok || last != 4 {
		t.Errorf("expected Last() = 4 after overflow, got %d", last)
	}
}

func TestRingBufferGet(t *testing.T) {
	rb := NewRingBuffer[int](3)

	// Empty buffer
	_, ok := rb.Get(0)
	if ok {
		t.Error("expected Get(0) to return false for empty buffer")
	}

	rb.Add(1)
	rb.Add(2)
	rb.Add(3)

	// Valid indices
	for i, expected := range []int{1, 2, 3} {
		val, ok := rb.Get(i)
		if !ok {
			t.Errorf("expected Get(%d) to return true", i)
		}
		if val != expected {
			t.Errorf("expected Get(%d) = %d, got %d", i, expected, val)
		}
	}

	// Out of bounds
	_, ok = rb.Get(-1)
	if ok {
		t.Error("expected Get(-1) to return false")
	}
	_, ok = rb.Get(3)
	if ok {
		t.Error("expected Get(3) to return false")
	}

	// After overflow
	rb.Add(4)
	rb.Add(5)

	expected := []int{3, 4, 5}
	for i, exp := range expected {
		val, ok := rb.Get(i)
		if !ok || val != exp {
			t.Errorf("after overflow: expected Get(%d) = %d, got %d", i, exp, val)
		}
	}
}

func TestRingBufferSlice(t *testing.T) {
	rb := NewRingBuffer[int](5)

	// Empty buffer
	slice := rb.Slice()
	if slice != nil {
		t.Errorf("expected nil slice for empty buffer, got %v", slice)
	}

	// Partially filled
	rb.Add(1)
	rb.Add(2)
	rb.Add(3)

	slice = rb.Slice()
	expected := []int{1, 2, 3}
	if !sliceEqual(slice, expected) {
		t.Errorf("expected %v, got %v", expected, slice)
	}

	// Full buffer
	rb.Add(4)
	rb.Add(5)

	slice = rb.Slice()
	expected = []int{1, 2, 3, 4, 5}
	if !sliceEqual(slice, expected) {
		t.Errorf("expected %v, got %v", expected, slice)
	}

	// After overflow
	rb.Add(6)
	rb.Add(7)

	slice = rb.Slice()
	expected = []int{3, 4, 5, 6, 7}
	if !sliceEqual(slice, expected) {
		t.Errorf("after overflow: expected %v, got %v", expected, slice)
	}
}

func TestRingBufferSliceFrom(t *testing.T) {
	rb := NewRingBuffer[int](5)
	rb.Add(1)
	rb.Add(2)
	rb.Add(3)
	rb.Add(4)
	rb.Add(5)

	// From middle
	slice := rb.SliceFrom(2)
	expected := []int{3, 4, 5}
	if !sliceEqual(slice, expected) {
		t.Errorf("expected %v, got %v", expected, slice)
	}

	// From start
	slice = rb.SliceFrom(0)
	expected = []int{1, 2, 3, 4, 5}
	if !sliceEqual(slice, expected) {
		t.Errorf("expected %v, got %v", expected, slice)
	}

	// From last
	slice = rb.SliceFrom(4)
	expected = []int{5}
	if !sliceEqual(slice, expected) {
		t.Errorf("expected %v, got %v", expected, slice)
	}

	// Out of bounds
	slice = rb.SliceFrom(5)
	if slice != nil {
		t.Errorf("expected nil for out of bounds, got %v", slice)
	}

	slice = rb.SliceFrom(-1)
	if slice != nil {
		t.Errorf("expected nil for negative index, got %v", slice)
	}
}

func TestRingBufferRange(t *testing.T) {
	rb := NewRingBuffer[int](3)
	rb.Add(10)
	rb.Add(20)
	rb.Add(30)

	var collected []int
	rb.Range(func(index int, value int) bool {
		collected = append(collected, value)
		return true
	})

	expected := []int{10, 20, 30}
	if !sliceEqual(collected, expected) {
		t.Errorf("expected %v, got %v", expected, collected)
	}

	// Test early termination
	collected = nil
	rb.Range(func(index int, value int) bool {
		collected = append(collected, value)
		return index < 1 // Stop after index 1
	})

	expected = []int{10, 20}
	if !sliceEqual(collected, expected) {
		t.Errorf("expected early termination at %v, got %v", expected, collected)
	}
}

func TestRingBufferClear(t *testing.T) {
	rb := NewRingBuffer[int](3)
	rb.Add(1)
	rb.Add(2)
	rb.Add(3)

	rb.Clear()

	if rb.Len() != 0 {
		t.Errorf("expected Len() = 0 after Clear(), got %d", rb.Len())
	}
	if !rb.IsEmpty() {
		t.Error("expected IsEmpty() = true after Clear()")
	}

	// Should be able to add again
	rb.Add(100)
	val, ok := rb.Last()
	if !ok || val != 100 {
		t.Errorf("expected Last() = 100 after re-add, got %d", val)
	}
}

func TestRingBufferWithMetricSample(t *testing.T) {
	rb := NewRingBuffer[MetricSample](3)

	rb.Add(MetricSample{Timestamp: 1000, Value: 1.0})
	rb.Add(MetricSample{Timestamp: 2000, Value: 2.0})
	rb.Add(MetricSample{Timestamp: 3000, Value: 3.0})

	last, ok := rb.Last()
	if !ok {
		t.Fatal("expected Last() to return true")
	}
	if last.Timestamp != 3000 || last.Value != 3.0 {
		t.Errorf("expected {3000, 3.0}, got {%d, %f}", last.Timestamp, last.Value)
	}

	// Overflow
	rb.Add(MetricSample{Timestamp: 4000, Value: 4.0})

	first, ok := rb.First()
	if !ok {
		t.Fatal("expected First() to return true")
	}
	if first.Timestamp != 2000 || first.Value != 2.0 {
		t.Errorf("expected {2000, 2.0}, got {%d, %f}", first.Timestamp, first.Value)
	}

	slice := rb.Slice()
	if len(slice) != 3 {
		t.Fatalf("expected 3 samples, got %d", len(slice))
	}
	expectedTimestamps := []int64{2000, 3000, 4000}
	for i, s := range slice {
		if s.Timestamp != expectedTimestamps[i] {
			t.Errorf("slice[%d].Timestamp = %d, expected %d", i, s.Timestamp, expectedTimestamps[i])
		}
	}
}

func TestRingBufferWrapAround(t *testing.T) {
	// Test multiple wrap-arounds to ensure consistency
	rb := NewRingBuffer[int](3)

	// Fill and overflow multiple times
	for i := 1; i <= 10; i++ {
		rb.Add(i)
	}

	// Should have last 3 values: 8, 9, 10
	slice := rb.Slice()
	expected := []int{8, 9, 10}
	if !sliceEqual(slice, expected) {
		t.Errorf("after 10 adds to size-3 buffer: expected %v, got %v", expected, slice)
	}

	first, _ := rb.First()
	if first != 8 {
		t.Errorf("expected First() = 8, got %d", first)
	}

	last, _ := rb.Last()
	if last != 10 {
		t.Errorf("expected Last() = 10, got %d", last)
	}
}

// Helper function to compare slices
func sliceEqual[T comparable](a, b []T) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
