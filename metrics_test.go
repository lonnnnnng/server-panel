package main

import (
	"encoding/json"
	"math"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestCPUGuestAndIdleAccounting(t *testing.T) {
	// long: 虚拟机 guest 与 iowait 最容易被重复统计，必须分别校验总量和分项。
	stat := parseCPULine("cpu 50 0 0 50 0 0 0 0 25 0", -1)
	usage, _ := cpuDelta(stat, CPUStat{})
	if stat.Total != 100 || usage != 50 {
		t.Fatalf("guest counted twice: total=%v usage=%v", stat.Total, usage)
	}
	stat = parseCPULine("cpu 20 0 10 60 10 0 0 0 0 0", -1)
	usage, b := cpuDelta(stat, CPUStat{})
	if math.Abs(usage-30) > 1e-9 || b.Idle != 60 || b.IOWait != 10 || b.User+b.System+b.Idle+b.IOWait+b.Steal != 100 {
		t.Fatalf("idle includes iowait twice: %v %+v", usage, b)
	}
}

func TestTemperatureMilliUnitsAndInvalidValues(t *testing.T) {
	for text, want := range map[string]float64{"-1000": -1, "500": 0.5, "1000": 1, "51000": 51} {
		if value, valid := parseMilliCelsius(text); !valid || value != want {
			t.Fatalf("temperature %s must be %v, got %v valid=%v", text, want, value, valid)
		}
	}
	for _, text := range []string{"", "NaN", "+Inf", "151000", "-51000"} {
		if _, valid := parseMilliCelsius(text); valid {
			t.Fatalf("invalid temperature accepted: %s", text)
		}
	}
}

func TestMemoryAvailability(t *testing.T) {
	for _, tc := range []struct {
		name, input      string
		available        uint64
		estimated, valid bool
	}{
		{"kernel", "MemTotal: 1000 kB\nMemAvailable: 200 kB\nCached: 600 kB\nBuffers: 100 kB", 200 * 1024, false, true},
		{"fallback", "MemTotal: 1000 kB\nMemFree: 100 kB\nCached: 500 kB\nBuffers: 100 kB\nSReclaimable: 50 kB", 750 * 1024, true, true},
		{"clamp", "MemTotal: 1000 kB\nMemAvailable: 2000 kB", 1000 * 1024, false, true},
		{"missing", "", 0, true, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			m := parseMemory(tc.input)
			if m.Available != tc.available || m.Estimated != tc.estimated || m.Valid != tc.valid || m.Used > m.Total {
				t.Fatalf("invalid memory accounting: %+v", m)
			}
		})
	}
}

func TestHistoryIncludesCurrentFrameAndUnknown(t *testing.T) {
	s := &AppState{maxHistory: 2}
	m := &Metrics{Timestamp: "first", CPU: CPUMetrics{Valid: true, Percent: 50, LoadValid: true, LoadPercent: 175}}
	s.appendHistory(m)
	if len(m.History) != 1 || m.History[0].Timestamp != m.Timestamp || *m.History[0].CPU != 50 || *m.History[0].Load != 175 || m.History[0].Memory != nil {
		t.Fatalf("history delayed or missing converted to zero: %+v", m.History)
	}
	for _, ts := range []string{"second", "third"} {
		s.appendHistory(&Metrics{Timestamp: ts})
	}
	if len(s.history) != 2 || s.history[0].Timestamp != "second" || m.History[0].Timestamp != "first" {
		t.Fatal("history window corrupted old snapshots")
	}
	out, err := json.Marshal(m.History)
	if err != nil || !strings.Contains(string(out), `"memory":null`) {
		t.Fatal("unknown history must serialize as null")
	}
}

func TestNetworkPhysicalSelectionAndHotplug(t *testing.T) {
	old := &NetworkRaw{Time: time.Unix(10, 0), Interfaces: map[string]NetInterface{
		"eno1": {Name: "eno1", Kind: "physical", RXBytes: 100, TXBytes: 200},
		"Meta": {Name: "Meta", Kind: "virtual-active", RXBytes: 100, TXBytes: 200},
	}}
	current := &NetworkRaw{Time: time.Unix(11, 0), Interfaces: map[string]NetInterface{
		"eno1": {Name: "eno1", Kind: "physical", RXBytes: 300, TXBytes: 250},
		"eno2": {Name: "eno2", Kind: "physical", RXBytes: 999999, TXBytes: 999999},
		"Meta": {Name: "Meta", Kind: "virtual-active", RXBytes: 300, TXBytes: 250},
	}}
	m := calculateNetwork(current, old)
	if m.Scope != "physical" || len(m.SelectedInterfaces) != 2 || len(m.Interfaces) != 3 || m.RXRate != 200 || m.TXRate != 50 || m.RXBytes != 1000299 {
		t.Fatalf("network duplicate or hotplug spike: %+v", m)
	}
	delete(current.Interfaces, "eno2")
	current.Interfaces["eno1"] = NetInterface{Name: "eno1", Kind: "physical", RXBytes: 1, TXBytes: 2}
	m = calculateNetwork(current, old)
	if m.RateValid || m.RXRate != 0 {
		t.Fatalf("reset must wait for fresh baseline: %+v", m)
	}
	delete(current.Interfaces, "eno1")
	m = calculateNetwork(current, nil)
	if m.Scope != "virtual-fallback" || m.RateValid {
		t.Fatal("virtual fallback or first sample not explicit")
	}
}

