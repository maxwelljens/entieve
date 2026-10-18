package sim

// The player's transaction ledger: one entry per wallet movement, kept as
// a capped ring in the Save (Save.Ledger). recordLedger is the single
// choke point every debit and credit site goes through — market trades
// (sim/system_economy.go), bounties (sim/system_damage.go) and contract
// payouts (sim/system_mission.go) — so each movement is recorded exactly
// once and the ring can never exceed LedgerCap.

// LedgerCap is the ring size of the ledger: the newest LedgerCap wallet
// movements, oldest evicted.
const LedgerCap = 200

// LedgerEntry is one recorded wallet movement: the NET amount of a single
// transaction (negative debits, positive credits) with a trade's broker
// fee and sales tax itemized inside the same row. What is the counterparty
// or item, Where the station or system it settled at, and Tick the Step
// count it settled on.
type LedgerEntry struct {
	Tick   int64   `json:"tick"`
	What   string  `json:"what"`
	Where  string  `json:"where"`
	Amount float64 `json:"amount"`
	Fee    float64 `json:"fee,omitempty"`
	Tax    float64 `json:"tax,omitempty"`
}

// recordLedger appends one wallet movement to the ring (oldest evicted
// past LedgerCap) — the only way an entry is ever added.
func (s *Sim) recordLedger(e LedgerEntry) {
	e.Tick = s.ticks
	s.ledger = append(s.ledger, e)
	s.trimLedger()
}

// trimLedger keeps the newest LedgerCap entries. Restore applies it too,
// so a hand-edited Save cannot grow the ring past its cap.
func (s *Sim) trimLedger() {
	if n := len(s.ledger) - LedgerCap; n > 0 {
		s.ledger = append(s.ledger[:0], s.ledger[n:]...)
	}
}

// Ledger returns a copy of the recorded wallet movements, oldest first.
func (s *Sim) Ledger() []LedgerEntry {
	return append([]LedgerEntry(nil), s.ledger...)
}

// Deployed is the player's capital held in open buy orders: the escrow
// retained at placement (order limit × quantity), still out of the wallet
// and refundable in full on cancel (the broker fee charged at placement
// is kept). Sell orders escrow goods, not money, so they never count.
func (s *Sim) Deployed() float64 {
	total := 0.0
	for _, st := range s.stationsSorted() {
		book := s.market[st]
		if book == nil {
			continue
		}
		for _, o := range book.Orders {
			if o.Owner == OwnerPlayer && o.Side == OrderBuy {
				total += o.Price * float64(o.Qty)
			}
		}
	}
	return total
}

// Receivables is the credits still owed on accepted contracts: their
// reward, paid when the contract completes.
func (s *Sim) Receivables() float64 {
	total := 0.0
	for _, m := range s.missions {
		if def, ok := s.Content.Contract(m.ID); ok {
			total += def.Reward.Credits
		}
	}
	return total
}

// systemName is the current system's display name ("" when unknown) — the
// "where" of a movement that happened in space rather than at a station.
func (s *Sim) systemName() string {
	if sys := s.System(); sys != nil {
		return sys.Name
	}
	return ""
}
