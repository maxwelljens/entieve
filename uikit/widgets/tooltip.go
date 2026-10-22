package widgets

import (
	"image"
	"image/color"
	"math"
	"strings"
	"time"

	"github.com/ebitenui/ebitenui/widget"
	"github.com/entieve/entieve/content"
	"github.com/entieve/entieve/sim"
	"github.com/entieve/entieve/uikit/icons"
	"github.com/entieve/entieve/uikit/theme"
	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/text/v2"
	"github.com/hajimehoshi/ebiten/v2/vector"
)

// Structured hover tooltips.
//
// A tooltip is a TooltipModel (data: icon, title, sub line, sections of
// label/value rows, hint footers) plus one pure layout function that turns
// (model, inner width) into every rect the panel draws:
//
//	┌────────────────────────────────┐
//	│ ▓  EMP Turret II               20 px icon + Bold title (FaceBold)
//	│    High slot 1 · Weapon        dim sub line (slot · class)
//	│ ─────────────────────────────  2 px ColAccent hairline
//	│ DAMAGE                         section label, ColHead, uppercase
//	│   Electro-magnetic       16.0  ColDim label · value on the shared x
//	│   Tracking              145.0  values ColText (or their tint)
//	│ ─────────────────────────────
//	│ Right-click: module menu      hint footer, ColDim, tiny step
//	└────────────────────────────────┘
//
// The panel draws itself straight onto the screen from those rects (no
// widget tree below the content container), so the geometry the tests pin
// and the pixels that ship are the same numbers. Bold is the title only:
// rows and values stay Regular so digits keep scanning.

// Tooltip geometry constants. The plain-text tip body this replaces was
// padded 8/5 with a 1 px Border edge over an opaque fill — the structured
// panel keeps that surface exactly and grows a header, hairlines and a
// value column.
const (
	TipPadX    = 8                 // side inset: what the shipped tip body had (text needs the room off the border)
	tipPadY    = theme.ContentsPad // top/bottom inset: the house panel margin, as before
	TipIcon    = theme.IconButton  // one icon size in every tooltip: 20 px, manifest-nudged
	tipIconGap = 6                 // icon column → title/sub column
	tipMaxW    = 320               // panel width cap; content sizes itself below it
	TipRuleH   = 2                 // divider hairline height
	tipRuleGap = 4                 // space above/below a divider
	tipSecGap  = 3                 // space between sections
	TipRowGap  = 4                 // label → value gutter inside a row
	tipHintGap = 2                 // space between hint lines
	TipRuleA   = 80                // divider alpha over the fill (low, per the anatomy)
)

// tipMaxInner is the widest content box (panel cap minus the padding).
const tipMaxInner = tipMaxW - 2*TipPadX

// TooltipRow is one label/value line. The label is left-anchored in ColDim,
// the value right-anchored on the panel's single ValueX in ColText — one
// fixed x for every row, so a number changing width never shifts a row.
// Tint (zero = the default ColText) colors a value: ColGood/ColDanger for
// signed modifiers.
type TooltipRow struct {
	Label string
	Value string
	Tint  color.RGBA
}

// TooltipSection is a labelled group of rows ("DAMAGE", "CHARGE"). The
// label is stored uppercase and drawn in ColHead at the small step.
type TooltipSection struct {
	Label string
	Rows  []TooltipRow
}

// TooltipModel is a tooltip's content and nothing else — no geometry.
// Absent data never reaches the model: a module without a charge has no
// CHARGE section instead of an empty one, and the layout draws exactly
// what it is given.
type TooltipModel struct {
	Icon     string     // semantic icon name ("" = text-only header)
	IconTint color.RGBA // icon tint; zero = ColText
	Title    string     // bold header line — the only bold text in a tooltip
	Sub      string     // dim class line under the title column
	Sections []TooltipSection
	Hints    []string // footer lines (ColDim, tiny step)
}

// ---------------------------------------------------------------------------
// Layout — pure: (model, inner width) → rects, in panel coordinates
// ---------------------------------------------------------------------------

