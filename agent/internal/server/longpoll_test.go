package server

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"testing"
	"testing/fstest"
	"time"

	"github.com/fsrodrig/anshin/agent/internal/recorder"
	"github.com/fsrodrig/anshin/agent/internal/recorder/probe"
	"github.com/fsrodrig/anshin/agent/internal/tailnet"
)

func at(s int) recorder.Instant {
	d := time.Duration(s) * time.Second
	return recorder.Instant{Monotonic: d, Boottime: d}
}

// scripted is a real recorder on a synthetic host, so Wait is the production one.
func scripted(t *testing.T) *recorder.Recorder {
	t.Helper()
	h := probe.Host{FS: fstest.MapFS{"proc/meminfo": &fstest.MapFile{Data: []byte("MemTotal: 1000 kB\nMemAvailable: 600 kB\n")}}}
	return recorder.Open(h, at(1), recorder.Config{Epoch: 1})
}

type answer struct {
	status int
	body   string
	err    error
}

func getAsync(ctx context.Context, url string) <-chan answer {
	done := make(chan answer, 1)
	go func() {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
		if err != nil {
			done <- answer{err: err}
			return
		}
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			done <- answer{err: err}
			return
		}
		defer resp.Body.Close()
		b, err := io.ReadAll(resp.Body)
		done <- answer{resp.StatusCode, string(b), err}
	}()
	return done
}

type wire struct {
	Kind   string `json:"kind"`
	Cursor string `json:"cursor"`
	Window struct {
		BoottimeMs []*int64 `json:"boottime_ms"`
	} `json:"window"`
}

func decode(t *testing.T, a answer) wire {
	t.Helper()
	if a.err != nil || a.status != http.StatusOK {
		t.Fatalf("status %d, %v: %s", a.status, a.err, a.body)
	}
	var w wire
	if err := json.Unmarshal([]byte(a.body), &w); err != nil {
		t.Fatal(err)
	}
	return w
}

func TestLongPollReturnsWhenTheNextSampleLands(t *testing.T) {
	rec := scripted(t)
	_, _, base := serve(t, untaggedSelf, &tailnet.Node{UserID: owner}, rec)
	done := getAsync(context.Background(), base+"/v1/pulse?after=0000000000000001.1&wait=10")
	select {
	case a := <-done:
		t.Fatalf("answered before the next Sample: %d %s", a.status, a.body)
	case <-time.After(200 * time.Millisecond):
	}
	rec.Sample(at(3))
	select {
	case a := <-done:
		if w := decode(t, a); w.Kind != "delta" || w.Cursor != "0000000000000001.2" || len(w.Window.BoottimeMs) != 1 {
			t.Fatalf("long-poll = %s", a.body)
		}
	case <-time.After(time.Second):
		t.Fatal("the long-poll did not answer within 1 s of the next Sample")
	}
}

func TestLongPollTimesOutWithAnEmptyDelta(t *testing.T) {
	_, _, base := serve(t, untaggedSelf, &tailnet.Node{UserID: owner}, scripted(t))
	start := time.Now()
	w := decode(t, <-getAsync(context.Background(), base+"/v1/pulse?after=0000000000000001.1&wait=1"))
	if took := time.Since(start); took < time.Second || took > 3*time.Second {
		t.Fatalf("wait=1 answered after %v", took)
	}
	if w.Kind != "delta" || w.Cursor != "0000000000000001.1" || w.Window.BoottimeMs == nil || len(w.Window.BoottimeMs) != 0 {
		t.Fatalf("expired wait = %+v, want an empty delta at the same Cursor", w)
	}
}

// heldRecorder holds every Wait until its context ends, recording what happened.
type heldRecorder struct {
	fixedRecorder
	entered, cancelled, returned chan struct{}
	deadline                     chan time.Duration
}

func newHeldRecorder() *heldRecorder {
	return &heldRecorder{fixedRecorder{reading}, make(chan struct{}, 1), make(chan struct{}, 1), make(chan struct{}, 1), make(chan time.Duration, 1)}
}

func (h *heldRecorder) Wait(ctx context.Context, c recorder.Cursor) (recorder.Batch, error) {
	defer func() { h.returned <- struct{}{} }()
	if d, ok := ctx.Deadline(); ok {
		h.deadline <- time.Until(d)
	}
	h.entered <- struct{}{}
	<-ctx.Done()
	h.cancelled <- struct{}{}
	return recorder.Batch{}, ctx.Err()
}

func signalled(t *testing.T, ch <-chan struct{}, what string) {
	t.Helper()
	select {
	case <-ch:
	case <-time.After(time.Second):
		t.Fatalf("%s within 1 s", what)
	}
}

