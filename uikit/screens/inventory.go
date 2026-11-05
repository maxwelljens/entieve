package screens

import (
	"image"
	"image/color"
	"math"
	"strconv"

	"github.com/ebitenui/ebitenui/widget"
	"github.com/entieve/entieve/content"
	"github.com/entieve/entieve/sim"
	"github.com/entieve/entieve/uikit/icons"
	"github.com/entieve/entieve/uikit/theme"
	"github.com/entieve/entieve/uikit/widgets"
	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/text/v2"
	"github.com/hajimehoshi/ebiten/v2/vector"
)

// Thumbnail-view geometry (design px): a tile is the rack/fitting cell's
// icon box (theme.IconTile) plus the name and count lines under it, so the
// two views draw items at the same glyph size.
const (
	tileW   = theme.IconTile + 22 // the icon box and the tile's side padding
	tileH   = theme.IconTile + 48 // the icon plus a name line and a count line (their floor; text may grow it)
	tileGap = 6                   // between tiles
)

// Inventory is the storage window: Cargo / Ore hold / Station hangar as
// Item/Qty/m³ tables (or as square item tiles) with a fullness bar per
// hold, a view toggle in the title bar beside the × and one toolbar verb
// (Sell). Selection is the Selection set (CONTEXT.md): a plain press
// selects one stack (a press on a member keeps the set, which is what
// drags it), Shift takes the range from the anchor and Ctrl toggles, so a
// drop or Sell can act on many stacks at once: every selected stack moves
// or sells WHOLE with no prompts, while one selected stack keeps its
// quantity picker. Each section is a split pane, so its height is dragged
// at the seam.
//
// Drag-and-drop is the transfer gesture (CONTEXT.md "Transfer popup"):
// press on a stack row/tile, a ghost chip follows the pointer (naming the
// stack count when the drag carries more than one), the hold under it
// lights as the drop target, and a release over ANOTHER hold moves the
// selection there ("to Station hangar"). Release on the same section or
// empty space is a no-op, as is a drop on the undocked hangar.
type Inventory struct {
	th  theme.Theme
	cv  widgets.ContentView
	as  widgets.ActionSink
	ptr *widgets.Pointer
	win *widget.Window

	body  *widget.Container // the split plus the toolbar
	pane  *widget.Container // the window's content: body under the drag ghost
	split *widgets.SplitPane

	cargo, ore, hangar *widgets.Table

	cargoLbl, oreLbl, hangLbl *widget.Text
	cargoBar, oreBar          *widgets.ValueBar

	cargoBody, oreBody, hangBody    *widget.Container
	cargoTiles, oreTiles, hangTiles *tileGrid
	hatch                           *hatchView

	cargoPane, orePane, hangPane    *widget.Container
	cargoFrame, oreFrame, hangFrame *widget.Container

	view     *widget.Button
	viewIcon *theme.IconBox
	viewTip  *widgets.TipView
	sell     *widget.Button

	sel    content.ItemID
	src    string // hold the selection lives in
	hasSel bool

	// The Selection set (CONTEXT.md): the selected stacks as membership
	// inside src, the Shift anchor (the last plain or Ctrl click, per
	// hold) and the press's own modifier sample. A gesture applies once:
	// the press claims it (pressSeen/pressCtrl) and the row's click at
	// release re-reads that press's sample instead of the keys' current
	// state, so a Ctrl toggle can never run twice and a Shift range
	// survives releasing Shift before the button.
	selSet      map[content.ItemID]bool
	anchor      content.ItemID
	anchorHold  string
	gestureMods widgets.Mods
	pressSeen   bool
	pressCtrl   bool

	tiles    bool // the view mode: false = tables, true = item tiles
	oreShown bool // the hull has an ore hold (the ore section exists)

	hangView string // what the hangar body holds: "table", "tiles" or "hatch"

	ghost    *dragGhost
	ghostAt  image.Point // the pointer, for the ghost's layout
	dragging bool
	dragHold string

	target string // the highlighted drop-target hold ("" = none)
}

// NewInventory builds the Inventory window over the shared environment.
// Mount it on a Kit with Kit.AddWindow and call Sync every frame.
func NewInventory(env widgets.Seams) *Inventory {
	inv := &Inventory{th: env, cv: env, as: env, ptr: env.Pointer()}
	p := env.Pal()

	cols := []widgets.Column{
		{Key: "Item", Title: "Item", Priority: 0},
		{Key: "Qty", Title: "Qty", Priority: 1, AlignRight: true},
		{Key: "Vol", Title: "m³", Icon: "stat_cargo", Priority: 2, AlignRight: true},
	}
	// One selection callback per section: the source hold of the next
	// transfer is whichever table the row was clicked in. Contents face:
	// these rows are names, quantities and volumes — IBM Plex Mono
	// lines the columns up (section labels and buttons stay IBM Plex Sans).
	mono := widgets.ContentsFace(env.FaceMono(theme.StepSmall))
	inv.cargo = widgets.NewTable(env, cols, mono, widgets.PrefsID("inventory"))
	inv.cargo.OnSelect(inv.selectFrom("cargo"))
	inv.ore = widgets.NewTable(env, cols, mono, widgets.PrefsID("inventory"))
	inv.ore.OnSelect(inv.selectFrom("ore"))
	inv.hangar = widgets.NewTable(env, cols, mono, widgets.PrefsID("inventory"))
	inv.hangar.OnSelect(inv.selectFrom("hangar"))
	// Inventory rows carry no other action: their RowSpec declares the
	// Show Info ref only (built in rowsOf), so double-click swaps the
	// Show Info window and right-click opens the one-entry menu — through
	// the installer, not hand wiring.

	inv.cargoTiles, inv.oreTiles, inv.hangTiles =
		newTileGrid(env), newTileGrid(env), newTileGrid(env)
	inv.hatch = newHatchView(env)

	newLabel := func(s string) *widget.Text {
		return widget.NewText(
			widget.TextOpts.Text(s, env.Face(theme.StepTiny), theme.Solid(p.Accent)),
		)
	}
	inv.cargoLbl = newLabel("Cargo")
	inv.oreLbl = newLabel("Ore hold")
	inv.hangLbl = newLabel("Station hangar")

	// Fullness: the HUD's bar-with-the-figure-inside (widgets.ValueBar),
	// one per hold that has a capacity. The hangar is unlimited (the
	// refinery's doc), so it keeps only its stack-count label.
	inv.cargoBar = widgets.NewValueBar(env, p.Accent, 0)
	inv.oreBar = widgets.NewValueBar(env, p.Accent, 0)

	inv.cargoBody = newSectionBody()
	inv.oreBody = newSectionBody()
	inv.hangBody = newSectionBody()
	inv.cargoBody.AddChild(inv.cargo.Container())
	inv.oreBody.AddChild(inv.ore.Container())
	inv.hangBody.AddChild(inv.hangar.Container())

	inv.view, inv.viewIcon, inv.viewTip = inv.viewToggle()
	inv.sell = theme.ActionButton("Sell", inv.sellClicked)

	// One toolbar: the one non-move verb left (Sell acts on the selected
	// stack). The view toggle is title-bar furniture now; moves are the
	// drag gesture.
	toolbar := widget.NewContainer(widget.ContainerOpts.Layout(widget.NewRowLayout(
		widget.RowLayoutOpts.Direction(widget.DirectionHorizontal),
		widget.RowLayoutOpts.Spacing(theme.Px(theme.ContentsPad)),
	)))
	toolbar.AddChild(inv.sell)
	inv.body = widget.NewContainer(
		widget.ContainerOpts.BackgroundImage(theme.WindowBodyBox(p, theme.WindowBody(p.BG), theme.NRGBA(p.Border, 200))),
		widget.ContainerOpts.Layout(widget.NewGridLayout(
			widget.GridLayoutOpts.Columns(1),
			widget.GridLayoutOpts.Stretch([]bool{true}, []bool{true, false}),
			widget.GridLayoutOpts.Spacing(0, theme.Px(4)),
			widget.GridLayoutOpts.Padding(widget.NewInsetsSimple(theme.Px(theme.ContentsPad))),
		)),
	)

	// The window content: the body, with the drag ghost laid over it (the
	// custom layout keeps the ghost out of the body's own grid).
	inv.ghost = newDragGhost(env)
	inv.oreShown = inv.hasOreHold()
	inv.split = inv.newSplit()
	inv.body.AddChild(inv.split.Container(), toolbar)

	inv.pane = widget.NewContainer(widget.ContainerOpts.Layout(invOverlay{inv: inv}))
	inv.pane.AddChild(inv.body, inv.ghost.container)
	theme.ShowWidget(inv.ghost.container.GetWidget(), false)

	title, _ := theme.NewTitleBar(env, "Inventory", "svc_inventory")
	// The view switch is the title bar's one action: immediately left of
	// the ×, the glyph drawn over the button's chrome.
	TitleBarAction(title, inv.view, inv.viewIcon.Widget())

	inv.win = widget.NewWindow(
		widget.WindowOpts.Contents(inv.pane),
		widget.WindowOpts.TitleBar(title, theme.Px(theme.TitleBarH)),
		// Content floor: every fixed row plus a full row of each hold
		// section (the sections scroll below that). The width is the
		// toolbar's (Sell alone) and the window's landing rect below.
		widget.WindowOpts.MinSize(theme.Px(320), theme.Px(420)),
		// The landing rect the game's Inventory Screen row also declares
		// (InventoryRect), so it opens here on first use, docked and in
		// space alike.
		widget.WindowOpts.Location(pxRect(InventoryRect())),
	)
	return inv
}

