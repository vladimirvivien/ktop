// Package logging configures the process-wide structured logger and routes
// records to a rotating file under the user's ktop directory.
//
// The TUI cannot write to stdout/stderr without corrupting its display, so
// diagnostic logs go to ~/.ktop/ktop.log by default. Init wires this once
// at startup; the rest of the program calls slog through the default logger.
package logging

import (
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"

	"github.com/vladimirvivien/ktop/internal/userdir"
	"gopkg.in/natefinch/lumberjack.v2"
)

const (
	// LogFileName is the rotating log file name inside the ktop directory.
	LogFileName = "ktop.log"

	logFileMode = 0o600

	// DefaultMaxSize is the size in MB at which the log rotates.
	DefaultMaxSize = 10
	// DefaultMaxBackups is the number of rotated log files retained.
	DefaultMaxBackups = 3
)

// Destination selects where log records are written.
type Destination string

const (
	// DestFile routes logs to ~/.ktop/ktop.log (the default).
	DestFile Destination = "file"
	// DestStderr routes logs to standard error.
	DestStderr Destination = "stderr"
)

// Config controls log level, format, and destination.
type Config struct {
	Level    string      // "debug" | "info" | "warn" | "error"; empty = info
	Format   string      // "text" | "json"; empty = text
	Dest     Destination // DestFile (default) or DestStderr
	Filename string      // override the default file path; ignored when Dest is stderr
}

// Init configures slog's default logger from cfg and returns an io.Closer that
// the caller should close on shutdown. The returned closer is always non-nil
// even on error, so callers can defer Close() unconditionally.
func Init(cfg Config) (io.Closer, error) {
	level, err := parseLevel(cfg.Level)
	if err != nil {
		return nopCloser{}, err
	}

	writer, closer, err := openSink(cfg)
	if err != nil {
		return nopCloser{}, err
	}

	handler, err := makeHandler(writer, cfg.Format, level)
	if err != nil {
		return closer, err
	}
	slog.SetDefault(slog.New(handler))
	return closer, nil
}

func parseLevel(s string) (slog.Level, error) {
	switch strings.ToLower(s) {
	case "", "info":
		return slog.LevelInfo, nil
	case "debug":
		return slog.LevelDebug, nil
	case "warn", "warning":
		return slog.LevelWarn, nil
	case "error":
		return slog.LevelError, nil
	default:
		return slog.LevelInfo, fmt.Errorf("unknown log level: %s", s)
	}
}

func openSink(cfg Config) (io.Writer, io.Closer, error) {
	if cfg.Dest == DestStderr {
		return os.Stderr, nopCloser{}, nil
	}

	path := cfg.Filename
	if path == "" {
		dir, err := userdir.Ensure()
		if err != nil {
			return nil, nopCloser{}, err
		}
		path = filepath.Join(dir, LogFileName)
	}

	// Touch the file ourselves so it inherits 0600; lumberjack reuses
	// the existing inode and preserves the permission bits.
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, logFileMode)
	if err != nil {
		return nil, nopCloser{}, fmt.Errorf("open log file %s: %w", path, err)
	}
	_ = f.Close()

	lj := &lumberjack.Logger{
		Filename:   path,
		MaxSize:    DefaultMaxSize,
		MaxBackups: DefaultMaxBackups,
		Compress:   false,
	}
	return lj, lj, nil
}

func makeHandler(w io.Writer, format string, level slog.Level) (slog.Handler, error) {
	opts := &slog.HandlerOptions{Level: level}
	switch strings.ToLower(format) {
	case "", "text":
		return slog.NewTextHandler(w, opts), nil
	case "json":
		return slog.NewJSONHandler(w, opts), nil
	default:
		return nil, fmt.Errorf("unknown log format: %s", format)
	}
}

type nopCloser struct{}

func (nopCloser) Close() error { return nil }
