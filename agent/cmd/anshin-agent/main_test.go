package main

import (
	"strings"
	"testing"
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