// InventoryRect is the inventory window's landing rect in design units:
// the one number its Screen row's DefaultRect and its constructor share.
func InventoryRect() image.Rectangle { return image.Rect(396, 256, 716, 690) }

// Win returns the inventory window for Kit.AddWindow.
func (inv *Inventory) Win() *widget.Window { return inv.win }

// Splits names the window's draggable seams for the layout sidecar: each
// key names the hold pane below its seam. A hull without an ore hold has
// no ore pane and no seam above it, so only the hangar seam remains.
func (inv *Inventory) Splits() []widgets.Seam {
	if inv.oreShown {
		return []widgets.Seam{
			{Name: "inventory-ore", Split: inv.split, Index: 0},
			{Name: "inventory-hangar", Split: inv.split, Index: 1},
		}
	}
	return []widgets.Seam{
		{Name: "inventory-hangar", Split: inv.split, Index: 0},
	}
}

// Tables are the window's tables for the layout sidecar. The three holds
// share one column set (they are the same three columns over the same
// holds), so a width dragged in one of them applies to all three. A hull
// without an ore hold still registers the ore table: its column prefs are
// the player's, not the hull's.
func (inv *Inventory) Tables() []*widgets.Table {
	return []*widgets.Table{inv.cargo, inv.ore, inv.hangar}
}

// ViewTiles reports the view mode (tests and the capture read it).
func (inv *Inventory) ViewTiles() bool { return inv.tiles }

// hatchShown reports whether the hangar body currently holds the undocked
// hatch (tests and the capture read it).
func (inv *Inventory) hatchShown() bool {
	kids := inv.hangBody.Children()
	return len(kids) == 1 && kids[0].GetWidget() == inv.hatch.Container().(*widget.Container).GetWidget()
}

// paneStack stacks one split pane's rows in a single column: the rows
// marked stretch take the pane's leftover height (its scroll body), the
// rest keep their preferred height (labels, bars, toolbars). Each body
// keeps its own ScrollRail gutter.
func paneStack(stretch []bool, rows ...widget.PreferredSizeLocateableWidget) *widget.Container {
	c := widget.NewContainer(widget.ContainerOpts.Layout(widget.NewGridLayout(
		widget.GridLayoutOpts.Columns(1),
		widget.GridLayoutOpts.Stretch([]bool{true}, stretch),
		widget.GridLayoutOpts.Spacing(0, theme.Px(4)),
	)))
	c.AddChild(rows...)
	return c
}

// newSectionBody is one hold's swap surface: a grid cell holding the
// section's current view (the table, the tile grid or the undocked hatch).
func newSectionBody() *widget.Container {
	return widget.NewContainer(widget.ContainerOpts.Layout(widget.NewGridLayout(
		widget.GridLayoutOpts.Columns(1),
		widget.GridLayoutOpts.Stretch([]bool{true}, []bool{true}),
	)))
}

// setBody replaces a section body's one child.
func setBody(body *widget.Container, child widget.PreferredSizeLocateableWidget) {
	if kids := body.Children(); len(kids) == 1 {
		body.ReplaceChild(kids[0], child)
		return
	}
	body.RemoveChildren()
	body.AddChild(child)
}

// sectionPane builds one hold's split cell: its label, its fullness bar
// (nil for the hangar) over its body, under a hidden accent frame the drag
// lights as the drop target.
func (inv *Inventory) sectionPane(lbl *widget.Text, bar *widgets.ValueBar, body *widget.Container) (widget.PreferredSizeLocateableWidget, *widget.Container, *widget.Container) {
	rows := []widget.PreferredSizeLocateableWidget{lbl}
	stretch := []bool{false}
	if bar != nil {
		rows = append(rows, bar.Container)
		stretch = append(stretch, false)
	}
	pane := paneStack(append(stretch, true), append(rows, body)...)
	p := inv.th.Pal()
	frame := widget.NewContainer(
		widget.ContainerOpts.BackgroundImage(theme.Bordered(theme.NRGBA(p.BGHard, 0), theme.Solid(p.Accent), theme.Px(1))),
	)
	theme.ShowWidget(frame.GetWidget(), false)
	wrap := widget.NewContainer(widget.ContainerOpts.Layout(widget.NewStackedLayout()))
	wrap.AddChild(pane, frame)
	return wrap, pane, frame
}

