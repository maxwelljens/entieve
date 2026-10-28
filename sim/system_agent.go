package sim

import "sort"

// ---------------------------------------------------------------------------
// System 18 — ledger agents.
//
// Appended after industry (17) at the end of Step, before the deferred
// death respawn:
//
//	agentSettle  — every tick: detect and settle fills against the agent's
//	               own resting orders (the counterparty side of a trade
//	               lands here one system later: a player's market command,
//	               a convoy withdrawal, a respawn requisition).
//	agentPass    — every tick, gated per agent by its own stateless
//	               cooldown: beliefs, imputation, one capital step,
//	               extraction, haul wishes (agent.go).
//	upkeepPass   — every agentUpkeepTicks: the station repair/rearm
//	               basket and its maintenance-fee money sink.
//
// Determinism: agents are held in ascending-id order, every list it
// touches is sorted, the cooldown gate is a pure hash of (tick, agent id)
// — restore reproduces it from the tick count alone — and it draws no RNG
// (rng_roles_test's master-stream pin holds unchanged).
// ---------------------------------------------------------------------------

// agentSettle reconciles every agent's resting-order ledger against the
// books: an order the book no longer has (fully filled or withdrawn) or
// whose remaining qty dropped settles the difference — buys debit capital
// and credit inventory, sells credit capital net of the sales tax and
// debit inventory — then the ledger is re-anchored to the book. Runs
// every tick so no fill can outlive its escrow.
func (s *Sim) agentSettle() {
	for i := range s.agents {
		a := &s.agents[i]
		if len(a.Orders) == 0 {
			continue
		}
		kept := make([]AgentOrder, 0, len(a.Orders))
		for _, o := range a.Orders { // ascending id: canonical
			remaining := o.Qty
			book := s.market[o.Station]
			if book == nil {
				s.agentSettleFill(a, o, remaining)
				continue
			}
			var found bool
			for _, b := range book.Orders { // canonical book order
				if b.ID == o.ID {
					found = true
					if b.Qty < remaining {
						s.agentSettleFill(a, o, remaining-b.Qty)
						remaining = b.Qty
					} else {
						remaining = b.Qty
					}
					break
				}
			}
			if !found {
				s.agentSettleFill(a, o, remaining)
				continue
			}
			if remaining > 0 {
				o.Qty = remaining
				kept = append(kept, o)
			}
		}
		a.Orders = kept
		sort.Slice(a.Orders, func(i, j int) bool { return a.Orders[i].ID < a.Orders[j].ID })
	}
}

// agentSettleFill applies qty units of one resting order's execution:
// the resting buy's escrow buys goods (capital out, inventory in), the
// resting sell's goods are paid net of the sales tax (inventory out,
// capital in). Inventory is clamped at zero — the invariant is that a
// sell order's units were escrowed in inventory when it was placed.
func (s *Sim) agentSettleFill(a *Agent, o AgentOrder, qty int) {
	if qty <= 0 {
		return
	}
	switch o.Side {
	case OrderBuy:
		a.Capital -= o.Price * float64(qty)
		addInv(a, o.Item, qty)
	case OrderSell:
		a.Capital += o.Price * float64(qty) * (1 - MarketSalesTax)
		s.routeFee(MarketSalesTax * o.Price * float64(qty))
		takeInv(a, o.Item, qty)
	}
}

// systemAgent runs system 18: settle resting agent fills, then (per agent
// cooldown) the behavior pass and, on the clock, the station upkeep cycle.
// No-op in worlds without a roster (the showcase tour and fixtures).
func (s *Sim) systemAgent() {
	if len(s.agents) == 0 {
		return
	}
	s.agentSettle()
	if s.ticks > 0 {
		s.agentPass()
	}
	if s.ticks > 0 && s.ticks%agentUpkeepTicks == 0 {
		s.upkeepPass()
	}
}
