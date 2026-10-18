package theme

// Spacing tokens: the furniture rhythm, in design units through the Px
// funnel. Furniture — buttons, title bars, scrollbar rails, panel gaps —
// takes its air from here. Data rows keep their own denser scale (the
// table's cellPad/entryPadV literals): information density in tables is
// deliberate (EVE-like), so these tokens never reach a row's metrics.
const (
	SpSm = 4  // hairline air: insets and rail padding
	SpMd = 8  // standard furniture gap: button and title-bar padding
	SpLg = 16 // the scrollbar handle: a visible length, not a gap
)

// ScrollGutter is the design width every scroll viewport reserves beside
// its content for the scrollbar rail: the handle (SpLg) plus SpSm of air
// on each side — exactly the slider's own preferred width, so the rail
// and its gutter agree. The theme's handle is Px(SpLg); a rail narrower
// than it let the handle overhang into the content ("the scrollbar is
// laid over content" — the old 8 px market rail). The gutter is reserved
// even while the content fits, so the viewport never jumps when a long
// list starts overflowing.
const ScrollGutter = SpLg + 2*SpSm // 24
