package game

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"

	uikit "github.com/entieve/entieve/uikit/screens"
	"github.com/hajimehoshi/ebiten/v2"
)

// Keybindings: the player's key overrides live in a sidecar next to the
// save file, <Options.SavePath>.keys.json — never in settings.json (UI
// prefs are global; bindings are too, but the file split keeps the two
// concerns apart). The format is action id → key, holding ONLY the player's
// deviations from the defaults:
//
//	{"version": 1, "binds": {"warp": {"key": "S"}, "stop": null}}
//
// A null entry is an action the player's last rebind left unbound (the
// replace policy below stole its key). Missing, corrupt or unknown-version
// files degrade to the defaults (never an error, never a crash); unknown
// action ids and unparseable key names are dropped. Rebinding writes the
// file immediately; reset-all removes it.
//
// Rebindable actions are the screen toggles the Screen spec declares and
// the world commands in worldKeyMap. The global chords are a FIXED layer
// handled literally in handleKeys and are never rebindable: Esc (the menu,
// and reserved as a plain binding), Ctrl+S, Ctrl+F, F9, [ ] - =.
//
// Conflict policy is REPLACE: a key is bound to exactly one action. Binding
// a key another action holds deletes that action's binding (it becomes
// unbound, shown as "—"), so replaying the overrides over the defaults on
// load reproduces the same state.

const keysVersion = 1

// binding is one live key combination: the key plus whether Shift must be
// held (Shift+Tab and Shift+L are defaults).
type binding struct {
	Key   ebiten.Key
	Shift bool
}

// keyAction is one rebindable action: its stable id (the Screen spec's
// action or a worldKeyMap id), the window label, its default binding and
// the keybindings window's section (derived from which table declared it:
// worldKeyMap or the Screen spec).
type keyAction struct {
	ID    string
	Label string
	Group string
	Def   binding
	// Keys is the fixed layer's display text (its key set); empty for a
	// rebindable action, whose chip reads the live binding instead.
	Keys string
}

// The keybindings window's section names, in draw order: the world
// commands, the Screen spec's toggles, then the fixed, never-rebindable
// chord layer.
const (
	groupShip  = "Ship controls"
	groupWin   = "Windows"
	groupFixed = "Fixed shortcuts"
)

// fixedActions are the global chord layer's action ids: documented in
// KeyMap but never overridable (the window skips them).
var fixedActions = map[string]bool{
	"menu": true, "save": true, "load": true, "market-search": true,
	"time-slower": true, "time-faster": true,
}

// keyActions is the rebindable action table in declaration order — world
// commands first, then the Screen spec's toggles. The order is also the
// keybindings window's list order and the deterministic replay order for
// the sidecar's overrides.
var keyActions []keyAction

// defaultBinds is the action id → default combination table.
var defaultBinds map[string]binding

func init() {
	seen := map[string]bool{}
	add := func(b Binding, group string) {
		if fixedActions[b.Action] || seen[b.Action] {
			return
		}
		seen[b.Action] = true
		keyActions = append(keyActions, keyAction{
			ID:    b.Action,
			Label: actionLabel(b.Desc),
			Group: group,
			Def:   binding{Key: b.Key, Shift: b.Shift},
		})
	}
	for _, b := range worldKeyMap {
		add(b, groupShip)
	}
	for _, b := range screenBindings(screens) {
		add(b, groupWin)
	}
	defaultBinds = make(map[string]binding, len(keyActions))
	for _, a := range keyActions {
		defaultBinds[a.ID] = a.Def
	}
}

// fixedRows is the fixed chord layer as the window's informational
// section: one row per fixed ACTION in KeyMap order, its keys joined
// ("[ / -"). Derived from KeyMap (which screens.go's init assigns after
// this file's), never a second hand-written list.
var fixedRows []keyAction

