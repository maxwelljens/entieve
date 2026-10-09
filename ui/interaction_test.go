package ui

import (
	"strings"
	"testing"

	"github.com/entieve/entieve/content"
)

// TestSortIndicator: only the active column shows ▲/▼, direction maps to
// the arrow (ascending = up).
func TestSortIndicator(t *testing.T) {
	if show, up := SortIndicator(SortDistance, false, true); show || up {
		t.Fatalf("inactive column shows indicator: show=%v up=%v", show, up)
	}
	if show, up := SortIndicator(SortName, true, true); !show || !up {
		t.Fatalf("active ascending: show=%v up=%v, want true/true", show, up)
	}
	if show, up := SortIndicator(SortVelocity, true, false); !show || up {
		t.Fatalf("active descending: show=%v up=%v, want true/false", show, up)
	}
}

// TestNumberColumnAnchors: right-aligned number columns use one fixed
// anchor per column — the pixel edge never depends on the value, row or
// scroll position.
func TestNumberColumnAnchors(t *testing.T) {
	for _, size := range designSizes {
		r := ComputeRegions(size[0], size[1]).Overview
		dist := OverviewColumn(r, 2)
		vel := OverviewColumn(r, 3)
		anchor := NumberAnchor(dist)
		if anchor != dist.Right()-padSmall {
			t.Fatalf("%vx%v: NumberAnchor = %v, want %v", size[0], size[1],
				anchor, dist.Right()-padSmall)
		}
		// Recomputation is stable (no dependence on row/scroll/content).
		for i := 0; i < 5; i++ {
			if got := NumberAnchor(OverviewColumn(r, 2)); got != anchor {
				t.Fatalf("%vx%v: anchor drifted: %v != %v", size[0], size[1], got, anchor)
			}
		}
		if NumberAnchor(vel) <= anchor {
			t.Fatalf("%vx%v: velocity anchor must sit right of distance anchor",
				size[0], size[1])
		}
	}
	// Market anchors are fixed fractions of the header, minus pad.
	minus := Rect{X: 800, Y: 100, W: 40, H: 40}
	if got, want := WalletAnchor(minus), QtyAnchorX(minus)-pad-padSmall; got != want {
		t.Fatalf("WalletAnchor = %v, want %v", got, want)
	}
}

// TestWarpRangeCycling: the segmented control choices and the Z-key
// command share one source (WarpRanges); indices normalize with wrap.
func TestWarpRangeCycling(t *testing.T) {
	if len(WarpRanges) != 3 || WarpRanges[0] != 0 || WarpRanges[1] != 10000 ||
		WarpRanges[2] != 50000 {
		t.Fatalf("WarpRanges = %v, want [0 10000 50000]", WarpRanges)
	}
	if got := CycleWarpRange(0); got != 0 {
		t.Fatalf("CycleWarpRange(0) = %d, want 0", got)
	}
	if got := CycleWarpRange(2); got != 2 {
		t.Fatalf("CycleWarpRange(2) = %d, want 2", got)
	}
	if got := CycleWarpRange(3); got != 0 {
		t.Fatalf("CycleWarpRange(3) = %d, want 0 (wrap)", got)
	}
	if got := CycleWarpRange(-1); got != 2 {
		t.Fatalf("CycleWarpRange(-1) = %d, want 2", got)
	}
	if got := CycleWarpRange(5); got != 2 {
		t.Fatalf("CycleWarpRange(5) = %d, want 2", got)
	}
	labels := []string{"Warp to 0 km", "Warp to 10 km", "Warp to 50 km"}
	for i, want := range labels {
		if got := WarpRangeLabel(i); got != want {
			t.Errorf("WarpRangeLabel(%d) = %q, want %q", i, got, want)
		}
	}
	if got := WarpRangeMeters(1); got != 10000 {
		t.Fatalf("WarpRangeMeters(1) = %v, want 10000", got)
	}
}

// TestTooltipDelay: module tooltips appear only after 300 ms of hover.
func TestTooltipDelay(t *testing.T) {
	if TooltipVisible(1.0, 1.29) {
		t.Fatal("tooltip shown before the 300 ms delay")
	}
	if !TooltipVisible(1.0, 1.30) {
		t.Fatal("tooltip not shown at the 300 ms boundary")
	}
	if !TooltipVisible(1.0, 5.0) {
		t.Fatal("tooltip not shown for a long hover")
	}
	if tooltipDelay != 0.3 {
		t.Fatalf("tooltipDelay = %v, want 0.3", tooltipDelay)
	}
}

