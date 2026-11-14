package recorder

import (
	"fmt"
	"io/fs"
	"sync/atomic"
	"testing"
	"testing/fstest"
	"time"

	"github.com/fsrodrig/anshin/agent/internal/capture/replay"
	"github.com/fsrodrig/anshin/agent/internal/recorder/probe"
)

type temperatureCountFS struct {
	fs.FS
	opens, stats, links atomic.Int32
}

func (f *temperatureCountFS) Open(p string) (fs.File, error) { f.opens.Add(1); return f.FS.Open(p) }
func (f *temperatureCountFS) Stat(p string) (fs.FileInfo, error) {
	f.stats.Add(1)
	return fs.Stat(f.FS, p)
}
func (f *temperatureCountFS) Lstat(p string) (fs.FileInfo, error) {
	f.stats.Add(1)
	return fs.Lstat(f.FS, p)
}
func (f *temperatureCountFS) ReadLink(p string) (string, error) {
	f.links.Add(1)
	return fs.ReadLink(f.FS, p)
}

func TestTemperatureSteadyStateOperations(t *testing.T) {
	tree := fstest.MapFS{
		"sys/class/hwmon/hwmon0/name":          {Data: []byte("coretemp")},
		"sys/class/hwmon/hwmon0/device":        {Mode: fs.ModeSymlink | 0755, Data: []byte("../../../devices/cpu")},
		"sys/devices/cpu/power/runtime_status": {Data: []byte("active")},
	}
	for n := 1; n <= 17; n++ {
		tree[fmt.Sprintf("sys/class/hwmon/hwmon0/temp%d_input", n)] = &fstest.MapFile{Data: []byte("61000")}
		tree[fmt.Sprintf("sys/class/hwmon/hwmon0/temp%d_label", n)] = &fstest.MapFile{Data: []byte(fmt.Sprintf("Core %d", n))}
	}
	tree["sys/class/hwmon/hwmon0/temp1_label"].Data = []byte("Package id 0")
	cases := map[string]fs.FS{"synthetic": tree}
	for _, c := range replay.LoadAll(t) {
		if c.Name == "desktop" {
			cases["desktop"] = c.Snapshots[0].Host().FS
		}
	}
	for name, tree := range cases {
		t.Run(name, func(t *testing.T) {
			f := &temperatureCountFS{FS: tree}
			var sampler temperatureSampler
			sampler.Read(probe.Host{FS: f}, "")
			f.opens.Store(0)
			f.stats.Store(0)
			f.links.Store(0)
			r := sampler.Read(probe.Host{FS: f}, "")
			opens, stats, links := f.opens.Load(), f.stats.Load(), f.links.Load()
			t.Logf("per Read: opens=%d Stats=%d ReadLinks=%d sensors=%d", opens, stats, links, len(r.Sensors))
			if total := opens + stats + links; total > int32(4*len(r.Sensors)+40) {
				t.Fatalf("steady-state operations=%d exceed per-sensor allowance", total)
			}
		})
	}
}

func TestRecorderFirstTemperatureCritAndChoiceChange(t *testing.T) {
	for _, change := range []bool{false, true} {
		t.Run(fmt.Sprintf("choiceChange=%t", change), func(t *testing.T) {
			f := &temperatureFS{}
			tree := temperatureTree("Package id 0", "97000", 1)
			(*tree)["sys/class/hwmon/hwmon0/temp1_crit"] = &fstest.MapFile{Data: []byte("105000")}
			(*tree)["sys/class/hwmon/hwmon1/temp1_crit"] = &fstest.MapFile{Data: []byte("99000")}
			f.tree.Store(tree)
			r := New(probe.Host{FS: f}, time.Unix(100, 0), Config{})
			first := boundedTemperatureRead(t, r)
			if !change {
				if first.CPUCritC == nil || *first.CPUCritC != 105 {
					t.Fatalf("first Read crit: %+v", first.TemperatureReading)
				}
				return
			}
			replacement := temperatureTree("edge", "40000", 2)
			(*replacement)["sys/class/hwmon/hwmon0/name"] = &fstest.MapFile{Data: []byte("amdgpu")}
			(*replacement)["sys/class/hwmon/hwmon1/temp1_crit"] = &fstest.MapFile{Data: []byte("99000")}
			f.tree.Store(replacement)
			next := boundedTemperatureRead(t, r)
			if next.CPUSensor == nil || *next.CPUSensor != "k10temp/Tctl" || next.CPUCritC == nil || *next.CPUCritC != 99 {
				t.Fatalf("changed CPU choice: %+v", next.TemperatureReading)
			}
		})
	}
}
