package screens

import (
	"image"
	"sort"
	"strconv"
	"time"

	"github.com/ebitenui/ebitenui/widget"
	"github.com/entieve/entieve/render"
	"github.com/entieve/entieve/sim"
	"github.com/entieve/entieve/uikit/theme"
	"github.com/entieve/entieve/uikit/widgets"
)

// Scanner geometry: a fixed body (status line, sweep bar, six contact rows)
// so the window never resizes as contacts come and go.
const (
	scanCycle    = 2.0  // sweep period, seconds (legacy ui scanCycle)
	scanMaxLines = 6    // contact rows reserved
	scanMax      = 1000 // sweep bar resolution (0.1 % steps)
	scanW        = 300
	scanBodyH    = 204 // padding + status + bar + rows, with slack
)

// scanCount is one anonymous off-grid contact group.
type scanCount struct {
	Kind string
	N    int
}

// scanKindOrder fixes the display order of aggregated kinds: the known
// contact types first, everything else alphabetically after them.
var scanKindOrder = [...]string{"ship", "asteroid", "wreck"}

// Scanner is the off-grid contact sweep (the anti-omniscience feature): a
// ~2 s progress bar plus anonymous per-kind counts of everything that is
// NOT visible to the player but sits within 3 × sensor range. No names, no
// distances — you learn what is out there, never exactly where.
type Scanner struct {
	th     theme.Theme
	cv     widgets.ContentView
	win    *widget.Window
	bar    *widget.ProgressBar
	status *widget.Text
	empty  *widget.Text
	labels [scanMaxLines]*widget.Text
	// Each contact line is a row: the kind glyph then the count label.
	// Visibility flips on the row (so the icon and label disappear
	// together) and on the label itself (the pool's own state).
	contactRows [scanMaxLines]*widget.Container
	contactIcon [scanMaxLines]*theme.IconBox

	base   time.Time // sweep clock origin (wall time)
	prev   float64   // previous sweep phase (wrap detection)
	counts []scanCount
}

// NewScanner builds the scanner window (draggable, fixed size) over the
// given environment. The default location sits above the bottom-left log.
func NewScanner(env interface {
	theme.Theme
	widgets.ContentView
}) *Scanner {
	p := env.Pal()
	s := &Scanner{th: env, cv: env, base: time.Now()}

	s.status = widget.NewText(
		widget.TextOpts.Text("Sweep 0%", env.Face(theme.StepTiny), theme.Solid(p.Dim)),
		widget.TextOpts.WidgetOpts(widget.WidgetOpts.LayoutData(widget.RowLayoutData{Stretch: true})),
	)
	rng := widget.NewText(
		widget.TextOpts.Text("range "+theme.FormatDistance(3*sim.SensorRange), env.Face(theme.StepTiny), theme.Solid(p.Dim)),
		widget.TextOpts.Position(widget.TextPositionEnd, widget.TextPositionStart),
	)
	status := widget.NewContainer(
		widget.ContainerOpts.Layout(widget.NewAnchorLayout()),
		widget.ContainerOpts.WidgetOpts(widget.WidgetOpts.LayoutData(widget.RowLayoutData{Stretch: true})),
	)
	// Anchor layout: sweep label left, range label right.
	s.status.GetWidget().LayoutData = widget.AnchorLayoutData{
		HorizontalPosition: widget.AnchorLayoutPositionStart,
		VerticalPosition:   widget.AnchorLayoutPositionCenter,
	}
	rng.GetWidget().LayoutData = widget.AnchorLayoutData{
		HorizontalPosition: widget.AnchorLayoutPositionEnd,
		VerticalPosition:   widget.AnchorLayoutPositionCenter,
	}
	status.AddChild(s.status, rng)

	s.bar = widget.NewProgressBar(
		widget.ProgressBarOpts.Values(0, scanMax, 0),
		widget.ProgressBarOpts.WidgetOpts(widget.WidgetOpts.MinSize(0, theme.Px(18)),
			widget.WidgetOpts.LayoutData(widget.RowLayoutData{Stretch: true})),
	)

	s.empty = widget.NewText(widget.TextOpts.Text("No off-grid contacts", env.Face(theme.StepSmall), theme.Solid(p.Dim)))
	lines := widget.NewContainer(
		widget.ContainerOpts.Layout(widget.NewRowLayout(
			widget.RowLayoutOpts.Direction(widget.DirectionVertical),
		)),
		widget.ContainerOpts.WidgetOpts(widget.WidgetOpts.MinSize(0, theme.Px(scanMaxLines*23))),
	)
	for i := range s.labels {
		lbl := widget.NewText(widget.TextOpts.Text("", env.Face(theme.StepSmall), theme.Solid(p.Text)))
		theme.ShowWidget(lbl.GetWidget(), false)
		s.labels[i] = lbl
		box := theme.NewIconBox(theme.IconInline)
		s.contactIcon[i] = box
		row := widget.NewContainer(
			widget.ContainerOpts.Layout(widget.NewRowLayout(
				widget.RowLayoutOpts.Direction(widget.DirectionHorizontal),
				widget.RowLayoutOpts.Spacing(theme.Px(theme.IconGap)),
			)),
		)
		theme.ShowWidget(row.GetWidget(), false)
		row.AddChild(box.Widget(), lbl)
		s.contactRows[i] = row
		lines.AddChild(row)
	}

	body := widget.NewContainer(
		widget.ContainerOpts.BackgroundImage(theme.WindowBodyBox(p, theme.WindowBody(p.BG), theme.NRGBA(p.Border, 200))),
		widget.ContainerOpts.Layout(widget.NewRowLayout(
			widget.RowLayoutOpts.Direction(widget.DirectionVertical),
			widget.RowLayoutOpts.Spacing(theme.Px(theme.ContentsPad)),
			widget.RowLayoutOpts.Padding(widget.NewInsetsSimple(theme.Px(theme.ContentsPad))),
		)),
	)
	body.AddChild(status, s.bar, s.empty)
	body.AddChild(lines)

	title, _ := theme.NewTitleBar(env, "Scanner", "svc_scanner")

	s.win = widget.NewWindow(
		widget.WindowOpts.Contents(body),
		widget.WindowOpts.TitleBar(title, theme.Px(theme.TitleBarH)),
		widget.WindowOpts.MinSize(theme.Px(260), theme.Px(theme.TitleBarH+scanBodyH)),
		// Default clears the rail's left gutter (RailInset); the game
		// re-anchors it to the centre column on first layout.
		widget.WindowOpts.Location(image.Rect(theme.Px(10+RailInset), theme.Px(251), theme.Px(10+RailInset+scanW), theme.Px(251+theme.TitleBarH+scanBodyH))),
	)
	return s
}