func TestLongPollStopsWhenTheClientCancels(t *testing.T) {
	rec := newHeldRecorder()
	_, _, base := serve(t, untaggedSelf, &tailnet.Node{UserID: owner}, rec)
	ctx, cancel := context.WithCancel(context.Background())
	done := getAsync(ctx, base+"/v1/pulse?after=0000000000000001.1&wait=30")
	signalled(t, rec.entered, "Wait not entered")
	cancel()
	signalled(t, rec.cancelled, "the held request's context was not cancelled")
	signalled(t, rec.returned, "Wait did not return")
	if a := <-done; !errors.Is(a.err, context.Canceled) {
		t.Fatalf("client = %+v, want its own cancellation", a)
	}
}

func TestRunEndsHeldLongPollsOnShutdown(t *testing.T) {
	rec := newHeldRecorder()
	tn := tailnet.NewFake(untaggedSelf)
	tn.SetPeer(callerAddr, tailnet.Node{UserID: owner})
	ln, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	served := make(chan error, 1)
	go func() {
		served <- serveUntil(ctx, []net.Listener{ln}, New(tn, untaggedSelf, rec), func(ctx context.Context) error { <-ctx.Done(); return nil })
	}()
	done := getAsync(context.Background(), "http://"+ln.Addr().String()+"/v1/pulse?after=0000000000000001.1&wait=30")
	signalled(t, rec.entered, "Wait not entered")
	start := time.Now()
	cancel()
	select {
	case err := <-served:
		if err != nil {
			t.Fatalf("serveUntil = %v, want nil", err)
		}
	case <-time.After(time.Second):
		t.Fatal("shutdown waited out a held long-poll")
	}
	if a := <-done; a.err != nil || a.status != http.StatusOK || !strings.Contains(a.body, `"cursor":"0000000000000001.1"`) {
		t.Fatalf("held client got %+v, want a 200 Pulse", a)
	}
	if took := time.Since(start); took > time.Second {
		t.Fatalf("shutdown took %v", took)
	}
}

func TestPulseQueryErrors(t *testing.T) {
	rec := newHeldRecorder()
	_, _, base := serve(t, untaggedSelf, &tailnet.Node{UserID: owner}, rec)
	_, _, refused := serve(t, untaggedSelf, &tailnet.Node{UserID: stranger}, rec)
	bad := []string{
		"after=", "wait=", "after", "wait", "after=x", "after=.", "after=0000000000000001", "after=0000000000000001.",
		"after=000000000000001.1", "after=00000000000000001.1", "after=000000000000000A.1", "after=000000000000000g.1",
		"after=0000000000000001.01", "after=0000000000000001.%2B1", "after=0000000000000001.1.1", "after=%200000000000000001.1",
		"after=0000000000000001.18446744073709551616",
		"after=0000000000000001.1&after=0000000000000001.1", "wait=1&wait=1",
		"wait=-1", "wait=%2B1", "wait=1.5", "wait=1s", "wait=%201", "wait=4294967296", "wait=99999999999999999999",
		"after=%zz", "after=0000000000000001.1;wait=1",
	}
	for _, q := range bad {
		if status, body := get(t, http.MethodGet, base+"/v1/pulse?"+q); status != http.StatusBadRequest {
			t.Errorf("?%s = %d (%s), want 400", q, status, body)
		}
		if status, _ := get(t, http.MethodGet, refused+"/v1/pulse?"+q); status != http.StatusForbidden {
			t.Errorf("?%s from a stranger = %d, want 403 before the query is read", q, status)
		}
	}
	for _, q := range []string{"", "after=0000000000000001.1", "wait=0", "after=ffffffffffffffff.18446744073709551615&wait=0", "unknown=1"} {
		if status, body := get(t, http.MethodGet, base+"/v1/pulse?"+q); status != http.StatusOK {
			t.Errorf("?%s = %d (%s), want 200", q, status, body)
		}
	}
	select {
	case <-rec.entered:
		t.Fatal("wait=0 held the request")
	default:
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := getAsync(ctx, base+"/v1/pulse?"+url.Values{"after": {"0000000000000001.1"}, "wait": {"999"}}.Encode())
	select {
	case d := <-rec.deadline:
		if d > maxWait || d < maxWait-5*time.Second {
			t.Fatalf("wait=999 held for %v, want at most %v", d, maxWait)
		}
	case <-time.After(time.Second):
		t.Fatal("wait=999 was not held")
	}
	cancel()
	<-done
}
