package probe

import (
	"cmp"
	"io/fs"
	"os"
	"path"
	"slices"
	"strconv"
	"sync"
	"time"
)

// Driver calls cannot be cancelled. One deadline covers discovery and values across all devices.
const TemperatureReadTimeout = 50 * time.Millisecond

type temperatureResult struct {
	sensors   []hwmonSensor
	crit      *float64
	discovery bool
	finished  time.Time
}

type temperatureDevice struct {
	identity   fs.FileInfo
	name, link string
	sensors    []hwmonSensor
	pending    chan temperatureResult
	discard    bool
}

type TemperatureSampler struct {
	readMu      sync.Mutex
	mu          sync.Mutex
	unavailable TemperatureReading
	devices     map[string]*temperatureDevice
}

func (t *TemperatureSampler) Read(h Host, override string) TemperatureReading {
	// Concurrent Pulses must not queue behind even a bounded driver wait.
	if !t.readMu.TryLock() {
		t.mu.Lock()
		defer t.mu.Unlock()
		return t.unavailable
	}
	defer t.readMu.Unlock()
	deadline := time.Now().Add(TemperatureReadTimeout)
	entries, _ := fs.ReadDir(h.FS, "sys/class/hwmon")
	slices.SortFunc(entries, func(a, b fs.DirEntry) int {
		return cmp.Or(cmp.Compare(len(a.Name()), len(b.Name())), cmp.Compare(a.Name(), b.Name()))
	})
	if t.devices == nil {
		t.devices = map[string]*temperatureDevice{}
	}
	active := map[string]*temperatureDevice{}
	for _, e := range entries {
		dir := "sys/class/hwmon/" + e.Name()
		name, ok := hwmonName(h.FS, dir)
		if !ok {
			continue
		}
		info, err := fs.Stat(h.FS, dir)
		link, _ := fs.ReadLink(h.FS, dir+"/device")
		d := t.devices[dir]
		if d == nil {
			d = &temperatureDevice{}
			t.devices[dir] = d
		}
		same := d.identity != nil && err == nil && d.name == name && d.link == path.Base(link) && sameTemperatureDevice(d.identity, info)
		if !same {
			d.sensors = nil
			d.discard = true
		}
		d.identity, d.name, d.link = info, name, path.Base(link)
		// Late values are never current, but completed discovery can establish sensor identities.
		if d.pending != nil {
			select {
			case result := <-d.pending:
				if same && !d.discard && result.discovery {
					d.sensors = result.sensors
				}
				d.pending = nil
			default:
			}
		}
		if err != nil {
			continue
		}
		active[dir] = d
		if d.sensors == nil && d.pending == nil {
			d.discard = false
			d.pending = make(chan temperatureResult, 1)
			ch := d.pending
			go func() {
				ids := hwmonSensorIDs(h.FS, dir, name)
				ch <- temperatureResult{sensors: ids, discovery: true, finished: time.Now()}
			}()
		}
	}
	// Keep path ownership through removal/replacement until an uncancellable call actually returns.
	for dir, d := range t.devices {
		if active[dir] == nil {
			d.sensors = nil
			d.discard = true
			if d.pending != nil {
				select {
				case <-d.pending:
					d.pending = nil
				default:
				}
			}
			if d.pending == nil {
				delete(t.devices, dir)
			}
		}
	}
	var sensors []hwmonSensor
	unknown := false
	for _, e := range entries {
		dir := "sys/class/hwmon/" + e.Name()
		d := active[dir]
		if d == nil {
			continue
		}
		if d.sensors == nil && d.pending != nil {
			if result, ok := waitTemperature(d.pending, deadline); ok {
				d.pending = nil
				if !d.discard && result.discovery {
					d.sensors = result.sensors
				}
			}
		}
		unknown = unknown || d.sensors == nil
		sensors = append(sensors, d.sensors...)
	}
	chosen := chooseCPU(sensors, override)
	// Until labels have answered, choosing another device could silently substitute a different CPU sensor.
	if unknown {
		chosen = -1
	}
	unavailable := temperatureReading(sensors, chosen, nil)
	t.mu.Lock()
	t.unavailable = unavailable
	t.mu.Unlock()
	jobs := map[string]chan temperatureResult{}
	for dir, d := range active {
		if d.pending != nil || d.sensors == nil {
			continue
		}
		d.discard = false
		d.pending = make(chan temperatureResult, 1)
		ch := d.pending
		jobs[dir] = ch
		ids := slices.Clone(d.sensors)
		cpu := -1
		if chosen >= 0 && sensors[chosen].dir == dir {
			cpu = sensors[chosen].index
		}
		go func() {
			result := temperatureResult{sensors: ids}
			for i := range ids {
				s := &ids[i]
				s.TempC = celsius(fs.ReadFile(h.FS, dir+"/temp"+strconv.Itoa(s.index)+"_input"))
			}
			if cpu >= 0 {
				result.crit = celsius(fs.ReadFile(h.FS, dir+"/temp"+strconv.Itoa(cpu)+"_crit"))
			}
			result.finished = time.Now()
			ch <- result
		}()
	}
	var crit *float64
	for dir, ch := range jobs {
		if result, ok := waitTemperature(ch, deadline); ok {
			active[dir].pending = nil
			if result.finished.After(deadline) {
				continue
			}
			info, err := fs.Stat(h.FS, dir)
			if err != nil || !sameTemperatureDevice(active[dir].identity, info) {
				active[dir].sensors = nil
				continue
			}
			for i := range sensors {
				for _, s := range result.sensors {
					if sensors[i].dir == s.dir && sensors[i].index == s.index {
						sensors[i].TempC = s.TempC
					}
				}
			}
			if chosen >= 0 && sensors[chosen].dir == dir {
				crit = result.crit
			}
		}
	}
	return temperatureReading(sensors, chosen, crit)
}

func sameTemperatureDevice(a, b fs.FileInfo) bool {
	if a.Sys() != nil && b.Sys() != nil {
		return os.SameFile(a, b)
	}
	return a.Name() == b.Name() && a.ModTime() == b.ModTime()
}

func waitTemperature(ch <-chan temperatureResult, deadline time.Time) (temperatureResult, bool) {
	timer := time.NewTimer(time.Until(deadline))
	defer timer.Stop()
	select {
	case v := <-ch:
		return v, true
	case <-timer.C:
		return temperatureResult{}, false
	}
}

func temperatureReading(sensors []hwmonSensor, chosen int, crit *float64) TemperatureReading {
	var r TemperatureReading
	for _, s := range sensors {
		r.Sensors = append(r.Sensors, Sensor{ID: s.ID, TempC: s.TempC})
	}
	if chosen >= 0 {
		id := sensors[chosen].ID
		r.CPUSensor = &id
		r.CPUTempC = sensors[chosen].TempC
		r.CPUCritC = crit
	}
	return r
}