// fixedKeyRows builds fixedRows on first use — the window's Rows closure
// is the only caller, and the action table is static per build.
func fixedKeyRows() []keyAction {
	if fixedRows != nil {
		return fixedRows
	}
	fixedRows = []keyAction{}
	at := map[string]int{}
	for _, b := range KeyMap {
		if !fixedActions[b.Action] {
			continue
		}
		if i, ok := at[b.Action]; ok {
			fixedRows[i].Keys += " · " + chordText(b.Desc)
			continue
		}
		at[b.Action] = len(fixedRows)
		fixedRows = append(fixedRows, keyAction{
			ID:    b.Action,
			Label: actionLabel(b.Desc),
			Group: groupFixed,
			Keys:  chordText(b.Desc),
		})
	}
	return fixedRows
}

// chordText is a KeyMap row's key text: the part before the em dash, which
// is how the chord is documented ("Ctrl+S", "[", "Esc") — the fixed layer
// shows it verbatim, modifiers included.
func chordText(desc string) string {
	key, _, _ := strings.Cut(desc, " — ")
	return key
}

// actionLabel turns a KeyMap description into a row label: the text after
// the em dash, without the trailing parenthetical.
func actionLabel(desc string) string {
	_, d, ok := strings.Cut(desc, " — ")
	if !ok {
		return desc
	}
	if i := strings.Index(d, " ("); i > 0 {
		d = d[:i]
	}
	return d
}

// keysFile is the overrides sidecar document.
type keysFile struct {
	Version int                 `json:"version"`
	Binds   map[string]*keyBind `json:"binds"`
}

// keyBind is one stored override. A nil *keyBind (JSON null) means the
// action is unbound, which happens when replace policy took its key.
type keyBind struct {
	Key   string `json:"key"`
	Shift bool   `json:"shift,omitempty"`
}

// keysPath is the sidecar location: the resolved save path plus .keys.json
// (e.g. …/entieve/save.json.keys.json).
func (g *Game) keysPath() string { return g.SavePath() + ".keys.json" }

// loadKeys installs the defaults and replays the sidecar's overrides over
// them (replace policy included). Called from New before the interface is
// built, so the very first frame uses the saved binds.
func (g *Game) loadKeys() {
	g.binds = make(map[string]binding, len(defaultBinds))
	for id, b := range defaultBinds {
		g.binds[id] = b
	}
	data, err := os.ReadFile(g.keysPath())
	if err != nil {
		return
	}
	var f keysFile
	if err := json.Unmarshal(data, &f); err != nil {
		return
	}
	if f.Version < 1 || f.Version > keysVersion {
		return
	}
	// Explicit unbound entries first, then the bindings: each stored key
	// steals from its holder exactly like the live rebind did.
	for _, a := range keyActions {
		if kb, ok := f.Binds[a.ID]; ok && kb == nil {
			delete(g.binds, a.ID)
		}
	}
	for _, a := range keyActions {
		kb, ok := f.Binds[a.ID]
		if !ok || kb == nil {
			continue
		}
		k, ok := parseKey(kb.Key)
		if !ok || reservedKey(k) {
			continue
		}
		g.assign(a.ID, binding{Key: k, Shift: kb.Shift})
	}
}

// assign binds one action, stealing the combination from its previous
// holder (replace policy).
func (g *Game) assign(action string, b binding) {
	for id, cur := range g.binds {
		if id != action && cur == b {
			delete(g.binds, id)
		}
	}
	g.binds[action] = b
}

// writeKeys persists the overrides (only deviations from the defaults; an
// empty set removes the file, which is also reset-all's flush).
func (g *Game) writeKeys() error {
	f := keysFile{Version: keysVersion, Binds: map[string]*keyBind{}}
	for _, a := range keyActions {
		cur, bound := g.binds[a.ID]
		if bound && cur == a.Def {
			continue
		}
		if !bound {
			f.Binds[a.ID] = nil
			continue
		}
		f.Binds[a.ID] = &keyBind{Key: cur.Key.String(), Shift: cur.Shift}
	}
	if len(f.Binds) == 0 {
		if err := os.Remove(g.keysPath()); err != nil && !os.IsNotExist(err) {
			return err
		}
		return nil
	}
	data, err := json.MarshalIndent(f, "", "  ")
	if err != nil {
		return err
	}
	path := g.keysPath()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o644)
}

