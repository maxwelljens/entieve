package render

import (
	"image/color"
	"math"

	"github.com/entieve/entieve/content"
	"github.com/entieve/entieve/core"
	"github.com/entieve/entieve/ecs"
	"github.com/entieve/entieve/format"
	"github.com/entieve/entieve/sim"
	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/vector"
)

// Palette constants shared by brackets and effects.
var (
	colPlayer  = color.RGBA{127, 227, 255, 255} // white/cyan
	colHostile = color.RGBA{255, 85, 85, 255}   // hostile NPC red
	colNeutral = color.RGBA{255, 159, 67, 255}  // neutral NPC orange
	colStation = color.RGBA{111, 168, 255, 255} // blue-ish
	colGate    = color.RGBA{127, 224, 143, 255} // green-ish
	colAst     = color.RGBA{153, 160, 170, 255} // asteroid grey
	colWreck   = color.RGBA{255, 215, 95, 255}  // wreck yellow
	colCel     = color.RGBA{143, 184, 255, 255} // celestial blue
	colAnomaly = color.RGBA{208, 128, 255, 255} // anomaly purple
	colSelect  = color.RGBA{255, 180, 84, 255}  // selection amber
	colLock    = color.RGBA{140, 230, 255, 255} // lock cyan
)

// bracketColor returns the EVE-style bracket color for a grid entity.
func bracketColor(kind string, hostile, isPlayer bool) color.RGBA {
	if isPlayer {
		return colPlayer
	}
	switch kind {
	case sim.KindShip:
		if hostile {
			return colHostile
		}
		return colNeutral
	case sim.KindStation:
		return colStation
	case sim.KindGate:
		return colGate
	case sim.KindAsteroid:
		return colAst
	case sim.KindWreck:
		return colWreck
	case sim.KindAnomaly:
		return colAnomaly
	default: // star, planet, moon, unknown
		return colCel
	}
}

// BracketColor returns the overview/bracket color for an entity kind
// (hostile NPC red, neutral orange, station blue, gate green, asteroid grey,
// wreck yellow, celestials blue). The player itself is ColPlayer in the UI.
func BracketColor(kind string, hostile bool) color.RGBA {
	return bracketColor(kind, hostile, false)
}

// cullMargin is how far outside the viewport a primitive's main extent may
// sit before it is skipped, in px. It is deliberately generous: bodies
// draw small satellites beyond their main radius (a gate's glow at 1.2×,
// a rock's specks at ~2.3×+8, a ship's flame at ~1.8×), and a just-off-
// screen body must keep them. It costs nothing — a body drawn at the
// margin still sits at ≤ ~2100 px, hundreds of times below the 1e7 px
// band where ebiten's path flattener degenerates (see offscreen).
const cullMargin = 256

// offscreen reports whether a primitive drawn around the screen point
// (sx, sy) with main extent px lies outside the viewport by more than
// cullMargin. The world body pass culls the same way (see
// Renderer.keepBody); the overlay's own passes use this so no vector path
// is ever built at the screen coordinates an AU-scale system produces
// off-screen, where ebiten's flattener degenerates.
func offscreen(cam Camera, sx, sy, px float64) bool {
	return sx+px+cullMargin < 0 || sx-px-cullMargin > cam.ScreenW ||
		sy+px+cullMargin < 0 || sy-px-cullMargin > cam.ScreenH
}

// seg adds an open line subpath to the cached path.
func (r *Renderer) seg(x0, y0, x1, y1 float32) {
	r.path.MoveTo(x0, y0)
	r.path.LineTo(x1, y1)
}

// bracket draws EVE-style corner brackets of the given half-size around
// (x, y).
func (r *Renderer) bracket(dst *ebiten.Image, x, y, size float64, col color.Color, width float32) {
	s := float32(size)
	arm := float32(size * 0.4)
	fx, fy := float32(x), float32(y)
	r.path.Reset()
	// top-left
	r.seg(fx-s, fy-s, fx-s+arm, fy-s)
	r.seg(fx-s, fy-s, fx-s, fy-s+arm)
	// top-right
	r.seg(fx+s, fy-s, fx+s-arm, fy-s)
	r.seg(fx+s, fy-s, fx+s, fy-s+arm)
	// bottom-left
	r.seg(fx-s, fy+s, fx-s+arm, fy+s)
	r.seg(fx-s, fy+s, fx-s, fy+s-arm)
	// bottom-right
	r.seg(fx+s, fy+s, fx+s-arm, fy+s)
	r.seg(fx+s, fy+s, fx+s, fy+s-arm)
	vector.StrokePath(dst, &r.path, &vector.StrokeOptions{Width: width},
		&vector.DrawPathOptions{AntiAlias: true, ColorScale: colorScale(col)})
}

