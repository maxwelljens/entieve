package game

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/ebitenui/ebitenui/widget"
	"github.com/entieve/entieve/uikit/theme"
	"github.com/entieve/entieve/uikit/widgets"
	"github.com/hajimehoshi/ebiten/v2"
)

// The redesigned keybindings window: sections derived from the action
// tables, chip states, unbind (Backspace), per-row reset and the scroll
// reachability of the whole list.

// keybinds returns the mounted keybindings panel on host (in-game or
// title — the same component).
func keybinds(t *testing.T, host *ScreenHost) *keybindPanel {
	t.Helper()
	p := bodyOf[keybindPanel](host, idKeybindings)
	if p.Keybindings == nil {
		t.Fatal("keybindings panel not mounted")
	}
	return &p
}

// TestKeybindSections: the window's sections derive from the declaration
// tables — every rebindable action appears exactly once under its table's
// header, and the fixed chord layer is complete, dim and informational.
func TestKeybindSections(t *testing.T) {
	g := newShowcaseGameAt(t, filepath.Join(t.TempDir(), "save.json"))
	g.applySize(1280, 720)
	g.host.SetOpen(idKeybindings, true)
	pump(t, g, 1280, 720)

	var groups []string
	seen := map[string]int{}
	for _, r := range keybinds(t, g.host).ModelForTest() {
		if len(groups) == 0 || groups[len(groups)-1] != r.Group {
			groups = append(groups, r.Group)
		}
		seen[r.Action]++
		if r.Fixed {
			continue
		}
		if r.Group != groupShip && r.Group != groupWin {
			t.Errorf("rebindable %q under group %q", r.Action, r.Group)
		}
	}
	if want := []string{groupShip, groupWin, groupFixed}; !slices.Equal(groups, want) {
		t.Fatalf("sections = %v, want %v", groups, want)
	}

	for _, a := range keyActions {
		if seen[a.ID] != 1 {
			t.Errorf("action %q listed %d times, want exactly once", a.ID, seen[a.ID])
		}
	}
	for id := range seen {
		if _, ok := defaultBinds[id]; ok {
			continue
		}
		if !fixedActions[id] {
			t.Errorf("row %q is neither rebindable nor a fixed action", id)
		}
	}
	// The fixed layer is complete: every fixed action derives a row.
	fixed := map[string]bool{}
	for _, r := range keybinds(t, g.host).ModelForTest() {
		if r.Fixed {
			fixed[r.Action] = true
			if r.Key == "" {
				t.Errorf("fixed %q shows no key", r.Action)
			}
		}
	}
	for id := range fixedActions {
		if !fixed[id] {
			t.Errorf("fixed action %q missing from the fixed section", id)
		}
	}
	// The fixed chips show the documented chord, modifiers and the
	// time-acceleration key pairs included (not the bare ebiten key name).
	wantKeys := map[string]string{"save": "Ctrl+S", "market-search": "Ctrl+F", "menu": "Esc", "time-slower": "[ · -"}
	for _, r := range keybinds(t, g.host).ModelForTest() {
		if r.Fixed && wantKeys[r.Action] != "" && r.Key != wantKeys[r.Action] {
			t.Errorf("fixed %q chip = %q, want %q", r.Action, r.Key, wantKeys[r.Action])
		}
	}
}

