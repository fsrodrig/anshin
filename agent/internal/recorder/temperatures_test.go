package recorder

import (
	"io/fs"
	"sync/atomic"
	"syscall"
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
	blockedPath   string
	failDiscovery atomic.Bool
	secondOpens   atomic.Int32
	tree          atomic.Pointer[fstest.MapFS]
	block         atomic.Pointer[temperatureBlock]
}

func (f *temperatureFS) Open(name string) (fs.File, error) {
	file, err := f.tree.Load().Open(name)
	if name == "sys/class/hwmon/hwmon0" && f.failDiscovery.Load() && err == nil {
		return temperatureFailDir{file}, nil
	}
	if name == "sys/class/hwmon/hwmon0/temp2_input" {
		f.secondOpens.Add(1)
	}
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
	case <-time.After(10 * TemperatureReadTimeout):
		t.Fatal("Read blocked behind a sensor driver")
		return Reading{}
	}
}

func TestBlockedTemperaturesKeepOtherMeasurementsAndIdentity(t *testing.T) {
	f := &temperatureFS{}
	f.tree.Store(temperatureTree("Package id 0", "61000", 1))
	r := New(probe.Host{FS: f}, time.Unix(100, 0), Config{})
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
			if replace && v.CPUSensor != nil {
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
			defer close(b.release)
			r := New(probe.Host{FS: f}, time.Unix(100, 0), Config{})
			f.block.Store(b)
			first := make(chan Reading, 1)
			go func() { first <- r.ReadAt(time.Unix(200, 0)) }()
			select {
			case <-b.started:
			case <-time.After(time.Second):
				t.Fatal("driver read not started")
			}
			v := boundedTemperatureRead(t, r)
			if (attr == "temp1_label" && v.CPUTempC != nil) || v.CPUCritC != nil || v.MemoryReading.Total == nil {
				t.Fatalf("concurrent Read: %+v", v)
			}
			select {
			case v = <-first:
				if (attr == "temp1_label" && v.CPUTempC != nil) || v.CPUCritC != nil || v.MemoryReading.Total == nil {
					t.Fatalf("blocked Read: %+v", v)
				}
				if attr == "temp1_label" && v.CPUSensor != nil {
					t.Fatal("selected a fallback before discovery answered")
				}
			case <-time.After(10 * TemperatureReadTimeout):
				t.Fatal("first Read blocked")
			}
			if b.calls.Load() != 1 {
				t.Fatal("duplicate driver reads", b.calls.Load())
			}
		})
	}
}

type temperatureFailDir struct{ fs.File }

func (f temperatureFailDir) ReadDir(int) ([]fs.DirEntry, error) { return nil, syscall.EIO }

func TestTemperatureDiscoveryRetriesAfterError(t *testing.T) {
	f := &temperatureFS{}
	f.tree.Store(temperatureTree("Package id 0", "61000", 1))
	f.failDiscovery.Store(true)
	var sampler temperatureSampler
	if r := sampler.Read(probe.Host{FS: f}, ""); r.CPUSensor != nil {
		t.Fatal("selected CPU after failed discovery")
	}
	f.failDiscovery.Store(false)
	r := sampler.Read(probe.Host{FS: f}, "")
	if r.CPUSensor == nil || *r.CPUSensor != "coretemp/Package id 0" || r.CPUTempC == nil || *r.CPUTempC != 61 {
		t.Fatalf("did not recover: %+v", r)
	}
}

func TestPendingTemperatureWorkerStopsAtReplacement(t *testing.T) {
	f := &temperatureFS{}
	tree := temperatureTree("Package id 0", "61000", 1)
	(*tree)["sys/class/hwmon/hwmon0/temp2_input"] = &fstest.MapFile{Data: []byte("62000")}
	f.tree.Store(tree)
	b := &temperatureBlock{started: make(chan struct{}), release: make(chan struct{})}
	f.block.Store(b)
	var sampler temperatureSampler
	sampler.Read(probe.Host{FS: f}, "")
	replacement := temperatureTree("Composite", "40000", 2)
	(*replacement)["sys/class/hwmon/hwmon0/name"] = &fstest.MapFile{Data: []byte("nvme")}
	(*replacement)["sys/class/hwmon/hwmon0/temp2_input"] = &fstest.MapFile{Data: []byte("41000")}
	f.tree.Store(replacement)
	sampler.Read(probe.Host{FS: f}, "")
	ch := sampler.devices["sys/class/hwmon/hwmon0"].pending
	close(b.release)
	<-ch
	if got := f.secondOpens.Load(); got != 0 {
		t.Fatalf("opened replacement's second input %d times", got)
	}
	if b.calls.Load() != 1 {
		t.Fatal("replaced outstanding operation")
	}
}

