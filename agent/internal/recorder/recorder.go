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
	net  baseline[[]probe.NetCounters, netRates]
	disk baseline[[]probe.DiskCounters, []Disk]
}

// baseline is a counter reading and when it was taken, with the rates last computed against the one before it.
type baseline[C, R any] struct {
	ok       bool
	counters C
	at       time.Time
	rates    R
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

// advance replaces b with counters read at now, computing rates against the old baseline with rate. An unreadable
// input drops the baseline, so the next interval has nothing to compare with either. Less than a second after the
// baseline, the last rates stand and the baseline stays put, as for CPU.
func advance[C, R any](b *baseline[C, R], now time.Time, counters C, err error, rate func(old C, dt float64) R) R {
	if err != nil {
		*b = baseline[C, R]{}
		var zero R
		return zero
	}
	var rates R
	if !b.ok {
		// Without a baseline every device is new, so each is listed with null rates.
		var none C
		rates = rate(none, 0)
	} else if dt := now.Sub(b.at); dt < time.Second {
		return b.rates
	} else {
		rates = rate(b.counters, dt.Seconds())
	}
	*b = baseline[C, R]{ok: true, counters: counters, at: now, rates: rates}
	return rates
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
	v := uint64(math.Round(float64(*d) * scale / dt))
	return &v
}

// sum adds d to total, or makes total nil if d is unavailable or the sum would overflow.
func sum(total **uint64, d *uint64) {
	if *total == nil || d == nil || math.MaxUint64-**total < *d {
		*total = nil
		return
	}
	**total += *d
}

func (r *Recorder) netRates(now time.Time) netRates {
	counters, err := probe.Network(r.host)
	return advance(&r.net, now, counters, err, func(old []probe.NetCounters, dt float64) netRates {
		var out netRates
		// The summary divides the counted interfaces' summed deltas, so it isn't off by each one's rounding.
		rx, tx := new(uint64(0)), new(uint64(0))
		counted := false
		prev := map[string]probe.NetCounters{}
		for _, c := range old {
			prev[c.Name] = c
		}
		for _, c := range counters {
			i := NetInterface{Name: c.Name, DeviceBacked: c.DeviceBacked}
			p, seen := prev[c.Name]
			delete(prev, c.Name)
			// An interface that appeared, or whose device link came or went, has no baseline of its own.
			var dRx, dTx *uint64
			if seen && equal(p.DeviceBacked, c.DeviceBacked) {
				dRx, dTx = delta(p.Rx, c.Rx), delta(p.Tx, c.Tx)
			}
			i.Rx, i.Tx = perSecond(dRx, 1, dt), perSecond(dTx, 1, dt)
			out.interfaces = append(out.interfaces, i)
			countedNow := c.DeviceBacked == nil || *c.DeviceBacked
			countedBefore := seen && (p.DeviceBacked == nil || *p.DeviceBacked)
			if !countedNow && !countedBefore {
				continue
			}
			counted = counted || countedNow
			// A link whose existence is unknown may be a counted interface, so it leaves the summary unknown too.
			if c.DeviceBacked == nil {
				dRx, dTx = nil, nil
			}
			sum(&rx, dRx)
			sum(&tx, dTx)
		}
		for _, p := range prev {
			// A counted interface that disappeared took its share of the interval with it.
			if p.DeviceBacked == nil || *p.DeviceBacked {
				rx, tx = nil, nil
			}
		}
		if counted {
			out.rx, out.tx = perSecond(rx, 1, dt), perSecond(tx, 1, dt)
		}
		return out
	})
}

func equal(a, b *bool) bool { return a == nil && b == nil || a != nil && b != nil && *a == *b }

func (r *Recorder) diskRates(now time.Time) []Disk {
	counters, err := probe.Disks(r.host)
	return advance(&r.disk, now, counters, err, func(old []probe.DiskCounters, dt float64) []Disk {
		prev := map[string]probe.DiskCounters{}
		for _, c := range old {
			prev[c.Name] = c
		}
		var out []Disk
		for _, c := range counters {
			d := Disk{Name: c.Name}
			if p, ok := prev[c.Name]; ok {
				// Sectors in diskstats are always 512 bytes, whatever the disk's logical sector size.
				d.Read = perSecond(delta(p.ReadSectors, c.ReadSectors), 512, dt)
				d.Write = perSecond(delta(p.WriteSectors, c.WriteSectors), 512, dt)
			}
			out = append(out, d)
		}
		return out
	})
}
