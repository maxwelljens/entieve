package uikit

import (
	"image"

	"github.com/ebitenui/ebitenui/widget"
	"github.com/entieve/entieve/content"
	"github.com/entieve/entieve/sim"
)

// slotPick is the fitting slot table's row payload: the slot reference
// plus the fitted module ID ("" = the slot is empty).
type slotPick struct {
	ref sim.SlotRef
	mod content.ModuleID
}

// Fitting is the ship fitting window: hull identity and derived stats,
// a four-type resist table, the high/mid/low slot rows, the hangar's
// modules and ships, and the Unfit/Fit/Assemble actions.
type Fitting struct {
	th  Theme
	cv  ContentView
	as  ActionSink
	win *widget.Window

	title   *widget.Text // title bar: hull name and class
	stats   *widget.Text // derived stat lines
	resists *Table       // EM/TH/KN/EX in one row
	slots   *Table
	hangar  *Table

	unfit    *widget.Button
	fitMod   *widget.Button
	assemble *widget.Button

	// Row interactions (menus, tooltips, double-click) all derive from
	// the two tables' rowspecs — see slotRows/hangarRows; the tooltip
	// cache lives in the Table now (one installer, one rowTip slot).

	selSlot slotPick
	hasSlot bool
	selMod  content.ModuleID
	hasMod  bool
	selShip content.ShipID
	hasShip bool
}

// NewFitting builds the Fitting window over the shared environment.
// Mount it on a Kit with Kit.AddWindow and call Sync every frame.
func NewFitting(env Seams) *Fitting {
	f := &Fitting{th: env, cv: env, as: env}
	p := env.Pal()

	f.resists = NewTable(env, []Column{
		{Key: "EM", Title: "EM", Priority: 0, AlignRight: true},
		{Key: "TH", Title: "TH", Priority: 1, AlignRight: true},
		{Key: "KN", Title: "KN", Priority: 2, AlignRight: true},
		{Key: "EX", Title: "EX", Priority: 3, AlignRight: true},
	})
	f.slots = NewTable(env, []Column{
		{Key: "Slot", Title: "Slot", Priority: 1},
		{Key: "Module", Title: "Module", Priority: 0},
	})
	f.slots.OnSelect(func(r TableRow) {
		if sp, ok := r.Data.(slotPick); ok {
			f.selSlot, f.hasSlot = sp, true
		}
	})
	// Double-click (Show Info) and the right-click menu (Show Info +
	// Fit/Unefit, gated on the clicked row — the menu's actions carry its
	// payload, so no click-time selection dance) come from the rows'
	// RowSpec, built in slotRows/hangarRows.

	f.hangar = NewTable(env, []Column{
		{Key: "Item", Title: "Hangar", Priority: 1},
		{Key: "Qty", Title: "Qty", Priority: 0, AlignRight: true},
	})
	f.hangar.OnSelect(f.selectHangar)

	f.stats = widget.NewText(
		widget.TextOpts.Text("", env.Face(StepTiny), solid(p.Text)),
	)

	newBtn := func(label string, clicked func()) *widget.Button {
		return widget.NewButton(
			widget.ButtonOpts.TextLabel(label),
			widget.ButtonOpts.WidgetOpts(
				widget.WidgetOpts.MinSize(0, Px(40)),
				widget.WidgetOpts.LayoutData(widget.RowLayoutData{Stretch: true}),
			),
			widget.ButtonOpts.ClickedHandler(func(*widget.ButtonClickedEventArgs) { clicked() }),
		)
	}
	f.unfit = newBtn("Unfit slot", f.unfitClicked)
	f.fitMod = newBtn("Fit module", f.fitClicked)
	f.assemble = newBtn("Assemble", f.assembleClicked)
	buttons := widget.NewContainer(
		widget.ContainerOpts.Layout(widget.NewRowLayout(
			widget.RowLayoutOpts.Direction(widget.DirectionHorizontal),
			widget.RowLayoutOpts.Spacing(Px(contentsPad)),
		)),
	)
	buttons.AddChild(f.unfit, f.fitMod, f.assemble)

	label := func(s string) *widget.Text {
		return widget.NewText(
			widget.TextOpts.Text(s, env.Face(StepTiny), solid(p.Accent)),
		)
	}

	// Body: fixed rows for the stat block, resist table and section
	// labels; the two data tables split the remaining height.
	body := widget.NewContainer(
		widget.ContainerOpts.BackgroundImage(bordered(solid(p.BG), nrgba(p.Border, 200), 1)),
		widget.ContainerOpts.Layout(widget.NewGridLayout(
			widget.GridLayoutOpts.Columns(1),
			widget.GridLayoutOpts.Stretch([]bool{true}, []bool{false, false, false, true, false, true, false}),
			widget.GridLayoutOpts.Spacing(0, Px(4)),
			widget.GridLayoutOpts.Padding(widget.NewInsetsSimple(Px(contentsPad))),
		)),
	)
	body.AddChild(
		f.stats,
		f.resists.Container(),
		label("Slots"), f.slots.Container(),
		label("Hangar"), f.hangar.Container(),
		buttons,
	)

	title, fitTitle := NewTitleBar(env, "Fitting", "svc_fitting")
	f.title = fitTitle

	f.win = widget.NewWindow(
		widget.WindowOpts.Contents(body),
		widget.WindowOpts.TitleBar(title, Px(titleBarH)),
		widget.WindowOpts.Draggable(),
		// Content floor: stat block, resist row and a full row of the
		// slot and hangar tables (they scroll below that).
		widget.WindowOpts.MinSize(Px(300), Px(430)),
		// Fallback only: the composed rect at the 1024×700 minimum.
		// The game recomposes from the real viewport at startup
		// (game/station_layout.go).
		widget.WindowOpts.Location(image.Rect(Px(709), Px(256), Px(1014), Px(690))),
	)
	return f
}

