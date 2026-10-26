package game

import (
	"bytes"
	"image"
	"io"
	"os"
	"sort"
	"testing"

	sfxassets "github.com/entieve/entieve/assets/sfx"
	"github.com/entieve/entieve/content"
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

// TestUISoundTriggers pins every approved trigger against the fake seam:
// a UI press edge clicks (once per press), a world press is silent, host
// mounts/unmounts open and close, the quantity intents confirm, and the
// keybind refusals refuse.
func TestUISoundTriggers(t *testing.T) {
	path := t.TempDir() + "/save.json"
	f := &fakeUISound{}
	g := newUISoundGame(t, path, f)
	pump(t, g, 1280, 720)
	f.plays = nil // ignore the boot mounts (the log opens by default)

	// Every press edge over interface chrome clicks; a drag frame (no
	// edge) does not, and a world press never does.
	log := g.host.Window(idLog).GetContainer().GetWidget().Rect
	at := image.Pt(log.Min.X+log.Dx()/2, log.Min.Y+log.Dy()/2)
	g.uiClick(Input{MouseX: float64(at.X), MouseY: float64(at.Y), JustClicked: [2]bool{true, false}})
	if len(f.plays) != 1 || f.plays[0] != sfxClick {
		t.Fatalf("press over the log = %v, want one click", f.plays)
	}
	// A right press on chrome is a press too (the context menu's edge).
	g.uiClick(Input{MouseX: float64(at.X), MouseY: float64(at.Y), JustClicked: [2]bool{false, true}})
	if len(f.plays) != 2 || f.plays[1] != sfxClick {
		t.Fatalf("right press over the log = %v, want a second click", f.plays)
	}
	f.plays = f.plays[:1]
	g.uiClick(Input{MouseX: float64(at.X), MouseY: float64(at.Y)}) // held, no edge
	if len(f.plays) != 1 {
		t.Errorf("a held press clicked again: %v", f.plays)
	}
	if world, ok := freePoint(g); ok {
		g.uiClick(Input{MouseX: float64(world.X), MouseY: float64(world.Y), JustClicked: [2]bool{true, false}})
		if len(f.plays) != 1 {
			t.Errorf("a world press played %v, want silence", f.plays[1:])
		}
	} else {
		t.Error("no point off the interface: cannot prove a world press is silent")
	}

	// Mount and unmount edges through the host (Reconcile owns them).
	g.host.SetOpen(idScanner, true)
	g.syncMounts()
	if len(f.plays) != 2 || f.plays[1] != sfxOpen {
		t.Fatalf("mount = %v, want a following open", f.plays)
	}
	g.host.SetOpen(idScanner, false)
	g.syncMounts()
	if len(f.plays) != 3 || f.plays[2] != sfxClose {
		t.Fatalf("unmount = %v, want a following close", f.plays)
	}
	// A frame with no edge in either direction stays silent.
	g.syncMounts()
	if len(f.plays) != 3 {
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
