// Package recorder measures the Machine. In the tracer bullet it knows only the hostname and total CPU %; lanes,
// the Window, cursors and the Verdict arrive with later slices.
package recorder

import (
	"bufio"
	"bytes"
	"errors"
	"io/fs"
	"math"
	"strconv"
	"strings"
	"sync"
)

// The kernel reports /proc/stat in USER_HZ, which is 100 on every architecture regardless of CONFIG_HZ.
const userHZ = 100

// Reading is what the recorder knows about the Machine right now. A nil field is unavailable.
type Reading struct {
	Hostname *string
	CPUPct   *float64
}

type Recorder struct {
	fsys fs.FS

	mu   sync.Mutex
	base *cpuTimes
	pct  *float64
}

// New reads from fsys rooted at the Machine's "/". It takes a CPU baseline immediately so the first Read a second
// later already has a CPU %.
func New(fsys fs.FS) *Recorder {
	r := &Recorder{fsys: fsys}
	if t, err := readCPU(fsys); err == nil {
		r.base = &t
	}
	return r
}

// Read returns the hostname and the total CPU % since the previous baseline. A baseline is only replaced once at
// least a second of CPU time has passed, so two callers polling close together don't get percentages computed
// from a handful of jiffies.
func (r *Recorder) Read() Reading {
	return Reading{Hostname: r.hostname(), CPUPct: r.cpuPct()}
}

func (r *Recorder) hostname() *string {
	b, err := fs.ReadFile(r.fsys, "proc/sys/kernel/hostname")
	if err != nil {
		return nil
	}
	h := string(bytes.TrimSpace(b))
	if h == "" {
		return nil
	}
	return &h
}

func (r *Recorder) cpuPct() *float64 {
	r.mu.Lock()
	defer r.mu.Unlock()

	now, err := readCPU(r.fsys)
	if err != nil {
		r.base, r.pct = nil, nil
		return nil
	}
	if r.base == nil || now.cpus != r.base.cpus || now.total < r.base.total || now.idle < r.base.idle {
		// A first reading, a CPU coming or going, or a counter running backwards: no honest delta exists, so
		// this interval is unavailable and the next one starts from here.
		r.base, r.pct = &now, nil
		return nil
	}
	dTotal := now.total - r.base.total
	if dTotal < uint64(now.cpus)*userHZ {
		return r.pct
	}
	dIdle := now.idle - r.base.idle
	pct := math.Round(1000*float64(dTotal-min(dIdle, dTotal))/float64(dTotal)) / 10
	r.base, r.pct = &now, &pct
	return r.pct
}

type cpuTimes struct {
	total, idle uint64
	cpus        int
}

var errNoCPULine = errors.New("/proc/stat has no cpu line")

func readCPU(fsys fs.FS) (cpuTimes, error) {
	b, err := fs.ReadFile(fsys, "proc/stat")
	if err != nil {
		return cpuTimes{}, err
	}
	var t cpuTimes
	found := false
	sc := bufio.NewScanner(bytes.NewReader(b))
	for sc.Scan() {
		f := strings.Fields(sc.Text())
		if len(f) == 0 || !strings.HasPrefix(f[0], "cpu") {
			continue
		}
		if f[0] != "cpu" {
			t.cpus++
			continue
		}
		// user nice system idle iowait irq softirq steal; guest and guest_nice are already counted in user and nice.
		if len(f) < 9 {
			return cpuTimes{}, errors.New("/proc/stat cpu line is too short")
		}
		for i, s := range f[1:9] {
			v, err := strconv.ParseUint(s, 10, 64)
			if err != nil {
				return cpuTimes{}, err
			}
			t.total += v
			if i == 3 || i == 4 {
				t.idle += v
			}
		}
		found = true
	}
	if !found {
		return cpuTimes{}, errNoCPULine
	}
	t.cpus = max(t.cpus, 1)
	return t, nil
}
