package qa

import (
	"context"
	"errors"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"
)

var errExit = errors.New("exit status 1")

// fakeEmulator answers the helper's adb, Gradle, signing and go commands as a rooted non-Play emulator would, and
// keeps the device state they change, so a test can assert what the run left behind.
type fakeEmulator struct {
	t    *testing.T
	repo string

	mu         sync.Mutex
	cmds       []string
	root       bool
	installed  bool
	running    bool
	marker     string
	reverses   map[string]string
	dnat       bool
	font       string
	night      string
	pushed     map[string]string
	seeded     string
	fail       map[string]error // command substring -> injected failure
	debuggable bool
	pkg        string
}

func newFakeEmulator(t *testing.T, repo string) *fakeEmulator {
	return &fakeEmulator{t: t, repo: repo, reverses: map[string]string{}, font: "1.0", night: "no", pushed: map[string]string{}, fail: map[string]error{}, pkg: AppID}
}

func (f *fakeEmulator) log() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return slices.Clone(f.cmds)
}

func (f *fakeEmulator) Run(ctx context.Context, c Cmd) ([]byte, []byte, error) {
	if c.Name == "git" {
		return ExecRunner{}.Run(ctx, c)
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	line := filepath.Base(c.Name) + " " + strings.Join(c.Args, " ")
	f.cmds = append(f.cmds, line)
	for sub, err := range f.fail {
		if strings.Contains(line, sub) {
			return nil, []byte("injected: " + sub), err
		}
	}
	out, err := f.answer(c, line)
	return []byte(out), nil, err
}

func (f *fakeEmulator) answer(c Cmd, line string) (string, error) {
	switch filepath.Base(c.Name) {
	case "gradlew":
		if slices.Contains(c.Args, "assembleRelease") {
			out := filepath.Join(f.repo, "android", "app", "build", "outputs")
			os.MkdirAll(filepath.Join(out, "apk", "release"), 0o755)
			os.MkdirAll(filepath.Join(out, "mapping", "release"), 0o755)
			os.WriteFile(filepath.Join(out, "apk", "release", "app-release-unsigned.apk"), []byte("apk"), 0o644)
			os.WriteFile(filepath.Join(out, "mapping", "release", "mapping.txt"), []byte("map"), 0o644)
			return "", nil
		}
		for _, a := range c.Args {
			if dir, ok := strings.CutPrefix(a, "-Panshin.qaSeedDir="); ok {
				for _, k := range ThemeKeys {
					os.WriteFile(filepath.Join(dir, k+".preferences_pb"), []byte("seed "+k), 0o600)
				}
			}
		}
		return "", nil
	case "keytool", "zipalign":
		return "", nil
	case "apksigner":
		if len(c.Args) > 0 && c.Args[0] == "sign" {
			os.WriteFile(c.Args[slices.Index(c.Args, "--out")+1], []byte("signed apk"), 0o600)
			return "", nil
		}
		if len(c.Args) == 3 && c.Args[0] == "verify" && c.Args[1] == "--print-certs" {
			return "Signer #1 certificate SHA-256 digest: " + strings.Repeat("ab", 32) + "\n", nil
		}
		return "", fmt.Errorf("unexpected apksigner arguments: %v", c.Args)
	case "aapt2":
		out := fmt.Sprintf("package: name='%s' versionCode='1' versionName='0.0.0' platformBuildVersionName='17'\n", f.pkg)
		if f.debuggable {
			out += "application-debuggable\n"
		}
		return out, nil
	case "go":
		out := c.Args[slices.Index(c.Args, "-o")+1]
		os.MkdirAll(filepath.Dir(out), 0o755)
		return "", os.WriteFile(out, []byte("bin"), 0o755)
	case "adb":
		return f.adb(c.Args)
	}
	f.t.Fatalf("unexpected command %s", line)
	return "", nil
}

func (f *fakeEmulator) adb(args []string) (string, error) {
	if len(args) >= 2 && args[0] == "-s" {
		args = args[2:]
	}
	switch args[0] {
	case "devices":
		return "List of devices attached\nemulator-5554\tdevice\n", nil
	case "emu":
		return "anshin-api37\nOK", nil
	case "get-state":
		return "device", nil
	case "root":
		f.root = true
		return "", nil
	case "unroot":
		f.root = false
		return "", nil
	case "wait-for-device":
		return "", nil
	case "install":
		f.installed = true
		if err := copyFile(args[1], filepath.Join(f.repo, "android", "app", "build", "installed.apk")); err != nil {
			return "", err
		}
		return "Success", nil
	case "uninstall":
		f.installed = false
		return "Success", nil
	case "push":
		f.pushed[args[2]] = args[1]
		return "", nil
	case "pull":
		return "", nil
	case "reverse":
		switch args[1] {
		case "--list":
			var lines []string
			for r, l := range f.reverses {
				lines = append(lines, "host-1 "+r+" "+l)
			}
			return strings.Join(lines, "\n"), nil
		case "--remove":
			delete(f.reverses, args[2])
			return "", nil
		}
		f.reverses[args[1]] = args[2]
		return "", nil
	case "shell":
		return f.shell(args[1])
	}
	return "", fmt.Errorf("unknown adb %v", args)
}

var seedScript = regexp.MustCompile(`cp (\S+) `)

func (f *fakeEmulator) shell(s string) (string, error) {
	switch {
	case s == "id -u":
		if f.root {
			return "0", nil
		}
		return "2000", nil
	case strings.HasPrefix(s, "if [ -e "+ownerMarker):
		if f.marker == "" {
			return "absent", nil
		}
		return "present\n" + f.marker, nil
	case strings.HasPrefix(s, "cat "+ownerMarker):
		return f.marker, nil
	case strings.HasPrefix(s, "set -C; echo ") && strings.HasSuffix(s, ownerMarker):
		if f.marker != "" {
			return "", fmt.Errorf("already claimed by %s", f.marker)
		}
		f.marker = strings.Fields(s)[3]
		return "", nil
	case s == "rm -f "+ownerMarker:
		f.marker = ""
		return "", nil
	case s == "settings get system font_scale":
		return f.font, nil
	case strings.HasPrefix(s, "settings put system font_scale "):
		f.font = strings.Fields(s)[4]
		return "", nil
	case s == "cmd uimode night":
		return "Night mode: " + f.night, nil
	case strings.HasPrefix(s, "cmd uimode night "):
		f.night = strings.Fields(s)[3]
		return "", nil
	case s == "pm list packages "+AppID:
		if f.installed {
			return "package:" + AppID, nil
		}
		return "", nil
	case s == "sha256sum '/data/app/base.apk'":
		sum, _, err := fileDigest(filepath.Join(f.repo, "android", "app", "build", "installed.apk"))
		return sum + "  /data/app/base.apk", err
	case s == "pm path "+AppID:
		if f.installed {
			return "package:/data/app/base.apk", nil
		}
		return "", errExit
	case s == "ps -A -o NAME":
		if f.running {
			return AppID, nil
		}
		return "", nil
	case s == "pidof "+AppID:
		if f.running {
			return "1234", nil
		}
		return "", errExit
	case strings.HasPrefix(s, "am force-stop"):
		f.running = false
		return "", nil
	case strings.HasPrefix(s, "iptables"):
		switch strings.Fields(s)[3] {
		case "-S":
			if f.dnat {
				return strings.TrimPrefix(dnatArgs("-A", DefaultAddress), "iptables -t nat "), nil
			}
			return "", nil
		case "-C":
			if f.dnat {
				return "", nil
			}
			return "", errExit
		case "-A":
			f.dnat = true
		case "-D":
			f.dnat = false
		}
		return "", nil
	case strings.Contains(s, "toybox nc"):
		return "HTTP/1.0 200 OK", nil
	case strings.HasPrefix(s, "stat -c"):
		return "10123:10123", nil
	case strings.HasPrefix(s, "set -e"):
		m := seedScript.FindStringSubmatch(s)
		local := f.pushed[m[1]]
		sum, _, err := fileDigest(local)
		if err != nil {
			return "", err
		}
		f.seeded = local
		return sum + "  " + datastoreDir + "/settings.preferences_pb", nil
	}
	return "", fmt.Errorf("unknown shell %q", s)
}

// fakeProcs starts a real loopback listener for the QA Agent's -listen, so up's readiness check passes, without
// running a binary.
type fakeProcs struct {
	mu  sync.Mutex
	lns map[int]net.Listener
	n   int
}

func (p *fakeProcs) Start(c Cmd, _, _ string) (int, string, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	ln, err := net.Listen("tcp", c.Args[slices.Index(c.Args, "-listen")+1])
	if err != nil {
		return 0, "", err
	}
	if p.lns == nil {
		p.lns = map[int]net.Listener{}
	}
	p.n++
	p.lns[p.n] = ln
	return p.n, "1", nil
}

func (p *fakeProcs) Alive(pid int, _ string) bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	_, ok := p.lns[pid]
	return ok
}

