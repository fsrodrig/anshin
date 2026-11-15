package recorder

import (
	"context"
	"errors"
	"fmt"
	"math"
	"strconv"
	"time"
)

// SampleInterval is the cadence of docs/design.md § The Window: every 2 s whether or not anyone is looking.
const SampleInterval = 2 * time.Second

// WindowRows is 30 minutes at SampleInterval.
const WindowRows = 900

const WindowSpan = 30 * time.Minute

// SuspendThreshold is far above the microseconds between reading the two clocks and below the 2 s cadence, so any
// suspend long enough to hide a Sample is caught.
const SuspendThreshold = time.Second

// Instant is the Machine's clocks at the start of a Sample. The caller reads them; the recorder never does.
type Instant struct {
	// Monotonic is CLOCK_MONOTONIC, which stops during suspend: rates, lanes, ages and the debounce use it.
	Monotonic time.Duration
	// Boottime is CLOCK_BOOTTIME, which counts through suspend, so a Window row sits at its true distance.
	Boottime time.Duration
}

func (i Instant) monotonic() time.Time { return time.Unix(0, int64(i.Monotonic)) }

// Cursor is where the phone is in a Window: the Agent run's epoch and the seq of the newest row it has. The zero
// Cursor is older than any Window, so it always gets the whole Window.
type Cursor struct{ Epoch, Seq uint64 }

var ErrMalformedCursor = errors.New("a Cursor is 16 lowercase hex digits, a dot and a decimal seq")

// ParseCursor accepts only the canonical form, so every Cursor has one spelling.
func ParseCursor(s string) (Cursor, error) {
	if len(s) < 18 || s[16] != '.' {
		return Cursor{}, ErrMalformedCursor
	}
	for _, c := range s[:16] {
		if (c < '0' || c > '9') && (c < 'a' || c > 'f') {
			return Cursor{}, ErrMalformedCursor
		}
	}
	digits := s[17:]
	for _, c := range digits {
		if c < '0' || c > '9' {
			return Cursor{}, ErrMalformedCursor
		}
	}
	if digits[0] == '0' && len(digits) > 1 {
		return Cursor{}, ErrMalformedCursor
	}
	epoch, err := strconv.ParseUint(s[:16], 16, 64)
	if err != nil {
		return Cursor{}, ErrMalformedCursor
	}
	seq, err := strconv.ParseUint(digits, 10, 64)
	if err != nil {
		return Cursor{}, ErrMalformedCursor
	}
	return Cursor{Epoch: epoch, Seq: seq}, nil
}

func (c Cursor) String() string { return fmt.Sprintf("%016x.%d", c.Epoch, c.Seq) }

// Kind says whether a Pulse's Window replaces what the phone holds or appends to it. It is an open string on the
// wire.
type Kind string

const (
	Full  Kind = "full"
	Delta Kind = "delta"
)

// Batch is one Pulse before encoding: the newest Sample's Reading and the Window rows the Cursor asked for.
type Batch struct {
	Kind    Kind
	Cursor  Cursor
	Window  *Window
	Reading Reading
}

// Window is columnar on the wire. Each series is named after the flat field it records. A row whose BoottimeMs is
// nil is a Gap. Elements are pointers because encoding/json writes a []uint8 as base64 and a nil pointer as null.
type Window struct {
	SpanS         int64      `json:"span_s"`
	BoottimeMs    []*int64   `json:"boottime_ms"`
	CPUPct        []*float64 `json:"cpu_pct"`
	Load1         []*float64 `json:"load_1"`
	MemUsedPct    []*float64 `json:"mem_used_pct"`
	SwapUsedBytes []*uint64  `json:"swap_used_bytes"`
	CPUTempC      []*float64 `json:"cpu_temp_c"`
	NetRx         []*uint64  `json:"net_rx_bps"`
	NetTx         []*uint64  `json:"net_tx_bps"`
	DiskRead      []*uint64  `json:"disk_read_bps"`
	DiskWrite     []*uint64  `json:"disk_write_bps"`
	BatteryPct    []*float64 `json:"battery_pct"`
	BatteryPowerW []*float64 `json:"battery_power_w"`
	CPUCorePct    [][]*uint8 `json:"cpu_core_pct"`
}

// Each column's null is a value outside what the wire can carry at that precision, so a stored row is exact and
// window.X[last] equals the flat field bit for bit.
const (
	gapRow     int64  = -1
	nullTenths uint16 = math.MaxUint16
	nullTemp   int16  = math.MinInt16
	nullHundth uint32 = math.MaxUint32
	nullCount  uint64 = math.MaxUint64
	nullCore   uint8  = math.MaxUint8
)

