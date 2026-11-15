package probe

import (
	"bufio"
	"bytes"
	"errors"
	"io/fs"
	"strconv"
	"strings"
)

// CPUTimes is /proc/stat's aggregate line: user, nice, system, idle, iowait, irq, softirq and steal, in USER_HZ.
// guest and guest_nice are left out because the kernel already counts them in user and nice.
type CPUTimes struct {
	Jiffies [8]uint64
	CPUs    int
	// Cores holds each cpuN line, in kernel order. It is nil when any cpuN line is malformed, so one bad line
	// can't shift which core a value belongs to; the aggregate still stands.
	Cores []CoreTimes
}

// CoreTimes is one cpuN line, with the aggregate's eight fields.
type CoreTimes struct {
	ID      int
	Jiffies [8]uint64
}

func (t CoreTimes) Total() uint64 { return CPUTimes{Jiffies: t.Jiffies}.Total() }

func (t CoreTimes) Idle() uint64 { return t.Jiffies[3] + t.Jiffies[4] }

func (t CPUTimes) Total() uint64 {
	var sum uint64
	for _, v := range t.Jiffies {
		sum += v
	}
	return sum
}

func (t CPUTimes) Idle() uint64 { return t.Jiffies[3] + t.Jiffies[4] }

var (
	ErrNoCPULine    = errors.New("/proc/stat has no cpu line")
	ErrShortCPULine = errors.New("/proc/stat cpu line is too short")
)

func CPU(h Host) (CPUTimes, error) {
	b, err := fs.ReadFile(h.FS, "proc/stat")
	if err != nil {
		return CPUTimes{}, err
	}
	var t CPUTimes
	found, coresOK := false, true
	sc := bufio.NewScanner(bytes.NewReader(b))
	for sc.Scan() {
		f := strings.Fields(sc.Text())
		if len(f) == 0 || !strings.HasPrefix(f[0], "cpu") {
			continue
		}
		if f[0] != "cpu" {
			t.CPUs++
			if core, ok := coreTimes(f); ok && coresOK {
				t.Cores = append(t.Cores, core)
			} else {
				coresOK = false
			}
			continue
		}
		if len(f) < 1+len(t.Jiffies) {
			return CPUTimes{}, ErrShortCPULine
		}
		for i := range t.Jiffies {
			v, err := strconv.ParseUint(f[1+i], 10, 64)
			if err != nil {
				return CPUTimes{}, err
			}
			t.Jiffies[i] = v
		}
		found = true
	}
	if !found {
		return CPUTimes{}, ErrNoCPULine
	}
	t.CPUs = max(t.CPUs, 1)
	if !coresOK {
		t.Cores = nil
	}
	return t, nil
}

func coreTimes(f []string) (CoreTimes, bool) {
	digits := strings.TrimPrefix(f[0], "cpu")
	id, err := strconv.Atoi(digits)
	if err != nil || digits[0] < '0' || digits[0] > '9' || len(f) < 1+8 {
		return CoreTimes{}, false
	}
	c := CoreTimes{ID: id}
	for i := range c.Jiffies {
		v, err := strconv.ParseUint(f[1+i], 10, 64)
		if err != nil {
			return CoreTimes{}, false
		}
		c.Jiffies[i] = v
	}
	return c, true
}
