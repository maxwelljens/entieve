package screens

import (
	"math"
	"os"
	"strings"
	"testing"

	"github.com/entieve/entieve/content"
	"github.com/entieve/entieve/internal/uitest"
	"github.com/entieve/entieve/sim"
	"github.com/entieve/entieve/uikit/theme"
	"github.com/entieve/entieve/uikit/widgets"
)

// TestTradePreviewMatchesTheDebit: the picker's trade line is the figure the
// command itself moves — the sim's quote walk composed with its own fee and
// tax (CONTEXT.md "Quote"), pinned against the wallet rather than against a
// second copy of the formula.
func TestTradePreviewMatchesTheDebit(t *testing.T) {
	env, cmds := newTestEnv(t)
	ui := newQtyUI(t, env)
	q := env.QtyWindow()
	env.Sim.Docked.Set(env.Sim.Player(), sim.Docked{At: "cel-haven-rest"})
	frame := func() { pumpQty(t, ui, env, func() {}, 1024, 700) }

	// Buy: the bare ask is 120 € a unit, the debit carries the 3 % fee.
	env.OpenQty(widgets.QtyPick{Mode: widgets.QtyBuy, Item: "ammo-emp-s"})
	frame()
	q.Slider().SetValue(qtyMax)
	frame()
	if got, want := q.DetailText(), "100 × 123.60 € ≈ 12,360.00 €"; got != want {
		t.Fatalf("buy preview = %q, want %q", got, want)
	}
	wallet := env.Sim.Wallet()
	uitest.FireButton(ui, q.Confirm())
	frame()
	env.Sim.Queue(lastCommand(t, cmds))
	env.Sim.Step()
	if got := env.Sim.Wallet(); math.Abs(wallet-got-12360) > 1e-9 {
		t.Fatalf("the buy debited %v, want the previewed 12,360.00 €", wallet-got)
	}

	// Sell: the preview carries the fee and the tax, and the credit is the
	// same figure. The book has moved a tick on, so this half pins the preview
	// against the measured credit rather than a fixture number.
	env.OpenQty(widgets.QtyPick{Mode: widgets.QtySell, Item: "ammo-emp-s"})
	frame()
	q.Slider().SetValue(qtyMax)
	frame()
	preview := q.DetailText()
	wallet = env.Sim.Wallet()
	uitest.FireButton(ui, q.Confirm())
	frame()
	env.Sim.Queue(lastCommand(t, cmds))
	env.Sim.Step()
	if credit := env.Sim.Wallet() - wallet; credit <= 0 || !strings.HasSuffix(preview, theme.FormatCredits(credit)) {
		t.Fatalf("the sell credited %v, want the previewed %q", credit, preview)
	}
}

// TestTradePreviewClampsToTheBookAndTheHold: an amount past the depth shows
// what actually fills, and an amount past the hold prices only the hold — the
// command's own two clamps.
func TestTradePreviewClampsToTheBookAndTheHold(t *testing.T) {
	env, _ := newTestEnv(t)
	ui := newQtyUI(t, env)
	q := env.QtyWindow()
	env.Sim.Docked.Set(env.Sim.Player(), sim.Docked{At: "cel-haven-rest"})
	frame := func() { pumpQty(t, ui, env, func() {}, 1024, 700) }

	// A side the book holds nothing on dashes: nobody bids for the round.
	env.OpenQty(widgets.QtyPick{Mode: widgets.QtySell, Item: "ammo-emp-s"})
	frame()
	q.Slider().SetValue(qtyMax)
	frame()
	if got, want := q.DetailText(), "— · no buy order"; got != want {
		t.Errorf("no-bid preview = %q, want %q", got, want)
	}

	// The fixture rests six barges at 750,000 €: a hundred asked fills six, so
	// the line names the six and prices only those (the fee rides the fill).
	env.OpenQty(widgets.QtyPick{Mode: widgets.QtyBuy, Item: "ship-husnek"})
	frame()
	q.Slider().SetValue(qtyMax)
	frame()
	if got, want := q.DetailText(), "6 of 100 × 772,500.00 € ≈ 4,635,000.00 €"; got != want {
		t.Fatalf("short-book preview = %q, want %q", got, want)
	}

	// With a bid resting, a hundred asked sells only the twelve rounds the
	// hold carries: the line prices those, exactly as the command credits.
	if _, ok := env.Sim.PlaceOrder("cel-haven-rest", sim.OrderBuy, "ammo-emp-s", 100, 110, sim.OwnerSystem); !ok {
		t.Fatal("fixture: the bid was refused")
	}
	env.OpenQty(widgets.QtyPick{Mode: widgets.QtySell, Item: "ammo-emp-s"})
	frame()
	q.Slider().SetValue(qtyMax)
	frame()
	if got, want := q.DetailText(), "12 × 103.95 € ≈ 1,247.40 €"; got != want {
		t.Fatalf("hold-clamped preview = %q, want %q", got, want)
	}
}

