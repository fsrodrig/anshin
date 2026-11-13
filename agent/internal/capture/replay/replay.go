// Package replay turns a committed capture back into probe.Hosts for tests. It imports testing/fstest, so only
// tests may import it; `make check-agent` fails if the anshin-agent binary does.
package replay

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"testing/fstest"

	"golang.org/x/sys/unix"

	"github.com/fsrodrig/anshin/agent/internal/capture"
	"github.com/fsrodrig/anshin/agent/internal/recorder/probe"
)

// ErrNotCaptured is a statfs on a path the capture skipped or never saw.
var ErrNotCaptured = errors.New("not in the capture")

type Capture struct {
	Name      string
	Manifest  capture.Manifest
	Snapshots []Snapshot
}

type Snapshot struct {
	Clock  capture.Clock
	fs     *replayFS
	statfs map[string]capture.StatfsRecord
}

// Host serves this snapshot alone.
func (s Snapshot) Host() probe.Host {
	return probe.Host{FS: s.fs, Statfs: func(p string) (probe.FsStat, error) { return statfs(s.statfs, p) }}
}

// ContractDir finds contract/ by walking up from the test's working directory.
func ContractDir(t testing.TB) string {
	t.Helper()
	dir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	for {
		if fi, err := os.Stat(filepath.Join(dir, "contract")); err == nil && fi.IsDir() {
			return filepath.Join(dir, "contract")
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatal("no contract/ directory above the test's working directory")
		}
		dir = parent
	}
}

// LoadAll loads every capture under contract/captures and fails the test if there is none, so a wrong path can't
// pass on zero captures.
func LoadAll(t testing.TB) []*Capture {
	t.Helper()
	root := filepath.Join(ContractDir(t), "captures")
	entries, err := os.ReadDir(root)
	if err != nil {
		t.Fatal(err)
	}
	var out []*Capture
	for _, e := range entries {
		if !e.IsDir() {
			t.Fatalf("%s: only capture directories belong in contract/captures", e.Name())
		}
		c, err := Load(filepath.Join(root, e.Name()))
		if err != nil {
			t.Fatal(err)
		}
		out = append(out, c)
	}
	if len(out) == 0 {
		t.Fatalf("no captures in %s", root)
	}
	return out
}

// Load checks a capture and builds a replayable Host for each snapshot.
func Load(dir string) (*Capture, error) {
	c := &Capture{Name: filepath.Base(dir)}
	b, err := os.ReadFile(filepath.Join(dir, capture.ManifestFile))
	if err != nil {
		return nil, err
	}
	if err := json.Unmarshal(b, &c.Manifest); err != nil {
		return nil, fmt.Errorf("%s: %w", capture.ManifestFile, err)
	}
	if c.Manifest.Format != capture.FormatVersion {
		return nil, fmt.Errorf("%s: format %d, want %d", dir, c.Manifest.Format, capture.FormatVersion)
	}
	n := len(c.Manifest.Snapshots)
	if n < 2 {
		return nil, fmt.Errorf("%s: %d snapshots, want at least 2", dir, n)
	}
	top, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	for _, e := range top {
		if e.Name() == capture.ManifestFile {
			continue
		}
		if i, err := strconv.Atoi(e.Name()); err != nil || i < 0 || i >= n || strconv.Itoa(i) != e.Name() || !e.IsDir() {
			return nil, fmt.Errorf("%s: stray %s; a capture holds only %s and snapshots 0..%d", dir, e.Name(), capture.ManifestFile, n-1)
		}
	}
	for i, clock := range c.Manifest.Snapshots {
		s, err := loadSnapshot(filepath.Join(dir, strconv.Itoa(i)))
		if err != nil {
			return nil, err
		}
		s.Clock = clock
		c.Snapshots = append(c.Snapshots, s)
	}
	return c, nil
}

