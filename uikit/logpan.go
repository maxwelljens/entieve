package uikit

import (
	"fmt"
	"image"
	"image/color"
	"strconv"
	"strings"

	"github.com/ebitenui/ebitenui/widget"
	"github.com/entieve/entieve/sim"
)

// Log panel geometry: five visible lines like the legacy log window, a
// 200-line history cap (older lines are dropped, not truncated in
// place), and the window defaults. The log is a normal draggable/
// resizable window docked bottom-left: the free pocket left of the HUD
// bar is 292 px wide at the 1024×700 minimum (the 420 px bar starts at
// x=302), so the default stays under it — with the defaults the window
// never overlaps the HUD, rack or overview.
const (
	logViewLines = 5
	logViewW     = 240
	logViewH     = logViewLines * 23 // lines × lineH(FontSmall)
	logMaxLines  = 200

	logDefaultW = 288 // fits the bottom-left pocket (see above)
	logDefaultH = 240
	logMinW     = logViewW + 2*contentsPad             // text-area floor + body padding
	logMinH     = titleBarH + logViewH + 2*contentsPad // + title bar
)

// logDefaultRect is the construction-time placement (bottom-left pocket
// at the 1024×700 minimum); the game re-anchors it to the real screen
// on first layout, like every other movable window.
func logDefaultRect() image.Rectangle {
	return image.Rect(10, 700-10-logDefaultH, 10+logDefaultW, 700-10)
}

// LogPanel is the combat/action log: a scrollable text area whose lines
// carry a dim sim-time stamp and a severity color through BBCode markup,
// inside a draggable/resizable window titled "Log". It is push-driven —
// the integrator forwards sim.Events() to Push each frame and may add
// its own lines with PushLine.
type LogPanel struct {
	env   Env
	win   *widget.Window
	ta    *widget.TextArea
	lines []string
}

// NewLog builds the log panel over the given environment as a window on
// the interface's window stack (WindowPanel): default logDefaultW ×
// logDefaultH, bottom-left, edge-resize chrome attached.
func NewLog(env Env) *LogPanel {
	p := env.Pal
	l := &LogPanel{env: env}

	// ScrollEnd keeps the view pinned to the newest line on every append;
	// the theme supplies the scrollbar chrome.
	l.ta = widget.NewTextArea(
		widget.TextAreaOpts.ProcessBBCode(true),
		widget.TextAreaOpts.ShowVerticalScrollbar(),
		widget.TextAreaOpts.VerticalScrollMode(widget.ScrollEnd),
		widget.TextAreaOpts.FontFace(env.Face(StepSmall)),
		widget.TextAreaOpts.FontColor(solid(p.Text)),
		widget.TextAreaOpts.ContainerOpts(widget.ContainerOpts.WidgetOpts(
			widget.WidgetOpts.LayoutData(widget.RowLayoutData{Stretch: true}),
			widget.WidgetOpts.MinSize(logViewW, logViewH),
		)),
	)

	body := widget.NewContainer(
		widget.ContainerOpts.BackgroundImage(bordered(solid(p.BG), nrgba(p.Border, 200), 1)),
		widget.ContainerOpts.Layout(widget.NewRowLayout(
			widget.RowLayoutOpts.Direction(widget.DirectionVertical),
			widget.RowLayoutOpts.Padding(widget.NewInsetsSimple(contentsPad)),
		)),
		widget.ContainerOpts.WidgetOpts(widget.WidgetOpts.LayoutData(widget.RowLayoutData{Stretch: true})),
	)
	body.AddChild(l.ta)

	titleText := widget.NewText(
		widget.TextOpts.Text("Log", env.Face(StepBase), solid(p.Accent)),
		widget.TextOpts.WidgetOpts(widget.WidgetOpts.LayoutData(widget.AnchorLayoutData{
			HorizontalPosition: widget.AnchorLayoutPositionStart,
			VerticalPosition:   widget.AnchorLayoutPositionCenter,
			Padding:            widget.NewInsetsSimple(contentsPad),
		})),
	)
	title := widget.NewContainer(
		widget.ContainerOpts.BackgroundImage(bordered(solid(p.BGHard), nrgba(p.Border, 200), 1)),
		widget.ContainerOpts.Layout(widget.NewAnchorLayout()),
		widget.ContainerOpts.WidgetOpts(
			widget.WidgetOpts.MinSize(0, titleBarH),
		),
	)
	title.AddChild(titleText)

	l.win = widget.NewWindow(
		widget.WindowOpts.Contents(body),
		widget.WindowOpts.TitleBar(title, titleBarH),
		widget.WindowOpts.Draggable(),
		widget.WindowOpts.MinSize(logMinW, logMinH),
		widget.WindowOpts.Location(logDefaultRect()),
	)
	makeEdgeResizable(l.win)
	return l
}