// tipLayout is one tooltip's computed geometry. Rects are panel-relative
// (0,0 = panel top-left, padding included); Inner is the content rect the
// padding carves out; ValueX is the single right anchor every value shares.
type tipLayout struct {
	Size     image.Point        // full panel size, padding included
	Inner    image.Rectangle    // content rect (Size inset by the padding)
	Icon     image.Rectangle    // 20 px glyph box (empty when Icon == "")
	Title    image.Rectangle    // bold title line
	Sub      image.Rectangle    // dim class line (empty when Sub == "")
	Rules    []image.Rectangle  // ColAccent hairlines, 2 px
	Sections []tipSectionLayout // section label + its row rects
	Hints    []image.Rectangle  // footer lines
	ValueX   int                // right edge shared by every value rect
}

// tipSectionLayout is one section's rects: the label line over its rows.
type tipSectionLayout struct {
	Label image.Rectangle
	Rows  []tipRowLayout
}

// tipRowLayout is one row: label and value rects share a Y range and can
// never touch horizontally (the value keeps a tipRowGap gutter).
type tipRowLayout struct {
	Label image.Rectangle
	Value image.Rectangle
}

// tipMeasure is the advance width of s in one of the panel's faces.
func TipMeasure(s string, face *text.Face) float64 {
	w, _ := text.Measure(s, *face, 0)
	return w
}

// tipLineH is a face's line box: the height every text rect in the panel
// takes (ascent + descent rounded up, the way Table derives rowH).
func TipLineH(face *text.Face) int {
	m := (*face).Metrics()
	return int(math.Ceil(m.HAscent + m.HDescent))
}

// tipInnerWidth is the content width a model wants, in pixels: the widest
// natural line, capped at the panel's 320 px. Rows measure as label + gap +
// value, so a model that fits here needs no ellipsis at all.
func tipInnerWidth(t theme.Theme, m TooltipModel) int {
	// The title and sub lines sit in the column right of the icon.
	indent := 0.0
	if m.Icon != "" {
		indent = float64(theme.Px(TipIcon) + theme.Px(tipIconGap))
	}
	w := indent + TipMeasure(m.Title, t.FaceBold(theme.StepBase))
	if sub := indent + TipMeasure(m.Sub, t.Face(theme.StepSmall)); sub > w {
		w = sub
	}
	for _, sec := range m.Sections {
		if lw := TipMeasure(sec.Label, t.Face(theme.StepSmall)); lw > w {
			w = lw
		}
		for _, row := range sec.Rows {
			rw := TipMeasure(row.Label, t.Face(theme.StepSmall)) + float64(theme.Px(TipRowGap)) +
				TipMeasure(row.Value, t.Face(theme.StepSmall))
			if rw > w {
				w = rw
			}
		}
	}
	for _, h := range m.Hints {
		if hw := TipMeasure(h, t.Face(theme.StepTiny)); hw > w {
			w = hw
		}
	}
	iw := int(math.Round(w))
	// The 320 px design cap, scaled: the panel never outgrows it.
	if iw > theme.Px(tipMaxInner) {
		iw = theme.Px(tipMaxInner)
	}
	return iw
}

