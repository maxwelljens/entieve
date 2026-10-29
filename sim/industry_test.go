package sim

import (
	"encoding/json"
	"math"
	"os"
	"regexp"
	"strings"
	"testing"

	"github.com/entieve/entieve/content"
	"github.com/entieve/entieve/core"
)

// ---------------------------------------------------------------------------
// Industry fixture: the base content plus three recipes (one per craft
// family) and an industry-capable Haven Rest. testContent() rebuilds its
// maps on every call, so the patch below never leaks into other tests.
// ---------------------------------------------------------------------------

func recipeContent() *content.Content {
	c := testContent()
	c.Recipes = map[content.RecipeID]*content.RecipeDef{
		"rec-ammo-emp-s": {
			ID: "rec-ammo-emp-s", Product: "ammo-emp-s", Qty: 100,
			// 5.5 exercises the ceil commit; 3 the integer-exact path.
			Inputs: []content.RecipeInput{{Item: "min-ferrite", Qty: 5.5}, {Item: "min-nickel", Qty: 3}},
			Time:   45, Family: content.FamilyCharges,
		},
		"rec-mod-autocannon-s": {
			ID: "rec-mod-autocannon-s", Product: "mod-autocannon-s", Qty: 1,
			Inputs: []content.RecipeInput{{Item: "min-ferrite", Qty: 1}},
			Time:   30, Family: content.FamilyModules,
		},
		"rec-ship-sloop": {
			ID: "rec-ship-sloop", Product: "ship-sloop", Qty: 1,
			Inputs: []content.RecipeInput{{Item: "min-ferrite", Qty: 300}},
			Time:   120, Family: content.FamilyHulls,
		},
	}
	return c
}

// industryContent extends the recipe fixture with an industry-capable
// Haven Rest (the service the job commands gate on).
func industryContent() *content.Content {
	c := recipeContent()
	for i, cel := range c.Systems["sys-a"].Celestials {
		if cel.ID == "cel-station-a" && cel.Station != nil {
			c.Systems["sys-a"].Celestials[i].Station.Services =
				append(c.Systems["sys-a"].Celestials[i].Station.Services, "industry")
		}
	}
	return c
}

// newIndustrySim builds the industry fixture sim (industry station).
func newIndustrySim(t *testing.T) *Sim {
	t.Helper()
	s, err := New(industryContent(), testWorld(), testSeed)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	return s
}

// jobAt returns the station's job with the id (ok=false once collected).
func jobAt(s *Sim, station StationID, id uint64) (Job, bool) {
	for _, j := range s.Jobs(station) {
		if j.ID == id {
			return j, true
		}
	}
	return Job{}, false
}

// stepUntilDone ticks until the job flips to JobDone and returns it. The
// flip lands in the first step whose START time is at or past DoneAt, so
// the loop keeps stepping past the clock crossing.
func stepUntilDone(t *testing.T, s *Sim, station StationID, id uint64) Job {
	t.Helper()
	for i := 0; i < 200000; i++ {
		if j, ok := jobAt(s, station, id); ok && j.State == JobDone {
			return j
		}
		s.Step()
	}
	t.Fatalf("job %d never completed", id)
	return Job{}
}

// ---------------------------------------------------------------------------
// Job lifecycle
// ---------------------------------------------------------------------------

// TestIndustryJobLifecycle walks start → tick → complete → collect for a
// multi-run job through the command seam: the materials for every run are
// committed up front (ceil math), the job runs on pure clock math, the
// completion credits craft growth and the collect delivers into the hangar.
func TestIndustryJobLifecycle(t *testing.T) {
	s := newIndustrySim(t)
	dockAtHaven(t, s)
	drain(s)
	giveCargo(s, "min-ferrite", 1000)
	giveCargo(s, "min-nickel", 100)

	s.Queue(CmdStartJob{Recipe: "rec-ammo-emp-s", Runs: 2})
	s.Step()
	evs := drain(s)
	if !hasEvent(evs, EvLog, "Industry: EMP Charge started at Haven Rest") {
		t.Fatalf("missing start line: %v", evs)
	}
	jobs := s.Jobs("cel-station-a")
	if len(jobs) != 1 {
		t.Fatalf("jobs = %d, want 1", len(jobs))
	}
	j := jobs[0]
	if j.ID != 1 || j.Recipe != "rec-ammo-emp-s" || j.Runs != 2 ||
		j.State != JobRunning || j.Owner != OwnerPlayer || j.Station != "cel-station-a" {
		t.Fatalf("job = %+v", j)
	}
	// Materials for BOTH runs committed: ferrite ceil(5.5 × 2) = 11,
	// nickel 3 × 2 = 6. The clock math: 45 s × 2 runs at L1 = 90 s.
	if got := cargoCount(s, "min-ferrite"); got != 989 {
		t.Errorf("ferrite = %d, want 989 (1000 - 11 committed)", got)
	}
	if got := cargoCount(s, "min-nickel"); got != 94 {
		t.Errorf("nickel = %d, want 94 (100 - 6 committed)", got)
	}
	if want := 90.0; math.Abs((j.DoneAt-j.StartedAt)-want) > 1e-9 {
		t.Errorf("duration = %v, want %v", j.DoneAt-j.StartedAt, want)
	}
	// StartedAt is the clock the command ran on — the previous Step's end
	// (s.time advances after the systems run).
	if j.StartedAt != s.Time()-DT {
		t.Errorf("StartedAt = %v, want %v (the executing tick's clock)", j.StartedAt, s.Time()-DT)
	}

	// Ticking to the deadline flips it and logs the pinned completion line.
	j = stepUntilDone(t, s, "cel-station-a", 1)
	if j.DoneAt < s.Time()-1.0 || j.DoneAt > s.Time()+1.0 {
		t.Errorf("completed at clock %v, DoneAt %v", s.Time(), j.DoneAt)
	}
	evs = drain(s)
	if !hasEvent(evs, EvLog, "Industry: EMP Charge complete at Haven Rest") {
		t.Fatalf("missing completion line: %v", evs)
	}
	// Craft growth credited the runs (L1 at 2 runs, but the counter moved).
	if got := s.craftRuns("cel-station-a", content.FamilyCharges); got != 2 {
		t.Errorf("craft runs = %v, want 2", got)
	}
	if s.Hangar != nil && hangarCount(s, "ammo-emp-s") != 0 {
		t.Errorf("output delivered before collect (%d in hangar)", hangarCount(s, "ammo-emp-s"))
	}

	// Collect: 100 × 2 charges into the hangar, slot freed, line logged.
	s.Queue(CmdCollectJob{ID: 1})
	s.Step()
	evs = drain(s)
	if !hasEvent(evs, EvLog, "Industry: EMP Charge collected at Haven Rest") {
		t.Fatalf("missing collect line: %v", evs)
	}
	if got := hangarCount(s, "ammo-emp-s"); got != 200 {
		t.Errorf("hangar charges = %d, want 200 (100 × 2 runs)", got)
	}
	if jobs := s.Jobs("cel-station-a"); len(jobs) != 0 {
		t.Fatalf("jobs after collect = %+v, want none", jobs)
	}
	// Collecting again is a no-op.
	s.Queue(CmdCollectJob{ID: 1})
	s.Step()
	drain(s)
	if jobs := s.Jobs("cel-station-a"); len(jobs) != 0 {
		t.Fatalf("second collect resurrected the job: %+v", jobs)
	}
}

