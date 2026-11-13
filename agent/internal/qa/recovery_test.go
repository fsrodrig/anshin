package qa

import (
	"context"
	"strings"
	"testing"
)

func TestInstallRecovery(t *testing.T) {
	for _, failure := range []string{"inspection", "response"} {
		for _, replacement := range []bool{false, true} {
			t.Run(failure+map[bool]string{false: "-owned", true: "-replacement"}[replacement], func(t *testing.T) {
				f := newFixture(t)
				run := f.init(t)
				if failure == "inspection" {
					f.emu.fail["sha256sum '"] = errExit
				} else {
					f.h.Runner = interceptRunner{next: f.emu, call: func(c Cmd) ([]byte, []byte, error, bool) {
						if strings.Contains(strings.Join(c.Args, " "), " install ") {
							a, b, _ := f.emu.Run(context.Background(), c)
							return a, b, errExit, true
						}
						return nil, nil, nil, false
					}}
				}
				if _, err := f.up(t, run); err == nil {
					t.Fatal("expected interrupted install")
				}
				delete(f.emu.fail, "sha256sum '")
				f.h.Runner = f.emu
				if !f.emu.installed {
					t.Fatal("install did not happen")
				}
				if replacement {
					f.h.Runner = interceptRunner{next: f.emu, call: func(c Cmd) ([]byte, []byte, error, bool) {
						if strings.Contains(strings.Join(c.Args, " "), "sha256sum '") {
							return []byte(strings.Repeat("ef", 32)), nil, nil, true
						}
						return nil, nil, nil, false
					}}
				}
				for range 2 {
					_, err := f.h.Down(context.Background(), run)
					if replacement {
						if err == nil || !f.emu.installed || f.emu.marker == "" {
							t.Fatalf("replacement removed: %v", err)
						}
					} else if err != nil || f.emu.installed || f.emu.marker != "" {
						t.Fatalf("cleanup failed: %v", err)
					}
				}
			})
		}
	}
}
