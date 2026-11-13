package probe

import (
	"bytes"
	"errors"
	"io/fs"
)

// ErrEmpty is a file that exists but holds nothing usable.
var ErrEmpty = errors.New("empty")

// Hostname is the kernel's hostname, trimmed.
func Hostname(h Host) (string, error) {
	b, err := fs.ReadFile(h.FS, "proc/sys/kernel/hostname")
	if err != nil {
		return "", err
	}
	s := string(bytes.TrimSpace(b))
	if s == "" {
		return "", ErrEmpty
	}
	return s, nil
}
