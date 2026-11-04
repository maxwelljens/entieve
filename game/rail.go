package game

import (
	uikit "github.com/entieve/entieve/uikit/screens"
)

// The navigation rail's derivation from the Screen spec (game/screens.go):
// one entry per openable window, grouped into the rail's sections. This
// file owns the one thing the rows cannot: the SECTION ORDER (a
// presentation decision, validated against by screenSpecErrors). The two
// always-on windows are ordinary rows now (the Persistent facet), so they
// derive their entries exactly like every other window. The Esc menu
// (transient) and the HUD/rack (permanent ship chrome) are deliberately
// absent: the pause menu keeps its key, the chrome has no window to open.
//
// railGroups is the rail's section order, top to bottom. Every
// non-transient Screen row must name one of these (screenSpecErrors),
// and railEntries draws them in this order — spec order inside a section.
var railGroups = [...]string{"Ship", "Trade", "Industry", "Station", "World", "Help & System"}

// railEntries derives the rail's table from the spec: for each section in
// railGroups, every Screen row of that section in spec order. Transient
// rows (the Esc menu) never enter the rail, and neither do the two rows
// flagged NoRail / Unlisted (Settings and Keybindings): the rail is the
// navigation, the Settings window is their door.
//
// The Open and Hint closures read the live state: the host's flags for
// every row (the modal scenes drive the same flags) and the live binding
// table for the hints (game/keyhints.go).
func railEntries(g *Game) []uikit.RailEntry {
	entries := make([]uikit.RailEntry, 0, len(screens))
	for _, group := range railGroups {
		for _, s := range screens {
			if s.DockPolicy == DockTransient || s.NoRail || s.Unlisted || s.Group != group {
				continue
			}
			row := s
			entries = append(entries, uikit.RailEntry{
				Icon:  row.Icon,
				Title: row.Title,
				// Live, local: the tooltip asks the binding table when it
				// is shown, so a rebind needs no invalidation here.
				Hint: func() string { return g.hints.Hint(row.Action) },

				Group:      row.Group,
				SpaceOnly:  row.SpaceOnly,
				DockedOnly: row.DockedOnly,
				Service:    row.Service,

				Open: func() bool { return g.host.IsOpen(row.ID) },
			})
		}
	}
	return entries
}
