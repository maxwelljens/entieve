package screens

import (
	"image"
	"image/color"
	"math"
	"sort"
	"strconv"
	"strings"

	eimage "github.com/ebitenui/ebitenui/image"
	"github.com/ebitenui/ebitenui/widget"
	"github.com/entieve/entieve/content"
	"github.com/entieve/entieve/uikit/theme"
	"github.com/entieve/entieve/uikit/widgets"
	"github.com/hajimehoshi/ebiten/v2"
)

// Show Info: the floating item-details window.
//
// One window per session, built like the help window (draggable, kit
// resize chrome, closeable through the × in its title bar, NON-modal)
// and filled on demand: Kit.ShowInfo(ref) swaps the whole page — no back
// stack — and remounts the window so the freshly shown page lands on top
// of the window stack (the remount rides the documented ebitenui
// window-remount leak, see Kit.AddWindow / game.setPanel).
//
// The body is a scroll container in the tooltip's visual language: icon +
// bold title + a category/origin line, a wrapped description prose
// paragraph (the first renderer of the content description fields), then
// grouped sections of right-anchored label/value rows. A section is only
// built when its def actually carries the fields — never "N/A". Rows that
// point at another definition (charges, loot drops, refine yields, the
// caste's hull) are links: right-click opens the Show Info menu,
// double-click swaps the window's content.

// Window geometry: a floating page, wide enough for label/value rows at
// the small step and tall enough to show several sections at once.
const (
	infoW      = 420 // default width
	infoH      = 460 // default height
	infoMinW   = 300 // content floor: header + prose + two rows
	infoMinH   = 340
	infoGap    = 6 // vertical gap between page blocks
	infoWheelN = 3 // wheel notches per scroll step (rows), like the tables
)

// ---------------------------------------------------------------------------
// Page model (pure: def → what the window shows)
// ---------------------------------------------------------------------------

// infoRow is one label/value line of a details page: the tooltip's row
// plus the optional link. Ref marks the row as clickable (right-click
// menu, double-click swap); RefOnValue puts the linked name in the value
// (the caste's hull) instead of the label (loot drops, charges, yields).
type infoRow struct {
	Label      string
	Value      string
	Tint       color.RGBA
	Ref        *widgets.InfoRef
	RefOnValue bool
}

// infoSection is a labelled group of rows ("DAMAGE", "LOOT").
type infoSection struct {
	Label string
	Rows  []infoRow
}

// infoPage is one windowful of content: header, prose and sections.
type infoPage struct {
	Icon     string
	Title    string
	Sub      string // category/origin line under the title
	Prose    string // wrapped description ("" = no prose block at all)
	Sections []infoSection
}

// addSection appends a section only when it has rows — a def without the
// fields simply has no section (never "N/A").
func addSection(p *infoPage, label string, rows []infoRow) {
	if len(rows) == 0 {
		return
	}
	p.Sections = append(p.Sections, infoSection{Label: label, Rows: rows})
}

// refRow builds a link row whose LABEL carries the linked name.
func refRow(label string, ref widgets.InfoRef, value string) infoRow {
	r := ref
	return infoRow{Label: label, Value: value, Ref: &r}
}

// valueRefRow builds a link row whose VALUE carries the linked name
// (label "Hull", value "Kestrel").
func valueRefRow(label, value string, ref widgets.InfoRef) infoRow {
	r := ref
	return infoRow{Label: label, Value: value, Ref: &r, RefOnValue: true}
}

// ---------------------------------------------------------------------------
// Industry chain references (the MANUFACTURED FROM / USED IN sections)
// ---------------------------------------------------------------------------

// recipeFor returns the first recipe (canonical id order) producing id.
// v1 ships exactly one recipe per product; the schema is a list, so "first
// in canonical order" is the documented resolution.
func recipeFor(cv widgets.ContentView, id string) *content.RecipeDef {
	if cv.Defs() == nil || id == "" {
		return nil
	}
	for _, r := range cv.Defs().RecipesInOrder() {
		if string(r.Product) == id {
			return r
		}
	}
	return nil
}

// recipesUsing returns every recipe consuming id as an input, canonical
// order.
func recipesUsing(cv widgets.ContentView, id string) []*content.RecipeDef {
	if cv.Defs() == nil || id == "" {
		return nil
	}
	var out []*content.RecipeDef
	for _, r := range cv.Defs().RecipesInOrder() {
		for _, in := range r.Inputs {
			if string(in.Item) == id {
				out = append(out, r)
				break
			}
		}
	}
	return out
}

