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
// The rows are declarative. Modality is a row property (the Modal facet):
// the host owns a modal row's open/close, its dim veil and its place in
// the Esc ladder, so no window needs a scene of its own.
// screenSpecErrors is the well-formedness rule for the whole
// table (New refuses to build on a bad spec, the tests drive it with
// deliberately broken rows). The left navigation rail derives from the
// same rows (game/rail.go): every openable, listed row is one rail entry,
// its Icon its glyph, its Group the rail section it lands in — a row opened
// from another window opts out with NoRail / Unlisted.
//
// Default geometry is DESIGN units (theme.Px scales them — see the Px
// funnel, uikit/theme/scale.go) written against the 1280×720 design viewport
// (defaultW×defaultH).

// DockPolicy says where a screen's window lives:
//
//	DockFloating    a free overlay: placeDefaults hangs it on the viewport
//	                from its DefaultRect + Anchor, the sidecar persists it
//	DockTransient   opened by the game and never persisted — the Esc menu,
//	                which must not reopen on the next boot
//
// There is no docked special case any more: a service window opened while
// docked is an ordinary floating window (the market's three-pane rect, the
// inventory/fitting rects) exactly like it is in space. The one window
// docking opens — the station board — declares the station column as its
// DefaultRect and answers its content floor (Fitted, uikit/screens/
// stationboard.go), so the host composes the height.
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
	// AnchorViewport is the full-viewport layer (the proficiency tree and
	// the starmap): the rect IS the viewport. The rail-gutter floor below
	// does not apply, because a layer is never placed in the rail's
	// column: which side of the rail it paints on is the row's layer
	// facet (Screen.OverChrome), not its anchor.
	AnchorViewport
	// AnchorTopRight / AnchorTopLeft hang the window on a viewport corner:
	// Min is the inset pair (from the right and the top / from the left and
	// the top), the window keeps the size its constructor gave it, and the
	// rail-gutter floor does not apply. They are the two always-on panels'
	// historical landing, and the Selected Item window has always sat 2
	// design px inside the floor railClearX would impose, which every capture
	// pins it there.
	AnchorTopRight
	AnchorTopLeft
)

// anchorLast is the highest valid Anchor (screenSpecErrors' bound).
const anchorLast = AnchorTopLeft

// Modal is a Screen row's modality. The host owns all of it: the open and
// close step, the veil it paints between the scene stack and the kit, and
// the row's place in the Esc ladder. A row that declares no Dim is an
// ordinary window and keeps whatever modality its own builder gave the
// window (the title phase's windows own their frame).
//
//	Dim     the veil alpha behind the row while it is up, AND the
//	        modality switch: a row that dims is modal (Esc closes it in
//	        declaration order, the world key layer yields to it). Zero =
//	        no modality.
//	Pauses  the pause layer: while it is up no other key toggles (the Esc
//	        menu and the session it owns). A viewport overlay like the
//	        proficiency tree is modal for the world but not a pause.
//	Session the row this row is a SESSION of: opened from it, this row is
//	        modal and dimmed and closing hands back to it (the Settings
//	        window opened from the Esc menu). Opened any other way it is
//	        the plain floating window its key toggles.
type Modal struct {
	Dim     uint8
	Pauses  bool
	Session ScreenID
}

// keyNone marks a screen with no key binding. It cannot be 0: ebiten's
// zero value is ebiten.KeyA.
const keyNone ebiten.Key = -1

// ScreenID is a Screen row's frozen identity: its layout-sidecar key and
// its stable name in logs and tests. The value is exactly the key a
// sidecar file carries on disk; renaming one needs a migration
// (migrateLayoutNames).
type ScreenID string

const (
	idOverview      ScreenID = "overview"
	idSelected      ScreenID = "selected"
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
	idProficiency   ScreenID = "proficiency"
	// idProficiencyOverview is the tree's summary window: it opens and
	// closes with the layer and is never a rail entry or a key of its own.
	idProficiencyOverview ScreenID = "proficiency-overview"
	idKeybindings         ScreenID = "keybindings"
	idMenu                ScreenID = "menu"
)

