package collector

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/shirou/gopsutil/v3/disk"
)

func TestApplyDiskRates_ComputesUtilAndLatency(t *testing.T) {
	prev := disk.IOCountersStat{
		ReadBytes:  100,
		WriteBytes: 200,
		ReadCount:  10,
		WriteCount: 20,
		ReadTime:   1000,
		WriteTime:  2000,
		IoTime:     500,
	}
	curr := disk.IOCountersStat{
		ReadBytes:  300,
		WriteBytes: 500,
		ReadCount:  20,
		WriteCount: 30,
		ReadTime:   1500,
		WriteTime:  2500,
		IoTime:     1300,
	}

	snap := DiskIOSnapshot{}
	applyDiskRates(&snap, prev, curr, 2.0)

	if snap.ReadRate != 100.0 {
		t.Errorf("expected ReadRate 100.0, got %f", snap.ReadRate)
	}
	if snap.WriteRate != 150.0 {
		t.Errorf("expected WriteRate 150.0, got %f", snap.WriteRate)
	}
	if snap.UtilPercent != 40.0 {
		t.Errorf("expected UtilPercent 40.0, got %f", snap.UtilPercent)
	}
	if snap.LatencyMs != 50.0 {
		t.Errorf("expected LatencyMs 50.0, got %f", snap.LatencyMs)
	}
}

func TestParentBlockDevice(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{name: "sda partition", in: "sda1", want: "sda"},
		{name: "mmc partition", in: "mmcblk0p2", want: "mmcblk0"},
		{name: "nvme partition", in: "nvme0n1p3", want: "nvme0n1"},
		{name: "base device", in: "sda", want: ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ParentBlockDevice(tt.in)
			if got != tt.want {
				t.Fatalf("ParentBlockDevice(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}

func TestNormalizeDeviceName_ResolvesSymlink(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "mmcblk0p2")
	link := filepath.Join(dir, "root")

	if err := os.WriteFile(target, []byte(""), 0644); err != nil {
		t.Fatalf("write target: %v", err)
	}
	if err := os.Symlink(target, link); err != nil {
		t.Fatalf("symlink: %v", err)
	}

	got := normalizeDeviceName(link)
	if got != "mmcblk0p2" {
		t.Fatalf("normalizeDeviceName(%q) = %q, want %q", link, got, "mmcblk0p2")
	}
}

func TestApplyDiskRates_RolloverProtected(t *testing.T) {
	prev := disk.IOCountersStat{
		ReadBytes:  1000,
		WriteBytes: 1000,
		ReadCount:  100,
		WriteCount: 100,
		ReadTime:   1000,
		WriteTime:  1000,
		IoTime:     1000,
	}
	curr := disk.IOCountersStat{
		ReadBytes:  1,
		WriteBytes: 1,
		ReadCount:  1,
		WriteCount: 1,
		ReadTime:   1,
		WriteTime:  1,
		IoTime:     1,
	}

	snap := DiskIOSnapshot{}
	applyDiskRates(&snap, prev, curr, 1.0)

	if snap.ReadRate != 0 {
		t.Errorf("expected ReadRate 0 on rollover, got %f", snap.ReadRate)
	}
	if snap.WriteRate != 0 {
		t.Errorf("expected WriteRate 0 on rollover, got %f", snap.WriteRate)
	}
	if snap.UtilPercent != 0 {
		t.Errorf("expected UtilPercent 0 on rollover, got %f", snap.UtilPercent)
	}
	if snap.LatencyMs != 0 {
		t.Errorf("expected LatencyMs 0 on rollover, got %f", snap.LatencyMs)
	}
}

func TestApplyDiskRates_UtilClampedTo100(t *testing.T) {
	prev := disk.IOCountersStat{IoTime: 0}
	curr := disk.IOCountersStat{IoTime: 3000}

	snap := DiskIOSnapshot{}
	applyDiskRates(&snap, prev, curr, 2.0)

	if snap.UtilPercent != 100.0 {
		t.Errorf("expected UtilPercent clamped at 100, got %f", snap.UtilPercent)
	}
}
