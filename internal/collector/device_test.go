package collector

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestSDMMCBlockDevices(t *testing.T) {
	root := t.TempDir()
	block := filepath.Join(root, "block")
	if err := os.MkdirAll(block, 0o755); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"mmcblk0", "mmcblk1", "sda", "nvme0n1"} {
		if err := os.Mkdir(filepath.Join(block, name), 0o755); err != nil {
			t.Fatal(err)
		}
	}

	got := SDMMCBlockDevices(root)
	want := []string{"mmcblk0", "mmcblk1"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("SDMMCBlockDevices = %v, want %v", got, want)
	}
}

func TestSDMMCBlockDevices_MissingSysfs(t *testing.T) {
	got := SDMMCBlockDevices(filepath.Join(t.TempDir(), "does-not-exist"))
	if got != nil {
		t.Errorf("expected nil for missing sysfs root, got %v", got)
	}
}

func TestSDMMCBlockDevices_EmptyBlockDir(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "block"), 0o755); err != nil {
		t.Fatal(err)
	}
	if got := SDMMCBlockDevices(root); got != nil {
		t.Errorf("expected nil for empty block dir, got %v", got)
	}
}
