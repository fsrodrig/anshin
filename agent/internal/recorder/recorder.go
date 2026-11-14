// Package recorder measures the Machine and owns the fast readings and cached slow-lane Machine info.
package recorder

import (
	"math"
	"sync"
	"time"

	"github.com/fsrodrig/anshin/agent/internal/recorder/probe"
)

// The kernel reports /proc/stat in USER_HZ, which is 100 on every architecture regardless of CONFIG_HZ.
const userHZ = 100

// Reading is what the recorder knows about the Machine right now. A nil field is unavailable.
type Reading struct {
	probe.MemoryReading
	probe.SwapReading
	probe.LoadReading
	probe.MachineInfo
	Filesystems   []probe.Filesystem `json:"filesystems"`
	PrimaryDisk   *string            `json:"primary_disk"`
	Hostname      *string            `json:"hostname"`
	CPUPct        *float64           `json:"cpu_pct"`
	NetRx         *uint64            `json:"net_rx_bps"`
	NetTx         *uint64            `json:"net_tx_bps"`
	NetInterfaces []NetInterface     `json:"net_interfaces"`
	Disks         []Disk             `json:"disks"`
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
	host   probe.Host
	info   probe.MachineInfo
	infoAt time.Time

	disks     map[string]*filesystemState
	busyPaths map[string]bool
	diskAt    time.Time
	mu        sync.Mutex
	base      *probe.CPUTimes
	pct       *float64
	net       map[string]*netBaseline
	disk      map[string]*diskBaseline
}

type netBaseline struct {
	rx, tx counterBaseline
	link   *bool
}

type diskBaseline struct {
	read, write counterBaseline
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

// New takes baselines immediately so the first read a second later already has rates.
// Callers supply monotonic times so wall-clock corrections cannot distort the interval.
func New(h probe.Host, at time.Time) *Recorder {
	r := &Recorder{host: h}
	if t, err := probe.CPU(h); err == nil {
		r.base = &t
	}
	r.netRates(at)
	r.diskRates(at)
	return r
}

func (r *Recorder) Read() Reading {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.readLocked(time.Now())
}

// ReadAt keeps the CPU baseline for at least a second of CPU time so nearby polls do not
// compute percentages from a handful of jiffies. I/O baselines use the supplied monotonic time.
func (r *Recorder) ReadAt(now time.Time) Reading {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.readLocked(now)
}

func (r *Recorder) readLocked(now time.Time) Reading {
	if r.infoAt.IsZero() || now.Sub(r.infoAt) >= 30*time.Second {
		r.info, r.infoAt = probe.Info(r.host), now
	}
	filesystems, primary := r.filesystems(now)
	net := r.netRates(now)
	return Reading{
		Filesystems: filesystems, PrimaryDisk: primary,
		Hostname: r.hostname(), CPUPct: r.cpuPct(), MemoryReading: probe.Memory(r.host), SwapReading: probe.Swap(r.host),
		LoadReading: probe.Load(r.host), MachineInfo: r.info,
		NetRx: net.rx, NetTx: net.tx, NetInterfaces: net.interfaces, Disks: r.diskRates(now),
	}
}

func (r *Recorder) hostname() *string {
	h, err := probe.Hostname(r.host)
	if err != nil {
		return nil
	}
	return &h
}

func (r *Recorder) cpuPct() *float64 {
	now, err := probe.CPU(r.host)
	if err != nil {
		r.base, r.pct = nil, nil
		return nil
	}
	if r.base == nil || now.CPUs != r.base.CPUs || wentBackwards(now, *r.base) {
		// A first reading, a CPU coming or going, or a counter running backwards: no honest delta exists, so
		// this interval is unavailable and the next one starts from here.
		r.base, r.pct = &now, nil
		return nil
	}
	dTotal := now.Total() - r.base.Total()
	if dTotal < uint64(now.CPUs)*userHZ {
		return r.pct
	}
	dIdle := now.Idle() - r.base.Idle()
	pct := math.Round(1000*float64(dTotal-min(dIdle, dTotal))/float64(dTotal)) / 10
	r.base, r.pct = &now, &pct
	return r.pct
}

// wentBackwards compares each busy counter on its own, so growth elsewhere can't hide a reset in the sums. idle and
// iowait are compared as one counter: the kernel documents that iowait can decrease when time it counted is
// reclassified as idle, and only their sum is used.
func wentBackwards(now, base probe.CPUTimes) bool {
	for i, v := range now.Jiffies {
		if i != 3 && i != 4 && v < base.Jiffies[i] {
			return true
		}
	}
	return now.Idle() < base.Idle()
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

func (r *Recorder) diskRates(now time.Time) []Disk {
	counters, err := probe.Disks(r.host)
	if err != nil {
		r.disk = nil
		return nil
	}
	next := make(map[string]*diskBaseline, len(counters))
	var out []Disk
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
		out = append(out, Disk{Name: c.Name, Read: perSecond(p.read.delta, 512, p.read.dt), Write: perSecond(p.write.delta, 512, p.write.dt)})
	}
	r.disk = next
	return out
}