// chainVal is a per-run quantity cell ("×720", "×2.5"): whole numbers
// take the grouped integer form, fractional recipe quantities keep their
// decimals (RecipeInput.Qty is float).
func chainVal(qty float64) string {
	if qty == math.Trunc(qty) {
		return "×" + theme.FormatInt(int64(qty))
	}
	return "×" + strconv.FormatFloat(qty, 'g', -1, 64)
}

// manufacturedRows builds the MANUFACTURED FROM rows: this product's
// recipe inputs as label-side link rows carrying their per-run quantity
// ("Ferrite ×720"). A hull's recipe also reports its family and base time
// (the craft curve the sim applies lives in the sim — this is the raw
// recipe figure).
func manufacturedRows(cv widgets.ContentView, id string) []infoRow {
	r := recipeFor(cv, id)
	if r == nil {
		return nil
	}
	var rows []infoRow
	if _, isShip := cv.Defs().Ship(content.ShipID(id)); isShip {
		rows = append(rows,
			infoRow{Label: "Family", Value: spacedKind(r.Family)},
			infoRow{Label: "Base time", Value: theme.GroupFloat(r.Time, 1) + " s"})
	}
	for _, in := range r.Inputs {
		name := widgets.ChainName(cv, in.Item)
		if ref, ok := widgets.RefOfID(cv, string(in.Item)); ok {
			rows = append(rows, refRow(name, ref, chainVal(in.Qty)))
			continue
		}
		rows = append(rows, infoRow{Label: name, Value: chainVal(in.Qty)})
	}
	return rows
}

// usedInRows builds the USED IN rows: every recipe consuming this def,
// linked on its product. The value describes the LABELED definition (the
// product's per-run output), so both sections read the same way: link on
// the label, quantity of the linked def on the right.
func usedInRows(cv widgets.ContentView, id string) []infoRow {
	var rows []infoRow
	for _, r := range recipesUsing(cv, id) {
		name := widgets.ChainName(cv, r.Product)
		if ref, ok := widgets.RefOfID(cv, string(r.Product)); ok {
			rows = append(rows, refRow(name, ref, chainVal(float64(r.Qty))))
			continue
		}
		rows = append(rows, infoRow{Label: name, Value: chainVal(float64(r.Qty))})
	}
	return rows
}

// sectionsOf converts the shared tooltip sections (moduleTip's output)
// into page rows: the module stat blocks are one builder, two renderers.
func sectionsOf(secs []widgets.TooltipSection) []infoSection {
	out := make([]infoSection, 0, len(secs))
	for _, s := range secs {
		rows := make([]infoRow, 0, len(s.Rows))
		for _, r := range s.Rows {
			rows = append(rows, infoRow{Label: r.Label, Value: r.Value, Tint: r.Tint})
		}
		out = append(out, infoSection{Label: s.Label, Rows: rows})
	}
	return out
}

// originLabel renders a ShipDef.Origin heritage tag ("moravian",
// "old-works") as display text ("Moravian", "Old works").
func originLabel(s string) string {
	return theme.KindLabel(strings.ReplaceAll(s, "-", " "))
}

// spacedKind renders a content kind/word with its separators spaced out
// and title-cased ("shield_boost" → "Shield boost").
func spacedKind(s string) string {
	return theme.KindLabel(strings.ReplaceAll(s, "_", " "))
}

// infoPageFor builds the page for ref (false when the def is unknown —
// Show then opens nothing rather than a hollow window).
func infoPageFor(cv widgets.ContentView, ref widgets.InfoRef) (infoPage, bool) {
	if !widgets.InfoRefExists(cv, ref) {
		return infoPage{}, false
	}
	switch ref.Kind {
	case widgets.InfoShip:
		def, _ := cv.Defs().Ship(content.ShipID(ref.ID))
		return shipPage(cv, def), true
	case widgets.InfoModule:
		def, _ := cv.Defs().Module(content.ModuleID(ref.ID))
		return modulePage(cv, def), true
	case widgets.InfoItem:
		def, _ := cv.Defs().Item(content.ItemID(ref.ID))
		return itemPage(cv, def), true
	case widgets.InfoNpc:
		def, _ := cv.Defs().Npc(content.NpcID(ref.ID))
		return npcPage(cv, def), true
	}
	return infoPage{}, false
}

