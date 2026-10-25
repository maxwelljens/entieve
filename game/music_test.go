package game

import (
	"bytes"
	"io/fs"
	"os"
	"path/filepath"
	"testing"

	audioassets "github.com/entieve/entieve/assets/audio"
	"github.com/entieve/entieve/content"
	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/audio/vorbis"
)

// fakeMusic is the injected Music the game tests pin the wiring with. It
// records intents only: no audio context is ever created in a test (the
// structural invariant: New installs nopMusic unless a test injects a
// fake, and the real engine is constructed only in cmdRun).
type fakeMusic struct {
	started, stopped, ticks int
	master, music           float64
	effective               float64
	shuffle                 bool
	current                 string
}

func (f *fakeMusic) Start() { f.started++ }
func (f *fakeMusic) Stop()  { f.stopped++ }
func (f *fakeMusic) Tick()  { f.ticks++ }
func (f *fakeMusic) SetVolume(master, music float64) {
	f.master, f.music = master, music
	f.effective = master * music
}
func (f *fakeMusic) SetShuffle(on bool) { f.shuffle = on }
func (f *fakeMusic) Current() string    { return f.current }

// newMusicGameAt builds the showcase session on savePath with an injected
// music seam (the helper above New's nil default).
func newMusicGameAt(t *testing.T, savePath string, m Music) *Game {
	t.Helper()
	c, err := content.Load(content.FS())
	if err != nil {
		t.Fatalf("content.Load: %v", err)
	}
	wd, ok := c.World("world-showcase")
	if !ok {
		t.Fatal("world-showcase missing")
	}
	g, err := New(Options{Content: c, World: wd, Seed: 42, SavePath: savePath, Music: m})
	if err != nil {
		t.Fatalf("game.New: %v", err)
	}
	return g
}

// TestMusicDefaultsAndPersistence pins the fresh-install audio trio, the
// seam's effective volume (master × music), the sidecar round-trip and
// the absent-key defaults of an older file.
func TestMusicDefaultsAndPersistence(t *testing.T) {
	path := filepath.Join(t.TempDir(), "save.json")
	fm := &fakeMusic{}
	g := newMusicGameAt(t, path, fm)

	if g.masterVol != 100 || g.musicVol != 70 || !g.shuffle {
		t.Fatalf("fresh audio settings = (%d, %d, shuffle %v), want (100, 70, true)",
			g.masterVol, g.musicVol, g.shuffle)
	}
	if fm.started != 1 {
		t.Errorf("music starts = %d, want 1", fm.started)
	}
	if fm.master != 1 || fm.music != 0.7 || fm.shuffle != true {
		t.Errorf("seam after boot = (%v, %v, shuffle %v), want (1, 0.7, true)", fm.master, fm.music, fm.shuffle)
	}
	if fm.effective != 0.7 {
		t.Errorf("effective volume = %v, want 0.7 (1 × 0.7)", fm.effective)
	}

	// The live absolute set: master 50%; 0.5 × 0.7 = 0.35 reaches the
	// seam as the effective volume (and writes nothing yet).
	g.setMasterVolume(50)
	if g.masterVol != 50 || fm.effective != 0.35 {
		t.Errorf("after -50%% master: setting %d, effective %v; want 50 and 0.35", g.masterVol, fm.effective)
	}
	// Clamped at both ends (no wrap, no negative volume).
	g.setMusicVolume(-20)
	if g.musicVol != 0 || fm.effective != 0 {
		t.Errorf("music floor = %d / %v, want 0/0", g.musicVol, fm.effective)
	}
	g.setMusicVolume(120)
	if g.musicVol != 100 {
		t.Errorf("music ceiling = %d, want 100", g.musicVol)
	}
	g.setShuffle(false)
	if fm.shuffle {
		t.Error("setShuffle(false) did not reach the seam")
	}

	// Explicit values round-trip through a second session.
	g.masterVol, g.musicVol, g.shuffle = 60, 30, false
	if err := g.writeSettings(); err != nil {
		t.Fatalf("writeSettings: %v", err)
	}
	fm2 := &fakeMusic{}
	g2 := newMusicGameAt(t, path, fm2)
	if g2.masterVol != 60 || g2.musicVol != 30 || g2.shuffle {
		t.Errorf("round trip = (%d, %d, shuffle %v), want (60, 30, false)", g2.masterVol, g2.musicVol, g2.shuffle)
	}
	if fm2.master != 0.6 || fm2.music != 0.3 || fm2.shuffle {
		t.Errorf("seam after restore = (%v, %v, shuffle %v), want (0.6, 0.3, false)", fm2.master, fm2.music, fm2.shuffle)
	}

	// A pre-audio sidecar (no keys) boots the documented defaults.
	if err := os.WriteFile(path+".settings.json", []byte(`{"ui_scale":1,"fullscreen":false}`), 0o644); err != nil {
		t.Fatal(err)
	}
	g2.masterVol, g2.musicVol, g2.shuffle = 0, 0, false
	g2.loadSettings()
	if g2.masterVol != 100 || g2.musicVol != 70 || !g2.shuffle {
		t.Errorf("old file = (%d, %d, shuffle %v), want the 100/70/true defaults", g2.masterVol, g2.musicVol, g2.shuffle)
	}

	// Out-of-range percents clamp rather than corrupt the volume.
	if err := os.WriteFile(path+".settings.json", []byte(`{"ui_scale":1,"master_volume":150,"music_volume":-5}`), 0o644); err != nil {
		t.Fatal(err)
	}
	g2.loadSettings()
	if g2.masterVol != 100 || g2.musicVol != 0 {
		t.Errorf("out-of-range percents = (%d, %d), want (100, 0)", g2.masterVol, g2.musicVol)
	}
}

