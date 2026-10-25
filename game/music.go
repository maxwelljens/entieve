package game

import (
	"bytes"
	"io"
	"math"
	"math/rand/v2"
	"strings"
	"sync"

	audioassets "github.com/entieve/entieve/assets/audio"
	"github.com/hajimehoshi/ebiten/v2/audio"
	"github.com/hajimehoshi/ebiten/v2/audio/vorbis"
)

// Music is the audio seam: the game ticks it once per frame, the settings
// rows drive it, and nothing else in the game reads it. New installs the
// silent nop when Options.Music is nil, which is every headless path
// (tests, simulate, validate); the real engine is built only by cmdRun.
// Keeping construction injected is what guarantees no test ever reaches
// audio.NewContext, which panics on a second call and must never run in a
// windowless process.
type Music interface {
	// Start begins the first track (idempotent).
	Start()
	// Stop silences playback (the device is left open).
	Stop()
	// Tick advances to the next track when the current one ended.
	Tick()
	// SetVolume takes the two user percentages (0..1 each); the seam
	// applies master × music to the output.
	SetVolume(master, music float64)
	// SetShuffle switches between the shuffled pool and declared order.
	SetShuffle(on bool)
	// Current names the playing track (no extension; "" = silent). For
	// tests and debug only — never logged into the game's event log.
	Current() string
}

// musicSampleRate is the one audio context rate (44100, the rate all six
// embedded tracks carry; a mismatched stream is resampled at the seam).
const musicSampleRate = 44100

// nopMusic is the silent Music: the default for every Game built without
// Options.Music, and the fake the test suite pins the game's wiring with.
type nopMusic struct{}

func (nopMusic) Start()                          {}
func (nopMusic) Stop()                           {}
func (nopMusic) Tick()                           {}
func (nopMusic) SetVolume(master, music float64) {}
func (nopMusic) SetShuffle(on bool)              {}
func (nopMusic) Current() string                 { return "" }

// musicContext is the process-wide ebiten audio context: NewContext may
// be called at most once (a second call panics), and the device itself
// opens in ebiten's before-update hook on the first game-loop update —
// never at construction.
var (
	musicContextOnce sync.Once
	musicContext     *audio.Context
)

// NewMusic builds the real engine over the embedded tracks: shuffled
// pool, one player at a time, seeded by seed (cmdRun passes the clock;
// tests that pin the ordering drive trackQueue directly). warn receives
// decode failures (cmdRun prints them to stderr); a failing track is
// skipped, and all six failing leaves the game silent, never panicked.
func NewMusic(seed int64, warn func(error)) Music {
	if warn == nil {
		warn = func(error) {}
	}
	m := &realMusic{warn: warn, master: 1, musicVol: 1}
	entries, err := audioassets.FS.ReadDir(".")
	if err != nil {
		warn(err)
		m.silent = true
		return m
	}
	for _, e := range entries { // ReadDir is sorted by name
		if !e.IsDir() && strings.HasSuffix(e.Name(), ".ogg") {
			m.tracks = append(m.tracks, e.Name())
		}
	}
	m.queue = newTrackQueue(len(m.tracks), true, seed)
	musicContextOnce.Do(func() { musicContext = audio.NewContext(musicSampleRate) })
	m.ctx = musicContext
	return m
}

// realMusic streams the embedded pool: one vorbis decode and one player
// per track, retired with PauseAndStopReading when the next starts.
type realMusic struct {
	ctx    *audio.Context
	tracks []string
	queue  *trackQueue
	player *audio.Player
	warn   func(error)

	master, musicVol float64
	started          bool
	silent           bool // every track failed to decode: stay silent
}

func (m *realMusic) Start() {
	if m.started || m.silent {
		return
	}
	m.started = true
	m.playNext()
}

func (m *realMusic) Stop() {
	m.started = false
	if m.player != nil {
		m.player.PauseAndStopReading()
		m.player = nil
	}
}

