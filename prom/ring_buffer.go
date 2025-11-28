package prom

// RingBuffer provides a fixed-size, allocation-free circular buffer for storing samples.
// Once full, new additions overwrite the oldest entries automatically.
// This is more memory-efficient than bounded slices as it avoids reallocations
// and GC pressure from slice trimming operations.
type RingBuffer[T any] struct {
	data  []T
	size  int
	head  int // Next write position
	count int // Current number of elements (0 to size)
}

// NewRingBuffer creates a new ring buffer with the specified capacity.
// The buffer is pre-allocated to avoid allocations during normal operation.
func NewRingBuffer[T any](size int) *RingBuffer[T] {
	if size <= 0 {
		size = 1
	}
	return &RingBuffer[T]{
		data: make([]T, size),
		size: size,
	}
}

// Add appends a value to the buffer.
// If the buffer is full, the oldest value is overwritten.
// This operation is O(1) and does not allocate.
func (rb *RingBuffer[T]) Add(value T) {
	rb.data[rb.head] = value
	rb.head = (rb.head + 1) % rb.size
	if rb.count < rb.size {
		rb.count++
	}
}

// Len returns the current number of elements in the buffer.
func (rb *RingBuffer[T]) Len() int {
	return rb.count
}

// Cap returns the maximum capacity of the buffer.
func (rb *RingBuffer[T]) Cap() int {
	return rb.size
}

// IsEmpty returns true if the buffer contains no elements.
func (rb *RingBuffer[T]) IsEmpty() bool {
	return rb.count == 0
}

// IsFull returns true if the buffer is at capacity.
func (rb *RingBuffer[T]) IsFull() bool {
	return rb.count == rb.size
}

// Last returns the most recently added value.
// Returns the zero value and false if the buffer is empty.
func (rb *RingBuffer[T]) Last() (T, bool) {
	if rb.count == 0 {
		var zero T
		return zero, false
	}
	// head points to next write position, so last written is head-1
	idx := (rb.head - 1 + rb.size) % rb.size
	return rb.data[idx], true
}

// First returns the oldest value in the buffer.
// Returns the zero value and false if the buffer is empty.
func (rb *RingBuffer[T]) First() (T, bool) {
	if rb.count == 0 {
		var zero T
		return zero, false
	}
	if rb.count < rb.size {
		// Not full yet - oldest is at index 0
		return rb.data[0], true
	}
	// Full - oldest is at head (next to be overwritten)
	return rb.data[rb.head], true
}

// Get returns the element at the given index (0 = oldest, Len()-1 = newest).
// Returns the zero value and false if index is out of bounds.
func (rb *RingBuffer[T]) Get(index int) (T, bool) {
	if index < 0 || index >= rb.count {
		var zero T
		return zero, false
	}

	var actualIdx int
	if rb.count < rb.size {
		// Not full - data starts at 0
		actualIdx = index
	} else {
		// Full - oldest is at head
		actualIdx = (rb.head + index) % rb.size
	}
	return rb.data[actualIdx], true
}

// Slice returns all values in chronological order (oldest first).
// This allocates a new slice - use iterator methods for zero-allocation access.
func (rb *RingBuffer[T]) Slice() []T {
	if rb.count == 0 {
		return nil
	}

	result := make([]T, rb.count)
	if rb.count < rb.size {
		// Not full yet - data is contiguous from 0
		copy(result, rb.data[:rb.count])
	} else {
		// Full - data wraps around
		// Oldest is at head, newest is at head-1
		tail := rb.size - rb.head
		copy(result, rb.data[rb.head:])        // [head, size) -> oldest portion
		copy(result[tail:], rb.data[:rb.head]) // [0, head) -> newest portion
	}
	return result
}

// SliceFrom returns values from the given index to the end (newest).
// Index 0 = oldest. This allocates a new slice.
func (rb *RingBuffer[T]) SliceFrom(startIndex int) []T {
	if startIndex < 0 || startIndex >= rb.count {
		return nil
	}

	length := rb.count - startIndex
	result := make([]T, length)

	for i := 0; i < length; i++ {
		val, _ := rb.Get(startIndex + i)
		result[i] = val
	}
	return result
}

// Range iterates over all elements in chronological order.
// The callback receives the index (0 = oldest) and value.
// Return false from the callback to stop iteration early.
func (rb *RingBuffer[T]) Range(fn func(index int, value T) bool) {
	for i := 0; i < rb.count; i++ {
		val, _ := rb.Get(i)
		if !fn(i, val) {
			return
		}
	}
}

// Clear removes all elements from the buffer.
// The underlying storage is retained for reuse.
func (rb *RingBuffer[T]) Clear() {
	rb.head = 0
	rb.count = 0
}