// shipPage renders a hull: construction (class · origin), slots, the
// survival block with resists, capacitor, propulsion, sensors and cargo.
func shipPage(cv widgets.ContentView, def *content.ShipDef) infoPage {
	p := infoPage{Icon: "row_hull", Title: def.Name, Prose: def.Description}
	if def.Class != "" {
		p.Sub = spacedKind(def.Class)
	}
	if def.Origin != "" {
		o := originLabel(def.Origin)
		if p.Sub == "" {
			p.Sub = o
		} else {
			p.Sub += " · " + o
		}
	}

	var cons []infoRow
	if def.Class != "" {
		cons = append(cons, infoRow{Label: "Class", Value: spacedKind(def.Class)})
	}
	if def.Origin != "" {
		cons = append(cons, infoRow{Label: "Origin", Value: originLabel(def.Origin)})
	}
	addSection(&p, "CONSTRUCTION", cons)

	var slots []infoRow
	for _, s := range []struct {
		label string
		n     int
	}{{"High", def.Slots.High}, {"Mid", def.Slots.Mid}, {"Low", def.Slots.Low}} {
		if s.n > 0 {
			slots = append(slots, infoRow{Label: s.label, Value: theme.FormatInt(int64(s.n))})
		}
	}
	addSection(&p, "SLOTS", slots)

	var surv []infoRow
	for _, l := range []struct {
		label string
		v     float64
	}{{"Shield", def.Shield}, {"Armor", def.Armor}, {"Structure", def.Structure}} {
		if l.v > 0 {
			surv = append(surv, infoRow{Label: l.label, Value: theme.GroupFloat(l.v, 0) + " HP"})
		}
	}
	if def.Resists != (content.Resists{}) {
		for _, r := range []struct {
			label string
			v     float64
		}{
			{"EM resist", def.Resists.EM},
			{"Thermal resist", def.Resists.Thermal},
			{"Kinetic resist", def.Resists.Kinetic},
			{"Explosive resist", def.Resists.Explosive},
		} {
			surv = append(surv, infoRow{Label: r.label, Value: theme.FormatPct(r.v)})
		}
	}
	addSection(&p, "SURVIVAL", surv)

	var cap []infoRow
	if def.Capacitor > 0 {
		cap = append(cap, infoRow{Label: "Capacity", Value: theme.GroupFloat(def.Capacitor, 1) + " GJ"})
	}
	if def.Recharge > 0 {
		cap = append(cap, infoRow{Label: "Recharge", Value: theme.GroupFloat(def.Recharge, 1) + " s"})
	}
	addSection(&p, "CAPACITOR", cap)

	var prop []infoRow
	if def.MaxSpeed > 0 {
		prop = append(prop, infoRow{Label: "Max speed", Value: theme.FormatSpeed(def.MaxSpeed)})
	}
	if def.Agility > 0 {
		prop = append(prop, infoRow{Label: "Align time", Value: theme.GroupFloat(def.Agility*10, 1) + " s"})
	}
	if def.Mass > 0 {
		prop = append(prop, infoRow{Label: "Mass", Value: theme.GroupFloat(def.Mass, 0) + " kg"})
	}
	if def.WarpSpeed > 0 {
		prop = append(prop, infoRow{Label: "Warp speed", Value: theme.GroupFloat(def.WarpSpeed, 1) + " AU/s"})
	}
	addSection(&p, "PROPULSION", prop)

	var sens []infoRow
	if def.SigRadius > 0 {
		sens = append(sens, infoRow{Label: "Sig radius", Value: theme.GroupFloat(def.SigRadius, 1) + " m"})
	}
	if def.ScanResolution > 0 {
		sens = append(sens, infoRow{Label: "Scan resolution", Value: theme.GroupFloat(def.ScanResolution, 0)})
	}
	if def.MaxTargetRange > 0 {
		sens = append(sens, infoRow{Label: "Max target range", Value: theme.FormatDistance(def.MaxTargetRange)})
	}
	if def.MaxTargets > 0 {
		sens = append(sens, infoRow{Label: "Max targets", Value: theme.FormatInt(int64(def.MaxTargets))})
	}
	addSection(&p, "SENSORS", sens)

	var cargo []infoRow
	if def.Cargo > 0 {
		cargo = append(cargo, infoRow{Label: "Cargo", Value: theme.FormatVolume(def.Cargo)})
	}
	if def.OreHold > 0 {
		cargo = append(cargo, infoRow{Label: "Ore hold", Value: theme.FormatVolume(def.OreHold)})
	}
	addSection(&p, "CARGO", cargo)

	// Manufacturing chain: only when a recipe actually references this def.
	addSection(&p, "MANUFACTURED FROM", manufacturedRows(cv, string(def.ID)))
	addSection(&p, "USED IN", usedInRows(cv, string(def.ID)))
	return p
}

