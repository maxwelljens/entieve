package game

import (
	"fmt"
	"image"
	"slices"
	"strings"

	"github.com/ebitenui/ebitenui/widget"
	"github.com/entieve/entieve/content"
	"github.com/entieve/entieve/uikit/icons"
	uikit "github.com/entieve/entieve/uikit/screens"
	"github.com/entieve/entieve/uikit/theme"
	"github.com/hajimehoshi/ebiten/v2"
)

// The Screen spec: one table declaring every window the player can open,
// and the single source the window registries derive from. Adding a
// window is one row here plus its Build; the panel enum order, panelDefs
// (the frame/mount/persistence table), the KeyMap toggle rows and their
// help-overlay text, the layout sidecar's name list, placeDefaults and the
// test label map all follow from the rows — each derivation below names
// what it feeds.
//
// The rows are declarative. Behaviour that belongs to a scene (the help
// and Esc overlays) stays where it is; a row reaches it through Modal
// instead of carrying the logic. screenSpecErrors is the well-formedness
// rule for the whole
// table (New refuses to build on a bad spec, the tests drive it with
// deliberately broken rows). The left navigation rail derives from the
// same rows (game/rail.go): every openable row is one rail entry, its
// Icon its glyph, its Group the rail section it lands in.
//
// Default geometry is DESIGN units (theme.Px scales them — see the Px
// funnel, uikit/theme/scale.go) written against the 1280×720 design viewport
// (defaultW×defaultH).

// DockPolicy says where a screen's window lives:
//
//	DockFloating    a free overlay: placeDefaults hangs it on the viewport
//	                from its DefaultRect + Anchor, the sidecar persists it
//	DockTransient   scene-owned and never persisted — the Esc menu, which
//	                must not reopen on the next boot
//
// There is no docked special case any more: a service window opened while
// docked is an ordinary floating window (the market's three-pane rect, the
// inventory/fitting rects) exactly like it is in space. The one window
// docking opens — the Station Services view — declares the station column
// as its DefaultRect and is re-placed to the board's content height by
// contracts.go.
type DockPolicy uint8

const (
	// DockFloating is the zero value: a row that forgets to say where it
	// lives is a normal floating window.
	DockFloating DockPolicy = iota
	DockTransient
)

// Anchor says which parts of a screen's DefaultRect the composition
// reads (see anchorRect). AnchorCenter is the zero value: a floating row
// that does not say where it goes lands centred.
type Anchor uint8

const (
	AnchorCenter     Anchor = iota // the viewport decides both axes
	AnchorFixed                    // Min = absolute design position
	AnchorBottomLeft               // Min.X = left inset, defaultH-Max.Y = bottom inset
	AnchorTopCenter                // Min.Y = top inset, width centred
)

// keyNone marks a screen with no key binding. It cannot be 0: ebiten's
// zero value is ebiten.KeyA.
const keyNone ebiten.Key = -1

