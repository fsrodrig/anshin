package server

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"os"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/fsrodrig/anshin/agent/internal/recorder"
	"github.com/fsrodrig/anshin/agent/internal/tailnet"
)

const owner, stranger int64 = 4242, 9999

var (
	machineAddrs = []netip.Addr{netip.MustParseAddr("100.64.0.1"), netip.MustParseAddr("fd7a:115c:a1e0::1")}
	untaggedSelf = tailnet.Self{Node: tailnet.Node{UserID: owner}, Addrs: machineAddrs}
	taggedSelf   = tailnet.Self{Node: tailnet.Node{UserID: owner, Tags: []string{"tag:server"}}, Addrs: machineAddrs}
	// httptest serves on loopback, so loopback is where the fake tailnet puts the caller.
	callerAddr = netip.MustParseAddr("127.0.0.1")
)

type fixedRecorder struct{ r recorder.Reading }

func (f fixedRecorder) Read() recorder.Reading { return f.r }

func ptr[T any](v T) *T { return &v }

var reading = recorder.Reading{Hostname: ptr("machine-a"), CPUPct: ptr(12.5)}

// agent serves the handler over loopback HTTP with the given Machine identity and caller.
func agent(t *testing.T, self tailnet.Self, caller *tailnet.Node) (*tailnet.Fake, *Server, string) {
	t.Helper()
	return serve(t, self, caller, fixedRecorder{reading})
}

func serve(t *testing.T, self tailnet.Self, caller *tailnet.Node, rec Recorder) (*tailnet.Fake, *Server, string) {
	t.Helper()
	tn := tailnet.NewFake(self)
	if caller != nil {
		tn.SetPeer(callerAddr, *caller)
	}
	s := New(tn, self, rec)
	ts := httptest.NewServer(s)
	t.Cleanup(ts.Close)
	return tn, s, ts.URL
}

