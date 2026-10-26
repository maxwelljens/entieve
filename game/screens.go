package game

import (
	"fmt"
	"image"
	"slices"
	"strings"

	"github.com/entieve/entieve/content"
	"github.com/entieve/entieve/uikit/icons"
	uikit "github.com/entieve/entieve/uikit/screens"
	"github.com/entieve/entieve/uikit/theme"
	"github.com/entieve/entieve/uikit/widgets"
	"github.com/hajimehoshi/ebiten/v2"
)

// The Screen spec: one table declaring every window the player can open,
// and the single source the window registries derive from. Adding a
// window is one row here plus its Build; the KeyMap toggle rows and their
// help-overlay text, the layout sidecar's name list, placeDefaults and the
// test label map all follow from the rows — each derivation below names
// what it feeds. The Screen host (game/screenhost.go) mounts, syncs and
// dismisses the built panels from the same rows.
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

// ScreenID is a Screen row's frozen identity: its layout-sidecar key and
// its stable name in logs and tests. The value is exactly the key a
// sidecar file carries on disk; renaming one needs a migration
// (migrateLayoutNames).
type ScreenID string

const (
	idInventory     ScreenID = "inventory"
	idFitting       ScreenID = "fitting"
	idMarket        ScreenID = "market"
	idManufacturing ScreenID = "manufacturing"
	idScanner       ScreenID = "scanner"
	idMap           ScreenID = "map"
	idHelp          ScreenID = "help"
	idLog           ScreenID = "log"
	idSettings      ScreenID = "settings"
	idServices      ScreenID = "services"
	idRefinery      ScreenID = "refinery"
	idFinances      ScreenID = "finances"
	idKeybindings   ScreenID = "keybindings"
	idMenu          ScreenID = "menu"
)

// Screen is one window the player can open — the spec row the host and
// the registries derive from.
//
//	ID            the frozen sidecar key, also the row's label; rows are
//	              declared in mount order (which fixes the draw order —
//	              the log lands before the Esc menu). A transient row has
//	              an id but no sidecar entry (DockPolicy)
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
//	Key, Shift    the toggle binding's DEFAULT (keyNone = none); Action +
//	              Desc are the KeyMap row that derives from it, which is
//	              also what the help overlay renders and what the rail's
//	              tooltip quotes as its hint. The live binding is the
//	              default overlaid with the player's overrides
//	              (game/keys.go) — pressScreen resolves by Action. A MODAL
//	              screen's key is wired to its scene instead (Esc → onEscape,
//	              H → help), so Modal rows are never flag-toggled here
//	DefaultRect   default geometry in design units (see Anchor for which
//	              parts the composition reads)
//	DockPolicy    floating (persisted under its ID) / transient (scene-owned,
//	              never persisted)
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
//	InTitle       the row also lives on the title screen: its host mounts
//	              it lazily and exclusively there (Settings, Keybindings)
//	Build         constructs the panel (window + Sync) on the host, once
//	              per Warm and freshly per title mount; the window is the
//	              Build's return value — there is no side-channel registry
type Screen struct {
	ID    ScreenID
	Title string
	Icon  string

	Key    ebiten.Key
	Shift  bool
	Action string
	Desc   string

	DefaultRect image.Rectangle
	Anchor      Anchor

	DockPolicy DockPolicy
	Modal      bool
	SpaceOnly  bool
	DockedOnly bool
	Group      string
	Service    string
	InTitle    bool

	Build func(*ScreenHost) widgets.WindowPanel
}

