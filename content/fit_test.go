package content

import (
	"strconv"
	"strings"
	"testing"
)

// ---------------------------------------------------------------------------
// Fitting limits: high-slot mounts (kind + size) and grid/cpu class quotas.
// The tables below are the binding design: a hull's mount sizes and quotas
// are its identity, not a function of its raw size. Mutation-checked like the
// naming registry — a content edit that drifts from the table fails here.
// ---------------------------------------------------------------------------

// wantMounts formats a hull's mount list for the pin ("turret s, launcher s").
func wantMounts(ms []Mount) string {
	out := make([]string, len(ms))
	for i, m := range ms {
		out[i] = m.Kind + " " + m.Size
	}
	return strings.Join(out, ", ")
}

// wantQuota formats a quota as "i:3 ii:2" (empty for an all-zero quota).
func wantQuota(q ClassQuota) string {
	var parts []string
	for _, c := range []struct {
		class string
		n     int
	}{{ClassI, q.I}, {ClassII, q.II}, {ClassIII, q.III}} {
		if c.n > 0 {
			parts = append(parts, c.class+":"+strconv.Itoa(c.n))
		}
	}
	return strings.Join(parts, " ")
}

var hullFittingTable = []struct {
	id     ShipID
	mounts string
	grid   ClassQuota
	cpu    ClassQuota
}{
	{"ship-sloop", "turret s, turret s, launcher s", ClassQuota{I: 5, II: 3}, ClassQuota{II: 2}},
	{"ship-cutlass", "turret s, turret s, turret s, turret s", ClassQuota{I: 6, II: 3}, ClassQuota{I: 2, II: 1}},
	{"ship-barge", "turret s, turret s", ClassQuota{I: 4, II: 1}, ClassQuota{II: 2}},
	{"ship-hauler", "turret s", ClassQuota{I: 2, II: 1}, ClassQuota{I: 1}},
	{"ship-npc-hauler", "turret s", ClassQuota{I: 2, II: 1}, ClassQuota{I: 1}},
	{"ship-npc-miner", "turret s", ClassQuota{I: 1}, ClassQuota{II: 1}},
	{"ship-npc-patrol", "turret s, turret s, turret s", ClassQuota{I: 3, II: 2}, ClassQuota{I: 1}},
	{"ship-npc-fleet", "turret m, turret m, turret m", ClassQuota{I: 3, II: 4}, ClassQuota{II: 1}},
	{"ship-rat-scuttler", "turret s, turret s", ClassQuota{I: 3, II: 1}, ClassQuota{}},
	{"ship-rat-marauder", "turret m, turret m, turret m", ClassQuota{I: 3, II: 4}, ClassQuota{II: 1}},
	{"ship-rat-hierarch", "turret l, turret l, turret l, turret l", ClassQuota{I: 5, II: 1, III: 4}, ClassQuota{II: 1}},
}

// TestShippedHullMountTable pins every hull's high mounts and class quotas.
func TestShippedHullMountTable(t *testing.T) {
	c := MustLoad()
	if len(c.Ships) != len(hullFittingTable) {
		t.Fatalf("%d shipped hulls, pin has %d", len(c.Ships), len(hullFittingTable))
	}
	for _, want := range hullFittingTable {
		s, ok := c.Ships[want.id]
		if !ok {
			t.Errorf("pinned hull %s is not shipped", want.id)
			continue
		}
		if got := wantMounts(s.Mounts); got != want.mounts {
			t.Errorf("%s mounts %q, want %q", want.id, got, want.mounts)
		}
		if len(s.Mounts) != s.Slots.High {
			t.Errorf("%s: %d mounts for %d high slots", want.id, len(s.Mounts), s.Slots.High)
		}
		for _, q := range []struct {
			axis string
			got  ClassQuota
			want ClassQuota
		}{{RuleGrid, s.GridClasses, want.grid}, {RuleCPU, s.CPUClasses, want.cpu}} {
			if q.got != q.want {
				t.Errorf("%s %s quotas %q, want %q", want.id, q.axis, wantQuota(q.got), wantQuota(q.want))
			}
		}
	}
}

