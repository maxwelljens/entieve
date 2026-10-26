package game

import (
	"bytes"
	"encoding/binary"
	"image"
	"io"
	"math"
	"os"
	"sort"
	"testing"

	"github.com/ebitenui/ebitenui/widget"
	sfxassets "github.com/entieve/entieve/assets/sfx"
	"github.com/entieve/entieve/content"
	"github.com/entieve/entieve/uikit/theme"
	"github.com/entieve/entieve/uikit/widgets"
	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/audio/wav"
)

// fakeUISound is the injected UISound the game tests pin the wiring with.
// It records cue names only — no audio context, no decode (the structural
// invariant: New installs nopUISound unless a test injects a fake, and the
// real players are built only by cmdRun's NewAudio).
type fakeUISound struct {
	plays      []string
	master, ui float64
	effective  float64
	setVolumes int
}

func (f *fakeUISound) Play(name string) { f.plays = append(f.plays, name) }
func (f *fakeUISound) SetVolume(master, ui float64) {
	f.master, f.ui, f.effective = master, ui, master*ui
	f.setVolumes++
}

// newUISoundGame builds the showcase session on savePath with an injected
// UI cue seam.
func newUISoundGame(t *testing.T, savePath string, u UISound) *Game {
	t.Helper()
	c, err := content.Load(content.FS())
	if err != nil {
		t.Fatalf("content.Load: %v", err)
	}
	wd, ok := c.World("world-showcase")
	if !ok {
		t.Fatal("world-showcase missing")
	}
	g, err := New(Options{Content: c, World: wd, Seed: 42, SavePath: savePath, UISound: u})
	if err != nil {
		t.Fatalf("game.New: %v", err)
	}
	return g
}

// TestDefaultUISoundIsNop is the structural invariant beside
// TestDefaultMusicIsNop: a Game built without Options.UISound (every test,
// simulate, validate) gets the silent nop, so no headless path can decode
// a cue or reach the one audio context.
func TestDefaultUISoundIsNop(t *testing.T) {
	g := newShowcaseGame(t)
	if _, ok := g.ui.(nopUISound); !ok {
		t.Fatalf("default UI sound = %T, want nopUISound", g.ui)
	}
	g.ui.Play(sfxClick) // must not panic or do anything
	g.ui.SetVolume(0.5, 0.5)
}

// pressAt feeds one synthetic press edge at pt through the production
// click gate and reports what the seam received for it.
func pressAt(g *Game, f *fakeUISound, pt image.Point) []string {
	before := len(f.plays)
	g.uiClick(Input{MouseX: float64(pt.X), MouseY: float64(pt.Y), JustClicked: [2]bool{true, false}})
	return f.plays[before:]
}

