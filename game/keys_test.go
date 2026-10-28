package game

import (
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/entieve/entieve/sim"
	"github.com/hajimehoshi/ebiten/v2"
)

// The keybinding layer: defaults without a sidecar, capture/replace,
// persistence round-trips, the reserved fixed chords, reset-all and the
// sidecar's exclusion from the save manager.

// resolve looks a live action → key up for the assertions.
func liveKey(g *Game, action string) (ebiten.Key, bool) {
	b, ok := g.binds[action]
	return b.Key, ok
}

// TestKeybindDefaults: a session without a sidecar has exactly the
// declared defaults, writes nothing, and the fixed chord layer is not
// rebindable (it never reaches the table).
func TestKeybindDefaults(t *testing.T) {
	dir := t.TempDir()
	g := newShowcaseGameAt(t, filepath.Join(dir, "save.json"))

	if len(g.binds) != len(defaultBinds) {
		t.Fatalf("live binds = %d actions, want the %d defaults", len(g.binds), len(defaultBinds))
	}
	for _, a := range keyActions {
		if got, ok := g.binds[a.ID]; !ok || got != a.Def {
			t.Errorf("%s = %+v (ok=%v), want the default %+v", a.ID, got, ok, a.Def)
		}
	}
	for _, fixed := range []string{"menu", "save", "load", "market-search", "time-slower", "time-faster"} {
		if _, ok := defaultBinds[fixed]; ok {
			t.Errorf("fixed action %q is rebindable", fixed)
		}
	}
	if _, err := os.Stat(g.keysPath()); !os.IsNotExist(err) {
		t.Errorf("sidecar exists without a rebind: %v", err)
	}
	// The new window's default key opens and closes it like any row.
	g.handleKeys(Input{JustPressed: []ebiten.Key{ebiten.KeyO}})
	if !g.host.IsOpen(idKeybindings) {
		t.Fatal("O did not open the keybindings window")
	}
	g.handleKeys(Input{JustPressed: []ebiten.Key{ebiten.KeyO}})
	if g.host.IsOpen(idKeybindings) {
		t.Fatal("O did not close the keybindings window")
	}
}

// TestKeybindReplaceRoundTrip swaps two screen toggles through the real
// capture path, steals a world key, then reloads from the sidecar: each
// key drives its new action only, in both directions.
func TestKeybindReplaceRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "save.json")
	g := newShowcaseGameAt(t, path)

	// Inventory takes F from Fitting (which loses its key).
	g.host.services.Keys.Begin("inventory")
	g.handleKeys(Input{JustPressed: []ebiten.Key{ebiten.KeyF}})
	if k, _ := liveKey(g, "inventory"); k != ebiten.KeyF {
		t.Fatalf("inventory = %v, want F", k)
	}
	if _, ok := g.binds["fitting"]; ok {
		t.Fatal("fitting kept F after inventory stole it")
	}
	// Fitting takes I (free now).
	g.host.services.Keys.Begin("fitting")
	g.handleKeys(Input{JustPressed: []ebiten.Key{ebiten.KeyI}})
	// Warp takes S from stop.
	g.host.services.Keys.Begin("warp")
	g.handleKeys(Input{JustPressed: []ebiten.Key{ebiten.KeyS}})
	if _, ok := g.binds["stop"]; ok {
		t.Fatal("stop kept S after warp stole it")
	}

	data, err := os.ReadFile(g.keysPath())
	if err != nil {
		t.Fatalf("sidecar not written: %v", err)
	}
	var f keysFile
	if err := json.Unmarshal(data, &f); err != nil {
		t.Fatalf("sidecar JSON: %v", err)
	}
	if f.Version != keysVersion {
		t.Errorf("sidecar version = %d, want %d", f.Version, keysVersion)
	}
	if kb := f.Binds["inventory"]; kb == nil || kb.Key != "F" {
		t.Errorf("stored inventory = %+v, want F", kb)
	}
	if kb, ok := f.Binds["stop"]; !ok || kb != nil {
		t.Errorf("stored stop = %+v (present=%v), want an explicit null (unbound)", kb, ok)
	}
	if _, ok := f.Binds["mine"]; ok {
		t.Error("sidecar stores an unchanged default (mine)")
	}

	// Reload: the overrides replay over the defaults.
	g2 := newShowcaseGameAt(t, path)
	if k, _ := liveKey(g2, "inventory"); k != ebiten.KeyF {
		t.Fatalf("reloaded inventory = %v, want F", k)
	}
	if k, _ := liveKey(g2, "fitting"); k != ebiten.KeyI {
		t.Fatalf("reloaded fitting = %v, want I", k)
	}
	if a, ok := g2.actionFor(ebiten.KeyS, false); !ok || a != "warp" {
		t.Errorf("S resolves to %q (ok=%v), want warp", a, ok)
	}
	if z, _ := liveKey(g2, "warp"); z == ebiten.KeyZ {
		t.Error("warp kept its default Z beside the new S")
	}

	// Both directions through the real key pass.
	g2.handleKeys(Input{JustPressed: []ebiten.Key{ebiten.KeyF}})
	if !g2.host.IsOpen(idInventory) {
		t.Error("F did not toggle inventory")
	}
	if g2.host.IsOpen(idFitting) {
		t.Error("F also toggled fitting (the old holder)")
	}
	g2.handleKeys(Input{JustPressed: []ebiten.Key{ebiten.KeyI}})
	if !g2.host.IsOpen(idFitting) {
		t.Error("I did not toggle fitting")
	}
	if !g2.host.IsOpen(idInventory) {
		t.Error("I toggled inventory back (the old holder)")
	}

	// The stolen world key still fires the world action: S starts a warp.
	var target sim.Info
	for _, info := range g2.s.Grid() {
		if info.CanWarp && info.Distance >= sim.WarpMinRange {
			target = info
			break
		}
	}
	if target.Entity.IsZero() {
		t.Fatal("fixture: no warpable target at least WarpMinRange away")
	}
	g2.setSelected(target.Entity)
	g2.handleKeys(Input{JustPressed: []ebiten.Key{ebiten.KeyS}})
	g2.s.Step()
	if _, ok := g2.s.Warps.Get(g2.s.Player()); !ok {
		t.Fatal("S did not reach the warp command after the rebind")
	}
}

