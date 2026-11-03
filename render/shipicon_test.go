package render

import (
	"fmt"
	"os"
	"strings"
	"testing"

	"github.com/entieve/entieve/content"
	"github.com/entieve/entieve/core"
	"github.com/entieve/entieve/sim"
)

// glyphSig is a glyph's identity: its unit points, printed exactly.
func glyphSig(g []core.Vec2) string {
	var b strings.Builder
	for _, p := range g {
		fmt.Fprintf(&b, "%.3f,%.3f;", p.X, p.Y)
	}
	return b.String()
}

// TestEveryShippedClassOwnsOneGlyph is the world icon's content pin: every
// hull shipped draws its class's glyph, all hulls of a class share one
// silhouette (the three rat castes included), and no two classes collide. A
// new class with no glyph falls back to the frigate mark and fails here.
func TestEveryShippedClassOwnsOneGlyph(t *testing.T) {
	c, err := content.Load(content.FS())
	if err != nil {
		t.Fatalf("content.Load: %v", err)
	}
	byClass := map[string]string{}
	owner := map[string]string{}
	for _, def := range c.Ships {
		sig := glyphSig(classGlyph(def.Class))
		if prev, ok := byClass[def.Class]; ok && prev != sig {
			t.Errorf("class %q draws two glyphs: %s and %s", def.Class, prev, sig)
		}
		byClass[def.Class] = sig
		if other, ok := owner[sig]; ok && other != def.Class {
			t.Errorf("classes %q and %q share one glyph (%s)", other, def.Class, sig)
		}
		owner[sig] = def.Class
	}
	if len(byClass) == 0 {
		t.Fatal("no ships loaded")
	}
	// A class that resolves to the default mark would collide with the
	// frigate's glyph; the castes stay one class, so they stay identical.
	for class := range byClass {
		if class != "frigate" && glyphSig(classGlyph(class)) == glyphSig(classGlyph("")) {
			t.Errorf("class %q has no glyph of its own (falls back to the frigate mark)", class)
		}
	}
}

// TestShipIconFitsItsBracket: the glyph stays inside the square slot the hull
// footprint gives it (bracketHalfSize draws at 1.5×), so the icon never
// covers its own bracket or a neighbour's.
func TestShipIconFitsItsBracket(t *testing.T) {
	for _, class := range []string{"frigate", "destroyer", "industrial", "barge", "rat"} {
		g := classGlyph(class)
		if len(g) < 3 {
			t.Fatalf("%s: %d points, not a glyph", class, len(g))
		}
		for _, p := range g {
			if x, y := p.X, p.Y; x < -1.2 || x > 1.2 || y < -1.2 || y > 1.2 {
				t.Errorf("%s: point (%.2f,%.2f) leaves the unit slot", class, x, y)
			}
		}
	}
}

// TestShipIconNeverRotates pins the owner's rule at the only place it can be
// checked headless (pixels need a display): the draw body takes the class
// glyph and blits it as-is: no facing, no rotation, no hull sprite. The same
// hull at two headings therefore issues the same draw calls.
func TestShipIconNeverRotates(t *testing.T) {
	src, err := os.ReadFile("render.go")
	if err != nil {
		t.Skipf("source not available: %v", err)
	}
	body := string(src)
	i := strings.Index(body, "func (r *Renderer) drawShip")
	if i < 0 {
		t.Fatal("drawShip not found in render.go")
	}
	body = body[i:]
	if j := strings.Index(body, "\n// shipTint"); j >= 0 {
		body = body[:j] // the next function in the file
	}
	if j := strings.Index(body, "\nfunc (r *Renderer) orDef"); j >= 0 {
		body = body[:j]
	}
	for _, forbidden := range []string{"o.facing", ".Rotate(", `spriteFor("hulls/`, "RestoreRNG"} {
		if strings.Contains(body, forbidden) {
			t.Errorf("drawShip draws %q: the world icon must be heading-blind and hull-art-free", forbidden)
		}
	}
	if !strings.Contains(body, "classGlyph(") {
		t.Error("drawShip does not draw the class glyph")
	}
}

// TestShipIconTintsFollowTheBracket: affiliation comes from the world's own
// rule, so the icon can never disagree with the bracket around it.
func TestShipIconTintsFollowTheBracket(t *testing.T) {
	cases := []struct {
		name string
		o    gridObj
		want string
	}{
		{"player", gridObj{info: sim.Info{Kind: sim.KindPlayer}}, "player cyan"},
		{"hostile ship", gridObj{info: sim.Info{Kind: sim.KindShip, Hostile: true}}, "hostile red"},
		{"neutral ship", gridObj{info: sim.Info{Kind: sim.KindShip}}, "neutral orange"},
	}
	for _, tc := range cases {
		got := shipTint(tc.o)
		want := bracketColor(tc.o.info.Kind, tc.o.info.Hostile, tc.o.info.Kind == sim.KindPlayer)
		if got != want {
			t.Errorf("%s: tint %v, bracket %v", tc.name, got, want)
		}
	}
	if shipTint(cases[0].o) != colPlayer || shipTint(cases[1].o) != colHostile ||
		shipTint(cases[2].o) != colNeutral {
		t.Errorf("tints drifted: %v %v %v", shipTint(cases[0].o), shipTint(cases[1].o), shipTint(cases[2].o))
	}
}
