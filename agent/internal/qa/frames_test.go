package qa

import (
	"context"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestParseGfxinfoSummary(t *testing.T) {
	b, err := os.ReadFile("testdata/gfxinfo.txt")
	if err != nil {
		t.Fatal(err)
	}
	s := ParseGfxinfo(string(b))
	n := func(v int64) *int64 { return &v }
	want := GfxSummary{
		TotalFrames: n(1840), JankyFrames: n(37), JankyPct: new(2.01), P50Ms: n(7), P90Ms: n(12), P95Ms: n(15), P99Ms: n(28),
		MissedVsync: n(4), SlowUIThread: n(9), FrameDeadlineMissed: n(31),
	}
	got := s
	got.Fields = nil
	if !reflect.DeepEqual(got, want) {
		t.Errorf("parsed %+v, want the first block's values", got)
	}
	if len(s.Fields) != len(gfxSummaryKeys) || s.Fields["99th gpu percentile"] != "9ms" || s.Fields["Janky frames (legacy)"] != "52 (2.83%)" {
		t.Errorf("fields %v, want every summary key once, from the first block", s.Fields)
	}

	empty := ParseGfxinfo("No process found for: " + AppID)
	if empty.TotalFrames != nil || empty.JankyPct != nil || empty.P90Ms != nil || len(empty.Fields) != 0 {
		t.Errorf("output without a summary parsed as %+v; a missing value must stay null", empty)
	}
}

func TestFramesResetsAndCollectsOnlyTheRunsOwnInstall(t *testing.T) {
	ctx := context.Background()
	f := newFixture(t)
	run := f.init(t)
	dumpsys := func() int {
		n := 0
		for _, c := range f.emu.log() {
			if strings.Contains(c, "dumpsys") {
				n++
			}
		}
		return n
	}
	if _, err := f.h.Frames(ctx, FramesOptions{Run: run, Reset: true}); err == nil {
		t.Fatal("reset frames without an owned install")
	}
	if _, err := f.up(t, run, "-machine", "full", "-samples", "live"); err != nil {
		t.Fatal(err)
	}
	for _, o := range []FramesOptions{{Run: run}, {Run: run, Reset: true, Collect: "x"}, {Run: run, Collect: "../escape"}, {Run: run, Collect: "Detail Scroll"}} {
		if _, err := f.h.Frames(ctx, o); err == nil {
			t.Errorf("%+v: accepted", o)
		}
	}
	if dumpsys() != 0 {
		t.Fatal("a refused frames call reached the device")
	}

	if rec, err := f.h.Frames(ctx, FramesOptions{Run: run, Reset: true}); err != nil || rec.Action != "reset" || f.emu.gfxResets != 1 {
		t.Fatalf("reset: %+v %v, %d resets", rec, err, f.emu.gfxResets)
	}
	rec, err := f.h.Frames(ctx, FramesOptions{Run: run, Collect: "detail-scroll"})
	if err != nil {
		t.Fatal(err)
	}
	if rec.Raw != filepath.Join(run, "frames", "detail-scroll.txt") || rec.Path != filepath.Join(run, "frames", "detail-scroll.json") || *rec.Summary.TotalFrames != 1840 {
		t.Fatalf("collect: %+v", rec)
	}
	var saved GfxSummary
	if err := readJSON(rec.Path, &saved); err != nil || *saved.JankyFrames != 37 {
		t.Fatalf("saved summary %+v %v", saved, err)
	}
	raw, _ := os.ReadFile(rec.Raw)
	want, _ := os.ReadFile("testdata/gfxinfo.txt")
	if !strings.HasPrefix(string(raw), strings.TrimSpace(string(want))) {
		t.Error("the raw file is not the device's output")
	}
	if _, err := f.h.Frames(ctx, FramesOptions{Run: run, Collect: "detail-scroll"}); err == nil {
		t.Error("a second collect replaced the first one's evidence")
	}
	log, _ := os.ReadFile(filepath.Join(run, "commands.log"))
	if !strings.Contains(string(log), "shell 'dumpsys gfxinfo "+AppID+" reset'") || !strings.Contains(string(log), "shell 'dumpsys gfxinfo "+AppID+"'") {
		t.Errorf("commands.log lacks the dumpsys lines:\n%s", log)
	}

	before := dumpsys()
	os.WriteFile(filepath.Join(f.repo, "android", "app", "build", "installed.apk"), []byte("someone else's build"), 0o644)
	if _, err := f.h.Frames(ctx, FramesOptions{Run: run, Collect: "replaced"}); err == nil {
		t.Error("collected from an install the run didn't make")
	}
	owner := f.emu.marker
	f.emu.marker = "another-run"
	if _, err := f.h.Frames(ctx, FramesOptions{Run: run, Reset: true}); err == nil {
		t.Error("reset on an emulator another run claimed")
	}
	if dumpsys() != before {
		t.Error("frames reached a device or install the run doesn't own")
	}
	f.emu.marker = owner
	f.h.Down(ctx, run)
}
