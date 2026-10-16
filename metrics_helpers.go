package main

import (
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
)

func validValue(value float64, valid bool) *float64 {
	if !valid || math.IsNaN(value) || math.IsInf(value, 0) {
		return nil
	}
	return &value
}

func (s *AppState) appendHistory(m *Metrics) {
	// long: 趋势与卡片必须包含同一帧；只保存摘要，避免历史快照嵌套并反复保留进程命令。
	s.history = append(s.history, HistoryPoint{
		Timestamp: m.Timestamp,
		CPU:       validValue(m.CPU.Percent, m.CPU.Valid),
		Memory:    validValue(m.Memory.Percent, m.Memory.Valid),
		Disk:      validValue(m.Disk.Percent, m.Disk.Valid),
		RXRate:    validValue(m.Network.RXRate, m.Network.Valid && m.Network.RateValid),
		TXRate:    validValue(m.Network.TXRate, m.Network.Valid && m.Network.RateValid),
		ReadRate:  validValue(m.Disk.IO.ReadRate, m.Disk.IO.Valid && m.Disk.IO.RateValid),
		WriteRate: validValue(m.Disk.IO.WriteRate, m.Disk.IO.Valid && m.Disk.IO.RateValid),
		Load:      validValue(m.CPU.LoadPercent, m.CPU.LoadValid),
	})
	limit := max(s.maxHistory, 1)
	if len(s.history) > limit {
		s.history = append([]HistoryPoint(nil), s.history[len(s.history)-limit:]...)
	}
	m.History = append([]HistoryPoint(nil), s.history...)
}

func filesystemPercent(used, available uint64) float64 {
	if used == 0 && available == 0 {
		return 0
	}
	return math.Ceil(float64(used) / (float64(used) + float64(available)) * 100)
}

func calculateDiskIO(current, previous *DiskRaw) DiskIO {
	selected := selectAggregateDiskDevices(current.Devices)
	result := DiskIO{Scope: "whole-system-physical"}
	selectedNames := map[string]bool{}
	for _, device := range selected {
		selectedNames[device.Name] = true
		result.Reads += device.Reads
		result.Writes += device.Writes
		result.ReadBytes += device.ReadBytes
		result.WriteBytes += device.WriteBytes
		result.IOMs += device.IOMs
		if !isPhysicalDiskDevice(device.Name) {
			result.Scope = "whole-system-fallback"
		}
	}
	for name, device := range current.Devices {
		if previous != nil && current.Time.After(previous.Time) {
			if old, ok := previous.Devices[name]; ok && device.ReadBytes >= old.ReadBytes && device.WriteBytes >= old.WriteBytes && device.IOMs >= old.IOMs {
				seconds := current.Time.Sub(previous.Time).Seconds()
				device.ReadRate = float64(delta(device.ReadBytes, old.ReadBytes)) / seconds
				device.WriteRate = float64(delta(device.WriteBytes, old.WriteBytes)) / seconds
				device.ReadOps = float64(delta(device.Reads, old.Reads)) / seconds
				device.WriteOps = float64(delta(device.Writes, old.Writes)) / seconds
				device.BusyPercent = clamp(float64(delta(device.IOMs, old.IOMs)) / (seconds * 1000) * 100)
				if selectedNames[name] {
					// long: 先逐设备差分再汇总，热插拔不能把设备的历史累计量算成本帧吞吐。
					result.RateValid = true
					result.ReadRate += device.ReadRate
					result.WriteRate += device.WriteRate
					result.ReadOps += device.ReadOps
					result.WriteOps += device.WriteOps
					result.BusyPercent = math.Max(result.BusyPercent, device.BusyPercent)
				}
			}
		}
		result.Devices = append(result.Devices, device)
	}
	return result
}

