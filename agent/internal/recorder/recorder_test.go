package recorder

import (
	"fmt"
	"io/fs"
	"math"
	"strings"
	"syscall"
	"testing"
	"testing/fstest"
	"time"

	"github.com/fsrodrig/anshin/agent/internal/recorder/probe"
)

// stat renders a two-CPU /proc/stat whose aggregate line has the given busy and idle jiffies. Busy time is split
// across user, system and steal; idle across idle and iowait; guest repeats part of user, as the kernel does.
func stat(busy, idle uint64) string {
	return fmt.Sprintf("cpu  %d 0 %d %d %d 0 0 %d %d 0\ncpu0 0 0 0 0 0 0 0 0 0 0\ncpu1 0 0 0 0 0 0 0 0 0 0\nintr 1 2 3\n",
		busy/2, busy/4, idle-idle/4, idle/4, busy-busy/2-busy/4, busy/2)
}

type machine struct {
	fs fstest.MapFS
	at time.Time
}

func (m *machine) host() probe.Host { return probe.Host{FS: m.fs} }

type scriptedRecorder struct {
	*Recorder
	machine *machine
}

func (r *scriptedRecorder) Read() Reading { return r.ReadAt(r.machine.at) }

func record(m *machine) *scriptedRecorder {
	return &scriptedRecorder{New(m.host(), m.at), m}
}

func newMachine(hostname, procStat string) *machine {
	m := &machine{fs: fstest.MapFS{}, at: time.Unix(1000, 0)}
	if hostname != "" {
		m.fs["proc/sys/kernel/hostname"] = &fstest.MapFile{Data: []byte(hostname + "\n")}
	}
	m.set(procStat)
	return m
}

func (m *machine) set(procStat string) {
	if procStat == "" {
		delete(m.fs, "proc/stat")
		return
	}
	m.fs["proc/stat"] = &fstest.MapFile{Data: []byte(procStat)}
}

func cpu(t *testing.T, r Reading) float64 {
	t.Helper()
	if r.CPUPct == nil {
		t.Fatal("CPUPct is nil, want a value")
	}
	return *r.CPUPct
}

func TestHostname(t *testing.T) {
	r := record(newMachine("machine-a", stat(0, 0))).Read()
	if r.Hostname == nil || *r.Hostname != "machine-a" {
		t.Fatalf("Hostname = %v, want machine-a", r.Hostname)
	}
}

func TestHostnameUnavailableIsNil(t *testing.T) {
	if r := record(newMachine("", stat(0, 0))).Read(); r.Hostname != nil {
		t.Fatalf("Hostname = %q, want nil", *r.Hostname)
	}
}

func TestCPUPctFromDelta(t *testing.T) {
	m := newMachine("machine-a", stat(1000, 9000))
	rec := record(m)
	// Two CPUs for one second is 200 jiffies: 50 busy, 150 idle.
	m.set(stat(1050, 9150))
	if got := cpu(t, rec.Read()); got != 25 {
		t.Fatalf("CPUPct = %v, want 25", got)
	}
	m.set(stat(1250, 9150))
	if got := cpu(t, rec.Read()); got != 100 {
		t.Fatalf("CPUPct = %v, want 100", got)
	}
}

func TestCPUPctKeepsBaselineUntilASecondHasPassed(t *testing.T) {
	m := newMachine("machine-a", stat(1000, 9000))
	rec := record(m)
	m.set(stat(1100, 9100))
	if got := cpu(t, rec.Read()); got != 50 {
		t.Fatalf("CPUPct = %v, want 50", got)
	}
	// Less than a second later: too few jiffies to judge, so the last value stands and the baseline stays put.
	m.set(stat(1110, 9100))
	if got := cpu(t, rec.Read()); got != 50 {
		t.Fatalf("CPUPct = %v, want the previous 50", got)
	}
	m.set(stat(1300, 9100))
	if got := cpu(t, rec.Read()); got != 100 {
		t.Fatalf("CPUPct = %v, want 100 measured from the kept baseline", got)
	}
}

func TestCPUPctUnavailableWithoutBaseline(t *testing.T) {
	m := newMachine("machine-a", "")
	rec := record(m)
	m.set(stat(1000, 9000))
	if r := rec.Read(); r.CPUPct != nil {
		t.Fatalf("CPUPct = %v, want nil on the first reading", *r.CPUPct)
	}
	m.set(stat(1100, 9100))
	if got := cpu(t, rec.Read()); got != 50 {
		t.Fatalf("CPUPct = %v, want 50", got)
	}
}

