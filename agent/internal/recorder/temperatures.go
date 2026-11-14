package recorder

import (
	"maps"
	"slices"
	"sync"
	"time"

	"github.com/fsrodrig/anshin/agent/internal/recorder/probe"
)

// Driver calls cannot be cancelled, so ownership survives the wait bound.
const TemperatureReadTimeout = 50 * time.Millisecond

// SlowTemperatureInterval is the slow lane of docs/design.md § Lanes. Each amdgpu hwmon read restarts the device's
// 5 s runtime-PM autosuspend delay, so reading an awake dGPU on the fast lane would keep it from ever suspending.
const SlowTemperatureInterval = 30 * time.Second

type temperatureResult struct {
	probe.TemperatureDeviceReading
	device    probe.TemperatureDevice
	finished  time.Time
	critIndex int
	critOnly  bool
}

type temperatureDevice struct {
	device   probe.TemperatureDevice
	sensors  []probe.TemperatureSensor
	known    bool
	crit     map[int]*float64
	pending  chan temperatureResult
	launched time.Time
	// values are the sensors of the last timely, complete read, launched at valuesAt.
	values   []probe.TemperatureSensor
	valuesAt time.Time
}

func (d *temperatureDevice) forget() {
	d.sensors, d.known, d.crit, d.values = nil, false, nil, nil
}

type temperatureRead struct {
	done   chan struct{}
	result probe.TemperatureReading
}

type temperatureSampler struct {
	inFlight *temperatureRead
	readMu   sync.Mutex
	devices  map[string]*temperatureDevice
}

// Read takes the Read's monotonic time, so the slow lane's cadence and the ages never consult the host clock.
func (t *temperatureSampler) Read(h probe.Host, override string, now time.Time) probe.TemperatureReading {
	t.readMu.Lock()
	if call := t.inFlight; call != nil {
		t.readMu.Unlock()
		<-call.done
		return call.result
	}
	call := &temperatureRead{done: make(chan struct{})}
	t.inFlight = call
	t.readMu.Unlock()
	call.result = t.read(h, override, now)
	t.readMu.Lock()
	t.inFlight = nil
	close(call.done)
	t.readMu.Unlock()
	return call.result
}