// modulePage renders a module: its own ACTIVATION block first (slot,
// kind, cycle, capacitor draw), then the shared moduleTip stat sections,
// the compatible charges as links and the market price.
func modulePage(cv widgets.ContentView, def *content.ModuleDef) infoPage {
	p := infoPage{Icon: theme.ModuleIcon(def.Kind), Title: def.Name, Prose: def.Description}
	if def.Kind != "" {
		p.Sub = spacedKind(def.Kind)
	}

	var act []infoRow
	if def.Slot != "" {
		act = append(act, infoRow{Label: "Slot", Value: spacedKind(def.Slot)})
	}
	if def.Kind != "" {
		act = append(act, infoRow{Label: "Kind", Value: spacedKind(def.Kind)})
	}
	if def.ActivationTime > 0 {
		act = append(act, infoRow{Label: "Cycle", Value: theme.GroupFloat(def.ActivationTime, 1) + " s"})
	}
	if def.ActivationCost > 0 {
		act = append(act, infoRow{Label: "Cap use", Value: theme.GroupFloat(def.ActivationCost, 1) + " GJ"})
	}
	addSection(&p, "ACTIVATION", act)

	// The rack/fitting tooltip's stat sections, shared verbatim (charge
	// context is an instance concern the def-only window has none of).
	p.Sections = append(p.Sections, sectionsOf(widgets.ModuleSections(def, "", 0))...)

	if def.Weapon != nil && def.Weapon.AmmoClass != "" {
		var rows []infoRow
		rows = append(rows, infoRow{Label: "Class", Value: spacedKind(def.Weapon.AmmoClass)})
		for _, id := range ammoItems(cv, def.Weapon.AmmoClass) {
			item := cv.Defs().Items[id]
			rows = append(rows, refRow(item.Name, widgets.ItemRef(id), theme.FormatVolume(item.Volume)))
		}
		addSection(&p, "CHARGES", rows)
	}
	if def.Price > 0 {
		addSection(&p, "MARKET", []infoRow{{Label: "Price", Value: theme.FormatCredits(def.Price)}})
	}

	// Manufacturing chain: only when a recipe actually references this def.
	addSection(&p, "MANUFACTURED FROM", manufacturedRows(cv, string(def.ID)))
	addSection(&p, "USED IN", usedInRows(cv, string(def.ID)))
	return p
}

// ammoItems lists every loaded item of an ammo class, by id for
// determinism (the charges block of the module window).
func ammoItems(cv widgets.ContentView, class string) []content.ItemID {
	var ids []content.ItemID
	for id, it := range cv.Defs().Items {
		if it.AmmoClass == class {
			ids = append(ids, id)
		}
	}
	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
	return ids
}

// itemPage renders an item: properties, market price, the refine yields
// as links and the ammo damage profile when it has one.
func itemPage(cv widgets.ContentView, def *content.ItemDef) infoPage {
	p := infoPage{Title: def.Name, Prose: def.Description}
	if def.Category != "" {
		p.Sub = spacedKind(def.Category)
	}

	var props []infoRow
	if def.Category != "" {
		props = append(props, infoRow{Label: "Category", Value: spacedKind(def.Category)})
	}
	if def.Volume > 0 {
		props = append(props, infoRow{Label: "Volume", Value: theme.FormatVolume(def.Volume)})
	}
	addSection(&p, "PROPERTIES", props)

	if def.BasePrice > 0 {
		addSection(&p, "MARKET", []infoRow{{Label: "Base price", Value: theme.FormatCredits(def.BasePrice)}})
	}

	if len(def.RefineYield) > 0 {
		ids := make([]content.ItemID, 0, len(def.RefineYield))
		for id := range def.RefineYield {
			ids = append(ids, id)
		}
		sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
		var rows []infoRow
		for _, id := range ids {
			name := string(id)
			if d, ok := cv.Defs().Item(id); ok {
				name = d.Name
			}
			if ref, ok := widgets.RefOfID(cv, string(id)); ok {
				rows = append(rows, refRow(name, ref, theme.FormatPct(def.RefineYield[id])))
				continue
			}
			rows = append(rows, infoRow{Label: name, Value: theme.FormatPct(def.RefineYield[id])})
		}
		addSection(&p, "REFINES TO", rows)
	}

	if d := def.Damage; d != nil {
		addSection(&p, "DAMAGE", []infoRow{
			{Label: "Per shot", Value: theme.GroupFloat(d.Total(), 1)},
			{Label: "Electro-magnetic", Value: theme.GroupFloat(d.EM, 1)},
			{Label: "Thermal", Value: theme.GroupFloat(d.Thermal, 1)},
			{Label: "Kinetic", Value: theme.GroupFloat(d.Kinetic, 1)},
			{Label: "Explosive", Value: theme.GroupFloat(d.Explosive, 1)},
		})
	}

	// Manufacturing chain: only when a recipe actually references this def.
	addSection(&p, "MANUFACTURED FROM", manufacturedRows(cv, string(def.ID)))
	addSection(&p, "USED IN", usedInRows(cv, string(def.ID)))
	return p
}

