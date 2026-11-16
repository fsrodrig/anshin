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
	Machine  string // pulse mode: "minimal" or "full"
	Samples  string // pulse mode: "fixed" or "live"
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
		{"verdict-judged", "-verdict judged", "verdict is what the Agent's own Verdict logic says of the synthetic Machine as a first Sample: null, since it lists no filesystem, memory or CPU sensor, unless a -battery setup gives it a battery with a known percent and status, which makes it ok with no Reasons; -machine full is ok with no Reasons too (default)"},
		{"verdict-null", "-verdict null", "verdict is null, as when no input could be judged: the phone shows the Verdict unavailable, never ok"},
		{"verdict-ok", "-verdict ok", "verdict is ok with no Reasons"},
		{"verdict-warn", "-verdict warn", "verdict is warn with one warn Reason, filesystem_full \"/ is 91.2 % full\" (the synthetic Machine lists no filesystem)"},
		{"verdict-crit", "-verdict crit", "verdict is crit with two Reasons, worst first: cpu_hot crit \"CPU at 97.0 °C (crit at 95.0 °C)\", then filesystem_full warn \"/ is 91.2 % full\""},
		{"window-full", "-window full", "a 30-minute Window of 900 Samples 2 s apart whose CPU follows a gentle wave ending at the current cpu_pct (default); memory and the rest are null on -machine minimal, and drift gently on -machine full"},
		{"window-gap", "-window gap", "the same with a 5-minute Gap ending 10 minutes before the newest Sample: the phone shows a hatched Gap · 5 min"},
		{"machine-minimal", "-machine minimal", "the synthetic Machine reports only hostname, CPU and the -battery setup; every other field is null (default)"},
		{"machine-full", "-machine full", "every detail section filled with synthetic values: distro QA Linux, kernel qa-kernel, x86_64, agent qa-build, uptime 86520 s; 16 GiB memory with 6 GiB available; swap /dev/zram0 (zram, 8 GiB, 1 GiB used) and /swapfile (4 GiB, 0 used); load 1.25 0.98 0.75; 8 CPUs with CPU 3 null; sensors k10temp/Tctl 61.5 °C (the CPU sensor), k10temp/Tccd1 58.0 °C, amdgpu/edge 47.0 °C aged 12 s, it87/temp3 null; filesystems / btrfs (primary), /boot vfat, /mnt/archive xfs with null capacity aged 90 s; interfaces lo and tailscale0 (no device), eth0 and wlan0 (device); disks nvme0n1 and sda; network and disk summaries are the sums over the device-backed rows. The Window carries every series and 8 per-CPU columns, and -verdict judged gives ok"},
		{"samples-fixed", "-samples fixed", "the QA Agent never takes a new Sample; every poll after the first gets an empty delta (default)"},
		{"samples-live", "-samples live", "a new Sample every 2 s by the QA Agent's own clock: the Window slides, the Cursor advances and the values change each Sample; needs -window full"},
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
	machine := fs.String("machine", "minimal", "pulse mode: minimal | full")
	samples := fs.String("samples", "fixed", "pulse mode: fixed | live")
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
	c := AgentConfig{Mode: *mode, WhoIs: *whois, CPU: *cpu, Hostname: *hostname, Battery: *battery, Verdict: *verdict, Window: *window, Machine: *machine, Samples: *samples, Status: *status, Delay: *delay}
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
		if err := oneOf("machine", c.Machine, "minimal", "full"); err != nil {
			return c, false, err
		}
		if err := oneOf("samples", c.Samples, "fixed", "live"); err != nil {
			return c, false, err
		}
		// Sliding a Gap out of the Window needs the production eviction rules; the QA tooling doesn't copy them.
		if c.Samples == "live" && c.Window == "gap" {
			return c, false, errors.New("-samples live needs -window full")
		}
	case "raw":
		for _, name := range []string{"whois", "cpu", "hostname", "battery", "verdict", "window", "machine", "samples"} {
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

// syntheticNewestMs is the newest fixed Sample's CLOCK_BOOTTIME: an hour after boot, so a full Window fits after it.
const syntheticNewestMs = 3_600_000

const (
	sampleStepMs   = 2_000
	windowSpanMs   = 1_800_000
	windowRows     = windowSpanMs / sampleStepMs
	syntheticCores = 8
)

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

// syntheticWindow is 30 minutes of Samples 2 s apart ending at newest, each row the Sample sampleAt gives for its
// time, so the last row is the flat fields, as the production recorder guarantees.
func syntheticWindow(c AgentConfig, newest int64) *recorder.Window {
	gapFrom, gapTo := int64(-1), int64(-1)
	if c.Window == "gap" {
		gapTo = newest - 600_000
		gapFrom = gapTo - 300_000
	}
	w := &recorder.Window{SpanS: windowSpanMs / 1000}
	if c.Machine == "full" {
		w.CPUCorePct = make([][]*uint8, syntheticCores)
	}
	add := func(t *int64, r recorder.Reading) {
		w.BoottimeMs = append(w.BoottimeMs, t)
		w.CPUPct, w.Load1, w.MemUsedPct = append(w.CPUPct, r.CPUPct), append(w.Load1, r.LoadReading.One), append(w.MemUsedPct, r.MemoryReading.Pct)
		w.SwapUsedBytes, w.CPUTempC = append(w.SwapUsedBytes, r.SwapReading.Used), append(w.CPUTempC, r.CPUTempC)
		w.NetRx, w.NetTx = append(w.NetRx, r.NetRx), append(w.NetTx, r.NetTx)
		w.DiskRead, w.DiskWrite = append(w.DiskRead, r.DiskRead), append(w.DiskWrite, r.DiskWrite)
		var pct, power *float64
		if r.Battery != nil {
			pct, power = r.Battery.Pct, r.Battery.PowerW
		}
		w.BatteryPct, w.BatteryPowerW = append(w.BatteryPct, pct), append(w.BatteryPowerW, power)
		for i := range w.CPUCorePct {
			var v *uint8
			if r.CPUCorePct != nil {
				v = r.CPUCorePct[i]
			}
			w.CPUCorePct[i] = append(w.CPUCorePct[i], v)
		}
	}
	for t := newest - windowSpanMs + sampleStepMs; t <= newest; t += sampleStepMs {
		if t > gapFrom && t < gapTo {
			continue
		}
		r := sampleAt(c, t)
		if t == gapTo {
			add(nil, recorder.Reading{})
			// The first Sample after a Gap has fresh baselines, so no rates, as in the production recorder.
			r.CPUPct, r.NetRx, r.NetTx, r.DiskRead, r.DiskWrite = nil, nil, nil, nil, nil
			if r.CPUCorePct != nil {
				r.CPUCorePct = make([]*uint8, len(r.CPUCorePct))
			}
		}
		add(&t, r)
	}
	return w
}

// wave is a slow oscillation that is 0 at syntheticNewestMs when phase is 0, so the fixed Sample keeps the
// recognizable base values while the Window and live Samples still move.
func wave(t int64, periodMs, phase float64) float64 {
	return math.Sin(2*math.Pi*float64(t-syntheticNewestMs)/periodMs + phase)
}

func roundTo(v float64, places int) float64 {
	p := math.Pow(10, float64(places))
	return math.Round(v*p) / p
}

// flatReading is the Pulse's flat fields at newest: the Sample there plus its Verdict.
func flatReading(c AgentConfig, newest int64) recorder.Reading {
	r := sampleAt(c, newest)
	r.Verdict = syntheticVerdict(c.Verdict, r)
	return r
}

// sampleAt is the synthetic Machine's Sample at boottime t, without a Verdict. It is a pure function of t, so a live
// Window can be rebuilt from any row's time.
func sampleAt(c AgentConfig, t int64) recorder.Reading {
	var r recorder.Reading
	if c.Hostname == "synthetic" {
		h := SyntheticHostname
		r.Hostname = &h
	}
	switch c.CPU {
	case "ordinary":
		v := max(0, min(100, roundTo(syntheticCPU+5*wave(t, 600_000, 0), 1)))
		r.CPUPct = &v
	case "zero":
		r.CPUPct = new(0.0)
	}
	r.Battery = syntheticBattery(c.Battery)
	if c.Machine == "full" {
		fullMachine(&r, t)
	}
	return r
}

const gib = 1 << 30

// fullMachine fills every section with fixed, recognizable values that drift gently with t. Summaries are
// computed from the rows exactly as the recorder computes them, so they always agree.
func fullMachine(r *recorder.Reading, t int64) {
	r.MachineInfo = probe.MachineInfo{Distro: new("QA Linux"), Kernel: new("qa-kernel"), Architecture: new("x86_64"), AgentVersion: new("qa-build")}
	r.LoadReading = probe.LoadReading{
		One: new(roundTo(1.25+0.25*wave(t, 900_000, 0), 2)), Five: new(0.98), Fifteen: new(0.75),
		Uptime: new(86_520 + float64(t-syntheticNewestMs)/1000),
	}

	total, available := uint64(16*gib), uint64(6*gib+int64(math.Round(512*wave(t, 900_000, 0)))*(1<<20))
	used := total - available
	r.MemoryReading = probe.MemoryReading{Total: &total, Available: &available, Used: &used, Pct: new(math.Round(1000*float64(used)/float64(total)) / 10)}
	swaps := []probe.SwapEntry{
		{Name: "/dev/zram0", Zram: true, Total: new(uint64(8 * gib)), Used: new(uint64(gib))},
		{Name: "/swapfile", Total: new(uint64(4 * gib)), Used: new(uint64(0))},
	}
	r.SwapReading = probe.SwapReading{Total: new(uint64(0)), Used: new(uint64(0)), Entries: swaps}
	for _, s := range swaps {
		*r.SwapReading.Total += *s.Total
		*r.SwapReading.Used += *s.Used
	}

	if r.CPUPct != nil {
		offsets := [syntheticCores]float64{-10, 15, 5, 0, -5, 20, -15, 10}
		r.CPUCorePct = make([]*uint8, syntheticCores)
		for i, o := range offsets {
			if i == 3 {
				continue
			}
			v := uint8(0)
			if *r.CPUPct != 0 {
				v = uint8(max(0, min(100, math.Round(*r.CPUPct+o+8*wave(t, 300_000, float64(i))))))
			}
			r.CPUCorePct[i] = &v
		}
	} else {
		r.CPUCorePct = make([]*uint8, syntheticCores)
	}

	tctl := roundTo(61.5+2*wave(t, 900_000, 0), 1)
	r.TemperatureReading = probe.TemperatureReading{
		CPUTempC: &tctl, CPUSensor: new("k10temp/Tctl"),
		Sensors: []probe.Sensor{
			{ID: "k10temp/Tctl", TempC: &tctl, Age: new(int64(0))},
			{ID: "k10temp/Tccd1", TempC: new(roundTo(58+1.5*wave(t, 900_000, 0), 1)), Age: new(int64(0))},
			{ID: "amdgpu/edge", TempC: new(47.0), Age: new(int64(12))},
			{ID: "it87/temp3"},
		},
	}

	r.Filesystems = []probe.Filesystem{
		filesystem("259:2", "/", "btrfs", 512*gib, 128*gib, 380*gib),
		filesystem("259:1", "/boot", "vfat", gib, 256<<20, 768<<20),
		{MajMin: "8:1", MountPoint: "/mnt/archive", Type: "xfs", Age: 90},
	}
	r.PrimaryDisk = new("259:2")

	rate := func(base float64, phase float64) *uint64 {
		return new(uint64(math.Round(base * (1 + 0.5*wave(t, 120_000, phase)))))
	}
	r.NetInterfaces = []recorder.NetInterface{
		{Name: "lo", DeviceBacked: new(false), Rx: rate(2_048, 0), Tx: rate(2_048, 0)},
		{Name: "eth0", DeviceBacked: new(true), Rx: rate(75_264, 0), Tx: rate(23_245, 1)},
		{Name: "wlan0", DeviceBacked: new(true), Rx: rate(4_096, 2), Tx: rate(1_024, 3)},
		{Name: "tailscale0", DeviceBacked: new(false), Rx: rate(30_720, 1), Tx: rate(10_240, 2)},
	}
	r.NetRx, r.NetTx = new(uint64(0)), new(uint64(0))
	for _, n := range r.NetInterfaces {
		if *n.DeviceBacked {
			*r.NetRx += *n.Rx
			*r.NetTx += *n.Tx
		}
	}
	r.Disks = []recorder.Disk{
		{Name: "nvme0n1", Read: rate(86_016, 0), Write: rate(212_992, 2)},
		{Name: "sda", Read: new(uint64(0)), Write: rate(4_096, 1)},
	}
	r.DiskRead, r.DiskWrite = new(uint64(0)), new(uint64(0))
	for _, d := range r.Disks {
		*r.DiskRead += *d.Read
		*r.DiskWrite += *d.Write
	}
}

// filesystem computes used_pct as probe.Capacity does.
func filesystem(majMin, mount, fsType string, total, used, available uint64) probe.Filesystem {
	return probe.Filesystem{MajMin: majMin, MountPoint: mount, Type: fsType, Total: &total, Used: &used, Available: &available,
		UsedPct: new(math.Round(1000*float64(used)/float64(used+available)) / 10)}
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

func recorderFor(c AgentConfig, now func() time.Time, after func(time.Duration) <-chan time.Time) server.Recorder {
	if c.Samples == "live" {
		return newLiveRecorder(c, now, after)
	}
	return syntheticRecorder{flatReading(c, syntheticNewestMs), syntheticWindow(c, syntheticNewestMs)}
}

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
		h = server.New(tn, self, recorderFor(c, time.Now, time.After))
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