// tipLayoutAt lays a model out inside an inner width of iw pixels. Every
// text line is emitted whether or not it fits — labels ellipsize at draw
// time, values never do — so the height depends on the model alone and the
// width only on the caller's box.
func tipLayoutAt(t theme.Theme, m TooltipModel, iw int) tipLayout {
	if iw < 0 {
		iw = 0
	}
	// Design constants × the current scale, bound once: every rect below
	// is positioned in live px, and the draw side uses the same values.
	padX, padY := theme.Px(TipPadX), theme.Px(tipPadY)
	icon, iconGap := theme.Px(TipIcon), theme.Px(tipIconGap)
	ruleH, ruleGap := theme.Px(TipRuleH), theme.Px(tipRuleGap)
	secGap, rowGap, hintGap := theme.Px(tipSecGap), theme.Px(TipRowGap), theme.Px(tipHintGap)

	x0 := padX
	x1 := x0 + iw
	titleX := x0
	if m.Icon != "" {
		titleX = x0 + icon + iconGap
	}

	titleH := TipLineH(t.FaceBold(theme.StepBase))
	smallH := TipLineH(t.Face(theme.StepSmall))
	tinyH := TipLineH(t.Face(theme.StepTiny))

	l := tipLayout{ValueX: x1}
	y := padY

	// Header: the glyph and the bold title share one line (the glyph
	// optically centered in it), the sub line hangs under the title column.
	hdrH := max(icon, titleH)
	if m.Icon != "" {
		top := y + (hdrH-icon)/2
		l.Icon = image.Rect(x0, top, x0+icon, top+icon)
	}
	top := y + (hdrH-titleH)/2
	l.Title = image.Rect(titleX, top, x1, top+titleH)
	y += hdrH
	if m.Sub != "" {
		l.Sub = image.Rect(titleX, y, x1, y+smallH)
		y += smallH
	}

	// Body: a hairline, then the sections' labels and rows stacked without
	// gaps (rows tile exactly), sections separated by tipSecGap.
	if len(m.Sections) > 0 {
		y += ruleGap
		l.Rules = append(l.Rules, image.Rect(x0, y, x1, y+ruleH))
		y += ruleH + ruleGap
		for si, sec := range m.Sections {
			if si > 0 {
				y += secGap
			}
			sl := tipSectionLayout{Label: image.Rect(x0, y, x1, y+smallH)}
			y += smallH
			for _, row := range sec.Rows {
				vw := int(math.Ceil(TipMeasure(row.Value, t.Face(theme.StepSmall))))
				if vw > iw {
					vw = iw
				}
				value := image.Rect(x1-vw, y, x1, y+smallH)
				lw := int(math.Ceil(TipMeasure(row.Label, t.Face(theme.StepSmall))))
				// The label gets everything left of the value's gutter, so
				// the two rects are disjoint by construction.
				if room := value.Min.X - rowGap - x0; lw > room {
					lw = room
				}
				if lw < 0 {
					lw = 0
				}
				sl.Rows = append(sl.Rows, tipRowLayout{
					Label: image.Rect(x0, y, x0+lw, y+smallH),
					Value: value,
				})
				y += smallH
			}
			l.Sections = append(l.Sections, sl)
		}
	}

	// Footer: a second hairline after the sections, then the hints.
	if len(m.Hints) > 0 {
		if len(m.Sections) > 0 {
			y += ruleGap
			l.Rules = append(l.Rules, image.Rect(x0, y, x1, y+ruleH))
			y += ruleH + ruleGap
		} else {
			y += ruleGap
		}
		for i := range m.Hints {
			if i > 0 {
				y += hintGap
			}
			l.Hints = append(l.Hints, image.Rect(x0, y, x1, y+tinyH))
			y += tinyH
		}
	}

	// The content box is the padding inset; the panel adds the bottom pad
	// after it (y here is the last line's end).
	l.Inner = image.Rect(x0, padY, x1, y)
	y += padY
	l.Size = image.Point{X: x1 + padX, Y: y}
	return l
}

// tipSize is the panel size a model wants at its content width.
func TipSize(t theme.Theme, m TooltipModel) image.Point {
	return tipLayoutAt(t, m, tipInnerWidth(t, m)).Size
}

// ---------------------------------------------------------------------------
// Placement clamp
// ---------------------------------------------------------------------------

// clampAxis pins a size inside [lo, hi): clamped to the low edge when it
// starts before it, slid back when it runs past the end, and pinned to lo
// when it is too big to fit at all (it then clips at the far edge).
func clampAxis(pos, size, lo, hi int) int {
	if size >= hi-lo {
		return lo
	}
	if pos < lo {
		return lo
	}
	if pos+size > hi {
		return hi - size
	}
	return pos
}

// clampTipRect is the tooltip placement rule: whatever anchor the hover
// hands us — the cursor-sticky position is cursor + offset and can land
// past any edge — the drawn panel ends up fully inside the screen. It is a
// no-op for the ordinary middle-of-the-screen case, so a tooltip that
// already fits never moves.
func ClampTipRect(r, screen image.Rectangle) image.Rectangle {
	w, h := r.Dx(), r.Dy()
	x := clampAxis(r.Min.X, w, screen.Min.X, screen.Max.X)
	y := clampAxis(r.Min.Y, h, screen.Min.Y, screen.Max.Y)
	return image.Rect(x, y, x+w, y+h)
}

