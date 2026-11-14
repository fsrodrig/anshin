package recorder

import (
	"io/fs"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"testing/fstest"
	"time"

	"github.com/fsrodrig/anshin/agent/internal/recorder/probe"
)

type laneFS struct {
	tree    atomic.Pointer[fstest.MapFS]
	block   atomic.Pointer[temperatureBlock]
	blocked string
	mu      sync.Mutex
	opens   map[string]int
}

func (f *laneFS) Open(name string) (fs.File, error) {
	if base := name[strings.LastIndex(name, "/")+1:]; strings.HasPrefix(base, "temp") {
		f.mu.Lock()
		f.opens[name[:strings.LastIndex(name, "/")]]++
		f.mu.Unlock()
	}
	if b := f.block.Load(); b != nil && name == f.blocked {
		if b.calls.Add(1) == 1 {
			close(b.started)
		}
		<-b.release
	}
	return f.tree.Load().Open(name)
}
func (f *laneFS) ReadLink(name string) (string, error)   { return f.tree.Load().ReadLink(name) }
func (f *laneFS) Lstat(name string) (fs.FileInfo, error) { return f.tree.Load().Lstat(name) }

// sensorOpens returns how many temp* attributes of each hwmon device were opened since the last call.
func (f *laneFS) sensorOpens() map[string]int {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := f.opens
	f.opens = map[string]int{}
	return out
}

func (f *laneFS) set(path, data string) {
	next := fstest.MapFS{}
	for k, v := range *f.tree.Load() {
		next[k] = v
	}
	next[path] = &fstest.MapFile{Data: []byte(data)}
	f.tree.Store(&next)
}

const (
	laneCPU         = "sys/class/hwmon/hwmon0"
	laneGPU         = "sys/class/hwmon/hwmon1"
	laneVirtual     = "sys/class/hwmon/hwmon2"
	laneUnsupported = "sys/class/hwmon/hwmon3"
	laneNoStatus    = "sys/class/hwmon/hwmon4"
	laneGPUStatus   = "sys/devices/gpu/power/runtime_status"
)

func laneTree(gpuGeneration int64) *fstest.MapFS {
	link := func(target string) *fstest.MapFile {
		return &fstest.MapFile{Mode: fs.ModeSymlink | 0755, Data: []byte("../../../devices/" + target)}
	}
	m := fstest.MapFS{
		laneCPU + "/name":                          {Data: []byte("k10temp")},
		laneCPU + "/device":                        link("cpu"),
		"sys/devices/cpu/power/runtime_status":     {Data: []byte("active")},
		laneCPU + "/temp1_label":                   {Data: []byte("Tctl")},
		laneCPU + "/temp1_input":                   {Data: []byte("61000")},
		laneGPU:                                    {Mode: fs.ModeDir | 0755, ModTime: time.Unix(gpuGeneration, 0)},
		laneGPU + "/name":                          {Data: []byte("amdgpu")},
		laneGPU + "/device":                        link("gpu"),
		laneGPUStatus:                              {Data: []byte("active")},
		laneGPU + "/temp1_label":                   {Data: []byte("edge")},
		laneGPU + "/temp1_input":                   {Data: []byte("67000")},
		laneVirtual + "/name":                      {Data: []byte("it87")},
		laneVirtual + "/temp1_input":               {Data: []byte("33000")},
		laneUnsupported + "/name":                  {Data: []byte("nct6775")},
		laneUnsupported + "/device":                link("superio"),
		"sys/devices/superio/power/runtime_status": {Data: []byte("unsupported")},
		laneUnsupported + "/temp1_input":           {Data: []byte("35000")},
		laneNoStatus + "/name":                     {Data: []byte("acpi_fan")},
		laneNoStatus + "/device":                   link("fan"),
		"sys/devices/fan/power":                    {Mode: fs.ModeDir | 0755},
		laneNoStatus + "/temp1_input":              {Data: []byte("30000")},
	}
	return &m
}

func newLaneFS() *laneFS {
	f := &laneFS{opens: map[string]int{}}
	f.tree.Store(laneTree(1))
	return f
}

func laneRow(t *testing.T, r probe.TemperatureReading, id string) (probe.Sensor, bool) {
	t.Helper()
	for _, s := range r.Sensors {
		if s.ID == id {
			return s, true
		}
	}
	return probe.Sensor{}, false
}

func wantRow(t *testing.T, r probe.TemperatureReading, id string, temp float64, age int64) {
	t.Helper()
	s, ok := laneRow(t, r, id)
	if !ok || s.TempC == nil || *s.TempC != temp || s.Age == nil || *s.Age != age {
		t.Fatalf("%s: want %v °C aged %d s, got %+v", id, temp, age, r.Sensors)
	}
}

