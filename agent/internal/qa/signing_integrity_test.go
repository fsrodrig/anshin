package qa

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func TestSigningRetriesUseRealKeytool(t *testing.T) {
	if _, err := exec.LookPath("keytool"); err != nil {
		t.Skip("keytool unavailable")
	}
	f := newFixture(t)
	run := f.init(t)
	f.h.Runner = interceptRunner{next: f.emu, call: func(c Cmd) ([]byte, []byte, error, bool) {
		if c.Name == "keytool" {
			a, b, e := (ExecRunner{}).Run(context.Background(), c)
			return a, b, e, true
		}
		return nil, nil, nil, false
	}}
	for range 2 {
		if _, err := f.up(t, run); err != nil {
			t.Fatal(err)
		}
		if _, err := f.h.Down(context.Background(), run); err != nil {
			t.Fatal(err)
		}
	}
	entries, err := os.ReadDir(filepath.Join(run, "signing"))
	if err != nil || len(entries) != 2 {
		t.Fatalf("attempt evidence: %v %v", entries, err)
	}
}

func TestFailedSignatureVerificationBlocksInstallAndMetadata(t *testing.T) {
	for _, failure := range []string{"exit", "digest", "subcommand"} {
		t.Run(failure, func(t *testing.T) {
			f := newFixture(t)
			run := f.init(t)
			f.h.Runner = interceptRunner{next: f.emu, call: func(c Cmd) ([]byte, []byte, error, bool) {
				if filepath.Base(c.Name) == "apksigner" && c.Args[0] == "verify" {
					switch failure {
					case "exit":
						return nil, nil, errExit, true
					case "digest":
						return []byte("verified without digest"), nil, nil, true
					case "subcommand":
						c.Args = c.Args[1:]
						a, b, e := f.emu.Run(context.Background(), c)
						return a, b, e, true
					}
				}
				return nil, nil, nil, false
			}}
			if _, err := f.up(t, run); err == nil {
				t.Fatal("invalid verification succeeded")
			}
			if f.emu.installed {
				t.Fatal("installed unverified APK")
			}
			if _, err := os.Stat(filepath.Join(run, "build", "apk.json")); !os.IsNotExist(err) {
				t.Fatal("published verified metadata")
			}
		})
	}
}
