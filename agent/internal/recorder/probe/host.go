// Package probe parses what the Machine exposes in /proc and /sys. It reads only through a Host, so the same
// parsers run against the live root in production and against a replayed capture in tests.
//
// A probe may use only path.Base of what fs.ReadLink returns: a capture records a link's target by its basename,
// because the full target lies outside the files it copies.
package probe

import "io/fs"

type Host struct {
	FS     fs.FS
	Statfs func(path string) (FsStat, error)
}

// FsStat is statfs(2) for one mount. Blocks, Bfree and Bavail are in Frsize units. f_fsid is left out on purpose:
// btrfs mixes the subvolume into it, so docs/design.md § Filesystems dedupes by MAJ:MIN instead.
type FsStat struct {
	Type, Bsize, Frsize, Flags int64
	Blocks, Bfree, Bavail      uint64
	Files, Ffree               uint64
}