// newSplit builds the pane stack for the holds the hull has. The panes are
// the same section containers across rebuilds; only the seam list changes.
func (inv *Inventory) newSplit() *widgets.SplitPane {
	cargo, cargoPane, cargoFrame := inv.sectionPane(inv.cargoLbl, inv.cargoBar, inv.cargoBody)
	inv.cargoPane, inv.cargoFrame = cargoPane, cargoFrame
	panes := []widget.PreferredSizeLocateableWidget{cargo}
	if inv.oreShown {
		ore, orePane, oreFrame := inv.sectionPane(inv.oreLbl, inv.oreBar, inv.oreBody)
		inv.orePane, inv.oreFrame = orePane, oreFrame
		panes = append(panes, ore)
	} else {
		inv.orePane, inv.oreFrame = nil, nil
	}
	hang, hangPane, hangFrame := inv.sectionPane(inv.hangLbl, nil, inv.hangBody)
	inv.hangPane, inv.hangFrame = hangPane, hangFrame
	panes = append(panes, hang)
	return widgets.NewSplitPane(inv.th, panes...)
}

// rebuildSplit re-stacks the panes when the hull's ore hold appears or
// goes (a ship change): the old split is replaced in the body's first
// cell. Sidecar shares are not carried across a different pane count.
func (inv *Inventory) rebuildSplit() {
	old := inv.split
	inv.split = inv.newSplit()
	if old != nil {
		inv.body.ReplaceChild(old.Container(), inv.split.Container())
	}
}

// hasOreHold reads the hull's ore hold: a dedicated hold with capacity.
func (inv *Inventory) hasOreHold() bool {
	if inv.cv == nil {
		return false
	}
	cargo, ok := inv.cv.Cargo(inv.cv.Player())
	return ok && cargo.OreCapacity > 0
}

// viewToggle is the title bar's two-state view switch: an icon-only
// button naming the view it switches TO, with that view's manifest glyph
// (cubes for the thumbnail grid, the row glyph for the table list). The
// tooltip carries the word the toolbar label used to.
func (inv *Inventory) viewToggle() (*widget.Button, *theme.IconBox, *widgets.TipView) {
	p := inv.th.Pal()
	btn := widget.NewButton(
		widget.ButtonOpts.WidgetOpts(widget.WidgetOpts.LayoutData(widget.AnchorLayoutData{
			StretchHorizontal: true,
			StretchVertical:   true,
		})),
		widget.ButtonOpts.ClickedHandler(func(*widget.ButtonClickedEventArgs) { inv.toggleView() }),
	)
	box := theme.NewIconBox(theme.IconTitle)
	box.Set("stat_structure", p.Text)
	box.Widget().GetWidget().LayoutData = widget.AnchorLayoutData{
		HorizontalPosition: widget.AnchorLayoutPositionCenter,
		VerticalPosition:   widget.AnchorLayoutPositionCenter,
	}
	tip, view := widgets.NewTip(inv.th, widgets.TooltipModel{Title: "Tiles"})
	btn.GetWidget().ToolTips = []*widget.ToolTip{tip}
	return btn, box, view
}

// toggleView flips the whole inventory between the table list and the
// item-tile grid.
func (inv *Inventory) toggleView() {
	inv.tiles = !inv.tiles
	inv.applyView()
}

// applyView swaps every section's body for the current view and points the
// toggle at the view it switches to.
func (inv *Inventory) applyView() {
	p := inv.th.Pal()
	if inv.tiles {
		inv.viewIcon.Set("row_module", p.Text)
		inv.viewTip.SetModel(widgets.TooltipModel{Title: "List"})
	} else {
		inv.viewIcon.Set("stat_structure", p.Text)
		inv.viewTip.SetModel(widgets.TooltipModel{Title: "Tiles"})
	}
	setBody(inv.cargoBody, inv.viewOf("cargo"))
	if inv.oreShown {
		setBody(inv.oreBody, inv.viewOf("ore"))
	}
	inv.hangView = ""
	inv.applyHangar()
}

// viewOf is a hold's current view widget (the table or its tile grid).
func (inv *Inventory) viewOf(hold string) widget.PreferredSizeLocateableWidget {
	if inv.tiles {
		return inv.tilesFor(hold).Container()
	}
	return inv.tableFor(hold).Container()
}

// tilesFor is a hold's tile grid.
func (inv *Inventory) tilesFor(hold string) *tileGrid {
	switch hold {
	case "ore":
		return inv.oreTiles
	case "hangar":
		return inv.hangTiles
	default:
		return inv.cargoTiles
	}
}

// applyHangar shows the hangar's table/tiles while docked and the
// not-docked hatch while in space. The section stays visible and keeps its
// fixed actions (the picker's sell/transfer gates stay the engine's).
func (inv *Inventory) applyHangar() {
	mode := "table"
	if inv.tiles {
		mode = "tiles"
	}
	if !inv.cv.PlayerDocked() {
		mode = "hatch"
	}
	if mode == inv.hangView {
		return
	}
	inv.hangView = mode
	switch mode {
	case "hatch":
		setBody(inv.hangBody, inv.hatch.Container())
	case "tiles":
		setBody(inv.hangBody, inv.hangTiles.Container())
	default:
		setBody(inv.hangBody, inv.hangar.Container())
	}
}

// selectFrom builds the per-section selection callback: the row CLICK half
// of a selection gesture (tables fire it at release; the press half is
// stepDrag's, and tile view has no click half at all). It defers to the
// press when this gesture had one (the press's own modifier sample, and a
// Ctrl toggle the press already applied, which must not run again) and
// applies the frame's current sample when the click came with no press
// (a programmatic click, like the tests' row fire).
func (inv *Inventory) selectFrom(hold string) func(widgets.TableRow) {
	return func(r widgets.TableRow) {
		item, ok := r.Data.(content.ItemID)
		if !ok {
			return
		}
		mods := inv.ptr.Mods()
		if inv.pressSeen {
			m, ctrlDone := inv.gestureMods, inv.pressCtrl
			inv.pressSeen, inv.pressCtrl = false, false
			if ctrlDone {
				return
			}
			mods = m
		}
		order := inv.displayOrder(hold)
		idx := indexOf(order, item)
		if idx < 0 {
			return
		}
		inv.applySelect(hold, item, idx, mods, false)
	}
}

