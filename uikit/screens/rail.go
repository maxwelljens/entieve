package screens

import (
	"image/color"
	"slices"
	"strconv"

	"github.com/ebitenui/ebitenui/widget"
	"github.com/entieve/entieve/uikit/theme"
	"github.com/entieve/entieve/uikit/widgets"
)

// The left navigation rail (EVE-neocom style): one icon per openable
// window, pinned to the left screen edge on the root overlay — chrome
// like the HUD and module rack, never a window (no title bar, no drag,
// not in the window stack, never persisted as geometry).
//
// The entries are DERIVED by the host from its window table (game:
// railEntries from the Screen spec) — the rail knows icons, titles,
// groups and context, not the host's panel ids. A click emits
// Toggle{Title} on the ActionSink; the host flips the window, so the
// first click opens (a fresh mount lands on top of the stack) and the
// second closes it.
//
// Geometry is DESIGN units through the Px funnel: the expanded width is
// RailInset, exactly the left gutter every default window keeps clear
// (game/placeDefaults and the spec's left-column rects read this
// constant), so the rail never hides behind the overview or the log.
// Collapsing hides the entries (a zero-width column) and leaves only the
// slim edge tab that expands it again; the state lives in the host and
// persists in the settings sidecar.
const (
	// RailInset is the expanded rail width AND the left inset every
	// window keeps clear of it (game reads this constant directly — one
	// source, no drift). It is also the entry cell square: the cells
	// grew 36 → 63 (+75 %) with the buttons, so the gutter follows and
	// windows can never cover an entry.
	RailInset = 63

	// railIcon is the entry glyph's design box: the 20 px Button glyph
	// grown with the cell (36 → 63 is ×1.75, so 20 → 35 keeps the
	// ~56 % fill the rail always had — a 20 px glyph in a 63 px cell
	// reads as a lost dot).
	railIcon = 35

	// railTabW is the edge tab's width in the collapsed rail (all that
	// is left of the column); railTabH its height.
	railTabW = 16
	railTabH = 20

	// RailTopInset is the rail's top margin: the edge tab sits level
	// with the window row's 8 px top margin (the game reads it when it
	// anchors the rail on the root).
	RailTopInset = 8

	// railGap is the vertical rhythm between entries and their group
	// hairlines; railHeadGap the gap under the edge tab.
	railGap     = 2
	railHeadGap = 6
)

// RailEntry is one window the rail offers. The host fills it from its
// window table; the rail owns rendering, context gating and emission.
//
//	Icon, Title, Key, Desc  identity exactly as the host's table
//	                        declares it — Key the display key, Desc the
//	                        "I — toggle inventory" tooltip hint ("" =
//	                        unkeyed)
//	Group                   section name; a hairline separates group
//	                        changes as the entries run down
//	SpaceOnly/DockedOnly    context gating: unavailable while docked /
//	                        in space (grayed, clicks inert)
//	Service                 station service the window needs; docked at
//	                        a station lacking it the entry grays ("" =
//	                        no service dependency)
//	Open                    the host's current open state (nil = never
//	                        open) — drives the entry's accent glow
type RailEntry struct {
	Icon  string
	Title string
	Key   string
	Desc  string

	Group      string
	SpaceOnly  bool
	DockedOnly bool
	Service    string

	Open func() bool
}

// Rail is the left icon rail: Theme + ContentView + ActionSink, like
// every screen. Sync refreshes gating and the open-state glow from the
// view once per frame.
type Rail struct {
	th theme.Theme
	cv widgets.ContentView
	as widgets.ActionSink

	entries []RailEntry
	btns    []*widget.Button
	icons   []*theme.IconBox
	enabled []bool
	tips    []widgets.RowTip

	// The two cell image sets, built ONCE: Sync swaps pointers instead
	// of building nineslices per frame (a per-frame rebuild feeds the
	// engine's deferred-image queue and shows up as linear live-heap
	// growth in TestMemoryFootprintPump).
	imgIdle *widget.ButtonImage
	imgOpen *widget.ButtonImage

	container *widget.Container
	list      *widget.Container
	scroll    *widget.ScrollContainer // viewport over the entry list (capped columns)
	tab       *widget.Button
	maxH      int // current column cap in live px (0 = uncapped)
	collapsed bool
}

