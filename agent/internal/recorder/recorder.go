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
	Hostname *string  `json:"hostname"`
	CPUPct   *float64 `json:"cpu_pct"`
}

type Recorder struct {
	host   probe.Host
	now    func() time.Time
	info   probe.MachineInfo
	infoAt time.Time

	mu   sync.Mutex
	base *probe.CPUTimes
	pct  *float64
}

// New reads the Machine through h. It takes a CPU baseline immediately so the first Read a second later already
// has a CPU %.
func New(h probe.Host) *Recorder {
	r := &Recorder{host: h, now: time.Now}
	if t, err := probe.CPU(h); err == nil {
		r.base = &t
	}
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
	return Reading{Hostname: r.hostname(), CPUPct: r.cpuPct(), MemoryReading: probe.Memory(r.host), SwapReading: probe.Swap(r.host), LoadReading: probe.Load(r.host), MachineInfo: r.info}
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
