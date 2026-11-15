package output

import (
	"encoding/json"
	"io"

	"github.com/vladimirvivien/ktop/collector"
	"github.com/vladimirvivien/ktop/internal/snapshot"
)

// JSONFormatter emits a single pretty-printed JSON report document once
// collection finishes.
type JSONFormatter struct{}

// WriteSample is a no-op for single-document JSON output.
func (f *JSONFormatter) WriteSample(io.Writer, *snapshot.Snapshot) error {
	return nil
}

// WriteFinal encodes agg as indented JSON onto w.
func (f *JSONFormatter) WriteFinal(w io.Writer, agg *collector.Aggregate) error {
	enc := json.NewEncoder(w)
	enc.SetEscapeHTML(false)
	enc.SetIndent("", "  ")
	return enc.Encode(agg)
}
