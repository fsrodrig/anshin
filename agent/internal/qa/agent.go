// Package qa is the test-only tooling behind the agent-qa skill: the synthetic QA Agent (anshin-qa) and the helper
// that builds, installs and routes a run on an emulator. Nothing here is linked into anshin-agent; `make
// check-agent` fails if it ever is.
package qa

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"math"
	"net"
	"net/http"
	"net/netip"
	"os"
	"slices"
	"strconv"
	"sync"
	"time"

	"github.com/fsrodrig/anshin/agent/internal/recorder"
	"github.com/fsrodrig/anshin/agent/internal/recorder/probe"
	"github.com/fsrodrig/anshin/agent/internal/server"
	"github.com/fsrodrig/anshin/agent/internal/tailnet"
)

// SyntheticHostname is the only Machine name a QA Agent reports, so no real hostname reaches a screenshot.
const SyntheticHostname = "qa-machine"

// syntheticCPU is a fixed, recognizable value: a screenshot showing 21% came from the QA Agent, not a real Machine.
const syntheticCPU = 21.0

// qaUserID is the synthetic Owner. Any value works, because the fake tailnet is the only one that hears it.
const qaUserID = 1

// AgentConfig is everything that shapes one QA Agent process. It is fixed for the process's life: a run restarts the
// QA Agent to change it, so there is no control API to secure.
type AgentConfig struct {
	Listen   netip.AddrPort
	Mode     string // "pulse" or "raw"
	WhoIs    string // pulse mode: "allow", "deny" or "localapi-error"
	CPU      string // pulse mode: "ordinary", "null" or "zero"
	Hostname string // pulse mode: "synthetic" or "null"
	Battery  string // pulse mode: "none", "discharging", "charging" or "unavailable"
	Verdict  string // pulse mode: "judged", "null", "ok", "warn", "crit" or "unknown"
	Window   string // pulse mode: "full" or "gap"
	Body     []byte // raw mode
	BodyPath string // raw mode, for the capability record only
	Status   int    // raw mode
	Delay    time.Duration
}

// Capability is one thing a QA Agent can be set up to do. Planners cite these IDs; nothing else is a capability.
type Capability struct {
	ID     string `json:"id"`
	Flags  string `json:"flags"`
	Effect string `json:"effect"`
}

// Capabilities lists every setup the QA Agent supports. It is the single list `anshin-qa -capabilities`, the
// helper's `capabilities` and the skill's planner read.
func Capabilities() []Capability {
	return []Capability{
		{"whois-allow", "-whois allow", "the production server answers 200 with a synthetic Pulse (default)"},
		{"whois-deny", "-whois deny", "the production server answers 403: the phone shows Forbidden"},
		{"whois-localapi-error", "-whois localapi-error", "the production server answers 503: the phone shows that the Agent can't reach tailscaled"},
		{"cpu-ordinary", "-cpu ordinary", "cpu_pct is 21 (default)"},
		{"cpu-null", "-cpu null", "cpu_pct is null: the phone shows CPU unavailable"},
		{"cpu-zero", "-cpu zero", "cpu_pct is 0: a real zero, which must not render as unavailable"},
		{"hostname-synthetic", "-hostname synthetic", "hostname is \"" + SyntheticHostname + "\" (default)"},
		{"hostname-null", "-hostname null", "hostname is null: the phone shows it unavailable"},
		{"battery-none", "-battery none", "battery is null: the phone shows no battery (default)"},
		{"battery-discharging", "-battery discharging", "one battery, BAT0, at 64% Discharging: 7.5 W, 9000 s left, health 87.5%"},
		{"battery-charging", "-battery charging", "one battery, BAT0, at 64% Charging: 15 W, no time left, health 87.5%"},
		{"battery-unavailable", "-battery unavailable", "battery is an object whose values are all null, as when the Agent can't classify a supply: the phone shows it unavailable, not no battery"},
		{"verdict-judged", "-verdict judged", "verdict is what the Agent's own Verdict logic says of the synthetic Machine as a first Sample: null, since it lists no filesystem, memory or CPU sensor, unless a -battery setup gives it a battery with a known percent and status, which makes it ok with no Reasons (default)"},
		{"verdict-null", "-verdict null", "verdict is null, as when no input could be judged: the phone shows the Verdict unavailable, never ok"},
		{"verdict-ok", "-verdict ok", "verdict is ok with no Reasons"},
		{"verdict-warn", "-verdict warn", "verdict is warn with one warn Reason, filesystem_full \"/ is 91.2 % full\" (the synthetic Machine lists no filesystem)"},
		{"verdict-crit", "-verdict crit", "verdict is crit with two Reasons, worst first: cpu_hot crit \"CPU at 97.0 °C (crit at 95.0 °C)\", then filesystem_full warn \"/ is 91.2 % full\""},
		{"window-full", "-window full", "a 30-minute Window of 900 Samples 2 s apart whose CPU follows a gentle wave ending at the current cpu_pct (default); memory and the rest are null as on the synthetic Machine"},
		{"window-gap", "-window gap", "the same with a 5-minute Gap ending 10 minutes before the newest Sample: the phone shows a hatched Gap · 5 min"},
		{"verdict-unknown", "-verdict unknown", "verdict level is \"severe\", one no Agent sends yet, with one Reason, fan_stalled \"A fan stopped\" at its own level \"severe\": the phone must keep an unknown fallback"},
		{"raw-file", "-mode raw -body <file> [-status <code>]", "TEST-ONLY: serves the file's exact bytes with the status (default 200) and no authorization; for schema mismatch, non-Pulse bodies and replaying fixture bytes. It proves the phone's decoding, never the production encoder or the truth table"},
		{"http-status", "-mode raw -status <code>", "TEST-ONLY: answers every request with that status and an empty body, such as 500"},
		{"delay", "-delay <duration>", "holds every response; under 5s the phone shows Connecting first, over 5s its read timeout turns it Unreachable"},
		{"stop-start", "helper agent stop | agent start", "stopping the QA Agent leaves the route in place, so the phone shows Unreachable; starting it again restores the answer"},
	}
}

