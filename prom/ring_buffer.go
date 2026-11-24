package prom

const ringInitialCap = 16

// RingBuffer provides a bounded circular buffer that grows lazily up to its
// maximum capacity. Once full, new additions overwrite the oldest entries
// without allocating.
type RingBuffer[T any] struct {
	data    []T
	maxSize int
	head    int // Next write position once full
	count   int // Current number of elements (0 to maxSize)
}

// NewRingBuffer creates a new ring buffer with the specified maximum capacity.
// Backing storage is allocated lazily on Add to avoid wasting memory on sparse
// or short-lived time series.
func NewRingBuffer[T any](size int) *RingBuffer[T] {
	if size <= 0 {
		size = 1
	}
	return &RingBuffer[T]{
		maxSize: size,
	}
}

// Add appends a value to the buffer.
// If the buffer is full, the oldest value is overwritten with zero allocations.
func (rb *RingBuffer[T]) Add(value T) {
	if rb.count < rb.maxSize {
		if rb.count == len(rb.data) {
			newCap := len(rb.data) * 2
			if newCap == 0 {
				newCap = min(rb.maxSize, ringInitialCap)
			} else if newCap > rb.maxSize {
				newCap = rb.maxSize
			}
			grown := make([]T, newCap)
			copy(grown, rb.data)
			rb.data = grown
		}
		rb.data[rb.count] = value
		rb.count++
		rb.head = rb.count % rb.maxSize
		return
	}
	rb.data[rb.head] = value
	rb.head = (rb.head + 1) % rb.maxSize
}

// Len returns the current number of elements in the buffer.
func (rb *RingBuffer[T]) Len() int {
	return rb.count
}

// Cap returns the maximum capacity of the buffer.
func (rb *RingBuffer[T]) Cap() int {
	return rb.maxSize
}

// AllocatedCap returns the currently allocated backing slice length.
func (rb *RingBuffer[T]) AllocatedCap() int {
	return len(rb.data)
}

// IsEmpty returns true if the buffer contains no elements.
func (rb *RingBuffer[T]) IsEmpty() bool {
	return rb.count == 0
}

// IsFull returns true if the buffer is at capacity.
func (rb *RingBuffer[T]) IsFull() bool {
	return rb.count == rb.maxSize
}

// Last returns the most recently added value.
// Returns the zero value and false if the buffer is empty.
func (rb *RingBuffer[T]) Last() (T, bool) {
	if rb.count == 0 {
		var zero T
		return zero, false
	}
	if rb.count < rb.maxSize {
		return rb.data[rb.count-1], true
	}
	// head points to next write position, so last written is head-1
	idx := (rb.head - 1 + rb.maxSize) % rb.maxSize
	return rb.data[idx], true
}

// First returns the oldest value in the buffer.
// Returns the zero value and false if the buffer is empty.
func (rb *RingBuffer[T]) First() (T, bool) {
	if rb.count == 0 {
		var zero T
		return zero, false
	}
	if rb.count < rb.maxSize {
		return rb.data[0], true
	}
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
	if rb.count < rb.maxSize {
		actualIdx = index
	} else {
		actualIdx = (rb.head + index) % rb.maxSize
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
	if rb.count < rb.maxSize {
		copy(result, rb.data[:rb.count])
	} else {
		tail := rb.maxSize - rb.head
		copy(result, rb.data[rb.head:])        // [head, maxSize) -> oldest portion
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
// The underlying storage is zeroed and retained for reuse.
func (rb *RingBuffer[T]) Clear() {
	clear(rb.data)
	rb.head = 0
	rb.count = 0
}