// ---------------------------------------------------------------------------
// Drawing
// ---------------------------------------------------------------------------

// drawTooltip paints one laid-out panel at origin: the tip's surface (opaque
// fill + 1 px border, exactly what the plain-text tip body used), the icon,
// the header, the hairlines, the rows and the hints. It is a thin renderer
// over tipLayoutAt — every rect it touches comes from the layout.
func drawTooltip(dst *ebiten.Image, m TooltipModel, l tipLayout, t theme.Theme, origin image.Point) {
	p := t.Pal()
	at := func(r image.Rectangle) image.Rectangle { return r.Add(origin) }
	size := l.Size

	// Surface: the current tooltip's own treatment (BGHard over a Border
	// edge at 220), drawn by hand because the panel is this widget's rect.
	theme.FillRect(dst, image.Rect(0, 0, size.X, size.Y).Add(origin), theme.Solid(p.BGHard))
	// Hairline border scales with the panel.
	bs := float32(theme.Scale())
	vector.StrokeRect(dst,
		float32(origin.X)+bs/2, float32(origin.Y)+bs/2,
		float32(size.X)-bs, float32(size.Y)-bs,
		bs, theme.NRGBA(p.Border, 220), true)

	if m.Icon != "" && !l.Icon.Empty() {
		tint := m.IconTint
		if tint == (color.RGBA{}) {
			tint = p.Text
		}
		r := at(l.Icon)
		opts := &ebiten.DrawImageOptions{}
		opts.GeoM.Translate(float64(r.Min.X), float64(r.Min.Y))
		dst.DrawImage(icons.Get(m.Icon, theme.Px(TipIcon), tint), opts)
	}
	theme.DrawCardText(dst, m.Title, t.FaceBold(theme.StepBase), at(l.Title).Min.X, at(l.Title).Min.Y, theme.Solid(p.Text))
	if m.Sub != "" {
		theme.DrawCardText(dst, m.Sub, t.Face(theme.StepSmall), at(l.Sub).Min.X, at(l.Sub).Min.Y, theme.Solid(p.Dim))
	}
	for _, r := range l.Rules {
		theme.FillRect(dst, at(r), theme.NRGBA(p.Accent, TipRuleA))
	}
	for si, sec := range m.Sections {
		sl := l.Sections[si]
		theme.DrawCardText(dst, sec.Label, t.Face(theme.StepSmall), at(sl.Label).Min.X, at(sl.Label).Min.Y, theme.Solid(p.Head))
		for ri, row := range sec.Rows {
			rr := sl.Rows[ri]
			theme.DrawCardText(dst, row.Label, t.Face(theme.StepSmall), at(rr.Label).Min.X, at(rr.Label).Min.Y, theme.Solid(p.Dim))
			col := row.Tint
			if col == (color.RGBA{}) {
				col = p.Text
			}
			// Right-anchored: the value's box ends on the shared x.
			drawValue(dst, row.Value, t.Face(theme.StepSmall), at(rr.Value).Max.X, at(rr.Value).Min.Y, theme.Solid(col))
		}
	}
	for i, h := range m.Hints {
		r := at(l.Hints[i])
		theme.DrawCardText(dst, h, t.Face(theme.StepTiny), r.Min.X, r.Min.Y, theme.Solid(p.Dim))
	}
}

// drawValue draws s right-anchored: its last glyph lands on x (and its box
// top on y), so numbers growing leftward leave the row's right edge alone.
func drawValue(dst *ebiten.Image, s string, face *text.Face, x, y int, col color.Color) {
	if s == "" || face == nil {
		return
	}
	opts := &text.DrawOptions{}
	opts.PrimaryAlign = text.AlignEnd
	opts.GeoM.Translate(float64(x), float64(y))
	opts.ColorScale.ScaleWithColor(col)
	text.Draw(dst, s, *face, opts)
}

// ---------------------------------------------------------------------------
// The widget
// ---------------------------------------------------------------------------