// Screen is one window the player can open — the spec row the host and
// the registries derive from.
//
//	ID            the frozen sidecar key, also the row's label; rows are
//	              declared in mount order (which fixes the draw order and
//	              the Esc ladder — the log lands before the Esc menu). A
//	              transient row has an id but no sidecar entry (DockPolicy)
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
//	              tooltip quotes as its hint. Desc is the DESCRIPTION
//	              only ("toggle inventory"): keyMapRow composes the chord
//	              in front of it, and the live key comes from the binding
//	              table (game/keyhints.go), so no surface ever quotes a
//	              chord frozen at declaration time. The live binding is
//	              the default overlaid with the player's overrides
//	              (game/keys.go) — pressScreen resolves by Action, so a
//	              keyed row (modal or not) is flag-toggled here
//	DefaultRect   default geometry in design units (see Anchor for which
//	              parts the composition reads)
//	DockPolicy    floating (persisted under its ID) / transient (opened by
//	              the game, never persisted — the Esc menu)
//	Modal         the row's modality (help, menu, the menu's settings
//	              session): the host paints its veil, closes it on Esc in
//	              declaration order and hands a session back to its owner
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
//	InTitle       the row also lives on the title screen: inTitleRows
//	              (game/title.go) selects it into the title host's table,
//	              which mounts it lazily and exclusively there (Settings,
//	              Keybindings)
//	Persistent    the host keeps the built panel mounted whenever the room
//	              is live and the row's flag only HIDES it (hide, don't
//	              unmount), the Overview and the Selected Item windows,
//	              built by the kit (uikit's OverviewPanel/SelectedPanel).
//	              A Persistent row's visibility is the room's rule AND its
//	              flag (ScreenHost.show)
//	NoRail        the row is not a navigation-rail launcher, while every
//	              other entry point stands (its own key, its title-menu
//	              row) — Settings
//	Unlisted      the row is opened from another window: never a rail
//	              entry and never a title-menu label (the Settings window's
//	              Keybindings button is its door; it has no key of its own,
//	              so a rebound chord cannot become a back door) — Keybindings
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
	Modal      Modal
	// OverChrome lifts an AnchorViewport row's layer over every window and
	// the top chrome (the rail): the host mounts it through
	// Kit.AddFrontWindow and the hit test answers its whole rect before
	// the chrome. Without it a viewport row is the backdrop layer, under
	// the rail (the proficiency tree).
	OverChrome bool
	SpaceOnly  bool
	DockedOnly bool
	Persistent bool
	Group      string
	Service    string
	InTitle    bool
	NoRail     bool
	Unlisted   bool

	Build func(*ScreenHost) widgets.WindowPanel
}

