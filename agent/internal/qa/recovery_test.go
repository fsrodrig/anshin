package qa

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"
)

func TestInstallRecovery(t *testing.T) {
	for _, failure := range []string{"inspection", "response"} {
		for _, replacement := range []bool{false, true} {
			t.Run(failure+map[bool]string{false: "-owned", true: "-replacement"}[replacement], func(t *testing.T) {
				f := newFixture(t)
				run := f.init(t)
				if failure == "inspection" {
					f.emu.fail["sha256sum '"] = errExit
				} else {
					f.h.Runner = interceptRunner{next: f.emu, call: func(c Cmd) ([]byte, []byte, error, bool) {
						if strings.Contains(strings.Join(c.Args, " "), " install ") {
							a, b, _ := f.emu.Run(context.Background(), c)
							return a, b, errExit, true
						}
						return nil, nil, nil, false
					}}
				}
				if _, err := f.up(t, run); err == nil {
					t.Fatal("expected interrupted install")
				}
				delete(f.emu.fail, "sha256sum '")
				f.h.Runner = f.emu
				if !f.emu.installed {
					t.Fatal("install did not happen")
				}
				if replacement {
					f.h.Runner = interceptRunner{next: f.emu, call: func(c Cmd) ([]byte, []byte, error, bool) {
						if strings.Contains(strings.Join(c.Args, " "), "sha256sum '") {
							return []byte(strings.Repeat("ef", 32)), nil, nil, true
						}
						return nil, nil, nil, false
					}}
				}
				for range 2 {
					_, err := f.h.Down(context.Background(), run)
					if replacement {
						if err == nil || !f.emu.installed || f.emu.marker == "" {
							t.Fatalf("replacement removed: %v", err)
						}
					} else if err != nil || f.emu.installed || f.emu.marker != "" {
						t.Fatalf("cleanup failed: %v", err)
					}
				}
			})
		}
	}
}

func TestDeviceLaunchRecovery(t *testing.T) {
	for _, failure := range []string{"inspection", "response", "exec", "reused", "transport", "before-launch"} {
		t.Run(failure, func(t *testing.T) {
			run := t.TempDir()
			l, err := loadLedger(run, "run")
			if err != nil {
				t.Fatal(err)
			}
			dir := deviceTmp + "/anshin-qa-run"
			stat := "1234 (anshin-qa) " + strings.Repeat("0 ", 19) + "42"
			receipt := "1234\n" + stat + "\n" + dir + "/anshin-qa\n" + dir
			alive, inspected, killed := false, false, false
			h := &Helper{Runner: interceptRunner{call: func(c Cmd) ([]byte, []byte, error, bool) {
				script := c.Args[len(c.Args)-1]
				switch {
				case strings.HasPrefix(script, "cd "):
					if failure == "before-launch" {
						return nil, nil, errExit, true
					}
					alive = true
					if failure == "response" || failure == "transport" {
						return nil, nil, errExit, true
					}
					return []byte(receipt), nil, nil, true
				case strings.HasPrefix(script, "cat "+dir+"/launch-"):
					if failure == "transport" || failure == "before-launch" {
						return nil, nil, errExit, true
					}
					return []byte(receipt), nil, nil, true
				case strings.HasPrefix(script, "if [ ! -d /proc/"):
					if !inspected {
						inspected = true
						return nil, nil, errExit, true
					}
					if failure == "exec" {
						alive = false
						return []byte("absent"), nil, nil, true
					}
					out := strings.TrimPrefix(receipt, "1234\n")
					if failure == "reused" {
						out = strings.Replace(out, "42", "43", 1)
					}
					return []byte(out), nil, nil, true
				case script == "kill 1234":
					killed = true
					alive = false
					return nil, nil, nil, true
				case len(c.Args) > 2 && c.Args[2] == "pull":
					return nil, nil, nil, true
				}
				t.Fatalf("unexpected command: %v", c)
				return nil, nil, nil, true
			}}}
			if err := h.startDeviceAgent(context.Background(), SDK{ADB: "adb"}, l, run, "emulator-5554", DefaultAddress, nil); err == nil {
				t.Fatal("expected launch failure")
			}
			l, err = loadLedger(run, "run")
			if err != nil {
				t.Fatal(err)
			}
			// The first inspection may never have run if the launch response was lost.
			inspected = true
			err = h.stopDeviceAgent(context.Background(), SDK{ADB: "adb"}, run, "emulator-5554", l.live(kindDeviceAgent)[0])
			if failure == "reused" || failure == "transport" || failure == "before-launch" {
				if err == nil || killed {
					t.Fatalf("unsafe recovery: %v killed=%v", err, killed)
				}
			} else if err != nil || alive || killed != (failure != "exec") {
				t.Fatalf("recovery: %v alive=%v killed=%v", err, alive, killed)
			}
		})
	}
}

