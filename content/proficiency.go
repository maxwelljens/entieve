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

// ProfInfo is one family's page in the tree's family card: what the family
// is for and which acts pay it practice. It is authored content (CONTEXT.md:
// Proficiency), kept beside the family vocabulary because a family is code,
// and a new family is a schema change: the validator rejects a family with
// no page and the sim's test pins each practice list against the act
// constants, so prose cannot drift from the awards.
type ProfInfo struct {
	Desc     string   // what the family is for (a paragraph, no em dashes)
	Practice []string // the acts that award practice, one line each
}

// proficiencyInfo is the family card table. Every shipped family has a page
// (validate.go's proficiencyCard); a family with no page is a validator
// problem, never a blank dialog.
var proficiencyInfo = map[string]ProfInfo{
	"autocannon": {
		Desc: "Short-ranged, fast-cycling guns: the starter weapon line, quick to track and light on grid. The line covers every calibre, so practice with a small autocannon carries to the large one.",
		Practice: []string{
			"Every point of damage an autocannon lands.",
			"The hull you fly takes a 10% cut of that damage.",
		},
	},
	"artillery": {
		Desc: "Long-ranged, heavy-hitting guns that cycle slowly and punish a target that holds still. Opens once Autocannons reaches II; the line then climbs its calibres without another gate.",
		Practice: []string{
			"Every point of damage artillery lands.",
			"The hull you fly takes a 10% cut of that damage.",
		},
	},
	"rocket": {
		Desc: "Launcher-mounted rockets: short-range volleys that arrive in a burst. This proficiency covers the launchers, and its levels gate the larger bays.",
		Practice: []string{
			"Every point of damage a rocket volley lands.",
			"The hull you fly takes a 10% cut of that damage.",
		},
	},
	"mining": {
		Desc: "The mining laser: ore yield per cycle, and the gate on the larger lasers. Rarer ore pays more practice for the same cycle, so a rich belt trains faster than a common one.",
		Practice: []string{
			"Each completed mining cycle, by ore rarity: rarity 1 pays 1, 2 pays 2, 3 pays 4 and 4 pays 8.",
			"The hull you fly takes a 10% cut of that practice.",
		},
	},
	"shield": {
		Desc: "Shield boosters and shield hardeners: the recharge that keeps a hull in a fight. Its levels gate the stronger boosters.",
		Practice: []string{
			"Each completed shield booster cycle.",
			"The hull you fly takes a 10% cut of that practice.",
		},
	},
	"armor": {
		Desc: "Armor repairers and plating: the second layer that holds after the shield is gone. Levels gate the stronger repairers and the heavier plates.",
		Practice: []string{
			"Each completed armor repairer cycle.",
			"The hull you fly takes a 10% cut of that practice.",
		},
	},
	"capacitor": {
		Desc: "The capacitor: the energy every active module spends. Boosters refill it in a burst, rechargers refill it continuously, and this proficiency gates the larger of both.",
		Practice: []string{
			"Each completed cap booster cycle.",
			"Every activation cost paid while a cap recharger is fitted.",
			"The hull you fly takes a 10% cut of that practice.",
		},
	},
	"propulsion": {
		Desc: "Afterburners and the hull's speed edge: the line that closes and opens range. Levels gate the stronger afterburners.",
		Practice: []string{
			"Each completed afterburner cycle.",
			"The hull you fly takes a 10% cut of that practice.",
		},
	},
	"ewar": {
		Desc: "Electronic warfare: stasis webs that slow a target and warp scramblers that hold it. Levels gate the stronger webs and the longer scrams.",
		Practice: []string{
			"Each completed web or scrambler cycle.",
			"The hull you fly takes a 10% cut of that practice.",
		},
	},
	"frigate": {
		Desc: "The frigate class: small, fast hulls that fly the starter line. Levels gate the frigate hulls you may assemble and undock.",
		Practice: []string{
			"10% of every mining, combat damage and support cycle act you fly in a frigate.",
			"Each kill, at full rate, scaled by the target's caste.",
			"Each completed delivery, at full rate, by the offer's level.",
			"Each completed warp, 1 point per 100 AU (half a point a hop at most).",
		},
	},
	"destroyer": {
		Desc: "The destroyer class: a battery of small turrets on a frigate hull. Opens once Frigates reaches II; levels gate the destroyer hulls.",
		Practice: []string{
			"10% of every mining, combat damage and support cycle act you fly in a destroyer.",
			"Each kill, at full rate, scaled by the target's caste.",
			"Each completed delivery, at full rate, by the offer's level.",
			"Each completed warp, 1 point per 100 AU (half a point a hop at most).",
		},
	},
	"industrial": {
		Desc: "The industrial class: the hauler line, built to carry rather than fight. Opens once Frigates reaches I; levels gate the industrial hulls.",
		Practice: []string{
			"10% of every mining, combat damage and support cycle act you fly in an industrial.",
			"Each kill, at full rate, scaled by the target's caste.",
			"Each completed delivery, at full rate, by the offer's level.",
			"Each completed warp, 1 point per 100 AU (half a point a hop at most).",
		},
	},
	"barge": {
		Desc: "The barge class: the dedicated mining hull, an ore hold in place of guns. Opens once Industrials reaches I; levels gate the barge hulls.",
		Practice: []string{
			"10% of every mining, combat damage and support cycle act you fly in a barge.",
			"Each kill, at full rate, scaled by the target's caste.",
			"Each completed delivery, at full rate, by the offer's level.",
			"Each completed warp, 1 point per 100 AU (half a point a hop at most).",
		},
	},
	"rat": {
		Desc: "The Old Works hulls: the machine line flown against you, never by you. This proficiency names the enemy castes (Scuttler, Marauder, Hierarch) and carries no practice of its own.",
		Practice: []string{
			"None: the machines are flown against the pilot, so no act awards this proficiency.",
		},
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
