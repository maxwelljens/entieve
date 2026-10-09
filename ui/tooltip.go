package ui

import (
	"github.com/entieve/entieve/content"
	"github.com/entieve/entieve/render"
	"github.com/hajimehoshi/ebiten/v2"
)

// tooltipDelay is the hover time (real seconds) before a module tooltip
// appears.
const tooltipDelay = 0.3

// TooltipVisible reports whether a tooltip whose hover started at hoverSince
// should be showing at UI clock now (≥ 300 ms delay).
func TooltipVisible(hoverSince, now float64) bool {
	return now-hoverSince >= tooltipDelay
}

// ModuleTipLines builds the tooltip body for a module rack cell: module
// name, slot and key stats — weapons show damage/optimal/tracking plus the
// loaded charge, boosters show their boost numbers, mining lasers their
// yield. Pure and tested.
func ModuleTipLines(def *content.ModuleDef, slotLabel, ammoName string) []string {
	if def == nil {
		return nil
	}
	lines := []string{def.Name, slotLabel}
	switch {
	case def.Weapon != nil:
		w := def.Weapon
		volley := w.Damage.EM + w.Damage.Thermal + w.Damage.Kinetic + w.Damage.Explosive
		lines = append(lines,
			"Damage "+groupFloat(volley, 1)+" per shot",
			"Optimal "+FormatDistance(w.Optimal)+"  Falloff "+FormatDistance(w.Falloff),
			"Tracking "+groupFloat(w.Tracking, 2)+" rad/s",
		)
		if w.AmmoClass != "" {
			lines = append(lines, "Charge: "+ammoName)
		}
	case def.Mining != nil:
		m := def.Mining
		lines = append(lines,
			"Yields "+groupFloat(m.VolumePerCycle, 1)+" m³ / "+groupFloat(m.CycleTime, 1)+" s",
		)
	case def.Boost != nil:
		b := def.Boost
		cycle := ""
		if def.ActivationTime > 0 {
			cycle = " / " + groupFloat(def.ActivationTime, 1) + " s"
		}
		if b.ShieldBoostPerCycle > 0 {
			lines = append(lines, "Shield +"+groupFloat(b.ShieldBoostPerCycle, 1)+" HP"+cycle)
		}
		if b.ArmorRepPerCycle > 0 {
			lines = append(lines, "Armor +"+groupFloat(b.ArmorRepPerCycle, 1)+" HP"+cycle)
		}
		if b.SpeedMultiplier > 1 {
			lines = append(lines, "Speed ×"+groupFloat(b.SpeedMultiplier, 2))
		}
		if b.WebStrength > 0 {
			lines = append(lines, "Web −"+FormatPct(b.WebStrength)+" target speed")
		}
		if b.ScramDuration > 0 {
			lines = append(lines, "Warp scramble "+groupFloat(b.ScramDuration, 1)+" s")
		}
		if b.CapRestore > 0 {
			lines = append(lines, "Restores "+groupFloat(b.CapRestore, 1)+" GJ capacitor")
		}
	}
	if def.ActivationCost > 0 {
		lines = append(lines, "Uses "+groupFloat(def.ActivationCost, 1)+" GJ per cycle")
	}
	return lines
}

// TooltipRect lays out a tooltip for lines near (x, y), clamped inside the
// screen. Width follows the longest line.
func TooltipRect(x, y, screenW, screenH float64, lines []string) Rect {
	var maxW float64
	for _, ln := range lines {
		if w := render.Measure(ln, FontSmall); w > maxW {
			maxW = w
		}
	}
	w := maxW + 2*padSmall + 4
	h := float64(len(lines))*lineH(FontSmall) + 2*padSmall
	r := Rect{X: x + padSmall, Y: y + padSmall, W: w, H: h}
	if r.Right() > screenW {
		r.X = x - padSmall - w
	}
	if r.Bottom() > screenH {
		r.Y = y - padSmall - h
	}
	return ClampRectToScreen(r, screenW, screenH)
}

// DrawTooltip draws a small floating tooltip box with the given lines.
func DrawTooltip(dst *ebiten.Image, r Rect, lines []string) {
	Panel(dst, r)
	y := r.Y + padSmall
	for _, ln := range lines {
		Text(dst, ln, r.X+padSmall, y, FontSmall, ColText)
		y += lineH(FontSmall)
	}
}
