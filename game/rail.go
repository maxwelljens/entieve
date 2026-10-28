package game

import (
	uikit "github.com/entieve/entieve/uikit/screens"
)

// The navigation rail's derivation from the Screen spec (game/screens.go):
// one entry per openable window, grouped into the rail's sections. This
// file owns the two things the rows cannot: the SECTION ORDER (a
// presentation decision, validated against by screenSpecErrors) and the
// two core windows — Overview and Selected Item are kit-owned chrome,
// not Screen rows, but they are windows the rail opens and closes, so
// they join the Ship section through Kit.ToggleCore instead of a spec
// row. The Esc menu (transient) and the HUD/rack (permanent ship chrome)
// are deliberately absent: the pause menu keeps its key, the chrome has
// no window to open.
// railGroups is the rail's section order, top to bottom. Every
// non-transient Screen row must name one of these (screenSpecErrors),
// and railEntries draws them in this order — spec order inside a section.
var railGroups = [...]string{"Ship", "Trade", "Industry", "Station", "World", "Help & System"}

// railEntries derives the rail's table from the spec plus the two core
// windows: for each section in railGroups, the core entries that belong
// to it (in declaration order), then every Screen row of that section in
// spec order. Transient rows (the Esc menu) never enter the rail, and
// neither do the two rows flagged NoRail / Unlisted (Settings and
// Keybindings): the rail is the navigation, the Settings window is their
// door.
//
// The Open closures read the live state: the panel flags for spec rows
// (the modal scenes drive the same flags), the kit's own close state for
// the core windows. The Hint closures read the live binding table the same
// way (game/keyhints.go).
func railEntries(g *Game) []uikit.RailEntry {
	core := []uikit.RailEntry{
		{
			Icon: "kind_ship", Title: "Overview", Group: "Ship", SpaceOnly: true,
			Open: func() bool { return g.kit.CoreOpen("Overview") },
		},
		{
			Icon: "act_lock", Title: "Selected Item", Group: "Ship", SpaceOnly: true,
			Open: func() bool { return g.kit.CoreOpen("Selected Item") },
		},
	}
	entries := make([]uikit.RailEntry, 0, len(core)+len(screens))
	for _, group := range railGroups {
		for _, e := range core {
			if e.Group == group {
				entries = append(entries, e)
			}
		}
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