// resetKeys restores every default and clears the sidecar.
func (g *Game) resetKeys() {
	g.binds = make(map[string]binding, len(defaultBinds))
	for id, b := range defaultBinds {
		g.binds[id] = b
	}
	g.cancelCapture()
	if err := os.Remove(g.keysPath()); err != nil && !os.IsNotExist(err) {
		g.report(err, "")
	}
	g.refreshHelp()
	g.report(nil, "Keybindings reset")
}

// beginCapture arms one action's row: the next key press assigns it.
func (g *Game) beginCapture(action string) {
	if _, ok := defaultBinds[action]; !ok {
		return
	}
	g.captureAction = action
	g.keyNotice = ""
}

// cancelCapture disarms the pending row.
func (g *Game) cancelCapture() { g.captureAction, g.keyNotice = "", "" }

// unbindAction leaves one action unbound (the sidecar's explicit null):
// the action triggers nothing until it is rebound, and replace policy has
// already done the same to a key's previous holder.
func (g *Game) unbindAction(action string) {
	if _, ok := defaultBinds[action]; !ok {
		return
	}
	delete(g.binds, action)
	g.cancelCapture()
	if err := g.writeKeys(); err != nil {
		g.report(err, "")
	}
	g.refreshHelp()
}

// resetKey restores one action's default binding (the row's Reset), which
// steals it back from a holder exactly like any rebind.
func (g *Game) resetKey(action string) {
	def, ok := defaultBinds[action]
	if !ok {
		return
	}
	g.assign(action, def)
	g.cancelCapture()
	if err := g.writeKeys(); err != nil {
		g.report(err, "")
	}
	g.refreshHelp()
}

// captureKey feeds one just-pressed key to the armed row and reports
// whether the capture layer consumed the key. Esc cancels; Backspace or
// Delete clears the binding (the action is then unbound); a modifier key
// keeps waiting; a fixed chord shows the notice and keeps waiting;
// anything else assigns (replace policy).
func (g *Game) captureKey(k ebiten.Key, ctrl, shift bool) bool {
	if g.captureAction == "" {
		return false
	}
	switch k {
	case ebiten.KeyShift, ebiten.KeyControl, ebiten.KeyAlt, ebiten.KeyMeta:
		return true
	}
	if k == ebiten.KeyEscape {
		g.cancelCapture()
		return true
	}
	action := g.captureAction
	switch {
	case k == ebiten.KeyBackspace || k == ebiten.KeyDelete:
		g.unbindAction(action)
	case ctrl:
		g.keyNotice = "Ctrl chords are fixed — try another key"
	case reservedKey(k):
		g.keyNotice = keyText(k) + " is a fixed shortcut"
	default:
		g.assign(action, binding{Key: k, Shift: shift})
		g.cancelCapture()
		if err := g.writeKeys(); err != nil {
			g.report(err, "")
		}
		g.refreshHelp()
	}
	return true
}

// actionFor resolves a pressed combination to its rebindable action. The
// table order makes the result deterministic (assign keeps combinations
// unique, so at most one action matches).
func (g *Game) actionFor(k ebiten.Key, shift bool) (string, bool) {
	for _, a := range keyActions {
		if b, ok := g.binds[a.ID]; ok && b.Key == k && b.Shift == shift {
			return a.ID, true
		}
	}
	return "", false
}

// actionHeld reports whether a held-key action's key is down. A binding
// that requires Shift demands it; a plain binding ignores the modifier,
// so the default arrow panning keeps working with Shift held.
func (g *Game) actionHeld(in Input, action string) bool {
	b, ok := g.binds[action]
	if !ok || !held(in.KeysDown, b.Key) {
		return false
	}
	return !b.Shift || held(in.KeysDown, ebiten.KeyShift)
}

