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
				if c.Name == "desktop" {
					mem := probe.Memory(h)
					available := []uint64{13441167360, 13392293888, 13345693696}[i]
					if mem.Total == nil || *mem.Total != 30188183552 || mem.Available == nil || *mem.Available != available || mem.Used == nil || *mem.Used != 30188183552-available {
						t.Fatalf("snapshot %d memory: %+v", i, mem)
					}
					swap := probe.Swap(h)
					used := []uint64{8598867968, 8598585344, 8598339584}[i]
					if swap.Total == nil || *swap.Total != 60375633920 || swap.Used == nil || *swap.Used != used || len(swap.Entries) != 2 || swap.Entries[0].Zram || !swap.Entries[1].Zram || *swap.Entries[0].Used != 0 {
						t.Fatalf("snapshot %d swap: %+v", i, swap)
					}
					load := probe.Load(h)
					uptime := []float64{74459.83, 74461.83, 74463.83}[i]
					one := []float64{15.92, 15.13, 15.13}[i]
					if load.Uptime == nil || *load.Uptime != uptime || load.One == nil || *load.One != one {
						t.Fatalf("snapshot %d load: %+v", i, load)
					}
					groups, err := probe.Mounts(h)
					if err != nil || len(groups) != 2 || len(groups[0]) != 4 || groups[0][0].Point != "/" {
						t.Fatalf("snapshot %d filesystem grouping: %+v %v", i, groups, err)
					}
					for _, g := range groups {
						mount := g[0]
						id, err := h.MountID(mount.Point)
						if err != nil || id != mount.ID {
							t.Fatalf("snapshot %d mount visibility: %v", i, err)
						}
						stat, err := h.Statfs(mount.Point)
						if err != nil {
							t.Fatal(err)
						}
						total, _, _, _ := probe.Capacity(stat)
						want := uint64(1022042832896)
						if mount.Point == "/boot" {
							want = 2143281152
						}
						if total == nil || *total != want {
							t.Fatalf("snapshot %d filesystem %s total: %v", i, mount.Point, total)
						}
					}
					info := probe.Info(h)
					if info.Distro == nil || *info.Distro != "Omarchy" || info.Kernel == nil || *info.Kernel != "7.2.15-1-omarchy" || info.Architecture != nil || info.AgentVersion != nil {
						t.Fatalf("snapshot %d info: %+v", i, info)
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
