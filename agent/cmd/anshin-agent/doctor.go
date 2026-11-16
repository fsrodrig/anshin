package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"io/fs"
	"log/slog"
	"net"
	"net/http"
	"net/netip"
	"os"
	"os/signal"
	"slices"
	"strings"
	"syscall"
	"time"

	"github.com/fsrodrig/anshin/agent/internal/recorder"
	"github.com/fsrodrig/anshin/agent/internal/recorder/probe"
	"github.com/fsrodrig/anshin/agent/internal/server"
	"github.com/fsrodrig/anshin/agent/internal/tailnet"
)

// doctorSettle is the pause between the doctor's two Samples: a filesystem's capacity is probed off the Sample that
// finds its mount and appears only on the next one.
const doctorSettle = time.Second

const selfTimeout = 10 * time.Second

// doctorEnv is everything the doctor touches, so tests can replace the live Machine, tailscaled and the bind.
type doctorEnv struct {
	getenv  func(string) string
	euid    int
	version string
	socket  string
	tn      tailnet.Client
	listen  func(ctx context.Context, addrs []netip.Addr, port int) ([]net.Listener, error)
	agentAt func(ctx context.Context, ap netip.AddrPort) bool
	host    probe.Host
	settle  func()
}

func runDoctor(args []string, stdout, stderr io.Writer) int {
	fl := flag.NewFlagSet("doctor", flag.ContinueOnError)
	fl.SetOutput(stderr)
	fl.Usage = func() { fmt.Fprintln(stderr, "usage: anshin-agent doctor") }
	if err := fl.Parse(args); err != nil {
		return 2
	}
	if fl.NArg() != 0 {
		fl.Usage()
		return 2
	}
	// The doctor explains every problem itself; the server's warnings would only repeat them.
	slog.SetDefault(slog.New(slog.DiscardHandler))
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	socket := tailnet.SocketPath()
	return doctor(ctx, doctorEnv{
		getenv: os.Getenv, euid: os.Geteuid(), version: agentVersion(), socket: socket,
		tn: tailnet.NewLocalAPI(socket), listen: server.Listen, agentAt: agentAnswers,
		host: liveHost(), settle: func() { time.Sleep(doctorSettle) },
	}, stdout)
}

func doctor(ctx context.Context, env doctorEnv, w io.Writer) int {
	r := &report{w: w}
	version := env.version
	if version == "" {
		version = "(version unknown)"
	}
	fmt.Fprintf(w, "anshin-agent %s\n", version)
	if env.euid == 0 {
		r.note("Running as root. The Agent runs as an unprivileged user, so it may read and bind less than this shows.")
	}
	self, ok := r.tailnet(ctx, env)
	r.bind(ctx, env, self, ok)
	r.machine(env)
	switch r.problems {
	case 0:
		fmt.Fprintln(w, "\nNothing stops the Agent from serving the Owner's phone.")
		return 0
	case 1:
		fmt.Fprintln(w, "\nThe problem above stops the Agent from serving the Owner's phone.")
	default:
		fmt.Fprintf(w, "\nThe %d problems above stop the Agent from serving the Owner's phone.\n", r.problems)
	}
	return 1
}

type report struct {
	w        io.Writer
	problems int
}

func (r *report) section(title string)         { fmt.Fprintf(r.w, "\n%s\n", title) }
func (r *report) ok(format string, a ...any)   { r.line("✓", format, a...) }
func (r *report) note(format string, a ...any) { r.line("!", format, a...) }
func (r *report) item(format string, a ...any) { r.line("-", format, a...) }
func (r *report) detail(format string, a ...any) {
	fmt.Fprintf(r.w, "    %s\n", fmt.Sprintf(format, a...))
}
func (r *report) problem(format string, a ...any) { r.problems++; r.line("✗", format, a...) }

func (r *report) line(mark, format string, a ...any) {
	fmt.Fprintf(r.w, "  %s %s\n", mark, fmt.Sprintf(format, a...))
}

func (r *report) tailnet(ctx context.Context, env doctorEnv) (tailnet.Self, bool) {
	r.section("Tailscale")
	where := env.socket + " (the default; TAILSCALE_SOCKET changes it)"
	if env.getenv("TAILSCALE_SOCKET") != "" {
		where = env.socket + ", from TAILSCALE_SOCKET"
	}
	ctx, cancel := context.WithTimeout(ctx, selfTimeout)
	defer cancel()
	self, err := env.tn.Self(ctx)
	if err != nil {
		r.problem("%s", tailnetProblem(err, where, env.socket))
		r.detail("The Agent exits until tailscaled can say who this Machine is, and systemd starts it again every 5 s.")
		return tailnet.Self{}, false
	}
	r.ok("tailscaled answers on %s.", where)
	addrs := make([]string, len(self.Addrs))
	for i, a := range self.Addrs {
		addrs[i] = a.String()
	}
	r.ok("This Machine is on the tailnet as %s.", strings.Join(addrs, " and "))
	if server.Allows(self, tailnet.Node{UserID: self.UserID}) {
		r.ok("This Machine is untagged, so the Owner's own untagged devices may watch it.")
	} else {
		r.problem("This Machine is tagged (%s), so the Agent answers every phone Forbidden: a tagged Machine belongs to the tailnet, not to an Owner. Remove the tag in the Tailscale admin console to watch it.", strings.Join(self.Tags, ", "))
	}
	return self, true
}

