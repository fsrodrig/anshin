package probe

import (
	"bufio"
	"bytes"
	"fmt"
	"io/fs"
	"math"
	"path"
	"slices"
	"strconv"
	"strings"
)

type Mount struct {
	ID, Parent                        uint64
	MajMin, Root, Point, Type, Source string
}

type Filesystem struct {
	MajMin     string   `json:"maj_min"`
	MountPoint string   `json:"mount_point"`
	Type       string   `json:"fs_type"`
	Total      *uint64  `json:"total_bytes"`
	Used       *uint64  `json:"used_bytes"`
	Available  *uint64  `json:"available_bytes"`
	UsedPct    *float64 `json:"used_pct"`
	Age        float64  `json:"age_s"`
}

// Mounts rejects the whole table on malformed input: losing an unsafe ancestor could make a child unsafe to probe.
func Mounts(h Host) ([][]Mount, error) {
	b, err := fs.ReadFile(h.FS, "proc/self/mountinfo")
	if err != nil {
		return nil, err
	}
	all, err := ParseMounts(b)
	if err != nil {
		return nil, err
	}
	groups := map[string][]Mount{}
	for _, m := range all {
		unsafe := false
		for _, a := range all {
			if (a.Point == m.Point || strings.HasPrefix(m.Point, strings.TrimSuffix(a.Point, "/")+"/")) && UnsafeMount(a) {
				unsafe = true
				break
			}
		}
		if !unsafe {
			groups[m.MajMin] = append(groups[m.MajMin], m)
		}
	}
	keys := make([]string, 0, len(groups))
	for k := range groups {
		keys = append(keys, k)
	}
	slices.Sort(keys)
	var out [][]Mount
	for _, k := range keys {
		g := groups[k]
		slices.SortFunc(g, func(a, b Mount) int {
			if len(a.Point) != len(b.Point) {
				return len(a.Point) - len(b.Point)
			}
			return strings.Compare(a.Point, b.Point)
		})
		out = append(out, g)
	}
	return out, nil
}

func ParseMounts(b []byte) ([]Mount, error) {
	var out []Mount
	ids := map[uint64]bool{}
	sc := bufio.NewScanner(bytes.NewReader(b))
	sc.Buffer(make([]byte, 4096), 1024*1024)
	for sc.Scan() {
		f := strings.Fields(sc.Text())
		sep := slices.Index(f, "-")
		if sep < 6 || len(f) != sep+4 {
			return nil, fmt.Errorf("malformed mountinfo")
		}
		id, e1 := strconv.ParseUint(f[0], 10, 64)
		parent, e2 := strconv.ParseUint(f[1], 10, 64)
		dev := strings.Split(f[2], ":")
		if e1 != nil || e2 != nil || id == 0 || ids[id] || len(dev) != 2 {
			return nil, fmt.Errorf("invalid mount identity")
		}
		for _, n := range dev {
			if _, err := strconv.ParseUint(n, 10, 32); err != nil {
				return nil, err
			}
		}
		decoded := make([]string, len(f))
		for i, v := range f {
			var err error
			decoded[i], err = mountUnescape(v)
			if err != nil {
				return nil, err
			}
		}
		root, point := decoded[3], decoded[4]
		if !path.IsAbs(point) || path.Clean(point) != point || (decoded[sep+1] != "nsfs" && (!path.IsAbs(root) || path.Clean(root) != root)) {
			return nil, fmt.Errorf("invalid mount path")
		}
		for _, opt := range f[6:sep] {
			parts := strings.SplitN(opt, ":", 2)
			if parts[0] == "" {
				return nil, fmt.Errorf("invalid optional mount field")
			}
			switch parts[0] {
			case "shared", "master", "propagate_from":
				if len(parts) != 2 {
					return nil, fmt.Errorf("missing propagation identity")
				}
				n, err := strconv.ParseUint(parts[1], 10, 64)
				if err != nil || n == 0 {
					return nil, fmt.Errorf("invalid propagation identity")
				}
			case "unbindable":
				if len(parts) != 1 {
					return nil, fmt.Errorf("invalid unbindable field")
				}
			}
		}
		ids[id] = true
		out = append(out, Mount{id, parent, f[2], root, point, decoded[sep+1], decoded[sep+2]})
	}
	return out, sc.Err()
}

func mountUnescape(s string) (string, error) {
	var out strings.Builder
	for i := 0; i < len(s); i++ {
		if s[i] != '\\' {
			out.WriteByte(s[i])
			continue
		}
		if i+3 >= len(s) {
			return "", fmt.Errorf("invalid mount escape")
		}
		v := s[i+1 : i+4]
		switch v {
		case "040":
			out.WriteByte(' ')
		case "011":
			out.WriteByte('\t')
		case "012":
			out.WriteByte('\n')
		case "134":
			out.WriteByte('\\')
		default:
			return "", fmt.Errorf("invalid mount escape")
		}
		i += 3
	}
	return out.String(), nil
}

// Local read-only filesystems and major-zero btrfs mounts are valid; neither property identifies an image.
func UnsafeMount(m Mount) bool {
	switch m.Type {
	case "fusectl", "proc", "sysfs", "devtmpfs", "devpts", "tmpfs", "ramfs", "rootfs", "cgroup", "cgroup2", "pstore", "securityfs", "debugfs", "tracefs", "configfs", "efivarfs", "mqueue", "hugetlbfs", "bpf", "binfmt_misc", "rpc_pipefs", "nsfs", "autofs", "overlay", "aufs", "squashfs", "iso9660", "udf", "erofs",
		"nfs", "nfs4", "cifs", "smb3", "smbfs", "ceph", "glusterfs", "afs", "9p", "lustre", "orangefs":
		return true
	}
	if m.Type == "fuse" || m.Type == "fuseblk" || strings.HasPrefix(m.Type, "fuse.") {
		return true
	}
	for _, p := range []string{m.Point, m.Root} {
		for _, part := range strings.Split(p, "/") {
			if strings.HasPrefix(part, "systemd-private-") {
				return true
			}
		}
	}
	return strings.HasPrefix(m.Source, "/dev/loop") || strings.HasPrefix(m.Source, "/dev/ram")
}

func Capacity(s FsStat) (total, used, available *uint64, pct *float64) {
	unit := s.Frsize
	if unit <= 0 {
		unit = s.Bsize
	}
	if unit <= 0 {
		return
	}
	mul := func(n uint64) *uint64 {
		if n > math.MaxInt64/uint64(unit) {
			return nil
		}
		v := n * uint64(unit)
		return &v
	}
	total = mul(s.Blocks)
	if s.Bfree <= s.Blocks {
		used = mul(s.Blocks - s.Bfree)
	}
	if s.Bavail <= s.Blocks {
		available = mul(s.Bavail)
	}
	if used != nil && available != nil && *used <= math.MaxInt64-*available && *used+*available > 0 {
		v := math.Round(1000*float64(*used)/float64(*used+*available)) / 10
		pct = &v
	}
	return
}
