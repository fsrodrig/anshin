package qa

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

func mustConfig(t *testing.T, args ...string) AgentConfig {
	t.Helper()
	c, _, err := ParseAgentFlags(append(args, "-listen", "127.0.0.1:0"), io.Discard)
	if err != nil {
		t.Fatal(err)
	}
	return c
}

// get sends a request the way the phone's connection arrives on the host route: from loopback.
func get(t *testing.T, h http.Handler, header http.Header) *httptest.ResponseRecorder {
	t.Helper()
	r := httptest.NewRequest(http.MethodGet, "/v1/pulse", nil)
	r.RemoteAddr = "127.0.0.1:40000"
	for k, v := range header {
		r.Header[k] = v
	}
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	return w
}

func TestPulseModeServesTheSyntheticMachineThroughTheProductionServer(t *testing.T) {
	cases := []struct {
		args []string
		want string
	}{
		{nil, `{"schema":1,"kind":"full","hostname":"qa-machine","cpu_pct":21}`},
		{[]string{"-cpu", "null"}, `{"schema":1,"kind":"full","hostname":"qa-machine","cpu_pct":null}`},
		{[]string{"-cpu", "zero"}, `{"schema":1,"kind":"full","hostname":"qa-machine","cpu_pct":0}`},
		{[]string{"-hostname", "null"}, `{"schema":1,"kind":"full","hostname":null,"cpu_pct":21}`},
	}
	for _, tc := range cases {
		w := get(t, NewAgentHandler(mustConfig(t, tc.args...)), nil)
		if w.Code != http.StatusOK || w.Body.String() != tc.want {
			t.Errorf("%v: got %d %s, want 200 %s", tc.args, w.Code, w.Body, tc.want)
		}
	}
}

func TestWhoIsModesUseTheProductionTruthTable(t *testing.T) {
	for args, want := range map[string]int{
		"allow":          http.StatusOK,
		"deny":           http.StatusForbidden,
		"localapi-error": http.StatusServiceUnavailable,
	} {
		w := get(t, NewAgentHandler(mustConfig(t, "-whois", args)), nil)
		if w.Code != want {
			t.Errorf("-whois %s: got %d, want %d", args, w.Code, want)
		}
	}
}

func TestAllowedQAAgentStillRefusesAnUnknownCaller(t *testing.T) {
	r := httptest.NewRequest(http.MethodGet, "/v1/pulse", nil)
	r.RemoteAddr = "100.64.0.99:40000"
	w := httptest.NewRecorder()
	NewAgentHandler(mustConfig(t)).ServeHTTP(w, r)
	if w.Code != http.StatusForbidden {
		t.Fatalf("got %d, want 403: allow mode must admit only the route's own source addresses", w.Code)
	}
}

func TestPulseModeKeepsTheProductionMethodRule(t *testing.T) {
	r := httptest.NewRequest(http.MethodPost, "/v1/pulse", nil)
	r.RemoteAddr = "127.0.0.1:40000"
	w := httptest.NewRecorder()
	NewAgentHandler(mustConfig(t)).ServeHTTP(w, r)
	if w.Code != http.StatusMethodNotAllowed {
		t.Fatalf("got %d, want 405", w.Code)
	}
}

func TestRawModeReplaysExactBytes(t *testing.T) {
	body := []byte("{\"schema\": 2,\n \"kind\":\"full\" }\x00\xff")
	p := filepath.Join(t.TempDir(), "future.json")
	os.WriteFile(p, body, 0o600)
	w := get(t, NewAgentHandler(mustConfig(t, "-mode", "raw", "-body", p)), nil)
	if w.Code != http.StatusOK || !bytes.Equal(w.Body.Bytes(), body) {
		t.Fatalf("got %d %q, want 200 and the file's exact bytes", w.Code, w.Body.Bytes())
	}
}

func TestRawModeAnswersAnExplicitStatus(t *testing.T) {
	w := get(t, NewAgentHandler(mustConfig(t, "-mode", "raw", "-status", "500")), nil)
	if w.Code != http.StatusInternalServerError || w.Body.Len() != 0 {
		t.Fatalf("got %d %q, want 500 with an empty body", w.Code, w.Body)
	}
}

