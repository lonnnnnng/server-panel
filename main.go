package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"math"
	"net"
	"net/http"
	"os"
	"os/exec"
	"os/user"
	"path/filepath"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

const (
	defaultPort       = "8888"
	defaultHost       = "0.0.0.0"
	defaultIntervalMs = 500
	defaultHistory    = 120
)

var (
	userLookupMu    sync.Mutex
	userLookupCache = map[string]string{}
)

type CPUStat struct {
	Index     int
	User      float64
	Nice      float64
	System    float64
	Idle      float64
	IOWait    float64
	IRQ       float64
	SoftIRQ   float64
	Steal     float64
	IdleAll   float64
	SystemAll float64
	Total     float64
}

type CPUSnapshot struct {
	Aggregate CPUStat
	Cores     []CPUStat
}

type NetworkRaw struct {
	Time       time.Time
	RXBytes    uint64
	TXBytes    uint64
	RXPackets  uint64
	TXPackets  uint64
	Interfaces map[string]NetInterface
}

type DiskRaw struct {
	Time       time.Time
	ReadBytes  uint64
	WriteBytes uint64
	Reads      uint64
	Writes     uint64
	IOMs       uint64
	Devices    map[string]DiskDevice
}

type ProcRaw struct {
	Time      time.Time
	Processes map[string]ProcSample
}

type AppState struct {
	mu          sync.Mutex
	latest      *Metrics
	history     []HistoryPoint
	maxHistory  int
	prevCPU     *CPUSnapshot
	prevNetwork *NetworkRaw
	prevDisk    *DiskRaw
	prevProc    *ProcRaw
	hardware    HardwareMetrics
	hardwareAt  time.Time
}

type Metrics struct {
	Timestamp   string          `json:"timestamp"`
	Host        HostMetrics     `json:"host"`
	Hardware    HardwareMetrics `json:"hardware"`
	CPU         CPUMetrics      `json:"cpu"`
	Memory      MemoryMetrics   `json:"memory"`
	Disk        DiskMetrics     `json:"disk"`
	Network     NetworkMetrics  `json:"network"`
	Processes   ProcMetrics     `json:"processes"`
	TCP         TCPMetrics      `json:"tcp"`
	Pressure    PressureSet     `json:"pressure"`
	Temperature TempMetrics     `json:"temperature"`
	FileHandles FileHandles     `json:"fileHandles"`
	History     []HistoryPoint  `json:"history"`
}

type HostMetrics struct {
	Hostname          string  `json:"hostname"`
	Platform          string  `json:"platform"`
	OSName            string  `json:"osName"`
	OSID              string  `json:"osId"`
	OSVersionID       string  `json:"osVersionId"`
	OSVersionCodename string  `json:"osVersionCodename"`
	Release           string  `json:"release"`
	KernelVersion     string  `json:"kernelVersion"`
	Arch              string  `json:"arch"`
	Machine           string  `json:"machine"`
	GoVersion         string  `json:"goVersion"`
	Uptime            float64 `json:"uptime"`
	BootTime          string  `json:"bootTime"`
	LocalTime         string  `json:"localTime"`
	Timezone          string  `json:"timezone"`
	User              string  `json:"user"`
	UID               string  `json:"uid"`
	GID               string  `json:"gid"`
	HomeDir           string  `json:"homeDir"`
	Shell             string  `json:"shell"`
	Virtualization    string  `json:"virtualization"`
	Container         string  `json:"container"`
	ProductVendor     string  `json:"productVendor"`
	ProductName       string  `json:"productName"`
	ProductVersion    string  `json:"productVersion"`
	ProductFamily     string  `json:"productFamily"`
	BoardVendor       string  `json:"boardVendor"`
	BoardName         string  `json:"boardName"`
	BoardVersion      string  `json:"boardVersion"`
	BIOSVendor        string  `json:"biosVendor"`
	BIOSVersion       string  `json:"biosVersion"`
	BIOSDate          string  `json:"biosDate"`
	ChassisVendor     string  `json:"chassisVendor"`
	ChassisType       string  `json:"chassisType"`
	ChassisVersion    string  `json:"chassisVersion"`
}

type HardwareMetrics struct {
	Motherboard MotherboardHardware `json:"motherboard"`
	CPU         CPUHardware         `json:"cpu"`
	Memory      MemoryHardware      `json:"memory"`
	Storage     []StorageHardware   `json:"storage"`
	GPUs        []PCIHardwareDevice `json:"gpus"`
	Network     []NetworkHardware   `json:"network"`
}

type MotherboardHardware struct {
	SystemVendor   string `json:"systemVendor"`
	SystemModel    string `json:"systemModel"`
	SystemVersion  string `json:"systemVersion"`
	ProductFamily  string `json:"productFamily"`
	BoardVendor    string `json:"boardVendor"`
	BoardName      string `json:"boardName"`
	BoardVersion   string `json:"boardVersion"`
	BIOSVendor     string `json:"biosVendor"`
	BIOSVersion    string `json:"biosVersion"`
	BIOSDate       string `json:"biosDate"`
	ChassisVendor  string `json:"chassisVendor"`
	ChassisType    string `json:"chassisType"`
	ChassisVersion string `json:"chassisVersion"`
}

type CPUHardware struct {
	Model          string   `json:"model"`
	Vendor         string   `json:"vendor"`
	Architecture   string   `json:"architecture"`
	LogicalCores   int      `json:"logicalCores"`
	PhysicalCores  int      `json:"physicalCores"`
	Sockets        int      `json:"sockets"`
	CoresPerSocket int      `json:"coresPerSocket"`
	SpeedMHz       int      `json:"speedMHz"`
	MaxMHz         int      `json:"maxMHz"`
	MinMHz         int      `json:"minMHz"`
	CacheSize      string   `json:"cacheSize"`
	BogoMIPS       string   `json:"bogomips"`
	Microcode      string   `json:"microcode"`
	Flags          []string `json:"flags"`
}

type MemoryHardware struct {
	Total          uint64 `json:"total"`
	Available      uint64 `json:"available"`
	SwapTotal      uint64 `json:"swapTotal"`
	SwapFree       uint64 `json:"swapFree"`
	PageSize       uint64 `json:"pageSize"`
	HugePageSize   uint64 `json:"hugePageSize"`
	HugePagesTotal uint64 `json:"hugePagesTotal"`
	HugePagesFree  uint64 `json:"hugePagesFree"`
	NUMANodes      int    `json:"numaNodes"`
}

type StorageHardware struct {
	Name        string   `json:"name"`
	Type        string   `json:"type"`
	Size        uint64   `json:"size"`
	Model       string   `json:"model"`
	Transport   string   `json:"transport"`
	Medium      string   `json:"medium"`
	Rotational  bool     `json:"rotational"`
	Removable   bool     `json:"removable"`
	Partitions  int      `json:"partitions"`
	Mountpoints []string `json:"mountpoints"`
	Filesystems []string `json:"filesystems"`
}

type PCIHardwareDevice struct {
	Slot            string `json:"slot"`
	Class           string `json:"class"`
	Vendor          string `json:"vendor"`
	Device          string `json:"device"`
	SubsystemVendor string `json:"subsystemVendor"`
	SubsystemDevice string `json:"subsystemDevice"`
}

type NetworkHardware struct {
	Name      string `json:"name"`
	Kind      string `json:"kind"`
	State     string `json:"state"`
	SpeedMbps int    `json:"speedMbps"`
	Duplex    string `json:"duplex"`
	MTU       int    `json:"mtu"`
	Driver    string `json:"driver"`
	Bus       string `json:"bus"`
	Vendor    string `json:"vendor"`
	Device    string `json:"device"`
	PCIID     string `json:"pciId"`
}

type CPUMetrics struct {
	Valid       bool         `json:"valid"`
	LoadValid   bool         `json:"loadValid"`
	Percent     float64      `json:"percent"`
	Cores       int          `json:"cores"`
	Load        []float64    `json:"load"`
	Model       string       `json:"model"`
	SpeedMHz    int          `json:"speedMHz"`
	Breakdown   CPUBreakdown `json:"breakdown"`
	PerCore     []CoreUsage  `json:"perCore"`
	LoadPercent float64      `json:"loadPercent"`
}

type CPUBreakdown struct {
	User   float64 `json:"user"`
	System float64 `json:"system"`
	IOWait float64 `json:"iowait"`
	Steal  float64 `json:"steal"`
	Idle   float64 `json:"idle"`
}

type CoreUsage struct {
	Index   int     `json:"index"`
	Percent float64 `json:"percent"`
}

type MemoryMetrics struct {
	Valid        bool    `json:"valid"`
	Estimated    bool    `json:"estimated"`
	Total        uint64  `json:"total"`
	Available    uint64  `json:"available"`
	Free         uint64  `json:"free"`
	Cached       uint64  `json:"cached"`
	Buffers      uint64  `json:"buffers"`
	Slab         uint64  `json:"slab"`
	SReclaimable uint64  `json:"sReclaimable"`
	Used         uint64  `json:"used"`
	Percent      float64 `json:"percent"`
	SwapTotal    uint64  `json:"swapTotal"`
	SwapUsed     uint64  `json:"swapUsed"`
	SwapPercent  float64 `json:"swapPercent"`
	Active       uint64  `json:"active"`
	Inactive     uint64  `json:"inactive"`
	Dirty        uint64  `json:"dirty"`
	Writeback    uint64  `json:"writeback"`
	AnonPages    uint64  `json:"anonPages"`
	Mapped       uint64  `json:"mapped"`
	Shmem        uint64  `json:"shmem"`
	PageTables   uint64  `json:"pageTables"`
	CommitLimit  uint64  `json:"commitLimit"`
	CommittedAS  uint64  `json:"committedAs"`
}

