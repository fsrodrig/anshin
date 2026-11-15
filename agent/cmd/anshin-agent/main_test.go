package main

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/fsrodrig/anshin/agent/internal/recorder"
)

func TestCPUSensorConfiguration(t *testing.T) {
	for _, s := range []string{"", "k10temp/Tctl", "coretemp/Package id 0", "it87/temp3"} {
		t.Setenv("ANSHIN_CPU_SENSOR", s)
		if got, err := cpuSensor(); err != nil || got != s {
			t.Errorf("ANSHIN_CPU_SENSOR=%q: got %q, %v; want it verbatim", s, got, err)
		}
	}
	for _, s := range []string{"k10temp", "/Tctl", "k10temp/", " k10temp/Tctl", "k10temp/Tctl ", "k10 temp/Tctl", "k10temp/Tctl\n"} {
		t.Setenv("ANSHIN_CPU_SENSOR", s)
		if _, err := cpuSensor(); err == nil || !strings.Contains(err.Error(), "ANSHIN_CPU_SENSOR") {
			t.Errorf("ANSHIN_CPU_SENSOR=%q: err = %v, want a configuration error naming it", s, err)
		}
	}
}

type countingSampler struct {
	mu sync.Mutex
	at []recorder.Instant
}

func (c *countingSampler) Sample(at recorder.Instant) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.at = append(c.at, at)
}

func (c *countingSampler) count() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return len(c.at)
}

func TestSampleEverySamplesEachTickAndStopsOnCancel(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	var rec countingSampler
	var calls int
	clocks := func() (recorder.Instant, error) {
		calls++
		if calls == 2 {
			return recorder.Instant{}, errors.New("clock unavailable")
		}
		d := time.Duration(calls) * time.Second
		return recorder.Instant{Monotonic: d, Boottime: d}, nil
	}
	done := make(chan struct{})
	go func() { sampleEvery(ctx, &rec, time.Millisecond, clocks); close(done) }()
	for deadline := time.Now().Add(5 * time.Second); rec.count() < 5; time.Sleep(time.Millisecond) {
		if time.Now().After(deadline) {
			t.Fatal("sampleEvery did not sample on each tick")
		}
	}
	cancel()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("sampleEvery kept running after cancel")
	}
	n := rec.count()
	time.Sleep(10 * time.Millisecond)
	if rec.count() != n {
		t.Fatal("sampled after it stopped")
	}
	for _, at := range rec.at {
		if at.Monotonic == 2*time.Second {
			t.Fatal("a tick whose clocks failed was sampled")
		}
	}
}
