package probe

import (
	"fmt"
	"io/fs"
	"reflect"
	"slices"
	"strings"
	"syscall"
	"testing"
	"testing/fstest"
	"time"
)

type hwmonDev struct {
	name    string
	parent  string // "" means no device link
	status  string // "" leaves power/ without runtime_status
	noPower bool
	attrs   map[string]string
}

// Directory identity stats must not hide directory listings from the never-wake assertions.
type recordingFS struct {
	m      fstest.MapFS
	errs   map[string]error
	opened []string
	stats  []string
}

// hwmonTree lays devices out as a Machine does: class entries are symlinks into /sys/devices, so a probe that
// filters them by IsDir fails here too.
func hwmonTree(devs map[int]hwmonDev) *recordingFS {
	m := fstest.MapFS{}
	link := func(p, target string) { m[p] = &fstest.MapFile{Data: []byte(target), Mode: fs.ModeSymlink | 0o777} }
	for n, d := range devs {
		real := fmt.Sprintf("sys/devices/virtual/hwmon/hwmon%d", n)
		if d.parent != "" {
			real = fmt.Sprintf("sys/devices/%s/hwmon/hwmon%d", d.parent, n)
		}
		link(fmt.Sprintf("sys/class/hwmon/hwmon%d", n), "../../"+strings.TrimPrefix(real, "sys/"))
		m[real+"/name"] = &fstest.MapFile{Data: []byte(d.name + "\n"), Mode: 0o444}
		for k, v := range d.attrs {
			m[real+"/"+k] = &fstest.MapFile{Data: []byte(v), Mode: 0o444}
		}
		if d.parent == "" {
			continue
		}
		link(real+"/device", "../../../"+d.parent)
		switch {
		case d.noPower:
			m["sys/devices/"+d.parent] = &fstest.MapFile{Mode: fs.ModeDir | 0o555}
		case d.status != "":
			m["sys/devices/"+d.parent+"/power/runtime_status"] = &fstest.MapFile{Data: []byte(d.status), Mode: 0o444}
		default:
			m["sys/devices/"+d.parent+"/power"] = &fstest.MapFile{Mode: fs.ModeDir | 0o555}
		}
	}
	return &recordingFS{m: m, errs: map[string]error{}}
}

func (r *recordingFS) host() Host { return Host{FS: r} }

func (r *recordingFS) Open(name string) (fs.File, error) {
	r.opened = append(r.opened, name)
	f, err := r.m.Open(name)
	if err != nil {
		return nil, err
	}
	if e, ok := r.errs[name]; ok {
		return failingFile{f, &fs.PathError{Op: "read", Path: name, Err: e}}, nil
	}
	return f, nil
}

func (r *recordingFS) Stat(name string) (fs.FileInfo, error) {
	r.stats = append(r.stats, name)
	return r.m.Stat(name)
}

func (r *recordingFS) ReadLink(name string) (string, error) {
	r.opened = append(r.opened, name)
	if e, ok := r.errs[name]; ok {
		return "", &fs.PathError{Op: "readlink", Path: name, Err: e}
	}
	return r.m.ReadLink(name)
}

func (r *recordingFS) Lstat(name string) (fs.FileInfo, error) {
	r.opened = append(r.opened, name)
	if e, ok := r.errs[name]; ok {
		return nil, &fs.PathError{Op: "lstat", Path: name, Err: e}
	}
	return r.m.Lstat(name)
}

type failingFile struct {
	fs.File
	err error
}

func (f failingFile) Read([]byte) (int, error) { return 0, f.err }

func (f failingFile) ReadDir(int) ([]fs.DirEntry, error) { return nil, f.err }

func hwmonPath(n int, rel string) string { return fmt.Sprintf("sys/class/hwmon/hwmon%d/%s", n, rel) }

func hwmonPaths(n int, rel ...string) []string {
	var out []string
	for _, p := range rel {
		out = append(out, hwmonPath(n, p))
	}
	return out
}

func (r *recordingFS) under(n int) []string {
	dir := fmt.Sprintf("sys/class/hwmon/hwmon%d", n)
	var out []string
	for _, p := range r.opened {
		if p == dir || strings.HasPrefix(p, dir+"/") {
			out = append(out, p)
		}
	}
	return out
}

