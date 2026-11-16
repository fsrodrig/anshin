package qa

import (
	"context"
	"fmt"
	"net/http"
	"net/netip"
	"sync"
	"testing"
	"time"

	"github.com/fsrodrig/anshin/agent/internal/recorder"
	"github.com/fsrodrig/anshin/agent/internal/server"
	"github.com/fsrodrig/anshin/agent/internal/tailnet"
)

type fakeClock struct {
	mu    sync.Mutex
	now   time.Time
	asked chan time.Duration
	fire  chan time.Time
}

func newFakeClock() *fakeClock {
	return &fakeClock{now: time.Date(2026, 11, 15, 12, 0, 0, 0, time.UTC), asked: make(chan time.Duration, 1), fire: make(chan time.Time)}
}

func (c *fakeClock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.now
}

func (c *fakeClock) advance(d time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.now = c.now.Add(d)
}

func (c *fakeClock) After(d time.Duration) <-chan time.Time {
	c.asked <- d
	return c.fire
}

func liveHandler(t *testing.T, clock *fakeClock, args ...string) http.Handler {
	t.Helper()
	c := mustConfig(t, append(args, "-samples", "live")...)
	self := tailnet.Self{Node: tailnet.Node{UserID: qaUserID}, Addrs: []netip.Addr{c.Listen.Addr()}}
	tn := tailnet.NewFake(self)
	tn.SetPeer(netip.MustParseAddr("127.0.0.1"), tailnet.Node{UserID: qaUserID})
	return server.New(tn, self, recorderFor(c, clock.Now, clock.After))
}

func rowsOf(p map[string]any) []any { return p["window"].(map[string]any)["boottime_ms"].([]any) }

func TestLiveSamplesSlideTheWindowEvery2s(t *testing.T) {
	for _, machine := range []string{"minimal", "full"} {
		clock := newFakeClock()
		h := liveHandler(t, clock, "-machine", machine)
		first := pulseOf(t, h, "")
		lastRowIsFlat(t, first)
		if first["cursor"] != "0000000000000002.900" || len(rowsOf(first)) != 900 {
			t.Fatalf("%s: started at %v with %d rows, want the fixed Window", machine, first["cursor"], len(rowsOf(first)))
		}

		clock.advance(2 * time.Second)
		delta := pulseOf(t, h, "?after=0000000000000002.900")
		rows := rowsOf(delta)
		if delta["kind"] != "delta" || delta["cursor"] != "0000000000000002.901" || len(rows) != 1 || rows[0] != 3_602_000.0 {
			t.Errorf("%s: +2 s = %v %v %v, want a one-row delta at boottime 3602000", machine, delta["kind"], delta["cursor"], rows)
		}
		lastRowIsFlat(t, delta)
		if machine == "full" && delta["cpu_pct"] == first["cpu_pct"] && delta["net_rx_bps"] == first["net_rx_bps"] {
			t.Errorf("%s: the values didn't change with the Sample", machine)
		}

		clock.advance(time.Second)
		if same := pulseOf(t, h, "?after=0000000000000002.901"); same["kind"] != "delta" || len(rowsOf(same)) != 0 || same["cursor"] != "0000000000000002.901" {
			t.Errorf("%s: +1 s more = %v %v with %d rows, want an empty delta", machine, same["kind"], same["cursor"], len(rowsOf(same)))
		}

		clock.advance(2000 * time.Second)
		full := pulseOf(t, h, "?after=0000000000000002.901")
		rows = rowsOf(full)
		if full["kind"] != "full" || full["cursor"] != "0000000000000002.1901" || len(rows) != 900 {
			t.Fatalf("%s: after +2000 s an evicted Cursor got %v %v with %d rows, want a full Window of 900", machine, full["kind"], full["cursor"], len(rows))
		}
		for i := 1; i < len(rows); i++ {
			if rows[i].(float64)-rows[i-1].(float64) != 2000 {
				t.Fatalf("%s: rows %d and %d are %v and %v, want 2 s apart", machine, i-1, i, rows[i-1], rows[i])
			}
		}
		if rows[899] != 3_600_000.0+1001*2000 {
			t.Errorf("%s: newest row at %v", machine, rows[899])
		}
		lastRowIsFlat(t, full)
		if older := pulseOf(t, h, "?after=0000000000000002.1002"); older["kind"] != "delta" || len(rowsOf(older)) != 899 {
			t.Errorf("%s: the oldest retained Cursor got %v with %d rows, want a delta of the other 899", machine, older["kind"], len(rowsOf(older)))
		}
		if evicted := pulseOf(t, h, "?after=0000000000000002.1001"); evicted["kind"] != "full" {
			t.Errorf("%s: an evicted Cursor got %v", machine, evicted["kind"])
		}
		if future := pulseOf(t, h, "?after=0000000000000002.1902"); future["kind"] != "full" {
			t.Errorf("%s: a future Cursor got %v", machine, future["kind"])
		}
		if other := pulseOf(t, h, "?after=0000000000000001.1901"); other["kind"] != "full" {
			t.Errorf("%s: another epoch got %v", machine, other["kind"])
		}
	}
}

func TestLiveWaitReturnsAtTheNextSample(t *testing.T) {
	clock := newFakeClock()
	r := newLiveRecorder(mustConfig(t, "-samples", "live"), clock.Now, clock.After)
	clock.advance(2500 * time.Millisecond)
	newest := recorder.Cursor{Epoch: syntheticEpoch, Seq: 901}

	got := make(chan recorder.Batch)
	go func() {
		b, err := r.Wait(context.Background(), newest)
		if err != nil {
			t.Error(err)
		}
		got <- b
	}()
	if d := <-clock.asked; d != 1500*time.Millisecond {
		t.Errorf("waited %v, want 1.5 s to the next Sample", d)
	}
	clock.advance(1500 * time.Millisecond)
	clock.fire <- clock.Now()
	if b := <-got; b.Kind != recorder.Delta || b.Cursor.Seq != 902 || len(b.Window.BoottimeMs) != 1 {
		t.Errorf("woke with %v at seq %d and %d rows, want a one-row delta", b.Kind, b.Cursor.Seq, len(b.Window.BoottimeMs))
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := r.Wait(ctx, recorder.Cursor{Epoch: syntheticEpoch, Seq: 902}); err != context.Canceled {
		t.Errorf("a canceled hold returned %v", err)
	}
	for _, c := range []recorder.Cursor{{Epoch: syntheticEpoch, Seq: 901}, {Epoch: 1, Seq: 902}} {
		if b, err := r.Wait(context.Background(), c); err != nil || b.Cursor.Seq != 902 {
			t.Errorf("%s: held or failed (%v); only the newest Cursor is held", fmt.Sprint(c), err)
		}
	}
}