func (p *fakeProcs) Stop(pid int, _ string) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	if ln, ok := p.lns[pid]; ok {
		ln.Close()
		delete(p.lns, pid)
	}
	return nil
}

func (p *fakeProcs) live() int {
	p.mu.Lock()
	defer p.mu.Unlock()
	return len(p.lns)
}

type fixture struct {
	h     *Helper
	emu   *fakeEmulator
	procs *fakeProcs
	repo  string
}

func gitRun(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
	cmd.Env = append(os.Environ(), "GIT_AUTHOR_NAME=qa", "GIT_AUTHOR_EMAIL=qa@example.invalid", "GIT_COMMITTER_NAME=qa", "GIT_COMMITTER_EMAIL=qa@example.invalid")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %v: %v %s", args, err, out)
	}
}

func newFixture(t *testing.T) *fixture {
	t.Helper()
	repo := t.TempDir()
	gitRun(t, repo, "init", "-q", "-b", "main")
	os.MkdirAll(filepath.Join(repo, "android"), 0o755)
	os.WriteFile(filepath.Join(repo, ".gitignore"), []byte("/android/app/build/\n"), 0o644)
	os.WriteFile(filepath.Join(repo, "android", "gradlew"), []byte("#!/bin/sh\n"), 0o755)
	gitRun(t, repo, "add", ".")
	gitRun(t, repo, "commit", "-q", "-m", "base")
	gitRun(t, repo, "checkout", "-q", "-b", "feat/9-thing")
	os.WriteFile(filepath.Join(repo, "change.txt"), []byte("x\n"), 0o644)
	os.WriteFile(filepath.Join(repo, "shot.png"), []byte("png"), 0o644)
	gitRun(t, repo, "add", ".")
	gitRun(t, repo, "commit", "-q", "-m", "change")

	sdk := t.TempDir()
	for _, p := range []string{"build-tools/37.0.0/zipalign", "build-tools/37.0.0/apksigner", "build-tools/37.0.0/aapt2", "platform-tools/adb"} {
		os.MkdirAll(filepath.Join(sdk, filepath.Dir(p)), 0o755)
		os.WriteFile(filepath.Join(sdk, p), []byte("#!/bin/sh\n"), 0o755)
	}
	state := t.TempDir()
	env := map[string]string{"ANDROID_HOME": sdk, "XDG_STATE_HOME": state, "HOME": t.TempDir()}
	emu := newFakeEmulator(t, repo)
	procs := &fakeProcs{}
	h := &Helper{Repo: repo, Runner: emu, Procs: procs, Getenv: func(k string) string { return env[k] },
		Now: func() time.Time { return time.Date(2026, 11, 13, 18, 0, 0, 0, time.UTC) }}
	return &fixture{h: h, emu: emu, procs: procs, repo: repo}
}