// TestUIClickLandsOnControlsOnly is the click-cue classification: a press
// speaks only for an interactive control — a button, a text input, a
// slider's grab band, a table row, a context-menu entry. Window bodies,
// title bars, plain labels, a slider's row outside its band, disabled
// buttons, dead space and the world stay silent (the user's "clicking
// anywhere on a window ticks").
func TestUIClickLandsOnControlsOnly(t *testing.T) {
	f := &fakeUISound{}
	g := newUISoundGame(t, t.TempDir()+"/save.json", f)
	pump(t, g, 1280, 720)
	f.plays = nil

	// A fixture window on top of the stack carrying one of each control,
	// laid out by the real layout pass.
	env := g.env
	btn := widget.NewButton(widget.ButtonOpts.TextLabel("Go"))
	off := widget.NewButton(widget.ButtonOpts.TextLabel("Off"))
	off.GetWidget().Disabled = true
	input := widget.NewTextInput(widget.TextInputOpts.WidgetOpts(widget.WidgetOpts.MinSize(theme.Px(120), theme.Px(30))))
	sl := widgets.NewSlider(env, widgets.SliderOpts{Mode: widgets.SliderFreeflow, Min: 0, Max: 100, Step: 1, Value: 50})
	row := widget.NewContainer(widget.ContainerOpts.WidgetOpts(
		widget.WidgetOpts.MinSize(0, theme.Px(30)),
		widget.WidgetOpts.LayoutData(widget.RowLayoutData{Stretch: true}),
	))
	body := widget.NewContainer(
		widget.ContainerOpts.Layout(widget.NewRowLayout(
			widget.RowLayoutOpts.Direction(widget.DirectionVertical),
			widget.RowLayoutOpts.Spacing(theme.Px(8)),
			widget.RowLayoutOpts.Padding(widget.NewInsetsSimple(theme.Px(theme.ContentsPad))),
		)),
	)
	body.AddChild(btn, off, input, sl, row)
	title, _ := theme.NewTitleBar(env, "Fixture", "")
	win := widget.NewWindow(
		widget.WindowOpts.Contents(body),
		widget.WindowOpts.TitleBar(title, theme.Px(theme.TitleBarH)),
		widget.WindowOpts.MinSize(theme.Px(320), theme.Px(320)),
		widget.WindowOpts.Location(image.Rect(theme.Px(140), theme.Px(120), theme.Px(460), theme.Px(440))),
	)
	g.kit.SetWindowClose(win, func() {}) // attaches the × the fixture presses
	g.kit.AddWindow(win)
	pump(t, g, 1280, 720)
	f.plays = nil

	centre := func(r image.Rectangle) image.Point {
		return image.Pt(r.Min.X+r.Dx()/2, r.Min.Y+r.Dy()/2)
	}
	// Loud: the controls.
	for name, pt := range map[string]image.Point{
		"button":       centre(btn.GetWidget().Rect),
		"text input":   centre(input.GetWidget().Rect),
		"slider band":  centre(sl.HitBand()),
		"window close": centre(g.kit.CloseButtonOf(win).GetWidget().Rect),
	} {
		if got := pressAt(g, f, pt); len(got) != 1 || got[0] != sfxClick {
			t.Errorf("press on the %s = %v, want one click", name, got)
		}
	}
	// Silent: everything that is furniture (a bare container is not a row;
	// the real table row below is).
	bodyTop := body.GetWidget().Rect
	band := sl.HitBand()
	silent := map[string]image.Point{
		"window body":  image.Pt(bodyTop.Min.X+2, bodyTop.Max.Y-2),
		"title bar":    image.Pt(title.GetWidget().Rect.Min.X+theme.Px(40), centre(title.GetWidget().Rect).Y),
		"disabled":     centre(off.GetWidget().Rect),
		"slider row":   image.Pt(centre(sl.HitBand()).X, sl.GetWidget().Rect.Max.Y-1),
		"title corner": image.Pt(bodyTop.Max.X-1, bodyTop.Min.Y+1),
	}
	for name, pt := range silent {
		if pt.In(band) {
			t.Fatalf("fixture: %s point %v sits in the slider band", name, pt)
		}
		if got := pressAt(g, f, pt); len(got) != 0 {
			t.Errorf("press on the %s = %v, want silence", name, got)
		}
	}
	// A drag frame (no press edge) never speaks.
	before := len(f.plays)
	g.uiClick(Input{MouseX: float64(centre(btn.GetWidget().Rect).X), MouseY: float64(centre(btn.GetWidget().Rect).Y)})
	if len(f.plays) != before {
		t.Errorf("a held press clicked again: %v", f.plays[before:])
	}
	// The world is silent.
	if world, ok := freePoint(g); ok {
		if got := pressAt(g, f, world); len(got) != 0 {
			t.Errorf("a world press played %v, want silence", got)
		}
	} else {
		t.Error("no point off the interface: cannot prove a world press is silent")
	}
	// A real table row is a control (the row tag, not a fixture).
	if tbl := g.kit.OverviewTable(); tbl != nil && len(tbl.RowsForTest()) > 0 {
		r := tbl.RowWidgetForTest(0).Rect
		if r.Dx() > 0 && r.Dy() > 0 {
			if got := pressAt(g, f, centre(r)); len(got) != 1 || got[0] != sfxClick {
				t.Errorf("press on an overview row = %v, want one click", got)
			}
		}
	} else {
		t.Error("the overview table has no rows to press")
	}
	// A context-menu entry is a control (the popup is drawn, not a widget).
	g.Do(widgets.OpenMenu{
		At:      image.Pt(theme.Px(600), theme.Px(300)),
		Entries: []widgets.MenuEntry{{Label: "Show Info", Action: func() {}}},
	})
	pump(t, g, 1280, 720)
	if r, open := g.kit.MenuRect(); open {
		if got := pressAt(g, f, image.Pt(r.Min.X+r.Dx()/2, r.Min.Y+6)); len(got) != 1 || got[0] != sfxClick {
			t.Errorf("press on a menu entry = %v, want one click", got)
		}
	} else {
		t.Error("the context menu did not open")
	}
}