// keybindRows renders the live table for the keybindings window: the
// rebindable actions in declaration order, then the fixed chord layer.
func (g *Game) keybindRows() []uikit.KeybindRow {
	rows := make([]uikit.KeybindRow, 0, len(keyActions)+len(fixedKeyRows()))
	for _, a := range keyActions {
		row := uikit.KeybindRow{Action: a.ID, Label: a.Label, Group: a.Group}
		if b, ok := g.binds[a.ID]; ok {
			row.Key = bindText(b)
			row.Rebound = b != a.Def
		}
		if g.captureAction == a.ID {
			row.Armed = true
			row.Notice = g.keyNotice
		}
		rows = append(rows, row)
	}
	for _, a := range fixedKeyRows() {
		rows = append(rows, uikit.KeybindRow{
			Action: a.ID, Label: a.Label, Group: a.Group, Key: a.Keys, Fixed: true,
		})
	}
	return rows
}

// liveKeyLabel is an action's display key when the player changed it:
// "" for an unchanged default or a fixed chord, "—" when unbound.
func (g *Game) liveKeyLabel(action string) string {
	def, ok := defaultBinds[action]
	if !ok {
		return ""
	}
	cur, bound := g.binds[action]
	switch {
	case !bound:
		return "—"
	case cur == def:
		return ""
	default:
		return bindText(cur)
	}
}

// refreshHelp re-renders an already-built help overlay with the live keys.
func (g *Game) refreshHelp() {
	if help := bodyOf[*uikit.HelpPanel](g.host, idHelp); help != nil {
		help.SetRows(helpRowsFor(g))
	}
}

// reservedKey is a key the fixed chord layer owns (Esc is the menu key and
// reserved as a plain binding too): assigning one would be shadowed before
// the data layer ever runs.
func reservedKey(k ebiten.Key) bool {
	switch k {
	case ebiten.KeyEscape, ebiten.KeyF9,
		ebiten.KeyBracketLeft, ebiten.KeyBracketRight, ebiten.KeyMinus, ebiten.KeyEqual:
		return true
	}
	return false
}

// keyText is a key's display name (arrow keys read as words, the rest as
// ebiten names them).
func keyText(k ebiten.Key) string {
	switch k {
	case ebiten.KeyArrowUp:
		return "Up arrow"
	case ebiten.KeyArrowDown:
		return "Down arrow"
	case ebiten.KeyArrowLeft:
		return "Left arrow"
	case ebiten.KeyArrowRight:
		return "Right arrow"
	}
	return k.String()
}

// bindText is a combination's display text ("Shift+Q").
func bindText(b binding) string {
	if b.Shift {
		return "Shift+" + keyText(b.Key)
	}
	return keyText(b.Key)
}

// keyByName maps ebiten's key names back to keys (the sidecar's spelling).
var keyByName = func() map[string]ebiten.Key {
	m := make(map[string]ebiten.Key)
	for k := ebiten.Key(0); k <= ebiten.KeyMax; k++ {
		if name := k.String(); name != "" {
			if _, ok := m[name]; !ok {
				m[name] = k
			}
		}
	}
	return m
}()

// parseKey resolves a stored key name.
func parseKey(name string) (ebiten.Key, bool) {
	k, ok := keyByName[name]
	return k, ok
}

// helpRowsFor converts KeyMap into the help panel's row shape, with the
// session's live keys in place of the unchanged defaults (nil = defaults).
func helpRowsFor(g *Game) []uikit.KeyRow {
	rows := make([]uikit.KeyRow, 0, len(KeyMap))
	for _, b := range KeyMap {
		key, desc := b.Desc, ""
		if i := strings.Index(b.Desc, " — "); i >= 0 {
			key, desc = b.Desc[:i], b.Desc[i+len(" — "):]
		}
		if g != nil {
			if live := g.liveKeyLabel(b.Action); live != "" {
				key = live
			}
		}
		rows = append(rows, uikit.KeyRow{Key: key, Desc: desc})
	}
	return rows
}
