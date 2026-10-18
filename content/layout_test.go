package content

import (
	"math"
	"testing"

	"github.com/entieve/entieve/core"
)

// ---------------------------------------------------------------------------
// System layout contract: the shipped systems are real-solar-system scale.
// These pins derive from the committed content/data/systems/*.json and keep
// the rescale (docs/SIM.md — "System layout") from silently re-bunching.
// ---------------------------------------------------------------------------

// auMeters is the house AU conversion (1 AU = 1.496e11 m) — the same value
// warpSpeedMS and FormatDistance use. Do not "correct" it.
const auMeters = 1.496e11

// TestSystemLayoutContract pins the placement bands and separation floors of
// every shipped system: star at the origin, inner planets close in, belts and
// belt-like anomalies in the middle belt, stations spread out, stargates on
// the rim — and nothing sharing a grid with anything else.
func TestSystemLayoutContract(t *testing.T) {
	c := MustLoad()
	bands := map[string][2]float64{ // kind -> [min, max] AU from the star
		CelPlanet:  {0.4, 2},
		CelMoon:    {0.4, 2},
		CelBelt:    {2, 10},
		CelAnomaly: {2, 10},
		CelStation: {1, 10},
		CelGate:    {20, 40},
	}
	for _, id := range keys(c.Systems) {
		sys := c.Systems[id]
		var inner []CelestialDef // station/belt/anomaly: must never bunch
		var gates []CelestialDef
		for i := range sys.Celestials {
			cel := &sys.Celestials[i]
			r := cel.Position.Len() / auMeters
			if cel.Kind == CelStar {
				if cel.Position != (core.Vec2{}) {
					t.Errorf("%s/%s: star at %v, want the origin", sys.ID, cel.ID, cel.Position)
				}
				continue
			}
			band, ok := bands[cel.Kind]
			if !ok {
				continue
			}
			if r < band[0] || r > band[1] {
				t.Errorf("%s/%s (%s) sits %.3f AU from the star, want %.1f–%.1f AU",
					sys.ID, cel.ID, cel.Kind, r, band[0], band[1])
			}
			switch cel.Kind {
			case CelGate:
				gates = append(gates, *cel)
			case CelStation, CelBelt, CelAnomaly:
				inner = append(inner, *cel) // planets may sit near their station
			}
		}
		for i := range inner {
			for j := i + 1; j < len(inner); j++ {
				d := inner[i].Position.Dist(inner[j].Position) / auMeters
				if d < 2 {
					t.Errorf("%s: %s and %s sit %.2f AU apart, want >= 2 AU (stations/belts must stay far apart)",
						sys.ID, inner[i].ID, inner[j].ID, d)
				}
			}
		}
		for i := range gates {
			for j := i + 1; j < len(gates); j++ {
				d := gates[i].Position.Dist(gates[j].Position) / auMeters
				if d < 5 {
					t.Errorf("%s: gates %s and %s sit %.2f AU apart, want >= 5 AU (rim gates must not bunch)",
						sys.ID, gates[i].ID, gates[j].ID, d)
				}
			}
		}
	}
}

// TestSystemSpansSystemScale is the one-number guard on the rescale: every
// shipped system must span tens of AU across its widest pair of bodies, so a
// revert to the old ~0.1 AU layout cannot pass.
func TestSystemSpansSystemScale(t *testing.T) {
	c := MustLoad()
	for _, id := range keys(c.Systems) {
		sys := c.Systems[id]
		widest := 0.0
		for i := range sys.Celestials {
			for j := i + 1; j < len(sys.Celestials); j++ {
				widest = math.Max(widest, sys.Celestials[i].Position.Dist(sys.Celestials[j].Position))
			}
		}
		if span := widest / auMeters; span < 20 {
			t.Errorf("%s spans %.2f AU across, want >= 20 AU (system-scale layout)", sys.ID, span)
		}
	}
}