func (r *recordingFS) didOpen(p string) bool { return slices.Contains(r.opened, p) }

func (r *recordingFS) openedMatching(pred func(string) bool) []string {
	var out []string
	for _, p := range r.opened {
		if pred(p) {
			out = append(out, p)
		}
	}
	return out
}

func isTemp(p string) bool { return strings.HasPrefix(p[strings.LastIndex(p, "/")+1:], "temp") }

func isCrit(p string) bool { return strings.HasSuffix(p, "_crit") }

func c(v float64) *float64 { return &v }

func sensor(id string, temp *float64) Sensor { return Sensor{ID: id, TempC: temp} }

func temp(label, input string) map[string]string {
	return map[string]string{"temp1_label": label, "temp1_input": input}
}

func assertSensors(t *testing.T, got TemperatureReading, want ...Sensor) {
	t.Helper()
	if !reflect.DeepEqual(got.Sensors, want) {
		t.Errorf("Sensors = %s, want %s", fmtSensors(got.Sensors), fmtSensors(want))
	}
}

func fmtSensors(s []Sensor) string {
	var parts []string
	for _, x := range s {
		parts = append(parts, fmt.Sprintf("{%s %s}", x.ID, fmtC(x.TempC)))
	}
	return "[" + strings.Join(parts, " ") + "]"
}

func fmtC(v *float64) string {
	if v == nil {
		return "nil"
	}
	return fmt.Sprint(*v)
}

func assertCPU(t *testing.T, got TemperatureReading, id string, tempC, critC *float64) {
	t.Helper()
	gotID := "<nil>"
	if got.CPUSensor != nil {
		gotID = *got.CPUSensor
	}
	wantID := id
	if id == "" {
		wantID = "<nil>"
	}
	if gotID != wantID || !reflect.DeepEqual(got.CPUTempC, tempC) || !reflect.DeepEqual(got.CPUCritC, critC) {
		t.Errorf("CPU = %s %s crit %s, want %s %s crit %s", gotID, fmtC(got.CPUTempC), fmtC(got.CPUCritC), wantID, fmtC(tempC), fmtC(critC))
	}
}

func amdLaptop() map[int]hwmonDev {
	return map[int]hwmonDev{
		0: {name: "k10temp", parent: "0000:00:18.3", status: "active", attrs: map[string]string{
			"temp1_input": "52125", "temp1_label": "Tctl", "temp3_input": "48000", "temp3_label": "Tccd1"}},
		1: {name: "amdgpu", parent: "0000:04:00.0", status: "active", attrs: map[string]string{
			"temp1_input": "45000", "temp1_label": "edge", "temp1_crit": "100000"}},
		2: {name: "nvme", parent: "nvme0", status: "active", attrs: temp("Composite", "38850")},
		3: {name: "acpitz", parent: "thermal_zone0", status: "active", attrs: map[string]string{"temp1_input": "50000"}},
	}
}

func TestAMDLaptopTctlWithoutCrit(t *testing.T) {
	r := hwmonTree(amdLaptop())
	got := Temperatures(r.host(), "")
	assertCPU(t, got, "k10temp/Tctl", c(52.1), nil)
	assertSensors(t, got, sensor("k10temp/Tctl", c(52.1)), sensor("k10temp/Tccd1", c(48)), sensor("amdgpu/edge", c(45)))
	if u := r.under(2); !reflect.DeepEqual(u, hwmonPaths(2, "name")) {
		t.Errorf("nvme: opened %v, want only its name", u)
	}
	if u := r.under(3); !reflect.DeepEqual(u, hwmonPaths(3, "name", "device")) {
		t.Errorf("thermal zone: opened %v, want only its name and device link", u)
	}
	if r.didOpen(hwmonPath(1, "temp1_crit")) {
		t.Error("read amdgpu's temp1_crit, but only the CPU sensor's is read")
	}
	for _, s := range got.Sensors {
		if !ValidSensorID(s.ID) {
			t.Errorf("listed ID %q is not a valid ANSHIN_CPU_SENSOR", s.ID)
		}
	}
}