// npcPage renders a caste: the dossier prose (only when the content
// stream gave it one — never placeholder text), the dossier block with
// its hull as a link, behavior ranges and the loot table as links.
func npcPage(cv widgets.ContentView, def *content.NpcDef) infoPage {
	p := infoPage{Icon: "kind_ship", Title: def.Name, Prose: def.Description}
	hull, hasHull := cv.Defs().Ship(def.Ship)
	if hasHull && hull.Class != "" {
		p.Sub = spacedKind(hull.Class)
	}

	var dos []infoRow
	if def.Faction != "" {
		dos = append(dos, infoRow{Label: "Faction", Value: def.Faction})
	}
	if def.Ship != "" {
		name := string(def.Ship)
		if hasHull {
			name = hull.Name
		}
		if hasHull {
			dos = append(dos, valueRefRow("Hull", name, widgets.ShipRef(def.Ship)))
		} else {
			dos = append(dos, infoRow{Label: "Hull", Value: name})
		}
	}
	if def.Bounty > 0 {
		dos = append(dos, infoRow{Label: "Bounty", Value: theme.FormatCredits(def.Bounty)})
	}
	addSection(&p, "DOSSIER", dos)

	var beh []infoRow
	if def.AggroRange > 0 {
		beh = append(beh, infoRow{Label: "Aggro range", Value: theme.FormatDistance(def.AggroRange)})
	}
	if def.OrbitRange > 0 {
		beh = append(beh, infoRow{Label: "Orbit range", Value: theme.FormatDistance(def.OrbitRange)})
	}
	if def.FleeThreshold > 0 {
		beh = append(beh, infoRow{Label: "Flee threshold", Value: theme.PctLabel(def.FleeThreshold)})
	}
	addSection(&p, "BEHAVIOR", beh)

	var loot []infoRow
	for _, d := range def.Loot {
		name := string(d.Item)
		if it, ok := cv.Defs().Item(d.Item); ok {
			name = it.Name
		}
		value := theme.PctLabel(d.Chance)
		switch {
		case d.MinQty > 0 && d.MinQty != d.MaxQty:
			value += " · ×" + theme.FormatInt(int64(d.MinQty)) + "–" + theme.FormatInt(int64(d.MaxQty))
		case d.MaxQty > 0:
			value += " · ×" + theme.FormatInt(int64(d.MaxQty))
		}
		if ref, ok := widgets.RefOfID(cv, string(d.Item)); ok {
			loot = append(loot, refRow(name, ref, value))
			continue
		}
		loot = append(loot, infoRow{Label: name, Value: value})
	}
	addSection(&p, "LOOT", loot)
	return p
}

// ---------------------------------------------------------------------------
// The window
// ---------------------------------------------------------------------------

// InfoRefRow is a rendered link row: the ref and the widget its clicks
// land on (tests fire those events; the game never needs the list).
type InfoRefRow struct {
	Ref    widgets.InfoRef
	Widget *widget.Widget
}

// InfoWin is the Show Info window: one floating, non-modal, draggable
// and resize-chromed window whose page swaps in place.
type InfoWin struct {
	th theme.Theme
	cv widgets.ContentView
	as widgets.ActionSink

	win    *widget.Window
	title  *widget.Text
	page   *widget.Container
	scroll *widget.ScrollContainer
	prose  *widget.Text

	proseW  float64 // inner width the prose is wrapped to
	refRows []InfoRefRow
	ref     widgets.InfoRef
	hasRef  bool
	dbl     widgets.RowDouble // ref-row double-click bookkeeping (the installer's half)
	onClose func()            // Kit-hosted: unmount (a bare env only hides)
}

