package qa

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

func TestProofRequiresSuccessfulExactSHASupport(t *testing.T) {
	for _, body := range []string{
		`"command":"make check","exit":1`,
		`"command":"make check"`,
		`"ciUrl":"https://example.invalid/job"`,
		`"ciUrl":"https://example.invalid/job","ciConclusion":"success","exit":1`,
	} {
		f, run := planned(t)
		info, _ := loadRun(run)
		writeRunFile(t, run, "proofs.json", fmt.Sprintf(`{"outsideQA":[{"id":"P1","state":"PASS","sha":%q,%s}]}`, info.TargetSHA, body))
		r := finish(t, f, run, FinishOptions{Mode: "planning"})
		if r.OutsideQA[0].State == "PASS" {
			t.Fatalf("accepted %s", body)
		}
	}
}

func TestPlannedSetupMustMatchObservedValues(t *testing.T) {
	run := t.TempDir()
	writeRunFile(t, run, "screen.png", "png")
	plan := PlanScenario{ID: "S1", Cells: []PlanCell{{ID: "a", Theme: "tokyo-night", FontScale: "1.3", Appearance: "dark"}}}
	for _, field := range []string{"theme", "font", "appearance", "missing"} {
		cell := Cell{ID: "a", Result: "PASS", Theme: "tokyo-night", FontMultiplier: "1.3", Appearance: "dark", Evidence: []string{"screen.png"}}
		switch field {
		case "theme":
			cell.Theme = "ink-and-bone"
		case "font":
			cell.FontMultiplier = "1.0"
		case "appearance":
			cell.Appearance = "light"
		case "missing":
			cell.Theme = ""
		}
		if result := scenarioResult(run, plan, Observation{Result: "PASS", Cells: []Cell{cell}}, func(string, ...any) {}); result != "INCONCLUSIVE" {
			t.Fatalf("%s: %s", field, result)
		}
	}
}

func TestDevicePassRequiresAPKAndSetup(t *testing.T) {
	for _, missing := range []string{"no-build", "digest", "sha", "signing", "r8", "up"} {
		t.Run(missing, func(t *testing.T) {
			f, run := planned(t)
			if missing != "no-build" {
				if _, err := f.up(t, run); err != nil {
					t.Fatal(err)
				}
				if _, err := f.h.Down(context.Background(), run); err != nil {
					t.Fatal(err)
				}
				var apk APKRecord
				readJSON(filepath.Join(run, "build", "apk.json"), &apk)
				switch missing {
				case "digest":
					os.WriteFile(apk.Path, []byte("replaced"), 0600)
				case "sha":
					apk.TargetSHA = "other"
				case "signing":
					apk.SignatureVerified = false
				case "r8":
					apk.R8Mapping = false
				case "up":
					os.Remove(filepath.Join(run, "up.json"))
				}
				writeJSON(filepath.Join(run, "build", "apk.json"), apk)
			}
			writeRunFile(t, run, "observations/S1.json", `{"id":"S1","result":"PASS","cells":[{"id":"S1.a","result":"PASS","theme":"ink-and-bone","evidence":["media/s1.png"]}]}`)
			writeRunFile(t, run, "observations/S2.json", `{"id":"S2","result":"PASS","cells":[{"id":"S2.a","result":"PASS","theme":"tokyo-night","evidence":["media/s1.png"]}]}`)
			r := finish(t, f, run, FinishOptions{})
			if r.Result != "INCONCLUSIVE" || r.Gates.StageClean {
				t.Fatalf("%+v", r)
			}
		})
	}
}

func TestLocalPendingProofAndDirtyFinishBlockStage(t *testing.T) {
	r := Result{Result: "NA", Target: ResultTarget{CleanTree: true}, Cleanup: ResultCleanup{Status: "nothing-owned"}, OutsideQA: []ResultProof{{State: "PENDING"}}}
	if gates(r).StageClean {
		t.Fatal("pending local proof allowed")
	}
	r.OutsideQA[0].Phase = "future-ci"
	if !gates(r).StageClean {
		t.Fatal("future CI blocked stage")
	}
	f, run := planned(t)
	os.WriteFile(filepath.Join(f.repo, "change.txt"), []byte("dirty"), 0600)
	got := finish(t, f, run, FinishOptions{Mode: "na", Reason: "docs"})
	if got.Target.CleanTree || !got.Target.InitCleanTree || got.Gates.StageClean {
		t.Fatalf("%+v", got)
	}
}

func TestBrokenDoctorIsEnvironmentNotMissingRunner(t *testing.T) {
	for _, name := range []string{"android sdk build-tools", "adb", "git checkout", "missing-report", "passing-doctor"} {
		f, run := planned(t)
		if name == "passing-doctor" {
			writeJSON(filepath.Join(run, "doctor.json"), DoctorReport{DeviceRunOK: true, Checks: []Check{{Name: "kvm", OK: true}}})
		} else if name == "missing-report" {
			os.Remove(filepath.Join(run, "doctor.json"))
		} else {
			writeJSON(filepath.Join(run, "doctor.json"), DoctorReport{Checks: []Check{{Name: name, OK: false, Detail: "broken"}}})
		}
		r := finish(t, f, run, FinishOptions{Mode: "planning"})
		if r.ReasonCode != "environment" || r.Gates.StageClean {
			t.Fatalf("%s: %+v", name, r)
		}
	}
}

func TestFinishHeadInspectionErrorsBlockStage(t *testing.T) {
	f, run := planned(t)
	f.h.Runner = interceptRunner{next: f.emu, call: func(c Cmd) ([]byte, []byte, error, bool) {
		if c.Name == "git" {
			return nil, nil, errExit, true
		}
		return nil, nil, nil, false
	}}
	r := finish(t, f, run, FinishOptions{Mode: "na", Reason: "docs"})
	if r.Target.CleanTree || !r.Target.HeadMismatch || r.Gates.StageClean {
		t.Fatalf("%+v", r)
	}
}

func TestUnversionedReleaseCanSupplyDeviceEvidence(t *testing.T) {
	f, run := planned(t)
	up, err := f.up(t, run)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.h.Down(context.Background(), run); err != nil {
		t.Fatal(err)
	}
	up.APK.VersionName, up.APK.VersionCode = "", ""
	writeJSON(filepath.Join(run, "build", "apk.json"), up.APK)
	writeJSON(filepath.Join(run, "up.json"), up)
	writeRunFile(t, run, "observations/S1.json", `{"id":"S1","result":"PASS","cells":[{"id":"S1.a","result":"PASS","theme":"ink-and-bone","evidence":["media/s1.png"]}]}`)
	writeRunFile(t, run, "observations/S2.json", `{"id":"S2","result":"PASS","cells":[{"id":"S2.a","result":"PASS","theme":"tokyo-night","evidence":["media/s1.png"]}]}`)
	if r := finish(t, f, run, FinishOptions{}); r.Result != "PASS" {
		t.Fatalf("unversioned verified release rejected: %+v", r)
	}
}
