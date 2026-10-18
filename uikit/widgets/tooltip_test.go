package widgets

import (
	"fmt"
	"github.com/entieve/entieve/uikit/theme"
	"image"
	"testing"
	"time"

	"github.com/ebitenui/ebitenui/widget"
	"github.com/entieve/entieve/content"
)

// tipTestEnv is a Theme with just what the tooltip maths needs (palette
// and faces): the layout never touches the view or the sink — the seam
// purity the tooltip-only widget proves.
func tipTestEnv() theme.Theme {
	return theme.NewSeam(theme.DefaultPalette(), theme.DefaultTypeScale())
}

// tipFixtureModel is the layout fixture: a header, two sections of rows and
// a hint — the anatomy in miniature, independent of any content file.
func tipFixtureModel() TooltipModel {
	return TooltipModel{
		Icon:  "mod_weapon",
		Title: "EMP Turret II",
		Sub:   "High slot 1 · Weapon",
		Sections: []TooltipSection{
			{Label: "DAMAGE", Rows: []TooltipRow{
				{Label: "Electro-magnetic", Value: "16.0"},
				{Label: "Thermal", Value: "5.0"},
			}},
			{Label: "CHARGE", Rows: []TooltipRow{
				{Label: "Loaded", Value: "Fusion L"},
				{Label: "In cargo", Value: "600"},
			}},
		},
		Hints: []string{"Right-click: cycle ammo"},
	}
}

// checkTipLayout pins the layout contract on one panel: every rect inside
// the content box, the box itself centred in the panel, rows tiling their
// section with no gaps, label/value rects disjoint, and every value ending
// on the single shared anchor.
func checkTipLayout(t *testing.T, l tipLayout) {
	t.Helper()

	// The content box is the panel inset by the padding, and the value
	// anchor is its right edge — one x for every row in the panel.
	wantInner := image.Rect(TipPadX, tipPadY, l.Size.X-TipPadX, l.Size.Y-tipPadY)
	if l.Inner != wantInner {
		t.Errorf("Inner = %v, want %v", l.Inner, wantInner)
	}
	if l.ValueX != l.Inner.Max.X {
		t.Errorf("ValueX = %d, want the content right edge %d", l.ValueX, l.Inner.Max.X)
	}

	inside := func(name string, r image.Rectangle) {
		if r.Empty() {
			return // absent element (no icon, no sub, no rules)
		}
		if !r.In(l.Inner) {
			t.Errorf("%s = %v escapes the content box %v", name, r, l.Inner)
		}
	}
	inside("Icon", l.Icon)
	inside("Title", l.Title)
	inside("Sub", l.Sub)
	for i, r := range l.Rules {
		inside("Rule", r)
		if r.Dy() != TipRuleH {
			t.Errorf("Rule[%d] height %d, want %d", i, r.Dy(), TipRuleH)
		}
		if r.Min.X != l.Inner.Min.X || r.Max.X != l.Inner.Max.X {
			t.Errorf("Rule[%d] = %v, want full content width", i, r)
		}
	}
	for _, r := range l.Hints {
		inside("Hint", r)
	}
	for si, sl := range l.Sections {
		inside("Section label", sl.Label)
		if sl.Label.Min.X != l.Inner.Min.X || sl.Label.Max.X != l.Inner.Max.X {
			t.Errorf("section %d label = %v, want full content width", si, sl.Label)
		}
		prev := sl.Label
		for ri, row := range sl.Rows {
			inside("row label", row.Label)
			inside("row value", row.Value)
			// Rows tile exactly: label line over value line, row under row.
			if row.Label.Min.Y != row.Value.Min.Y || row.Label.Max.Y != row.Value.Max.Y {
				t.Errorf("section %d row %d: label %v / value %v differ in y", si, ri, row.Label, row.Value)
			}
			if row.Label.Min.Y != prev.Max.Y {
				t.Errorf("section %d row %d starts at %d, want %d (previous end)",
					si, ri, row.Label.Min.Y, prev.Max.Y)
			}
			// Disjoint with the row gutter: the label can never touch the value.
			if row.Label.Max.X+TipRowGap > row.Value.Min.X && !row.Label.Empty() {
				t.Errorf("section %d row %d: label %v / value %v leave less than %d px",
					si, ri, row.Label, row.Value, TipRowGap)
			}
			if row.Value.Max.X != l.ValueX {
				t.Errorf("section %d row %d value ends at %d, want the shared anchor %d",
					si, ri, row.Value.Max.X, l.ValueX)
			}
			prev = row.Value
		}
		if si > 0 {
			if want := l.Sections[si-1].Rows[len(l.Sections[si-1].Rows)-1].Value.Max.Y + tipSecGap; sl.Label.Min.Y != want {
				t.Errorf("section %d label starts at %d, want %d (previous section end + gap)",
					si, sl.Label.Min.Y, want)
			}
		}
	}
}

