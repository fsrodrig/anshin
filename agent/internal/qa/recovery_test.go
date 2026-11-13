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
				case strings.HasPrefix(script, "mkdir ") && strings.HasSuffix(script, ".launch"):
					return nil, nil, nil, true
				case strings.HasPrefix(script, "mkdir "):
					if failure == "before-launch" {
						return nil, nil, errExit, true
					}
					alive = true
					if failure == "response" || failure == "transport" {
						return nil, nil, errExit, true
					}
					if strings.Contains(script, "& echo $!") {
						return []byte("1234"), nil, nil, true
					}
					return []byte(receipt), nil, nil, true
				case strings.HasPrefix(script, "if mkdir "+dir+"/launch-"):
					if failure == "transport" {
						return nil, nil, errExit, true
					}
					if failure == "before-launch" {
						return []byte("nonlaunch"), nil, nil, true
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
			if failure == "reused" || failure == "transport" {
				if err == nil || killed {
					t.Fatalf("unsafe recovery: %v killed=%v", err, killed)
				}
			} else if err != nil || alive || killed != (failure != "exec" && failure != "before-launch") {
				t.Fatalf("recovery: %v alive=%v killed=%v", err, alive, killed)
			}
		})
	}
}

func TestDeviceLaunchScriptPersistsExecIdentity(t *testing.T) {
	dir := t.TempDir()
	writeDeviceLaunchTestBinary(t, dir)
	receipt := filepath.Join(dir, "launch-1.identity")
	if err := os.Mkdir(receipt+".launch", 0700); err != nil {
		t.Fatal(err)
	}
	script := deviceLaunchScript(dir, receipt, []string{"-test.run=^TestDeviceLaunchChildProcess$"}, filepath.Join(dir, "access"), filepath.Join(dir, "stderr"))
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

func TestDeviceLaunchScriptRejectsExecFailure(t *testing.T) {
	dir := t.TempDir()
	receipt := filepath.Join(dir, "launch-1.identity")
	if err := os.Mkdir(receipt+".launch", 0700); err != nil {
		t.Fatal(err)
	}
	script := deviceLaunchScript(dir, receipt, nil, filepath.Join(dir, "access"), filepath.Join(dir, "stderr"))
	if _, err := exec.Command("sh", "-c", script).Output(); err == nil {
		t.Fatal("missing executable acknowledged as launched")
	}
	out, err := os.ReadFile(receipt)
	if err != nil {
		t.Fatal(err)
	}
	if err := recordDeviceLaunch(&Resource{Data: map[string]string{"dir": dir}}, strings.TrimSpace(string(out))); err != nil {
		t.Fatal(err)
	}
}

func TestDeviceNonlaunchRecoveryShell(t *testing.T) {
	for _, failure := range []string{"cd", "redirection", "receipt-write", "never-executed"} {
		t.Run(failure, func(t *testing.T) {
			dir := t.TempDir()
			receipt := filepath.Join(dir, "launch-1.identity")
			if err := os.Mkdir(receipt+".launch", 0700); err != nil {
				t.Fatal(err)
			}
			cwd, access := dir, filepath.Join(dir, "access")
			switch failure {
			case "cd":
				cwd = filepath.Join(dir, "missing")
			case "redirection":
				access = filepath.Join(dir, "missing", "access")
			case "receipt-write":
				if err := os.Mkdir(receipt+".tmp", 0700); err != nil {
					t.Fatal(err)
				}
			}
			script := deviceLaunchScript(cwd, receipt, nil, access, filepath.Join(dir, "stderr"))
			if failure != "never-executed" {
				if out, err := exec.Command("sh", "-c", script).CombinedOutput(); err == nil {
					t.Fatalf("launch unexpectedly succeeded: %s", out)
				}
			}
			if _, err := os.Stat(receipt); !os.IsNotExist(err) {
				t.Fatalf("expected no receipt: %v", err)
			}
			for range 2 {
				out, err := exec.Command("sh", "-c", deviceRecoverLaunchScript(receipt)).Output()
				if err != nil || strings.TrimSpace(string(out)) != "nonlaunch" {
					t.Fatalf("recovery: %q %v", out, err)
				}
			}
			if out, err := exec.Command("sh", "-c", script).CombinedOutput(); err == nil {
				t.Fatalf("cancelled launch admitted: %s", out)
			}
			// A later up may recreate the run directory, but not this launch's parent.
			if err := os.RemoveAll(receipt + ".launch"); err != nil {
				t.Fatal(err)
			}
			// A queued transport must not start after cleanup has cancelled its launch.
			if out, err := exec.Command("sh", "-c", script).CombinedOutput(); err == nil {
				t.Fatalf("late launch admitted: %s", out)
			}
		})
	}
}

func TestDeviceLaunchRecoveryRejectsUnknownShellState(t *testing.T) {
	for _, state := range []string{"pending", "partial", "replacement", "state-symlink", "gate-symlink"} {
		t.Run(state, func(t *testing.T) {
			dir := t.TempDir()
			receipt := filepath.Join(dir, "launch-1.identity")
			if err := os.Mkdir(receipt+".launch", 0700); err != nil {
				t.Fatal(err)
			}
			gate := receipt + ".launch/gate"
			if state == "gate-symlink" {
				if err := os.Symlink(dir, gate); err != nil {
					t.Fatal(err)
				}
			} else {
				if err := os.Mkdir(gate, 0700); err != nil {
					t.Fatal(err)
				}
				if state == "state-symlink" {
					other := filepath.Join(dir, "other")
					if err := os.WriteFile(other, []byte("nonlaunch\n"), 0600); err != nil {
						t.Fatal(err)
					}
					if err := os.Symlink(other, gate+"/state"); err != nil {
						t.Fatal(err)
					}
				} else if state != "pending" {
					if err := os.WriteFile(gate+"/state", []byte(state), 0600); err != nil {
						t.Fatal(err)
					}
				}
			}
			if out, err := exec.Command("sh", "-c", deviceRecoverLaunchScript(receipt)).CombinedOutput(); err == nil {
				t.Fatalf("unknown state accepted: %s", out)
			}
		})
	}
}

func TestDeviceNonlaunchDownRecovery(t *testing.T) {
	for _, failure := range []string{"preparation", "never-executed", "redirection", "receipt-write", "pending", "transport"} {
		t.Run(failure, func(t *testing.T) {
			f := newFixture(t)
			run := f.init(t)
			if _, err := f.up(t, run); err != nil {
				t.Fatal(err)
			}
			info, err := loadRun(run)
			if err != nil {
				t.Fatal(err)
			}
			l, err := loadLedger(run, info.RunID)
			if err != nil {
				t.Fatal(err)
			}
			dir := deviceTmp + "/anshin-qa-" + l.RunID
			local := t.TempDir()
			f.h.Runner = interceptRunner{next: f.emu, call: func(c Cmd) ([]byte, []byte, error, bool) {
				script := c.Args[len(c.Args)-1]
				if strings.HasPrefix(script, "mkdir "+dir+"/launch-") && strings.HasSuffix(script, ".launch") {
					if failure == "preparation" {
						return nil, nil, errExit, true
					}
					out, err := exec.Command("sh", "-c", strings.ReplaceAll(script, dir, local)).Output()
					return out, nil, err, true
				}
				if strings.HasPrefix(script, "mkdir "+dir+"/launch-") || strings.HasPrefix(script, "cd "+dir+" ") {
					receipt := filepath.Join(local, "launch-"+strconv.Itoa(len(l.Resources))+".identity")
					switch failure {
					case "never-executed", "transport":
						return nil, nil, errExit, true
					case "pending":
						if err := os.Mkdir(receipt+".launch/gate", 0700); err != nil {
							t.Fatal(err)
						}
						return nil, nil, errExit, true
					case "receipt-write":
						if err := os.Mkdir(receipt+".tmp", 0700); err != nil {
							t.Fatal(err)
						}
					case "redirection":
						// Fail redirection before the child shell can create its receipt.
						script = strings.Replace(script, "> "+dir+"/access-", "> "+dir+"/missing/access-", 1)
					}
					out, err := exec.Command("sh", "-c", strings.ReplaceAll(script, dir, local)).CombinedOutput()
					return out, nil, err, true
				}
				if strings.HasPrefix(script, "if mkdir "+dir+"/launch-") || strings.HasPrefix(script, "cat "+dir+"/launch-") {
					if failure == "transport" {
						return nil, nil, errExit, true
					}
					out, err := exec.Command("sh", "-c", strings.ReplaceAll(script, dir, local)).Output()
					return out, nil, err, true
				}
				return nil, nil, nil, false
			}}
			f.h.useRun(run)
			if err := f.h.startDeviceAgent(context.Background(), SDK{ADB: "adb"}, l, run, l.Serial, DefaultAddress, nil); err == nil {
				t.Fatal("expected failed launch")
			}
			for range 2 {
				_, err := f.h.Down(context.Background(), run)
				if failure == "pending" || failure == "transport" {
					if err == nil || f.emu.marker == "" || !f.emu.root {
						t.Fatalf("unknown launch released dependencies: %v", err)
					}
				} else if err != nil || f.emu.marker != "" || f.emu.root || f.emu.dnat || f.emu.installed {
					t.Fatalf("nonlaunch retained dependencies: %v", err)
				}
			}
		})
	}
}

func TestDeviceLaunchCancellationRace(t *testing.T) {
	for range 10 {
		dir := t.TempDir()
		writeDeviceLaunchTestBinary(t, dir)
		receipt := filepath.Join(dir, "launch-1.identity")
		if err := os.Mkdir(receipt+".launch", 0700); err != nil {
			t.Fatal(err)
		}
		launch := exec.Command("sh", "-c", deviceLaunchScript(dir, receipt, []string{"-test.run=^TestDeviceLaunchChildProcess$"}, filepath.Join(dir, "access"), filepath.Join(dir, "stderr")))
		if err := launch.Start(); err != nil {
			t.Fatal(err)
		}
		out, recoverErr := exec.Command("sh", "-c", deviceRecoverLaunchScript(receipt)).Output()
		launchErr := launch.Wait()
		if recoverErr != nil {
			// A launch that claimed the gate may still be writing its receipt.
			out, recoverErr = exec.Command("sh", "-c", deviceRecoverLaunchScript(receipt)).Output()
		}
		if recoverErr != nil {
			t.Fatalf("settled launch unrecoverable: %v", recoverErr)
		}
		if strings.TrimSpace(string(out)) == "nonlaunch" {
			if launchErr == nil {
				t.Fatal("cancelled launch executed")
			}
			if _, err := os.Stat(receipt); !os.IsNotExist(err) {
				t.Fatalf("cancelled launch created receipt: %v", err)
			}
			continue
		}
		r := &Resource{Data: map[string]string{"dir": dir}}
		if err := recordDeviceLaunch(r, strings.TrimSpace(string(out))); err != nil {
			t.Fatal(err)
		}
		pid, _ := strconv.Atoi(r.Data["pid"])
		syscall.Kill(pid, syscall.SIGKILL)
		if launchErr != nil {
			t.Fatalf("owned launch failed: %v", launchErr)
		}
	}
}

func writeDeviceLaunchTestBinary(t *testing.T, dir string) {
	t.Helper()
	t.Setenv("ANSHIN_QA_TEST_CHILD", "1")
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	// Multicall utilities such as Ubuntu's sleep cannot run under the QA binary's name.
	binary, err := os.ReadFile(executable)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "anshin-qa"), binary, 0700); err != nil {
		t.Fatal(err)
	}
}

func TestDeviceLaunchChildProcess(t *testing.T) {
	if os.Getenv("ANSHIN_QA_TEST_CHILD") == "1" {
		time.Sleep(30 * time.Second)
	}
}
