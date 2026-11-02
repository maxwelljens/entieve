package sim

import (
	"math"
	"sort"

	"github.com/entieve/entieve/content"
	"github.com/entieve/entieve/ecs"
	"github.com/entieve/entieve/format"
)

// Proficiency (CONTEXT.md): practice with a fitted family builds a persisted
// counter; each level gates advanced defs of that family and grants a small
// edge while the family is in use. No RNG, no wall clock, no decay.
//
// Level thresholds:
//
//	level = min(5, floor(sqrt(xp / 10)))
//
// so L1 needs 10 xp, L2 40, L3 90, L4 160 and L5 250. The square root makes
// the early ladder cheap and later levels expensive, exactly the shape the
// quality multipliers lean on: a basic ore cut pays 1, a rarity-4 cut 8.
const (
	proficiencyStep    = 10.0
	proficiencyEdgePer = 0.02 // +2% per level while the family is in use

	// combatXPPerDamage is the practice paid per point of applied damage:
	// a Scuttler (~300) pays about 12 (level I), a Hierarch (~1,400) about
	// 56, so sustained fights carry a line to V.
	combatXPPerDamage = 0.04
	// hullPracticeShare is the flown hull's cut of a job act (mining,
	// combat damage, support cycles); kills and deliveries stay full rate.
	hullPracticeShare = 0.1
	// supportXPPerCycle is one completed support-module cycle's practice
	// (shield/armor rep, cap booster, afterburner, web/scram). It is a
	// fraction because those cycles repeat every few seconds; a full point
	// would cap the family inside one fight.
	supportXPPerCycle = 0.1
	// flightXPMetres is one flight practice point: 100 AU of actual warp
	// displacement. flightHopCap caps one hop at 50 AU, so a warp pays at
	// most half a point and ordinary hopping stays minor next to a fight.
	flightXPMetres = 100 * format.AU
	flightHopCap   = 50 * format.AU
)

// Proficiency thresholds are the only source of levels: a family at V banks
// no more XP (Save.Proficiency clamps at proficiencyThreshold(5)).

// SavedProficiency is one family's practice counter. Persisted in ascending
// family order in Save.Proficiency; it never decays.
type SavedProficiency struct {
	Family string  `json:"family"`
	XP     float64 `json:"xp"`
}

// Proficiency is the player's level in a family (0 = untrained).
func (s *Sim) Proficiency(family string) int {
	return proficiencyLevel(s.proficiencyXP(family))
}

// proficiencyLevel is the pure level formula (0 xp → L0).
func proficiencyLevel(xp float64) int {
	if xp < proficiencyStep {
		return 0
	}
	level := int(math.Floor(math.Sqrt(xp / proficiencyStep)))
	if level > content.ProficiencyMax {
		return content.ProficiencyMax
	}
	return level
}

// proficiencyThreshold is the total XP at which a level starts
// (0, 10, 40, 90, 160, 250).
func proficiencyThreshold(level int) float64 {
	return proficiencyStep * float64(level*level)
}

// ProfPrereq is one prerequisite edge as the read API exposes it.
type ProfPrereq struct {
	Family string
	Level  int
}

// ProfFamily is one practice family as the tech-tree screen reads it. Plain
// data, canonical order, copies only: no UI types and no live handles.
//
// XP is the total banked toward the next level (clamped at 250 at MaxLevel)
// and XPNext is the total the next level needs (0 at MaxLevel). Meets means
// the player's own level clears the family's entry gate and every
// prerequisite edge is satisfied.
type ProfFamily struct {
	Family   string
	Name     string
	Level    int
	XP       int
	XPNext   int
	MaxLevel int
	Prereqs  []ProfPrereq
	Meets    bool
	Starter  bool
}

