package probe

import (
	"io/fs"
	"math"
	"path"
	"strconv"
	"strings"
)

type MemoryReading struct {
	Total     *uint64  `json:"mem_total_bytes"`
	Available *uint64  `json:"mem_available_bytes"`
	Used      *uint64  `json:"mem_used_bytes"`
	Pct       *float64 `json:"mem_used_pct"`
}

type SwapEntry struct {
	Name  string  `json:"name"`
	Zram  bool    `json:"zram"`
	Total *uint64 `json:"total_bytes"`
	Used  *uint64 `json:"used_bytes"`
}

type SwapReading struct {
	Total   *uint64     `json:"swap_total_bytes"`
	Used    *uint64     `json:"swap_used_bytes"`
	Entries []SwapEntry `json:"swap_entries"`
}

type LoadReading struct {
	One     *float64 `json:"load_1"`
	Five    *float64 `json:"load_5"`
	Fifteen *float64 `json:"load_15"`
	Uptime  *float64 `json:"uptime_s"`
}

type MachineInfo struct {
	Distro       *string `json:"distro"`
	Kernel       *string `json:"kernel"`
	Architecture *string `json:"arch"`
	AgentVersion *string `json:"agent_version"`
}

func kib(s string) *uint64 {
	n, err := strconv.ParseUint(s, 10, 64)
	if err != nil || n > math.MaxInt64/1024 {
		return nil
	}
	n *= 1024
	return &n
}

func Memory(h Host) MemoryReading {
	var m MemoryReading
	b, err := fs.ReadFile(h.FS, "proc/meminfo")
	if err != nil {
		return m
	}
	for line := range strings.SplitSeq(string(b), "\n") {
		f := strings.Fields(line)
		if len(f) != 3 || f[2] != "kB" {
			continue
		}
		switch f[0] {
		case "MemTotal:":
			m.Total = kib(f[1])
		case "MemAvailable:":
			m.Available = kib(f[1])
		}
	}
	if m.Total != nil && m.Available != nil && *m.Available <= *m.Total {
		used := *m.Total - *m.Available
		m.Used = &used
		if *m.Total > 0 {
			pct := math.Round(1000*float64(used)/float64(*m.Total)) / 10
			m.Pct = &pct
		}
	}
	return m
}

func Swap(h Host) SwapReading {
	var s SwapReading
	b, err := fs.ReadFile(h.FS, "proc/swaps")
	if err != nil {
		return s
	}
	lines := strings.Split(strings.TrimSpace(string(b)), "\n")
	if len(lines) == 0 || strings.Join(strings.Fields(lines[0]), " ") != "Filename Type Size Used Priority" {
		return s
	}
	var total, used uint64
	totalOK, usedOK := true, true
	for _, line := range lines[1:] {
		f := strings.Fields(line)
		if len(f) == 0 {
			continue
		}
		if len(f) != 5 {
			totalOK, usedOK = false, false
			continue
		}
		e := SwapEntry{Name: f[0], Zram: isZram(f[0]), Total: kib(f[2]), Used: kib(f[3])}
		if e.Total != nil && e.Used != nil && *e.Used > *e.Total {
			e.Used = nil
		}
		if e.Total == nil || math.MaxInt64-total < *e.Total {
			totalOK = false
		} else {
			total += *e.Total
		}
		if e.Used == nil || math.MaxInt64-used < *e.Used {
			usedOK = false
		} else {
			used += *e.Used
		}
		s.Entries = append(s.Entries, e)
	}
	if len(s.Entries) > 0 {
		if totalOK {
			s.Total = &total
		}
		if usedOK {
			s.Used = &used
		}
	}
	return s
}

func isZram(name string) bool {
	if path.Dir(name) != "/dev" {
		return false
	}
	suffix, ok := strings.CutPrefix(path.Base(name), "zram")
	if !ok || suffix == "" {
		return false
	}
	for _, c := range suffix {
		if c < '0' || c > '9' {
			return false
		}
	}
	return true
}

func nonnegative(s string) *float64 {
	n, err := strconv.ParseFloat(s, 64)
	if err != nil || math.IsNaN(n) || math.IsInf(n, 0) || n < 0 {
		return nil
	}
	return &n
}

func Load(h Host) LoadReading {
	var l LoadReading
	if b, err := fs.ReadFile(h.FS, "proc/loadavg"); err == nil {
		f := strings.Fields(string(b))
		values := []**float64{&l.One, &l.Five, &l.Fifteen}
		for i, dst := range values {
			if i < len(f) {
				*dst = nonnegative(f[i])
			}
		}
	}
	if b, err := fs.ReadFile(h.FS, "proc/uptime"); err == nil {
		if f := strings.Fields(string(b)); len(f) > 0 {
			l.Uptime = nonnegative(f[0])
		}
	}
	return l
}

func text(s string) *string {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil
	}
	return &s
}

func Info(h Host) MachineInfo {
	m := MachineInfo{Architecture: h.Architecture, AgentVersion: h.AgentVersion}
	if b, err := fs.ReadFile(h.FS, "proc/sys/kernel/osrelease"); err == nil {
		m.Kernel = text(string(b))
	}
	if b, err := fs.ReadFile(h.FS, "etc/os-release"); err == nil {
		values := map[string]*string{}
		for line := range strings.SplitSeq(string(b), "\n") {
			k, v, ok := strings.Cut(strings.TrimSpace(line), "=")
			if !ok || (k != "PRETTY_NAME" && k != "NAME") {
				continue
			}
			if strings.HasPrefix(v, "\"") {
				// os-release allows shell quoting, but never shell expansion or execution.
				if len(v) < 2 || !strings.HasSuffix(v, "\"") {
					continue
				}
				v = strings.NewReplacer(`\$`, `$`, "\\`", "`", `\"`, `"`, `\\`, `\`).Replace(v[1 : len(v)-1])
			} else if strings.HasPrefix(v, "'") {
				if len(v) < 2 || !strings.HasSuffix(v, "'") {
					continue
				}
				v = v[1 : len(v)-1]
			} else if strings.ContainsAny(v, " \t\"'") {
				continue
			}
			values[k] = text(v)
		}
		m.Distro = values["PRETTY_NAME"]
		if m.Distro == nil {
			m.Distro = values["NAME"]
		}
	}
	return m
}
