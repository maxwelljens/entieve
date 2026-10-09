package ui

import (
	"sort"

	"github.com/entieve/entieve/sim"
)

// SortColumn selects an overview sort key.
type SortColumn int

// Overview sort columns (icon column sorts by kind).
const (
	SortKind SortColumn = iota
	SortName
	SortDistance
	SortVelocity
)

// SortOverview sorts rows in place by the given column. The sort is stable:
// rows with equal keys keep their previous relative order (tested).
// asc selects ascending order; descending reverses the comparison while
// preserving stability.
func SortOverview(rows []sim.Info, col SortColumn, asc bool) {
	less := func(a, b sim.Info) bool {
		switch col {
		case SortKind:
			return a.Kind < b.Kind
		case SortName:
			return a.Name < b.Name
		case SortDistance:
			return a.Distance < b.Distance
		case SortVelocity:
			return a.Vel.Len() < b.Vel.Len()
		default:
			return false
		}
	}
	sort.SliceStable(rows, func(i, j int) bool {
		if asc {
			return less(rows[i], rows[j])
		}
		return less(rows[j], rows[i])
	})
}
