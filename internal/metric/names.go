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
	DiskPercent = "disk_percent"
	Load1       = "load1"
	Load5       = "load5"
	Load15      = "load15"
	ThrottleRaw = "throttle_raw"
)

// Prefixes for dynamic metric keys (appended with ":" + identifier).
const (
	PrefixDiskPercent  = "disk_percent:"
	PrefixDiskRead     = "disk_read:"
	PrefixDiskWrite    = "disk_write:"
	PrefixTemp         = "temp:"
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
