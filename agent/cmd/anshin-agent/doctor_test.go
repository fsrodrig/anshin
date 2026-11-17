package main

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io/fs"
	"net"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
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

func testEnv(vars map[string]string) doctorEnv {
	var elapsed time.Duration
	return doctorEnv{
		getenv: env(vars), euid: 1000, version: "1.2.3", socket: "/run/tailscale/tailscaled.sock",
		tn: tailnet.NewFake(untaggedSelf), listen: bindsAll,
		agentAt: func(context.Context, netip.AddrPort) bool { return false },
		host:    probe.Host{FS: fstest.MapFS{}}, settle: func() {},
		clocks: func() (recorder.Instant, error) {
			elapsed += doctorSettle
			return recorder.Instant{Monotonic: elapsed, Boottime: elapsed}, nil
		},
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

// captureHost replays the desktop capture's first snapshot, which never changes, so the doctor's two Samples see
// the same Machine, as two Samples a second apart nearly always do.
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

const mountinfo = "proc/self/mountinfo"

// unmountAfterSettle drops a mount from the table once the doctor has settled, so only a slow-lane re-read sees it gone.
type unmountAfterSettle struct {
	fs.FS
	mountinfo []byte
	settled   atomic.Bool
}

func (u *unmountAfterSettle) Open(name string) (fs.File, error) {
	if name == mountinfo && u.settled.Load() {
		return fstest.MapFS{mountinfo: {Data: u.mountinfo}}.Open(name)
	}
	return u.FS.Open(name)
}

func TestDoctorUsesInjectedClocks(t *testing.T) {
	t.Parallel()
	for _, elapsed := range []time.Duration{doctorSettle, 31 * time.Second} {
		t.Run(elapsed.String(), func(t *testing.T) {
			t.Parallel()
			e := testEnv(nil)
			e.host = captureHost(t)
			mounts, err := fs.ReadFile(e.host.FS, mountinfo)
			if err != nil {
				t.Fatal(err)
			}
			var kept []string
			for _, line := range strings.SplitAfter(string(mounts), "\n") {
				if !strings.Contains(line, " /boot ") {
					kept = append(kept, line)
				}
			}
			unmounted := &unmountAfterSettle{FS: e.host.FS, mountinfo: []byte(strings.Join(kept, ""))}
			e.host.FS = unmounted
			settled := false
			e.settle = func() {
				time.Sleep(doctorSettle)
				settled = true
				unmounted.settled.Store(true)
			}
			calls := 0
			e.clocks = func() (recorder.Instant, error) {
				if settled != (calls == 1) {
					t.Fatal("clocks must be read before each Sample, around the settle")
				}
				at := time.Hour + time.Duration(calls)*elapsed
				calls++
				return recorder.Instant{Monotonic: at, Boottime: at}, nil
			}
			code, out := runDoctorEnv(t, e)
			if code != 0 || calls != 2 {
				t.Fatalf("exit %d, clock calls %d; want 0, 2", code, calls)
			}
			if elapsed == doctorSettle {
				wantLines(t, out, "/ (btrfs, 0:35): 1022042832896 bytes")
				if strings.Contains(out, "capacity unavailable") {
					t.Errorf("settled capacities lost:\n%s", out)
				}
			} else {
				wantLines(t, out, "/ (btrfs, 0:35): capacity unavailable")
			}
			// Only a slow-lane re-read can see the unmount, so /boot's absence proves the injected clocks drove it.
			if listed := strings.Contains(out, "/boot (vfat"); listed != (elapsed == doctorSettle) {
				t.Errorf("/boot listed %v after %v:\n%s", listed, elapsed, out)
			}
		})
	}
}

func TestDoctorReportsClockErrors(t *testing.T) {
	t.Parallel()
	for _, failAt := range []int{1, 2} {
		t.Run(fmt.Sprint(failAt), func(t *testing.T) {
			t.Parallel()
			e := testEnv(nil)
			calls, settles := 0, 0
			e.clocks = func() (recorder.Instant, error) {
				calls++
				if calls == failAt {
					return recorder.Instant{}, errors.New("clock unavailable")
				}
				return recorder.Instant{Monotonic: time.Hour, Boottime: time.Hour}, nil
			}
			e.settle = func() { settles++ }
			code, out := runDoctorEnv(t, e)
			if code != 1 || calls != failAt || settles != failAt-1 {
				t.Errorf("exit %d, clock calls %d, settles %d; want 1, %d, %d", code, calls, settles, failAt, failAt-1)
			}
			wantLines(t, out, "✗ This Machine's clocks could not be read, so the Agent would not start.", "clock unavailable")
			if strings.Contains(out, "CPU sensor") || strings.Contains(out, "Filesystems") || strings.Contains(out, "Nothing stops") {
				t.Errorf("measurements reported after a clock failure:\n%s", out)
			}
		})
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
