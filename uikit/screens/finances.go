package screens

import (
	"fmt"
	"image"
	"image/color"

	"github.com/ebitenui/ebitenui/widget"
	"github.com/entieve/entieve/sim"
	"github.com/entieve/entieve/uikit/theme"
	"github.com/entieve/entieve/uikit/widgets"
)

// Finances is the wallet window: a holdings header — spendable cash, the
// capital deployed in open buy orders, the credits accepted contracts
// still owe, and their total — over the transaction ledger, one row per
// wallet movement exactly as the sim recorded it (newest first).
//
// The three pools are all the player's money, in the three places it can
// sit: cash is the wallet, deployed is escrowed and out of it (refundable
// in full on cancel — the broker fee charged at placement is not), and
// receivables are earned but not yet paid.
type Finances struct {
	cv  widgets.ContentView
	win *widget.Window

	cash, deployed, receivables, total *widget.Text
	tbl                                *widgets.Table
}

// NewFinances builds the Finances window over the shared environment.
// Mount it on a Kit with Kit.AddWindow and call Sync every frame.
func NewFinances(env widgets.Seams) *Finances {
	f := &Finances{cv: env}
	p := env.Pal()

	// Holdings block: label/value lines, the values in the tabular
	// contents face so the digits do not jitter while money moves.
	head := widget.NewContainer(
		widget.ContainerOpts.Layout(widget.NewRowLayout(
			widget.RowLayoutOpts.Direction(widget.DirectionVertical),
			widget.RowLayoutOpts.Spacing(theme.Px(2)),
		)),
	)
	metric := func(label string, tint color.RGBA) *widget.Text {
		value := widget.NewText(widget.TextOpts.Text(
			theme.FormatCredits(0), env.FaceMono(theme.StepSmall), theme.Solid(tint)))
		line := widget.NewContainer(widget.ContainerOpts.Layout(widget.NewRowLayout(
			widget.RowLayoutOpts.Direction(widget.DirectionHorizontal),
			widget.RowLayoutOpts.Spacing(theme.Px(theme.ContentsPad)),
		)))
		line.AddChild(widget.NewText(widget.TextOpts.Text(
			label, env.Face(theme.StepSmall), theme.Solid(p.Dim))), value)
		head.AddChild(line)
		return value
	}
	f.cash = metric("Cash", p.Text)
	f.deployed = metric("Deployed in orders", p.Text)
	// Receivables: the reward an accepted contract still owes (paid on
	// completion) — the player's own third pool of money.
	f.receivables = metric("Receivables", p.Text)
	f.total = metric("Total", p.Accent)

	cols := []widgets.Column{
		{Key: "When", Title: "When", Priority: 3},
		{Key: "What", Title: "What", Priority: 0},
		{Key: "Where", Title: "Where", Priority: 2},
		{Key: "Amount", Title: "Amount", Priority: 1, AlignRight: true},
	}
	// The ledger is names, dates and amounts: the mono contents face lines
	// the columns up. Chronological order is the sim's own; the window
	// shows it newest first, so the last movement is the top row.
	f.tbl = widgets.NewTable(env, cols, widgets.ContentsFace(env.FaceMono(theme.StepSmall)))

	body := widget.NewContainer(
		widget.ContainerOpts.BackgroundImage(theme.Bordered(theme.WindowBody(p.BG), theme.NRGBA(p.Border, 200), 1)),
		widget.ContainerOpts.Layout(widget.NewGridLayout(
			widget.GridLayoutOpts.Columns(1),
			// The table owns the rest of the window: its viewport is
			// bounded by the frame, so a full 200-entry ring scrolls
			// instead of growing the window.
			widget.GridLayoutOpts.Stretch([]bool{true}, []bool{false, true}),
			widget.GridLayoutOpts.Spacing(0, theme.Px(4)),
			widget.GridLayoutOpts.Padding(widget.NewInsetsSimple(theme.Px(theme.ContentsPad))),
		)),
	)
	body.AddChild(head, f.tbl.Container())

	title, _ := theme.NewTitleBar(env, "Finances", "svc_finances")

	f.win = widget.NewWindow(
		widget.WindowOpts.Contents(body),
		widget.WindowOpts.TitleBar(title, theme.Px(theme.TitleBarH)),
		// Content floor: the holdings block, the table header and a few
		// ledger rows (the list scrolls below that).
		widget.WindowOpts.MinSize(theme.Px(360), theme.Px(300)),
		// Fallback only: the composed rect at the 1024×700 minimum (the
		// game re-centers it from placeDefaults).
		widget.WindowOpts.Location(image.Rect(theme.Px(312), theme.Px(170), theme.Px(712), theme.Px(530))),
	)
	return f
}

// Win returns the finances window for Kit.AddWindow.
func (f *Finances) Win() *widget.Window { return f.win }

// Sync refreshes the holdings block and the ledger rows from the
// simulation. Call it every frame while mounted.
func (f *Finances) Sync() {
	cash, deployed, receivables := f.cv.Wallet(), f.cv.Deployed(), f.cv.Receivables()
	f.cash.Label = theme.FormatCredits(cash)
	f.deployed.Label = theme.FormatCredits(deployed)
	f.receivables.Label = theme.FormatCredits(receivables)
	f.total.Label = theme.FormatCredits(cash + deployed + receivables)

	entries := f.cv.Ledger()
	rows := make([]widgets.TableRow, 0, len(entries))
	for i := len(entries) - 1; i >= 0; i-- { // newest first
		rows = append(rows, ledgerRow(entries[i]))
	}
	f.tbl.SetRows(rows)
	f.tbl.Sync()
}

// ledgerRow renders one movement: when, what (with the broker fee and
// sales tax itemized inside the row), where, and the net amount. A debit
// keeps its minus sign (theme.FormatCredits).
func ledgerRow(e sim.LedgerEntry) widgets.TableRow {
	what := e.What
	if e.Fee > 0 {
		what += " · fee " + theme.FormatCredits(e.Fee)
	}
	if e.Tax > 0 {
		what += " · tax " + theme.FormatCredits(e.Tax)
	}
	return widgets.TableRow{Cells: []string{
		ledgerWhen(e.Tick),
		what,
		e.Where,
		theme.FormatCredits(e.Amount),
	}}
}

// ledgerWhen renders an entry's tick as elapsed sim time ("T+00:12:34"):
// the when column reads the same on every window size, where a raw Step
// count does not.
func ledgerWhen(tick int64) string {
	sec := tick / 60 // sim.DT is 1/60 s
	return fmt.Sprintf("T+%02d:%02d:%02d", sec/3600, (sec/60)%60, sec%60)
}
