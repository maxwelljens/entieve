package sim

import (
	"encoding/json"
	"math"
	"testing"

	"github.com/entieve/entieve/content"
	"github.com/entieve/entieve/core"
	"github.com/entieve/entieve/ecs"
)

// ---------------------------------------------------------------------------
// Warp travel is a process: align gate -> three-segment tunnel (accel,
// distance-scaled cruise, decel) -> land on the point. These tests pin the
// durations, the speed profile, the landing accuracy, scram behaviour and
// determinism of the warp entry path.
// ---------------------------------------------------------------------------

// farCelestial places a static warp destination far outside the fixture
// system so tests can measure distance-scaled tunnel durations.
func farCelestial(s *Sim, name string, pos core.Vec2) ecs.Entity {
	e := s.create()
	s.Names.Set(e, Name{Value: name})
	s.Transforms.Set(e, Transform{Pos: pos})
	s.Celestials.Set(e, Celestial{Kind: content.CelPlanet, Cel: content.CelID(name)})
	return e
}

// warpRun steps a queued warp to completion, recording the drive phase seen
// after every tick plus the start/land event ticks (-1 when never seen).
type warpRun struct {
	startTick, landTick int64
	phases              []string
	evs                 []Event
}

func runWarp(s *Sim, cmd CmdWarpTo, budget int) warpRun {
	r := warpRun{startTick: -1, landTick: -1}
	s.Queue(cmd)
	for i := 0; i < budget; i++ {
		s.Step()
		if wd, ok := s.Warps.Get(s.player); ok {
			r.phases = append(r.phases, wd.Phase)
		} else {
			r.phases = append(r.phases, "")
		}
		for _, ev := range drain(s) {
			r.evs = append(r.evs, ev)
			if ev.Kind == EvWarp && ev.Phase == "start" && r.startTick < 0 {
				r.startTick = s.Ticks()
			}
			if ev.Kind == EvWarp && ev.Phase == "land" {
				r.landTick = s.Ticks()
			}
		}
		if r.landTick >= 0 {
			break
		}
	}
	return r
}

// warpSeconds is the duration of the tunnel ("warp") phase in seconds.
func (r warpRun) warpSeconds() float64 {
	return float64(r.landTick-r.startTick) * DT
}

// TestWarpIsProcessAlignTunnelLand pins the whole sequence: the drive runs
// align -> warp -> land in that order (via WarpDrive.Phase samples and the
// EvWarp start/land events), the tunnel lasts on the order of seconds —
// never a blink — and the whole hop stays inside the align+warp budget.
func TestWarpIsProcessAlignTunnelLand(t *testing.T) {
	s := newTestSim(t)
	gate := entityByName(s, "Gate to Beta")
	if gate.IsZero() {
		t.Fatal("no gate")
	}
	setPlayerPos(s, v(1e7, 1e7))
	r := runWarp(s, CmdWarpTo{Target: gate}, 1800)
	if r.startTick < 0 || r.landTick < 0 {
		t.Fatalf("warp never completed: start=%d land=%d events=%v",
			r.startTick, r.landTick, r.evs)
	}
	if r.startTick >= r.landTick {
		t.Fatalf("land before start: %d >= %d", r.startTick, r.landTick)
	}
	if len(r.phases) == 0 || r.phases[0] != "align" {
		t.Fatalf("first phase=%q want align", r.phases[0])
	}
	// Phases must run align* warp* done* with no backtracking.
	i := 0
	for i < len(r.phases) && r.phases[i] == "align" {
		i++
	}
	for i < len(r.phases) && r.phases[i] == "warp" {
		i++
	}
	for i < len(r.phases) && r.phases[i] == "" {
		i++
	}
	if i != len(r.phases) {
		t.Fatalf("phases not in align->warp->done order: %v", r.phases)
	}
	if findEvent(r.evs, EvWarp, "Warp drive active") == nil {
		t.Fatalf("missing warp start event: %v", r.evs)
	}
	if findEvent(r.evs, EvWarp, "Warp complete") == nil {
		t.Fatalf("missing warp land event: %v", r.evs)
	}
	if w := r.warpSeconds(); w < 6 {
		t.Fatalf("tunnel phase %.2fs: warp must read as a process (>=6s)", w)
	} else if w > 30 {
		t.Fatalf("tunnel phase %.2fs exceeds the warp budget", w)
	}
	if total := float64(r.landTick) * DT; total > 30 {
		t.Fatalf("align+tunnel total %.2fs exceeds the 30s budget", total)
	}
	if s.Warps.Has(s.player) {
		t.Fatal("drive still engaged after landing")
	}
	if d := s.distBetween(s.player, gate); d > warpLandRange {
		t.Fatalf("landed %v m from the gate (want <= %d)", d, warpLandRange)
	}
}

