package game

import (
	"image"

	"github.com/entieve/entieve/uikit"
)

// Docked service-window composition (docs/GAMEPLAY.md, "Docked at a
// station").
//
// While docked the screen splits into two columns: the station window
// owns the fixed left one (contract board, content height — see
// contracts.go) and the log keeps its bottom-left pocket under it. The
// three service windows then tile the viewport to the right of that
// column: the market takes the full top band (its columns need the
// width), and inventory/fitting split the band below it in half. The
// rects come out pairwise disjoint and fully on-screen by construction —
// they are gutters of dockGutter inside one bounding rect.
//
// Chrome clearance: the composition clears the station column and the
// log pocket (the only chrome on screen while docked). The HUD bar, the
// module rack and the scanner are space chrome — syncMounts hides or
// unmounts all three while docked — so their pockets are free for the
// docked set; reserving them as well is arithmetically impossible at the
// 1024×700 minimum: station 394×426 + log 288×240 + HUD bar 420×130 +
// scanner 300×248 + the trio's content minimums (300×240, 300×420,
// 300×430) leave ≈28,000 px² of slack, less than the screen margins
// (≈32,000 px²) need, before a single gutter.
//
// Widths: the market band is the whole service width — 608 px at
// 1024×700, 864 px at 1280×720, 1504 px at 1920×1080 — over the 560 px
// the trade columns need to render un-truncated (the shipped boards
// measure ≈357 px of natural column width, so even the minimum size
// reads whole; the fixed 300 px rect the constructor used to carry was
// what ellipsized them). The bottom row splits what is left in half:
// 300 px per window at 1024, 428 px at 1280, 748 px at 1920.
//
// Validity: every rect is ≥ its window's MinSize at every viewport the
// window system supports (Layout clamps to ≥1024×700), and the exact
// tuples are pinned by TestStationLayoutDisjoint1024/1280/1920.
const (
	// dockGutter is the gap between composed windows (both axes).
	dockGutter = 8

	// Content minimums mirrored from the uikit window constructors
	// (uikit/{market,inventory,fitting}.go); TestWindowMinSizesMatch
	// pins the mirrors to the real windows so they cannot drift.
	//
	// market     300×240 — 3.5 trade-table rows over the quantity
	//            toolbar (the table scrolls; one row needs ~163).
	// inventory  300×420 — every fixed row plus a full row of each hold
	//            section (the floor for that is 405; the old 440 left
	//            the bottom row short at 1024×700).
	// fitting    300×430 — stat block, resist row and a full row of the
	//            slot and hangar tables (floor: 430; was 400, which
	//            clipped those tables to a partial row).
	marketMinW, marketMinH       = 300, 240
	inventoryMinW, inventoryMinH = 300, 420
	fittingMinW, fittingMinH     = 300, 430
)

// dockedLayout is the composed docked window set for one viewport.
type dockedLayout struct {
	Station   image.Rectangle // contract board, content height
	Market    image.Rectangle
	Inventory image.Rectangle
	Fitting   image.Rectangle
}

// stationLayout composes the docked set for a w×h viewport. stationH is
// the contract board's content height (the station window hugs its own
// board; the rest of the layout never depends on it).
//
// Pure: same inputs → same rects, no window handles, no side effects.
func stationLayout(w, h, stationH int) dockedLayout {
	// Bounding box of the service area: right of the station column,
	// inside the top/side/bottom margins.
	// Design constants × the current scale (once per quantity): the
	// viewport (w, h) already arrives in live px.
	x0 := uikit.Px(stationX + stationWinW + 2) // 406: station column + gutter
	y0 := uikit.Px(stationY)                   // 8
	x1 := w - uikit.Px(10)
	y1 := h - uikit.Px(10)

	// The market takes the top band: a third of the free height, clamped
	// between its own minimum and whatever leaves the bottom row its
	// content minimum. The bottom row (inventory | fitting) gets the
	// rest, so it can never come back below max(inventory, fitting).
	avail := (y1 - y0) - uikit.Px(dockGutter)
	// The floors mirror the uikit windows' own uikit.Px(min) MinSizes, so
	// they scale the same way or the clamp would fight the window system.
	bottomMin := max(uikit.Px(inventoryMinH), uikit.Px(fittingMinH))
	mktH := clampInt(avail/3, uikit.Px(marketMinH), max(uikit.Px(marketMinH), avail-bottomMin))

	by := y0 + mktH + uikit.Px(dockGutter)
	invW := ((x1 - x0) - uikit.Px(dockGutter)) / 2
	return dockedLayout{
		Station: image.Rect(uikit.Px(stationX), uikit.Px(stationY),
			uikit.Px(stationX+stationWinW), uikit.Px(stationY)+max(stationH, uikit.Px(stationMinH))),
		Market:    image.Rect(x0, y0, x1, y0+mktH),
		Inventory: image.Rect(x0, by, x0+invW, y1),
		Fitting:   image.Rect(x0+invW+dockGutter, by, x1, y1),
	}
}
