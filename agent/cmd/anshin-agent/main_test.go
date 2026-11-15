package main

import (
	"strings"
	"testing"
)

func TestCPUSensorConfiguration(t *testing.T) {
	for _, s := range []string{"", "k10temp/Tctl", "coretemp/Package id 0", "it87/temp3"} {
		if got, err := cpuSensor(env(map[string]string{"ANSHIN_CPU_SENSOR": s})); err != nil || got != s {
			t.Errorf("ANSHIN_CPU_SENSOR=%q: got %q, %v; want it verbatim", s, got, err)
		}
	}
	for _, s := range []string{"k10temp", "/Tctl", "k10temp/", " k10temp/Tctl", "k10temp/Tctl ", "k10 temp/Tctl", "k10temp/Tctl\n"} {
		if _, err := cpuSensor(env(map[string]string{"ANSHIN_CPU_SENSOR": s})); err == nil || !strings.Contains(err.Error(), "ANSHIN_CPU_SENSOR") {
			t.Errorf("ANSHIN_CPU_SENSOR=%q: err = %v, want a configuration error naming it", s, err)
		}
	}
}

func env(vars map[string]string) func(string) string {
	return func(k string) string { return vars[k] }
}