func (t *temperatureSampler) read(h probe.Host, override string, now time.Time) probe.TemperatureReading {
	deadline := time.Now().Add(TemperatureReadTimeout)
	devices := probe.TemperatureDevices(h)
	if t.devices == nil {
		t.devices = map[string]*temperatureDevice{}
	}
	active := map[string]bool{}
	jobs := map[string]chan temperatureResult{}
	changed := make(chan struct{}, len(devices)+1)
	availableCrit := map[string]map[int]*float64{}
	critStarted := map[string]bool{}
	launched := map[string]bool{}
	for _, device := range devices {
		dir := device.Dir
		active[dir] = true
		d := t.devices[dir]
		if d == nil {
			d = &temperatureDevice{}
			t.devices[dir] = d
		}
		if !d.device.Same(device) {
			d.forget()
			d.launched = time.Time{}
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
		availableCrit[dir] = maps.Clone(d.crit)
	}
	fast := t.fastLane(devices, override)
	for _, device := range devices {
		dir := device.Dir
		d := t.devices[dir]
		due := fast[dir] || d.launched.IsZero() || now.Sub(d.launched) >= SlowTemperatureInterval
		if d.pending == nil && due {
			d.values = nil
			d.launched = now
			launched[dir] = true
			d.pending = make(chan temperatureResult, 1)
			ch := d.pending
			var metadata []probe.TemperatureSensor
			if d.known {
				metadata = append([]probe.TemperatureSensor{}, d.sensors...)
			}
			jobs[dir] = ch
			go func() {
				result := probe.ReadTemperatureDeviceSensors(h, device, "", metadata)
				ch <- temperatureResult{TemperatureDeviceReading: result, device: device, finished: time.Now()}
				changed <- struct{}{}
			}()
		}
	}
	for dir, d := range t.devices {
		if active[dir] {
			continue
		}
		d.forget()
		if d.pending != nil {
			select {
			case <-d.pending:
				d.pending = nil
			default:
			}
		}
		if d.pending == nil && now.Sub(d.launched) >= SlowTemperatureInterval {
			delete(t.devices, dir)
		}
	}
	current := map[string]temperatureResult{}
	timer := time.NewTimer(time.Until(deadline))
	defer timer.Stop()
	expired := false
	for {
		for dir, ch := range jobs {
			select {
			case result := <-ch:
				delete(jobs, dir)
				d := t.devices[dir]
				d.pending = nil
				t.remember(d, result)
				if result.Complete && !result.finished.After(deadline) && result.device.Same(d.device) {
					if result.critOnly {
						if availableCrit[dir] == nil {
							availableCrit[dir] = map[int]*float64{}
						}
						availableCrit[dir][result.critIndex] = result.Crit
					} else {
						current[dir] = result
						d.values, d.valuesAt = slices.Clone(result.Sensors), now
					}
				}
			default:
			}
		}
		sensors, chosen := t.sensorsAndCPU(devices, current, launched, override, now)
		if chosen >= 0 && !expired && time.Now().Before(deadline) {
			sensor := sensors[chosen]
			dir := sensor.DeviceDir()
			d := t.devices[dir]
			_, cached := d.crit[sensor.Index()]
			if _, ok := current[dir]; ok && !cached && !critStarted[dir] && d.pending == nil {
				critStarted[dir] = true
				ch := make(chan temperatureResult, 1)
				d.pending, jobs[dir] = ch, ch
				device := d.device
				go func() {
					result := probe.ReadTemperatureCrit(h, device, sensor)
					ch <- temperatureResult{TemperatureDeviceReading: result, device: device, finished: time.Now(), critOnly: true, critIndex: sensor.Index()}
					changed <- struct{}{}
				}()
			}
		}
		if expired || len(jobs) == 0 {
			break
		}
		select {
		case <-changed:
		case <-timer.C:
			expired = true
		}
	}
	for _, device := range devices {
		if !device.SameIdentity(h.FS) {
			t.devices[device.Dir].forget()
			delete(current, device.Dir)
		}
	}
	sensors, chosen := t.sensorsAndCPU(devices, current, launched, override, now)
	var crit *float64
	if chosen >= 0 {
		sensor := sensors[chosen]
		crit = availableCrit[sensor.DeviceDir()][sensor.Index()]
	}
	return probe.TemperatureValues(sensors, chosen, crit)
}

func (t *temperatureSampler) remember(d *temperatureDevice, result temperatureResult) {
	if !result.Complete || !d.device.Same(result.device) {
		return
	}
	if result.critOnly {
		if d.crit == nil {
			d.crit = map[int]*float64{}
		}
		d.crit[result.critIndex] = result.Crit
		return
	}
	d.sensors = slices.Clone(result.Sensors)
	for i := range d.sensors {
		d.sensors[i].TempC = nil
	}
	d.known = true
}

func aged(sensors []probe.TemperatureSensor, age int64) []probe.TemperatureSensor {
	out := slices.Clone(sensors)
	for i := range out {
		out[i].Age = nil
		if out[i].TempC != nil {
			out[i].Age = &age
		}
	}
	return out
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

// fastLane holds the devices read on every Read: those without runtime PM, and the one holding the CPU sensor.
// While the CPU choice is unavailable, every device that could hold it stands in for it, and nothing else does.
func (t *temperatureSampler) fastLane(devices []probe.TemperatureDevice, override string) map[string]bool {
	sensors, chosen := t.sensorsAndCPU(devices, nil, nil, override, time.Time{})
	unknown := false
	for _, device := range devices {
		unknown = unknown || (!t.devices[device.Dir].known && device.CouldSelectCPU(override))
	}
	fast := map[string]bool{}
	for _, device := range devices {
		holdsCPU := chosen >= 0 && sensors[chosen].DeviceDir() == device.Dir
		fast[device.Dir] = !device.RuntimePM() || holdsCPU || (unknown && device.CouldSelectCPU(override))
	}
	return fast
}

// A device not launched in this Read and with no outstanding operation shows its last timely values, aged; a
// device whose read is outstanding or missed the deadline shows its IDs with null temperatures.
func (t *temperatureSampler) sensorsAndCPU(devices []probe.TemperatureDevice, current map[string]temperatureResult, launched map[string]bool, override string, now time.Time) ([]probe.TemperatureSensor, int) {
	var sensors []probe.TemperatureSensor
	unknown := false
	for _, device := range devices {
		d := t.devices[device.Dir]
		unknown = unknown || (!d.known && device.CouldSelectCPU(override))
		if result, ok := current[device.Dir]; ok {
			sensors = append(sensors, aged(result.Sensors, 0)...)
		} else if d.values != nil && d.pending == nil && !launched[device.Dir] {
			sensors = append(sensors, aged(d.values, int64(max(0, now.Sub(d.valuesAt).Seconds())))...)
		} else {
			sensors = append(sensors, d.sensors...)
		}
	}
	chosen := probe.ChooseCPU(sensors, override)
	if unknown {
		chosen = -1
	}
	return sensors, chosen
}
