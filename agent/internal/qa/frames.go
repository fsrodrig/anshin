package qa

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"
)

// FramesOptions selects one frames operation: reset the app's frame statistics, or collect them under a name.
type FramesOptions struct {
	Run     string
	Reset   bool
	Collect string
}

// FramesRecord is what frames hands back. Summary is nil for a reset.
type FramesRecord struct {
	Action  string      `json:"action"`
	Command string      `json:"command"`
	Raw     string      `json:"raw,omitempty"`
	Path    string      `json:"summaryPath,omitempty"`
	Summary *GfxSummary `json:"summary,omitempty"`
}

// GfxSummary is the summary block of `dumpsys gfxinfo`. A field is null when the output didn't carry it, so a
// missing line never reads as zero frames or zero jank.
type GfxSummary struct {
	TotalFrames         *int64            `json:"totalFrames"`
	JankyFrames         *int64            `json:"jankyFrames"`
	JankyPct            *float64          `json:"jankyPct"`
	P50Ms               *int64            `json:"p50Ms"`
	P90Ms               *int64            `json:"p90Ms"`
	P95Ms               *int64            `json:"p95Ms"`
	P99Ms               *int64            `json:"p99Ms"`
	MissedVsync         *int64            `json:"missedVsync"`
	SlowUIThread        *int64            `json:"slowUiThread"`
	FrameDeadlineMissed *int64            `json:"frameDeadlineMissed"`
	Fields              map[string]string `json:"fields"`
}

var gfxSummaryKeys = []string{
	"Stats since", "Total frames rendered", "Janky frames", "Janky frames (legacy)",
	"50th percentile", "90th percentile", "95th percentile", "99th percentile",
	"Number Missed Vsync", "Number High input latency", "Number Slow UI thread", "Number Slow bitmap uploads",
	"Number Slow issue draw commands", "Number Frame deadline missed", "Number Frame deadline missed (legacy)",
	"50th gpu percentile", "90th gpu percentile", "95th gpu percentile", "99th gpu percentile",
}

var framesName = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{0,63}$`)

// ParseGfxinfo reads the first occurrence of each summary key: newer Androids repeat the block per window, and the
// first one is the whole app's.
func ParseGfxinfo(out string) GfxSummary {
	s := GfxSummary{Fields: map[string]string{}}
	sc := bufio.NewScanner(strings.NewReader(out))
	for sc.Scan() {
		key, value, ok := strings.Cut(strings.TrimSpace(sc.Text()), ": ")
		if !ok || !slices.Contains(gfxSummaryKeys, key) {
			continue
		}
		if _, seen := s.Fields[key]; !seen {
			s.Fields[key] = strings.TrimSpace(value)
		}
	}
	count := func(key string) *int64 {
		v, err := strconv.ParseInt(strings.Fields(s.Fields[key] + " x")[0], 10, 64)
		if err != nil {
			return nil
		}
		return &v
	}
	ms := func(key string) *int64 {
		v, err := strconv.ParseInt(strings.TrimSuffix(s.Fields[key], "ms"), 10, 64)
		if err != nil {
			return nil
		}
		return &v
	}
	s.TotalFrames, s.JankyFrames = count("Total frames rendered"), count("Janky frames")
	if m := regexp.MustCompile(`\(([0-9.]+)%\)`).FindStringSubmatch(s.Fields["Janky frames"]); m != nil {
		if v, err := strconv.ParseFloat(m[1], 64); err == nil {
			s.JankyPct = &v
		}
	}
	s.P50Ms, s.P90Ms, s.P95Ms, s.P99Ms = ms("50th percentile"), ms("90th percentile"), ms("95th percentile"), ms("99th percentile")
	s.MissedVsync, s.SlowUIThread, s.FrameDeadlineMissed = count("Number Missed Vsync"), count("Number Slow UI thread"), count("Number Frame deadline missed")
	return s
}

// Frames resets or collects the installed app's frame statistics. It reads only what the app rendered while the
// runner drove it through the bound command; it never drives the UI itself.
func (h *Helper) Frames(ctx context.Context, o FramesOptions) (FramesRecord, error) {
	var rec FramesRecord
	if o.Reset == (o.Collect != "") {
		return rec, errors.New("frames needs exactly one of --reset or --collect NAME")
	}
	if o.Collect != "" && !framesName.MatchString(o.Collect) {
		return rec, fmt.Errorf("--collect %q: want lowercase letters, digits and dashes", o.Collect)
	}
	unlock, lockErr := lockRun(o.Run)
	if lockErr != nil {
		return rec, lockErr
	}
	defer unlock()
	_, l, err := h.openRun(o.Run)
	if err != nil {
		return rec, err
	}
	if !l.owns(kindInstall) {
		return rec, errors.New("this run did not install the app, so it won't read the app's frame statistics")
	}
	sdk, err := findSDK(h.Repo, h.Getenv)
	if err != nil {
		return rec, err
	}
	for _, installed := range l.live(kindInstall) {
		if err := h.requireInstall(ctx, sdk, l, installed); err != nil {
			return rec, err
		}
	}
	if identity, err := h.installedIdentity(ctx, sdk, l.Serial); err != nil {
		return rec, err
	} else if identity == "" {
		return rec, errors.New("the run's installed app is absent")
	}

	rec.Command = "dumpsys gfxinfo " + AppID
	if o.Reset {
		rec.Action, rec.Command = "reset", rec.Command+" reset"
	} else {
		rec.Action = "collect"
	}
	dir := filepath.Join(o.Run, "frames")
	raw, path := filepath.Join(dir, o.Collect+".txt"), filepath.Join(dir, o.Collect+".json")
	if !o.Reset {
		if err := os.MkdirAll(dir, 0o700); err != nil {
			return rec, err
		}
		// A collect is evidence: a repeated name must not replace what an earlier measurement recorded.
		for _, p := range []string{raw, path} {
			if _, err := os.Stat(p); err == nil {
				return rec, fmt.Errorf("%s already exists; collect under another name", p)
			}
		}
	}
	out, err := h.shell(ctx, sdk, l.Serial, rec.Command)
	if err != nil || o.Reset {
		return rec, err
	}
	if err := os.WriteFile(raw, []byte(out+"\n"), 0o600); err != nil {
		return rec, err
	}
	summary := ParseGfxinfo(out)
	if summary.TotalFrames == nil {
		return rec, fmt.Errorf("no frame summary in the gfxinfo output; it is kept in %s", raw)
	}
	rec.Raw, rec.Path, rec.Summary = raw, path, &summary
	return rec, writeJSON(path, summary)
}