// applySelect is the one place a selection gesture lands (both halves, all
// views): the modifier sample decides between replacing the set (plain,
// unless the pressed stack is already a member), taking the display-order
// range from the Shift anchor and toggling membership (Ctrl, Ctrl+Shift
// adds the range instead of replacing it). A press in ANOTHER hold starts
// that hold's set: one hold owns the selection, because a transfer has one
// From. It ends by pushing the set to the tables, so the press's highlight
// paints in the same frame (as SelectData's single tint used to).
func (inv *Inventory) applySelect(hold string, item content.ItemID, idx int, mods widgets.Mods, fromPress bool) {
	shift, ctrl := mods&widgets.ModShift != 0, mods&widgets.ModCtrl != 0
	if fromPress {
		inv.gestureMods, inv.pressSeen, inv.pressCtrl = mods, true, ctrl && !shift
	}
	order := inv.displayOrder(hold)

	if hold != inv.src {
		inv.src = hold
		inv.selSet = map[content.ItemID]bool{item: true}
		inv.anchor, inv.anchorHold, inv.sel, inv.hasSel = item, hold, item, true
		inv.pushSelSet()
		return
	}

	if shift && inv.anchorHold == hold {
		if lo, hi, ok := rangeOf(order, inv.anchor, idx); ok {
			if ctrl {
				if inv.selSet == nil {
					inv.selSet = map[content.ItemID]bool{}
				}
				for _, id := range order[lo : hi+1] {
					inv.selSet[id] = true
				}
			} else {
				inv.selSet = make(map[content.ItemID]bool, hi-lo+1)
				for _, id := range order[lo : hi+1] {
					inv.selSet[id] = true
				}
			}
			inv.sel, inv.hasSel = item, true
			inv.pushSelSet()
			return
		}
		// The anchor left this hold (its stack moved away): fall through
		// to the plain rule, which re-anchors.
	}

	if ctrl {
		if inv.selSet == nil {
			inv.selSet = map[content.ItemID]bool{}
		}
		if inv.selSet[item] {
			delete(inv.selSet, item)
		} else {
			inv.selSet[item] = true
		}
		inv.anchor, inv.anchorHold = item, hold
		inv.reprimary()
		inv.hasSel = len(inv.selSet) > 0
		inv.pushSelSet()
		return
	}

	// Plain: a press on a member keeps the set (the set is what drags);
	// anything else collapses the selection to this one stack.
	if !inv.selSet[item] {
		inv.selSet = map[content.ItemID]bool{item: true}
	}
	inv.sel, inv.hasSel = item, true
	inv.anchor, inv.anchorHold = item, hold
	inv.pushSelSet()
}

// displayOrder is the hold's items as the current view shows them (the
// display-order rows of the list, the grid's order in the thumbnails): the
// order every range and every bulk walk takes.
func (inv *Inventory) displayOrder(hold string) []content.ItemID {
	if inv.tiles {
		return inv.tilesFor(hold).items
	}
	rows := inv.tableFor(hold).Rows
	out := make([]content.ItemID, 0, len(rows))
	for _, r := range rows {
		if id, ok := r.Data.(content.ItemID); ok {
			out = append(out, id)
		}
	}
	return out
}

// indexOf is item's position in the display order (-1 when it has no row:
// the stack left between the press and the click).
func indexOf(order []content.ItemID, item content.ItemID) int {
	for i, id := range order {
		if id == item {
			return i
		}
	}
	return -1
}

// rangeOf is the inclusive display-order span between the anchor and idx,
// reported as lo/hi slice bounds (ok when the anchor still has a row).
func rangeOf(order []content.ItemID, anchor content.ItemID, idx int) (int, int, bool) {
	a := indexOf(order, anchor)
	if a < 0 || idx < 0 || idx >= len(order) {
		return 0, 0, false
	}
	if a > idx {
		a, idx = idx, a
	}
	return a, idx, true
}

// reprimary points sel at a member that still exists (the first in
// display order), or clears it: sel is the chip's identity and the
// single-stack picker's target, and it must stay inside the set.
func (inv *Inventory) reprimary() {
	if inv.sel != "" && inv.selSet[inv.sel] {
		return
	}
	inv.sel = ""
	for _, id := range inv.displayOrder(inv.src) {
		if inv.selSet[id] {
			inv.sel = id
			break
		}
	}
}

// revalidateSel drops every member whose stack left its hold (a stack that
// moved away must stop gating the actions) and follows with the primary.
func (inv *Inventory) revalidateSel() {
	if len(inv.selSet) == 0 {
		inv.sel, inv.hasSel = "", false
		return
	}
	have := make(map[content.ItemID]bool, len(inv.selSet))
	for _, st := range inv.stacksOf(inv.src) {
		have[st.Item] = true
	}
	for id := range inv.selSet {
		if !have[id] {
			delete(inv.selSet, id)
		}
	}
	inv.hasSel = len(inv.selSet) > 0
	if !inv.hasSel {
		inv.sel = ""
		return
	}
	inv.reprimary()
}

// pushSelSet hands the set to all three tables: the owning section's
// members (in display order, so the repaint diff is stable) and an empty
// set elsewhere, which latches every inventory table into set-driven paint
// and keeps a stale click highlight from ever showing outside src.
func (inv *Inventory) pushSelSet() {
	members := []any{}
	if inv.src != "" {
		for _, id := range inv.displayOrder(inv.src) {
			if inv.selSet[id] {
				members = append(members, id)
			}
		}
	}
	inv.cargo.SetSelSet(membersIf(inv.src == "cargo", members))
	inv.ore.SetSelSet(membersIf(inv.src == "ore", members))
	inv.hangar.SetSelSet(membersIf(inv.src == "hangar", members))
}

// membersIf is the set for one section: the members when the section owns
// the selection, a cleared (non-nil) set otherwise.
func membersIf(own bool, members []any) []any {
	if own {
		return members
	}
	return []any{}
}

// setFor is one tile grid's membership map (nil outside src: no mark).
func (inv *Inventory) setFor(hold string) map[content.ItemID]bool {
	if inv.src != hold {
		return nil
	}
	return inv.selSet
}

// selectedStacks is the selection's stacks in display order, resolved
// against the hold's live stacks (the walk every bulk move and sale takes).
func (inv *Inventory) selectedStacks() []content.CargoStack {
	if len(inv.selSet) == 0 {
		return nil
	}
	stacks := inv.stacksOf(inv.src)
	var out []content.CargoStack
	for _, id := range inv.displayOrder(inv.src) {
		if !inv.selSet[id] {
			continue
		}
		for _, st := range stacks {
			if st.Item == id {
				out = append(out, st)
				break
			}
		}
	}
	return out
}

// tableFor returns the section table the hold names (the default keeps
// the initial state pointed at cargo, which has no selection).
func (inv *Inventory) tableFor(src string) *widgets.Table {
	switch src {
	case "ore":
		return inv.ore
	case "hangar":
		return inv.hangar
	default:
		return inv.cargo
	}
}