// Win returns the fitting window for Kit.AddWindow.
func (f *Fitting) Win() *widget.Window { return f.win }

// Sync refreshes the hull title, derived stats, resist row, slot rows,
// hangar rows and the action gating. Call it every frame before
// Kit.Update.
func (f *Fitting) Sync() {
	s := f.cv
	player := s.Player()

	ship, hasShip := s.ShipState(player)
	hullName, hullClass := "No ship", ""
	if hasShip {
		if def, ok := f.cv.Defs().Ship(ship.Hull); ok {
			hullName, hullClass = def.Name, def.Class
		}
	}
	f.title.Label = "Fitting — " + hullName
	if hullClass != "" {
		f.title.Label += " (" + hullClass + ")"
	}
	f.stats.Label = f.statLines(ship, hasShip)

	if hasShip {
		f.resists.SetRows([]TableRow{{
			Cells: []string{
				formatPct(ship.Resists.EM),
				formatPct(ship.Resists.Thermal),
				formatPct(ship.Resists.Kinetic),
				formatPct(ship.Resists.Explosive),
			},
			Data: "resists",
		}})
	} else {
		f.resists.SetRows(nil)
	}
	f.slots.SetRows(f.slotRows(ship, hasShip))
	f.hangar.SetRows(f.hangarRows())
	f.resists.Sync()
	f.slots.Sync()
	f.hangar.Sync()

	// Action gating: unfit needs a fitted slot, fit needs a hangar
	// module plus an empty slot of its class, assemble needs a ship —
	// the same fitReady/unequipReady rules the row menus read.
	f.unfit.GetWidget().Disabled = !f.hasSlot || !unequipReady(f.selSlot)
	f.fitMod.GetWidget().Disabled = !f.hasMod || !f.fitReady(f.selMod, f.selSlot)
	f.assemble.GetWidget().Disabled = !f.hasShip
}

// fitReady reports whether mod fits into slot: a real module into a real
// empty slot of its class. The toolbar's Fit button and both row menus
// (slot rows, hangar rows) gate through this one rule, so the seam keeps
// them from drifting apart.
func (f *Fitting) fitReady(mod content.ModuleID, slot slotPick) bool {
	if mod == "" || slot.mod != "" {
		return false
	}
	def, ok := f.cv.Defs().Module(mod)
	return ok && def.Slot == slot.ref.Class
}

// unequipReady reports whether a slot row can be unfit (it holds a
// module) — the toolbar and the slot menu's Unefit entry share it.
func unequipReady(slot slotPick) bool { return slot.mod != "" }