// ProficiencyFamilies returns every content family in ascending id order. It
// is a fresh slice per call; callers may sort or mutate their copy.
func (s *Sim) ProficiencyFamilies() []ProfFamily {
	ids := content.ProficiencyFamilyIDs()
	out := make([]ProfFamily, 0, len(ids))
	for _, id := range ids {
		total := s.proficiencyXP(id)
		level := proficiencyLevel(total)
		xp := int(math.Round(total))
		next := 0
		if level < content.ProficiencyMax {
			next = int(math.Round(proficiencyThreshold(level + 1)))
		}
		edges := content.Prerequisites(id)
		prereqs := make([]ProfPrereq, 0, len(edges))
		meets := s.Proficiency(id) >= s.familyEntryGate(id)
		for _, p := range edges {
			prereqs = append(prereqs, ProfPrereq{Family: p.Family, Level: p.Level})
			if s.Proficiency(p.Family) < p.Level {
				meets = false
			}
		}
		out = append(out, ProfFamily{
			Family:   id,
			Name:     content.FamilyLabel(id),
			Level:    level,
			XP:       xp,
			XPNext:   next,
			MaxLevel: content.ProficiencyMax,
			Prereqs:  prereqs,
			Meets:    meets,
			Starter:  s.starterFamily(id),
		})
	}
	return out
}

// familyEntryGate is the lowest gate among the family's defs: the level at
// which the family opens. 0 when no def uses the family.
func (s *Sim) familyEntryGate(family string) int {
	entry := -1
	for _, m := range s.Content.Modules {
		if m.Family != family {
			continue
		}
		if entry < 0 || m.Gate < entry {
			entry = m.Gate
		}
	}
	for _, sh := range s.Content.Ships {
		if sh.Family != family {
			continue
		}
		if entry < 0 || sh.Gate < entry {
			entry = sh.Gate
		}
	}
	if entry < 0 {
		return 0
	}
	return entry
}

// starterFamily reports whether the current world's start fits or flies the
// family.
func (s *Sim) starterFamily(family string) bool {
	w, ok := s.Content.World(s.worldID)
	if !ok {
		return false
	}
	if hull, ok := s.Content.Ship(w.Start.Ship); ok && hull.Family == family {
		return true
	}
	for _, mid := range w.Start.Fit {
		if m, ok := s.Content.Module(mid); ok && m.Family == family {
			return true
		}
	}
	return false
}

// proficiencyCheck is the one use gate: the def's own family level first,
// then every authored prerequisite edge in order. It returns the first missing
// requirement ("Requires Autocannon II") and false.
func (s *Sim) proficiencyCheck(family string, gate int) (string, bool) {
	if s.Proficiency(family) < gate {
		return proficiencyRequirement(family, gate), false
	}
	for _, p := range content.Prerequisites(family) {
		if s.Proficiency(p.Family) < p.Level {
			return proficiencyRequirement(p.Family, p.Level), false
		}
	}
	return "", true
}

// proficiencyXP returns a family's counter (0 = no entry).
func (s *Sim) proficiencyXP(family string) float64 {
	for i := range s.prof {
		if s.prof[i].Family == family {
			return s.prof[i].XP
		}
	}
	return 0
}

// proficiencyEdge is the multiplier a family's level grants (1 at level 0).
func (s *Sim) proficiencyEdge(family string) float64 {
	return 1 + proficiencyEdgePer*float64(s.Proficiency(family))
}

// awardProficiency credits xp to a family, keeping the canonical order, and
// logs one line per level gained. A hull-family level also re-derives the
// player's speed edge immediately.
func (s *Sim) awardProficiency(family string, xp float64) {
	if family == "" || xp <= 0 {
		return
	}
	maxXP := proficiencyThreshold(content.ProficiencyMax)
	idx := sort.Search(len(s.prof), func(i int) bool { return s.prof[i].Family >= family })
	if idx < len(s.prof) && s.prof[idx].Family == family {
		if s.prof[idx].XP >= maxXP {
			return
		}
		before := proficiencyLevel(s.prof[idx].XP)
		s.prof[idx].XP = math.Min(maxXP, s.prof[idx].XP+xp)
		s.announceProficiency(family, before)
		return
	}
	s.prof = append(s.prof, SavedProficiency{})
	copy(s.prof[idx+1:], s.prof[idx:])
	s.prof[idx] = SavedProficiency{Family: family, XP: math.Min(maxXP, xp)}
	s.announceProficiency(family, 0)
}

