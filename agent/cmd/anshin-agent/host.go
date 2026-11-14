package main

import (
	"errors"
	"os"
	"runtime/debug"
	"time"

	"golang.org/x/sys/unix"

	"github.com/fsrodrig/anshin/agent/internal/capture"
	"github.com/fsrodrig/anshin/agent/internal/recorder/probe"
)

// This file holds the only adapters that touch the live Machine; `make check-agent` fails if they appear anywhere
// else, so tests can never sample the host they run on.

var version string

func liveHost() probe.Host {
	h := probe.Host{FS: os.DirFS("/"), Statfs: statfs, MountID: mountID, OpenFilesystem: openFilesystem}
	var u unix.Utsname
	if unix.Uname(&u) == nil {
		arch := unix.ByteSliceToString(u.Machine[:])
		if arch != "" {
			h.Architecture = &arch
		}
	}
	v := version
	if v == "" {
		if b, ok := debug.ReadBuildInfo(); ok && b.Main.Version != "" {
			v = b.Main.Version
		}
	}
	if v != "" {
		h.AgentVersion = &v
	}
	return h
}

func statfs(path string) (probe.FsStat, error) {
	var s unix.Statfs_t
	if err := unix.Statfs(path, &s); err != nil {
		return probe.FsStat{}, err
	}
	return fsStat(s), nil
}

func fsStat(s unix.Statfs_t) probe.FsStat {
	// The explicit conversions are what let the same code build on amd64 and arm64, whose Statfs_t field types differ.
	return probe.FsStat{
		Type: int64(s.Type), Bsize: int64(s.Bsize), Frsize: int64(s.Frsize), Flags: int64(s.Flags),
		Blocks: uint64(s.Blocks), Bfree: uint64(s.Bfree), Bavail: uint64(s.Bavail),
		Files: uint64(s.Files), Ffree: uint64(s.Ffree),
	}
}

func mountID(path string) (uint64, error) {
	var s unix.Statx_t
	// AT_NO_AUTOMOUNT: asking which mount is at a path must not mount anything there.
	if err := unix.Statx(unix.AT_FDCWD, path, unix.AT_NO_AUTOMOUNT|unix.AT_SYMLINK_NOFOLLOW, unix.STATX_MNT_ID, &s); err != nil {
		return 0, err
	}
	if s.Mask&unix.STATX_MNT_ID == 0 {
		return 0, errors.New("mount ID unavailable")
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

func openFilesystem(path string) (probe.FilesystemHandle, error) {
	f, err := os.OpenFile(path, unix.O_PATH|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0)
	if err != nil {
		return probe.FilesystemHandle{}, err
	}
	var s unix.Statx_t
	if err := unix.Statx(int(f.Fd()), "", unix.AT_EMPTY_PATH|unix.AT_NO_AUTOMOUNT, unix.STATX_MNT_ID, &s); err != nil {
		f.Close()
		return probe.FilesystemHandle{}, err
	}
	if s.Mask&unix.STATX_MNT_ID == 0 {
		f.Close()
		return probe.FilesystemHandle{}, errors.New("mount ID unavailable")
	}
	return probe.FilesystemHandle{MountID: s.Mnt_id, Close: f.Close, Statfs: func() (probe.FsStat, error) {
		var stat unix.Statfs_t
		if err := unix.Fstatfs(int(f.Fd()), &stat); err != nil {
			return probe.FsStat{}, err
		}
		return fsStat(stat), nil
	}}, nil
}