func loadSnapshot(dir string) (Snapshot, error) {
	b, err := os.ReadFile(filepath.Join(dir, capture.IndexFile))
	if err != nil {
		return Snapshot{}, err
	}
	index, err := capture.DecodeIndex(b)
	if err != nil {
		return Snapshot{}, fmt.Errorf("%s: %w", dir, err)
	}
	files := map[string][]byte{}
	onDisk := map[string]bool{}
	for _, e := range index {
		if !capture.Allowed(e.Path) {
			return Snapshot{}, fmt.Errorf("%s: %s is not on the capture allowlist", dir, e.Path)
		}
		if e.Kind == capture.File {
			files[e.Path] = nil
		}
	}
	err = filepath.WalkDir(dir, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(dir, p)
		rel = filepath.ToSlash(rel)
		switch {
		case d.Type()&fs.ModeSymlink != 0:
			return fmt.Errorf("%s: %s is a symlink; a capture records links in its index", dir, rel)
		case d.IsDir() || rel == capture.IndexFile || rel == capture.StatfsFile:
			return nil
		case !d.Type().IsRegular():
			return fmt.Errorf("%s: %s is not a regular file", dir, rel)
		}
		if _, ok := files[rel]; !ok {
			return fmt.Errorf("%s: %s is not in the index", dir, rel)
		}
		onDisk[rel] = true
		files[rel], err = os.ReadFile(p)
		return err
	})
	if err != nil {
		return Snapshot{}, err
	}
	for p := range files {
		if !onDisk[p] {
			return Snapshot{}, fmt.Errorf("%s: the index lists %s but the file is missing", dir, p)
		}
	}
	s := Snapshot{statfs: map[string]capture.StatfsRecord{}}
	if b, err = os.ReadFile(filepath.Join(dir, capture.StatfsFile)); err != nil {
		return Snapshot{}, err
	}
	if err := json.Unmarshal(b, &s.statfs); err != nil {
		return Snapshot{}, fmt.Errorf("%s: %w", filepath.Join(dir, capture.StatfsFile), err)
	}
	if s.fs, err = build(index, files); err != nil {
		return Snapshot{}, fmt.Errorf("%s: %w", dir, err)
	}
	return s, nil
}

// build turns an index into a MapFS with real symlinks, so ReadLink, Lstat and ReadDir's entry types match
// production. Each link's subtree moves to .replay/<k>/<target basename> and the link points there; a probe that,
// say, filtered /sys/class/hwmon entries with IsDir would fail in replay as it does on a real Machine.
func build(index []capture.Entry, files map[string][]byte) (*replayFS, error) {
	type node struct {
		kind capture.Kind
		arg  string
		orig string
	}
	var nodes []*node
	paths := map[*node]string{}
	var links []*node
	for _, e := range index {
		n := &node{kind: e.Kind, arg: e.Arg, orig: e.Path}
		paths[n] = e.Path
		nodes = append(nodes, n)
		if e.Kind == capture.Link {
			links = append(links, n)
		}
	}
	slices.SortStableFunc(links, func(a, b *node) int {
		return strings.Count(a.orig, "/") - strings.Count(b.orig, "/")
	})
	m := fstest.MapFS{}
	for k, l := range links {
		from := paths[l]
		target := fmt.Sprintf(".replay/%d/%s", k, l.arg)
		occupied := false
		for _, n := range nodes {
			p := paths[n]
			if n == l || (p != from && !strings.HasPrefix(p, from+"/")) {
				continue
			}
			paths[n] = target + strings.TrimPrefix(p, from)
			occupied = true
		}
		if !occupied {
			m[target] = &fstest.MapFile{Mode: fs.ModeDir | 0o555}
		}
		m[from] = &fstest.MapFile{Mode: fs.ModeSymlink | 0o777, Data: []byte(strings.Repeat("../", strings.Count(from, "/")) + target)}
	}
	r := &replayFS{m: m, errs: map[string]syscall.Errno{}}
	for _, n := range nodes {
		p := paths[n]
		switch n.kind {
		case capture.Dir:
			if _, ok := m[p]; !ok {
				m[p] = &fstest.MapFile{Mode: fs.ModeDir | 0o555}
			}
		case capture.File:
			m[p] = &fstest.MapFile{Data: files[n.orig], Mode: 0o444}
		case capture.Error:
			errno, ok := errnos[n.arg]
			if !ok {
				return nil, fmt.Errorf("%s: unknown errno %q", n.orig, n.arg)
			}
			if _, exists := m[p]; !exists {
				m[p] = &fstest.MapFile{Mode: 0o444}
			}
			r.errs[n.orig] = errno
		}
	}
	return r, nil
}

