package screens

import (
	"image"
	"slices"
	"sort"

	"github.com/ebitenui/ebitenui/widget"
	"github.com/entieve/entieve/content"
	"github.com/entieve/entieve/render"
	"github.com/entieve/entieve/sim"
	"github.com/entieve/entieve/uikit/theme"
	"github.com/entieve/entieve/uikit/widgets"
)

// Refinery is the station refinery window: every ore stack in the general
// cargo and the ore hold, the minerals it would yield at the stepper
// quantity (the sim's own floor(RefineYield × qty) math), and the Refine
// action. Ore refines in whole 100-ore batches only: the stepper offers
// 100, 200, 300, … capped at the selected ore's stock, and a stock under
// one batch disables Refine. Refined minerals always land in the station
// hangar (never the ship's hold), which is unlimited — so nothing to warn.
//
// Gating follows the game's menu convention: the button stays VISIBLE and
// grays out while undocked, at a station without the refine service, with
// no ore selected or with nothing held — never hidden. Every action
// enqueues a sim.Command (the kit never mutates sim state).
type Refinery struct {
	th  theme.Theme
	cv  widgets.ContentView
	as  widgets.ActionSink
	win *widget.Window

	ores   *widgets.Table
	status *widget.Text

	stepper *widgets.QtyStepper

	refine *widget.Button

	sel    refineryRow
	hasSel bool
}

// refineryRow is one ore table row: the ore id plus its Show Info ref.
type refineryRow struct {
	ore content.ItemID
	ref widgets.InfoRef
}

// InfoRefOf resolves the row payload inside the seam's Show Info resolver.
func (r refineryRow) InfoRefOf(cv widgets.ContentView) (widgets.InfoRef, bool) {
	return r.ref, widgets.InfoRefExists(cv, r.ref)
}

// NewRefinery builds the refinery window over the shared environment. Mount
// it on a Kit with Kit.AddWindow and call Sync every frame.
func NewRefinery(env widgets.Seams) *Refinery {
	r := &Refinery{th: env, cv: env, as: env}
	p := env.Pal()

	cols := []widgets.Column{
		{Key: "Ore", Title: "Ore", Priority: 0},
		{Key: "Held", Title: "Held", Priority: 2, AlignRight: true},
		{Key: "Yield", Title: "Yield @ qty → hangar", Priority: 1},
	}
	r.ores = widgets.NewTable(env, cols, widgets.PlateBand(func() image.Rectangle {
		return headerPlateBand(r.win)
	}))
	r.ores.SetSort("Ore", true)
	r.ores.SetSortFunc(canonicalFirst("Ore", cols))
	r.ores.OnSelect(func(row widgets.TableRow) {
		if v, ok := row.Data.(refineryRow); ok {
			r.sel, r.hasSel = v, true
		}
	})

	r.status = widget.NewText(
		widget.TextOpts.Text("Select an ore to refine.", env.Face(theme.StepSmall), theme.Solid(p.Text)),
	)
	// The ladder is the selected ore's whole batches: SetSteps in Sync
	// re-points it as the selection and the stock move.
	r.stepper = widgets.NewQtyStepper(env, nil)

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
	r.refine = newBtn("Refine", r.refineClicked)
	r.stepper.Row().AddChild(r.refine)

	body := widget.NewContainer(
		widget.ContainerOpts.Layout(widget.NewGridLayout(
			widget.GridLayoutOpts.Columns(1),
			widget.GridLayoutOpts.Stretch([]bool{true}, []bool{true, false, false}),
			widget.GridLayoutOpts.Spacing(0, theme.Px(4)),
			widget.GridLayoutOpts.Padding(widget.NewInsetsSimple(theme.Px(theme.ContentsPad))),
		)),
	)
	body.AddChild(r.ores.Container(), r.status, r.stepper.Row())

	title, _ := theme.NewTitleBar(env, "Refinery", "svc_refinery")

	// The body panel is the header band's: it paints the panel, the refining
	// plate over it and then the body (docs/ART.md "Window headers").
	panel := theme.Bordered(theme.WindowBody(p.BG), theme.NRGBA(p.Border, 200), 1)

	r.win = widget.NewWindow(
		widget.WindowOpts.Contents(newHeaderBand(body, panel, render.RefiningHeader)),
		widget.WindowOpts.TitleBar(title, theme.Px(theme.TitleBarH)),
		widget.WindowOpts.MinSize(theme.Px(340), theme.Px(320)),
		// Construction landing rect: the game centers it on first open.
		widget.WindowOpts.Location(image.Rect(theme.Px(430), theme.Px(160), theme.Px(850), theme.Px(560))),
	)
	return r
}

