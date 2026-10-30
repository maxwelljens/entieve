package sim

import "testing"

// Behaviour-tree core semantics, pinned before implementation:
//
//	Sequence — children evaluate in declaration order. The first Failure
//	stops the run and the node fails; a Running stops the run and
//	propagates; every Success means Success. No children: Success
//	(vacuously true: nothing to fail).
//
//	Selector — children evaluate in declaration order. The first Success
//	wins and stops the run; a Running stops the run and propagates; all
//	Failure means Failure. No children: Failure (nothing handles the tick).
//
//	A tick is synchronous and allocation-free: a tree's children slices are
//	static package data and the context is caller-owned (see btNode).

func btRecorder(order *[]string, name string, st btStatus) btNode {
	return btLeaf(name, func(*btCtx) btStatus {
		*order = append(*order, name)
		return st
	})
}

// TestBTSequence pins ordering, short-circuit and Running propagation.
func TestBTSequence(t *testing.T) {
	var order []string
	var c btCtx
	cases := []struct {
		name string
		tree btNode
		want btStatus
		ran  []string
	}{
		{"all success", btSeq("s", btRecorder(&order, "a", btSuccess), btRecorder(&order, "b", btSuccess)), btSuccess, []string{"a", "b"}},
		{"failure stops", btSeq("s", btRecorder(&order, "a", btSuccess), btRecorder(&order, "b", btFailure), btRecorder(&order, "c", btSuccess)), btFailure, []string{"a", "b"}},
		{"first fails", btSeq("s", btRecorder(&order, "a", btFailure), btRecorder(&order, "b", btSuccess)), btFailure, []string{"a"}},
		{"running stops", btSeq("s", btRecorder(&order, "a", btRunning), btRecorder(&order, "b", btSuccess)), btRunning, []string{"a"}},
		{"empty", btSeq("s"), btSuccess, nil},
	}
	for _, tc := range cases {
		order = nil
		if got := tc.tree.Tick(&c); got != tc.want {
			t.Errorf("%s: status = %v, want %v", tc.name, got, tc.want)
		}
		if len(order) != len(tc.ran) {
			t.Fatalf("%s: ran %v, want %v", tc.name, order, tc.ran)
		}
		for i := range order {
			if order[i] != tc.ran[i] {
				t.Fatalf("%s: ran %v, want %v", tc.name, order, tc.ran)
			}
		}
	}
}

// TestBTSelector pins first-success-continues/first-failure-stops ordering
// and Running propagation.
func TestBTSelector(t *testing.T) {
	var order []string
	var c btCtx
	cases := []struct {
		name string
		tree btNode
		want btStatus
		ran  []string
	}{
		{"second wins", btSel("s", btRecorder(&order, "a", btFailure), btRecorder(&order, "b", btSuccess), btRecorder(&order, "c", btSuccess)), btSuccess, []string{"a", "b"}},
		{"first wins", btSel("s", btRecorder(&order, "a", btSuccess), btRecorder(&order, "b", btSuccess)), btSuccess, []string{"a"}},
		{"all fail", btSel("s", btRecorder(&order, "a", btFailure), btRecorder(&order, "b", btFailure)), btFailure, []string{"a", "b"}},
		{"running stops", btSel("s", btRecorder(&order, "a", btRunning), btRecorder(&order, "b", btSuccess)), btRunning, []string{"a"}},
		{"empty", btSel("s"), btFailure, nil},
	}
	for _, tc := range cases {
		order = nil
		if got := tc.tree.Tick(&c); got != tc.want {
			t.Errorf("%s: status = %v, want %v", tc.name, got, tc.want)
		}
		if len(order) != len(tc.ran) {
			t.Fatalf("%s: ran %v, want %v", tc.name, order, tc.ran)
		}
		for i := range order {
			if order[i] != tc.ran[i] {
				t.Fatalf("%s: ran %v, want %v", tc.name, order, tc.ran)
			}
		}
	}
}

// TestBTBranchRecordsWinningChild pins the trace hook: the top-level selector
// records the name of the child that handled the tick, and nothing else.
func TestBTBranchRecordsWinningChild(t *testing.T) {
	tree := btSel("root",
		btSeq("idle", btLeaf("no", func(*btCtx) btStatus { return btFailure })),
		btSeq("engaged", btLeaf("yes", func(*btCtx) btStatus { return btSuccess })),
	)
	var c btCtx
	if got := tree.Tick(&c); got != btSuccess {
		t.Fatalf("status = %v", got)
	}
	if c.branch != "engaged" {
		t.Fatalf("branch = %q, want engaged", c.branch)
	}
}

// TestBTTickDoesNotAllocate pins the steady-state contract: ticking a built
// tree allocates nothing (static children, caller-owned context).
func TestBTTickDoesNotAllocate(t *testing.T) {
	tree := btSel("root",
		btSeq("a", btLeaf("1", btNoop), btLeaf("2", btNoop)),
		btSeq("b", btLeaf("3", btNoop), btLeaf("4", btNoop)),
	)
	var c btCtx
	if n := testing.AllocsPerRun(1000, func() { tree.Tick(&c) }); n != 0 {
		t.Fatalf("tree tick allocates %v per run, want 0", n)
	}
}

func btNoop(*btCtx) btStatus { return btSuccess }
