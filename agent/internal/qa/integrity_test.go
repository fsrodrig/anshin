package qa

import (
	"context"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

type interceptRunner struct {
	next Runner
	call func(Cmd) ([]byte, []byte, error, bool)
}

func (r interceptRunner) Run(ctx context.Context, c Cmd) ([]byte, []byte, error) {
	if a, b, e, ok := r.call(c); ok {
		return a, b, e
	}
	return r.next.Run(ctx, c)
}

func TestUnsafeBranchAndLoadedRunIDs(t *testing.T) {
	f := newFixture(t)
	gitRun(t, f.repo, "branch", "-m", "qa$(id>qa-proof)")
	run := f.init(t)
	info, err := loadRun(run)
	if err != nil {
		t.Fatal(err)
	}
	if strings.ContainsAny(info.RunID, "$()>") {
		t.Fatal(info.RunID)
	}
	info.RunID = "qa$(id)"
	if err := writeJSON(filepath.Join(run, "run.json"), info); err != nil {
		t.Fatal(err)
	}
	if _, err := loadRun(run); err == nil {
		t.Fatal("loaded unsafe run ID")
	}
}

func TestReplacementOwnershipAndAPKArePreserved(t *testing.T) {
	for _, replacement := range []string{"marker", "apk"} {
		t.Run(replacement, func(t *testing.T) {
			f := newFixture(t)
			run := f.init(t)
			if _, err := f.up(t, run); err != nil {
				t.Fatal(err)
			}
			if replacement == "marker" {
				f.emu.marker = "replacement-run"
			} else {
				f.h.Runner = interceptRunner{next: f.emu, call: func(c Cmd) ([]byte, []byte, error, bool) {
					if strings.Contains(strings.Join(c.Args, " "), "sha256sum '/data/app/base.apk'") {
						return []byte(strings.Repeat("ef", 32)), nil, nil, true
					}
					return nil, nil, nil, false
				}}
			}
			if _, err := f.h.Seed(context.Background(), run, "tokyo-night"); err == nil {
				t.Fatal("seed changed replacement data")
			}
			if _, err := f.h.Down(context.Background(), run); err == nil {
				t.Fatal("cleanup hid ownership conflict")
			}
			if !f.emu.installed || f.emu.marker == "" || !f.emu.root {
				t.Fatal("replacement or retry prerequisites removed")
			}
		})
	}
}

func TestAtomicDeviceClaim(t *testing.T) {
	f := newFixture(t)
	f.emu.root = true
	start := make(chan struct{})
	errs := make(chan error, 2)
	var wg sync.WaitGroup
	for _, id := range []string{"run-a", "run-b"} {
		l, err := loadLedger(t.TempDir(), id)
		if err != nil {
			t.Fatal(err)
		}
		wg.Add(1)
		go func(l *Ledger) {
			defer wg.Done()
			<-start
			h := &Helper{Runner: f.emu}
			errs <- h.claimDevice(context.Background(), SDK{ADB: "adb"}, l, "emulator-5554")
		}(l)
	}
	close(start)
	wg.Wait()
	close(errs)
	success := 0
	for err := range errs {
		if err == nil {
			success++
		}
	}
	if success != 1 {
		t.Fatalf("%d claims succeeded", success)
	}
}

func TestDevicePIDReuseNeverKills(t *testing.T) {
	for _, mismatch := range []string{"start", "exe", "cwd", "missing", "invalid-pid"} {
		t.Run(mismatch, func(t *testing.T) {
			killed := false
			dir := deviceTmp + "/anshin-qa-run"
			stat := "1234 (anshin-qa) " + strings.Repeat("0 ", 19) + "42"
			output := stat + "\n" + dir + "/anshin-qa\n" + dir
			if mismatch == "start" {
				output = strings.Replace(output, "42", "43", 1)
			}
			if mismatch == "exe" {
				output = strings.Replace(output, dir+"/anshin-qa", deviceTmp+"/anshin-qa-other/anshin-qa", 1)
			}
			if mismatch == "cwd" {
				output += "-other"
			}
			r := &Resource{Data: map[string]string{"pid": "1234", "dir": dir, "identity": "42:" + dir + "/anshin-qa:" + dir}}
			if mismatch == "missing" {
				delete(r.Data, "identity")
			}
			if mismatch == "invalid-pid" {
				r.Data["pid"] = "1234;id"
			}
			h := &Helper{Runner: interceptRunner{call: func(c Cmd) ([]byte, []byte, error, bool) {
				if strings.Contains(strings.Join(c.Args, " "), "shell kill ") {
					killed = true
				}
				return []byte(output), nil, nil, true
			}}}
			if err := h.stopDeviceAgent(context.Background(), SDK{ADB: "adb"}, t.TempDir(), "emulator-5554", r); err == nil || killed {
				t.Fatalf("identity conflict: err=%v killed=%v", err, killed)
			}
		})
	}
}

func TestCleanupInspectionAndDeletionFailuresRetainOwnership(t *testing.T) {
	for _, command := range []string{"iptables -t nat -S", "iptables -t nat -D", "pm list packages", "pm path ", "cat " + ownerMarker, "unroot"} {
		t.Run(command, func(t *testing.T) {
			f := newFixture(t)
			run := f.init(t)
			if _, err := f.up(t, run); err != nil {
				t.Fatal(err)
			}
			f.emu.fail[command] = errExit
			if _, err := f.h.Down(context.Background(), run); err == nil {
				t.Fatal("inspection failure reported success")
			}
			if f.emu.marker == "" || !f.emu.root {
				t.Fatal("released retry prerequisites")
			}
			delete(f.emu.fail, command)
			if _, err := f.h.Down(context.Background(), run); err != nil {
				t.Fatal(err)
			}
			if f.emu.dnat || f.emu.installed || f.emu.marker != "" {
				t.Fatal("retry left resources")
			}
		})
	}
}

type blockedStop struct {
	ProcessControl
	stopped chan struct{}
	resume  chan struct{}
}

func (p blockedStop) Stop(pid int, start string) error {
	err := p.ProcessControl.Stop(pid, start)
	close(p.stopped)
	<-p.resume
	return err
}
func TestConcurrentOperationsRejectBeforeLedgerRead(t *testing.T) {
	f := newFixture(t)
	run := f.init(t)
	if _, err := f.up(t, run); err != nil {
		t.Fatal(err)
	}
	a := *f.h
	b := *f.h
	gate := blockedStop{f.procs, make(chan struct{}), make(chan struct{})}
	a.Procs = gate
	done := make(chan error, 1)
	go func() { done <- a.AgentStop(context.Background(), run) }()
	<-gate.stopped
	if err := b.AgentStart(context.Background(), run, nil); err == nil {
		t.Error("overlapping start accepted")
	}
	if _, err := b.Finish(context.Background(), FinishOptions{Run: run}); err == nil {
		t.Error("overlapping finish accepted")
	}
	close(gate.resume)
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	if _, err := f.h.Down(context.Background(), run); err != nil {
		t.Fatal(err)
	}
	if f.procs.live() != 0 {
		t.Fatal("orphaned process")
	}
}

func TestUnsafeLedgerOperandsAreRejected(t *testing.T) {
	for _, key := range []string{"path", "pid", "font_scale", "address", "access"} {
		run := t.TempDir()
		l, err := loadLedger(run, "run")
		if err != nil {
			t.Fatal(err)
		}
		l.Resources = []*Resource{{Kind: kindDeviceAgent, Data: map[string]string{key: "1; id"}, State: stateActive}}
		if err := l.save(); err != nil {
			t.Fatal(err)
		}
		if _, err := loadLedger(run, "run"); err == nil {
			t.Fatalf("accepted unsafe %s", key)
		}
	}
}

func TestDNATInspectionAcceptsCanonicalIptablesOutput(t *testing.T) {
	h := &Helper{Runner: interceptRunner{call: func(Cmd) ([]byte, []byte, error, bool) {
		return []byte("-A OUTPUT -d 100.64.0.7/32 -p tcp -m tcp --dport 7247 -j DNAT --to-destination 127.0.0.1:7247"), nil, nil, true
	}}}
	exists, err := h.dnatExists(context.Background(), SDK{ADB: "adb"}, "emulator-5554", DefaultAddress)
	if err != nil || !exists {
		t.Fatalf("canonical rule not found: %v %v", exists, err)
	}
}

func TestInstalledIdentityAcceptsAndroidRandomizedPaths(t *testing.T) {
	path := "/data/app/~~random==/package-random==/base.apk"
	sum := strings.Repeat("ab", 32)
	h := &Helper{Runner: interceptRunner{call: func(c Cmd) ([]byte, []byte, error, bool) {
		script := c.Args[len(c.Args)-1]
		switch script {
		case "pm list packages " + AppID:
			return []byte("package:" + AppID), nil, nil, true
		case "pm path " + AppID:
			return []byte("package:" + path), nil, nil, true
		case "sha256sum '" + path + "'":
			return []byte(sum + "  " + path), nil, nil, true
		default:
			t.Fatalf("unexpected shell: %s", script)
		}
		return nil, nil, nil, true
	}}}
	identity, err := h.installedIdentity(context.Background(), SDK{ADB: "adb"}, "emulator-5554")
	if err != nil || identity != path+":"+sum {
		t.Fatalf("%q %v", identity, err)
	}
}

func TestDeviceStopAcceptsOnlyMatchingIdentityOrConfirmedAbsence(t *testing.T) {
	for _, absent := range []bool{false, true} {
		dir := deviceTmp + "/anshin-qa-run"
		killed := false
		output := "1234 (anshin-qa) " + strings.Repeat("0 ", 19) + "42\n" + dir + "/anshin-qa\n" + dir
		if absent {
			output = "absent"
		}
		h := &Helper{Runner: interceptRunner{call: func(c Cmd) ([]byte, []byte, error, bool) {
			if strings.Contains(strings.Join(c.Args, " "), "shell kill 1234") {
				killed = true
			}
			return []byte(output), nil, nil, true
		}}}
		r := &Resource{Data: map[string]string{"pid": "1234", "dir": dir, "identity": "42:" + dir + "/anshin-qa:" + dir}}
		if err := h.stopDeviceAgent(context.Background(), SDK{ADB: "adb"}, t.TempDir(), "emulator-5554", r); err != nil {
			t.Fatal(err)
		}
		if killed == absent {
			t.Fatalf("absent=%v killed=%v", absent, killed)
		}
	}
}
