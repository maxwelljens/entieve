package game

// Key hint: what one action's key line says on the surface that shows it —
// the chord, the description, or the two composed. Every read goes through
// the live binding table, so a rebind shows on the help overlay, the rail's
// tooltip, the keybindings chip and the settings footer without any of them
// being rebuilt or invalidated; the fixed chord layer answers from its
// literals and never looks at g.binds.
type keyHints struct{ g *Game }

// Chord is the action's live chord ("U", "Shift+L", "Esc", "F1"), the
// fixed layer's keys joined ("[ · -"). "" means no chord: the action is
// unbound (replace policy took its key), fixed-but-unknown, or unknown —
// callers that print a dash (the keybindings chip) add it themselves, the
// settings footer drops its clause.
func (h keyHints) Chord(action string) string {
	if action == "" {
		return ""
	}
	if _, fixed := fixedActions[action]; fixed {
		for _, a := range fixedKeyRows() {
			if a.ID == action {
				return a.Keys
			}
		}
		return ""
	}
	b, bound := h.g.binds[action]
	if !bound {
		return ""
	}
	return chordOf(b.Key, b.Shift, false)
}

// Desc is the action's description only ("toggle inventory").
func (h keyHints) Desc(action string) string { return descOf(action) }

// Hint is the whole tooltip line: "chord — description", or the explicit
// "description · unbound" form while the action has no key. "" means there
// is nothing to say (an action no table documents, a keyless Screen row).
func (h keyHints) Hint(action string) string {
	desc := h.Desc(action)
	switch c := h.Chord(action); {
	case desc == "":
		return ""
	case c == "":
		return desc + " · unbound"
	default:
		return c + " — " + desc
	}
}

// descOf is an action's description-only text, read from the declaration
// tables (never from KeyMap, whose Desc carries the chord in front).
func descOf(action string) string {
	if action == "" {
		return ""
	}
	for _, b := range worldKeyMap {
		if b.Action == action {
			return b.Desc
		}
	}
	for _, s := range screens {
		if s.Action == action {
			return s.Desc
		}
	}
	return ""
}
