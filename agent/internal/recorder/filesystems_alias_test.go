package recorder

import (
	"github.com/fsrodrig/anshin/agent/internal/recorder/probe"
	"sync/atomic"
	"testing"
	"time"
)

func TestBlockedRootSurvivesAddedAlias(t *testing.T) {
	started, release := make(chan struct{}), make(chan struct{})
	var calls atomic.Int32
	r, table := diskRecorder(rootMount, func(string) (uint64, error) { return 2, nil }, func(string) (probe.FsStat, error) {
		calls.Add(1)
		close(started)
		<-release
		return probe.FsStat{Frsize: 1, Blocks: 100}, nil
	})
	defer close(release)
	now := time.Unix(100, 0)
	r.now = func() time.Time { return now }
	r.Read()
	<-started
	before := awaitReading(t, r, func(v Reading) bool { return len(v.Filesystems) == 1 })
	if before.PrimaryDisk == nil {
		t.Fatal("baseline missing root")
	}
	table.Data = []byte(rootMount + "3 2 0:35 /@home /home rw - btrfs /dev/disk rw\n")
	for i := 0; i < 3; i++ {
		now = now.Add(30 * time.Second)
		got := r.Read()
		t.Logf("elapsed=%ds rows=%d primary=%v calls=%d", (i+1)*30, len(got.Filesystems), got.PrimaryDisk, calls.Load())
		if len(got.Filesystems) != 1 || got.PrimaryDisk == nil || got.Filesystems[0].Age != float64((i+1)*30) || got.Filesystems[0].Total != nil || calls.Load() != 1 {
			t.Errorf("unchanged, verified root disappears when sibling alias is added")
		}
	}
}
