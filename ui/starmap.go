package ui

import (
	"image/color"
	"sort"

	"github.com/entieve/entieve/content"
	"github.com/entieve/entieve/core"
	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/vector"
)

// SystemNode is one starmap system with its graph position (light-years).
type SystemNode struct {
	ID       content.SystemID
	Pos      core.Vec2
	Security string
}

// systemNodes lists all systems in deterministic (ID-sorted) order, merging
// universe positions with any systems the universe list does not mention.
func (u *UI) systemNodes() []SystemNode {
	var out []SystemNode
	seen := map[content.SystemID]bool{}
	if u.c.Universe != nil {
		for _, s := range u.c.Universe.Systems {
			out = append(out, SystemNode{ID: s.ID, Pos: s.Position, Security: s.Security})
			seen[s.ID] = true
		}
	}
	for id, s := range u.c.Systems {
		if !seen[id] {
			out = append(out, SystemNode{ID: id, Pos: s.Position, Security: s.Security})
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out
}

// MapNode is a system's projected starmap position (screen pixels).
type MapNode struct {
	ID content.SystemID
	X  float64
	Y  float64
}

// StarmapNodes projects system graph positions into the given panel rect,
// preserving aspect ratio and centering the bounds. Pure and deterministic.
func StarmapNodes(r Rect, systems []SystemNode) []MapNode {
	nodes := make([]MapNode, len(systems))
	if len(systems) == 0 {
		return nodes
	}
	minX, maxX := systems[0].Pos.X, systems[0].Pos.X
	minY, maxY := systems[0].Pos.Y, systems[0].Pos.Y
	for _, s := range systems {
		if s.Pos.X < minX {
			minX = s.Pos.X
		}
		if s.Pos.X > maxX {
			maxX = s.Pos.X
		}
		if s.Pos.Y < minY {
			minY = s.Pos.Y
		}
		if s.Pos.Y > maxY {
			maxY = s.Pos.Y
		}
	}
	const pad = 48
	spanX, spanY := maxX-minX, maxY-minY
	availW, availH := r.W-2*pad, r.H-2*pad
	scale := 1.0
	if spanX > 0 && spanY > 0 {
		scale = min(availW/spanX, availH/spanY)
	} else if spanX > 0 {
		scale = availW / spanX
	} else if spanY > 0 {
		scale = availH / spanY
	}
	offX := r.X + pad + (availW-spanX*scale)/2
	offY := r.Y + pad + (availH-spanY*scale)/2
	for i, s := range systems {
		nodes[i] = MapNode{
			ID: s.ID,
			X:  offX + (s.Pos.X-minX)*scale,
			Y:  offY + (s.Pos.Y-minY)*scale,
		}
	}
	return nodes
}

// securityColor colors starmap nodes by security status.
func securityColor(sec string) color.RGBA {
	switch sec {
	case "high":
		return ColGood
	case "low":
		return ColWarn
	case "null":
		return ColDanger
	default:
		return ColDim
	}
}

// DrawStarmap renders the starmap overlay (used by the game's map scene).
func (u *UI) DrawStarmap(dst *ebiten.Image) {
	reg := u.reg()
	r := reg.Starmap
	Panel(dst, r)
	title := "Starmap"
	if u.c.Universe != nil {
		title = "Starmap — " + u.c.Universe.Region + " / " + u.c.Universe.Constellation
	}
	PanelTitle(dst, r, title)

	// Gate lines from every system's gate celestials.
	byID := map[content.SystemID]MapNode{}
	sysNodes := u.systemNodes()
	proj := StarmapNodes(r, sysNodes)
	for i, s := range sysNodes {
		byID[s.ID] = proj[i]
	}
	cur := content.SystemID("")
	if sys := u.s.System(); sys != nil {
		cur = sys.ID
	}

	for _, s := range sysNodes {
		def, ok := u.c.System(s.ID)
		if !ok {
			continue
		}
		a := byID[s.ID]
		for i := range def.Celestials {
			g := def.Celestials[i].Gate
			if g == nil {
				continue
			}
			b, ok := byID[g.ToSystem]
			if !ok {
				continue
			}
			col := color.RGBA{90, 100, 118, 160}
			if s.ID == cur || g.ToSystem == cur {
				col = withAlpha(ColAccent, 170)
			}
			vector.StrokeLine(dst, float32(a.X), float32(a.Y), float32(b.X), float32(b.Y), 1, col, true)
		}
	}

	// System nodes + labels.
	for _, s := range sysNodes {
		n := byID[s.ID]
		col := securityColor(s.Security)
		rad := float32(5)
		if s.ID == cur {
			rad = 7
			vector.StrokeCircle(dst, float32(n.X), float32(n.Y), 12, 1.5, ColAccent, true)
		}
		if s.ID == u.mapSel && s.ID != cur {
			vector.StrokeCircle(dst, float32(n.X), float32(n.Y), 11, 1.5, ColText, true)
		}
		vector.DrawFilledCircle(dst, float32(n.X), float32(n.Y), rad, col, true)
		labelCol := ColDim
		if s.ID == cur {
			labelCol = ColAccent
		} else if s.ID == u.mapSel {
			labelCol = ColText
		}
		TextCentered(dst, string(s.ID), n.X, n.Y+14, 10, labelCol)
	}

	// Selected-system info footer (starmap is informational; travel happens
	// through the gates in-world).
	foot := r.Bottom() - 64
	vector.StrokeLine(dst, float32(r.X+8), float32(foot-8), float32(r.Right()-8), float32(foot-8),
		1, ColBorder, false)
	if def, ok := u.c.System(u.mapSel); ok && u.mapSel != "" {
		Text(dst, def.Name+"  ("+string(def.ID)+")", r.X+12, foot, 13, ColText)
		Text(dst, "security: "+def.Security+"   constellation: "+def.Constellation,
			r.X+12, foot+18, 10, securityColor(def.Security))
		var conns []string
		for i := range def.Celestials {
			if g := def.Celestials[i].Gate; g != nil {
				conns = append(conns, string(g.ToSystem))
			}
		}
		if len(conns) > 0 {
			Text(dst, "gates to: "+joinIDs(conns), r.X+12, foot+34, 10, ColDim)
		}
	} else {
		Text(dst, "Click a system for details. Travel: fly to a stargate and press J.",
			r.X+12, foot, 11, ColDim)
	}
}

// ClickStarmap selects the system node under the cursor; reports whether the
// click landed inside the starmap panel.
func (u *UI) ClickStarmap(x, y float64) bool {
	reg := u.reg()
	r := reg.Starmap
	if !r.Contains(x, y) {
		return false
	}
	for _, n := range StarmapNodes(r, u.systemNodes()) {
		dx, dy := x-n.X, y-n.Y
		if dx*dx+dy*dy <= 14*14 {
			u.mapSel = n.ID
			return true
		}
	}
	return true // consumed by the panel even if no node was hit
}

// joinIDs renders a list of system IDs as "a, b, c".
func joinIDs(ids []string) string {
	out := ""
	for i, id := range ids {
		if i > 0 {
			out += ", "
		}
		out += id
	}
	return out
}
