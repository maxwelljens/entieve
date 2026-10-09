package ui

import (
	"image/color"

	"github.com/entieve/entieve/content"
	"github.com/entieve/entieve/sim"
	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/vector"
)

// ---------------------------------------------------------------------------
// Inventory (toggle: I)
// ---------------------------------------------------------------------------

func (u *UI) drawInventory(dst *ebiten.Image, s *sim.Sim, reg Regions) {
	r := reg.Inventory
	Panel(dst, r)
	PanelTitle(dst, r, "Inventory")
	player := s.Player()

	cargo, ok := s.Cargos.Get(player)
	if !ok {
		Text(dst, "No cargo hold", r.X+7, r.Y+28, 11, ColDim)
		return
	}
	cargoSec := Rect{r.X + 6, r.Y + 26, r.W - 12, 116}
	oreSec := Rect{r.X + 6, r.Y + 150, r.W - 12, 84}
	u.drawCargoSection(dst, cargoSec, "Cargo Hold", cargo.Stacks, cargo.Capacity, s)
	u.drawCargoSection(dst, oreSec, "Ore Hold", cargo.Ore, cargo.OreCapacity, s)

	if u.docked() {
		Text(dst, "Docked — Station Services (G) moves items to your hangar.",
			r.X+7, r.Bottom()-16, 9, ColAccent)
	}
}

// drawCargoSection draws one labeled stack list with a capacity meter.
func (u *UI) drawCargoSection(dst *ebiten.Image, sec Rect, title string,
	stacks []content.CargoStack, capacity float64, s *sim.Sim) {

	Text(dst, title, sec.X, sec.Y, 11, ColAccent)
	used := 0.0
	for _, st := range stacks {
		if def, ok := s.Content.Item(st.Item); ok {
			used += def.Volume * float64(st.Qty)
		}
	}
	// volume meter + text on the header line
	meterR := Rect{sec.X + sec.W - 130, sec.Y + 1, 130, 9}
	if capacity > 0 {
		Meter(dst, meterR, used/capacity, ColInfo)
		TextRight(dst, FormatVolume(used)+" / "+FormatVolume(capacity),
			sec.Right(), sec.Y+12, 9, ColDim)
	}

	maxRows := int((sec.H - 18) / 15)
	if len(stacks) == 0 {
		Text(dst, "empty", sec.X+4, sec.Y+18, 10, ColDim)
		return
	}
	for i, st := range stacks {
		if i >= maxRows {
			Text(dst, "+ more...", sec.X+4, sec.Y+18+float64(maxRows)*15, 10, ColDim)
			break
		}
		y := sec.Y + 18 + float64(i)*15
		name := string(st.Item)
		if def, ok := s.Content.Item(st.Item); ok {
			name = def.Name
		}
		Text(dst, name, sec.X+4, y, 10, ColText)
		vol := 0.0
		if def, ok := s.Content.Item(st.Item); ok {
			vol = def.Volume * float64(st.Qty)
		}
		TextRight(dst, "×"+intStr(int64(st.Qty))+"  "+FormatVolume(vol),
			sec.Right()-2, y, 10, ColDim)
	}
}

// ---------------------------------------------------------------------------
// Fitting (toggle: F)
// ---------------------------------------------------------------------------

func (u *UI) drawFitting(dst *ebiten.Image, s *sim.Sim, reg Regions) {
	r := reg.Fitting
	Panel(dst, r)
	player := s.Player()
	ship, hasShip := s.Ships.Get(player)
	hullName, hullClass := "No ship", ""
	if hasShip {
		if def, ok := u.c.Ship(ship.Hull); ok {
			hullName, hullClass = def.Name, def.Class
		}
	}
	title := "Fitting — " + hullName
	if hullClass != "" {
		title += " (" + hullClass + ")"
	}
	PanelTitle(dst, r, title)

	fit, hasFit := s.Fittings.Get(player)
	slots := content.Slots{}
	if hasShip {
		if def, ok := u.c.Ship(ship.Hull); ok {
			slots = def.Slots
		}
	}
	classes := [modRows]string{content.SlotHigh, content.SlotMid, content.SlotLow}
	labels := [modRows]string{"High", "Mid", "Low"}
	slotCounts := [modRows]int{slots.High, slots.Mid, slots.Low}
	maxCells := FittingMaxCells(r)

	for row := 0; row < modRows; row++ {
		rr := FittingSlotRow(r, row)
		Text(dst, labels[row], rr.X, rr.Y+8, 10, ColHead)
		if !hasFit {
			continue
		}
		class := classes[row]
		defs := fit.Defs(class)
		count := slotCounts[row]
		if len(defs) > count {
			count = len(defs)
		}
		if count > maxCells {
			count = maxCells
		}
		for i := 0; i < count; i++ {
			cell := FittingCellRect(rr, i)
			var def *content.ModuleDef
			if i < len(defs) {
				def, _ = u.c.Module(defs[i])
			}
			bg, border := ColCell, ColBorder
			if def == nil {
				border = withAlpha(ColBorder, 110)
			}
			vector.DrawFilledRect(dst, float32(cell.X), float32(cell.Y), float32(cell.W), float32(cell.H),
				bg, false)
			vector.StrokeRect(dst, float32(cell.X)+0.5, float32(cell.Y)+0.5,
				float32(cell.W)-1, float32(cell.H)-1, 1, border, false)
			if def != nil {
				TextCentered(dst, moduleLetter(def.Name), cell.X+cell.W/2, cell.Y+7, 13, moduleAccent(def.Kind))
				// Unfit button.
				ur := FittingUnfitRect(cell)
				Button(dst, ur, "x", true)
			}
		}
	}

	// Derived stats from the sim's own derived values.
	stats := u.derivedStats(s, ship, hasShip)
	y := FittingSlotRow(r, modRows-1).Bottom() + 8
	lines := [][2]string{
		{"Shield " + intStr(int64(stats.shield)) +
			"   Armor " + intStr(int64(stats.armor)) +
			"   Structure " + intStr(int64(stats.structure)), ""},
		{"Resists", "EM " + pctStr(stats.resists.EM) + "  TH " + pctStr(stats.resists.Thermal) +
			"  KN " + pctStr(stats.resists.Kinetic) + "  EX " + pctStr(stats.resists.Explosive)},
		{"Capacitor", fmtGJ(stats.cap) + " GJ  recharge " + groupFloat(stats.tau, 1) + " s"},
		{"Speed", FormatSpeed(stats.speed) + "   Align " + groupFloat(stats.align, 1) +
			" s   Sig " + groupFloat(stats.sig, 0) + " m"},
		{"Cargo", FormatVolume(stats.cargo) + "   Ore hold " + FormatVolume(stats.oreHold)},
		{"Warp", groupFloat(stats.warp, 1) + " AU/s   Targets " + intStr(int64(stats.maxTargets)) +
			"   Scan " + groupFloat(stats.scanRes, 0)},
	}
	for _, ln := range lines {
		Text(dst, ln[0], r.X+7, y, 10, ColText)
		if ln[1] != "" {
			TextRight(dst, ln[1], r.Right()-7, y, 10, ColDim)
		}
		y += 14
	}
}

