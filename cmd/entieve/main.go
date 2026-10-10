// Command entieve is the Entieve game and engine toolkit.
//
// Subcommands:
//
//	entieve run       — play the game (Ebitengine window)
//	entieve validate  — lint and cross-check all game content
//	entieve simulate  — run the simulation headlessly (AI-agent tooling)
//	entieve version   — print version
//
// The simulate subcommand is the AI-controllability seam: agents script the
// game with tick-scheduled command files (see docs/CONTENT_AUTHORING.md and
// the script package) and read back the event log and state report.
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/entieve/entieve/content"
	"github.com/entieve/entieve/game"
	"github.com/entieve/entieve/script"
	"github.com/entieve/entieve/sim"
)

// version is stamped at build time via -ldflags "-X main.version=...".
var version = "0.1.0-dev"

const usage = `Entieve — a love letter to EVE Online

Usage:
  entieve run       [--world ID] [--seed N] [--load save.json] [--save save.json]
  entieve validate  [--dir content/data]
  entieve simulate  [--world ID] [--seed N] [--ticks N] [--script steps.json] [--quiet]
  entieve describe  <id>            (any content id: item, ship, module, npc, system, world, contract)
  entieve version

Run "entieve <subcommand> -h" for subcommand flags.`

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}

func run(args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		fmt.Fprintln(stderr, usage)
		return 2
	}
	switch args[0] {
	case "run":
		return cmdRun(args[1:], stdout, stderr)
	case "validate":
		return cmdValidate(args[1:], stdout, stderr)
	case "simulate":
		return cmdSimulate(args[1:], stdout, stderr)
	case "describe":
		return cmdDescribe(args[1:], stdout, stderr)
	case "version":
		fmt.Fprintf(stdout, "entieve %s\n", version)
		return 0
	case "-h", "--help", "help":
		fmt.Fprintln(stdout, usage)
		return 0
	default:
		fmt.Fprintf(stderr, "entieve: unknown subcommand %q\n\n%s\n", args[0], usage)
		return 2
	}
}

func cmdRun(args []string, stdout, stderr io.Writer) int {
	fs := newFlagSet("run", stderr)
	world := fs.String("world", "world-default", "world (scenario) ID to play")
	seed := fs.Int64("seed", 42, "simulation RNG seed")
	load := fs.String("load", "", "save file to load at startup")
	save := fs.String("save", defaultSavePath(), "save file written by the in-game save key")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	c, err := content.Load(content.FS())
	if err != nil {
		fmt.Fprintf(stderr, "entieve: load content: %v\n", err)
		return 1
	}
	wd, ok := c.World(content.WorldID(*world))
	if !ok {
		fmt.Fprintf(stderr, "entieve: unknown world %q (see worlds/ in content data)\n", *world)
		return 1
	}
	opts := game.Options{
		Content:  c,
		World:    wd,
		Title:    "Entieve — " + wd.Name,
		SavePath: *save,
		Seed:     *seed,
	}
	if *load != "" {
		save, err := readSave(*load)
		if err != nil {
			fmt.Fprintf(stderr, "entieve: load save: %v\n", err)
			return 1
		}
		opts.Save = save
	}
	g, err := game.New(opts)
	if err != nil {
		fmt.Fprintf(stderr, "entieve: start game: %v\n", err)
		return 1
	}
	if err := g.Run(); err != nil {
		fmt.Fprintf(stderr, "entieve: game: %v\n", err)
		return 1
	}
	return 0
}

func cmdValidate(args []string, stdout, stderr io.Writer) int {
	fs := newFlagSet("validate", stderr)
	dir := fs.String("dir", "", "validate content from a directory instead of embedded data")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	var (
		c   *content.Content
		err error
	)
	if *dir != "" {
		c, err = content.LoadDir(*dir)
	} else {
		c, err = content.Load(content.FS())
	}
	if err != nil {
		fmt.Fprintf(stderr, "entieve validate: load: %v\n", err)
		return 1
	}
	if err := c.Validate(); err != nil {
		fmt.Fprintf(stderr, "entieve validate: FAILED\n%v\n", err)
		return 1
	}
	fmt.Fprintf(stdout, "content OK: %d items, %d ships, %d modules, %d npcs, %d systems, %d worlds, contracts: %d\n",
		len(c.Items), len(c.Ships), len(c.Modules), len(c.NPCs), len(c.Systems), len(c.Worlds), len(c.Contracts))
	return 0
}