func TestIntelCoretempPackageWithCrit(t *testing.T) {
	r := hwmonTree(map[int]hwmonDev{
		0: {name: "acpitz", parent: "thermal_zone0", status: "active", attrs: map[string]string{"temp1_input": "27800"}},
		1: {name: "pch_cannonlake", parent: "0000:00:12.0", status: "active", attrs: map[string]string{"temp1_input": "46000"}},
		2: {name: "coretemp", parent: "coretemp.0", status: "unsupported", attrs: map[string]string{
			"temp1_label": "Package id 0", "temp1_input": "61000", "temp1_crit": "100000",
			"temp2_label": "Core 0", "temp2_input": "59000", "temp2_crit": "100000",
			"temp3_label": "Core 1", "temp3_input": "60000", "temp3_crit": "100000"}},
		3: {name: "iwlwifi_1", parent: "thermal_zone1", status: "active", attrs: map[string]string{"temp1_input": "41000"}},
	})
	got := Temperatures(r.host(), "")
	assertCPU(t, got, "coretemp/Package id 0", c(61), c(100))
	assertSensors(t, got, sensor("pch_cannonlake/temp1", c(46)), sensor("coretemp/Package id 0", c(61)),
		sensor("coretemp/Core 0", c(59)), sensor("coretemp/Core 1", c(60)))
	if !r.didOpen(hwmonPath(2, "temp1_crit")) || r.didOpen(hwmonPath(2, "temp2_crit")) || r.didOpen(hwmonPath(2, "temp3_crit")) {
		t.Errorf("crit reads %v, want only the package's", r.openedMatching(isCrit))
	}
	for _, n := range []int{0, 3} {
		if u := r.under(n); !reflect.DeepEqual(u, hwmonPaths(n, "name", "device")) {
			t.Errorf("thermal zone hwmon%d: opened %v, want only its name and device link", n, u)
		}
	}
}

// A hybrid AMD laptop: reading a runtime-suspended dGPU's sensors would wake it, so only its guards are read.
func TestSuspendedDGPUIsNeverRead(t *testing.T) {
	r := hwmonTree(map[int]hwmonDev{
		0: {name: "k10temp", parent: "0000:00:18.3", status: "active", attrs: temp("Tctl", "55000")},
		1: {name: "amdgpu", parent: "0000:05:00.0", status: "active", attrs: temp("edge", "50000")},
		2: {name: "amdgpu", parent: "0000:03:00.0", status: "suspended", attrs: map[string]string{
			"temp1_input": "40000", "temp1_label": "edge", "temp1_crit": "100000",
			"temp2_input": "42000", "temp2_label": "junction", "temp3_input": "44000", "temp3_label": "mem"}},
	})
	got := Temperatures(r.host(), "")
	assertSensors(t, got, sensor("k10temp/Tctl", c(55)), sensor("amdgpu/edge", c(50)))
	if !slices.Contains(r.stats, "sys/class/hwmon/hwmon2") {
		t.Error("suspended dGPU identity was not statted")
	}
	guards := hwmonPaths(2, "name", "device", RuntimeStatusPath)
	if u := r.under(2); !reflect.DeepEqual(u, guards) {
		t.Errorf("suspended dGPU: opened %v, want exactly %v", u, guards)
	}
	got = Temperatures(r.host(), "amdgpu/junction")
	assertCPU(t, got, "", nil, nil)
	for _, p := range r.under(2) {
		if isTemp(p) {
			t.Errorf("an override naming the suspended dGPU opened %s", p)
		}
	}
}

func TestNoCPUSensor(t *testing.T) {
	t.Run("only display sensors", func(t *testing.T) {
		r := hwmonTree(map[int]hwmonDev{
			0: {name: "amdgpu", parent: "0000:04:00.0", status: "active", attrs: temp("edge", "60000")},
			1: {name: "nvme", parent: "nvme0", status: "active", attrs: temp("Composite", "38000")},
			2: {name: "it87", parent: "it87.2608", status: "unsupported", attrs: map[string]string{"temp1_input": "35000"}},
		})
		got := Temperatures(r.host(), "")
		assertCPU(t, got, "", nil, nil)
		assertSensors(t, got, sensor("amdgpu/edge", c(60)), sensor("it87/temp1", c(35)))
		if crit := r.openedMatching(isCrit); crit != nil {
			t.Errorf("opened %v with no CPU sensor", crit)
		}
	})
	t.Run("no hwmon class", func(t *testing.T) {
		if got := Temperatures(Host{FS: fstest.MapFS{}}, ""); !reflect.DeepEqual(got, TemperatureReading{}) {
			t.Errorf("got %+v, want the zero reading", got)
		}
	})
	t.Run("every device ruled out", func(t *testing.T) {
		r := hwmonTree(map[int]hwmonDev{
			0: {name: "nvme", parent: "nvme0", status: "active", attrs: temp("Composite", "38000")},
			1: {name: "acpitz", parent: "thermal_zone0", status: "active", attrs: map[string]string{"temp1_input": "30000"}},
		})
		if got := Temperatures(r.host(), ""); got.Sensors != nil {
			t.Errorf("Sensors = %#v, want nil", got.Sensors)
		}
	})
}

