package replay_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"testing/fstest"
	"time"

	"github.com/fsrodrig/anshin/agent/internal/capture"
	"github.com/fsrodrig/anshin/agent/internal/capture/capturetest"
	"github.com/fsrodrig/anshin/agent/internal/capture/replay"
	"github.com/fsrodrig/anshin/agent/internal/recorder/probe"
)

func captured(t *testing.T, edit func(*capturetest.Machine)) string {
	t.Helper()
	m := capturetest.NewMachine()
	if edit != nil {
		edit(m)
	}
	t.Cleanup(m.Release)
	dir := filepath.Join(t.TempDir(), "desktop")
	opt := capture.Options{Name: "desktop", Snapshots: 2, Interval: time.Millisecond, StatfsTimeout: 100 * time.Millisecond}
	if err := capture.Run(context.Background(), m.Source(), dir, opt, &bytes.Buffer{}); err != nil {
		t.Fatal(err)
	}
	return dir
}

func TestReplayMatchesTheCapturedMachine(t *testing.T) {
	dir := captured(t, func(m *capturetest.Machine) {
		for p := range m.FS {
			if strings.HasPrefix(p, "sys/class/power_supply/") {
				delete(m.FS, p)
			}
		}
		m.FS["sys/class/power_supply"] = &fstest.MapFile{Mode: fs.ModeDir | 0o555}
	})
	c, err := replay.Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	if c.Name != "desktop" || len(c.Snapshots) != 2 || *c.Manifest.CPUsOnline != 2 {
		t.Fatalf("capture = %s with %d snapshots", c.Name, len(c.Snapshots))
	}
	h := c.Snapshots[0].Host()

	if got, err := probe.Hostname(h); err != nil || got != "desktop" {
		t.Errorf("Hostname = %q, %v", got, err)
	}
	stat, err := os.ReadFile(filepath.Join(dir, "0", "proc", "stat"))
	if err != nil {
		t.Fatal(err)
	}
	want, _ := probe.CPU(probe.Host{FS: fstest.MapFS{"proc/stat": {Data: stat}}})
	if got, err := probe.CPU(h); err != nil || got != want {
		t.Errorf("CPU = %+v, %v; want %+v", got, err, want)
	}
	if b, err := fs.ReadFile(h.FS, "sys/class/hwmon/hwmon0/temp1_input"); err != nil || string(b) != "45000\n" {
		t.Errorf("k10temp temp1_input = %q, %v", b, err)
	}
	if b, err := fs.ReadFile(h.FS, "etc/os-release"); err != nil || !strings.HasPrefix(string(b), "NAME=Fake") {
		t.Errorf("os-release through its link = %q, %v", b, err)
	}

	target, err := fs.ReadLink(h.FS, "sys/class/hwmon/hwmon2/device")
	if err != nil || path.Base(target) != "thermal_zone0" {
		t.Errorf("ReadLink(hwmon2/device) = %q, %v; want basename thermal_zone0", target, err)
	}
	if fi, err := fs.Lstat(h.FS, "sys/class/hwmon/hwmon0"); err != nil || fi.Mode()&fs.ModeSymlink == 0 {
		t.Errorf("Lstat(hwmon0) = %v, %v; want a symlink", fi, err)
	}
	entries, err := fs.ReadDir(h.FS, "sys/class/hwmon")
	if err != nil || len(entries) != 6 {
		t.Fatalf("ReadDir(hwmon) = %d entries, %v", len(entries), err)
	}
	for _, e := range entries {
		if e.Type()&fs.ModeSymlink == 0 || e.IsDir() {
			t.Errorf("hwmon entry %s has type %v; on a real Machine it is a symlink", e.Name(), e.Type())
		}
	}
	if fi, err := fs.Stat(h.FS, "sys/class/net/wlan0/device"); err != nil || !fi.IsDir() {
		t.Errorf("Stat(wlan0/device) = %v, %v; want the device to exist as a directory", fi, err)
	}
	if _, err := fs.Stat(h.FS, "sys/class/net/lo/device"); !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("Stat(lo/device) = %v, want not exist", err)
	}
	if supplies, err := fs.ReadDir(h.FS, "sys/class/power_supply"); err != nil || len(supplies) != 0 {
		t.Errorf("ReadDir(power_supply) = %v, %v; want an empty directory", supplies, err)
	}
	if _, err := fs.ReadFile(h.FS, "sys/class/hwmon/hwmon5/temp2_input"); !errors.Is(err, syscall.ENODATA) {
		t.Errorf("reading the sensor that wasn't ready = %v, want ENODATA", err)
	}
	if _, err := fs.ReadFile(h.FS, "proc/cpuinfo"); !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("proc/cpuinfo = %v, want not exist", err)
	}

	if st, err := h.Statfs("/"); err != nil || st.Blocks != 100 || st.Bavail != 30 || st.Type != 0xef53 {
		t.Errorf("Statfs(/) = %+v, %v", st, err)
	}
	if _, err := h.Statfs("/mnt/denied"); !errors.Is(err, syscall.EACCES) {
		t.Errorf("Statfs(/mnt/denied) = %v, want EACCES", err)
	}
	if _, err := h.Statfs("/mnt/slow"); !errors.Is(err, capture.ErrStatfsTimeout) {
		t.Errorf("Statfs(/mnt/slow) = %v, want ErrStatfsTimeout", err)
	}
	for _, mp := range []string{"/mnt/auto", "/nowhere"} {
		if _, err := h.Statfs(mp); !errors.Is(err, replay.ErrNotCaptured) {
			t.Errorf("Statfs(%s) = %v, want ErrNotCaptured", mp, err)
		}
	}
}

