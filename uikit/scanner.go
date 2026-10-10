package uikit

import (
	"image"
	"sort"
	"strconv"
	"time"

	"github.com/ebitenui/ebitenui/widget"
	"github.com/entieve/entieve/render"
	"github.com/entieve/entieve/sim"
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
	env    Env
	win    *widget.Window
	bar    *widget.ProgressBar
	status *widget.Text
	empty  *widget.Text
	labels [scanMaxLines]*widget.Text

	base   time.Time // sweep clock origin (wall time)
	prev   float64   // previous sweep phase (wrap detection)
	counts []scanCount
}

// NewScanner builds the scanner window (draggable, fixed size) over the
// given environment. The default location sits above the bottom-left log.
func NewScanner(env Env) *Scanner {
	p := env.Pal
	s := &Scanner{env: env, base: time.Now()}

	s.status = widget.NewText(
		widget.TextOpts.Text("Sweep 0%", env.Face(StepTiny), solid(p.Dim)),
		widget.TextOpts.WidgetOpts(widget.WidgetOpts.LayoutData(widget.RowLayoutData{Stretch: true})),
	)
	rng := widget.NewText(
		widget.TextOpts.Text("range "+formatDistance(3*sim.SensorRange), env.Face(StepTiny), solid(p.Dim)),
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
		widget.ProgressBarOpts.WidgetOpts(widget.WidgetOpts.MinSize(0, 18),
			widget.WidgetOpts.LayoutData(widget.RowLayoutData{Stretch: true})),
	)

	s.empty = widget.NewText(widget.TextOpts.Text("No off-grid contacts", env.Face(StepSmall), solid(p.Dim)))
	lines := widget.NewContainer(
		widget.ContainerOpts.Layout(widget.NewRowLayout(
			widget.RowLayoutOpts.Direction(widget.DirectionVertical),
		)),
		widget.ContainerOpts.WidgetOpts(widget.WidgetOpts.MinSize(0, scanMaxLines*23)),
	)
	for i := range s.labels {
		lbl := widget.NewText(widget.TextOpts.Text("", env.Face(StepSmall), solid(p.Text)))
		showWidget(lbl.GetWidget(), false)
		s.labels[i] = lbl
		lines.AddChild(lbl)
	}

	body := widget.NewContainer(
		widget.ContainerOpts.BackgroundImage(bordered(solid(p.BG), nrgba(p.Border, 200), 1)),
		widget.ContainerOpts.Layout(widget.NewRowLayout(
			widget.RowLayoutOpts.Direction(widget.DirectionVertical),
			widget.RowLayoutOpts.Spacing(contentsPad),
			widget.RowLayoutOpts.Padding(widget.NewInsetsSimple(contentsPad)),
		)),
	)
	body.AddChild(status, s.bar, s.empty)
	body.AddChild(lines)

	title := widget.NewContainer(
		widget.ContainerOpts.BackgroundImage(bordered(solid(p.BGHard), nrgba(p.Border, 200), 1)),
		widget.ContainerOpts.Layout(widget.NewAnchorLayout()),
	)
	title.AddChild(widget.NewText(
		widget.TextOpts.Text("Scanner", env.Face(StepBase), solid(p.Accent)),
		widget.TextOpts.WidgetOpts(widget.WidgetOpts.LayoutData(widget.AnchorLayoutData{
			HorizontalPosition: widget.AnchorLayoutPositionStart,
			VerticalPosition:   widget.AnchorLayoutPositionCenter,
			Padding:            widget.NewInsetsSimple(contentsPad),
		})),
	))

	s.win = widget.NewWindow(
		widget.WindowOpts.Contents(body),
		widget.WindowOpts.TitleBar(title, titleBarH),
		widget.WindowOpts.Draggable(),
		widget.WindowOpts.MinSize(260, titleBarH+scanBodyH),
		widget.WindowOpts.Location(image.Rect(10, 251, 10+scanW, 251+titleBarH+scanBodyH)),
	)
	return s
}

// Win returns the scanner window for the interface to add.
func (s *Scanner) Win() *widget.Window { return s.win }

// Sync advances the sweep: the bar and status line track the phase, and the
// anonymous counts refresh whenever a sweep completes (and on first sync).
func (s *Scanner) Sync() {
	if s.env.Sim == nil {
		return
	}
	phase := scanPhase(time.Since(s.base).Seconds(), scanCycle)
	if s.counts == nil || phase < s.prev {
		s.counts = scanCounts(s.env.Sim.Grid(), sim.SensorRange)
		s.applyCounts()
	}
	s.prev = phase
	s.bar.SetCurrent(int(phase * scanMax))
	s.status.Label = "Sweep " + pctLabel(phase)
}

// applyCounts pushes the last sweep's counts into the label pool.
func (s *Scanner) applyCounts() {
	empty := len(s.counts) == 0
	showWidget(s.empty.GetWidget(), empty)
	for i, lbl := range s.labels {
		if empty || i >= len(s.counts) {
			showWidget(lbl.GetWidget(), false)
			continue
		}
		c := s.counts[i]
		showWidget(lbl.GetWidget(), true)
		lbl.Label = scanKindLabel(c.Kind, c.N)
		lbl.SetColor(render.BracketColor(c.Kind, false))
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
	return kindLabel(kind) + " ×" + strconv.Itoa(n)
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
