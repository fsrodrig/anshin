package main

import (
	"os"
	"time"

	"golang.org/x/sys/unix"

	"github.com/fsrodrig/anshin/agent/internal/capture"
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

func mountID(path string) (uint64, error) {
	var s unix.Statx_t
	// AT_NO_AUTOMOUNT: asking which mount is at a path must not mount anything there.
	if err := unix.Statx(unix.AT_FDCWD, path, unix.AT_NO_AUTOMOUNT|unix.AT_SYMLINK_NOFOLLOW, unix.STATX_MNT_ID, &s); err != nil {
		return 0, err
	}
	return s.Mnt_id, nil
}

func clock() capture.Clock {
	c := capture.Clock{Wall: time.Now()}
	var ts unix.Timespec
	if unix.ClockGettime(unix.CLOCK_BOOTTIME, &ts) == nil {
		c.BoottimeNs = ts.Nano()
	}
	if unix.ClockGettime(unix.CLOCK_MONOTONIC, &ts) == nil {
		c.MonotonicNs = ts.Nano()
	}
	return c
}
