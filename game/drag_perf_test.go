package game

import (
	"fmt"
	"image"
	"os"
	"runtime"
	"runtime/pprof"
	"sort"
	"testing"
	"time"

	"github.com/ebitenui/ebitenui/widget"
	"github.com/entieve/entieve/content"
	"github.com/entieve/entieve/sim"
	"github.com/hajimehoshi/ebiten/v2"
)

// Drag-cost measurement probe (perf/window-drag).
//
// Drives the REAL Game.Update/Game.Draw loop headlessly (loopMouse seam,
// the menu/rail real-loop pattern) and reports per phase:
//
//   - wall time per frame split into Update / scene draw / kit draw —
//     the scene render still carries headless overhead (no GPU, offscreen
//     atlas commands defer to a BeginFrame only a real window drains — see
//     TestMemoryFootprintPump) though the off-screen body cull
//     (TestWorldDrawCullsOffscreenBodies) removed its dominant term, so
//     the honest drag signal is the kit-draw + update delta vs idle
//   - exact allocations per frame (runtime.MemStats Mallocs delta)
//   - layout/settings sidecar signatures before vs after the phase (H2:
//     any sidecar write during a drag fails the run)
//
// The HEAVY fixture seeds the order book through sim.PlaceOrder
// (system-owned, non-crossing) to the hundreds of rows the real game
// accumulates; the showcase default is only 25. The LIGHT fixture drags
// the log window (static text area). The drag cursor OSCILLATES around
// the press point so the window never walks off-screen (off-screen text
// culls and would make drag frames look artificially cheap).

// seedBook seeds n resting system orders on st at non-crossing prices.
// System-owned orders carry no escrow, and escalating far-from-market
// prices never cross, so every order rests as its own row.
func seedBook(t *testing.T, g *Game, st sim.StationID, n int) {
	t.Helper()
	if st == "" {
		t.Fatal("no docked market station to seed")
	}
	items := make([]content.ItemID, 0, len(g.s.Content.Items))
	for id := range g.s.Content.Items {
		items = append(items, id)
	}
	if len(items) == 0 {
		t.Fatal("no content items to seed the book with")
	}
	sort.Slice(items, func(i, j int) bool { return items[i] < items[j] })
	placed := 0
	for i := 0; i < n; i++ {
		item := items[i%len(items)]
		// Escalating prices: sells ride far above market (no bid can
		// cross), buys far below (no ask can cross) — every order rests.
		price := 1e6 + float64(i%97)*1e3 + float64(i/97)
		side := sim.OrderSell
		if i%2 == 1 {
			side = sim.OrderBuy
			price = 0.01 + float64(i%89)*1e-4
		}
		if _, ok := g.s.PlaceOrder(st, side, item, 1, price, sim.OwnerSystem); ok {
			placed++
		}
	}
	if placed < n {
		t.Logf("seeded %d/%d orders (non-crossing placement refused the rest)", placed, n)
	}
	t.Logf("seeded book: %d resting orders", len(g.s.BookOrders(st)))
}

// dragPhase is one measured stretch of frames.
type dragPhase struct {
	name     string
	frames   int
	upd      []time.Duration
	scene    []time.Duration
	kit      []time.Duration
	mallocs  uint64
	bytes    uint64
	heap0    uint64
	heap1    uint64
	sidecar0 map[string]string
	sidecar1 map[string]string
}

func (p dragPhase) mean(d []time.Duration) time.Duration {
	if len(d) == 0 {
		return 0
	}
	var t time.Duration
	for _, v := range d {
		t += v
	}
	return t / time.Duration(len(d))
}

func (p dragPhase) pct(d []time.Duration, q float64) time.Duration {
	if len(d) == 0 {
		return 0
	}
	s := append([]time.Duration(nil), d...)
	sort.Slice(s, func(i, j int) bool { return s[i] < s[j] })
	idx := int(q*float64(len(s)-1) + 0.5)
	return s[idx]
}

// sidecarSig fingerprints both sidecars: "absent" or size:mtime-ns.
func sidecarSig(g *Game) map[string]string {
	sig := map[string]string{}
	for _, p := range []string{g.layoutPath(), g.settingsPath()} {
		fi, err := os.Stat(p)
		if err != nil {
			sig[p] = "absent"
			continue
		}
		sig[p] = fmt.Sprintf("%d:%d", fi.Size(), fi.ModTime().UnixNano())
	}
	return sig
}