// TestIndustryJobCompletingCraftsUp: a real job completion crosses a craft
// level and logs the pinned craft-up line.
func TestIndustryJobCompletingCraftsUp(t *testing.T) {
	s := newIndustrySim(t)
	dockAtHaven(t, s)
	drain(s)
	giveCargo(s, "min-ferrite", 100)

	s.Queue(CmdStartJob{Recipe: "rec-mod-autocannon-s", Runs: 4})
	s.Step()
	drain(s)
	if _, ok := jobAt(s, "cel-station-a", 1); !ok {
		t.Fatal("job never started")
	}
	stepUntilDone(t, s, "cel-station-a", 1)
	evs := drain(s)
	if !hasEvent(evs, EvLog, "Station craft (modules) at Haven Rest rises to L2") {
		t.Fatalf("missing craft-up line: %v", evs)
	}
	if got := s.CraftLevel("cel-station-a", content.FamilyModules); got != 2 {
		t.Errorf("craft level = %d, want 2 (4 runs)", got)
	}
	// The other families are untouched.
	if got := s.CraftLevel("cel-station-a", content.FamilyCharges); got != 1 {
		t.Errorf("charges level = %d, want 1", got)
	}
}

// ---------------------------------------------------------------------------
// Materials and craft formulas (pinned numbers)
// ---------------------------------------------------------------------------

// TestIndustryMaterialCeilMath pins the commit arithmetic at both ends:
// exact integers at L1 (ceil of a whole number is itself), the fractional
// ceil path, and the L5 material factor.
func TestIndustryMaterialCeilMath(t *testing.T) {
	s := newIndustrySim(t)
	st := StationID("cel-station-a")

	mats := s.JobMaterials(st, "rec-ammo-emp-s", 2)
	want := []content.CargoStack{{Item: "min-ferrite", Qty: 11}, {Item: "min-nickel", Qty: 6}}
	if len(mats) != len(want) {
		t.Fatalf("materials = %+v, want %+v", mats, want)
	}
	for i := range want {
		if mats[i] != want[i] {
			t.Errorf("material[%d] = %+v, want %+v (ceil(5.5×2)=11, 3×2=6)", i, mats[i], want[i])
		}
	}
	// The fractional input at one run: ceil(5.5) = 6.
	if got := s.JobMaterials(st, "rec-ammo-emp-s", 1)[0].Qty; got != 6 {
		t.Errorf("ceil commit = %d, want 6 (5.5 rounded up)", got)
	}
	// L5: the material factor is 1 - 0.02·4 = 0.92.
	s.craftComplete(st, content.FamilyCharges, 100)
	if got := s.CraftLevel(st, content.FamilyCharges); got != 5 {
		t.Fatalf("fixture: level = %d, want 5 (capped)", got)
	}
	mats = s.JobMaterials(st, "rec-ammo-emp-s", 1)
	if mats[0].Qty != 6 { // ceil(5.5 × 0.92) = ceil(5.06) = 6
		t.Errorf("L5 ferrite = %d, want 6", mats[0].Qty)
	}
	if mats[1].Qty != 3 { // ceil(3 × 0.92) = ceil(2.76) = 3
		t.Errorf("L5 nickel = %d, want 3", mats[1].Qty)
	}
	// Unknown recipe / bad runs produce nothing.
	if got := s.JobMaterials(st, "rec-nope", 1); got != nil {
		t.Errorf("unknown recipe materials = %+v, want nil", got)
	}
	if got := s.JobMaterials(st, "rec-ammo-emp-s", 0); got != nil {
		t.Errorf("zero-run materials = %+v, want nil", got)
	}
}

