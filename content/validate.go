package content

import (
	"errors"
	"fmt"
	"math"
	"slices"
	"strings"
)

// Allowed value sets. model.go constants are authoritative.
var (
	validCategories = map[string]bool{
		CatMineral: true, CatOre: true, CatAmmo: true, CatCharge: true, CatLoot: true,
	}
	slotRanks  = map[string]int{SlotHigh: 0, SlotMid: 1, SlotLow: 2}
	validKinds = map[string]bool{
		KindWeapon: true, KindShieldBoost: true, KindShieldExtender: true,
		KindArmorRep: true, KindArmorPlate: true, KindResistCoat: true,
		KindCapBooster: true, KindCapRecharger: true, KindAfterburner: true,
		KindMiningLaser: true, KindStasisWeb: true, KindWarpScram: true,
	}
	boostKinds = map[string]bool{
		KindShieldBoost: true, KindArmorRep: true, KindCapBooster: true,
		KindAfterburner: true, KindStasisWeb: true, KindWarpScram: true,
	}
	passiveKinds = map[string]bool{
		KindShieldExtender: true, KindArmorPlate: true, KindResistCoat: true, KindCapRecharger: true,
	}
	validCelKinds = map[string]bool{
		CelStar: true, CelPlanet: true, CelMoon: true, CelStation: true,
		CelBelt: true, CelGate: true, CelAnomaly: true,
	}
	validServices   = map[string]bool{"repair": true, "market": true, "refine": true, "fitting": true, "industry": true}
	validSecurities = map[string]bool{"high": true, "low": true, "null": true}
	// recipeFamilies is the craft-growth vocabulary of RecipeDef.Family
	// (model.go constants are authoritative).
	recipeFamilies = map[string]bool{FamilyHulls: true, FamilyModules: true, FamilyCharges: true}
	// validOrigins is the hull heritage vocabulary of docs/lore/NAMING.md's
	// origin column (lowercase in JSON). Empty origin is allowed here —
	// shipped-data completeness is enforced by content/*_test.go, the same
	// way contract descriptions are.
	validOrigins = map[string]bool{
		"moravian": true, "polish": true, "nordic": true, "latinate": true, "old-works": true,
	}
	// activeKinds run cycles (used by the balance sanity tests).
	activeKinds = map[string]bool{
		KindWeapon: true, KindShieldBoost: true, KindArmorRep: true,
		KindCapBooster: true, KindAfterburner: true, KindMiningLaser: true,
		KindStasisWeb: true, KindWarpScram: true,
	}
)

type validator struct {
	c    *Content
	errs []string
}

func (v *validator) addf(format string, args ...any) {
	v.errs = append(v.errs, fmt.Sprintf(format, args...))
}

