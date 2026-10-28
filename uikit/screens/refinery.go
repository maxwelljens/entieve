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
// cargo and the ore hold, the minerals one whole batch of it yields (the
// sim's own floor(RefineYield × qty) math), and the static status line.
// Ore refines in whole 100-ore batches only, so selecting an ore opens the
// quantity picker (CONTEXT.md "Quantity picker") on its notched ladder
// 100, 200, 300, … capped at that ore's stock; a stock under one batch
// still opens it, with an empty ladder and Confirm greyed. Refined
// minerals always land in the station hangar (never the ship's hold),
// which is unlimited — so nothing to warn.
//
// Gating follows the game's menu convention: an action stays VISIBLE and
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
		{Key: "Yield", Title: "Yield / batch → hangar", Priority: 1},
	}
	r.ores = widgets.NewTable(env, cols, widgets.PrefsID("ores"), widgets.PlateBand(func() image.Rectangle {
		return headerPlateBand(r.win)
	}))
	r.ores.SetSort("Ore", true)
	r.ores.SetSortFunc(canonicalFirst("Ore", cols))
	// Selecting an ore is the whole gesture: it picks the target AND opens
	// the quantity picker on it (the row menu's Refine does the same).
	r.ores.OnSelect(func(row widgets.TableRow) {
		if v, ok := row.Data.(refineryRow); ok {
			r.sel, r.hasSel = v, true
			r.openPicker(v)
		}
	})

	r.status = widget.NewText(
		widget.TextOpts.Text("Select an ore to refine.", env.Face(theme.StepSmall), theme.Solid(p.Text)),
	)

	body := widget.NewContainer(
		widget.ContainerOpts.Layout(widget.NewGridLayout(
			widget.GridLayoutOpts.Columns(1),
			widget.GridLayoutOpts.Stretch([]bool{true}, []bool{true, false}),
			widget.GridLayoutOpts.Spacing(0, theme.Px(4)),
			widget.GridLayoutOpts.Padding(widget.NewInsetsSimple(theme.Px(theme.ContentsPad))),
		)),
	)
	body.AddChild(r.ores.Container(), r.status)

	title, _ := theme.NewTitleBar(env, "Refinery", "svc_refinery")

	// The body panel is the header band's: it paints the panel, the refining
	// plate over it and then the body (docs/ART.md "Window headers").
	panel := theme.WindowBodyBox(p, theme.WindowBody(p.BG), theme.NRGBA(p.Border, 200))

	r.win = widget.NewWindow(
		widget.WindowOpts.Contents(newHeaderBand(body, panel, render.RefiningHeader)),
		widget.WindowOpts.TitleBar(title, theme.Px(theme.TitleBarH)),
		widget.WindowOpts.MinSize(theme.Px(340), theme.Px(320)),
		// Construction landing rect: the game centers it on first open;
		// the row declares the same rect (RefineryRect) as its size.
		widget.WindowOpts.Location(pxRect(RefineryRect())),
	)
	return r
}

// RefineryRect is the refinery window's landing rect in design units: the
// one number its Screen row's DefaultRect and its constructor share.
func RefineryRect() image.Rectangle { return image.Rect(430, 160, 850, 560) }

// Win returns the refinery window for Kit.AddWindow.
func (r *Refinery) Win() *widget.Window { return r.win }

// Tables are the window's table for the layout sidecar (the ore list's
// column widths).
func (r *Refinery) Tables() []*widgets.Table { return []*widgets.Table{r.ores} }

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
func outputsText(cv widgets.ContentView, outs []content.CargoStack) string {
	s := ""
	for i, o := range outs {
		if i > 0 {
			s += " · "
		}
		s += theme.FormatInt(int64(o.Qty)) + " " + cv.TradeableName(o.Item)
	}
	return s
}

// oneBatchLine is the sub-batch wording the window and the picker share: a
// stock under one whole batch has no valid refine quantity.
func oneBatchLine(cv widgets.ContentView, ore content.ItemID, stock int) string {
	return "One batch needs " + theme.FormatInt(int64(sim.RefineBatch)) + " ore — you hold " +
		theme.FormatInt(int64(stock)) + " " + cv.TradeableName(ore) + "."
}

// Sync refreshes the ore list and the status line from the simulation. Call
// it every frame before Kit.Update.
func (r *Refinery) Sync() {
	ores := r.oreStacks()
	rows := make([]widgets.TableRow, 0, len(ores))
	for _, st := range ores {
		ref, _ := widgets.ResolveTradeable(r.cv.Defs(), st.Item).Ref()
		row := refineryRow{ore: st.Item, ref: ref}
		def, _ := r.cv.Defs().Item(st.Item)
		rows = append(rows, widgets.TableRow{
			Cells: []string{
				r.cv.TradeableName(st.Item),
				theme.FormatInt(int64(st.Qty)),
				outputsText(r.cv, refineOutputs(r.cv, def, sim.RefineBatch)),
			},
			Icons: []string{widgets.TradeableIcon(r.cv.Defs(), st.Item), "", ""},
			Data:  row,
			Spec: widgets.RowSpec{Ref: &row.ref, Actions: []widgets.RowAction{{
				Label: "Refine", Disabled: !r.refineReadyFor(row),
				Intent: widgets.QtyPick{Mode: widgets.QtyRefine, Item: row.ore},
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

	if !r.hasSel {
		r.status.Label = "Select an ore to refine."
	} else if !refineService(r.cv) {
		r.status.Label = "Refine needs a station with the refine service."
	} else if batchSteps(r.stockOf(r.sel.ore)) == 0 {
		r.status.Label = oneBatchLine(r.cv, r.sel.ore, r.stockOf(r.sel.ore))
	} else {
		r.status.Label = "Whole " + theme.FormatInt(int64(sim.RefineBatch)) +
			"-ore batches only — Refine opens the quantity picker."
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
	if row.ore == "" || !refineService(r.cv) {
		return false
	}
	return batchSteps(r.stockOf(row.ore)) > 0
}

// refineService reports whether the docked station offers refine (in space
// the sim's own stationAccess gate refuses).
func refineService(cv widgets.ContentView) bool {
	info, ok := cv.DockedStation()
	return ok && slices.Contains(info.Services, "refine")
}

// batchSteps is the number of whole batches an ore holding stock units
// has — the picker's ladder length, zero below a batch.
func batchSteps(stock int) int {
	return stock / sim.RefineBatch
}

// openPicker opens the quantity picker on an ore — the selection gesture.
// Nothing opens away from a refine station: there is no quantity to pick.
func (r *Refinery) openPicker(row refineryRow) {
	if !refineService(r.cv) {
		return
	}
	r.as.Do(widgets.QtyPick{Mode: widgets.QtyRefine, Item: row.ore})
}