// TestKeybindChipStates: the chip classifies each row — default, rebound,
// unbound, capturing — and a rebound row offers its Reset.
func TestKeybindChipStates(t *testing.T) {
	g := newShowcaseGameAt(t, filepath.Join(t.TempDir(), "save.json"))
	g.applySize(1280, 720)
	g.host.SetOpen(idKeybindings, true)
	pump(t, g, 1280, 720)
	kb := keybinds(t, g.host)
	pump(t, g, 1280, 720) // one Sync writes the shipped state

	if text, state, ok := kb.ChipForTest("inventory"); !ok || text != "I" || state != "default" {
		t.Fatalf("default chip = %q/%q (ok=%v), want I/default", text, state, ok)
	}
	if kb.ResetVisibleForTest("inventory") {
		t.Error("a default row offers Reset")
	}

	g.host.services.Keys.Begin("warp") // takes S from stop (replace policy)
	g.handleKeys(Input{JustPressed: []ebiten.Key{ebiten.KeyS}})
	pump(t, g, 1280, 720)
	if text, state, _ := kb.ChipForTest("warp"); text != "S" || state != "rebound" {
		t.Errorf("rebound chip = %q/%q, want S/rebound", text, state)
	}
	if !kb.ResetVisibleForTest("warp") {
		t.Error("a rebound row offers no Reset")
	}
	if text, state, _ := kb.ChipForTest("stop"); text != "—" || state != "unbound" {
		t.Errorf("stolen chip = %q/%q, want an unbound dash", text, state)
	}
	if !kb.ResetVisibleForTest("stop") {
		t.Error("an unbound row offers no Reset (the only way back to its default)")
	}

	g.host.services.Keys.Begin("mine")
	pump(t, g, 1280, 720)
	if text, state, _ := kb.ChipForTest("mine"); !strings.Contains(text, "press a key") || state != "capturing" {
		t.Errorf("capturing chip = %q/%q", text, state)
	}
}

// TestKeybindUnbindRoundTrip: Backspace while capturing leaves the action
// unbound, the sidecar stores the explicit null, a reload keeps it, and
// the action triggers nothing until rebound.
func TestKeybindUnbindRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "save.json")
	g := newShowcaseGameAt(t, path)
	g.applySize(1280, 720)
	g.host.SetOpen(idKeybindings, true)
	pump(t, g, 1280, 720)

	g.host.services.Keys.Begin("inventory")
	g.handleKeys(Input{JustPressed: []ebiten.Key{ebiten.KeyBackspace}})
	if _, ok := g.binds["inventory"]; ok {
		t.Fatal("Backspace left the action bound")
	}
	data, err := os.ReadFile(g.keysPath())
	if err != nil {
		t.Fatalf("sidecar not written: %v", err)
	}
	var f keysFile
	if err := json.Unmarshal(data, &f); err != nil {
		t.Fatalf("sidecar JSON: %v", err)
	}
	if kb, ok := f.Binds["inventory"]; !ok || kb != nil {
		t.Errorf("stored inventory = %+v (present=%v), want an explicit null", kb, ok)
	}
	pump(t, g, 1280, 720)
	if text, state, _ := keybinds(t, g.host).ChipForTest("inventory"); text != "—" || state != "unbound" {
		t.Errorf("unbound chip = %q/%q, want the em dash", text, state)
	}

	g2 := newShowcaseGameAt(t, path)
	if _, ok := g2.binds["inventory"]; ok {
		t.Fatal("the reload rebound the action the sidecar left null")
	}
	g2.handleKeys(Input{JustPressed: []ebiten.Key{ebiten.KeyI}})
	if g2.host.IsOpen(idInventory) {
		t.Error("I still opened inventory after it was unbound")
	}
	// The row's Reset is the way back from an unbound action.
	g2.host.services.Keys.ResetRow("inventory")
	if k, ok := liveKey(g2, "inventory"); !ok || k != ebiten.KeyI {
		t.Fatalf("reset of the unbound row = %v (ok=%v), want the default I", k, ok)
	}
	g2.handleKeys(Input{JustPressed: []ebiten.Key{ebiten.KeyI}})
	if !g2.host.IsOpen(idInventory) {
		t.Error("I did not drive inventory after the row reset")
	}
}

