package main

import (
	"strings"
	"testing"
)

func TestFormatBytes(t *testing.T) {
	tests := []struct {
		bytes uint64
		want  string
	}{
		{0, "0 B"},
		{1024, "1.0 KB"},
		{1536, "1.5 KB"},
		{1048576, "1.0 MB"},
		{1073741824, "1.0 GB"},
		{1099511627776, "1.0 TB"},
	}
	for _, tt := range tests {
		got := formatBytes(tt.bytes)
		if got != tt.want {
			t.Errorf("formatBytes(%d) = %q, want %q", tt.bytes, got, tt.want)
		}
	}
}

func TestPickerViewShowsSize(t *testing.T) {
	pm := pickerModel{
		mounts: []discoveredMount{
			{Device: "/dev/sda1", Path: "/mnt/data", FSType: "ext4", Size: 500 * 1073741824},
			{Device: "/dev/sdb1", Path: "/media/usb0", FSType: "vfat", Size: 16 * 1073741824},
		},
	}
	view := pm.View()
	if !strings.Contains(view, "500.0 GB") {
		t.Errorf("expected 500.0 GB in view, got:\n%s", view)
	}
	if !strings.Contains(view, "16.0 GB") {
		t.Errorf("expected 16.0 GB in view, got:\n%s", view)
	}
}
