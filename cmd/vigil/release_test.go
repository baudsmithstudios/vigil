//go:build !demo

package main

import (
	"flag"
	"testing"
)

func TestReleaseBuildHasNoDemo(t *testing.T) {
	if demo != nil {
		t.Error("release build sets the demo hook")
	}
	if flag.Lookup("demo") != nil {
		t.Error("release build defines --demo")
	}
}
