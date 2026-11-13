// Package capturetest is a fake Machine for capture and replay tests: a /proc and /sys tree laid out as on a real
// Machine (class entries are symlinks into /sys/devices), salted with identifiers and with files capture must
// never open. Like replay, it imports testing/fstest, so only tests may import it.
//
// Every identifier here is assembled at runtime, so the identifier scan never finds one in this file.
package capturetest

import (
	"errors"
	"fmt"
	"io/fs"
	"path"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"testing/fstest"
	"time"

	"github.com/fsrodrig/anshin/agent/internal/capture"
	"github.com/fsrodrig/anshin/agent/internal/recorder/probe"
)

// Identifiers is everything the fake Machine leaks that a capture must not keep.
type Identifiers struct {
	Hostname, ShortHostname, EtcHostname, User, DenyWord string
	MAC, BluetoothMAC, MACInterface                      string
	Serial, TailnetName, TailnetV4, TailnetV6            string
}

// All lists the raw values, for a sweep that asserts none survived.
func (id Identifiers) All() []string {
	return []string{id.Hostname, id.ShortHostname, id.EtcHostname, id.User, id.DenyWord, id.MAC, id.BluetoothMAC,
		id.MACInterface, id.Serial, id.TailnetName, id.TailnetV4, id.TailnetV6}
}

// IdentifiersFile is the -identifiers content naming the deny word and the user.
func (id Identifiers) IdentifiersFile() []byte {
	return []byte(fmt.Sprintf("# private\n%s\nuser:%s\n", id.DenyWord, id.User))
}

func NewIdentifiers() Identifiers {
	octets := func(sep string, base int) string {
		var o []string
		for i := range 6 {
			o = append(o, fmt.Sprintf("%02x", base+i*7))
		}
		return strings.Join(o, sep)
	}
	short := fmt.Sprintf("box%d", 40+2)
	return Identifiers{
		Hostname:      short + ".lan",
		ShortHostname: short,
		EtcHostname:   fmt.Sprintf("other%d", 3*3),
		User:          fmt.Sprintf("ann%d", 4+3),
		DenyWord:      fmt.Sprintf("hidden%dnet", 12),
		MAC:           octets(":", 0x3c),
		BluetoothMAC:  strings.ToUpper(octets(":", 0x5a)),
		MACInterface:  "enx" + octets("", 0x1e),
		Serial:        fmt.Sprintf("PF%d", 12345*7),
		TailnetName:   fmt.Sprintf("tail%d.%s", 1000+234, "ts.net"),
		TailnetV4:     fmt.Sprintf("100.%d.12.7", 64+37),
		TailnetV6:     fmt.Sprintf("%s:ab%02d:4843::1", "fd7a:115c:a1e0", 12),
	}
}

// StatfsResult is how the fake answers statfs for one mount point. Block makes the call hang until the test ends.
type StatfsResult struct {
	Stat  probe.FsStat
	Err   error
	Block bool
}

// Machine is the fake. Opened and StatfsCalls record every access, so tests can prove what capture never touched.
type Machine struct {
	ID     Identifiers
	FS     fstest.MapFS
	Errs   map[string]syscall.Errno
	Statfs map[string]StatfsResult

	mu      sync.Mutex
	opened  []string
	statfsd []string
	release chan struct{}
	jiffies uint64
	ticks   int
	clock   time.Time
}

// Never are paths capture must never open: drive and sleeping-device temperatures, MACs, serials, uevents.
func (m *Machine) Never() []string {
	return []string{
		"sys/class/hwmon/hwmon1/temp1_input", // nvme
		"sys/class/hwmon/hwmon2/temp1_input", // thermal zone
		"sys/class/hwmon/hwmon3/temp1_input", // suspended amdgpu
		"sys/class/hwmon/hwmon4/temp1_input", // drivetemp
		"sys/class/hwmon/hwmon0/temp1_max",
		"sys/class/hwmon/hwmon0/uevent",
		"sys/class/net/wlan0/address",
		"sys/class/net/" + m.ID.MACInterface + "/address",
		"sys/class/power_supply/BAT0/serial_number",
		"sys/class/power_supply/BAT0/model_name",
		"sys/class/power_supply/BAT0/uevent",
		"proc/cpuinfo",
	}
}

