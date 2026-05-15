package logging

import (
	"bytes"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestParseLevel(t *testing.T) {
	cases := []struct {
		in      string
		want    slog.Level
		wantErr bool
	}{
		{"", slog.LevelInfo, false},
		{"info", slog.LevelInfo, false},
		{"INFO", slog.LevelInfo, false},
		{"debug", slog.LevelDebug, false},
		{"warn", slog.LevelWarn, false},
		{"warning", slog.LevelWarn, false},
		{"error", slog.LevelError, false},
		{"trace", slog.LevelInfo, true},
	}
	for _, tc := range cases {
		t.Run(tc.in, func(t *testing.T) {
			got, err := parseLevel(tc.in)
			if (err != nil) != tc.wantErr {
				t.Fatalf("parseLevel(%q) err = %v, wantErr = %v", tc.in, err, tc.wantErr)
			}
			if got != tc.want {
				t.Errorf("parseLevel(%q) = %v, want %v", tc.in, got, tc.want)
			}
		})
	}
}

func TestMakeHandler_FormatSelection(t *testing.T) {
	var buf bytes.Buffer

	text, err := makeHandler(&buf, "text", slog.LevelInfo)
	if err != nil {
		t.Fatalf("text handler error: %v", err)
	}
	if _, ok := text.(*slog.TextHandler); !ok {
		t.Errorf("text format produced %T, want *slog.TextHandler", text)
	}

	json, err := makeHandler(&buf, "json", slog.LevelInfo)
	if err != nil {
		t.Fatalf("json handler error: %v", err)
	}
	if _, ok := json.(*slog.JSONHandler); !ok {
		t.Errorf("json format produced %T, want *slog.JSONHandler", json)
	}

	if _, err := makeHandler(&buf, "yaml", slog.LevelInfo); err == nil {
		t.Errorf("makeHandler(yaml) = nil error, want error")
	}
}

func TestInit_WritesToFile(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("KTOP_DIR", dir)

	closer, err := Init(Config{Level: "debug", Format: "text"})
	if err != nil {
		t.Fatalf("Init error: %v", err)
	}
	defer func() { _ = closer.Close() }()

	slog.Info("test record", "key", "value")

	// lumberjack writes synchronously; close to flush before reading.
	if err := closer.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}

	path := filepath.Join(dir, LogFileName)
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read log: %v", err)
	}
	if !strings.Contains(string(data), "test record") {
		t.Errorf("log file did not contain expected record; got: %s", data)
	}

	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat: %v", err)
	}
	if mode := info.Mode().Perm(); mode != 0o600 {
		t.Errorf("log file mode = %v, want 0600", mode)
	}
}

func TestInit_StderrDestination(t *testing.T) {
	// Redirect stderr so we can capture it.
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("pipe: %v", err)
	}
	oldStderr := os.Stderr
	os.Stderr = w
	t.Cleanup(func() { os.Stderr = oldStderr })

	closer, err := Init(Config{Level: "info", Format: "text", Dest: DestStderr})
	if err != nil {
		t.Fatalf("Init error: %v", err)
	}
	defer func() { _ = closer.Close() }()

	slog.Info("stderr record")
	_ = w.Close()

	captured, _ := io.ReadAll(r)
	if !strings.Contains(string(captured), "stderr record") {
		t.Errorf("stderr did not contain expected record; got: %s", captured)
	}
}
