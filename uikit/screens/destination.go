package screens

import (
	"sort"

	"github.com/entieve/entieve/content"
	"github.com/entieve/entieve/uikit/widgets"
)

// The destination read: the Starmap's planned system, its route and the
// gate the route leaves by. It is client-side only, so the sim never sees
// it; the map, the HUD readout and the Overview's gate tint share these
// walks so they can never disagree about the next hop.

// routePath is the shortest gate path from cur to dest, both ends included
// (nil when either is unknown, they are equal, or no gate chain connects
// them). Gates are authored both ways; the reverse edge is added so a
// one-way slip can never hide a route the player can actually fly.
func routePath(cv widgets.ContentView, cur, dest content.SystemID) []content.SystemID {
	if cv == nil || cv.Defs() == nil || cur == "" || dest == "" || cur == dest {
		return nil
	}
	defs := cv.Defs()
	ids := make([]content.SystemID, 0, len(defs.Systems))
	for id := range defs.Systems {
		ids = append(ids, id)
	}
	// Canonical order keeps equal-length routes stable, so the graph and
	// the readout pick the same next hop every frame.
	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
	adj := make(map[content.SystemID][]content.SystemID, len(ids))
	for _, id := range ids {
		def, ok := defs.System(id)
		if !ok {
			continue
		}
		for i := range def.Celestials {
			if g := def.Celestials[i].Gate; g != nil {
				adj[id] = append(adj[id], g.ToSystem)
				adj[g.ToSystem] = append(adj[g.ToSystem], id)
			}
		}
	}
	prev := make(map[content.SystemID]content.SystemID, len(ids))
	seen := map[content.SystemID]bool{cur: true}
	queue := []content.SystemID{cur}
	for len(queue) > 0 {
		a := queue[0]
		queue = queue[1:]
		if a == dest {
			path := []content.SystemID{dest}
			for path[0] != cur {
				path = append([]content.SystemID{prev[path[0]]}, path...)
			}
			return path
		}
		for _, b := range adj[a] {
			if !seen[b] {
				seen[b] = true
				prev[b] = a
				queue = append(queue, b)
			}
		}
	}
	return nil
}

// gateTo is the current system's gate toward to ("" = none): the authored
// celestial a route leaves by and the Overview tints.
func gateTo(cv widgets.ContentView, system, to content.SystemID) (content.CelID, bool) {
	if cv == nil || cv.Defs() == nil {
		return "", false
	}
	def, ok := cv.Defs().System(system)
	if !ok {
		return "", false
	}
	for i := range def.Celestials {
		if g := def.Celestials[i].Gate; g != nil && g.ToSystem == to {
			return def.Celestials[i].ID, true
		}
	}
	return "", false
}