// NeverStatfsd are mount points capture must skip.
var NeverStatfsd = []string{"/mnt/auto", "/mnt/nas", "/run/user/1000/doc", "/mnt/ssh"}

func NewMachine() *Machine {
	id := NewIdentifiers()
	m := &Machine{ID: id, FS: fstest.MapFS{}, Errs: map[string]syscall.Errno{}, release: make(chan struct{}),
		jiffies: 1000, clock: time.Date(2026, 1, 2, 3, 4, 5, 6, time.FixedZone("x", 3600))}
	file := func(p, data string) { m.FS[p] = &fstest.MapFile{Data: []byte(data), Mode: 0o444} }
	link := func(p, target string) { m.FS[p] = &fstest.MapFile{Data: []byte(target), Mode: fs.ModeSymlink | 0o777} }
	dir := func(p string) { m.FS[p] = &fstest.MapFile{Mode: fs.ModeDir | 0o555} }

	file("proc/sys/kernel/hostname", id.Hostname+"\n")
	file("proc/sys/kernel/osrelease", "7.2.0-1\n")
	file("etc/hostname", id.EtcHostname+"\n")
	file("etc/passwd", "root:x:0:0::/root:/bin/sh\n"+id.User+":x:1000:1000::/home/"+id.User+":/bin/sh\nnobody:x:65534:65534::/:/bin/false\n")
	link("etc/os-release", "../usr/lib/os-release")
	file("usr/lib/os-release", "NAME=Fake\nPRETTY_NAME=\""+strings.ToUpper(id.ShortHostname)+" Linux\"\n"+
		"SERIAL_NUMBER="+id.Serial+"\nLINK="+id.MAC+"\nUser="+id.User+"\n")
	m.setStat()
	file("proc/loadavg", "0.10 0.20 0.30 1/100 4242\n")
	file("proc/uptime", "100.00 150.00\n")
	file("proc/meminfo", "MemTotal: 1000 kB\nMemAvailable: 500 kB\n")
	file("proc/swaps", "Filename Type Size Used Priority\n/dev/zram0 partition 100 0 100\n")
	file("proc/diskstats", " 259 0 nvme0n1 1 2 3 4 5 6 7 8 0 9 10\n")
	file("proc/cpuinfo", "model name: never copied\n")
	link("proc/self", "4242")
	link("proc/net", "self/net")
	file("proc/4242/net/dev", "Inter-| Receive\n lo: 1 2\n "+id.MACInterface+": 3 4\n wlan0: 5 6\n")
	file("proc/4242/mountinfo", strings.Join([]string{
		"1 0 259:2 / / rw - ext4 /dev/nvme0n1p2 rw",
		"2 1 259:2 /home /home/" + id.User + "/data rw - ext4 /dev/nvme0n1p2 rw",
		"3 1 0:40 / /proc/sys/fs/binfmt_misc rw - autofs systemd-1 rw",
		"4 3 0:41 / /proc/sys/fs/binfmt_misc rw - binfmt_misc binfmt_misc rw",
		"5 1 0:42 / /mnt/auto rw - autofs systemd-1 rw",
		"6 1 0:43 / /mnt/nas rw - nfs4 nas." + id.TailnetName + ":/export rw,addr=" + id.TailnetV4,
		"7 1 0:44 / /run/user/1000/doc rw - fuse.portal portal rw",
		"8 1 0:45 / /mnt/ssh rw - fuse.sshfs " + id.User + "@" + id.DenyWord + ":/ rw",
		"9 1 0:46 / /mnt/slow rw - ext4 /dev/sdb1 rw",
		"10 1 0:47 / /mnt/denied rw - ext4 /dev/sdc1 rw",
		"11 1 0:48 / /mnt/with\\040space rw - ext4 /dev/sdd1 rw,v6=" + id.TailnetV6,
	}, "\n")+"\n")
	file("sys/devices/system/cpu/online", "0-1\n")

	pci := "sys/devices/pci0000:00/"
	hwmon := func(n int, name, device, status string, temps map[string]string) {
		real := fmt.Sprintf("%s0000:00:%02x.0/hwmon/hwmon%d", pci, n, n)
		if device == "" {
			device = fmt.Sprintf("0000:00:%02x.0", n)
		}
		link(fmt.Sprintf("sys/class/hwmon/hwmon%d", n), fmt.Sprintf("../../devices/pci0000:00/0000:00:%02x.0/hwmon/hwmon%d", n, n))
		file(real+"/name", name+"\n")
		file(real+"/uevent", "DEVTYPE=hwmon\n")
		link(real+"/device", "../../../"+device)
		if status != "" {
			file(pci+device+"/power/runtime_status", status+"\n")
		} else {
			dir(pci + device)
		}
		for k, v := range temps {
			file(real+"/"+k, v+"\n")
		}
	}
	hwmon(0, "k10temp", "", "active", map[string]string{"temp1_input": "45000", "temp1_label": "Tctl", "temp1_max": "70000"})
	hwmon(1, "nvme", "", "active", map[string]string{"temp1_input": "38000"})
	hwmon(2, "acpitz", "thermal_zone0", "active", map[string]string{"temp1_input": "30000"})
	hwmon(3, "amdgpu", "", "suspended", map[string]string{"temp1_input": "40000"})
	hwmon(4, "drivetemp", "", "active", map[string]string{"temp1_input": "35000"})
	hwmon(5, "coretemp", "", "", map[string]string{"temp1_input": "50000", "temp1_crit": "100000", "temp2_input": ""})
	m.Errs["sys/class/hwmon/hwmon5/temp2_input"] = syscall.ENODATA

	supply := func(name string, attrs map[string]string) {
		real := "sys/devices/platform/" + name + "/power_supply/" + name
		link("sys/class/power_supply/"+name, "../../devices/platform/"+name+"/power_supply/"+name)
		for k, v := range attrs {
			file(real+"/"+k, v+"\n")
		}
	}
	bat := map[string]string{"type": "Battery", "status": "Discharging", "capacity": "80", "energy_now": "40000000",
		"model_name": "Fake", "uevent": "POWER_SUPPLY_SERIAL_NUMBER=" + id.Serial}
	// Keyed indirectly, or the identifier scan reads this source line as a serial number.
	bat[path.Base("serial_number")] = id.Serial
	supply("BAT0", bat)
	supply("hid-"+id.BluetoothMAC+"-battery", map[string]string{"type": "Battery", "scope": "Device", "capacity": "50"})

	netdev := func(name string, device bool) {
		real := "sys/devices/virtual/net/" + name
		if device {
			real = "sys/devices/pci0000:00/0000:00:1c.0/net/" + name
			link(real+"/device", "../../../0000:00:1c.0")
		} else {
			dir(real)
		}
		link("sys/class/net/"+name, "../../"+strings.TrimPrefix(real, "sys/"))
		file(real+"/address", id.MAC+"\n")
	}
	netdev("lo", false)
	netdev("wlan0", true)
	netdev(id.MACInterface, true)
	link("sys/class/block/nvme0n1", "../../devices/pci0000:00/0000:00:1d.0/nvme/nvme0/nvme0n1")
	link("sys/devices/pci0000:00/0000:00:1d.0/nvme/nvme0/nvme0n1/device", "../../nvme0")
	dir("sys/devices/pci0000:00/0000:00:1d.0/nvme/nvme0")
	link("sys/class/block/zram0", "../../devices/virtual/block/zram0")
	dir("sys/devices/virtual/block/zram0")

	m.Statfs = map[string]StatfsResult{
		"/":                          {Stat: probe.FsStat{Type: 0xef53, Bsize: 4096, Frsize: 4096, Blocks: 100, Bfree: 40, Bavail: 30, Files: 10, Ffree: 5, Flags: 32}},
		"/home/" + id.User + "/data": {Stat: probe.FsStat{Type: 0xef53, Bsize: 4096, Frsize: 4096, Blocks: 100, Bfree: 40, Bavail: 30}},
		"/proc/sys/fs/binfmt_misc":   {Stat: probe.FsStat{Type: 0x42494e4d, Bsize: 4096, Frsize: 4096}},
		"/mnt/slow":                  {Block: true},
		"/mnt/denied":                {Err: &fs.PathError{Op: "statfs", Path: "/mnt/denied", Err: syscall.EACCES}},
		"/mnt/with space":            {Stat: probe.FsStat{Type: 0xef53, Bsize: 1024, Frsize: 1024, Blocks: 7}},
	}
	return m
}

