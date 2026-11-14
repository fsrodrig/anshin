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

	"github.com/fsrodrig/anshin/agent/internal/capture/replay"
	"github.com/fsrodrig/anshin/agent/internal/recorder"
	"github.com/fsrodrig/anshin/agent/internal/tailnet"
)

// update is defined here alone: `go test ./... -update` would fail in every package that doesn't define it.
var update = flag.Bool("update", false, "rewrite contract/fixtures from contract/captures")

const regenerate = "cd agent && go test ./internal/server -run '^TestGolden' -update"

// futureFixture is hand-written, so no capture produces it and -update never touches it.
const futureFixture = "future.json"

// TestGoldenFixtures replays each capture through the recorder and the real server, snapshot by snapshot, and
// compares the last Pulse with contract/fixtures/<capture>.json. No clock or epoch is on the wire yet; when the
// recorder takes time (#8), it gets seq.Clock() here, never the host's.
func TestGoldenFixtures(t *testing.T) {
	fixtures := filepath.Join(replay.ContractDir(t), "fixtures")
	for _, c := range replay.LoadAll(t) {
		t.Run(c.Name, func(t *testing.T) {
			seq := c.Sequence()
			_, _, url := serve(t, untaggedSelf, &tailnet.Node{UserID: owner}, recorder.New(seq.Host()))
			var body string
			for seq.Advance() {
				status, b := get(t, http.MethodGet, url+"/v1/pulse")
				if status != http.StatusOK {
					t.Fatalf("snapshot status = %d (%s)", status, b)
				}
				body = b
			}
			var got bytes.Buffer
			if err := json.Indent(&got, []byte(body), "", "  "); err != nil {
				t.Fatal(err)
			}
			got.WriteByte('\n')

			path := filepath.Join(fixtures, c.Name+".json")
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
				t.Fatalf("contract/fixtures/%s.json differs from what the Agent emits (- fixture, + Agent):\n%s\n"+
					"If the change is intended, regenerate with: %s\nthen read the diff before committing.",
					c.Name, lineDiff(string(want), got.String()), regenerate)
			}
		})
	}
}

// TestGoldenFixturesMatchCaptures keeps the fixture set equal to the capture set: a fixture without a capture can
// never be regenerated, so it would silently stop tracking the Agent.
func TestGoldenFixturesMatchCaptures(t *testing.T) {
	fixtures := filepath.Join(replay.ContractDir(t), "fixtures")
	captured := map[string]bool{}
	for _, c := range replay.LoadAll(t) {
		captured[c.Name+".json"] = true
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
