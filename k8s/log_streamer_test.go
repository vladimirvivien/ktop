package k8s

import (
	"context"
	"io"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestLogStreamer_StreamsAndCancels(t *testing.T) {
	streamer := NewLogStreamer()
	done := make(chan struct{})
	var mu sync.Mutex
	var lines []string

	opener := func(ctx context.Context, ns, pod string, opts LogOptions) (io.ReadCloser, error) {
		return io.NopCloser(strings.NewReader("line-1\nline-2\nline-3\n")), nil
	}

	streamer.Start(context.Background(), opener, "default", "web-0", LogOptions{Container: "app"}, LogStreamCallbacks{
		OnLine: func(line string) {
			mu.Lock()
			lines = append(lines, line)
			mu.Unlock()
		},
		OnDone: func() { close(done) },
	})

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for log stream completion")
	}

	mu.Lock()
	defer mu.Unlock()
	if len(lines) != 3 || lines[0] != "line-1" || lines[2] != "line-3" {
		t.Fatalf("unexpected streamed lines: %v", lines)
	}

	// Verify Stop is safe on an idle streamer.
	streamer.Stop()
}