// TestWarpDurationScalesWithDistance pins the distance-scaled cruise term:
// a 10x longer hop takes strictly longer in the tunnel, while short hops
// never fall below the profile floor.
func TestWarpDurationScalesWithDistance(t *testing.T) {
	warpSecondsFor := func(d float64) float64 {
		t.Helper()
		s := newTestSim(t)
		setPlayerPos(s, v())
		target := farCelestial(s, "Tunnel End", v(d, 0))
		r := runWarp(s, CmdWarpTo{Target: target}, 20000)
		if r.startTick < 0 || r.landTick < 0 {
			t.Fatalf("hop of %g m never landed", d)
		}
		return r.warpSeconds()
	}
	// 2e12 m already exceeds the full-profile distance (4v) for the
	// fixture sloop (WarpSpeed 2 AU/s), so both hops show the cruise term.
	short := warpSecondsFor(2e12)
	long := warpSecondsFor(2e13)
	if short < 6 {
		t.Fatalf("2e12 m hop tunnel = %.2fs, want >= 6s", short)
	}
	if long <= short {
		t.Fatalf("10x longer hop not strictly slower: %.2fs vs %.2fs", long, short)
	}
	// The extra 1.8e13 m at warpSpeed alone is ~60 s of cruise.
	if long < short+30 {
		t.Fatalf("cruise term not visible: %.2fs vs %.2fs", long, short)
	}
}

// TestWarpShortHopNeverTeleports pins the clamp: even a 10 km hop spends
// seconds in the tunnel and creeps along the warp vector instead of
// jumping. The drive refuses inside WarpMinRange (150 km), so the hop
// starts just beyond it and lands WarpMinRange short of the station —
// 160 km out, 10 km of travel, still no teleport.
func TestWarpShortHopNeverTeleports(t *testing.T) {
	s := newTestSim(t)
	station := entityByName(s, "Haven Rest")
	stPos := gatePos(s, station)
	start := stPos.Add(v(WarpMinRange+10000, 0)) // 160 km off the station
	setPlayerPos(s, start)
	r := runWarp(s, CmdWarpTo{Target: station, Range: WarpMinRange}, 1800)
	if r.startTick < 0 || r.landTick < 0 {
		t.Fatalf("short hop never landed: %v", r.evs)
	}
	if w := r.warpSeconds(); w < 2 {
		t.Fatalf("10 km tunnel = %.2fs, that is a teleport (want >= 2s)", w)
	} else if w > 30 {
		t.Fatalf("10 km tunnel = %.2fs, far past the budget", w)
	}
	// Lands WarpMinRange short of the station, on the landing point.
	if d := s.distBetween(s.player, station); math.Abs(d-WarpMinRange) > warpLandRange {
		t.Fatalf("landed %v m from the station, want ≈ %d (WarpMinRange)", d, WarpMinRange)
	}
	land := stPos.Sub(stPos.Sub(start).Norm().Scale(WarpMinRange))
	if d := playerPos(s).Dist(land); d > warpLandRange {
		t.Fatalf("%v m from the landing point (want <= %d)", d, warpLandRange)
	}
}

