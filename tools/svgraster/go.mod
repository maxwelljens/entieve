// Module tools/svgraster is Entieve's shared build-time SVG rasterizer: the
// subset parser and scanner rasterizer behind tools/emblemraster and
// tools/hullraster. It is a separate module so the game binary never links a
// rasterizer (see svgraster.go).
module github.com/entieve/entieve/tools/svgraster

go 1.27.1

require (
	github.com/srwiley/rasterx v0.0.0-20220730225603-2ab79fcdd4ef
	golang.org/x/image v0.46.0
)

require golang.org/x/sys v0.48.0 // indirect
