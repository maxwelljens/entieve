package uikit

import (
	"image"
	"slices"
	"sort"
	"strconv"

	"github.com/ebitenui/ebitenui/widget"
	"github.com/entieve/entieve/content"
	"github.com/entieve/entieve/sim"
	"github.com/entieve/entieve/uikit/icons"
)

// Factory is the station factory window: the manufacturing recipe board
// (product name + icon, family, per-run time at the docked station's
// current craft level, inputs with have/need counts) over the quantity
// stepper and the Start action, plus the station's job list with per-job
// progress and the Collect action — in the market panel's shape (two
// tables, one toolbar each, rows wired for Show Info).
//
// Gating follows the game's menu convention: controls stay VISIBLE and
// gray out (widget Disabled) when the player is undocked, the station
// offers no industry service, the materials are short or the slots are
// full — never hidden. Every action enqueues a sim.Command (the kit never
// mutates sim state); the sim logs the refusal line if a gated action
// still gets through.
type Factory struct {
	env Env
	win *widget.Window

	recipes *Table
	jobs    *Table

	craftLbl *widget.Text
	jobsLbl  *widget.Text
	qtyLabel *widget.Text
	qtyBar   *widget.Container // stepper row: relaid when the readout changes
	title    *widget.Text

	minus, plus *widget.Button
	start       *widget.Button
	collect     *widget.Button

	qtyIdx int
	sel    factoryRow
	hasSel bool
}

// factoryRow is the factory tables' row payload: the recipe id (recipe
// rows) or job id (job rows) plus the product ref both resolve Show Info
// through (InfoRefOf handles the type; see infowin.go).
type factoryRow struct {
	recipe content.RecipeID
	job    uint64
	ref    InfoRef
}

