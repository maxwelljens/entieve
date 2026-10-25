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
// spec order. Transient rows (the Esc menu) never enter the rail.
//
// The Open closures read the live state: the panel flags for spec rows
// (the modal scenes drive the same flags), the kit's own close state for
// the core windows.
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
			if s.DockPolicy == DockTransient || s.Group != group {
				continue
			}
			row := s
			entries = append(entries, uikit.RailEntry{
				Icon:  row.Icon,
				Title: row.Title,
				Key:   keyLabel(row),
				Desc:  row.Desc,

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

// keyLabel is a row's display key for the rail tooltip ("" = unkeyed;
// the tooltip's hint line is the row's Desc, which already reads
// "Shift+L — toggle log window").
func keyLabel(s Screen) string {
	if s.Key == keyNone {
		return ""
	}
	return s.Key.String()
}