// screens is THE table, in mount order (the log lands before the menu).
var screens = []Screen{
	{
		// An ordinary floating window, docked or in space: it opens at its
		// own declared default (uikit.NewInventory's landing rect), not in
		// a composed docked band.
		ID: idInventory, Title: "Inventory", Icon: "svc_inventory",
		Key: ebiten.KeyI, Action: "inventory", Desc: "I — toggle inventory",
		DefaultRect: image.Rect(396, 256, 716, 690), Anchor: AnchorFixed,
		DockPolicy: DockFloating, Group: "Trade",
		Build: func(h *ScreenHost) widgets.WindowPanel { return uikit.NewInventory(h.services.Env) },
	},
	{
		ID: idFitting, Title: "Fitting", Icon: "svc_fitting",
		Key: ebiten.KeyF, Action: "fitting", Desc: "F — toggle fitting",
		DefaultRect: image.Rect(370, 8, 1014, 690), Anchor: AnchorFixed,
		DockPolicy: DockFloating,
		Group:      "Ship", Service: "fitting",
		Build: func(h *ScreenHost) widgets.WindowPanel { return uikit.NewFitting(h.services.Env) },
	},
	{
		// The market's per-type default is its landing rect
		// (uikit.NewMarket) — 760×560, the three-pane width that starts on
		// the rail gutter's clear line and stays inside the 1024×700
		// minimum. Docked or in space it opens here; the old docked band
		// stretched it across the viewport.
		ID: idMarket, Title: "Market", Icon: "svc_market",
		Key: ebiten.KeyG, Action: "market", Desc: "G — toggle market window (trade while docked)",
		DefaultRect: image.Rect(97, 8, 857, 568), Anchor: AnchorFixed,
		DockPolicy: DockFloating,
		Group:      "Trade", Service: "market",
		Build: func(h *ScreenHost) widgets.WindowPanel { return uikit.NewMarket(h.services.Env) },
	},
	{
		// A station service that floats: it mounts only while docked and
		// centres like the overlays on first open (its Screen row carries
		// no Anchor, so the centred fallback applies).
		ID: idManufacturing, Title: "Manufacturing", Icon: "svc_manufacturing",
		Key: ebiten.KeyB, Action: "manufacturing", Desc: "B — toggle manufacturing window (manufacture while docked)",
		DefaultRect: image.Rect(312, 120, 792, 600),
		DockPolicy:  DockFloating, DockedOnly: true,
		Group: "Industry", Service: "industry",
		Build: func(h *ScreenHost) widgets.WindowPanel { return uikit.NewManufacturing(h.services.Env) },
	},
	{
		// Space chrome: hidden while docked (legacy parity). Fixed in the
		// centre column under the top row — uikit.NewScanner's own
		// constructor default is 10+RailInset,251 (above the bottom-left
		// log). The x carries the rail inset: the left column (rail,
		// overview, log, station) all start clear of uikit.RailInset.
		ID: idScanner, Title: "Scanner", Icon: "svc_scanner",
		Key: ebiten.KeyC, Action: "scanner", Desc: "C — toggle scanner (off-grid sweep)",
		DefaultRect: image.Rect(396+uikit.RailInset, 276, 696+uikit.RailInset, 524), // 300×248 (uikit scanW × titleBarH+scanBodyH)
		Anchor:      AnchorFixed,
		DockPolicy:  DockFloating, SpaceOnly: true,
		Group: "World",
		Build: func(h *ScreenHost) widgets.WindowPanel { return uikit.NewScanner(h.services.Env) },
	},
	{
		// An ordinary floating window: N flips its flag like every other
		// keyed row, and the graph's own hover/select/destination live in
		// the view (uikit/screens/mapview.go).
		ID: idMap, Title: "Starmap", Icon: "svc_map",
		Key: ebiten.KeyN, Action: "starmap", Desc: "N — toggle starmap",
		DefaultRect: image.Rect(160, 80, 870, 634), // uikit.NewMapView's landing rect
		DockPolicy:  DockFloating,
		Group:       "World",
		Build:       func(h *ScreenHost) widgets.WindowPanel { return uikit.NewMapView(h.services.Env) },
	},
	{
		// Modal overlay driven by HelpScene; the window sizes itself from
		// its rows (uikit.NewHelp), so the rect declares the top inset
		// and the design viewport edges only.
		ID: idHelp, Title: "Help", Icon: "svc_help",
		Key: ebiten.KeyH, Action: "help", Desc: "H — toggle help",
		DefaultRect: image.Rect(0, 24, defaultW, defaultH),
		Anchor:      AnchorTopCenter,
		DockPolicy:  DockFloating, Modal: true,
		Group: "Help & System",
		Build: func(h *ScreenHost) widgets.WindowPanel { return uikit.NewHelp(h.services.Env, h.services.HelpRows()) },
	},
	{
		// Bottom-left pocket beside the HUD pair: left/bottom insets of 10
		// against the design viewport (uikit logDefaultW/H = 264×240).
		// The pocket starts clear of the rail and keeps the historical
		// 361 right edge, so the log narrows by the rail inset to its
		// content floor (its overlap with the HUD pair at the 1024×700
		// minimum is inherent — see TestVirginPlacementTable).
		ID: idLog, Title: "Log", Icon: "svc_log",
		Key: ebiten.KeyL, Shift: true, Action: "log", Desc: "Shift+L — toggle log window",
		DefaultRect: image.Rect(10+uikit.RailInset, defaultH-10-240, 10+264, defaultH-10),
		Anchor:      AnchorBottomLeft,
		DockPolicy:  DockFloating,
		Group:       "World",
		Build:       func(h *ScreenHost) widgets.WindowPanel { return uikit.NewLog(h.services.Env) },
	},
	{
		// Like every panel it is rebuilt by the host's Warm, so its live
		// state (scale ladder, fullscreen, vsync, rail, audio volumes,
		// shuffle, resets) is re-read from the action bundle in Sync.
		ID: idSettings, Title: "Settings", Icon: "kind_unknown",
		Key: ebiten.KeyP, Action: "settings", Desc: "P — toggle settings (UI scale, display, layout)",
		DefaultRect: image.Rect(450, 130, 830, 590), // uikit.NewSettings' landing rect
		DockPolicy:  DockFloating, InTitle: true,
		Group: "Help & System",
		Build: func(h *ScreenHost) widgets.WindowPanel { return uikit.NewSettings(h.services.Env, h.services.Settings) },
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
		ID: idServices, Title: "Station Services", Icon: "kind_station",
		Key:         keyNone, // docking opens it; the rail reopens it
		DefaultRect: image.Rect(stationX, stationY, stationX+stationWinW, stationY+stationMinH),
		Anchor:      AnchorFixed,
		DockPolicy:  DockFloating, DockedOnly: true,
		Group: "Station",
		Build: func(h *ScreenHost) widgets.WindowPanel { return h.services.Station() },
	},
	{
		// The refinery: an ore→mineral service window, docked-only like the
		// manufacturing panel, centred on first open. Its own key would
		// crowd the bindings; the rail and the services row open it.
		ID: idRefinery, Title: "Refinery", Icon: "svc_refinery",
		Key:         keyNone,
		DefaultRect: image.Rect(430, 160, 850, 560),
		DockPolicy:  DockFloating, DockedOnly: true,
		Group: "Industry", Service: "refine",
		Build: func(h *ScreenHost) widgets.WindowPanel { return uikit.NewRefinery(h.services.Env) },
	},
	{
		// The wallet window: a floating read-only ledger, so it opens in
		// space and docked alike (nothing in it needs a station service).
		// K: F and G already belong to fitting and market.
		ID: idFinances, Title: "Finances", Icon: "svc_finances",
		Key: ebiten.KeyK, Action: "finances", Desc: "K — toggle finances window (holdings and transactions)",
		DefaultRect: image.Rect(312, 170, 712, 530),
		DockPolicy:  DockFloating,
		Group:       "Trade",
		Build:       func(h *ScreenHost) widgets.WindowPanel { return uikit.NewFinances(h.services.Env) },
	},
	{
		// The rebinding window: one row per rebindable action (the screen
		// toggles and the world commands), a click arms capture and the
		// next key assigns. The fixed chord layer is not listed (it never
		// rebinds). O: the remaining free letter near the UI cluster.
		ID: idKeybindings, Title: "Keybindings", Icon: "svc_help",
		Key: ebiten.KeyO, Action: "keybindings", Desc: "O — toggle keybindings (rebind screen and world keys)",
		DefaultRect: image.Rect(0, 0, 480, 640), // uikit.NewKeybindings' content floor
		DockPolicy:  DockFloating, InTitle: true,
		Group: "Help & System",
		Build: func(h *ScreenHost) widgets.WindowPanel {
			return keybindPanel{Keybindings: uikit.NewKeybindings(h.services.Env, h.services.Keys), cancel: h.services.Cancel}
		},
	},
	{
		// The Esc menu: scene-owned (MenuScene flips the flag), modal,
		// static buttons — no Sync — and never persisted, so a restart
		// cannot reopen it. It is deliberately NOT in the rail: the pause
		// menu keeps its key and its function.
		ID: idMenu, Title: "Menu",
		Key: ebiten.KeyEscape, Action: "menu", Desc: "Esc — menu (Resume / Save / Load / Quit)",
		DefaultRect: image.Rect(0, 0, 240, 304), // buildMenuWindow's landing rect (5 rows)
		DockPolicy:  DockTransient, Modal: true,
		Build: func(h *ScreenHost) widgets.WindowPanel { return h.services.Menu() },
	},
}

