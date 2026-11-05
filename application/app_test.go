package application

import (
	"sync"
	"testing"
	"time"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
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

func TestApplication_ModalPageLifecycle(t *testing.T) {
	tapp := tview.NewApplication()
	panel := newPanel(tapp)
	panel.root = tview.NewPages()
	panel.root.AddPage("main", tview.NewBox(), true, true)

	app := &Application{
		tviewApp: tapp,
		panel:    panel,
	}

	if app.HasActiveModalPage() {
		t.Fatalf("expected no active modal page initially")
	}

	modalBox := tview.NewBox()
	app.ShowModalPage("test-modal", modalBox)

	if !app.HasActiveModalPage() {
		t.Fatalf("expected active modal page after ShowModalPage")
	}

	app.DismissModalPage("test-modal")

	if app.HasActiveModalPage() {
		t.Fatalf("expected no active modal page after DismissModalPage")
	}
}

func TestApplication_ModalPageEscapeNotInterrupted(t *testing.T) {
	tapp := tview.NewApplication()
	panel := newPanel(tapp)
	panel.root = tview.NewPages()
	panel.root.AddPage("main", tview.NewBox(), true, true)

	app := &Application{
		tviewApp: tapp,
		panel:    panel,
	}

	modalBox := tview.NewBox()
	app.ShowModalPage("col-select", modalBox)

	// Build the input capture function as in Start()
	inputCapture := func(event *tcell.EventKey) *tcell.EventKey {
		if app.HasActiveToast() {
			if event.Key() == tcell.KeyEsc {
				return nil
			}
			return event
		}

		if app.HasActiveModalPage() {
			return event
		}

		if event.Key() == tcell.KeyEsc {
			app.pendingQuit = true
			return nil
		}
		return event
	}

	// Pressing ESC while modal is active should pass the event through (not trigger quit)
	evEsc := tcell.NewEventKey(tcell.KeyEsc, 0, tcell.ModNone)
	res := inputCapture(evEsc)

	if res == nil {
		t.Fatalf("expected ESC event to be passed through to modal, got nil (consumed)")
	}
	if app.pendingQuit {
		t.Fatalf("expected pendingQuit to remain false when modal is active")
	}
}
