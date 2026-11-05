package overview

import (
	"slices"
	"strings"
	"testing"

	"github.com/vladimirvivien/ktop/config"
	"github.com/vladimirvivien/ktop/internal/userdir"
	"github.com/vladimirvivien/ktop/ui"
)

func TestNodePanel_SetVisibleColumns(t *testing.T) {
	panel := NewNodePanel(nil, " Nodes ").(*nodePanel)
	initialCols := []string{"NAME", "STATUS", "CPU", "MEM"}
	panel.DrawHeader(initialCols)
	panel.DrawBody(testNodeFixtures())

	if !slices.Equal(panel.GetVisibleColumns(), initialCols) {
		t.Fatalf("expected visible columns %v, got %v", initialCols, panel.GetVisibleColumns())
	}

	newCols := []string{"NAME", "DISK"}
	panel.SetVisibleColumns(newCols)

	if !slices.Equal(panel.GetVisibleColumns(), newCols) {
		t.Fatalf("expected visible columns %v, got %v", newCols, panel.GetVisibleColumns())
	}

	ts, err := ui.NewTestScreen(100, 10)
	if err != nil {
		t.Fatalf("NewTestScreen failed: %v", err)
	}
	t.Cleanup(ts.Close)

	ts.Draw(panel.GetRootView())
	text := ts.Text()

	if !strings.Contains(text, "DISK") {
		t.Fatalf("expected DISK column in rendered output after SetVisibleColumns, got:\n%s", text)
	}
	if strings.Contains(text, "MEM") || strings.Contains(text, "CPU") {
		t.Fatalf("did not expect MEM or CPU in rendered output, got:\n%s", text)
	}
}

func TestPodPanel_SetVisibleColumns(t *testing.T) {
	panel := NewPodPanel(nil, " Pods ").(*podPanel)
	initialCols := []string{"NAMESPACE", "POD", "READY", "STATUS"}
	panel.DrawHeader(initialCols)
	panel.DrawBody(testPodFixtures())

	if !slices.Equal(panel.GetVisibleColumns(), initialCols) {
		t.Fatalf("expected visible columns %v, got %v", initialCols, panel.GetVisibleColumns())
	}

	newCols := []string{"NAMESPACE", "POD", "NODE"}
	panel.SetVisibleColumns(newCols)

	if !slices.Equal(panel.GetVisibleColumns(), newCols) {
		t.Fatalf("expected visible columns %v, got %v", newCols, panel.GetVisibleColumns())
	}

	ts, err := ui.NewTestScreen(100, 10)
	if err != nil {
		t.Fatalf("NewTestScreen failed: %v", err)
	}
	t.Cleanup(ts.Close)

	ts.Draw(panel.GetRootView())
	text := ts.Text()

	if !strings.Contains(text, "NODE") {
		t.Fatalf("expected NODE column in rendered output after SetVisibleColumns, got:\n%s", text)
	}
	if strings.Contains(text, "READY") {
		t.Fatalf("did not expect READY in rendered output, got:\n%s", text)
	}
}

func TestNodePanel_ConfigureColumnsTrigger(t *testing.T) {
	panel := NewNodePanel(nil, " Nodes ").(*nodePanel)
	panel.DrawHeader([]string{"NAME", "STATUS", "CPU"})

	triggered := false
	panel.SetOnConfigureColumns(func() {
		triggered = true
	})

	root := panel.GetRootView()
	ts, err := ui.NewTestScreen(100, 10)
	if err != nil {
		t.Fatalf("NewTestScreen failed: %v", err)
	}
	t.Cleanup(ts.Close)

	ts.Draw(root)

	// Pressing lowercase 'c' should sort, not trigger column config
	ts.SendRune(root, 'c')
	if triggered {
		t.Fatalf("lowercase 'c' should not trigger configure columns")
	}

	// Pressing '=' should trigger configure columns
	ts.SendRune(root, '=')
	if !triggered {
		t.Fatalf("'=' should trigger configure columns")
	}
}