// Sync refreshes the three sections, their bars and the action gating, and
// steps the drag gesture. Call it every frame before Kit.Update.
func (inv *Inventory) Sync() {
	inv.split.Poll(inv.ptr)
	s := inv.cv
	player := s.Player()

	var cargo sim.Cargo
	cargo, hasCargo := s.Cargo(player)
	docked := s.PlayerDocked()

	// The hull changed its ore hold: the section (and its seam) comes or
	// goes with it.
	if ore := hasCargo && cargo.OreCapacity > 0; ore != inv.oreShown {
		inv.oreShown = ore
		inv.rebuildSplit()
		setBody(inv.oreBody, inv.viewOf("ore"))
	}

	inv.cargo.SetRows(inv.cargoRows(cargo, hasCargo))
	inv.ore.SetRows(inv.oreRows(cargo, hasCargo))
	inv.hangar.SetRows(inv.rowsOf(inv.hangarRows()))

	inv.cargoLbl.Label = "Cargo"
	if !inv.hasHold("cargo") {
		inv.cargoLbl.Label = "Cargo · no cargo hold"
	}
	inv.oreLbl.Label = "Ore hold"
	inv.hangLbl.Label = "Station hangar · " + theme.FormatInt(int64(len(inv.hangarRows()))) + " stacks"
	inv.setBar(inv.cargoBar, "cargo")
	inv.setBar(inv.oreBar, "ore")
	theme.ShowWidget(inv.cargoBar.Container.GetWidget(), inv.hasHold("cargo"))
	theme.ShowWidget(inv.oreBar.Container.GetWidget(), inv.hasHold("ore"))

	inv.cargo.Sync()
	inv.ore.Sync()
	inv.hangar.Sync()
	inv.applyHangar()
	if !docked {
		// The hatch fills the hangar body's current rect (one repaint per
		// size change).
		if r := inv.hangBody.GetWidget().Rect; r.Dx() > 0 && r.Dy() > 0 {
			inv.hatch.Paint(r.Dx(), r.Dy())
		}
	}
	if inv.tiles {
		if inv.oreShown {
			inv.oreTiles.Sync()
		}
		inv.cargoTiles.Sync()
		if docked {
			inv.hangTiles.Sync()
		}
	}

	// Revalidate the Selection set against the section's live stacks (a
	// stack that left the hold must stop gating the actions), push it to
	// the tables and mirror it into the tiles: the click-time set alone
	// would keep the actions armed on rows that no longer exist.
	inv.revalidateSel()
	inv.pushSelSet()
	if inv.tiles {
		inv.cargoTiles.Set(inv.stacksOf("cargo"), inv.setFor("cargo"))
		if inv.oreShown {
			inv.oreTiles.Set(inv.stacksOf("ore"), inv.setFor("ore"))
		}
		inv.hangTiles.Set(inv.stacksOf("hangar"), inv.setFor("hangar"))
	}

	// Action gating: selling needs a selection and a docked player (the
	// drag's transfer gates live in transfer()).
	inv.sell.GetWidget().Disabled = !inv.hasSel || !docked

	inv.stepDrag()
}

// hasHold reports whether a hold exists with a positive capacity (the bar
// carries a value only then).
func (inv *Inventory) hasHold(hold string) bool {
	_, cap, ok := inv.cv.Holds().Used(hold)
	return ok && cap > 0
}

// setBar paints one hold's fullness bar: used/capacity clamped 0..1, the
// same "used / capacity" figures the label carried before. Hidden by the
// caller when the hull has no such hold.
func (inv *Inventory) setBar(bar *widgets.ValueBar, hold string) {
	used, cap, ok := inv.cv.Holds().Used(hold)
	if !ok || cap <= 0 {
		bar.Set(0, "")
		return
	}
	bar.Set(math.Max(0, math.Min(1, used/cap)),
		theme.FormatVolumeShort(used)+" / "+theme.FormatVolumeShort(cap))
}

// cargoRows renders the general cargo hold (empty when the ship has
// none).
func (inv *Inventory) cargoRows(cargo sim.Cargo, hasCargo bool) []widgets.TableRow {
	if !hasCargo {
		return nil
	}
	return inv.rowsOf(cargo.Stacks)
}

// oreRows renders the dedicated ore hold (empty when the hull has none).
func (inv *Inventory) oreRows(cargo sim.Cargo, hasCargo bool) []widgets.TableRow {
	if !hasCargo || cargo.OreCapacity <= 0 {
		return nil
	}
	return inv.rowsOf(cargo.Ore)
}

// hangarRows is the station hangar's stacks (for the section total).
func (inv *Inventory) hangarRows() []content.CargoStack {
	return inv.cv.Hangar().Items
}

// stacksOf is one hold's stacks as the views and the drag read them.
func (inv *Inventory) stacksOf(hold string) []content.CargoStack {
	switch hold {
	case "ore":
		return inv.cv.Holds().Ore()
	case "hangar":
		return inv.cv.Hangar().Items
	default:
		return inv.cv.Holds().Cargo()
	}
}

// rowsOf converts stacks to Item/Qty/m³ table rows, each declaring the
// full-stack "Sell order" verb (grayed, never hidden, while the sim's market
// service is unavailable (orders rest on the docked station's book). The
// action carries the clicked stack's own quantity as the order window's
// sell default.
func (inv *Inventory) rowsOf(stacks []content.CargoStack) []widgets.TableRow {
	rows := make([]widgets.TableRow, 0, len(stacks))
	market := inv.cv.Market().Docked() != ""
	for _, st := range stacks {
		vol, name := 0.0, inv.cv.TradeableName(st.Item)
		if def, ok := inv.cv.Defs().Item(st.Item); ok {
			vol = def.Volume * float64(st.Qty)
		}
		rows = append(rows, widgets.TableRow{
			Cells: []string{name, "×" + theme.FormatInt(int64(st.Qty)), theme.FormatVolumeShort(vol)},
			Data:  st.Item,
			// The whole interaction surface: Show Info, plus the sell verb.
			Spec: widgets.RowSpec{
				Ref: widgets.RowRef(inv.cv, st.Item),
				Actions: []widgets.RowAction{
					{Label: "Sell order", Disabled: !market,
						Intent: widgets.OpenOrder{Item: st.Item, Side: sim.OrderSell, Qty: st.Qty}},
				},
			},
		})
	}
	return rows
}

// stepDrag runs the transfer gesture: a press on a stack arms a drag and
// applies the click's selection (the frame's modifier sample: range,
// toggle or replace), a held press moves the ghost and lights the hovered
// hold, and the release over another hold moves the selection there (the
// quantity picker on a single stack, every selected stack whole on a set;
// transfer rules stay transfer()'s and the sim's).
func (inv *Inventory) stepDrag() {
	pt := inv.ptr.At()
	if !inv.dragging {
		if !inv.ptr.Press(widgets.Left) {
			return
		}
		hold, ok := inv.sectionAt(pt)
		if !ok {
			return
		}
		item, idx, ok := inv.stackAt(hold, pt)
		if !ok {
			return
		}
		inv.dragging, inv.dragHold = true, hold
		inv.applySelect(hold, item, idx, inv.ptr.Mods(), true)
		inv.ghost.Set(item, inv.th.Pal().Text, len(inv.selSet))
		inv.moveGhost(pt)
		theme.ShowWidget(inv.ghost.container.GetWidget(), true)
		inv.highlight("")
		return
	}
	if inv.ptr.Down(widgets.Left) {
		inv.moveGhost(pt)
		hold, ok := inv.sectionAt(pt)
		if !ok || hold == inv.dragHold || !inv.transferOK(hold) {
			inv.highlight("")
		} else {
			inv.highlight(hold)
		}
		return
	}
	// Released: drop only over another, valid hold.
	hold, ok := inv.sectionAt(pt)
	if ok && hold != inv.dragHold && inv.transferOK(hold) {
		inv.transfer(hold)
	}
	inv.endDrag()
}

