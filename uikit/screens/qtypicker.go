package screens

import (
	"image"
	"math"
	"slices"

	"github.com/ebitenui/ebitenui/widget"
	"github.com/entieve/entieve/content"
	"github.com/entieve/entieve/sim"
	"github.com/entieve/entieve/uikit/theme"
	"github.com/entieve/entieve/uikit/widgets"
	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/inpututil"
)

// qtyMax is the freeflow picker's ceiling: the retired ± ladder's top, so
// the trade, transfer and runs surfaces offer 1..100 in single steps.
const qtyMax = 100

// qtyMinW/qtyMinH are the picker's design-px floor: title bar plus the
// five body rows (item line, slider row, preview line, note line, buttons)
// and their ContentsPad gaps. The width fits the longest stock preview
// ("Refine N × <ore> → <minerals>") at the tiny mono face; content growth
// past this fails in TestQtyWindowFits.
const (
	qtyMinW = 460
	qtyMinH = 232
)

// QtyWin is the quantity picker: a small, modal, transient popup that asks
// for ONE action's quantity (CONTEXT.md "Quantity picker"). Every ± ticker
// in the interface was replaced by it: the player picks the amount on a
// slider and confirms, and the picker emits the surface's existing intent
// (Refine, StartJob, Buy, Sell, Transfer) — nothing new reaches the sim.
//
// Slider model: the refinery is the one notched surface (whole 100-ore
// batches, an empty ladder below one batch), everything else freeflow
// 1..qtyMax. The confirmed value is remembered per mode and re-seeded on
// the next open, clamped to the live range — the persistence the ± tickers
// had (a rebuilt interface starts at 1 again).
//
// One instance per session: the session's services own it (Session.
// QtyWindow) and a Kit mounts it on first use (Kit.OpenQty). A bare
// session only shows it — its host mounts the window by hand.
type QtyWin struct {
	th theme.Theme
	cv widgets.ContentView
	as widgets.ActionSink

	win   *widget.Window
	title *theme.ChromeText
	info  *widget.Text

	sliderRow *widget.Container
	qtyLbl    *widget.Text
	box       *widget.Container // holds the current slider (rebuilt per range)
	slider    *widgets.Slider
	read      *widget.Text

	detail  *widget.Text
	note    *widget.Text // the preview's second half (destination, inputs)
	confirm *widget.Button
	cancel  *widget.Button

	req     widgets.QtyPick
	notches []float64               // the notched ladder, to notice a re-point
	last    map[widgets.QtyMode]int // per-mode memory of the confirmed value
	// shortcut: Sell is one mode, so the market and the inventory share its
	// seed; split the key per panel if the two must remember separately.
	onClose func() // Kit-hosted: unmount (a bare env only hides)
}

