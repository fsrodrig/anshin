package recorder

import (
	"encoding/json"
	"reflect"
	"testing"
	"testing/fstest"

	"github.com/fsrodrig/anshin/agent/internal/recorder/probe"
)

func filesystemAt(mount string, pct float64) probe.Filesystem {
	return probe.Filesystem{MajMin: "8:1", MountPoint: mount, Type: "ext4", UsedPct: &pct}
}

func memoryOf(total, available uint64) probe.MemoryReading {
	return probe.MemoryReading{Total: &total, Available: &available}
}

func cpuAt(sensor string, temp float64, crit *float64) probe.TemperatureReading {
	return probe.TemperatureReading{CPUSensor: &sensor, CPUTempC: &temp, CPUCritC: crit}
}

func batteryAt(pct float64, status string) *probe.BatteryReading {
	return &probe.BatteryReading{Pct: &pct, Status: &status}
}

// settled feeds the same Reading for as many Samples as the debounce needs, so the result is the steady Verdict.
func settled(r Reading) *Verdict {
	var j judge
	var v *Verdict
	for range temperatureSamples {
		v = j.observe(r)
	}
	return v
}

func levelOf(v *Verdict) Level {
	if v == nil {
		return "null"
	}
	return v.Level
}

func TestVerdictThresholds(t *testing.T) {
	cases := []struct {
		name string
		r    Reading
		want Level
	}{
		{"filesystem below warn", Reading{Filesystems: []probe.Filesystem{filesystemAt("/", 89.9)}}, LevelOK},
		{"filesystem at warn", Reading{Filesystems: []probe.Filesystem{filesystemAt("/", 90)}}, LevelWarn},
		{"filesystem above warn", Reading{Filesystems: []probe.Filesystem{filesystemAt("/", 90.1)}}, LevelWarn},
		{"filesystem below crit", Reading{Filesystems: []probe.Filesystem{filesystemAt("/", 94.9)}}, LevelWarn},
		{"filesystem at crit", Reading{Filesystems: []probe.Filesystem{filesystemAt("/", 95)}}, LevelCrit},
		{"filesystem above crit", Reading{Filesystems: []probe.Filesystem{filesystemAt("/", 95.1)}}, LevelCrit},

		{"memory above warn", Reading{MemoryReading: memoryOf(1000, 101)}, LevelOK},
		{"memory at warn", Reading{MemoryReading: memoryOf(1000, 100)}, LevelOK},
		{"memory below warn", Reading{MemoryReading: memoryOf(1000, 99)}, LevelWarn},
		{"memory above crit", Reading{MemoryReading: memoryOf(1000, 51)}, LevelWarn},
		{"memory at crit", Reading{MemoryReading: memoryOf(1000, 50)}, LevelWarn},
		{"memory below crit", Reading{MemoryReading: memoryOf(1000, 49)}, LevelCrit},
		{"memory rounds before comparing", Reading{MemoryReading: memoryOf(100000, 9996)}, LevelOK},

		{"cpu with crit below warn", Reading{TemperatureReading: cpuAt("k10temp/Tctl", 94.9, new(100.0))}, LevelOK},
		{"cpu with crit at warn", Reading{TemperatureReading: cpuAt("k10temp/Tctl", 95, new(100.0))}, LevelWarn},
		{"cpu with crit above warn", Reading{TemperatureReading: cpuAt("k10temp/Tctl", 95.1, new(100.0))}, LevelWarn},
		{"cpu with crit below crit", Reading{TemperatureReading: cpuAt("k10temp/Tctl", 99.9, new(100.0))}, LevelWarn},
		{"cpu with crit at crit", Reading{TemperatureReading: cpuAt("k10temp/Tctl", 100, new(100.0))}, LevelCrit},
		{"cpu with crit above crit", Reading{TemperatureReading: cpuAt("k10temp/Tctl", 100.1, new(100.0))}, LevelCrit},
		{"cpu without crit below warn", Reading{TemperatureReading: cpuAt("k10temp/Tctl", 89.9, nil)}, LevelOK},
		{"cpu without crit at warn", Reading{TemperatureReading: cpuAt("k10temp/Tctl", 90, nil)}, LevelWarn},
		{"cpu without crit above warn", Reading{TemperatureReading: cpuAt("k10temp/Tctl", 90.1, nil)}, LevelWarn},
		{"cpu without crit below crit", Reading{TemperatureReading: cpuAt("k10temp/Tctl", 94.9, nil)}, LevelWarn},
		{"cpu without crit at crit", Reading{TemperatureReading: cpuAt("k10temp/Tctl", 95, nil)}, LevelCrit},
		{"cpu without crit above crit", Reading{TemperatureReading: cpuAt("k10temp/Tctl", 95.1, nil)}, LevelCrit},

		{"battery above warn", Reading{Battery: batteryAt(15.1, "Discharging")}, LevelOK},
		{"battery at warn", Reading{Battery: batteryAt(15, "Discharging")}, LevelOK},
		{"battery below warn", Reading{Battery: batteryAt(14.9, "Discharging")}, LevelWarn},
		{"battery above crit", Reading{Battery: batteryAt(7.1, "Discharging")}, LevelWarn},
		{"battery at crit", Reading{Battery: batteryAt(7, "Discharging")}, LevelWarn},
		{"battery below crit", Reading{Battery: batteryAt(6.9, "Discharging")}, LevelCrit},
		{"battery low while charging", Reading{Battery: batteryAt(3, "Charging")}, LevelOK},
		{"battery low while not charging", Reading{Battery: batteryAt(3, "Not charging")}, LevelOK},
	}
	for _, tc := range cases {
		if got := levelOf(settled(tc.r)); got != tc.want {
			t.Errorf("%s: got %s, want %s", tc.name, got, tc.want)
		}
	}
}