// setStat writes a two-CPU /proc/stat whose counters grow by one second of half-busy CPU each Tick.
func (m *Machine) setStat() {
	busy, idle := m.jiffies, 9*m.jiffies
	m.FS["proc/stat"] = &fstest.MapFile{Mode: 0o444, Data: []byte(fmt.Sprintf(
		"cpu  %d 0 %d %d 0 0 0 0 0 0\ncpu0 0 0 0 0 0 0 0 0 0 0\ncpu1 0 0 0 0 0 0 0 0 0 0\nintr 1\n", busy/2, busy-busy/2, idle))}
}

// Clock is a capture.Source clock that also moves the Machine forward a second, so consecutive snapshots differ.
func (m *Machine) Clock() capture.Clock {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.ticks > 0 {
		m.jiffies += 100
	}
	m.ticks++
	m.setStat()
	c := capture.Clock{Wall: m.clock, BoottimeNs: m.clock.UnixNano() + 7, MonotonicNs: m.clock.UnixNano()}
	m.clock = m.clock.Add(time.Second)
	return c
}

// Release unblocks every statfs the fake is holding.
func (m *Machine) Release() {
	m.mu.Lock()
	defer m.mu.Unlock()
	select {
	case <-m.release:
	default:
		close(m.release)
	}
}