// TestTipLayoutTilesAtBothWidths lays the fixture out at the 320 px cap and
// at its content-sized width, pins the hand-computable x geometry (padding,
// icon column, title indent, shared anchor) and runs the tiling contract on
// both.
func TestTipLayoutTilesAtBothWidths(t *testing.T) {
	env := tipTestEnv()
	m := tipFixtureModel()

	// Content-sized: the widest natural line, capped at the 304 px box.
	natural := tipInnerWidth(env, m)
	if natural >= tipMaxInner {
		t.Fatalf("fixture wants %d px, should fit under the %d px cap", natural, tipMaxInner)
	}
	minW := tipLayoutAt(env, m, natural)
	if got := TipSize(env, m); got != minW.Size {
		t.Errorf("tipSize = %v, want %v (the content-sized layout)", got, minW.Size)
	}
	// Capped: what the panel does when the content wants more than 320.
	maxed := tipLayoutAt(env, m, tipMaxInner)
	if maxed.Size.X != tipMaxW {
		t.Errorf("capped panel width = %d, want %d", maxed.Size.X, tipMaxW)
	}

	for _, tc := range []struct {
		name string
		l    tipLayout
		iw   int
	}{
		{"320 cap", maxed, tipMaxInner},
		{"content minimum", minW, natural},
	} {
		l := tc.l
		if l.Size.X != tc.iw+2*TipPadX {
			t.Errorf("%s: panel width %d, want inner %d + %d padding",
				tc.name, l.Size.X, tc.iw, 2*TipPadX)
		}
		checkTipLayout(t, l)

		// Hand-computable x geometry: the icon column leads, the title and
		// sub hang right of it, every rule and label spans the content box.
		if l.Icon != (image.Rect(TipPadX, l.Icon.Min.Y, TipPadX+TipIcon, l.Icon.Max.Y)) {
			t.Errorf("%s: icon column = %v, want x %d..%d", tc.name, l.Icon, TipPadX, TipPadX+TipIcon)
		}
		if l.Icon.Dy() != TipIcon {
			t.Errorf("%s: icon = %v, want %d px square", tc.name, l.Icon, TipIcon)
		}
		wantTitleX := TipPadX + TipIcon + tipIconGap
		if l.Title.Min.X != wantTitleX || l.Sub.Min.X != wantTitleX {
			t.Errorf("%s: title/sub x = %d/%d, want %d", tc.name, l.Title.Min.X, l.Sub.Min.Y, wantTitleX)
		}
		if l.Sub.Min.Y != l.Title.Max.Y {
			t.Errorf("%s: sub starts at %d, want the title line's end %d", tc.name, l.Sub.Min.Y, l.Title.Max.Y)
		}
		// The hairline sits tipRuleGap under the header block, and the
		// first section label tipRuleGap under the rule.
		if len(l.Rules) != 2 || len(l.Sections) != 2 || len(l.Hints) != 1 {
			t.Fatalf("%s: got %d rules / %d sections / %d hints, want 2/2/1",
				tc.name, len(l.Rules), len(l.Sections), len(l.Hints))
		}
		if want := l.Sub.Max.Y + tipRuleGap; l.Rules[0].Min.Y != want {
			t.Errorf("%s: first rule at %d, want %d (header end + %d)",
				tc.name, l.Rules[0].Min.Y, want, tipRuleGap)
		}
		if want := l.Rules[0].Max.Y + tipRuleGap; l.Sections[0].Label.Min.Y != want {
			t.Errorf("%s: first section label at %d, want %d (rule end + %d)",
				tc.name, l.Sections[0].Label.Min.Y, want, tipRuleGap)
		}
		// The footer rule follows the last section, the hint follows that.
		if want := l.Sections[1].Rows[len(l.Sections[1].Rows)-1].Value.Max.Y + tipRuleGap; l.Rules[1].Min.Y != want {
			t.Errorf("%s: footer rule at %d, want %d", tc.name, l.Rules[1].Min.Y, want)
		}
		if want := l.Rules[1].Max.Y + tipRuleGap; l.Hints[0].Min.Y != want {
			t.Errorf("%s: hint at %d, want %d", tc.name, l.Hints[0].Min.Y, want)
		}
	}

	// Regression pin: the fixture's panel size at both widths. The height
	// is width-independent (the same lines, stacked), so a change in either
	// number is a change to the shipped tooltip's geometry.
	if got, want := maxed.Size, (image.Point{X: tipMaxW, Y: 231}); got != want {
		t.Errorf("capped fixture panel = %v, want %v", got, want)
	}
	if got, want := minW.Size, (image.Point{X: 190, Y: 231}); got != want {
		t.Errorf("content-sized fixture panel = %v, want %v", got, want)
	}
	if minW.Size.X >= maxed.Size.X {
		t.Errorf("content-sized width %d should be narrower than the cap %d", minW.Size.X, maxed.Size.X)
	}
}

