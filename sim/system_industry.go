package sim

import (
	"math"
	"sort"

	"github.com/entieve/entieve/content"
)

// ---------------------------------------------------------------------------
// Industry — manufacturing jobs executed from recipe content, and station
// craft growth ("learning by doing").
//
// A job runs a recipe's Runs sequential runs at one station. The materials
// for EVERY run are committed up front from the player's storage (integer
// units per input: ceil((input.Qty × runs) × craft material factor)); the
// output lands in the player's hangar when the job is collected. The fee
// (jobFeeRate of the recipe's mineral value) is paid up front too and is
// kept on cancel, which returns the materials. All stations share
// industrySlots job slots. Timing is pure clock math — Job.StartedAt/DoneAt
// against the sim clock — so the subsystem draws no RNG at all, and every
// list it touches keeps a canonical order (jobs ascending by id, craft by
// (station, family)).
//
// Craft growth is per station + per recipe family (hulls|modules|charges):
//
//	L = min(craftMax, 1 + floor(sqrt(totalRuns) / 2))
//
// totalRuns is the family's completed-run counter (a start or a completion
// resets its idle clock); it decays by one run-equivalent every
// craftDecayTicks idle ticks (floor 0, and an entry that decays to zero is
// dropped — no entry means L1). A station crafts at:
//
//	job time    × (1 - 0.04·(L-1))   → -4% per level, -16% at L5
//	materials   × (1 - 0.02·(L-1))   → -2% per level,  -8% at L5
//
// Both factors are frozen at job start (StartJob computes DoneAt and the
// committed materials once), so a mid-job craft-up never re-times a job.
//
// Owner tags mirror the market API: StartJob/CollectJob take an opaque
// owner (OwnerPlayer, OwnerSystem, or an agent's `agent:<id>` tag) and
// only that owner's job can be collected. Materials come from the player's
// storage and output lands in the player's hangar — except for an agent
// owner, whose bill and output move through the agent's own inventory
// (the ledger-agent material seam in agent.go).
// ---------------------------------------------------------------------------

const (
	// industrySlots is how many jobs ALL stations run concurrently: the
	// slots are a global pool, not per station. A completed-but-uncollected
	// job still occupies its slot.
	industrySlots = 3
	// jobFeeRate is the manufacturing fee: a share of the recipe's mineral
	// value at base prices, paid up front from the wallet.
	jobFeeRate = 0.03
	// craftMax is the craft-level cap (L5).
	craftMax = 5
	// craftDecayTicks is the idle stretch (ticks) that costs a family one
	// run-equivalent of craft — 3600 ticks = 60 sim seconds.
	craftDecayTicks = 3600
	// craftTimeBonus/craftMatBonus are the per-level benefits above L1.
	craftTimeBonus = 0.04
	craftMatBonus  = 0.02
)

// JobState is a job's place in its lifecycle.
type JobState string

const (
	// JobRunning: materials committed, waiting out DoneAt.
	JobRunning JobState = "running"
	// JobDone: finished; the output waits for CollectJob.
	JobDone JobState = "done"
)

// Job is one manufacturing job: Runs sequential runs of Recipe at Station,
// materials committed at StartedAt, output deliverable from DoneAt on.
// Persisted verbatim in Save.Jobs (ascending id order).
type Job struct {
	ID        uint64           `json:"id"`
	Station   StationID        `json:"station"`
	Recipe    content.RecipeID `json:"recipe"`
	Runs      int              `json:"runs"`
	StartedAt float64          `json:"started_at"` // sim clock
	DoneAt    float64          `json:"done_at"`    // sim clock
	State     JobState         `json:"state"`
	Owner     string           `json:"owner,omitempty"` // market-style owner tag
}

// SavedCraft is one station's craft counter for one recipe family.
// Persisted in canonical (station, family) order in Save.Craft.
type SavedCraft struct {
	Station StationID `json:"station"`
	Family  string    `json:"family"`
	Runs    float64   `json:"runs"` // completed-run total (drives CraftLevel)
	Idle    int64     `json:"idle"` // ticks since a start/completion reset it
}