type DiskMetrics struct {
	Valid       bool         `json:"valid"`
	Filesystem  string       `json:"filesystem"`
	Type        string       `json:"type"`
	Mount       string       `json:"mount"`
	Total       uint64       `json:"total"`
	Used        uint64       `json:"used"`
	Available   uint64       `json:"available"`
	Percent     float64      `json:"percent"`
	Filesystems []Filesystem `json:"filesystems"`
	IO          DiskIO       `json:"io"`
}

type Filesystem struct {
	Filesystem string  `json:"filesystem"`
	Type       string  `json:"type"`
	Mount      string  `json:"mount"`
	Total      uint64  `json:"total"`
	Used       uint64  `json:"used"`
	Available  uint64  `json:"available"`
	Percent    float64 `json:"percent"`
}

type DiskIO struct {
	Valid       bool         `json:"valid"`
	RateValid   bool         `json:"rateValid"`
	Scope       string       `json:"scope"`
	Reads       uint64       `json:"reads"`
	Writes      uint64       `json:"writes"`
	ReadBytes   uint64       `json:"readBytes"`
	WriteBytes  uint64       `json:"writeBytes"`
	IOMs        uint64       `json:"ioMs"`
	ReadRate    float64      `json:"readRate"`
	WriteRate   float64      `json:"writeRate"`
	ReadOps     float64      `json:"readOps"`
	WriteOps    float64      `json:"writeOps"`
	BusyPercent float64      `json:"busyPercent"`
	Devices     []DiskDevice `json:"devices"`
}

type DiskDevice struct {
	Name        string  `json:"name"`
	Reads       uint64  `json:"reads"`
	Writes      uint64  `json:"writes"`
	ReadBytes   uint64  `json:"readBytes"`
	WriteBytes  uint64  `json:"writeBytes"`
	IOMs        uint64  `json:"ioMs"`
	ReadRate    float64 `json:"readRate"`
	WriteRate   float64 `json:"writeRate"`
	ReadOps     float64 `json:"readOps"`
	WriteOps    float64 `json:"writeOps"`
	BusyPercent float64 `json:"busyPercent"`
}

type NetworkMetrics struct {
	Valid              bool           `json:"valid"`
	RateValid          bool           `json:"rateValid"`
	Scope              string         `json:"scope"`
	SelectedInterfaces []string       `json:"selectedInterfaces"`
	RXBytes            uint64         `json:"rxBytes"`
	TXBytes            uint64         `json:"txBytes"`
	RXPackets          uint64         `json:"rxPackets"`
	TXPackets          uint64         `json:"txPackets"`
	RXErrors           uint64         `json:"rxErrors"`
	RXDrops            uint64         `json:"rxDrops"`
	TXErrors           uint64         `json:"txErrors"`
	TXDrops            uint64         `json:"txDrops"`
	RXRate             float64        `json:"rxRate"`
	TXRate             float64        `json:"txRate"`
	RXPacketRate       float64        `json:"rxPacketRate"`
	TXPacketRate       float64        `json:"txPacketRate"`
	Errors             uint64         `json:"errors"`
	Drops              uint64         `json:"drops"`
	Addresses          []Address      `json:"addresses"`
	Interfaces         []NetInterface `json:"interfaces"`
}

type Address struct {
	Name    string `json:"name"`
	Address string `json:"address"`
}

type NetInterface struct {
	RateValid    bool    `json:"rateValid"`
	Name         string  `json:"name"`
	Kind         string  `json:"kind"`
	State        string  `json:"state"`
	SpeedMbps    int     `json:"speedMbps"`
	Duplex       string  `json:"duplex"`
	MTU          int     `json:"mtu"`
	Driver       string  `json:"driver"`
	RXBytes      uint64  `json:"rxBytes"`
	RXPackets    uint64  `json:"rxPackets"`
	RXErrors     uint64  `json:"rxErrors"`
	RXDrops      uint64  `json:"rxDrops"`
	TXBytes      uint64  `json:"txBytes"`
	TXPackets    uint64  `json:"txPackets"`
	TXErrors     uint64  `json:"txErrors"`
	TXDrops      uint64  `json:"txDrops"`
	RXRate       float64 `json:"rxRate"`
	TXRate       float64 `json:"txRate"`
	RXPacketRate float64 `json:"rxPacketRate"`
	TXPacketRate float64 `json:"txPacketRate"`
}

type ProcMetrics struct {
	Valid     bool          `json:"valid"`
	Total     int           `json:"total"`
	Running   int           `json:"running"`
	Sleeping  int           `json:"sleeping"`
	Stopped   int           `json:"stopped"`
	Zombie    int           `json:"zombie"`
	Uptime    float64       `json:"uptime"`
	TopMemory []ProcessInfo `json:"topMemory"`
	TopCPU    []ProcessInfo `json:"topCpu"`
}

type ProcessInfo struct {
	CPUValid      bool    `json:"cpuValid"`
	PID           int     `json:"pid"`
	PPID          int     `json:"ppid"`
	UID           string  `json:"uid"`
	User          string  `json:"user"`
	Name          string  `json:"name"`
	Command       string  `json:"command"`
	State         string  `json:"state"`
	Threads       int     `json:"threads"`
	RSS           uint64  `json:"rss"`
	VSZ           uint64  `json:"vsz"`
	RssAnon       uint64  `json:"rssAnon"`
	RssFile       uint64  `json:"rssFile"`
	VmData        uint64  `json:"vmData"`
	VmSwap        uint64  `json:"vmSwap"`
	MemoryPercent float64 `json:"memoryPercent"`
	CPU           float64 `json:"cpu"`
}

type TCPMetrics struct {
	Valid        bool           `json:"valid"`
	Partial      bool           `json:"partial"`
	Total        int            `json:"total"`
	Established  int            `json:"established"`
	Listening    int            `json:"listening"`
	UDPListening int            `json:"udpListening"`
	TimeWait     int            `json:"timeWait"`
	CloseWait    int            `json:"closeWait"`
	States       map[string]int `json:"states"`
	Ports        []int          `json:"ports"`
	Listeners    []PortListener `json:"listeners"`
}

type PortListener struct {
	Protocol     string `json:"protocol"`
	LocalAddress string `json:"localAddress"`
	Port         int    `json:"port"`
	PID          int    `json:"pid"`
	Process      string `json:"process"`
	Command      string `json:"command"`
	User         string `json:"user"`
}

type PressureSet struct {
	CPU    map[string]PressureValues `json:"cpu"`
	Memory map[string]PressureValues `json:"memory"`
	IO     map[string]PressureValues `json:"io"`
}

type PressureValues struct {
	Avg10  float64 `json:"avg10"`
	Avg60  float64 `json:"avg60"`
	Avg300 float64 `json:"avg300"`
	Total  float64 `json:"total"`
}

type TempMetrics struct {
	Max   *float64   `json:"max"`
	CPU   *TempZone  `json:"cpu"`
	Zones []TempZone `json:"zones"`
}

type TempZone struct {
	Name    string  `json:"name"`
	Type    string  `json:"type"`
	Celsius float64 `json:"celsius"`
}

type FileHandles struct {
	Valid     bool    `json:"valid"`
	Allocated uint64  `json:"allocated"`
	Unused    uint64  `json:"unused"`
	Used      uint64  `json:"used"`
	Max       uint64  `json:"max"`
	Percent   float64 `json:"percent"`
}

type HistoryPoint struct {
	Timestamp string   `json:"timestamp"`
	CPU       *float64 `json:"cpu"`
	Memory    *float64 `json:"memory"`
	Disk      *float64 `json:"disk"`
	RXRate    *float64 `json:"rxRate"`
	TXRate    *float64 `json:"txRate"`
	ReadRate  *float64 `json:"readRate"`
	WriteRate *float64 `json:"writeRate"`
	Load      *float64 `json:"load"`
}

func main() {
	if err := run(); err != nil {
		fmt.Fprintf(os.Stderr, "server failed: %v\n", err)
		os.Exit(1)
	}
}

func (s *AppState) snapshot() *Metrics {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.latest
}

func (s *AppState) collect() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	now := time.Now().UTC()
	cpu := readCPU(s)
	mem := readMemory()
	disk := readDisk(s)
	netm := readNetwork(s)
	procs := readProcesses(s, mem.Total)
	tcp := readTCP()
	host := readHost()
	metrics := &Metrics{
		// long: 半秒采样必须保留亚秒时间，避免两帧指标和趋势点共享同一个秒级时间戳。
		Timestamp:   now.Format(time.RFC3339Nano),
		Host:        host,
		Hardware:    s.readHardware(host, cpu, mem, disk, netm),
		CPU:         cpu,
		Memory:      mem,
		Disk:        disk,
		Network:     netm,
		Processes:   procs,
		TCP:         tcp,
		Pressure:    readPressure(),
		Temperature: readTemperature(),
		FileHandles: readFileHandles(),
	}

	s.appendHistory(metrics)
	s.latest = metrics
	return nil
}

func readCPU(s *AppState) CPUMetrics {
	current := parseCPUStats()
	info := readCPUInfo()
	load := readLoadAvg()
	metrics := CPUMetrics{
		LoadValid: len(load) == 3,
		Cores:     runtime.NumCPU(),
		Load:      load,
		Model:     info.model,
		SpeedMHz:  info.speedMHz,
		Breakdown: CPUBreakdown{
			Idle: 100,
		},
	}
	if metrics.Cores == 0 {
		metrics.Cores = len(current.Cores)
	}
	if s.prevCPU == nil || current.Aggregate.Total == 0 {
		for _, core := range current.Cores {
			metrics.PerCore = append(metrics.PerCore, CoreUsage{Index: core.Index, Percent: 0})
		}
		if current.Aggregate.Total > 0 {
			s.prevCPU = current
		} else {
			s.prevCPU = nil
		}
		metrics.LoadPercent = (first(load) / float64(max(metrics.Cores, 1))) * 100
		return metrics
	}

	percent, breakdown := cpuDelta(current.Aggregate, s.prevCPU.Aggregate)
	metrics.Percent = percent
	metrics.Valid = current.Aggregate.Total > s.prevCPU.Aggregate.Total
	metrics.Breakdown = breakdown
	prevCores := map[int]CPUStat{}
	for _, core := range s.prevCPU.Cores {
		prevCores[core.Index] = core
	}
	for _, core := range current.Cores {
		prev, ok := prevCores[core.Index]
		value := 0.0
		if ok {
			value, _ = cpuDelta(core, prev)
		}
		metrics.PerCore = append(metrics.PerCore, CoreUsage{Index: core.Index, Percent: value})
	}
	metrics.LoadPercent = (first(load) / float64(max(metrics.Cores, 1))) * 100
	s.prevCPU = current
	return metrics
}