// NewQtyWin builds the closed picker over the given environment.
func NewQtyWin(env widgets.Seams) *QtyWin {
	p := env.Pal()
	o := &QtyWin{th: env, cv: env, as: env, last: map[widgets.QtyMode]int{}}

	o.info = widget.NewText(
		widget.TextOpts.Text("", env.Face(theme.StepTiny), theme.Solid(p.Dim)),
	)
	o.qtyLbl = widget.NewText(
		widget.TextOpts.Text("Qty", env.Face(theme.StepSmall), theme.Solid(p.Text)),
		widget.TextOpts.WidgetOpts(widget.WidgetOpts.LayoutData(widget.GridLayoutData{
			VerticalPosition: widget.GridLayoutPositionCenter,
		})),
	)
	o.read = widget.NewText(
		widget.TextOpts.Text("1", env.FaceMono(theme.StepSmall), theme.Solid(p.Accent)),
		widget.TextOpts.WidgetOpts(widget.WidgetOpts.LayoutData(widget.GridLayoutData{
			VerticalPosition: widget.GridLayoutPositionCenter,
		})),
	)
	// The slider lives in its own one-cell box: a slider never changes its
	// range after construction, so a new target replaces the box's child
	// instead of disturbing the row's column order.
	o.box = widget.NewContainer(widget.ContainerOpts.Layout(widget.NewGridLayout(
		widget.GridLayoutOpts.Columns(1),
		widget.GridLayoutOpts.Stretch([]bool{true}, []bool{false}),
	)))
	o.sliderRow = widget.NewContainer(
		widget.ContainerOpts.Layout(widget.NewGridLayout(
			widget.GridLayoutOpts.Columns(3),
			widget.GridLayoutOpts.Stretch([]bool{false, true, false}, []bool{false}),
			widget.GridLayoutOpts.Spacing(theme.Px(theme.ContentsPad), 0),
		)),
		// Full body width: the vertical RowLayout stretches its children on
		// the cross axis only when asked (the settings rows' rule).
		widget.ContainerOpts.WidgetOpts(widget.WidgetOpts.LayoutData(widget.RowLayoutData{Stretch: true})),
	)
	o.sliderRow.AddChild(o.qtyLbl, o.box, o.read)
	o.detail = widget.NewText(
		widget.TextOpts.Text("", env.FaceMono(theme.StepTiny), theme.Solid(p.Dim)),
	)
	o.note = widget.NewText(
		widget.TextOpts.Text("", env.FaceMono(theme.StepTiny), theme.Solid(p.Dim)),
	)

	newBtn := func(label string, clicked func()) *widget.Button {
		return widget.NewButton(
			widget.ButtonOpts.TextLabel(label),
			widget.ButtonOpts.WidgetOpts(
				widget.WidgetOpts.MinSize(theme.Px(40), theme.Px(40)),
				widget.WidgetOpts.LayoutData(widget.RowLayoutData{Stretch: true}),
			),
			widget.ButtonOpts.ClickedHandler(func(*widget.ButtonClickedEventArgs) { clicked() }),
		)
	}
	o.confirm = newBtn("Confirm", o.submit)
	o.cancel = newBtn("Cancel", o.Close)

	row := func(children ...widget.PreferredSizeLocateableWidget) *widget.Container {
		c := widget.NewContainer(widget.ContainerOpts.Layout(widget.NewRowLayout(
			widget.RowLayoutOpts.Direction(widget.DirectionHorizontal),
			widget.RowLayoutOpts.Spacing(theme.Px(theme.ContentsPad)),
		)))
		c.AddChild(children...)
		return c
	}

	body := widget.NewContainer(
		widget.ContainerOpts.BackgroundImage(theme.WindowBodyBox(p, theme.WindowBody(p.BG), theme.NRGBA(p.Border, 200))),
		widget.ContainerOpts.Layout(widget.NewRowLayout(
			widget.RowLayoutOpts.Direction(widget.DirectionVertical),
			widget.RowLayoutOpts.Spacing(theme.Px(theme.ContentsPad)),
			widget.RowLayoutOpts.Padding(widget.NewInsetsSimple(theme.Px(theme.ContentsPad))),
		)),
	)
	body.AddChild(o.info, o.sliderRow, o.detail, o.note, row(o.confirm, o.cancel))

	// The title bar carries the base display name; the × close is attached by the
	// kit's close seam (SetWindowClose), like Show Info and the order window.
	bar, title := theme.NewTitleBar(env, "Quantity", "")
	o.title = title

	o.win = widget.NewWindow(
		widget.WindowOpts.Contents(body),
		widget.WindowOpts.TitleBar(bar, theme.Px(theme.TitleBarH)),
		widget.WindowOpts.MinSize(theme.Px(qtyMinW), theme.Px(qtyMinH)),
		// Modal: the picker's fullscreen input layer owns every click, so a
		// press outside the little window never reaches the window beneath
		// it (and never dismisses it) — the one-picker-at-a-time rule.
		widget.WindowOpts.Modal(),
		// Fallback only: the kit centres the picker on the viewport when it
		// opens it.
		widget.WindowOpts.Location(image.Rect(theme.Px(480), theme.Px(260), theme.Px(480+qtyMinW), theme.Px(260+qtyMinH))),
	)
	theme.ShowWidget(o.win.GetContainer().GetWidget(), false)
	return o
}

