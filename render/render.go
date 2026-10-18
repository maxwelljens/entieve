package render

import (
	"errors"
	"image/color"
	"math"
	"strconv"
	"strings"

	"github.com/entieve/entieve/content"
	"github.com/entieve/entieve/core"
	"github.com/entieve/entieve/ecs"
	"github.com/entieve/entieve/sim"
	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/vector"
)

// labelZoomMax is the zoom (px/m) at or below which name/distance labels
// draw; zoomed in past it the sprites are big enough to identify directly.
const labelZoomMax = 0.02

// FontScale is the UI typography scale: ui.FontBase (21) is 1.75× the
// legacy 12 px base, i.e. text everywhere is ~75% larger. render cannot
// import ui (ui imports render), so the scale is mirrored here for the
// in-world labels; ui's TestFontScaleMatchesRender pins both sides.
const FontScale = 1.75

// In-world label sizes, scaled by the same factor as the UI text (the old
// hardcoded 11/10 px labels).
const (
	LabelSize      = 11 * FontScale // 19.25 — bracket name labels
	LabelSizeSmall = 10 * FontScale // 17.5 — distance labels under names
)

// labelLinePitch is the line height between a name and its distance
// (mirrors ui.LineH's ceil(size×1.35) factor).
func labelLinePitch(size float64) float64 {
	return math.Ceil(size * 1.35)
}

// starTileSize is the pixel size of one starfield tile.
const starTileSize = 256

type starKey struct {
	seed  uint64
	layer int
}

type glowKey struct {
	size int
	col  color.RGBA
}

// gridObj is one grid entity with the extra lookups rendering needs,
// resolved once per frame during bucketing.
type gridObj struct {
	info      sim.Info
	radius    float64 // celestial radius, m (0 if none)
	facing    float64 // radians
	hull      string  // hull class of ships
	shipID    string  // hull content ID (assets/hulls/<shipID>.png)
	ore       string  // asteroid ore ID (assets/asteroids/<ore>.png)
	speedMult float64 // Motion.SpeedMultiplier (1 = none)
}

// Renderer draws sim state. Create with New; it caches procedural sprites
// and reuses scratch slices across frames.
type Renderer struct {
	c      *content.Content
	path   vector.Path
	tiles  map[starKey]*ebiten.Image
	glows  map[glowKey]*ebiten.Image
	bodies map[uint32]*ebiten.Image

	// Background plates keyed by resolved path: decoded plates are ~26 MB
	// each (2560² RGBA). Shared "default" plates persist across system
	// switches; per-system plates are evicted on change (see plate).
	plates   map[string]*bgPlate
	plateSys content.SystemID

	selected ecs.Entity

	// Per-frame buckets, reused to avoid allocations in Draw.
	bStar, bPlanet, bAnomaly, bStatic, bRock, bWreck, bShip []gridObj
	poly                                                    []core.Vec2

	// bodyPrims counts the body primitives the last Draw submitted: every
	// on-screen celestial, rock, wreck and ship. Read by
	// BodiesDrawnForTest — the frame-cost pin.
	bodyPrims int
}

// New returns a Renderer over the loaded content.
func New(c *content.Content) *Renderer {
	return &Renderer{
		c:      c,
		tiles:  map[starKey]*ebiten.Image{},
		glows:  map[glowKey]*ebiten.Image{},
		bodies: map[uint32]*ebiten.Image{},
		plates: map[string]*bgPlate{},
	}
}

// SetSelected marks an entity for the amber selection highlight. The Game
// syncs this from the UI each frame before Draw.
func (r *Renderer) SetSelected(e ecs.Entity) { r.selected = e }

// BodiesDrawnForTest is the number of body primitives the last Draw
// submitted — the off-screen culling pin's counter (see keepBody). Tests
// only; production never reads it.
func (r *Renderer) BodiesDrawnForTest() int { return r.bodyPrims }

// glow returns a cached radial glow sprite.
func (r *Renderer) glow(size int, col color.RGBA) *ebiten.Image {
	k := glowKey{size: size, col: col}
	if img, ok := r.glows[k]; ok {
		return img
	}
	img := glowSprite(size, col)
	r.glows[k] = img
	return img
}