// statLines renders the derived ship statistics (mirrors
// ui.derivedStats, which reads the sim's own fit-computed values).
func (f *Fitting) statLines(ship sim.ShipState, hasShip bool) string {
	if !hasShip {
		return "No ship"
	}
	s := f.cv
	player := s.Player()
	lines := []string{
		"Shield " + FormatInt(int64(ship.MaxShield)) +
			" · Armor " + FormatInt(int64(ship.MaxArmor)) +
			" · Structure " + FormatInt(int64(ship.MaxStructure)),
	}
	if cap, ok := s.Capacitor(player); ok {
		lines = append(lines, "Capacitor "+groupFloat(cap.Capacity, 1)+
			" GJ · recharge "+groupFloat(cap.Tau, 1)+" s")
	}
	if m, ok := s.Motion(player); ok {
		lines = append(lines, "Speed "+formatSpeed(m.MaxSpeed)+
			" · Align "+groupFloat(m.Agility*10, 1)+" s"+
			" · Sig "+groupFloat(m.SigRadius, 0)+" m")
	}
	if cg, ok := s.Cargo(player); ok {
		lines = append(lines, "Cargo "+formatVolume(cg.Capacity)+
			" · Ore hold "+formatVolume(cg.OreCapacity))
	}
	warp := 0.0
	if def, ok := f.cv.Defs().Ship(ship.Hull); ok {
		warp = def.WarpSpeed
	}
	if tg, ok := s.Targeting(player); ok {
		lines = append(lines, "Warp "+groupFloat(warp, 1)+" AU/s"+
			" · Targets "+FormatInt(int64(tg.MaxTargets))+
			" · Scan "+groupFloat(tg.ScanRes, 0))
	}
	return joinLines(lines)
}

// slotRows renders one row per hull slot: filled slots carry the module
// name, empty ones read "(empty)".
func (f *Fitting) slotRows(ship sim.ShipState, hasShip bool) []TableRow {
	if !hasShip {
		return nil
	}
	hull, ok := f.cv.Defs().Ship(ship.Hull)
	if !ok {
		return nil
	}
	fit, hasFit := f.cv.Fitting(f.cv.Player())
	classes := []string{content.SlotHigh, content.SlotMid, content.SlotLow}
	labels := map[string]string{content.SlotHigh: "High", content.SlotMid: "Mid", content.SlotLow: "Low"}
	counts := map[string]int{
		content.SlotHigh: hull.Slots.High,
		content.SlotMid:  hull.Slots.Mid,
		content.SlotLow:  hull.Slots.Low,
	}
	var rows []TableRow
	for _, class := range classes {
		var defs []content.ModuleID
		if hasFit {
			defs = fit.Defs(class)
		}
		n := counts[class]
		if len(defs) > n {
			n = len(defs)
		}
		for i := 0; i < n; i++ {
			pick := slotPick{ref: sim.SlotRef{Class: class, Index: i}}
			name := "(empty)"
			icon := ""
			if i < len(defs) {
				pick.mod = defs[i]
				name = itemLabel(f.cv, content.ItemID(defs[i]))
				if def, ok := f.cv.Defs().Module(defs[i]); ok {
					icon = ModuleIcon(def.Kind)
				}
			}
			rows = append(rows, TableRow{
				Cells: []string{labels[class] + " " + FormatInt(int64(i+1)), name},
				Icons: []string{"", icon},
				Data:  pick,
				Spec:  f.slotSpec(pick),
			})
		}
	}
	return rows
}

// slotSpec is one slot row's declared interactions: Show Info on the
// fitted module (declared even when empty — the entry shows grayed),
// Fit/Unefit gated on THIS row (the menu acts on the row it was opened
// on; the toolbar keeps acting on the selection) and the shared module
// tooltip with the slot in its sub line. The panel is built lazily by
// the installer — only when the row's tooltip first attaches or its key
// moves.
func (f *Fitting) slotSpec(pick slotPick) RowSpec {
	spec := RowSpec{
		Ref: rowRef(f.cv, pick),
		Actions: []RowAction{
			{Label: "Fit module", Disabled: !f.hasMod || !f.fitReady(f.selMod, pick),
				Intent: Fit{Module: f.selMod, Slot: pick.ref}},
			{Label: "Unefit slot", Disabled: !unequipReady(pick),
				Intent: Unequip{Slot: pick.ref}},
		},
	}
	if pick.mod == "" {
		return spec // empty slot: no tooltip (and a grayed Fit/Unefit pair)
	}
	def, ok := f.cv.Defs().Module(pick.mod)
	if !ok {
		return spec
	}
	slot := slotLabel(pick.ref)
	spec.Tip = TipData{Key: string(pick.mod) + "|" + slot, Build: func() TooltipModel {
		return moduleTip(def, slot, "", 0)
	}}
	return spec
}

