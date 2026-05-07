package main

import (
	"encoding/json"
	"io"
	"time"

	"vigil/internal/checker"
	"vigil/internal/collector"
	"vigil/internal/config"
)

type onceOutput struct {
	Timestamp   time.Time         `json:"timestamp"`
	CPU         onceCPU           `json:"cpu"`
	Memory      onceMemory        `json:"memory"`
	Disks       []onceDisk        `json:"disks"`
	DiskIO      []onceDiskIO      `json:"disk_io"`
	SDErrors    []onceSDError     `json:"sd_errors"`
	Network     []onceNetwork     `json:"network"`
	Load        onceLoad          `json:"load"`
	Temperature []onceTemperature `json:"temperature"`
	Containers  []onceContainer   `json:"containers"`
	Throttle    onceThrottle      `json:"throttle"`
	Mounts      []onceMount       `json:"mounts"`
	Services    []onceService     `json:"services"`
	UptimeSec   uint64            `json:"uptime_sec"`
}

type onceCPU struct {
	PercentPerCore []float64 `json:"percent_per_core"`
	PercentTotal   float64   `json:"percent_total"`
	UserPercent    float64   `json:"user_percent"`
	SystemPercent  float64   `json:"system_percent"`
	IOWaitPercent  float64   `json:"iowait_percent"`
	IdlePercent    float64   `json:"idle_percent"`
	Ready          bool      `json:"ready"`
}

type onceMemory struct {
	TotalBytes     uint64  `json:"total_bytes"`
	UsedBytes      uint64  `json:"used_bytes"`
	AvailableBytes uint64  `json:"available_bytes"`
	CachedBytes    uint64  `json:"cached_bytes"`
	BuffersBytes   uint64  `json:"buffers_bytes"`
	Percent        float64 `json:"percent"`
	SwapTotalBytes uint64  `json:"swap_total_bytes"`
	SwapUsedBytes  uint64  `json:"swap_used_bytes"`
	SwapPercent    float64 `json:"swap_percent"`
	SwapReady      bool    `json:"swap_ready"`
	SwapInBytes    uint64  `json:"swap_in_bytes"`
	SwapOutBytes   uint64  `json:"swap_out_bytes"`
	SwapInRate     float64 `json:"swap_in_rate"`
	SwapOutRate    float64 `json:"swap_out_rate"`
}

type onceDisk struct {
	MountPoint string  `json:"mount_point"`
	TotalBytes uint64  `json:"total_bytes"`
	UsedBytes  uint64  `json:"used_bytes"`
	FreeBytes  uint64  `json:"free_bytes"`
	Percent    float64 `json:"percent"`
	Device     string  `json:"device"`
}

type onceDiskIO struct {
	Device      string  `json:"device"`
	ReadRate    float64 `json:"read_rate"`
	WriteRate   float64 `json:"write_rate"`
	UtilPercent float64 `json:"util_percent"`
	LatencyMs   float64 `json:"latency_ms"`
}

type onceSDError struct {
	Host  string `json:"host"`
	Delta uint64 `json:"delta"`
}

type onceNetwork struct {
	Interface string  `json:"interface"`
	BytesSent uint64  `json:"bytes_sent"`
	BytesRecv uint64  `json:"bytes_recv"`
	SendRate  float64 `json:"send_rate"`
	RecvRate  float64 `json:"recv_rate"`
	ErrIn     uint64  `json:"err_in"`
	ErrOut    uint64  `json:"err_out"`
	DropIn    uint64  `json:"drop_in"`
	DropOut   uint64  `json:"drop_out"`
	DropRate  float64 `json:"drop_rate"`
	ErrRate   float64 `json:"err_rate"`
}

type onceLoad struct {
	Load1  float64 `json:"load1"`
	Load5  float64 `json:"load5"`
	Load15 float64 `json:"load15"`
}

type onceTemperature struct {
	SensorKey string  `json:"sensor_key"`
	Celsius   float64 `json:"celsius"`
}

