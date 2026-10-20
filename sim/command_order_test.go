package sim

import (
	"reflect"
	"testing"

	"github.com/entieve/entieve/content"
	"github.com/entieve/entieve/ecs"
)

// ---------------------------------------------------------------------------
// F6 — command.go fans commands into four defer buckets that the Step runs
// in a fixed order (loot -> economy -> travel -> mission), so the outcome of
// same-tick command combinations is a contract of Step, not of queue order.
// ---------------------------------------------------------------------------

// eventLines renders events as exact "kind|phase|text" lines so a table row
// can pin the whole tick's outcome, not just one event.
func eventLines(evs []Event) []string {
	out := make([]string, 0, len(evs))
	for _, ev := range evs {
		out = append(out, string(ev.Kind)+"|"+ev.Phase+"|"+ev.Text)
	}
	return out
}

func TestSameTickCommandOrdering(t *testing.T) {
	stationName := "Docked at Haven Rest"
	cases := []struct {
		name string
		// docked starts the player docked at Haven Rest; wreck places a
		// one-stack wreck on top of the player before the tick.
		docked, wreck bool
		cmds          func(station, wreck ecs.Entity) []Command
		want          []string
		wantDocked    bool
	}{
		{
			name:   "dock queued before buy: economy runs while still undocked",
			docked: false,
			cmds: func(station, _ ecs.Entity) []Command {
				return []Command{
					CmdDock{Station: station},
					CmdBuy{Item: "ammo-emp-s", Qty: 10},
				}
			},
			// travel (13) runs after economy (12): the buy sees no station.
			want: []string{
				"log||You must be docked to use station services",
				"docked||" + stationName,
			},
			wantDocked: true,
		},
		{
			name:   "buy queued before dock: bucket order beats queue order",
			docked: false,
			cmds: func(station, _ ecs.Entity) []Command {
				return []Command{
					CmdBuy{Item: "ammo-emp-s", Qty: 10},
					CmdDock{Station: station},
				}
			},
			want: []string{
				"log||You must be docked to use station services",
				"docked||" + stationName,
			},
			wantDocked: true,
		},
		{
			name:   "dock and undock in one tick: both apply, travel order preserved",
			docked: false,
			cmds: func(station, _ ecs.Entity) []Command {
				return []Command{CmdDock{Station: station}, CmdUndock{}}
			},
			want: []string{
				"docked||" + stationName,
				"undocked||Undocked from Haven Rest",
			},
			wantDocked: false,
		},
		{
			name:   "undock queued before accept: mission runs after travel",
			docked: true,
			cmds: func(_, _ ecs.Entity) []Command {
				return []Command{
					CmdUndock{},
					CmdAcceptMission{ID: "contract-ghost"},
				}
			},
			want: []string{
				"undocked||Undocked from Haven Rest",
				"mission|refused|Contract refused: You must be docked to accept a contract",
			},
			wantDocked: false,
		},
		{
			name:   "accept queued before undock: mission still runs last",
			docked: true,
			cmds: func(_, _ ecs.Entity) []Command {
				return []Command{
					CmdAcceptMission{ID: "contract-ghost"},
					CmdUndock{},
				}
			},
			want: []string{
				"undocked||Undocked from Haven Rest",
				"mission|refused|Contract refused: You must be docked to accept a contract",
			},
			wantDocked: false,
		},
		{
			name:   "dock queued before loot: loot still runs first",
			docked: false,
			wreck:  true,
			cmds: func(station, wreck ecs.Entity) []Command {
				return []Command{CmdDock{Station: station}, CmdLoot{Wreck: wreck}}
			},
			want: []string{
				"looted||Looted 1 x Alloy Scrap",
				"docked||" + stationName,
			},
			wantDocked: true,
		},
		{
			name:   "loot queued before dock: same outcome as the reverse",
			docked: false,
			wreck:  true,
			cmds: func(station, wreck ecs.Entity) []Command {
				return []Command{CmdLoot{Wreck: wreck}, CmdDock{Station: station}}
			},
			want: []string{
				"looted||Looted 1 x Alloy Scrap",
				"docked||" + stationName,
			},
			wantDocked: true,
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			s := newTestSim(t)
			station := entityByName(s, "Haven Rest")
			if station.IsZero() {
				t.Fatal("no station entity")
			}
			if c.docked {
				dockAtHaven(t, s)
			}
			var wreck ecs.Entity
			if c.wreck {
				wreck = s.create()
				s.Names.Set(wreck, Name{Value: "Test Wreck"})
				s.Transforms.Set(wreck, Transform{Pos: playerPos(s)})
				s.Wrecks.Set(wreck, Wreck{
					Loot:      []content.CargoStack{{Item: "loot-alloy-scrap", Qty: 1}},
					ExpiresAt: s.time + 300,
				})
			}
			drain(s)

			for _, cmd := range c.cmds(station, wreck) {
				s.Queue(cmd)
			}
			s.Step()

			if got := eventLines(drain(s)); !reflect.DeepEqual(got, c.want) {
				t.Fatalf("tick events:\n got %q\nwant %q", got, c.want)
			}
			if s.playerDocked() != c.wantDocked {
				t.Fatalf("docked=%v want %v", s.playerDocked(), c.wantDocked)
			}
			// No row buys anything: a refused purchase must not move funds.
			if s.Wallet() != 100000 {
				t.Fatalf("wallet=%v want 100000", s.Wallet())
			}
			if n := hangarCount(s, "ammo-emp-s"); n != 0 {
				t.Fatalf("hangar holds %d charges, want 0", n)
			}
		})
	}
}

// TestDockedThisTickSurvivesSameTickUndock pins the mission-side half of the
// travel contract: execDock stamps dockedThisTick for the auto-claim
// trigger, an undock in the same Step does not clear it (systemMission runs
// after travel, sees the docking event, then resets the marker).
func TestDockedThisTickSurvivesSameTickUndock(t *testing.T) {
	s := newTestSim(t)
	station := entityByName(s, "Haven Rest")
	if station.IsZero() {
		t.Fatal("no station entity")
	}
	s.Queue(CmdDock{Station: station})
	s.Queue(CmdUndock{})
	s.runCommands()
	s.systemTravel()
	if s.playerDocked() {
		t.Fatal("fixture: player still docked after undock")
	}
	if s.dockedThisTick != "cel-station-a" {
		t.Fatalf("dockedThisTick=%q after undock, want cel-station-a (dock stamp survives)", s.dockedThisTick)
	}
	s.systemMission()
	if s.dockedThisTick != "" {
		t.Fatalf("dockedThisTick=%q after mission, want cleared", s.dockedThisTick)
	}
}