// TestUISoundTriggers pins the non-click triggers against the fake seam:
// host mounts/unmounts open and close, the quantity intents confirm, and
// the keybind refusals refuse.
func TestUISoundTriggers(t *testing.T) {
	g := newUISoundGame(t, t.TempDir()+"/save.json", &fakeUISound{})
	f := g.ui.(*fakeUISound)
	pump(t, g, 1280, 720)
	if len(f.plays) != 0 {
		t.Fatalf("the boot composition played %v, want silence", f.plays)
	}

	// Mount and unmount edges through the host (Reconcile owns them).
	g.host.SetOpen(idScanner, true)
	g.syncMounts()
	if len(f.plays) != 1 || f.plays[0] != sfxOpen {
		t.Fatalf("mount = %v, want one open", f.plays)
	}
	g.host.SetOpen(idScanner, false)
	g.syncMounts()
	if len(f.plays) != 2 || f.plays[1] != sfxClose {
		t.Fatalf("unmount = %v, want a following close", f.plays)
	}
	// A frame with no edge in either direction stays silent.
	g.syncMounts()
	if len(f.plays) != 2 {
		t.Errorf("an unchanged room replayed cues: %v", f.plays)
	}

	// The quantity verbs confirm on the press that lands them.
	f.plays = nil
	g.Do(widgets.Refine{Ore: "ore-cordite", Qty: 10})
	if len(f.plays) != 1 || f.plays[0] != sfxConfirm {
		t.Errorf("Refine intent = %v, want one confirm", f.plays)
	}
	f.plays = nil
	g.Do(widgets.Buy{Item: "ore-cordite", Qty: 1})
	if len(f.plays) != 1 || f.plays[0] != sfxConfirm {
		t.Errorf("Buy intent = %v, want one confirm", f.plays)
	}

	// The keybind refusals refuse; an assignment confirms.
	f.plays = nil
	g.beginCapture("settings")
	if !g.captureKey(ebiten.KeyF9, false, false) { // a fixed chord
		t.Fatal("captureKey did not consume the fixed chord")
	}
	if g.keyNotice == "" {
		t.Fatal("fixed chord left no refusal notice")
	}
	if len(f.plays) != 1 || f.plays[0] != sfxRefuse {
		t.Errorf("fixed chord = %v, want one refuse", f.plays)
	}
	f.plays = nil
	g.captureKey(ebiten.KeyJ, false, false) // a free key assigns
	if len(f.plays) != 1 || f.plays[0] != sfxConfirm {
		t.Errorf("assignment = %v, want one confirm", f.plays)
	}
}

// TestUISoundVolume pins the channel math: master × UI at the seam, live
// on every change, and the 80 % fresh-install default round-tripping
// through the sidecar (absent key included) and back on reset-all.
func TestUISoundVolume(t *testing.T) {
	path := t.TempDir() + "/save.json"
	f := &fakeUISound{}
	g := newUISoundGame(t, path, f)
	if g.uiVol != defaultUIVolume {
		t.Fatalf("UI volume default = %d, want %d (80)", g.uiVol, defaultUIVolume)
	}
	if f.master != 1 || f.ui != 0.8 || f.effective != 0.8 {
		t.Fatalf("seam after boot = (%v, %v) = %v, want (1, 0.8) = 0.8", f.master, f.ui, f.effective)
	}
	g.setMasterVolume(50)
	if f.effective != 0.40 {
		t.Errorf("master 50 × UI 80 = %v, want 0.40", f.effective)
	}
	g.setUIVolume(120)
	if g.uiVol != 100 || f.effective != 0.5 {
		t.Errorf("UI ceiling = %d / %v, want 100 / 0.5", g.uiVol, f.effective)
	}
	g.setUIVolume(-5)
	if g.uiVol != 0 || f.effective != 0 {
		t.Errorf("UI floor = %d / %v, want 0 / 0", g.uiVol, f.effective)
	}

	// The real seam's own math (no player, no context needed).
	s := &realUISound{master: 0.5, uiVol: 0.8}
	if got := s.effective(); got != 0.4 {
		t.Errorf("realUISound.effective = %v, want 0.4 (master 0.5 × UI 0.8)", got)
	}

	// Round trip, and an older file without the key boots 80.
	g.masterVol, g.uiVol = 60, 30
	if err := g.writeSettings(); err != nil {
		t.Fatalf("writeSettings: %v", err)
	}
	f2 := &fakeUISound{}
	g2 := newUISoundGame(t, path, f2)
	if g2.uiVol != 30 || f2.master != 0.6 || f2.ui != 0.3 || f2.effective != 0.18 {
		t.Errorf("round trip = %d / (%v, %v) = %v, want 30 / (0.6, 0.3) = 0.18",
			g2.uiVol, f2.master, f2.ui, f2.effective)
	}
	if err := os.WriteFile(path+".settings.json", []byte(`{"ui_scale":1,"fullscreen":false}`), 0o644); err != nil {
		t.Fatal(err)
	}
	g2.uiVol = 7
	g2.loadSettings()
	if g2.uiVol != 80 {
		t.Errorf("pre-UI sidecar = %d, want the 80 default", g2.uiVol)
	}

	// Reset-all restores 80 and re-applies it at the seam.
	g2.uiVol = 15
	g2.resetAllSettings()
	if g2.uiVol != defaultUIVolume || f2.effective != 0.8 {
		t.Errorf("after reset = %d / %v, want 80 / 0.8", g2.uiVol, f2.effective)
	}
	if f2.plays[len(f2.plays)-1] != sfxConfirm {
		t.Errorf("reset-all fired %v, want the confirm cue last", f2.plays)
	}
}