type onceContainer struct {
	Name       string  `json:"name"`
	ID         string  `json:"id"`
	Status     string  `json:"status"`
	CPUPercent float64 `json:"cpu_percent"`
	MemUsed    uint64  `json:"mem_used"`
	MemLimit   uint64  `json:"mem_limit"`
	MemPercent float64 `json:"mem_percent"`
}

type onceThrottle struct {
	Raw                    uint32 `json:"raw"`
	UnderVoltage           bool   `json:"under_voltage"`
	FreqCapped             bool   `json:"freq_capped"`
	Throttled              bool   `json:"throttled"`
	SoftTempLimit          bool   `json:"soft_temp_limit"`
	UnderVoltageSinceBoot  bool   `json:"under_voltage_since_boot"`
	FreqCappedSinceBoot    bool   `json:"freq_capped_since_boot"`
	ThrottledSinceBoot     bool   `json:"throttled_since_boot"`
	SoftTempLimitSinceBoot bool   `json:"soft_temp_limit_since_boot"`
	Available              bool   `json:"available"`
	Status                 string `json:"status"`
}

type onceMount struct {
	Path     string `json:"path"`
	Label    string `json:"label"`
	Mounted  bool   `json:"mounted"`
	Unstable bool   `json:"unstable"`
}

type onceService struct {
	Name       string    `json:"name"`
	CheckType  string    `json:"check_type"`
	Up         bool      `json:"up"`
	StatusCode int       `json:"status_code"`
	LatencyMs  int64     `json:"latency_ms"`
	CheckedAt  time.Time `json:"checked_at"`
	Error      string    `json:"error"`
}

func runOnceJSON(w io.Writer, cfg config.Config) error {
	var svcChecker *checker.ServiceChecker
	if len(cfg.HTTPChecks) > 0 || len(cfg.PortChecks) > 0 {
		svcChecker = checker.New(cfg.Services, cfg.HTTPChecks, cfg.PortChecks)
		svcChecker.CheckOnce()
	}

	runner, _ := collector.New(cfg.Interval.Duration, cfg.Docker.Socket, cfg.MountChecks, svcChecker)
	snap := runner.Collect(time.Now())

	enc := json.NewEncoder(w)
	return enc.Encode(onceOutputFromSnapshot(snap))
}

