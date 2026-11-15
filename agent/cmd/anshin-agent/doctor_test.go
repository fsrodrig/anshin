package main

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"testing/fstest"
	"time"

	"github.com/fsrodrig/anshin/agent/internal/capture/replay"
	"github.com/fsrodrig/anshin/agent/internal/recorder"
	"github.com/fsrodrig/anshin/agent/internal/recorder/probe"
	"github.com/fsrodrig/anshin/agent/internal/server"
	"github.com/fsrodrig/anshin/agent/internal/tailnet"
)

var (
	v4, v6       = netip.MustParseAddr("100.64.0.1"), netip.MustParseAddr("fd7a:115c:a1e0::1")
	untaggedSelf = tailnet.Self{Node: tailnet.Node{UserID: 4242}, Addrs: []netip.Addr{v4, v6}}
)

type fakeListener struct{ addr netip.AddrPort }

func (l fakeListener) Accept() (net.Conn, error) { return nil, net.ErrClosed }
func (l fakeListener) Close() error              { return nil }
func (l fakeListener) Addr() net.Addr            { return net.TCPAddrFromAddrPort(l.addr) }

// bindsAll stands in for server.Listen on a Machine where every address binds, so tests never bind tailnet
// addresses they don't own.
func bindsAll(_ context.Context, addrs []netip.Addr, port int) ([]net.Listener, error) {
	var lns []net.Listener
	for _, a := range addrs {
		lns = append(lns, fakeListener{netip.AddrPortFrom(a, uint16(port))})
	}
	return lns, nil
}

// testEnv is a healthy untagged Machine with no measurements; each test changes what it is about.
func testEnv(vars map[string]string) doctorEnv {
	return doctorEnv{
		getenv: env(vars), euid: 1000, version: "1.2.3", socket: "/run/tailscale/tailscaled.sock",
		tn: tailnet.NewFake(untaggedSelf), listen: bindsAll,
		agentAt: func(context.Context, netip.AddrPort) bool { return false },
		host:    probe.Host{FS: fstest.MapFS{}}, settle: func() {},
	}
}

func runDoctorEnv(t *testing.T, e doctorEnv) (int, string) {
	t.Helper()
	var out bytes.Buffer
	code := doctor(context.Background(), e, &out)
	return code, out.String()
}

func wantLines(t *testing.T, out string, lines ...string) {
	t.Helper()
	for _, l := range lines {
		if !strings.Contains(out, l) {
			t.Errorf("output lacks %q:\n%s", l, out)
		}
	}
}

// captureHost replays the desktop capture's first snapshot, which never changes, so the doctor's two Reads see the
// same Machine, as two Reads a second apart nearly always do.
func captureHost(t *testing.T) probe.Host {
	t.Helper()
	for _, c := range replay.LoadAll(t) {
		if c.Name == "desktop" {
			return c.Snapshots[0].Host()
		}
	}
	t.Fatal("no desktop capture")
	return probe.Host{}
}

func TestDoctorOnACapture(t *testing.T) {
	t.Parallel()
	e := testEnv(nil)
	e.host = captureHost(t)
	e.settle = func() { time.Sleep(doctorSettle) }
	code, out := runDoctorEnv(t, e)
	if code != 0 {
		t.Errorf("exit %d, want 0", code)
	}
	wantLines(t, out,
		"anshin-agent 1.2.3\n",
		"  ✓ tailscaled answers on /run/tailscale/tailscaled.sock (the default; TAILSCALE_SOCKET changes it).\n",
		"  ✓ This Machine is on the tailnet as 100.64.0.1 and fd7a:115c:a1e0::1.\n",
		"  ✓ This Machine is untagged, so the Owner's own untagged devices may watch it.\n",
		"  ✓ Port 7247, the default (ANSHIN_PORT changes it).\n",
		"  ✓ The Agent can bind 100.64.0.1:7247.\n",
		"  ✓ The Agent can bind [fd7a:115c:a1e0::1]:7247.\n",
		"  ✓ k10temp/Tctl, chosen automatically: ",
		"add `Environment=\"ANSHIN_CPU_SENSOR=<hwmon name>/<label>\"` under [Service]",
		"  - amdgpu/edge: ",
		"  - No system battery, so the phone shows none.\n",
		"  - / (btrfs, 0:35): 1022042832896 bytes (951.9 GiB), 8.8 % used; the primary disk.\n",
		"  - /boot (vfat, 259:1): ",
		"\nNothing stops the Agent from serving the Owner's phone.\n",
	)
	if strings.Contains(out, "✗") {
		t.Errorf("a healthy Machine shows a problem:\n%s", out)
	}
}