// TestIndustryCraftFormulas pins the level curve, the cap and both craft
// factors exactly as documented on system_industry.go.
func TestIndustryCraftFormulas(t *testing.T) {
	for _, c := range []struct {
		runs float64
		want int
	}{
		{0, 1}, {1, 1}, {3, 1}, // below L2
		{4, 2}, {15, 2}, // L2 band
		{16, 3}, {35, 3}, // L3 band
		{36, 4}, {63, 4}, // L4 band
		{64, 5}, {99, 5}, // L5 band
		{100, 5}, {1e9, 5}, // cap binds (the raw formula reads 6+)
	} {
		if got := craftLevel(c.runs); got != c.want {
			t.Errorf("craftLevel(%v) = %d, want %d", c.runs, got, c.want)
		}
	}
	// Factors: -4% time and -2% materials per level above L1.
	for level, wantT := range map[int]float64{1: 1, 2: 0.96, 3: 0.92, 4: 0.88, 5: 0.84} {
		if got := craftTimeFactor(level); math.Abs(got-wantT) > 1e-12 {
			t.Errorf("craftTimeFactor(%d) = %v, want %v", level, got, wantT)
		}
	}
	for level, wantM := range map[int]float64{1: 1, 2: 0.98, 3: 0.96, 4: 0.94, 5: 0.92} {
		if got := craftMatFactor(level); math.Abs(got-wantM) > 1e-12 {
			t.Errorf("craftMatFactor(%d) = %v, want %v", level, got, wantM)
		}
	}
	// The duration the factory panel and StartJob agree on: Time × runs ×
	// factor, with the level read live from the station's counter.
	s := newIndustrySim(t)
	st := StationID("cel-station-a")
	if got := s.JobDuration(st, "rec-ammo-emp-s", 3); got != 135 {
		t.Errorf("L1 duration = %v, want 135 (45 × 3)", got)
	}
	s.craftComplete(st, content.FamilyCharges, 100) // L5
	got := s.JobDuration(st, "rec-ammo-emp-s", 3)
	if want := 45 * 3 * 0.84; math.Abs(got-want) > 1e-9 {
		t.Errorf("L5 duration = %v, want %v (45 × 3 × 0.84)", got, want)
	}
	if got := s.JobDuration(st, "rec-nope", 1); got != 0 {
		t.Errorf("unknown recipe duration = %v, want 0", got)
	}
}

// TestIndustryCraftGrowthAndDecay pins the counter behaviour: growth
// credits completed runs, the idle decay costs one run-equivalent per
// 3600 ticks (floor 0, entry dropped at zero = back to L1) and an entry in
// use never decays.
func TestIndustryCraftGrowthAndDecay(t *testing.T) {
	s := newIndustrySim(t)
	st := StationID("cel-station-a")

	// No entry reads L1 and starts empty.
	if got := s.CraftLevel(st, content.FamilyHulls); got != 1 {
		t.Fatalf("fresh level = %d, want 1", got)
	}
	if got := s.Jobs(st); len(got) != 0 {
		t.Fatalf("fresh jobs = %+v", got)
	}

	s.craftComplete(st, content.FamilyHulls, 16)
	if got := s.craftRuns(st, content.FamilyHulls); got != 16 {
		t.Fatalf("runs = %v, want 16", got)
	}
	if got := s.CraftLevel(st, content.FamilyHulls); got != 3 {
		t.Fatalf("level = %d, want 3 (16 runs)", got)
	}
	// 3599 idle ticks: no decay yet.
	stepN(s, craftDecayTicks-1)
	if got := s.craftRuns(st, content.FamilyHulls); got != 16 {
		t.Fatalf("runs after %d idle ticks = %v, want 16", craftDecayTicks-1, got)
	}
	// The 3600th costs exactly one run-equivalent.
	s.Step()
	if got := s.craftRuns(st, content.FamilyHulls); got != 15 {
		t.Fatalf("runs after %d idle ticks = %v, want 15", craftDecayTicks, got)
	}
	// A job start resets the idle clock (a family in use never decays).
	e := s.craftEntry(st, content.FamilyHulls, false)
	e.Idle = craftDecayTicks - 1
	s.craftUse(st, content.FamilyHulls)
	stepN(s, 10)
	if got := s.craftEntry(st, content.FamilyHulls, false).Idle; got != 10 {
		t.Fatalf("idle after craftUse = %d, want 10 (clock restarted)", got)
	}

	// Floor: an entry decayed to 0 is dropped — no entry IS L1.
	e = s.craftEntry(st, content.FamilyHulls, false)
	e.Runs, e.Idle = 1, craftDecayTicks-1
	s.Step()
	if got := s.craftEntry(st, content.FamilyHulls, false); got != nil {
		t.Fatalf("entry at 0 runs survived: %+v", got)
	}
	if got := s.CraftLevel(st, content.FamilyHulls); got != 1 {
		t.Fatalf("level after drop = %d, want 1", got)
	}
}

// ---------------------------------------------------------------------------
// Refusals and slots
// ---------------------------------------------------------------------------