func TestPodPanel_ConfigureColumnsTrigger(t *testing.T) {
	panel := NewPodPanel(nil, " Pods ").(*podPanel)
	panel.DrawHeader([]string{"NAMESPACE", "POD", "CPU"})

	triggered := false
	panel.SetOnConfigureColumns(func() {
		triggered = true
	})

	root := panel.GetRootView()
	ts, err := ui.NewTestScreen(100, 10)
	if err != nil {
		t.Fatalf("NewTestScreen failed: %v", err)
	}
	t.Cleanup(ts.Close)

	ts.Draw(root)

	// Pressing lowercase 'c' should sort, not trigger column config
	ts.SendRune(root, 'c')
	if triggered {
		t.Fatalf("lowercase 'c' should not trigger configure columns")
	}

	// Pressing '=' should trigger configure columns
	ts.SendRune(root, '=')
	if !triggered {
		t.Fatalf("'=' should trigger configure columns")
	}
}

func TestColumnSelection_PersistenceIntegration(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv(userdir.EnvVar, tmpDir)

	selectedNodeCols := []string{"NAME", "STATUS", "CPU", "MEM"}
	selectedPodCols := []string{"NAMESPACE", "POD", "AGE"}

	if err := config.SaveOverviewColumns(selectedNodeCols, selectedPodCols); err != nil {
		t.Fatalf("SaveOverviewColumns failed: %v", err)
	}

	loaded, err := config.LoadAppConfig()
	if err != nil || loaded == nil {
		t.Fatalf("LoadAppConfig failed: %v", err)
	}

	if !slices.Equal(loaded.UI.Overview.NodeCols, selectedNodeCols) {
		t.Errorf("persisted node columns mismatch: expected %v, got %v", selectedNodeCols, loaded.UI.Overview.NodeCols)
	}
	if !slices.Equal(loaded.UI.Overview.PodCols, selectedPodCols) {
		t.Errorf("persisted pod columns mismatch: expected %v, got %v", selectedPodCols, loaded.UI.Overview.PodCols)
	}
}

func TestFilterColumns_OrderPreserved(t *testing.T) {
	all := []string{"NAME", "STATUS", "RST", "PODS", "DISK", "CPU", "MEM"}
	filter := []string{"CPU", "NAME", "DISK"}

	result := filterColumns(all, filter)
	expected := []string{"CPU", "NAME", "DISK"}

	if !slices.Equal(result, expected) {
		t.Fatalf("filterColumns did not preserve specified order: got %v, expected %v", result, expected)
	}
}

func TestMainPanel_DefaultColumns(t *testing.T) {
	mp := NewWithColumnOptions(nil, "Overview", false, nil, nil)
	mp.Layout(nil)

	np, ok := mp.nodePanel.(*nodePanel)
	if !ok {
		t.Fatalf("expected *nodePanel")
	}
	if !slices.Equal(np.GetVisibleColumns(), config.DefaultNodeColumns) {
		t.Fatalf("expected default node columns %v, got %v", config.DefaultNodeColumns, np.GetVisibleColumns())
	}

	pp, ok := mp.podPanel.(*podPanel)
	if !ok {
		t.Fatalf("expected *podPanel")
	}
	if !slices.Equal(pp.GetVisibleColumns(), config.DefaultPodColumns) {
		t.Fatalf("expected default pod columns %v, got %v", config.DefaultPodColumns, pp.GetVisibleColumns())
	}
}

func TestMainPanel_ShowAllColumns(t *testing.T) {
	mp := NewWithColumnOptions(nil, "Overview", true, nil, nil)
	mp.Layout(nil)

	np, ok := mp.nodePanel.(*nodePanel)
	if !ok {
		t.Fatalf("expected *nodePanel")
	}
	if !slices.Equal(np.GetVisibleColumns(), config.AllNodeColumns) {
		t.Fatalf("expected all node columns %v, got %v", config.AllNodeColumns, np.GetVisibleColumns())
	}

	pp, ok := mp.podPanel.(*podPanel)
	if !ok {
		t.Fatalf("expected *podPanel")
	}
	if !slices.Equal(pp.GetVisibleColumns(), config.AllPodColumns) {
		t.Fatalf("expected all pod columns %v, got %v", config.AllPodColumns, pp.GetVisibleColumns())
	}
}