// NewInfoWin builds the closed window over the given environment. It
// starts hidden: the window appears only on demand and its geometry is
// what the layout sidecar persists.
func NewInfoWin(env widgets.Seams) *InfoWin {
	p := env.Pal()
	w := &InfoWin{th: env, cv: env, as: env}

	w.page = widget.NewContainer(
		widget.ContainerOpts.Layout(widget.NewRowLayout(
			widget.RowLayoutOpts.Direction(widget.DirectionVertical),
			widget.RowLayoutOpts.Spacing(theme.Px(infoGap)),
			widget.RowLayoutOpts.Padding(widget.NewInsetsSimple(theme.Px(theme.ContentsPad))),
		)),
	)
	w.scroll = widget.NewScrollContainer(
		widget.ScrollContainerOpts.Content(w.page),
		widget.ScrollContainerOpts.StretchContentWidth(),
		widget.ScrollContainerOpts.Image(&widget.ScrollContainerImage{
			Idle:     theme.Bordered(theme.Solid(p.BG), theme.NRGBA(p.Border, 160), 1),
			Disabled: theme.Bordered(theme.NRGBA(p.Cell, 140), theme.NRGBA(p.Border, 110), 1),
			Mask:     eimage.NewNineSliceColor(theme.Solid(p.BG)),
		}),
	)
	// The viewport itself does not scroll (the tables own their offset
	// the same way): one handler turns wheel notches into ScrollTop.
	w.scroll.GetWidget().ScrolledEvent.AddHandler(func(args any) {
		if ev, ok := args.(*widget.WidgetScrolledEventArgs); ok {
			w.scrollBy(ev.Y)
		}
	})

	body := widget.NewContainer(
		widget.ContainerOpts.BackgroundImage(theme.Bordered(theme.Solid(p.BG), theme.NRGBA(p.Border, 200), 1)),
		widget.ContainerOpts.Layout(widget.NewGridLayout(
			widget.GridLayoutOpts.Columns(1),
			widget.GridLayoutOpts.Stretch([]bool{true}, []bool{true}),
			widget.GridLayoutOpts.Padding(widget.NewInsetsSimple(theme.Px(theme.ContentsPad))),
		)),
	)
	body.AddChild(w.scroll)

	// Title bar: name + "— Info", and the window's close affordance (the
	// only mouse-only close in the kit — this window has no key).
	bar, title := theme.NewTitleBar(env, "Info", "")
	w.title = title
	bar.AddChild(widget.NewButton(
		widget.ButtonOpts.TextLabel("×"),
		widget.ButtonOpts.WidgetOpts(
			widget.WidgetOpts.MinSize(theme.Px(46), theme.Px(40)), // ≥40 scaled hit box
			widget.WidgetOpts.LayoutData(widget.AnchorLayoutData{
				HorizontalPosition: widget.AnchorLayoutPositionEnd,
				VerticalPosition:   widget.AnchorLayoutPositionCenter,
			}),
		),
		widget.ButtonOpts.ClickedHandler(func(*widget.ButtonClickedEventArgs) { w.Close() }),
	))

	w.win = widget.NewWindow(
		widget.WindowOpts.Contents(body),
		widget.WindowOpts.TitleBar(bar, theme.Px(theme.TitleBarH)),
		widget.WindowOpts.Draggable(),
		// Resize comes from the kit's edge grips, not Resizeable() —
		// same as every other kit window (Kit.AddWindow attaches them).
		widget.WindowOpts.MinSize(theme.Px(infoMinW), theme.Px(infoMinH)),
		// Fallback only: the composed rect at the 1024×700 minimum.
		// The game recomposes (centers) it for the real viewport at
		// startup (game/placeDefaults).
		widget.WindowOpts.Location(image.Rect(theme.Px(302), theme.Px(120), theme.Px(722), theme.Px(580))),
	)
	theme.ShowWidget(w.win.GetContainer().GetWidget(), false)
	return w
}

// Win returns the window for Kit.AddWindow / the layout sidecar.
func (w *InfoWin) Win() *widget.Window { return w.win }

// Sync is the Panel seam (Kit.Sync drives it); its one job is
// re-wrapping the description to the page's current width.
func (w *InfoWin) Sync() { w.rewrap() }

// IsOpen reports whether the window is revealed (shown, and mounted when
// a Kit hosts it).
func (w *InfoWin) IsOpen() bool {
	return w.win != nil && w.win.GetContainer().GetWidget().IsVisible()
}

// Ref is what the window currently shows (ok = a page was ever built).
func (w *InfoWin) Ref() (widgets.InfoRef, bool) { return w.ref, w.hasRef }

// TitleText is the title-bar label (tests pin "<Name> — Info").
func (w *InfoWin) TitleText() string {
	if w.title == nil {
		return ""
	}
	return w.title.Label
}

// RefRows exposes the current page's link rows (the widgets the right-
// click / double-click handlers live on).
func (w *InfoWin) RefRows() []InfoRefRow { return w.refRows }

// Show swaps the window's whole page to ref's details and reveals the
// window. It reports whether the ref resolved: unknown ids open nothing.
// Mounting (and the raise-to-top remount) is the host's job — see
// Kit.ShowInfo.
func (w *InfoWin) Show(ref widgets.InfoRef) bool {
	page, ok := infoPageFor(w.cv, ref)
	if !ok {
		return false
	}
	w.ref, w.hasRef = ref, true
	w.render(page)
	w.title.Label = page.Title + " — Info"
	w.scroll.ScrollTop = 0
	theme.ShowWidget(w.win.GetContainer().GetWidget(), true)
	return true
}

