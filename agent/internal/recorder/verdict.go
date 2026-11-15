package recorder

import (
	"fmt"
	"math"
	"slices"
	"time"

	"github.com/fsrodrig/anshin/agent/internal/recorder/probe"
)

// Level is a Verdict's or a Reason's level. It is an open string on the wire, so a newer Agent may send others.
type Level string

const (
	LevelOK   Level = "ok"
	LevelWarn Level = "warn"
	LevelCrit Level = "crit"
)

// The fixed defaults in docs/design.md § Verdict.
const (
	filesystemWarnPct = 90.0
	filesystemCritPct = 95.0
	memoryWarnPct     = 10.0
	memoryCritPct     = 5.0
	cpuDefaultCritC   = 95.0
	cpuWarnBelowCritC = 5.0
	batteryWarnPct    = 15.0
	batteryCritPct    = 7.0

	// temperatureSamples is 10 s at the 2 s cadence: long enough that a boost spike doesn't flap the Verdict.
	temperatureSamples = 5
)

// Verdict is nil on a Reading when no input could be judged, because an ok from nothing would treat unavailable as
// healthy.
type Verdict struct {
	Level   Level    `json:"level"`
	Reasons []Reason `json:"reasons"`
}

type Reason struct {
	Code    string `json:"code"`
	Level   Level  `json:"level"`
	Message string `json:"message"`
}

func rank(l Level) int {
	switch l {
	case LevelCrit:
		return 2
	case LevelWarn:
		return 1
	}
	return 0
}

// condition is one debounced temperature condition. It changes state only after the opposite has held for
// temperatureSamples consecutive Samples spanning 8 s, so clearing is as slow as raising.
type condition struct {
	on    bool
	run   int
	since time.Time
}

func (c *condition) observe(holds bool, at time.Time) {
	if holds == c.on {
		c.run = 0
		return
	}
	if c.run == 0 {
		c.since = at
	}
	c.run++
	if c.run >= temperatureSamples && at.Sub(c.since) >= 8*time.Second {
		c.on, c.run = holds, 0
	}
}

type judge struct {
	sensor     string
	warn, crit condition
}

// observe judges one Sample. Until #8 adds the sampling loop, each Read is a Sample.
func (j *judge) observe(r Reading, at time.Time) *Verdict {
	judged := false
	var reasons []Reason
	add := func(code string, l Level, format string, args ...any) {
		judged = true
		if l != LevelOK {
			reasons = append(reasons, Reason{Code: code, Level: l, Message: fmt.Sprintf(format, args...)})
		}
	}
	for _, f := range r.Filesystems {
		if f.UsedPct != nil {
			add("filesystem_full", atLeast(*f.UsedPct, filesystemWarnPct, filesystemCritPct), "%s is %.1f %% full", f.MountPoint, *f.UsedPct)
		}
	}
	if pct := memoryAvailablePct(r.MemoryReading); pct != nil {
		add("memory_low", below(*pct, memoryWarnPct, memoryCritPct), "Only %.1f %% of memory is available", *pct)
	}
	if l, crit, ok := j.temperature(r.TemperatureReading, at); ok {
		add("cpu_hot", l, "CPU at %.1f °C (crit at %.1f °C)", *r.CPUTempC, crit)
	}
	if b := r.Battery; b != nil && b.Pct != nil && b.Status != nil {
		l := LevelOK
		if *b.Status == "Discharging" {
			l = below(*b.Pct, batteryWarnPct, batteryCritPct)
		}
		add("battery_low", l, "Battery at %.1f %% and discharging", *b.Pct)
	}
	if !judged {
		return nil
	}
	slices.SortStableFunc(reasons, func(a, b Reason) int { return rank(b.Level) - rank(a.Level) })
	v := &Verdict{Level: LevelOK, Reasons: []Reason{}}
	if len(reasons) > 0 {
		v.Level, v.Reasons = reasons[0].Level, reasons
	}
	return v
}

// temperature steps the debounce with this Sample's thresholds. ok is false when the CPU temperature is unavailable,
// which also resets the debounce: a condition must hold for consecutive measured Samples of one CPU sensor.
func (j *judge) temperature(t probe.TemperatureReading, at time.Time) (l Level, crit float64, ok bool) {
	if t.CPUTempC == nil || t.CPUSensor == nil {
		*j = judge{}
		return LevelOK, 0, false
	}
	if *t.CPUSensor != j.sensor {
		*j = judge{sensor: *t.CPUSensor}
	}
	crit = cpuDefaultCritC
	if t.CPUCritC != nil {
		crit = *t.CPUCritC
	}
	j.warn.observe(*t.CPUTempC >= crit-cpuWarnBelowCritC, at)
	j.crit.observe(*t.CPUTempC >= crit, at)
	switch {
	case j.crit.on:
		return LevelCrit, crit, true
	case j.warn.on:
		return LevelWarn, crit, true
	}
	return LevelOK, crit, true
}

// memoryAvailablePct is rounded to one decimal before it is compared, like every percent on the wire.
func memoryAvailablePct(m probe.MemoryReading) *float64 {
	if m.Total == nil || m.Available == nil || *m.Total == 0 || *m.Available > *m.Total {
		return nil
	}
	pct := math.Round(1000*float64(*m.Available)/float64(*m.Total)) / 10
	return &pct
}

func atLeast(v, warn, crit float64) Level {
	switch {
	case v >= crit:
		return LevelCrit
	case v >= warn:
		return LevelWarn
	}
	return LevelOK
}

func below(v, warn, crit float64) Level {
	switch {
	case v < crit:
		return LevelCrit
	case v < warn:
		return LevelWarn
	}
	return LevelOK
}

// VerdictOf judges one Reading as a first Sample, with no debounce history, so no temperature condition holds yet.
func VerdictOf(r Reading) *Verdict {
	var j judge
	return j.observe(r, time.Time{})
}