func TestDoctorExplainsTheCPUSensorOverride(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		override string
		code     int
		line     string
	}{
		{"amdgpu/edge", 0, "  ✓ amdgpu/edge, named by ANSHIN_CPU_SENSOR: "},
		{"it87/temp1", 0, "  ! ANSHIN_CPU_SENSOR=it87/temp1 matches no sensor on this Machine, so the CPU temperature is unavailable: the Agent never falls back to another sensor.\n"},
		{"k10temp", 1, "  ✗ ANSHIN_CPU_SENSOR=\"k10temp\" is not <hwmon name>/<label>"},
	} {
		t.Run(tc.override, func(t *testing.T) {
			t.Parallel()
			e := testEnv(map[string]string{"ANSHIN_CPU_SENSOR": tc.override})
			e.host = captureHost(t)
			e.settle = func() { time.Sleep(doctorSettle) }
			code, out := runDoctorEnv(t, e)
			if code != tc.code {
				t.Errorf("exit %d, want %d", code, tc.code)
			}
			wantLines(t, out, tc.line)
		})
	}
}

func TestDoctorReportsUnavailableMeasurements(t *testing.T) {
	t.Parallel()
	var out bytes.Buffer
	r := report{w: &out}
	sensor := "coretemp/Package id 0"
	r.cpuSensor(recorder.Reading{CPUSensor: &sensor}, "", nil)
	r.battery(&probe.BatteryReading{Batteries: []probe.BatteryRow{{Name: "BAT0"}}})
	wantLines(t, out.String(),
		"coretemp/Package id 0, chosen automatically: reading unavailable.",
		"BAT0: charge unavailable, status unavailable, health unavailable.",
	)
	if strings.Contains(out.String(), "0.0 °C") || strings.Contains(out.String(), "0 %") {
		t.Errorf("unavailable measurements became zero:\n%s", out.String())
	}
}

func TestDoctorListsSystemBatteriesOnly(t *testing.T) {
	t.Parallel()
	var octets []string
	for i := range 6 {
		octets = append(octets, fmt.Sprintf("%02X", 0x5a+i*7))
	}
	peripheral := "hid-" + strings.Join(octets, ":") + "-battery"
	file := func(s string) *fstest.MapFile { return &fstest.MapFile{Data: []byte(s + "\n")} }
	e := testEnv(nil)
	e.host = probe.Host{FS: fstest.MapFS{
		"sys/class/power_supply/BAT0/type":                   file("Battery"),
		"sys/class/power_supply/BAT0/status":                 file("Discharging"),
		"sys/class/power_supply/BAT0/capacity":               file("81"),
		"sys/class/power_supply/BAT0/energy_now":             file("40500000"),
		"sys/class/power_supply/BAT0/energy_full":            file("50000000"),
		"sys/class/power_supply/BAT0/energy_full_design":     file("52000000"),
		"sys/class/power_supply/BAT0/power_now":              file("8100000"),
		"sys/class/power_supply/AC/type":                     file("Mains"),
		"sys/class/power_supply/" + peripheral + "/type":     file("Battery"),
		"sys/class/power_supply/" + peripheral + "/scope":    file("Device"),
		"sys/class/power_supply/" + peripheral + "/capacity": file("50"),
	}}
	_, out := runDoctorEnv(t, e)
	wantLines(t, out, "  - BAT0: 81 %, Discharging, health 96 %.\n")
	if strings.Contains(out, "hid-") || strings.Contains(out, "- AC") {
		t.Errorf("the doctor names a supply that is not a system battery:\n%s", out)
	}
}