// Close hides the window: through the Kit it also unmounts (so a closed
// window stops gating world input); in a bare environment it just hides.
func (w *InfoWin) Close() {
	if w.onClose != nil {
		w.onClose()
		return
	}
	w.hide()
}

// hide flips the container invisible (the kit's unmount path).
func (w *InfoWin) hide() {
	theme.ShowWidget(w.win.GetContainer().GetWidget(), false)
}

// scrollBy turns wheel notches into rows of scroll offset over the page
// overflow — the table's convention (positive notch scrolls up).
func (w *InfoWin) scrollBy(notches float64) {
	_, h := w.page.PreferredSize()
	over := float64(h) - float64(w.scroll.ViewRect().Dy())
	if over <= 0 {
		w.scroll.ScrollTop = 0
		return
	}
	step := notches * infoWheelN * float64(widgets.TipLineH(w.th.Face(theme.StepSmall)))
	top := w.scroll.ScrollTop - step/over
	w.scroll.ScrollTop = max(0, min(1, top))
}

// rewrap keeps the description paragraph at the page's inner width: the
// window is resizable, widget.Text wraps at MaxWidth rather than at its
// rect, so the two are pushed back together when they drift.
func (w *InfoWin) rewrap() {
	if w.prose == nil {
		return
	}
	d := w.page.GetWidget().Rect.Dx() - 2*theme.Px(theme.ContentsPad)
	if d <= 0 {
		return
	}
	want := float64(d)
	if want != w.proseW {
		w.proseW = want
		w.prose.MaxWidth = want
		w.page.RequestRelayout()
	}
}

// render rebuilds the page container from a model: header, prose and the
// section rows (link rows get their click seam).
func (w *InfoWin) render(p infoPage) {
	env, pal := w.th, w.th.Pal()
	w.page.RemoveChildren()
	w.refRows = nil
	w.prose = nil

	// Header: the class glyph and the bold title share one line, the
	// category/origin line hangs under the title column (tooltip anatomy).
	iconW := 0
	line := widget.NewContainer(
		widget.ContainerOpts.Layout(widget.NewRowLayout(
			widget.RowLayoutOpts.Direction(widget.DirectionHorizontal),
			widget.RowLayoutOpts.Spacing(theme.Px(theme.IconGap)),
		)),
		widget.ContainerOpts.WidgetOpts(widget.WidgetOpts.LayoutData(widget.RowLayoutData{Stretch: true})),
	)
	if p.Icon != "" {
		g := theme.NewIcon(p.Icon, widgets.TipIcon, pal.Text)
		g.GetWidget().LayoutData = widget.RowLayoutData{Position: widget.RowLayoutPositionCenter}
		line.AddChild(g)
		// Scaled ONCE here (it mirrors the drawn icon box + row gap);
		// the spacer's MinSize below reads it already in live px.
		iconW = theme.Px(widgets.TipIcon) + theme.Px(theme.IconGap)
	}
	line.AddChild(widget.NewText(
		widget.TextOpts.Text(p.Title, env.FaceBold(theme.StepBase), theme.Solid(pal.Text)),
	))
	w.page.AddChild(line)
	if p.Sub != "" {
		sub := widget.NewContainer(
			widget.ContainerOpts.Layout(widget.NewRowLayout(
				widget.RowLayoutOpts.Direction(widget.DirectionHorizontal),
				widget.RowLayoutOpts.Spacing(theme.Px(theme.IconGap)),
			)),
			widget.ContainerOpts.WidgetOpts(widget.WidgetOpts.LayoutData(widget.RowLayoutData{Stretch: true})),
		)
		if iconW > 0 {
			sub.AddChild(widget.NewText(
				widget.TextOpts.Text("", env.Face(theme.StepSmall), theme.Solid(pal.Dim)),
				widget.TextOpts.WidgetOpts(widget.WidgetOpts.MinSize(iconW, 0)),
			))
		}
		sub.AddChild(widget.NewText(
			widget.TextOpts.Text(p.Sub, env.Face(theme.StepSmall), theme.Solid(pal.Dim)),
		))
		w.page.AddChild(sub)
	}

	// The description prose: wrapped, present only when the def carries
	// one (empty content never renders a placeholder block).
	if p.Prose != "" {
		w.prose = widget.NewText(
			widget.TextOpts.Text(p.Prose, env.Face(theme.StepTiny), theme.Solid(pal.Text)),
			widget.TextOpts.MaxWidth(infoInnerW()),
			widget.TextOpts.WidgetOpts(widget.WidgetOpts.LayoutData(widget.RowLayoutData{Stretch: true})),
		)
		w.page.AddChild(w.prose)
	}

	// One hairline under the header block, then the sections.
	if len(p.Sections) > 0 {
		w.page.AddChild(widget.NewContainer(
			widget.ContainerOpts.BackgroundImage(theme.NineSlice(theme.NRGBA(pal.Accent, widgets.TipRuleA))),
			widget.ContainerOpts.WidgetOpts(widget.WidgetOpts.LayoutData(widget.RowLayoutData{Stretch: true})),
			widget.ContainerOpts.WidgetOpts(widget.WidgetOpts.MinSize(0, theme.Px(widgets.TipRuleH))),
		))
	}
	for _, sec := range p.Sections {
		w.page.AddChild(widget.NewText(
			widget.TextOpts.Text(sec.Label, env.Face(theme.StepSmall), theme.Solid(pal.Head)),
		))
		for _, row := range sec.Rows {
			w.page.AddChild(w.rowWidget(row))
		}
	}
	w.rewrap()
}

