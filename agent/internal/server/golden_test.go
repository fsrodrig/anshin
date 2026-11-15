package server

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/fsrodrig/anshin/agent/internal/capture"
	"github.com/fsrodrig/anshin/agent/internal/capture/replay"
	"github.com/fsrodrig/anshin/agent/internal/recorder"
	"github.com/fsrodrig/anshin/agent/internal/tailnet"
)

// update is defined here alone: `go test ./... -update` would fail in every package that doesn't define it.
var update = flag.Bool("update", false, "rewrite contract/fixtures from contract/captures")

const regenerate = "cd agent && go test ./internal/server -run '^TestGolden' -update"

// futureFixture is hand-written, so no capture produces it and -update never touches it.
const futureFixture = "future.json"

// suspendBefore is the scripted suspend of <capture>.suspend.json: CLOCK_BOOTTIME runs this far ahead of
// CLOCK_MONOTONIC from the last snapshot on, as if the Machine slept just before it.
const suspendBefore = 300 * time.Second

func instantOf(c capture.Clock, slept time.Duration) recorder.Instant {
	return recorder.Instant{Monotonic: time.Duration(c.MonotonicNs), Boottime: time.Duration(c.BoottimeNs) + slept}
}

func awaitIdle(t *testing.T, rec *recorder.Recorder) {
	t.Helper()
	for deadline := time.Now().Add(5 * time.Second); !rec.Idle(); time.Sleep(time.Millisecond) {
		if time.Now().After(deadline) {
			t.Fatal("filesystem replay did not settle")
		}
	}
}

// replayed takes one Sample per snapshot of c, at the capture's own clocks, and serves the result. The recorder's
// time is never the host's, so rates divide by the interval the capture really spanned.
func replayed(t *testing.T, c *replay.Capture, suspend bool) (url string, samples int) {
	t.Helper()
	seq := c.Sequence()
	h := seq.Host()
	// Bind asynchronous slow reads to an immutable snapshot before the mutable sequence advances.
	slow := c.Snapshots[0].Host()
	h.MountID, h.Statfs, h.OpenFilesystem = slow.MountID, slow.Statfs, slow.OpenFilesystem
	rec := recorder.Open(h, instantOf(seq.Clock(), 0), recorder.Config{Epoch: 1})
	awaitIdle(t, rec)
	samples = 1
	for seq.Advance() {
		samples++
		var slept time.Duration
		if suspend && samples == len(c.Snapshots) {
			slept = suspendBefore
		}
		rec.Sample(instantOf(seq.Clock(), slept))
		awaitIdle(t, rec)
	}
	_, _, url = serve(t, untaggedSelf, &tailnet.Node{UserID: owner}, rec)
	return url, samples
}

// TestGoldenFixtures replays each capture through the recorder and the real server and compares what it serves with
// contract/fixtures: <capture>.json is the full Pulse after the last snapshot, <capture>.delta.json the delta that
// answers the second-to-last Sample's Cursor, and <capture>.suspend.json the full Pulse of the same replay with a
// suspend before the last snapshot.
func TestGoldenFixtures(t *testing.T) {
	fixtures := filepath.Join(replay.ContractDir(t), "fixtures")
	for _, c := range replay.LoadAll(t) {
		t.Run(c.Name, func(t *testing.T) {
			url, n := replayed(t, c, false)
			golden(t, fixtures, c.Name+".json", pulseAt(t, url+"/v1/pulse"))
			golden(t, fixtures, c.Name+".delta.json", pulseAt(t, fmt.Sprintf("%s/v1/pulse?after=%s", url, recorder.Cursor{Epoch: 1, Seq: uint64(n - 1)})))
			url, _ = replayed(t, c, true)
			golden(t, fixtures, c.Name+".suspend.json", pulseAt(t, url+"/v1/pulse"))
		})
	}
}

func pulseAt(t *testing.T, url string) string {
	t.Helper()
	status, body := get(t, http.MethodGet, url)
	if status != http.StatusOK {
		t.Fatalf("GET %s = %d (%s)", url, status, body)
	}
	return body
}

func golden(t *testing.T, fixtures, name, body string) {
	t.Helper()
	var got bytes.Buffer
	if err := json.Indent(&got, []byte(body), "", "  "); err != nil {
		t.Fatal(err)
	}
	got.WriteByte('\n')
	path := filepath.Join(fixtures, name)
	if *update {
		if err := os.MkdirAll(fixtures, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, got.Bytes(), 0o644); err != nil {
			t.Fatal(err)
		}
		return
	}
	want, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("%v\nregenerate with: %s", err, regenerate)
	}
	if !bytes.Equal(want, got.Bytes()) {
		t.Fatalf("contract/fixtures/%s differs from what the Agent emits (- fixture, + Agent):\n%s\n"+
			"If the change is intended, regenerate with: %s\nthen read the diff before committing.",
			name, lineDiff(string(want), got.String()), regenerate)
	}
}

// TestGoldenFixturesMatchCaptures keeps the fixture set equal to the capture set: a fixture without a capture can
// never be regenerated, so it would silently stop tracking the Agent.
func TestGoldenFixturesMatchCaptures(t *testing.T) {
	fixtures := filepath.Join(replay.ContractDir(t), "fixtures")
	captured := map[string]bool{}
	for _, c := range replay.LoadAll(t) {
		for _, suffix := range []string{".json", ".delta.json", ".suspend.json"} {
			captured[c.Name+suffix] = true
		}
	}
	files, err := filepath.Glob(filepath.Join(fixtures, "*.json"))
	if err != nil {
		t.Fatal(err)
	}
	for _, f := range files {
		name := filepath.Base(f)
		if name == futureFixture || captured[name] {
			continue
		}
		if *update {
			if err := os.Remove(f); err != nil {
				t.Fatal(err)
			}
			continue
		}
		t.Errorf("contract/fixtures/%s has no capture in contract/captures; regenerate with: %s", name, regenerate)
	}
}

func lineDiff(a, b string) string {
	x, y := strings.Split(a, "\n"), strings.Split(b, "\n")
	lcs := make([][]int, len(x)+1)
	for i := range lcs {
		lcs[i] = make([]int, len(y)+1)
	}
	for i := len(x) - 1; i >= 0; i-- {
		for j := len(y) - 1; j >= 0; j-- {
			if x[i] == y[j] {
				lcs[i][j] = lcs[i+1][j+1] + 1
			} else {
				lcs[i][j] = max(lcs[i+1][j], lcs[i][j+1])
			}
		}
	}
	var out strings.Builder
	i, j := 0, 0
	for i < len(x) || j < len(y) {
		switch {
		case i < len(x) && j < len(y) && x[i] == y[j]:
			fmt.Fprintf(&out, "  %s\n", x[i])
			i, j = i+1, j+1
		case j < len(y) && (i == len(x) || lcs[i][j+1] >= lcs[i+1][j]):
			fmt.Fprintf(&out, "+ %s\n", y[j])
			j++
		default:
			fmt.Fprintf(&out, "- %s\n", x[i])
			i++
		}
	}
	return out.String()
}