// keys returns the sorted keys of a map keyed by string-like types so error
// output is deterministic.
func keys[K ~string, V any](m map[K]V) []K {
	out := make([]K, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	slices.Sort(out)
	return out
}

// Validate cross-checks the whole content graph and returns ONE error whose
// message lists every problem found, one per line, each prefixed with its
// file/id context. It returns nil when the content is clean.
//
// Load already rejects duplicate top-level IDs across files (with file
// names); Validate covers everything else — including duplicate celestial
// IDs, which survive loading because celestials live in slices.
func (c *Content) Validate() error {
	if c == nil {
		return errors.New("content: nil content")
	}
	v := &validator{c: c}
	v.items()
	v.ships()
	v.modules()
	v.recipes()
	v.npcs()
	v.systems()
	v.stationMarkets()
	v.contracts()
	v.traffic()
	v.worlds()
	v.universe()
	if len(v.errs) == 0 {
		return nil
	}
	return fmt.Errorf("content: %d validation problem(s):\n%s", len(v.errs), strings.Join(v.errs, "\n"))
}

func (v *validator) items() {
	c := v.c
	for _, id := range keys(c.Items) {
		it := c.Items[id]
		if id == "" {
			v.addf("items: missing id (name %q)", it.Name)
			continue
		}
		ctx := "items/" + string(id)
		if it.Name == "" {
			v.addf("%s: missing name", ctx)
		}
		if !validCategories[it.Category] {
			v.addf("%s: unknown category %q (want mineral|ore|ammo|charge|loot)", ctx, it.Category)
		}
		if it.Volume < 0 {
			v.addf("%s: negative volume %v", ctx, it.Volume)
		}
		if it.BasePrice < 0 {
			v.addf("%s: negative base price %v", ctx, it.BasePrice)
		}
		if it.Category == CatOre {
			v.oreYields(ctx, it)
		}
		if it.Damage != nil && (it.Damage.EM < 0 || it.Damage.Thermal < 0 || it.Damage.Kinetic < 0 || it.Damage.Explosive < 0) {
			v.addf("%s: negative damage component", ctx)
		}
	}
}

func (v *validator) oreYields(ctx string, it *ItemDef) {
	if len(it.RefineYield) == 0 {
		v.addf("%s: ore has no refine_yield", ctx)
		return
	}
	sum := 0.0
	for _, target := range keys(it.RefineYield) {
		y := it.RefineYield[target]
		if _, ok := v.c.Items[target]; !ok {
			v.addf("%s: refine yield references unknown item %q", ctx, target)
		}
		if y <= 0 || y > 1 {
			v.addf("%s: refine yield for %s is %v, must be in (0,1]", ctx, target, y)
		}
		sum += y
	}
	if sum > 1 {
		v.addf("%s: refine yields sum to %v, must be <= 1", ctx, sum)
	}
}

func (v *validator) ships() {
	for _, id := range keys(v.c.Ships) {
		s := v.c.Ships[id]
		if id == "" {
			v.addf("ships: missing id (name %q)", s.Name)
			continue
		}
		ctx := "ships/" + string(id)
		if s.Name == "" {
			v.addf("%s: missing name", ctx)
		}
		if s.Origin != "" && !validOrigins[s.Origin] {
			v.addf("%s: unknown origin %q (want moravian|polish|nordic|latinate|old-works)", ctx, s.Origin)
		}
		if s.Shield < 0 {
			v.addf("%s: negative shield %v", ctx, s.Shield)
		}
		if s.Armor < 0 {
			v.addf("%s: negative armor %v", ctx, s.Armor)
		}
		if s.Structure < 0 {
			v.addf("%s: negative structure %v", ctx, s.Structure)
		}
		if s.Capacitor < 0 {
			v.addf("%s: negative capacitor %v", ctx, s.Capacitor)
		}
		if s.Recharge < 0 {
			v.addf("%s: negative recharge %v", ctx, s.Recharge)
		}
		if s.MaxSpeed < 0 {
			v.addf("%s: negative max speed %v", ctx, s.MaxSpeed)
		}
		if s.Agility < 0 {
			v.addf("%s: negative agility %v", ctx, s.Agility)
		}
		if s.Mass < 0 {
			v.addf("%s: negative mass %v", ctx, s.Mass)
		}
		if s.SigRadius < 0 {
			v.addf("%s: negative sig radius %v", ctx, s.SigRadius)
		}
		if s.Cargo < 0 {
			v.addf("%s: negative cargo %v", ctx, s.Cargo)
		}
		if s.OreHold < 0 {
			v.addf("%s: negative ore hold %v", ctx, s.OreHold)
		}
		if s.WarpSpeed < 0 {
			v.addf("%s: negative warp speed %v", ctx, s.WarpSpeed)
		}
		if s.MaxTargetRange < 0 {
			v.addf("%s: negative max target range %v", ctx, s.MaxTargetRange)
		}
		if s.ScanResolution < 0 {
			v.addf("%s: negative scan resolution %v", ctx, s.ScanResolution)
		}
		if s.Slots.High < 0 || s.Slots.Mid < 0 || s.Slots.Low < 0 {
			v.addf("%s: negative slot count (high %d, mid %d, low %d)", ctx, s.Slots.High, s.Slots.Mid, s.Slots.Low)
		}
		if s.MaxTargets < 0 {
			v.addf("%s: negative max targets %d", ctx, s.MaxTargets)
		}
		v.resists(ctx, s.Resists)
	}
}

func (v *validator) resists(ctx string, r Resists) {
	for _, t := range AllDamageTypes {
		x := r.Of(t)
		if x < 0 || x >= 1 {
			v.addf("%s: resist %s %v outside [0,1)", ctx, t, x)
		}
	}
}

func (v *validator) modules() {
	for _, id := range keys(v.c.Modules) {
		m := v.c.Modules[id]
		if id == "" {
			v.addf("modules: missing id (name %q)", m.Name)
			continue
		}
		ctx := "modules/" + string(id)
		if m.Name == "" {
			v.addf("%s: missing name", ctx)
		}
		if _, ok := slotRanks[m.Slot]; !ok {
			v.addf("%s: unknown slot %q (want high|mid|low)", ctx, m.Slot)
		}
		if !validKinds[m.Kind] {
			v.addf("%s: unknown kind %q", ctx, m.Kind)
		}
		if m.ActivationCost < 0 {
			v.addf("%s: negative activation cost %v", ctx, m.ActivationCost)
		}
		if m.ActivationTime < 0 {
			v.addf("%s: negative activation time %v", ctx, m.ActivationTime)
		}
		if m.Price < 0 {
			v.addf("%s: negative price %v", ctx, m.Price)
		}

		// Required attribute blocks per kind.
		if m.Kind == KindWeapon {
			if m.Weapon == nil {
				v.addf("%s: kind %q requires a weapon block", ctx, m.Kind)
			}
		}
		if m.Kind == KindMiningLaser {
			if m.Mining == nil {
				v.addf("%s: kind %q requires a mining block", ctx, m.Kind)
			} else {
				if m.Mining.VolumePerCycle < 0 {
					v.addf("%s: negative volume per cycle %v", ctx, m.Mining.VolumePerCycle)
				}
				if m.Mining.CycleTime < 0 {
					v.addf("%s: negative cycle time %v", ctx, m.Mining.CycleTime)
				}
			}
		}
		if boostKinds[m.Kind] && m.Boost == nil {
			v.addf("%s: kind %q requires a boost block", ctx, m.Kind)
		}
		if passiveKinds[m.Kind] && m.Passive == nil {
			v.addf("%s: kind %q requires a passive block", ctx, m.Kind)
		}

		if m.Weapon != nil {
			w := m.Weapon
			if w.Damage.EM < 0 || w.Damage.Thermal < 0 || w.Damage.Kinetic < 0 || w.Damage.Explosive < 0 {
				v.addf("%s: negative weapon damage component", ctx)
			}
			if w.Optimal < 0 {
				v.addf("%s: negative optimal range %v", ctx, w.Optimal)
			}
			if w.Falloff < 0 {
				v.addf("%s: negative falloff %v", ctx, w.Falloff)
			}
			if w.Tracking < 0 {
				v.addf("%s: negative tracking %v", ctx, w.Tracking)
			}
			if w.ShotTime < 0 {
				v.addf("%s: negative shot time %v", ctx, w.ShotTime)
			}
			if w.AmmoClass != "" && !v.ammoClassExists(w.AmmoClass) {
				v.addf("%s: weapon ammo class %q matches no ammo or charge item", ctx, w.AmmoClass)
			}
		}
		if m.Passive != nil {
			p := m.Passive
			if p.ShieldHP < 0 {
				v.addf("%s: negative passive shield hp %v", ctx, p.ShieldHP)
			}
			if p.ArmorHP < 0 {
				v.addf("%s: negative passive armor hp %v", ctx, p.ArmorHP)
			}
			if p.Capacitor < 0 {
				v.addf("%s: negative passive capacitor %v", ctx, p.Capacitor)
			}
			v.resists(ctx+" passive resist bonus", p.ResistBonus)
		}
		if m.Boost != nil {
			b := m.Boost
			if b.ShieldBoostPerCycle < 0 {
				v.addf("%s: negative shield boost per cycle %v", ctx, b.ShieldBoostPerCycle)
			}
			if b.ArmorRepPerCycle < 0 {
				v.addf("%s: negative armor rep per cycle %v", ctx, b.ArmorRepPerCycle)
			}
			if b.CapRestore < 0 {
				v.addf("%s: negative cap restore %v", ctx, b.CapRestore)
			}
			if b.SpeedMultiplier < 0 {
				v.addf("%s: negative speed multiplier %v", ctx, b.SpeedMultiplier)
			}
			if b.WebStrength < 0 || b.WebStrength > 1 {
				v.addf("%s: web strength %v outside [0,1]", ctx, b.WebStrength)
			}
			if b.ScramDuration < 0 {
				v.addf("%s: negative scram duration %v", ctx, b.ScramDuration)
			}
		}
	}
}

// ammoClassExists reports whether some ammo or charge item uses the class.
func (v *validator) ammoClassExists(class string) bool {
	for _, id := range keys(v.c.Items) {
		it := v.c.Items[id]
		if (it.Category == CatAmmo || it.Category == CatCharge) && it.AmmoClass == class {
			return true
		}
	}
	return false
}

// recipes cross-checks every manufacturing recipe: the rec- prefix, product
// resolution (the market-order convention: item, then ship, then module —
// and only ships/modules/ammo/charges may be products), mineral-only inputs
// (the v1 two-tier rule), positive qty/time, the family vocabulary, and
// completeness: every ship, module and ammo/charge item must be produced by
// at least one recipe.
func (v *validator) recipes() {
	c := v.c
	covered := map[ItemID]bool{}
	for _, id := range keys(c.Recipes) {
		r := c.Recipes[id]
		if id == "" {
			v.addf("recipes: missing id (product %q)", r.Product)
			continue
		}
		ctx := "recipes/" + string(id)
		if !strings.HasPrefix(string(id), "rec-") {
			v.addf("%s: id must use the rec- prefix", ctx)
		}
		if r.Product == "" {
			v.addf("%s: missing product", ctx)
		} else {
			covered[r.Product] = true
			if it, ok := c.Items[r.Product]; ok {
				if it.Category != CatAmmo && it.Category != CatCharge {
					v.addf("%s: product %q is category %q; only ships, modules and ammo/charge items are manufactured",
						ctx, r.Product, it.Category)
				}
			} else if _, isShip := c.Ships[ShipID(r.Product)]; !isShip {
				if _, isMod := c.Modules[ModuleID(r.Product)]; !isMod {
					v.addf("%s: product %q is not an item, ship, or module", ctx, r.Product)
				}
			}
		}
		if r.Qty < 1 {
			v.addf("%s: qty %d must be >= 1", ctx, r.Qty)
		}
		if r.Time <= 0 {
			v.addf("%s: time %v must be > 0", ctx, r.Time)
		}
		if !recipeFamilies[r.Family] {
			v.addf("%s: unknown family %q (want hulls|modules|charges)", ctx, r.Family)
		}
		if len(r.Inputs) == 0 {
			v.addf("%s: no inputs", ctx)
		}
		for i, in := range r.Inputs {
			item, ok := c.Items[in.Item]
			if !ok {
				v.addf("%s: inputs[%d] references unknown item %q", ctx, i, in.Item)
				continue
			}
			if item.Category != CatMineral {
				v.addf("%s: inputs[%d] item %q is category %q; recipes take minerals only (ore refines via refine_yield)",
					ctx, i, in.Item, item.Category)
			}
			if in.Qty <= 0 {
				v.addf("%s: inputs[%d] qty %v must be > 0", ctx, i, in.Qty)
			}
		}
	}
	// Completeness: every manufactured product has at least one recipe.
	for _, id := range keys(c.Ships) {
		if !covered[ItemID(id)] {
			v.addf("recipes: no recipe produces ship %q", id)
		}
	}
	for _, id := range keys(c.Modules) {
		if !covered[ItemID(id)] {
			v.addf("recipes: no recipe produces module %q", id)
		}
	}
	for _, id := range keys(c.Items) {
		it := c.Items[id]
		if (it.Category == CatAmmo || it.Category == CatCharge) && !covered[id] {
			v.addf("recipes: no recipe produces item %q", id)
		}
	}
}

func (v *validator) npcs() {
	for _, id := range keys(v.c.NPCs) {
		n := v.c.NPCs[id]
		if id == "" {
			v.addf("npcs: missing id (name %q)", n.Name)
			continue
		}
		ctx := "npcs/" + string(id)
		if n.Name == "" {
			v.addf("%s: missing name", ctx)
		}
		hull, hullOK := v.c.Ships[n.Ship]
		if !hullOK {
			v.addf("%s: unknown ship %q", ctx, n.Ship)
		} else {
			v.checkFit(ctx+"/fit", n.Fit, hull)
		}
		if n.AggroRange < 0 {
			v.addf("%s: negative aggro range %v", ctx, n.AggroRange)
		}
		if n.OrbitRange < 0 {
			v.addf("%s: negative orbit range %v", ctx, n.OrbitRange)
		}
		if n.FleeThreshold < 0 || n.FleeThreshold > 1 {
			v.addf("%s: flee threshold %v outside [0,1]", ctx, n.FleeThreshold)
		}
		if n.Bounty < 0 {
			v.addf("%s: negative bounty %v", ctx, n.Bounty)
		}
		for i, d := range n.Loot {
			if _, ok := v.c.Items[d.Item]; !ok {
				v.addf("%s: loot[%d] references unknown item %q", ctx, i, d.Item)
			}
			if d.Chance < 0 || d.Chance > 1 {
				v.addf("%s: loot[%d] chance %v outside [0,1]", ctx, i, d.Chance)
			}
			if d.MinQty < 1 {
				v.addf("%s: loot[%d] min_qty %d must be >= 1", ctx, i, d.MinQty)
			}
			if d.MaxQty < 1 {
				v.addf("%s: loot[%d] max_qty %d must be >= 1", ctx, i, d.MaxQty)
			}
			if d.MinQty > d.MaxQty {
				v.addf("%s: loot[%d] min_qty %d > max_qty %d", ctx, i, d.MinQty, d.MaxQty)
			}
		}
	}
}

// checkFit validates a fit list: every module must exist, classes must be
// grouped high-then-mid-then-low, and no class may overflow the hull.
func (v *validator) checkFit(ctx string, fit []ModuleID, hull *ShipDef) {
	var counts [3]int
	last := -1
	for i, mid := range fit {
		m, ok := v.c.Modules[mid]
		if !ok {
			v.addf("%s[%d]: unknown module %q", ctx, i, mid)
			continue
		}
		rank, known := slotRanks[m.Slot]
		if !known {
			continue // unknown slot already reported by modules()
		}
		if rank < last {
			v.addf("%s[%d]: module %s has slot class %q placed out of order (fit must list high, then mid, then low)", ctx, i, mid, m.Slot)
		} else if rank > last {
			last = rank
		}
		counts[rank]++
	}
	limits := [3]int{hull.Slots.High, hull.Slots.Mid, hull.Slots.Low}
	names := [3]string{SlotHigh, SlotMid, SlotLow}
	for i := 0; i < 3; i++ {
		if counts[i] > limits[i] {
			v.addf("%s: %d %s-slot modules exceed hull %s %s slots (%d)", ctx, counts[i], names[i], hull.ID, names[i], limits[i])
		}
	}
}

func (v *validator) systems() {
	for _, id := range keys(v.c.Systems) {
		sys := v.c.Systems[id]
		if id == "" {
			v.addf("systems: missing id (name %q)", sys.Name)
			continue
		}
		ctx := "systems/" + string(id)
		if sys.Name == "" {
			v.addf("%s: missing name", ctx)
		}
		if !validSecurities[sys.Security] {
			v.addf("%s: unknown security %q (want high|low|null)", ctx, sys.Security)
		}

		seenCel := map[CelID]bool{}
		for i := range sys.Celestials {
			cel := &sys.Celestials[i]
			if cel.ID == "" {
				v.addf("%s: celestial[%d] missing id (name %q)", ctx, i, cel.Name)
				continue
			}
			cctx := ctx + "/" + string(cel.ID)
			if seenCel[cel.ID] {
				v.addf("%s: duplicate celestial id %q", ctx, cel.ID)
			}
			seenCel[cel.ID] = true
			if cel.Name == "" {
				v.addf("%s: missing name", cctx)
			}
			if !validCelKinds[cel.Kind] {
				v.addf("%s: unknown celestial kind %q", cctx, cel.Kind)
			}
			if cel.Radius < 0 {
				v.addf("%s: negative radius %v", cctx, cel.Radius)
			}
			v.celestialBlocks(cctx, sys, cel)
			v.spawns(cctx, cel.Spawns)
		}

		v.market(ctx, sys)
	}
}

// celestialBlocks checks the kind<->block coherence of a celestial and all
// cross-references inside it, including gate reciprocity.
func (v *validator) celestialBlocks(cctx string, sys *SystemDef, cel *CelestialDef) {
	// Block must be present exactly when its kind says so.
	pairs := []struct {
		kind string
		has  bool
		what string
	}{
		{CelGate, cel.Gate != nil, "gate"},
		{CelStation, cel.Station != nil, "station"},
		{CelBelt, cel.Belt != nil, "belt"},
		{CelAnomaly, cel.Anomaly != nil, "anomaly"},
	}
	for _, p := range pairs {
		if cel.Kind == p.kind && !p.has {
			v.addf("%s: %s celestial missing %s block", cctx, p.kind, p.what)
		}
		if cel.Kind != p.kind && p.has {
			v.addf("%s: kind %q must not have a %s block", cctx, cel.Kind, p.what)
		}
	}

	if cel.Gate != nil {
		v.gate(cctx, sys, cel)
	}
	if cel.Station != nil {
		for _, s := range cel.Station.Services {
			if !validServices[s] {
				v.addf("%s: unknown station service %q (allowed: repair, market, refine, fitting, industry)", cctx, s)
			}
		}
	}
	if cel.Belt != nil {
		for i, a := range cel.Belt.Asteroids {
			ore, ok := v.c.Items[a.Ore]
			if !ok {
				v.addf("%s: belt references unknown ore %q", cctx, a.Ore)
			} else if ore.Category != CatOre {
				v.addf("%s: belt asteroid ore %q is not category ore", cctx, a.Ore)
			}
			if a.Count < 1 {
				v.addf("%s: asteroid[%d] count %d must be >= 1", cctx, i, a.Count)
			}
			if a.Volume <= 0 {
				v.addf("%s: asteroid[%d] volume %v must be > 0", cctx, i, a.Volume)
			}
		}
	}
	if cel.Anomaly != nil {
		if cel.Anomaly.RespawnSec < 0 {
			v.addf("%s: anomaly respawn_sec %v must be >= 0", cctx, cel.Anomaly.RespawnSec)
		}
		v.spawns(cctx+"/anomaly", cel.Anomaly.Spawns)
	}
}

func (v *validator) gate(cctx string, sys *SystemDef, cel *CelestialDef) {
	g := cel.Gate
	if g.ToSystem == "" {
		v.addf("%s: gate missing to_system", cctx)
		return
	}
	dest, ok := v.c.Systems[g.ToSystem]
	if !ok {
		v.addf("%s: gate destination system %q does not exist", cctx, g.ToSystem)
		return
	}
	if g.ToCel == "" {
		v.addf("%s: gate missing to_cel", cctx)
		return
	}
	found := false
	for i := range dest.Celestials {
		if dest.Celestials[i].ID == g.ToCel {
			found = true
			break
		}
	}
	if !found {
		v.addf("%s: gate destination celestial %q not found in %s", cctx, g.ToCel, g.ToSystem)
		return
	}
	// Reciprocal: the destination must have a gate pointing back to this
	// very celestial.
	reciprocal := false
	for i := range dest.Celestials {
		d := &dest.Celestials[i]
		if d.Gate != nil && d.Gate.ToSystem == sys.ID && d.Gate.ToCel == cel.ID {
			reciprocal = true
			break
		}
	}
	if !reciprocal {
		v.addf("%s: no reciprocal gate in %s pointing back to %s", cctx, g.ToSystem, cel.ID)
	}
}

func (v *validator) spawns(ctx string, spawns []SpawnDef) {
	for i, s := range spawns {
		if _, ok := v.c.NPCs[s.NpcID]; !ok {
			v.addf("%s: spawn[%d] references unknown npc %q", ctx, i, s.NpcID)
		}
		if s.Count < 1 {
			v.addf("%s: spawn[%d] count %d must be >= 1", ctx, i, s.Count)
		}
		if s.RespawnSec < 0 {
			v.addf("%s: spawn[%d] respawn_sec %v must be >= 0", ctx, i, s.RespawnSec)
		}
	}
}

// market validates the founding orders that seed station books. Orders
// live ONLY on stations since the v2 market schema: a system-level block
// is rejected, station orders need the market service, both sides must
// name a valid side, and the founding book must open never-crossed (every
// bid strictly below every ask for the same item).
func (v *validator) market(ctx string, sys *SystemDef) {
	if len(sys.Market) > 0 {
		v.addf("%s: system-level market block is not allowed (founding orders live on station.market)", ctx)
	}
	for i := range sys.Celestials {
		cel := &sys.Celestials[i]
		if cel.Station == nil || len(cel.Station.Market) == 0 {
			continue
		}
		cctx := ctx + "/" + string(cel.ID)
		if !slices.Contains(cel.Station.Services, "market") {
			v.addf("%s: market orders on a station without the market service", cctx)
		}
		// Best founding ask/bid per item, for the never-crossed seed check.
		ask := map[ItemID]float64{}
		bid := map[ItemID]float64{}
		for _, o := range cel.Station.Market {
			// Orders resolve item first, then a ship ID (ships are listed on
			// the market by casting ShipID to ItemID — see
			// docs/CONTENT_AUTHORING.md), then a module ID.
			if _, ok := v.c.Items[o.Item]; !ok {
				if _, isShip := v.c.Ships[ShipID(o.Item)]; !isShip {
					if _, isMod := v.c.Modules[ModuleID(o.Item)]; !isMod {
						v.addf("%s: market order for %q is not an item, ship, or module", cctx, o.Item)
					}
				}
			}
			if o.Qty < 1 {
				v.addf("%s: market order %q qty %d must be >= 1", cctx, o.Item, o.Qty)
			}
			if o.Price < 0 {
				v.addf("%s: market order %q price %v must be >= 0", cctx, o.Item, o.Price)
			}
			switch o.Side {
			case "", "sell":
				if p, ok := ask[o.Item]; !ok || o.Price < p {
					ask[o.Item] = o.Price
				}
			case "buy":
				if p, ok := bid[o.Item]; !ok || o.Price > p {
					bid[o.Item] = o.Price
				}
			default:
				v.addf("%s: market order %q has unknown side %q (sell|buy)", cctx, o.Item, o.Side)
			}
		}
		// Never-crossed invariant starts at seeding: a founding bid at or
		// above the founding ask would trade against itself.
		for _, item := range keys(bid) {
			if a, ok := ask[item]; ok && bid[item] >= a {
				v.addf("%s: founding bid %v for %q crosses its ask %v", cctx, bid[item], item, a)
			}
		}
	}
}

// stationMarkets checks the station celestial ids the market books are
// keyed by: two systems sharing a station id would silently share one book.
func (v *validator) stationMarkets() {
	seen := map[CelID]SystemID{}
	for _, id := range keys(v.c.Systems) {
		for i := range v.c.Systems[id].Celestials {
			cel := &v.c.Systems[id].Celestials[i]
			if cel.Kind != CelStation {
				continue
			}
			if prev, dup := seen[cel.ID]; dup {
				v.addf("systems/%s: station id %q is already used by system %s (station markets are keyed by celestial id)",
					id, cel.ID, prev)
				continue
			}
			seen[cel.ID] = id
		}
	}
}

// contracts cross-checks every mission-board contract: kind-specific
// objective references (rat class, cargo item, stations), counts and reward.
func (v *validator) contracts() {
	for _, id := range keys(v.c.Contracts) {
		ct := v.c.Contracts[id]
		if id == "" {
			v.addf("contracts: missing id (title %q)", ct.Title)
			continue
		}
		ctx := "contracts/" + string(id)
		if ct.Title == "" {
			v.addf("%s: missing title", ctx)
		}
		switch ct.Kind {
		case ContractKill, ContractDeliver, ContractMine:
		default:
			v.addf("%s: unknown kind %q (want kill|deliver|mine)", ctx, ct.Kind)
		}
		if ct.Level < 1 {
			v.addf("%s: level %d must be >= 1", ctx, ct.Level)
		}
		if !v.isStation(ct.Station) {
			v.addf("%s: offering station %q is not a station", ctx, ct.Station)
		}
		obj := &ct.Objective
		if obj.Count < 1 {
			v.addf("%s: count %d must be >= 1", ctx, obj.Count)
		}
		switch ct.Kind {
		case ContractKill:
			if _, ok := v.c.NPCs[obj.Target]; !ok {
				v.addf("%s: unknown rat class %q", ctx, obj.Target)
			}
			if obj.Item != "" || obj.Destination != "" {
				v.addf("%s: kill contract must not set item/destination", ctx)
			}
		case ContractDeliver:
			if _, ok := v.c.Items[obj.Item]; !ok {
				v.addf("%s: unknown cargo item %q", ctx, obj.Item)
			} else if def := v.c.Items[obj.Item]; def.Volume > 0 {
				units := float64(obj.Count) / def.Volume
				if math.Abs(units-math.Round(units)) > 1e-6 {
					v.addf("%s: count %d m³ is not a whole number of units (volume %v)", ctx, obj.Count, def.Volume)
				}
			}
			if !v.isStation(obj.Destination) {
				v.addf("%s: destination %q is not a station", ctx, obj.Destination)
			} else if obj.Destination == ct.Station {
				v.addf("%s: destination must differ from the offering station", ctx)
			}
			if obj.Target != "" {
				v.addf("%s: deliver contract must not set target", ctx)
			}
		case ContractMine:
			if def, ok := v.c.Items[obj.Item]; !ok {
				v.addf("%s: unknown ore %q", ctx, obj.Item)
			} else if def.Category != CatOre {
				v.addf("%s: ore %q is not category ore", ctx, obj.Item)
			}
			if obj.Destination != "" || obj.Target != "" {
				v.addf("%s: mine contract must not set target/destination", ctx)
			}
		}
		rw := &ct.Reward
		if rw.Credits < 0 {
			v.addf("%s: negative credits %v", ctx, rw.Credits)
		}
		if rw.StandingGain < 0 {
			v.addf("%s: negative standing_gain %v", ctx, rw.StandingGain)
		}
		if rw.Item != "" {
			if _, ok := v.c.Items[rw.Item]; !ok {
				v.addf("%s: reward references unknown item %q", ctx, rw.Item)
			}
			if rw.Qty < 1 {
				v.addf("%s: reward item %q needs qty >= 1", ctx, rw.Item)
			}
		}
	}
}

// traffic cross-checks every system's living-universe traffic configuration:
// route endpoints resolve inside the system, hulls/cargo/raiders exist, the
// cargo fits the hull, and intervals are sane. Low/null systems must name a
// raider class that actually spawns in that system.
func (v *validator) traffic() {
	for _, sys := range keys(v.c.Traffic) {
		td := v.c.Traffic[sys]
		if sys == "" {
			v.addf("traffic: missing system")
			continue
		}
		ctx := "traffic/" + string(sys)
		sysDef, ok := v.c.Systems[sys]
		if !ok {
			v.addf("%s: unknown system %q", ctx, sys)
			continue
		}
		if len(td.Routes) == 0 {
			v.addf("%s: no routes (at least one route required)", ctx)
		}
		security := sysDef.Security
		if security != "high" && td.Raider == "" {
			v.addf("%s: security %q traffic requires a raider npc id", ctx, security)
		}
		if td.Raider != "" {
			if _, ok := v.c.NPCs[td.Raider]; !ok {
				v.addf("%s: unknown raider npc %q", ctx, td.Raider)
			} else if !npcSpawnsInSystem(sysDef, td.Raider) {
				v.addf("%s: raider %q is not spawned anywhere in this system", ctx, td.Raider)
			}
		}
		for i, r := range td.Routes {
			rctx := fmt.Sprintf("%s/routes[%d]", ctx, i)
			if r.From == r.To {
				v.addf("%s: from and to must differ", rctx)
			}
			for _, end := range []struct {
				what string
				id   CelID
			}{{"from", r.From}, {"to", r.To}} {
				if end.id == "" {
					v.addf("%s: missing %s endpoint", rctx, end.what)
					continue
				}
				cel := celestialIn(sysDef, end.id)
				if cel == nil {
					v.addf("%s: unknown %s endpoint %q (not a celestial of %s)", rctx, end.what, end.id, sys)
					continue
				}
				switch cel.Kind {
				case CelStation, CelGate, CelBelt:
				default:
					v.addf("%s: %s endpoint %q is kind %q (want station|gate|belt)", rctx, end.what, end.id, cel.Kind)
				}
			}
			hull, ok := v.c.Ships[r.Hull]
			if !ok {
				v.addf("%s: unknown hull %q", rctx, r.Hull)
			}
			if r.IntervalTicks < 1 {
				v.addf("%s: interval_ticks %d must be >= 1", rctx, r.IntervalTicks)
			}
			if r.Cargo.Qty < 1 {
				v.addf("%s: cargo qty %d must be >= 1", rctx, r.Cargo.Qty)
			}
			item, ok := v.c.Items[r.Cargo.Item]
			if !ok {
				v.addf("%s: unknown cargo item %q", rctx, r.Cargo.Item)
			} else if hull != nil && item.Volume > 0 {
				if vol := item.Volume * float64(r.Cargo.Qty); vol > hull.Cargo {
					v.addf("%s: cargo volume %v exceeds hull %s capacity %v", rctx, vol, hull.ID, hull.Cargo)
				}
			}
		}
		if td.Patrol != nil {
			if len(td.Patrol.Hulls) == 0 {
				v.addf("%s/patrol: no hulls (at least one required)", ctx)
			}
			for i, h := range td.Patrol.Hulls {
				if _, ok := v.c.Ships[h]; !ok {
					v.addf("%s/patrol: hulls[%d] unknown hull %q", ctx, i, h)
					continue
				}
				if _, ok := v.c.GuardFor(h); !ok {
					v.addf("%s/patrol: hulls[%d] %q has no npc-guard-* definition", ctx, i, h)
				}
			}
			if td.Patrol.IntervalTicks < 1 {
				v.addf("%s/patrol: interval_ticks %d must be >= 1", ctx, td.Patrol.IntervalTicks)
			}
		}
	}
}

// celestialIn returns the celestial definition with the given ID inside sys.
func celestialIn(sys *SystemDef, id CelID) *CelestialDef {
	for i := range sys.Celestials {
		if sys.Celestials[i].ID == id {
			return &sys.Celestials[i]
		}
	}
	return nil
}

// npcSpawnsInSystem reports whether npc appears in any spawn group (ambient
// or anomaly) of the system.
func npcSpawnsInSystem(sys *SystemDef, npc NpcID) bool {
	for i := range sys.Celestials {
		cel := &sys.Celestials[i]
		for _, sp := range cel.Spawns {
			if sp.NpcID == npc {
				return true
			}
		}
		if cel.Anomaly != nil {
			for _, sp := range cel.Anomaly.Spawns {
				if sp.NpcID == npc {
					return true
				}
			}
		}
	}
	return false
}

// isStation reports whether cel is a station celestial in any system.
func (v *validator) isStation(cel CelID) bool {
	if cel == "" {
		return false
	}
	for _, id := range keys(v.c.Systems) {
		for i := range v.c.Systems[id].Celestials {
			c := &v.c.Systems[id].Celestials[i]
			if c.ID == cel && c.Kind == CelStation {
				return true
			}
		}
	}
	return false
}

func (v *validator) worlds() {
	for _, id := range keys(v.c.Worlds) {
		w := v.c.Worlds[id]
		if id == "" {
			v.addf("worlds: missing id (name %q)", w.Name)
			continue
		}
		ctx := "worlds/" + string(id)
		if w.Name == "" {
			v.addf("%s: missing name", ctx)
		}
		if _, ok := v.c.Systems[w.Start.System]; !ok {
			v.addf("%s: unknown start system %q", ctx, w.Start.System)
		}
		hull, hullOK := v.c.Ships[w.Start.Ship]
		if !hullOK {
			v.addf("%s: unknown start ship %q", ctx, w.Start.Ship)
		} else {
			v.checkFit(ctx+"/start.fit", w.Start.Fit, hull)
		}
		if w.Start.Wallet < 0 {
			v.addf("%s: negative wallet %v", ctx, w.Start.Wallet)
		}
		for i, st := range w.Start.Cargo {
			if _, ok := v.c.Items[st.Item]; !ok {
				v.addf("%s: start.cargo[%d]: unknown item %q", ctx, i, st.Item)
			}
			if st.Qty < 1 {
				v.addf("%s: start.cargo[%d]: qty %d must be >= 1", ctx, i, st.Qty)
			}
		}
		for i, cp := range w.Scenario {
			if cp.Name == "" {
				v.addf("%s: checkpoint[%d] missing name", ctx, i)
			}
			if cp.Feature == "" {
				v.addf("%s: checkpoint[%d] missing feature", ctx, i)
			}
		}
	}
	// The showcase world is required: it drives tests/showcase_test.go.
	if sc, ok := v.c.Worlds["world-showcase"]; !ok {
		v.addf("worlds: missing required world %q", "world-showcase")
	} else if len(sc.Scenario) < 10 {
		v.addf("worlds/world-showcase: %d scenario checkpoints, need at least 10", len(sc.Scenario))
	}
}

func (v *validator) universe() {
	u := v.c.Universe
	if u == nil {
		v.addf("universe.json: missing universe definition")
		return
	}
	if u.StartSystem == "" {
		v.addf("universe.json: missing start_system")
	} else if _, ok := v.c.Systems[u.StartSystem]; !ok {
		v.addf("universe.json: unknown start system %q", u.StartSystem)
	}
	if len(u.Systems) > 0 {
		v.addf("universe.json: systems must be empty (systems come from systems/*.json)")
		return
	}
	// Reachability: every system must be reachable through gates from the
	// start system.
	if u.StartSystem == "" {
		return
	}
	if _, ok := v.c.Systems[u.StartSystem]; !ok {
		return
	}
	seen := map[SystemID]bool{u.StartSystem: true}
	queue := []SystemID{u.StartSystem}
	for len(queue) > 0 {
		cur := queue[0]
		queue = queue[1:]
		sys := v.c.Systems[cur]
		if sys == nil {
			continue
		}
		for i := range sys.Celestials {
			g := sys.Celestials[i].Gate
			if g == nil {
				continue
			}
			if _, ok := v.c.Systems[g.ToSystem]; !ok {
				continue
			}
			if !seen[g.ToSystem] {
				seen[g.ToSystem] = true
				queue = append(queue, g.ToSystem)
			}
		}
	}
	for _, id := range keys(v.c.Systems) {
		if !seen[id] {
			v.addf("systems/%s: unreachable from start system %s", id, u.StartSystem)
		}
	}
}
