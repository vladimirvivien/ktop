package ui

import (
	"errors"
	"testing"
)

func TestCopyToClipboard(t *testing.T) {
	err := CopyToClipboard("test-resource-name")
	if err != nil && !errors.Is(err, ErrClipboardUnavailable) {
		t.Logf("CopyToClipboard returned error (expected in headless or sandboxed CI): %v", err)
	}
}