func TestDoctorExplainsWhyTailscaledCannotAnswer(t *testing.T) {
	t.Parallel()
	stub := func(t *testing.T, body string) string {
		sock := filepath.Join(t.TempDir(), "tailscaled.sock")
		ln, err := net.Listen("unix", sock)
		if err != nil {
			t.Fatal(err)
		}
		srv := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.Write([]byte(body)) }))
		srv.Listener = ln
		srv.Start()
		t.Cleanup(srv.Close)
		return sock
	}
	for _, tc := range []struct {
		name   string
		socket func(t *testing.T) string
		line   string
	}{
		{"missing", func(t *testing.T) string { return filepath.Join(t.TempDir(), "tailscaled.sock") }, "No tailscaled socket at "},
		{"stale", func(t *testing.T) string {
			sock := filepath.Join(t.TempDir(), "tailscaled.sock")
			ln, err := net.Listen("unix", sock)
			if err != nil {
				t.Fatal(err)
			}
			ln.(*net.UnixListener).SetUnlinkOnClose(false)
			ln.Close()
			return sock
		}, "Nothing answers on "},
		{"unreadable", func(t *testing.T) string {
			if os.Geteuid() == 0 {
				t.Skip("root connects to any socket")
			}
			sock := stub(t, "{}")
			if err := os.Chmod(sock, 0); err != nil {
				t.Fatal(err)
			}
			return sock
		}, "Permission denied on "},
		{"logged out", func(t *testing.T) string { return stub(t, `{"BackendState":"NeedsLogin"}`) }, "tailscaled is logged out (NeedsLogin)."},
		{"stopped", func(t *testing.T) string { return stub(t, `{"BackendState":"Stopped"}`) }, "Tailscale is stopped (Stopped)."},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			sock := tc.socket(t)
			e := testEnv(map[string]string{"TAILSCALE_SOCKET": sock})
			e.socket, e.tn = sock, tailnet.NewLocalAPI(sock)
			e.listen = func(context.Context, []netip.Addr, int) ([]net.Listener, error) {
				t.Error("bound without tailnet addresses")
				return nil, errors.New("unreachable")
			}
			code, out := runDoctorEnv(t, e)
			if code != 1 {
				t.Errorf("exit %d, want 1", code)
			}
			wantLines(t, out, "  ✗ "+tc.line, "  ! Not tried: the Agent binds only the tailnet addresses tailscaled reports.\n")
			if strings.Contains(tc.line, " on ") || strings.Contains(tc.line, " at ") {
				wantLines(t, out, sock+", from TAILSCALE_SOCKET")
			}
		})
	}
}

func TestDoctorRefusesATaggedMachine(t *testing.T) {
	t.Parallel()
	e := testEnv(nil)
	e.tn = tailnet.NewFake(tailnet.Self{Node: tailnet.Node{UserID: 4242, Tags: []string{"tag:server"}}, Addrs: []netip.Addr{v4}})
	code, out := runDoctorEnv(t, e)
	if code != 1 {
		t.Errorf("exit %d, want 1", code)
	}
	wantLines(t, out, "  ✗ This Machine is tagged (tag:server), so the Agent answers every phone Forbidden")
}