// tipView is one tooltip's drawn panel. It is the content container's only
// child (stretched to it), reports the model's size as its preferred size —
// which is what widget.ToolTip sizes its window and clamps from — and paints
// itself at clampTipRect(widget.Rect, screen): a hover anchor that lands off
// a screen edge cannot clip the panel, because the clamp is the last word
// on where the pixels go.
type TipView struct {
	th     theme.Theme
	model  TooltipModel
	widget *widget.Widget

	layout    tipLayout // cached for layoutFor
	layoutFor int       // inner width the cache covers (-1 = model changed)
}

// newTipView builds a view over the given model (call SetModel to change it).
func newTipView(th theme.Theme, m TooltipModel) *TipView {
	v := &TipView{th: th, model: m, layoutFor: -1}
	v.widget = widget.NewWidget(widget.WidgetOpts.LayoutData(widget.AnchorLayoutData{
		HorizontalPosition: widget.AnchorLayoutPositionStart,
		VerticalPosition:   widget.AnchorLayoutPositionStart,
		StretchHorizontal:  true,
		StretchVertical:    true,
	}))
	return v
}

// newTipContent builds the structured panel a tooltip shows: a transparent
// anchor container holding the view stretched across it. Tests and captures
// mount this directly (without the hover state machine) to draw the panel.
func NewTipContent(th theme.Theme, m TooltipModel) (*widget.Container, *TipView) {
	view := newTipView(th, m)
	body := widget.NewContainer(
		widget.ContainerOpts.Layout(widget.NewAnchorLayout()),
	)
	body.AddChild(view)
	return body, view
}

// rackTipDelay is the shared hover delay of every attached tip (the
// module rack's cells set the precedent: 300 ms).
const rackTipDelay = 300 * time.Millisecond

// newTip builds a hover tooltip over a fresh structured panel: ebitenui's
// cursor-sticky content window with the shared rack delay (300 ms) and the
// 12 px offset, so show/hide behaviour is exactly what it was. The returned
// view is what callers refresh with SetModel as state changes.
func NewTip(th theme.Theme, m TooltipModel) (*widget.ToolTip, *TipView) {
	body, view := NewTipContent(th, m)
	tip := widget.NewToolTip(
		widget.ToolTipOpts.Content(body),
		widget.ToolTipOpts.Delay(rackTipDelay),
		widget.ToolTipOpts.Position(widget.TOOLTIP_POS_CURSOR_STICKY),
		widget.ToolTipOpts.Offset(image.Point{X: theme.Px(12), Y: theme.Px(12)}),
	)
	return tip, view
}

// GetWidget, SetLocation and Validate make tipView a container child
// (widget.PreferredSizeLocateableWidget); PreferredSize is what the tooltip
// window is measured with.
func (v *TipView) GetWidget() *widget.Widget     { return v.widget }
func (v *TipView) SetLocation(r image.Rectangle) { v.widget.Rect = r }
func (v *TipView) Validate()                     {}

// SetModel swaps the panel's content. Callers only call it when their state
// actually changed, so the layout below is re-measured rarely.
func (v *TipView) SetModel(m TooltipModel) {
	v.model = m
	v.layoutFor = -1
}

// Model returns the content the view currently draws.
func (v *TipView) Model() TooltipModel { return v.model }

// rowTip is one widget's hover-tooltip slot: a ToolTip over a structured
// panel, built on first use and refreshed only when the caller's change key
// moves — Sync runs every frame, the panel (and the measurements behind its
// preferred size) must not be rebuilt every frame with it. An empty key
// detaches the tooltip: the row no longer describes anything hoverable.
type RowTip struct {
	tip  *widget.ToolTip
	View *TipView
	tips []*widget.ToolTip
	key  string
}

// set attaches the model for key to w (building the tooltip if this slot
// has never had one) or drops the attachment when key is "". build is
// only called when the tooltip is first built or when key moved — a
// per-frame Sync must not pay for a model it throws away — and it always
// derives the model from the inputs key summarizes, so what shows is
// current, never stale.
func (t *RowTip) Set(th theme.Theme, w *widget.Widget, key string, build func() TooltipModel) {
	if key == "" {
		if t.tip != nil {
			w.ToolTips = nil
		}
		t.key = ""
		return
	}
	if t.tip == nil {
		t.tip, t.View = NewTip(th, build())
		t.tips = []*widget.ToolTip{t.tip}
	} else if t.key != key {
		t.View.SetModel(build())
	}
	t.key = key
	w.ToolTips = t.tips
}