// fitStats are the fitting window's derived ship statistics.
type fitStats struct {
	shield, armor, structure float64
	resists                  content.Resists
	cap, tau                 float64
	speed, align, sig        float64
	cargo, oreHold           float64
	warp                     float64
	maxTargets, scanRes      float64
}

// derivedStats reads the sim's derived values (already fit-computed) for the
// fitting window; missing stores fall back to zeros.
func (u *UI) derivedStats(s *sim.Sim, ship *sim.ShipState, hasShip bool) fitStats {
	var st fitStats
	if !hasShip {
		return st
	}
	st.shield, st.armor, st.structure = ship.MaxShield, ship.MaxArmor, ship.MaxStructure
	st.resists = ship.Resists
	if def, ok := u.c.Ship(ship.Hull); ok {
		st.warp = def.WarpSpeed
	}
	player := s.Player()
	if cap, ok := s.Caps.Get(player); ok {
		st.cap, st.tau = cap.Capacity, cap.Tau
	}
	if m, ok := s.Motions.Get(player); ok {
		st.speed, st.align, st.sig = m.MaxSpeed, m.Agility*10, m.SigRadius
	}
	if cg, ok := s.Cargos.Get(player); ok {
		st.cargo, st.oreHold = cg.Capacity, cg.OreCapacity
	}
	if tg, ok := s.Targetings.Get(player); ok {
		st.maxTargets, st.scanRes = float64(tg.MaxTargets), tg.ScanRes
	}
	return st
}

// pctStr formats a resist fraction as "45%".
func pctStr(f float64) string { return FormatPct(f) }

// clickFitting handles unfit clicks in the fitting window.
func (u *UI) clickFitting(x, y float64, reg Regions) {
	r := reg.Fitting
	fit, ok := u.s.Fittings.Get(u.s.Player())
	if !ok {
		return
	}
	classes := [modRows]string{content.SlotHigh, content.SlotMid, content.SlotLow}
	for row := 0; row < modRows; row++ {
		rr := FittingSlotRow(r, row)
		if y < rr.Y || y >= rr.Bottom() {
			continue
		}
		defs := fit.Defs(classes[row])
		for i := range defs {
			cell := FittingCellRect(rr, i)
			if FittingUnfitRect(cell).Contains(x, y) {
				u.enqueue(sim.CmdUnfitModule{Slot: sim.SlotRef{Class: classes[row], Index: i}})
				return
			}
		}
	}
}

// ---------------------------------------------------------------------------
// Combat / action log (bottom-left)
// ---------------------------------------------------------------------------

func (u *UI) drawLog(dst *ebiten.Image, reg Regions) {
	r := reg.Log
	Panel(dst, r)
	PanelTitle(dst, r, "Log")

	const lineH = 14.0
	maxLines := int((r.H - 26) / lineH)
	if maxLines < 1 {
		return
	}
	start := len(u.log) - maxLines
	if start < 0 {
		start = 0
	}
	y := r.Y + r.H - 6 - float64(len(u.log)-start)*lineH
	for i := start; i < len(u.log); i++ {
		line := u.log[i]
		Text(dst, "["+FormatTime(line.Time)+"]", r.X+5, y, 10, ColDim)
		Text(dst, ellipsize(line.Text, r.W-46, 10), r.X+42, y, 10, severityColor(line.Sev))
		y += lineH
	}
}

// severityColor maps sim severities to UI colors.
func severityColor(sev sim.Severity) color.RGBA {
	switch sev {
	case sim.SevGood:
		return ColGood
	case sim.SevWarning:
		return ColWarn
	case sim.SevDanger:
		return ColDanger
	default:
		return ColInfo
	}
}
