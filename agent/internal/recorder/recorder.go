// Package recorder measures the Machine and owns the fast readings and cached slow-lane Machine info.
package recorder

import (
	"math"
	"slices"
	"sync"
	"sync/atomic"
	"time"

	"github.com/fsrodrig/anshin/agent/internal/recorder/probe"
)

// The kernel reports /proc/stat in USER_HZ, which is 100 on every architecture regardless of CONFIG_HZ.
const userHZ = 100

type Config struct {
	// CPUSensor is ANSHIN_CPU_SENSOR, a sensor ID such as "coretemp/Package id 0". Empty chooses the CPU sensor
	// as docs/design.md § Temperatures says.
	CPUSensor string
	// Epoch names this Agent run in every Cursor. cmd/anshin-agent draws it at random, so a restart never reuses
	// one, even across reboots; replays fix it so fixtures stay the same.
	Epoch uint64
}

// Reading is what the recorder knows about the Machine right now. A nil field is unavailable.
type Reading struct {
	probe.MemoryReading
	probe.SwapReading
	probe.LoadReading
	probe.MachineInfo
	probe.TemperatureReading
	Filesystems   []probe.Filesystem    `json:"filesystems"`
	PrimaryDisk   *string               `json:"primary_disk"`
	Hostname      *string               `json:"hostname"`
	CPUPct        *float64              `json:"cpu_pct"`
	CPUCorePct    []*uint8              `json:"cpu_core_pct"`
	NetRx         *uint64               `json:"net_rx_bps"`
	NetTx         *uint64               `json:"net_tx_bps"`
	NetInterfaces []NetInterface        `json:"net_interfaces"`
	Disks         []Disk                `json:"disks"`
	DiskRead      *uint64               `json:"disk_read_bps"`
	DiskWrite     *uint64               `json:"disk_write_bps"`
	Battery       *probe.BatteryReading `json:"battery"`
	Verdict       *Verdict              `json:"verdict"`
}

// NetInterface is one interface's rates in bytes per second. DeviceBacked is nil when it couldn't be determined.
type NetInterface struct {
	Name         string  `json:"name"`
	DeviceBacked *bool   `json:"device_backed"`
	Rx           *uint64 `json:"rx_bps"`
	Tx           *uint64 `json:"tx_bps"`
}

type Disk struct {
	Name  string  `json:"name"`
	Read  *uint64 `json:"read_bps"`
	Write *uint64 `json:"write_bps"`
}

type Recorder struct {
	host         probe.Host
	cfg          Config
	temperatures temperatureSampler
	info         probe.MachineInfo
	infoAt       time.Time
	verdict      judge

	disks     map[string]*filesystemState
	busyPaths map[string]bool
	diskAt    time.Time
	diskTable []byte
	mu        sync.Mutex
	base      *probe.CPUTimes
	pct       *float64
	net       map[string]*netBaseline
	disk      map[string]*diskBaseline
	// coreIDs are the columns of cpu_core_pct, fixed at the first Sample that lists CPUs so the Window's width
	// never changes in a run.
	coreIDs []int
	corePct []*uint8
	// offset is CLOCK_BOOTTIME − CLOCK_MONOTONIC at the last Sample. It grows only while the Machine is suspended.
	offset time.Duration
	// probing counts filesystem capacity workers still running, for replays that must not race them.
	probing atomic.Int64

	windowMu sync.Mutex
	ring     ring
	newest   Reading
	newer    chan struct{}
}

type netBaseline struct {
	rx, tx counterBaseline
	link   *bool
}

type diskBaseline struct {
	read, write counterBaseline
}

type diskRates struct {
	disks       []Disk
	read, write *uint64
}

type counterBaseline struct {
	at       time.Time
	baseline *uint64
	latest   *uint64
	delta    *uint64
	dt       float64
}

func (b *counterBaseline) advance(at time.Time, value *uint64) {
	dt := at.Sub(b.at)
	reset := delta(b.latest, value) == nil
	b.latest = value
	if dt < time.Second && !reset {
		return
	}
	b.delta = delta(b.baseline, value)
	if reset {
		// A reset cannot wait for the minimum interval: the next delta must start in this counter lifetime.
		b.delta = nil
	}
	b.baseline, b.at, b.dt = value, at, dt.Seconds()
}

func newCounterBaseline(at time.Time, value *uint64) counterBaseline {
	return counterBaseline{at: at, baseline: value, latest: value}
}

type netRates struct {
	rx, tx     *uint64
	interfaces []NetInterface
}

// Open records Sample 1 at once, so a Window always has a Sample. Its rates are null; it is every counter's
// baseline, and the next Sample has rates.
func Open(h probe.Host, at Instant, cfg Config) *Recorder {
	r := &Recorder{host: h, cfg: cfg, ring: newRing(), newer: make(chan struct{}), offset: at.Boottime - at.Monotonic}
	r.Sample(at)
	return r
}

// Sample measures the Machine once and appends it to the Window. The sampling loop in cmd/anshin-agent calls it
// every SampleInterval; nothing else measures, so a Pulse never waits on a probe.
func (r *Recorder) Sample(at Instant) { r.sample(at) }

