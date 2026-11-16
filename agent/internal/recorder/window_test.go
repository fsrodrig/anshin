package recorder

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"reflect"
	"runtime"
	"testing"
	"testing/fstest"
	"time"
)

// run is a scripted Machine whose counters advance evenly: 25 % CPU on two CPUs, 1000 B/s on eth0 each way and
// 8 sectors a second on sda each way.
type run struct {
	m                          *machine
	rec                        *scriptedRecorder
	busy, idle, rx, sectors    uint64
	core0, core1, idle0, idle1 uint64
	temp                       int
}

func newRun(t *testing.T) *run {
	t.Helper()
	m := newMachine("machine-a", "")
	file := func(p, data string) { m.fs[p] = &fstest.MapFile{Data: []byte(data)} }
	file("proc/meminfo", "MemTotal: 1000 kB\nMemAvailable: 600 kB\n")
	file("proc/loadavg", "0.50 0.40 0.30 1/100 1\n")
	file("sys/class/hwmon/hwmon0/name", "k10temp\n")
	file("sys/class/hwmon/hwmon0/temp1_label", "Tctl\n")
	file("sys/class/power_supply/BAT0/type", "Battery\n")
	file("sys/class/power_supply/BAT0/status", "Discharging\n")
	file("sys/class/power_supply/BAT0/capacity", "64\n")
	file("sys/class/power_supply/BAT0/energy_now", "40000000\n")
	file("sys/class/power_supply/BAT0/energy_full", "62500000\n")
	file("sys/class/power_supply/BAT0/power_now", "8850000\n")
	m.device("net", "eth0")
	m.device("block", "sda")
	r := &run{m: m, temp: 50000}
	r.write()
	r.rec = record(m)
	return r
}

func (r *run) write() {
	r.m.set(fmt.Sprintf("cpu  %d 0 0 %d 0 0 0 0 0 0\ncpu0 %d 0 0 %d 0 0 0 0 0 0\ncpu1 %d 0 0 %d 0 0 0 0 0 0\n",
		r.busy, r.idle, r.core0, r.idle0, r.core1, r.idle1))
	r.m.netdev(fmt.Sprintf("eth0 %d %d", r.rx, r.rx))
	r.m.diskstats(fmt.Sprintf("sda %d %d", r.sectors, r.sectors))
	r.m.fs["sys/class/hwmon/hwmon0/temp1_input"] = &fstest.MapFile{Data: fmt.Appendf(nil, "%d\n", r.temp)}
}

// advance moves both clocks and every counter by d of running time.
func (r *run) advance(d time.Duration) {
	j := uint64(d / (10 * time.Millisecond))
	r.busy, r.idle = r.busy+j/2, r.idle+3*j/2
	r.core0, r.idle0 = r.core0+j*4/10, r.idle0+j*6/10
	r.core1, r.idle1 = r.core1+j/10, r.idle1+j*9/10
	r.rx += uint64(1000 * d.Seconds())
	r.sectors += uint64(8 * d.Seconds())
	r.m.tick(d)
	r.write()
}

func (r *run) sample(d time.Duration) Reading {
	r.advance(d)
	return r.rec.Read()
}

func (r *run) samples(n int) {
	for range n {
		r.sample(SampleInterval)
	}
}

func (r *run) read(c Cursor) Batch { return r.rec.Recorder.Read(c) }

func rows(b Batch) int { return len(b.Window.BoottimeMs) }