// NewFactory builds the Factory window over the shared environment. Mount
// it on a Kit with Kit.AddWindow and call Sync every frame.
func NewFactory(env Env) *Factory {
	f := &Factory{env: env}
	p := env.Pal

	recipeCols := []Column{
		{Key: "Product", Title: "Product", Priority: 0},
		{Key: "Family", Title: "Family", Priority: 1},
		{Key: "Time", Title: "Time", Priority: 2, AlignRight: true},
		{Key: "Inputs", Title: "Inputs", Priority: 3},
	}
	f.recipes = NewTable(env, recipeCols)
	jobCols := []Column{
		{Key: "Product", Title: "Product", Priority: 0},
		{Key: "Runs", Title: "Runs", Priority: 3, AlignRight: true},
		{Key: "Progress", Title: "Progress", Priority: 2, AlignRight: true},
		{Key: "State", Title: "State", Priority: 1},
	}
	f.jobs = NewTable(env, jobCols)

	// Canonical display order: the sim's own RecipesInOrder / job-id
	// order (reversing on a second header click, any other header click
	// falls back to plain cell sorting — the market table's rule).
	f.recipes.SetSort("Product", true)
	f.recipes.SetSortFunc(canonicalFirst("Product", recipeCols))
	f.jobs.SetSort("Product", true)
	f.jobs.SetSortFunc(canonicalFirst("Product", jobCols))

	attachInfo := func(t *Table) {
		t.OnSelect(func(r TableRow) {
			if row, ok := r.Data.(factoryRow); ok {
				f.sel, f.hasSel = row, true
			}
		})
		t.OnActivate(func(r TableRow) { showInfoData(env, r.Data) })
		t.OnMenu(func(r TableRow, at image.Point) {
			ref, ok := env.InfoRefOf(r.Data)
			row, isRow := r.Data.(factoryRow)
			if isRow {
				f.sel, f.hasSel = row, true
				t.SelectData(r.Data)
			}
			entries := []MenuEntry{showInfoMenuEntry(env, ref, ok)}
			if isRow {
				entries = append(entries, MenuSep, f.rowAction(row))
			}
			env.OpenMenu(entries, at)
		})
	}
	attachInfo(f.recipes)
	attachInfo(f.jobs)

	f.craftLbl = widget.NewText(
		widget.TextOpts.Text("Craft", env.Face(StepTiny), solid(p.Accent)),
	)
	f.jobsLbl = widget.NewText(
		widget.TextOpts.Text("Jobs", env.Face(StepTiny), solid(p.Accent)),
	)
	f.qtyLabel = widget.NewText(
		widget.TextOpts.Text("Qty 1", env.FaceMono(StepSmall), solid(p.Accent)),
		widget.TextOpts.WidgetOpts(widget.WidgetOpts.LayoutData(widget.RowLayoutData{
			Position: widget.RowLayoutPositionCenter,
		})),
	)

	newBtn := func(label string, clicked func()) *widget.Button {
		return widget.NewButton(
			widget.ButtonOpts.TextLabel(label),
			widget.ButtonOpts.WidgetOpts(
				widget.WidgetOpts.MinSize(Px(40), Px(40)),
				widget.WidgetOpts.LayoutData(widget.RowLayoutData{Stretch: true}),
			),
			widget.ButtonOpts.ClickedHandler(func(*widget.ButtonClickedEventArgs) { clicked() }),
		)
	}
	f.minus = newBtn("-", func() { f.stepQty(-1) })
	f.plus = newBtn("+", func() { f.stepQty(1) })
	f.start = newBtn("Start", f.startClicked)
	f.collect = newBtn("Collect", f.collectClicked)

	startBar := widget.NewContainer(
		widget.ContainerOpts.Layout(widget.NewRowLayout(
			widget.RowLayoutOpts.Direction(widget.DirectionHorizontal),
			widget.RowLayoutOpts.Spacing(Px(contentsPad)),
		)),
	)
	startBar.AddChild(f.minus, f.qtyLabel, f.plus, f.start)
	f.qtyBar = startBar
	collectBar := widget.NewContainer(
		widget.ContainerOpts.Layout(widget.NewRowLayout(
			widget.RowLayoutOpts.Direction(widget.DirectionHorizontal),
			widget.RowLayoutOpts.Spacing(Px(contentsPad)),
		)),
	)
	collectBar.AddChild(f.collect)

	// Body: craft/readout lines over the two table sections and their
	// toolbars; the grid gives each table its bounded cell (their own
	// preferred height never grows the window).
	body := widget.NewContainer(
		widget.ContainerOpts.BackgroundImage(bordered(solid(p.BG), nrgba(p.Border, 200), 1)),
		widget.ContainerOpts.Layout(widget.NewGridLayout(
			widget.GridLayoutOpts.Columns(1),
			widget.GridLayoutOpts.Stretch([]bool{true},
				[]bool{false, true, false, false, true, false}),
			widget.GridLayoutOpts.Spacing(0, Px(4)),
			widget.GridLayoutOpts.Padding(widget.NewInsetsSimple(Px(contentsPad))),
		)),
	)
	body.AddChild(f.craftLbl, f.recipes.Container(), startBar,
		f.jobsLbl, f.jobs.Container(), collectBar)

	title, factoryTitle := NewTitleBar(env, "Factory", "row_module")
	f.title = factoryTitle

	f.win = widget.NewWindow(
		widget.WindowOpts.Contents(body),
		widget.WindowOpts.TitleBar(title, Px(titleBarH)),
		widget.WindowOpts.Draggable(),
		// Content floor: the craft line, a few rows of each table and
		// both toolbars.
		widget.WindowOpts.MinSize(Px(360), Px(430)),
		// Fallback only: the composed rect at the 1024×700 minimum (the
		// game re-centers it from placeDefaults).
		widget.WindowOpts.Location(image.Rect(Px(332), Px(130), Px(732), Px(560))),
	)
	return f
}

// Win returns the factory window for Kit.AddWindow.
func (f *Factory) Win() *widget.Window { return f.win }

