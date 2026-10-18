package screens

import (
	"image"

	"github.com/ebitenui/ebitenui/widget"
	"github.com/entieve/entieve/uikit/theme"
)

// Help window geometry: two columns of key rows plus the sensor blurb. The
// key column is fixed so every row's description starts on the same x.
const (
	helpW      = 900                                            // window width (two columns; wide enough for the legacy key descriptions to stay on one line)
	helpKeyW   = 80                                             // key column width
	helpColGap = 16                                             // gap between the two key columns
	helpRowGap = 8                                              // gap between a key and its description
	helpLeadW  = (helpW - 2*theme.ContentsPad - helpColGap) / 2 // one column
	helpDescW  = helpLeadW - helpKeyW - helpRowGap              // description wrap
	helpStartY = 24                                             // default y (legacy HelpRect)
)

// KeyRow is one row of the key map: the key and what it does. The game
// passes its own game.KeyMap data through this type — uikit never imports
// game (that would be circular).
type KeyRow struct {
	Key  string
	Desc string
}

// HelpPanel is the key-map window: the rows render as a two-column table
// above a short "Sensors and visibility" blurb.
type HelpPanel struct {
	th    theme.Theme
	win   *widget.Window
	blurb *widget.Text
}

// NewHelp builds the help window over the given environment and key map.
// The window sizes itself to the rows; Sync is a no-op (the content is
// static for the panel's lifetime).
func NewHelp(d theme.Theme, rows []KeyRow) *HelpPanel {
	p := d.Pal()
	h := &HelpPanel{th: d}
	faceTiny := d.Face(theme.StepTiny)

	newRow := func(k KeyRow) *widget.Container {
		key := widget.NewText(
			widget.TextOpts.Text(k.Key, faceTiny, theme.Solid(p.Accent)),
			widget.TextOpts.WidgetOpts(widget.WidgetOpts.MinSize(theme.Px(helpKeyW), 0)),
		)
		desc := widget.NewText(
			widget.TextOpts.Text(k.Desc, faceTiny, theme.Solid(p.Text)),
			widget.TextOpts.MaxWidth(float64(theme.Px(helpDescW))),
		)
		box := widget.NewContainer(
			widget.ContainerOpts.Layout(widget.NewRowLayout(
				widget.RowLayoutOpts.Direction(widget.DirectionHorizontal),
				widget.RowLayoutOpts.Spacing(theme.Px(helpRowGap)),
			)),
		)
		box.AddChild(key, desc)
		return box
	}
	newColumn := func() *widget.Container {
		return widget.NewContainer(
			widget.ContainerOpts.Layout(widget.NewRowLayout(
				widget.RowLayoutOpts.Direction(widget.DirectionVertical),
				widget.RowLayoutOpts.Spacing(theme.Px(2)),
			)),
		)
	}

	perCol := (len(rows) + 1) / 2
	left, right := newColumn(), newColumn()
	for i, k := range rows {
		if i < perCol {
			left.AddChild(newRow(k))
		} else {
			right.AddChild(newRow(k))
		}
	}
	columns := widget.NewContainer(
		widget.ContainerOpts.Layout(widget.NewRowLayout(
			widget.RowLayoutOpts.Direction(widget.DirectionHorizontal),
			widget.RowLayoutOpts.Spacing(theme.Px(helpColGap)),
		)),
	)
	columns.AddChild(left, right)

	heading := widget.NewText(widget.TextOpts.Text("Sensors and visibility", h.th.Face(theme.StepSmall), theme.Solid(p.Accent)))
	blurb := widget.NewText(
		widget.TextOpts.Text(
			"Entieve has no omniscience: the client only shows what your ship could perceive. "+
				"Ships, asteroids and wrecks are visible only within the 250 km sensor radius; "+
				"system landmarks are always visible. The scanner (C) sweeps to 3 × sensor range "+
				"(750 km) and reports anonymous counts only — no names, no distances. "+
				"Warp-to-within (Z) picks the landing range; travel still means flying to the gate.",
			faceTiny, theme.Solid(p.Dim)),
		widget.TextOpts.MaxWidth(float64(theme.Px(helpW-2*theme.ContentsPad))),
	)
	h.blurb = blurb

	body := widget.NewContainer(
		widget.ContainerOpts.BackgroundImage(theme.Bordered(theme.Solid(p.BG), theme.NRGBA(p.Border, 200), 1)),
		widget.ContainerOpts.Layout(widget.NewRowLayout(
			widget.RowLayoutOpts.Direction(widget.DirectionVertical),
			widget.RowLayoutOpts.Spacing(theme.Px(10)),
			widget.RowLayoutOpts.Padding(widget.NewInsetsSimple(theme.Px(theme.ContentsPad))),
		)),
	)
	if len(rows) > 0 {
		body.AddChild(columns)
	}
	body.AddChild(heading, blurb)

	titleBar, _ := theme.NewTitleBar(d, "Help — Entieve key map", "svc_help")

	// Size the window to the content so a short key map never leaves the
	// blurb floating in dead space (bw/bh arrive already scaled; the
	// design-constant floors go through Px exactly once here).
	bw, bh := body.PreferredSize()
	w := max(theme.Px(helpW), bw)
	wh := theme.Px(theme.TitleBarH) + bh
	// Default: centered for the 1024 px minimum width, 24 px from the
	// top like the legacy help overlay, with the left edge floored clear
	// of the rail's gutter (10+RailInset) when the centered x would land
	// inside it; the integrator re-centers it under the same floor.
	x0 := max(theme.Px(10+RailInset), (theme.Px(1024)-w)/2)
	h.win = widget.NewWindow(
		widget.WindowOpts.Contents(body),
		widget.WindowOpts.TitleBar(titleBar, theme.Px(theme.TitleBarH)),
		widget.WindowOpts.MinSize(theme.Px(400), theme.Px(200)),
		// Default: centered for the 1024 px minimum width, 24 px from the
		// top like the legacy help overlay; the integrator re-centers it.
		widget.WindowOpts.Location(image.Rect(x0, theme.Px(helpStartY), x0+w, theme.Px(helpStartY)+wh)),
	)
	return h
}

// Win returns the help window for the interface to add.
func (h *HelpPanel) Win() *widget.Window { return h.win }

// Sync is a no-op: the key map and blurb are fixed at construction.
func (h *HelpPanel) Sync() {}
