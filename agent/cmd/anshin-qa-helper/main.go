// Command anshin-qa-helper is the agent-qa skill's environment helper, run through scripts/qa. It builds, signs
// and installs the release app on an emulator the coordinator opened, routes it to a synthetic QA Agent, records
// what the run owns, removes exactly that, and writes the run's result files. It never drives the app's UI.
package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/fsrodrig/anshin/agent/internal/qa"
)

const usage = `scripts/qa: the agent-qa environment helper. Every command prints JSON on stdout.

  scripts/qa help                       this text
  scripts/qa capabilities               the QA Agent setups, routes, seed themes and operations
  scripts/qa doctor                     host prerequisites; never touches a device (exit 3: no device run possible)
  scripts/qa init [--base main] [--pr N] [--caller TEXT]
                                        new run directory for the current checkout, with its diffs; needs only git
  scripts/qa up --run DIR (--device AVD | --serial emulator-N) [--route reverse|device]
                [--address 100.64.x.y] [-- QA-AGENT-FLAGS]
                                        build, sign and verify the release APK from the clean target SHA, generate the
                                        theme seeds, then on the opened emulator: root, install, start the QA Agent
                                        and route the synthetic address to it; every resource enters the run's ledger
                                        first. Never boots, opens or drives the device.
  scripts/qa agent start --run DIR [-- QA-AGENT-FLAGS]
                                        restart the run's QA Agent with a new setup on the same route
  scripts/qa agent stop --run DIR       stop the QA Agent and keep the route: the phone sees an Unreachable Agent
  scripts/qa seed --run DIR --theme KEY put a generated theme preference in the stopped app's DataStore
  scripts/qa down --run DIR             remove what the ledger says this run owns, newest first; safe to repeat
  scripts/qa finish --run DIR [--mode device|planning|na] [--reason TEXT] [--posting none|caller|authorized]
                                        write result.json, report.md, pr-summary.md and evidence-manifest.json

QA-AGENT-FLAGS are anshin-qa's (scripts/qa capabilities lists them): -whois allow|deny|localapi-error,
-cpu ordinary|null|zero, -hostname synthetic|null, -delay DURATION, and the test-only -mode raw -body FILE
-status CODE. The helper adds -listen.

Exit status: 0 done, 1 the operation failed (details on stderr and in the run), 2 bad usage, 3 doctor found no
possible device run.
`

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}

func run(args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 || args[0] == "help" || args[0] == "-h" || args[0] == "--help" {
		fmt.Fprint(stdout, usage)
		return 0
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	h := &qa.Helper{Repo: repoRoot(), Runner: qa.ExecRunner{}, Procs: qa.OSProcesses{}, Getenv: os.Getenv, Now: time.Now, Stdout: stdout, Stderr: stderr}

	cmd, rest := args[0], args[1:]
	if cmd == "agent" {
		if len(rest) == 0 {
			fmt.Fprint(stderr, usage)
			return 2
		}
		cmd, rest = "agent "+rest[0], rest[1:]
	}
	own, passthrough := splitDashes(rest)
	fs := flag.NewFlagSet(cmd, flag.ContinueOnError)
	fs.SetOutput(stderr)
	runDir := fs.String("run", "", "run directory from init")

	var out any
	var err error
	switch cmd {
	case "capabilities":
		if err = parse(fs, own); err == nil {
			out = map[string]any{"operations": qa.Operations, "routes": []string{"reverse", "device"}, "seedThemes": qa.ThemeKeys, "agentCapabilities": qa.Capabilities()}
		}
	case "doctor":
		if err = parse(fs, own); err == nil {
			r := h.Doctor(ctx)
			emit(stdout, r)
			if !r.DeviceRunOK {
				return 3
			}
			return 0
		}
	case "init":
		base := fs.String("base", "main", "branch the change will merge into")
		pr := fs.Int("pr", 0, "pull request number, if one exists")
		caller := fs.String("caller", "user", "who asked: the user's words, or the pipeline stage and what it needs")
		if err = parse(fs, own); err == nil {
			var prp *int
			if *pr > 0 {
				prp = pr
			}
			var dir string
			var info qa.RunInfo
			dir, info, err = h.Init(ctx, *base, *caller, prp)
			out = map[string]any{"run": dir, "info": info}
		}
	case "up":
		o := qa.UpOptions{AgentArgs: passthrough}
		fs.StringVar(&o.Device, "device", "", "AVD name, as device_list reports the device id")
		fs.StringVar(&o.Serial, "serial", "", "emulator serial, instead of --device")
		fs.StringVar(&o.Route, "route", "reverse", "reverse | device")
		fs.StringVar(&o.Address, "address", qa.DefaultAddress, "synthetic tailnet address the app is pointed at")
		if err = parse(fs, own); err == nil {
			o.Run = *runDir
			out, err = h.Up(ctx, o)
		}
	case "agent start":
		if err = parse(fs, own); err == nil {
			err = h.AgentStart(ctx, *runDir, passthrough)
			out = map[string]any{"started": err == nil, "args": passthrough}
		}
	case "agent stop":
		if err = parse(fs, own); err == nil {
			err = h.AgentStop(ctx, *runDir)
			out = map[string]any{"stopped": err == nil}
		}
	case "seed":
		theme := fs.String("theme", "", "one of "+strings.Join(qa.ThemeKeys, ", "))
		if err = parse(fs, own); err == nil {
			out, err = h.Seed(ctx, *runDir, *theme)
		}
	case "down":
		if err = parse(fs, own); err == nil {
			out, err = h.Down(ctx, *runDir)
		}
	case "finish":
		o := qa.FinishOptions{}
		fs.StringVar(&o.Mode, "mode", "device", "device | planning | na")
		fs.StringVar(&o.Reason, "reason", "", "why: required for na, recommended for planning")
		fs.StringVar(&o.Posting, "posting", "none", "none (--no-post) | caller | authorized")
		if err = parse(fs, own); err == nil {
			o.Run = *runDir
			out, err = h.Finish(ctx, o)
		}
	default:
		fmt.Fprintf(stderr, "unknown command %q\n\n%s", cmd, usage)
		return 2
	}
	var usageErr usageError
	if errors.As(err, &usageErr) {
		fmt.Fprintln(stderr, err)
		return 2
	}
	if out != nil {
		emit(stdout, out)
	}
	if err != nil {
		fmt.Fprintln(stderr, "scripts/qa "+cmd+":", err)
		return 1
	}
	return 0
}

type usageError struct{ error }

func parse(fs *flag.FlagSet, args []string) error {
	if err := fs.Parse(args); err != nil {
		return usageError{err}
	}
	if fs.NArg() > 0 {
		return usageError{fmt.Errorf("unexpected arguments %v; QA Agent flags go after --", fs.Args())}
	}
	return nil
}

func splitDashes(args []string) ([]string, []string) {
	for i, a := range args {
		if a == "--" {
			return args[:i], args[i+1:]
		}
	}
	return args, nil
}

func emit(w io.Writer, v any) {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	enc.SetEscapeHTML(false)
	enc.Encode(v)
}

// repoRoot is the checkout scripts/qa was run from; the launcher passes it, so the helper works from any directory.
func repoRoot() string {
	if r := os.Getenv("ANSHIN_REPO"); r != "" {
		return r
	}
	out, err := exec.Command("git", "rev-parse", "--show-toplevel").Output()
	if err != nil {
		wd, _ := os.Getwd()
		return wd
	}
	return strings.TrimSpace(string(out))
}
