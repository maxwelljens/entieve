package ui

import (
	"image/color"
	"math"

	"github.com/entieve/entieve/content"
	"github.com/entieve/entieve/render"
	"github.com/entieve/entieve/sim"
	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/vector"
)

// ---------------------------------------------------------------------------
// Overview (top-right)
// ---------------------------------------------------------------------------

func (u *UI) drawOverview(dst *ebiten.Image, s *sim.Sim, reg Regions) {
	r := reg.Overview
	Panel(dst, r)

	// Header strip with sort state.
	vector.DrawFilledRect(dst, float32(r.X+1), float32(r.Y+2), float32(r.W-2), float32(ovHeaderH),
		mustHex("#131926"), false)
	headers := [4]string{"", "Name", "Distance", "Velocity"}
	for c := 0; c < 4; c++ {
		col := OverviewColumn(r, c)
		label := headers[c]
		if SortColumn(c) == u.sortCol && label != "" {
			if u.sortAsc {
				label += " ^"
			} else {
				label += " v"
			}
		}
		x := col.X + 3
		if c == 2 || c == 3 { // right-align numeric columns
			TextRight(dst, label, col.Right()-4, r.Y+6, 10, ColHead)
		} else {
			Text(dst, label, x, r.Y+6, 10, ColHead)
		}
	}

	rows := u.overviewRows()
	max := OverviewMaxRows(r)
	for i := 0; i < max && i < len(rows); i++ {
		inf := rows[i]
		row := OverviewRowRect(r, i)
		if i%2 == 1 {
			vector.DrawFilledRect(dst, float32(row.X), float32(row.Y), float32(row.W), float32(row.H),
				withAlpha(ColBGHard, 90), false)
		}
		if inf.Entity == u.Selected {
			vector.DrawFilledRect(dst, float32(row.X), float32(row.Y), float32(row.W), float32(row.H),
				withAlpha(ColAccent, 36), false)
		}
		drawKindIcon(dst, inf.Kind, inf.Entity == u.s.Player(), row.X+4, row.Y+row.H/2)
		nameCol := render.BracketColor(inf.Kind, inf.Hostile)
		if inf.Entity == u.s.Player() {
			nameCol = ColPlayer
		}
		Text(dst, inf.Name, row.X+ovIconW+2, row.Y+4, 11, nameCol)
		TextRight(dst, FormatDistance(inf.Distance), OverviewColumn(r, 2).Right()-4, row.Y+4, 11, ColText)
		TextRight(dst, FormatSpeed(inf.Vel.Len()), OverviewColumn(r, 3).Right()-4, row.Y+4, 11, ColDim)
	}
}

// drawKindIcon draws the overview's tiny per-kind icon.
func drawKindIcon(dst *ebiten.Image, kind string, isPlayer bool, x, cy float64) {
	col := render.BracketColor(kind, false)
	if isPlayer {
		col = ColPlayer
	}
	w := float32(1)
	switch kind {
	case sim.KindShip, sim.KindPlayer:
		vector.StrokeLine(dst, float32(x+1), float32(cy-4), float32(x+9), float32(cy), w, col, true)
		vector.StrokeLine(dst, float32(x+9), float32(cy), float32(x+1), float32(cy+4), w, col, true)
		vector.StrokeLine(dst, float32(x+1), float32(cy+4), float32(x+1), float32(cy-4), w, col, true)
	case sim.KindStation:
		vector.StrokeRect(dst, float32(x+2), float32(cy-4), 7, 8, w, col, true)
	case sim.KindGate:
		vector.StrokeCircle(dst, float32(x+5), float32(cy), 4, w, col, true)
	case sim.KindWreck:
		vector.StrokeLine(dst, float32(x+2), float32(cy-3), float32(x+8), float32(cy+3), w, col, true)
		vector.StrokeLine(dst, float32(x+8), float32(cy-3), float32(x+2), float32(cy+3), w, col, true)
	case sim.KindAsteroid:
		vector.DrawFilledCircle(dst, float32(x+5), float32(cy), 3.5, col, true)
	default: // celestials, anomaly: filled dim dot
		vector.DrawFilledCircle(dst, float32(x+5), float32(cy), 3, withAlpha(col, 170), true)
	}
}

// ---------------------------------------------------------------------------
// Selected Item (top-left)
// ---------------------------------------------------------------------------