// bracketHalfSize returns the bracket half-size in px for a grid object.
func bracketHalfSize(o gridObj, cam Camera) float64 {
	var radiusPx float64
	switch {
	case o.info.Kind == sim.KindShip || o.info.Kind == sim.KindPlayer:
		radiusPx = hullSize(o.hull) * cam.Zoom * 1.5
	case o.radius > 0:
		radiusPx = o.radius * cam.Zoom * 1.2
	default:
		radiusPx = 10
	}
	return clamp(radiusPx, 11, 220)
}

// ---------------------------------------------------------------------------
// Effects: gun fire (flash + projectile + impact), mining beams
// ---------------------------------------------------------------------------

// drawEffects draws per-ship active-module effects. Gun fire is drawn as
// discrete shots, never as a muzzle→target beam: the shot moment comes from
// the module runtime's cycle clock (see shotAge), so no sim event feed is
// needed in Draw's signature.
func (r *Renderer) drawEffects(dst *ebiten.Image, cam Camera, s *sim.Sim, now float64) {
	r.shotPrims, r.shotStrokes = 0, r.shotStrokes[:0]
	for _, o := range r.bShip {
		fit, ok := s.Fittings.Get(o.info.Entity)
		if !ok {
			continue
		}
		sx, sy := cam.WorldToScreen(o.info.Pos)
		sizePx := clamp(hullSize(o.hull)*cam.Zoom, 7, 160)
		if offscreen(cam, sx, sy, sizePx) {
			continue // its effects would build paths far outside the viewport
		}
		nose := core.V(sx, sy).Add(core.V(math.Cos(o.facing), math.Sin(o.facing)).Scale(sizePx))

		for _, class := range [...]string{content.SlotHigh, content.SlotMid} {
			defs := fit.Defs(class)
			rts := fit.Runtimes(class)
			for i := range rts {
				if !rts[i].Active || i >= len(defs) {
					continue
				}
				def, ok := r.c.Module(defs[i])
				if !ok {
					continue
				}
				switch def.Kind {
				case content.KindWeapon:
					age, fired := shotAge(rts[i].Active, rts[i].CycleEnds, shotCycle(def), now)
					if !fired {
						continue
					}
					tgt, haveTgt := targetPoint(cam, s, o.info.Entity, class, i)
					r.weaponFire(dst, nose, tgt, haveTgt, sizePx, age, def.Weapon.ShotTime >= heavyCycle)
				case content.KindMiningLaser:
					if rp, ok := targetPoint(cam, s, o.info.Entity, class, i); ok {
						r.miningBeam(dst, nose, rp)
					}
				}
			}
		}
	}
}

// heavyCycle is the shot time at or above which a gun renders as artillery:
// a slow heavy projectile instead of an autocannon's fast rattle.
const heavyCycle = 6

// targetPoint is the screen position of one module's own target — the ship
// its activation bound it to. A ship engaging several locks fires every
// module at the target that module was activated on (the sim resolves each
// cycle against it), so its effects follow the same binding instead of
// picking a lock.
func targetPoint(cam Camera, s *sim.Sim, ship ecs.Entity, class string, index int) (core.Vec2, bool) {
	tgt, ok := s.ModuleTarget(ship, sim.SlotRef{Class: class, Index: index})
	if !ok {
		return core.Vec2{}, false
	}
	tr, ok := s.Transforms.Get(tgt)
	if !ok {
		return core.Vec2{}, false
	}
	x, y := cam.WorldToScreen(tr.Pos)
	return core.V(x, y), true
}

// shotAge is the time since a weapon's last shot, read off its cycle clock:
// fireWeapon sets CycleEnds = shot + period, so the shot sits one period
// back. ok is false before the first shot and once the cycle has run out.
func shotAge(active bool, cycleEnds, period, now float64) (age float64, ok bool) {
	if !active || cycleEnds <= 0 || period <= 0 {
		return 0, false
	}
	age = now - (cycleEnds - period)
	return age, age >= 0 && age < period
}

// shotCycle is the gun's fire period, derived exactly as fireWeapon does.
func shotCycle(def *content.ModuleDef) float64 {
	switch {
	case def.Weapon != nil && def.Weapon.ShotTime > 0:
		return def.Weapon.ShotTime
	case def.ActivationTime > 0:
		return def.ActivationTime
	}
	return 1
}

// shotStyle is one gun family's shot timeline, in seconds after the shot.
type shotStyle struct {
	bolts  int     // rounds per cycle
	gap    float64 // spacing between round launches
	flight float64 // muzzle → target travel
	muzzle float64 // muzzle-flash life
	impact float64 // impact-glow life, from arrival
}

