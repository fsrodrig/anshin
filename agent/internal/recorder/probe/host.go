// Package probe parses what the Machine exposes in /proc and /sys. It reads only through a Host, so the same
// parsers run against the live root in production and against a replayed capture in tests.
//
// A probe may use only path.Base of what fs.ReadLink returns: a capture records a link's target by its basename,
// because the full target lies outside the files it copies.
package probe

import "io/fs"

type Host struct {
	Architecture   *string
	AgentVersion   *string
	FS             fs.FS
	OpenFilesystem func(path string) (FilesystemHandle, error)
	MountID        func(path string) (uint64, error)
	Statfs         func(path string) (FsStat, error)
}

// FsStat is statfs(2) for one mount. Blocks, Bfree and Bavail are in Frsize units. f_fsid is left out on purpose:
// btrfs mixes the subvolume into it, so docs/design.md § Filesystems dedupes by MAJ:MIN instead.
type FsStat struct {
	Type, Bsize, Frsize, Flags int64
	Blocks, Bfree, Bavail      uint64
	Files, Ffree               uint64
}

// FilesystemHandle pins one mount until Close. Statfs must read that mount even if its path is replaced.
// MountID is verified before Statfs, so visibility can be published even when capacity blocks.
type FilesystemHandle struct {
	MountID uint64
	Statfs  func() (FsStat, error)
	Close   func() error
}
