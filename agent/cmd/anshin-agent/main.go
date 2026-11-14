// Command anshin-agent measures this Machine and answers the Owner's phone over the tailnet. It only reads.
package main

import (
	"context"
	"fmt"
	"log/slog"
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
	rec := recorder.New(liveHost(), time.Now(), recorder.Config{CPUSensor: sensor})
	if err := server.Run(ctx, tn, rec, port); err != nil {
		slog.Error("stopping", "err", err)
		return 1
	}
	return 0
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
