package k8s

import (
	"bufio"
	"context"
	"errors"
	"io"
	"sync"
)

// LogStreamOpener opens a container log stream for the given pod and options.
type LogStreamOpener func(ctx context.Context, namespace, podName string, opts LogOptions) (io.ReadCloser, error)

// LogStreamCallbacks receives line, error, and completion notifications from LogStreamer.
type LogStreamCallbacks struct {
	OnLine  func(line string)
	OnError func(err error)
	OnDone  func()
}

// LogStreamer manages a single cancellable container log stream independently of any UI toolkit.
type LogStreamer struct {
	mu     sync.Mutex
	cancel context.CancelFunc
}

// NewLogStreamer constructs a reusable LogStreamer.
func NewLogStreamer() *LogStreamer {
	return &LogStreamer{}
}

// Start cancels any previously active stream and begins scanning lines from opener
// in a background goroutine until EOF, error, or Stop/parent context cancellation.
func (s *LogStreamer) Start(parent context.Context, opener LogStreamOpener, namespace, podName string, opts LogOptions, cb LogStreamCallbacks) {
	if parent == nil {
		parent = context.Background()
	}

	s.mu.Lock()
	if s.cancel != nil {
		s.cancel()
	}
	ctx, cancel := context.WithCancel(parent)
	s.cancel = cancel
	s.mu.Unlock()

	go func() {
		if opener == nil {
			if cb.OnError != nil {
				cb.OnError(errors.New("log stream function not configured"))
			}
			return
		}

		stream, err := opener(ctx, namespace, podName, opts)
		if err != nil {
			if ctx.Err() == nil && cb.OnError != nil {
				cb.OnError(err)
			}
			return
		}
		defer stream.Close()

		if cb.OnDone != nil {
			defer cb.OnDone()
		}

		scanner := bufio.NewScanner(stream)
		buf := make([]byte, 0, 64*1024)
		scanner.Buffer(buf, 1024*1024)

		for scanner.Scan() {
			select {
			case <-ctx.Done():
				return
			default:
				if cb.OnLine != nil {
					cb.OnLine(scanner.Text())
				}
			}
		}

		if err := scanner.Err(); err != nil && ctx.Err() == nil && cb.OnError != nil {
			cb.OnError(err)
		}
	}()
}

// Stop cancels the currently active log stream, if any.
func (s *LogStreamer) Stop() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.cancel != nil {
		s.cancel()
		s.cancel = nil
	}
}