// screens is THE table, in mount order (the log lands before the menu).
var screens = []Screen{
	{
		// The Overview: the grid list, always on in space. The kit builds
		// the panel (its table, the row click's selection wiring) and this
		// row gives it what every window has: the rail entry, the sidecar
		// key ("overview", the very key the retired coreLayout wrote, so
		// existing files restore unchanged) and the corner placement.
		// Persistent: its flag hides it, never unmounts it, and the room
		// (space) hides it while docked.
		ID: idOverview, Title: "Overview", Icon: "rail_overview",
		Key: keyNone, // the rail entry and its × are its doors
		// Min is the inset pair AnchorTopRight reads: 10 design px from
		// the right edge, 8 from the top. The window keeps the size its
		// constructor gave it (the corner anchors never resize).
		DefaultRect: image.Rect(10, 8, 10, 8),
		Anchor:      AnchorTopRight,
		DockPolicy:  DockFloating, SpaceOnly: true, Persistent: true,
		Group: "Ship",
		Build: func(h *ScreenHost) widgets.WindowPanel { return h.services.Overview() },
	},
	{
		// The Selected Item window: the verbs for the current selection,
		// top-left clear of the rail's gutter (8 + RailInset design px,
		// 2 inside the generic rail-clear floor, its historical landing).
		// Same facets as the Overview; no sidecar facet of its own (no
		// table, no filter).
		ID: idSelected, Title: "Selected Item", Icon: "act_lock",
		Key:         keyNone,
		DefaultRect: image.Rect(8+uikit.RailInset, 8, 8+uikit.RailInset, 8),
		Anchor:      AnchorTopLeft,
		DockPolicy:  DockFloating, SpaceOnly: true, Persistent: true,
		Group: "Ship",
		Build: func(h *ScreenHost) widgets.WindowPanel { return h.services.SelectedItem() },
	},
	{
		// An ordinary floating window, docked or in space: it opens at its
		// own declared default (uikit.InventoryRect, the panel's landing
		// rect), not in a composed docked band.
		ID: idInventory, Title: "Inventory", Icon: "rail_inventory",
		Key: ebiten.KeyI, Action: "inventory", Desc: "toggle inventory",
		DefaultRect: uikit.InventoryRect(), Anchor: AnchorFixed,
		DockPolicy: DockFloating, Group: "Trade",
		Build: func(h *ScreenHost) widgets.WindowPanel { return uikit.NewInventory(h.services.Env) },
	},
	{
		ID: idFitting, Title: "Fitting", Icon: "rail_fitting",
		Key: ebiten.KeyF, Action: "fitting", Desc: "toggle fitting",
		DefaultRect: uikit.FittingRect(), Anchor: AnchorFixed,
		DockPolicy: DockFloating,
		Group:      "Ship", Service: "fitting",
		Build: func(h *ScreenHost) widgets.WindowPanel { return uikit.NewFitting(h.services.Env) },
	},
	{
		// The market's per-type default is its landing rect
		// (uikit.MarketRect) — 760×560, the three-pane width that starts on
		// the rail gutter's clear line and stays inside the 1024×700
		// minimum. Docked or in space it opens here; the old docked band
		// stretched it across the viewport.
		ID: idMarket, Title: "Market", Icon: "rail_market",
		Key: ebiten.KeyG, Action: "market", Desc: "toggle market window (trade while docked)",
		DefaultRect: uikit.MarketRect(), Anchor: AnchorFixed,
		DockPolicy: DockFloating,
		Group:      "Trade", Service: "market",
		Build: func(h *ScreenHost) widgets.WindowPanel { return uikit.NewMarket(h.services.Env) },
	},
	{
		// A station service that floats: it mounts only while docked and
		// centres like the overlays on first open (its Screen row carries
		// no Anchor, so the centred fallback applies).
		ID: idManufacturing, Title: "Manufacturing", Icon: "rail_manufacturing",
		Key: ebiten.KeyB, Action: "manufacturing", Desc: "toggle manufacturing window (manufacture while docked)",
		DefaultRect: uikit.ManufacturingRect(),
		DockPolicy:  DockFloating, DockedOnly: true,
		Group: "Industry", Service: "industry",
		Build: func(h *ScreenHost) widgets.WindowPanel { return uikit.NewManufacturing(h.services.Env) },
	},
	{
		// Space chrome: hidden while docked (legacy parity). Fixed in the
		// centre column under the top row — uikit.ScannerRect, which the
		// panel's constructor also lands at (its old 10+RailInset,251 copy
		// was stale; the host always placed from this row). The x carries
		// the rail inset: the left column (rail, overview, log, station)
		// all start clear of uikit.RailInset.
		ID: idScanner, Title: "Scanner", Icon: "svc_scanner",
		Key: ebiten.KeyC, Action: "scanner", Desc: "toggle scanner (off-grid sweep)",
		DefaultRect: uikit.ScannerRect(), // 300×248 (uikit scanW × titleBarH+scanBodyH)
		Anchor:      AnchorFixed,
		DockPolicy:  DockFloating, SpaceOnly: true,
		Group: "World",
		Build: func(h *ScreenHost) widgets.WindowPanel { return uikit.NewScanner(h.services.Env) },
	},
	{
		// The starmap: the house's over-chrome layer (OverChrome), so the
		// host mounts it through Kit.AddFrontWindow: painted over the
		// world, every window and all chrome, including the rail. It has
		// no title bar (the region and constellation ride inside the
		// canvas, the × in its top-right corner) and re-fits itself to
		// the viewport every Sync. N, the rail entry, Esc and the canvas
		// × close it. The search field is the layer's one typing
		// surface; game.handleKeys yields it the frame while focused.
		ID: idMap, Title: "Starmap", Icon: "rail_starmap",
		Key: ebiten.KeyN, Action: "starmap", Desc: "toggle starmap",
		DefaultRect: uikit.MapViewRect(),
		Anchor:      AnchorViewport,
		OverChrome:  true,
		DockPolicy:  DockFloating,
		Group:       "World",
		Build: func(h *ScreenHost) widgets.WindowPanel {
			m := uikit.NewMapView(h.services.Env, h.services.Viewport, func() { h.Toggle(idMap) })
			// A picked destination lands in the session, where the HUD
			// readout and the Overview's gate tint read it.
			m.SetDestination(h.services.Env.SetDestination)
			return m
		},
	},
	{
		// Modal overlay: the host paints its veil and closes it on Esc
		// (declaration order, so the menu declared below outranks it).
		// The window sizes itself from its rows (uikit.NewHelp), so the
		// rect declares the top inset and the design viewport edges only.
		ID: idHelp, Title: "Help", Icon: "rail_help",
		Key: ebiten.KeyH, Action: "help", Desc: "toggle help",
		DefaultRect: image.Rect(0, 24, defaultW, defaultH),
		Anchor:      AnchorTopCenter,
		DockPolicy:  DockFloating, Modal: Modal{Dim: 180},
		Group: "Help & System",
		Build: func(h *ScreenHost) widgets.WindowPanel { return uikit.NewHelp(h.services.Env, h.services.HelpRows()) },
	},
	{
		// Bottom-left pocket beside the HUD pair: left/bottom insets of 10
		// against the design viewport (uikit.LogDefaultW/H = 264×240; the
		// panel's own fallback rect is composed at the 1024×700 minimum).
		// The pocket starts clear of the rail and keeps the historical
		// 361 right edge, so the log narrows by the rail inset to its
		// content floor (its overlap with the HUD pair at the 1024×700
		// minimum is inherent — see TestVirginPlacementTable).
		ID: idLog, Title: "Log", Icon: "rail_log",
		Key: ebiten.KeyL, Shift: true, Action: "log", Desc: "toggle log window",
		DefaultRect: image.Rect(10+uikit.RailInset, defaultH-10-uikit.LogDefaultH,
			10+uikit.RailInset+uikit.LogDefaultW, defaultH-10),
		Anchor:     AnchorBottomLeft,
		DockPolicy: DockFloating,
		Group:      "World",
		Build:      func(h *ScreenHost) widgets.WindowPanel { return uikit.NewLog(h.services.Env) },
	},
	{
		// Like every panel it is rebuilt by the host's Warm, so its live
		// state (scale ladder, fullscreen, vsync, rail, audio volumes,
		// shuffle, resets) is re-read from the action bundle in Sync.
		// Not a rail launcher (NoRail): its key and its title-menu row are
		// its doors. The Keybindings button inside it is the second
		// window's only door, wired per host in Build.
		//
		// Modal only as the Esc menu's SESSION: the menu's Settings button
		// opens it from that row, so it dims, pauses the keys and hands
		// back to the menu when it closes; its own key (and the title
		// host, whose table has no menu row) opens the plain floating
		// window. The session is host state and survives a kit rebuild.
		ID: idSettings, Title: "Settings", Icon: "kind_unknown",
		Key: ebiten.KeyP, Action: "settings", Desc: "toggle settings (UI scale, display, layout)",
		DefaultRect: uikit.SettingsRect(),
		DockPolicy:  DockFloating, InTitle: true, NoRail: true,
		Modal: Modal{Dim: 170, Pauses: true, Session: idMenu},
		Group: "Help & System",
		Build: func(h *ScreenHost) widgets.WindowPanel {
			// The Settings window's Keybindings button: one host, one
			// toggle. The game host flips the row's flag exactly as the
			// rail's entry did; the title host's SetOpen is exclusive, so
			// the button there closes Settings and opens Keybindings (the
			// menu row it used to have).
			act := h.services.Settings
			act.OpenKeybindings = func() { h.Toggle(idKeybindings) }
			return uikit.NewSettings(h.services.Env, act)
		},
	},
	{
		// The contacts view: the ONE window docking opens. An ordinary
		// floating window whose declared default is the station column
		// (left of the viewport, clear of the rail gutter); the panel
		// answers its content floor (FitSize, uikit/screens/contacts.go) and
		// the host composes the column from it (stationRect). Unkeyed by
		// design — docking opens it, the rail reopens it, and there is
		// nothing to press in space where it cannot mount.
		ID: idServices, Title: "Contacts", Icon: "rail_station",
		Key:         keyNone, // docking opens it; the rail reopens it
		DefaultRect: image.Rect(stationX, stationY, stationX+stationWinW, stationY+stationMinH),
		Anchor:      AnchorFixed,
		DockPolicy:  DockFloating, DockedOnly: true,
		Group: "Station",
		Build: func(h *ScreenHost) widgets.WindowPanel { return uikit.NewContacts(h.services.Env) },
	},
	{
		// The refinery: an ore→mineral service window, docked-only like the
		// manufacturing panel, centred on first open. Its own key would
		// crowd the bindings; the rail's Industry section opens it.
		ID: idRefinery, Title: "Refinery", Icon: "rail_refinery",
		Key:         keyNone,
		DefaultRect: uikit.RefineryRect(),
		DockPolicy:  DockFloating, DockedOnly: true,
		Group: "Industry", Service: "refine",
		Build: func(h *ScreenHost) widgets.WindowPanel { return uikit.NewRefinery(h.services.Env) },
	},
	{
		// The wallet window: a floating read-only ledger, so it opens in
		// space and docked alike (nothing in it needs a station service).
		// K: F and G already belong to fitting and market.
		ID: idFinances, Title: "Finances", Icon: "rail_finances",
		Key: ebiten.KeyK, Action: "finances", Desc: "toggle finances window (holdings and transactions)",
		DefaultRect: uikit.FinancesRect(),
		DockPolicy:  DockFloating,
		Group:       "Trade",
		Build:       func(h *ScreenHost) widgets.WindowPanel { return uikit.NewFinances(h.services.Env) },
	},
	{
		// The proficiency tree: the house's backdrop layer (the starmap is
		// the over-chrome one). It is an ordinary Screen row (key, rail
		// entry, sidecar entry) whose panel answers Backdrop(), so the host
		// mounts it through Kit.AddBackdropWindow: painted over the world and
		// below every window and all chrome, with its own space inert but
		// blocking the world. It has no title bar (a layer takes no window chrome: Esc
		// and its key close it) and re-fits itself to the viewport. The
		// name distinguishes the row from its summary window, which is the
		// one titled "Proficiency".
		ID: idProficiency, Title: "Proficiency tree", Icon: "rail_proficiency",
		Key: ebiten.KeyT, Action: "proficiency", Desc: "toggle proficiency tree (levels and prerequisites)",
		DefaultRect: uikit.ProficiencyRect(),
		Anchor:      AnchorViewport,
		DockPolicy:  DockFloating,
		Group:       "Ship",
		Build: func(h *ScreenHost) widgets.WindowPanel {
			return uikit.NewProficiency(h.services.Env, h.services.Proficiency,
				h.services.Viewport, h.services.ProfFamily)
		},
	},
	{
		// The proficiency tree's summary window: the house answer to the
		// header that used to be printed on the layer. It is a plain
		// floating window (title bar, ×, drag) with no rail entry and no
		// key: the game opens it with the tree and closes it with the
		// tree, so its only door is the tree itself. It is titled
		// "Proficiency" (the row's own name lives in the tree's title).
		// It lands in the left column just under the Selected Item window,
		// the one spot the tree's first open leaves free; its placement
		// persists like every other window's.
		ID: idProficiencyOverview, Title: "Proficiency", Icon: "rail_proficiency",
		Key:         keyNone,
		DefaultRect: uikit.ProficiencyOverviewRect(),
		Anchor:      AnchorFixed,
		DockPolicy:  DockFloating,
		Group:       "Ship",
		Unlisted:    true,
		Build: func(h *ScreenHost) widgets.WindowPanel {
			return uikit.NewProficiencyOverview(h.services.Env, h.services.Proficiency)
		},
	},
	{
		// The rebinding window: one row per rebindable action (the screen
		// toggles and the world commands), a click arms capture and the
		// next key assigns. The fixed chord layer is not listed (it never
		// rebinds). Unlisted: the Settings window's Keybindings button is
		// its ONLY door — no rail entry, no title-menu label, and no key of
		// its own, so a rebound chord can never become a back door.
		ID: idKeybindings, Title: "Keybindings", Icon: "svc_help",
		Key:         keyNone, // the Settings button opens it
		Unlisted:    true,
		DefaultRect: uikit.KeybindingsRect(),
		DockPolicy:  DockFloating, InTitle: true,
		Group: "Help & System",
		Build: func(h *ScreenHost) widgets.WindowPanel {
			return keybindPanel{Keybindings: uikit.NewKeybindings(h.services.Env, h.services.Keys), cancel: h.services.Cancel}
		},
	},
	{
		// Modal, and the pause layer: while it is up no other key toggles.
		// The Settings button inside it opens the Settings row as this
		// row's session (see the settings row below). Static buttons — no
		// Sync — and never persisted, so a restart cannot reopen it. It is
		// deliberately NOT in the rail: the pause menu keeps its key and
		// its function. Declared last: Esc closes it before any modal
		// declared above it.
		ID: idMenu, Title: "Menu",
		Key: ebiten.KeyEscape, Action: "menu", Desc: "menu (Resume / Save / Load / Quit)",
		DefaultRect: image.Rect(0, 0, 240, 304), // buildMenuWindow's landing rect (5 rows)
		DockPolicy:  DockTransient, Modal: Modal{Dim: 170, Pauses: true},
		Build: func(h *ScreenHost) widgets.WindowPanel { return h.services.Menu() },
	},
}

