package probe_test

import (
	"testing"

	"github.com/fsrodrig/anshin/agent/internal/capture"
	"github.com/fsrodrig/anshin/agent/internal/capture/replay"
	"github.com/fsrodrig/anshin/agent/internal/recorder/probe"
)

// TestProbesOnCaptures checks collector correctness against real Machines: each probe's answer is compared with
// a fact the capture recorded from an independent kernel source, or with how a counter must behave.
func TestProbesOnCaptures(t *testing.T) {
	for _, c := range replay.LoadAll(t) {
		t.Run(c.Name, func(t *testing.T) {
			if c.Manifest.CPUsOnline == nil {
				t.Fatal("capture.json has no cpus_online; capture again")
			}
			seq := c.Sequence()
			h := seq.Host()
			var prev *probe.CPUTimes
			for i := 0; ; i++ {
				if got, err := probe.Hostname(h); err != nil || got != c.Name {
					t.Errorf("snapshot %d: Hostname = %q, %v; want the capture name %q", i, got, err, c.Name)
				}
				cpu, err := probe.CPU(h)
				if err != nil {
					t.Fatalf("snapshot %d: CPU: %v", i, err)
				}
				if cpu.CPUs != *c.Manifest.CPUsOnline {
					t.Errorf("snapshot %d: CPU().CPUs = %d, want %d from /sys/devices/system/cpu/online", i, cpu.CPUs, *c.Manifest.CPUsOnline)
				}
				if prev != nil {
					for k, v := range cpu.Jiffies {
						// iowait alone may decrease (Documentation/filesystems/proc.rst); idle+iowait may not.
						if k != 4 && v < prev.Jiffies[k] {
							t.Errorf("snapshot %d: /proc/stat field %d went from %d to %d", i, k, prev.Jiffies[k], v)
						}
					}
					if cpu.Idle() < prev.Idle() || cpu.Total() <= prev.Total() {
						t.Errorf("snapshot %d: idle %d→%d, total %d→%d; want both to grow", i, prev.Idle(), cpu.Idle(), prev.Total(), cpu.Total())
					}
				}
				prev = &cpu
				if !seq.Advance() {
					break
				}
			}
			// A probe reading a file the capture never copied would replay as a silent null.
			for _, p := range seq.Opened() {
				if !capture.Allowed(p) {
					t.Errorf("a probe opened %s, which captures don't copy; add it to the allowlist and capture again", p)
				}
			}
		})
	}
}
