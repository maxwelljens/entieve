package ui

import (
	"image/color"

	"github.com/entieve/entieve/content"
	"github.com/entieve/entieve/render"
	"github.com/entieve/entieve/sim"
	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/vector"
)

// ---------------------------------------------------------------------------
// Inventory (toggle: I)
// ---------------------------------------------------------------------------

func (u *UI) drawInventory(dst *ebiten.Image, s *sim.Sim, reg Regions) {
	r := u.WindowRect(WinInventory, reg)
	DrawWindow(dst, r, WindowTitle(WinInventory), closableWindows[WinInventory])
	contentY := r.Y + titleBarH
	player := s.Player()

	cargo, ok := s.Cargos.Get(player)
	if !ok {
		Text(dst, "No cargo hold", r.X+pad, contentY+pad, FontSmall, ColDim)
		return
	}
	cargoSec := Rect{X: r.X + pad, Y: contentY + pad, W: r.W - 2*pad, H: invCargoH}
	oreSec := Rect{X: cargoSec.X, Y: cargoSec.Bottom() + pad, W: cargoSec.W, H: invOreH}
	u.drawCargoSection(dst, cargoSec, "Cargo Hold", cargo.Stacks, cargo.Capacity, s)
	u.drawCargoSection(dst, oreSec, "Ore Hold", cargo.Ore, cargo.OreCapacity, s)

	if u.docked() {
		Text(dst, "Docked — Station Services (G) moves items to your hangar.",
			r.X+pad, oreSec.Bottom()+pad, FontTiny, ColAccent)
	}
}

// drawCargoSection draws one labeled stack list with a capacity meter.
func (u *UI) drawCargoSection(dst *ebiten.Image, sec Rect, title string,
	stacks []content.CargoStack, capacity float64, s *sim.Sim) {

	Text(dst, title, sec.X, sec.Y, FontSmall, ColAccent)
	used := 0.0
	for _, st := range stacks {
		if def, ok := s.Content.Item(st.Item); ok {
			used += def.Volume * float64(st.Qty)
		}
	}
	if capacity > 0 {
		TextRight(dst, FormatVolume(used)+" / "+FormatVolume(capacity),
			sec.Right(), sec.Y, FontSmall, ColDim)
		Meter(dst, Rect{X: sec.X, Y: sec.Y + invSecRowH, W: sec.W, H: invMeterH},
			used/capacity, ColInfo)
	}

	maxRows := int((sec.H - invSecHeadH) / invSecRowH)
	if len(stacks) == 0 {
		Text(dst, "empty", sec.X+padSmall, sec.Y+invSecHeadH, FontSmall, ColDim)
		return
	}
	for i, st := range stacks {
		y := sec.Y + invSecHeadH + float64(i)*invSecRowH
		if i >= maxRows {
			Text(dst, "+ more...", sec.X+padSmall, y, FontSmall, ColDim)
			break
		}
		name := string(st.Item)
		vol := 0.0
		if def, ok := s.Content.Item(st.Item); ok {
			name = def.Name
			vol = def.Volume * float64(st.Qty)
		}
		Text(dst, ellipsize(name, sec.W*0.6, FontSmall), sec.X+padSmall, y, FontSmall, ColText)
		TextRight(dst, "×"+intStr(int64(st.Qty))+"  "+FormatVolume(vol),
			NumberAnchor(Rect{X: sec.X, W: sec.W}), y, FontSmall, ColDim)
	}
}

// ---------------------------------------------------------------------------
// Fitting (toggle: F)
// ---------------------------------------------------------------------------

func (u *UI) drawFitting(dst *ebiten.Image, s *sim.Sim, reg Regions) {
	r := u.WindowRect(WinFitting, reg)
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
	DrawWindow(dst, r, title, closableWindows[WinFitting])

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
		Text(dst, labels[row], rr.X, rr.Y+(rr.H-lineH(FontTiny))/2, FontTiny, ColHead)
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
			if cell.Contains(u.mx, u.my) && def != nil {
				border = withAlpha(ColAccent, 180) // hover
			}
			vector.DrawFilledRect(dst, float32(cell.X), float32(cell.Y), float32(cell.W), float32(cell.H),
				bg, false)
			vector.StrokeRect(dst, float32(cell.X)+0.5, float32(cell.Y)+0.5,
				float32(cell.W)-1, float32(cell.H)-1, 1, border, false)
			if def != nil {
				TextCentered(dst, moduleLetter(def.Name), cell.X+cell.W/2,
					cell.Y+(cell.H-lineH(FontLarge))/2, FontLarge, moduleAccent(def.Kind))
				// Unfit button (small visual, see FittingUnfitRect note).
				u.button(dst, FittingUnfitRect(cell), "x", true)
			}
		}
	}

	// Derived stats from the sim's own derived values.
	stats := u.derivedStats(s, ship, hasShip)
	y := FittingSlotRow(r, modRows-1).Bottom() + pad
	rightAnchor := NumberAnchor(Rect{X: r.X, W: r.W - 1})
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
		Text(dst, ln[0], r.X+pad, y, FontSmall, ColText)
		if ln[1] != "" {
			TextRight(dst, ln[1], rightAnchor, y, FontSmall, ColDim)
		}
		y += lineH(FontSmall)
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
func (u *UI) clickFitting(x, y float64, r Rect) {
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
	r := u.WindowRect(WinLog, reg)
	DrawWindow(dst, r, WindowTitle(WinLog), false)

	maxLines := int((r.H - titleBarH - 2*pad) / logLineH)
	if maxLines < 1 {
		return
	}
	start := len(u.log) - maxLines
	if start < 0 {
		start = 0
	}
	timeW := render.Measure("[999:99]", FontSmall) + padSmall
	y := r.Bottom() - pad - float64(len(u.log)-start)*logLineH
	for i := start; i < len(u.log); i++ {
		line := u.log[i]
		Text(dst, "["+FormatTime(line.Time)+"]", r.X+pad, y, FontSmall, ColDim)
		Text(dst, ellipsize(line.Text, r.W-2*pad-timeW, FontSmall),
			r.X+pad+timeW, y, FontSmall, severityColor(line.Sev))
		y += logLineH
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