func parseCPUStats() *CPUSnapshot {
	text := readText("/proc/stat")
	var snap CPUSnapshot
	for _, line := range strings.Split(text, "\n") {
		if strings.HasPrefix(line, "cpu ") {
			snap.Aggregate = parseCPULine(line, -1)
		} else if strings.HasPrefix(line, "cpu") {
			fields := strings.Fields(line)
			if len(fields) > 0 && len(fields[0]) > 3 {
				idx, err := strconv.Atoi(fields[0][3:])
				if err == nil {
					stat := parseCPULine(line, idx)
					snap.Cores = append(snap.Cores, stat)
				}
			}
		}
	}
	return &snap
}

func parseCPULine(line string, index int) CPUStat {
	fields := strings.Fields(line)
	values := make([]float64, 10)
	for i := 1; i < len(fields) && i <= 10; i++ {
		values[i-1], _ = strconv.ParseFloat(fields[i], 64)
	}
	stat := CPUStat{
		Index:   index,
		User:    values[0],
		Nice:    values[1],
		System:  values[2],
		Idle:    values[3],
		IOWait:  values[4],
		IRQ:     values[5],
		SoftIRQ: values[6],
		Steal:   values[7],
	}
	stat.IdleAll = stat.Idle + stat.IOWait
	stat.SystemAll = stat.System + stat.IRQ + stat.SoftIRQ
	// long: guest 已计入 user/nice，再累加会让虚拟化负载下的 CPU 使用率虚高。
	for _, value := range values[:8] {
		stat.Total += value
	}
	return stat
}

func cpuDelta(current, prev CPUStat) (float64, CPUBreakdown) {
	total := current.Total - prev.Total
	if total <= 0 {
		return 0, CPUBreakdown{Idle: 100}
	}
	idle := current.IdleAll - prev.IdleAll
	user := current.User + current.Nice - prev.User - prev.Nice
	system := current.SystemAll - prev.SystemAll
	iowait := current.IOWait - prev.IOWait
	steal := current.Steal - prev.Steal
	return clamp((1 - idle/total) * 100), CPUBreakdown{
		User:   clamp(user / total * 100),
		System: clamp(system / total * 100),
		IOWait: clamp(iowait / total * 100),
		Steal:  clamp(steal / total * 100),
		Idle:   clamp((current.Idle - prev.Idle) / total * 100),
	}
}

func readMemory() MemoryMetrics {
	return parseMemory(readText("/proc/meminfo"))
}

func parseMemory(text string) MemoryMetrics {
	values := map[string]uint64{}
	for _, line := range strings.Split(text, "\n") {
		parts := strings.Fields(line)
		if len(parts) >= 2 {
			key := strings.TrimSuffix(parts[0], ":")
			value, _ := strconv.ParseUint(parts[1], 10, 64)
			values[key] = value * 1024
		}
	}
	total := values["MemTotal"]
	available := values["MemAvailable"]
	free := values["MemFree"]
	cached := values["Cached"] + values["SReclaimable"]
	buffers := values["Buffers"]
	_, hasAvailable := values["MemAvailable"]
	if !hasAvailable {
		// long: 老内核缺少 MemAvailable 时只能估算，必须标记，不能误报内存占满。
		available = free + buffers + cached
	}
	if available > total {
		available = total
	}
	slab := values["Slab"]
	used := uint64(0)
	if total > available {
		used = total - available
	}
	swapTotal := values["SwapTotal"]
	swapUsed := uint64(0)
	if swapTotal > values["SwapFree"] {
		swapUsed = swapTotal - values["SwapFree"]
	}
	return MemoryMetrics{
		Valid:        total > 0,
		Estimated:    !hasAvailable,
		Total:        total,
		Available:    available,
		Free:         free,
		Cached:       cached,
		Buffers:      buffers,
		Slab:         slab,
		SReclaimable: values["SReclaimable"],
		Used:         used,
		Percent:      percent(used, total),
		SwapTotal:    swapTotal,
		SwapUsed:     swapUsed,
		SwapPercent:  percent(swapUsed, swapTotal),
		Active:       values["Active"],
		Inactive:     values["Inactive"],
		Dirty:        values["Dirty"],
		Writeback:    values["Writeback"],
		AnonPages:    values["AnonPages"],
		Mapped:       values["Mapped"],
		Shmem:        values["Shmem"],
		PageTables:   values["PageTables"],
		CommitLimit:  values["CommitLimit"],
		CommittedAS:  values["Committed_AS"],
	}
}

func readDisk(s *AppState) DiskMetrics {
	filesystems := readFilesystems()
	root := Filesystem{Filesystem: "unknown", Type: "unknown", Mount: "/", Total: 0}
	for _, fs := range filesystems {
		if fs.Mount == "/" {
			root = fs
			break
		}
	}
	ioMetrics := readDiskIO(s)
	return DiskMetrics{
		Valid:       root.Total > 0,
		Filesystem:  root.Filesystem,
		Type:        root.Type,
		Mount:       root.Mount,
		Total:       root.Total,
		Used:        root.Used,
		Available:   root.Available,
		Percent:     root.Percent,
		Filesystems: filesystems,
		IO:          ioMetrics,
	}
}

func readFilesystems() []Filesystem {
	out, err := exec.Command("df", "-B1", "-P", "-T").Output()
	if err != nil {
		return nil
	}
	var filesystems []Filesystem
	lines := strings.Split(strings.TrimSpace(string(out)), "\n")
	for _, line := range lines[1:] {
		parts := strings.Fields(line)
		if len(parts) < 7 {
			continue
		}
		total := parseUint(parts[2])
		used := parseUint(parts[3])
		available := parseUint(parts[4])
		mount := strings.Join(parts[6:], " ")
		if total == 0 || (mount != "/" && (parts[1] == "tmpfs" || parts[1] == "devtmpfs")) {
			continue
		}
		filesystems = append(filesystems, Filesystem{
			Filesystem: parts[0],
			Type:       parts[1],
			Mount:      mount,
			Total:      total,
			Used:       used,
			Available:  available,
			// long: 与 df 一致排除保留块，并向上取整，避免面板与命令行使用率冲突。
			Percent: filesystemPercent(used, available),
		})
	}
	sort.Slice(filesystems, func(i, j int) bool {
		if filesystems[i].Mount == filesystems[j].Mount {
			return false
		}
		if filesystems[i].Mount == "/" {
			return true
		}
		if filesystems[j].Mount == "/" {
			return false
		}
		return filesystems[i].Percent > filesystems[j].Percent
	})
	if len(filesystems) > 10 {
		filesystems = filesystems[:10]
	}
	return filesystems
}

func readDiskIO(s *AppState) DiskIO {
	now := time.Now()
	devices := map[string]DiskDevice{}
	text := readText("/proc/diskstats")
	for _, line := range strings.Split(text, "\n") {
		parts := strings.Fields(line)
		if len(parts) < 14 || !isDiskDevice(parts[2]) {
			continue
		}
		name := parts[2]
		reads := parseUint(parts[3])
		writes := parseUint(parts[7])
		readBytes := parseUint(parts[5]) * 512
		writeBytes := parseUint(parts[9]) * 512
		ioMs := parseUint(parts[12])
		devices[name] = DiskDevice{Name: name, Reads: reads, Writes: writes, ReadBytes: readBytes, WriteBytes: writeBytes, IOMs: ioMs}
	}
	raw := &DiskRaw{Time: now, Devices: devices}
	for _, d := range selectAggregateDiskDevices(devices) {
		raw.Reads += d.Reads
		raw.Writes += d.Writes
		raw.ReadBytes += d.ReadBytes
		raw.WriteBytes += d.WriteBytes
		raw.IOMs += d.IOMs
	}
	result := calculateDiskIO(raw, s.prevDisk)
	result.Valid = text != "" && len(devices) > 0
	sort.Slice(result.Devices, func(i, j int) bool {
		return result.Devices[i].ReadRate+result.Devices[i].WriteRate > result.Devices[j].ReadRate+result.Devices[j].WriteRate
	})
	if result.Valid {
		s.prevDisk = raw
	} else {
		s.prevDisk = nil
	}
	return result
}

func readNetwork(s *AppState) NetworkMetrics {
	now := time.Now()
	var interfaces []NetInterface
	netLines := strings.Split(readText("/proc/net/dev"), "\n")
	if len(netLines) < 3 {
		netLines = nil
	} else {
		netLines = netLines[2:]
	}
	for _, line := range netLines {
		parts := strings.Split(line, ":")
		if len(parts) != 2 {
			continue
		}
		name := strings.TrimSpace(parts[0])
		if name == "" || name == "lo" {
			continue
		}
		values := strings.Fields(parts[1])
		if len(values) < 16 {
			continue
		}
		details := readNetworkInterfaceDetails(name)
		interfaces = append(interfaces, NetInterface{
			Name:      name,
			Kind:      details.Kind,
			State:     details.State,
			SpeedMbps: details.SpeedMbps,
			Duplex:    details.Duplex,
			MTU:       details.MTU,
			Driver:    details.Driver,
			RXBytes:   parseUint(values[0]),
			RXPackets: parseUint(values[1]),
			RXErrors:  parseUint(values[2]),
			RXDrops:   parseUint(values[3]),
			TXBytes:   parseUint(values[8]),
			TXPackets: parseUint(values[9]),
			TXErrors:  parseUint(values[10]),
			TXDrops:   parseUint(values[11]),
		})
	}
	raw := &NetworkRaw{Time: now, Interfaces: map[string]NetInterface{}}
	for _, item := range interfaces {
		raw.RXBytes += item.RXBytes
		raw.TXBytes += item.TXBytes
		raw.RXPackets += item.RXPackets
		raw.TXPackets += item.TXPackets
		raw.Interfaces[item.Name] = item
	}
	result := calculateNetwork(raw, s.prevNetwork)
	result.Valid = netLines != nil
	result.Addresses = readAddresses()
	sort.Slice(result.Interfaces, func(i, j int) bool {
		return result.Interfaces[i].RXRate+result.Interfaces[i].TXRate > result.Interfaces[j].RXRate+result.Interfaces[j].TXRate
	})
	if result.Valid {
		s.prevNetwork = raw
	} else {
		s.prevNetwork = nil
	}
	return result
}