func TestCPUPctUnavailableWhenStatUnreadable(t *testing.T) {
	m := newMachine("machine-a", stat(1000, 9000))
	rec := record(m)
	m.set("")
	if r := rec.Read(); r.CPUPct != nil {
		t.Fatalf("CPUPct = %v, want nil", *r.CPUPct)
	}
	if r := rec.Read(); r.Hostname == nil {
		t.Fatal("an unreadable /proc/stat must not take the hostname with it")
	}
	m.set("intr 1 2 3\n")
	if r := rec.Read(); r.CPUPct != nil {
		t.Fatalf("CPUPct = %v, want nil without a cpu line", *r.CPUPct)
	}
}

func TestCPUPctResetsWhenCountersGoBackwards(t *testing.T) {
	m := newMachine("machine-a", stat(1000, 9000))
	rec := record(m)
	m.set(stat(500, 4000))
	if r := rec.Read(); r.CPUPct != nil {
		t.Fatalf("CPUPct = %v, want nil across a counter reset", *r.CPUPct)
	}
	m.set(stat(600, 4100))
	if got := cpu(t, rec.Read()); got != 50 {
		t.Fatalf("CPUPct = %v, want 50 from the new baseline", got)
	}
}

// line renders a two-CPU /proc/stat with only user, system, idle and iowait set.
func line(user, system, idle, iowait uint64) string {
	return fmt.Sprintf("cpu  %d 0 %d %d %d 0 0 0 0 0\ncpu0 0 0 0 0 0 0 0 0 0 0\ncpu1 0 0 0 0 0 0 0 0 0 0\n",
		user, system, idle, iowait)
}

func TestCPUPctResetsWhenABusyCounterGoesBackwardsBehindGrowthInOthers(t *testing.T) {
	m := newMachine("machine-a", line(1000, 1000, 9000, 0))
	rec := record(m)
	m.set(line(1200, 950, 9100, 0))
	if r := rec.Read(); r.CPUPct != nil {
		t.Fatalf("CPUPct = %v, want nil when system goes backwards", *r.CPUPct)
	}
	m.set(line(1250, 950, 9250, 0))
	if got := cpu(t, rec.Read()); got != 25 {
		t.Fatalf("CPUPct = %v, want 25 from the new baseline", got)
	}
}

func TestCPUPctToleratesIowaitMovingIntoIdle(t *testing.T) {
	m := newMachine("machine-a", line(1000, 0, 9000, 1000))
	rec := record(m)
	// Documentation/filesystems/proc.rst: iowait can decrease. Here 50 jiffies it counted are now idle.
	m.set(line(1100, 0, 9350, 950))
	if got := cpu(t, rec.Read()); got != 25 {
		t.Fatalf("CPUPct = %v, want 25 with idle+iowait still growing", got)
	}
	m.set(line(1100, 0, 9300, 950))
	if r := rec.Read(); r.CPUPct != nil {
		t.Fatalf("CPUPct = %v, want nil when idle+iowait goes backwards", *r.CPUPct)
	}
}

func TestCPUPctResetsWhenACPUAppears(t *testing.T) {
	m := newMachine("machine-a", stat(1000, 9000))
	rec := record(m)
	m.set(stat(1100, 9100) + "cpu2 0 0 0 0 0 0 0 0 0 0\n")
	if r := rec.Read(); r.CPUPct != nil {
		t.Fatalf("CPUPct = %v, want nil when the CPU count changes", *r.CPUPct)
	}
}

func TestCPUPctUnavailableWhenStatIsMalformed(t *testing.T) {
	m := newMachine("machine-a", stat(1000, 9000))
	rec := record(m)
	m.set("cpu  1 2 3\n")
	if r := rec.Read(); r.CPUPct != nil {
		t.Fatalf("CPUPct = %v, want nil when probe cannot parse /proc/stat", *r.CPUPct)
	}
}

