// Package server binds the Agent to its tailnet addresses, authorizes every caller against the WhoIs truth table
// and serves the Pulse. It is the only place authorization is decided.
package server

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"slices"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/fsrodrig/anshin/agent/internal/recorder"
	"github.com/fsrodrig/anshin/agent/internal/tailnet"
)

// DefaultPort is unassigned in the IANA registry (7245–7261) and below Linux's ephemeral range.
const DefaultPort = 7247

const whoisTTL = 30 * time.Second

// A restarting tailscaled answers "no match" for every address until its netmap loads, so a not-a-peer answer is
// kept only briefly; otherwise the Owner's phone would be refused for the full TTL after every tailscaled restart.
const notPeerTTL = 5 * time.Second

// selfRecheck is how often Run asks tailscaled whether this Machine's identity changed.
var selfRecheck = 30 * time.Second

// maxWait is far below the server's IdleTimeout and common NAT idle timeouts. A longer wait is held for maxWait
// rather than refused, so a newer phone can ask an older Agent for more.
const maxWait = 30 * time.Second

// maxHeaderBytes is far above the phone's few short headers. Headers arrive before WhoIs can refuse anyone, so
// Go's 1 MiB default would let any caller that reaches the port hold that much per connection.
const maxHeaderBytes = 8 << 10

// Recorder is what the server needs from the recorder. Neither call measures the Machine.
type Recorder interface {
	Read(recorder.Cursor) recorder.Batch
	Wait(context.Context, recorder.Cursor) (recorder.Batch, error)
}

// Run binds only this Machine's tailnet addresses on port and serves until ctx ends. It returns an error, so the
// Agent exits non-zero and systemd restarts it, when tailscaled can't say who this Machine is at start or later
// reports a different identity: rebinding in-process would be a second, rarely exercised startup path.
func Run(ctx context.Context, tn tailnet.Client, rec Recorder, port int) error {
	ctx, stop := context.WithCancel(ctx)
	defer stop()

	startCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	self, err := tn.Self(startCtx)
	cancel()
	if err != nil {
		if ctx.Err() != nil {
			return nil
		}
		return fmt.Errorf("asking tailscaled who this Machine is: %w", err)
	}

	lns, err := Listen(ctx, self.Addrs, port)
	if err != nil {
		return err
	}
	return serveUntil(ctx, lns, New(tn, self, rec), func(ctx context.Context) error { return watchSelf(ctx, tn, self) })
}

// serveUntil serves h on lns until ctx ends or watch or a listener fails. Requests' contexts end before Shutdown
// starts, so a held long-poll answers at once instead of making Shutdown wait it out.
func serveUntil(ctx context.Context, lns []net.Listener, h http.Handler, watch func(context.Context) error) error {
	serveCtx, endRequests := context.WithCancel(context.Background())
	defer endRequests()
	srv := &http.Server{
		Handler:           h,
		ReadHeaderTimeout: 5 * time.Second,
		MaxHeaderBytes:    maxHeaderBytes,
		IdleTimeout:       2 * time.Minute,
		BaseContext:       func(net.Listener) context.Context { return serveCtx },
	}
	errc := make(chan error, len(lns)+1)
	for _, ln := range lns {
		slog.Info("listening", "addr", ln.Addr().String())
		go func() { errc <- srv.Serve(ln) }()
	}
	go func() { errc <- watch(ctx) }()

	var err error
	select {
	case <-ctx.Done():
	case err = <-errc:
	}
	endRequests()
	shutCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if shutErr := srv.Shutdown(shutCtx); shutErr != nil && err == nil {
		err = shutErr
	}
	return err
}

func watchSelf(ctx context.Context, tn tailnet.Client, was tailnet.Self) error {
	t := time.NewTicker(selfRecheck)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return nil
		case <-t.C:
		}
		now, err := tn.Self(ctx)
		if err != nil {
			// Requests already answer 503 while tailscaled is unreachable; the bind is still right.
			slog.Warn("rechecking this Machine's tailnet identity", "err", err)
			continue
		}
		if !slices.Equal(now.Addrs, was.Addrs) || now.DNSName != was.DNSName || now.UserID != was.UserID ||
			!slices.Equal(now.Tags, was.Tags) {
			return errors.New("this Machine's tailnet addresses, name or identity changed; exiting to rebind")
		}
	}
}

