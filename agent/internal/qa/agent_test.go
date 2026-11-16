package qa

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/fsrodrig/anshin/agent/internal/recorder"
)

// stillHandler is a QA Agent whose clock never moves, so its Window is the base synthetic Window however slowly the
// test runs.
func stillHandler(c AgentConfig) http.Handler {
	at := time.Unix(1_000_000, 0)
	return newAgentHandler(c, func() time.Time { return at })
}

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
		{[]string{"-battery", "discharging"}, `{"schema":1,"kind":"full","hostname":"qa-machine","cpu_pct":21,"verdict":{"level":"ok","reasons":[]},"battery":{"pct":64,"status":"Discharging","power_w":7.5,"time_left_s":9000,"batteries":[{"name":"BAT0","pct":64,"status":"Discharging","power_w":7.5,"health_pct":87.5}]}}`},
		{[]string{"-battery", "charging"}, `{"schema":1,"kind":"full","hostname":"qa-machine","cpu_pct":21,"verdict":{"level":"ok","reasons":[]},"battery":{"pct":64,"status":"Charging","power_w":15,"time_left_s":null,"batteries":[{"name":"BAT0","pct":64,"status":"Charging","power_w":15,"health_pct":87.5}]}}`},
		{[]string{"-battery", "unavailable"}, `{"schema":1,"kind":"full","hostname":"qa-machine","cpu_pct":21,"battery":{"pct":null,"status":null,"power_w":null,"time_left_s":null,"batteries":null}}`},
		{[]string{"-verdict", "null"}, `{"schema":1,"kind":"full","hostname":"qa-machine","cpu_pct":21,"verdict":null}`},
		{[]string{"-verdict", "ok"}, `{"schema":1,"kind":"full","hostname":"qa-machine","cpu_pct":21,"verdict":{"level":"ok","reasons":[]}}`},
		{[]string{"-verdict", "warn"}, `{"schema":1,"kind":"full","hostname":"qa-machine","cpu_pct":21,"verdict":{"level":"warn","reasons":[{"code":"filesystem_full","level":"warn","message":"/ is 91.2 % full"}]}}`},
		{[]string{"-verdict", "crit"}, `{"schema":1,"kind":"full","hostname":"qa-machine","cpu_pct":21,"verdict":{"level":"crit","reasons":[{"code":"cpu_hot","level":"crit","message":"CPU at 97.0 °C (crit at 95.0 °C)"},{"code":"filesystem_full","level":"warn","message":"/ is 91.2 % full"}]}}`},
		{[]string{"-verdict", "unknown"}, `{"schema":1,"kind":"full","hostname":"qa-machine","cpu_pct":21,"verdict":{"level":"severe","reasons":[{"code":"fan_stalled","level":"severe","message":"A fan stopped"}]}}`},
		{[]string{"-verdict", "warn", "-battery", "discharging"}, `{"schema":1,"kind":"full","hostname":"qa-machine","cpu_pct":21,"verdict":{"level":"warn","reasons":[{"code":"filesystem_full","level":"warn","message":"/ is 91.2 % full"}]},"battery":{"pct":64,"status":"Discharging","power_w":7.5,"time_left_s":9000,"batteries":[{"name":"BAT0","pct":64,"status":"Discharging","power_w":7.5,"health_pct":87.5}]}}`},
	}
	for _, tc := range cases {
		w := get(t, stillHandler(mustConfig(t, tc.args...)), nil)
		var got, want map[string]any
		if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
			t.Fatal(err)
		}
		if err := json.Unmarshal([]byte(tc.want), &want); err != nil {
			t.Fatal(err)
		}
		window, _ := got["window"].(map[string]any)
		times, _ := window["boottime_ms"].([]any)
		if got["cursor"] != "0000000000000002.900" || len(times) != 900 {
			t.Errorf("%v: cursor %v with %d rows, want a full Window of 900 Samples", tc.args, got["cursor"], len(times))
		}
		lastRowIsFlat(t, got)
		delete(got, "cursor")
		delete(got, "window")
		for _, k := range strings.Fields("mem_total_bytes mem_available_bytes mem_used_bytes mem_used_pct swap_total_bytes swap_used_bytes swap_entries load_1 load_5 load_15 uptime_s distro kernel arch agent_version cpu_temp_c cpu_sensor temperatures filesystems primary_disk net_rx_bps net_tx_bps net_interfaces disks battery verdict cpu_core_pct disk_read_bps disk_write_bps") {
			if _, set := want[k]; !set {
				want[k] = nil
			}
		}
		if w.Code != http.StatusOK || !reflect.DeepEqual(got, want) {
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

// lastRowIsFlat is invariant 3 of contract/README.md § Window.
func lastRowIsFlat(t *testing.T, pulse map[string]any) {
	t.Helper()
	flat := map[string]any{}
	for k, v := range pulse {
		flat[k] = v
	}
	if b, ok := pulse["battery"].(map[string]any); ok {
		flat["battery_pct"], flat["battery_power_w"] = b["pct"], b["power_w"]
	} else {
		flat["battery_pct"], flat["battery_power_w"] = nil, nil
	}
	window, _ := pulse["window"].(map[string]any)
	for name, column := range window {
		series, ok := column.([]any)
		if name == "boottime_ms" || name == "cpu_core_pct" || !ok {
			continue
		}
		if len(series) > 0 && !reflect.DeepEqual(series[len(series)-1], flat[name]) {
			t.Errorf("window.%s ends %v, flat %v", name, series[len(series)-1], flat[name])
		}
	}
	if window["cpu_core_pct"] != nil || pulse["cpu_core_pct"] != nil {
		t.Errorf("per-core = %v, %v; want null on the synthetic Machine", window["cpu_core_pct"], pulse["cpu_core_pct"])
	}
}

func pulseOf(t *testing.T, h http.Handler, query string) map[string]any {
	t.Helper()
	r := httptest.NewRequest(http.MethodGet, "/v1/pulse"+query, nil)
	r.RemoteAddr = "127.0.0.1:40000"
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	var got map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &got); w.Code != http.StatusOK || err != nil {
		t.Fatalf("GET %s = %d %s", query, w.Code, w.Body)
	}
	return got
}

