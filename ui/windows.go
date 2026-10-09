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

// button draws a chrome button with pointer hover/pressed feedback.
func (u *UI) button(dst *ebiten.Image, r Rect, label string, enabled bool) {
	hover := r.Contains(u.mx, u.my)
	ButtonState(dst, r, label, enabled, hover, enabled && hover && u.mouse0)
}

// ---------------------------------------------------------------------------
// Overview (top-right)
// ---------------------------------------------------------------------------

func (u *UI) drawOverview(dst *ebiten.Image, s *sim.Sim, reg Regions) {
	r := u.WindowRect(WinOverview, reg)
	rows := u.overviewRows()
	body := OverviewBody(r)
	u.ovScroll = ClampScroll(u.ovScroll, float64(len(rows))*ovRowH, body.H)

	DrawWindow(dst, r, WindowTitle(WinOverview), closableWindows[WinOverview])

	// Filter toggle in the title bar (opens the kind-filter dropdown).
	fb := OverviewFilterButtonRect(r)
	if fb.Contains(u.mx, u.my) {
		vector.DrawFilledRect(dst, float32(fb.X), float32(fb.Y), float32(fb.W), float32(fb.H),
			hoverFill(18), false)
	}
	arrowCol := ColDim
	if u.ovFilterOpen {
		arrowCol = ColAccent
	}
	drawSortArrow(dst, fb.X+fb.W/2, fb.Y+fb.H/2, 11, u.ovFilterOpen, arrowCol)

	// Column header strip: active column tinted accent + ▲/▼ indicator.
	head := OverviewHeaderRect(r)
	vector.DrawFilledRect(dst, float32(head.X), float32(head.Y), float32(head.W), float32(head.H),
		ColHeadBg, false)
	headers := [4]string{"", "Name", "Distance", "Velocity"}
	textY := head.Y + (head.H-lineH(FontSmall))/2
	for c := 0; c < 4; c++ {
		col := OverviewColumn(r, c)
		label := headers[c]
		active := SortColumn(c) == u.sortCol && label != ""
		headCol := ColHead
		if active {
			vector.DrawFilledRect(dst, float32(col.X), float32(col.Y), float32(col.W), float32(col.H),
				hoverFill(24), false)
			headCol = ColAccent
		}
		show, up := SortIndicator(SortColumn(c), active, u.sortAsc)
		arrowW := 0.0
		if show {
			arrowW = 14
		}
		if c == 2 || c == 3 { // right-align numeric headers on fixed anchors
			TextRight(dst, label, NumberAnchor(col)-arrowW, textY, FontSmall, headCol)
		} else {
			Text(dst, label, col.X+3, textY, FontSmall, headCol)
		}
		if show {
			drawSortArrow(dst, col.Right()-padSmall-4, head.Y+head.H/2, 8, up, ColAccent)
		}
	}

	// Data rows, clipped to the body so scrolling never overflows chrome.
	first := int(u.ovScroll / ovRowH)
	if first < 0 {
		first = 0
	}
	last := first + OverviewMaxRows(r) + 1
	nameCol := OverviewColumn(r, 1)
	distCol := OverviewColumn(r, 2)
	velCol := OverviewColumn(r, 3)
	full := dst
	dst = subImage(dst, body) // clip row drawing to the viewport
	for i := first; i <= last && i < len(rows); i++ {
		inf := rows[i]
		row := OverviewRowRect(r, i, u.ovScroll)
		if row.Bottom() < body.Y || row.Y > body.Bottom() {
			continue
		}
		if i%2 == 1 {
			vector.DrawFilledRect(dst, float32(row.X), float32(row.Y), float32(row.W), float32(row.H),
				withAlpha(ColBGHard, 90), false)
		}
		if row.Contains(u.mx, u.my) {
			vector.DrawFilledRect(dst, float32(row.X), float32(row.Y), float32(row.W), float32(row.H),
				hoverFill(22), false) // hover: subtle accent tint
		}
		if inf.Entity == u.Selected {
			vector.DrawFilledRect(dst, float32(row.X), float32(row.Y), float32(row.W), float32(row.H),
				withAlpha(ColAccent, 36), false)
		}
		// Optical alignment: the icon centers on the text line, not the
		// geometric row box.
		rowTextY := row.Y + (row.H-lineH(FontSmall))/2
		iconCY := rowTextY + lineH(FontSmall)/2 - FontSmall*0.08
		drawKindIcon(dst, inf.Kind, inf.Entity == u.s.Player(), row.X+padSmall, iconCY)

		nameX := row.X + ovIconW + padSmall
		nameColor := render.BracketColor(inf.Kind, inf.Hostile)
		if inf.Entity == u.s.Player() {
			nameColor = ColPlayer
		}
		Text(dst, ellipsize(inf.Name, nameCol.Right()-nameX-padSmall, FontSmall),
			nameX, rowTextY, FontSmall, nameColor)
		TextRight(dst, FormatDistance(inf.Distance), NumberAnchor(distCol), rowTextY,
			FontSmall, ColText)
		TextRight(dst, FormatSpeed(inf.Vel.Len()), NumberAnchor(velCol), rowTextY,
			FontSmall, ColDim)
	}
	dst = full // scrollbar and popup draw unclipped

	// Scrollbar (reserved strip right of the rows): track + 2-rect thumb.
	contentH := float64(len(rows)) * ovRowH
	if contentH > body.H {
		track := ScrollbarTrack(body)
		thumb := ScrollThumbRect(track, contentH, body.H, u.ovScroll)
		vector.DrawFilledRect(dst, float32(track.X), float32(track.Y), float32(track.W),
			float32(track.H), withAlpha(ColCell, 200), false)
		vector.DrawFilledRect(dst, float32(thumb.X), float32(thumb.Y), float32(thumb.W),
			float32(thumb.H), withAlpha(ColHead, 235), false)
		inner := Rect{X: thumb.X + 2, Y: thumb.Y + 2, W: thumb.W - 4, H: thumb.H - 4}
		if inner.W > 0 && inner.H > 0 {
			vector.DrawFilledRect(dst, float32(inner.X), float32(inner.Y), float32(inner.W),
				float32(inner.H), withAlpha(ColDim, 140), false) // rounded-feel stack
		}
	}

	// Kind-filter dropdown (popup over the rows).
	if u.ovFilterOpen {
		u.drawFilterPopup(dst, r)
	}
}