// TestTipNaturalWidthMatchesLayout pins the width rule: the content box is
// the widest natural line (label + gutter + value included), capped at the
// panel's maximum — and the layout at that width agrees with tipSize.
func TestTipNaturalWidthMatchesLayout(t *testing.T) {
	env := tipTestEnv()
	for _, m := range []TooltipModel{
		tipFixtureModel(),
		{Title: "Warp unavailable", Sub: "Target within warp minimum range (150 km)"},
		ModuleTip(&content.ModuleDef{
			Name: "EMP Turret", Kind: content.KindWeapon, Slot: content.SlotHigh,
			ActivationCost: 2, ActivationTime: 3,
			Weapon: &content.WeaponAttrs{
				Damage:    content.Damage{EM: 16, Thermal: 5, Kinetic: 2, Explosive: 1},
				Optimal:   12000,
				Falloff:   6000,
				Tracking:  145,
				AmmoClass: "light",
			},
		}, "High slot 1", "Fusion L", 600, "Right-click: cycle ammo", "Click: activate"),
	} {
		iw := tipInnerWidth(env, m)
		l := tipLayoutAt(env, m, iw)
		if l.Size != TipSize(env, m) {
			t.Errorf("size mismatch at the natural width for %q: %v vs %v",
				m.Title, l.Size, TipSize(env, m))
		}
		// A model that fits needs no ellipsis: every measured line is
		// inside the box the layout was given.
		inner := float64(iw)
		if w := TipMeasure(m.Title, env.FaceBold(theme.StepBase)); w+(TipIcon+tipIconGap) > inner {
			t.Errorf("%q: title wants %.1f px of the %d px box", m.Title, w+TipIcon+tipIconGap, iw)
		}
		for _, sec := range m.Sections {
			for _, row := range sec.Rows {
				w := TipMeasure(row.Label, env.Face(theme.StepSmall)) + TipRowGap +
					TipMeasure(row.Value, env.Face(theme.StepSmall))
				if w > inner {
					t.Errorf("%q/%q wants %.1f px of the %d px box", sec.Label, row.Label, w, iw)
				}
			}
		}
	}
}

