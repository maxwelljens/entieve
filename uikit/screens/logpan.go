package screens

import (
	"fmt"
	"image"
	"image/color"
	"strconv"
	"strings"

	"github.com/ebitenui/ebitenui/widget"
	"github.com/entieve/entieve/sim"
	"github.com/entieve/entieve/uikit/theme"
	"github.com/entieve/entieve/uikit/widgets"
)

// Log panel geometry: five visible lines like the legacy log window, a
// 200-line history cap (older lines are dropped, not truncated in
// place), and the window defaults. The log is a normal draggable/
// resizable window docked bottom-left: with the rail gutter (RailInset)
// on its left, 264 keeps its right edge at the historical 361 (the pocket
// clear of the HUD bar at the 1024×700 minimum), so it still fits with
// room to spare over the HUD — with the defaults the window never
// overlaps the HUD, rack or overview.
const (
	logViewLines = 5
	logViewW     = 240
	logViewH     = logViewLines * 23 // lines × lineH(FontSmall)
	logMaxLines  = 200

	logDefaultW = 264 // 361 − (10 + RailInset): the pocket right edge, clear of the HUD bar
	logDefaultH = 240
	logMinW     = logViewW + 2*theme.ContentsPad                   // text-area floor + body padding
	logMinH     = theme.TitleBarH + logViewH + 2*theme.ContentsPad // + title bar
)

// logDefaultRect is the construction-time placement (bottom-left pocket
// at the 1024×700 minimum, clear of the rail's left gutter); the game
// re-anchors it to the real screen on first layout, like every other
// movable window — its spec row carries the same 10+RailInset inset.
func logDefaultRect() image.Rectangle {
	return image.Rect(theme.Px(10+RailInset), theme.Px(700-10-logDefaultH), theme.Px(10+RailInset+logDefaultW), theme.Px(700-10))
}

// LogPanel is the combat/action log: a scrollable text area whose lines
// carry a dim sim-time stamp and a severity color through BBCode markup,
// inside a draggable/resizable window titled "Log". It is push-driven —
// the integrator forwards sim.Events() to Push each frame and may add
// its own lines with PushLine.
type LogPanel struct {
	th    theme.Theme
	cv    widgets.ContentView
	win   *widget.Window
	ta    *widget.TextArea
	lines []string
}

// NewLog builds the log panel over the given environment as a window on
// the interface's window stack (WindowPanel): default logDefaultW ×
// logDefaultH, bottom-left, edge-resize chrome attached.
func NewLog(env interface {
	theme.Theme
	widgets.ContentView
}) *LogPanel {
	p := env.Pal()
	l := &LogPanel{th: env, cv: env}

	// The text area is pinned to the body's single stretched grid cell
	// (built below): RowLayout — the old body — stretches only in the
	// cross axis, so the primary axis kept the TextArea's PREFERRED
	// height, the whole history, and the rect leapt past the window on
	// every relayout. The grid's stretched row takes the remaining space
	// instead, the same bounded-viewport pattern uikit/widgets/table.go uses;
	// MinSize stays as the content floor the window MinSize math is
	// derived from. ScrollEnd keeps the view pinned to the newest line on
	// every append; the theme supplies the scrollbar chrome.
	l.ta = widget.NewTextArea(
		widget.TextAreaOpts.ProcessBBCode(true),
		widget.TextAreaOpts.ShowVerticalScrollbar(),
		widget.TextAreaOpts.VerticalScrollMode(widget.ScrollEnd),
		widget.TextAreaOpts.FontFace(env.Face(theme.StepSmall)),
		widget.TextAreaOpts.FontColor(theme.Solid(p.Text)),
		widget.TextAreaOpts.ContainerOpts(widget.ContainerOpts.WidgetOpts(
			widget.WidgetOpts.LayoutData(widget.GridLayoutData{}),
			widget.WidgetOpts.MinSize(theme.Px(logViewW), theme.Px(logViewH)),
		)),
	)

	body := widget.NewContainer(
		widget.ContainerOpts.BackgroundImage(theme.Bordered(theme.WindowBody(p.BG), theme.NRGBA(p.Border, 200), 1)),
		widget.ContainerOpts.Layout(widget.NewGridLayout(
			widget.GridLayoutOpts.Columns(1),
			widget.GridLayoutOpts.Stretch([]bool{true}, []bool{true}),
			widget.GridLayoutOpts.Padding(widget.NewInsetsSimple(theme.Px(theme.SpMd))),
		)),
		widget.ContainerOpts.WidgetOpts(widget.WidgetOpts.LayoutData(widget.GridLayoutData{})),
	)
	body.AddChild(l.ta)

	title, _ := theme.NewTitleBar(env, "Log", "svc_log")
	title.GetWidget().MinHeight = theme.Px(theme.TitleBarH)

	l.win = widget.NewWindow(
		widget.WindowOpts.Contents(body),
		widget.WindowOpts.TitleBar(title, theme.Px(theme.TitleBarH)),
		widget.WindowOpts.MinSize(theme.Px(logMinW), theme.Px(logMinH)),
		widget.WindowOpts.Location(logDefaultRect()),
	)
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
	if l.cv != nil {
		now = l.cv.Time()
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
	p := l.th.Pal()
	return "[color=" + hexColor(p.Dim) + "][" + stamp + "][/color] " +
		"[color=" + hexColor(l.sevColor(sev)) + "]" + text + "[/color]"
}

// sevColor maps a sim severity to the palette color (SevInfo → info blue).
func (l *LogPanel) sevColor(sev sim.Severity) color.RGBA {
	switch sev {
	case sim.SevGood:
		return l.th.Pal().Good
	case sim.SevWarning:
		return l.th.Pal().Warn
	case sim.SevDanger:
		return l.th.Pal().Danger
	default:
		return l.th.Pal().Info
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
