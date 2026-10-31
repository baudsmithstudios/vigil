package collector

import (
	"time"

	"vigil/internal/checker"
)

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
	UptimeSec        uint64
}

type CPUSnapshot struct {
	PercentPerCore []float64
	PercentTotal   float64
	UserPercent    float64
	SystemPercent  float64
	IOWaitPercent  float64
	IdlePercent    float64
	Ready          bool // false until the second tick; every other field is zero until then
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
	SwapInBytes    uint64 // cumulative since boot
	SwapOutBytes   uint64 // cumulative since boot
	SwapInRate     float64
	SwapOutRate    float64
}

type DiskSnapshot struct {
	MountPoint string
	TotalBytes uint64
	UsedBytes  uint64
	FreeBytes  uint64
	Percent    float64
	Device     string
}

type DiskIOSnapshot struct {
	Device      string
	ReadRate    float64 // bytes/sec since last tick (zero on first tick)
	WriteRate   float64
	UtilPercent float64 // percentage of wall time device was busy
	LatencyMs   float64 // average latency per completed IO over interval
}

type SDErrorSnapshot struct {
	Host  string
	Delta uint64 // errors since the previous tick
}

type NetSnapshot struct {
	Interface string
	BytesSent uint64
	BytesRecv uint64
	// Bytes/sec since last tick (zero on first tick)
	SendRate float64
	RecvRate float64
	// Cumulative
	ErrIn   uint64
	ErrOut  uint64
	DropIn  uint64
	DropOut uint64
	// In+out per second since last tick (zero on first tick)
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
	Path     string
	Label    string
	Mounted  bool
	Unstable bool // set by the alert handler
}

type ContainerSnapshot struct {
	Name       string
	ID         string
	Status     string
	CPUPercent float64 // 100 = one full core
	MemUsed    uint64
	MemLimit   uint64  // 0 if unlimited
	MemPercent float64 // of MemLimit
}