// moveGhost records the pointer the overlay lays the chip out from and
// dirties the pane: invOverlay.Layout runs only on a dirty layout, and the
// highlight flips that used to dirty it are incidental, so setting the
// field alone left the chip frozen between drop targets.
func (inv *Inventory) moveGhost(pt image.Point) {
	if inv.ghostAt == pt {
		return
	}
	inv.ghostAt = pt
	inv.pane.RequestRelayout()
}

// endDrag clears the gesture and its chrome.
func (inv *Inventory) endDrag() {
	inv.dragging = false
	inv.dragHold = ""
	inv.highlight("")
	theme.ShowWidget(inv.ghost.container.GetWidget(), false)
}

// highlight lights one hold's pane as the drop target ("" clears it). The
// pane's tinted border is the only chrome that changes, so the section's
// content stays readable.
func (inv *Inventory) highlight(hold string) {
	if hold == inv.target {
		return
	}
	inv.target = hold
	for _, c := range []struct {
		hold  string
		frame *widget.Container
	}{
		{"cargo", inv.cargoFrame}, {"ore", inv.oreFrame}, {"hangar", inv.hangFrame},
	} {
		if c.frame == nil {
			continue
		}
		theme.ShowWidget(c.frame.GetWidget(), c.hold == hold)
	}
}

// sectionAt resolves the hold whose section contains pt (an empty hold
// answers too: it is a valid drop target even with no rows).
func (inv *Inventory) sectionAt(pt image.Point) (string, bool) {
	for _, c := range []struct {
		hold string
		body *widget.Container
	}{
		{"cargo", inv.cargoBody}, {"ore", inv.oreBody}, {"hangar", inv.hangBody},
	} {
		if c.hold == "ore" && !inv.oreShown {
			continue
		}
		if pt.In(c.body.GetWidget().Rect) && inv.owns(pt) {
			return c.hold, true
		}
	}
	return "", false
}

// owns reports whether pt sits inside the inventory window, so a hidden
// window never arms a drag.
//
// shortcut: the press gate is the window rect, not OwnsAt: a table row
// lives in the scroll viewport's elevated layer, which OwnsAt on the
// section body does not answer. Do not re-check the layer while the
// pointer is over another window (the popup and modal windows own the
// frame, and no drag can arm through them).
func (inv *Inventory) owns(pt image.Point) bool {
	w := inv.win.GetContainer().GetWidget()
	return w.IsVisible() && pt.In(w.Rect)
}

// stackAt is the item under pt in one hold and its display-order index:
// a table row in list view, a tile in thumbnail view (the index is what a
// Shift range counts from).
func (inv *Inventory) stackAt(hold string, pt image.Point) (content.ItemID, int, bool) {
	if inv.tiles {
		return inv.tilesFor(hold).at(pt)
	}
	i, ok := inv.tableFor(hold).RowIndexAt(pt)
	if !ok {
		return "", 0, false
	}
	id, isItem := inv.tableFor(hold).Rows[i].Data.(content.ItemID)
	if !isItem {
		return "", 0, false
	}
	return id, i, true
}

// transferOK reports whether a hold can receive the dragged stack: a
// fitted ore hold, and a docked player for the hangar side (the sim
// validates capacity and the rest).
func (inv *Inventory) transferOK(to string) bool {
	if to == inv.dragHold {
		return false
	}
	if to == "ore" && !inv.oreShown {
		return false
	}
	if (inv.dragHold == "hangar" || to == "hangar") && !inv.cv.PlayerDocked() {
		return false
	}
	return true
}

// transfer moves the selection to hold ("cargo", "ore" or "hangar"): ONE
// selected stack opens the quantity picker (split it or take it whole),
// a MULTI-selection moves every selected stack whole with no prompts (one
// Transfer intent per stack in display order, the sim arbitrating capacity
// stack by stack, exactly as it arbitrates a single move). Confirm emits
// the transfer; the drag's release calls it for the dropped-on hold.
func (inv *Inventory) transfer(to string) {
	if !inv.hasSel || inv.src == to {
		return
	}
	// The hangar side needs a docked player (sim validates the rest).
	if (inv.src == "hangar" || to == "hangar") && !inv.cv.PlayerDocked() {
		return
	}
	if to == "ore" {
		cargo, ok := inv.cv.Cargo(inv.cv.Player())
		if !ok || cargo.OreCapacity <= 0 {
			return
		}
	}
	stacks := inv.selectedStacks()
	if len(stacks) == 0 {
		return
	}
	if len(stacks) > 1 {
		for _, st := range stacks {
			inv.as.Do(widgets.Transfer{Item: st.Item, Qty: st.Qty, From: inv.src, To: to})
		}
		return
	}
	inv.as.Do(widgets.QtyPick{
		Mode: widgets.QtyTransfer,
		Item: stacks[0].Item,
		From: inv.src,
		To:   to,
	})
}

// sellClicked sells the selection at the docked station's market: one
// selected stack opens the quantity picker (partial sales stay on its
// slider), a MULTI-selection sells every selected stack whole with no
// prompts (one Sell intent per stack, the stack's own quantity).
func (inv *Inventory) sellClicked() {
	if !inv.hasSel || inv.cv.Market().Docked() == "" {
		return
	}
	stacks := inv.selectedStacks()
	if len(stacks) == 0 {
		return
	}
	if len(stacks) > 1 {
		for _, st := range stacks {
			inv.as.Do(widgets.Sell{Item: st.Item, Qty: st.Qty})
		}
		return
	}
	inv.as.Do(widgets.QtyPick{Mode: widgets.QtySell, Item: stacks[0].Item})
}

// ---------------------------------------------------------------------------
// Thumbnail view: a wrapped grid of square item tiles
// ---------------------------------------------------------------------------

// tileGrid is one section's thumbnail view: a flow-wrapped grid of item
// tiles (large icon, name, count) inside the house scroll rail.
type tileGrid struct {
	th      theme.Theme
	defs    *content.Content
	measure func(string) float64
	grid    *widget.Container
	body    *flowLayout
	scroll  *widget.ScrollContainer
	rail    *widgets.ScrollRail
	tiles   []*widget.Container
	items   []content.ItemID
	sig     string
}