// TestTipFits320px is the shipped fit: every module in the content registry
// lays out at or under the 320 px cap, so no tooltip row ever ellipsizes a
// value in the real game.
func TestTipFits320px(t *testing.T) {
	env, _ := newChromeEnv(t)
	if len(env.Content.Modules) == 0 {
		t.Fatal("content has no modules")
	}
	widest, widestID := 0, ""
	for id, def := range env.Content.Modules {
		// The widest case a rack cell can show: charge + both hints.
		charge, qty := "", 0
		if def.Weapon != nil && def.Weapon.AmmoClass != "" {
			charge, qty = "Prograde XL", 12345
		}
		m := ModuleTip(def, "High slot 1", charge, qty,
			"Right-click: cycle ammo", "Click: activate")
		if w := TipSize(env, m).X; w > tipMaxW {
			t.Errorf("%s (%s): tooltip %d px wide, cap is %d", id, def.Name, w, tipMaxW)
		} else if w > widest {
			widest, widestID = w, string(id)
		}
	}
	t.Logf("widest shipped tooltip: %s at %d px (cap %d)", widestID, widest, tipMaxW)
}

// TestTipPlacementClamp pins the placement rule at every screen edge: the
// cursor-sticky anchor is cursor + the 12 px offset with the panel trailing
// left and below, so each edge hands the clamp a rect that starts (or ends)
// off-screen — and each one must come back fully inside.
func TestTipPlacementClamp(t *testing.T) {
	screen := image.Rect(0, 0, 1280, 720)
	size := image.Point{X: 300, Y: 180}
	anchored := func(cursor image.Point) image.Rectangle {
		// The panel's top-left as the cursor-sticky tooltip computes it:
		// cursor + offset, then the content anchored to the end/start.
		return image.Rect(cursor.X+12-size.X, cursor.Y+12, cursor.X+12, cursor.Y+12+size.Y)
	}
	for _, tc := range []struct {
		name   string
		cursor image.Point
	}{
		{"top-left", image.Point{X: 0, Y: 0}},
		{"top-right", image.Point{X: screen.Max.X, Y: 0}},
		{"bottom-left", image.Point{X: 0, Y: screen.Max.Y}},
		{"bottom-right", image.Point{X: screen.Max.X, Y: screen.Max.Y}},
		{"left edge", image.Point{X: 0, Y: 360}},
		{"right edge", image.Point{X: screen.Max.X, Y: 360}},
		{"top edge", image.Point{X: 640, Y: 0}},
		{"bottom edge", image.Point{X: 640, Y: screen.Max.Y}},
	} {
		raw := anchored(tc.cursor)
		got := ClampTipRect(raw, screen)
		if !got.In(screen) {
			t.Errorf("%s: cursor %v puts the panel at %v → %v, outside %v",
				tc.name, tc.cursor, raw, got, screen)
		}
		if got.Size() != size {
			t.Errorf("%s: panel size changed to %v, want %v", tc.name, got.Size(), size)
		}
	}

	// Mid-screen the clamp is a no-op: a tooltip that already fits never
	// moves (no shimmer while the cursor travels).
	mid := anchored(image.Point{X: 640, Y: 360})
	if got := ClampTipRect(mid, screen); got != mid {
		t.Errorf("mid-screen clamp moved %v to %v, want it untouched", mid, got)
	}

	// A panel too big for the screen pins to the origin and clips at the
	// far edge rather than drifting off the near one.
	big := image.Rect(-40, -40, 1400, 900)
	if got := ClampTipRect(big, screen); got.Min != screen.Min {
		t.Errorf("oversized panel pinned to %v, want the screen origin", got.Min)
	}
}

// TestTipClampAxisIsMonotonic covers the axis helper directly: clamped at
// the low edge, slid back at the high edge, pinned when it cannot fit.
func TestTipClampAxisIsMonotonic(t *testing.T) {
	for _, tc := range []struct{ pos, size, lo, hi, want int }{
		{0, 10, 0, 100, 0},   // inside, untouched
		{90, 10, 0, 100, 90}, // flush right, untouched
		{-5, 10, 0, 100, 0},  // before the low edge
		{95, 10, 0, 100, 90}, // past the high edge
		{-5, 200, 0, 100, 0}, // too big: pinned to the origin
		{50, 100, 0, 100, 0}, // exactly as big as the screen
		{5, 0, 0, 100, 5},    // empty: harmless
	} {
		if got := clampAxis(tc.pos, tc.size, tc.lo, tc.hi); got != tc.want {
			t.Errorf("clampAxis(%d, %d, %d, %d) = %d, want %d",
				tc.pos, tc.size, tc.lo, tc.hi, got, tc.want)
		}
	}
}