// ParseAgentFlags reads anshin-qa's command line. It is strict: an unknown value is an error, never a default,
// because a typo would otherwise run the wrong scenario and still look like evidence.
func ParseAgentFlags(args []string, stderr io.Writer) (AgentConfig, bool, error) {
	fs := flag.NewFlagSet("anshin-qa", flag.ContinueOnError)
	fs.SetOutput(stderr)
	listen := fs.String("listen", "", "address:port to listen on (required)")
	mode := fs.String("mode", "pulse", "pulse | raw")
	whois := fs.String("whois", "allow", "pulse mode: allow | deny | localapi-error")
	cpu := fs.String("cpu", "ordinary", "pulse mode: ordinary | null | zero")
	hostname := fs.String("hostname", "synthetic", "pulse mode: synthetic | null")
	battery := fs.String("battery", "none", "pulse mode: none | discharging | charging | unavailable")
	verdict := fs.String("verdict", "judged", "pulse mode: judged | null | ok | warn | crit | unknown")
	window := fs.String("window", "full", "pulse mode: full | gap")
	body := fs.String("body", "", "raw mode: file whose exact bytes every response carries")
	status := fs.Int("status", http.StatusOK, "raw mode: HTTP status of every response")
	delay := fs.Duration("delay", 0, "hold every response this long")
	caps := fs.Bool("capabilities", false, "print the capability list as JSON and exit")
	if err := fs.Parse(args); err != nil {
		return AgentConfig{}, false, err
	}
	if *caps {
		return AgentConfig{}, true, nil
	}
	if fs.NArg() > 0 {
		return AgentConfig{}, false, fmt.Errorf("unexpected arguments: %v", fs.Args())
	}
	c := AgentConfig{Mode: *mode, WhoIs: *whois, CPU: *cpu, Hostname: *hostname, Battery: *battery, Verdict: *verdict, Window: *window, Status: *status, Delay: *delay}
	ap, err := netip.ParseAddrPort(*listen)
	if err != nil {
		return c, false, fmt.Errorf("-listen %q: %w", *listen, err)
	}
	c.Listen = ap
	set := map[string]bool{}
	fs.Visit(func(f *flag.Flag) { set[f.Name] = true })
	switch c.Mode {
	case "pulse":
		for _, name := range []string{"body", "status"} {
			if set[name] {
				return c, false, fmt.Errorf("-%s needs -mode raw", name)
			}
		}
		if err := oneOf("whois", c.WhoIs, "allow", "deny", "localapi-error"); err != nil {
			return c, false, err
		}
		if err := oneOf("cpu", c.CPU, "ordinary", "null", "zero"); err != nil {
			return c, false, err
		}
		if err := oneOf("hostname", c.Hostname, "synthetic", "null"); err != nil {
			return c, false, err
		}
		if err := oneOf("battery", c.Battery, "none", "discharging", "charging", "unavailable"); err != nil {
			return c, false, err
		}
		if err := oneOf("verdict", c.Verdict, "judged", "null", "ok", "warn", "crit", "unknown"); err != nil {
			return c, false, err
		}
		if err := oneOf("window", c.Window, "full", "gap"); err != nil {
			return c, false, err
		}
	case "raw":
		for _, name := range []string{"whois", "cpu", "hostname", "battery", "verdict", "window"} {
			if set[name] {
				return c, false, fmt.Errorf("-%s has no effect with -mode raw, which bypasses the production server", name)
			}
		}
		if c.Status < 100 || c.Status > 599 {
			return c, false, fmt.Errorf("-status %d is not an HTTP status", c.Status)
		}
		if *body != "" {
			b, err := os.ReadFile(*body)
			if err != nil {
				return c, false, err
			}
			c.Body, c.BodyPath = b, *body
		}
	default:
		return c, false, fmt.Errorf("-mode %q: want pulse or raw", c.Mode)
	}
	if c.Delay < 0 {
		return c, false, errors.New("-delay must not be negative")
	}
	return c, false, nil
}

