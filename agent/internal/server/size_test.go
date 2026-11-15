package server

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/fsrodrig/anshin/agent/internal/capture/replay"
	"github.com/fsrodrig/anshin/agent/internal/recorder"
)

// fullWindow serves a recorded Reading with a full Window built from two recorded rows.
type fullWindow struct {
	fixedRecorder
	window *recorder.Window
}

func (f fullWindow) Read(recorder.Cursor) recorder.Batch {
	return recorder.Batch{Kind: recorder.Full, Cursor: recorder.Cursor{Epoch: ^uint64(0), Seq: 1 << 40}, Reading: f.r, Window: f.window}
}

// TestFullPulseSize measures the bytes of a full Pulse with a 900-row Window, for docs/design.md § Budgets. Its rows
// alternate the desktop capture's last two Samples, which have every rate and per-core value, so no row is shortened
// by nulls; the flat fields are the capture's last Sample. Run with -v to see the number.
func TestFullPulseSize(t *testing.T) {
	for _, c := range replay.LoadAll(t) {
		rec, _ := replayed(t, c, false)
		_, body := get(t, http.MethodGet, served(t, rec)+"/v1/pulse")
		plain := len(body)
		b := rec.Read(recorder.Cursor{})
		src, n := b.Window, len(b.Window.BoottimeMs)
		w := &recorder.Window{SpanS: src.SpanS, CPUCorePct: make([][]*uint8, len(src.CPUCorePct))}
		start := *src.BoottimeMs[n-1] - 2_000*(recorder.WindowRows-1)
		for i := range recorder.WindowRows {
			from := n - 2 + i%2
			ms := start + 2_000*int64(i)
			w.BoottimeMs = append(w.BoottimeMs, &ms)
			w.CPUPct, w.Load1, w.MemUsedPct = append(w.CPUPct, src.CPUPct[from]), append(w.Load1, src.Load1[from]), append(w.MemUsedPct, src.MemUsedPct[from])
			w.SwapUsedBytes, w.CPUTempC = append(w.SwapUsedBytes, src.SwapUsedBytes[from]), append(w.CPUTempC, src.CPUTempC[from])
			w.NetRx, w.NetTx = append(w.NetRx, src.NetRx[from]), append(w.NetTx, src.NetTx[from])
			w.DiskRead, w.DiskWrite = append(w.DiskRead, src.DiskRead[from]), append(w.DiskWrite, src.DiskWrite[from])
			w.BatteryPct, w.BatteryPowerW = append(w.BatteryPct, src.BatteryPct[from]), append(w.BatteryPowerW, src.BatteryPowerW[from])
			for core := range w.CPUCorePct {
				w.CPUCorePct[core] = append(w.CPUCorePct[core], src.CPUCorePct[core][from])
			}
		}
		status, full := get(t, http.MethodGet, served(t, fullWindow{fixedRecorder{b.Reading}, w})+"/v1/pulse")
		if status != http.StatusOK {
			t.Fatal(status)
		}
		perCore, err := json.Marshal(w.CPUCorePct)
		if err != nil {
			t.Fatal(err)
		}
		t.Logf("%s, %d CPUs: full Pulse with a %d-row Window is %d bytes (the replay's 3-row Pulse is %d); %d of them are window.cpu_core_pct",
			c.Name, len(w.CPUCorePct), recorder.WindowRows, len(full), plain, len(perCore))
	}
}