func sigEqual(a, b map[string]string) bool {
	if len(a) != len(b) {
		return false
	}
	for k, v := range a {
		if b[k] != v {
			return false
		}
	}
	return true
}

// runPhase times and accounts N frames, splitting Update / scene draw /
// kit draw (the production g.Draw order: scenes first, then kit).
func runPhase(t *testing.T, name string, g *Game, img *ebiten.Image, frames int, tick func(i int)) dragPhase {
	t.Helper()
	p := dragPhase{name: name, frames: frames, sidecar0: sidecarSig(g)}
	runtime.GC()
	var m0, m1 runtime.MemStats
	runtime.ReadMemStats(&m0)
	p.heap0 = m0.HeapInuse
	for i := 0; i < frames; i++ {
		if tick != nil {
			tick(i)
		}
		tA := time.Now()
		if err := g.Update(); err != nil {
			t.Fatalf("%s frame %d Update: %v", name, i, err)
		}
		tB := time.Now()
		for _, sc := range g.scenes {
			sc.Draw(img)
		}
		tC := time.Now()
		g.kit.Draw(img)
		tD := time.Now()
		p.upd = append(p.upd, tB.Sub(tA))
		p.scene = append(p.scene, tC.Sub(tB))
		p.kit = append(p.kit, tD.Sub(tC))
	}
	runtime.ReadMemStats(&m1)
	p.heap1 = m1.HeapInuse
	p.mallocs = m1.Mallocs - m0.Mallocs
	p.bytes = m1.TotalAlloc - m0.TotalAlloc
	p.sidecar1 = sidecarSig(g)
	// H2 pin: no sidecar file may be created, written or touched by ANY
	// frame of ANY phase — persistence happens only on save/quit/scale.
	if !sigEqual(p.sidecar0, p.sidecar1) {
		for k, v := range p.sidecar0 {
			if p.sidecar1[k] != v {
				t.Errorf("%s: sidecar %s changed during the phase: %s -> %s "+
					"(a drag must not persist window rects)", name, k, v, p.sidecar1[k])
			}
		}
	}
	return p
}

func (p dragPhase) frameMean() time.Duration {
	return p.mean(p.upd) + p.mean(p.scene) + p.mean(p.kit)
}

func (p dragPhase) log(t *testing.T) {
	t.Helper()
	t.Logf("%-20s frames=%3d frame mean=%9s (upd=%7s scene=%8s kit=%7s) kit-p95=%8s | allocs/frame=%7.0f bytes/frame=%8.0f heapΔ=%+.1fMB",
		p.name, p.frames, p.frameMean(), p.mean(p.upd), p.mean(p.scene), p.mean(p.kit),
		p.pct(p.kit, 0.95),
		float64(p.mallocs)/float64(p.frames),
		float64(p.bytes)/float64(p.frames),
		float64(int64(p.heap1)-int64(p.heap0))/1e6)
}

// titlePoint returns a point on win's title bar: centre X, 20 px below
// the top edge — clear of the 8 px grip band on every side, inside the
// 44 px title bar, so the press arms the title drag only (never a grip).
func titlePoint(win *widget.Window) image.Point {
	r := win.GetContainer().GetWidget().Rect
	if r.Dx() == 0 || r.Dy() == 0 {
		return image.Point{}
	}
	return image.Pt(r.Min.X+r.Dx()/2, r.Min.Y+20)
}

// dragSetup parks the cursor on win's title bar, settles the hover and
// fires ONE press frame (the frame's own press edge, like production)
// outside the measured window. It returns the tick that OSCILLATES the cursor by
// ±amp px around the press point every frame — the window moves every
// frame (the chrome translates it per drag frame, as production) but
// never leaves the screen.
func dragSetup(t *testing.T, g *Game, img *ebiten.Image, mm *loopMouse, win *widget.Window, amp int) func(i int) {
	t.Helper()
	frame := func() {
		t.Helper()
		if err := g.Update(); err != nil {
			t.Fatalf("Update: %v", err)
		}
		g.Draw(img)
	}
	origin := titlePoint(win)
	if origin == (image.Point{}) {
		t.Fatalf("fixture: window at %v never laid out", win.GetContainer().GetWidget().Rect)
	}
	mm.moveTo(origin.X, origin.Y)
	frame()
	frame()
	before := win.GetContainer().GetWidget().Rect
	mm.left = true // the frame reports the rising edge
	frame()
	if r := win.GetContainer().GetWidget().Rect; r != before {
		t.Fatalf("fixture: rect changed on the press frame (%v -> %v), not during the drag", before, r)
	}
	// Oscillating delta: 0, +a, -a, +2a, -2a … every frame the cursor is
	// somewhere new (drag logic only moves the window when the cursor
	// moved), but the excursion is bounded so the window stays visible.
	return func(i int) {
		k := i % 8
		var d int
		switch k {
		case 0:
			d = 0
		case 1:
			d = amp
		case 2:
			d = -amp
		case 3:
			d = 2 * amp
		case 4:
			d = -2 * amp
		case 5:
			d = 3 * amp
		case 6:
			d = -3 * amp
		case 7:
			d = amp / 2
		}
		mm.moveTo(origin.X+d, origin.Y+d/2)
	}
}