func TestSlowLaneTemperatures(t *testing.T) {
	f := newLaneFS()
	h := probe.Host{FS: f}
	var sampler temperatureSampler
	t0 := time.Unix(1000, 0)
	r := sampler.Read(h, "", t0)
	if opens := f.sensorOpens(); opens[laneGPU] == 0 {
		t.Fatal("slow-lane device not read on the first Read that sees it")
	}
	wantRow(t, r, "amdgpu/edge", 67, 0)
	f.set(laneGPU+"/temp1_input", "70000")
	for _, s := range []int64{2, 28} {
		r = sampler.Read(h, "", t0.Add(time.Duration(s)*time.Second))
		opens := f.sensorOpens()
		if opens[laneGPU] != 0 {
			t.Fatalf("read slow-lane device at +%d s", s)
		}
		for _, dir := range []string{laneCPU, laneVirtual, laneUnsupported, laneNoStatus} {
			if opens[dir] == 0 {
				t.Fatalf("fast-lane device %s not read at +%d s", dir, s)
			}
		}
		wantRow(t, r, "amdgpu/edge", 67, s)
		wantRow(t, r, "k10temp/Tctl", 61, 0)
		wantRow(t, r, "it87/temp1", 33, 0)
		wantRow(t, r, "nct6775/temp1", 35, 0)
		wantRow(t, r, "acpi_fan/temp1", 30, 0)
		if r.CPUSensor == nil || *r.CPUSensor != "k10temp/Tctl" || r.CPUTempC == nil || *r.CPUTempC != 61 {
			t.Fatalf("CPU at +%d s: %+v", s, r)
		}
	}
	r = sampler.Read(h, "", t0.Add(30*time.Second))
	if f.sensorOpens()[laneGPU] == 0 {
		t.Fatal("slow-lane device not read 30 s after its last launch")
	}
	wantRow(t, r, "amdgpu/edge", 70, 0)
}

func TestCPUSensorOverrideKeepsItsDeviceOnTheFastLane(t *testing.T) {
	f := newLaneFS()
	h := probe.Host{FS: f}
	var sampler temperatureSampler
	t0 := time.Unix(1000, 0)
	sampler.Read(h, "", t0)
	f.sensorOpens()
	for i := 1; i <= 3; i++ {
		r := sampler.Read(h, "amdgpu/edge", t0.Add(time.Duration(2*i)*time.Second))
		opens := f.sensorOpens()
		if opens[laneGPU] == 0 {
			t.Fatalf("CPU sensor's device not read on Read %d", i)
		}
		if r.CPUSensor == nil || *r.CPUSensor != "amdgpu/edge" || r.CPUTempC == nil || *r.CPUTempC != 67 {
			t.Fatalf("override CPU: %+v", r)
		}
		wantRow(t, r, "amdgpu/edge", 67, 0)
		if i > 1 {
			if opens[laneCPU] != 0 {
				t.Fatal("read a runtime-PM device that no longer holds the CPU sensor")
			}
		}
	}
	r := sampler.Read(h, "", t0.Add(8*time.Second))
	if opens := f.sensorOpens(); opens[laneGPU] != 0 || opens[laneCPU] == 0 {
		t.Fatalf("lanes did not follow the CPU choice back: %v", opens)
	}
	wantRow(t, r, "amdgpu/edge", 67, 2)
}

func TestUnknownCPUChoiceKeepsOtherRuntimePMDevicesSlow(t *testing.T) {
	f := newLaneFS()
	f.blocked = laneCPU + "/temp1_label"
	b := &temperatureBlock{started: make(chan struct{}), release: make(chan struct{})}
	f.block.Store(b)
	defer close(b.release)
	h := probe.Host{FS: f}
	var sampler temperatureSampler
	t0 := time.Unix(1000, 0)
	sampler.Read(h, "", t0)
	f.sensorOpens()
	r := sampler.Read(h, "", t0.Add(2*time.Second))
	if f.sensorOpens()[laneGPU] != 0 {
		t.Fatal("an unknown CPU choice put an unrelated runtime-PM device on the fast lane")
	}
	if r.CPUSensor != nil {
		t.Fatalf("selected CPU before discovery: %+v", r)
	}
	wantRow(t, r, "amdgpu/edge", 67, 2)
}

func TestSlowLaneDeviceSuspendedIsNeverOpened(t *testing.T) {
	f := newLaneFS()
	h := probe.Host{FS: f}
	var sampler temperatureSampler
	t0 := time.Unix(1000, 0)
	sampler.Read(h, "", t0)
	f.sensorOpens()
	f.set(laneGPUStatus, "suspended")
	for _, s := range []int64{2, 30, 60} {
		r := sampler.Read(h, "", t0.Add(time.Duration(s)*time.Second))
		if f.sensorOpens()[laneGPU] != 0 {
			t.Fatalf("opened a suspended device at +%d s", s)
		}
		if _, ok := laneRow(t, r, "amdgpu/edge"); ok {
			t.Fatalf("listed a suspended device at +%d s: %+v", s, r.Sensors)
		}
	}
}

