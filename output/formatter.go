package output

import (
	"fmt"
	"io"
	"strings"

	"github.com/vladimirvivien/ktop/collector"
	"github.com/vladimirvivien/ktop/internal/snapshot"
)

// Formatter serializes headless collection samples and final aggregates.
type Formatter interface {
	WriteSample(w io.Writer, snap *snapshot.Snapshot) error
	WriteFinal(w io.Writer, agg *collector.Aggregate) error
}

// NewFormatter constructs a Formatter for the requested format ("json" or "jsonl").
func NewFormatter(format string) (Formatter, error) {
	switch strings.ToLower(strings.TrimSpace(format)) {
	case "", "json":
		return &JSONFormatter{}, nil
	case "jsonl":
		return &JSONLFormatter{}, nil
	default:
		return nil, fmt.Errorf("invalid output format %q (valid: json, jsonl)", format)
	}
}
