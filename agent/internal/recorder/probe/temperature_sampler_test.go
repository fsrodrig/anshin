package probe

import (
	"io/fs"
	"reflect"
	"sync"
	"testing"
)

type synchronizedTemperatureFS struct {
	mu sync.Mutex
	*recordingFS
}

func (f *synchronizedTemperatureFS) Open(name string) (fs.File, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.recordingFS.Open(name)
}
func (f *synchronizedTemperatureFS) ReadLink(name string) (string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.recordingFS.ReadLink(name)
}
func (f *synchronizedTemperatureFS) Lstat(name string) (fs.FileInfo, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.recordingFS.Lstat(name)
}

func TestTemperatureSamplerPreservesSelectionAndGuards(t *testing.T) {
	devs := amdLaptop()
	devs[4] = hwmonDev{name: "amdgpu", parent: "gpu", status: "suspended", attrs: temp("edge", "70000")}
	devs[6] = hwmonDev{name: "fan", attrs: map[string]string{"fan1_input": "1200"}}
	devs[5] = hwmonDev{name: "drivetemp", attrs: temp("disk", "40000")}
	devs[0].attrs["temp1_crit"] = "95000"
	for _, override := range []string{"", "amdgpu/edge", "missing/sensor"} {
		t.Run(override, func(t *testing.T) {
			want := Temperatures(hwmonTree(devs).host(), override)
			f := &synchronizedTemperatureFS{recordingFS: hwmonTree(devs)}
			var sampler TemperatureSampler
			for range 2 {
				got := sampler.Read(Host{FS: f}, override)
				if !reflect.DeepEqual(got, want) {
					t.Fatalf("got %+v, want %+v", got, want)
				}
			}
			f.mu.Lock()
			defer f.mu.Unlock()
			for _, n := range []int{2, 3, 4, 5} {
				for _, p := range f.under(n) {
					if isTemp(p) {
						t.Errorf("opened skipped device attribute %s", p)
					}
				}
			}
			for _, p := range f.openedMatching(isCrit) {
				if override == "" && p != hwmonPath(0, "temp1_crit") || override == "amdgpu/edge" && p != hwmonPath(1, "temp1_crit") || override == "missing/sensor" {
					t.Errorf("read non-CPU critical temperature %s", p)
				}
			}
		})
	}
}
