package recorder

import (
	"slices"
	"sync"
	"time"

	"github.com/fsrodrig/anshin/agent/internal/recorder/probe"
)

// Driver calls cannot be cancelled, so ownership survives the wait bound.
const TemperatureReadTimeout = 50 * time.Millisecond

type temperatureResult struct {
	probe.TemperatureDeviceReading
	device   probe.TemperatureDevice
	finished time.Time
}

type temperatureDevice struct {
	device  probe.TemperatureDevice
	sensors []probe.TemperatureSensor
	known   bool
	pending chan temperatureResult
}

type temperatureSampler struct {
	readMu  sync.Mutex
	devices map[string]*temperatureDevice
	cpuID   string
	cpuDir  string
}

func (t *temperatureSampler) Read(h probe.Host, override string) probe.TemperatureReading {
	t.readMu.Lock()
	defer t.readMu.Unlock()
	deadline := time.Now().Add(TemperatureReadTimeout)
	devices := probe.TemperatureDevices(h)
	if t.devices == nil {
		t.devices = map[string]*temperatureDevice{}
	}
	active := map[string]bool{}
	jobs := map[string]chan temperatureResult{}
	for _, device := range devices {
		dir := device.Dir
		active[dir] = true
		d := t.devices[dir]
		if d == nil {
			d = &temperatureDevice{}
			t.devices[dir] = d
		}
		if !d.device.Same(device) {
			d.sensors = nil
			d.known = false
		}
		d.device = device
		if d.pending != nil {
			select {
			case result := <-d.pending:
				t.remember(d, result)
				d.pending = nil
			default:
			}
		}
		if d.pending == nil {
			d.pending = make(chan temperatureResult, 1)
			ch, cpuID := d.pending, ""
			if dir == t.cpuDir {
				cpuID = t.cpuID
			}
			jobs[dir] = ch
			go func() {
				result := probe.ReadTemperatureDevice(h, device, cpuID)
				ch <- temperatureResult{TemperatureDeviceReading: result, device: device, finished: time.Now()}
			}()
		}
	}
	for dir, d := range t.devices {
		if active[dir] {
			continue
		}
		d.sensors = nil
		d.known = false
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
	current := map[string]temperatureResult{}
	for _, device := range devices {
		dir := device.Dir
		if ch := jobs[dir]; ch != nil {
			if result, ok := waitTemperature(ch, deadline); ok {
				d := t.devices[dir]
				d.pending = nil
				t.remember(d, result)
				if result.Complete && !result.finished.After(deadline) && result.device.Same(device) {
					current[dir] = result
				}
			}
		}
	}
	// A replacement or failed guard invalidates even a timely completion.
	verified := probe.TemperatureDevices(h)
	var sensors []probe.TemperatureSensor
	unknown := false
	var crit *float64
	for _, device := range devices {
		d := t.devices[device.Dir]
		if !slices.ContainsFunc(verified, device.Same) {
			d.sensors = nil
			d.known = false
			continue
		}
		unknown = unknown || (!d.known && device.CouldSelectCPU(override))
		if result, ok := current[device.Dir]; ok {
			sensors = append(sensors, result.Sensors...)
			if result.Crit != nil {
				crit = result.Crit
			}
		} else {
			sensors = append(sensors, d.sensors...)
		}
	}
	chosen := probe.ChooseCPU(sensors, override)
	if unknown {
		chosen = -1
	}
	previous, previousDir := t.cpuID, t.cpuDir
	t.cpuID = ""
	t.cpuDir = ""
	if chosen >= 0 {
		t.cpuID = sensors[chosen].ID
		t.cpuDir = sensors[chosen].DeviceDir()
	}
	if t.cpuID != previous || t.cpuDir != previousDir {
		crit = nil
	}
	return probe.TemperatureValues(sensors, chosen, crit)
}

func (t *temperatureSampler) remember(d *temperatureDevice, result temperatureResult) {
	if !result.Complete || !d.device.Same(result.device) {
		return
	}
	d.sensors = slices.Clone(result.Sensors)
	for i := range d.sensors {
		d.sensors[i].TempC = nil
	}
	d.known = true
}

func waitTemperature(ch <-chan temperatureResult, deadline time.Time) (temperatureResult, bool) {
	select {
	case v := <-ch:
		return v, true
	default:
	}
	timer := time.NewTimer(time.Until(deadline))
	defer timer.Stop()
	select {
	case v := <-ch:
		return v, true
	case <-timer.C:
		select {
		case v := <-ch:
			return v, true
		default:
			return temperatureResult{}, false
		}
	}
}
