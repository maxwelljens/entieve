package content

import (
	"fmt"
	"sort"
)

// Proficiency family vocabulary (CONTEXT.md: Proficiency). A family is a
// practice line, not a single def: small, medium and large autocannons share
// one line, so practicing the starter tier opens the next. The vocabulary is
// code, like the recipe families: new families are a schema change.
var proficiencyFamilies = map[string]string{
	"autocannon": "Autocannon",
	"artillery":  "Artillery",
	"rocket":     "Rocket",
	"mining":     "Mining",
	"shield":     "Shield",
	"armor":      "Armor",
	"capacitor":  "Capacitor",
	"propulsion": "Propulsion",
	"ewar":       "Electronic warfare",
	"frigate":    "Frigate",
	"destroyer":  "Destroyer",
	"industrial": "Industrial",
	"barge":      "Barge",
	"rat":        "Rat hull",
}

// Prereq is one authored prerequisite edge: the family must be at level or
// above before the dependent family's use gate opens.
type Prereq struct {
	Family string
	Level  int
}

// proficiencyPrereqs is the authored dependency graph (CONTEXT.md:
// Proficiency). Every family appears, even with no edges. The gun lines carry
// their calibre steps as gates inside the line itself (medium/large tiers), so
// the cross-family edges are the ones the content can honestly express:
// artillery needs the gun line, hull classes need the previous class, and the
// barge edge is trivial because the showcase tour must fly it from the start.
var proficiencyPrereqs = map[string][]Prereq{
	"autocannon": nil,
	"artillery":  {{Family: "autocannon", Level: 2}},
	"rocket":     nil,
	"mining":     nil,
	"shield":     nil,
	"armor":      nil,
	"capacitor":  nil,
	"propulsion": nil,
	"ewar":       nil,
	"frigate":    nil,
	"destroyer":  {{Family: "frigate", Level: 2}},
	"industrial": {{Family: "frigate", Level: 1}},
	"barge":      {{Family: "industrial", Level: 0}},
	"rat":        nil,
}

// ProficiencyMax is the house cap (Craft level's five).
const ProficiencyMax = 5

// ValidProficiencyFamily reports whether family is in the vocabulary.
func ValidProficiencyFamily(family string) bool {
	_, ok := proficiencyFamilies[family]
	return ok
}

// FamilyLabel is a family's display name (the raw id for an unknown family).
func FamilyLabel(family string) string {
	if label, ok := proficiencyFamilies[family]; ok {
		return label
	}
	return family
}

// ProficiencyFamilyIDs returns every family id in ascending order (the
// canonical order of the read API and the tech-tree screen).
func ProficiencyFamilyIDs() []string {
	ids := make([]string, 0, len(proficiencyFamilies))
	for id := range proficiencyFamilies {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	return ids
}

// Prerequisites returns a family's authored prerequisite edges as a copy
// (authored order; empty for a root family).
func Prerequisites(family string) []Prereq {
	return append([]Prereq(nil), proficiencyPrereqs[family]...)
}

// proficiencyGraphProblems validates the authored graph: every known family
// has a table entry, every edge resolves to a known family and a level in
// 0..5, and there is no cycle. It returns human-readable problems (empty =
// clean), the same shape the content validator reports.
func proficiencyGraphProblems(table map[string][]Prereq, families map[string]string) []string {
	var problems []string
	ids := make([]string, 0, len(families))
	for id := range families {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	for _, id := range ids {
		edges, ok := table[id]
		if !ok {
			problems = append(problems, fmt.Sprintf("proficiency: family %q has no prerequisite entry", id))
			continue
		}
		for _, p := range edges {
			if _, ok := families[p.Family]; !ok {
				problems = append(problems, fmt.Sprintf("proficiency: %q requires unknown family %q", id, p.Family))
			}
			if p.Level < 0 || p.Level > ProficiencyMax {
				problems = append(problems, fmt.Sprintf("proficiency: %q requires %q at level %d, must be 0..%d", id, p.Family, p.Level, ProficiencyMax))
			}
		}
	}

	const (
		white = 0
		grey  = 1
		black = 2
	)
	state := make(map[string]int, len(families))
	var visit func(string) bool
	visit = func(id string) bool {
		switch state[id] {
		case grey:
			return true
		case black:
			return false
		}
		state[id] = grey
		for _, p := range table[id] {
			if _, ok := families[p.Family]; !ok {
				continue
			}
			if visit(p.Family) {
				return true
			}
		}
		state[id] = black
		return false
	}
	for _, id := range ids {
		if visit(id) {
			problems = append(problems, fmt.Sprintf("proficiency: prerequisite cycle through %q", id))
			break
		}
	}
	return problems
}

// LevelLabel is a proficiency level as the interface and refusals print it:
// 0 → "0", 1..5 → roman numerals (the cap is five).
func LevelLabel(level int) string {
	switch level {
	case 1:
		return "I"
	case 2:
		return "II"
	case 3:
		return "III"
	case 4:
		return "IV"
	case 5:
		return "V"
	}
	return "0"
}
