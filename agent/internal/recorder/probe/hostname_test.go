package probe

import (
	"errors"
	"io/fs"
	"testing"
)

func TestHostnameTrimsTheNewline(t *testing.T) {
	got, err := Hostname(host(map[string]string{"proc/sys/kernel/hostname": "machine-a\n"}))
	if err != nil || got != "machine-a" {
		t.Fatalf("Hostname = %q, %v; want machine-a", got, err)
	}
}

func TestHostnameUnavailable(t *testing.T) {
	if _, err := Hostname(host(map[string]string{"proc/sys/kernel/hostname": " \n"})); !errors.Is(err, ErrEmpty) {
		t.Fatalf("blank: err = %v, want ErrEmpty", err)
	}
	if _, err := Hostname(host(nil)); !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("missing: err = %v, want ErrNotExist", err)
	}
}