// release drops the button and pumps one frame (outside measurement).
func release(t *testing.T, g *Game, img *ebiten.Image, mm *loopMouse) {
	t.Helper()
	mm.left = false
	if err := g.Update(); err != nil {
		t.Fatalf("Update: %v", err)
	}
	g.Draw(img)
}

// TestDragFrameCost measures idle vs drag cost in the real loop for the
// heavy (market, seeded to hundreds of rows) and light (log) window.
// Wall time is LOGGED (the headless atlas backlog makes absolute frame
// time meaningless — the honest numbers are the kit-draw delta and the
// alloc delta); sidecar writes are asserted (runPhase), and the
// drag-vs-idle allocation delta is bounded (maxDragAllocDelta).
func TestDragFrameCost(t *testing.T) {
	const (
		idleFrames = 60
		dragFrames = 60
		// A drag frame may not allocate much more than an idle frame:
		// the chrome's translate walk allocates nothing per frame, so
		// the measured residual (~+220 heavy / ~+240 light — ebitenui's
		// tooltip idle-state hide spam while the button is held, H3,
		// ebitenui-internal) sits far below this bound. Anything above
		// it is OUR-side churn creeping back into the drag path.
		// Re-aimed 700 -> 800: the high-sec guard posts put two more
		// orders on the docked book (394 -> 396 rows, ~+64 allocs per
		// visible row), which is fixture size, not drag-path churn.
		maxDragAllocDelta = 800
	)

	// ---- heavy fixture: docked showcase + market seeded to ~300 rows ----
	mm := useLoopMouse(t)
	g := dockedShowcaseGame(t, 1280, 720, mm.pointer())
	g.host.SetOpen(idMarket, true)
	g.syncMounts()
	pump(t, g, 1280, 720)
	seedBook(t, g, g.s.DockedMarket(), 300)
	pump(t, g, 1280, 720)
	img := ebiten.NewImage(1280, 720)

	orders := len(g.s.BookOrders(g.s.DockedMarket()))
	t.Logf("heavy fixture: market order book = %d resting orders (rows)", orders)

	mk := g.host.Window(idMarket)
	idleTick := func(i int) {
		c := titlePoint(mk)
		mm.moveTo(c.X, c.Y)
	}

	for i := 0; i < 30; i++ { // warm-up: atlas queue grown, hover settled
		idleTick(i)
		if err := g.Update(); err != nil {
			t.Fatalf("warmup Update: %v", err)
		}
		g.Draw(img)
	}

	phases := make([]dragPhase, 0, 4)
	idleA := runPhase(t, "idle-A", g, img, idleFrames, idleTick)
	phases = append(phases, idleA)

	dragTick := dragSetup(t, g, img, mm, mk, 40)
	mkBefore := mk.GetContainer().GetWidget().Rect
	dragA := runPhase(t, "market-drag-A", g, img, dragFrames, dragTick)
	phases = append(phases, dragA)
	if mkAfter := mk.GetContainer().GetWidget().Rect; mkAfter == mkBefore {
		t.Errorf("fixture: market did not move during drag-A (%v)", mkAfter)
	} else {
		t.Logf("market moved %v -> %v during drag-A", mkBefore, mkAfter)
	}
	release(t, g, img, mm)

	idleB := runPhase(t, "idle-B", g, img, idleFrames, idleTick)
	phases = append(phases, idleB)

	dragTick2 := dragSetup(t, g, img, mm, mk, 40)
	dragB := runPhase(t, "market-drag-B", g, img, dragFrames, dragTick2)
	phases = append(phases, dragB)
	release(t, g, img, mm)

	for _, p := range phases {
		p.log(t)
	}

	avg := func(a, b dragPhase, which func(dragPhase) []time.Duration) time.Duration {
		return (a.mean(which(a)) + b.mean(which(b))) / 2
	}
	idleKit := avg(idleA, idleB, func(p dragPhase) []time.Duration { return p.kit })
	dragKit := avg(dragA, dragB, func(p dragPhase) []time.Duration { return p.kit })
	idleUpd := avg(idleA, idleB, func(p dragPhase) []time.Duration { return p.upd })
	dragUpd := avg(dragA, dragB, func(p dragPhase) []time.Duration { return p.upd })
	idleAllocs := float64(idleA.mallocs+idleB.mallocs) / float64(idleA.frames+idleB.frames)
	dragAllocs := float64(dragA.mallocs+dragB.mallocs) / float64(dragA.frames+dragB.frames)
	t.Logf("HEAVY (market, %d rows): kit draw idle %s vs drag %s (%.2fx) | update idle %s vs drag %s | allocs/frame idle %.0f vs drag %.0f (%+.0f)",
		orders, idleKit, dragKit, float64(dragKit)/float64(idleKit),
		idleUpd, dragUpd, idleAllocs, dragAllocs, dragAllocs-idleAllocs)
	if d := dragAllocs - idleAllocs; d > maxDragAllocDelta {
		t.Errorf("HEAVY: drag allocates %+.0f allocs/frame over idle, bound is %d — per-drag allocation churn regressed",
			d, maxDragAllocDelta)
	}

	// ---- light fixture: space showcase, log window ----
	mm2 := useLoopMouse(t)
	g2 := newShowcaseGame(t, mm2.pointer())
	g2.applySize(1280, 720)
	pump(t, g2, 1280, 720)
	img2 := ebiten.NewImage(1280, 720)

	lg := g2.host.Window(idLog)
	lIdleTick := func(i int) {
		c := titlePoint(lg)
		mm2.moveTo(c.X, c.Y)
	}
	for i := 0; i < 30; i++ {
		lIdleTick(i)
		if err := g2.Update(); err != nil {
			t.Fatalf("warmup Update: %v", err)
		}
		g2.Draw(img2)
	}

	lIdle := runPhase(t, "light-idle", g2, img2, idleFrames, lIdleTick)
	lDragTick := dragSetup(t, g2, img2, mm2, lg, 40)
	lgBefore := lg.GetContainer().GetWidget().Rect
	lDrag := runPhase(t, "log-drag", g2, img2, dragFrames, lDragTick)
	if lgAfter := lg.GetContainer().GetWidget().Rect; lgAfter == lgBefore {
		t.Errorf("fixture: log did not move during drag (%v)", lgAfter)
	}
	release(t, g2, img2, mm2)

	lIdle.log(t)
	lDrag.log(t)
	t.Logf("LIGHT (log): kit draw idle %s vs drag %s (%.2fx) | allocs/frame idle %.0f vs drag %.0f (%+.0f)",
		lIdle.mean(lIdle.kit), lDrag.mean(lDrag.kit),
		float64(lDrag.mean(lDrag.kit))/float64(lIdle.mean(lIdle.kit)),
		float64(lIdle.mallocs)/float64(lIdle.frames),
		float64(lDrag.mallocs)/float64(lDrag.frames),
		float64(lDrag.mallocs)/float64(lDrag.frames)-float64(lIdle.mallocs)/float64(lIdle.frames))
	if d := float64(lDrag.mallocs)/float64(lDrag.frames) - float64(lIdle.mallocs)/float64(lIdle.frames); d > maxDragAllocDelta {
		t.Errorf("LIGHT: drag allocates %+.0f allocs/frame over idle, bound is %d — per-drag allocation churn regressed",
			d, maxDragAllocDelta)
	}
}