// drawFilterPopup renders the overview's filter dropdown.
func (u *UI) drawFilterPopup(dst *ebiten.Image, r Rect) {
	p := OverviewFilterPopupRect(r)
	Panel(dst, p)
	for i, g := range filterGroups {
		rr := OverviewFilterRowRect(p, i)
		if rr.Contains(u.mx, u.my) {
			vector.DrawFilledRect(dst, float32(rr.X), float32(rr.Y), float32(rr.W), float32(rr.H),
				hoverFill(20), false)
		}
		box := Rect{X: rr.X + padSmall, Y: rr.Y + (rr.H-(minHit-2*padSmall))/2,
			W: minHit - 2*padSmall, H: minHit - 2*padSmall}
		vector.StrokeRect(dst, float32(box.X)+0.5, float32(box.Y)+0.5, float32(box.W)-1,
			float32(box.H)-1, 1, ColBorder, false)
		on := u.ovFilters[g]
		if on {
			vector.DrawFilledRect(dst, float32(box.X+3), float32(box.Y+3),
				float32(box.W-6), float32(box.H-6), ColAccent, false)
		}
		fg := ColDim
		if on {
			fg = ColText
		}
		Text(dst, filterGroupLabels[i], box.Right()+padSmall,
			rr.Y+(rr.H-lineH(FontSmall))/2, FontSmall, fg)
	}
}