func TestVerdictMessagesNameTheInput(t *testing.T) {
	r := Reading{
		Filesystems:        []probe.Filesystem{filesystemAt("/", 91.2)},
		MemoryReading:      memoryOf(1000, 84),
		TemperatureReading: cpuAt("coretemp/Package id 0", 92, nil),
		Battery:            batteryAt(12, "Discharging"),
	}
	want := []Reason{
		{"filesystem_full", LevelWarn, "/ is 91.2 % full"},
		{"memory_low", LevelWarn, "Only 8.4 % of memory is available"},
		{"cpu_hot", LevelWarn, "CPU at 92.0 °C (crit at 95.0 °C)"},
		{"battery_low", LevelWarn, "Battery at 12.0 % and discharging"},
	}
	if got := settled(r); !reflect.DeepEqual(got, &Verdict{LevelWarn, want}) {
		t.Fatalf("got %+v, want %+v", got, want)
	}
}

func TestVerdictWithoutBatteryOrCPUSensorComesFromFilesystemsAndMemory(t *testing.T) {
	fine := Reading{Filesystems: []probe.Filesystem{filesystemAt("/", 40), filesystemAt("/home", 60)}, MemoryReading: memoryOf(1000, 500)}
	if got := settled(fine); !reflect.DeepEqual(got, &Verdict{LevelOK, []Reason{}}) {
		t.Errorf("fine: got %+v, want ok with no Reasons", got)
	}
	firing := Reading{Filesystems: []probe.Filesystem{filesystemAt("/", 40), filesystemAt("/home", 96)}, MemoryReading: memoryOf(1000, 80)}
	want := &Verdict{LevelCrit, []Reason{
		{"filesystem_full", LevelCrit, "/home is 96.0 % full"},
		{"memory_low", LevelWarn, "Only 8.0 % of memory is available"},
	}}
	if got := settled(firing); !reflect.DeepEqual(got, want) {
		t.Errorf("firing: got %+v, want %+v", got, want)
	}
}

func TestVerdictIsNullWhenNothingCanBeJudged(t *testing.T) {
	unavailable := []Reading{
		{},
		{
			Filesystems:        []probe.Filesystem{{MountPoint: "/"}},
			MemoryReading:      probe.MemoryReading{Total: new(uint64(1000))},
			TemperatureReading: probe.TemperatureReading{CPUSensor: new("k10temp/Tctl"), CPUCritC: new(100.0)},
			Battery:            &probe.BatteryReading{Pct: new(5.0)},
		},
		{MemoryReading: memoryOf(0, 0)},
		{MemoryReading: memoryOf(1000, 1001)},
		{Battery: &probe.BatteryReading{Status: new("Discharging")}},
	}
	for i, r := range unavailable {
		if got := settled(r); got != nil {
			t.Errorf("case %d: got %+v, want null", i, got)
		}
	}
	if got := settled(Reading{Battery: batteryAt(80, "Full")}); levelOf(got) != LevelOK {
		t.Errorf("a known battery that isn't discharging is judged: got %s, want ok", levelOf(got))
	}
}

func TestVerdictEncodesEmptyReasonsAndNull(t *testing.T) {
	for _, tc := range []struct {
		v    *Verdict
		want string
	}{
		{settled(Reading{MemoryReading: memoryOf(1000, 500)}), `{"level":"ok","reasons":[]}`},
		{settled(Reading{}), `null`},
	} {
		b, err := json.Marshal(tc.v)
		if err != nil || string(b) != tc.want {
			t.Errorf("got %s %v, want %s", b, err, tc.want)
		}
	}
}

func TestVerdictReasonsAreWorstFirstThenInInputOrder(t *testing.T) {
	r := Reading{
		Filesystems:        []probe.Filesystem{filesystemAt("/", 91), filesystemAt("/home", 97), filesystemAt("/data", 92)},
		MemoryReading:      memoryOf(1000, 40),
		TemperatureReading: cpuAt("k10temp/Tctl", 91, nil),
		Battery:            batteryAt(5, "Discharging"),
	}
	var codes []string
	v := settled(r)
	for _, reason := range v.Reasons {
		codes = append(codes, string(reason.Level)+" "+reason.Code+" "+reason.Message[:5])
	}
	want := []string{
		"crit filesystem_full /home",
		"crit memory_low Only ",
		"crit battery_low Batte",
		"warn filesystem_full / is ",
		"warn filesystem_full /data",
		"warn cpu_hot CPU a",
	}
	if v.Level != LevelCrit || !reflect.DeepEqual(codes, want) {
		t.Fatalf("got %s %q, want crit %q", v.Level, codes, want)
	}
}