func (u *UI) drawSelected(dst *ebiten.Image, s *sim.Sim, reg Regions) {
	r := reg.Selected
	Panel(dst, r)
	PanelTitle(dst, r, "Selected Item")

	info, ok := s.Info(u.Selected)
	if !ok {
		Text(dst, "Nothing selected", r.X+7, r.Y+26, 11, ColDim)
		return
	}
	Text(dst, info.Name, r.X+7, r.Y+23, 13, ColText)
	kindCol := render.BracketColor(info.Kind, info.Hostile)
	Text(dst, info.Kind, r.X+7, r.Y+38, 10, kindCol)
	TextRight(dst, FormatDistance(info.Distance), r.Right()-7, r.Y+38, 10, ColDim)

	acts := ActionsFor(info)
	for i, ar := range ActionRects(r, len(acts)) {
		label := string(acts[i])
		if acts[i] == ActionOrbit {
			label = "Orbit " + FormatDistance(OrbitRanges[u.orbitIdx%len(OrbitRanges)])
		}
		Button(dst, ar, label, true)
	}
}

// ---------------------------------------------------------------------------
// Ship HUD (bottom-center)
// ---------------------------------------------------------------------------

func (u *UI) drawHUD(dst *ebiten.Image, s *sim.Sim, reg Regions) {
	r := reg.HUD
	Panel(dst, r)
	player := s.Player()

	// --- Shield / armor / structure rings around a ship silhouette ---
	cx, cy := r.X+56, r.Y+r.H/2
	shield, armor, hull := 1.0, 1.0, 1.0
	if sh, ok := s.Ships.Get(player); ok {
		if sh.MaxShield > 0 {
			shield = clamp(sh.Shield/sh.MaxShield, 0, 1)
		}
		if sh.MaxArmor > 0 {
			armor = clamp(sh.Armor/sh.MaxArmor, 0, 1)
		}
		if sh.MaxStructure > 0 {
			hull = clamp(sh.Structure/sh.MaxStructure, 0, 1)
		}
	}
	drawLayerRing(dst, cx, cy, 42, shield, 4, ColShield)
	drawLayerRing(dst, cx, cy, 34, armor, 4, ColWarn)
	drawLayerRing(dst, cx, cy, 26, hull, 4, ColStruct)
	drawShipGlyph(dst, cx, cy)

	// --- HP readout ---
	hpX := r.X + 108
	if sh, ok := s.Ships.Get(player); ok {
		Text(dst, "Shield "+hpStr(sh.Shield)+"/"+hpStr(sh.MaxShield), hpX, r.Y+8, 10, ColShield)
		Text(dst, "Armor "+hpStr(sh.Armor)+"/"+hpStr(sh.MaxArmor), hpX, r.Y+22, 10, ColWarn)
		Text(dst, "Structure "+hpStr(sh.Structure)+"/"+hpStr(sh.MaxStructure), hpX, r.Y+36, 10, ColStruct)
	} else {
		Text(dst, "No ship", hpX, r.Y+8, 10, ColDim)
	}

	// --- Capacitor bar ---
	capR := Rect{hpX, r.Y + 54, r.W - (hpX - r.X) - 8, 14}
	frac := 1.0
	label := ""
	if cap, ok := s.Caps.Get(player); ok && cap.Capacity > 0 {
		frac = clamp(cap.Current/cap.Capacity, 0, 1)
		label = fmtGJ(cap.Current) + " / " + fmtGJ(cap.Capacity) + " GJ"
	}
	fill := ColCap
	if frac < 0.25 {
		fill = ColDanger
	} else if frac < 0.5 {
		fill = ColWarn
	}
	Meter(dst, capR, frac, fill)
	if label != "" {
		TextCentered(dst, label, capR.X+capR.W/2, capR.Y+2, 10, ColBGHard)
	}
	Text(dst, "capacitor", hpX, r.Y+72, 9, ColDim)
	if frac < 0.999 {
		Text(dst, "recharging", hpX+58, r.Y+72, 9, withAlpha(ColWarn, 190))
	}

	// --- Speed readout + time acceleration ---
	vel := 0.0
	if m, ok := s.Motions.Get(player); ok {
		vel = m.Vel.Len()
	}
	TextRight(dst, FormatSpeed(vel), r.Right()-8, r.Y+8, 13, ColText)
	TextRight(dst, "×"+intStr(int64(u.timeScale))+" time", r.Right()-8, r.Y+26, 10, ColAccent)

	u.drawTargets(dst, s, reg)
}

// drawLayerRing draws a track ring plus a fraction fill starting at 12 o'clock.
func drawLayerRing(dst *ebiten.Image, cx, cy, rad, frac float64, width float32, col color.RGBA) {
	const twoPi = 2 * math.Pi
	start := -math.Pi / 2
	render.Arc(dst, float32(cx), float32(cy), float32(rad), float32(start), float32(start+twoPi),
		width, withAlpha(col, 55))
	if frac > 0 {
		render.Arc(dst, float32(cx), float32(cy), float32(rad), float32(start), float32(start+twoPi*frac),
			width, col)
	}
}

