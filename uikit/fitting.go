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
	env Env
	win *widget.Window

	title   *widget.Text // title bar: hull name and class
	stats   *widget.Text // derived stat lines
	resists *Table       // EM/TH/KN/EX in one row
	slots   *Table
	hangar  *Table

	unfit    *widget.Button
	fitMod   *widget.Button
	assemble *widget.Button

	selSlot slotPick
	hasSlot bool
	selMod  content.ModuleID
	hasMod  bool
	selShip content.ShipID
	hasShip bool
}

// NewFitting builds the Fitting window over the shared environment.
// Mount it on a Kit with Kit.AddWindow and call Sync every frame.
func NewFitting(env Env) *Fitting {
	f := &Fitting{env: env}
	p := env.Pal

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
	f.hangar = NewTable(env, []Column{
		{Key: "Item", Title: "Hangar", Priority: 1},
		{Key: "Qty", Title: "Qty", Priority: 0, AlignRight: true},
	})
	f.hangar.OnSelect(func(r TableRow) {
		switch id := r.Data.(type) {
		case content.ModuleID:
			f.selMod, f.hasMod = id, true
		case content.ShipID:
			f.selShip, f.hasShip = id, true
		}
	})

	f.stats = widget.NewText(
		widget.TextOpts.Text("", env.Face(StepTiny), solid(p.Text)),
	)

	newBtn := func(label string, clicked func()) *widget.Button {
		return widget.NewButton(
			widget.ButtonOpts.TextLabel(label),
			widget.ButtonOpts.WidgetOpts(
				widget.WidgetOpts.MinSize(0, 40),
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
			widget.RowLayoutOpts.Spacing(contentsPad),
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
			widget.GridLayoutOpts.Spacing(0, 4),
			widget.GridLayoutOpts.Padding(widget.NewInsetsSimple(contentsPad)),
		)),
	)
	body.AddChild(
		f.stats,
		f.resists.Container(),
		label("Slots"), f.slots.Container(),
		label("Hangar"), f.hangar.Container(),
		buttons,
	)

	title := widget.NewContainer(
		widget.ContainerOpts.BackgroundImage(bordered(solid(p.BGHard), nrgba(p.Border, 200), 1)),
		widget.ContainerOpts.Layout(widget.NewAnchorLayout()),
	)
	f.title = widget.NewText(
		widget.TextOpts.Text("Fitting", env.FaceBold(StepBase), solid(p.Accent)),
		widget.TextOpts.WidgetOpts(widget.WidgetOpts.LayoutData(widget.AnchorLayoutData{
			HorizontalPosition: widget.AnchorLayoutPositionStart,
			VerticalPosition:   widget.AnchorLayoutPositionCenter,
			Padding:            widget.NewInsetsSimple(contentsPad),
		})),
	)
	title.AddChild(f.title)

	f.win = widget.NewWindow(
		widget.WindowOpts.Contents(body),
		widget.WindowOpts.TitleBar(title, titleBarH),
		widget.WindowOpts.Draggable(),
		// Content floor: stat block, resist row and a full row of the
		// slot and hangar tables (they scroll below that).
		widget.WindowOpts.MinSize(300, 430),
		// Fallback only: the composed rect at the 1024×700 minimum.
		// The game recomposes from the real viewport at startup
		// (game/station_layout.go).
		widget.WindowOpts.Location(image.Rect(709, 256, 1014, 690)),
	)
	return f
}

// Win returns the fitting window for Kit.AddWindow.
func (f *Fitting) Win() *widget.Window { return f.win }

