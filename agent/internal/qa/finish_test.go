package qa

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const testPlan = `{
  "schemaVersion": 1,
  "scope": "theme engine",
  "scenarios": [
    {"id": "S1", "title": "Live Pulse in the default theme", "cells": [{"id": "S1.a", "state": "live", "theme": "ink-and-bone"}]},
    {"id": "S2", "title": "Seeded Tokyo Night restores after relaunch", "cells": [{"id": "S2.a", "theme": "tokyo-night"}], "gaps": ["G1"]},
    {"id": "S3", "title": "Optional smoke at 100.64.0.7", "required": false}
  ],
  "outsideQA": [{"id": "P1", "criterion": "no color literals", "proof": "ColorLiteralTest", "phase": "future-ci"}],
  "manualCriteria": [{"id": "M1", "criterion": "reviewer confirms accents", "confirmation": "a reviewer comment"}],
  "capabilityGaps": [{"id": "G1", "summary": "seed permissions unverified", "blocks": ["S2"]}]
}`

func writeRunFile(t *testing.T, run, name, body string) {
	t.Helper()
	os.MkdirAll(filepath.Dir(filepath.Join(run, name)), 0o700)
	if err := os.WriteFile(filepath.Join(run, name), []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
}

func planned(t *testing.T) (*fixture, string) {
	f := newFixture(t)
	run := f.init(t)
	writeRunFile(t, run, "plan.json", testPlan)
	writeRunFile(t, run, "plan.md", "# QA plan\n")
	writeJSON(filepath.Join(run, "doctor.json"), DoctorReport{Checks: []Check{{Name: "kvm", OK: false}}})
	writeRunFile(t, run, "media/s1.png", "png")
	return f, run
}

func finish(t *testing.T, f *fixture, run string, o FinishOptions) Result {
	t.Helper()
	o.Run = run
	r, err := f.h.Finish(context.Background(), o)
	if err != nil {
		t.Fatal(err)
	}
	return r
}

func TestPlanningOnlyIsInconclusiveNoRunnerAndClaimsNothing(t *testing.T) {
	f, run := planned(t)
	r := finish(t, f, run, FinishOptions{Mode: "planning", Reason: "no device in this environment"})
	if r.Result != "INCONCLUSIVE" || r.ReasonCode != "no_runner" || r.Gates.StageClean {
		t.Fatalf("%+v", r)
	}
	sum, _ := os.ReadFile(filepath.Join(run, "pr-summary.md"))
	if !strings.Contains(string(sum), "planning only") || !strings.Contains(string(sum), "not exercised") || strings.Contains(string(sum), "| PASS") {
		t.Fatalf("pr-summary:\n%s", sum)
	}
}

func TestEveryRequiredCellObservedIsAPass(t *testing.T) {
	f, run := planned(t)
	if _, err := f.up(t, run); err != nil {
		t.Fatal(err)
	}
	if _, err := f.h.Down(context.Background(), run); err != nil {
		t.Fatal(err)
	}
	writeRunFile(t, run, "observations/S1.json", `{"id":"S1","result":"PASS","cells":[{"id":"S1.a","result":"PASS","theme":"ink-and-bone","fontMultiplier":"1.0","evidence":["media/s1.png"]}]}`)
	writeRunFile(t, run, "observations/S2.json", `{"id":"S2","result":"PASS","cells":[{"id":"S2.a","result":"PASS","theme":"tokyo-night","evidence":["media/s1.png"]}]}`)
	r := finish(t, f, run, FinishOptions{})
	if r.Result != "PASS" || r.ReasonCode != "observed" {
		t.Fatalf("%+v", r)
	}
	if r.Gates.OutsideQA != "PENDING" || r.Gates.Manual != "PENDING" || r.Gates.StageClean != true {
		t.Fatalf("gates %+v: a device PASS with nothing owned is a clean stage, while outside-QA and manual gates stay pending", r.Gates)
	}
}

func TestAMissingRequiredCellKeepsTheRunInconclusive(t *testing.T) {
	f, run := planned(t)
	if _, err := f.up(t, run); err != nil {
		t.Fatal(err)
	}
	if _, err := f.h.Down(context.Background(), run); err != nil {
		t.Fatal(err)
	}
	writeRunFile(t, run, "observations/S1.json", `{"id":"S1","result":"PASS","cells":[{"id":"S1.a","result":"PASS","theme":"ink-and-bone","evidence":["media/s1.png"]}]}`)
	writeRunFile(t, run, "observations/S2.json", `{"id":"S2","result":"PASS","cells":[]}`)
	r := finish(t, f, run, FinishOptions{})
	if r.Result != "INCONCLUSIVE" || r.ReasonCode != "missing_observation" {
		t.Fatalf("%+v", r)
	}
}

func TestPassWithoutEvidenceIsNotAPass(t *testing.T) {
	f, run := planned(t)
	if _, err := f.up(t, run); err != nil {
		t.Fatal(err)
	}
	if _, err := f.h.Down(context.Background(), run); err != nil {
		t.Fatal(err)
	}
	writeRunFile(t, run, "observations/S1.json", `{"id":"S1","result":"PASS","cells":[{"id":"S1.a","result":"PASS","theme":"ink-and-bone","evidence":["media/missing.png"]}]}`)
	writeRunFile(t, run, "observations/S2.json", `{"id":"S2","result":"PASS","cells":[{"id":"S2.a","result":"PASS","theme":"tokyo-night","evidence":["/etc/passwd"]}]}`)
	r := finish(t, f, run, FinishOptions{})
	if r.Result != "INCONCLUSIVE" || len(r.Problems) < 2 {
		t.Fatalf("%+v", r)
	}
}

func TestOneFailedRequiredCellFailsTheRun(t *testing.T) {
	f, run := planned(t)
	if _, err := f.up(t, run); err != nil {
		t.Fatal(err)
	}
	if _, err := f.h.Down(context.Background(), run); err != nil {
		t.Fatal(err)
	}
	writeRunFile(t, run, "observations/S1.json", `{"id":"S1","result":"FAIL","cells":[{"id":"S1.a","result":"FAIL","theme":"ink-and-bone","evidence":["media/s1.png"]}]}`)
	r := finish(t, f, run, FinishOptions{})
	if r.Result != "FAIL" || r.ReasonCode != "observed_defect" {
		t.Fatalf("%+v", r)
	}
}

func TestNotApplicableNeedsAReason(t *testing.T) {
	f, run := planned(t)
	if _, err := f.h.Finish(context.Background(), FinishOptions{Run: run, Mode: "na"}); err == nil {
		t.Fatal("NA without a reason")
	}
	r := finish(t, f, run, FinishOptions{Mode: "na", Reason: "docs only"})
	if r.Result != "NA" || r.ReasonCode != "no_phone_runtime_change" {
		t.Fatalf("%+v", r)
	}
}

func TestAProofForAnotherSHAStaysPending(t *testing.T) {
	f, run := planned(t)
	writeRunFile(t, run, "proofs.json", `{"outsideQA":[{"id":"P1","state":"PASS","sha":"0000000","command":"make check","exit":0}],
		"manualCriteria":[{"id":"M1","state":"CONFIRMED","evidence":"review comment"}]}`)
	r := finish(t, f, run, FinishOptions{Mode: "planning"})
	if r.OutsideQA[0].State != "PENDING" || r.Gates.Manual != "CONFIRMED" {
		t.Fatalf("%+v", r)
	}
	var info RunInfo
	readJSON(filepath.Join(run, "run.json"), &info)
	writeRunFile(t, run, "proofs.json", `{"outsideQA":[{"id":"P1","state":"PASS","sha":"`+info.TargetSHA+`","command":"make check","exit":0}]}`)
	if r := finish(t, f, run, FinishOptions{Mode: "planning"}); r.OutsideQA[0].State != "PASS" || r.Gates.OutsideQA != "PASS" {
		t.Fatalf("%+v", r.OutsideQA)
	}
}

func TestAnOwnedDeviceWithoutDownIsNotAClean(t *testing.T) {
	f, run := planned(t)
	if _, err := f.up(t, run); err != nil {
		t.Fatal(err)
	}
	writeRunFile(t, run, "observations/S1.json", `{"id":"S1","result":"PASS","cells":[{"id":"S1.a","result":"PASS","theme":"ink-and-bone","evidence":["media/s1.png"]}]}`)
	writeRunFile(t, run, "observations/S2.json", `{"id":"S2","result":"PASS","cells":[{"id":"S2.a","result":"PASS","theme":"tokyo-night","evidence":["media/s1.png"]}]}`)
	r := finish(t, f, run, FinishOptions{})
	if r.Result != "PASS" || r.Cleanup.Status != "not-run" || r.Gates.StageClean {
		t.Fatalf("observations passed but cleanup did not run: %+v %+v", r.Cleanup, r.Gates)
	}
	if _, err := f.h.Down(context.Background(), run); err != nil {
		t.Fatal(err)
	}
	if r := finish(t, f, run, FinishOptions{}); r.Cleanup.Status != "ok" || !r.Gates.StageClean || r.APK == nil {
		t.Fatalf("%+v %+v", r.Cleanup, r.Gates)
	}
}

func TestTheSummaryCarriesNoPrivateToken(t *testing.T) {
	f, run := planned(t)
	r := finish(t, f, run, FinishOptions{Mode: "planning"})
	sum, _ := os.ReadFile(filepath.Join(run, "pr-summary.md"))
	if strings.Contains(string(sum), "100.64.0.7") || strings.Contains(string(sum), run) {
		t.Fatalf("pr-summary leaks:\n%s", sum)
	}
	if !strings.Contains(strings.Join(r.Problems, "\n"), "redacted") {
		t.Fatalf("a redaction must be reported so the plan text gets fixed: %v", r.Problems)
	}
	report, _ := os.ReadFile(filepath.Join(run, "report.md"))
	if !strings.Contains(string(report), run) {
		t.Fatal("the private report should link the run's files")
	}
}

func TestPublicSafeRedactsAddressesPathsAndSerials(t *testing.T) {
	in := "at 100.64.0.7:7247 on emulator-5554, see /home/user/x and ~/y, box.example.ts.net, fd7a:115c:a1e0::1, 00:00:00:00:00:00, by machinehost"
	out, n := publicSafe(in, []string{"machinehost"})
	for _, leak := range []string{"100.64", "emulator-5554", "/home", "~/", "ts.net", "fd7a", "00:00:00", "machinehost"} {
		if strings.Contains(out, leak) {
			t.Errorf("%q survived: %s", leak, out)
		}
	}
	if n < 8 {
		t.Errorf("counted %d redactions", n)
	}
	if clean, n := publicSafe("Device QA: **PASS** on `c492ff4db5f0`; S1 Live Pulse", nil); n != 0 || !strings.Contains(clean, "c492ff4db5f0") {
		t.Errorf("redacted an ordinary line: %s", clean)
	}
}

func TestTheManifestNeverListsSigningMaterialOrMarksAnythingPublic(t *testing.T) {
	f, run := planned(t)
	if _, err := f.up(t, run); err != nil {
		t.Fatal(err)
	}
	f.h.Down(context.Background(), run)
	finish(t, f, run, FinishOptions{})
	var m []ManifestEntry
	if err := readJSON(filepath.Join(run, "evidence-manifest.json"), &m); err != nil {
		t.Fatal(err)
	}
	for _, e := range m {
		if strings.HasPrefix(e.Path, "signing") || e.Public {
			t.Fatalf("manifest entry %+v", e)
		}
	}
}

func TestCIProofForAnotherSHAStaysPending(t *testing.T) {
	f, run := planned(t)
	writeRunFile(t, run, "proofs.json", `{"outsideQA":[{"id":"P1","state":"PASS","sha":"0000000","ciUrl":"https://example.invalid/actions/runs/1"}]}`)
	r := finish(t, f, run, FinishOptions{Mode: "planning"})
	if r.OutsideQA[0].State != "PENDING" {
		t.Fatalf("old CI proof accepted: %+v", r.OutsideQA[0])
	}
}