// checkSections compares a model's sections (label, rows and value tints)
// against the expected ones, reporting the first difference.
func checkSections(t *testing.T, name string, got, want []TooltipSection) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("%s sections = %v, want %v", name, tipSectionLabels(TooltipModel{Sections: got}), tipSectionLabels(TooltipModel{Sections: want}))
	}
	for i := range want {
		if got[i].Label != want[i].Label {
			t.Errorf("%s section %d = %q, want %q", name, i, got[i].Label, want[i].Label)
		}
		if len(got[i].Rows) != len(want[i].Rows) {
			t.Fatalf("%s section %q has %d rows, want %d (%+v)", name, want[i].Label,
				len(got[i].Rows), len(want[i].Rows), got[i].Rows)
		}
		for j := range want[i].Rows {
			if got[i].Rows[j] != want[i].Rows[j] {
				t.Errorf("%s %s row %d = %+v, want %+v", name, want[i].Label, j, got[i].Rows[j], want[i].Rows[j])
			}
		}
	}
}

// TestModuleTipCarriesModuleDef is the content contract: every row in the
// panel comes from the ModuleDef, in the sections of the approved anatomy —
// header (icon/title/sub), DAMAGE/RANGE/FIRING, the charge block, the
// activation block — and the footer hints are the caller's.
func TestModuleTipCarriesModuleDef(t *testing.T) {
	weapon := &content.ModuleDef{
		ID: "mod-emp-turret", Name: "EMP Turret", Slot: content.SlotHigh, Kind: content.KindWeapon,
		ActivationCost: 2, ActivationTime: 3,
		Weapon: &content.WeaponAttrs{
			Damage:    content.Damage{EM: 16, Thermal: 5, Kinetic: 2, Explosive: 1},
			Optimal:   12000,
			Falloff:   6000,
			Tracking:  145,
			AmmoClass: "light",
			ShotTime:  3,
		},
	}
	m := ModuleTip(weapon, "High slot 1", "Fusion L", 600, "Right-click: cycle ammo", "Click: activate")
	if m.Icon != "mod_weapon" || m.Title != "EMP Turret" || m.Sub != "High slot 1 · Weapon" {
		t.Errorf("header = %q/%q/%q, want mod_weapon / EMP Turret / High slot 1 · Weapon",
			m.Icon, m.Title, m.Sub)
	}
	checkSections(t, "weapon", m.Sections, []TooltipSection{
		{Label: "DAMAGE", Rows: []TooltipRow{
			{Label: "Per shot", Value: "24.0"},
			{Label: "Electro-magnetic", Value: "16.0"},
			{Label: "Thermal", Value: "5.0"},
			{Label: "Kinetic", Value: "2.0"},
			{Label: "Explosive", Value: "1.0"},
		}},
		{Label: "RANGE", Rows: []TooltipRow{
			{Label: "Optimal", Value: "12.0 km"},
			{Label: "Falloff", Value: "6.0 km"},
		}},
		{Label: "FIRING", Rows: []TooltipRow{
			{Label: "Tracking", Value: "145.00 rad/s"},
		}},
		{Label: "CHARGE", Rows: []TooltipRow{
			{Label: "Loaded", Value: "Fusion L"},
			{Label: "In cargo", Value: "600"},
		}},
		{Label: "ACTIVATION", Rows: []TooltipRow{
			{Label: "Cycle", Value: "3.0 s"},
			{Label: "Cap use", Value: "2.0 GJ"},
		}},
	})
	if len(m.Hints) != 2 || m.Hints[0] != "Right-click: cycle ammo" || m.Hints[1] != "Click: activate" {
		t.Errorf("hints = %q, want both footers", m.Hints)
	}

	// No count row when the charge is Auto (there is no single charge to
	// count) — but the charge block itself stays.
	m = ModuleTip(weapon, "High slot 1", "Auto", 0)
	if sec := tipSection(m, "CHARGE"); sec == nil || len(sec.Rows) != 1 || sec.Rows[0].Value != "Auto" {
		t.Errorf("Auto charge block = %+v, want Loaded Auto and no count", sec)
	}

	// An ammo-less weapon omits the CHARGE section entirely, whatever the
	// caller passes in — absent data is never shown as a placeholder.
	plain := *weapon
	plain.Weapon = &content.WeaponAttrs{
		Damage:   weapon.Weapon.Damage,
		Optimal:  weapon.Weapon.Optimal,
		Falloff:  weapon.Weapon.Falloff,
		Tracking: weapon.Weapon.Tracking,
	}
	m = ModuleTip(&plain, "High slot 3", "Fusion L", 600)
	if tipSection(m, "CHARGE") != nil {
		t.Errorf("ammo-less weapon has sections %v, want no CHARGE", tipSectionLabels(m))
	}
}

