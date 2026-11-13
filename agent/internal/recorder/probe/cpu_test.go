package probe

import (
	"errors"
	"testing"
	"testing/fstest"
)

func host(files map[string]string) Host {
	m := fstest.MapFS{}
	for name, data := range files {
		m[name] = &fstest.MapFile{Data: []byte(data)}
	}
	return Host{FS: m}
}

func TestCPUReadsTheAggregateLineWithoutGuest(t *testing.T) {
	got, err := CPU(host(map[string]string{"proc/stat": "cpu  1 2 3 4 5 6 7 8 90 91\n" +
		"cpu0 1 0 0 0 0 0 0 0 0 0\ncpu1 0 0 0 0 0 0 0 0 0 0\ncpu3 0 0 0 0 0 0 0 0 0 0\nintr 1 2 3\nctxt 9\n"}))
	if err != nil {
		t.Fatal(err)
	}
	want := CPUTimes{Jiffies: [8]uint64{1, 2, 3, 4, 5, 6, 7, 8}, CPUs: 3}
	if got != want {
		t.Fatalf("CPU = %+v, want %+v", got, want)
	}
	if got.Total() != 36 || got.Idle() != 9 {
		t.Fatalf("Total, Idle = %d, %d; want 36, 9", got.Total(), got.Idle())
	}
}

func TestCPURejectsMalformedStat(t *testing.T) {
	for _, tc := range []struct {
		name, stat string
		want       error
	}{
		{"too short", "cpu  1 2 3\n", ErrShortCPULine},
		{"no cpu line", "intr 1 2 3\n", ErrNoCPULine},
		{"only per-CPU lines", "cpu0 1 2 3 4 5 6 7 8 0 0\n", ErrNoCPULine},
	} {
		if _, err := CPU(host(map[string]string{"proc/stat": tc.stat})); !errors.Is(err, tc.want) {
			t.Errorf("%s: err = %v, want %v", tc.name, err, tc.want)
		}
	}
	if _, err := CPU(host(map[string]string{"proc/stat": "cpu  a b c d e f g h i j\n"})); err == nil {
		t.Error("non-numeric: err = nil, want a parse error")
	}
	if _, err := CPU(host(nil)); err == nil {
		t.Error("missing /proc/stat: err = nil")
	}
}
