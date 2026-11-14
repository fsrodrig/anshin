package probe

import (
	"errors"
	"io/fs"
	"strconv"
	"strings"
)

// NetCounters is one /proc/net/dev row. A nil counter could not be parsed. DeviceBacked is nil when the existence
// of /sys/class/net/<if>/device could not be determined.
type NetCounters struct {
	Name         string
	DeviceBacked *bool
	Rx, Tx       *uint64
}

// DiskCounters is one /proc/diskstats row of a disk backed by a device, in 512-byte sectors whatever the disk's
// sector size, as Documentation/admin-guide/iostats.rst defines them.
type DiskCounters struct {
	Name                      string
	ReadSectors, WriteSectors *uint64
}

func counter(s string) *uint64 {
	n, err := strconv.ParseUint(s, 10, 64)
	if err != nil {
		return nil
	}
	return &n
}

// deviceLink reports whether dir/device exists. Only the link itself is examined, never anything behind it.
func deviceLink(h Host, dir string) *bool {
	_, err := fs.Lstat(h.FS, dir+"/device")
	switch {
	case err == nil:
		return new(true)
	case errors.Is(err, fs.ErrNotExist):
		return new(false)
	}
	return nil
}

func Network(h Host) ([]NetCounters, error) {
	b, err := fs.ReadFile(h.FS, "proc/net/dev")
	if err != nil {
		return nil, err
	}
	var out []NetCounters
	for line := range strings.SplitSeq(string(b), "\n") {
		// The kernel forbids ':' in interface names, and the two header lines have none.
		name, rest, ok := strings.Cut(line, ":")
		name = strings.TrimSpace(name)
		if !ok || name == "" {
			continue
		}
		c := NetCounters{Name: name, DeviceBacked: deviceLink(h, "sys/class/net/"+name)}
		if f := strings.Fields(rest); len(f) == 16 {
			c.Rx, c.Tx = counter(f[0]), counter(f[8])
		}
		out = append(out, c)
	}
	return out, nil
}

// Disks lists the disks in /proc/diskstats that are backed by a device, in kernel order. Partitions, dm, zram and
// loop devices have no device link: the kernel already counts a partition's or dm device's I/O on its parent disk.
// A disk whose link can't be determined is left out rather than guessed at.
func Disks(h Host) ([]DiskCounters, error) {
	b, err := fs.ReadFile(h.FS, "proc/diskstats")
	if err != nil {
		return nil, err
	}
	var out []DiskCounters
	for line := range strings.SplitSeq(string(b), "\n") {
		f := strings.Fields(line)
		if len(f) < 14 {
			continue
		}
		// sysfs spells a '/' in a block device's name as '!' (cciss/c0d0 is /sys/class/block/cciss!c0d0).
		if backed := deviceLink(h, "sys/class/block/"+strings.ReplaceAll(f[2], "/", "!")); backed == nil || !*backed {
			continue
		}
		out = append(out, DiskCounters{Name: f[2], ReadSectors: counter(f[5]), WriteSectors: counter(f[9])})
	}
	return out, nil
}