// rowWidget builds one label/value line: label left, value right-anchored
// on the row's shared edge, link rows tinted accent and wired for
// right-click (menu) and double-click (swap).
func (w *InfoWin) rowWidget(row infoRow) widget.PreferredSizeLocateableWidget {
	env, pal := w.th, w.th.Pal()
	face := env.Face(theme.StepSmall)
	lineH := widgets.TipLineH(face)

	labelCol, valueCol := theme.Solid(pal.Dim), theme.Solid(pal.Text)
	if row.Tint != (color.RGBA{}) {
		valueCol = theme.Solid(row.Tint)
	}
	if row.Ref != nil {
		if row.RefOnValue {
			valueCol = theme.Solid(pal.Accent)
		} else {
			labelCol = theme.Solid(pal.Accent)
		}
	}

	// Long labels ellipsize against the room the value leaves them (at
	// the default width; a resize refits the prose, not these).
	label := row.Label
	if row.Value != "" && row.Label != "" {
		maxW := float64(theme.Px(infoW-2*theme.ContentsPad-2*theme.ContentsPad-widgets.TipRowGap)) -
			widgets.TipMeasure(row.Value, face)
		label = widgets.Ellipsize(label, maxW, func(s string) float64 { return widgets.TipMeasure(s, face) })
	}

	ref := widgets.InfoRef{}
	if row.Ref != nil {
		ref = *row.Ref
	}
	// The row's declared interactions, through the installer: right-click
	// opens the derived (single-entry) menu, double-click swaps — strictly,
	// only a resolvable Ref does.
	spec := widgets.RowSpec{Ref: &ref}
	handler := func(a *widget.WidgetMouseButtonClickedEventArgs) {
		if row.Ref == nil {
			return
		}
		switch a.Button {
		case ebiten.MouseButtonRight:
			widgets.OpenRowMenu(w.cv, w.as, spec, widgets.ClickPoint(a))
		case ebiten.MouseButtonLeft:
			w.dbl.Click(w.cv, w.as, spec)
		}
	}

	ctr := widget.NewContainer(
		widget.ContainerOpts.Layout(widget.NewAnchorLayout()),
		widget.ContainerOpts.WidgetOpts(
			widget.WidgetOpts.LayoutData(widget.RowLayoutData{Stretch: true}),
			widget.WidgetOpts.MinSize(0, lineH),
			widget.WidgetOpts.MouseButtonClickedHandler(handler),
		),
	)
	ctr.AddChild(widget.NewText(
		widget.TextOpts.Text(label, face, labelCol),
		widget.TextOpts.Position(widget.TextPositionStart, widget.TextPositionCenter),
		widget.TextOpts.WidgetOpts(widget.WidgetOpts.LayoutData(widget.AnchorLayoutData{
			HorizontalPosition: widget.AnchorLayoutPositionStart,
			VerticalPosition:   widget.AnchorLayoutPositionCenter,
		})),
	))
	if row.Value != "" {
		ctr.AddChild(widget.NewText(
			widget.TextOpts.Text(row.Value, face, valueCol),
			widget.TextOpts.Position(widget.TextPositionEnd, widget.TextPositionCenter),
			widget.TextOpts.WidgetOpts(widget.WidgetOpts.LayoutData(widget.AnchorLayoutData{
				HorizontalPosition: widget.AnchorLayoutPositionEnd,
				VerticalPosition:   widget.AnchorLayoutPositionCenter,
			})),
		))
	}
	if row.Ref != nil {
		w.refRows = append(w.refRows, InfoRefRow{Ref: ref, Widget: ctr.GetWidget()})
	}
	return ctr
}

// infoInnerW is the description's wrap width at the default geometry —
// the first frame before layout runs; rewrap keeps up afterwards.
func infoInnerW() float64 {
	return float64(theme.Px(infoW - 2*theme.ContentsPad - 2*theme.ContentsPad))
}