// TestKeybindReservedAndCancel: Esc cancels capture (and never becomes a
// binding or opens the menu), a fixed chord is refused with a visible
// notice and the row stays armed.
func TestKeybindReservedAndCancel(t *testing.T) {
	g := newShowcaseGameAt(t, filepath.Join(t.TempDir(), "save.json"))

	g.host.services.Keys.Begin("inventory")
	g.handleKeys(Input{JustPressed: []ebiten.Key{ebiten.KeyEscape}})
	if g.captureAction != "" || g.keyNotice != "" {
		t.Errorf("Esc left capture armed (action=%q notice=%q)", g.captureAction, g.keyNotice)
	}
	if k, _ := liveKey(g, "inventory"); k != ebiten.KeyI {
		t.Errorf("Esc assigned inventory = %v, want the unchanged I", k)
	}
	if _, ok := g.top().(*MenuScene); ok {
		t.Error("Esc opened the menu while capture was armed")
	}

	g.host.services.Keys.Begin("inventory")
	g.handleKeys(Input{JustPressed: []ebiten.Key{ebiten.KeyF9}})
	if g.captureAction != "inventory" {
		t.Fatal("a fixed chord cancelled the capture instead of refusing it")
	}
	if !strings.Contains(g.keyNotice, "fixed") {
		t.Errorf("notice = %q, want a fixed-shortcut notice", g.keyNotice)
	}
	if k, _ := liveKey(g, "inventory"); k != ebiten.KeyI {
		t.Errorf("F9 assigned inventory = %v, want I", k)
	}
	if rows := g.host.services.Keys.Rows(); func() bool {
		for _, r := range rows {
			if r.Action == "inventory" {
				return r.Armed && strings.Contains(r.Notice, "fixed")
			}
		}
		return false
	}() == false {
		t.Error("the armed row does not carry the refusal notice")
	}
}

// TestKeybindSidecarDropsInvalid: unknown ids, unparseable key names, a
// reserved key and a future version are all dropped; a valid override
// beside them still applies.
func TestKeybindSidecarDropsInvalid(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "save.json")
	body := `{"version": 1, "binds": {
		"nope": {"key": "A"},
		"inventory": {"key": "NotAKey"},
		"warp": {"key": "Escape"},
		"fitting": {"key": "V"}
	}}`
	if err := os.WriteFile(path+".keys.json", []byte(body), 0o644); err != nil {
		t.Fatalf("write sidecar: %v", err)
	}
	g := newShowcaseGameAt(t, path)
	if k, _ := liveKey(g, "inventory"); k != ebiten.KeyI {
		t.Errorf("invalid key name assigned inventory = %v, want I", k)
	}
	if k, _ := liveKey(g, "warp"); k != ebiten.KeyZ {
		t.Errorf("reserved key assigned warp = %v, want Z", k)
	}
	if k, _ := liveKey(g, "fitting"); k != ebiten.KeyV {
		t.Errorf("valid override beside the invalid ones = %v, want V", k)
	}
	if _, ok := g.binds["nope"]; ok {
		t.Error("unknown action id entered the live table")
	}

	future := filepath.Join(t.TempDir(), "save.json")
	if err := os.WriteFile(future+".keys.json", []byte(`{"version": 99, "binds": {"inventory": {"key": "V"}}}`), 0o644); err != nil {
		t.Fatalf("write sidecar: %v", err)
	}
	g = newShowcaseGameAt(t, future)
	if k, _ := liveKey(g, "inventory"); k != ebiten.KeyI {
		t.Errorf("future-version sidecar applied inventory = %v, want the default I", k)
	}
}

