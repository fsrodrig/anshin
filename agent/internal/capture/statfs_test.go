package capture

import "testing"

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
