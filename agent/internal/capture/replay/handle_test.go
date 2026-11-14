package replay

import (
	"testing"

	"github.com/fsrodrig/anshin/agent/internal/capture"
)

func TestOpenFilesystemPinsSequenceSnapshot(t *testing.T) {
	id1, id2 := uint64(2), uint64(3)
	c := Capture{Snapshots: []Snapshot{
		{statfs: map[string]capture.StatfsRecord{"/": {MntID: &id1, Stat: &capture.Stat{Blocks: 100}}}},
		{statfs: map[string]capture.StatfsRecord{"/": {MntID: &id2, Stat: &capture.Stat{Blocks: 999}}}},
	}}
	seq := c.Sequence()
	handle, err := seq.Host().OpenFilesystem("/")
	if err != nil {
		t.Fatal(err)
	}
	defer handle.Close()
	if !seq.Advance() {
		t.Fatal("no replacement snapshot")
	}
	stat, err := handle.Statfs()
	if err != nil || handle.MountID != 2 || stat.Blocks != 100 {
		t.Fatalf("replaced opened mount: %+v %v", stat, err)
	}
	next, err := seq.Host().OpenFilesystem("/")
	if err != nil {
		t.Fatal(err)
	}
	defer next.Close()
	stat, err = next.Statfs()
	if err != nil || next.MountID != 3 || stat.Blocks != 999 {
		t.Fatalf("new handle did not see replacement: %+v %v", stat, err)
	}
	if len(seq.StatfsCalls()) != 2 {
		t.Fatal(seq.StatfsCalls())
	}
}
