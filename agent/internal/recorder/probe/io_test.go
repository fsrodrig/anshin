package probe

import (
	"io/fs"
	"reflect"
	"syscall"
	"testing"
	"testing/fstest"
)

// deniedFS fails Lstat on one path with EACCES, as sysfs can when a directory isn't searchable.
type deniedFS struct {
	fstest.MapFS
	denied string
}

func (d deniedFS) Lstat(name string) (fs.FileInfo, error) {
	if name == d.denied {
		return nil, &fs.PathError{Op: "lstat", Path: name, Err: syscall.EACCES}
	}
	return d.MapFS.Lstat(name)
}

const netDevHeader = "Inter-|   Receive                                                |  Transmit\n" +
	" face |bytes    packets errs drop fifo frame compressed multicast|bytes    packets errs drop fifo colls carrier compressed\n"

func link() *fstest.MapFile { return &fstest.MapFile{Mode: fs.ModeSymlink, Data: []byte("target")} }

func TestNetwork(t *testing.T) {
	m := fstest.MapFS{
		"proc/net/dev": &fstest.MapFile{Data: []byte(netDevHeader +
			"    lo: 10 1 0 0 0 0 0 0 10 1 0 0 0 0 0 0\n" +
			"  eth0: 200 2 0 0 0 0 0 0 300 3 0 0 0 0 0 0\n" +
			"  wlan0: 1 2 3\n" +
			"  wg0: bad 0 0 0 0 0 0 0 7 0 0 0 0 0 0 0\n")},
		"sys/class/net/eth0/device": link(),
	}
	got, err := Network(Host{FS: deniedFS{m, "sys/class/net/wg0/device"}})
	if err != nil {
		t.Fatal(err)
	}
	want := []NetCounters{
		{Name: "lo", DeviceBacked: new(false), Rx: new(uint64(10)), Tx: new(uint64(10))},
		{Name: "eth0", DeviceBacked: new(true), Rx: new(uint64(200)), Tx: new(uint64(300))},
		{Name: "wlan0", DeviceBacked: new(false)},
		{Name: "wg0", Tx: new(uint64(7))},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("Network = %+v, want %+v", got, want)
	}
	if _, err := Network(Host{FS: fstest.MapFS{}}); err == nil {
		t.Fatal("Network on a Machine without /proc/net/dev: want an error")
	}
}

func TestDisks(t *testing.T) {
	m := fstest.MapFS{
		"proc/diskstats": &fstest.MapFile{Data: []byte(
			"   8       0 sda 10 0 100 0 20 0 200\n" +
				"   8      16 sdb 10 0 100 0 20 0 200 0 0 0 0 0 0 0 0 0 0\n" +
				"   8      17 sdb1 10 0 100 0 20 0 200 0 0 0 0 0 0 0 0 0 0\n" +
				" 104       0 cciss/c0d0 1 0 5 0 2 0 6 0 0 0 0 0 0 0\n" +
				"   8      32 sdc 1 0 x 0 2 0 9 0 0 0 0 0 0 0\n" +
				"   8      48 sdd 1 0 1 0 2 0 1 0 0 0 0 0 0 0\n")},
		"sys/class/block/sda/device":        link(),
		"sys/class/block/sdb/device":        link(),
		"sys/class/block/sdb1/partition":    &fstest.MapFile{Data: []byte("1\n")},
		"sys/class/block/cciss!c0d0/device": link(),
		"sys/class/block/sdc/device":        link(),
		"sys/class/block/sdd/device":        link(),
	}
	got, err := Disks(Host{FS: deniedFS{m, "sys/class/block/sdd/device"}})
	if err != nil {
		t.Fatal(err)
	}
	want := []DiskCounters{
		{Name: "sdb", ReadSectors: new(uint64(100)), WriteSectors: new(uint64(200))},
		{Name: "cciss/c0d0", ReadSectors: new(uint64(5)), WriteSectors: new(uint64(6))},
		{Name: "sdc", WriteSectors: new(uint64(9))},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("Disks = %+v, want %+v (sda's row is too short and sdd's link can't be determined, so both are left out)", got, want)
	}
	if _, err := Disks(Host{FS: fstest.MapFS{}}); err == nil {
		t.Fatal("Disks on a Machine without /proc/diskstats: want an error")
	}
}