func TestSyntheticWindow(t *testing.T) {
	for _, args := range [][]string{nil, {"-window", "gap"}, {"-cpu", "null"}, {"-cpu", "zero", "-window", "gap"}, {"-battery", "discharging"}} {
		h := stillHandler(mustConfig(t, args...))
		full := pulseOf(t, h, "")
		lastRowIsFlat(t, full)
		window := full["window"].(map[string]any)
		times := window["boottime_ms"].([]any)
		cpu := window["cpu_pct"].([]any)
		if window["span_s"] != 1800.0 || times[len(times)-1] != 3_600_000.0 || len(cpu) != len(times) {
			t.Fatalf("%v: window = span %v ending %v", args, window["span_s"], times[len(times)-1])
		}
		gaps := 0
		for i, v := range times {
			if v == nil {
				gaps++
				if times[i-1] != 2_700_000.0 || times[i+1] != 3_000_000.0 || cpu[i+1] != nil {
					t.Errorf("%v: Gap from %v to %v, resume CPU %v; want 5 minutes ending 10 before the newest, then null", args, times[i-1], times[i+1], cpu[i+1])
				}
			} else if i > 0 && times[i-1] != nil && v.(float64)-times[i-1].(float64) != 2000 {
				t.Errorf("%v: Samples %v and %v are not 2 s apart", args, times[i-1], v)
			}
			if first := times[0]; i == 0 && first.(float64) <= 3_600_000-1_800_000 {
				t.Errorf("%v: the Window starts at %v, more than 30 minutes before the newest", args, first)
			}
		}
		wantGaps, wantRows := 0, 900
		if len(args) > 0 && args[len(args)-1] == "gap" {
			wantGaps, wantRows = 1, 752
		}
		if gaps != wantGaps || len(times) != wantRows {
			t.Errorf("%v: %d rows with %d Gaps, want %d with %d", args, len(times), gaps, wantRows, wantGaps)
		}
		for i, v := range cpu {
			switch args := strings.Join(args, " "); {
			case strings.Contains(args, "-cpu null") && v != nil,
				strings.Contains(args, "-cpu zero") && v != nil && v != 0.0,
				!strings.Contains(args, "-cpu") && v != nil && (v.(float64) < 16 || v.(float64) > 26):
				t.Errorf("%s: cpu_pct[%d] = %v", args, i, v)
			}
		}

		cursor := full["cursor"].(string)
		delta := pulseOf(t, h, "?after="+cursor)
		if delta["kind"] != "delta" || delta["cursor"] != cursor || len(delta["window"].(map[string]any)["boottime_ms"].([]any)) != 0 {
			t.Errorf("%v: after the newest = %v %v, want an empty delta", args, delta["kind"], delta["cursor"])
		}
		older := pulseOf(t, h, fmt.Sprintf("?after=0000000000000002.%d", len(times)-10))
		if older["kind"] != "delta" || len(older["window"].(map[string]any)["boottime_ms"].([]any)) != 10 {
			t.Errorf("%v: ten rows before the newest = %v, want a delta of ten", args, older["kind"])
		}
		if future := pulseOf(t, h, fmt.Sprintf("?after=0000000000000002.%d", len(times)+1)); future["kind"] != "full" {
			t.Errorf("%v: a future Cursor got %v", args, future["kind"])
		}
		if other := pulseOf(t, h, "?after=0000000000000001.3"); other["kind"] != "full" {
			t.Errorf("%v: a foreign epoch got %v", args, other["kind"])
		}
	}
}