func TestWindowEvictsAfterThirtyMinutes(t *testing.T) {
	r := newRun(t)
	r.samples(WindowRows)
	full := r.read(Cursor{})
	if full.Kind != Full || rows(full) != WindowRows || full.Cursor != (Cursor{1, WindowRows + 1}) {
		t.Fatalf("901 Samples: %s with %d rows to %v, want full with 900 to 1.901", full.Kind, rows(full), full.Cursor)
	}
	if b := r.read(Cursor{1, 1}); b.Kind != Full {
		t.Fatalf("evicted seq 1: %s, want full", b.Kind)
	}
	if b := r.read(Cursor{1, 2}); b.Kind != Delta || rows(b) != WindowRows-1 {
		t.Fatalf("oldest seq 2: %s with %d rows, want a delta of 899", b.Kind, rows(b))
	}

	r = newRun(t)
	r.samples(870)
	r.m.suspend(10 * time.Minute)
	r.sample(SampleInterval)
	b := r.read(Cursor{})
	newest := *b.Window.BoottimeMs[rows(b)-1]
	if first := *b.Window.BoottimeMs[0]; newest-first >= WindowSpan.Milliseconds() || newest-first < WindowSpan.Milliseconds()-2000 {
		t.Fatalf("after a 10-minute suspend the Window spans %d ms, want the last 30 minutes", newest-first)
	}
	if b.Window.BoottimeMs[rows(b)-2] != nil {
		t.Fatal("the row before the resume Sample is not the Gap")
	}

	r.m.suspend(31 * time.Minute)
	r.sample(SampleInterval)
	b = r.read(Cursor{})
	if rows(b) != 2 || b.Window.BoottimeMs[0] != nil || b.Window.BoottimeMs[1] == nil {
		t.Fatalf("after a 31-minute suspend: %v, want the Gap then the resume Sample", b.Window.BoottimeMs)
	}

	for range 29 {
		r.sample(time.Minute)
	}
	if b = r.read(Cursor{}); rows(b) != 31 || b.Window.BoottimeMs[0] != nil {
		t.Fatalf("29 minutes after the resume: %d rows, want the Gap, the resume Sample and 29 more", rows(b))
	}
	r.sample(time.Minute)
	if b = r.read(Cursor{}); rows(b) != 30 || b.Window.BoottimeMs[0] == nil {
		t.Fatalf("30 minutes after the resume: %d rows starting %v, want the Gap evicted with the resume Sample", rows(b), b.Window.BoottimeMs[0])
	}
}

func TestCursorRules(t *testing.T) {
	r := newRun(t)
	r.samples(WindowRows + 4)
	const oldest, newest = 6, WindowRows + 5
	for _, tc := range []struct {
		name   string
		cursor Cursor
		kind   Kind
	}{
		{"none", Cursor{}, Full},
		{"foreign epoch", Cursor{2, newest}, Full},
		{"older than the oldest", Cursor{1, oldest - 1}, Full},
		{"seq 0", Cursor{1, 0}, Full},
		{"the oldest", Cursor{1, oldest}, Delta},
		{"middle", Cursor{1, 800}, Delta},
		{"the newest", Cursor{1, newest}, Delta},
		{"future", Cursor{1, newest + 1}, Full},
	} {
		b := r.read(tc.cursor)
		want := WindowRows
		if tc.kind == Delta {
			want = int(newest - tc.cursor.Seq)
		}
		if b.Kind != tc.kind || rows(b) != want || b.Cursor != (Cursor{1, newest}) {
			t.Errorf("%s: %s with %d rows to %v, want %s with %d to 1.%d", tc.name, b.Kind, rows(b), b.Cursor, tc.kind, want, newest)
		}
	}
	got, err := json.Marshal(r.read(Cursor{1, newest}).Window)
	if err != nil {
		t.Fatal(err)
	}
	want := `{"span_s":1800,"boottime_ms":[],"cpu_pct":[],"load_1":[],"mem_used_pct":[],"swap_used_bytes":[],"cpu_temp_c":[],` +
		`"net_rx_bps":[],"net_tx_bps":[],"disk_read_bps":[],"disk_write_bps":[],"battery_pct":[],"battery_power_w":[],"cpu_core_pct":[[],[]]}`
	if string(got) != want {
		t.Fatalf("empty delta = %s\nwant %s", got, want)
	}
}

func TestParseCursor(t *testing.T) {
	for s, want := range map[string]Cursor{
		"9f3c2a71d04e5b68.5":                    {0x9f3c2a71d04e5b68, 5},
		"0000000000000001.0":                    {1, 0},
		"ffffffffffffffff.18446744073709551615": {math.MaxUint64, math.MaxUint64},
	} {
		if got, err := ParseCursor(s); err != nil || got != want {
			t.Errorf("ParseCursor(%q) = %v, %v; want %v", s, got, err, want)
		}
	}
	for _, s := range []string{
		"", ".", "0000000000000001", "0000000000000001.", ".5", "000000000000001.5", "00000000000000001.5",
		"000000000000000A.5", "000000000000000g.5", "+000000000000001.5", "0000000000000001.+5", "0000000000000001.-5",
		"0000000000000001.05", "0000000000000001.5.1", "0000000000000001 .5", " 0000000000000001.5", "0000000000000001.5 ",
		"0000000000000001.18446744073709551616",
	} {
		if got, err := ParseCursor(s); !errors.Is(err, ErrMalformedCursor) {
			t.Errorf("ParseCursor(%q) = %v, %v; want ErrMalformedCursor", s, got, err)
		}
	}
}

