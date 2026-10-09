package game

import (
	"fmt"
	"strings"
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
)

// TestKeyMapUnique: no (Key, Shift, Ctrl) combination is bound to two
// actions — the conflict guarantee behind the documented key map.
func TestKeyMapUnique(t *testing.T) {
	type combo struct {
		key   string
		shift bool
		ctrl  bool
	}
	seen := map[combo]string{}
	for _, b := range KeyMap {
		c := combo{key: fmt.Sprint(b.Key), shift: b.Shift, ctrl: b.Ctrl}
		if prev, dup := seen[c]; dup {
			t.Errorf("key %s (shift=%v ctrl=%v) bound to both %q and %q",
				c.key, c.shift, c.ctrl, prev, b.Action)
		}
		seen[c] = b.Action
	}
}

// TestKeyMapWellFormed: every binding has an action id and help text, and
// the actions the game relies on exist exactly where documented.
func TestKeyMapWellFormed(t *testing.T) {
	required := []string{
		"lock-nearest", "lock-selected", "approach", "orbit", "stop", "align",
		"warp", "dock", "jump", "loot", "mine", "inventory", "fitting",
		"scanner", "market", "starmap", "help", "menu", "time-slower", "time-faster",
		"save", "load",
	}
	found := map[string]bool{}
	for i := 1; i <= 8; i++ {
		required = append(required, fmt.Sprintf("high-%d", i))
	}
	for _, b := range KeyMap {
		if b.Action == "" || b.Desc == "" {
			t.Errorf("binding %+v lacks action or description", b)
		}
		found[b.Action] = true
	}
	for _, a := range required {
		if !found[a] {
			t.Errorf("action %q missing from KeyMap", a)
		}
	}
}

// TestKeyMapConflictResolutions pins the documented decisions: F5 is a
// module slot (save moved to Ctrl+S), market is G (M stays mine).
func TestKeyMapConflictResolutions(t *testing.T) {
	find := func(k ebiten.Key, shift, ctrl bool) string {
		for _, b := range KeyMap {
			if b.Key == k && b.Shift == shift && b.Ctrl == ctrl {
				return b.Action
			}
		}
		return ""
	}
	if got := find(ebiten.KeyF5, false, false); got != "high-5" {
		t.Errorf("F5 = %q, want high-5 (save is Ctrl+S)", got)
	}
	if got := find(ebiten.KeyS, false, true); got != "save" {
		t.Errorf("Ctrl+S = %q, want save", got)
	}
	if got := find(ebiten.KeyM, false, false); got != "mine" {
		t.Errorf("M = %q, want mine", got)
	}
	if got := find(ebiten.KeyG, false, false); got != "market" {
		t.Errorf("G = %q, want market", got)
	}
	if got := find(ebiten.KeyF9, false, false); got != "load" {
		t.Errorf("F9 = %q, want load", got)
	}
	// The scanner toggle (added with the sensors/visibility pass).
	if got := find(ebiten.KeyC, false, false); got != "scanner" {
		t.Errorf("C = %q, want scanner", got)
	}
}

// TestMouseMapCoversNewGestures: the help overlay documents the ammo
// right-click and overview wheel scrolling alongside the classic gestures.
func TestMouseMapCoversNewGestures(t *testing.T) {
	want := []string{
		"cycle loaded charge",
		"scroll the list",
		"drag a title bar",
	}
	for _, w := range want {
		found := false
		for _, b := range MouseMap {
			if strings.Contains(b.Desc, w) {
				found = true
			}
		}
		if !found {
			t.Errorf("MouseMap missing a line containing %q", w)
		}
	}
}
