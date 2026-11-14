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
	"net"
	"net/http"
	"net/netip"
	"os"
	"slices"
	"strconv"
	"sync"
	"time"

	"github.com/fsrodrig/anshin/agent/internal/recorder"
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
	c := AgentConfig{Mode: *mode, WhoIs: *whois, CPU: *cpu, Hostname: *hostname, Status: *status, Delay: *delay}
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
	case "raw":
		for _, name := range []string{"whois", "cpu", "hostname"} {
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

type syntheticRecorder struct{ reading recorder.Reading }

func (r syntheticRecorder) Read() recorder.Reading { return r.reading }

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
	return r
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
		h = server.New(tn, self, syntheticRecorder{syntheticReading(c)})
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