func (f *fixture) init(t *testing.T) string {
	t.Helper()
	run, _, err := f.h.Init(context.Background(), "main", "test", nil)
	if err != nil {
		t.Fatal(err)
	}
	return run
}

func (f *fixture) up(t *testing.T, run string, args ...string) (UpRecord, error) {
	t.Helper()
	return f.h.Up(context.Background(), UpOptions{Run: run, Device: "anshin-api37", AgentArgs: args})
}

func TestInitSlugsTheBranchAndNeverReusesADirectory(t *testing.T) {
	f := newFixture(t)
	a, info := f.init(t), RunInfo{}
	b := f.init(t)
	if filepath.Base(a) != "feat-9-thing-20261113T180000Z" || filepath.Base(b) != "feat-9-thing-20261113T180000Z-2" {
		t.Fatalf("run directories %s, %s", filepath.Base(a), filepath.Base(b))
	}
	if err := readJSON(filepath.Join(a, "run.json"), &info); err != nil {
		t.Fatal(err)
	}
	if !info.CleanTree || info.MergeBase == "" || info.TargetSHA == info.MergeBase || !slices.Equal(info.FocusedExclusions, []string{"shot.png"}) {
		t.Fatalf("run info %+v", info)
	}
	focused, _ := os.ReadFile(filepath.Join(a, "focused.patch"))
	full, _ := os.ReadFile(filepath.Join(a, "changes.patch"))
	if strings.Contains(string(focused), "shot.png") || !strings.Contains(string(full), "shot.png") || !strings.Contains(string(focused), "change.txt") {
		t.Fatalf("focused.patch must drop only excluded files:\n%s", focused)
	}
}

