package probe

import (
	"errors"
	"io/fs"
	"math"
	"strconv"
	"strings"
)

// BatteryReading is the Machine's system batteries. A nil *BatteryReading is a Machine without one. When a supply
// can't be classified, the summary fields are all nil, because that supply could be a system battery.
type BatteryReading struct {
	Pct       *float64     `json:"pct"`
	Status    *string      `json:"status"`
	PowerW    *float64     `json:"power_w"`
	TimeLeft  *uint64      `json:"time_left_s"`
	Batteries []BatteryRow `json:"batteries"`
}

// BatteryRow is one system battery. Status is the kernel's string verbatim; PowerW is a magnitude, and the
// direction comes from Status.
type BatteryRow struct {
	Name   string   `json:"name"`
	Pct    *float64 `json:"pct"`
	Status *string  `json:"status"`
	PowerW *float64 `json:"power_w"`
	Health *float64 `json:"health_pct"`
}

const supplyClass = "sys/class/power_supply"

type supplyKind int

const (
	notBattery supplyKind = iota
	systemBattery
	unclassified
)

// family is a battery's unit system: energy in µWh and µW, or charge in µAh and µA. The two are never mixed,
// because converting between them would need a voltage guess.
type family int

const (
	noFamily family = iota
	energy
	charge
)

// cell is one battery's raw values, in µ-units, for the Machine-level sums.
type cell struct {
	row                                 BatteryRow
	energyNow, energyFull, energyDesign *float64
	chargeNow, chargeFull, chargeDesign *float64
	powerNow, currentNow, voltageNow    *float64
	watts                               *float64
}

func Battery(h Host) *BatteryReading {
	entries, err := fs.ReadDir(h.FS, supplyClass)
	if errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	if err != nil {
		return &BatteryReading{}
	}
	var cells []cell
	undetermined := false
	for _, e := range entries {
		base := supplyClass + "/" + e.Name()
		switch classify(h, base) {
		case unclassified:
			undetermined = true
		case systemBattery:
			cells = append(cells, readCell(h, e.Name(), base))
		}
	}
	if len(cells) == 0 && !undetermined {
		return nil
	}
	r := &BatteryReading{}
	for _, c := range cells {
		r.Batteries = append(r.Batteries, c.row)
	}
	if !undetermined {
		summarize(r, cells)
	}
	return r
}

// classify reads type and then scope, and nothing else, before a supply is known to be a system battery. A
// peripheral's directory name can carry a Bluetooth MAC or a USB serial, and reading a HID battery's capacity can
// send the device a request.
func classify(h Host, base string) supplyKind {
	b, err := fs.ReadFile(h.FS, base+"/type")
	switch {
	case errors.Is(err, fs.ErrNotExist):
		return notBattery
	case err != nil:
		return unclassified
	case strings.TrimSpace(string(b)) != "Battery":
		return notBattery
	}
	b, err = fs.ReadFile(h.FS, base+"/scope")
	switch {
	case errors.Is(err, fs.ErrNotExist):
		// ACPI batteries have no scope attribute.
		return systemBattery
	case err != nil:
		return unclassified
	case strings.TrimSpace(string(b)) == "Device":
		return notBattery
	}
	return systemBattery
}

func attr(h Host, base, name string) *float64 {
	b, err := fs.ReadFile(h.FS, base+"/"+name)
	if err != nil {
		return nil
	}
	n, err := strconv.ParseInt(strings.TrimSpace(string(b)), 10, 64)
	if err != nil {
		return nil
	}
	v := float64(n)
	return &v
}

func atLeast(v *float64, lo float64) *float64 {
	if v == nil || *v < lo {
		return nil
	}
	return v
}

// magnitude drops the sign: the ABI documents a negative current while discharging, but ACPI reports a positive
// rate, so a signed value would mean different things on different Machines.
func magnitude(v *float64) *float64 {
	if v == nil {
		return nil
	}
	m := math.Abs(*v)
	return &m
}

func readCell(h Host, name, base string) cell {
	c := cell{
		energyNow: atLeast(attr(h, base, "energy_now"), 0), energyFull: atLeast(attr(h, base, "energy_full"), 0),
		energyDesign: atLeast(attr(h, base, "energy_full_design"), 0),
		chargeNow:    atLeast(attr(h, base, "charge_now"), 0), chargeFull: atLeast(attr(h, base, "charge_full"), 0),
		chargeDesign: atLeast(attr(h, base, "charge_full_design"), 0),
		powerNow:     magnitude(attr(h, base, "power_now")), currentNow: magnitude(attr(h, base, "current_now")),
	}
	// A battery always has a positive voltage; drivers write 0 when they have no reading.
	if v := attr(h, base, "voltage_now"); v != nil && *v > 0 {
		c.voltageNow = v
	}
	c.row = BatteryRow{Name: name, Status: readStatus(h, base)}
	// capacity is what the desktop shows, so it wins over a ratio computed here.
	if capacity := attr(h, base, "capacity"); capacity != nil && *capacity >= 0 && *capacity <= 100 {
		c.row.Pct = capacity
	} else if now, full := c.nowAndFull(); now != nil {
		c.row.Pct = percent(*now, *full)
	}
	switch {
	case c.powerNow != nil:
		c.watts = finite(*c.powerNow / 1e6)
	case c.currentNow != nil && c.voltageNow != nil:
		c.watts = finite(*c.currentNow * *c.voltageNow / 1e12)
	}
	c.row.PowerW = rounded(c.watts, 100)
	// Health is not clamped: a new cell's full charge can exceed its design capacity.
	if c.energyFull != nil && c.energyDesign != nil && *c.energyDesign > 0 {
		c.row.Health = rounded(finite(100**c.energyFull / *c.energyDesign), 10)
	} else if c.chargeFull != nil && c.chargeDesign != nil && *c.chargeDesign > 0 {
		c.row.Health = rounded(finite(100**c.chargeFull / *c.chargeDesign), 10)
	}
	return c
}

