package widgets

import (
	"image"
	"testing"

	"github.com/entieve/entieve/content"
	"github.com/entieve/entieve/ecs"
	"github.com/entieve/entieve/sim"
)

// Intent dispatch: every intent maps onto the sim command its handler
// queues, and only the intents a host executes itself (the session's and
// the Esc menu's) map onto nothing. This is THE intent→command table —
// Game.Do runs every intent through the kit's executor, which reads it.

func TestCommandForMapsEveryCommandIntent(t *testing.T) {
	e := ecs.Entity(7)
	slot := sim.SlotRef{Class: content.SlotHigh, Index: 2}
	cases := []struct {
		name string
		in   Intent
		want sim.Command
	}{
		{"buy", Buy{Item: "ammo-emp-s", Qty: 3}, sim.CmdBuy{Item: "ammo-emp-s", Qty: 3}},
		{"sell", Sell{Item: "min-ore", Qty: 1}, sim.CmdSell{Item: "min-ore", Qty: 1}},
		{"place order", PlaceOrder{Item: "min-ore", Side: sim.OrderBuy, Price: 4.5, Qty: 2},
			sim.CmdPlaceOrder{Item: "min-ore", Side: sim.OrderBuy, Price: 4.5, Qty: 2}},
		{"cancel order", CancelOrder{ID: 9}, sim.CmdCancelOrder{ID: 9}},
		{"transfer", Transfer{Item: "min-ore", Qty: 5, From: "cargo", To: "hangar"},
			sim.CmdTransfer{Item: "min-ore", Qty: 5, From: "cargo", To: "hangar"}},
		{"fit", Fit{Module: "mod-autocannon-s", Slot: slot},
			sim.CmdFitModule{Item: "mod-autocannon-s", Slot: slot}},
		{"unequip", Unequip{Slot: slot}, sim.CmdUnfitModule{Slot: slot}},
		{"assemble", Assemble{Ship: "ship-sloop"}, sim.CmdAssemble{Ship: "ship-sloop"}},
		{"start job", StartJob{Recipe: "rec-ammo-emp-s", Runs: 2},
			sim.CmdStartJob{Recipe: "rec-ammo-emp-s", Runs: 2}},
		{"collect job", CollectJob{Job: 9}, sim.CmdCollectJob{ID: 9}},
		{"accept", Accept{Contract: "contract-solace-cleanup-1"},
			sim.CmdAcceptMission{ID: "contract-solace-cleanup-1"}},
		{"abandon", Abandon{Contract: "contract-solace-cleanup-1"},
			sim.CmdAbandonMission{ID: "contract-solace-cleanup-1"}},
		{"activate", Activate{Slot: slot, Target: e}, sim.CmdActivate{Slot: slot, Target: e}},
		{"deactivate", Deactivate{Slot: slot}, sim.CmdDeactivate{Slot: slot}},
		{"cycle charge", CycleCharge{Slot: slot, Item: "ammo-fusion-s"},
			sim.CmdSetAmmo{Slot: slot, Item: "ammo-fusion-s"}},
		{"approach", Approach{Entity: e}, sim.CmdApproach{Target: e}},
		{"lock", Lock{Entity: e}, sim.CmdLock{Target: e}},
		{"stop", Stop{}, sim.CmdStop{}},
		{"dock", Dock{Entity: e}, sim.CmdDock{Station: e}},
		{"jump", Jump{Entity: e}, sim.CmdJump{Gate: e}},
		{"loot", Loot{Entity: e}, sim.CmdLoot{Wreck: e}},
		{"mine", Mine{Entity: e, Slot: slot}, sim.CmdMine{Asteroid: e, Slot: slot}},
		{"undock", Undock{}, sim.CmdUndock{}},
	}
	for _, c := range cases {
		got, ok := commandFor(c.in, 0, 0)
		if !ok {
			t.Errorf("%s: commandFor reports no command for %#v", c.name, c.in)
			continue
		}
		if got != c.want {
			t.Errorf("%s: command = %#v, want %#v", c.name, got, c.want)
		}
	}
}

// TestCommandForReadsRangeAtDispatch: WarpTo/Orbit carry no range — the
// executor reads the Selected Item selectors when the intent fires, so
// the declared intent never freezes a stale landing distance.
func TestCommandForReadsRangeAtDispatch(t *testing.T) {
	e := ecs.Entity(3)
	if c, ok := commandFor(WarpTo{Entity: e}, 12000, 0); !ok ||
		c != (sim.CmdWarpTo{Target: e, Range: 12000}) {
		t.Errorf("WarpTo → %#v, want CmdWarpTo at the passed warp range", c)
	}
	if c, ok := commandFor(Orbit{Entity: e}, 0, 2500); !ok ||
		c != (sim.CmdOrbit{Target: e, Range: 2500}) {
		t.Errorf("Orbit → %#v, want CmdOrbit at the passed orbit range", c)
	}
}

// TestCommandForSkipsHostIntents: the session's and the Esc menu's
// intents map onto no command — their handlers live in the host.
func TestCommandForSkipsHostIntents(t *testing.T) {
	host := []Intent{
		ShowInfo{Ref: ItemRef("ammo-emp-s")},
		OpenMenu{At: image.Pt(1, 2)},
		OpenOrder{Item: "ammo-emp-s", Side: sim.OrderSell, Qty: 1},
		Toggle{Screen: "Market"},
		Save{}, Load{}, Quit{},
	}
	for _, i := range host {
		if cmd, ok := commandFor(i, 0, 0); ok {
			t.Errorf("%T mapped onto %#v, want no command", i, cmd)
		}
	}
}