func TestSlowLaneAndIndependentFailures(t *testing.T) {
	m := newMachine("machine", stat(0, 0))
	m.fs["etc/os-release"] = &fstest.MapFile{Data: []byte("PRETTY_NAME=First")}
	m.fs["proc/uptime"] = &fstest.MapFile{Data: []byte("123 0")}
	r := record(m)
	m.at = time.Unix(100, 0)
	first := r.Read()
	if first.Distro == nil || *first.Distro != "First" || first.Uptime == nil || *first.Uptime != 123 || first.MemoryReading.Total != nil {
		t.Fatal(first)
	}
	m.fs["etc/os-release"].Data = []byte("PRETTY_NAME=Second")
	m.fs["proc/uptime"].Data = []byte("124 0")
	m.at = m.at.Add(29 * time.Second)
	next := r.Read()
	if *next.Distro != "First" || *next.Uptime != 124 {
		t.Fatal(next)
	}
	m.at = m.at.Add(time.Second)
	if next := r.Read(); *next.Distro != "Second" {
		t.Fatal(next)
	}
	delete(m.fs, "etc/os-release")
	m.at = m.at.Add(30 * time.Second)
	if next := r.Read(); next.Distro != nil || next.Uptime == nil {
		t.Fatal(next)
	}
}

// deniedFS fails Lstat on one path with EACCES, so a device link's existence can't be determined.
type deniedFS struct {
	fstest.MapFS
	denied string
}

func (d deniedFS) Lstat(name string) (fs.FileInfo, error) {
	if name == d.denied {
		return nil, &fs.PathError{Op: "lstat", Path: name, Err: syscall.EACCES}
	}
	return d.MapFS.Lstat(name)
}

func (m *machine) tick(d time.Duration) { m.at = m.at.Add(d) }

// netdev writes /proc/net/dev with one row per "name rx tx", in that order.
func (m *machine) netdev(rows ...string) {
	var b strings.Builder
	b.WriteString("Inter-|   Receive |  Transmit\n face |bytes packets errs drop fifo frame compressed multicast|bytes packets errs drop fifo colls carrier compressed\n")
	for _, r := range rows {
		f := strings.Fields(r)
		fmt.Fprintf(&b, "%6s: %s 1 0 0 0 0 0 0 %s 1 0 0 0 0 0 0\n", f[0], f[1], f[2])
	}
	m.fs["proc/net/dev"] = &fstest.MapFile{Data: []byte(b.String())}
}

// diskstats writes /proc/diskstats with one row per "name sectors-read sectors-written".
func (m *machine) diskstats(rows ...string) {
	var b strings.Builder
	for i, r := range rows {
		f := strings.Fields(r)
		fmt.Fprintf(&b, "   8 %7d %s 1 0 %s 0 1 0 %s 0 0 0 0 0 0 0 0 0 0\n", 16*i, f[0], f[1], f[2])
	}
	m.fs["proc/diskstats"] = &fstest.MapFile{Data: []byte(b.String())}
}

func (m *machine) device(class string, names ...string) {
	for _, n := range names {
		m.fs["sys/class/"+class+"/"+n+"/device"] = &fstest.MapFile{Mode: fs.ModeSymlink, Data: []byte("target")}
	}
}

func rate(t *testing.T, what string, got *uint64, want uint64) {
	t.Helper()
	if got == nil || *got != want {
		t.Fatalf("%s = %v, want %d", what, got, want)
	}
}

func null(t *testing.T, what string, got *uint64) {
	t.Helper()
	if got != nil {
		t.Fatalf("%s = %d, want null", what, *got)
	}
}

func iface(t *testing.T, r Reading, name string) NetInterface {
	t.Helper()
	for _, i := range r.NetInterfaces {
		if i.Name == name {
			return i
		}
	}
	t.Fatalf("no interface %s in %+v", name, r.NetInterfaces)
	return NetInterface{}
}

func TestNetRatesDivideByElapsedTime(t *testing.T) {
	m := newMachine("machine-a", stat(0, 0))
	m.device("net", "eth0", "wlan0")
	m.netdev("lo 1000 1000", "eth0 1000 5000", "wlan0 0 0", "tailscale0 0 0")
	rec := record(m)
	m.tick(2500 * time.Millisecond)
	m.netdev("lo 3500 3500", "eth0 22000 5250", "wlan0 2500 0", "tailscale0 2500 2500")
	r := rec.Read()
	rate(t, "eth0 rx", iface(t, r, "eth0").Rx, 8400)
	rate(t, "eth0 tx", iface(t, r, "eth0").Tx, 100)
	rate(t, "lo rx", iface(t, r, "lo").Rx, 1000)
	// The summary counts eth0 and wlan0 only: lo and the tunnel would count traffic twice.
	rate(t, "net rx", r.NetRx, 9400)
	rate(t, "net tx", r.NetTx, 100)
	if got := []string{r.NetInterfaces[0].Name, r.NetInterfaces[3].Name}; got[0] != "lo" || got[1] != "tailscale0" || len(r.NetInterfaces) != 4 {
		t.Fatalf("interfaces = %+v, want every interface in kernel order", r.NetInterfaces)
	}
	if b := iface(t, r, "lo").DeviceBacked; b == nil || *b {
		t.Fatalf("lo device_backed = %v, want false", b)
	}
}