// newTileGrid builds the empty grid over the theme seam.
func newTileGrid(env widgets.Seams) *tileGrid {
	g := &tileGrid{th: env, defs: env.Defs()}
	face := env.Face(theme.StepTiny)
	g.measure = func(s string) float64 {
		w, _ := text.Measure(s, *face, 0)
		return w
	}
	g.body = &flowLayout{gap: theme.Px(tileGap)}
	g.grid = widget.NewContainer(widget.ContainerOpts.Layout(g.body))
	p := env.Pal()
	g.scroll = widget.NewScrollContainer(
		widget.ScrollContainerOpts.Content(g.grid),
		widget.ScrollContainerOpts.StretchContentWidth(),
		widget.ScrollContainerOpts.Image(&widget.ScrollContainerImage{
			Idle:     theme.Bordered(theme.WindowBody(p.BG), theme.NRGBA(p.Border, 160), 1),
			Disabled: theme.Bordered(theme.NRGBA(p.Cell, 140), theme.NRGBA(p.Border, 110), 1),
			Mask:     theme.NineSlice(theme.WindowBody(p.BG)),
		}),
	)
	g.rail = widgets.NewScrollRail(env, g.scroll, g.scroll, g.grid)
	return g
}

// Container is the section body's tile view (the rail around the grid).
func (g *tileGrid) Container() widget.Containerer { return g.rail.Container() }

// Sync mirrors the rail (and re-runs the flow's wrap).
func (g *tileGrid) Sync() { g.rail.Sync() }

// At is the tile under pt, for the drag's press.
func (g *tileGrid) At(pt image.Point) (content.ItemID, bool) {
	id, _, ok := g.at(pt)
	return id, ok
}

// at is At with the tile's index: the display-order position a Shift
// range counts from.
func (g *tileGrid) at(pt image.Point) (content.ItemID, int, bool) {
	for i, c := range g.tiles {
		if pt.In(c.GetWidget().Rect) {
			return g.items[i], i, true
		}
	}
	return "", 0, false
}

// Set rebuilds the tiles when the stacks or the wrapped selection mark
// changed (an identical Set is a no-op). sel is the owning section's
// Selection set (nil outside it): every member's tile wears the accent
// frame, so multi-selection reads in the thumbnail view the way it does in
// the list.
func (g *tileGrid) Set(stacks []content.CargoStack, sel map[content.ItemID]bool) {
	var sig []byte
	sig = strconv.AppendInt(sig, int64(g.wrap()), 10)
	sig = append(sig, '|')
	for _, st := range stacks {
		sig = append(sig, string(st.Item)...)
		sig = strconv.AppendInt(sig, int64(st.Qty), 10)
		if sel[st.Item] {
			sig = append(sig, '+')
		} else {
			sig = append(sig, '-')
		}
		sig = append(sig, ',')
	}
	if string(sig) == g.sig {
		return
	}
	g.sig = string(sig)
	g.tiles, g.items = g.tiles[:0], g.items[:0]
	g.grid.RemoveChildren()
	p := g.th.Pal()
	for _, st := range stacks {
		mark := widgets.ResolveTradeable(g.defs, st.Item)
		tile := widget.NewContainer(
			widget.ContainerOpts.BackgroundImage(theme.Bordered(theme.Solid(p.Cell), theme.NRGBA(p.Border, 160), theme.Px(1))),
			widget.ContainerOpts.WidgetOpts(widget.WidgetOpts.MinSize(theme.Px(tileW), theme.Px(tileH))),
			widget.ContainerOpts.Layout(widget.NewRowLayout(
				widget.RowLayoutOpts.Direction(widget.DirectionVertical),
				widget.RowLayoutOpts.Spacing(theme.Px(2)),
				widget.RowLayoutOpts.Padding(widget.NewInsetsSimple(theme.Px(3))),
			)),
		)
		icon := theme.NewIcon(mark.Icon, theme.IconTile, p.Text)
		icon.GetWidget().LayoutData = widget.RowLayoutData{Position: widget.RowLayoutPositionCenter}
		name := widget.NewText(
			widget.TextOpts.Text(widgets.Ellipsize(mark.Name, float64(theme.Px(tileW-8)), g.measure),
				g.th.Face(theme.StepTiny), theme.Solid(p.Text)),
			widget.TextOpts.Position(widget.TextPositionCenter, widget.TextPositionCenter),
			widget.TextOpts.WidgetOpts(widget.WidgetOpts.LayoutData(widget.RowLayoutData{Stretch: true})),
		)
		count := widget.NewText(
			widget.TextOpts.Text("×"+theme.FormatInt(int64(st.Qty)), g.th.FaceMono(theme.StepTiny), theme.Solid(p.Accent)),
			widget.TextOpts.Position(widget.TextPositionCenter, widget.TextPositionCenter),
			widget.TextOpts.WidgetOpts(widget.WidgetOpts.LayoutData(widget.RowLayoutData{Stretch: true})),
		)
		tile.AddChild(icon, name, count)
		if sel[st.Item] {
			tile.SetBackgroundImage(theme.Bordered(theme.NRGBA(p.Accent, 36), theme.Solid(p.Accent), theme.Px(1)))
		}
		g.grid.AddChild(tile)
		g.tiles = append(g.tiles, tile)
		g.items = append(g.items, st.Item)
	}
	g.grid.RequestRelayout()
}

// wrap is the column count the flow will use at the current view width.
func (g *tileGrid) wrap() int {
	return g.body.cols(g.scroll.ViewRect().Dx())
}

// flowLayout tiles uniform children left to right, wrapping at the
// container width — the thumbnail view's file-explorer grid. Children are
// expected to share one size (the tile builder's fixed floor).
type flowLayout struct {
	gap   int
	width int // the last laid-out width (PreferredSize needs it before the first Layout)
}

// cols is the wrap count at a width.
func (f *flowLayout) cols(width int) int {
	if f.gap < 0 {
		return 1
	}
	cw := theme.Px(tileW)
	if cw <= 0 || width <= 0 {
		return 1
	}
	return max(1, (width+f.gap)/(cw+f.gap))
}

// PreferredSize is the wrapped grid's extent at the last known width.
func (f *flowLayout) PreferredSize(ws []widget.PreferredSizeLocateableWidget) (int, int) {
	if len(ws) == 0 {
		return 0, 0
	}
	cw, ch := ws[0].PreferredSize()
	n := f.cols(f.width)
	rows := (len(ws) + n - 1) / n
	w := n*cw + (n-1)*f.gap
	return w, rows*ch + (rows-1)*f.gap
}