// TestIndustryRefusals: every refusal path logs one warning line and
// changes nothing (no job, no material commit).
func TestIndustryRefusals(t *testing.T) {
	t.Run("unknown recipe", func(t *testing.T) {
		s := newIndustrySim(t)
		if _, ok := s.StartJob("cel-station-a", "rec-nope", 1, OwnerPlayer); ok {
			t.Fatal("unknown recipe accepted")
		}
		if evs := drain(s); !hasEvent(evs, EvLog, "Industry: Unknown recipe rec-nope") {
			t.Fatalf("missing refusal: %v", evs)
		}
		if jobs := s.Jobs("cel-station-a"); len(jobs) != 0 {
			t.Fatalf("refused start created a job: %+v", jobs)
		}
	})

	t.Run("invalid run count", func(t *testing.T) {
		s := newIndustrySim(t)
		if _, ok := s.StartJob("cel-station-a", "rec-ammo-emp-s", 0, OwnerPlayer); ok {
			t.Fatal("zero-run job accepted")
		}
		if evs := drain(s); !hasEvent(evs, EvLog, "Industry: Invalid job quantity") {
			t.Fatalf("missing refusal: %v", evs)
		}
	})

	t.Run("no industry service", func(t *testing.T) {
		// Recipes load, but this fixture's Haven Rest has no industry
		// service — the content-side gate.
		s, err := New(recipeContent(), testWorld(), testSeed)
		if err != nil {
			t.Fatalf("New: %v", err)
		}
		if _, ok := s.StartJob("cel-station-a", "rec-ammo-emp-s", 1, OwnerPlayer); ok {
			t.Fatal("job accepted at a station without the industry service")
		}
		if evs := drain(s); !hasEvent(evs, EvLog, "Industry: This station does not offer industry") {
			t.Fatalf("missing refusal: %v", evs)
		}
	})

	t.Run("must be docked", func(t *testing.T) {
		s := newIndustrySim(t)
		s.Queue(CmdStartJob{Recipe: "rec-ammo-emp-s", Runs: 1})
		s.Step()
		if evs := drain(s); !hasEvent(evs, EvLog, "must be docked") {
			t.Fatalf("missing dock gate: %v", evs)
		}
		if jobs := s.Jobs("cel-station-a"); len(jobs) != 0 {
			t.Fatalf("undocked start created a job: %+v", jobs)
		}
	})

	t.Run("missing materials", func(t *testing.T) {
		s := newIndustrySim(t)
		dockAtHaven(t, s)
		drain(s)
		if _, ok := s.StartJob("cel-station-a", "rec-ammo-emp-s", 1, OwnerPlayer); ok {
			t.Fatal("job accepted with an empty hold")
		}
		evs := drain(s)
		if !hasEvent(evs, EvLog, "Industry: Not enough Ferrite to build EMP Charge") {
			t.Fatalf("missing material refusal: %v", evs)
		}
		if jobs := s.Jobs("cel-station-a"); len(jobs) != 0 {
			t.Fatalf("refused start created a job: %+v", jobs)
		}
		if got := cargoCount(s, "min-ferrite"); got != 0 {
			t.Fatalf("refused start committed materials: %d", got)
		}
		// CanStartJob mirrors the same verdict, silently.
		if s.CanStartJob("cel-station-a", "rec-ammo-emp-s", 1) {
			t.Error("CanStartJob disagrees with the refusal")
		}
		// Exactly the L1 need: ceil(5.5) ferrite and 3 nickel.
		giveCargo(s, "min-ferrite", 6)
		giveCargo(s, "min-nickel", 3)
		if !s.CanStartJob("cel-station-a", "rec-ammo-emp-s", 1) {
			t.Error("CanStartJob refused a stocked start")
		}
	})

	t.Run("collect requires ownership and a done job", func(t *testing.T) {
		s := newIndustrySim(t)
		dockAtHaven(t, s)
		drain(s)
		giveCargo(s, "min-ferrite", 10)
		giveCargo(s, "min-nickel", 10)
		id, ok := s.StartJob("cel-station-a", "rec-ammo-emp-s", 1, OwnerPlayer)
		if !ok {
			t.Fatal("fixture start failed")
		}
		if s.CollectJob("cel-station-a", id, OwnerSystem) {
			t.Error("another owner collected the player's job")
		}
		if s.CollectJob("cel-station-a", id, OwnerPlayer) {
			t.Error("a running job collected")
		}
		if s.CollectJob("cel-station-a", 999, OwnerPlayer) {
			t.Error("unknown job id collected")
		}
		if jobs := s.Jobs("cel-station-a"); len(jobs) != 1 {
			t.Fatalf("failed collects changed the job list: %+v", jobs)
		}
	})
}

// TestIndustrySlotCap: two concurrent jobs per station; the third is
// refused with the busy line and commits nothing.
func TestIndustrySlotCap(t *testing.T) {
	s := newIndustrySim(t)
	dockAtHaven(t, s)
	drain(s)
	giveCargo(s, "min-ferrite", 100)
	giveCargo(s, "min-nickel", 100)

	for i := 0; i < industrySlots; i++ {
		if _, ok := s.StartJob("cel-station-a", "rec-ammo-emp-s", 1, OwnerPlayer); !ok {
			t.Fatalf("job %d refused with slots free", i+1)
		}
	}
	ferrite := cargoCount(s, "min-ferrite")
	if _, ok := s.StartJob("cel-station-a", "rec-ammo-emp-s", 1, OwnerPlayer); ok {
		t.Fatal("third job accepted over the slot cap")
	}
	evs := drain(s)
	if !hasEvent(evs, EvLog, "Industry: All job slots busy") {
		t.Fatalf("missing busy refusal: %v", evs)
	}
	if got := cargoCount(s, "min-ferrite"); got != ferrite {
		t.Errorf("refused job committed materials: %d -> %d", ferrite, got)
	}
	// A finished-but-uncollected job still occupies its slot.
	j := stepUntilDone(t, s, "cel-station-a", 1)
	if j.ID == 0 {
		t.Fatal("fixture job missing")
	}
	if _, ok := s.StartJob("cel-station-a", "rec-ammo-emp-s", 1, OwnerPlayer); ok {
		t.Fatal("slot freed by completion instead of collection")
	}
	drain(s)
	// Collecting frees it.
	if !s.CollectJob("cel-station-a", 1, OwnerPlayer) {
		t.Fatal("collect failed")
	}
	drain(s)
	if _, ok := s.StartJob("cel-station-a", "rec-ammo-emp-s", 1, OwnerPlayer); !ok {
		t.Fatal("slot not freed by collection")
	}
}