// TestMusicTickMainline pins that Update ticks the seam once per frame in
// BOTH phases: title and in-game (music is not a title special case).
func TestMusicTickMainline(t *testing.T) {
	fm := &fakeMusic{}
	g := newMusicGameAt(t, filepath.Join(t.TempDir(), "save.json"), fm)
	g.applySize(1280, 720)
	if err := g.Update(); err != nil {
		t.Fatalf("in-game Update: %v", err)
	}
	if fm.ticks != 1 {
		t.Errorf("in-game ticks = %d, want 1", fm.ticks)
	}

	g.title = true
	if err := g.Update(); err != nil {
		t.Fatalf("title Update: %v", err)
	}
	if fm.ticks != 2 {
		t.Errorf("title ticks = %d, want 2 (the mainline tick runs at the title too)", fm.ticks)
	}

	// A clean quit stops playback (the device closes with the process).
	g.quit = true
	if err := g.Update(); err != ebiten.Termination {
		t.Fatalf("quit Update = %v, want Termination", err)
	}
	if fm.stopped != 1 {
		t.Errorf("music stops on quit = %d, want 1", fm.stopped)
	}
}

// TestMusicResetAllRestoresTrio: reset-all restores master 100, music 70
// and shuffle on, live at the seam and in the sidecar.
func TestMusicResetAllRestoresTrio(t *testing.T) {
	resetScale(t)
	path := filepath.Join(t.TempDir(), "save.json")
	fm := &fakeMusic{}
	g := newMusicGameAt(t, path, fm)
	g.applySize(1280, 720)
	g.setMasterVolume(60)
	g.setMusicVolume(40)
	g.setShuffle(false)
	if fm.effective == 0.7 || fm.shuffle {
		t.Fatalf("fixture: audio settings not applied (%v, shuffle %v)", fm.effective, fm.shuffle)
	}

	g.resetAllSettings()
	if g.masterVol != 100 || g.musicVol != 70 || !g.shuffle {
		t.Errorf("after reset = (%d, %d, shuffle %v), want (100, 70, true)", g.masterVol, g.musicVol, g.shuffle)
	}
	if fm.master != 1 || fm.music != 0.7 || !fm.shuffle {
		t.Errorf("seam after reset = (%v, %v, shuffle %v), want (1, 0.7, true)", fm.master, fm.music, fm.shuffle)
	}
	raw := readSettingsMap(t, g.settingsPath())
	if raw["master_volume"] != float64(100) || raw["music_volume"] != float64(70) || raw["shuffle_off"] != nil {
		t.Errorf("sidecar after reset = %v, want master 100, music 70, shuffle key gone", raw)
	}
}

