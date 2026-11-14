package probe

import (
	"encoding/json"
	"io/fs"
	"reflect"
	"slices"
	"strings"
	"syscall"
	"testing"
	"testing/fstest"
)

// sysfs serves a MapFS, fails the listed paths with an errno and records every path it is asked for, so a test can
// prove what Battery never read.
type sysfs struct {
	fstest.MapFS
	errs   map[string]syscall.Errno
	opened *[]string
}

func (s sysfs) check(op, name string) error {
	if s.opened != nil {
		*s.opened = append(*s.opened, name)
	}
	if e, ok := s.errs[name]; ok {
		return &fs.PathError{Op: op, Path: name, Err: e}
	}
	return nil
}

func (s sysfs) Open(name string) (fs.File, error) {
	if err := s.check("open", name); err != nil {
		return nil, err
	}
	return s.MapFS.Open(name)
}

func (s sysfs) ReadFile(name string) ([]byte, error) {
	if err := s.check("read", name); err != nil {
		return nil, err
	}
	return s.MapFS.ReadFile(name)
}

func (s sysfs) ReadDir(name string) ([]fs.DirEntry, error) {
	if err := s.check("readdir", name); err != nil {
		return nil, err
	}
	return s.MapFS.ReadDir(name)
}

func (s sysfs) Stat(name string) (fs.FileInfo, error) {
	if err := s.check("stat", name); err != nil {
		return nil, err
	}
	return s.MapFS.Stat(name)
}

type supplies map[string]map[string]string

// tree lays the supplies out the way sysfs does: every class entry is a symlink into /sys/devices, and every value
// ends in a newline.
func tree(s supplies) fstest.MapFS {
	m := fstest.MapFS{"sys/class/power_supply": &fstest.MapFile{Mode: fs.ModeDir | 0o755}}
	for name, attrs := range s {
		real := "sys/devices/platform/" + strings.ReplaceAll(name, ":", "_") + "/power_supply/" + name
		m["sys/class/power_supply/"+name] = &fstest.MapFile{Mode: fs.ModeSymlink, Data: []byte("../../" + strings.TrimPrefix(real, "sys/"))}
		m[real] = &fstest.MapFile{Mode: fs.ModeDir | 0o755}
		for k, v := range attrs {
			m[real+"/"+k] = &fstest.MapFile{Data: []byte(v + "\n")}
		}
	}
	return m
}

func battery(t *testing.T, s supplies, errs map[string]syscall.Errno) (*BatteryReading, []string) {
	t.Helper()
	var opened []string
	got := Battery(Host{FS: sysfs{tree(s), errs, &opened}})
	if _, err := json.Marshal(got); err != nil {
		t.Fatalf("json.Marshal(%+v): %v", got, err)
	}
	return got, opened
}

func assertBattery(t *testing.T, got, want *BatteryReading) {
	t.Helper()
	if !reflect.DeepEqual(got, want) {
		g, _ := json.Marshal(got)
		w, _ := json.Marshal(want)
		t.Fatalf("Battery = %s\nwant      %s", g, w)
	}
}

// Synthetic laptop tree using the energy_* family, not a Capture from a real Machine (the real one is #30).
func syntheticLaptopEnergy() supplies {
	return supplies{
		"BAT0": {"type": "Battery", "status": "Discharging", "capacity": "81", "energy_now": "40500000",
			"energy_full": "50000000", "energy_full_design": "54000000", "power_now": "8100000", "voltage_now": "11400000"},
		"AC":                          {"type": "Mains", "online": "0"},
		"ucsi-source-psy-USBC000:001": {"type": "USB", "online": "0"},
	}
}

// Synthetic laptop tree using the charge_* family and no capacity, not a Capture from a real Machine.
func syntheticLaptopCharge(status, current string) supplies {
	return supplies{
		"AC": {"type": "Mains", "online": "1"},
		"BAT1": {"type": "Battery", "status": status, "charge_now": "3000000", "charge_full": "4000000",
			"charge_full_design": "4400000", "current_now": current, "voltage_now": "11400000"},
	}
}

