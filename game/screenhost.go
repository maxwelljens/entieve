package game

// The Screen host: the Game-free state machine between a Screen spec row
// and its built panel. It owns the open flag, the mount, the committed
// close and the placement record; the panel owns Sync. Two instances run
// per session — the game host (every row) and the title host (the
// title-scoped rows, game/title.go) — so the title's Settings and
// Keybindings are the same rows mounted through the same seam.

import (
	"image"
	"strings"

	"github.com/ebitenui/ebitenui/widget"
	uikit "github.com/entieve/entieve/uikit/screens"
	"github.com/entieve/entieve/uikit/widgets"
)

// hostSink is what the host needs from the interface kit: mount/remove a
// window, register its ×, and read whether a committed close fade is
// running. *uikit.Kit implements it; tests substitute a recorder.
type hostSink interface {
	AddWindow(*widget.Window) widget.RemoveWindowFunc
	SetWindowClose(*widget.Window, func())
	Closing(*widget.Window) bool
}

// screenServices is the game-backed bundle a row's Build may reach
// through the host. It keeps *Game out of the Build signature: the few
// constructions that genuinely need session state arrive as closures.
type screenServices struct {
	Env      uikit.Session
	Settings uikit.SettingsActions
	Keys     uikit.KeybindActions
	HelpRows func() []uikit.KeyRow
	Menu     func() widgets.WindowPanel
	Station  func() widgets.WindowPanel
	// Close is the owner's close semantics for a row's ×: the game routes
	// modal rows to their scene, the title host leaves it nil (plain
	// toggle).
	Close func(ScreenID)
	// Cancel disarms an armed keybind capture (the keybindings panel's
	// Unmounted hook).
	Cancel func()
}

// Room is the frame's context the mount rule reads.
type Room struct {
	Title  bool
	Docked bool
}

type hostScope uint8

const (
	gameScope hostScope = iota
	titleScope
)

// screenSlot is one row's live state.
type screenSlot struct {
	row       Screen
	panel     widgets.WindowPanel
	open      bool
	mount     widget.RemoveWindowFunc
	dismissed bool
}

// ScreenHost mounts, syncs and dismisses a Screen table's panels.
type ScreenHost struct {
	scope    hostScope
	slots    []*screenSlot
	byID     map[ScreenID]*screenSlot
	byTitle  map[string]*screenSlot
	byAction map[string]*screenSlot
	sink     hostSink
	services screenServices

	// placed is the last rect this session composed for each window;
	// restored marks the windows positioned from the layout sidecar. A
	// window is virgin — safe to (re)place when the viewport changes —
	// while it carries no restored flag and still sits exactly where we
	// last put it.
	placed   map[*widget.Window]image.Rectangle
	restored map[*widget.Window]bool

	// onMount/onUnmount are the game's UI-sound hooks (nil = silent):
	// they fire on a row's mount and unmount edges only, never per frame.
	onMount, onUnmount func(ScreenID)

	// quiet silences the next reconcile pass: the boot composition and the
	// remount after Rebind (a rebuilt kit brings back the windows the
	// player already had) are not opens, so they must not speak.
	quiet bool
}

// newScreenHost builds the host over a row table. scope title keeps
// panels lazy (fresh on every open) and rows exclusive; its table is the
// title's own set (game/title.go).
func newScreenHost(rows []Screen, scope hostScope, sink hostSink, services screenServices) *ScreenHost {
	h := &ScreenHost{
		scope:    scope,
		byID:     make(map[ScreenID]*screenSlot, len(rows)),
		byTitle:  make(map[string]*screenSlot, len(rows)),
		byAction: make(map[string]*screenSlot, len(rows)),
		sink:     sink,
		services: services,
		placed:   map[*widget.Window]image.Rectangle{},
		restored: map[*widget.Window]bool{},
		quiet:    true, // the boot composition is not an open
	}
	for _, row := range rows {
		s := &screenSlot{row: row}
		h.slots = append(h.slots, s)
		h.byID[row.ID] = s
		h.byTitle[strings.ToLower(row.Title)] = s
		if row.Action != "" {
			h.byAction[row.Action] = s
		}
	}
	return h
}