func tailnetProblem(err error, where, socket string) string {
	var notRunning *tailnet.NotRunningError
	var netErr net.Error
	switch {
	case errors.As(err, &notRunning):
		switch notRunning.State {
		case "NeedsLogin":
			return "tailscaled is logged out (NeedsLogin). Run `sudo tailscale up` and sign in as the Owner."
		case "NeedsMachineAuth":
			return "This Machine waits for approval in the Tailscale admin console (NeedsMachineAuth)."
		case "Stopped":
			return "Tailscale is stopped (Stopped). Run `sudo tailscale up`."
		case "Starting":
			return "tailscaled is still starting (Starting). Run doctor again in a moment."
		}
		return fmt.Sprintf("tailscaled reports %q, but the Agent needs it Running. `tailscale status` says why.", notRunning.State)
	case errors.Is(err, fs.ErrNotExist):
		return "No tailscaled socket at " + where + ". Is Tailscale installed and running? `sudo systemctl enable --now tailscaled`, then `sudo tailscale up`."
	case errors.Is(err, syscall.ECONNREFUSED):
		return "Nothing answers on " + where + ", so tailscaled is not running. Start it with `sudo systemctl start tailscaled`."
	case errors.Is(err, fs.ErrPermission):
		return "Permission denied on " + where + ". tailscaled's socket is normally open to every user; check it with `ls -l " + socket + "`."
	case errors.As(err, &netErr) && netErr.Timeout(), errors.Is(err, context.DeadlineExceeded):
		return "tailscaled did not answer on " + where + " in time."
	}
	return fmt.Sprintf("tailscaled could not say who this Machine is: %v.", err)
}

func (r *report) bind(ctx context.Context, env doctorEnv, self tailnet.Self, haveSelf bool) {
	r.section("Bind")
	port, err := port(env.getenv)
	switch {
	case err != nil:
		r.problem("ANSHIN_PORT=%q is not a TCP port from 1 to 65535, so the Agent would not start.", env.getenv("ANSHIN_PORT"))
		return
	case env.getenv("ANSHIN_PORT") != "":
		r.ok("Port %d, from ANSHIN_PORT.", port)
	default:
		r.ok("Port %d, the default (ANSHIN_PORT changes it).", port)
	}
	if !haveSelf {
		r.note("Not tried: the Agent binds only the tailnet addresses tailscaled reports.")
		return
	}
	lns, err := env.listen(ctx, self.Addrs, port)
	if err != nil {
		r.bindProblem(ctx, env, self, port, err)
		return
	}
	bound := map[netip.Addr]bool{}
	for _, ln := range lns {
		if ap, err := netip.ParseAddrPort(ln.Addr().String()); err == nil {
			bound[ap.Addr()] = true
		}
		ln.Close()
	}
	for _, a := range self.Addrs {
		if bound[a] {
			r.ok("The Agent can bind %s.", netip.AddrPortFrom(a, uint16(port)))
		} else {
			r.note("IPv6 is disabled on this Machine, so the Agent skips %s and serves on IPv4 alone.", a)
		}
	}
}

func (r *report) bindProblem(ctx context.Context, env doctorEnv, self tailnet.Self, port int, err error) {
	switch {
	case errors.Is(err, syscall.EADDRINUSE):
		for _, a := range self.Addrs {
			if ap := netip.AddrPortFrom(a, uint16(port)); env.agentAt(ctx, ap) {
				r.ok("An Agent already serves on %s, most likely the anshin-agent service.", ap)
				return
			}
		}
		r.problem("Port %d is already in use on this Machine's tailnet address, and what holds it does not answer as an Agent. Stop that program, or choose another port with ANSHIN_PORT.", port)
	case errors.Is(err, fs.ErrPermission):
		r.problem("The Agent may not bind port %d: ports below 1024 need a privilege it never has. Choose an ANSHIN_PORT of 1024 or more.", port)
	default:
		r.problem("The Agent could not bind its tailnet addresses.")
	}
	r.detail("%v", err)
}

// agentAnswers tells the running Agent apart from another program holding its port: only an Agent answers a
// Pulse.
func agentAnswers(ctx context.Context, ap netip.AddrPort) bool {
	ctx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "http://"+ap.String()+"/v1/pulse", nil)
	if err != nil {
		return false
	}
	c := &http.Client{
		Transport:     &http.Transport{},
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
	}
	defer c.CloseIdleConnections()
	resp, err := c.Do(req)
	if err != nil {
		return false
	}
	defer resp.Body.Close()
	var p struct {
		Schema *int `json:"schema"`
	}
	return resp.StatusCode == http.StatusOK && json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&p) == nil && p.Schema != nil
}