// Win returns the scanner window for the interface to add.
func (s *Scanner) Win() *widget.Window { return s.win }

// Sync advances the sweep: the bar and status line track the phase, and the
// anonymous counts refresh whenever a sweep completes (and on first sync).
func (s *Scanner) Sync() {
	if s.cv == nil {
		return
	}
	phase := scanPhase(time.Since(s.base).Seconds(), scanCycle)
	if s.counts == nil || phase < s.prev {
		s.counts = scanCounts(s.cv.Grid(), sim.SensorRange)
		s.applyCounts()
	}
	s.prev = phase
	s.bar.SetCurrent(int(phase * scanMax))
	s.status.Label = "Sweep " + theme.PctLabel(phase)
}

// applyCounts pushes the last sweep's counts into the label pool.
func (s *Scanner) applyCounts() {
	empty := len(s.counts) == 0
	theme.ShowWidget(s.empty.GetWidget(), empty)
	for i, lbl := range s.labels {
		if empty || i >= len(s.counts) {
			theme.ShowWidget(lbl.GetWidget(), false)
			theme.ShowWidget(s.contactRows[i].GetWidget(), false)
			continue
		}
		c := s.counts[i]
		theme.ShowWidget(lbl.GetWidget(), true)
		theme.ShowWidget(s.contactRows[i].GetWidget(), true)
		lbl.Label = scanKindLabel(c.Kind, c.N)
		// The glyph wears the same bracket color as the count, so the
		// line reads as one unit.
		col := render.BracketColor(c.Kind, false)
		lbl.SetColor(col)
		s.contactIcon[i].Set(theme.KindIcon(c.Kind), col)
	}
}

// scanCounts aggregates off-grid contacts (Info.Visible == false) within
// 3 × sensorRange of the player into anonymous per-kind counts, in a
// deterministic order. The player and always-visible landmarks never appear.
func scanCounts(infos []sim.Info, sensorRange float64) []scanCount {
	maxD := 3 * sensorRange
	counts := map[string]int{}
	for _, inf := range infos {
		if inf.Visible || inf.Kind == sim.KindPlayer {
			continue
		}
		if inf.Distance > maxD {
			continue
		}
		counts[inf.Kind]++
	}
	out := make([]scanCount, 0, len(counts))
	seen := map[string]bool{}
	for _, k := range scanKindOrder {
		if counts[k] > 0 {
			out = append(out, scanCount{Kind: k, N: counts[k]})
			seen[k] = true
		}
	}
	var rest []string
	for k := range counts {
		if !seen[k] {
			rest = append(rest, k)
		}
	}
	sort.Strings(rest)
	for _, k := range rest {
		out = append(out, scanCount{Kind: k, N: counts[k]})
	}
	return out
}

// scanKindLabel renders an anonymous count label: "Ship ×3".
func scanKindLabel(kind string, n int) string {
	return theme.KindLabel(kind) + " ×" + strconv.Itoa(n)
}

// scanPhase returns the sweep progress in [0,1) for elapsed time t over a
// cycle of the given period.
func scanPhase(t, cycle float64) float64 {
	if cycle <= 0 {
		return 0
	}
	m := t - float64(int(t/cycle))*cycle
	if m < 0 {
		m += cycle
	}
	return m / cycle
}
