package recorder

import (
	"errors"
	"fmt"
	"reflect"
	"strings"
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

func (r *diskRecorder) Read() Reading { return r.sampleAt(r.now) }

// newDiskRecorder's adapt changes the host before Open, whose first Sample already launches the filesystem workers.
func newDiskRecorder(table string, id func(string) (uint64, error), stat func(string) (probe.FsStat, error), adapt ...func(*probe.Host)) (*diskRecorder, *fstest.MapFile) {
	f := &fstest.MapFile{Data: []byte(table)}
	now := time.Unix(100, 0)
	h := probe.Host{FS: fstest.MapFS{"proc/self/mountinfo": f}, OpenFilesystem: func(p string) (probe.FilesystemHandle, error) {
		mount, err := id(p)
		return probe.FilesystemHandle{MountID: mount, Statfs: func() (probe.FsStat, error) { return stat(p) }, Close: func() error { return nil }}, err
	}}
	for _, a := range adapt {
		a(&h)
	}
	return &diskRecorder{openAt(h, now, Config{}), now}, f
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

// awaitWorkers waits without taking a Sample, so the next Read is the first Sample after the launch.
func awaitWorkers(t *testing.T, r *diskRecorder) {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for !r.Idle() {
		if time.Now().After(deadline) {
			t.Fatal("filesystem worker did not finish")
		}
		time.Sleep(time.Millisecond)
	}
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

func TestFailedRefreshIsUnavailableFromTheSampleThatSeesIt(t *testing.T) {
	var fail atomic.Bool
	r, _ := newDiskRecorder(rootMount, func(string) (uint64, error) { return 2, nil }, func(string) (probe.FsStat, error) {
		if fail.Load() {
			return probe.FsStat{}, errors.New("unavailable")
		}
		return probe.FsStat{Frsize: 1, Blocks: 10, Bfree: 10, Bavail: 10}, nil
	})
	awaitReading(t, r, func(v Reading) bool { return len(v.Filesystems) == 1 && v.Filesystems[0].Total != nil })
	fail.Store(true)
	r.now = r.now.Add(30 * time.Second)
	if v := r.Read(); len(v.Filesystems) != 1 || v.Filesystems[0].Total == nil || *v.Filesystems[0].UsedPct != 0 || v.Filesystems[0].Age != 30 {
		t.Fatal("launch Sample dropped the last capacity", v)
	}
	awaitWorkers(t, r)
	r.now = r.now.Add(2 * time.Second)
	v := r.Read()
	if len(v.Filesystems) != 1 {
		t.Fatal("failed refresh lost its filesystem row", v.Filesystems)
	}
	f := v.Filesystems[0]
	if f.Total != nil || f.Used != nil || f.Available != nil || f.UsedPct != nil || f.Age != 32 {
		t.Fatal("failure not unavailable or not aged from the last success", v)
	}
}

func TestMountTableChangesAtRefreshLaunch(t *testing.T) {
	const table = "1 9 0:10 / / rw - tmpfs tmpfs rw\n" +
		"2 1 8:1 / /data rw - ext4 /dev/disk rw\n"
	nested := strings.Replace(table, " /data ", " /mnt/data ", 1)
	movingTmpfs := nested + "3 1 0:11 / /other rw - tmpfs tmpfs rw\n"
	sibling := strings.Replace(table, " /data ", " /mnt ", 1)
	escaped := strings.Replace(table, " /data ", ` /my\040mount/data `, 1)
	multipleCandidates := rootMount + "3 2 0:35 /@home /home rw - btrfs /dev/disk rw\n"
	filtered := strings.Replace(multipleCandidates, " /home ", " /data ", 1) + "4 3 0:11 / /data rw - tmpfs tmpfs rw\n"
	for _, tc := range []struct {
		name, before, after string
		visibleID           uint64
		keepCapacity        bool
	}{
		{"stacked local mount", table, table + "3 2 8:2 / /data rw - ext4 /dev/other rw\n", 3, false},
		{"stacked tmpfs", table, table + "3 2 0:11 / /data rw - tmpfs tmpfs rw\n", 3, false},
		{"mount options only", table, strings.Replace(table, "/data rw", "/data ro", 1), 2, true},
		{"ancestor over-mount", nested, nested + "3 1 0:11 / /mnt rw - tmpfs tmpfs rw\n", 3, false},
		{"tmpfs moves over ancestor", movingTmpfs, strings.Replace(movingTmpfs, " /other ", " /mnt ", 1), 3, false},
		{"removed escaped ancestor", escaped + "3 1 0:11 / /my\\040mount rw - tmpfs tmpfs rw\n", escaped, 2, false},
		{"unrelated mount beside primary", rootMount, rootMount + "3 2 0:11 / /other rw - tmpfs tmpfs rw\n", 2, true},
		{"sibling prefix", sibling, sibling + "3 1 0:11 / /mn rw - tmpfs tmpfs rw\n", 2, true},
		{"over-mount at second candidate", multipleCandidates, multipleCandidates + "4 3 0:11 / /home rw - tmpfs tmpfs rw\n", 2, false},
		{"filtered mount ID replaced", filtered, strings.Replace(filtered, "\n4 3", "\n5 3", 1), 2, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var visibleID atomic.Uint64
			visibleID.Store(2)
			r, f := newDiskRecorder(tc.before, func(string) (uint64, error) {
				return visibleID.Load(), nil
			}, func(string) (probe.FsStat, error) {
				return probe.FsStat{Frsize: 1, Blocks: 100, Bfree: 2, Bavail: 2}, nil
			})
			awaitWorkers(t, r)
			before := r.Read()
			hasFullReason := func(v Reading) bool {
				if v.Verdict != nil {
					for _, reason := range v.Verdict.Reasons {
						if reason.Code == "filesystem_full" {
							return true
						}
					}
				}
				return false
			}
			if len(before.Filesystems) != 1 || !hasFullReason(before) || before.Verdict.Level != LevelCrit {
				t.Fatal("expected one full filesystem before the table change", before)
			}
			previous := before.Filesystems[0]
			checkLaunch := func(launch Reading, want probe.Filesystem, keepCapacity bool) {
				t.Helper()
				want.Age = 30
				if !keepCapacity {
					want.Total, want.Used, want.Available, want.UsedPct = nil, nil, nil, nil
				}
				if len(launch.Filesystems) != 1 || !reflect.DeepEqual(launch.Filesystems[0], want) {
					t.Errorf("unexpected launch Sample filesystem, want %+v, got %+v", want, launch.Filesystems)
				}
				if hasFullReason(launch) != keepCapacity {
					t.Errorf("launch Sample filesystem_full Reason, want present=%t, got %+v", keepCapacity, launch.Verdict)
				}
			}

			visibleID.Store(tc.visibleID)
			f.Data = []byte(tc.after)
			r.now = r.now.Add(30 * time.Second)
			checkLaunch(r.Read(), previous, tc.keepCapacity)

			awaitWorkers(t, r)
			next := r.Read()
			if tc.visibleID != 2 {
				for _, disk := range next.Filesystems {
					if disk.MajMin == previous.MajMin {
						t.Fatal("hidden filesystem still listed", next.Filesystems)
					}
				}
				return
			}
			if len(next.Filesystems) != 1 || !reflect.DeepEqual(next.Filesystems[0], previous) {
				t.Fatal("visible filesystem did not regain capacity", next.Filesystems)
			}
			r.now = r.now.Add(30 * time.Second)
			checkLaunch(r.Read(), previous, true)
			awaitWorkers(t, r)
		})
	}
}

func TestMountTableChangeClearsOnlyAffectedFilesystem(t *testing.T) {
	const table = rootMount + "3 2 8:1 / /data rw - ext4 /dev/other rw\n"
	var dataMountID atomic.Uint64
	dataMountID.Store(3)
	r, f := newDiskRecorder(table, func(p string) (uint64, error) {
		if p == "/data" {
			return dataMountID.Load(), nil
		}
		return 2, nil
	}, func(string) (probe.FsStat, error) {
		return probe.FsStat{Frsize: 1, Blocks: 100, Bfree: 5, Bavail: 5}, nil
	})
	awaitWorkers(t, r)
	want := r.Read().Filesystems
	if len(want) != 2 || want[0].MountPoint != "/" || want[1].MountPoint != "/data" || want[0].Total == nil || want[1].Total == nil {
		t.Fatalf("expected capacity for both filesystems before the change, got %+v", want)
	}
	want[0].Age, want[1].Age = 30, 30
	want[1].Total, want[1].Used, want[1].Available, want[1].UsedPct = nil, nil, nil, nil
	dataMountID.Store(4)
	f.Data = []byte(table + "4 3 0:11 / /data rw - tmpfs tmpfs rw\n")
	r.now = r.now.Add(30 * time.Second)
	got := r.Read().Filesystems
	awaitWorkers(t, r)
	if !reflect.DeepEqual(got, want) {
		t.Errorf("launch Sample filesystems: got %+v, want %+v", got, want)
	}
}

func TestFastRefreshNeverReportsUnavailable(t *testing.T) {
	var calls atomic.Uint64
	r, _ := newDiskRecorder(rootMount, func(string) (uint64, error) { return 2, nil }, func(string) (probe.FsStat, error) {
		n := calls.Add(1)
		return probe.FsStat{Frsize: 1, Blocks: 100 * n, Bfree: 5 * n, Bavail: 5 * n}, nil
	})
	awaitReading(t, r, func(v Reading) bool { return len(v.Filesystems) == 1 && v.Filesystems[0].Total != nil })
	check := func(v Reading, total uint64, age float64) {
		t.Helper()
		if len(v.Filesystems) != 1 || v.PrimaryDisk == nil {
			t.Fatal(v)
		}
		f := v.Filesystems[0]
		if f.Total == nil || *f.Total != total || f.Used == nil || f.Available == nil || f.UsedPct == nil || f.Age != age {
			t.Fatal("unexpected capacity or age", f, total, age)
		}
		if v.Verdict == nil || len(v.Verdict.Reasons) != 1 || v.Verdict.Reasons[0].Code != "filesystem_full" {
			t.Fatal("filesystem_full dropped out of the Verdict", v.Verdict)
		}
	}
	for n := uint64(1); n <= 3; n++ {
		r.now = r.now.Add(30 * time.Second)
		check(r.Read(), 100*n, 30)
		awaitWorkers(t, r)
		check(r.Read(), 100*(n+1), 0)
	}
}

func TestBlockedRefreshIsUnavailableFromTheSampleAfterItsLaunch(t *testing.T) {
	var block atomic.Bool
	started, release := make(chan struct{}), make(chan struct{})
	r, _ := newDiskRecorder(rootMount, func(string) (uint64, error) { return 2, nil }, func(string) (probe.FsStat, error) {
		if block.Load() {
			close(started)
			<-release
			return probe.FsStat{Frsize: 1, Blocks: 200}, nil
		}
		return probe.FsStat{Frsize: 1, Blocks: 100}, nil
	})
	var releaseOnce sync.Once
	defer releaseOnce.Do(func() { close(release) })
	awaitReading(t, r, func(v Reading) bool { return len(v.Filesystems) == 1 && v.Filesystems[0].Total != nil })
	block.Store(true)
	r.now = r.now.Add(30 * time.Second)
	if v := r.Read(); len(v.Filesystems) != 1 || v.Filesystems[0].Total == nil || *v.Filesystems[0].Total != 100 || v.Filesystems[0].Age != 30 {
		t.Fatal("launch Sample dropped the last capacity", v)
	}
	<-started
	r.now = r.now.Add(2 * time.Second)
	v := r.Read()
	if len(v.Filesystems) != 1 || v.PrimaryDisk == nil {
		t.Fatal("blocked refresh lost its primary filesystem row", v)
	}
	f := v.Filesystems[0]
	if f.Total != nil || f.Used != nil || f.Available != nil || f.UsedPct != nil || f.Age != 32 {
		t.Fatal("blocked refresh not unavailable or not aged from the last success", v)
	}
	releaseOnce.Do(func() { close(release) })
	awaitIdle(t, r)
	if v := r.Read(); len(v.Filesystems) != 1 || v.Filesystems[0].Total == nil || *v.Filesystems[0].Total != 200 || v.Filesystems[0].Age != 2 {
		t.Fatal("late success not published or not aged from its launch", v)
	}
}

func TestNewMountGenerationNeverInheritsCapacityAtLaunch(t *testing.T) {
	started, release := make(chan struct{}), make(chan struct{})
	var calls atomic.Int32
	r, f := newDiskRecorder(rootMount, func(p string) (uint64, error) {
		if calls.Load() > 0 {
			return 3, nil
		}
		return 2, nil
	}, func(string) (probe.FsStat, error) {
		if calls.Add(1) > 1 {
			close(started)
			<-release
		}
		return probe.FsStat{Frsize: 1, Blocks: 100}, nil
	})
	var releaseOnce sync.Once
	defer releaseOnce.Do(func() { close(release) })
	awaitReading(t, r, func(v Reading) bool { return len(v.Filesystems) == 1 && v.Filesystems[0].Total != nil })
	f.Data = []byte("3 1 0:35 /@ / rw - btrfs /dev/disk rw\n")
	r.now = r.now.Add(30 * time.Second)
	if v := r.Read(); len(v.Filesystems) != 0 || v.PrimaryDisk != nil {
		t.Fatal("new mount generation inherited capacity at launch", v)
	}
	<-started
	if v := awaitReading(t, r, func(v Reading) bool { return len(v.Filesystems) == 1 }); v.Filesystems[0].Total != nil || v.Filesystems[0].Age != 0 {
		t.Fatal("new mount generation inherited capacity or age", v)
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

func TestDelayedProbeKeepsItsMountGeneration(t *testing.T) {
	f := &fstest.MapFile{Data: []byte(rootMount)}
	r := &Recorder{host: probe.Host{
		FS: fstest.MapFS{"proc/self/mountinfo": f},
		OpenFilesystem: func(string) (probe.FilesystemHandle, error) {
			return probe.FilesystemHandle{
				MountID: 2,
				Statfs: func() (probe.FsStat, error) {
					return probe.FsStat{Frsize: 1, Blocks: 100}, nil
				},
				Close: func() error { return nil },
			}, nil
		},
	}}
	now := time.Unix(100, 0)
	// Holding the lock delays the worker before its mount check until the replacement has been observed.
	r.mu.Lock()
	r.filesystems(now)
	f.Data = []byte("3 1 0:35 /@ / rw - btrfs /dev/disk rw\n")
	r.filesystems(now.Add(31 * time.Second))
	r.mu.Unlock()
	deadline := time.Now().Add(time.Second)
	for !r.Idle() {
		if time.Now().After(deadline) {
			t.Fatal("filesystem worker did not finish")
		}
		time.Sleep(time.Millisecond)
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	values, primary := r.filesystems(now.Add(31 * time.Second))
	if len(values) != 0 || primary != nil {
		t.Fatal("replacement inherited obsolete mount capacity", values, primary)
	}
}