// Win returns the window for Kit.AddWindow / the host's hit test.
func (o *QtyWin) Win() *widget.Window { return o.win }

// Show re-targets the picker on req, re-seeds the slider from the mode's
// memory (clamped to the live range) and reveals the window.
func (o *QtyWin) Show(req widgets.QtyPick) bool {
	if o.cv == nil {
		return false
	}
	o.req = req
	// The title is the base display name only (a window title is
	// furniture): the picker's own lines carry the amount and the preview,
	// and the item where the mode names one.
	o.title.Label = "Quantity"
	o.qtyLbl.Label = "Qty"
	if req.Mode == widgets.QtyStart {
		o.qtyLbl.Label = "Runs"
	}
	o.buildSlider(o.seed())
	o.refresh()
	theme.ShowWidget(o.win.GetContainer().GetWidget(), true)
	return true
}

// Close closes the picker: through the Kit it also unmounts (so a closed
// picker stops gating world input); in a bare environment it just hides.
func (o *QtyWin) Close() {
	if o.onClose != nil {
		o.onClose()
		return
	}
	o.hide()
}

// hide flips the container invisible (the kit's unmount path).
func (o *QtyWin) hide() { theme.ShowWidget(o.win.GetContainer().GetWidget(), false) }

// IsOpen reports whether the picker is revealed (shown, and mounted when a
// Kit hosts it).
func (o *QtyWin) IsOpen() bool {
	return o.win != nil && o.win.GetContainer().GetWidget().IsVisible()
}

// TitleText is the title-bar label (the base display name).
func (o *QtyWin) TitleText() string {
	if o.title == nil {
		return ""
	}
	return o.title.Label
}

// DetailText is the live preview line (tests pin the yield, the run
// duration/fee and the trade total); NoteText is its second line (the
// destination or the have/need list).
func (o *QtyWin) DetailText() string { return o.detail.Label }
func (o *QtyWin) NoteText() string   { return o.note.Label }

// Value is the quantity Confirm would emit.
func (o *QtyWin) Value() int { return int(math.Round(o.slider.Value())) }

// Slider is the picker's slider (the tests point at rungs through it).
func (o *QtyWin) Slider() *widgets.Slider { return o.slider }

// Confirm is the picker's confirm button (tests read its disabled gate).
func (o *QtyWin) Confirm() *widget.Button { return o.confirm }

// Sync runs one frame of the picker: the slider's input, then the live
// readout, preview and Confirm gate. The kit calls it from Kit.Sync, so a
// drag and a moving hold both land before the frame is drawn.
func (o *QtyWin) Sync() {
	if !o.IsOpen() {
		return
	}
	o.slider.Poll()
	// The refinery ladder follows the stock (a refine lands, cargo moves):
	// re-point it and let the slider clamp to the nearest whole batch — the
	// retired stepper's SetSteps rule.
	if o.req.Mode == widgets.QtyRefine {
		if steps := qtyBatches(o.stockOf(o.req.Item)); !slices.Equal(steps, o.notches) {
			o.buildSlider(o.Value())
		}
	}
	o.refresh()
}

// buildSlider puts the mode's slider in the row, seeded at v: the
// refinery's whole-batch ladder or the freeflow 1..qtyMax range. A slider
// never changes its range after construction, so a range change (or a new
// target) rebuilds it — the old one's grab goes with it.
func (o *QtyWin) buildSlider(v int) {
	opts := widgets.SliderOpts{
		Mode:     widgets.SliderFreeflow,
		Min:      1,
		Max:      qtyMax,
		Step:     1,
		Value:    float64(v),
		OnChange: func(float64) { o.refresh() },
	}
	var steps []float64
	if o.req.Mode == widgets.QtyRefine {
		steps = qtyBatches(o.stockOf(o.req.Item))
		opts = widgets.SliderOpts{
			Mode:     widgets.SliderNotches,
			Notches:  steps,
			Value:    float64(v),
			OnChange: func(float64) { o.refresh() },
		}
	}
	o.notches = steps
	s := widgets.NewSlider(o.th, opts)
	if o.slider != nil {
		o.box.ReplaceChild(o.slider, s)
	} else {
		o.box.AddChild(s)
	}
	o.slider = s
}