// lastCommand takes the command the picker's last Confirm enqueued.
func lastCommand(t *testing.T, cmds *[]sim.Command) sim.Command {
	t.Helper()
	if len(*cmds) == 0 {
		t.Fatal("Confirm enqueued no command")
	}
	cmd := (*cmds)[len(*cmds)-1]
	*cmds = (*cmds)[:len(*cmds)-1]
	return cmd
}

// TestNoPriceWalkInScreens: the walk that prices an amount lives in the sim
// (CONTEXT.md "Quote"); a screen composes the display from it and holds no
// second copy. The picker reads the quote, never the displayed-price ladder.
//
// The ladder has exactly one surviving reader in the screens: the market
// catalog's per-row quote column (a display figure, never a trade). Pinned
// here so no trade path can start pricing off it and quietly disagree with
// the picker.
func TestNoPriceWalkInScreens(t *testing.T) {
	ents, err := os.ReadDir(".")
	if err != nil {
		t.Fatalf("read package dir: %v", err)
	}
	gone := []string{"bookValue", "crossAvail", "marketStations"}
	for _, e := range ents {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".go") || strings.HasSuffix(e.Name(), "_test.go") {
			continue
		}
		src, err := os.ReadFile(e.Name())
		if err != nil {
			t.Fatalf("read %s: %v", e.Name(), err)
		}
		for _, name := range gone {
			if strings.Contains(string(src), name) {
				t.Errorf("%s still mentions %s: the walk lives in the sim", e.Name(), name)
			}
		}
		n := strings.Count(string(src), "PriceLadder(")
		if e.Name() != "market.go" && n > 0 {
			t.Errorf("%s reads the price ladder: a trade prices with the sim quote", e.Name())
		}
		if e.Name() == "market.go" {
			if n != 1 {
				t.Errorf("market.go has %d ladder reads, want the one quote column", n)
			}
			for i, line := range strings.Split(string(src), "\n") {
				if strings.Contains(line, "PriceLadder(") && !strings.Contains(line, "quote :=") {
					t.Errorf("market.go:%d reads the ladder outside the quote column", i+1)
				}
			}
		}
	}
}

// TestQuoteSeamIsTheSim: the seam's two reads forward the sim's own answers,
// so a screen cannot price an amount the command would price differently.
func TestQuoteSeamIsTheSim(t *testing.T) {
	env, _ := newTestEnv(t)
	env.Sim.Docked.Set(env.Sim.Player(), sim.Docked{At: "cel-haven-rest"})
	for _, item := range []content.ItemID{"ammo-emp-s", "min-ferrite", "loot-navdata"} {
		want := env.Sim.Quote("cel-haven-rest", item, sim.OrderBuy, 25)
		if got := env.Market().Quote("cel-haven-rest", item, sim.OrderBuy, 25); got.Walk != want.Walk || got.Priced != want.Priced {
			t.Errorf("Quote(%s) = %+v, want the sim's %+v", item, got, want)
		}
		price, ok := env.Sim.CheapestAsk(item)
		if got, gotOK := env.Market().CheapestAsk(item); got != price || gotOK != ok {
			t.Errorf("CheapestAsk(%s) = %v/%v, want the sim's %v/%v", item, got, gotOK, price, ok)
		}
	}
}