// Rebind points the host at a rebuilt interface: panels, mounts and the
// placement records drop (the old kit owns them), open flags survive —
// a load rebuild must reopen exactly what the player had open.
func (h *ScreenHost) Rebind(sink hostSink, services screenServices) {
	h.sink = sink
	h.services = services
	for _, s := range h.slots {
		s.panel, s.mount, s.dismissed = nil, nil, false
	}
	h.placed = map[*widget.Window]image.Rectangle{}
	h.restored = map[*widget.Window]bool{}
	h.quiet = true // the rebuilt kit remounts what was already open
}

// Warm builds every panel and registers its ×, in row order, before the
// first mount (today's buildKit order).
func (h *ScreenHost) Warm() {
	for _, s := range h.slots {
		h.build(s)
	}
}

// build constructs one panel and registers its close, once.
func (h *ScreenHost) build(s *screenSlot) {
	if s.panel != nil {
		return
	}
	s.panel = s.row.Build(h)
	win := s.panel.Win()
	if s.row.Modal && win != nil {
		win.Modal = true
	}
	id := s.row.ID
	h.sink.SetWindowClose(win, func() { h.closeRow(id) })
}

// closeRow runs a row's × semantics: the owner's close hook when it has
// one (the game's scene-driven modal rows), else a plain toggle.
func (h *ScreenHost) closeRow(id ScreenID) {
	if h.services.Close != nil {
		h.services.Close(id)
		return
	}
	h.Toggle(id)
}

// want is the per-row mount rule. A title host's table IS the title set
// (game/title.go: titleRows), so its rows follow their flag inside the
// title room — the room, not a per-row marker, gates the host.
func (h *ScreenHost) want(s *screenSlot, room Room) bool {
	if h.scope == titleScope {
		return room.Title && s.open
	}
	if room.Title {
		return false
	}
	return s.open &&
		(!s.row.SpaceOnly || !room.Docked) &&
		(!s.row.DockedOnly || room.Docked)
}

// Reconcile mounts and unmounts the rows to match the room. The pass after
// a Rebind (and the boot one) is quiet: those mounts are windows coming
// back, not the player opening them.
func (h *ScreenHost) Reconcile(room Room) {
	quiet := h.quiet
	h.quiet = false
	for _, s := range h.slots {
		h.setMounted(s, h.want(s, room), quiet)
	}
}

// setMounted is the mount lifecycle: a mount builds on demand (title),
// mounts and runs Sync in the same frame; an unmount runs the panel's
// Unmounted hook, then removes.
func (h *ScreenHost) setMounted(s *screenSlot, want, quiet bool) {
	if want == (s.mount != nil) {
		return
	}
	if want {
		h.build(s)
		s.mount = h.sink.AddWindow(s.panel.Win())
		s.dismissed = false
		s.panel.Sync()
		if h.onMount != nil && !quiet {
			h.onMount(s.row.ID)
		}
		return
	}
	if u, ok := s.panel.(interface{ Unmounted() }); ok {
		u.Unmounted()
	}
	s.mount()
	s.mount, s.dismissed = nil, false
	if h.onUnmount != nil && !quiet {
		h.onUnmount(s.row.ID)
	}
	if h.scope == titleScope {
		s.panel = nil
	}
}

// Sync refreshes the mounted panels. A committed chrome close (the ×)
// runs the panel's one-shot Dismissed hook first — same frame, before the
// body Sync — so a hotkey unmount, which never reports Closing, leaves the
// session state alone.
func (h *ScreenHost) Sync() {
	for _, s := range h.slots {
		if s.mount == nil {
			continue
		}
		if !s.dismissed && h.sink.Closing(s.panel.Win()) {
			s.dismissed = true
			if d, ok := s.panel.(interface{ Dismissed() }); ok {
				d.Dismissed()
			}
		}
		s.panel.Sync()
	}
}

// SetOpen sets a row's flag. On the title host opening one row closes the
// others (they are modal there).
func (h *ScreenHost) SetOpen(id ScreenID, open bool) bool {
	s := h.byID[id]
	if s == nil {
		return false
	}
	if open && h.scope == titleScope {
		for _, o := range h.slots {
			o.open = false
		}
	}
	s.open = open
	return true
}

// Toggle flips a row's flag.
func (h *ScreenHost) Toggle(id ScreenID) bool {
	s := h.byID[id]
	if s == nil {
		return false
	}
	return h.SetOpen(id, !s.open)
}