// seed is the value the picker opens at: the mode's last confirmed value
// (1 on a fresh session — the retired ladder's first rung); a notched
// slider clamps it to the nearest live rung.
func (o *QtyWin) seed() int {
	if v := o.last[o.req.Mode]; v >= 1 {
		return v
	}
	return 1
}

// stockOf is the units of one ore in the two holds (the refinery's own
// availability read: CmdRefine consumes from both).
func (o *QtyWin) stockOf(ore content.ItemID) int {
	n := 0
	for _, st := range o.cv.CargoStacks() {
		if st.Item == ore {
			n += st.Qty
		}
	}
	for _, st := range o.cv.OreStacks() {
		if st.Item == ore {
			n += st.Qty
		}
	}
	return n
}

// refresh rewrites the readout, the item line, the preview and the
// Confirm gate from the slider's current value.
func (o *QtyWin) refresh() {
	q := o.Value()
	setStepperValue(o.sliderRow, o.read, theme.FormatInt(int64(q)))
	o.info.Label = o.infoLine()
	o.detail.Label = o.detailLine(q)
	o.note.Label = o.noteLine(q)
	o.confirm.GetWidget().Disabled = !o.ready(q)
}

// infoLine is the picker's item line: what the picked id is and the range
// the slider offers.
func (o *QtyWin) infoLine() string {
	name := o.cv.TradeableName(o.req.Item)
	switch o.req.Mode {
	case widgets.QtyRefine:
		return theme.FormatInt(int64(o.stockOf(o.req.Item))) + " " + name +
			" held · whole " + theme.FormatInt(int64(sim.RefineBatch)) + "-ore batches"
	case widgets.QtyStart:
		return "1–" + theme.FormatInt(qtyMax) + " runs · " +
			theme.FormatInt(int64(len(o.cv.AllJobs()))) + "/" + theme.FormatInt(int64(sim.IndustrySlots())) + " job slots · " +
			theme.FormatCredits(o.cv.Wallet()) + " in the wallet"
	case widgets.QtyBuy:
		return "1–" + theme.FormatInt(qtyMax) + " units · " +
			theme.FormatCredits(o.cv.Wallet()) + " in the wallet"
	case widgets.QtySell:
		return "1–" + theme.FormatInt(qtyMax) + " units · " +
			theme.FormatInt(int64(o.cv.PlayerStock(o.req.Item))) + " " + name + " held"
	default:
		return "1–" + theme.FormatInt(qtyMax) + " units → " + holdName(o.req.To)
	}
}

// detailLine is the live preview's first line: the figures the surface
// used to show in its own window (the refinery's yield, the recipe's run
// time and fee, the trade total). noteLine is its second line — the
// destination or the have/need list — "" where the first says it all.
func (o *QtyWin) detailLine(q int) string {
	name := o.cv.TradeableName(o.req.Item)
	switch o.req.Mode {
	case widgets.QtyRefine:
		if q < sim.RefineBatch {
			return oneBatchLine(o.cv, o.req.Item, o.stockOf(o.req.Item))
		}
		def, _ := o.cv.Defs().Item(o.req.Item)
		outs := outputsText(o.cv, refineOutputs(o.cv, def, q))
		if outs == "" {
			outs = "nothing"
		}
		return "Refine " + theme.FormatInt(int64(q)) + " × " + name + " → " + outs
	case widgets.QtyStart:
		st := o.cv.DockedIndustry()
		return theme.FormatInt(int64(q)) + " runs · " +
			theme.GroupFloat(o.cv.JobDuration(st, o.req.Recipe, q), 1) + " s · fee " +
			theme.FormatCredits(o.cv.JobFee(o.req.Recipe, q))
	case widgets.QtyBuy, widgets.QtySell:
		price := o.cv.PriceLadder(o.cv.DockedMarket(), o.req.Item)
		return theme.FormatInt(int64(q)) + " × " + theme.FormatCredits(price) + " ≈ " +
			theme.FormatCredits(price*float64(q))
	default:
		return theme.FormatInt(int64(q)) + " × " + name
	}
}

