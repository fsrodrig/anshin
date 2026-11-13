package probe

import (
	"bytes"
	"errors"
	"io/fs"
)

var ErrEmpty = errors.New("empty")

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
