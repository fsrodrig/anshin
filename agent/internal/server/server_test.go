package server

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"os"
	"reflect"
	"strings"
	"syscall"
	"testing"
	"testing/fstest"
	"time"

	"github.com/fsrodrig/anshin/agent/internal/recorder"
	"github.com/fsrodrig/anshin/agent/internal/recorder/probe"
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

func TestHeadersCannotChangeWhoTheCallerIs(t *testing.T) {
	ownerPhone := netip.MustParseAddr("100.64.0.10")
	for _, caller := range []*tailnet.Node{nil, {UserID: stranger}} {
		tn, _, url := agent(t, untaggedSelf, caller)
		tn.SetPeer(ownerPhone, tailnet.Node{UserID: owner})
		req, err := http.NewRequest(http.MethodGet, url+"/v1/pulse", nil)
		if err != nil {
			t.Fatal(err)
		}
		for _, h := range []string{"X-Forwarded-For", "X-Real-IP", "True-Client-IP", "X-Client-IP"} {
			req.Header.Set(h, ownerPhone.String())
		}
		req.Header.Set("Forwarded", "for="+ownerPhone.String())
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		resp.Body.Close()
		if resp.StatusCode != http.StatusForbidden {
			t.Fatalf("caller %v claiming the Owner's phone in headers = %d, want 403", caller, resp.StatusCode)
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
	for _, k := range strings.Fields("mem_total_bytes mem_available_bytes mem_used_bytes mem_used_pct swap_total_bytes swap_used_bytes swap_entries load_1 load_5 load_15 uptime_s distro kernel arch agent_version filesystems primary_disk cpu_temp_c cpu_sensor temperatures net_rx_bps net_tx_bps net_interfaces disks battery verdict") {
		want[k] = nil
	}
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
	for _, field := range []string{`"hostname":null`, `"cpu_pct":null`, `"cpu_temp_c":null`, `"cpu_sensor":null`, `"temperatures":null`} {
		if !strings.Contains(body, field) {
			t.Fatalf("Pulse = %s, want %s", body, field)
		}
	}
}

// The CPU sensor's critical temperature is for the Verdict; the Pulse doesn't carry it until the phone needs it.
func TestCPUCritStaysOffTheWire(t *testing.T) {
	r := recorder.Reading{TemperatureReading: probe.TemperatureReading{CPUCritC: ptr(100.0)}}
	_, _, url := serve(t, untaggedSelf, &tailnet.Node{UserID: owner}, fixedRecorder{r})
	_, body := get(t, http.MethodGet, url+"/v1/pulse")
	if strings.Contains(body, "crit") || strings.Contains(body, "100") {
		t.Fatalf("Pulse = %s, want no critical temperature", body)
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

func TestRunExitsWhenThisMachineIsTaggedOrChangesUser(t *testing.T) {
	old := selfRecheck
	selfRecheck = 10 * time.Millisecond
	t.Cleanup(func() { selfRecheck = old })

	self := tailnet.Self{Node: tailnet.Node{UserID: owner}, Addrs: []netip.Addr{netip.MustParseAddr("127.0.0.1")}}
	tagged, reowned := self, self
	tagged.Tags = []string{"tag:server"}
	reowned.UserID = stranger
	for name, changed := range map[string]tailnet.Self{"tagged": tagged, "other user": reowned} {
		t.Run(name, func(t *testing.T) {
			tn := tailnet.NewFake(self)
			done := make(chan error, 1)
			go func() { done <- Run(context.Background(), tn, fixedRecorder{}, 0) }()
			select {
			case err := <-done:
				t.Fatalf("Run returned %v while the identity was unchanged", err)
			case <-time.After(100 * time.Millisecond):
			}
			tn.SetSelf(changed, nil)
			select {
			case err := <-done:
				if err == nil {
					t.Fatal("Run returned nil, want an error so the Agent exits non-zero")
				}
			case <-time.After(5 * time.Second):
				t.Fatal("Run kept serving after this Machine's identity changed")
			}
		})
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

func TestPulseMeasurementRoundTrip(t *testing.T) {
	r := recorder.Reading{
		Hostname: ptr("machine"), CPUPct: ptr(0.0),
		MemoryReading: probe.MemoryReading{Total: ptr(uint64(4096)), Available: ptr(uint64(3072)), Used: ptr(uint64(1024)), Pct: ptr(25.0)},
		SwapReading:   probe.SwapReading{Total: ptr(uint64(2048)), Used: ptr(uint64(0)), Entries: []probe.SwapEntry{{Name: "/dev/zram0", Zram: true, Total: ptr(uint64(2048)), Used: ptr(uint64(0))}}},
		LoadReading:   probe.LoadReading{One: ptr(0.0), Five: ptr(1.25), Fifteen: ptr(2.5), Uptime: ptr(123.45)},
		MachineInfo:   probe.MachineInfo{Distro: ptr("Example Linux"), Kernel: ptr("test-kernel"), Architecture: ptr("aarch64"), AgentVersion: ptr("test-build")},
		TemperatureReading: probe.TemperatureReading{CPUTempC: ptr(0.0), CPUSensor: ptr("k10temp/Tctl"),
			Sensors: []probe.Sensor{{ID: "k10temp/Tctl", TempC: ptr(0.0), Age: ptr(int64(0))}, {ID: "amdgpu/edge", TempC: ptr(67.0), Age: ptr(int64(4))}, {ID: "amdgpu/junction"}}},
		NetRx: ptr(uint64(0)), NetTx: ptr(uint64(2048)),
		NetInterfaces: []recorder.NetInterface{{Name: "eth0", DeviceBacked: ptr(true), Rx: ptr(uint64(0)), Tx: ptr(uint64(2048))}, {Name: "wg0", DeviceBacked: nil, Rx: nil, Tx: ptr(uint64(1))}},
		Disks:         []recorder.Disk{{Name: "sda", Read: ptr(uint64(512)), Write: nil}},
		Battery: &probe.BatteryReading{Pct: ptr(59.1), Status: ptr("Discharging"), PowerW: ptr(0.0), TimeLeft: ptr(uint64(13371)), Batteries: []probe.BatteryRow{
			{Name: "BAT0", Pct: ptr(75.0), Status: ptr("Unknown"), PowerW: ptr(0.0), Health: ptr(104.2)},
			{Name: "BAT1"},
		}},
	}
	_, _, url := serve(t, untaggedSelf, &tailnet.Node{UserID: owner}, fixedRecorder{r})
	status, body := get(t, http.MethodGet, url+"/v1/pulse")
	var got pulse
	if err := json.Unmarshal([]byte(body), &got); err != nil {
		t.Fatal(err)
	}
	if status != http.StatusOK || got.Schema != 1 || got.Kind != "full" || !reflect.DeepEqual(got.Reading, r) {
		t.Fatalf("round trip: %s", body)
	}
	var wire struct {
		Temperatures []map[string]any `json:"temperatures"`
	}
	if err := json.Unmarshal([]byte(body), &wire); err != nil {
		t.Fatal(err)
	}
	for i, want := range []any{0.0, 4.0, nil} {
		if age, ok := wire.Temperatures[i]["age_s"]; !ok || age != want {
			t.Fatalf("temperatures[%d].age_s = %v (present %t), want %v: %s", i, age, ok, want, body)
		}
	}
}

// This is a synthetic no-swap tree, not a real-Machine Capture.
func TestPulseSyntheticNoSwap(t *testing.T) {
	h := probe.Host{FS: fstest.MapFS{"proc/swaps": &fstest.MapFile{Data: []byte("Filename Type Size Used Priority\n")}, "proc/meminfo": &fstest.MapFile{Data: []byte("MemTotal: 100 kB\nMemAvailable: 100 kB\n")}}}
	_, _, url := serve(t, untaggedSelf, &tailnet.Node{UserID: owner}, recorder.New(h, time.Now(), recorder.Config{}))
	status, body := get(t, http.MethodGet, url+"/v1/pulse")
	var got map[string]any
	if err := json.Unmarshal([]byte(body), &got); err != nil {
		t.Fatal(err)
	}
	for _, k := range []string{"swap_total_bytes", "swap_used_bytes", "swap_entries"} {
		if v, ok := got[k]; !ok || v != nil {
			t.Fatalf("%s: %s", k, body)
		}
	}
	if status != http.StatusOK || got["mem_used_pct"] != 0.0 {
		t.Fatal(body)
	}
}

// This is a synthetic laptop tree with a Bluetooth peripheral, not a real-Machine Capture. The peripheral's MAC is
// built at runtime so no identifier sits in this source.
func TestPulseSyntheticBatteries(t *testing.T) {
	var octets []string
	for i := range 6 {
		octets = append(octets, fmt.Sprintf("%02X", 0x5a+i*7))
	}
	peripheral := "hid-" + strings.Join(octets, ":") + "-battery"
	file := func(s string) *fstest.MapFile { return &fstest.MapFile{Data: []byte(s + "\n")} }
	h := probe.Host{FS: fstest.MapFS{
		"sys/class/power_supply/BAT0/type":                   file("Battery"),
		"sys/class/power_supply/BAT0/status":                 file("Discharging"),
		"sys/class/power_supply/BAT0/capacity":               file("81"),
		"sys/class/power_supply/BAT0/energy_now":             file("40500000"),
		"sys/class/power_supply/BAT0/power_now":              file("8100000"),
		"sys/class/power_supply/AC/type":                     file("Mains"),
		"sys/class/power_supply/" + peripheral + "/type":     file("Battery"),
		"sys/class/power_supply/" + peripheral + "/scope":    file("Device"),
		"sys/class/power_supply/" + peripheral + "/capacity": file("50"),
	}}
	_, _, url := serve(t, untaggedSelf, &tailnet.Node{UserID: owner}, recorder.New(h, time.Now(), recorder.Config{}))
	status, body := get(t, http.MethodGet, url+"/v1/pulse")
	if status != http.StatusOK {
		t.Fatalf("status = %d", status)
	}
	if strings.Contains(strings.ToLower(body), strings.ToLower(peripheral)) || strings.Contains(body, "hid-") {
		t.Fatal("the Pulse names a peripheral battery")
	}
	var got struct {
		Battery map[string]json.RawMessage `json:"battery"`
	}
	if err := json.Unmarshal([]byte(body), &got); err != nil {
		t.Fatal(err)
	}
	want := []string{"pct", "status", "power_w", "time_left_s", "batteries"}
	if len(got.Battery) != len(want) {
		t.Fatalf("battery = %s, want exactly the keys %v", body, want)
	}
	for _, k := range want {
		if _, ok := got.Battery[k]; !ok {
			t.Errorf("battery.%s missing", k)
		}
	}
	var rows []map[string]json.RawMessage
	if err := json.Unmarshal(got.Battery["batteries"], &rows); err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 || string(rows[0]["name"]) != `"BAT0"` {
		t.Fatalf("batteries = %s, want only BAT0", got.Battery["batteries"])
	}
	rowKeys := []string{"name", "pct", "status", "power_w", "health_pct"}
	if len(rows[0]) != len(rowKeys) {
		t.Fatalf("battery row = %s, want exactly the keys %v", got.Battery["batteries"], rowKeys)
	}
	for _, k := range rowKeys {
		if _, ok := rows[0][k]; !ok {
			t.Errorf("battery row.%s missing", k)
		}
	}
}