func cmdSimulate(args []string, stdout, stderr io.Writer) int {
	fs := newFlagSet("simulate", stderr)
	world := fs.String("world", "world-showcase", "world (scenario) ID to simulate")
	seed := fs.Int64("seed", 42, "simulation RNG seed")
	ticks := fs.Int64("ticks", 600, "number of ticks to run (60 ticks = 1 sim second)")
	scriptPath := fs.String("script", "", "JSON command script to execute during the run")
	quiet := fs.Bool("quiet", false, "suppress the event log (report still printed)")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	c, err := content.Load(content.FS())
	if err != nil {
		fmt.Fprintf(stderr, "entieve simulate: load content: %v\n", err)
		return 1
	}
	wd, ok := c.World(content.WorldID(*world))
	if !ok {
		fmt.Fprintf(stderr, "entieve simulate: unknown world %q\n", *world)
		return 1
	}
	s, err := sim.New(c, wd, *seed)
	if err != nil {
		fmt.Fprintf(stderr, "entieve simulate: %v\n", err)
		return 1
	}
	var steps []script.Step
	if *scriptPath != "" {
		data, err := os.ReadFile(*scriptPath)
		if err != nil {
			fmt.Fprintf(stderr, "entieve simulate: read script: %v\n", err)
			return 1
		}
		steps, err = script.Parse(data)
		if err != nil {
			fmt.Fprintf(stderr, "entieve simulate: %v\n", err)
			return 1
		}
	}
	onEvent := func(ev sim.Event) {
		if !*quiet {
			fmt.Fprintf(stdout, "[%7.2fs] %-8s %s\n", ev.Time, ev.Kind, ev.Text)
		}
	}
	res := script.Run(s, steps, *ticks, onEvent)
	for _, st := range res.Unresolved {
		fmt.Fprintf(stderr, "entieve simulate: unresolved step at tick %d: %s %s\n",
			st.AtTick, st.Action, st.Target)
	}
	printReport(stdout, s, res, *ticks)
	return 0
}

func printReport(w io.Writer, s *sim.Sim, res *script.Result, ticks int64) {
	fmt.Fprintf(w, "\n== simulation report ==\n")
	fmt.Fprintf(w, "ticks: %d (%.2fs sim time)\n", ticks, s.Time())
	fmt.Fprintf(w, "wallet: %.2f cr\n", s.Wallet())
	// Standing and the contract book next to the wallet: the read surface
	// of the contracts loop (docs/GAMEPLAY.md, "Contracts & standing").
	fmt.Fprintf(w, "standing: %.2f\n", s.Standing())
	active := s.Missions()
	var completed []string
	for _, ev := range res.Events {
		if ev.Kind == sim.EvMission && ev.Phase == sim.MissionCompleted {
			completed = append(completed, ev.ID)
		}
	}
	fmt.Fprintf(w, "missions: %d active, %d completed\n", len(active), len(completed))
	for _, m := range active {
		line := "  active " + string(m.ID)
		if def, ok := s.Content.Contract(m.ID); ok {
			line += fmt.Sprintf(" %g/%g", m.Progress, float64(def.Objective.Count))
		}
		fmt.Fprintln(w, line)
	}
	for _, id := range completed {
		fmt.Fprintln(w, "  completed "+id)
	}
	fmt.Fprintf(w, "state hash: %016x\n", s.Hash())
	fmt.Fprintf(w, "events: %d, steps run: %d, unresolved: %d\n",
		len(res.Events), res.Ran, len(res.Unresolved))
	kindCount := map[string]int{}
	for _, ev := range res.Events {
		kindCount[string(ev.Kind)]++
	}
	var keys []string
	for k := range kindCount {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		fmt.Fprintf(w, "  %-16s %d\n", k, kindCount[k])
	}
	grid := s.Grid()
	sort.Slice(grid, func(i, j int) bool { return grid[i].Name < grid[j].Name })
	fmt.Fprintf(w, "grid: %d entities\n", len(grid))
	for _, in := range grid {
		fmt.Fprintf(w, "  %-10s %-28s dist %12.0f m\n", in.Kind, in.Name, in.Distance)
	}
}