// body returns a cached shaded-disc sprite for a planet/moon color.
func (r *Renderer) body(col color.RGBA) *ebiten.Image {
	k := uint32(col.R)<<24 | uint32(col.G)<<16 | uint32(col.B)<<8 | uint32(col.A)
	if img, ok := r.bodies[k]; ok {
		return img
	}
	img := bodySprite(col)
	r.bodies[k] = img
	return img
}

// starTile returns a cached starfield tile for a system seed and layer.
func (r *Renderer) starTile(seed uint64, layer int) *ebiten.Image {
	k := starKey{seed: seed, layer: layer}
	if img, ok := r.tiles[k]; ok {
		return img
	}
	counts := [3]int{90, 55, 30}
	img := ebiten.NewImage(starTileSize, starTileSize)
	for _, s := range Starfield(seed+uint64(layer)*7919, counts[layer]) {
		a := uint8(220 * s.Bright)
		vector.DrawFilledCircle(img, float32(s.X*starTileSize), float32(s.Y*starTileSize),
			float32(s.Size), color.RGBA{255, 255, 255, a}, true)
	}
	r.tiles[k] = img
	return img
}

// bgPlate is one decoded background plate with the draw path committed by its
// pixel probe (see plateSky).
type bgPlate struct {
	img *ebiten.Image // never nil in the cache
	sky bool          // opaque sky: bottom layer under the nebula washes
	def bool          // resolved from the shared "default" plate: survives switches
}

// plate resolves one starfield layer for a system: the system's own plate when
// it ships one, else the shared cluster-wide default plate (`default.png`,
// `default-01.png`, …), else nil — the caller then draws the untouched
// procedural art. The cache is keyed by resolved path: shared defaults persist
// across system switches (they serve every system), per-system plates are
// evicted when the system changes (a decoded 2560² RGBA plate is ~26 MB, so
// the decode cache stays bounded at defaults + one system's overrides).
func (r *Renderer) plate(layer int, sys *content.SystemDef) *bgPlate {
	var id content.SystemID
	if sys != nil {
		id = sys.ID
	}
	if id != r.plateSys {
		r.plateSys = id
		for k, p := range r.plates {
			if !p.def {
				delete(r.plates, k) // per-system plate: drop with the system
			}
		}
	}
	path := platePath(plateKey(string(id), layer))
	def := false
	if path == "" {
		path = platePath(plateKey(defaultPlateID, layer))
		def = path != ""
	}
	if path == "" {
		return nil // no plate for this layer: procedural fallback
	}
	if pl, ok := r.plates[path]; ok {
		return pl
	}
	img, sky := loadPlate(path)
	if img == nil {
		return nil
	}
	pl := &bgPlate{img: img, sky: sky, def: def}
	r.plates[path] = pl
	return pl
}

// starLayerTile selects what one starfield layer draws and at what native
// tile size: the resolved plate when the layer ships one (sky plates are the
// backdrop's job), otherwise the cached procedural tile — but only when the
// system has no star plates at all. A resolved star-plate set IS the sky:
// layers without a plate draw nothing, so no stock procedural dots bleed
// through next to author art. Returns nil when the layer draws nothing.
func (r *Renderer) starLayerTile(seed uint64, layer int, sys *content.SystemDef) (*ebiten.Image, float64, float64) {
	if pl := r.plate(layer, sys); pl != nil && !pl.sky {
		b := pl.img.Bounds()
		return pl.img, float64(b.Dx()), float64(b.Dy())
	}
	if r.plateStarSet(sys) {
		return nil, 0, 0
	}
	return r.starTile(seed, layer), starTileSize, starTileSize
}

// plateStarSet reports whether any starfield layer resolves to a
// star-classified plate (per-system override or shared default) for this
// system — the "the plates are the sky" trigger.
func (r *Renderer) plateStarSet(sys *content.SystemDef) bool {
	for l := 0; l < 3; l++ {
		if pl := r.plate(l, sys); pl != nil && !pl.sky {
			return true
		}
	}
	return false
}