// TestCmdWarpToMinimumRangeBoundary pins EVE's 150 km rule at the exact
// boundary: one metre inside WarpMinRange the command is refused — no
// drive, no movement, no intent, and a SevWarning log line naming the
// minimum — while a target at exactly WarpMinRange engages.
func TestCmdWarpToMinimumRangeBoundary(t *testing.T) {
	// 149,999 m: refused, state undisturbed.
	s := newTestSim(t)
	target := farCelestial(s, "Boundary Mark", v(1e7, 0))
	setPlayerPos(s, v(1e7-(WarpMinRange-1), 0))
	start := playerPos(s)
	s.Queue(CmdWarpTo{Target: target, Range: 10000})
	s.Step()
	if s.Warps.Has(s.player) {
		t.Fatal("warp drive engaged inside the minimum range")
	}
	if s.Intents.Has(s.player) {
		t.Fatal("refusal left a movement intent behind")
	}
	if playerPos(s) != start {
		t.Fatalf("refusal moved the ship: %v -> %v", start, playerPos(s))
	}
	evs := drain(s)
	if ev := findEvent(evs, EvLog, "Target within warp minimum range (150 km)"); ev == nil {
		t.Fatalf("no refusal log line: %v", evs)
	} else if ev.Severity != SevWarning {
		t.Errorf("refusal severity = %v, want SevWarning", ev.Severity)
	}
	if d := s.distBetween(s.player, target); d >= WarpMinRange {
		t.Fatalf("fixture: target at %v m, want inside %d", d, WarpMinRange)
	}

	// 150,000 m exactly: the drive engages (align phase).
	s2 := newTestSim(t)
	target2 := farCelestial(s2, "Boundary Mark", v(1e7, 0))
	setPlayerPos(s2, v(1e7-WarpMinRange, 0))
	s2.Queue(CmdWarpTo{Target: target2})
	s2.Step()
	if !s2.Warps.Has(s2.player) {
		evs := drain(s2)
		t.Fatalf("warp refused at exactly WarpMinRange: %v", evs)
	}
	if wd, _ := s2.Warps.Get(s2.player); wd.Phase != "align" {
		t.Errorf("phase = %q, want align", wd.Phase)
	}
}

// TestWarpSpeedProfileSegments pins WarpDrive.Speed during the tunnel: it
// rises through accel, holds exactly warpSpeed through the cruise, and
// falls through decel — sampled by stepping and reading the component.
func TestWarpSpeedProfileSegments(t *testing.T) {
	s := newTestSim(t)
	setPlayerPos(s, v())
	target := farCelestial(s, "Tunnel End", v(2e12, 0))
	peak := 2 * 1.496e11 // fixture sloop: WarpSpeed 2 AU/s

	s.Queue(CmdWarpTo{Target: target})
	var speeds []float64
	for i := 0; i < 8000; i++ {
		s.Step()
		landed := false
		for _, ev := range drain(s) {
			if ev.Kind == EvWarp && ev.Phase == "land" {
				landed = true
			}
		}
		if wd, ok := s.Warps.Get(s.player); ok && wd.Phase == "warp" {
			speeds = append(speeds, wd.Speed)
		}
		if landed {
			break
		}
	}
	if len(speeds) < 300 {
		t.Fatalf("only %d tunnel ticks observed", len(speeds))
	}
	// Rises from (near) rest during accel.
	if speeds[0] >= peak*0.1 {
		t.Fatalf("tunnel entry speed %v must be near 0", speeds[0])
	}
	if speeds[1] <= speeds[0] {
		t.Fatalf("speed must rise during accel: %v -> %v", speeds[0], speeds[1])
	}
	// Reaches exactly the hull warp speed.
	maxSpeed := 0.0
	for _, sp := range speeds {
		if sp > maxSpeed {
			maxSpeed = sp
		}
	}
	if math.Abs(maxSpeed-peak) > 1e-6*peak {
		t.Fatalf("peak tunnel speed %v want %v", maxSpeed, peak)
	}
	firstPeak := -1
	for i, sp := range speeds {
		if sp >= maxSpeed*(1-1e-12) {
			firstPeak = i
			break
		}
	}
	if firstPeak <= 0 || firstPeak >= len(speeds)-1 {
		t.Fatalf("peak must sit strictly inside the tunnel (firstPeak=%d of %d)",
			firstPeak, len(speeds))
	}
	for i := 1; i <= firstPeak; i++ {
		if speeds[i] < speeds[i-1]-1e-9*peak {
			t.Fatalf("speed fell during accel at tick %d", i)
		}
	}
	for i := firstPeak + 1; i < len(speeds); i++ {
		if speeds[i] > speeds[i-1]+1e-9*peak {
			t.Fatalf("speed rose during decel at tick %d", i)
		}
	}
	// Falls well below warpSpeed before landing.
	if last := speeds[len(speeds)-1]; last >= peak*0.5 {
		t.Fatalf("decel never happened: final speed %v", last)
	}
	// The cruise holds peak for at least warpCruiseMin seconds.
	atPeak := 0
	for _, sp := range speeds {
		if sp == peak {
			atPeak++
		}
	}
	if atPeak < int(warpCruiseMin*60) {
		t.Fatalf("cruise held peak for only %d ticks (want >= %d)",
			atPeak, int(warpCruiseMin*60))
	}
}