func TestUpOwnsEveryResourceAndDownRemovesExactlyThem(t *testing.T) {
	f := newFixture(t)
	run := f.init(t)
	rec, err := f.up(t, run, "-cpu", "null")
	if err != nil {
		t.Fatal(err)
	}
	if rec.Serial != "emulator-5554" || rec.APK.SHA256 == "" || rec.APK.SignerSHA256 != strings.Repeat("ab", 32) || rec.APK.Debuggable || len(rec.Seeds) != 4 || !strings.HasPrefix(rec.Probe, "answered") {
		t.Fatalf("up record %+v", rec)
	}
	if !f.emu.root || !f.emu.installed || !f.emu.dnat || f.emu.marker == "" || len(f.emu.reverses) != 1 || f.procs.live() != 1 {
		t.Fatalf("device after up: %+v", f.emu)
	}
	f.emu.font, f.emu.night = "1.35", "yes" // what a run's settings calls change

	c, err := f.h.Down(context.Background(), run)
	if err != nil || c.Status != "ok" {
		t.Fatalf("down: %v %+v", err, c)
	}
	if f.emu.root || f.emu.installed || f.emu.dnat || f.emu.marker != "" || len(f.emu.reverses) != 0 || f.procs.live() != 0 || f.emu.font != "1.0" || f.emu.night != "no" {
		t.Fatalf("device after down: %+v, live agents %d", f.emu, f.procs.live())
	}
	before := len(f.emu.log())
	if c, err := f.h.Down(context.Background(), run); err != nil || c.Status != "ok" || len(f.emu.log()) != before {
		t.Fatalf("a second down must be a no-op: %v %+v", err, c)
	}
}

func TestAFailedUpLeavesALedgerThatDownCanFinish(t *testing.T) {
	f := newFixture(t)
	run := f.init(t)
	f.emu.fail["install "] = errExit
	if _, err := f.up(t, run); err == nil {
		t.Fatal("up succeeded although install failed")
	}
	var l Ledger
	readJSON(filepath.Join(run, ledgerFile), &l)
	kinds := []string{}
	for _, r := range l.Resources {
		kinds = append(kinds, r.Kind+":"+r.State)
	}
	want := []string{"device-owner-marker:active", "adb-root:active", "device-settings:active", "app-install:pending"}
	if !slices.Equal(kinds, want) || len(l.Failures) != 1 {
		t.Fatalf("ledger %v failures %v, want %v", kinds, l.Failures, want)
	}
	c, err := f.h.Down(context.Background(), run)
	if err != nil || c.Status != "ok" || f.emu.root || f.emu.marker != "" {
		t.Fatalf("down after a partial up: %v %+v %+v", err, c, f.emu)
	}
	if slices.ContainsFunc(f.emu.log(), func(s string) bool { return strings.Contains(s, "uninstall") }) {
		t.Fatal("down uninstalled an app the failed install never put there")
	}
}

