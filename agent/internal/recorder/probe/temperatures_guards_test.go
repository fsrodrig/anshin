package probe

import (
	"io/fs"
	"reflect"
	"strings"
	"testing"
	"testing/fstest"
	"time"
)

type temperatureHookFS struct {
	*recordingFS
	before func(string)
}

func (f *temperatureHookFS) Open(name string) (fs.File, error) {
	f.before(name)
	return f.recordingFS.Open(name)
}

func TestTemperatureGuardsRecheckedAfterEnumerationAndDriverCalls(t *testing.T) {
	for _, trigger := range []string{hwmonPath(1, "name"), hwmonPath(0, "temp1_label"), hwmonPath(0, "temp1_input")} {
		t.Run(trigger, func(t *testing.T) {
			tree := hwmonTree(map[int]hwmonDev{
				0: {name: "coretemp", parent: "cpu", status: "active", attrs: map[string]string{"temp1_label": "Package id 0", "temp1_input": "61000", "temp1_crit": "100000", "temp2_input": "62000"}},
				1: {name: "amdgpu", attrs: temp("edge", "40000")},
			})
			suspended := false
			forbidden := []string{}
			f := &temperatureHookFS{recordingFS: tree}
			f.before = func(name string) {
				if suspended && strings.HasPrefix(name, hwmonPath(0, "temp")) {
					forbidden = append(forbidden, name)
				}
				if name == trigger {
					tree.m["sys/devices/cpu/power/runtime_status"].Data = []byte("suspended")
					suspended = true
				}
			}
			Temperatures(Host{FS: f}, "")
			if !suspended || len(forbidden) != 0 {
				t.Fatalf("suspended=%v, subsequent sensor opens=%v", suspended, forbidden)
			}
		})
	}
}

func TestTemperatureDevicePreservesSelectionAndGuards(t *testing.T) {
	devs := amdLaptop()
	devs[4] = hwmonDev{name: "amdgpu", parent: "gpu", status: "suspended", attrs: temp("edge", "70000")}
	devs[6] = hwmonDev{name: "fan", attrs: map[string]string{"fan1_input": "1200"}}
	devs[5] = hwmonDev{name: "drivetemp", attrs: temp("disk", "40000")}
	devs[0].attrs["temp1_crit"] = "95000"
	for _, override := range []string{"", "amdgpu/edge", "missing/sensor"} {
		t.Run(override, func(t *testing.T) {
			want := Temperatures(hwmonTree(devs).host(), override)
			f := hwmonTree(devs)

			for range 2 {
				h := Host{FS: f}
				var sensors []TemperatureSensor
				var crit *float64
				cpuID := ""
				if want.CPUSensor != nil {
					cpuID = *want.CPUSensor
				}
				for _, d := range TemperatureDevices(h) {
					result := ReadTemperatureDevice(h, d, cpuID)
					if !result.Complete {
						t.Fatal("device did not complete")
					}
					sensors = append(sensors, result.Sensors...)
					if result.Crit != nil {
						crit = result.Crit
					}
				}
				got := TemperatureValues(sensors, ChooseCPU(sensors, override), crit)
				if !reflect.DeepEqual(got, want) {
					t.Fatalf("got %+v, want %+v", got, want)
				}
			}
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

type statusReplacementFS struct {
	*recordingFS
	statusReads, trigger int
	replaced             bool
	forbidden            []string
}

func (f *statusReplacementFS) Open(name string) (fs.File, error) {
	file, err := f.recordingFS.Open(name)
	if f.replaced && strings.HasPrefix(name, hwmonPath(0, "temp")) {
		f.forbidden = append(f.forbidden, name)
	}
	if name == hwmonPath(0, RuntimeStatusPath) {
		f.statusReads++
		if f.statusReads == f.trigger {
			f.m["sys/devices/cpu/hwmon/hwmon0"] = &fstest.MapFile{Mode: fs.ModeDir | 0755, ModTime: time.Unix(2, 0)}
			f.m["sys/devices/cpu/hwmon/hwmon0/name"] = &fstest.MapFile{Data: []byte("nvme")}
			f.m["sys/devices/cpu/power/runtime_status"] = &fstest.MapFile{Data: []byte("suspended")}
			f.replaced = true
		}
	}
	return file, err
}

func TestTemperatureReplacementDuringRuntimeStatus(t *testing.T) {
	for _, operation := range []string{"label", "input", "crit"} {
		t.Run(operation, func(t *testing.T) {
			tree := hwmonTree(map[int]hwmonDev{0: {name: "coretemp", parent: "cpu", status: "active", attrs: map[string]string{"temp1_label": "Package id 0", "temp1_input": "61000", "temp1_crit": "105000"}}})
			tree.m["sys/devices/cpu/hwmon/hwmon0"] = &fstest.MapFile{Mode: fs.ModeDir | 0755, ModTime: time.Unix(1, 0)}
			h := tree.host()
			d := TemperatureDevices(h)[0]
			metadata := ReadTemperatureDevice(h, d, "").Sensors
			f := &statusReplacementFS{recordingFS: tree, trigger: 2}
			h.FS = f
			switch operation {
			case "label":
				ReadTemperatureDevice(h, d, "")
			case "input":
				ReadTemperatureDeviceSensors(h, d, "", metadata)
			case "crit":
				f.trigger = 1
				ReadTemperatureCrit(h, d, metadata[0])
			}
			if !f.replaced || len(f.forbidden) != 0 {
				t.Fatalf("replacement=%v, forbidden opens=%v", f.replaced, f.forbidden)
			}
		})
	}
}
