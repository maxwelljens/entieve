package screens

import (
	"image"
	"slices"
	"sort"
	"strconv"

	"github.com/ebitenui/ebitenui/widget"
	"github.com/entieve/entieve/content"
	"github.com/entieve/entieve/render"
	"github.com/entieve/entieve/sim"
	"github.com/entieve/entieve/uikit/theme"
	"github.com/entieve/entieve/uikit/widgets"
)

// Manufacturing is the station manufacturing window: the recipe board
// (product name + icon, family, per-run time at the docked station's
// current craft level, per-run fee, one run's inputs with have/need) over
// the GLOBAL job list (every station, grouped by station) with per-job
// progress and the Collect / Cancel actions — in the market panel's shape
// (two tables, toolbars, rows wired for Show Info). Selecting a recipe
// opens the quantity picker on its runs (CONTEXT.md "Quantity picker");
// Confirm emits StartJob.
//
// Gating follows the game's menu convention: controls stay VISIBLE and
// gray out (widget Disabled) when the player is undocked, the station
// offers no industry service, the materials or wallet are short or the
// slots are full — never hidden. Jobs at another station are listed but
// only the docked station's own jobs can be collected or cancelled. Every
// action enqueues a sim.Command (the kit never mutates sim state); the sim
// logs the refusal line if a gated action still gets through.
type Manufacturing struct {
	th  theme.Theme
	cv  widgets.ContentView
	as  widgets.ActionSink
	ptr *widgets.Pointer
	win *widget.Window

	recipes *widgets.Table
	jobs    *widgets.Table
	split   *widgets.SplitPane

	craftLbl *widget.Text
	jobsLbl  *widget.Text
	title    *theme.ChromeText

	collect, cancel *widget.Button

	sel    manufacturingRow
	hasSel bool
}

// manufacturingRow is the manufacturing tables' row payload: the recipe id (recipe
// rows) or job id (job rows) plus the product ref both resolve Show Info
// through (InfoRefOf handles the type; see infowin.go).
type manufacturingRow struct {
	recipe content.RecipeID
	job    uint64
	ref    widgets.InfoRef
}

// InfoRefOf resolves the row payload inside the seam's Show Info
// resolver (widgets/infoRefPayload): the row already carries its ref.
func (r manufacturingRow) InfoRefOf(cv widgets.ContentView) (widgets.InfoRef, bool) {
	return r.ref, widgets.InfoRefExists(cv, r.ref)
}