// Server answers the Pulse API for one Machine.
type Server struct {
	tn   tailnet.Client
	self tailnet.Self
	rec  Recorder
	now  func() time.Time

	mu    sync.Mutex
	whois map[netip.Addr]whoisEntry
}

type whoisEntry struct {
	node    tailnet.Node
	notPeer bool
	at      time.Time
}

func (e whoisEntry) fresh(now time.Time) bool {
	ttl := whoisTTL
	if e.notPeer {
		ttl = notPeerTTL
	}
	return now.Sub(e.at) < ttl
}

func New(tn tailnet.Client, self tailnet.Self, rec Recorder) *Server {
	return &Server{tn: tn, self: self, rec: rec, now: time.Now, whois: map[netip.Addr]whoisEntry{}}
}

func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if status := s.authorize(r); status != http.StatusOK {
		http.Error(w, http.StatusText(status), status)
		return
	}
	if !s.isSelf(r.Host) {
		http.Error(w, http.StatusText(http.StatusMisdirectedRequest), http.StatusMisdirectedRequest)
		return
	}
	if r.URL.Path != "/v1/pulse" {
		http.NotFound(w, r)
		return
	}
	if r.Method != http.MethodGet {
		w.Header().Set("Allow", http.MethodGet)
		http.Error(w, http.StatusText(http.StatusMethodNotAllowed), http.StatusMethodNotAllowed)
		return
	}
	after, wait, err := pulseQuery(r.URL.RawQuery)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	s.pulse(r.Context(), w, after, wait)
}

// pulseQuery reads docs/design.md § Responses' query. A missing after is the zero Cursor, which gets the whole
// Window; unknown parameters are ignored so a newer phone can add some.
func pulseQuery(raw string) (recorder.Cursor, time.Duration, error) {
	q, err := url.ParseQuery(raw)
	if err != nil {
		return recorder.Cursor{}, 0, errors.New("malformed query")
	}
	one := func(name string) (string, bool, error) {
		v, ok := q[name]
		switch {
		case !ok:
			return "", false, nil
		case len(v) != 1:
			return "", false, fmt.Errorf("%s is repeated", name)
		case v[0] == "":
			return "", false, fmt.Errorf("%s is empty", name)
		}
		return v[0], true, nil
	}
	var after recorder.Cursor
	if v, ok, err := one("after"); err != nil {
		return recorder.Cursor{}, 0, err
	} else if ok {
		if after, err = recorder.ParseCursor(v); err != nil {
			return recorder.Cursor{}, 0, fmt.Errorf("after: %w", err)
		}
	}
	var wait time.Duration
	if v, ok, err := one("wait"); err != nil {
		return recorder.Cursor{}, 0, err
	} else if ok {
		if strings.TrimLeft(v, "0123456789") != "" {
			return recorder.Cursor{}, 0, errors.New("wait is whole seconds")
		}
		n, err := strconv.ParseUint(v, 10, 32)
		if err != nil {
			return recorder.Cursor{}, 0, errors.New("wait is whole seconds")
		}
		wait = min(time.Duration(n)*time.Second, maxWait)
	}
	return after, wait, nil
}

type pulse struct {
	recorder.Reading
	Schema int              `json:"schema"`
	Kind   recorder.Kind    `json:"kind"`
	Cursor string           `json:"cursor"`
	Window *recorder.Window `json:"window"`
}