func onceOutputFromSnapshot(snap collector.Snapshot) onceOutput {
	out := onceOutput{
		Timestamp: snap.Timestamp,
		CPU: onceCPU{
			PercentPerCore: append(make([]float64, 0, len(snap.CPU.PercentPerCore)), snap.CPU.PercentPerCore...),
			PercentTotal:   snap.CPU.PercentTotal,
			UserPercent:    snap.CPU.UserPercent,
			SystemPercent:  snap.CPU.SystemPercent,
			IOWaitPercent:  snap.CPU.IOWaitPercent,
			IdlePercent:    snap.CPU.IdlePercent,
			Ready:          snap.CPU.Ready,
		},
		Memory: onceMemory{
			TotalBytes:     snap.Memory.TotalBytes,
			UsedBytes:      snap.Memory.UsedBytes,
			AvailableBytes: snap.Memory.AvailableBytes,
			CachedBytes:    snap.Memory.CachedBytes,
			BuffersBytes:   snap.Memory.BuffersBytes,
			Percent:        snap.Memory.Percent,
			SwapTotalBytes: snap.Memory.SwapTotalBytes,
			SwapUsedBytes:  snap.Memory.SwapUsedBytes,
			SwapPercent:    snap.Memory.SwapPercent,
			SwapReady:      snap.Memory.SwapReady,
			SwapInBytes:    snap.Memory.SwapInBytes,
			SwapOutBytes:   snap.Memory.SwapOutBytes,
			SwapInRate:     snap.Memory.SwapInRate,
			SwapOutRate:    snap.Memory.SwapOutRate,
		},
		Disks:       make([]onceDisk, 0, len(snap.Disks)),
		DiskIO:      make([]onceDiskIO, 0, len(snap.DiskIO)),
		SDErrors:    make([]onceSDError, 0, len(snap.SDErrors)),
		Network:     make([]onceNetwork, 0, len(snap.Network)),
		Load:        onceLoad{Load1: snap.Load.Load1, Load5: snap.Load.Load5, Load15: snap.Load.Load15},
		Temperature: make([]onceTemperature, 0, len(snap.Temperature)),
		Containers:  make([]onceContainer, 0, len(snap.Containers)),
		Throttle: onceThrottle{
			Raw:                    snap.Throttle.Raw,
			UnderVoltage:           snap.Throttle.UnderVoltage,
			FreqCapped:             snap.Throttle.FreqCapped,
			Throttled:              snap.Throttle.Throttled,
			SoftTempLimit:          snap.Throttle.SoftTempLimit,
			UnderVoltageSinceBoot:  snap.Throttle.UnderVoltageSinceBoot,
			FreqCappedSinceBoot:    snap.Throttle.FreqCappedSinceBoot,
			ThrottledSinceBoot:     snap.Throttle.ThrottledSinceBoot,
			SoftTempLimitSinceBoot: snap.Throttle.SoftTempLimitSinceBoot,
			Available:              snap.Throttle.Available,
			Status:                 snap.Throttle.Status(),
		},
		Mounts:    make([]onceMount, 0, len(snap.Mounts)),
		Services:  make([]onceService, 0, len(snap.Services)),
		UptimeSec: snap.UptimeSec,
	}

	for _, d := range snap.Disks {
		out.Disks = append(out.Disks, onceDisk{
			MountPoint: d.MountPoint,
			TotalBytes: d.TotalBytes,
			UsedBytes:  d.UsedBytes,
			FreeBytes:  d.FreeBytes,
			Percent:    d.Percent,
			Device:     d.Device,
		})
	}
	for _, d := range snap.DiskIO {
		out.DiskIO = append(out.DiskIO, onceDiskIO{
			Device:      d.Device,
			ReadRate:    d.ReadRate,
			WriteRate:   d.WriteRate,
			UtilPercent: d.UtilPercent,
			LatencyMs:   d.LatencyMs,
		})
	}
	for _, s := range snap.SDErrors {
		out.SDErrors = append(out.SDErrors, onceSDError{Host: s.Host, Delta: s.Delta})
	}
	for _, n := range snap.Network {
		out.Network = append(out.Network, onceNetwork{
			Interface: n.Interface,
			BytesSent: n.BytesSent,
			BytesRecv: n.BytesRecv,
			SendRate:  n.SendRate,
			RecvRate:  n.RecvRate,
			ErrIn:     n.ErrIn,
			ErrOut:    n.ErrOut,
			DropIn:    n.DropIn,
			DropOut:   n.DropOut,
			DropRate:  n.DropRate,
			ErrRate:   n.ErrRate,
		})
	}
	for _, t := range snap.Temperature {
		out.Temperature = append(out.Temperature, onceTemperature{
			SensorKey: t.SensorKey,
			Celsius:   t.Celsius,
		})
	}
	for _, c := range snap.Containers {
		out.Containers = append(out.Containers, onceContainer{
			Name:       c.Name,
			ID:         c.ID,
			Status:     c.Status,
			CPUPercent: c.CPUPercent,
			MemUsed:    c.MemUsed,
			MemLimit:   c.MemLimit,
			MemPercent: c.MemPercent,
		})
	}
	for _, m := range snap.Mounts {
		out.Mounts = append(out.Mounts, onceMount{
			Path:     m.Path,
			Label:    m.Label,
			Mounted:  m.Mounted,
			Unstable: m.Unstable,
		})
	}
	for _, s := range snap.Services {
		out.Services = append(out.Services, onceService{
			Name:       s.Name,
			CheckType:  s.CheckType,
			Up:         s.Up,
			StatusCode: s.StatusCode,
			LatencyMs:  s.Latency.Milliseconds(),
			CheckedAt:  s.CheckedAt,
			Error:      s.Error,
		})
	}
	return out
}