// Synthetic two-battery tree, laid out like a ThinkPad with an internal and a removable battery, not a Capture from
// a real Machine.
func syntheticThinkPad() supplies {
	return supplies{
		"BAT0": {"type": "Battery", "status": "Unknown", "capacity": "75", "energy_now": "18000000",
			"energy_full": "24000000", "energy_full_design": "24050000", "power_now": "0"},
		"BAT1": {"type": "Battery", "status": "Discharging", "capacity": "40", "energy_now": "8000000",
			"energy_full": "20000000", "energy_full_design": "23200000", "power_now": "7000000"},
		"AC": {"type": "Mains", "online": "0"},
	}
}

func TestSyntheticLaptopEnergy(t *testing.T) {
	got, opened := battery(t, syntheticLaptopEnergy(), nil)
	row := BatteryRow{Name: "BAT0", Pct: new(81.0), Status: new("Discharging"), PowerW: new(8.1), Health: new(92.6)}
	assertBattery(t, got, &BatteryReading{Pct: new(81.0), Status: new("Discharging"), PowerW: new(8.1), TimeLeft: new(uint64(18000)), Batteries: []BatteryRow{row}})
	for _, p := range opened {
		if strings.HasPrefix(p, "sys/class/power_supply/AC/") && p != "sys/class/power_supply/AC/type" {
			t.Errorf("read %s of a Mains supply", p)
		}
	}

	tr := syntheticLaptopEnergy()
	tr["BAT0"]["capacity"] = "79"
	got, _ = battery(t, tr, nil)
	if got.Pct == nil || *got.Pct != 79 || *got.Batteries[0].Pct != 79 {
		t.Fatalf("capacity 79 with energy 40.5/50 Wh: pct %v; want the driver's capacity, not the ratio", got.Pct)
	}
}

func TestSyntheticLaptopCharge(t *testing.T) {
	got, _ := battery(t, syntheticLaptopCharge("Charging", "1500000"), nil)
	row := BatteryRow{Name: "BAT1", Pct: new(75.0), Status: new("Charging"), PowerW: new(17.1), Health: new(90.9)}
	assertBattery(t, got, &BatteryReading{Pct: new(75.0), Status: new("Charging"), PowerW: new(17.1), Batteries: []BatteryRow{row}})

	got, _ = battery(t, syntheticLaptopCharge("Discharging", "-2000000"), nil)
	row = BatteryRow{Name: "BAT1", Pct: new(75.0), Status: new("Discharging"), PowerW: new(22.8), Health: new(90.9)}
	assertBattery(t, got, &BatteryReading{Pct: new(75.0), Status: new("Discharging"), PowerW: new(22.8), TimeLeft: new(uint64(5400)), Batteries: []BatteryRow{row}})
}

func TestSyntheticPeripheralBatteriesAreExcluded(t *testing.T) {
	peripherals := supplies{
		"hid-00:00:00:00:00:00-battery":           {"type": "Battery", "scope": "Device", "capacity": "50", "status": "Discharging"},
		"hidpp_battery_0":                         {"type": "Battery", "scope": "Device", "capacity": "20", "status": "Discharging"},
		"ps-controller-battery-00:00:00:00:00:00": {"type": "Battery", "scope": "Device", "capacity": "90", "status": "Charging"},
	}
	tr := syntheticLaptopEnergy()
	for k, v := range peripherals {
		tr[k] = v
	}
	got, opened := battery(t, tr, nil)
	want, _ := battery(t, syntheticLaptopEnergy(), nil)
	assertBattery(t, got, want)
	for name := range peripherals {
		base := "sys/class/power_supply/" + name + "/"
		for _, p := range opened {
			if strings.HasPrefix(p, base) && p != base+"type" && p != base+"scope" {
				t.Errorf("read %s of a peripheral; only type and scope may be read", p)
			}
		}
	}

	if got, _ := battery(t, peripherals, nil); got != nil {
		t.Fatalf("peripherals only: Battery = %+v, want nil (no system battery)", got)
	}
}

func TestSyntheticNoBattery(t *testing.T) {
	if got, _ := battery(t, supplies{}, nil); got != nil {
		t.Errorf("empty power_supply class: %+v, want nil", got)
	}
	if got := Battery(Host{FS: fstest.MapFS{}}); got != nil {
		t.Errorf("no power_supply class: %+v, want nil", got)
	}
	if got, _ := battery(t, supplies{"AC": {"type": "Mains", "online": "1"}}, nil); got != nil {
		t.Errorf("AC only: %+v, want nil", got)
	}
}

