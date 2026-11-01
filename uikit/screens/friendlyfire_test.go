package screens

import (
	"testing"

	"github.com/entieve/entieve/ecs"
	"github.com/entieve/entieve/internal/uitest"
	"github.com/entieve/entieve/sim"
)

// TestFriendlyFireDialogConfirms: the confirm dialog is built on first open,
// names the target and the module, and its primary button re-dispatches the
// same CmdActivate with Confirm set. Cancel and × decline without a command.
func TestFriendlyFireDialogConfirms(t *testing.T) {
	k, cmds := newTestKit(t)
	pumpKit(t, k, 1280, 720)
	if k.friendlyFireWin != nil || k.FriendlyFireOpen() {
		t.Fatal("the confirm dialog was built at boot")
	}

	var target ecs.Entity
	for _, inf := range k.Sim.Grid() {
		if inf.Kind == sim.KindShip && inf.Entity != k.Sim.Player() {
			target = inf.Entity
			break
		}
	}
	if target.IsZero() {
		t.Fatal("fixture: no NPC ship on the grid")
	}
	info, _ := k.Sim.Info(target)
	slot := sim.SlotRef{Class: "high", Index: 2}
	k.OpenFriendlyFire(slot, target, "250mm Auto Cannon")
	pumpKit(t, k, 1280, 720)

	if !k.FriendlyFireOpen() {
		t.Fatal("OpenFriendlyFire did not open the dialog")
	}
	f := k.friendlyFireWin
	if f.target.Label != info.Name {
		t.Fatalf("target label = %q, want %q", f.target.Label, info.Name)
	}
	if f.module.Label != "250mm Auto Cannon" {
		t.Fatalf("module label = %q", f.module.Label)
	}
	// The primary button wears the owner's words exactly.
	if got := f.Confirm().Text().Label; got != "Confirm foolishness" {
		t.Fatalf("confirm label = %q, want %q", got, "Confirm foolishness")
	}
	if k.closes[f.Win()] == nil {
		t.Fatal("the dialog registered no × close")
	}

	uitest.FireButton(k.ui, f.Confirm())
	pumpKit(t, k, 1280, 720)
	if k.FriendlyFireOpen() {
		t.Fatal("Confirm left the dialog open")
	}
	if len(*cmds) != 1 {
		t.Fatalf("Confirm enqueued %d commands, want 1: %+v", len(*cmds), *cmds)
	}
	act, ok := (*cmds)[0].(sim.CmdActivate)
	if !ok || !act.Confirm || act.Slot != slot || act.Target != target {
		t.Fatalf("confirm command = %+v, want CmdActivate{%+v %v confirm}", (*cmds)[0], slot, target)
	}
}

// TestFriendlyFireDialogCancels: Cancel and a second open/close cycle send
// nothing.
func TestFriendlyFireDialogCancels(t *testing.T) {
	k, cmds := newTestKit(t)
	pumpKit(t, k, 1280, 720)
	var target ecs.Entity
	for _, inf := range k.Sim.Grid() {
		if inf.Kind == sim.KindShip && inf.Entity != k.Sim.Player() {
			target = inf.Entity
			break
		}
	}
	slot := sim.SlotRef{Class: "high", Index: 0}

	k.OpenFriendlyFire(slot, target, "Stasis Web")
	pumpKit(t, k, 1280, 720)
	uitest.FireButton(k.ui, k.friendlyFireWin.Cancel())
	pumpKit(t, k, 1280, 720)
	if k.FriendlyFireOpen() {
		t.Fatal("Cancel left the dialog open")
	}
	if len(*cmds) != 0 {
		t.Fatalf("Cancel enqueued %d commands, want 0", len(*cmds))
	}

	// A second launch re-targets the same single instance.
	k.OpenFriendlyFire(slot, target, "Warp Scrambler")
	pumpKit(t, k, 1280, 720)
	if !k.FriendlyFireOpen() || k.friendlyFireWin.module.Label != "Warp Scrambler" {
		t.Fatal("a second launch did not re-target the dialog")
	}
	k.CloseFriendlyFire()
	if k.FriendlyFireOpen() {
		t.Fatal("CloseFriendlyFire left the dialog open")
	}
}