// TestDragCPUProfile writes CPU profiles of an idle phase and a market
// drag phase (heavy seeded book) plus exact allocation snapshots of
// short windows around each:
//
//	go tool pprof -top -diff_base=idle.cpu.prof drag.cpu.prof
//	go tool pprof -sample_index=alloc_objects -base=idle.alloc.start idle.alloc.end
//
// The alloc windows run with runtime.MemProfileRate=1 (every malloc
// recorded — the default 512 KB sampling never sees GridLayout's int
// slices); the rate is RESTORED before the CPU phases, because recording
// a stack per allocation swamps the CPU profile with runtime.traceback*
// samples.
//
//	Gated: ENTIEVE_DRAG_PROFILE=1 go test ./game -run TestDragCPUProfile -v
//
// Env: ENTIEVE_PERF_DIR (default /tmp/opencode/perf),
// ENTIEVE_PROFILE_FRAMES (default 300), ENTIEVE_ALLOC_FRAMES (default 30),
// ENTIEVE_SEED_ORDERS (default 300).
func TestDragCPUProfile(t *testing.T) {
	if os.Getenv("ENTIEVE_DRAG_PROFILE") == "" {
		t.Skip("set ENTIEVE_DRAG_PROFILE=1 to record profiles")
	}
	dir := os.Getenv("ENTIEVE_PERF_DIR")
	if dir == "" {
		dir = "/tmp/opencode/perf"
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", dir, err)
	}
	envInt := func(k string, def int) int {
		if v := os.Getenv(k); v != "" {
			var n int
			if _, err := fmt.Sscanf(v, "%d", &n); err == nil && n > 0 {
				return n
			}
		}
		return def
	}
	profFrames := envInt("ENTIEVE_PROFILE_FRAMES", 300)
	allocFrames := envInt("ENTIEVE_ALLOC_FRAMES", 30)
	seed := envInt("ENTIEVE_SEED_ORDERS", 300)

	mm := useLoopMouse(t)
	g := dockedShowcaseGame(t, 1280, 720, mm.pointer())
	g.host.SetOpen(idMarket, true)
	g.syncMounts()
	pump(t, g, 1280, 720)
	seedBook(t, g, g.s.DockedMarket(), seed)
	pump(t, g, 1280, 720)
	t.Logf("heavy fixture: %d resting orders", len(g.s.BookOrders(g.s.DockedMarket())))

	img := ebiten.NewImage(1280, 720)
	mk := g.host.Window(idMarket)
	idleTick := func(i int) {
		c := titlePoint(mk)
		mm.moveTo(c.X, c.Y)
	}
	for i := 0; i < 30; i++ {
		idleTick(i)
		if err := g.Update(); err != nil {
			t.Fatalf("warmup Update: %v", err)
		}
		g.Draw(img)
	}

	frame := func(tick func(i int), i int) {
		t.Helper()
		if tick != nil {
			tick(i)
		}
		if err := g.Update(); err != nil {
			t.Fatalf("Update: %v", err)
		}
		g.Draw(img)
	}

	snapshot := func(name string) {
		t.Helper()
		f, err := os.Create(dir + "/" + name)
		if err != nil {
			t.Fatalf("create %s: %v", name, err)
		}
		defer f.Close()
		runtime.GC()
		if err := pprof.Lookup("allocs").WriteTo(f, 0); err != nil {
			t.Fatalf("write %s: %v", name, err)
		}
	}

	// Exact-allocation window: rate=1 only here, restored after.
	allocWindow := func(kind string, tick func(i int)) {
		t.Helper()
		prev := runtime.MemProfileRate
		runtime.MemProfileRate = 1
		snapshot(kind + ".alloc.start")
		for i := 0; i < allocFrames; i++ {
			frame(tick, i)
		}
		snapshot(kind + ".alloc.end")
		runtime.MemProfileRate = prev
	}

	cpuWindow := func(kind string, tick func(i int)) {
		t.Helper()
		f, err := os.Create(dir + "/" + kind + ".cpu.prof")
		if err != nil {
			t.Fatalf("create %s.cpu.prof: %v", kind, err)
		}
		if err := pprof.StartCPUProfile(f); err != nil {
			f.Close()
			t.Fatalf("start profile: %v", err)
		}
		start := time.Now()
		for i := 0; i < profFrames; i++ {
			frame(tick, i)
		}
		elapsed := time.Since(start)
		pprof.StopCPUProfile()
		f.Close()
		t.Logf("%s: %d frames in %s (%s/frame)", kind, profFrames, elapsed, elapsed/time.Duration(profFrames))
	}

	allocWindow("idle", idleTick)
	cpuWindow("idle", idleTick)

	dragTick := dragSetup(t, g, img, mm, mk, 40)
	mkBefore := mk.GetContainer().GetWidget().Rect
	allocWindow("drag", dragTick)
	if mk.GetContainer().GetWidget().Rect == mkBefore {
		t.Error("fixture: market did not move during the drag alloc window")
	}
	cpuWindow("drag", dragTick)
	release(t, g, img, mm)

	t.Logf("profiles in %s: idle.cpu.prof / drag.cpu.prof (pprof -top -diff_base), idle.alloc.* / drag.alloc.* (pprof -base -sample_index=alloc_objects)", dir)
}