// temperatureLevels feeds one CPU temperature per Sample and returns the cpu_hot level after each.
func temperatureLevels(j *judge, sensor string, temps ...*float64) []Level {
	var out []Level
	for _, temp := range temps {
		r := Reading{TemperatureReading: probe.TemperatureReading{CPUSensor: &sensor, CPUTempC: temp}, MemoryReading: memoryOf(1000, 500)}
		out = append(out, levelOf(j.observe(r)))
	}
	return out
}

func temps(vs ...float64) []*float64 {
	out := make([]*float64, len(vs))
	for i, v := range vs {
		out[i] = &v
	}
	return out
}

func TestOneHotSampleDoesNotChangeTheVerdictButFiveInARowDo(t *testing.T) {
	var j judge
	got := temperatureLevels(&j, "k10temp/Tctl", temps(70, 96, 70, 96, 96, 96, 96, 96)...)
	want := []Level{LevelOK, LevelOK, LevelOK, LevelOK, LevelOK, LevelOK, LevelOK, LevelCrit}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v, want %v", got, want)
	}
}

func TestFourHotSamplesThenOneCoolDoNotChangeTheVerdict(t *testing.T) {
	var j judge
	got := temperatureLevels(&j, "k10temp/Tctl", temps(96, 96, 96, 96, 70, 96, 96, 96, 96)...)
	for i, l := range got {
		if l != LevelOK {
			t.Fatalf("Sample %d: got %s, want ok in %v", i, l, got)
		}
	}
}

func TestCritClearsOnlyAfterFiveConsecutiveNonCritSamples(t *testing.T) {
	var j judge
	temperatureLevels(&j, "k10temp/Tctl", temps(96, 96, 96, 96, 96)...)
	got := temperatureLevels(&j, "k10temp/Tctl", temps(92, 92, 92, 92, 96, 92, 92, 92, 92, 92, 70, 70, 70, 70, 70)...)
	want := []Level{
		LevelCrit, LevelCrit, LevelCrit, LevelCrit, LevelCrit,
		LevelCrit, LevelCrit, LevelCrit, LevelCrit, LevelWarn,
		LevelWarn, LevelWarn, LevelWarn, LevelWarn, LevelOK,
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v, want %v", got, want)
	}
}

func TestNullTemperatureExcludesTheInputAndResetsTheDebounce(t *testing.T) {
	var j judge
	temperatureLevels(&j, "k10temp/Tctl", temps(96, 96, 96, 96, 96)...)
	sensor := "k10temp/Tctl"
	r := Reading{TemperatureReading: probe.TemperatureReading{CPUSensor: &sensor}, MemoryReading: memoryOf(1000, 500)}
	if v := j.observe(r); !reflect.DeepEqual(v, &Verdict{LevelOK, []Reason{}}) {
		t.Fatalf("null temperature: got %+v, want ok from memory alone", v)
	}
	got := temperatureLevels(&j, "k10temp/Tctl", temps(96, 96, 96, 96, 96)...)
	want := []Level{LevelOK, LevelOK, LevelOK, LevelOK, LevelCrit}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("after null: got %v, want %v", got, want)
	}

	j = judge{}
	temperatureLevels(&j, "k10temp/Tctl", temps(96, 96, 96, 96)...)
	temperatureLevels(&j, "k10temp/Tctl", nil)
	if got := temperatureLevels(&j, "k10temp/Tctl", temps(96, 96, 96, 96)...); got[3] != LevelOK {
		t.Fatalf("a null Sample must restart the count: got %v", got)
	}
}

func TestCPUSensorChangeResetsTheDebounce(t *testing.T) {
	var j judge
	temperatureLevels(&j, "k10temp/Tctl", temps(96, 96, 96, 96)...)
	got := temperatureLevels(&j, "coretemp/Package id 0", temps(96, 96, 96, 96, 96)...)
	want := []Level{LevelOK, LevelOK, LevelOK, LevelOK, LevelCrit}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v, want %v", got, want)
	}
}

func TestEachRecorderReadIsOneSampleOfTheVerdict(t *testing.T) {
	m := newMachine("", "")
	m.fs["proc/meminfo"] = &fstest.MapFile{Data: []byte("MemTotal: 1000 kB\nMemAvailable: 40 kB\n")}
	r := record(m)
	want := &Verdict{LevelCrit, []Reason{{"memory_low", LevelCrit, "Only 4.0 % of memory is available"}}}
	if got := r.Read().Verdict; !reflect.DeepEqual(got, want) {
		t.Fatalf("got %+v, want %+v", got, want)
	}
	delete(m.fs, "proc/meminfo")
	if got := r.Read().Verdict; got != nil {
		t.Fatalf("nothing to judge: got %+v, want null", got)
	}
}
