package game

import (
	"testing"

	"github.com/entieve/entieve/sim"
	"github.com/entieve/entieve/uikit/widgets"
)

// TestStationIntentReachesSim: the board's verbs are typed intents; the
// game's executor maps Accept/Abandon onto the sim's commands through the
// one commandFor table. The panel-side emission (which intent a pressed
// row fires) is pinned in uikit/screens on the stub view.
func TestStationIntentReachesSim(t *testing.T) {
	g := dockedShowcaseGame(t, 1280, 720)
	var got []sim.Command
	g.enqueue = func(c sim.Command) { got = append(got, c) }

	g.Do(widgets.Accept{Contract: "contract-solace-cleanup-1"})
	g.Do(widgets.Abandon{Contract: "contract-solace-cleanup-1"})
	if len(got) != 2 {
		t.Fatalf("intents enqueued %d commands, want 2", len(got))
	}
	if cmd, ok := got[0].(sim.CmdAcceptMission); !ok || cmd.ID != "contract-solace-cleanup-1" {
		t.Errorf("Accept enqueued %#v, want CmdAcceptMission{contract-solace-cleanup-1}", got[0])
	}
	if cmd, ok := got[1].(sim.CmdAbandonMission); !ok || cmd.ID != "contract-solace-cleanup-1" {
		t.Errorf("Abandon enqueued %#v, want CmdAbandonMission{contract-solace-cleanup-1}", got[1])
	}
}