// drawKindIcon draws the overview's tiny per-kind icon (1.5× the legacy
// glyph, optically centered on the row's text line).
func drawKindIcon(dst *ebiten.Image, kind string, isPlayer bool, x, cy float64) {
	col := render.BracketColor(kind, false)
	if isPlayer {
		col = ColPlayer
	}
	w := float32(1.4)
	switch kind {
	case sim.KindShip, sim.KindPlayer:
		vector.StrokeLine(dst, float32(x+1.5), float32(cy-6), float32(x+13.5), float32(cy), w, col, true)
		vector.StrokeLine(dst, float32(x+13.5), float32(cy), float32(x+1.5), float32(cy+6), w, col, true)
		vector.StrokeLine(dst, float32(x+1.5), float32(cy+6), float32(x+1.5), float32(cy-6), w, col, true)
	case sim.KindStation:
		vector.StrokeRect(dst, float32(x+3), float32(cy-6), 10.5, 12, w, col, true)
	case sim.KindGate:
		vector.StrokeCircle(dst, float32(x+7.5), float32(cy), 6, w, col, true)
	case sim.KindWreck:
		vector.StrokeLine(dst, float32(x+3), float32(cy-4.5), float32(x+12), float32(cy+4.5), w, col, true)
		vector.StrokeLine(dst, float32(x+12), float32(cy-4.5), float32(x+3), float32(cy+4.5), w, col, true)
	case sim.KindAsteroid:
		vector.DrawFilledCircle(dst, float32(x+7.5), float32(cy), 5.25, col, true)
	default: // celestials, belts, anomaly: filled dim dot
		vector.DrawFilledCircle(dst, float32(x+7.5), float32(cy), 4.5, withAlpha(col, 170), true)
	}
}

// ---------------------------------------------------------------------------
// Selected Item (top-left)
// ---------------------------------------------------------------------------

func (u *UI) drawSelected(dst *ebiten.Image, s *sim.Sim, reg Regions) {
	r := u.WindowRect(WinSelected, reg)
	DrawWindow(dst, r, WindowTitle(WinSelected), false)
	nameY := r.Y + titleBarH + pad
	kindY := nameY + lineH(FontLarge)

	info, ok := s.Info(u.Selected)
	if ok {
		Text(dst, ellipsize(info.Name, r.W-2*pad, FontLarge), r.X+pad, nameY, FontLarge, ColText)
		kindCol := render.BracketColor(info.Kind, info.Hostile)
		Text(dst, info.Kind, r.X+pad, kindY, FontSmall, kindCol)
		TextRight(dst, FormatDistance(info.Distance), r.Right()-1-pad, kindY, FontSmall, ColDim)
	} else {
		Text(dst, "Nothing selected", r.X+pad, nameY, FontSmall, ColDim)
	}

	// Warp-to-within segmented control (0 | 10 | 50 km).
	for i, sr := range WarpSegRects(r, len(WarpRanges)) {
		label := itoa(int(WarpRanges[i] / 1000))
		u.button(dst, sr, label, true)
		if i == u.warpIdx {
			vector.StrokeRect(dst, float32(sr.X)+0.75, float32(sr.Y)+0.75,
				float32(sr.W)-1.5, float32(sr.H)-1.5, 1.5, ColAccent, false)
			TextCentered(dst, label, sr.X+sr.W/2, sr.Y+(sr.H-lineH(FontSmall))/2,
				FontSmall, ColAccent)
		}
	}
	wb := WarpButtonRect(r, len(WarpRanges))
	u.button(dst, wb, WarpRangeLabel(u.warpIdx), ok && info.CanWarp)

	// Action grid (the warp action lives in its own row above).
	if ok {
		grid := actionsExcept(ActionsFor(info), ActionWarp)
		for i, ar := range ActionRects(r, len(grid)) {
			u.button(dst, ar, string(grid[i]), true)
		}
	}
}

// ---------------------------------------------------------------------------
// Ship HUD (bottom-center chrome) + target boxes
// ---------------------------------------------------------------------------