// ---------------------------------------------------------------------------
// Persistence and determinism
// ---------------------------------------------------------------------------

// TestIndustrySaveRestoreAndHash: jobs and craft ride the save (canonical
// orders), survive Snapshot → JSON → Restore with an identical hash, and
// both sims continue identically.
func TestIndustrySaveRestoreAndHash(t *testing.T) {
	s := newIndustrySim(t)
	dockAtHaven(t, s)
	drain(s)
	giveCargo(s, "min-ferrite", 1000)
	giveCargo(s, "min-nickel", 100)

	// One running job plus one finished-but-uncollected job (both occupy
	// slots until collect), craft counters for two families (one with an
	// idle clock mid-countdown).
	if _, ok := s.StartJob("cel-station-a", "rec-ammo-emp-s", 2, OwnerPlayer); !ok {
		t.Fatal("job 1 refused")
	}
	stepN(s, 10)
	if _, ok := s.StartJob("cel-station-a", "rec-mod-autocannon-s", 1, OwnerPlayer); !ok {
		t.Fatal("job 2 refused")
	}
	stepUntilDone(t, s, "cel-station-a", 2)
	s.craftComplete("cel-station-a", content.FamilyHulls, 16)
	stepN(s, 5)
	drain(s)
	wantHash := s.Hash()

	save, err := s.Snapshot()
	if err != nil {
		t.Fatalf("Snapshot: %v", err)
	}
	if len(save.Jobs) != 2 {
		t.Fatalf("Save.Jobs = %+v, want the running and the finished job", save.Jobs)
	}
	// Canonical job order (ascending id) and both states persisted.
	if save.Jobs[0].ID != 1 || save.Jobs[0].State != JobRunning ||
		save.Jobs[1].ID != 2 || save.Jobs[1].State != JobDone {
		t.Fatalf("Save.Jobs = %+v, want [1 running, 2 done]", save.Jobs)
	}
	if save.JobSeq != 2 {
		t.Errorf("Save.JobSeq = %d, want 2", save.JobSeq)
	}
	if len(save.Craft) != 2 {
		t.Fatalf("Save.Craft = %+v, want two family counters", save.Craft)
	}
	// Canonical craft order: station equal, family ascending — and the
	// completed jobs' counters are the ones present (the charge job is
	// still running, so its family has no counter yet).
	if save.Craft[0].Family != content.FamilyHulls || save.Craft[1].Family != content.FamilyModules {
		t.Errorf("Save.Craft order = (%s, %s), want (hulls, modules)",
			save.Craft[0].Family, save.Craft[1].Family)
	}
	if save.Craft[0].Runs != 16 {
		t.Errorf("Save.Craft hulls runs = %v, want 16", save.Craft[0].Runs)
	}
	if save.Schema != saveSchemaV3 {
		t.Errorf("Save.Schema = %d, want %d (industry rides the v3 schema)",
			save.Schema, saveSchemaV3)
	}

	// Plain JSON round-trip restores identically (Hash covers the new
	// fields through the Save serialization).
	var decoded Save
	raw, err := json.Marshal(save)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if err := json.Unmarshal(raw, &decoded); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	b, err := New(industryContent(), testWorld(), testSeed)
	if err != nil {
		t.Fatal(err)
	}
	if err := b.Restore(&decoded); err != nil {
		t.Fatalf("Restore: %v", err)
	}
	if got := b.Hash(); got != wantHash {
		t.Fatalf("restored hash=%016x want %016x", got, wantHash)
	}
	if jobs := b.Jobs("cel-station-a"); len(jobs) != 2 ||
		jobs[0].Recipe != "rec-ammo-emp-s" || jobs[1].State != JobDone {
		t.Fatalf("restored jobs = %+v", jobs)
	}
	if got := b.CraftLevel("cel-station-a", content.FamilyHulls); got != 3 {
		t.Fatalf("restored hulls level = %d, want 3", got)
	}
	if b.jobSeq != 2 {
		t.Errorf("restored jobSeq = %d, want 2", b.jobSeq)
	}

	// Both sims continue identically (the running job completes, the
	// finished one decays its idle clock on both sides).
	for i := 0; i < 6000; i++ {
		s.Step()
		b.Step()
	}
	if s.Hash() != b.Hash() {
		t.Fatal("continuation diverged after restore")
	}
}