func TestFrozenSamplesHoldAWaitUntilTheRequestEnds(t *testing.T) {
	h := NewAgentHandler(mustConfig(t, "-samples", "frozen"))
	start := time.Now()
	held := pulseOf(t, h, "?after=0000000000000002.900&wait=1")
	rows := len(held["window"].(map[string]any)["boottime_ms"].([]any))
	if took := time.Since(start); took < time.Second || held["kind"] != "delta" || rows != 0 || held["cursor"] != "0000000000000002.900" {
		t.Fatalf("wait=1 at the newest answered %v with %d rows after %v, want an empty delta after 1 s", held["kind"], rows, took)
	}
}

type fakeClock struct {
	mu sync.Mutex
	t  time.Time
}

func (c *fakeClock) now() time.Time { c.mu.Lock(); defer c.mu.Unlock(); return c.t }

func (c *fakeClock) advance(d time.Duration) { c.mu.Lock(); defer c.mu.Unlock(); c.t = c.t.Add(d) }

func TestLiveSamplesArriveEveryTwoSecondsAndSlideTheWindow(t *testing.T) {
	for _, setup := range []string{"full", "gap"} {
		clock := &fakeClock{t: time.Unix(1_000_000, 0)}
		h := newAgentHandler(mustConfig(t, "-window", setup), clock.now)
		first := pulseOf(t, h, "")
		baseRows := len(first["window"].(map[string]any)["boottime_ms"].([]any))
		cursor := first["cursor"].(string)

		clock.advance(1999 * time.Millisecond)
		if same := pulseOf(t, h, "?after="+cursor); same["cursor"] != cursor || len(same["window"].(map[string]any)["boottime_ms"].([]any)) != 0 {
			t.Fatalf("%s: a Sample arrived before 2 s: %v", setup, same["cursor"])
		}
		clock.advance(time.Millisecond)
		delta := pulseOf(t, h, "?after="+cursor)
		times := delta["window"].(map[string]any)["boottime_ms"].([]any)
		if want := fmt.Sprintf("0000000000000002.%d", baseRows+1); delta["kind"] != "delta" || delta["cursor"] != want || len(times) != 1 || times[0] != 3_602_000.0 {
			t.Fatalf("%s: at 2 s got %v %v %v, want one new row at 3602000 with Cursor %s", setup, delta["kind"], delta["cursor"], times, want)
		}
		lastRowIsFlat(t, delta)

		// A day of Samples: the Window keeps exactly 30 minutes, ending at the newest, and a Gap leaves with the Sample
		// after it.
		for _, step := range []time.Duration{9 * time.Minute, 2 * time.Minute, 4 * time.Minute, 24 * time.Hour} {
			clock.advance(step)
			full := pulseOf(t, h, "")
			lastRowIsFlat(t, full)
			window := full["window"].(map[string]any)
			times := window["boottime_ms"].([]any)
			newest := times[len(times)-1].(float64)
			if times[0] == nil {
				if times[1].(float64) <= newest-1_800_000 {
					t.Errorf("%s: a leading Gap stayed after its Sample left", setup)
				}
			} else if times[0].(float64) <= newest-1_800_000 {
				t.Errorf("%s: the Window starts at %v, 30 minutes or more before %v", setup, times[0], newest)
			}
			if len(times) > 900 || len(window["cpu_pct"].([]any)) != len(times) {
				t.Errorf("%s: %d rows", setup, len(times))
			}
			for i := 1; i < len(times); i++ {
				if times[i] != nil && times[i-1] != nil && times[i].(float64)-times[i-1].(float64) != 2000 {
					t.Fatalf("%s: rows %d and %d are not 2 s apart", setup, i-1, i)
				}
			}
		}
		if setup == "full" {
			if old := pulseOf(t, h, "?after="+cursor); old["kind"] != "full" {
				t.Errorf("a Cursor older than the Window got %v, want full", old["kind"])
			}
		}
	}
}

