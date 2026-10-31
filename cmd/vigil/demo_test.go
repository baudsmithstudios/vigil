//go:build demo

package main

import (
	"path/filepath"
	"testing"
	"time"

	"vigil/internal/store"
)

func TestOpenDemoDBUsesThrowawayDir(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("TMPDIR", tmp) // os.TempDir() reads it, so the demo db lands somewhere this test owns
	pattern := filepath.Join(tmp, "vigil-demo-*", "vigil.db")

	db, cleanup, err := openDemoDB()
	if err != nil {
		t.Fatal(err)
	}
	if err := db.WriteAlert(store.AlertRecord{Name: "probe", Message: "probe", FiredAt: time.Now()}); err != nil {
		t.Fatal(err)
	}
	if found, _ := filepath.Glob(pattern); len(found) != 1 {
		t.Fatalf("found %v, want one demo db under %s", found, tmp)
	}
	cleanup()
	if left, _ := filepath.Glob(filepath.Join(tmp, "vigil-demo-*")); len(left) != 0 {
		t.Errorf("cleanup left %v", left)
	}
}
