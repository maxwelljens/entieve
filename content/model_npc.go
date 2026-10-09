package content

// NpcDef defines a non-player character ship archetype.
type NpcDef struct {
	ID            NpcID      `json:"id"`
	Name          string     `json:"name"`
	Faction       string     `json:"faction"`
	Ship          ShipID     `json:"ship"`
	Fit           []ModuleID `json:"fit"`
	AggroRange    float64    `json:"aggro_range"`    // m; becomes hostile when player enters
	OrbitRange    float64    `json:"orbit_range"`    // m
	FleeThreshold float64    `json:"flee_threshold"` // structure fraction 0..1; flee below this
	Bounty        float64    `json:"bounty"`         // credits to killer wallet
	Loot          []LootDrop `json:"loot"`
}