// Container returns the log window's root container. The log used to be
// root-anchored chrome; the game now gates world input over this rect
// through the normal window path (it is also reachable via Win).
func (l *LogPanel) Container() *widget.Container {
	return l.win.GetContainer().(*widget.Container)
}

// Win returns the log window for Kit.AddWindow: LogPanel is a
// WindowPanel, mounted and toggled like every other window.
func (l *LogPanel) Win() *widget.Window { return l.win }

// Sync is a no-op: the log is push-driven so the integrator owns the sim
// event drain (Push is the seam; draining here would swallow events other
// consumers may want).
func (l *LogPanel) Sync() {}

// Lines returns the rendered log lines (including their BBCode markup) so
// history round-trips exactly through SetLines across a save/load rebuild.
func (l *LogPanel) Lines() []string {
	out := make([]string, len(l.lines))
	copy(out, l.lines)
	return out
}

// SetLines replaces the log content (e.g. after a save/load rebuild).
// The cap still applies: only the newest lines survive.
func (l *LogPanel) SetLines(lines []string) {
	l.lines = nil
	for _, ln := range lines {
		l.append(ln)
	}
	l.refresh()
}

// Push appends sim events as colored lines, each stamped with the event's
// own sim time.
func (l *LogPanel) Push(evs []sim.Event) {
	for _, ev := range evs {
		l.append(l.compose(ev.Severity, l.stamp(ev.Time), ev.Text))
	}
	if len(evs) > 0 {
		l.refresh()
	}
}

// PushLine appends one line stamped with the current sim time, colored by
// severity.
func (l *LogPanel) PushLine(sev sim.Severity, text string) {
	now := 0.0
	if l.env.Sim != nil {
		now = l.env.Sim.Time()
	}
	l.append(l.compose(sev, l.stamp(now), text))
	l.refresh()
}

// append records one composed line and drops the oldest beyond the cap.
func (l *LogPanel) append(line string) {
	l.lines = append(l.lines, line)
	if len(l.lines) > logMaxLines {
		over := len(l.lines) - logMaxLines
		l.lines = append(l.lines[:0], l.lines[over:]...)
	}
}

// refresh pushes the history into the text area (ScrollEnd keeps the view
// at the bottom).
func (l *LogPanel) refresh() { l.ta.SetText(strings.Join(l.lines, "\n")) }

// compose renders one line as BBCode: dim "[m:ss]" timestamp, then the
// severity color wrapped around the text.
func (l *LogPanel) compose(sev sim.Severity, stamp, text string) string {
	p := l.env.Pal
	return "[color=" + hexColor(p.Dim) + "][" + stamp + "][/color] " +
		"[color=" + hexColor(l.sevColor(sev)) + "]" + text + "[/color]"
}

// sevColor maps a sim severity to the palette color (SevInfo → info blue).
func (l *LogPanel) sevColor(sev sim.Severity) color.RGBA {
	switch sev {
	case sim.SevGood:
		return l.env.Pal.Good
	case sim.SevWarning:
		return l.env.Pal.Warn
	case sim.SevDanger:
		return l.env.Pal.Danger
	default:
		return l.env.Pal.Info
	}
}

// stamp renders sim seconds as m:ss for the line prefix.
func (l *LogPanel) stamp(sec float64) string {
	if sec < 0 {
		sec = 0
	}
	s := int64(sec)
	secs := s % 60
	out := strconv.FormatInt(s/60, 10) + ":"
	if secs < 10 {
		out += "0"
	}
	return out + strconv.FormatInt(secs, 10)
}

// hexColor renders a palette color as the #rrggbb form BBCode expects.
func hexColor(c color.RGBA) string {
	return fmt.Sprintf("#%02x%02x%02x", c.R, c.G, c.B)
}