// PreferredSize is the panel at its content width — the size widget.ToolTip
// asks for every frame it is showing.
func (v *TipView) PreferredSize() (int, int) {
	l := v.Panel(tipInnerWidth(v.th, v.model))
	return l.Size.X, l.Size.Y
}

// panel returns the cached layout for an inner width of iw.
func (v *TipView) Panel(iw int) tipLayout {
	if v.layoutFor != iw {
		v.layout = tipLayoutAt(v.th, v.model, iw)
		v.layoutFor = iw
	}
	return v.layout
}

// Render paints the panel, clamped into the screen it is handed.
func (v *TipView) Render(screen *ebiten.Image) {
	r := v.widget.Rect
	if r.Dx() <= 0 || r.Dy() <= 0 {
		return
	}
	l := v.Panel(r.Dx() - 2*theme.Px(TipPadX))
	drawTooltip(screen, v.model, l, v.th, ClampTipRect(r, screen.Bounds()).Min)
}

// ---------------------------------------------------------------------------
// Content builders
// ---------------------------------------------------------------------------

// goodTint/dangerTint are the value tints for signed modifiers.
var (
	goodTint   = theme.DefaultPalette().Good
	dangerTint = theme.DefaultPalette().Danger
)

// signed renders v with an explicit sign — "+60.0" / "−45.0" (U+2212, the
// minus IBM Plex Sans ships).
func signed(v float64, prec int) string {
	if v < 0 {
		return "−" + theme.GroupFloat(-v, prec)
	}
	return "+" + theme.GroupFloat(v, prec)
}

// slotLabel renders a slot reference as "High slot 1" — the rack cells and
// the Fitting window's slot rows quote the same wording in their sub line.
func SlotLabel(ref sim.SlotRef) string {
	return theme.KindLabel(ref.Class) + " slot " + theme.FormatInt(int64(ref.Index+1))
}

// tipSub is the dim class line: the caller's slot text and the module kind
// with its underscores spaced out ("High slot 1 · Mining laser").
func TipSub(slotText, kind string) string {
	word := theme.KindLabel(strings.ReplaceAll(kind, "_", " "))
	if slotText == "" {
		return word
	}
	return slotText + " · " + word
}