// init derives the two package-level registries from the spec: the
// KeyMap toggle rows and the sidecar name list. The rows' Build closures
// run later (on a host), so nothing here reaches session code.
func init() {
	KeyMap = slices.Concat(worldKeyMap, screenBindings(screens))
	layoutNames = append(coreLayoutNames(), sidecarNames(screens)...)
}

// sidecarNames returns the persisted windows' keys in spec order;
// layout.go prefixes them with the core windows. A transient screen (the
// Esc menu) is not among them.
func sidecarNames(rows []Screen) []string {
	names := make([]string, 0, len(rows))
	for _, s := range rows {
		if s.DockPolicy != DockTransient {
			names = append(names, string(s.ID))
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

// screenLabel is a screen's registry label: its frozen id (the sidecar
// key for persisted rows; the Esc menu's id is not persisted).
func screenLabel(s Screen) string { return string(s.ID) }

// pressScreen performs the toggle a rebindable screen action names: the
// modal help scene, or the flag of the keyed non-modal row (the Screen
// spec is the table; the live key for the action is resolved by the
// caller through Game.actionFor). Reports whether a row claimed it.
func (g *Game) pressScreen(action string) bool {
	id, ok := g.host.ByAction(action)
	if !ok {
		return false
	}
	g.toggleScreenID(id)
	return true
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
	ids := map[ScreenID]string{}
	titles := map[string]string{}
	keys := map[keyCombo]string{}
	actions := map[string]string{}

	for i, s := range rows {
		if s.ID == "" {
			add("row %d: zero id", i)
		} else if string(s.ID) != strings.ToLower(string(s.ID)) || strings.ContainsAny(string(s.ID), " \t") {
			add("row %d (%s): id %q must be a lowercase sidecar key", i, s.Title, s.ID)
		}
		if prev, dup := ids[s.ID]; dup {
			add("row %d (%s): id %q already used by %q", i, s.Title, s.ID, prev)
		}
		ids[s.ID] = s.Title

		if s.Title == "" {
			add("row %d (id %s): zero title", i, s.ID)
		} else if prev, dup := titles[strings.ToLower(s.Title)]; dup {
			add("row %d (%s): title already used by %q", i, s.Title, prev)
		} else {
			titles[strings.ToLower(s.Title)] = s.Title
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

		// Persistence: the id IS the sidecar key, so id uniqueness above
		// keeps the sidecar's windows unique; DockPolicy decides whether
		// the row appears in the file at all.
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