// NewRail builds the rail over the given seams. entries arrive in draw
// order (group order first, table order inside a group); an empty table
// still yields the edge tab.
func NewRail(env widgets.Seams, entries []RailEntry) *Rail {
	p := env.Pal()
	r := &Rail{
		th:      env,
		cv:      env,
		as:      env,
		entries: entries,
		btns:    make([]*widget.Button, len(entries)),
		icons:   make([]*theme.IconBox, len(entries)),
		enabled: make([]bool, len(entries)),
		tips:    make([]widgets.RowTip, len(entries)),
	}

	r.imgIdle, r.imgOpen = railCellImages(p, false), railCellImages(p, true)

	r.tab = widget.NewButton(
		widget.ButtonOpts.Image(r.imgIdle),
		widget.ButtonOpts.TextLabel("<"),
		// The tab sizes to its glyph, not to the theme's button padding:
		// collapsed, its width IS the rail's (railTabW).
		widget.ButtonOpts.TextPadding(&widget.Insets{}),
		widget.ButtonOpts.WidgetOpts(
			widget.WidgetOpts.MinSize(theme.Px(railTabW), theme.Px(railTabH)),
			widget.WidgetOpts.LayoutData(widget.RowLayoutData{Stretch: true}),
			widget.WidgetOpts.MouseButtonClickedHandler(func(*widget.WidgetMouseButtonClickedEventArgs) {
				r.SetCollapsed(!r.collapsed)
			}),
		),
	)

	r.list = widget.NewContainer(
		widget.ContainerOpts.Layout(widget.NewRowLayout(
			widget.RowLayoutOpts.Direction(widget.DirectionVertical),
			widget.RowLayoutOpts.Spacing(theme.Px(railGap)),
		)),
		widget.ContainerOpts.WidgetOpts(widget.WidgetOpts.LayoutData(widget.RowLayoutData{Stretch: true})),
	)
	prev := ""
	for i, e := range entries {
		if prev != "" && e.Group != prev {
			r.list.AddChild(railSeparator(p))
		}
		prev = e.Group
		btn, glyph := r.newEntryButton(i, e)
		r.btns[i], r.icons[i] = btn, glyph
		r.list.AddChild(r.iconCell(btn, glyph))
	}

	// The entry list rides a scroll viewport: uncapped it IS the list's
	// full height (the viewport passes straight through), and the host's
	// SetMaxHeight caps it so a tall entry table clips + wheel-scrolls
	// inside the screen instead of running off the bottom edge (twelve
	// 63 px cells do not fit the 720 design viewport).
	r.scroll = widget.NewScrollContainer(
		widget.ScrollContainerOpts.Content(r.list),
		// The column has no chrome of its own: an invisible viewport
		// frame, the mask still clipping the entries to the cap.
		widget.ScrollContainerOpts.Image(&widget.ScrollContainerImage{
			Idle: theme.NineSlice(color.NRGBA{}),
			Mask: theme.NineSlice(theme.Solid(p.BG)),
		}),
		widget.ScrollContainerOpts.WidgetOpts(widget.WidgetOpts.LayoutData(widget.RowLayoutData{
			Stretch: true,
		})),
	)
	// Wheel scrolls the column; ScrollTop is a 0..1 fraction of the
	// overflow, so the step scales by the measured overflow (the
	// tables' convention).
	r.scroll.GetWidget().ScrolledEvent.AddHandler(func(args any) {
		if ev, ok := args.(*widget.WidgetScrolledEventArgs); ok {
			r.scrollBy(ev.Y)
		}
	})

	r.container = widget.NewContainer(
		widget.ContainerOpts.Layout(widget.NewRowLayout(
			widget.RowLayoutOpts.Direction(widget.DirectionVertical),
			widget.RowLayoutOpts.Spacing(theme.Px(railHeadGap)),
		)),
		// The anchor the game re-passes through Kit.AddContainer (the
		// rack does the same), so a standalone mount pins the same way.
		widget.ContainerOpts.WidgetOpts(widget.WidgetOpts.LayoutData(widget.AnchorLayoutData{
			HorizontalPosition: widget.AnchorLayoutPositionStart,
			VerticalPosition:   widget.AnchorLayoutPositionStart,
			Padding:            &widget.Insets{Top: theme.Px(RailTopInset)},
		})),
	)
	r.container.AddChild(r.tab, r.scroll)
	r.Sync()
	return r
}