// TestKeybindRowReset: one row's Reset restores exactly that action's
// default (and takes the key back from whoever holds it).
func TestKeybindRowReset(t *testing.T) {
	path := filepath.Join(t.TempDir(), "save.json")
	g := newShowcaseGameAt(t, path)
	g.applySize(1280, 720)
	g.host.SetOpen(idKeybindings, true)

	g.host.services.Keys.Begin("warp")
	g.handleKeys(Input{JustPressed: []ebiten.Key{ebiten.KeyS}})
	if _, ok := g.binds["stop"]; ok {
		t.Fatal("fixture: warp did not steal S from stop")
	}
	g.host.services.Keys.ResetRow("warp")
	if k, ok := liveKey(g, "warp"); !ok || k != ebiten.KeyZ {
		t.Errorf("reset warp = %v (ok=%v), want the default Z", k, ok)
	}
	// The theft is the stolen action's own state to fix: stop stays
	// unbound until its own Reset, exactly like a manual rebind.
	if _, ok := liveKey(g, "stop"); ok {
		t.Error("resetting warp restored stop (a theft is not undone by the thief)")
	}
	g.host.services.Keys.ResetRow("stop")
	if k, ok := liveKey(g, "stop"); !ok || k != ebiten.KeyS {
		t.Errorf("reset stop = %v (ok=%v), want its S back", k, ok)
	}
	pump(t, g, 1280, 720)
	if kb := keybinds(t, g.host); kb.ResetVisibleForTest("warp") || kb.ResetVisibleForTest("stop") {
		t.Error("a reset row still offers Reset")
	}
	if _, err := os.Stat(g.keysPath()); !os.IsNotExist(err) {
		t.Errorf("reset rows left the sidecar (all bindings are defaults): %v", err)
	}
}

// TestKeybindScrollReach: at every supported UI scale the whole list —
// every section and the last action — is reachable by scrolling.
func TestKeybindScrollReach(t *testing.T) {
	for _, sc := range []float64{0.75, 1, 2.5} {
		sc := sc
		t.Run(strings.ReplaceAll(fmt.Sprint(sc), ".", "_"), func(t *testing.T) {
			theme.SetScale(sc)
			t.Cleanup(func() { theme.SetScale(1) })
			g := newTitleGameAt(t, filepath.Join(t.TempDir(), "save.json"))
			w, h := int(1280*sc), int(720*sc)
			g.applySize(w, h)
			g.titleUI.openID(idKeybindings)
			pump(t, g, w, h)

			kb := keybinds(t, g.titleUI.host)
			scroll := kb.ScrollForTest()
			content := widgets.ScrollContentOf(scroll)
			page, ok := content.(widget.Containerer)
			if !ok {
				t.Fatal("no scroll content container")
			}
			if len(page.Children()) < len(keyActions)+len(fixedKeyRows()) {
				t.Fatalf("page holds %d widgets, fewer than the %d rows", len(page.Children()), len(keyActions)+len(fixedKeyRows()))
			}
			scroll.ScrollTop = 1
			pump(t, g, w, h)
			view := scroll.ViewRect()
			if view.Empty() {
				t.Fatal("empty viewport")
			}
			pr := content.GetWidget().Rect
			if pr.Max.Y > view.Max.Y+theme.Px(1) {
				t.Errorf("full scroll leaves the page bottom %d below the viewport %d", pr.Max.Y, view.Max.Y)
			} else if pr.Max.Y < view.Max.Y-theme.Px(1) {
				t.Errorf("full scroll ends %d above the viewport %d: content is not all reachable", view.Max.Y-pr.Max.Y, view.Max.Y)
			}
			if pr.Min.Y >= view.Min.Y {
				t.Errorf("the list did not scroll (page top %d, viewport top %d)", pr.Min.Y, view.Min.Y)
			}
			// Nothing sticks out to the right: the chips sit inside the
			// reserved scroll gutter, never under it.
			if pr.Max.X > view.Max.X+theme.Px(1) {
				t.Errorf("page is %d px wider than the viewport (chips clipped)", pr.Max.X-view.Max.X)
			}
			for i, kid := range page.Children() {
				if r := kid.GetWidget().Rect; r.Max.Y > view.Max.Y+theme.Px(1) {
					t.Errorf("widget %d (bottom %d) is unreachable below the viewport %d", i, r.Max.Y, view.Max.Y)
				}
			}
		})
	}
}
