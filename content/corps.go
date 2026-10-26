package content

// Corporation: a named manufacturing house — the layer that builds the
// goods the market sells (CONTEXT.md, "Corporation"). It is not a power
// bloc: corporations own no territory, hold no law and fly no warships
// (docs/lore/FACTIONS.md); they are independent suppliers whose emblems
// appear on the goods they make.
//
// Every manufactured def — ShipDef, ModuleDef and ammo/charge ItemDefs —
// carries a `maker` slug from this roster; rat hulls, ores, minerals and
// loot stay unattributed (empty maker). The emblems live in
// uikit/emblems, keyed by the same slug.
//
// Registry: docs/lore/NAMING.md, APPROVED 2026-10-18. Names are binding.

// Corp is one manufacturer: the registry record behind a def's maker slug.
type Corp struct {
	ID       string // corp-* slug: the value a def's maker field carries
	Name     string // display name (shown in Show Info)
	Heritage string // naming-layer heritage (docs/lore/NAMING.md)
}

// AllCorps is the ten-firm manufacturer roster — the maker vocabulary.
var AllCorps = []Corp{
	{ID: "corp-borealis", Name: "Bůral", Heritage: "nordic"},
	{ID: "corp-nordvakt", Name: "Noldvar Guard", Heritage: "nordic"},
	{ID: "corp-iskald", Name: "Iškoven", Heritage: "nordic"},
	{ID: "corp-ferrum", Name: "Kumbřan Steelworks", Heritage: "polish"},
	{ID: "corp-druzstvo", Name: "Dvužek", Heritage: "moravian"},
	{ID: "corp-burza", Name: "Bríždan Factory", Heritage: "polish-moravian"},
	{ID: "corp-celeritas", Name: "Cylvar", Heritage: "latinate"},
	{ID: "corp-praetor", Name: "Preštor", Heritage: "latinate"},
	{ID: "corp-aldemar", Name: "Alďmar", Heritage: "indistinct-european"},
	{ID: "corp-vantera", Name: "Vantera", Heritage: "indistinct-european"},
}

// corpByID indexes AllCorps by slug.
var corpByID = func() map[string]Corp {
	m := make(map[string]Corp, len(AllCorps))
	for _, c := range AllCorps {
		m[c.ID] = c
	}
	return m
}()

// CorpByID returns the manufacturer with the given slug.
func CorpByID(id string) (Corp, bool) {
	c, ok := corpByID[id]
	return c, ok
}

// CorpName returns a maker slug's display name, or "" when unknown (a
// maker-less def resolves to "" too).
func CorpName(id string) string {
	return corpByID[id].Name
}