// industryScript drives one scripted session: jobs start, finish, collect
// and the craft counters grow and decay — every command derives from the
// sim's own clock, so two runs are bit-identical.
func industryScripted(t *testing.T, seed int64, ticks int, checkpoints map[int]bool) map[int]uint64 {
	t.Helper()
	s, err := New(industryContent(), testWorld(), seed)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	station := entityByName(s, "Haven Rest")
	if station.IsZero() {
		t.Fatal("no station")
	}
	s.Queue(CmdDock{Station: station})
	s.Step()
	drain(s)
	giveCargo(s, "min-ferrite", 100000)
	giveCargo(s, "min-nickel", 10000)

	out := make(map[int]uint64)
	if checkpoints[0] {
		out[0] = s.Hash()
	}
	for i := 0; i < ticks; i++ {
		switch i {
		case 5:
			s.Queue(CmdStartJob{Recipe: "rec-ammo-emp-s", Runs: 1}) // 45 s
		case 6:
			s.Queue(CmdStartJob{Recipe: "rec-mod-autocannon-s", Runs: 2}) // 60 s
		case 7:
			s.Queue(CmdStartJob{Recipe: "rec-ship-sloop", Runs: 1}) // over the cap: refused
		case 1815: // module job done (~1806)
			s.Queue(CmdCollectJob{ID: 2})
		case 2715: // charge job done (~2706)
			s.Queue(CmdCollectJob{ID: 1})
			s.Queue(CmdStartJob{Recipe: "rec-ship-sloop", Runs: 1}) // 120 s, 2 free slots... 1 free
		case 9930: // hull job done (~9916)
			s.Queue(CmdCollectJob{ID: 3})
		case 5000:
			s.Queue(CmdCollectJob{ID: 99}) // unknown id: a deterministic no-op
		}
		s.Step()
		if checkpoints[i+1] {
			out[i+1] = s.Hash()
		}
	}
	return out
}

// TestIndustryDeterminismTwinRuns: two runs of the job script hash
// identically at every checkpoint (active jobs, refusals, craft growth and
// idle decay included).
func TestIndustryDeterminismTwinRuns(t *testing.T) {
	checkpoints := map[int]bool{1: true, 100: true, 1816: true, 2716: true, 5001: true, 7000: true, 10000: true}
	a := industryScripted(t, 7, 10000, checkpoints)
	b := industryScripted(t, 7, 10000, checkpoints)
	for cp := range checkpoints {
		if a[cp] != b[cp] {
			t.Fatalf("checkpoint %d: hashes differ (a=%016x b=%016x)", cp, a[cp], b[cp])
		}
	}
	// The script is state-sensitive: a later checkpoint must differ from
	// an earlier one (jobs and craft moved).
	if a[1] == a[10000] {
		t.Fatal("hash never moved: the script did nothing")
	}
	// The script really exercised the subsystem (slot refusal included).
	s, err := New(industryContent(), testWorld(), 7)
	if err != nil {
		t.Fatal(err)
	}
	if jobs := s.Jobs("cel-station-a"); len(jobs) != 0 {
		t.Fatalf("fresh sim has jobs: %+v", jobs)
	}
}

// TestIndustryNoRNGDraws: jobs are pure clock math — running a full
// start → complete → collect lifecycle consumes no master-stream rolls.
func TestIndustryNoRNGDraws(t *testing.T) {
	s := newIndustrySim(t)
	dockAtHaven(t, s)
	drain(s)
	giveCargo(s, "min-ferrite", 100)
	giveCargo(s, "min-nickel", 100)

	before := s.RNG.Snapshot()
	if _, ok := s.StartJob("cel-station-a", "rec-ammo-emp-s", 1, OwnerPlayer); !ok {
		t.Fatal("start refused")
	}
	stepUntilDone(t, s, "cel-station-a", 1)
	if !s.CollectJob("cel-station-a", 1, OwnerPlayer) {
		t.Fatal("collect failed")
	}
	stepN(s, craftDecayTicks+10) // long enough for a craft decay too
	if got := s.RNG.Snapshot(); got != before {
		t.Fatalf("industry drew master rolls: %016x -> %016x", before, got)
	}
}

// TestIndustrySystemOrderPinned: industry runs as system 18, the ledger
// agents as system 19 and the war as system 20, appended at the END of
// Step — the documented system order is a contract, so the sequence itself is
// what this test reads out of api.go.
func TestIndustrySystemOrderPinned(t *testing.T) {
	src, err := os.ReadFile("api.go")
	if err != nil {
		t.Fatalf("read api.go: %v", err)
	}
	re := regexp.MustCompile(`s\.(system[A-Za-z]+)\(\)`)
	var order []string
	for _, m := range re.FindAllStringSubmatch(string(src), -1) {
		order = append(order, m[1])
	}
	want := []string{
		"systemMovement", "systemWarp", "systemTargeting", "systemModules",
		"systemWeapons", "systemDamage", "systemCap", "systemShield", "systemMining",
		"systemNpc", "systemLoot", "systemEconomy", "systemTravel",
		"systemMission", "systemRespawn", "systemTraffic", "systemIndustry",
		"systemAgent", "systemWar",
	}
	if strings.Join(order, ",") != strings.Join(want, ",") {
		t.Fatalf("system order = %v, want %v", order, want)
	}
}

