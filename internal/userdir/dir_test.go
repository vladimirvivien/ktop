package userdir

import (
	"os"
	"path/filepath"
	"testing"
)

func TestPath_EnvOverride(t *testing.T) {
	want := filepath.Join(t.TempDir(), "custom-ktop")
	t.Setenv(EnvVar, want)

	got, err := Path()
	if err != nil {
		t.Fatalf("Path() error: %v", err)
	}
	if got != want {
		t.Errorf("Path() = %q, want %q", got, want)
	}
}

func TestPath_DefaultsToHome(t *testing.T) {
	t.Setenv(EnvVar, "")

	home, err := os.UserHomeDir()
	if err != nil {
		t.Skipf("user home dir unavailable: %v", err)
	}
	want := filepath.Join(home, ".ktop")

	got, err := Path()
	if err != nil {
		t.Fatalf("Path() error: %v", err)
	}
	if got != want {
		t.Errorf("Path() = %q, want %q", got, want)
	}
}

func TestEnsure_CreatesDirectoryWithMode0700(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "ktop")
	t.Setenv(EnvVar, dir)

	got, err := Ensure()
	if err != nil {
		t.Fatalf("Ensure() error: %v", err)
	}
	if got != dir {
		t.Errorf("Ensure() returned %q, want %q", got, dir)
	}

	info, err := os.Stat(dir)
	if err != nil {
		t.Fatalf("stat %s: %v", dir, err)
	}
	if !info.IsDir() {
		t.Fatalf("%s is not a directory", dir)
	}
	if mode := info.Mode().Perm(); mode != 0o700 {
		t.Errorf("dir mode = %v, want 0700", mode)
	}
}

func TestEnsure_IsIdempotent(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "ktop")
	t.Setenv(EnvVar, dir)

	if _, err := Ensure(); err != nil {
		t.Fatalf("first Ensure() error: %v", err)
	}
	if _, err := Ensure(); err != nil {
		t.Fatalf("second Ensure() error: %v", err)
	}
}