// ---------------------------------------------------------------------------
// Craft growth
// ---------------------------------------------------------------------------

// CraftLevel is a station's craft level in a recipe family:
//
//	L = min(craftMax, 1 + floor(sqrt(totalRuns)/2))
//
// so L1 needs 0 runs, L2 4, L3 16, L4 36 and L5 64 (the cap binds from
// 100 runs on, where the raw formula would read L6).
func (s *Sim) CraftLevel(station StationID, family string) int {
	return craftLevel(s.craftRuns(station, family))
}

// craftLevel is the pure CraftLevel formula (0 runs → L1).
func craftLevel(runs float64) int {
	if runs <= 0 {
		return 1
	}
	l := 1 + int(math.Sqrt(runs)/2)
	if l > craftMax {
		l = craftMax
	}
	return l
}

// craftTimeFactor is the job-duration multiplier at a level
// (L1 → 1, L5 → 0.84).
func craftTimeFactor(level int) float64 {
	return 1 - craftTimeBonus*float64(level-1)
}

// craftMatFactor is the material multiplier at a level (L1 → 1, L5 → 0.92).
func craftMatFactor(level int) float64 {
	return 1 - craftMatBonus*float64(level-1)
}

// craftRuns returns a station's family counter (0 = no entry = L1).
func (s *Sim) craftRuns(station StationID, family string) float64 {
	if e := s.craftEntry(station, family, false); e != nil {
		return e.Runs
	}
	return 0
}

// craftEntry finds (or optionally inserts) the canonical craft record.
func (s *Sim) craftEntry(station StationID, family string, create bool) *SavedCraft {
	key := SavedCraft{Station: station, Family: family}
	for i := range s.craft {
		if s.craft[i].Station == key.Station && s.craft[i].Family == key.Family {
			return &s.craft[i]
		}
	}
	if !create {
		return nil
	}
	// Insertion keeps the slice sorted by (station, family) — the
	// canonical iteration order for decay, save and hash.
	idx := sort.Search(len(s.craft), func(i int) bool {
		a := s.craft[i]
		if a.Station != key.Station {
			return a.Station > key.Station
		}
		return a.Family >= key.Family
	})
	s.craft = append(s.craft, SavedCraft{})
	copy(s.craft[idx+1:], s.craft[idx:])
	s.craft[idx] = key
	return &s.craft[idx]
}

// craftUse marks a family in use (a job just started there): its idle
// decay clock restarts.
func (s *Sim) craftUse(station StationID, family string) {
	if e := s.craftEntry(station, family, false); e != nil {
		e.Idle = 0
	}
}

// craftComplete credits completed runs to a station's family counter
// (resetting its idle clock) and announces a level-up.
func (s *Sim) craftComplete(station StationID, family string, runs float64) {
	e := s.craftEntry(station, family, true)
	before := craftLevel(e.Runs)
	e.Runs += runs
	e.Idle = 0
	if after := craftLevel(e.Runs); after > before {
		s.emit(Event{Kind: EvLog, Severity: SevGood, Actor: s.player,
			Text: "Station craft (" + family + ") at " + s.stationName(station) +
				" rises to L" + itoaN(after)})
	}
}

// tickCraft ages every station-family counter by one tick and decays the
// idle ones: every craftDecayTicks of disuse cost one run-equivalent
// (floor 0; an entry that reaches 0 is dropped — no entry IS L1). Entries
// iterate in canonical order; the decrements are independent per entry,
// so the order cannot change the result.
func (s *Sim) tickCraft() {
	kept := s.craft[:0]
	for i := range s.craft {
		e := &s.craft[i]
		e.Idle++
		if e.Idle >= craftDecayTicks {
			e.Idle -= craftDecayTicks
			e.Runs = max(0, e.Runs-1)
		}
		if e.Runs > 0 {
			kept = append(kept, *e)
		}
	}
	s.craft = kept
}