func TestEpochChange(t *testing.T) {
	a, b := newRun(t), newRun(t)
	b.rec.Recorder = Open(b.m.host(), b.m.instant(), Config{Epoch: 2})
	a.samples(3)
	b.samples(5)
	from := a.read(Cursor{}).Cursor
	if got := b.read(from); got.Kind != Full || got.Cursor != (Cursor{2, 6}) || rows(got) != 6 {
		t.Fatalf("a Cursor from another run: %s to %v with %d rows, want the whole Window", got.Kind, got.Cursor, rows(got))
	}
	for _, c := range []Cursor{{}, {0x9f3c2a71d04e5b68, 5}, {math.MaxUint64, math.MaxUint64}} {
		if s := c.String(); len(s) < 18 {
			t.Errorf("%v.String() = %q", c, s)
		} else if back, err := ParseCursor(s); err != nil || back != c {
			t.Errorf("ParseCursor(%q) = %v, %v; want %v", s, back, err, c)
		}
	}
	if s := (Cursor{0x9f3c2a71d04e5b68, 5}).String(); s != "9f3c2a71d04e5b68.5" {
		t.Errorf("String = %q", s)
	}
}

func TestSuspendMakesAGap(t *testing.T) {
	r := newRun(t)
	r.samples(2)
	r.m.suspend(999 * time.Millisecond)
	r.sample(SampleInterval)
	if b := r.read(Cursor{}); rows(b) != 4 || b.Window.NetRx[3] == nil {
		t.Fatalf("999 ms of boottime growth: %d rows, net %v; want a fourth Sample with rates and no Gap", rows(b), b.Window.NetRx)
	}

	r.m.suspend(5 * time.Minute)
	// Counters that kept running through the suspend must not become a spike.
	r.rx += 300_000_000
	r.sample(SampleInterval)
	b := r.read(Cursor{})
	if rows(b) != 6 || b.Cursor != (Cursor{1, 6}) {
		t.Fatalf("a 5-minute suspend: %d rows to %v, want the Gap as seq 5 and the resume Sample as seq 6", rows(b), b.Cursor)
	}
	w := b.Window
	encoded, err := json.Marshal(w)
	if err != nil {
		t.Fatal(err)
	}
	var columns map[string]json.RawMessage
	if err := json.Unmarshal(encoded, &columns); err != nil {
		t.Fatal(err)
	}
	for name, raw := range columns {
		if name == "span_s" {
			continue
		}
		var series []json.RawMessage
		if name == "cpu_core_pct" {
			var cores [][]json.RawMessage
			if err := json.Unmarshal(raw, &cores); err != nil || len(cores) != 2 {
				t.Fatalf("cpu_core_pct = %s", raw)
			}
			series = append(cores[0][4:5], cores[1][4])
		} else if err := json.Unmarshal(raw, &series); err != nil {
			t.Fatal(err)
		} else {
			series = series[4:5]
		}
		for _, v := range series {
			if string(v) != "null" {
				t.Errorf("Gap row %s = %s, want null", name, v)
			}
		}
	}
	resume := r.read(Cursor{}).Reading
	if w.CPUPct[5] != nil || w.CPUCorePct[0][5] != nil || w.CPUCorePct[1][5] != nil || w.NetRx[5] != nil || w.NetTx[5] != nil ||
		w.DiskRead[5] != nil || w.DiskWrite[5] != nil || resume.NetInterfaces[0].Rx != nil || resume.Disks[0].Read != nil {
		t.Fatalf("resume Sample has rates across the suspend: %+v", resume)
	}
	if w.MemUsedPct[5] == nil || w.Load1[5] == nil || w.CPUTempC[5] == nil || w.BatteryPct[5] == nil {
		t.Fatalf("resume Sample lost a level: mem %v load %v temp %v battery %v", w.MemUsedPct[5], w.Load1[5], w.CPUTempC[5], w.BatteryPct[5])
	}

	next := r.sample(SampleInterval)
	rate(t, "net rx 2 s after the resume", next.NetRx, 1000)
	rate(t, "disk read 2 s after the resume", next.DiskRead, 4096)
	if next.CPUPct == nil || *next.CPUPct != 25 {
		t.Fatalf("CPU 2 s after the resume = %v, want 25", next.CPUPct)
	}
}

