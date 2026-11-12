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
	"slices"
	"sync"
	"syscall"
	"time"

	"github.com/fsrodrig/anshin/agent/internal/recorder"
	"github.com/fsrodrig/anshin/agent/internal/tailnet"
)

// DefaultPort is unassigned in the IANA registry (7245–7261) and below Linux's ephemeral range.
const DefaultPort = 7247

const whoisTTL = 30 * time.Second

// selfRecheck is how often Run asks tailscaled whether this Machine's identity changed.
var selfRecheck = 30 * time.Second

// Recorder is what the server needs from the recorder.
type Recorder interface {
	Read() recorder.Reading
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
	srv := &http.Server{
		Handler:           New(tn, self, rec),
		ReadHeaderTimeout: 5 * time.Second,
		IdleTimeout:       2 * time.Minute,
	}
	errc := make(chan error, len(lns)+1)
	for _, ln := range lns {
		slog.Info("listening", "addr", ln.Addr().String())
		go func() { errc <- srv.Serve(ln) }()
	}
	go func() { errc <- watchSelf(ctx, tn, self) }()

	select {
	case <-ctx.Done():
		err = nil
	case err = <-errc:
	}
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
		if !slices.Equal(now.Addrs, was.Addrs) || now.UserID != was.UserID || !slices.Equal(now.Tags, was.Tags) {
			return errors.New("this Machine's tailnet addresses or identity changed; exiting to rebind")
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

func New(tn tailnet.Client, self tailnet.Self, rec Recorder) *Server {
	return &Server{tn: tn, self: self, rec: rec, now: time.Now, whois: map[netip.Addr]whoisEntry{}}
}

func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if status := s.authorize(r); status != http.StatusOK {
		http.Error(w, http.StatusText(status), status)
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
	s.pulse(w)
}

type pulse struct {
	Schema   int      `json:"schema"`
	Kind     string   `json:"kind"`
	Hostname *string  `json:"hostname"`
	CPUPct   *float64 `json:"cpu_pct"`
}

func (s *Server) pulse(w http.ResponseWriter) {
	reading := s.rec.Read()
	body, err := json.Marshal(pulse{Schema: 1, Kind: "full", Hostname: reading.Hostname, CPUPct: reading.CPUPct})
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
	// Node.User is whoever created a node, which for a tagged node says nothing about who owns it, so any tag on
	// either side denies.
	if s.self.Tagged() || caller.Tagged() || caller.UserID != s.self.UserID {
		return http.StatusForbidden
	}
	return http.StatusOK
}

func (s *Server) whoIs(ctx context.Context, addr netip.Addr) (tailnet.Node, error) {
	now := s.now()
	s.mu.Lock()
	e, ok := s.whois[addr]
	s.mu.Unlock()
	if ok && now.Sub(e.at) < whoisTTL {
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
		if now.Sub(old.at) >= whoisTTL {
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
