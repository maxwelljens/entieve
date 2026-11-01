package screens

import (
	"strings"
	"testing"

	"github.com/ebitenui/ebitenui/widget"
	"github.com/entieve/entieve/ecs"
	"github.com/entieve/entieve/sim"
)

// TestHudCombatFeed: the feed is one line per player-relevant combat event
// (a hit taken red, a hit dealt blue, an applied EWAR named both ways), it
// expires after hudFeedFrames, it keeps only hudFeedLines lines, it
// hides with nothing to say, and it is an overlay above the ship status: the
// pair's own rect does not move when a line appears.
func TestHudCombatFeed(t *testing.T) {
	env, _ := newChromeEnv(t)
	h := NewHud(env)
	c := newChromeUI(env)
	c.AddPanel(h.Container(), h)
	c.root.AddChild(h.Feed().Container())
	c.Frame(t, 1280, 720)
	pairBefore := h.pair.GetWidget().Rect
	strip := h.strip.GetWidget().Rect
	rows := h.feed.rows.GetWidget()

	if rows.GetVisibility() != widget.Visibility_Hide {
		t.Error("the feed shows with no line")
	}

	// A hit taken, a hit dealt and an applied scramble.
	giver := ecs.Entity(0)
	for _, inf := range env.Sim.Grid() {
		if inf.Entity != env.Sim.Player() && giver == 0 {
			giver = inf.Entity
		}
	}
	if giver == 0 {
		t.Fatal("fixture: no other entity")
	}
	p := env.Sim.Player()
	cv := h.cv
	giverName := "Kuštor"
	h.Push([]sim.Event{
		{Kind: sim.EvCombat, Phase: sim.CombatDamage, Actor: giver, Target: p, Value: 210, Name: giverName},
		{Kind: sim.EvCombat, Phase: sim.CombatDamage, Actor: p, Target: giver, Value: 42},
		{Kind: sim.EvCombat, Phase: sim.CombatScram, Actor: giver, Target: p, Name: giverName},
	})
	// The dealt-damage line names the target through Info.
	name, ok := cv.Info(giver)
	if !ok {
		t.Fatal("fixture: the target has no Info")
	}
	c.Frame(t, 1280, 720)

	if rows.GetVisibility() != widget.Visibility_Show {
		t.Fatal("the feed stays hidden with three live lines")
	}
	want := []hudFeedLine{
		{pre: giverName + " hits you for ", key: "210", loss: true},
		{pre: "You hit " + name.Name + " for ", key: "42"},
		{key: "Scrambled", post: " by " + giverName, loss: true},
	}
	for i, w := range want {
		row := h.feed.rowsAt[hudFeedLines-len(want)+i]
		if row.pre.Label != w.pre || row.key.Label != w.key || row.post.Label != w.post {
			t.Errorf("line %d = %q/%q/%q, want %q/%q/%q", i,
				row.pre.Label, row.key.Label, row.post.Label, w.pre, w.key, w.post)
		}
		if row.staged.loss != w.loss {
			t.Errorf("line %d loss = %v, want %v (red taking, blue dealing)", i, row.staged.loss, w.loss)
		}
	}
	// The feed is an overlay: the bottom pair kept its rect, and the feed sits
	// clear above the strip (which is above the status).
	if got := h.pair.GetWidget().Rect; got != pairBefore {
		t.Errorf("the pair moved %v → %v when the feed appeared (it must not grow the column)", pairBefore, got)
	}
	fr := h.feed.col.GetWidget().Rect
	if fr.Empty() || fr.Max.Y > strip.Min.Y {
		t.Errorf("feed %v is not above the strip %v", fr, strip)
	}

	// Expiry: the lines drop after their frames run out and the feed hides.
	for i := 0; i <= hudFeedFrames; i++ {
		h.Sync()
	}
	c.Frame(t, 1280, 720)
	if rows.GetVisibility() != widget.Visibility_Hide {
		t.Error("the feed outlived its window")
	}
	for i, row := range h.feed.rowsAt {
		if row.box.GetWidget().GetVisibility() != widget.Visibility_Hide {
			t.Errorf("expired line %d still shows %q", i, row.key.Label)
		}
	}

	// Cap: a burst keeps only the newest hudFeedLines.
	var burst []sim.Event
	for i := 0; i < hudFeedLines+2; i++ {
		burst = append(burst, sim.Event{Kind: sim.EvCombat, Phase: sim.CombatDamage,
			Actor: giver, Target: p, Value: float64(10 + i), Name: giverName})
	}
	h.Push(burst)
	if len(h.feed.lines) != hudFeedLines {
		t.Fatalf("feed kept %d lines, want the cap %d", len(h.feed.lines), hudFeedLines)
	}
	c.Frame(t, 1280, 720)
	last := h.feed.rowsAt[hudFeedLines-1]
	if want := hudHP(10 + hudFeedLines + 1); last.key.Label != want {
		t.Errorf("newest line = %q, want %q (the burst's last)", last.key.Label, want)
	}

	// A destroyed target reads its name from the event (Info cannot answer
	// for a dead hull): the kill is the player's, so it is blue.
	h.feed.lines = nil
	h.Push([]sim.Event{{Kind: sim.EvDestroyed, Actor: giver, Target: p, Name: giverName,
		Text: giverName + " destroyed"}})
	c.Frame(t, 1280, 720)
	row := h.feed.rowsAt[hudFeedLines-1]
	if row.key.Label != giverName+" destroyed" || row.staged.loss {
		t.Errorf("kill line = %q (loss %v), want %q in the gain colour",
			row.key.Label, row.staged.loss, giverName+" destroyed")
	}
	// And the player's own death is a loss.
	h.feed.lines = nil
	h.Push([]sim.Event{{Kind: sim.EvDestroyed, Actor: p, Text: "Your Sloop was destroyed"}})
	c.Frame(t, 1280, 720)
	row = h.feed.rowsAt[hudFeedLines-1]
	if !strings.Contains(row.key.Label, "destroyed") || !row.staged.loss {
		t.Errorf("own-death line = %q (loss %v), want a red destruction", row.key.Label, row.staged.loss)
	}
}

// TestHudCombatFeedIgnoresOtherEvents: the feed speaks for player-relevant
// combat only; a log line, a market fill or somebody else's fight adds
// nothing.
func TestHudCombatFeedIgnoresOtherEvents(t *testing.T) {
	env, _ := newChromeEnv(t)
	h := NewHud(env)
	p := env.Sim.Player()
	var a, b ecs.Entity
	for _, inf := range env.Sim.Grid() {
		if inf.Entity == p {
			continue
		}
		if a == 0 {
			a = inf.Entity
		} else if b == 0 {
			b = inf.Entity
			break
		}
	}
	h.Push([]sim.Event{
		{Kind: sim.EvLog, Text: "something happened"},
		{Kind: sim.EvMarket, Text: "sold"},
		{Kind: sim.EvCombat, Phase: sim.CombatDamage, Actor: a, Target: b, Value: 12, Name: "x"},
	})
	if n := len(h.feed.lines); n != 0 {
		t.Errorf("feed staged %d line(s) from unrelated events, want none", n)
	}
}
