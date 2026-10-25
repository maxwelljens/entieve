package main

import "testing"

// TestStartInTitle: `run` boots into the title screen, except when a save
// is loaded from the CLI (--load bypasses it).
func TestStartInTitle(t *testing.T) {
	if !startInTitle("") {
		t.Error("run without --load must start in the title")
	}
	if startInTitle("save.json") {
		t.Error("--load must bypass the title")
	}
}