// ---------------------------------------------------------------------------
// Job API (the agent seam; the player path is CmdStartJob/CmdCollectJob)
// ---------------------------------------------------------------------------

// JobDuration is a job's craft-adjusted wall clock:
//
//	Recipe.Time × runs × (1 - 0.04·(L-1))
//
// at the station's CURRENT craft level — StartJob freezes the value into
// Job.DoneAt. 0 for an unknown recipe or a non-positive run count.
func (s *Sim) JobDuration(station StationID, recipe content.RecipeID, runs int) float64 {
	rec, ok := s.Content.Recipes[recipe]
	if !ok || rec == nil || runs < 1 {
		return 0
	}
	return rec.Time * float64(runs) * craftTimeFactor(s.CraftLevel(station, rec.Family))
}

// JobFee is the up-front manufacturing fee of a job: jobFeeRate (3%) of
// the recipe's mineral value at content base prices × runs. The fee does
// not depend on the station, the craft level or the market, so it is
// recomputable from the recipe and run count alone (it is never persisted
// on the Job). Unknown recipes and non-positive runs cost nothing.
func (s *Sim) JobFee(recipe content.RecipeID, runs int) float64 {
	rec, ok := s.Content.Recipes[recipe]
	if !ok || rec == nil || runs < 1 {
		return 0
	}
	value := 0.0
	for _, in := range rec.Inputs {
		if in.Qty <= 0 {
			continue
		}
		value += in.Qty * s.marketBasePrice(in.Item)
	}
	return value * float64(runs) * jobFeeRate
}

// JobMaterials is the per-input integer commit of a job at the station's
// current craft level:
//
//	ceil((input.Qty × runs) × (1 - 0.02·(L-1)))
//
// — the exact numbers StartJob takes out of the player's storage and the
// factory panel's have/need column shows. Inputs with a non-positive
// quantity contribute nothing.
func (s *Sim) JobMaterials(station StationID, recipe content.RecipeID, runs int) []content.CargoStack {
	rec, ok := s.Content.Recipes[recipe]
	if !ok || rec == nil || runs < 1 {
		return nil
	}
	factor := craftMatFactor(s.CraftLevel(station, rec.Family))
	out := make([]content.CargoStack, 0, len(rec.Inputs))
	for _, in := range rec.Inputs {
		if in.Qty <= 0 {
			continue
		}
		n := int(math.Ceil(in.Qty * float64(runs) * factor))
		if n > 0 {
			out = append(out, content.CargoStack{Item: in.Item, Qty: n})
		}
	}
	return out
}

// AllJobs returns every job (all stations) in ascending id order — a copy,
// so callers never mutate sim state. The manufacturing window reads it for
// the player's job list (filtered by OwnerPlayer) and the global slot
// count; jobs are visible everywhere, controllable only where docked.
func (s *Sim) AllJobs() []Job { return append([]Job(nil), s.jobs...) }

// Jobs returns one station's jobs in ascending id order (a copy — callers
// never mutate sim state).
func (s *Sim) Jobs(station StationID) []Job {
	var out []Job
	for i := range s.jobs {
		if s.jobs[i].Station == station {
			out = append(out, s.jobs[i])
		}
	}
	return out
}

// jobCount is how many of the global slots every station's jobs take.
func (s *Sim) jobCount() int { return len(s.jobs) }