func TestSlowLaneIdentityChangeDropsRetainedValues(t *testing.T) {
	f := newLaneFS()
	h := probe.Host{FS: f}
	var sampler temperatureSampler
	t0 := time.Unix(1000, 0)
	sampler.Read(h, "", t0)
	f.tree.Store(laneTree(2))
	f.blocked = laneGPU + "/temp1_input"
	b := &temperatureBlock{started: make(chan struct{}), release: make(chan struct{})}
	f.block.Store(b)
	defer close(b.release)
	r := sampler.Read(h, "", t0.Add(2*time.Second))
	if b.calls.Load() != 1 {
		t.Fatal("replacement not read on the first Read that sees it")
	}
	for _, s := range r.Sensors {
		if strings.HasPrefix(s.ID, "amdgpu/") && s.TempC != nil {
			t.Fatalf("replacement presented retained values: %+v", r.Sensors)
		}
	}
}

func TestSlowLaneRefreshNeverResurrectsRetainedValues(t *testing.T) {
	f := newLaneFS()
	h := probe.Host{FS: f}
	var sampler temperatureSampler
	t0 := time.Unix(1000, 0)
	wantRow(t, sampler.Read(h, "", t0), "amdgpu/edge", 67, 0)
	f.blocked = laneGPU + "/temp1_input"
	for _, seconds := range []int64{30, 60, 600} {
		b := &temperatureBlock{started: make(chan struct{}), release: make(chan struct{})}
		f.block.Store(b)
		r := sampler.Read(h, "", t0.Add(time.Duration(seconds)*time.Second))
		ch := sampler.devices[laneGPU].pending
		close(b.release)
		if ch == nil {
			t.Fatal("slow read did not remain pending")
		}
		result := <-ch
		ch <- result
		for _, reading := range []probe.TemperatureReading{r, sampler.Read(h, "", t0.Add(time.Duration(seconds+2)*time.Second))} {
			s, ok := laneRow(t, reading, "amdgpu/edge")
			if !ok || s.TempC != nil || s.Age != nil {
				t.Fatalf("refresh resurrected retained value: %+v", reading.Sensors)
			}
		}
	}
	f.block.Store(nil)
	f.set(laneGPU+"/temp1_input", "70000")
	wantRow(t, sampler.Read(h, "", t0.Add(630*time.Second)), "amdgpu/edge", 70, 0)
	wantRow(t, sampler.Read(h, "", t0.Add(632*time.Second)), "amdgpu/edge", 70, 2)
}

func TestSlowLaneCadenceSurvivesSkippedDevice(t *testing.T) {
	for _, absent := range []bool{false, true} {
		t.Run(map[bool]string{false: "suspended", true: "absent"}[absent], func(t *testing.T) {
			f := newLaneFS()
			h := probe.Host{FS: f}
			var sampler temperatureSampler
			t0 := time.Unix(1000, 0)
			sampler.Read(h, "", t0)
			f.sensorOpens()
			if absent {
				tree := laneTree(1)
				for key := range *tree {
					if strings.HasPrefix(key, laneGPU) {
						delete(*tree, key)
					}
				}
				f.tree.Store(tree)
			} else {
				f.set(laneGPUStatus, "suspended")
			}
			r := sampler.Read(h, "", t0.Add(6*time.Second))
			if _, ok := laneRow(t, r, "amdgpu/edge"); ok {
				t.Fatal("listed skipped device")
			}
			f.tree.Store(laneTree(1))
			for _, seconds := range []int64{8, 28} {
				r = sampler.Read(h, "", t0.Add(time.Duration(seconds)*time.Second))
				if f.sensorOpens()[laneGPU] != 0 {
					t.Fatalf("opened resumed device at +%d s", seconds)
				}
				if _, ok := laneRow(t, r, "amdgpu/edge"); ok {
					t.Fatal("restored skipped device's retained IDs")
				}
			}
			wantRow(t, sampler.Read(h, "", t0.Add(30*time.Second)), "amdgpu/edge", 67, 0)
			if f.sensorOpens()[laneGPU] == 0 {
				t.Fatal("did not refresh at +30 s")
			}
			f.set(laneGPUStatus, "suspended")
			sampler.Read(h, "", t0.Add(60*time.Second))
			if _, ok := sampler.devices[laneGPU]; ok {
				t.Fatal("kept expired launch timestamp")
			}
		})
	}
}

func TestSlowLaneSkippedReplacementStartsFresh(t *testing.T) {
	f := newLaneFS()
	h := probe.Host{FS: f}
	var sampler temperatureSampler
	t0 := time.Unix(1000, 0)
	sampler.Read(h, "", t0)
	f.set(laneGPUStatus, "suspended")
	sampler.Read(h, "", t0.Add(6*time.Second))
	f.sensorOpens()
	f.tree.Store(laneTree(2))
	wantRow(t, sampler.Read(h, "", t0.Add(8*time.Second)), "amdgpu/edge", 67, 0)
	if f.sensorOpens()[laneGPU] == 0 {
		t.Fatal("replacement inherited cadence")
	}
}