func TestNetRatesKeepBaselineUntilASecondHasPassed(t *testing.T) {
	m := newMachine("machine-a", stat(0, 0))
	m.device("net", "eth0")
	m.netdev("eth0 0 0")
	rec := record(m)
	m.tick(time.Second)
	m.netdev("eth0 1000 0")
	rate(t, "net rx", rec.Read().NetRx, 1000)
	m.tick(100 * time.Millisecond)
	m.netdev("eth0 1100 0")
	rate(t, "net rx", rec.Read().NetRx, 1000)
	m.tick(900 * time.Millisecond)
	m.netdev("eth0 3000 0")
	rate(t, "net rx measured from the kept baseline", rec.Read().NetRx, 2000)
}

func TestNetCounterResetIsNullForThatInterval(t *testing.T) {
	m := newMachine("machine-a", stat(0, 0))
	m.device("net", "eth0", "eth1")
	m.netdev("eth0 5000 5000", "eth1 0 0")
	rec := record(m)
	m.tick(time.Second)
	// eth0's driver was reloaded: its rx counter starts again from zero.
	m.netdev("eth0 100 6000", "eth1 10 10")
	r := rec.Read()
	null(t, "eth0 rx", iface(t, r, "eth0").Rx)
	rate(t, "eth0 tx", iface(t, r, "eth0").Tx, 1000)
	rate(t, "eth1 rx", iface(t, r, "eth1").Rx, 10)
	null(t, "net rx", r.NetRx)
	rate(t, "net tx", r.NetTx, 1010)
	m.tick(time.Second)
	m.netdev("eth0 300 6000", "eth1 10 10")
	r = rec.Read()
	rate(t, "eth0 rx from the new baseline", iface(t, r, "eth0").Rx, 200)
	rate(t, "net rx", r.NetRx, 200)
}

func TestNetInterfaceAppearingMidRun(t *testing.T) {
	m := newMachine("machine-a", stat(0, 0))
	m.device("net", "eth0", "usb0")
	m.netdev("eth0 0 0")
	rec := record(m)
	m.tick(time.Second)
	m.netdev("eth0 100 100", "veth0 50 50")
	r := rec.Read()
	null(t, "veth0 rx", iface(t, r, "veth0").Rx)
	rate(t, "net rx while an uncounted interface appears", r.NetRx, 100)
	m.tick(time.Second)
	m.netdev("eth0 200 200", "veth0 60 60", "usb0 7000 7000")
	r = rec.Read()
	null(t, "usb0 rx", iface(t, r, "usb0").Rx)
	rate(t, "veth0 rx", iface(t, r, "veth0").Rx, 10)
	null(t, "net rx while a counted interface appears", r.NetRx)
	null(t, "net tx while a counted interface appears", r.NetTx)
	m.tick(time.Second)
	m.netdev("eth0 300 300", "veth0 60 60", "usb0 7500 7000")
	r = rec.Read()
	rate(t, "usb0 rx", iface(t, r, "usb0").Rx, 500)
	rate(t, "net rx", r.NetRx, 600)
}

func TestNetInterfaceDisappearing(t *testing.T) {
	m := newMachine("machine-a", stat(0, 0))
	m.device("net", "eth0", "usb0")
	m.netdev("eth0 0 0", "veth0 0 0", "usb0 0 0")
	rec := record(m)
	m.tick(time.Second)
	m.netdev("eth0 100 100", "usb0 100 100")
	rate(t, "net rx while an uncounted interface goes", rec.Read().NetRx, 200)
	m.tick(time.Second)
	m.netdev("eth0 200 200")
	r := rec.Read()
	null(t, "net rx while a counted interface goes", r.NetRx)
	rate(t, "eth0 rx", iface(t, r, "eth0").Rx, 100)
	m.tick(time.Second)
	m.netdev("eth0 300 300")
	rate(t, "net rx", rec.Read().NetRx, 100)
}

func TestNetSummaryNullWithoutDeviceBackedInterface(t *testing.T) {
	m := newMachine("machine-a", stat(0, 0))
	m.netdev("lo 0 0", "tailscale0 0 0")
	rec := record(m)
	m.tick(time.Second)
	m.netdev("lo 10 10", "tailscale0 10 10")
	r := rec.Read()
	null(t, "net rx", r.NetRx)
	null(t, "net tx", r.NetTx)
	rate(t, "lo rx", iface(t, r, "lo").Rx, 10)
}

