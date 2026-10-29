package theme

import (
	"image/color"
	"testing"

	"github.com/entieve/entieve/uikit/icons"
)

// statCensus is every stat label the interface renders, by family (the
// binding taxonomy). A new row label belongs here AND in statStyles, so a
// later stat cannot ship iconless by accident.
var statCensus = []string{
	"Per shot", "Total DPS", "Optimal", "Falloff", "Tracking",
	"Shield", "Armor", "Structure", "Restored",
	"Capacitor", "Capacity", "Recharge", "Cap use", "Cap drain", "Cap recharge",
	"Max speed", "Speed", "Target speed", "Align time", "Align", "Mass",
	"Warp speed", "Warp",
	"Sig radius", "Signature", "Scan resolution", "Scan", "Max target range",
	"Max targets", "Targets", "Warp scramble", "Aggro range", "Orbit range",
	"Flee threshold",
	"Yield", "Per cycle", "Ore hold",
	"Price", "Base price", "Bounty", "Cash", "Receivables", "Deployed in orders",
	"Total", "Cargo",
	"High", "Mid", "Low", "Slot", "Grid class", "CPU class", "Mount", "Size",
	"Volume", "Cycle", "Base time", "Clip", "Loaded", "In cargo",
	"Grid quotas", "CPU quotas", "Mounts",
	"EM", "Electro-magnetic", "EM resist", "Thermal", "Thermal resist",
	"Kinetic", "Kinetic resist", "Explosive", "Explosive resist",
}

// TestStatTaxonomyCoversCensus pins the table to the census: every label has
// a glyph and a family, every glyph is a manifest key, and the identity
// allowlist stays iconless.
func TestStatTaxonomyCoversCensus(t *testing.T) {
	for _, label := range statCensus {
		icon, cat := StatIcon(label), StatCategoryOf(label)
		if icon == "" || cat == StatNone {
			t.Errorf("%q: icon %q, category %v — add it to statStyles", label, icon, cat)
			continue
		}
		if !icons.Has(icon) {
			t.Errorf("%q: glyph %q is not in the icon manifest", label, icon)
		}
	}
	for _, label := range []string{
		"Class", "Maker", "Origin", "Family", "Faction", "Hull", "Kind", "Category", "Qty",
	} {
		if icon := StatIcon(label); icon != "" {
			t.Errorf("identity row %q carries glyph %q", label, icon)
		}
	}
	// An unknown label is never a silent stat.
	if icon, cat := StatIcon("Definitely not a stat"), StatCategoryOf("Definitely not a stat"); icon != "" || cat != StatNone {
		t.Errorf("unknown label resolved to %q/%v", icon, cat)
	}
}

// TestSectionBarGlyphs pins the section-bar map: every section the pages emit
// has a rule (grey for the identity blocks), and every icon is manifest art.
func TestSectionBarGlyphs(t *testing.T) {
	p := DefaultPalette()
	for _, label := range []string{
		"CONSTRUCTION", "FITTING", "SLOTS", "SURVIVAL", "CAPACITOR", "PROPULSION",
		"SENSORS", "CARGO", "MANUFACTURED FROM", "USED IN", "PROPERTIES",
		"ACTIVATION", "DAMAGE", "RANGE", "FIRING", "CHARGE", "YIELD", "REPAIR",
		"EFFECT", "BONUSES", "CHARGES", "MARKET", "REFINES TO", "DOSSIER",
		"BEHAVIOR", "LOOT",
	} {
		icon, tint := SectionBar(p, label)
		if icon != "" && !icons.Has(icon) {
			t.Errorf("%s: glyph %q is not in the icon manifest", label, icon)
		}
		if tint == (color.RGBA{}) {
			t.Errorf("%s: no tint", label)
		}
	}
}
