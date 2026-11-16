package qa

import (
	"context"
	"time"

	"github.com/fsrodrig/anshin/agent/internal/recorder"
)

// liveRecorder takes a new Sample every 2 s of its clock, starting from the fixed recorder's 900 rows. Rows are
// derived from now() on each call rather than appended by a goroutine, so nothing outlives a request and the clock
// is the only seam a test needs.
type liveRecorder struct {
	cfg   AgentConfig
	start time.Time
	now   func() time.Time
	after func(time.Duration) <-chan time.Time
}

func newLiveRecorder(c AgentConfig, now func() time.Time, after func(time.Duration) <-chan time.Time) *liveRecorder {
	return &liveRecorder{cfg: c, start: now(), now: now, after: after}
}

func (r *liveRecorder) samples() int64 {
	return max(0, int64(r.now().Sub(r.start)/(sampleStepMs*time.Millisecond)))
}

func (r *liveRecorder) Read(c recorder.Cursor) recorder.Batch {
	return r.read(c, r.samples())
}

func (r *liveRecorder) read(c recorder.Cursor, n int64) recorder.Batch {
	newestSeq := uint64(windowRows + n)
	oldestSeq := newestSeq - windowRows + 1
	newest := syntheticNewestMs + n*sampleStepMs
	w := syntheticWindow(r.cfg, newest)
	b := recorder.Batch{Kind: recorder.Full, Cursor: recorder.Cursor{Epoch: syntheticEpoch, Seq: newestSeq}, Reading: flatReading(r.cfg, newest), Window: w}
	if c.Epoch == syntheticEpoch && c.Seq >= oldestSeq && c.Seq <= newestSeq {
		b.Kind, b.Window = recorder.Delta, w.Since(int(c.Seq-oldestSeq+1))
	}
	return b
}

func (r *liveRecorder) Wait(ctx context.Context, c recorder.Cursor) (recorder.Batch, error) {
	n := r.samples()
	if c.Epoch != syntheticEpoch || c.Seq != uint64(windowRows+n) {
		return r.read(c, n), nil
	}
	next := r.start.Add(time.Duration(n+1) * sampleStepMs * time.Millisecond)
	select {
	case <-ctx.Done():
		return recorder.Batch{}, ctx.Err()
	case <-r.after(next.Sub(r.now())):
		return r.Read(c), nil
	}
}
