package ui

import (
	"sort"
	"strconv"

	"github.com/entieve/entieve/content"
)

// AmmoCandidates returns the charge choices for a weapon slot: "" (Auto)
// first, then every distinct cargo item whose ammo class matches the
// weapon's, ordered by item id for determinism. Weapons without an ammo
// class or with no matching cargo get nil — right-click cycling is
// disabled cleanly for them (non-weapons included).
func AmmoCandidates(c *content.Content, cargo []content.CargoStack, ammoClass string) []content.ItemID {
	if ammoClass == "" || c == nil {
		return nil
	}
	seen := map[content.ItemID]bool{}
	var ids []content.ItemID
	for _, st := range cargo {
		if st.Qty <= 0 || seen[st.Item] {
			continue
		}
		def, ok := c.Item(st.Item)
		if !ok || def.AmmoClass != ammoClass {
			continue
		}
		seen[st.Item] = true
		ids = append(ids, st.Item)
	}
	if len(ids) == 0 {
		return nil
	}
	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
	out := make([]content.ItemID, 0, len(ids)+1)
	return append(append(out, ""), ids...)
}

// NextAmmoChoice advances the current charge choice through cands with
// wraparound; an unknown current choice restarts at the first candidate.
func NextAmmoChoice(cur content.ItemID, cands []content.ItemID) content.ItemID {
	if len(cands) == 0 {
		return cur
	}
	for i, id := range cands {
		if id == cur {
			return cands[(i+1)%len(cands)]
		}
	}
	return cands[0]
}

// AmmoChoiceName renders a charge choice for tooltips: "" = Auto.
func AmmoChoiceName(c *content.Content, id content.ItemID) string {
	if id == "" {
		return "Auto"
	}
	if c != nil {
		if def, ok := c.Item(id); ok {
			return def.Name
		}
	}
	return string(id)
}

// itoa formats a small non-negative int (warp ranges: 0/10/50).
func itoa(n int) string { return strconv.Itoa(n) }