// init derives the two package-level registries from the spec: the
// KeyMap rows (composed from the description-only tables) and the sidecar
// name list. The rows' Build closures run later (on a host), so nothing
// here reaches session code.
func init() {
	KeyMap = keyMapRows(slices.Concat(worldKeyMap, screenBindings(screens)))
	layoutNames = append(sidecarNames(screens), infoLayoutName)
}

// keyMapRow composes one KeyMap row from a description-only declaration:
// the chord the binding names, the em dash, the description. Every row is
// built here, so no table can bake a chord into its own text — the rot
// screenSpecErrors guards against on the Screen spec.
func keyMapRow(key ebiten.Key, shift, ctrl bool, action, desc string) Binding {
	return Binding{Key: key, Shift: shift, Ctrl: ctrl, Action: action,
		Desc: chordOf(key, shift, ctrl) + ": " + desc}
}

// keyMapRows composes the description-only tables — worldKeyMap and the
// Screen spec's bindings — into KeyMap rows.
func keyMapRows(rows []Binding) []Binding {
	out := make([]Binding, len(rows))
	for i, b := range rows {
		out[i] = keyMapRow(b.Key, b.Shift, b.Ctrl, b.Action, b.Desc)
	}
	return out
}

// sidecarNames returns the persisted windows' keys in spec order: every
// row but a transient one, the two always-on windows included (their ids
// are the keys the retired core layout wrote). A transient screen (the Esc
// menu) is not among them.
func sidecarNames(rows []Screen) []string {
	names := make([]string, 0, len(rows))
	for _, s := range rows {
		if s.DockPolicy != DockTransient {
			names = append(names, string(s.ID))
		}
	}
	return names
}