// Screen is one window the player can open — the spec row every registry
// derives from.
//
//	ID            panel id (the enum slot); rows are declared in enum
//	              order, which fixes the mount — and therefore the draw —
//	              order (the log lands before the Esc menu)
//	Title         the window's canonical name (its title bar, the test
//	              labels; must be non-empty and unique)
//	Icon          the title-bar glyph AND the navigation rail's entry
//	              glyph — the rail renders one icon per openable row, so
//	              a non-transient row needs one (screenSpecErrors
//	              rejects an empty Icon outside transients; non-empty
//	              names are checked against the icon manifest). The uikit
//	              constructors draw their own bars — the starmap appends
//	              its region — so the spec declares the identity, it does
//	              not re-render it
//	Key, Shift    the toggle binding (keyNone = none); Action + Desc are
//	              the KeyMap row that derives from it, which is also what
//	              the help overlay renders and what the rail's tooltip
//	              quotes as its hint. A MODAL screen's key is wired to its
//	              scene instead (Esc → onEscape, H → help), so Modal rows
//	              are never flag-toggled here
//	DefaultRect   default geometry in design units (see Anchor for which
//	              parts the composition reads)
//	DockPolicy    floating / transient (not in the sidecar)
//	Modal         the window is modal while up (help, menu)
//	SpaceOnly     mounts only in space (the scanner's docked counterpart
//	              rule); DockedOnly mounts only while docked (the manufacturing
//	              and the services view)
//	Group         the rail's section: railEntries (game/rail.go) groups
//	              and orders the entries by it (transients carry none and
//	              never appear; every other row must name a known group)
//	Service       the station service this window needs: docked at a
//	              station lacking it the rail entry grays, clicking it
//	              does nothing ("" = no dependency; in space the window
//	              opens exactly as its key always did)
//	SidecarName   the layout-sidecar key; "" = not persisted (transient)
//	Sync          refresh the panel from the sim (nil = static content);
//	              runs every frame while mounted and once when it mounts
//	Build         constructs the window on the session, once per buildKit
type Screen struct {
	ID    panelID
	Title string
	Icon  string

	Key    ebiten.Key
	Shift  bool
	Action string
	Desc   string

	DefaultRect image.Rectangle
	Anchor      Anchor

	DockPolicy  DockPolicy
	Modal       bool
	SpaceOnly   bool
	DockedOnly  bool
	Group       string
	Service     string
	SidecarName string

	Sync  func(*Game)
	Build func(*Game) *widget.Window
}

