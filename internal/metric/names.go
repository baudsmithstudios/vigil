package metric

// Metric name constants used for persistence and alert evaluation.
const (
	CPUPercent  = "cpu_percent"
	CPUUser     = "cpu_user"
	CPUSystem   = "cpu_system"
	CPUIowait   = "cpu_iowait"
	CPUIdle     = "cpu_idle"
	MemPercent  = "mem_percent"
	SwapPercent = "swap_percent"
	SwapIn      = "swap_in"
	SwapOut     = "swap_out"
	DiskPercent = "disk_percent"
	DiskUtil    = "disk_util"
	DiskLatency = "disk_latency_ms"
	SDErrors    = "sd_errors"
	Load1       = "load1"
	Load5       = "load5"
	Load15      = "load15"
	ThrottleRaw = "throttle_raw"
)

// Prefixes for dynamic metric keys (appended with ":" + identifier).
const (
	PrefixDiskPercent   = "disk_percent:"
	PrefixDiskRead      = "disk_read:"
	PrefixDiskWrite     = "disk_write:"
	PrefixDiskUtil      = "disk_util:"
	PrefixDiskLatency   = "disk_latency_ms:"
	PrefixSDErrors      = "sd_errors:"
	PrefixTemp          = "temp:"
	PrefixMountMissing  = "mount_missing:"
	PrefixMountUnstable = "mount_unstable:"
	PrefixNetSend       = "net_send:"
	PrefixNetRecv       = "net_recv:"
	PrefixNetDrops      = "net_drops:"
	PrefixNetErrors     = "net_errors:"
)

// Alert name for throttle conditions (managed outside the alert engine).
const AlertThrottle = "throttle"

// Alert name prefix for service health checks (managed outside the alert engine).
const PrefixServiceDown = "service_down:"