// announceProficiency logs every level crossed and re-derives the hull edge.
func (s *Sim) announceProficiency(family string, before int) {
	after := s.Proficiency(family)
	if after <= before {
		return
	}
	for level := before + 1; level <= after; level++ {
		s.log(SevGood, s.player, "Proficiency: "+content.FamilyLabel(family)+" "+content.LevelLabel(level))
	}
	if ship, ok := s.Ships.Get(s.player); ok {
		if hull, ok := s.Content.Ship(ship.Hull); ok && hull.Family == family {
			s.recomputeFit(s.player)
		}
	}
}

// proficiencyRequirement is the refusal text a gate prints: the family's
// display name and the missing level ("Requires Autocannon II").
func proficiencyRequirement(family string, gate int) string {
	return "Requires " + content.FamilyLabel(family) + " " + content.LevelLabel(gate)
}

// oreRarity is the practice paid by one mining cycle: 2^(rarity-1), so the
// authored 1..4 rarity maps to 1, 2, 4 or 8 (a def without the field pays 1).
func (s *Sim) oreRarity(ore content.ItemID) float64 {
	r := 1
	if def, ok := s.Content.Item(ore); ok && def.Rarity > 0 {
		r = def.Rarity
	}
	if r < 1 {
		r = 1
	}
	if r > 4 {
		r = 4
	}
	return math.Pow(2, float64(r-1))
}

// flownHullFamily is the class line of the player's current hull ("" when
// there is none).
func (s *Sim) flownHullFamily() string {
	ship, ok := s.Ships.Get(s.player)
	if !ok {
		return ""
	}
	hull, ok := s.Content.Ship(ship.Hull)
	if !ok {
		return ""
	}
	return hull.Family
}

// awardJob credits a tool-family act and the flown hull's share of it
// (mining, combat damage, support cycles). Kills and deliveries call
// awardProficiency directly at full hull rate.
func (s *Sim) awardJob(family string, xp float64) {
	if xp <= 0 {
		return
	}
	s.awardProficiency(family, xp)
	if hull := s.flownHullFamily(); hull != "" && hull != family {
		s.awardProficiency(hull, xp*hullPracticeShare)
	}
}

// awardSupportCycle pays one completed support-module cycle: shield and armor
// reps, cap boosters, afterburner time and web/scram application each train
// their own family (and the hull at the job share).
func (s *Sim) awardSupportCycle(e ecs.Entity, def *content.ModuleDef) {
	if e != s.player || def == nil || def.Family == "" {
		return
	}
	switch def.Kind {
	case content.KindShieldBoost, content.KindArmorRep, content.KindCapBooster,
		content.KindAfterburner, content.KindStasisWeb, content.KindWarpScram:
		s.awardJob(def.Family, supportXPPerCycle)
	}
}

// hasFittedKind reports whether e carries a module of the kind.
func (s *Sim) hasFittedKind(e ecs.Entity, kind string) bool {
	fit, ok := s.Fittings.Get(e)
	if !ok {
		return false
	}
	for _, class := range slotClasses {
		for _, id := range fit.Defs(class) {
			if def, ok := s.Content.Module(id); ok && def.Kind == kind {
				return true
			}
		}
	}
	return false
}

// awardFlightHop credits one completed warp's actual displacement to the
// flown hull: one point per 100 AU, capped at 50 AU per hop (half a point).
// An idle or station-bound hull earns nothing; a session of ordinary hopping
// is visible but minor next to a fight.
func (s *Sim) awardFlightHop(e ecs.Entity, dist float64) {
	if e != s.player || dist <= 0 {
		return
	}
	if dist > flightHopCap {
		dist = flightHopCap
	}
	if hull := s.flownHullFamily(); hull != "" {
		s.awardProficiency(hull, dist/flightXPMetres)
	}
}

// casteWeight is the kill bonus's difficulty rung. Bounty is the authored
// difficulty figure (Scuttler 7,500 -> 1, Marauder 65,000 -> 2, Hierarch
// 250,000 -> 6); combat practice itself is damage-scaled, not caste-scaled.
func (s *Sim) casteWeight(e ecs.Entity) float64 {
	ai, ok := s.AIs.Get(e)
	if !ok {
		return 1
	}
	def, ok := s.Content.Npc(ai.Npc)
	if !ok || def.Bounty <= 0 {
		return 1
	}
	w := 1 + math.Floor(def.Bounty/50000)
	if w > 6 {
		w = 6
	}
	return w
}
