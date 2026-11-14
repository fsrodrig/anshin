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

// RuntimeStatusPath is the runtime PM state of a hwmon device's parent, relative to the hwmon device.
const RuntimeStatusPath = "device/power/runtime_status"

// storageSensors are hwmon names whose temperature reads reach the drive: an NVMe read sends a SMART log command
// (and even temp*_max sends Get Features), and drivetemp reads can reset an HDD's spin-down timer.
var storageSensors = map[string]bool{"nvme": true, "drivetemp": true}

var thermalZone = regexp.MustCompile(`^thermal_zone[0-9]+$`)

var tempInput = regexp.MustCompile(`^temp([0-9]+)_input$`)

// cpuDrivers are the hwmon drivers whose sensors measure the CPU itself, rather than the board or a device.
var cpuDrivers = map[string]bool{"coretemp": true, "k10temp": true, "zenpower": true, "k8temp": true, "via_cputemp": true}

// HwmonSkip says why a hwmon device's temperatures must not be read, or "" when they may be. Callers read the
// guards in order (name, then the device link's basename, then the runtime status) and call it after each, with
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

// ValidSensorID reports whether s has the shape of a Sensor ID: "<hwmon name>/<label>", with no surrounding
// whitespace on either part. It checks syntax only; ANSHIN_CPU_SENSOR is validated with it.
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

// Temperatures lists every hwmon temperature the guards allow and chooses the CPU sensor: cpuSensor when it is
// set, else by docs/design.md § Temperatures. Only the CPU sensor's tempN_crit is read.
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

// hwmonName returns the device's name when its guards allow reading it. A guard that can't be read rules the
// device out, except a missing device link, which virtual devices have and which has no runtime PM to wake.
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
		s.TempC = celsius(fs.ReadFile(fsys, prefix+"_input"))
		out = append(out, s)
	}
	slices.SortStableFunc(out, func(a, b hwmonSensor) int { return cmp.Compare(a.index, b.index) })
	return out
}

// celsius converts hwmon millidegrees to °C with one decimal.
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

// chooseCPU returns the index of the CPU sensor, or -1. The choice is by identity, not by readability: falling
// back to another sensor when the chosen one fails would silently change what the CPU temperature means.
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
