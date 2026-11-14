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
	Hostname      *string        `json:"hostname"`
	CPUPct        *float64       `json:"cpu_pct"`
	NetRx         *uint64        `json:"net_rx_bps"`
	NetTx         *uint64        `json:"net_tx_bps"`
	NetInterfaces []NetInterface `json:"net_interfaces"`
	Disks         []Disk         `json:"disks"`
}

// NetInterface is one interface's rates in bytes per second. DeviceBacked is nil when it couldn't be determined.
type NetInterface struct {
	Name         string  `json:"name"`
	DeviceBacked *bool   `json:"device_backed"`
	Rx           *uint64 `json:"rx_bps"`
	Tx           *uint64 `json:"tx_bps"`
}

// Disk is one device-backed disk's rates in bytes per second.
type Disk struct {
	Name  string  `json:"name"`
	Read  *uint64 `json:"read_bps"`
	Write *uint64 `json:"write_bps"`
}

type Recorder struct {
	host   probe.Host
	now    func() time.Time
	info   probe.MachineInfo
	infoAt time.Time

	mu   sync.Mutex
	base *probe.CPUTimes
	pct  *float64
	net  map[string]*ioBaseline
	disk map[string]*ioBaseline
}

type ioBaseline struct {
	at                    time.Time
	first, second         *uint64
	lastFirst, lastSecond *uint64
	link                  *bool
	dFirst, dSecond       *uint64
	dt                    float64
}

func (b *ioBaseline) advance(at time.Time, first, second *uint64) {
	dt := at.Sub(b.at)
	resetFirst := delta(b.lastFirst, first) == nil
	resetSecond := delta(b.lastSecond, second) == nil
	b.lastFirst, b.lastSecond = first, second
	if dt < time.Second && !resetFirst && !resetSecond {
		return
	}
	b.dFirst, b.dSecond = delta(b.first, first), delta(b.second, second)
	if resetFirst {
		b.dFirst = nil
	}
	if resetSecond {
		b.dSecond = nil
	}
	if dt < time.Second {
		// A reset cannot wait for the minimum interval: the next delta must start in this device lifetime.
		b.dFirst, b.dSecond = nil, nil
	}
	b.first, b.second, b.at, b.dt = first, second, at, dt.Seconds()
}

func newIOBaseline(at time.Time, first, second *uint64, link *bool) *ioBaseline {
	return &ioBaseline{at: at, first: first, second: second, lastFirst: first, lastSecond: second, link: link}
}

type netRates struct {
	rx, tx     *uint64
	interfaces []NetInterface
}

// New reads the Machine through h and takes the time from now, which in production is time.Now so elapsed time
// uses the monotonic clock. It takes CPU, network and disk baselines immediately so the first Read a second later
// already has rates.
func New(h probe.Host, now func() time.Time) *Recorder {
	r := &Recorder{host: h, now: now}
	if t, err := probe.CPU(h); err == nil {
		r.base = &t
	}
	at := now()
	r.netRates(at)
	r.diskRates(at)
	return r
}

// Read refreshes measurements and caches Machine info for 30 seconds. The CPU baseline is only replaced once at
// least a second of CPU time has passed, so two callers polling close together don't get percentages computed
// from a handful of jiffies.
func (r *Recorder) Read() Reading {
	r.mu.Lock()
	defer r.mu.Unlock()
	now := r.now()
	if r.infoAt.IsZero() || now.Sub(r.infoAt) >= 30*time.Second {
		r.info, r.infoAt = probe.Info(r.host), now
	}
	net := r.netRates(now)
	return Reading{
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
	next := make(map[string]*ioBaseline, len(counters))
	rx, tx := new(uint64(0)), new(uint64(0))
	var rxRate, txRate float64
	counted := false
	for _, c := range counters {
		p, seen := r.net[c.Name]
		countedBefore := seen && (p.link == nil || *p.link)
		if !seen || !equal(p.link, c.DeviceBacked) {
			p = newIOBaseline(now, c.Rx, c.Tx, c.DeviceBacked)
		} else {
			p.advance(now, c.Rx, c.Tx)
		}
		next[c.Name] = p
		out.interfaces = append(out.interfaces, NetInterface{Name: c.Name, DeviceBacked: c.DeviceBacked,
			Rx: perSecond(p.dFirst, 1, p.dt), Tx: perSecond(p.dSecond, 1, p.dt)})
		countedNow := c.DeviceBacked == nil || *c.DeviceBacked
		if !countedNow && !countedBefore {
			continue
		}
		counted = counted || countedNow
		dRx, dTx := p.dFirst, p.dSecond
		if c.DeviceBacked == nil {
			dRx, dTx = nil, nil
		}
		sum(&rx, dRx)
		sum(&tx, dTx)
		if dRx != nil {
			rxRate += float64(*dRx) / p.dt
		}
		if dTx != nil {
			txRate += float64(*dTx) / p.dt
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
	next := make(map[string]*ioBaseline, len(counters))
	var out []Disk
	for _, c := range counters {
		p, seen := r.disk[c.Name]
		if !seen {
			p = newIOBaseline(now, c.ReadSectors, c.WriteSectors, nil)
		} else {
			p.advance(now, c.ReadSectors, c.WriteSectors)
		}
		next[c.Name] = p
		// Sectors in diskstats are always 512 bytes, whatever the disk's logical sector size.
		out = append(out, Disk{Name: c.Name, Read: perSecond(p.dFirst, 512, p.dt), Write: perSecond(p.dSecond, 512, p.dt)})
	}
	r.disk = next
	return out
}