// TestDefaultMusicIsNop is the structural invariant: a Game built without
// Options.Music (every test, simulate, validate) gets the silent nop, so
// no headless path can ever create the one audio context.
func TestDefaultMusicIsNop(t *testing.T) {
	g := newShowcaseGame(t)
	if _, ok := g.music.(nopMusic); !ok {
		t.Fatalf("default music = %T, want nopMusic", g.music)
	}
	g.music.Tick() // must not panic or do anything
	if g.music.Current() != "" {
		t.Errorf("nop Current = %q, want empty", g.music.Current())
	}
}

// TestTrackQueueOrder pins the pure ordering: a shuffled pool deals
// permutations of all six, never repeats back to back (also across a
// wrap), and shuffle-off walks the declared order.
func TestTrackQueueOrder(t *testing.T) {
	q := newTrackQueue(6, true, 7)
	var got []int
	for range 6 {
		got = append(got, q.next())
	}
	seen := map[int]bool{}
	for _, i := range got {
		if i < 0 || i >= 6 || seen[i] {
			t.Fatalf("first cycle = %v, want a permutation of 0..5", got)
		}
		seen[i] = true
	}
	prev := got[len(got)-1]
	for range 30 {
		i := q.next()
		if i == prev {
			t.Fatalf("immediate repeat of track %d in a shuffled pool", i)
		}
		prev = i
	}

	// Same seed, same order (the cmd path seeds from the clock).
	q2 := newTrackQueue(6, true, 7)
	for n, want := range got {
		if got := q2.next(); got != want {
			t.Fatalf("seeded order diverges at %d: %d vs %d", n, got, want)
		}
	}

	// Shuffle off = declared order, wrapping.
	off := newTrackQueue(6, false, 1)
	for n, want := range []int{0, 1, 2, 3, 4, 5, 0, 1} {
		if got := off.next(); got != want {
			t.Fatalf("declared order at %d = %d, want %d", n, got, want)
		}
	}
	// Toggling shuffle on mid-stream starts a fresh cycle that does not
	// repeat the track that just played.
	off.setShuffle(true)
	if got := off.next(); got == 1 {
		t.Error("shuffle-on repeated the last declared track")
	}
}

// TestEmbeddedTracks pins the embed shim: exactly the six shipped .ogg
// files, non-empty, all decodable as 44.1 kHz stereo (the context rate,
// so no resampling happens for the shipped pool).
func TestEmbeddedTracks(t *testing.T) {
	names, err := fs.Glob(audioassets.FS, "*.ogg")
	if err != nil {
		t.Fatal(err)
	}
	if len(names) != 6 {
		t.Fatalf("embedded tracks = %v (%d), want the six .ogg files", names, len(names))
	}
	for _, name := range names {
		data, err := audioassets.FS.ReadFile(name)
		if err != nil || len(data) == 0 {
			t.Errorf("%s: read %d bytes, err %v", name, len(data), err)
			continue
		}
		s, err := vorbis.DecodeF32(bytes.NewReader(data))
		if err != nil {
			t.Errorf("%s: decode: %v", name, err)
			continue
		}
		if s.SampleRate() != musicSampleRate {
			t.Errorf("%s: sample rate %d, want the context rate %d", name, s.SampleRate(), musicSampleRate)
		}
	}
}

// TestRealMusicVolumeMath pins the real seam's effective volume without an
// audio context (no player exists, so SetVolume only stores/clamps): the
// value play() hands to the player on every track start is master × music.
func TestRealMusicVolumeMath(t *testing.T) {
	m := &realMusic{master: 1, musicVol: 1}
	m.SetVolume(0.5, 0.7)
	if got := m.effective(); got != 0.35 {
		t.Errorf("effective volume = %v, want 0.35 (0.5 × 0.7)", got)
	}
	m.SetVolume(2, -3)
	if m.master != 1 || m.musicVol != 0 || m.effective() != 0 {
		t.Errorf("clamped volume = (%v, %v, %v), want (1, 0, 0)", m.master, m.musicVol, m.effective())
	}
}