func TestCounterResetIsNullInTheWindow(t *testing.T) {
	r := newRun(t)
	r.samples(2)
	r.rx = 0
	r.sample(SampleInterval)
	r.sample(SampleInterval)
	w := r.read(Cursor{}).Window
	if w.NetRx[3] != nil || w.NetRx[4] == nil || *w.NetRx[4] != 1000 {
		t.Fatalf("net rx across a counter reset = %v, then %v; want null then 1000", w.NetRx[3], w.NetRx[4])
	}
	r.sectors = 0
	r.sample(SampleInterval)
	r.sample(SampleInterval)
	w = r.read(Cursor{}).Window
	if w.DiskRead[5] != nil || w.DiskRead[6] == nil || *w.DiskRead[6] != 4096 {
		t.Fatalf("disk read across a counter reset = %v, then %v; want null then 4096", w.DiskRead[5], w.DiskRead[6])
	}
}

// lastRowIsFlat checks invariant 3 of contract/README.md § Window on the encoded Pulse.
func lastRowIsFlat(t *testing.T, b Batch) {
	t.Helper()
	var flat map[string]any
	reading, _ := json.Marshal(b.Reading)
	if err := json.Unmarshal(reading, &flat); err != nil {
		t.Fatal(err)
	}
	var columns map[string]any
	window, _ := json.Marshal(b.Window)
	if err := json.Unmarshal(window, &columns); err != nil {
		t.Fatal(err)
	}
	battery, _ := flat["battery"].(map[string]any)
	flat["battery_pct"], flat["battery_power_w"] = battery["pct"], battery["power_w"]
	for name, column := range columns {
		if name == "span_s" || name == "boottime_ms" {
			continue
		}
		series, _ := column.([]any)
		if name == "cpu_core_pct" {
			cores, _ := flat[name].([]any)
			for c, s := range series {
				core := s.([]any)
				if !reflect.DeepEqual(core[len(core)-1], cores[c]) {
					t.Errorf("cpu_core_pct[%d]: window %v, flat %v", c, core[len(core)-1], cores[c])
				}
			}
			continue
		}
		if !reflect.DeepEqual(series[len(series)-1], flat[name]) {
			t.Errorf("%s: window %v, flat %v", name, series[len(series)-1], flat[name])
		}
	}
}

func TestFlatFieldsAreTheNewestSample(t *testing.T) {
	r := newRun(t)
	check := func() { t.Helper(); lastRowIsFlat(t, r.read(Cursor{})) }
	check()
	for i := range 20 {
		r.temp += 1234
		switch i {
		case 5:
			r.rx = 0
		case 9:
			r.m.suspend(time.Minute)
		case 13:
			delete(r.m.fs, "proc/meminfo")
		}
		r.sample(SampleInterval)
		check()
	}
	if r.read(Cursor{}).Window.SwapUsedBytes[0] != nil {
		t.Fatal("swap without /proc/swaps is not null")
	}
}

func coreLine(name string, busy, idle uint64) string {
	return fmt.Sprintf("%s %d 0 0 %d 0 0 0 0 0 0\n", name, busy, idle)
}

func cores(r Reading) []any {
	out := []any{}
	for _, c := range r.CPUCorePct {
		if c == nil {
			out = append(out, nil)
		} else {
			out = append(out, int(*c))
		}
	}
	return out
}