// IsOpen reports a row's flag.
func (h *ScreenHost) IsOpen(id ScreenID) bool {
	if h == nil {
		return false
	}
	s := h.byID[id]
	return s != nil && s.open
}

// Mounted reports whether a row currently has a mount.
func (h *ScreenHost) Mounted(id ScreenID) bool {
	s := h.byID[id]
	return s != nil && s.mount != nil
}

// Window returns a row's built window (nil before Warm).
func (h *ScreenHost) Window(id ScreenID) *widget.Window {
	if s := h.byID[id]; s != nil && s.panel != nil {
		return s.panel.Win()
	}
	return nil
}

// Panel returns a row's built panel (nil before Warm).
func (h *ScreenHost) Panel(id ScreenID) widgets.WindowPanel {
	if s := h.byID[id]; s != nil {
		return s.panel
	}
	return nil
}

// ByTitle resolves a window title (case-insensitive) to its row id.
func (h *ScreenHost) ByTitle(title string) (ScreenID, bool) {
	s, ok := h.byTitle[strings.ToLower(title)]
	if !ok {
		return "", false
	}
	return s.row.ID, true
}

// ByAction resolves a KeyMap action id to its row id.
func (h *ScreenHost) ByAction(action string) (ScreenID, bool) {
	s, ok := h.byAction[action]
	if !ok {
		return "", false
	}
	return s.row.ID, true
}

// AnyOpen reports whether any row's flag is set.
func (h *ScreenHost) AnyOpen() bool {
	for _, s := range h.slots {
		if s.open {
			return true
		}
	}
	return false
}

// CloseAll clears every row's flag (title exclusivity / room teardown).
func (h *ScreenHost) CloseAll() {
	for _, s := range h.slots {
		s.open = false
	}
}

// IDs returns the row ids in spec order.
func (h *ScreenHost) IDs() []ScreenID {
	out := make([]ScreenID, len(h.slots))
	for i, s := range h.slots {
		out[i] = s.row.ID
	}
	return out
}

// OpenFlags snapshots the flags (tests and title bookkeeping).
func (h *ScreenHost) OpenFlags() map[ScreenID]bool {
	out := make(map[ScreenID]bool, len(h.slots))
	for _, s := range h.slots {
		out[s.row.ID] = s.open
	}
	return out
}

// Want exposes the mount rule for tests and composition checks.
func (h *ScreenHost) Want(id ScreenID, room Room) bool {
	s := h.byID[id]
	return s != nil && h.want(s, room)
}

// Slot resolves a sidecar name (== row id) for the layout sidecar.
func (h *ScreenHost) Slot(name string) *screenSlot { return h.byID[ScreenID(name)] }

// Splits returns a row's named seams (nil when the panel has no panes or
// is not built).
func (h *ScreenHost) Splits(id ScreenID) []widgets.Seam {
	s := h.byID[id]
	if s == nil || s.panel == nil {
		return nil
	}
	if p, ok := s.panel.(interface{ Splits() []widgets.Seam }); ok {
		return p.Splits()
	}
	return nil
}

// Tables returns a row's tables (nil when the panel has none or is not
// built).
func (h *ScreenHost) Tables(id ScreenID) []*widgets.Table {
	s := h.byID[id]
	if s == nil || s.panel == nil {
		return nil
	}
	if p, ok := s.panel.(interface{ Tables() []*widgets.Table }); ok {
		return p.Tables()
	}
	return nil
}

// TablesHit reports whether pt lands on one of win's table rows: the click
// cue's row test, asked per window so the kit's press gate honours the
// stack (only the topmost window under the pointer is consulted).
func (h *ScreenHost) TablesHit(win *widget.Window, pt image.Point) bool {
	for _, s := range h.slots {
		if s.panel == nil || s.panel.Win() != win {
			continue
		}
		for _, t := range h.Tables(s.row.ID) {
			if t.RowAt(pt) {
				return true
			}
		}
	}
	return false
}

// PersistedOpen is the open value the sidecar records for a persisted row.
func (h *ScreenHost) PersistedOpen(id ScreenID) bool {
	return h.IsOpen(id)
}

// RestoreOpen applies a sidecar's open flag.
func (h *ScreenHost) RestoreOpen(id ScreenID, open bool) {
	if s := h.byID[id]; s != nil {
		s.open = open
	}
}

