package recorder

import (
	"sync/atomic"
	"testing"

	"github.com/fsrodrig/anshin/agent/internal/recorder/probe"
)

func TestReplacementBetweenIdentityAndCapacity(t *testing.T) {
	var mounted atomic.Bool
	mounted.Store(true)
	var closed atomic.Int32
	r, _ := newDiskRecorder("3 2 8:17 / /mnt/archive rw - ext4 disk rw\n", func(string) (uint64, error) { return 3, nil }, func(string) (probe.FsStat, error) {
		mounted.Store(false)
		return probe.FsStat{Frsize: 1, Blocks: 999}, nil
	})
	r.host.MountID = func(string) (uint64, error) { return 3, nil }
	r.host.Statfs = func(string) (probe.FsStat, error) {
		mounted.Store(false)
		return probe.FsStat{Frsize: 1, Blocks: 999}, nil
	}
	r.host.OpenFilesystem = func(string) (probe.FilesystemHandle, error) {
		// The opened descriptor keeps the disk while the path starts resolving to the root.
		mounted.Store(false)
		return probe.FilesystemHandle{MountID: 3, Close: func() error { closed.Add(1); return nil }, Statfs: func() (probe.FsStat, error) { return probe.FsStat{Frsize: 1, Blocks: 100}, nil }}, nil
	}
	v := awaitReading(t, r, func(v Reading) bool { return len(v.Filesystems) == 1 && v.Filesystems[0].Total != nil })
	if *v.Filesystems[0].Total != 100 || mounted.Load() || closed.Load() != 1 {
		t.Fatalf("replacement capacity or leaked handle: %+v closed=%d", v, closed.Load())
	}
}

func TestHiddenFilesystemClosesHandleWithoutCapacity(t *testing.T) {
	var closed atomic.Int32
	r, _ := newDiskRecorder(rootMount, nil, nil)
	r.host.OpenFilesystem = func(string) (probe.FilesystemHandle, error) {
		return probe.FilesystemHandle{MountID: 99, Close: func() error { closed.Add(1); return nil }, Statfs: func() (probe.FsStat, error) { t.Error("hidden capacity read"); return probe.FsStat{}, nil }}, nil
	}
	r.Read()
	awaitIdle(t, r)
	if len(r.Read().Filesystems) != 0 || closed.Load() != 1 {
		t.Fatal("hidden handle not discarded", closed.Load())
	}
}