// drawShipGlyph draws the HUD's small ship silhouette.
func drawShipGlyph(dst *ebiten.Image, cx, cy float64) {
	pts := [][2]float64{{0, -11}, {7, 8}, {0, 3}, {-7, 8}}
	col := ColStruct
	for i := 0; i < len(pts); i++ {
		a, b := pts[i], pts[(i+1)%len(pts)]
		vector.StrokeLine(dst,
			float32(cx+a[0]), float32(cy+a[1]),
			float32(cx+b[0]), float32(cy+b[1]),
			1.5, col, true)
	}
}

// ---------------------------------------------------------------------------
// Target boxes (top-center)
// ---------------------------------------------------------------------------

func (u *UI) drawTargets(dst *ebiten.Image, s *sim.Sim, reg Regions) {
	tg, ok := s.Targetings.Get(s.Player())
	if !ok || len(tg.Locks) == 0 {
		return
	}
	for i, l := range tg.Locks {
		boxes := TargetBoxRects(reg.Targets, len(tg.Locks))
		if i >= len(boxes) {
			break
		}
		b := boxes[i]
		locked := l.Progress >= 1
		border := ColAccent
		if locked {
			border = ColShield
		}
		vector.DrawFilledRect(dst, float32(b.X), float32(b.Y), float32(b.W), float32(b.H),
			ColBG, false)
		vector.StrokeRect(dst, float32(b.X)+0.5, float32(b.Y)+0.5, float32(b.W)-1, float32(b.H)-1,
			1, border, false)

		info, hasInfo := s.Info(l.Target)
		pcX, pcY := b.X+b.W/2, b.Y+26
		// HP arcs around the portrait (three layers for ships, one bar else).
		if sh, ok := s.Ships.Get(l.Target); ok {
			drawMiniRing(dst, pcX, pcY, 18, fracOf(sh.Shield, sh.MaxShield), ColShield)
			drawMiniRing(dst, pcX, pcY, 15, fracOf(sh.Armor, sh.MaxArmor), ColWarn)
			drawMiniRing(dst, pcX, pcY, 12, fracOf(sh.Structure, sh.MaxStructure), ColStruct)
		} else if hasInfo {
			drawMiniRing(dst, pcX, pcY, 16, info.Health, ColGood)
		}
		// Portrait placeholder polygon.
		col := ColStruct
		if hasInfo {
			col = render.BracketColor(info.Kind, info.Hostile)
		}
		vector.StrokeLine(dst, float32(pcX), float32(pcY-8), float32(pcX+6), float32(pcY+6), 1.2, col, true)
		vector.StrokeLine(dst, float32(pcX+6), float32(pcY+6), float32(pcX-6), float32(pcY+6), 1.2, col, true)
		vector.StrokeLine(dst, float32(pcX-6), float32(pcY+6), float32(pcX), float32(pcY-8), 1.2, col, true)

		name := ""
		if hasInfo {
			name = ellipsize(info.Name, b.W-8, 10)
		}
		TextCentered(dst, name, pcX, b.Y+48, 10, ColText)

		// Lock progress / locked state strip.
		pr := Rect{b.X + 4, b.Bottom() - 8, b.W - 8, 4}
		if locked {
			Meter(dst, pr, 1, ColShield)
		} else {
			Meter(dst, pr, l.Progress, ColAccent)
		}
	}
}

// drawMiniRing draws a small fraction ring (target portrait HP arcs).
func drawMiniRing(dst *ebiten.Image, cx, cy, rad, frac float64, col color.RGBA) {
	const twoPi = 2 * math.Pi
	start := -math.Pi / 2
	render.Arc(dst, float32(cx), float32(cy), float32(rad), float32(start), float32(start+twoPi),
		2, withAlpha(col, 50))
	if frac > 0 {
		render.Arc(dst, float32(cx), float32(cy), float32(rad), float32(start), float32(start+twoPi*frac),
			2, col)
	}
}

// fracOf returns cur/max clamped to 0..1 (0 when max is unknown).
func fracOf(cur, max float64) float64 {
	if max <= 0 {
		return 0
	}
	return clamp(cur/max, 0, 1)
}

// ---------------------------------------------------------------------------
// Module rack (bottom-right)
// ---------------------------------------------------------------------------