// Win returns the refinery window for Kit.AddWindow.
func (r *Refinery) Win() *widget.Window { return r.win }

// headerPlateBand is the band a table's PlateBand knob reads: the window's
// artwork header rect, or empty when it carries no band or the build ships no
// plate — the tables then paint exactly as before (docs/ART.md).
func headerPlateBand(w *widget.Window) image.Rectangle {
	if w == nil {
		return image.Rectangle{}
	}
	hb := headerBandOf(w)
	if hb == nil || hb.plate == nil || hb.plate() == nil {
		return image.Rectangle{}
	}
	return headerBandRect(hb.GetWidget().Rect)
}

// oreStacks merges the general cargo and ore hold stacks into the ore list
// in first-seen order (duplicates summed) — the two places CmdRefine
// consumes from.
func (r *Refinery) oreStacks() []content.CargoStack {
	var out []content.CargoStack
	add := func(st content.CargoStack) {
		def, ok := r.cv.Defs().Item(st.Item)
		if !ok || def.Category != content.CatOre || st.Qty <= 0 {
			return
		}
		for i := range out {
			if out[i].Item == st.Item {
				out[i].Qty += st.Qty
				return
			}
		}
		out = append(out, st)
	}
	for _, st := range r.cv.CargoStacks() {
		add(st)
	}
	for _, st := range r.cv.OreStacks() {
		add(st)
	}
	return out
}

// refineOutputs is the exact conversion CmdRefine applies: floor(yield ×
// qty) per mineral, in sorted mineral-id order, dropping zero yields.
func refineOutputs(cv widgets.ContentView, def *content.ItemDef, qty int) []content.CargoStack {
	if def == nil || qty < 1 {
		return nil
	}
	ids := make([]content.ItemID, 0, len(def.RefineYield))
	for id := range def.RefineYield {
		ids = append(ids, id)
	}
	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
	var out []content.CargoStack
	for _, id := range ids {
		n := int(def.RefineYield[id] * float64(qty))
		if n > 0 {
			out = append(out, content.CargoStack{Item: id, Qty: n})
		}
	}
	return out
}

// outputsText renders an output list as "45 Sideron · 30 Nixium" ("" for
// an empty list).
func (r *Refinery) outputsText(outs []content.CargoStack) string {
	s := ""
	for i, o := range outs {
		if i > 0 {
			s += " · "
		}
		s += theme.FormatInt(int64(o.Qty)) + " " + r.cv.TradeableName(o.Item)
	}
	return s
}