func TestCPUSensorOrder(t *testing.T) {
	dev := func(name string, attrs map[string]string) hwmonDev {
		return hwmonDev{name: name, parent: "0000:00:18.3", status: "active", attrs: attrs}
	}
	for _, tc := range []struct {
		name  string
		devs  map[int]hwmonDev
		errs  map[string]error
		id    string
		tempC *float64
	}{
		{"Tdie beats Tctl", map[int]hwmonDev{0: dev("k10temp", map[string]string{
			"temp1_label": "Tctl", "temp1_input": "95000", "temp2_label": "Tdie", "temp2_input": "75000",
			"temp3_label": "Tccd1", "temp3_input": "70000"})}, nil, "k10temp/Tdie", c(75)},
		{"zenpower Tdie", map[int]hwmonDev{0: dev("zenpower", map[string]string{
			"temp1_label": "Tdie", "temp1_input": "70000", "temp2_label": "Tctl", "temp2_input": "80000"})}, nil, "zenpower/Tdie", c(70)},
		{"zenpower Tctl", map[int]hwmonDev{0: dev("zenpower", map[string]string{
			"temp2_label": "Tctl", "temp2_input": "80000"})}, nil, "zenpower/Tctl", c(80)},
		{"coretemp without a package", map[int]hwmonDev{0: dev("coretemp", map[string]string{
			"temp2_label": "Core 0", "temp2_input": "50000", "temp3_label": "Core 1", "temp3_input": "51000"})}, nil, "coretemp/Core 0", c(50)},
		{"unlabelled k10temp", map[int]hwmonDev{0: dev("k10temp", map[string]string{"temp1_input": "47000"})}, nil, "k10temp/temp1", c(47)},
		{"k8temp", map[int]hwmonDev{0: dev("k8temp", map[string]string{"temp1_input": "40000", "temp2_input": "41000"})}, nil, "k8temp/temp1", c(40)},
		{"a CPU driver beats an earlier board sensor", map[int]hwmonDev{
			0: dev("it87", map[string]string{"temp1_input": "30000"}),
			1: dev("via_cputemp", map[string]string{"temp1_input": "44000"})}, nil, "via_cputemp/temp1", c(44)},
		{"board sensor only", map[int]hwmonDev{0: dev("it87", map[string]string{"temp1_input": "30000"})}, nil, "", nil},
		{"no fallback from an unreadable choice", map[int]hwmonDev{0: dev("k10temp", map[string]string{
			"temp1_label": "Tctl", "temp1_input": "80000", "temp2_label": "Tdie", "temp2_input": ""})},
			map[string]error{hwmonPath(0, "temp2_input"): syscall.ENODATA}, "k10temp/Tdie", nil},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r := hwmonTree(tc.devs)
			for k, v := range tc.errs {
				r.errs[k] = v
			}
			got := Temperatures(r.host(), "")
			assertCPU(t, got, tc.id, tc.tempC, nil)
			if tc.errs != nil {
				assertSensors(t, got, sensor("k10temp/Tctl", c(80)), sensor("k10temp/Tdie", nil))
			}
		})
	}
}

