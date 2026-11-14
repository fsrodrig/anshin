package probe

import (
	"math"
	"reflect"
	"testing"
	"testing/fstest"
)

func hostFile(name, data string) Host {
	return Host{FS: fstest.MapFS{name: &fstest.MapFile{Data: []byte(data)}}}
}
func equalPtr[T comparable](t *testing.T, got *T, want T) {
	t.Helper()
	if got == nil || *got != want {
		t.Fatalf("got %v, want %v", got, want)
	}
}

func TestMemory(t *testing.T) {
	m := Memory(hostFile("proc/meminfo", "MemTotal: 1000 kB\nMemFree: 100 kB\nMemAvailable: 750 kB\n"))
	equalPtr(t, m.Total, uint64(1024000))
	equalPtr(t, m.Available, uint64(768000))
	equalPtr(t, m.Used, uint64(256000))
	equalPtr(t, m.Pct, 25.0)
	for _, input := range []string{"", "MemTotal: -1 kB", "MemTotal: 9007199254740992 kB", "MemTotal: 18446744073709551615 kB", "MemTotal: bad kB", "MemTotal: 1 MB"} {
		if m := Memory(hostFile("proc/meminfo", input)); m.Total != nil || m.Used != nil || m.Pct != nil {
			t.Fatalf("%q: %+v", input, m)
		}
	}
	for _, input := range []string{"MemTotal: 10 kB", "MemTotal: 10 kB\nMemAvailable: bad kB", "MemTotal: 10 kB\nMemAvailable: 11 kB"} {
		m := Memory(hostFile("proc/meminfo", input))
		equalPtr(t, m.Total, uint64(10240))
		if m.Used != nil || m.Pct != nil {
			t.Fatal(m)
		}
	}
	m = Memory(hostFile("proc/meminfo", "MemTotal: 0 kB\nMemAvailable: 0 kB"))
	equalPtr(t, m.Used, uint64(0))
	if m.Pct != nil {
		t.Fatal(m)
	}
	m = Memory(hostFile("proc/meminfo", "MemTotal: 10 kB\nMemAvailable: 10 kB"))
	equalPtr(t, m.Pct, 0.0)
	m = Memory(hostFile("proc/meminfo", "MemTotal: bad kB\nMemAvailable: 10 kB"))
	equalPtr(t, m.Available, uint64(10240))
	if m := Memory(Host{FS: fstest.MapFS{}}); !reflect.DeepEqual(m, MemoryReading{}) {
		t.Fatal(m)
	}
}

const swapHeader = "Filename Type Size Used Priority\n"

// Synthetic no-swap input, not a Capture from a real Machine.
func TestSyntheticNoSwap(t *testing.T) {
	s := Swap(hostFile("proc/swaps", swapHeader))
	if s.Total != nil || s.Used != nil || s.Entries != nil {
		t.Fatal(s)
	}
}

func TestSwap(t *testing.T) {
	s := Swap(hostFile("proc/swaps", swapHeader+"/dev/zram0 partition 100 25 100\n/swapfile file 200 0 -2\n"))
	equalPtr(t, s.Total, uint64(307200))
	equalPtr(t, s.Used, uint64(25600))
	if len(s.Entries) != 2 || !s.Entries[0].Zram || s.Entries[1].Zram {
		t.Fatal(s)
	}
	equalPtr(t, s.Entries[1].Used, uint64(0))
	for _, name := range []string{"/dev/zram", "/dev/zramx", "/tmp/zram0", "/swapfile"} {
		if isZram(name) {
			t.Fatal(name)
		}
	}
	for _, input := range []string{"", "bad header", swapHeader + "bad row", swapHeader + "/swap file 18446744073709551615 bad 0"} {
		s := Swap(hostFile("proc/swaps", input))
		if s.Total != nil || s.Used != nil {
			t.Fatal(s)
		}
	}
	s = Swap(hostFile("proc/swaps", swapHeader+"/swap file 100 bad 0"))
	equalPtr(t, s.Total, uint64(102400))
	if s.Used != nil || s.Entries[0].Used != nil {
		t.Fatal(s)
	}
	s = Swap(hostFile("proc/swaps", swapHeader+"/swap file bad 0 0"))
	equalPtr(t, s.Used, uint64(0))
	if s.Total != nil {
		t.Fatal(s)
	}
	s = Swap(hostFile("proc/swaps", swapHeader+"/swap file 100 101 0"))
	if s.Used != nil {
		t.Fatal(s)
	}
	s = Swap(hostFile("proc/swaps", swapHeader+"/a file 9007199254740991 0 0\n/b file 9007199254740991 0 0"))
	if s.Total != nil {
		t.Fatal("aggregate overflow", s)
	}
	equalPtr(t, s.Used, uint64(0))
	if s := Swap(Host{FS: fstest.MapFS{}}); !reflect.DeepEqual(s, SwapReading{}) {
		t.Fatal(s)
	}
}

func TestLoadAndUptime(t *testing.T) {
	h := Host{FS: fstest.MapFS{"proc/loadavg": &fstest.MapFile{Data: []byte("0 2.35 3.45 1/5 9")}, "proc/uptime": &fstest.MapFile{Data: []byte("123.45 800.9")}}}
	l := Load(h)
	equalPtr(t, l.One, 0.0)
	equalPtr(t, l.Five, 2.35)
	equalPtr(t, l.Fifteen, 3.45)
	equalPtr(t, l.Uptime, 123.45)
	l = Load(hostFile("proc/loadavg", "bad 1.5 -1"))
	if l.One != nil || l.Fifteen != nil || l.Uptime != nil {
		t.Fatal(l)
	}
	equalPtr(t, l.Five, 1.5)
	l = Load(hostFile("proc/loadavg", "1"))
	equalPtr(t, l.One, 1.0)
	if l.Five != nil || l.Fifteen != nil {
		t.Fatal(l)
	}
	for _, v := range []string{"", "NaN", "+Inf", "-1", "1e9999", "bad"} {
		if Load(hostFile("proc/uptime", v)).Uptime != nil {
			t.Fatal(v)
		}
	}
	if l := Load(Host{FS: fstest.MapFS{}}); !reflect.DeepEqual(l, LoadReading{}) {
		t.Fatal(l)
	}
	if nonnegative("0") == nil || nonnegative("0") != nil && math.Signbit(*nonnegative("0")) {
		t.Fatal("zero")
	}
}

func TestMachineInfo(t *testing.T) {
	for input, want := range map[string]string{"PRETTY_NAME=Linux": "Linux", "NAME='Some Linux'": "Some Linux", "NAME=Fallback\nPRETTY_NAME=\"Pretty Linux\"": "Pretty Linux", "NAME=Fallback\nPRETTY_NAME=\"broken": "Fallback", `PRETTY_NAME="A \\"quoted\\" name"`: `A \"quoted\" name`} {
		m := Info(hostFile("etc/os-release", input))
		equalPtr(t, m.Distro, want)
	}
	arch, version := "aarch64", "test-build"
	h := hostFile("proc/sys/kernel/osrelease", "6.12-test\n")
	h.Architecture, h.AgentVersion = &arch, &version
	m := Info(h)
	equalPtr(t, m.Kernel, "6.12-test")
	equalPtr(t, m.Architecture, arch)
	equalPtr(t, m.AgentVersion, version)
	if m.Distro != nil {
		t.Fatal(m)
	}
	if m := Info(Host{FS: fstest.MapFS{}}); !reflect.DeepEqual(m, MachineInfo{}) {
		t.Fatal(m)
	}
}
