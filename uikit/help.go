package uikit

import (
	"image"

	"github.com/ebitenui/ebitenui/widget"
)

// Help window geometry: two columns of key rows plus the sensor blurb. The
// key column is fixed so every row's description starts on the same x.
const (
	helpW      = 900                                      // window width (two columns; wide enough for the legacy key descriptions to stay on one line)
	helpKeyW   = 80                                       // key column width
	helpColGap = 16                                       // gap between the two key columns
	helpRowGap = 8                                        // gap between a key and its description
	helpLeadW  = (helpW - 2*contentsPad - helpColGap) / 2 // one column
	helpDescW  = helpLeadW - helpKeyW - helpRowGap        // description wrap
	helpStartY = 24                                       // default y (legacy HelpRect)
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
	env   Env
	win   *widget.Window
	blurb *widget.Text
}

// NewHelp builds the help window over the given environment and key map.
// The window sizes itself to the rows; Sync is a no-op (the content is
// static for the panel's lifetime).
func NewHelp(env Env, rows []KeyRow) *HelpPanel {
	p := env.Pal
	h := &HelpPanel{env: env}
	faceTiny := env.Face(StepTiny)

	newRow := func(k KeyRow) *widget.Container {
		key := widget.NewText(
			widget.TextOpts.Text(k.Key, faceTiny, solid(p.Accent)),
			widget.TextOpts.WidgetOpts(widget.WidgetOpts.MinSize(helpKeyW, 0)),
		)
		desc := widget.NewText(
			widget.TextOpts.Text(k.Desc, faceTiny, solid(p.Text)),
			widget.TextOpts.MaxWidth(helpDescW),
		)
		box := widget.NewContainer(
			widget.ContainerOpts.Layout(widget.NewRowLayout(
				widget.RowLayoutOpts.Direction(widget.DirectionHorizontal),
				widget.RowLayoutOpts.Spacing(helpRowGap),
			)),
		)
		box.AddChild(key, desc)
		return box
	}
	newColumn := func() *widget.Container {
		return widget.NewContainer(
			widget.ContainerOpts.Layout(widget.NewRowLayout(
				widget.RowLayoutOpts.Direction(widget.DirectionVertical),
				widget.RowLayoutOpts.Spacing(2),
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
			widget.RowLayoutOpts.Spacing(helpColGap),
		)),
	)
	columns.AddChild(left, right)

	heading := widget.NewText(widget.TextOpts.Text("Sensors and visibility", env.Face(StepSmall), solid(p.Accent)))
	blurb := widget.NewText(
		widget.TextOpts.Text(
			"Entieve has no omniscience: the client only shows what your ship could perceive. "+
				"Ships, asteroids and wrecks are visible only within the 250 km sensor radius; "+
				"system landmarks are always visible. The scanner (C) sweeps to 3 × sensor range "+
				"(750 km) and reports anonymous counts only — no names, no distances. "+
				"Warp-to-within (Z) picks the landing range; travel still means flying to the gate.",
			faceTiny, solid(p.Dim)),
		widget.TextOpts.MaxWidth(helpW-2*contentsPad),
	)
	h.blurb = blurb

	body := widget.NewContainer(
		widget.ContainerOpts.BackgroundImage(bordered(solid(p.BG), nrgba(p.Border, 200), 1)),
		widget.ContainerOpts.Layout(widget.NewRowLayout(
			widget.RowLayoutOpts.Direction(widget.DirectionVertical),
			widget.RowLayoutOpts.Spacing(10),
			widget.RowLayoutOpts.Padding(widget.NewInsetsSimple(contentsPad)),
		)),
	)
	if len(rows) > 0 {
		body.AddChild(columns)
	}
	body.AddChild(heading, blurb)

	titleBar := widget.NewContainer(
		widget.ContainerOpts.BackgroundImage(bordered(solid(p.BGHard), nrgba(p.Border, 200), 1)),
		widget.ContainerOpts.Layout(widget.NewAnchorLayout()),
	)
	titleBar.AddChild(widget.NewText(
		widget.TextOpts.Text("Help — Entieve key map", env.Face(StepBase), solid(p.Accent)),
		widget.TextOpts.WidgetOpts(widget.WidgetOpts.LayoutData(widget.AnchorLayoutData{
			HorizontalPosition: widget.AnchorLayoutPositionStart,
			VerticalPosition:   widget.AnchorLayoutPositionCenter,
			Padding:            widget.NewInsetsSimple(contentsPad),
		})),
	))

	// Size the window to the content so a short key map never leaves the
	// blurb floating in dead space.
	bw, bh := body.PreferredSize()
	w := max(helpW, bw)
	wh := titleBarH + bh
	h.win = widget.NewWindow(
		widget.WindowOpts.Contents(body),
		widget.WindowOpts.TitleBar(titleBar, titleBarH),
		widget.WindowOpts.Draggable(),
		widget.WindowOpts.MinSize(400, 200),
		// Default: centered for the 1024 px minimum width, 24 px from the
		// top like the legacy help overlay; the integrator re-centers it.
		widget.WindowOpts.Location(image.Rect(max(10, (1024-w)/2), helpStartY, max(10, (1024-w)/2)+w, helpStartY+wh)),
	)
	makeEdgeResizable(h.win) // four-edge resize
	return h
}

// Win returns the help window for the interface to add.
func (h *HelpPanel) Win() *widget.Window { return h.win }

// Sync is a no-op: the key map and blurb are fixed at construction.
func (h *HelpPanel) Sync() {}