func oneOf(name, v string, allowed ...string) error {
	if slices.Contains(allowed, v) {
		return nil
	}
	return fmt.Errorf("-%s %q: want one of %v", name, v, allowed)
}

// syntheticEpoch tells a synthetic Window's Cursor apart from the golden fixtures' epoch 1.
const syntheticEpoch = 2

// syntheticNewestMs is the newest Sample's CLOCK_BOOTTIME: an hour after boot, so a full Window fits after it.
const syntheticNewestMs = 3_600_000

// syntheticRecorder never takes a new Sample: every Pulse is the same Reading and Window.
type syntheticRecorder struct {
	reading recorder.Reading
	window  *recorder.Window
}

func (r syntheticRecorder) rows() uint64 { return uint64(len(r.window.BoottimeMs)) }

// Read answers a Cursor as the production recorder does, with seqs 1 to rows().
func (r syntheticRecorder) Read(c recorder.Cursor) recorder.Batch {
	b := recorder.Batch{Kind: recorder.Full, Cursor: recorder.Cursor{Epoch: syntheticEpoch, Seq: r.rows()}, Reading: r.reading, Window: r.window}
	if c.Epoch == syntheticEpoch && c.Seq >= 1 && c.Seq <= r.rows() {
		b.Kind, b.Window = recorder.Delta, r.window.Since(int(c.Seq))
	}
	return b
}

func (r syntheticRecorder) Wait(ctx context.Context, c recorder.Cursor) (recorder.Batch, error) {
	if c.Epoch != syntheticEpoch || c.Seq != r.rows() {
		return r.Read(c), nil
	}
	<-ctx.Done()
	return recorder.Batch{}, ctx.Err()
}

// syntheticWindow is 30 minutes of Samples 2 s apart ending at syntheticNewestMs. Every series but CPU holds the
// flat field's value throughout, so the last row is the flat fields, as the production recorder guarantees. CPU is a
// wave of ±5 points with a 10-minute period that ends on the current cpu_pct.
func syntheticWindow(setup string, r recorder.Reading) *recorder.Window {
	const step, span = 2_000, 1_800_000
	gapFrom, gapTo := int64(-1), int64(-1)
	if setup == "gap" {
		gapTo = syntheticNewestMs - 600_000
		gapFrom = gapTo - 300_000
	}
	w := &recorder.Window{SpanS: span / 1000}
	var mem, load, temp, batteryPct, batteryPower *float64
	mem, load, temp = r.MemoryReading.Pct, r.LoadReading.One, r.CPUTempC
	if r.Battery != nil {
		batteryPct, batteryPower = r.Battery.Pct, r.Battery.PowerW
	}
	add := func(t *int64, cpu *float64) {
		w.BoottimeMs = append(w.BoottimeMs, t)
		w.CPUPct = append(w.CPUPct, cpu)
		gap := t == nil
		level := func(v *float64) *float64 {
			if gap {
				return nil
			}
			return v
		}
		count := func(v *uint64) *uint64 {
			if gap {
				return nil
			}
			return v
		}
		w.Load1, w.MemUsedPct, w.CPUTempC = append(w.Load1, level(load)), append(w.MemUsedPct, level(mem)), append(w.CPUTempC, level(temp))
		w.BatteryPct, w.BatteryPowerW = append(w.BatteryPct, level(batteryPct)), append(w.BatteryPowerW, level(batteryPower))
		w.SwapUsedBytes = append(w.SwapUsedBytes, count(r.SwapReading.Used))
		w.NetRx, w.NetTx = append(w.NetRx, count(r.NetRx)), append(w.NetTx, count(r.NetTx))
		w.DiskRead, w.DiskWrite = append(w.DiskRead, count(r.DiskRead)), append(w.DiskWrite, count(r.DiskWrite))
	}
	for t := int64(syntheticNewestMs - span + step); t <= syntheticNewestMs; t += step {
		if t > gapFrom && t < gapTo {
			continue
		}
		if t == gapTo {
			add(nil, nil)
		}
		// The first Sample after a Gap has no CPU baseline, as in the production recorder.
		var cpu *float64
		if r.CPUPct != nil && t != gapTo {
			v := *r.CPUPct
			if v != 0 {
				v = max(0, min(100, math.Round(10*(v+5*math.Sin(2*math.Pi*float64(t-syntheticNewestMs)/600_000)))/10))
			}
			cpu = &v
		}
		add(&t, cpu)
	}
	return w
}

