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
	if r.base == nil || now.cpus != r.base.cpus || now.wentBackwards(*r.base) {
		// A first reading, a CPU coming or going, or a counter running backwards: no honest delta exists, so
		// this interval is unavailable and the next one starts from here.
		r.base, r.pct = &now, nil
		return nil
	}
	dTotal := now.total() - r.base.total()
	if dTotal < uint64(now.cpus)*userHZ {
		return r.pct
	}
	dIdle := now.idle() - r.base.idle()
	pct := math.Round(1000*float64(dTotal-min(dIdle, dTotal))/float64(dTotal)) / 10
	r.base, r.pct = &now, &pct
	return r.pct
}

// cpuTimes holds the aggregate line's user, nice, system, idle, iowait, irq, softirq and steal jiffies. guest and
// guest_nice are left out because the kernel already counts them in user and nice.
type cpuTimes struct {
	fields [8]uint64
	cpus   int
}

func (t cpuTimes) total() uint64 {
	var sum uint64
	for _, v := range t.fields {
		sum += v
	}
	return sum
}

func (t cpuTimes) idle() uint64 { return t.fields[3] + t.fields[4] }

// wentBackwards compares each busy counter on its own, so growth elsewhere can't hide a reset in the sums. idle and
// iowait are compared as one counter: the kernel documents that iowait can decrease when time it counted is
// reclassified as idle, and only their sum is used.
func (t cpuTimes) wentBackwards(base cpuTimes) bool {
	for i, v := range t.fields {
		if i != 3 && i != 4 && v < base.fields[i] {
			return true
		}
	}
	return t.idle() < base.idle()
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
		if len(f) < 1+len(t.fields) {
			return cpuTimes{}, errors.New("/proc/stat cpu line is too short")
		}
		for i := range t.fields {
			v, err := strconv.ParseUint(f[1+i], 10, 64)
			if err != nil {
				return cpuTimes{}, err
			}
			t.fields[i] = v
		}
		found = true
	}
	if !found {
		return cpuTimes{}, errNoCPULine
	}
	t.cpus = max(t.cpus, 1)
	return t, nil
}