// TestWarpLandsRangeShortOfTarget pins landing accuracy: Range lands the
// ship that many metres short of the target, inside the 2500 m tolerance
// around the computed landing point.
func TestWarpLandsRangeShortOfTarget(t *testing.T) {
	s := newTestSim(t)
	station := entityByName(s, "Haven Rest")
	start := v(3e7, 3e7)
	setPlayerPos(s, start)
	const wantRange = 50000.0
	r := runWarp(s, CmdWarpTo{Target: station, Range: wantRange}, 1800)
	if r.landTick < 0 {
		t.Fatalf("warp never landed: %v", r.evs)
	}
	if s.Warps.Has(s.player) {
		t.Fatal("drive still engaged after landing")
	}
	if d := s.distBetween(s.player, station); math.Abs(d-wantRange) > 1 {
		t.Fatalf("landed %v m from target, want ≈ %v (Range)", d, wantRange)
	}
	stPos := gatePos(s, station)
	landPoint := stPos.Sub(stPos.Sub(start).Norm().Scale(wantRange))
	if d := playerPos(s).Dist(landPoint); d > warpLandRange {
		t.Fatalf("%v m from the landing point (want <= %d)", d, warpLandRange)
	}
}

// TestWarpScramBlocksStartAndCancelsMidCruise pins both scramble rules:
// no warp start while scrambled, and a mid-cruise warp is cancelled with
// the "scrammed" warning while the ship drops out between origin and
// target — stopped, never teleported.
func TestWarpScramBlocksStartAndCancelsMidCruise(t *testing.T) {
	// Blocks warp start.
	s := newTestSim(t)
	gate := entityByName(s, "Gate to Beta")
	ship, _ := s.Ships.Get(s.player)
	ship.ScrambledUntil = s.time + 100
	s.Queue(CmdWarpTo{Target: gate})
	s.Step()
	evs := drain(s)
	if s.Warps.Has(s.player) {
		t.Fatal("warp started while scrambled")
	}
	ev := findEvent(evs, EvWarp, "Warp scrambled")
	if ev == nil || ev.Severity != SevWarning || ev.Phase != "scrammed" {
		t.Fatalf("scram event=%+v want SevWarning/\"scrammed\"", ev)
	}

	// Cancels a warp mid-cruise.
	s2 := newTestSim(t)
	setPlayerPos(s2, v())
	target := farCelestial(s2, "Tunnel End", v(2e12, 0))
	origin := playerPos(s2)
	to := gatePos(s2, target).Sub(origin)
	d0 := to.Len()
	dir := to.Scale(1 / d0)
	peak := 2 * 1.496e11

	s2.Queue(CmdWarpTo{Target: target})
	cruise := false
	for i := 0; i < 600 && !cruise; i++ {
		s2.Step()
		drain(s2)
		if wd, ok := s2.Warps.Get(s2.player); ok && wd.Phase == "warp" {
			tr, _ := s2.Transforms.Get(s2.player)
			frac := (wd.Dist - wd.To.Sub(tr.Pos).Len()) / wd.Dist
			if frac > 0.3 && frac < 0.7 && wd.Speed >= peak*0.999 {
				cruise = true
			}
		}
	}
	if !cruise {
		t.Fatal("warp never reached the cruise segment")
	}
	ship2, _ := s2.Ships.Get(s2.player)
	ship2.ScrambledUntil = s2.time + 100
	s2.Step()
	evs = drain(s2)
	if s2.Warps.Has(s2.player) {
		t.Fatal("scram did not cancel the mid-cruise warp")
	}
	ev = findEvent(evs, EvWarp, "Warp scrambled")
	if ev == nil || ev.Severity != SevWarning || ev.Phase != "scrammed" {
		t.Fatalf("scram event=%+v want SevWarning/\"scrammed\"", ev)
	}

	// Dropped out on the warp line between origin and target, sublight.
	pos := playerPos(s2)
	proj := pos.Sub(origin).Dot(dir)
	perp := pos.Sub(origin).Sub(dir.Scale(proj)).Len()
	if proj <= 0 || proj >= d0-warpLandRange {
		t.Fatalf("drop-out at %.3g of the hop (d0=%g): not between origin and target",
			proj/d0, d0)
	}
	if perp > 1 {
		t.Fatalf("dropped off the warp line: perp=%v", perp)
	}
	m, _ := s2.Motions.Get(s2.player)
	if v := m.Vel.Len(); v > m.EffectiveMaxSpeed(s2.time)+1 {
		t.Fatalf("drop-out speed %v above sublight max %v", v, m.EffectiveMaxSpeed(s2.time))
	}
	// It brakes to a standstill right there — no coasting to the target.
	stepN(s2, 600)
	if v := m.Vel.Len(); v > 1 {
		t.Fatalf("ship did not stop after the scram: %v", v)
	}
	drift := playerPos(s2).Sub(origin).Dot(dir) - proj
	if drift <= 0 || drift > 2000 {
		t.Fatalf("post-scram drift %v m: want a small forward stop", drift)
	}
	if d := s2.distBetween(s2.player, target); d <= warpLandRange {
		t.Fatal("ship teleported to the warp destination on scram")
	}
}

