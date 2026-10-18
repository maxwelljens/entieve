package screens

import (
	"testing"

	"github.com/entieve/entieve/sim"
	"github.com/entieve/entieve/uikit/theme"
	"github.com/entieve/entieve/uikit/widgets"
)

// finView is the Finances screen's ContentView: the finance reads plus the
// wallet. It embeds a nil ContentView, so a read beyond these panics
// instead of quietly finding one — no simulation exists in this test.
type finView struct {
	widgets.ContentView
	cash, deployed, receivables float64
	entries                     []sim.LedgerEntry
}

func (v *finView) Wallet() float64 { return v.cash }

func (v *finView) Deployed() float64 { return v.deployed }

func (v *finView) Receivables() float64 { return v.receivables }

func (v *finView) Ledger() []sim.LedgerEntry { return v.entries }

// TestFinancesRendersHoldingsAndLedger: the window builds, syncs and
// renders from the seams alone — money in the NAMING.md euro form
// ("-1,234.50 €"), a trade's broker fee and sales tax itemized inside its
// own row, and the ledger newest first.
func TestFinancesRendersHoldingsAndLedger(t *testing.T) {
	th := theme.NewSeam(theme.DefaultPalette(), theme.DefaultTypeScale())
	cv := &finView{
		cash: 1234.5, deployed: 90, receivables: 30000,
		entries: []sim.LedgerEntry{
			{Tick: 60, What: "Repairs — 250.0 HP", Where: "Jernborg", Amount: -1234.5},
			{Tick: 120, What: "Market buy — 10 x EMP Charge", Where: "Jernborg", Amount: -51.5, Fee: 1.5},
			{Tick: 223380, What: "Contract — Belt cleanup", Where: "Jernborg", Amount: 30000},
		},
	}
	f := NewFinances(widgets.NewSeams(th, cv, &recordingSink{}))
	f.Sync()

	if len(f.tbl.Rows) != 3 {
		t.Fatalf("rows = %d, want the 3 stub movements", len(f.tbl.Rows))
	}
	// Newest first: the last movement recorded is the top row.
	if got := f.tbl.Rows[0].Cells[1]; got != "Contract — Belt cleanup" {
		t.Errorf("top row = %q, want the newest movement", got)
	}
	if got := f.tbl.Rows[0].Cells[0]; got != "T+01:02:03" {
		t.Errorf("when cell = %q, want the elapsed sim clock", got)
	}
	if got := f.tbl.Rows[0].Cells[3]; got != "30,000.00 €" {
		t.Errorf("credit cell = %q, want the euro form", got)
	}
	// A negative amount keeps its sign in front (NAMING.md money strings).
	if got := f.tbl.Rows[2].Cells[3]; got != "-1,234.50 €" {
		t.Errorf("debit cell = %q, want \"-1,234.50 €\"", got)
	}
	// The fee is itemized inside the trade's own row — not a row of its own.
	if got := f.tbl.Rows[1].Cells[1]; got != "Market buy — 10 x EMP Charge · fee 1.50 €" {
		t.Errorf("trade row = %q, want the broker fee itemized", got)
	}
	if got := f.tbl.Rows[1].Cells[3]; got != "-51.50 €" {
		t.Errorf("trade net = %q, want the net amount", got)
	}
	if got := f.tbl.Rows[1].Cells[2]; got != "Jernborg" {
		t.Errorf("where cell = %q, want the station", got)
	}

	// Holdings: the three pools and their total.
	for _, c := range []struct {
		name string
		got  string
		want string
	}{
		{"cash", f.cash.Label, "1,234.50 €"},
		{"deployed", f.deployed.Label, "90.00 €"},
		{"receivables", f.receivables.Label, "30,000.00 €"},
		{"total", f.total.Label, "31,324.50 €"},
	} {
		if c.got != c.want {
			t.Errorf("%s = %q, want %q", c.name, c.got, c.want)
		}
	}

	// An empty ledger renders no rows and still reads the holdings.
	cv.entries = nil
	f.Sync()
	if len(f.tbl.Rows) != 0 {
		t.Errorf("empty ledger rendered %d rows", len(f.tbl.Rows))
	}
}