func TestUpRefusesAnAppItDidNotInstall(t *testing.T) {
	f := newFixture(t)
	run := f.init(t)
	f.emu.installed = true
	if _, err := f.up(t, run); err == nil || !strings.Contains(err.Error(), "already installed") {
		t.Fatalf("up over an existing install: %v", err)
	}
	if _, err := f.h.Down(context.Background(), run); err != nil {
		t.Fatal(err)
	}
	if !f.emu.installed {
		t.Fatal("down removed the Owner's existing install")
	}
}

func TestDownLeavesAReverseMappingSomeoneElseReplaced(t *testing.T) {
	f := newFixture(t)
	run := f.init(t)
	if _, err := f.up(t, run); err != nil {
		t.Fatal(err)
	}
	f.emu.reverses["tcp:7247"] = "tcp:1" // another tool re-mapped the port after up
	if _, err := f.h.Down(context.Background(), run); err != nil {
		t.Fatal(err)
	}
	if f.emu.reverses["tcp:7247"] != "tcp:1" {
		t.Fatal("down removed a reverse mapping that was no longer the run's")
	}
}

func TestUpRefusesAnEmulatorAnotherRunClaimed(t *testing.T) {
	f := newFixture(t)
	run := f.init(t)
	f.emu.marker = "other-run"
	if _, err := f.up(t, run); err == nil || !strings.Contains(err.Error(), "other-run") {
		t.Fatalf("up on a claimed emulator: %v", err)
	}
	f.h.Down(context.Background(), run)
	if f.emu.marker != "other-run" {
		t.Fatal("down removed another run's claim")
	}
}

func TestABuildFailureTouchesNoDevice(t *testing.T) {
	f := newFixture(t)
	run := f.init(t)
	f.emu.fail["assembleRelease"] = errExit
	if _, err := f.up(t, run); err == nil || !strings.Contains(err.Error(), "release build") {
		t.Fatalf("up with a failing build: %v", err)
	}
	if slices.ContainsFunc(f.emu.log(), func(s string) bool { return strings.HasPrefix(s, "adb") }) {
		t.Fatalf("a failed build still ran adb: %v", f.emu.log())
	}
	res, err := f.h.Finish(context.Background(), FinishOptions{Run: run})
	if err != nil {
		t.Fatal(err)
	}
	if res.Result != "INCONCLUSIVE" || res.ReasonCode != "environment" || res.Cleanup.Status != "nothing-owned" {
		t.Fatalf("finish after a failed build: %+v", res)
	}
}

func TestADebuggableOrForeignAPKIsRefused(t *testing.T) {
	for name, set := range map[string]func(*fakeEmulator){
		"debuggable": func(e *fakeEmulator) { e.debuggable = true },
		"package":    func(e *fakeEmulator) { e.pkg = "com.example.other" },
	} {
		f := newFixture(t)
		run := f.init(t)
		set(f.emu)
		if _, err := f.up(t, run); err == nil {
			t.Errorf("%s: up accepted the APK", name)
		}
	}
}

func TestUpBuildsOnlyTheCleanTargetSHA(t *testing.T) {
	f := newFixture(t)
	run := f.init(t)
	os.WriteFile(filepath.Join(f.repo, "dirty.txt"), []byte("x"), 0o644)
	if _, err := f.up(t, run); err == nil || !strings.Contains(err.Error(), "clean") {
		t.Fatalf("up on a dirty tree: %v", err)
	}
	if len(f.emu.log()) != 0 {
		t.Fatalf("up ran commands before refusing: %v", f.emu.log())
	}
}