// Sync refreshes the ore list, the yield preview and every gate from the
// simulation. Call it every frame before Kit.Update.
func (r *Refinery) Sync() {
	ores := r.oreStacks()
	rows := make([]widgets.TableRow, 0, len(ores))
	for _, st := range ores {
		ref, _ := widgets.RefOfID(r.cv, string(st.Item))
		row := refineryRow{ore: st.Item, ref: ref}
		eff := r.qtyFor(st.Qty)
		def, _ := r.cv.Defs().Item(st.Item)
		rows = append(rows, widgets.TableRow{
			Cells: []string{
				r.cv.TradeableName(st.Item),
				theme.FormatInt(int64(st.Qty)),
				r.outputsText(refineOutputs(r.cv, def, eff)),
			},
			Icons: []string{productIcon(r.cv, st.Item), "", ""},
			Data:  row,
			Spec: widgets.RowSpec{Ref: &row.ref, Actions: []widgets.RowAction{{
				Label: "Refine", Disabled: !r.refineReadyFor(row),
				Intent: widgets.Refine{Ore: row.ore, Qty: eff},
			}}},
		})
	}
	r.ores.SetRows(rows)
	r.ores.Sync()

	// Revalidate the cached selection against the rows themselves.
	r.hasSel = false
	if sel, ok := r.ores.Selected(); ok {
		if row, isRow := sel.Data.(refineryRow); isRow && row.ore != "" {
			r.sel, r.hasSel = row, true
		}
	}
	if !r.hasSel {
		r.sel = refineryRow{}
	}

	// A smaller selection can drop the stepper's cap: the ladder is the
	// selected ore's whole batches, so the widget clamps to a whole batch.
	r.stepper.SetSteps(batchSteps(r.stockOf(r.sel.ore)))
	qty := r.stepper.Value()
	r.refine.GetWidget().Disabled = !r.refineReadyFor(r.sel)

	if !r.hasSel {
		r.status.Label = "Select an ore to refine."
	} else if qty < sim.RefineBatch {
		r.status.Label = "One batch needs " + theme.FormatInt(int64(sim.RefineBatch)) + " ore — you hold " +
			theme.FormatInt(int64(r.stockOf(r.sel.ore))) + " " + r.cv.TradeableName(r.sel.ore) + "."
	} else {
		def, _ := r.cv.Defs().Item(r.sel.ore)
		outs := refineOutputs(r.cv, def, qty)
		line := "Refine " + theme.FormatInt(int64(qty)) + " × " + r.cv.TradeableName(r.sel.ore) + " → "
		if text := r.outputsText(outs); text != "" {
			line += text
		} else {
			line += "nothing"
		}
		r.status.Label = line + " → hangar"
	}
}

// stockOf is the units of one ore in the two holds (the sim's own
// availability read).
func (r *Refinery) stockOf(ore content.ItemID) int {
	n := 0
	for _, st := range r.oreStacks() {
		if st.Item == ore {
			n += st.Qty
		}
	}
	return n
}

// refineReadyFor is the Refine gate: a selected ore, the docked station's
// refine service and at least one whole batch of that ore in the holds.
func (r *Refinery) refineReadyFor(row refineryRow) bool {
	if row.ore == "" || !r.refineService() {
		return false
	}
	return r.qtyFor(r.stockOf(row.ore)) >= sim.RefineBatch
}

// refineService reports whether the docked station offers refine (in space
// the sim's own stationAccess gate refuses).
func (r *Refinery) refineService() bool {
	info, ok := r.cv.DockedStation()
	return ok && slices.Contains(info.Services, "refine")
}

// batchSteps is the stepper ladder for an ore holding stock units: one
// rung per whole batch of 100, none below a batch.
func batchSteps(stock int) []int {
	n := stock / sim.RefineBatch
	if n < 1 {
		return nil
	}
	steps := make([]int, n)
	for i := range steps {
		steps[i] = (i + 1) * sim.RefineBatch
	}
	return steps
}

// qtyFor is the stepper's unit count for an ore holding stock units:
// whole batches only, capped by the stepper position and that stack's
// stock.
func (r *Refinery) qtyFor(stock int) int {
	n := min(r.stepper.Index()+1, stock/sim.RefineBatch)
	if n < 1 {
		return 0
	}
	return n * sim.RefineBatch
}

// refineClicked emits Refine for the selected ore (gated even when called
// directly, the market buttons' rule).
func (r *Refinery) refineClicked() {
	if !r.refineReadyFor(r.sel) {
		return
	}
	r.as.Do(widgets.Refine{Ore: r.sel.ore, Qty: r.stepper.Value()})
}
