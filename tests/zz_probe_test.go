package tests

// Temporary integration probe (deleted after diagnosis).

import (
	"strings"
	"testing"

	"github.com/entieve/entieve/content"
	"github.com/entieve/entieve/ecs"
	"github.com/entieve/entieve/script"
	"github.com/entieve/entieve/sim"
)

func TestZZProbeNPCAmmo(t *testing.T) {
	c, err := content.Load(content.FS())
	if err != nil {
		t.Fatal(err)
	}
	wd, _ := c.World("world-showcase")
	s, err := sim.New(c, wd, 42)
	if err != nil {
		t.Fatal(err)
	}
	steps, err := script.Parse(mustRead(t, "scripts/showcase.json"))
	if err != nil {
		t.Fatal(err)
	}
	shown := 0
	total := 0
	kinds := map[string]int{}
	script.RunH(s, steps, 58000, script.Hooks{
		OnEvent: func(ev sim.Event) {
			total++
			kinds[string(ev.Kind)]++
			if total <= 12 {
				t.Logf("sample [%6.2fs] %-12s %s", ev.Time, ev.Kind, ev.Text)
			}
			if !strings.Contains(ev.Text, "no charges") || shown >= 6 {
				return
			}
			shown++
			t.Logf("[%6.2fs] %s (actor)", ev.Time, ev.Text)
			a := ev.Actor
			if cg, ok := s.Cargos.Get(a); ok {
				t.Logf("   actor cargo=%v", cg.Stacks)
			} else {
				t.Logf("   actor has NO cargo component")
			}
			if nm, ok := s.Names.Get(a); ok {
				t.Logf("   actor name=%s alive=%v", nm.Value, s.World.Alive(a))
			}
			if f, ok := s.Fittings.Get(a); ok {
				t.Logf("   actor fit high=%v rt=%v", f.High, f.HighRT)
			}
		},
	})
	if shown == 0 {
		t.Log("no 'no charges' events in window")
	}
	t.Logf("total events=%d kinds=%v", total, kinds)
	var _ = ecs.Entity(0)
}

func mustRead(t *testing.T, rel string) []byte {
	t.Helper()
	b, err := readFile(rel)
	if err != nil {
		t.Fatal(err)
	}
	return b
}
