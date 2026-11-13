package capture_test

import (
	"bytes"
	"context"
	"encoding/json"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"syscall"
	"testing"
	"testing/fstest"
	"time"

	"github.com/fsrodrig/anshin/agent/internal/capture"
	"github.com/fsrodrig/anshin/agent/internal/capture/capturetest"
)

func options(m *capturetest.Machine) capture.Options {
	return capture.Options{Name: "desktop", Snapshots: 2, Interval: time.Millisecond,
		StatfsTimeout: 200 * time.Millisecond, Identifiers: m.ID.IdentifiersFile()}
}

// run captures the fake Machine into a fresh directory and returns the directory and the report.
func run(t *testing.T, m *capturetest.Machine, opt capture.Options) (string, string) {
	t.Helper()
	t.Cleanup(m.Release)
	dir := filepath.Join(t.TempDir(), "desktop")
	var report bytes.Buffer
	if err := capture.Run(context.Background(), m.Source(), dir, opt, &report); err != nil {
		t.Fatalf("Run: %v", err)
	}
	return dir, report.String()
}

func index(t *testing.T, dir string, snapshot int) []capture.Entry {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(dir, string(rune('0'+snapshot)), capture.IndexFile))
	if err != nil {
		t.Fatal(err)
	}
	entries, err := capture.DecodeIndex(b)
	if err != nil {
		t.Fatal(err)
	}
	return entries
}

func entry(entries []capture.Entry, kind capture.Kind, path string) (capture.Entry, bool) {
	for _, e := range entries {
		if e.Kind == kind && e.Path == path {
			return e, true
		}
	}
	return capture.Entry{}, false
}

func statfsRecords(t *testing.T, dir string) map[string]capture.StatfsRecord {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(dir, "0", capture.StatfsFile))
	if err != nil {
		t.Fatal(err)
	}
	var out map[string]capture.StatfsRecord
	if err := json.Unmarshal(b, &out); err != nil {
		t.Fatal(err)
	}
	return out
}