func (r *Recorder) sample(at Instant) Reading {
	now := at.monotonic()
	temperatures := r.temperatures.Read(r.host, r.cfg.CPUSensor, now)
	r.mu.Lock()
	defer r.mu.Unlock()
	offset := at.Boottime - at.Monotonic
	suspended := offset-r.offset >= SuspendThreshold
	r.offset = offset
	if suspended {
		// Counters kept running through the suspend while the monotonic clock stood still, so a delta across it
		// would be a spike. Every rate starts over from this Sample. The Gap row's temperature is null, and a null
		// temperature resets the debounce.
		r.base, r.pct, r.corePct, r.net, r.disk = nil, nil, nil, nil, nil
		r.verdict = judge{}
	}
	v := r.readLocked(now)
	v.TemperatureReading = temperatures
	v.Verdict = r.verdict.observe(v, now)
	r.publish(at, v, suspended)
	return v
}

// Idle reports that no filesystem capacity call launched by a Sample is still running. Replays wait on it so a
// fixture never depends on goroutine scheduling.
func (r *Recorder) Idle() bool { return r.probing.Load() == 0 }

func (r *Recorder) readLocked(now time.Time) Reading {
	if r.infoAt.IsZero() || now.Sub(r.infoAt) >= 30*time.Second {
		r.info, r.infoAt = probe.Info(r.host), now
	}
	filesystems, primary := r.filesystems(now)
	net := r.netRates(now)
	disks := r.diskRates(now)
	cpu := r.cpuPct()
	return Reading{
		Filesystems: filesystems, PrimaryDisk: primary,
		Hostname: r.hostname(), CPUPct: cpu, CPUCorePct: slices.Clone(r.corePct),
		MemoryReading: probe.Memory(r.host), SwapReading: probe.Swap(r.host),
		LoadReading: probe.Load(r.host), MachineInfo: r.info,
		NetRx: net.rx, NetTx: net.tx, NetInterfaces: net.interfaces,
		Disks: disks.disks, DiskRead: disks.read, DiskWrite: disks.write,
		Battery: probe.Battery(r.host),
	}
}

func (r *Recorder) hostname() *string {
	h, err := probe.Hostname(r.host)
	if err != nil {
		return nil
	}
	return &h
}

// cpuPct also sets corePct, measured over the same interval, so a core's bar never disagrees with the total.
func (r *Recorder) cpuPct() *float64 {
	now, err := probe.CPU(r.host)
	if err != nil {
		r.base, r.pct = nil, nil
		r.corePct = r.nullCores()
		return nil
	}
	if r.coreIDs == nil && len(now.Cores) > 0 {
		for _, c := range now.Cores {
			r.coreIDs = append(r.coreIDs, c.ID)
		}
	}
	if r.base == nil || now.CPUs != r.base.CPUs || wentBackwards(now.Jiffies, r.base.Jiffies) {
		// A first reading, a CPU coming or going, or a counter running backwards: no honest delta exists, so
		// this interval is unavailable and the next one starts from here.
		r.base, r.pct = &now, nil
		r.corePct = r.nullCores()
		return nil
	}
	dTotal := now.Total() - r.base.Total()
	if dTotal < uint64(now.CPUs)*userHZ {
		return r.pct
	}
	dIdle := now.Idle() - r.base.Idle()
	pct := math.Round(1000*float64(dTotal-min(dIdle, dTotal))/float64(dTotal)) / 10
	r.corePct = r.cores(now.Cores, r.base.Cores)
	r.base, r.pct = &now, &pct
	return r.pct
}

func (r *Recorder) nullCores() []*uint8 {
	if r.coreIDs == nil {
		return nil
	}
	return make([]*uint8, len(r.coreIDs))
}

// cores is null for a core missing at either end, or whose own counters went backwards or didn't advance: a core
// taken offline and back has no honest delta.
func (r *Recorder) cores(now, base []probe.CoreTimes) []*uint8 {
	out := r.nullCores()
	for i, id := range r.coreIDs {
		a, ok := findCore(base, id)
		b, ok2 := findCore(now, id)
		if !ok || !ok2 || wentBackwards(b.Jiffies, a.Jiffies) || b.Total() <= a.Total() {
			continue
		}
		dTotal, dIdle := b.Total()-a.Total(), b.Idle()-a.Idle()
		pct := uint8(math.Round(100 * float64(dTotal-min(dIdle, dTotal)) / float64(dTotal)))
		out[i] = &pct
	}
	return out
}

func findCore(cores []probe.CoreTimes, id int) (probe.CoreTimes, bool) {
	for _, c := range cores {
		if c.ID == id {
			return c, true
		}
	}
	return probe.CoreTimes{}, false
}

// wentBackwards compares each busy counter on its own, so growth elsewhere can't hide a reset in the sums. idle and
// iowait are compared as one counter: the kernel documents that iowait can decrease when time it counted is
// reclassified as idle, and only their sum is used.
func wentBackwards(now, base [8]uint64) bool {
	for i, v := range now {
		if i != 3 && i != 4 && v < base[i] {
			return true
		}
	}
	return now[3]+now[4] < base[3]+base[4]
}

