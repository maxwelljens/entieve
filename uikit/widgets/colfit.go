package widgets

import "github.com/entieve/entieve/uikit/theme"

// ---------------------------------------------------------------------------
// Column fitting — FitColumns is the legacy elastic fit the overview
// shipped with (surplus to Name, shrink order Name-last); Table uses the
// generic fitColumns with per-column priorities. Pure and unit-tested.
// ---------------------------------------------------------------------------
// ColWidth is one column's measurement input and fitted output for
// FitColumns: Width is the natural (content + padding) width on input
// and the fitted width on output; Min is the shrink floor (header +
// padding).
type ColWidth struct {
	Label string
	Width float64
	Min   float64
}

// FitColumns fits natural column widths into avail pixels. When there is
// surplus space it is all given to the Name column; when space is short,
// columns shrink toward their Min, later columns first (Velocity gives
// way before Name), and Name never drops below its Min (its cells
// ellipsize instead). Pure and unit-tested.
func FitColumns(cols []ColWidth, avail float64) []ColWidth {
	prio := make([]int, len(cols))
	for i := range prio {
		prio[i] = i // shrink order: highest index yields first
	}
	return fitColumns(cols, prio, nameColumn(cols), avail)
}

// nameColumn returns the index of the column named "Name".
func nameColumn(cols []ColWidth) int {
	for i, c := range cols {
		if c.Label == "Name" {
			return i
		}
	}
	return 0
}

// Ellipsize shortens s with a trailing "…" so the result measures at most
// w, using measure (which must be monotonic in string length). An empty
// string is returned when even the ellipsis does not fit. The body lives in
// theme (theme <- widgets), which needs it for the title bar's own overflow.
func Ellipsize(s string, w float64, measure func(string) float64) string {
	return theme.Ellipsize(s, w, measure)
}