// screens is THE table. Enum order, one row per panel id.
var screens = []Screen{
	{
		// An ordinary floating window, docked or in space: it opens at its
		// own declared default (uikit.NewInventory's landing rect), not in
		// a composed docked band.
		ID: pInventory, Title: "Inventory", Icon: "svc_inventory",
		Key: ebiten.KeyI, Action: "inventory", Desc: "I — toggle inventory",
		DefaultRect: image.Rect(396, 256, 701, 690), Anchor: AnchorFixed,
		DockPolicy: DockFloating, SidecarName: "inventory", Group: "Trade",
		Sync:  func(g *Game) { g.inv.Sync() },
		Build: func(g *Game) *widget.Window { g.inv = uikit.NewInventory(g.env); return g.inv.Win() },
	},
	{
		ID: pFitting, Title: "Fitting", Icon: "svc_fitting",
		Key: ebiten.KeyF, Action: "fitting", Desc: "F — toggle fitting",
		DefaultRect: image.Rect(370, 8, 1014, 690), Anchor: AnchorFixed,
		DockPolicy: DockFloating, SidecarName: "fitting",
		Group: "Ship", Service: "fitting",
		Sync:  func(g *Game) { g.fitt.Sync() },
		Build: func(g *Game) *widget.Window { g.fitt = uikit.NewFitting(g.env); return g.fitt.Win() },
	},
	{
		// The market's per-type default is its landing rect
		// (uikit.NewMarket) — 760×560, the three-pane width that starts on
		// the rail gutter's clear line and stays inside the 1024×700
		// minimum. Docked or in space it opens here; the old docked band
		// stretched it across the viewport.
		ID: pMarket, Title: "Market", Icon: "svc_market",
		Key: ebiten.KeyG, Action: "market", Desc: "G — toggle market window (trade while docked)",
		DefaultRect: image.Rect(97, 8, 857, 568), Anchor: AnchorFixed,
		DockPolicy: DockFloating, SidecarName: "market",
		Group: "Trade", Service: "market",
		Sync:  func(g *Game) { g.mkt.Sync() },
		Build: func(g *Game) *widget.Window { g.mkt = uikit.NewMarket(g.env); return g.mkt.Win() },
	},
	{
		// A station service that floats: it mounts only while docked and
		// centres like the overlays on first open (its Screen row carries
		// no Anchor, so the centred fallback applies).
		ID: pManufacturing, Title: "Manufacturing", Icon: "svc_manufacturing",
		Key: ebiten.KeyB, Action: "manufacturing", Desc: "B — toggle manufacturing window (manufacture while docked)",
		DefaultRect: image.Rect(312, 120, 792, 600),
		DockPolicy:  DockFloating, DockedOnly: true, SidecarName: "manufacturing",
		Group: "Industry", Service: "industry",
		Sync:  func(g *Game) { g.manu.Sync() },
		Build: func(g *Game) *widget.Window { g.manu = uikit.NewManufacturing(g.env); return g.manu.Win() },
	},
	{
		// Space chrome: hidden while docked (legacy parity). Fixed in the
		// centre column under the top row — uikit.NewScanner's own
		// constructor default is 10+RailInset,251 (above the bottom-left
		// log). The x carries the rail inset: the left column (rail,
		// overview, log, station) all start clear of uikit.RailInset.
		ID: pScanner, Title: "Scanner", Icon: "svc_scanner",
		Key: ebiten.KeyC, Action: "scanner", Desc: "C — toggle scanner (off-grid sweep)",
		DefaultRect: image.Rect(396+uikit.RailInset, 276, 696+uikit.RailInset, 524), // 300×248 (uikit scanW × titleBarH+scanBodyH)
		Anchor:      AnchorFixed,
		DockPolicy:  DockFloating, SpaceOnly: true, SidecarName: "scanner",
		Group: "World",
		Sync:  func(g *Game) { g.scan.Sync() },
		Build: func(g *Game) *widget.Window { g.scan = uikit.NewScanner(g.env); return g.scan.Win() },
	},
	{
		// An ordinary floating window: N flips its flag like every other
		// keyed row, and the graph's own hover/select/destination live in
		// the view (uikit/screens/mapview.go).
		ID: pMap, Title: "Starmap", Icon: "svc_map",
		Key: ebiten.KeyN, Action: "starmap", Desc: "N — toggle starmap",
		DefaultRect: image.Rect(160, 80, 870, 634), // uikit.NewMapView's landing rect
		DockPolicy:  DockFloating, SidecarName: "map",
		Group: "World",
		Sync:  func(g *Game) { g.mp.Sync() },
		Build: func(g *Game) *widget.Window { g.mp = uikit.NewMapView(g.env); return g.mp.Win() },
	},
	{
		// Modal overlay driven by HelpScene; the window sizes itself from
		// its rows (uikit.NewHelp), so the rect declares the top inset
		// and the design viewport edges only.
		ID: pHelp, Title: "Help", Icon: "svc_help",
		Key: ebiten.KeyH, Action: "help", Desc: "H — toggle help",
		DefaultRect: image.Rect(0, 24, defaultW, defaultH),
		Anchor:      AnchorTopCenter,
		DockPolicy:  DockFloating, Modal: true, SidecarName: "help",
		Group: "Help & System",
		Sync:  func(g *Game) { g.help.Sync() },
		Build: func(g *Game) *widget.Window { g.help = uikit.NewHelp(g.env, helpRows()); return g.help.Win() },
	},
	{
		// Bottom-left pocket beside the HUD pair: left/bottom insets of 10
		// against the design viewport (uikit logDefaultW/H = 264×240).
		// The pocket starts clear of the rail and keeps the historical
		// 361 right edge, so the log narrows by the rail inset to its
		// content floor (its overlap with the HUD pair at the 1024×700
		// minimum is inherent — see TestVirginPlacementTable).
		ID: pLog, Title: "Log", Icon: "svc_log",
		Key: ebiten.KeyL, Shift: true, Action: "log", Desc: "Shift+L — toggle log window",
		DefaultRect: image.Rect(10+uikit.RailInset, defaultH-10-240, 10+264, defaultH-10),
		Anchor:      AnchorBottomLeft,
		DockPolicy:  DockFloating, SidecarName: "log",
		Group: "World",
		Sync:  func(g *Game) { g.logP.Sync() },
		Build: func(g *Game) *widget.Window { g.logP = uikit.NewLog(g.env); return g.logP.Win() },
	},
	{
		// Like every panel it is rebuilt by buildKit, so its live state
		// (scale ladder, fullscreen, vsync, rail, audio volumes, shuffle,
		// resets) is re-read from g in Sync.
		ID: pSettings, Title: "Settings", Icon: "kind_unknown",
		Key: ebiten.KeyP, Action: "settings", Desc: "P — toggle settings (UI scale, display, layout)",
		DefaultRect: image.Rect(490, 130, 790, 590), // uikit.NewSettings' landing rect
		DockPolicy:  DockFloating, SidecarName: "settings",
		Group: "Help & System",
		Sync:  func(g *Game) { g.sett.Sync() },
		Build: func(g *Game) *widget.Window {
			g.sett = uikit.NewSettings(g.env, g.settActions)
			return g.sett.Win()
		},
	},
	{
		// The station services view: the ONE window docking opens, the
		// window that replaced the old station strip. An ordinary floating
		// window whose declared default is the station column (left of the
		// viewport, clear of the rail gutter); its content height drives
		// the column — contracts.go re-places it to the board's height
		// (stationRect). Unkeyed by design — docking opens it, the rail
		// reopens it, and there is nothing to press in space where it
		// cannot mount.
		ID: pServices, Title: "Station Services", Icon: "kind_station",
		Key:         keyNone, // docking opens it; the rail reopens it
		DefaultRect: image.Rect(stationX, stationY, stationX+stationWinW, stationY+stationMinH),
		Anchor:      AnchorFixed,
		DockPolicy:  DockFloating, DockedOnly: true, SidecarName: "services",
		Group: "Station",
		Sync:  func(g *Game) { g.svcUI.Sync() },
		Build: func(g *Game) *widget.Window { return g.buildServicesWindow() },
	},
	{
		// The refinery: an ore→mineral service window, docked-only like the
		// manufacturing panel, centred on first open. Its own key would
		// crowd the bindings; the rail and the services row open it.
		ID: pRefinery, Title: "Refinery", Icon: "svc_refinery",
		Key:         keyNone,
		DefaultRect: image.Rect(430, 160, 850, 560),
		DockPolicy:  DockFloating, DockedOnly: true, SidecarName: "refinery",
		Group: "Industry", Service: "refine",
		Sync:  func(g *Game) { g.ref.Sync() },
		Build: func(g *Game) *widget.Window { g.ref = uikit.NewRefinery(g.env); return g.ref.Win() },
	},
	{
		// The wallet window: a floating read-only ledger, so it opens in
		// space and docked alike (nothing in it needs a station service).
		// K: F and G already belong to fitting and market.
		ID: pFinances, Title: "Finances", Icon: "svc_finances",
		Key: ebiten.KeyK, Action: "finances", Desc: "K — toggle finances window (holdings and transactions)",
		DefaultRect: image.Rect(312, 170, 712, 530),
		DockPolicy:  DockFloating, SidecarName: "finances",
		Group: "Trade",
		Sync:  func(g *Game) { g.fin.Sync() },
		Build: func(g *Game) *widget.Window { g.fin = uikit.NewFinances(g.env); return g.fin.Win() },
	},
	{
		// The Esc menu: scene-owned (MenuScene flips the flag), modal,
		// static buttons — no Sync — and never persisted, so a restart
		// cannot reopen it. It is deliberately NOT in the rail: the pause
		// menu keeps its key and its function.
		ID: pMenu, Title: "Menu",
		Key: ebiten.KeyEscape, Action: "menu", Desc: "Esc — menu (Resume / Save / Load / Quit)",
		DefaultRect: image.Rect(0, 0, 240, 304), // buildMenuWindow's landing rect (5 rows)
		DockPolicy:  DockTransient, Modal: true, SidecarName: "",
		Sync:  nil,
		Build: func(g *Game) *widget.Window { g.menuWin = g.buildMenuWindow(); return g.menuWin },
	},
}

