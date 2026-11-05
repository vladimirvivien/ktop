package application

import (
	"sync"
	"testing"
	"time"
)

func TestApplicationRefreshNonBlockingCoalescing(t *testing.T) {
	app := &Application{
		refreshQ: make(chan struct{}, 1),
	}

	// 1. Initial refresh queues 1 item
	app.Refresh()
	if len(app.refreshQ) != 1 {
		t.Fatalf("expected 1 item in refreshQ, got %d", len(app.refreshQ))
	}

	// 2. Subsequent rapid refreshes do not block when channel is full and are coalesced
	done := make(chan struct{})
	go func() {
		for i := 0; i < 100; i++ {
			app.Refresh()
		}
		close(done)
	}()

	select {
	case <-done:
		// Succeeded without blocking
	case <-time.After(time.Second):
		t.Fatal("app.Refresh() blocked when refreshQ was full")
	}

	// 3. Drain the single queued event
	select {
	case <-app.refreshQ:
	default:
		t.Fatal("expected an event in refreshQ")
	}

	// 4. Channel is empty now
	if len(app.refreshQ) != 0 {
		t.Fatalf("expected 0 items in refreshQ after drain, got %d", len(app.refreshQ))
	}

	// 5. Concurrent Refresh calls from multiple goroutines never deadlock
	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 50; j++ {
				app.Refresh()
			}
		}()
	}
	wg.Wait()
}
