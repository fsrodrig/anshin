package capture

import (
	"github.com/fsrodrig/anshin/agent/internal/recorder/probe"
	"testing"
	"time"
)

func TestPendingStatfsChecksCurrentMountID(t *testing.T) {
	for _, id := range []uint64{1, 2} {
		done := make(chan StatfsRecord, 1)
		done <- StatfsRecord{Stat: &Stat{Blocks: 100}, MntID: &id}
		c := capturer{pending: map[string]chan StatfsRecord{"/": done}}
		r := c.statfs("/", 2)
		if id == 1 {
			if r.Skipped != "hidden/unverified" || r.Stat != nil {
				t.Fatal("returned a replaced mount's measurements")
			}
		} else if r.Stat == nil || r.Blocks != 100 {
			t.Fatal("discarded matching mount")
		}
	}
}

func TestCaptureCapacityUsesOpenedMount(t *testing.T) {
	closed := make(chan struct{}, 1)
	c := capturer{opt: Options{StatfsTimeout: time.Second}, src: Source{Host: probe.Host{OpenFilesystem: func(string) (probe.FilesystemHandle, error) {
		return probe.FilesystemHandle{MountID: 2, Close: func() error { closed <- struct{}{}; return nil }, Statfs: func() (probe.FsStat, error) { return probe.FsStat{Blocks: 100}, nil }}, nil
	}, Statfs: func(string) (probe.FsStat, error) {
		t.Error("resolved path again")
		return probe.FsStat{Blocks: 999}, nil
	}}}}
	r := c.statfs("/", 2)
	if r.Stat == nil || r.Blocks != 100 || r.MntID == nil || *r.MntID != 2 {
		t.Fatal(r)
	}
	select {
	case <-closed:
	case <-time.After(time.Second):
		t.Fatal("handle not closed")
	}
}
