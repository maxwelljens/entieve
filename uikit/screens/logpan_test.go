package screens

import (
	"image"
	"strconv"
	"strings"
	"testing"

	"github.com/ebitenui/ebitenui/widget"
	"github.com/entieve/entieve/sim"
	"github.com/entieve/entieve/uikit/theme"
)

// TestLogPushAndColors covers the line pipeline: BBCode severity colors,
// the dim sim-time stamp, event pushes and the 200-line cap.
func TestLogPushAndColors(t *testing.T) {
	env, _ := newChromeEnv(t)
	l := NewLog(env)
	l.Sync() // no-op by contract: the log is push-driven
	if l.ta.GetText() != "" {
		t.Fatalf("fresh log text = %q, want empty", l.ta.GetText())
	}

	// Severity → the palette role the panel injects (the BBCode color the
	// text area must carry). Palette-derived, so the test survives a
	// reskin without pinning hexes of its own.
	pal := theme.DefaultPalette()
	for _, c := range []struct {
		sev  sim.Severity
		want string
	}{
		{sim.SevInfo, hexColor(pal.Info)},
		{sim.SevGood, hexColor(pal.Good)},
		{sim.SevWarning, hexColor(pal.Warn)},
		{sim.SevDanger, hexColor(pal.Danger)},
	} {
		text := "severity " + strconv.Itoa(int(c.sev))
		l.PushLine(c.sev, text)
		got := l.ta.GetText()
		if !strings.Contains(got, "[color="+c.want+"]"+text+"[/color]") {
			t.Errorf("severity %d line lacks color %s:\n%s", c.sev, c.want, got)
		}
	}

	// Push stamps each event with the event's own sim time.
	l.Push([]sim.Event{
		{Kind: sim.EvLog, Severity: sim.SevGood, Time: 65, Text: "wrecking shot"},
		{Kind: sim.EvLog, Severity: sim.SevDanger, Time: 7.5, Text: "structure hit"},
	})
	got := l.ta.GetText()
	if !strings.Contains(got, "[color="+hexColor(pal.Dim)+"][1:05][/color] [color="+hexColor(pal.Good)+"]wrecking shot[/color]") {
		t.Errorf("event line missing stamp/color:\n%s", got)
	}
	if !strings.Contains(got, "[0:07]") || !strings.Contains(got, "structure hit") {
		t.Errorf("event line missing truncated-second stamp:\n%s", got)
	}
	// PushLine stamps with the current sim time (0 at world start).
	if !strings.Contains(got, "[0:00]") {
		t.Errorf("PushLine line missing the sim-time stamp:\n%s", got)
	}
}

// TestLogCapDropsOldest pushes past the cap and checks the history keeps
// exactly the newest logMaxLines lines.
func TestLogCapDropsOldest(t *testing.T) {
	env, _ := newChromeEnv(t)
	l := NewLog(env)
	l.PushLine(sim.SevInfo, "head")
	for i := 0; i < logMaxLines+54; i++ {
		l.PushLine(sim.SevInfo, "filler "+strconv.Itoa(i))
	}
	lines := strings.Split(l.ta.GetText(), "\n")
	if len(lines) != logMaxLines {
		t.Fatalf("log holds %d lines, want %d", len(lines), logMaxLines)
	}
	// "head" plus fillers 0..53 were dropped (55 overflow lines).
	if !strings.Contains(lines[0], "filler 54") {
		t.Errorf("first line = %q, want filler 54", lines[0])
	}
	if !strings.Contains(lines[len(lines)-1], "filler "+strconv.Itoa(logMaxLines+53)) {
		t.Errorf("last line = %q", lines[len(lines)-1])
	}
}

// TestLogWindowDrawSmoke draws the log window (it is a WindowPanel now,
// not root-anchored chrome) at both design sizes; the default sits in
// the bottom-left pocket and MinSize holds the window to logMinW×logMinH.
func TestLogWindowDrawSmoke(t *testing.T) {
	env, _ := newChromeEnv(t)
	l := NewLog(env)
	l.PushLine(sim.SevInfo, "warp drive active")
	l.Push([]sim.Event{{Severity: sim.SevDanger, Time: 3, Text: "shield collapse"}})
	c := newChromeUI(env)
	c.AddWindow(l)
	c.Frame(t, 1280, 720)

	r := l.Win().GetContainer().GetWidget().Rect
	if r != logDefaultRect() {
		t.Errorf("log rect = %v, want the default %v", r, logDefaultRect())
	}
	if l.Container() != l.Win().GetContainer().(*widget.Container) {
		t.Error("Container() does not return the window's root container")
	}

	// The window's MinSize is the resize floor (edge grips clamp to it).
	l.Win().SetLocation(image.Rect(0, 0, 10, 10))
	if r := l.Win().GetContainer().GetWidget().Rect; r.Dx() != logMinW || r.Dy() != logMinH {
		t.Errorf("min clamp rect = %v, want %dx%d", r, logMinW, logMinH)
	}
	l.Win().SetLocation(logDefaultRect())
	c.Smoke(t)
}
