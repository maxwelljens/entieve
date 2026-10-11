package content

import "strings"

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

// IsGuard reports whether the def is a security patrol (npc-guard-*): the
// same registry prefix GuardFor keys on. Guards are friendly — they never
// initiate combat with the player, only defend when hit (sim/systemNpc);
// the aggro-range/intrusion threat triggers apply to rats alone.
func (d *NpcDef) IsGuard() bool {
	return strings.HasPrefix(string(d.ID), "npc-guard-")
}