func TestBatteryParsing(t *testing.T) {
	ratio := map[string]string{"energy_now": "30000000", "energy_full": "60000000"}
	with := func(base map[string]string, kv ...string) map[string]string {
		m := map[string]string{"type": "Battery"}
		for k, v := range base {
			m[k] = v
		}
		for i := 0; i < len(kv); i += 2 {
			m[kv[i]] = kv[i+1]
		}
		return m
	}
	for _, tc := range []struct {
		name  string
		attrs map[string]string
		row   BatteryRow
		time  *uint64
	}{
		{"capacity above 100 falls back to the ratio", with(ratio, "capacity", "101"), BatteryRow{Pct: new(50.0)}, nil},
		{"negative capacity falls back", with(ratio, "capacity", "-1"), BatteryRow{Pct: new(50.0)}, nil},
		{"non-numeric capacity falls back", with(ratio, "capacity", "abc"), BatteryRow{Pct: new(50.0)}, nil},
		{"capacity out of int64 range falls back", with(ratio, "capacity", "9223372036854775808"), BatteryRow{Pct: new(50.0)}, nil},
		{"now above full is null", with(nil, "energy_now", "70000000", "energy_full", "60000000"), BatteryRow{}, nil},
		{"negative energy is null", with(nil, "energy_now", "-5", "energy_full", "60000000"), BatteryRow{}, nil},
		{"zero design capacity gives no health", with(nil, "energy_full", "60000000", "energy_full_design", "0"), BatteryRow{}, nil},
		{"health above 100 is kept", with(nil, "energy_full", "52000000", "energy_full_design", "50000000"), BatteryRow{Health: new(104.0)}, nil},
		{"negative power_now is a magnitude", with(nil, "status", "Discharging", "energy_now", "10000000", "power_now", "-5000000"),
			BatteryRow{Status: new("Discharging"), PowerW: new(5.0)}, new(uint64(7200))},
		{"zero power while discharging has no time left", with(nil, "status", "Discharging", "energy_now", "10000000", "power_now", "0"),
			BatteryRow{Status: new("Discharging"), PowerW: new(0.0)}, nil},
		{"no power_now uses current times voltage", with(nil, "status", "Discharging", "energy_now", "12000000", "current_now", "2000000", "voltage_now", "12000000"),
			BatteryRow{Status: new("Discharging"), PowerW: new(24.0)}, new(uint64(1800))},
		{"zero voltage gives no power", with(nil, "current_now", "2000000", "voltage_now", "0"), BatteryRow{}, nil},
		{"empty status is null", with(nil, "status", ""), BatteryRow{}, nil},
		{"blank status is null", with(nil, "status", "  "), BatteryRow{}, nil},
		{"an unknown status is kept verbatim", with(nil, "status", "Weird"), BatteryRow{Status: new("Weird")}, nil},
		{"whitespace around values is trimmed", with(nil, "status", " Full ", "capacity", "  81  "), BatteryRow{Status: new("Full"), Pct: new(81.0)}, nil},
		{"not charging has no time left", with(nil, "status", "Not charging", "energy_now", "10000000", "power_now", "5000000"),
			BatteryRow{Status: new("Not charging"), PowerW: new(5.0)}, nil},
		{"an unrepresentable time left is null", with(nil, "status", "Discharging", "energy_now", "9223372036854775807", "power_now", "1"),
			BatteryRow{Status: new("Discharging"), PowerW: new(0.0)}, nil},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, _ := battery(t, supplies{"BAT0": tc.attrs}, nil)
			tc.row.Name = "BAT0"
			assertBattery(t, got, &BatteryReading{Pct: tc.row.Pct, Status: tc.row.Status, PowerW: tc.row.PowerW, TimeLeft: tc.time, Batteries: []BatteryRow{tc.row}})
		})
	}
}

