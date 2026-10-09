package ui

import (
	"sort"

	"github.com/entieve/entieve/render"
	"github.com/entieve/entieve/sim"
	"github.com/hajimehoshi/ebiten/v2"
)

// scanCycle is the scanner's slow sweep period (seconds).
const scanCycle = 2.0

// ScanCount is one anonymous off-grid contact group.
type ScanCount struct {
	Kind string
	N    int
}

// scanKindOrder fixes the display order of aggregated kinds: known
// contact types first, then any others alphabetically.
var scanKindOrder = [...]string{"ship", "asteroid", "wreck"}

// ScanKindLabel renders an anonymous count label: "Ship ×3". No names, no
// distances — the sweep only reports how many of each kind are out there.
func ScanKindLabel(kind string, n int) string {
	name := ""
	switch kind {
	case "ship":
		name = "Ship"
	case "asteroid":
		name = "Asteroid"
	case "wreck":
		name = "Wreck"
	case "player":
		name = "Ship"
	default:
		if n > 0 && kind != "" {
			name = string(kind[0]-('a'-'A')) + kind[1:]
		}
	}
	return name + " ×" + itoa(n)
}

// ScanCounts aggregates OFF-grid contacts (Info.Visible == false) within
// 3 × sensorRange of the player into anonymous per-kind counts — the
// deliberate anti-omniscience of the Scanner window: players learn what is
// out there, never exactly where or what it is called. The player itself
// and landmarks (always Visible) never appear. Order is deterministic.
func ScanCounts(infos []sim.Info, sensorRange float64) []ScanCount {
	maxD := 3 * sensorRange
	counts := map[string]int{}
	for _, inf := range infos {
		if inf.Visible || inf.Kind == "player" {
			continue
		}
		if inf.Distance > maxD {
			continue
		}
		counts[inf.Kind]++
	}
	out := make([]ScanCount, 0, len(counts))
	seen := map[string]bool{}
	for _, k := range scanKindOrder {
		if counts[k] > 0 {
			out = append(out, ScanCount{Kind: k, N: counts[k]})
			seen[k] = true
		}
	}
	var rest []string
	for k := range counts {
		if !seen[k] {
			rest = append(rest, k)
		}
	}
	sort.Strings(rest)
	for _, k := range rest {
		out = append(out, ScanCount{Kind: k, N: counts[k]})
	}
	return out
}

// SweepPhase returns the sweep progress in [0,1) for elapsed time t.
func SweepPhase(t, cycle float64) float64 {
	if cycle <= 0 {
		return 0
	}
	m := t - float64(int(t/cycle))*cycle
	if m < 0 {
		m += cycle
	}
	return m / cycle
}

// advanceScanner ticks the sweep clock and refreshes the anonymous counts
// when a sweep completes (or before the first one).
func (u *UI) advanceScanner(dt float64) {
	prev := SweepPhase(u.scanT, scanCycle)
	u.scanT += dt
	cur := SweepPhase(u.scanT, scanCycle)
	if u.scanCounts == nil || cur < prev {
		u.scanCounts = ScanCounts(u.s.Grid(), sim.SensorRange)
	}
}

// drawScanner renders the Scanner window: sweep progress bar plus the
// anonymous off-grid contact counts from the last completed sweep.
func (u *UI) drawScanner(dst *ebiten.Image, reg Regions) {
	r := u.WindowRect(WinScanner, reg)
	DrawWindow(dst, r, WindowTitle(WinScanner), closableWindows[WinScanner])
	contentY := r.Y + titleBarH

	phase := SweepPhase(u.scanT, scanCycle)
	Text(dst, "Sweep "+FormatPct(phase), r.X+pad, contentY+pad, FontTiny, ColDim)
	TextRight(dst, "range "+FormatDistance(3*sim.SensorRange), r.Right()-1-pad,
		contentY+pad, FontTiny, ColDim)
	barY := contentY + pad + scanStatusH
	Meter(dst, Rect{X: r.X + pad, Y: barY, W: r.W - 2*pad, H: scanBarH}, phase, ColAccent)

	countsY := barY + scanBarH + padSmall
	if len(u.scanCounts) == 0 {
		Text(dst, "No off-grid contacts", r.X+pad, countsY, FontSmall, ColDim)
		return
	}
	for i, c := range u.scanCounts {
		if i >= scanLines {
			break
		}
		col := render.BracketColor(c.Kind, false)
		Text(dst, ScanKindLabel(c.Kind, c.N), r.X+pad, countsY+float64(i)*scanLineH,
			FontSmall, col)
	}
}