// TestModuleTipOmitsAbsentData pins the omission rule: a section exists
// only when the def carries the data behind it (no "N/A" rows), and a nil
// def produces an empty panel rather than a panic.
func TestModuleTipOmitsAbsentData(t *testing.T) {
	// Mining: yield and activation, no weapon blocks, no charge.
	miner := &content.ModuleDef{
		Name: "Mining Laser", Kind: content.KindMiningLaser, ActivationCost: 20, ActivationTime: 30,
		Mining: &content.MiningAttrs{VolumePerCycle: 100, CycleTime: 30},
	}
	m := ModuleTip(miner, "High slot 2", "", 0)
	checkSections(t, "miner", m.Sections, []TooltipSection{
		{Label: "YIELD", Rows: []TooltipRow{
			{Label: "Per cycle", Value: "100.0 m³"},
		}},
		{Label: "ACTIVATION", Rows: []TooltipRow{
			{Label: "Cycle", Value: "30.0 s"},
			{Label: "Cap use", Value: "20.0 GJ"},
		}},
	})

	// Passive module: bonuses only — no activation block to draw.
	extender := &content.ModuleDef{
		Name: "Shield Extender", Kind: content.KindShieldExtender,
		Passive: &content.PassiveAttrs{ShieldHP: 280},
	}
	m = ModuleTip(extender, "Mid slot 1", "", 0)
	checkSections(t, "extender", m.Sections, []TooltipSection{
		{Label: "BONUSES", Rows: []TooltipRow{
			{Label: "Shield", Value: "+280 HP", Tint: goodTint},
		}},
	})

	// A resist coat contributes one row per non-zero resist, all tinted.
	coat := &content.ModuleDef{
		Name: "Resist Coat", Kind: content.KindResistCoat,
		Passive: &content.PassiveAttrs{ResistBonus: content.Resists{EM: .15, Thermal: .15, Kinetic: .15, Explosive: .15}},
	}
	m = ModuleTip(coat, "Low slot 3", "", 0)
	if rows := tipSection(m, "BONUSES"); rows == nil || len(rows.Rows) != 4 {
		t.Fatalf("resist rows = %+v, want four", rows)
	} else if rows.Rows[0] != (TooltipRow{Label: "EM resist", Value: "+15%", Tint: goodTint}) {
		t.Errorf("first resist row = %+v, want +15%% tinted good", rows.Rows[0])
	}
	if tipSection(m, "ACTIVATION") != nil {
		t.Error("passive module carries an ACTIVATION section")
	}

	// Nothing at all: an empty panel, not a crash.
	if got := ModuleTip(nil, "High slot 1", "Auto", 1, "hint"); got.Title != "" || len(got.Sections) != 0 {
		t.Errorf("moduleTip(nil) = %+v, want an empty model", got)
	}
}