// Container returns the rail's root widget for the interface root
// (anchor layout, pinned top-left).
func (r *Rail) Container() *widget.Container { return r.container }

// Collapsed reports whether the entries are hidden (only the edge tab
// shows). The host persists this — the rail holds no settings of its own.
func (r *Rail) Collapsed() bool { return r.collapsed }

// SetCollapsed hides (true) or shows the entry column. Idempotent.
func (r *Rail) SetCollapsed(c bool) {
	if r.collapsed == c {
		return
	}
	r.collapsed = c
	// Both hide: the scroll column keeps the container's width (the
	// collapsed rail is exactly the edge tab) and the list behind it
	// reports the entries' own state to the pins.
	theme.ShowWidget(r.scroll.GetWidget(), !c)
	theme.ShowWidget(r.list.GetWidget(), !c)
	label := "<"
	if c {
		label = ">"
	}
	r.tab.SetText(label)
}

// Tab returns the collapse/expand edge tab — the handle tests click.
func (r *Rail) Tab() *widget.Button { return r.tab }

// SetMaxHeight caps the rail column (tab + entry list) in LIVE px — the
// host passes the room under the top inset (the game: viewport height
// minus RailTopInset and its bottom margin). While the entries fit, the
// column keeps its full preferred height; past the cap the column
// becomes a wheel-scrollable viewport (clipped like the tables), so
// every entry stays reachable and the column stays on screen at the
// design sizes. 0 removes the cap.
func (r *Rail) SetMaxHeight(px int) {
	h := 0
	if px > 0 {
		// The cap covers the whole column; the scroll owns what is left
		// under the tab and the gap below it. The tab measures itself —
		// bind the session's widget theme and validate it first, since
		// the game caps at build time, before any layout has resolved
		// its face (the NewStackRow convention: the theme the mounted UI
		// carries is the one bound here).
		r.tab.GetWidget().SetTheme(r.th.WidgetTheme())
		r.tab.Validate()
		_, tabH := r.tab.PreferredSize()
		h = px - tabH - theme.Px(railHeadGap)
		if h < 1 {
			h = 1
		}
	}
	if r.maxH == h {
		return
	}
	r.maxH = h
	r.scroll.GetWidget().LayoutData = widget.RowLayoutData{Stretch: true, MaxHeight: h}
	r.container.RequestRelayout()
	// Re-lay the COLUMN itself, not just its children: a layout-data
	// change alone only dirties this container, while ebitenui
	// propagates a parent relayout from a widget's visibility change —
	// so pulse it (restoring whatever state the collapse left).
	vis := r.scroll.GetWidget().GetVisibility() == widget.Visibility_Show
	theme.ShowWidget(r.scroll.GetWidget(), !vis)
	theme.ShowWidget(r.scroll.GetWidget(), vis)
}

// scrollBy turns wheel notches into a fraction of the column overflow
// (positive notch scrolls up — the tables' convention); two cells per
// notch reads right on a rail. No overflow means nothing to scroll and
// a pinned top (a stale offset would bottom-anchor the list).
func (r *Rail) scrollBy(notches float64) {
	over := r.scrollOverflow()
	if over <= 0 {
		r.scroll.ScrollTop = 0
		return
	}
	top := r.scroll.ScrollTop - notches*2*float64(theme.Px(RailInset+railGap))/over
	r.scroll.ScrollTop = max(0, min(1, top))
}