func (u *UI) drawHUD(dst *ebiten.Image, s *sim.Sim, reg Regions) {
	r := reg.HUD
	Panel(dst, r)
	player := s.Player()

	// Shield / armor / structure rings around a ship silhouette.
	cx, cy := r.X+hudH/2, r.Y+hudH/2
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
	arcW := float32(hudRingStep / 3)
	drawLayerRing(dst, cx, cy, hudRingR, shield, arcW, ColShield)
	drawLayerRing(dst, cx, cy, hudRingR-hudRingStep, armor, arcW, ColWarn)
	drawLayerRing(dst, cx, cy, hudRingR-2*hudRingStep, hull, arcW, ColStruct)
	drawShipGlyph(dst, cx, cy)

	// HP readout (left of the speed block, ellipsized so numbers never
	// collide with the right-aligned speed/time).
	hpX := cx + hudRingR + pad
	speedText := FormatSpeed(0)
	if m, ok := s.Motions.Get(player); ok {
		speedText = FormatSpeed(m.Vel.Len())
	}
	timeText := "×" + intStr(int64(u.timeScale)) + " time"
	speedBlock := math.Max(render.Measure(speedText, FontBase), render.Measure(timeText, FontSmall))
	hpMaxW := r.Right() - pad - speedBlock - pad - hpX
	if sh, ok := s.Ships.Get(player); ok {
		lines := [][2]string{
			{"Shield " + hpStr(sh.Shield) + "/" + hpStr(sh.MaxShield), ""},
			{"Armor " + hpStr(sh.Armor) + "/" + hpStr(sh.MaxArmor), ""},
			{"Structure " + hpStr(sh.Structure) + "/" + hpStr(sh.MaxStructure), ""},
		}
		cols := [3]color.RGBA{ColShield, ColWarn, ColStruct}
		for i, ln := range lines {
			Text(dst, ellipsize(ln[0], hpMaxW, FontSmall), hpX,
				r.Y+pad+float64(i)*lineH(FontSmall), FontSmall, cols[i])
		}
	} else {
		Text(dst, "No ship", hpX, r.Y+pad, FontSmall, ColDim)
	}

	// Capacitor bar.
	capR := Rect{X: hpX, Y: r.Y + pad + 3*lineH(FontSmall), W: r.Right() - pad - hpX, H: hudCapH}
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
		TextCentered(dst, label, capR.X+capR.W/2, capR.Y+(capR.H-lineH(FontSmall))/2,
			FontSmall, ColBGHard)
	}
	capLabelY := capR.Bottom()
	Text(dst, "capacitor", hpX, capLabelY, FontTiny, ColDim)
	if frac < 0.999 {
		rechX := hpX + render.Measure("capacitor", FontTiny) + padSmall
		Text(dst, "recharging", rechX, capLabelY, FontTiny, withAlpha(ColWarn, 190))
	}

	// Speed readout + time acceleration (right-aligned, fixed anchors).
	TextRight(dst, speedText, r.Right()-pad, r.Y+pad, FontBase, ColText)
	TextRight(dst, timeText, r.Right()-pad, r.Y+pad+lineH(FontBase), FontSmall, ColAccent)

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

// drawShipGlyph draws the HUD's ship silhouette (sized off the ring radii).
func drawShipGlyph(dst *ebiten.Image, cx, cy float64) {
	pts := [][2]float64{
		{0, -hudRingR * 0.30},
		{hudRingR * 0.19, hudRingR * 0.24},
		{0, hudRingR * 0.08},
		{-hudRingR * 0.19, hudRingR * 0.24},
	}
	col := ColStruct
	for i := 0; i < len(pts); i++ {
		a, b := pts[i], pts[(i+1)%len(pts)]
		vector.StrokeLine(dst,
			float32(cx+a[0]), float32(cy+a[1]),
			float32(cx+b[0]), float32(cy+b[1]),
			1.5, col, true)
	}
}

