package game

// creditSection is one block of the title screen's credits window: a
// tracked section header and its role/credit rows. The text is authored
// from the in-repo provenance (LICENSE, uikit/icons/CREDITS.md, uikit/
// hulls + uikit/emblems manifests, docs/lore/AESTHETIC.md, go.mod);
// game/credits_test.go pins the strings that could drift from those
// sources.
type creditSection struct {
	Title string
	Rows  []creditRow
}

// creditRow is one line of a section: a short role in the left column and
// the credit itself.
type creditRow struct {
	Role string
	Text string
}

var creditSections = []creditSection{
	{
		Title: "Development",
		Rows: []creditRow{
			{Role: "Project", Text: "Entieve"},
			{Role: "Licence", Text: "European Union Public Licence 1.2, © 2026 The Entieve Authors."},
		},
	},
	{
		Title: "Iconography",
		Rows: []creditRow{
			{Role: "Glyph set", Text: "game-icons.net — monochrome silhouettes, CC BY 3.0."},
			{Role: "Icons", Text: "Delapouite, Skoll, Lorc, sbed, Cathelineau and Quoting."},
			{Role: "Credits", Text: "Available on https://game-icons.net — see uikit/icons/CREDITS.md for the per-icon credits."},
		},
	},
	{
		Title: "Module renders",
		Rows: []creditRow{
			{Role: "Renders", Text: "Five AC/artillery module renders (autocannon S/M, artillery S/M/L), CC0 1.0 / public domain."},
			{Role: "Source", Text: "github.com/maxwelljens/entieve."},
		},
	},
	{
		Title: "Hull & emblem artwork",
		Rows: []creditRow{
			{Role: "Artwork", Text: "Original artwork by the Entieve authors (uikit/hulls and uikit/emblems manifests)."},
		},
	},
	{
		Title: "Interface form",
		Rows: []creditRow{
			{Role: "Form", Text: "Chrome shapes and structure follow the Cyber Terminal UI Kit (kit by Kerem Bolek, 2026; form vocabulary only — no kit file is shipped, and no kit colour is used)."},
		},
	},
	{
		Title: "Typefaces",
		Rows: []creditRow{
			{Role: "Faces", Text: "IBM Plex Sans and IBM Plex Mono, © 2017 IBM Corp."},
			{Role: "Licence", Text: "SIL Open Font License, Version 1.1."},
		},
	},
	{
		Title: "Built with",
		Rows: []creditRow{
			{Role: "Engine", Text: "Go (BSD-3-Clause) · Ebitengine (Apache-2.0)"},
			{Role: "Libraries", Text: "ebitenui (MIT) · sahilm/fuzzy (MIT)"},
			{Role: "Licences", Text: "Licences upstream."},
		},
	},
}
