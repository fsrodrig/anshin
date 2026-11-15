// Command anshin-agent measures this Machine and answers the Owner's phone over the tailnet. It only reads.
package main

import (
	"context"
	"fmt"
	"log/slog"
	"math/rand/v2"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	"github.com/fsrodrig/anshin/agent/internal/recorder"
	"github.com/fsrodrig/anshin/agent/internal/recorder/probe"
	"github.com/fsrodrig/anshin/agent/internal/server"
	"github.com/fsrodrig/anshin/agent/internal/tailnet"
)

func main() {
	if len(os.Args) > 1 && os.Args[1] == "capture" {
		os.Exit(runCapture(os.Args[2:], os.Stdout, os.Stderr))
	}
	os.Exit(run())
}

func run() int {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	port, err := port()
	if err != nil {
		slog.Error("bad configuration", "err", err)
		return 2
	}
	sensor, err := cpuSensor()
	if err != nil {
		slog.Error("bad configuration", "err", err)
		return 2
	}
	tn := tailnet.NewLocalAPI(tailnet.SocketPath())
	first, err := instant()
	if err != nil {
		slog.Error("reading this Machine's clocks", "err", err)
		return 1
	}
	rec := recorder.Open(liveHost(), first, recorder.Config{CPUSensor: sensor, Epoch: rand.Uint64()})
	go sampleEvery(ctx, rec, recorder.SampleInterval, instant)
	if err := server.Run(ctx, tn, rec, port); err != nil {
		slog.Error("stopping", "err", err)
		return 1
	}
	return 0
}

type sampler interface{ Sample(recorder.Instant) }

// sampleEvery is the Window's sampling loop: it runs whether or not anyone is looking, so a phone that opens sees
// the last 30 minutes at once. A tick whose clocks can't be read is skipped rather than given a guessed time.
func sampleEvery(ctx context.Context, rec sampler, every time.Duration, clocks func() (recorder.Instant, error)) {
	t := time.NewTicker(every)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
		}
		at, err := clocks()
		if err != nil {
			slog.Warn("skipping a Sample: reading this Machine's clocks", "err", err)
			continue
		}
		rec.Sample(at)
	}
}

func port() (int, error) {
	s := os.Getenv("ANSHIN_PORT")
	if s == "" {
		return server.DefaultPort, nil
	}
	p, err := strconv.Atoi(s)
	if err != nil || p < 1 || p > 65535 {
		return 0, fmt.Errorf("ANSHIN_PORT=%q is not a TCP port", s)
	}
	return p, nil
}

func cpuSensor() (string, error) {
	s := os.Getenv("ANSHIN_CPU_SENSOR")
	if s != "" && !probe.ValidSensorID(s) {
		return "", fmt.Errorf("ANSHIN_CPU_SENSOR=%q is not <hwmon name>/<label>, such as coretemp/Package id 0; Machine details on the phone lists every sensor", s)
	}
	return s, nil
}