// NewManufacturing builds the Manufacturing window over the shared environment. Mount
// it on a Kit with Kit.AddWindow and call Sync every frame.
func NewManufacturing(env widgets.Seams) *Manufacturing {
	f := &Manufacturing{th: env, cv: env, as: env, ptr: env.Pointer()}
	p := env.Pal()

	recipeCols := []widgets.Column{
		{Key: "Product", Title: "Product", Priority: 0},
		{Key: "Family", Title: "Family", Priority: 1},
		{Key: "Time", Title: "Time", Priority: 2, AlignRight: true},
		{Key: "Fee", Title: "Fee", Priority: 3, AlignRight: true},
		{Key: "Inputs", Title: "Inputs", Priority: 4},
	}
	f.recipes = widgets.NewTable(env, recipeCols, widgets.PrefsID("recipes"), widgets.PlateBand(func() image.Rectangle {
		return headerPlateBand(f.win)
	}))
	jobCols := []widgets.Column{
		{Key: "Product", Title: "Product", Priority: 0},
		{Key: "Station", Title: "Station", Priority: 1},
		{Key: "Runs", Title: "Runs", Priority: 4, AlignRight: true},
		{Key: "Progress", Title: "Progress", Priority: 2, AlignRight: true},
		{Key: "State", Title: "State", Priority: 1},
	}
	f.jobs = widgets.NewTable(env, jobCols, widgets.PrefsID("jobs"))

	// Canonical display order: the sim's own RecipesInOrder / job-id
	// order (reversing on a second header click, any other header click
	// falls back to plain cell sorting — the market table's rule).
	f.recipes.SetSort("Product", true)
	f.recipes.SetSortFunc(canonicalFirst("Product", recipeCols))
	f.jobs.SetSort("Product", true)
	f.jobs.SetSortFunc(canonicalFirst("Product", jobCols))

	attachSelect := func(t *widgets.Table, recipe bool) {
		t.OnSelect(func(r widgets.TableRow) {
			row, ok := r.Data.(manufacturingRow)
			if !ok {
				return
			}
			f.sel, f.hasSel = row, true
			if recipe {
				f.openPicker(row)
			}
		})
	}
	attachSelect(f.recipes, true)
	attachSelect(f.jobs, false)
	// Show Info, double-click and the Start/Collect action derive from
	// the rows' RowSpec (rowSpec), built in Sync — no hand wiring.

	f.craftLbl = widget.NewText(
		widget.TextOpts.Text("Craft", env.Face(theme.StepTiny), theme.Solid(p.Accent)),
	)
	f.jobsLbl = widget.NewText(
		widget.TextOpts.Text("Jobs", env.Face(theme.StepTiny), theme.Solid(p.Accent)),
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
	f.collect = newBtn("Collect", f.collectClicked)
	f.cancel = newBtn("Cancel", f.cancelClicked)

	collectBar := widget.NewContainer(
		widget.ContainerOpts.Layout(widget.NewRowLayout(
			widget.RowLayoutOpts.Direction(widget.DirectionHorizontal),
			widget.RowLayoutOpts.Spacing(theme.Px(theme.ContentsPad)),
		)),
	)
	collectBar.AddChild(f.collect, f.cancel)

	// Body: the craft readout over two panes — the recipe board (its rows
	// open the quantity picker, so it needs no toolbar), the job list with
	// Collect/Cancel — split by a draggable seam (equal halves by default;
	// a drag persists in the layout sidecar).
	f.split = widgets.NewSplitPane(env,
		paneStack([]bool{true}, f.recipes.Container()),
		paneStack([]bool{false, true, false}, f.jobsLbl, f.jobs.Container(), collectBar),
	)
	body := widget.NewContainer(
		widget.ContainerOpts.Layout(widget.NewGridLayout(
			widget.GridLayoutOpts.Columns(1),
			widget.GridLayoutOpts.Stretch([]bool{true}, []bool{false, true}),
			widget.GridLayoutOpts.Spacing(0, theme.Px(4)),
			widget.GridLayoutOpts.Padding(widget.NewInsetsSimple(theme.Px(theme.ContentsPad))),
		)),
	)
	body.AddChild(f.craftLbl, f.split.Container())

	title, titleText := theme.NewTitleBar(env, "Manufacturing", "svc_manufacturing")
	f.title = titleText

	// The body panel is the header band's: it paints the panel, the
	// manufacturing plate over it and then the body (docs/ART.md "Window
	// headers").
	panel := theme.WindowBodyBox(p, theme.WindowBody(p.BG), theme.NRGBA(p.Border, 200))

	f.win = widget.NewWindow(
		widget.WindowOpts.Contents(newHeaderBand(body, panel, render.ManufacturingHeader)),
		widget.WindowOpts.TitleBar(title, theme.Px(theme.TitleBarH)),
		// Content floor: the craft line, a few rows of each table and the
		// job toolbar.
		widget.WindowOpts.MinSize(theme.Px(420), theme.Px(440)),
		// Construction landing rect: the game centers the window on first
		// open (placeDefaults, AnchorCenter); the row declares the same
		// rect (ManufacturingRect) as its size.
		widget.WindowOpts.Location(pxRect(ManufacturingRect())),
	)
	return f
}

// ManufacturingRect is the manufacturing window's landing rect in design
// units: the one number its Screen row's DefaultRect and its constructor
// share.
func ManufacturingRect() image.Rectangle { return image.Rect(312, 120, 792, 600) }

// Win returns the manufacturing window for Kit.AddWindow.
func (f *Manufacturing) Win() *widget.Window { return f.win }

// Splits names the window's draggable seam for the layout sidecar (the
// job-list pane below it).
func (f *Manufacturing) Splits() []widgets.Seam {
	return []widgets.Seam{{Name: "manufacturing-jobs", Split: f.split, Index: 0}}
}

// Tables are the window's tables for the layout sidecar: the recipe
// catalog and the job list are separate lists.
func (f *Manufacturing) Tables() []*widgets.Table {
	return []*widgets.Table{f.recipes, f.jobs}
}

// canonicalFirst is the market table's sort rule: the given key keeps the
// rows in the order Sync delivered them (canonical sim order), any other
// column sorts its cells lexicographically.
func canonicalFirst(key string, cols []widgets.Column) func(string, bool, []widgets.TableRow) {
	return func(k string, asc bool, rows []widgets.TableRow) {
		if k == key {
			if !asc {
				slices.Reverse(rows)
			}
			return
		}
		idx := -1
		for i, c := range cols {
			if c.Key == k {
				idx = i
				break
			}
		}
		if idx < 0 {
			return
		}
		sort.SliceStable(rows, func(a, b int) bool {
			x, y := rows[a].Cells[idx], rows[b].Cells[idx]
			if asc {
				return x < y
			}
			return y < x
		})
	}
}

// Sync refreshes the recipe board, the job list and every gate from the
// simulation. Call it every frame before Kit.Update.
func (f *Manufacturing) Sync() {
	f.split.Poll(f.ptr)
	s := f.cv
	station := s.DockedIndustry()
	// The board quotes one run: the chosen run count lives in the quantity
	// picker (its preview re-prices at the picked value), so the columns stay
	// a stable per-run reference.
	runs := 1

	// Recipe board: the content's canonical order, one row per recipe.
	recRows := make([]widgets.TableRow, 0, len(f.cv.Defs().Recipes))
	for _, rec := range f.cv.Defs().RecipesInOrder() {
		mats := s.JobMaterials(station, rec.ID, runs)
		row := manufacturingRow{recipe: rec.ID, ref: f.productRef(rec.Product)}
		recRows = append(recRows, widgets.TableRow{
			Cells: []string{
				f.cv.TradeableName(rec.Product),
				rec.Family,
				theme.GroupFloat(s.JobDuration(station, rec.ID, 1), 1) + " s",
				theme.FormatCredits(s.JobFee(rec.ID, runs)),
				inputsText(f.cv, mats),
			},
			Icons: []string{widgets.TradeableIcon(f.cv.Defs(), rec.Product), "", "", "", ""},
			Data:  row,
			Spec:  f.rowSpec(row),
		})
	}
	f.recipes.SetRows(recRows)
	f.recipes.Sync()

	// Job list: the PLAYER's jobs at every station (the global slot pool),
	// grouped by station and ascending by id within a group; only the
	// docked station's rows act.
	all := s.AllJobs()
	jobs := make([]sim.Job, 0, len(all))
	for _, j := range all {
		if j.Owner == sim.OwnerPlayer {
			jobs = append(jobs, j)
		}
	}
	sort.SliceStable(jobs, func(i, j int) bool {
		if jobs[i].Station != jobs[j].Station {
			return jobs[i].Station < jobs[j].Station
		}
		return jobs[i].ID < jobs[j].ID
	})
	jobRows := make([]widgets.TableRow, 0, len(jobs))
	for _, j := range jobs {
		row := manufacturingRow{job: j.ID, ref: f.productRef(f.productOfJob(j))}
		jobRows = append(jobRows, widgets.TableRow{
			Cells: []string{
				f.cv.TradeableName(f.productOfJob(j)),
				s.StationName(j.Station),
				theme.FormatInt(int64(j.Runs)),
				f.progressOf(j),
				jobStateLabel(j.State),
			},
			Icons: []string{widgets.TradeableIcon(f.cv.Defs(), f.productOfJob(j)), "", "", "", ""},
			Data:  row,
			Spec:  f.rowSpec(row),
		})
	}
	f.jobs.SetRows(jobRows)
	f.jobs.Sync()

	// Revalidate the cached selection against the rows themselves (a
	// recipe/job that left a table must stop gating the actions).
	f.hasSel = false
	if r, ok := f.recipes.Selected(); ok {
		if row, isRow := r.Data.(manufacturingRow); isRow && row.recipe != "" {
			f.sel, f.hasSel = row, true
		}
	}
	if r, ok := f.jobs.Selected(); ok {
		if row, isRow := r.Data.(manufacturingRow); isRow && row.job != 0 {
			f.sel, f.hasSel = row, true
		}
	}
	if !f.hasSel {
		f.sel = manufacturingRow{} // a vanished row must stop gating the actions
	}

	// Readouts and gates (visible-but-grayed, never hidden).
	slots := sim.IndustrySlots()
	if station != "" {
		f.craftLbl.Label = "Craft — hulls L" + theme.FormatInt(int64(s.CraftLevel(station, content.FamilyHulls))) +
			" · modules L" + theme.FormatInt(int64(s.CraftLevel(station, content.FamilyModules))) +
			" · charges L" + theme.FormatInt(int64(s.CraftLevel(station, content.FamilyCharges))) +
			" · time −4%/lvl · mats −2%/lvl"
	} else {
		f.craftLbl.Label = "Craft — no industry service here"
	}
	slotsUsed := len(all)
	f.jobsLbl.Label = "Jobs — " + theme.FormatInt(int64(slotsUsed)) + "/" + theme.FormatInt(int64(slots)) + " slots"

	f.collect.GetWidget().Disabled = !f.collectReadyFor(f.sel)
	f.cancel.GetWidget().Disabled = !f.cancelReadyFor(f.sel)

	// The title is the base display name only (a window title is
	// furniture): the selected recipe, its duration, fee, craft level and
	// the job slots are the body's own readouts.
	f.title.Label = "Manufacturing"
}

// inputsText renders the have/need line of a job's materials: one
// "Name have/need" pair per input, joined by the table's ellipsis rules.
// The board and the picker's preview share it.
func inputsText(cv widgets.ContentView, mats []content.CargoStack) string {
	if len(mats) == 0 {
		return "—"
	}
	out := ""
	for i, m := range mats {
		if i > 0 {
			out += " · "
		}
		name := string(m.Item)
		if def, ok := cv.Defs().Item(m.Item); ok {
			name = def.Name
		}
		out += name + " " + theme.FormatInt(int64(cv.PlayerStock(m.Item))) +
			"/" + theme.FormatInt(int64(m.Qty))
	}
	return out
}

// productRef resolves a recipe's product for the row's Show Info entry.
func (f *Manufacturing) productRef(product content.ItemID) widgets.InfoRef {
	ref, _ := widgets.ResolveTradeable(f.cv.Defs(), product).Ref()
	return ref
}

// productOfJob resolves a job's recipe product ("" when the recipe no
// longer loads — the sim keeps such jobs collectable-guarded).
func (f *Manufacturing) productOfJob(j sim.Job) content.ItemID {
	if rec, ok := f.cv.Defs().Recipes[j.Recipe]; ok {
		return rec.Product
	}
	return ""
}

// progressOf renders a job's progress: "100%" once done, else the elapsed
// fraction of its clock window.
func (f *Manufacturing) progressOf(j sim.Job) string {
	if j.State == sim.JobDone {
		return "100%"
	}
	span := j.DoneAt - j.StartedAt
	if span <= 0 {
		return "100%"
	}
	pct := (f.cv.Time() - j.StartedAt) / span
	if pct < 0 {
		pct = 0
	}
	if pct > 1 {
		pct = 1
	}
	return strconv.Itoa(int(pct*100)) + "%"
}

// jobStateLabel is the jobs table's State cell.
func jobStateLabel(state sim.JobState) string {
	if state == sim.JobDone {
		return "Ready"
	}
	return "Running"
}

// startReadyFor is the Start gate for one recipe row: the sim would accept
// it right now at one run (CanStartJob covers the industry service, the
// slots and the materials) — the picker re-checks its own run count.
func (f *Manufacturing) startReadyFor(row manufacturingRow) bool {
	if row.recipe == "" {
		return false
	}
	return f.cv.CanStartJob(f.cv.DockedIndustry(), row.recipe, 1)
}

// collectReadyFor is the Collect gate for one job row: the docked
// station's own, finished, player-owned job.
func (f *Manufacturing) collectReadyFor(row manufacturingRow) bool {
	if row.job == 0 {
		return false
	}
	for _, j := range f.cv.AllJobs() {
		if j.ID == row.job {
			return j.Station == f.cv.DockedIndustry() && j.State == sim.JobDone &&
				j.Owner == sim.OwnerPlayer
		}
	}
	return false
}

// cancelReadyFor is the Cancel gate for one job row: a running,
// player-owned job at the docked station (a finished one is collected).
func (f *Manufacturing) cancelReadyFor(row manufacturingRow) bool {
	if row.job == 0 {
		return false
	}
	for _, j := range f.cv.AllJobs() {
		if j.ID == row.job {
			return j.Station == f.cv.DockedIndustry() && j.State == sim.JobRunning &&
				j.Owner == sim.OwnerPlayer
		}
	}
	return false
}

// rowSpec is one manufacturing row's declared interactions: the product's
// Show Info (grayed when the id no longer resolves) plus its one action
// (Start for a recipe, Collect or Cancel for a job) with the same gate as
// the toolbar button.
func (f *Manufacturing) rowSpec(row manufacturingRow) widgets.RowSpec {
	return widgets.RowSpec{Ref: &row.ref, Actions: f.rowActions(row)}
}

// rowActions is a row's context-menu action entry (Start for a recipe,
// Collect for a done job, Cancel for a running one) with the same gate as
// the selection path: Start opens the quantity picker, never a fixed run
// count.
func (f *Manufacturing) rowActions(row manufacturingRow) []widgets.RowAction {
	switch {
	case row.recipe != "":
		return []widgets.RowAction{{Label: "Start", Disabled: !f.startReadyFor(row),
			Intent: widgets.QtyPick{Mode: widgets.QtyStart, Item: f.recipeProduct(row.recipe), Recipe: row.recipe}}}
	case row.job != 0:
		if f.collectReadyFor(row) {
			return []widgets.RowAction{{Label: "Collect",
				Intent: widgets.CollectJob{Job: row.job}}}
		}
		return []widgets.RowAction{{Label: "Cancel", Disabled: !f.cancelReadyFor(row),
			Intent: widgets.CancelJob{Job: row.job}}}
	}
	return []widgets.RowAction{{Label: "Start", Disabled: true}}
}

// recipeProduct is a recipe's product id ("" when the recipe no longer
// loads) — the picker's title and item line.
func (f *Manufacturing) recipeProduct(rec content.RecipeID) content.ItemID {
	if def, ok := f.cv.Defs().Recipes[rec]; ok {
		return def.Product
	}
	return ""
}

// openPicker opens the quantity picker on a recipe's runs — the selection
// gesture. Nothing opens away from an industry station.
func (f *Manufacturing) openPicker(row manufacturingRow) {
	if row.recipe == "" || f.cv.DockedIndustry() == "" {
		return
	}
	f.as.Do(widgets.QtyPick{Mode: widgets.QtyStart, Item: f.recipeProduct(row.recipe), Recipe: row.recipe})
}

// collectClicked emits CollectJob for the selected job.
func (f *Manufacturing) collectClicked() { f.collectWith(f.sel) }

// collectWith emits CollectJob for one job row (gated even when called
// directly).
func (f *Manufacturing) collectWith(row manufacturingRow) {
	if !f.collectReadyFor(row) {
		return
	}
	f.as.Do(widgets.CollectJob{Job: row.job})
}

// cancelClicked emits CancelJob for the selected job.
func (f *Manufacturing) cancelClicked() { f.cancelWith(f.sel) }

// cancelWith emits CancelJob for one job row (gated even when called
// directly).
func (f *Manufacturing) cancelWith(row manufacturingRow) {
	if !f.cancelReadyFor(row) {
		return
	}
	f.as.Do(widgets.CancelJob{Job: row.job})
}