// Sync refreshes the hull title, derived stats, resist row, slot rows,
// hangar rows and the action gating. Call it every frame before
// Kit.Update.
func (f *Fitting) Sync() {
	s := f.env.Sim
	player := s.Player()

	ship, hasShip := s.Ships.Get(player)
	hullName, hullClass := "No ship", ""
	if hasShip {
		if def, ok := f.env.Content.Ship(ship.Hull); ok {
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
	// module plus an empty slot of its class, assemble needs a ship.
	f.unfit.GetWidget().Disabled = !f.hasSlot || f.selSlot.mod == ""
	modFits := f.hasMod && f.hasSlot && f.selSlot.mod == ""
	if modFits {
		if def, ok := f.env.Content.Module(f.selMod); !ok || def.Slot != f.selSlot.ref.Class {
			modFits = false
		}
	}
	f.fitMod.GetWidget().Disabled = !modFits
	f.assemble.GetWidget().Disabled = !f.hasShip
}

// statLines renders the derived ship statistics (mirrors
// ui.derivedStats, which reads the sim's own fit-computed values).
func (f *Fitting) statLines(ship *sim.ShipState, hasShip bool) string {
	if !hasShip {
		return "No ship"
	}
	s := f.env.Sim
	player := s.Player()
	lines := []string{
		"Shield " + FormatInt(int64(ship.MaxShield)) +
			" · Armor " + FormatInt(int64(ship.MaxArmor)) +
			" · Structure " + FormatInt(int64(ship.MaxStructure)),
	}
	if cap, ok := s.Caps.Get(player); ok {
		lines = append(lines, "Capacitor "+groupFloat(cap.Capacity, 1)+
			" GJ · recharge "+groupFloat(cap.Tau, 1)+" s")
	}
	if m, ok := s.Motions.Get(player); ok {
		lines = append(lines, "Speed "+formatSpeed(m.MaxSpeed)+
			" · Align "+groupFloat(m.Agility*10, 1)+" s"+
			" · Sig "+groupFloat(m.SigRadius, 0)+" m")
	}
	if cg, ok := s.Cargos.Get(player); ok {
		lines = append(lines, "Cargo "+formatVolume(cg.Capacity)+
			" · Ore hold "+formatVolume(cg.OreCapacity))
	}
	warp := 0.0
	if def, ok := f.env.Content.Ship(ship.Hull); ok {
		warp = def.WarpSpeed
	}
	if tg, ok := s.Targetings.Get(player); ok {
		lines = append(lines, "Warp "+groupFloat(warp, 1)+" AU/s"+
			" · Targets "+FormatInt(int64(tg.MaxTargets))+
			" · Scan "+groupFloat(tg.ScanRes, 0))
	}
	return joinLines(lines)
}

// slotRows renders one row per hull slot: filled slots carry the module
// name, empty ones read "(empty)".
func (f *Fitting) slotRows(ship *sim.ShipState, hasShip bool) []TableRow {
	if !hasShip {
		return nil
	}
	hull, ok := f.env.Content.Ship(ship.Hull)
	if !ok {
		return nil
	}
	fit, hasFit := f.env.Sim.Fittings.Get(f.env.Sim.Player())
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
			if i < len(defs) {
				pick.mod = defs[i]
				name = itemLabel(f.env, content.ItemID(defs[i]))
			}
			rows = append(rows, TableRow{
				Cells: []string{labels[class] + " " + FormatInt(int64(i+1)), name},
				Data:  pick,
			})
		}
	}
	return rows
}

// hangarRows renders the hangar's fit-able modules and stored ships
// (other hangar items live in the Inventory window).
func (f *Fitting) hangarRows() []TableRow {
	if f.env.Sim.Hangar == nil {
		return nil
	}
	var rows []TableRow
	for _, st := range f.env.Sim.Hangar.Items {
		mod, ok := f.env.Content.Module(content.ModuleID(st.Item))
		if !ok || st.Qty <= 0 {
			continue
		}
		rows = append(rows, TableRow{
			Cells: []string{mod.Name, "×" + FormatInt(int64(st.Qty))},
			Data:  content.ModuleID(st.Item),
		})
	}
	for _, ss := range f.env.Sim.Hangar.Ships {
		name := string(ss.Hull)
		if def, ok := f.env.Content.Ship(ss.Hull); ok {
			name = def.Name
		}
		rows = append(rows, TableRow{
			Cells: []string{name, "×1"},
			Data:  ss.Hull,
		})
	}
	return rows
}

// unfitClicked enqueues CmdUnfitModule for the selected fitted slot.
func (f *Fitting) unfitClicked() {
	if !f.hasSlot || f.selSlot.mod == "" {
		return
	}
	f.env.Enqueue(sim.CmdUnfitModule{Slot: f.selSlot.ref})
}

// fitClicked enqueues CmdFitModule: the selected hangar module into the
// selected empty slot of its class.
func (f *Fitting) fitClicked() {
	if !f.hasMod || !f.hasSlot || f.selSlot.mod != "" {
		return
	}
	def, ok := f.env.Content.Module(f.selMod)
	if !ok || def.Slot != f.selSlot.ref.Class {
		return
	}
	f.env.Enqueue(sim.CmdFitModule{Item: def.ID, Slot: f.selSlot.ref})
}

// assembleClicked enqueues CmdAssemble for the selected hangar ship.
func (f *Fitting) assembleClicked() {
	if !f.hasShip {
		return
	}
	f.env.Enqueue(sim.CmdAssemble{Ship: f.selShip})
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