// screenBindings returns one description-only KeyMap entry per keyed
// Screen row — the help overlay's toggle rows derive from this
// (game/input.go appends it to the world bindings).
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
// row's flag (a modal row's too — the host owns the modal step, so no key
// is wired to a scene). The Screen spec is the table; the live key for
// the action is resolved by the caller through Game.actionFor. Reports
// whether a row claimed it.
func (g *Game) pressScreen(action string) bool {
	id, ok := g.host.ByAction(action)
	if !ok {
		return false
	}
	g.host.Toggle(id)
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
	case AnchorViewport:
		// The layer is the viewport: no gutter floor, no declared size.
		return image.Rect(0, 0, w, h)
	case AnchorTopRight:
		x, y = w-theme.Px(d.Min.X)-dw, theme.Px(d.Min.Y)
		return image.Rect(x, y, x+dw, y+dh)
	case AnchorTopLeft:
		x, y = theme.Px(d.Min.X), theme.Px(d.Min.Y)
		return image.Rect(x, y, x+dw, y+dh)
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
		if s.Anchor > anchorLast {
			add("row %d (%s): unknown anchor %d", i, s.Title, s.Anchor)
		}
		// The over-chrome facet is a viewport layer's: only a layer that is
		// already the whole screen can cover the rail.
		if s.OverChrome && s.Anchor != AnchorViewport {
			add("row %d (%s): OverChrome needs the AnchorViewport layer", i, s.Title)
		}
		if s.DockPolicy > DockTransient {
			add("row %d (%s): unknown dock policy %d", i, s.Title, s.DockPolicy)
		}
		// Modality: the veil is the switch (a row with no Dim is an
		// ordinary window), so the other two facets need one. The
		// session's owner id is pinned by the session tests: a title table
		// carries a session row without its owner and must stay valid.
		if s.Modal.Dim == 0 && (s.Modal.Pauses || s.Modal.Session != "") {
			add("row %d (%s): modal facet without a dim veil (Dim is the modality switch)", i, s.Title)
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
			if strings.Contains(s.Desc, ": ") {
				add("row %d (%s): desc %q carries a chord; the KeyMap row composes it (Desc is the description only)", i, s.Title, s.Desc)
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
