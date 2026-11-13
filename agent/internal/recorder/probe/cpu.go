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
	// CPUs counts the per-CPU lines, which the kernel prints only for online CPUs.
	CPUs int
}

func (t CPUTimes) Total() uint64 {
	var sum uint64
	for _, v := range t.Jiffies {
		sum += v
	}
	return sum
}

// Idle is idle plus iowait.
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
	found := false
	sc := bufio.NewScanner(bytes.NewReader(b))
	for sc.Scan() {
		f := strings.Fields(sc.Text())
		if len(f) == 0 || !strings.HasPrefix(f[0], "cpu") {
			continue
		}
		if f[0] != "cpu" {
			t.CPUs++
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
	return t, nil
}