// canonicalFirst is the market table's sort rule: the given key keeps the
// rows in the order Sync delivered them (canonical sim order), any other
// column sorts its cells lexicographically.
func canonicalFirst(key string, cols []Column) func(string, bool, []TableRow) {
	return func(k string, asc bool, rows []TableRow) {
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
func (f *Factory) Sync() {
	s := f.env.Sim
	station := s.DockedIndustry()
	runs := f.qty()

	// Recipe board: the content's canonical order, one row per recipe.
	recRows := make([]TableRow, 0, len(f.env.Content.Recipes))
	for _, rec := range f.env.Content.RecipesInOrder() {
		mats := s.JobMaterials(station, rec.ID, runs)
		recRows = append(recRows, TableRow{
			Cells: []string{
				itemLabel(f.env, rec.Product),
				rec.Family,
				groupFloat(s.JobDuration(station, rec.ID, 1), 1) + " s",
				f.inputsCell(mats),
			},
			Icons: []string{productIcon(f.env, rec.Product), "", "", ""},
			Data:  factoryRow{recipe: rec.ID, ref: f.productRef(rec.Product)},
		})
	}
	f.recipes.SetRows(recRows)
	f.recipes.Sync()

	// Job list: the docked station's own jobs, in job-id order.
	jobs := s.Jobs(station)
	jobRows := make([]TableRow, 0, len(jobs))
	for _, j := range jobs {
		jobRows = append(jobRows, TableRow{
			Cells: []string{
				itemLabel(f.env, f.productOfJob(j)),
				FormatInt(int64(j.Runs)),
				f.progressOf(j),
				jobStateLabel(j.State),
			},
			Icons: []string{productIcon(f.env, f.productOfJob(j)), "", "", ""},
			Data:  factoryRow{job: j.ID, ref: f.productRef(f.productOfJob(j))},
		})
	}
	f.jobs.SetRows(jobRows)
	f.jobs.Sync()

	// Revalidate the cached selection against the rows themselves (a
	// recipe/job that left a table must stop gating the actions).
	f.hasSel = false
	if r, ok := f.recipes.Selected(); ok {
		if row, isRow := r.Data.(factoryRow); isRow && row.recipe != "" {
			f.sel, f.hasSel = row, true
		}
	}
	if r, ok := f.jobs.Selected(); ok {
		if row, isRow := r.Data.(factoryRow); isRow && row.job != 0 {
			f.sel, f.hasSel = row, true
		}
	}
	if !f.hasSel {
		f.sel = factoryRow{} // a vanished row must stop gating the actions
	}

	// Readouts and gates (visible-but-grayed, never hidden).
	slots := sim.IndustrySlots()
	if station != "" {
		f.craftLbl.Label = "Craft — hulls L" + FormatInt(int64(s.CraftLevel(station, content.FamilyHulls))) +
			" · modules L" + FormatInt(int64(s.CraftLevel(station, content.FamilyModules))) +
			" · charges L" + FormatInt(int64(s.CraftLevel(station, content.FamilyCharges)))
	} else {
		f.craftLbl.Label = "Craft — no industry service here"
	}
	slotsUsed := len(jobs)
	f.jobsLbl.Label = "Jobs — " + FormatInt(int64(slotsUsed)) + "/" + FormatInt(int64(slots)) + " slots"

	setStepperValue(f.qtyBar, f.qtyLabel, "Qty "+FormatInt(int64(runs)))
	f.start.GetWidget().Disabled = !f.startReadyFor(f.sel)
	f.collect.GetWidget().Disabled = !f.collectReadyFor(f.sel)

	slotsLbl := FormatInt(int64(slotsUsed)) + "/" + FormatInt(int64(slots)) + " jobs"
	if f.hasSel && f.sel.recipe != "" {
		if rec, ok := f.env.Content.Recipes[f.sel.recipe]; ok {
			f.title.Label = "Factory — " + itemLabel(f.env, rec.Product) + " " +
				groupFloat(s.JobDuration(station, rec.ID, 1), 1) + " s · " + slotsLbl
			return
		}
	}
	f.title.Label = "Factory — " + slotsLbl
}

// inputsCell renders the have/need line of a job's materials: one
// "Name have/need" pair per input, joined by the table's ellipsis rules.
func (f *Factory) inputsCell(mats []content.CargoStack) string {
	if len(mats) == 0 {
		return "—"
	}
	out := ""
	for i, m := range mats {
		if i > 0 {
			out += " · "
		}
		name := string(m.Item)
		if def, ok := f.env.Content.Item(m.Item); ok {
			name = def.Name
		}
		out += name + " " + FormatInt(int64(f.env.Sim.PlayerStock(m.Item))) +
			"/" + FormatInt(int64(m.Qty))
	}
	return out
}

// productRef resolves a recipe's product for the row's Show Info entry.
func (f *Factory) productRef(product content.ItemID) InfoRef {
	ref, _ := f.env.refOfID(string(product))
	return ref
}

// productOfJob resolves a job's recipe product ("" when the recipe no
// longer loads — the sim keeps such jobs collectable-guarded).
func (f *Factory) productOfJob(j sim.Job) content.ItemID {
	if rec, ok := f.env.Content.Recipes[j.Recipe]; ok {
		return rec.Product
	}
	return ""
}

// progressOf renders a job's progress: "100%" once done, else the elapsed
// fraction of its clock window.
func (f *Factory) progressOf(j sim.Job) string {
	if j.State == sim.JobDone {
		return "100%"
	}
	span := j.DoneAt - j.StartedAt
	if span <= 0 {
		return "100%"
	}
	pct := (f.env.Sim.Time() - j.StartedAt) / span
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

// productIcon picks a recipe/job row's product glyph: hulls, module
// classes, and the neutral gear for plain items (charges, minerals).
func productIcon(env Env, product content.ItemID) string {
	if _, ok := env.Content.Ship(content.ShipID(product)); ok {
		return "row_hull"
	}
	if def, ok := env.Content.Module(content.ModuleID(product)); ok {
		return ModuleIcon(def.Kind)
	}
	if _, ok := env.Content.Item(product); ok {
		return icons.Fallback
	}
	return ""
}

// startReadyFor is the Start gate for one recipe row: the sim would accept
// it right now at the stepper quantity (CanStartJob covers the industry
// service, the slots and the materials).
func (f *Factory) startReadyFor(row factoryRow) bool {
	if row.recipe == "" {
		return false
	}
	return f.env.Sim.CanStartJob(f.env.Sim.DockedIndustry(), row.recipe, f.qty())
}

// collectReadyFor is the Collect gate for one job row: the docked
// station's own, finished, player-owned job.
func (f *Factory) collectReadyFor(row factoryRow) bool {
	if row.job == 0 {
		return false
	}
	for _, j := range f.env.Sim.Jobs(f.env.Sim.DockedIndustry()) {
		if j.ID == row.job {
			return j.State == sim.JobDone && j.Owner == sim.OwnerPlayer
		}
	}
	return false
}

// rowAction is a row's context-menu action entry (Start for a recipe,
// Collect for a job) with the same gate as the toolbar button.
func (f *Factory) rowAction(row factoryRow) MenuEntry {
	switch {
	case row.recipe != "":
		return MenuEntry{Label: "Start", Disabled: !f.startReadyFor(row),
			Action: func() { f.startWith(row) }}
	case row.job != 0:
		return MenuEntry{Label: "Collect", Disabled: !f.collectReadyFor(row),
			Action: func() { f.collectWith(row) }}
	}
	return MenuEntry{Label: "Start", Disabled: true, Action: func() {}}
}

// stepQty moves the quantity stepper one step (clamped).
func (f *Factory) stepQty(delta int) {
	f.qtyIdx += delta
	if f.qtyIdx < 0 {
		f.qtyIdx = 0
	}
	if f.qtyIdx >= len(qtySteps) {
		f.qtyIdx = len(qtySteps) - 1
	}
	setStepperValue(f.qtyBar, f.qtyLabel, "Qty "+FormatInt(int64(f.qty())))
}

// qty is the current run count the stepper drives.
func (f *Factory) qty() int { return qtySteps[f.qtyIdx] }

// startClicked enqueues CmdStartJob for the selected recipe and quantity.
func (f *Factory) startClicked() { f.startWith(f.sel) }

// startWith enqueues the start of one recipe row (gated even when called
// directly, the market buttons' rule).
func (f *Factory) startWith(row factoryRow) {
	if !f.startReadyFor(row) {
		return
	}
	f.env.Enqueue(sim.CmdStartJob{Recipe: row.recipe, Runs: f.qty()})
}

// collectClicked enqueues CmdCollectJob for the selected job.
func (f *Factory) collectClicked() { f.collectWith(f.sel) }

// collectWith enqueues the collect of one job row (gated even when called
// directly).
func (f *Factory) collectWith(row factoryRow) {
	if !f.collectReadyFor(row) {
		return
	}
	f.env.Enqueue(sim.CmdCollectJob{ID: row.job})
}