// scrollOverflow is the scrollable distance in px: the list's preferred
// height minus the viewport (≤ 0 exactly while the entries fit).
func (r *Rail) scrollOverflow() float64 {
	_, h := r.list.PreferredSize()
	return float64(h - r.scroll.ViewRect().Dy())
}

// Button returns the entry button titled title (nil when the rail has no
// such entry) — the handle tests click and captures pin.
func (r *Rail) Button(title string) *widget.Button {
	for i, e := range r.entries {
		if e.Title == title {
			return r.btns[i]
		}
	}
	return nil
}

// Entries returns the rail's table, in draw order (derivation pins).
func (r *Rail) Entries() []RailEntry { return r.entries }

// newEntryButton builds one icon entry's two halves: the chrome button
// (stretched edge to edge of its cell — see iconCell) that emits
// Toggle{Title} when the entry is available here, and the glyph box the
// cell anchors at its centre. Grayed entries emit nothing (the
// established convention) — ebitenui still fires clicks on disabled
// widgets, so the handler gates on the last synced state.
func (r *Rail) newEntryButton(i int, e RailEntry) (*widget.Button, *theme.IconBox) {
	btn := widget.NewButton(
		widget.ButtonOpts.Image(r.imgIdle),
		widget.ButtonOpts.WidgetOpts(
			widget.WidgetOpts.LayoutData(widget.AnchorLayoutData{
				StretchHorizontal: true,
				StretchVertical:   true,
			}),
			widget.WidgetOpts.MouseButtonClickedHandler(func(*widget.WidgetMouseButtonClickedEventArgs) {
				if !r.enabled[i] {
					return // grayed: unavailable in this context
				}
				r.as.Do(widgets.Toggle{Screen: e.Title})
			}),
		),
	)
	// The house icon box: its widget's preferred rect IS the Px-scaled
	// icon (a bare Graphic reports 50×50 unless Image is set), and Set
	// costs a string compare per Sync. The cell anchors it, so its rect
	// is the real icon box the inside-bounds pin reads.
	glyph := theme.NewIconBox(railIcon)
	glyph.Widget().GetWidget().LayoutData = widget.AnchorLayoutData{
		HorizontalPosition: widget.AnchorLayoutPositionCenter,
		VerticalPosition:   widget.AnchorLayoutPositionCenter,
	}
	return btn, glyph
}

// iconCell is one entry's square: the chrome button stretched edge to
// edge under its glyph, the glyph anchored dead centre in the cell.
//
// The glyph used to ride the button's own graphic slot, which ebitenui
// lays out as [empty label][10 px fixed spacing][graphic] inside the
// button's padded anchor — a fixed right-offset that hung the glyph over
// the cell's right edge at every scale. Owning the glyph here centres it
// exactly ((cell − icon)/2 on both axes) and keeps its rect reachable
// for the inside-bounds pin.
func (r *Rail) iconCell(btn *widget.Button, glyph *theme.IconBox) *widget.Container {
	cell := widget.NewContainer(
		widget.ContainerOpts.Layout(widget.NewAnchorLayout()),
		widget.ContainerOpts.WidgetOpts(widget.WidgetOpts.MinSize(theme.Px(RailInset), theme.Px(RailInset))),
	)
	cell.AddChild(btn, glyph.Widget()) // glyph last: it draws over the chrome
	return cell
}