func TestNetUnreadable(t *testing.T) {
	m := newMachine("machine-a", stat(0, 0))
	m.device("net", "eth0")
	rec := record(m)
	if r := rec.Read(); r.NetRx != nil || r.NetInterfaces != nil {
		t.Fatalf("without /proc/net/dev: %+v", r)
	}
	m.netdev("eth0 0 0")
	m.tick(time.Second)
	r := rec.Read()
	null(t, "net rx without a baseline", r.NetRx)
	null(t, "eth0 rx without a baseline", iface(t, r, "eth0").Rx)
	m.tick(time.Second)
	m.netdev("eth0 10 10")
	rate(t, "net rx", rec.Read().NetRx, 10)
	delete(m.fs, "proc/net/dev")
	m.tick(time.Second)
	if r := rec.Read(); r.NetRx != nil || r.NetInterfaces != nil || r.Hostname == nil {
		t.Fatalf("with /proc/net/dev gone: %+v", r)
	}
	m.netdev("eth0 20 20")
	m.tick(time.Second)
	null(t, "net rx after the input came back", rec.Read().NetRx)
}

func TestNetUndeterminedLinkIsNotAGuess(t *testing.T) {
	m := newMachine("machine-a", stat(0, 0))
	m.device("net", "eth0")
	m.netdev("eth0 0 0", "eth1 0 0")
	rec := &scriptedRecorder{New(probe.Host{FS: deniedFS{m.fs, "sys/class/net/eth1/device"}}, m.at), m}
	m.tick(time.Second)
	m.netdev("eth0 10 10", "eth1 10 10")
	r := rec.Read()
	if b := iface(t, r, "eth1").DeviceBacked; b != nil {
		t.Fatalf("eth1 device_backed = %v, want null", *b)
	}
	rate(t, "eth1 rx", iface(t, r, "eth1").Rx, 10)
	null(t, "net rx while eth1 may or may not be counted", r.NetRx)
}

func TestDiskRates(t *testing.T) {
	m := newMachine("machine-a", stat(0, 0))
	m.device("block", "sda", "sdb")
	m.diskstats("sda 1000 2000", "sda1 1000 2000", "sdb 0 0")
	rec := record(m)
	m.tick(2 * time.Second)
	m.diskstats("sda 1004 2400", "sda1 1004 2400", "sdb 0 0")
	r := rec.Read()
	if len(r.Disks) != 2 || r.Disks[0].Name != "sda" || r.Disks[1].Name != "sdb" {
		t.Fatalf("disks = %+v, want sda and sdb only", r.Disks)
	}
	// Sectors are 512 bytes: 4 sectors over 2 s is 1024 B/s.
	rate(t, "sda read", r.Disks[0].Read, 1024)
	rate(t, "sda write", r.Disks[0].Write, 102400)
	rate(t, "sdb read", r.Disks[1].Read, 0)

	m.tick(2 * time.Second)
	m.diskstats("sda 10 2400", "sdc 0 0", "sdb 4 0")
	m.device("block", "sdc")
	r = rec.Read()
	null(t, "sda read across a counter reset", r.Disks[0].Read)
	rate(t, "sda write", r.Disks[0].Write, 0)
	null(t, "sdc read as it appears", r.Disks[1].Read)
	rate(t, "sdb read", r.Disks[2].Read, 1024)

	m.tick(time.Second)
	m.diskstats("sda 20 2400", "sdc 1 0")
	r = rec.Read()
	rate(t, "sda read", r.Disks[0].Read, 5120)
	rate(t, "sdc read", r.Disks[1].Read, 512)
	if len(r.Disks) != 2 {
		t.Fatalf("disks = %+v after sdb went", r.Disks)
	}

	delete(m.fs, "proc/diskstats")
	m.tick(time.Second)
	if r := rec.Read(); r.Disks != nil {
		t.Fatalf("disks = %+v, want null when /proc/diskstats is unreadable", r.Disks)
	}
}

func TestNetSummaryOverflowIsNull(t *testing.T) {
	m := newMachine("machine-a", stat(0, 0))
	m.device("net", "eth0", "eth1")
	m.netdev("eth0 0 0", "eth1 0 0")
	rec := record(m)
	m.tick(time.Second)
	m.netdev("eth0 18446744073709551615 1", "eth1 1 1")
	r := rec.Read()
	null(t, "net rx when the summed deltas overflow", r.NetRx)
	rate(t, "net tx", r.NetTx, 2)
}

