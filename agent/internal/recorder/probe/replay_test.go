package probe_test

import (
	"io/fs"
	"reflect"
	"slices"
	"strconv"
	"strings"
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
				temps := probe.Temperatures(h, "")
				if temps.CPUSensor != nil {
					row := slices.IndexFunc(temps.Sensors, func(s probe.Sensor) bool { return s.ID == *temps.CPUSensor })
					if row < 0 || !reflect.DeepEqual(temps.Sensors[row].TempC, temps.CPUTempC) {
						t.Errorf("snapshot %d: CPU sensor %s at %v is not the same row of %+v", i, *temps.CPUSensor, temps.CPUTempC, temps.Sensors)
					}
				}
				if c.Name == "desktop" {
					// An override isn't replayed here: it would read amdgpu's temp1_crit, which the Capture never copied.
					cpuTemp := []float64{90, 90, 87.9}[i]
					if temps.CPUSensor == nil || *temps.CPUSensor != "k10temp/Tctl" || temps.CPUTempC == nil || *temps.CPUTempC != cpuTemp ||
						temps.CPUCritC != nil || len(temps.Sensors) != 2 || !reflect.DeepEqual(temps.Sensors[1], probe.Sensor{ID: "amdgpu/edge", TempC: ptr(67.0)}) {
						t.Fatalf("snapshot %d temperatures: %+v", i, temps)
					}
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
					checkDesktopIO(t, h, i)
				}
				prev = &cpu
				if !seq.Advance() {
					break
				}
			}
			if c.Name == "desktop" {
				for _, tc := range []struct {
					dir  string
					want []string
				}{
					{"sys/class/hwmon/hwmon1", []string{"name"}},           // nvme
					{"sys/class/hwmon/hwmon0", []string{"name", "device"}}, // acpitz: a thermal zone, though its runtime_status was captured
				} {
					for _, p := range seq.Opened() {
						if (p == tc.dir || strings.HasPrefix(p, tc.dir+"/")) && !slices.Contains(tc.want, strings.TrimPrefix(p, tc.dir+"/")) {
							t.Errorf("opened %s; only %v may be read there", p, tc.want)
						}
					}
				}
				if slices.Contains(seq.Opened(), "sys/class/hwmon/hwmon3/temp1_crit") {
					t.Error("read amdgpu's temp1_crit, but only the CPU sensor's is read")
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

func ptr[T any](v T) *T { return &v }

// checkDesktopIO checks the desktop capture's network and disk counters. Its sysfs has a device link on the three
// physical NICs only, and on nvme0n1 only: nvme0n1p1 and nvme0n1p2 are partitions, dm-0 is LUKS on nvme0n1p2 and
// zram0 is compressed RAM.
func checkDesktopIO(t *testing.T, h probe.Host, i int) {
	t.Helper()
	net, err := probe.Network(h)
	if err != nil {
		t.Fatalf("snapshot %d: Network: %v", i, err)
	}
	var names, backed []string
	for _, n := range net {
		names = append(names, n.Name)
		if n.DeviceBacked == nil || n.Rx == nil || n.Tx == nil {
			t.Fatalf("snapshot %d: %+v has an unavailable field", i, n)
		}
		if *n.DeviceBacked {
			backed = append(backed, n.Name)
		}
		if n.Name == "enp2s0" {
			rx := []uint64{7172913514, 7173132038, 7173282563}[i]
			tx := []uint64{2069894483, 2071192573, 2071239112}[i]
			if *n.Rx != rx || *n.Tx != tx {
				t.Errorf("snapshot %d: enp2s0 rx, tx = %d, %d; want %d, %d", i, *n.Rx, *n.Tx, rx, tx)
			}
		}
	}
	if want := []string{"lo", "enp2s0", "eno1", "wlp4s0", "tailscale0", "docker0", "br-d6b54ffbf6ad", "br-a1f8b72674e0", "veth1a2a14e"}; !slices.Equal(names, want) {
		t.Errorf("snapshot %d: interfaces %v, want %v in kernel order", i, names, want)
	}
	if want := []string{"enp2s0", "eno1", "wlp4s0"}; !slices.Equal(backed, want) {
		t.Errorf("snapshot %d: device-backed interfaces %v, want %v", i, backed, want)
	}

	disks, err := probe.Disks(h)
	if err != nil {
		t.Fatalf("snapshot %d: Disks: %v", i, err)
	}
	if len(disks) != 1 || disks[0].Name != "nvme0n1" {
		t.Fatalf("snapshot %d: disks %+v, want nvme0n1 alone", i, disks)
	}
	rows := diskstats(t, h)
	nvme := rows["nvme0n1"]
	if *disks[0].ReadSectors != nvme[0] || *disks[0].WriteSectors != nvme[1] {
		t.Errorf("snapshot %d: nvme0n1 sectors %d, %d; want %v", i, *disks[0].ReadSectors, *disks[0].WriteSectors, nvme)
	}
	// The roll-up: the LUKS device's I/O reaches the disk through its partition, and the disk already counts
	// both partitions, so the left-out devices hold nothing the disk doesn't.
	p1, p2, dm := rows["nvme0n1p1"], rows["nvme0n1p2"], rows["dm-0"]
	for k := range 2 {
		if dm[k] > p2[k] || p1[k]+p2[k] > nvme[k] {
			t.Errorf("snapshot %d: sectors field %d: dm-0 %d, nvme0n1p2 %d, nvme0n1p1 %d, nvme0n1 %d; want dm-0 ≤ p2 and p1+p2 ≤ disk", i, k, dm[k], p2[k], p1[k], nvme[k])
		}
	}
}

func diskstats(t *testing.T, h probe.Host) map[string][2]uint64 {
	t.Helper()
	b, err := fs.ReadFile(h.FS, "proc/diskstats")
	if err != nil {
		t.Fatal(err)
	}
	rows := map[string][2]uint64{}
	for line := range strings.SplitSeq(strings.TrimSpace(string(b)), "\n") {
		f := strings.Fields(line)
		r, err1 := strconv.ParseUint(f[5], 10, 64)
		w, err2 := strconv.ParseUint(f[9], 10, 64)
		if err1 != nil || err2 != nil {
			t.Fatalf("diskstats row %q", line)
		}
		rows[f[2]] = [2]uint64{r, w}
	}
	return rows
}