// startJobCheck is StartJob's validation as a predicate: it returns the
// resolved recipe and either ok, or the refusal reason StartJob logs.
// Side-effect free — CanStartJob is the exported form the factory panel
// gates its Start button on. owner routes the material check: an agent
// owner's bill comes out of the agent's inventory (the ledger-agent seam
// in agent.go), everyone else's out of the player's storage.
func (s *Sim) startJobCheck(station StationID, recipe content.RecipeID, runs int, owner string) (*content.RecipeDef, string, bool) {
	rec, ok := s.Content.Recipes[recipe]
	if !ok || rec == nil {
		return nil, "Unknown recipe " + string(recipe), false
	}
	if runs < 1 {
		return rec, "Invalid job quantity", false
	}
	if !s.stationService(station, content.SvcIndustry) {
		return rec, "This station does not offer industry", false
	}
	if s.jobCount() >= industrySlots {
		return rec, "All job slots busy", false
	}
	for _, m := range s.JobMaterials(station, recipe, runs) {
		have := s.playerStock(m.Item)
		if a := s.agentByOwner(owner); a != nil {
			have = a.freeStock(m.Item)
		}
		if have < m.Qty {
			return rec, "Not enough " + s.itemName(m.Item) + " to build " +
				s.itemName(rec.Product), false
		}
	}
	// The fee is a player wallet debit; agent jobs settle through the
	// agent's own capital seam and are not charged here.
	if owner == OwnerPlayer && s.wallet < s.JobFee(recipe, runs) {
		return rec, "Not enough credits for the job fee", false
	}
	return rec, "", true
}

// CanStartJob reports whether StartJob would accept right now (no side
// effects, no log lines) — the factory panel's gated Start button.
func (s *Sim) CanStartJob(station StationID, recipe content.RecipeID, runs int) bool {
	_, _, ok := s.startJobCheck(station, recipe, runs, OwnerPlayer)
	return ok
}

// StartJob begins a manufacturing job at station for owner and returns its
// id. Materials for every run are committed up front from the player's
// storage (cargo → ore hold → hangar → stored hulls, the market's order) —
// except for an agent owner, whose bill is debited from the agent's own
// inventory (and whose CollectJob delivers there): the ledger-agent
// material seam, so agents never touch the player's hangar. Refusals
// (unknown recipe, bad run count, no industry service, busy slots,
// missing materials) log one SevWarning line and change nothing.
// The player path is Queue(CmdStartJob{...}), which runs through
// systemIndustry with the docked-station service gate.
func (s *Sim) StartJob(station StationID, recipe content.RecipeID, runs int, owner string) (uint64, bool) {
	rec, reason, ok := s.startJobCheck(station, recipe, runs, owner)
	if !ok {
		s.industryRefusal(reason)
		return 0, false
	}

	// Commit the materials: count first (checked above), then take, and
	// roll the whole commit back on the impossible mismatch — never keep
	// a partial commit (execSell's rule).
	materials := s.JobMaterials(station, recipe, runs)
	agent := s.agentByOwner(owner)
	if agent != nil {
		for _, m := range materials { // freeStock >= m.Qty was just checked
			takeInv(agent, m.Item, m.Qty)
		}
	} else {
		var committed []content.CargoStack
		for _, m := range materials {
			taken := s.takePlayerStock(m.Item, m.Qty)
			if taken != m.Qty {
				s.storePlayerStock(m.Item, taken)
				for _, c := range committed {
					s.storePlayerStock(c.Item, c.Qty)
				}
				s.industryRefusal("Material count mismatch")
				return 0, false
			}
			committed = append(committed, m)
		}
	}

	level := s.CraftLevel(station, rec.Family)
	// The player's manufacturing fee is paid up front, before the job
	// exists (startJobCheck already proved the wallet covers it). Agent
	// owners settle through their own capital seam and pay no fee.
	if owner == OwnerPlayer {
		if fee := s.JobFee(recipe, runs); fee > 0 {
			s.wallet -= fee
			s.recordLedger(LedgerEntry{
				What:   "Industry fee — " + itoaN(runs) + " x " + s.itemName(rec.Product),
				Where:  s.stationName(station),
				Amount: -fee,
			})
		}
	}
	s.jobSeq++
	job := Job{
		ID: s.jobSeq, Station: station, Recipe: recipe, Runs: runs,
		StartedAt: s.time,
		DoneAt:    s.time + rec.Time*float64(runs)*craftTimeFactor(level),
		State:     JobRunning, Owner: owner,
	}
	s.jobs = append(s.jobs, job) // ids ascend: canonical order by construction
	s.craftUse(station, rec.Family)
	s.emit(Event{Kind: EvLog, Severity: SevInfo, Actor: s.player, Item: rec.Product,
		Text: "Industry: " + s.itemName(rec.Product) + " started at " + s.stationName(station)})
	return job.ID, true
}

