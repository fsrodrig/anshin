package qa

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"
	"time"
)

// Helper runs the operations behind scripts/qa. Its fields are the seams the tests replace.
type Helper struct {
	Repo   string
	Runner Runner
	Procs  ProcessControl
	Getenv func(string) string
	Now    func() time.Time
	Stdout io.Writer
	Stderr io.Writer

	logged *loggedRunner
}

func (h *Helper) run(ctx context.Context, c Cmd) ([]byte, []byte, error) {
	if h.logged != nil {
		return h.logged.Run(ctx, c)
	}
	return h.Runner.Run(ctx, c)
}

// useRun sends every later command's line to the run's commands.log.
func (h *Helper) useRun(run string) {
	h.logged = &loggedRunner{next: h.Runner, path: filepath.Join(run, "commands.log")}
}

// RunInfo pins the run to one commit. Every later operation compares the checkout with it.
type RunInfo struct {
	SchemaVersion int       `json:"schemaVersion"`
	RunID         string    `json:"runId"`
	Created       time.Time `json:"created"`
	Branch        string    `json:"branch"`
	TargetSHA     string    `json:"targetSha"`
	Base          string    `json:"base"`
	MergeBase     string    `json:"mergeBase"`
	PR            *int      `json:"pr"`
	CleanTree     bool      `json:"cleanTree"`
	Caller        string    `json:"caller"`
	// FocusedExclusions are changed files left out of focused.patch (still in changes.patch) for the planner to inspect
	// by name: bulky captures and binary goldens would bury the code diff.
	FocusedExclusions []string `json:"focusedExclusions"`
}

var focusedExcludes = []string{"contract/captures", "android/app/src/test/screenshots", "*.png", "*.ttf", "*.otf", "*.webp"}

func branchSlug(branch string) string {
	s := strings.NewReplacer("/", "-", " ", "-", "\\", "-").Replace(branch)
	if s == "" {
		s = "detached"
	}
	return s
}

func (h *Helper) stateDir() string {
	if d := h.Getenv("XDG_STATE_HOME"); d != "" {
		return filepath.Join(d, "anshin", "qa")
	}
	return filepath.Join(h.Getenv("HOME"), ".local", "state", "anshin", "qa")
}

func (h *Helper) git(ctx context.Context, args ...string) (string, error) {
	c := Cmd{Name: "git", Args: append([]string{"-C", h.Repo}, args...)}
	out, errb, err := h.run(ctx, c)
	if err != nil {
		return "", cmdError(c, errb, err)
	}
	return strings.TrimSpace(string(out)), nil
}

// headState reads the checkout's commit and whether it has uncommitted or untracked files.
func (h *Helper) headState(ctx context.Context) (sha string, clean bool, err error) {
	if sha, err = h.git(ctx, "rev-parse", "HEAD"); err != nil {
		return "", false, err
	}
	st, err := h.git(ctx, "status", "--porcelain=v1", "--untracked-files=normal")
	return sha, st == "", err
}

// Init makes a fresh run directory for the current checkout and writes the diffs the planner reads. It needs only
// git, so a planning-only or not-applicable run gets the same directory a device run does.
func (h *Helper) Init(ctx context.Context, base, caller string, pr *int) (string, RunInfo, error) {
	info := RunInfo{SchemaVersion: 1, Base: base, PR: pr, Caller: caller, Created: h.Now().UTC()}
	var err error
	if info.TargetSHA, info.CleanTree, err = h.headState(ctx); err != nil {
		return "", info, err
	}
	if info.Branch, err = h.git(ctx, "branch", "--show-current"); err != nil {
		return "", info, err
	}
	for _, ref := range []string{"origin/" + base, base} {
		if mb, err := h.git(ctx, "merge-base", "HEAD", ref); err == nil {
			info.MergeBase = mb
			break
		}
	}
	if info.MergeBase == "" {
		return "", info, fmt.Errorf("no merge base between HEAD and %s", base)
	}

	root := h.stateDir()
	if err := os.MkdirAll(root, 0o700); err != nil {
		return "", info, err
	}
	stem := branchSlug(info.Branch) + "-" + info.Created.Format("20060102T150405Z")
	var run string
	for i := 1; ; i++ {
		name := stem
		if i > 1 {
			name = fmt.Sprintf("%s-%d", stem, i)
		}
		run = filepath.Join(root, name)
		if err := os.Mkdir(run, 0o700); err == nil {
			info.RunID = name
			break
		} else if !errors.Is(err, os.ErrExist) || i > 50 {
			return "", info, err
		}
	}
	h.useRun(run)

	rng := info.MergeBase + "..HEAD"
	diffs := map[string][]string{
		"changes.patch":       {"diff", "--binary", rng},
		"changes.stat":        {"diff", "--stat=200", rng},
		"changes.name-status": {"diff", "--name-status", rng},
		"uncommitted.patch":   {"diff", "HEAD"},
	}
	focused := []string{"diff", rng, "--", "."}
	for _, ex := range focusedExcludes {
		if strings.HasPrefix(ex, "*") {
			focused = append(focused, ":(exclude,glob)**/"+ex)
		} else {
			focused = append(focused, ":(exclude)"+ex)
		}
	}
	diffs["focused.patch"] = focused
	for name, args := range diffs {
		out, err := h.git(ctx, args...)
		if err != nil {
			return run, info, err
		}
		if err := os.WriteFile(filepath.Join(run, name), []byte(out+"\n"), 0o600); err != nil {
			return run, info, err
		}
	}
	names, _ := h.git(ctx, "diff", "--name-only", rng)
	for _, f := range strings.Split(names, "\n") {
		if f != "" && excludedFromFocus(f) {
			info.FocusedExclusions = append(info.FocusedExclusions, f)
		}
	}
	return run, info, writeJSON(filepath.Join(run, "run.json"), info)
}

