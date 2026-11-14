package recorder

import (
	"io/fs"
	"sync/atomic"
	"testing"
	"testing/fstest"
	"time"

	"github.com/fsrodrig/anshin/agent/internal/recorder/probe"
)

type temperatureBlock struct {
	started chan struct{}
	release chan struct{}
	calls   atomic.Int32
}

type temperatureFS struct {
	blockedPath string
	tree        atomic.Pointer[fstest.MapFS]
	block       atomic.Pointer[temperatureBlock]
}

func (f *temperatureFS) Open(name string) (fs.File, error) {
	file, err := f.tree.Load().Open(name)
	blocked := f.blockedPath
	if blocked == "" {
		blocked = "sys/class/hwmon/hwmon0/temp1_input"
	}
	if name == blocked {
		if b := f.block.Load(); b != nil {
			if b.calls.Add(1) == 1 {
				close(b.started)
			}
			<-b.release
		}
	}
	return file, err
}
func (f *temperatureFS) ReadLink(name string) (string, error)   { return f.tree.Load().ReadLink(name) }
func (f *temperatureFS) Lstat(name string) (fs.FileInfo, error) { return f.tree.Load().Lstat(name) }

func temperatureTree(label, value string, generation int64) *fstest.MapFS {
	m := fstest.MapFS{
		"proc/meminfo":                       {Data: []byte("MemTotal: 1000 kB\nMemAvailable: 400 kB\n")},
		"sys/class/hwmon/hwmon0":             {Mode: fs.ModeDir | 0755, ModTime: time.Unix(generation, 0)},
		"sys/class/hwmon/hwmon0/name":        {Data: []byte("coretemp")},
		"sys/class/hwmon/hwmon0/temp1_label": {Data: []byte(label)},
		"sys/class/hwmon/hwmon0/temp1_input": {Data: []byte(value)},
		"sys/class/hwmon/hwmon1/name":        {Data: []byte("k10temp")},
		"sys/class/hwmon/hwmon1/temp1_label": {Data: []byte("Tctl")},
		"sys/class/hwmon/hwmon1/temp1_input": {Data: []byte("40000")},
	}
	return &m
}

func boundedTemperatureRead(t *testing.T, r *Recorder) Reading {
	t.Helper()
	done := make(chan Reading, 1)
	go func() { done <- r.ReadAt(time.Unix(200, 0)) }()
	select {
	case v := <-done:
		return v
	case <-time.After(10 * probe.TemperatureReadTimeout):
		t.Fatal("Read blocked behind a sensor driver")
		return Reading{}
	}
}

func TestBlockedTemperaturesKeepOtherMeasurementsAndIdentity(t *testing.T) {
	f := &temperatureFS{}
	f.tree.Store(temperatureTree("Package id 0", "61000", 1))
	r := New(probe.Host{FS: f}, time.Unix(100, 0), Config{CPUSensor: "coretemp/Package id 0"})
	v := boundedTemperatureRead(t, r)
	if v.CPUTempC == nil || *v.CPUTempC != 61 {
		t.Fatalf("initial CPU: %+v", v.TemperatureReading)
	}
	b := &temperatureBlock{started: make(chan struct{}), release: make(chan struct{})}
	f.block.Store(b)
	defer close(b.release)
	for range 3 {
		v = boundedTemperatureRead(t, r)
		if v.CPUSensor == nil || *v.CPUSensor != "coretemp/Package id 0" || v.CPUTempC != nil || v.Sensors[0].TempC != nil {
			t.Fatalf("blocked sensor changed identity or retained value: %+v", v.TemperatureReading)
		}
		if v.MemoryReading.Total == nil {
			t.Fatal("memory unavailable while temperature blocks")
		}
	}
	if b.calls.Load() != 1 {
		t.Fatalf("started %d reads while blocked", b.calls.Load())
	}
}

func TestLateTemperatureCompletionAndReplacement(t *testing.T) {
	for _, replace := range []bool{false, true} {
		t.Run(map[bool]string{false: "same device", true: "replacement"}[replace], func(t *testing.T) {
			f := &temperatureFS{}
			f.tree.Store(temperatureTree("Package id 0", "61000", 1))
			r := New(probe.Host{FS: f}, time.Unix(100, 0), Config{CPUSensor: "coretemp/Package id 0"})
			boundedTemperatureRead(t, r)
			old := &temperatureBlock{started: make(chan struct{}), release: make(chan struct{})}
			f.block.Store(old)
			boundedTemperatureRead(t, r)
			next := &temperatureBlock{started: make(chan struct{}), release: make(chan struct{})}
			defer close(next.release)
			f.block.Store(next)
			if replace {
				f.tree.Store(temperatureTree("Core 2", "72000", 2))
			}
			v := boundedTemperatureRead(t, r)
			if old.calls.Load() != 1 || next.calls.Load() != 0 || v.CPUTempC != nil {
				t.Fatal("replaced pending read or retained temperature")
			}
			close(old.release)
			deadline := time.Now().Add(time.Second)
			for next.calls.Load() == 0 && time.Now().Before(deadline) {
				v = boundedTemperatureRead(t, r)
				if v.CPUTempC != nil {
					t.Fatal("late temperature presented as current", *v.CPUTempC)
				}
			}
			if next.calls.Load() != 1 {
				t.Fatal("did not resume after old read finished")
			}
			if replace && (v.CPUSensor != nil || v.Sensors[0].ID != "coretemp/Core 2") {
				t.Fatalf("replacement identity: %+v", v.TemperatureReading)
			}
		})
	}
}

func TestTemperatureDiscoveryAndCritCannotBlockConcurrentReads(t *testing.T) {
	for _, attr := range []string{"temp1_label", "temp1_crit"} {
		t.Run(attr, func(t *testing.T) {
			f := &temperatureFS{blockedPath: "sys/class/hwmon/hwmon0/" + attr}
			f.tree.Store(temperatureTree("Package id 0", "61000", 1))
			b := &temperatureBlock{started: make(chan struct{}), release: make(chan struct{})}
			f.block.Store(b)
			defer close(b.release)
			r := New(probe.Host{FS: f}, time.Unix(100, 0), Config{})
			first := make(chan Reading, 1)
			go func() { first <- r.ReadAt(time.Unix(200, 0)) }()
			select {
			case <-b.started:
			case <-time.After(time.Second):
				t.Fatal("driver read not started")
			}
			v := boundedTemperatureRead(t, r)
			if v.CPUTempC != nil || v.MemoryReading.Total == nil {
				t.Fatalf("concurrent Read: %+v", v)
			}
			select {
			case v = <-first:
				if v.CPUTempC != nil || v.MemoryReading.Total == nil {
					t.Fatalf("blocked Read: %+v", v)
				}
				if attr == "temp1_label" && v.CPUSensor != nil {
					t.Fatal("selected a fallback before discovery answered")
				}
			case <-time.After(10 * probe.TemperatureReadTimeout):
				t.Fatal("first Read blocked")
			}
			if b.calls.Load() != 1 {
				t.Fatal("duplicate driver reads", b.calls.Load())
			}
		})
	}
}
