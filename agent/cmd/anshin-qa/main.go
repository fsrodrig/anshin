// Command anshin-qa is the synthetic QA Agent for the agent-qa skill. It is never shipped: it serves a fixed,
// synthetic Pulse to an emulator, and its raw mode serves arbitrary bytes without authorization.
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/netip"
	"os"
	"os/signal"
	"syscall"

	"github.com/fsrodrig/anshin/agent/internal/qa"
)

func main() {
	os.Exit(run())
}

func run() int {
	cfg, caps, err := qa.ParseAgentFlags(os.Args[1:], os.Stderr)
	if err != nil {
		fmt.Fprintln(os.Stderr, "anshin-qa:", err)
		return 2
	}
	if caps {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		enc.Encode(qa.Capabilities())
		return 0
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	// The access log goes to stdout, so whoever starts the process decides where it lands; diagnostics stay on stderr.
	err = qa.ServeAgent(ctx, cfg, os.Stdout, func(ap netip.AddrPort) {
		slog.Info("anshin-qa listening", "addr", ap.String(), "mode", cfg.Mode)
	})
	if err != nil {
		slog.Error("anshin-qa stopping", "err", err)
		return 1
	}
	return 0
}