// CollectJob delivers a finished job's output into the player's hangar and
// frees its slot (hulls land as StoredShips — the storePlayerStock rule);
// an agent owner's output lands in the agent's inventory instead (the same
// seam StartJob debits from). Only the job's own owner collects, and only
// JobDone jobs deliver; a job whose recipe no longer loads is dropped with
// a warning rather than blocking its slot forever (impossible with frozen
// content). The player path is Queue(CmdCollectJob{ID}).
func (s *Sim) CollectJob(station StationID, id uint64, owner string) bool {
	for i := range s.jobs {
		j := &s.jobs[i]
		if j.ID != id || j.Station != station {
			continue
		}
		if j.Owner != owner {
			return false
		}
		rec, ok := s.Content.Recipes[j.Recipe]
		if !ok || rec == nil {
			s.jobs = append(s.jobs[:i], s.jobs[i+1:]...)
			s.industryRefusal("Recipe " + string(j.Recipe) + " is no longer loaded")
			return false
		}
		if j.State != JobDone {
			return false
		}
		if a := s.agentByOwner(owner); a != nil {
			addInv(a, rec.Product, rec.Qty*j.Runs)
		} else {
			s.storePlayerStock(rec.Product, rec.Qty*j.Runs)
		}
		s.jobs = append(s.jobs[:i], s.jobs[i+1:]...)
		s.emit(Event{Kind: EvLog, Severity: SevGood, Actor: s.player, Item: rec.Product,
			Text: "Industry: " + s.itemName(rec.Product) + " collected at " + s.stationName(station)})
		return true
	}
	return false
}

// CancelJob removes a RUNNING job of owner at station and returns its
// committed materials to that owner (player hangar; agent inventory), the
// fee being kept. A finished job is never cancellable — collect it. Only
// the job's own owner cancels. Refund quantities are recomputed with
// JobMaterials (a craft level that moved mid-job can drift them by a
// unit; shortcut: freeze per-job materials only if that ever matters).
// The player path is Queue(CmdCancelJob{ID}).
func (s *Sim) CancelJob(station StationID, id uint64, owner string) bool {
	for i := range s.jobs {
		j := &s.jobs[i]
		if j.ID != id || j.Station != station {
			continue
		}
		if j.Owner != owner {
			return false
		}
		rec, ok := s.Content.Recipes[j.Recipe]
		if !ok || rec == nil {
			s.jobs = append(s.jobs[:i], s.jobs[i+1:]...)
			s.industryRefusal("Recipe " + string(j.Recipe) + " is no longer loaded")
			return false
		}
		if j.State != JobRunning {
			s.industryRefusal("Job is ready — collect it at " + s.stationName(station))
			return false
		}
		agent := s.agentByOwner(owner)
		for _, m := range s.JobMaterials(station, j.Recipe, j.Runs) {
			if agent != nil {
				addInv(agent, m.Item, m.Qty)
			} else {
				s.storePlayerStock(m.Item, m.Qty)
			}
		}
		s.jobs = append(s.jobs[:i], s.jobs[i+1:]...)
		s.emit(Event{Kind: EvLog, Severity: SevInfo, Actor: s.player, Item: rec.Product,
			Text: "Industry: " + s.itemName(rec.Product) + " cancelled at " +
				s.stationName(station) + " — materials returned"})
		return true
	}
	return false
}