func readProcesses(s *AppState, memoryTotal uint64) ProcMetrics {
	entries, readErr := os.ReadDir("/proc")
	now := time.Now()
	previous := s.prevProc
	seconds := 0.0
	if previous != nil {
		seconds = math.Max(now.Sub(previous.Time).Seconds(), 0.001)
	}
	current := &ProcRaw{Time: now, Processes: map[string]ProcSample{}}
	var rows []ProcessInfo
	result := ProcMetrics{Valid: readErr == nil, Uptime: readUptime()}
	tickHz := clockTicks()
	for _, entry := range entries {
		if !entry.IsDir() || !isDigits(entry.Name()) {
			continue
		}
		pid := entry.Name()
		status := readText("/proc/" + pid + "/status")
		if status == "" {
			continue
		}
		name := matchStatus(status, "Name")
		state := stringFirst(matchStatus(status, "State"))
		rss := parseUint(matchStatus(status, "VmRSS")) * 1024
		vsz := parseUint(matchStatus(status, "VmSize")) * 1024
		rssAnon := parseUint(matchStatus(status, "RssAnon")) * 1024
		rssFile := parseUint(matchStatus(status, "RssFile")) * 1024
		vmData := parseUint(matchStatus(status, "VmData")) * 1024
		vmSwap := parseUint(matchStatus(status, "VmSwap")) * 1024
		ppid := int(parseUint(matchStatus(status, "PPid")))
		uid := matchStatus(status, "Uid")
		threads := int(parseUint(matchStatus(status, "Threads")))
		sample := parseProcSample(readText("/proc/" + pid + "/stat"))
		current.Processes[pid] = sample
		cpuPercent := 0.0
		cpuValid := false
		if previous != nil {
			if old, ok := previous.Processes[pid]; ok {
				cpuPercent, cpuValid = processCPU(sample, old, tickHz, seconds)
			}
		}
		pidNum, _ := strconv.Atoi(pid)
		cmd := strings.ReplaceAll(readText("/proc/"+pid+"/cmdline"), "\x00", " ")
		row := ProcessInfo{
			CPUValid:      cpuValid,
			PID:           pidNum,
			PPID:          ppid,
			UID:           uid,
			User:          usernameForUID(uid),
			Name:          name,
			Command:       strings.TrimSpace(cmd),
			State:         state,
			Threads:       threads,
			RSS:           rss,
			VSZ:           vsz,
			RssAnon:       rssAnon,
			RssFile:       rssFile,
			VmData:        vmData,
			VmSwap:        vmSwap,
			MemoryPercent: percent(rss, memoryTotal),
			CPU:           cpuPercent,
		}
		rows = append(rows, row)
		result.Total++
		switch state {
		case "R":
			result.Running++
		case "S", "I":
			result.Sleeping++
		case "T":
			result.Stopped++
		case "Z":
			result.Zombie++
		}
	}
	s.prevProc = current
	sort.Slice(rows, func(i, j int) bool { return rows[i].RSS > rows[j].RSS })
	result.TopMemory = append(result.TopMemory, rows[:min(len(rows), 8)]...)
	sort.Slice(rows, func(i, j int) bool { return rows[i].CPU > rows[j].CPU })
	result.TopCPU = append(result.TopCPU, rows[:min(len(rows), 8)]...)
	return result
}

func readTCP() TCPMetrics {
	stateMap := map[string]string{"01": "established", "02": "synSent", "03": "synRecv", "04": "finWait1", "05": "finWait2", "06": "timeWait", "07": "close", "08": "closeWait", "09": "lastAck", "0A": "listening", "0B": "closing"}
	result := TCPMetrics{States: map[string]int{}}
	ports := map[int]bool{}
	socketFiles := []struct {
		path     string
		protocol string
		tcp      bool
	}{
		{path: "/proc/net/tcp", protocol: "tcp", tcp: true},
		{path: "/proc/net/tcp6", protocol: "tcp6", tcp: true},
		{path: "/proc/net/udp", protocol: "udp"},
		{path: "/proc/net/udp6", protocol: "udp6"},
	}
	var listenerEntries []socketEntry
	for _, file := range socketFiles {
		if _, err := os.ReadFile(file.path); err != nil {
			result.Partial = true
			continue
		}
		result.Valid = true
		for _, entry := range readSocketEntries(file.path, file.protocol, file.tcp, stateMap) {
			if file.tcp {
				result.Total++
				result.States[entry.State]++
				if entry.State == "established" {
					result.Established++
				}
				if entry.State == "timeWait" {
					result.TimeWait++
				}
				if entry.State == "closeWait" {
					result.CloseWait++
				}
				if entry.State != "listening" {
					continue
				}
				result.Listening++
			} else {
				result.UDPListening++
			}
			if entry.Port <= 0 {
				continue
			}
			ports[entry.Port] = true
			listenerEntries = append(listenerEntries, entry)
		}
	}
	processesByInode := readSocketProcessMap(listenerEntries)
	for _, entry := range listenerEntries {
		process := processesByInode[entry.Inode]
		result.Listeners = append(result.Listeners, PortListener{
			Protocol:     entry.Protocol,
			LocalAddress: entry.Address,
			Port:         entry.Port,
			PID:          process.PID,
			Process:      process.Name,
			Command:      process.Command,
			User:         process.User,
		})
	}
	for port := range ports {
		result.Ports = append(result.Ports, port)
	}
	sort.Ints(result.Ports)
	sort.Slice(result.Listeners, func(i, j int) bool {
		if result.Listeners[i].Port == result.Listeners[j].Port {
			return result.Listeners[i].Protocol < result.Listeners[j].Protocol
		}
		return result.Listeners[i].Port < result.Listeners[j].Port
	})
	return result
}

func readPressure() PressureSet {
	return PressureSet{
		CPU:    parsePressure(readText("/proc/pressure/cpu")),
		Memory: parsePressure(readText("/proc/pressure/memory")),
		IO:     parsePressure(readText("/proc/pressure/io")),
	}
}

func parsePressure(text string) map[string]PressureValues {
	result := map[string]PressureValues{}
	for _, line := range strings.Split(strings.TrimSpace(text), "\n") {
		parts := strings.Fields(line)
		if len(parts) == 0 {
			continue
		}
		values := PressureValues{}
		for _, part := range parts[1:] {
			kv := strings.SplitN(part, "=", 2)
			if len(kv) != 2 {
				continue
			}
			value, _ := strconv.ParseFloat(kv[1], 64)
			switch kv[0] {
			case "avg10":
				values.Avg10 = value
			case "avg60":
				values.Avg60 = value
			case "avg300":
				values.Avg300 = value
			case "total":
				values.Total = value
			}
		}
		result[parts[0]] = values
	}
	return result
}

func readTemperature() TempMetrics {
	entries, _ := os.ReadDir("/sys/class/thermal")
	var zones []TempZone
	for _, entry := range entries {
		if !isThermalZoneName(entry.Name()) {
			continue
		}
		base := "/sys/class/thermal/" + entry.Name()
		celsius, valid := parseMilliCelsius(readText(base + "/temp"))
		if !valid {
			continue
		}
		zones = append(zones, TempZone{Name: entry.Name(), Type: strings.TrimSpace(readText(base + "/type")), Celsius: celsius})
	}
	zones = append(zones, readCPUHwmonTemperatures()...)
	sort.Slice(zones, func(i, j int) bool { return zones[i].Celsius > zones[j].Celsius })
	result := TempMetrics{Zones: zones[:min(len(zones), 6)]}
	if len(zones) > 0 {
		maxTemp := zones[0].Celsius
		result.Max = &maxTemp
	}
	for _, zone := range zones {
		if isCPUTemperatureType(zone.Type) {
			cpuZone := zone
			result.CPU = &cpuZone
			break
		}
	}
	return result
}

func readFileHandles() FileHandles {
	parts := strings.Fields(readText("/proc/sys/fs/file-nr"))
	if len(parts) < 3 {
		return FileHandles{}
	}
	allocated := parseUint(parts[0])
	unused := parseUint(parts[1])
	maxFiles := parseUint(parts[2])
	used := uint64(0)
	if allocated > unused {
		used = allocated - unused
	}
	return FileHandles{Valid: true, Allocated: allocated, Unused: unused, Used: used, Max: maxFiles, Percent: percent(used, maxFiles)}
}