func (r *report) machine(env doctorEnv) {
	sensor, err := cpuSensor(env.getenv)
	rec := recorder.Open(env.host, wallInstant(), recorder.Config{CPUSensor: sensor})
	env.settle()
	rec.Sample(wallInstant())
	reading := rec.Read(recorder.Cursor{}).Reading

	r.section("CPU sensor")
	r.cpuSensor(reading, sensor, err)
	r.section("Other sensors (display-only)")
	r.otherSensors(reading)
	r.section("Battery")
	r.battery(reading.Battery)
	r.section("Filesystems")
	r.filesystems(reading)
}

// The doctor samples for one second, too short for a suspend, so the wall clock can stand in for both of the
// Agent's clocks.
func wallInstant() recorder.Instant {
	d := time.Duration(time.Now().UnixNano())
	return recorder.Instant{Monotonic: d, Boottime: d}
}

func (r *report) cpuSensor(reading recorder.Reading, override string, err error) {
	const how = "run `sudo systemctl edit anshin-agent` and add `Environment=\"ANSHIN_CPU_SENSOR=<hwmon name>/<label>\"` under [Service], with a sensor listed below."
	switch {
	case err != nil:
		r.problem("%v, so the Agent would not start.", err)
		return
	case reading.CPUSensor != nil && override != "":
		r.ok("%s, named by ANSHIN_CPU_SENSOR: %s.", *reading.CPUSensor, celsius(reading.CPUTempC))
		return
	case reading.CPUSensor != nil:
		r.ok("%s, chosen automatically: %s.", *reading.CPUSensor, celsius(reading.CPUTempC))
		r.detail("To name another, %s", how)
		return
	case override != "" && !slices.ContainsFunc(reading.Sensors, func(s probe.Sensor) bool { return s.ID == override }):
		r.note("ANSHIN_CPU_SENSOR=%s matches no sensor on this Machine, so the CPU temperature is unavailable: the Agent never falls back to another sensor.", override)
	case override != "":
		r.note("%s, named by ANSHIN_CPU_SENSOR, has no known reading yet. Run doctor again in a moment.", override)
	default:
		r.note("No CPU sensor: no recognised CPU driver reports a temperature, so the CPU temperature is unavailable and the Verdict leaves it out.")
	}
	r.detail("To name one, %s", how)
}

func (r *report) otherSensors(reading recorder.Reading) {
	n := 0
	for _, s := range reading.Sensors {
		if reading.CPUSensor != nil && s.ID == *reading.CPUSensor {
			continue
		}
		r.item("%s: %s", s.ID, celsius(s.TempC))
		n++
	}
	if n == 0 {
		r.item("None.")
	}
	r.detail("Sleeping devices, drives and thermal zones are never read, so they are not listed.")
}

func (r *report) battery(b *probe.BatteryReading) {
	if b == nil {
		r.item("No system battery, so the phone shows none.")
	}
	if b != nil {
		for _, row := range b.Batteries {
			r.item("%s: %s, %s, %s.", row.Name, orUnavailable(row.Pct, "%.0f %%", "charge"), statusText(row.Status), orUnavailable(row.Health, "health %.0f %%", "health"))
		}
		switch {
		case b.Pct == nil && b.Status == nil && b.PowerW == nil && b.TimeLeft == nil:
			r.note("The battery summary is unavailable: a power supply could not be classified, or its charge could not be read.")
		case len(b.Batteries) > 1:
			r.item("All batteries: %s, %s.", orUnavailable(b.Pct, "%.0f %%", "charge"), statusText(b.Status))
		}
	}
	r.detail("Peripheral batteries, such as a mouse's or a headset's, are never read.")
}

func (r *report) filesystems(reading recorder.Reading) {
	if len(reading.Filesystems) == 0 {
		r.note("The Agent sees no filesystem, so the phone shows none.")
		return
	}
	for _, f := range reading.Filesystems {
		primary := ""
		if reading.PrimaryDisk != nil && *reading.PrimaryDisk == f.MajMin {
			primary = "; the primary disk"
		}
		r.item("%s (%s, %s): %s%s.", f.MountPoint, f.Type, f.MajMin, capacity(f), primary)
	}
	if reading.PrimaryDisk == nil {
		r.note("No visible filesystem holds /, so the phone shows no primary disk.")
	}
	r.detail("Pseudo, network and read-only image filesystems are left out, and each device is listed once, at its shortest mount point.")
}

func celsius(v *float64) string { return orUnavailable(v, "%.1f °C", "reading") }

func orUnavailable(v *float64, format, what string) string {
	if v == nil {
		return what + " unavailable"
	}
	return fmt.Sprintf(format, *v)
}

func statusText(s *string) string {
	if s == nil {
		return "status unavailable"
	}
	return *s
}

func capacity(f probe.Filesystem) string {
	if f.Total == nil {
		return "capacity unavailable"
	}
	size := fmt.Sprintf("%d bytes (%.1f GiB)", *f.Total, float64(*f.Total)/(1<<30))
	if f.UsedPct == nil {
		return size + ", use unavailable"
	}
	return fmt.Sprintf("%s, %.1f %% used", size, *f.UsedPct)
}