var moduleDemandTable = []struct {
	id    ModuleID
	mount string
	size  string
	grid  string
	cpu   string
}{
	{"mod-autocannon-s", MountTurret, SizeS, ClassI, ""},
	{"mod-autocannon-m", MountTurret, SizeM, ClassII, ""},
	{"mod-autocannon-l", MountTurret, SizeL, ClassIII, ""},
	{"mod-artillery-s", MountTurret, SizeS, ClassI, ""},
	{"mod-artillery-m", MountTurret, SizeM, ClassII, ""},
	{"mod-artillery-l", MountTurret, SizeL, ClassIII, ""},
	{"mod-rocket-pod", MountLauncher, SizeS, ClassI, ""},
	{"mod-mining-laser", MountTurret, SizeS, ClassI, ClassII},
	{"mod-shield-boost", "", "", ClassII, ""},
	{"mod-shield-extender", "", "", ClassI, ""},
	{"mod-armor-rep", "", "", ClassII, ""},
	{"mod-armor-plate", "", "", ClassI, ""},
	{"mod-resist-coat", "", "", ClassI, ""},
	{"mod-cap-booster", "", "", ClassI, ""},
	{"mod-cap-recharger", "", "", ClassI, ""},
	{"mod-afterburner", "", "", ClassII, ""},
	{"mod-stasis-web", "", "", ClassI, ClassII},
	{"mod-warp-scram", "", "", ClassI, ClassII},
}

// TestShippedModuleDemandTable pins every module's mount kind/size and its
// grid/cpu class demand.
func TestShippedModuleDemandTable(t *testing.T) {
	c := MustLoad()
	if len(c.Modules) != len(moduleDemandTable) {
		t.Fatalf("%d shipped modules, pin has %d", len(c.Modules), len(moduleDemandTable))
	}
	for _, want := range moduleDemandTable {
		m, ok := c.Modules[want.id]
		if !ok {
			t.Errorf("pinned module %s is not shipped", want.id)
			continue
		}
		if m.Mount != want.mount || m.Size != want.size {
			t.Errorf("%s mount %q/%q, want %q/%q", want.id, m.Mount, m.Size, want.mount, want.size)
		}
		if m.GridClass != want.grid || m.CPUClass != want.cpu {
			t.Errorf("%s classes grid %q cpu %q, want %q/%q", want.id, m.GridClass, m.CPUClass, want.grid, want.cpu)
		}
		if m.GridClass == "" && m.CPUClass == "" {
			t.Errorf("%s demands neither class: it could be stacked without bound", want.id)
		}
	}
}

// TestAuthoredFitsRespectLimits walks every authored fit (NPC fits and world
// starts) module by module and names the first rule a fit would break.
func TestAuthoredFitsRespectLimits(t *testing.T) {
	c := MustLoad()
	check := func(ctx string, fit []ModuleID, hull *ShipDef) {
		var fitted []*ModuleDef
		for i, mid := range fit {
			m, ok := c.Modules[mid]
			if !ok {
				t.Errorf("%s[%d]: unknown module %q", ctx, i, mid)
				continue
			}
			if issue := hull.FitProblem(m, fitted); issue != nil {
				t.Errorf("%s[%d]: %s breaks the %s rule (%s) on %s", ctx, i, mid, issue.Rule, issue.Kind, hull.ID)
			}
			fitted = append(fitted, m)
		}
	}
	for _, id := range keys(c.NPCs) {
		n := c.NPCs[id]
		if hull, ok := c.Ships[n.Ship]; ok {
			check("npcs/"+string(id)+"/fit", n.Fit, hull)
		}
	}
	for _, id := range keys(c.Worlds) {
		w := c.Worlds[id]
		if hull, ok := c.Ships[w.Start.Ship]; ok {
			check("worlds/"+string(id)+"/start.fit", w.Start.Fit, hull)
		}
	}
}