var (
	autoShot = shotStyle{bolts: 3, gap: 0.14, flight: 0.28, muzzle: 0.07, impact: 0.10}
	artyShot = shotStyle{bolts: 1, flight: 1.1, muzzle: 0.26, impact: 0.30}
)

// boltTail is the on-screen length of a travelling bolt, px.
const boltTail = 22

// weaponFire draws one gun's shot at age seconds after it fired: a muzzle
// flash at the nose, a bolt per round travelling to the target, an impact
// glow on arrival. Nothing is drawn for the rest of the cycle — fire is an
// event, not a persistent beam; the target bracket's distance label carries
// the range cue.
// shortcut: the flight is cosmetic (a shot resolves instantly in the sim);
// give the bolt real travel time if shots ever become simulated objects.
func (r *Renderer) weaponFire(dst *ebiten.Image, nose, tgt core.Vec2, haveTgt bool, sizePx, age float64, heavy bool) {
	st := autoShot
	boltW, glowSize := float32(1.6), 9.0
	boltCol := color.RGBA{255, 232, 180, 235}
	flashCol := color.RGBA{255, 220, 150, 255}
	if heavy {
		st = artyShot
		boltW, glowSize = 2.4, 13.0
		boltCol = color.RGBA{255, 205, 130, 240}
		flashCol = color.RGBA{255, 190, 110, 255}
	}
	dx, dy := tgt.X-nose.X, tgt.Y-nose.Y
	dist := math.Hypot(dx, dy)

	for k := 0; k < st.bolts; k++ {
		t := age - float64(k)*st.gap
		if t < 0 {
			continue
		}
		if f := t / st.muzzle; f < 1 {
			fr := sizePx * (0.6 + 0.5*(1-f))
			if heavy {
				fr = sizePx * (1.5 + 0.6*(1-f))
			}
			drawGlowCentered(dst, r.glow(64, flashCol), nose.X, nose.Y, fr*2, true)
			r.shotPrims++
		}
		if !haveTgt || dist < 1 {
			continue
		}
		switch f := t / st.flight; {
		case f < 1: // in flight: a short bright segment, never a full line
			bx, by := nose.X+dx*f, nose.Y+dy*f
			tl := math.Min(boltTail, dist) / dist
			r.shotLine(dst, bx, by, bx-dx*tl, by-dy*tl, boltW, boltCol)
			if heavy { // a thrown shell glows at its head
				drawGlowCentered(dst, r.glow(32, flashCol), bx, by, 9, true)
				r.shotPrims++
			}
		case t < st.flight+st.impact: // arrival
			drawGlowCentered(dst, r.glow(64, flashCol), tgt.X, tgt.Y, glowSize*1.6, true)
			r.shotPrims++
		}
	}
}

// shotLine strokes a weapon-effect segment and records its length for the
// render pins — how "no full-length weapon line" is asserted. Tests only
// read the record; production never does.
func (r *Renderer) shotLine(dst *ebiten.Image, x0, y0, x1, y1 float64, w float32, col color.RGBA) {
	vector.StrokeLine(dst, float32(x0), float32(y0), float32(x1), float32(y1), w, col, true)
	r.shotPrims++
	r.shotStrokes = append(r.shotStrokes, float32(math.Hypot(x1-x0, y1-y0)))
}

// miningBeam draws the laser link from ship to the rock its laser is cutting
// (the module's own target, not the nearest rock on screen).
func (r *Renderer) miningBeam(dst *ebiten.Image, nose, rock core.Vec2) {
	rx, ry := rock.X, rock.Y
	vector.StrokeLine(dst, float32(nose.X), float32(nose.Y), float32(rx), float32(ry),
		2.4, color.RGBA{110, 235, 150, 190}, true)
	vector.StrokeLine(dst, float32(nose.X), float32(nose.Y), float32(rx), float32(ry),
		0.9, color.RGBA{230, 255, 235, 230}, true)
	img := r.glow(32, color.RGBA{140, 255, 170, 255})
	drawGlowCentered(dst, img, rx, ry, 14, true)
}

// ---------------------------------------------------------------------------
// Overlay: brackets, labels, selection, lock lines
// ---------------------------------------------------------------------------

