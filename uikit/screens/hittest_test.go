package screens

import (
	"image"
	"testing"

	enuiinput "github.com/ebitenui/ebitenui/input"
	"github.com/ebitenui/ebitenui/widget"
	"github.com/entieve/entieve/uikit/internal/uitest"
	"github.com/entieve/entieve/uikit/theme"
	"github.com/entieve/entieve/uikit/widgets"
)

// TestOwnPointTable is the one hit test's own table: one fixture carrying
// every layer at once — the rail (top chrome), a HUD-shaped root container
// with an anchored pair, a window with a button and a registered row
// surface, the Overview's real table rows, an open menu and the modal
// quantity picker — and one asserted {Layer, Control, Window} per point.
// The points are derived from the live rects, so the table holds at any
// scale and no synthetic screen position is baked in.
func TestOwnPointTable(t *testing.T) {
	uitest.UseScale(t, 1)
	in := &scriptPointer{}
	k, _ := newTestKitWith(t, widgets.NewPointer(in))

	// Root chrome, the HUD idiom: a full-screen container (it stretches over
	// the whole root) whose anchored pair is the only thing it paints.
	hud := widget.NewContainer(widget.ContainerOpts.Layout(widget.NewAnchorLayout()))
	pair := widget.NewContainer(widget.ContainerOpts.WidgetOpts(
		widget.WidgetOpts.LayoutData(widget.AnchorLayoutData{
			HorizontalPosition: widget.AnchorLayoutPositionCenter,
			VerticalPosition:   widget.AnchorLayoutPositionEnd,
		}),
		widget.WidgetOpts.MinSize(theme.Px(400), theme.Px(120)),
	))
	hud.AddChild(pair)
	k.AddContainer(hud, widget.AnchorLayoutData{StretchHorizontal: true, StretchVertical: true})

	// Top chrome: the rail.
	open := false
	rail := NewRail(k.Env(), railEntries3(&open))
	k.AddTopContainer(rail, widget.AnchorLayoutData{
		HorizontalPosition: widget.AnchorLayoutPositionStart,
		VerticalPosition:   widget.AnchorLayoutPositionStart,
		Padding:            &widget.Insets{Top: theme.Px(RailTopInset)},
	})

	// A window with a button and a registered row surface (the host's
	// SetWindowRows contract: a live closure the walk consults after the
	// widget walk fails).
	btn := widget.NewButton(widget.ButtonOpts.TextLabel("Go"))
	body := widget.NewContainer(widget.ContainerOpts.Layout(widget.NewAnchorLayout()))
	body.AddChild(btn)
	win := widget.NewWindow(
		widget.WindowOpts.Contents(body),
		widget.WindowOpts.MinSize(theme.Px(320), theme.Px(300)),
		widget.WindowOpts.Location(image.Rect(600, 60, 920, 360)),
	)
	k.AddWindow(win)
	var rowBand image.Rectangle
	k.SetWindowRows(win, func(pt image.Point) bool { return pt.In(rowBand) })

	pumpKit(t, k, 1280, 720)

	wr := win.GetContainer().GetWidget().Rect
	rowBand = image.Rect(wr.Min.X+8, wr.Min.Y+wr.Dy()/2, wr.Max.X-8, wr.Min.Y+wr.Dy()/2+theme.Px(24))
	btnPt := mid(btn.GetWidget().Rect)
	bodyPt := image.Pt(wr.Min.X+4, wr.Max.Y-4) // below the button and the band
	rowPt := mid(rowBand)
	pairPt := mid(pair.GetWidget().Rect)
	railPt := mid(rail.Button("Overview").GetWidget().Rect)
	worldPt := image.Pt(theme.Px(1000), theme.Px(400))

	// Fixture sanity: the rules this table is about are actually exercised.
	if hud.GetWidget().Rect != k.root.GetWidget().Rect {
		t.Fatalf("fixture: HUD container %v is not the full root %v", hud.GetWidget().Rect, k.root.GetWidget().Rect)
	}
	if !pairPt.In(hud.GetWidget().Rect) || worldPt.In(pair.GetWidget().Rect) {
		t.Fatalf("fixture: pair %v / world point %v do not separate the container from its child", pair.GetWidget().Rect, worldPt)
	}
	if bodyPt.In(rowBand) || bodyPt.In(btn.GetWidget().Rect) {
		t.Fatalf("fixture: window body point %v collides with a control or the row band", bodyPt)
	}
	if !railPt.In(rail.Container().GetWidget().Rect) || railPt.In(wr) {
		t.Fatalf("fixture: rail point %v is not the column's own", railPt)
	}
	ovRow := k.OverviewTable().RowWidgetForTest(0).Rect
	if ovRow.Empty() {
		t.Fatal("fixture: the overview table has no laid-out row")
	}
	ovPt := mid(ovRow)
	if ovPt.In(wr) {
		t.Fatalf("fixture: overview row %v sits under the fixture window %v", ovRow, wr)
	}

	type point struct {
		name    string
		pt      image.Point
		layer   Layer
		control bool
		win     *widget.Window
	}
	check := func(rows []point) {
		t.Helper()
		for _, r := range rows {
			got := k.Own(r.pt)
			if got.Layer != r.layer || got.Control != r.control || got.Window != r.win {
				t.Errorf("%s at %v = {layer %d, window %v, control %v}, want {layer %d, window %v, control %v}",
					r.name, r.pt, got.Layer, got.Window, got.Control, r.layer, r.win, r.control)
			}
		}
	}

	check([]point{
		// The world, and the HUD container's own (full-screen) rect: a root
		// chrome layer claims its DESCENDANTS only.
		{"world point", worldPt, LayerWorld, false, nil},
		{"hud container rect", image.Pt(theme.Px(1000), theme.Px(550)), LayerWorld, false, nil},
		{"hud pair (root chrome)", pairPt, LayerRootChrome, false, nil},
		// The rail's entries live inside its scroll viewport, which the
		// widget walk cannot descend into: a rail press is silent (the same
		// answer the pre-hit-test PressTarget gave).
		{"rail (top chrome)", railPt, LayerTopChrome, false, nil},
		{"window body", bodyPt, LayerWindow, false, win},
		{"window button", btnPt, LayerWindow, true, win},
		{"window row surface", rowPt, LayerWindow, true, win},
		{"overview row", ovPt, LayerWindow, true, k.OverviewWindow()},
	})

	// The popup: polled with the pointer on an entry, so an entry press
	// speaks while the same frame's dismiss point (off the popup) stays
	// silent — Popup.Control is true for entries only.
	k.Menu().Open([]widgets.MenuEntry{
		{Label: "Show Info", Action: func() {}},
		widgets.MenuSep,
		{Label: "Warp", Action: func() {}},
	}, image.Pt(theme.Px(600), theme.Px(200)))
	entry := k.Menu().RowRect(0)
	entryPt := mid(entry)
	dismissPt := image.Pt(entry.Min.X+2, k.Menu().Rect().Max.Y+4)
	in.x, in.y = entryPt.X, entryPt.Y
	k.Pointer().Begin()
	if !k.MenuInput() || !k.Held() {
		t.Fatal("fixture: the open menu did not take the frame")
	}
	check([]point{
		{"menu entry", entryPt, LayerPopup, true, nil},
		{"dismiss point off the popup", dismissPt, LayerPopup, false, nil},
		{"menu separator", mid(k.Menu().RowRect(1)), LayerPopup, false, nil},
	})

	// The modal quantity picker: its own (fullscreen) layer owns the frame;
	// its controls are the loud part.
	k.Menu().Close()
	k.OpenQty(widgets.QtyPick{Mode: widgets.QtyRefine, Item: "ore-cordite"})
	pumpKit(t, k, 1280, 720)
	qtyBtn := k.CloseButtonOf(k.QtyWindow())
	if qtyBtn == nil {
		t.Fatal("fixture: the picker's × was never attached")
	}
	check([]point{
		{"modal picker control", mid(qtyBtn.GetWidget().Rect), LayerPopup, true, nil},
		{"modal picker off its rect", worldPt, LayerPopup, false, nil},
	})

	// Last and never a raise: ebitenui's hover flag, at a point no rect owns
	// — with the picker closed, so the popup latch is not what answers.
	k.CloseQty()
	k.MenuInput()

	// A visible modal window owns every point, even outside its own rect:
	// its input layer is fullscreen, so the overview under it is nobody's
	// (and nothing below it could rise through it either).
	log := NewLog(k.Env())
	log.Win().Modal = true
	removeLog := k.AddWindow(log.Win())
	log.Win().SetLocation(image.Rect(theme.Px(600), theme.Px(60), theme.Px(900), theme.Px(300)))
	pumpKit(t, k, 1280, 720)
	check([]point{{"overview row under a modal", ovPt, LayerWindow, false, log.Win()}})
	removeLog()

	hoverPt := image.Pt(theme.Px(1000), theme.Px(650))
	enuiinput.UIHovered = true
	defer func() { enuiinput.UIHovered = false }()
	check([]point{{"hover backstop", hoverPt, LayerHovered, false, nil}})
}

// mid is a rect's centre: the point a press lands on.
func mid(r image.Rectangle) image.Point { return image.Pt(r.Min.X+r.Dx()/2, r.Min.Y+r.Dy()/2) }
