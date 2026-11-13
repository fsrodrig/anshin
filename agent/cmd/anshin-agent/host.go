package main

import (
	"os"

	"golang.org/x/sys/unix"

	"github.com/fsrodrig/anshin/agent/internal/recorder/probe"
)

// This file holds the only adapters that touch the live Machine; `make check-agent` fails if they appear anywhere
// else, so tests can never sample the host they run on.

func liveHost() probe.Host {
	return probe.Host{FS: os.DirFS("/"), Statfs: statfs}
}

func statfs(path string) (probe.FsStat, error) {
	var s unix.Statfs_t
	if err := unix.Statfs(path, &s); err != nil {
		return probe.FsStat{}, err
	}
	// The explicit conversions are what let the same code build on amd64 and arm64, whose Statfs_t field types differ.
	return probe.FsStat{
		Type: int64(s.Type), Bsize: int64(s.Bsize), Frsize: int64(s.Frsize), Flags: int64(s.Flags),
		Blocks: uint64(s.Blocks), Bfree: uint64(s.Bfree), Bavail: uint64(s.Bavail),
		Files: uint64(s.Files), Ffree: uint64(s.Ffree),
	}, nil
}
