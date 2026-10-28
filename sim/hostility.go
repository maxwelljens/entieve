package sim

// Faction strings as content ships them (content/data/npcs/*.json) and as
// docs/lore/FACTIONS.md names them. The relations below are keyed on these
// strings: data is author-owned, so a new faction must gain a row here and a
// test fails until it does (sim/hostility_test.go).
const (
	factionCharter     = "Charter Authority"
	factionOldWorks    = "Old Works"
	factionDvuzek      = "Dvužek"
	factionIndependent = "independents" // the player and its traffic haulers: no flag
	// factionFleets is the Corporate Fleets bloc — the commissioned war
	// fleets. One row now; per-corp rows may grow when a corp needs its name
	// on screen.
	factionFleets = "Corporate Fleets"
)

// hostility is FACTIONS.md's Relations table as code: faction -> the factions
// it may act against. The Old Works raid outward (material-driven: every
// non-Old-Works hull is salvage) and the Charter answers them (attrition); the
// Corporate Fleets fight the Old Works under the Commission and nothing else;
// the corporate houses write their losses off, and independents start nothing.
// Same-faction and unknown factions are never hostile (fail closed): a missing
// row means "neutral", never "hostile by default".
var hostility = map[string]map[string]bool{
	factionOldWorks:    {factionCharter: true, factionDvuzek: true, factionIndependent: true, factionFleets: true},
	factionCharter:     {factionOldWorks: true},
	factionFleets:      {factionOldWorks: true},
	factionDvuzek:      {},
	factionIndependent: {},
}

// hostile reports whether from may initiate on to.
func hostile(from, to string) bool {
	if from == to {
		return false
	}
	return hostility[from][to]
}