func (u *UI) drawModules(dst *ebiten.Image, s *sim.Sim, reg Regions) {
	r := reg.Modules
	Panel(dst, r)
	PanelTitle(dst, r, "Modules")
	player := s.Player()
	fit, ok := s.Fittings.Get(player)
	if !ok {
		return
	}
	classes := [modRows]string{content.SlotHigh, content.SlotMid, content.SlotLow}
	labels := [modRows]string{"High", "Mid", "Low"}
	slots := [modRows]int{}
	if sh, ok := s.Ships.Get(player); ok {
		if def, ok := u.c.Ship(sh.Hull); ok {
			slots = [modRows]int{def.Slots.High, def.Slots.Mid, def.Slots.Low}
		}
	}
	now := s.Time()
	maxCells := ModuleMaxCells(r)

	for row := 0; row < modRows; row++ {
		class := classes[row]
		lr := ModuleRowLabelRect(r, row)
		Text(dst, labels[row], lr.X, lr.Y, 10, ColHead)

		defs := fit.Defs(class)
		rts := fit.Runtimes(class)
		count := len(defs)
		if slots[row] > count {
			count = slots[row]
		}
		if count > maxCells {
			count = maxCells
		}
		for i := 0; i < count; i++ {
			cell := ModuleCellRect(r, row, i)
			var def *content.ModuleDef
			if i < len(defs) {
				def, _ = u.c.Module(defs[i])
			}
			active := i < len(rts) && rts[i].Active

			bg := ColCell
			border := ColBorder
			if def == nil {
				border = withAlpha(ColBorder, 110) // empty slot
			}
			if active {
				bg = withAlpha(ColAccent, 60)
				border = ColAccent
			}
			vector.DrawFilledRect(dst, float32(cell.X), float32(cell.Y), float32(cell.W), float32(cell.H),
				bg, false)
			vector.StrokeRect(dst, float32(cell.X)+0.5, float32(cell.Y)+0.5,
				float32(cell.W)-1, float32(cell.H)-1, 1, border, false)

			if def != nil {
				letter := moduleLetter(def.Name)
				accent := moduleAccent(def.Kind)
				TextCentered(dst, letter, cell.X+cell.W/2, cell.Y+cell.H/2-7, 14, accent)
				if active && def.ActivationTime > 0 && i < len(rts) && rts[i].CycleEnds > 0 {
					p := clamp((now-(rts[i].CycleEnds-def.ActivationTime))/def.ActivationTime, 0, 1)
					Meter(dst, Rect{cell.X + 2, cell.Bottom() - 5, cell.W - 4, 3}, p, ColAccent)
				}
			}
			// F-key labels on high slots (EVE style).
			if row == 0 {
				TextCentered(dst, "F"+intStr(int64(i+1)), cell.X+cell.W/2, cell.Y-11, 9, ColDim)
			}
		}
	}
}

// moduleLetter returns the first upper-case rune of a module name.
func moduleLetter(name string) string {
	for _, c := range name {
		if c == ' ' {
			continue
		}
		return string(upperRune(c))
	}
	return "?"
}

// upperRune upper-cases ASCII a-z (module names are English).
func upperRune(c rune) rune {
	if c >= 'a' && c <= 'z' {
		return c - 'a' + 'A'
	}
	return c
}

// moduleAccent colors the module letter by kind.
func moduleAccent(kind string) color.RGBA {
	switch kind {
	case content.KindWeapon:
		return ColDanger
	case content.KindShieldBoost, content.KindArmorRep:
		return ColShield
	case content.KindAfterburner:
		return ColAccent
	case content.KindMiningLaser:
		return ColGood
	case content.KindStasisWeb, content.KindWarpScram:
		return mustHex("#b07fe0")
	case content.KindCapBooster, content.KindCapRecharger:
		return mustHex("#e0d060")
	default:
		return ColInfo
	}
}

// ---------------------------------------------------------------------------
// Small text/format helpers
// ---------------------------------------------------------------------------

// intStr formats an integer without fmt.
func intStr(n int64) string {
	return groupInt(n)
}

// hpStr formats whole hit points from a float HP value.
func hpStr(v float64) string {
	if v < 0 {
		v = 0
	}
	return groupInt(int64(v + 0.5))
}

// fmtGJ formats a GJ amount with one decimal.
func fmtGJ(v float64) string {
	return groupFloat(v, 1)
}

// ellipsize shortens s to fit maxPx at the given font size.
func ellipsize(s string, maxPx, size float64) string {
	if render.Measure(s, size) <= maxPx {
		return s
	}
	runes := []rune(s)
	for len(runes) > 0 {
		runes = runes[:len(runes)-1]
		cand := string(runes) + "..."
		if render.Measure(cand, size) <= maxPx {
			return cand
		}
	}
	return ""
}
