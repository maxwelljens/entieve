package game

import (
	"fmt"
	"slices"
	"strconv"
	"strings"
	"testing"
	"unicode"

	uikit "github.com/entieve/entieve/uikit/screens"
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
		"scanner", "market", "starmap", "help", "settings", "log", "menu", "time-slower", "time-faster",
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

// TestKeyMapScreenRowsDeriveFromSpec: the screen toggles in KeyMap are
// the Screen spec's rows — one binding per keyed row, its chord the one
// chordOf composes from the declared key and its description the spec's
// own text, no other binding claims a screen action, and the help overlay
// renders exactly those rows. A new keyed Screen shows up in the help
// window without touching this file.
func TestKeyMapScreenRowsDeriveFromSpec(t *testing.T) {
	derived := keyMapRows(screenBindings(screens))
	for _, b := range derived {
		if n := countBinding(KeyMap, b); n != 1 {
			t.Errorf("screen binding %+v appears %d times in KeyMap, want 1", b, n)
		}
	}
	screenActions := map[string]bool{}
	for _, s := range screens {
		if s.Key != keyNone {
			screenActions[s.Action] = true
		}
	}
	rows := 0
	for _, b := range KeyMap {
		if screenActions[b.Action] {
			rows++
		}
	}
	if rows != len(derived) {
		t.Errorf("KeyMap carries %d screen rows, want %d", rows, len(derived))
	}
	// The chord is composed and the description is the spec's text alone:
	// the help overlay renders both, nothing is parsed back out of a row.
	help := helpRows()
	for _, b := range derived {
		key, desc := chordOf(b.Key, b.Shift, b.Ctrl), descOf(b.Action)
		if strings.Contains(desc, " — ") {
			t.Errorf("spec description %q carries a chord", desc)
		}
		if !slices.Contains(help, uikit.KeyRow{Key: key, Desc: desc}) {
			t.Errorf("help rows lack the spec's row %q — %q", key, desc)
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

// TestHelpRangeNumbersMatchExportedArrays: no help text states a range
// number of its own. Q/Z act at the Selected Item selectors' current
// values, so their rows carry no digits at all (the stale numbers the
// merge removed must not come back), and the one range number the mouse
// table shows — the right-click orbit — is derived from
// uikit.OrbitRanges (scene.go flies at the same value).
func TestHelpRangeNumbersMatchExportedArrays(t *testing.T) {
	for _, action := range []string{"orbit", "warp"} {
		desc := ""
		for _, b := range KeyMap {
			if b.Action == action {
				desc = b.Desc
			}
		}
		if desc == "" {
			t.Fatalf("no KeyMap binding for action %q", action)
		}
		if strings.IndexFunc(desc, unicode.IsDigit) >= 0 {
			t.Errorf("%s: help text %q states a number; the key acts at the "+
				"selector's value, so no range may be hardcoded", action, desc)
		}
	}

	// Parse the number back out of the rendered mouse line: a hand-typed
	// hint that disagrees with the array fails here.
	const prefix, suffix = "Orbit target under cursor (", " m)"
	hint := ""
	for _, b := range MouseMap {
		if strings.HasPrefix(b.Desc, prefix) {
			hint = strings.TrimSuffix(strings.TrimPrefix(b.Desc, prefix), suffix)
		}
	}
	if hint == "" {
		t.Fatal("MouseMap has no right-click orbit line")
	}
	got, err := strconv.ParseFloat(strings.ReplaceAll(hint, ",", ""), 64)
	if err != nil {
		t.Fatalf("orbit hint %q is not a number: %v", hint, err)
	}
	if want := uikit.OrbitRanges[0]; got != want {
		t.Errorf("right-click orbit hint = %v m, want OrbitRanges[0] = %v m", got, want)
	}
	if orbitDefaultRange != uikit.OrbitRanges[0] {
		t.Errorf("orbitDefaultRange = %v, want OrbitRanges[0] = %v (the action and the help text share the array)",
			orbitDefaultRange, uikit.OrbitRanges[0])
	}
}

// TestMouseMapCoversNewGestures: the help overlay documents the context
// menu (including the module menu that carries Cycle charge) and
// overview wheel scrolling alongside the classic gestures.
func TestMouseMapCoversNewGestures(t *testing.T) {
	want := []string{
		"context menu",
		"Cycle charge",
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