// ---------------------------------------------------------------------------
// Hex palette colors
// ---------------------------------------------------------------------------

// ParseHex parses "#rgb", "#rrggbb" or "#rrggbbaa" (leading # optional).
func ParseHex(s string) (color.RGBA, error) {
	s = strings.TrimPrefix(s, "#")
	var errBad = errors.New("invalid hex color")
	switch len(s) {
	case 3:
		var v [3]byte
		for i := 0; i < 3; i++ {
			n, e := strconv.ParseUint(string(s[i]), 16, 8)
			if e != nil {
				return color.RGBA{}, errBad
			}
			v[i] = uint8(n)
		}
		return color.RGBA{R: v[0] * 17, G: v[1] * 17, B: v[2] * 17, A: 255}, nil
	case 6, 8:
		n, e := strconv.ParseUint(s, 16, 64)
		if e != nil {
			return color.RGBA{}, errBad
		}
		if len(s) == 6 {
			return color.RGBA{R: uint8(n >> 16), G: uint8(n >> 8), B: uint8(n), A: 255}, nil
		}
		return color.RGBA{R: uint8(n >> 24), G: uint8(n >> 16), B: uint8(n >> 8), A: uint8(n)}, nil
	}
	return color.RGBA{}, errBad
}

// palette looks up a system palette color with a fallback.
func palette(sys *content.SystemDef, key, fallback string) color.RGBA {
	if sys != nil {
		if v, ok := sys.Palette[key]; ok {
			if c, err := ParseHex(v); err == nil {
				return c
			}
		}
	}
	c, err := ParseHex(fallback)
	if err != nil {
		return color.RGBA{A: 255}
	}
	return c
}

// celDef finds a celestial definition by ID in a system.
func celDef(sys *content.SystemDef, id content.CelID) *content.CelestialDef {
	if sys == nil {
		return nil
	}
	for i := range sys.Celestials {
		if sys.Celestials[i].ID == id {
			return &sys.Celestials[i]
		}
	}
	return nil
}

// ---------------------------------------------------------------------------
// Draw
// ---------------------------------------------------------------------------

// Draw renders the current sim state. It reads sim stores read-only.
func (r *Renderer) Draw(dst *ebiten.Image, cam Camera, s *sim.Sim) {
	if cam.ScreenW <= 0 || cam.ScreenH <= 0 {
		b := dst.Bounds()
		cam.ScreenW, cam.ScreenH = float64(b.Dx()), float64(b.Dy())
	}
	if cam.Zoom <= 0 {
		cam.Zoom = 1e-3
	}
	sys := s.System()
	now := s.Time()
	r.bodyPrims = 0

	dst.Fill(color.RGBA{2, 3, 6, 255})
	r.drawBackdrop(dst, cam, sys)
	r.drawNebula(dst, cam, sys)
	r.drawStarfield(dst, cam, sys)
	r.drawGrid(dst, cam)

	r.bucket(s, sys)
	for _, o := range r.bStar {
		r.drawStar(dst, cam, o, sys)
	}
	for _, o := range r.bPlanet {
		r.drawPlanet(dst, cam, o, sys)
	}
	for _, o := range r.bAnomaly {
		r.drawAnomaly(dst, cam, o, now)
	}
	for _, o := range r.bStatic {
		r.drawStatic(dst, cam, o)
	}
	for _, o := range r.bRock {
		r.drawRock(dst, cam, o)
	}
	for _, o := range r.bWreck {
		r.drawWreck(dst, cam, o)
	}
	for _, o := range r.bShip {
		r.drawShip(dst, cam, o, now)
	}
	r.drawEffects(dst, cam, s, now)
	r.drawOverlay(dst, cam, s)
	r.drawWarpTunnel(dst, cam, s, now)
}

