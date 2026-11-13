package capture

import (
	"bytes"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/fsrodrig/anshin/agent/internal/recorder/probe"
)

// FormatVersion is the on-disk layout described in contract/README.md § Captures. Replay refuses any other.
const FormatVersion = 1

const (
	ManifestFile = "capture.json"
	IndexFile    = "index"
	StatfsFile   = "statfs.json"
)

// Clock is when a snapshot started. boottime − monotonic grows only while the Machine is suspended, which is what
// suspend detection (#8) needs.
type Clock struct {
	Wall        time.Time `json:"wall"`
	BoottimeNs  int64     `json:"boottime_ns"`
	MonotonicNs int64     `json:"monotonic_ns"`
}

// Manifest is capture.json.
type Manifest struct {
	Format int `json:"format"`
	// CPUsOnline comes from /sys/devices/system/cpu/online, a kernel source independent of /proc/stat, so replay
	// can check probe's CPU count against it. Null when the file was unreadable.
	CPUsOnline *int    `json:"cpus_online"`
	Snapshots  []Clock `json:"snapshots"`
}

// Kind is an index entry's type.
type Kind byte

const (
	Dir   Kind = 'd'
	File  Kind = 'f'
	Link  Kind = 'l'
	Error Kind = 'e'
)

// Entry is one line of a snapshot's index. Arg is a link's target basename or a read error's errno name.
type Entry struct {
	Kind Kind
	Path string
	Arg  string
}

// EncodeIndex sorts by path, then kind, so a link and the file read through it stay next to each other.
func EncodeIndex(entries []Entry) ([]byte, error) {
	sorted := slices.Clone(entries)
	slices.SortFunc(sorted, func(a, b Entry) int {
		if c := strings.Compare(a.Path, b.Path); c != 0 {
			return c
		}
		return int(a.Kind) - int(b.Kind)
	})
	var buf bytes.Buffer
	for _, e := range sorted {
		if strings.ContainsAny(e.Path+e.Arg, "\t\n") {
			return nil, fmt.Errorf("index entry %q holds a tab or newline", e.Path)
		}
		switch e.Kind {
		case Dir, File:
			fmt.Fprintf(&buf, "%c\t%s\n", e.Kind, e.Path)
		case Link, Error:
			fmt.Fprintf(&buf, "%c\t%s\t%s\n", e.Kind, e.Path, e.Arg)
		default:
			return nil, fmt.Errorf("index entry %q has unknown kind %q", e.Path, e.Kind)
		}
	}
	return buf.Bytes(), nil
}

func DecodeIndex(b []byte) ([]Entry, error) {
	var out []Entry
	for i, line := range strings.Split(strings.TrimSuffix(string(b), "\n"), "\n") {
		if line == "" {
			continue
		}
		f := strings.Split(line, "\t")
		want := 0
		switch f[0] {
		case string(Dir), string(File):
			want = 2
		case string(Link), string(Error):
			want = 3
		}
		if want == 0 || len(f) != want || f[1] == "" {
			return nil, fmt.Errorf("index line %d is malformed: %q", i+1, line)
		}
		e := Entry{Kind: Kind(f[0][0]), Path: f[1]}
		if want == 3 {
			e.Arg = f[2]
		}
		out = append(out, e)
	}
	return out, nil
}

// StatfsRecord is one mount point in statfs.json: a result, an error, or the reason it was never statfs'd.
type StatfsRecord struct {
	*Stat
	Error   string  `json:"error,omitempty"`
	Skipped string  `json:"skipped,omitempty"`
	MntID   *uint64 `json:"mnt_id,omitempty"`
}

type Stat struct {
	Type   int64  `json:"type"`
	Bsize  int64  `json:"bsize"`
	Frsize int64  `json:"frsize"`
	Blocks uint64 `json:"blocks"`
	Bfree  uint64 `json:"bfree"`
	Bavail uint64 `json:"bavail"`
	Files  uint64 `json:"files"`
	Ffree  uint64 `json:"ffree"`
	Flags  int64  `json:"flags"`
}

func StatOf(s probe.FsStat) *Stat {
	return &Stat{Type: s.Type, Bsize: s.Bsize, Frsize: s.Frsize, Blocks: s.Blocks, Bfree: s.Bfree, Bavail: s.Bavail,
		Files: s.Files, Ffree: s.Ffree, Flags: s.Flags}
}

func (s Stat) FsStat() probe.FsStat {
	return probe.FsStat{Type: s.Type, Bsize: s.Bsize, Frsize: s.Frsize, Blocks: s.Blocks, Bfree: s.Bfree,
		Bavail: s.Bavail, Files: s.Files, Ffree: s.Ffree, Flags: s.Flags}
}

// TimeoutError is the statfs.json error for a call still blocked after -statfs-timeout.
const TimeoutError = "timeout"

// ErrStatfsTimeout is what a replayed Host.Statfs returns where the capture recorded a timeout.
var ErrStatfsTimeout = errors.New("statfs timed out during capture")

// Skip reasons in statfs.json.
const (
	SkipAutofs  = "autofs"
	SkipNetwork = "network"
	SkipFuse    = "fuse"
)

// Reserved names: future.json is the hand-written fixture and kept/ holds old fixtures, so no capture may produce
// a fixture with either name.
var reservedNames = []string{"future", "kept"}