func calculateNetwork(current, previous *NetworkRaw) NetworkMetrics {
	result := NetworkMetrics{Scope: "physical", SelectedInterfaces: []string{}}
	hasPhysical := false
	for _, item := range current.Interfaces {
		hasPhysical = hasPhysical || item.Kind == "physical"
	}
	if !hasPhysical {
		result.Scope = "virtual-fallback"
	}
	for name, item := range current.Interfaces {
		if previous != nil && current.Time.After(previous.Time) {
			if old, ok := previous.Interfaces[name]; ok && old.Kind == item.Kind && item.RXBytes >= old.RXBytes && item.TXBytes >= old.TXBytes {
				seconds := current.Time.Sub(previous.Time).Seconds()
				item.RateValid = true
				item.RXRate = float64(delta(item.RXBytes, old.RXBytes)) / seconds
				item.TXRate = float64(delta(item.TXBytes, old.TXBytes)) / seconds
				item.RXPacketRate = float64(delta(item.RXPackets, old.RXPackets)) / seconds
				item.TXPacketRate = float64(delta(item.TXPackets, old.TXPackets)) / seconds
			}
		}
		result.Interfaces = append(result.Interfaces, item)
		// long: 物理网口、TUN 与 bridge 是不同层，同一流量不能跨层重复求和。
		if hasPhysical && item.Kind != "physical" {
			continue
		}
		result.SelectedInterfaces = append(result.SelectedInterfaces, name)
		result.RXBytes += item.RXBytes
		result.TXBytes += item.TXBytes
		result.RXPackets += item.RXPackets
		result.TXPackets += item.TXPackets
		result.RXErrors += item.RXErrors
		result.TXErrors += item.TXErrors
		result.RXDrops += item.RXDrops
		result.TXDrops += item.TXDrops
		result.RXRate += item.RXRate
		result.TXRate += item.TXRate
		result.RXPacketRate += item.RXPacketRate
		result.TXPacketRate += item.TXPacketRate
		result.RateValid = result.RateValid || item.RateValid
	}
	sort.Strings(result.SelectedInterfaces)
	result.Errors = result.RXErrors + result.TXErrors
	result.Drops = result.RXDrops + result.TXDrops
	return result
}

func interfaceIsUp(name string) bool {
	flags, err := strconv.ParseUint(strings.TrimSpace(readText("/sys/class/net/"+name+"/flags")), 0, 64)
	return err == nil && flags&1 != 0
}

type ProcSample struct {
	Ticks     uint64
	StartTime uint64
	Valid     bool
}

func parseProcSample(stat string) ProcSample {
	end := strings.LastIndex(stat, ") ")
	if end < 0 {
		return ProcSample{}
	}
	fields := strings.Fields(stat[end+2:])
	if len(fields) < 20 {
		return ProcSample{}
	}
	user, e1 := strconv.ParseUint(fields[11], 10, 64)
	system, e2 := strconv.ParseUint(fields[12], 10, 64)
	start, e3 := strconv.ParseUint(fields[19], 10, 64)
	return ProcSample{Ticks: user + system, StartTime: start, Valid: e1 == nil && e2 == nil && e3 == nil}
}

func processCPU(current, previous ProcSample, hz, seconds float64) (float64, bool) {
	// long: PID 可被复用；只有启动时刻相同的进程才能沿用上帧 CPU 计数。
	if !current.Valid || !previous.Valid || current.StartTime != previous.StartTime || current.Ticks < previous.Ticks || hz <= 0 || seconds <= 0 {
		return 0, false
	}
	return float64(current.Ticks-previous.Ticks) / hz / seconds * 100, true
}

var tickOnce sync.Once
var tickHz float64

func clockTicks() float64 {
	tickOnce.Do(func() {
		// long: USER_HZ 由运行系统决定，检测失败时保留未知，不能假设所有 Linux 都为 100。
		out, err := exec.Command("getconf", "CLK_TCK").Output()
		if err == nil {
			tickHz, _ = strconv.ParseFloat(strings.TrimSpace(string(out)), 64)
		}
	})
	return tickHz
}

func readCPUHwmonTemperatures() []TempZone {
	var zones []TempZone
	entries, _ := os.ReadDir("/sys/class/hwmon")
	for _, entry := range entries {
		base := filepath.Join("/sys/class/hwmon", entry.Name())
		kind := strings.TrimSpace(readText(filepath.Join(base, "name")))
		if !isCPUTemperatureType(kind) {
			continue
		}
		inputs, _ := filepath.Glob(filepath.Join(base, "temp*_input"))
		for _, input := range inputs {
			celsius, valid := parseMilliCelsius(readText(input))
			if valid {
				zones = append(zones, TempZone{Name: entry.Name() + "/" + filepath.Base(input), Type: kind, Celsius: celsius})
			}
		}
	}
	return zones
}

func parseMilliCelsius(text string) (float64, bool) {
	// long: sysfs 固定使用毫摄氏度，负温和低于 1°C 也必须换算；拒绝非数值以免破坏整帧 JSON。
	raw, err := strconv.ParseFloat(strings.TrimSpace(text), 64)
	celsius := raw / 1000
	return celsius, err == nil && !math.IsNaN(celsius) && !math.IsInf(celsius, 0) && celsius >= -50 && celsius <= 150
}
