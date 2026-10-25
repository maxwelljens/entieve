package game

// creditSection is one block of the title screen's credits window. The
// text is authored from the in-repo provenance (LICENSE, uikit/icons/
// CREDITS.md, uikit/hulls + uikit/emblems manifests, docs/lore/
// AESTHETIC.md, go.mod); game/credits_test.go pins the strings that could
// drift from those sources.
type creditSection struct {
	Title string
	Lines []string
}

var creditSections = []creditSection{
	{
		Title: "Entieve",
		Lines: []string{
			"MIT License, © 2026 The Entieve Authors.",
		},
	},
	{
		Title: "Icons",
		Lines: []string{
			"Icons made by Delapouite, Skoll, Lorc, sbed, Cathelineau and Quoting. Available on https://game-icons.net",
			"CC BY 3.0 — see uikit/icons/CREDITS.md for the per-icon credits.",
		},
	},
	{
		Title: "Module renders",
		Lines: []string{
			"Five AC/artillery module renders (autocannon S/M, artillery S/M/L), CC0 1.0 / public domain.",
			"Source: github.com/maxwelljens/entieve.",
		},
	},
	{
		Title: "Hull & emblem artwork",
		Lines: []string{
			"Original artwork by the Entieve authors (uikit/hulls and uikit/emblems manifests).",
		},
	},
	{
		Title: "Interface form",
		Lines: []string{
			"Chrome shapes and structure follow the Cyber Terminal UI Kit (kit by Kerem Bolek, 2026; form vocabulary only — no kit file is shipped, and no kit colour is used).",
		},
	},
	{
		Title: "Typefaces",
		Lines: []string{
			"IBM Plex Sans and IBM Plex Mono, © 2017 IBM Corp, SIL Open Font License, Version 1.1.",
		},
	},
	{
		Title: "Built with",
		Lines: []string{
			"Go (BSD-3-Clause) · Ebitengine (Apache-2.0)",
			"ebitenui (MIT) · sahilm/fuzzy (MIT)",
			"Licences upstream.",
		},
	},
}