// panelDef is one toggleable panel's whole registration: the single
// place a panel has to appear in for the frame, mount and persistence
// paths to agree. It is DERIVED from the Screen row (derivePanelDefs) —
// the row is what a new window declares.
//
//	name   sidecar key ("" = not persisted — the Esc menu is scene-owned
//	       and must not reopen on the next boot)
//	window the window backing the id (the Build's return value)
//	open   the player's toggle flag (scene-driven for map/help/menu)
//	want   the desired mount for this frame: (game, docked), derived from
//	       SpaceOnly / DockedOnly
//	sync   refresh the panel's widgets from the sim (nil = static
//	       content); run every frame while mounted and once when the
//	       window mounts, so a freshly mounted panel draws current data
type panelDef struct {
	name   string
	window func(*Game) *widget.Window
	open   func(*Game) *bool
	want   func(*Game, bool) bool
	sync   func(*Game)
}

// panelDefs is indexed by panelID in enum order — syncMounts iterates
// it, so the mount order follows the enum (the log lands before the Esc
// menu, which keeps the top of the draw order). Rows are declared in
// enum order, so position == id; screenSpecErrors enforces that.
//
// Assigned in init: the rows' Build closures reach session code that
// reads these same registries (the menu's Save button → writeLayout →
// layoutNames, the help rows → KeyMap), so deriving them in package-level
// initializers would be a circular initialization. init runs after every
// variable exists and before any session is built.
var panelDefs []panelDef

