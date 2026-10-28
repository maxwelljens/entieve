package game

import (
	"bytes"
	"image"
	"io"
	"strings"

	sfxassets "github.com/entieve/entieve/assets/sfx"
	"github.com/hajimehoshi/ebiten/v2/audio"
	"github.com/hajimehoshi/ebiten/v2/audio/wav"
)

// UISound is the UI sound seam: five fixed cues, played by name, with the
// master × UI volume applied inside the seam. New installs the silent nop
// when Options.UISound is nil — every headless path (tests, simulate,
// validate) — and only cmdRun's NewAudio builds the real engine, on the
// same process-wide audio context as the music (see audioCtx).
type UISound interface {
	// Play fires one cue ("click", "open", "close", "confirm", "refuse").
	Play(name string)
	// SetVolume takes the two user percentages (0..1 each); the seam
	// applies master × ui on every fire.
	SetVolume(master, ui float64)
}

// The five cue names, one per embedded assets/sfx/<name>.wav.
const (
	sfxClick   = "click"
	sfxOpen    = "open"
	sfxClose   = "close"
	sfxConfirm = "confirm"
	sfxRefuse  = "refuse"
)

// nopUISound is the silent UISound: the default for every Game built
// without Options.UISound (all headless paths, so no test can decode or
// play a cue).
type nopUISound struct{}

func (nopUISound) Play(string)                  {}
func (nopUISound) SetVolume(master, ui float64) {}

// newUISounds decodes the five embedded cues once, on the process-wide
// audio context (audioCtx: NewContext may run only once per process).
// NewAudio is its only caller, so decoding never happens on a headless
// path; a cue that fails to decode is warned about and stays silent.
func newUISounds(warn func(error)) UISound {
	s := &realUISound{warn: warn, master: 1, uiVol: 1, players: map[string]*audio.Player{}}
	ctx := audioCtx()
	entries, err := sfxassets.FS.ReadDir(".")
	if err != nil {
		warn(err)
		return s
	}
	for _, e := range entries { // ReadDir is sorted by name
		name := strings.TrimSuffix(e.Name(), ".wav")
		p, err := uiPlayer(ctx, e.Name())
		if err != nil {
			warn(err)
			continue
		}
		s.players[name] = p
	}
	return s
}

// uiPlayer decodes one embedded wav to PCM bytes once and wraps them in a
// player. Decoding needs no device, but building the player does — this is
// run-path-only code. A differing sample rate is resampled to the
// context's (the shipped cues are already 44100).
func uiPlayer(ctx *audio.Context, name string) (*audio.Player, error) {
	data, err := sfxassets.FS.ReadFile(name)
	if err != nil {
		return nil, err
	}
	stream, err := wav.DecodeF32(bytes.NewReader(data))
	if err != nil {
		return nil, err
	}
	var src io.Reader = stream
	if stream.SampleRate() != musicSampleRate {
		src = audio.ResampleReaderF32(stream, stream.Length(), stream.SampleRate(), musicSampleRate)
	}
	pcm, err := io.ReadAll(src)
	if err != nil {
		return nil, err
	}
	return ctx.NewPlayerF32FromBytes(pcm), nil
}

// realUISound plays the embedded cues: one in-memory player per cue,
// re-triggered by Rewind+Play, so a click cuts its own tail and distinct
// cues overlap.
type realUISound struct {
	players       map[string]*audio.Player
	warn          func(error)
	master, uiVol float64
}

func (s *realUISound) Play(name string) {
	p := s.players[name]
	if p == nil {
		return
	}
	p.SetVolume(s.effective()) // re-applied on every fire
	if err := p.Rewind(); err != nil {
		s.warn(err)
	}
	p.Play()
}

func (s *realUISound) SetVolume(master, ui float64) {
	s.master, s.uiVol = clamp01(master), clamp01(ui)
	for _, p := range s.players {
		p.SetVolume(s.effective())
	}
}

// effective is the seam's volume: the product of the two user settings.
func (s *realUISound) effective() float64 { return s.master * s.uiVol }

// uiClick fires the click cue for a mouse press edge (either button) that
// landed on an interactive control — a button, a text input, a slider's
// band, a table row or a context-menu entry (Kit.PressTarget is the kit's
// one hit test; the rows are registered per window, the entries from the
// popup's own bands). Window bodies, title bars, labels and dead space are
// silent, and so is every world press — including the press that only
// dismisses the popup (Popup.Control is true for entries only). Split out
// of Update so a test can drive one synthetic press without an ebiten
// input backend.
func (g *Game) uiClick(in Input) {
	if !(in.JustClicked[0] || in.JustClicked[1]) {
		return
	}
	pt := image.Pt(int(in.MouseX), int(in.MouseY))
	if g.kit.PressTarget(pt) {
		g.ui.Play(sfxClick)
	}
}