// noteLine is the preview's second line.
func (o *QtyWin) noteLine(q int) string {
	switch o.req.Mode {
	case widgets.QtyRefine:
		if q < sim.RefineBatch {
			return ""
		}
		return "→ station hangar"
	case widgets.QtyStart:
		return inputsText(o.cv, o.cv.JobMaterials(o.cv.DockedIndustry(), o.req.Recipe, q))
	case widgets.QtyBuy, widgets.QtySell:
		return ""
	default:
		return "→ " + holdName(o.req.To)
	}
}

// ready is the Confirm gate: the refinery needs a whole batch and its
// station's refine service, a job the sim's own start gate at these runs,
// every other surface just a quantity (its opening action already gated).
func (o *QtyWin) ready(q int) bool {
	switch o.req.Mode {
	case widgets.QtyRefine:
		return len(o.notches) > 0 && refineService(o.cv)
	case widgets.QtyStart:
		return o.cv.CanStartJob(o.cv.DockedIndustry(), o.req.Recipe, q)
	}
	return q >= 1
}

// submit remembers the confirmed quantity and emits the surface's intent
// (the engine answers a refusal with its own SevWarning).
func (o *QtyWin) submit() {
	q := o.Value()
	if !o.ready(q) {
		return
	}
	o.last[o.req.Mode] = q
	switch o.req.Mode {
	case widgets.QtyRefine:
		o.as.Do(widgets.Refine{Ore: o.req.Item, Qty: q})
	case widgets.QtyStart:
		o.as.Do(widgets.StartJob{Recipe: o.req.Recipe, Runs: q})
	case widgets.QtyBuy:
		o.as.Do(widgets.Buy{Item: o.req.Item, Qty: q})
	case widgets.QtySell:
		o.as.Do(widgets.Sell{Item: o.req.Item, Qty: q})
	case widgets.QtyTransfer:
		o.as.Do(widgets.Transfer{Item: o.req.Item, Qty: q, From: o.req.From, To: o.req.To})
	}
	o.Close()
}

// qtyBatches is the refinery's notched ladder for an ore holding stock
// units: one rung per whole batch of 100, none below a batch.
func qtyBatches(stock int) []float64 {
	n := stock / sim.RefineBatch
	if n < 1 {
		return nil
	}
	steps := make([]float64, n)
	for i := range steps {
		steps[i] = float64((i + 1) * sim.RefineBatch)
	}
	return steps
}

// holdName is a hold key as the picker names it.
func holdName(hold string) string {
	switch hold {
	case "ore":
		return "Ore"
	case "hangar":
		return "Hangar"
	default:
		return "Cargo"
	}
}

// qtyEsc reads the Esc edge for the picker. Production polls ebiten
// directly (the menu's own read is not exported); headless tests swap it
// through SetQtyEscForTest so Esc never needs an OS event.
var qtyEsc = func() bool { return inpututil.IsKeyJustPressed(ebiten.KeyEscape) }

// SetQtyEscForTest overrides the picker's Esc read for a headless test and
// returns a restore func. Test-only seam: never call it from production code.
func SetQtyEscForTest(f func() bool) func() {
	prev := qtyEsc
	qtyEsc = f
	return func() { qtyEsc = prev }
}