// hangarRows renders the hangar's fit-able modules and stored ships
// (other hangar items live in the Inventory window).
func (f *Fitting) hangarRows() []TableRow {
	var rows []TableRow
	for _, st := range f.cv.HangarItems() {
		mod, ok := f.cv.Defs().Module(content.ModuleID(st.Item))
		if !ok || st.Qty <= 0 {
			continue
		}
		id := content.ModuleID(st.Item)
		rows = append(rows, TableRow{
			Cells: []string{mod.Name, "×" + FormatInt(int64(st.Qty))},
			Icons: []string{ModuleIcon(mod.Kind), ""},
			Data:  id,
			Spec:  f.hangarSpec(id),
		})
	}
	for _, ss := range f.cv.HangarShips() {
		name := string(ss.Hull)
		if def, ok := f.cv.Defs().Ship(ss.Hull); ok {
			name = def.Name
		}
		rows = append(rows, TableRow{
			Cells: []string{name, "×1"},
			Icons: []string{"row_hull", ""},
			Data:  ss.Hull,
			Spec:  f.hangarSpec(ss.Hull),
		})
	}
	return rows
}

// hangarSpec is one hangar row's declared interactions: Show Info plus
// the Fit action into the SELECTED slot — gated by this row's own data
// (a stored hull is not a module, so its Fit stays grayed instead of
// arming from a stale module selection) and the slotless module tooltip
// (stored hulls carry none: they aren't fitted yet).
func (f *Fitting) hangarSpec(data any) RowSpec {
	spec := RowSpec{
		Ref: rowRef(f.cv, data),
	}
	mod, isMod := data.(content.ModuleID)
	spec.Actions = []RowAction{
		{Label: "Fit module", Disabled: !f.fitReady(mod, f.selSlot),
			Intent: Fit{Module: mod, Slot: f.selSlot.ref}},
	}
	def, ok := f.cv.Defs().Module(mod)
	if !isMod || !ok {
		return spec
	}
	spec.Tip = TipData{Key: string(mod), Build: func() TooltipModel {
		return moduleTip(def, "", "", 0)
	}}
	return spec
}

// selectHangar records the hangar row's payload as the pending fit or
// assemble target (the selection callback and the row menu share it).
func (f *Fitting) selectHangar(r TableRow) {
	switch id := r.Data.(type) {
	case content.ModuleID:
		f.selMod, f.hasMod = id, true
	case content.ShipID:
		f.selShip, f.hasShip = id, true
	}
}

// fitInto emits Fit for one slot row: the selected hangar module into
// the right-clicked slot (the toolbar's fitClicked targets the selected
// slot instead).
func (f *Fitting) fitInto(pick slotPick) {
	if !f.hasMod || !f.fitReady(f.selMod, pick) {
		return
	}
	f.as.Do(Fit{Module: f.selMod, Slot: pick.ref})
}

// fitFrom emits Fit for one hangar row's module into the selected slot.
func (f *Fitting) fitFrom(mod content.ModuleID) {
	if !f.fitReady(mod, f.selSlot) {
		return
	}
	f.as.Do(Fit{Module: mod, Slot: f.selSlot.ref})
}

// unfitClicked emits Unequip for the selected fitted slot.
func (f *Fitting) unfitClicked() { f.as.Do(Unequip{Slot: f.selSlot.ref}) }

// fitClicked emits Fit: the selected hangar module into the selected
// empty slot of its class.
func (f *Fitting) fitClicked() { f.fitFrom(f.selMod) }

// assembleClicked emits Assemble for the selected hangar ship.
func (f *Fitting) assembleClicked() {
	if !f.hasShip {
		return
	}
	f.as.Do(Assemble{Ship: f.selShip})
}

// joinLines concatenates stat lines with newlines (widget.Text renders
// them as separate lines).
func joinLines(lines []string) string {
	out := ""
	for i, l := range lines {
		if i > 0 {
			out += "\n"
		}
		out += l
	}
	return out
}