func TestPerCoreCPU(t *testing.T) {
	m := newMachine("", coreLine("cpu ", 0, 0)+coreLine("cpu0", 0, 0)+coreLine("cpu1", 0, 0))
	rec := record(m)
	if got := rec.Recorder.Read(Cursor{}).Reading; !reflect.DeepEqual(cores(got), []any{nil, nil}) {
		t.Fatalf("first Sample per-core = %v, want two nulls", cores(got))
	}
	m.tick(2 * time.Second)
	m.set(coreLine("cpu ", 100, 300) + coreLine("cpu0", 80, 120) + coreLine("cpu1", 20, 180))
	if got := cores(rec.Read()); !reflect.DeepEqual(got, []any{40, 10}) {
		t.Fatalf("per-core = %v, want [40 10]", got)
	}
	// Too few jiffies for cpu_pct: the per-core values stay those of cpu_pct's interval.
	m.tick(500 * time.Millisecond)
	m.set(coreLine("cpu ", 150, 350) + coreLine("cpu0", 130, 120) + coreLine("cpu1", 20, 230))
	if got := rec.Read(); *got.CPUPct != 25 || !reflect.DeepEqual(cores(got), []any{40, 10}) {
		t.Fatalf("per-core inside cpu_pct's minimum interval = %v at %v, want [40 10] at 25", cores(got), *got.CPUPct)
	}
	m.tick(2 * time.Second)
	m.set(coreLine("cpu ", 300, 500) + coreLine("cpu0", 280, 120) + coreLine("cpu1", 20, 380))
	if got := cores(rec.Read()); !reflect.DeepEqual(got, []any{100, 0}) {
		t.Fatalf("per-core = %v, want [100 0] measured from the kept baseline", got)
	}
	m.tick(2 * time.Second)
	m.set(coreLine("cpu ", 400, 800) + coreLine("cpu0", 10, 20) + coreLine("cpu1", 120, 480))
	if got := cores(rec.Read()); !reflect.DeepEqual(got, []any{nil, 50}) {
		t.Fatalf("per-core with cpu0 going backwards = %v, want [null 50]", got)
	}
	m.tick(2 * time.Second)
	m.set(coreLine("cpu ", 500, 1100) + coreLine("cpu0", 60, 170) + coreLine("cpu2", 0, 0))
	got := rec.Read()
	if !reflect.DeepEqual(cores(got), []any{25, nil}) || got.CPUPct == nil {
		t.Fatalf("per-core with cpu1 replaced by cpu2 = %v at %v, want [25 null]: cpu2 is outside the start set", cores(got), got.CPUPct)
	}
	if w := rec.Recorder.Read(Cursor{}).Window; len(w.CPUCorePct) != 2 || len(w.CPUCorePct[0]) != 6 {
		t.Fatalf("per-core Window is %d columns, want 2 of 6 rows", len(w.CPUCorePct))
	}
	m.tick(2 * time.Second)
	m.set(coreLine("cpu ", 600, 1200) + coreLine("cpu0", 110, 220))
	if got := rec.Read(); got.CPUPct != nil || !reflect.DeepEqual(cores(got), []any{nil, nil}) {
		t.Fatalf("a CPU going away: per-core %v at %v, want nulls with a null cpu_pct", cores(got), got.CPUPct)
	}

	never := newMachine("", "cpu  0 0 0 0 0 0 0 0\n")
	if got := record(never).Recorder.Read(Cursor{}); got.Reading.CPUCorePct != nil || got.Window.CPUCorePct != nil {
		t.Fatal("per-core is not null on a Machine with no readable per-core line")
	}
}

func TestDiskSummary(t *testing.T) {
	m := newMachine("", "")
	m.device("block", "sda", "sdb", "sdc")
	m.diskstats("sda 0 0", "sdb 0 0")
	rec := record(m)
	if r := rec.Recorder.Read(Cursor{}).Reading; r.DiskRead != nil || r.DiskWrite != nil {
		t.Fatal("the first Sample has a disk summary")
	}
	m.tick(2 * time.Second)
	m.diskstats("sda 4 2", "sdb 1 2")
	r := rec.Read()
	rate(t, "disk read", r.DiskRead, 1280)
	rate(t, "disk write", r.DiskWrite, 1024)

	m.tick(2 * time.Second)
	m.diskstats("sda 0 4", "sdb 1 4")
	r = rec.Read()
	null(t, "disk read across sda's reset", r.DiskRead)
	rate(t, "disk write", r.DiskWrite, 1024)

	m.tick(2 * time.Second)
	m.diskstats("sda 0 4", "sdb 1 4", "sdc 0 0")
	r = rec.Read()
	null(t, "disk read as sdc appears", r.DiskRead)
	null(t, "disk write as sdc appears", r.DiskWrite)

	m.tick(2 * time.Second)
	m.diskstats("sda 0 4", "sdb 1 4")
	r = rec.Read()
	null(t, "disk read as sdc goes", r.DiskRead)

	m.tick(2 * time.Second)
	m.diskstats("sda 8 4", "sdb 1 4")
	r = rec.Read()
	rate(t, "disk read", r.DiskRead, 2048)
	rate(t, "disk write", r.DiskWrite, 0)

	m.tick(2 * time.Second)
	m.diskstats()
	if r := rec.Read(); r.DiskRead != nil || r.DiskWrite != nil {
		t.Fatal("a disk summary with no device-backed disk")
	}
}