// moduleSections builds every stat section a module's def carries EXCEPT
// ACTIVATION: the weapon/mining blocks, the active-effect families and
// the passive bonuses, plus the loaded-charge block when the caller
// knows the instance's charge. It is the shared body behind moduleTip
// (rack + fitting tooltips) and the Show Info window's module page — one
// builder, two renderers, so the two surfaces can never disagree about a
// module's numbers. An unset field never becomes a row (never "N/A").
func ModuleSections(def *content.ModuleDef, charge string, chargeQty int) []TooltipSection {
	if def == nil {
		return nil
	}
	var secs []TooltipSection

	switch {
	case def.Weapon != nil:
		w := def.Weapon
		secs = append(secs,
			TooltipSection{Label: "DAMAGE", Rows: []TooltipRow{
				{Label: "Per shot", Value: theme.GroupFloat(w.Damage.Total(), 1)},
				{Label: "Electro-magnetic", Value: theme.GroupFloat(w.Damage.EM, 1)},
				{Label: "Thermal", Value: theme.GroupFloat(w.Damage.Thermal, 1)},
				{Label: "Kinetic", Value: theme.GroupFloat(w.Damage.Kinetic, 1)},
				{Label: "Explosive", Value: theme.GroupFloat(w.Damage.Explosive, 1)},
			}},
			TooltipSection{Label: "RANGE", Rows: []TooltipRow{
				{Label: "Optimal", Value: theme.FormatDistance(w.Optimal)},
				{Label: "Falloff", Value: theme.FormatDistance(w.Falloff)},
			}},
			TooltipSection{Label: "FIRING", Rows: []TooltipRow{
				{Label: "Tracking", Value: theme.GroupFloat(w.Tracking, 2) + " rad/s"},
			}},
		)
		if w.AmmoClass != "" && charge != "" {
			rows := []TooltipRow{{Label: "Loaded", Value: charge}}
			if chargeQty > 0 {
				rows = append(rows, TooltipRow{Label: "In cargo", Value: theme.FormatInt(int64(chargeQty))})
			}
			secs = append(secs, TooltipSection{Label: "CHARGE", Rows: rows})
		}
	case def.Mining != nil:
		secs = append(secs, TooltipSection{Label: "YIELD", Rows: []TooltipRow{
			{Label: "Per cycle", Value: theme.FormatVolume(def.Mining.VolumePerCycle)},
		}})
	}

	// Active effect blocks: one section per family (content defs carry one
	// family each; a def with several still gets all of its rows).
	if b := def.Boost; b != nil {
		if b.ShieldBoostPerCycle > 0 || b.ArmorRepPerCycle > 0 {
			var rows []TooltipRow
			if b.ShieldBoostPerCycle > 0 {
				rows = append(rows, TooltipRow{Label: "Shield", Value: signed(b.ShieldBoostPerCycle, 1) + " HP", Tint: goodTint})
			}
			if b.ArmorRepPerCycle > 0 {
				rows = append(rows, TooltipRow{Label: "Armor", Value: signed(b.ArmorRepPerCycle, 1) + " HP", Tint: goodTint})
			}
			secs = append(secs, TooltipSection{Label: "REPAIR", Rows: rows})
		}
		if b.CapRestore > 0 {
			secs = append(secs, TooltipSection{Label: "CAPACITOR", Rows: []TooltipRow{
				{Label: "Restored", Value: signed(b.CapRestore, 1) + " GJ", Tint: goodTint},
			}})
		}
		if b.SpeedMultiplier > 1 {
			secs = append(secs, TooltipSection{Label: "PROPULSION", Rows: []TooltipRow{
				{Label: "Speed", Value: "×" + theme.GroupFloat(b.SpeedMultiplier, 2), Tint: goodTint},
			}})
		}
		if b.WebStrength > 0 || b.ScramDuration > 0 {
			var rows []TooltipRow
			if b.WebStrength > 0 {
				// A negative modifier on the target: tinted by its sign.
				rows = append(rows, TooltipRow{Label: "Target speed", Value: "−" + theme.PctLabel(b.WebStrength), Tint: dangerTint})
			}
			if b.ScramDuration > 0 {
				rows = append(rows, TooltipRow{Label: "Warp scramble", Value: theme.GroupFloat(b.ScramDuration, 1) + " s"})
			}
			secs = append(secs, TooltipSection{Label: "EFFECT", Rows: rows})
		}
	}
	if ps := def.Passive; ps != nil {
		var rows []TooltipRow
		if ps.ShieldHP > 0 {
			rows = append(rows, TooltipRow{Label: "Shield", Value: signed(ps.ShieldHP, 0) + " HP", Tint: goodTint})
		}
		if ps.ArmorHP > 0 {
			rows = append(rows, TooltipRow{Label: "Armor", Value: signed(ps.ArmorHP, 0) + " HP", Tint: goodTint})
		}
		if ps.Capacitor > 0 {
			rows = append(rows, TooltipRow{Label: "Capacitor", Value: signed(ps.Capacitor, 1) + " GJ", Tint: goodTint})
		}
		if ps.Recharge != 0 {
			// Seconds off the recharge time constant: negative is faster,
			// so the sign that makes the sim number smaller is the good one.
			rows = append(rows, TooltipRow{Label: "Recharge", Value: signed(ps.Recharge, 1) + " s", Tint: goodTint})
		}
		if ps.MaxSpeed > 0 {
			rows = append(rows, TooltipRow{Label: "Speed", Value: signed(ps.MaxSpeed, 0) + " m/s", Tint: goodTint})
		}
		for _, rt := range []struct {
			label string
			v     float64
		}{
			{"EM resist", ps.ResistBonus.EM},
			{"Thermal resist", ps.ResistBonus.Thermal},
			{"Kinetic resist", ps.ResistBonus.Kinetic},
			{"Explosive resist", ps.ResistBonus.Explosive},
		} {
			if rt.v > 0 {
				rows = append(rows, TooltipRow{Label: rt.label, Value: "+" + theme.PctLabel(rt.v), Tint: goodTint})
			}
		}
		if len(rows) > 0 {
			secs = append(secs, TooltipSection{Label: "BONUSES", Rows: rows})
		}
	}
	return secs
}

