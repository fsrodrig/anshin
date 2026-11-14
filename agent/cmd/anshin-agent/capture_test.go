package main

import (
	"bytes"
	"path/filepath"
	"testing"
)

func TestCaptureRefusesBadUsage(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "c")
	for _, args := range [][]string{
		{"-name", "desktop"},
		{"-name", "desktop", dir, "extra"},
		{"-name", "desktop", "-bogus", dir},
		{"-name", "desktop", "-interval", "500ms", dir},
		{"-name", "desktop", "-identifiers", filepath.Join(t.TempDir(), "missing"), dir},
	} {
		var out, errOut bytes.Buffer
		if code := runCapture(args, &out, &errOut); code != 2 {
			t.Errorf("capture %q = exit %d, want 2", args, code)
		}
	}
}