func TestSensorsInDeviceThenIndexOrder(t *testing.T) {
	r := hwmonTree(map[int]hwmonDev{
		10: {name: "amdgpu", parent: "0000:04:00.0", status: "active", attrs: temp("edge", "50000")},
		2: {name: "k10temp", parent: "0000:00:18.3", status: "active", attrs: map[string]string{
			"temp1_label": "Tctl", "temp1_input": "60000", "temp10_label": "Tccd8", "temp10_input": "58000",
			"temp3_label": "Tccd1", "temp3_input": "59000"}},
	})
	assertSensors(t, Temperatures(r.host(), ""), sensor("k10temp/Tctl", c(60)), sensor("k10temp/Tccd1", c(59)),
		sensor("k10temp/Tccd8", c(58)), sensor("amdgpu/edge", c(50)))
}

func TestCPUSensorOverride(t *testing.T) {
	tree := func() *recordingFS {
		devs := amdLaptop()
		devs[4] = hwmonDev{name: "it87", parent: "it87.2608", status: "unsupported", attrs: map[string]string{"temp2_input": "33000"}}
		return hwmonTree(devs)
	}
	listed := []Sensor{sensor("k10temp/Tctl", c(52.1)), sensor("k10temp/Tccd1", c(48)), sensor("amdgpu/edge", c(45)), sensor("it87/temp2", c(33))}

	r := tree()
	got := Temperatures(r.host(), "amdgpu/edge")
	assertCPU(t, got, "amdgpu/edge", c(45), c(100))
	if r.didOpen(hwmonPath(0, "temp1_crit")) {
		t.Error("read k10temp's temp1_crit, but the override chose amdgpu")
	}

	r = tree()
	assertCPU(t, Temperatures(r.host(), "it87/temp2"), "it87/temp2", c(33), nil)

	r = tree()
	got = Temperatures(r.host(), "coretemp/Package id 0")
	assertCPU(t, got, "", nil, nil)
	assertSensors(t, got, listed...)
	if crit := r.openedMatching(isCrit); crit != nil {
		t.Errorf("opened %v with no CPU sensor", crit)
	}

	r = tree()
	assertCPU(t, Temperatures(r.host(), "nvme/Composite"), "", nil, nil)
	if u := r.under(2); !reflect.DeepEqual(u, hwmonPaths(2, "name")) {
		t.Errorf("an override naming nvme opened %v", u)
	}

	r = tree()
	assertCPU(t, Temperatures(r.host(), "acpitz/temp1"), "", nil, nil)
}

func TestTemperatureValues(t *testing.T) {
	for _, tc := range []struct {
		input string
		err   error
		want  *float64
	}{
		{"45000\n", nil, c(45)},
		{"  45000 ", nil, c(45)},
		{"87875", nil, c(87.9)},
		{"-40000", nil, c(-40)},
		{"0", nil, c(0)},
		{"-273151", nil, nil},
		{"", nil, nil},
		{"abc", nil, nil},
		{"45000.5", nil, nil},
		{"99999999999999999999", nil, nil},
		{"45000", syscall.ENODATA, nil},
		{"45000", syscall.EIO, nil},
	} {
		t.Run(fmt.Sprintf("%q %v", tc.input, tc.err), func(t *testing.T) {
			r := hwmonTree(map[int]hwmonDev{0: {name: "k10temp", parent: "0000:00:18.3", status: "active", attrs: temp("Tctl", tc.input)}})
			if tc.err != nil {
				r.errs[hwmonPath(0, "temp1_input")] = tc.err
			}
			got := Temperatures(r.host(), "")
			assertSensors(t, got, sensor("k10temp/Tctl", tc.want))
			assertCPU(t, got, "k10temp/Tctl", tc.want, nil)
		})
	}
}

