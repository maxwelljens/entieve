package sim

import "testing"

// TestShipBasePriceBacksTheLadder pins the hull rung of the price ladder:
// a book with no trade and no orders quotes the hull's def base price, so
// a ship row never reads 0 € (content/data/ships carries base_price).
func TestShipBasePriceBacksTheLadder(t *testing.T) {
	s := newTestSim(t)
	s.Content.Ships["ship-vastrek"].BasePrice = 360_000

	if got := s.marketPrice(nil, "ship-vastrek"); got != 360_000 {
		t.Fatalf("marketPrice(nil, ship-vastrek) = %v, want the def base price 360000", got)
	}
	if got := s.MarketPrice("cel-station-a", "ship-vastrek"); got != 360_000 {
		t.Fatalf("MarketPrice(empty book, ship-vastrek) = %v, want 360000", got)
	}
}