// TestWarpTwinRunsIdentical pins determinism: two sims with the same seed
// and the same warp command hash identically at every checkpoint and land
// on the same tick.
func TestWarpTwinRunsIdentical(t *testing.T) {
	run := func() ([]uint64, int64) {
		s := newTestSim(t)
		gate := entityByName(s, "Gate to Beta")
		setPlayerPos(s, v(1e7, 1e7))
		s.Queue(CmdWarpTo{Target: gate})
		var hashes []uint64
		land := int64(-1)
		for i := 0; i < 900; i++ {
			s.Step()
			if i%90 == 0 {
				hashes = append(hashes, s.Hash())
			}
			for _, ev := range drain(s) {
				if ev.Kind == EvWarp && ev.Phase == "land" && land < 0 {
					land = s.Ticks()
				}
			}
		}
		return hashes, land
	}
	hashesA, landA := run()
	hashesB, landB := run()
	if landA < 0 {
		t.Fatal("warp never landed")
	}
	if landA != landB {
		t.Fatalf("arrival ticks differ: %d vs %d", landA, landB)
	}
	if len(hashesA) != len(hashesB) {
		t.Fatalf("checkpoint counts differ: %d vs %d", len(hashesA), len(hashesB))
	}
	for i := range hashesA {
		if hashesA[i] != hashesB[i] {
			t.Fatalf("checkpoint %d: hashes differ (%016x vs %016x)",
				i, hashesA[i], hashesB[i])
		}
	}
}

// TestWarpMidWarpSaveRestoreRoundTrip pins that the presentation fields
// (Dist/Speed) are part of the frozen Save schema: a snapshot taken inside
// the tunnel restores exactly (equal Hash) and both runs continue in
// lockstep.
func TestWarpMidWarpSaveRestoreRoundTrip(t *testing.T) {
	s := newTestSim(t)
	setPlayerPos(s, v())
	target := farCelestial(s, "Tunnel End", v(2e12, 0))
	s.Queue(CmdWarpTo{Target: target})
	inTunnel := false
	for i := 0; i < 600 && !inTunnel; i++ {
		s.Step()
		drain(s)
		if wd, ok := s.Warps.Get(s.player); ok && wd.Phase == "warp" && wd.Speed > 0 {
			inTunnel = true
		}
	}
	if !inTunnel {
		t.Fatal("warp never entered the tunnel")
	}
	wdBefore, _ := s.Warps.Get(s.player)
	want := *wdBefore
	wantHash := s.Hash()

	save, err := s.Snapshot()
	if err != nil {
		t.Fatalf("snapshot: %v", err)
	}
	raw, err := json.Marshal(save)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var decoded Save
	if err := json.Unmarshal(raw, &decoded); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	b, err := New(testContent(), testWorld(), testSeed)
	if err != nil {
		t.Fatal(err)
	}
	if err := b.Restore(&decoded); err != nil {
		t.Fatalf("restore: %v", err)
	}
	if got := b.Hash(); got != wantHash {
		t.Fatalf("hash after restore=%016x want %016x", got, wantHash)
	}
	wdAfter, ok := b.Warps.Get(b.player)
	if !ok {
		t.Fatal("drive lost across the round-trip")
	}
	if wdAfter.Phase != want.Phase || wdAfter.Dist != want.Dist ||
		wdAfter.Speed != want.Speed || wdAfter.To != want.To {
		t.Fatalf("drive=%+v want %+v", *wdAfter, want)
	}
	// Both runs continue in lockstep through the rest of the tunnel.
	for i := 0; i < 600; i++ {
		s.Step()
		b.Step()
		drain(s)
		drain(b)
		if s.Hash() != b.Hash() {
			t.Fatalf("continuation diverged at tick %d", i)
		}
	}
}
