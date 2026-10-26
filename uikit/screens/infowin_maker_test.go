package screens

import (
	"testing"

	"github.com/entieve/entieve/content"
	"github.com/entieve/entieve/uikit/emblems"
	"github.com/entieve/entieve/uikit/internal/uitest"
	"github.com/entieve/entieve/uikit/widgets"
)

// The manufacturer layer on the Show Info page: a manufactured def's Sub
// line reads "class · maker display name", its CONSTRUCTION block carries
// a Maker row and the header leads with the maker's emblem; the
// unattributed rat hulls keep the old "class · origin"/Origin-row
// treatment with no emblem.

// TestInfoPageMakerShipped pins the shipped maker display for one product
// of each family against the real roster (the JSON assignment can drift
// only with the roster).
func TestInfoPageMakerShipped(t *testing.T) {
	env, _ := newChromeEnv(t)
	cases := []struct {
		name      string
		ref       widgets.InfoRef
		sub       string
		makerName string
	}{
		{"hull", widgets.ShipRef("ship-sloop"), "Frigate · Bůral", "Bůral"},
		{"module", widgets.ModuleRef("mod-autocannon-s"), "Weapon · Noldvar Guard", "Noldvar Guard"},
		{"charge", widgets.ItemRef("ammo-emp-s"), "Ammo · Bríždan Factory", "Bríždan Factory"},
		{"charge-cap-cell", widgets.ItemRef("charge-cap-cell"), "Charge · Iškoven", "Iškoven"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			page, ok := infoPageFor(env, tc.ref)
			if !ok {
				t.Fatalf("%v did not resolve", tc.ref)
			}
			if page.Sub != tc.sub {
				t.Errorf("Sub = %q, want %q", page.Sub, tc.sub)
			}
			maker := mustRow(t, page, "CONSTRUCTION", "Maker")
			if maker.Value != tc.makerName {
				t.Errorf("Maker row = %+v, want %q", maker, tc.makerName)
			}
			if page.Emblem == "" {
				t.Error("no emblem slug on the manufactured page")
			}
			if _, has := rowOf(sectionOf(page, "CONSTRUCTION"), "Origin"); has {
				t.Error("a manufactured page still renders an Origin row")
			}
		})
	}
}

// TestInfoPageMakerRatHullUnchanged pins the rat-hull carve-out: Old Works
// hulls keep their heritage origin line and Origin row, with no maker row
// and no emblem.
func TestInfoPageMakerRatHullUnchanged(t *testing.T) {
	env, _ := newChromeEnv(t)
	page, ok := infoPageFor(env, widgets.ShipRef("ship-rat-scuttler"))
	if !ok {
		t.Fatal("rat hull did not resolve")
	}
	if page.Sub != "Rat · Old works" {
		t.Errorf("rat Sub = %q, want %q", page.Sub, "Rat · Old works")
	}
	if page.Emblem != "" {
		t.Errorf("rat hull carries emblem %q (want none)", page.Emblem)
	}
	if _, has := rowOf(sectionOf(page, "CONSTRUCTION"), "Maker"); has {
		t.Error("rat hull rendered a Maker row")
	}
	if r := mustRow(t, page, "CONSTRUCTION", "Origin"); r.Value != "Old works" {
		t.Errorf("rat Origin row = %+v, want Old works", r)
	}
}

// TestInfoPageUnattributedItemUnchanged pins an ore (no maker, no origin):
// no CONSTRUCTION section, no emblem.
func TestInfoPageUnattributedItemUnchanged(t *testing.T) {
	env, _ := newChromeEnv(t)
	page, ok := infoPageFor(env, widgets.ItemRef("ore-sterngrit"))
	if !ok {
		t.Fatal("ore did not resolve")
	}
	if page.Emblem != "" {
		t.Errorf("ore carries emblem %q (want none)", page.Emblem)
	}
	if s := sectionOf(page, "CONSTRUCTION"); s != nil {
		t.Errorf("ore rendered a CONSTRUCTION block: %+v", s.Rows)
	}
	if page.Sub != "Ore" {
		t.Errorf("ore Sub = %q, want just the category", page.Sub)
	}
}