func TestSeedWritesOnlyAStoppedAppTheRunInstalled(t *testing.T) {
	f := newFixture(t)
	run := f.init(t)
	if _, err := f.h.Seed(context.Background(), run, "tokyo-night"); err == nil {
		t.Fatal("seeded without an owned install")
	}
	if _, err := f.up(t, run); err != nil {
		t.Fatal(err)
	}
	f.emu.running = true
	if _, err := f.h.Seed(context.Background(), run, "tokyo-night"); err == nil || !strings.Contains(err.Error(), "running") {
		t.Fatalf("seeded a running app: %v", err)
	}
	f.emu.running = false
	if _, err := f.h.Seed(context.Background(), run, "solarized"); err == nil {
		t.Fatal("seeded an unknown theme")
	}
	rec, err := f.h.Seed(context.Background(), run, "tokyo-night")
	if err != nil {
		t.Fatal(err)
	}
	if f.emu.seeded != rec.Path || !strings.HasSuffix(rec.Path, "tokyo-night.preferences_pb") {
		t.Fatalf("seeded %s, want %s", f.emu.seeded, rec.Path)
	}
}

func TestAgentStopKeepsTheRouteAndStartRestartsOnIt(t *testing.T) {
	f := newFixture(t)
	run := f.init(t)
	if _, err := f.up(t, run); err != nil {
		t.Fatal(err)
	}
	if err := f.h.AgentStop(context.Background(), run); err != nil || f.procs.live() != 0 || len(f.emu.reverses) != 1 || !f.emu.dnat {
		t.Fatalf("stop: %v live %d", err, f.procs.live())
	}
	if err := f.h.AgentStart(context.Background(), run, []string{"-whois", "nonsense"}); err == nil {
		t.Fatal("started a QA Agent with an invalid setup")
	}
	if err := f.h.AgentStart(context.Background(), run, []string{"-whois", "deny"}); err != nil || f.procs.live() != 1 {
		t.Fatalf("start: %v live %d", err, f.procs.live())
	}
	if err := f.h.AgentStart(context.Background(), run, []string{"-cpu", "zero"}); err != nil || f.procs.live() != 1 {
		t.Fatalf("restart must replace, not add: %v live %d", err, f.procs.live())
	}
}

func TestRawBodyIsCopiedIntoTheRunWithItsDigest(t *testing.T) {
	f := newFixture(t)
	run := f.init(t)
	body := filepath.Join(t.TempDir(), "schema-2.json")
	os.WriteFile(body, []byte(`{"schema":2}`), 0o600)
	if _, err := f.up(t, run, "-mode", "raw", "-body", body); err != nil {
		t.Fatal(err)
	}
	var l Ledger
	readJSON(filepath.Join(run, ledgerFile), &l)
	var agent *Resource
	for _, r := range l.Resources {
		if r.Kind == kindHostAgent {
			agent = r
		}
	}
	sum, _, _ := fileDigest(body)
	if agent == nil || agent.Data["bodySha256"] != sum || !strings.HasPrefix(agent.Data["body"], run) || !strings.Contains(agent.Data["command"], agent.Data["body"]) {
		t.Fatalf("agent resource %+v", agent)
	}
}

func TestASecondUpNeedsDownFirstAndThenWorks(t *testing.T) {
	f := newFixture(t)
	run := f.init(t)
	if _, err := f.up(t, run); err != nil {
		t.Fatal(err)
	}
	if _, err := f.up(t, run); err == nil {
		t.Fatal("a second up stacked on live resources")
	}
	if _, err := f.h.Down(context.Background(), run); err != nil {
		t.Fatal(err)
	}
	if _, err := f.h.Up(context.Background(), UpOptions{Run: run, Device: "anshin-api37", Route: "reverse"}); err != nil {
		t.Fatalf("up after down: %v", err)
	}
	if _, err := f.h.Down(context.Background(), run); err != nil || f.emu.installed || f.emu.root {
		t.Fatalf("down: %v %+v", err, f.emu)
	}
}
