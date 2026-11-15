package output

import (
	"encoding/json"
	"io"

	"github.com/vladimirvivien/ktop/collector"
	"github.com/vladimirvivien/ktop/internal/snapshot"
)

// JSONLFormatter streams one minified JSON Snapshot line per interval sample.
type JSONLFormatter struct{}

// WriteSample encodes snap as a single JSON line and flushes buffered writers.
func (f *JSONLFormatter) WriteSample(w io.Writer, snap *snapshot.Snapshot) error {
	enc := json.NewEncoder(w)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(snap); err != nil {
		return err
	}
	flushWriter(w)
	return nil
}

// WriteFinal is a no-op because each sample was already emitted by WriteSample.
func (f *JSONLFormatter) WriteFinal(io.Writer, *collector.Aggregate) error {
	return nil
}

func flushWriter(w io.Writer) {
	switch f := w.(type) {
	case interface{ Flush() error }:
		_ = f.Flush()
	case interface{ Flush() }:
		f.Flush()
	}
}