// Sync refreshes every entry's gating and open-state glow from the view:
// context (docked/undocked), the docked station's services and the
// host's open flags. Nothing is rebuilt — state writes only.
func (r *Rail) Sync() {
	p := r.th.Pal()
	docked := false
	var services []string
	if r.cv != nil {
		docked = r.cv.PlayerDocked()
		if info, ok := r.cv.DockedStation(); ok {
			services = info.Services
		}
	}
	for i, e := range r.entries {
		enabled := entryEnabled(e, docked, services)
		open := enabled && e.Open != nil && e.Open()
		r.enabled[i] = enabled
		btn := r.btns[i]
		btn.GetWidget().Disabled = !enabled
		// The glyph lives beside the button in the cell now, so its
		// grayed look is written here — state writes only (Set is a
		// string compare when nothing moved).
		tint := p.Text
		if !enabled {
			tint = iconColor(p.Dim, theme.IconDisabledA)
		}
		r.icons[i].Set(e.Icon, tint)
		img := r.imgIdle
		if open {
			img = r.imgOpen
		}
		btn.SetImage(img)
		r.tips[i].Set(r.th, btn.GetWidget(), entryTipKey(e, enabled), func() widgets.TooltipModel {
			return entryTip(e, enabled)
		})
	}
	// Pin the column's offset to the current overflow (a cap change on
	// resize can make a scrolled column fit again).
	if over := r.scrollOverflow(); over <= 0 {
		r.scroll.ScrollTop = 0
	} else {
		r.scroll.ScrollTop = max(0, min(1, r.scroll.ScrollTop))
	}
}

// entryEnabled is one entry's context rule (pure — unit-tested): the
// space/docked gates first, then the station service the window needs
// (checked only while docked — in space the service windows open exactly
// as their keys always did).
func entryEnabled(e RailEntry, docked bool, services []string) bool {
	if e.SpaceOnly && docked {
		return false
	}
	if e.DockedOnly && !docked {
		return false
	}
	if e.Service != "" && docked && !slices.Contains(services, e.Service) {
		return false
	}
	return true
}

// entryTipKey is the tooltip change key: the model depends on the title,
// the hint and whether the entry is available here — never on the glow.
func entryTipKey(e RailEntry, enabled bool) string {
	return e.Title + "|" + e.Desc + "|" + strconv.FormatBool(enabled)
}

// entryTip is the rail tooltip: icon + title + the hotkey hint (the
// host's Desc, which reads "I — toggle inventory"), plus the grayed
// explanation when the entry is unavailable in this context.
func entryTip(e RailEntry, enabled bool) widgets.TooltipModel {
	m := widgets.TooltipModel{Icon: e.Icon, Title: e.Title}
	switch {
	case !enabled:
		m.Hints = []string{"Unavailable here"}
	case e.Desc != "":
		m.Hints = []string{e.Desc}
	}
	return m
}

// railSeparator is the one-pixel hairline between two groups: stretched
// across the rail (a vertical list's cross axis), exactly Px(1) tall.
func railSeparator(p theme.Palette) *widget.Container {
	return widget.NewContainer(
		widget.ContainerOpts.BackgroundImage(theme.NineSlice(theme.NRGBA(p.Border, 160))),
		widget.ContainerOpts.WidgetOpts(
			widget.WidgetOpts.MinSize(0, theme.Px(1)),
			widget.WidgetOpts.LayoutData(widget.RowLayoutData{Stretch: true}),
		),
	)
}

// railCellImages is the entry cell chrome: quiet idle/hover/press, and
// the OPEN state's accent glow — the module rack's active-cell
// treatment, so "this window is up" reads like "this module is running".
// Disabled (grayed) draws the theme's dim cell over whichever is set.
func railCellImages(p theme.Palette, open bool) *widget.ButtonImage {
	body, edge := theme.NRGBA(p.BG, 210), theme.NRGBA(p.Border, 200)
	if open {
		body, edge = theme.NRGBA(p.Accent, 60), theme.Solid(p.Accent)
	}
	return &widget.ButtonImage{
		Idle:         theme.Bordered(body, edge, 1),
		Hover:        theme.Bordered(theme.NRGBA(p.Accent, 95), theme.Solid(p.Accent), 1),
		Pressed:      theme.Bordered(theme.NRGBA(p.Accent, 110), theme.NRGBA(p.Accent, 220), 1),
		PressedHover: theme.Bordered(theme.NRGBA(p.Accent, 110), theme.Solid(p.Accent), 1),
		Disabled:     theme.Bordered(theme.NRGBA(p.Cell, 140), theme.NRGBA(p.Border, 110), 1),
	}
}