func TestDoctorExplainsTheBind(t *testing.T) {
	t.Parallel()
	inUse := func(context.Context, []netip.Addr, int) ([]net.Listener, error) {
		return nil, fmt.Errorf("binding %s: %w", v4, &net.OpError{Op: "listen", Net: "tcp4", Err: os.NewSyscallError("bind", syscall.EADDRINUSE)})
	}
	for _, tc := range []struct {
		name    string
		vars    map[string]string
		listen  func(context.Context, []netip.Addr, int) ([]net.Listener, error)
		agentAt func(context.Context, netip.AddrPort) bool
		code    int
		lines   []string
	}{
		{"port from ANSHIN_PORT", map[string]string{"ANSHIN_PORT": "8080"}, bindsAll, nil, 0,
			[]string{"  ✓ Port 8080, from ANSHIN_PORT.\n", "  ✓ The Agent can bind 100.64.0.1:8080.\n"}},
		{"bad ANSHIN_PORT", map[string]string{"ANSHIN_PORT": "http"}, nil, nil, 1,
			[]string{"  ✗ ANSHIN_PORT=\"http\" is not a TCP port from 1 to 65535, so the Agent would not start.\n"}},
		{"IPv6 disabled", nil, func(ctx context.Context, addrs []netip.Addr, port int) ([]net.Listener, error) {
			return bindsAll(ctx, addrs[:1], port)
		}, nil, 0, []string{"  ✓ The Agent can bind 100.64.0.1:7247.\n", "  ! IPv6 is disabled on this Machine, so the Agent skips fd7a:115c:a1e0::1 and serves on IPv4 alone.\n"}},
		{"in use by the Agent", nil, inUse, func(_ context.Context, ap netip.AddrPort) bool { return ap.Addr() == v4 }, 0,
			[]string{"  ✓ An Agent already serves on 100.64.0.1:7247, most likely the anshin-agent service.\n"}},
		{"in use by another program", nil, inUse, nil, 1,
			[]string{"  ✗ Port 7247 is already in use on this Machine's tailnet address, and what holds it does not answer as an Agent."}},
		{"privileged port", map[string]string{"ANSHIN_PORT": "80"}, func(context.Context, []netip.Addr, int) ([]net.Listener, error) {
			return nil, fmt.Errorf("binding %s: %w", v4, &net.OpError{Op: "listen", Net: "tcp4", Err: os.NewSyscallError("bind", syscall.EACCES)})
		}, nil, 1, []string{"  ✗ The Agent may not bind port 80: ports below 1024 need a privilege it never has."}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			e := testEnv(tc.vars)
			e.listen = tc.listen
			if tc.agentAt != nil {
				e.agentAt = tc.agentAt
			}
			code, out := runDoctorEnv(t, e)
			if code != tc.code {
				t.Errorf("exit %d, want %d", code, tc.code)
			}
			wantLines(t, out, tc.lines...)
		})
	}
}

// TestDoctorSeesARealPortInUse binds with the Agent's own Listen, so a change in how it reports a taken port can't
// leave the doctor guessing.
func TestDoctorSeesARealPortInUse(t *testing.T) {
	t.Parallel()
	held, err := server.Listen(context.Background(), []netip.Addr{v4}, 0)
	if err != nil {
		t.Fatal(err)
	}
	defer held[0].Close()
	port := netip.MustParseAddrPort(held[0].Addr().String()).Port()
	e := testEnv(map[string]string{"ANSHIN_PORT": fmt.Sprint(port)})
	e.tn = tailnet.NewFake(tailnet.Self{Node: tailnet.Node{UserID: 4242}, Addrs: []netip.Addr{v4}})
	e.listen = server.Listen
	code, out := runDoctorEnv(t, e)
	if code != 1 {
		t.Errorf("exit %d, want 1", code)
	}
	wantLines(t, out, fmt.Sprintf("  ✗ Port %d is already in use", port))
}

func TestAgentAnswersOnlyForAPulse(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name    string
		handler http.HandlerFunc
		want    bool
	}{
		{"a Pulse", func(w http.ResponseWriter, _ *http.Request) { w.Write([]byte(`{"schema":1,"kind":"full"}`)) }, true},
		{"forbidden", func(w http.ResponseWriter, _ *http.Request) { http.Error(w, "Forbidden", http.StatusForbidden) }, false},
		{"another program", func(w http.ResponseWriter, _ *http.Request) { w.Write([]byte("<html></html>")) }, false},
		{"JSON without a schema", func(w http.ResponseWriter, _ *http.Request) { w.Write([]byte(`{"ok":true}`)) }, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			srv := httptest.NewServer(tc.handler)
			defer srv.Close()
			if got := agentAnswers(context.Background(), netip.MustParseAddrPort(srv.Listener.Addr().String())); got != tc.want {
				t.Errorf("agentAnswers = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestDoctorUsage(t *testing.T) {
	var out, errOut bytes.Buffer
	if code := runDoctor([]string{"extra"}, &out, &errOut); code != 2 {
		t.Errorf("doctor extra = exit %d, want 2", code)
	}
	if code := runDoctor([]string{"-bogus"}, &out, &errOut); code != 2 {
		t.Errorf("doctor -bogus = exit %d, want 2", code)
	}
}