// TestKeybindResetClearsFile: the two-step control's action removes the
// overrides and the file, and the default keys work again.
func TestKeybindResetClearsFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "save.json")
	g := newShowcaseGameAt(t, path)
	g.host.services.Keys.Begin("inventory")
	g.handleKeys(Input{JustPressed: []ebiten.Key{ebiten.KeyV}})
	if _, err := os.Stat(g.keysPath()); err != nil {
		t.Fatalf("sidecar not written: %v", err)
	}
	g.host.services.Keys.Reset()
	if _, err := os.Stat(g.keysPath()); !os.IsNotExist(err) {
		t.Errorf("reset left the sidecar: %v", err)
	}
	if k, _ := liveKey(g, "inventory"); k != ebiten.KeyI {
		t.Errorf("reset left inventory = %v, want I", k)
	}
	g.handleKeys(Input{JustPressed: []ebiten.Key{ebiten.KeyV}})
	if g.host.IsOpen(idInventory) {
		t.Error("V still toggled inventory after the reset")
	}
	g.handleKeys(Input{JustPressed: []ebiten.Key{ebiten.KeyI}})
	if !g.host.IsOpen(idInventory) {
		t.Error("I did not toggle inventory after the reset")
	}
}

// TestKeybindCaptureRows: the window's rows follow the capture lifecycle
// (idle key → armed → assigned / cancelled).
func TestKeybindCaptureRows(t *testing.T) {
	g := newShowcaseGameAt(t, filepath.Join(t.TempDir(), "save.json"))
	row := func() (key string, armed, found bool) {
		for _, r := range g.host.services.Keys.Rows() {
			if r.Action == "inventory" {
				return r.Key, r.Armed, true
			}
		}
		return "", false, false
	}
	if key, armed, ok := row(); !ok || key != "I" || armed {
		t.Fatalf("idle inventory row = %q armed=%v (ok=%v), want key I", key, armed, ok)
	}
	g.host.services.Keys.Begin("inventory")
	if _, armed, ok := row(); !ok || !armed {
		t.Fatalf("armed inventory row armed=%v (ok=%v)", armed, ok)
	}
	g.handleKeys(Input{JustPressed: []ebiten.Key{ebiten.KeyEscape}})
	if key, armed, ok := row(); !ok || armed || key != "I" {
		t.Fatalf("cancelled row = %q armed=%v (ok=%v), want the idle I row", key, armed, ok)
	}
	g.host.services.Keys.Begin("inventory")
	g.handleKeys(Input{JustPressed: []ebiten.Key{ebiten.KeyV}})
	if key, armed, ok := row(); !ok || armed || key != "V" {
		t.Fatalf("assigned row = %q armed=%v (ok=%v), want key V", key, armed, ok)
	}
}

// TestKeybindSidecarExcludedFromSaves: the keybindings sidecar is not a
// save file in the title's manager.
func TestKeybindSidecarExcludedFromSaves(t *testing.T) {
	dir := t.TempDir()
	for _, name := range []string{
		"save.json", "save.json.keys.json", "save.json.layout.json", "save.json.settings.json",
	} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte("{}"), 0o644); err != nil {
			t.Fatalf("write %s: %v", name, err)
		}
	}
	saves := listSaves(dir)
	if len(saves) != 1 || filepath.Base(saves[0].Path) != "save.json" {
		t.Errorf("listSaves = %+v, want only save.json", saves)
	}
}

// TestTitleKeybindings: the menu entry sits between Settings and Credits,
// the window mounts from the title, and a capture there binds for the
// session that follows.
func TestTitleKeybindings(t *testing.T) {
	g := newTitleGameAt(t, filepath.Join(t.TempDir(), "save.json"))
	g.applySize(1280, 720)
	labels := g.titleUI.labels()
	si, ki, ci := slices.Index(labels, "Settings"), slices.Index(labels, "Keybindings"), slices.Index(labels, "Credits")
	if ki < 0 || !(si < ki && ki < ci) {
		t.Fatalf("menu = %v, want Settings < Keybindings < Credits", labels)
	}

	g.titleUI.openID(idKeybindings)
	if err := g.Update(); err != nil {
		t.Fatalf("Update: %v", err)
	}
	if !g.titleUI.host.Mounted(idKeybindings) {
		t.Fatal("keybindings window did not mount from the title")
	}
	g.Draw(ebiten.NewImage(1280, 720)) // lay the window out once
	if r := g.titleUI.host.Window(idKeybindings).GetContainer().GetWidget().Rect; r.Dx() <= 0 || r.Dy() <= 0 || r.Max.X > 1280 || r.Max.Y > 720 {
		t.Errorf("keybindings window rect = %v, want an on-screen size", r)
	}
	g.host.services.Keys.Begin("inventory")
	if err := g.updateTitle(Input{JustPressed: []ebiten.Key{ebiten.KeyV}}); err != nil {
		t.Fatalf("updateTitle capture: %v", err)
	}
	if k, _ := liveKey(g, "inventory"); k != ebiten.KeyV {
		t.Errorf("title capture assigned inventory = %v, want V", k)
	}
	g.titleUI.closeTop()
	if err := g.Update(); err != nil {
		t.Fatalf("Update after close: %v", err)
	}
	if g.titleUI.host.Mounted(idKeybindings) {
		t.Error("the keybindings window survived its close")
	}
	if !g.title {
		t.Error("Esc at the title left the title phase")
	}
}