// Layout places every tile in row-major order, wrapping at the rect's
// width.
func (f *flowLayout) Layout(ws []widget.PreferredSizeLocateableWidget, rect image.Rectangle) {
	f.width = rect.Dx()
	n := f.cols(rect.Dx())
	for i, w := range ws {
		cw, ch := w.PreferredSize()
		x := rect.Min.X + (i%n)*(cw+f.gap)
		y := rect.Min.Y + (i/n)*(ch+f.gap)
		w.SetLocation(image.Rect(x, y, x+cw, y+ch))
	}
}

// ---------------------------------------------------------------------------
// The undocked hangar hatch
// ---------------------------------------------------------------------------

// hatchView is the undocked hangar's placeholder: a procedural diagonal
// crosshatch in palette colours with "Not docked" (on a dim plate) over
// it. Repainted only when the body's size changes.
type hatchView struct {
	th   theme.Theme
	root *widget.Container
	img  *ebiten.Image
	gfx  *widget.Graphic
	text *widget.Text
}

// hatchLabel is the placeholder's line.
const hatchLabel = "Not docked"

// newHatchView builds the hatch over the theme seam.
func newHatchView(env widgets.Seams) *hatchView {
	p := env.Pal()
	h := &hatchView{th: env}
	h.img = ebiten.NewImage(1, 1)
	h.gfx = widget.NewGraphic(widget.GraphicOpts.Image(h.img))
	h.text = widget.NewText(
		widget.TextOpts.Text(hatchLabel, env.Face(theme.StepSmall), theme.Solid(p.Text)),
		widget.TextOpts.Position(widget.TextPositionCenter, widget.TextPositionCenter),
	)
	h.root = widget.NewContainer(widget.ContainerOpts.Layout(widget.NewStackedLayout()))
	h.root.AddChild(h.gfx, h.text)
	return h
}

// Container is the hangar body's hatch view.
func (h *hatchView) Container() widget.Containerer { return h.root }

// Paint redraws the hatch at w×h when the size changed.
func (h *hatchView) Paint(w, ht int) {
	if w <= 0 || ht <= 0 {
		return
	}
	if b := h.img.Bounds(); b.Dx() == w && b.Dy() == ht {
		return
	}
	h.img = ebiten.NewImage(w, ht)
	p := h.th.Pal()
	theme.FillRect(h.img, h.img.Bounds(), theme.WindowBody(p.BG))

	// The crosshatch: two 45° families of hairlines in the border role.
	col := theme.NRGBA(p.Border, 150)
	step := max(theme.Px(10), 2)
	for x := -ht; x < w; x += step {
		vector.StrokeLine(h.img, float32(x), 0, float32(x+ht), float32(ht), 1, col, false)
		vector.StrokeLine(h.img, float32(x), 0, float32(x-ht), float32(ht), 1, col, false)
	}

	// The dim plate: sized to the label and centred, so the text keeps its
	// contrast over the hatch.
	tw, lh := text.Measure(hatchLabel, *h.th.Face(theme.StepSmall), 0)
	pw, ph := int(math.Ceil(tw))+theme.Px(16), int(math.Ceil(lh))+theme.Px(8)
	plate := image.Rect((w-pw)/2, (ht-ph)/2, (w-pw)/2+pw, (ht-ph)/2+ph)
	theme.FillRect(h.img, plate, theme.NRGBA(p.BGHard, 230))
	vector.StrokeRect(h.img, float32(plate.Min.X), float32(plate.Min.Y),
		float32(plate.Dx()), float32(plate.Dy()), 1, theme.NRGBA(p.Border, 220), false)

	h.gfx.Image = h.img
	h.root.RequestRelayout()
}

// ---------------------------------------------------------------------------
// The drag ghost and the window overlay
// ---------------------------------------------------------------------------

// dragGhost is the chip that follows the pointer while a stack is dragged:
// the item's icon and name in a bordered plate (palette chrome only).
type dragGhost struct {
	defs      *content.Content
	container *widget.Container
	icon      *widget.Graphic
	name      *widget.Text
}

// newDragGhost builds the hidden chip.
func newDragGhost(env widgets.Seams) *dragGhost {
	p := env.Pal()
	g := &dragGhost{defs: env.Defs()}
	g.icon = widget.NewGraphic(widget.GraphicOpts.Image(icons.Get(icons.Fallback, theme.Px(theme.IconButton), p.Text)))
	g.icon.GetWidget().LayoutData = widget.RowLayoutData{Position: widget.RowLayoutPositionCenter}
	g.name = widget.NewText(widget.TextOpts.Text("", env.Face(theme.StepTiny), theme.Solid(p.Text)))
	g.container = widget.NewContainer(
		widget.ContainerOpts.BackgroundImage(theme.Bordered(theme.NRGBA(p.BGHard, 235), theme.Solid(p.Accent), theme.Px(1))),
		widget.ContainerOpts.Layout(widget.NewRowLayout(
			widget.RowLayoutOpts.Direction(widget.DirectionHorizontal),
			widget.RowLayoutOpts.Spacing(theme.Px(6)),
			widget.RowLayoutOpts.Padding(widget.NewInsetsSimple(theme.Px(4))),
		)),
	)
	g.container.AddChild(g.icon, g.name)
	return g
}

// Set points the chip at one item over the ghost's own theme seam, with
// the number of stacks the drag carries (a multi-selection names its
// count beside the item, so the chip never pretends the set is one stack).
func (g *dragGhost) Set(item content.ItemID, tint color.RGBA, stacks int) {
	mark := widgets.ResolveTradeable(g.defs, item)
	g.name.Label = mark.Name
	if stacks > 1 {
		g.name.Label = mark.Name + " · " + strconv.Itoa(stacks) + " stacks"
	}
	if mark.Icon != "" {
		g.icon.Image = icons.Get(mark.Icon, theme.Px(theme.IconButton), tint)
	}
}

// invOverlay lays the window body over the whole content rect and the drag
// ghost at the pointer offset on top, clamped inside the body.
type invOverlay struct{ inv *Inventory }

// PreferredSize is the body's.
func (o invOverlay) PreferredSize(ws []widget.PreferredSizeLocateableWidget) (int, int) {
	if len(ws) == 0 {
		return 0, 0
	}
	return ws[0].PreferredSize()
}

// Layout stretches the body and places the ghost near the pointer.
func (o invOverlay) Layout(ws []widget.PreferredSizeLocateableWidget, rect image.Rectangle) {
	if len(ws) == 0 {
		return
	}
	ws[0].SetLocation(rect)
	if len(ws) < 2 {
		return
	}
	gw, gh := ws[1].PreferredSize()
	x := clampInt(o.inv.ghostAt.X-rect.Min.X+theme.Px(12), 0, max(0, rect.Dx()-gw))
	y := clampInt(o.inv.ghostAt.Y-rect.Min.Y+theme.Px(12), 0, max(0, rect.Dy()-gh))
	ws[1].SetLocation(image.Rect(rect.Min.X+x, rect.Min.Y+y, rect.Min.X+x+gw, rect.Min.Y+y+gh))
}

func clampInt(v, lo, hi int) int {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}