// TestFitProblemMounts pins the mount matcher's edge cases: two mounts of one
// kind must each hold a module at or below their size; a module without a size
// takes any mount of its kind; a module without a kind claims no mount.
func TestFitProblemMounts(t *testing.T) {
	hull := &ShipDef{Mounts: []Mount{{Kind: MountTurret, Size: SizeS}, {Kind: MountTurret, Size: SizeM}}}
	small := &ModuleDef{Slot: SlotHigh, Mount: MountTurret, Size: SizeS}
	large := &ModuleDef{Slot: SlotHigh, Mount: MountTurret, Size: SizeM}
	huge := &ModuleDef{Slot: SlotHigh, Mount: MountTurret, Size: SizeL}
	any := &ModuleDef{Slot: SlotHigh, Mount: MountTurret}
	rocket := &ModuleDef{Slot: SlotHigh, Mount: MountLauncher, Size: SizeS}

	if issue := hull.FitProblem(large, nil); issue != nil {
		t.Errorf("medium turret on a medium mount: %+v", issue)
	}
	if issue := hull.FitProblem(small, []*ModuleDef{large}); issue != nil {
		t.Errorf("small turret after medium turret: %+v", issue)
	}
	if issue := hull.FitProblem(large, []*ModuleDef{large}); issue == nil || issue.Oversize {
		t.Errorf("second medium turret has no free mount (not oversize), got %+v", issue)
	}
	if issue := hull.FitProblem(any, []*ModuleDef{large}); issue != nil {
		t.Errorf("sizeless turret fits the free small mount: %+v", issue)
	}
	if issue := hull.FitProblem(huge, nil); issue == nil || !issue.Oversize {
		t.Errorf("large turret must be oversize, got %+v", issue)
	}
	if issue := hull.FitProblem(rocket, nil); issue == nil || issue.Oversize {
		t.Errorf("launcher on a turretless hull: %+v", issue)
	}
}

// TestValidateRejectsFittingLimits pins the new validator errors on mutated
// shipped content: oversize for a mount, a mount kind the hull lacks, and a
// class quota exceeded by an authored fit.
func TestValidateRejectsFittingLimits(t *testing.T) {
	cases := []struct {
		name   string
		mutate func(c *Content)
		want   string
	}{
		{"oversize", func(c *Content) {
			c.Worlds["world-default"].Start.Fit = []ModuleID{"mod-autocannon-m"}
		}, "is oversize for the turret mounts on hull ship-sloop"},
		{"missing mount kind", func(c *Content) {
			c.Worlds["world-default"].Start.Ship = "ship-cutlass"
			c.Worlds["world-default"].Start.Fit = []ModuleID{"mod-rocket-pod"}
		}, "needs a launcher mount and hull ship-cutlass has none free"},
		{"class quota", func(c *Content) {
			c.Worlds["world-default"].Start.Ship = "ship-cutlass"
			c.Worlds["world-default"].Start.Fit = []ModuleID{
				"mod-armor-rep", "mod-armor-rep", "mod-armor-rep", "mod-armor-rep",
			}
		}, "exceeds hull ship-cutlass's grid Class II quota (4 modules, quota 3)"},
		{"mount kind vocabulary", func(c *Content) {
			c.Modules["mod-mining-laser"].Mount = "beam"
		}, `modules/mod-mining-laser: unknown mount kind "beam"`},
		{"class vocabulary", func(c *Content) {
			c.Modules["mod-afterburner"].GridClass = "iv"
		}, `modules/mod-afterburner: unknown grid class "iv"`},
		{"mount on mid module", func(c *Content) {
			c.Modules["mod-afterburner"].Mount = MountTurret
		}, `modules/mod-afterburner: mount kind "turret" on a mid-slot module`},
		{"mount count", func(c *Content) {
			c.Ships["ship-sloop"].Mounts = c.Ships["ship-sloop"].Mounts[:2]
		}, "high_mounts declares 2 mounts for 3 high slots"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			c := MustLoad()
			tc.mutate(c)
			err := c.Validate()
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("validation missing %q, got:\n%v", tc.want, err)
			}
		})
	}
}
