package probe

import (
	"cmp"
	"errors"
	"io/fs"
	"math"
	"path"
	"regexp"
	"slices"
	"strconv"
	"strings"
)

const RuntimeStatusPath = "device/power/runtime_status"

// An NVMe read sends a SMART log command (even temp*_max sends Get Features),
// and drivetemp reads can reset an HDD's spin-down timer.
var storageSensors = map[string]bool{"nvme": true, "drivetemp": true}

var thermalZone = regexp.MustCompile(`^thermal_zone[0-9]+$`)

var tempInput = regexp.MustCompile(`^temp([0-9]+)_input$`)

var cpuDrivers = map[string]bool{"coretemp": true, "k10temp": true, "zenpower": true, "k8temp": true, "via_cputemp": true}

// Read the guards in order (name, then the device link's basename, then the runtime status) and call it after each, with
// the ones not yet read left empty, so they stop before the next read. capture applies the same rule, so a Capture
// holds every file the probe reads.
func HwmonSkip(name, deviceLink, runtimeStatus string) string {
	switch {
	case storageSensors[name]:
		return "storage sensor (" + name + "): reading it wakes or commands the drive"
	case thermalZone.MatchString(deviceLink):
		return "backed by a thermal zone: probe ignores it, and some (iwlwifi) answer with a firmware command"
	case runtimeStatus == "suspended":
		return "runtime-suspended: reading it would wake the device"
	}
	return ""
}

type Sensor struct {
	ID    string   `json:"sensor"`
	TempC *float64 `json:"temp_c"`
}

type TemperatureReading struct {
	CPUTempC  *float64 `json:"cpu_temp_c"`
	CPUSensor *string  `json:"cpu_sensor"`
	// CPUCritC is the CPU sensor's tempN_crit. Only the Verdict needs it, so it stays off the wire.
	CPUCritC *float64 `json:"-"`
	Sensors  []Sensor `json:"temperatures"`
}

// ANSHIN_CPU_SENSOR validation checks syntax only; sensor availability can change.
func ValidSensorID(s string) bool {
	name, label, ok := strings.Cut(s, "/")
	return ok && name != "" && label != "" && !strings.ContainsAny(name, " \t\n") &&
		strings.TrimSpace(label) == label && !strings.ContainsAny(label, "\t\n")
}

type hwmonSensor struct {
	Sensor
	name, label, dir string
	index            int
}

// Only the CPU sensor's tempN_crit is read, because other sensors do not affect the Verdict.
func Temperatures(h Host, cpuSensor string) TemperatureReading {
	entries, err := fs.ReadDir(h.FS, "sys/class/hwmon")
	if err != nil {
		return TemperatureReading{}
	}
	devices := make([]string, 0, len(entries))
	for _, e := range entries {
		devices = append(devices, e.Name())
	}
	slices.SortFunc(devices, func(a, b string) int { return cmp.Or(cmp.Compare(len(a), len(b)), cmp.Compare(a, b)) })
	var sensors []hwmonSensor
	for _, d := range devices {
		dir := "sys/class/hwmon/" + d
		if name, ok := hwmonName(h.FS, dir); ok {
			sensors = append(sensors, hwmonSensors(h.FS, dir, name)...)
		}
	}
	var r TemperatureReading
	for _, s := range sensors {
		r.Sensors = append(r.Sensors, s.Sensor)
	}
	if i := chooseCPU(sensors, cpuSensor); i >= 0 {
		s := sensors[i]
		r.CPUSensor, r.CPUTempC = &s.ID, s.TempC
		r.CPUCritC = celsius(fs.ReadFile(h.FS, s.dir+"/temp"+strconv.Itoa(s.index)+"_crit"))
	}
	return r
}

// An unreadable guard rules the device out, except a missing device link:
// virtual devices have no runtime PM to wake.
func hwmonName(fsys fs.FS, dir string) (string, bool) {
	b, err := fs.ReadFile(fsys, dir+"/name")
	name := strings.TrimSpace(string(b))
	if err != nil || name == "" || HwmonSkip(name, "", "") != "" {
		return "", false
	}
	target, err := fs.ReadLink(fsys, dir+"/device")
	if errors.Is(err, fs.ErrNotExist) {
		return name, true
	}
	if err != nil {
		return "", false
	}
	link := path.Base(target)
	if HwmonSkip(name, link, "") != "" {
		return "", false
	}
	status, err := fs.ReadFile(fsys, dir+"/"+RuntimeStatusPath)
	if errors.Is(err, fs.ErrNotExist) {
		// A parent without runtime PM has power/ but no runtime_status; without power/ the link is not what it seems.
		if _, err := fs.Lstat(fsys, path.Dir(dir+"/"+RuntimeStatusPath)); err != nil {
			return "", false
		}
	} else if err != nil {
		return "", false
	}
	return name, HwmonSkip(name, link, strings.TrimSpace(string(status))) == ""
}

func hwmonSensors(fsys fs.FS, dir, name string) []hwmonSensor {
	out := hwmonSensorIDs(fsys, dir, name)
	for i := range out {
		s := &out[i]
		s.TempC = celsius(fs.ReadFile(fsys, dir+"/temp"+strconv.Itoa(s.index)+"_input"))
	}
	return out
}

func hwmonSensorIDs(fsys fs.FS, dir, name string) []hwmonSensor {
	entries, err := fs.ReadDir(fsys, dir)
	if err != nil {
		return nil
	}
	var out []hwmonSensor
	for _, e := range entries {
		m := tempInput.FindStringSubmatch(e.Name())
		if m == nil {
			continue
		}
		n, err := strconv.Atoi(m[1])
		if err != nil {
			continue
		}
		prefix := dir + "/temp" + m[1]
		b, err := fs.ReadFile(fsys, prefix+"_label")
		label := strings.TrimSpace(string(b))
		if err != nil || label == "" {
			label = "temp" + m[1]
		}
		s := hwmonSensor{name: name, label: label, dir: dir, index: n}
		s.ID = name + "/" + label
		out = append(out, s)
	}
	slices.SortStableFunc(out, func(a, b hwmonSensor) int { return cmp.Compare(a.index, b.index) })
	return out
}

func celsius(b []byte, err error) *float64 {
	if err != nil {
		return nil
	}
	m, err := strconv.ParseInt(strings.TrimSpace(string(b)), 10, 64)
	if err != nil || m < -273150 {
		return nil
	}
	c := math.Round(float64(m)/100) / 10
	return &c
}

// Falling back when the chosen sensor fails would silently change what the CPU temperature means.
func chooseCPU(sensors []hwmonSensor, override string) int {
	if override != "" {
		return slices.IndexFunc(sensors, func(s hwmonSensor) bool { return s.ID == override })
	}
	amd := func(s hwmonSensor) bool { return s.name == "k10temp" || s.name == "zenpower" }
	for _, match := range []func(hwmonSensor) bool{
		func(s hwmonSensor) bool { return s.name == "coretemp" && s.label == "Package id 0" },
		func(s hwmonSensor) bool { return amd(s) && s.label == "Tdie" },
		func(s hwmonSensor) bool { return amd(s) && s.label == "Tctl" },
		func(s hwmonSensor) bool { return cpuDrivers[s.name] },
	} {
		if i := slices.IndexFunc(sensors, match); i >= 0 {
			return i
		}
	}
	return -1
}
