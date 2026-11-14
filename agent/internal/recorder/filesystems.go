package recorder

import (
	"math"
	"reflect"
	"slices"
	"time"

	"github.com/fsrodrig/anshin/agent/internal/recorder/probe"
)

type filesystemState struct {
	mounts                    []probe.Mount
	identity                  *probe.Mount
	generation                uint64
	present, pending, visible bool
	primary                   bool
	verified                  bool
	first, success            time.Time
	value                     probe.Filesystem
	events                    chan filesystemEvent
}

type filesystemEvent struct {
	generation    uint64
	mount         probe.Mount
	visible, done bool
	primary       bool
	value         probe.Filesystem
	success       bool
	at            time.Time
}

func (r *Recorder) filesystems(now time.Time) ([]probe.Filesystem, *string) {
	if r.disks == nil {
		r.disks = map[string]*filesystemState{}
		r.busyPaths = map[string]bool{}
	}
	// A worker publishes visibility before entering statfs so a blocked call still has an unavailable row.
	for key, s := range r.disks {
	drain:
		for s.pending {
			select {
			case e := <-s.events:
				if e.done {
					s.pending = false
				}
				if e.generation == s.generation && s.present {
					s.visible = e.visible
					s.primary = e.primary
					if e.visible {
						s.verified = true
						s.identity = &e.mount
					}
					if e.done {
						s.value = e.value
						if e.success {
							s.success = e.at
						}
					}
				}
			default:
				break drain
			}
		}
		if !s.present && !s.pending {
			delete(r.disks, key)
		}
	}
	if r.diskAt.IsZero() || now.Sub(r.diskAt) >= 30*time.Second {
		r.diskAt = now
		groups, _ := probe.Mounts(r.host)
		for _, s := range r.disks {
			s.present = false
		}
		for _, g := range groups {
			key := g[0].MajMin
			s := r.disks[key]
			if s == nil {
				s = &filesystemState{first: now}
				r.disks[key] = s
			}
			identityChanged := !reflect.DeepEqual(s.mounts, g)
			if s.identity != nil {
				identityChanged = !slices.Contains(g, *s.identity)
			}
			if identityChanged {
				s.generation++
				s.verified = false
				s.identity = nil
				s.primary = false
				s.first = now
				s.success = time.Time{}
				s.visible = false
				s.value = probe.Filesystem{}
			}
			s.mounts = g
			if s.pending && s.verified {
				s.visible = true
			}
			s.present = true
			if !s.pending {
				s.pending = true
				s.value = probe.Filesystem{}
				s.events = make(chan filesystemEvent, 2)
				go r.filesystemProbe(s.generation, g, s.events, now)
			}
		}
		for _, s := range r.disks {
			if !s.present {
				s.generation++
				s.visible = false
				s.value = probe.Filesystem{}
			}
		}
	}
	var out []probe.Filesystem
	var primary *string
	keys := make([]string, 0, len(r.disks))
	for key := range r.disks {
		keys = append(keys, key)
	}
	slices.Sort(keys)
	for _, key := range keys {
		s := r.disks[key]
		if !s.present || !s.visible {
			continue
		}
		v := s.value
		v.MajMin = key
		v.MountPoint = s.mounts[0].Point
		v.Type = s.mounts[0].Type
		at := s.success
		if at.IsZero() {
			at = s.first
		}
		v.Age = math.Floor(max(0, now.Sub(at).Seconds()))
		out = append(out, v)
		if s.primary {
			k := key
			primary = &k
		}
	}
	return out, primary
}

func (r *Recorder) filesystemProbe(generation uint64, mounts []probe.Mount, events chan filesystemEvent, attempt time.Time) {
	started := time.Now()
	e := filesystemEvent{generation: generation, done: true}
	if r.host.MountID == nil || r.host.Statfs == nil {
		events <- e
		return
	}
	for _, m := range mounts {
		// Path ownership survives mount replacement as well as disappearance. Kernel calls cannot be cancelled.
		r.mu.Lock()
		busy := r.busyPaths[m.Point]
		if !busy {
			r.busyPaths[m.Point] = true
		}
		r.mu.Unlock()
		if busy {
			continue
		}
		id, err := r.host.MountID(m.Point)
		if err != nil || id != m.ID {
			r.releasePath(m.Point)
			continue
		}
		e.mount = m
		e.visible = true
		e.primary = m.Point == "/"
		events <- filesystemEvent{generation: generation, visible: true, primary: e.primary, mount: m}
		stat, err := r.host.Statfs(m.Point)
		r.releasePath(m.Point)
		if err == nil {
			e.value.Total, e.value.Used, e.value.Available, e.value.UsedPct = probe.Capacity(stat)
			e.success = true
			e.at = attempt.Add(time.Since(started))
		}
		events <- e
		return
	}
	events <- e
}

func (r *Recorder) releasePath(p string) {
	r.mu.Lock()
	delete(r.busyPaths, p)
	r.mu.Unlock()
}