func TestCaptureWritesOnlyAllowlistedPaths(t *testing.T) {
	m := capturetest.NewMachine()
	dir, _ := run(t, m, options(m))
	for _, e := range index(t, dir, 0) {
		if !capture.Allowed(e.Path) {
			t.Errorf("index holds %s, which is not allowlisted", e.Path)
		}
	}
	err := filepath.WalkDir(dir, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		for _, decoy := range []string{"address", "uevent", "serial_number", "model_name", "cpuinfo", "passwd", "hostname"} {
			if d.Name() == decoy && !strings.HasSuffix(p, "proc/sys/kernel/hostname") {
				t.Errorf("captured %s", p)
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, "0", "etc", "hostname")); err == nil {
		t.Error("/etc/hostname was copied; it is read only to build the scrubber")
	}
}

func TestCaptureNeverTouchesWhatCouldWakeHardwareOrIdentify(t *testing.T) {
	m := capturetest.NewMachine()
	run(t, m, options(m))
	opened := m.Opened()
	for _, never := range m.Never() {
		if slices.Contains(opened, never) {
			t.Errorf("capture opened %s", never)
		}
	}
	calls := m.StatfsCalls()
	for _, never := range capturetest.NeverStatfsd {
		if slices.Contains(calls, never) {
			t.Errorf("capture statfs'd %s", never)
		}
	}
	if !slices.Contains(calls, "/proc/sys/fs/binfmt_misc") {
		t.Error("binfmt_misc mounted over its autofs trigger was not statfs'd; the topmost entry decides")
	}
	if !slices.Contains(opened, "sys/class/hwmon/hwmon0/temp1_input") {
		t.Error("the awake k10temp's temperature was not read")
	}
}

func TestCaptureRecordsStatfs(t *testing.T) {
	m := capturetest.NewMachine()
	start := time.Now()
	dir, report := run(t, m, options(m))
	// Two snapshots, each with one blocked mount bounded at 200 ms.
	if d := time.Since(start); d > 2*time.Second {
		t.Errorf("capture took %v with a hung mount; the timeout did not bound it", d)
	}
	got := statfsRecords(t, dir)
	root := got["/"]
	if root.Stat == nil || root.Blocks != 100 || root.Bavail != 30 || root.Frsize != 4096 || root.MntID == nil || *root.MntID != 1 {
		t.Errorf("/ = %+v, want its statfs result and mount ID", root)
	}
	if got["/mnt/denied"].Error != "EACCES" || got["/mnt/denied"].Stat != nil {
		t.Errorf("/mnt/denied = %+v, want EACCES", got["/mnt/denied"])
	}
	if got["/mnt/slow"].Error != capture.TimeoutError {
		t.Errorf("/mnt/slow = %+v, want a timeout", got["/mnt/slow"])
	}
	for mp, why := range map[string]string{"/mnt/auto": capture.SkipAutofs, "/mnt/nas": capture.SkipNetwork,
		"/run/user/1000/doc": capture.SkipFuse, "/mnt/ssh": capture.SkipFuse} {
		if got[mp].Skipped != why {
			t.Errorf("%s = %+v, want skipped as %s", mp, got[mp], why)
		}
		if !strings.Contains(report, mp+": ") {
			t.Errorf("the report does not list the skipped %s", mp)
		}
	}
	if got["/mnt/with space"].Stat == nil {
		t.Errorf("an escaped mount point was not unescaped before statfs: %v", got)
	}
	if got["/home/user/data"].Stat == nil {
		t.Errorf("the scrubbed home mount is missing: %v", got)
	}
}

func TestCaptureRecordsLinksDirectoriesAndErrors(t *testing.T) {
	m := capturetest.NewMachine()
	delete(m.FS, "sys/class/power_supply/BAT0")
	for p := range m.FS {
		if strings.HasPrefix(p, "sys/class/power_supply/") {
			delete(m.FS, p)
		}
	}
	m.FS["sys/class/power_supply"] = &fstest.MapFile{Mode: fs.ModeDir | 0o555}
	dir, _ := run(t, m, options(m))
	entries := index(t, dir, 0)
	for path, want := range map[string]string{
		"sys/class/hwmon/hwmon0":        "hwmon0",
		"sys/class/hwmon/hwmon0/device": "0000:00:00.0",
		"sys/class/hwmon/hwmon2/device": "thermal_zone0",
		"sys/class/net/wlan0/device":    "0000:00:1c.0",
		"sys/class/block/nvme0n1":       "nvme0n1",
		"proc/self":                     "4242",
		"etc/os-release":                "os-release",
	} {
		if e, ok := entry(entries, capture.Link, path); !ok || e.Arg != want {
			t.Errorf("link %s = %+v, want target basename %s", path, e, want)
		}
	}
	if _, ok := entry(entries, capture.File, "etc/os-release"); !ok {
		t.Error("the file read through etc/os-release is not in the index")
	}
	if _, ok := entry(entries, capture.Link, "sys/class/block/zram0/device"); ok {
		t.Error("zram0 has no device, but the index says it does")
	}
	if _, ok := entry(entries, capture.Dir, "sys/class/power_supply"); !ok {
		t.Error("an empty power_supply class has no d entry, so replay would see it missing")
	}
	if e, ok := entry(entries, capture.Error, "sys/class/hwmon/hwmon5/temp2_input"); !ok || e.Arg != "ENODATA" {
		t.Errorf("a sensor that isn't ready = %+v, want an e entry with ENODATA", e)
	}
	err := filepath.WalkDir(dir, func(p string, d fs.DirEntry, err error) error {
		if err == nil && d.Type()&fs.ModeSymlink != 0 {
			t.Errorf("%s is a symlink on disk", p)
		}
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestCaptureTakesTimedSnapshots(t *testing.T) {
	m := capturetest.NewMachine()
	opt := options(m)
	opt.Snapshots = 3
	dir, _ := run(t, m, opt)
	b, err := os.ReadFile(filepath.Join(dir, capture.ManifestFile))
	if err != nil {
		t.Fatal(err)
	}
	var man capture.Manifest
	if err := json.Unmarshal(b, &man); err != nil {
		t.Fatal(err)
	}
	if man.Format != capture.FormatVersion || len(man.Snapshots) != 3 || man.CPUsOnline == nil || *man.CPUsOnline != 2 {
		t.Fatalf("manifest = %s", b)
	}
	for i, c := range man.Snapshots {
		want := time.Date(2026, 1, 2, 2, 4, 5+i, 6, time.UTC)
		if !c.Wall.Equal(want) || c.Wall.Location() != time.UTC || c.BoottimeNs-c.MonotonicNs != 7 {
			t.Errorf("snapshot %d clock = %+v, want %v in UTC", i, c, want)
		}
	}
	if !strings.Contains(string(b), `"wall": "2026-01-02T02:04:05.000000006Z"`) {
		t.Errorf("wall time is not RFC 3339 UTC with nanoseconds: %s", b)
	}
	s0, _ := os.ReadFile(filepath.Join(dir, "0", "proc", "stat"))
	s2, _ := os.ReadFile(filepath.Join(dir, "2", "proc", "stat"))
	if bytes.Equal(s0, s2) {
		t.Error("snapshots 0 and 2 have the same /proc/stat")
	}
}

func TestCaptureScrubsContentsAndPaths(t *testing.T) {
	m := capturetest.NewMachine()
	dir, report := run(t, m, options(m))
	read := func(p string) string {
		b, err := os.ReadFile(filepath.Join(dir, "0", p))
		if err != nil {
			t.Fatal(err)
		}
		return string(b)
	}
	if got := read("proc/sys/kernel/hostname"); got != "desktop\n" {
		t.Errorf("hostname = %q, want the capture name", got)
	}
	osr := read("etc/os-release")
	for _, want := range []string{`PRETTY_NAME="desktop Linux"`, "SERIAL_NUMBER=SCRUBBED", "LINK=00:00:00:00:00:00", "User=user"} {
		if !strings.Contains(osr, want) {
			t.Errorf("os-release lacks %q:\n%s", want, osr)
		}
	}
	mounts := read("proc/self/mountinfo")
	for _, want := range []string{"/home/user/data", "nas.example.ts.net:/export", "addr=100.64.0.1", "user@scrubbed:/", "v6=fd7a:115c:a1e0::1"} {
		if !strings.Contains(mounts, want) {
			t.Errorf("mountinfo lacks %q:\n%s", want, mounts)
		}
	}
	if !strings.Contains(read("proc/net/dev"), " enx000000000000: ") {
		t.Errorf("a MAC-named interface was not scrubbed in /proc/net/dev")
	}
	entries := index(t, dir, 0)
	for _, p := range []string{"sys/class/net/enx000000000000/device", "sys/class/power_supply/hid-00:00:00:00:00:00-battery"} {
		if _, ok := entry(entries, capture.Link, p); !ok {
			t.Errorf("index lacks the scrubbed path %s", p)
		}
	}
	if _, err := os.Stat(filepath.Join(dir, "0", "sys/class/power_supply/hid-00:00:00:00:00:00-battery/capacity")); err != nil {
		t.Errorf("the scrubbed Bluetooth battery's file is missing: %v", err)
	}
	for _, rule := range []string{capture.RuleSerial, capture.RuleMAC, capture.RuleMACInterface, capture.RuleHostname,
		capture.RuleDenylist, capture.RuleHome, capture.RuleUsername, capture.RuleTailnetName, capture.RuleTailnetV4,
		capture.RuleTailnetV6} {
		if !strings.Contains(report, "  "+rule+": ") {
			t.Errorf("the report has no count for %s", rule)
		}
	}
	assertNoIdentifier(t, m.ID, dir, report)
}

// assertNoIdentifier sweeps every path and byte under dir, and the report, for any raw identifier.
func assertNoIdentifier(t *testing.T, id capturetest.Identifiers, dir, report string) {
	t.Helper()
	check := func(where, text string) {
		lower := strings.ToLower(text)
		for _, raw := range id.All() {
			if strings.Contains(lower, strings.ToLower(raw)) {
				t.Errorf("%s holds a raw identifier (%d characters)", where, len(raw))
			}
		}
	}
	check("the report", report)
	err := filepath.WalkDir(dir, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(dir, p)
		check("path "+rel, rel)
		if d.Type().IsRegular() {
			b, err := os.ReadFile(p)
			if err != nil {
				return err
			}
			check(rel, string(b))
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestScrubbingIsIdempotent(t *testing.T) {
	m := capturetest.NewMachine()
	dir, _ := run(t, m, options(m))
	scr, err := capture.NewScrubber("desktop", capture.Terms{
		Hostnames: []string{m.ID.Hostname, m.ID.ShortHostname}, Words: []string{m.ID.DenyWord}, Users: []string{m.ID.User}})
	if err != nil {
		t.Fatal(err)
	}
	err = filepath.WalkDir(dir, func(p string, d fs.DirEntry, err error) error {
		if err != nil || !d.Type().IsRegular() {
			return err
		}
		b, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		if again := scr.Scrub(p, b); !bytes.Equal(again, b) {
			t.Errorf("scrubbing %s again changed it", p)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(scr.Counts()) != 0 {
		t.Errorf("a second scrub replaced something: %v", scr.Counts())
	}
}

func TestCaptureRefusesAPathCollision(t *testing.T) {
	m := capturetest.NewMachine()
	t.Cleanup(m.Release)
	other := strings.Replace(m.ID.BluetoothMAC, "5A", "5B", 1)
	m.FS["sys/class/power_supply/hid-"+other+"-battery"] = m.FS["sys/class/power_supply/hid-"+m.ID.BluetoothMAC+"-battery"]
	err := capture.Run(context.Background(), m.Source(), filepath.Join(t.TempDir(), "c"), options(m), &bytes.Buffer{})
	if err == nil || !strings.Contains(err.Error(), "scrub to") {
		t.Fatalf("Run = %v, want a collision error", err)
	}
	for _, raw := range m.ID.All() {
		if strings.Contains(strings.ToLower(err.Error()), strings.ToLower(raw)) {
			t.Error("the collision error names a raw identifier")
		}
	}
}

func TestCaptureRefuses(t *testing.T) {
	m := capturetest.NewMachine()
	t.Cleanup(m.Release)
	nonEmpty := t.TempDir()
	if err := os.WriteFile(filepath.Join(nonEmpty, "x"), nil, 0o644); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name string
		edit func(*capture.Options, *capture.Source)
		dir  string
	}{
		{"a non-empty directory", func(*capture.Options, *capture.Source) {}, nonEmpty},
		{"no name", func(o *capture.Options, _ *capture.Source) { o.Name = "" }, ""},
		{"a name with capitals", func(o *capture.Options, _ *capture.Source) { o.Name = "Desktop" }, ""},
		{"the future fixture's name", func(o *capture.Options, _ *capture.Source) { o.Name = "future" }, ""},
		{"the kept directory's name", func(o *capture.Options, _ *capture.Source) { o.Name = "kept" }, ""},
		{"a name containing the hostname", func(o *capture.Options, _ *capture.Source) { o.Name = "my-" + m.ID.ShortHostname }, ""},
		{"a denylist entry shorter than 3", func(o *capture.Options, _ *capture.Source) { o.Identifiers = []byte("ab\n") }, ""},
		{"a denylist entry inside a placeholder", func(o *capture.Options, _ *capture.Source) { o.Identifiers = []byte("crub\n") }, ""},
		{"one snapshot", func(o *capture.Options, _ *capture.Source) { o.Snapshots = 1 }, ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			opt, src := options(m), m.Source()
			tc.edit(&opt, &src)
			dir := tc.dir
			if dir == "" {
				dir = filepath.Join(t.TempDir(), "c")
			}
			before := len(m.Opened())
			if err := capture.Run(context.Background(), src, dir, opt, &bytes.Buffer{}); err == nil {
				t.Fatal("Run succeeded")
			}
			if _, err := os.Stat(filepath.Join(dir, "0")); err == nil {
				t.Error("a refused capture wrote a snapshot")
			}
			for _, p := range m.Opened()[before:] {
				if p != "proc/sys/kernel/hostname" && p != "etc/hostname" && p != "etc/passwd" && p != "proc" && p != "etc" && !strings.HasPrefix(p, "proc/sys") {
					t.Errorf("a refused capture read %s", p)
				}
			}
		})
	}
}

func TestKnownUsernameTokens(t *testing.T) {
	m := capturetest.NewMachine()
	scr, err := capture.NewScrubber("desktop", capture.Terms{Users: []string{m.ID.User}})
	if err != nil {
		t.Fatal(err)
	}
	for _, text := range []string{"/mnt/" + m.ID.User, "username=" + strings.ToUpper(m.ID.User) + ",rw", m.ID.User + "/" + m.ID.User} {
		got := scr.Quiet(text)
		if strings.Contains(strings.ToLower(got), strings.ToLower(m.ID.User)) {
			t.Fatal("known username survived")
		}
	}
	if got := scr.Quiet("prefix" + m.ID.User + "suffix"); got != "prefix"+m.ID.User+"suffix" {
		t.Fatal("matched partial token")
	}
}

func TestCaptureFailureIsNotPublished(t *testing.T) {
	for _, mode := range []string{"initial hostname", "changed hostname", "cancel"} {
		t.Run(mode, func(t *testing.T) {
			m := capturetest.NewMachine()
			t.Cleanup(m.Release)
			parent := t.TempDir()
			dir := filepath.Join(parent, "desktop")
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			src := m.Source()
			clock := src.Clock
			src.Clock = func() capture.Clock {
				c := clock()
				if mode == "cancel" {
					cancel()
				}
				if mode == "changed hostname" {
					m.FS["proc/sys/kernel/hostname"].Data = []byte("changed-placeholder")
				}
				return c
			}
			if mode == "initial hostname" {
				delete(m.FS, "proc/sys/kernel/hostname")
				delete(m.FS, "etc/hostname")
			}
			if err := capture.Run(ctx, src, dir, options(m), &bytes.Buffer{}); err == nil {
				t.Fatal("capture succeeded")
			}
			entries, err := os.ReadDir(parent)
			if err != nil || len(entries) != 0 {
				t.Fatal("failed capture left output or staging directory")
			}
		})
	}
}

func TestCaptureSkipsUnreadableSensorGuards(t *testing.T) {
	for _, attr := range []string{"name", "device/power/runtime_status"} {
		t.Run(attr, func(t *testing.T) {
			m := capturetest.NewMachine()
			m.Errs["sys/class/hwmon/hwmon0/"+attr] = syscall.EACCES
			_, report := run(t, m, options(m))
			if slices.Contains(m.Opened(), "sys/class/hwmon/hwmon0/temp1_input") {
				t.Fatal("read temperature without guard")
			}
			if !strings.Contains(report, "unreadable") {
				t.Fatal("missing skip reason")
			}
		})
	}
}

func TestCaptureStatfsSafety(t *testing.T) {
	m := capturetest.NewMachine()
	m.FS["proc/4242/mountinfo"].Data = append(m.FS["proc/4242/mountinfo"].Data, []byte("12 5 0:60 / /mnt/auto/child rw - ext4 /dev/sde rw\n")...)
	src := m.Source()
	mountID := src.MountID
	src.MountID = func(p string) (uint64, error) {
		if p == "/mnt/denied" {
			return 999, nil
		}
		return mountID(p)
	}
	t.Cleanup(m.Release)
	dir := filepath.Join(t.TempDir(), "desktop")
	if err := capture.Run(context.Background(), src, dir, options(m), &bytes.Buffer{}); err != nil {
		t.Fatal(err)
	}
	calls := m.StatfsCalls()
	for _, p := range []string{"/mnt/auto/child", "/mnt/denied"} {
		if slices.Contains(calls, p) {
			t.Fatalf("unsafe statfs on %s", p)
		}
	}
	n := 0
	for _, p := range calls {
		if p == "/mnt/slow" {
			n++
		}
	}
	if n != 1 {
		t.Fatalf("blocked mount calls = %d, want 1", n)
	}
	records := statfsRecords(t, dir)
	if records["/mnt/auto/child"].Skipped == "" || records["/mnt/denied"].Skipped == "" {
		t.Fatal("missing skip records")
	}
}