// industryRefusal logs one SevWarning refusal line (the event/log surface
// for every refused start/collect).
func (s *Sim) industryRefusal(reason string) {
	s.log(SevWarning, s.player, "Industry: "+reason)
}

// stationService reports whether the CONTENT station def at the station
// celestial offers svc. Systems are scanned in sorted id order so the
// answer never depends on map order (the stationSystem pattern), and the
// lookup is content-side so agents address off-grid stations.
func (s *Sim) stationService(station StationID, svc string) bool {
	if station == "" {
		return false
	}
	systems := make([]content.SystemID, 0, len(s.Content.Systems))
	for id := range s.Content.Systems {
		systems = append(systems, id)
	}
	sort.Slice(systems, func(i, j int) bool { return systems[i] < systems[j] })
	for _, id := range systems {
		for _, cel := range s.Content.Systems[id].Celestials {
			if cel.ID != station || cel.Station == nil {
				continue
			}
			for _, have := range cel.Station.Services {
				if have == svc {
					return true
				}
			}
			return false
		}
	}
	return false
}

// ---------------------------------------------------------------------------
// System 17
// ---------------------------------------------------------------------------

// systemIndustry runs the industry command bucket (start/collect, gated by
// systemEconomy-style station access in execStartJob/execCollectJob), then
// advances the job clocks: jobs whose DoneAt has arrived flip to JobDone
// (crediting craft growth and logging the completion line), and finally
// every craft counter ages one tick toward its idle decay. Pure clock
// math — zero RNG draws.
func (s *Sim) systemIndustry() {
	for _, cmd := range s.deferIndustry {
		switch c := cmd.(type) {
		case CmdStartJob:
			s.execStartJob(c)
		case CmdCollectJob:
			s.execCollectJob(c)
		case CmdCancelJob:
			s.execCancelJob(c)
		}
	}
	s.deferIndustry = nil

	// Completions first: a family that finishes a job this tick is in
	// use, so the decay pass below must not idle-decrement it.
	for i := range s.jobs {
		j := &s.jobs[i]
		if j.State != JobRunning || s.time < j.DoneAt {
			continue
		}
		j.State = JobDone
		rec, ok := s.Content.Recipes[j.Recipe]
		if !ok || rec == nil {
			continue // impossible with frozen content (CollectJob guards it)
		}
		s.craftComplete(j.Station, rec.Family, float64(j.Runs))
		s.emit(Event{Kind: EvLog, Severity: SevGood, Actor: s.player, Item: rec.Product,
			Text: "Industry: " + s.itemName(rec.Product) + " complete at " + s.stationName(j.Station)})
	}
	s.tickCraft()
}

// execStartJob runs CmdStartJob: the docked-station service gate first
// (stationAccess), then the shared job path as the player.
func (s *Sim) execStartJob(c CmdStartJob) {
	if _, ok := s.stationAccess(content.SvcIndustry); !ok {
		return
	}
	s.StartJob(s.dockedStationID(), c.Recipe, c.Runs, OwnerPlayer)
}

// execCollectJob runs CmdCollectJob against the station the player is
// docked at (a job at another station never collects from here).
func (s *Sim) execCollectJob(c CmdCollectJob) {
	if _, ok := s.stationAccess(content.SvcIndustry); !ok {
		return
	}
	s.CollectJob(s.dockedStationID(), c.ID, OwnerPlayer)
}

// execCancelJob runs CmdCancelJob against the docked station (same gate
// and local-only rule as CollectJob).
func (s *Sim) execCancelJob(c CmdCancelJob) {
	if _, ok := s.stationAccess(content.SvcIndustry); !ok {
		return
	}
	s.CancelJob(s.dockedStationID(), c.ID, OwnerPlayer)
}

// StationName is the display name of a station celestial ("" when unknown)
// — the manufacturing window's per-station job grouping.
func (s *Sim) StationName(station StationID) string { return s.stationName(station) }