func TestLiveSamplesAnswerAWaitAtTheNextSample(t *testing.T) {
	// The QA Agent's clock jumps 1.7 s once it has started, so its next Sample lands about 300 ms from now.
	var ahead atomic.Int64
	h := newAgentHandler(mustConfig(t), func() time.Time { return time.Now().Add(time.Duration(ahead.Load())) })
	ahead.Store(int64(recorder.SampleInterval - 300*time.Millisecond))
	start := time.Now()
	held := pulseOf(t, h, "?after=0000000000000002.900&wait=10")
	took := time.Since(start)
	rows := len(held["window"].(map[string]any)["boottime_ms"].([]any))
	if took > 2*time.Second || held["kind"] != "delta" || rows != 1 || held["cursor"] != "0000000000000002.901" {
		t.Fatalf("wait=10 at the newest answered %v %v with %d rows after %v, want one new row within about 300 ms", held["kind"], held["cursor"], rows, took)
	}
}

func TestFlagsRejectAnythingThatWouldRunTheWrongScenario(t *testing.T) {
	bad := [][]string{
		{"-whois", "maybe"},
		{"-cpu", "high"},
		{"-mode", "replay"},
		{"-status", "500"},                 // status without raw would silently serve a Pulse
		{"-mode", "raw", "-whois", "deny"}, // raw bypasses authorization; a WhoIs flag would mislead
		{"-battery", "full"},
		{"-mode", "raw", "-battery", "charging"},
		{"-verdict", "severe"},
		{"-verdict", "warning"},
		{"-mode", "raw", "-verdict", "crit"},
		{"-window", "partial"},
		{"-mode", "raw", "-window", "gap"},
		{"-samples", "paused"},
		{"-mode", "raw", "-samples", "frozen"},
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

func TestAccessLogRecordsQueryNamesNeverValues(t *testing.T) {
	var buf bytes.Buffer
	h := NewAccessLog(&buf).Wrap(stillHandler(mustConfig(t)))
	r := httptest.NewRequest(http.MethodGet, "/v1/pulse?wait=0&after=0000000000000002.900", nil)
	r.RemoteAddr = "127.0.0.1:40000"
	h.ServeHTTP(httptest.NewRecorder(), r)
	if strings.Contains(buf.String(), "0000000000000002") {
		t.Fatalf("a query value reached the log: %s", buf.String())
	}
	var e AccessEntry
	if err := json.Unmarshal(buf.Bytes(), &e); err != nil {
		t.Fatal(err)
	}
	if strings.Join(e.QueryNames, ",") != "after,wait" {
		t.Fatalf("query names %v, want [after wait]", e.QueryNames)
	}
	buf.Reset()
	get(t, h, nil)
	if !strings.Contains(buf.String(), `"query_names":[]`) {
		t.Fatalf("a request without a query logged %s, want an empty list", buf.String())
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