func TestHwmonGuardsFailClosed(t *testing.T) {
	gpu := sensor("amdgpu/edge", c(40))
	for _, tc := range []struct {
		name   string
		edit   func(*hwmonDev)
		errs   map[string]error
		listed bool
		opened []string
	}{
		{"unreadable name", nil, map[string]error{hwmonPath(0, "name"): syscall.EACCES}, false, []string{"name"}},
		{"empty name", func(d *hwmonDev) { d.name = "\n" }, nil, false, nil},
		{"unreadable device link", nil, map[string]error{hwmonPath(0, "device"): syscall.EIO}, false, []string{"name", "device"}},
		{"device is not a link", nil, map[string]error{hwmonPath(0, "device"): syscall.EINVAL}, false, nil},
		{"unreadable runtime status", nil, map[string]error{hwmonPath(0, RuntimeStatusPath): syscall.EACCES}, false, nil},
		{"no power directory", func(d *hwmonDev) { d.noPower = true }, nil, false, []string{"name", "device", RuntimeStatusPath, "device/power"}},
		{"suspended with a newline", func(d *hwmonDev) { d.status = "suspended\n" }, nil, false, nil},
		{"power without runtime status", func(d *hwmonDev) { d.status = "" }, nil, true, nil},
		{"no device link", func(d *hwmonDev) { d.parent = "" }, nil, true, nil},
		{"active", nil, nil, true, nil},
		{"unsupported", func(d *hwmonDev) { d.status = "unsupported" }, nil, true, nil},
		{"unlistable device", nil, map[string]error{"sys/class/hwmon/hwmon0": syscall.EACCES}, false, nil},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cpu := hwmonDev{name: "k10temp", parent: "0000:00:18.3", status: "active", attrs: temp("Tctl", "50000")}
			if tc.edit != nil {
				tc.edit(&cpu)
			}
			r := hwmonTree(map[int]hwmonDev{0: cpu, 1: {name: "amdgpu", parent: "0000:04:00.0", status: "active", attrs: temp("edge", "40000")}})
			for k, v := range tc.errs {
				r.errs[k] = v
			}
			got := Temperatures(r.host(), "")
			if tc.listed {
				assertSensors(t, got, sensor("k10temp/Tctl", c(50)), gpu)
			} else {
				assertSensors(t, got, gpu)
				for _, p := range r.under(0) {
					if isTemp(p) {
						t.Errorf("opened %s on a device its guards rule out", p)
					}
				}
			}
			if tc.opened != nil {
				if u := r.under(0); !reflect.DeepEqual(u, hwmonPaths(0, tc.opened...)) {
					t.Errorf("opened %v, want %v", u, hwmonPaths(0, tc.opened...))
				}
			}
			if cpu.parent == "" && r.didOpen(hwmonPath(0, RuntimeStatusPath)) {
				t.Error("read a runtime status with no device link")
			}
		})
	}
}

func TestValidSensorID(t *testing.T) {
	for s, want := range map[string]bool{
		"k10temp/Tctl": true, "coretemp/Package id 0": true, "it87/temp3": true,
		"": false, "k10temp": false, "/Tctl": false, "k10temp/": false, " k10temp/Tctl": false,
		"k10temp/Tctl ": false, "k10 temp/Tctl": false, "k10temp/Tctl\n": false,
	} {
		if got := ValidSensorID(s); got != want {
			t.Errorf("ValidSensorID(%q) = %v, want %v", s, got, want)
		}
	}
}

type enumerationReplacementFS struct {
	*recordingFS
	replacement fstest.MapFS
}

func (f *enumerationReplacementFS) Open(name string) (fs.File, error) {
	file, err := f.recordingFS.Open(name)
	if name == hwmonPath(0, RuntimeStatusPath) && f.replacement != nil {
		f.m = f.replacement
		f.replacement = nil
	}
	return file, err
}

func TestTemperatureEnumerationDropsReplacementDuringGuards(t *testing.T) {
	tree := func(name string, generation int64) *recordingFS {
		f := hwmonTree(map[int]hwmonDev{0: {name: name, parent: "cpu", status: "active", attrs: map[string]string{"temp1_label": "Package id 0", "temp1_input": "61000", "temp1_crit": "105000"}}})
		f.m["sys/devices/cpu/hwmon/hwmon0"] = &fstest.MapFile{Mode: fs.ModeDir | 0755, ModTime: time.Unix(generation, 0)}
		return f
	}
	f := &enumerationReplacementFS{recordingFS: tree("coretemp", 1), replacement: tree("nvme", 2).m}
	r := Temperatures(Host{FS: f}, "")
	if len(r.Sensors) != 0 || r.CPUSensor != nil {
		t.Fatalf("listed replacement using old guards: %+v", r)
	}
	for _, attr := range []string{"temp1_label", "temp1_input", "temp1_crit"} {
		if f.didOpen(hwmonPath(0, attr)) {
			t.Fatalf("opened replacement's %s", attr)
		}
	}
}