// TestModuleTipLines: weapon tooltips carry name, slot, damage/optimal/
// tracking and the loaded charge; boosters carry their boost numbers.
func TestModuleTipLines(t *testing.T) {
	turret := &content.ModuleDef{
		ID: "mod-test", Name: "Test Turret", Kind: content.KindWeapon,
		ActivationCost: 12.5, ActivationTime: 4,
		Weapon: &content.WeaponAttrs{
			Damage:    content.Damage{EM: 10, Thermal: 5},
			Optimal:   8000,
			Falloff:   4000,
			Tracking:  0.4,
			AmmoClass: "light",
		},
	}
	lines := ModuleTipLines(turret, "High slot 1", "EMP Charge")
	if lines[0] != "Test Turret" || lines[1] != "High slot 1" {
		t.Fatalf("header lines = %q", lines[:2])
	}
	joined := strings.Join(lines, " | ")
	for _, want := range []string{"Damage 15.0", "8.0 km", "0.40 rad/s",
		"Charge: EMP Charge", "12.5 GJ"} {
		if !strings.Contains(joined, want) {
			t.Errorf("weapon tooltip missing %q:\n%s", want, joined)
		}
	}

	booster := &content.ModuleDef{
		ID: "mod-shield-boost", Name: "Shield Booster", Kind: content.KindShieldBoost,
		ActivationTime: 3,
		Boost:          &content.BoostAttrs{ShieldBoostPerCycle: 45},
	}
	lines = ModuleTipLines(booster, "Mid slot 2", "")
	joined = strings.Join(lines, " | ")
	if !strings.Contains(joined, "Shield +45.0 HP / 3.0 s") {
		t.Errorf("booster tooltip missing boost numbers:\n%s", joined)
	}

	if got := ModuleTipLines(nil, "High slot 1", ""); got != nil {
		t.Fatalf("nil module produced lines: %v", got)
	}
}

// ammoFixture builds minimal content for charge-choice tests.
func ammoFixture() *content.Content {
	return &content.Content{
		Items: map[content.ItemID]*content.ItemDef{
			"charge-emp":   {ID: "charge-emp", Name: "EMP Charge", Category: content.CatAmmo, AmmoClass: "light"},
			"charge-ap":    {ID: "charge-ap", Name: "AP Charge", Category: content.CatAmmo, AmmoClass: "light"},
			"charge-heavy": {ID: "charge-heavy", Name: "Heavy Charge", Category: content.CatAmmo, AmmoClass: "heavy"},
			"min-ferrite":  {ID: "min-ferrite", Name: "Ferrite", Category: content.CatMineral},
		},
	}
}

// TestAmmoCandidates: cycling choices are Auto first plus every distinct
// cargo charge matching the weapon's ammo class (sorted); mismatched class
// or weaponless modules get nothing (cycling disabled).
func TestAmmoCandidates(t *testing.T) {
	c := ammoFixture()
	cargo := []content.CargoStack{
		{Item: "charge-ap", Qty: 50},
		{Item: "min-ferrite", Qty: 100},
		{Item: "charge-emp", Qty: 30},
		{Item: "charge-emp", Qty: 5},   // second stack: deduplicated
		{Item: "charge-heavy", Qty: 0}, // empty stack: skipped
		{Item: "charge-heavy", Qty: 10},
	}
	got := AmmoCandidates(c, cargo, "light")
	want := []content.ItemID{"", "charge-ap", "charge-emp"}
	if len(got) != len(want) {
		t.Fatalf("candidates = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("candidates = %v, want %v", got, want)
		}
	}
	// Heavy class: only the heavy charge (heavy is not in the light list
	// above because its stack is present — recheck directly).
	got = AmmoCandidates(c, cargo, "heavy")
	if len(got) != 2 || got[0] != "" || got[1] != "charge-heavy" {
		t.Fatalf("heavy candidates = %v", got)
	}
	// No cargo for this class: nil (right-click disabled cleanly).
	if got := AmmoCandidates(c, cargo, "light-fast"); got != nil {
		t.Fatalf("unknown class candidates = %v, want nil", got)
	}
	// Weapons without an ammo class never cycle.
	if got := AmmoCandidates(c, cargo, ""); got != nil {
		t.Fatalf("empty ammo class candidates = %v, want nil", got)
	}
	// No matching cargo at all: nil.
	if got := AmmoCandidates(c, nil, "light"); got != nil {
		t.Fatalf("empty cargo candidates = %v, want nil", got)
	}
}

// TestNextAmmoChoiceCycle: right-click advances Auto → charge → … → Auto
// with wraparound.
func TestNextAmmoChoiceCycle(t *testing.T) {
	cands := []content.ItemID{"", "charge-ap", "charge-emp"}
	if got := NextAmmoChoice("", cands); got != "charge-ap" {
		t.Fatalf("auto → %q, want charge-ap", got)
	}
	if got := NextAmmoChoice("charge-ap", cands); got != "charge-emp" {
		t.Fatalf("charge-ap → %q, want charge-emp", got)
	}
	if got := NextAmmoChoice("charge-emp", cands); got != "" {
		t.Fatalf("last → %q, want Auto (wrap)", got)
	}
	// Unknown current choice restarts at Auto.
	if got := NextAmmoChoice("nope", cands); got != "" {
		t.Fatalf("unknown → %q, want Auto", got)
	}
	// No candidates: unchanged.
	if got := NextAmmoChoice("charge-ap", nil); got != "charge-ap" {
		t.Fatalf("nil candidates → %q, want unchanged", got)
	}
	// Display names for tooltips.
	c := ammoFixture()
	if n := AmmoChoiceName(c, ""); n != "Auto" {
		t.Fatalf("Auto label = %q", n)
	}
	if n := AmmoChoiceName(c, "charge-emp"); n != "EMP Charge" {
		t.Fatalf("charge label = %q", n)
	}
}