// bucket classifies every perceivable grid entity into per-layer slices.
// Info.Visible is the sensor gate: landmarks plus everything within
// sim.SensorRange of the player; off-grid entities are never drawn (the
// client has no omniscience — the Scanner reports them anonymously only).
func (r *Renderer) bucket(s *sim.Sim, sys *content.SystemDef) {
	r.bStar = r.bStar[:0]
	r.bPlanet = r.bPlanet[:0]
	r.bAnomaly = r.bAnomaly[:0]
	r.bStatic = r.bStatic[:0]
	r.bRock = r.bRock[:0]
	r.bWreck = r.bWreck[:0]
	r.bShip = r.bShip[:0]
	for _, inf := range s.Grid() {
		if !inf.Visible {
			continue
		}
		o := gridObj{info: inf, speedMult: 1}
		if cel, ok := s.Celestials.Get(inf.Entity); ok {
			o.radius = cel.Radius
		}
		if tr, ok := s.Transforms.Get(inf.Entity); ok {
			o.facing = tr.Facing
		}
		if sh, ok := s.Ships.Get(inf.Entity); ok {
			o.shipID = string(sh.Hull)
			if def, ok := r.c.Ship(sh.Hull); ok {
				o.hull = def.Class
			}
		}
		if a, ok := s.Asteroids.Get(inf.Entity); ok {
			o.ore = string(a.Ore)
		}
		if m, ok := s.Motions.Get(inf.Entity); ok && m.SpeedMultiplier != 0 {
			o.speedMult = m.SpeedMultiplier
		}
		switch inf.Kind {
		case sim.KindStar:
			r.bStar = append(r.bStar, o)
		case sim.KindPlanet, sim.KindMoon:
			r.bPlanet = append(r.bPlanet, o)
		case sim.KindAnomaly:
			r.bAnomaly = append(r.bAnomaly, o)
		case sim.KindAsteroid:
			r.bRock = append(r.bRock, o)
		case sim.KindWreck:
			r.bWreck = append(r.bWreck, o)
		case sim.KindShip, sim.KindPlayer:
			r.bShip = append(r.bShip, o)
		default: // station, gate, belt, unknown celestials
			r.bStatic = append(r.bStatic, o)
		}
	}
}

// ---------------------------------------------------------------------------
// Background layers
// ---------------------------------------------------------------------------

// drawBackdrop paints the sky-classified plates — assets/backgrounds/
// <system-id>[-NN].png whose pixel probe read an opaque sky — over the flat
// background fill and under the nebula washes and starfield, covering the
// whole screen with the aspect preserved. Transparent star plates skip this
// pass and draw with their starfield layer instead (drawStarfield). No plate
// = no-op: the stock background draws as always.
// shortcut: sky plates are screen-anchored with no parallax — the shipped art
// probes as the star layer, so this path only serves future opaque art;
// give it the clamped parallax offset when such art lands.
func (r *Renderer) drawBackdrop(dst *ebiten.Image, cam Camera, sys *content.SystemDef) {
	for layer := 0; layer < 3; layer++ {
		pl := r.plate(layer, sys)
		if pl == nil || !pl.sky {
			continue
		}
		b := pl.img.Bounds()
		s := math.Max(cam.ScreenW/float64(b.Dx()), cam.ScreenH/float64(b.Dy()))
		drawSprite(dst, pl.img, cam.ScreenW/2, cam.ScreenH/2,
			float64(b.Dx())*s, float64(b.Dy())*s, 0)
	}
}

// drawNebula paints the system palette's nebula washes (screen-anchored).
func (r *Renderer) drawNebula(dst *ebiten.Image, cam Camera, sys *content.SystemDef) {
	keys := [3]string{"nebula1", "nebula2", "nebula3"}
	defs := [3]string{"#16233f", "#2a1636", "#102a24"}
	var seed uint64
	if sys != nil {
		seed = HashID(string(sys.ID))
	}
	for i := 0; i < 3; i++ {
		col := palette(sys, keys[i], defs[i])
		col.A = 96
		img := r.glow(128, col)
		// Deterministic placement per system, spread over the screen.
		fx := float64((seed>>uint(8*i))&0xff) / 255
		fy := float64((seed>>uint(8*i+4))&0x1ff) / 511
		cx := (0.25 + 0.5*fx) * cam.ScreenW
		cy := (0.25 + 0.5*fy) * cam.ScreenH
		size := math.Max(cam.ScreenW, cam.ScreenH) * (1.2 + 0.3*float64(i))
		drawGlowCentered(dst, img, cx, cy, size, false)
	}
}