// ring holds the Window in fixed-point columns allocated once, so 30 minutes of Samples cost a known, constant
// amount of memory: 64 bytes per row plus one per CPU (TestWindowStorage).
type ring struct {
	start, n  int
	oldestSeq uint64

	boottime                  []int64
	cpu, mem, battery         []uint16
	temp                      []int16
	load, power               []uint32
	swap, rx, tx, read, write []uint64

	cores int
	core  []uint8
}

func newRing() ring {
	return ring{
		oldestSeq: 1,
		boottime:  make([]int64, WindowRows),
		cpu:       make([]uint16, WindowRows), mem: make([]uint16, WindowRows), battery: make([]uint16, WindowRows),
		temp: make([]int16, WindowRows),
		load: make([]uint32, WindowRows), power: make([]uint32, WindowRows),
		swap: make([]uint64, WindowRows), rx: make([]uint64, WindowRows), tx: make([]uint64, WindowRows),
		read: make([]uint64, WindowRows), write: make([]uint64, WindowRows),
	}
}

func (r *ring) newestSeq() uint64 { return r.oldestSeq + uint64(r.n) - 1 }

func (r *ring) at(i int) int { return (r.start + i) % WindowRows }

// setCores allocates the per-core block the first time a Sample lists CPUs. Earlier rows read as null.
func (r *ring) setCores(n int) {
	r.cores = n
	r.core = make([]uint8, WindowRows*n)
	for i := range r.core {
		r.core[i] = nullCore
	}
}

func (r *ring) push() int {
	if r.n == WindowRows {
		r.evict()
	}
	p := r.at(r.n)
	r.n++
	return p
}

func (r *ring) evict() {
	r.start = (r.start + 1) % WindowRows
	r.n--
	r.oldestSeq++
}

func (r *ring) pushGap() {
	p := r.push()
	r.boottime[p] = gapRow
	r.cpu[p], r.mem[p], r.battery[p] = nullTenths, nullTenths, nullTenths
	r.temp[p] = nullTemp
	r.load[p], r.power[p] = nullHundth, nullHundth
	r.swap[p], r.rx[p], r.tx[p], r.read[p], r.write[p] = nullCount, nullCount, nullCount, nullCount, nullCount
	for c := range r.cores {
		r.core[p*r.cores+c] = nullCore
	}
}

func (r *ring) pushSample(boottime time.Duration, v Reading) {
	p := r.push()
	r.boottime[p] = boottime.Milliseconds()
	r.cpu[p], r.mem[p] = tenths(v.CPUPct), tenths(v.MemoryReading.Pct)
	r.temp[p] = signedTenths(v.CPUTempC)
	r.load[p] = hundredths(v.LoadReading.One)
	r.swap[p], r.rx[p], r.tx[p], r.read[p], r.write[p] = count(v.SwapReading.Used), count(v.NetRx), count(v.NetTx), count(v.DiskRead), count(v.DiskWrite)
	r.battery[p], r.power[p] = nullTenths, nullHundth
	if b := v.Battery; b != nil {
		r.battery[p], r.power[p] = tenths(b.Pct), hundredths(b.PowerW)
	}
	for c := range r.cores {
		r.core[p*r.cores+c] = nullCore
		if c < len(v.CPUCorePct) && v.CPUCorePct[c] != nil {
			r.core[p*r.cores+c] = *v.CPUCorePct[c]
		}
	}
	r.evictOlderThan(boottime.Milliseconds() - WindowSpan.Milliseconds())
}

// evictOlderThan drops Samples at or before cutoff, oldest first. A Gap goes with the Sample after it, so a Window
// may start with a Gap (the Machine slept before its first Sample) but never with a hole.
func (r *ring) evictOlderThan(cutoff int64) {
	old := func(i int) bool { b := r.boottime[r.at(i)]; return b != gapRow && b <= cutoff }
	for r.n > 0 {
		if b := r.boottime[r.at(0)]; b == gapRow {
			if r.n > 1 && old(1) {
				r.evict()
				continue
			}
			return
		}
		if !old(0) {
			return
		}
		r.evict()
	}
}