func readHost() HostMetrics {
	hostname, _ := os.Hostname()
	osInfo := readOSRelease()
	now := time.Now()
	zoneName, zoneOffset := now.Zone()
	currentUser := userInfo{}
	if u, err := user.Current(); err == nil {
		currentUser.username = u.Username
		currentUser.uid = u.Uid
		currentUser.gid = u.Gid
		currentUser.homeDir = u.HomeDir
		currentUser.shell = readUserShell(u.Username, u.Uid)
	}
	return HostMetrics{
		Hostname:          hostname,
		Platform:          runtime.GOOS,
		OSName:            firstNonEmpty(osInfo["PRETTY_NAME"], runtime.GOOS),
		OSID:              osInfo["ID"],
		OSVersionID:       osInfo["VERSION_ID"],
		OSVersionCodename: firstNonEmpty(osInfo["VERSION_CODENAME"], osInfo["UBUNTU_CODENAME"]),
		Release:           strings.TrimSpace(commandOutput("uname", "-r")),
		KernelVersion:     strings.TrimSpace(commandOutput("uname", "-v")),
		Arch:              runtime.GOARCH,
		Machine:           strings.TrimSpace(commandOutput("uname", "-m")),
		GoVersion:         runtime.Version(),
		Uptime:            readUptime(),
		BootTime:          readBootTime(),
		LocalTime:         now.Format(time.RFC3339),
		Timezone:          fmt.Sprintf("%s (%s)", zoneName, formatUTCOffset(zoneOffset)),
		User:              currentUser.username,
		UID:               currentUser.uid,
		GID:               currentUser.gid,
		HomeDir:           currentUser.homeDir,
		Shell:             currentUser.shell,
		Virtualization:    readVirtualization(),
		Container:         readContainer(),
		ProductVendor:     readDMI("sys_vendor"),
		ProductName:       readDMI("product_name"),
		ProductVersion:    readDMI("product_version"),
		ProductFamily:     readDMI("product_family"),
		BoardVendor:       readDMI("board_vendor"),
		BoardName:         readDMI("board_name"),
		BoardVersion:      readDMI("board_version"),
		BIOSVendor:        readDMI("bios_vendor"),
		BIOSVersion:       readDMI("bios_version"),
		BIOSDate:          readDMI("bios_date"),
		ChassisVendor:     readDMI("chassis_vendor"),
		ChassisType:       readChassisType(),
		ChassisVersion:    readDMI("chassis_version"),
	}
}

func (s *AppState) readHardware(host HostMetrics, cpu CPUMetrics, mem MemoryMetrics, disk DiskMetrics, netm NetworkMetrics) HardwareMetrics {
	if !s.hardwareAt.IsZero() && time.Since(s.hardwareAt) < 30*time.Second {
		// long: 仅缓存静态硬件，拷贝切片后更新动态值，避免修改旧快照引发数据竞争。
		hardware := s.hardware
		hardware.CPU.SpeedMHz = cpu.SpeedMHz
		hardware.Memory = readMemoryHardware(mem)
		hardware.Network = append([]NetworkHardware(nil), s.hardware.Network...)
		for i := range hardware.Network {
			item := &hardware.Network[i]
			details := readNetworkInterfaceDetails(item.Name)
			item.Kind, item.State = details.Kind, details.State
			item.SpeedMbps, item.Duplex, item.MTU = details.SpeedMbps, details.Duplex, details.MTU
		}
		return hardware
	}
	pciDevices := readPCIDevices()
	hardware := HardwareMetrics{
		Motherboard: readMotherboardHardware(host),
		CPU:         readCPUHardware(cpu),
		Memory:      readMemoryHardware(mem),
		Storage:     readStorageHardware(disk),
		GPUs:        filterPCIDevices(pciDevices, isGPUPCIClass),
		Network:     readNetworkHardware(netm, pciDevices),
	}
	s.hardware = hardware
	s.hardwareAt = time.Now()
	return hardware
}

func readMotherboardHardware(host HostMetrics) MotherboardHardware {
	return MotherboardHardware{
		SystemVendor:   host.ProductVendor,
		SystemModel:    host.ProductName,
		SystemVersion:  host.ProductVersion,
		ProductFamily:  host.ProductFamily,
		BoardVendor:    host.BoardVendor,
		BoardName:      host.BoardName,
		BoardVersion:   host.BoardVersion,
		BIOSVendor:     host.BIOSVendor,
		BIOSVersion:    host.BIOSVersion,
		BIOSDate:       host.BIOSDate,
		ChassisVendor:  host.ChassisVendor,
		ChassisType:    host.ChassisType,
		ChassisVersion: host.ChassisVersion,
	}
}

func readCPUHardware(cpu CPUMetrics) CPUHardware {
	records := parseCPUInfoRecords()
	firstRecord := map[string]string{}
	if len(records) > 0 {
		firstRecord = records[0]
	}

	sockets := map[string]bool{}
	corePairs := map[string]bool{}
	coresPerSocket := 0
	for _, record := range records {
		physicalID := record["physical id"]
		coreID := record["core id"]
		if physicalID != "" {
			sockets[physicalID] = true
		}
		if physicalID != "" && coreID != "" {
			corePairs[physicalID+":"+coreID] = true
		}
		if value := parseIntValue(record["cpu cores"]); value > coresPerSocket {
			coresPerSocket = value
		}
	}

	socketCount := len(sockets)
	if socketCount == 0 && len(records) > 0 {
		socketCount = 1
	}
	physicalCores := len(corePairs)
	if physicalCores == 0 && socketCount > 0 && coresPerSocket > 0 {
		physicalCores = socketCount * coresPerSocket
	}
	if physicalCores == 0 {
		physicalCores = cpu.Cores
	}

	model := firstNonEmpty(cpu.Model, firstRecord["model name"], firstRecord["hardware"], firstRecord["processor"])
	if strings.EqualFold(model, "unknown") {
		model = firstNonEmpty(firstRecord["model name"], firstRecord["hardware"], firstRecord["processor"], cpu.Model)
	}

	return CPUHardware{
		Model:          model,
		Vendor:         firstNonEmpty(firstRecord["vendor_id"], firstRecord["vendor"], firstRecord["cpu implementer"]),
		Architecture:   firstNonEmpty(strings.TrimSpace(commandOutput("uname", "-m")), runtime.GOARCH),
		LogicalCores:   max(cpu.Cores, len(records)),
		PhysicalCores:  physicalCores,
		Sockets:        socketCount,
		CoresPerSocket: coresPerSocket,
		SpeedMHz:       cpu.SpeedMHz,
		MaxMHz:         readCPUFrequencyMHz("cpuinfo_max_freq", "scaling_max_freq"),
		MinMHz:         readCPUFrequencyMHz("cpuinfo_min_freq", "scaling_min_freq"),
		CacheSize:      firstRecord["cache size"],
		BogoMIPS:       firstRecord["bogomips"],
		Microcode:      firstRecord["microcode"],
		Flags:          strings.Fields(firstNonEmpty(firstRecord["flags"], firstRecord["features"])),
	}
}

func parseCPUInfoRecords() []map[string]string {
	var records []map[string]string
	current := map[string]string{}
	for _, line := range strings.Split(readText("/proc/cpuinfo"), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			if len(current) > 0 {
				records = append(records, current)
				current = map[string]string{}
			}
			continue
		}
		parts := strings.SplitN(line, ":", 2)
		if len(parts) != 2 {
			continue
		}
		key := strings.ToLower(strings.TrimSpace(parts[0]))
		if _, exists := current[key]; !exists {
			current[key] = strings.TrimSpace(parts[1])
		}
	}
	if len(current) > 0 {
		records = append(records, current)
	}
	return records
}

func readCPUFrequencyMHz(names ...string) int {
	for _, name := range names {
		for _, path := range []string{
			"/sys/devices/system/cpu/cpu0/cpufreq/" + name,
			"/sys/devices/system/cpu/cpufreq/policy0/" + name,
		} {
			value := parseIntValue(readText(path))
			if value > 0 {
				return int(math.Round(float64(value) / 1000))
			}
		}
	}
	return 0
}

func readMemoryHardware(mem MemoryMetrics) MemoryHardware {
	values, counts := readMemInfoValues()
	return MemoryHardware{
		Total:          mem.Total,
		Available:      mem.Available,
		SwapTotal:      mem.SwapTotal,
		SwapFree:       values["SwapFree"],
		PageSize:       uint64(os.Getpagesize()),
		HugePageSize:   values["Hugepagesize"],
		HugePagesTotal: counts["HugePages_Total"],
		HugePagesFree:  counts["HugePages_Free"],
		NUMANodes:      readNUMANodeCount(),
	}
}

func readMemInfoValues() (map[string]uint64, map[string]uint64) {
	bytesValues := map[string]uint64{}
	countValues := map[string]uint64{}
	for _, line := range strings.Split(readText("/proc/meminfo"), "\n") {
		parts := strings.Fields(line)
		if len(parts) < 2 {
			continue
		}
		key := strings.TrimSuffix(parts[0], ":")
		value := parseUint(parts[1])
		if len(parts) >= 3 && strings.EqualFold(parts[2], "kB") {
			bytesValues[key] = value * 1024
		} else {
			countValues[key] = value
		}
	}
	return bytesValues, countValues
}

func readNUMANodeCount() int {
	entries, err := os.ReadDir("/sys/devices/system/node")
	if err != nil {
		return 0
	}
	count := 0
	for _, entry := range entries {
		if entry.IsDir() && strings.HasPrefix(entry.Name(), "node") && isDigits(strings.TrimPrefix(entry.Name(), "node")) {
			count++
		}
	}
	return count
}

type lsblkOutput struct {
	BlockDevices []lsblkDevice `json:"blockdevices"`
}

type lsblkDevice struct {
	Name       string        `json:"name"`
	Type       string        `json:"type"`
	Size       uint64        `json:"size"`
	Model      string        `json:"model"`
	Transport  string        `json:"tran"`
	Rotational bool          `json:"rota"`
	Removable  bool          `json:"rm"`
	Mountpoint string        `json:"mountpoint"`
	FSType     string        `json:"fstype"`
	Children   []lsblkDevice `json:"children"`
}