func (m *Machine) Opened() []string {
	m.mu.Lock()
	defer m.mu.Unlock()
	return append([]string(nil), m.opened...)
}

func (m *Machine) StatfsCalls() []string {
	m.mu.Lock()
	defer m.mu.Unlock()
	return append([]string(nil), m.statfsd...)
}

func (m *Machine) Source() capture.Source {
	return capture.Source{Host: m.Host(), Clock: m.Clock, User: m.ID.User, MountID: func(p string) (uint64, error) {
		var id uint64
		for _, line := range strings.Split(string(m.FS["proc/4242/mountinfo"].Data), "\n") {
			f := strings.Fields(line)
			if len(f) > 4 && strings.ReplaceAll(f[4], `\040`, " ") == p {
				id, _ = strconv.ParseUint(f[0], 10, 64)
			}
		}
		return id, nil
	}}
}

func (m *Machine) Host() probe.Host {
	return probe.Host{FS: recordingFS{m}, Statfs: func(p string) (probe.FsStat, error) {
		m.mu.Lock()
		m.statfsd = append(m.statfsd, p)
		r, ok := m.Statfs[p]
		release := m.release
		m.mu.Unlock()
		if !ok {
			return probe.FsStat{}, &fs.PathError{Op: "statfs", Path: p, Err: syscall.ENOENT}
		}
		if r.Block {
			<-release
		}
		return r.Stat, r.Err
	}}
}

type recordingFS struct{ m *Machine }

func (r recordingFS) note(name string) {
	r.m.mu.Lock()
	defer r.m.mu.Unlock()
	r.m.opened = append(r.m.opened, name)
}

func (r recordingFS) Open(name string) (fs.File, error) {
	r.note(name)
	r.m.mu.Lock()
	errno, bad := r.m.Errs[name]
	r.m.mu.Unlock()
	f, err := r.m.FS.Open(name)
	if err != nil {
		return nil, toErrno(err)
	}
	if bad {
		return failingFile{f, &fs.PathError{Op: "read", Path: name, Err: errno}}, nil
	}
	return f, nil
}

func (r recordingFS) ReadLink(name string) (string, error) {
	r.note(name)
	s, err := r.m.FS.ReadLink(name)
	return s, toErrno(err)
}

func (r recordingFS) Lstat(name string) (fs.FileInfo, error) {
	r.note(name)
	fi, err := r.m.FS.Lstat(name)
	return fi, toErrno(err)
}

// toErrno makes MapFS's errors look like the kernel's, which is what os.DirFS returns.
func toErrno(err error) error {
	var pe *fs.PathError
	if errors.As(err, &pe) && errors.Is(pe.Err, fs.ErrNotExist) {
		return &fs.PathError{Op: pe.Op, Path: pe.Path, Err: syscall.ENOENT}
	}
	return err
}

type failingFile struct {
	fs.File
	err error
}

func (f failingFile) Read([]byte) (int, error) { return 0, f.err }

func (f failingFile) ReadDir(int) ([]fs.DirEntry, error) { return nil, f.err }