func TestDiskScopeBusyAndHotplug(t *testing.T) {
	old := &DiskRaw{Time: time.Unix(1, 0), Devices: map[string]DiskDevice{
		"sda":  {Name: "sda", ReadBytes: 100, IOMs: 10},
		"dm-0": {Name: "dm-0", ReadBytes: 100, IOMs: 10},
	}}
	current := &DiskRaw{Time: time.Unix(2, 0), Devices: map[string]DiskDevice{
		"sda":  {Name: "sda", ReadBytes: 300, IOMs: 110},
		"sdb":  {Name: "sdb", ReadBytes: 999999, IOMs: 999999},
		"dm-0": {Name: "dm-0", ReadBytes: 300, IOMs: 910},
	}}
	m := calculateDiskIO(current, old)
	if m.ReadRate != 200 || m.BusyPercent != 10 || !m.RateValid || m.Scope != "whole-system-physical" || len(m.Devices) != 3 {
		t.Fatalf("disk layer or hotplug counted twice: %+v", m)
	}
	if filesystemPercent(41, 54) != 44 || filesystemPercent(0, 0) != 0 {
		t.Fatal("filesystem percent must use df denominator and rounding")
	}
}

func TestProcessIdentityAndClockTicks(t *testing.T) {
	fields := strings.Fields("S 1 2 3 4 5 6 7 8 9 10 50 25 13 14 15 16 17 18 12345")
	sample := parseProcSample("123 (name with ) spaces) " + strings.Join(fields, " "))
	if !sample.Valid || sample.Ticks != 75 || sample.StartTime != 12345 {
		t.Fatalf("incorrect proc stat offsets: %+v", sample)
	}
	previous := ProcSample{Ticks: 25, StartTime: 12345, Valid: true}
	value, ok := processCPU(sample, previous, 100, 0.25)
	if !ok || value != 200 {
		t.Fatalf("multicore CPU must allow >100: %v %v", value, ok)
	}
	previous.StartTime++
	if _, ok := processCPU(sample, previous, 100, 1); ok {
		t.Fatal("reused PID must not inherit old counter")
	}
	previous.StartTime--
	if _, ok := processCPU(sample, previous, 0, 1); ok || parseProcSample("bad").Valid {
		t.Fatal("unknown clock or unreadable stat must remain invalid")
	}
}

func TestUDPSocketOnlyUnconnected(t *testing.T) {
	path := filepath.Join(t.TempDir(), "udp")
	fixture := "header\n 0: 0100007F:0035 00000000:0000 07 0 0 0 1000 0 123\n 1: 0100007F:1111 0100007F:0035 01 0 0 0 1000 0 124\n"
	if err := os.WriteFile(path, []byte(fixture), 0600); err != nil {
		t.Fatal(err)
	}
	entries := readSocketEntries(path, "udp", false, nil)
	if len(entries) != 1 || entries[0].Port != 53 || entries[0].Address != "127.0.0.1" {
		t.Fatalf("connected UDP incorrectly listed: %+v", entries)
	}
}

func TestPCISysfsFollowsDeviceSymlinks(t *testing.T) {
	root := t.TempDir()
	device := filepath.Join(root, "target")
	if err := os.Mkdir(device, 0700); err != nil {
		t.Fatal(err)
	}
	for name, value := range map[string]string{"vendor": "0x8086", "device": "0x1234", "class": "0x030000"} {
		if err := os.WriteFile(filepath.Join(device, name), []byte(value), 0600); err != nil {
			t.Fatal(err)
		}
	}
	links := filepath.Join(root, "devices")
	if err := os.Mkdir(links, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(device, filepath.Join(links, "0000:00:01.0")); err != nil {
		t.Fatal(err)
	}
	devices := readPCIDevicesAt(links)
	if len(devices) != 1 || devices[0].Vendor != "0x8086" {
		t.Fatalf("sysfs symlink omitted: %+v", devices)
	}
}

func TestHardwareCachedDynamicValuesDoNotMutateSnapshot(t *testing.T) {
	s := &AppState{hardwareAt: time.Now(), hardware: HardwareMetrics{CPU: CPUHardware{SpeedMHz: 1000}, Memory: MemoryHardware{Available: 500}, Network: []NetworkHardware{{Name: "nonexistent-test", State: "up"}}}}
	m := s.readHardware(HostMetrics{}, CPUMetrics{SpeedMHz: 2000}, MemoryMetrics{Available: 100}, DiskMetrics{}, NetworkMetrics{})
	if m.CPU.SpeedMHz != 2000 || m.Memory.Available != 100 || s.hardware.Network[0].State != "up" || s.hardware.CPU.SpeedMHz != 1000 {
		t.Fatal("dynamic hardware stale or old snapshot mutated")
	}
}