// drawStarfield paints three parallax layers. A layer with a drop-in plate
// (per-system override or the shared default) draws that plate at its native
// size on the same camera-parallax wrap and rate as the procedural layer it
// replaces. The resolved star-plate set is the sky: layers without a plate
// draw nothing once any star plate exists — only a system with no plates at
// all renders the untouched stock procedural starfield, byte-identically.
// shortcut: plates tile 1:1 (the shipped plates measure seam-clean at the
// edges), art whose opposite edges don't match will show the seam — switch
// such a plate to a screen-anchored clamped draw when it lands.
func (r *Renderer) drawStarfield(dst *ebiten.Image, cam Camera, sys *content.SystemDef) {
	var seed uint64
	if sys != nil {
		seed = HashID(string(sys.ID))
	}
	factors := [3]float64{0.01, 0.03, 0.06}
	op := &ebiten.DrawImageOptions{}
	for layer := 0; layer < 3; layer++ {
		tile, tw, th := r.starLayerTile(seed, layer, sys)
		if tile == nil {
			continue // plate set is the sky: this layer draws nothing
		}
		f := factors[layer]
		ox := math.Mod(cam.Center.X*cam.Zoom*f, tw)
		oy := math.Mod(cam.Center.Y*cam.Zoom*f, th)
		if ox < 0 {
			ox += tw
		}
		if oy < 0 {
			oy += th
		}
		startX := math.Mod(-ox, tw)
		if startX > 0 {
			startX -= tw
		}
		startY := math.Mod(-oy, th)
		if startY > 0 {
			startY -= th
		}
		for x := startX; x < cam.ScreenW; x += tw {
			for y := startY; y < cam.ScreenH; y += th {
				op.GeoM.Reset()
				op.GeoM.Translate(x, y)
				dst.DrawImage(tile, op)
			}
		}
	}
}

// drawGrid paints faint world-space grid dots at a power-of-ten spacing.
func (r *Renderer) drawGrid(dst *ebiten.Image, cam Camera) {
	sp := 10.0
	for sp*cam.Zoom < 64 && sp < 1e14 {
		sp *= 10
	}
	halfW := cam.ScreenW / 2 / cam.Zoom
	halfH := cam.ScreenH / 2 / cam.Zoom
	x0 := math.Floor((cam.Center.X-halfW)/sp) * sp
	y0 := math.Floor((cam.Center.Y-halfH)/sp) * sp
	col := color.RGBA{120, 150, 190, 22}
	for x := x0; x <= cam.Center.X+halfW; x += sp {
		for y := y0; y <= cam.Center.Y+halfH; y += sp {
			sx, sy := cam.WorldToScreen(core.V(x, y))
			vector.DrawFilledCircle(dst, float32(sx), float32(sy), 1.2, col, false)
		}
	}
}

// drawGlowCentered draws a sprite centered at (cx, cy) scaled to size px.
func drawGlowCentered(dst *ebiten.Image, img *ebiten.Image, cx, cy, size float64, add bool) {
	op := &ebiten.DrawImageOptions{}
	s := size / float64(img.Bounds().Dx())
	op.GeoM.Scale(s, s)
	op.GeoM.Translate(cx-size/2, cy-size/2)
	if add {
		op.Blend = ebiten.BlendLighter
	}
	dst.DrawImage(img, op)
}

// ---------------------------------------------------------------------------
// Celestial bodies
// ---------------------------------------------------------------------------

// drawStar renders the system star: big radial glow plus a hot core.
func (r *Renderer) drawStar(dst *ebiten.Image, cam Camera, o gridObj, sys *content.SystemDef) {
	sx, sy := cam.WorldToScreen(o.info.Pos)
	col := palette(sys, "star", "#ffd28a")
	if o.radius <= 0 {
		o.radius = 3e8
	}
	glowR := clamp(o.radius*cam.Zoom, 80, 0.45*math.Max(cam.ScreenW, cam.ScreenH))
	if !r.keepBody(cam, sx, sy, glowR) {
		return
	}
	img := r.glow(128, color.RGBA{col.R, col.G, col.B, 220})
	drawGlowCentered(dst, img, sx, sy, glowR*2, true)
	vector.DrawFilledCircle(dst, float32(sx), float32(sy), float32(glowR*0.22),
		color.RGBA{255, 245, 220, 255}, true)
}