// window copies rows [from, n) out of the ring. Each series is backed by one array, so a full Window costs two
// allocations per series rather than one per value.
func (r *ring) window(from int) *Window {
	n := r.n - from
	w := &Window{SpanS: int64(WindowSpan / time.Second)}
	boot := make([]int64, n)
	w.BoottimeMs = make([]*int64, n)
	for i := range n {
		if b := r.boottime[r.at(from+i)]; b != gapRow {
			boot[i] = b
			w.BoottimeMs[i] = &boot[i]
		}
	}
	rows := func(get func(p int) (float64, bool)) []*float64 {
		vals, out := make([]float64, n), make([]*float64, n)
		for i := range n {
			if v, ok := get(r.at(from + i)); ok {
				vals[i] = v
				out[i] = &vals[i]
			}
		}
		return out
	}
	counts := func(col []uint64) []*uint64 {
		vals, out := make([]uint64, n), make([]*uint64, n)
		for i := range n {
			if v := col[r.at(from+i)]; v != nullCount {
				vals[i] = v
				out[i] = &vals[i]
			}
		}
		return out
	}
	fromTenths := func(col []uint16) []*float64 {
		return rows(func(p int) (float64, bool) { return float64(col[p]) / 10, col[p] != nullTenths })
	}
	fromHundredths := func(col []uint32) []*float64 {
		return rows(func(p int) (float64, bool) { return float64(col[p]) / 100, col[p] != nullHundth })
	}
	w.CPUPct, w.MemUsedPct, w.BatteryPct = fromTenths(r.cpu), fromTenths(r.mem), fromTenths(r.battery)
	w.CPUTempC = rows(func(p int) (float64, bool) { return float64(r.temp[p]) / 10, r.temp[p] != nullTemp })
	w.Load1, w.BatteryPowerW = fromHundredths(r.load), fromHundredths(r.power)
	w.SwapUsedBytes, w.NetRx, w.NetTx, w.DiskRead, w.DiskWrite = counts(r.swap), counts(r.rx), counts(r.tx), counts(r.read), counts(r.write)
	if r.cores > 0 {
		vals := make([]uint8, r.cores*n)
		w.CPUCorePct = make([][]*uint8, r.cores)
		for c := range r.cores {
			w.CPUCorePct[c] = make([]*uint8, n)
			for i := range n {
				if v := r.core[r.at(from+i)*r.cores+c]; v != nullCore {
					vals[c*n+i] = v
					w.CPUCorePct[c][i] = &vals[c*n+i]
				}
			}
		}
	}
	return w
}

func tenths(v *float64) uint16 {
	if v == nil || !(*v >= 0 && *v*10 < float64(nullTenths)) {
		return nullTenths
	}
	return uint16(math.Round(*v * 10))
}

func signedTenths(v *float64) int16 {
	if v == nil || !(*v*10 > math.MinInt16 && *v*10 <= math.MaxInt16) {
		return nullTemp
	}
	return int16(math.Round(*v * 10))
}

func hundredths(v *float64) uint32 {
	if v == nil || !(*v >= 0 && *v*100 < float64(nullHundth)) {
		return nullHundth
	}
	return uint32(math.Round(*v * 100))
}

func count(v *uint64) uint64 {
	if v == nil {
		return nullCount
	}
	return *v
}

// Read answers a Cursor without measuring anything: a delta of the rows newer than a valid Cursor, otherwise the
// whole Window. A Cursor is valid when it is this run's and still inside the Window.
func (r *Recorder) Read(c Cursor) Batch {
	r.windowMu.Lock()
	defer r.windowMu.Unlock()
	b := Batch{Kind: Full, Cursor: Cursor{r.cfg.Epoch, r.ring.newestSeq()}, Reading: r.newest}
	from := 0
	if r.valid(c) {
		b.Kind, from = Delta, int(c.Seq-r.ring.oldestSeq+1)
	}
	b.Window = r.ring.window(from)
	return b
}

func (r *Recorder) valid(c Cursor) bool {
	return c.Epoch == r.cfg.Epoch && r.ring.oldestSeq <= c.Seq && c.Seq <= r.ring.newestSeq()
}

// Wait holds a Cursor that is already at the newest row until the next Sample lands, then answers it as Read does.
// Any other Cursor is answered at once. It returns ctx.Err() if ctx ends first. Nothing is registered, so a
// cancelled Wait leaves nothing behind.
func (r *Recorder) Wait(ctx context.Context, c Cursor) (Batch, error) {
	r.windowMu.Lock()
	held := r.valid(c) && c.Seq == r.ring.newestSeq()
	newer := r.newer
	r.windowMu.Unlock()
	if held {
		select {
		case <-newer:
		case <-ctx.Done():
			return Batch{}, ctx.Err()
		}
	}
	return r.Read(c), nil
}

// publish writes a Sample, and the Gap before it when there was one, in one step, so no Read sees a Window that
// ends with a Gap.
func (r *Recorder) publish(at Instant, v Reading, gap bool) {
	r.windowMu.Lock()
	defer r.windowMu.Unlock()
	if r.ring.core == nil && v.CPUCorePct != nil {
		r.ring.setCores(len(v.CPUCorePct))
	}
	if gap && r.ring.n > 0 {
		r.ring.pushGap()
	}
	r.ring.pushSample(at.Boottime, v)
	r.newest = v
	close(r.newer)
	r.newer = make(chan struct{})
}