// TestUISoundEmbedPin pins the embedded cues: five wavs, non-empty, and
// each one decodes to PCM through ebiten's wav reader alone — no device,
// no player, so this runs headless.
func TestUISoundEmbedPin(t *testing.T) {
	entries, err := sfxassets.FS.ReadDir(".")
	if err != nil {
		t.Fatalf("read embedded cues: %v", err)
	}
	var names []string
	for _, e := range entries {
		names = append(names, e.Name())
	}
	sort.Strings(names)
	want := []string{"click.wav", "close.wav", "confirm.wav", "open.wav", "refuse.wav"}
	if len(names) != len(want) {
		t.Fatalf("embedded cues = %v, want %v", names, want)
	}
	for i, name := range want {
		if names[i] != name {
			t.Fatalf("embedded cues = %v, want %v", names, want)
		}
		data, err := sfxassets.FS.ReadFile(name)
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if len(data) == 0 {
			t.Fatalf("%s is empty", name)
		}
		stream, err := wav.DecodeF32(bytes.NewReader(data))
		if err != nil {
			t.Fatalf("%s does not decode: %v", name, err)
		}
		if stream.SampleRate() != musicSampleRate || stream.Length() <= 0 {
			t.Errorf("%s = %d Hz / %d bytes, want %d Hz PCM", name, stream.SampleRate(), stream.Length(), musicSampleRate)
		}
		pcm, err := io.ReadAll(stream)
		if err != nil || len(pcm) == 0 {
			t.Errorf("%s PCM = %d bytes (%v), want audio", name, len(pcm), err)
		}
	}
}

// freePoint returns a screen point no interface chrome covers (the world
// press a UI click must not be confused with), or false when the whole
// screen is chrome.
func freePoint(g *Game) (image.Point, bool) {
	for y := 20; y < g.h; y += 40 {
		for x := 20; x < g.w; x += 40 {
			if !g.overUI(float64(x), float64(y)) {
				return image.Pt(x, y), true
			}
		}
	}
	return image.Point{}, false
}

// cueMinPeakDB is the floor a cue's own peak must clear: the seam plays at
// master × UI (0.8 fresh), so a file ~30 dB under its siblings is silent in
// practice however correctly it fires.
const cueMinPeakDB = -24.0

// TestUISoundCueLevels pins that every embedded cue is actually audible.
// It is the pin for the reported "the open/close sounds do not play": the
// triggers fired, but the two Air FX files peaked at -44 / -41.6 dBFS —
// ~30 dB below the click — so nothing could be heard.
func TestUISoundCueLevels(t *testing.T) {
	for _, name := range []string{"click", "open", "close", "confirm", "refuse"} {
		data, err := sfxassets.FS.ReadFile(name + ".wav")
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		stream, err := wav.DecodeF32(bytes.NewReader(data))
		if err != nil {
			t.Fatalf("%s decode: %v", name, err)
		}
		pcm, err := io.ReadAll(stream)
		if err != nil {
			t.Fatalf("%s pcm: %v", name, err)
		}
		var peak float64
		for i := 0; i+3 < len(pcm); i += 4 {
			v := math.Abs(float64(math.Float32frombits(binary.LittleEndian.Uint32(pcm[i:]))))
			peak = max(peak, v)
		}
		if peak == 0 {
			t.Fatalf("%s is digital silence", name)
		}
		if db := 20 * math.Log10(peak); db < cueMinPeakDB {
			t.Errorf("%s peaks at %.1f dBFS, want ≥ %.0f (inaudible at the seam's volume)", name, db, cueMinPeakDB)
		}
	}
}