func TestSequenceStepsThroughSnapshotsAndRecordsAccess(t *testing.T) {
	c, err := replay.Load(captured(t, nil))
	if err != nil {
		t.Fatal(err)
	}
	seq := c.Sequence()
	h := seq.Host()
	first, _ := probe.CPU(h)
	if seq.Clock() != c.Snapshots[0].Clock {
		t.Error("a new Sequence is not at snapshot 0")
	}
	if !seq.Advance() {
		t.Fatal("Advance = false with a second snapshot left")
	}
	second, _ := probe.CPU(h)
	if second.Total() <= first.Total() {
		t.Errorf("the same Host did not move to snapshot 1: %d then %d jiffies", first.Total(), second.Total())
	}
	if seq.Clock() != c.Snapshots[1].Clock || seq.Advance() {
		t.Error("Sequence did not stop at the last snapshot")
	}
	h.Statfs("/")
	opened, calls := strings.Join(seq.Opened(), " "), strings.Join(seq.StatfsCalls(), " ")
	if !strings.Contains(opened, "proc/stat") || calls != "/" {
		t.Errorf("Opened = %q, StatfsCalls = %q", opened, calls)
	}
}

func TestLoadRejects(t *testing.T) {
	for _, tc := range []struct {
		name    string
		breakIt func(t *testing.T, dir string)
	}{
		{"a stray top-level file", func(t *testing.T, dir string) { write(t, dir, "notes.txt", "x") }},
		{"a snapshot directory past the manifest", func(t *testing.T, dir string) { write(t, dir, "2/index", "") }},
		{"a file missing from the index", func(t *testing.T, dir string) { write(t, dir, "0/proc/cpuinfo", "x") }},
		{"an on-disk symlink", func(t *testing.T, dir string) {
			if err := os.Symlink("stat", filepath.Join(dir, "0", "proc", "stat2")); err != nil {
				t.Fatal(err)
			}
		}},
		{"format 2", func(t *testing.T, dir string) { editManifest(t, dir, func(m *capture.Manifest) { m.Format = 2 }) }},
		{"one snapshot", func(t *testing.T, dir string) {
			editManifest(t, dir, func(m *capture.Manifest) { m.Snapshots = m.Snapshots[:1] })
			if err := os.RemoveAll(filepath.Join(dir, "1")); err != nil {
				t.Fatal(err)
			}
		}},
		{"a path off the allowlist", func(t *testing.T, dir string) {
			write(t, dir, "0/proc/cpuinfo", "x")
			appendIndex(t, dir, "f\tproc/cpuinfo\n")
		}},
		{"an indexed file missing on disk", func(t *testing.T, dir string) {
			if err := os.Remove(filepath.Join(dir, "0", "proc", "loadavg")); err != nil {
				t.Fatal(err)
			}
		}},
		{"a malformed index line", func(t *testing.T, dir string) { appendIndex(t, dir, "x\tproc/stat\n") }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := captured(t, nil)
			if _, err := replay.Load(dir); err != nil {
				t.Fatalf("the unbroken capture fails to load: %v", err)
			}
			tc.breakIt(t, dir)
			if _, err := replay.Load(dir); err == nil {
				t.Fatal("Load accepted the broken capture")
			}
		})
	}
}

func write(t *testing.T, dir, name, data string) {
	t.Helper()
	p := filepath.Join(dir, filepath.FromSlash(name))
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(data), 0o644); err != nil {
		t.Fatal(err)
	}
}

func appendIndex(t *testing.T, dir, line string) {
	t.Helper()
	f, err := os.OpenFile(filepath.Join(dir, "0", capture.IndexFile), os.O_APPEND|os.O_WRONLY, 0)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	if _, err := f.WriteString(line); err != nil {
		t.Fatal(err)
	}
}

func editManifest(t *testing.T, dir string, edit func(*capture.Manifest)) {
	t.Helper()
	p := filepath.Join(dir, capture.ManifestFile)
	b, err := os.ReadFile(p)
	if err != nil {
		t.Fatal(err)
	}
	var m capture.Manifest
	if err := json.Unmarshal(b, &m); err != nil {
		t.Fatal(err)
	}
	edit(&m)
	if b, err = json.Marshal(m); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, b, 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestReplayPreservesDirectoryErrorAndDanglingLink(t *testing.T) {
	dir := captured(t, func(m *capturetest.Machine) {
		m.Errs["sys/class/power_supply"] = syscall.EACCES
		m.FS["sys/devices/pci0000:00/0000:00:1c.0/net/wlan0/device"].Data = []byte("missing-target")
	})
	c, err := replay.Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	h := c.Snapshots[0].Host()
	if _, err := fs.ReadDir(h.FS, "sys/class/power_supply"); !errors.Is(err, syscall.EACCES) {
		t.Fatalf("ReadDir = %v", err)
	}
	if _, err := fs.Stat(h.FS, "sys/class/net/wlan0/device"); !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("dangling Stat = %v", err)
	}
	if _, err := fs.ReadLink(h.FS, "sys/class/net/wlan0/device"); err != nil {
		t.Fatal(err)
	}
}