func readStorageHardware(disk DiskMetrics) []StorageHardware {
	out := commandOutput("lsblk", "-J", "-b", "-o", "NAME,TYPE,SIZE,MODEL,TRAN,ROTA,RM,MOUNTPOINT,FSTYPE")
	if out == "" {
		return readStorageHardwareFromSys(disk)
	}
	var data lsblkOutput
	if err := json.Unmarshal([]byte(out), &data); err != nil {
		return readStorageHardwareFromSys(disk)
	}
	var result []StorageHardware
	for _, device := range data.BlockDevices {
		if device.Type != "disk" {
			continue
		}
		mountpoints, filesystems := collectBlockDeviceUsage(device)
		result = append(result, StorageHardware{
			Name:        device.Name,
			Type:        device.Type,
			Size:        device.Size,
			Model:       cleanHardwareValue(device.Model),
			Transport:   cleanHardwareValue(device.Transport),
			Medium:      storageMedium(device),
			Rotational:  device.Rotational,
			Removable:   device.Removable,
			Partitions:  countBlockPartitions(device),
			Mountpoints: mountpoints,
			Filesystems: filesystems,
		})
	}
	sort.Slice(result, func(i, j int) bool { return result[i].Name < result[j].Name })
	if len(result) > 0 {
		return result
	}
	return readStorageHardwareFromSys(disk)
}

func collectBlockDeviceUsage(device lsblkDevice) ([]string, []string) {
	mountMap := map[string]bool{}
	fsMap := map[string]bool{}
	var walk func(lsblkDevice)
	walk = func(item lsblkDevice) {
		if value := cleanHardwareValue(item.Mountpoint); value != "" {
			mountMap[value] = true
		}
		if value := cleanHardwareValue(item.FSType); value != "" {
			fsMap[value] = true
		}
		for _, child := range item.Children {
			walk(child)
		}
	}
	walk(device)
	return sortedKeys(mountMap), sortedKeys(fsMap)
}

func countBlockPartitions(device lsblkDevice) int {
	count := 0
	var walk func(lsblkDevice)
	walk = func(item lsblkDevice) {
		for _, child := range item.Children {
			if child.Type == "part" {
				count++
			}
			walk(child)
		}
	}
	walk(device)
	return count
}

func storageMedium(device lsblkDevice) string {
	if device.Removable {
		return "Removable"
	}
	if strings.EqualFold(device.Transport, "nvme") {
		return "NVMe SSD"
	}
	if device.Rotational {
		return "HDD"
	}
	return "SSD"
}

func readStorageHardwareFromSys(disk DiskMetrics) []StorageHardware {
	entries, err := os.ReadDir("/sys/block")
	if err != nil {
		return nil
	}
	fsMounts := map[string][]string{}
	for _, fs := range disk.Filesystems {
		name := strings.TrimPrefix(filepath.Base(fs.Filesystem), "/")
		if name != "" {
			fsMounts[name] = append(fsMounts[name], fs.Mount)
		}
	}
	var result []StorageHardware
	for _, entry := range entries {
		name := entry.Name()
		if !isDiskDevice(name) || strings.HasPrefix(name, "loop") || strings.HasPrefix(name, "ram") {
			continue
		}
		base := "/sys/block/" + name
		rotational := strings.TrimSpace(readText(base+"/queue/rotational")) == "1"
		removable := strings.TrimSpace(readText(base+"/removable")) == "1"
		device := StorageHardware{
			Name:       name,
			Type:       "disk",
			Size:       parseUint(readText(base+"/size")) * 512,
			Model:      cleanHardwareValue(readText(base + "/device/model")),
			Medium:     storageMedium(lsblkDevice{Transport: cleanHardwareValue(readText(base + "/device/transport")), Rotational: rotational, Removable: removable}),
			Rotational: rotational,
			Removable:  removable,
		}
		if mounts, ok := fsMounts[name]; ok {
			device.Mountpoints = mounts
		}
		children, _ := os.ReadDir(base)
		for _, child := range children {
			if strings.HasPrefix(child.Name(), name) && child.Name() != name {
				device.Partitions++
			}
		}
		result = append(result, device)
	}
	sort.Slice(result, func(i, j int) bool { return result[i].Name < result[j].Name })
	return result
}

func readPCIDevices() []PCIHardwareDevice {
	out := commandOutput("lspci", "-mm")
	if out != "" {
		var result []PCIHardwareDevice
		for _, line := range strings.Split(strings.TrimSpace(out), "\n") {
			if device := parseLspciMMLine(line); device.Slot != "" {
				result = append(result, device)
			}
		}
		if len(result) > 0 {
			return result
		}
	}
	return readPCIDevicesFromSys()
}

func parseLspciMMLine(line string) PCIHardwareDevice {
	tokens := splitLspciMMLine(line)
	if len(tokens) < 4 {
		return PCIHardwareDevice{}
	}
	device := PCIHardwareDevice{
		Slot:   cleanHardwareValue(tokens[0]),
		Class:  cleanHardwareValue(tokens[1]),
		Vendor: cleanHardwareValue(tokens[2]),
		Device: cleanHardwareValue(tokens[3]),
	}
	var extra []string
	for _, token := range tokens[4:] {
		if strings.HasPrefix(token, "-") {
			continue
		}
		extra = append(extra, token)
	}
	if len(extra) >= 2 {
		device.SubsystemVendor = cleanHardwareValue(extra[len(extra)-2])
		device.SubsystemDevice = cleanHardwareValue(extra[len(extra)-1])
	}
	return device
}

func splitLspciMMLine(line string) []string {
	var tokens []string
	var current strings.Builder
	inQuote := false
	escaped := false
	flush := func() {
		if current.Len() == 0 {
			return
		}
		tokens = append(tokens, current.String())
		current.Reset()
	}
	for _, ch := range line {
		if escaped {
			current.WriteRune(ch)
			escaped = false
			continue
		}
		if ch == '\\' {
			escaped = true
			continue
		}
		if ch == '"' {
			inQuote = !inQuote
			continue
		}
		if !inQuote && (ch == ' ' || ch == '\t') {
			flush()
			continue
		}
		current.WriteRune(ch)
	}
	flush()
	return tokens
}

func readPCIDevicesFromSys() []PCIHardwareDevice {
	return readPCIDevicesAt("/sys/bus/pci/devices")
}

func readPCIDevicesAt(root string) []PCIHardwareDevice {
	entries, err := os.ReadDir(root)
	if err != nil {
		return nil
	}
	var result []PCIHardwareDevice
	for _, entry := range entries {
		base := filepath.Join(root, entry.Name())
		// long: sysfs 的 PCI 目录项通常是符号链接，需要跟随链接才能识别设备。
		info, err := os.Stat(base)
		if err != nil || !info.IsDir() {
			continue
		}
		vendorID := cleanHardwareValue(readText(base + "/vendor"))
		deviceID := cleanHardwareValue(readText(base + "/device"))
		result = append(result, PCIHardwareDevice{
			Slot:   entry.Name(),
			Class:  pciClassLabel(readText(base + "/class")),
			Vendor: vendorID,
			Device: deviceID,
		})
	}
	return result
}

func filterPCIDevices(devices []PCIHardwareDevice, predicate func(string) bool) []PCIHardwareDevice {
	var result []PCIHardwareDevice
	for _, device := range devices {
		if predicate(device.Class) {
			result = append(result, device)
		}
	}
	sort.Slice(result, func(i, j int) bool { return result[i].Slot < result[j].Slot })
	return result
}

func readNetworkHardware(netm NetworkMetrics, pciDevices []PCIHardwareDevice) []NetworkHardware {
	entries, err := os.ReadDir("/sys/class/net")
	if err != nil {
		return nil
	}
	pciBySlot := map[string]PCIHardwareDevice{}
	for _, device := range pciDevices {
		pciBySlot[device.Slot] = device
		if !strings.HasPrefix(device.Slot, "0000:") {
			pciBySlot["0000:"+device.Slot] = device
		}
	}
	activeNames := map[string]bool{}
	for _, iface := range netm.Interfaces {
		activeNames[iface.Name] = interfaceIsUp(iface.Name)
	}

	var result []NetworkHardware
	for _, entry := range entries {
		name := entry.Name()
		if name == "lo" {
			continue
		}
		base := "/sys/class/net/" + name
		item := NetworkHardware{
			Name:   name,
			Kind:   "virtual",
			State:  strings.TrimSpace(readText(base + "/operstate")),
			Duplex: cleanHardwareValue(readText(base + "/duplex")),
			MTU:    parseIntValue(readText(base + "/mtu")),
		}
		if speed := parseIntValue(readText(base + "/speed")); speed > 0 {
			item.SpeedMbps = speed
		}

		devicePath, err := filepath.EvalSymlinks(base + "/device")
		if err == nil && devicePath != "" && !strings.Contains(devicePath, "/devices/virtual/") {
			item.Kind = "physical"
			item.Bus = filepath.Base(devicePath)
			vendorID := cleanHardwareValue(readText(base + "/device/vendor"))
			deviceID := cleanHardwareValue(readText(base + "/device/device"))
			item.PCIID = formatPCIID(vendorID, deviceID)
			item.Vendor = vendorID
			item.Device = deviceID
			if driverPath, err := filepath.EvalSymlinks(base + "/device/driver"); err == nil {
				item.Driver = cleanHardwareValue(filepath.Base(driverPath))
			}
			if pci, ok := pciBySlot[item.Bus]; ok {
				item.Vendor = firstNonEmpty(pci.Vendor, item.Vendor)
				item.Device = firstNonEmpty(pci.Device, item.Device)
			}
		}
		if item.Kind == "virtual" && activeNames[name] {
			item.Kind = "virtual-active"
		}
		result = append(result, item)
	}
	sort.Slice(result, func(i, j int) bool {
		leftPhysical := result[i].Kind == "physical"
		rightPhysical := result[j].Kind == "physical"
		if leftPhysical != rightPhysical {
			return leftPhysical
		}
		return result[i].Name < result[j].Name
	})
	return result
}

func readOSName() string {
	osInfo := readOSRelease()
	if osInfo["PRETTY_NAME"] != "" {
		return osInfo["PRETTY_NAME"]
	}
	return runtime.GOOS
}

type userInfo struct {
	username string
	uid      string
	gid      string
	homeDir  string
	shell    string
}

