// Module tools/emblemraster is Entieve's build-time emblem rasterizer: it
// turns uikit/emblems/svg/*.svg into the embedded PNGs. It is a separate
// module so the game binary never links a rasterizer (see main.go and
// uikit/emblems/emblems.go for the regeneration command).
module github.com/entieve/entieve/tools/emblemraster

go 1.27.1

require github.com/entieve/entieve/tools/svgraster v0.0.0-00010101000000-000000000000

require (
	github.com/srwiley/rasterx v0.0.0-20220730225603-2ab79fcdd4ef // indirect
	golang.org/x/image v0.46.0 // indirect
	golang.org/x/sys v0.48.0 // indirect
)

replace github.com/entieve/entieve/tools/svgraster => ../svgraster