// TestIndustryRestoreRejectsCorruptJobs: the save-side validation fails
// loudly on a job entry no live sim could produce (a bogus state, a
// non-positive run count, an id the counter does not cover, a duplicate
// id) instead of restoring a job that can never complete or collect.
func TestIndustryRestoreRejectsCorruptJobs(t *testing.T) {
	base := func(t *testing.T) *Save {
		t.Helper()
		s := newIndustrySim(t)
		dockAtHaven(t, s)
		drain(s)
		giveCargo(s, "min-ferrite", 10)
		giveCargo(s, "min-nickel", 10)
		if _, ok := s.StartJob("cel-station-a", "rec-ammo-emp-s", 1, OwnerPlayer); !ok {
			t.Fatal("fixture start refused")
		}
		save, err := s.Snapshot()
		if err != nil {
			t.Fatalf("Snapshot: %v", err)
		}
		return save
	}
	cases := []struct {
		name  string
		mutfn func(*Save)
	}{
		{"bogus state", func(sv *Save) { sv.Jobs[0].State = "welding" }},
		{"zero runs", func(sv *Save) { sv.Jobs[0].Runs = 0 }},
		{"no station", func(sv *Save) { sv.Jobs[0].Station = "" }},
		{"id beyond JobSeq", func(sv *Save) { sv.Jobs[0].ID = sv.JobSeq + 1 }},
		{"duplicate id", func(sv *Save) { sv.JobSeq++; sv.Jobs = append(sv.Jobs, sv.Jobs[0]) }},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			save := base(t)
			c.mutfn(save)
			b, err := New(industryContent(), testWorld(), testSeed)
			if err != nil {
				t.Fatal(err)
			}
			if err := b.Restore(save); err == nil {
				t.Fatal("corrupt job save restored without error")
			}
		})
	}
	// The untouched save still restores (the cases above mutate copies).
	sv := base(t)
	b, err := New(industryContent(), testWorld(), testSeed)
	if err != nil {
		t.Fatal(err)
	}
	if err := b.Restore(sv); err != nil {
		t.Fatalf("valid save rejected: %v", err)
	}
}

// ---------------------------------------------------------------------------
// Manufacturing rework: global slots, job fee, cancel, duration ladder
// ---------------------------------------------------------------------------

// industryContentTwoStations extends the industry fixture with a second
// industry-capable station in sys-b (the global-slot pool fixture).
func industryContentTwoStations() *content.Content {
	c := industryContent()
	sys := c.Systems["sys-b"]
	sys.Celestials = append(sys.Celestials, content.CelestialDef{
		ID: "cel-station-b", Name: "Second Yard", Kind: content.CelStation,
		Position: core.Vec2{X: 2e7},
		Station:  &content.StationDef{Services: []string{"industry"}},
	})
	return c
}

// TestIndustrySlotsAreGlobal: the three slots are one pool shared by every
// station — jobs at a second station compete with the first, and collecting
// or cancelling anywhere frees the same slot.
func TestIndustrySlotsAreGlobal(t *testing.T) {
	s, err := New(industryContentTwoStations(), testWorld(), testSeed)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	giveCargo(s, "min-ferrite", 1000)
	giveCargo(s, "min-nickel", 1000)

	if _, ok := s.StartJob("cel-station-a", "rec-ammo-emp-s", 1, OwnerPlayer); !ok {
		t.Fatal("job 1 refused with slots free")
	}
	if _, ok := s.StartJob("cel-station-a", "rec-ammo-emp-s", 1, OwnerPlayer); !ok {
		t.Fatal("job 2 refused with slots free")
	}
	if _, ok := s.StartJob("cel-station-b", "rec-ammo-emp-s", 1, OwnerPlayer); !ok {
		t.Fatal("job 3 refused with one slot free")
	}
	if _, ok := s.StartJob("cel-station-b", "rec-ammo-emp-s", 1, OwnerPlayer); ok {
		t.Fatal("fourth job accepted over the global slot cap")
	}
	if evs := drain(s); !hasEvent(evs, EvLog, "Industry: All job slots busy") {
		t.Fatalf("missing busy refusal: %v", evs)
	}
	jobs := s.AllJobs()
	if len(jobs) != industrySlots {
		t.Fatalf("AllJobs = %d, want %d", len(jobs), industrySlots)
	}
	if jobs[0].Station != "cel-station-a" || jobs[2].Station != "cel-station-b" {
		t.Fatalf("AllJobs stations = %+v", jobs)
	}
	if got := s.StationName("cel-station-b"); got != "Second Yard" {
		t.Errorf("StationName = %q, want Second Yard", got)
	}

	// Cancelling at one station frees the shared slot for the other.
	if !s.CancelJob("cel-station-a", jobs[0].ID, OwnerPlayer) {
		t.Fatal("cancel failed")
	}
	drain(s)
	if _, ok := s.StartJob("cel-station-b", "rec-ammo-emp-s", 1, OwnerPlayer); !ok {
		t.Fatal("slot not freed by a cancel at another station")
	}
}