func syntheticReading(c AgentConfig) recorder.Reading {
	var r recorder.Reading
	if c.Hostname == "synthetic" {
		h := SyntheticHostname
		r.Hostname = &h
	}
	switch c.CPU {
	case "ordinary":
		v := syntheticCPU
		r.CPUPct = &v
	case "zero":
		v := 0.0
		r.CPUPct = &v
	}
	r.Battery = syntheticBattery(c.Battery)
	r.Verdict = syntheticVerdict(c.Verdict, r)
	return r
}

// syntheticVerdict defaults to the recorder's own judgement, so a setup that doesn't ask for a Verdict never shows
// one the synthetic Machine's measurements wouldn't produce.
func syntheticVerdict(setup string, r recorder.Reading) *recorder.Verdict {
	fullDisk := recorder.Reason{Code: "filesystem_full", Level: recorder.LevelWarn, Message: "/ is 91.2 % full"}
	switch setup {
	case "judged":
		return recorder.VerdictOf(r)
	case "ok":
		return &recorder.Verdict{Level: recorder.LevelOK, Reasons: []recorder.Reason{}}
	case "warn":
		return &recorder.Verdict{Level: recorder.LevelWarn, Reasons: []recorder.Reason{fullDisk}}
	case "crit":
		hot := recorder.Reason{Code: "cpu_hot", Level: recorder.LevelCrit, Message: "CPU at 97.0 °C (crit at 95.0 °C)"}
		return &recorder.Verdict{Level: recorder.LevelCrit, Reasons: []recorder.Reason{hot, fullDisk}}
	case "unknown":
		return &recorder.Verdict{Level: "severe", Reasons: []recorder.Reason{{Code: "fan_stalled", Level: "severe", Message: "A fan stopped"}}}
	}
	return nil
}

// syntheticBattery has fixed, recognizable values: a screenshot showing 64% and 87.5% came from the QA Agent.
func syntheticBattery(setup string) *probe.BatteryReading {
	row := probe.BatteryRow{Name: "BAT0", Pct: new(64.0), Health: new(87.5)}
	switch setup {
	case "discharging":
		row.Status, row.PowerW = new("Discharging"), new(7.5)
		return &probe.BatteryReading{Pct: row.Pct, Status: row.Status, PowerW: row.PowerW, TimeLeft: new(uint64(9000)), Batteries: []probe.BatteryRow{row}}
	case "charging":
		row.Status, row.PowerW = new("Charging"), new(15.0)
		return &probe.BatteryReading{Pct: row.Pct, Status: row.Status, PowerW: row.PowerW, Batteries: []probe.BatteryRow{row}}
	case "unavailable":
		return &probe.BatteryReading{}
	}
	return nil
}

var errLocalAPI = errors.New("synthetic LocalAPI failure")

