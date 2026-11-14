package probe

import (
	"io/fs"
	"strings"
	"testing"
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