// delta is now − old. It is nil when either counter is unavailable or the counter went backwards, which is a
// reset: no honest delta exists.
func delta(old, now *uint64) *uint64 {
	if old == nil || now == nil || *now < *old {
		return nil
	}
	d := *now - *old
	return &d
}

func perSecond(d *uint64, scale, dt float64) *uint64 {
	if d == nil {
		return nil
	}
	return roundedRate(float64(*d) * scale / dt)
}

func roundedRate(rate float64) *uint64 {
	v := math.Round(rate)
	// MaxInt64 rounds up to 2^63 as a float, so equality is already outside the phone's range.
	if math.IsNaN(v) || math.IsInf(v, 0) || v < 0 || v >= float64(math.MaxInt64) {
		return nil
	}
	n := uint64(v)
	return &n
}

func sum(total **uint64, d *uint64) {
	if *total == nil || d == nil || **total > math.MaxInt64 || *d > math.MaxInt64-**total {
		*total = nil
		return
	}
	**total += *d
}

func (r *Recorder) netRates(now time.Time) netRates {
	counters, err := probe.Network(r.host)
	if err != nil {
		r.net = nil
		return netRates{}
	}
	var out netRates
	next := make(map[string]*netBaseline, len(counters))
	rx, tx := new(uint64(0)), new(uint64(0))
	var rxRate, txRate float64
	counted := false
	for _, c := range counters {
		p, seen := r.net[c.Name]
		countedBefore := seen && (p.link == nil || *p.link)
		if !seen || !equal(p.link, c.DeviceBacked) {
			p = &netBaseline{rx: newCounterBaseline(now, c.Rx), tx: newCounterBaseline(now, c.Tx), link: c.DeviceBacked}
		} else {
			p.rx.advance(now, c.Rx)
			p.tx.advance(now, c.Tx)
		}
		next[c.Name] = p
		out.interfaces = append(out.interfaces, NetInterface{Name: c.Name, DeviceBacked: c.DeviceBacked,
			Rx: perSecond(p.rx.delta, 1, p.rx.dt), Tx: perSecond(p.tx.delta, 1, p.tx.dt)})
		countedNow := c.DeviceBacked == nil || *c.DeviceBacked
		if !countedNow && !countedBefore {
			continue
		}
		counted = counted || countedNow
		dRx, dTx := p.rx.delta, p.tx.delta
		if c.DeviceBacked == nil {
			dRx, dTx = nil, nil
		}
		sum(&rx, dRx)
		sum(&tx, dTx)
		if dRx != nil {
			rxRate += float64(*dRx) / p.rx.dt
		}
		if dTx != nil {
			txRate += float64(*dTx) / p.tx.dt
		}
	}
	for name, p := range r.net {
		if _, exists := next[name]; !exists && (p.link == nil || *p.link) {
			rx, tx = nil, nil
		}
	}
	r.net = next
	if counted {
		// Add before rounding so small rates across interfaces are not lost.
		if rx != nil {
			out.rx = roundedRate(rxRate)
		}
		if tx != nil {
			out.tx = roundedRate(txRate)
		}
	}
	return out
}

func equal(a, b *bool) bool { return a == nil && b == nil || a != nil && b != nil && *a == *b }

// diskRates sums the disks the way netRates sums interfaces: before rounding, and null when any disk's rate is
// null or a disk came or went, because a sum over a changing set is no one interval's I/O.
func (r *Recorder) diskRates(now time.Time) diskRates {
	counters, err := probe.Disks(r.host)
	if err != nil {
		r.disk = nil
		return diskRates{}
	}
	next := make(map[string]*diskBaseline, len(counters))
	var out []Disk
	readOK, writeOK := len(counters) > 0, len(counters) > 0
	var readRate, writeRate float64
	for _, c := range counters {
		p, seen := r.disk[c.Name]
		if !seen {
			p = &diskBaseline{read: newCounterBaseline(now, c.ReadSectors), write: newCounterBaseline(now, c.WriteSectors)}
		} else {
			p.read.advance(now, c.ReadSectors)
			p.write.advance(now, c.WriteSectors)
		}
		next[c.Name] = p
		// Sectors in diskstats are always 512 bytes, whatever the disk's logical sector size.
		d := Disk{Name: c.Name, Read: perSecond(p.read.delta, 512, p.read.dt), Write: perSecond(p.write.delta, 512, p.write.dt)}
		out = append(out, d)
		readOK, writeOK = readOK && d.Read != nil, writeOK && d.Write != nil
		if d.Read != nil {
			readRate += float64(*p.read.delta) * 512 / p.read.dt
		}
		if d.Write != nil {
			writeRate += float64(*p.write.delta) * 512 / p.write.dt
		}
	}
	for name := range r.disk {
		if _, exists := next[name]; !exists {
			readOK, writeOK = false, false
		}
	}
	r.disk = next
	rates := diskRates{disks: out}
	if readOK {
		rates.read = roundedRate(readRate)
	}
	if writeOK {
		rates.write = roundedRate(writeRate)
	}
	return rates
}