// NewAgentHandler builds the QA Agent's HTTP handler. Pulse mode is the production server.New behind the fake
// tailnet, so encoding and the truth table are the shipped code; only who answers WhoIs and what the recorder says
// are synthetic. Raw mode replaces the server entirely and is test-only.
func NewAgentHandler(c AgentConfig) http.Handler {
	var h http.Handler
	if c.Mode == "raw" {
		h = rawResponder{body: c.Body, status: c.Status}
	} else {
		self := tailnet.Self{Node: tailnet.Node{UserID: qaUserID}, Addrs: []netip.Addr{c.Listen.Addr()}}
		tn := tailnet.NewFake(self)
		switch c.WhoIs {
		case "allow":
			// The phone reaches a host QA Agent through adb reverse, so its connection arrives from loopback; on the
			// in-emulator route it arrives from the address the QA Agent listens on.
			for _, a := range []netip.Addr{netip.MustParseAddr("127.0.0.1"), netip.IPv6Loopback(), c.Listen.Addr()} {
				tn.SetPeer(a, tailnet.Node{UserID: qaUserID})
			}
		case "localapi-error":
			tn.FailWhoIs(errLocalAPI)
		}
		r := syntheticReading(c)
		h = server.New(tn, self, syntheticRecorder{r, syntheticWindow(c.Window, r)})
	}
	if c.Delay > 0 {
		h = delayed{next: h, delay: c.Delay}
	}
	return h
}

type rawResponder struct {
	body   []byte
	status int
}

func (r rawResponder) ServeHTTP(w http.ResponseWriter, _ *http.Request) {
	if len(r.body) > 0 {
		w.Header().Set("Content-Type", "application/json")
	}
	w.Header().Set("Content-Length", strconv.Itoa(len(r.body)))
	w.WriteHeader(r.status)
	w.Write(r.body)
}

type delayed struct {
	next  http.Handler
	delay time.Duration
}

func (d delayed) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	t := time.NewTimer(d.delay)
	defer t.Stop()
	select {
	case <-r.Context().Done():
		return
	case <-t.C:
	}
	d.next.ServeHTTP(w, r)
}

// AccessEntry is one request line of the access log. It holds header names only: values could carry anything the
// phone sent, and the evidence must show that no credential header exists, not what one said.
type AccessEntry struct {
	Time        time.Time `json:"time"`
	Method      string    `json:"method"`
	Path        string    `json:"path"`
	Status      int       `json:"status"`
	HeaderNames []string  `json:"header_names"`
	DurationMS  int64     `json:"duration_ms"`
	Canceled    bool      `json:"canceled"`
}

// AccessLog writes one JSON line per request. It covers only requests that reached this QA Agent, so it can never
// show that the phone made no connection somewhere else.
type AccessLog struct {
	mu  sync.Mutex
	w   io.Writer
	now func() time.Time
}

func NewAccessLog(w io.Writer) *AccessLog { return &AccessLog{w: w, now: time.Now} }

type statusRecorder struct {
	http.ResponseWriter
	status int
}

func (s *statusRecorder) WriteHeader(code int) {
	if s.status == 0 {
		s.status = code
	}
	s.ResponseWriter.WriteHeader(code)
}

func (s *statusRecorder) Write(b []byte) (int, error) {
	if s.status == 0 {
		s.status = http.StatusOK
	}
	return s.ResponseWriter.Write(b)
}

func (l *AccessLog) Wrap(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := l.now()
		rec := &statusRecorder{ResponseWriter: w}
		next.ServeHTTP(rec, r)
		names := make([]string, 0, len(r.Header))
		for k := range r.Header {
			names = append(names, k)
		}
		slices.Sort(names)
		e := AccessEntry{
			Time: start.UTC(), Method: r.Method, Path: r.URL.Path, Status: rec.status, HeaderNames: names,
			DurationMS: l.now().Sub(start).Milliseconds(), Canceled: r.Context().Err() != nil && rec.status == 0,
		}
		b, _ := json.Marshal(e)
		l.mu.Lock()
		defer l.mu.Unlock()
		l.w.Write(append(b, '\n'))
	})
}

// ServeAgent listens on c.Listen and serves until ctx ends. ready, when not nil, receives the bound address.
func ServeAgent(ctx context.Context, c AgentConfig, log io.Writer, ready func(netip.AddrPort)) error {
	var lc net.ListenConfig
	ln, err := lc.Listen(ctx, "tcp", c.Listen.String())
	if err != nil {
		return err
	}
	if ready != nil {
		ready(netip.MustParseAddrPort(ln.Addr().String()))
	}
	srv := &http.Server{Handler: NewAccessLog(log).Wrap(NewAgentHandler(c)), ReadHeaderTimeout: 5 * time.Second}
	errc := make(chan error, 1)
	go func() { errc <- srv.Serve(ln) }()
	select {
	case <-ctx.Done():
	case err := <-errc:
		return err
	}
	shutCtx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	srv.Shutdown(shutCtx)
	return nil
}
