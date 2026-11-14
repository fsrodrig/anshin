package probe

import (
	"cmp"
	"errors"
	"io/fs"
	"math"
	"os"
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

type TemperatureSensor struct {
	Sensor
	name, label, dir string
	index            int
}

func Temperatures(h Host, cpuSensor string) TemperatureReading {
	var sensors []TemperatureSensor
	devices := TemperatureDevices(h)
	for _, d := range devices {
		result := ReadTemperatureDevice(h, d, "")
		sensors = append(sensors, result.Sensors...)
	}
	var crit *float64
	if i := ChooseCPU(sensors, cpuSensor); i >= 0 {
		s := sensors[i]
		for _, d := range devices {
			if d.Dir == s.dir {
				crit, _ = readTemperatureAttribute(h.FS, d, "temp"+strconv.Itoa(s.index)+"_crit")
			}
		}
	}
	return TemperatureValues(sensors, ChooseCPU(sensors, cpuSensor), crit)
}

func (s TemperatureSensor) DeviceDir() string { return s.dir }

type TemperatureDevice struct {
	Dir, Name string
	identity  fs.FileInfo
	link      string
}

func TemperatureDevices(h Host) []TemperatureDevice {
	entries, _ := fs.ReadDir(h.FS, "sys/class/hwmon")
	slices.SortFunc(entries, func(a, b fs.DirEntry) int {
		return cmp.Or(cmp.Compare(len(a.Name()), len(b.Name())), cmp.Compare(a.Name(), b.Name()))
	})
	var out []TemperatureDevice
	for _, e := range entries {
		dir := "sys/class/hwmon/" + e.Name()
		if name, ok := hwmonName(h.FS, dir); ok {
			info, err := fs.Stat(h.FS, dir)
			if err != nil {
				continue
			}
			link, _ := fs.ReadLink(h.FS, dir+"/device")
			out = append(out, TemperatureDevice{Dir: dir, Name: name, identity: info, link: path.Base(link)})
		}
	}
	return out
}

func (d TemperatureDevice) Same(other TemperatureDevice) bool {
	if d.identity == nil || other.identity == nil || d.Name != other.Name || d.link != other.link {
		return false
	}
	if d.identity.Sys() != nil && other.identity.Sys() != nil {
		return os.SameFile(d.identity, other.identity)
	}
	return d.identity.Name() == other.identity.Name() && d.identity.ModTime() == other.identity.ModTime()
}

func (d TemperatureDevice) SameIdentity(fsys fs.FS) bool {
	info, err := fs.Stat(fsys, d.Dir)
	return err == nil && d.Same(TemperatureDevice{Name: d.Name, identity: info, link: d.link})
}

func (d TemperatureDevice) valid(fsys fs.FS) bool {
	if d.link != "." {
		status, err := fs.ReadFile(fsys, d.Dir+"/"+RuntimeStatusPath)
		if errors.Is(err, fs.ErrNotExist) {
			_, err = fs.Lstat(fsys, path.Dir(d.Dir+"/"+RuntimeStatusPath))
		} else if err == nil && strings.TrimSpace(string(status)) == "suspended" {
			return false
		}
		if err != nil {
			return false
		}
	}
	// The residual check-to-open window is inherent to path-based fs.FS reads; pinning a directory descriptor needs a Host adapter change.
	return d.SameIdentity(fsys)
}

func (d TemperatureDevice) CouldSelectCPU(override string) bool {
	if override != "" {
		name, _, _ := strings.Cut(override, "/")
		return name == d.Name
	}
	return cpuDrivers[d.Name]
}

type TemperatureDeviceReading struct {
	Sensors  []TemperatureSensor
	Crit     *float64
	Complete bool
}

func readTemperatureAttribute(fsys fs.FS, d TemperatureDevice, attr string) (*float64, bool) {
	if !d.valid(fsys) {
		return nil, false
	}
	b, err := fs.ReadFile(fsys, d.Dir+"/"+attr)
	return celsius(b, err), true
}

func ReadTemperatureDevice(h Host, d TemperatureDevice, cpuID string) TemperatureDeviceReading {
	return ReadTemperatureDeviceSensors(h, d, cpuID, nil)
}

func ReadTemperatureDeviceSensors(h Host, d TemperatureDevice, cpuID string, metadata []TemperatureSensor) TemperatureDeviceReading {
	var r TemperatureDeviceReading
	if !d.valid(h.FS) {
		return r
	}
	if metadata == nil {
		entries, err := fs.ReadDir(h.FS, d.Dir)
		if err != nil {
			return r
		}
		var indices []int
		for _, e := range entries {
			if m := tempInput.FindStringSubmatch(e.Name()); m != nil {
				if n, err := strconv.Atoi(m[1]); err == nil {
					indices = append(indices, n)
				}
			}
		}
		slices.Sort(indices)
		for _, n := range indices {
			prefix := "temp" + strconv.Itoa(n)
			if !d.valid(h.FS) {
				return r
			}
			b, err := fs.ReadFile(h.FS, d.Dir+"/"+prefix+"_label")
			if err != nil && !errors.Is(err, fs.ErrNotExist) {
				return r
			}
			label := strings.TrimSpace(string(b))
			if err != nil || label == "" {
				label = prefix
			}
			s := TemperatureSensor{Sensor: Sensor{ID: d.Name + "/" + label}, name: d.Name, label: label, dir: d.Dir, index: n}
			metadata = append(metadata, s)
		}
	}
	for _, s := range metadata {
		prefix := "temp" + strconv.Itoa(s.index)
		value, ok := readTemperatureAttribute(h.FS, d, prefix+"_input")
		if !ok {
			return r
		}
		s.TempC = value
		r.Sensors = append(r.Sensors, s)
		if s.ID == cpuID {
			var ok bool
			r.Crit, ok = readTemperatureAttribute(h.FS, d, prefix+"_crit")
			cpuID = ""
			if !ok {
				return r
			}
		}
	}
	r.Complete = d.valid(h.FS)
	return r
}

func (s TemperatureSensor) Index() int { return s.index }

func ReadTemperatureCrit(h Host, d TemperatureDevice, s TemperatureSensor) TemperatureDeviceReading {
	if !d.valid(h.FS) {
		return TemperatureDeviceReading{}
	}
	b, err := fs.ReadFile(h.FS, d.Dir+"/temp"+strconv.Itoa(s.index)+"_crit")
	return TemperatureDeviceReading{Crit: celsius(b, err), Complete: (err == nil || errors.Is(err, fs.ErrNotExist)) && d.valid(h.FS)}
}

func TemperatureValues(sensors []TemperatureSensor, chosen int, crit *float64) TemperatureReading {
	var r TemperatureReading
	for _, s := range sensors {
		r.Sensors = append(r.Sensors, s.Sensor)
	}
	if chosen >= 0 {
		id := sensors[chosen].ID
		r.CPUSensor, r.CPUTempC, r.CPUCritC = &id, sensors[chosen].TempC, crit
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
func ChooseCPU(sensors []TemperatureSensor, override string) int {
	if override != "" {
		return slices.IndexFunc(sensors, func(s TemperatureSensor) bool { return s.ID == override })
	}
	amd := func(s TemperatureSensor) bool { return s.name == "k10temp" || s.name == "zenpower" }
	for _, match := range []func(TemperatureSensor) bool{
		func(s TemperatureSensor) bool { return s.name == "coretemp" && s.label == "Package id 0" },
		func(s TemperatureSensor) bool { return amd(s) && s.label == "Tdie" },
		func(s TemperatureSensor) bool { return amd(s) && s.label == "Tctl" },
		func(s TemperatureSensor) bool { return cpuDrivers[s.name] },
	} {
		if i := slices.IndexFunc(sensors, match); i >= 0 {
			return i
		}
	}
	return -1
}