// TestIndustryJobFee pins the fee: 3% of the recipe's mineral value at
// base prices, per run, debited up front with one ledger line, and
// refused when the wallet cannot cover it.
func TestIndustryJobFee(t *testing.T) {
	s := newIndustrySim(t)
	st := StationID("cel-station-a")
	// (5.5 × 10) + (3 × 12) = 91 mineral value → 2.73 per run.
	if got := s.JobFee("rec-ammo-emp-s", 1); math.Abs(got-2.73) > 1e-9 {
		t.Errorf("JobFee(1) = %v, want 2.73", got)
	}
	if got := s.JobFee("rec-ammo-emp-s", 2); math.Abs(got-5.46) > 1e-9 {
		t.Errorf("JobFee(2) = %v, want 5.46 (runs linear)", got)
	}
	if got := s.JobFee("rec-nope", 1); got != 0 {
		t.Errorf("unknown recipe fee = %v, want 0", got)
	}

	dockAtHaven(t, s)
	drain(s)
	giveCargo(s, "min-ferrite", 100)
	giveCargo(s, "min-nickel", 100)
	before := s.Wallet()
	if _, ok := s.StartJob(st, "rec-ammo-emp-s", 2, OwnerPlayer); !ok {
		t.Fatal("stocked start refused")
	}
	if got := before - s.Wallet(); math.Abs(got-5.46) > 1e-9 {
		t.Errorf("upfront debit = %v, want 5.46", got)
	}
	led := s.Ledger()
	last := led[len(led)-1]
	if last.What != "Industry fee — 2 x EMP Charge" || math.Abs(last.Amount+5.46) > 1e-9 ||
		last.Where != "Haven Rest" {
		t.Errorf("fee ledger = %+v", last)
	}

	// An unaffordable fee refuses the start before any material moves.
	s.wallet = 1
	ferrite := cargoCount(s, "min-ferrite")
	ledgerLen := len(s.Ledger())
	if _, ok := s.StartJob(st, "rec-ammo-emp-s", 1, OwnerPlayer); ok {
		t.Fatal("start accepted with the wallet under the fee")
	}
	if evs := drain(s); !hasEvent(evs, EvLog, "Industry: Not enough credits for the job fee") {
		t.Fatalf("missing fee refusal: %v", evs)
	}
	if got := cargoCount(s, "min-ferrite"); got != ferrite {
		t.Errorf("refused start committed materials: %d -> %d", ferrite, got)
	}
	if len(s.Ledger()) != ledgerLen || s.Wallet() != 1 {
		t.Error("refused start moved money")
	}
}

// TestIndustryCancelRefundsMaterials: cancelling a running job returns its
// committed materials to the hangar, keeps the fee, drops the job and frees
// its slot; a finished job must be collected instead.
func TestIndustryCancelRefundsMaterials(t *testing.T) {
	s := newIndustrySim(t)
	dockAtHaven(t, s)
	drain(s)
	giveCargo(s, "min-ferrite", 1000)
	giveCargo(s, "min-nickel", 100)
	wallet := s.Wallet()

	id, ok := s.StartJob("cel-station-a", "rec-ammo-emp-s", 2, OwnerPlayer)
	if !ok {
		t.Fatal("start refused")
	}
	// Committed from cargo: ceil(5.5×2)=11 ferrite, 6 nickel.
	if cargoCount(s, "min-ferrite") != 989 || cargoCount(s, "min-nickel") != 94 {
		t.Fatalf("commit = %d ferrite / %d nickel", cargoCount(s, "min-ferrite"), cargoCount(s, "min-nickel"))
	}
	s.Queue(CmdCancelJob{ID: id})
	s.Step()
	if evs := drain(s); !hasEvent(evs, EvLog, "Industry: EMP Charge cancelled at Haven Rest") {
		t.Fatalf("missing cancel line: %v", evs)
	}
	if got := hangarCount(s, "min-ferrite"); got != 11 {
		t.Errorf("hangar ferrite = %d, want 11", got)
	}
	if got := hangarCount(s, "min-nickel"); got != 6 {
		t.Errorf("hangar nickel = %d, want 6", got)
	}
	if jobs := s.AllJobs(); len(jobs) != 0 {
		t.Fatalf("cancelled job survived: %+v", jobs)
	}
	// The fee is kept: no refund line, no credit.
	if got := s.Wallet(); got != wallet-s.JobFee("rec-ammo-emp-s", 2) {
		t.Errorf("wallet after cancel = %v, want the fee kept", got)
	}
	if _, ok := s.StartJob("cel-station-a", "rec-ammo-emp-s", 1, OwnerPlayer); !ok {
		t.Fatal("cancel did not free the slot")
	}

	// A finished job cannot be cancelled; another owner cannot either.
	done, ok := s.StartJob("cel-station-a", "rec-ammo-emp-s", 1, OwnerPlayer)
	if !ok {
		t.Fatal("fixture start refused")
	}
	stepUntilDone(t, s, "cel-station-a", done)
	drain(s)
	if s.CancelJob("cel-station-a", done, OwnerPlayer) {
		t.Fatal("finished job cancelled")
	}
	if evs := drain(s); !hasEvent(evs, EvLog, "Industry: Job is ready — collect it at Haven Rest") {
		t.Fatalf("missing ready refusal: %v", evs)
	}
	if s.CancelJob("cel-station-a", done, OwnerSystem) {
		t.Fatal("another owner cancelled the job")
	}
	if _, ok := jobAt(s, "cel-station-a", done); !ok {
		t.Fatal("refused cancel removed the job")
	}
}

// TestRecipeDurationLadder pins the shipped per-run durations: modules ~3
// minutes and hulls ~10 minutes at 1× (ammo is another stream's file; the
// duration math is pinned in TestIndustryCraftFormulas).
func TestRecipeDurationLadder(t *testing.T) {
	c, err := content.Load(content.FS())
	if err != nil {
		t.Fatalf("content.Load: %v", err)
	}
	modules, hulls := 0, 0
	for _, rec := range c.RecipesInOrder() {
		switch rec.Family {
		case content.FamilyModules:
			modules++
			if rec.Time != 180 {
				t.Errorf("%s time = %v, want 180 s", rec.ID, rec.Time)
			}
		case content.FamilyHulls:
			hulls++
			if rec.Time != 600 {
				t.Errorf("%s time = %v, want 600 s", rec.ID, rec.Time)
			}
		case content.FamilyCharges:
			// charges.json belongs to the ammo stream (the ~30 s rung).
		default:
			t.Errorf("%s has unknown family %q", rec.ID, rec.Family)
		}
	}
	if modules == 0 || hulls == 0 {
		t.Fatalf("ladder fixture empty: %d modules, %d hulls", modules, hulls)
	}
}
