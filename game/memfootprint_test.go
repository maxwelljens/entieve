package game

import (
	"fmt"
	"runtime"
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
)

// TestMemoryFootprintPump is the memory regression guard: it drives the
// production pipeline (Game.Update / Game.Draw against a 1280×720
// offscreen image, no window) for 6,000 frames and asserts the live heap
// plateaus — growth between the 3,000- and 6,000-frame marks stays under
// 10 MB. A per-frame leak (sync churn retained, a growing cache, an
// unbounded handler/row/tooltip slice) shows up as linear growth here.
//
// Draw runs at frame 1 and at every 1,000-frame checkpoint rather than
// every frame: offscreen, ebiten is outside its frame loop, so every
// draw call lands on the atlas deferred-command queue
// (internal/atlas image.go: "inFrame ... function calls on an image
// should be deferred until the next BeginFrame"), which only a real
// windowed frame drains. Drawing all 6,000 frames would grow HeapInuse
// ~0.8 MB per frame from that engine-side backlog — a headless artifact,
// not an Entieve leak — and would bury the signal this test watches.
// The Update path (everything the churn fixes touch) runs every frame.
func TestMemoryFootprintPump(t *testing.T) {
	const (
		frames     = 6000
		markFrames = 1000
		plateauAt  = 3000
		// 10 MB covers the ~0.8 MB per checkpoint draw (3 in the
		// measured window) plus Update-side noise with margin; a real
		// leak grows tens of MB over 3,000 frames.
		maxGrowth = 10 << 20
	)

	g := newShowcaseGame(t)
	g.applySize(1280, 720)

	// Defensive: an environment that cannot create an offscreen image
	// (a finished ebiten loop, no graphics device) skips instead of
	// panicking — the guard must never take the suite down with it.
	img, err := newOffscreen(1280, 720)
	if err != nil {
		t.Skipf("no offscreen image available: %v", err)
	}

	var (
		records   []string
		atMark    uint64 // HeapInuse at plateauAt
		atEnd     uint64 // HeapInuse at frames
		baseAlloc uint64 // TotalAlloc at frame 0
	)
	runtime.GC()
	var first runtime.MemStats
	runtime.ReadMemStats(&first)
	baseAlloc = first.TotalAlloc

	for f := 1; f <= frames; f++ {
		if err := g.Update(); err != nil {
			t.Fatalf("Update frame %d: %v", f, err)
		}
		if f == 1 || f%markFrames == 0 {
			g.Draw(img)
		}
		if f%markFrames != 0 {
			continue
		}
		runtime.GC()
		var m runtime.MemStats
		runtime.ReadMemStats(&m)
		records = append(records, fmt.Sprintf(
			"frame %4d: HeapInuse=%7.2f MB HeapSys=%7.2f MB Sys=%7.2f MB TotalAllocΔ=%8.2f MB NumGC=%d",
			f,
			float64(m.HeapInuse)/1e6,
			float64(m.HeapSys)/1e6,
			float64(m.Sys)/1e6,
			float64(m.TotalAlloc-baseAlloc)/1e6,
			m.NumGC))
		if f == plateauAt {
			atMark = m.HeapInuse
		}
		if f == frames {
			atEnd = m.HeapInuse
		}
	}
	for _, r := range records {
		t.Log(r)
	}
	if atMark == 0 {
		t.Fatal("no measurement at the plateau mark")
	}

	growth := int64(atEnd) - int64(atMark)
	t.Logf("HeapInuse growth, frame %d -> %d: %+.2f MB (limit %.0f MB)",
		plateauAt, frames, float64(growth)/1e6, float64(maxGrowth)/1e6)
	if growth >= maxGrowth {
		t.Errorf("HeapInuse grew %+.2f MB between frames %d and %d — the live heap must plateau, not grow linearly",
			float64(growth)/1e6, plateauAt, frames)
	}
}

// newOffscreen builds an offscreen image, reporting a graphics-less
// environment as an error instead of panicking (see the skip above).
func newOffscreen(w, h int) (img *ebiten.Image, err error) {
	defer func() {
		if r := recover(); r != nil {
			img, err = nil, fmt.Errorf("%v", r)
		}
	}()
	return ebiten.NewImage(w, h), nil
}