// screenByTitle keys every row by its lowercased Title for
// Toggle{Screen} (game/intents.go). Like panelDefs it is assigned in
// init: the rows' Build closures reach session code that resolves
// titles through it, so a package-level initializer would be a circular
// initialization.
var screenByTitle map[string]Screen

func init() {
	panelDefs = derivePanelDefs(screens)
	screenByTitle = titleIndex(screens)
	KeyMap = slices.Concat(worldKeyMap, screenBindings(screens))
	layoutNames = append([]string{"overview", "selected", "infowin"}, sidecarNames(screens)...)
}

// titleIndex keys rows by lowercased title — Toggle{Screen}'s lookup
// (titles compare case-insensitively, as the intent documents).
func titleIndex(rows []Screen) map[string]Screen {
	m := make(map[string]Screen, len(rows))
	for _, s := range rows {
		m[strings.ToLower(s.Title)] = s
	}
	return m
}

// derivePanelDefs builds panelDefs from any Screen table (the tests run
// it over a fixture row too).
func derivePanelDefs(rows []Screen) []panelDef {
	defs := make([]panelDef, len(rows))
	for i, s := range rows {
		defs[i] = panelDef{
			name:   s.SidecarName,
			window: func(g *Game) *widget.Window { return g.screenWin[s.ID] },
			open:   func(g *Game) *bool { return &g.open[s.ID] },
			want: func(g *Game, docked bool) bool {
				return g.open[s.ID] &&
					(!s.SpaceOnly || !docked) &&
					(!s.DockedOnly || docked)
			},
			sync: s.Sync,
		}
	}
	return defs
}

// sidecarNames returns the persisted windows' sidecar keys in spec order;
// layout.go prefixes them with the two core windows and the Show Info
// window. A transient screen (the Esc menu) is not among them.
func sidecarNames(rows []Screen) []string {
	names := make([]string, 0, len(rows))
	for _, s := range rows {
		if s.SidecarName != "" {
			names = append(names, s.SidecarName)
		}
	}
	return names
}

// screenBindings returns one KeyMap entry per keyed Screen row — the
// help overlay's toggle rows derive from this (game/input.go appends it
// to the world bindings).
func screenBindings(rows []Screen) []Binding {
	out := make([]Binding, 0, len(rows))
	for _, s := range rows {
		if s.Key == keyNone {
			continue
		}
		out = append(out, Binding{Key: s.Key, Shift: s.Shift, Action: s.Action, Desc: s.Desc})
	}
	return out
}