// drawPlanet renders a planet or moon with baked limb shading.
func (r *Renderer) drawPlanet(dst *ebiten.Image, cam Camera, o gridObj, sys *content.SystemDef) {
	sx, sy := cam.WorldToScreen(o.info.Pos)
	def := celDef(sys, o.info.Cel)
	colName := ""
	if def != nil && len(def.Colors) > 0 {
		colName = def.Colors[0]
	}
	fallback := "#4a7fc0"
	if o.info.Kind == sim.KindMoon {
		fallback = "#9aa2ab"
	}
	col := palette(&content.SystemDef{Palette: map[string]string{"c": colName}}, "c", fallback)
	col.A = 255
	if o.radius <= 0 {
		o.radius = 3e7
	}
	maxDim := math.Max(cam.ScreenW, cam.ScreenH)
	px := clamp(o.radius*cam.Zoom, 6, 0.4*maxDim)
	if !r.keepBody(cam, sx, sy, px) {
		return
	}
	drawGlowCentered(dst, r.body(col), sx, sy, px*2, false)
}

// drawAnomaly renders a combat anomaly: purple wash and a slow hexagon.
func (r *Renderer) drawAnomaly(dst *ebiten.Image, cam Camera, o gridObj, now float64) {
	sx, sy := cam.WorldToScreen(o.info.Pos)
	col := color.RGBA{208, 128, 255, 90}
	img := r.glow(128, col)
	px := clamp(2e4*cam.Zoom, 40, 0.3*math.Max(cam.ScreenW, cam.ScreenH))
	if !r.keepBody(cam, sx, sy, px) {
		return
	}
	drawGlowCentered(dst, img, sx, sy, px*2, true)
	ang := now * 0.4
	r.path.Reset()
	for i := 0; i <= 6; i++ {
		a := ang + float64(i)/6*2*math.Pi
		x := float32(sx + math.Cos(a)*px*0.7)
		y := float32(sy + math.Sin(a)*px*0.7)
		if i == 0 {
			r.path.MoveTo(x, y)
		} else {
			r.path.LineTo(x, y)
		}
	}
	r.path.Close()
	vector.StrokePath(dst, &r.path, &vector.StrokeOptions{Width: 1.5},
		&vector.DrawPathOptions{AntiAlias: true, ColorScale: colorScale(color.RGBA{216, 180, 255, 200})})
}

// drawCelSprite draws the assets/stations/<cel>.png sprite for a static
// celestial, fitted to the same px footprint its procedural art would get.
// It reports whether a sprite existed — the caller then skips the
// procedural branch (no file = always false, vector art untouched).
func drawCelSprite(dst *ebiten.Image, sx, sy, px float64, cel content.CelID) bool {
	if cel == "" {
		return false
	}
	img := spriteFor("stations/" + string(cel))
	if img == nil {
		return false
	}
	dw, dh := fitSprite(img.Bounds().Dx(), img.Bounds().Dy(), px)
	drawSprite(dst, img, sx, sy, dw, dh, 0)
	return true
}