func readOSRelease() map[string]string {
	result := map[string]string{}
	for _, line := range strings.Split(readText("/etc/os-release"), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		parts := strings.SplitN(line, "=", 2)
		if len(parts) != 2 {
			continue
		}
		result[parts[0]] = strings.Trim(parts[1], `"`)
	}
	return result
}

func readUserShell(username, uid string) string {
	for _, line := range strings.Split(readText("/etc/passwd"), "\n") {
		parts := strings.Split(line, ":")
		if len(parts) < 7 {
			continue
		}
		if parts[0] == username || parts[2] == uid {
			return parts[6]
		}
	}
	return ""
}

func readBootTime() string {
	for _, line := range strings.Split(readText("/proc/stat"), "\n") {
		if !strings.HasPrefix(line, "btime ") {
			continue
		}
		parts := strings.Fields(line)
		if len(parts) < 2 {
			continue
		}
		seconds, err := strconv.ParseInt(parts[1], 10, 64)
		if err != nil || seconds <= 0 {
			continue
		}
		return time.Unix(seconds, 0).Format(time.RFC3339)
	}
	uptime := readUptime()
	if uptime <= 0 {
		return ""
	}
	return time.Now().Add(-time.Duration(uptime) * time.Second).Format(time.RFC3339)
}

func readVirtualization() string {
	if value := strings.TrimSpace(commandOutput("systemd-detect-virt", "--vm")); value != "" {
		return value
	}
	return ""
}

func readContainer() string {
	if value := strings.TrimSpace(commandOutput("systemd-detect-virt", "--container")); value != "" {
		return value
	}
	text := readText("/proc/1/cgroup")
	for _, marker := range []string{"docker", "kubepods", "containerd", "lxc", "podman"} {
		if strings.Contains(text, marker) {
			return marker
		}
	}
	if _, err := os.Stat("/.dockerenv"); err == nil {
		return "docker"
	}
	return ""
}

func readDMI(name string) string {
	value := cleanDMIValue(readText("/sys/devices/virtual/dmi/id/" + name))
	if value != "" {
		return value
	}
	return cleanDMIValue(readText("/sys/class/dmi/id/" + name))
}

func readChassisType() string {
	value := readDMI("chassis_type")
	code, err := strconv.Atoi(value)
	if err != nil {
		return value
	}
	if label := chassisTypeLabel(code); label != "" {
		return fmt.Sprintf("%s (%d)", label, code)
	}
	return value
}

func cleanDMIValue(value string) string {
	value = strings.TrimSpace(strings.ReplaceAll(value, "\x00", ""))
	if value == "" {
		return ""
	}
	normalized := strings.ToLower(value)
	placeholders := []string{
		"not specified",
		"to be filled by o.e.m.",
		"to be filled by oem",
		"none",
		"unknown",
		"system product name",
		"system manufacturer",
		"base board product name",
	}
	for _, placeholder := range placeholders {
		if normalized == placeholder {
			return ""
		}
	}
	return value
}

func cleanHardwareValue(value string) string {
	value = strings.TrimSpace(strings.ReplaceAll(value, "\x00", ""))
	if value == "" {
		return ""
	}
	normalized := strings.ToLower(value)
	if normalized == "(null)" || normalized == "unknown" || normalized == "n/a" || normalized == "not available" {
		return ""
	}
	return value
}

func parseIntValue(value string) int {
	value = strings.TrimSpace(value)
	if value == "" {
		return 0
	}
	fields := strings.Fields(value)
	if len(fields) > 0 {
		value = fields[0]
	}
	value = strings.TrimSuffix(value, ",")
	parsed, err := strconv.Atoi(value)
	if err != nil {
		return 0
	}
	return parsed
}

func sortedKeys(values map[string]bool) []string {
	var result []string
	for value := range values {
		if strings.TrimSpace(value) != "" {
			result = append(result, value)
		}
	}
	sort.Strings(result)
	return result
}

func formatPCIID(vendorID, deviceID string) string {
	vendorID = cleanHardwareValue(vendorID)
	deviceID = cleanHardwareValue(deviceID)
	if vendorID == "" || deviceID == "" {
		return ""
	}
	return vendorID + ":" + deviceID
}

func pciClassLabel(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	lower := strings.ToLower(value)
	if strings.HasPrefix(lower, "0x03") {
		return "Display controller"
	}
	if strings.HasPrefix(lower, "0x02") {
		return "Network controller"
	}
	if strings.HasPrefix(lower, "0x01") {
		return "Mass storage controller"
	}
	if strings.HasPrefix(lower, "0x04") {
		return "Multimedia controller"
	}
	if strings.HasPrefix(lower, "0x0c") {
		return "Serial bus controller"
	}
	return cleanHardwareValue(value)
}

func isGPUPCIClass(class string) bool {
	class = strings.ToLower(class)
	return strings.Contains(class, "vga") ||
		strings.Contains(class, "display") ||
		strings.Contains(class, "3d controller") ||
		strings.HasPrefix(class, "0x03")
}

func chassisTypeLabel(code int) string {
	labels := map[int]string{
		1:  "Other",
		2:  "Unknown",
		3:  "Desktop",
		4:  "Low Profile Desktop",
		5:  "Pizza Box",
		6:  "Mini Tower",
		7:  "Tower",
		8:  "Portable",
		9:  "Laptop",
		10: "Notebook",
		11: "Hand Held",
		12: "Docking Station",
		13: "All In One",
		14: "Sub Notebook",
		15: "Space Saving",
		16: "Lunch Box",
		17: "Main Server Chassis",
		18: "Expansion Chassis",
		19: "Sub Chassis",
		20: "Bus Expansion Chassis",
		21: "Peripheral Chassis",
		22: "RAID Chassis",
		23: "Rack Mount Chassis",
		24: "Sealed-case PC",
		25: "Multi-system Chassis",
		26: "Compact PCI",
		27: "Advanced TCA",
		28: "Blade",
		29: "Blade Enclosure",
		30: "Tablet",
		31: "Convertible",
		32: "Detachable",
		33: "IoT Gateway",
		34: "Embedded PC",
		35: "Mini PC",
		36: "Stick PC",
	}
	return labels[code]
}