func (s *Server) pulse(ctx context.Context, w http.ResponseWriter, after recorder.Cursor, wait time.Duration) {
	var b recorder.Batch
	if wait > 0 {
		held, cancel := context.WithTimeout(ctx, wait)
		var err error
		b, err = s.rec.Wait(held, after)
		cancel()
		if err != nil {
			b = s.rec.Read(after)
		}
	} else {
		b = s.rec.Read(after)
	}
	body, err := json.Marshal(pulse{Schema: 1, Kind: b.Kind, Cursor: b.Cursor.String(), Window: b.Window, Reading: b.Reading})
	if err != nil {
		http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.Write(body)
}

// authorize applies the truth table in docs/design.md § Security model. Identity comes only from WhoIs on the
// connection's source address, never from a header or anything else the caller sends.
func (s *Server) authorize(r *http.Request) int {
	ap, err := netip.ParseAddrPort(r.RemoteAddr)
	if err != nil {
		return http.StatusForbidden
	}
	caller, err := s.whoIs(r.Context(), ap.Addr().Unmap())
	switch {
	case errors.Is(err, tailnet.ErrNotPeer):
		return http.StatusForbidden
	case err != nil:
		slog.Warn("tailscaled could not identify a caller", "err", err)
		return http.StatusServiceUnavailable
	}
	if !Allows(s.self, caller) {
		return http.StatusForbidden
	}
	return http.StatusOK
}

// Allows is the identity half of the truth table: whether a tailnet peer may watch this Machine. Node.User is
// whoever created a node, which for a tagged node says nothing about who owns it, so any tag on either side denies.
// No tailnet user has ID 0, so a 0 means an answer tailscaled didn't shape as expected and never matches.
func Allows(self tailnet.Self, caller tailnet.Node) bool {
	return !self.Tagged() && !caller.Tagged() && self.UserID != 0 && caller.UserID == self.UserID
}

// isSelf reports whether a request's Host names this Machine: one of its tailnet addresses, its MagicDNS name or
// that name's first label. WhoIs can't tell the phone from a web page on the Owner's laptop whose domain an
// attacker has rebound to this Machine's address; only the Host that browser sends, the attacker's domain, can.
func (s *Server) isSelf(host string) bool {
	if h, _, err := net.SplitHostPort(host); err == nil {
		host = h
	} else {
		host = strings.TrimSuffix(strings.TrimPrefix(host, "["), "]")
	}
	if a, err := netip.ParseAddr(host); err == nil {
		return slices.Contains(s.self.Addrs, a.Unmap())
	}
	name := strings.ToLower(strings.TrimSuffix(host, "."))
	if name == "" || s.self.DNSName == "" {
		return false
	}
	short, _, _ := strings.Cut(s.self.DNSName, ".")
	return name == s.self.DNSName || name == short
}

func (s *Server) whoIs(ctx context.Context, addr netip.Addr) (tailnet.Node, error) {
	now := s.now()
	s.mu.Lock()
	e, ok := s.whois[addr]
	s.mu.Unlock()
	if ok && e.fresh(now) {
		if e.notPeer {
			return tailnet.Node{}, tailnet.ErrNotPeer
		}
		return e.node, nil
	}

	node, err := s.tn.WhoIs(ctx, addr)
	if err != nil && !errors.Is(err, tailnet.ErrNotPeer) {
		// Not cached: tailscaled being down says nothing about the caller, and the next request should retry.
		return tailnet.Node{}, err
	}
	s.mu.Lock()
	for a, old := range s.whois {
		if !old.fresh(now) {
			delete(s.whois, a)
		}
	}
	s.whois[addr] = whoisEntry{node: node, notPeer: err != nil, at: now}
	s.mu.Unlock()
	return node, err
}

// Listen binds each address on port. IP_FREEBIND lets the bind succeed before tailscaled has put the addresses on
// its interface, so the order the two services start in doesn't matter.
func Listen(ctx context.Context, addrs []netip.Addr, port int) ([]net.Listener, error) {
	lc := net.ListenConfig{Control: freebind}
	return listen(ctx, addrs, port, lc.Listen)
}

var errNothingBound = errors.New("no tailnet addresses to bind")

type bindFunc func(ctx context.Context, network, address string) (net.Listener, error)

func listen(ctx context.Context, addrs []netip.Addr, port int, bind bindFunc) ([]net.Listener, error) {
	var lns []net.Listener
	for _, a := range addrs {
		network := "tcp4"
		if a.Is6() {
			network = "tcp6"
		}
		ln, err := bind(ctx, network, netip.AddrPortFrom(a, uint16(port)).String())
		if a.Is6() && errors.Is(err, syscall.EAFNOSUPPORT) {
			// tailscaled lists the tailnet IPv6 address even on a kernel booted with IPv6 disabled, where no IPv6
			// socket can exist. Failing here would leave the Machine restarting forever.
			slog.Warn("IPv6 is disabled on this Machine; serving on its tailnet IPv4 address only", "skipped", a.String())
			continue
		}
		if err != nil {
			for _, l := range lns {
				l.Close()
			}
			return nil, fmt.Errorf("binding %s: %w", a, err)
		}
		lns = append(lns, ln)
	}
	if len(lns) == 0 {
		return nil, errNothingBound
	}
	return lns, nil
}
