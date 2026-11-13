package capture

import (
	"regexp"
	"strings"
)

// fixedFiles are copied as they are. Everything else comes from the class directories below.
var fixedFiles = []string{
	"etc/os-release",
	"proc/diskstats",
	"proc/loadavg",
	"proc/meminfo",
	"proc/net/dev",
	"proc/self/mountinfo",
	"proc/stat",
	"proc/swaps",
	"proc/sys/kernel/hostname",
	"proc/sys/kernel/osrelease",
	"proc/uptime",
}

var tempAttr = regexp.MustCompile(`^temp[0-9]+_(input|label|crit)$`)

// supplyAttrs are what docs/design.md § Battery needs. model_name and serial_number identify the battery and are
// never read.
var supplyAttrs = map[string]bool{
	"type": true, "scope": true, "status": true, "capacity": true,
	"energy_now": true, "energy_full": true, "energy_full_design": true,
	"charge_now": true, "charge_full": true, "charge_full_design": true,
	"power_now": true, "current_now": true, "voltage_now": true,
}

const hwmonRuntimeStatus = "device/power/runtime_status"

// storageSensors are hwmon names whose temperature reads reach the drive: an NVMe read sends a SMART log command
// (and even temp*_max sends Get Features), and drivetemp reads can reset an HDD's spin-down timer.
var storageSensors = map[string]bool{"nvme": true, "drivetemp": true}

var thermalZone = regexp.MustCompile(`^thermal_zone[0-9]+$`)

// hwmonSkip says why a hwmon device's temperatures must not be read, or "" when they may be. It is decided from
// name, the device link and runtime_status alone, all of which are read before any temperature.
func hwmonSkip(name, deviceLink, runtimeStatus string) string {
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

// Allowed reports whether a capture may hold path (relative to the Machine's "/"), as a file, directory, link or
// read error. Replay rejects a capture with any other path, and probe's replay test fails if it opens one, so a
// probe that starts reading a new file must add it here and be captured again.
func Allowed(path string) bool {
	for _, f := range fixedFiles {
		if path == f || strings.HasPrefix(f, path+"/") {
			return true
		}
	}
	s := strings.Split(path, "/")
	if len(s) < 3 || s[0] != "sys" || s[1] != "class" {
		return path == "sys" || path == "sys/class"
	}
	for _, c := range s {
		if c == "" || c == "." || c == ".." {
			return false
		}
	}
	switch rest := s[3:]; s[2] {
	case "hwmon":
		switch len(rest) {
		case 0, 1:
			return true
		case 2:
			return rest[1] == "name" || rest[1] == "device" || tempAttr.MatchString(rest[1])
		default:
			p := strings.Join(rest[1:], "/")
			return p == "device/power" || p == hwmonRuntimeStatus
		}
	case "power_supply":
		return len(rest) <= 1 || len(rest) == 2 && supplyAttrs[rest[1]]
	case "net", "block":
		return len(rest) <= 1 || len(rest) == 2 && rest[1] == "device"
	}
	return false
}
