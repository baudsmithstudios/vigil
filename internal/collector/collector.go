package collector

import (
	"time"

	"vigil/internal/checker"
)

// Snapshot is one collection tick's metrics across every subsystem.
type Snapshot struct {
	Timestamp        time.Time
	CPU              CPUSnapshot
	Memory           MemSnapshot
	Disks            []DiskSnapshot
	DiskIO           []DiskIOSnapshot
	SDErrors         []SDErrorSnapshot
	Network          []NetSnapshot
	Load             LoadSnapshot
	Temperature      []TempSnapshot
	Containers       []ContainerSnapshot
	Throttle         ThrottleSnapshot
	Mounts           []MountStatus
	Services         []checker.ServiceStatus
	ServiceCycleTime time.Time
	UptimeSec        uint64 // system uptime in seconds
}

type CPUSnapshot struct {
	PercentPerCore []float64 // empty until second tick
	PercentTotal   float64   // 0 until second tick
	// Time breakdown as percentages (zero until second tick)
	UserPercent   float64
	SystemPercent float64
	IOWaitPercent float64
	IdlePercent   float64
	Ready         bool // false until first real reading available
}

type MemSnapshot struct {
	TotalBytes     uint64
	UsedBytes      uint64
	AvailableBytes uint64
	CachedBytes    uint64
	BuffersBytes   uint64
	Percent        float64

	SwapTotalBytes uint64
	SwapUsedBytes  uint64
	SwapPercent    float64
	SwapReady      bool
	SwapInBytes    uint64 // cumulative bytes swapped in since boot
	SwapOutBytes   uint64 // cumulative bytes swapped out since boot
	SwapInRate     float64
	SwapOutRate    float64
}

// DiskSnapshot is per-partition space usage.
type DiskSnapshot struct {
	MountPoint string
	TotalBytes uint64
	UsedBytes  uint64
	FreeBytes  uint64
	Percent    float64
	Device     string
}

// DiskIOSnapshot is per-device throughput, utilization, and latency.
type DiskIOSnapshot struct {
	Device      string
	ReadRate    float64 // bytes/sec since last tick (zero on first tick)
	WriteRate   float64
	UtilPercent float64 // percentage of wall time device was busy
	LatencyMs   float64 // average latency per completed IO over interval
}

// SDErrorSnapshot is the per-host MMC error count delta since the previous tick.
type SDErrorSnapshot struct {
	Host  string
	Delta uint64 // count delta since previous tick
}

type NetSnapshot struct {
	Interface string
	BytesSent uint64
	BytesRecv uint64
	// Rates in bytes/sec since last tick (zero on first tick)
	SendRate float64
	RecvRate float64
	// Cumulative error and drop counts
	ErrIn   uint64
	ErrOut  uint64
	DropIn  uint64
	DropOut uint64
	// Combined (in+out) rates per second since last tick (zero on first tick)
	DropRate float64
	ErrRate  float64
}

type LoadSnapshot struct {
	Load1  float64
	Load5  float64
	Load15 float64
}

type TempSnapshot struct {
	SensorKey string
	Celsius   float64
}

type MountStatus struct {
	Path     string // configured path
	Label    string // user-friendly name
	Mounted  bool   // present in /proc/mounts this tick
	Unstable bool   // flap detection triggered (set by alert handler)
}

type ContainerSnapshot struct {
	Name       string  // container name (without leading slash)
	ID         string  // short container ID (12 chars)
	Status     string  // running, exited, restarting, paused, etc.
	CPUPercent float64 // CPU usage as a percentage of total host CPU
	MemUsed    uint64  // current memory usage in bytes
	MemLimit   uint64  // memory limit in bytes (0 if unlimited)
	MemPercent float64 // memory usage as percentage of limit
}