// cmdDescribe prints the definition of any content ID as pretty JSON —
// introspection tooling for content authors and AI agents ("what exactly is
// mod-emp-turret?"). Unknown IDs exit 1 with near-miss suggestions.
func cmdDescribe(args []string, stdout, stderr io.Writer) int {
	fs := newFlagSet("describe", stderr)
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if fs.NArg() != 1 {
		fmt.Fprintln(stderr, "entieve describe: want exactly one content id")
		return 2
	}
	id := fs.Arg(0)
	c, err := content.Load(content.FS())
	if err != nil {
		fmt.Fprintf(stderr, "entieve describe: load content: %v\n", err)
		return 1
	}
	def, err := describeLookup(c, id)
	if err != nil {
		fmt.Fprintf(stderr, "entieve describe: %v\n", err)
		return 1
	}
	out, err := json.MarshalIndent(def, "", "  ")
	if err != nil {
		fmt.Fprintf(stderr, "entieve describe: %v\n", err)
		return 1
	}
	fmt.Fprintln(stdout, string(out))
	return 0
}

// describeLookup resolves an ID against every content kind (exact match) and
// suggests near misses on failure.
func describeLookup(c *content.Content, id string) (any, error) {
	type bucket struct {
		kind string
		get  func(string) (any, bool)
		list func() []string
	}
	buckets := []bucket{
		{"item", func(s string) (any, bool) { d, ok := c.Item(content.ItemID(s)); return d, ok },
			func() []string {
				return ids(len(c.Items), func(yield func(string)) {
					for k := range c.Items {
						yield(string(k))
					}
				})
			}},
		{"ship", func(s string) (any, bool) { d, ok := c.Ship(content.ShipID(s)); return d, ok },
			func() []string {
				return ids(len(c.Ships), func(yield func(string)) {
					for k := range c.Ships {
						yield(string(k))
					}
				})
			}},
		{"module", func(s string) (any, bool) { d, ok := c.Module(content.ModuleID(s)); return d, ok },
			func() []string {
				return ids(len(c.Modules), func(yield func(string)) {
					for k := range c.Modules {
						yield(string(k))
					}
				})
			}},
		{"npc", func(s string) (any, bool) { d, ok := c.Npc(content.NpcID(s)); return d, ok },
			func() []string {
				return ids(len(c.NPCs), func(yield func(string)) {
					for k := range c.NPCs {
						yield(string(k))
					}
				})
			}},
		{"system", func(s string) (any, bool) { d, ok := c.System(content.SystemID(s)); return d, ok },
			func() []string {
				return ids(len(c.Systems), func(yield func(string)) {
					for k := range c.Systems {
						yield(string(k))
					}
				})
			}},
		{"world", func(s string) (any, bool) { d, ok := c.World(content.WorldID(s)); return d, ok },
			func() []string {
				return ids(len(c.Worlds), func(yield func(string)) {
					for k := range c.Worlds {
						yield(string(k))
					}
				})
			}},
		{"contract", func(s string) (any, bool) { d, ok := c.Contract(content.ContractID(s)); return d, ok },
			func() []string {
				return ids(len(c.Contracts), func(yield func(string)) {
					for k := range c.Contracts {
						yield(string(k))
					}
				})
			}},
	}
	var all []string
	for _, b := range buckets {
		if def, ok := b.get(id); ok {
			return def, nil
		}
		all = append(all, b.list()...)
	}
	// Near misses: prefix or substring matches make agent iteration fast.
	var near []string
	lower := strings.ToLower(id)
	for _, cand := range all {
		if strings.Contains(strings.ToLower(cand), lower) {
			near = append(near, cand)
		}
	}
	sort.Strings(near)
	if len(near) > 8 {
		near = near[:8]
	}
	if len(near) > 0 {
		return nil, fmt.Errorf("unknown id %q (did you mean: %s?)", id, strings.Join(near, ", "))
	}
	return nil, fmt.Errorf("unknown id %q (%d known ids across items/ships/modules/npcs/systems/worlds/contracts)", id, len(all))
}

// ids collects a range-over-func style iterator into a slice.
func ids(n int, each func(yield func(string))) []string {
	out := make([]string, 0, n)
	each(func(s string) { out = append(out, s) })
	return out
}

func readSave(path string) (*sim.Save, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var sv sim.Save
	if err := json.Unmarshal(data, &sv); err != nil {
		return nil, fmt.Errorf("parse %s: %w", filepath.Base(path), err)
	}
	return &sv, nil
}

func defaultSavePath() string {
	dir, err := os.UserConfigDir()
	if err != nil {
		return "save.json"
	}
	return filepath.Join(dir, "entieve", "save.json")
}

// newFlagSet builds a flag set with a compact, predictable error style.
func newFlagSet(name string, stderr io.Writer) *flag.FlagSet {
	fs := flag.NewFlagSet(name, flag.ContinueOnError)
	fs.SetOutput(stderr)
	return fs
}
