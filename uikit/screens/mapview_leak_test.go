package screens

import (
	"fmt"
	"os"
	"runtime"
	"runtime/debug"
	"strconv"
	"strings"
	"testing"
)

// memSample is one memory reading: the Go heap live after a GC (retention),
// the cumulative allocation counter (churn) and the process RSS.
type memSample struct {
	heap  uint64
	total uint64
	rss   uint64
}

func readMem(t *testing.T) memSample {
	t.Helper()
	runtime.GC()
	var ms runtime.MemStats
	runtime.ReadMemStats(&ms)
	return memSample{heap: ms.HeapAlloc, total: ms.TotalAlloc, rss: rssBytes(t)}
}

// rssBytes reads the process resident set from /proc (Linux only; 0 when
// the file is unavailable). RSS is the signal that catches allocations the
// Go heap does not own: ebiten textures and driver buffers.
func rssBytes(t *testing.T) uint64 {
	t.Helper()
	raw, err := os.ReadFile("/proc/self/statm")
	if err != nil {
		return 0
	}
	fields := strings.Fields(string(raw))
	if len(fields) < 2 {
		return 0
	}
	pages, err := strconv.ParseUint(fields[1], 10, 64)
	if err != nil {
		return 0
	}
	return pages * uint64(os.Getpagesize())
}

func delta(a, b uint64) int64 { return int64(b) - int64(a) }

// TestMapViewHoverAndClickDoNotLeak is the starmap bug's feedback loop. It
// drives the exact interaction the owner reports: one hover frame over a
// system node (hover lag) and ONE click on it (the ~1 GiB step). Bounded by
// construction: three repaints at most, a GC after every measurement, so a
// red run cannot exhaust the machine (the owner's leak is ~1 GiB per click).
//
// The cause was ebiten 2.10.4: a zero-sweep arc (a fractional-centred circle)
// became a cubic whose rounding cusps expanded under round-joined stroking
// into tens of thousands of vertices. Drawing eight such rings per repaint
// retained hundreds of MiB in ebiten's command queue. v2.10.5 fixed it
// (vector: skip zero-sweep arc curves, #3858); this test fails if the pin
// ever regresses.
func TestMapViewHoverAndClickDoNotLeak(t *testing.T) {
	m, at, _, env := mapUnderTest(t)
	if len(m.nodes) == 0 {
		t.Fatal("fixture: no system nodes")
	}
	target := m.nodes[0].id

	// Warm the fixture: the first paint populates the icon and font caches,
	// which is one-time cost, not the leak.
	m.Sync()
	baseline := readMem(t)

	// One hover frame.
	p := at(target)
	env.in.x, env.in.y = p.X, p.Y
	env.in.edge = [3]bool{false, false, false}
	env.Pointer().Begin()
	m.Sync()
	hovered := readMem(t)
	runtime.GC()
	debug.FreeOSMemory()

	// ONE click (press edge) on the node.
	env.in.edge = [3]bool{true, false, false}
	env.Pointer().Begin()
	m.Sync()
	clicked := readMem(t)
	runtime.GC()
	debug.FreeOSMemory()

	retained := delta(baseline.heap, clicked.heap)
	rssGrowth := delta(baseline.rss, clicked.rss)
	hoverChurn := delta(baseline.total, hovered.total)
	clickChurn := delta(hovered.total, clicked.total)

	t.Logf("hover: heapΔ=%d KiB rssΔ=%d KiB churn=%d KiB",
		delta(baseline.heap, hovered.heap)/1024,
		delta(baseline.rss, hovered.rss)/1024, hoverChurn/1024)
	t.Logf("click: heapΔ=%d KiB rssΔ=%d KiB churn=%d KiB",
		delta(hovered.heap, clicked.heap)/1024,
		delta(hovered.rss, clicked.rss)/1024, clickChurn/1024)
	t.Logf("retention: heapΔ=%d KiB (%.1f MiB) rssΔ=%d KiB (%.1f MiB)",
		retained/1024, float64(retained)/(1<<20), rssGrowth/1024, float64(rssGrowth)/(1<<20))

	const maxRetained = 64 << 20
	const maxRSS = 128 << 20
	const maxChurn = 64 << 20
	if retained > maxRetained {
		t.Errorf("retained heap grew %d bytes (%s) across one hover+click; want under %d",
			retained, humanBytes(retained), maxRetained)
	}
	if rssGrowth > maxRSS {
		t.Errorf("process RSS grew %d bytes (%s) across one hover+click; want under %d",
			rssGrowth, humanBytes(rssGrowth), maxRSS)
	}
	// One repaint in this harness; a repaint whose antialiased strokes expand
	// into hundreds of thousands of vertices (the ebiten zero-sweep arc bug)
	// churns hundreds of MiB here.
	if hoverChurn > maxChurn {
		t.Errorf("the hover repaint allocated %s; want under %d per repaint",
			humanBytes(hoverChurn), maxChurn)
	}
	if clickChurn > maxChurn {
		t.Errorf("the click repaint allocated %s; want under %d per repaint",
			humanBytes(clickChurn), maxChurn)
	}
}

func humanBytes(n int64) string {
	if n < 0 {
		return "-" + humanBytes(-n)
	}
	units := []string{"B", "KiB", "MiB", "GiB"}
	f := float64(n)
	i := 0
	for f >= 1024 && i < len(units)-1 {
		f /= 1024
		i++
	}
	return fmt.Sprintf("%.1f %s", f, units[i])
}
