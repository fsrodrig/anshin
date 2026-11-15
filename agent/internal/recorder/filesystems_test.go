package recorder

import (
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"testing/fstest"
	"time"

	"github.com/fsrodrig/anshin/agent/internal/recorder/probe"
)

const rootMount = "2 1 0:35 /@ / rw - btrfs /dev/disk rw\n"

// diskRecorder reads at a time the test advances, so ages and slow-lane refreshes don't depend on the host clock.
type diskRecorder struct {
	*Recorder
	now time.Time
}

func (r *diskRecorder) Read() Reading { return r.ReadAt(r.now) }

func newDiskRecorder(table string, id func(string) (uint64, error), stat func(string) (probe.FsStat, error)) (*diskRecorder, *fstest.MapFile) {
	f := &fstest.MapFile{Data: []byte(table)}
	now := time.Unix(100, 0)
	return &diskRecorder{New(probe.Host{FS: fstest.MapFS{"proc/self/mountinfo": f}, OpenFilesystem: func(p string) (probe.FilesystemHandle, error) {
		mount, err := id(p)
		return probe.FilesystemHandle{MountID: mount, Statfs: func() (probe.FsStat, error) { return stat(p) }, Close: func() error { return nil }}, err
	}}, now, Config{}), now}, f
}

func awaitReading(t *testing.T, r *diskRecorder, accept func(Reading) bool) Reading {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		v := r.Read()
		if accept(v) {
			return v
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatal("filesystem worker did not finish")
	return Reading{}
}

func TestVisibilityFallbackAndOmission(t *testing.T) {
	for _, visible := range []bool{true, false} {
		t.Run(fmt.Sprint(visible), func(t *testing.T) {
			var calls atomic.Int32
			r, _ := newDiskRecorder(rootMount+"3 2 0:35 /@home /home rw - btrfs disk rw\n", func(p string) (uint64, error) {
				if p == "/home" && visible {
					return 3, nil
				}
				return 99, nil
			}, func(p string) (probe.FsStat, error) {
				calls.Add(1)
				if p != "/home" {
					t.Error("statfs on hidden mount")
				}
				return probe.FsStat{Frsize: 1, Blocks: 10, Bavail: 5, Bfree: 5}, nil
			})
			r.Read()
			if visible {
				v := awaitReading(t, r, func(v Reading) bool { return len(v.Filesystems) == 1 && v.Filesystems[0].Total != nil })
				if v.PrimaryDisk != nil || v.Filesystems[0].MountPoint != "/" || calls.Load() != 1 {
					t.Fatal(v, calls.Load())
				}
			} else {
				awaitIdle(t, r)
				if v := r.Read(); len(v.Filesystems) != 0 || v.PrimaryDisk != nil || calls.Load() != 0 {
					t.Fatal(v)
				}
			}
		})
	}
}

func awaitIdle(t *testing.T, r *diskRecorder) {
	t.Helper()
	awaitReading(t, r, func(Reading) bool {
		r.mu.Lock()
		defer r.mu.Unlock()
		for _, s := range r.disks {
			if s.pending {
				return false
			}
		}
		return true
	})
}

func TestBlockedCallSurvivesRemovalAndReplacement(t *testing.T) {
	started, release := make(chan struct{}), make(chan struct{})
	var calls atomic.Int32
	r, f := newDiskRecorder(rootMount, func(string) (uint64, error) { return 2, nil }, func(string) (probe.FsStat, error) {
		calls.Add(1)
		close(started)
		<-release
		return probe.FsStat{Frsize: 1, Blocks: 10, Bavail: 5, Bfree: 5}, nil
	})
	var releaseOnce sync.Once
	defer releaseOnce.Do(func() { close(release) })
	r.Read()
	<-started
	v := awaitReading(t, r, func(v Reading) bool { return len(v.Filesystems) == 1 })
	if v.Filesystems[0].Total != nil || v.Filesystems[0].Age != 0 {
		t.Fatal(v)
	}
	for i := 0; i < 20; i++ {
		r.now = r.now.Add(30 * time.Second)
		if i%2 == 0 {
			f.Data = nil
		} else {
			f.Data = []byte(rootMount)
		}
		done := make(chan Reading, 1)
		go func() { done <- r.Read() }()
		select {
		case <-done:
		case <-time.After(time.Second):
			t.Fatal("Sample blocked")
		}
	}
	if calls.Load() != 1 {
		t.Fatal("blocked call replaced", calls.Load())
	}
	// Reappearance cannot publish completion from the removed mount generation.
	releaseOnce.Do(func() { close(release) })
	awaitIdle(t, r)
	if v := r.Read(); len(v.Filesystems) != 1 || v.Filesystems[0].Total != nil || v.Filesystems[0].Age != 600 {
		t.Fatal("stale completion or lost blocked age", v)
	}
}

func TestRefreshClearsOldCapacityAndRetainsAge(t *testing.T) {
	var fail atomic.Bool
	r, _ := newDiskRecorder(rootMount, func(string) (uint64, error) { return 2, nil }, func(string) (probe.FsStat, error) {
		if fail.Load() {
			return probe.FsStat{}, errors.New("unavailable")
		}
		return probe.FsStat{Frsize: 1, Blocks: 10, Bfree: 10, Bavail: 10}, nil
	})
	first := awaitReading(t, r, func(v Reading) bool { return len(v.Filesystems) == 1 && v.Filesystems[0].Total != nil })
	fail.Store(true)
	r.now = r.now.Add(30 * time.Second)
	v := r.Read()
	if len(v.Filesystems) != 1 || v.Filesystems[0].Total != nil || v.Filesystems[0].UsedPct != nil {
		t.Fatal(v)
	}
	awaitIdle(t, r)
	v = r.Read()
	if v.Filesystems[0].Total != nil || v.Filesystems[0].Age < 29 || *first.Filesystems[0].UsedPct != 0 {
		t.Fatal(v, first)
	}
}

func TestFiveSubvolumesDifferentFsidsMakeOneCapacityCall(t *testing.T) {
	table := rootMount
	ids := map[string]uint64{"/": 2, "/home": 3, "/log": 4, "/cache": 5, "/snapshots": 6, "/boot": 7}
	// Each subvolume's synthetic kernel fsid differs. FsStat intentionally discards that field at the adapter.
	records := map[string]struct {
		fsid int
		stat probe.FsStat
	}{}
	for i, p := range []string{"/", "/home", "/log", "/cache", "/snapshots", "/boot"} {
		dev := "0:35"
		if p == "/boot" {
			dev = "8:1"
		}
		if p != "/" {
			table += fmt.Sprintf("%d 2 %s /sub%d %s ro - btrfs disk ro\n", ids[p], dev, i, p)
		}
		records[p] = struct {
			fsid int
			stat probe.FsStat
		}{i, probe.FsStat{Frsize: 1, Blocks: uint64(100 + i), Bfree: 10, Bavail: 10}}
	}
	var calls atomic.Int32
	r, _ := newDiskRecorder(table, func(p string) (uint64, error) { return ids[p], nil }, func(p string) (probe.FsStat, error) { calls.Add(1); return records[p].stat, nil })
	v := awaitReading(t, r, func(v Reading) bool {
		return len(v.Filesystems) == 2 && v.Filesystems[0].Total != nil && v.Filesystems[1].Total != nil
	})
	if calls.Load() != 2 || *v.Filesystems[0].Total != 100 || *v.Filesystems[1].Total != 105 {
		t.Fatal(v, calls.Load())
	}
}

func TestUnsafeMountsNeverReachAdapters(t *testing.T) {
	r, _ := newDiskRecorder("2 1 8:1 / /bad rw - nfs server:/share rw\n3 2 8:2 / /bad/child rw - ext4 disk rw\n", func(string) (uint64, error) { t.Error("unsafe visibility call"); return 0, nil }, func(string) (probe.FsStat, error) { t.Error("unsafe capacity call"); return probe.FsStat{}, nil })
	if v := r.Read(); len(v.Filesystems) != 0 {
		t.Fatal(v)
	}
	awaitIdle(t, r)
}

func TestBlockedVisibilityDoesNotBlockSampleOrSpawnReplacement(t *testing.T) {
	var calls atomic.Int32
	started, release := make(chan struct{}), make(chan struct{})
	r, _ := newDiskRecorder(rootMount, func(string) (uint64, error) { calls.Add(1); close(started); <-release; return 2, nil }, func(string) (probe.FsStat, error) { return probe.FsStat{}, nil })
	r.Read()
	<-started
	for i := 0; i < 10; i++ {
		r.now = r.now.Add(30 * time.Second)
		if v := r.Read(); len(v.Filesystems) != 0 {
			t.Fatal(v)
		}
	}
	close(release)
	awaitIdle(t, r)
	if calls.Load() != 1 {
		t.Fatal(calls.Load())
	}
}

func TestReplacementCannotReuseAPathWithBlockedCapacity(t *testing.T) {
	var calls atomic.Int32
	started, release := make(chan struct{}), make(chan struct{})
	r, f := newDiskRecorder(rootMount, func(string) (uint64, error) { return 2, nil }, func(string) (probe.FsStat, error) {
		calls.Add(1)
		close(started)
		<-release
		return probe.FsStat{Frsize: 1, Blocks: 100}, nil
	})
	r.Read()
	<-started
	awaitReading(t, r, func(v Reading) bool { return len(v.Filesystems) == 1 })
	// A different filesystem now occupies the same path while the original kernel call is still pending.
	f.Data = []byte("3 1 8:1 / / rw - ext4 disk rw\n")
	r.now = r.now.Add(30 * time.Second)
	r.Read()
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		r.Read()
		r.mu.Lock()
		s := r.disks["8:1"]
		done := s != nil && !s.pending
		r.mu.Unlock()
		if done {
			break
		}
		time.Sleep(time.Millisecond)
	}
	if calls.Load() != 1 {
		t.Fatal("replacement probed blocked path", calls.Load())
	}
	close(release)
	awaitIdle(t, r)
	if v := r.Read(); len(v.Filesystems) != 0 || v.PrimaryDisk != nil {
		t.Fatal("replacement inherited old capacity", v)
	}
}

func TestSlowSuccessIsAgedFromItsLaunch(t *testing.T) {
	started, release := make(chan struct{}), make(chan struct{})
	r, _ := newDiskRecorder(rootMount, func(string) (uint64, error) { return 2, nil }, func(string) (probe.FsStat, error) {
		close(started)
		<-release
		return probe.FsStat{Frsize: 1, Blocks: 100}, nil
	})
	r.Read()
	<-started
	r.now = r.now.Add(10 * time.Second)
	close(release)
	v := awaitReading(t, r, func(v Reading) bool { return len(v.Filesystems) == 1 && v.Filesystems[0].Total != nil })
	if v.Filesystems[0].Age != 10 {
		t.Fatal("success not aged from its launch", v.Filesystems[0].Age)
	}
}
