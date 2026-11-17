package probe

import (
	"fmt"
	"math"
	"testing"
	"testing/fstest"
)

func mountHost(table string) Host {
	return Host{FS: fstest.MapFS{"proc/self/mountinfo": {Data: []byte(table)}}}
}

func TestFiveBtrfsSubvolumesAndSeparateDisk(t *testing.T) {
	table := ""
	for i, p := range []string{"/", "/home", "/var/log", "/var/cache", "/snapshots"} {
		table += fmt.Sprintf("%d 1 0:35 /sub%d %s rw shared:1 - btrfs /dev/mapper/disk rw\n", i+2, i, p)
	}
	table += "9 2 8:1 / /boot ro - ext4 /dev/disk ro\n"
	groups, _, err := Mounts(mountHost(table))
	if err != nil || len(groups) != 2 || len(groups[0]) != 5 || groups[0][0].Point != "/" || groups[1][0].Point != "/boot" {
		t.Fatalf("%+v %v", groups, err)
	}
}

func TestMountEscapesAndOptionalFields(t *testing.T) {
	m, err := ParseMounts([]byte("2 1 0:35 /a\\134b /space\\040tab\\011line\\012end rw shared:2 master:1 propagate_from:3 unbindable future:abc - btrfs /dev/disk rw\n"))
	if err != nil || len(m) != 1 || m[0].Root != "/a\\b" || m[0].Point != "/space tab\tline\nend" {
		t.Fatalf("%+v %v", m, err)
	}
	for _, line := range []string{"2 1 0:35 / /bad\\041 rw - ext4 /dev/disk rw", "2 1 bad / / rw - ext4 /dev/disk rw", "0 1 8:1 / / rw - ext4 /dev/disk rw", "2 1 8:1 / /a/../b rw - ext4 /dev/disk rw", "2 1 8:1 / / rw shared:bad - ext4 disk rw", "2 1 8:1 / / rw master:0 - ext4 disk rw", "2 1 8:1 / / rw - ext4 disk", "2 1 8:1 / / rw - ext4 disk rw\n2 1 8:2 / /b rw - ext4 disk rw"} {
		if _, _, err := Mounts(mountHost(line)); err == nil {
			t.Errorf("accepted %q", line)
		}
	}
}

func TestUnsafeMountsAndAncestors(t *testing.T) {
	for _, kind := range []string{"proc", "sysfs", "autofs", "nfs4", "cifs", "9p", "fuse", "fuseblk", "fuse.sshfs", "squashfs", "iso9660", "erofs", "overlay"} {
		t.Run(kind, func(t *testing.T) {
			table := fmt.Sprintf("2 1 8:1 / /bad rw - %s source rw\n3 2 8:2 / /bad/child rw - ext4 /dev/disk rw\n4 1 8:3 / /okay ro - ext4 /dev/disk ro\n", kind)
			g, _, err := Mounts(mountHost(table))
			if err != nil || len(g) != 1 || g[0][0].Point != "/okay" {
				t.Fatalf("%+v %v", g, err)
			}
		})
	}
	for _, pair := range [][2]string{{"/tmp/systemd-private-token-unit/tmp", "disk"}, {"/images", "/dev/loop0"}, {"/memory", "/dev/ram0"}} {
		table := fmt.Sprintf("2 1 8:1 / %s rw - ext4 %s rw\n3 2 8:2 / %s/child rw - btrfs /dev/disk rw\n", pair[0], pair[1], pair[0])
		if g, _, err := Mounts(mountHost(table)); err != nil || len(g) != 0 {
			t.Fatalf("%+v %v", g, err)
		}
	}
	// A malformed ancestor must not silently disappear and expose its child to probing.
	if g, _, err := Mounts(mountHost("bad\n3 2 8:2 / /bad/child rw - ext4 disk rw")); err == nil || g != nil {
		t.Fatal(g, err)
	}
}

func TestCapacity(t *testing.T) {
	total, used, avail, pct := Capacity(FsStat{Frsize: 1024, Blocks: 100, Bfree: 30, Bavail: 20})
	if *total != 102400 || *used != 71680 || *avail != 20480 || *pct != 77.8 {
		t.Fatal(total, used, avail, pct)
	}
	_, used, _, pct = Capacity(FsStat{Bsize: 4096, Blocks: 10, Bfree: 10, Bavail: 10})
	if used == nil || *used != 0 || pct == nil || *pct != 0 {
		t.Fatal("genuine zero lost")
	}
	for _, s := range []FsStat{{}, {Frsize: 1, Blocks: 0}, {Frsize: 4096, Blocks: math.MaxUint64, Bfree: math.MaxUint64, Bavail: 1}, {Frsize: 1, Blocks: 10, Bfree: 11, Bavail: 3}, {Frsize: 1, Blocks: 10, Bfree: 5, Bavail: 11}} {
		total, used, avail, pct := Capacity(s)
		if pct != nil && s.Blocks != math.MaxUint64 {
			t.Fatalf("invalid percentage %+v: %v", s, *pct)
		}
		if s.Blocks == math.MaxUint64 && (total != nil || used == nil || *used != 0 || avail == nil) {
			t.Fatal("fields must be independently nullable")
		}
	}
}

func TestRealDiskBelowRun(t *testing.T) {
	table := "2 1 8:1 / / rw - ext4 disk rw\n3 2 0:20 / /run rw - tmpfs tmpfs rw\n4 3 8:17 / /run/media/user/archive rw - ext4 disk rw\n5 4 8:17 / /archive rw - ext4 disk rw\n"
	groups, _, err := Mounts(mountHost(table))
	if err != nil || len(groups) != 2 || len(groups[1]) != 2 || groups[1][0].Point != "/archive" {
		t.Fatalf("groups=%+v err=%v", groups, err)
	}
}