// ModuleTip builds the shared module tooltip model — the rack cells and the
// Fitting window's module rows both come through here, so the two can never
// disagree about a module's numbers.
//
//	slotText   sub-line slot text ("" = slotless, e.g. a hangar row)
//	charge     loaded charge name ("" = unknown: no CHARGE section is
//	           guessed rather than shown wrong)
//	chargeQty  charge count in cargo (0 = no count row)
//	hints      the consumer's footer lines — the rack knows what a click
//	           does, the fitting table does not
//
// Every row is driven by what the ModuleDef actually carries; a field the
// def doesn't set simply never becomes a row (never "N/A").
func ModuleTip(def *content.ModuleDef, slotText, charge string, chargeQty int, hints ...string) TooltipModel {
	if def == nil {
		return TooltipModel{}
	}
	m := TooltipModel{
		Icon:  theme.ModuleIcon(def.Kind),
		Title: def.Name,
		Sub:   TipSub(slotText, def.Kind),
		Hints: hints,
	}
	m.Sections = ModuleSections(def, charge, chargeQty)
	if def.ActivationTime > 0 || def.ActivationCost > 0 {
		var rows []TooltipRow
		if def.ActivationTime > 0 {
			rows = append(rows, TooltipRow{Label: "Cycle", Value: theme.GroupFloat(def.ActivationTime, 1) + " s"})
		}
		if def.ActivationCost > 0 {
			rows = append(rows, TooltipRow{Label: "Cap use", Value: theme.GroupFloat(def.ActivationCost, 1) + " GJ"})
		}
		m.Sections = append(m.Sections, TooltipSection{Label: "ACTIVATION", Rows: rows})
	}
	return m
}

// ClipTip adds the bay ("clip") row to a charge-fed module's tooltip model:
// the bay's m³, the whole rounds it holds of the round it would load and the
// load cycle (one firing cycle). round is that charge (nil = the bay waits
// for a choice, so the row says so and counts nothing). A module without a
// bay comes back unchanged — no row, no placeholder.
func ClipTip(m TooltipModel, def *content.ModuleDef, round *content.ItemDef) TooltipModel {
	if clipBay(def) <= 0 {
		return m
	}
	row := TooltipRow{Label: "Clip", Value: "waiting for ammo selection"}
	if round != nil && round.Volume > 0 {
		row.Value = theme.FormatVolume(clipBay(def)) + " · " +
			theme.FormatInt(int64(def.BayRounds(round.Volume))) + " rounds · " +
			theme.GroupFloat(clipCycle(def), 1) + " s"
	}
	// The bay belongs to the charge block; a model without one (the cap
	// booster, whose charge the tooltip does not name) gets its own.
	for i := range m.Sections {
		if m.Sections[i].Label == "CHARGE" {
			m.Sections[i].Rows = append(m.Sections[i].Rows, row)
			return m
		}
	}
	m.Sections = append(m.Sections, TooltipSection{Label: "CHARGE", Rows: []TooltipRow{row}})
	return m
}

// clipBay is a module's bay capacity in m³ (0 = the module loads no rounds):
// the gun's weapon bay, or the cap booster's cell bay.
func clipBay(def *content.ModuleDef) float64 {
	switch {
	case def == nil:
		return 0
	case def.Weapon != nil:
		return def.Weapon.BayM3
	case def.Boost != nil:
		return def.Boost.BayM3
	}
	return 0
}

// clipCycle is the time a bay load costs: one firing cycle (the shot time),
// the activation time for the cap booster.
func clipCycle(def *content.ModuleDef) float64 {
	if def.Weapon != nil && def.Weapon.ShotTime > 0 {
		return def.Weapon.ShotTime
	}
	return def.ActivationTime
}