func hotRun(t *testing.T) *run {
	r := newRun(t)
	r.temp = 97000
	r.write()
	r.rec = record(r.m)
	return r
}

func cpuHot(r Reading) Level {
	if r.Verdict == nil {
		return ""
	}
	for _, reason := range r.Verdict.Reasons {
		if reason.Code == "cpu_hot" {
			return reason.Level
		}
	}
	return LevelOK
}

func TestVerdictIsJudgedOncePerSample(t *testing.T) {
	r := hotRun(t)
	for i := range 5 {
		if i > 0 {
			r.sample(SampleInterval)
		}
		want := LevelOK
		if i == 4 {
			want = LevelCrit
		}
		for range 100 {
			if got := cpuHot(r.read(Cursor{}).Reading); got != want {
				t.Fatalf("hot Sample %d read again: cpu_hot %q, want %q", i+1, got, want)
			}
		}
	}

	m := newMachine("", "")
	m.fs["proc/meminfo"] = &fstest.MapFile{Data: []byte("MemTotal: 1000 kB\nMemAvailable: 40 kB\n")}
	rec := record(m)
	want := &Verdict{LevelCrit, []Reason{{"memory_low", LevelCrit, "Only 4.0 % of memory is available"}}}
	if got := rec.Recorder.Read(Cursor{}).Reading.Verdict; !reflect.DeepEqual(got, want) {
		t.Fatalf("got %+v, want %+v", got, want)
	}
	delete(m.fs, "proc/meminfo")
	if got := rec.Recorder.Read(Cursor{}).Reading.Verdict; !reflect.DeepEqual(got, want) {
		t.Fatalf("a Read measured: got %+v, want the Sample's %+v", got, want)
	}
	if got := rec.Read().Verdict; got != nil {
		t.Fatalf("nothing to judge: got %+v, want null", got)
	}
}

func TestGapResetsTheTemperatureDebounce(t *testing.T) {
	r := hotRun(t)
	r.samples(4)
	if got := cpuHot(r.read(Cursor{}).Reading); got != LevelCrit {
		t.Fatalf("before the suspend: %q, want crit", got)
	}
	r.m.suspend(5 * time.Minute)
	for i := range 5 {
		want := LevelOK
		if i == 4 {
			want = LevelCrit
		}
		if got := cpuHot(r.sample(SampleInterval)); got != want {
			t.Fatalf("hot Sample %d after the Gap: %q, want %q", i+1, got, want)
		}
	}
}

func TestWindowStorage(t *testing.T) {
	r := newRun(t)
	v := reflect.ValueOf(&r.rec.ring).Elem()
	size := func() (bytes int) {
		for i := range v.NumField() {
			if f := v.Field(i); f.Kind() == reflect.Slice {
				bytes += f.Cap() * int(f.Type().Elem().Size())
			}
		}
		return bytes
	}
	backing := func() []uintptr {
		var out []uintptr
		for i := range v.NumField() {
			if f := v.Field(i); f.Kind() == reflect.Slice {
				out = append(out, f.Pointer())
			}
		}
		return out
	}
	before := backing()
	r.samples(2000)
	if got, want := size(), WindowRows*(64+2); got != want {
		t.Fatalf("the Window holds %d bytes, want %d: 64 per row plus one per CPU", got, want)
	}
	if after := backing(); !reflect.DeepEqual(before, after) {
		t.Fatal("a column was reallocated after Open")
	}
}