func get(t *testing.T, method, url string) (int, string) {
	t.Helper()
	req, err := http.NewRequest(method, url, nil)
	if err != nil {
		t.Fatal(err)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, string(body)
}

// TestTruthTable covers every row of docs/design.md § Security model.
func TestTruthTable(t *testing.T) {
	untagged := func(user int64) *tailnet.Node { return &tailnet.Node{UserID: user} }
	tagged := func(user int64) *tailnet.Node { return &tailnet.Node{UserID: user, Tags: []string{"tag:ci"}} }

	for _, row := range []struct {
		name     string
		self     tailnet.Self
		caller   *tailnet.Node
		whoisErr error
		want     int
	}{
		{"same user, caller untagged, Machine untagged: allow", untaggedSelf, untagged(owner), nil, http.StatusOK},
		{"same user, caller tagged, Machine untagged", untaggedSelf, tagged(owner), nil, http.StatusForbidden},
		{"same user, caller tagged, Machine tagged", taggedSelf, tagged(owner), nil, http.StatusForbidden},
		{"same user, caller untagged, Machine tagged", taggedSelf, untagged(owner), nil, http.StatusForbidden},
		{"other user, untagged, Machine untagged", untaggedSelf, untagged(stranger), nil, http.StatusForbidden},
		{"other user, tagged, Machine untagged", untaggedSelf, tagged(stranger), nil, http.StatusForbidden},
		{"other user, untagged, Machine tagged", taggedSelf, untagged(stranger), nil, http.StatusForbidden},
		{"other user, tagged, Machine tagged", taggedSelf, tagged(stranger), nil, http.StatusForbidden},
		{"not a tailnet peer", untaggedSelf, nil, nil, http.StatusForbidden},
		{"LocalAPI unreachable or erroring", untaggedSelf, untagged(owner), errors.New("connection refused"), http.StatusServiceUnavailable},
	} {
		t.Run(row.name, func(t *testing.T) {
			tn, _, url := agent(t, row.self, row.caller)
			tn.FailWhoIs(row.whoisErr)
			status, body := get(t, http.MethodGet, url+"/v1/pulse")
			if status != row.want {
				t.Fatalf("status = %d, want %d (body %q)", status, row.want, body)
			}
			if row.want != http.StatusOK && strings.Contains(body, "machine-a") {
				t.Fatalf("a refused caller got the Pulse: %q", body)
			}
		})
	}
}

func TestRefusedCallersLearnNothingAboutRoutes(t *testing.T) {
	_, _, url := agent(t, untaggedSelf, nil)
	for _, path := range []string{"/v1/pulse", "/nope"} {
		for _, method := range []string{http.MethodGet, http.MethodPost} {
			if status, _ := get(t, method, url+path); status != http.StatusForbidden {
				t.Fatalf("%s %s from a stranger = %d, want 403", method, path, status)
			}
		}
	}
}

func TestPulse(t *testing.T) {
	_, _, url := agent(t, untaggedSelf, &tailnet.Node{UserID: owner})
	status, body := get(t, http.MethodGet, url+"/v1/pulse")
	if status != http.StatusOK {
		t.Fatalf("status = %d", status)
	}
	var got map[string]any
	if err := json.Unmarshal([]byte(body), &got); err != nil {
		t.Fatal(err)
	}
	want := map[string]any{"schema": 1.0, "kind": "full", "hostname": "machine-a", "cpu_pct": 12.5}
	if len(got) != len(want) {
		t.Fatalf("Pulse = %s, want exactly %v", body, want)
	}
	for k, v := range want {
		if got[k] != v {
			t.Fatalf("Pulse[%q] = %v, want %v (body %s)", k, got[k], v, body)
		}
	}
}

func TestPulseUnavailableIsNull(t *testing.T) {
	tn := tailnet.NewFake(untaggedSelf)
	tn.SetPeer(callerAddr, tailnet.Node{UserID: owner})
	ts := httptest.NewServer(New(tn, untaggedSelf, fixedRecorder{}))
	defer ts.Close()
	_, body := get(t, http.MethodGet, ts.URL+"/v1/pulse")
	if !strings.Contains(body, `"hostname":null`) || !strings.Contains(body, `"cpu_pct":null`) {
		t.Fatalf("Pulse = %s, want null hostname and cpu_pct", body)
	}
}

func TestOnlyGET(t *testing.T) {
	_, _, url := agent(t, untaggedSelf, &tailnet.Node{UserID: owner})
	for _, m := range []string{http.MethodHead, http.MethodPost, http.MethodPut, http.MethodDelete, http.MethodOptions} {
		if status, _ := get(t, m, url+"/v1/pulse"); status != http.StatusMethodNotAllowed {
			t.Fatalf("%s = %d, want 405", m, status)
		}
	}
	if status, _ := get(t, http.MethodGet, url+"/v1/other"); status != http.StatusNotFound {
		t.Fatalf("GET /v1/other = %d, want 404", status)
	}
}

func TestWhoIsIsCachedByAddressForAboutThirtySeconds(t *testing.T) {
	tn, s, url := agent(t, untaggedSelf, &tailnet.Node{UserID: owner})
	clock := time.Unix(1_000_000, 0)
	s.now = func() time.Time { return clock }

	for range 3 {
		get(t, http.MethodGet, url+"/v1/pulse")
	}
	if n := tn.WhoIsCalls(); n != 1 {
		t.Fatalf("WhoIs calls = %d, want 1 within the TTL", n)
	}

	// The cached answer stands until it expires: a caller retagged within the TTL is still allowed.
	tn.SetPeer(callerAddr, tailnet.Node{UserID: owner, Tags: []string{"tag:ci"}})
	clock = clock.Add(29 * time.Second)
	if status, _ := get(t, http.MethodGet, url+"/v1/pulse"); status != http.StatusOK {
		t.Fatalf("status = %d inside the TTL, want the cached 200", status)
	}
	clock = clock.Add(2 * time.Second)
	if status, _ := get(t, http.MethodGet, url+"/v1/pulse"); status != http.StatusForbidden {
		t.Fatalf("status = %d after the TTL, want 403 for the retagged caller", status)
	}
	if n := tn.WhoIsCalls(); n != 2 {
		t.Fatalf("WhoIs calls = %d, want 2", n)
	}
}

func TestNotPeerIsCachedOnlyBrieflyAndLocalAPIErrorsAreNot(t *testing.T) {
	tn, s, url := agent(t, untaggedSelf, nil)
	clock := time.Unix(1_000_000, 0)
	s.now = func() time.Time { return clock }
	get(t, http.MethodGet, url+"/v1/pulse")
	get(t, http.MethodGet, url+"/v1/pulse")
	if n := tn.WhoIsCalls(); n != 1 {
		t.Fatalf("WhoIs calls = %d for a cached stranger, want 1", n)
	}
	// As after a tailscaled restart: the Owner's phone was "no match" until the netmap loaded.
	tn.SetPeer(callerAddr, tailnet.Node{UserID: owner})
	clock = clock.Add(6 * time.Second)
	if status, _ := get(t, http.MethodGet, url+"/v1/pulse"); status != http.StatusOK {
		t.Fatalf("status = %d 6 s after a not-a-peer answer, want 200 from a fresh WhoIs", status)
	}

	tn, _, url = agent(t, untaggedSelf, &tailnet.Node{UserID: owner})
	tn.FailWhoIs(errors.New("connection refused"))
	if status, _ := get(t, http.MethodGet, url+"/v1/pulse"); status != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want 503", status)
	}
	tn.FailWhoIs(nil)
	if status, _ := get(t, http.MethodGet, url+"/v1/pulse"); status != http.StatusOK {
		t.Fatalf("status = %d once tailscaled recovers, want 200", status)
	}
}

func TestListenBindsAddressesNotYetOnAnInterface(t *testing.T) {
	// Fixture addresses are on no interface here, as the tailnet's aren't before tailscaled has started.
	lns, err := Listen(context.Background(), []netip.Addr{netip.MustParseAddr("100.64.0.1")}, 0)
	if err != nil {
		t.Fatalf("Listen: %v", err)
	}
	lns[0].Close()

	lns, err = Listen(context.Background(), []netip.Addr{netip.MustParseAddr("fd7a:115c:a1e0::1")}, 0)
	if errors.Is(err, errNothingBound) {
		t.Skip("IPv6 is disabled on this host")
	}
	if err != nil {
		t.Fatalf("Listen v6: %v", err)
	}
	lns[0].Close()
}

