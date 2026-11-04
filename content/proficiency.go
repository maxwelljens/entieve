package content

import (
	"fmt"
	"sort"
	"strings"
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
	// The two workshop lines: edges on the industry and refinery paths, no
	// gates of their own (they open at 0, like every gun line).
	"manufacturing": "Manufacturing",
	"refining":      "Refining",
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
	// The workshop lines are cross-cutting effects, not gear: they open
	// from the start and gate nothing (owner decision, gate 0).
	"manufacturing": nil,
	"refining":      nil,
}

// ProficiencyMax is the house cap (Craft level's five).
const ProficiencyMax = 5

// ProfInfo is one family's page in the tree's family card: what the family
// is for and which acts pay it practice. It is authored content (CONTEXT.md:
// Proficiency), kept beside the family vocabulary because a family is code,
// and a new family is a schema change: the validator rejects a family with
// no page and the sim's test pins each practice list against the act
// constants, so prose cannot drift from the awards.
type ProfInfo struct {
	Desc     string   // what the family is for (a paragraph, no em dashes)
	Practice []string // the acts that award practice, one line each
	// Edges is the family's effect line (the numbers and their caps) for
	// the cards that grant a cross-cutting edge. "" for a family whose
	// edge is the in-use percent its description already states.
	Edges string
}

// proficiencyInfo is the family card table. Every shipped family has a page
// (validate.go's proficiencyCard); a family with no page is a validator
// problem, never a blank dialog.
var proficiencyInfo = map[string]ProfInfo{
	"autocannon": {
		Desc: "Autocannons are short-ranged, fast-cycling guns. The line covers every calibre, so practice with a small autocannon carries to the large one.",
		Practice: []string{
			"Every point of damage an autocannon lands.",
			"The hull you fly takes a 10% cut of that damage.",
		},
	},
	"artillery": {
		Desc: "Long-ranged, heavy-hitting guns that cycle slowly and strike targets at long distances. Covers every calibre of artillery, so practise with small artillery carries to the larger types.",
		Practice: []string{
			"Every point of damage artillery lands.",
			"The hull you fly takes a 10% cut of that damage.",
		},
	},
	"rocket": {
		Desc: "Launcher-mounted rockets; short-range volleys that arrive in a burst. This proficiency covers the launchers.",
		Practice: []string{
			"Every point of damage a rocket volley lands.",
			"The hull you fly takes a 10% cut of that damage.",
		},
	},
	"mining": {
		Desc: "Mining is a proficiency governing the use of mining lasers and other ore-extracting equipment. Rarer ore pays more practice for the same cycle, so a rich belt trains faster than a common one.",
		Practice: []string{
			"Each completed mining cycle, by ore rarity: rarity 1 pays 1, 2 pays 2, 3 pays 4 and 4 pays 8.",
			"The hull you fly takes a 10% cut of that practice.",
		},
	},
	"shield": {
		Desc: "Proficiency in the usage of shield boosters and shield hardeners, the first layer of a ship's defence.",
		Practice: []string{
			"Each completed shield booster cycle.",
			"The hull you fly takes a 10% cut of that practice.",
		},
	},
	"armor": {
		Desc: "Armor repairers and plating, the second defence layer that holds after the shield.",
		Practice: []string{
			"Each completed armor repairer cycle.",
			"The hull you fly takes a 10% cut of that practice.",
		},
	},
	"capacitor": {
		Desc: "Proficiency in capacitor usage, the energy most active modules spend to operate.",
		Practice: []string{
			"Each completed cap booster cycle.",
			"Every activation cost paid while a cap recharger is fitted.",
			"The hull you fly takes a 10% cut of that practice.",
		},
	},
	"propulsion": {
		Desc: "Proficiency in using afterburners and other propulsion systems.",
		Practice: []string{
			"Each completed afterburner cycle.",
			"The hull you fly takes a 10% cut of that practice.",
		},
	},
	"ewar": {
		Desc: "Electronic warfare, such as stasis webs that slow a target and warp scramblers that bind it in place.",
		Practice: []string{
			"Each completed web or scrambler cycle.",
			"The hull you fly takes a 10% cut of that practice.",
		},
	},
	"frigate": {
		Desc: "The frigate class: small, fast hulls that fly the starter line. Levels govern the frigate hulls one may pilot.",
		Practice: []string{
			"10% of every mining, combat damage and support cycle act you fly in a frigate.",
			"Each kill, at full rate, scaled by the target's caste.",
			"Each completed delivery, at full rate, by the offer's level.",
			"Each completed warp, 1 point per 100 AU (half a point a hop at most).",
		},
	},
	"destroyer": {
		Desc: "The destroyer class: a large battery of turrets on a frigate hull. Levels govern the destroyer hulls one may pilot.",
		Practice: []string{
			"10% of every mining, combat damage and support cycle act you fly in a destroyer.",
			"Each kill, at full rate, scaled by the target's caste.",
			"Each completed delivery, at full rate, by the offer's level.",
			"Each completed warp, 1 point per 100 AU (half a point a hop at most).",
		},
	},
	"industrial": {
		Desc: "The industrial class: the hauler line, built primarily for cargo transport. Levels govern the industrial hulls one may pilot.",
		Practice: []string{
			"10% of every mining, combat damage and support cycle act you fly in an industrial.",
			"Each kill, at full rate, scaled by the target's caste.",
			"Each completed delivery, at full rate, by the offer's level.",
			"Each completed warp, 1 point per 100 AU (half a point a hop at most).",
		},
	},
	"barge": {
		Desc: "The barge class: the dedicated mining hull, usually with a large ore hold in place of regular cargohold. Levels govern the barge hulls one may pilot.",
		Practice: []string{
			"10% of every mining, combat damage and support cycle act you fly in a barge.",
			"Each kill, at full rate, scaled by the target's caste.",
			"Each completed delivery, at full rate, by the offer's level.",
			"Each completed warp, 1 point per 100 AU (half a point a hop at most).",
		},
	},
	"rat": {
		Desc: "The Old Works hulls, hulls of the rogue machines.",
		Practice: []string{
			"N/A",
		},
	},
	"manufacturing": {
		Desc: "Manufacturing is the workshop line: every job you run at an industry station teaches it. Levels cut what a job takes out of your storage, and on a batch recipe (ammunition, charges) they add to what each run makes.",
		Practice: []string{
			"Each completed manufacturing job, by product rarity: rarity 1 pays 1, 2 pays 2, 3 pays 4 and 4 pays 8.",
			"Only the jobs you start yourself count, and the product pays, not the recipe's family.",
		},
		Edges: "Materials: 2% less per level, capped at 10% at V, stacking additively with the station's own craft discount. Output: 2% more per level, capped at 10% at V, on recipes that make more than one unit per run.",
	},
	"refining": {
		Desc: "Refining is the refinery line: every whole batch of 100 ore you convert into minerals teaches it. Levels add to the minerals a batch yields, so the same ore comes out richer.",
		Practice: []string{
			"Each completed refine batch, by ore rarity: rarity 1 pays 1, 2 pays 2, 3 pays 4 and 4 pays 8.",
		},
		Edges: "Yield: 2% more minerals per level, capped at 10% at V, on every mineral a batch yields. The ore a batch eats does not change.",
	},
}