func TestReadyTemperatureResultWinsExpiredDeadline(t *testing.T) {
	deadline := time.Now().Add(-time.Second)
	for range 200 {
		ch := make(chan temperatureResult, 1)
		ch <- temperatureResult{finished: deadline.Add(-time.Millisecond)}
		result, ok := waitTemperature(ch, deadline)
		if !ok || result.finished.After(deadline) {
			t.Fatal("lost timely result to expired timer")
		}
	}
}

func TestBlockedDiscoveryOnlySuppressesPossibleCPUChoices(t *testing.T) {
	for _, name := range []string{"amdgpu", "coretemp"} {
		for _, override := range []string{"", "coretemp/Package id 0"} {
			t.Run(name+override, func(t *testing.T) {
				f := &temperatureFS{blockedPath: "sys/class/hwmon/hwmon1/temp1_label"}
				tree := temperatureTree("Package id 0", "61000", 1)
				(*tree)["sys/class/hwmon/hwmon1/name"] = &fstest.MapFile{Data: []byte(name)}
				f.tree.Store(tree)
				b := &temperatureBlock{started: make(chan struct{}), release: make(chan struct{})}
				f.block.Store(b)
				defer close(b.release)
				var sampler temperatureSampler
				for range 3 {
					r := sampler.Read(probe.Host{FS: f}, override)
					if name == "coretemp" {
						if r.CPUSensor != nil || r.CPUTempC != nil {
							t.Fatalf("selected before CPU discovery: %+v", r)
						}
					} else if r.CPUSensor == nil || r.CPUTempC == nil || *r.CPUTempC != 61 {
						t.Fatalf("unrelated discovery suppressed CPU: %+v", r)
					}
				}
				if b.calls.Load() != 1 {
					t.Fatal("duplicate discovery")
				}
			})
		}
	}
}

func TestTimelyCPUCompletesBehindBlockedGPU(t *testing.T) {
	f := &temperatureFS{}
	tree := temperatureTree("edge", "40000", 1)
	(*tree)["sys/class/hwmon/hwmon0/name"] = &fstest.MapFile{Data: []byte("amdgpu")}
	(*tree)["sys/class/hwmon/hwmon1/name"] = &fstest.MapFile{Data: []byte("coretemp")}
	(*tree)["sys/class/hwmon/hwmon1/temp1_label"] = &fstest.MapFile{Data: []byte("Package id 0")}
	(*tree)["sys/class/hwmon/hwmon1/temp1_input"] = &fstest.MapFile{Data: []byte("61000")}
	f.tree.Store(tree)
	b := &temperatureBlock{started: make(chan struct{}), release: make(chan struct{})}
	f.block.Store(b)
	defer close(b.release)
	var sampler temperatureSampler
	r := sampler.Read(probe.Host{FS: f}, "")
	if r.CPUTempC == nil || *r.CPUTempC != 61 {
		t.Fatalf("lost CPU behind blocked GPU: %+v", r)
	}
}

func TestConcurrentTemperatureReadsShareResult(t *testing.T) {
	f := &temperatureFS{}
	f.tree.Store(temperatureTree("Package id 0", "61000", 1))
	b := &temperatureBlock{started: make(chan struct{}), release: make(chan struct{})}
	f.block.Store(b)
	var sampler temperatureSampler
	results := make(chan probe.TemperatureReading, 32)
	go func() { results <- sampler.Read(probe.Host{FS: f}, "") }()
	<-b.started
	for range 31 {
		go func() { results <- sampler.Read(probe.Host{FS: f}, "") }()
	}
	time.Sleep(20 * time.Millisecond)
	close(b.release)
	for range 32 {
		select {
		case r := <-results:
			if r.CPUTempC == nil || *r.CPUTempC != 61 {
				t.Fatalf("concurrent result: %+v", r)
			}
		case <-time.After(2 * TemperatureReadTimeout):
			t.Fatal("concurrent Reads queued beyond shared bound")
		}
	}
	if b.calls.Load() != 1 {
		t.Fatalf("started %d reads", b.calls.Load())
	}
}
