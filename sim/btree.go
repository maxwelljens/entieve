package sim

// A minimal behaviour tree for NPC role policy (see docs/SIM.md, "NPC
// behaviour"). Nodes are values, not interfaces: a role's tree is a
// package-level static, so its children slices are never rebuilt, and one
// caller-owned btCtx keeps a tick allocation-free. Evaluation is synchronous
// — no clock, no goroutine, no RNG — and every leaf is a pure read of Sim
// state plus the decision it writes into the context (the NPC's persisted
// NpcAI fields stay the memory; a tree is re-evaluated from scratch each
// tick).

// btStatus is one node's outcome for the tick.
type btStatus uint8

const (
	// btFailure: the node does not apply this tick (a selector moves on).
	btFailure btStatus = iota
	// btSuccess: the node applied (or its sequence completed).
	btSuccess
	// btRunning: the node consumed the tick without resolving; selection
	// stops and propagates it unchanged.
	btRunning
)

// btNode is a leaf (fn) or a combinator (kind + children).
type btNode struct {
	name string
	kind btKind
	fn   func(*btCtx) btStatus
	kids []btNode
}

type btKind uint8

const (
	btLeafNode btKind = iota
	btSequenceNode
	btSelectorNode
)

// btLeaf wraps a rule: it returns what it did this tick.
func btLeaf(name string, fn func(*btCtx) btStatus) btNode {
	return btNode{name: name, kind: btLeafNode, fn: fn}
}

// btSeq runs its children in order, stopping on the first non-success.
func btSeq(name string, kids ...btNode) btNode {
	return btNode{name: name, kind: btSequenceNode, kids: kids}
}

// btSel runs its children in order, stopping on the first non-failure. The
// child that handled the tick is recorded in ctx.branch (the trace's
// "which branch handled this tick").
func btSel(name string, kids ...btNode) btNode {
	return btNode{name: name, kind: btSelectorNode, kids: kids}
}

// Tick evaluates the tree once for ctx.
func (n *btNode) Tick(c *btCtx) btStatus { return n.tick(c, 0) }

func (n *btNode) tick(c *btCtx, depth int) btStatus {
	switch n.kind {
	case btSequenceNode:
		for i := range n.kids {
			if st := n.kids[i].tick(c, depth+1); st != btSuccess {
				return st
			}
		}
		return btSuccess
	case btSelectorNode:
		for i := range n.kids {
			st := n.kids[i].tick(c, depth+1)
			if st == btFailure {
				continue
			}
			if depth == 0 {
				c.branch = n.kids[i].name
			}
			return st
		}
		return btFailure
	default:
		return n.fn(c)
	}
}