func TestDeviceLaunchScriptPersistsExecIdentity(t *testing.T) {
	dir := t.TempDir()
	binary, err := os.ReadFile("/bin/sleep")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "anshin-qa"), binary, 0700); err != nil {
		t.Fatal(err)
	}
	receipt := filepath.Join(dir, "launch-1.identity")
	script := deviceLaunchScript(dir, receipt, []string{"30"}, filepath.Join(dir, "access"), filepath.Join(dir, "stderr"))
	out, err := exec.Command("sh", "-c", script).Output()
	if err != nil {
		t.Fatalf("launch: %v", err)
	}
	r := &Resource{Data: map[string]string{"dir": dir}}
	if err := recordDeviceLaunch(r, strings.TrimSpace(string(out))); err != nil {
		t.Fatal(err)
	}
	pid, _ := strconv.Atoi(r.Data["pid"])
	t.Cleanup(func() { syscall.Kill(pid, syscall.SIGKILL) })
	stored, err := os.ReadFile(receipt)
	if err != nil || strings.TrimSpace(string(stored)) != strings.TrimSpace(string(out)) {
		t.Fatalf("receipt: %q %v", stored, err)
	}
	for range 100 {
		stat, err := os.ReadFile("/proc/" + r.Data["pid"] + "/stat")
		if err != nil {
			t.Fatal(err)
		}
		exe, _ := os.Readlink("/proc/" + r.Data["pid"] + "/exe")
		cwd, _ := os.Readlink("/proc/" + r.Data["pid"] + "/cwd")
		if exe == dir+"/anshin-qa" {
			identity, err := parseDeviceProcessIdentity(strings.TrimSpace(string(stat))+"\n"+exe+"\n"+cwd, dir)
			if err != nil || identity != r.Data["identity"] {
				t.Fatalf("exec identity: %q %v", identity, err)
			}
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("child never executed QA binary")
}

func TestMarkerCleanupRecovery(t *testing.T) {
	for _, failure := range []string{"claim-before", "claim-response", "delete-response"} {
		t.Run(failure, func(t *testing.T) {
			f := newFixture(t)
			run := f.init(t)
			lost := false
			if failure == "claim-before" {
				f.emu.fail["set -C;"] = errExit
			} else {
				f.h.Runner = interceptRunner{next: f.emu, call: func(c Cmd) ([]byte, []byte, error, bool) {
					line := strings.Join(c.Args, " ")
					target := "shell rm -f " + ownerMarker
					if failure == "claim-response" {
						target = "shell set -C;"
					}
					if !lost && strings.Contains(line, target) {
						lost = true
						a, b, _ := f.emu.Run(context.Background(), c)
						return a, b, errExit, true
					}
					return nil, nil, nil, false
				}}
			}
			_, err := f.up(t, run)
			if failure == "delete-response" {
				if err != nil {
					t.Fatal(err)
				}
				if _, err = f.h.Down(context.Background(), run); err == nil {
					t.Fatal("expected lost deletion response")
				}
				if f.emu.marker != "" || f.emu.installed || f.emu.dnat {
					t.Fatal("marker removed before dependencies")
				}
			} else if err == nil {
				t.Fatal("expected failed claim")
			}
			delete(f.emu.fail, "set -C;")
			for range 2 {
				if _, err := f.h.Down(context.Background(), run); err != nil {
					t.Fatal(err)
				}
			}
			if f.emu.marker != "" || f.emu.installed || f.emu.root {
				t.Fatal("cleanup left resources")
			}
		})
	}
}

func TestMarkerCleanupRequiresConfirmedAbsence(t *testing.T) {
	for _, out := range []string{"absent", "", "present\nother-run", "present\nrun"} {
		for _, inspectionFailure := range []bool{false, true} {
			removed := false
			h := &Helper{Runner: interceptRunner{call: func(c Cmd) ([]byte, []byte, error, bool) {
				script := c.Args[len(c.Args)-1]
				if script == "rm -f "+ownerMarker {
					removed = true
					return nil, nil, nil, true
				}
				if !strings.HasPrefix(script, "if [ -e "+ownerMarker) {
					t.Fatalf("unexpected inspection: %s", script)
				}
				var err error
				if inspectionFailure {
					err = errExit
				}
				return []byte(out), nil, err, true
			}}}
			err := h.undo(context.Background(), SDK{ADB: "adb"}, &Ledger{RunID: "run"}, t.TempDir(), &Resource{Kind: kindMarker})
			wantOK := !inspectionFailure && (out == "absent" || out == "present\nrun")
			if (err == nil) != wantOK || removed != (!inspectionFailure && out == "present\nrun") {
				t.Fatalf("out=%q failure=%v err=%v removed=%v", out, inspectionFailure, err, removed)
			}
		}
	}
}