// drawOverlay draws brackets, name/distance labels, selection highlight and
// the player's target lock lines (topmost world layers).
func (r *Renderer) drawOverlay(dst *ebiten.Image, cam Camera, s *sim.Sim) {
	player := s.Player()
	var ppos core.Vec2
	havePlayer := false
	if tr, ok := s.Transforms.Get(player); ok {
		ppos, havePlayer = tr.Pos, true
	}
	showLabels := cam.Zoom <= labelZoomMax

	for _, bucket := range [][]gridObj{r.bStar, r.bPlanet, r.bAnomaly, r.bStatic, r.bRock, r.bWreck, r.bShip} {
		for _, o := range bucket {
			sx, sy := cam.WorldToScreen(o.info.Pos)
			if sx < -240 || sx > cam.ScreenW+240 || sy < -240 || sy > cam.ScreenH+240 {
				continue
			}
			half := bracketHalfSize(o, cam)
			col := bracketColor(o.info.Kind, o.info.Hostile, o.info.Entity == player)
			isSel := o.info.Entity == r.selected
			if isSel {
				col = colSelect
			}
			r.bracket(dst, sx, sy, half, col, 1.6)
			if showLabels {
				nameCol := color.RGBA{216, 222, 233, 255}
				if isSel {
					nameCol = colSelect
				} else if o.info.Hostile {
					nameCol = color.RGBA{255, 130, 130, 255}
				}
				w := Measure(o.info.Name, LabelSize)
				nameY := sy + half + 5*FontScale
				Text(dst, o.info.Name, sx-w/2, nameY, LabelSize, nameCol)
				dist := format.Dist(o.info.Distance)
				wd := Measure(dist, LabelSizeSmall)
				Text(dst, dist, sx-wd/2, nameY+labelLinePitch(LabelSize),
					LabelSizeSmall, color.RGBA{135, 146, 163, 255})
			}
		}
	}

	// Player lock lines and lock progress (above brackets).
	if havePlayer {
		if tg, ok := s.Targetings.Get(player); ok && len(tg.Locks) > 0 {
			x0, y0 := cam.WorldToScreen(ppos)
			for _, l := range tg.Locks {
				tr, ok := s.Transforms.Get(l.Target)
				if !ok {
					continue
				}
				x1, y1 := cam.WorldToScreen(tr.Pos)
				if offscreen(cam, x1, y1, 15) {
					continue // a line to an off-screen target: no pixel, huge coords
				}
				vector.StrokeLine(dst, float32(x0), float32(y0), float32(x1), float32(y1), 1,
					color.RGBA{140, 230, 255, 110}, true)
				if l.Progress < 1 {
					f := clamp(l.Progress, 0, 1)
					strokeArc(dst, float32(x1), float32(y1), 15, -float32(math.Pi/2),
						float32(-math.Pi/2+2*math.Pi*f), 2, colLock)
				} else {
					r.bracket(dst, x1, y1, 15, colLock, 2)
				}
			}
		}
	}

	// Selection highlight on top: bigger amber corners plus a soft ring.
	for _, bucket := range [][]gridObj{r.bStar, r.bPlanet, r.bAnomaly, r.bStatic, r.bRock, r.bWreck, r.bShip} {
		for _, o := range bucket {
			if o.info.Entity != r.selected {
				continue
			}
			sx, sy := cam.WorldToScreen(o.info.Pos)
			half := bracketHalfSize(o, cam) + 5
			if offscreen(cam, sx, sy, half+6) {
				continue
			}
			r.bracket(dst, sx, sy, half, colSelect, 2)
			vector.StrokeCircle(dst, float32(sx), float32(sy), float32(half+6), 1,
				color.RGBA{255, 180, 84, 90}, true)
		}
	}
}

// drawWarpTunnel paints radial streaks while the player is in warp.
func (r *Renderer) drawWarpTunnel(dst *ebiten.Image, cam Camera, s *sim.Sim, now float64) {
	player := s.Player()
	wd, ok := s.Warps.Get(player)
	if !ok || wd.Phase != "warp" {
		return
	}
	cx, cy := cam.ScreenW/2, cam.ScreenH/2
	maxd := math.Hypot(cx, cy)
	const n = 44
	for i := 0; i < n; i++ {
		a := float64(i)/n*2*math.Pi + now*0.5
		phase := math.Mod(now*2.4+float64(i)*0.6180339887, 1)
		r0 := (0.08 + 0.7*phase) * maxd
		length := 40 + 200*phase
		alpha := uint8(210 * (1 - phase))
		x0 := cx + math.Cos(a)*r0
		y0 := cy + math.Sin(a)*r0
		x1 := cx + math.Cos(a)*(r0+length)
		y1 := cy + math.Sin(a)*(r0+length)
		vector.StrokeLine(dst, float32(x0), float32(y0), float32(x1), float32(y1),
			1.6, color.RGBA{200, 230, 255, alpha}, true)
	}
}