func TestWaitReturnsOnTheNextSample(t *testing.T) {
	r := newRun(t)
	r.samples(2)
	at := r.read(Cursor{}).Cursor
	done := make(chan Batch, 1)
	go func() {
		b, err := r.rec.Wait(context.Background(), at)
		if err != nil {
			t.Error(err)
		}
		done <- b
	}()
	select {
	case <-done:
		t.Fatal("Wait at the newest row returned before a new Sample")
	case <-time.After(50 * time.Millisecond):
	}
	r.sample(SampleInterval)
	select {
	case b := <-done:
		if b.Kind != Delta || rows(b) != 1 || b.Cursor != (Cursor{1, at.Seq + 1}) {
			t.Fatalf("Wait: %s with %d rows to %v, want a delta of the one new row", b.Kind, rows(b), b.Cursor)
		}
	case <-time.After(time.Second):
		t.Fatal("Wait did not return on the next Sample")
	}
}

func TestWaitReturnsAtOnceForAnInvalidOrBehindCursor(t *testing.T) {
	r := newRun(t)
	r.samples(2)
	for _, tc := range []struct {
		cursor Cursor
		kind   Kind
		rows   int
	}{{Cursor{}, Full, 3}, {Cursor{2, 3}, Full, 3}, {Cursor{1, 2}, Delta, 1}, {Cursor{1, 9}, Full, 3}} {
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		b, err := r.rec.Wait(ctx, tc.cursor)
		cancel()
		if err != nil || b.Kind != tc.kind || rows(b) != tc.rows {
			t.Errorf("Wait(%v) = %s with %d rows, %v; want %s with %d at once", tc.cursor, b.Kind, rows(b), err, tc.kind, tc.rows)
		}
	}
}

func TestWaitStopsWhenCancelled(t *testing.T) {
	r := newRun(t)
	at := r.read(Cursor{}).Cursor
	baseline := runtime.NumGoroutine()
	ctx, cancel := context.WithCancel(context.Background())
	errs := make(chan error, 100)
	for range 100 {
		go func() {
			_, err := r.rec.Wait(ctx, at)
			errs <- err
		}()
	}
	time.Sleep(20 * time.Millisecond)
	cancel()
	deadline := time.After(time.Second)
	for range 100 {
		select {
		case err := <-errs:
			if !errors.Is(err, context.Canceled) {
				t.Fatalf("Wait = %v, want context.Canceled", err)
			}
		case <-deadline:
			t.Fatal("a cancelled Wait did not return within 1 s")
		}
	}
	for start := time.Now(); runtime.NumGoroutine() > baseline; time.Sleep(time.Millisecond) {
		if time.Since(start) > time.Second {
			t.Fatalf("%d goroutines, want the %d from before the waiters", runtime.NumGoroutine(), baseline)
		}
	}
}

func TestCapacityEvictsGapWithResumeSample(t *testing.T) {
	r := newRun(t)
	r.m.suspend(31 * time.Minute)
	r.sample(SampleInterval)
	r.samples(WindowRows - 2)
	before := r.read(Cursor{})
	if rows(before) != WindowRows || before.Window.BoottimeMs[0] != nil {
		t.Fatal("expected a full Window starting with a Gap")
	}
	resume := *before.Window.BoottimeMs[1]
	r.sample(SampleInterval)
	after := r.read(Cursor{})
	if rows(after) != WindowRows-1 || after.Window.BoottimeMs[0] == nil || *after.Window.BoottimeMs[0] != resume+SampleInterval.Milliseconds() {
		t.Fatal("capacity retained the resume Sample without its Gap")
	}
	for _, seq := range []uint64{2, 3} {
		if b := r.read(Cursor{1, seq}); b.Kind != Full {
			t.Fatalf("evicted seq %d still accepts a delta", seq)
		}
	}
	if b := r.read(Cursor{1, 4}); b.Kind != Delta || rows(b) != WindowRows-2 {
		t.Fatal("oldest retained Cursor did not return the contiguous suffix")
	}
}

func TestSuspendAtThresholdMakesAGap(t *testing.T) {
	r := newRun(t)
	r.samples(2)
	r.m.suspend(SuspendThreshold)
	r.sample(SampleInterval)
	b := r.read(Cursor{})
	if rows(b) != 5 || b.Window.BoottimeMs[3] != nil || b.Window.BoottimeMs[4] == nil || b.Window.NetRx[4] != nil {
		t.Fatal("exactly SuspendThreshold must insert a Gap and reset resume rates", b.Window)
	}
}
