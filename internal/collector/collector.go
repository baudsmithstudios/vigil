package collector

import (
	"time"

	"vigil/internal/checker"
)

// Snapshot holds all metric readings from a single collection tick.
type Snapshot struct {
	Timestamp   time.Time
	CPU         CPUSnapshot
	Memory      MemSnapshot
	Disks       []DiskSnapshot
	DiskIO      []DiskIOSnapshot
	Network     []NetSnapshot
	Load        LoadSnapshot
	Temperature []TempSnapshot
	Containers  []ContainerSnapshot
	Throttle    ThrottleSnapshot
	Mounts      []MountStatus
	Services    []checker.ServiceStatus
	UptimeSec   uint64 // system uptime in seconds
}

// CPUSnapshot holds per-collection CPU data.
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

// MemSnapshot holds memory and swap usage data.
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
}

// DiskSnapshot holds per-partition disk space data.
type DiskSnapshot struct {
	MountPoint string
	TotalBytes uint64
	UsedBytes  uint64
	FreeBytes  uint64
	Percent    float64
	Device     string
}

// DiskIOSnapshot holds per-device disk I/O throughput.
type DiskIOSnapshot struct {
	Device    string
	ReadRate  float64 // bytes/sec since last tick (zero on first tick)
	WriteRate float64
}

// NetSnapshot holds per-interface network counters.
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

// LoadSnapshot holds system load averages.
type LoadSnapshot struct {
	Load1  float64
	Load5  float64
	Load15 float64
}

// TempSnapshot holds a thermal sensor reading.
type TempSnapshot struct {
	SensorKey string
	Celsius   float64
}

// MountStatus holds the presence state of a watched mount point.
type MountStatus struct {
	Path     string // configured path
	Label    string // user-friendly name
	Mounted  bool   // present in /proc/mounts this tick
	Unstable bool   // flap detection triggered (set by alert handler)
}

// ContainerSnapshot holds per-container resource usage from the Docker API.
type ContainerSnapshot struct {
	Name       string  // container name (without leading slash)
	ID         string  // short container ID (12 chars)
	Status     string  // running, exited, restarting, paused, etc.
	CPUPercent float64 // CPU usage as a percentage of total host CPU
	MemUsed    uint64  // current memory usage in bytes
	MemLimit   uint64  // memory limit in bytes (0 if unlimited)
	MemPercent float64 // memory usage as percentage of limit
}