// screenLabel is a screen's registry label: its sidecar key, or the
// lowercased title when it is not persisted (the Esc menu — draw_test's
// panelNames map is built from it, and screenSpecErrors uses it to keep
// the labels unique).
func screenLabel(s Screen) string {
	if s.SidecarName != "" {
		return s.SidecarName
	}
	return strings.ToLower(s.Title)
}

// pressScreen toggles the flag of the keyed, non-modal screen bound to
// (k, shift) and reports whether one claimed the key. The modal screens
// (help, menu) are scene-owned: their keys push/pop the scene in
// handleKeys instead of flipping a flag, so they are skipped here.
func (g *Game) pressScreen(k ebiten.Key, shift bool) bool {
	for _, s := range screens {
		if s.Modal || s.Key == keyNone || s.Key != k || s.Shift != shift {
			continue
		}
		g.open[s.ID] = !g.open[s.ID]
		return true
	}
	return false
}

// anchorRect composes a screen's default rect for a w×h viewport: the
// window keeps the size it measured (placement positions, never resizes),
// and DefaultRect + Anchor decide where that size sits — an absolute
// design coordinate, the viewport centre, or the declared insets to the
// bottom-left / top edges. Every rule lands clear of the rail's left
// gutter: no default placement is allowed inside it (the rail is top
// chrome; a window tucked under it would only fight for pixels).
func anchorRect(d image.Rectangle, a Anchor, dw, dh, w, h int) image.Rectangle {
	var x, y int
	switch a {
	case AnchorFixed:
		x, y = theme.Px(d.Min.X), theme.Px(d.Min.Y)
	case AnchorBottomLeft:
		x = theme.Px(d.Min.X)
		y = h - theme.Px(defaultH-d.Max.Y) - dh
	case AnchorTopCenter:
		r := centerRect(dw, dh, w, h)
		x, y = r.Min.X, theme.Px(d.Min.Y)
	default: // AnchorCenter: the viewport decides both axes.
		return centerRect(dw, dh, w, h)
	}
	return image.Rect(max(x, railClearX()), y, max(x, railClearX())+dw, y+dh)
}

// centerRect centres a dw×dh window in a w×h viewport (the AnchorCenter
// rule, also used by the Show Info window, which is not a Screen row),
// floored clear of the rail gutter like every other default placement —
// a window wider than the viewport minus the gutter starts at the floor
// instead of inside the rail.
func centerRect(dw, dh, w, h int) image.Rectangle {
	x := max((w-dw)/2, railClearX())
	y := (h - dh) / 2
	return image.Rect(x, y, x+dw, y+dh)
}

// railClearX is the left edge every default window placement clears: the
// rail's expanded width plus the design window margin, in live px.
func railClearX() int { return theme.Px(10 + uikit.RailInset) }