// TestNewTipKeepsHoverContract pins the behaviour the plain-text tooltip
// shipped with: the shared 300 ms delay, the cursor-sticky position and the
// 12 px offset — the structured panel changes what is drawn, not when.
func TestNewTipKeepsHoverContract(t *testing.T) {
	env := tipTestEnv()
	tip, view := NewTip(env, tipFixtureModel())
	if rackTipDelay != 300*time.Millisecond {
		t.Fatalf("rackTipDelay = %v, want 300 ms", rackTipDelay)
	}
	if tip.Delay != rackTipDelay {
		t.Errorf("tooltip delay = %v, want the shared %v", tip.Delay, rackTipDelay)
	}
	if tip.Position != widget.TOOLTIP_POS_CURSOR_STICKY {
		t.Errorf("tooltip position = %v, want cursor-sticky", tip.Position)
	}
	if tip.Offset != (image.Point{X: 12, Y: 12}) {
		t.Errorf("tooltip offset = %v, want (12,12)", tip.Offset)
	}
	if view == nil || view.Model().Title == "" {
		t.Fatal("newTip returned no populated view")
	}
}

// TestTipPanelDrawSmoke mounts a real tooltip panel — the content container
// a ToolTip shows, without the hover state machine — in an interface root
// and draws it at both design sizes. The widget geometry must be the layout
// the tests pin: the drawn panel and the pure maths are the same rects, and
// an anchored panel never leaves the window.
func TestTipPanelDrawSmoke(t *testing.T) {
	env, _ := newChromeEnv(t)
	m := tipFixtureModel()
	body, view := NewTipContent(env, m)
	body.GetWidget().LayoutData = widget.AnchorLayoutData{
		HorizontalPosition: widget.AnchorLayoutPositionEnd,
		VerticalPosition:   widget.AnchorLayoutPositionEnd,
		Padding:            &widget.Insets{Right: 10, Bottom: 10},
	}
	c := newChromeUI(env)
	c.root.AddChild(body)
	c.Frame(t, 1280, 720)

	size := TipSize(env, m)
	if got, want := body.GetWidget().Rect.Size(), size; got != want {
		t.Fatalf("panel rect = %v, want the model's size %v", got, want)
	}
	// The view stretches across its container, so its rect IS the panel the
	// layout coordinates are relative to.
	vr := view.GetWidget().Rect
	if vr.Size() != size {
		t.Errorf("view rect = %v, want %v", vr, size)
	}
	l := view.Panel(tipInnerWidth(env, m))
	// The layout's rects are panel-relative; drawTooltip paints them offset
	// by the view's origin, so translated they must sit inside the view.
	inPanel := func(name string, r image.Rectangle) {
		if r.Empty() {
			return
		}
		if got := r.Add(vr.Min); !got.In(vr) {
			t.Errorf("%s %v escapes the drawn panel %v", name, got, vr)
		}
	}
	inPanel("icon", l.Icon)
	inPanel("title", l.Title)
	inPanel("sub", l.Sub)
	for i := range l.Sections {
		inPanel("section label", l.Sections[i].Label)
		for j := range l.Sections[i].Rows {
			inPanel("row label", l.Sections[i].Rows[j].Label)
			inPanel("row value", l.Sections[i].Rows[j].Value)
		}
	}
	for i, r := range l.Rules {
		inPanel(fmt.Sprintf("rule %d", i), r)
	}
	for i, r := range l.Hints {
		inPanel(fmt.Sprintf("hint %d", i), r)
	}
	// The clamp is a no-op here: an on-screen panel never moves.
	if got := ClampTipRect(vr, image.Rect(0, 0, 1280, 720)); got != vr {
		t.Errorf("clamp moved the on-screen panel from %v to %v", vr, got)
	}
	// Bottom-right anchored: fully inside the window at the design sizes.
	for _, wh := range [][2]int{{1280, 720}, {1024, 700}} {
		c.Frame(t, wh[0], wh[1])
		r := view.GetWidget().Rect
		if r.Min.X < 0 || r.Min.Y < 0 || r.Max.X > wh[0] || r.Max.Y > wh[1] {
			t.Errorf("%dx%d: panel %v out of bounds", wh[0], wh[1], r)
		}
	}
}