// drawTargets draws the top-center lock list (chrome panel).
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
		pcX, pcY := b.X+b.W/2, b.Y+padSmall+targetBoxW*0.25
		// HP arcs around the portrait (three layers for ships, one else).
		if sh, ok := s.Ships.Get(l.Target); ok {
			drawMiniRing(dst, pcX, pcY, targetBoxW*0.24, fracOf(sh.Shield, sh.MaxShield), ColShield)
			drawMiniRing(dst, pcX, pcY, targetBoxW*0.20, fracOf(sh.Armor, sh.MaxArmor), ColWarn)
			drawMiniRing(dst, pcX, pcY, targetBoxW*0.16, fracOf(sh.Structure, sh.MaxStructure), ColStruct)
		} else if hasInfo {
			drawMiniRing(dst, pcX, pcY, targetBoxW*0.20, info.Health, ColGood)
		}
		// Portrait placeholder polygon.
		col := ColStruct
		if hasInfo {
			col = render.BracketColor(info.Kind, info.Hostile)
		}
		vector.StrokeLine(dst, float32(pcX), float32(pcY-10), float32(pcX+7), float32(pcY+7), 1.4, col, true)
		vector.StrokeLine(dst, float32(pcX+7), float32(pcY+7), float32(pcX-7), float32(pcY+7), 1.4, col, true)
		vector.StrokeLine(dst, float32(pcX-7), float32(pcY+7), float32(pcX), float32(pcY-10), 1.4, col, true)

		name := ""
		if hasInfo {
			name = ellipsize(info.Name, b.W-2*padSmall, FontSmall)
		}
		TextCentered(dst, name, pcX, b.Y+targetBoxW+padSmall, FontSmall, ColText)

		// Lock progress / locked state strip.
		pr := Rect{X: b.X + padSmall, Y: b.Y + targetBoxH - 6, W: b.W - 2*padSmall, H: 4}
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
	r := u.WindowRect(WinModules, reg)
	DrawWindow(dst, r, WindowTitle(WinModules), false)
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
		Text(dst, labels[row], lr.X, lr.Y, FontTiny, ColHead)

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
			hover := def != nil && cell.Contains(u.mx, u.my)

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
			if hover {
				vector.DrawFilledRect(dst, float32(cell.X), float32(cell.Y), float32(cell.W),
					float32(cell.H), hoverFill(24), false)
				if !active {
					border = withAlpha(ColAccent, 180)
				}
			}
			if hover && u.mouse0 && def != nil {
				// Pressed feedback: dimmed fill + inset ring.
				vector.DrawFilledRect(dst, float32(cell.X), float32(cell.Y), float32(cell.W),
					float32(cell.H), withAlpha(ColBGHard, 90), false)
				vector.StrokeRect(dst, float32(cell.X)+1.5, float32(cell.Y)+1.5,
					float32(cell.W)-3, float32(cell.H)-3, 1, withAlpha(ColBGHard, 160), false)
			}
			vector.StrokeRect(dst, float32(cell.X)+0.5, float32(cell.Y)+0.5,
				float32(cell.W)-1, float32(cell.H)-1, 1, border, false)

			if def != nil {
				letter := moduleLetter(def.Name)
				accent := moduleAccent(def.Kind)
				TextCentered(dst, letter, cell.X+cell.W/2,
					cell.Y+(cell.H-lineH(FontLarge))/2, FontLarge, accent)
				if active && def.ActivationTime > 0 && i < len(rts) && rts[i].CycleEnds > 0 {
					p := clamp((now-(rts[i].CycleEnds-def.ActivationTime))/def.ActivationTime, 0, 1)
					Meter(dst, Rect{X: cell.X + padSmall, Y: cell.Bottom() - 4,
						W: cell.W - 2*padSmall, H: 3}, p, ColAccent)
				}
			}
			// F-key labels on high slots (EVE style), in the row label band.
			if row == 0 {
				TextCentered(dst, "F"+intStr(int64(i+1)), cell.X+cell.W/2,
					cell.Y-lineH(FontTiny), FontTiny, ColDim)
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
	if maxPx <= 0 {
		return ""
	}
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