// drawStatic renders stations, stargates and other static celestials.
func (r *Renderer) drawStatic(dst *ebiten.Image, cam Camera, o gridObj) {
	sx, sy := cam.WorldToScreen(o.info.Pos)
	maxDim := math.Max(cam.ScreenW, cam.ScreenH)
	switch o.info.Kind {
	case sim.KindStation:
		px := clamp(orDef(o.radius, 2500)*cam.Zoom, 14, 0.25*maxDim)
		if !r.keepBody(cam, sx, sy, px) {
			return
		}
		if drawCelSprite(dst, sx, sy, px, o.info.Cel) {
			break
		}
		// Octagonal hub with spokes.
		r.path.Reset()
		for i := 0; i < 8; i++ {
			a := float64(i)/8*2*math.Pi + math.Pi/8
			x := float32(sx + math.Cos(a)*px)
			y := float32(sy + math.Sin(a)*px)
			if i == 0 {
				r.path.MoveTo(x, y)
			} else {
				r.path.LineTo(x, y)
			}
		}
		r.path.Close()
		vector.FillPath(dst, &r.path, nil, &vector.DrawPathOptions{
			AntiAlias:  true,
			ColorScale: colorScale(color.RGBA{70, 84, 104, 235}),
		})
		vector.StrokePath(dst, &r.path, &vector.StrokeOptions{Width: 1.6},
			&vector.DrawPathOptions{AntiAlias: true, ColorScale: colorScale(color.RGBA{143, 164, 192, 255})})
		vector.DrawFilledCircle(dst, float32(sx), float32(sy), float32(px*0.34),
			color.RGBA{44, 53, 66, 255}, true)
		// Amber running lights.
		for i := 0; i < 4; i++ {
			a := float64(i)/4*2*math.Pi + math.Pi/4
			vector.DrawFilledCircle(dst,
				float32(sx+math.Cos(a)*px*0.72), float32(sy+math.Sin(a)*px*0.72),
				float32(math.Max(1.2, px*0.05)), color.RGBA{255, 180, 84, 255}, true)
		}
	case sim.KindGate:
		px := clamp(orDef(o.radius, 1500)*cam.Zoom, 16, 0.3*maxDim)
		if !r.keepBody(cam, sx, sy, px) {
			return
		}
		if drawCelSprite(dst, sx, sy, px, o.info.Cel) {
			break
		}
		img := r.glow(128, color.RGBA{80, 220, 130, 140})
		drawGlowCentered(dst, img, sx, sy, px*2.4, true)
		vector.StrokeCircle(dst, float32(sx), float32(sy), float32(px), 3,
			color.RGBA{127, 224, 143, 230}, true)
		vector.StrokeCircle(dst, float32(sx), float32(sy), float32(px*0.62), 1.5,
			color.RGBA{127, 224, 143, 150}, true)
		for i := 0; i < 3; i++ {
			a := float64(i)/3*2*math.Pi - math.Pi/2
			vector.DrawFilledCircle(dst,
				float32(sx+math.Cos(a)*px), float32(sy+math.Sin(a)*px), 3,
				color.RGBA{200, 255, 215, 255}, true)
		}
	default: // belts, unknown celestials: faint dust circle
		px := clamp(orDef(o.radius, 4e4)*cam.Zoom, 20, 0.4*maxDim)
		if !r.keepBody(cam, sx, sy, px) {
			return
		}
		if drawCelSprite(dst, sx, sy, px, o.info.Cel) {
			break
		}
		for i := 0; i < 12; i++ {
			a := float64(i) / 12 * 2 * math.Pi
			a2 := a + 0.22
			strokeArc(dst, float32(sx), float32(sy), float32(px), float32(a), float32(a2), 1,
				color.RGBA{140, 150, 165, 70})
		}
	}
}

// drawRock renders one asteroid as a jagged rock with scattered specks.
func (r *Renderer) drawRock(dst *ebiten.Image, cam Camera, o gridObj) {
	sx, sy := cam.WorldToScreen(o.info.Pos)
	px := clamp(70*cam.Zoom, 4, 60)
	if !r.keepBody(cam, sx, sy, px) {
		return
	}
	if img := spriteFor("asteroids/" + o.ore); img != nil {
		dw, dh := fitSprite(img.Bounds().Dx(), img.Bounds().Dy(), px)
		drawSprite(dst, img, sx, sy, dw, dh, 0)
		return
	}
	r.poly = r.poly[:0]
	for _, p := range rockPoly(uint64(o.info.Entity), 7) {
		r.poly = append(r.poly, core.V(sx+p.X*px, sy+p.Y*px))
	}
	r.fillPoly(dst, r.poly, color.RGBA{96, 100, 108, 255})
	r.strokePoly(dst, r.poly, 1, color.RGBA{150, 156, 166, 255})
	// Belt feel: a couple of specks orbiting the rock.
	rng := rockPoly(uint64(o.info.Entity)*31+7, 3)
	for i, p := range rng {
		dx, dy := p.X*(px*2.2+6), p.Y*(px*2.2+6)
		vector.DrawFilledCircle(dst, float32(sx+dx), float32(sy+dy), float32(1+float64(i)*0.4),
			color.RGBA{110, 115, 124, 200}, false)
	}
}