// TestInfoWindowEmblemRenders pins the draw seam: opening a manufactured
// def's page puts a ≥32 px emblem graphic in the window header; the rat
// hull's does not.
func TestInfoWindowEmblemRenders(t *testing.T) {
	env, _ := newChromeEnv(t)
	w := env.InfoWindow()
	ui := newWindowUI(t, env, w.Win())
	uitest.PumpWindow(t, ui, w.Sync, 1024, 700)

	if !w.Show(widgets.ShipRef("ship-sloop")) {
		t.Fatal("ship-sloop did not resolve")
	}
	g := w.Emblem()
	if g == nil {
		t.Fatal("manufactured hull page has no emblem graphic")
	}
	if img := g.Image; img == nil || img.Bounds().Dx() < 32 {
		t.Errorf("emblem image = %v, want a block of at least 32 px", img)
	}

	if !w.Show(widgets.ShipRef("ship-rat-marauder")) {
		t.Fatal("rat hull did not resolve")
	}
	if w.Emblem() != nil {
		t.Error("rat hull page rendered an emblem graphic")
	}
	// And an unattributed item likewise.
	if !w.Show(widgets.ItemRef("min-ferrite")) {
		t.Fatal("mineral did not resolve")
	}
	if w.Emblem() != nil {
		t.Error("mineral page rendered an emblem graphic")
	}

	// Swapping back brings the emblem with the new page.
	if !w.Show(widgets.ModuleRef("mod-cap-booster")) {
		t.Fatal("module did not resolve")
	}
	if w.Emblem() == nil {
		t.Error("module page lost its emblem after a swap")
	}
}

// TestEveryCorpEmblemShows drives one shipped product per corporation and
// pins that its page names the corporation and carries its emblem — the
// join between the content roster and the emblems package.
func TestEveryCorpEmblemShows(t *testing.T) {
	env, _ := newChromeEnv(t)
	w := env.InfoWindow()
	ui := newWindowUI(t, env, w.Win())
	uitest.PumpWindow(t, ui, w.Sync, 1024, 700)

	// One shipped manufactured def per corp.
	refs := map[string]widgets.InfoRef{
		"corp-borealis":  widgets.ShipRef("ship-sloop"),
		"corp-nordvakt":  widgets.ModuleRef("mod-rocket-pod"),
		"corp-iskald":    widgets.ModuleRef("mod-cap-recharger"),
		"corp-ferrum":    widgets.ShipRef("ship-hauler"),
		"corp-druzstvo":  widgets.ShipRef("ship-barge"),
		"corp-burza":     widgets.ItemRef("ammo-plutonium-s"),
		"corp-celeritas": widgets.ModuleRef("mod-afterburner"),
		"corp-praetor":   widgets.ShipRef("ship-npc-patrol"),
		"corp-aldemar":   widgets.ModuleRef("mod-armor-rep"),
		"corp-vantera":   widgets.ModuleRef("mod-shield-extender"),
	}
	for _, corp := range content.AllCorps {
		ref, ok := refs[corp.ID]
		if !ok {
			t.Fatalf("no fixture product for %s", corp.ID)
		}
		w.Show(ref)
		page, _ := infoPageFor(env, ref)
		if page.Emblem != corp.ID {
			t.Errorf("%s: page emblem = %q, want %q", corp.ID, page.Emblem, corp.ID)
		}
		if got := mustRow(t, page, "CONSTRUCTION", "Maker").Value; got != corp.Name {
			t.Errorf("%s: Maker row = %q, want %q", corp.ID, got, corp.Name)
		}
		if g := w.Emblem(); g == nil || g.Image == nil {
			t.Errorf("%s: no emblem graphic rendered", corp.ID)
		}
	}
	// The emblems the pages ask for actually exist (not a silent fallback).
	for _, corp := range content.AllCorps {
		if emblems.Get(corp.ID, 40) == nil {
			t.Errorf("%s: emblems.Get returned nil", corp.ID)
		}
	}
}