// FamilyInfo returns a family's card (the zero value for an unknown family;
// Validate refuses a family without one).
func FamilyInfo(family string) ProfInfo { return proficiencyInfo[family] }

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

// ProficiencyRequirement is the one sentence a proficiency gate prints: the
// family's display name and the level it needs ("Requires Autocannon II").
// The sim's fit/fly refusal, the tech-tree node, the family card and Show
// Info all read it, so no two surfaces can word the same gate differently.
func ProficiencyRequirement(family string, gate int) string {
	return "Requires " + FamilyLabel(family) + " " + LevelLabel(gate)
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

// proficiencyCardProblems validates the family cards: every known family
// has a page, every page has a description and at least one practice line,
// no line is blank, and the prose carries no em dash (the house rule).
func proficiencyCardProblems(cards map[string]ProfInfo, families map[string]string) []string {
	var problems []string
	ids := make([]string, 0, len(families))
	for id := range families {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	for _, id := range ids {
		card, ok := cards[id]
		if !ok {
			problems = append(problems, fmt.Sprintf("proficiency: family %q has no card (description and practice)", id))
			continue
		}
		if strings.TrimSpace(card.Desc) == "" {
			problems = append(problems, fmt.Sprintf("proficiency: family %q has no description", id))
		}
		if strings.Contains(card.Desc, "\u2014") {
			problems = append(problems, fmt.Sprintf("proficiency: family %q description carries an em dash", id))
		}
		if len(card.Practice) == 0 {
			problems = append(problems, fmt.Sprintf("proficiency: family %q lists no practice", id))
		}
		if strings.Contains(card.Edges, "\u2014") {
			problems = append(problems, fmt.Sprintf("proficiency: family %q edges line carries an em dash", id))
		}
		for i, line := range card.Practice {
			if strings.TrimSpace(line) == "" {
				problems = append(problems, fmt.Sprintf("proficiency: family %q practice line %d is blank", id, i))
			}
			if strings.Contains(line, "\u2014") {
				problems = append(problems, fmt.Sprintf("proficiency: family %q practice line %d carries an em dash", id, i))
			}
		}
	}
	return problems
}

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
