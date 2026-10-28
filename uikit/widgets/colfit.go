package widgets

import "github.com/entieve/entieve/uikit/theme"

// ColWidth is one column's measurement input and fitted output for
// fitColumns: Width is the natural (content + padding) width on input
// and the fitted width on output; Min is the shrink floor (header +
// padding).
type ColWidth struct {
	Label string
	Width float64
	Min   float64
}

// Ellipsize shortens s with a trailing "…" so the result measures at most
// w, using measure (which must be monotonic in string length). An empty
// string is returned when even the ellipsis does not fit. The body lives in
// theme (theme <- widgets), which needs it for the title bar's own overflow.
func Ellipsize(s string, w float64, measure func(string) float64) string {
	return theme.Ellipsize(s, w, measure)
}