// errnos inverts unix.ErrnoName, which x/sys offers only one way.
var errnos = func() map[string]syscall.Errno {
	m := map[string]syscall.Errno{}
	for i := syscall.Errno(1); i < 4096; i++ {
		if name := unix.ErrnoName(i); name != "" {
			if _, dup := m[name]; !dup {
				m[name] = i
			}
		}
	}
	return m
}()

func statfs(records map[string]capture.StatfsRecord, p string) (probe.FsStat, error) {
	r, ok := records[p]
	switch {
	case !ok || r.Skipped != "":
		return probe.FsStat{}, &fs.PathError{Op: "statfs", Path: p, Err: ErrNotCaptured}
	case r.Error == capture.TimeoutError:
		return probe.FsStat{}, &fs.PathError{Op: "statfs", Path: p, Err: capture.ErrStatfsTimeout}
	case r.Error != "":
		if errno, ok := errnos[r.Error]; ok {
			return probe.FsStat{}, &fs.PathError{Op: "statfs", Path: p, Err: errno}
		}
		return probe.FsStat{}, &fs.PathError{Op: "statfs", Path: p, Err: errors.New(r.Error)}
	case r.Stat == nil:
		return probe.FsStat{}, &fs.PathError{Op: "statfs", Path: p, Err: ErrNotCaptured}
	}
	return r.Stat.FsStat(), nil
}

// replayFS implements only Open, ReadLink and Lstat, so every helper in io/fs goes through Open and gets recorded.
type replayFS struct {
	m      fstest.MapFS
	errs   map[string]syscall.Errno
	record func(string)
}

func (r *replayFS) note(name string) {
	if r.record != nil {
		r.record(name)
	}
}

func (r *replayFS) Open(name string) (fs.File, error) {
	r.note(name)
	f, err := r.m.Open(name)
	if err != nil {
		return nil, err
	}
	if errno, ok := r.errs[name]; ok {
		return erroringFile{File: f, err: &fs.PathError{Op: "read", Path: name, Err: errno}}, nil
	}
	return f, nil
}

func (r *replayFS) ReadLink(name string) (string, error) {
	r.note(name)
	return r.m.ReadLink(name)
}

func (r *replayFS) Lstat(name string) (fs.FileInfo, error) {
	r.note(name)
	return r.m.Lstat(name)
}

// erroringFile opens like the real file did and fails on read, as a sensor that isn't ready does.
type erroringFile struct {
	fs.File
	err error
}

func (f erroringFile) Read([]byte) (int, error) { return 0, f.err }

// Sequence steps through a capture's snapshots in recorded order behind one Host, the way a long-running Agent sees
// one Machine change over time.
type Sequence struct {
	c       *Capture
	i       int
	opened  []string
	statfsd []string
}

func (c *Capture) Sequence() *Sequence { return &Sequence{c: c} }

func (s *Sequence) Host() probe.Host {
	return probe.Host{FS: seqFS{s}, Statfs: func(p string) (probe.FsStat, error) {
		s.statfsd = append(s.statfsd, p)
		return statfs(s.c.Snapshots[s.i].statfs, p)
	}}
}

// Advance moves to the next snapshot and reports whether there was one.
func (s *Sequence) Advance() bool {
	if s.i+1 >= len(s.c.Snapshots) {
		return false
	}
	s.i++
	return true
}

func (s *Sequence) Clock() capture.Clock { return s.c.Snapshots[s.i].Clock }

// Opened lists every name passed to Open, ReadLink or Lstat, in order.
func (s *Sequence) Opened() []string { return s.opened }

func (s *Sequence) StatfsCalls() []string { return s.statfsd }

type seqFS struct{ s *Sequence }

func (f seqFS) current() *replayFS {
	r := *f.s.c.Snapshots[f.s.i].fs
	r.record = func(name string) { f.s.opened = append(f.s.opened, path.Clean(name)) }
	return &r
}

func (f seqFS) Open(name string) (fs.File, error)      { return f.current().Open(name) }
func (f seqFS) ReadLink(name string) (string, error)   { return f.current().ReadLink(name) }
func (f seqFS) Lstat(name string) (fs.FileInfo, error) { return f.current().Lstat(name) }