// screenSpecErrors reports every way a Screen table can be malformed.
// New refuses to build a session on a bad spec, and the spec tests drive
// this with deliberately broken rows, so the rules cannot rot into
// comments. It is pure: pass any table (the tests pass one with an extra
// fixture row appended).
func screenSpecErrors(rows []Screen) []string {
	var probs []string
	add := func(format string, args ...any) {
		probs = append(probs, fmt.Sprintf(format, args...))
	}
	type keyCombo struct {
		k     ebiten.Key
		shift bool
	}
	ids := map[panelID]string{}
	names := map[string]string{}
	labels := map[string]string{}
	keys := map[keyCombo]string{}
	actions := map[string]string{}

	for i, s := range rows {
		if int(s.ID) != i {
			add("row %d (%s): id %d is out of order — rows are declared in enum order", i, s.Title, s.ID)
		}
		if prev, dup := ids[s.ID]; dup {
			add("row %d (%s): id %d already used by %q", i, s.Title, s.ID, prev)
		}
		ids[s.ID] = s.Title

		if s.Title == "" {
			add("row %d (id %d): zero title", i, s.ID)
		}
		if s.Icon == "" {
			// The rail renders one glyph per openable entry, so every
			// non-transient row names one; the Esc menu has no entry.
			if s.DockPolicy != DockTransient {
				add("row %d (%s): missing icon (the rail renders one glyph per entry)", i, s.Title)
			}
		} else if !icons.Has(s.Icon) {
			add("row %d (%s): unknown icon %q", i, s.Title, s.Icon)
		}
		if s.Build == nil {
			add("row %d (%s): missing builder", i, s.Title)
		}
		if s.SpaceOnly && s.DockedOnly {
			add("row %d (%s): SpaceOnly and DockedOnly together never mounts", i, s.Title)
		}
		if s.Anchor > AnchorTopCenter {
			add("row %d (%s): unknown anchor %d", i, s.Title, s.Anchor)
		}
		if s.DockPolicy > DockTransient {
			add("row %d (%s): unknown dock policy %d", i, s.Title, s.DockPolicy)
		}

		// Key binding: a keyed row carries a complete, unique KeyMap row.
		if s.Key == keyNone {
			if s.Action != "" || s.Desc != "" {
				add("row %d (%s): help text without a key binding", i, s.Title)
			}
		} else {
			if s.Action == "" {
				add("row %d (%s): key %v without an action id", i, s.Title, s.Key)
			} else if prev, dup := actions[s.Action]; dup {
				add("row %d (%s): action %q already used by %q", i, s.Title, s.Action, prev)
			} else {
				actions[s.Action] = s.Title
			}
			if !strings.Contains(s.Desc, " — ") {
				add("row %d (%s): desc %q must read \"key — description\" (the help overlay splits it)", i, s.Title, s.Desc)
			}
			combo := keyCombo{s.Key, s.Shift}
			if prev, dup := keys[combo]; dup {
				add("row %d (%s): key %v (shift=%v) already bound by %q", i, s.Title, s.Key, s.Shift, prev)
			}
			keys[combo] = s.Title
		}

		// Sidecar: transient screens stay out of the file, every other
		// screen persists under a unique key and label.
		switch {
		case s.DockPolicy == DockTransient && s.SidecarName != "":
			add("row %d (%s): transient screen must not carry sidecar name %q", i, s.Title, s.SidecarName)
		case s.DockPolicy != DockTransient && s.SidecarName == "":
			add("row %d (%s): persisted screen needs a sidecar name", i, s.Title)
		case s.SidecarName != "":
			if prev, dup := names[s.SidecarName]; dup {
				add("row %d (%s): sidecar name %q already used by %q", i, s.Title, s.SidecarName, prev)
			}
			names[s.SidecarName] = s.Title
		}
		label := screenLabel(s)
		if prev, dup := labels[label]; dup {
			add("row %d (%s): label %q already used by %q", i, s.Title, label, prev)
		}
		labels[label] = s.Title

		// Rail grouping and the station service the window needs:
		// transients never enter the rail and must not claim a section,
		// every other row must name a known group, and a service must be
		// one the content table can offer (content.Services).
		if s.DockPolicy == DockTransient {
			if s.Group != "" {
				add("row %d (%s): transient screen must not carry rail group %q", i, s.Title, s.Group)
			}
		} else if s.Group == "" {
			add("row %d (%s): missing rail group", i, s.Title)
		} else if !slices.Contains(railGroups[:], s.Group) {
			add("row %d (%s): unknown rail group %q", i, s.Title, s.Group)
		}
		if s.Service != "" {
			if _, ok := content.Service(s.Service); !ok {
				add("row %d (%s): unknown station service %q", i, s.Title, s.Service)
			}
		}
	}
	return probs
}