func excludedFromFocus(path string) bool {
	for _, ex := range focusedExcludes {
		if strings.HasPrefix(ex, "*") {
			if strings.HasSuffix(path, ex[1:]) {
				return true
			}
		} else if path == ex || strings.HasPrefix(path, ex+"/") {
			return true
		}
	}
	return false
}

func loadRun(run string) (RunInfo, error) {
	var info RunInfo
	if run == "" {
		return info, errors.New("--run is required")
	}
	if err := readJSON(filepath.Join(run, "run.json"), &info); err != nil {
		return info, fmt.Errorf("not a run directory made by `scripts/qa init`: %w", err)
	}
	return info, nil
}

// Check is one doctor line.
type Check struct {
	Name     string `json:"name"`
	OK       bool   `json:"ok"`
	Required string `json:"requiredFor"`
	Detail   string `json:"detail"`
}

// DoctorReport is what a coordinator reads before deciding whether a device run is possible.
type DoctorReport struct {
	SchemaVersion int          `json:"schemaVersion"`
	DeviceRunOK   bool         `json:"deviceRunOk"`
	Checks        []Check      `json:"checks"`
	Operations    []string     `json:"operations"`
	Routes        []string     `json:"routes"`
	Themes        []string     `json:"seedThemes"`
	Agent         []Capability `json:"agentCapabilities"`
	// Unverified lists what only `up` on a real emulator can show: doctor never touches a device.
	Unverified []string `json:"unverified"`
}

// Operations is the helper's command list, the one help, doctor and the skill agree on.
var Operations = []string{"help", "capabilities", "doctor", "init", "up", "agent start", "agent stop", "seed", "down", "finish"}

// Doctor checks the host without touching a device or starting adb's server. A device-run failure here is an
// environment gap the coordinator reports; it never makes doctor fail to answer.
func (h *Helper) Doctor(ctx context.Context) DoctorReport {
	r := DoctorReport{SchemaVersion: 1, Operations: Operations, Routes: []string{"reverse", "device"}, Themes: ThemeKeys, Agent: Capabilities(),
		Unverified: []string{"adb root on the emulator", "the iptables DNAT or loopback-alias route", "the QA Agent reachable from the app", "theme seed file permissions"}}
	add := func(name, requiredFor string, err error, detail string) {
		c := Check{Name: name, OK: err == nil, Required: requiredFor, Detail: detail}
		if err != nil {
			c.Detail = err.Error()
		}
		r.Checks = append(r.Checks, c)
	}
	sha, clean, err := h.headState(ctx)
	add("git checkout", "init", err, sha)
	if err == nil && !clean {
		add("clean tree", "up", errors.New("uncommitted or untracked files: a device run builds only a committed, clean SHA"), "")
	} else if err == nil {
		add("clean tree", "up", nil, "clean")
	}
	add("go", "up", nil, runtime.Version())
	_, kerr := exec.LookPath("keytool")
	add("keytool", "up", kerr, "throwaway QA signing key")
	_, gerr := os.Stat(filepath.Join(h.Repo, "android", "gradlew"))
	add("gradle wrapper", "up", gerr, "android/gradlew")
	sdk, serr := findSDK(h.Repo, h.Getenv)
	add("android sdk build-tools", "up", serr, sdk.BuildTools)
	if serr == nil || sdk.ADB != "" {
		aerr := error(nil)
		if !isExecutable(sdk.ADB) {
			aerr = errors.New("no platform-tools/adb in the SDK")
		}
		add("adb", "up, seed, down", aerr, sdk.ADB)
	}
	kvmErr := syscall.Access("/dev/kvm", 6)
	if kvmErr != nil {
		kvmErr = fmt.Errorf("/dev/kvm is not readable and writable: %w", kvmErr)
	}
	add("kvm", "emulator", kvmErr, "/dev/kvm")
	r.DeviceRunOK = true
	for _, c := range r.Checks {
		if !c.OK && c.Required != "init" {
			r.DeviceRunOK = false
		}
	}
	return r
}
