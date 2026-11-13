package recorder

import (
	"fmt"
	"testing"
	"testing/fstest"

	"github.com/fsrodrig/anshin/agent/internal/recorder/probe"
)

// stat renders a two-CPU /proc/stat whose aggregate line has the given busy and idle jiffies. Busy time is split
// across user, system and steal; idle across idle and iowait; guest repeats part of user, as the kernel does.
func stat(busy, idle uint64) string {
	return fmt.Sprintf("cpu  %d 0 %d %d %d 0 0 %d %d 0\ncpu0 0 0 0 0 0 0 0 0 0 0\ncpu1 0 0 0 0 0 0 0 0 0 0\nintr 1 2 3\n",
		busy/2, busy/4, idle-idle/4, idle/4, busy-busy/2-busy/4, busy/2)
}

type machine struct{ fs fstest.MapFS }

func (m *machine) host() probe.Host { return probe.Host{FS: m.fs} }

func newMachine(hostname, procStat string) *machine {
	m := &machine{fs: fstest.MapFS{}}
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
	r := New(newMachine("machine-a", stat(0, 0)).host()).Read()
	if r.Hostname == nil || *r.Hostname != "machine-a" {
		t.Fatalf("Hostname = %v, want machine-a", r.Hostname)
	}
}

func TestHostnameUnavailableIsNil(t *testing.T) {
	if r := New(newMachine("", stat(0, 0)).host()).Read(); r.Hostname != nil {
		t.Fatalf("Hostname = %q, want nil", *r.Hostname)
	}
}

func TestCPUPctFromDelta(t *testing.T) {
	m := newMachine("machine-a", stat(1000, 9000))
	rec := New(m.host())
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
	rec := New(m.host())
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
	rec := New(m.host())
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
	rec := New(m.host())
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
	rec := New(m.host())
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
	rec := New(m.host())
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
	rec := New(m.host())
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
	rec := New(m.host())
	m.set(stat(1100, 9100) + "cpu2 0 0 0 0 0 0 0 0 0 0\n")
	if r := rec.Read(); r.CPUPct != nil {
		t.Fatalf("CPUPct = %v, want nil when the CPU count changes", *r.CPUPct)
	}
}

func TestCPUPctUnavailableWhenStatIsMalformed(t *testing.T) {
	m := newMachine("machine-a", stat(1000, 9000))
	rec := New(m.host())
	m.set("cpu  1 2 3\n")
	if r := rec.Read(); r.CPUPct != nil {
		t.Fatalf("CPUPct = %v, want nil when probe cannot parse /proc/stat", *r.CPUPct)
	}
}
