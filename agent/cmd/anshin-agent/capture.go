package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"os/user"
	"syscall"
	"time"

	"github.com/fsrodrig/anshin/agent/internal/capture"
)

// minInterval is the CPU time the recorder needs between two readings before it reports a CPU %; a capture with
// snapshots closer than that would replay as null.
const minInterval = time.Second

// runCapture is `anshin-agent capture`. It never touches the tailnet or the server, so it runs without tailscaled.
func runCapture(args []string, stdout, stderr io.Writer) int {
	fl := flag.NewFlagSet("capture", flag.ContinueOnError)
	fl.SetOutput(stderr)
	fl.Usage = func() {
		fmt.Fprintln(stderr, "usage: anshin-agent capture -name <placeholder> [flags] <dir>")
		fl.PrintDefaults()
	}
	name := fl.String("name", "", "placeholder for this Machine's hostname, also the fixture name (required)")
	snapshots := fl.Int("snapshots", 3, "snapshots to take (at least 2)")
	interval := fl.Duration("interval", 2*time.Second, "time between snapshots (at least 1s)")
	statfsTimeout := fl.Duration("statfs-timeout", 2*time.Second, "give up on a statfs call after this long")
	identifiers := fl.String("identifiers", "", "private denylist to scrub as well (.git/info/identifiers format)")
	if err := fl.Parse(args); err != nil {
		return 2
	}
	if fl.NArg() != 1 {
		fl.Usage()
		return 2
	}
	if *interval < minInterval {
		fmt.Fprintf(stderr, "capture: -interval must be at least %v\n", minInterval)
		return 2
	}
	opt := capture.Options{Name: *name, Snapshots: *snapshots, Interval: *interval, StatfsTimeout: *statfsTimeout}
	if *identifiers != "" {
		b, err := os.ReadFile(*identifiers)
		if err != nil {
			fmt.Fprintf(stderr, "capture: %v\n", err)
			return 2
		}
		opt.Identifiers = b
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	src := capture.Source{Host: liveHost(), MountID: mountID, Clock: clock, User: currentUser()}
	if err := capture.Run(ctx, src, fl.Arg(0), opt, stdout); err != nil {
		fmt.Fprintf(stderr, "capture: %v\n", err)
		if errors.Is(err, context.Canceled) {
			return 130
		}
		return 1
	}
	return 0
}

func currentUser() string {
	if u, err := user.Current(); err == nil {
		return u.Username
	}
	return os.Getenv("USER")
}
