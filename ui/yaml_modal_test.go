package ui

import (
	"strings"
	"testing"

	"github.com/gdamore/tcell/v2"
	appsv1 "k8s.io/api/apps/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestFormatObjectYAML_StripsManagedFieldsAndPreservesTypeMeta(t *testing.T) {
	dep := &appsv1.Deployment{
		TypeMeta: metav1.TypeMeta{APIVersion: "apps/v1", Kind: "Deployment"},
		ObjectMeta: metav1.ObjectMeta{
			Name:      "api-server",
			Namespace: "load-test",
			ManagedFields: []metav1.ManagedFieldsEntry{
				{Manager: "kubectl-client-side-apply"},
			},
		},
	}

	out, err := FormatObjectYAML(dep)
	if err != nil {
		t.Fatalf("FormatObjectYAML failed: %v", err)
	}
	for _, want := range []string{"apiVersion: apps/v1", "kind: Deployment", "name: api-server", "namespace: load-test"} {
		if !strings.Contains(out, want) {
			t.Fatalf("expected %q in YAML output, got:\n%s", want, out)
		}
	}
	if strings.Contains(out, "managedFields") {
		t.Fatalf("expected managedFields stripped from YAML output, got:\n%s", out)
	}
	if len(dep.ManagedFields) != 1 {
		t.Fatalf("expected source object ManagedFields untouched, got %d", len(dep.ManagedFields))
	}
}

func TestYAMLModal_RenderScrollEscapeAndMonochrome(t *testing.T) {
	t.Cleanup(func() {
		_, _ = ResolveAndApplyTheme("dark", "truecolor")
	})

	sample := "apiVersion: apps/v1\nkind: Deployment\nmetadata:\n  name: api-server\n  annotations:\n    json: '{\"items\": [1, 2]}'\n"
	closed := false
	modal := NewYAMLModal(YAMLModalConfig{
		Title:   "Deployment YAML: api-server",
		Content: sample,
		OnClose: func() { closed = true },
	})

	ts, err := NewTestScreen(100, 24)
	if err != nil {
		t.Fatalf("NewTestScreen failed: %v", err)
	}
	t.Cleanup(ts.Close)

	ts.Draw(modal.GetRootView())
	rendered := ts.Text()
	for _, want := range []string{"Deployment YAML: api-server", "apiVersion: apps/v1", "kind: Deployment", "[1, 2]"} {
		if !strings.Contains(rendered, want) {
			t.Fatalf("expected %q in rendered YAML modal, got:\n%s", want, rendered)
		}
	}

	modal.HandleInput(tcell.NewEventKey(tcell.KeyEscape, 0, tcell.ModNone))
	if !closed {
		t.Fatal("expected ESC to close YAML modal")
	}

	// Verify monochrome emits zero non-default colors.
	if _, err := ResolveAndApplyTheme("monochrome", "none"); err != nil {
		t.Fatalf("ResolveAndApplyTheme failed: %v", err)
	}
	monoModal := NewYAMLModal(YAMLModalConfig{
		Title:   "Deployment YAML: api-server",
		Content: sample,
	})
	ts.Draw(monoModal.GetRootView())
	cells, w, h := ts.Screen().GetContents()
	for i := 0; i < w*h; i++ {
		fg, bg, _ := cells[i].Style.Decompose()
		if fg != tcell.ColorDefault || bg != tcell.ColorDefault {
			t.Fatalf("cell %d emitted non-default monochrome colors fg=%v bg=%v", i, fg, bg)
		}
	}
}