func TestIOSubsecondChanges(t *testing.T) {
	for _, change := range []string{"reset", "reappear", "appear"} {
		t.Run(change, func(t *testing.T) {
			m := newMachine("machine", stat(0, 0))
			m.device("net", "eth0", "eth1")
			m.device("block", "sda", "sdb")
			set := func(n uint64, present bool) {
				net, disk := []string{"eth1 0 0"}, []string{"sdb 0 0"}
				if present {
					net = append(net, fmt.Sprintf("eth0 %d %d", n, n))
					disk = append(disk, fmt.Sprintf("sda %d %d", n, n))
				}
				m.netdev(net...)
				m.diskstats(disk...)
			}
			set(5000, change != "appear")
			rec := record(m)
			m.tick(time.Second)
			set(6000, change != "appear")
			rec.Read()
			m.tick(100 * time.Millisecond)
			if change == "reappear" {
				set(0, false)
				r := rec.Read()
				null(t, "summary on disappearance", r.NetRx)
				if len(r.NetInterfaces) != 1 || len(r.Disks) != 1 {
					t.Fatal("disappeared rows remain")
				}
				m.tick(100 * time.Millisecond)
			}
			set(100, true)
			r := rec.Read()
			i := iface(t, r, "eth0")
			null(t, "changed rx", i.Rx)
			null(t, "changed tx", i.Tx)
			null(t, "summary rx", r.NetRx)
			null(t, "summary tx", r.NetTx)
			null(t, "changed disk read", r.Disks[1].Read)
			null(t, "changed disk write", r.Disks[1].Write)
			rate(t, "unaffected interface", iface(t, r, "eth1").Rx, 0)
			rate(t, "unaffected disk", r.Disks[0].Read, 0)
			m.tick(900 * time.Millisecond)
			set(7000, true)
			null(t, "minimum after new baseline", rec.Read().NetRx)
			m.tick(100 * time.Millisecond)
			set(7100, true)
			r = rec.Read()
			rate(t, "rx from new baseline", iface(t, r, "eth0").Rx, 7000)
			rate(t, "disk from new baseline", r.Disks[1].Read, 7000*512)
		})
	}
}

func TestIORateBounds(t *testing.T) {
	for _, tc := range []struct {
		name, net, disk string
		want            *uint64
	}{
		{"below", "9223372036854774784", "18014398509481982", new(uint64(9223372036854774784))},
		{"rounded over", "9223372036854775807", "18014398509481984", nil},
		{"over", "9223372036854775808", "36028797018963968", nil},
	} {
		t.Run(tc.name, func(t *testing.T) {
			m := newMachine("machine", stat(0, 0))
			m.device("net", "eth0")
			m.device("block", "sda")
			m.netdev("eth0 0 0")
			m.diskstats("sda 0 0")
			rec := record(m)
			m.tick(time.Second)
			m.netdev("eth0 " + tc.net + " 1")
			m.diskstats("sda " + tc.disk + " 1")
			r := rec.Read()
			for name, got := range map[string]*uint64{"interface": iface(t, r, "eth0").Rx, "summary": r.NetRx, "disk": r.Disks[0].Read} {
				if tc.want == nil {
					null(t, name, got)
				} else {
					rate(t, name, got, *tc.want)
				}
			}
			rate(t, "tx unaffected", r.NetTx, 1)
			rate(t, "write unaffected", r.Disks[0].Write, 512)
		})
	}
}

func TestNetSummedDeltaSignedOverflow(t *testing.T) {
	m := newMachine("machine", stat(0, 0))
	m.device("net", "eth0", "eth1")
	m.netdev("eth0 0 0", "eth1 0 0")
	rec := record(m)
	m.tick(2 * time.Second)
	m.netdev("eth0 4611686018427387904 1", "eth1 4611686018427387904 1")
	r := rec.Read()
	rate(t, "individual rx", iface(t, r, "eth0").Rx, 2305843009213693952)
	null(t, "signed sum overflow", r.NetRx)
}

func TestPerSecondRejectsNonFinite(t *testing.T) {
	for _, dt := range []float64{0, math.NaN(), math.SmallestNonzeroFloat64} {
		null(t, "non-finite rate", perSecond(new(uint64(1)), 512, dt))
	}
}
