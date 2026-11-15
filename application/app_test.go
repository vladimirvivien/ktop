package application

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
	"github.com/vladimirvivien/ktop/k8s"
	"github.com/vladimirvivien/ktop/ui"
	"k8s.io/cli-runtime/pkg/genericclioptions"
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

func TestApplication_ShowHelpModal(t *testing.T) {
	tapp := tview.NewApplication()
	panel := newPanel(tapp)
	panel.root = tview.NewPages()
	panel.root.AddPage("main", tview.NewBox(), true, true)

	app := &Application{
		tviewApp: tapp,
		panel:    panel,
		navStack: NewNavigationStack(),
	}

	if app.HasActiveModalPage() {
		t.Fatalf("expected no active modal page initially")
	}

	app.ShowHelpModal()

	if !app.HasActiveModalPage() {
		t.Fatalf("expected active modal page after ShowHelpModal")
	}

	app.DismissModalPage(helpModalID)

	if app.HasActiveModalPage() {
		t.Fatalf("expected no active modal page after DismissModalPage")
	}
}

func TestApplication_HeaderAllNamespacesNotClipped(t *testing.T) {
	t.Cleanup(func() {
		_, _ = ui.ResolveAndApplyTheme("dark", "truecolor")
	})

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"major":"1","minor":"35","gitVersion":"v1.35.0"}`)
	}))
	defer srv.Close()

	kcPath := filepath.Join(t.TempDir(), "kubeconfig")
	kcYAML := fmt.Sprintf(`apiVersion: v1
kind: Config
clusters:
- cluster:
    server: %s
  name: kind-kind
contexts:
- context:
    cluster: kind-kind
    user: kind-kind
  name: kind-kind
current-context: kind-kind
users:
- name: kind-kind
  user: {}
`, srv.URL)
	if err := os.WriteFile(kcPath, []byte(kcYAML), 0o600); err != nil {
		t.Fatalf("WriteFile kubeconfig failed: %v", err)
	}

	flags := genericclioptions.NewConfigFlags(false)
	allNS := k8s.AllNamespaces
	flags.KubeConfig = &kcPath
	flags.Namespace = &allNS
	client, err := k8s.New(flags)
	if err != nil {
		t.Fatalf("k8s.New failed: %v", err)
	}

	for _, tc := range []struct {
		theme string
		mode  string
	}{
		{"light", "truecolor"},
		{"monochrome", "none"},
	} {
		t.Run(tc.theme, func(t *testing.T) {
			if _, err := ui.ResolveAndApplyTheme(tc.theme, tc.mode); err != nil {
				t.Fatalf("ResolveAndApplyTheme failed: %v", err)
			}

			tapp := tview.NewApplication()
			panel := newPanel(tapp)
			panel.header = tview.NewTable()
			panel.header.SetBorders(false)
			panel.header.SetBorder(true)
			ui.StyleTable(panel.header)

			app := &Application{
				tviewApp:  tapp,
				panel:     panel,
				k8sClient: client,
				navStack:  NewNavigationStack(),
			}

			hdr := app.buildHeaderString(app.getNamespaceDisplay())
			panel.DrawHeader(hdr)

			ts, err := ui.NewTestScreen(160, 5)
			if err != nil {
				t.Fatalf("NewTestScreen failed: %v", err)
			}
			defer ts.Close()

			ts.Draw(panel.header)
			rendered := ts.Text()
			if !strings.Contains(rendered, "Namespace: (all)") {
				t.Fatalf("expected unclipped 'Namespace: (all)' in %s header, got:\n%s", tc.theme, rendered)
			}
			if strings.Contains(rendered, "(all...") {
				t.Fatalf("unexpected '(all...' truncation in %s header, got:\n%s", tc.theme, rendered)
			}
		})
	}
}