func (m *realMusic) Tick() {
	if !m.started || m.silent {
		return
	}
	if m.player == nil || !m.player.IsPlaying() {
		m.playNext()
	}
}

func (m *realMusic) SetVolume(master, music float64) {
	m.master, m.musicVol = clamp01(master), clamp01(music)
	if m.player != nil {
		m.player.SetVolume(m.effective())
	}
}

func (m *realMusic) SetShuffle(on bool) {
	if m.queue != nil {
		m.queue.setShuffle(on)
	}
}

func (m *realMusic) Current() string {
	if m.silent || !m.started || m.queue == nil || m.queue.last < 0 {
		return ""
	}
	return strings.TrimSuffix(m.tracks[m.queue.last], ".ogg")
}

// effective is the seam's volume: the product of the two user settings.
func (m *realMusic) effective() float64 { return m.master * m.musicVol }

// playNext retires the finished player and starts the queue's next track,
// skipping (and warning about) undecodable ones. A full pass of failures
// latches silence so a broken pool is never retried every tick.
func (m *realMusic) playNext() {
	if len(m.tracks) == 0 {
		m.silent = true
		return
	}
	for range m.tracks {
		idx := m.queue.next()
		if idx < 0 {
			break
		}
		if err := m.play(idx); err != nil {
			m.warn(err)
			continue
		}
		return
	}
	m.silent = true
}

// play decodes one embedded track and starts it, applying the current
// volume. The stream is decoded fresh per track, so a sample-rate
// mismatch can be resampled (the six shipped tracks are all 44100 Hz).
func (m *realMusic) play(i int) error {
	data, err := audioassets.FS.ReadFile(m.tracks[i])
	if err != nil {
		return err
	}
	stream, err := vorbis.DecodeF32(bytes.NewReader(data))
	if err != nil {
		return err
	}
	var src io.Reader = stream
	if stream.SampleRate() != musicSampleRate {
		src = audio.ResampleReaderF32(stream, stream.Length(), stream.SampleRate(), musicSampleRate)
	}
	p, err := m.ctx.NewPlayerF32(src)
	if err != nil {
		return err
	}
	p.SetVolume(m.effective())
	p.Play()
	if m.player != nil {
		m.player.PauseAndStopReading()
	}
	m.player = p
	return nil
}

// clamp01 pins a volume to the documented 0..1 range.
func clamp01(v float64) float64 { return math.Max(0, math.Min(1, v)) }

// trackQueue is the pure ordering half of the engine (no audio, so tests
// drive it): declared order when shuffle is off, otherwise consecutive
// permutations of all tracks with no immediate repeat across a wrap.
type trackQueue struct {
	n       int
	shuffle bool
	rng     *rand.Rand
	pending []int
	last    int
}

// newTrackQueue builds an ordering over n tracks; seed makes it
// reproducible (cmdRun seeds from the clock).
func newTrackQueue(n int, shuffle bool, seed int64) *trackQueue {
	return &trackQueue{
		n:       n,
		shuffle: shuffle,
		rng:     rand.New(rand.NewPCG(uint64(seed), 0x9e3779b97f4a7c15)),
		last:    -1,
	}
}

// next returns the next track index. Shuffle-off walks the declared
// order and wraps; shuffle-on deals fresh permutations, never starting
// one with the track that just played.
func (q *trackQueue) next() int {
	if q.n == 0 {
		return -1
	}
	if !q.shuffle {
		q.last = (q.last + 1) % q.n
		return q.last
	}
	if len(q.pending) == 0 {
		q.pending = q.rng.Perm(q.n)
	}
	if q.pending[0] == q.last && len(q.pending) > 1 {
		q.pending[0], q.pending[1] = q.pending[1], q.pending[0]
	}
	q.last = q.pending[0]
	q.pending = q.pending[1:]
	return q.last
}

// setShuffle switches modes; a fresh shuffled cycle starts on the next
// pick so the current track is not immediately repeated.
func (q *trackQueue) setShuffle(on bool) {
	q.shuffle = on
	q.pending = nil
}