func TestListenSkipsIPv6OnlyWhenTheKernelHasNone(t *testing.T) {
	v4, v6 := netip.MustParseAddr("127.0.0.1"), netip.MustParseAddr("fd7a:115c:a1e0::1")
	failing := func(failNetwork string, errno syscall.Errno) bindFunc {
		return func(ctx context.Context, network, address string) (net.Listener, error) {
			if network == failNetwork {
				return nil, &net.OpError{Op: "listen", Net: network, Err: os.NewSyscallError("socket", errno)}
			}
			return net.Listen("tcp4", "127.0.0.1:0")
		}
	}
	ctx := context.Background()

	lns, err := listen(ctx, []netip.Addr{v4, v6}, 0, failing("tcp6", syscall.EAFNOSUPPORT))
	if err != nil || len(lns) != 1 {
		t.Fatalf("listen with IPv6 disabled = %d listeners, %v; want the IPv4 one", len(lns), err)
	}
	lns[0].Close()

	if _, err := listen(ctx, []netip.Addr{v6}, 0, failing("tcp6", syscall.EAFNOSUPPORT)); !errors.Is(err, errNothingBound) {
		t.Fatalf("listen with only an unusable IPv6 address = %v, want errNothingBound", err)
	}
	if _, err := listen(ctx, []netip.Addr{v4, v6}, 0, failing("tcp6", syscall.EADDRINUSE)); err == nil {
		t.Fatal("an IPv6 bind failing for another reason was skipped, want an error")
	}
	if _, err := listen(ctx, []netip.Addr{v4, v6}, 0, failing("tcp4", syscall.EAFNOSUPPORT)); err == nil {
		t.Fatal("an IPv4 bind failure was skipped, want an error")
	}
}

func TestListenBindsOnlyTheGivenAddresses(t *testing.T) {
	lns, err := Listen(context.Background(), []netip.Addr{netip.MustParseAddr("127.0.0.1")}, 0)
	if err != nil {
		t.Fatal(err)
	}
	defer lns[0].Close()
	got := netip.MustParseAddrPort(lns[0].Addr().String()).Addr()
	if got != netip.MustParseAddr("127.0.0.1") {
		t.Fatalf("bound %v, want only 127.0.0.1", got)
	}
}

func TestRunFailsWhenSelfFails(t *testing.T) {
	tn := tailnet.NewFake(tailnet.Self{})
	tn.SetSelf(tailnet.Self{}, errors.New("dial unix /run/tailscale/tailscaled.sock: connect: no such file"))
	if err := Run(context.Background(), tn, fixedRecorder{}, 0); err == nil {
		t.Fatal("Run succeeded without tailscaled, want an error")
	}
}

func TestRunExitsWhenIdentityChanges(t *testing.T) {
	old := selfRecheck
	selfRecheck = 10 * time.Millisecond
	t.Cleanup(func() { selfRecheck = old })

	self := tailnet.Self{Node: tailnet.Node{UserID: owner}, Addrs: []netip.Addr{netip.MustParseAddr("127.0.0.1")}}
	tn := tailnet.NewFake(self)
	done := make(chan error, 1)
	go func() { done <- Run(context.Background(), tn, fixedRecorder{}, 0) }()

	select {
	case err := <-done:
		t.Fatalf("Run returned %v while the identity was unchanged", err)
	case <-time.After(100 * time.Millisecond):
	}
	moved := self
	moved.Addrs = []netip.Addr{netip.MustParseAddr("127.0.0.2")}
	tn.SetSelf(moved, nil)
	select {
	case err := <-done:
		if err == nil {
			t.Fatal("Run returned nil, want an error so the Agent exits non-zero")
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Run kept serving after the tailnet addresses changed")
	}
}

func TestRunStopsCleanlyOnCancel(t *testing.T) {
	self := tailnet.Self{Node: tailnet.Node{UserID: owner}, Addrs: []netip.Addr{netip.MustParseAddr("127.0.0.1")}}
	// A SIGTERM can arrive before the bind or after it; neither is a failure.
	for _, after := range []time.Duration{0, 50 * time.Millisecond} {
		ctx, cancel := context.WithCancel(context.Background())
		if after == 0 {
			cancel()
		}
		done := make(chan error, 1)
		go func() { done <- Run(ctx, tailnet.NewFake(self), fixedRecorder{}, 0) }()
		time.Sleep(after)
		cancel()
		select {
		case err := <-done:
			if err != nil {
				t.Fatalf("Run = %v when cancelled after %v, want nil", err, after)
			}
		case <-time.After(5 * time.Second):
			t.Fatalf("Run did not stop when cancelled after %v", after)
		}
	}
}