func TestFlagsRejectAnythingThatWouldRunTheWrongScenario(t *testing.T) {
	bad := [][]string{
		{"-whois", "maybe"},
		{"-cpu", "high"},
		{"-mode", "replay"},
		{"-status", "500"},                 // status without raw would silently serve a Pulse
		{"-mode", "raw", "-whois", "deny"}, // raw bypasses authorization; a WhoIs flag would mislead
		{"-mode", "raw", "-status", "42"},
		{"-mode", "raw", "-body", "/nonexistent/file"},
		{"-delay", "-1s"},
		{"extra"},
	}
	for _, args := range bad {
		if _, _, err := ParseAgentFlags(append(args, "-listen", "127.0.0.1:0"), io.Discard); err == nil {
			t.Errorf("%v: accepted", args)
		}
	}
	if _, _, err := ParseAgentFlags(nil, io.Discard); err == nil {
		t.Error("no -listen: accepted")
	}
}

func TestEveryCapabilityFlagParses(t *testing.T) {
	body := filepath.Join(t.TempDir(), "b.json")
	os.WriteFile(body, []byte("{}"), 0o600)
	for _, c := range Capabilities() {
		if strings.HasPrefix(c.Flags, "helper ") {
			continue
		}
		args := strings.Fields(strings.NewReplacer("<file>", body, "<code>", "500", "<duration>", "2s", "[", "", "]", "").Replace(c.Flags))
		if _, _, err := ParseAgentFlags(append(args, "-listen", "127.0.0.1:0"), io.Discard); err != nil {
			t.Errorf("capability %s (%s): %v", c.ID, c.Flags, err)
		}
	}
}

func TestAccessLogRecordsHeaderNamesNeverValues(t *testing.T) {
	var buf bytes.Buffer
	h := NewAccessLog(&buf).Wrap(NewAgentHandler(mustConfig(t)))
	get(t, h, http.Header{"Authorization": {"Bearer secret-value"}, "User-Agent": {"okhttp"}})
	if strings.Contains(buf.String(), "secret-value") || strings.Contains(buf.String(), "okhttp") {
		t.Fatalf("a header value reached the log: %s", buf.String())
	}
	var e AccessEntry
	if err := json.Unmarshal(buf.Bytes(), &e); err != nil {
		t.Fatal(err)
	}
	if e.Method != "GET" || e.Path != "/v1/pulse" || e.Status != 200 || strings.Join(e.HeaderNames, ",") != "Authorization,User-Agent" {
		t.Fatalf("entry %+v", e)
	}
}

func TestDelayHoldsTheResponseAndLogsACanceledRequest(t *testing.T) {
	var buf syncBuffer
	cfg := mustConfig(t, "-delay", "5s")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	ready := make(chan netip.AddrPort, 1)
	done := make(chan error, 1)
	go func() { done <- ServeAgent(ctx, cfg, &buf, func(ap netip.AddrPort) { ready <- ap }) }()
	ap := <-ready
	client := http.Client{Timeout: 200 * time.Millisecond}
	start := time.Now()
	if _, err := client.Get("http://" + ap.String() + "/v1/pulse"); err == nil {
		t.Fatal("the delayed response arrived before the client's timeout")
	}
	if time.Since(start) > 2*time.Second {
		t.Fatal("the client waited far past its timeout")
	}
	deadline := time.Now().Add(2 * time.Second)
	for !strings.Contains(buf.String(), `"canceled":true`) {
		if time.Now().After(deadline) {
			t.Fatalf("no canceled entry in %q", buf.String())
		}
		time.Sleep(20 * time.Millisecond)
	}
	cancel()
	if err := <-done; err != nil {
		t.Fatal(err)
	}
}

type syncBuffer struct {
	mu sync.Mutex
	b  bytes.Buffer
}

func (s *syncBuffer) Write(p []byte) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.b.Write(p)
}

func (s *syncBuffer) String() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.b.String()
}

// The skill points runners at these files for Incompatible; they must stay what their names say.
func TestTestdataBodiesAreWhatTheSkillSaysTheyAre(t *testing.T) {
	var p struct {
		Schema int `json:"schema"`
	}
	b, err := os.ReadFile("testdata/schema-2.json")
	if err != nil || json.Unmarshal(b, &p) != nil || p.Schema != 2 {
		t.Fatalf("testdata/schema-2.json must be a schema 2 Pulse: %v %s", err, b)
	}
	b, _ = os.ReadFile("testdata/not-a-pulse.txt")
	if json.Valid(b) {
		t.Fatal("testdata/not-a-pulse.txt must not be JSON")
	}
	for _, name := range []string{"testdata/schema-2.json", "testdata/not-a-pulse.txt"} {
		want, _ := os.ReadFile(name)
		w := get(t, NewAgentHandler(mustConfig(t, "-mode", "raw", "-body", name)), nil)
		if !bytes.Equal(w.Body.Bytes(), want) {
			t.Errorf("%s: served %q", name, w.Body.Bytes())
		}
	}
}