// layoutEntry adapts a slot for the layout sidecar: geometry always, open
// flag live, panes and tables only when the panel implements the facet.
func (s *screenSlot) Win() *widget.Window {
	if s.panel == nil {
		return nil
	}
	return s.panel.Win()
}
func (s *screenSlot) PersistOpen() bool  { return s.open }
func (s *screenSlot) RestoreOpen(o bool) { s.open = o }
func (s *screenSlot) Splits() []widgets.Seam {
	if s.panel == nil {
		return nil
	}
	if p, ok := s.panel.(interface{ Splits() []widgets.Seam }); ok {
		return p.Splits()
	}
	return nil
}
func (s *screenSlot) Tables() []*widgets.Table {
	if s.panel == nil {
		return nil
	}
	if p, ok := s.panel.(interface{ Tables() []*widgets.Table }); ok {
		return p.Tables()
	}
	return nil
}

// Place moves win to rect unless the player owns its position (restored
// or moved away from the last composed rect).
func (h *ScreenHost) Place(win *widget.Window, rect image.Rectangle) {
	if win == nil || h.restored[win] {
		return
	}
	if prev, ok := h.placed[win]; ok && win.GetContainer().GetWidget().Rect != prev {
		return
	}
	win.SetLocation(rect)
	h.placed[win] = win.GetContainer().GetWidget().Rect
}

// MarkRestored records a sidecar-restored window: its rect is the
// player's arrangement from here on.
func (h *ScreenHost) MarkRestored(win *widget.Window) {
	h.placed[win] = win.GetContainer().GetWidget().Rect
	h.restored[win] = true
}

// ResetPlacement drops the tracker (a layout reset / interface rebuild).
func (h *ScreenHost) ResetPlacement() {
	h.placed = map[*widget.Window]image.Rectangle{}
	h.restored = map[*widget.Window]bool{}
	h.quiet = true // the rebuilt kit remounts what was already open
}

// PlaceDefaults re-places every row's window from its declared rect +
// anchor (the virgin rules of Place apply).
func (h *ScreenHost) PlaceDefaults(w, height int) {
	for _, s := range h.slots {
		win := s.Win()
		if win == nil {
			continue
		}
		r := win.GetContainer().GetWidget().Rect
		if r.Dx() <= 0 || r.Dy() <= 0 {
			continue
		}
		h.Place(win, anchorRect(s.row.DefaultRect, s.row.Anchor, r.Dx(), r.Dy(), w, height))
	}
}

// ClampRestored re-establishes the on-screen guarantee for restored
// windows when the viewport changes.
func (h *ScreenHost) ClampRestored(w, height int) {
	screen := image.Rect(0, 0, w, height)
	for win, restored := range h.restored {
		if !restored {
			continue
		}
		minSize := image.Point{X: 1, Y: 1}
		if win.MinSize != nil {
			minSize = *win.MinSize
		}
		r := win.GetContainer().GetWidget().Rect
		if c := clampLayoutRect(r, minSize, screen); c != r {
			win.SetLocation(c)
		}
		h.placed[win] = win.GetContainer().GetWidget().Rect
	}
}

// bodyOf returns the built panel behind id as T (nil when the panel is
// absent or another type) — the typed accessor for the few panels the
// game itself calls into.
func bodyOf[T any](h *ScreenHost, id ScreenID) T {
	var zero T
	if h == nil {
		return zero
	}
	s := h.byID[id]
	if s == nil || s.panel == nil {
		return zero
	}
	if v, ok := s.panel.(T); ok {
		return v
	}
	return zero
}

// keybindPanel is the keybindings panel plus its teardown hook: a close
// while a capture is armed must cancel it (the one mount-cycle special
// case the old setPanel carried).
type keybindPanel struct {
	*uikit.Keybindings
	cancel func()
}

// Unmounted cancels an armed capture when the window leaves the stack.
func (p keybindPanel) Unmounted() {
	if p.cancel != nil {
		p.cancel()
	}
}

// winPanel adapts a game-built window (the Esc menu, the station view)
// to the panel seam.
type winPanel struct {
	win  *widget.Window
	sync func()
}

func (p winPanel) Win() *widget.Window { return p.win }
func (p winPanel) Sync() {
	if p.sync != nil {
		p.sync()
	}
}