func TestMultipleBatteries(t *testing.T) {
	got, _ := battery(t, syntheticThinkPad(), nil)
	assertBattery(t, got, &BatteryReading{Pct: new(59.1), Status: new("Discharging"), PowerW: new(7.0), TimeLeft: new(uint64(13371)), Batteries: []BatteryRow{
		{Name: "BAT0", Pct: new(75.0), Status: new("Unknown"), PowerW: new(0.0), Health: new(99.8)},
		{Name: "BAT1", Pct: new(40.0), Status: new("Discharging"), PowerW: new(7.0), Health: new(86.2)},
	}})

	mixed := supplies{
		"BAT0": {"type": "Battery", "status": "Discharging", "energy_now": "10000000", "energy_full": "20000000", "power_now": "5000000"},
		"BAT1": {"type": "Battery", "status": "Discharging", "charge_now": "1000000", "charge_full": "2000000", "current_now": "1000000", "voltage_now": "10000000"},
	}
	got, _ = battery(t, mixed, nil)
	if got.Pct != nil || got.TimeLeft != nil || got.Status == nil || *got.Status != "Discharging" || got.PowerW == nil || *got.PowerW != 15 {
		t.Errorf("energy and charge batteries: %+v; want pct and time null, Discharging at 15 W", got)
	}
	if len(got.Batteries) != 2 || *got.Batteries[0].Pct != 50 || *got.Batteries[1].Pct != 50 {
		t.Errorf("each row keeps its own percent: %+v", got.Batteries)
	}

	for _, tc := range []struct {
		a, b   string
		status *string
		power  bool
	}{
		{"Charging", "Discharging", nil, false},
		{"Full", "Full", new("Full"), true},
		{"Full", "Not charging", nil, true},
		{"Charging", "Full", new("Charging"), true},
	} {
		tr := syntheticThinkPad()
		tr["BAT0"]["status"], tr["BAT1"]["status"] = tc.a, tc.b
		got, _ := battery(t, tr, nil)
		if !reflect.DeepEqual(got.Status, tc.status) || (got.PowerW != nil) != tc.power || got.TimeLeft != nil && tc.b != "Discharging" {
			t.Errorf("%s and %s: status %v, power %v, time %v; want status %v, power set %v", tc.a, tc.b, got.Status, got.PowerW, got.TimeLeft, tc.status, tc.power)
		}
	}
}

func TestBatteryUnreadable(t *testing.T) {
	got, _ := battery(t, syntheticLaptopEnergy(), map[string]syscall.Errno{"sys/class/power_supply": syscall.EACCES})
	assertBattery(t, got, &BatteryReading{})

	bat0 := BatteryRow{Name: "BAT0", Pct: new(81.0), Status: new("Discharging"), PowerW: new(8.1), Health: new(92.6)}
	for _, p := range []string{"sys/class/power_supply/hid-00:00:00:00:00:00-battery/type", "sys/class/power_supply/hid-00:00:00:00:00:00-battery/scope"} {
		tr := syntheticLaptopEnergy()
		tr["hid-00:00:00:00:00:00-battery"] = map[string]string{"type": "Battery", "scope": "Device", "capacity": "50"}
		got, opened := battery(t, tr, map[string]syscall.Errno{p: syscall.EACCES})
		assertBattery(t, got, &BatteryReading{Batteries: []BatteryRow{bat0}})
		if slices.Contains(opened, "sys/class/power_supply/hid-00:00:00:00:00:00-battery/capacity") {
			t.Errorf("EACCES on %s: an unclassified supply's capacity was read", p)
		}
	}

	got, _ = battery(t, syntheticLaptopEnergy(), map[string]syscall.Errno{"sys/class/power_supply/BAT0/type": syscall.EACCES})
	assertBattery(t, got, &BatteryReading{})

	got, _ = battery(t, syntheticLaptopEnergy(), map[string]syscall.Errno{"sys/class/power_supply/BAT0/status": syscall.EIO})
	assertBattery(t, got, &BatteryReading{Pct: new(81.0), PowerW: new(8.1), Batteries: []BatteryRow{{Name: "BAT0", Pct: new(81.0), PowerW: new(8.1), Health: new(92.6)}}})

	got, _ = battery(t, syntheticLaptopEnergy(), map[string]syscall.Errno{"sys/class/power_supply/BAT0/energy_now": syscall.EIO})
	assertBattery(t, got, &BatteryReading{Pct: new(81.0), Status: new("Discharging"), PowerW: new(8.1), Batteries: []BatteryRow{bat0}})
}