// drawWreck renders a broken hull: jagged dark shape.
func (r *Renderer) drawWreck(dst *ebiten.Image, cam Camera, o gridObj) {
	sx, sy := cam.WorldToScreen(o.info.Pos)
	px := clamp(55*cam.Zoom, 5, 40)
	if !r.keepBody(cam, sx, sy, px) {
		return
	}
	r.poly = r.poly[:0]
	src := rockPoly(uint64(o.info.Entity)^0xBEEF, 9)
	for i, p := range src {
		k := 1.0
		if i%2 == 0 {
			k = 0.45 // spikier alternation reads as torn hull
		}
		r.poly = append(r.poly, core.V(sx+p.X*px*k, sy+p.Y*px*k))
	}
	r.fillPoly(dst, r.poly, color.RGBA{52, 56, 62, 255})
	r.strokePoly(dst, r.poly, 1, color.RGBA{140, 130, 90, 255})
}

// drawShip renders a hull-class vector silhouette pointing along Facing,
// plus afterburner flame when SpeedMultiplier > 1.
func (r *Renderer) drawShip(dst *ebiten.Image, cam Camera, o gridObj, now float64) {
	sx, sy := cam.WorldToScreen(o.info.Pos)
	sizePx := clamp(hullSize(o.hull)*cam.Zoom, 7, 160)
	if !r.keepBody(cam, sx, sy, sizePx) {
		return
	}

	// Afterburner flame (behind the ship, opposite the nose).
	if o.speedMult > 1 {
		flame := []core.Vec2{
			{X: -0.85, Y: -0.26}, {X: -1.7 - 0.2*math.Sin(now*30), Y: 0}, {X: -0.85, Y: 0.26},
		}
		r.poly = r.poly[:0]
		for _, p := range flame {
			q := p.Rotate(o.facing).Scale(sizePx)
			r.poly = append(r.poly, core.V(sx+q.X, sy+q.Y))
		}
		r.fillPoly(dst, r.poly, color.RGBA{255, 180, 84, 170})
	}

	if img := spriteFor("hulls/" + o.shipID); img != nil {
		dw, dh := fitSprite(img.Bounds().Dx(), img.Bounds().Dy(), sizePx)
		drawSprite(dst, img, sx, sy, dw, dh, o.facing)
		return
	}
	r.poly = r.poly[:0]
	for _, p := range hullPoly(o.hull) {
		q := p.Rotate(o.facing).Scale(sizePx)
		r.poly = append(r.poly, core.V(sx+q.X, sy+q.Y))
	}
	r.fillPoly(dst, r.poly, color.RGBA{143, 163, 189, 255})
	r.strokePoly(dst, r.poly, 1.4, color.RGBA{38, 46, 58, 255})
}

// orDef returns v or d when v <= 0.
func orDef(v, d float64) float64 {
	if v <= 0 {
		return d
	}
	return v
}

// keepBody reports whether a body drawn as a disc of radius px around the
// screen point (sx, sy) belongs in this frame, counting the ones it
// submits. A body whose disc cannot reach the viewport (see cullMargin)
// is skipped, not merely clipped: ebiten's vector path flattener
// (Arc → cubicTo → AddStroke) degenerates
// when its input coordinates run into the 1e7..1e13 px band — exactly
// where AU-scale systems' world units land once the camera is tight — so
// an off-screen body costs ~5-20 ms per stroked primitive against ~15 µs
// on-screen, every frame, for geometry no pixel sees. The overlay already
// gates its brackets this way; the body layers must too.
func (r *Renderer) keepBody(cam Camera, sx, sy, px float64) bool {
	if offscreen(cam, sx, sy, px) {
		return false
	}
	r.bodyPrims++
	return true
}