func readStatus(h Host, base string) *string {
	b, err := fs.ReadFile(h.FS, base+"/status")
	if err != nil {
		return nil
	}
	return text(string(b))
}

func (c cell) family() family {
	switch {
	case c.energyNow != nil:
		return energy
	case c.chargeNow != nil:
		return charge
	}
	return noFamily
}

// nowAndFull is the battery's present and full amounts in its own family, or nils.
func (c cell) nowAndFull() (*float64, *float64) {
	switch c.family() {
	case energy:
		if c.energyFull != nil {
			return c.energyNow, c.energyFull
		}
	case charge:
		if c.chargeFull != nil {
			return c.chargeNow, c.chargeFull
		}
	}
	return nil, nil
}

// rate is the battery's flow in its family's unit, µW or µA, from measurements of the same instant.
func (c cell) rate() *float64 {
	switch c.family() {
	case energy:
		if c.powerNow != nil {
			return c.powerNow
		}
		if c.currentNow != nil && c.voltageNow != nil {
			return finite(*c.currentNow * *c.voltageNow / 1e6)
		}
	case charge:
		if c.currentNow != nil {
			return c.currentNow
		}
		if c.powerNow != nil && c.voltageNow != nil {
			return finite(*c.powerNow * 1e6 / *c.voltageNow)
		}
	}
	return nil
}

// percent is nil when now exceeds full: the two readings disagree, and either could be the wrong one.
func percent(now, full float64) *float64 {
	if full <= 0 || now > full {
		return nil
	}
	return rounded(finite(100*now/full), 10)
}

func finite(v float64) *float64 {
	if math.IsNaN(v) || math.IsInf(v, 0) {
		return nil
	}
	return &v
}

func rounded(v *float64, scale float64) *float64 {
	if v == nil {
		return nil
	}
	return finite(math.Round(*v*scale) / scale)
}

func summarize(r *BatteryReading, cells []cell) {
	if len(cells) == 0 {
		return
	}
	r.Status = summaryStatus(cells)
	if len(cells) == 1 {
		r.Pct = cells[0].row.Pct
	} else {
		r.Pct = summaryPct(cells)
	}
	r.PowerW = summaryPower(cells)
	if r.Status != nil && *r.Status == "Discharging" {
		r.TimeLeft = timeLeft(cells)
	}
}

func flows(cells []cell) (discharging, charging bool) {
	for _, c := range cells {
		s := c.row.Status
		discharging = discharging || s != nil && *s == "Discharging"
		charging = charging || s != nil && *s == "Charging"
	}
	return discharging, charging
}

func summaryStatus(cells []cell) *string {
	discharging, charging := flows(cells)
	switch {
	case discharging && charging:
		return nil
	case discharging:
		return new("Discharging")
	case charging:
		return new("Charging")
	}
	first := cells[0].row.Status
	for _, c := range cells {
		if first == nil || c.row.Status == nil || *c.row.Status != *first {
			return nil
		}
	}
	return first
}

func sameFamily(cells []cell) family {
	f := cells[0].family()
	for _, c := range cells {
		if c.family() != f {
			return noFamily
		}
	}
	return f
}

func summaryPct(cells []cell) *float64 {
	if sameFamily(cells) == noFamily {
		return nil
	}
	var now, full float64
	for _, c := range cells {
		n, f := c.nowAndFull()
		if n == nil {
			return nil
		}
		now += *n
		full += *f
	}
	return percent(now, full)
}

// summaryPower is nil when one battery charges while another discharges: opposite flows don't add.
func summaryPower(cells []cell) *float64 {
	if discharging, charging := flows(cells); discharging && charging {
		return nil
	}
	var sum float64
	for _, c := range cells {
		if c.watts == nil {
			return nil
		}
		sum += *c.watts
	}
	return rounded(finite(sum), 100)
}

// timeLeft is Machine-level only: on a Machine with two batteries, the one discharging alone would report too few
// hours.
func timeLeft(cells []cell) *uint64 {
	if sameFamily(cells) == noFamily {
		return nil
	}
	var now, rate float64
	for _, c := range cells {
		r := c.rate()
		if r == nil {
			return nil
		}
		now += *c.familyNow()
		rate += *r
	}
	if rate <= 0 {
		return nil
	}
	s := math.Round(now / rate * 3600)
	// MaxInt64 rounds up to 2^63 as a float, so equality is already outside the phone's range.
	if math.IsNaN(s) || math.IsInf(s, 0) || s < 0 || s >= float64(math.MaxInt64) {
		return nil
	}
	n := uint64(s)
	return &n
}

func (c cell) familyNow() *float64 {
	if c.family() == energy {
		return c.energyNow
	}
	return c.chargeNow
}