func formatUTCOffset(offsetSeconds int) string {
	sign := "+"
	if offsetSeconds < 0 {
		sign = "-"
		offsetSeconds = -offsetSeconds
	}
	hours := offsetSeconds / 3600
	minutes := (offsetSeconds % 3600) / 60
	return fmt.Sprintf("UTC%s%02d:%02d", sign, hours, minutes)
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

type cpuInfo struct {
	model    string
	speedMHz int
}

func readCPUInfo() cpuInfo {
	text := readText("/proc/cpuinfo")
	var result cpuInfo
	var speeds []int
	for _, line := range strings.Split(text, "\n") {
		if strings.HasPrefix(line, "model name") && result.model == "" {
			parts := strings.SplitN(line, ":", 2)
			if len(parts) == 2 {
				result.model = strings.TrimSpace(parts[1])
			}
		}
		if strings.HasPrefix(line, "cpu MHz") {
			parts := strings.SplitN(line, ":", 2)
			if len(parts) == 2 {
				value, _ := strconv.ParseFloat(strings.TrimSpace(parts[1]), 64)
				if value > 0 {
					speeds = append(speeds, int(math.Round(value)))
				}
			}
		}
	}
	if result.model == "" {
		result.model = "unknown"
	}
	if len(speeds) > 0 {
		sum := 0
		for _, speed := range speeds {
			sum += speed
		}
		result.speedMHz = sum / len(speeds)
	}
	return result
}

func readLoadAvg() []float64 {
	parts := strings.Fields(readText("/proc/loadavg"))
	if len(parts) < 3 {
		return nil
	}
	load := []float64{0, 0, 0}
	for i := 0; i < len(parts) && i < 3; i++ {
		value, err := strconv.ParseFloat(parts[i], 64)
		if err != nil || value < 0 || math.IsNaN(value) || math.IsInf(value, 0) {
			return nil
		}
		load[i] = value
	}
	return load
}

func readUptime() float64 {
	parts := strings.Fields(readText("/proc/uptime"))
	if len(parts) == 0 {
		return 0
	}
	value, _ := strconv.ParseFloat(parts[0], 64)
	return value
}

func readAddresses() []Address {
	if out := commandOutput("ip", "-o", "-4", "addr", "show", "scope", "global"); out != "" {
		var result []Address
		for _, line := range strings.Split(strings.TrimSpace(out), "\n") {
			parts := strings.Fields(line)
			if len(parts) < 4 {
				continue
			}
			address := strings.SplitN(parts[3], "/", 2)[0]
			if address == "" {
				continue
			}
			result = append(result, Address{Name: parts[1], Address: address})
			if len(result) >= 8 {
				return result
			}
		}
		if len(result) > 0 {
			return result
		}
	}

	var result []Address
	ifaces, _ := net.Interfaces()
	for _, iface := range ifaces {
		addrs, _ := iface.Addrs()
		for _, addr := range addrs {
			ipNet, ok := addr.(*net.IPNet)
			if !ok {
				continue
			}
			ip := ipNet.IP.To4()
			if ip == nil || ip.IsLoopback() {
				continue
			}
			result = append(result, Address{Name: iface.Name, Address: ip.String()})
			if len(result) >= 8 {
				return result
			}
		}
	}
	return result
}

type socketEntry struct {
	Protocol string
	Address  string
	Port     int
	State    string
	Inode    string
}

type socketProcess struct {
	PID     int
	Name    string
	Command string
	User    string
}

func readNetworkInterfaceDetails(name string) NetInterface {
	base := "/sys/class/net/" + name
	item := NetInterface{
		Name:   name,
		Kind:   "virtual",
		State:  strings.TrimSpace(readText(base + "/operstate")),
		Duplex: cleanHardwareValue(readText(base + "/duplex")),
		MTU:    parseIntValue(readText(base + "/mtu")),
	}
	if speed := parseIntValue(readText(base + "/speed")); speed > 0 {
		item.SpeedMbps = speed
	}
	if devicePath, err := filepath.EvalSymlinks(base + "/device"); err == nil && devicePath != "" && !strings.Contains(devicePath, "/devices/virtual/") {
		item.Kind = "physical"
		if driverPath, err := filepath.EvalSymlinks(base + "/device/driver"); err == nil {
			item.Driver = cleanHardwareValue(filepath.Base(driverPath))
		}
	} else if interfaceIsUp(name) {
		item.Kind = "virtual-active"
	}
	return item
}

func readSocketEntries(path, protocol string, tcp bool, stateMap map[string]string) []socketEntry {
	text := strings.TrimSpace(readText(path))
	if text == "" {
		return nil
	}
	var result []socketEntry
	lines := strings.Split(text, "\n")
	for _, line := range lines[1:] {
		parts := strings.Fields(line)
		if len(parts) < 10 {
			continue
		}
		address, port := parseSocketLocal(parts[1], protocol)
		if port <= 0 {
			continue
		}
		state := "bound"
		// long: connected UDP socket 不是监听入口，只把 UNCONN(07) 计入端口占用。
		if !tcp && parts[3] != "07" {
			continue
		}
		if tcp {
			state = stateMap[parts[3]]
			if state == "" {
				state = parts[3]
			}
		}
		result = append(result, socketEntry{
			Protocol: protocol,
			Address:  address,
			Port:     port,
			State:    state,
			Inode:    parts[9],
		})
	}
	return result
}

func readSocketProcessMap(entries []socketEntry) map[string]socketProcess {
	wanted := map[string]bool{}
	for _, entry := range entries {
		if entry.Inode != "" && entry.Inode != "0" {
			wanted[entry.Inode] = true
		}
	}
	if len(wanted) == 0 {
		return nil
	}
	result := map[string]socketProcess{}
	procEntries, _ := os.ReadDir("/proc")
	for _, procEntry := range procEntries {
		if len(result) >= len(wanted) {
			break
		}
		if !procEntry.IsDir() || !isDigits(procEntry.Name()) {
			continue
		}
		pid := procEntry.Name()
		fdEntries, err := os.ReadDir("/proc/" + pid + "/fd")
		if err != nil {
			continue
		}
		var process *socketProcess
		for _, fd := range fdEntries {
			if len(result) >= len(wanted) {
				break
			}
			target, err := os.Readlink("/proc/" + pid + "/fd/" + fd.Name())
			if err != nil {
				continue
			}
			inode := parseSocketInode(target)
			if !wanted[inode] {
				continue
			}
			if process == nil {
				info := readSocketProcess(pid)
				process = &info
			}
			result[inode] = *process
		}
	}
	return result
}

func readSocketProcess(pid string) socketProcess {
	status := readText("/proc/" + pid + "/status")
	name := matchStatus(status, "Name")
	uid := matchStatus(status, "Uid")
	cmd := strings.TrimSpace(strings.ReplaceAll(readText("/proc/"+pid+"/cmdline"), "\x00", " "))
	pidNum, _ := strconv.Atoi(pid)
	return socketProcess{
		PID:     pidNum,
		Name:    name,
		Command: cmd,
		User:    usernameForUID(uid),
	}
}

func parseSocketInode(target string) string {
	if !strings.HasPrefix(target, "socket:[") || !strings.HasSuffix(target, "]") {
		return ""
	}
	return strings.TrimSuffix(strings.TrimPrefix(target, "socket:["), "]")
}

func parseSocketLocal(value, protocol string) (string, int) {
	parts := strings.Split(value, ":")
	if len(parts) != 2 {
		return "", 0
	}
	portValue, err := strconv.ParseInt(parts[1], 16, 32)
	if err != nil {
		return "", 0
	}
	if strings.HasSuffix(protocol, "6") {
		return parseIPv6Hex(parts[0]), int(portValue)
	}
	return parseIPv4Hex(parts[0]), int(portValue)
}

func parseIPv4Hex(value string) string {
	if len(value) != 8 {
		return value
	}
	bytes := make([]byte, 4)
	for i := 0; i < 4; i++ {
		parsed, err := strconv.ParseUint(value[(3-i)*2:(4-i)*2], 16, 8)
		if err != nil {
			return value
		}
		bytes[i] = byte(parsed)
	}
	return net.IPv4(bytes[0], bytes[1], bytes[2], bytes[3]).String()
}

func parseIPv6Hex(value string) string {
	if len(value) != 32 {
		return value
	}
	bytes := make([]byte, 16)
	for word := 0; word < 4; word++ {
		base := word * 8
		for index := 0; index < 4; index++ {
			start := base + (3-index)*2
			parsed, err := strconv.ParseUint(value[start:start+2], 16, 8)
			if err != nil {
				return value
			}
			bytes[word*4+index] = byte(parsed)
		}
	}
	return net.IP(bytes).String()
}

func usernameForUID(uid string) string {
	uid = strings.TrimSpace(uid)
	if uid == "" {
		return ""
	}
	userLookupMu.Lock()
	if cached, ok := userLookupCache[uid]; ok {
		userLookupMu.Unlock()
		return cached
	}
	userLookupMu.Unlock()

	value := uid
	if account, err := user.LookupId(uid); err == nil && account.Username != "" {
		value = account.Username
	}

	userLookupMu.Lock()
	userLookupCache[uid] = value
	userLookupMu.Unlock()
	return value
}

func parseProcTicks(stat string) uint64 {
	end := strings.LastIndex(stat, ") ")
	if end < 0 {
		return 0
	}
	fields := strings.Fields(stat[end+2:])
	if len(fields) < 13 {
		return 0
	}
	return parseUint(fields[11]) + parseUint(fields[12])
}

func matchStatus(status, key string) string {
	prefix := key + ":"
	scanner := bufio.NewScanner(strings.NewReader(status))
	for scanner.Scan() {
		line := scanner.Text()
		if strings.HasPrefix(line, prefix) {
			value := strings.TrimSpace(strings.TrimPrefix(line, prefix))
			fields := strings.Fields(value)
			if len(fields) == 0 {
				return ""
			}
			return fields[0]
		}
	}
	return ""
}

func stringFirst(value string) string {
	if value == "" {
		return "?"
	}
	return value[:1]
}

func writeJSON(w http.ResponseWriter, value any) {
	payload, _ := json.Marshal(value)
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	_, _ = w.Write(payload)
}

func readText(path string) string {
	data, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	return string(data)
}

func commandOutput(name string, args ...string) string {
	out, err := exec.Command(name, args...).Output()
	if err != nil {
		return ""
	}
	return string(out)
}

func envString(key, fallback string) string {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return fallback
	}
	return value
}

func envInt(key string, fallback int) int {
	value, err := strconv.Atoi(strings.TrimSpace(os.Getenv(key)))
	if err != nil || value <= 0 {
		return fallback
	}
	return value
}

func parseUint(value string) uint64 {
	parsed, _ := strconv.ParseUint(strings.TrimSpace(value), 10, 64)
	return parsed
}

func percent(used, total uint64) float64 {
	if total == 0 {
		return 0
	}
	return clamp(float64(used) / float64(total) * 100)
}

func clamp(value float64) float64 {
	if math.IsNaN(value) || math.IsInf(value, 0) {
		return 0
	}
	if value < 0 {
		return 0
	}
	if value > 100 {
		return 100
	}
	return value
}

func delta(current, previous uint64) uint64 {
	if current < previous {
		return 0
	}
	return current - previous
}

func first(values []float64) float64 {
	if len(values) == 0 {
		return 0
	}
	return values[0]
}

func isDigits(value string) bool {
	if value == "" {
		return false
	}
	for _, ch := range value {
		if ch < '0' || ch > '9' {
			return false
		}
	}
	return true
}

func isDiskDevice(name string) bool {
	if strings.HasPrefix(name, "dm-") {
		return isDigits(strings.TrimPrefix(name, "dm-"))
	}
	if strings.HasPrefix(name, "md") {
		return isDigits(strings.TrimPrefix(name, "md"))
	}
	if strings.HasPrefix(name, "nvme") {
		parts := strings.Split(name, "p")
		return len(parts) == 1 && strings.Contains(name, "n")
	}
	if strings.HasPrefix(name, "mmcblk") {
		return isDigits(strings.TrimPrefix(name, "mmcblk"))
	}
	if strings.HasPrefix(name, "sd") || strings.HasPrefix(name, "vd") || strings.HasPrefix(name, "xvd") {
		last := name[len(name)-1]
		return last < '0' || last > '9'
	}
	return false
}

func selectAggregateDiskDevices(devices map[string]DiskDevice) []DiskDevice {
	var physical []DiskDevice
	var fallback []DiskDevice
	for _, device := range devices {
		fallback = append(fallback, device)
		if isPhysicalDiskDevice(device.Name) {
			physical = append(physical, device)
		}
	}
	if len(physical) > 0 {
		return physical
	}
	return fallback
}

func isPhysicalDiskDevice(name string) bool {
	if strings.HasPrefix(name, "nvme") {
		return strings.Contains(name, "n") && !strings.Contains(name, "p")
	}
	if strings.HasPrefix(name, "mmcblk") {
		return isDigits(strings.TrimPrefix(name, "mmcblk"))
	}
	if strings.HasPrefix(name, "sd") || strings.HasPrefix(name, "vd") || strings.HasPrefix(name, "xvd") {
		last := name[len(name)-1]
		return last < '0' || last > '9'
	}
	return false
}

func isThermalZoneName(name string) bool {
	return strings.HasPrefix(name, "thermal_zone") && isDigits(strings.TrimPrefix(name, "thermal_zone"))
}

func isCPUTemperatureType(kind string) bool {
	switch strings.ToLower(kind) {
	case "x86_pkg_temp", "coretemp", "k10temp", "cpu_thermal", "cpu-thermal", "soc_thermal":
		return true
	default:
		return false
	}
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}
