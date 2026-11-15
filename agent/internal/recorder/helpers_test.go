package recorder

import (
	"time"

	"github.com/fsrodrig/anshin/agent/internal/recorder/probe"
)

// instantAt is a Machine that never slept: both clocks read t.
func instantAt(t time.Time) Instant {
	d := time.Duration(t.UnixNano())
	return Instant{Monotonic: d, Boottime: d}
}

func openAt(h probe.Host, t time.Time, cfg Config) *Recorder { return Open(h, instantAt(t), cfg) }

func (r *Recorder) sampleAt(t time.Time) Reading { return r.sample(instantAt(t)) }
